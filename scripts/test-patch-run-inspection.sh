#!/usr/bin/env bash
set -euo pipefail
repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT
mkdir -p "$work_dir/openapi" "$work_dir/spec"
printf 'module inspectionfixture\n\ngo 1.22\n' > "$work_dir/go.mod"
python3 - "$work_dir" <<'PY'
import json
from pathlib import Path
import re
import sys
root = Path(sys.argv[1])
branches = {
    'AvailableInitialAuthoringView': ('available', 'input'),
    'UnavailableInitialAuthoringView': ('unavailable', 'reason'),
    'AvailableCanonicalIngestionPreparationView': ('preparation_available', 'preparation'),
    'UnavailableCanonicalIngestionPreparationView': ('preparation_unavailable', 'reason'),
    'AvailableCanonicalIngestionRecoveryView': ('recovery_available', 'recovery'),
    'UnavailableCanonicalIngestionRecoveryView': ('recovery_unavailable', 'reason'),
}
wrappers = {
    'ExperimentalRunConversationInspectionResponse': list(branches),
    'ExperimentalInitialAuthoringViewResponse': list(branches)[:2],
    'ExperimentalCanonicalIngestionPreparationViewResponse': list(branches)[2:4],
    'ExperimentalCanonicalIngestionRecoveryViewResponse': list(branches)[4:],
}
schemas = {}
def filename(name):
    name = re.sub(r'([A-Z]+)([A-Z][a-z])', r'\1_\2', name)
    return 'model_' + re.sub(r'([a-z0-9])([A-Z])', r'\1_\2', name).lower() + '.go'
for name, (status, payload) in branches.items():
    schemas[name] = {'type': 'object', 'additionalProperties': False,
                     'properties': {'status': {'type': 'string', 'const': status},
                                    payload: {'type': 'string'}}, 'required': ['status', payload]}
    field = payload.capitalize()
    (root / 'openapi' / filename(name)).write_text(f'''package fixture
import "encoding/json"
type {name} struct {{ Status string `json:"status"`; {field} string `json:"{payload}"` }}
func (o *{name}) UnmarshalJSON(data []byte) (err error) {{
 type alias {name}
 return json.Unmarshal(data, (*alias)(o))
}}
''')
for name, names in wrappers.items():
    schemas[name] = {'oneOf': [{'$ref': '#/components/schemas/' + child} for child in names],
                     'discriminator': {'propertyName': 'status', 'mapping': {
                         branches[child][0]: '#/components/schemas/' + child for child in names}}}
    fields = '\n'.join(f' {child} *{child}' for child in names)
    (root / 'openapi' / filename(name)).write_text(f'''package fixture
import ("encoding/json"; "fmt")
type {name} struct {{
{fields}
}}
func (dst *{name}) UnmarshalJSON(data []byte) error {{ return fmt.Errorf("unpatched inspection union") }}
var _ = json.Unmarshal
''')
schemas['LegacyInspection'] = {'type': 'object', 'additionalProperties': True,
                                'properties': {'status': {'type': 'string'}}}
(root / 'openapi/model_legacy_inspection.go').write_text('''package fixture
import "encoding/json"
type LegacyInspection struct { Status string `json:"status"` }
func (o *LegacyInspection) UnmarshalJSON(data []byte) (err error) {
 type alias LegacyInspection
 return json.Unmarshal(data, (*alias)(o))
}
''')
(root / 'spec/vertesia-openapi.json').write_text(json.dumps({'components': {'schemas': schemas}}))
PY
bash "$repo_dir/scripts/patch-openapi-permissive-decode.sh" "$work_dir/openapi"
cp -R "$work_dir/openapi" "$work_dir/first"
bash "$repo_dir/scripts/patch-openapi-permissive-decode.sh" "$work_dir/openapi"
diff -r "$work_dir/first" "$work_dir/openapi"
cat > "$work_dir/openapi/inspection_test.go" <<'GO'
package fixture
import ("encoding/json"; "testing")
func TestInspectionStatusAndClosure(t *testing.T) {
 valid := []string{
  `{"status":"available","input":"saved"}`,
  `{"status":"unavailable","reason":"not_recorded"}`,
  `{"status":"preparation_available","preparation":"saved"}`,
  `{"status":"preparation_unavailable","reason":"not_recorded"}`,
  `{"status":"recovery_available","recovery":"saved"}`,
  `{"status":"recovery_unavailable","reason":"not_recorded"}`,
 }
 var response ExperimentalRunConversationInspectionResponse
 for index, body := range valid {
  if err := json.Unmarshal([]byte(body), &response); err != nil { t.Fatalf("valid %s: %v", body, err) }
  selected := []bool{response.AvailableInitialAuthoringView != nil, response.UnavailableInitialAuthoringView != nil,
   response.AvailableCanonicalIngestionPreparationView != nil, response.UnavailableCanonicalIngestionPreparationView != nil,
   response.AvailableCanonicalIngestionRecoveryView != nil, response.UnavailableCanonicalIngestionRecoveryView != nil}
  for branch, present := range selected { if present != (branch == index) { t.Fatalf("wrong branch/reset for %s", body) } }
 }
 for _, body := range []string{
  `{}`, `null`, `{"status":"future","reason":"not_recorded"}`,
  `{"status":"preparation_unavailable","reason":"not_recorded","recovery":{}}`,
  `{"status":"preparation_unavailable","reason":"not_recorded","preparation":"saved"}`,
  `{"status":"recovery_unavailable","reason":"not_recorded","input":"saved"}`,
  `{"status":"unavailable","reason":"not_recorded","recovery":"saved"}`,
  `{"status":"preparation_available","recovery":"saved"}`,
  `{"status":"recovery_available","recovery":null}`,
  `{"status":1,"reason":"not_recorded"}`,
 } {
  if err := json.Unmarshal([]byte(body), &response); err == nil { t.Fatalf("accepted invalid %s", body) }
 }
 var preparation UnavailableCanonicalIngestionPreparationView
 if err := json.Unmarshal([]byte(`{"status":"recovery_unavailable","reason":"not_recorded"}`), &preparation); err == nil {
  t.Fatal("named preparation leaf accepted recovery status")
 }
 var view ExperimentalCanonicalIngestionPreparationViewResponse
 if err := json.Unmarshal([]byte(valid[3]), &view); err != nil { t.Fatal(err) }
 if err := json.Unmarshal([]byte(valid[5]), &view); err == nil { t.Fatal("preparation wrapper accepted recovery") }
 var initial ExperimentalInitialAuthoringViewResponse
 if err := json.Unmarshal([]byte(valid[1]), &initial); err != nil { t.Fatal(err) }
 var recovery ExperimentalCanonicalIngestionRecoveryViewResponse
 if err := json.Unmarshal([]byte(valid[5]), &recovery); err != nil { t.Fatal(err) }
 var legacy LegacyInspection
 if err := json.Unmarshal([]byte(`{"status":"old","future":1}`), &legacy); err != nil { t.Fatal(err) }
}
GO
(cd "$work_dir" && gofmt -w openapi && go test ./... && go vet ./...)
