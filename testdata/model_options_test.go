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

func TestModelOptionsRejectsMalformedData(t *testing.T) {
	for _, body := range []string{`[]`, `123`, `"text"`, `{"_option_id":123}`, `{"_option_id":"vertexai-claude","max_tokens":"bad"}`} {
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
	if err := json.Unmarshal([]byte(`{"_option_id":123}`), &options); err == nil {
		t.Fatal("accepted non-string ID")
	}
	if options.GetActualInstance() != nil {
		t.Fatal("failed decode retained a previous branch")
	}
}

func TestModelOptionsPreservesUntaggedAndUnknownOptions(t *testing.T) {
	for _, body := range []string{
		`{}`, `null`,
		`{"temperature":0.2,"max_tokens":1024}`,
		`{"_option_id":"future-provider","nested":{"values":[1,false,null]},"large":9007199254740993}`,
		`{"_option_id":"","max_tokens":42}`,
		`{"_option_id":null,"max_tokens":42}`,
	} {
		t.Run(body, func(t *testing.T) {
			var options ModelOptions
			data := []byte(body)
			if err := json.Unmarshal(data, &options); err != nil {
				t.Fatal(err)
			}
			if string(options.Raw) != body {
				t.Fatalf("lost raw payload: %s", options.Raw)
			}
			for _, actual := range []any{options.GetActualInstance(), options.GetActualInstanceValue()} {
				raw, ok := actual.(json.RawMessage)
				if !ok || string(raw) != body {
					t.Fatalf("expected raw options, got %#v", actual)
				}
			}
			// UnmarshalJSON must own its bytes, just like json.RawMessage.
			data[0] = 'X'
			encoded, err := json.Marshal(options)
			if err != nil {
				t.Fatal(err)
			}
			if string(encoded) != body {
				t.Fatalf("round trip changed payload: %s", encoded)
			}
		})
	}
}

func TestModelOptionsSwitchesBetweenTypedAndRaw(t *testing.T) {
	var options ModelOptions
	for _, body := range []string{
		`{"_option_id":"vertexai-claude","max_tokens":42}`,
		`{"max_tokens":42}`,
		`{"_option_id":"openai-text","max_tokens":42}`,
		`{"_option_id":"future-provider","extra":true}`,
	} {
		if err := json.Unmarshal([]byte(body), &options); err != nil {
			t.Fatal(err)
		}
		var tag struct {
			ID string `json:"_option_id"`
		}
		if err := json.Unmarshal([]byte(body), &tag); err != nil {
			t.Fatal(err)
		}
		raw := tag.ID == "" || tag.ID == "future-provider"
		if raw {
			if options.VertexAIClaudeOptions != nil || options.OpenAiTextOptions != nil || options.Raw == nil {
				t.Fatal("raw decode retained typed branch")
			}
		} else if options.Raw != nil {
			t.Fatal("typed decode retained raw data")
		}
	}
	if err := json.Unmarshal([]byte(`{"_option_id":12}`), &options); err == nil {
		t.Fatal("accepted invalid ID type")
	}
	if options.Raw != nil || options.GetActualInstance() != nil {
		t.Fatal("failed decode retained raw data")
	}
}
