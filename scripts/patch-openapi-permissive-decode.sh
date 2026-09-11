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

# OpenAPI Generator's Go oneOf decoder ignores discriminators and tries every branch. With
# permissive response decoding, structurally identical branches such as EndpointRef's
# {kind,id} variants all match. Dispatch this public union by its declared wire discriminator
# while retaining explicit required-field validation.
endpoint_ref_file="$openapi_dir/model_endpoint_ref.go"
if [[ -f "$endpoint_ref_file" ]]; then
  python3 - "$endpoint_ref_file" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
source = path.read_text()
start = source.index('func (dst *EndpointRef) UnmarshalJSON(data []byte) error {')
end = source.index('\n}\n\n// Marshal data from the first non-nil pointers', start) + 2
replacement = '''func (dst *EndpointRef) UnmarshalJSON(data []byte) error {
\tvar discriminator struct {
\t\tKind string `json:"kind"`
\t}
\tif err := json.Unmarshal(data, &discriminator); err != nil {
\t\treturn err
\t}
\tswitch discriminator.Kind {
\tcase "subject":
\t\tvar value SubjectEndpointRef
\t\tif err := json.Unmarshal(data, &value); err != nil { return err }
\t\tif value.Id == "" { return fmt.Errorf("subject endpoint requires id") }
\t\t*dst = EndpointRef{SubjectEndpointRef: &value}
\tcase "document":
\t\tvar value DocumentEndpointRef
\t\tif err := json.Unmarshal(data, &value); err != nil { return err }
\t\tif value.Id == "" { return fmt.Errorf("document endpoint requires id") }
\t\t*dst = EndpointRef{DocumentEndpointRef: &value}
\tcase "document_version":
\t\tvar value DocumentVersionEndpointRef
\t\tif err := json.Unmarshal(data, &value); err != nil { return err }
\t\tif value.Id == "" { return fmt.Errorf("document_version endpoint requires id") }
\t\t*dst = EndpointRef{DocumentVersionEndpointRef: &value}
\tcase "external":
\t\tvar value ExternalEndpointRef
\t\tif err := json.Unmarshal(data, &value); err != nil { return err }
\t\tif value.Namespace == "" || value.Value == "" { return fmt.Errorf("external endpoint requires namespace and value") }
\t\t*dst = EndpointRef{ExternalEndpointRef: &value}
\tdefault:
\t\treturn fmt.Errorf("unknown endpoint kind %q", discriminator.Kind)
\t}
\treturn nil
}'''
path.write_text(source[:start] + replacement + source[end:])
PY
  sed -i '/"gopkg.in\/validator.v2"/d' "$endpoint_ref_file"
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
