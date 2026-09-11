package evidence

import (
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func newEvidenceCaseContextV2(t *testing.T, input domainsecurity.TurnSecurityContextInput) domainsecurity.TurnSecurityContext {
	t.Helper()
	if input.TenantID == "" {
		input.TenantID = domainsecurity.LocalTenantID
	}
	if input.UserID == "" {
		input.UserID = domainsecurity.LocalUserID
	}
	policyDigest := domainsecurity.SHA256Hex([]byte("evidence-test-risk-policy:\x00" + input.ThreadID + "\x00" + input.WorkspaceRealPath))
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: policyDigest,
		RiskClass:              domainsecurity.RiskClassCase,
		Disposition:            domainsecurity.PublicationDispositionCaseEvidenceGate,
		CaseBindingState:       domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte(
			"evidence-test-binding-observation:\x00" + input.ThreadID + "\x00" + input.TurnID + "\x00" + input.CaseBindingHash,
		)),
		BlockerCode: domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := securitycontexttest.WitnessedRiskBinding(
		input.ThreadID,
		input.WorkspaceRealPath,
		domainsecurity.RiskClassCase,
		policy.ThreadRiskPolicyDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	input.PublicationPolicy = policy
	input.RiskAuthorityBinding = binding
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(input)
	if err != nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil {
		t.Fatalf("evidence V2 test context is invalid: context=%#v err=%v", securityContext, err)
	}
	return securityContext
}
