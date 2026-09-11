package architecture_test

import (
	"go/ast"
	"go/parser"
	"path/filepath"
	"strings"
	"testing"
)

func TestLargeOpaqueRawOwnerRequiresConcreteCompositeLease(t *testing.T) {
	root := runtimeGoRoot(t)
	file := filepath.Join(root, "internal", "adapters", "outbound", "rawartifact", "large_opaque_store.go")
	parsed := parseGoFile(t, file, parser.AllErrors)
	for _, wanted := range []string{"NewLargeOpaqueChunkStore", "PrepareLargeOpaqueChunkStoreRecoveryV1"} {
		found := false
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Name.Name != wanted {
				continue
			}
			found = true
			if function.Type.Params == nil || len(function.Type.Params.List) == 0 {
				t.Fatalf("%s has no concrete owner parameter", wanted)
			}
			parameter := function.Type.Params.List[len(function.Type.Params.List)-1]
			pointer, ok := parameter.Type.(*ast.StarExpr)
			if !ok {
				t.Fatalf("%s owner parameter is not a concrete pointer", wanted)
			}
			selector, ok := pointer.X.(*ast.SelectorExpr)
			packageName, packageOK := selector.X.(*ast.Ident)
			if !ok || !packageOK || packageName.Name != "persistencefs" || selector.Sel.Name != "CompositeLease" {
				t.Fatalf("%s owner parameter is not *persistencefs.CompositeLease", wanted)
			}
		}
		if !found {
			t.Fatalf("%s is missing", wanted)
		}
	}
}

func TestLargeOpaqueStoreDoesNotReuseMaterializedPrivateCAS(t *testing.T) {
	root := runtimeGoRoot(t)
	directory := filepath.Join(root, "internal", "adapters", "outbound", "finalauthority")
	forbidden := map[string]bool{
		"SecurePrivateCAS":                  true,
		"securePrivateCASList":              true,
		"securePrivateCASVisit":             true,
		"securePrivateCASValidateInventory": true,
		"capturePrivateCASShardIdentities":  true,
		"attachPrivateCASRootGeneration":    true,
		"ReadAll":                           true,
		"ReadFile":                          true,
		"Buffer":                            true,
	}
	for _, file := range goFiles(t, directory) {
		name := filepath.Base(file)
		if !strings.HasPrefix(name, "large_opaque_") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		ast.Inspect(parsed, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok && identifier.Name == "Fclonefileat" {
				if name != "large_opaque_stage_darwin.go" {
					t.Fatalf("%s contains Fclonefileat outside the exact Darwin commit adapter", rel(t, root, file))
				}
				return true
			}
			if ok && forbidden[identifier.Name] {
				t.Fatalf("%s references materialized private CAS identifier %s", rel(t, root, file), identifier.Name)
			}
			return true
		})
	}
}

func TestLargeOpaqueRawStoreRemainsUncomposedUntilAuthorityAndActivationChainExists(t *testing.T) {
	root := runtimeGoRoot(t)
	for _, file := range goFiles(t, root) {
		relative := rel(t, root, file)
		if strings.HasSuffix(relative, "_test.go") ||
			strings.HasPrefix(relative, "internal/adapters/outbound/finalauthority/large_opaque_") ||
			relative == "internal/adapters/outbound/rawartifact/large_opaque_store.go" {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := ""
			switch invoked := call.Fun.(type) {
			case *ast.Ident:
				name = invoked.Name
			case *ast.SelectorExpr:
				name = invoked.Sel.Name
			}
			if name == "NewLargeOpaqueStore" || name == "NewLargeOpaqueChunkStore" {
				t.Fatalf("%s composes large opaque raw storage before authenticated acquisition, receipt, witness, and activation-guard integration", relative)
			}
			return true
		})
	}
}

func TestLargeOpaqueImplementationNeverDeletesRenamesOrOverwritesPathNamedObjects(t *testing.T) {
	root := runtimeGoRoot(t)
	directory := filepath.Join(root, "internal", "adapters", "outbound", "finalauthority")
	forbidden := map[string]bool{
		"Chmod":                                  true,
		"Ftruncate":                              true,
		"O_TRUNC":                                true,
		"Rename":                                 true,
		"Renameat":                               true,
		"Remove":                                 true,
		"RemoveAll":                              true,
		"Unlink":                                 true,
		"Unlinkat":                               true,
		"largeOpaqueUnixRemoveOwnedTemp":         true,
		"newPrivateCASRootAuthority":             true,
		"privateCASUnixRemoveOwnedCreateResidue": true,
		"privateCASUnixCreateBoundDirectory":     true,
		"securePrivateCommitNoReplace":           true,
		"Truncate":                               true,
		"withPrivateCASAccess":                   true,
	}
	for _, file := range goFiles(t, directory) {
		name := filepath.Base(file)
		if !strings.HasPrefix(name, "large_opaque_") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		ast.Inspect(parsed, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok && forbidden[identifier.Name] {
				t.Fatalf("%s contains path-named mutation %s", rel(t, root, file), identifier.Name)
			}
			return true
		})
	}
}

func TestLargeOpaqueDarwinCloneCommitIsExactFDToNoReplaceName(t *testing.T) {
	root := runtimeGoRoot(t)
	file := filepath.Join(
		root,
		"internal",
		"adapters",
		"outbound",
		"finalauthority",
		"large_opaque_stage_darwin.go",
	)
	parsed := parseGoFile(t, file, 0)
	count := 0
	ast.Inspect(parsed, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		packageName, packageOK := selector.X.(*ast.Ident)
		if !packageOK || packageName.Name != "unix" || selector.Sel.Name != "Fclonefileat" {
			return true
		}
		count++
		if len(call.Args) != 4 ||
			!largeOpaqueExactIdentifier(call.Args[0], "sourceFD") ||
			!largeOpaqueExactIdentifier(call.Args[1], "shard") ||
			!largeOpaqueExactIdentifier(call.Args[2], "finalName") ||
			!largeOpaqueExactSelector(call.Args[3], "unix", "CLONE_NOOWNERCOPY") {
			t.Fatalf("%s Fclonefileat is not the exact source-FD/no-replace destination commit", rel(t, root, file))
		}
		return true
	})
	if count != 1 {
		t.Fatalf("%s has %d Fclonefileat calls, want exactly one", rel(t, root, file), count)
	}
}

func largeOpaqueExactIdentifier(expression ast.Expr, name string) bool {
	identifier, ok := expression.(*ast.Ident)
	return ok && identifier.Name == name
}

func largeOpaqueExactSelector(expression ast.Expr, packageName string, name string) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	identifier, packageOK := selector.X.(*ast.Ident)
	return ok && packageOK && identifier.Name == packageName && selector.Sel.Name == name
}

func TestLargeOpaqueSecurityTestsNeverSkipRequiredCoverage(t *testing.T) {
	root := runtimeGoRoot(t)
	for _, directory := range []string{
		filepath.Join(root, "internal", "adapters", "outbound", "finalauthority"),
		filepath.Join(root, "internal", "adapters", "outbound", "rawartifact"),
	} {
		for _, file := range goFiles(t, directory) {
			name := filepath.Base(file)
			if !strings.HasPrefix(name, "large_opaque_") || !strings.HasSuffix(name, "_test.go") {
				continue
			}
			parsed := parseGoFile(t, file, 0)
			ast.Inspect(parsed, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if ok && (selector.Sel.Name == "Skip" || selector.Sel.Name == "Skipf" || selector.Sel.Name == "SkipNow") {
					t.Fatalf("%s skips required LargeOpaque security coverage with %s", rel(t, root, file), selector.Sel.Name)
				}
				return true
			})
		}
	}
}
