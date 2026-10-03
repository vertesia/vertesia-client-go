#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT
mkdir -p "$work_dir/openapi" "$work_dir/spec"
printf 'module initialauthoringfixture\n\ngo 1.22\n' > "$work_dir/go.mod"
cat > "$work_dir/spec/vertesia-openapi.json" <<'JSON'
{"components":{"schemas":{
  "ExperimentalInitialAuthoringViewResponse":{"oneOf":[{"$ref":"#/components/schemas/AvailableInitialAuthoringView"},{"$ref":"#/components/schemas/UnavailableInitialAuthoringView"}],"discriminator":{"propertyName":"status","mapping":{"available":"#/components/schemas/AvailableInitialAuthoringView","unavailable":"#/components/schemas/UnavailableInitialAuthoringView"}}},
  "AvailableInitialAuthoringView":{"required":["status","input"]},
  "UnavailableInitialAuthoringView":{"required":["status","reason"]}
}}}
JSON
cat > "$work_dir/openapi/model_experimental_initial_authoring_view_response.go" <<'GO'
package fixture
import (
    "encoding/json"
    "fmt"
)
type AvailableInitialAuthoringView struct { Status string `json:"status"`; Input string `json:"input"` }
type UnavailableInitialAuthoringView struct { Status string `json:"status"`; Reason string `json:"reason"` }
type ExperimentalInitialAuthoringViewResponse struct {
    AvailableInitialAuthoringView *AvailableInitialAuthoringView
    UnavailableInitialAuthoringView *UnavailableInitialAuthoringView
}
func (dst *ExperimentalInitialAuthoringViewResponse) UnmarshalJSON(data []byte) error { return fmt.Errorf("unpatched initial authoring view") }
var _ = json.Unmarshal
GO
cat > "$work_dir/openapi/initial_authoring_view_test.go" <<'GO'
package fixture
import (
    "encoding/json"
    "testing"
)
func TestInitialAuthoringViewDispatch(t *testing.T) {
    var view ExperimentalInitialAuthoringViewResponse
    if err := json.Unmarshal([]byte(`{"status":"available","input":"saved"}`), &view); err != nil { t.Fatal(err) }
    if view.AvailableInitialAuthoringView == nil || view.UnavailableInitialAuthoringView != nil { t.Fatal("available branch was not selected") }
    if err := json.Unmarshal([]byte(`{"status":"unavailable","reason":"retention"}`), &view); err != nil { t.Fatal(err) }
    if view.UnavailableInitialAuthoringView == nil || view.AvailableInitialAuthoringView != nil { t.Fatal("previous branch was not reset") }
    for _, body := range []string{`{}`, `{"status":"future","input":"saved"}`} {
        if err := json.Unmarshal([]byte(body), &view); err == nil { t.Fatalf("accepted %s", body) }
    }
}
GO
bash "$repo_dir/scripts/patch-openapi-permissive-decode.sh" "$work_dir/openapi"
before="$(sha256sum "$work_dir/openapi/model_experimental_initial_authoring_view_response.go")"
bash "$repo_dir/scripts/patch-openapi-permissive-decode.sh" "$work_dir/openapi"
after="$(sha256sum "$work_dir/openapi/model_experimental_initial_authoring_view_response.go")"
[[ "$before" == "$after" ]] || { echo 'initial authoring view patch changed on second pass' >&2; exit 1; }
(cd "$work_dir" && go test ./...)
