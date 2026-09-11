package threadriskauthority

import (
	"context"
	"errors"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	casecontextport "analytix.local/runtime-go/internal/ports/casecontext"
)

var ErrGeneralOnlyDenied = errors.New("host general-only risk authority denied elevated or indeterminate risk")

// GeneralOnlyAuthority is the production-safe fallback when no independently
// enrolled ThreadRisk witness is available. It derives a restart-stable policy
// only from the compiled host rules and a fresh exact-missing case-binding
// observation. It never signs, persists, or simulates a monotonic witness.
type GeneralOnlyAuthority struct {
	observer casecontextport.Observer
}

func NewGeneralOnlyAuthority(observer casecontextport.Observer) (*GeneralOnlyAuthority, error) {
	if observer == nil {
		return nil, ErrInvalidInput
	}
	return &GeneralOnlyAuthority{observer: observer}, nil
}

func (authority *GeneralOnlyAuthority) ResolveOrRaise(ctx context.Context, input ResolveOrRaiseInput) (Head, error) {
	if authority == nil || authority.observer == nil || ctx == nil || contextError(ctx) != nil ||
		strings.TrimSpace(input.ThreadID) == "" || input.ThreadID != strings.TrimSpace(input.ThreadID) ||
		strings.TrimSpace(input.WorkspaceRealPath) == "" || input.WorkspaceRealPath != strings.TrimSpace(input.WorkspaceRealPath) ||
		input.RequestedRisk != domainsecurity.RiskClassGeneral || input.Origin != domainsecurity.RiskPolicyOriginGeneralWorkspace ||
		domainsecurity.ValidateCaseBindingObservationV1(input.BindingObservation) != nil ||
		input.BindingObservation.State != domainsecurity.CaseBindingStateMissing ||
		input.BindingObservation.WorkspaceRealPath != input.WorkspaceRealPath {
		return Head{}, ErrGeneralOnlyDenied
	}
	current, err := authority.observer.Observe(input.WorkspaceRealPath)
	if err != nil {
		return Head{}, errors.Join(ErrUnavailable, err)
	}
	if contextError(ctx) != nil {
		return Head{}, contextError(ctx)
	}
	if domainsecurity.ValidateCaseBindingObservationV1(current) != nil || current != input.BindingObservation ||
		current.State != domainsecurity.CaseBindingStateMissing {
		return Head{}, ErrGeneralOnlyDenied
	}
	policy, err := domainsecurity.NewGeneralOnlyRiskPolicyV1(input.ThreadID, input.WorkspaceRealPath, current.ObservationDigest)
	if err != nil {
		return Head{}, ErrIntegrity
	}
	binding, err := domainsecurity.NewHostGeneralOnlyRiskAuthorityBindingV1(policy)
	if err != nil {
		return Head{}, ErrIntegrity
	}
	return Head{RiskAuthorityBinding: binding, GeneralOnlyPolicy: policy, Found: true}, nil
}

func (authority *GeneralOnlyAuthority) ValidateCurrent(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) error {
	if authority == nil || authority.observer == nil || ctx == nil || contextError(ctx) != nil ||
		domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) != nil ||
		!domainsecurity.TurnSecurityContextUsesHostGeneralOnlyRisk(securityContext) {
		return ErrInvalidInput
	}
	current, err := authority.observer.Observe(securityContext.WorkspaceRealPath)
	if err != nil {
		return errors.Join(ErrUnavailable, err)
	}
	if contextError(ctx) != nil {
		return contextError(ctx)
	}
	if domainsecurity.ValidateCaseBindingObservationV1(current) != nil || current.State != domainsecurity.CaseBindingStateMissing ||
		current.WorkspaceRealPath != securityContext.WorkspaceRealPath ||
		current.ObservationDigest != securityContext.PublicationPolicy.BindingObservationDigest {
		return ErrGeneralOnlyDenied
	}
	policy, err := domainsecurity.NewGeneralOnlyRiskPolicyV1(
		securityContext.ThreadID, securityContext.WorkspaceRealPath, current.ObservationDigest,
	)
	if err != nil || domainsecurity.ValidateHostGeneralOnlyRiskAuthorityBindingV1(securityContext.RiskAuthorityBinding, policy) != nil ||
		domainsecurity.ValidateTurnPublicationPolicyForGeneralOnlyRiskPolicyV1(securityContext.PublicationPolicy, policy) != nil {
		return ErrIntegrity
	}
	return nil
}
