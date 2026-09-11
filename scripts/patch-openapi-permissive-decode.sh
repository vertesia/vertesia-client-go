#!/usr/bin/env bash
set -euo pipefail

openapi_dir="${1:-openapi}"

if [[ ! -d "$openapi_dir" ]]; then
  echo "OpenAPI output directory not found: $openapi_dir" >&2
  exit 1
fi

find "$openapi_dir" -name 'model_*.go' -print0 | xargs -0 perl -0pi -e '
  s/\n\t"bytes"//g;
  s/decoder := json\.NewDecoder\(bytes\.NewReader\(data\)\)\n\tdecoder\.DisallowUnknownFields\(\)\n\terr = decoder\.Decode\(&([A-Za-z0-9_]+)\)/err = json.Unmarshal(data, &$1)/g;
'

# OpenAPI Generator emits discriminator dispatch for ConversationTurn, but one mapped branch is
# itself a union. Supply the serialization hook the generated parent expects and dispatch that
# nested union from the canonical provenance discriminator (and generation_id for generated turns).
agent_turn_file="$openapi_dir/model_conversation_agent_turn.go"
if [[ -f "$agent_turn_file" ]]; then
  python3 - "$agent_turn_file" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
source = path.read_text()
signature = 'func (dst *ConversationAgentTurn) UnmarshalJSON(data []byte) error {'
start = source.index(signature)
brace = source.index('{', start)
depth = 0
end = brace
for end in range(brace, len(source)):
    if source[end] == '{': depth += 1
    elif source[end] == '}':
        depth -= 1
        if depth == 0: break
replacement = '''func (dst *ConversationAgentTurn) UnmarshalJSON(data []byte) error {
\tvar envelope struct { Provenance struct { Type string `json:"type"` } `json:"provenance"` }
\tif err := json.Unmarshal(data, &envelope); err != nil { return err }
\t*dst = ConversationAgentTurn{}
\tswitch envelope.Provenance.Type {
\tcase "generated":
\t\tdst.ConversationGeneratedAgentTurn = &ConversationGeneratedAgentTurn{}
\t\treturn json.Unmarshal(data, dst.ConversationGeneratedAgentTurn)
\tcase "imported":
\t\tdst.ConversationImportedAgentTurn = &ConversationImportedAgentTurn{}
\t\treturn json.Unmarshal(data, dst.ConversationImportedAgentTurn)
\tcase "derived":
\t\tdst.ConversationDerivedAgentTurn = &ConversationDerivedAgentTurn{}
\t\treturn json.Unmarshal(data, dst.ConversationDerivedAgentTurn)
\tcase "received", "inserted":
\t\tdst.ConversationNongeneratedAgentTurn = &ConversationNongeneratedAgentTurn{}
\t\treturn json.Unmarshal(data, dst.ConversationNongeneratedAgentTurn)
\tdefault:
\t\treturn fmt.Errorf("unknown ConversationAgentTurn provenance %q", envelope.Provenance.Type)
\t}
}'''
source = source[:start] + replacement + source[end + 1:]
if 'func (o ConversationAgentTurn) ToMap()' not in source:
    source += '''

func (o ConversationAgentTurn) ToMap() (map[string]interface{}, error) {
\tdata, err := json.Marshal(o)
\tif err != nil { return nil, err }
\tresult := map[string]interface{}{}
\terr = json.Unmarshal(data, &result)
\treturn result, err
}
'''
path.write_text(source)
PY
fi

python3 - "$openapi_dir" <<'PY'
import json
from pathlib import Path
import re
import sys

root = Path(sys.argv[1])
spec = json.loads((root.parent / 'spec' / 'vertesia-openapi.json').read_text())
models = {}
for type_name, schema in spec['components']['schemas'].items():
    if not (type_name.startswith('Conversation') or type_name == 'RunConversationResponse'):
        continue
    discriminator = schema.get('discriminator')
    if not discriminator:
        continue
    filename = re.sub(r'(?<!^)(?=[A-Z])', '_', type_name).lower()
    branches = {value: ref.rsplit('/', 1)[-1] for value, ref in discriminator['mapping'].items()}
    models[filename] = (discriminator['propertyName'], branches)

def replace_function(source, signature, replacement):
    start = source.index(signature)
    brace = source.index('{', start)
    depth = 0
    for end in range(brace, len(source)):
        if source[end] == '{': depth += 1
        elif source[end] == '}':
            depth -= 1
            if depth == 0: return source[:start] + replacement + source[end + 1:]
    raise ValueError(f'unclosed function {signature}')

for filename, (wire_field, branches) in models.items():
    path = root / f'model_{filename}.go'
    if not path.exists(): continue
    type_name = ''.join(part.title() for part in filename.split('_'))
    cases = []
    for value, branch in branches.items():
        cases.append(f'''\tcase "{value}":
\t\tdst.{branch} = &{branch}{{}}
\t\treturn json.Unmarshal(data, dst.{branch})''')
    replacement = f'''func (dst *{type_name}) UnmarshalJSON(data []byte) error {{
\tvar discriminator struct {{ Value string `json:"{wire_field}"` }}
\tif err := json.Unmarshal(data, &discriminator); err != nil {{ return err }}
\t*dst = {type_name}{{}}
\tswitch discriminator.Value {{
{chr(10).join(cases)}
\tdefault:
\t\treturn fmt.Errorf("unknown {type_name} discriminator %q", discriminator.Value)
\t}}
}}'''
    source = replace_function(path.read_text(), f'func (dst *{type_name}) UnmarshalJSON(data []byte) error {{', replacement)
    source = source.replace('\n\tvalidator "gopkg.in/validator.v2"', '')
    source = source.replace('\n\t"gopkg.in/validator.v2"', '')
    path.write_text(source)
PY

tool_definition_file="$openapi_dir/model_conversation_tool_definition.go"
if [[ -f "$tool_definition_file" ]]; then
  python3 - "$tool_definition_file" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
lines = path.read_text().splitlines(keepends=True)
path.write_text(''.join(
    line.replace('map[string]interface{}', 'interface{}') if 'InputSchema' in line or 'inputSchema' in line else line
    for line in lines
))
PY
fi

if [[ -f "$openapi_dir/utils.go" ]]; then
  perl -0pi -e '
    s#// A wrapper for strict JSON decoding#// A wrapper for JSON decoding used by generated union models#;
    s/\n\tdec\.DisallowUnknownFields\(\)//g;
  ' "$openapi_dir/utils.go"
fi

client_file="$openapi_dir/client.go"
if [[ -f "$client_file" ]]; then
  perl -0pi -e '
    s/httputil\.DumpRequestOut\(request, true\)/dumpRedactedRequestOut(request)/g;
    s/httputil\.DumpResponse\(resp, true\)/dumpRedactedResponse(resp)/g;
  ' "$client_file"

  if ! grep -q "func dumpRedactedRequestOut" "$client_file"; then
    perl -0pi -e 's#\n// callAPI do the request\.#\nconst redactedDebugValue = "<redacted>"\n\nfunc dumpRedactedRequestOut(request *http.Request) ([]byte, error) {\n\tsanitized := &http.Request{\n\t\tMethod:     request.Method,\n\t\tURL:        redactedDebugURL(request.URL),\n\t\tProto:      request.Proto,\n\t\tProtoMajor: request.ProtoMajor,\n\t\tProtoMinor: request.ProtoMinor,\n\t\tHeader:     redactedDebugHeader(request.Header),\n\t\tHost:       request.Host,\n\t}\n\treturn httputil.DumpRequestOut(sanitized, false)\n}\n\nfunc dumpRedactedResponse(resp *http.Response) ([]byte, error) {\n\tsanitized := &http.Response{\n\t\tStatus:        resp.Status,\n\t\tStatusCode:    resp.StatusCode,\n\t\tProto:         resp.Proto,\n\t\tProtoMajor:    resp.ProtoMajor,\n\t\tProtoMinor:    resp.ProtoMinor,\n\t\tHeader:        redactedDebugHeader(resp.Header),\n\t\tContentLength: -1,\n\t}\n\treturn httputil.DumpResponse(sanitized, false)\n}\n\nfunc redactedDebugHeader(header http.Header) http.Header {\n\tif header == nil {\n\t\treturn nil\n\t}\n\tredacted := make(http.Header, len(header))\n\tfor name, values := range header {\n\t\tcopied := append([]string(nil), values...)\n\t\tif isSensitiveDebugName(name) {\n\t\t\tfor i := range copied {\n\t\t\t\tcopied[i] = redactedDebugValue\n\t\t\t}\n\t\t}\n\t\tredacted[name] = copied\n\t}\n\treturn redacted\n}\n\nfunc redactedDebugURL(original *url.URL) *url.URL {\n\tif original == nil {\n\t\treturn nil\n\t}\n\tredacted := *original\n\tif redacted.User != nil {\n\t\tusername := redacted.User.Username()\n\t\tif _, hasPassword := redacted.User.Password(); hasPassword {\n\t\t\tredacted.User = url.UserPassword(username, redactedDebugValue)\n\t\t}\n\t}\n\tquery := redacted.Query()\n\tchanged := false\n\tfor name, values := range query {\n\t\tif isSensitiveDebugName(name) {\n\t\t\tfor i := range values {\n\t\t\t\tvalues[i] = redactedDebugValue\n\t\t\t}\n\t\t\tquery[name] = values\n\t\t\tchanged = true\n\t\t}\n\t}\n\tif changed {\n\t\tredacted.RawQuery = query.Encode()\n\t}\n\treturn &redacted\n}\n\nfunc isSensitiveDebugName(name string) bool {\n\tnormalized := strings.ToLower(name)\n\tnormalized = strings.NewReplacer("-", "", "_", "").Replace(normalized)\n\tswitch normalized {\n\tcase "authorization", "proxyauthorization", "cookie", "setcookie", "xapikey", "xauthtoken":\n\t\treturn true\n\t}\n\treturn strings.Contains(normalized, "apikey") ||\n\t\tstrings.Contains(normalized, "token") ||\n\t\tstrings.Contains(normalized, "secret") ||\n\t\tstrings.Contains(normalized, "password")\n}\n\n// callAPI do the request.#' "$client_file"
  fi
fi
