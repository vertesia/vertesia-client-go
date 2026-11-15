package openapi

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/validator.v2"
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

func TestForwardCompatLiteralTagSelectsTheBranch(t *testing.T) {
	// McpPublicParameter ignores unknown properties, so the generated decoder took it for any object.
	body := `{"type":"oauth","title":"token","authorization_url":"https://example.com"}`
	var parameter McpConnectionParameter
	if err := json.Unmarshal([]byte(body), &parameter); err != nil {
		t.Fatal(err)
	}
	if string(parameter.Raw) != body || parameter.McpPublicParameter != nil {
		t.Fatalf("parameter = %#v", parameter)
	}
	if encoded, err := json.Marshal(parameter); err != nil || string(encoded) != body {
		t.Fatalf("re-encoded %s, %v", encoded, err)
	}

	if err := json.Unmarshal([]byte(`{"type":"secret","title":"key"}`), &parameter); err != nil {
		t.Fatal(err)
	}
	if parameter.McpSecretParameter == nil {
		t.Fatalf("secret decoded as %#v", parameter)
	}
	for _, public := range []string{`{"type":"string","title":"name"}`, `{"title":"name"}`} {
		if err := json.Unmarshal([]byte(public), &parameter); err != nil {
			t.Fatal(err)
		}
		if parameter.McpPublicParameter == nil {
			t.Fatalf("%s decoded as %#v", public, parameter)
		}
	}
}

// The spec's patterns describe what the server accepts today, and a newer server can widen them. A
// typed branch keeps a value that breaks the pattern this client was generated with, and one such
// result does not fail the results beside it. The server validates what it receives.
func TestForwardCompatPatternsAreLeftToTheServer(t *testing.T) {
	// The id pattern is ^[a-z][a-z0-9_-]*$.
	list := `{"type":"list","id":"INVALID ID","label":"Items","title":{"field":"document.title"}}`
	var display ViewDisplayConfiguration
	if err := json.Unmarshal([]byte(list), &display); err != nil {
		t.Fatal(err)
	}
	if display.ViewListDisplay == nil || display.ViewListDisplay.Id != "INVALID ID" {
		t.Fatalf("decoded as %#v", display.GetActualInstance())
	}
	assertSameJSON(t, list, display)

	// An audio result whose value and mime type are outside ^(?:gs|s3):\/\/... and ^audio\/.*
	results := `[{"type":"text","value":"hello"},` +
		`{"type":"audio","value":"https://cdn.example.com/a.ogg","mime_type":"application/ogg"},` +
		`{"type":"image","value":"gs://bucket/image.png"}]`
	var decoded []CompletionResult
	if err := json.Unmarshal([]byte(results), &decoded); err != nil {
		t.Fatal(err)
	}
	for i, result := range decoded {
		if result.GetActualInstance() == nil || result.Raw != nil {
			t.Fatalf("result %d is not typed: %#v", i, result)
		}
	}
	assertSameJSON(t, results, decoded)

	// Requests are not checked against the patterns either.
	for _, payload := range []interface{}{
		AuditAggregationMetric{Id: "bad id"},
		CopyFilePayload{Source: "a/b.txt", Dest: "../d.txt"},
	} {
		if err := validator.Validate(payload); err != nil {
			t.Errorf("%T rejected: %v", payload, err)
		}
	}
}

// Types are still checked, and a required string, number or struct that the spec does not let be
// null cannot be null: it would decode as its zero value and be written back as "".
func TestForwardCompatRequiredPropertiesAreNotNull(t *testing.T) {
	for _, body := range []string{
		`{"type":"audio","value":"gs://bucket/a.wav","mime_type":null}`,
		`{"type":"audio","value":null,"mime_type":"audio/wav"}`,
		`[{"type":"text","value":"hello"},{"type":"audio","value":"gs://bucket/a.wav","mime_type":null}]`,
		`{"status":"enabled","billing_method":null,"portal_url":null}`,
		`{"type":"audio","value":"gs://bucket/a.wav","mime_type":7}`,
	} {
		var err error
		switch {
		case strings.HasPrefix(body, "["):
			var results []CompletionResult
			err = json.Unmarshal([]byte(body), &results)
		case strings.Contains(body, "billing_method"):
			var status StripeBillingStatusResponse
			err = json.Unmarshal([]byte(body), &status)
		default:
			var result CompletionResult
			err = json.Unmarshal([]byte(body), &result)
		}
		if err == nil {
			t.Errorf("accepted %s", body)
		}
	}
	// A required property that is nullable can still be null.
	enabled := `{"status":"enabled","billing_method":"stripe","portal_url":null}`
	var status StripeBillingStatusResponse
	if err := json.Unmarshal([]byte(enabled), &status); err != nil {
		t.Fatal(err)
	}
	assertSameJSON(t, enabled, status)
}

// The spec decides which required properties can be null, whatever their Go type: a tool call
// without input, and the payload that removes an installation's tool allowlist.
func TestForwardCompatNullableRequiredPropertiesAcceptNull(t *testing.T) {
	var calls []ToolUse
	body := `[{"id":"1","tool_name":"search","tool_input":{"q":"a"}},{"id":"2","tool_name":"now","tool_input":null}]`
	if err := json.Unmarshal([]byte(body), &calls); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0].ToolInput["q"] != "a" || calls[1].ToolName != "now" || calls[1].ToolInput != nil {
		t.Fatalf("decoded %#v", calls)
	}

	var payload UpdateAppInstallationToolAllowlistPayload
	if err := json.Unmarshal([]byte(`{"tool_allowlist":null}`), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ToolAllowlist != nil {
		t.Fatalf("decoded %#v", payload)
	}
	// The property is still required.
	if err := json.Unmarshal([]byte(`{}`), &payload); err == nil {
		t.Fatal("accepted a payload without tool_allowlist")
	}
}

func assertSameJSON(t *testing.T, want string, value interface{}) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var before, after interface{}
	if err := json.Unmarshal([]byte(want), &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &after); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("written as %s, want %s", encoded, want)
	}
}
