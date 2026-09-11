package security

import (
	"strings"
	"testing"
	"time"
)

func TestCanonicalJSONHashRejectsAmbiguousAuthorityInput(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte(`{"amount":1,"amount":2}`),
		[]byte(`{"amount":1}{"amount":2}`),
		[]byte(`{"text":"\uD800"}`),
		[]byte(`{"amount":1e10001}`),
		append([]byte(`{"text":"`), append([]byte{0xff}, []byte(`"}`)...)...),
	} {
		if hash := CanonicalJSONHash(raw); hash != "" {
			t.Fatalf("ambiguous authority JSON received a hash: raw=%q hash=%s", raw, hash)
		}
	}
	if hash := CanonicalJSONHash([]byte(`{"amount":9007199254740993}`)); len(hash) != 64 || strings.ToLower(hash) != hash {
		t.Fatalf("exact authority JSON did not receive SHA-256: %q", hash)
	}
}

func TestCanonicalJSONHashMatchesFundsHostContextCrossLanguageFixture(t *testing.T) {
	raw := []byte(`{"sql":"SELECT COUNT(*) AS n FROM analysis_txn_detail_idx WHERE amount < 5 AND summary <> '&'","purpose":"verify Go-compatible argument hashing"}`)
	const expected = "245f8452a007e039a44ff6b652494850bf665f389bd989da0ed6371746e29f58"
	if actual := CanonicalJSONHash(raw); actual != expected {
		t.Fatalf("cross-language host argument hash mismatch: got %s want %s", actual, expected)
	}
}

func TestTurnSecurityContextDigestBindsCaseTurnAndSnapshot(t *testing.T) {
	issuedAt := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	base := TurnSecurityContextInput{
		ThreadID:           "thread_1",
		TurnID:             "turn_1",
		WorkspaceRealPath:  "/workspace/case-a",
		TenantID:           "local",
		UserID:             "user_1",
		CaseID:             "case_a",
		CaseBindingHash:    SHA256Hex([]byte("binding-a")),
		DatasetSnapshotID:  "snapshot-a",
		SourceManifestHash: SHA256Hex([]byte("manifest-a")),
		ContextEpoch:       3,
		IssuedAt:           issuedAt,
	}
	left := NewTurnSecurityContext(base)
	right := NewTurnSecurityContext(base)
	if left.ContextDigest == "" || left.ContextDigest != right.ContextDigest {
		t.Fatalf("context digest must be deterministic: %#v %#v", left, right)
	}
	base.CaseID = "case_b"
	if changed := NewTurnSecurityContext(base); changed.ContextDigest == left.ContextDigest {
		t.Fatal("case change must change the context digest")
	}
	base.CaseID = "case_a"
	base.DatasetSnapshotID = "snapshot-b"
	if changed := NewTurnSecurityContext(base); changed.ContextDigest == left.ContextDigest {
		t.Fatal("dataset snapshot change must change the context digest")
	}
}

func TestTurnSecurityContextUsesExplicitLocalAndUnboundSentinels(t *testing.T) {
	context := NewTurnSecurityContext(TurnSecurityContextInput{
		ThreadID: "thread_1", TurnID: "turn_1", WorkspaceRealPath: "/workspace", IssuedAt: time.Now().UTC(),
	})
	if context.TenantID != LocalTenantID || context.UserID != LocalUserID || context.CaseID != UnboundCaseID ||
		context.CaseBindingHash != UnboundCaseBindingHash("/workspace") || context.DatasetSnapshotID != NoDatasetSnapshotID ||
		context.SourceManifestHash != EmptySourceManifestHash {
		t.Fatalf("local/unbound authority must be explicit: %#v", context)
	}
	if err := ValidateTurnSecurityContext(context); err != nil {
		t.Fatalf("normalized local context must validate: %v", err)
	}
}

func TestTurnSecurityContextRejectsNonSHAAuthorityHashes(t *testing.T) {
	context := NewTurnSecurityContext(TurnSecurityContextInput{
		ThreadID: "thread_1", TurnID: "turn_1", WorkspaceRealPath: "/workspace", CaseID: "case_a",
		CaseBindingHash: "not-a-sha", DatasetSnapshotID: "snapshot-a", SourceManifestHash: SHA256Hex([]byte("manifest")), IssuedAt: time.Now().UTC(),
	})
	if err := ValidateTurnSecurityContext(context); err == nil {
		t.Fatal("non-SHA authority hashes must fail closed")
	}
}

func TestUnknownHistoryAuthorityIsCaseSensitiveAndFailsClosed(t *testing.T) {
	caseSensitive, err := ClassifyCaseSensitiveThread(map[string]any{"id": "thread-a", "historyAuthority": "provider-verified"})
	if !caseSensitive || err == nil {
		t.Fatalf("unknown history authority was treated as ordinary: caseSensitive=%t err=%v", caseSensitive, err)
	}
}

func TestCanonicalThreadClassifierRecognizesAllPublicationAuthorityMarkers(t *testing.T) {
	tests := map[string]map[string]any{
		"history authority": {"id": "thread-a", "historyAuthority": "case_boundary_only_v1", "turns": []any{}},
		"case binding":      {"id": "thread-a", "caseBindingHash": SHA256Hex([]byte("binding")), "turns": []any{}},
		"turn projection": {"id": "thread-a", "turns": []any{map[string]any{
			"id": "turn-a", "caseHistoryProjection": "user_only_untrusted_v1", "items": []any{},
		}}},
		"accepted final view": {"id": "thread-a", "turns": []any{map[string]any{
			"id": "turn-a", "items": []any{map[string]any{"acceptedFinalView": map[string]any{"schemaVersion": 1}}},
		}}},
		"publication commit": {"id": "thread-a", "turns": []any{map[string]any{
			"id": "turn-a", "publicationCommitId": SHA256Hex([]byte("commit")), "items": []any{},
		}}},
	}
	for name, thread := range tests {
		t.Run(name, func(t *testing.T) {
			caseSensitive, err := ClassifyCaseSensitiveThread(thread)
			if err != nil || !caseSensitive {
				t.Fatalf("publication authority marker was treated as ordinary: caseSensitive=%t err=%v", caseSensitive, err)
			}
		})
	}
	if caseSensitive, err := ClassifyCaseSensitiveThread(map[string]any{"id": "thread-a", "turns": "malformed"}); !caseSensitive || err == nil {
		t.Fatalf("malformed turn inventory did not fail closed: caseSensitive=%t err=%v", caseSensitive, err)
	}
}

func TestExecutionGrantBindsNegotiatedMCPProtocolVersion(t *testing.T) {
	now := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	context := NewTurnSecurityContext(TurnSecurityContextInput{
		ThreadID: "thread-protocol", TurnID: "turn-protocol", WorkspaceRealPath: "/workspace",
		CaseID: "case-protocol", CaseBindingHash: SHA256Hex([]byte("binding")), DatasetSnapshotID: "snapshot-protocol",
		SourceManifestHash: SHA256Hex([]byte("manifest")), ContextEpoch: 1, IssuedAt: now,
	})
	instance := SHA256Hex([]byte("protocol-runtime-instance"))
	preferred, err := NewVerifiedMCPServerIdentityForProtocol("funds", "analytix_funds", "1.0.0", "2025-11-25", instance, 1)
	if err != nil {
		t.Fatal(err)
	}
	compatible, err := NewVerifiedMCPServerIdentityForProtocol("funds", "analytix_funds", "1.0.0", "2025-06-18", instance, 1)
	if err != nil {
		t.Fatal(err)
	}
	newGrant := func(identity string) ExecutionGrant {
		return NewExecutionGrant(ExecutionGrantInput{
			Context: context, Provider: "provider", ServerIdentity: identity, ToolName: "mcp__funds__query", ToolCallID: securityTestHostToolCallID("protocol"),
			ConnectionEpoch: 1, ArgsHash: SHA256Hex([]byte("args")), SchemaHash: SHA256Hex([]byte("schema")),
			ScopeHash: SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: now, ExpiresAt: now.Add(time.Minute),
		})
	}
	preferredGrant := newGrant(preferred)
	compatibleGrant := newGrant(compatible)
	if err := ValidateExecutionGrant(preferredGrant); err != nil {
		t.Fatal(err)
	}
	if err := ValidateExecutionGrant(compatibleGrant); err != nil {
		t.Fatal(err)
	}
	if preferred == compatible || preferredGrant.GrantID == compatibleGrant.GrantID {
		t.Fatal("negotiated MCP protocol version was not bound into identity and execution grant")
	}
	legacy := "mcpv1:ZnVuZHM:YW5hbHl0aXhfZnVuZHM:MS4wLjA:" + instance + ":1"
	if err := ValidateExecutionGrant(newGrant(legacy)); err == nil {
		t.Fatal("legacy MCP identity authorized a new execution grant")
	}
}

func TestExecutionGrantRequiresHostToolCallIdentityWithoutConstructorRewrite(t *testing.T) {
	now := time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)
	context := mustGeneralTurnSecurityContextV2(t, "thread_raw_call", "turn_raw_call", "/workspace")
	const rawProviderID = "provider_call_6222020202020202020"
	grant := NewExecutionGrant(ExecutionGrantInput{
		Context: context, Provider: "provider", ServerIdentity: "host:builtin", ToolName: "read", ToolCallID: rawProviderID,
		ArgsHash: SHA256Hex([]byte("args")), SchemaHash: SHA256Hex([]byte("schema")), ScopeHash: SHA256Hex([]byte("scope")),
		ReadOnly: true, ApprovalState: "not_required", IssuedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	if grant.ToolCallID != rawProviderID {
		t.Fatalf("constructor silently rewrote invalid identity: %#v", grant)
	}
	if err := ValidateExecutionGrant(grant); err == nil || strings.Contains(err.Error(), rawProviderID) {
		t.Fatalf("raw provider identity must fail closed without echo: %v", err)
	}
	if err := ValidateExecutionGrantForAudit(grant); err != nil {
		t.Fatalf("historical raw provider identity lost audit readability: %v", err)
	}
	if err := ValidateExecutionGrantForContext(grant, context); err == nil || strings.Contains(err.Error(), rawProviderID) {
		t.Fatalf("raw provider identity crossed the execution boundary: %v", err)
	}
}
