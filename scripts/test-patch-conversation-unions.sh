#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT
mkdir -p "$work_dir/openapi" "$work_dir/spec"

printf '%s\n' '{"components":{"schemas":{"RunConversationResponse":{"discriminator":{"propertyName":"status","mapping":{"available":"#/components/schemas/AvailableRunConversation","unavailable":"#/components/schemas/UnavailableRunConversation"}}}}}}' > "$work_dir/spec/vertesia-openapi.json"
printf '%s\n' 'package fixture
import (
	"encoding/json"
	"fmt"
	"gopkg.in/validator.v2"
)
type AvailableRunConversation struct{}
type UnavailableRunConversation struct{}
type RunConversationResponse struct {
    AvailableRunConversation *AvailableRunConversation
    UnavailableRunConversation *UnavailableRunConversation
}
func (dst *RunConversationResponse) UnmarshalJSON(data []byte) error {
    return validator.Validate(dst)
}
var _ = json.Unmarshal
var _ = fmt.Errorf' > "$work_dir/openapi/model_run_conversation_response.go"
printf '%s\n' 'package fixture
import (
	"encoding/json"
	"fmt"
)
type ConversationGeneratedAgentTurn struct{}
type ConversationImportedAgentTurn struct{}
type ConversationDerivedAgentTurn struct{}
type ConversationNongeneratedAgentTurn struct{}
type ConversationAgentTurn struct {
	ConversationGeneratedAgentTurn *ConversationGeneratedAgentTurn
	ConversationImportedAgentTurn *ConversationImportedAgentTurn
	ConversationDerivedAgentTurn *ConversationDerivedAgentTurn
	ConversationNongeneratedAgentTurn *ConversationNongeneratedAgentTurn
}
func (dst *ConversationAgentTurn) UnmarshalJSON(data []byte) error {
	return fmt.Errorf("unpatched")
}
func (o ConversationAgentTurn) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct{}{})
}' > "$work_dir/openapi/model_conversation_agent_turn.go"

bash "$repo_dir/scripts/patch-openapi-permissive-decode.sh" "$work_dir/openapi"
grep -q '\*dst = RunConversationResponse{}' "$work_dir/openapi/model_run_conversation_response.go"
grep -q 'case "available"' "$work_dir/openapi/model_run_conversation_response.go"
if grep -q 'validator.v2' "$work_dir/openapi/model_run_conversation_response.go"; then
    echo 'unused validator import was not removed' >&2
    exit 1
fi

printf '%s\n' 'module fixture

go 1.24' > "$work_dir/go.mod"
printf '%s\n' 'package fixture

import (
	"encoding/json"
	"testing"
)

func TestRunConversationDispatchAndReset(t *testing.T) {
	var response RunConversationResponse
	if err := json.Unmarshal([]byte(`{"status":"available"}`), &response); err != nil { t.Fatal(err) }
	if response.AvailableRunConversation == nil { t.Fatal("available branch not selected") }
	if err := json.Unmarshal([]byte(`{"status":"unavailable"}`), &response); err != nil { t.Fatal(err) }
	if response.AvailableRunConversation != nil || response.UnavailableRunConversation == nil {
		t.Fatalf("stale branch after reuse: %#v", response)
	}
	if err := json.Unmarshal([]byte(`{"status":"future"}`), &response); err == nil {
		t.Fatal("unknown discriminator accepted")
	}
}

func TestAgentProvenanceWinsOverGenerationIDAndResets(t *testing.T) {
	var turn ConversationAgentTurn
	if err := json.Unmarshal([]byte(`{"generation_id":"g1","provenance":{"type":"imported"}}`), &turn); err != nil { t.Fatal(err) }
	if turn.ConversationImportedAgentTurn == nil || turn.ConversationGeneratedAgentTurn != nil { t.Fatalf("wrong imported branch: %#v", turn) }
	if err := json.Unmarshal([]byte(`{"generation_id":"g2","provenance":{"type":"derived"}}`), &turn); err != nil { t.Fatal(err) }
	if turn.ConversationImportedAgentTurn != nil || turn.ConversationDerivedAgentTurn == nil { t.Fatalf("stale derived branch: %#v", turn) }
	if err := json.Unmarshal([]byte(`{"provenance":{"type":"future"}}`), &turn); err == nil { t.Fatal("unknown provenance accepted") }
}' > "$work_dir/openapi/patch_runtime_test.go"

(cd "$work_dir" && go test ./...)
