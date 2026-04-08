package app

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestCommandFilesDoNotExportSharedHelpers(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve test file path")
	}
	dir := filepath.Dir(thisFile)

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(info os.FileInfo) bool {
		name := info.Name()
		return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse app package: %v", err)
	}
	pkgAST, ok := pkgs["app"]
	if !ok {
		t.Fatal("app package not found")
	}

	declaredInCmdFile := map[string]string{}
	files := make([]*ast.File, 0, len(pkgAST.Files))
	for path, file := range pkgAST.Files {
		files = append(files, file)
		if !isCommandFile(path) {
			continue
		}
		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				if decl.Recv == nil {
					declaredInCmdFile[decl.Name.Name] = path
				}
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					switch spec := spec.(type) {
					case *ast.TypeSpec:
						declaredInCmdFile[spec.Name.Name] = path
					case *ast.ValueSpec:
						for _, name := range spec.Names {
							declaredInCmdFile[name.Name] = path
						}
					}
				}
			}
		}
	}

	violations := map[string]bool{}
	for path, file := range pkgAST.Files {
		localNames := definedNames(file)
		stack := []ast.Node{}
		ast.Inspect(file, func(n ast.Node) bool {
			if n == nil {
				stack = stack[:len(stack)-1]
				return true
			}
			var parent ast.Node
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			stack = append(stack, n)

			ident, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			declPath, ok := declaredInCmdFile[ident.Name]
			if !ok || path == declPath || localNames[ident.Name] || shouldIgnoreIdentifier(file, parent, ident) {
				return true
			}
			violations[fmt.Sprintf("%s references %s from %s", filepath.Base(path), ident.Name, filepath.Base(declPath))] = true
			return true
		})
	}

	if len(violations) == 0 {
		return
	}

	lines := make([]string, 0, len(violations))
	for line := range violations {
		lines = append(lines, line)
	}
	sort.Strings(lines)
	t.Fatalf("command-file helper boundary violated:\n%s", strings.Join(lines, "\n"))
}

func isCommandFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasPrefix(base, "cmd_") && strings.HasSuffix(base, ".go")
}

func definedNames(file *ast.File) map[string]bool {
	names := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok || ident.Obj == nil || ident.Obj.Pos() != ident.Pos() {
			return true
		}
		names[ident.Name] = true
		return true
	})
	return names
}

func shouldIgnoreIdentifier(file *ast.File, parent ast.Node, ident *ast.Ident) bool {
	if ident == file.Name {
		return true
	}
	switch parent := parent.(type) {
	case *ast.SelectorExpr:
		return parent.Sel == ident
	case *ast.KeyValueExpr:
		return parent.Key == ident
	case *ast.Field, *ast.ImportSpec, *ast.LabeledStmt, *ast.BranchStmt:
		return true
	default:
		return false
	}
}
