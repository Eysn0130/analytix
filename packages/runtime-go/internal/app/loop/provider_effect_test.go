package loop

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"testing"
	"time"

	appmodel "analytix.local/runtime-go/internal/app/model"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestAcquirePendingToolEffectSelectsExactPerCallAuthorityBeforeLease(t *testing.T) {
	issuedAt := time.Date(2026, 7, 27, 14, 0, 0, 0, time.UTC)
	boundary := loopWitnessedBoundaryContext(t, issuedAt)
	ordinary := loopPendingToolCall(t, boundary, "read", `{"path":"README.md"}`, true, issuedAt)
	acquires := 0
	caseDataEffect := true
	acquire := func(ctx context.Context, _ domainsecurity.TurnSecurityContext, protected bool) (context.Context, func(), error) {
		acquires++
		caseDataEffect = protected
		return ctx, func() {}, nil
	}
	_, release, err := AcquirePendingToolEffect(context.Background(), ordinary, acquire)
	if err != nil || release == nil || acquires != 1 || caseDataEffect {
		t.Fatalf("ordinary boundary call selected the wrong effect: acquires=%d protected=%t err=%v", acquires, caseDataEffect, err)
	}
	release()

	protected := loopPendingToolCall(t, boundary, "stage_case_report", `{}`, false, issuedAt)
	acquires = 0
	if _, release, err := AcquirePendingToolEffect(context.Background(), protected, acquire); err == nil || release != nil || acquires != 0 {
		t.Fatalf("protected boundary call reached an effect lease: acquires=%d release=%v err=%v", acquires, release != nil, err)
	}

	for name, mutate := range map[string]func(*appmodel.PendingToolCall){
		"tool name": func(value *appmodel.PendingToolCall) { value.Call.Name = "stage_case_report" },
		"call id":   func(value *appmodel.PendingToolCall) { value.Call.ID = loopToolCallID("changed") },
		"arguments": func(value *appmodel.PendingToolCall) { value.Call.Arguments = json.RawMessage(`{"path":"another.md"}`) },
	} {
		t.Run(name, func(t *testing.T) {
			changed := ordinary
			mutate(&changed)
			acquires = 0
			if _, release, err := AcquirePendingToolEffect(context.Background(), changed, acquire); err == nil || release != nil || acquires != 0 {
				t.Fatalf("changed grant/call binding reached an effect lease: acquires=%d release=%v err=%v", acquires, release != nil, err)
			}
		})
	}
}

func loopWitnessedBoundaryContext(t *testing.T, issuedAt time.Time) domainsecurity.TurnSecurityContext {
	t.Helper()
	quarantined, err := securitycontexttest.BoundaryOnlyContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-loop-effect", TurnID: "turn-loop-effect", WorkspaceRealPath: "/workspace/loop-effect",
		ContextEpoch: 1, IssuedAt: issuedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := securitycontexttest.WitnessedRiskBinding(
		quarantined.ThreadID,
		quarantined.WorkspaceRealPath,
		domainsecurity.RiskClassCase,
		quarantined.PublicationPolicy.ThreadRiskPolicyDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: quarantined.ThreadID, TurnID: quarantined.TurnID, WorkspaceRealPath: quarantined.WorkspaceRealPath,
		TenantID: quarantined.TenantID, UserID: quarantined.UserID, CaseID: quarantined.CaseID,
		CaseBindingHash: quarantined.CaseBindingHash, DatasetSnapshotID: quarantined.DatasetSnapshotID,
		SourceManifestHash: quarantined.SourceManifestHash, ContextEpoch: quarantined.ContextEpoch, IssuedAt: issuedAt,
		PublicationPolicy: quarantined.PublicationPolicy, RiskAuthorityBinding: binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func loopPendingToolCall(
	t *testing.T,
	securityContext domainsecurity.TurnSecurityContext,
	toolName string,
	arguments string,
	readOnly bool,
	issuedAt time.Time,
) appmodel.PendingToolCall {
	t.Helper()
	call := domainmodel.ToolCall{ID: loopToolCallID(toolName), Name: toolName, Arguments: json.RawMessage(arguments)}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-test", ServerIdentity: "host:builtin",
		ToolName: call.Name, ToolCallID: call.ID, ArgsHash: domainsecurity.CanonicalJSONHash(call.Arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("schema:" + call.Name)), ScopeHash: domainsecurity.SHA256Hex([]byte("scope:" + call.Name)),
		ReadOnly: readOnly, ApprovalState: "not_required", IssuedAt: issuedAt,
	})
	if err := domainsecurity.ValidateExecutionGrant(grant); err != nil {
		t.Fatal(err)
	}
	return appmodel.PendingToolCall{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, ProviderID: grant.Provider,
		Call: call, SecurityContext: securityContext, ExecutionGrant: grant,
	}
}

func loopToolCallID(seed string) string {
	entropy := sha256.Sum256([]byte("loop-provider-effect-test\x00" + seed))
	identity, err := domainmodel.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		panic(err)
	}
	return identity
}
