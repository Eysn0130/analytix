package turn

import (
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func newTurnExecutionContextV2(t *testing.T, threadID, turnID, workspace string, epoch uint64, issuedAt time.Time) domainsecurity.TurnSecurityContext {
	t.Helper()
	policyDigest := domainsecurity.SHA256Hex([]byte("turn-test-risk-policy:\x00" + threadID + "\x00" + workspace))
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: policyDigest, RiskClass: domainsecurity.RiskClassGeneral,
		Disposition: domainsecurity.PublicationDispositionGeneralOutput, CaseBindingState: domainsecurity.CaseBindingStateMissing,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("turn-test-binding-observation:\x00" + threadID + "\x00" + workspace)),
		BlockerCode:              domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := securitycontexttest.WitnessedRiskBinding(threadID, workspace, domainsecurity.RiskClassGeneral, policyDigest)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: domainsecurity.UnboundCaseID, CaseBindingHash: domainsecurity.UnboundCaseBindingHash(workspace),
		DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID, SourceManifestHash: domainsecurity.EmptySourceManifestHash,
		ContextEpoch: epoch, IssuedAt: issuedAt, PublicationPolicy: policy, RiskAuthorityBinding: binding,
	})
	if err != nil || domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) != nil {
		t.Fatalf("turn V2 execution fixture is invalid: context=%#v err=%v", securityContext, err)
	}
	return securityContext
}

func newTurnCaseExecutionContextV2(t *testing.T, threadID, turnID, workspace string, epoch uint64, issuedAt time.Time) domainsecurity.TurnSecurityContext {
	t.Helper()
	caseID := "case-a"
	caseBindingHash := domainsecurity.SHA256Hex([]byte("turn-test-case-binding:\x00" + threadID + "\x00" + workspace))
	policyDigest := domainsecurity.SHA256Hex([]byte("turn-test-case-risk-policy:\x00" + threadID + "\x00" + workspace))
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: policyDigest,
		RiskClass:              domainsecurity.RiskClassCase,
		Disposition:            domainsecurity.PublicationDispositionCaseEvidenceGate,
		CaseBindingState:       domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte(
			"turn-test-case-binding-observation:\x00" + threadID + "\x00" + turnID + "\x00" + caseBindingHash,
		)),
		BlockerCode: domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := securitycontexttest.WitnessedRiskBinding(threadID, workspace, domainsecurity.RiskClassCase, policyDigest)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: caseID, CaseBindingHash: caseBindingHash,
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID(threadID + "\x00" + turnID + "\x00" + caseBindingHash),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("turn-test-case-source-manifest:\x00" + threadID + "\x00" + turnID)),
		ContextEpoch:       epoch, IssuedAt: issuedAt, PublicationPolicy: policy, RiskAuthorityBinding: binding,
	})
	if err != nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil {
		t.Fatalf("turn case V2 execution fixture is invalid: context=%#v err=%v", securityContext, err)
	}
	return securityContext
}

func newTurnBoundaryOnlyContextV2(t *testing.T, threadID, turnID, workspace string, epoch uint64, issuedAt time.Time) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := securitycontexttest.BoundaryOnlyContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace, ContextEpoch: epoch, IssuedAt: issuedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}
