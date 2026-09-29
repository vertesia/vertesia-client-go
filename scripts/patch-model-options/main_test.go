package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFixture(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMissingGeneratedBranchDoesNotWritePartialPatch(t *testing.T) {
	dir := t.TempDir()
	spec := writeFixture(t, dir, "spec.json", `{"components":{"schemas":{
 "ModelOptions":{"anyOf":[{"$ref":"#/components/schemas/ExampleOptions"}]},
 "ExampleOptions":{"properties":{"_option_id":{"const":"example"}}}
 }}}`)
	original := `package example
import "encoding/json"
type ModelOptions struct { ExampleOptions *ExampleOptions }
func (v *ModelOptions) UnmarshalJSON(data []byte) error { return nil }
func (v ModelOptions) MarshalJSON() ([]byte,error) { return json.Marshal(nil) }
`
	model := writeFixture(t, dir, "model_model_options.go", original)
	err := run(model, spec)
	if err == nil || !strings.Contains(err.Error(), "missing generated branch files") {
		t.Fatalf("expected missing-branch error, got %v", err)
	}
	actual, err := os.ReadFile(model)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != original {
		t.Fatal("failed transformation wrote a partial patch")
	}
}

func TestBranchTransformationIgnoresLayoutAndRetainsComments(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "model_example_options.go", `package example
import "encoding/json"
// ExampleOptions is generated.
type ExampleOptions struct {
 // Keep this field documentation.
 Value *float32 `+"`json:\"value,omitempty\"`"+`
}
func (
 o ExampleOptions,
) ToMap() (map[string]interface{},error) {
 toSerialize:=map[string]interface{}{}
 if o.Value!=nil {toSerialize["value"]=o.Value}
 return toSerialize,nil
}
`)
	missingValidator, err := readSource(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = patchBranch(missingValidator, "ExampleOptions", true); err == nil {
		t.Fatal("accepted missing required-field validator")
	}
	// Go permits a trailing comma in the receiver parameter list.
	first, err := readSource(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = patchBranch(first, "ExampleOptions", false); err != nil {
		t.Fatal(err)
	}
	output, err := first.result()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output, []byte("Keep this field documentation.")) {
		t.Fatal("field documentation was lost")
	}
	if err = os.WriteFile(path, output, 0600); err != nil {
		t.Fatal(err)
	}
	second, err := readSource(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = patchBranch(second, "ExampleOptions", false); err != nil {
		t.Fatal(err)
	}
	again, err := second.result()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output, again) {
		t.Fatal("reformatted branch patch is not idempotent")
	}
}

func TestDuplicateSchemaIDIsRejected(t *testing.T) {
	dir := t.TempDir()
	spec := writeFixture(t, dir, "spec.json", `{"components":{"schemas":{
 "ModelOptions":{"anyOf":[{"$ref":"#/components/schemas/First"},{"$ref":"#/components/schemas/Second"}]},
 "First":{"properties":{"_option_id":{"const":"same"}}},
 "Second":{"properties":{"_option_id":{"enum":["same"]}}}
 }}}`)
	if _, err := loadBranches(spec); err == nil {
		t.Fatal("accepted duplicate family ID")
	}
}
