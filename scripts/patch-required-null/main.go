// Command patch-required-null makes the generated decoders reject null for a required property
// whose Go type cannot hold it. The generated decoder only checks that each required key is
// present, so a null decoded as the zero value: "mime_type": null became "".
package main

import (
	"bytes"
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

const marker = "// A required property that is not nullable cannot be null"

func main() {
	dir := flag.String("dir", "openapi", "directory of the generated package")
	flag.Parse()
	models, err := filepath.Glob(filepath.Join(*dir, "model_*.go"))
	if err != nil {
		fail(err)
	}
	for _, model := range models {
		data, err := os.ReadFile(model)
		if err != nil {
			fail(err)
		}
		patched, err := patch(data)
		if err != nil {
			fail(fmt.Errorf("%s: %w", model, err))
		}
		if !bytes.Equal(patched, data) {
			if err := os.WriteFile(model, patched, 0o644); err != nil {
				fail(err)
			}
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

// patch adds, after the required-property check of each UnmarshalJSON, a check that the required
// properties whose type cannot hold null are not null.
func patch(data []byte) ([]byte, error) {
	if bytes.Contains(data, []byte(marker)) {
		return data, nil
	}
	fset := token.NewFileSet()
	tree, err := parser.ParseFile(fset, "", data, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	fields := structFields(tree)
	type insertion struct {
		offset int
		text   string
	}
	var insertions []insertion
	for _, decl := range tree.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "UnmarshalJSON" || fn.Recv == nil || len(fn.Recv.List) != 1 || fn.Body == nil {
			continue
		}
		star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		receiver, ok := star.X.(*ast.Ident)
		if !ok {
			continue
		}
		required, check := requiredCheck(fn.Body)
		if check == nil {
			continue
		}
		var notNull []string
		for _, name := range required {
			if fieldType, ok := fields[receiver.Name][name]; ok && !holdsNull(fieldType) {
				notNull = append(notNull, strconv.Quote(name))
			}
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

// structFields returns, for each struct type, the Go type of each field by its JSON name.
func structFields(tree *ast.File) map[string]map[string]ast.Expr {
	types := map[string]map[string]ast.Expr{}
	ast.Inspect(tree, func(node ast.Node) bool {
		spec, ok := node.(*ast.TypeSpec)
		if !ok {
			return true
		}
		st, ok := spec.Type.(*ast.StructType)
		if !ok {
			return false
		}
		fields := map[string]ast.Expr{}
		for _, field := range st.Fields.List {
			if field.Tag == nil || len(field.Names) == 0 {
				continue
			}
			tag, err := strconv.Unquote(field.Tag.Value)
			if err != nil {
				continue
			}
			name, _, _ := strings.Cut(reflect.StructTag(tag).Get("json"), ",")
			if name != "" && name != "-" {
				fields[name] = field.Type
			}
		}
		types[spec.Name.Name] = fields
		return false
	})
	return types
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

// holdsNull reports whether a field of this type can hold a JSON null: a pointer, an interface or
// a generated Nullable wrapper.
func holdsNull(fieldType ast.Expr) bool {
	switch t := fieldType.(type) {
	case *ast.StarExpr, *ast.InterfaceType:
		return true
	case *ast.Ident:
		return t.Name == "any" || strings.HasPrefix(t.Name, "Nullable")
	}
	return false
}
