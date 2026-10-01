#!/usr/bin/env bash
set -euo pipefail
repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT
mkdir -p "$work_dir/openapi" "$work_dir/spec"
cat > "$work_dir/spec/vertesia-openapi.json" <<'JSON'
{"components":{"schemas":{"ExperimentalAgentConversationSourceDescriptor":{"oneOf":[{"$ref":"#/components/schemas/ExperimentalAgentConversationSourceUninitialized"},{"$ref":"#/components/schemas/ExperimentalAgentConversationSourceInitialized"}],"discriminator":{"propertyName":"status","mapping":{"uninitialized":"#/components/schemas/ExperimentalAgentConversationSourceUninitialized","initialized":"#/components/schemas/ExperimentalAgentConversationSourceInitialized"}}}}}}
JSON
cat > "$work_dir/openapi/model_experimental_agent_conversation_source_descriptor.go" <<'GO'
package fixture
import (
    "encoding/json"
    "fmt"
	validator "gopkg.in/validator.v2"
)
type ExperimentalAgentConversationSourceUninitialized struct { Status string `json:"status"` }
type ExperimentalAgentConversationSourceInitialized struct { Status string `json:"status"`; ConversationID string `json:"conversation_id"` }
type ExperimentalAgentConversationSourceDescriptor struct { ExperimentalAgentConversationSourceUninitialized *ExperimentalAgentConversationSourceUninitialized; ExperimentalAgentConversationSourceInitialized *ExperimentalAgentConversationSourceInitialized }
func (dst *ExperimentalAgentConversationSourceDescriptor) UnmarshalJSON(data []byte) error { return validator.Validate(dst) }
var _ = json.Unmarshal
var _ = fmt.Errorf
GO
for model in ExperimentalCanonicalInteractionExecutionRequest ExperimentalCanonicalNamedInteractionExecutionRequest; do
  snake="$(printf '%s' "$model" | sed -E 's/([a-z0-9])([A-Z])/@\1_\2/g; s/@//g' | tr '[:upper:]' '[:lower:]')"
  cat > "$work_dir/openapi/model_${snake}.go" <<GO
package fixture
import "encoding/json"
type ${model} struct {
    Data interface{} \`json:"data,omitempty"\`
}
type _${model} ${model}
func (o *${model}) GetDataOk() (*interface{}, bool) { if o == nil || IsNil(o.Data) { return nil, false }; return &o.Data, true }
func (o *${model}) HasData() bool { return o != nil && !IsNil(o.Data) }
func (o *${model}) SetData(v interface{}) { o.Data = v }
func (o ${model}) MarshalJSON() ([]byte,error) { m,err:=o.ToMap(); if err!=nil{return nil,err}; return json.Marshal(m) }
func (o ${model}) ToMap() (map[string]interface{},error) {
    toSerialize:=map[string]interface{}{}
    if !IsNil(o.Data) {
        toSerialize["data"] = o.Data
    }
    return toSerialize,nil
}
func (o *${model}) UnmarshalJSON(data []byte) (err error) {
    allProperties:=map[string]interface{}{}
    if err=json.Unmarshal(data,&allProperties);err!=nil{return err}
    var${model}:=_${model}{}
    if err=json.Unmarshal(data,&var${model});err!=nil{return err}
    *o = ${model}(var${model})
    return nil
}
GO
done
cat > "$work_dir/openapi/utils.go" <<'GO'
package fixture
import "reflect"
func IsNil(i interface{}) bool { if i==nil{return true}; v:=reflect.ValueOf(i); return (v.Kind()==reflect.Ptr || v.Kind()==reflect.Map || v.Kind()==reflect.Slice || v.Kind()==reflect.Interface) && v.IsNil() }
GO
cat > "$work_dir/openapi/patch_test.go" <<'GO'
package fixture

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSourceDiscriminatorAndReset(t *testing.T) {
	var descriptor ExperimentalAgentConversationSourceDescriptor
	if err := json.Unmarshal([]byte(`{"status":"initialized","conversation_id":"c"}`), &descriptor); err != nil {
		t.Fatal(err)
	}
	if descriptor.ExperimentalAgentConversationSourceInitialized == nil {
		t.Fatal("initialized branch not selected")
	}
	if err := json.Unmarshal([]byte(`{"status":"uninitialized"}`), &descriptor); err != nil {
		t.Fatal(err)
	}
	if descriptor.ExperimentalAgentConversationSourceInitialized != nil ||
		descriptor.ExperimentalAgentConversationSourceUninitialized == nil {
		t.Fatalf("stale branch after reuse: %#v", descriptor)
	}
	if err := json.Unmarshal([]byte(`{"status":"future"}`), &descriptor); err == nil {
		t.Fatal("unknown status accepted")
	}
}

type dataAccessors interface {
	HasData() bool
	GetDataOk() (*interface{}, bool)
}

func assertDataRoundTrip(t *testing.T, target dataAccessors, body string) {
	t.Helper()
	if err := json.Unmarshal([]byte(body), target); err != nil {
		t.Fatal(err)
	}
	serialized, err := json.Marshal(target)
	if err != nil {
		t.Fatal(err)
	}
	var before, after map[string]interface{}
	if err := json.Unmarshal([]byte(body), &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(serialized, &after); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("%s => %s", body, serialized)
	}
	_, present := target.GetDataOk()
	if present != target.HasData() {
		t.Fatal("accessors disagree")
	}
	_, wantPresent := before["data"]
	if present != wantPresent {
		t.Fatalf("presence=%v want=%v", present, wantPresent)
	}
}

func TestDataOmittedNullAndRoots(t *testing.T) {
	bodies := []string{
		`{}`,
		`{"data":null}`,
		`{"data":false}`,
		`{"data":7}`,
		`{"data":"x"}`,
		`{"data":[null,{"adapter":"a"}]}`,
		`{"data":{"protocol":"p"}}`,
	}
	for _, body := range bodies {
		assertDataRoundTrip(t, &ExperimentalCanonicalInteractionExecutionRequest{}, body)
		assertDataRoundTrip(t, &ExperimentalCanonicalNamedInteractionExecutionRequest{}, body)
	}

	request := &ExperimentalCanonicalInteractionExecutionRequest{}
	request.SetData(nil)
	if !request.HasData() {
		t.Fatal("SetData(nil) lost presence")
	}
	value, ok := request.GetDataOk()
	if !ok || value == nil || *value != nil {
		t.Fatal("GetDataOk explicit null is incoherent")
	}
}

func TestDataDirectAssignmentAndDecodeReset(t *testing.T) {
	for _, value := range []interface{}{false, float64(0), "", []interface{}{}, map[string]interface{}{}} {
		request := ExperimentalCanonicalInteractionExecutionRequest{Data: value}
		if !request.HasData() {
			t.Fatalf("direct assignment %#v not set", value)
		}
		serialized, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		var decoded map[string]interface{}
		if err := json.Unmarshal(serialized, &decoded); err != nil {
			t.Fatal(err)
		}
		if _, ok := decoded["data"]; !ok {
			t.Fatalf("direct assignment omitted: %s", serialized)
		}
	}

	var request ExperimentalCanonicalInteractionExecutionRequest
	if err := json.Unmarshal([]byte(`{"data":null}`), &request); err != nil {
		t.Fatal(err)
	}
	if !request.HasData() {
		t.Fatal("explicit null missing")
	}
	if err := json.Unmarshal([]byte(`{}`), &request); err != nil {
		t.Fatal(err)
	}
	if request.HasData() || request.Data != nil {
		t.Fatalf("omission did not reset: %#v", request)
	}

	request.SetData("stale")
	if err := request.UnmarshalJSON([]byte(`{`)); err == nil {
		t.Fatal("malformed JSON accepted")
	}
	if request.HasData() || request.Data != nil {
		t.Fatalf("failed decode retained data: %#v", request)
	}
}
GO
printf 'module fixture\n\ngo 1.24\n\nrequire gopkg.in/validator.v2 v2.0.1\n' > "$work_dir/go.mod"
cp "$repo_dir/go.sum" "$work_dir/go.sum"
bash "$repo_dir/scripts/patch-openapi-permissive-decode.sh" "$work_dir/openapi"
gofmt -w "$work_dir/openapi"
(
  cd "$work_dir/openapi"
  find . -type f -print0 | sort -z | xargs -0 sha256sum > "$work_dir/first-patch.sha256"
)
bash "$repo_dir/scripts/patch-openapi-permissive-decode.sh" "$work_dir/openapi"
gofmt -w "$work_dir/openapi"
(
  cd "$work_dir/openapi"
  find . -type f -print0 | sort -z | xargs -0 sha256sum > "$work_dir/second-patch.sha256"
)
if ! cmp -s "$work_dir/first-patch.sha256" "$work_dir/second-patch.sha256"; then
  echo 'post-generation patch changed files when reapplied after gofmt' >&2
  diff -u "$work_dir/first-patch.sha256" "$work_dir/second-patch.sha256" >&2 || true
  exit 1
fi
(cd "$work_dir" && go test ./...)
