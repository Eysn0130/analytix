package fundsquerysource

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	casecontextport "analytix.local/runtime-go/internal/ports/casecontext"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
)

// Service projects one freshly observed FPC1 DSV2 selection into the fixed,
// path-free account-flow query descriptor. It owns no source registry or
// storage authority: the host source remains usable only while the existing
// DSV2 capability is live.
type Service struct {
	datasetAuthority datasetsnapshotport.CurrentAuthorityV2
	displayAuthority datasetsnapshotport.AuthorityV2
	bindingObserver  casecontextport.Observer
	materials        datasetsnapshotport.AdmissionMaterialReaderV2
	hostSource       fundsquerysourceport.HostExactSource
}

// accountFlowPostNativeCurrentnessV1 is the process-local, one-shot bridge
// from a live funds source callback back to its already-issued exact DSV2
// selection capability. Copies of its use closure share this state. It owns no
// registry, grant or durable authority and is never serializable.
type accountFlowPostNativeCurrentnessV1 struct {
	mu              sync.Mutex
	condition       *sync.Cond
	active          bool
	used            bool
	inFlight        bool
	selection       datasetsnapshotport.CurrentSelectionV2
	capability      datasetsnapshotport.CurrentSelectionCapabilityV2
	securityContext domainsecurity.TurnSecurityContext
	descriptor      domainfundsquerysource.DescriptorV1
}

type accountFlowPostNativeSelectionCapabilityV1 interface {
	UsePostNativeExact(
		datasetsnapshotport.CurrentSelectionV2,
		domainsecurity.TurnSecurityContext,
		func(context.Context) error,
	) error
}

func newAccountFlowPostNativeCurrentnessV1(
	selection datasetsnapshotport.CurrentSelectionV2,
	capability datasetsnapshotport.CurrentSelectionCapabilityV2,
	securityContext domainsecurity.TurnSecurityContext,
	descriptor domainfundsquerysource.DescriptorV1,
) *accountFlowPostNativeCurrentnessV1 {
	currentness := &accountFlowPostNativeCurrentnessV1{
		active: true, selection: selection, capability: capability,
		securityContext: securityContext, descriptor: descriptor,
	}
	currentness.condition = sync.NewCond(&currentness.mu)
	return currentness
}

func (currentness *accountFlowPostNativeCurrentnessV1) use(
	validationContext context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	descriptor domainfundsquerysource.DescriptorV1,
	continueValidation func(context.Context) error,
) error {
	if currentness == nil {
		return errors.Join(
			fundsquerysourceport.ErrUnavailable,
			errors.New("funds post-native currentness capability is unavailable"),
		)
	}
	currentness.mu.Lock()
	if !currentness.active || currentness.used || currentness.inFlight ||
		dependencyIsNilV1(currentness.capability) {
		currentness.mu.Unlock()
		return errors.Join(
			fundsquerysourceport.ErrUnavailable,
			errors.New("funds post-native currentness capability is inactive"),
		)
	}
	currentness.used = true
	if validationContext == nil || continueValidation == nil {
		currentness.mu.Unlock()
		return errors.Join(
			fundsquerysourceport.ErrUnavailable,
			errors.New("funds post-native currentness input is unavailable"),
		)
	}
	if securityContext != currentness.securityContext || descriptor != currentness.descriptor {
		currentness.mu.Unlock()
		return errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds post-native currentness binding changed"),
		)
	}
	currentness.inFlight = true
	selection := currentness.selection
	capability := currentness.capability
	currentness.mu.Unlock()
	defer func() {
		currentness.mu.Lock()
		currentness.inFlight = false
		if currentness.condition != nil {
			currentness.condition.Broadcast()
		}
		currentness.mu.Unlock()
	}()

	if err := validationContext.Err(); err != nil {
		return err
	}
	var callbacks atomic.Uint32
	useExact := capability.UseExact
	if postNative, ok := capability.(accountFlowPostNativeSelectionCapabilityV1); ok {
		useExact = postNative.UsePostNativeExact
	}
	err := useExact(
		selection,
		securityContext,
		func(leaseContext context.Context) error {
			if leaseContext == nil || leaseContext.Err() != nil || !callbacks.CompareAndSwap(0, 1) {
				return errors.Join(
					fundsquerysourceport.ErrUnavailable,
					errors.New("funds post-native currentness callback is unavailable"),
				)
			}
			boundedContext, cancel := context.WithCancel(validationContext)
			stop := context.AfterFunc(leaseContext, cancel)
			defer func() {
				stop()
				cancel()
			}()
			if err := boundedContext.Err(); err != nil {
				return err
			}
			return continueValidation(boundedContext)
		},
	)
	if err != nil {
		return err
	}
	if callbacks.Load() != 1 {
		return errors.Join(
			fundsquerysourceport.ErrUnavailable,
			errors.New("funds post-native currentness callback was not invoked"),
		)
	}
	return nil
}

func (currentness *accountFlowPostNativeCurrentnessV1) close() {
	if currentness == nil {
		return
	}
	currentness.mu.Lock()
	currentness.active = false
	for currentness.inFlight {
		currentness.condition.Wait()
	}
	currentness.capability = nil
	currentness.mu.Unlock()
}

func NewService(
	datasetAuthority datasetsnapshotport.CurrentAuthorityV2,
	bindingObserver casecontextport.Observer,
	materials datasetsnapshotport.AdmissionMaterialReaderV2,
	hostSource fundsquerysourceport.HostExactSource,
) (*Service, error) {
	if dependencyIsNilV1(datasetAuthority) || dependencyIsNilV1(bindingObserver) ||
		dependencyIsNilV1(materials) || dependencyIsNilV1(hostSource) {
		return nil, errors.New("funds query source service configuration is invalid")
	}
	return &Service{
		datasetAuthority: datasetAuthority,
		displayAuthority: func() datasetsnapshotport.AuthorityV2 {
			authority, _ := datasetAuthority.(datasetsnapshotport.AuthorityV2)
			return authority
		}(),
		bindingObserver: bindingObserver,
		materials:       materials,
		hostSource:      hostSource,
	}, nil
}

// UseCurrentLocalDisplay is the non-Agent, non-publication exact-read seam for
// Direct Source Preview. It resolves only the currently witnessed case
// snapshot, opens the existing path-free host source once, and re-resolves the
// same witnessed snapshot after use. A concurrent case or snapshot change
// discards the typed result before it can reach the renderer.
func (service *Service) UseCurrentLocalDisplay(
	ctx context.Context,
	workspaceRealPath string,
	tenantID string,
	userID string,
	use func(context.Context, domainfundsquerysource.DescriptorV1, fundsquerysourceport.ExactReadLease) error,
) error {
	workspaceRealPath = strings.TrimSpace(workspaceRealPath)
	tenantID = strings.TrimSpace(tenantID)
	userID = strings.TrimSpace(userID)
	if service == nil || ctx == nil || use == nil || workspaceRealPath == "" ||
		tenantID == "" || userID == "" || dependencyIsNilV1(service.displayAuthority) ||
		dependencyIsNilV1(service.bindingObserver) || dependencyIsNilV1(service.hostSource) {
		return errors.Join(fundsquerysourceport.ErrUnavailable, errors.New("funds local display source is unavailable"))
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	observation, err := service.bindingObserver.Observe(workspaceRealPath)
	if err != nil || observation.WorkspaceRealPath != workspaceRealPath ||
		domainsecurity.ValidateCaseBindingObservationV1(observation) != nil ||
		observation.State != domainsecurity.CaseBindingStateValid {
		return errors.Join(
			fundsquerysourceport.ErrUnavailable,
			errors.New("funds local display case binding is unavailable"),
			err,
		)
	}
	resolveInput := datasetsnapshotport.ResolveInputV2{
		TenantID: tenantID, UserID: userID, Observation: observation,
	}
	before, err := service.displayAuthority.ResolveWitnessedV2(ctx, resolveInput)
	if err != nil {
		return errors.Join(fundsquerysourceport.ErrUnavailable, errors.New("funds local display snapshot is unavailable"), err)
	}
	descriptor, err := DescriptorForCurrentLocalDisplayV1(before, tenantID, userID, observation)
	if err != nil {
		return err
	}
	if err := service.confirmLocalDisplaySelectionV1(ctx, resolveInput, before); err != nil {
		return err
	}
	var callbacks atomic.Uint32
	var active atomic.Bool
	active.Store(true)
	err = service.hostSource.WithExact(
		ctx,
		descriptor,
		func(sourceContext context.Context, lease fundsquerysourceport.ExactReadLease) error {
			if !active.Load() || !callbacks.CompareAndSwap(0, 1) || sourceContext == nil ||
				sourceContext.Err() != nil || dependencyIsNilV1(lease) {
				return errors.Join(fundsquerysourceport.ErrUnavailable, errors.New("funds local display exact lease is unavailable"))
			}
			return use(sourceContext, descriptor, lease)
		},
	)
	active.Store(false)
	if err != nil {
		return err
	}
	if callbacks.Load() != 1 {
		return errors.Join(fundsquerysourceport.ErrUnavailable, errors.New("funds local display exact callback was not invoked"))
	}
	return service.confirmLocalDisplaySelectionV1(ctx, resolveInput, before)
}

// ResolveCurrentLocalDisplay reuses the same witnessed DSV2 derivation for a
// metadata-only currentness check. It opens no DuckDB and returns no exact
// source lease, path, row, grant, or publication authority.
func (service *Service) ResolveCurrentLocalDisplay(
	ctx context.Context,
	workspaceRealPath string,
	tenantID string,
	userID string,
) (domainfundsquerysource.DescriptorV1, error) {
	workspaceRealPath = strings.TrimSpace(workspaceRealPath)
	tenantID = strings.TrimSpace(tenantID)
	userID = strings.TrimSpace(userID)
	if service == nil || ctx == nil || workspaceRealPath == "" || tenantID == "" || userID == "" ||
		dependencyIsNilV1(service.displayAuthority) || dependencyIsNilV1(service.bindingObserver) {
		return domainfundsquerysource.DescriptorV1{}, errors.Join(
			fundsquerysourceport.ErrUnavailable, errors.New("funds local display descriptor is unavailable"),
		)
	}
	if err := ctx.Err(); err != nil {
		return domainfundsquerysource.DescriptorV1{}, err
	}
	observation, err := service.bindingObserver.Observe(workspaceRealPath)
	if err != nil || observation.WorkspaceRealPath != workspaceRealPath ||
		domainsecurity.ValidateCaseBindingObservationV1(observation) != nil ||
		observation.State != domainsecurity.CaseBindingStateValid {
		return domainfundsquerysource.DescriptorV1{}, errors.Join(
			fundsquerysourceport.ErrUnavailable, errors.New("funds local display case binding is unavailable"), err,
		)
	}
	resolveInput := datasetsnapshotport.ResolveInputV2{
		TenantID: tenantID, UserID: userID, Observation: observation,
	}
	resolved, err := service.displayAuthority.ResolveWitnessedV2(ctx, resolveInput)
	if err != nil {
		return domainfundsquerysource.DescriptorV1{}, errors.Join(
			fundsquerysourceport.ErrUnavailable, errors.New("funds local display snapshot is unavailable"), err,
		)
	}
	descriptor, err := DescriptorForCurrentLocalDisplayV1(resolved, tenantID, userID, observation)
	if err != nil {
		return domainfundsquerysource.DescriptorV1{}, err
	}
	if err := service.confirmLocalDisplaySelectionV1(ctx, resolveInput, resolved); err != nil {
		return domainfundsquerysource.DescriptorV1{}, err
	}
	return descriptor, nil
}

// UseRetainedAcceptedSlotDisplay reuses the current DSV2 witness to resolve
// one accepted result's original immutable source file/row/field lineage. It
// never opens or scans the current analytical database and never treats the
// canonical comparison value as display material.
func (service *Service) UseRetainedAcceptedSlotDisplay(
	ctx context.Context,
	active domainsecurity.TurnSecurityContext,
	historical domainsecurity.TurnSecurityContext,
	bindings []domainevidence.AcceptedSlotSourceBindingV1,
	expectedCanonicalEntity string,
	use func(string) error,
) error {
	if service == nil || ctx == nil || use == nil {
		return errors.Join(fundsquerysourceport.ErrUnavailable, errors.New("retained accepted slot source is unavailable"))
	}
	reader, ok := service.datasetAuthority.(datasetsnapshotport.RetainedSelectionReaderV2)
	if !ok || dependencyIsNilV1(reader) ||
		dependencyIsNilV1(service.bindingObserver) || dependencyIsNilV1(service.materials) ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(active) != nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(historical) != nil ||
		!retainedDisplayContextsShareCaseV1(active, historical) || len(bindings) == 0 {
		return errors.Join(fundsquerysourceport.ErrUnavailable, errors.New("retained accepted slot source is unavailable"))
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	observation, err := service.bindingObserver.Observe(active.WorkspaceRealPath)
	if err != nil || !observationMatchesContextV1(observation, active) {
		return errors.Join(fundsquerysourceport.ErrUnavailable, errors.New("retained accepted slot case binding is unavailable"), err)
	}
	input := datasetsnapshotport.RetainedSelectionInputV2{
		CurrentResolveInput: datasetsnapshotport.ResolveInputV2{
			TenantID: active.TenantID, UserID: active.UserID,
			Observation: observation, ExpectedDatasetSnapshotID: active.DatasetSnapshotID,
		},
		CurrentSecurityContext:     active,
		RetainedDatasetSnapshotID:  historical.DatasetSnapshotID,
		RetainedSourceManifestHash: historical.SourceManifestHash,
	}
	var callbacks atomic.Uint32
	err = reader.WithRetainedSelectionV2(
		ctx,
		input,
		func(leaseContext context.Context, retained datasetsnapshotport.RetainedSelectionV2) error {
			if leaseContext == nil || leaseContext.Err() != nil || !callbacks.CompareAndSwap(0, 1) ||
				!retainedDisplaySelectionMatchesV1(retained, active, historical) {
				return errors.Join(fundsquerysourceport.ErrMismatch, errors.New("retained accepted slot selection is invalid"))
			}
			resolver := newAccountFlowSourceRowResolverV1(
				leaseContext, retained.Snapshot.Manifest, historical, service.materials,
			)
			defer resolver.close()
			return resolver.useExactAcceptedSlotEntityV1(bindings, expectedCanonicalEntity, use)
		},
	)
	if err != nil || callbacks.Load() != 1 {
		return errors.Join(fundsquerysourceport.ErrUnavailable, errors.New("retained accepted slot exact use failed"), err)
	}
	after, err := service.bindingObserver.Observe(active.WorkspaceRealPath)
	if err != nil || after != observation || !observationMatchesContextV1(after, active) {
		return errors.Join(fundsquerysourceport.ErrMismatch, errors.New("retained accepted slot case binding changed"), err)
	}
	return nil
}

func retainedDisplayContextsShareCaseV1(
	active domainsecurity.TurnSecurityContext,
	historical domainsecurity.TurnSecurityContext,
) bool {
	return active.TenantID == historical.TenantID && active.UserID == historical.UserID &&
		active.WorkspaceRealPath == historical.WorkspaceRealPath && active.CaseID == historical.CaseID &&
		active.CaseBindingHash == historical.CaseBindingHash
}

func retainedDisplaySelectionMatchesV1(
	retained datasetsnapshotport.RetainedSelectionV2,
	active domainsecurity.TurnSecurityContext,
	historical domainsecurity.TurnSecurityContext,
) bool {
	current := retained.Current.Snapshot
	original := retained.Snapshot
	currentBinding := current.Record.Binding
	originalBinding := original.Record.Binding
	return current.Record.DatasetSnapshotID == active.DatasetSnapshotID &&
		current.Record.SourceManifestHash == active.SourceManifestHash &&
		current.Manifest.SourceManifestHash == active.SourceManifestHash &&
		current.Record.Binding == current.Manifest.Binding &&
		currentBinding.TenantID == active.TenantID && currentBinding.UserID == active.UserID &&
		currentBinding.WorkspaceRealPath == active.WorkspaceRealPath &&
		currentBinding.CaseID == active.CaseID && currentBinding.CaseBindingHash == active.CaseBindingHash &&
		currentBinding.BindingObservationDigest == active.PublicationPolicy.BindingObservationDigest &&
		original.Record.DatasetSnapshotID == historical.DatasetSnapshotID &&
		original.Record.SourceManifestHash == historical.SourceManifestHash &&
		original.Manifest.SourceManifestHash == historical.SourceManifestHash &&
		original.Record.Binding == original.Manifest.Binding &&
		originalBinding.TenantID == historical.TenantID && originalBinding.UserID == historical.UserID &&
		originalBinding.WorkspaceRealPath == historical.WorkspaceRealPath &&
		originalBinding.CaseID == historical.CaseID && originalBinding.CaseBindingHash == historical.CaseBindingHash &&
		originalBinding.BindingObservationDigest == historical.PublicationPolicy.BindingObservationDigest
}

func (service *Service) confirmLocalDisplaySelectionV1(
	ctx context.Context,
	input datasetsnapshotport.ResolveInputV2,
	expected datasetsnapshotport.ResolvedSnapshotV2,
) error {
	currentObservation, err := service.bindingObserver.Observe(input.Observation.WorkspaceRealPath)
	if err != nil || currentObservation != input.Observation {
		return errors.Join(fundsquerysourceport.ErrMismatch, errors.New("funds local display case binding changed"), err)
	}
	current, err := service.displayAuthority.ResolveWitnessedV2(ctx, input)
	if err != nil || !reflect.DeepEqual(current, expected) {
		return errors.Join(fundsquerysourceport.ErrMismatch, errors.New("funds local display snapshot changed"), err)
	}
	return nil
}

// UseCurrent preserves the existing exact-source seam for account ingress and
// source-row callers that do not execute a native account-flow operation.
func (service *Service) UseCurrent(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	use func(
		context.Context,
		domainfundsquerysource.DescriptorV1,
		fundsquerysourceport.ExactReadLease,
		domainnative.AccountFlowSourceRowResolverV1,
	) error,
) error {
	if use == nil {
		return service.UseCurrentAccountFlow(ctx, securityContext, nil)
	}
	return service.UseCurrentAccountFlow(
		ctx,
		securityContext,
		func(
			sourceContext context.Context,
			descriptor domainfundsquerysource.DescriptorV1,
			lease fundsquerysourceport.ExactReadLease,
			resolveSourceRow domainnative.AccountFlowSourceRowResolverV1,
			_ func(
				context.Context,
				domainsecurity.TurnSecurityContext,
				domainfundsquerysource.DescriptorV1,
				func(context.Context) error,
			) error,
		) error {
			return use(sourceContext, descriptor, lease, resolveSourceRow)
		},
	)
}

// UseCurrentAccountFlow is the additive native account-flow composition seam.
// It adds one callback-scoped post-native currentness closure without changing
// the existing ingress/source-row API or the permanent ordinary Agent base.
func (service *Service) UseCurrentAccountFlow(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	use func(
		context.Context,
		domainfundsquerysource.DescriptorV1,
		fundsquerysourceport.ExactReadLease,
		domainnative.AccountFlowSourceRowResolverV1,
		func(
			context.Context,
			domainsecurity.TurnSecurityContext,
			domainfundsquerysource.DescriptorV1,
			func(context.Context) error,
		) error,
	) error,
) error {
	if service == nil || ctx == nil || use == nil ||
		dependencyIsNilV1(service.datasetAuthority) ||
		dependencyIsNilV1(service.bindingObserver) ||
		dependencyIsNilV1(service.materials) ||
		dependencyIsNilV1(service.hostSource) {
		return errors.Join(
			fundsquerysourceport.ErrUnavailable,
			errors.New("funds query source use is unavailable"),
		)
	}
	if err := domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext); err != nil {
		return errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds query source requires a case-fact TurnSecurityContext V2"),
			err,
		)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	observation, err := service.bindingObserver.Observe(securityContext.WorkspaceRealPath)
	if err != nil {
		return errors.Join(
			fundsquerysourceport.ErrUnavailable,
			errors.New("funds query source case binding cannot be observed"),
			err,
		)
	}
	if !observationMatchesContextV1(observation, securityContext) {
		return errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds query source case binding does not match the turn"),
		)
	}

	resolveInput := datasetsnapshotport.ResolveInputV2{
		TenantID:                  securityContext.TenantID,
		UserID:                    securityContext.UserID,
		Observation:               observation,
		ExpectedDatasetSnapshotID: securityContext.DatasetSnapshotID,
	}
	var selectionCallbacks atomic.Uint32
	var selectionActive atomic.Bool
	selectionActive.Store(true)
	err = service.datasetAuthority.WithCurrentSelectionV2(
		ctx,
		resolveInput,
		securityContext,
		func(
			selection datasetsnapshotport.CurrentSelectionV2,
			capability datasetsnapshotport.CurrentSelectionCapabilityV2,
		) error {
			if !selectionActive.Load() {
				return errors.Join(
					fundsquerysourceport.ErrUnavailable,
					errors.New("funds query source dataset callback is inactive"),
				)
			}
			if !selectionCallbacks.CompareAndSwap(0, 1) {
				return errors.Join(
					fundsquerysourceport.ErrMismatch,
					errors.New("funds query source dataset callback was repeated"),
				)
			}
			if dependencyIsNilV1(capability) {
				return errors.Join(
					fundsquerysourceport.ErrUnavailable,
					errors.New("funds query source dataset capability is unavailable"),
				)
			}
			descriptor, descriptorErr := DescriptorForCurrentSelectionV1(
				selection,
				securityContext,
				observation,
			)
			if descriptorErr != nil {
				return descriptorErr
			}
			if observationErr := service.confirmObservationV1(
				securityContext,
				observation,
			); observationErr != nil {
				return observationErr
			}

			var exactCallbacks atomic.Uint32
			var exactActive atomic.Bool
			exactActive.Store(true)
			exactErr := capability.UseExact(
				selection,
				securityContext,
				func(leaseContext context.Context) error {
					if !exactActive.Load() {
						return errors.Join(
							fundsquerysourceport.ErrUnavailable,
							errors.New("funds query source DSV2 exact-use callback is inactive"),
						)
					}
					if !exactCallbacks.CompareAndSwap(0, 1) {
						return errors.Join(
							fundsquerysourceport.ErrMismatch,
							errors.New("funds query source DSV2 exact-use callback was repeated"),
						)
					}
					if leaseContext == nil || leaseContext.Err() != nil {
						return errors.Join(
							fundsquerysourceport.ErrUnavailable,
							errors.New("funds query source DSV2 lease is unavailable"),
						)
					}
					postNativeCurrentness := newAccountFlowPostNativeCurrentnessV1(
						selection,
						capability,
						securityContext,
						descriptor,
					)
					defer postNativeCurrentness.close()

					var sourceCallbacks atomic.Uint32
					var sourceActive atomic.Bool
					sourceActive.Store(true)
					sourceErr := service.hostSource.WithExact(
						leaseContext,
						descriptor,
						func(
							sourceContext context.Context,
							lease fundsquerysourceport.ExactReadLease,
						) error {
							if !sourceActive.Load() {
								return errors.Join(
									fundsquerysourceport.ErrUnavailable,
									errors.New("funds query source host callback is inactive"),
								)
							}
							if !sourceCallbacks.CompareAndSwap(0, 1) {
								return errors.Join(
									fundsquerysourceport.ErrMismatch,
									errors.New("funds query source host callback was repeated"),
								)
							}
							if sourceContext == nil || sourceContext.Err() != nil ||
								dependencyIsNilV1(lease) {
								return errors.Join(
									fundsquerysourceport.ErrUnavailable,
									errors.New("funds query source host lease is unavailable"),
								)
							}
							resolver := newAccountFlowSourceRowResolverV1(
								sourceContext,
								selection.Snapshot.Manifest,
								securityContext,
								service.materials,
							)
							defer resolver.close()
							return use(
								sourceContext,
								descriptor,
								lease,
								resolver.resolve,
								postNativeCurrentness.use,
							)
						},
					)
					sourceActive.Store(false)
					if sourceErr != nil {
						return sourceErr
					}
					if sourceCallbacks.Load() != 1 {
						return errors.Join(
							fundsquerysourceport.ErrUnavailable,
							errors.New("funds query source host callback was not invoked"),
						)
					}
					return service.confirmObservationV1(securityContext, observation)
				},
			)
			exactActive.Store(false)
			if exactErr != nil {
				return exactErr
			}
			if exactCallbacks.Load() != 1 {
				return errors.Join(
					fundsquerysourceport.ErrUnavailable,
					errors.New("funds query source DSV2 exact-use callback was not invoked"),
				)
			}
			return nil
		},
	)
	selectionActive.Store(false)
	if err != nil {
		return err
	}
	if selectionCallbacks.Load() != 1 {
		return errors.Join(
			fundsquerysourceport.ErrUnavailable,
			errors.New("funds query source dataset callback was not invoked"),
		)
	}
	return service.confirmObservationV1(securityContext, observation)
}

func (service *Service) confirmObservationV1(
	securityContext domainsecurity.TurnSecurityContext,
	expected domainsecurity.CaseBindingObservationV1,
) error {
	current, err := service.bindingObserver.Observe(securityContext.WorkspaceRealPath)
	if err != nil {
		return errors.Join(
			fundsquerysourceport.ErrUnavailable,
			errors.New("funds query source case binding cannot be re-observed"),
			err,
		)
	}
	if current != expected || !observationMatchesContextV1(current, securityContext) {
		return errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds query source case binding changed during exact use"),
		)
	}
	return nil
}

// DescriptorForCurrentSelectionV1 derives the one path-free analytical source
// descriptor for an exact current selection. It does not grant source access;
// callers must remain inside the issuing CurrentSelectionCapabilityV2 lease.
func DescriptorForCurrentSelectionV1(
	selection datasetsnapshotport.CurrentSelectionV2,
	securityContext domainsecurity.TurnSecurityContext,
	observation domainsecurity.CaseBindingObservationV1,
) (domainfundsquerysource.DescriptorV1, error) {
	snapshot := selection.Snapshot
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		!observationMatchesContextV1(observation, securityContext) ||
		datasetsnapshotport.ValidateResolvedSnapshotV2(snapshot) != nil {
		return domainfundsquerysource.DescriptorV1{}, errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds query source current DSV2 selection is invalid"),
		)
	}
	if snapshot.Manifest.ProducerContentContract !=
		domainsecurity.FundsProducerContentManifestContractV1 ||
		snapshot.FundsProducerContentV2 != (domainsecurity.FundsProducerContentManifestV2{}) {
		return domainfundsquerysource.DescriptorV1{}, errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds query source requires the FPC1 analytical producer"),
		)
	}
	if snapshot.Manifest.AnalyticalDuckDB == "" {
		return domainfundsquerysource.DescriptorV1{}, errors.Join(
			fundsquerysourceport.ErrUnavailable,
			errors.New("funds query source analytical DuckDB is not bound"),
		)
	}
	if err := validateAccountFlowTransactionPolicyV1(snapshot.Manifest); err != nil {
		return domainfundsquerysource.DescriptorV1{}, err
	}
	analytical, err := snapshot.Manifest.AnalyticalDuckDB.Values()
	if err != nil || !domainfundsquerysource.QueryProfileSupportsAccountFlowV1(
		analytical.QueryProfileDigest,
	) {
		return domainfundsquerysource.DescriptorV1{}, errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds query source analytical query profile is invalid"),
			err,
		)
	}
	producer, err := domainsecurity.ResolveFundsProducerContentBindingV2(
		snapshot.Manifest,
		snapshot.FundsProducerContent,
		snapshot.FundsProducerContentV2,
	)
	if err != nil || producer.Contract != domainsecurity.FundsProducerContentManifestContractV1 {
		return domainfundsquerysource.DescriptorV1{}, errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds query source producer binding is invalid"),
			err,
		)
	}

	binding := snapshot.Record.Binding
	if domainsecurity.ValidateDatasetSnapshotBindingKeyForObservationV1(
		binding,
		securityContext.TenantID,
		securityContext.UserID,
		observation,
	) != nil ||
		domainsecurity.ValidateDatasetSnapshotIndexRecordV2(
			selection.SelectedIndex,
			snapshot.Record,
		) != nil ||
		snapshot.Manifest.Binding != binding ||
		selection.SelectedIndex.Binding != binding ||
		selection.SelectedIndex.SnapshotRecordDigest != snapshot.Record.RecordDigest ||
		snapshot.Record.DatasetSnapshotID != securityContext.DatasetSnapshotID ||
		snapshot.Record.SourceManifestHash != securityContext.SourceManifestHash ||
		snapshot.Manifest.SourceManifestHash != securityContext.SourceManifestHash ||
		binding.WorkspaceRealPath != securityContext.WorkspaceRealPath ||
		binding.CaseID != securityContext.CaseID ||
		binding.CaseBindingHash != securityContext.CaseBindingHash ||
		binding.BindingObservationDigest != securityContext.PublicationPolicy.BindingObservationDigest ||
		producer.CaseID != securityContext.CaseID ||
		producer.ID != snapshot.Manifest.ProducerContentID ||
		producer.ManifestSHA256 != snapshot.Manifest.ProducerContentManifestSHA256 ||
		producer.ManifestByteLength != snapshot.Manifest.ProducerContentManifestByteLength {
		return domainfundsquerysource.DescriptorV1{}, errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds query source selection provenance is mismatched"),
		)
	}

	descriptor, err := domainfundsquerysource.NewDescriptorV1(
		domainfundsquerysource.DescriptorInputV1{
			SnapshotRecordDigest:     snapshot.Record.RecordDigest,
			DatasetSnapshotID:        snapshot.Record.DatasetSnapshotID,
			SourceManifestHash:       snapshot.Manifest.SourceManifestHash,
			CaseID:                   binding.CaseID,
			CaseBindingHash:          binding.CaseBindingHash,
			DatasetBindingDigest:     binding.BindingKeyDigest,
			BindingObservationDigest: binding.BindingObservationDigest,

			FundsProducerContentID:                 producer.ID,
			FundsProducerContentManifestSHA256:     producer.ManifestSHA256,
			FundsProducerContentManifestByteLength: producer.ManifestByteLength,

			DuckDBSHA256:                 analytical.DuckDBSHA256,
			DuckDBByteLength:             analytical.DuckDBByteLength,
			DuckDBContentSnapshotDigest:  analytical.DuckDBContentSnapshotDigest,
			DuckDBSnapshotManifestSHA256: analytical.DuckDBSnapshotManifestSHA256,
			MaterializationIdentity:      analytical.MaterializationIdentity,
			SchemaDigest:                 analytical.SchemaDigest,
			DatasetUTCOffsetMinutes:      analytical.DatasetUTCOffsetMinutes,
			ExpectedCurrency:             analytical.ExpectedCurrency,
			MinorUnitScale:               analytical.MinorUnitScale,
			QueryProfileDigest:           analytical.QueryProfileDigest,
		},
	)
	if err != nil {
		return domainfundsquerysource.DescriptorV1{}, errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds query source descriptor cannot be derived"),
			err,
		)
	}
	return descriptor, nil
}

// DescriptorForCurrentLocalDisplayV1 derives the same path-free immutable
// source descriptor from a freshly witnessed current DSV2 read without
// manufacturing a TurnSecurityContext. This seam is restricted to local
// typed display and accepts only the query profile that explicitly includes
// the fixed direct preview operation.
func DescriptorForCurrentLocalDisplayV1(
	snapshot datasetsnapshotport.ResolvedSnapshotV2,
	tenantID string,
	userID string,
	observation domainsecurity.CaseBindingObservationV1,
) (domainfundsquerysource.DescriptorV1, error) {
	tenantID = strings.TrimSpace(tenantID)
	userID = strings.TrimSpace(userID)
	if tenantID == "" || userID == "" ||
		domainsecurity.ValidateCaseBindingObservationV1(observation) != nil ||
		observation.State != domainsecurity.CaseBindingStateValid ||
		datasetsnapshotport.ValidateResolvedSnapshotV2(snapshot) != nil ||
		snapshot.Manifest.ProducerContentContract != domainsecurity.FundsProducerContentManifestContractV1 ||
		snapshot.FundsProducerContentV2 != (domainsecurity.FundsProducerContentManifestV2{}) ||
		snapshot.Manifest.AnalyticalDuckDB == "" || validateAccountFlowTransactionPolicyV1(snapshot.Manifest) != nil {
		return domainfundsquerysource.DescriptorV1{}, errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds local display current DSV2 selection is invalid"),
		)
	}
	analytical, err := snapshot.Manifest.AnalyticalDuckDB.Values()
	if err != nil || !domainfundsquerysource.QueryProfileSupportsDirectSourcePreviewV1(analytical.QueryProfileDigest) {
		return domainfundsquerysource.DescriptorV1{}, errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds local display query profile is invalid"),
			err,
		)
	}
	producer, err := domainsecurity.ResolveFundsProducerContentBindingV2(
		snapshot.Manifest,
		snapshot.FundsProducerContent,
		snapshot.FundsProducerContentV2,
	)
	if err != nil || producer.Contract != domainsecurity.FundsProducerContentManifestContractV1 {
		return domainfundsquerysource.DescriptorV1{}, errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds local display producer binding is invalid"),
			err,
		)
	}
	binding := snapshot.Record.Binding
	if domainsecurity.ValidateDatasetSnapshotBindingKeyForObservationV1(
		binding, tenantID, userID, observation,
	) != nil || snapshot.Manifest.Binding != binding ||
		snapshot.Record.SourceManifestHash != snapshot.Manifest.SourceManifestHash ||
		binding.WorkspaceRealPath != observation.WorkspaceRealPath ||
		binding.CaseID != observation.CaseID ||
		binding.CaseBindingHash != observation.CaseBindingHash ||
		binding.BindingObservationDigest != observation.ObservationDigest ||
		producer.CaseID != binding.CaseID || producer.ID != snapshot.Manifest.ProducerContentID ||
		producer.ManifestSHA256 != snapshot.Manifest.ProducerContentManifestSHA256 ||
		producer.ManifestByteLength != snapshot.Manifest.ProducerContentManifestByteLength {
		return domainfundsquerysource.DescriptorV1{}, errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds local display selection provenance is mismatched"),
		)
	}
	descriptor, err := domainfundsquerysource.NewDescriptorV1(
		domainfundsquerysource.DescriptorInputV1{
			SnapshotRecordDigest: snapshot.Record.RecordDigest,
			DatasetSnapshotID:    snapshot.Record.DatasetSnapshotID,
			SourceManifestHash:   snapshot.Manifest.SourceManifestHash,
			CaseID:               binding.CaseID, CaseBindingHash: binding.CaseBindingHash,
			DatasetBindingDigest:                   binding.BindingKeyDigest,
			BindingObservationDigest:               binding.BindingObservationDigest,
			FundsProducerContentID:                 producer.ID,
			FundsProducerContentManifestSHA256:     producer.ManifestSHA256,
			FundsProducerContentManifestByteLength: producer.ManifestByteLength,
			DuckDBSHA256:                           analytical.DuckDBSHA256,
			DuckDBByteLength:                       analytical.DuckDBByteLength,
			DuckDBContentSnapshotDigest:            analytical.DuckDBContentSnapshotDigest,
			DuckDBSnapshotManifestSHA256:           analytical.DuckDBSnapshotManifestSHA256,
			MaterializationIdentity:                analytical.MaterializationIdentity,
			SchemaDigest:                           analytical.SchemaDigest,
			DatasetUTCOffsetMinutes:                analytical.DatasetUTCOffsetMinutes,
			ExpectedCurrency:                       analytical.ExpectedCurrency,
			MinorUnitScale:                         analytical.MinorUnitScale,
			QueryProfileDigest:                     analytical.QueryProfileDigest,
		},
	)
	if err != nil {
		return domainfundsquerysource.DescriptorV1{}, errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds local display descriptor cannot be derived"),
			err,
		)
	}
	return descriptor, nil
}

func validateAccountFlowTransactionPolicyV1(
	manifest domainsecurity.DatasetSnapshotManifestV2,
) error {
	policy, ok := domainevidence.ResolveSourceRowProducerPolicyV1(
		manifest.ProducerPolicyID,
	)
	if !ok || !accountFlowTransactionPolicyIDV1(policy.PolicyID) ||
		domainevidence.ValidateSourceRowProducerPolicyV1(policy) != nil ||
		manifest.SourceType != policy.SourceType ||
		manifest.ProducerPolicyID != policy.PolicyID ||
		manifest.ProducerPolicyDigest != policy.PolicyDigest ||
		manifest.ProducerComponentID != policy.ProducerComponentID ||
		manifest.ProducerComponentVersion != policy.ProducerComponentVersion ||
		manifest.ProducerOperation != policy.Operation ||
		manifest.ProducerOperationSchemaHash != policy.OperationSchemaHash ||
		manifest.ParserID != policy.ParserID ||
		manifest.ParserVersion != policy.ParserVersion {
		return errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds query source requires the registered transaction source-row policy"),
		)
	}
	return nil
}

func observationMatchesContextV1(
	observation domainsecurity.CaseBindingObservationV1,
	securityContext domainsecurity.TurnSecurityContext,
) bool {
	return domainsecurity.ValidateCaseBindingObservationV1(observation) == nil &&
		observation.State == domainsecurity.CaseBindingStateValid &&
		observation.WorkspaceRealPath == securityContext.WorkspaceRealPath &&
		observation.CaseID == securityContext.CaseID &&
		observation.CaseBindingHash == securityContext.CaseBindingHash &&
		observation.ObservationDigest == securityContext.PublicationPolicy.BindingObservationDigest
}

func dependencyIsNilV1(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
