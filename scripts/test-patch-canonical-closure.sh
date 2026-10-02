#!/usr/bin/env bash
set -euo pipefail
repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT
mkdir -p "$work_dir/openapi" "$work_dir/spec"
printf 'module canonicalfixture\n\ngo 1.22\n' > "$work_dir/go.mod"
python3 - "$work_dir" <<'PY'
import json
from pathlib import Path
import sys
root = Path(sys.argv[1])
# Component V1 is not an inline ordinal. HTTP is an acronym, not four words.
schemas = {
    'ConversationDocument': {'properties': {'edit': {'$ref': '#/components/schemas/ConversationHTTPV1'},
                                           'empty': {'$ref': '#/components/schemas/ConversationEmpty'}}},
    'ConversationEmpty': {'type': 'object', 'properties': {}, 'additionalProperties': False},
    'ConversationHTTPV1': {'anyOf': [
        {'type': 'object', 'additionalProperties': False, 'properties': {
            'kind': {'const': 'first'}, 'version': {'const': 1}, 'enabled': {'const': False},
            'pointer': {'$ref': '#/components/schemas/Pointer'},
            'status': {'type': 'string', 'enum': ['known']},
            'empty': {'type': 'string', 'const': ''},
            'optional_pointer': {'$ref': '#/components/schemas/Pointer'},
            'nullable': {'anyOf': [{'type': 'string'}, {'type': 'null'}]},
        }, 'required': ['kind', 'version', 'enabled', 'pointer']},
        {'type': 'object', 'additionalProperties': False, 'properties': {
            'kind': {'const': 'second'}, 'version': {'const': 2},
        }, 'required': ['kind', 'version']},
    ]},
    'Pointer': {'type': 'string', 'pattern': '^(?:\\/(?:[^~]|~[01])*)?$'},
    'ConversationFile': {'type': 'object', 'additionalProperties': False,
                         'properties': {'id': {'type': 'string'}}},
}
(root / 'spec/vertesia-openapi.json').write_text(json.dumps({'components': {'schemas': schemas}}))
(root / 'openapi/model_conversation_httpv1.go').write_text('''package fixture
import ("encoding/json"; "fmt")
type ConversationHTTPV1 struct {
 ConversationHTTPV1AnyOf *ConversationHTTPV1AnyOf
 ConversationHTTPV1AnyOf1 *ConversationHTTPV1AnyOf1
}
func (dst *ConversationHTTPV1) UnmarshalJSON(data []byte) error { return fmt.Errorf("unpatched") }
var _ = json.Unmarshal
''')
for suffix, fields in [('', 'Kind string `json:"kind"`; Version float32 `json:"version"`; Enabled bool `json:"enabled"`; Pointer string `json:"pointer"`; Status string `json:"status"`; Empty *string `json:"empty"`; OptionalPointer *string `json:"optional_pointer"`; Nullable *string `json:"nullable"`'),
                       ('1', 'Kind string `json:"kind"`; Version float32 `json:"version"`')]:
    name = 'ConversationHTTPV1AnyOf' + suffix
    filename = 'model_conversation_httpv1_any_of' + ('_1' if suffix else '') + '.go'
    (root / 'openapi' / filename).write_text(f'''package fixture
import "encoding/json"
type {name} struct {{ {fields} }}
func (o *{name}) UnmarshalJSON(data []byte) (err error) {{
 type alias {name}
 return json.Unmarshal(data, (*alias)(o))
}}
''')
(root / 'openapi/model_conversation_empty.go').write_text('package fixture\ntype ConversationEmpty struct {}\n')
(root / 'openapi/model_conversation_file.go').write_text('''package fixture
import "encoding/json"
type ConversationFile struct { Id string `json:"id"` }
func (o *ConversationFile) UnmarshalJSON(data []byte) (err error) {
 type alias ConversationFile
 return json.Unmarshal(data, (*alias)(o))
}
''')
PY
bash "$repo_dir/scripts/patch-openapi-permissive-decode.sh" "$work_dir/openapi"
cp -R "$work_dir/openapi" "$work_dir/first"
bash "$repo_dir/scripts/patch-openapi-permissive-decode.sh" "$work_dir/openapi"
diff -r "$work_dir/first" "$work_dir/openapi"
cat > "$work_dir/openapi/closure_test.go" <<'GO'
package fixture
import ("encoding/json"; "testing")
func TestCanonicalNumericClosure(t *testing.T) {
 for _, value := range []string{
  `{"kind":"first","version":1,"enabled":false,"pointer":"/a~1b","status":"future"}`,
  `{"kind":"first","version":1.0,"enabled":false,"pointer":""}`,
  `{"kind":"first","version":1e0,"enabled":false,"pointer":"/a"}`,
  `{"kind":"second","version":2}`,
  `{"kind":"first","version":1,"enabled":false,"pointer":"","empty":"","optional_pointer":"/a","nullable":null}`,
 } {
  var decoded ConversationHTTPV1
  if err := json.Unmarshal([]byte(value), &decoded); err != nil { t.Fatalf("valid %s: %v", value, err) }
 }
 for _, value := range []string{
  `{"kind":"first","version":2,"enabled":false,"pointer":""}`,
  `{"kind":"first","version":"1","enabled":false,"pointer":""}`,
  `{"kind":"first","version":null,"enabled":false,"pointer":""}`,
  `{"kind":"first","version":1,"enabled":null,"pointer":""}`,
  `{"kind":"first","version":1,"enabled":false,"pointer":"/~2"}`,
  `{"kind":"first","version":1,"enabled":false,"pointer":"","future":1}`,
  `{"kind":"second","version":3}`,
  `{"kind":"first","version":1,"enabled":false,"pointer":"","empty":null}`,
  `{"kind":"first","version":1,"enabled":false,"pointer":"","optional_pointer":null}`,
  `{"kind":"first","version":1,"enabled":false,"pointer":"","optional_pointer":"/~2"}`,
  `{"kind":"second","version":2,"pointer":""}`,
  `{"kind":"future","version":2}`,
  `{"kind":"second"}`,
 } {
  var decoded ConversationHTTPV1
  if err := json.Unmarshal([]byte(value), &decoded); err == nil { t.Errorf("accepted invalid %s", value) }
 }
 var empty ConversationEmpty
 if err := json.Unmarshal([]byte(`{}`), &empty); err != nil { t.Fatal(err) }
 for _, value := range []string{`{"extra":1}`, `null`} {
  if err := json.Unmarshal([]byte(value), &empty); err == nil { t.Fatalf("accepted closed empty object %s", value) }
 }
 var legacy ConversationFile
 if err := json.Unmarshal([]byte(`{"id":"old","future":1}`), &legacy); err != nil { t.Fatal(err) }
 if legacy.Id != "old" { t.Fatal("lost legacy field") }
}
GO
cd "$work_dir"
gofmt -w openapi
go test ./...
go vet ./...

# Keep extraction endpoint roots covered by the same canonical recipe CI gate.
bash "$repo_dir/scripts/test-patch-asset-extraction.sh"
