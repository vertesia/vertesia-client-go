package openapi

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestModelOptionsDiscriminatorCoverage(t *testing.T) {
	raw, err := os.ReadFile("../spec/vertesia-openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Components struct {
			Schemas map[string]struct {
				Discriminator struct{ Mapping map[string]string }
			}
		}
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	mapping := spec.Components.Schemas["ModelOptions"].Discriminator.Mapping
	if len(mapping) == 0 {
		t.Fatal("ModelOptions discriminator mapping is missing")
	}
	for id, ref := range mapping {
		t.Run(id, func(t *testing.T) {
			payload := map[string]any{"_option_id": id}
			if id == "groq-deepseek-thinking" {
				payload["reasoning_format"] = "parsed"
			}
			if id == "bedrock-nova-canvas" {
				payload["taskType"] = "TEXT_IMAGE"
			}
			data, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			var options ModelOptions
			if err := json.Unmarshal(data, &options); err != nil {
				t.Fatal(err)
			}
			actual := options.GetActualInstance()
			if actual == nil {
				t.Fatal("decoded without selecting a branch")
			}
			want := strings.TrimPrefix(ref, "#/components/schemas/")
			if got := reflect.TypeOf(actual).Elem().Name(); got != want {
				t.Fatalf("decoded %s as %s, want %s", id, got, want)
			}
			encoded, err := json.Marshal(options)
			if err != nil {
				t.Fatal(err)
			}
			var roundTrip map[string]any
			if err := json.Unmarshal(encoded, &roundTrip); err != nil {
				t.Fatal(err)
			}
			if roundTrip["_option_id"] != id {
				t.Fatalf("lost discriminator: %s", encoded)
			}
		})
	}
}

func TestModelOptionsRejectsInvalidDiscriminators(t *testing.T) {
	for _, body := range []string{`{}`, `null`, `{"_option_id":"unknown"}`, `{"_option_id":123}`, `{"_option_id":"vertexai-claude","max_tokens":"bad"}`} {
		var options ModelOptions
		if err := json.Unmarshal([]byte(body), &options); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
}

func TestModelOptionsReusedDestination(t *testing.T) {
	var options ModelOptions
	for _, id := range []string{"vertexai-claude", "openai-text"} {
		if err := json.Unmarshal([]byte(`{"_option_id":"`+id+`","max_tokens":42,"future_field":true}`), &options); err != nil {
			t.Fatal(err)
		}
	}
	if options.VertexAIClaudeOptions != nil || options.OpenAiTextOptions == nil {
		t.Fatal("reusing the destination retained the previous branch")
	}
	if err := json.Unmarshal([]byte(`{"_option_id":"unknown"}`), &options); err == nil {
		t.Fatal("accepted unknown ID")
	}
	if options.GetActualInstance() != nil {
		t.Fatal("failed decode retained a previous branch")
	}
}
