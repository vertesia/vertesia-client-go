#!/usr/bin/env bash
set -euo pipefail
repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT
mkdir -p "$work_dir/openapi" "$work_dir/spec"
printf 'module hostfixture\n\ngo 1.22\n' > "$work_dir/go.mod"
python3 - "$work_dir" <<'PY'
import json
from pathlib import Path
import re
import sys

root = Path(sys.argv[1])
schemas = {}
def filename(name):
    name = re.sub(r'([A-Z]+)([A-Z][a-z])', r'\1_\2', name)
    return 'model_' + re.sub(r'([a-z0-9])([A-Z])', r'\1_\2', name).lower() + '.go'

def obj(name, properties, required, fields):
    schemas[name] = {'type': 'object', 'additionalProperties': False,
                     'properties': properties, 'required': required}
    (root / 'openapi' / filename(name)).write_text(f'''package fixture
import "encoding/json"
type {name} struct {{ {fields} }}
func (o *{name}) UnmarshalJSON(data []byte) (err error) {{
 type alias {name}
 return json.Unmarshal(data, (*alias)(o))
}}
''')

def ref(name):
    return {'$ref': '#/components/schemas/' + name}

timestamp = {'type': 'integer', 'format': 'int64', 'minimum': 0, 'maximum': 9007199254740991}
obj('HostStatus', {'status': {'type': 'string', 'enum': ['running', 'completed']}},
    ['status'], 'Status string `json:"status"`')
obj('HostEditingControl', {'client_message_id': {'type': 'string'},
                          'editing_action': {'type': 'string'}},
    ['client_message_id', 'editing_action'],
    'ClientMessageId string `json:"client_message_id"`; EditingAction string `json:"editing_action"`')
obj('HostControl', {'timestamp': timestamp, 'editing': ref('HostEditingControl')},
    ['timestamp', 'editing'], 'Timestamp int64 `json:"timestamp"`; Editing HostEditingControl `json:"editing"`')
obj('HostContent', {'turn_id': {'type': 'string'}}, ['turn_id'], 'TurnId string `json:"turn_id"`')
branches = [('HostStatusEnvelope', 'status', 'status', 'HostStatus'),
            ('HostControlEnvelope', 'run_control', 'control', 'HostControl'),
            ('HostContentEnvelope', 'content', 'content', 'HostContent')]
for name, kind, payload, child in branches:
    obj(name, {'type': {'type': 'string', 'const': kind}, payload: ref(child)},
        ['type', payload], f'Type string `json:"type"`; {payload.capitalize()} {child} `json:"{payload}"`')
name = 'ExperimentalAgentRunStreamEnvelope'
schemas[name] = {'oneOf': [ref(branch[0]) for branch in branches],
                 'discriminator': {'propertyName': 'type', 'mapping': {
                     kind: '#/components/schemas/' + child for child, kind, _, _ in branches}}}
fields = '\n'.join(f' {child} *{child}' for child, _, _, _ in branches)
(root / 'openapi' / filename(name)).write_text(f'''package fixture
import ("encoding/json"; "fmt")
type {name} struct {{
{fields}
}}
func (dst *{name}) UnmarshalJSON(data []byte) error {{ return fmt.Errorf("unpatched host union") }}
var _ = json.Unmarshal
''')
obj('HostControlPage', {'after': timestamp, 'has_more': {'type': 'boolean'}},
    ['after', 'has_more'], 'After int64 `json:"after"`; HasMore bool `json:"has_more"`')
obj('ExperimentalAgentRunUpdatesResponse', {'controls': {'type': 'array', 'items': ref('HostControl')},
                                          'control_page': ref('HostControlPage')},
    ['controls', 'control_page'], 'Controls []HostControl `json:"controls"`; ControlPage HostControlPage `json:"control_page"`')
# Outside the finite roots, the ordinary decoder and enum remain forward compatible.
obj('OrdinaryResponse', {'status': {'type': 'string', 'enum': ['known']}},
    ['status'], 'Status string `json:"status"`')
(root / 'spec/vertesia-openapi.json').write_text(json.dumps({'components': {'schemas': schemas}}))
PY
bash "$repo_dir/scripts/patch-openapi-permissive-decode.sh" "$work_dir/openapi"
cp -R "$work_dir/openapi" "$work_dir/first"
bash "$repo_dir/scripts/patch-openapi-permissive-decode.sh" "$work_dir/openapi"
diff -r "$work_dir/first" "$work_dir/openapi"
cat > "$work_dir/openapi/host_test.go" <<'GO'
package fixture

import (
	"encoding/json"
	"testing"
)

func TestHostEnvelopeDispatchAndClosure(t *testing.T) {
	var response ExperimentalAgentRunStreamEnvelope
	for index, body := range []string{
		`{"type":"status","status":{"status":"future-status"}}`,
		`{"type":"run_control","control":{"timestamp":1790812800123,"editing":{"client_message_id":"edit-1","editing_action":"update"}}}`,
		`{"type":"content","content":{"turn_id":"turn-1"}}`,
	} {
		if err := json.Unmarshal([]byte(body), &response); err != nil {
			t.Fatalf("valid %s: %v", body, err)
		}
		selected := []bool{response.HostStatusEnvelope != nil, response.HostControlEnvelope != nil, response.HostContentEnvelope != nil}
		for branch, present := range selected {
			if present != (index == branch) {
				t.Fatalf("wrong branch/reset for %s", body)
			}
		}
		if index == 1 && response.HostControlEnvelope.Control.Timestamp != 1790812800123 {
			t.Fatal("timestamp truncated")
		}
	}
	for _, body := range []string{
		`{}`, `null`, `{"type":"future","status":{"status":"running"}}`,
		`{"type":"status","status":null}`, `{"type":"status","status":{"status":"running","extra":true}}`,
		`{"type":"run_control","control":{"timestamp":1790812800123,"editing":{"editing_action":"update"}}}`,
		`{"type":"run_control","control":{"timestamp":-1,"editing":{"client_message_id":"edit-1","editing_action":"update"}}}`,
		`{"type":"content","content":{}}`,
	} {
		if err := json.Unmarshal([]byte(body), &response); err == nil {
			t.Fatalf("accepted invalid %s", body)
		}
		if response.HostStatusEnvelope != nil || response.HostControlEnvelope != nil || response.HostContentEnvelope != nil {
			t.Fatal("failed decode retained branch")
		}
	}
	var named HostControlEnvelope
	if err := json.Unmarshal([]byte(`{"type":"status","control":{"timestamp":0,"editing":{"client_message_id":"e","editing_action":"update"}}}`), &named); err == nil {
		t.Fatal("named leaf accepted wrong literal")
	}
	var ordinary OrdinaryResponse
	if err := json.Unmarshal([]byte(`{"status":"future","extra":1}`), &ordinary); err != nil || ordinary.Status != "future" {
		t.Fatalf("ordinary response lost compatibility: %v", err)
	}
}

func TestHostPollRequiredZeroCursorAndInt64(t *testing.T) {
	for _, cursor := range []int64{0, 1790812800124} {
		body, err := json.Marshal(map[string]any{"controls": []any{}, "control_page": map[string]any{"after": cursor, "has_more": false}})
		if err != nil {
			t.Fatal(err)
		}
		var response ExperimentalAgentRunUpdatesResponse
		if err := json.Unmarshal(body, &response); err != nil {
			t.Fatal(err)
		}
		if response.ControlPage.After != cursor {
			t.Fatalf("cursor = %d, want %d", response.ControlPage.After, cursor)
		}
		encoded, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		var roundtrip ExperimentalAgentRunUpdatesResponse
		if err := json.Unmarshal(encoded, &roundtrip); err != nil || roundtrip.ControlPage.After != cursor {
			t.Fatalf("cursor roundtrip: %v", err)
		}
	}
	for _, body := range []string{
		`{"controls":[],"control_page":{"has_more":false}}`,
		`{"controls":[],"control_page":{"after":null,"has_more":false}}`,
		`{"controls":[],"control_page":{"after":0}}`,
		`{"controls":[],"control_page":{"after":0,"has_more":false,"extra":1}}`,
		`{"controls":[],"control_page":{"after":9007199254740992,"has_more":false}}`,
		`{"controls":[]}`, `{"control_page":{"after":0,"has_more":false}}`,
	} {
		var response ExperimentalAgentRunUpdatesResponse
		if err := json.Unmarshal([]byte(body), &response); err == nil {
			t.Fatalf("accepted invalid %s", body)
		}
	}
}
GO
(cd "$work_dir" && gofmt -w openapi && go test ./... && go vet ./...)
