// Command patch-model-options adds compatible ModelOptions decoding after generation.
// It locates declarations with go/ast, replaces only their source spans, and
// formats the result. Generated validators and unrelated declarations are retained.
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
)

type schema struct {
	Required      []string          `json:"required"`
	OneOf         []reference       `json:"oneOf"`
	AnyOf         []reference       `json:"anyOf"`
	Properties    map[string]schema `json:"properties"`
	Const         json.RawMessage   `json:"const"`
	Enum          []json.RawMessage `json:"enum"`
	Discriminator *struct {
		PropertyName string            `json:"propertyName"`
		Mapping      map[string]string `json:"mapping"`
	} `json:"discriminator"`
}
type reference struct {
	Ref string `json:"$ref"`
}
type branch struct {
	id, name, field string
	required        bool
}
type edit struct {
	start, end int
	text       string
}
type source struct {
	data  []byte
	tree  *ast.File
	fset  *token.FileSet
	edits []edit
}

func readSource(path string) (*source, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	tree, err := parser.ParseFile(fset, path, data, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	return &source{data: data, tree: tree, fset: fset}, nil
}
func (s *source) text(n ast.Node) string {
	return string(s.data[s.fset.Position(n.Pos()).Offset:s.fset.Position(n.End()).Offset])
}
func (s *source) replace(n ast.Node, text string) {
	s.edits = append(s.edits, edit{s.fset.Position(n.Pos()).Offset, s.fset.Position(n.End()).Offset, text})
}
func (s *source) append(text string) {
	s.edits = append(s.edits, edit{len(s.data), len(s.data), "\n" + text + "\n"})
}
func receiver(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) != 1 {
		return ""
	}
	t := fn.Recv.List[0].Type
	if p, ok := t.(*ast.StarExpr); ok {
		t = p.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}
func (s *source) function(typ, name string) *ast.FuncDecl {
	for _, decl := range s.tree.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == name && receiver(fn) == typ {
			return fn
		}
	}
	return nil
}
func (s *source) structure(name string) *ast.StructType {
	for _, decl := range s.tree.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.TYPE {
			for _, spec := range gen.Specs {
				t := spec.(*ast.TypeSpec)
				if t.Name.Name == name {
					st, _ := t.Type.(*ast.StructType)
					return st
				}
			}
		}
	}
	return nil
}
func (s *source) setFunction(typ, name, text string, required bool) error {
	if fn := s.function(typ, name); fn != nil {
		s.replace(fn, text)
	} else if required {
		return fmt.Errorf("missing generated %s.%s", typ, name)
	} else {
		s.append(text)
	}
	return nil
}
func (s *source) result() ([]byte, error) {
	sort.SliceStable(s.edits, func(i, j int) bool { return s.edits[i].start < s.edits[j].start })
	var out bytes.Buffer
	pos := 0
	for _, e := range s.edits {
		if e.start < pos {
			return nil, fmt.Errorf("overlapping AST edits")
		}
		out.Write(s.data[pos:e.start])
		out.WriteString(e.text)
		pos = e.end
	}
	out.Write(s.data[pos:])
	return format.Source(out.Bytes())
}
func (s *source) imports(add string, remove ...string) {
	found := false
	for _, decl := range s.tree.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.IMPORT {
			continue
		}
		var specs []string
		for _, spec := range gen.Specs {
			imp := spec.(*ast.ImportSpec)
			path, _ := strconv.Unquote(imp.Path.Value)
			if path == add {
				found = true
			}
			drop := false
			for _, r := range remove {
				if path == r {
					drop = true
				}
			}
			if !drop {
				specs = append(specs, s.text(imp))
			}
		}
		if add != "" && !found {
			specs = append(specs, strconv.Quote(add))
			found = true
		}
		s.replace(gen, "import (\n"+strings.Join(specs, "\n")+"\n)")
	}
}
func loadBranches(path string) ([]branch, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var spec struct {
		Components struct {
			Schemas map[string]schema `json:"schemas"`
		} `json:"components"`
	}
	if err = json.Unmarshal(data, &spec); err != nil {
		return nil, err
	}
	union, ok := spec.Components.Schemas["ModelOptions"]
	if !ok {
		return nil, fmt.Errorf("missing ModelOptions schema")
	}
	refs := union.OneOf
	if len(refs) == 0 {
		refs = union.AnyOf
	} else if len(union.AnyOf) > 0 {
		return nil, fmt.Errorf("ambiguous union composition")
	}
	if len(refs) == 0 {
		return nil, fmt.Errorf("empty ModelOptions union")
	}
	property := "_option_id"
	if union.Discriminator != nil {
		property = union.Discriminator.PropertyName
	}
	if property != "_option_id" {
		return nil, fmt.Errorf("unexpected discriminator %q", property)
	}
	mapping := map[string]string{}
	var branches []branch
	for _, ref := range refs {
		if !strings.HasPrefix(ref.Ref, "#/components/schemas/") {
			return nil, fmt.Errorf("unsupported branch ref %q", ref.Ref)
		}
		name := strings.TrimPrefix(ref.Ref, "#/components/schemas/")
		member := spec.Components.Schemas[name]
		tag := member.Properties[property]
		value := tag.Const
		if len(tag.Enum) > 0 {
			if len(tag.Enum) != 1 {
				return nil, fmt.Errorf("non-literal ID for %s", name)
			}
			value = tag.Enum[0]
		}
		var id string
		if err := json.Unmarshal(value, &id); err != nil || id == "" || mapping[id] != "" {
			return nil, fmt.Errorf("missing or duplicate literal ID for %s", name)
		}
		mapping[id] = ref.Ref
		branches = append(branches, branch{id: id, name: name, required: len(member.Required) > 0})
	}
	if union.Discriminator != nil && union.Discriminator.Mapping != nil && !reflect.DeepEqual(mapping, union.Discriminator.Mapping) {
		return nil, fmt.Errorf("discriminator mapping does not cover union")
	}
	return branches, nil
}

func patchUnion(s *source, branches []branch) error {
	st := s.structure("ModelOptions")
	if st == nil {
		return fmt.Errorf("missing ModelOptions struct")
	}
	var fields []branch
	raw := false
	for _, f := range st.Fields.List {
		if len(f.Names) != 1 {
			return fmt.Errorf("unexpected ModelOptions field")
		}
		name := f.Names[0].Name
		if name == "Raw" {
			if s.text(f.Type) != "json.RawMessage" {
				return fmt.Errorf("unexpected Raw field type")
			}
			raw = true
			continue
		}
		ptr, ok := f.Type.(*ast.StarExpr)
		if !ok {
			return fmt.Errorf("unexpected union field %s", name)
		}
		typ, ok := ptr.X.(*ast.Ident)
		if !ok {
			return fmt.Errorf("unexpected union pointer type")
		}
		fields = append(fields, branch{name: typ.Name, field: name})
	}
	if len(fields) != len(branches) {
		return fmt.Errorf("generated fields do not cover ModelOptions")
	}
	for i := range branches {
		found := false
		for _, f := range fields {
			if f.name == branches[i].name {
				branches[i].field = f.field
				found = true
			}
		}
		if !found {
			return fmt.Errorf("missing generated branch %s", branches[i].name)
		}
	}
	if !raw {
		s.edits = append(s.edits, edit{s.fset.Position(st.Fields.Closing).Offset, s.fset.Position(st.Fields.Closing).Offset, "// Raw preserves options without a recognized family ID.\nRaw json.RawMessage `json:\"-\"`\n"})
	}
	var decode strings.Builder
	decode.WriteString("func (dst *ModelOptions) UnmarshalJSON(data []byte) error {\n*dst = ModelOptions{}\nvar tag struct { ID string `json:\"_option_id\"` }; if err:=json.Unmarshal(data,&tag); err!=nil{return err};switch tag.ID {\n")
	for _, b := range branches {
		fmt.Fprintf(&decode, "case %q: var branch %s; if err:=json.Unmarshal(data,&branch);err!=nil{return err};dst.%s=&branch;return nil\n", b.id, b.name, b.field)
	}
	decode.WriteString("default: return dst.Raw.UnmarshalJSON(data)\n}}")
	if err := s.setFunction("ModelOptions", "UnmarshalJSON", decode.String(), true); err != nil {
		return err
	}
	for _, method := range []struct{ name, recv, returns, expr, fallback string }{
		{"MarshalJSON", "src ModelOptions", "([]byte,error)", "json.Marshal(src.%s)", "json.Marshal(src.Raw)"},
		{"GetActualInstance", "obj *ModelOptions", "interface{}", "obj.%s", "obj.Raw"},
		{"GetActualInstanceValue", "obj ModelOptions", "interface{}", "*obj.%s", "obj.Raw"},
	} {
		variable := strings.Fields(method.recv)[0]
		var body strings.Builder
		fmt.Fprintf(&body, "func (%s) %s() %s {\n", method.recv, method.name, method.returns)
		if method.name == "GetActualInstance" {
			body.WriteString("if obj==nil{return nil}\n")
		}
		for _, f := range fields {
			fmt.Fprintf(&body, "if %s.%s!=nil{return %s}\n", variable, f.field, fmt.Sprintf(method.expr, f.field))
		}
		if method.name != "MarshalJSON" {
			body.WriteString("if obj.Raw==nil{return nil}\n")
		}
		fmt.Fprintf(&body, "return %s\n}", method.fallback)
		if err := s.setFunction("ModelOptions", method.name, body.String(), method.name == "MarshalJSON"); err != nil {
			return err
		}
	}
	for _, f := range fields {
		name := f.field + "AsModelOptions"
		if s.function("", name) == nil {
			s.append(fmt.Sprintf("func %s(v *%s) ModelOptions {return ModelOptions{%s:v}}", name, f.name, f.field))
		}
	}
	if s.function("", "modelOptionsIsKnownField") == nil {
		s.append(`// Match encoding/json field lookup so alternate casing cannot undo a typed clear.
func modelOptionsIsKnownField(name string, known []string) bool {
 for _, field := range known { if strings.EqualFold(name,field) { return true } }; return false
}`)
	}
	s.imports("strings", "fmt", "gopkg.in/validator.v2")
	return nil
}

func assignedName(stmt ast.Stmt) string {
	a, ok := stmt.(*ast.AssignStmt)
	if !ok || len(a.Lhs) != 1 {
		return ""
	}
	id, _ := a.Lhs[0].(*ast.Ident)
	if id != nil {
		return id.Name
	}
	return ""
}
func (s *source) cleanValidator(fn *ast.FuncDecl) error {
	// Preserve the generator's required-field checks and typed decoder. Its old
	// extension extraction is replaced by the precision-preserving wrapper.
	for _, stmt := range fn.Body.List {
		switch assignedName(stmt) {
		case "allProperties":
			a := stmt.(*ast.AssignStmt)
			if len(a.Rhs) != 1 {
				return fmt.Errorf("unexpected required-field check")
			}
			call, ok := a.Rhs[0].(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return fmt.Errorf("unexpected required-field map")
			}
			s.replace(call.Args[0], "map[string]json.RawMessage")
		case "additionalProperties":
			s.replace(stmt, "")
		}
		if conditional, ok := stmt.(*ast.IfStmt); ok && conditional.Init != nil {
			init, ok := conditional.Init.(*ast.AssignStmt)
			if !ok || len(init.Rhs) != 1 {
				continue
			}
			call, ok := init.Rhs[0].(*ast.CallExpr)
			if !ok || len(call.Args) != 2 {
				continue
			}
			arg, ok := call.Args[1].(*ast.UnaryExpr)
			if !ok {
				continue
			}
			id, ok := arg.X.(*ast.Ident)
			if ok && id.Name == "additionalProperties" {
				s.replace(conditional, "")
			}
		}
	}
	return nil
}
func patchBranch(s *source, name string, required bool) error {
	st := s.structure(name)
	if st == nil {
		return fmt.Errorf("missing struct %s", name)
	}
	var known []string
	hasExtra := false
	for _, f := range st.Fields.List {
		if len(f.Names) != 1 {
			return fmt.Errorf("unexpected %s field", name)
		}
		if f.Names[0].Name == "AdditionalProperties" {
			m, ok := f.Type.(*ast.MapType)
			if !ok {
				return fmt.Errorf("unexpected extension map for %s", name)
			}
			key, ok := m.Key.(*ast.Ident)
			value, iface := m.Value.(*ast.InterfaceType)
			if !ok || key.Name != "string" || !iface || len(value.Methods.List) != 0 {
				return fmt.Errorf("unexpected extension map type for %s", name)
			}
			s.replace(f, "AdditionalProperties map[string]interface{} `json:\"-\"`")
			hasExtra = true
			continue
		}
		if f.Tag == nil {
			return fmt.Errorf("missing wire tag in %s", name)
		}
		tag, err := strconv.Unquote(f.Tag.Value)
		if err != nil {
			return err
		}
		wire := strings.Split(reflect.StructTag(tag).Get("json"), ",")[0]
		if wire == "" || wire == "-" {
			return fmt.Errorf("unexpected wire field in %s", name)
		}
		known = append(known, strconv.Quote(wire))
	}
	if len(known) == 0 {
		return fmt.Errorf("no generated fields for %s", name)
	}
	literal := "[]string{" + strings.Join(known, ",") + "}"
	if !hasExtra {
		s.edits = append(s.edits, edit{s.fset.Position(st.Fields.Closing).Offset, s.fset.Position(st.Fields.Closing).Offset, "// AdditionalProperties preserves unknown fields across read-edit-save.\nAdditionalProperties map[string]interface{} `json:\"-\"`\n"})
	}
	validator := s.function(name, "unmarshalKnownJSON")
	if validator == nil {
		if original := s.function(name, "UnmarshalJSON"); original != nil {
			s.replace(original.Name, "unmarshalKnownJSON")
			if err := s.cleanValidator(original); err != nil {
				return err
			}
		} else {
			if required {
				return fmt.Errorf("missing generated validator for required fields in %s", name)
			}
			s.append(fmt.Sprintf("func (o *%s) unmarshalKnownJSON(data []byte) error {type plain %s;return json.Unmarshal(data,(*plain)(o))}", name, name))
		}
	} // An existing private validator has already been transformed.
	wrapper := fmt.Sprintf(`func (o *%s) UnmarshalJSON(data []byte) error {
 *o=%s{};var decoded %s
 if err:=decoded.unmarshalKnownJSON(data);err!=nil{return err}
 var extra map[string]json.RawMessage
 if err:=json.Unmarshal(data,&extra);err!=nil{return err}
 for key:=range extra {if modelOptionsIsKnownField(key,%s){delete(extra,key)}}
 if len(extra)>0 {decoded.AdditionalProperties=make(map[string]interface{},len(extra));for key,value:=range extra {decoded.AdditionalProperties[key]=value}}
 *o=decoded;return nil
}`, name, name, name, literal)
	if validator != nil {
		if err := s.setFunction(name, "UnmarshalJSON", wrapper, true); err != nil {
			return err
		}
	} else {
		s.append(wrapper)
	}
	toMap := s.function(name, "ToMap")
	if toMap == nil || len(toMap.Body.List) == 0 {
		return fmt.Errorf("missing generated %s.ToMap", name)
	}
	hasLoop := false
	for _, stmt := range toMap.Body.List {
		loop, ok := stmt.(*ast.RangeStmt)
		if !ok {
			continue
		}
		selector, ok := loop.X.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "AdditionalProperties" {
			continue
		}
		if hasLoop {
			return fmt.Errorf("duplicate extension loops in %s", name)
		}
		hasLoop = true
		s.replace(loop, fmt.Sprintf("for key,value:=range o.AdditionalProperties {if !modelOptionsIsKnownField(key,%s){toSerialize[key]=value}}", literal))
	}
	if !hasLoop {
		first := toMap.Body.List[0]
		if assignedName(first) != "toSerialize" {
			return fmt.Errorf("unexpected %s.ToMap initialization", name)
		}
		pos := s.fset.Position(first.End()).Offset
		s.edits = append(s.edits, edit{pos, pos, fmt.Sprintf("\n// Typed fields retain precedence, including when cleared.\nfor key,value:=range o.AdditionalProperties {if !modelOptionsIsKnownField(key,%s){toSerialize[key]=value}}\n", literal)})
	}
	return nil
}

func run(modelPath, specPath string) error {
	branches, err := loadBranches(specPath)
	if err != nil {
		return err
	}
	union, err := readSource(modelPath)
	if err != nil {
		return err
	}
	if err = patchUnion(union, branches); err != nil {
		return err
	}
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(modelPath), "model_*.go"))
	if err != nil {
		return err
	}
	pending := map[string][]byte{}
	if pending[modelPath], err = union.result(); err != nil {
		return err
	}
	remaining := map[string]bool{}
	for _, b := range branches {
		remaining[b.name] = true
	}
	for _, path := range paths {
		if path == modelPath {
			continue
		}
		s, err := readSource(path)
		if err != nil {
			return err
		}
		changed := false
		for _, b := range branches {
			if s.structure(b.name) != nil {
				if !remaining[b.name] {
					return fmt.Errorf("duplicate generated type %s", b.name)
				}
				if err = patchBranch(s, b.name, b.required); err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
				delete(remaining, b.name)
				changed = true
			}
		}
		if changed {
			if pending[path], err = s.result(); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
		}
	}
	if len(remaining) > 0 {
		return fmt.Errorf("missing generated branch files: %v", remaining)
	}
	// Validate every transformation before changing any generated files.
	for path, data := range pending {
		if err = os.WriteFile(path, data, 0644); err != nil {
			return err
		}
	}
	return nil
}
func main() {
	model := flag.String("model", "", "generated model_model_options.go")
	spec := flag.String("spec", "", "OpenAPI JSON specification")
	flag.Parse()
	if *model == "" || *spec == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(filepath.Clean(*model), *spec); err != nil {
		fmt.Fprintln(os.Stderr, "patch-model-options:", err)
		os.Exit(1)
	}
}
