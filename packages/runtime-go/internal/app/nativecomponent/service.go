package nativecomponent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	"analytix.local/runtime-go/internal/contracts"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	grantregistryport "analytix.local/runtime-go/internal/ports/grantregistry"
	nativecomponentport "analytix.local/runtime-go/internal/ports/nativecomponent"
)

var (
	ErrUnavailable                = errors.New("native_component_admission_unavailable")
	ErrRequestInvalid             = errors.New("native_component_admission_request_invalid")
	ErrAuthorityInvalid           = errors.New("native_component_admission_authority_invalid")
	ErrGrantInvalid               = errors.New("native_component_admission_grant_invalid")
	ErrResultInvalid              = errors.New("native_component_admission_result_invalid")
	ErrAccountFlowExecutionFailed = errors.New("native_component_account_flow_execution_failed")
)

const fundsAnalyzeAccountFlowsToolName = "mcp__analytix_funds__analyze_account_flows"

type ExecuteInput struct {
	ThreadID string
	TurnID   string
	GrantID  string
}

// AnalyzeAccountFlowsInput is the complete public application input. Exact
// dataset descriptors, source leases and private entity resolution remain
// host-owned dependencies and cannot be supplied by the caller.
type AnalyzeAccountFlowsInput struct {
	ExecuteInput ExecuteInput
	Intent       domainnative.AnalyzeAccountFlowsProviderIntentV1
}

// AccountFlowSubjectResolutionV1 is returned only by the host-owned stable
// entity resolver while the current DSV2/effect lease is live. It is private
// process material and cannot enter ordinary JSON, logs, or history.
type AccountFlowSubjectResolutionV1 struct {
	private accountFlowSubjectPrivateV1
}

type accountFlowSubjectPrivateV1 struct {
	use func(func(domaincaseentity.ReferenceV1, domaincaseentity.ModelEntityAliasV1, string, string) error) error
}

func newAccountFlowSubjectResolutionV1(
	reference domaincaseentity.ReferenceV1,
	alias domaincaseentity.ModelEntityAliasV1,
	canonicalValue string,
	bindingResolutionDigest string,
) AccountFlowSubjectResolutionV1 {
	var used atomic.Bool
	return AccountFlowSubjectResolutionV1{
		private: accountFlowSubjectPrivateV1{
			use: func(consume func(domaincaseentity.ReferenceV1, domaincaseentity.ModelEntityAliasV1, string, string) error) error {
				if consume == nil || !used.CompareAndSwap(false, true) {
					return ErrAuthorityInvalid
				}
				return consume(reference, alias, canonicalValue, bindingResolutionDigest)
			},
		},
	}
}

func (private accountFlowSubjectPrivateV1) useExact(
	consume func(domaincaseentity.ReferenceV1, domaincaseentity.ModelEntityAliasV1, string, string) error,
) error {
	if private.use == nil || consume == nil {
		return ErrAuthorityInvalid
	}
	calls := 0
	err := private.use(func(reference domaincaseentity.ReferenceV1, alias domaincaseentity.ModelEntityAliasV1, canonicalValue string, digest string) error {
		calls++
		if calls != 1 {
			return ErrAuthorityInvalid
		}
		return consume(reference, alias, canonicalValue, digest)
	})
	if calls != 1 {
		return ErrAuthorityInvalid
	}
	return err
}

func (resolution AccountFlowSubjectResolutionV1) MarshalJSON() ([]byte, error) {
	return nil, ErrRequestInvalid
}

func (*AccountFlowSubjectResolutionV1) UnmarshalJSON([]byte) error {
	return ErrRequestInvalid
}

func (resolution AccountFlowSubjectResolutionV1) String() string {
	return "AccountFlowSubjectResolutionV1{resolvedAccountKey:[REDACTED],bindingResolutionDigest:[REDACTED]}"
}

func (resolution AccountFlowSubjectResolutionV1) GoString() string {
	return resolution.String()
}

type ResolveAccountFlowSubject func(
	context.Context,
	domainsecurity.TurnSecurityContext,
	domainfundsquerysource.DescriptorV1,
	domaincaseentity.ModelEntityAliasV1,
	ValidateCurrentAccountFlowCallback,
	func(AccountFlowSubjectResolutionV1) error,
) error

// ResolveAccountFlowCounterparty is invoked only from inside the current
// account-flow source callback. Exact account text is host-private input; the
// callback returns only a case-scoped stable reference and validated natural
// label. ErrAccountFlowCounterpartySemanticUnavailableV1 means the row remains
// useful but its counterparty semantics must be marked partial.
type ResolveAccountFlowCounterparty func(
	context.Context,
	domainsecurity.TurnSecurityContext,
	domainfundsquerysource.DescriptorV1,
	string,
	string,
	ValidateCurrentAccountFlowCallback,
	func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error,
) error

// NewPersistentAccountFlowSubjectResolver is the production resolver factory.
// It accepts only the persistent case-entity service, whose opaque result is
// minted after private-store resolution and installation-keyed HMAC
// revalidation. The returned resolver runs under the account-flow effect lease
// and the caller's already-live exact funds-query-source callback. It binds the
// descriptor to the turn and never reacquires the same dataset authority.
func NewPersistentAccountFlowSubjectResolver(
	service *caseentityapp.Service,
) ResolveAccountFlowSubject {
	if service == nil {
		return nil
	}
	return func(
		ctx context.Context,
		securityContext domainsecurity.TurnSecurityContext,
		descriptor domainfundsquerysource.DescriptorV1,
		alias domaincaseentity.ModelEntityAliasV1,
		validateCurrent ValidateCurrentAccountFlowCallback,
		use func(AccountFlowSubjectResolutionV1) error,
	) error {
		if ctx == nil || validateCurrent == nil || use == nil ||
			domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
			domaincaseentity.ValidateModelEntityAliasV1(string(alias)) != nil {
			return ErrAuthorityInvalid
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		var useErr error
		err := service.UseAccountFlowSubjectV1(
			ctx,
			caseentityapp.NewUseAccountFlowSubjectInputV1(
				securityContext,
				descriptor,
				alias,
				validateCurrent,
			),
			func(reference domaincaseentity.ReferenceV1, canonicalValue string, recordDigest string) error {
				useErr = use(newAccountFlowSubjectResolutionV1(reference, alias, canonicalValue, recordDigest))
				return nil
			},
		)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			return ErrAuthorityInvalid
		}
		return useErr
	}
}

// NewPersistentAccountFlowCounterpartyResolver reuses the existing case-entity
// private store and the caller's already-live DSV2/source lease. It never
// acquires a nested dataset lease and maps unsafe source semantics to an
// explicit partial-result disposition without reflecting private input.
func NewPersistentAccountFlowCounterpartyResolver(
	service *caseentityapp.Service,
) ResolveAccountFlowCounterparty {
	if service == nil {
		return nil
	}
	return func(
		ctx context.Context,
		securityContext domainsecurity.TurnSecurityContext,
		descriptor domainfundsquerysource.DescriptorV1,
		sourceExactValue string,
		bankInstitution string,
		validateCurrent ValidateCurrentAccountFlowCallback,
		use func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error,
	) error {
		if ctx == nil || validateCurrent == nil || use == nil {
			return ErrAuthorityInvalid
		}
		err := service.UseAccountFlowCounterpartyV1(
			ctx,
			caseentityapp.NewUseAccountFlowCounterpartyInputV1(
				securityContext,
				descriptor,
				domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
				sourceExactValue,
				caseentityapp.DisplayLabelSemanticV1{
					Institution: bankInstitution,
					AccountType: domainnative.AccountFlowCounterpartyAccountTypeV1,
				},
				validateCurrent,
			),
			use,
		)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return err
		case errors.Is(err, caseentityapp.ErrInvalidReferenceInput):
			return domainnative.ErrAccountFlowCounterpartySemanticUnavailableV1
		default:
			return ErrAuthorityInvalid
		}
	}
}

// UseCurrentAccountFlowSource is the host-owned composition seam. Its
// implementation must nest the existing DSV2 current-selection capability and
// funds-query-source exact-use callback, provide the existing source-row
// resolver only inside that same callback, and return only after their post-use
// currentness checks have completed.
type UseCurrentAccountFlowSource func(
	context.Context,
	domainsecurity.TurnSecurityContext,
	func(
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
) error

// ValidateCurrentAccountFlowCallback revalidates current principal, risk and
// case binding while the caller is already inside the exact DSV2 selection
// callback. It deliberately does not reacquire the complete DSV2 inventory;
// the callback-scoped selection capability owns that currentness proof.
type ValidateCurrentAccountFlowCallback func(
	context.Context,
	domainsecurity.TurnSecurityContext,
) error

// ValidateCurrentAccountFlowOuterGrant revalidates provider, schema, scope and
// current catalog identity from host-owned state. Callers do not supply an
// expectation that could bless their own grant.
type ValidateCurrentAccountFlowOuterGrant func(
	context.Context,
	domainsecurity.TurnSecurityContext,
	domainsecurity.ExecutionGrant,
) error

type DurableAuthority interface {
	ValidateCurrent(string, map[string]any) (domainsecurity.TurnSecurityContext, error)
}

type LiveAuthority interface {
	ValidateCurrent(context.Context, domainsecurity.TurnSecurityContext) error
}

type AcquireEffect func(context.Context, domainsecurity.TurnSecurityContext) (context.Context, func(), error)

type Dependencies struct {
	Threads                              grantregistryport.Reader
	DurableAuthority                     DurableAuthority
	LiveAuthority                        LiveAuthority
	HealthOnlyAuthority                  LiveAuthority
	AcquireEffect                        AcquireEffect
	Runner                               nativecomponentport.Runner
	AccountFlowRunner                    nativecomponentport.AccountFlowRunner
	AccountIngressRunner                 nativecomponentport.AccountIngressRunner
	UseCurrentAccountFlowSource          UseCurrentAccountFlowSource
	UseCurrentAccountIngressSource       UseCurrentAccountIngressSource
	ResolveAccountFlowSubject            ResolveAccountFlowSubject
	ResolveAccountFlowCounterparty       ResolveAccountFlowCounterparty
	ValidateCurrentAccountFlowCallback   ValidateCurrentAccountFlowCallback
	ValidateCurrentAccountFlowOuterGrant ValidateCurrentAccountFlowOuterGrant
	Now                                  func() time.Time
}

type Service struct {
	dependencies Dependencies
}

// callbackUseBarrier keeps a dependency callback synchronous with its outer
// authority scope. It is process-local lifecycle state, not an authority or a
// persistence contract.
type callbackUseBarrier struct {
	mu        sync.Mutex
	condition *sync.Cond
	active    bool
	attempts  uint32
	inFlight  bool
	completed bool
}

func newCallbackUseBarrier() *callbackUseBarrier {
	barrier := &callbackUseBarrier{active: true}
	barrier.condition = sync.NewCond(&barrier.mu)
	return barrier
}

func (barrier *callbackUseBarrier) begin() bool {
	barrier.mu.Lock()
	defer barrier.mu.Unlock()
	barrier.attempts++
	if !barrier.active || barrier.attempts != 1 || barrier.inFlight {
		return false
	}
	barrier.inFlight = true
	return true
}

func (barrier *callbackUseBarrier) commit(commit func()) bool {
	if barrier == nil || commit == nil {
		return false
	}
	barrier.mu.Lock()
	defer barrier.mu.Unlock()
	if !barrier.active || barrier.attempts != 1 || !barrier.inFlight {
		return false
	}
	commit()
	return true
}

func (barrier *callbackUseBarrier) end(succeeded bool) {
	barrier.mu.Lock()
	defer barrier.mu.Unlock()
	if barrier.inFlight {
		barrier.completed = succeeded && barrier.active && barrier.attempts == 1
		barrier.inFlight = false
	}
	barrier.condition.Broadcast()
}

func (barrier *callbackUseBarrier) close() (uint32, bool) {
	barrier.mu.Lock()
	defer barrier.mu.Unlock()
	barrier.active = false
	for barrier.inFlight {
		barrier.condition.Wait()
	}
	return barrier.attempts, barrier.completed
}

type LiveAuthorityFunc func(context.Context, domainsecurity.TurnSecurityContext) error

func (validate LiveAuthorityFunc) ValidateCurrent(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) error {
	if validate == nil {
		return ErrUnavailable
	}
	return validate(ctx, securityContext)
}

type healthOnlyCurrentnessV1 struct {
	ordinaryEffect   LiveAuthority
	dataEnginePinned func(context.Context) error
}

// NewHealthOnlyCurrentnessV1 composes the only two metadata checks permitted
// for the fixed data-engine health operation. Service admission owns the exact
// operation/grant/request gate; this dependency cannot mint source, dataset,
// provider, or case-fact readiness.
func NewHealthOnlyCurrentnessV1(
	ordinaryEffect LiveAuthority,
	dataEnginePinned func(context.Context) error,
) LiveAuthority {
	if dependencyIsNil(ordinaryEffect) || dataEnginePinned == nil {
		return nil
	}
	return healthOnlyCurrentnessV1{
		ordinaryEffect:   ordinaryEffect,
		dataEnginePinned: dataEnginePinned,
	}
}

func (currentness healthOnlyCurrentnessV1) ValidateCurrent(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
) error {
	if ctx == nil || ctx.Err() != nil || dependencyIsNil(currentness.ordinaryEffect) ||
		currentness.dataEnginePinned == nil {
		return ErrAuthorityInvalid
	}
	if err := currentness.ordinaryEffect.ValidateCurrent(ctx, securityContext); err != nil {
		return err
	}
	return currentness.dataEnginePinned(ctx)
}

type admission struct {
	request         domainnative.Request
	securityContext domainsecurity.TurnSecurityContext
	entry           domainsecurity.ExecutionGrantRegistryEntry
	policy          domainnative.OperationPolicy
	deadline        time.Time
}

func NewService(dependencies Dependencies) *Service {
	return &Service{dependencies: dependencies}
}

func (service *Service) Available() bool {
	return service != nil && !dependencyIsNil(service.dependencies.Threads) && !dependencyIsNil(service.dependencies.DurableAuthority) &&
		!dependencyIsNil(service.dependencies.LiveAuthority) && service.dependencies.AcquireEffect != nil &&
		!dependencyIsNil(service.dependencies.Runner) && service.dependencies.Now != nil
}

func (service *Service) accountFlowsAvailable() bool {
	return service != nil && service.Available() &&
		!dependencyIsNil(service.dependencies.AccountFlowRunner) &&
		service.dependencies.UseCurrentAccountFlowSource != nil &&
		service.dependencies.ResolveAccountFlowSubject != nil &&
		service.dependencies.ResolveAccountFlowCounterparty != nil &&
		service.dependencies.ValidateCurrentAccountFlowCallback != nil &&
		service.dependencies.ValidateCurrentAccountFlowOuterGrant != nil
}

// Execute accepts only durable authority identifiers. Context, grant,
// operation, canonical arguments, deadline, executable identity and process
// configuration are all reconstructed or derived by the host.
func (service *Service) Execute(ctx context.Context, input ExecuteInput) (domainnative.Result, error) {
	if !service.Available() {
		return domainnative.Result{}, ErrUnavailable
	}
	if ctx == nil || !validExecuteInput(input) {
		return domainnative.Result{}, ErrRequestInvalid
	}
	if err := ctx.Err(); err != nil {
		return domainnative.Result{}, err
	}
	before, err := service.admit(ctx, input, nil, nil, service.dependencies.Now().UTC())
	if err != nil {
		return domainnative.Result{}, err
	}
	effectCtx, release, err := service.dependencies.AcquireEffect(ctx, before.request.Context)
	if err != nil || effectCtx == nil || release == nil {
		if release != nil {
			release()
		}
		if contextErr := healthContextError(ctx, err); contextErr != nil {
			return domainnative.Result{}, contextErr
		}
		return domainnative.Result{}, ErrAuthorityInvalid
	}
	defer release()

	current, err := service.admit(effectCtx, input, nil, nil, service.dependencies.Now().UTC())
	if err != nil || current.request.Context != before.request.Context || current.entry != before.entry {
		if contextErr := healthContextError(effectCtx, err); contextErr != nil {
			return domainnative.Result{}, contextErr
		}
		return domainnative.Result{}, ErrAuthorityInvalid
	}
	runCtx, cancel := context.WithDeadline(effectCtx, current.request.Deadline)
	result, runErr := service.dependencies.Runner.Execute(runCtx, current.request)
	cancel()

	originalContextErr := healthContextError(effectCtx, runErr)
	if runErr != nil && !errors.Is(runErr, context.Canceled) && !errors.Is(runErr, context.DeadlineExceeded) &&
		runErr != nativecomponentport.ErrUnavailable {
		// Cancellation never downgrades a simultaneous protocol, registry,
		// containment, termination, or unknown runner failure.
		originalContextErr = nil
	}
	validationBudget := 5 * time.Second
	if operationDeadline, ok := effectCtx.Deadline(); ok {
		remaining := time.Until(operationDeadline)
		if remaining <= 0 {
			if originalContextErr != nil {
				return domainnative.Result{}, originalContextErr
			}
			return domainnative.Result{}, context.DeadlineExceeded
		}
		if remaining < validationBudget {
			validationBudget = remaining
		}
	}
	validationNow := service.dependencies.Now().UTC()
	validationCtx, validationCancel := context.WithTimeout(context.WithoutCancel(effectCtx), validationBudget)
	defer validationCancel()
	after, validationErr := service.admit(validationCtx, input, nil, nil, validationNow)
	if validationErr != nil || after.request.Context != current.request.Context || after.entry != current.entry {
		if originalContextErr != nil {
			return domainnative.Result{}, originalContextErr
		}
		if contextErr := healthContextError(validationCtx, validationErr); contextErr != nil {
			return domainnative.Result{}, contextErr
		}
		return domainnative.Result{}, ErrAuthorityInvalid
	}
	if originalContextErr != nil {
		return domainnative.Result{}, originalContextErr
	}
	if runErr != nil {
		return domainnative.Result{}, runErr
	}
	if domainnative.ValidateResult(result, current.policy) != nil {
		return domainnative.Result{}, ErrResultInvalid
	}
	return result, nil
}

// AnalyzeAccountFlows admits only the provider-safe intent and delegates exact
// snapshot selection to the host-owned DSV2 composition. The exact native
// result is re-admitted and projected while the source-row resolver is live,
// and never crosses that callback. A successful provider-safe semantic result
// remains provisional through the source DSV2 postcheck, a pre-consumer
// admission, one strictly process-local in-memory evidence capture, and a final
// durable/live/catalog admission under the same effect lease. consumeEvidence
// must not persist, publish, settle, or perform any external effect.
func (service *Service) AnalyzeAccountFlows(
	ctx context.Context,
	input AnalyzeAccountFlowsInput,
	consumeEvidence domainnative.AccountFlowHostEvidenceProjectionConsumerV1,
) (domainnative.AccountFlowProviderSemanticResultV1, error) {
	zero := domainnative.AccountFlowProviderSemanticResultV1{}
	if !service.accountFlowsAvailable() {
		return zero, ErrUnavailable
	}
	if ctx == nil || !validExecuteInput(input.ExecuteInput) ||
		domainnative.ValidateAnalyzeAccountFlowsProviderIntentV1(input.Intent) != nil ||
		consumeEvidence == nil {
		return zero, ErrRequestInvalid
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	before, err := service.admit(
		ctx,
		input.ExecuteInput,
		&input.Intent,
		nil,
		service.dependencies.Now().UTC(),
	)
	if err != nil {
		return zero, err
	}
	effectCtx, release, err := service.dependencies.AcquireEffect(ctx, before.securityContext)
	if err != nil || effectCtx == nil || release == nil {
		if release != nil {
			release()
		}
		if contextErr := healthContextError(ctx, err); contextErr != nil {
			return zero, contextErr
		}
		return zero, ErrAuthorityInvalid
	}
	defer release()

	currentEffect, err := service.admit(
		effectCtx,
		input.ExecuteInput,
		&input.Intent,
		nil,
		service.dependencies.Now().UTC(),
	)
	if err != nil || currentEffect.securityContext != before.securityContext || currentEffect.entry != before.entry {
		if contextErr := healthContextError(effectCtx, err); contextErr != nil {
			return zero, contextErr
		}
		return zero, ErrAuthorityInvalid
	}

	var provisionalSemantic domainnative.AccountFlowProviderSemanticResultV1
	var provisionalProjection domainnative.AccountFlowHostEvidenceProjectionV1
	var provisionalArguments domainnative.AnalyzeAccountFlowsArgumentsV1
	var provisionalDescriptor domainfundsquerysource.DescriptorV1
	var provisionalAdmission admission
	defer func() {
		domainnative.DiscardAccountFlowHostEvidenceProjectionV1(provisionalProjection)
	}()
	sourceBarrier := newCallbackUseBarrier()
	var sourceAttempts uint32
	var sourceCompleted bool
	var callbackErrMu sync.Mutex
	var callbackErr error
	recordCallbackErr := func(err error) error {
		callbackErrMu.Lock()
		defer callbackErrMu.Unlock()
		if callbackErr == nil {
			callbackErr = err
		}
		return err
	}
	useErr := func() error {
		defer func() {
			sourceAttempts, sourceCompleted = sourceBarrier.close()
		}()
		return service.dependencies.UseCurrentAccountFlowSource(
			effectCtx,
			currentEffect.securityContext,
			func(
				sourceCtx context.Context,
				descriptor domainfundsquerysource.DescriptorV1,
				source fundsquerysourceport.ExactReadLease,
				resolveSourceRow domainnative.AccountFlowSourceRowResolverV1,
				revalidatePostNative func(
					context.Context,
					domainsecurity.TurnSecurityContext,
					domainfundsquerysource.DescriptorV1,
					func(context.Context) error,
				) error,
			) error {
				if !sourceBarrier.begin() {
					return recordCallbackErr(ErrAuthorityInvalid)
				}
				succeeded := false
				defer func() { sourceBarrier.end(succeeded) }()

				var resolution AccountFlowSubjectResolutionV1
				resolutionBarrier := newCallbackUseBarrier()
				var resolutionAttempts uint32
				var resolutionCompleted bool
				resolutionErr := func() error {
					defer func() {
						resolutionAttempts, resolutionCompleted = resolutionBarrier.close()
					}()
					return service.dependencies.ResolveAccountFlowSubject(
						sourceCtx,
						currentEffect.securityContext,
						descriptor,
						domaincaseentity.ModelEntityAliasV1(input.Intent.SubjectAlias),
						service.dependencies.ValidateCurrentAccountFlowCallback,
						func(current AccountFlowSubjectResolutionV1) error {
							if !resolutionBarrier.begin() {
								return ErrAuthorityInvalid
							}
							resolutionSucceeded := false
							defer func() { resolutionBarrier.end(resolutionSucceeded) }()
							if !resolutionBarrier.commit(func() { resolution = current }) {
								return ErrAuthorityInvalid
							}
							resolutionSucceeded = true
							return nil
						},
					)
				}()
				if resolutionErr != nil || resolutionAttempts != 1 || !resolutionCompleted {
					if contextErr := healthContextError(sourceCtx, resolutionErr); contextErr != nil {
						return recordCallbackErr(contextErr)
					}
					return recordCallbackErr(ErrAuthorityInvalid)
				}
				afterResolution, resolutionAdmissionErr := service.admitInsideCurrentAccountFlowSource(
					sourceCtx,
					input.ExecuteInput,
					&input.Intent,
					nil,
					service.dependencies.Now().UTC(),
				)
				if resolutionAdmissionErr != nil ||
					afterResolution.securityContext != currentEffect.securityContext ||
					afterResolution.entry != currentEffect.entry ||
					!accountFlowDescriptorMatches(descriptor, afterResolution.securityContext) {
					if contextErr := healthContextError(sourceCtx, resolutionAdmissionErr); contextErr != nil {
						return recordCallbackErr(contextErr)
					}
					return recordCallbackErr(ErrAuthorityInvalid)
				}
				result, arguments, current, exactErr := service.runResolvedAccountFlowExact(
					sourceCtx,
					input,
					afterResolution,
					descriptor,
					source,
					resolution,
					revalidatePostNative,
				)
				if exactErr != nil {
					return recordCallbackErr(exactErr)
				}
				beforeProjection, projectionAdmissionErr := service.admitInsideCurrentAccountFlowSource(
					sourceCtx,
					input.ExecuteInput,
					&input.Intent,
					&arguments,
					service.dependencies.Now().UTC(),
				)
				if projectionAdmissionErr != nil ||
					beforeProjection.request.Context != current.request.Context ||
					beforeProjection.entry != current.entry ||
					!accountFlowDescriptorMatches(descriptor, beforeProjection.request.Context) {
					if contextErr := healthContextError(sourceCtx, projectionAdmissionErr); contextErr != nil {
						return recordCallbackErr(contextErr)
					}
					return recordCallbackErr(ErrAuthorityInvalid)
				}
				semantic, projection, projectionErr := domainnative.ProjectAnalyzeAccountFlowsResultV1(
					result,
					arguments,
					resolveSourceRow,
					func(
						sourceExactAccount string,
						bankInstitution string,
						consume func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error,
					) error {
						return service.dependencies.ResolveAccountFlowCounterparty(
							sourceCtx,
							afterResolution.securityContext,
							descriptor,
							sourceExactAccount,
							bankInstitution,
							service.dependencies.ValidateCurrentAccountFlowCallback,
							consume,
						)
					},
				)
				if projectionErr != nil {
					return recordCallbackErr(sanitizeAccountFlowProjectionError(sourceCtx, projectionErr))
				}
				if !sourceBarrier.commit(func() {
					provisionalSemantic = semantic
					provisionalProjection = projection
					provisionalArguments = arguments
					provisionalDescriptor = descriptor
					provisionalAdmission = beforeProjection
				}) {
					domainnative.DiscardAccountFlowHostEvidenceProjectionV1(projection)
					return recordCallbackErr(ErrAuthorityInvalid)
				}
				succeeded = true
				return nil
			},
		)
	}()
	callbackErrMu.Lock()
	currentCallbackErr := callbackErr
	callbackErrMu.Unlock()
	if contextErr := healthContextError(effectCtx, currentCallbackErr); contextErr != nil {
		return zero, contextErr
	}
	if contextErr := healthContextError(effectCtx, useErr); contextErr != nil {
		return zero, contextErr
	}
	if currentCallbackErr != nil {
		return zero, currentCallbackErr
	}
	if useErr != nil {
		return zero, ErrAuthorityInvalid
	}
	if sourceAttempts != 1 || !sourceCompleted {
		return zero, ErrAuthorityInvalid
	}

	afterOuter, err := service.admit(
		effectCtx,
		input.ExecuteInput,
		&input.Intent,
		&provisionalArguments,
		service.dependencies.Now().UTC(),
	)
	if err != nil || afterOuter.request.Context != provisionalAdmission.request.Context ||
		afterOuter.entry != provisionalAdmission.entry ||
		!accountFlowDescriptorMatches(provisionalDescriptor, afterOuter.request.Context) {
		if contextErr := healthContextError(effectCtx, err); contextErr != nil {
			return zero, contextErr
		}
		return zero, ErrAuthorityInvalid
	}
	if err := effectCtx.Err(); err != nil {
		return zero, err
	}
	if err := domainnative.ConsumeAccountFlowHostEvidenceProjectionV1(
		provisionalProjection,
		consumeEvidence,
	); err != nil {
		if contextErr := healthContextError(effectCtx, err); contextErr != nil {
			return zero, contextErr
		}
		return zero, ErrResultInvalid
	}
	if err := effectCtx.Err(); err != nil {
		return zero, err
	}
	afterConsumer, err := service.admit(
		effectCtx,
		input.ExecuteInput,
		&input.Intent,
		&provisionalArguments,
		service.dependencies.Now().UTC(),
	)
	if err != nil || afterConsumer.request.Context != afterOuter.request.Context ||
		afterConsumer.entry != afterOuter.entry ||
		!accountFlowDescriptorMatches(provisionalDescriptor, afterConsumer.request.Context) {
		if contextErr := healthContextError(effectCtx, err); contextErr != nil {
			return zero, contextErr
		}
		return zero, ErrAuthorityInvalid
	}
	return provisionalSemantic, nil
}

func sanitizeAccountFlowProjectionError(ctx context.Context, err error) error {
	if contextErr := healthContextError(ctx, err); contextErr != nil {
		return contextErr
	}
	for _, candidate := range []error{
		fundsquerysourceport.ErrUnavailable,
		fundsquerysourceport.ErrNotFound,
		fundsquerysourceport.ErrMismatch,
		fundsquerysourceport.ErrCorrupt,
	} {
		if errors.Is(err, candidate) {
			return candidate
		}
	}
	return ErrResultInvalid
}

func (service *Service) runResolvedAccountFlowExact(
	effectCtx context.Context,
	input AnalyzeAccountFlowsInput,
	currentPublic admission,
	descriptor domainfundsquerysource.DescriptorV1,
	source fundsquerysourceport.ExactReadLease,
	resolution AccountFlowSubjectResolutionV1,
	revalidatePostNative func(
		context.Context,
		domainsecurity.TurnSecurityContext,
		domainfundsquerysource.DescriptorV1,
		func(context.Context) error,
	) error,
) (
	domainnative.AnalyzeAccountFlowsResultV1,
	domainnative.AnalyzeAccountFlowsArgumentsV1,
	admission,
	error,
) {
	zeroResult := domainnative.AnalyzeAccountFlowsResultV1{}
	zeroArguments := domainnative.AnalyzeAccountFlowsArgumentsV1{}
	zeroAdmission := admission{}
	arguments, err := accountFlowArgumentsFor(
		descriptor,
		currentPublic.securityContext,
		input.Intent,
		resolution,
	)
	if err != nil {
		return zeroResult, zeroArguments, zeroAdmission, ErrAuthorityInvalid
	}
	if revalidatePostNative == nil {
		return zeroResult, zeroArguments, zeroAdmission, ErrAuthorityInvalid
	}
	current, err := service.admitInsideCurrentAccountFlowSource(
		effectCtx,
		input.ExecuteInput,
		&input.Intent,
		&arguments,
		service.dependencies.Now().UTC(),
	)
	if err != nil || current.securityContext != currentPublic.securityContext || current.entry != currentPublic.entry ||
		!accountFlowDescriptorMatches(descriptor, current.securityContext) {
		if contextErr := healthContextError(effectCtx, err); contextErr != nil {
			return zeroResult, zeroArguments, zeroAdmission, contextErr
		}
		return zeroResult, zeroArguments, zeroAdmission, ErrAuthorityInvalid
	}
	runCtx, cancel := context.WithDeadline(effectCtx, current.request.Deadline)
	result, runErr := service.dependencies.AccountFlowRunner.AnalyzeAccountFlows(
		runCtx,
		current.request,
		descriptor,
		source,
	)
	cancel()

	originalContextErr := healthContextError(effectCtx, runErr)
	if runErr != nil && !errors.Is(runErr, context.Canceled) && !errors.Is(runErr, context.DeadlineExceeded) {
		// An unknown native/source failure is never downgraded to cancellation,
		// but its untrusted text is not allowed to escape this host boundary.
		originalContextErr = nil
	}
	validationBudget := 5 * time.Second
	if operationDeadline, ok := effectCtx.Deadline(); ok {
		remaining := time.Until(operationDeadline)
		if remaining <= 0 {
			if originalContextErr != nil {
				return zeroResult, zeroArguments, zeroAdmission, originalContextErr
			}
			return zeroResult, zeroArguments, zeroAdmission, context.DeadlineExceeded
		}
		if remaining < validationBudget {
			validationBudget = remaining
		}
	}
	validationNow := service.dependencies.Now().UTC()
	validationCtx, validationCancel := context.WithTimeout(
		context.WithoutCancel(effectCtx),
		validationBudget,
	)
	defer validationCancel()
	var after admission
	validationErr := revalidatePostNative(
		validationCtx,
		current.request.Context,
		descriptor,
		func(currentnessContext context.Context) error {
			var currentnessErr error
			after, currentnessErr = service.admitInsideCurrentAccountFlowSource(
				currentnessContext,
				input.ExecuteInput,
				&input.Intent,
				&arguments,
				validationNow,
			)
			return currentnessErr
		},
	)
	if validationErr != nil || after.request.Context != current.request.Context || after.entry != current.entry ||
		!accountFlowDescriptorMatches(descriptor, after.request.Context) {
		if originalContextErr != nil {
			return zeroResult, zeroArguments, zeroAdmission, originalContextErr
		}
		if contextErr := healthContextError(validationCtx, validationErr); contextErr != nil {
			return zeroResult, zeroArguments, zeroAdmission, contextErr
		}
		return zeroResult, zeroArguments, zeroAdmission, ErrAuthorityInvalid
	}
	if originalContextErr != nil {
		return zeroResult, zeroArguments, zeroAdmission, originalContextErr
	}
	if runErr != nil {
		return zeroResult, zeroArguments, zeroAdmission, ErrAccountFlowExecutionFailed
	}
	if domainnative.ValidateAnalyzeAccountFlowsResultV1(result, arguments) != nil {
		return zeroResult, zeroArguments, zeroAdmission, ErrResultInvalid
	}
	return result, arguments, after, nil
}

func (service *Service) admit(
	ctx context.Context,
	input ExecuteInput,
	accountFlowIntent *domainnative.AnalyzeAccountFlowsProviderIntentV1,
	accountFlowArguments *domainnative.AnalyzeAccountFlowsArgumentsV1,
	now time.Time,
) (admission, error) {
	return service.admitWithCurrentness(
		ctx,
		input,
		accountFlowIntent,
		accountFlowArguments,
		now,
		service.dependencies.LiveAuthority.ValidateCurrent,
	)
}

// admitInsideCurrentAccountFlowSource preserves every durable thread, grant,
// catalog, request and deadline check from admit while the already-live exact
// DSV2 callback supplies dataset currentness. The injected validator retains
// fresh principal, risk and case-binding checks without resolving the complete
// dataset inventory again.
func (service *Service) admitInsideCurrentAccountFlowSource(
	ctx context.Context,
	input ExecuteInput,
	accountFlowIntent *domainnative.AnalyzeAccountFlowsProviderIntentV1,
	accountFlowArguments *domainnative.AnalyzeAccountFlowsArgumentsV1,
	now time.Time,
) (admission, error) {
	return service.admitWithCurrentness(
		ctx,
		input,
		accountFlowIntent,
		accountFlowArguments,
		now,
		service.dependencies.ValidateCurrentAccountFlowCallback,
	)
}

func (service *Service) admitWithCurrentness(
	ctx context.Context,
	input ExecuteInput,
	accountFlowIntent *domainnative.AnalyzeAccountFlowsProviderIntentV1,
	accountFlowArguments *domainnative.AnalyzeAccountFlowsArgumentsV1,
	now time.Time,
	validateCurrent func(context.Context, domainsecurity.TurnSecurityContext) error,
) (admission, error) {
	if ctx == nil {
		return admission{}, ErrAuthorityInvalid
	}
	if err := ctx.Err(); err != nil {
		return admission{}, err
	}
	thread, err := service.dependencies.Threads.GetThread(input.ThreadID)
	if err != nil || thread == nil || strings.TrimSpace(contracts.StringField(thread, "id")) != input.ThreadID {
		return admission{}, ErrAuthorityInvalid
	}
	securityContext, err := service.dependencies.DurableAuthority.ValidateCurrent(input.ThreadID, thread)
	if err != nil || securityContext.ThreadID != input.ThreadID || securityContext.TurnID != input.TurnID ||
		domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) != nil ||
		!domainsecurity.TurnSecurityContextAllowsCaseEvidence(securityContext) {
		return admission{}, ErrAuthorityInvalid
	}
	turn, ok := appmodel.TurnByID(thread, input.TurnID)
	status := strings.TrimSpace(contracts.StringField(turn, "status"))
	if !ok || (status != "running" && status != "waiting") {
		return admission{}, ErrAuthorityInvalid
	}
	turnContext, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || turnContext != securityContext {
		return admission{}, ErrAuthorityInvalid
	}
	deferCurrentnessForHealthCandidate := accountFlowIntent == nil && accountFlowArguments == nil
	if !deferCurrentnessForHealthCandidate {
		if liveErr := service.validateAdmissionCurrentnessV1(
			ctx,
			securityContext,
			domainnative.OperationPolicy{},
			nil,
			validateCurrent,
		); liveErr != nil {
			return admission{}, liveErr
		}
	}
	registry, err := executiongrantapp.RegistryFromThread(input.ThreadID, thread, input.TurnID)
	if err != nil {
		return admission{}, ErrGrantInvalid
	}
	entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, input.GrantID)
	if !found || domainsecurity.VerifyExecutionGrantMembership(
		registry, input.ThreadID, input.TurnID, entry.Grant, domainsecurity.GrantRegistryActive,
	) != nil {
		return admission{}, ErrGrantInvalid
	}
	if accountFlowIntent != nil {
		if service.dependencies.ValidateCurrentAccountFlowOuterGrant == nil {
			return admission{}, ErrGrantInvalid
		}
		if validationErr := service.dependencies.ValidateCurrentAccountFlowOuterGrant(
			ctx,
			securityContext,
			entry.Grant,
		); validationErr != nil {
			if contextErr := healthContextError(ctx, validationErr); contextErr != nil {
				return admission{}, contextErr
			}
			return admission{}, ErrGrantInvalid
		}
	}
	policy, grant, ok := nativeRequestAuthority(
		securityContext,
		entry.Grant,
		accountFlowIntent,
		accountFlowArguments,
	)
	if !ok {
		return admission{}, ErrGrantInvalid
	}
	deadline, err := hostDeadline(ctx, entry.Grant, policy, now)
	if err != nil {
		return admission{}, ErrGrantInvalid
	}
	if accountFlowIntent != nil && accountFlowArguments == nil {
		return admission{
			securityContext: securityContext,
			entry:           entry,
			policy:          policy,
			deadline:        deadline,
		}, nil
	}
	request := domainnative.Request{
		ComponentID:          policy.ComponentID,
		Operation:            policy.Operation,
		AccountFlowArguments: accountFlowArguments,
		Context:              securityContext,
		Grant:                grant,
		Deadline:             deadline,
	}
	if domainnative.ValidateRequest(request, now) != nil {
		return admission{}, ErrGrantInvalid
	}
	if deferCurrentnessForHealthCandidate {
		if liveErr := service.validateAdmissionCurrentnessV1(
			ctx,
			securityContext,
			policy,
			&request,
			validateCurrent,
		); liveErr != nil {
			return admission{}, liveErr
		}
	}
	return admission{
		request: request, securityContext: securityContext, entry: entry, policy: policy, deadline: deadline,
	}, nil
}

func (service *Service) validateAdmissionCurrentnessV1(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	policy domainnative.OperationPolicy,
	request *domainnative.Request,
	fallback func(context.Context, domainsecurity.TurnSecurityContext) error,
) error {
	validateCurrent := fallback
	if request != nil && exactCanonicalHealthAdmissionV1(policy, *request) {
		if service == nil || dependencyIsNil(service.dependencies.HealthOnlyAuthority) {
			return ErrAuthorityInvalid
		}
		validateCurrent = service.dependencies.HealthOnlyAuthority.ValidateCurrent
	}
	if validateCurrent == nil {
		return ErrAuthorityInvalid
	}
	if liveErr := validateCurrent(ctx, securityContext); liveErr != nil {
		if contextErr := healthContextError(ctx, liveErr); contextErr != nil {
			return contextErr
		}
		return ErrAuthorityInvalid
	}
	return nil
}

func exactCanonicalHealthAdmissionV1(
	policy domainnative.OperationPolicy,
	request domainnative.Request,
) bool {
	canonicalArguments, canonical := domainnative.CanonicalArgumentsV1(
		domainnative.ComponentDataEngine,
		"health",
	)
	return canonical && canonicalArguments == "{}" &&
		policy.ComponentID == domainnative.ComponentDataEngine && policy.Operation == "health" &&
		policy.ReadOnly && policy.MaxDuration == 10*time.Second &&
		len(policy.Required) == 0 && len(policy.Optional) == 0 &&
		request.ComponentID == policy.ComponentID && request.Operation == policy.Operation &&
		request.AccountFlowArguments == nil
}

// nativeRequestAuthority preserves the durable provider/MCP grant as the
// only registered authority for a funds call. The fixed native operation is a
// host-private implementation effect underneath that active outer grant, so
// its narrowly scoped grant is reconstructed in memory and is never added to
// the thread registry.
func nativeRequestAuthority(
	securityContext domainsecurity.TurnSecurityContext,
	outer domainsecurity.ExecutionGrant,
	accountFlowIntent *domainnative.AnalyzeAccountFlowsProviderIntentV1,
	accountFlowArguments *domainnative.AnalyzeAccountFlowsArgumentsV1,
) (domainnative.OperationPolicy, domainsecurity.ExecutionGrant, bool) {
	if accountFlowIntent == nil && accountFlowArguments == nil {
		policy, ok := domainnative.ParseToolName(outer.ToolName)
		return policy, outer, ok
	}
	if accountFlowIntent == nil {
		return domainnative.OperationPolicy{}, domainsecurity.ExecutionGrant{}, false
	}
	policy, ok := domainnative.Policy(
		domainnative.ComponentDataEngine,
		domainnative.OperationFundsAnalyzeAccountFlows,
	)
	if !ok || domainsecurity.ValidateExecutionGrantForContext(outer, securityContext) != nil ||
		outer.ToolName != fundsAnalyzeAccountFlowsToolName || !outer.ReadOnly ||
		domainnative.AnalyzeAccountFlowsProviderIntentHashV1(*accountFlowIntent) == "" ||
		outer.ArgsHash != domainnative.AnalyzeAccountFlowsProviderIntentHashV1(*accountFlowIntent) ||
		(outer.ApprovalState != "not_required" && outer.ApprovalState != "approved") {
		return domainnative.OperationPolicy{}, domainsecurity.ExecutionGrant{}, false
	}
	serverIdentity, identityErr := domainsecurity.ParseVerifiedMCPServerIdentity(outer.ServerIdentity)
	if identityErr != nil || !domainsecurity.VerifiedMCPServerIdentityCanAuthorizeFacts(serverIdentity) ||
		serverIdentity.ServerID != "analytix_funds" ||
		serverIdentity.ConnectionEpoch != outer.ConnectionEpoch {
		return domainnative.OperationPolicy{}, domainsecurity.ExecutionGrant{}, false
	}
	if accountFlowArguments == nil {
		return policy, domainsecurity.ExecutionGrant{}, true
	}
	issuedAt, issuedErr := time.Parse(time.RFC3339Nano, outer.IssuedAt)
	expiresAt, expiresErr := time.Parse(time.RFC3339Nano, outer.ExpiresAt)
	argumentsHash := domainnative.AnalyzeAccountFlowsArgumentsHashV1(*accountFlowArguments)
	if issuedErr != nil || expiresErr != nil || argumentsHash == "" {
		return domainnative.OperationPolicy{}, domainsecurity.ExecutionGrant{}, false
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context:         securityContext,
		Provider:        domainnative.NativeProvider,
		ServerIdentity:  domainnative.NativeServerIdentity,
		ToolName:        domainnative.ToolName(policy.ComponentID, policy.Operation),
		ToolCallID:      outer.ToolCallID,
		ConnectionEpoch: 0,
		ArgsHash:        argumentsHash,
		SchemaHash:      policy.SchemaHash,
		ScopeHash:       domainnative.ScopeHash(securityContext, policy.ComponentID, policy.Operation),
		ReadOnly:        policy.ReadOnly,
		ApprovalState:   outer.ApprovalState,
		IssuedAt:        issuedAt,
		ExpiresAt:       expiresAt,
	})
	if domainsecurity.ValidateExecutionGrantForContext(grant, securityContext) != nil {
		return domainnative.OperationPolicy{}, domainsecurity.ExecutionGrant{}, false
	}
	return policy, grant, true
}

func accountFlowDescriptorMatches(
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
		domainfundsquerysource.QueryProfileSupportsAccountFlowV1(
			descriptor.QueryProfileDigest,
		)
}

func accountFlowArgumentsFor(
	descriptor domainfundsquerysource.DescriptorV1,
	securityContext domainsecurity.TurnSecurityContext,
	intent domainnative.AnalyzeAccountFlowsProviderIntentV1,
	resolution AccountFlowSubjectResolutionV1,
) (domainnative.AnalyzeAccountFlowsArgumentsV1, error) {
	var zeroArguments domainnative.AnalyzeAccountFlowsArgumentsV1
	if !accountFlowDescriptorMatches(descriptor, securityContext) ||
		domainnative.ValidateAnalyzeAccountFlowsProviderIntentV1(intent) != nil {
		return zeroArguments, ErrAuthorityInvalid
	}
	var arguments domainnative.AnalyzeAccountFlowsArgumentsV1
	err := resolution.private.useExact(func(
		reference domaincaseentity.ReferenceV1,
		alias domaincaseentity.ModelEntityAliasV1,
		canonicalValue string,
		bindingResolutionDigest string,
	) error {
		if alias != domaincaseentity.ModelEntityAliasV1(intent.SubjectAlias) || canonicalValue == "" ||
			!domainsecurity.IsSHA256Hex(bindingResolutionDigest) {
			return ErrAuthorityInvalid
		}
		input := domainnative.NewAnalyzeAccountFlowsArgumentsInputV1(domainnative.AnalyzeAccountFlowsArgumentsInputV1{
			CaseID:                               securityContext.CaseID,
			DatasetSnapshotID:                    securityContext.DatasetSnapshotID,
			ContextEpoch:                         securityContext.ContextEpoch,
			ContextDigest:                        securityContext.ContextDigest,
			CaseBindingHash:                      securityContext.CaseBindingHash,
			ExpectedProducerContentID:            descriptor.FundsProducerContentID,
			ExpectedProducerManifestSHA256:       descriptor.FundsProducerContentManifestSHA256,
			ExpectedDuckDBContentSnapshotDigest:  descriptor.DuckDBContentSnapshotDigest,
			ExpectedDuckDBSnapshotManifestSHA256: descriptor.DuckDBSnapshotManifestSHA256,
			ExpectedMaterializationIdentity:      descriptor.MaterializationIdentity,
			SubjectAlias:                         intent.SubjectAlias,
			SubjectRef:                           string(reference),
			SubjectResolutionDigest:              bindingResolutionDigest,
			StartInclusive:                       intent.StartInclusive,
			EndInclusive:                         intent.EndInclusive,
			EvidenceRowLimit:                     intent.EvidenceRowLimit,
			DatasetUTCOffsetMinutes:              descriptor.DatasetUTCOffsetMinutes,
			ExpectedCurrency:                     descriptor.ExpectedCurrency,
			MinorUnitScale:                       descriptor.MinorUnitScale,
			ScanCap:                              domainnative.AccountFlowMaximumScanRowsV1,
		}, canonicalValue)
		var argumentErr error
		arguments, argumentErr = domainnative.NewAnalyzeAccountFlowsArgumentsV1(input)
		return argumentErr
	})
	if err != nil {
		return zeroArguments, ErrAuthorityInvalid
	}
	return arguments, nil
}

func validExecuteInput(input ExecuteInput) bool {
	return input.ThreadID != "" && input.ThreadID == strings.TrimSpace(input.ThreadID) &&
		input.TurnID != "" && input.TurnID == strings.TrimSpace(input.TurnID) &&
		domainsecurity.IsSHA256Hex(input.GrantID)
}

func hostDeadline(ctx context.Context, grant domainsecurity.ExecutionGrant, policy domainnative.OperationPolicy, now time.Time) (time.Time, error) {
	expiresAt, err := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	if err != nil || now.IsZero() || policy.MaxDuration <= 0 || !now.Before(expiresAt) {
		return time.Time{}, ErrGrantInvalid
	}
	deadline := now.Add(policy.MaxDuration)
	if expiresAt.Before(deadline) {
		deadline = expiresAt
	}
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if !deadline.After(now) {
		return time.Time{}, ErrGrantInvalid
	}
	return deadline.UTC(), nil
}
