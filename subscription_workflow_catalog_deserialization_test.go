package vertesia

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/vertesia/vertesia-client-go/openapi"
)

func TestSubscriptionWorkflowCatalogPreservesSchemasAndTemplates(t *testing.T) {
	var catalog openapi.SubscriptionWorkflowCatalog
	payload := `{"workflows":[{"endpoint":"StandardIntake","title":"Standard Intake","description":"Process content","variables_schema":{"type":"object","properties":{"forceGeneration":{"type":"boolean"}}},"variables_template":{"forceGeneration":false}}]}`
	if err := json.Unmarshal([]byte(payload), &catalog); err != nil {
		t.Fatal(err)
	}
	workflow := catalog.GetWorkflows()[0]
	if workflow.GetEndpoint() != "StandardIntake" {
		t.Fatal("workflow endpoint was lost")
	}
	if workflow.GetVariablesTemplate()["forceGeneration"] != false {
		t.Fatal("template was lost")
	}
	schema, err := json.Marshal(workflow.GetVariablesSchema())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(schema, []byte(`"type":"object"`)) {
		t.Fatal("schema was lost")
	}
}

func TestSubscriptionWorkflowCatalogRejectsMissingMetadata(t *testing.T) {
	var catalog openapi.SubscriptionWorkflowCatalog
	if err := json.Unmarshal([]byte(`{"workflows":[{"endpoint":"StandardIntake"}]}`), &catalog); err == nil {
		t.Fatal("expected missing workflow metadata to be rejected")
	}
}
