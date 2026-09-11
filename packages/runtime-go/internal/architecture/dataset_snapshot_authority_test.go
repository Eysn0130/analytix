package architecture_test

import (
	"go/ast"
	"path/filepath"
	"strings"
	"testing"
)

func TestDatasetSnapshotStoresCannotSelectLocalCurrentAuthority(t *testing.T) {
	root := runtimeGoRoot(t)
	for _, directory := range []string{
		filepath.Join(root, "internal", "ports", "datasetsnapshot"),
		filepath.Join(root, "internal", "adapters", "outbound", "datasetsnapshot"),
	} {
		for _, path := range goFiles(t, directory) {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			parsed := parseGoFile(t, path, 0)
			ast.Inspect(parsed, func(node ast.Node) bool {
				switch typed := node.(type) {
				case *ast.SelectorExpr:
					switch typed.Sel.Name {
					case "ReadDir", "Walk", "WalkDir", "Glob":
						t.Errorf("%s invokes forbidden local dataset authority inventory %s", rel(t, root, path), typed.Sel.Name)
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
								t.Errorf("%s exposes forbidden dataset authority selector %s", rel(t, root, path), name.Name)
							}
						}
					}
				}
				return true
			})
		}
	}
}

func TestDatasetSnapshotChildAdvanceHasSingleProductionCaller(t *testing.T) {
	root := runtimeGoRoot(t)
	allowed := "internal/app/datasetsnapshot/service.go"
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
			if ok && selector.Sel.Name == "AdvanceDatasetSnapshot" && rel(t, root, path) != allowed {
				t.Fatalf("%s bypasses the sole dataset snapshot authority writer", rel(t, root, path))
			}
			return true
		})
	}
}
