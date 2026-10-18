package vertesia

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// FederationError exposes sanitized OAuth metadata and an inspectable underlying cause.
// Error never includes remote bodies or assertion-provider error text.
type FederationError struct {
	StatusCode int
	OAuthCode  string
	cause      error
}

func (e *FederationError) Error() string {
	if e.StatusCode != 0 {
		if e.OAuthCode != "" {
			return fmt.Sprintf("federation exchange failed: HTTP %d (%s)", e.StatusCode, e.OAuthCode)
		}
		return fmt.Sprintf("federation exchange failed: HTTP %d", e.StatusCode)
	}
	return "federation authentication failed"
}
func (e *FederationError) Unwrap() error    { return e.cause }
func (e *FederationError) GoString() string { return e.Error() }

func federationOAuthCode(body []byte) string {
	var value struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &value) != nil {
		return ""
	}
	switch value.Error {
	case "invalid_grant", "invalid_client", "invalid_scope", "unauthorized_client", "invalid_target", "invalid_request", "temporarily_unavailable":
		return value.Error
	}
	return ""
}

// FederationOptions selects an account-owned trust policy and one target project.
// Use one Client per user, project, resource and scope. Providers must return fresh assertions on renewal.
type FederationOptions struct {
	ConnectionID string
	ProjectID    string
	Resource     string
	Scope        string
	// SubjectTokenType defaults to urn:ietf:params:oauth:token-type:jwt.
	SubjectTokenType     string
	SubjectTokenProvider func(context.Context) (string, error)
	// TokenEndpoint optionally overrides the complete endpoint, including a tenant path.
	TokenEndpoint string
	// Optional client authentication uses client_secret_post or a freshly signed private_key_jwt.
	ClientID                string
	ClientSecret            string
	ClientSecretBasic       bool
	ClientAssertionProvider func(context.Context) (string, error)
}

// FederationToken is a one-shot exchange result. ExpiresIn is conservative remaining seconds.
// No refresh token is issued.
type FederationToken struct {
	AccessToken     string `json:"access_token"`
	TokenType       string `json:"token_type"`
	ExpiresIn       int    `json:"expires_in"`
	IssuedTokenType string `json:"issued_token_type"`
	Scope           string `json:"scope,omitempty"`
}

func federationEndpoint(opts ClientOptions, f FederationOptions) (string, error) {
	if f.ConnectionID == "" || f.ProjectID == "" || f.Resource == "" {
		return "", errors.New("federation requires connection ID, project ID and resource")
	}
	if (f.ClientSecretBasic && f.ClientSecret == "") || (f.ClientSecret != "" || f.ClientAssertionProvider != nil) != (f.ClientID != "") || (f.ClientSecret != "" && f.ClientAssertionProvider != nil) {
		return "", errors.New("federation client authentication requires a client ID and exactly one credential method")
	}
	endpoint := f.TokenEndpoint
	if endpoint == "" {
		endpoints, err := resolveClientEndpoints(opts)
		if err != nil {
			return "", err
		}
		if !endpoints.TokenServerURLExplicit && (!endpoints.TokenServerURLSafelyDerived || !hostedFederationSTS(endpoints.TokenServerURL)) {
			return "", errors.New("explicit federation TokenServerURL or TokenEndpoint required for custom endpoints")
		}
		endpoint = endpoints.TokenServerURL + "/oauth/token"
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) {
		return "", errors.New("federation token endpoint must be HTTPS (HTTP is allowed only on loopback) without credentials, query or fragment")
	}
	return endpoint, nil
}

func hostedFederationSTS(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return u.Scheme == "https" && (u.Port() == "" || u.Port() == "443") && (host == "sts.vertesia.io" || (strings.HasPrefix(host, "sts.") && strings.HasSuffix(host, ".vertesia.io")))
}

var federationBearerToken = regexp.MustCompile(`^[A-Za-z0-9._~+/-]+=*$`)

// ExchangeFederationToken exchanges one IdP assertion without caching or modifying client authentication.
func ExchangeFederationToken(ctx context.Context, opts ClientOptions, f FederationOptions, subjectToken string) (*FederationToken, error) {
	return exchangeFederationToken(ctx, opts, f, subjectToken, time.Now)
}

func exchangeFederationToken(ctx context.Context, opts ClientOptions, f FederationOptions, subjectToken string, clock func() time.Time) (*FederationToken, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	endpoint, err := federationEndpoint(opts, f)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(subjectToken) == "" {
		return nil, errors.New("empty IdP assertion")
	}
	tokenType := f.SubjectTokenType
	if tokenType == "" {
		tokenType = "urn:ietf:params:oauth:token-type:jwt"
	}
	values := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:token-exchange"}, "subject_token": {subjectToken}, "subject_token_type": {tokenType}, "federation_connection_id": {f.ConnectionID}, "project_id": {f.ProjectID}, "resource": {f.Resource}}
	if f.Scope != "" {
		values.Set("scope", f.Scope)
	}
	if f.ClientID != "" && !f.ClientSecretBasic {
		values.Set("client_id", f.ClientID)
	}
	if f.ClientSecret != "" && !f.ClientSecretBasic {
		values.Set("client_secret", f.ClientSecret)
	}
	if f.ClientAssertionProvider != nil {
		assertion, err := f.ClientAssertionProvider(ctx)
		if err != nil {
			return nil, &FederationError{cause: err}
		}
		if strings.TrimSpace(assertion) == "" {
			return nil, errors.New("empty client assertion")
		}
		values.Set("client_assertion", assertion)
		values.Set("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, errors.New("invalid federation request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if f.ClientSecretBasic {
		credential := url.QueryEscape(f.ClientID) + ":" + url.QueryEscape(f.ClientSecret)
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(credential)))
	}
	base := opts.HTTPClient
	if base == nil {
		base = http.DefaultClient
	}
	client := *base
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if client.Timeout == 0 {
		client.Timeout = 60 * time.Second
	}
	started := clock()
	response, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &FederationError{cause: err}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 65537))
		code := ""
		if len(body) <= 65536 {
			code = federationOAuthCode(body)
		}
		return nil, &FederationError{StatusCode: response.StatusCode, OAuthCode: code}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(body) > 65536 {
		return nil, errors.New("invalid federation response")
	}
	var wire struct {
		AccessToken     string  `json:"access_token"`
		TokenType       string  `json:"token_type"`
		ExpiresIn       float64 `json:"expires_in"`
		IssuedTokenType string  `json:"issued_token_type"`
		Scope           string  `json:"scope"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, errors.New("invalid federation token response")
	}
	result := FederationToken{AccessToken: wire.AccessToken, TokenType: wire.TokenType, IssuedTokenType: wire.IssuedTokenType, Scope: wire.Scope}
	if !federationBearerToken.MatchString(result.AccessToken) || !strings.EqualFold(result.TokenType, "Bearer") || result.IssuedTokenType != "urn:ietf:params:oauth:token-type:access_token" || wire.ExpiresIn < 1 || wire.ExpiresIn > 86400 {
		return nil, errors.New("invalid federation token response")
	}
	elapsed := clock().Sub(started)
	if elapsed < 0 {
		elapsed = 0
	}
	result.ExpiresIn = int(math.Floor(wire.ExpiresIn - elapsed.Seconds()))
	if result.ExpiresIn < 1 {
		return nil, errors.New("federation token expired during exchange")
	}
	result.TokenType = "Bearer"
	return &result, nil
}

type federationTokenSource struct {
	opts       ClientOptions
	federation FederationOptions
	mu         sync.Mutex
	inFlight   chan struct{}
	failure    error
	retryAt    time.Time
	token      string
	renewAt    time.Time
	now        func() time.Time
}

func (s *federationTokenSource) Token(ctx context.Context) (string, error) {
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		s.mu.Lock()
		if s.token != "" && s.now().Before(s.renewAt) {
			token := s.token
			s.mu.Unlock()
			return token, nil
		}
		if s.failure != nil && s.now().Before(s.retryAt) {
			failure := s.failure
			s.mu.Unlock()
			return "", failure
		}
		if done := s.inFlight; done != nil {
			s.mu.Unlock()
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-done:
				continue
			}
		}
		done := make(chan struct{})
		s.inFlight = done
		s.mu.Unlock()
		token, ttl, err := s.renew(ctx)
		s.mu.Lock()
		if err == nil {
			s.token = token
			lifetime := time.Duration(ttl) * time.Second
			s.renewAt = s.now().Add(lifetime - lifetime/10)
		}
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			s.failure = err
			s.retryAt = s.now().Add(time.Second)
		} else {
			s.failure = nil
		}
		s.inFlight = nil
		close(done)
		s.mu.Unlock()
		return token, err
	}
}

func (s *federationTokenSource) renew(ctx context.Context) (string, int, error) {
	assertion, err := s.federation.SubjectTokenProvider(ctx)
	if ctx.Err() != nil {
		return "", 0, ctx.Err()
	}
	if err != nil {
		return "", 0, &FederationError{cause: err}
	}
	result, err := exchangeFederationToken(ctx, s.opts, s.federation, assertion, s.now)
	if ctx.Err() != nil {
		return "", 0, ctx.Err()
	}
	if err != nil {
		return "", 0, err
	}
	return result.AccessToken, result.ExpiresIn, nil
}

// String redacts credentials when configuration or exchange results are formatted.
func (f FederationOptions) String() string   { return "FederationOptions{credentials redacted}" }
func (f FederationOptions) GoString() string { return f.String() }

// String redacts the bearer token.
func (f FederationToken) String() string {
	return fmt.Sprintf("FederationToken{access token redacted, expires_in:%d}", f.ExpiresIn)
}
func (f FederationToken) GoString() string { return f.String() }

// Invalidate drops only the token rejected by this API request; it never replays the request.
func (s *federationTokenSource) Invalidate(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token == token {
		s.token = ""
		s.failure = nil
	}
}
