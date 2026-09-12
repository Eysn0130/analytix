package architecture_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

const serverOutboundPrefix = "analytix.local/runtime-go/internal/adapters/outbound/"

type serverConstructionOwner struct {
	file, function, adapter, symbol, kind string
}

func serverCompatibilityConstructionOwners() map[serverConstructionOwner]int {
	owners := make(map[serverConstructionOwner]int)
	add := func(file, function, adapter, symbol, kind string, maximum int) {
		owners[serverConstructionOwner{"internal/server/" + file, function, serverOutboundPrefix + adapter, symbol, kind}] = maximum
	}
	// The existing durable store owns one shared mutex and these cooperating
	// persistence adapters. This is a closed compatibility seam, not permission
	// to assemble an unrelated authority elsewhere in server (or twice here).
	for adapter, symbol := range map[string]string{
		"eventlog": "NewStoreWithPreservationV1", "usageindexfs": "New",
		"threadsummaryindexfs": "New", "acceptedfinalevent": "NewStore",
		"checkpointcapture": "NewStore",
	} {
		add("durable_core.go", "newDurableEventSessionStoreWithOriginalUsagePreservationV1", adapter, symbol, "factory", 1)
	}
	add("runtime_components.go", "NewRuntimeServerHandlerFromComponents", "filestore", "OpenConditionalMutationAuthority", "factory", 1)
	// Stateless case-binding observation still has these transitional consumers.
	// A new consumer or a different concrete reader must be composed via a port.
	for file, functions := range map[string][]string{
		"runtime_components.go": {"NewRuntimeServerHandlerFromComponents", "NewRuntimeServerHandlerFromComponents"},
		"tools_execution.go":    {"runtimeServerHandler.authorizeRuntimePending", "runtimeServerHandler.acquireRuntimeToolCallAdmission"},
		"gates_routes.go":       {"runtimeServerHandler.runtimeGateContinuationDependencies"},
		"turn_start.go":         {"runtimeServerHandler.startRuntimeTurn"},
		"thread_routes.go":      {"runtimeServerHandler.runtimeThreadService"},
		"tool_settlement.go":    {"runtimeServerHandler.settleRuntimeToolResultWithEffectAuthority"},
	} {
		for _, function := range functions {
			key := serverConstructionOwner{"internal/server/" + file, function, serverOutboundPrefix + "filestore", "CaseBindingReader", "literal"}
			owners[key]++
		}
	}
	return owners
}

// This deliberately checks construction, not all adapter references: existing
// data inputs, interface/type references and delegated tool calls are separate
// seams. New/Open/Create/Acquire factories and concrete effect types must not
// become a second composition root, including through import or local aliases.
func serverCompositionViolations(file string, source []byte, owners map[serverConstructionOwner]int) ([]string, error) {
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, source, 0)
	if err != nil {
		return nil, err
	}
	imports := make(map[string]string)
	var violations []string
	for _, imported := range parsed.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(path, serverOutboundPrefix) {
			continue
		}
		alias := path[strings.LastIndex(path, "/")+1:]
		if imported.Name != nil {
			alias = imported.Name.Name
		}
		if alias == "." {
			violations = append(violations, file+": outbound dot import hides composition ownership")
		}
		imports[alias] = path
	}
	seen := make(map[serverConstructionOwner]int)
	for _, declaration := range parsed.Decls {
		function := "<package>"
		if fn, ok := declaration.(*ast.FuncDecl); ok {
			function = fn.Name.Name
			if fn.Recv != nil && len(fn.Recv.List) == 1 {
				receiver := fn.Recv.List[0].Type
				if pointer, ok := receiver.(*ast.StarExpr); ok {
					receiver = pointer.X
				}
				if name, ok := receiver.(*ast.Ident); ok {
					function = name.Name + "." + function
				}
			}
		}
		ast.Inspect(declaration, func(node ast.Node) bool {
			var expression ast.Expr
			kind := ""
			switch current := node.(type) {
			case *ast.CallExpr:
				expression, kind = current.Fun, "factory"
				if name, ok := current.Fun.(*ast.Ident); ok && name.Name == "new" && name.Obj == nil && len(current.Args) == 1 {
					expression, kind = current.Args[0], "literal"
				}
			case *ast.CompositeLit:
				expression, kind = current.Type, "literal"
			default:
				return true
			}
			adapter, symbol := serverOutboundSymbol(expression, imports, 0)
			if adapter == "" || (kind == "factory" && !serverFactoryName(symbol)) ||
				(kind == "literal" && !serverEffectTypeName(symbol)) {
				return true
			}
			key := serverConstructionOwner{file, function, adapter, symbol, kind}
			seen[key]++
			if seen[key] > owners[key] {
				violations = append(violations, fmt.Sprintf("%s: %s constructs outbound %s.%s outside its compatibility owner; compose in runtimeapp and inject its port", fset.Position(node.Pos()), function, adapter, symbol))
			}
			return true
		})
	}
	return violations, nil
}

func serverOutboundSymbol(expression ast.Expr, imports map[string]string, depth int) (string, string) {
	if depth > 16 {
		return "", ""
	}
	switch current := expression.(type) {
	case *ast.SelectorExpr:
		if alias, ok := current.X.(*ast.Ident); ok && alias.Obj == nil {
			return imports[alias.Name], current.Sel.Name
		}
	case *ast.ParenExpr:
		return serverOutboundSymbol(current.X, imports, depth+1)
	case *ast.IndexExpr:
		return serverOutboundSymbol(current.X, imports, depth+1)
	case *ast.IndexListExpr:
		return serverOutboundSymbol(current.X, imports, depth+1)
	case *ast.Ident:
		if current.Obj == nil {
			return "", ""
		}
		switch declaration := current.Obj.Decl.(type) {
		case *ast.TypeSpec:
			return serverOutboundSymbol(declaration.Type, imports, depth+1)
		case *ast.ValueSpec:
			for index, name := range declaration.Names {
				if name.Name == current.Name && index < len(declaration.Values) {
					return serverOutboundSymbol(declaration.Values[index], imports, depth+1)
				}
			}
		case *ast.AssignStmt:
			for index, value := range declaration.Lhs {
				if name, ok := value.(*ast.Ident); ok && name.Name == current.Name && index < len(declaration.Rhs) {
					return serverOutboundSymbol(declaration.Rhs[index], imports, depth+1)
				}
			}
		}
	}
	return "", ""
}

func serverFactoryName(name string) bool {
	for _, prefix := range []string{"New", "Open", "Create", "Acquire"} {
		if strings.HasPrefix(name, prefix) && (len(name) == len(prefix) || unicode.IsUpper(rune(name[len(prefix)]))) {
			return prefix == "New" || prefix == "Open" || len(name) == len(prefix) || serverEffectTypeName(strings.TrimPrefix(name, prefix))
		}
	}
	return false
}

func serverEffectTypeName(name string) bool {
	// Version suffixes do not turn an authority into a data-only DTO.
	name = strings.TrimRight(name, "0123456789")
	name = strings.TrimSuffix(name, "V")
	for _, suffix := range []string{"Authority", "Reader", "Store", "Client", "Manager", "Executor", "Lease", "Service", "Transport", "Session", "Driver", "Backend", "Handle", "Registry"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func TestServerCompositionBoundaryRejectsRegrowth(t *testing.T) {
	for _, test := range []struct {
		name, file, source string
		wantViolation      bool
	}{
		{"create authority factory", "routes.go", `import a "analytix.local/runtime-go/internal/adapters/outbound/example"; func handle() { a.CreateAuthorityV1() }`, true},
		{"new factory", "routes.go", `import a "analytix.local/runtime-go/internal/adapters/outbound/example"; func handle() { a.NewStore() }`, true},
		{"renamed final authority import", "active_inherited_history.go", `import renamed "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"; func read() { renamed.NewAcceptedFinalCASReader("root") }`, true},
		{"captured factory invocation", "routes.go", `import fs "analytix.local/runtime-go/internal/adapters/outbound/example"; func handle() { makeStore := fs.NewStore; makeStore() }`, true},
		{"concrete authority", "routes.go", `import a "analytix.local/runtime-go/internal/adapters/outbound/example"; var authority = &a.AuthorityV1{}`, true},
		{"new concrete reader", "routes.go", `import a "analytix.local/runtime-go/internal/adapters/outbound/example"; var reader = new(a.Reader)`, true},
		{"aliased concrete authority", "routes.go", `import a "analytix.local/runtime-go/internal/adapters/outbound/example"; type Hidden = a.Authority; var authority = Hidden{}`, true},
		{"dot import", "routes.go", `import . "analytix.local/runtime-go/internal/adapters/outbound/example"; func handle() { NewStore() }`, true},
		{"approved factory at owner", "durable_core.go", `import renamed "analytix.local/runtime-go/internal/adapters/outbound/eventlog"; func newDurableEventSessionStoreWithOriginalUsagePreservationV1() { renamed.NewStoreWithPreservationV1() }`, false},
		{"approved factory in wrong function", "durable_core.go", `import log "analytix.local/runtime-go/internal/adapters/outbound/eventlog"; func handle() { log.NewStoreWithPreservationV1() }`, true},
		{"approved factory in wrong file", "routes.go", `import log "analytix.local/runtime-go/internal/adapters/outbound/eventlog"; func newDurableEventSessionStoreWithOriginalUsagePreservationV1() { log.NewStoreWithPreservationV1() }`, true},
		{"duplicate approved factory", "durable_core.go", `import log "analytix.local/runtime-go/internal/adapters/outbound/eventlog"; func newDurableEventSessionStoreWithOriginalUsagePreservationV1() { log.NewStoreWithPreservationV1(); log.NewStoreWithPreservationV1() }`, true},
		{"different factory at approved owner", "durable_core.go", `import log "analytix.local/runtime-go/internal/adapters/outbound/eventlog"; func newDurableEventSessionStoreWithOriginalUsagePreservationV1() { log.NewStore() }`, true},
		{"approved observer at owner", "gates_routes.go", `import fs "analytix.local/runtime-go/internal/adapters/outbound/filestore"; type runtimeServerHandler struct{}; func (h *runtimeServerHandler) runtimeGateContinuationDependencies() { _ = fs.CaseBindingReader{} }`, false},
		{"approved observer in wrong owner", "gates_routes.go", `import fs "analytix.local/runtime-go/internal/adapters/outbound/filestore"; type anotherHandler struct{}; func (h *anotherHandler) runtimeGateContinuationDependencies() { _ = fs.CaseBindingReader{} }`, true},
		{"pure references and delegated calls", "routes.go", `import fs "analytix.local/runtime-go/internal/adapters/outbound/filestore"; var reader *fs.CaseBindingReader; var constructor = fs.NewStore; func handle() { fs.ExecuteReadTextTool(fs.ReadTextToolInput{}); fs.CreateCheckpointRescueRecord() }`, false},
		{"pure validation growth", "runtime_handler.go", `func handle(value string) bool { if value == "" { return false }; return len(value) < 256 }`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			violations, err := serverCompositionViolations("internal/server/"+test.file, []byte("package server\n"+test.source), serverCompatibilityConstructionOwners())
			if err != nil {
				t.Fatal(err)
			}
			if (len(violations) > 0) != test.wantViolation {
				t.Fatalf("composition violations = %v, want violation %v", violations, test.wantViolation)
			}
		})
	}
}
