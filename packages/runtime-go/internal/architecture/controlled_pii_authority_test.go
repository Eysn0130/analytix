package architecture_test

import (
	"go/ast"
	"strings"
	"testing"
)

func TestExactPIITypesHaveOnlyPrivateProductionOwners(t *testing.T) {
	root := runtimeGoRoot(t)
	allowed := map[string]bool{
		"internal/domain/evidence/claim.go":                         true,
		"internal/domain/evidence/source_field_binding_v2.go":       true,
		"internal/domain/piiauthorization/controlled_artifact.go":   true,
		"internal/app/piiauthorization/evidence_authority.go":       true,
		"internal/app/piiauthorization/source_exact_renderer_v2.go": true,
	}
	for _, path := range goFiles(t, root) {
		if strings.HasSuffix(path, "_test.go") || hasBuildTag(t, path, "!analytix_prod") {
			continue
		}
		relative := rel(t, root, path)
		parsed := parseGoFile(t, path, 0)
		ast.Inspect(parsed, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok || identifier.Name != "SourceFieldBindingV2" && identifier.Name != "ControlledPIIFieldV1" {
				return true
			}
			if !allowed[relative] {
				t.Fatalf("%s exposes exact PII type %s outside its private evidence/render owner", relative, identifier.Name)
			}
			return true
		})
	}
}

func TestControlledPIIRenderOperationHasSingleProductionImplementation(t *testing.T) {
	root := runtimeGoRoot(t)
	implementations := []string{}
	for _, path := range goFiles(t, root) {
		if strings.HasSuffix(path, "_test.go") || hasBuildTag(t, path, "!analytix_prod") {
			continue
		}
		parsed := parseGoFile(t, path, 0)
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Recv != nil && function.Name.Name == "RenderCurrentControlledPIIArtifactV2" {
				implementations = append(implementations, rel(t, root, path))
			}
		}
	}
	if len(implementations) != 1 || implementations[0] != "internal/app/piiauthorization/evidence_authority.go" {
		t.Fatalf("controlled PII render authority implementation is not unique: %v", implementations)
	}
}
