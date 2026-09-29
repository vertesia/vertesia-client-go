package vertesia_test

import (
	"encoding/json"
	"testing"

	"github.com/vertesia/vertesia-client-go/openapi"
)

func TestGeneratedModelsIgnoreUnknownResponseFields(t *testing.T) {
	var response openapi.ErrorResponse
	err := json.Unmarshal([]byte(`{
		"error": "Bad Request",
		"status": 400,
		"message": "invalid request",
		"server_added_field": {"nested": true}
	}`), &response)
	if err != nil {
		t.Fatalf("unmarshal ErrorResponse with unknown field failed: %v", err)
	}
	if response.GetError() != "Bad Request" || response.GetStatus() != 400 {
		t.Fatalf("unexpected ErrorResponse: %#v", response)
	}
}

func TestGeneratedModelsIgnoreNestedUnknownResponseFields(t *testing.T) {
	var response openapi.AccountProjectsResponse
	err := json.Unmarshal([]byte(`{
		"data": [{
			"id": "project-1",
			"name": "Project One",
			"account": "account-1",
			"server_added_nested_field": {"ignored": true}
		}],
		"server_added_top_level_field": "ignored"
	}`), &response)
	if err != nil {
		t.Fatalf("unmarshal AccountProjectsResponse with nested unknown field failed: %v", err)
	}
	if len(response.GetData()) != 1 {
		t.Fatalf("decoded project count = %d, want 1", len(response.GetData()))
	}
	project := response.GetData()[0]
	if project.GetId() != "project-1" || project.GetName() != "Project One" || project.GetAccount() != "account-1" {
		t.Fatalf("unexpected nested ProjectRef: %#v", project)
	}
}

func TestGeneratedUnionsIgnoreUnknownFields(t *testing.T) {
	var response openapi.EmbeddingsApiInput
	err := json.Unmarshal([]byte(`{
		"type": "text",
		"text": "hello",
		"server_added_field": "ignored"
	}`), &response)
	if err != nil {
		t.Fatalf("unmarshal EmbeddingsApiInput with unknown field failed: %v", err)
	}
	if response.EmbeddingsApiTextInput == nil {
		t.Fatalf("EmbeddingsApiInput did not resolve to EmbeddingsApiTextInput: %#v", response)
	}
	if response.EmbeddingsApiTextInput.GetText() == "" {
		t.Fatal("EmbeddingsApiTextInput text was not decoded")
	}
}

func TestGeneratedEnumsUseUnknownDefaultForUnknownValues(t *testing.T) {
	var accountType openapi.AccountType
	if err := json.Unmarshal([]byte(`"new-account-kind"`), &accountType); err != nil {
		t.Fatalf("unmarshal unknown AccountType failed: %v", err)
	}
	if accountType != openapi.ACCOUNTTYPE_UNKNOWN_DEFAULT_OPEN_API {
		t.Fatalf("AccountType = %q, want %q", accountType, openapi.ACCOUNTTYPE_UNKNOWN_DEFAULT_OPEN_API)
	}
}
