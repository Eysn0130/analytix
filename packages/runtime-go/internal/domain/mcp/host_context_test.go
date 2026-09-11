package mcp

import (
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
	toolidentitytest "analytix.local/runtime-go/internal/testsupport/toolidentity"
)

func TestHostContextEnvelopeRequiresMatchingHostAuthority(t *testing.T) {
	now := time.Date(2026, 7, 11, 1, 0, 0, 0, time.UTC)
	securityContext := hostContextTestExecutionContext(t, "turn-a", now)
	serverIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity("docs", "docs", "1.0.0", domainsecurity.SHA256Hex([]byte("host-context-test-instance")), 4)
	if err != nil {
		t.Fatal(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-a", ServerIdentity: serverIdentity,
		ToolName: "mcp__docs__lookup", ToolCallID: toolidentitytest.MustHostToolCallIDV1("domain-mcp-host-context"), ConnectionEpoch: 4,
		ArgsHash: domainsecurity.SHA256Hex([]byte("args")), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	envelope, err := NewHostContextEnvelope(securityContext, grant)
	if err != nil {
		t.Fatal(err)
	}
	record, err := envelope.RuntimeContextRecord()
	if err != nil || record["contextDigest"] != securityContext.ContextDigest || record["grantId"] != grant.GrantID ||
		record["workspaceRealPath"] != securityContext.WorkspaceRealPath || record["tenantId"] != securityContext.TenantID ||
		record["userId"] != securityContext.UserID || record["provider"] != grant.Provider ||
		record["grantIssuedAt"] != grant.IssuedAt || record["connectionEpoch"] != grant.ConnectionEpoch {
		t.Fatalf("host context record mismatch: record=%#v err=%v", record, err)
	}
	for _, legacyAlias := range []string{"workspaceRoot", "workspace_root", "cwd", "thread_id", "turn_id"} {
		if _, found := record[legacyAlias]; found {
			t.Fatalf("host context record retained ambiguous alias %q: %#v", legacyAlias, record)
		}
	}
	otherContext := hostContextTestExecutionContext(t, "turn-b", now)
	if _, err := NewHostContextEnvelope(otherContext, grant); err == nil {
		t.Fatal("grant for another turn constructed a host context envelope")
	}
	if err := ValidateHostContextEnvelope(HostContextEnvelope{}); err == nil {
		t.Fatal("zero host context envelope was accepted")
	}
}

func hostContextTestExecutionContext(t *testing.T, turnID string, now time.Time) domainsecurity.TurnSecurityContext {
	t.Helper()
	policyDigest := domainsecurity.SHA256Hex([]byte("host-context-test-risk-policy"))
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: policyDigest, RiskClass: domainsecurity.RiskClassGeneral,
		Disposition: domainsecurity.PublicationDispositionGeneralOutput, CaseBindingState: domainsecurity.CaseBindingStateMissing,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("host-context-test-binding-observation")),
		BlockerCode:              domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := securitycontexttest.WitnessedRiskBinding("thread-a", "/workspace", domainsecurity.RiskClassGeneral, policyDigest)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-a", TurnID: turnID, WorkspaceRealPath: "/workspace",
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: domainsecurity.UnboundCaseID, CaseBindingHash: domainsecurity.UnboundCaseBindingHash("/workspace"),
		DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID, SourceManifestHash: domainsecurity.EmptySourceManifestHash,
		ContextEpoch: 3, IssuedAt: now, PublicationPolicy: policy, RiskAuthorityBinding: binding,
	})
	if err != nil || domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) != nil {
		t.Fatalf("host context V2 fixture is invalid: context=%#v err=%v", securityContext, err)
	}
	return securityContext
}
