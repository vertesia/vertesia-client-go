package vertesia_test

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"
)

var specVersionPattern = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(-.+)?$`)

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
	wantVersion, ok := goPackageVersionFromSpec(spec.Info.Version)
	if !ok {
		t.Fatalf("unsupported OpenAPI spec info.version %q (want X.Y.Z[-prerelease])", spec.Info.Version)
	}
	if configVersion != wantVersion {
		t.Fatalf(
			"generator packageVersion = %q, want %q for OpenAPI spec info.version %q",
			configVersion,
			wantVersion,
			spec.Info.Version,
		)
	}
}

func TestGoPackageVersionFromSpec(t *testing.T) {
	tests := []struct {
		name        string
		specVersion string
		wantVersion string
	}{
		{name: "release", specVersion: "1.5.0", wantVersion: "1.5.0"},
		{name: "dev prerelease", specVersion: "1.5.0-dev", wantVersion: "1.5.0"},
		{name: "timestamped dev prerelease", specVersion: "1.5.0-dev.20260615.051508Z", wantVersion: "1.5.0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotVersion, ok := goPackageVersionFromSpec(test.specVersion)
			if !ok {
				t.Fatalf("goPackageVersionFromSpec(%q) rejected a supported version", test.specVersion)
			}
			if gotVersion != test.wantVersion {
				t.Fatalf("goPackageVersionFromSpec(%q) = %q, want %q", test.specVersion, gotVersion, test.wantVersion)
			}
		})
	}
}

func TestGoPackageVersionFromSpecRejectsUnsupportedVersions(t *testing.T) {
	for _, specVersion := range []string{"", "1.5", "v1.5.0", "1.5.0+build"} {
		t.Run(specVersion, func(t *testing.T) {
			if gotVersion, ok := goPackageVersionFromSpec(specVersion); ok {
				t.Fatalf("goPackageVersionFromSpec(%q) = %q, want rejection", specVersion, gotVersion)
			}
		})
	}
}

func goPackageVersionFromSpec(specVersion string) (string, bool) {
	match := specVersionPattern.FindStringSubmatch(specVersion)
	if match == nil {
		return "", false
	}
	return match[1] + "." + match[2] + "." + match[3], true
}
