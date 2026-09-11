package nativecomponent

import (
	"context"
	"errors"
	"time"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
)

const accountIngressResolutionExecutionLimitV1 = 10 * time.Second

var ErrAccountIngressResolutionExecutionFailed = errors.New(
	"native_component_account_ingress_resolution_failed",
)

// UseCurrentAccountIngressSource is composed from the existing funds query
// source service. It must own the complete DSV2 WithCurrentSelectionV2,
// UseExact, exact-source and postcheck lifecycle and invoke use exactly once.
// The callback has no source-row resolver because the fixed native operation
// proves its source acct_no witness internally and returns no raw source row.
type UseCurrentAccountIngressSource func(
	context.Context,
	domainsecurity.TurnSecurityContext,
	func(
		context.Context,
		domainfundsquerysource.DescriptorV1,
		fundsquerysourceport.ExactReadLease,
	) error,
) error

// ResolveAccountIngressInputV1 binds a private candidate batch to the exact
// current case turn. Candidate values remain opaque domain carriers and cannot
// be supplied through a provider, MCP, HTTP, SSE, history, or renderer schema.
type ResolveAccountIngressInputV1 struct {
	SecurityContext domainsecurity.TurnSecurityContext
	Candidates      []domainnative.ResolveAccountIngressCandidateInputV1
}

func NewResolveAccountIngressInputV1(
	securityContext domainsecurity.TurnSecurityContext,
	candidates []domainnative.ResolveAccountIngressCandidateInputV1,
) ResolveAccountIngressInputV1 {
	return ResolveAccountIngressInputV1{
		SecurityContext: securityContext,
		Candidates: append(
			[]domainnative.ResolveAccountIngressCandidateInputV1(nil),
			candidates...,
		),
	}
}

func (service *Service) accountIngressResolutionAvailable() bool {
	return service != nil && service.Available() &&
		!dependencyIsNil(service.dependencies.AccountIngressRunner) &&
		service.dependencies.UseCurrentAccountIngressSource != nil
}

// ResolveAccountIngress is a top-level pre-compilation operation: callers must
// invoke it before entering any other effect lease or DSV2 exact-use callback.
// It executes one bounded private batch under exactly one effect lease and one
// exact DSV2 source callback, then returns a closed safe result that a later
// compiler step can consume without reacquiring snapshot authority. It
// revalidates live authority before, during, and after the exact source use.
// Ambiguous or integrity-failed native dispositions are represented by the
// protocol but never returned as a usable result; verified not_found remains
// available so the caller can safely withhold an unresolved account-shaped
// prompt span.
func (service *Service) ResolveAccountIngress(
	ctx context.Context,
	input ResolveAccountIngressInputV1,
) (
	domainnative.ResolveAccountIngressResultV1,
	domainfundsquerysource.DescriptorV1,
	error,
) {
	zero := domainnative.ResolveAccountIngressResultV1{}
	zeroDescriptor := domainfundsquerysource.DescriptorV1{}
	if !service.accountIngressResolutionAvailable() {
		return zero, zeroDescriptor, ErrUnavailable
	}
	if ctx == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.SecurityContext) != nil ||
		len(input.Candidates) == 0 ||
		len(input.Candidates) > domainnative.AccountIngressResolutionMaximumCandidatesV1 {
		return zero, zeroDescriptor, ErrRequestInvalid
	}
	if err := ctx.Err(); err != nil {
		return zero, zeroDescriptor, err
	}
	input.Candidates = append(
		[]domainnative.ResolveAccountIngressCandidateInputV1(nil),
		input.Candidates...,
	)
	if err := service.dependencies.LiveAuthority.ValidateCurrent(ctx, input.SecurityContext); err != nil {
		if contextErr := healthContextError(ctx, err); contextErr != nil {
			return zero, zeroDescriptor, contextErr
		}
		return zero, zeroDescriptor, ErrAuthorityInvalid
	}

	effectCtx, release, err := service.dependencies.AcquireEffect(ctx, input.SecurityContext)
	if err != nil || effectCtx == nil || release == nil {
		if release != nil {
			release()
		}
		if contextErr := healthContextError(ctx, err); contextErr != nil {
			return zero, zeroDescriptor, contextErr
		}
		return zero, zeroDescriptor, ErrAuthorityInvalid
	}
	defer release()
	if err := service.dependencies.LiveAuthority.ValidateCurrent(effectCtx, input.SecurityContext); err != nil {
		if contextErr := healthContextError(effectCtx, err); contextErr != nil {
			return zero, zeroDescriptor, contextErr
		}
		return zero, zeroDescriptor, ErrAuthorityInvalid
	}

	var provisional domainnative.ResolveAccountIngressResultV1
	var provisionalDescriptor domainfundsquerysource.DescriptorV1
	barrier := newCallbackUseBarrier()
	var attempts uint32
	var completed bool
	useErr := func() error {
		defer func() { attempts, completed = barrier.close() }()
		return service.dependencies.UseCurrentAccountIngressSource(
			effectCtx,
			input.SecurityContext,
			func(
				sourceCtx context.Context,
				descriptor domainfundsquerysource.DescriptorV1,
				source fundsquerysourceport.ExactReadLease,
			) error {
				if sourceCtx == nil || sourceCtx.Err() != nil ||
					exactReadLeaseDependencyIsNilV1(source) ||
					!accountIngressDescriptorMatchesV1(descriptor, input.SecurityContext) {
					return ErrAuthorityInvalid
				}
				if !barrier.begin() {
					return ErrAuthorityInvalid
				}
				succeeded := false
				defer func() { barrier.end(succeeded) }()
				arguments, argumentsErr := domainnative.NewResolveAccountIngressArgumentsV1(
					domainnative.ResolveAccountIngressArgumentsInputV1{
						CaseID:                               input.SecurityContext.CaseID,
						DatasetSnapshotID:                    input.SecurityContext.DatasetSnapshotID,
						ContextEpoch:                         input.SecurityContext.ContextEpoch,
						ContextDigest:                        input.SecurityContext.ContextDigest,
						CaseBindingHash:                      input.SecurityContext.CaseBindingHash,
						ExpectedProducerContentID:            descriptor.FundsProducerContentID,
						ExpectedProducerManifestSHA256:       descriptor.FundsProducerContentManifestSHA256,
						ExpectedDuckDBContentSnapshotDigest:  descriptor.DuckDBContentSnapshotDigest,
						ExpectedDuckDBSnapshotManifestSHA256: descriptor.DuckDBSnapshotManifestSHA256,
						ExpectedMaterializationIdentity:      descriptor.MaterializationIdentity,
						Candidates:                           input.Candidates,
					},
				)
				if argumentsErr != nil ||
					domainnative.ValidateResolveAccountIngressArgumentsAuthorityV1(
						arguments,
						input.SecurityContext,
						descriptor,
					) != nil {
					return ErrAuthorityInvalid
				}
				if liveErr := service.dependencies.LiveAuthority.ValidateCurrent(
					sourceCtx,
					input.SecurityContext,
				); liveErr != nil {
					if contextErr := healthContextError(sourceCtx, liveErr); contextErr != nil {
						return contextErr
					}
					return ErrAuthorityInvalid
				}
				runCtx, cancel := context.WithTimeout(
					sourceCtx,
					accountIngressResolutionExecutionLimitV1,
				)
				result, runErr := service.dependencies.AccountIngressRunner.ResolveAccountIngress(
					runCtx,
					input.SecurityContext,
					arguments,
					descriptor,
					source,
				)
				cancel()
				if contextErr := healthContextError(sourceCtx, runErr); contextErr != nil {
					return contextErr
				}
				if runErr != nil || domainnative.ValidateResolveAccountIngressResultV1(result, arguments) != nil {
					return ErrAccountIngressResolutionExecutionFailed
				}
				if liveErr := service.dependencies.LiveAuthority.ValidateCurrent(
					sourceCtx,
					input.SecurityContext,
				); liveErr != nil {
					if contextErr := healthContextError(sourceCtx, liveErr); contextErr != nil {
						return contextErr
					}
					return ErrAuthorityInvalid
				}
				if !barrier.commit(func() {
					provisional = result
					provisionalDescriptor = descriptor
				}) {
					return ErrAuthorityInvalid
				}
				succeeded = true
				return nil
			},
		)
	}()
	if contextErr := healthContextError(effectCtx, useErr); contextErr != nil {
		return zero, zeroDescriptor, contextErr
	}
	if useErr != nil || attempts != 1 || !completed {
		if errors.Is(useErr, ErrAccountIngressResolutionExecutionFailed) {
			return zero, zeroDescriptor, ErrAccountIngressResolutionExecutionFailed
		}
		return zero, zeroDescriptor, ErrAuthorityInvalid
	}
	if err := service.dependencies.LiveAuthority.ValidateCurrent(effectCtx, input.SecurityContext); err != nil {
		if contextErr := healthContextError(effectCtx, err); contextErr != nil {
			return zero, zeroDescriptor, contextErr
		}
		return zero, zeroDescriptor, ErrAuthorityInvalid
	}
	if domainnative.ResolveAccountIngressResultHasFailClosedDispositionV1(provisional) {
		return zero, zeroDescriptor, ErrAccountIngressResolutionExecutionFailed
	}
	if domainnative.ValidateResolveAccountIngressResultAuthorityV1(
		provisional,
		input.SecurityContext,
		provisionalDescriptor,
	) != nil {
		return zero, zeroDescriptor, ErrAuthorityInvalid
	}
	return provisional, provisionalDescriptor, nil
}

func accountIngressDescriptorMatchesV1(
	descriptor domainfundsquerysource.DescriptorV1,
	securityContext domainsecurity.TurnSecurityContext,
) bool {
	return domainfundsquerysource.ValidateDescriptorV1(descriptor) == nil &&
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) == nil &&
		descriptor.CaseID == securityContext.CaseID &&
		descriptor.DatasetSnapshotID == securityContext.DatasetSnapshotID &&
		descriptor.SourceManifestHash == securityContext.SourceManifestHash &&
		descriptor.CaseBindingHash == securityContext.CaseBindingHash &&
		descriptor.BindingObservationDigest == securityContext.PublicationPolicy.BindingObservationDigest &&
		domainfundsquerysource.QueryProfileSupportsAccountIngressResolutionV1(
			descriptor.QueryProfileDigest,
		)
}

func exactReadLeaseDependencyIsNilV1(source fundsquerysourceport.ExactReadLease) bool {
	return dependencyIsNil(source)
}
