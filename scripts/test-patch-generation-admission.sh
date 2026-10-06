#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT
mkdir -p "$work_dir/openapi" "$work_dir/spec"
printf 'module generationfixture\n\ngo 1.22\n' > "$work_dir/go.mod"
cat > "$work_dir/spec/vertesia-openapi.json" <<'JSON'
{"components":{"schemas":{
  "ExperimentalAdmitAgentGenerationPayload":{"oneOf":[{"$ref":"#/components/schemas/ExperimentalAdmitAgentGenerationUserPayload"},{"$ref":"#/components/schemas/ExperimentalAdmitAgentGenerationToolsPayload"},{"$ref":"#/components/schemas/ExperimentalAdmitAgentGenerationCheckpointPayload"}],"discriminator":{"propertyName":"operation","mapping":{"user":"#/components/schemas/ExperimentalAdmitAgentGenerationUserPayload","tools":"#/components/schemas/ExperimentalAdmitAgentGenerationToolsPayload","checkpoint_summary":"#/components/schemas/ExperimentalAdmitAgentGenerationCheckpointPayload"}}},
  "ExperimentalAdmitAgentGenerationUserPayload":{"required":["operation","request"],"properties":{"request":{"$ref":"#/components/schemas/ExperimentalCanonicalUserMessagePayload"}}},
  "ExperimentalAdmitAgentGenerationToolsPayload":{"required":["operation","request"],"properties":{"request":{"$ref":"#/components/schemas/ExperimentalCanonicalToolResultsPayload"}}},
  "ExperimentalAdmitAgentGenerationCheckpointPayload":{"required":["operation","request"],"properties":{"request":{"$ref":"#/components/schemas/ExperimentalCanonicalCheckpointSummaryPayload"}}},
  "ExperimentalCanonicalUserMessagePayload":{"required":["run","input_append"]},
  "ExperimentalCanonicalToolResultsPayload":{"required":["run","continuation_anchor"]},
  "ExperimentalCanonicalCheckpointSummaryPayload":{"required":["run","source"]}
}}}
JSON
cat > "$work_dir/openapi/model_experimental_admit_agent_generation_payload.go" <<'GO'
package fixture
import (
    "encoding/json"
    "fmt"
)
type ExperimentalAdmitAgentGenerationUserPayload struct { Operation string `json:"operation"`; Request map[string]any `json:"request"` }
type ExperimentalAdmitAgentGenerationToolsPayload struct { Operation string `json:"operation"`; Request map[string]any `json:"request"` }
type ExperimentalAdmitAgentGenerationCheckpointPayload struct { Operation string `json:"operation"`; Request map[string]any `json:"request"` }
type ExperimentalAdmitAgentGenerationPayload struct {
    ExperimentalAdmitAgentGenerationUserPayload *ExperimentalAdmitAgentGenerationUserPayload
    ExperimentalAdmitAgentGenerationToolsPayload *ExperimentalAdmitAgentGenerationToolsPayload
    ExperimentalAdmitAgentGenerationCheckpointPayload *ExperimentalAdmitAgentGenerationCheckpointPayload
}
func (dst *ExperimentalAdmitAgentGenerationPayload) UnmarshalJSON(data []byte) error { return fmt.Errorf("unpatched admission") }
var _ = json.Unmarshal
GO
cat > "$work_dir/openapi/generation_test.go" <<'GO'
package fixture
import (
    "encoding/json"
    "testing"
)
func TestGenerationDispatchAndNestedRequiredFields(t *testing.T) {
    var admission ExperimentalAdmitAgentGenerationPayload
    for _, body := range []string{
        `{"operation":"user","request":{"run":{},"input_append":{}}}`,
        `{"operation":"tools","request":{"run":{},"continuation_anchor":{}}}`,
        `{"operation":"checkpoint_summary","request":{"run":{},"source":{}}}`,
    } {
        if err := json.Unmarshal([]byte(body), &admission); err != nil { t.Fatal(err) }
    }
    if admission.ExperimentalAdmitAgentGenerationCheckpointPayload == nil || admission.ExperimentalAdmitAgentGenerationUserPayload != nil { t.Fatal("branch was not reset") }
    for _, body := range []string{
        `{}`, `{"operation":"future","request":{"run":{}}}`,
        `{"operation":"tools","request":{"run":{},"input_append":{}}}`,
        `{"operation":"tools","request":{"run":{},"continuation_anchor":null}}`,
        `{"operation":"user"}`, `{"operation":"checkpoint_summary","request":null}`,
    } {
        if err := json.Unmarshal([]byte(body), &admission); err == nil { t.Fatalf("accepted %s", body) }
    }
}
GO
bash "$repo_dir/scripts/patch-openapi-permissive-decode.sh" "$work_dir/openapi"
before="$(sha256sum "$work_dir/openapi/model_experimental_admit_agent_generation_payload.go")"
bash "$repo_dir/scripts/patch-openapi-permissive-decode.sh" "$work_dir/openapi"
after="$(sha256sum "$work_dir/openapi/model_experimental_admit_agent_generation_payload.go")"
[[ "$before" == "$after" ]] || { echo 'admission patch changed on second pass' >&2; exit 1; }
(cd "$work_dir" && go test ./...)
