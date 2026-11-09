package vertesia

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/vertesia/vertesia-client-go/openapi"
)

// Shared project responses exercise generated timestamps and the open integration map.

func project(t *testing.T, name string) openapi.Project {
	t.Helper()
	var parsed openapi.Project
	if err := json.Unmarshal([]byte(studioFixture(t, name)), &parsed); err != nil {
		t.Fatalf("unmarshal %s failed: %v", name, err)
	}
	return parsed
}

func TestDeserializesAFullyPopulatedProject(t *testing.T) {
	parsed := project(t, "project-populated.json")
	if parsed.Id != "68b1779130afe5403a1589ba" {
		t.Fatalf("id = %q", parsed.Id)
	}
	if parsed.Name != "Acme" || parsed.Namespace != "acme" {
		t.Fatalf("name/namespace = %q/%q", parsed.Name, parsed.Namespace)
	}
	if parsed.Description == nil || *parsed.Description != "A project" {
		t.Fatalf("description = %v", parsed.Description)
	}
	if len(parsed.Plugins) != 1 || parsed.Plugins[0] != "plugin-a" {
		t.Fatalf("plugins = %v", parsed.Plugins)
	}
	text := parsed.Configuration.Embeddings.Text
	if text == nil || !text.Enabled {
		t.Fatalf("configuration.embeddings.text = %v", text)
	}
}

// The whole point of the conversion, in the type system that can see it. `format: date-time`
// survived it — a plain `z.string()` in the Zod schema would have dropped the format and narrowed
// these to `string`, which no TypeScript check could have caught.
func TestKeepsTheTimestampsAsTimeTime(t *testing.T) {
	parsed := project(t, "project-populated.json")
	var createdAt time.Time = parsed.CreatedAt
	want := time.Date(2026, time.July, 30, 10, 0, 0, 0, time.UTC)
	if !createdAt.Equal(want) {
		t.Fatalf("created_at = %v, want %v", createdAt, want)
	}
	if !parsed.UpdatedAt.Equal(time.Date(2026, time.July, 30, 11, 0, 0, 0, time.UTC)) {
		t.Fatalf("updated_at = %v", parsed.UpdatedAt)
	}
}

// Never a `Map<string, unknown>` on the wire — Mongoose flattens the model's map in `toJSON`, so
// what a client receives is a plain object with per-integration configuration of no common shape.
// That is why the component is an open map rather than a named union.
func TestReadsIntegrationsAsAnOpenMap(t *testing.T) {
	integrations := project(t, "project-populated.json").Integrations
	if len(integrations) != 2 {
		t.Fatalf("integrations = %v", integrations)
	}
	// The generated element type is interface{}: the component spells its open map as
	// `additionalProperties: true`, which the generator renders as map[string]interface{}. The
	// empty-schema spelling would instead have produced map[string]*interface{}, needing a
	// dereference here — the two are equivalent to a validator but not to a code generator.
	value, ok := integrations["github"]
	if !ok {
		t.Fatalf("integrations is missing github: %v", integrations)
	}
	github, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("integrations[github] = %#v", value)
	}
	if enabled, _ := github["enabled"].(bool); !enabled {
		t.Fatalf("integrations[github].enabled = %v", github["enabled"])
	}
	if _, ok := integrations["serper"]; !ok {
		t.Fatalf("integrations is missing serper: %v", integrations)
	}
}

// The mapper supplies `created_at`, `updated_at`, `created_by` and `updated_by`, which is what makes
// this response deserializable at all: all four are required by the closed component, and the raw
// document has none of them.
func TestDeserializesAProjectStoredBeforeTheModelHadTimestampsOrProvenance(t *testing.T) {
	parsed := project(t, "project-legacy-normalized.json")
	want := time.Date(2017, time.November, 29, 3, 36, 28, 0, time.UTC)
	if !parsed.CreatedAt.Equal(want) {
		t.Fatalf("created_at = %v, want %v", parsed.CreatedAt, want)
	}
	if !parsed.UpdatedAt.Equal(parsed.CreatedAt) {
		t.Fatalf("updated_at = %v, want it to equal created_at", parsed.UpdatedAt)
	}
	if parsed.CreatedBy != "unknown" {
		t.Fatalf("created_by = %q", parsed.CreatedBy)
	}
	// Omitted rather than sent as null, so the generated fields stay nil instead of empty.
	if parsed.Integrations != nil {
		t.Fatalf("integrations = %v, want nil", parsed.Integrations)
	}
	if parsed.Description != nil {
		t.Fatalf("description = %v, want nil", parsed.Description)
	}
}
