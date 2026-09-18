package vertesia

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Exercise the post-generation patch without changing committed generated files.
func TestModelOptionsGenerationPatch(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"openapi", "spec"} {
		if err := os.CopyFS(filepath.Join(dir, name), os.DirFS(name)); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"go.mod", "go.sum"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	generated := filepath.Join(dir, "openapi")
	first := make(map[string][]byte)
	for i := 0; i < 2; i++ {
		cmd := exec.Command("bash", "scripts/patch-openapi-permissive-decode.sh", generated)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("patch: %v: %s", err, output)
		}
		models, err := filepath.Glob(filepath.Join(generated, "model_*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, model := range models {
			patched, err := os.ReadFile(model)
			if err != nil {
				t.Fatal(err)
			}
			if i == 1 && !bytes.Equal(first[model], patched) {
				t.Fatalf("patch is not idempotent for %s", filepath.Base(model))
			}
			first[model] = patched
		}
	}
	tests, err := os.ReadFile("testdata/model_options_test.go")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(generated, "model_options_test.go"), tests, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "test", "./openapi")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated model tests: %v: %s", err, output)
	}
}
