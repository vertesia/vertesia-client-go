package vertesia

import (
	"encoding/json"
	"testing"

	"github.com/vertesia/vertesia-client-go/openapi"
)

// Proves generated Go clients preserve process-agent skill activation and execution exclusions.
func TestProcessAgentLaunchPolicyDeserializes(t *testing.T) {
	var node openapi.NodeDefinition
	body := []byte(`{
		"type":"agent",
		"interaction":"sys:AppDeveloper",
		"inherit_context":false,
		"initial_skills":["app_quick_fix"],
		"excluded_tools":["learn_app_development"],
		"agent_policy":{
			"phases":[{
				"id":"source_typechecked",
				"tools":["app_workspace_typecheck"],
				"recovery_tools":["app_workspace_read","app_workspace_edit"],
				"recovery_prompt":"Repair only after a concrete typecheck failure."
			}],
			"restrict_to_phase_tools":true
		}
	}`)

	if err := json.Unmarshal(body, &node); err != nil {
		t.Fatalf("NodeDefinition did not decode: %v", err)
	}
	if node.Type != "agent" {
		t.Fatalf("type = %q, want agent", node.Type)
	}
	if node.InheritContext == nil || *node.InheritContext {
		t.Fatalf("inherit_context = %v, want false", node.InheritContext)
	}
	if len(node.InitialSkills) != 1 || node.InitialSkills[0] != "app_quick_fix" {
		t.Fatalf("initial_skills = %v", node.InitialSkills)
	}
	if len(node.ExcludedTools) != 1 || node.ExcludedTools[0] != "learn_app_development" {
		t.Fatalf("excluded_tools = %v", node.ExcludedTools)
	}
	if node.AgentPolicy == nil || len(node.AgentPolicy.Phases) != 1 {
		t.Fatalf("agent_policy = %#v", node.AgentPolicy)
	}
	phase := node.AgentPolicy.Phases[0]
	if len(phase.RecoveryTools) != 2 || phase.RecoveryTools[0] != "app_workspace_read" || phase.RecoveryTools[1] != "app_workspace_edit" {
		t.Fatalf("recovery_tools = %v", phase.RecoveryTools)
	}
	if node.AgentPolicy.RestrictToPhaseTools == nil || !*node.AgentPolicy.RestrictToPhaseTools {
		t.Fatalf("restrict_to_phase_tools = %v", node.AgentPolicy.RestrictToPhaseTools)
	}
}
