package vertesia

import (
	"encoding/json"
	"testing"

	"github.com/vertesia/vertesia-client-go/openapi"
)

const accountKeyMetadata = `{"id":"key","name":"IdP","account":"account","scope":"account","profile":"account_admin_v1","enabled":true,"maskedValue":"sk-hidden","created_by":"user:admin","updated_by":"user:admin","created_at":"2026-09-15T00:00:00Z","updated_at":"2026-09-15T00:00:00Z"}`

func TestAccountKeyNeedsNoProject(t *testing.T) {
	var key openapi.AccountApiKey
	if err := json.Unmarshal([]byte(accountKeyMetadata), &key); err != nil {
		t.Fatal(err)
	}
	if key.GetName() != "IdP" {
		t.Fatal("unexpected name")
	}
	var created openapi.AccountApiKeyWithValue
	if err := json.Unmarshal([]byte(accountKeyMetadata), &created); err == nil {
		t.Fatal("creation must require a secret")
	}
	if err := json.Unmarshal([]byte(`{"id":"key"}`), &key); err == nil {
		t.Fatal("incomplete metadata accepted")
	}
}
