// strip_locale_messages removes MyGo's locale-bound Messages facade. Muse's
// locales package is app-internal: all repository consumers use package-level
// accessors, and no consumer calls In(locale).Name(). Keep the transform here
// because the upstream generator has no option to omit this unused API.
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
)

func main() {
	path := flag.String("file", "", "generated catalog Go file")
	flag.Parse()
	if *path == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: strip_locale_messages -file <catalog_gen.go>")
		os.Exit(2)
	}
	if err := strip(*path); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func strip(path string) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return err
	}
	type access struct{ key, args ast.Expr }
	methods := map[string]access{}
	removed := map[ast.Decl]bool{}
	comments := map[*ast.CommentGroup]bool{}
	hasType, hasIn := false, false

	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.GenDecl:
			for _, s := range d.Specs {
				t, ok := s.(*ast.TypeSpec)
				if ok && t.Name.Name == "Messages" {
					if len(d.Specs) != 1 {
						return fmt.Errorf("Messages type shares a declaration; update the transform before regenerating")
					}
					hasType, removed[d] = true, true
					comments[d.Doc], comments[t.Doc], comments[t.Comment] = d.Doc != nil, t.Doc != nil, t.Comment != nil
				}
			}
		case *ast.FuncDecl:
			if d.Recv == nil {
				if d.Name.Name == "In" {
					hasIn, removed[d] = true, true
					if d.Doc != nil {
						comments[d.Doc] = true
					}
				}
				continue
			}
			if !messagesReceiver(d.Recv) {
				continue
			}
			call, ok := returnedCall(d)
			if !ok || len(call.Args) != 3 {
				return fmt.Errorf("Messages.%s: unexpected generated body", d.Name.Name)
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !ident(selector.X, "Catalog") || selector.Sel.Name != "Format" {
				return fmt.Errorf("Messages.%s: expected Catalog.Format", d.Name.Name)
			}
			locale, ok := call.Args[0].(*ast.SelectorExpr)
			if !ok || !ident(locale.X, "msgs") || locale.Sel.Name != "locale" {
				return fmt.Errorf("Messages.%s: expected msgs.locale", d.Name.Name)
			}
			if _, exists := methods[d.Name.Name]; exists {
				return fmt.Errorf("duplicate Messages.%s", d.Name.Name)
			}
			methods[d.Name.Name] = access{call.Args[1], call.Args[2]}
			removed[d] = true
			if d.Doc != nil {
				comments[d.Doc] = true
			}
		}
	}
	if len(methods) == 0 {
		if hasType || hasIn {
			return fmt.Errorf("incomplete Messages facade")
		}
		return nil
	}
	if !hasType || !hasIn {
		return fmt.Errorf("incomplete Messages facade")
	}

	changed := 0
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Recv != nil {
			continue
		}
		a, ok := methods[fn.Name.Name]
		if !ok {
			continue
		}
		call, ok := returnedCall(fn)
		if !ok {
			return fmt.Errorf("%s: expected generated accessor wrapper", fn.Name.Name)
		}
		method, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || method.Sel.Name != fn.Name.Name {
			return fmt.Errorf("%s: accessor wrapper mismatch", fn.Name.Name)
		}
		in, ok := method.X.(*ast.CallExpr)
		if !ok || !ident(in.Fun, "In") || len(in.Args) != 1 || !localeCall(in.Args[0]) {
			return fmt.Errorf("%s: expected In(locale) wrapper", fn.Name.Name)
		}
		fn.Body.List[0] = &ast.ReturnStmt{Results: []ast.Expr{&ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent("Catalog"), Sel: ast.NewIdent("Format")},
			Args: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("i18n"), Sel: ast.NewIdent("Locale")}}, a.key, a.args},
		}}}
		changed++
	}
	if changed != len(methods) {
		return fmt.Errorf("matched %d of %d generated accessors", changed, len(methods))
	}

	decls := f.Decls[:0]
	for _, d := range f.Decls {
		if !removed[d] {
			decls = append(decls, d)
		}
	}
	f.Decls = decls
	groups := f.Comments[:0]
	for _, c := range f.Comments {
		if !comments[c] {
			groups = append(groups, c)
		}
	}
	f.Comments = groups

	var out bytes.Buffer
	if err := format.Node(&out, fset, f); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out.Bytes(), info.Mode().Perm())
}

func messagesReceiver(r *ast.FieldList) bool {
	if r == nil || len(r.List) != 1 {
		return false
	}
	t, ok := r.List[0].Type.(*ast.Ident)
	return ok && t.Name == "Messages"
}

func returnedCall(fn *ast.FuncDecl) (*ast.CallExpr, bool) {
	if fn.Body == nil || len(fn.Body.List) != 1 {
		return nil, false
	}
	r, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(r.Results) != 1 {
		return nil, false
	}
	c, ok := r.Results[0].(*ast.CallExpr)
	return c, ok
}

func ident(x ast.Expr, name string) bool {
	i, ok := x.(*ast.Ident)
	return ok && i.Name == name
}

func localeCall(x ast.Expr) bool {
	c, ok := x.(*ast.CallExpr)
	if !ok || len(c.Args) != 0 {
		return false
	}
	s, ok := c.Fun.(*ast.SelectorExpr)
	return ok && ident(s.X, "i18n") && s.Sel.Name == "Locale"
}
