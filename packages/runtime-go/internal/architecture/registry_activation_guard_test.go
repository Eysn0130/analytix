package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const registryDeferredActivationContract = `func guard() {
if semanticErr != nil || !preparedRegistry.WitnessedV2ActivationAllowed() {
	result := runtimeSharedEvidenceDatasetSnapshotV2{evidence: evidenceAuthority, snapshot: sealed}
	if semanticErr == nil && preparedRegistry.FreshImportInventoryV2() {
		result.registryOwner = newRuntimeFreshImportRegistryV1(registryRoot, registryAccess, evidenceAuthority, sealed, preparedRegistry, openRegistry)
	}
	return result, true, nil
}
}`

func sharedRegistryActivationIssue(function *ast.FuncDecl) string {
	if function == nil || function.Body == nil {
		return "registry activation function is unavailable"
	}
	definition, guard, invocation := -1, -1, -1
	var open *ast.FuncLit
	references, calls := 0, 0
	for index, statement := range function.Body.List {
		if assignment, ok := statement.(*ast.AssignStmt); ok && len(assignment.Lhs) == 1 && len(assignment.Rhs) == 1 {
			if name, ok := assignment.Lhs[0].(*ast.Ident); ok && name.Name == "openRegistry" {
				if definition != -1 || assignment.Tok != token.DEFINE {
					return "registry open closure is reassigned"
				}
				open, _ = assignment.Rhs[0].(*ast.FuncLit)
				definition = index
			}
		}
		if conditional, ok := statement.(*ast.IfStmt); ok {
			wrapped := &ast.FuncDecl{Name: ast.NewIdent("guard"), Type: &ast.FuncType{Params: &ast.FieldList{}}, Body: &ast.BlockStmt{List: []ast.Stmt{conditional}}}
			if exactRegistryMethod(wrapped, registryDeferredActivationContract) {
				if guard != -1 {
					return "registry activation guard is duplicated"
				}
				guard = index
			}
		}
		ast.Inspect(statement, func(node ast.Node) bool {
			if name, ok := node.(*ast.Ident); ok && name.Name == "openRegistry" {
				references++
			}
			if call, ok := node.(*ast.CallExpr); ok {
				if name, ok := call.Fun.(*ast.Ident); ok && name.Name == "openRegistry" {
					calls++
					invocation = index
				}
			}
			return true
		})
	}
	if open == nil || definition < 0 || guard <= definition || invocation <= guard || calls != 1 || references != 3 {
		return "registry closure must be defined, captured by guarded activation, then invoked once after the early return"
	}
	assignment, ok := function.Body.List[invocation].(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 3 || len(assignment.Rhs) != 1 {
		return "registry activation must consume the immediate open result"
	}
	for index, expected := range []string{"registry", "closeStores", "err"} {
		if name, ok := assignment.Lhs[index].(*ast.Ident); !ok || name.Name != expected {
			return "registry activation changed its result bindings"
		}
	}
	call, ok := assignment.Rhs[0].(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return "registry activation does not directly open its bound closure"
	}
	if name, ok := call.Fun.(*ast.Ident); !ok || name.Name != "openRegistry" {
		return "registry activation changed its open owner"
	}
	for _, constructor := range [][2]string{
		{"evidenceregistrystore", "NewAuthorityIndexStoreV2"},
		{"evidenceregistrystore", "NewAuthorityCapsuleStoreV2"},
		{"evidenceregistryapp", "New"},
	} {
		if runtimeEvidenceRegistryCallCount(open.Body, constructor[0], constructor[1]) != 1 ||
			runtimeEvidenceRegistryCallCount(function.Body, constructor[0], constructor[1]) != 1 {
			return "registry construction escaped its one deferred open closure"
		}
	}
	return ""
}

func TestRegistryActivationGuardRejectsEagerOrUnboundConstruction(t *testing.T) {
	file := filepath.Join(runtimeGoRoot(t), "internal", "runtimeapp", "shared_evidence_dataset_snapshot_v2.go")
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range [][2]string{
		{"if semanticErr != nil || !preparedRegistry.WitnessedV2ActivationAllowed() {", "openRegistry(); if semanticErr != nil || !preparedRegistry.WitnessedV2ActivationAllowed() {"},
		{"semanticErr == nil && preparedRegistry.FreshImportInventoryV2()", "preparedRegistry.FreshImportInventoryV2()"},
		{"preparedRegistry, openRegistry)", "preparedRegistry, replacement)"},
		{"registry, closeStores, err := openRegistry()", "registry, closeStores, err := other()"},
		{"registry, closeStores, err := openRegistry()", "defer openRegistry()"},
		{"registry, closeStores, err := openRegistry()", "evidenceregistrystore.NewAuthorityIndexStoreV2(); registry, closeStores, err := openRegistry()"},
	} {
		if !strings.Contains(string(body), mutation[0]) {
			t.Fatal("activation mutation no longer reaches its target")
		}
		changed, err := parser.ParseFile(token.NewFileSet(), "fixture.go", strings.ReplaceAll(string(body), mutation[0], mutation[1]), parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range changed.Decls {
			if function, ok := declaration.(*ast.FuncDecl); ok && function.Name.Name == "newRuntimeSharedEvidenceDatasetSnapshotV2" && sharedRegistryActivationIssue(function) == "" {
				t.Fatalf("activation guard accepted %q", mutation[0])
			}
		}
	}
}

func TestFreshRegistryFactoryOnlyCapturesDeferredActivation(t *testing.T) {
	file := parseGoFile(t, filepath.Join(runtimeGoRoot(t), "internal", "runtimeapp", "import_activated_evidence_registry_v1.go"), 0)
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "newRuntimeFreshImportRegistryV1" {
			continue
		}
		if function.Body == nil || len(function.Body.List) != 1 {
			t.Fatal("fresh registry factory performs work before activation")
		}
		returned, ok := function.Body.List[0].(*ast.ReturnStmt)
		if !ok || len(returned.Results) != 1 {
			t.Fatal("fresh registry factory does not return one deferred owner")
		}
		address, ok := returned.Results[0].(*ast.UnaryExpr)
		if !ok || address.Op != token.AND {
			t.Fatal("fresh registry factory invokes rather than captures its owner")
		}
		literal, ok := address.X.(*ast.CompositeLit)
		if !ok || len(literal.Elts) != 1 {
			t.Fatal("fresh registry factory evaluates unexpected fields")
		}
		if owner, ok := literal.Type.(*ast.Ident); !ok || owner.Name != "runtimeImportActivatedRegistryV1" {
			t.Fatal("fresh registry factory substitutes its shared owner type")
		}
		field, ok := literal.Elts[0].(*ast.KeyValueExpr)
		if !ok {
			t.Fatal("fresh registry factory lost its activation field")
		}
		name, named := field.Key.(*ast.Ident)
		_, deferred := field.Value.(*ast.FuncLit)
		if !named || name.Name != "activate" || !deferred {
			t.Fatal("fresh registry factory eagerly evaluates activation")
		}
		return
	}
	t.Fatal("fresh registry factory is missing")
}
