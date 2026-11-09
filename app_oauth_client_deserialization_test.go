package vertesia

import (
	"encoding/json"
	"testing"

	"github.com/vertesia/vertesia-client-go/openapi"
)

func TestAppOAuthClientManifestCallbacks(t *testing.T) {
	var config openapi.AppOAuthClientConfig
	if err := json.Unmarshal([]byte(`{"redirect_uris":["https://app.example/app"]}`), &config); err != nil {
		t.Fatal(err)
	}
	if len(config.GetRedirectUris()) != 1 || config.GetRedirectUris()[0] != "https://app.example/app" {
		t.Fatalf("lost manifest callback: %#v", config)
	}
}

func TestSignedOAuthClientRegistration(t *testing.T) {
	payload := `{"client_id":"vts_test","client_name":"Middleware","client_type":"confidential",
        "redirect_uris":["https://app.example/callback"],"grant_types":["authorization_code","urn:ietf:params:oauth:grant-type:token-exchange"],
        "response_types":["code"],"token_endpoint_auth_method":"private_key_jwt",
        "jwks_uri":"https://app.example/keys","allowed_scopes":["content:read"],
        "registration_source":"admin","status":"active","project_binding_mode":"user_select",
        "created_at":"2026-10-17T00:00:00Z","updated_at":"2026-10-17T00:00:00Z"}`
	var client openapi.OAuthClient
	if err := json.Unmarshal([]byte(payload), &client); err != nil {
		t.Fatal(err)
	}
	if client.GetTokenEndpointAuthMethod() != "private_key_jwt" || client.GetJwksUri() != "https://app.example/keys" {
		t.Fatalf("lost signed-client registration: %#v", client)
	}
	if err := json.Unmarshal([]byte(`[]`), &client); err == nil {
		t.Fatal("accepted an array instead of a client registration object")
	}
}

func TestFederationClientAllowlist(t *testing.T) {
	var policy openapi.OidcFederationConnectionPayload
	if err := json.Unmarshal([]byte(`{"name":"Trust","issuer":"https://idp.example","external_id_claim":"sub",
        "allowed_audiences":["middleware"],"allowed_algorithms":["RS256"],
        "subject_token_type":"urn:ietf:params:oauth:token-type:jwt",
        "allowed_project_ids":["012345678901234567890124"],"allowed_client_ids":["vts_test"],
        "max_token_ttl_seconds":300,"enabled":false}`), &policy); err != nil {
		t.Fatal(err)
	}
	if len(policy.GetAllowedClientIds()) != 1 || policy.GetAllowedClientIds()[0] != "vts_test" {
		t.Fatalf("lost client allowlist: %#v", policy)
	}
}

func TestFederationAllProjects(t *testing.T) {
	payload := `{"name":"Trust","issuer":"https://idp.example","external_id_claim":"sub",
        "allowed_audiences":["middleware"],"allowed_algorithms":["RS256"],
        "subject_token_type":"urn:ietf:params:oauth:token-type:jwt",
        "all_projects":true,"allowed_project_ids":[],"max_token_ttl_seconds":300,"enabled":false}`
	var policy openapi.OidcFederationConnectionPayload
	if err := json.Unmarshal([]byte(payload), &policy); err != nil {
		t.Fatal(err)
	}
	if !policy.GetAllProjects() || len(policy.GetAllowedProjectIds()) != 0 {
		t.Fatalf("lost all-project mode: %#v", policy)
	}
	if err := json.Unmarshal([]byte(`{"all_projects":{}}`), &policy); err == nil {
		t.Fatal("accepted non-boolean all_projects")
	}
}
