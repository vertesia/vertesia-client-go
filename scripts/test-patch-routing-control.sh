#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT
mkdir -p "$work_dir/openapi" "$work_dir/spec"
printf 'module routingfixture\n\ngo 1.22\n' > "$work_dir/go.mod"
cat > "$work_dir/spec/vertesia-openapi.json" <<'JSON'
{"components":{"schemas":{
  "ExperimentalAgentRoutingControlReceipt":{"oneOf":[{"$ref":"#/components/schemas/ExperimentalAgentRoutingInitialReceipt"},{"$ref":"#/components/schemas/ExperimentalAgentRoutingChangeReceipt"}],"discriminator":{"propertyName":"kind","mapping":{"initial":"#/components/schemas/ExperimentalAgentRoutingInitialReceipt","change":"#/components/schemas/ExperimentalAgentRoutingChangeReceipt"}}},
  "ExperimentalAgentRoutingInitialReceipt":{"required":["kind","initial"]},
  "ExperimentalAgentRoutingChangeReceipt":{"required":["kind","change"]},
  "ExperimentalAgentRoutingControlChange":{"anyOf":[{}, {}, {}]},
  "ExperimentalAgentRoutingIntent":{"anyOf":[{}, {}]}
}}}
JSON
cat > "$work_dir/openapi/model_experimental_agent_routing_control_receipt.go" <<'GO'
package fixture
import (
    "encoding/json"
    "fmt"
)
type ExperimentalAgentRoutingInitialReceipt struct { Kind string `json:"kind"`; Initial string `json:"initial"` }
type ExperimentalAgentRoutingChangeReceipt struct { Kind string `json:"kind"`; Change string `json:"change"` }
type ExperimentalAgentRoutingControlReceipt struct {
    ExperimentalAgentRoutingInitialReceipt *ExperimentalAgentRoutingInitialReceipt
    ExperimentalAgentRoutingChangeReceipt *ExperimentalAgentRoutingChangeReceipt
}
func (dst *ExperimentalAgentRoutingControlReceipt) UnmarshalJSON(data []byte) error { return fmt.Errorf("unpatched receipt") }
var _ = json.Unmarshal
GO
cat > "$work_dir/openapi/model_experimental_agent_routing_control_change.go" <<'GO'
package fixture
import (
    "encoding/json"
    "fmt"
)
type ExperimentalAgentRoutingControlChangeAnyOf struct { Model string `json:"model"`; Effort *string `json:"effort"` }
type ExperimentalAgentRoutingControlChangeAnyOf1 struct { InferenceProfile string `json:"inference_profile"`; Effort *string `json:"effort"` }
type ExperimentalAgentRoutingControlChangeAnyOf2 struct { Effort *string `json:"effort"` }
type ExperimentalAgentRoutingControlChange struct {
    ExperimentalAgentRoutingControlChangeAnyOf *ExperimentalAgentRoutingControlChangeAnyOf
    ExperimentalAgentRoutingControlChangeAnyOf1 *ExperimentalAgentRoutingControlChangeAnyOf1
    ExperimentalAgentRoutingControlChangeAnyOf2 *ExperimentalAgentRoutingControlChangeAnyOf2
}
func (dst *ExperimentalAgentRoutingControlChange) UnmarshalJSON(data []byte) error { return fmt.Errorf("unpatched change") }
var _ = json.Unmarshal
GO
cat > "$work_dir/openapi/model_experimental_agent_routing_intent.go" <<'GO'
package fixture
import (
    "encoding/json"
    "fmt"
)
type ExperimentalAgentRoutingIntentAnyOf struct { Model string `json:"model"`; Effort *string `json:"effort"` }
type ExperimentalAgentRoutingIntentAnyOf1 struct { InferenceProfile string `json:"inference_profile"`; Effort *string `json:"effort"` }
type ExperimentalAgentRoutingIntent struct {
    ExperimentalAgentRoutingIntentAnyOf *ExperimentalAgentRoutingIntentAnyOf
    ExperimentalAgentRoutingIntentAnyOf1 *ExperimentalAgentRoutingIntentAnyOf1
}
func (dst *ExperimentalAgentRoutingIntent) UnmarshalJSON(data []byte) error { return fmt.Errorf("unpatched intent") }
var _ = json.Unmarshal
GO
for model in experimental_agent_routing_control_change_any_of experimental_agent_routing_control_change_any_of_1 experimental_agent_routing_control_change_any_of_2 experimental_agent_routing_intent_any_of experimental_agent_routing_intent_any_of_1; do
  # The patch inspects the generated branch model source, while the fixture keeps
  # branch declarations together above to make its runtime test compact.
  case "$model" in
    *_change_any_of|*_intent_any_of) field='model' ;;
    *_change_any_of_1|*_intent_any_of_1) field='inference_profile' ;;
    *) field='effort' ;;
  esac
  printf 'package fixture\n// json:"%s"\n' "$field" > "$work_dir/openapi/model_${model}.go"
done
cat > "$work_dir/openapi/routing_test.go" <<'GO'
package fixture
import (
    "encoding/json"
    "testing"
)
func TestReceiptDispatchAndRequiredFields(t *testing.T) {
    var receipt ExperimentalAgentRoutingControlReceipt
    for _, body := range []string{`{"kind":"initial","initial":"one"}`, `{"kind":"change","change":"two"}`} {
        if err := json.Unmarshal([]byte(body), &receipt); err != nil { t.Fatal(err) }
    }
    if receipt.ExperimentalAgentRoutingChangeReceipt == nil || receipt.ExperimentalAgentRoutingInitialReceipt != nil { t.Fatal("branch was not reset") }
    for _, body := range []string{`{}`, `{"kind":"future"}`, `{"kind":"change"}`, `{"kind":"change","change":null}`} {
        if err := json.Unmarshal([]byte(body), &receipt); err == nil { t.Fatalf("accepted %s", body) }
    }
}
func TestRouteKeysSelectExactBranch(t *testing.T) {
    var change ExperimentalAgentRoutingControlChange
    if err := json.Unmarshal([]byte(`{"inference_profile":"profile"}`), &change); err != nil { t.Fatal(err) }
    if change.ExperimentalAgentRoutingControlChangeAnyOf1 == nil { t.Fatal("profile lost") }
    if err := json.Unmarshal([]byte(`{"effort":null}`), &change); err != nil { t.Fatal(err) }
    if change.ExperimentalAgentRoutingControlChangeAnyOf2 == nil || change.ExperimentalAgentRoutingControlChangeAnyOf1 != nil { t.Fatal("change branch was not reset") }
    var intent ExperimentalAgentRoutingIntent
    if err := json.Unmarshal([]byte(`{"inference_profile":"profile","effort":null}`), &intent); err != nil { t.Fatal(err) }
    if intent.ExperimentalAgentRoutingIntentAnyOf1 == nil { t.Fatal("intent profile lost") }
    for _, body := range []string{`{"model":null}`, `{"inference_profile":null}`, `{"model":"m","inference_profile":"p"}`} {
        if err := json.Unmarshal([]byte(body), &change); err == nil { t.Fatalf("change accepted %s", body) }
        if err := json.Unmarshal([]byte(body), &intent); err == nil { t.Fatalf("intent accepted %s", body) }
    }
}
GO
bash "$repo_dir/scripts/patch-openapi-permissive-decode.sh" "$work_dir/openapi"
before="$(sha256sum "$work_dir"/openapi/model_experimental_agent_routing_*.go)"
bash "$repo_dir/scripts/patch-openapi-permissive-decode.sh" "$work_dir/openapi"
after="$(sha256sum "$work_dir"/openapi/model_experimental_agent_routing_*.go)"
[[ "$before" == "$after" ]] || { echo 'routing patch changed on second pass' >&2; exit 1; }
(cd "$work_dir" && go test ./...)
