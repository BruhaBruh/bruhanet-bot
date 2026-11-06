// Package langguard finds Cyrillic string literals in developer-facing
// constructs (errors, logs, panics). Developer-visible text is English; Russian
// is only for human-facing messages, produced at the boundary.
package langguard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// usererr.Errorf(en, ru, ...) is the sanctioned way to carry a Russian message:
// only its first (English) argument is checked.

// Marker excuses a deliberate Russian human-facing literal. Put it as a comment
// on the call's line or on the line right above it.
const Marker = "//lang:ru"

// Hit is one Cyrillic literal inside a developer-facing construct.
type Hit struct {
	File string
	Line int
	Call string // errors.New, fmt.Errorf, slog.Info, panic, Error() method, ...
	Text string
}

var logMethods = map[string]bool{
	"Debug": true, "Info": true, "Warn": true, "Error": true,
	"DebugContext": true, "InfoContext": true, "WarnContext": true, "ErrorContext": true,
	"Log": true, "LogAttrs": true,
}

var testMethods = map[string]bool{"Error": true, "Errorf": true, "Fatal": true, "Fatalf": true}

func hasCyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

// Scan walks the roots and returns hits. Test files are included only when
// withTests is set; marked call sites are skipped unless ignoreMarker is set.
// Helpers that forward a string parameter into an error or log call (for
// example func invalid(msg string) error) are checked at their call sites too.
func Scan(roots []string, withTests, ignoreMarker bool) ([]Hit, error) {
	fset := token.NewFileSet()
	var files []*ast.File
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				switch d.Name() {
				case "node_modules", "dist", "vendor", ".git":
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || (!withTests && strings.HasSuffix(path, "_test.go")) {
				return nil
			}
			src, err := os.ReadFile(path) //nolint:gosec // developer tool
			if err != nil {
				return err
			}
			f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
			if err != nil {
				return err
			}
			files = append(files, f)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	wrappers := findWrappers(files)
	var hits []Hit
	for _, f := range files {
		hits = append(hits, scanFile(fset, f, wrappers, ignoreMarker)...)
	}
	return hits, nil
}

// findWrappers returns names of functions that pass one of their parameters
// straight into a developer-facing call.
func findWrappers(files []*ast.File) map[string]bool {
	w := map[string]bool{}
	for _, f := range files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil || fd.Type.Params == nil {
				continue
			}
			params := map[string]bool{}
			for _, p := range fd.Type.Params.List {
				for _, n := range p.Names {
					params[n.Name] = true
				}
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				c, ok := n.(*ast.CallExpr)
				if !ok || !isErrCall(calleeName(c)) {
					return true
				}
				for _, a := range c.Args {
					if id, ok := a.(*ast.Ident); ok && params[id.Name] {
						w[fd.Name.Name] = true
					}
				}
				return true
			})
		}
	}
	return w
}

func scanFile(fset *token.FileSet, f *ast.File, wrappers map[string]bool, ignoreMarker bool) []Hit {
	path := fset.Position(f.Pos()).Filename
	isTest := strings.HasSuffix(path, "_test.go")
	marked := map[int]bool{}
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if strings.HasPrefix(c.Text, Marker) {
				marked[fset.Position(c.Pos()).Line] = true
			}
		}
	}
	consts := stringConsts(f)
	var hits []Hit
	seen := map[token.Pos]bool{}
	add := func(call string, node ast.Node, exprs []ast.Expr) {
		line := fset.Position(node.Pos()).Line
		if !ignoreMarker && (marked[line] || marked[line-1]) {
			return
		}
		for _, e := range exprs {
			ast.Inspect(e, func(n ast.Node) bool {
				var val string
				switch x := n.(type) {
				case *ast.CallExpr:
					// usererr.Errorf(en, ru, ...): the Russian text is the deliberate human-facing one
					return calleeName(x) != "usererr.Errorf"
				case *ast.BasicLit:
					if x.Kind != token.STRING {
						return true
					}
					val = x.Value
				case *ast.Ident:
					v, ok := consts[x.Name]
					if !ok {
						return true
					}
					val = v
				default:
					return true
				}
				if hasCyrillic(val) && !seen[n.Pos()] {
					seen[n.Pos()] = true
					hits = append(hits, Hit{path, fset.Position(n.Pos()).Line, call, strings.TrimSpace(val)})
				}
				return true
			})
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			if x.Name.Name == "Error" && x.Recv != nil && x.Body != nil {
				add("Error() method", x, []ast.Expr{funcBodyExpr(x.Body)})
			}
		case *ast.CallExpr:
			name := calleeName(x)
			if name == "usererr.Errorf" && len(x.Args) > 0 {
				add(name, x, x.Args[:1]) // only the English text
			} else if isDevCall(name, isTest) {
				add(name, x, x.Args)
			} else if short := name[strings.LastIndex(name, ".")+1:]; short != "" && wrappers[short] {
				add(name+" (wrapper)", x, x.Args)
			}
		}
		return true
	})
	return hits
}

// funcBodyExpr wraps a body so ast.Inspect can walk it as an expression.
func funcBodyExpr(b *ast.BlockStmt) ast.Expr { return &ast.FuncLit{Type: &ast.FuncType{}, Body: b} }

func calleeName(c *ast.CallExpr) string {
	switch fn := c.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		if id, ok := fn.X.(*ast.Ident); ok {
			return id.Name + "." + fn.Sel.Name
		}
		return "." + fn.Sel.Name
	}
	return ""
}

func isErrCall(name string) bool {
	switch name {
	case "errors.New", "errors.Join", "fmt.Errorf", "panic":
		return true
	}
	return false
}

func isDevCall(name string, isTest bool) bool {
	if isErrCall(name) {
		return true
	}
	pkg, fn, ok := strings.Cut(name, ".")
	if !ok {
		return false
	}
	switch pkg {
	case "slog", "stlog", "log":
		return true
	case "": // method on a non-identifier receiver, e.g. stlog.L(ctx).Info
		return logMethods[fn] || isTest && testMethods[fn]
	}
	return logMethods[fn] || isTest && testMethods[fn]
}

// stringConsts maps package-level and local const/var names initialised with a
// string literal in this file (one level of indirection).
func stringConsts(f *ast.File) map[string]string {
	m := map[string]string{}
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range vs.Names {
			if i < len(vs.Values) {
				if bl, ok := vs.Values[i].(*ast.BasicLit); ok && bl.Kind == token.STRING {
					m[name.Name] = bl.Value
				}
			}
		}
		return true
	})
	return m
}
