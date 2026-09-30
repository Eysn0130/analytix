package evidenceauthorityhostlocal

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The accepted first-stage host_local profile selects one exact signed head
// by CAS. The existing witnessed evidenceauthority package independently
// forbids local current selection. This guard prevents the local profile from
// silently replacing its selector with directory-order or maximum-generation
// selection while allowing complete Visit only to reject orphans.
func TestHostLocalAdapterNeverSelectsFromDirectoryInventory(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("host-local source directory is unavailable")
	}
	entries, err := os.ReadDir(filepath.Dir(source))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(filepath.Dir(source), entry.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch selector.Sel.Name {
			case "List", "ReadDir", "Walk", "WalkDir", "Glob", "ResolveCurrent", "ReconcileExact":
				t.Errorf("%s invokes forbidden local selection or residue promotion %s", entry.Name(), selector.Sel.Name)
			}
			return true
		})
	}
}
