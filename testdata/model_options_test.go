package openapi

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestModelOptionsConvenienceAPI(t *testing.T) {
	options := &TextFallbackOptions{}
	wrapped := TextFallbackOptionsAsModelOptions(options)
	if wrapped.GetActualInstance() != options || !reflect.DeepEqual(wrapped.GetActualInstanceValue(), *options) {
		t.Fatal("typed convenience API must preserve the wrapped options")
	}
}

func TestModelOptionsDiscriminatorCoverage(t *testing.T) {
	raw, err := os.ReadFile("../spec/vertesia-openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	type reference struct {
		Ref string `json:"$ref"`
	}
	type component struct {
		OneOf      []reference `json:"oneOf"`
		AnyOf      []reference `json:"anyOf"`
		Properties map[string]struct {
			Enum  []json.RawMessage `json:"enum"`
			Const json.RawMessage   `json:"const"`
		} `json:"properties"`
	}
	var spec struct {
		Components struct{ Schemas map[string]component }
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	union := spec.Components.Schemas["ModelOptions"]
	refs := append(union.OneOf, union.AnyOf...)
	mapping := make(map[string]string)
	for _, ref := range refs {
		member := spec.Components.Schemas[strings.TrimPrefix(ref.Ref, "#/components/schemas/")]
		var id string
		if value := member.Properties["_option_id"].Const; len(value) > 0 {
			if err := json.Unmarshal(value, &id); err != nil {
				t.Fatal(err)
			}
		}
		if values := member.Properties["_option_id"].Enum; len(values) == 1 {
			if err := json.Unmarshal(values[0], &id); err != nil {
				t.Fatal(err)
			}
		}
		if id == "" || mapping[id] != "" {
			t.Fatalf("missing or duplicate option ID in %s", ref.Ref)
		}
		mapping[id] = ref.Ref
	}
	if len(mapping) == 0 {
		t.Fatal("ModelOptions union is empty")
	}
	for id, ref := range mapping {
		t.Run(id, func(t *testing.T) {
			payload := map[string]any{"_option_id": id, "future_option": json.RawMessage(`{"large":9007199254740993,"value":null}`)}
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
			var preserved map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &preserved); err != nil {
				t.Fatal(err)
			}
			if string(preserved["future_option"]) != string(payload["future_option"].(json.RawMessage)) {
				t.Fatalf("lost extension or numeric precision: %s", encoded)
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

func TestModelOptionsReadEditSavePreservesExtensions(t *testing.T) {
	data := []byte(`{"_option_id":"openai-text","max_tokens":42,"temperature":0.5,"future_option":{"large":9007199254740993,"huge":1e1000,"values":[null,false,{},[]]}}`)
	var options ModelOptions
	if err := json.Unmarshal(data, &options); err != nil {
		t.Fatal(err)
	}
	branch := options.OpenAiTextOptions
	if branch == nil || len(branch.AdditionalProperties) != 1 {
		t.Fatal("expected typed options with only unknown fields retained")
	}
	data[0] = 'X'
	branch.SetMaxTokens(128)
	branch.Temperature = nil
	// Even manually supplied extensions cannot override typed values or undo clears.
	branch.AdditionalProperties["max_tokens"] = json.RawMessage(`999`)
	branch.AdditionalProperties["TEMPERATURE"] = json.RawMessage(`0.9`)
	rewrapped := OpenAiTextOptionsAsModelOptions(branch)
	encoded, err := json.Marshal(rewrapped)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["max_tokens"]) != "128" || fields["temperature"] != nil || fields["TEMPERATURE"] != nil {
		t.Fatalf("typed edit or clear lost: %s", encoded)
	}
	if string(fields["future_option"]) != `{"large":9007199254740993,"huge":1e1000,"values":[null,false,{},[]]}` {
		t.Fatalf("extension changed: %s", encoded)
	}
	delete(branch.AdditionalProperties, "future_option")
	mapped, err := branch.ToMap()
	if err != nil || mapped["future_option"] != nil {
		t.Fatalf("extension removal failed: %v, %v", mapped, err)
	}
}

func TestModelOptionsConcreteDecoderClearsExtensions(t *testing.T) {
	var options OpenAiTextOptions
	for _, body := range []string{
		`{"_option_id":"openai-text","MAX_TOKENS":42,"future":true}`,
		`{"_option_id":"openai-text","max_tokens":64}`,
	} {
		if err := json.Unmarshal([]byte(body), &options); err != nil {
			t.Fatal(err)
		}
		if options.AdditionalProperties["MAX_TOKENS"] != nil {
			t.Fatal("case-insensitive known field was also retained as an extension")
		}
	}
	if len(options.AdditionalProperties) != 0 || options.GetMaxTokens() != 64 {
		t.Fatal("reused typed destination retained stale data")
	}
	if err := json.Unmarshal([]byte(`{"_option_id":"openai-text","max_tokens":"bad","future":true}`), &options); err == nil {
		t.Fatal("extension support weakened typed validation")
	}
	if options.MaxTokens != nil || len(options.AdditionalProperties) != 0 {
		t.Fatal("failed decode retained previous values")
	}
}
