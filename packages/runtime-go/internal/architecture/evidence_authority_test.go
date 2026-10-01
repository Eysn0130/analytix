package architecture_test

import (
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// The shared evidence authority may resolve only a content address selected
// by a fresh independent witness. A local inventory/current selector would
// make rollbackable files capable of manufacturing freshness.
func TestEvidenceAuthorityHasNoListOrLocalCurrentSelection(t *testing.T) {
	root := runtimeGoRoot(t)
	hostSelectors := 0
	for _, directory := range []string{
		filepath.Join(root, "internal", "app", "evidenceauthority"),
		filepath.Join(root, "internal", "ports", "evidenceauthority"),
	} {
		for _, path := range goFiles(t, directory) {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			parsed := parseGoFile(t, path, 0)
			ast.Inspect(parsed, func(node ast.Node) bool {
				switch typed := node.(type) {
				case *ast.SelectorExpr:
					if typed.Sel.Name == "List" {
						t.Errorf("%s invokes forbidden local List selection", rel(t, root, path))
					}
				case *ast.TypeSpec:
					if interfaceType, ok := typed.Type.(*ast.InterfaceType); ok {
						for _, method := range interfaceType.Methods.List {
							for _, name := range method.Names {
								if name.Name == "Current" && typed.Name.Name == "HostLocalHeadCoordinator" && rel(t, root, path) == "internal/ports/evidenceauthority/store.go" {
									expected, err := parser.ParseExpr("func(context.Context) (domainhost.HeadV1, bool, error)")
									if err != nil || normalizedAuthorityTypeV1(method.Type) == "" || normalizedAuthorityTypeV1(method.Type) != normalizedAuthorityTypeV1(expected) {
										t.Fatal("host-local selector lost its closed HeadV1 result")
									}
									hostSelectors++
									continue
								}
								if name.Name == "List" || name.Name == "Current" {
									t.Errorf("%s exposes forbidden %s authority selector", rel(t, root, path), name.Name)
								}
							}
						}
					}
				}
				return true
			})
		}
	}
	if hostSelectors != 1 {
		t.Fatalf("host-local selector count = %d, want one closed mode-specific port", hostSelectors)
	}
}

func normalizedAuthorityTypeV1(node ast.Node) string {
	var body strings.Builder
	if err := format.Node(&body, token.NewFileSet(), node); err != nil {
		return ""
	}
	return body.String()
}
