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
			physicalInventory := map[*ast.SelectorExpr]bool{}
			for _, declaration := range parsed.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if !ok || !registryOriginalObservationOwner(rel(t, root, path), function) {
					continue
				}
				if problem := registryOriginalObservationProblem(rel(t, root, path), function); problem != "" {
					t.Errorf("%s#%s: %s", rel(t, root, path), function.Name.Name, problem)
					continue
				}
				ast.Inspect(function.Body, func(node ast.Node) bool {
					if selector, ok := node.(*ast.SelectorExpr); ok {
						physicalInventory[selector] = true
					}
					return true
				})
			}
			ast.Inspect(parsed, func(node ast.Node) bool {
				switch typed := node.(type) {
				case *ast.SelectorExpr:
					switch typed.Sel.Name {
					case "ReadDir", "Walk", "WalkDir", "Glob":
						if physicalInventory[typed] {
							break
						}
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

// Original observation is not current selection. Its narrow exception requires
// a prepared physical owner, only the reviewed read graph, and both preflight
// and deferred revalidation. It never exempts the rest of either source file.
func registryOriginalObservationOwner(relative string, function *ast.FuncDecl) bool {
	owners := map[string]string{
		"SnapshotOriginalLegacyInventoryV1": "original_inventory.go",
		"SnapshotOriginalLegacyFilesV1":     "original_inventory.go",
		"readOriginalRegistryBodyV1":        "original_inventory.go",
		"SnapshotOriginalFilesV1":           "original_inventory_v2.go",
		"snapshotOriginalFilesV2":           "original_inventory_v2.go",
		"validateOriginalOpenedFileV2":      "original_inventory_v2.go",
	}
	file, found := owners[function.Name.Name]
	if !found || relative != "internal/adapters/outbound/evidenceregistry/"+file {
		return false
	}
	if function.Name.Name == "readOriginalRegistryBodyV1" {
		return function.Recv == nil
	}
	if function.Recv == nil || len(function.Recv.List) != 1 || len(function.Recv.List[0].Names) != 1 || function.Recv.List[0].Names[0].Name != "prepared" {
		return false
	}
	pointer, ok := function.Recv.List[0].Type.(*ast.StarExpr)
	return ok && privateCASIdent(pointer.X) == "PreparedRecoveryV2"
}

func registryObservationExpressionName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return registryObservationExpressionName(value.X) + "." + value.Sel.Name
	case *ast.CallExpr:
		return registryObservationExpressionName(value.Fun) + "()"
	default:
		return ""
	}
}

func registryOriginalObservationDefersRevalidation(function *ast.FuncDecl) bool {
	found := false
	ast.Inspect(function.Body, func(node ast.Node) bool {
		deferred, ok := node.(*ast.DeferStmt)
		if !ok {
			return true
		}
		ast.Inspect(deferred.Call, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if ok && privateCASCallExpressionMatches(call, "prepared", "RevalidatePhysicalV2", 1) && privateCASIdent(call.Args[0]) == "ctx" {
				found = true
			}
			return true
		})
		return true
	})
	return found
}

func registryOriginalObservationProblem(relative string, function *ast.FuncDecl) string {
	if !registryOriginalObservationOwner(relative, function) || function.Body == nil {
		return "original inventory is outside its prepared observation owner"
	}
	allowed := map[string]bool{}
	for _, name := range strings.Fields(`
		len int64 uint32 make
		errors.New errors.Join io.ReadAll io.LimitReader
		os.Open os.Lstat os.SameFile
		filepath.Rel filepath.ToSlash filepath.Join filepath.FromSlash
		path.Join path.Dir strings.Split strings.TrimSuffix
		domainsecurity.SHA256Hex domainprivatecas.ValidShardV1
		domainprivatecas.ValidDigestV1 domainprivatecas.ClassifyRecordResidueNameV1
		ctx.Err entry.Info entry.Name file.Close file.Stat
		before.IsDir before.Mode before.Mode().Perm before.Mode().IsRegular
		before.Size before.ModTime before.ModTime().Equal
		opened.Mode opened.Size opened.ModTime
		prepared.RevalidatePhysicalV2 prepared.indexes.Present
		prepared.capsules.Present prepared.topology.PresentV1
		prepared.SnapshotOriginalLegacyFilesV1 prepared.snapshotOriginalFilesV2
		prepared.validateOriginalOpenedFileV2 plan.VisitCommittedFiles
		readOriginalRegistryBodyV1 validateOpenedRegistryFile registryRegularFileLinkCount
		ParseOriginalLegacyInventoryV1
	`) {
		allowed[name] = true
	}
	snapshot := function.Name.Name == "SnapshotOriginalLegacyFilesV1" || function.Name.Name == "snapshotOriginalFilesV2"
	if snapshot {
		allowed["filepath.WalkDir"] = true
		if len(privateCASMatchingCalls(function, "prepared", "RevalidatePhysicalV2")) != 2 || !registryOriginalObservationDefersRevalidation(function) {
			return "original inventory lost its complete physical revalidation bracket"
		}
		if function.Type.Results == nil || len(function.Type.Results.List) != 2 {
			return "original inventory no longer returns only raw entries and error"
		}
		entries, ok := function.Type.Results.List[0].Type.(*ast.MapType)
		if !ok || privateCASIdent(entries.Key) != "string" || privateCASIdent(entries.Value) != "OriginalLegacyEntryV1" {
			return "original inventory exports authority instead of raw entries"
		}
	}
	if function.Name.Name == "validateOriginalOpenedFileV2" {
		allowed["os.ReadDir"] = true
	}
	problem := ""
	ast.Inspect(function.Body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.Ident:
			if strings.HasPrefix(value.Name, "New") && value.Name != "New" {
				problem = "original inventory references an authority constructor " + value.Name
			}
		case *ast.GoStmt:
			problem = "original inventory escapes synchronous physical observation"
		case *ast.CallExpr:
			if _, deferred := value.Fun.(*ast.FuncLit); deferred {
				return true
			}
			name := registryObservationExpressionName(value.Fun)
			if !allowed[name] {
				problem = "original inventory invokes unreviewed effect or selector " + name
			}
			if name == "filepath.WalkDir" && (len(value.Args) != 2 || !privateCASSelectorMatches(value.Args[0], "prepared", "root")) {
				problem = "original inventory walks outside its prepared root"
			}
		case *ast.SelectorExpr:
			// Method values cannot smuggle a writer past the call checker.
			for _, forbidden := range []string{"Write", "Put", "Create", "Remove", "Rename", "Sign", "Advance", "Current", "Latest", "ResolveCurrent"} {
				if strings.HasPrefix(value.Sel.Name, forbidden) {
					problem = "original inventory references a writer or current selector " + value.Sel.Name
				}
			}
		case *ast.AssignStmt:
			for _, target := range value.Lhs {
				name := registryObservationExpressionName(target)
				if name == "prepared" || strings.HasPrefix(name, "prepared.") {
					problem = "original inventory mutates its prepared physical owner"
				}
			}
		case *ast.CompositeLit:
			if _, imported := value.Type.(*ast.SelectorExpr); imported {
				problem = "original inventory constructs an imported capability"
			}
			if name, ok := value.Type.(*ast.Ident); ok && name.Name != "OriginalLegacyEntryV1" && name.Name != "originalRegistryCommittedBodyV2" {
				problem = "original inventory constructs an unreviewed value " + name.Name
			}
		}
		return true
	})
	return problem
}

func TestOriginalRegistryInventoryGuardRejectsAuthorityAndObservationMutations(t *testing.T) {
	root := runtimeGoRoot(t)
	const relative = "internal/adapters/outbound/evidenceregistry/original_inventory_v2.go"
	path := filepath.Join(root, filepath.FromSlash(relative))
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	cases := map[string]string{
		"original":              source,
		"current selector":      strings.Replace(source, "files = map[string]OriginalLegacyEntryV1{}", "_ = prepared.ResolveCurrent(ctx)\nfiles = map[string]OriginalLegacyEntryV1{}", 1),
		"writer":                strings.Replace(source, "files = map[string]OriginalLegacyEntryV1{}", "_ = os.WriteFile(prepared.root, nil, 0600)\nfiles = map[string]OriginalLegacyEntryV1{}", 1),
		"writer method value":   strings.Replace(source, "files = map[string]OriginalLegacyEntryV1{}", "write := os.WriteFile\n_ = write\nfiles = map[string]OriginalLegacyEntryV1{}", 1),
		"authority constructor": strings.Replace(source, "files = map[string]OriginalLegacyEntryV1{}", "_ = NewStore(prepared.root, nil)\nfiles = map[string]OriginalLegacyEntryV1{}", 1),
		"authority literal":     strings.Replace(source, "files = map[string]OriginalLegacyEntryV1{}", "_ = Store{}\nfiles = map[string]OriginalLegacyEntryV1{}", 1),
		"different root":        strings.Replace(source, "filepath.WalkDir(prepared.root,", "filepath.WalkDir(otherRoot,", 1),
		"missing revalidation":  strings.Replace(source, "prepared.RevalidatePhysicalV2(ctx), ctx.Err()", "ctx.Err()", 1),
		"mutated owner":         strings.Replace(source, "files = map[string]OriginalLegacyEntryV1{}", "prepared.root = otherRoot\nfiles = map[string]OriginalLegacyEntryV1{}", 1),
		"different owner":       strings.Replace(source, "func (prepared *PreparedRecoveryV2) snapshotOriginalFilesV2", "func (prepared *Store) snapshotOriginalFilesV2", 1),
	}
	for name, candidate := range cases {
		t.Run(name, func(t *testing.T) {
			if name != "original" && candidate == source {
				t.Fatal("mutation did not change the source")
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), relative, candidate, 0)
			if err != nil {
				t.Fatal(err)
			}
			function := privateCASFunctionDeclaration(t, parsed, "snapshotOriginalFilesV2")
			problem := registryOriginalObservationProblem(relative, function)
			if (problem == "") != (name == "original") {
				t.Fatalf("guard result: %s", problem)
			}
		})
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
	selectedImportActivation := 0
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
					shared.Name == "sharedEvidenceDatasetSnapshotV2" && right.Sel.Name == "registryOwner" {
					selectedSharedRegistry++
				}
			}
			if left.Name == "activateImportRegistry" {
				method, ok := typed.Rhs[0].(*ast.SelectorExpr)
				if !ok || method.Sel.Name != "ActivateAfterImport" {
					t.Fatal("confirmed import activation lost its shared registry method")
				}
				owner, ok := method.X.(*ast.SelectorExpr)
				if !ok || owner.Sel.Name != "registryOwner" {
					t.Fatal("confirmed import activation bypasses the registry owner")
				}
				shared, ok := owner.X.(*ast.Ident)
				if !ok || shared.Name != "sharedEvidenceDatasetSnapshotV2" {
					t.Fatal("confirmed import activation selects a different shared registry")
				}
				selectedImportActivation++
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
	if selectedSharedRegistry != 1 || selectedImportActivation != 1 || exactCaseFinalizerInitializers != 1 ||
		hostConstructors != 1 || compatibilityConstructors != 0 {
		t.Fatalf(
			"production witnessed registry composition drifted: sharedSelections=%d importActivations=%d exactInitializers=%d hostConstructors=%d compatibilityConstructors=%d",
			selectedSharedRegistry, selectedImportActivation, exactCaseFinalizerInitializers, hostConstructors, compatibilityConstructors,
		)
	}
}

func TestRuntimeSharedEvidenceRegistryOpensOnlyAfterNonEmptySemanticActivation(t *testing.T) {
	root := runtimeGoRoot(t)
	file := parseGoFile(t, filepath.Join(root, "internal", "runtimeapp", "shared_evidence_dataset_snapshot_v2.go"), 0)
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok && function.Name.Name == "newRuntimeSharedEvidenceDatasetSnapshotV2" {
			if issue := sharedRegistryActivationIssue(function); issue != "" {
				t.Fatal(issue)
			}
			return
		}
	}
	t.Fatal("runtime shared evidence composition function is unavailable")
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
