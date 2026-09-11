package architecture_test

import (
	"go/ast"
	"go/parser"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcceptedFinalEventMutationCallsStayInsideWitnessedPipeline(t *testing.T) {
	root := runtimeGoRoot(t)
	allowed := map[string]map[string]bool{
		"Stage": {
			"internal/adapters/outbound/acceptedfinalevent/store.go": true,
			"internal/runtimeapp/app.go":                             true,
		},
		"WithReservation": {
			"internal/runtimeapp/app.go": true,
		},
		"ActivateAndPublish": {
			"internal/runtimeapp/app.go": true,
		},
	}
	seen := map[string]int{}
	for _, file := range goFiles(t, filepath.Join(root, "internal")) {
		relative := filepath.ToSlash(rel(t, root, file))
		if strings.HasSuffix(file, "_test.go") || strings.HasPrefix(relative, "internal/testsupport/") || hasBuildTag(t, file, "!analytix_prod") {
			continue
		}
		parsed := parseGoFile(t, file, parser.SkipObjectResolution)
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			method := selector.Sel.Name
			owners, guarded := allowed[method]
			if !guarded {
				return true
			}
			if !owners[relative] {
				t.Fatalf("%s calls accepted-final event mutation %s outside the witnessed pipeline", relative, method)
			}
			seen[method]++
			return true
		})
	}
	for method, owners := range allowed {
		if seen[method] != len(owners) {
			t.Fatalf("accepted-final event mutation %s has %d production calls, want %d", method, seen[method], len(owners))
		}
	}
}

func TestAcceptedFinalEventStoreHasNoStageAndPublishShortcut(t *testing.T) {
	root := runtimeGoRoot(t)
	forbidden := map[string]bool{
		"RecordAcceptedFinalEventBundle":                  true,
		"StageAcceptedFinalEventBundle":                   true,
		"StageAcceptedFinalEventBundleContext":            true,
		"PublishAcceptedFinalEventBundle":                 true,
		"WithAcceptedFinalEventReservation":               true,
		"PrepareAcceptedFinalEventDelivery":               true,
		"ActivateAndPublishAcceptedFinalEventDelivery":    true,
		"PublishVerifiedAcceptedFinalEventsWithAuthority": true,
	}
	for _, file := range goFiles(t, filepath.Join(root, "internal")) {
		relative := filepath.ToSlash(rel(t, root, file))
		if strings.HasSuffix(file, "_test.go") || strings.HasPrefix(relative, "internal/testsupport/") || hasBuildTag(t, file, "!analytix_prod") {
			continue
		}
		parsed := parseGoFile(t, file, parser.SkipObjectResolution)
		ast.Inspect(parsed, func(node ast.Node) bool {
			declaration, ok := node.(*ast.FuncDecl)
			if ok && forbidden[declaration.Name.Name] {
				t.Fatalf("%s exposes forbidden accepted-final shortcut %s", relative, declaration.Name.Name)
			}
			return true
		})
	}
}
