package vertesia

import (
	"context"
	"testing"

	"github.com/vertesia/vertesia-client-go/openapi"
)

// These calls intentionally mirror the public Go client's integration test. Their value is at
// compile time: OpenAPI Generator derives constructor argument order and request-body setters from
// the document, so semantically equivalent schema rewrites can still break downstream Go source.
func TestPromptAPIRetainsItsGeneratedSourceShape(t *testing.T) {
	payload := openapi.NewPromptTemplateCreatePayload(
		"prompt-name",
		openapi.PROMPTROLE_USER,
		"Hello, <%= name %>.",
		openapi.TEMPLATETYPE_JST,
	)
	if payload.GetName() != "prompt-name" {
		t.Fatalf("prompt name = %q", payload.GetName())
	}

	client := openapi.NewAPIClient(openapi.NewConfiguration())
	request := client.PromptTemplatesAPI.RenderPrompt(context.Background(), "prompt-id").
		RequestBody(map[string]interface{}{"name": "Vertesia"})
	_ = request
}
