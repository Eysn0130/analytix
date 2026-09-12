package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
)

func TestPrivateCASProductionCodeCannotUseTestAccessAuthority(t *testing.T) {
	root := runtimeGoRoot(t)
	const forbidden = "analytix.local/runtime-go/internal/testsupport/privatecas"
	for _, file := range goFiles(t, root) {
		relative := rel(t, root, file)
		if strings.HasSuffix(file, "_test.go") || strings.HasPrefix(relative, "internal/testsupport/") {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		for _, imported := range parsed.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err == nil && path == forbidden {
				t.Fatalf("%s imports the test-only private CAS access authority", relative)
			}
		}
	}
}

func TestPrivateCASAccessAuthorityHasOnlyPersistenceFSProductionImplementations(t *testing.T) {
	root := runtimeGoRoot(t)
	for _, file := range goFiles(t, root) {
		relative := rel(t, root, file)
		if strings.HasSuffix(file, "_test.go") || strings.HasPrefix(relative, "internal/testsupport/") {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || function.Name.Name != "WithPrivateCASAccess" {
				continue
			}
			if !strings.HasPrefix(relative, "internal/adapters/outbound/persistencefs/") {
				t.Fatalf("%s implements production private CAS access authority outside persistencefs", relative)
			}
		}
	}
}

func TestPrivateCASHasNoAuthorityFreeProductionOpenPath(t *testing.T) {
	root := runtimeGoRoot(t)
	for _, file := range goFiles(t, root) {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		relative := rel(t, root, file)
		if relative != "internal/adapters/outbound/finalauthority/private_cas.go" && hasPrivateCASConstruction(parsed) {
			t.Fatalf("%s bypasses the sole private CAS constructor", relative)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			function, ok := node.(*ast.FuncDecl)
			if ok && function.Name.Name == "OpenSecurePrivateCAS" {
				t.Fatalf("%s restores an authority-free private CAS constructor", relative)
			}
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
			if name == "OpenSecurePrivateCASWithAccessAuthority" || name == "OpenSecurePrivateCASWithAccessAuthorityContext" {
				expected := 3
				authorityIndex := 2
				if name == "OpenSecurePrivateCASWithAccessAuthorityContext" {
					expected = 4
					authorityIndex = 3
				}
				if len(call.Args) != expected {
					t.Fatalf("%s opens private CAS without the required authority contract", relative)
				}
				if identifier, ok := call.Args[authorityIndex].(*ast.Ident); ok && identifier.Name == "nil" {
					t.Fatalf("%s passes nil as private CAS access authority", relative)
				}
			}
			return true
		})
	}
}

// A container of existing CAS handles does not construct CAS authority. Inspect
// the literal's own type, rather than matching the element type's spelling.
func hasPrivateCASConstruction(node ast.Node) bool {
	found := false
	ast.Inspect(node, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.CallExpr:
			found = found || privateCASCallName(typed) == "openSecurePrivateCAS"
		case *ast.CompositeLit:
			found = found || privateCASLiteralConstructsAuthority(typed, typed.Type)
		}
		return !found
	})
	return found
}

func privateCASLiteralConstructsAuthority(literal *ast.CompositeLit, kind ast.Expr) bool {
	if literal.Type != nil {
		kind = literal.Type
	}
	if pointer, ok := kind.(*ast.StarExpr); ok {
		kind = pointer.X
	}
	var element ast.Expr
	switch typed := kind.(type) {
	case *ast.Ident:
		return typed.Name == "SecurePrivateCAS"
	case *ast.SelectorExpr:
		return typed.Sel.Name == "SecurePrivateCAS"
	case *ast.ArrayType:
		element = typed.Elt
	case *ast.MapType:
		element = typed.Value
	}
	if element == nil {
		return false
	}
	for _, value := range literal.Elts {
		if keyed, ok := value.(*ast.KeyValueExpr); ok {
			value = keyed.Value
		}
		if nested, ok := value.(*ast.CompositeLit); ok && privateCASLiteralConstructsAuthority(nested, element) {
			return true
		}
	}
	return false
}

func TestPrivateCASConstructionGuardDistinguishesHandlesFromAuthority(t *testing.T) {
	for _, tc := range []struct {
		source    string
		violation bool
	}{
		{"[]*owner.SecurePrivateCAS{first, second}", false},
		{"map[string]*owner.SecurePrivateCAS{\"first\": first}", false},
		{"owner.SecurePrivateCAS{}", true},
		{"&SecurePrivateCAS{}", true},
		{"[]*owner.SecurePrivateCAS{{}}", true},
		{"map[string]*owner.SecurePrivateCAS{\"first\": {}}", true},
		{"[][]owner.SecurePrivateCAS{{{}}}", true},
		{"openSecurePrivateCAS(root)", true},
		{"owner.openSecurePrivateCAS(root)", true},
	} {
		expression, err := parser.ParseExpr(tc.source)
		if err != nil {
			t.Fatal(err)
		}
		if got := hasPrivateCASConstruction(expression); got != tc.violation {
			t.Errorf("%s: violation=%v, want %v", tc.source, got, tc.violation)
		}
	}
}

func TestEveryRuntimePrivateCASConstructorHasStartupOwnerRecovery(t *testing.T) {
	root := runtimeGoRoot(t)
	const finalAuthorityPath = "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	type functionInfo struct {
		exported bool
		direct   bool
		calls    map[string]bool
	}
	packages := map[string]map[string]*functionInfo{}
	for _, file := range goFiles(t, filepath.Join(root, "internal", "adapters", "outbound")) {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		directory := filepath.Dir(rel(t, root, file))
		packagePath := "analytix.local/runtime-go/" + filepath.ToSlash(directory)
		imports := map[string]string{}
		for _, imported := range parsed.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				continue
			}
			alias := filepath.Base(path)
			if imported.Name != nil {
				alias = imported.Name.Name
			}
			imports[alias] = path
		}
		if packages[packagePath] == nil {
			packages[packagePath] = map[string]*functionInfo{}
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || function.Body == nil {
				continue
			}
			info := &functionInfo{exported: ast.IsExported(function.Name.Name), calls: map[string]bool{}}
			if packagePath == finalAuthorityPath && (function.Name.Name == "OpenSecurePrivateCASWithAccessAuthority" ||
				function.Name.Name == "OpenSecurePrivateCASWithAccessAuthorityContext" || function.Name.Name == "openSecurePrivateCAS") {
				info.direct = true
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch invoked := call.Fun.(type) {
				case *ast.Ident:
					info.calls[invoked.Name] = true
				case *ast.SelectorExpr:
					identifier, ok := invoked.X.(*ast.Ident)
					if ok && imports[identifier.Name] == finalAuthorityPath &&
						(invoked.Sel.Name == "OpenSecurePrivateCASWithAccessAuthority" || invoked.Sel.Name == "OpenSecurePrivateCASWithAccessAuthorityContext") {
						info.direct = true
					}
				}
				return true
			})
			packages[packagePath][function.Name.Name] = info
		}
	}
	usesCAS := map[string]map[string]bool{}
	for packagePath, functions := range packages {
		usesCAS[packagePath] = map[string]bool{}
		for name, info := range functions {
			usesCAS[packagePath][name] = info.direct
		}
		changed := true
		for changed {
			changed = false
			for name, info := range functions {
				if usesCAS[packagePath][name] {
					continue
				}
				for called := range info.calls {
					if usesCAS[packagePath][called] {
						usesCAS[packagePath][name] = true
						changed = true
						break
					}
				}
			}
		}
	}

	activeOwners := map[string]bool{}
	for _, file := range goFiles(t, filepath.Join(root, "internal", "runtimeapp")) {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		imports := map[string]string{}
		for _, imported := range parsed.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				continue
			}
			alias := filepath.Base(path)
			if imported.Name != nil {
				alias = imported.Name.Name
			}
			imports[alias] = path
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			identifier, identifierOK := selector.X.(*ast.Ident)
			if !identifierOK {
				return true
			}
			packagePath := imports[identifier.Name]
			if info := packages[packagePath][selector.Sel.Name]; info != nil && info.exported && usesCAS[packagePath][selector.Sel.Name] {
				activeOwners[packagePath] = true
			}
			return true
		})
	}
	if len(activeOwners) == 0 {
		t.Fatal("runtimeapp has no detected private CAS owner constructors")
	}

	recoveryPath := filepath.Join(root, "internal", "runtimeapp", "private_cas_recovery.go")
	recovery := parseGoFile(t, recoveryPath, 0)
	recoveryImports := map[string]string{}
	for _, imported := range recovery.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			continue
		}
		alias := filepath.Base(path)
		if imported.Name != nil {
			alias = imported.Name.Name
		}
		recoveryImports[alias] = path
	}
	prepare := map[string]bool{}
	ast.Inspect(recovery, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, identifierOK := selector.X.(*ast.Ident)
		if !identifierOK {
			return true
		}
		packagePath := recoveryImports[identifier.Name]
		if strings.HasPrefix(selector.Sel.Name, "Prepare") {
			prepare[packagePath] = true
		}
		return true
	})
	for packagePath := range activeOwners {
		if !prepare[packagePath] {
			t.Fatalf("runtime private CAS owner %s lacks prepared startup recovery", packagePath)
		}
	}
	expectedRootCounts := map[string]int{}
	for _, spec := range domainprivatecas.RuntimeRootSpecsV1() {
		expectedRootCounts[spec.RecoveryGroupID]++
	}
	manifestRootCounts := map[string]int{}
	ast.Inspect(recovery, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		ownerName := ""
		expectedGroup := ""
		hasExpectedRoots := false
		for _, element := range literal.Elts {
			field, ok := element.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := field.Key.(*ast.Ident)
			if !ok {
				continue
			}
			switch key.Name {
			case "name":
				value, ok := field.Value.(*ast.BasicLit)
				if ok {
					ownerName, _ = strconv.Unquote(value.Value)
				}
			case "expectedRoots":
				hasExpectedRoots = true
				call, ok := field.Value.(*ast.CallExpr)
				if !ok || privateCASCallName(call) != "runtimePrivateCASExpectedRoots" || len(call.Args) != 2 {
					t.Fatal("runtime private CAS owner bypasses the topology-derived root resolver")
				}
				value, ok := call.Args[1].(*ast.BasicLit)
				if !ok || value.Kind != token.STRING {
					t.Fatal("runtime private CAS owner recovery group is not a fixed catalog identity")
				}
				expectedGroup, _ = strconv.Unquote(value.Value)
			}
		}
		if ownerName != "" && hasExpectedRoots {
			if ownerName != expectedGroup {
				t.Fatalf("runtime private CAS owner %s resolves roots for %s", ownerName, expectedGroup)
			}
			if _, duplicate := manifestRootCounts[ownerName]; duplicate {
				t.Fatalf("runtime private CAS owner manifest repeats %s", ownerName)
			}
			manifestRootCounts[ownerName] = len(domainprivatecas.RootsForRecoveryGroupV1(ownerName))
		}
		return true
	})
	for owner, count := range expectedRootCounts {
		if manifestRootCounts[owner] != count {
			t.Fatalf("runtime private CAS owner %s exact root count = %d, want %d", owner, manifestRootCounts[owner], count)
		}
	}
	if len(manifestRootCounts) != len(expectedRootCounts) {
		t.Fatalf("runtime private CAS owner manifest has unexpected owners: %#v", manifestRootCounts)
	}
	transactionV4Calls := 0
	transactionV3Calls := 0
	transactionV2Calls := 0
	rootManifestValidationCalls := 0
	topologyManifestValidationCalls := 0
	ast.Inspect(recovery, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch invoked := call.Fun.(type) {
		case *ast.Ident:
			if invoked.Name == "validateRuntimePrivateCASOwnerRootManifest" {
				rootManifestValidationCalls++
			}
			if invoked.Name == "validateRuntimePrivateCASOwnerTopologyManifest" {
				topologyManifestValidationCalls++
			}
		case *ast.SelectorExpr:
			switch invoked.Sel.Name {
			case "ApplyPreparedSecurePrivateCASRecoveryTransactionV4":
				transactionV4Calls++
			case "ApplyPreparedSecurePrivateCASRecoveryTransactionV3":
				transactionV3Calls++
			case "ApplyPreparedSecurePrivateCASRecoveryTransactionV2":
				transactionV2Calls++
			}
		}
		return true
	})
	if rootManifestValidationCalls != 1 || topologyManifestValidationCalls != 1 ||
		transactionV4Calls != 1 || transactionV3Calls != 0 || transactionV2Calls != 0 {
		t.Fatalf(
			"runtime private CAS recovery lacks exact root/topology gates and one V4-only transaction: roots=%d topologies=%d v4=%d v3=%d v2=%d",
			rootManifestValidationCalls, topologyManifestValidationCalls, transactionV4Calls, transactionV3Calls, transactionV2Calls,
		)
	}
}

func TestPrivateCASTopologyCatalogIsTheSingleTypedRootAuthority(t *testing.T) {
	if err := domainprivatecas.ValidateRuntimeDirectorySlotsV1(); err != nil {
		t.Fatal(err)
	}
	specs := domainprivatecas.RuntimeRootSpecsV1()
	if len(specs) != 56 {
		t.Fatalf("runtime private CAS physical root count = %d, want 56", len(specs))
	}
	groups := map[string]int{}
	opaque := 0
	for _, spec := range specs {
		groups[spec.RecoveryGroupID]++
		if spec.SnapshotBodyPolicy == domainprivatecas.SnapshotOpaqueBytesV1 {
			opaque++
		}
	}
	if len(groups) != 19 || opaque != 4 {
		t.Fatalf("runtime private CAS topology = %d groups and %d opaque roots, want 19 and 4", len(groups), opaque)
	}

	root := runtimeGoRoot(t)
	recoveryBody, err := os.ReadFile(filepath.Join(root, "internal", "runtimeapp", "private_cas_recovery.go"))
	if err != nil {
		t.Fatal(err)
	}
	recovery := string(recoveryBody)
	for _, required := range []string{
		"domainprivatecas.RootsForRecoveryGroupV1(",
		"domainprivatecas.ValidateRuntimeDirectorySlotsV1()",
	} {
		if !strings.Contains(recovery, required) {
			t.Fatalf("runtime private CAS recovery does not use topology authority %q", required)
		}
	}
	if strings.Contains(recovery, "expectedRoots: []string{") {
		t.Fatal("runtime private CAS recovery restored a handwritten physical-root manifest")
	}

	snapshotPath := filepath.Join(root, "internal", "adapters", "outbound", "persistencefs", "snapshot.go")
	snapshotBody, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := string(snapshotBody)
	for _, required := range []string{
		"domainprivatecas.RootContainingRelativePathV1(",
		"domainprivatecas.RuntimeRootSpecsV1()",
		"domainprivatecas.ClassifyRecordResidueNameV1(",
	} {
		if !strings.Contains(snapshot, required) {
			t.Fatalf("strict snapshot does not use topology authority %q", required)
		}
	}
	topLevelOwners := map[string]struct{}{}
	for _, spec := range specs {
		topLevelOwners[strings.Split(spec.RelativeCASRoot, "/")[0]] = struct{}{}
	}
	parsedSnapshot := parseGoFile(t, snapshotPath, 0)
	ast.Inspect(parsedSnapshot, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err == nil {
			if _, duplicated := topLevelOwners[value]; duplicated {
				t.Fatalf("strict snapshot duplicates private CAS owner identity %q", value)
			}
		}
		return true
	})

	finalAuthorityPath := filepath.Join(root, "internal", "adapters", "outbound", "finalauthority", "files.go")
	finalAuthorityBody, err := os.ReadFile(finalAuthorityPath)
	if err != nil {
		t.Fatal(err)
	}
	finalAuthority := string(finalAuthorityBody)
	for _, required := range []string{
		"domainprivatecas.PrivateWriteTempNameV1(",
		"domainprivatecas.ClassifyRecordResidueNameV1(",
		"domainprivatecas.RecoveryQuarantineNameV1(",
		"domainprivatecas.CreateDirectoryResidueNameV1(",
		"domainprivatecas.ValidShardV1(",
		"domainprivatecas.ValidDigestV1(",
	} {
		if !strings.Contains(finalAuthority, required) {
			t.Fatalf("final authority does not delegate private CAS grammar %q", required)
		}
	}
	for _, forbidden := range []string{
		`".analytix-cas-recovery-v2-stage-"`,
		`".analytix-cas-recovery-v2-commit-"`,
		`".analytix-cas-create-"`,
	} {
		for _, file := range goFiles(t, filepath.Join(root, "internal", "adapters", "outbound", "finalauthority")) {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			body, readErr := os.ReadFile(file)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if strings.Contains(string(body), forbidden) {
				t.Fatalf("%s duplicates private CAS residue grammar %s", rel(t, root, file), forbidden)
			}
		}
	}
}

func TestPrivateCASRecordDeletionHasOnlySignedV4ProductionAuthority(t *testing.T) {
	root := runtimeGoRoot(t)
	productionRoots := []string{
		filepath.Join(root, "cmd"),
		filepath.Join(root, "internal", "runtimeapp"),
		filepath.Join(root, "internal", "adapters", "outbound"),
	}
	forbiddenSymbols := []string{
		"ApplyPreparedSecurePrivateCASRecoveryTransactionV2(",
		"ApplyPreparedSecurePrivateCASRecoveryTransactionV3(",
		"RecoverSecurePrivateCASIfPresent(",
		"RecoverPrivateStoreIfPresent(",
		"ApplyBeforePrivateCASRecoveryV2(",
	}
	forbiddenRecoveryMethods := map[string][]string{
		"internal/adapters/outbound/finalauthority/private_cas.go": {
			"func (prepared *PreparedSecurePrivateCASRecoveryV1) Apply(",
		},
		"internal/adapters/outbound/finalauthority/owner_recovery_plan.go": {
			"func (prepared *PreparedSecurePrivateCASOwnerRecoveryV1) Apply(",
		},
		"internal/adapters/outbound/finalauthority/private_store_recovery_plan.go": {
			"func (prepared *PreparedPrivateStoreRecoveryV1) Apply(",
		},
	}
	for _, relative := range []string{
		"internal/adapters/outbound/attachmentauthority/store_recovery_plan.go",
		"internal/adapters/outbound/authorityadvancefs/recovery_plan.go",
		"internal/adapters/outbound/backendgenerationfs/recovery.go",
		"internal/adapters/outbound/cachetelemetrystore/store_recovery_plan.go",
		"internal/adapters/outbound/casethreadauthority/store_recovery_plan.go",
		"internal/adapters/outbound/checkpointauthority/store_recovery_plan.go",
		"internal/adapters/outbound/continuationstore/store_recovery_plan.go",
		"internal/adapters/outbound/pendingworkstore/store_recovery_plan.go",
		"internal/adapters/outbound/piiauthorization/store_recovery_plan.go",
		"internal/adapters/outbound/reportpublication/store_recovery_plan.go",
		"internal/adapters/outbound/turnterminalstore/store_recovery_plan.go",
	} {
		forbiddenRecoveryMethods[relative] = append(
			forbiddenRecoveryMethods[relative], "func (prepared *PreparedRecoveryV1) Apply(",
		)
	}
	forbiddenRecoveryMethods["internal/adapters/outbound/piiauthorization/access_store_recovery_plan.go"] = []string{
		"func (prepared *PreparedAccessRecoveryV1) Apply(",
	}
	forbiddenRecoveryMethods["internal/adapters/outbound/piiauthorization/access_store_recovery_plan_v2.go"] = []string{
		"func (prepared *PreparedAccessRecoveryV2) Apply(",
	}

	v4Callsites := map[string]int{}
	commitCallsites := map[string]int{}
	rollbackCallsites := map[string]int{}
	revokeCallsites := map[string]int{}
	for _, directory := range productionRoots {
		for _, file := range goFiles(t, directory) {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			relative := rel(t, root, file)
			body, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			source := string(body)
			for _, forbidden := range forbiddenSymbols {
				if strings.Contains(source, forbidden) {
					t.Fatalf("%s restores unsigned private CAS deletion authority %q", relative, forbidden)
				}
			}
			for _, forbidden := range forbiddenRecoveryMethods[relative] {
				if strings.Contains(source, forbidden) {
					t.Fatalf("%s restores prepared recovery deletion method %q", relative, forbidden)
				}
			}
			parsed := parseGoFile(t, file, 0)
			ast.Inspect(parsed, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := privateCASCallName(call)
				switch name {
				case "ApplyPreparedSecurePrivateCASRecoveryTransactionV4":
					v4Callsites[relative]++
				case "commitPreparedPrivateCASRecoveryAuthoritySetWithWitnessV4":
					commitCallsites[relative]++
				case "rollbackPreparedPrivateCASRecoveryTransactionV4":
					rollbackCallsites[relative]++
				case "revokePrivateCASRootGeneration":
					revokeCallsites[relative]++
				}
				return true
			})
		}
	}
	requireExactPrivateCASCallsites(t, "signed V4 coordinator", v4Callsites, map[string]int{
		"internal/adapters/outbound/backendgenerationfs/recovery.go": 1,
		"internal/runtimeapp/private_cas_recovery.go":                1,
	})
	requireExactPrivateCASCallsites(t, "witness-bound commit", commitCallsites, map[string]int{
		"internal/adapters/outbound/finalauthority/private_cas_recovery_v4.go": 1,
	})
	requireExactPrivateCASCallsites(t, "pre-witness rollback", rollbackCallsites, map[string]int{
		"internal/adapters/outbound/finalauthority/private_cas_recovery_v4.go": 1,
	})
	requireExactPrivateCASCallsites(t, "generation revocation", revokeCallsites, map[string]int{
		"internal/adapters/outbound/finalauthority/private_cas.go":             1,
		"internal/adapters/outbound/finalauthority/private_cas_recovery_v4.go": 1,
	})

	backendBody, err := os.ReadFile(filepath.Join(
		root, "internal", "adapters", "outbound", "backendgenerationfs", "recovery.go",
	))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"func (prepared *PreparedRecoveryV1) ApplyV4(",
		"journal privatecasport.RecoveryJournalV1",
		"if journal == nil",
	} {
		if !strings.Contains(string(backendBody), required) {
			t.Fatalf("backend generation signed recovery is missing %q", required)
		}
	}
}

func requireExactPrivateCASCallsites(t *testing.T, name string, got map[string]int, want map[string]int) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("private CAS %s callsites = %#v, want %#v", name, got, want)
	}
}

func TestPrivateCASDirectoryRecoveryUsesOneTopologyAndNoCreateTraversal(t *testing.T) {
	if err := domainprivatecas.ValidateRuntimeDirectorySlotsV1(); err != nil {
		t.Fatal(err)
	}
	if len(domainprivatecas.FixedDirectorySlotsV1()) != 76 ||
		len(domainprivatecas.CreateRecoveryScanParentPathsV1()) != 77 ||
		len(domainprivatecas.RecoverableOwnerDirectoryGroupsV1()) != 15 ||
		domainprivatecas.MaximumCreateResidueCandidateLocationsV1() != 14_412 {
		t.Fatal("private CAS directory recovery topology changed")
	}
	root := runtimeGoRoot(t)
	commonPath := filepath.Join(
		root,
		"internal",
		"adapters",
		"outbound",
		"finalauthority",
		"private_cas_create_recovery.go",
	)
	commonBody, err := os.ReadFile(commonPath)
	if err != nil {
		t.Fatal(err)
	}
	common := string(commonBody)
	for _, required := range []string{
		"domainprivatecas.ValidateRuntimeDirectorySlotsV1()",
		"domainprivatecas.CreateRecoveryScanParentPathsV1()",
		"domainprivatecas.FixedDirectorySlotsV1()",
		"domainprivatecas.ShardParentPathsV1()",
		"withExistingPrivateCASAccess(",
		"PrepareSecurePrivateCASCreateResidueRecoveryV1(",
		"PrepareSecurePrivateCASOrphanTopologyRecoveryV1(",
		"privateCASRecoveryExclusion.acquireRecovery(ctx)",
	} {
		if !strings.Contains(common, required) {
			t.Fatalf("private CAS directory recovery is missing %q", required)
		}
	}
	for _, file := range []string{
		commonPath,
		filepath.Join(filepath.Dir(commonPath), "private_cas_create_recovery_unix.go"),
		filepath.Join(filepath.Dir(commonPath), "private_cas_create_recovery_windows.go"),
	} {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"withPrivateCASAccess(",
			"os.RemoveAll(",
			"os.Mkdir(",
			"os.MkdirAll(",
			"filepath.Walk(",
			"filepath.WalkDir(",
		} {
			if strings.Contains(string(body), forbidden) {
				t.Fatalf("%s grants directory recovery forbidden operation %q", rel(t, root, file), forbidden)
			}
		}
		if file != commonPath &&
			!strings.Contains(string(body), "domainprivatecas.RecoverableOwnerDirectoryGroupsV1()") {
			t.Fatalf("%s does not derive partial-owner rollback from the topology authority", rel(t, root, file))
		}
	}
	windowsBody, err := os.ReadFile(filepath.Join(filepath.Dir(commonPath), "private_cas_create_recovery_windows.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(windowsBody), "windows.FILE_CREATE") {
		t.Fatal("Windows private CAS directory recovery can create a missing path")
	}
	windowsRecovery := string(windowsBody)
	for _, required := range []string{
		"privateCASWindowsOpenDeletionGuard(",
		"windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE",
		"privateWindowsReopenFile(",
	} {
		if !strings.Contains(windowsRecovery, required) {
			t.Fatalf("Windows private CAS directory recovery is missing %q", required)
		}
	}
	if strings.Contains(windowsRecovery, "windows.DuplicateHandle") {
		t.Fatal("Windows private CAS directory recovery shares a consumed directory cursor")
	}
	requiredIndependentCursors := map[string]bool{
		"privateCASWindowsWalkDir":        false,
		"privateCASWindowsReadDirBounded": false,
	}
	parsedWindowsCAS := parseGoFile(t, filepath.Join(filepath.Dir(commonPath), "private_cas_windows.go"), 0)
	for _, declaration := range parsedWindowsCAS.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		if _, required := requiredIndependentCursors[function.Name.Name]; !required {
			continue
		}
		requiredIndependentCursors[function.Name.Name] = true
		hasReopen := false
		ast.Inspect(function.Body, func(node ast.Node) bool {
			switch current := node.(type) {
			case *ast.CallExpr:
				if privateCASCallName(current) == "privateWindowsReopenFile" {
					hasReopen = true
				}
			case *ast.SelectorExpr:
				if current.Sel.Name == "DuplicateHandle" {
					t.Fatalf(
						"Windows private CAS function %s shares a consumed directory cursor",
						function.Name.Name,
					)
				}
			}
			return true
		})
		if !hasReopen {
			t.Fatalf(
				"Windows private CAS function %s does not reopen an independent directory cursor",
				function.Name.Name,
			)
		}
	}
	for function, found := range requiredIndependentCursors {
		if !found {
			t.Fatalf("Windows private CAS is missing %s", function)
		}
	}
	privateSecureWindows := filepath.Join(filepath.Dir(commonPath), "private_secure_windows.go")
	parsedPrivateSecureWindows := parseGoFile(t, privateSecureWindows, 0)
	foundIndependentReadDir := false
	for _, declaration := range parsedPrivateSecureWindows.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil || function.Name.Name != "privateWindowsReadDir" {
			continue
		}
		foundIndependentReadDir = true
		hasReopen := false
		ast.Inspect(function.Body, func(node ast.Node) bool {
			switch current := node.(type) {
			case *ast.CallExpr:
				if privateCASCallName(current) == "privateWindowsReopenFile" {
					hasReopen = true
				}
			case *ast.SelectorExpr:
				if current.Sel.Name == "DuplicateHandle" {
					t.Fatal("Windows private authority shares a consumed directory cursor")
				}
			}
			return true
		})
		if !hasReopen {
			t.Fatal("Windows private authority does not reopen an independent directory cursor")
		}
	}
	if !foundIndependentReadDir {
		t.Fatal("Windows private authority is missing privateWindowsReadDir")
	}
}

func TestPrivateCASTopologyDomainCannotDependOnAdaptersOrNativeHandles(t *testing.T) {
	root := runtimeGoRoot(t)
	directory := filepath.Join(root, "internal", "domain", "privatecastopology")
	for _, file := range goFiles(t, directory) {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed := parseGoFile(t, file, parser.ImportsOnly)
		for _, imported := range parsed.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				continue
			}
			if strings.Contains(path, "/internal/adapters/") || strings.Contains(path, "/internal/runtimeapp") ||
				path == "os" || path == "syscall" || path == "unsafe" || strings.HasPrefix(path, "golang.org/x/sys/") {
				t.Fatalf("%s imports forbidden concrete authority %q", rel(t, root, file), path)
			}
		}
	}
}

func TestProductionPrivateCASRecoveryCannotMaterializeCommittedBodies(t *testing.T) {
	root := runtimeGoRoot(t)
	finalAuthorityRoot := filepath.Join(root, "internal", "adapters", "outbound", "finalauthority")
	for _, file := range goFiles(t, root) {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		relative := rel(t, root, file)
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), ".CommittedFiles(") {
			t.Fatalf("%s restores eager prepared-recovery body materialization", relative)
		}
	}
	privateCASBody, err := os.ReadFile(filepath.Join(finalAuthorityRoot, "private_cas.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"committed   []SecurePrivateCASFile",
		"committed []SecurePrivateCASFile",
		"func (prepared *PreparedSecurePrivateCASRecoveryV1) CommittedFiles(",
	} {
		if strings.Contains(string(privateCASBody), forbidden) {
			t.Fatalf("prepared private CAS observation retains body field %q", forbidden)
		}
	}
	ownerBody, err := os.ReadFile(filepath.Join(finalAuthorityRoot, "owner_recovery_plan.go"))
	if err != nil {
		t.Fatal(err)
	}
	requiredBySource := map[string]struct {
		source   string
		required []string
	}{
		"private CAS": {
			source: string(privateCASBody),
			required: []string{
				"func (prepared *PreparedSecurePrivateCASRecoveryV1) VisitCommittedFiles(",
				"func (prepared *PreparedSecurePrivateCASRecoveryV1) VisitCommittedMaterials(",
				"if err := prepared.Revalidate(ctx); err != nil {",
				"body, err := prepared.readPreparedCommittedFile(ctx, material.Digest)",
				"return prepared.Revalidate(ctx)",
			},
		},
		"private CAS owner": {
			source: string(ownerBody),
			required: []string{
				"func (prepared *PreparedSecurePrivateCASOwnerRecoveryV1) VisitCommittedFiles(",
				"return leaf.plan.VisitCommittedFiles(ctx, visit)",
				"func (prepared *PreparedSecurePrivateCASOwnerRecoveryV1) VisitCommittedMaterials(",
				"return leaf.plan.VisitCommittedMaterials(ctx, visit)",
			},
		},
	}
	for owner, assertion := range requiredBySource {
		for _, required := range assertion.required {
			if !strings.Contains(assertion.source, required) {
				t.Fatalf("%s streaming prepared recovery API is missing %q", owner, required)
			}
		}
	}
	if strings.Contains(string(ownerBody),
		"func (prepared *PreparedSecurePrivateCASOwnerRecoveryV1) CommittedFiles(",
	) {
		t.Fatal("private CAS owner restores eager committed-file materialization API")
	}
	reportRecovery := parseGoFile(t, filepath.Join(
		root, "internal", "adapters", "outbound", "reportpublication", "store_recovery_plan.go",
	), 0)
	validator := privateCASFunctionDeclaration(t, reportRecovery, "validateDomainSemantics")
	if !reportArtifactRecoveryUsesMetadata(validator) {
		t.Fatal("report artifact recovery must consume only its bound, body-free material visitor")
	}
	if total, metadata := reportArtifactMaterialVisits(reportRecovery); total != 1 || metadata != 1 {
		t.Fatal("report recovery introduces an extra artifact reader outside its bound validator")
	}
}

func TestPrivateCASDirectoryAndOwnerRecoveryOrderIsStrict(t *testing.T) {
	root := runtimeGoRoot(t)
	parsed := parseGoFile(t, filepath.Join(root, "internal", "runtimeapp", "app.go"), 0)
	var target *ast.FuncDecl
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "newRuntimeServerHandlerWithRootsModeE" {
			target = function
			break
		}
	}
	if target == nil {
		t.Fatal("runtime startup composition function is missing")
	}
	calls := make([]string, 0)
	ast.Inspect(target.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch function := call.Fun.(type) {
		case *ast.Ident:
			calls = append(calls, function.Name)
		case *ast.SelectorExpr:
			if receiver, ok := function.X.(*ast.Ident); ok {
				calls = append(calls, receiver.Name+"."+function.Sel.Name)
			}
		}
		return true
	})
	callIndex := func(name string) int {
		for index, call := range calls {
			if call == name {
				return index
			}
		}
		return -1
	}
	createResidues := callIndex("recoverRuntimePrivateCASCreateResidues")
	journal := callIndex("semanticBuilder.RecoverAuthenticatedExisting")
	orphanTopology := callIndex("recoverRuntimePrivateCASOrphanTopology")
	owners := callIndex("recoverRuntimePrivateCASOwnersWithPreservationV1")
	baseline := callIndex("startupapp.BeginReadOnlyStartupPlanV1")
	if createResidues < 0 || journal < 0 || orphanTopology < 0 || owners < 0 || baseline < 0 ||
		!(createResidues < journal && journal < orphanTopology && orphanTopology < owners && owners < baseline) {
		t.Fatalf(
			"startup recovery call order is unsafe: create=%d journal=%d orphan=%d owners=%d baseline=%d calls=%v",
			createResidues,
			journal,
			orphanTopology,
			owners,
			baseline,
			calls,
		)
	}
}

func privateCASRecoveryPreservationBindingValid(call *ast.CallExpr) bool {
	if len(call.Args) != 6 || !privateCASSelectorMatches(call.Args[1], "config", "DataDir") {
		return false
	}
	for index, name := range []string{"ctx", "", "privateCASAccessAuthority", "optionalInstallation", "reportRestartPreservation", "privateCASRecoveryJournal"} {
		if name != "" && privateCASIdent(call.Args[index]) != name {
			return false
		}
	}
	return true
}

func TestPrivateCASRecoveryGuardRejectsSubstitutedAuthority(t *testing.T) {
	valid := "recoverRuntimePrivateCASOwnersWithPreservationV1(ctx, config.DataDir, privateCASAccessAuthority, optionalInstallation, reportRestartPreservation, privateCASRecoveryJournal)"
	candidates := []string{valid, "recoverRuntimePrivateCASOwnersWithPreservationV1()"}
	for _, authority := range []string{"ctx", "config.DataDir", "privateCASAccessAuthority", "optionalInstallation", "reportRestartPreservation", "privateCASRecoveryJournal"} {
		candidates = append(candidates, strings.Replace(valid, authority, "nil", 1))
	}
	for index, candidate := range candidates {
		expression, err := parser.ParseExpr(candidate)
		if err != nil {
			t.Fatal(err)
		}
		if privateCASRecoveryPreservationBindingValid(expression.(*ast.CallExpr)) != (index == 0) {
			t.Fatalf("recovery guard misclassified mutation %d", index)
		}
	}
}

func TestLiveCheckpointRecoveryCompletesBeforeSemanticBaselineAndUsesOnlyAuthorizedDelta(t *testing.T) {
	root := runtimeGoRoot(t)
	app := parseGoFile(t, filepath.Join(root, "internal", "runtimeapp", "app.go"), 0)
	composition := privateCASFunctionDeclaration(t, app, "newRuntimeServerHandlerWithRootsModeE")
	owners := privateCASExactlyOneCall(t, composition, "", "recoverRuntimePrivateCASOwnersWithPreservationV1")
	if !privateCASRecoveryPreservationBindingValid(owners) {
		t.Fatal("owner recovery must retain the exact access, installation, preservation and signed journal authorities")
	}
	recoveryCall := privateCASExactlyOneCall(
		t, composition, "", "recoverAuthenticatedCheckpointOperationsBeforeSemanticBaselineV1",
	)
	semanticBaseline := privateCASExactlyOneCall(t, composition, "startupapp", "BeginReadOnlyStartupPlanV1")
	bind := privateCASExactlyOneCall(t, composition, "startupSession", "BindConfiguration")
	buildPlan := privateCASExactlyOneCall(t, composition, "startupapp", "BuildSemanticStartupPlanV1")
	retirement := privateCASExactlyOneCall(
		t, composition, "", "withRetiredRuntimePrivateCASOwnerGenerationsForSemanticApply",
	)
	postApplyVerify := privateCASExactlyOneCall(t, composition, "checkpointRecovery", "Verify")
	if !(owners.Pos() < recoveryCall.Pos() &&
		recoveryCall.Pos() < semanticBaseline.Pos() &&
		semanticBaseline.Pos() < bind.Pos() &&
		bind.Pos() < buildPlan.Pos() &&
		buildPlan.Pos() < retirement.Pos() &&
		retirement.Pos() < postApplyVerify.Pos()) {
		t.Fatal("runtime composition does not recover, seal, retire/apply, then post-verify in order")
	}
	if len(retirement.Args) != 7 ||
		privateCASIdent(retirement.Args[0]) != "ctx" ||
		!privateCASSelectorMatches(retirement.Args[1], "config", "DataDir") ||
		privateCASIdent(retirement.Args[2]) != "privateCASAccessAuthority" ||
		privateCASIdent(retirement.Args[3]) != "optionalInstallation" ||
		!privateCASCallExpressionMatches(retirement.Args[4], "prepared", "Plan", 0) ||
		!privateCASSelectorMatches(retirement.Args[5], "prepared", "Apply") ||
		privateCASIdent(retirement.Args[6]) != "reportRestartPreservation" {
		t.Fatal("runtime composition does not bind the exact prepared plan/apply pair to the retirement wrapper")
	}
	if direct := privateCASMatchingCalls(composition, "prepared", "Apply"); len(direct) != 0 {
		t.Fatal("runtime composition bypasses the retirement wrapper with a direct prepared.Apply call")
	}
	if len(postApplyVerify.Args) != 2 ||
		privateCASIdent(postApplyVerify.Args[0]) != "ctx" ||
		privateCASIdent(postApplyVerify.Args[1]) != "checkpointRecoveryDelta" {
		t.Fatal("runtime composition post-apply checkpoint verification is not bound to the recovered delta")
	}

	recovery := parseGoFile(
		t, filepath.Join(root, "internal", "runtimeapp", "checkpoint_operation_recovery.go"), 0,
	)
	recoverFunction := privateCASFunctionDeclaration(
		t, recovery, "recoverAuthenticatedCheckpointOperationsBeforeSemanticBaselineV1",
	)
	captures := privateCASMatchingCalls(recoverFunction, "reader", "CaptureManagedSnapshotV1")
	validations := privateCASMatchingCalls(recoverFunction, "domainstartup", "ValidateManagedSnapshotV1")
	verifications := privateCASMatchingCalls(recoverFunction, "prepared", "Verify")
	if len(captures) != 2 || len(validations) != 2 || len(verifications) != 2 {
		t.Fatalf(
			"checkpoint recovery proof is incomplete: captures=%d validations=%d verifications=%d",
			len(captures), len(validations), len(verifications),
		)
	}
	present := privateCASExactlyOneCall(t, recoverFunction, "", "checkpointAuthorityPresentInManagedSnapshot")
	prepare := privateCASExactlyOneCall(t, recoverFunction, "", "prepareLiveCheckpointOperationRecovery")
	recover := privateCASExactlyOneCall(t, recoverFunction, "prepared", "Recover")
	condition := privateCASExactlyOneCall(t, recoverFunction, "delta", "HasChanges")
	authorized := privateCASExactlyOneCall(t, recoverFunction, "startupapp", "ValidateAuthorizedManagedDeltaV1")
	if !(captures[0].Pos() < validations[0].Pos() &&
		validations[0].Pos() < present.Pos() &&
		present.Pos() < prepare.Pos() &&
		prepare.Pos() < recover.Pos() &&
		recover.Pos() < verifications[0].Pos() &&
		verifications[0].Pos() < captures[1].Pos() &&
		captures[1].Pos() < validations[1].Pos() &&
		validations[1].Pos() < condition.Pos() &&
		condition.Pos() < authorized.Pos() &&
		authorized.Pos() < verifications[1].Pos()) {
		t.Fatal("checkpoint recovery does not prove its exact before/after authorized delta order")
	}
	privateCASRequireUnchangedSnapshotElse(t, recoverFunction)

	retirementSource := parseGoFile(
		t, filepath.Join(root, "internal", "runtimeapp", "private_cas_recovery.go"), 0,
	)
	retirementWrapper := privateCASFunctionDeclaration(
		t, retirementSource, "withRetiredRuntimePrivateCASOwnerGenerationsForSemanticApply",
	)
	prepareOwners := privateCASExactlyOneCall(
		t, retirementWrapper, "", "prepareAndValidateRuntimePrivateCASOwners",
	)
	participants := privateCASExactlyOneCall(
		t, retirementWrapper, "", "runtimePrivateCASRecoveryParticipantsV4",
	)
	affectedRoots := privateCASExactlyOneCall(
		t, retirementWrapper, "", "runtimePrivateCASSemanticApplyRootIDs",
	)
	delegate := privateCASExactlyOneCall(
		t, retirementWrapper, "finalauthority",
		"WithRetiredPreparedSecurePrivateCASGenerationsForSemanticApplyV1",
	)
	if !(prepareOwners.Pos() < participants.Pos() &&
		participants.Pos() < affectedRoots.Pos() &&
		affectedRoots.Pos() < delegate.Pos()) {
		t.Fatal("runtime retirement wrapper does not close owner preparation before semantic apply")
	}
	if len(delegate.Args) != 4 ||
		privateCASIdent(delegate.Args[0]) != "ctx" ||
		privateCASIdent(delegate.Args[1]) != "participants" ||
		privateCASIdent(delegate.Args[2]) != "affectedRootIDs" ||
		privateCASIdent(delegate.Args[3]) != "apply" {
		t.Fatal("runtime retirement wrapper does not delegate the exact prepared apply callback")
	}
	if direct := privateCASMatchingCalls(retirementWrapper, "", "apply"); len(direct) != 0 {
		t.Fatal("runtime retirement wrapper invokes apply outside the finalauthority exclusion")
	}
	rootResolver := privateCASFunctionDeclaration(t, retirementSource, "runtimePrivateCASSemanticApplyRootIDs")
	validatePlan := privateCASExactlyOneCall(t, rootResolver, "domainstartup", "ValidateSemanticStartupPlanV1")
	if len(validatePlan.Args) != 1 || privateCASIdent(validatePlan.Args[0]) != "plan" {
		t.Fatal("runtime retirement wrapper does not validate the exact semantic plan")
	}

	finalAuthority := parseGoFile(
		t, filepath.Join(
			root, "internal", "adapters", "outbound", "finalauthority", "private_cas_recovery_v4.go",
		), 0,
	)
	exclusion := privateCASFunctionDeclaration(
		t, finalAuthority, "WithRetiredPreparedSecurePrivateCASGenerationsForSemanticApplyV1",
	)
	acquire := privateCASExactlyOneCall(t, exclusion, "privateCASRecoveryExclusion", "acquireRecovery")
	prepareSet := privateCASExactlyOneCall(t, exclusion, "", "prepareSecurePrivateCASRecoveryAuthoritySetV4")
	targets := privateCASExactlyOneCall(t, exclusion, "prepared", "currentTargets")
	revoke := privateCASExactlyOneCall(t, exclusion, "", "revokePreparedPrivateCASRecoveryGenerationSubsetV4")
	apply := privateCASExactlyOneCall(t, exclusion, "privateCASRecoveryExclusion", "withSemanticObservationV1")
	if !(acquire.Pos() < prepareSet.Pos() &&
		prepareSet.Pos() < targets.Pos() &&
		targets.Pos() < revoke.Pos() &&
		revoke.Pos() < apply.Pos()) {
		t.Fatal("finalauthority retirement exclusion does not retire prepared generations before apply")
	}
	if len(apply.Args) != 2 || privateCASIdent(apply.Args[0]) != "ctx" || privateCASIdent(apply.Args[1]) != "apply" ||
		!privateCASReturnsCall(exclusion, apply) ||
		!privateCASDefersCall(exclusion, "privateCASRecoveryExclusion", "releaseRecovery") {
		t.Fatal("finalauthority retirement exclusion does not return apply under its held recovery lease")
	}
	if len(privateCASMatchingCalls(exclusion, "", "apply")) != 0 {
		t.Fatal("retirement apply bypasses its bounded semantic observation context")
	}
	observationSource := parseGoFile(t, filepath.Join(root, "internal", "adapters", "outbound", "finalauthority", "private_cas_semantic_observation.go"), 0)
	observation := privateCASFunctionDeclaration(t, observationSource, "withSemanticObservationV1")
	callback := privateCASExactlyOneCall(t, observation, "", "apply")
	if len(callback.Args) != 1 || !privateCASCallExpressionMatches(callback.Args[0], "context", "WithValue", 3) ||
		!privateCASReturnsCall(observation, callback) {
		t.Fatal("semantic observation must invoke the exact callback with its scoped context and return its error")
	}
	boundContext := callback.Args[0].(*ast.CallExpr)
	if privateCASIdent(boundContext.Args[0]) != "observationContext" || privateCASIdent(boundContext.Args[2]) != "scope" {
		t.Fatal("semantic callback lost its cancellable context or exact scope")
	}
}

func TestPrivateFinalStoreHasNoAuthorityFreeProductionConstructor(t *testing.T) {
	root := runtimeGoRoot(t)
	constructorCalls := 0
	for _, file := range goFiles(t, root) {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := privateCASCallName(call)
			if name != "NewPrivateStore" && name != "NewPrivateStoreContext" {
				return true
			}
			constructorCalls++
			expected := 2
			authorityIndex := 1
			if name == "NewPrivateStoreContext" {
				expected = 3
				authorityIndex = 2
			}
			if len(call.Args) != expected {
				t.Fatalf("%s constructs private final store without access authority", rel(t, root, file))
			}
			if identifier, ok := call.Args[authorityIndex].(*ast.Ident); ok && identifier.Name == "nil" {
				t.Fatalf("%s passes nil private final access authority", rel(t, root, file))
			}
			return true
		})
	}
	if constructorCalls != 3 {
		t.Fatalf("production private final store constructor calls = %d, want context wrapper, runtimeapp, and testsupport only", constructorCalls)
	}
	path := filepath.Join(root, "internal", "adapters", "outbound", "finalauthority", "private_store.go")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"privateRootAuthority", "secureListPrivateFiles(", "secureReadPrivateFile("} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("private final store retains legacy authority bypass %q", forbidden)
		}
	}
	for _, required := range []string{"*SecurePrivateCAS", "OpenSecurePrivateCASWithAccessAuthorityContext", ".Visit("} {
		if !strings.Contains(string(body), required) {
			t.Fatalf("private final store secure CAS wiring is missing %q", required)
		}
	}
}

func TestFinalAuthorityPreflightCannotRestoreEagerPrivateInventoryReads(t *testing.T) {
	root := runtimeGoRoot(t)
	path := filepath.Join(root, "internal", "app", "evidence", "final_authority_preflight.go")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"privateStore.List(", "privateStore.ListDispositions("} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("final authority preflight restored eager inventory call %q", forbidden)
		}
	}
	for _, required := range []string{"privateStore.VisitAcceptedFinals(", "privateStore.VisitDispositions("} {
		if strings.Count(string(body), required) != 1 {
			t.Fatalf("final authority preflight streaming call count for %q is not exactly one", required)
		}
	}
}

func TestWindowsAuthorityTraversalCannotRequestAncestorWriteAccess(t *testing.T) {
	root := runtimeGoRoot(t)
	for _, relative := range []string{
		"internal/adapters/outbound/finalauthority/private_secure_windows.go",
		"internal/adapters/outbound/persistencefs/secure_managed_windows.go",
	} {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		if !strings.Contains(text, "TraverseDirectoryAccess") || !strings.Contains(text, "ReopenSameDirectoryForMutation") {
			t.Fatalf("%s is missing split Windows traversal/mutation authority", relative)
		}
		for _, forbidden := range []string{
			"volumeRoot, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE",
			"volume+string(os.PathSeparator), windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE",
		} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s requests write access while opening a Windows volume ancestor", relative)
			}
		}
	}
}

func TestCurrentWindowsAuthorityCannotUseTruncatedFileIndex(t *testing.T) {
	root := runtimeGoRoot(t)
	for _, directory := range []string{
		"internal/adapters/outbound/finalauthority",
		"internal/adapters/outbound/persistencefs",
	} {
		for _, file := range goFiles(t, filepath.Join(root, filepath.FromSlash(directory))) {
			if strings.HasSuffix(file, "_test.go") || strings.HasSuffix(file, "legacy_v3_migration_windows.go") {
				continue
			}
			body, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(body), "FileIndexHigh") || strings.Contains(string(body), "FileIndexLow") {
				t.Fatalf("%s uses truncated Windows FileIndex outside the explicit V3 migration boundary", rel(t, root, file))
			}
		}
	}
}

func TestWindowsPrivateCASFinalDirectoriesCannotBeCreatedInPlace(t *testing.T) {
	root := runtimeGoRoot(t)
	targets := map[string]map[string]bool{
		"internal/adapters/outbound/finalauthority/private_cas_windows.go": {
			"privateCASWindowsOpenBoundRoot":            false,
			"privateCASWindowsOpenPinnedShard":          false,
			"privateCASWindowsOpenPinnedShardForCommit": false,
		},
		"internal/adapters/outbound/finalauthority/private_secure_windows.go": {
			"privateWindowsOpenShard": false,
		},
	}
	for relative, functions := range targets {
		file := filepath.Join(root, filepath.FromSlash(relative))
		parsed := parseGoFile(t, file, 0)
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			if _, targeted := functions[function.Name.Name]; !targeted {
				continue
			}
			functions[function.Name.Name] = true
			callsStagedCreate := false
			ast.Inspect(function.Body, func(node ast.Node) bool {
				switch current := node.(type) {
				case *ast.SelectorExpr:
					if current.Sel.Name == "FILE_CREATE" {
						t.Fatalf("%s %s directly creates a canonical final directory", relative, function.Name.Name)
					}
				case *ast.CallExpr:
					if privateCASCallName(current) == "privateCASWindowsCreateBoundDirectory" {
						callsStagedCreate = true
					}
				}
				return true
			})
			if !callsStagedCreate {
				t.Fatalf("%s %s does not delegate directory installation to the staged no-replace primitive", relative, function.Name.Name)
			}
		}
		for function, found := range functions {
			if !found {
				t.Fatalf("%s is missing Windows private CAS function %s", relative, function)
			}
		}
	}
}

func TestRuntimeCompositionCannotBypassPrivateCASAccessAuthority(t *testing.T) {
	root := runtimeGoRoot(t)
	appPath := filepath.Join(root, "internal", "runtimeapp", "app.go")
	appBody, err := os.ReadFile(appPath)
	if err != nil {
		t.Fatal(err)
	}
	app := string(appBody)
	for _, forbidden := range []string{
		"func newRuntimeServerHandlerWithRootsE(",
		"func newRuntimeServerHandlerWithRoots(",
	} {
		if strings.Contains(app, forbidden) {
			t.Fatalf("runtimeapp restored roots-only composition path %q", forbidden)
		}
	}
	for _, required := range []string{
		"privateCASAccessAuthority finalauthority.SecurePrivateCASRecoveryAccessAuthority",
		"privateCASAccessAuthority == nil",
		"rootAuthority, journalAuthority, privateCASAccessAuthority",
	} {
		if !strings.Contains(app, required) {
			t.Fatalf("runtimeapp private CAS authority wiring is missing %q", required)
		}
	}
	for _, file := range goFiles(t, filepath.Join(root, "internal", "server")) {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && (function.Name.Name == "NewRuntimeServerHandler" || function.Name.Name == "newCompatibilityRuntimeServerHandler") {
				t.Fatalf("%s restores a second production runtime composition factory", rel(t, root, file))
			}
		}
	}
}

func TestSemanticStageUsesOnlyItsBoundPrivateCASAuthority(t *testing.T) {
	root := runtimeGoRoot(t)
	appPath := filepath.Join(root, "internal", "runtimeapp", "app.go")
	app := parseGoFile(t, appPath, 0)
	constructorCalls := 0
	for _, file := range goFiles(t, root) {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || privateCASCallName(call) != "NewSemanticStagePrivateCASAccessAuthority" {
				return true
			}
			constructorCalls++
			if rel(t, root, file) != "internal/runtimeapp/app.go" {
				t.Fatalf("%s creates semantic-stage private CAS authority outside runtimeapp", rel(t, root, file))
			}
			return true
		})
	}
	if constructorCalls != 1 {
		t.Fatalf("semantic-stage private CAS authority constructor calls = %d, want 1", constructorCalls)
	}
	var callback *ast.FuncLit
	ast.Inspect(app, func(node ast.Node) bool {
		literal, ok := node.(*ast.FuncLit)
		if !ok {
			return true
		}
		found := false
		ast.Inspect(literal.Body, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if ok && privateCASCallName(call) == "NewSemanticStagePrivateCASAccessAuthority" {
				found = true
			}
			return true
		})
		if found {
			if callback != nil {
				t.Fatal("semantic-stage private CAS constructor is nested in ambiguous callbacks")
			}
			callback = literal
			return false
		}
		return true
	})
	if callback == nil {
		t.Fatal("runtimeapp semantic-stage private CAS callback is missing")
	}
	constructorExact := false
	handlerExact := false
	namedStageError := false
	deferredClose := false
	joinedCloseError := false
	usedOuterAuthority := false
	if callback.Type.Results != nil && len(callback.Type.Results.List) == 1 {
		result := callback.Type.Results.List[0]
		namedStageError = len(result.Names) == 1 && result.Names[0].Name == "stageErr"
	}
	ast.Inspect(callback.Body, func(node ast.Node) bool {
		switch current := node.(type) {
		case *ast.Ident:
			if current.Name == "privateCASAccessAuthority" {
				usedOuterAuthority = true
			}
		case *ast.CallExpr:
			switch privateCASCallName(current) {
			case "NewSemanticStagePrivateCASAccessAuthority":
				constructorExact = len(current.Args) == 2 && privateCASIdent(current.Args[0]) == "stageRoots" &&
					privateCASIdent(current.Args[1]) == "stageAuthority"
			case "newRuntimeServerHandlerWithRootsModeE":
				if len(current.Args) >= 6 && privateCASIdent(current.Args[2]) == "stageRoots" &&
					privateCASIdent(current.Args[3]) == "stageAuthority" && privateCASIdent(current.Args[5]) == "stageAccessAuthority" {
					handlerExact = true
				}
			}
		case *ast.DeferStmt:
			ast.Inspect(current.Call, func(deferred ast.Node) bool {
				call, ok := deferred.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if ok && selector.Sel.Name == "Close" && privateCASIdent(selector.X) == "stageAccessAuthority" {
					deferredClose = true
				}
				if ok && selector.Sel.Name == "Join" && privateCASIdent(selector.X) == "errors" && len(call.Args) == 2 &&
					privateCASIdent(call.Args[0]) == "stageErr" {
					ast.Inspect(call.Args[1], func(argument ast.Node) bool {
						closeCall, ok := argument.(*ast.CallExpr)
						if !ok {
							return true
						}
						closeSelector, ok := closeCall.Fun.(*ast.SelectorExpr)
						if ok && closeSelector.Sel.Name == "Close" && privateCASIdent(closeSelector.X) == "stageAccessAuthority" {
							joinedCloseError = true
						}
						return true
					})
				}
				return true
			})
		}
		return true
	})
	if !constructorExact || !handlerExact || !namedStageError || !deferredClose || !joinedCloseError || usedOuterAuthority {
		t.Fatalf(
			"semantic-stage authority wiring invalid: constructor=%v handler=%v named=%v close=%v joined=%v outer=%v",
			constructorExact, handlerExact, namedStageError, deferredClose, joinedCloseError, usedOuterAuthority,
		)
	}
}

func TestRuntimeAppIsTheOnlyProductionComponentCompositionCaller(t *testing.T) {
	root := runtimeGoRoot(t)
	const allowed = "internal/runtimeapp/app.go"
	for _, file := range goFiles(t, root) {
		if strings.HasSuffix(file, "_test.go") {
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
			if name != "NewRuntimeServerHandlerFromComponents" {
				return true
			}
			if relative := rel(t, root, file); relative != allowed {
				t.Fatalf("%s creates a second production runtime composition root", relative)
			}
			return true
		})
	}
}

func TestPrivateCASAccessPortRemainsNeutral(t *testing.T) {
	root := runtimeGoRoot(t)
	path := filepath.Join(root, "internal", "ports", "privatecas", "access.go")
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, imported := range parsed.Imports {
		value, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			continue
		}
		if strings.HasPrefix(value, "analytix.local/runtime-go/internal/") || value == "os" || value == "syscall" ||
			value == "unsafe" || strings.HasPrefix(value, "golang.org/x/sys/") {
			t.Fatalf("private CAS access port depends on native or concrete runtime package %q", value)
		}
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"uintptr", "os.File", "windows.Handle", "syscall.Handle", " any"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("private CAS access port exposes forbidden native capability %q", forbidden)
		}
	}
}

func privateCASFunctionDeclaration(t *testing.T, file *ast.File, name string) *ast.FuncDecl {
	t.Helper()
	var found *ast.FuncDecl
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != name {
			continue
		}
		if found != nil {
			t.Fatalf("production source repeats function %s", name)
		}
		found = function
	}
	if found == nil || found.Body == nil {
		t.Fatalf("production source is missing function %s", name)
	}
	return found
}

func privateCASMatchingCalls(function *ast.FuncDecl, receiver, name string) []*ast.CallExpr {
	matches := make([]*ast.CallExpr, 0)
	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		actualReceiver, actualName := privateCASCallReceiver(call)
		if actualReceiver == receiver && actualName == name {
			matches = append(matches, call)
		}
		return true
	})
	return matches
}

func privateCASExactlyOneCall(
	t *testing.T,
	function *ast.FuncDecl,
	receiver string,
	name string,
) *ast.CallExpr {
	t.Helper()
	matches := privateCASMatchingCalls(function, receiver, name)
	if len(matches) != 1 {
		t.Fatalf("%s has %d calls to %s.%s, want 1", function.Name.Name, len(matches), receiver, name)
	}
	return matches[0]
}

func privateCASCallReceiver(call *ast.CallExpr) (string, string) {
	if call == nil {
		return "", ""
	}
	switch invoked := call.Fun.(type) {
	case *ast.Ident:
		return "", invoked.Name
	case *ast.SelectorExpr:
		receiver, _ := invoked.X.(*ast.Ident)
		if receiver == nil {
			return "", invoked.Sel.Name
		}
		return receiver.Name, invoked.Sel.Name
	default:
		return "", ""
	}
}

func privateCASSelectorMatches(expression ast.Expr, receiver, name string) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != name {
		return false
	}
	return privateCASIdent(selector.X) == receiver
}

func privateCASCallExpressionMatches(expression ast.Expr, receiver, name string, argumentCount int) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) != argumentCount {
		return false
	}
	actualReceiver, actualName := privateCASCallReceiver(call)
	return actualReceiver == receiver && actualName == name
}

func privateCASReturnsCall(function *ast.FuncDecl, target *ast.CallExpr) bool {
	found := false
	ast.Inspect(function.Body, func(node ast.Node) bool {
		statement, ok := node.(*ast.ReturnStmt)
		if ok && len(statement.Results) == 1 && statement.Results[0] == target {
			found = true
		}
		return true
	})
	return found
}

func privateCASDefersCall(function *ast.FuncDecl, receiver, name string) bool {
	found := false
	ast.Inspect(function.Body, func(node ast.Node) bool {
		statement, ok := node.(*ast.DeferStmt)
		if !ok {
			return true
		}
		actualReceiver, actualName := privateCASCallReceiver(statement.Call)
		if actualReceiver == receiver && actualName == name {
			found = true
		}
		return true
	})
	return found
}

func privateCASRequireUnchangedSnapshotElse(t *testing.T, function *ast.FuncDecl) {
	t.Helper()
	var changeCondition *ast.IfStmt
	ast.Inspect(function.Body, func(node ast.Node) bool {
		statement, ok := node.(*ast.IfStmt)
		if !ok {
			return true
		}
		call, ok := statement.Cond.(*ast.CallExpr)
		if !ok {
			return true
		}
		receiver, name := privateCASCallReceiver(call)
		if receiver == "delta" && name == "HasChanges" {
			if changeCondition != nil {
				t.Fatal("checkpoint recovery repeats its authorized-delta condition")
			}
			changeCondition = statement
		}
		return true
	})
	if changeCondition == nil {
		t.Fatal("checkpoint recovery is missing its authorized-delta condition")
	}
	unchanged, ok := changeCondition.Else.(*ast.IfStmt)
	if !ok {
		t.Fatal("checkpoint recovery has no exact unchanged-snapshot alternative")
	}
	fields := map[string]bool{}
	inequalities := 0
	var collect func(ast.Expr) bool
	collect = func(expression ast.Expr) bool {
		binary, ok := expression.(*ast.BinaryExpr)
		if !ok {
			return false
		}
		if binary.Op == token.LOR {
			return collect(binary.X) && collect(binary.Y)
		}
		if binary.Op != token.NEQ {
			return false
		}
		leftReceiver, leftField := privateCASSelectorIdentity(binary.X)
		rightReceiver, rightField := privateCASSelectorIdentity(binary.Y)
		if leftReceiver != "before" || rightReceiver != "after" || leftField == "" || leftField != rightField {
			return false
		}
		inequalities++
		fields[leftField] = true
		return true
	}
	if !collect(unchanged.Cond) || inequalities != 3 || len(fields) != 3 ||
		!fields["RootBindingDigest"] || !fields["RawCaptureDigest"] || !fields["SnapshotDigest"] {
		t.Fatal("checkpoint recovery unchanged branch does not bind every managed snapshot digest")
	}
}

func privateCASSelectorIdentity(expression ast.Expr) (string, string) {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return "", ""
	}
	return privateCASIdent(selector.X), selector.Sel.Name
}

func privateCASCallName(call *ast.CallExpr) string {
	if call == nil {
		return ""
	}
	switch invoked := call.Fun.(type) {
	case *ast.Ident:
		return invoked.Name
	case *ast.SelectorExpr:
		return invoked.Sel.Name
	default:
		return ""
	}
}

func privateCASIdent(expression ast.Expr) string {
	identifier, _ := expression.(*ast.Ident)
	if identifier == nil {
		return ""
	}
	return identifier.Name
}
