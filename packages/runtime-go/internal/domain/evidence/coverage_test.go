package evidence

import "testing"

func TestConflictingCoverageCannotOverwriteIncompleteMarker(t *testing.T) {
	merged := MergeConservativeCoverage(
		map[string]any{"complete": false, "paginationComplete": false, "truncated": true, "coverageStatus": "partial"},
		map[string]any{"complete": true, "paginationComplete": true, "truncated": false, "coverageStatus": "complete"},
	)
	if merged["complete"] != false || merged["paginationComplete"] != false || merged["truncated"] != true || merged["coverageStatus"] != "partial" {
		t.Fatalf("later complete assertion erased incomplete coverage: %#v", merged)
	}
	reversed := MergeConservativeCoverage(
		map[string]any{"complete": true, "truncated": false, "coverageStatus": "complete"},
		map[string]any{"complete": false, "truncated": true, "coverageStatus": "partial"},
	)
	if reversed["complete"] != false || reversed["truncated"] != true || reversed["coverageStatus"] != "partial" {
		t.Fatalf("later incomplete assertion was lost: %#v", reversed)
	}
}

func TestCoverageDescriptorRejectsInvalidSafetyMarkerTypes(t *testing.T) {
	for _, descriptor := range []map[string]any{
		{"paginationComplete": "false"},
		{"truncated": 1},
		{"coverageStatus": "maybe"},
		{"nested": map[string]any{"complete": nil}},
	} {
		if ValidateCoverageDescriptor(descriptor) == nil {
			t.Fatalf("invalid coverage descriptor was accepted: %#v", descriptor)
		}
	}
}

func TestCoverageDescriptorRejectsUnknownFields(t *testing.T) {
	if err := ValidateCoverageDescriptor(map[string]any{"coveredAccounts": []any{"a"}}); err == nil {
		t.Fatal("unknown coverage field passed closed descriptor contract")
	}
}
