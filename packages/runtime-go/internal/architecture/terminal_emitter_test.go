package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
)

type productionFunctionRefV1 struct {
	file     string
	function string
}

func TestTerminalProductionCallGraphIsClosed(t *testing.T) {
	expectedCalls := map[string][]productionFunctionRefV1{
		"PersistBoundary": {
			{file: "app/attachmentpublication/guard.go", function: "PersistBoundary"},
			{file: "app/evidence/publication_authority.go", function: "PersistBoundary"},
			{file: "app/evidence/terminal_emitter.go", function: "PersistCaseTerminalBoundary"},
		},
		"PersistCaseTerminalBoundary": {
			{file: "app/control/interrupt.go", function: "InterruptActiveTurn"},
			{file: "app/evidence/continuation.go", function: "PersistContinuation"},
			{file: "app/evidence/failure_persistence.go", function: "PersistRuntimeFailure"},
			{file: "app/publicationauthority/service.go", function: "PersistCurrentCaseCandidate"},
			{file: "app/publicationauthority/service.go", function: "PersistCurrentCaseFixed"},
		},
		"PersistCurrentCaseCandidate": {
			{file: "server/turn_finalize.go", function: "finalizeRuntimeTurnAfterLoop"},
			{file: "server/turn_start.go", function: "completeStartedRuntimeTurn"},
		},
		"PersistCurrentCaseFixed": {
			{file: "server/runtime_restore.go", function: "abortStaleRuntimeTurnAfterRestart"},
			{file: "server/turn_finalize.go", function: "finalizeRuntimeTurnAfterLoop"},
			{file: "server/turn_start.go", function: "completeHostBoundaryTurn"},
			{file: "server/turn_start.go", function: "completeStartedRuntimeTurn"},
			{file: "server/turn_terminal.go", function: "persistRuntimeTurnFailureForInput"},
		},
		"PersistContinuation": {
			{file: "server/turn_finalize.go", function: "finalizeRuntimeTurnAfterLoop"},
			{file: "server/turn_finalize.go", function: "finalizeRuntimeTurnAfterLoop"},
			{file: "server/turn_finalize.go", function: "finalizeRuntimeTurnAfterLoop"},
		},
		"PersistRuntimeFailure": {
			{file: "server/turn_terminal.go", function: "persistRuntimeTurnFailureForInput"},
			{file: "server/turn_terminal.go", function: "persistRuntimeTurnFailureForInput"},
		},
		"CommitCurrentGeneralCompletion": {
			{file: "app/publicationauthority/service.go", function: "CommitCurrentGeneralCompletion"},
			{file: "server/turn_start.go", function: "completeStartedRuntimeTurn"},
		},
		"CommitGeneralFailureTerminal": {
			{file: "app/control/interrupt.go", function: "InterruptActiveTurn"},
			{file: "app/turn/failure_persistence.go", function: "PersistFailure"},
			{file: "server/turn_start.go", function: "completeStartedRuntimeTurn"},
		},
		"FinalizeCurrentGeneralAfterLoop": {
			{file: "app/publicationauthority/service.go", function: "FinalizeCurrentGeneralAfterLoop"},
			// Continuation's timing observer wraps the existing authority method
			// value; the same owner still performs the fenced publication.
			{file: "server/turn_finalize.go", function: "finalizeRuntimeTurnAfterLoop"},
		},
		"WithCurrentGeneralFixedTerminal": {
			{file: "server/runtime_restore.go", function: "abortStaleRuntimeTurnAfterRestart"},
			{file: "server/turn_finalize.go", function: "finalizeRuntimeTurnAfterLoop"},
			{file: "server/turn_start.go", function: "completeStartedRuntimeTurn"},
			{file: "server/turn_terminal.go", function: "persistRuntimeTurnFailureForInput"},
		},
		"PersistCompletionItems": {
			{file: "app/subagent/startup_recovery.go", function: "AdmitBackgroundCompletionLifecycleV1"},
		},
		"AdmitBackgroundCompletionLifecycleV1": {
			{file: "app/subagent/startup_recovery.go", function: "AdmitAndPublishBackgroundCompletionLifecycleV1"},
		},
		"AdmitAndPublishBackgroundCompletionLifecycleV1": {
			{file: "app/subagent/startup_recovery.go", function: "recordLifecycle"},
			{file: "app/subagent/startup_recovery.go", function: "RecordBackgroundJobLifecycleV1"},
			{file: "app/subagent/startup_recovery.go", function: "RecordBackgroundSubagentLifecycleV1"},
		},
		"recordLifecycle": {
			{file: "app/subagent/startup_recovery.go", function: "RecoverInterrupted"},
			{file: "app/subagent/startup_recovery.go", function: "RecoverPendingDeliveries"},
			{file: "app/subagent/startup_recovery.go", function: "reconcileSettledDeliveredLifecycle"},
		},
		"reconcileSettledDeliveredLifecycle": {
			{file: "app/subagent/startup_recovery.go", function: "RecoverPendingDeliveries"},
		},
		"RecordBackgroundSubagentLifecycleV1": {
			{file: "app/subagent/startup_recovery.go", function: "RecordSubagentLifecycleV1"},
		},
		"RecordBackgroundJobLifecycleV1": {
			{file: "app/subagent/startup_recovery.go", function: "RecordJobLifecycleV1"},
		},
		"FinishBackgroundLifecycleRecordV1": {
			{file: "app/subagent/startup_recovery.go", function: "finishLifecycleRecordV1"},
		},
		"PublishBackgroundCompletionLifecycleExact": {
			{file: "app/subagent/startup_recovery.go", function: "AdmitAndPublishBackgroundCompletionLifecycleV1"},
		},
		"ReconcileBackgroundCompletionLifecycleExactV1": {
			{file: "server/subagent_jobs.go", function: "PublishBackgroundCompletionLifecycleExact"},
		},
		"BuildDurableSubagentLifecycleEventsV1": {
			{file: "app/subagent/startup_recovery.go", function: "RecordBackgroundSubagentLifecycleV1"},
		},
		"BuildBackgroundJobCompletionNotificationItem": {
			{file: "app/subagent/background_delivery.go", function: "PersistCompletionItems"},
			{file: "app/subagent/startup_recovery.go", function: "reconcileSettledDeliveredLifecycle"},
		},
		"BuildBackgroundJobCompletionNotificationEvent": {
			{file: "app/subagent/background_delivery.go", function: "admitCompletion"},
			{file: "app/subagent/job_identity.go", function: "BuildJobLifecycleEvents"},
			{file: "app/subagent/progress_events.go", function: "BuildSubagentProgressEvents"},
			{file: "app/subagent/progress_events.go", function: "BuildSubagentProgressEvents"},
		},
	}
	actual, _ := productionASTInventoryV1(t, expectedCalls, nil)
	for name, want := range expectedCalls {
		assertProductionRefsV1(t, name, actual[name], want)
	}
}

func TestBackgroundSubagentLifecycleReloadDominatesTerminalClassification(t *testing.T) {
	path := filepath.Join(runtimeGoRoot(t), "internal", "app", "subagent", "startup_recovery.go")
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var target *ast.FuncDecl
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "RecordBackgroundSubagentLifecycleV1" {
			target = function
			break
		}
	}
	if target == nil || target.Body == nil {
		t.Fatal("RecordBackgroundSubagentLifecycleV1 production owner is missing")
	}
	positions := map[string][]token.Pos{}
	ast.Inspect(target.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := calledFunctionNameV1(call.Fun)
		switch name {
		case "LoadChildRun", "TaskJobTerminal", "BuildDurableSubagentLifecycleEventsV1":
			positions[name] = append(positions[name], call.Pos())
		}
		return true
	})
	if len(positions["LoadChildRun"]) != 1 || len(positions["TaskJobTerminal"]) != 1 ||
		len(positions["BuildDurableSubagentLifecycleEventsV1"]) != 1 {
		t.Fatalf("background lifecycle canonicalization graph changed: %#v", positions)
	}
	if positions["LoadChildRun"][0] >= positions["TaskJobTerminal"][0] {
		t.Fatal("durable child reload no longer dominates terminal classification")
	}
}

func TestTerminalReasonOwnersAndNonTerminalVocabularyAreClosed(t *testing.T) {
	expectedOwners := map[evidenceapp.TerminalProductionOwnerV1]productionFunctionRefV1{
		evidenceapp.TerminalOwnerForegroundCompletionV1: {file: "server/turn_start.go", function: "completeStartedRuntimeTurn"},
		evidenceapp.TerminalOwnerHostBoundaryV1:         {file: "server/turn_start.go", function: "completeHostBoundaryTurn"},
		evidenceapp.TerminalOwnerRuntimeFailureV1:       {file: "server/turn_terminal.go", function: "persistRuntimeTurnFailureForInput"},
		evidenceapp.TerminalOwnerGateContinuationV1:     {file: "server/turn_finalize.go", function: "finalizeRuntimeTurnAfterLoop"},
		evidenceapp.TerminalOwnerInterruptV1:            {file: "app/control/interrupt.go", function: "InterruptActiveTurn"},
		evidenceapp.TerminalOwnerRestartV1:              {file: "server/runtime_restore.go", function: "abortStaleRuntimeTurnAfterRestart"},
		evidenceapp.TerminalOwnerCurrentAuthorityV1:     {file: "app/evidence/publication_authority.go", function: "PersistBoundary"},
	}
	_, functions := productionASTInventoryV1(t, nil, nil)
	seenOwners := map[evidenceapp.TerminalProductionOwnerV1]bool{}
	seenReasons := map[evidenceapp.TerminalReason]bool{}
	for _, row := range evidenceapp.ProductionTerminalReasonOwnershipV1() {
		if seenReasons[row.Reason] {
			t.Fatalf("duplicate terminal reason ownership row: %s", row.Reason)
		}
		seenReasons[row.Reason] = true
		for _, owner := range row.Owners {
			location, ok := expectedOwners[owner]
			if !ok {
				t.Fatalf("terminal reason %s names unknown production owner %s", row.Reason, owner)
			}
			seenOwners[owner] = true
			if !functions[location] {
				t.Fatalf("terminal owner %s is not the expected production function %#v", owner, location)
			}
		}
		if row.Class == evidenceapp.TerminalEmissionTurnV1 && len(row.Owners) == 0 {
			t.Fatalf("turn terminal reason has no production owner: %s", row.Reason)
		}
	}
	if len(seenReasons) != len(evidenceapp.AllTerminalReasons()) || len(seenOwners) != len(expectedOwners) {
		t.Fatalf("terminal ownership inventory is incomplete: reasons=%d/%d owners=%d/%d", len(seenReasons), len(evidenceapp.AllTerminalReasons()), len(seenOwners), len(expectedOwners))
	}

	dormant := map[string][]productionFunctionRefV1{
		"TerminalResume": {
			{file: "app/evidence/final_gate.go"},
			{file: "app/evidence/terminal_ownership.go", function: "ProductionTerminalReasonOwnershipV1"},
		},
		"TerminalReportFallback": {
			{file: "app/evidence/final_gate.go"},
			{file: "app/evidence/terminal_ownership.go", function: "ProductionTerminalReasonOwnershipV1"},
		},
		"TerminalBackgroundCompletion": {
			{file: "app/evidence/final_gate.go"},
			{file: "app/evidence/terminal_ownership.go", function: "ProductionTerminalReasonOwnershipV1"},
		},
	}
	identifiers, _ := productionASTInventoryV1(t, nil, dormant)
	for name, want := range dormant {
		assertProductionRefsV1(t, name, identifiers[name], want)
	}
}

func productionASTInventoryV1(
	t *testing.T,
	callNames map[string][]productionFunctionRefV1,
	identifierNames map[string][]productionFunctionRefV1,
) (map[string][]productionFunctionRefV1, map[productionFunctionRefV1]bool) {
	t.Helper()
	internalRoot := filepath.Join(runtimeGoRoot(t), "internal")
	references := map[string][]productionFunctionRefV1{}
	functions := map[productionFunctionRefV1]bool{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(internalRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(internalRoot, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			if relative == "testsupport" || strings.HasPrefix(relative, "testsupport/") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			location := productionFunctionRefV1{file: relative, function: function.Name.Name}
			functions[location] = true
			ast.Inspect(function.Body, func(node ast.Node) bool {
				if call, ok := node.(*ast.CallExpr); ok {
					name := calledFunctionNameV1(call.Fun)
					if _, tracked := callNames[name]; tracked {
						references[name] = append(references[name], location)
					}
				}
				if identifier, ok := node.(*ast.Ident); ok {
					if _, tracked := identifierNames[identifier.Name]; tracked {
						references[identifier.Name] = append(references[identifier.Name], location)
					}
				}
				return true
			})
		}
		if len(identifierNames) > 0 {
			for _, declaration := range parsed.Decls {
				general, ok := declaration.(*ast.GenDecl)
				if !ok {
					continue
				}
				ast.Inspect(general, func(node ast.Node) bool {
					identifier, ok := node.(*ast.Ident)
					if ok {
						if _, tracked := identifierNames[identifier.Name]; tracked {
							references[identifier.Name] = append(references[identifier.Name], productionFunctionRefV1{file: relative})
						}
					}
					return true
				})
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return references, functions
}

func calledFunctionNameV1(function ast.Expr) string {
	switch typed := function.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.SelectorExpr:
		return typed.Sel.Name
	default:
		return ""
	}
}

func assertProductionRefsV1(t *testing.T, name string, got, want []productionFunctionRefV1) {
	t.Helper()
	sortRefs := func(refs []productionFunctionRefV1) []productionFunctionRefV1 {
		out := append([]productionFunctionRefV1(nil), refs...)
		sort.Slice(out, func(i, j int) bool {
			if out[i].file != out[j].file {
				return out[i].file < out[j].file
			}
			return out[i].function < out[j].function
		})
		return out
	}
	got, want = sortRefs(got), sortRefs(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("production reference graph changed for %s: got=%#v want=%#v", name, got, want)
	}
}
