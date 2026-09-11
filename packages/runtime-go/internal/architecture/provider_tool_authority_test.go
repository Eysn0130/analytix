package architecture_test

import (
	"go/ast"
	"path/filepath"
	"strings"
	"testing"
)

// Provider-originated tools have one execution gateway. The loop or a durable
// approval/user-input continuation may select the single-call wrapper, and the
// loop may select the read-only batch wrapper, but none may expose a local/MCP
// executor before the current effect lease and grant revalidation.
func TestProviderToolExecutionHasSingleGrantGatedDispatcher(t *testing.T) {
	root := runtimeGoRoot(t)
	serverRoot := filepath.Join(root, "internal", "server")
	executors := providerPendingExecutors(t, serverRoot)
	if len(executors) == 0 {
		t.Fatal("provider tool executor inventory is empty")
	}

	for _, path := range goFiles(t, serverRoot) {
		if strings.HasSuffix(path, "_test.go") || hasBuildTag(t, path, "!analytix_prod") {
			continue
		}
		relative := rel(t, root, path)
		parsed := parseGoFile(t, path, 0)
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			caller := function.Name.Name
			ast.Inspect(function.Body, func(node ast.Node) bool {
				switch typed := node.(type) {
				case *ast.SelectorExpr:
					assertProviderToolExecutionReference(t, relative, caller, typed.Sel.Name, executors)
				case *ast.CallExpr:
					if identifier, ok := typed.Fun.(*ast.Ident); ok {
						assertProviderToolExecutionReference(t, relative, caller, identifier.Name, executors)
					}
				}
				return true
			})
		}
	}

	assertGrantGateIsFirstStatement(t, filepath.Join(serverRoot, "tools_execution.go"))
}

func providerPendingExecutors(t *testing.T, serverRoot string) map[string]bool {
	t.Helper()
	excluded := map[string]bool{
		"executeRuntimeToolWithEffectAuthority":     true,
		"executeAndSettleRuntimeTool":               true,
		"executeRuntimeAuthorizedReadOnlyToolBatch": true,
	}
	executors := map[string]bool{}
	for _, path := range goFiles(t, serverRoot) {
		if strings.HasSuffix(path, "_test.go") || hasBuildTag(t, path, "!analytix_prod") {
			continue
		}
		parsed := parseGoFile(t, path, 0)
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || excluded[function.Name.Name] || !strings.HasPrefix(function.Name.Name, "execute") ||
				!functionAcceptsRuntimePendingToolCall(function) {
				continue
			}
			executors[function.Name.Name] = true
		}
	}
	return executors
}

func functionAcceptsRuntimePendingToolCall(function *ast.FuncDecl) bool {
	if function == nil || function.Type == nil || function.Type.Params == nil {
		return false
	}
	found := false
	ast.Inspect(function.Type.Params, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok && identifier.Name == "runtimePendingToolCall" {
			found = true
			return false
		}
		return !found
	})
	return found
}

func assertProviderToolExecutionReference(t *testing.T, relative, caller, referenced string, executors map[string]bool) {
	t.Helper()
	switch referenced {
	case "executeAndSettleRuntimeTool":
		allowed := relative == "internal/server/agent_loop.go" && caller == "runtimeToolStepDriver" ||
			relative == "internal/server/gates_routes.go" && caller == "runtimeGateContinuationDependencies" ||
			relative == "internal/server/plan_tools.go" && caller == "materializeRuntimeCreatePlanText"
		if !allowed {
			t.Fatalf("%s#%s binds provider tool wrapper %s outside an admitted execution owner", relative, caller, referenced)
		}
	case "executeRuntimeAuthorizedReadOnlyToolBatch":
		if relative != "internal/server/agent_loop.go" || caller != "runtimeToolStepDriver" {
			t.Fatalf("%s#%s binds provider batch wrapper outside the sole loop driver", relative, caller)
		}
	case "executeRuntimeToolWithEffectAuthority":
		if relative != "internal/server/tools_execution.go" ||
			(caller != "executeAndSettleRuntimeTool" && caller != "executeRuntimeAuthorizedReadOnlyToolBatch") {
			t.Fatalf("%s#%s reaches the provider tool dispatcher outside an effect-authority wrapper", relative, caller)
		}
	default:
		if executors[referenced] &&
			(relative != "internal/server/tools_execution.go" || caller != "executeRuntimeToolWithEffectAuthority") {
			t.Fatalf("%s#%s executes %s outside the current-grant dispatcher", relative, caller, referenced)
		}
	}
}

func assertGrantGateIsFirstStatement(t *testing.T, path string) {
	t.Helper()
	root := runtimeGoRoot(t)
	parsed := parseGoFile(t, path, 0)
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "executeRuntimeToolWithEffectAuthority" {
			continue
		}
		if function.Body == nil || len(function.Body.List) == 0 {
			t.Fatalf("%s provider tool dispatcher has no body", rel(t, root, path))
		}
		gate, ok := function.Body.List[0].(*ast.IfStmt)
		if !ok || gate.Init == nil || len(gate.Body.List) != 1 {
			t.Fatalf("%s provider tool dispatcher does not start with one closed grant gate", rel(t, root, path))
		}
		assignment, ok := gate.Init.(*ast.AssignStmt)
		if !ok || len(assignment.Rhs) != 1 {
			t.Fatalf("%s provider tool dispatcher grant gate is not an exact authorization call", rel(t, root, path))
		}
		call, ok := assignment.Rhs[0].(*ast.CallExpr)
		if !ok {
			t.Fatalf("%s provider tool dispatcher grant gate is not a call", rel(t, root, path))
		}
		selector, selectorOK := call.Fun.(*ast.SelectorExpr)
		if !selectorOK || selector.Sel.Name != "authorizeRuntimePending" {
			t.Fatalf("%s provider tool dispatcher does not revalidate current grant before all effects", rel(t, root, path))
		}
		if _, ok := gate.Body.List[0].(*ast.ReturnStmt); !ok {
			t.Fatalf("%s provider tool dispatcher grant rejection is not fail-closed", rel(t, root, path))
		}
		return
	}
	t.Fatalf("%s is missing executeRuntimeToolWithEffectAuthority", rel(t, root, path))
}
