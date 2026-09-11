package runtimeapp

import (
	"context"
	"errors"
	"strings"

	fundsquerysourceadapter "analytix.local/runtime-go/internal/adapters/outbound/fundsquerysource"
	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	fundsquerysourceapp "analytix.local/runtime-go/internal/app/fundsquerysource"
	nativecomponentapp "analytix.local/runtime-go/internal/app/nativecomponent"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	mcp "analytix.local/runtime-go/internal/mcp"
	casecontextport "analytix.local/runtime-go/internal/ports/casecontext"
	caseentityport "analytix.local/runtime-go/internal/ports/caseentity"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
)

// runtimeFundsAccountFlowCompositionV1 is an additive capability dependency
// bundle. An empty value disables only the protected account-flow tool; it
// never changes the ordinary Agent, thread, shell, file, Todo, or subagent
// surface.
type runtimeFundsAccountFlowCompositionV1 struct {
	localDisplaySource      *fundsquerysourceapp.Service
	useCurrentSource        nativecomponentapp.UseCurrentAccountFlowSource
	useCurrentIngressSource nativecomponentapp.UseCurrentAccountIngressSource
	useCurrentLocalDisplay  func(
		context.Context,
		string,
		string,
		string,
		func(context.Context, domainfundsquerysource.DescriptorV1, fundsquerysourceport.ExactReadLease) error,
	) error
	useRetainedAcceptedSlotDisplay func(
		context.Context,
		domainsecurity.TurnSecurityContext,
		domainsecurity.TurnSecurityContext,
		[]domainevidence.AcceptedSlotSourceBindingV1,
		string,
		func(string) error,
	) error
	resolveSubject      nativecomponentapp.ResolveAccountFlowSubject
	resolveCounterparty nativecomponentapp.ResolveAccountFlowCounterparty
	caseEntities        *caseentityapp.Service
}

func (composition runtimeFundsAccountFlowCompositionV1) available() bool {
	return composition.useCurrentSource != nil && composition.useCurrentIngressSource != nil &&
		composition.resolveSubject != nil &&
		composition.resolveCounterparty != nil
}

func (composition runtimeFundsAccountFlowCompositionV1) applyToRuntimeAuthorityV1(
	dependencies *nativecomponentapp.RuntimeAuthorityDependencies,
	validateCallbackCurrent nativecomponentapp.ValidateCurrentAccountFlowCallback,
	validateOuterGrant nativecomponentapp.ValidateCurrentAccountFlowOuterGrant,
) bool {
	if dependencies == nil || !composition.available() || validateCallbackCurrent == nil ||
		validateOuterGrant == nil {
		return false
	}
	dependencies.UseCurrentAccountFlowSource = composition.useCurrentSource
	dependencies.UseCurrentAccountIngressSource = composition.useCurrentIngressSource
	dependencies.ResolveAccountFlowSubject = composition.resolveSubject
	dependencies.ResolveAccountFlowCounterparty = composition.resolveCounterparty
	dependencies.ValidateCurrentAccountFlowCallback = validateCallbackCurrent
	dependencies.ValidateCurrentAccountFlowOuterGrant = validateOuterGrant
	return true
}

func runtimeAccountIngressResolverV1(
	authority *nativecomponentapp.RuntimeAuthority,
) caseentityapp.ResolveAccountIngressCandidatesV1 {
	if authority == nil || !authority.AccountIngressAvailable() {
		return nil
	}
	return func(
		ctx context.Context,
		securityContext domainsecurity.TurnSecurityContext,
		candidates caseentityapp.AccountIngressCandidateBatchV1,
	) (caseentityapp.AccountIngressResolutionBatchV1, error) {
		if candidates.CandidateCountV1() == 0 ||
			candidates.CandidateCountV1() > domainnative.AccountIngressResolutionMaximumCandidatesV1 {
			return caseentityapp.AccountIngressResolutionBatchV1{}, errors.New("account ingress candidate batch is invalid")
		}
		var nativeResult domainnative.ResolveAccountIngressResultV1
		var sourceDescriptor domainfundsquerysource.DescriptorV1
		if err := candidates.UseExactV1(func(values []string) error {
			if len(values) != int(candidates.CandidateCountV1()) {
				return errors.New("account ingress candidate batch changed")
			}
			nativeCandidates := make([]domainnative.ResolveAccountIngressCandidateInputV1, len(values))
			for index, value := range values {
				candidate, err := domainnative.NewResolveAccountIngressCandidateInputV1(uint32(index), value)
				if err != nil {
					return errors.New("account ingress candidate is invalid")
				}
				nativeCandidates[index] = candidate
			}
			var err error
			nativeResult, sourceDescriptor, err = authority.ResolveAccountIngress(
				ctx,
				nativecomponentapp.NewResolveAccountIngressInputV1(
					securityContext,
					nativeCandidates,
				),
			)
			return err
		}); err != nil {
			return caseentityapp.AccountIngressResolutionBatchV1{}, err
		}
		if nativeResult.Provenance.DatasetSnapshotID != securityContext.DatasetSnapshotID ||
			nativeResult.Provenance.ContextEpoch != securityContext.ContextEpoch ||
			nativeResult.Provenance.ContextDigest != securityContext.ContextDigest ||
			nativeResult.Provenance.CaseBindingHash != securityContext.CaseBindingHash ||
			len(nativeResult.Resolutions) != int(candidates.CandidateCountV1()) {
			return caseentityapp.AccountIngressResolutionBatchV1{}, errors.New("account ingress result authority changed")
		}
		return caseentityapp.NewAccountIngressResolutionBatchV1(
			securityContext,
			sourceDescriptor,
			nativeResult,
		)
	}
}

func runtimeFundsAccountFlowExecutorV1(
	authority *nativecomponentapp.RuntimeAuthority,
) mcp.AccountFlowExecutor {
	if authority == nil || !authority.AccountFlowsAvailable() {
		return nil
	}
	return func(
		ctx context.Context,
		input mcp.AccountFlowExecutionInput,
		consumeSummary domainnative.AccountFlowHostEvidenceSummaryConsumerV1,
		consumeRow domainnative.AccountFlowHostEvidenceRowConsumerV1,
	) (domainnative.AccountFlowProviderSemanticResultV1, error) {
		return authority.AnalyzeAccountFlows(
			ctx,
			nativecomponentapp.AnalyzeAccountFlowsInput{
				ExecuteInput: nativecomponentapp.ExecuteInput{
					ThreadID: input.ThreadID,
					TurnID:   input.TurnID,
					GrantID:  input.GrantID,
				},
				Intent: input.Intent,
			},
			func(projection domainnative.AccountFlowHostEvidenceProjectionV1) error {
				return projection.UseExactV1(consumeSummary, consumeRow)
			},
		)
	}
}

func composeRuntimeFundsAccountFlowV1(
	userDataRoot string,
	keyed caseentityapp.KeyedPayloadDigester,
	store caseentityport.Store,
	datasetAuthority datasetsnapshotport.CurrentAuthorityV2,
	bindingObserver casecontextport.Observer,
	materials datasetsnapshotport.AdmissionMaterialReaderV2,
	validateCurrent func(context.Context, domainsecurity.TurnSecurityContext) error,
) runtimeFundsAccountFlowCompositionV1 {
	if strings.TrimSpace(userDataRoot) == "" || keyed == nil || store == nil ||
		datasetAuthority == nil || bindingObserver == nil || materials == nil ||
		validateCurrent == nil {
		return runtimeFundsAccountFlowCompositionV1{}
	}
	caseEntities := caseentityapp.NewPersistentService(
		keyed,
		store,
		datasetAuthority,
		bindingObserver,
		validateCurrent,
	)
	hostSource, err := fundsquerysourceadapter.NewHostExactSource(userDataRoot)
	if err != nil {
		return runtimeFundsAccountFlowCompositionV1{}
	}
	querySource, err := fundsquerysourceapp.NewService(
		datasetAuthority,
		bindingObserver,
		materials,
		hostSource,
	)
	if err != nil {
		return runtimeFundsAccountFlowCompositionV1{}
	}
	return runtimeFundsAccountFlowCompositionV1{
		localDisplaySource:             querySource,
		useCurrentSource:               querySource.UseCurrentAccountFlow,
		useCurrentLocalDisplay:         querySource.UseCurrentLocalDisplay,
		useRetainedAcceptedSlotDisplay: querySource.UseRetainedAcceptedSlotDisplay,
		useCurrentIngressSource: func(
			ctx context.Context,
			securityContext domainsecurity.TurnSecurityContext,
			use func(
				context.Context,
				domainfundsquerysource.DescriptorV1,
				fundsquerysourceport.ExactReadLease,
			) error,
		) error {
			return querySource.UseCurrent(
				ctx,
				securityContext,
				func(
					sourceCtx context.Context,
					descriptor domainfundsquerysource.DescriptorV1,
					source fundsquerysourceport.ExactReadLease,
					_ domainnative.AccountFlowSourceRowResolverV1,
				) error {
					return use(sourceCtx, descriptor, source)
				},
			)
		},
		resolveSubject:      nativecomponentapp.NewPersistentAccountFlowSubjectResolver(caseEntities),
		resolveCounterparty: nativecomponentapp.NewPersistentAccountFlowCounterpartyResolver(caseEntities),
		caseEntities:        caseEntities,
	}
}
