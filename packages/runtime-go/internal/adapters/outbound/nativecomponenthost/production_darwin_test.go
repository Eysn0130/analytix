//go:build darwin

package nativecomponenthost

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"
)

func TestProductionAdmissionDelegatesOnlyToDarwinTrustCandidate(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("production admission test source path unavailable")
	}
	parsed, err := parser.ParseFile(
		token.NewFileSet(),
		filepath.Join(filepath.Dir(currentFile), "production_darwin.go"),
		nil,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range parsed.Decls {
		function, isFunction := declaration.(*ast.FuncDecl)
		if !isFunction || function.Name.Name != "OpenProduction" {
			continue
		}
		if function.Body == nil || len(function.Body.List) != 1 {
			t.Fatal("OpenProduction must contain only the Darwin trust-candidate delegation")
		}
		returnStatement, ok := function.Body.List[0].(*ast.ReturnStmt)
		if !ok || len(returnStatement.Results) != 1 {
			t.Fatal("OpenProduction must return exactly the Darwin trust-candidate result")
		}
		call, ok := returnStatement.Results[0].(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			t.Fatal("OpenProduction must make one Darwin trust-candidate call")
		}
		callee, calleeOK := call.Fun.(*ast.Ident)
		profileRoot, profileRootOK := call.Args[0].(*ast.Ident)
		if !calleeOK || callee.Name != "openDarwinHostCandidate" ||
			!profileRootOK || profileRoot.Name != "profileRoot" {
			t.Fatal("OpenProduction bypassed the Darwin trust-candidate admission")
		}
		return
	}
	t.Fatal("OpenProduction production admission function is missing")
}
