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

if [[ -f "$openapi_dir/../spec/vertesia-openapi.json" ]]; then
python3 - "$openapi_dir" <<'PY'
import json
from pathlib import Path
import re
import sys

root = Path(sys.argv[1])
spec = json.loads((root.parent / 'spec' / 'vertesia-openapi.json').read_text())
models = {}
for type_name, schema in spec['components']['schemas'].items():
    if not (
        type_name.startswith('Conversation')
        or type_name == 'RunConversationResponse'
        or type_name == 'ExperimentalCanonicalInteractionInitialState'
    ):
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
\t\tselected := &{branch}{{}}
\t\tif err := json.Unmarshal(data, selected); err != nil {{
\t\t\treturn fmt.Errorf("failed to unmarshal {type_name} as {branch}: %w", err)
\t\t}}
\t\tdst.{branch} = selected
\t\treturn nil''')
    replacement = f'''func (dst *{type_name}) UnmarshalJSON(data []byte) error {{
\t*dst = {type_name}{{}}
\tvar discriminator struct {{ Value string `json:"{wire_field}"` }}
\tif err := json.Unmarshal(data, &discriminator); err != nil {{ return err }}
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
fi

# Canonical conversation fingerprints bind the exact RFC 3339 string, including fractional
# second spelling. The generator's date-time type normalizes that spelling through time.Time.
# schemaMappings suppresses the generated date-time model; retain the public schema name as an
# exact string alias so only fields that reference ConversationTimestamp use lexical strings.
if python3 - "$openapi_dir/../spec/vertesia-openapi.json" <<'PY'
import json
from pathlib import Path
import sys

spec_path = Path(sys.argv[1])
if not spec_path.is_file():
    raise SystemExit(1)
spec = json.loads(spec_path.read_text())
raise SystemExit(0 if 'ConversationTimestamp' in spec.get('components', {}).get('schemas', {}) else 1)
PY
then
  package_name="$(python3 - "$openapi_dir" <<'PY'
from pathlib import Path
import re
import sys

root = Path(sys.argv[1])
for path in sorted(root.glob('*.go')):
    match = re.search(r'^package\s+([A-Za-z_][A-Za-z0-9_]*)$', path.read_text(), re.MULTILINE)
    if match:
        print(match.group(1))
        raise SystemExit(0)
raise SystemExit('generated Go package declaration not found')
PY
)"
  cat > "$openapi_dir/model_conversation_timestamp.go" <<EOF
package $package_name

// ConversationTimestamp preserves the exact RFC 3339 lexical value used by canonical fingerprints.
type ConversationTimestamp = string
EOF
fi

result_schema_file="$openapi_dir/model_experimental_canonical_interaction_result_schema_input.go"
if [[ -f "$result_schema_file" ]]; then
  python3 - "$result_schema_file" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
source = path.read_text()
signature = 'func (o *ExperimentalCanonicalInteractionResultSchemaInput) UnmarshalJSON(data []byte) (err error) {'
patched_signature = 'func (o *ExperimentalCanonicalInteractionResultSchemaInput) UnmarshalJSON(data []byte) error {'
if signature not in source:
    if patched_signature in source:
        raise SystemExit(0)
    raise ValueError('generated result-schema UnmarshalJSON signature not found')
start = source.index(signature)
brace = source.index('{', start)
depth = 0
for end in range(brace, len(source)):
    if source[end] == '{':
        depth += 1
    elif source[end] == '}':
        depth -= 1
        if depth == 0:
            break
else:
    raise ValueError(f'unclosed function {signature}')

# OpenAPI Generator first unmarshals this free-form object through the generated alias. Its
# exported AdditionalProperties field case-insensitively captures the legitimate JSON Schema
# keyword "additionalProperties" and rejects boolean values before the free-form map is decoded.
# Decode the complete JSON object directly into the map so every user key remains data.
replacement = '''func (o *ExperimentalCanonicalInteractionResultSchemaInput) UnmarshalJSON(data []byte) error {
\tadditionalProperties := make(map[string]interface{})
\tif err := json.Unmarshal(data, &additionalProperties); err != nil {
\t\treturn err
\t}
\to.AdditionalProperties = additionalProperties
\treturn nil
}'''
path.write_text(source[:start] + replacement + source[end + 1:])
PY
fi

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

# Dispatch ModelOptions by its published discriminator without weakening validation
# for unrelated oneOf models through the generator-wide lookup option.
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "$openapi_dir/model_model_options.go" ]]; then
  go run "$script_dir/patch-model-options/main.go" -model "$openapi_dir/model_model_options.go" -spec "$script_dir/../spec/vertesia-openapi.json"
fi

client_file="$openapi_dir/client.go"
if [[ -f "$client_file" ]]; then
  perl -0pi -e '
    s/headers\[h\] = \[\]string\{v\}/headers.Set(h, v)/g;
    s/for header, value := range c\.cfg\.DefaultHeader \{\n\t\tlocalVarRequest\.Header\.Add\(header, value\)\n\t\}/for header, value := range c.cfg.DefaultHeader {\n\t\tif localVarRequest.Header.Get(header) == "" {\n\t\t\tlocalVarRequest.Header.Add(header, value)\n\t\t}\n\t}/g;
    s/httputil\.DumpRequestOut\(request, true\)/dumpRedactedRequestOut(request)/g;
    s/httputil\.DumpResponse\(resp, true\)/dumpRedactedResponse(resp)/g;
  ' "$client_file"

  if ! grep -q "func dumpRedactedRequestOut" "$client_file"; then
    perl -0pi -e 's#\n// callAPI do the request\.#\nconst redactedDebugValue = "<redacted>"\n\nfunc dumpRedactedRequestOut(request *http.Request) ([]byte, error) {\n\tsanitized := &http.Request{\n\t\tMethod:     request.Method,\n\t\tURL:        redactedDebugURL(request.URL),\n\t\tProto:      request.Proto,\n\t\tProtoMajor: request.ProtoMajor,\n\t\tProtoMinor: request.ProtoMinor,\n\t\tHeader:     redactedDebugHeader(request.Header),\n\t\tHost:       request.Host,\n\t}\n\treturn httputil.DumpRequestOut(sanitized, false)\n}\n\nfunc dumpRedactedResponse(resp *http.Response) ([]byte, error) {\n\tsanitized := &http.Response{\n\t\tStatus:        resp.Status,\n\t\tStatusCode:    resp.StatusCode,\n\t\tProto:         resp.Proto,\n\t\tProtoMajor:    resp.ProtoMajor,\n\t\tProtoMinor:    resp.ProtoMinor,\n\t\tHeader:        redactedDebugHeader(resp.Header),\n\t\tContentLength: -1,\n\t}\n\treturn httputil.DumpResponse(sanitized, false)\n}\n\nfunc redactedDebugHeader(header http.Header) http.Header {\n\tif header == nil {\n\t\treturn nil\n\t}\n\tredacted := make(http.Header, len(header))\n\tfor name, values := range header {\n\t\tcopied := append([]string(nil), values...)\n\t\tif isSensitiveDebugName(name) {\n\t\t\tfor i := range copied {\n\t\t\t\tcopied[i] = redactedDebugValue\n\t\t\t}\n\t\t}\n\t\tredacted[name] = copied\n\t}\n\treturn redacted\n}\n\nfunc redactedDebugURL(original *url.URL) *url.URL {\n\tif original == nil {\n\t\treturn nil\n\t}\n\tredacted := *original\n\tif redacted.User != nil {\n\t\tusername := redacted.User.Username()\n\t\tif _, hasPassword := redacted.User.Password(); hasPassword {\n\t\t\tredacted.User = url.UserPassword(username, redactedDebugValue)\n\t\t}\n\t}\n\tquery := redacted.Query()\n\tchanged := false\n\tfor name, values := range query {\n\t\tif isSensitiveDebugName(name) {\n\t\t\tfor i := range values {\n\t\t\t\tvalues[i] = redactedDebugValue\n\t\t\t}\n\t\t\tquery[name] = values\n\t\t\tchanged = true\n\t\t}\n\t}\n\tif changed {\n\t\tredacted.RawQuery = query.Encode()\n\t}\n\treturn &redacted\n}\n\nfunc isSensitiveDebugName(name string) bool {\n\tnormalized := strings.ToLower(name)\n\tnormalized = strings.NewReplacer("-", "", "_", "").Replace(normalized)\n\tswitch normalized {\n\tcase "authorization", "proxyauthorization", "cookie", "setcookie", "xapikey", "xauthtoken":\n\t\treturn true\n\t}\n\treturn strings.Contains(normalized, "apikey") ||\n\t\tstrings.Contains(normalized, "token") ||\n\t\tstrings.Contains(normalized, "secret") ||\n\t\tstrings.Contains(normalized, "password")\n}\n\n// callAPI do the request.#' "$client_file"
  fi
fi

# Required version headers are supplied by facade configuration before generated validation.
# Explicit per-request versions still take precedence, and raw clients without either still fail.
find "$openapi_dir" -name 'api_*.go' -print0 | xargs -0 perl -0pi -e '
  s/\tif r\.xApiVersion == nil \{\n(\t\treturn [^\n]+\n)\t\}/\tif r.xApiVersion == nil {\n\t\tversion := a.client.cfg.DefaultHeader["x-api-version"]\n\t\tif version == "" {\n\t$1\t\t}\n\t\tr.xApiVersion = \&version\n\t}/g;
'

# Permissive object decoding must still honor the CompletionResult discriminator.
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "$openapi_dir/model_completion_result.go" ]]; then
  go run "$script_dir/patch-completion-result/main.go" -model "$openapi_dir/model_completion_result.go" -spec "$script_dir/../spec/vertesia-openapi.json"
fi
