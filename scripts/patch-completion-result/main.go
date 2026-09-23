// Patch CompletionResult to dispatch by the canonical wire discriminator.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
)

func run(modelPath, specPath string) error {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return err
	}
	type schema struct {
		OneOf []struct {
			Ref string `json:"$ref"`
		} `json:"oneOf"`
		Required   []string `json:"required"`
		Properties map[string]struct {
			Const json.RawMessage   `json:"const"`
			Enum  []json.RawMessage `json:"enum"`
		} `json:"properties"`
		Discriminator struct {
			PropertyName string            `json:"propertyName"`
			Mapping      map[string]string `json:"mapping"`
		} `json:"discriminator"`
	}
	var spec struct {
		Components struct {
			Schemas map[string]schema `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		return err
	}
	union := spec.Components.Schemas["CompletionResult"]
	if union.Discriminator.PropertyName != "type" || len(union.OneOf) == 0 {
		return fmt.Errorf("missing CompletionResult type discriminator")
	}
	data, err = os.ReadFile(modelPath)
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	tree, err := parser.ParseFile(fset, modelPath, data, parser.ParseComments)
	if err != nil {
		return err
	}
	fields := map[string]string{}
	var decode *ast.FuncDecl
	for _, decl := range tree.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "UnmarshalJSON" && fn.Recv != nil && len(fn.Recv.List) == 1 {
			if ptr, ok := fn.Recv.List[0].Type.(*ast.StarExpr); ok {
				if id, ok := ptr.X.(*ast.Ident); ok && id.Name == "CompletionResult" {
					decode = fn
				}
			}
		}
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.TYPE {
			for _, item := range gen.Specs {
				typ := item.(*ast.TypeSpec)
				if typ.Name.Name != "CompletionResult" {
					continue
				}
				st, ok := typ.Type.(*ast.StructType)
				if !ok {
					return fmt.Errorf("unexpected CompletionResult declaration")
				}
				for _, field := range st.Fields.List {
					ptr, ok := field.Type.(*ast.StarExpr)
					if !ok || len(field.Names) != 1 {
						return fmt.Errorf("unexpected CompletionResult field")
					}
					id, ok := ptr.X.(*ast.Ident)
					if !ok {
						return fmt.Errorf("unexpected CompletionResult branch")
					}
					fields[id.Name] = field.Names[0].Name
				}
			}
		}
	}
	if decode == nil || len(fields) != len(union.OneOf) || len(union.Discriminator.Mapping) != len(fields) {
		return fmt.Errorf("generated CompletionResult does not match spec")
	}
	var body strings.Builder
	body.WriteString("func (dst *CompletionResult) UnmarshalJSON(data []byte) error {\n*dst = CompletionResult{}\nvar tag struct { Type string `json:\"type\"` }; if err := json.Unmarshal(data, &tag); err != nil { return err }; switch tag.Type {\n")
	seen := map[string]bool{}
	for _, ref := range union.OneOf {
		name := strings.TrimPrefix(ref.Ref, "#/components/schemas/")
		member, ok := spec.Components.Schemas[name]
		if !ok || fields[name] == "" {
			return fmt.Errorf("unknown CompletionResult branch %s", name)
		}
		tag := member.Properties["type"]
		literal := tag.Const
		if len(tag.Enum) == 1 {
			literal = tag.Enum[0]
		} else if len(tag.Enum) > 1 {
			return fmt.Errorf("nonliteral type in %s", name)
		}
		var id string
		if err := json.Unmarshal(literal, &id); err != nil {
			return fmt.Errorf("non-string discriminator in %s", name)
		}
		required := false
		for _, field := range member.Required {
			if field == "type" {
				required = true
			}
		}
		if !required || id == "" || seen[id] || union.Discriminator.Mapping[id] != ref.Ref {
			return fmt.Errorf("invalid discriminator mapping for %s", name)
		}
		seen[id] = true
		fmt.Fprintf(&body, "case %q: var value %s; if err := json.Unmarshal(data, &value); err != nil { return err }; dst.%s = &value; return nil\n", id, name, fields[name])
	}
	body.WriteString("default: return fmt.Errorf(\"unknown CompletionResult type %q\", tag.Type)\n}\n}")
	start, end := fset.Position(decode.Pos()).Offset, fset.Position(decode.End()).Offset
	output := append(append(append([]byte{}, data[:start]...), []byte(body.String())...), data[end:]...)
	// The old trial-and-error decoder alone used validator; retain fmt for errors.
	tree, err = parser.ParseFile(fset, modelPath, output, parser.ParseComments)
	if err != nil {
		return err
	}
	for _, imp := range tree.Imports {
		name, _ := strconv.Unquote(imp.Path.Value)
		if name == "gopkg.in/validator.v2" {
			start, end = fset.Position(imp.Pos()).Offset, fset.Position(imp.End()).Offset
			output = append(output[:start], output[end:]...)
			break
		}
	}
	output, err = format.Source(output)
	if err != nil {
		return err
	}
	return os.WriteFile(modelPath, output, 0644)
}

func main() {
	model := flag.String("model", "", "generated CompletionResult model")
	spec := flag.String("spec", "", "OpenAPI JSON specification")
	flag.Parse()
	if *model == "" || *spec == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*model, *spec); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
