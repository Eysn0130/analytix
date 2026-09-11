package threadriskauthority

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	casecontextport "analytix.local/runtime-go/internal/ports/casecontext"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	policyport "analytix.local/runtime-go/internal/ports/threadriskpolicy"
)

var ErrHostPolicyDenied = errors.New("host thread risk policy authority denied current effect")

// HostPolicyAuthority implements the existing turn-security risk authority
// with an installation-signed policy plus a fresh host case-binding
// observation. It is the first-stage composition used when the deferred
// independent monotonic witness is not configured. Exact policy CAS provides
// restart readback and integrity, but this implementation deliberately makes
// no latest-state or rollback-resistance claim.
type HostPolicyAuthority struct {
	observer    casecontextport.Observer
	authority   finalauthorityport.Authority
	policies    policyport.Store
	generalOnly *GeneralOnlyAuthority
}

func NewHostPolicyAuthority(
	observer casecontextport.Observer,
	authority finalauthorityport.Authority,
	policies policyport.Store,
) (*HostPolicyAuthority, error) {
	if observer == nil || authority == nil || policies == nil ||
		!domainsecurity.IsSHA256Hex(authority.KeyID()) ||
		authority.KeyID() != domainsecurity.SHA256Hex(authority.PublicKey()) {
		return nil, ErrInvalidInput
	}
	generalOnly, err := NewGeneralOnlyAuthority(observer)
	if err != nil {
		return nil, err
	}
	return &HostPolicyAuthority{
		observer: observer, authority: authority, policies: policies,
		generalOnly: generalOnly,
	}, nil
}

func (authority *HostPolicyAuthority) ResolveOrRaise(
	ctx context.Context,
	input ResolveOrRaiseInput,
) (Head, error) {
	if authority == nil || authority.observer == nil || authority.authority == nil ||
		authority.policies == nil || ctx == nil || contextError(ctx) != nil ||
		validateResolveInput(input) != nil ||
		domainsecurity.ValidateCaseBindingObservationV1(input.BindingObservation) != nil ||
		input.BindingObservation.WorkspaceRealPath != input.WorkspaceRealPath {
		return Head{}, ErrInvalidInput
	}
	if input.RequestedRisk == domainsecurity.RiskClassGeneral {
		return authority.generalOnly.ResolveOrRaise(ctx, input)
	}
	if input.RequestedRisk != domainsecurity.RiskClassCase {
		return Head{}, ErrHostPolicyDenied
	}
	current, err := authority.observeExact(ctx, input.WorkspaceRealPath)
	if err != nil {
		return Head{}, err
	}
	if current != input.BindingObservation {
		return Head{}, ErrHostPolicyDenied
	}

	var previous domainsecurity.ThreadRiskPolicyV1
	if input.PreviousPolicyDigest != "" {
		previous, err = authority.resolveExactPolicy(ctx, input.PreviousPolicyDigest)
		if err != nil || previous.ThreadID != input.ThreadID ||
			previous.WorkspaceRealPath != input.WorkspaceRealPath ||
			previous.RiskClass != domainsecurity.RiskClassCase {
			return Head{}, errors.Join(ErrHostPolicyDenied, err)
		}
	}
	policy, err := domainsecurity.NewThreadRiskPolicyV1(
		domainsecurity.ThreadRiskPolicyInputV1{
			ThreadID: input.ThreadID, WorkspaceRealPath: input.WorkspaceRealPath,
			RiskClass: input.RequestedRisk, Origin: input.Origin,
			SignalsDigest:           input.SignalsDigest,
			PredecessorPolicyDigest: input.PreviousPolicyDigest,
			IssuedAt:                input.IssuedAt, AuthorityKeyID: authority.authority.KeyID(),
			AuthorityPublicKey: authority.authority.PublicKey(),
		},
		func(message []byte) ([]byte, error) {
			return authority.authority.Sign(ctx, message)
		},
	)
	if err != nil || authority.validatePolicy(ctx, policy) != nil {
		return Head{}, errors.Join(ErrIntegrity, err)
	}
	if input.PreviousPolicyDigest != "" &&
		domainsecurity.ValidateThreadRiskPolicyTransitionV1(previous, policy) != nil {
		return Head{}, ErrHostPolicyDenied
	}
	if err := authority.policies.PutIfAbsent(ctx, policy); err != nil {
		return Head{}, errors.Join(ErrUnavailable, err)
	}
	readback, err := authority.resolveExactPolicy(ctx, policy.PolicyDigest)
	if err != nil || !equalPolicy(readback, policy) {
		return Head{}, errors.Join(ErrIntegrity, err)
	}
	currentAfter, err := authority.observeExact(ctx, input.WorkspaceRealPath)
	if err != nil {
		return Head{}, err
	}
	if currentAfter != current {
		return Head{}, ErrHostPolicyDenied
	}
	binding, err := domainsecurity.NewHostPolicyRiskAuthorityBindingV1(policy, currentAfter)
	if err != nil {
		return Head{}, errors.Join(ErrIntegrity, err)
	}
	return Head{
		RiskAuthorityBinding: binding,
		Policy:               policy,
		Found:                true,
	}, nil
}

func (authority *HostPolicyAuthority) ValidateCurrent(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
) error {
	if authority == nil || authority.observer == nil || authority.authority == nil ||
		authority.policies == nil || ctx == nil || contextError(ctx) != nil {
		return ErrInvalidInput
	}
	if domainsecurity.TurnSecurityContextUsesHostGeneralOnlyRisk(securityContext) {
		return authority.generalOnly.ValidateCurrent(ctx, securityContext)
	}
	if domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(securityContext) != nil ||
		!domainsecurity.TurnSecurityContextUsesHostRiskPolicy(securityContext) {
		return ErrInvalidInput
	}
	policy, err := authority.resolveExactPolicy(
		ctx,
		securityContext.RiskAuthorityBinding.ThreadPolicyDigest,
	)
	if err != nil || policy.ThreadID != securityContext.ThreadID ||
		policy.WorkspaceRealPath != securityContext.WorkspaceRealPath ||
		policy.RiskClass != domainsecurity.RiskClassCase ||
		domainsecurity.ValidateTurnPublicationPolicyForThreadRiskPolicyV1(
			securityContext.PublicationPolicy,
			policy,
		) != nil {
		return errors.Join(ErrIntegrity, err)
	}
	current, err := authority.observeExact(ctx, securityContext.WorkspaceRealPath)
	if err != nil {
		return err
	}
	if domainsecurity.ValidateHostPolicyRiskAuthorityBindingV1(
		securityContext.RiskAuthorityBinding,
		policy,
		current,
	) != nil ||
		securityContext.PublicationPolicy.BindingObservationDigest != current.ObservationDigest {
		return ErrHostPolicyDenied
	}
	if domainsecurity.TurnSecurityContextAllowsCaseEvidence(securityContext) &&
		(current.State != domainsecurity.CaseBindingStateValid ||
			current.CaseID != securityContext.CaseID ||
			current.CaseBindingHash != securityContext.CaseBindingHash) {
		return ErrHostPolicyDenied
	}
	return nil
}

func (authority *HostPolicyAuthority) observeExact(
	ctx context.Context,
	workspaceRealPath string,
) (domainsecurity.CaseBindingObservationV1, error) {
	if ctx == nil || contextError(ctx) != nil ||
		workspaceRealPath == "" || workspaceRealPath != strings.TrimSpace(workspaceRealPath) {
		return domainsecurity.CaseBindingObservationV1{}, ErrInvalidInput
	}
	observation, err := authority.observer.Observe(workspaceRealPath)
	if err != nil {
		return domainsecurity.CaseBindingObservationV1{}, errors.Join(ErrUnavailable, err)
	}
	if contextError(ctx) != nil {
		return domainsecurity.CaseBindingObservationV1{}, contextError(ctx)
	}
	if domainsecurity.ValidateCaseBindingObservationV1(observation) != nil ||
		observation.WorkspaceRealPath != workspaceRealPath {
		return domainsecurity.CaseBindingObservationV1{}, ErrIntegrity
	}
	return observation, nil
}

func (authority *HostPolicyAuthority) resolveExactPolicy(
	ctx context.Context,
	digest string,
) (domainsecurity.ThreadRiskPolicyV1, error) {
	if !domainsecurity.IsSHA256Hex(digest) || digest != strings.TrimSpace(digest) {
		return domainsecurity.ThreadRiskPolicyV1{}, ErrIntegrity
	}
	policy, err := authority.policies.Resolve(ctx, digest)
	if err != nil {
		return domainsecurity.ThreadRiskPolicyV1{}, errors.Join(ErrUnavailable, err)
	}
	if policy.PolicyDigest != digest || authority.validatePolicy(ctx, policy) != nil {
		return domainsecurity.ThreadRiskPolicyV1{}, ErrIntegrity
	}
	return policy, nil
}

func (authority *HostPolicyAuthority) validatePolicy(
	ctx context.Context,
	policy domainsecurity.ThreadRiskPolicyV1,
) error {
	if domainsecurity.ValidateThreadRiskPolicyV1ForInstallation(
		policy,
		authority.authority.KeyID(),
		authority.authority.PublicKey(),
	) != nil {
		return ErrIntegrity
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(policy.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(policy.AuthoritySignature)
	if publicErr != nil || signatureErr != nil {
		return ErrIntegrity
	}
	defer clear(publicKey)
	defer clear(signature)
	if err := authority.authority.VerifyTrusted(
		ctx,
		policy.AuthorityKeyID,
		publicKey,
		domainsecurity.ThreadRiskPolicySigningBytesV1(policy),
		signature,
	); err != nil {
		return ErrIntegrity
	}
	return nil
}

var _ interface {
	ResolveOrRaise(context.Context, ResolveOrRaiseInput) (Head, error)
	ValidateCurrent(context.Context, domainsecurity.TurnSecurityContext) error
} = (*HostPolicyAuthority)(nil)
