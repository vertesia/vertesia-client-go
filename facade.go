package vertesia

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/vertesia/vertesia-client-go/openapi"
)

const (
	defaultSite         = "api.vertesia.io"
	defaultTokenURL     = "https://sts.vertesia.io"
	defaultAPIVersion   = "20260319"
	tokenRefreshWindow  = time.Minute
	tokenIssueOperation = "TokenServiceAPIService.IssueToken"
)

var (
	ErrInvalidClientOptions = errors.New("invalid Vertesia client options")
	ErrUnsupportedAPIKey    = errors.New("unsupported Vertesia API key")
)

// ClientOptions configures the high-level Vertesia client.
type ClientOptions struct {
	// Region selects a hosted Vertesia region, for example us1 or eu1.
	Region string
	// Preview selects the preview API host for the configured region or global site.
	Preview bool
	// Site overrides the Vertesia API host without a scheme, for example api.vertesia.io or api.us1.vertesia.io.
	Site string
	// ServerURL overrides the Studio API base URL.
	ServerURL string
	// StoreURL overrides the Store/Zeno API base URL.
	StoreURL string
	// TokenServerURL overrides the STS base URL.
	TokenServerURL string
	// APIKey is an sk- secret key. It is exchanged for a short-lived token through STS.
	APIKey string
	// Token is an already-issued bearer token.
	Token string
	// HTTPClient is the base HTTP client used by generated clients.
	HTTPClient *http.Client
	// APIVersion sets the x-api-version header. Defaults to the current stable version.
	APIVersion string
}

// Client is the hand-written Vertesia client facade over the generated OpenAPI client.
type Client struct {
	Studio *openapi.APIClient
	Store  *openapi.APIClient

	StudioURL      string
	StoreURL       string
	TokenServerURL string

	APIKeysAPI              *openapi.APIKeysAPIService
	AccessControlEntriesAPI *openapi.AccessControlEntriesAPIService
	AccountsAPI             *openapi.AccountsAPIService
	AgentRunsAPI            *openapi.AgentRunsAPIService
	AppsAPI                 *openapi.AppsAPIService
	AuditTrailAPI           *openapi.AuditTrailAPIService
	BulkOperationsAPI       *openapi.BulkOperationsAPIService
	CollectionsAPI          *openapi.CollectionsAPIService
	CommandsAPI             *openapi.CommandsAPIService
	ContentObjectTypesAPI   *openapi.ContentObjectTypesAPIService
	CostsAPI                *openapi.CostsAPIService
	DataAPI                 *openapi.DataAPIService
	EnvironmentsAPI         *openapi.EnvironmentsAPIService
	FilesAPI                *openapi.FilesAPIService
	InteractionRunsAPI      *openapi.InteractionRunsAPIService
	InteractionsAPI         *openapi.InteractionsAPIService
	OAuthClientsAPI         *openapi.OAuthClientsAPIService
	OAuthGrantsAPI          *openapi.OAuthGrantsAPIService
	OAuthProvidersAPI       *openapi.OAuthProvidersAPIService
	ObjectsAPI              *openapi.ObjectsAPIService
	ProcessesAPI            *openapi.ProcessesAPIService
	ProjectsAPI             *openapi.ProjectsAPIService
	PromptTemplatesAPI      *openapi.PromptTemplatesAPIService
	RemoteMCPConnectionsAPI *openapi.RemoteMCPConnectionsAPIService
	RenderingAPI            *openapi.RenderingAPIService
	RolesAPI                *openapi.RolesAPIService
	TasksAPI                *openapi.TasksAPIService
	TokenServiceAPI         *openapi.TokenServiceAPIService
	UserGroupsAPI           *openapi.UserGroupsAPIService
	UsersAPI                *openapi.UsersAPIService
	WorkflowDefinitionsAPI  *openapi.WorkflowDefinitionsAPIService
	WorkflowRulesAPI        *openapi.WorkflowRulesAPIService
	WorkflowRunsAPI         *openapi.WorkflowRunsAPIService
}

// NewClient creates a high-level client that routes Studio and Store APIs and injects bearer auth.
func NewClient(opts ClientOptions) (*Client, error) {
	endpoints, err := resolveClientEndpoints(opts)
	if err != nil {
		return nil, err
	}

	apiVersion := strings.TrimSpace(opts.APIVersion)
	if apiVersion == "" {
		apiVersion = defaultAPIVersion
	}

	baseHTTPClient := opts.HTTPClient
	if baseHTTPClient == nil {
		baseHTTPClient = http.DefaultClient
	}

	tokenSource, err := newBearerTokenSource(opts, endpoints, apiVersion)
	if err != nil {
		return nil, err
	}

	authHTTPClient := &http.Client{
		Transport: &authTransport{
			base:   baseHTTPClient.Transport,
			source: tokenSource,
		},
		CheckRedirect: baseHTTPClient.CheckRedirect,
		Jar:           baseHTTPClient.Jar,
		Timeout:       baseHTTPClient.Timeout,
	}

	studio := openapi.NewAPIClient(newGeneratedConfig(endpoints.StudioURL, endpoints.TokenServerURL, authHTTPClient, apiVersion))
	store := openapi.NewAPIClient(newGeneratedConfig(endpoints.StoreURL, endpoints.TokenServerURL, authHTTPClient, apiVersion))
	tokenClient := openapi.NewAPIClient(newGeneratedConfig(endpoints.TokenServerURL, endpoints.TokenServerURL, authHTTPClient, apiVersion))

	client := &Client{
		Studio:         studio,
		Store:          store,
		StudioURL:      endpoints.StudioURL,
		StoreURL:       endpoints.StoreURL,
		TokenServerURL: endpoints.TokenServerURL,

		APIKeysAPI:              studio.APIKeysAPI,
		AccessControlEntriesAPI: studio.AccessControlEntriesAPI,
		AccountsAPI:             studio.AccountsAPI,
		AppsAPI:                 studio.AppsAPI,
		AuditTrailAPI:           studio.AuditTrailAPI,
		EnvironmentsAPI:         studio.EnvironmentsAPI,
		InteractionRunsAPI:      studio.InteractionRunsAPI,
		InteractionsAPI:         studio.InteractionsAPI,
		OAuthClientsAPI:         studio.OAuthClientsAPI,
		OAuthGrantsAPI:          studio.OAuthGrantsAPI,
		OAuthProvidersAPI:       studio.OAuthProvidersAPI,
		ProjectsAPI:             studio.ProjectsAPI,
		PromptTemplatesAPI:      studio.PromptTemplatesAPI,
		RemoteMCPConnectionsAPI: studio.RemoteMCPConnectionsAPI,
		RolesAPI:                studio.RolesAPI,
		TokenServiceAPI:         tokenClient.TokenServiceAPI,
		UserGroupsAPI:           studio.UserGroupsAPI,
		UsersAPI:                studio.UsersAPI,

		AgentRunsAPI:           store.AgentRunsAPI,
		BulkOperationsAPI:      store.BulkOperationsAPI,
		CollectionsAPI:         store.CollectionsAPI,
		CommandsAPI:            store.CommandsAPI,
		ContentObjectTypesAPI:  store.ContentObjectTypesAPI,
		CostsAPI:               store.CostsAPI,
		DataAPI:                store.DataAPI,
		FilesAPI:               store.FilesAPI,
		ObjectsAPI:             store.ObjectsAPI,
		ProcessesAPI:           store.ProcessesAPI,
		RenderingAPI:           store.RenderingAPI,
		TasksAPI:               store.TasksAPI,
		WorkflowDefinitionsAPI: store.WorkflowDefinitionsAPI,
		WorkflowRulesAPI:       store.WorkflowRulesAPI,
		WorkflowRunsAPI:        store.WorkflowRunsAPI,
	}

	return client, nil
}

type clientEndpoints struct {
	StudioURL                   string
	StoreURL                    string
	TokenServerURL              string
	TokenServerURLExplicit      bool
	TokenServerURLSafelyDerived bool
}

func resolveClientEndpoints(opts ClientOptions) (clientEndpoints, error) {
	site := strings.TrimSpace(opts.Site)
	region := strings.TrimSpace(opts.Region)
	serverURL := strings.TrimSpace(opts.ServerURL)
	storeURL := strings.TrimSpace(opts.StoreURL)

	if site != "" && region != "" {
		return clientEndpoints{}, fmt.Errorf("%w: set either Site or Region, not both", ErrInvalidClientOptions)
	}
	if region != "" {
		var err error
		site, err = siteFromRegion(region, opts.Preview)
		if err != nil {
			return clientEndpoints{}, err
		}
	} else if site == "" && opts.Preview {
		site = previewSite(defaultSite)
	} else if site == "" && serverURL == "" && storeURL == "" {
		site = defaultSite
	}

	var err error
	if serverURL == "" {
		if site == "" {
			return clientEndpoints{}, fmt.Errorf("%w: Site or ServerURL is required", ErrInvalidClientOptions)
		}
		serverURL, err = siteToHTTPSURL(site)
		if err != nil {
			return clientEndpoints{}, err
		}
	}
	if storeURL == "" {
		if site == "" {
			return clientEndpoints{}, fmt.Errorf("%w: Site or StoreURL is required", ErrInvalidClientOptions)
		}
		storeURL, err = siteToHTTPSURL(site)
		if err != nil {
			return clientEndpoints{}, err
		}
	}

	studioURL, err := normalizeAPIURL(serverURL)
	if err != nil {
		return clientEndpoints{}, fmt.Errorf("%w: invalid ServerURL: %w", ErrInvalidClientOptions, err)
	}
	normalizedStoreURL, err := normalizeAPIURL(storeURL)
	if err != nil {
		return clientEndpoints{}, fmt.Errorf("%w: invalid StoreURL: %w", ErrInvalidClientOptions, err)
	}

	tokenURL := strings.TrimSpace(opts.TokenServerURL)
	tokenURLExplicit := tokenURL != ""
	tokenURLSafelyDerived := false
	if tokenURL == "" {
		tokenURL, tokenURLSafelyDerived = deriveTokenServerURL(site, serverURL, storeURL)
	}
	tokenURL, err = normalizeServerURL(tokenURL)
	if err != nil {
		return clientEndpoints{}, fmt.Errorf("%w: invalid TokenServerURL: %w", ErrInvalidClientOptions, err)
	}

	return clientEndpoints{
		StudioURL:                   studioURL,
		StoreURL:                    normalizedStoreURL,
		TokenServerURL:              tokenURL,
		TokenServerURLExplicit:      tokenURLExplicit,
		TokenServerURLSafelyDerived: tokenURLSafelyDerived,
	}, nil
}

func siteToHTTPSURL(site string) (string, error) {
	if strings.Contains(site, "://") {
		return site, nil
	}
	if strings.Contains(site, "/") {
		return "", fmt.Errorf("%w: Site must be a host, not a URL path", ErrInvalidClientOptions)
	}
	return "https://" + site, nil
}

func siteFromRegion(region string, preview bool) (string, error) {
	region = strings.ToLower(strings.TrimSpace(region))
	if region == "" {
		return "", fmt.Errorf("%w: Region is required", ErrInvalidClientOptions)
	}
	if !isRegionID(region) {
		return "", fmt.Errorf("%w: Region must be a region id such as us1 or eu1", ErrInvalidClientOptions)
	}
	site := "api." + region + ".vertesia.io"
	if preview {
		return previewSite(site), nil
	}
	return site, nil
}

func previewSite(site string) string {
	if strings.HasPrefix(site, "api-preview.") {
		return site
	}
	if strings.HasPrefix(site, "api.") {
		return "api-preview." + strings.TrimPrefix(site, "api.")
	}
	return site
}

func isRegionID(region string) bool {
	if region == "" || strings.HasPrefix(region, "-") || strings.HasSuffix(region, "-") {
		return false
	}
	for _, ch := range region {
		if ch >= 'a' && ch <= 'z' {
			continue
		}
		if ch >= '0' && ch <= '9' {
			continue
		}
		if ch == '-' {
			continue
		}
		return false
	}
	return true
}

func normalizeAPIURL(raw string) (string, error) {
	normalized, err := normalizeServerURL(raw)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", err
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		parsed.Path = "/api/v1"
	} else if !strings.HasSuffix(path, "/api/v1") {
		parsed.Path = path + "/api/v1"
	} else {
		parsed.Path = path
	}
	return parsed.String(), nil
}

func normalizeServerURL(raw string) (string, error) {
	value := strings.TrimRight(strings.TrimSpace(raw), "/")
	if value == "" {
		return "", errors.New("URL is required")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("URL must include scheme and host")
	}
	return parsed.String(), nil
}

func deriveTokenServerURL(site string, serverURL string, storeURL string) (string, bool) {
	candidate := strings.TrimSpace(site)
	if candidate != "" {
		if !strings.Contains(candidate, "://") {
			candidate = "https://" + candidate
		}
		if tokenURL := tokenURLFromAPIHost(candidate); tokenURL != "" {
			return tokenURL, true
		}
	}

	if tokenURL := tokenURLFromAPIHost(serverURL); tokenURL != "" {
		return tokenURL, true
	}
	if tokenURL := tokenURLFromAPIHost(storeURL); tokenURL != "" {
		return tokenURL, true
	}
	return defaultTokenURL, false
}

func tokenURLFromAPIHost(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	hostname := parsed.Hostname()
	if !strings.HasPrefix(hostname, "api") {
		return ""
	}
	stsHost := strings.Replace(hostname, "api-preview.", "api.", 1)
	stsHost = strings.Replace(stsHost, "api", "sts", 1)
	return "https://" + stsHost
}

func newGeneratedConfig(baseURL string, tokenURL string, httpClient *http.Client, apiVersion string) *openapi.Configuration {
	cfg := openapi.NewConfiguration()
	cfg.Servers = openapi.ServerConfigurations{{URL: baseURL, Description: "Vertesia API"}}
	cfg.OperationServers = map[string]openapi.ServerConfigurations{
		tokenIssueOperation: {{URL: tokenURL, Description: "Vertesia STS"}},
	}
	cfg.HTTPClient = httpClient
	cfg.AddDefaultHeader("x-api-version", apiVersion)
	return cfg
}

type bearerTokenSource interface {
	Token(context.Context) (string, error)
}

func newBearerTokenSource(opts ClientOptions, endpoints clientEndpoints, apiVersion string) (bearerTokenSource, error) {
	apiKey := strings.TrimSpace(opts.APIKey)
	token := strings.TrimSpace(opts.Token)
	if apiKey != "" && token != "" {
		return nil, fmt.Errorf("%w: set either APIKey or Token, not both", ErrInvalidClientOptions)
	}
	if token != "" {
		return staticTokenSource(token), nil
	}
	if apiKey != "" {
		if !strings.HasPrefix(apiKey, "sk-") {
			return nil, fmt.Errorf("%w: APIKey must be an sk- secret key", ErrUnsupportedAPIKey)
		}
		if !endpoints.TokenServerURLExplicit && !endpoints.TokenServerURLSafelyDerived {
			return nil, fmt.Errorf("%w: TokenServerURL is required when using APIKey with custom endpoints", ErrInvalidClientOptions)
		}
		httpClient := opts.HTTPClient
		if httpClient == nil {
			httpClient = http.DefaultClient
		}
		return &apiKeyTokenSource{
			apiKey:         apiKey,
			tokenServerURL: endpoints.TokenServerURL,
			httpClient:     httpClient,
			apiVersion:     apiVersion,
		}, nil
	}
	return staticTokenSource(""), nil
}

type staticTokenSource string

func (s staticTokenSource) Token(context.Context) (string, error) {
	return string(s), nil
}

type apiKeyTokenSource struct {
	apiKey         string
	tokenServerURL string
	httpClient     *http.Client
	apiVersion     string

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func (s *apiKeyTokenSource) Token(ctx context.Context) (string, error) {
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.token != "" && now.Before(s.expiresAt.Add(-tokenRefreshWindow)) {
		return s.token, nil
	}

	cfg := newGeneratedConfig(s.tokenServerURL, s.tokenServerURL, s.httpClient, s.apiVersion)
	client := openapi.NewAPIClient(cfg)
	tokenCtx := context.WithValue(ctx, openapi.ContextAccessToken, s.apiKey)
	issued, _, err := client.TokenServiceAPI.IssueToken(tokenCtx).Execute()
	if err != nil {
		return "", err
	}
	if issued == nil || strings.TrimSpace(issued.GetToken()) == "" {
		return "", errors.New("Vertesia STS returned an empty token")
	}

	s.token = issued.GetToken()
	s.expiresAt = tokenExpiry(s.token, now, issued.GetExpiresIn())
	return s.token, nil
}

func tokenExpiry(token string, now time.Time, expiresIn float32) time.Time {
	if exp, ok := jwtExpiry(token); ok {
		return exp
	}
	if expiresIn > 0 {
		return now.Add(time.Duration(float64(expiresIn) * float64(time.Second)))
	}
	return now.Add(time.Hour)
}

func jwtExpiry(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp json.Number `json:"exp"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.UseNumber()
	if err := decoder.Decode(&claims); err != nil || claims.Exp == "" {
		return time.Time{}, false
	}
	seconds, err := claims.Exp.Int64()
	if err != nil {
		floatSeconds, floatErr := claims.Exp.Float64()
		if floatErr != nil {
			return time.Time{}, false
		}
		seconds = int64(floatSeconds)
	}
	return time.Unix(seconds, 0), true
}

type authTransport struct {
	base   http.RoundTripper
	source bearerTokenSource
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	if t.source == nil || req.Header.Get("Authorization") != "" {
		return base.RoundTrip(req)
	}

	token, err := t.source.Token(req.Context())
	if err != nil {
		return nil, err
	}
	if token == "" {
		return base.RoundTrip(req)
	}

	cloned := req.Clone(req.Context())
	cloned.Header = req.Header.Clone()
	cloned.Header.Set("Authorization", "Bearer "+token)
	return base.RoundTrip(cloned)
}
