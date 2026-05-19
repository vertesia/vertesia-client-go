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

if [[ -f "$openapi_dir/utils.go" ]]; then
  perl -0pi -e '
    s#// A wrapper for strict JSON decoding#// A wrapper for JSON decoding used by generated union models#;
    s/\n\tdec\.DisallowUnknownFields\(\)//g;
  ' "$openapi_dir/utils.go"
fi
