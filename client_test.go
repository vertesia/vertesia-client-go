package vertesia_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/vertesia/vertesia-client-go/openapi"
)

const (
	apiVersion       = "20260319"
	intakeTimeout    = 3 * time.Minute
	intakePoll       = 3 * time.Second
	defaultStudioURL = "http://localhost:8091/api/v1"
	defaultZenoURL   = "http://localhost:8092/api/v1"
	defaultSTSURL    = "http://localhost:8093"
	liveTestsEnv     = "VERTESIA_LIVE_TESTS"
)

var (
	studioClient *APIClient
	zenoClient   *APIClient
	studioCtx    context.Context
	zenoCtx      context.Context

	createdPrompts      []string
	createdInteractions []string
	createdObjects      []string
	createdDataStores   []string
)

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func loadDotEnv() {
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	for {
		if loadDotEnvFile(filepath.Join(dir, ".env")) {
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

func loadDotEnvFile(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key != "" && os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
	return true
}

func liveTestsEnabled() bool {
	return os.Getenv(liveTestsEnv) == "1"
}

func isCI() bool {
	return os.Getenv("CI") != ""
}

func validLiveAPIKey(apiKey string) bool {
	apiKey = strings.TrimSpace(apiKey)
	return strings.HasPrefix(apiKey, "sk-") && len(apiKey) > len("sk-")
}

func TestDotEnvExampleDoesNotEnableLiveTests(t *testing.T) {
	data, err := os.ReadFile(".env.example")
	if err != nil {
		t.Fatalf("read .env.example failed: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(strings.TrimPrefix(key, "export "))
		value = strings.TrimSpace(strings.Trim(value, `"'`))
		if key == liveTestsEnv && value == "1" {
			t.Fatalf(".env.example must not set %s=1", liveTestsEnv)
		}
		if key == "VERTESIA_API_KEY" && validLiveAPIKey(value) {
			t.Fatal(".env.example must not contain a usable VERTESIA_API_KEY placeholder")
		}
	}
}

func liveClients(t *testing.T) {
	t.Helper()
	if studioClient == nil || zenoClient == nil {
		t.Skip("live OpenAPI client integration tests are disabled; set VERTESIA_LIVE_TESTS=1 with VERTESIA_API_KEY to run them")
	}
}

func uniqueName(prefix string) string {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(random))
}

func configuredClient(baseURL string, bearerToken string) (*APIClient, context.Context) {
	cfg := NewConfiguration()
	cfg.Servers = ServerConfigurations{{URL: baseURL, Description: "test target"}}
	cfg.OperationServers = map[string]ServerConfigurations{
		"TokenServiceAPIService.IssueToken": {{URL: baseURL, Description: "test target"}},
	}
	cfg.HTTPClient = &http.Client{Timeout: 180 * time.Second}
	cfg.AddDefaultHeader("x-api-version", apiVersion)
	return NewAPIClient(cfg), context.WithValue(context.Background(), ContextAccessToken, bearerToken)
}

func TestMain(m *testing.M) {
	loadDotEnv()
	if liveTestsEnabled() {
		apiKey := strings.TrimSpace(os.Getenv("VERTESIA_API_KEY"))
		if !validLiveAPIKey(apiKey) {
			if isCI() {
				fmt.Fprintln(os.Stderr, "VERTESIA_LIVE_TESTS=1 requires VERTESIA_API_KEY to be a non-placeholder sk- secret key")
				os.Exit(1)
			}
			fmt.Fprintln(os.Stderr, "skipping live OpenAPI client integration tests: VERTESIA_API_KEY is missing, invalid, or a placeholder")
		} else {
			stsURL := env("VERTESIA_STS_URL", defaultSTSURL)
			stsClient, stsCtx := configuredClient(stsURL, apiKey)
			token, _, err := stsClient.TokenServiceAPI.IssueToken(stsCtx).Execute()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to issue OpenAPI test token: %v\n", err)
				os.Exit(1)
			}
			if token == nil || token.GetToken() == "" {
				fmt.Fprintln(os.Stderr, "STS returned an empty OpenAPI test token")
				os.Exit(1)
			}

			studioClient, studioCtx = configuredClient(env("VERTESIA_STUDIO_URL", defaultStudioURL), token.GetToken())
			zenoClient, zenoCtx = configuredClient(env("VERTESIA_ZENO_URL", defaultZenoURL), token.GetToken())
		}
	}

	code := m.Run()
	cleanup()
	os.Exit(code)
}

func cleanup() {
	if zenoClient != nil {
		for _, objectID := range createdObjects {
			_, _, _ = zenoClient.ObjectsAPI.DeleteObject(zenoCtx, objectID).Execute()
		}
		for _, storeID := range createdDataStores {
			_, _, _ = zenoClient.DataAPI.DeleteDataStore(zenoCtx, storeID).Execute()
		}
	}
	if studioClient != nil {
		for _, interactionID := range createdInteractions {
			_, _, _ = studioClient.InteractionsAPI.DeleteInteraction(studioCtx, interactionID).Execute()
		}
		for _, promptID := range createdPrompts {
			_, _, _ = studioClient.PromptTemplatesAPI.DeletePrompt(studioCtx, promptID).Execute()
		}
	}
}

func optional[T any](call func() (T, *http.Response, error), statuses ...int) (T, bool, error) {
	value, resp, err := call()
	if err == nil {
		return value, true, nil
	}
	if resp != nil {
		for _, status := range statuses {
			if resp.StatusCode == status {
				var zero T
				return zero, false, nil
			}
		}
	}
	return value, false, err
}

func resolveProjectID(t *testing.T) string {
	t.Helper()
	if projectID := os.Getenv("VERTESIA_PROJECT_ID"); projectID != "" {
		return projectID
	}
	projects, _, err := studioClient.AccountsAPI.ListAccountProjects(studioCtx).Execute()
	if err != nil {
		t.Fatalf("listAccountProjects failed: %v", err)
	}
	if projects == nil || len(projects.GetData()) == 0 {
		t.Skip("no projects available on this account")
	}
	return projects.GetData()[0].GetId()
}

func defaultContentTypeID(t *testing.T) string {
	t.Helper()
	types, _, err := zenoClient.ContentObjectTypesAPI.ListContentObjectTypes(zenoCtx).
		Name("Default").
		Chunkable(true).
		Layout(true).
		Schema(true).
		Limit(10).
		Execute()
	if err != nil {
		t.Fatalf("listContentObjectTypes failed: %v", err)
	}
	if len(types) == 0 || types[0].GetId() == "" {
		t.Skip("no Default content object type available")
	}
	return types[0].GetId()
}

func TestGeneratedGoClientReadEndpoints(t *testing.T) {
	liveClients(t)

	account, _, err := studioClient.AccountsAPI.GetCurrentAccount(studioCtx).Execute()
	if err != nil {
		t.Fatalf("getCurrentAccount failed: %v", err)
	}
	if account == nil {
		t.Fatal("getCurrentAccount returned nil")
	}

	project, _, err := studioClient.ProjectsAPI.GetProject(studioCtx, resolveProjectID(t)).Execute()
	if err != nil {
		t.Fatalf("getProject failed: %v", err)
	}
	if project == nil {
		t.Fatal("getProject returned nil")
	}

	if prompts, _, err := studioClient.PromptTemplatesAPI.ListPrompts(studioCtx).Limit(10).Execute(); err != nil {
		t.Fatalf("listPrompts failed: %v", err)
	} else if prompts == nil {
		t.Fatal("listPrompts returned nil")
	}
	if interactions, _, err := studioClient.InteractionsAPI.ListInteractions(studioCtx).Limit(10).Execute(); err != nil {
		t.Fatalf("listInteractions failed: %v", err)
	} else if interactions == nil {
		t.Fatal("listInteractions returned nil")
	}
	searchPayload := NewComplexSearchPayload()
	searchPayload.SetLimit(10)
	if objects, _, err := zenoClient.ObjectsAPI.SearchObjects(zenoCtx).
		ComplexSearchPayload(*searchPayload).
		Execute(); err != nil {
		t.Fatalf("searchObjects failed: %v", err)
	} else if objects == nil || objects.GetResults() == nil {
		t.Fatal("searchObjects returned no results field")
	}
	if stores, _, err := zenoClient.DataAPI.ListDataStores(zenoCtx).Execute(); err != nil {
		t.Fatalf("listDataStores failed: %v", err)
	} else if stores == nil {
		t.Fatal("listDataStores returned nil")
	}
	if workflows, _, err := zenoClient.WorkflowDefinitionsAPI.ListWorkflowDefinitions(zenoCtx).Execute(); err != nil {
		t.Fatalf("listWorkflowDefinitions failed: %v", err)
	} else if workflows == nil {
		t.Fatal("listWorkflowDefinitions returned nil")
	}
}

func TestGeneratedGoClientPromptInteractionLifecycle(t *testing.T) {
	liveClients(t)

	promptPayload := NewPromptTemplateCreatePayload(
		uniqueName("go-openapi-prompt"),
		PROMPTROLE_USER,
		"Answer with a short greeting for <%= name %>.",
		TEMPLATETYPE_JST,
	)
	prompt, ok, err := optional(func() (*PromptTemplate, *http.Response, error) {
		return studioClient.PromptTemplatesAPI.CreatePrompt(studioCtx).
			PromptTemplateCreatePayload(*promptPayload).
			Execute()
	}, http.StatusForbidden)
	if err != nil {
		t.Fatalf("createPrompt failed: %v", err)
	}
	if !ok {
		t.Skip("prompt creation is not permitted for this test principal")
	}
	if prompt == nil || prompt.GetId() == "" {
		t.Fatal("createPrompt returned no id")
	}
	createdPrompts = append(createdPrompts, prompt.GetId())

	rendered, _, err := studioClient.PromptTemplatesAPI.RenderPrompt(studioCtx, prompt.GetId()).
		RequestBody(map[string]interface{}{"name": "Vertesia"}).
		Execute()
	if err != nil {
		t.Fatalf("renderPrompt failed: %v", err)
	}
	if rendered == nil || rendered.GetRendered() == "" {
		t.Fatal("renderPrompt returned no rendered output")
	}

	segment := NewPromptSegmentDef(PROMPTSEGMENTDEFTYPE_TEMPLATE)
	promptID := prompt.GetId()
	segment.SetTemplate(PromptSegmentDefTemplate{String: &promptID})
	interactionPayload := NewInteractionCreatePayload(
		INTERACTIONSTATUS_DRAFT,
		[]PromptSegmentDef{*segment},
		uniqueName("go-openapi-interaction"),
	)
	interactionPayload.SetVisibility(INTERACTIONVISIBILITY_PRIVATE)
	interactionPayload.SetTags([]string{"integration-test", "openapi-go"})
	interaction, _, err := studioClient.InteractionsAPI.CreateInteraction(studioCtx).
		InteractionCreatePayload(*interactionPayload).
		Execute()
	if err != nil {
		t.Fatalf("createInteraction failed: %v", err)
	}
	if interaction == nil || interaction.GetId() == "" {
		t.Fatal("createInteraction returned no id")
	}
	createdInteractions = append(createdInteractions, interaction.GetId())

	execPayload := NewInteractionExecutionPayload()
	execPayload.SetData(map[string]interface{}{"name": "Vertesia"})
	execResult, resp, err := studioClient.InteractionsAPI.ExecuteInteraction(studioCtx, interaction.GetId()).
		InteractionExecutionPayload(*execPayload).
		Execute()
	if err != nil {
		if resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 &&
			strings.Contains(err.Error(), "data matches more than one schema") {
			return
		}
		t.Fatalf("executeInteraction failed: %v", err)
	}
	if execResult == nil || (execResult.GetStatus() != EXECUTIONRUNSTATUS_COMPLETED && execResult.GetStatus() != EXECUTIONRUNSTATUS_FAILED) {
		t.Fatalf("unexpected executeInteraction status: %v", execResult)
	}

	_, _, _ = studioClient.InteractionsAPI.DeleteInteraction(studioCtx, interaction.GetId()).Execute()
	createdInteractions = nil
	_, _, _ = studioClient.PromptTemplatesAPI.DeletePrompt(studioCtx, prompt.GetId()).Execute()
	createdPrompts = nil
}

func TestGeneratedGoClientDataStoreCrud(t *testing.T) {
	liveClients(t)

	payload := NewCreateDataStorePayload(uniqueName("go-openapi-data-store"))
	payload.SetDescription("Created by Go OpenAPI integration tests")
	payload.SetTags([]string{"integration-test", "openapi-go"})
	created, ok, err := optional(func() (*DataStore, *http.Response, error) {
		return zenoClient.DataAPI.CreateDataStore(zenoCtx).CreateDataStorePayload(*payload).Execute()
	}, http.StatusForbidden)
	if err != nil {
		t.Fatalf("createDataStore failed: %v", err)
	}
	if !ok {
		t.Skip("data store creation is not permitted for this test principal")
	}
	if created == nil || created.GetId() == "" {
		t.Fatal("createDataStore returned no id")
	}
	createdDataStores = append(createdDataStores, created.GetId())

	if retrieved, _, err := zenoClient.DataAPI.GetDataStore(zenoCtx, created.GetId()).Execute(); err != nil {
		t.Fatalf("getDataStore failed: %v", err)
	} else if retrieved.GetId() != created.GetId() {
		t.Fatal("getDataStore returned a different store")
	}
	if schema, _, err := zenoClient.DataAPI.GetDataStoreSchema(zenoCtx, created.GetId()).Execute(); err != nil {
		t.Fatalf("getDataStoreSchema failed: %v", err)
	} else if schema == nil {
		t.Fatal("getDataStoreSchema returned nil")
	}
	if _, _, err := zenoClient.DataAPI.DeleteDataStore(zenoCtx, created.GetId()).Execute(); err != nil {
		t.Fatalf("deleteDataStore failed: %v", err)
	}
	createdDataStores = nil
}

func TestGeneratedGoClientImageIntake(t *testing.T) {
	liveClients(t)

	objectTypeID := defaultContentTypeID(t)
	fileName := uniqueName("go-openapi-image") + ".jpg"
	imageBytes := testJPEG(t)

	uploadPayload := NewGetUploadUrlPayload(fileName)
	uploadPayload.SetMimeType("image/jpeg")
	upload, _, err := zenoClient.FilesAPI.GetFileUploadUrl(zenoCtx).
		GetUploadUrlPayload(*uploadPayload).
		Execute()
	if err != nil {
		t.Fatalf("getFileUploadUrl failed: %v", err)
	}
	if upload == nil || upload.GetUrl() == "" || upload.GetId() == "" {
		t.Fatal("getFileUploadUrl returned incomplete upload info")
	}

	req, err := http.NewRequest(http.MethodPut, upload.GetUrl(), bytes.NewReader(imageBytes))
	if err != nil {
		t.Fatalf("build signed upload request failed: %v", err)
	}
	req.Header.Set("Content-Type", "image/jpeg")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("signed upload failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("signed upload returned HTTP %d", resp.StatusCode)
	}

	content := NewContentSource()
	content.SetSource(upload.GetId())
	content.SetType("image/jpeg")
	content.SetName(fileName)
	payload := NewCreateContentObjectPayload()
	payload.SetType(objectTypeID)
	payload.SetName(fileName)
	payload.SetContent(*content)
	payload.SetSecurity(map[string][]string{
		"content:read":   {"project:*"},
		"content:write":  {"project:*"},
		"content:delete": {"project:*"},
	})
	payload.SetTags([]string{"integration-test", "openapi-go", "file-intake"})
	payload.SetProperties(map[string]interface{}{"test_type": "openapi-go-image-intake"})

	created, _, err := zenoClient.ObjectsAPI.CreateObject(zenoCtx).
		CreateContentObjectPayload(*payload).
		Execute()
	if err != nil {
		t.Fatalf("createObject failed: %v", err)
	}
	if created == nil || created.GetId() == "" {
		t.Fatal("createObject returned no id")
	}
	createdObjects = append(createdObjects, created.GetId())

	deadline := time.Now().Add(intakeTimeout)
	for time.Now().Before(deadline) {
		latest, _, err := zenoClient.ObjectsAPI.GetObject(zenoCtx, created.GetId()).Execute()
		if err != nil {
			t.Fatalf("getObject failed: %v", err)
		}
		if latest.GetStatus() == CONTENTOBJECTSTATUS_COMPLETED || latest.GetStatus() == CONTENTOBJECTSTATUS_READY {
			source, _, err := zenoClient.ObjectsAPI.GetObjectContentSource(zenoCtx, latest.GetId()).Execute()
			if err != nil {
				t.Fatalf("getObjectContentSource failed: %v", err)
			}
			if source == nil || source.GetSource() == "" {
				t.Fatal("getObjectContentSource returned no signed source")
			}
			return
		}
		if latest.GetStatus() == CONTENTOBJECTSTATUS_FAILED {
			t.Fatal("object intake failed")
		}
		time.Sleep(intakePoll)
	}
	t.Fatal("timed out waiting for object intake")
}

func testJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 0, color.RGBA{G: 255, A: 255})
	img.Set(0, 1, color.RGBA{B: 255, A: 255})
	img.Set(1, 1, color.RGBA{R: 255, G: 255, B: 255, A: 255})

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode test jpeg failed: %v", err)
	}
	return buf.Bytes()
}

func TestGeneratedGoClientRequestShapes(t *testing.T) {
	liveClients(t)

	_, _, err := optional(func() (*BulkOperationResponse, *http.Response, error) {
		return zenoClient.BulkOperationsAPI.RunBulkContentOperation(zenoCtx).
			BulkOperationPayload(*NewBulkOperationPayload("update", []string{}, map[string]interface{}{})).
			Execute()
	}, http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound, http.StatusConflict)
	if err != nil {
		t.Fatalf("runBulkContentOperation request shape failed before API response: %v", err)
	}
}
