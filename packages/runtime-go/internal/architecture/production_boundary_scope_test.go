package architecture_test

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestProductionDependencyGuardsExcludeOnlyGoTestFixtures(t *testing.T) {
	for _, name := range []string{"fixture_test.go", "fixture_linux_test.go", "fixture_darwin_test.go"} {
		if productionBoundarySourceV1(name) {
			t.Fatalf("Go test fixture entered production graph: %s", name)
		}
	}
	for _, name := range []string{"service.go", "service_linux.go", "service_darwin.go", "test_support.go"} {
		if !productionBoundarySourceV1(name) {
			t.Fatalf("production source escaped graph: %s", name)
		}
	}
	for _, source := range []string{
		`package app; import "os"`,
		`package app; import local "path/filepath"`,
		`package app; import adapter "analytix.local/runtime-go/internal/adapters/outbound/filestore"`,
	} {
		parsed, err := parser.ParseFile(token.NewFileSet(), "service.go", source, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		if len(forbiddenLayerImportsV1(parsed, []string{"os", "path/filepath", "analytix.local/runtime-go/internal/adapters"})) != 1 {
			t.Fatal("production dependency mutation escaped guard", source)
		}
	}
}
