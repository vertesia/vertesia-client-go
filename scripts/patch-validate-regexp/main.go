// Command patch-validate-regexp makes the generated `validate:"regexp=..."` tags match what the spec
// patterns allow.
//
// openapi-generator writes each pattern into the tag with every backslash doubled and every slash
// escaped, but gopkg.in/validator.v2 compiles the tag parameter as it is: `^[a-z]+(?:\.[a-z]+)*$`
// becomes a pattern that wants a literal backslash, and rejects "document.title". validator.v2 also
// splits its parameters on commas, so a quantifier such as {1,64} makes every value fail with
// "unknown tag". After this patch each tag holds the spec pattern, with \uXXXX written as \x{XXXX}
// (Go's regexp has no \u) and commas escaped for validator.v2. A pattern that does not compile, or that
// validator.v2 would not read back unchanged, fails the patch.
//
// A patched file ends with a marker comment, so that running the patch again changes nothing.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	marker = "// The validate regexp tags of this file were unescaped by scripts/patch-validate-regexp.\n"
	prefix = "regexp="
)

var (
	validateTag = regexp.MustCompile(`validate:"(?:[^"\\]|\\.)*"`)
	// validator.v2 splits its parameters on a comma preceded by an even number of backslashes.
	separator = regexp.MustCompile(`((?:^|[^\\])(?:\\\\)*),`)
)

// specPattern undoes the generator's escaping: a doubled backslash is one backslash, and an escaped
// slash is a slash.
func specPattern(generated string) (string, error) {
	var pattern strings.Builder
	for i := 0; i < len(generated); i++ {
		if generated[i] != '\\' {
			pattern.WriteByte(generated[i])
			continue
		}
		if i+1 == len(generated) || (generated[i+1] != '\\' && generated[i+1] != '/') {
			return "", fmt.Errorf("unexpected escape in generated pattern %q", generated)
		}
		i++
		pattern.WriteByte(generated[i])
	}
	return pattern.String(), nil
}

// goPattern rewrites \uXXXX, which Go's regexp does not accept, as \x{XXXX}.
func goPattern(pattern string) string {
	var out strings.Builder
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '\\' || i+1 == len(pattern) {
			out.WriteByte(pattern[i])
			continue
		}
		if pattern[i+1] == 'u' && i+6 <= len(pattern) {
			if _, err := strconv.ParseUint(pattern[i+2:i+6], 16, 16); err == nil {
				out.WriteString(`\x{` + pattern[i+2:i+6] + `}`)
				i += 5
				continue
			}
		}
		out.WriteString(pattern[i : i+2])
		i++
	}
	return out.String()
}

// validatorParam returns the parameter validator.v2 reads from a tag value.
func validatorParam(value string) (string, bool) {
	parts := []string{}
	last := 0
	for _, match := range separator.FindAllStringIndex(value, -1) {
		parts = append(parts, value[last:match[1]-1])
		last = match[1]
	}
	parts = append(parts, value[last:])
	if len(parts) != 1 || !strings.HasPrefix(parts[0], prefix) {
		return "", false
	}
	return strings.TrimSpace(strings.ReplaceAll(parts[0], `\,`, ",")[len(prefix):]), true
}

// tagValue returns the validate tag value that makes validator.v2 compile pattern.
func tagValue(generated string) (string, error) {
	pattern, err := specPattern(generated)
	if err != nil {
		return "", err
	}
	pattern = goPattern(pattern)
	if _, err := regexp.Compile(pattern); err != nil {
		return "", fmt.Errorf("pattern %q does not compile: %w", pattern, err)
	}
	value := prefix + strings.ReplaceAll(pattern, ",", `\,`)
	if param, ok := validatorParam(value); !ok || param != pattern {
		return "", fmt.Errorf("validator.v2 would not read back pattern %q", pattern)
	}
	return value, nil
}

type edit struct {
	start, end int
	text       string
}

func patchFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if strings.Contains(string(data), marker) {
		return nil
	}
	fset := token.NewFileSet()
	tree, err := parser.ParseFile(fset, path, data, parser.ParseComments)
	if err != nil {
		return err
	}
	var edits []edit
	var failure error
	ast.Inspect(tree, func(n ast.Node) bool {
		field, ok := n.(*ast.Field)
		if !ok || field.Tag == nil || failure != nil {
			return failure == nil
		}
		raw, err := strconv.Unquote(field.Tag.Value)
		if err != nil || !strings.HasPrefix(field.Tag.Value, "`") {
			return true
		}
		current, ok := reflect.StructTag(raw).Lookup("validate")
		if !ok || !strings.HasPrefix(current, prefix) {
			return true
		}
		value, err := tagValue(current[len(prefix):])
		if err != nil {
			failure = fmt.Errorf("%s: %w", fset.Position(field.Pos()), err)
			return false
		}
		quoted := strconv.Quote(value)
		updated := validateTag.ReplaceAllLiteralString(raw, "validate:"+quoted)
		if strings.Contains(updated, "`") || reflect.StructTag(updated).Get("validate") != value {
			failure = fmt.Errorf("%s: cannot write tag for %q", fset.Position(field.Pos()), value)
			return false
		}
		start, end := fset.Position(field.Tag.Pos()).Offset, fset.Position(field.Tag.End()).Offset
		edits = append(edits, edit{start, end, "`" + updated + "`"})
		return true
	})
	if failure != nil {
		return failure
	}
	if len(edits) == 0 {
		return nil
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	output := append([]byte{}, data...)
	for _, e := range edits {
		output = append(output[:e.start:e.start], append([]byte(e.text), output[e.end:]...)...)
	}
	output, err = format.Source(append(output, "\n"+marker...))
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return os.WriteFile(path, output, 0644)
}

func main() {
	dir := flag.String("dir", "", "generated openapi package directory")
	flag.Parse()
	if *dir == "" {
		flag.Usage()
		os.Exit(2)
	}
	models, err := filepath.Glob(filepath.Join(*dir, "model_*.go"))
	if err == nil {
		for _, model := range models {
			if err = patchFile(model); err != nil {
				break
			}
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
