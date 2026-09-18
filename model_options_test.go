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
	var first []byte
	for i := 0; i < 2; i++ {
		cmd := exec.Command("bash", "scripts/patch-openapi-permissive-decode.sh", generated)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("patch: %v: %s", err, output)
		}
		patched, err := os.ReadFile(filepath.Join(generated, "model_model_options.go"))
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 && !bytes.Equal(first, patched) {
			t.Fatal("ModelOptions patch is not idempotent")
		}
		first = patched
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
