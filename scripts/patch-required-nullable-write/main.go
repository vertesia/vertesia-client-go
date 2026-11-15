// Command patch-required-nullable-write makes the generated ToMap methods always write a required
// property that is nullable and is a list, a map or a free-form value.
//
// openapi-generator has no Nullable wrapper for those types, so it guards them with a nil check:
//
//	if o.ToolAllowlist != nil {
//		toSerialize["tool_allowlist"] = o.ToolAllowlist
//	}
//
// A nil value then drops the property, and the request fails because a required property is
// missing. For a required property, nil can only mean null: after this patch ToMap writes it
// unconditionally, so a nil slice, map or interface is encoded as null. Optional properties keep
// their guard, because omitting them is how a caller leaves them unchanged.
//
// A property is required when its struct field has no omitempty option, which is how the generator
// writes required fields. The patch locates the ToMap methods with go/ast, replaces only the source
// spans of the guards, and is idempotent: a patched file has no guard left to replace.
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
	"sort"
	"strconv"
	"strings"
)

type edit struct {
	start, end int
	text       string
}

// requiredFields returns, for each struct type of the file, the JSON names of its fields without
// omitempty, keyed by Go field name.
func requiredFields(tree *ast.File) map[string]map[string]string {
	types := map[string]map[string]string{}
	for _, decl := range tree.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec := spec.(*ast.TypeSpec)
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}
			fields := map[string]string{}
			for _, field := range structType.Fields.List {
				if field.Tag == nil || len(field.Names) != 1 {
					continue
				}
				raw, err := strconv.Unquote(field.Tag.Value)
				if err != nil {
					continue
				}
				tag, ok := reflect.StructTag(raw).Lookup("json")
				if !ok {
					continue
				}
				name, options, _ := strings.Cut(tag, ",")
				if name == "" || name == "-" || containsOption(options, "omitempty") {
					continue
				}
				fields[field.Names[0].Name] = name
			}
			types[typeSpec.Name.Name] = fields
		}
	}
	return types
}

func containsOption(options, option string) bool {
	for _, o := range strings.Split(options, ",") {
		if o == option {
			return true
		}
	}
	return false
}

// receiverField returns the field name when expr is receiver.Field.
func receiverField(expr ast.Expr, receiver string) (string, bool) {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	ident, ok := selector.X.(*ast.Ident)
	if !ok || ident.Name != receiver {
		return "", false
	}
	return selector.Sel.Name, true
}

// guardedWrite matches `if o.F != nil { toSerialize["key"] = o.F }` and returns F, the key and the
// assignment.
func guardedWrite(stmt ast.Stmt, receiver string) (string, string, *ast.AssignStmt, bool) {
	ifStmt, ok := stmt.(*ast.IfStmt)
	if !ok || ifStmt.Init != nil || ifStmt.Else != nil || len(ifStmt.Body.List) != 1 {
		return "", "", nil, false
	}
	cond, ok := ifStmt.Cond.(*ast.BinaryExpr)
	if !ok || cond.Op != token.NEQ {
		return "", "", nil, false
	}
	if nilIdent, ok := cond.Y.(*ast.Ident); !ok || nilIdent.Name != "nil" {
		return "", "", nil, false
	}
	field, ok := receiverField(cond.X, receiver)
	if !ok {
		return "", "", nil, false
	}
	assign, ok := ifStmt.Body.List[0].(*ast.AssignStmt)
	if !ok || assign.Tok != token.ASSIGN || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return "", "", nil, false
	}
	index, ok := assign.Lhs[0].(*ast.IndexExpr)
	if !ok {
		return "", "", nil, false
	}
	if target, ok := index.X.(*ast.Ident); !ok || target.Name != "toSerialize" {
		return "", "", nil, false
	}
	literal, ok := index.Index.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", "", nil, false
	}
	key, err := strconv.Unquote(literal.Value)
	if err != nil {
		return "", "", nil, false
	}
	if value, ok := receiverField(assign.Rhs[0], receiver); !ok || value != field {
		return "", "", nil, false
	}
	return field, key, assign, true
}

// patchSource returns src with the nil guards of required properties removed from ToMap.
func patchSource(path string, src []byte) ([]byte, bool, error) {
	fset := token.NewFileSet()
	tree, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil, false, err
	}
	required := requiredFields(tree)
	var edits []edit
	for _, decl := range tree.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "ToMap" || fn.Recv == nil || len(fn.Recv.List) != 1 || fn.Body == nil {
			continue
		}
		recv := fn.Recv.List[0]
		typeIdent, ok := recv.Type.(*ast.Ident)
		if !ok || len(recv.Names) != 1 {
			continue
		}
		fields := required[typeIdent.Name]
		for _, stmt := range fn.Body.List {
			field, key, assign, ok := guardedWrite(stmt, recv.Names[0].Name)
			if !ok || fields[field] != key {
				continue
			}
			start := fset.Position(stmt.Pos()).Offset
			end := fset.Position(stmt.End()).Offset
			text := src[fset.Position(assign.Pos()).Offset:fset.Position(assign.End()).Offset]
			edits = append(edits, edit{start, end, string(text)})
		}
	}
	if len(edits) == 0 {
		return src, false, nil
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	output := append([]byte{}, src...)
	for _, e := range edits {
		output = append(output[:e.start:e.start], append([]byte(e.text), output[e.end:]...)...)
	}
	output, err = format.Source(output)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", path, err)
	}
	return output, true, nil
}

func patchFile(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	output, changed, err := patchSource(path, src)
	if err != nil || !changed {
		return err
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
