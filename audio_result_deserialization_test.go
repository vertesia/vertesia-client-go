package vertesia

import (
	"encoding/json"
	"testing"

	"github.com/vertesia/vertesia-client-go/openapi"
)

func TestAudioResultCompletionRoundTrip(t *testing.T) {
	body := `{"type":"audio","value":"gs://bucket/speech.wav","mime_type":"audio/wav","container":"wav"}`
	var result openapi.CompletionResult
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatal(err)
	}
	if result.AudioResult == nil || result.AudioResult.MimeType != "audio/wav" {
		t.Fatalf("audio metadata lost: %#v", result.GetActualInstance())
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var again openapi.CompletionResult
	if err := json.Unmarshal(encoded, &again); err != nil {
		t.Fatal(err)
	}
	if again.AudioResult == nil {
		t.Fatal("audio branch lost during round trip")
	}
}

func TestAudioResultRequiresMimeType(t *testing.T) {
	var audio openapi.AudioResult
	if err := json.Unmarshal([]byte(`{"type":"audio","value":"gs://bucket/speech.wav"}`), &audio); err == nil {
		t.Fatal("accepted audio without mime_type")
	}
}

// A malformed audio value must not fall through to a different result branch.
func TestCompletionResultRejectsInvalidAudio(t *testing.T) {
	for _, body := range []string{
		`{"type":"audio","value":"gs://bucket/speech.wav"}`,
		`{"type":"audio","mime_type":"audio/wav"}`,
		`{"type":"audio","value":[],"mime_type":"audio/wav"}`,
	} {
		var result openapi.CompletionResult
		if err := json.Unmarshal([]byte(body), &result); err == nil {
			t.Fatalf("accepted invalid audio: %s", body)
		}
		if result.GetActualInstance() != nil {
			t.Fatalf("invalid audio selected another branch: %s", body)
		}
	}
}
