package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func reportASTParents(file *ast.File) map[ast.Node]ast.Node {
	parents := map[ast.Node]ast.Node{}
	var stack []ast.Node
	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if len(stack) != 0 {
			parents[node] = stack[len(stack)-1]
		}
		stack = append(stack, node)
		return true
	})
	return parents
}

func reportContainingFunction(node ast.Node, parents map[ast.Node]ast.Node) (*ast.FuncDecl, *ast.FuncLit) {
	var callback *ast.FuncLit
	for node != nil {
		if literal, ok := node.(*ast.FuncLit); ok && callback == nil {
			callback = literal
		}
		if function, ok := node.(*ast.FuncDecl); ok {
			return function, callback
		}
		node = parents[node]
	}
	return nil, callback
}

func reportNamedFunction(file *ast.File, name string) *ast.FuncDecl {
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok && function.Name.Name == name {
			return function
		}
	}
	return nil
}

func reportPublicationCompositionProblems(relative string, file *ast.File) []string {
	const ownerImport = "analytix.local/runtime-go/internal/app/reportpublication"
	alias := ""
	for _, imported := range file.Imports {
		name, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			return []string{"invalid report owner import"}
		}
		if name != ownerImport {
			continue
		}
		alias = "reportpublication"
		if imported.Name != nil {
			alias = imported.Name.Name
		}
		if alias == "." || alias == "_" || strings.TrimSpace(alias) == "" {
			return []string{"non-auditable report publication import mode"}
		}
	}
	if alias == "" {
		return nil
	}
	parents := reportASTParents(file)
	// These were already admitted by the original gate. Additional recovery
	// data below grants no capability; new constructors remain denied by default.
	recovery := map[string]bool{"VerifyTrustedInventoryV1": true, "PreflightRestartV1": true, "RestartPreflightConfigV1": true}
	data := map[string]bool{}
	for _, name := range strings.Fields(`RestartPlanV1 RestartAttemptV1
        RestartAttemptReservedV1 RestartAttemptCandidateDurableV1 RestartAttemptMaterialsDurableV1
        RestartAttemptIntentDurableV1 RestartAttemptCommittedSettlementV1 RestartAttemptCommitSelectionV1
        RestartAttemptCommitReceiptV1 RestartAttemptDeliveryDecisionV1 RestartAttemptGrantSettlementV1
        RestartAttemptStageDispositionV1 RestartAttemptStageCompletionV1 RestartAttemptDeliveryProjectionV1
        RestartAttemptDeliveryRejectionV1 RestartAttemptDeliveryBlockedV1 RestartAttemptAbortedV1`) {
		data[name] = true
	}
	var problems []string
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || privateCASIdent(selector.X) != alias {
			return true
		}
		name := selector.Sel.Name
		if recovery[name] {
			return true
		}
		function, callback := reportContainingFunction(selector, parents)
		functionName := ""
		if function != nil {
			functionName = function.Name.Name
		}
		if strings.HasPrefix(relative, "internal/runtimeapp/") && data[name] {
			return true
		}
		if relative == "internal/runtimeapp/original_report_deferred.go" && functionName == "validateRuntimeDeferredOriginalReportResultV1" && name == "ValidateStoredSettledResultV1" {
			if call, ok := parents[selector].(*ast.CallExpr); ok && call.Fun == selector && len(call.Args) == 5 && privateCASIdent(call.Args[0]) == "ctx" && privateCASIdent(call.Args[1]) == "entry" && privateCASIdent(call.Args[2]) == "settled" && registryObservationExpressionName(call.Args[3]) == "core.verification.KeyID()" && registryObservationExpressionName(call.Args[4]) == "core.verification.PublicKey()" {
				return true
			}
		}
		if relative == "internal/runtimeapp/original_report_history.go" && functionName == "prepareRuntimeReportHistoryEndpointV1" && name == "VerifyStoredAttemptHistoryV1" {
			if call, ok := parents[selector].(*ast.CallExpr); ok && call.Fun == selector && len(call.Args) == 3 && privateCASIdent(call.Args[0]) == "ctx" && privateCASIdent(call.Args[1]) == "historical" && privateCASIdent(call.Args[2]) == "entry" {
				return true
			}
		}
		if relative == "internal/runtimeapp/optional_publication_graph.go" && functionName == "validateRuntimePublicationGraphV1" && name == "StoredControlledOutcomesV1" {
			if value, ok := parents[selector].(*ast.ValueSpec); ok && value.Type == selector && len(value.Names) == 1 && value.Names[0].Name == "outcomes" && len(value.Values) == 0 {
				return true
			}
		}
		if relative == "internal/runtimeapp/optional_publication_startup_gates.go" &&
			(functionName == "prepareRuntimeReportScopeBeforeRecoveryV1" || functionName == "validateRuntimeOriginalHeldReportAttemptsV1") &&
			(name == "VerifyAttemptCoreInventoryV1" || name == "ErrAttemptCoreLinkageV1") {
			return true
		}
		if name == "ProjectedDeliveryAuthorityV1" && relative == "internal/runtimeapp/original_report_history.go" {
			for ancestor := parents[selector]; ancestor != nil; ancestor = parents[ancestor] {
				if declared, ok := ancestor.(*ast.TypeSpec); ok && declared.Name.Name == "runtimeOriginalReportHistoryV1" {
					pointer, pointerOK := parents[selector].(*ast.StarExpr)
					field, fieldOK := parents[pointer].(*ast.Field)
					if pointerOK && fieldOK && len(field.Names) == 1 && field.Names[0].Name == "authority" {
						return true
					}
				}
			}
		}
		if relative == "internal/runtimeapp/original_report_history.go" && functionName == "prepareRuntimeReportHistoryEndpointV1" && callback == nil {
			var call *ast.CallExpr
			if name == "NewHistoricalProjectedDeliveryAuthorityV1" {
				call, _ = parents[selector].(*ast.CallExpr)
			}
			if name == "ProjectedDeliveryAuthorityConfigV1" {
				if literal, ok := parents[selector].(*ast.CompositeLit); ok {
					call, _ = parents[literal].(*ast.CallExpr)
				}
			}
			if reportHistoricalConstructionValid(call, alias, function) {
				return true
			}
		}
		if name == "NewHistoricalArtifactDeliveryBridgeV1" {
			call, _ := parents[selector].(*ast.CallExpr)
			if reportHistoricalBridgeCompositionValid(relative, function, callback, call, parents) {
				return true
			}
		}
		problems = append(problems, "report selector is outside its admitted historical boundary: "+name)
		return true
	})
	return problems
}

func reportHistoricalConstructionValid(call *ast.CallExpr, alias string, function *ast.FuncDecl) bool {
	if call == nil || !privateCASCallExpressionMatches(call, alias, "NewHistoricalProjectedDeliveryAuthorityV1", 1) {
		return false
	}
	config, ok := call.Args[0].(*ast.CompositeLit)
	if !ok || !privateCASSelectorMatches(config.Type, alias, "ProjectedDeliveryAuthorityConfigV1") {
		return false
	}
	fields := map[string]ast.Expr{}
	for _, element := range config.Elts {
		field, ok := element.(*ast.KeyValueExpr)
		if !ok {
			return false
		}
		name := privateCASIdent(field.Key)
		if name == "" || fields[name] != nil {
			return false
		}
		switch name {
		case "Evidence", "PIIAuthority", "ValidateCurrent", "AcquireContextEffect":
			return false
		}
		fields[name] = field.Value
	}
	if registryObservationExpressionName(fields["Authority"]) != "core.verification" || privateCASIdent(fields["Pending"]) != "completed" {
		return false
	}
	completed, preflight := false, false
	ast.Inspect(function.Body, func(node ast.Node) bool {
		earlier, ok := node.(*ast.CallExpr)
		if ok && earlier.Pos() < call.Pos() {
			completed = completed || privateCASCallName(earlier) == "PrepareCompletedReportSnapshotV1"
			preflight = preflight || privateCASCallExpressionMatches(earlier, alias, "PreflightRestartV1", 2)
		}
		return true
	})
	return completed && preflight
}

func reportHistoricalBridgeCompositionValid(relative string, function *ast.FuncDecl, callback *ast.FuncLit, call *ast.CallExpr, parents map[ast.Node]ast.Node) bool {
	if function == nil || call == nil || len(call.Args) != 4 {
		return false
	}
	var scope ast.Node = function.Body
	var expected []string
	switch {
	case relative == "internal/runtimeapp/app.go" && function.Name.Name == "newRuntimeServerHandlerWithRootsModeE":
		if callback == nil || callback.Type.Params == nil || len(callback.Type.Params.List) != 1 {
			return false
		}
		parameter := callback.Type.Params.List[0]
		pointer, ok := parameter.Type.(*ast.StarExpr)
		if !ok || privateCASIdent(pointer.X) != "runtimeOriginalReportHistoryV1" || len(parameter.Names) != 1 || parameter.Names[0].Name != "candidate" {
			return false
		}
		gate, ok := parents[callback].(*ast.CallExpr)
		if !ok || registryObservationExpressionName(gate.Fun) != "history.semantic.withSemanticCandidateV1" || len(gate.Args) != 5 || gate.Args[4] != callback || privateCASIdent(gate.Args[0]) != "ctx" || privateCASIdent(gate.Args[1]) != "nil" || privateCASIdent(gate.Args[2]) != "nil" {
			return false
		}
		guardAssignment, ok := parents[gate].(*ast.AssignStmt)
		if !ok || len(guardAssignment.Lhs) != 1 || privateCASIdent(guardAssignment.Lhs[0]) != "err" {
			return false
		}
		guard, ok := parents[guardAssignment].(*ast.IfStmt)
		if !ok || guard.Init != guardAssignment || len(guard.Body.List) != 1 {
			return false
		}
		condition, ok := guard.Cond.(*ast.BinaryExpr)
		if !ok || condition.Op != token.NEQ || privateCASIdent(condition.X) != "err" || privateCASIdent(condition.Y) != "nil" {
			return false
		}
		failure, ok := guard.Body.List[0].(*ast.ReturnStmt)
		if !ok || len(failure.Results) != 2 || privateCASIdent(failure.Results[0]) != "nil" || privateCASIdent(failure.Results[1]) != "err" {
			return false
		}
		empty, ok := staticReportStringV1(gate.Args[3])
		if !ok || empty != "" {
			return false
		}
		scope = callback.Body
		expected = []string{"candidate.authority", "reportPublicationStores.Commits", "reportPublicationStores.Receipts", "finalAuthority"}
	case relative == "internal/runtimeapp/original_controlled_access_history.go" && function.Name.Name == "verifyRuntimeControlledAccessEndpointV2" && callback == nil:
		expected = []string{"history.authority", "reports.Commits", "reports.Receipts", "core.verification"}
		completed, originalReaders := false, false
		ast.Inspect(function.Body, func(node ast.Node) bool {
			prior, ok := node.(*ast.CallExpr)
			if ok && prior.Pos() < call.Pos() {
				completed = completed || privateCASCallExpressionMatches(prior, "history", "matchesCompletedPlanV1", 1)
				originalReaders = originalReaders || privateCASCallName(prior) == "ParseOriginalReadersV1"
			}
			return true
		})
		if !completed || !originalReaders {
			return false
		}
	default:
		return false
	}
	for index, want := range expected {
		if registryObservationExpressionName(call.Args[index]) != want {
			return false
		}
	}
	assignment, ok := parents[call].(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 2 || len(assignment.Rhs) != 1 || privateCASIdent(assignment.Lhs[0]) != "bridge" || privateCASIdent(assignment.Lhs[1]) != "err" {
		return false
	}
	// The bridge can feed only the Historical slot of the audit dependency;
	// aliases, returns, method values and current/live dependency slots fail.
	valid, used := true, false
	ast.Inspect(scope, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok || identifier.Name != "bridge" {
			return true
		}
		if identifier == assignment.Lhs[0] {
			return true
		}
		field, ok := parents[identifier].(*ast.KeyValueExpr)
		if !ok || field.Value != identifier || privateCASIdent(field.Key) != "Historical" {
			valid = false
			return true
		}
		literal, ok := parents[field].(*ast.CompositeLit)
		if !ok {
			valid = false
			return true
		}
		owner, ok := literal.Type.(*ast.SelectorExpr)
		if !ok || owner.Sel.Name != "ControlledAccessInventoryDependenciesV2" {
			valid = false
			return true
		}
		used = true
		return true
	})
	return valid && used
}

// The callback name is not itself evidence of a completed semantic check.
// Require its implementation to reconstruct the exact candidate, reject a
// mismatched completed plan, and invoke use only after those checks.
func reportSemanticCandidateGateProblem(file *ast.File) string {
	function := reportNamedFunction(file, "withSemanticCandidateV1")
	if function == nil || function.Body == nil {
		return "report semantic candidate gate is missing"
	}
	parents := reportASTParents(file)
	var prepared, matched, used *ast.CallExpr
	problem := ""
	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch registryObservationExpressionName(call.Fun) {
		case "prepareRuntimeReportHistoryEndpointV1":
			if prepared != nil {
				problem = "report semantic candidate is reconstructed ambiguously"
			}
			prepared = call
		case "preserved.history.matchesCompletedPlanV1":
			matched = call
		case "use":
			if used != nil {
				problem = "report semantic callback has multiple invocation paths"
			}
			used = call
		}
		return true
	})
	if problem != "" {
		return problem
	}
	if prepared == nil || matched == nil || used == nil || !(prepared.Pos() < matched.Pos() && matched.Pos() < used.Pos()) {
		return "report semantic callback precedes completed candidate validation"
	}
	assignment, ok := parents[prepared].(*ast.AssignStmt)
	if !ok || len(assignment.Lhs) != 2 || privateCASIdent(assignment.Lhs[0]) != "candidate" {
		return "report semantic callback does not use its reconstructed candidate"
	}
	if len(matched.Args) != 1 || !privateCASSelectorMatches(matched.Args[0], "candidate", "plan") || len(used.Args) != 1 || privateCASIdent(used.Args[0]) != "candidate" {
		return "report semantic callback substitutes its completed candidate"
	}
	negative, ok := parents[matched].(*ast.UnaryExpr)
	if !ok || negative.Op != token.NOT {
		return "report semantic gate lost its fail-closed plan comparison"
	}
	guard, ok := parents[negative].(*ast.IfStmt)
	if !ok || guard.Cond != negative || len(guard.Body.List) != 1 {
		return "report semantic gate lost its mismatch rejection"
	}
	rejected, ok := guard.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(rejected.Results) != 1 || !privateCASCallExpressionMatches(rejected.Results[0], "errors", "New", 1) {
		return "report semantic gate does not return its mismatch error"
	}
	returned, ok := parents[used].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 || returned.Results[0] != used {
		return "report semantic gate ignores the callback error"
	}
	return ""
}

func reportHistoricalOwnerProblem(authority, bridge *ast.File) string {
	historical := reportNamedFunction(authority, "NewHistoricalProjectedDeliveryAuthorityV1")
	if historical == nil || historical.Body == nil || len(historical.Body.List) < 4 {
		return "historical report constructor is missing"
	}
	fields := []string{"Evidence", "PIIAuthority", "ValidateCurrent", "AcquireContextEffect"}
	for index, name := range fields {
		assignment, ok := historical.Body.List[index].(*ast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 || !privateCASSelectorMatches(assignment.Lhs[0], "config", name) || privateCASIdent(assignment.Rhs[0]) != "nil" {
			return "historical report constructor retained current capability " + name
		}
	}
	problem := ""
	ast.Inspect(historical.Body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, target := range value.Lhs {
				name := registryObservationExpressionName(target)
				if name == "config" {
					problem = "historical constructor substitutes its sanitized configuration"
				}
				for index, field := range fields {
					if name == "config."+field && value != historical.Body.List[index] {
						problem = "historical constructor restores current capability " + field
					}
				}
			}
		case *ast.ReturnStmt:
			if len(value.Results) != 2 {
				problem = "historical constructor returns an unreviewed capability"
				break
			}
			if privateCASIdent(value.Results[0]) == "nil" && privateCASIdent(value.Results[1]) == "ErrProjectedDeliveryUnavailable" {
				break
			}
			address, ok := value.Results[0].(*ast.UnaryExpr)
			if !ok || address.Op != token.AND || privateCASIdent(value.Results[1]) != "nil" {
				problem = "historical constructor returns another authority"
				break
			}
			literal, ok := address.X.(*ast.CompositeLit)
			if !ok || privateCASIdent(literal.Type) != "ProjectedDeliveryAuthorityV1" || len(literal.Elts) != 1 {
				problem = "historical constructor returns another authority"
				break
			}
			field, ok := literal.Elts[0].(*ast.KeyValueExpr)
			if !ok || privateCASIdent(field.Key) != "config" || privateCASIdent(field.Value) != "config" {
				problem = "historical constructor discards sanitized configuration"
			}
		case *ast.CallExpr:
			name := registryObservationExpressionName(value.Fun)
			if name == "validHistoricalProjectedDeliveryConfigV1" || name == "append" {
				return true
			}
			if array, ok := value.Fun.(*ast.ArrayType); ok && array.Len == nil && privateCASIdent(array.Elt) == "byte" {
				return true
			}
			problem = "historical constructor invokes an unreviewed capability provider"
		}
		return true
	})
	if problem != "" {
		return problem
	}
	current := reportNamedFunction(authority, "resolveCurrentProjectedDeliveryV1")
	if current == nil || current.Body == nil {
		return "current report resolution guard is missing"
	}
	denied := false
	for _, statement := range current.Body.List {
		guard, ok := statement.(*ast.IfStmt)
		if !ok {
			continue
		}
		found := map[string]bool{}
		var walk func(ast.Expr) bool
		walk = func(expression ast.Expr) bool {
			binary, ok := expression.(*ast.BinaryExpr)
			if !ok {
				return false
			}
			if binary.Op == token.LOR {
				return walk(binary.X) && walk(binary.Y)
			}
			name := registryObservationExpressionName(binary.X)
			if binary.Op != token.EQL || privateCASIdent(binary.Y) != "nil" || !strings.HasPrefix(name, "authority.config.") {
				return false
			}
			found[strings.TrimPrefix(name, "authority.config.")] = true
			return true
		}
		if !walk(guard.Cond) || len(found) != len(fields) || len(guard.Body.List) != 1 {
			continue
		}
		all := true
		for _, name := range fields {
			all = all && found[name]
		}
		result, ok := guard.Body.List[0].(*ast.ReturnStmt)
		if all && ok && len(result.Results) == 2 && privateCASIdent(result.Results[1]) == "ErrProjectedDeliveryUnavailable" {
			denied = true
			break
		}
	}
	if !denied {
		return "current report resolution no longer rejects historical-only authority"
	}
	bridgeShape := false
	ast.Inspect(bridge, func(node ast.Node) bool {
		declared, ok := node.(*ast.TypeSpec)
		if !ok || declared.Name.Name != "HistoricalArtifactDeliveryBridgeV1" {
			return true
		}
		structure, ok := declared.Type.(*ast.StructType)
		if !ok || len(structure.Fields.List) != 2 {
			return false
		}
		projected, materials := structure.Fields.List[0], structure.Fields.List[1]
		bridgeShape = len(projected.Names) == 1 && projected.Names[0].Name == "projected" && privateCASSelectorMatches(projected.Type, "publicationport", "HistoricalProjectedDeliveryAuthority") &&
			len(materials.Names) == 1 && materials.Names[0].Name == "materials" && privateCASIdent(materials.Type) == "artifactDeliveryBridgeMaterialsV1"
		return false
	})
	if !bridgeShape {
		return "historical artifact bridge embeds or stores current capability"
	}
	methods := 0
	for _, declaration := range bridge.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv == nil || len(function.Recv.List) != 1 {
			continue
		}
		receiver := function.Recv.List[0].Type
		if pointer, ok := receiver.(*ast.StarExpr); ok {
			receiver = pointer.X
		}
		if privateCASIdent(receiver) != "HistoricalArtifactDeliveryBridgeV1" {
			continue
		}
		if function.Name.Name != "ResolveTrustedHistorical" {
			return "historical artifact bridge acquired a non-historical method"
		}
		methods++
	}
	if methods != 1 {
		return "historical artifact bridge lost its sole audit method"
	}
	return ""
}

func TestReportHistoricalCompositionGuardRejectsCurrentAndCallbackEscapes(t *testing.T) {
	root := runtimeGoRoot(t)
	const relative = "internal/runtimeapp/app.go"
	body, err := os.ReadFile(filepath.Join(root, relative))
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	constructor := "reportpublicationapp.NewHistoricalArtifactDeliveryBridgeV1(candidate.authority, reportPublicationStores.Commits, reportPublicationStores.Receipts, finalAuthority)"
	cases := map[string]string{
		"original":                            source,
		"current bridge":                      strings.Replace(source, "reportpublicationapp.NewHistoricalArtifactDeliveryBridgeV1(", "reportpublicationapp.NewCurrentArtifactDeliveryBridgeV1(", 1),
		"publication service":                 strings.Replace(source, "bridge, err := "+constructor, "bridge, err := reportpublicationapp.NewService(nil)", 1),
		"outside candidate gate":              strings.Replace(source, "history.semantic.withSemanticCandidateV1(ctx, nil, nil, \"\", func(candidate", "history.semantic.withUncheckedCandidateV1(ctx, nil, nil, \"\", func(candidate", 1),
		"old authority":                       strings.Replace(source, constructor, strings.Replace(constructor, "candidate.authority", "history.authority", 1), 1),
		"current dependency":                  strings.Replace(source, "Historical: bridge", "Current: bridge", 1),
		"escaped bridge":                      strings.Replace(source, "Historical: bridge", "Historical: bridge, Escaped: bridge", 1),
		"fresh store bridge outside callback": strings.Replace(source, "historicalControlledAvailable := false", "historicalControlledAvailable := false\n_, _ = reportpublicationapp.NewHistoricalArtifactDeliveryBridgeV1(history.authority, reportPublicationStores.Commits, reportPublicationStores.Receipts, finalAuthority)", 1),
	}
	for name, candidate := range cases {
		t.Run(name, func(t *testing.T) {
			if name != "original" && candidate == source {
				t.Fatal("mutation did not change source")
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), relative, candidate, 0)
			if err != nil {
				t.Fatal(err)
			}
			problems := reportPublicationCompositionProblems(relative, parsed)
			if (len(problems) == 0) != (name == "original") {
				t.Fatalf("composition guard: %v", problems)
			}
		})
	}
}

func TestReportHistoricalOwnerGuardRejectsCurrentCapabilityRestoration(t *testing.T) {
	root := runtimeGoRoot(t)
	authorityPath := filepath.Join(root, "internal/app/reportpublication/projected_delivery_authority.go")
	bridgePath := filepath.Join(root, "internal/app/reportpublication/artifact_delivery_bridge.go")
	body, err := os.ReadFile(authorityPath)
	if err != nil {
		t.Fatal(err)
	}
	bridgeBody, err := os.ReadFile(bridgePath)
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, field := range []string{"original", "Evidence", "PIIAuthority", "ValidateCurrent", "AcquireContextEffect", "restored current capability", "current nil guard", "embedded current bridge", "current method"} {
		t.Run(field, func(t *testing.T) {
			candidate, bridgeSource := source, string(bridgeBody)
			if field == "current method" {
				bridgeSource += "\nfunc (bridge *HistoricalArtifactDeliveryBridgeV1) ResolveCurrent() {}\n"
			} else if field == "current nil guard" {
				candidate = strings.Replace(source, "authority.config.Evidence == nil || ", "", 1)
			} else if field == "embedded current bridge" {
				bridgeSource = strings.Replace(bridgeSource, "type HistoricalArtifactDeliveryBridgeV1 struct {", "type HistoricalArtifactDeliveryBridgeV1 struct {\n*CurrentArtifactDeliveryBridgeV1", 1)
			} else if field == "restored current capability" {
				candidate = strings.Replace(source, "config.AcquireContextEffect = nil", "config.AcquireContextEffect = nil\nconfig.Evidence = currentCapability", 1)
			} else if field != "original" {
				candidate = strings.Replace(source, "config."+field+" = nil", "config."+field+" = currentCapability", 1)
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), authorityPath, candidate, 0)
			if err != nil {
				t.Fatal(err)
			}
			parsedBridge, err := parser.ParseFile(token.NewFileSet(), bridgePath, bridgeSource, 0)
			if err != nil {
				t.Fatal(err)
			}
			problem := reportHistoricalOwnerProblem(parsed, parsedBridge)
			if (problem == "") != (field == "original") {
				t.Fatalf("historical capability guard: %s", problem)
			}
		})
	}
}

func TestReportSemanticCandidateGuardRejectsIncompleteOrSubstitutedCallback(t *testing.T) {
	root := runtimeGoRoot(t)
	path := filepath.Join(root, "internal/runtimeapp/original_report_semantic_preservation.go")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for name, candidate := range map[string]string{
		"original":                 source,
		"missing completed check":  strings.Replace(source, "preserved.history.matchesCompletedPlanV1(candidate.plan)", "preserved.history.acceptUncheckedPlanV1(candidate.plan)", 1),
		"reversed completed check": strings.Replace(source, "!preserved.history.matchesCompletedPlanV1(candidate.plan)", "preserved.history.matchesCompletedPlanV1(candidate.plan)", 1),
		"different callback input": strings.Replace(source, "return use(candidate)", "return use(preserved.history)", 1),
		"early callback":           strings.Replace(source, "candidate, err := prepareRuntimeReportHistoryEndpointV1", "_ = use(preserved.history)\ncandidate, err := prepareRuntimeReportHistoryEndpointV1", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if name != "original" && candidate == source {
				t.Fatal("mutation did not change source")
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), path, candidate, 0)
			if err != nil {
				t.Fatal(err)
			}
			problem := reportSemanticCandidateGateProblem(parsed)
			if (problem == "") != (name == "original") {
				t.Fatalf("semantic candidate guard: %s", problem)
			}
		})
	}
}
