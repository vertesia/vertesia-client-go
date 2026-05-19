package vertesia_test

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"
)

func TestGeneratorPackageVersionMatchesOpenAPISpec(t *testing.T) {
	specData, err := os.ReadFile("spec/vertesia-openapi.json")
	if err != nil {
		t.Fatalf("read OpenAPI spec failed: %v", err)
	}

	var spec struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := json.Unmarshal(specData, &spec); err != nil {
		t.Fatalf("parse OpenAPI spec failed: %v", err)
	}
	if spec.Info.Version == "" {
		t.Fatal("OpenAPI spec info.version is empty")
	}

	configData, err := os.ReadFile("openapi-generator-config.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI generator config failed: %v", err)
	}
	match := regexp.MustCompile(`(?m)^\s*packageVersion:\s*"?([^"\s]+)"?\s*$`).FindSubmatch(configData)
	if match == nil {
		t.Fatal("openapi-generator-config.yaml packageVersion is missing")
	}
	configVersion := string(match[1])
	if configVersion != spec.Info.Version {
		t.Fatalf("generator packageVersion = %q, want OpenAPI spec info.version %q", configVersion, spec.Info.Version)
	}
}
