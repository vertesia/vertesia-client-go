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
      "ExperimentalAgentConversationStreamEnvelope": {
        "discriminator": {
          "propertyName": "type",
          "mapping": {
            "conversation_event": "#/components/schemas/ExperimentalAgentConversationEvent",
            "preview_unavailable": "#/components/schemas/ExperimentalAgentConversationPreviewUnavailable",
            "accepted_output": "#/components/schemas/ExperimentalAgentConversationAcceptedOutput"
          }
        }
      },
      "ExperimentalCanonicalInteractionAutoTurnSelection": {
        "type": "object",
        "properties": {
          "mode": { "type": "string", "const": "auto" }
        },
        "required": ["mode"]
      },
      "ExperimentalCanonicalInteractionNoneTurnSelection": {
        "type": "object",
        "properties": {
          "mode": { "type": "string", "const": "none" }
        },
        "required": ["mode"]
      },
      "ExperimentalCanonicalInteractionRequiredTurnSelection": {
        "type": "object",
        "properties": {
          "mode": { "type": "string", "const": "required" },
          "tool_name": { "type": "string" }
        },
        "required": ["mode"]
      },
      "ExperimentalCanonicalInteractionTurnSelection": {
        "oneOf": [
          { "$ref": "#/components/schemas/ExperimentalCanonicalInteractionAutoTurnSelection" },
          { "$ref": "#/components/schemas/ExperimentalCanonicalInteractionNoneTurnSelection" },
          { "$ref": "#/components/schemas/ExperimentalCanonicalInteractionRequiredTurnSelection" }
        ],
        "discriminator": {
          "propertyName": "mode",
          "mapping": {
            "auto": "#/components/schemas/ExperimentalCanonicalInteractionAutoTurnSelection",
            "none": "#/components/schemas/ExperimentalCanonicalInteractionNoneTurnSelection",
            "required": "#/components/schemas/ExperimentalCanonicalInteractionRequiredTurnSelection"
          }
        }
      },
      "ConversationJsonValue": {},
      "AppendRunConversationProgramTurnPayload": {
        "oneOf": [
          {
            "type": "object",
            "properties": {
              "conversation_id": { "type": "string" },
              "expected_revision": { "type": "integer" },
              "operation_id": { "type": "string" },
              "recorded_at": { "$ref": "#/components/schemas/ConversationTimestamp" },
              "purpose": { "type": "string", "const": "controller_corrective" },
              "text": { "type": "string" }
            },
            "required": ["conversation_id", "expected_revision", "operation_id", "recorded_at", "purpose", "text"]
          },
          {
            "type": "object",
            "properties": {
              "conversation_id": { "type": "string" },
              "expected_revision": { "type": "integer" },
              "operation_id": { "type": "string" },
              "recorded_at": { "$ref": "#/components/schemas/ConversationTimestamp" },
              "purpose": { "type": "string", "const": "terminal_result" },
              "result": {
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
                      "type": { "type": "string", "const": "json" },
                      "value": { "$ref": "#/components/schemas/ConversationJsonValue" }
                    },
                    "required": ["type", "value"]
                  }
                ],
                "discriminator": { "propertyName": "type" }
              }
            },
            "required": ["conversation_id", "expected_revision", "operation_id", "recorded_at", "purpose", "result"]
          }
        ],
        "discriminator": { "propertyName": "purpose" }
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
type ExperimentalAgentConversationEvent struct { Type string `json:"type"` }
type ExperimentalAgentConversationPreviewUnavailable struct { Type string `json:"type"` }
type ExperimentalAgentConversationAcceptedOutput struct { Type string `json:"type"` }
type ExperimentalAgentConversationStreamEnvelope struct {
	ExperimentalAgentConversationEvent *ExperimentalAgentConversationEvent
	ExperimentalAgentConversationPreviewUnavailable *ExperimentalAgentConversationPreviewUnavailable
	ExperimentalAgentConversationAcceptedOutput *ExperimentalAgentConversationAcceptedOutput
}
func (dst *ExperimentalAgentConversationStreamEnvelope) UnmarshalJSON(data []byte) error {
	return validator.Validate(dst)
}
var _ = json.Unmarshal
var _ = fmt.Errorf' > "$work_dir/openapi/model_experimental_agent_conversation_stream_envelope.go"
printf '%s\n' 'package fixture
import (
	"encoding/json"
	"fmt"
)
type ExperimentalCanonicalInteractionAutoTurnSelection struct {
	Mode string `json:"mode"`
}
type ExperimentalCanonicalInteractionNoneTurnSelection struct {
	Mode string `json:"mode"`
}
type ExperimentalCanonicalInteractionRequiredTurnSelection struct {
	Mode string `json:"mode"`
	ToolName *string `json:"tool_name,omitempty"`
}
type ExperimentalCanonicalInteractionTurnSelection struct {
	ExperimentalCanonicalInteractionAutoTurnSelection *ExperimentalCanonicalInteractionAutoTurnSelection
	ExperimentalCanonicalInteractionNoneTurnSelection *ExperimentalCanonicalInteractionNoneTurnSelection
	ExperimentalCanonicalInteractionRequiredTurnSelection *ExperimentalCanonicalInteractionRequiredTurnSelection
}
func (dst *ExperimentalCanonicalInteractionTurnSelection) UnmarshalJSON(data []byte) error {
	return fmt.Errorf("unpatched generated turn-selection union")
}
var _ = json.Unmarshal' > "$work_dir/openapi/model_experimental_canonical_interaction_turn_selection.go"
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

printf '%s\n' 'package fixture
import (
    "encoding/json"
    "fmt"
    "gopkg.in/validator.v2"
)
type AppendRunConversationProgramTurnPayloadOneOf struct {
    ConversationId string `json:"conversation_id"`
    ExpectedRevision int32 `json:"expected_revision"`
    OperationId string `json:"operation_id"`
    RecordedAt string `json:"recorded_at"`
    Purpose string `json:"purpose"`
    Text string `json:"text"`
}
func (value *AppendRunConversationProgramTurnPayloadOneOf) UnmarshalJSON(data []byte) error {
    fields := map[string]json.RawMessage{}
    if err := json.Unmarshal(data, &fields); err != nil { return err }
    for _, required := range []string{"conversation_id", "expected_revision", "operation_id", "recorded_at", "purpose", "text"} {
        if _, ok := fields[required]; !ok { return fmt.Errorf("missing %s", required) }
    }
    type alias AppendRunConversationProgramTurnPayloadOneOf
    return json.Unmarshal(data, (*alias)(value))
}
type AppendRunConversationProgramTurnPayloadOneOf1 struct {
    ConversationId string `json:"conversation_id"`
    ExpectedRevision int32 `json:"expected_revision"`
    OperationId string `json:"operation_id"`
    RecordedAt string `json:"recorded_at"`
    Purpose string `json:"purpose"`
    Result AppendRunConversationProgramTurnPayloadOneOf1Result `json:"result"`
}
func (value *AppendRunConversationProgramTurnPayloadOneOf1) UnmarshalJSON(data []byte) error {
    fields := map[string]json.RawMessage{}
    if err := json.Unmarshal(data, &fields); err != nil { return err }
    for _, required := range []string{"conversation_id", "expected_revision", "operation_id", "recorded_at", "purpose", "result"} {
        if _, ok := fields[required]; !ok { return fmt.Errorf("missing %s", required) }
    }
    type alias AppendRunConversationProgramTurnPayloadOneOf1
    return json.Unmarshal(data, (*alias)(value))
}
type AppendRunConversationProgramTurnPayload struct {
    AppendRunConversationProgramTurnPayloadOneOf *AppendRunConversationProgramTurnPayloadOneOf
    AppendRunConversationProgramTurnPayloadOneOf1 *AppendRunConversationProgramTurnPayloadOneOf1
}
func (dst *AppendRunConversationProgramTurnPayload) UnmarshalJSON(data []byte) error {
    return validator.Validate(dst)
}
func (src AppendRunConversationProgramTurnPayload) MarshalJSON() ([]byte, error) {
    if src.AppendRunConversationProgramTurnPayloadOneOf != nil { return json.Marshal(src.AppendRunConversationProgramTurnPayloadOneOf) }
    if src.AppendRunConversationProgramTurnPayloadOneOf1 != nil { return json.Marshal(src.AppendRunConversationProgramTurnPayloadOneOf1) }
    return nil, nil
}' > "$work_dir/openapi/model_append_run_conversation_program_turn_payload.go"
printf '%s\n' 'package fixture
import (
    "encoding/json"
    "fmt"
    "gopkg.in/validator.v2"
)
type AppendRunConversationProgramTurnPayloadOneOf1ResultOneOf struct {
    Type string `json:"type"`
    Text string `json:"text"`
}
func (value *AppendRunConversationProgramTurnPayloadOneOf1ResultOneOf) UnmarshalJSON(data []byte) error {
    fields := map[string]json.RawMessage{}
    if err := json.Unmarshal(data, &fields); err != nil { return err }
    for _, required := range []string{"type", "text"} {
        if _, ok := fields[required]; !ok { return fmt.Errorf("missing %s", required) }
    }
    type alias AppendRunConversationProgramTurnPayloadOneOf1ResultOneOf
    return json.Unmarshal(data, (*alias)(value))
}
type AppendRunConversationProgramTurnPayloadOneOf1Result struct {
    AppendRunConversationProgramTurnPayloadOneOf1ResultOneOf *AppendRunConversationProgramTurnPayloadOneOf1ResultOneOf
    AppendRunConversationProgramTurnPayloadOneOf1ResultOneOf1 *AppendRunConversationProgramTurnPayloadOneOf1ResultOneOf1
}
func (dst *AppendRunConversationProgramTurnPayloadOneOf1Result) UnmarshalJSON(data []byte) error {
    return validator.Validate(dst)
}
func (src AppendRunConversationProgramTurnPayloadOneOf1Result) MarshalJSON() ([]byte, error) {
    if src.AppendRunConversationProgramTurnPayloadOneOf1ResultOneOf != nil { return json.Marshal(src.AppendRunConversationProgramTurnPayloadOneOf1ResultOneOf) }
    if src.AppendRunConversationProgramTurnPayloadOneOf1ResultOneOf1 != nil { return json.Marshal(src.AppendRunConversationProgramTurnPayloadOneOf1ResultOneOf1) }
    return nil, nil
}' > "$work_dir/openapi/model_append_run_conversation_program_turn_payload_one_of_1_result.go"
printf '%s\n' 'package fixture
import (
    "encoding/json"
    "fmt"
)
type AppendRunConversationProgramTurnPayloadOneOf1ResultOneOf1 struct {
    Type string `json:"type"`
    Value interface{} `json:"value"`
}
func (o AppendRunConversationProgramTurnPayloadOneOf1ResultOneOf1) MarshalJSON() ([]byte, error) {
    value, err := o.ToMap()
    if err != nil { return nil, err }
    return json.Marshal(value)
}
func (o AppendRunConversationProgramTurnPayloadOneOf1ResultOneOf1) ToMap() (map[string]interface{}, error) {
    toSerialize := map[string]interface{}{"type": o.Type}
    if o.Value != nil {
        toSerialize["value"] = o.Value
    }
    return toSerialize, nil
}
func (o *AppendRunConversationProgramTurnPayloadOneOf1ResultOneOf1) UnmarshalJSON(data []byte) error {
    fields := map[string]json.RawMessage{}
    if err := json.Unmarshal(data, &fields); err != nil { return err }
    for _, required := range []string{"type", "value"} {
        if _, ok := fields[required]; !ok { return fmt.Errorf("missing %s", required) }
    }
    type alias AppendRunConversationProgramTurnPayloadOneOf1ResultOneOf1
    return json.Unmarshal(data, (*alias)(o))
}' > "$work_dir/openapi/model_append_run_conversation_program_turn_payload_one_of_1_result_one_of_1.go"

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
grep -q '\*dst = ExperimentalAgentConversationStreamEnvelope{}' \
    "$work_dir/openapi/model_experimental_agent_conversation_stream_envelope.go"
grep -q 'case "accepted_output"' \
    "$work_dir/openapi/model_experimental_agent_conversation_stream_envelope.go"
if grep -q 'validator.v2' "$work_dir/openapi/model_experimental_agent_conversation_stream_envelope.go"; then
    echo 'unused experimental agent stream validator import was not removed' >&2
    exit 1
fi
grep -q '\*dst = ExperimentalCanonicalInteractionTurnSelection{}' \
    "$work_dir/openapi/model_experimental_canonical_interaction_turn_selection.go"
grep -q 'case "auto"' "$work_dir/openapi/model_experimental_canonical_interaction_turn_selection.go"
grep -q 'case "none"' "$work_dir/openapi/model_experimental_canonical_interaction_turn_selection.go"
grep -q 'case "required"' "$work_dir/openapi/model_experimental_canonical_interaction_turn_selection.go"
grep -q '\*dst = AppendRunConversationProgramTurnPayload{}' \
    "$work_dir/openapi/model_append_run_conversation_program_turn_payload.go"
grep -q 'case "terminal_result"' \
    "$work_dir/openapi/model_append_run_conversation_program_turn_payload.go"
grep -q '\*dst = AppendRunConversationProgramTurnPayloadOneOf1Result{}' \
    "$work_dir/openapi/model_append_run_conversation_program_turn_payload_one_of_1_result.go"
grep -q 'case "json"' \
    "$work_dir/openapi/model_append_run_conversation_program_turn_payload_one_of_1_result.go"
grep -q 'toSerialize\["value"\] = o.Value' \
    "$work_dir/openapi/model_append_run_conversation_program_turn_payload_one_of_1_result_one_of_1.go"
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

func TestExperimentalAgentStreamEnvelopeDispatchAndReset(t *testing.T) {
	var envelope ExperimentalAgentConversationStreamEnvelope
	if err := json.Unmarshal([]byte(`{"type":"preview_unavailable"}`), &envelope); err != nil { t.Fatal(err) }
	if envelope.ExperimentalAgentConversationPreviewUnavailable == nil { t.Fatal("preview branch not selected") }
	if err := json.Unmarshal([]byte(`{"type":"accepted_output"}`), &envelope); err != nil { t.Fatal(err) }
	if envelope.ExperimentalAgentConversationPreviewUnavailable != nil || envelope.ExperimentalAgentConversationAcceptedOutput == nil {
		t.Fatalf("stale branch after accepted output reuse: %#v", envelope)
	}
	if err := json.Unmarshal([]byte(`{"type":"future_envelope"}`), &envelope); err == nil {
		t.Fatal("unknown experimental agent stream discriminator accepted")
	}
	if envelope.ExperimentalAgentConversationEvent != nil ||
		envelope.ExperimentalAgentConversationPreviewUnavailable != nil ||
		envelope.ExperimentalAgentConversationAcceptedOutput != nil {
		t.Fatalf("failed reuse retained a branch: %#v", envelope)
	}
}

func TestCanonicalTurnSelectionDispatchValidationAndReset(t *testing.T) {
	var selection ExperimentalCanonicalInteractionTurnSelection
	if err := json.Unmarshal([]byte(`{"mode":"auto"}`), &selection); err != nil { t.Fatal(err) }
	if selection.ExperimentalCanonicalInteractionAutoTurnSelection == nil ||
		selection.ExperimentalCanonicalInteractionNoneTurnSelection != nil ||
		selection.ExperimentalCanonicalInteractionRequiredTurnSelection != nil {
		t.Fatalf("auto branch not selected: %#v", selection)
	}
	if err := json.Unmarshal([]byte(`{"mode":"none"}`), &selection); err != nil { t.Fatal(err) }
	if selection.ExperimentalCanonicalInteractionAutoTurnSelection != nil ||
		selection.ExperimentalCanonicalInteractionNoneTurnSelection == nil ||
		selection.ExperimentalCanonicalInteractionRequiredTurnSelection != nil {
		t.Fatalf("none branch did not reset auto branch: %#v", selection)
	}
	if err := json.Unmarshal([]byte(`{"mode":"required"}`), &selection); err != nil { t.Fatal(err) }
	if selection.ExperimentalCanonicalInteractionRequiredTurnSelection == nil ||
		selection.ExperimentalCanonicalInteractionRequiredTurnSelection.ToolName != nil {
		t.Fatalf("unnamed required branch not selected: %#v", selection)
	}
	if err := json.Unmarshal([]byte(`{"mode":"required","tool_name":"lookup"}`), &selection); err != nil { t.Fatal(err) }
	required := selection.ExperimentalCanonicalInteractionRequiredTurnSelection
	if required == nil || required.ToolName == nil || *required.ToolName != "lookup" {
		t.Fatalf("named required branch changed: %#v", selection)
	}
	for name, body := range map[string]string{
		"missing mode": `{}`,
		"unknown mode": `{"mode":"future"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(`{"mode":"required","tool_name":"seed"}`), &selection); err != nil { t.Fatal(err) }
			if err := json.Unmarshal([]byte(body), &selection); err == nil { t.Fatalf("invalid selection accepted: %s", body) }
			if selection.ExperimentalCanonicalInteractionAutoTurnSelection != nil ||
				selection.ExperimentalCanonicalInteractionNoneTurnSelection != nil ||
				selection.ExperimentalCanonicalInteractionRequiredTurnSelection != nil {
				t.Fatalf("failed selection reuse retained a branch: %#v", selection)
			}
		})
	}
}

func TestTerminalProgramDispatchPreservesJSONRootsAndResets(t *testing.T) {
	prefix := `{"conversation_id":"conversation","expected_revision":1,"operation_id":"operation","recorded_at":"2026-09-30T00:00:00.000Z","purpose":"terminal_result","result":`
	for _, result := range []string{
		`{"type":"json","value":null}`,
		`{"type":"json","value":false}`,
		`{"type":"json","value":[null,false,{"nested":true}]}`,
		`{"type":"text","text":"done"}`,
	} {
		body := prefix + result + `}`
		var payload AppendRunConversationProgramTurnPayload
		if err := json.Unmarshal([]byte(body), &payload); err != nil { t.Fatal(err) }
		terminal := payload.AppendRunConversationProgramTurnPayloadOneOf1
		if terminal == nil || payload.AppendRunConversationProgramTurnPayloadOneOf != nil {
			t.Fatalf("terminal branch not selected: %#v", payload)
		}
		serialized, err := json.Marshal(payload)
		if err != nil { t.Fatal(err) }
		var before, after map[string]interface{}
		if err := json.Unmarshal([]byte(body), &before); err != nil { t.Fatal(err) }
		if err := json.Unmarshal(serialized, &after); err != nil { t.Fatal(err) }
		if !reflect.DeepEqual(before, after) { t.Fatalf("terminal result changed: %s", serialized) }
	}

	controller := `{"conversation_id":"conversation","expected_revision":1,"operation_id":"operation","recorded_at":"2026-09-30T00:00:00.000Z","purpose":"controller_corrective","text":"continue"}`
	var payload AppendRunConversationProgramTurnPayload
	if err := json.Unmarshal([]byte(controller), &payload); err != nil { t.Fatal(err) }
	if payload.AppendRunConversationProgramTurnPayloadOneOf == nil || payload.AppendRunConversationProgramTurnPayloadOneOf1 != nil {
		t.Fatalf("controller branch not selected: %#v", payload)
	}

	invalid := map[string]string{
		"missing purpose": `{"conversation_id":"conversation"}`,
		"unknown purpose": `{"purpose":"future"}`,
		"unknown result type": prefix + `{"type":"future","text":"done"}}`,
		"json result missing value": prefix + `{"type":"json"}}`,
		"text result missing text": prefix + `{"type":"text"}}`,
	}
	for name, body := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(controller), &payload); err != nil { t.Fatal(err) }
			if err := json.Unmarshal([]byte(body), &payload); err == nil { t.Fatalf("invalid payload accepted: %s", body) }
			if payload.AppendRunConversationProgramTurnPayloadOneOf != nil || payload.AppendRunConversationProgramTurnPayloadOneOf1 != nil {
				t.Fatalf("failed payload reuse retained a branch: %#v", payload)
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
