#!/usr/bin/env bash
# Generated-shape regression for the extraction roots' transitive canonical closure.
set -euo pipefail
repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT
mkdir -p "$work_dir/openapi" "$work_dir/spec"
printf 'module extractionfixture\n\ngo 1.22\n' > "$work_dir/go.mod"
python3 - "$work_dir" <<'PY'
import json
from pathlib import Path
import sys
root = Path(sys.argv[1])
def ref(name):
    return {'$ref': '#/components/schemas/' + name}
def obj(properties, required):
    return {'type': 'object', 'properties': properties, 'required': required, 'additionalProperties': False}
identity = {'operation_id': {'type': 'string'}}
schemas = {
    'ExperimentalExtractAgentAssetPayload': obj({'operation_id': {'type': 'string'},
        'transform': {'const': 'document_text/v1'}}, ['operation_id', 'transform']),
    'ExperimentalAgentAssetExtraction': {'oneOf': [ref('ExperimentalAgentAssetExtraction' + branch)
        for branch in ['Pending', 'Available', 'Failed']], 'discriminator': {
            'propertyName': 'status', 'mapping': {status: '#/components/schemas/ExperimentalAgentAssetExtraction' + branch
                for status, branch in [('pending', 'Pending'), ('available', 'Available'), ('failed', 'Failed')]}}},
    'ExperimentalAgentAssetExtractionPending': obj({**identity, 'status': {'const': 'pending'}}, ['operation_id', 'status']),
    'ExperimentalAgentAssetExtractionAvailable': obj({**identity, 'status': {'const': 'available'},
        'derivation': ref('ExperimentalAgentAssetDerivation')}, ['operation_id', 'status', 'derivation']),
    'ExperimentalAgentAssetExtractionFailed': obj({**identity, 'status': {'const': 'failed'},
        'reason': {'type': 'string', 'enum': ['source_unavailable']}, 'message': {'type': 'string'}},
        ['operation_id', 'status', 'reason', 'message']),
    'ExperimentalAgentAssetDerivation': obj({'version': {'const': 1},
        'source': ref('ExperimentalAgentAssetDerivationSource'),
        'transform': ref('ExperimentalAgentAssetDerivationTransform'),
        'output': ref('ExperimentalPublishedAgentAsset')}, ['version', 'source', 'transform', 'output']),
    'ExperimentalAgentAssetDerivationSource': obj({'content_hash': {'type': 'string'}}, ['content_hash']),
    'ExperimentalAgentAssetDerivationTransform': obj({'id': {'const': 'vertesia.document_text'},
        'version': {'const': '1'}}, ['id', 'version']),
    'ExperimentalPublishedAgentAsset': obj({'byte_length': {'type': 'integer'},
        'content_hash': {'type': 'string'}, 'metadata': {'type': 'object', 'additionalProperties': True}},
        ['byte_length', 'content_hash']),
    'UnrelatedLegacyAsset': obj({'id': {'type': 'string'}}, ['id']),
}
(root / 'spec/vertesia-openapi.json').write_text(json.dumps({'components': {'schemas': schemas}}))
fields = {
    'ExperimentalExtractAgentAssetPayload': 'OperationId string `json:"operation_id"`; Transform string `json:"transform"`',
    'ExperimentalAgentAssetExtractionPending': 'OperationId string `json:"operation_id"`; Status string `json:"status"`',
    'ExperimentalAgentAssetExtractionAvailable': 'OperationId string `json:"operation_id"`; Status string `json:"status"`; Derivation ExperimentalAgentAssetDerivation `json:"derivation"`',
    'ExperimentalAgentAssetExtractionFailed': 'OperationId string `json:"operation_id"`; Status string `json:"status"`; Reason string `json:"reason"`; Message string `json:"message"`',
    'ExperimentalAgentAssetDerivation': 'Version float32 `json:"version"`; Source ExperimentalAgentAssetDerivationSource `json:"source"`; Transform ExperimentalAgentAssetDerivationTransform `json:"transform"`; Output ExperimentalPublishedAgentAsset `json:"output"`',
    'ExperimentalAgentAssetDerivationSource': 'ContentHash string `json:"content_hash"`',
    'ExperimentalAgentAssetDerivationTransform': 'Id string `json:"id"`; Version string `json:"version"`',
    'ExperimentalPublishedAgentAsset': 'ByteLength int64 `json:"byte_length"`; ContentHash string `json:"content_hash"`; Metadata map[string]interface{} `json:"metadata,omitempty"`',
    'UnrelatedLegacyAsset': 'Id string `json:"id"`',
}
import re
def filename(name):
    return re.sub(r'([a-z0-9])([A-Z])', r'\1_\2', name).lower()
for name, fields in fields.items():
    (root / 'openapi' / ('model_' + filename(name) + '.go')).write_text(f'''package fixture
import "encoding/json"
type {name} struct {{ {fields} }}
func (o *{name}) UnmarshalJSON(data []byte) (err error) {{
 type alias {name}
 return json.Unmarshal(data, (*alias)(o))
}}
''')
(root / 'openapi/model_experimental_agent_asset_extraction.go').write_text('''package fixture
import ("encoding/json"; "fmt")
type ExperimentalAgentAssetExtraction struct {
 ExperimentalAgentAssetExtractionPending *ExperimentalAgentAssetExtractionPending
 ExperimentalAgentAssetExtractionAvailable *ExperimentalAgentAssetExtractionAvailable
 ExperimentalAgentAssetExtractionFailed *ExperimentalAgentAssetExtractionFailed
}
func (dst *ExperimentalAgentAssetExtraction) UnmarshalJSON(data []byte) error { return fmt.Errorf("unpatched") }
func (o ExperimentalAgentAssetExtraction) MarshalJSON() ([]byte, error) {
 if o.ExperimentalAgentAssetExtractionPending != nil { return json.Marshal(o.ExperimentalAgentAssetExtractionPending) }
 if o.ExperimentalAgentAssetExtractionAvailable != nil { return json.Marshal(o.ExperimentalAgentAssetExtractionAvailable) }
 if o.ExperimentalAgentAssetExtractionFailed != nil { return json.Marshal(o.ExperimentalAgentAssetExtractionFailed) }
 return nil, fmt.Errorf("missing branch")
}
''')
PY
bash "$repo_dir/scripts/patch-openapi-permissive-decode.sh" "$work_dir/openapi"
cp -R "$work_dir/openapi" "$work_dir/first"
bash "$repo_dir/scripts/patch-openapi-permissive-decode.sh" "$work_dir/openapi"
diff -r "$work_dir/first" "$work_dir/openapi"
cat > "$work_dir/openapi/extraction_test.go" <<'GO'
package fixture
import ("encoding/json"; "testing"; "reflect")
func TestExtractionClosure(t *testing.T) {
 valid := []string{
  `{"operation_id":"one","status":"pending"}`,
  `{"operation_id":"one","status":"available","derivation":{"version":1,"source":{"content_hash":"source"},"transform":{"id":"vertesia.document_text","version":"1"},"output":{"byte_length":3,"content_hash":"output","metadata":{"nullable":null,"nested":{"keep_null":null},"flag":false}}}}`,
  `{"operation_id":"one","status":"failed","reason":"source_unavailable","message":"unavailable"}`,
  `{"operation_id":"one","status":"failed","reason":"future_reason","message":"forward compatible"}`,
 }
 for _, body := range valid {
  var result ExperimentalAgentAssetExtraction
  if err := json.Unmarshal([]byte(body), &result); err != nil { t.Fatalf("valid %s: %v", body, err) }
  roundtrip, err := json.Marshal(result); if err != nil { t.Fatal(err) }
  var before, after any
  if err := json.Unmarshal([]byte(body), &before); err != nil { t.Fatal(err) }
  if err := json.Unmarshal(roundtrip, &after); err != nil { t.Fatal(err) }
  if !reflect.DeepEqual(before, after) { t.Fatalf("roundtrip lost JSON: %s", roundtrip) }
 }
 invalid := []string{
  `{"operation_id":"one","status":"available"}`,
  `{"operation_id":"one","status":"failed","message":"unavailable"}`,
  `{"operation_id":"one","status":"failed","reason":"source_unavailable"}`,
  `{"operation_id":"one","status":"unknown"}`,
  `{"operation_id":"one"}`,
  `{"operation_id":"one","status":"pending","extra":1}`,
  `{"operation_id":"one","status":"available","derivation":{"version":1,"source":{"content_hash":"source"},"transform":{"id":"wrong","version":"1"},"output":{"byte_length":3,"content_hash":"output"}}}`,
  `{"operation_id":"one","status":"available","derivation":{"version":1,"source":{"content_hash":"source"},"transform":{"id":"vertesia.document_text","version":"2"},"output":{"byte_length":3,"content_hash":"output"}}}`,
  `{"operation_id":"one","status":"available","derivation":{"version":1,"source":{},"transform":{"id":"vertesia.document_text","version":"1"},"output":{"byte_length":3,"content_hash":"output"}}}`,
  `{"operation_id":"one","status":"available","derivation":{"version":1,"source":{"content_hash":"source"},"transform":{"id":"vertesia.document_text","version":"1"},"output":{"byte_length":3}}}`,
 }
 for _, body := range invalid {
  var result ExperimentalAgentAssetExtraction
  if err := json.Unmarshal([]byte(body), &result); err == nil { t.Fatalf("accepted invalid %s", body) }
 }
 for _, body := range []string{`{"operation_id":"one","transform":"document_text/v2"}`, `{"operation_id":"one"}`} {
  var request ExperimentalExtractAgentAssetPayload
  if err := json.Unmarshal([]byte(body), &request); err == nil { t.Fatalf("accepted request %s", body) }
 }
 var request ExperimentalExtractAgentAssetPayload
 if err := json.Unmarshal([]byte(`{"operation_id":"one","transform":"document_text/v1"}`), &request); err != nil { t.Fatal(err) }
 var legacy UnrelatedLegacyAsset
 if err := json.Unmarshal([]byte(`{"id":"old","future":1}`), &legacy); err != nil { t.Fatal(err) }
 if legacy.Id != "old" { t.Fatal("lost legacy field") }
}
GO
cd "$work_dir"
gofmt -w openapi
go test ./...
go vet ./...
