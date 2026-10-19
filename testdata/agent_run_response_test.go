package openapi

import (
	"encoding/json"
	"strings"
	"testing"
)

// An agent run carries an execution config whose inference_profile is a nullable, pattern-constrained
// string. Before the patch dropped regexp tags the validator cannot evaluate, every oneOf branch
// failed validation and the union reported "data failed to match schemas in oneOf(AgentRunResponse)".
func TestAgentRunResponseWithExecutionConfig(t *testing.T) {
	const base = `{
		"id": "000000000000000000000001",
		"account": "000000000000000000000002",
		"project": "000000000000000000000003",
		"interaction": "sys:GeneralAgent",
		"interactionRef": {"id": "sys:GeneralAgent", "name": "sys:GeneralAgent", "endpoint": "sys:GeneralAgent",
			"status": "code", "version": 0, "tags": [], "updated_at": "1970-01-01T00:00:00.000Z"},
		"run_kind": "agent",
		"run_type": "autonomous",
		"status": "completed",
		"started_by": "user:000000000000000000000004",
		"started_at": "2026-07-28T11:08:41.127Z",
		"created_at": "2026-07-28T11:08:41.128Z",
		"updated_at": "2026-07-29T10:59:13.166Z",
		"visibility": "project",
		"source_type": "api",
		"interactive": true,
		"tool_approval_mode": "full_control",
		"config": {"environment": "000000000000000000000005", "model": "claude", "model_options": {"_option_id": "vertexai-claude"}CONFIG}
	}`
	cases := map[string]string{
		"profile absent": ``,
		"profile null":   `, "inference_profile": null`,
		"profile set":    `, "inference_profile": "000000000000000000000006"`,
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			var run AgentRunResponse
			if err := json.Unmarshal([]byte(strings.Replace(base, "CONFIG", extra, 1)), &run); err != nil {
				t.Fatal(err)
			}
			if run.AutonomousRunResponse == nil {
				t.Fatalf("AgentRunResponse did not resolve to AutonomousRunResponse: %#v", run)
			}
			if run.AutonomousRunResponse.GetConfig().Environment == nil {
				t.Fatal("execution config was not decoded")
			}
		})
	}
}
