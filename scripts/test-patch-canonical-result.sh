#!/usr/bin/env bash
# Result-root reachability plus schema-derived bounds, without tightening legacy models.
set -euo pipefail
repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT
mkdir -p "$work_dir/openapi" "$work_dir/spec"
printf 'module resultfixture\n\ngo 1.22\n' > "$work_dir/go.mod"
python3 - "$work_dir" <<'PY'
import json
from pathlib import Path
import sys
root = Path(sys.argv[1])
def obj(properties, required):
    return {'type': 'object', 'properties': properties, 'required': required, 'additionalProperties': False}
def ref(name):
    return {'$ref': '#/components/schemas/' + name}
schemas = {
    'ExperimentalCanonicalInteractionExecutionResult': obj({
        'virtual_generation': ref('ExperimentalCanonicalVirtualGenerationBinding')}, []),
    'ExperimentalCanonicalVirtualGenerationBinding': obj({
        'version': {'const': 1},
        'configured_occurrence': {'type': 'integer', 'minimum': 0, 'maximum': 255},
        'large': {'type': 'integer', 'minimum': 9007199254740992, 'maximum': 9007199254740994},
        'plan_fingerprint': {'type': 'string', 'pattern': '^sha256:[a-f0-9]{64}$'},
        'label': {'type': 'string', 'minLength': 1, 'maxLength': 2},
        'ordinary_enum': {'type': 'string', 'enum': ['known']},
        'temperature': {'type': 'number'},
        'nullable': {'type': ['string', 'null'], 'minLength': 1, 'maxLength': 2},
        'source': ref('WinnerSource')},
        ['version', 'configured_occurrence', 'large', 'plan_fingerprint', 'label', 'source']),
    'WinnerSource': obj({'revision': {'type': 'integer', 'minimum': 0}}, ['revision']),
    'UnrelatedLegacyResult': obj({'id': {'type': 'string', 'maxLength': 1}}, ['id']),
}
(root / 'spec/vertesia-openapi.json').write_text(json.dumps({'components': {'schemas': schemas}}))
fields = {
    'ExperimentalCanonicalInteractionExecutionResult': 'VirtualGeneration *ExperimentalCanonicalVirtualGenerationBinding `json:"virtual_generation,omitempty"`',
    'ExperimentalCanonicalVirtualGenerationBinding': 'Version float64 `json:"version"`; ConfiguredOccurrence float64 `json:"configured_occurrence"`; Large json.Number `json:"large"`; PlanFingerprint string `json:"plan_fingerprint"`; Label string `json:"label"`; OrdinaryEnum string `json:"ordinary_enum,omitempty"`; Temperature float64 `json:"temperature,omitempty"`; Nullable *string `json:"nullable,omitempty"`; Source WinnerSource `json:"source"`',
    'WinnerSource': 'Revision float64 `json:"revision"`',
    'UnrelatedLegacyResult': 'Id string `json:"id"`',
}
import re
for name, fields in fields.items():
    filename = re.sub(r'([a-z0-9])([A-Z])', r'\1_\2', name).lower()
    (root / 'openapi' / ('model_' + filename + '.go')).write_text(f'''package fixture
import "encoding/json"
type {name} struct {{ {fields} }}
func (o *{name}) UnmarshalJSON(data []byte) (err error) {{
 type alias {name}
 return json.Unmarshal(data, (*alias)(o))
}}
''')
PY
bash "$repo_dir/scripts/patch-openapi-permissive-decode.sh" "$work_dir/openapi"
cp -R "$work_dir/openapi" "$work_dir/first"
bash "$repo_dir/scripts/patch-openapi-permissive-decode.sh" "$work_dir/openapi"
diff -r "$work_dir/first" "$work_dir/openapi"
cat > "$work_dir/openapi/result_test.go" <<'GO'
package fixture
import ("encoding/json"; "testing"; "strings"; "fmt")
func TestResultRootClosureAndExactScalarBounds(t *testing.T) {
    fingerprint := "sha256:" + strings.Repeat("a", 64)
    winner := func(occurrence, large, label string) string {
        return fmt.Sprintf(`{"version":1,"configured_occurrence":%s,"large":%s,"plan_fingerprint":%q,"label":%q,"source":{"revision":0},"ordinary_enum":"future","temperature":0.5,"nullable":null}`, occurrence, large, fingerprint, label)
    }
    valid := []string{`{}`}
    for _, occurrence := range []string{"0", "255", "1.0", "1e0", "-0"} {
        for _, label := range []string{"😀", "😀😀", "e\u0301"} {
            valid = append(valid, `{"virtual_generation":` + winner(occurrence, "9007199254740993", label) + `}`)
        }
    }
    for _, body := range valid {
        var result ExperimentalCanonicalInteractionExecutionResult
        if err := json.Unmarshal([]byte(body), &result); err != nil { t.Fatalf("valid %s: %v", body, err) }
    }
    good := winner("1", "9007199254740993", "a")
    invalid := []string{`{"virtual_generation":null}`, `{"extra":1}`}
    for _, occurrence := range []string{"-1", "256", "0.5", "255.0000000000000000000001", `"1"`} {
        invalid = append(invalid, `{"virtual_generation":` + winner(occurrence, "9007199254740993", "a") + `}`)
    }
    for _, large := range []string{"9007199254740991.999999999999", "9007199254740994.000000000001"} {
        invalid = append(invalid, `{"virtual_generation":` + winner("1", large, "a") + `}`)
    }
    for _, label := range []string{"", "😀😀😀"} {
        invalid = append(invalid, `{"virtual_generation":` + winner("1", "9007199254740993", label) + `}`)
    }
    for _, bad := range []string{
        strings.Replace(good, `"version":1`, `"version":2`, 1),
        strings.Replace(good, fingerprint, "sha256:forged", 1),
        strings.Replace(good, `"source":{"revision":0}`, `"source":{"revision":-1}`, 1),
        strings.Replace(good, `"source":{"revision":0}`, `"source":{}`, 1),
        strings.Replace(good, `"label":"a"`, `"label":null`, 1),
        strings.Replace(good, `"version":1`, `"future":true,"version":1`, 1),
    } { invalid = append(invalid, `{"virtual_generation":` + bad + `}`) }
    for _, body := range invalid {
        var result ExperimentalCanonicalInteractionExecutionResult
        if err := json.Unmarshal([]byte(body), &result); err == nil { t.Errorf("invalid accepted %s", body) }
    }
    // The closed schema on this unrelated legacy component deliberately remains permissive.
    var legacy UnrelatedLegacyResult
    if err := json.Unmarshal([]byte(`{"id":"longer-than-one","future":true}`), &legacy); err != nil { t.Fatal(err) }
}
GO
(cd "$work_dir" && gofmt -w openapi && go test ./... && go vet ./...)
