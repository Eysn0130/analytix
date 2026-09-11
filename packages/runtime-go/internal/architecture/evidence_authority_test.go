package architecture_test

import (
	"go/ast"
	"path/filepath"
	"strings"
	"testing"
)

// The shared evidence authority may resolve only a content address selected
// by a fresh independent witness. A local inventory/current selector would
// make rollbackable files capable of manufacturing freshness.
func TestEvidenceAuthorityHasNoListOrLocalCurrentSelection(t *testing.T) {
	root := runtimeGoRoot(t)
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
}
