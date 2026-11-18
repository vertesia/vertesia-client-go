// Command patch-required-null makes the generated decoders reject null for a required property that
// the spec does not let be null and whose Go type cannot hold it. The generated decoder only checks
// that each required key is present, so such a null decoded as the zero value: "mime_type": null
// became "".
//
// The spec decides: a required property it lets be null (a "null" type, a nullable reference, a
// union with a null branch, a schema without a type) is left alone, as is one the generator marked
// nullable, which covers the models the generator names after an inline schema. A property whose
// Go type holds null (a pointer, a slice, a map, an interface or a Nullable wrapper) keeps it and
// is left alone too.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const (
	marker = "// A required property that is not nullable cannot be null"
	// The generated getter of a required property the generator reads as nullable says so.
	nullableGetter = "If the value is explicit nil"
	schemaPrefix   = "#/components/schemas/"
)

type schema struct {
	Ref        string            `json:"$ref"`
	Type       json.RawMessage   `json:"type"`
	Nullable   bool              `json:"nullable"`
	Enum       []json.RawMessage `json:"enum"`
	Const      json.RawMessage   `json:"const"`
	OneOf      []schema          `json:"oneOf"`
	AnyOf      []schema          `json:"anyOf"`
	AllOf      []schema          `json:"allOf"`
	Properties map[string]schema `json:"properties"`
}

// spec holds the component schemas and the component each generated type name stands for.
type spec struct {
	schemas map[string]schema
	byType  map[string]string
}

func main() {
	dir := flag.String("dir", "openapi", "directory of the generated package")
	specPath := flag.String("spec", "", "OpenAPI JSON specification")
	flag.Parse()
	if *specPath == "" {
		fail(fmt.Errorf("-spec is required"))
	}
	s, err := loadSpec(*specPath)
	if err != nil {
		fail(err)
	}
	if err := run(*dir, s); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func loadSpec(path string) (*spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var document struct {
		Components struct {
			Schemas map[string]schema `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	return newSpec(document.Components.Schemas), nil
}

func newSpec(schemas map[string]schema) *spec {
	s := &spec{schemas: schemas, byType: map[string]string{}}
	ambiguous := map[string]bool{}
	for name := range schemas {
		typ := typeName(name)
		if other, ok := s.byType[typ]; ok && other != name {
			ambiguous[typ] = true
		}
		s.byType[typ] = name
	}
	for typ := range ambiguous {
		delete(s.byType, typ)
	}
	return s
}

// typeName returns the Go type name openapi-generator gives a component: Foo_bar becomes FooBar.
func typeName(component string) string {
	var name strings.Builder
	for _, part := range strings.FieldsFunc(component, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		runes := []rune(part)
		runes[0] = unicode.ToUpper(runes[0])
		name.WriteString(string(runes))
	}
	return name.String()
}

func run(dir string, s *spec) error {
	models, err := filepath.Glob(filepath.Join(dir, "model_*.go"))
	if err != nil {
		return err
	}
	// The types the models declare, by whether a value of that type can hold null. The Nullable
	// wrappers of utils.go are recognised by name.
	holders := map[string]bool{}
	for _, model := range models {
		tree, err := parser.ParseFile(token.NewFileSet(), model, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for typ, holds := range declaredTypes(tree) {
			holders[typ] = holds
		}
	}
	for _, model := range models {
		data, err := os.ReadFile(model)
		if err != nil {
			return err
		}
		patched, err := patch(data, s, holders)
		if err != nil {
			return fmt.Errorf("%s: %w", model, err)
		}
		if !bytes.Equal(patched, data) {
			if err := os.WriteFile(model, patched, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

// patch adds, after the required-property check of each UnmarshalJSON, a check that the required
// properties that cannot be null are not null.
func patch(data []byte, s *spec, holders map[string]bool) ([]byte, error) {
	if bytes.Contains(data, []byte(marker)) {
		return data, nil
	}
	fset := token.NewFileSet()
	tree, err := parser.ParseFile(fset, "", data, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	for typ, holds := range declaredTypes(tree) {
		if _, ok := holders[typ]; !ok {
			holders[typ] = holds
		}
	}
	fields := structFields(tree)
	nullable := nullableGetters(tree)
	type insertion struct {
		offset int
		text   string
	}
	var insertions []insertion
	for _, decl := range tree.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "UnmarshalJSON" || fn.Body == nil {
			continue
		}
		receiver := receiverType(fn)
		if receiver == "" {
			continue
		}
		required, check := requiredCheck(fn.Body)
		if check == nil {
			continue
		}
		var notNull []string
		for _, name := range required {
			f, ok := fields[receiver][name]
			if !ok || holdsNull(f.typ, holders) || nullable[receiver]["Get"+f.name] {
				continue
			}
			if allowed, known := s.allowsNull(receiver, name); known && allowed {
				continue
			}
			notNull = append(notNull, strconv.Quote(name))
		}
		if len(notNull) == 0 {
			continue
		}
		insertions = append(insertions, insertion{
			offset: fset.Position(check.End()).Offset,
			text: "\n\n\t" + marker + ": it would decode as its zero value.\n" +
				"\tfor _, requiredProperty := range []string{" + strings.Join(notNull, ", ") + "} {\n" +
				"\t\tif allProperties[requiredProperty] == nil {\n" +
				"\t\t\treturn fmt.Errorf(\"required property %v is null\", requiredProperty)\n" +
				"\t\t}\n" +
				"\t}",
		})
	}
	if len(insertions) == 0 {
		return data, nil
	}
	sort.Slice(insertions, func(i, j int) bool { return insertions[i].offset > insertions[j].offset })
	out := append([]byte(nil), data...)
	for _, in := range insertions {
		out = append(out[:in.offset], append([]byte(in.text), out[in.offset:]...)...)
	}
	return format.Source(out)
}

// allowsNull reports whether the spec lets the property of the component behind a generated type be
// null, and whether the spec answers at all: a type named after an inline schema has no component.
func (s *spec) allowsNull(typ, property string) (allowed, known bool) {
	component, ok := s.byType[typ]
	if !ok {
		return false, false
	}
	definitions := s.property(s.schemas[component], property, map[string]bool{component: true})
	if len(definitions) == 0 {
		return false, false
	}
	for _, definition := range definitions {
		if !s.nullAllowed(definition, map[string]bool{}) {
			return false, true
		}
	}
	return true, true
}

// property returns the definitions of a property in a schema and in the schemas it extends.
func (s *spec) property(sc schema, name string, seen map[string]bool) []schema {
	var definitions []schema
	if definition, ok := sc.Properties[name]; ok {
		definitions = append(definitions, definition)
	}
	if target, ok := s.resolve(sc.Ref, seen); ok {
		definitions = append(definitions, s.property(target, name, seen)...)
	}
	for _, member := range sc.AllOf {
		definitions = append(definitions, s.property(member, name, seen)...)
	}
	return definitions
}

func (s *spec) resolve(ref string, seen map[string]bool) (schema, bool) {
	name, ok := strings.CutPrefix(ref, schemaPrefix)
	if !ok || seen[name] {
		return schema{}, false
	}
	target, ok := s.schemas[name]
	if ok {
		seen[name] = true
	}
	return target, ok
}

// nullAllowed reports whether a schema accepts null. Every keyword it carries must accept it, and a
// schema that cannot be read to the end, such as an unknown or recursive reference, accepts it.
func (s *spec) nullAllowed(sc schema, seen map[string]bool) bool {
	if sc.Nullable {
		return true
	}
	if sc.Ref != "" {
		name, _ := strings.CutPrefix(sc.Ref, schemaPrefix)
		if target, ok := s.resolve(sc.Ref, seen); ok {
			allowed := s.nullAllowed(target, seen)
			delete(seen, name)
			if !allowed {
				return false
			}
		}
	}
	if len(sc.Type) > 0 && !typeIncludesNull(sc.Type) {
		return false
	}
	if len(sc.Enum) > 0 && !containsNull(sc.Enum) {
		return false
	}
	if len(sc.Const) > 0 && string(sc.Const) != "null" {
		return false
	}
	for _, branches := range [][]schema{sc.OneOf, sc.AnyOf} {
		if len(branches) == 0 {
			continue
		}
		allows := false
		for _, branch := range branches {
			if s.nullAllowed(branch, seen) {
				allows = true
				break
			}
		}
		if !allows {
			return false
		}
	}
	for _, member := range sc.AllOf {
		if !s.nullAllowed(member, seen) {
			return false
		}
	}
	return true
}

func typeIncludesNull(raw json.RawMessage) bool {
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return single == "null"
	}
	var several []string
	if json.Unmarshal(raw, &several) != nil {
		return true
	}
	for _, typ := range several {
		if typ == "null" {
			return true
		}
	}
	return false
}

func containsNull(values []json.RawMessage) bool {
	for _, value := range values {
		if string(bytes.TrimSpace(value)) == "null" {
			return true
		}
	}
	return false
}

type field struct {
	name string
	typ  ast.Expr
}

// structFields returns, for each struct type, its fields by JSON name.
func structFields(tree *ast.File) map[string]map[string]field {
	types := map[string]map[string]field{}
	ast.Inspect(tree, func(node ast.Node) bool {
		ts, ok := node.(*ast.TypeSpec)
		if !ok {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return false
		}
		fields := map[string]field{}
		for _, f := range st.Fields.List {
			if f.Tag == nil || len(f.Names) == 0 {
				continue
			}
			tag, err := strconv.Unquote(f.Tag.Value)
			if err != nil {
				continue
			}
			name, _, _ := strings.Cut(reflect.StructTag(tag).Get("json"), ",")
			if name != "" && name != "-" {
				fields[name] = field{name: f.Names[0].Name, typ: f.Type}
			}
		}
		types[ts.Name.Name] = fields
		return false
	})
	return types
}

// nullableGetters returns, for each type, the getters the generator documents as returning an
// explicit nil.
func nullableGetters(tree *ast.File) map[string]map[string]bool {
	getters := map[string]map[string]bool{}
	for _, decl := range tree.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Doc == nil || !strings.Contains(fn.Doc.Text(), nullableGetter) {
			continue
		}
		if receiver := receiverType(fn); receiver != "" {
			if getters[receiver] == nil {
				getters[receiver] = map[string]bool{}
			}
			getters[receiver][fn.Name.Name] = true
		}
	}
	return getters
}

func receiverType(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) != 1 {
		return ""
	}
	typ := fn.Recv.List[0].Type
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	if ident, ok := typ.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// requiredCheck returns the names in requiredProperties and the loop that checks they are present.
func requiredCheck(body *ast.BlockStmt) ([]string, *ast.RangeStmt) {
	var names []string
	var check *ast.RangeStmt
	for _, stmt := range body.List {
		switch stmt := stmt.(type) {
		case *ast.AssignStmt:
			if len(stmt.Lhs) != 1 || len(stmt.Rhs) != 1 || !isIdent(stmt.Lhs[0], "requiredProperties") {
				continue
			}
			list, ok := stmt.Rhs[0].(*ast.CompositeLit)
			if !ok {
				continue
			}
			for _, element := range list.Elts {
				if literal, ok := element.(*ast.BasicLit); ok && literal.Kind == token.STRING {
					if name, err := strconv.Unquote(literal.Value); err == nil {
						names = append(names, name)
					}
				}
			}
		case *ast.RangeStmt:
			if isIdent(stmt.X, "requiredProperties") {
				check = stmt
			}
		}
	}
	return names, check
}

func isIdent(expr ast.Expr, name string) bool {
	// openapi-generator writes range(requiredProperties), and the patch runs before gofmt.
	for {
		paren, ok := expr.(*ast.ParenExpr)
		if !ok {
			break
		}
		expr = paren.X
	}
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == name
}

// declaredTypes returns the types a file declares, by whether a value of the type can hold null.
func declaredTypes(tree *ast.File) map[string]bool {
	types := map[string]bool{}
	for _, decl := range tree.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, ts := range gen.Specs {
			typ := ts.(*ast.TypeSpec)
			types[typ.Name.Name] = strings.HasPrefix(typ.Name.Name, "Nullable") || holdsNull(typ.Type, nil)
		}
	}
	return types
}

// holdsNull reports whether a value of this type keeps a JSON null apart from every other value:
// a pointer, a slice, a map, an interface or a generated Nullable wrapper decode null as nil or
// unset, where a string, a number, a struct or an enum decode it as their zero value.
func holdsNull(fieldType ast.Expr, holders map[string]bool) bool {
	switch t := fieldType.(type) {
	case *ast.StarExpr, *ast.InterfaceType, *ast.MapType:
		return true
	case *ast.ArrayType:
		return t.Len == nil
	case *ast.Ident:
		return t.Name == "any" || strings.HasPrefix(t.Name, "Nullable") || holders[t.Name]
	}
	return false
}
