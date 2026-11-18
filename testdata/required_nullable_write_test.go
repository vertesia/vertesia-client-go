package openapi

import (
	"encoding/json"
	"testing"
)

// A required nullable list, map or free-form property is always written: nil encodes as null.
// Optional properties are still omitted when nil.
func TestRequiredNullableWrite(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value interface{}
		want  string
	}{
		{"clear allowlist", UpdateAppInstallationToolAllowlistPayload{}, `{"tool_allowlist":null}`},
		{"empty allowlist", UpdateAppInstallationToolAllowlistPayload{ToolAllowlist: []string{}}, `{"tool_allowlist":[]}`},
		{"allowlist", UpdateAppInstallationToolAllowlistPayload{ToolAllowlist: []string{"search"}},
			`{"tool_allowlist":["search"]}`},
		{"tool use without input", ToolUse{Id: "1", ToolName: "search"},
			`{"id":"1","tool_input":null,"tool_name":"search"}`},
		{"tool use", ToolUse{Id: "1", ToolName: "search", ToolInput: map[string]interface{}{"q": "x"}},
			`{"id":"1","tool_input":{"q":"x"},"tool_name":"search"}`},
		// Optional nullable lists: omitting them leaves the installation unchanged.
		{"optional", UpdateAppInstallationToolPermissionsPayload{}, `{}`},
		{"optional set", UpdateAppInstallationToolPermissionsPayload{ToolsRequiringApproval: []string{"delete"}},
			`{"tools_requiring_approval":["delete"]}`},
	} {
		encoded, err := json.Marshal(tc.value)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if string(encoded) != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, encoded, tc.want)
		}
	}

	// What the client writes, it reads back.
	var payload UpdateAppInstallationToolAllowlistPayload
	if err := json.Unmarshal([]byte(`{"tool_allowlist":null}`), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ToolAllowlist != nil {
		t.Fatalf("null allowlist decoded as %v", payload.ToolAllowlist)
	}
	var use ToolUse
	if err := json.Unmarshal([]byte(`{"id":"1","tool_name":"search","tool_input":null}`), &use); err != nil {
		t.Fatal(err)
	}
	if use.ToolInput != nil {
		t.Fatalf("null tool input decoded as %v", use.ToolInput)
	}
}
