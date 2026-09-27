package vertesia

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEndpointUnionGenerationPatch(t *testing.T) {
	generated := t.TempDir()
	if err := os.WriteFile(filepath.Join(generated, "go.mod"), []byte("module endpointpatch\n\ngo 1.23.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	models := `package endpointpatch

type SubjectEndpointRef struct { Kind string; Id string }
type DocumentEndpointRef struct { Kind string; Id string }
type DocumentVersionEndpointRef struct { Kind string; Id string }
type ExternalEndpointRef struct { Kind string; Namespace string; Value string }
type RelationshipContributionEntityEndpoint struct { Kind string; Key string }
`
	if err := os.WriteFile(filepath.Join(generated, "model_branches.go"), []byte(models), 0600); err != nil {
		t.Fatal(err)
	}
	for _, union := range []struct{ name, filename, fields string }{
		{"EndpointRef", "model_endpoint_ref.go", "SubjectEndpointRef *SubjectEndpointRef; DocumentEndpointRef *DocumentEndpointRef; DocumentVersionEndpointRef *DocumentVersionEndpointRef; ExternalEndpointRef *ExternalEndpointRef"},
		{"RelationshipContributionEndpoint", "model_relationship_contribution_endpoint.go", "RelationshipContributionEntityEndpoint *RelationshipContributionEntityEndpoint; SubjectEndpointRef *SubjectEndpointRef; DocumentEndpointRef *DocumentEndpointRef; DocumentVersionEndpointRef *DocumentVersionEndpointRef; ExternalEndpointRef *ExternalEndpointRef"},
	} {
		source := "package endpointpatch\nimport (\"encoding/json\"; \"fmt\")\n" +
			"type " + union.name + " struct { " + union.fields + " }\n" +
			"func (dst *" + union.name + ") UnmarshalJSON(data []byte) error { return nil\n}\n\n" +
			"// Marshal data from the first non-nil pointers\n" +
			"func (dst " + union.name + ") MarshalJSON() ([]byte, error) { return json.Marshal(nil) }\n" +
			"var _ = fmt.Errorf\n"
		if err := os.WriteFile(filepath.Join(generated, union.filename), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}

	patch := func() {
		t.Helper()
		cmd := exec.Command("bash", "scripts/patch-openapi-permissive-decode.sh", generated)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("patch failed: %v: %s", err, output)
		}
	}
	patch()
	first := map[string][]byte{}
	for _, name := range []string{"model_endpoint_ref.go", "model_relationship_contribution_endpoint.go"} {
		data, err := os.ReadFile(filepath.Join(generated, name))
		if err != nil {
			t.Fatal(err)
		}
		first[name] = data
	}
	patch()
	for name, before := range first {
		after, err := os.ReadFile(filepath.Join(generated, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Errorf("patch changed %s on second run", name)
		}
	}
	tests := `package endpointpatch
import ("encoding/json"; "testing")
func TestEndpointKinds(t *testing.T) {
    for _, input := range []string{
        "{\"kind\":\"subject\",\"id\":\"s\"}",
        "{\"kind\":\"document\",\"id\":\"d\"}",
        "{\"kind\":\"document_version\",\"id\":\"v\"}",
        "{\"kind\":\"external\",\"namespace\":\"n\",\"value\":\"x\"}",
    } {
        var endpoint EndpointRef
        if err := json.Unmarshal([]byte(input), &endpoint); err != nil { t.Fatal(err) }
        var contribution RelationshipContributionEndpoint
        if err := json.Unmarshal([]byte(input), &contribution); err != nil { t.Fatal(err) }
    }
    var entity RelationshipContributionEndpoint
    if err := json.Unmarshal([]byte("{\"kind\":\"entity\",\"key\":\"local-1\"}"), &entity); err != nil { t.Fatal(err) }
    if entity.RelationshipContributionEntityEndpoint == nil || entity.RelationshipContributionEntityEndpoint.Key != "local-1" { t.Fatal("entity branch not selected") }
    for _, input := range []string{
        "{\"kind\":\"entity\"}", "{\"kind\":\"subject\"}",
        "{\"kind\":\"external\",\"namespace\":\"n\"}",
        "{\"kind\":\"unknown\",\"id\":\"x\"}",
    } {
        var contribution RelationshipContributionEndpoint
        if err := json.Unmarshal([]byte(input), &contribution); err == nil { t.Fatalf("accepted invalid %s", input) }
    }
    var endpoint EndpointRef
    if err := json.Unmarshal([]byte("{\"kind\":\"entity\",\"key\":\"x\"}"), &endpoint); err == nil { t.Fatal("EndpointRef accepted entity") }
}
`
	if err := os.WriteFile(filepath.Join(generated, "endpoint_test.go"), []byte(tests), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = generated
	cmd.Env = append(os.Environ(), "GOCACHE=/tmp/vertesia-client-go-gocache")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated endpoint tests failed: %v: %s", err, strings.TrimSpace(string(output)))
	}
}
