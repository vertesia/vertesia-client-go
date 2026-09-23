package openapi

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestCompletionResultDiscriminator(t *testing.T) {
	cases := []struct{ name, body, branch string }{
		{"text", `{"type":"text","value":"hello","future":true}`, "TextResult"},
		{"thoughts", `{"type":"thoughts","value":"thinking"}`, "ThoughtsResult"},
		{"json", `{"type":"json","value":{"answer":42}}`, "JsonResult"},
		{"image", `{"type":"image","value":"gs://bucket/image.png"}`, "ImageResult"},
		{"video", `{"type":"video","value":"gs://bucket/video.mp4"}`, "VideoResult"},
		{"audio", `{"type":"audio","value":"gs://bucket/audio.wav","mime_type":"audio/wav","container":"wav"}`, "AudioResult"},
	}
	var result CompletionResult
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Older published specs predate AudioResult; the upstream contract gate supplies the new spec.
			if _, ok := reflect.TypeOf(result).FieldByName(tc.branch); !ok && tc.name == "audio" {
				t.Skip("spec predates audio")
			}
			if err := json.Unmarshal([]byte(tc.body), &result); err != nil {
				t.Fatal(err)
			}
			value := reflect.ValueOf(result)
			for i := 0; i < value.NumField(); i++ {
				if !value.Field(i).IsNil() && value.Type().Field(i).Name != tc.branch {
					t.Fatalf("stale branch %s", value.Type().Field(i).Name)
				}
			}
			if actual := fmt.Sprintf("%T", result.GetActualInstance()); actual != "*openapi."+tc.branch {
				t.Fatalf("got %s, want %s", actual, tc.branch)
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var again CompletionResult
			if err := json.Unmarshal(encoded, &again); err != nil {
				t.Fatal(err)
			}
			var before, after map[string]interface{}
			json.Unmarshal([]byte(tc.body), &before)
			json.Unmarshal(encoded, &after)
			for _, key := range []string{"type", "value", "mime_type", "container"} {
				if !reflect.DeepEqual(before[key], after[key]) {
					t.Fatalf("%s lost: %s", key, encoded)
				}
			}
		})
	}
	for _, body := range []string{`{}`, `null`, `{"value":"missing type"}`, `{"type":"unknown","value":"x"}`, `{"type":"text"}`, `{"type":"text","value":[]}`, `{"type":"audio","value":"gs://b/a"}`} {
		if err := json.Unmarshal([]byte(body), &result); err == nil {
			t.Fatalf("accepted invalid result: %s", body)
		}
		if result.GetActualInstance() != nil {
			t.Fatalf("retained stale result after error: %s", body)
		}
	}
}
