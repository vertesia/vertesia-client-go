package vertesia

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequiredVersionPatchIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "api_example.go")
	source := "\tif r.xApiVersion == nil {\n\t\treturn result, nil, errors.New(\"xApiVersion is required and must be specified\")\n\t}\n"
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	var first string
	for i := 0; i < 2; i++ {
		cmd := exec.Command("bash", "scripts/patch-openapi-permissive-decode.sh", dir)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("patch: %v: %s", err, output)
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		patched := string(data)
		if !strings.Contains(patched, `version := a.client.cfg.DefaultHeader["x-api-version"]`) ||
			!strings.Contains(patched, `if version == "" {`) ||
			!strings.Contains(patched, `r.xApiVersion = &version`) ||
			!strings.Contains(patched, `xApiVersion is required and must be specified`) {
			t.Fatalf("missing fallback or required-header guard: %s", patched)
		}
		if i == 1 && patched != first {
			t.Fatal("patch is not idempotent")
		}
		first = patched
	}
}
