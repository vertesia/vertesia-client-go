package main

import (
	"strings"
	"testing"
)

// As openapi-generator 7.22.0 writes a model with a required nullable list, a required nullable
// map, an optional nullable list and an optional pointer.
const generated = `package openapi

type Payload struct {
	Required []string ` + "`json:\"required_list\"`" + `
	Input map[string]interface{} ` + "`json:\"tool_input\"`" + `
	Optional []string ` + "`json:\"optional_list,omitempty\"`" + `
	Signature *string ` + "`json:\"signature,omitempty\"`" + `
}

func (o Payload) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	if o.Required != nil {
		toSerialize["required_list"] = o.Required
	}
	if o.Input != nil {
		toSerialize["tool_input"] = o.Input
	}
	if o.Optional != nil {
		toSerialize["optional_list"] = o.Optional
	}
	if !IsNil(o.Signature) {
		toSerialize["signature"] = o.Signature
	}
	return toSerialize, nil
}

// Not a ToMap method: left alone.
func (o Payload) other() map[string]interface{} {
	toSerialize := map[string]interface{}{}
	if o.Required != nil {
		toSerialize["required_list"] = o.Required
	}
	return toSerialize
}
`

func TestPatchSource(t *testing.T) {
	patched, changed, err := patchSource("model_payload.go", []byte(generated))
	if err != nil || !changed {
		t.Fatalf("patchSource: changed=%v err=%v", changed, err)
	}
	toMap, other, _ := strings.Cut(string(patched), "// Not a ToMap method")
	for _, want := range []string{
		"toSerialize := map[string]interface{}{}\n\ttoSerialize[\"required_list\"] = o.Required\n" +
			"\ttoSerialize[\"tool_input\"] = o.Input\n\tif o.Optional != nil {",
		"if !IsNil(o.Signature) {",
	} {
		if !strings.Contains(toMap, want) {
			t.Errorf("ToMap lacks %q:\n%s", want, toMap)
		}
	}
	for _, guard := range []string{"if o.Required != nil", "if o.Input != nil"} {
		if strings.Contains(toMap, guard) {
			t.Errorf("ToMap kept %q:\n%s", guard, toMap)
		}
	}
	if !strings.Contains(other, "if o.Required != nil") {
		t.Errorf("patched a method other than ToMap:\n%s", other)
	}

	again, changed, err := patchSource("model_payload.go", patched)
	if err != nil || changed || string(again) != string(patched) {
		t.Fatalf("patch is not idempotent: changed=%v err=%v", changed, err)
	}
}

func TestPatchSourceKeepsMismatchedGuards(t *testing.T) {
	for name, body := range map[string]string{
		"other key":   "if o.Required != nil {\n\t\ttoSerialize[\"other\"] = o.Required\n\t}",
		"other value": "if o.Required != nil {\n\t\ttoSerialize[\"required_list\"] = o.Input\n\t}",
		"two writes": "if o.Required != nil {\n\t\ttoSerialize[\"required_list\"] = o.Required\n" +
			"\t\ttoSerialize[\"tool_input\"] = o.Input\n\t}",
		"else branch": "if o.Required != nil {\n\t\ttoSerialize[\"required_list\"] = o.Required\n" +
			"\t} else {\n\t\ttoSerialize[\"tool_input\"] = o.Input\n\t}",
		"other receiver": "if p := o; p.Required != nil {\n\t\ttoSerialize[\"required_list\"] = p.Required\n\t}",
	} {
		src := "package openapi\n\ntype Payload struct {\n\tRequired []string `json:\"required_list\"`\n" +
			"\tInput map[string]interface{} `json:\"tool_input\"`\n}\n\n" +
			"func (o Payload) ToMap() (map[string]interface{}, error) {\n" +
			"\ttoSerialize := map[string]interface{}{}\n\t" + body + "\n\treturn toSerialize, nil\n}\n"
		if _, changed, err := patchSource("model_payload.go", []byte(src)); err != nil || changed {
			t.Errorf("%s: changed=%v err=%v", name, changed, err)
		}
	}
}
