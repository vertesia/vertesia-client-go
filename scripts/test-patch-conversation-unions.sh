#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT
mkdir -p "$work_dir/openapi" "$work_dir/spec"

cat > "$work_dir/spec/vertesia-openapi.json" <<'JSON'
{
  "components": {
    "schemas": {
      "ConversationTimestamp": { "type": "string", "format": "date-time" },
      "RunConversationResponse": {
        "discriminator": {
          "propertyName": "status",
          "mapping": {
            "available": "#/components/schemas/AvailableRunConversation",
            "unavailable": "#/components/schemas/UnavailableRunConversation"
          }
        }
      },
      "ExperimentalCanonicalInteractionInitialState": {
        "discriminator": {
          "propertyName": "type",
          "mapping": {
            "new": "#/components/schemas/ExperimentalCanonicalInteractionNewState",
            "document": "#/components/schemas/ExperimentalCanonicalInteractionDocumentState",
            "reference": "#/components/schemas/ExperimentalCanonicalInteractionReferenceState"
          }
        }
      },
      "ConversationStreamDraftBlock": {
        "type": "object",
        "required": ["type"],
        "discriminator": { "propertyName": "type" },
        "oneOf": [
          {
            "type": "object",
            "properties": {
              "type": { "type": "string", "const": "text" },
              "text": { "type": "string" }
            },
            "required": ["type", "text"]
          },
          {
            "type": "object",
            "properties": {
              "type": { "type": "string", "enum": ["image", "audio", "video", "document"] },
              "mime_type": { "type": "string" }
            },
            "required": ["type", "mime_type"]
          }
        ]
      },
      "ConversationStreamEvent": {
        "type": "object",
        "required": ["type"],
        "discriminator": { "propertyName": "type" },
        "oneOf": [
          {
            "type": "object",
            "properties": {
              "type": { "type": "string", "const": "draft_started" },
              "event_id": { "type": "string" }
            },
            "required": ["type", "event_id"]
          },
          {
            "type": "object",
            "properties": {
              "type": { "type": "string", "const": "draft_text_delta" },
              "text": { "type": "string" }
            },
            "required": ["type", "text"]
          }
        ]
      }
    }
  }
}
JSON
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
	"gopkg.in/validator.v2"
)
type ConversationStreamDraftBlockOneOf struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
func (value *ConversationStreamDraftBlockOneOf) UnmarshalJSON(data []byte) error {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &fields); err != nil { return err }
	for _, required := range []string{"type", "text"} {
		if _, ok := fields[required]; !ok { return fmt.Errorf("missing %s", required) }
	}
	type alias ConversationStreamDraftBlockOneOf
	return json.Unmarshal(data, (*alias)(value))
}
type ConversationStreamDraftBlockOneOf1 struct {
	Type string `json:"type"`
	MimeType string `json:"mime_type"`
}
func (value *ConversationStreamDraftBlockOneOf1) UnmarshalJSON(data []byte) error {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &fields); err != nil { return err }
	for _, required := range []string{"type", "mime_type"} {
		if _, ok := fields[required]; !ok { return fmt.Errorf("missing %s", required) }
	}
	type alias ConversationStreamDraftBlockOneOf1
	return json.Unmarshal(data, (*alias)(value))
}
type ConversationStreamDraftBlock struct {
	ConversationStreamDraftBlockOneOf *ConversationStreamDraftBlockOneOf
	ConversationStreamDraftBlockOneOf1 *ConversationStreamDraftBlockOneOf1
}
func (dst *ConversationStreamDraftBlock) UnmarshalJSON(data []byte) error {
	return validator.Validate(dst)
}
var _ = fmt.Errorf' > "$work_dir/openapi/model_conversation_stream_draft_block.go"
printf '%s\n' 'package fixture
import (
	"encoding/json"
	"fmt"
	"gopkg.in/validator.v2"
)
type ConversationStreamEventOneOf struct {
	Type string `json:"type"`
	EventID string `json:"event_id"`
}
func (value *ConversationStreamEventOneOf) UnmarshalJSON(data []byte) error {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &fields); err != nil { return err }
	for _, required := range []string{"type", "event_id"} {
		if _, ok := fields[required]; !ok { return fmt.Errorf("missing %s", required) }
	}
	type alias ConversationStreamEventOneOf
	return json.Unmarshal(data, (*alias)(value))
}
type ConversationStreamEventOneOf1 struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
func (value *ConversationStreamEventOneOf1) UnmarshalJSON(data []byte) error {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &fields); err != nil { return err }
	for _, required := range []string{"type", "text"} {
		if _, ok := fields[required]; !ok { return fmt.Errorf("missing %s", required) }
	}
	type alias ConversationStreamEventOneOf1
	return json.Unmarshal(data, (*alias)(value))
}
type ConversationStreamEvent struct {
	ConversationStreamEventOneOf *ConversationStreamEventOneOf
	ConversationStreamEventOneOf1 *ConversationStreamEventOneOf1
}
func (dst *ConversationStreamEvent) UnmarshalJSON(data []byte) error {
	return validator.Validate(dst)
}
var _ = fmt.Errorf' > "$work_dir/openapi/model_conversation_stream_event.go"
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
printf '%s\n' 'package fixture
import "encoding/json"
type ExperimentalCanonicalInteractionResultSchemaInput struct {
	AdditionalProperties map[string]interface{}
}
type _ExperimentalCanonicalInteractionResultSchemaInput ExperimentalCanonicalInteractionResultSchemaInput
func (o ExperimentalCanonicalInteractionResultSchemaInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(o.AdditionalProperties)
}
func (o *ExperimentalCanonicalInteractionResultSchemaInput) UnmarshalJSON(data []byte) (err error) {
	decoded := _ExperimentalCanonicalInteractionResultSchemaInput{}
	if err = json.Unmarshal(data, &decoded); err != nil { return err }
	*o = ExperimentalCanonicalInteractionResultSchemaInput(decoded)
	additionalProperties := make(map[string]interface{})
	if err = json.Unmarshal(data, &additionalProperties); err == nil { o.AdditionalProperties = additionalProperties }
	return err
}
type NullableExperimentalCanonicalInteractionResultSchemaInput struct {
	value *ExperimentalCanonicalInteractionResultSchemaInput
	isSet bool
}
func (v NullableExperimentalCanonicalInteractionResultSchemaInput) MarshalJSON() ([]byte, error) { return json.Marshal(v.value) }
func (v *NullableExperimentalCanonicalInteractionResultSchemaInput) UnmarshalJSON(data []byte) error {
	v.isSet = true
	return json.Unmarshal(data, &v.value)
}
type ExperimentalCanonicalInteractionExecutionRequest struct {
	ResultSchema NullableExperimentalCanonicalInteractionResultSchemaInput `json:"result_schema,omitempty"`
}
func (o ExperimentalCanonicalInteractionExecutionRequest) MarshalJSON() ([]byte, error) {
	value := map[string]interface{}{}
	if o.ResultSchema.isSet { value["result_schema"] = o.ResultSchema.value }
	return json.Marshal(value)
}' > "$work_dir/openapi/model_experimental_canonical_interaction_result_schema_input.go"

bash "$repo_dir/scripts/patch-openapi-permissive-decode.sh" "$work_dir/openapi"
bash "$repo_dir/scripts/patch-openapi-permissive-decode.sh" "$work_dir/openapi"
grep -q '^type ConversationTimestamp = string$' "$work_dir/openapi/model_conversation_timestamp.go"
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
grep -q 'case "image", "audio", "video", "document"' \
    "$work_dir/openapi/model_conversation_stream_draft_block.go"
grep -q 'case "draft_text_delta"' "$work_dir/openapi/model_conversation_stream_event.go"
if grep -q 'validator.v2' "$work_dir/openapi/model_conversation_stream_draft_block.go" ||
    grep -q 'validator.v2' "$work_dir/openapi/model_conversation_stream_event.go"; then
    echo 'unused inline-discriminator validator import was not removed' >&2
    exit 1
fi

printf '%s\n' 'module fixture

go 1.24' > "$work_dir/go.mod"
printf '%s\n' 'package fixture

import (
	"encoding/json"
	"reflect"
	"testing"
)

type CanonicalTimestampFixture struct {
	CreatedAt ConversationTimestamp `json:"created_at"`
	Nested struct {
		RecordedAt ConversationTimestamp `json:"recorded_at"`
		CompletedAt ConversationTimestamp `json:"completed_at"`
	} `json:"nested"`
}

func TestCanonicalTimestampPreservesExactLexicalValue(t *testing.T) {
	body := []byte(`{"created_at":"2026-09-30T00:00:00.000Z","nested":{"recorded_at":"2026-09-30T00:00:00.120Z","completed_at":"2026-09-30T00:00:00.123456789123Z"}}`)
	var value CanonicalTimestampFixture
	if err := json.Unmarshal(body, &value); err != nil { t.Fatal(err) }
	serialized, err := json.Marshal(value)
	if err != nil { t.Fatal(err) }
	var before, after map[string]any
	if err := json.Unmarshal(body, &before); err != nil { t.Fatal(err) }
	if err := json.Unmarshal(serialized, &after); err != nil { t.Fatal(err) }
	if !reflect.DeepEqual(before, after) { t.Fatalf("timestamp spelling changed: %s", serialized) }
}

func TestCanonicalResultSchemaPreservesOmissionNullAndArbitraryProperties(t *testing.T) {
	bodies := []string{
		`{}`,
		`{"result_schema":null}`,
		`{"result_schema":{"type":"object","additionalProperties":false,"x-test":{"enabled":true,"nullable":null,"values":["text",false,{"nested":"value"}]}}}`,
	}
	for _, body := range bodies {
		var request ExperimentalCanonicalInteractionExecutionRequest
		if err := json.Unmarshal([]byte(body), &request); err != nil { t.Fatalf("decode %s: %v", body, err) }
		serialized, err := json.Marshal(request)
		if err != nil { t.Fatalf("encode %s: %v", body, err) }
		var before, after map[string]any
		if err := json.Unmarshal([]byte(body), &before); err != nil { t.Fatal(err) }
		if err := json.Unmarshal(serialized, &after); err != nil { t.Fatal(err) }
		if !reflect.DeepEqual(before, after) { t.Fatalf("result schema changed: before=%#v after=%#v", before, after) }
	}
}

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
}

func TestInlineDiscriminatorDispatchValidationAndReset(t *testing.T) {
	var block ConversationStreamDraftBlock
	if err := json.Unmarshal([]byte(`{"type":"text","text":"hello"}`), &block); err != nil { t.Fatal(err) }
	if block.ConversationStreamDraftBlockOneOf == nil || block.ConversationStreamDraftBlockOneOf1 != nil {
		t.Fatalf("text branch not selected: %#v", block)
	}
	for _, mediaType := range []string{"image", "audio", "video", "document"} {
		body := `{"type":"` + mediaType + `","mime_type":"application/octet-stream"}`
		if err := json.Unmarshal([]byte(body), &block); err != nil { t.Fatalf("decode %s: %v", mediaType, err) }
		if block.ConversationStreamDraftBlockOneOf != nil || block.ConversationStreamDraftBlockOneOf1 == nil {
			t.Fatalf("%s did not select shared media branch: %#v", mediaType, block)
		}
	}
	invalidBlocks := map[string]string{
		"missing type": `{"text":"hello"}`,
		"non-string type": `{"type":7,"text":"hello"}`,
		"unknown type": `{"type":"future","text":"hello"}`,
		"text missing text": `{"type":"text"}`,
		"media missing mime": `{"type":"image"}`,
	}
	for name, body := range invalidBlocks {
		t.Run("block "+name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(`{"type":"text","text":"seed"}`), &block); err != nil { t.Fatal(err) }
			if err := json.Unmarshal([]byte(body), &block); err == nil { t.Fatalf("invalid block accepted: %s", body) }
			if block.ConversationStreamDraftBlockOneOf != nil || block.ConversationStreamDraftBlockOneOf1 != nil {
				t.Fatalf("failed block reuse retained a branch: %#v", block)
			}
		})
	}

	var event ConversationStreamEvent
	if err := json.Unmarshal([]byte(`{"type":"draft_started","event_id":"event-1"}`), &event); err != nil { t.Fatal(err) }
	if event.ConversationStreamEventOneOf == nil || event.ConversationStreamEventOneOf1 != nil {
		t.Fatalf("started branch not selected: %#v", event)
	}
	if err := json.Unmarshal([]byte(`{"type":"draft_text_delta","text":"delta"}`), &event); err != nil { t.Fatal(err) }
	if event.ConversationStreamEventOneOf != nil || event.ConversationStreamEventOneOf1 == nil {
		t.Fatalf("delta branch did not reset started branch: %#v", event)
	}
	invalidEvents := map[string]string{
		"unknown type": `{"type":"future","text":"delta"}`,
		"started missing event": `{"type":"draft_started"}`,
		"delta missing text": `{"type":"draft_text_delta"}`,
	}
	for name, body := range invalidEvents {
		t.Run("event "+name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(`{"type":"draft_started","event_id":"seed"}`), &event); err != nil { t.Fatal(err) }
			if err := json.Unmarshal([]byte(body), &event); err == nil { t.Fatalf("invalid event accepted: %s", body) }
			if event.ConversationStreamEventOneOf != nil || event.ConversationStreamEventOneOf1 != nil {
				t.Fatalf("failed event reuse retained a branch: %#v", event)
			}
		})
	}
}' > "$work_dir/openapi/patch_runtime_test.go"

(cd "$work_dir" && go test ./...)
