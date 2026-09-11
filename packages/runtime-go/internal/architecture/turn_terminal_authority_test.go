package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCaseTerminalMutationHasSingleProductionCoordinator(t *testing.T) {
	root := runtimeGoRoot(t)
	allowedCalls := map[string]map[string]map[string]bool{
		"CommitV1": {
			"internal/app/evidence/case_boundary_completion.go": {"persistPrivateFinalV1": true},
			"internal/app/turnterminal/recovery.go":             {"RecoverV1": true},
		},
		"PersistAcceptedFinalTerminal": {
			"internal/app/turnterminal/coordinator.go": {"CommitV1": true},
		},
		"FinishTurnIfActiveWithAcceptedFinalAuthority": {
			"internal/app/turn/accepted_final.go": {"PersistAcceptedFinalTerminal": true},
		},
		"ResolveCommittedV1": {
			"internal/app/evidence/case_boundary_completion.go": {"ReconcileCommittedInterrupt": true},
		},
		"CloseTurn": {
			"internal/app/turnterminal/coordinator.go": {"CommitV1": true},
		},
		"FinishTurnIfActiveWithItemsAndFields": {
			"internal/app/turn/finalize_after_loop.go": {"CommitCompletedTurn": true},
			"internal/app/turn/failure_persistence.go": {"CommitGeneralFailureTerminal": true},
		},
		"FinishTurnIfActive":          {},
		"FinishTurnIfActiveWithItems": {},
		"FinishTurn":                  {},
		"FailTurnIfActive":            {},
		"FailTurnIfActiveWithItems":   {},
	}
	for _, file := range goFiles(t, root) {
		if strings.HasSuffix(file, "_test.go") || hasBuildTag(t, file, "!analytix_prod") {
			continue
		}
		relative := rel(t, root, file)
		parsed := parseGoFile(t, file, 0)
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if function.Name.Name == "PersistAcceptedFinalCompletion" {
				t.Fatalf("%s#%s restores the retired case-terminal publication alias", relative, function.Name.Name)
			}
			if function.Name.Name == "FinishTurnIfActiveWithAcceptedFinalAuthority" &&
				relative != "internal/server/durable_turns.go" {
				t.Fatalf("%s#%s implements the authorized accepted-final CAS outside its sole adapter", relative, function.Name.Name)
			}
			if function.Body == nil {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := ""
				switch target := call.Fun.(type) {
				case *ast.SelectorExpr:
					name = target.Sel.Name
				case *ast.Ident:
					name = target.Name
				}
				allowFiles, guarded := allowedCalls[name]
				if !guarded {
					return true
				}
				if !allowFiles[relative][function.Name.Name] {
					t.Fatalf("%s#%s bypasses the sole case-terminal mutation owner through %s", relative, function.Name.Name, name)
				}
				return true
			})
		}
	}
}

func TestAcceptedFinalDispositionHasSingleProductionIssuer(t *testing.T) {
	root := runtimeGoRoot(t)
	forbiddenFunctions := map[string]bool{
		"ApplyFinalAuthorityDispositionRepairs": true,
		"ApplyAcceptedFinalPublicCommitRepairs": true,
	}
	for _, file := range goFiles(t, root) {
		if strings.HasSuffix(file, "_test.go") || hasBuildTag(t, file, "!analytix_prod") {
			continue
		}
		relative := rel(t, root, file)
		if strings.HasPrefix(relative, "internal/testsupport/") {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if forbiddenFunctions[function.Name.Name] {
				t.Fatalf("%s#%s restores an independent accepted-final repair writer", relative, function.Name.Name)
			}
			if function.Body == nil {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				switch value := node.(type) {
				case *ast.Ident:
					if value.Name == "DispositionRepairs" {
						t.Fatalf("%s#%s restores preflight-issued disposition repairs", relative, function.Name.Name)
					}
				case *ast.CallExpr:
					selector, ok := value.Fun.(*ast.SelectorExpr)
					if ok && selector.Sel.Name == "NewAcceptedFinalDispositionRecordV2" &&
						(relative != "internal/app/turnterminal/coordinator.go" || function.Name.Name != "ensureAcceptedFinalDispositionV1") {
						t.Fatalf("%s#%s signs accepted-final disposition outside the terminal coordinator", relative, function.Name.Name)
					}
				}
				return true
			})
		}
	}
}

func TestTurnTerminalAuthorityStaysOutsideTransitionalServer(t *testing.T) {
	root := runtimeGoRoot(t)
	serverRoot := filepath.Join(root, "internal", "server")
	forbiddenImports := map[string]bool{
		"analytix.local/runtime-go/internal/app/turnterminal":                      true,
		"analytix.local/runtime-go/internal/domain/turnterminal":                   true,
		"analytix.local/runtime-go/internal/ports/turnterminalstore":               true,
		"analytix.local/runtime-go/internal/adapters/outbound/turnterminalstore":   true,
		"analytix.local/runtime-go/internal/app/cachetelemetry":                    true,
		"analytix.local/runtime-go/internal/adapters/outbound/cachetelemetrystore": true,
		"analytix.local/runtime-go/internal/ports/finalauthority":                  true,
		"analytix.local/runtime-go/internal/adapters/outbound/finalauthority":      true,
	}
	for _, file := range goFiles(t, serverRoot) {
		if strings.HasSuffix(file, "_test.go") || hasBuildTag(t, file, "!analytix_prod") {
			continue
		}
		parsed := parseGoFile(t, file, parser.ParseComments)
		for _, imported := range parsed.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err == nil && forbiddenImports[path] {
				t.Fatalf("%s imports case-terminal authority owner %q", rel(t, root, file), path)
			}
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(literal.Value)
			if err == nil && strings.Contains(value, "turn-terminal-authority") {
				t.Fatalf("%s owns the case-terminal authority persistence name", rel(t, root, file))
			}
			return true
		})
	}
}

func TestTrustedFinalProjectionAcceptsOnlyTerminalCompleteAuthority(t *testing.T) {
	root := runtimeGoRoot(t)
	allowedCalls := map[string]map[string]map[string]bool{
		"SeedTerminalComplete": {
			"internal/runtimeapp/app.go": {"newRuntimeServerHandlerWithRootsModeE": true},
		},
		"StageTerminalComplete": {
			"internal/app/evidence/case_boundary_completion.go": {
				"persistPrivateFinalV1": true, "ReconcileCommittedInterrupt": true,
			},
			"internal/app/gateprojection/trusted_final.go": {"RegisterTerminalComplete": true},
		},
	}
	for _, file := range goFiles(t, root) {
		if strings.HasSuffix(file, "_test.go") || hasBuildTag(t, file, "!analytix_prod") {
			continue
		}
		relative := rel(t, root, file)
		parsed := parseGoFile(t, file, 0)
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			switch function.Name.Name {
			case "SeedCommitted", "RegisterCommitted", "StageCommitted":
				t.Fatalf("%s#%s restores a projection API that omits terminal-complete authority", relative, function.Name.Name)
			}
			if function.Body == nil {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				allowedFiles, guarded := allowedCalls[selector.Sel.Name]
				if guarded && !allowedFiles[relative][function.Name.Name] {
					t.Fatalf("%s#%s bypasses terminal-complete projection ownership through %s", relative, function.Name.Name, selector.Sel.Name)
				}
				return true
			})
		}
	}
}
