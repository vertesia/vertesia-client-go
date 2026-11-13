package openapi

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// A released client must keep decoding responses from a newer server. These cases are what a
// newer server can add without a breaking API change: a property, an enum value, a union variant.

const forwardCompatRun = `{
	"id": "000000000000000000000001",
	"account": "000000000000000000000002",
	"project": "000000000000000000000003",
	"interaction": "sys:GeneralAgent",
	"interactionRef": {"id": "sys:GeneralAgent", "name": "sys:GeneralAgent", "endpoint": "sys:GeneralAgent",
		"status": "code", "version": 0, "tags": [], "updated_at": "1970-01-01T00:00:00.000Z", "future_ref_field": 1},
	"run_kind": "agent",
	"run_type": "RUN_TYPE",
	"status": "STATUS",
	"started_by": "user:000000000000000000000004",
	"started_at": "2026-07-28T11:08:41.127Z",
	"created_at": "2026-07-28T11:08:41.128Z",
	"updated_at": "2026-07-29T10:59:13.166Z",
	"future_top_level_field": {"nested": true}
}`

func forwardCompatRunJSON(runType, status string) []byte {
	return []byte(strings.NewReplacer("RUN_TYPE", runType, "STATUS", status).Replace(forwardCompatRun))
}

func TestForwardCompatUnknownProperties(t *testing.T) {
	// Top level and nested in a union member.
	var run AgentRunResponse
	if err := json.Unmarshal(forwardCompatRunJSON("autonomous", "completed"), &run); err != nil {
		t.Fatal(err)
	}
	if run.AutonomousRunResponse == nil || run.AutonomousRunResponse.GetInteractionRef().Name != "sys:GeneralAgent" {
		t.Fatalf("run not decoded: %#v", run)
	}
	// Inside a union member.
	var result CompletionResult
	if err := json.Unmarshal([]byte(`{"type":"text","value":"hello","future":{"a":1}}`), &result); err != nil {
		t.Fatal(err)
	}
	if result.TextResult == nil || result.TextResult.Value != "hello" {
		t.Fatalf("text result not decoded: %#v", result)
	}
}

func TestForwardCompatUnknownEnumValueIsKept(t *testing.T) {
	var accountType AccountType
	if err := json.Unmarshal([]byte(`"new-account-kind"`), &accountType); err != nil {
		t.Fatal(err)
	}
	if accountType != "new-account-kind" || accountType.IsValid() {
		t.Fatalf("AccountType = %q (valid %v), want the received value, reported invalid", accountType, accountType.IsValid())
	}
	// Read-modify-write sends the value back, not a placeholder.
	var run AgentRunResponse
	if err := json.Unmarshal(forwardCompatRunJSON("autonomous", "hibernating"), &run); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"status":"hibernating"`)) {
		t.Fatalf("unknown status not written back: %s", encoded)
	}
}

func assertKeptRaw(t *testing.T, body string, union interface {
	GetActualInstance() interface{}
}, raw json.RawMessage, encode func() ([]byte, error)) {
	t.Helper()
	if !bytes.Equal(raw, []byte(body)) {
		t.Fatalf("Raw = %s, want %s", raw, body)
	}
	if got, ok := union.GetActualInstance().(json.RawMessage); !ok || !bytes.Equal(got, []byte(body)) {
		t.Fatalf("GetActualInstance = %#v, want the raw variant", union.GetActualInstance())
	}
	encoded, err := encode()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, []byte(body)) {
		t.Fatalf("re-encoded %s, want %s", encoded, body)
	}
}

func TestForwardCompatUnknownDiscriminatedVariant(t *testing.T) {
	body := `{"type":"speech","value":"gs://bucket/a.ogg","voice":"alloy"}`
	var result CompletionResult
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatalf("unknown CompletionResult type: %v", err)
	}
	assertSingleBranch(t, result, "Raw")
	assertKeptRaw(t, body, &result, result.Raw, result.MarshalJSON)

	// One unknown element must not fail the list around it.
	var results []CompletionResult
	if err := json.Unmarshal([]byte(`[{"type":"text","value":"a"},`+body+`]`), &results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].TextResult == nil || results[1].Raw == nil {
		t.Fatalf("results = %#v", results)
	}

	runBody := string(forwardCompatRunJSON("scheduled", "completed"))
	var run AgentRunResponse
	if err := json.Unmarshal([]byte(runBody), &run); err != nil {
		t.Fatalf("unknown AgentRunResponse run_type: %v", err)
	}
	assertSingleBranch(t, run, "Raw")
	if !bytes.Equal(run.Raw, []byte(runBody)) {
		t.Fatalf("AgentRunResponse.Raw = %s", run.Raw)
	}

	// A discriminated union the generator decodes by trial: an unknown type used to be read as
	// whichever branch has the same required fields.
	targetBody := `{"type":"slack","url":"https://hooks.slack.com/x","channel":"#ops"}`
	var target EventDeliveryTarget
	if err := json.Unmarshal([]byte(targetBody), &target); err != nil {
		t.Fatal(err)
	}
	assertSingleBranch(t, target, "Raw")
	assertKeptRaw(t, targetBody, &target, target.Raw, target.MarshalJSON)

	// Known variants stay typed and strict; a missing discriminator is still an error.
	for _, invalid := range []string{`{}`, `null`, `{"value":"missing type"}`, `{"type":"text"}`, `{"type":7}`} {
		if err := json.Unmarshal([]byte(invalid), &result); err == nil {
			t.Fatalf("accepted invalid result %s: %#v", invalid, result)
		}
		if result.GetActualInstance() != nil {
			t.Fatalf("retained a stale variant after %s", invalid)
		}
	}
}

func TestForwardCompatUnknownUndiscriminatedVariant(t *testing.T) {
	// oneOf of two objects: neither branch's required properties are present.
	body := `{"loop":{"over":"items"},"node":7}`
	var branch NodeDefinitionBranchesInner
	if err := json.Unmarshal([]byte(body), &branch); err != nil {
		t.Fatalf("unknown NodeDefinition branch: %v", err)
	}
	assertSingleBranch(t, branch, "Raw")
	assertKeptRaw(t, body, &branch, branch.Raw, branch.MarshalJSON)
	if err := json.Unmarshal([]byte(`{"to":"next"}`), &branch); err != nil || branch.BranchDefinition == nil || branch.Raw != nil {
		t.Fatalf("known branch: %v %#v", err, branch)
	}

	// anyOf, and a variant of another JSON type.
	var parameter McpConnectionParameter
	if err := json.Unmarshal([]byte(`["oauth"]`), &parameter); err != nil {
		t.Fatalf("unknown McpConnectionParameter: %v", err)
	}
	if string(parameter.Raw) != `["oauth"]` || parameter.McpPublicParameter != nil {
		t.Fatalf("parameter = %#v", parameter)
	}
	if encoded, err := json.Marshal(parameter); err != nil || string(encoded) != `["oauth"]` {
		t.Fatalf("re-encoded %s, %v", encoded, err)
	}
}

func TestForwardCompatModelOptionsKeepEveryReceivedOption(t *testing.T) {
	// Stored options are often merged from several families (a run's over its interaction's).
	gemini := `"temperature":0.2,"thinking_budget_tokens":2048,"effort":"high","person_generation":"allow_adult"`
	// This one fits no family: Claude's cache_ttl and an image size beside Gemini options.
	mixed := `"temperature":0.2,"thinking_budget_tokens":2048,"cache_ttl":"1h","size":"1024x1024"`
	for _, options := range []string{gemini, mixed} {
		for _, tagged := range []bool{false, true} {
			body := "{" + options + "}"
			if tagged {
				body = `{"_option_id":"vertexai-gemini",` + options + "}"
			}
			var execution StatelessExecutionOptions
			if err := json.Unmarshal([]byte(`{"model":"gemini","model_options":`+body+`}`), &execution); err != nil {
				t.Fatalf("%s: %v", body, err)
			}
			// The family ID selects the branch; without one the options are kept raw.
			if _, typed := execution.ModelOptions.GetActualInstance().(*VertexAIGeminiOptions); typed != tagged {
				t.Fatalf("%s: decoded as %T", body, execution.ModelOptions.GetActualInstance())
			}
			out, err := json.Marshal(execution)
			if err != nil {
				t.Fatal(err)
			}
			var received, written map[string]json.RawMessage
			var wrapper struct {
				ModelOptions map[string]json.RawMessage `json:"model_options"`
			}
			if err := json.Unmarshal([]byte(body), &received); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(out, &wrapper); err != nil {
				t.Fatal(err)
			}
			written = wrapper.ModelOptions
			for key, value := range received {
				if !bytes.Equal(value, written[key]) {
					t.Errorf("%s: %s was written as %s", body, key, written[key])
				}
			}
		}
	}
}
