package vertesia

import (
	"encoding/json"
	"testing"

	"github.com/vertesia/vertesia-client-go/openapi"
)

func TestDeserializesMediaInteractionAndProfile(t *testing.T) {
	var target openapi.WorkflowEventDeliveryTarget
	payload := `{"type":"workflow","endpoint":"StandardIntake","vars":{"media_analysis":{"video":{"interaction":"custom:AnalyzeVideo","config":{"inference_profile":"111111111111111111111111"}}}}}`
	if err := json.Unmarshal([]byte(payload), &target); err != nil {
		t.Fatal(err)
	}
	media := target.GetVars()["media_analysis"].(map[string]any)
	video := media["video"].(map[string]any)
	config := video["config"].(map[string]any)
	if video["interaction"] != "custom:AnalyzeVideo" {
		t.Fatal("interaction was lost")
	}
	if config["inference_profile"] != "111111111111111111111111" {
		t.Fatal("profile was lost")
	}
}

// The pinned Go client's permissive oneOf decoder cannot disambiguate a workflow target from
// other targets that have only optional fields. Exercise the supported request-writing path here.
func TestSerializesFullSubscriptionReplacement(t *testing.T) {
	id := "sys:StandardIntake"
	target := openapi.WorkflowEventDeliveryTargetInput{Type: "workflow", Endpoint: "StandardIntake"}
	payload := openapi.CreateEventSubscriptionPayload{
		Name: "Normal intake", OverridesSystemSubscriptionId: &id,
		Filter: openapi.EventSubscriptionFilter{}, RunAsRole: openapi.SystemRoles("automation"),
		Target: openapi.EventDeliveryTargetInput{WorkflowEventDeliveryTargetInput: &target},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["overrides_system_subscription_id"] != id {
		t.Fatal("replacement identity was lost")
	}
	if wire["target"].(map[string]any)["type"] != "workflow" {
		t.Fatal("workflow target was lost")
	}
}

func TestCustomWorkflowVariablesDoNotEnforceIntakeMediaShape(t *testing.T) {
	var target openapi.WorkflowEventDeliveryTarget
	if err := json.Unmarshal([]byte(`{"type":"workflow","endpoint":"CustomWorker","vars":{"media_analysis":"custom-layout"}}`), &target); err != nil {
		t.Fatal(err)
	}
	if target.GetVars()["media_analysis"] != "custom-layout" {
		t.Fatal("custom variables were lost")
	}
}
