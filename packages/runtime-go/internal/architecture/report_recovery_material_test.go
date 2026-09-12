package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// The owner now lends visitors inside a revalidated observation, rather than
// asking the report adapter to reopen the prepared owner itself. Inspect the
// actual artifact call and callback type, not the retired API's source text.
func reportArtifactRecoveryUsesMetadata(function *ast.FuncDecl) bool {
	if function == nil || function.Type.Params == nil || function.Body == nil {
		return false
	}
	bound := false
	for _, field := range function.Type.Params.List {
		if len(field.Names) == 1 && field.Names[0].Name == "visitMaterials" &&
			privateCASSelectorMatches(field.Type, "finalauthorityadapter", "PrivateCASDomainMaterialVisitor") {
			bound = true
		}
	}
	artifactCalls, validCalls := reportArtifactMaterialVisits(function.Body)
	return bound && artifactCalls == 1 && validCalls == 1
}

func reportArtifactMaterialVisits(node ast.Node) (artifactCalls, validCalls int) {
	ast.Inspect(node, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		for _, argument := range call.Args {
			literal, ok := argument.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				continue
			}
			value, err := strconv.Unquote(literal.Value)
			if err != nil || value != "artifacts" {
				continue
			}
			artifactCalls++
			if privateCASIdent(call.Fun) != "visitMaterials" || len(call.Args) != 2 || call.Args[0] != argument {
				continue
			}
			callback, ok := call.Args[1].(*ast.FuncLit)
			if !ok || callback.Type.Params == nil || len(callback.Type.Params.List) != 1 {
				continue
			}
			if privateCASSelectorMatches(callback.Type.Params.List[0].Type, "finalauthorityadapter", "SecurePrivateCASPreparedMaterialV1") {
				validCalls++
			}
		}
		return true
	})
	return artifactCalls, validCalls
}

func TestReportArtifactMaterialGuardRejectsBodyReadersAndMissingVisits(t *testing.T) {
	metadata := `visitMaterials("artifacts", func(material finalauthorityadapter.SecurePrivateCASPreparedMaterialV1) error { return nil })`
	for index, body := range []string{
		metadata,
		`// visitMaterials("artifacts", callback)`,
		`visit("artifacts", func(file finalauthorityadapter.SecurePrivateCASFile) error { return nil })`,
		`visitMaterials("artifacts", func(file finalauthorityadapter.SecurePrivateCASFile) error { return nil })`,
		`prepared.VisitCommittedFiles(ctx, "artifacts", callback)`,
		metadata + "; " + metadata,
		metadata + `; prepared.VisitCommittedFiles(ctx, "artifacts", callback)`,
	} {
		source := "package fixture; func validate(visitMaterials finalauthorityadapter.PrivateCASDomainMaterialVisitor) {\n" + body + "\n}"
		parsed, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
		if err != nil {
			t.Fatal(err)
		}
		function := parsed.Decls[0].(*ast.FuncDecl)
		if reportArtifactRecoveryUsesMetadata(function) != (index == 0) {
			t.Fatalf("artifact guard misclassified fixture %d", index)
		}
		if index == 0 {
			function.Type.Params.List = nil
			if reportArtifactRecoveryUsesMetadata(function) {
				t.Fatal("artifact guard accepted an unbound material visitor")
			}
		}
	}
}
