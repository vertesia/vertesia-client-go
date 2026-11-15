package main

import "testing"

func TestTagValue(t *testing.T) {
	for generated, want := range map[string]string{
		// As openapi-generator writes the spec patterns, after the struct tag is unquoted.
		`^[A-Za-z0-9_]+(?:\\.[A-Za-z0-9_]+)*$`: `regexp=^[A-Za-z0-9_]+(?:\.[A-Za-z0-9_]+)*$`,
		`^[A-Za-z0-9_-]{1,64}$`:                `regexp=^[A-Za-z0-9_-]{1\,64}$`,
		`^audio\/.*`:                           `regexp=^audio/.*`,
		`[^\\\\\/]`:                            `regexp=[^\\/]`,
		`^[\\u0020\\u00a0]*\\S`:                `regexp=^[\x{0020}\x{00a0}]*\S`,
	} {
		got, err := tagValue(generated)
		if err != nil || got != want {
			t.Errorf("tagValue(%q) = %q, %v; want %q", generated, got, err, want)
		}
		if param, ok := validatorParam(got); !ok || param == "" {
			t.Errorf("validator.v2 cannot read %q", got)
		}
	}
	for _, generated := range []string{`\.`, `(`} {
		if _, err := tagValue(generated); err == nil {
			t.Errorf("tagValue(%q) accepted an unexpected pattern", generated)
		}
	}
}
