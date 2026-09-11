package job

import (
	"crypto/sha256"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestJobSecurityBindingRejectsCrossCaseAndTampering(t *testing.T) {
	now := time.Date(2026, 7, 10, 16, 0, 0, 0, time.UTC)
	contextA := jobCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread_1", TurnID: "turn_a", WorkspaceRealPath: "/workspace", CaseID: "case_a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-a")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot_a"), ContextEpoch: 1, IssuedAt: now,
	})
	callID := jobTestHostToolCallID("case-a")
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: contextA, Provider: "provider_1", ServerIdentity: "host:builtin", ToolName: "task", ToolCallID: callID,
		ArgsHash: domainsecurity.SHA256Hex([]byte("args")), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	binding, err := NewSecurityBinding(contextA, grant, callID)
	if err != nil || ValidateSecurityBinding(binding) != nil || !SecurityBindingMatchesContext(binding, contextA) {
		t.Fatalf("valid job security binding rejected: binding=%#v err=%v", binding, err)
	}
	contextB := jobCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread_1", TurnID: "turn_b", WorkspaceRealPath: "/workspace", CaseID: "case_b",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-b")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot_b"), ContextEpoch: 2, IssuedAt: now,
	})
	if SecurityBindingMatchesContext(binding, contextB) {
		t.Fatal("case A job binding matched case B context")
	}
	tampered := CloneSecurityBinding(binding)
	tampered.ParentDatasetSnapshot = "snapshot_forged"
	if ValidateSecurityBinding(tampered) == nil {
		t.Fatal("tampered job security binding remained valid")
	}
}

func TestJobSecurityBindingCaseEpochScopeAllowsValidatedDescendantThreadOnlyWithinSameCase(t *testing.T) {
	now := time.Date(2026, 7, 10, 16, 0, 0, 0, time.UTC)
	contextA := jobCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread_a", TurnID: "turn_a", WorkspaceRealPath: "/workspace", CaseID: "case_a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-a")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot_a"), ContextEpoch: 7, IssuedAt: now,
	})
	callID := jobTestHostToolCallID("case-epoch")
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: contextA, Provider: "provider_1", ServerIdentity: "host:builtin", ToolName: "task", ToolCallID: callID,
		ArgsHash: domainsecurity.SHA256Hex([]byte("args")), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	binding, err := NewSecurityBinding(contextA, grant, grant.ToolCallID)
	if err != nil {
		t.Fatal(err)
	}
	contextB := jobDescendantContextV2(t, contextA, "thread_b", "turn_b", contextA.ContextEpoch)
	if SecurityBindingMatchesCaseEpoch(binding, contextB) {
		t.Fatal("exact parent-thread comparison must reject descendant thread")
	}
	if !SecurityBindingMatchesCaseEpochScope(binding, contextB) {
		t.Fatal("same frozen case epoch scope should remain eligible after separate ancestry validation")
	}
	contextB = jobDescendantContextV2(t, contextA, "thread_b", "turn_b", contextA.ContextEpoch+100)
	if !SecurityBindingMatchesWorkspaceScope(binding, contextB) {
		t.Fatal("thread-local epoch must not invalidate another thread sharing the exact workspace authority")
	}
	contextB = jobDescendantContextWithDatasetV2(t, contextA, "thread_b", "turn_b", contextA.ContextEpoch, securitycontexttest.DatasetSnapshotID("snapshot_b"))
	if SecurityBindingMatchesCaseEpochScope(binding, contextB) {
		t.Fatal("different dataset snapshot must reject descendant-thread reuse")
	}
	if SecurityBindingMatchesWorkspaceScope(binding, contextB) {
		t.Fatal("different dataset snapshot must invalidate shared-workspace jobs")
	}
	contextB = jobDescendantContextWithSourceManifestV2(
		t, contextA, "thread_b", "turn_b", contextA.ContextEpoch,
		domainsecurity.SHA256Hex([]byte("source-manifest-b")),
	)
	if SecurityBindingMatchesCaseEpochScope(binding, contextB) {
		t.Fatal("different source manifest must reject descendant-thread reuse even within the same epoch")
	}
}

func TestJobSecurityBindingScopeIdentityIncludesTenantAndUser(t *testing.T) {
	now := time.Date(2026, 7, 10, 16, 0, 0, 0, time.UTC)
	contextA := jobCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "shared-thread-id", TurnID: "turn-a", WorkspaceRealPath: "/workspace/shared", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "case-a", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-a")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-a"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-a")), ContextEpoch: 1, IssuedAt: now,
	})
	callID := jobTestHostToolCallID("principal")
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: contextA, Provider: "provider-1", ServerIdentity: "host:builtin", ToolName: "task", ToolCallID: callID,
		ArgsHash: domainsecurity.SHA256Hex([]byte("args")), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	binding, err := NewSecurityBinding(contextA, grant, grant.ToolCallID)
	if err != nil {
		t.Fatal(err)
	}
	otherPrincipal := jobCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: contextA.ThreadID, TurnID: "turn-b", WorkspaceRealPath: contextA.WorkspaceRealPath, TenantID: "tenant-b", UserID: "user-b",
		CaseID: "case-b", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-b")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-b"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-b")), ContextEpoch: 2, IssuedAt: now,
	})
	if SecurityBindingSharesThread(binding, otherPrincipal) {
		t.Fatal("same thread id in another tenant/user namespace was treated as the same thread")
	}
	if SecurityBindingSharesWorkspace(binding, otherPrincipal) {
		t.Fatal("same path in another tenant/user namespace was treated as the same workspace")
	}
}

func TestAuditOnlyV1CannotMintBoundJobAuthority(t *testing.T) {
	now := time.Now().UTC()
	legacy := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-v1", TurnID: "turn-v1", WorkspaceRealPath: "/workspace", ContextEpoch: 1, IssuedAt: now,
	})
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: legacy, Provider: "provider", ServerIdentity: "host:builtin", ToolName: "task", ToolCallID: "call-v1",
		ArgsHash: domainsecurity.SHA256Hex([]byte("args")), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	if binding, err := NewSecurityBinding(legacy, grant, grant.ToolCallID); err == nil || binding != nil {
		t.Fatalf("audit-only V1 minted a live job binding: binding=%#v err=%v", binding, err)
	}
	legacyBinding := &SecurityBinding{Version: 1, ParentContextVersion: domainsecurity.TurnSecurityContextVersionV1}
	if ValidateSecurityBinding(legacyBinding) == nil {
		t.Fatal("legacy job binding version remained live")
	}
}

func jobCaseContextV2(t *testing.T, input domainsecurity.TurnSecurityContextInput) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := securitycontexttest.CaseExecutionContextV2(input)
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func jobTestHostToolCallID(seed string) string {
	entropy := sha256.Sum256([]byte("domain-job-test-tool-call:\x00" + seed))
	identity, err := domainsecurity.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		panic(err)
	}
	return identity
}

func jobDescendantContextV2(t *testing.T, parent domainsecurity.TurnSecurityContext, threadID, turnID string, epoch uint64) domainsecurity.TurnSecurityContext {
	return jobDescendantContextWithDatasetV2(t, parent, threadID, turnID, epoch, parent.DatasetSnapshotID)
}

func jobDescendantContextWithDatasetV2(t *testing.T, parent domainsecurity.TurnSecurityContext, threadID, turnID string, epoch uint64, snapshotID string) domainsecurity.TurnSecurityContext {
	return jobDescendantContextWithScopeV2(t, parent, threadID, turnID, epoch, snapshotID, parent.SourceManifestHash)
}

func jobDescendantContextWithSourceManifestV2(t *testing.T, parent domainsecurity.TurnSecurityContext, threadID, turnID string, epoch uint64, sourceManifestHash string) domainsecurity.TurnSecurityContext {
	return jobDescendantContextWithScopeV2(t, parent, threadID, turnID, epoch, parent.DatasetSnapshotID, sourceManifestHash)
}

func jobDescendantContextWithScopeV2(t *testing.T, parent domainsecurity.TurnSecurityContext, threadID, turnID string, epoch uint64, snapshotID string, sourceManifestHash string) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: parent.WorkspaceRealPath,
		TenantID: parent.TenantID, UserID: parent.UserID, CaseID: parent.CaseID, CaseBindingHash: parent.CaseBindingHash,
		DatasetSnapshotID: snapshotID, SourceManifestHash: sourceManifestHash, ContextEpoch: epoch,
		IssuedAt: time.Now().UTC(), PublicationPolicy: parent.PublicationPolicy, RiskAuthorityBinding: parent.RiskAuthorityBinding,
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}
