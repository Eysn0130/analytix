package startup

import "testing"

func TestLegacyPrimaryThreadAllowsEventOrderMigrationV1RequiresExactLegacyShape(t *testing.T) {
	legacy := map[string]any{
		"id":    "thr_legacy",
		"turns": []any{map[string]any{"id": "turn_legacy", "items": []any{map[string]any{"kind": "tool_call"}}}},
	}
	if !LegacyPrimaryThreadAllowsEventOrderMigrationV1(legacy, "thr_legacy") {
		t.Fatal("valid legacy primary shape was rejected")
	}

	for name, record := range map[string]map[string]any{
		"missing_id":      {"turns": []any{}},
		"wrong_id":        {"id": "thr_other", "turns": []any{}},
		"missing_turns":   {"id": "thr_legacy"},
		"malformed_turns": {"id": "thr_legacy", "turns": map[string]any{}},
		"malformed_turn":  {"id": "thr_legacy", "turns": []any{"not-an-object"}},
		"malformed_items": {"id": "thr_legacy", "turns": []any{map[string]any{"items": "not-an-array"}}},
		"malformed_item":  {"id": "thr_legacy", "turns": []any{map[string]any{"items": []any{"not-an-object"}}}},
	} {
		t.Run(name, func(t *testing.T) {
			if LegacyPrimaryThreadAllowsEventOrderMigrationV1(record, "thr_legacy") {
				t.Fatal("malformed or mismatched primary shape was accepted")
			}
		})
	}
}

func TestContainsCurrentEventOrderAuthorityV1RecognizesOnlyStructuredExecutionMarkers(t *testing.T) {
	markers := []map[string]any{
		{"acceptedFinal": nil},
		{"nested": map[string]any{"publicationCommitId": "forged"}},
		{"nested": []any{map[string]any{"generalTerminalCommitId": "forged"}}},
		{"kind": "accepted_final_batch"},
		{"purpose": "analytix.general-terminal-delivery-batch/v1"},
		{"securityState": nil},
		{"approvalItemId": nil},
		{"approvalId": "approval-current", "continuationReceiptId": "receipt-current"},
		{"inputId": "input-current", "continuationReceiptId": "receipt-current"},
		{"datasetSnapshotId": nil},
		{"caseId": nil},
		{"caseBindingHash": nil},
	}
	for index, marker := range markers {
		if !ContainsCurrentEventOrderAuthorityV1(marker) {
			t.Fatalf("current authority marker %d was accepted as legacy: %#v", index, marker)
		}
	}
	if ContainsCurrentEventOrderAuthorityV1(map[string]any{
		"kind": "approval_requested", "approvalId": "legacy-handle", "inputId": "legacy-input",
	}) {
		t.Fatal("legacy gate handles were promoted to current execution authority")
	}
	if ContainsCurrentEventOrderAuthorityV1(map[string]any{"message": "acceptedFinal is ordinary user text"}) {
		t.Fatal("ordinary text was classified as authority")
	}
	if ContainsCurrentEventOrderAuthorityV1(map[string]any{
		"kind": "tool_progress",
		"details": map[string]any{
			"caseId": "case-untrusted", "workspaceRealPath": "/tmp/sk-untrusted", "approvalId": "approval-untrusted",
			"kind": "general_terminal_batch",
		},
	}) {
		t.Fatal("arbitrary nested content promoted same-name fields to host authority")
	}
}

func TestCurrentEventOrderAuthorityKeyV1CoversFrozenApprovalAuthority(t *testing.T) {
	for _, key := range []string{
		"contextDigest", "contextEpoch", "executionGrant", "executionGrantId", "parentGrantId",
		"hostEvidenceSettlement", "approvalTransition", "approvalTransitionId", "approvalItemId",
		"continuationDispositionId", "continuationReceiptId", "approvalId", "inputId",
		"workspaceRealPath", "tenantId", "userId", "caseId", "caseBindingHash", "datasetSnapshotId", "sourceManifestHash",
	} {
		if !IsCurrentEventOrderAuthorityKeyV1(key) {
			t.Fatalf("frozen authority key %q is absent from the canonical startup predicate", key)
		}
	}
	for _, key := range []string{"message", "timestamp", "issuedAt"} {
		if IsCurrentEventOrderAuthorityKeyV1(key) {
			t.Fatalf("legacy operational key %q was promoted to current authority", key)
		}
	}
}

func TestFrozenEventOrderAuthorityStableV1PreservesValuesAndTerminalContainers(t *testing.T) {
	before := map[string]any{
		"securityState": map[string]any{"workspaceRealPath": "/cases/sk-abcdefghijk", "contextDigest": "digest"},
		"title":         "Authorization: Bearer ordinary-secret",
	}
	after := map[string]any{
		"securityState": map[string]any{"workspaceRealPath": "/cases/sk-abcdefghijk", "contextDigest": "digest"},
		"title":         "<redacted>",
	}
	if !FrozenEventOrderAuthorityStableV1(before, after) {
		t.Fatal("ordinary content projection was rejected despite exact frozen authority")
	}
	after["securityState"].(map[string]any)["workspaceRealPath"] = "/cases/<redacted>"
	if FrozenEventOrderAuthorityStableV1(before, after) {
		t.Fatal("mutated frozen workspace authority was accepted")
	}

	terminalBefore := map[string]any{"acceptedFinal": nil, "text": "signed text"}
	terminalAfter := map[string]any{"acceptedFinal": nil, "text": "<redacted>"}
	if FrozenEventOrderAuthorityStableV1(terminalBefore, terminalAfter) {
		t.Fatal("mutated terminal publication container was accepted")
	}

	hostileBefore := map[string]any{
		"kind": "tool_progress", "details": map[string]any{"caseId": "sk-hostile", "workspaceRealPath": "/tmp/sk-hostile"},
	}
	hostileAfter := map[string]any{
		"kind": "tool_progress", "details": map[string]any{"caseId": "<redacted>", "workspaceRealPath": "/tmp/<redacted>"},
	}
	if !FrozenEventOrderAuthorityStableV1(hostileBefore, hostileAfter) {
		t.Fatal("ordinary nested same-name fields were promoted to frozen host authority")
	}
}

func TestIsEventOrderTransactionResidueV1RejectsKnownAndUnknownBundleFiles(t *testing.T) {
	for _, name := range []string{
		".events-bundle-v1.tmp",
		".events-bundle-journal-v1.tmp",
		".events-bundle-journal-v1.json",
		".events-bundle-future-v9.candidate",
	} {
		if !IsEventOrderTransactionResidueV1(name) {
			t.Fatalf("event bundle residue %q was not recognized", name)
		}
	}
	if IsEventOrderTransactionResidueV1("events.jsonl") {
		t.Fatal("canonical event log was classified as transaction residue")
	}
}
