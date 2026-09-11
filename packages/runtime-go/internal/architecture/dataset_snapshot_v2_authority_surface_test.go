package architecture_test

import (
	"go/ast"
	"path/filepath"
	"strings"
	"testing"
)

func TestDatasetSnapshotV2StructuralPackagesExposeNoGenericAuthorityMintingSurface(t *testing.T) {
	root := runtimeGoRoot(t)
	targets := []string{
		filepath.Join(root, "internal", "domain", "security", "dataset_snapshot_authority_v2.go"),
		filepath.Join(root, "internal", "domain", "evidence", "raw_artifact_acquisition_v1.go"),
		filepath.Join(root, "internal", "domain", "evidence", "raw_artifact_content_v1.go"),
		filepath.Join(root, "internal", "domain", "evidence", "raw_artifact_manifest_v1.go"),
		filepath.Join(root, "internal", "domain", "evidence", "parsed_generation_v1.go"),
		filepath.Join(root, "internal", "domain", "evidence", "parsed_generation_receipt_v1.go"),
		filepath.Join(root, "internal", "domain", "evidence", "source_row_ledger_v1.go"),
		filepath.Join(root, "internal", "domain", "evidence", "source_row_ledger_index_v1.go"),
		filepath.Join(root, "internal", "domain", "evidence", "source_row_lineage_v1.go"),
		filepath.Join(root, "internal", "domain", "evidence", "dataset_snapshot_manifest_v2.go"),
		filepath.Join(root, "internal", "domain", "evidence", "funds_canonical_csv_admission_v1.go"),
	}
	for _, target := range targets {
		parsed := parseGoFile(t, target, 0)
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || !ast.IsExported(function.Name.Name) {
				continue
			}
			name := function.Name.Name
			if name == "NewDatasetSnapshotAuthorityRecordV2" ||
				name == "DatasetSnapshotAuthorityRecordSigningBytesV2" ||
				name == "NewSourceRowDatasetSnapshotManifestV2" ||
				name == "NewSourceRowRecordV1" ||
				name == "NewSourceRowRecordWithLineageV1" ||
				name == "NewSourceRowLineageV1" ||
				name == "NewSourceRowLedgerRootV1" ||
				name == "ValidateSourceRowLedgerRootWithPagesV1" ||
				strings.HasPrefix(name, "NewRawArtifact") ||
				strings.HasPrefix(name, "BuildRawArtifact") ||
				strings.HasPrefix(name, "AdmitRawArtifact") ||
				strings.HasPrefix(name, "NewParsed") ||
				strings.HasPrefix(name, "BuildParsed") ||
				strings.HasPrefix(name, "AdmitParsed") ||
				strings.HasPrefix(name, "IssueDatasetSnapshotAuthorityRecordV2") ||
				strings.HasPrefix(name, "SignDatasetSnapshotAuthorityRecordV2") {
				t.Fatalf("%s re-exposes pre-admission DSV2 authority surface %s", filepath.Base(target), name)
			}
			if function.Type.Params != nil && fieldListContainsSourceRowPageSliceV1(function.Type.Params) {
				t.Fatalf("%s exports full-page materialization through %s", filepath.Base(target), name)
			}
		}
	}
}

func TestDatasetSnapshotV2PrivateStructuralConstructorsHaveClosedProductionCallSites(t *testing.T) {
	root := runtimeGoRoot(t)
	files, err := filepath.Glob(filepath.Join(root, "internal", "domain", "evidence", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]map[string]bool{
		"newSourceRowRecordV1": {
			"source_row_lineage_v1.go": true,
		},
		"newSourceRowLineageV1": {
			"source_row_lineage_v1.go": true,
		},
		"newRawArtifactContentIndexPageDescriptorV1": {
			"raw_artifact_content_v1.go": true,
		},
		"newRawArtifactManifestPageDescriptorV1": {
			"raw_artifact_manifest_v1.go": true,
		},
		"newParsedPageDescriptorV1": {
			"parsed_generation_receipt_v1.go": true,
		},
		"newParsedPageIndexDescriptorV1": {
			"parsed_generation_receipt_v1.go": true,
		},
	}
	closed := map[string]bool{
		"newSourceRowRecordV1": true, "newSourceRowLineageV1": true,
		"newSourceRowRecordWithLineageV1": true, "newSourceRowDatasetSnapshotManifestV2": true,
		"newRawArtifactAcquisitionIntentV1": true, "newRawArtifactSourceLocatorV1": true,
		"newRawArtifactContentChunkDescriptorV1": true, "newRawArtifactContentIndexPageV1": true,
		"newRawArtifactContentIndexPageDescriptorV1": true, "newRawArtifactContentRootV1": true,
		"newRawArtifactEntryV1": true, "newRawArtifactManifestPageV1": true,
		"newRawArtifactManifestPageDescriptorV1": true, "newRawArtifactManifestV1": true,
		"newParsedGenerationIdentityV1": true, "newParsedPageV1": true,
		"newParsedPageDescriptorV1": true, "newParsedPageIndexV1": true,
		"newParsedPageIndexDescriptorV1": true, "newParsedGenerationReceiptV1": true,
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			identifier, ok := call.Fun.(*ast.Ident)
			if !ok || !closed[identifier.Name] {
				return true
			}
			// The one fixed canonical-CSV composer is the production admission
			// owner for this closed constructor set. It accepts no caller-selected
			// material family and emits only opaque exact CAS objects.
			if filepath.Base(file) == "funds_canonical_csv_admission_v1.go" {
				return true
			}
			if !allowed[identifier.Name][filepath.Base(file)] {
				t.Fatalf("%s calls closed pre-admission constructor %s", filepath.Base(file), identifier.Name)
			}
			return true
		})
	}
}

func TestDatasetSnapshotV2MintableValuesCannotBeReexportedUnderAnotherConstructorName(t *testing.T) {
	root := runtimeGoRoot(t)
	targets := []string{
		filepath.Join(root, "internal", "domain", "security", "dataset_snapshot_authority_v2.go"),
		filepath.Join(root, "internal", "domain", "evidence", "source_row_lineage_v1.go"),
		filepath.Join(root, "internal", "domain", "evidence", "raw_artifact_acquisition_v1.go"),
		filepath.Join(root, "internal", "domain", "evidence", "raw_artifact_content_v1.go"),
		filepath.Join(root, "internal", "domain", "evidence", "raw_artifact_manifest_v1.go"),
		filepath.Join(root, "internal", "domain", "evidence", "parsed_generation_v1.go"),
		filepath.Join(root, "internal", "domain", "evidence", "parsed_generation_receipt_v1.go"),
	}
	allowedParsers := map[string]bool{
		"ParseDatasetSnapshotAuthorityRecordV2":        true,
		"ParseVersionedDatasetSnapshotAuthorityRecord": true,
		"ParseSourceRowLineageV1":                      true,
		"ParseRawArtifactAcquisitionIntentV1":          true,
		"ParseRawArtifactSourceLocatorV1":              true,
		"ParseRawArtifactContentIndexPageV1":           true,
		"ParseRawArtifactContentRootV1":                true,
		"ParseRawArtifactEntryV1":                      true,
		"ParseRawArtifactManifestPageV1":               true,
		"ParseRawArtifactManifestV1":                   true,
		"ParseParsedGenerationIdentityV1":              true,
		"ParseParsedPageV1":                            true,
		"ParseParsedPageIndexV1":                       true,
		"ParseParsedGenerationReceiptV1":               true,
	}
	mintableTypes := map[string]bool{
		"DatasetSnapshotAuthorityRecordV2":        true,
		"VersionedDatasetSnapshotAuthorityRecord": true,
		"SourceRowRecordV1":                       true,
		"SourceRowLineageV1":                      true,
		"RawArtifactAcquisitionIntentV1":          true,
		"RawArtifactSourceLocatorV1":              true,
		"RawArtifactContentChunkDescriptorV1":     true,
		"RawArtifactContentIndexPageV1":           true,
		"RawArtifactContentIndexPageDescriptorV1": true,
		"RawArtifactContentRootV1":                true,
		"RawArtifactEntryV1":                      true,
		"RawArtifactManifestPageV1":               true,
		"RawArtifactManifestPageDescriptorV1":     true,
		"RawArtifactManifestV1":                   true,
		"ParsedGenerationIdentityV1":              true,
		"ParsedPageV1":                            true,
		"ParsedPageDescriptorV1":                  true,
		"ParsedPageIndexV1":                       true,
		"ParsedPageIndexDescriptorV1":             true,
		"ParsedGenerationReceiptV1":               true,
		"ParsedOutcomeV1":                         true,
		"ParsedTypedScalarV1":                     true,
		"ParsedTypedFieldV1":                      true,
	}
	for _, target := range targets {
		parsed := parseGoFile(t, target, 0)
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || !ast.IsExported(function.Name.Name) ||
				function.Type.Results == nil || allowedParsers[function.Name.Name] {
				continue
			}
			ast.Inspect(function.Type.Results, func(node ast.Node) bool {
				identifier, ok := node.(*ast.Ident)
				if ok && mintableTypes[identifier.Name] {
					t.Fatalf("%s exports pre-admission value %s through %s", filepath.Base(target), identifier.Name, function.Name.Name)
				}
				return true
			})
		}
	}
}

func fieldListContainsSourceRowPageSliceV1(fields *ast.FieldList) bool {
	found := false
	ast.Inspect(fields, func(node ast.Node) bool {
		array, ok := node.(*ast.ArrayType)
		if !ok || array.Len != nil {
			return true
		}
		identifier, ok := array.Elt.(*ast.Ident)
		if ok && identifier.Name == "SourceRowLedgerPageV1" {
			found = true
			return false
		}
		return true
	})
	return found
}
