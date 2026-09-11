package nativecomponent

import (
	"context"
	"errors"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	nativecomponentport "analytix.local/runtime-go/internal/ports/nativecomponent"
)

var ErrRuntimeAuthorityUnavailable = errors.New("native_component_runtime_authority_unavailable")

type RuntimeAuthorityDependencies struct {
	Health                               HealthDependencies
	LiveAuthority                        LiveAuthority
	HealthOnlyAuthority                  LiveAuthority
	Owner                                nativecomponentport.Owner
	UseCurrentAccountFlowSource          UseCurrentAccountFlowSource
	UseCurrentAccountIngressSource       UseCurrentAccountIngressSource
	ResolveAccountFlowSubject            ResolveAccountFlowSubject
	ResolveAccountFlowCounterparty       ResolveAccountFlowCounterparty
	ValidateCurrentAccountFlowCallback   ValidateCurrentAccountFlowCallback
	ValidateCurrentAccountFlowOuterGrant ValidateCurrentAccountFlowOuterGrant
}

// RuntimeAuthority is the single host authority for native-component turn
// admission, durable health settlement, and lifecycle shutdown. It is always
// assembled: a host without an admitted native runner uses explicit
// unavailable mode and still durably settles a metadata-only health result.
type RuntimeAuthority struct {
	health  *HealthCoordinator
	service *Service
	owner   nativecomponentport.Owner
}

func NewReadyRuntimeAuthority(dependencies RuntimeAuthorityDependencies) (*RuntimeAuthority, error) {
	if dependencies.Health.Service != nil || dependencies.LiveAuthority == nil ||
		dependencyIsNil(dependencies.LiveAuthority) || dependencies.HealthOnlyAuthority == nil ||
		dependencyIsNil(dependencies.HealthOnlyAuthority) || dependencyIsNil(dependencies.Owner) {
		return nil, ErrRuntimeAuthorityUnavailable
	}
	var accountFlowRunner nativecomponentport.AccountFlowRunner
	if candidate, ok := dependencies.Owner.(nativecomponentport.AccountFlowRunner); ok && !dependencyIsNil(candidate) {
		accountFlowRunner = candidate
	}
	var accountIngressRunner nativecomponentport.AccountIngressRunner
	if candidate, ok := dependencies.Owner.(nativecomponentport.AccountIngressRunner); ok && !dependencyIsNil(candidate) {
		accountIngressRunner = candidate
	}
	service := NewService(Dependencies{
		Threads:                              dependencies.Health.Store,
		DurableAuthority:                     dependencies.Health.DurableAuthority,
		LiveAuthority:                        dependencies.LiveAuthority,
		HealthOnlyAuthority:                  dependencies.HealthOnlyAuthority,
		AcquireEffect:                        dependencies.Health.AcquireEffect,
		Runner:                               dependencies.Owner,
		AccountFlowRunner:                    accountFlowRunner,
		AccountIngressRunner:                 accountIngressRunner,
		UseCurrentAccountFlowSource:          dependencies.UseCurrentAccountFlowSource,
		UseCurrentAccountIngressSource:       dependencies.UseCurrentAccountIngressSource,
		ResolveAccountFlowSubject:            dependencies.ResolveAccountFlowSubject,
		ResolveAccountFlowCounterparty:       dependencies.ResolveAccountFlowCounterparty,
		ValidateCurrentAccountFlowCallback:   dependencies.ValidateCurrentAccountFlowCallback,
		ValidateCurrentAccountFlowOuterGrant: dependencies.ValidateCurrentAccountFlowOuterGrant,
		Now:                                  dependencies.Health.Now,
	})
	if !service.Available() {
		return nil, ErrRuntimeAuthorityUnavailable
	}
	dependencies.Health.Service = service
	health := NewHealthCoordinator(dependencies.Health)
	if !health.Available() {
		return nil, ErrRuntimeAuthorityUnavailable
	}
	return &RuntimeAuthority{health: health, service: service, owner: dependencies.Owner}, nil
}

func NewUnavailableRuntimeAuthority(dependencies HealthDependencies) (*RuntimeAuthority, error) {
	if dependencies.Service != nil {
		return nil, ErrRuntimeAuthorityUnavailable
	}
	health := NewHealthCoordinator(dependencies)
	if !health.Available() {
		return nil, ErrRuntimeAuthorityUnavailable
	}
	return &RuntimeAuthority{health: health}, nil
}

func (authority *RuntimeAuthority) Available() bool {
	return authority != nil && authority.health != nil && authority.health.Available()
}

// AccountFlowsAvailable reports whether the existing native runtime authority
// was composed with every dependency required by the additive funds lane. It
// does not affect ordinary native health or the permanent general Agent tool
// base; callers use it only to decide whether to advertise the funds tool.
func (authority *RuntimeAuthority) AccountFlowsAvailable() bool {
	return authority != nil && authority.service != nil && authority.service.accountFlowsAvailable()
}

// AccountIngressAvailable reports only the optional host-private account
// ingress resolver. Its absence never changes runtime startup or the ordinary
// Agent tool base.
func (authority *RuntimeAuthority) AccountIngressAvailable() bool {
	return authority != nil && authority.service != nil && authority.service.accountIngressResolutionAvailable()
}

func (authority *RuntimeAuthority) PrepareCaseTurn(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
) error {
	if domainsecurity.ValidateTurnSecurityContext(securityContext) != nil {
		return ErrRuntimeAuthorityUnavailable
	}
	if domainsecurity.TurnSecurityContextIsGeneral(securityContext) ||
		domainsecurity.TurnSecurityContextIsBoundaryOnly(securityContext) {
		return nil
	}
	if domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) != nil ||
		!domainsecurity.TurnSecurityContextAllowsCaseEvidence(securityContext) {
		return ErrRuntimeAuthorityUnavailable
	}
	if !authority.Available() {
		return ErrRuntimeAuthorityUnavailable
	}
	if ctx == nil {
		return ErrRuntimeAuthorityUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := authority.health.ensureDataEngineReady(ctx, securityContext)
	if err != nil {
		if caseHealthFailureLeavesOrdinaryLaneSafe(err) {
			return errors.Join(ErrHealthUnavailableSettled, err)
		}
		return err
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func caseHealthFailureLeavesOrdinaryLaneSafe(err error) bool {
	if !errors.Is(err, ErrHealthExecutionUnsafe) ||
		errors.Is(err, ErrHealthAuthorityInvalid) || errors.Is(err, ErrHealthSettlement) ||
		errors.Is(err, ErrHealthDuplicate) || errors.Is(err, ErrAuthorityInvalid) ||
		errors.Is(err, ErrGrantInvalid) || errors.Is(err, ErrRequestInvalid) ||
		errors.Is(err, nativecomponentport.ErrTerminationUnconfirmed) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return true
}

// AnalyzeAccountFlows exposes the typed case operation as provider-safe
// semantics plus one synchronous host-private evidence projection. A runtime
// without an admitted native flow runner rejects this call without changing
// the health, ordinary Agent, or lifecycle behavior.
//
// consumeEvidence is permitted only to assemble a transient process-local
// in-memory value. It must never persist, publish, settle, log, emit, or perform
// any filesystem, network, or other external effect. Production composition
// must pass an in-memory assembler and perform later effects only after this
// method returns successfully.
func (authority *RuntimeAuthority) AnalyzeAccountFlows(
	ctx context.Context,
	input AnalyzeAccountFlowsInput,
	consumeEvidence domainnative.AccountFlowHostEvidenceProjectionConsumerV1,
) (domainnative.AccountFlowProviderSemanticResultV1, error) {
	if authority == nil || authority.service == nil || !authority.service.accountFlowsAvailable() {
		return domainnative.AccountFlowProviderSemanticResultV1{}, ErrUnavailable
	}
	return authority.service.AnalyzeAccountFlows(ctx, input, consumeEvidence)
}

// ResolveAccountIngress performs the pre-compilation private batch lookup.
// The call must finish before caseentity.CompileAccountIngressV1 enters its
// separate private-state DSV2 lease.
func (authority *RuntimeAuthority) ResolveAccountIngress(
	ctx context.Context,
	input ResolveAccountIngressInputV1,
) (
	domainnative.ResolveAccountIngressResultV1,
	domainfundsquerysource.DescriptorV1,
	error,
) {
	if authority == nil || authority.service == nil || !authority.service.accountIngressResolutionAvailable() {
		return domainnative.ResolveAccountIngressResultV1{},
			domainfundsquerysource.DescriptorV1{},
			ErrUnavailable
	}
	return authority.service.ResolveAccountIngress(ctx, input)
}

func (authority *RuntimeAuthority) Close() error {
	if authority == nil || dependencyIsNil(authority.owner) {
		return nil
	}
	return authority.owner.Close()
}
