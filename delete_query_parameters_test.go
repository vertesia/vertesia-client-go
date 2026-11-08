package vertesia

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/vertesia/vertesia-client-go/openapi"
)

// serveJSON answers every request with body and records the query string the client sent.
func serveJSON(t *testing.T, body string) (*openapi.APIClient, func() url.Values) {
	t.Helper()
	var mu sync.Mutex
	var query url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		query = r.URL.Query()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	cfg := openapi.NewConfiguration()
	cfg.Servers = openapi.ServerConfigurations{{URL: server.URL}}
	return openapi.NewAPIClient(cfg), func() url.Values {
		mu.Lock()
		defer mu.Unlock()
		return query
	}
}

// The generated client requires the header and only the test server reads it.
const testAPIVersion = "20260101"

const deletedAppSummary = `{"confirmed":true,"app_id":"app","app_name":"name","versions":0,"installations":0,"storage_prefix":"apps/name","deleted":true,"warnings":[]}`

// Without a declared `confirm` parameter a generated client could only ever dry-run: DeleteApp answered 200 with
// deleted:false and removed nothing, with no way to ask for more.
func TestDeleteAppCanConfirm(t *testing.T) {
	client, sent := serveJSON(t, deletedAppSummary)
	if _, _, err := client.AppsAPI.DeleteApp(context.Background(), "app").XApiVersion(testAPIVersion).Confirm(true).Execute(); err != nil {
		t.Fatal(err)
	}
	if got := sent().Get("confirm"); got != "true" {
		t.Fatalf("confirm = %q, want true", got)
	}
}

func TestDeleteAppPreviewsByDefault(t *testing.T) {
	client, sent := serveJSON(t, deletedAppSummary)
	if _, _, err := client.AppsAPI.DeleteApp(context.Background(), "app").XApiVersion(testAPIVersion).Execute(); err != nil {
		t.Fatal(err)
	}
	if sent().Has("confirm") {
		t.Fatal("an unconfirmed delete must not send confirm")
	}
}

func TestDeleteInteractionCanCascade(t *testing.T) {
	client, sent := serveJSON(t, `{"id":"interaction"}`)
	if _, _, err := client.InteractionsAPI.DeleteInteraction(context.Background(), "interaction").XApiVersion(testAPIVersion).Cascade(true).Execute(); err != nil {
		t.Fatal(err)
	}
	if got := sent().Get("cascade"); got != "true" {
		t.Fatalf("cascade = %q, want true", got)
	}
}
