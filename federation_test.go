package vertesia

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const federationResponse = `{"access_token":"delegated","token_type":"Bearer","expires_in":30,"issued_token_type":"urn:ietf:params:oauth:token-type:access_token"}`

func TestFederationRenewalAndRouting(t *testing.T) {
	var exchanges, providers, apiCalls atomic.Int32
	fail := false
	var captured url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			exchanges.Add(1)
			if r.Header.Get("Authorization") != "" {
				t.Error("STS inherited bearer auth")
			}
			_ = r.ParseForm()
			captured = r.PostForm
			if fail {
				w.WriteHeader(401)
				fmt.Fprint(w, "secret-assertion")
				return
			}
			fmt.Fprint(w, federationResponse)
		} else {
			apiCalls.Add(1)
			if r.Header.Get("Authorization") != "Bearer delegated" {
				t.Error("missing delegated bearer")
			}
			fmt.Fprint(w, "[]")
		}
	}))
	defer server.Close()
	f := FederationOptions{ConnectionID: "connection", ProjectID: "project", Resource: server.URL, Scope: "content:read", ClientID: "middleware", ClientSecret: "a+b%secret", SubjectTokenProvider: func(context.Context) (string, error) { providers.Add(1); return "fresh+assertion", nil }}
	opts := ClientOptions{ServerURL: server.URL, StoreURL: server.URL, TokenServerURL: server.URL, Federation: &f}
	endpoints, _ := resolveClientEndpoints(opts)
	source, err := newBearerTokenSource(opts, endpoints, defaultAPIVersion)
	if err != nil {
		t.Fatal(err)
	}
	s := source.(*federationTokenSource)
	now := time.Now()
	s.now = func() time.Time { return now }
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if token, err := s.Token(context.Background()); err != nil || token != "delegated" {
				t.Error(token, err)
			}
		}()
	}
	wg.Wait()
	if exchanges.Load() != 1 || providers.Load() != 1 {
		t.Fatal("concurrent requests did not share exchange")
	}
	if captured.Get("subject_token") != "fresh+assertion" || captured.Get("client_secret") != "a+b%secret" || captured.Get("project_id") != "project" || captured.Get("scope") != "content:read" {
		t.Fatal("incorrect form encoding", captured)
	}
	now = now.Add(28 * time.Second)
	if _, err = s.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if exchanges.Load() != 2 || providers.Load() != 2 {
		t.Fatal("did not renew")
	}
	now = now.Add(31 * time.Second)
	fail = true
	if token, err := s.Token(context.Background()); err == nil || token != "" || strings.Contains(err.Error(), "secret-assertion") {
		t.Fatal("failed renewal leaked or reused token")
	}
	fail = false
	client, err := NewClient(opts)
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.Studio.GetConfig().HTTPClient.Get(server.URL + "/api/v1/projects")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	res, err = client.Store.GetConfig().HTTPClient.Get(server.URL + "/api/v1/collections")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if apiCalls.Load() != 2 || exchanges.Load() != 4 {
		t.Fatal("client routing/cache failed")
	}
}

func TestFederationRejectsAdverseResponses(t *testing.T) {
	for _, body := range []string{`[]`, `{}`, `not-json`, strings.Replace(federationResponse, `"expires_in":30`, `"expires_in":0`, 1), strings.Replace(federationResponse, `"expires_in":30`, `"expires_in":301`, 1), strings.Replace(federationResponse, `"expires_in":30`, `"expires_in":"30"`, 1), strings.Replace(federationResponse, `"Bearer"`, `"Other"`, 1), strings.Replace(federationResponse, `"delegated"`, `""`, 1), strings.Repeat("x", 65537)} {
		t.Run(fmt.Sprint(len(body), body[:1]), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			_, err := ExchangeFederationToken(context.Background(), ClientOptions{}, FederationOptions{ConnectionID: "c", ProjectID: "p", Resource: "api", TokenEndpoint: server.URL + "/tenant/token"}, "sensitive")
			if err == nil || strings.Contains(err.Error(), "sensitive") {
				t.Fatal("accepted invalid response")
			}
		})
	}
}

func TestFederationRedirectAndProviderErrors(t *testing.T) {
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls.Add(1) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", target.URL)
		w.WriteHeader(307)
	}))
	defer redirect.Close()
	f := FederationOptions{ConnectionID: "c", ProjectID: "p", Resource: "r", TokenEndpoint: redirect.URL}
	if _, err := ExchangeFederationToken(context.Background(), ClientOptions{}, f, "secret"); err == nil || targetCalls.Load() != 0 {
		t.Fatal("followed credential redirect")
	}
	f.SubjectTokenProvider = func(context.Context) (string, error) { return "", errors.New("secret") }
	clientSource := &federationTokenSource{opts: ClientOptions{}, federation: f, now: time.Now}
	if _, err := clientSource.Token(context.Background()); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("leaked provider failure")
	}
	f.ClientID = "middleware"
	f.ClientSecret = "secret"
	f.ClientAssertionProvider = func(context.Context) (string, error) { return "proof", nil }
	if _, err := ExchangeFederationToken(context.Background(), ClientOptions{}, f, "secret"); err == nil {
		t.Fatal("accepted mixed credentials")
	}
	f.ClientSecret = ""
	f.ClientAssertionProvider = nil
	f.ClientID = ""
	f.TokenEndpoint = "http://untrusted.example/token"
	if _, err := ExchangeFederationToken(context.Background(), ClientOptions{}, f, "secret"); err == nil {
		t.Fatal("accepted insecure remote STS")
	}
	if _, err := NewClient(ClientOptions{Token: "token", Federation: &f}); err == nil {
		t.Fatal("accepted mixed auth")
	}
}

func TestFederationSignedClientProofIsFresh(t *testing.T) {
	var n atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("client_assertion") != fmt.Sprint(n.Load()) || r.PostForm.Get("client_secret") != "" {
			t.Error("bad proof")
		}
		fmt.Fprint(w, federationResponse)
	}))
	defer server.Close()
	f := FederationOptions{ConnectionID: "c", ProjectID: "p", Resource: "r", TokenEndpoint: server.URL + "/tenant/token", ClientID: "m", ClientAssertionProvider: func(context.Context) (string, error) { return fmt.Sprint(n.Add(1)), nil }}
	for i := 0; i < 2; i++ {
		if _, err := ExchangeFederationToken(context.Background(), ClientOptions{}, f, "idp"); err != nil {
			t.Fatal(err)
		}
	}
	if n.Load() != 2 {
		t.Fatal("reused signed proof")
	}
}

func TestFederationBasicEncodingAndExpiryDuringExchange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, secret, ok := r.BasicAuth()
		if !ok || id != "client%2Bid" || secret != "a%2Bb%25%3Asecret" {
			t.Error("incorrect form-encoded Basic credentials")
		}
		_ = r.ParseForm()
		if r.PostForm.Get("client_secret") != "" || r.PostForm.Get("client_id") != "" {
			t.Error("duplicated Basic credentials in body")
		}
		fmt.Fprint(w, federationResponse)
	}))
	defer server.Close()
	now := time.Now()
	f := FederationOptions{ConnectionID: "c", ProjectID: "p", Resource: "r", TokenEndpoint: server.URL, ClientID: "client+id", ClientSecret: "a+b%:secret", ClientSecretBasic: true}
	if _, err := ExchangeFederationToken(context.Background(), ClientOptions{Token: "must-not-forward"}, f, "idp"); err != nil {
		t.Fatal(err)
	}
	f.SubjectTokenProvider = func(context.Context) (string, error) { now = now.Add(time.Minute); return "idp", nil }
	s := &federationTokenSource{opts: ClientOptions{}, federation: f, now: func() time.Time { return now }}
	if _, err := s.Token(context.Background()); err == nil {
		t.Fatal("accepted token whose lifetime elapsed during exchange")
	}
	if strings.Contains(fmt.Sprintf("%#v", f), "a+b%:secret") {
		t.Fatal("credentials exposed by formatting")
	}
}
