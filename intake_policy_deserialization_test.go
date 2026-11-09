package vertesia

import (
	"encoding/json"
	"testing"

	"github.com/vertesia/vertesia-client-go/openapi"
)

// The shared intake policy exercises generated integer counters and provider options.

func intakePolicy(t *testing.T) openapi.ContentTypeIntakePolicy {
	t.Helper()
	var policy openapi.ContentTypeIntakePolicy
	if err := json.Unmarshal([]byte(studioFixture(t, "content-type-intake-policy.json")), &policy); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	return policy
}

func TestDeserializesAFullIntakePolicy(t *testing.T) {
	policy := intakePolicy(t)
	if policy.Mode == nil || *policy.Mode != "programmatic" {
		t.Fatalf("mode = %v", policy.Mode)
	}
	if policy.DefaultView == nil || string(*policy.DefaultView) != "pdf" {
		t.Fatalf("default_view = %v", policy.DefaultView)
	}
	if policy.Extraction == nil || policy.Extraction.Grounding == nil {
		t.Fatal("extraction.grounding should be populated")
	}
	if policy.Embeddings == nil || policy.Embeddings.Text == nil || !*policy.Embeddings.Text {
		t.Fatalf("embeddings.text = %v", policy.Embeddings)
	}
	// The enums that were unconstrained strings in the schema this replaced.
	if policy.Extraction.Config == nil || policy.Extraction.Config.ConfigMode == nil {
		t.Fatal("extraction.config.configMode should be populated")
	}
	if string(*policy.Extraction.Config.ConfigMode) != "RUN_CONFIG_ONLY" {
		t.Fatalf("configMode = %v", *policy.Extraction.Config.ConfigMode)
	}
}

// The whole point of the batch, in the type system that can see it. int32 rather than int64 is what
// `z.int32()` buys: retaining z.int()'s safe-integer bounds would have generated 64-bit fields for a
// page count.
func TestMapsThePageAndDpiCountersToInt32(t *testing.T) {
	policy := intakePolicy(t)

	var maxPages int32 = *policy.Extraction.MaxPages
	if maxPages != 6 {
		t.Fatalf("extraction.max_pages = %d", maxPages)
	}
	if got := *policy.Extraction.Vision.MaxImageTokens; got != 9000 {
		t.Fatalf("vision.max_image_tokens = %d", got)
	}
	if got := *policy.Extraction.Vision.MaxPagesPerCall; got != 4 {
		t.Fatalf("vision.max_pages_per_call = %d", got)
	}
	if got := *policy.Extraction.Grounding.MaxPages; got != 12 {
		t.Fatalf("grounding.max_pages = %d", got)
	}
	if got := *policy.Extraction.Grounding.WindowPages; got != 4 {
		t.Fatalf("grounding.window_pages = %d", got)
	}
	if got := *policy.TextConversion.RenderDpi; got != 150 {
		t.Fatalf("text_conversion.render_dpi = %d", got)
	}
	if got := *policy.Locate.MinPages; got != 10 {
		t.Fatalf("locate.min_pages = %d", got)
	}

	// float32, and asserted as the contrast: `max_payload_mb` is a megabyte budget rather than a
	// count, so it stayed `z.number()`. Only the counters this batch moved to `z.int32()` generate
	// as int32, which is what makes that change visible rather than a blanket property of the
	// converted schema.
	var payloadMb float32 = *policy.Extraction.Vision.MaxPayloadMb
	if payloadMb != 12 {
		t.Fatalf("vision.max_payload_mb = %v", payloadMb)
	}
}

// `number[][]`, not a tuple: a Zod tuple emits prefixItems, which this generator renders as a
// positional struct rather than a slice. Negative indexes count from the end of the document, so the
// element type has to be signed.
func TestKeepsPageRangesAsSlicesOfInt32(t *testing.T) {
	var ranges [][]int32 = intakePolicy(t).Extraction.PageRanges
	want := [][]int32{{1, 2}, {-1, -1}}
	if len(ranges) != len(want) {
		t.Fatalf("page_ranges = %v", ranges)
	}
	for i := range want {
		if len(ranges[i]) != 2 || ranges[i][0] != want[i][0] || ranges[i][1] != want[i][1] {
			t.Fatalf("page_ranges[%d] = %v, want %v", i, ranges[i], want[i])
		}
	}
}

func TestModelOptionsResolvesDiscriminators(t *testing.T) {
	var options openapi.ModelOptions
	if err := json.Unmarshal([]byte(`{"_option_id":"vertexai-claude","max_tokens":4096}`), &options); err != nil {
		t.Fatal(err)
	}
	if options.VertexAIClaudeOptions == nil || options.VertexAIClaudeOptions.MaxTokens == nil || *options.VertexAIClaudeOptions.MaxTokens != 4096 {
		t.Fatalf("wrong Vertex Claude options: %+v", options)
	}
	if err := json.Unmarshal([]byte(`{"_option_id":"anthropic-claude","cache_ttl":"1h","effort":"high"}`), &options); err != nil {
		t.Fatal(err)
	}
	if options.AnthropicClaudeOptions == nil || options.VertexAIClaudeOptions != nil {
		t.Fatal("Anthropic decoding must select exactly its branch, including when reusing a destination")
	}
	for _, body := range []string{`[]`, `{"_option_id":12}`, `{"_option_id":"anthropic-claude","max_tokens":"bad"}`} {
		if err := json.Unmarshal([]byte(body), &options); err == nil {
			t.Fatalf("accepted invalid model options: %s", body)
		}
	}
}

func TestOpenAITextOptionsToolChoiceDecodes(t *testing.T) {
	var options openapi.OpenAiTextOptions
	if err := json.Unmarshal([]byte(`{"_option_id":"openai-text","tool_choice":"required"}`), &options); err != nil {
		t.Fatalf("OpenAiTextOptions did not decode: %v", err)
	}
	if options.ToolChoice == nil || string(*options.ToolChoice) != "required" {
		t.Fatalf("tool_choice = %v, want required", options.ToolChoice)
	}
}

func TestOpenRouterRoutingOptionShape(t *testing.T) {
	var options openapi.OpenRouterTextOptions
	err := json.Unmarshal([]byte(`{
		"_option_id":"openrouter-text",
		"provider_sort":"throughput",
		"provider_order":["google-vertex","amazon-bedrock"],
		"provider_allow_fallbacks":false
	}`), &options)
	if err != nil {
		t.Fatalf("OpenRouterTextOptions failed to decode: %v", err)
	}
	if options.ProviderSort == nil || *options.ProviderSort != "throughput" {
		t.Fatalf("provider_sort = %v", options.ProviderSort)
	}
	if len(options.ProviderOrder) != 2 || options.ProviderOrder[0] != "google-vertex" {
		t.Fatalf("provider_order = %v", options.ProviderOrder)
	}
	if options.ProviderAllowFallbacks == nil || *options.ProviderAllowFallbacks {
		t.Fatalf("provider_allow_fallbacks = %v", options.ProviderAllowFallbacks)
	}
}

func TestOpenAICompatibleExtraBodyShape(t *testing.T) {
	var options openapi.OpenAiTextOptions
	err := json.Unmarshal([]byte(`{
		"_option_id":"openai-text",
		"extra_body":{
			"provider":{"sort":"throughput","allow_fallbacks":false},
			"baseten":{"performance":"max"}
		}
	}`), &options)
	if err != nil {
		t.Fatalf("OpenAiTextOptions failed to decode: %v", err)
	}
	provider, ok := options.ExtraBody["provider"].(map[string]interface{})
	if !ok || provider["sort"] != "throughput" || provider["allow_fallbacks"] != false {
		t.Fatalf("extra_body.provider = %v", options.ExtraBody["provider"])
	}
	baseten, ok := options.ExtraBody["baseten"].(map[string]interface{})
	if !ok || baseten["performance"] != "max" {
		t.Fatalf("extra_body.baseten = %v", options.ExtraBody["baseten"])
	}
}

// Untagged options are valid in requests and responses. Response decoding also
// preserves unknown future families that the current request schema cannot validate.
func TestModelOptionsPreservesLegacyAndFutureResponses(t *testing.T) {
	for _, body := range []string{
		`{}`, `null`, `{"temperature":0.2,"max_tokens":1024}`,
		`{"_option_id":"future-provider","max_tokens":1024,"nested":{"enabled":true}}`,
	} {
		var options openapi.ModelOptions
		if err := json.Unmarshal([]byte(body), &options); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(options)
		if err != nil {
			t.Fatal(err)
		}
		if string(encoded) != body {
			t.Fatalf("options lost data: %s", encoded)
		}
	}
	for _, body := range []string{`{}`, `{"model_options":null}`} {
		var config openapi.InteractionExecutionConfiguration
		if err := json.Unmarshal([]byte(body), &config); err != nil {
			t.Fatal(err)
		}
		if config.ModelOptions != nil {
			t.Fatal("absent/null options should remain absent")
		}
	}
	var config openapi.InteractionExecutionConfiguration
	if err := json.Unmarshal([]byte(`{"model_options":{"temperature":0.2}}`), &config); err != nil {
		t.Fatal(err)
	}
	if config.ModelOptions == nil || string(config.ModelOptions.Raw) != `{"temperature":0.2}` {
		t.Fatal("nested legacy options were not preserved")
	}
}
