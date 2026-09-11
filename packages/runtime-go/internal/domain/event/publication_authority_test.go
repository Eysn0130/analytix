package event

import "testing"

func TestContainsAcceptedFinalPublicationAuthorityAtEverySupportedNestingShape(t *testing.T) {
	for _, value := range []any{
		map[string]any{"acceptedFinal": map[string]any{"recordDigest": "forged"}},
		map[string]any{"details": map[string]any{"publicationCommitId": "forged"}},
		map[string]any{"details": []any{map[string]any{"acceptedFinalView": "forged"}}},
		map[string]any{"details": []map[string]any{{"publicationPayloadDigest": "forged"}}},
	} {
		if !ContainsAcceptedFinalPublicationAuthority(value) {
			t.Fatalf("accepted-final authority marker was hidden from detection: %#v", value)
		}
	}
	if ContainsAcceptedFinalPublicationAuthority(map[string]any{
		"kind": "progress", "details": []map[string]any{{"status": "safe"}},
	}) {
		t.Fatal("ordinary event metadata was mistaken for accepted-final authority")
	}
}

func TestContainsPrivateAcceptedFinalAuthorityRejectsFullRecordsAndDetachedFragments(t *testing.T) {
	for _, value := range []any{
		map[string]any{"acceptedFinal": map[string]any{
			"schemaVersion":                  float64(5),
			"factFinalWitnessAdmission":      map[string]any{"schemaVersion": float64(2)},
			"publicationSnapshotProofDigest": "private",
		}},
		map[string]any{"details": map[string]any{"factFinalWitnessAdmission": map[string]any{"schemaVersion": float64(2)}}},
		map[string]any{"details": []any{map[string]any{"publicationSnapshotProofDigest": "private"}}},
		map[string]any{"details": []map[string]any{{"publicationSnapshotProof": map[string]any{"private": true}}}},
		map[string]any{"details": map[string]any{"acceptedFinal": "malformed-private-record"}},
		privateAcceptedFinalV5StructuralFixture(),
	} {
		if !ContainsPrivateAcceptedFinalAuthority(value) {
			t.Fatalf("private accepted-final authority was hidden from detection: %#v", value)
		}
	}
	if ContainsPrivateAcceptedFinalAuthority(map[string]any{
		"kind": "item_completed",
		"item": map[string]any{"acceptedFinalView": map[string]any{"schemaVersion": float64(3)}},
	}) {
		t.Fatal("closed V3 public view was mistaken for private accepted-final authority")
	}
	if ContainsPrivateAcceptedFinalAuthority(map[string]any{
		"kind":            "tool_call_ready",
		"securityContext": map[string]any{"workspaceRealPath": "private"},
	}) {
		t.Fatal("ordinary lifecycle security context was mistaken for accepted-final authority")
	}
}

func TestContainsPrivateAcceptedFinalAuthorityAllowsGenericFieldNamesOutsideCompleteV5(t *testing.T) {
	for _, value := range []any{
		map[string]any{
			"kind": "ordinary_result", "envelope": "mail-envelope", "registryHead": "package-index",
			"publicationIntent": "documentation", "storeDigest": "ordinary-cache-key",
		},
		map[string]any{"details": []any{
			map[string]any{"envelope": map[string]any{"subject": "ordinary"}},
			map[string]any{"registryHead": map[string]any{"branch": "main"}},
			map[string]any{"publicationIntent": map[string]any{"channel": "docs"}},
			map[string]any{"storeDigest": "not-an-accepted-final-authority"},
		}},
	} {
		if ContainsPrivateAcceptedFinalAuthority(value) {
			t.Fatalf("generic ordinary field names were mistaken for private accepted-final authority: %#v", value)
		}
	}
}

func privateAcceptedFinalV5StructuralFixture() map[string]any {
	record := map[string]any{}
	for key := range historicalAcceptedFinalRecordV5Keys {
		record[key] = "private"
	}
	record["schemaVersion"] = float64(5)
	record["factFinalWitnessAdmission"] = map[string]any{"schemaVersion": float64(2)}
	return record
}

func TestContainsGeneralTerminalPublicationAuthorityAtEverySupportedNestingShape(t *testing.T) {
	for _, value := range []any{
		map[string]any{"generalTerminalPublication": nil},
		map[string]any{"details": map[string]any{"generalTerminalCommitId": "forged"}},
		map[string]any{"details": []any{map[string]any{"generalTerminalCASBinding": "forged"}}},
		map[string]any{"details": []map[string]any{{"generalTerminalAuthorityDigest": "forged"}}},
	} {
		if !ContainsGeneralTerminalPublicationAuthority(value) || !ContainsTerminalPublicationAuthority(value) {
			t.Fatalf("general-terminal authority marker was hidden from detection: %#v", value)
		}
	}
	if ContainsGeneralTerminalPublicationAuthority(map[string]any{
		"kind": "turn_completed", "terminalReason": "success",
	}) {
		t.Fatal("legacy bare terminal event was mistaken for general-terminal authority")
	}
}
