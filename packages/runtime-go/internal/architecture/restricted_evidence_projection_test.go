package architecture_test

import (
	"encoding/json"
	"go/ast"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	rawartifactapp "analytix.local/runtime-go/internal/app/rawartifact"
	domainrestrictedevidence "analytix.local/runtime-go/internal/domain/restrictedevidence"
)

func TestRestrictedEvidenceTypesStayOutsideInboundPublicAndRuntimeProjectionLayers(t *testing.T) {
	root := runtimeGoRoot(t)
	var files []string
	err := filepath.Walk(filepath.Join(root, "internal"), func(path string, info fs.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	allowedFragments := []string{
		filepath.Join("internal", "domain", "evidence") + string(filepath.Separator),
		filepath.Join("internal", "domain", "security") + string(filepath.Separator),
		filepath.Join("internal", "app", "rawartifact") + string(filepath.Separator),
		filepath.Join("internal", "app", "datasetsnapshot") + string(filepath.Separator),
		filepath.Join("internal", "app", "fundsquerysource") + string(filepath.Separator),
		filepath.Join("internal", "app", "piiauthorization") + string(filepath.Separator),
		filepath.Join("internal", "ports", "rawartifact") + string(filepath.Separator),
		filepath.Join("internal", "ports", "datasetsnapshot") + string(filepath.Separator),
		filepath.Join("internal", "adapters", "outbound", "rawartifact") + string(filepath.Separator),
		filepath.Join("internal", "adapters", "outbound", "datasetsnapshot") + string(filepath.Separator),
		filepath.Join("internal", "testsupport", "rawartifactfixture") + string(filepath.Separator),
		filepath.Join("internal", "testsupport", "datasetsnapshotv2fixture") + string(filepath.Separator),
		filepath.Join("internal", "architecture") + string(filepath.Separator),
	}
	for _, file := range files {
		allowed := false
		for _, fragment := range allowedFragments {
			if strings.Contains(file, fragment) {
				allowed = true
				break
			}
		}
		if allowed {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		ast.Inspect(parsed, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok && identifier.Name == "SourceFieldBindingV2" && strings.HasSuffix(file, "_test.go") {
				return true
			}
			if ok && restrictedEvidenceTypeNameV1(identifier.Name) {
				t.Fatalf("restricted evidence type %s escaped into %s", identifier.Name, file)
			}
			return true
		})
	}
}

func TestFundsQuerySourceRestrictedEvidenceStaysPackagePrivate(t *testing.T) {
	root := runtimeGoRoot(t)
	directory := filepath.Join(root, "internal", "app", "fundsquerysource")
	inspectType := func(file string, declaration string, node ast.Node) {
		ast.Inspect(node, func(candidate ast.Node) bool {
			identifier, ok := candidate.(*ast.Ident)
			if ok && restrictedEvidenceTypeNameV1(identifier.Name) {
				t.Fatalf("exported funds-query source declaration %s in %s exposes restricted evidence type %s", declaration, rel(t, root, file), identifier.Name)
			}
			return true
		})
	}
	for _, file := range goFiles(t, directory) {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		for _, declaration := range parsed.Decls {
			switch current := declaration.(type) {
			case *ast.FuncDecl:
				if current.Name.IsExported() {
					inspectType(file, current.Name.Name, current.Type)
				}
			case *ast.GenDecl:
				for _, specification := range current.Specs {
					typeSpec, ok := specification.(*ast.TypeSpec)
					if !ok {
						continue
					}
					structType, ok := typeSpec.Type.(*ast.StructType)
					if !ok {
						continue
					}
					for _, field := range structType.Fields.List {
						for _, name := range field.Names {
							if name.IsExported() {
								inspectType(file, typeSpec.Name.Name+"."+name.Name, field.Type)
							}
						}
					}
				}
			}
		}
	}
}

func TestRawArtifactStagedSummaryHasNoSerializableOrExportedDataFields(t *testing.T) {
	typ := reflect.TypeOf(rawartifactapp.StagedHierarchyV1{})
	for index := 0; index < typ.NumField(); index++ {
		if typ.Field(index).IsExported() {
			t.Fatalf("staged hierarchy exposes restricted field %s", typ.Field(index).Name)
		}
	}
	body, err := json.Marshal(rawartifactapp.StagedHierarchyV1{})
	if err != nil || string(body) != "{}" {
		t.Fatalf("staged hierarchy became a public JSON projection: body=%s err=%v", body, err)
	}
	if (rawartifactapp.StagedHierarchyV1{}).CanAuthorizeFacts() {
		t.Fatal("staged hierarchy acquired factual authority")
	}
}

func TestUntrustedRawArtifactCandidateVerifierHasNoProductionCaller(t *testing.T) {
	root := runtimeGoRoot(t)
	importPath := "analytix.local/runtime-go/internal/app/rawartifact"
	for _, file := range goFiles(t, root) {
		if strings.HasSuffix(file, "_test.go") || hasBuildTag(t, file, "!analytix_prod") {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		aliases := importedAliases(parsed, importPath)
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "VerifyAndStage" {
				return true
			}
			identifier, ok := selector.X.(*ast.Ident)
			if ok && aliases[identifier.Name] {
				t.Fatalf("untrusted raw artifact candidate verifier escaped into production caller %s", rel(t, root, file))
			}
			return true
		})
	}
}

func TestBoundedRawArtifactCASOracleIsExcludedFromProduction(t *testing.T) {
	root := runtimeGoRoot(t)
	file := filepath.Join(root, "internal", "adapters", "outbound", "rawartifact", "staging_store.go")
	if !hasBuildTag(t, file, "!analytix_prod") {
		t.Fatal("capacity-incompatible raw artifact CAS oracle entered analytix_prod")
	}
}

func TestRestrictedEvidencePurposeSetTracksPrivateContractPurposes(t *testing.T) {
	root := runtimeGoRoot(t)
	for _, directory := range []string{
		filepath.Join(root, "internal", "domain", "evidence"),
		filepath.Join(root, "internal", "domain", "security"),
	} {
		for _, file := range goFiles(t, directory) {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			parsed := parseGoFile(t, file, 0)
			for _, declaration := range parsed.Decls {
				generic, ok := declaration.(*ast.GenDecl)
				if !ok || generic.Tok != token.CONST {
					continue
				}
				for _, specification := range generic.Specs {
					values, ok := specification.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for index, name := range values.Names {
						if !strings.Contains(name.Name, "Purpose") || index >= len(values.Values) {
							continue
						}
						literal, ok := values.Values[index].(*ast.BasicLit)
						if !ok || literal.Kind != token.STRING {
							continue
						}
						purpose, err := strconv.Unquote(literal.Value)
						if err != nil || !privateEvidencePurposeContractV1(purpose) {
							continue
						}
						if !domainrestrictedevidence.Contains(map[string]any{"purpose": purpose}) {
							t.Fatalf("private contract purpose %s in %s is absent from the ordinary projection gate", purpose, rel(t, root, file))
						}
					}
				}
			}
		}
	}
	if domainrestrictedevidence.Contains(map[string]any{"purpose": "analytix.source-row-producer-policy/v1"}) {
		t.Fatal("fixed source-row producer policy was over-classified as a private evidence instance")
	}
}

func privateEvidencePurposeContractV1(purpose string) bool {
	switch purpose {
	case "analytix.source-row-producer-policy/v1":
		return false
	case "analytix.dataset-snapshot-authority/v1", "analytix.dataset-snapshot-authority/v2",
		"analytix.dataset-snapshot-index/v1", "analytix.dataset-snapshot-manifest/v2",
		"analytix.source-field-binding/v2", "analytix.canonical-evidence/v2":
		return true
	}
	return strings.HasPrefix(purpose, "analytix.raw-artifact-") && strings.HasSuffix(purpose, "/v1") ||
		strings.HasPrefix(purpose, "analytix.parsed-") && strings.HasSuffix(purpose, "/v1") ||
		strings.HasPrefix(purpose, "analytix.source-row-") && strings.HasSuffix(purpose, "/v1")
}

func restrictedEvidenceTypeNameV1(name string) bool {
	return name == "DatasetSnapshotManifestV2" || name == "DatasetSnapshotManifestInputV2" ||
		name == "DatasetSnapshotAuthorityRecordV2" || name == "VersionedDatasetSnapshotAuthorityRecord" ||
		name == "ExactObjectReferenceV1" || name == "StagedHierarchyV1" ||
		name == "SourceFieldBindingV2" ||
		strings.HasPrefix(name, "ParsedGeneration") && strings.HasSuffix(name, "V1") ||
		strings.HasPrefix(name, "ParsedPage") && strings.HasSuffix(name, "V1") ||
		strings.HasPrefix(name, "ParsedOutcome") && strings.HasSuffix(name, "V1") ||
		strings.HasPrefix(name, "ParsedTyped") && strings.HasSuffix(name, "V1") ||
		strings.HasPrefix(name, "RawArtifact") && strings.HasSuffix(name, "V1") ||
		strings.HasPrefix(name, "SourceRowLineage") && strings.HasSuffix(name, "V1") ||
		strings.HasPrefix(name, "SourceRowLocator") && strings.HasSuffix(name, "V1") ||
		strings.HasPrefix(name, "SourceRowRecord") && strings.HasSuffix(name, "V1") ||
		strings.HasPrefix(name, "SourceRowWitness") && strings.HasSuffix(name, "V1") ||
		strings.HasPrefix(name, "SourceRowLedger") && strings.HasSuffix(name, "V1")
}
