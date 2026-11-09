package vertesia

import (
	"encoding/json"
	"testing"

	"github.com/vertesia/vertesia-client-go/openapi"
)

// Pins the generated response model to the title/tags fields normalized by the resolve endpoint.
func TestResolvedCatalogInteractionIncludesNormalizedTitleAndTags(t *testing.T) {
	body := `{
		"type":"sys",
		"id":"sys:GeneralAgent",
		"name":"GeneralAgent",
		"title":"General Agent",
		"tags":["agent"],
		"prompts":[]
	}`
	var parsed openapi.ResolvedCatalogInteraction
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("unmarshal resolved interaction failed: %v", err)
	}

	if parsed.Title != "General Agent" {
		t.Fatalf("title = %q", parsed.Title)
	}
	if len(parsed.Tags) != 1 || parsed.Tags[0] != "agent" {
		t.Fatalf("tags = %v", parsed.Tags)
	}
}
