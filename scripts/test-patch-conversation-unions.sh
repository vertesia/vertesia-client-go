#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT
mkdir -p "$work_dir/openapi" "$work_dir/spec"

printf '%s\n' '{"components":{"schemas":{"RunConversationResponse":{"discriminator":{"propertyName":"status","mapping":{"available":"#/components/schemas/AvailableRunConversation","unavailable":"#/components/schemas/UnavailableRunConversation"}}},"ExperimentalCanonicalInteractionInitialState":{"discriminator":{"propertyName":"type","mapping":{"new":"#/components/schemas/ExperimentalCanonicalInteractionNewState","document":"#/components/schemas/ExperimentalCanonicalInteractionDocumentState","reference":"#/components/schemas/ExperimentalCanonicalInteractionReferenceState"}}}}}}' > "$work_dir/spec/vertesia-openapi.json"
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
	"gopkg.in/validator.v2"
)
type ExperimentalCanonicalInteractionNewState struct { Type string `json:"type"` }
type ExperimentalCanonicalInteractionDocumentState struct {
	Type string `json:"type"`
	Document map[string]interface{} `json:"document"`
}
func (state *ExperimentalCanonicalInteractionDocumentState) UnmarshalJSON(data []byte) error {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &fields); err != nil { return err }
	if _, ok := fields["document"]; !ok { return fmt.Errorf("missing document") }
	type alias ExperimentalCanonicalInteractionDocumentState
	return json.Unmarshal(data, (*alias)(state))
}
type ExperimentalCanonicalInteractionReferenceState struct {
	Type string `json:"type"`
	Reference map[string]interface{} `json:"reference"`
	OperationID string `json:"operation_id"`
}
func (state *ExperimentalCanonicalInteractionReferenceState) UnmarshalJSON(data []byte) error {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &fields); err != nil { return err }
	for _, required := range []string{"reference", "operation_id"} {
		if _, ok := fields[required]; !ok { return fmt.Errorf("missing %s", required) }
	}
	type alias ExperimentalCanonicalInteractionReferenceState
	return json.Unmarshal(data, (*alias)(state))
}
type ExperimentalCanonicalInteractionInitialState struct {
	ExperimentalCanonicalInteractionNewState *ExperimentalCanonicalInteractionNewState
	ExperimentalCanonicalInteractionDocumentState *ExperimentalCanonicalInteractionDocumentState
	ExperimentalCanonicalInteractionReferenceState *ExperimentalCanonicalInteractionReferenceState
}
func (dst *ExperimentalCanonicalInteractionInitialState) UnmarshalJSON(data []byte) error {
	return validator.Validate(dst)
}
var _ = json.Unmarshal
var _ = fmt.Errorf' > "$work_dir/openapi/model_experimental_canonical_interaction_initial_state.go"
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
grep -q '\*dst = ExperimentalCanonicalInteractionInitialState{}' \
    "$work_dir/openapi/model_experimental_canonical_interaction_initial_state.go"
grep -q 'case "reference"' "$work_dir/openapi/model_experimental_canonical_interaction_initial_state.go"
if grep -q 'validator.v2' "$work_dir/openapi/model_experimental_canonical_interaction_initial_state.go"; then
    echo 'unused experimental initial-state validator import was not removed' >&2
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
}

func TestCanonicalInitialStateDispatchValidationAndReset(t *testing.T) {
	var state ExperimentalCanonicalInteractionInitialState
	if err := json.Unmarshal([]byte(`{"type":"new"}`), &state); err != nil { t.Fatal(err) }
	if state.ExperimentalCanonicalInteractionNewState == nil { t.Fatal("new branch not selected") }
	if err := json.Unmarshal([]byte(`{"type":"document","document":{"id":"conversation-1"}}`), &state); err != nil { t.Fatal(err) }
	if state.ExperimentalCanonicalInteractionNewState != nil || state.ExperimentalCanonicalInteractionDocumentState == nil {
		t.Fatalf("stale branch after document reuse: %#v", state)
	}
	if err := json.Unmarshal([]byte(`{"type":"reference","reference":{"run_id":"run-1"},"operation_id":"op-1"}`), &state); err != nil { t.Fatal(err) }
	if state.ExperimentalCanonicalInteractionDocumentState != nil || state.ExperimentalCanonicalInteractionReferenceState == nil {
		t.Fatalf("stale branch after reference reuse: %#v", state)
	}
	// encoding/json rejects malformed syntax before calling UnmarshalJSON, so call the hook directly
	// to verify its own reset-before-decode guarantee.
	if err := state.UnmarshalJSON([]byte(`{`)); err == nil { t.Fatal("malformed state accepted") }
	if state.ExperimentalCanonicalInteractionNewState != nil ||
		state.ExperimentalCanonicalInteractionDocumentState != nil ||
		state.ExperimentalCanonicalInteractionReferenceState != nil {
		t.Fatalf("direct malformed decode retained a branch: %#v", state)
	}
	invalid := map[string]string{
		"missing type": `{}`,
		"non-string type": `{"type":7}`,
		"unknown type": `{"type":"future"}`,
		"document missing document": `{"type":"document"}`,
		"reference missing operation": `{"type":"reference","reference":{"run_id":"run-1"}}`,
	}
	for name, body := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(`{"type":"reference","reference":{"run_id":"run-1"},"operation_id":"op-1"}`), &state); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(body), &state); err == nil { t.Fatalf("invalid state accepted: %s", body) }
			if state.ExperimentalCanonicalInteractionNewState != nil ||
				state.ExperimentalCanonicalInteractionDocumentState != nil ||
				state.ExperimentalCanonicalInteractionReferenceState != nil {
				t.Fatalf("failed reuse retained a branch: %#v", state)
			}
		})
	}
}' > "$work_dir/openapi/patch_runtime_test.go"

(cd "$work_dir" && go test ./...)
