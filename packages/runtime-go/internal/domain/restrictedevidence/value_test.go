package restrictedevidence

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateRejectsRestrictedPurposeBelowNeutralWrappers(t *testing.T) {
	tests := []any{
		map[string]any{"review": map[string]any{"output": map[string]any{
			"schemaVersion": 1, "purpose": "analytix.raw-artifact-manifest/v1",
		}}},
		map[string]any{"payload": []any{map[string]any{
			"Schema_Version": 1, "PURPOSE": " ANALYTIX.PARSED-GENERATION-RECEIPT/V1 ",
		}}},
		map[string]any{"value": map[string]string{
			"purpose": "analytix.source-row-lineage/v1",
		}},
		`{"neutral":{"purpose":"analytix.dataset-snapshot-manifest/v2"}}`,
		[]byte(`[{"purpose":"analytix.dataset-snapshot-authority/v2"}]`),
		map[string]any{"payload": map[string]any{
			"schemaVersion": 2, "purpose": "analytix.source-field-binding/v2", "sourceExactValue": "0012-3456",
		}},
		map[string]any{"payload": map[string]any{
			"schemaVersion": 2, "purpose": "analytix.canonical-evidence/v2", "sourceFieldBindings": []any{},
		}},
	}
	for _, value := range tests {
		if err := Validate(value); !errors.Is(err, ErrRestrictedEvidence) {
			t.Fatalf("restricted evidence was accepted: value=%#v err=%v", value, err)
		}
	}
}

func TestValidateRejectsTypedLocalResponseIdentitiesWithNeutralCanaries(t *testing.T) {
	tests := []struct {
		kind   string
		canary string
	}{
		{kind: "import_mapping_preview", canary: "NEUTRAL_GO_CANARY_IMPORT"},
		{kind: "cleaning_diff_preview", canary: "NEUTRAL_GO_CANARY_CLEANING"},
		{kind: "direct_source_preview", canary: "NEUTRAL_GO_CANARY_DIRECT"},
		{kind: "accepted_slot_display", canary: "NEUTRAL_GO_CANARY_ACCEPTED"},
	}
	for _, test := range tests {
		if err := Validate(test.canary); err != nil {
			t.Fatalf("neutral typed-local canary matched an unrelated detector: kind=%s err=%v", test.kind, err)
		}
		response := map[string]any{
			"schemaVersion": 1,
			"kind":          test.kind,
			"nested":        map[string]any{"displayValue": test.canary},
		}
		for _, value := range []any{
			response,
			map[string]any{"wrapper": map[string]any{"response": response}},
			`{"schemaVersion":1,"kind":"` + test.kind + `","nested":{"displayValue":"` + test.canary + `"}}`,
		} {
			if err := Validate(value); !errors.Is(err, ErrRestrictedEvidence) {
				t.Fatalf("typed-local response crossed generic validation: kind=%s value=%#v err=%v", test.kind, value, err)
			}
		}
	}
}

func TestValidateAllowsTypedLocalFamilyProseAndIncompleteIdentity(t *testing.T) {
	for _, value := range []any{
		"Please discuss typed-local-data-surface/v1 and direct_source_preview naming.",
		map[string]any{"schemaVersion": 1, "message": "ordinary"},
		map[string]any{"kind": "direct_source_preview", "message": "ordinary"},
		map[string]any{"schemaVersion": "1", "kind": "direct_source_preview", "message": "ordinary"},
		map[string]any{"schemaVersion": 2, "kind": "direct_source_preview", "message": "ordinary"},
		map[string]any{"schemaVersion": 1, "kind": "unknown_preview", "message": "ordinary"},
	} {
		if err := Validate(value); err != nil {
			t.Fatalf("ordinary typed-local discussion was over-blocked: value=%#v err=%v", value, err)
		}
	}
}

func TestValidateRejectsPurposeStrippedExactReferences(t *testing.T) {
	digest := strings.Repeat("a", 64)
	sha256 := strings.Repeat("b", 64)
	other := strings.Repeat("c", 64)
	tests := []map[string]any{
		{
			"Raw_Artifact-Manifest Digest":      digest,
			"rawArtifactManifestSHA256":         sha256,
			"RAW ARTIFACT MANIFEST BYTE LENGTH": 42,
		},
		{
			"parsedGenerationReceiptDigest":     digest,
			"parsedGenerationReceiptSha256":     sha256,
			"parsedGenerationReceiptByteLength": 42,
		},
		{
			"classificationLedgerDigest":     digest,
			"classificationLedgerSha256":     sha256,
			"classificationLedgerByteLength": 42,
		},
		{
			"sourceRowLedgerRootDigest":     digest,
			"sourceRowLedgerRootSha256":     sha256,
			"sourceRowLedgerRootByteLength": 42,
		},
		{
			"lineageDigest": digest, "lineageSha256": sha256, "lineageByteLength": 42,
		},
		{
			"sourceExactValue": "0012-3456", "sourceExactValueSha256": sha256, "bindingDigest": digest,
		},
		{
			"rawArtifactSha256": digest, "sourceRecordSha256": sha256, "sourceExactValueSha256": other,
		},
		{
			"facts":                       []any{map[string]any{"factId": "fact-a"}},
			"sourceFieldBindings":         []any{map[string]any{"bindingDigest": digest}},
			"sourceFieldBindingSetDigest": other,
		},
	}
	for _, value := range tests {
		if err := Validate(map[string]any{"neutral": value}); !errors.Is(err, ErrRestrictedEvidence) {
			t.Fatalf("purpose-stripped reference was accepted: value=%#v err=%v", value, err)
		}
	}
}

func TestValidateAllowsPublicHashesAndProseMentions(t *testing.T) {
	hash := strings.Repeat("a", 64)
	tests := []any{
		map[string]any{"datasetSnapshotId": "snapshot-v2", "sourceManifestHash": hash},
		map[string]any{"reportSha256": hash, "claimLedgerHash": hash},
		map[string]any{"rawArtifactManifestDigest": hash},
		"请解释 analytix.raw-artifact-manifest/v1 与 lineageDigest 的含义",
		`prefix {"purpose":"analytix.raw-artifact-manifest/v1"}`,
		`{"purpose":"analytix.public-report/v1","reportSha256":"` + hash + `"}`,
		map[string]any{"purpose": "analytix.source-row-producer-policy/v1", "policyDigest": hash},
		map[string]any{"purpose": "analytix.dataset-snapshot-id/v1", "datasetSnapshotId": "snapshot-v2"},
		map[string]any{
			"rawArtifactManifestDigest": "digest", "rawArtifactManifestSha256": "sha", "rawArtifactManifestByteLength": 42,
		},
	}
	for _, value := range tests {
		if err := Validate(value); err != nil {
			t.Fatalf("ordinary public value was rejected: value=%#v err=%v", value, err)
		}
	}
}

func TestValidateFailsClosedOnSerializedJSONInspectionLimits(t *testing.T) {
	deep := strings.Repeat("[", maxInspectionDepth+1) + strings.Repeat("]", maxInspectionDepth+1)
	large := `{"value":"` + strings.Repeat("x", maxSerializedJSONBytes) + `"}`
	duplicate := `{"purpose":"analytix.raw-artifact-manifest/v1","purpose":"analytix.public/v1"}`
	for _, value := range []string{deep, large, duplicate} {
		if err := Validate(value); !errors.Is(err, ErrInspectionLimit) {
			t.Fatalf("inspection limit did not fail closed: err=%v", err)
		}
	}
}

func TestValidateRejectsNormalizedPurposeCollisionDeterministically(t *testing.T) {
	value := map[string]any{
		"purpose": "analytix.public-report/v1",
		"PURPOSE": "analytix.source-field-binding/v2",
	}
	for attempt := 0; attempt < 100; attempt++ {
		if err := Validate(value); !errors.Is(err, ErrRestrictedEvidence) {
			t.Fatalf("normalized purpose collision bypassed inspection on attempt %d: %v", attempt, err)
		}
	}
}
