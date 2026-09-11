package security

import (
	"testing"
	"time"
)

func TestOrdinaryEffectStructuralValidatorAcceptsOnlyWitnessedV2Boundary(t *testing.T) {
	const (
		threadID  = "thread-ordinary-boundary"
		turnID    = "turn-ordinary-boundary"
		workspace = "/workspace/ordinary-boundary"
	)
	policy := mustTurnPublicationPolicyV1(t, TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: SHA256Hex([]byte("ordinary-boundary-risk")),
		RiskClass:              RiskClassCase,
		Disposition:            PublicationDispositionCaseBoundaryOnly,
		CaseBindingState:       CaseBindingStateInvalid,
		BindingObservationDigest: SHA256Hex(
			[]byte("ordinary-boundary-observation"),
		),
		BlockerCode: PublicationBlockerCaseBindingInvalid,
	})
	witnessed, err := NewTurnSecurityContextV2(TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		TenantID: LocalTenantID, UserID: LocalUserID,
		CaseID: UnboundCaseID, CaseBindingHash: UnboundCaseBindingHash(workspace),
		DatasetSnapshotID: NoDatasetSnapshotID, SourceManifestHash: EmptySourceManifestHash,
		ContextEpoch: 3, IssuedAt: time.Date(2026, 7, 27, 1, 0, 0, 0, time.UTC),
		PublicationPolicy: policy,
		RiskAuthorityBinding: mustTestWitnessedRiskAuthorityBindingV1(
			t, threadID, workspace, RiskClassCase, policy.ThreadRiskPolicyDigest,
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTurnSecurityContextForOrdinaryEffect(witnessed); err != nil {
		t.Fatalf("witnessed boundary disabled ordinary effects: %v", err)
	}
	if err := ValidateTurnSecurityContextForExecution(witnessed); err == nil {
		t.Fatal("ordinary structural admission widened strict case execution")
	}
	if TurnSecurityContextAllowsCaseEvidence(witnessed) {
		t.Fatal("ordinary structural admission minted case evidence authority")
	}

	quarantined, err := NewTurnSecurityContextV2(TurnSecurityContextInput{
		ThreadID: threadID + "-quarantined", TurnID: turnID, WorkspaceRealPath: workspace,
		TenantID: LocalTenantID, UserID: LocalUserID,
		CaseID: UnboundCaseID, CaseBindingHash: UnboundCaseBindingHash(workspace),
		DatasetSnapshotID: NoDatasetSnapshotID, SourceManifestHash: EmptySourceManifestHash,
		ContextEpoch: 3, IssuedAt: time.Date(2026, 7, 27, 1, 0, 0, 0, time.UTC),
		PublicationPolicy:    policy,
		RiskAuthorityBinding: NewQuarantinedRiskAuthorityBindingV1(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTurnSecurityContextForOrdinaryEffect(quarantined); err == nil {
		t.Fatal("quarantined risk authority authorized an ordinary effect")
	}

	legacy := NewTurnSecurityContext(TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		TenantID: LocalTenantID, UserID: LocalUserID,
		CaseID: UnboundCaseID, CaseBindingHash: UnboundCaseBindingHash(workspace),
		DatasetSnapshotID: NoDatasetSnapshotID, SourceManifestHash: EmptySourceManifestHash,
		ContextEpoch: 3, IssuedAt: time.Date(2026, 7, 27, 1, 0, 0, 0, time.UTC),
	})
	if err := ValidateTurnSecurityContextForOrdinaryEffect(legacy); err == nil {
		t.Fatal("audit-only V1 context authorized an ordinary effect")
	}
}

func TestOrdinaryEffectStructuralValidatorRejectsInvalidCoreBindings(t *testing.T) {
	context := mustGeneralTurnSecurityContextV2(
		t, "thread-ordinary-invalid", "turn-ordinary-invalid", "/workspace/ordinary-invalid",
	)
	for name, mutate := range map[string]func(*TurnSecurityContext){
		"identity":  func(value *TurnSecurityContext) { value.UserID = "" },
		"workspace": func(value *TurnSecurityContext) { value.WorkspaceRealPath = "" },
		"epoch":     func(value *TurnSecurityContext) { value.ContextEpoch = 0 },
		"binding":   func(value *TurnSecurityContext) { value.CaseBindingHash = "not-a-sha256" },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := context
			mutate(&invalid)
			invalid.ContextDigest = ""
			invalid.ContextDigest = hashContract(invalid)
			if err := ValidateTurnSecurityContextForOrdinaryEffect(invalid); err == nil {
				t.Fatalf("invalid %s binding authorized an ordinary effect", name)
			}
		})
	}
}
