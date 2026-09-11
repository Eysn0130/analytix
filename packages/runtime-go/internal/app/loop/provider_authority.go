package loop

import (
	"context"

	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
)

type ProviderCaseLineage interface {
	IsCaseThread(string) bool
}

type ProviderAttemptAuthorityInput struct {
	OperationContext context.Context
	Authority        turnsecurityapp.WorkspaceSecurityAuthority
	SecurityContext  domainsecurity.TurnSecurityContext
	Workspace        string
	CaseDataEffect   bool
	FundsDataEffect  bool
	CaseLineage      ProviderCaseLineage
	Source           any
}

// ValidateProviderAttemptAuthority revalidates only the authority required by
// the exact provider effect. Dataset revocation blocks protected case-data
// attempts without disabling ordinary work in the same Agent or thread. Only
// funds-data effects require the current analytix_funds source probe; other
// case-data effects, such as case-bound attachments, retain the exact current
// case and dataset authority without depending on that plugin.
func ValidateProviderAttemptAuthority(input ProviderAttemptAuthorityInput) error {
	ctx := input.OperationContext
	securityContext := input.SecurityContext
	if ctx == nil {
		return providerAuthorityFailure("provider authority is unavailable", "turn_security_authority_unavailable")
	}
	if input.FundsDataEffect && !input.CaseDataEffect {
		return providerAuthorityFailure("funds provider effect requires case-data authority", "turn_security_risk_policy_mismatch")
	}
	if err := turnsecurityapp.ValidateCurrentForEffect(turnsecurityapp.CurrentValidationInput{
		OperationContext: ctx, Identity: input.Authority.Identity,
		Observer: input.Authority.Observer, RiskAuthority: input.Authority.RiskAuthority,
		SnapshotAuthority: input.Authority.SnapshotAuthority, SnapshotAuthorityV2: input.Authority.SnapshotAuthorityV2,
		Context: securityContext, Workspace: input.Workspace,
	}, input.CaseDataEffect); err != nil {
		return TurnFailureError{
			Message:  "provider authority changed before dispatch",
			Code:     turnsecurityapp.CurrentFailureCode(err),
			Severity: "error",
		}
	}
	if !input.CaseDataEffect {
		return nil
	}
	if input.CaseLineage == nil {
		return providerAuthorityFailure("case lineage authority is unavailable", "turn_security_authority_unavailable")
	}
	if input.CaseLineage.IsCaseThread(securityContext.ThreadID) &&
		!domainsecurity.TurnSecurityContextRequiresFinalEvidenceGate(securityContext) {
		return providerAuthorityFailure("signed case lineage cannot use a general provider attempt", "turn_security_risk_policy_mismatch")
	}
	if !domainsecurity.TurnSecurityContextAllowsCaseEvidence(securityContext) {
		return providerAuthorityFailure("case evidence authority is unavailable", "turn_security_risk_policy_mismatch")
	}
	if !input.FundsDataEffect {
		return nil
	}
	validator, ok := input.Source.(sourceprobeport.CurrentValidator)
	if !ok || validator == nil ||
		validator.ValidateCurrentProbe(ctx, "analytix_funds", securityContext) != nil {
		return providerAuthorityFailure("case source authority changed before provider dispatch", "source_probe_unavailable")
	}
	return nil
}

func providerAuthorityFailure(message, code string) error {
	return TurnFailureError{Message: message, Code: code, Severity: "error"}
}
