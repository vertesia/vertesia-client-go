// Command patch-unions makes the generated oneOf/anyOf models forward compatible.
//
// A newer server can return a union variant this client was generated without. The generated
// decoders fail on it, and one unknown element fails the whole response. After this patch:
//
//   - every union model gets a Raw json.RawMessage field that holds such a variant as the server
//     sent it; MarshalJSON re-encodes it unchanged and GetActualInstance returns it;
//   - a union with a published discriminator dispatches on that property: a known value decodes
//     into its typed branch (and fails if the branch is invalid), an unknown value goes to Raw, and
//     a missing value is an error. The generated decoder tries every branch and fails when several
//     match, which happens whenever branches share their required fields;
//   - any other union keeps its generated decoder, but when that decoder cannot pick a branch the
//     payload goes to Raw instead of failing.
//
// The patch locates declarations with go/ast, replaces only their source spans, and is idempotent.
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
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	rawField = "Raw"
	// The generated MarshalJSON of every oneOf and anyOf model carries this comment.
	unionMarker = "Marshal data from the first non-nil pointers in the struct to JSON"
	// The generated decoder of a non-discriminated union, kept under this name behind the wrapper.
	variantDecoder = "unmarshalVariantJSON"
)

type schema struct {
	Ref        string            `json:"$ref"`
	OneOf      []schema          `json:"oneOf"`
	Required   []string          `json:"required"`
	Const      json.RawMessage   `json:"const"`
	Enum       []json.RawMessage `json:"enum"`
	Properties map[string]schema `json:"properties"`
	// A pointer so that a schema without a discriminator decodes to nil.
	Discriminator *struct {
		PropertyName string            `json:"propertyName"`
		Mapping      map[string]string `json:"mapping"`
	} `json:"discriminator"`
}

type branch struct{ id, typ, field string }

type edit struct {
	start, end int
	text       string
}

type source struct {
	path  string
	data  []byte
	fset  *token.FileSet
	tree  *ast.File
	edits []edit
}

func (s *source) offset(pos token.Pos) int { return s.fset.Position(pos).Offset }
func (s *source) text(n ast.Node) string   { return string(s.data[s.offset(n.Pos()):s.offset(n.End())]) }
func (s *source) insert(pos token.Pos, text string) {
	s.edits = append(s.edits, edit{s.offset(pos), s.offset(pos), text})
}
func (s *source) replace(n ast.Node, text string) {
	s.edits = append(s.edits, edit{s.offset(n.Pos()), s.offset(n.End()), text})
}

func receiverType(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) != 1 {
		return ""
	}
	typ := fn.Recv.List[0].Type
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	if id, ok := typ.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

func (s *source) method(typ, name string) *ast.FuncDecl {
	for _, decl := range s.tree.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == name && receiverType(fn) == typ {
			return fn
		}
	}
	return nil
}

// unions returns the union structs of the file, by name.
func (s *source) unions() map[string]*ast.StructType {
	found := map[string]*ast.StructType{}
	for _, decl := range s.tree.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "MarshalJSON" || fn.Doc == nil || !strings.Contains(fn.Doc.Text(), unionMarker) {
			continue
		}
		name := receiverType(fn)
		for _, decl := range s.tree.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				if typ := spec.(*ast.TypeSpec); typ.Name.Name == name {
					if st, ok := typ.Type.(*ast.StructType); ok {
						found[name] = st
					}
				}
			}
		}
	}
	return found
}

// branches returns the typed branches of a union struct, by Go type name, and whether it already
// has the Raw field.
func (s *source) branches(name string, st *ast.StructType) (map[string]string, bool, error) {
	fields := map[string]string{}
	raw := false
	for _, field := range st.Fields.List {
		if len(field.Names) != 1 {
			return nil, false, fmt.Errorf("%s: unexpected %s field", s.path, name)
		}
		if field.Names[0].Name == rawField {
			if s.text(field.Type) != "json.RawMessage" {
				return nil, false, fmt.Errorf("%s: %s.%s is not a json.RawMessage", s.path, name, rawField)
			}
			raw = true
			continue
		}
		ptr, ok := field.Type.(*ast.StarExpr)
		if !ok {
			return nil, false, fmt.Errorf("%s: unexpected %s branch %s", s.path, name, field.Names[0].Name)
		}
		fields[s.text(ptr.X)] = field.Names[0].Name
	}
	return fields, raw, nil
}

func mentionsRaw(s *source, fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == rawField {
			found = true
		}
		return !found
	})
	return found
}

// discriminatorValue returns the single string value a branch publishes for the discriminator.
func discriminatorValue(schemas map[string]schema, member schema, property string) (string, error) {
	tag := member.Properties[property]
	for tag.Ref != "" {
		tag = schemas[strings.TrimPrefix(tag.Ref, "#/components/schemas/")]
	}
	literal := tag.Const
	if len(tag.Enum) == 1 {
		literal = tag.Enum[0]
	} else if len(tag.Enum) > 1 {
		return "", fmt.Errorf("nonliteral %s", property)
	}
	var id string
	if err := json.Unmarshal(literal, &id); err != nil || id == "" {
		return "", fmt.Errorf("non-string %s", property)
	}
	return id, nil
}

// dispatch returns the discriminator property and branches of a discriminated union schema.
func dispatch(schemas map[string]schema, union string, fields map[string]string) (string, []branch, error) {
	unionSchema := schemas[union]
	property := unionSchema.Discriminator.PropertyName
	if property == "" || len(unionSchema.OneOf) == 0 || len(fields) != len(unionSchema.OneOf) ||
		len(unionSchema.Discriminator.Mapping) != len(fields) {
		return "", nil, fmt.Errorf("generated %s does not match spec", union)
	}
	var branches []branch
	seen := map[string]bool{}
	for _, ref := range unionSchema.OneOf {
		name := strings.TrimPrefix(ref.Ref, "#/components/schemas/")
		member, ok := schemas[name]
		if !ok || fields[name] == "" {
			return "", nil, fmt.Errorf("unknown %s branch %s", union, name)
		}
		id, err := discriminatorValue(schemas, member, property)
		if err != nil {
			return "", nil, fmt.Errorf("%s in %s", err, name)
		}
		required := false
		for _, field := range member.Required {
			required = required || field == property
		}
		if !required || seen[id] || unionSchema.Discriminator.Mapping[id] != ref.Ref {
			return "", nil, fmt.Errorf("invalid discriminator mapping for %s", name)
		}
		seen[id] = true
		branches = append(branches, branch{id, name, fields[name]})
	}
	return property, branches, nil
}

func keepRaw(variable string) string {
	return "// A variant added after this client was generated: keep it as the server sent it.\n" +
		variable + ".Raw = append(json.RawMessage(nil), data...)\nreturn nil\n"
}

func patchUnion(s *source, schemas map[string]schema, name string, st *ast.StructType) error {
	fields, raw, err := s.branches(name, st)
	if err != nil {
		return err
	}
	if !raw {
		s.insert(st.Fields.Closing, "// Raw holds a variant this client version does not know, as the server sent it.\n"+
			"// It is nil for known variants, and MarshalJSON writes it back unchanged.\n"+
			"Raw json.RawMessage `json:\"-\"`\n")
	}
	decode := s.method(name, "UnmarshalJSON")
	if decode == nil {
		return fmt.Errorf("%s: missing %s.UnmarshalJSON", s.path, name)
	}
	if unionSchema, ok := schemas[name]; ok && unionSchema.Discriminator != nil {
		property, branches, err := dispatch(schemas, name, fields)
		if err != nil {
			return err
		}
		var body strings.Builder
		fmt.Fprintf(&body, "func (dst *%s) UnmarshalJSON(data []byte) error {\n*dst = %s{}\n", name, name)
		fmt.Fprintf(&body, "var tag struct { Discriminator *string `json:%q` }\n", property)
		body.WriteString("if err := json.Unmarshal(data, &tag); err != nil { return err }\n")
		fmt.Fprintf(&body, "if tag.Discriminator == nil || *tag.Discriminator == \"\" { return fmt.Errorf(%q) }\n", "missing "+name+" "+property)
		body.WriteString("switch *tag.Discriminator {\n")
		for _, b := range branches {
			fmt.Fprintf(&body, "case %q: var value %s; if err := json.Unmarshal(data, &value); err != nil { return err }; dst.%s = &value; return nil\n", b.id, b.typ, b.field)
		}
		fmt.Fprintf(&body, "default:\n%s}\n}", keepRaw("dst"))
		s.replace(decode, body.String())
	} else if !mentionsRaw(s, decode) && s.method(name, variantDecoder) == nil {
		// Keep the generated decoder for known variants, under another name.
		s.replace(decode.Name, variantDecoder)
		at := decode.Pos()
		if decode.Doc != nil {
			at = decode.Doc.Pos()
		}
		s.insert(at, fmt.Sprintf(`// UnmarshalJSON decodes a known variant, or keeps the payload in Raw when the generated
// decoder cannot pick a single known variant.
func (dst *%s) UnmarshalJSON(data []byte) error {
*dst = %s{}
err := dst.%s(data)
if err == nil {
return nil
}
*dst = %s{}
if string(data) == "null" || !json.Valid(data) {
return err
}
%s}

// %s is the generated decoder of the known variants.
//
`, name, name, variantDecoder, name, keepRaw("dst"), variantDecoder))
	}
	for _, method := range []struct{ name, result string }{
		{"MarshalJSON", "[]byte(%s.Raw), nil"},
		{"GetActualInstance", "%s.Raw"},
		{"GetActualInstanceValue", "%s.Raw"},
	} {
		fn := s.method(name, method.name)
		if fn == nil || mentionsRaw(s, fn) {
			continue
		}
		statements := fn.Body.List
		if len(statements) == 0 {
			return fmt.Errorf("%s: empty %s.%s", s.path, name, method.name)
		}
		variable := fn.Recv.List[0].Names[0].Name
		s.insert(statements[len(statements)-1].Pos(), fmt.Sprintf("if %s.Raw != nil {\nreturn %s\n}\n\n", variable, fmt.Sprintf(method.result, variable)))
	}
	return nil
}

// removeUnusedImport drops an import whose package the patched source no longer references.
func removeUnusedImport(output []byte, path, name string) ([]byte, error) {
	fset := token.NewFileSet()
	tree, err := parser.ParseFile(fset, "", output, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	used := false
	ast.Inspect(tree, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == name {
				used = true
			}
		}
		return !used
	})
	if used {
		return output, nil
	}
	for _, imp := range tree.Imports {
		if value, _ := strconv.Unquote(imp.Path.Value); value == path {
			start, end := fset.Position(imp.Pos()).Offset, fset.Position(imp.End()).Offset
			return append(output[:start:start], output[end:]...), nil
		}
	}
	return output, nil
}

func patchFile(path string, schemas map[string]schema, declared, patched map[string]bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	tree, err := parser.ParseFile(fset, path, data, parser.ParseComments)
	if err != nil {
		return err
	}
	s := &source{path: path, data: data, fset: fset, tree: tree}
	for _, decl := range tree.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.TYPE {
			for _, spec := range gen.Specs {
				declared[spec.(*ast.TypeSpec).Name.Name] = true
			}
		}
	}
	unions := s.unions()
	if len(unions) == 0 {
		return nil
	}
	names := make([]string, 0, len(unions))
	for name := range unions {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := patchUnion(s, schemas, name, unions[name]); err != nil {
			return err
		}
		patched[name] = true
	}
	sort.SliceStable(s.edits, func(i, j int) bool { return s.edits[i].start > s.edits[j].start })
	output := append([]byte{}, data...)
	for _, e := range s.edits {
		output = append(output[:e.start:e.start], append([]byte(e.text), output[e.end:]...)...)
	}
	// Dispatch replaces the only validator calls of a discriminated union.
	if output, err = removeUnusedImport(output, "gopkg.in/validator.v2", "validator"); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if output, err = format.Source(output); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return os.WriteFile(path, output, 0644)
}

func run(dir, specPath string) error {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return err
	}
	var spec struct {
		Components struct {
			Schemas map[string]schema `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		return err
	}
	models, err := filepath.Glob(filepath.Join(dir, "model_*.go"))
	if err != nil {
		return err
	}
	declared, patched := map[string]bool{}, map[string]bool{}
	for _, model := range models {
		if err := patchFile(model, spec.Components.Schemas, declared, patched); err != nil {
			return err
		}
	}
	// A discriminated union the generator emitted in another shape would silently keep the failing
	// decoder.
	for name, s := range spec.Components.Schemas {
		if s.Discriminator != nil && len(s.OneOf) > 0 && declared[name] && !patched[name] {
			return fmt.Errorf("discriminated union %s is not a generated union model", name)
		}
	}
	return nil
}

func main() {
	dir := flag.String("dir", "", "generated openapi package directory")
	spec := flag.String("spec", "", "OpenAPI JSON specification")
	flag.Parse()
	if *dir == "" || *spec == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*dir, *spec); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
