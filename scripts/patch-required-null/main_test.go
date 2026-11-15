package main

import (
	"strings"
	"testing"
)

const generated = `package openapi

import (
	"encoding/json"
	"fmt"
)

type Example struct {
	Name      string            ` + "`json:\"name\"`" + `
	Tags      []string          ` + "`json:\"tags\"`" + `
	Portal    NullableString    ` + "`json:\"portal_url\"`" + `
	Value     interface{}       ` + "`json:\"value\"`" + `
	Owner     *string           ` + "`json:\"owner\"`" + `
	Note      *string           ` + "`json:\"note,omitempty\"`" + `
}

func (o *Example) UnmarshalJSON(data []byte) (err error) {
	requiredProperties := []string{
		"name",
		"tags",
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
`

func TestPatch(t *testing.T) {
	patched, err := patch([]byte(generated))
	if err != nil {
		t.Fatal(err)
	}
	want := "\t\t}\n\t}\n\n\t" + marker + ": it would decode as its zero value.\n" +
		"\tfor _, requiredProperty := range []string{\"name\", \"tags\"} {\n" +
		"\t\tif allProperties[requiredProperty] == nil {\n" +
		"\t\t\treturn fmt.Errorf(\"required property %v is null\", requiredProperty)\n" +
		"\t\t}\n\t}\n\n\treturn nil\n"
	if !strings.Contains(string(patched), want) {
		t.Fatalf("patched:\n%s", patched)
	}
	again, err := patch(patched)
	if err != nil || string(again) != string(patched) {
		t.Fatalf("not idempotent: %v", err)
	}
}

func TestRawGeneratorOutput(t *testing.T) {
	// As openapi-generator writes it, before gofmt.
	raw := strings.Replace(generated, "range requiredProperties", "range(requiredProperties)", 1)
	patched, err := patch([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(patched), `range []string{"name", "tags"}`) {
		t.Fatalf("patched:\n%s", patched)
	}
}

func TestModelWithoutRequiredPropertiesIsLeftUnchanged(t *testing.T) {
	source := strings.Replace(generated, `"name",
		"tags",
		"portal_url",
		"value",
		"owner",`, `"portal_url",`, 1)
	patched, err := patch([]byte(source))
	if err != nil || string(patched) != source {
		t.Fatalf("changed a model whose required properties all hold null: %v\n%s", err, patched)
	}
}
