package architecture_test

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestWitnessedEvidenceRegistryStoresCannotSelectLocalCurrentAuthority(t *testing.T) {
	root := runtimeGoRoot(t)
	for _, directory := range []string{
		filepath.Join(root, "internal", "ports", "evidenceregistry"),
		filepath.Join(root, "internal", "adapters", "outbound", "evidenceregistry"),
	} {
		for _, path := range goFiles(t, directory) {
			if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "store.go") || strings.HasSuffix(path, "authority_index.go") {
				// Legacy V1 local-index files remain audit/migration material until
				// the production composition switches to the V2 service. The V2
				// stores themselves must never acquire a local current selector.
				continue
			}
			parsed := parseGoFile(t, path, 0)
			ast.Inspect(parsed, func(node ast.Node) bool {
				switch typed := node.(type) {
				case *ast.SelectorExpr:
					switch typed.Sel.Name {
					case "ReadDir", "Walk", "WalkDir", "Glob":
						t.Errorf("%s invokes forbidden local registry authority inventory %s", rel(t, root, path), typed.Sel.Name)
					}
				case *ast.TypeSpec:
					interfaceType, ok := typed.Type.(*ast.InterfaceType)
					if !ok {
						return true
					}
					for _, method := range interfaceType.Methods.List {
						for _, name := range method.Names {
							switch name.Name {
							case "List", "Current", "ResolveCurrent", "Active", "Latest":
								t.Errorf("%s exposes forbidden registry authority selector %s", rel(t, root, path), name.Name)
							}
						}
					}
				}
				return true
			})
		}
	}
}

func TestEvidenceRegistryChildAdvanceHasSingleProductionCaller(t *testing.T) {
	root := runtimeGoRoot(t)
	allowed := "internal/app/evidenceregistry/service.go"
	for _, path := range goFiles(t, root) {
		if strings.HasSuffix(path, "_test.go") || hasBuildTag(t, path, "!analytix_prod") {
			continue
		}
		parsed := parseGoFile(t, path, 0)
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if ok && selector.Sel.Name == "AdvanceEvidenceRegistry" && rel(t, root, path) != allowed {
				t.Fatalf("%s bypasses the sole evidence registry authority writer", rel(t, root, path))
			}
			return true
		})
	}
}

func TestFactFinalWitnessCapabilityHasSingleProductionImplementation(t *testing.T) {
	root := runtimeGoRoot(t)
	methods := map[string]map[string]bool{}
	for _, path := range goFiles(t, root) {
		if strings.HasSuffix(path, "_test.go") || hasBuildTag(t, path, "!analytix_prod") {
			continue
		}
		relative := rel(t, root, path)
		parsed := parseGoFile(t, path, 0)
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || len(function.Recv.List) != 1 ||
				(function.Name.Name != "PrivateFinal" && function.Name.Name != "UseExact") {
				continue
			}
			receiver := ""
			switch typed := function.Recv.List[0].Type.(type) {
			case *ast.Ident:
				receiver = typed.Name
			case *ast.StarExpr:
				if name, ok := typed.X.(*ast.Ident); ok {
					receiver = name.Name
				}
			}
			if receiver == "" {
				continue
			}
			key := relative + "#" + receiver
			if methods[key] == nil {
				methods[key] = map[string]bool{}
			}
			methods[key][function.Name.Name] = true
		}
	}
	implementations := []string{}
	for key, names := range methods {
		if names["PrivateFinal"] && names["UseExact"] {
			implementations = append(implementations, key)
		}
	}
	if len(implementations) != 1 || implementations[0] != "internal/app/evidenceregistry/service.go#factFinalWitnessCapabilityV1" {
		t.Fatalf("fact-final witness capability implementation is not unique: %v", implementations)
	}
}

func TestRuntimeCompositionUsesOneWitnessedRegistryForHostFactFinalization(t *testing.T) {
	root := runtimeGoRoot(t)
	path := filepath.Join(root, "internal", "runtimeapp", "app.go")
	parsed := parseGoFile(t, path, 0)
	selectedSharedRegistry := 0
	hostConstructors := 0
	compatibilityConstructors := 0
	exactCaseFinalizerInitializers := 0
	ast.Inspect(parsed, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.AssignStmt:
			if len(typed.Lhs) != 1 || len(typed.Rhs) != 1 {
				return true
			}
			left, leftOK := typed.Lhs[0].(*ast.Ident)
			if !leftOK {
				return true
			}
			if right, rightOK := typed.Rhs[0].(*ast.SelectorExpr); rightOK {
				shared, sharedOK := right.X.(*ast.Ident)
				if sharedOK && left.Name == "evidenceStore" &&
					shared.Name == "sharedEvidenceDatasetSnapshotV2" && right.Sel.Name == "registry" {
					selectedSharedRegistry++
				}
			}
			if left.Name == "caseFinalizer" {
				outer, ok := typed.Rhs[0].(*ast.CallExpr)
				if !ok || !runtimeEvidenceRegistryPackageCall(outer, "evidenceapp", "WithToolEvidenceAuthority") ||
					len(outer.Args) < 1 {
					t.Fatal("production caseFinalizer is not initialized through WithToolEvidenceAuthority")
				}
				inner, ok := outer.Args[0].(*ast.CallExpr)
				if !ok || !runtimeEvidenceRegistryPackageCall(
					inner, "evidenceapp", "NewCasePublicationFinalizerWithHostEvidenceAuthority",
				) || len(inner.Args) < 2 {
					t.Fatal("production caseFinalizer does not directly wrap the host evidence authority constructor")
				}
				registry, registryOK := inner.Args[0].(*ast.Ident)
				issuer, issuerOK := inner.Args[1].(*ast.Ident)
				if !registryOK || !issuerOK || registry.Name != "evidenceStore" || issuer.Name != registry.Name {
					t.Fatal("production caseFinalizer initializer does not share the exact evidenceStore instance")
				}
				exactCaseFinalizerInitializers++
			}
		case *ast.CallExpr:
			selector, ok := typed.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch selector.Sel.Name {
			case "NewCasePublicationFinalizerWithPublicationSnapshots":
				compatibilityConstructors++
			case "NewCasePublicationFinalizerWithHostEvidenceAuthority":
				hostConstructors++
				if len(typed.Args) < 2 {
					t.Fatal("production host finalizer constructor lost registry arguments")
				}
				registry, registryOK := typed.Args[0].(*ast.Ident)
				issuer, issuerOK := typed.Args[1].(*ast.Ident)
				if !registryOK || !issuerOK || registry.Name != "evidenceStore" || issuer.Name != registry.Name {
					t.Fatal("production host finalizer does not receive the same evidence registry and witness issuer instance")
				}
			}
		}
		return true
	})
	if selectedSharedRegistry != 1 || exactCaseFinalizerInitializers != 1 ||
		hostConstructors != 1 || compatibilityConstructors != 0 {
		t.Fatalf(
			"production witnessed registry composition drifted: sharedSelections=%d exactInitializers=%d hostConstructors=%d compatibilityConstructors=%d",
			selectedSharedRegistry, exactCaseFinalizerInitializers, hostConstructors, compatibilityConstructors,
		)
	}
}

func TestRuntimeSharedEvidenceRegistryOpensOnlyAfterNonEmptySemanticActivation(t *testing.T) {
	root := runtimeGoRoot(t)
	path := filepath.Join(root, "internal", "runtimeapp", "shared_evidence_dataset_snapshot_v2.go")
	parsed := parseGoFile(t, path, 0)
	var target *ast.FuncDecl
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "newRuntimeSharedEvidenceDatasetSnapshotV2" {
			target = function
			break
		}
	}
	if target == nil || target.Body == nil {
		t.Fatal("runtime shared evidence composition function is unavailable")
	}

	guardIndex := -1
	for index, statement := range target.Body.List {
		conditional, ok := statement.(*ast.IfStmt)
		if !ok {
			continue
		}
		binary, binaryOK := conditional.Cond.(*ast.BinaryExpr)
		if !binaryOK || binary.Op != token.LOR {
			continue
		}
		negated, negatedOK := binary.Y.(*ast.UnaryExpr)
		call, callOK := func() (*ast.CallExpr, bool) {
			if !negatedOK || negated.Op != token.NOT {
				return nil, false
			}
			value, ok := negated.X.(*ast.CallExpr)
			return value, ok
		}()
		if !callOK {
			continue
		}
		selector, selectorOK := call.Fun.(*ast.SelectorExpr)
		receiver, receiverOK := func() (*ast.Ident, bool) {
			if !selectorOK {
				return nil, false
			}
			value, ok := selector.X.(*ast.Ident)
			return value, ok
		}()
		if !receiverOK || receiver.Name != "preparedRegistry" ||
			selector.Sel.Name != "WitnessedV2ActivationAllowed" {
			continue
		}
		_, returns := func() (*ast.ReturnStmt, bool) {
			if len(conditional.Body.List) != 1 {
				return nil, false
			}
			value, ok := conditional.Body.List[0].(*ast.ReturnStmt)
			return value, ok
		}()
		if !returns || guardIndex != -1 {
			t.Fatal("runtime shared registry activation guard is not one exact early return")
		}
		guardIndex = index
	}
	if guardIndex < 0 {
		t.Fatal("runtime shared registry has no non-empty semantic activation guard")
	}

	constructors := []struct {
		packageName string
		function    string
	}{
		{packageName: "evidenceregistrystore", function: "NewAuthorityIndexStoreV2"},
		{packageName: "evidenceregistrystore", function: "NewAuthorityCapsuleStoreV2"},
		{packageName: "evidenceregistryapp", function: "New"},
	}
	for _, constructor := range constructors {
		before := 0
		after := 0
		for index, statement := range target.Body.List {
			count := runtimeEvidenceRegistryCallCount(statement, constructor.packageName, constructor.function)
			if index <= guardIndex {
				before += count
			} else {
				after += count
			}
		}
		if before != 0 || after != 1 {
			t.Fatalf("%s.%s crosses registry activation guard: before=%d after=%d", constructor.packageName, constructor.function, before, after)
		}
	}
}

func runtimeEvidenceRegistryCallCount(node ast.Node, packageName string, functionName string) int {
	count := 0
	ast.Inspect(node, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if ok && runtimeEvidenceRegistryPackageCall(call, packageName, functionName) {
			count++
		}
		return true
	})
	return count
}

func runtimeEvidenceRegistryPackageCall(call *ast.CallExpr, packageName string, functionName string) bool {
	if call == nil {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != functionName {
		return false
	}
	owner, ok := selector.X.(*ast.Ident)
	return ok && owner.Name == packageName
}
