package main

import (
	"encoding/json"
	"strings"
	"testing"
)

const generated = `package openapi

import (
	"encoding/json"
	"fmt"
)

type Mode string

type Example struct {
	Name   string            ` + "`json:\"name\"`" + `
	Count  int32             ` + "`json:\"count\"`" + `
	Kind   string            ` + "`json:\"kind\"`" + `
	Mode   Mode              ` + "`json:\"mode\"`" + `
	Base   string            ` + "`json:\"base\"`" + `
	Tags   []string          ` + "`json:\"tags\"`" + `
	Input  map[string]interface{} ` + "`json:\"input\"`" + `
	Portal NullableString    ` + "`json:\"portal_url\"`" + `
	Value  interface{}       ` + "`json:\"value\"`" + `
	Owner  *string           ` + "`json:\"owner\"`" + `
	Note   *string           ` + "`json:\"note,omitempty\"`" + `
}

func (o *Example) UnmarshalJSON(data []byte) (err error) {
	requiredProperties := []string{
		"name",
		"count",
		"kind",
		"mode",
		"base",
		"tags",
		"input",
		"portal_url",
		"value",
		"owner",
	}

	allProperties := make(map[string]interface{})

	err = json.Unmarshal(data, &allProperties)

	if err != nil {
		return err
	}

	for _, requiredProperty := range requiredProperties {
		if _, exists := allProperties[requiredProperty]; !exists {
			return fmt.Errorf("no value given for required property %v", requiredProperty)
		}
	}

	return nil
}

type ExampleInline struct {
	Id    string ` + "`json:\"id\"`" + `
	Label string ` + "`json:\"label\"`" + `
}

// GetLabel returns the Label field value
// If the value is explicit nil, the zero value for string will be returned
func (o *ExampleInline) GetLabel() string {
	return o.Label
}

func (o *ExampleInline) UnmarshalJSON(data []byte) (err error) {
	requiredProperties := []string{
		"id",
		"label",
	}

	allProperties := make(map[string]interface{})

	err = json.Unmarshal(data, &allProperties)

	if err != nil {
		return err
	}

	for _, requiredProperty := range requiredProperties {
		if _, exists := allProperties[requiredProperty]; !exists {
			return fmt.Errorf("no value given for required property %v", requiredProperty)
		}
	}

	return nil
}
`

// The components behind Example. ExampleInline stands for an inline schema, so it has none.
const components = `{
	"Example": {
		"allOf": [{"$ref": "#/components/schemas/Base_schema"}],
		"properties": {
			"name": {"type": "string", "pattern": "^[a-z]+$"},
			"count": {"$ref": "#/components/schemas/Count"},
			"kind": {"type": ["string", "null"]},
			"mode": {"anyOf": [{"$ref": "#/components/schemas/Mode"}, {"type": "null"}]},
			"tags": {"type": "array", "items": {"type": "string"}},
			"input": {"type": "object"},
			"portal_url": {"type": ["string", "null"]},
			"value": {},
			"owner": {"type": "string"}
		}
	},
	"Base_schema": {"properties": {"base": {"$ref": "#/components/schemas/Nullable"}}},
	"Count": {"type": "integer"},
	"Mode": {"type": "string", "enum": ["a", "b"]},
	"Nullable": {"type": "string", "nullable": true}
}`

func testSpec(t *testing.T) *spec {
	t.Helper()
	var schemas map[string]schema
	if err := json.Unmarshal([]byte(components), &schemas); err != nil {
		t.Fatal(err)
	}
	return newSpec(schemas)
}

func check(names ...string) string {
	return "\t" + marker + ": it would decode as its zero value.\n" +
		"\tfor _, requiredProperty := range []string{" + strings.Join(names, ", ") + "} {\n" +
		"\t\tif allProperties[requiredProperty] == nil {\n" +
		"\t\t\treturn fmt.Errorf(\"required property %v is null\", requiredProperty)\n" +
		"\t\t}\n\t}\n\n\treturn nil\n"
}

func TestPatch(t *testing.T) {
	patched, err := patch([]byte(generated), testSpec(t), map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	// name and count are not nullable in the spec. kind, mode and base are, through a null type, a
	// union with a null branch and a nullable reference in an extended schema. tags, input, portal_url,
	// value and owner hold null in Go.
	if !strings.Contains(string(patched), check(`"name"`, `"count"`)) {
		t.Fatalf("Example:\n%s", patched)
	}
	// Without a component, the generator's nullable marker decides.
	if !strings.Contains(string(patched), check(`"id"`)) {
		t.Fatalf("ExampleInline:\n%s", patched)
	}
	again, err := patch(patched, testSpec(t), map[string]bool{})
	if err != nil || string(again) != string(patched) {
		t.Fatalf("not idempotent: %v", err)
	}
}

func TestRawGeneratorOutput(t *testing.T) {
	// As openapi-generator writes it, before gofmt.
	raw := strings.ReplaceAll(generated, "range requiredProperties", "range(requiredProperties)")
	patched, err := patch([]byte(raw), testSpec(t), map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(patched), `range []string{"name", "count"}`) {
		t.Fatalf("patched:\n%s", patched)
	}
}

func TestPackageTypesThatHoldNull(t *testing.T) {
	// A required property of a named map type, such as a JSON object, keeps null apart from {}.
	source := strings.Replace(generated, "type Mode string", "type Mode string\n\ntype JSONObject map[string]interface{}", 1)
	source = strings.Replace(source, "Count  int32 ", "Count  JSONObject ", 1)
	patched, err := patch([]byte(source), testSpec(t), map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(patched), `range []string{"name"} {`) {
		t.Fatalf("patched:\n%s", patched)
	}
}

func TestModelWithoutRequiredPropertiesIsLeftUnchanged(t *testing.T) {
	source := strings.Replace(generated, `"name",
		"count",
		"kind",`, `"kind",`, 1)
	source = strings.Replace(source, `"id",
		"label",`, `"label",`, 1)
	patched, err := patch([]byte(source), testSpec(t), map[string]bool{})
	if err != nil || string(patched) != source {
		t.Fatalf("changed a model whose required properties can all be null: %v\n%s", err, patched)
	}
}

func TestNullAllowed(t *testing.T) {
	s := testSpec(t)
	for body, want := range map[string]bool{
		`{}`:                                   true,
		`{"description": "any value"}`:         true,
		`{"type": "string"}`:                   false,
		`{"type": "null"}`:                     true,
		`{"type": ["array", "null"]}`:          true,
		`{"type": ["array", "object"]}`:        false,
		`{"type": "string", "nullable": true}`: true,
		`{"enum": ["a", null]}`:                true,
		`{"enum": ["a"]}`:                      false,
		`{"const": "a"}`:                       false,
		`{"const": null}`:                      true,
		`{"anyOf": [{"type": "string"}, {"type": "null"}]}`:    true,
		`{"oneOf": [{"type": "string"}, {"type": "integer"}]}`: false,
		`{"allOf": [{}, {"type": "string"}]}`:                  false,
		`{"$ref": "#/components/schemas/Count"}`:               false,
		`{"$ref": "#/components/schemas/Nullable"}`:            true,
		`{"$ref": "#/components/schemas/Unknown"}`:             true,
	} {
		var sc schema
		if err := json.Unmarshal([]byte(body), &sc); err != nil {
			t.Fatal(err)
		}
		if got := s.nullAllowed(sc, map[string]bool{}); got != want {
			t.Errorf("%s: got %v", body, got)
		}
	}
}

func TestTypeName(t *testing.T) {
	for component, want := range map[string]string{
		"ToolUse":                             "ToolUse",
		"SupportedIntegrations_aws":           "SupportedIntegrationsAws",
		"TransientToken_UserInviteTokenData_": "TransientTokenUserInviteTokenData",
	} {
		if got := typeName(component); got != want {
			t.Errorf("%s: got %s", component, got)
		}
	}
}
