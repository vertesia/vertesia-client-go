package vertesia

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vertesia/vertesia-client-go/openapi"
)

func TestResolveClientEndpoints(t *testing.T) {
	tests := []struct {
		name      string
		opts      ClientOptions
		studioURL string
		storeURL  string
		tokenURL  string
	}{
		{
			name:      "default",
			opts:      ClientOptions{},
			studioURL: "https://api.vertesia.io/api/v1",
			storeURL:  "https://api.vertesia.io/api/v1",
			tokenURL:  "https://sts.vertesia.io",
		},
		{
			name:      "global preview",
			opts:      ClientOptions{Preview: true},
			studioURL: "https://api-preview.vertesia.io/api/v1",
			storeURL:  "https://api-preview.vertesia.io/api/v1",
			tokenURL:  "https://sts.vertesia.io",
		},
		{
			name:      "regional",
			opts:      ClientOptions{Region: "eu1"},
			studioURL: "https://api.eu1.vertesia.io/api/v1",
			storeURL:  "https://api.eu1.vertesia.io/api/v1",
			tokenURL:  "https://sts.eu1.vertesia.io",
		},
		{
			name:      "regional preview",
			opts:      ClientOptions{Region: "us1", Preview: true},
			studioURL: "https://api-preview.us1.vertesia.io/api/v1",
			storeURL:  "https://api-preview.us1.vertesia.io/api/v1",
			tokenURL:  "https://sts.us1.vertesia.io",
		},
		{
			name:      "explicit site",
			opts:      ClientOptions{Site: "api.us1.vertesia.io"},
			studioURL: "https://api.us1.vertesia.io/api/v1",
			storeURL:  "https://api.us1.vertesia.io/api/v1",
			tokenURL:  "https://sts.us1.vertesia.io",
		},
		{
			name: "local split",
			opts: ClientOptions{
				ServerURL:      "http://localhost:8091",
				StoreURL:       "http://localhost:8092/",
				TokenServerURL: "http://localhost:8093/",
			},
			studioURL: "http://localhost:8091/api/v1",
			storeURL:  "http://localhost:8092/api/v1",
			tokenURL:  "http://localhost:8093",
		},
		{
			name: "already normalized",
			opts: ClientOptions{
				ServerURL:      "https://api.dev1.vertesia.io/api/v1",
				StoreURL:       "https://api.dev1.vertesia.io/api/v1",
				TokenServerURL: "https://sts.dev1.vertesia.io",
			},
			studioURL: "https://api.dev1.vertesia.io/api/v1",
			storeURL:  "https://api.dev1.vertesia.io/api/v1",
			tokenURL:  "https://sts.dev1.vertesia.io",
		},
		{
			name: "custom host fallback",
			opts: ClientOptions{
				ServerURL: "https://studio-server-dev-main.api.dev1.vertesia.io",
				StoreURL:  "https://zeno-server-dev-main.api.dev1.vertesia.io",
			},
			studioURL: "https://studio-server-dev-main.api.dev1.vertesia.io/api/v1",
			storeURL:  "https://zeno-server-dev-main.api.dev1.vertesia.io/api/v1",
			tokenURL:  "https://sts.vertesia.io",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			endpoints, err := resolveClientEndpoints(test.opts)
			if err != nil {
				t.Fatalf("resolveClientEndpoints failed: %v", err)
			}
			if endpoints.StudioURL != test.studioURL {
				t.Fatalf("StudioURL = %q, want %q", endpoints.StudioURL, test.studioURL)
			}
			if endpoints.StoreURL != test.storeURL {
				t.Fatalf("StoreURL = %q, want %q", endpoints.StoreURL, test.storeURL)
			}
			if endpoints.TokenServerURL != test.tokenURL {
				t.Fatalf("TokenServerURL = %q, want %q", endpoints.TokenServerURL, test.tokenURL)
			}
		})
	}
}

func TestNewClientRequiresCompleteSplitURLs(t *testing.T) {
	if _, err := NewClient(ClientOptions{ServerURL: "http://localhost:8091"}); err == nil {
		t.Fatal("NewClient succeeded with only ServerURL")
	}
	if _, err := NewClient(ClientOptions{StoreURL: "http://localhost:8092"}); err == nil {
		t.Fatal("NewClient succeeded with only StoreURL")
	}
}

func TestNewClientRejectsAmbiguousEndpointOptions(t *testing.T) {
	if _, err := NewClient(ClientOptions{Site: "api.us1.vertesia.io", Region: "us1"}); err == nil {
		t.Fatal("NewClient succeeded with both Site and Region")
	}
	if _, err := NewClient(ClientOptions{Region: "api.us1.vertesia.io"}); err == nil {
		t.Fatal("NewClient succeeded with host-shaped Region")
	}
}

func TestNewClientRejectsInvalidAuthOptions(t *testing.T) {
	if _, err := NewClient(ClientOptions{APIKey: "not-secret"}); err == nil {
		t.Fatal("NewClient succeeded with unsupported API key")
	}
	if _, err := NewClient(ClientOptions{APIKey: "sk-test", Token: "token"}); err == nil {
		t.Fatal("NewClient succeeded with both APIKey and Token")
	}
}

func TestNewClientRequiresTokenServerForSecretKeyCustomSplitURLs(t *testing.T) {
	_, err := NewClient(ClientOptions{
		ServerURL: "https://studio-server-dev-main.example.com",
		StoreURL:  "https://zeno-server-dev-main.example.com",
		APIKey:    "sk-test",
	})
	if err == nil {
		t.Fatal("NewClient succeeded with APIKey and custom endpoints without TokenServerURL")
	}
	if !errors.Is(err, ErrInvalidClientOptions) {
		t.Fatalf("NewClient error = %v, want ErrInvalidClientOptions", err)
	}
	if !strings.Contains(err.Error(), "TokenServerURL is required") {
		t.Fatalf("NewClient error = %q, want TokenServerURL guidance", err.Error())
	}
}

func TestNewClientSetsVersionAndAliases(t *testing.T) {
	stsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("STS should not be called for direct bearer tokens")
	}))
	defer stsServer.Close()

	seenStudio := false
	seenStore := false
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-version"); got != "20260101" {
			t.Fatalf("x-api-version = %q", got)
		}
		switch r.URL.Path {
		case "/api/v1/account":
			seenStudio = true
		case "/api/v1/objects/search":
			seenStore = true
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		writeNullJSON(w)
	}))
	defer apiServer.Close()

	client, err := NewClient(ClientOptions{
		ServerURL:      apiServer.URL,
		StoreURL:       apiServer.URL,
		TokenServerURL: stsServer.URL,
		Token:          "token",
		APIVersion:     "20260101",
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if client.AccountsAPI != client.Studio.AccountsAPI {
		t.Fatal("AccountsAPI alias does not point at Studio")
	}
	if client.ObjectsAPI != client.Store.ObjectsAPI {
		t.Fatal("ObjectsAPI alias does not point at Store")
	}

	if _, _, err := client.AccountsAPI.GetCurrentAccount(context.Background()).Execute(); err != nil {
		t.Fatalf("GetCurrentAccount failed: %v", err)
	}
	payload := openapi.NewComplexSearchPayload()
	payload.SetLimit(1)
	if _, _, err := client.ObjectsAPI.SearchObjects(context.Background()).ComplexSearchPayload(*payload).Execute(); err != nil {
		t.Fatalf("SearchObjects failed: %v", err)
	}
	if !seenStudio || !seenStore {
		t.Fatalf("did not exercise studio/store aliases: studio=%v store=%v", seenStudio, seenStore)
	}
}

func TestNewClientUsesDirectBearerToken(t *testing.T) {
	var stsCalls int
	stsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stsCalls++
		t.Fatalf("STS should not be called for direct bearer tokens")
	}))
	defer stsServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/account" {
			t.Fatalf("path = %q, want /api/v1/account", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer direct-token" {
			t.Fatalf("Authorization = %q", got)
		}
		writeNullJSON(w)
	}))
	defer apiServer.Close()

	client, err := NewClient(ClientOptions{
		ServerURL:      apiServer.URL,
		StoreURL:       apiServer.URL,
		TokenServerURL: stsServer.URL,
		Token:          "direct-token",
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if _, _, err := client.Studio.AccountsAPI.GetCurrentAccount(context.Background()).Execute(); err != nil {
		t.Fatalf("GetCurrentAccount failed: %v", err)
	}
	if stsCalls != 0 {
		t.Fatalf("STS calls = %d, want 0", stsCalls)
	}
}

func TestNewClientExchangesSecretKeyForGeneratedClients(t *testing.T) {
	issuedToken := makeTestJWT(time.Now().Add(time.Hour))
	var stsCalls int

	stsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stsCalls++
		if r.URL.Path != "/token/issue" {
			t.Fatalf("STS path = %q, want /token/issue", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("STS Authorization = %q", got)
		}
		if got := r.Header.Get("x-api-version"); got != "20260101" {
			t.Fatalf("STS x-api-version = %q", got)
		}
		writeIssueTokenResponse(t, w, issuedToken, 3600)
	}))
	defer stsServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+issuedToken {
			t.Fatalf("API Authorization = %q", got)
		}
		writeNullJSON(w)
	}))
	defer apiServer.Close()

	client, err := NewClient(ClientOptions{
		ServerURL:      apiServer.URL,
		StoreURL:       apiServer.URL,
		TokenServerURL: stsServer.URL,
		APIKey:         "sk-test",
		APIVersion:     "20260101",
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if _, _, err := client.Studio.AccountsAPI.GetCurrentAccount(context.Background()).Execute(); err != nil {
		t.Fatalf("studio generated request failed: %v", err)
	}
	if _, _, err := client.Store.AccountsAPI.GetCurrentAccount(context.Background()).Execute(); err != nil {
		t.Fatalf("store generated request failed: %v", err)
	}
	if stsCalls != 1 {
		t.Fatalf("STS calls = %d, want 1", stsCalls)
	}
}

func TestNewClientRefreshesNearExpiredSecretKeyToken(t *testing.T) {
	tokens := []string{
		makeTestJWT(time.Now().Add(30 * time.Second)),
		makeTestJWT(time.Now().Add(time.Hour)),
	}
	var stsCalls int

	stsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if stsCalls >= len(tokens) {
			t.Fatalf("unexpected extra STS call")
		}
		writeIssueTokenResponse(t, w, tokens[stsCalls], 3600)
		stsCalls++
	}))
	defer stsServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeNullJSON(w)
	}))
	defer apiServer.Close()

	client, err := NewClient(ClientOptions{
		ServerURL:      apiServer.URL,
		StoreURL:       apiServer.URL,
		TokenServerURL: stsServer.URL,
		APIKey:         "sk-test",
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	for i := 0; i < 2; i++ {
		if _, _, err := client.Studio.AccountsAPI.GetCurrentAccount(context.Background()).Execute(); err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
	}
	if stsCalls != 2 {
		t.Fatalf("STS calls = %d, want 2", stsCalls)
	}
}

func TestNewClientCoalescesConcurrentSecretKeyRefresh(t *testing.T) {
	issuedToken := makeTestJWT(time.Now().Add(time.Hour))
	var mu sync.Mutex
	stsCalls := 0

	stsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(25 * time.Millisecond)
		mu.Lock()
		stsCalls++
		mu.Unlock()
		writeIssueTokenResponse(t, w, issuedToken, 3600)
	}))
	defer stsServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeNullJSON(w)
	}))
	defer apiServer.Close()

	client, err := NewClient(ClientOptions{
		ServerURL:      apiServer.URL,
		StoreURL:       apiServer.URL,
		TokenServerURL: stsServer.URL,
		APIKey:         "sk-test",
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := client.Studio.AccountsAPI.GetCurrentAccount(context.Background()).Execute()
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("generated request failed: %v", err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if stsCalls != 1 {
		t.Fatalf("STS calls = %d, want 1", stsCalls)
	}
}

func makeTestJWT(exp time.Time) string {
	encode := func(value any) string {
		data, err := json.Marshal(value)
		if err != nil {
			panic(err)
		}
		return base64.RawURLEncoding.EncodeToString(data)
	}
	return fmt.Sprintf("%s.%s.signature",
		encode(map[string]string{"alg": "none", "typ": "JWT"}),
		encode(map[string]int64{"exp": exp.Unix()}),
	)
}

func writeIssueTokenResponse(t *testing.T, w http.ResponseWriter, token string, expiresIn int) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	payload := fmt.Sprintf(`{"token":%q,"token_type":"Bearer","expires_in":%d}`, token, expiresIn)
	if _, err := w.Write([]byte(payload)); err != nil && !strings.Contains(err.Error(), "connection") {
		t.Fatalf("write response failed: %v", err)
	}
}

func writeNullJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte("null"))
}
