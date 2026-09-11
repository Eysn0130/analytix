package caseentity

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	fundsquerysourceapp "analytix.local/runtime-go/internal/app/fundsquerysource"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainprivacyprojection "analytix.local/runtime-go/internal/domain/privacyprojection"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	casecontextport "analytix.local/runtime-go/internal/ports/casecontext"
	caseentityport "analytix.local/runtime-go/internal/ports/caseentity"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
)

const (
	referenceDigestPurposeV1            = "case_entity.reference/v1"
	maxResolveCandidatesV1              = 4_096
	maxAccountIngressTextBytesV1        = 256 * 1024
	maxAccountIngressSpansV1            = 128
	maxAccountIngressUniqueValuesV1     = 64
	maxAccountIngressProjectionPassesV1 = 16
	minCompleteAccountDigitsV1          = 8
	maxCompleteAccountDigitsV1          = 32
	maxAccountIngressCandidateBytesV1   = 256
)

type AccountIngressCompilationStatusV1 string

const (
	AccountIngressCompilationStatusNoopV1      AccountIngressCompilationStatusV1 = "noop"
	AccountIngressCompilationStatusPersistedV1 AccountIngressCompilationStatusV1 = "persisted"
	AccountIngressCompilationStatusBlockedV1   AccountIngressCompilationStatusV1 = "blocked"
)

var (
	ErrInvalidReferenceInput    = errors.New("case entity reference input is invalid")
	ErrReferenceDerivation      = errors.New("case entity reference derivation failed")
	ErrReferenceNotFound        = errors.New("case entity reference is not resolved")
	ErrReferenceAmbiguous       = errors.New("case entity reference resolution is ambiguous")
	ErrPrivateStateUnavailable  = errors.New("case entity private state is unavailable")
	ErrPrivateStateNotFound     = errors.New("case entity private state is not found")
	ErrPrivateStateConflict     = errors.New("case entity private state conflicts with existing state")
	ErrPrivateStateIntegrity    = errors.New("case entity private state integrity validation failed")
	ErrInvalidAccountIngress    = errors.New("case account ingress input is invalid")
	ErrUnsupportedIngressPII    = errors.New("case account ingress contains unsupported or non-canonical restricted PII")
	ErrAccountIngressProjection = errors.New("case account ingress projection is invalid")
	ErrAccountIngressResolution = errors.New("case account ingress source resolution is unavailable or ambiguous")
)

// KeyedPayloadDigester is implemented by pendingwork.Service. Keeping this
// interface narrow reuses the installation-keyed HMAC without exposing the
// installation signing authority to case-entity logic.
type KeyedPayloadDigester interface {
	KeyedPayloadHash(context.Context, string, []byte) (string, error)
}

type privateTextUseV1 func(func(string) error) error

type privateTextSliceUseV1 func(func([]string) error) error

func newPrivateTextUseV1(value string) privateTextUseV1 {
	return func(use func(string) error) error {
		if use == nil {
			return ErrInvalidReferenceInput
		}
		return use(value)
	}
}

func privateTextValueV1(privateUse privateTextUseV1) (string, error) {
	if privateUse == nil {
		return "", ErrInvalidReferenceInput
	}
	var value string
	if err := privateUse(func(privateValue string) error {
		value = privateValue
		return nil
	}); err != nil {
		return "", ErrInvalidReferenceInput
	}
	return value, nil
}

func newPrivateTextSliceUseV1(values []string) privateTextSliceUseV1 {
	privateValues := append([]string(nil), values...)
	return func(use func([]string) error) error {
		if use == nil {
			return ErrInvalidReferenceInput
		}
		return use(append([]string(nil), privateValues...))
	}
}

func privateTextSliceValueV1(privateUse privateTextSliceUseV1) ([]string, error) {
	if privateUse == nil {
		return nil, ErrInvalidReferenceInput
	}
	var values []string
	if err := privateUse(func(privateValues []string) error {
		values = append([]string(nil), privateValues...)
		return nil
	}); err != nil {
		return nil, ErrInvalidReferenceInput
	}
	return values, nil
}

type DeriveReferenceInputV1 struct {
	SecurityContext       domainsecurity.TurnSecurityContext
	FinancialAccountType  string
	sourceExactValueUseV1 privateTextUseV1
}

func NewDeriveReferenceInputV1(
	securityContext domainsecurity.TurnSecurityContext,
	financialAccountType string,
	sourceExactValue string,
) DeriveReferenceInputV1 {
	return DeriveReferenceInputV1{
		SecurityContext: securityContext, FinancialAccountType: financialAccountType,
		sourceExactValueUseV1: newPrivateTextUseV1(sourceExactValue),
	}
}

type ResolveReferenceInputV1 struct {
	SecurityContext                 domainsecurity.TurnSecurityContext
	FinancialAccountType            string
	Reference                       domaincaseentity.ReferenceV1
	candidateSourceExactValuesUseV1 privateTextSliceUseV1
}

func NewResolveReferenceInputV1(
	securityContext domainsecurity.TurnSecurityContext,
	financialAccountType string,
	reference domaincaseentity.ReferenceV1,
	candidateSourceExactValues []string,
) ResolveReferenceInputV1 {
	return ResolveReferenceInputV1{
		SecurityContext: securityContext, FinancialAccountType: financialAccountType,
		Reference:                       reference,
		candidateSourceExactValuesUseV1: newPrivateTextSliceUseV1(candidateSourceExactValues),
	}
}

type Service struct {
	keyed            KeyedPayloadDigester
	store            caseentityport.Store
	datasetAuthority datasetsnapshotport.CurrentAuthorityV2
	bindingObserver  casecontextport.Observer
	validateCurrent  func(context.Context, domainsecurity.TurnSecurityContext) error
}

func NewService(keyed KeyedPayloadDigester) *Service {
	return &Service{keyed: keyed}
}

func NewPersistentService(
	keyed KeyedPayloadDigester,
	store caseentityport.Store,
	datasetAuthority datasetsnapshotport.CurrentAuthorityV2,
	bindingObserver casecontextport.Observer,
	validateCurrent func(context.Context, domainsecurity.TurnSecurityContext) error,
) *Service {
	return &Service{
		keyed: keyed, store: store,
		datasetAuthority: datasetAuthority, bindingObserver: bindingObserver,
		validateCurrent: validateCurrent,
	}
}

func (service *Service) withCurrentPrivateStateV1(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	use func(context.Context) error,
) error {
	if use == nil {
		return ErrPrivateStateUnavailable
	}
	return service.withCurrentPrivateSelectionV1(
		ctx,
		securityContext,
		func(leaseContext context.Context, _ datasetsnapshotport.CurrentSelectionV2) error {
			return use(leaseContext)
		},
	)
}

func (service *Service) withCurrentPrivateSelectionV1(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	use func(context.Context, datasetsnapshotport.CurrentSelectionV2) error,
) error {
	if service == nil || ctx == nil || use == nil ||
		dependencyIsNilV1(service.datasetAuthority) || dependencyIsNilV1(service.bindingObserver) ||
		service.validateCurrent == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil {
		return ErrPrivateStateUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := service.validateCurrent(ctx, securityContext); err != nil {
		return privateCurrentValidationErrorV1(err)
	}
	observation, err := service.bindingObserver.Observe(securityContext.WorkspaceRealPath)
	if err != nil || !privateStateObservationMatchesContextV1(observation, securityContext) {
		return ErrPrivateStateUnavailable
	}
	resolveInput := datasetsnapshotport.ResolveInputV2{
		TenantID: securityContext.TenantID, UserID: securityContext.UserID,
		Observation: observation, ExpectedDatasetSnapshotID: securityContext.DatasetSnapshotID,
	}
	var selectionCalls atomic.Uint32
	var exactCalls atomic.Uint32
	var operationErr error
	err = service.datasetAuthority.WithCurrentSelectionV2(
		ctx,
		resolveInput,
		securityContext,
		func(
			selection datasetsnapshotport.CurrentSelectionV2,
			capability datasetsnapshotport.CurrentSelectionCapabilityV2,
		) error {
			if !selectionCalls.CompareAndSwap(0, 1) || dependencyIsNilV1(capability) {
				return ErrPrivateStateIntegrity
			}
			return capability.UseExact(
				selection,
				securityContext,
				func(leaseContext context.Context) error {
					if !exactCalls.CompareAndSwap(0, 1) || leaseContext == nil {
						return ErrPrivateStateIntegrity
					}
					if err := leaseContext.Err(); err != nil {
						return err
					}
					if err := service.validateCurrent(leaseContext, securityContext); err != nil {
						return privateCurrentValidationErrorV1(err)
					}
					operationErr = use(leaseContext, selection)
					currentErr := service.validateCurrent(leaseContext, securityContext)
					afterObservation, observationErr := service.bindingObserver.Observe(
						securityContext.WorkspaceRealPath,
					)
					if observationErr != nil || !privateStateObservationMatchesContextV1(
						afterObservation,
						securityContext,
					) {
						observationErr = ErrPrivateStateUnavailable
					}
					return errors.Join(privateCurrentValidationErrorV1(currentErr), observationErr)
				},
			)
		},
	)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return err
		case errors.Is(err, ErrInvalidReferenceInput), errors.Is(err, ErrReferenceDerivation),
			errors.Is(err, ErrReferenceNotFound), errors.Is(err, ErrReferenceAmbiguous),
			errors.Is(err, ErrPrivateStateNotFound), errors.Is(err, ErrPrivateStateConflict),
			errors.Is(err, ErrPrivateStateIntegrity):
			return err
		default:
			return ErrPrivateStateUnavailable
		}
	}
	if selectionCalls.Load() != 1 || exactCalls.Load() != 1 {
		return ErrPrivateStateIntegrity
	}
	if err := service.validateCurrent(ctx, securityContext); err != nil {
		return privateCurrentValidationErrorV1(err)
	}
	afterObservation, err := service.bindingObserver.Observe(securityContext.WorkspaceRealPath)
	if err != nil || !privateStateObservationMatchesContextV1(afterObservation, securityContext) {
		return ErrPrivateStateUnavailable
	}
	return operationErr
}

func privateCurrentValidationErrorV1(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return ErrPrivateStateUnavailable
}

func privateStateObservationMatchesContextV1(
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

// DeriveReferenceV1 is a pure, non-persistent derivation helper. Production
// ingestion uses BindReferenceV1, which acquires and revalidates the live
// TSCV2/DSV2 authority around both derivation and private persistence.
func (service *Service) DeriveReferenceV1(
	ctx context.Context,
	input DeriveReferenceInputV1,
) (domaincaseentity.ReferenceV1, error) {
	sourceExactValue, sourceErr := privateTextValueV1(input.sourceExactValueUseV1)
	if sourceErr != nil {
		return "", ErrInvalidReferenceInput
	}
	canonicalValue, err := validateReferenceInputV1(
		input.SecurityContext,
		input.FinancialAccountType,
		sourceExactValue,
	)
	if err != nil {
		return "", ErrInvalidReferenceInput
	}
	return service.deriveCanonicalReferenceV1(
		ctx,
		input.SecurityContext,
		input.FinancialAccountType,
		canonicalValue,
	)
}

// UseResolvedReferenceV1 enumerates a bounded host-supplied candidate set and
// exposes the unique canonical preimage only inside live TSCV2/DSV2 authority.
func (service *Service) UseResolvedReferenceV1(
	ctx context.Context,
	input ResolveReferenceInputV1,
	use func(context.Context, string) error,
) error {
	if use == nil {
		return ErrInvalidReferenceInput
	}
	return service.withCurrentPrivateStateV1(
		ctx,
		input.SecurityContext,
		func(leaseContext context.Context) error {
			resolved, err := service.resolveReferenceExactV1(leaseContext, input)
			if err != nil {
				return err
			}
			return use(leaseContext, resolved)
		},
	)
}

func (service *Service) resolveReferenceExactV1(
	ctx context.Context,
	input ResolveReferenceInputV1,
) (string, error) {
	candidateSourceExactValues, candidateErr := privateTextSliceValueV1(
		input.candidateSourceExactValuesUseV1,
	)
	if service == nil || service.keyed == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.SecurityContext) != nil ||
		!validFinancialAccountTypeV1(input.FinancialAccountType) ||
		domaincaseentity.ValidateReferenceV1(string(input.Reference)) != nil ||
		candidateErr != nil || len(candidateSourceExactValues) == 0 ||
		len(candidateSourceExactValues) > maxResolveCandidatesV1 {
		return "", ErrInvalidReferenceInput
	}

	seenCanonical := make(map[string]struct{}, len(candidateSourceExactValues))
	resolved := ""
	for _, sourceExactValue := range candidateSourceExactValues {
		canonicalValue, err := domaincontrolledaccount.CanonicalFinancialAccountTextV2(sourceExactValue)
		if err != nil {
			return "", ErrInvalidReferenceInput
		}
		if _, duplicate := seenCanonical[canonicalValue]; duplicate {
			continue
		}
		seenCanonical[canonicalValue] = struct{}{}

		candidate, err := service.deriveCanonicalReferenceV1(
			ctx,
			input.SecurityContext,
			input.FinancialAccountType,
			canonicalValue,
		)
		if err != nil {
			return "", err
		}
		if candidate != input.Reference {
			continue
		}
		if resolved != "" {
			return "", ErrReferenceAmbiguous
		}
		resolved = canonicalValue
	}
	if resolved == "" {
		return "", ErrReferenceNotFound
	}
	return resolved, nil
}

// BindReferenceV1 acquires the current TSCV2/DSV2 exact-use lease and captures
// the private reverse mapping under a no-replace semantic key. The returned
// ReferenceV1 is provider-safe metadata; source-exact bytes remain only in the
// private store.
func (service *Service) BindReferenceV1(
	ctx context.Context,
	input DeriveReferenceInputV1,
) (domaincaseentity.ReferenceV1, error) {
	var reference domaincaseentity.ReferenceV1
	err := service.withCurrentPrivateStateV1(
		ctx,
		input.SecurityContext,
		func(leaseContext context.Context) error {
			var bindErr error
			reference, bindErr = service.bindReferenceExactV1(leaseContext, input)
			return bindErr
		},
	)
	if err != nil {
		return "", err
	}
	return reference, nil
}

func (service *Service) bindReferenceExactV1(
	ctx context.Context,
	input DeriveReferenceInputV1,
) (domaincaseentity.ReferenceV1, error) {
	if service == nil || service.store == nil {
		return "", ErrPrivateStateUnavailable
	}
	reference, err := service.DeriveReferenceV1(ctx, input)
	if err != nil {
		return "", err
	}
	sourceExactValue, sourceErr := privateTextValueV1(input.sourceExactValueUseV1)
	if sourceErr != nil {
		return "", ErrInvalidReferenceInput
	}
	record, err := service.store.EnsureBinding(
		ctx,
		domaincaseentity.NewCaseEntityBindingRecordInputV1(
			input.SecurityContext, input.FinancialAccountType, reference, sourceExactValue,
		),
	)
	if err != nil {
		return "", privateStateErrorV1(err)
	}
	if err := service.verifyBindingRecordV1(ctx, input.SecurityContext, record); err != nil {
		return "", err
	}
	persisted, err := service.store.ResolveBinding(ctx, record.BindingKey)
	if err != nil {
		return "", privateStateErrorV1(err)
	}
	if persisted.RecordDigest != record.RecordDigest || service.verifyBindingRecordV1(
		ctx,
		input.SecurityContext,
		persisted,
	) != nil {
		return "", ErrPrivateStateIntegrity
	}
	return reference, nil
}

type ResolveBoundReferenceInputV1 struct {
	SecurityContext      domainsecurity.TurnSecurityContext
	FinancialAccountType string
	Reference            domaincaseentity.ReferenceV1
}

// ResolveVerifiedBindingByReferenceInputV1 contains only provider-safe scope
// and stable-reference material. The resolved exact value never leaves the
// synchronous current-authority callback.
type ResolveVerifiedBindingByReferenceInputV1 struct {
	SecurityContext domainsecurity.TurnSecurityContext
	Reference       domaincaseentity.ReferenceV1
}

// ResolveVerifiedBindingByAliasInputV1 carries only a closed model alias and
// the exact current case authority. The alias has no meaning without this
// context and cannot name an entity in another case.
type ResolveVerifiedBindingByAliasInputV1 struct {
	SecurityContext domainsecurity.TurnSecurityContext
	Alias           domaincaseentity.ModelEntityAliasV1
}

// UseAccountFlowSubjectInputV1 is constructed only by the host while the
// existing funds-query-source exact callback is live. SourceDescriptor binds
// the alias lookup below to that callback's exact case and snapshot.
type UseAccountFlowSubjectInputV1 struct {
	SecurityContext  domainsecurity.TurnSecurityContext
	SourceDescriptor domainfundsquerysource.DescriptorV1
	Alias            domaincaseentity.ModelEntityAliasV1
	validateCurrent  func(context.Context, domainsecurity.TurnSecurityContext) error
}

func NewUseAccountFlowSubjectInputV1(
	securityContext domainsecurity.TurnSecurityContext,
	sourceDescriptor domainfundsquerysource.DescriptorV1,
	alias domaincaseentity.ModelEntityAliasV1,
	validateCurrent func(context.Context, domainsecurity.TurnSecurityContext) error,
) UseAccountFlowSubjectInputV1 {
	return UseAccountFlowSubjectInputV1{
		SecurityContext: securityContext, SourceDescriptor: sourceDescriptor, Alias: alias,
		validateCurrent: validateCurrent,
	}
}

// UseRetainedBindingByReferenceInputV1 carries the active current context and
// the accepted final's historical context separately. The private binding key
// remains case-scoped, while DSV2 retained selection proves which immutable
// historical snapshot is allowed to use it.
type UseRetainedBindingByReferenceInputV1 struct {
	ActiveSecurityContext     domainsecurity.TurnSecurityContext
	HistoricalSecurityContext domainsecurity.TurnSecurityContext
	Reference                 domaincaseentity.ReferenceV1
}

type UseCaseAcceptedDisplayBindingInputV1 struct {
	SecurityContext domainsecurity.TurnSecurityContext
	BindingDigest   string
}

// DisplayLabelSemanticV1 contains only safe, deterministic attributes already
// derived by the trusted data plane. Source-exact account/card values and
// internal entity references are deliberately absent.
type DisplayLabelSemanticV1 struct {
	Institution string
	AccountType string
}

type UseDisplayLabelInputV1 struct {
	SecurityContext domainsecurity.TurnSecurityContext
	Reference       domaincaseentity.ReferenceV1
	Semantic        DisplayLabelSemanticV1
}

// UseAccountFlowCounterpartyInputV1 is constructed only by the host while the
// existing funds-query-source exact callback is live. SourceDescriptor binds
// the private persistence below to that callback's exact case and snapshot;
// source-exact account text remains closure-backed and has no JSON surface.
type UseAccountFlowCounterpartyInputV1 struct {
	SecurityContext       domainsecurity.TurnSecurityContext
	SourceDescriptor      domainfundsquerysource.DescriptorV1
	FinancialAccountType  string
	Semantic              DisplayLabelSemanticV1
	sourceExactValueUseV1 privateTextUseV1
	validateCurrent       func(context.Context, domainsecurity.TurnSecurityContext) error
}

func NewUseAccountFlowCounterpartyInputV1(
	securityContext domainsecurity.TurnSecurityContext,
	sourceDescriptor domainfundsquerysource.DescriptorV1,
	financialAccountType string,
	sourceExactValue string,
	semantic DisplayLabelSemanticV1,
	validateCurrent func(context.Context, domainsecurity.TurnSecurityContext) error,
) UseAccountFlowCounterpartyInputV1 {
	return UseAccountFlowCounterpartyInputV1{
		SecurityContext:       securityContext,
		SourceDescriptor:      sourceDescriptor,
		FinancialAccountType:  financialAccountType,
		Semantic:              semantic,
		sourceExactValueUseV1: newPrivateTextUseV1(sourceExactValue),
		validateCurrent:       validateCurrent,
	}
}

// UseDisplayLabelV1 derives a natural, non-colliding ordinary label while the
// current TSCV2/DSV2 lease is live. Stable identity comes from the case-private
// binding ordinal; only the final four digits are copied from the canonical
// source value. The source-exact value and internal reference never cross the
// callback boundary.
func (service *Service) UseDisplayLabelV1(
	ctx context.Context,
	input UseDisplayLabelInputV1,
	use func(domaincaseentity.DisplayLabelV1) error,
) error {
	if use == nil {
		return ErrInvalidReferenceInput
	}
	return service.withCurrentPrivateStateV1(
		ctx,
		input.SecurityContext,
		func(leaseContext context.Context) error {
			record, err := service.resolveBindingRecordByReferenceV1(
				leaseContext,
				input.SecurityContext,
				input.Reference,
			)
			if err != nil {
				return err
			}
			var label domaincaseentity.DisplayLabelV1
			if err := record.UseCanonicalValueV1(func(canonicalValue string) error {
				safeSuffix := ""
				if len(canonicalValue) >= 4 {
					safeSuffix = canonicalValue[len(canonicalValue)-4:]
				}
				var labelErr error
				label, labelErr = domaincaseentity.NewDisplayLabelV1(domaincaseentity.DisplayLabelInputV1{
					EntityType:    record.EntityType,
					StableOrdinal: record.StableOrdinal,
					SafeSuffix:    safeSuffix,
					Institution:   input.Semantic.Institution,
					AccountType:   input.Semantic.AccountType,
				})
				return labelErr
			}); err != nil || domaincaseentity.ValidateDisplayLabelV1(label) != nil {
				return ErrPrivateStateIntegrity
			}
			return use(label)
		},
	)
}

// UseAccountFlowCounterpartyV1 binds one exact counterparty account and emits
// only its stable case-scoped reference plus validated natural display label.
// It deliberately does not acquire DatasetSnapshotAuthorityV2: the caller must
// already be inside the exact funds-query-source callback represented by
// SourceDescriptor. Fresh turn/case checks bracket the private store use, and
// the outer source callback performs the authoritative DSV2 postcheck.
func (service *Service) UseAccountFlowCounterpartyV1(
	ctx context.Context,
	input UseAccountFlowCounterpartyInputV1,
	use func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error,
) error {
	sourceExactValue, sourceErr := privateTextValueV1(input.sourceExactValueUseV1)
	canonicalValue, canonicalErr := validateReferenceInputV1(
		input.SecurityContext,
		input.FinancialAccountType,
		sourceExactValue,
	)
	if service == nil || service.keyed == nil || service.store == nil ||
		service.bindingObserver == nil || input.validateCurrent == nil ||
		ctx == nil || use == nil || sourceErr != nil || canonicalErr != nil {
		return ErrInvalidReferenceInput
	}
	safeSuffix := ""
	if len(canonicalValue) >= 4 {
		safeSuffix = canonicalValue[len(canonicalValue)-4:]
	}
	if _, err := domaincaseentity.NewDisplayLabelV1(domaincaseentity.DisplayLabelInputV1{
		EntityType:    input.FinancialAccountType,
		StableOrdinal: 1,
		SafeSuffix:    safeSuffix,
		Institution:   input.Semantic.Institution,
		AccountType:   input.Semantic.AccountType,
	}); err != nil {
		return ErrInvalidReferenceInput
	}
	if !accountFlowDescriptorMatchesCurrentV1(input.SourceDescriptor, input.SecurityContext) {
		return ErrPrivateStateUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := input.validateCurrent(ctx, input.SecurityContext); err != nil {
		return privateCurrentValidationErrorV1(err)
	}
	observation, err := service.bindingObserver.Observe(input.SecurityContext.WorkspaceRealPath)
	if err != nil || !privateStateObservationMatchesContextV1(observation, input.SecurityContext) {
		return ErrPrivateStateUnavailable
	}

	reference, err := service.bindReferenceExactV1(
		ctx,
		NewDeriveReferenceInputV1(
			input.SecurityContext,
			input.FinancialAccountType,
			canonicalValue,
		),
	)
	if err != nil {
		return err
	}
	record, err := service.resolveBindingRecordV1(
		ctx,
		input.SecurityContext,
		input.FinancialAccountType,
		reference,
	)
	if err != nil {
		return err
	}
	label, err := domaincaseentity.NewDisplayLabelV1(domaincaseentity.DisplayLabelInputV1{
		EntityType:    record.EntityType,
		StableOrdinal: record.StableOrdinal,
		SafeSuffix:    safeSuffix,
		Institution:   input.Semantic.Institution,
		AccountType:   input.Semantic.AccountType,
	})
	if err != nil || domaincaseentity.ValidateDisplayLabelV1(label) != nil {
		return ErrPrivateStateIntegrity
	}
	operationErr := use(reference, label)
	if err := input.validateCurrent(ctx, input.SecurityContext); err != nil {
		return privateCurrentValidationErrorV1(err)
	}
	afterObservation, err := service.bindingObserver.Observe(input.SecurityContext.WorkspaceRealPath)
	if err != nil || afterObservation != observation ||
		!privateStateObservationMatchesContextV1(afterObservation, input.SecurityContext) {
		return ErrPrivateStateUnavailable
	}
	return operationErr
}

func accountFlowDescriptorMatchesCurrentV1(
	descriptor domainfundsquerysource.DescriptorV1,
	securityContext domainsecurity.TurnSecurityContext,
) bool {
	return domainfundsquerysource.ValidateDescriptorV1(descriptor) == nil &&
		domainfundsquerysource.QueryProfileSupportsAccountFlowV1(descriptor.QueryProfileDigest) &&
		descriptor.DatasetSnapshotID == securityContext.DatasetSnapshotID &&
		descriptor.SourceManifestHash == securityContext.SourceManifestHash &&
		descriptor.CaseID == securityContext.CaseID &&
		descriptor.CaseBindingHash == securityContext.CaseBindingHash &&
		descriptor.BindingObservationDigest == securityContext.PublicationPolicy.BindingObservationDigest
}

// UseAccountFlowSubjectV1 resolves one closed account/card alias while the
// caller's exact funds-query-source callback remains live. It deliberately
// acquires no nested DatasetSnapshotAuthorityV2; fresh turn/case checks and
// binding observations bracket the private-store lookup, while the outer
// source callback performs the authoritative DSV2 postcheck.
func (service *Service) UseAccountFlowSubjectV1(
	ctx context.Context,
	input UseAccountFlowSubjectInputV1,
	use func(domaincaseentity.ReferenceV1, string, string) error,
) error {
	entityType, ordinal, aliasErr := domaincaseentity.ParseModelEntityAliasV1(string(input.Alias))
	if service == nil || service.keyed == nil || service.store == nil ||
		service.bindingObserver == nil || input.validateCurrent == nil || ctx == nil || use == nil ||
		aliasErr != nil {
		return ErrInvalidReferenceInput
	}
	if !accountFlowDescriptorMatchesCurrentV1(input.SourceDescriptor, input.SecurityContext) {
		return ErrPrivateStateUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := input.validateCurrent(ctx, input.SecurityContext); err != nil {
		return privateCurrentValidationErrorV1(err)
	}
	observation, err := service.bindingObserver.Observe(input.SecurityContext.WorkspaceRealPath)
	if err != nil || !privateStateObservationMatchesContextV1(observation, input.SecurityContext) {
		return ErrPrivateStateUnavailable
	}

	record, err := service.store.ResolveBindingByStableOrdinal(
		ctx,
		input.SecurityContext,
		entityType,
		ordinal,
	)
	if err != nil {
		if errors.Is(err, caseentityport.ErrNotFound) {
			return ErrReferenceNotFound
		}
		return privateStateErrorV1(err)
	}
	alias, err := domaincaseentity.NewModelEntityAliasV1(record.EntityType, record.StableOrdinal)
	if err != nil || alias != input.Alias || record.EntityType != entityType || record.StableOrdinal != ordinal ||
		service.verifyBindingRecordV1(ctx, input.SecurityContext, record) != nil {
		return ErrPrivateStateIntegrity
	}
	operationErr := record.UseCanonicalValueV1(func(canonicalValue string) error {
		return use(record.Reference, canonicalValue, record.RecordDigest)
	})
	if err := input.validateCurrent(ctx, input.SecurityContext); err != nil {
		return privateCurrentValidationErrorV1(err)
	}
	afterObservation, err := service.bindingObserver.Observe(input.SecurityContext.WorkspaceRealPath)
	if err != nil || afterObservation != observation ||
		!privateStateObservationMatchesContextV1(afterObservation, input.SecurityContext) {
		return ErrPrivateStateUnavailable
	}
	return operationErr
}

// UseVerifiedBindingByReferenceV1 resolves exactly one financial entity type,
// re-derives its case-scoped stable reference, and exposes the canonical value
// only while the existing live TSCV2 and DSV2 authorities remain current.
func (service *Service) UseVerifiedBindingByReferenceV1(
	ctx context.Context,
	input ResolveVerifiedBindingByReferenceInputV1,
	use func(canonicalValue string, recordDigest string) error,
) error {
	if use == nil {
		return ErrInvalidReferenceInput
	}
	return service.withCurrentPrivateStateV1(
		ctx,
		input.SecurityContext,
		func(leaseContext context.Context) error {
			record, err := service.resolveBindingRecordByReferenceV1(
				leaseContext,
				input.SecurityContext,
				input.Reference,
			)
			if err != nil {
				return err
			}
			return record.UseCanonicalValueV1(func(canonicalValue string) error {
				return use(canonicalValue, record.RecordDigest)
			})
		},
	)
}

// UseVerifiedBindingByAliasV1 resolves one closed account/card alias through
// the existing binding inventory while the current TSCV2/DSV2 lease remains
// live. Missing, wrong-prefix/type, duplicate, corrupt, stale, revoked, and
// cross-case bindings all close without reflecting a value or authority ref
// through the returned error.
func (service *Service) UseVerifiedBindingByAliasV1(
	ctx context.Context,
	input ResolveVerifiedBindingByAliasInputV1,
	use func(domaincaseentity.ReferenceV1, string, string) error,
) error {
	if use == nil {
		return ErrInvalidReferenceInput
	}
	entityType, ordinal, err := domaincaseentity.ParseModelEntityAliasV1(string(input.Alias))
	if err != nil {
		return ErrInvalidReferenceInput
	}
	return service.withCurrentPrivateStateV1(
		ctx,
		input.SecurityContext,
		func(leaseContext context.Context) error {
			record, resolveErr := service.store.ResolveBindingByStableOrdinal(
				leaseContext,
				input.SecurityContext,
				entityType,
				ordinal,
			)
			if resolveErr != nil {
				if errors.Is(resolveErr, caseentityport.ErrNotFound) {
					return ErrReferenceNotFound
				}
				return privateStateErrorV1(resolveErr)
			}
			alias, aliasErr := domaincaseentity.NewModelEntityAliasV1(
				record.EntityType,
				record.StableOrdinal,
			)
			if aliasErr != nil || alias != input.Alias ||
				service.verifyBindingRecordV1(leaseContext, input.SecurityContext, record) != nil {
				return ErrPrivateStateIntegrity
			}
			return record.UseCanonicalValueV1(func(canonicalValue string) error {
				return use(record.Reference, canonicalValue, record.RecordDigest)
			})
		},
	)
}

// UseRetainedBindingByReferenceV1 resolves one accepted-final entity binding
// against its historical immutable snapshot while the active case remains
// current. The exact private value never leaves the retained DSV2 callback,
// and the active TSCV2/binding checks bracket the whole operation.
func (service *Service) UseRetainedBindingByReferenceV1(
	ctx context.Context,
	input UseRetainedBindingByReferenceInputV1,
	use func(string, string) error,
) error {
	if use == nil {
		return ErrInvalidReferenceInput
	}
	active := input.ActiveSecurityContext
	historical := input.HistoricalSecurityContext
	if service == nil || service.store == nil || service.keyed == nil ||
		service.bindingObserver == nil || service.validateCurrent == nil ||
		service.datasetAuthority == nil || ctx == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(active) != nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(historical) != nil ||
		domaincaseentity.ValidateReferenceV1(string(input.Reference)) != nil ||
		!retainedContextsShareCaseScopeV1(active, historical) {
		return ErrPrivateStateUnavailable
	}
	reader, ok := service.datasetAuthority.(datasetsnapshotport.RetainedSelectionReaderV2)
	if !ok || dependencyIsNilV1(reader) {
		return ErrPrivateStateUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := service.validateCurrent(ctx, active); err != nil {
		return privateCurrentValidationErrorV1(err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	observation, err := service.bindingObserver.Observe(active.WorkspaceRealPath)
	if err != nil || !privateStateObservationMatchesContextV1(observation, active) {
		return ErrPrivateStateUnavailable
	}
	retainedInput := datasetsnapshotport.RetainedSelectionInputV2{
		CurrentResolveInput: datasetsnapshotport.ResolveInputV2{
			TenantID: active.TenantID, UserID: active.UserID,
			Observation: observation, ExpectedDatasetSnapshotID: active.DatasetSnapshotID,
		},
		CurrentSecurityContext:     active,
		RetainedDatasetSnapshotID:  historical.DatasetSnapshotID,
		RetainedSourceManifestHash: historical.SourceManifestHash,
	}
	var (
		callbackCalls atomic.Uint32
		operationErr  error
	)
	err = reader.WithRetainedSelectionV2(
		ctx,
		retainedInput,
		func(
			leaseContext context.Context,
			retained datasetsnapshotport.RetainedSelectionV2,
		) error {
			if leaseContext == nil || !callbackCalls.CompareAndSwap(0, 1) ||
				!retainedSelectionMatchesContextsV1(retained, active, historical) {
				return ErrPrivateStateIntegrity
			}
			record, resolveErr := service.resolveBindingRecordByReferenceV1(
				leaseContext, historical, input.Reference,
			)
			if resolveErr != nil {
				return resolveErr
			}
			return record.UseCanonicalValueV1(func(canonicalValue string) error {
				operationErr = use(canonicalValue, record.RecordDigest)
				return operationErr
			})
		},
	)
	if err != nil {
		return privateRetainedBindingUseErrorV1(err)
	}
	if callbackCalls.Load() != 1 {
		return ErrPrivateStateIntegrity
	}
	if err := service.validateCurrent(ctx, active); err != nil {
		return privateCurrentValidationErrorV1(err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	afterObservation, err := service.bindingObserver.Observe(active.WorkspaceRealPath)
	if err != nil || afterObservation != observation ||
		!privateStateObservationMatchesContextV1(afterObservation, active) {
		return ErrPrivateStateUnavailable
	}
	return operationErr
}

// UseCaseAcceptedDisplayBindingV1 resolves one opaque display-binding digest
// only from the active thread's inherited private longitudinal record and the
// exact current case index. Original thread, turn, and accepted-final identity
// are never accepted from the caller.
func (service *Service) UseCaseAcceptedDisplayBindingV1(
	ctx context.Context,
	input UseCaseAcceptedDisplayBindingInputV1,
	use func(domaincaseentity.CaseAcceptedDisplayBindingV1) error,
) error {
	if service == nil || service.store == nil || service.bindingObserver == nil || service.validateCurrent == nil ||
		use == nil || ctx == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.SecurityContext) != nil ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(input.BindingDigest)) ||
		input.BindingDigest != strings.TrimSpace(input.BindingDigest) {
		return ErrPrivateStateUnavailable
	}
	if err := service.validateCurrent(ctx, input.SecurityContext); err != nil {
		return privateCurrentValidationErrorV1(err)
	}
	observation, err := service.bindingObserver.Observe(input.SecurityContext.WorkspaceRealPath)
	if err != nil || !privateStateObservationMatchesContextV1(observation, input.SecurityContext) {
		return ErrPrivateStateUnavailable
	}
	operationErr := func() error {
		index, err := service.store.ResolveLatestCaseLongitudinalContext(ctx, input.SecurityContext)
		if err != nil || index.CurrentDatasetSnapshotID != input.SecurityContext.DatasetSnapshotID ||
			index.CurrentContextEpoch != input.SecurityContext.ContextEpoch {
			return ErrPrivateStateIntegrity
		}
		thread, err := service.store.ResolveLatestThreadContextForScope(
			ctx, input.SecurityContext, input.SecurityContext.ThreadID,
		)
		if err != nil || thread.TenantID != input.SecurityContext.TenantID ||
			thread.UserID != input.SecurityContext.UserID || thread.CaseID != input.SecurityContext.CaseID ||
			thread.CaseBindingHash != input.SecurityContext.CaseBindingHash ||
			thread.ThreadID != input.SecurityContext.ThreadID ||
			thread.CurrentDatasetSnapshotID != input.SecurityContext.DatasetSnapshotID ||
			thread.CurrentContextEpoch != input.SecurityContext.ContextEpoch {
			return ErrPrivateStateIntegrity
		}
		resolve := func(bindings []domaincaseentity.CaseAcceptedDisplayBindingV1) (domaincaseentity.CaseAcceptedDisplayBindingV1, error) {
			var resolved domaincaseentity.CaseAcceptedDisplayBindingV1
			for _, binding := range bindings {
				if binding.BindingDigest != input.BindingDigest {
					continue
				}
				if resolved.BindingDigest != "" {
					return domaincaseentity.CaseAcceptedDisplayBindingV1{}, ErrPrivateStateIntegrity
				}
				resolved = binding
			}
			if resolved.BindingDigest == "" {
				return domaincaseentity.CaseAcceptedDisplayBindingV1{}, ErrPrivateStateNotFound
			}
			return resolved, nil
		}
		threadBinding, err := resolve(thread.DisplayBindings)
		if err != nil {
			return err
		}
		indexBinding, err := resolve(index.DisplayBindings)
		if err != nil || !reflect.DeepEqual(threadBinding, indexBinding) ||
			domaincaseentity.ValidateCaseAcceptedDisplayBindingV1(threadBinding) != nil {
			return ErrPrivateStateIntegrity
		}
		entityBinding, err := service.resolveBindingRecordByReferenceV1(
			ctx, input.SecurityContext, threadBinding.EntityReference,
		)
		if err != nil || entityBinding.RecordDigest != threadBinding.EntityBindingDigest {
			return ErrPrivateStateIntegrity
		}
		if err := use(threadBinding); err != nil {
			return ErrPrivateStateUnavailable
		}
		return nil
	}()
	if operationErr != nil {
		return operationErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := service.validateCurrent(ctx, input.SecurityContext); err != nil {
		return privateCurrentValidationErrorV1(err)
	}
	afterObservation, err := service.bindingObserver.Observe(input.SecurityContext.WorkspaceRealPath)
	if err != nil || afterObservation != observation ||
		!privateStateObservationMatchesContextV1(afterObservation, input.SecurityContext) {
		return ErrPrivateStateUnavailable
	}
	return nil
}

func retainedContextsShareCaseScopeV1(
	active domainsecurity.TurnSecurityContext,
	historical domainsecurity.TurnSecurityContext,
) bool {
	return active.TenantID == historical.TenantID &&
		active.UserID == historical.UserID &&
		active.WorkspaceRealPath == historical.WorkspaceRealPath &&
		active.CaseID == historical.CaseID &&
		active.CaseBindingHash == historical.CaseBindingHash
}

func retainedSelectionMatchesContextsV1(
	retained datasetsnapshotport.RetainedSelectionV2,
	active domainsecurity.TurnSecurityContext,
	historical domainsecurity.TurnSecurityContext,
) bool {
	current := retained.Current.Snapshot
	snapshot := retained.Snapshot
	currentBinding := current.Record.Binding
	binding := snapshot.Record.Binding
	return current.Record.DatasetSnapshotID == active.DatasetSnapshotID &&
		current.Record.SourceManifestHash == active.SourceManifestHash &&
		current.Manifest.SourceManifestHash == active.SourceManifestHash &&
		current.Record.Binding == current.Manifest.Binding &&
		currentBinding.TenantID == active.TenantID && currentBinding.UserID == active.UserID &&
		currentBinding.WorkspaceRealPath == active.WorkspaceRealPath &&
		currentBinding.CaseID == active.CaseID &&
		currentBinding.CaseBindingHash == active.CaseBindingHash &&
		currentBinding.BindingObservationDigest == active.PublicationPolicy.BindingObservationDigest &&
		snapshot.Record.DatasetSnapshotID == historical.DatasetSnapshotID &&
		snapshot.Record.SourceManifestHash == historical.SourceManifestHash &&
		snapshot.Manifest.SourceManifestHash == historical.SourceManifestHash &&
		binding.TenantID == active.TenantID && binding.UserID == active.UserID &&
		binding.WorkspaceRealPath == active.WorkspaceRealPath &&
		binding.CaseID == active.CaseID &&
		binding.CaseBindingHash == active.CaseBindingHash &&
		binding.BindingObservationDigest == active.PublicationPolicy.BindingObservationDigest &&
		binding == snapshot.Manifest.Binding
}

func privateRetainedBindingUseErrorV1(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(err, ErrInvalidReferenceInput) || errors.Is(err, ErrReferenceDerivation) ||
		errors.Is(err, ErrReferenceNotFound) || errors.Is(err, ErrReferenceAmbiguous) ||
		errors.Is(err, ErrPrivateStateNotFound) || errors.Is(err, ErrPrivateStateConflict) ||
		errors.Is(err, ErrPrivateStateIntegrity) {
		return err
	}
	return ErrPrivateStateUnavailable
}

// UseBoundReferenceV1 exposes private canonical source bytes only inside the
// current TSCV2/DSV2 callback and completes their post-use checks before
// returning.
func (service *Service) UseBoundReferenceV1(
	ctx context.Context,
	input ResolveBoundReferenceInputV1,
	use func(string) error,
) error {
	if use == nil {
		return ErrInvalidReferenceInput
	}
	return service.withCurrentPrivateStateV1(
		ctx,
		input.SecurityContext,
		func(leaseContext context.Context) error {
			canonicalValue, err := service.resolveBoundReferenceExactV1(leaseContext, input)
			if err != nil {
				return err
			}
			return use(canonicalValue)
		},
	)
}

func (service *Service) resolveBoundReferenceExactV1(
	ctx context.Context,
	input ResolveBoundReferenceInputV1,
) (string, error) {
	if service == nil || service.store == nil || service.keyed == nil {
		return "", ErrPrivateStateUnavailable
	}
	bindingKey, err := domaincaseentity.CaseEntityBindingLookupKeyV1(
		input.SecurityContext,
		input.FinancialAccountType,
		input.Reference,
	)
	if err != nil {
		return "", ErrInvalidReferenceInput
	}
	record, err := service.store.ResolveBinding(ctx, bindingKey)
	if err != nil {
		if errors.Is(err, caseentityport.ErrNotFound) {
			return "", ErrReferenceNotFound
		}
		return "", privateStateErrorV1(err)
	}
	if record.EntityType != input.FinancialAccountType ||
		record.Reference != input.Reference ||
		service.verifyBindingRecordV1(ctx, input.SecurityContext, record) != nil {
		return "", ErrPrivateStateIntegrity
	}
	var canonicalValue string
	if err := record.UseCanonicalValueV1(func(value string) error {
		canonicalValue = value
		return nil
	}); err != nil {
		return "", ErrPrivateStateIntegrity
	}
	return canonicalValue, nil
}

// AccountIngressCandidateBatchV1 carries one deduplicated ingress batch behind
// a one-use callback. It deliberately has no ordinary serialization or
// formatting path. The batch resolver may acquire the existing effect and
// DSV2 exact-source lease, but it must finish before CompileAccountIngressV1
// enters the separate private-state exact lease.
type AccountIngressCandidateBatchV1 struct {
	candidateCount uint32
	useExactV1     privateTextSliceUseV1
	useAttemptedV1 *atomic.Bool
	useCompletedV1 *atomic.Bool
}

func newAccountIngressCandidateBatchV1(values []string) AccountIngressCandidateBatchV1 {
	privateValues := append([]string(nil), values...)
	attempted := &atomic.Bool{}
	completed := &atomic.Bool{}
	return AccountIngressCandidateBatchV1{
		candidateCount: uint32(len(privateValues)),
		useAttemptedV1: attempted,
		useCompletedV1: completed,
		useExactV1: func(use func([]string) error) error {
			if use == nil || !attempted.CompareAndSwap(false, true) {
				return ErrAccountIngressResolution
			}
			if err := use(append([]string(nil), privateValues...)); err != nil {
				return err
			}
			if !completed.CompareAndSwap(false, true) {
				return ErrAccountIngressResolution
			}
			return nil
		},
	}
}

func (batch AccountIngressCandidateBatchV1) CandidateCountV1() uint32 {
	return batch.candidateCount
}

func (batch AccountIngressCandidateBatchV1) UseExactV1(
	use func([]string) error,
) error {
	if batch.candidateCount == 0 || batch.useExactV1 == nil ||
		batch.useAttemptedV1 == nil || batch.useCompletedV1 == nil {
		return ErrAccountIngressResolution
	}
	return batch.useExactV1(use)
}

func (batch AccountIngressCandidateBatchV1) exactUseCompletedV1() bool {
	return batch.useCompletedV1 != nil && batch.useCompletedV1.Load()
}

// ResolveAccountIngressCandidatesV1 is a host-owned batch lookup against the
// exact current source for SecurityContext. It receives complete candidates
// only through AccountIngressCandidateBatchV1.UseExactV1 and returns no raw
// value or raw-derived identifier. A production resolver owns one bounded,
// read-only native query under one effect + DSV2 lease and returns only after
// that lease is closed. Unknown, ambiguous, stale, unavailable, partial, or
// out-of-scope lookups return an error or a closed negative disposition.
type ResolveAccountIngressCandidatesV1 func(
	context.Context,
	domainsecurity.TurnSecurityContext,
	AccountIngressCandidateBatchV1,
) (AccountIngressResolutionBatchV1, error)

type AccountIngressResolutionDispositionV1 uint8

const (
	AccountIngressResolutionInvalidV1 AccountIngressResolutionDispositionV1 = iota
	AccountIngressResolutionResolvedAccountV1
	AccountIngressResolutionVerifiedNoCurrentAccountMatchV1
)

type AccountIngressCandidateResolutionV1 struct {
	Ordinal              uint32
	Disposition          AccountIngressResolutionDispositionV1
	FinancialAccountType string
	BankInstitution      string
	AccountType          string
}

// AccountIngressResolutionBatchV1 is safe host metadata. Candidate values and
// their digests are absent; ordinal order is meaningful only to the private
// caller that owns the one-use candidate batch.
type AccountIngressResolutionBatchV1 struct {
	DatasetSnapshotID string
	ContextEpoch      uint64
	ContextDigest     string
	CaseBindingHash   string
	Resolutions       []AccountIngressCandidateResolutionV1
	sourceDescriptor  domainfundsquerysource.DescriptorV1
	nativeResult      domainnative.ResolveAccountIngressResultV1
}

func NewAccountIngressResolutionBatchV1(
	securityContext domainsecurity.TurnSecurityContext,
	sourceDescriptor domainfundsquerysource.DescriptorV1,
	nativeResult domainnative.ResolveAccountIngressResultV1,
) (AccountIngressResolutionBatchV1, error) {
	if domainnative.ValidateResolveAccountIngressResultAuthorityV1(
		nativeResult,
		securityContext,
		sourceDescriptor,
	) != nil || domainnative.ResolveAccountIngressResultHasFailClosedDispositionV1(nativeResult) {
		return AccountIngressResolutionBatchV1{}, ErrAccountIngressResolution
	}
	resolutions := make([]AccountIngressCandidateResolutionV1, len(nativeResult.Resolutions))
	for index, nativeResolution := range nativeResult.Resolutions {
		if nativeResolution.Ordinal != uint32(index) {
			return AccountIngressResolutionBatchV1{}, ErrAccountIngressResolution
		}
		resolution := AccountIngressCandidateResolutionV1{Ordinal: uint32(index)}
		switch nativeResolution.Disposition {
		case domainnative.AccountIngressResolutionDispositionResolvedV1:
			resolution.Disposition = AccountIngressResolutionResolvedAccountV1
			resolution.FinancialAccountType = nativeResolution.EntityType
			resolution.BankInstitution = nativeResolution.BankInstitution
			resolution.AccountType = nativeResolution.AccountType
		case domainnative.AccountIngressResolutionDispositionNotFoundV1:
			resolution.Disposition = AccountIngressResolutionVerifiedNoCurrentAccountMatchV1
		default:
			return AccountIngressResolutionBatchV1{}, ErrAccountIngressResolution
		}
		resolutions[index] = resolution
	}
	nativeResult.Resolutions = append(
		[]domainnative.AccountIngressResolutionV1(nil),
		nativeResult.Resolutions...,
	)
	batch := AccountIngressResolutionBatchV1{
		DatasetSnapshotID: securityContext.DatasetSnapshotID,
		ContextEpoch:      securityContext.ContextEpoch,
		ContextDigest:     securityContext.ContextDigest,
		CaseBindingHash:   securityContext.CaseBindingHash,
		Resolutions:       append([]AccountIngressCandidateResolutionV1(nil), resolutions...),
		sourceDescriptor:  sourceDescriptor,
		nativeResult:      nativeResult,
	}
	if validateAccountIngressResolutionBatchV1(batch, securityContext, len(resolutions)) != nil {
		return AccountIngressResolutionBatchV1{}, ErrAccountIngressResolution
	}
	return batch, nil
}

func validateAccountIngressResolutionBatchV1(
	batch AccountIngressResolutionBatchV1,
	securityContext domainsecurity.TurnSecurityContext,
	candidateCount int,
) error {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		candidateCount <= 0 || candidateCount > maxAccountIngressUniqueValuesV1 ||
		batch.DatasetSnapshotID != securityContext.DatasetSnapshotID ||
		batch.ContextEpoch != securityContext.ContextEpoch ||
		batch.ContextDigest != securityContext.ContextDigest ||
		batch.CaseBindingHash != securityContext.CaseBindingHash ||
		domainfundsquerysource.ValidateDescriptorV1(batch.sourceDescriptor) != nil ||
		!domainfundsquerysource.QueryProfileSupportsAccountIngressResolutionV1(
			batch.sourceDescriptor.QueryProfileDigest,
		) ||
		batch.sourceDescriptor.DatasetSnapshotID != securityContext.DatasetSnapshotID ||
		batch.sourceDescriptor.SourceManifestHash != securityContext.SourceManifestHash ||
		batch.sourceDescriptor.CaseID != securityContext.CaseID ||
		batch.sourceDescriptor.CaseBindingHash != securityContext.CaseBindingHash ||
		batch.sourceDescriptor.BindingObservationDigest !=
			securityContext.PublicationPolicy.BindingObservationDigest ||
		domainnative.ValidateResolveAccountIngressResultAuthorityV1(
			batch.nativeResult,
			securityContext,
			batch.sourceDescriptor,
		) != nil ||
		domainnative.ResolveAccountIngressResultHasFailClosedDispositionV1(batch.nativeResult) ||
		len(batch.nativeResult.Resolutions) != candidateCount ||
		len(batch.Resolutions) != candidateCount {
		return ErrAccountIngressResolution
	}
	for index, resolution := range batch.Resolutions {
		nativeResolution := batch.nativeResult.Resolutions[index]
		if resolution.Ordinal != uint32(index) || nativeResolution.Ordinal != uint32(index) {
			return ErrAccountIngressResolution
		}
		switch resolution.Disposition {
		case AccountIngressResolutionResolvedAccountV1:
			if nativeResolution.Disposition !=
				domainnative.AccountIngressResolutionDispositionResolvedV1 ||
				resolution.FinancialAccountType != nativeResolution.EntityType ||
				resolution.BankInstitution != nativeResolution.BankInstitution ||
				resolution.AccountType != nativeResolution.AccountType ||
				!validFinancialAccountTypeV1(resolution.FinancialAccountType) {
				return ErrAccountIngressResolution
			}
			if _, err := domaincaseentity.NewDisplayLabelV1(domaincaseentity.DisplayLabelInputV1{
				EntityType:    resolution.FinancialAccountType,
				StableOrdinal: 1,
				Institution:   resolution.BankInstitution,
				AccountType:   resolution.AccountType,
			}); err != nil {
				return ErrAccountIngressResolution
			}
		case AccountIngressResolutionVerifiedNoCurrentAccountMatchV1:
			if nativeResolution.Disposition !=
				domainnative.AccountIngressResolutionDispositionNotFoundV1 ||
				resolution.FinancialAccountType != "" ||
				resolution.BankInstitution != "" || resolution.AccountType != "" {
				return ErrAccountIngressResolution
			}
		default:
			return ErrAccountIngressResolution
		}
	}
	return nil
}

// CompileAccountIngressInputV1 keeps source text and the current-snapshot
// resolver behind host-private fields. Neither is available to ordinary JSON
// or formatting paths.
type CompileAccountIngressInputV1 struct {
	SecurityContext  domainsecurity.TurnSecurityContext
	IngressKind      string
	IngressOrdinal   uint32
	rawTextUseV1     privateTextUseV1
	resolveV1        ResolveAccountIngressCandidatesV1
	relationV1       string
	sourceThreadIDV1 string
}

func NewCompileAccountIngressInputV1(
	securityContext domainsecurity.TurnSecurityContext,
	ingressKind string,
	ingressOrdinal uint32,
	rawText string,
	resolve ResolveAccountIngressCandidatesV1,
) CompileAccountIngressInputV1 {
	return CompileAccountIngressInputV1{
		SecurityContext: securityContext,
		IngressKind:     ingressKind,
		IngressOrdinal:  ingressOrdinal,
		rawTextUseV1:    newPrivateTextUseV1(rawText),
		resolveV1:       resolve,
	}
}

// WithCaseContinuityV1 binds source-exact ingress admission to the same typed
// fork/resume/restart classification used by alias-only continuity. The source
// thread is checked before any new binding or ingress record is persisted.
func (input CompileAccountIngressInputV1) WithCaseContinuityV1(
	relation string,
	sourceThreadID string,
) CompileAccountIngressInputV1 {
	input.relationV1 = strings.TrimSpace(relation)
	input.sourceThreadIDV1 = strings.TrimSpace(sourceThreadID)
	return input
}

type privateRecordReferenceUseV1 func(func(PrivateRecordReferenceV1) error) error

type privateIngressEntitySelectionUseV1 func(
	func(PrivateRecordReferenceV1, []domaincaseentity.ReferenceV1, []domaincaseentity.ModelEntityAliasV1) error,
) error

// providerIngressEntityDescriptorV1 is assembled only from the trusted native
// resolution which admitted an ingress identity. It deliberately has no
// canonical account value, source offset, private record identity, or
// raw-derived digest.
type providerIngressEntityDescriptorV1 struct {
	reference            domaincaseentity.ReferenceV1
	alias                domaincaseentity.ModelEntityAliasV1
	entityType           string
	financialAccountType string
	bankInstitution      string
	accountType          string
}

// ProviderIngressDescriptorConsumerV1 receives one provider-safe identity
// descriptor in deterministic first-occurrence order. The native contract
// currently names the precise financial account subtype as entityType, so the
// app exposes both that source semantic and its financialAccountType meaning;
// they must remain equal until the trusted source contract separates them.
type ProviderIngressDescriptorConsumerV1 func(
	ordinal uint32,
	alias domaincaseentity.ModelEntityAliasV1,
	entityType string,
	financialAccountType string,
	bankInstitution string,
	accountType string,
) error

// ProviderIngressDescriptorsUseV1 is valid only during the surrounding
// ProviderIngressProjectionV1 callback and may be consumed exactly once.
type ProviderIngressDescriptorsUseV1 func(ProviderIngressDescriptorConsumerV1) error

type providerIngressProjectionUseV1 func(
	withDescriptors bool,
	use func(string, []providerIngressEntityDescriptorV1, ProviderIngressLongitudinalStateV1) error,
) error

type ProviderIngressClaimDigestV1 struct {
	Digest             string
	InvestigationState string
}

type ProviderIngressEvidenceDigestV1 struct {
	Digest      string
	Currentness string
}

type ProviderIngressContinuationDigestV1 struct {
	Digest      string
	Currentness string
}

const (
	ProviderIngressLongitudinalSchemaVersionV1   = 1
	ProviderIngressLongitudinalSelectionBudgetV1 = 32

	ProviderIngressLongitudinalCurrentVerifiedFactV1      = "current_verified_fact"
	ProviderIngressLongitudinalHistoricalComparisonFactV1 = "historical_comparison_fact"
	ProviderIngressLongitudinalKeyRelationshipV1          = "key_relationship"
	ProviderIngressLongitudinalCounterevidenceRefutedV1   = "counterevidence_refuted_finding"
	ProviderIngressLongitudinalDataGapV1                  = "data_gap"
	ProviderIngressLongitudinalEvidenceClaimReferenceV1   = "evidence_claim_reference"
	ProviderIngressLongitudinalSnapshotDifferenceV1       = "snapshot_difference"

	ProviderIngressLongitudinalReferenceClaimV1        = "claim"
	ProviderIngressLongitudinalReferenceEvidenceV1     = "evidence"
	ProviderIngressLongitudinalReferenceContinuationV1 = "continuation"
	ProviderIngressLongitudinalReferenceDataGapV1      = "data_gap"
	ProviderIngressLongitudinalReferenceOpenQuestionV1 = "open_question"
	ProviderIngressLongitudinalReferenceSnapshotV1     = "snapshot"
)

type ProviderIngressLongitudinalItemV1 struct {
	Kind                          string
	ReferenceKind                 string
	ReferenceDigest               string
	Digest                        string
	Currentness                   string
	InvestigationState            string
	ClaimType                     string
	EvidenceBindingDigest         string
	EvidenceReferenceCount        uint32
	CounterEvidenceBindingDigest  string
	CounterEvidenceReferenceCount uint32
	SnapshotBindingDigest         string
	ComparedSnapshotBindingDigest string
}

type ProviderIngressLongitudinalOmittedCoverageV1 struct {
	Kind  string
	Count uint32
}

type ProviderIngressLongitudinalStateV1 struct {
	SchemaVersion      int
	ScopeBindingDigest string
	SelectionBudget    uint32
	Items              []ProviderIngressLongitudinalItemV1
	OmittedTotal       uint32
	OmittedCoverage    []ProviderIngressLongitudinalOmittedCoverageV1

	// These three lists are exact subsets derived from Items for the existing
	// case-delegation protocol. They are never serialized into provider ingress.
	Currentness   string
	Claims        []ProviderIngressClaimDigestV1
	Evidence      []ProviderIngressEvidenceDigestV1
	Continuations []ProviderIngressContinuationDigestV1
}

// ProviderIngressProjectionV1 is a one-use, process-private carrier for text
// containing stable case-entity references and, when compiled from a trusted
// resolver result, deduplicated provider-safe semantic descriptors. It has no
// fields, formatting, or JSON surface from which an ordinary caller can copy
// either payload. Only the provider request assembler consumes it
// synchronously.
type ProviderIngressProjectionV1 struct {
	useExactV1 providerIngressProjectionUseV1
}

func newProviderIngressProjectionV1(
	text string,
	descriptors ...providerIngressEntityDescriptorV1,
) ProviderIngressProjectionV1 {
	return newProviderIngressProjectionWithLongitudinalStateV1(
		text,
		ProviderIngressLongitudinalStateV1{},
		descriptors...,
	)
}

func newProviderIngressProjectionWithLongitudinalStateV1(
	text string,
	longitudinal ProviderIngressLongitudinalStateV1,
	descriptors ...providerIngressEntityDescriptorV1,
) ProviderIngressProjectionV1 {
	privateDescriptors := append([]providerIngressEntityDescriptorV1(nil), descriptors...)
	privateLongitudinal := cloneProviderIngressLongitudinalStateV1(longitudinal)
	var used atomic.Bool
	return ProviderIngressProjectionV1{useExactV1: func(
		_ bool,
		use func(string, []providerIngressEntityDescriptorV1, ProviderIngressLongitudinalStateV1) error,
	) error {
		if use == nil || !used.CompareAndSwap(false, true) ||
			domaincaseentity.ContainsReferenceCandidateV1(text) ||
			validateProviderIngressDescriptorsV1(text, privateDescriptors) != nil ||
			validateProviderIngressLongitudinalStateV1(privateLongitudinal) != nil {
			return ErrPrivateStateIntegrity
		}
		if err := use(
			text,
			append([]providerIngressEntityDescriptorV1(nil), privateDescriptors...),
			cloneProviderIngressLongitudinalStateV1(privateLongitudinal),
		); err != nil {
			// Callback errors are untrusted and may contain the private projection.
			// Never propagate their text to an ordinary error sink.
			return ErrPrivateStateUnavailable
		}
		return nil
	}}
}

func (projection ProviderIngressProjectionV1) UseExactV1(use func(string) error) error {
	if projection.useExactV1 == nil || use == nil {
		return ErrPrivateStateUnavailable
	}
	return projection.useExactV1(false, func(text string, _ []providerIngressEntityDescriptorV1, _ ProviderIngressLongitudinalStateV1) error {
		return use(text)
	})
}

// UseExactWithDescriptorsV1 consumes the provider text and every verified
// semantic descriptor under one synchronous, one-use boundary. The nested
// descriptor callback cannot be retained past the outer callback. Callback
// failures are mapped to a closed error so references or safe private context
// cannot be echoed through an ordinary error surface.
func (projection ProviderIngressProjectionV1) UseExactWithDescriptorsV1(
	use func(string, uint32, ProviderIngressDescriptorsUseV1) error,
) error {
	if projection.useExactV1 == nil || use == nil {
		return ErrPrivateStateUnavailable
	}
	return projection.useExactV1(true, func(
		text string,
		descriptors []providerIngressEntityDescriptorV1,
		_ ProviderIngressLongitudinalStateV1,
	) error {
		var active atomic.Bool
		var descriptorUseStarted atomic.Bool
		active.Store(true)
		defer active.Store(false)
		useDescriptors := func(consume ProviderIngressDescriptorConsumerV1) error {
			if consume == nil || !active.Load() ||
				!descriptorUseStarted.CompareAndSwap(false, true) {
				return ErrPrivateStateUnavailable
			}
			for index, descriptor := range descriptors {
				if validateProviderIngressEntityDescriptorV1(descriptor) != nil {
					return ErrPrivateStateIntegrity
				}
				if err := consume(
					uint32(index),
					descriptor.alias,
					descriptor.entityType,
					descriptor.financialAccountType,
					descriptor.bankInstitution,
					descriptor.accountType,
				); err != nil {
					return ErrPrivateStateUnavailable
				}
			}
			return nil
		}
		if err := use(text, uint32(len(descriptors)), useDescriptors); err != nil {
			return ErrPrivateStateUnavailable
		}
		if len(descriptors) != 0 && !descriptorUseStarted.Load() {
			return ErrPrivateStateUnavailable
		}
		return nil
	})
}

func (projection ProviderIngressProjectionV1) UseExactWithDescriptorsAndStateV1(
	use func(string, uint32, ProviderIngressDescriptorsUseV1, ProviderIngressLongitudinalStateV1) error,
) error {
	if projection.useExactV1 == nil || use == nil {
		return ErrPrivateStateUnavailable
	}
	return projection.useExactV1(true, func(
		text string,
		descriptors []providerIngressEntityDescriptorV1,
		longitudinal ProviderIngressLongitudinalStateV1,
	) error {
		var active atomic.Bool
		var descriptorUseStarted atomic.Bool
		active.Store(true)
		defer active.Store(false)
		useDescriptors := func(consume ProviderIngressDescriptorConsumerV1) error {
			if consume == nil || !active.Load() || !descriptorUseStarted.CompareAndSwap(false, true) {
				return ErrPrivateStateUnavailable
			}
			for index, descriptor := range descriptors {
				if validateProviderIngressEntityDescriptorV1(descriptor) != nil {
					return ErrPrivateStateIntegrity
				}
				if err := consume(uint32(index), descriptor.alias, descriptor.entityType,
					descriptor.financialAccountType, descriptor.bankInstitution, descriptor.accountType); err != nil {
					return ErrPrivateStateUnavailable
				}
			}
			return nil
		}
		if err := use(text, uint32(len(descriptors)), useDescriptors, cloneProviderIngressLongitudinalStateV1(longitudinal)); err != nil {
			return ErrPrivateStateUnavailable
		}
		if len(descriptors) != 0 && !descriptorUseStarted.Load() {
			return ErrPrivateStateUnavailable
		}
		return nil
	})
}

// useDescriptorScopedProviderIngressProjectionV1 limits a recompiled
// projection to the synchronous host callback which requested it. The caller
// must consume both provider text and descriptors through the descriptor-aware
// API before returning; a retained carrier, a text-only use, or an ignored
// carrier fails closed.
func useDescriptorScopedProviderIngressProjectionV1(
	projection ProviderIngressProjectionV1,
	use func(ProviderIngressProjectionV1) error,
) error {
	if projection.useExactV1 == nil || use == nil {
		return ErrPrivateStateUnavailable
	}
	var active atomic.Bool
	var consumeStarted atomic.Bool
	var consumeCompleted atomic.Bool
	active.Store(true)
	defer active.Store(false)
	scoped := ProviderIngressProjectionV1{useExactV1: func(
		withDescriptors bool,
		consume func(string, []providerIngressEntityDescriptorV1, ProviderIngressLongitudinalStateV1) error,
	) error {
		if !withDescriptors || consume == nil || !active.Load() ||
			!consumeStarted.CompareAndSwap(false, true) {
			return ErrPrivateStateUnavailable
		}
		if err := projection.useExactV1(true, consume); err != nil {
			return err
		}
		consumeCompleted.Store(true)
		return nil
	}}
	callbackErr := use(scoped)
	if callbackErr != nil || !consumeStarted.Load() || !consumeCompleted.Load() {
		return ErrPrivateStateUnavailable
	}
	return nil
}

// AccountIngressCompilationV1 deliberately separates provider-only stable
// references from the ordinary durable/public projection. The complete value,
// private record identity, and raw-dependent digest are absent from ordinary
// fields and serialization.
type AccountIngressCompilationV1 struct {
	Status               AccountIngressCompilationStatusV1
	PublicText           string
	SpanCount            uint32
	UniqueReferenceCount uint32
	privateReferenceUse  privateRecordReferenceUseV1
	privateSelectionUse  privateIngressEntitySelectionUseV1
	providerProjectionV1 ProviderIngressProjectionV1
}

func (result AccountIngressCompilationV1) IsNoop() bool {
	return result.Status == AccountIngressCompilationStatusNoopV1
}

func (result AccountIngressCompilationV1) IsPersisted() bool {
	return result.Status == AccountIngressCompilationStatusPersistedV1
}

func (result AccountIngressCompilationV1) IsBlocked() bool {
	return result.Status == AccountIngressCompilationStatusBlockedV1
}

func (result AccountIngressCompilationV1) UsePrivateRecordReferenceV1(
	use func(PrivateRecordReferenceV1) error,
) error {
	if result.privateReferenceUse == nil || use == nil || !result.IsPersisted() {
		return ErrPrivateStateUnavailable
	}
	return result.privateReferenceUse(use)
}

// UsePrivateIngressEntitySelectionV1 returns the persisted ingress identity
// and the exact stable-reference set produced by the same source-resolved
// compilation. It is a one-use host-only bridge for downstream effect scope;
// no raw account value or model-supplied selector participates.
func (result AccountIngressCompilationV1) UsePrivateIngressEntitySelectionV1(
	use func(PrivateRecordReferenceV1, []domaincaseentity.ReferenceV1, []domaincaseentity.ModelEntityAliasV1) error,
) error {
	if result.privateSelectionUse == nil || use == nil || !result.IsPersisted() {
		return ErrPrivateStateUnavailable
	}
	return result.privateSelectionUse(use)
}

// UseProviderProjectionV1 hands the already-persisted compilation's one-use
// provider carrier to the request assembler without making its text or
// descriptors ordinary fields on AccountIngressCompilationV1.
func (result AccountIngressCompilationV1) UseProviderProjectionV1(
	use func(ProviderIngressProjectionV1) error,
) error {
	if !result.IsPersisted() || result.providerProjectionV1.useExactV1 == nil || use == nil {
		return ErrPrivateStateUnavailable
	}
	if err := use(result.providerProjectionV1); err != nil {
		return ErrPrivateStateUnavailable
	}
	return nil
}

// useProviderProjectionV1 is package-private test and invariant support. The
// production server deliberately restores the committed projection by its
// exact private coordinate instead of consuming this transient compilation
// result.
func (result AccountIngressCompilationV1) useProviderProjectionV1(
	use func(string) error,
) error {
	return result.UseProviderProjectionV1(func(projection ProviderIngressProjectionV1) error {
		return projection.UseExactV1(use)
	})
}

type accountIngressRawSpanV1 struct {
	StartByte            int
	EndByte              int
	CanonicalValue       string
	FinancialAccountType string
	Placeholder          string
}

type accountIngressDiscoverySegmentV1 struct {
	ProjectedStartByte int
	ProjectedEndByte   int
	RawStartByte       int
}

// CompileAccountIngressV1 compiles only source-verified current-snapshot
// identities. Every account-shaped candidate is discovered independently of
// labels. All candidates are resolved before any binding is written, so an
// unsupported, unresolved, or ambiguous candidate yields a safe blocked dual
// projection with no private write. A later store failure may leave only an
// idempotent, non-enumerable private binding; no provider/public result or
// reusable private handle is returned, and retry reuses the same binding.
func (service *Service) CompileAccountIngressV1(
	ctx context.Context,
	input CompileAccountIngressInputV1,
) (AccountIngressCompilationV1, error) {
	rawText, rawErr := privateTextValueV1(input.rawTextUseV1)
	if service == nil || ctx == nil || rawErr != nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.SecurityContext) != nil ||
		(input.IngressKind != domaincaseentity.CaseIngressKindTurnV1 &&
			input.IngressKind != domaincaseentity.CaseIngressKindSteerV1) ||
		rawText == "" || len(rawText) > maxAccountIngressTextBytesV1 || !utf8.ValidString(rawText) {
		return AccountIngressCompilationV1{}, ErrInvalidAccountIngress
	}
	if domaincaseentity.ContainsReferenceCandidateV1(rawText) {
		return blockedAccountIngressCompilationV1(rawText), nil
	}

	rawSpans, discoveryErr := accountIngressRawSpansV1(rawText)
	if discoveryErr != nil {
		if errors.Is(discoveryErr, ErrUnsupportedIngressPII) ||
			errors.Is(discoveryErr, ErrAccountIngressProjection) {
			return blockedAccountIngressCompilationV1(rawText), nil
		}
		return AccountIngressCompilationV1{}, discoveryErr
	}
	if len(rawSpans) == 0 {
		return AccountIngressCompilationV1{
			Status:     AccountIngressCompilationStatusNoopV1,
			PublicText: rawText,
		}, nil
	}
	if input.resolveV1 == nil {
		return blockedAccountIngressCompilationV1(rawText), nil
	}
	candidateValues := make([]string, 0, len(rawSpans))
	seenCandidates := make(map[string]bool, len(rawSpans))
	for _, rawSpan := range rawSpans {
		if rawSpan.CanonicalValue == "" {
			return blockedAccountIngressCompilationV1(rawText), nil
		}
		if !seenCandidates[rawSpan.CanonicalValue] {
			seenCandidates[rawSpan.CanonicalValue] = true
			candidateValues = append(candidateValues, rawSpan.CanonicalValue)
		}
	}
	if len(candidateValues) == 0 || len(candidateValues) > maxAccountIngressUniqueValuesV1 {
		return blockedAccountIngressCompilationV1(rawText), nil
	}
	candidateBatch := newAccountIngressCandidateBatchV1(candidateValues)
	resolvedBatch, resolveErr := input.resolveV1(ctx, input.SecurityContext, candidateBatch)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return AccountIngressCompilationV1{}, ctxErr
	}
	if resolveErr != nil || !candidateBatch.exactUseCompletedV1() ||
		validateAccountIngressResolutionBatchV1(
			resolvedBatch,
			input.SecurityContext,
			len(candidateValues),
		) != nil {
		return blockedAccountIngressCompilationV1(rawText), nil
	}
	resolvedByCanonical := make(map[string]AccountIngressCandidateResolutionV1, len(candidateValues))
	for index, canonical := range candidateValues {
		resolvedByCanonical[canonical] = resolvedBatch.Resolutions[index]
	}

	var compiled AccountIngressCompilationV1
	err := service.withCurrentPrivateSelectionV1(
		ctx,
		input.SecurityContext,
		func(
			leaseContext context.Context,
			selection datasetsnapshotport.CurrentSelectionV2,
		) error {
			if !service.accountIngressDescriptorMatchesCurrentSelectionV1(
				resolvedBatch.sourceDescriptor,
				input.SecurityContext,
				selection,
			) {
				return ErrAccountIngressResolution
			}
			resolvedByTypedCanonical := make(map[string]AccountIngressCandidateResolutionV1)
			accountSpans := make([]accountIngressRawSpanV1, 0, len(rawSpans))
			resolutions := make([]AccountIngressCandidateResolutionV1, len(rawSpans))
			for index := range rawSpans {
				rawSpan := &rawSpans[index]
				resolution, resolved := resolvedByCanonical[rawSpan.CanonicalValue]
				if !resolved {
					return ErrAccountIngressResolution
				}
				switch resolution.Disposition {
				case AccountIngressResolutionVerifiedNoCurrentAccountMatchV1:
					if resolution.FinancialAccountType != "" ||
						resolution.BankInstitution != "" || resolution.AccountType != "" {
						return ErrAccountIngressResolution
					}
				case AccountIngressResolutionResolvedAccountV1:
					canonicalAccount, canonicalErr := domaincontrolledaccount.CanonicalFinancialAccountTextV2(
						rawText[rawSpan.StartByte:rawSpan.EndByte],
					)
					if !validFinancialAccountTypeV1(resolution.FinancialAccountType) ||
						canonicalErr != nil || canonicalAccount != rawSpan.CanonicalValue {
						return ErrAccountIngressResolution
					}
				default:
					return ErrAccountIngressResolution
				}
				resolutions[index] = resolution
			}
			for index := range rawSpans {
				rawSpan := &rawSpans[index]
				resolution := resolutions[index]
				if resolution.Disposition == AccountIngressResolutionVerifiedNoCurrentAccountMatchV1 {
					// A source-negative lookup proves only that this span is not a
					// current account. It does not prove that the raw digits are
					// ordinary, in-scope, or safe for a provider/public sink.
					return ErrAccountIngressResolution
				}
				typedKey := resolution.FinancialAccountType + "\x00" + rawSpan.CanonicalValue
				resolvedByTypedCanonical[typedKey] = resolution
				if len(resolvedByTypedCanonical) > maxAccountIngressUniqueValuesV1 {
					return ErrAccountIngressResolution
				}
				rawSpan.FinancialAccountType = resolution.FinancialAccountType
				rawSpan.Placeholder = domainprivacyprojection.Placeholder(
					domainprivacyprojection.KindAccount,
				)
				accountSpans = append(accountSpans, *rawSpan)
			}
			derivedReferencesByTypedCanonical := make(map[string]domaincaseentity.ReferenceV1, len(resolvedByTypedCanonical))
			for _, rawSpan := range accountSpans {
				typedKey := rawSpan.FinancialAccountType + "\x00" + rawSpan.CanonicalValue
				if _, found := derivedReferencesByTypedCanonical[typedKey]; found {
					continue
				}
				reference, deriveErr := service.DeriveReferenceV1(
					leaseContext,
					NewDeriveReferenceInputV1(
						input.SecurityContext,
						rawSpan.FinancialAccountType,
						rawSpan.CanonicalValue,
					),
				)
				if deriveErr != nil {
					return deriveErr
				}
				derivedReferencesByTypedCanonical[typedKey] = reference
			}
			derivedReferences := make([]domaincaseentity.ReferenceV1, 0, len(derivedReferencesByTypedCanonical))
			for _, reference := range derivedReferencesByTypedCanonical {
				derivedReferences = append(derivedReferences, reference)
			}
			sort.Slice(derivedReferences, func(left, right int) bool {
				return derivedReferences[left] < derivedReferences[right]
			})
			if _, _, continuityErr := service.classifyCaseContinuityTransitionExactV1(
				leaseContext,
				input.SecurityContext,
				input.relationV1,
				input.sourceThreadIDV1,
				derivedReferences,
			); continuityErr != nil {
				return continuityErr
			}
			referencesByTypedCanonical := make(map[string]domaincaseentity.ReferenceV1, len(resolvedByTypedCanonical))
			aliasesByTypedCanonical := make(map[string]domaincaseentity.ModelEntityAliasV1, len(resolvedByTypedCanonical))
			for _, rawSpan := range accountSpans {
				typedKey := rawSpan.FinancialAccountType + "\x00" + rawSpan.CanonicalValue
				if _, found := referencesByTypedCanonical[typedKey]; found {
					continue
				}
				reference, bindErr := service.bindReferenceExactV1(
					leaseContext,
					NewDeriveReferenceInputV1(
						input.SecurityContext,
						rawSpan.FinancialAccountType,
						rawSpan.CanonicalValue,
					),
				)
				if bindErr != nil {
					return bindErr
				}
				if reference != derivedReferencesByTypedCanonical[typedKey] {
					return ErrPrivateStateIntegrity
				}
				referencesByTypedCanonical[typedKey] = reference
				record, resolveErr := service.resolveBindingRecordV1(
					leaseContext,
					input.SecurityContext,
					rawSpan.FinancialAccountType,
					reference,
				)
				if resolveErr != nil {
					return resolveErr
				}
				alias, aliasErr := domaincaseentity.NewModelEntityAliasV1(record.EntityType, record.StableOrdinal)
				if aliasErr != nil {
					return ErrAccountIngressProjection
				}
				aliasesByTypedCanonical[typedKey] = alias
			}
			providerDescriptors, descriptorErr := compileProviderIngressDescriptorsV1(
				accountSpans,
				referencesByTypedCanonical,
				aliasesByTypedCanonical,
				resolvedByTypedCanonical,
			)
			if descriptorErr != nil {
				return descriptorErr
			}

			providerText, publicText, persistedSpans, projectionErr := projectAccountIngressTextV1(
				rawText,
				accountSpans,
				referencesByTypedCanonical,
				aliasesByTypedCanonical,
			)
			if projectionErr != nil {
				return projectionErr
			}
			privateProjectedText, privatePersistedSpans, privateProjectionErr := projectAccountIngressPrivateRecordV1(
				rawText,
				accountSpans,
				referencesByTypedCanonical,
			)
			if privateProjectionErr != nil {
				return privateProjectionErr
			}
			privateReference, persistErr := service.persistIngressExactV1(
				leaseContext,
				newPersistIngressInputV1(
					input.SecurityContext,
					input.IngressKind,
					input.IngressOrdinal,
					rawText,
					privateProjectedText,
					privatePersistedSpans,
				),
			)
			if persistErr != nil {
				return persistErr
			}
			compiled = AccountIngressCompilationV1{
				Status:               AccountIngressCompilationStatusPersistedV1,
				PublicText:           publicText,
				SpanCount:            uint32(len(persistedSpans)),
				UniqueReferenceCount: uint32(len(referencesByTypedCanonical)),
				privateReferenceUse:  newPrivateRecordReferenceUseV1(privateReference),
				privateSelectionUse: newPrivateIngressEntitySelectionUseV1(
					privateReference,
					providerIngressDescriptorReferencesV1(providerDescriptors),
					providerIngressDescriptorAliasesV1(providerDescriptors),
				),
				providerProjectionV1: newProviderIngressProjectionV1(
					providerText,
					providerDescriptors...,
				),
			}
			return nil
		},
	)
	if errors.Is(err, ErrAccountIngressResolution) || errors.Is(err, ErrReferenceAmbiguous) ||
		errors.Is(err, ErrReferenceNotFound) {
		return blockedAccountIngressCompilationV1(rawText), nil
	}
	if err != nil {
		return AccountIngressCompilationV1{}, err
	}
	return compiled, nil
}

func compileProviderIngressDescriptorsV1(
	rawSpans []accountIngressRawSpanV1,
	referencesByTypedCanonical map[string]domaincaseentity.ReferenceV1,
	aliasesByTypedCanonical map[string]domaincaseentity.ModelEntityAliasV1,
	resolutionsByTypedCanonical map[string]AccountIngressCandidateResolutionV1,
) ([]providerIngressEntityDescriptorV1, error) {
	if len(rawSpans) == 0 || len(referencesByTypedCanonical) == 0 ||
		len(aliasesByTypedCanonical) != len(referencesByTypedCanonical) ||
		len(resolutionsByTypedCanonical) != len(referencesByTypedCanonical) {
		return nil, ErrAccountIngressProjection
	}
	descriptors := make([]providerIngressEntityDescriptorV1, 0, len(referencesByTypedCanonical))
	byReference := make(map[domaincaseentity.ReferenceV1]providerIngressEntityDescriptorV1, len(referencesByTypedCanonical))
	for _, rawSpan := range rawSpans {
		typedKey := rawSpan.FinancialAccountType + "\x00" + rawSpan.CanonicalValue
		reference, referenceFound := referencesByTypedCanonical[typedKey]
		alias, aliasFound := aliasesByTypedCanonical[typedKey]
		resolution, resolutionFound := resolutionsByTypedCanonical[typedKey]
		if !referenceFound || !aliasFound || !resolutionFound ||
			resolution.Disposition != AccountIngressResolutionResolvedAccountV1 ||
			resolution.FinancialAccountType != rawSpan.FinancialAccountType {
			return nil, ErrAccountIngressProjection
		}
		descriptor := providerIngressEntityDescriptorV1{
			reference:            reference,
			alias:                alias,
			entityType:           resolution.FinancialAccountType,
			financialAccountType: resolution.FinancialAccountType,
			bankInstitution:      resolution.BankInstitution,
			accountType:          resolution.AccountType,
		}
		if validateProviderIngressEntityDescriptorV1(descriptor) != nil {
			return nil, ErrAccountIngressProjection
		}
		if existing, found := byReference[reference]; found {
			if existing != descriptor {
				return nil, ErrAccountIngressProjection
			}
			continue
		}
		byReference[reference] = descriptor
		descriptors = append(descriptors, descriptor)
	}
	if len(descriptors) != len(referencesByTypedCanonical) {
		return nil, ErrAccountIngressProjection
	}
	return descriptors, nil
}

func validateProviderIngressDescriptorsV1(
	providerText string,
	descriptors []providerIngressEntityDescriptorV1,
) error {
	textAliases := providerIngressTextAliasesV1(providerText)
	if len(descriptors) > maxAccountIngressUniqueValuesV1 ||
		len(textAliases) > maxAccountIngressUniqueValuesV1 ||
		domaincaseentity.ContainsReferenceCandidateV1(providerText) {
		return ErrPrivateStateIntegrity
	}
	seen := make(map[domaincaseentity.ModelEntityAliasV1]bool, len(descriptors))
	for _, descriptor := range descriptors {
		if validateProviderIngressEntityDescriptorV1(descriptor) != nil ||
			seen[descriptor.alias] ||
			!textAliases[descriptor.alias] {
			return ErrPrivateStateIntegrity
		}
		seen[descriptor.alias] = true
	}
	return nil
}

func providerIngressTextAliasesV1(providerText string) map[domaincaseentity.ModelEntityAliasV1]bool {
	aliases := map[domaincaseentity.ModelEntityAliasV1]bool{}
	for _, field := range strings.FieldsFunc(providerText, func(value rune) bool {
		return value != ':' && (value < '0' || value > '9') &&
			(value < 'a' || value > 'z')
	}) {
		if domaincaseentity.ValidateModelEntityAliasV1(field) == nil {
			aliases[domaincaseentity.ModelEntityAliasV1(field)] = true
		}
	}
	return aliases
}

func validateProviderIngressEntityDescriptorV1(
	descriptor providerIngressEntityDescriptorV1,
) error {
	if domaincaseentity.ValidateReferenceV1(string(descriptor.reference)) != nil ||
		domaincaseentity.ValidateModelEntityAliasV1(string(descriptor.alias)) != nil ||
		!validFinancialAccountTypeV1(descriptor.entityType) ||
		descriptor.financialAccountType != descriptor.entityType {
		return ErrPrivateStateIntegrity
	}
	aliasType, _, err := domaincaseentity.ParseModelEntityAliasV1(string(descriptor.alias))
	if err != nil || aliasType != descriptor.entityType {
		return ErrPrivateStateIntegrity
	}
	if _, err := domaincaseentity.NewDisplayLabelV1(domaincaseentity.DisplayLabelInputV1{
		EntityType:    descriptor.entityType,
		StableOrdinal: 1,
		Institution:   descriptor.bankInstitution,
		AccountType:   descriptor.accountType,
	}); err != nil {
		return ErrPrivateStateIntegrity
	}
	return nil
}

func (service *Service) accountIngressDescriptorMatchesCurrentSelectionV1(
	resolved domainfundsquerysource.DescriptorV1,
	securityContext domainsecurity.TurnSecurityContext,
	selection datasetsnapshotport.CurrentSelectionV2,
) bool {
	if domainfundsquerysource.ValidateDescriptorV1(resolved) != nil ||
		!domainfundsquerysource.QueryProfileSupportsAccountIngressResolutionV1(
			resolved.QueryProfileDigest,
		) ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil {
		return false
	}
	observation, err := service.bindingObserver.Observe(securityContext.WorkspaceRealPath)
	if err != nil || !privateStateObservationMatchesContextV1(observation, securityContext) {
		return false
	}
	expected, err := fundsquerysourceapp.DescriptorForCurrentSelectionV1(
		selection,
		securityContext,
		observation,
	)
	return err == nil && expected == resolved
}

func newPrivateRecordReferenceUseV1(reference PrivateRecordReferenceV1) privateRecordReferenceUseV1 {
	var used atomic.Bool
	return func(use func(PrivateRecordReferenceV1) error) error {
		if use == nil || !used.CompareAndSwap(false, true) {
			return ErrPrivateStateUnavailable
		}
		return use(reference)
	}
}

func newPrivateIngressEntitySelectionUseV1(
	record PrivateRecordReferenceV1,
	references []domaincaseentity.ReferenceV1,
	aliases []domaincaseentity.ModelEntityAliasV1,
) privateIngressEntitySelectionUseV1 {
	privateReferences := append([]domaincaseentity.ReferenceV1(nil), references...)
	privateAliases := append([]domaincaseentity.ModelEntityAliasV1(nil), aliases...)
	var used atomic.Bool
	return func(use func(PrivateRecordReferenceV1, []domaincaseentity.ReferenceV1, []domaincaseentity.ModelEntityAliasV1) error) error {
		if use == nil || !used.CompareAndSwap(false, true) ||
			!domainsecurity.IsSHA256Hex(record.RecordID) ||
			!domainsecurity.IsSHA256Hex(record.RecordDigest) ||
			len(privateReferences) == 0 || len(privateReferences) != len(privateAliases) {
			return ErrPrivateStateUnavailable
		}
		return use(
			record,
			append([]domaincaseentity.ReferenceV1(nil), privateReferences...),
			append([]domaincaseentity.ModelEntityAliasV1(nil), privateAliases...),
		)
	}
}

func providerIngressDescriptorReferencesV1(
	descriptors []providerIngressEntityDescriptorV1,
) []domaincaseentity.ReferenceV1 {
	references := make([]domaincaseentity.ReferenceV1, 0, len(descriptors))
	for _, descriptor := range descriptors {
		references = append(references, descriptor.reference)
	}
	sort.Slice(references, func(left, right int) bool { return references[left] < references[right] })
	return references
}

func providerIngressDescriptorAliasesV1(
	descriptors []providerIngressEntityDescriptorV1,
) []domaincaseentity.ModelEntityAliasV1 {
	aliases := make([]domaincaseentity.ModelEntityAliasV1, 0, len(descriptors))
	for _, descriptor := range descriptors {
		aliases = append(aliases, descriptor.alias)
	}
	sort.Slice(aliases, func(left, right int) bool { return aliases[left] < aliases[right] })
	return aliases
}

func blockedAccountIngressCompilationV1(rawText string) AccountIngressCompilationV1 {
	projected := maskAccountIngressInternalReferencesV1(rawText)
	projected = domainprivacyprojection.ProjectText(maskAccountIngressNumericCandidatesV1(projected)).Text
	return AccountIngressCompilationV1{
		Status:     AccountIngressCompilationStatusBlockedV1,
		PublicText: projected,
	}
}

func accountIngressRawSpansV1(rawText string) ([]accountIngressRawSpanV1, error) {
	findings, err := discoverAccountIngressFindingsV1(rawText)
	if err != nil {
		return nil, err
	}
	rawSpans := make([]accountIngressRawSpanV1, 0, len(findings)+4)
	for _, finding := range findings {
		if finding.Kind != domainprivacyprojection.KindAccount {
			return nil, ErrUnsupportedIngressPII
		}
		canonicalValue, _ := domaincontrolledaccount.CanonicalFinancialAccountTextV2(
			rawText[finding.StartByte:finding.EndByte],
		)
		if len(canonicalValue) < minCompleteAccountDigitsV1 ||
			len(canonicalValue) > maxCompleteAccountDigitsV1 {
			canonicalValue = ""
		}
		rawSpans = append(rawSpans, accountIngressRawSpanV1{
			StartByte:      finding.StartByte,
			EndByte:        finding.EndByte,
			CanonicalValue: canonicalValue,
			Placeholder:    domainprivacyprojection.Placeholder(finding.Kind),
		})
	}
	for _, candidate := range genericAccountIngressNumericSpansV1(rawText) {
		overlapped := false
		for _, existing := range rawSpans {
			if candidate.StartByte < existing.EndByte && existing.StartByte < candidate.EndByte {
				if candidate.StartByte < existing.StartByte || candidate.EndByte > existing.EndByte {
					return nil, ErrAccountIngressProjection
				}
				overlapped = true
				break
			}
		}
		if !overlapped {
			rawSpans = append(rawSpans, candidate)
		}
	}
	sort.Slice(rawSpans, func(left, right int) bool {
		return rawSpans[left].StartByte < rawSpans[right].StartByte
	})
	if len(rawSpans) > maxAccountIngressSpansV1 {
		return nil, ErrAccountIngressProjection
	}
	for index := 1; index < len(rawSpans); index++ {
		if rawSpans[index].StartByte < rawSpans[index-1].EndByte {
			return nil, ErrAccountIngressProjection
		}
	}
	return rawSpans, nil
}

func genericAccountIngressNumericSpansV1(rawText string) []accountIngressRawSpanV1 {
	type numericRunV1 struct {
		start, lastDigitEnd, digits int
	}
	runs := make([]accountIngressRawSpanV1, 0, 4)
	current := numericRunV1{start: -1}
	flush := func() {
		if current.start >= 0 && current.digits >= minCompleteAccountDigitsV1 && current.lastDigitEnd > current.start {
			rawValue := rawText[current.start:current.lastDigitEnd]
			canonical := accountIngressASCIIDigitsV1(rawValue)
			if len(canonical) < minCompleteAccountDigitsV1 || len(canonical) > maxCompleteAccountDigitsV1 ||
				len(rawValue) > maxAccountIngressCandidateBytesV1 {
				canonical = ""
			}
			runs = append(runs, accountIngressRawSpanV1{
				StartByte: current.start, EndByte: current.lastDigitEnd,
				CanonicalValue: canonical,
				Placeholder:    domainprivacyprojection.Placeholder(domainprivacyprojection.KindNumber),
			})
		}
		current = numericRunV1{start: -1}
	}
	for _, sourceRune := range accountIngressNFKCSourceRunesV1(rawText) {
		offset := sourceRune.startByte
		character := sourceRune.value
		switch {
		case character >= '0' && character <= '9':
			if current.start < 0 {
				current.start = offset
			}
			current.digits++
			current.lastDigitEnd = sourceRune.endByte
		case unicode.IsNumber(character):
			if current.start < 0 {
				current.start = offset
			}
			current.digits++
			current.lastDigitEnd = sourceRune.endByte
		case current.start >= 0 && accountIngressVisualSeparatorV1(character):
		default:
			flush()
		}
	}
	flush()
	return runs
}

type accountIngressNormalizedSourceRuneV1 struct {
	value     rune
	startByte int
	endByte   int
}

func accountIngressNFKCSourceRunesV1(value string) []accountIngressNormalizedSourceRuneV1 {
	var iterator norm.Iter
	iterator.InitString(norm.NFKC, value)
	result := make([]accountIngressNormalizedSourceRuneV1, 0, utf8.RuneCountInString(value))
	segmentStart := 0
	for !iterator.Done() {
		segment := iterator.Next()
		segmentEnd := iterator.Pos()
		for len(segment) > 0 {
			current, width := utf8.DecodeRune(segment)
			result = append(result, accountIngressNormalizedSourceRuneV1{
				value:     current,
				startByte: segmentStart,
				endByte:   segmentEnd,
			})
			segment = segment[width:]
		}
		segmentStart = segmentEnd
	}
	return result
}

func accountIngressVisualSeparatorV1(value rune) bool {
	return unicode.IsSpace(value) || unicode.IsPunct(value) || unicode.IsSymbol(value) ||
		unicode.IsMark(value) || unicode.IsControl(value) || unicode.In(value, unicode.Cf)
}

func accountIngressASCIIDigitsV1(value string) string {
	var digits strings.Builder
	digits.Grow(len(value))
	for _, character := range value {
		switch {
		case character >= '0' && character <= '9':
			digits.WriteRune(character)
		case accountIngressVisualSeparatorV1(character):
		default:
			return ""
		}
	}
	return digits.String()
}

func maskAccountIngressNumericCandidatesV1(rawText string) string {
	spans := genericAccountIngressNumericSpansV1(rawText)
	if len(spans) == 0 {
		return rawText
	}
	var projected strings.Builder
	cursor := 0
	for _, span := range spans {
		if span.StartByte < cursor || span.EndByte <= span.StartByte || span.EndByte > len(rawText) {
			return domainprivacyprojection.Placeholder(domainprivacyprojection.KindNumber)
		}
		projected.WriteString(rawText[cursor:span.StartByte])
		projected.WriteString(span.Placeholder)
		cursor = span.EndByte
	}
	projected.WriteString(rawText[cursor:])
	return projected.String()
}

func maskAccountIngressInternalReferencesV1(rawText string) string {
	projected, _ := domaincaseentity.MaskReferenceCandidatesV1(rawText)
	return projected
}

// discoverAccountIngressFindingsV1 preserves original byte offsets across the
// privacy projector's fixed-point passes. Known spans are replaced with the
// projector's ordinary placeholder only for discovery; those placeholders are
// never returned as model identity.
func discoverAccountIngressFindingsV1(
	rawText string,
) ([]domainprivacyprojection.Finding, error) {
	findings := make([]domainprivacyprojection.Finding, 0, 4)
	for pass := 0; pass < maxAccountIngressProjectionPassesV1; pass++ {
		discoveryText, segments, err := accountIngressDiscoveryTextV1(rawText, findings)
		if err != nil {
			return nil, err
		}
		projection := domainprivacyprojection.ProjectText(discoveryText)
		if len(projection.Findings) == 0 {
			sort.Slice(findings, func(left, right int) bool {
				return findings[left].StartByte < findings[right].StartByte
			})
			return findings, nil
		}
		for _, finding := range projection.Findings {
			if finding.Kind != domainprivacyprojection.KindAccount {
				return nil, ErrUnsupportedIngressPII
			}
			rawFinding, ok := accountIngressRawFindingV1(finding, segments)
			if !ok || !validAccountIngressFindingV1(rawText, rawFinding) ||
				accountIngressFindingOverlapsV1(rawFinding, findings) {
				return nil, ErrAccountIngressProjection
			}
			findings = append(findings, rawFinding)
			if len(findings) > maxAccountIngressSpansV1 {
				return nil, ErrAccountIngressProjection
			}
		}
	}
	return nil, ErrAccountIngressProjection
}

func accountIngressDiscoveryTextV1(
	rawText string,
	findings []domainprivacyprojection.Finding,
) (string, []accountIngressDiscoverySegmentV1, error) {
	ordered := append([]domainprivacyprojection.Finding(nil), findings...)
	sort.Slice(ordered, func(left, right int) bool {
		return ordered[left].StartByte < ordered[right].StartByte
	})
	var projected strings.Builder
	projected.Grow(len(rawText))
	segments := make([]accountIngressDiscoverySegmentV1, 0, len(ordered)+1)
	rawCursor := 0
	for _, finding := range ordered {
		if finding.Kind != domainprivacyprojection.KindAccount ||
			!validAccountIngressFindingV1(rawText, finding) ||
			finding.StartByte < rawCursor {
			return "", nil, ErrAccountIngressProjection
		}
		if finding.StartByte > rawCursor {
			projectedStart := projected.Len()
			projected.WriteString(rawText[rawCursor:finding.StartByte])
			segments = append(segments, accountIngressDiscoverySegmentV1{
				ProjectedStartByte: projectedStart,
				ProjectedEndByte:   projected.Len(),
				RawStartByte:       rawCursor,
			})
		}
		projected.WriteString(domainprivacyprojection.Placeholder(finding.Kind))
		rawCursor = finding.EndByte
	}
	if rawCursor < len(rawText) {
		projectedStart := projected.Len()
		projected.WriteString(rawText[rawCursor:])
		segments = append(segments, accountIngressDiscoverySegmentV1{
			ProjectedStartByte: projectedStart,
			ProjectedEndByte:   projected.Len(),
			RawStartByte:       rawCursor,
		})
	}
	return projected.String(), segments, nil
}

func accountIngressRawFindingV1(
	finding domainprivacyprojection.Finding,
	segments []accountIngressDiscoverySegmentV1,
) (domainprivacyprojection.Finding, bool) {
	if finding.StartByte < 0 || finding.EndByte <= finding.StartByte {
		return domainprivacyprojection.Finding{}, false
	}
	for _, segment := range segments {
		if finding.StartByte < segment.ProjectedStartByte ||
			finding.EndByte > segment.ProjectedEndByte {
			continue
		}
		return domainprivacyprojection.Finding{
			Kind:      finding.Kind,
			StartByte: segment.RawStartByte + finding.StartByte - segment.ProjectedStartByte,
			EndByte:   segment.RawStartByte + finding.EndByte - segment.ProjectedStartByte,
		}, true
	}
	return domainprivacyprojection.Finding{}, false
}

func validAccountIngressFindingV1(
	rawText string,
	finding domainprivacyprojection.Finding,
) bool {
	return finding.Kind == domainprivacyprojection.KindAccount &&
		finding.StartByte >= 0 && finding.EndByte > finding.StartByte &&
		finding.EndByte <= len(rawText) &&
		(finding.StartByte == 0 || utf8.RuneStart(rawText[finding.StartByte])) &&
		(finding.EndByte == len(rawText) || utf8.RuneStart(rawText[finding.EndByte]))
}

func accountIngressFindingOverlapsV1(
	candidate domainprivacyprojection.Finding,
	findings []domainprivacyprojection.Finding,
) bool {
	for _, existing := range findings {
		if candidate.StartByte < existing.EndByte && existing.StartByte < candidate.EndByte {
			return true
		}
	}
	return false
}

func projectAccountIngressTextV1(
	rawText string,
	rawSpans []accountIngressRawSpanV1,
	referencesByTypedCanonical map[string]domaincaseentity.ReferenceV1,
	aliasesByTypedCanonical map[string]domaincaseentity.ModelEntityAliasV1,
) (string, string, []domaincaseentity.CaseIngressSpanV1, error) {
	var provider strings.Builder
	var public strings.Builder
	provider.Grow(len(rawText))
	public.Grow(len(rawText))
	persistedSpans := make([]domaincaseentity.CaseIngressSpanV1, 0, len(rawSpans))
	rawCursor := 0
	for _, rawSpan := range rawSpans {
		if rawSpan.StartByte < rawCursor || rawSpan.EndByte <= rawSpan.StartByte ||
			rawSpan.EndByte > len(rawText) ||
			!validFinancialAccountTypeV1(rawSpan.FinancialAccountType) {
			return "", "", nil, ErrAccountIngressProjection
		}
		typedKey := rawSpan.FinancialAccountType + "\x00" + rawSpan.CanonicalValue
		reference, found := referencesByTypedCanonical[typedKey]
		alias, aliasFound := aliasesByTypedCanonical[typedKey]
		if !found || !aliasFound || domaincaseentity.ValidateReferenceV1(string(reference)) != nil ||
			domaincaseentity.ValidateModelEntityAliasV1(string(alias)) != nil {
			return "", "", nil, ErrAccountIngressProjection
		}
		provider.WriteString(rawText[rawCursor:rawSpan.StartByte])
		public.WriteString(rawText[rawCursor:rawSpan.StartByte])
		projectedStart := provider.Len()
		provider.WriteString(string(alias))
		public.WriteString(domainprivacyprojection.Placeholder(domainprivacyprojection.KindAccount))
		persistedSpans = append(persistedSpans, domaincaseentity.CaseIngressSpanV1{
			EntityType:         rawSpan.FinancialAccountType,
			Reference:          reference,
			RawStartByte:       rawSpan.StartByte,
			RawEndByte:         rawSpan.EndByte,
			ProjectedStartByte: projectedStart,
			ProjectedEndByte:   provider.Len(),
		})
		rawCursor = rawSpan.EndByte
	}
	provider.WriteString(rawText[rawCursor:])
	public.WriteString(rawText[rawCursor:])
	providerText := provider.String()
	publicText := domainprivacyprojection.ProjectText(public.String()).Text
	if domaincaseentity.ContainsReferenceCandidateV1(publicText) {
		return "", "", nil, ErrAccountIngressProjection
	}
	return providerText, publicText, persistedSpans, nil
}

func projectAccountIngressPrivateRecordV1(
	rawText string,
	rawSpans []accountIngressRawSpanV1,
	referencesByTypedCanonical map[string]domaincaseentity.ReferenceV1,
) (string, []domaincaseentity.CaseIngressSpanV1, error) {
	var projected strings.Builder
	projected.Grow(len(rawText))
	persistedSpans := make([]domaincaseentity.CaseIngressSpanV1, 0, len(rawSpans))
	rawCursor := 0
	for _, rawSpan := range rawSpans {
		if rawSpan.StartByte < rawCursor || rawSpan.EndByte <= rawSpan.StartByte ||
			rawSpan.EndByte > len(rawText) ||
			!validFinancialAccountTypeV1(rawSpan.FinancialAccountType) {
			return "", nil, ErrAccountIngressProjection
		}
		gap, gapErr := domaincaseentity.ProjectCaseIngressPrivateGapV1(
			rawText[rawCursor:rawSpan.StartByte],
		)
		if gapErr != nil {
			return "", nil, ErrAccountIngressProjection
		}
		projected.WriteString(gap)
		typedKey := rawSpan.FinancialAccountType + "\x00" + rawSpan.CanonicalValue
		reference, found := referencesByTypedCanonical[typedKey]
		if !found || domaincaseentity.ValidateReferenceV1(string(reference)) != nil {
			return "", nil, ErrAccountIngressProjection
		}
		projectedStart := projected.Len()
		projected.WriteString(string(reference))
		persistedSpans = append(persistedSpans, domaincaseentity.CaseIngressSpanV1{
			EntityType:         rawSpan.FinancialAccountType,
			Reference:          reference,
			RawStartByte:       rawSpan.StartByte,
			RawEndByte:         rawSpan.EndByte,
			ProjectedStartByte: projectedStart,
			ProjectedEndByte:   projected.Len(),
		})
		rawCursor = rawSpan.EndByte
	}
	gap, gapErr := domaincaseentity.ProjectCaseIngressPrivateGapV1(rawText[rawCursor:])
	if gapErr != nil {
		return "", nil, ErrAccountIngressProjection
	}
	projected.WriteString(gap)
	return projected.String(), persistedSpans, nil
}

type PersistIngressInputV1 struct {
	SecurityContext domainsecurity.TurnSecurityContext
	IngressKind     string
	IngressOrdinal  uint32
	rawTextUseV1    privateTextUseV1
	ProjectedText   string
	Spans           []domaincaseentity.CaseIngressSpanV1
}

func newPersistIngressInputV1(
	securityContext domainsecurity.TurnSecurityContext,
	ingressKind string,
	ingressOrdinal uint32,
	rawText string,
	projectedText string,
	spans []domaincaseentity.CaseIngressSpanV1,
) PersistIngressInputV1 {
	return PersistIngressInputV1{
		SecurityContext: securityContext, IngressKind: ingressKind,
		IngressOrdinal: ingressOrdinal, rawTextUseV1: newPrivateTextUseV1(rawText),
		ProjectedText: projectedText, Spans: append([]domaincaseentity.CaseIngressSpanV1(nil), spans...),
	}
}

type PrivateRecordReferenceV1 struct {
	RecordID     string
	RecordDigest string
}

// persistIngressV1 is intentionally package-private: only the source-resolving
// compiler may create a durable ingress record. Tests use it to exercise the
// exact-store seam without creating a second production entry point.
func (service *Service) persistIngressV1(
	ctx context.Context,
	input PersistIngressInputV1,
) (PrivateRecordReferenceV1, error) {
	var reference PrivateRecordReferenceV1
	err := service.withCurrentPrivateStateV1(
		ctx,
		input.SecurityContext,
		func(leaseContext context.Context) error {
			var persistErr error
			reference, persistErr = service.persistIngressExactV1(leaseContext, input)
			return persistErr
		},
	)
	if err != nil {
		return PrivateRecordReferenceV1{}, err
	}
	return reference, nil
}

func (service *Service) persistIngressExactV1(
	ctx context.Context,
	input PersistIngressInputV1,
) (PrivateRecordReferenceV1, error) {
	if service == nil || service.store == nil {
		return PrivateRecordReferenceV1{}, ErrPrivateStateUnavailable
	}
	sourceRawText, rawErr := privateTextValueV1(input.rawTextUseV1)
	if rawErr != nil {
		return PrivateRecordReferenceV1{}, ErrPrivateStateIntegrity
	}
	record, err := domaincaseentity.NewCaseIngressRecordV1(
		domaincaseentity.NewCaseIngressRecordInputV1(
			input.SecurityContext,
			input.IngressKind,
			input.IngressOrdinal,
			sourceRawText,
			input.ProjectedText,
			input.Spans,
		),
	)
	if err != nil {
		return PrivateRecordReferenceV1{}, ErrPrivateStateIntegrity
	}
	var rawText string
	if err := record.UseRawTextV1(func(value string) error {
		rawText = value
		return nil
	}); err != nil {
		return PrivateRecordReferenceV1{}, ErrPrivateStateIntegrity
	}
	for _, span := range record.Spans {
		bound, err := service.resolveBoundReferenceExactV1(ctx, ResolveBoundReferenceInputV1{
			SecurityContext:      input.SecurityContext,
			FinancialAccountType: span.EntityType,
			Reference:            span.Reference,
		})
		if err != nil {
			return PrivateRecordReferenceV1{}, err
		}
		raw := rawText[span.RawStartByte:span.RawEndByte]
		canonical, err := domaincontrolledaccount.CanonicalFinancialAccountTextV2(raw)
		if err != nil || canonical != bound {
			return PrivateRecordReferenceV1{}, ErrPrivateStateIntegrity
		}
	}
	if err := service.store.PutIngressIfAbsent(ctx, record); err != nil {
		return PrivateRecordReferenceV1{}, privateStateErrorV1(err)
	}
	return PrivateRecordReferenceV1{
		RecordID:     record.IngressID,
		RecordDigest: record.RecordDigest,
	}, nil
}

// UseIngressPrivateV1 restores one exact ingress and exposes its raw text only
// inside the live callback for the same exact TSCV2/DSV2 turn that created it.
func (service *Service) UseIngressPrivateV1(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	ingressID string,
	use func(domaincaseentity.CaseIngressRecord, string) error,
) error {
	if use == nil {
		return ErrPrivateStateUnavailable
	}
	return service.withCurrentPrivateStateV1(
		ctx,
		securityContext,
		func(leaseContext context.Context) error {
			record, rawText, err := service.resolveIngressExactV1(
				leaseContext,
				securityContext,
				ingressID,
			)
			if err != nil {
				return err
			}
			return use(record, rawText)
		},
	)
}

// VerifyIngressRawTextV1 proves that a public durable replay names the exact
// host-private ingress already bound to its coordinate. It performs the
// comparison only inside the live TSCV2/DSV2 callback and never returns either
// operand or a raw-derived digest. Callers must run this before invoking a
// resolver for an unchanged public replay so a different source value cannot
// create an unrelated stable binding before the replay conflict is detected.
func (service *Service) VerifyIngressRawTextV1(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	ingressKind string,
	ingressOrdinal uint32,
	candidateRawText string,
) error {
	if candidateRawText == "" {
		return ErrInvalidAccountIngress
	}
	ingressID, err := domaincaseentity.CaseIngressLookupIDV1(
		securityContext,
		ingressKind,
		ingressOrdinal,
	)
	if err != nil {
		return ErrPrivateStateUnavailable
	}
	return service.withCurrentPrivateStateV1(
		ctx,
		securityContext,
		func(leaseContext context.Context) error {
			_, persistedRawText, resolveErr := service.resolveIngressExactV1(
				leaseContext,
				securityContext,
				ingressID,
			)
			if resolveErr != nil {
				return resolveErr
			}
			if len(persistedRawText) != len(candidateRawText) ||
				subtle.ConstantTimeCompare([]byte(persistedRawText), []byte(candidateRawText)) != 1 {
				return ErrPrivateStateConflict
			}
			return nil
		},
	)
}

// UseIngressProviderProjectionV1 restores the provider-only stable-reference
// projection for one exact turn/steer coordinate. Restart and resume callers
// receive only a one-use carrier and never receive the raw text, private record
// identifier, record digest, or a copyable provider string.
func (service *Service) UseIngressProviderProjectionV1(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	ingressKind string,
	ingressOrdinal uint32,
	use func(ProviderIngressProjectionV1) error,
) error {
	if use == nil {
		return ErrPrivateStateUnavailable
	}
	ingressID, err := domaincaseentity.CaseIngressLookupIDV1(
		securityContext,
		ingressKind,
		ingressOrdinal,
	)
	if err != nil {
		return ErrPrivateStateUnavailable
	}
	return service.withCurrentPrivateStateV1(
		ctx,
		securityContext,
		func(leaseContext context.Context) error {
			record, _, resolveErr := service.resolveIngressExactV1(
				leaseContext,
				securityContext,
				ingressID,
			)
			if resolveErr != nil {
				return resolveErr
			}
			projection, projectionErr := service.providerProjectionForIngressRecordExactV1(
				leaseContext,
				securityContext,
				record,
			)
			if projectionErr != nil {
				return projectionErr
			}
			if err := use(projection); err != nil {
				return ErrPrivateStateUnavailable
			}
			return nil
		},
	)
}

func (service *Service) providerProjectionForIngressRecordExactV1(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	record domaincaseentity.CaseIngressRecord,
) (ProviderIngressProjectionV1, error) {
	if service == nil || domaincaseentity.ValidateCaseIngressRecordV1(record) != nil ||
		!domaincaseentity.ContainsReferenceV1(record.ProjectedText) {
		return ProviderIngressProjectionV1{}, ErrPrivateStateIntegrity
	}
	var projected strings.Builder
	projected.Grow(len(record.ProjectedText))
	cursor := 0
	for _, span := range record.Spans {
		if span.ProjectedStartByte < cursor || span.ProjectedEndByte <= span.ProjectedStartByte ||
			span.ProjectedEndByte > len(record.ProjectedText) ||
			record.ProjectedText[span.ProjectedStartByte:span.ProjectedEndByte] != string(span.Reference) {
			return ProviderIngressProjectionV1{}, ErrPrivateStateIntegrity
		}
		binding, err := service.resolveBindingRecordV1(
			ctx,
			securityContext,
			span.EntityType,
			span.Reference,
		)
		if err != nil {
			return ProviderIngressProjectionV1{}, err
		}
		alias, err := domaincaseentity.NewModelEntityAliasV1(binding.EntityType, binding.StableOrdinal)
		if err != nil {
			return ProviderIngressProjectionV1{}, ErrPrivateStateIntegrity
		}
		projected.WriteString(record.ProjectedText[cursor:span.ProjectedStartByte])
		projected.WriteString(string(alias))
		cursor = span.ProjectedEndByte
	}
	projected.WriteString(record.ProjectedText[cursor:])
	providerText := projected.String()
	if domaincaseentity.ContainsReferenceCandidateV1(providerText) ||
		len(providerIngressTextAliasesV1(providerText)) == 0 {
		return ProviderIngressProjectionV1{}, ErrPrivateStateIntegrity
	}
	return newProviderIngressProjectionV1(providerText), nil
}

type accountIngressRecompileEntityV1 struct {
	canonicalValue string
	entityType     string
	reference      domaincaseentity.ReferenceV1
}

type accountIngressRecompileSeedV1 struct {
	rawText        string
	ingressID      string
	recordDigest   string
	spanCount      uint32
	uniqueEntities []accountIngressRecompileEntityV1
}

// UseRecompiledIngressProviderProjectionV1 restores one exact private ingress,
// re-resolves its identities against the current immutable source, and proves
// that recompilation is the same no-replace ingress record before exposing a
// descriptor-bearing provider carrier. Raw text, private record identity, and
// record digest remain internal. The carrier must be consumed synchronously
// through UseExactWithDescriptorsV1 and expires when use returns.
func (service *Service) UseRecompiledIngressProviderProjectionV1(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	ingressKind string,
	ingressOrdinal uint32,
	resolve ResolveAccountIngressCandidatesV1,
	use func(ProviderIngressProjectionV1) error,
) error {
	if service == nil || ctx == nil || resolve == nil || use == nil {
		return ErrPrivateStateUnavailable
	}
	ingressID, err := domaincaseentity.CaseIngressLookupIDV1(
		securityContext,
		ingressKind,
		ingressOrdinal,
	)
	if err != nil {
		return ErrPrivateStateUnavailable
	}

	var seed accountIngressRecompileSeedV1
	err = service.withCurrentPrivateStateV1(
		ctx,
		securityContext,
		func(leaseContext context.Context) error {
			record, rawText, resolveErr := service.resolveIngressExactV1(
				leaseContext,
				securityContext,
				ingressID,
			)
			if resolveErr != nil {
				return resolveErr
			}
			var seedErr error
			seed, seedErr = accountIngressRecompileSeedForRecordV1(
				record,
				rawText,
				ingressID,
				ingressKind,
				ingressOrdinal,
			)
			return seedErr
		},
	)
	if err != nil {
		return err
	}
	defer func() {
		seed.rawText = ""
		seed.uniqueEntities = nil
	}()

	replayResolver := accountIngressRecompileResolverV1(seed.uniqueEntities, resolve)
	compiled, compileErr := service.CompileAccountIngressV1(
		ctx,
		NewCompileAccountIngressInputV1(
			securityContext,
			ingressKind,
			ingressOrdinal,
			seed.rawText,
			replayResolver,
		),
	)
	if compileErr != nil {
		return compileErr
	}
	if !compiled.IsPersisted() || compiled.SpanCount != seed.spanCount ||
		compiled.UniqueReferenceCount != uint32(len(seed.uniqueEntities)) {
		return ErrAccountIngressResolution
	}
	var recompiledReference PrivateRecordReferenceV1
	var privateReferenceCalls atomic.Uint32
	if err := compiled.UsePrivateRecordReferenceV1(func(reference PrivateRecordReferenceV1) error {
		if !privateReferenceCalls.CompareAndSwap(0, 1) {
			return ErrPrivateStateIntegrity
		}
		recompiledReference = reference
		return nil
	}); err != nil || privateReferenceCalls.Load() != 1 ||
		recompiledReference.RecordID != seed.ingressID ||
		recompiledReference.RecordDigest != seed.recordDigest {
		return ErrPrivateStateIntegrity
	}

	return service.withCurrentPrivateStateV1(
		ctx,
		securityContext,
		func(leaseContext context.Context) error {
			record, rawText, resolveErr := service.resolveIngressExactV1(
				leaseContext,
				securityContext,
				seed.ingressID,
			)
			if resolveErr != nil {
				return resolveErr
			}
			if record.IngressID != seed.ingressID ||
				record.RecordDigest != seed.recordDigest || rawText != seed.rawText {
				return ErrPrivateStateIntegrity
			}
			return compiled.UseProviderProjectionV1(func(projection ProviderIngressProjectionV1) error {
				return useDescriptorScopedProviderIngressProjectionV1(projection, use)
			})
		},
	)
}

func accountIngressRecompileSeedForRecordV1(
	record domaincaseentity.CaseIngressRecord,
	rawText string,
	expectedIngressID string,
	expectedIngressKind string,
	expectedIngressOrdinal uint32,
) (accountIngressRecompileSeedV1, error) {
	if domaincaseentity.ValidateCaseIngressRecordV1(record) != nil ||
		record.IngressID != expectedIngressID ||
		record.IngressKind != expectedIngressKind ||
		record.IngressOrdinal != expectedIngressOrdinal ||
		domaincaseentity.ContainsReferenceCandidateV1(rawText) {
		return accountIngressRecompileSeedV1{}, ErrPrivateStateIntegrity
	}
	rawSpans, err := accountIngressRawSpansV1(rawText)
	if err != nil || len(rawSpans) == 0 || len(rawSpans) != len(record.Spans) {
		return accountIngressRecompileSeedV1{}, ErrPrivateStateIntegrity
	}
	uniqueEntities := make([]accountIngressRecompileEntityV1, 0, len(record.Spans))
	byCanonical := make(map[string]accountIngressRecompileEntityV1, len(record.Spans))
	for index, rawSpan := range rawSpans {
		persistedSpan := record.Spans[index]
		if rawSpan.CanonicalValue == "" ||
			rawSpan.StartByte != persistedSpan.RawStartByte ||
			rawSpan.EndByte != persistedSpan.RawEndByte ||
			!validFinancialAccountTypeV1(persistedSpan.EntityType) ||
			domaincaseentity.ValidateReferenceV1(string(persistedSpan.Reference)) != nil {
			return accountIngressRecompileSeedV1{}, ErrPrivateStateIntegrity
		}
		entity := accountIngressRecompileEntityV1{
			canonicalValue: rawSpan.CanonicalValue,
			entityType:     persistedSpan.EntityType,
			reference:      persistedSpan.Reference,
		}
		if existing, found := byCanonical[rawSpan.CanonicalValue]; found {
			if existing != entity {
				return accountIngressRecompileSeedV1{}, ErrPrivateStateIntegrity
			}
			continue
		}
		byCanonical[rawSpan.CanonicalValue] = entity
		uniqueEntities = append(uniqueEntities, entity)
	}
	if len(uniqueEntities) == 0 || len(uniqueEntities) > maxAccountIngressUniqueValuesV1 {
		return accountIngressRecompileSeedV1{}, ErrPrivateStateIntegrity
	}
	return accountIngressRecompileSeedV1{
		rawText:        rawText,
		ingressID:      record.IngressID,
		recordDigest:   record.RecordDigest,
		spanCount:      uint32(len(record.Spans)),
		uniqueEntities: uniqueEntities,
	}, nil
}

func accountIngressRecompileResolverV1(
	expected []accountIngressRecompileEntityV1,
	resolve ResolveAccountIngressCandidatesV1,
) ResolveAccountIngressCandidatesV1 {
	expectedEntities := append([]accountIngressRecompileEntityV1(nil), expected...)
	return func(
		ctx context.Context,
		securityContext domainsecurity.TurnSecurityContext,
		candidates AccountIngressCandidateBatchV1,
	) (AccountIngressResolutionBatchV1, error) {
		if resolve == nil || len(expectedEntities) == 0 ||
			candidates.CandidateCountV1() != uint32(len(expectedEntities)) {
			return AccountIngressResolutionBatchV1{}, ErrAccountIngressResolution
		}
		resolved, err := resolve(ctx, securityContext, candidates)
		if err != nil || !candidates.exactUseCompletedV1() ||
			validateAccountIngressResolutionBatchV1(
				resolved,
				securityContext,
				len(expectedEntities),
			) != nil {
			return AccountIngressResolutionBatchV1{}, ErrAccountIngressResolution
		}
		for index, expectedEntity := range expectedEntities {
			resolution := resolved.Resolutions[index]
			if resolution.Ordinal != uint32(index) ||
				resolution.Disposition != AccountIngressResolutionResolvedAccountV1 ||
				resolution.FinancialAccountType != expectedEntity.entityType {
				return AccountIngressResolutionBatchV1{}, ErrAccountIngressResolution
			}
		}
		return resolved, nil
	}
}

func (service *Service) resolveIngressExactV1(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	ingressID string,
) (domaincaseentity.CaseIngressRecord, string, error) {
	if service == nil || service.store == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil {
		return domaincaseentity.CaseIngressRecord{}, "", ErrPrivateStateUnavailable
	}
	record, err := service.store.ResolveIngress(ctx, ingressID)
	if err != nil {
		return domaincaseentity.CaseIngressRecord{}, "", privateStateErrorV1(err)
	}
	if record.TenantID != securityContext.TenantID ||
		record.UserID != securityContext.UserID ||
		record.CaseID != securityContext.CaseID ||
		record.CaseBindingHash != securityContext.CaseBindingHash ||
		record.ThreadID != securityContext.ThreadID ||
		record.TurnID != securityContext.TurnID ||
		record.ContextDigest != securityContext.ContextDigest ||
		record.ContextEpoch != securityContext.ContextEpoch ||
		record.DatasetSnapshotID != securityContext.DatasetSnapshotID ||
		record.IngressID != ingressID {
		return domaincaseentity.CaseIngressRecord{}, "", ErrPrivateStateIntegrity
	}
	var rawText string
	if err := record.UseRawTextV1(func(value string) error {
		rawText = value
		return nil
	}); err != nil {
		return domaincaseentity.CaseIngressRecord{}, "", ErrPrivateStateIntegrity
	}
	for _, span := range record.Spans {
		binding, err := service.resolveBindingRecordV1(
			ctx,
			securityContext,
			span.EntityType,
			span.Reference,
		)
		if err != nil {
			return domaincaseentity.CaseIngressRecord{}, "", ErrPrivateStateIntegrity
		}
		raw := rawText[span.RawStartByte:span.RawEndByte]
		canonical, err := domaincontrolledaccount.CanonicalFinancialAccountTextV2(raw)
		bindingMatches := false
		bindingErr := binding.UseCanonicalValueV1(func(value string) error {
			bindingMatches = canonical == value
			return nil
		})
		if err != nil || bindingErr != nil || !bindingMatches {
			return domaincaseentity.CaseIngressRecord{}, "", ErrPrivateStateIntegrity
		}
	}
	return record, rawText, nil
}

// PersistThreadCaseContextV1 acquires the current TSCV2/DSV2 exact-use lease,
// writes one immutable generation, and verifies its exact predecessor when
// Generation > 1. It does not discover or select a "latest" record.
func (service *Service) PersistThreadCaseContextV1(
	ctx context.Context,
	input domaincaseentity.ThreadCaseContextRecordInputV1,
) (PrivateRecordReferenceV1, error) {
	var reference PrivateRecordReferenceV1
	err := service.withCurrentPrivateStateV1(
		ctx,
		input.SecurityContext,
		func(leaseContext context.Context) error {
			var persistErr error
			reference, persistErr = service.persistThreadCaseContextExactV1(leaseContext, input)
			return persistErr
		},
	)
	if err != nil {
		return PrivateRecordReferenceV1{}, err
	}
	return reference, nil
}

func (service *Service) persistThreadCaseContextExactV1(
	ctx context.Context,
	input domaincaseentity.ThreadCaseContextRecordInputV1,
) (PrivateRecordReferenceV1, error) {
	if service == nil || service.store == nil {
		return PrivateRecordReferenceV1{}, ErrPrivateStateUnavailable
	}
	record, err := domaincaseentity.NewThreadCaseContextRecordV1(input)
	if err != nil {
		return PrivateRecordReferenceV1{}, ErrPrivateStateIntegrity
	}
	for _, reference := range record.EntityReferences {
		if _, err := service.resolveBindingRecordByReferenceV1(
			ctx,
			input.SecurityContext,
			reference,
		); err != nil {
			return PrivateRecordReferenceV1{}, err
		}
	}
	if record.Generation > 1 {
		previousKey, err := domaincaseentity.ThreadCaseContextStorageKeyV1(
			input.SecurityContext,
			record.Generation-1,
		)
		if err != nil {
			return PrivateRecordReferenceV1{}, ErrPrivateStateIntegrity
		}
		previous, err := service.store.ResolveThreadContext(ctx, previousKey)
		if err != nil {
			return PrivateRecordReferenceV1{}, privateStateErrorV1(err)
		}
		if domaincaseentity.ValidateThreadCaseContextEvolutionV1(previous, record) != nil {
			return PrivateRecordReferenceV1{}, ErrPrivateStateIntegrity
		}
	}
	if err := service.store.PutThreadContextIfAbsent(ctx, record); err != nil {
		return PrivateRecordReferenceV1{}, privateStateErrorV1(err)
	}
	return PrivateRecordReferenceV1{
		RecordID:     record.StorageKey,
		RecordDigest: record.RecordDigest,
	}, nil
}

// ResolveThreadCaseContextPrivateV1 restores one exact immutable generation
// after live TSCV2/DSV2 pre/post checks; it deliberately has no "latest" or
// listing operation.
func (service *Service) ResolveThreadCaseContextPrivateV1(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	generation uint64,
) (domaincaseentity.ThreadCaseContextRecord, error) {
	var record domaincaseentity.ThreadCaseContextRecord
	err := service.withCurrentPrivateStateV1(
		ctx,
		securityContext,
		func(leaseContext context.Context) error {
			var resolveErr error
			record, resolveErr = service.resolveThreadCaseContextExactV1(
				leaseContext,
				securityContext,
				generation,
			)
			return resolveErr
		},
	)
	if err != nil {
		return domaincaseentity.ThreadCaseContextRecord{}, err
	}
	return record, nil
}

func (service *Service) resolveThreadCaseContextExactV1(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	generation uint64,
) (domaincaseentity.ThreadCaseContextRecord, error) {
	if service == nil || service.store == nil {
		return domaincaseentity.ThreadCaseContextRecord{}, ErrPrivateStateUnavailable
	}
	key, err := domaincaseentity.ThreadCaseContextStorageKeyV1(securityContext, generation)
	if err != nil {
		return domaincaseentity.ThreadCaseContextRecord{}, ErrPrivateStateIntegrity
	}
	record, err := service.store.ResolveThreadContext(ctx, key)
	if err != nil {
		return domaincaseentity.ThreadCaseContextRecord{}, privateStateErrorV1(err)
	}
	if record.TenantID != securityContext.TenantID ||
		record.UserID != securityContext.UserID ||
		record.CaseID != securityContext.CaseID ||
		record.CaseBindingHash != securityContext.CaseBindingHash ||
		record.ThreadID != securityContext.ThreadID {
		return domaincaseentity.ThreadCaseContextRecord{}, ErrPrivateStateIntegrity
	}
	for _, reference := range record.EntityReferences {
		if _, err := service.resolveBindingRecordByReferenceV1(
			ctx,
			securityContext,
			reference,
		); err != nil {
			return domaincaseentity.ThreadCaseContextRecord{}, ErrPrivateStateIntegrity
		}
	}
	return record, nil
}

type CaseContinuityTransitionV1 string

const (
	CaseContinuityIndependentV1 CaseContinuityTransitionV1 = "independent_new_thread"
	CaseContinuityRestartV1     CaseContinuityTransitionV1 = "restart"
	CaseContinuityResumeV1      CaseContinuityTransitionV1 = "resume"
	CaseContinuityForkV1        CaseContinuityTransitionV1 = "fork"
	CaseContinuityCompactionV1  CaseContinuityTransitionV1 = "compaction"
)

type UseCaseLongitudinalAliasesInputV1 struct {
	SecurityContext    domainsecurity.TurnSecurityContext
	Aliases            []domaincaseentity.ModelEntityAliasV1
	ProviderText       string
	Relation           string
	SourceThreadID     string
	ExpectedTransition CaseContinuityTransitionV1
}

type UseCaseLongitudinalEntityTypesInputV1 struct {
	SecurityContext    domainsecurity.TurnSecurityContext
	EntityTypes        []string
	ProviderText       string
	Relation           string
	SourceThreadID     string
	ExpectedTransition CaseContinuityTransitionV1
}

type AppendCaseLongitudinalIngressInputV1 struct {
	SecurityContext    domainsecurity.TurnSecurityContext
	References         []domaincaseentity.ReferenceV1
	Relation           string
	SourceThreadID     string
	ExpectedTransition CaseContinuityTransitionV1
}

type CaseLongitudinalAliasSelectionV1 struct {
	Record             PrivateRecordReferenceV1
	Transition         CaseContinuityTransitionV1
	References         []domaincaseentity.ReferenceV1
	Aliases            []domaincaseentity.ModelEntityAliasV1
	ProviderProjection ProviderIngressProjectionV1
}

// AppendCaseLongitudinalIngressV1 evolves the existing case-level index and
// the current thread's typed context from one trusted persisted ingress. Both
// records stay in the existing thread-context CAS partition.
func (service *Service) AppendCaseLongitudinalIngressV1(
	ctx context.Context,
	input AppendCaseLongitudinalIngressInputV1,
) error {
	if len(input.References) == 0 {
		return ErrPrivateStateIntegrity
	}
	return service.withCurrentPrivateStateV1(ctx, input.SecurityContext, func(leaseContext context.Context) error {
		transition, currentThread, transitionErr := service.classifyCaseContinuityTransitionExactV1(
			leaseContext,
			input.SecurityContext,
			input.Relation,
			input.SourceThreadID,
			input.References,
		)
		if transitionErr != nil || input.ExpectedTransition != "" && transition != input.ExpectedTransition {
			return ErrPrivateStateIntegrity
		}
		index, err := service.appendCaseLongitudinalIndexExactV1(leaseContext, input.SecurityContext, input.References)
		if err != nil {
			return err
		}
		threadReferences := input.References
		if transition == CaseContinuityRestartV1 || transition == CaseContinuityCompactionV1 {
			threadReferences = unionCaseEntityReferencesV1(currentThread.EntityReferences, input.References)
		}
		_, err = service.persistCurrentThreadContinuityExactV1(
			leaseContext,
			input.SecurityContext,
			threadReferences,
			index,
			transition,
		)
		return err
	})
}

// UseCaseLongitudinalEntityTypesV1 selects only the account/card identity
// types explicitly requested by the current task. It never returns an index,
// entity count, or maximum ordinal; the selected aliases are revalidated by
// UseCaseLongitudinalAliasesV1 before any thread state is written.
func (service *Service) UseCaseLongitudinalEntityTypesV1(
	ctx context.Context,
	input UseCaseLongitudinalEntityTypesInputV1,
	use func(CaseLongitudinalAliasSelectionV1) error,
) error {
	if use == nil || len(input.EntityTypes) == 0 || len(input.EntityTypes) > 2 {
		return ErrPrivateStateUnavailable
	}
	wanted := make(map[string]struct{}, len(input.EntityTypes))
	for _, entityType := range input.EntityTypes {
		if !validFinancialAccountTypeV1(entityType) {
			return ErrPrivateStateIntegrity
		}
		if _, duplicate := wanted[entityType]; duplicate {
			return ErrPrivateStateIntegrity
		}
		wanted[entityType] = struct{}{}
	}
	aliases := make([]domaincaseentity.ModelEntityAliasV1, 0, len(wanted))
	err := service.withCurrentPrivateStateV1(ctx, input.SecurityContext, func(leaseContext context.Context) error {
		index, err := service.ensureCaseLongitudinalIndexCurrentExactV1(leaseContext, input.SecurityContext)
		if err != nil {
			return err
		}
		selectedByType := make(map[string]domaincaseentity.ModelEntityAliasV1, len(wanted))
		for _, identity := range index.EntityIdentities {
			if _, selected := wanted[identity.EntityType]; !selected {
				continue
			}
			alias, aliasErr := domaincaseentity.NewModelEntityAliasV1(identity.EntityType, identity.StableOrdinal)
			if aliasErr != nil {
				return ErrPrivateStateIntegrity
			}
			if _, ambiguous := selectedByType[identity.EntityType]; ambiguous {
				// A type-only task cannot silently enumerate the case inventory.
				// Require an explicit alias or source-exact value when more than one
				// entity of that type is available.
				return ErrPrivateStateIntegrity
			}
			selectedByType[identity.EntityType] = alias
		}
		for _, entityType := range input.EntityTypes {
			alias, found := selectedByType[entityType]
			if !found {
				return ErrPrivateStateNotFound
			}
			aliases = append(aliases, alias)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return service.UseCaseLongitudinalAliasesV1(ctx, UseCaseLongitudinalAliasesInputV1{
		SecurityContext: input.SecurityContext, Aliases: aliases, ProviderText: input.ProviderText,
		Relation: input.Relation, SourceThreadID: input.SourceThreadID,
		ExpectedTransition: input.ExpectedTransition,
	}, use)
}

// UseCaseLongitudinalAliasesV1 resolves only aliases explicitly selected by
// the current task, revalidates their membership in the exact current case
// index, and persists a transition-specific thread context before use.
func (service *Service) UseCaseLongitudinalAliasesV1(
	ctx context.Context,
	input UseCaseLongitudinalAliasesInputV1,
	use func(CaseLongitudinalAliasSelectionV1) error,
) error {
	if use == nil || len(input.Aliases) == 0 || len(input.Aliases) > maxAccountIngressUniqueValuesV1 {
		return ErrPrivateStateUnavailable
	}
	return service.withCurrentPrivateStateV1(ctx, input.SecurityContext, func(leaseContext context.Context) error {
		index, err := service.ensureCaseLongitudinalIndexCurrentExactV1(leaseContext, input.SecurityContext)
		if err != nil {
			return err
		}
		indexAliases := make(map[domaincaseentity.ModelEntityAliasV1]domaincaseentity.ReferenceV1, len(index.EntityIdentities))
		for _, identity := range index.EntityIdentities {
			alias, aliasErr := domaincaseentity.NewModelEntityAliasV1(identity.EntityType, identity.StableOrdinal)
			if aliasErr != nil {
				return ErrPrivateStateIntegrity
			}
			indexAliases[alias] = identity.Reference
		}
		aliases := append([]domaincaseentity.ModelEntityAliasV1(nil), input.Aliases...)
		sort.Slice(aliases, func(left, right int) bool { return aliases[left] < aliases[right] })
		references := make([]domaincaseentity.ReferenceV1, 0, len(aliases))
		descriptors := make([]providerIngressEntityDescriptorV1, 0, len(aliases))
		for aliasIndex, alias := range aliases {
			if aliasIndex > 0 && alias == aliases[aliasIndex-1] {
				return ErrPrivateStateIntegrity
			}
			entityType, ordinal, parseErr := domaincaseentity.ParseModelEntityAliasV1(string(alias))
			if parseErr != nil {
				return ErrPrivateStateIntegrity
			}
			record, resolveErr := service.store.ResolveBindingByStableOrdinal(
				leaseContext,
				input.SecurityContext,
				entityType,
				ordinal,
			)
			if resolveErr != nil || service.verifyBindingRecordV1(leaseContext, input.SecurityContext, record) != nil {
				return ErrPrivateStateIntegrity
			}
			if indexedReference, admitted := indexAliases[alias]; !admitted || indexedReference != record.Reference ||
				record.EntityType != entityType || record.StableOrdinal != ordinal {
				return ErrPrivateStateIntegrity
			}
			references = append(references, record.Reference)
			descriptors = append(descriptors, providerIngressEntityDescriptorV1{
				reference: record.Reference, alias: alias, entityType: entityType,
				financialAccountType: entityType,
			})
		}

		transition, currentThread, transitionErr := service.classifyCaseContinuityTransitionExactV1(
			leaseContext,
			input.SecurityContext,
			input.Relation,
			input.SourceThreadID,
			references,
		)
		if transitionErr != nil || input.ExpectedTransition != "" && transition != input.ExpectedTransition {
			return ErrPrivateStateIntegrity
		}
		threadReferences := references
		if transition == CaseContinuityRestartV1 || transition == CaseContinuityCompactionV1 {
			threadReferences = unionCaseEntityReferencesV1(currentThread.EntityReferences, references)
		}
		threadRecord, err := service.persistCurrentThreadContinuityExactV1(
			leaseContext,
			input.SecurityContext,
			threadReferences,
			index,
			transition,
		)
		if err != nil {
			return err
		}
		providerText, providerTextErr := caseLongitudinalProviderTextV1(input.ProviderText, aliases)
		if providerTextErr != nil {
			return providerTextErr
		}
		longitudinalState, longitudinalErr := providerIngressLongitudinalStateFromIndexV1(index)
		if longitudinalErr != nil {
			return longitudinalErr
		}
		selection := CaseLongitudinalAliasSelectionV1{
			Record:     PrivateRecordReferenceV1{RecordID: threadRecord.StorageKey, RecordDigest: threadRecord.RecordDigest},
			Transition: transition,
			References: append([]domaincaseentity.ReferenceV1(nil), references...),
			Aliases:    append([]domaincaseentity.ModelEntityAliasV1(nil), aliases...),
			ProviderProjection: newProviderIngressProjectionWithLongitudinalStateV1(
				providerText,
				longitudinalState,
				descriptors...,
			),
		}
		if err := use(selection); err != nil {
			return ErrPrivateStateUnavailable
		}
		return nil
	})
}

func caseLongitudinalProviderTextV1(
	text string,
	aliases []domaincaseentity.ModelEntityAliasV1,
) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" || len(aliases) == 0 || domaincaseentity.ContainsReferenceCandidateV1(text) {
		return "", ErrPrivateStateIntegrity
	}
	present := providerIngressTextAliasesV1(text)
	missing := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		if !present[alias] {
			missing = append(missing, string(alias))
		}
	}
	if len(missing) != 0 {
		text += "\n\nHost-verified task-selected case aliases: " + strings.Join(missing, " ")
	}
	return text, nil
}

func (service *Service) appendCaseLongitudinalIndexExactV1(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	references []domaincaseentity.ReferenceV1,
) (domaincaseentity.ThreadCaseContextRecord, error) {
	identities := make([]domaincaseentity.CaseEntityIdentityStateV1, 0, len(references))
	for _, reference := range references {
		record, err := service.resolveBindingRecordByReferenceV1(ctx, securityContext, reference)
		if err != nil {
			return domaincaseentity.ThreadCaseContextRecord{}, ErrPrivateStateIntegrity
		}
		identities = append(identities, domaincaseentity.CaseEntityIdentityStateV1{
			Reference: record.Reference, EntityType: record.EntityType, StableOrdinal: record.StableOrdinal,
		})
	}
	previous, err := service.store.ResolveLatestCaseLongitudinalContext(ctx, securityContext)
	if errors.Is(err, caseentityport.ErrNotFound) {
		return service.persistCaseLongitudinalIndexExactV1(ctx, domaincaseentity.ThreadCaseContextRecordInputV1{
			SecurityContext:  securityContext,
			Generation:       1,
			EntityReferences: unionCaseEntityReferencesV1(nil, references),
			EntityIdentities: identities,
			Snapshots: []domaincaseentity.CaseSnapshotStateV1{{
				DatasetSnapshotID: securityContext.DatasetSnapshotID,
				ContextEpoch:      securityContext.ContextEpoch,
				Currentness:       domaincaseentity.SnapshotCurrentV1,
			}},
			Claims: []domaincaseentity.CaseClaimStateV1{}, Evidence: []domaincaseentity.CaseEvidenceStateV1{},
			OpenQuestionReferences: []string{}, DataGapReferences: []string{}, Continuations: []domaincaseentity.CaseContinuationStateV1{},
		})
	}
	if err != nil {
		return domaincaseentity.ThreadCaseContextRecord{}, privateStateErrorV1(err)
	}
	entityIdentities, identityErr := unionCaseEntityIdentityStatesV1(previous.EntityIdentities, identities)
	if identityErr != nil {
		return domaincaseentity.ThreadCaseContextRecord{}, ErrPrivateStateIntegrity
	}
	snapshots, claims, evidence, continuations, displayBindings, err := evolveCaseLongitudinalCurrentnessV1(previous, securityContext)
	if err != nil {
		return domaincaseentity.ThreadCaseContextRecord{}, ErrPrivateStateIntegrity
	}
	entityReferences := unionCaseEntityReferencesV1(previous.EntityReferences, references)
	if sameCaseEntityReferencesV1(entityReferences, previous.EntityReferences) &&
		securityContext.DatasetSnapshotID == previous.CurrentDatasetSnapshotID &&
		securityContext.ContextEpoch == previous.CurrentContextEpoch {
		return previous, nil
	}
	return service.persistCaseLongitudinalIndexExactV1(ctx, domaincaseentity.ThreadCaseContextRecordInputV1{
		SecurityContext: securityContext, Generation: previous.Generation + 1,
		PreviousRecordDigest: previous.RecordDigest, EntityReferences: entityReferences, EntityIdentities: entityIdentities,
		Snapshots: snapshots, Claims: claims, Evidence: evidence,
		OpenQuestionReferences: previous.OpenQuestionReferences, DataGapReferences: previous.DataGapReferences,
		Continuations: continuations, DisplayBindings: displayBindings,
	})
}

func (service *Service) ensureCaseLongitudinalIndexCurrentExactV1(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) (domaincaseentity.ThreadCaseContextRecord, error) {
	record, write, err := service.prepareCaseLongitudinalIndexCurrentExactV1(ctx, securityContext)
	if err != nil {
		return domaincaseentity.ThreadCaseContextRecord{}, err
	}
	if write {
		if err := service.store.PutThreadContextIfAbsent(ctx, record); err != nil {
			return domaincaseentity.ThreadCaseContextRecord{}, privateStateErrorV1(err)
		}
	}
	return record, nil
}

func (service *Service) prepareCaseLongitudinalIndexCurrentExactV1(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
) (domaincaseentity.ThreadCaseContextRecord, bool, error) {
	previous, err := service.store.ResolveLatestCaseLongitudinalContext(ctx, securityContext)
	if errors.Is(err, caseentityport.ErrNotFound) {
		return domaincaseentity.ThreadCaseContextRecord{}, false, ErrPrivateStateNotFound
	}
	if err != nil {
		return domaincaseentity.ThreadCaseContextRecord{}, false, privateStateErrorV1(err)
	}
	if previous.CurrentDatasetSnapshotID == securityContext.DatasetSnapshotID &&
		previous.CurrentContextEpoch == securityContext.ContextEpoch {
		return previous, false, nil
	}
	snapshots, claims, evidence, continuations, displayBindings, err := evolveCaseLongitudinalCurrentnessV1(previous, securityContext)
	if err != nil {
		return domaincaseentity.ThreadCaseContextRecord{}, false, err
	}
	next, err := domaincaseentity.NewCaseLongitudinalIndexRecordV1(domaincaseentity.ThreadCaseContextRecordInputV1{
		SecurityContext: securityContext, Generation: previous.Generation + 1,
		PreviousRecordDigest: previous.RecordDigest, EntityReferences: previous.EntityReferences,
		EntityIdentities: previous.EntityIdentities,
		Snapshots:        snapshots, Claims: claims, Evidence: evidence,
		OpenQuestionReferences: previous.OpenQuestionReferences, DataGapReferences: previous.DataGapReferences,
		Continuations: continuations, DisplayBindings: displayBindings,
	})
	if err != nil || domaincaseentity.ValidateThreadCaseContextEvolutionV1(previous, next) != nil {
		return domaincaseentity.ThreadCaseContextRecord{}, false, ErrPrivateStateIntegrity
	}
	return next, true, nil
}

type CaseLongitudinalEvidenceDigestV1 struct {
	EvidenceReference string
	EvidenceDigest    string
}

type CaseLongitudinalClaimDigestV1 struct {
	ClaimReference            string
	ClaimDigest               string
	ClaimType                 string
	InvestigationState        string
	EvidenceReferences        []string
	CounterEvidenceReferences []string
}

type AppendCaseLongitudinalOwnerStateInputV1 struct {
	SecurityContext    domainsecurity.TurnSecurityContext
	Evidence           []CaseLongitudinalEvidenceDigestV1
	Claims             []CaseLongitudinalClaimDigestV1
	ContinuationDigest string
	finalizedSlots     []caseLongitudinalFinalizedSlotV1
}

// AppendCaseLongitudinalOwnerStateV1 records only opaque digests and typed
// currentness produced by the existing evidence/claim/continuation owners.
// It never accepts prose, raw tool bodies, or source-exact values.
func (service *Service) AppendCaseLongitudinalOwnerStateV1(
	ctx context.Context,
	input AppendCaseLongitudinalOwnerStateInputV1,
) error {
	if len(input.Evidence) == 0 && len(input.Claims) == 0 && strings.TrimSpace(input.ContinuationDigest) == "" &&
		len(input.finalizedSlots) == 0 {
		return ErrPrivateStateIntegrity
	}
	return service.withCurrentPrivateStateV1(ctx, input.SecurityContext, func(leaseContext context.Context) error {
		index, err := service.ensureCaseLongitudinalIndexCurrentExactV1(leaseContext, input.SecurityContext)
		if err != nil {
			return err
		}
		evidence := append([]domaincaseentity.CaseEvidenceStateV1(nil), index.Evidence...)
		evidenceByReference := make(map[string]domaincaseentity.CaseEvidenceStateV1, len(evidence)+len(input.Evidence))
		for _, state := range evidence {
			evidenceByReference[state.EvidenceReference+"\x00"+state.DatasetSnapshotID] = state
		}
		for _, current := range input.Evidence {
			if strings.TrimSpace(current.EvidenceReference) != current.EvidenceReference || current.EvidenceReference == "" ||
				!domainsecurity.IsSHA256Hex(current.EvidenceDigest) {
				return ErrPrivateStateIntegrity
			}
			state := domaincaseentity.CaseEvidenceStateV1{
				EvidenceReference: current.EvidenceReference, EvidenceDigest: current.EvidenceDigest,
				DatasetSnapshotID: input.SecurityContext.DatasetSnapshotID,
				Currentness:       domaincaseentity.SnapshotCurrentV1,
			}
			key := state.EvidenceReference + "\x00" + state.DatasetSnapshotID
			if existing, found := evidenceByReference[key]; found {
				if existing != state {
					return ErrPrivateStateIntegrity
				}
				continue
			}
			evidenceByReference[key] = state
			evidence = append(evidence, state)
		}

		claims := make([]domaincaseentity.CaseClaimStateV1, len(index.Claims))
		for claimIndex, claim := range index.Claims {
			claims[claimIndex] = claim
			if claim.TypedState != nil {
				typedState := *claim.TypedState
				claims[claimIndex].TypedState = &typedState
			}
			claims[claimIndex].EvidenceReferences = append([]string{}, claim.EvidenceReferences...)
			claims[claimIndex].CounterEvidenceReferences = append([]string{}, claim.CounterEvidenceReferences...)
		}
		claimsByReference := make(map[string]domaincaseentity.CaseClaimStateV1, len(claims)+len(input.Claims))
		for _, state := range claims {
			claimsByReference[state.ClaimReference+"\x00"+state.DatasetSnapshotID] = state
		}
		for _, current := range input.Claims {
			if strings.TrimSpace(current.ClaimReference) != current.ClaimReference || current.ClaimReference == "" ||
				!domainsecurity.IsSHA256Hex(current.ClaimDigest) {
				return ErrPrivateStateIntegrity
			}
			var typedState *domaincaseentity.CaseClaimTypedStateV1
			if current.ClaimType != "" {
				typedState, err = domaincaseentity.NewCaseClaimTypedStateV1(current.ClaimType)
				if err != nil {
					return ErrPrivateStateIntegrity
				}
			}
			state := domaincaseentity.CaseClaimStateV1{
				ClaimReference: current.ClaimReference, ClaimDigest: current.ClaimDigest,
				TypedState:                typedState,
				DatasetSnapshotID:         input.SecurityContext.DatasetSnapshotID,
				Currentness:               domaincaseentity.SnapshotCurrentV1,
				InvestigationState:        current.InvestigationState,
				EvidenceReferences:        append([]string{}, current.EvidenceReferences...),
				CounterEvidenceReferences: append([]string{}, current.CounterEvidenceReferences...),
			}
			for _, reference := range append(append([]string(nil), state.EvidenceReferences...), state.CounterEvidenceReferences...) {
				if _, found := evidenceByReference[reference+"\x00"+state.DatasetSnapshotID]; !found {
					return ErrPrivateStateIntegrity
				}
			}
			key := state.ClaimReference + "\x00" + state.DatasetSnapshotID
			if existing, found := claimsByReference[key]; found {
				if !reflect.DeepEqual(existing, state) {
					return ErrPrivateStateIntegrity
				}
				continue
			}
			claimsByReference[key] = state
			claims = append(claims, state)
		}

		continuations := append([]domaincaseentity.CaseContinuationStateV1(nil), index.Continuations...)
		if digest := strings.TrimSpace(input.ContinuationDigest); digest != "" {
			if digest != input.ContinuationDigest || !domainsecurity.IsSHA256Hex(digest) {
				return ErrPrivateStateIntegrity
			}
			found := false
			for _, existing := range continuations {
				found = found || existing.ContinuationDigest == digest &&
					existing.DatasetSnapshotID == input.SecurityContext.DatasetSnapshotID
			}
			if !found {
				continuations = append(continuations, domaincaseentity.CaseContinuationStateV1{
					ContinuationDigest: digest,
					DatasetSnapshotID:  input.SecurityContext.DatasetSnapshotID,
					Currentness:        domaincaseentity.SnapshotCurrentV1,
				})
			}
		}

		displayBindings := cloneCaseAcceptedDisplayBindingsV1(index.DisplayBindings)
		displayByDigest := make(map[string]domaincaseentity.CaseAcceptedDisplayBindingV1, len(displayBindings))
		displayByFinalSlot := make(map[string]string, len(displayBindings))
		for _, binding := range displayBindings {
			displayByDigest[binding.BindingDigest] = binding
			displayByFinalSlot[binding.AcceptedFinalDigest+"\x00"+binding.SlotID] = binding.BindingDigest
		}
		seenFinalSlots := make(map[string]struct{}, len(input.finalizedSlots))
		for _, finalized := range input.finalizedSlots {
			if !reflect.DeepEqual(finalized.securityContext, input.SecurityContext) ||
				!domainsecurity.IsSHA256Hex(finalized.acceptedFinalDigest) ||
				!domainsecurity.IsSHA256Hex(finalized.dispositionDigest) ||
				finalized.finalGateVersion != domainevidence.FinalEvidenceGateVersion {
				return ErrPrivateStateIntegrity
			}
			identity := finalized.acceptedFinalDigest + "\x00" + finalized.slot.SlotID
			if _, duplicate := seenFinalSlots[identity]; duplicate {
				return ErrPrivateStateIntegrity
			}
			seenFinalSlots[identity] = struct{}{}
			claimBindings := make([]domaincaseentity.CaseAcceptedDisplayClaimBindingV1, len(finalized.slot.ClaimIDs))
			for claimIndex, claimID := range finalized.slot.ClaimIDs {
				state, found := claimsByReference[claimID+"\x00"+input.SecurityContext.DatasetSnapshotID]
				if !found || !domainsecurity.IsSHA256Hex(state.ClaimDigest) {
					return ErrPrivateStateIntegrity
				}
				claimBindings[claimIndex] = domaincaseentity.CaseAcceptedDisplayClaimBindingV1{
					ClaimReference: state.ClaimReference, ClaimDigest: state.ClaimDigest,
				}
			}
			evidenceBindings := make([]domaincaseentity.CaseAcceptedDisplayEvidenceBindingV1, len(finalized.slot.ReceiptIDs))
			for evidenceIndex, receiptID := range finalized.slot.ReceiptIDs {
				state, found := evidenceByReference[receiptID+"\x00"+input.SecurityContext.DatasetSnapshotID]
				if !found || !domainsecurity.IsSHA256Hex(state.EvidenceDigest) {
					return ErrPrivateStateIntegrity
				}
				evidenceBindings[evidenceIndex] = domaincaseentity.CaseAcceptedDisplayEvidenceBindingV1{
					EvidenceReference: state.EvidenceReference, EvidenceDigest: state.EvidenceDigest,
				}
			}
			referenceCalls := 0
			var reference domaincaseentity.ReferenceV1
			if err := finalized.slot.UseReferenceV1(func(candidate domaincaseentity.ReferenceV1) error {
				referenceCalls++
				reference = candidate
				return nil
			}); err != nil || referenceCalls != 1 ||
				!referenceSetContainsV1(index.EntityReferences, []domaincaseentity.ReferenceV1{reference}) {
				return ErrPrivateStateIntegrity
			}
			entityBinding, err := service.resolveBindingRecordByReferenceV1(
				leaseContext, input.SecurityContext, reference,
			)
			if err != nil {
				return ErrPrivateStateIntegrity
			}
			binding, err := domaincaseentity.NewCaseAcceptedDisplayBindingV1(
				domaincaseentity.CaseAcceptedDisplayBindingInputV1{
					CaseBindingHash:     input.SecurityContext.CaseBindingHash,
					OriginalThreadID:    input.SecurityContext.ThreadID,
					OriginalTurnID:      input.SecurityContext.TurnID,
					AcceptedFinalDigest: finalized.acceptedFinalDigest,
					DispositionDigest:   finalized.dispositionDigest,
					FinalGateVersion:    finalized.finalGateVersion,
					ContextDigest:       input.SecurityContext.ContextDigest,
					DatasetSnapshotID:   input.SecurityContext.DatasetSnapshotID,
					ContextEpoch:        input.SecurityContext.ContextEpoch,
					EntityReference:     reference, EntityBindingDigest: entityBinding.RecordDigest,
					SlotID: finalized.slot.SlotID, ClaimBindings: claimBindings,
					EvidenceReceiptBindings: evidenceBindings,
					Currentness:             domaincaseentity.SnapshotCurrentV1,
				},
			)
			if err != nil {
				return ErrPrivateStateIntegrity
			}
			if existingDigest, found := displayByFinalSlot[identity]; found {
				if existingDigest != binding.BindingDigest || !reflect.DeepEqual(displayByDigest[existingDigest], binding) {
					return ErrPrivateStateIntegrity
				}
				continue
			}
			displayByDigest[binding.BindingDigest] = binding
			displayByFinalSlot[identity] = binding.BindingDigest
			displayBindings = append(displayBindings, binding)
		}
		next := index
		if !reflect.DeepEqual(evidence, index.Evidence) || !reflect.DeepEqual(claims, index.Claims) ||
			!reflect.DeepEqual(continuations, index.Continuations) ||
			!reflect.DeepEqual(displayBindings, index.DisplayBindings) {
			next, err = service.persistCaseLongitudinalIndexExactV1(leaseContext, domaincaseentity.ThreadCaseContextRecordInputV1{
				SecurityContext: input.SecurityContext, Generation: index.Generation + 1,
				PreviousRecordDigest: index.RecordDigest, EntityReferences: index.EntityReferences,
				EntityIdentities: index.EntityIdentities, Snapshots: index.Snapshots,
				Claims: claims, Evidence: evidence, Continuations: continuations,
				DisplayBindings:        displayBindings,
				OpenQuestionReferences: index.OpenQuestionReferences, DataGapReferences: index.DataGapReferences,
			})
			if err != nil {
				return err
			}
		}
		currentThread, threadErr := service.store.ResolveLatestThreadContextForScope(
			leaseContext, input.SecurityContext, input.SecurityContext.ThreadID,
		)
		if threadErr != nil {
			return privateStateErrorV1(threadErr)
		}
		_, err = service.persistCurrentThreadContinuityExactV1(
			leaseContext, input.SecurityContext, currentThread.EntityReferences, next, CaseContinuityRestartV1,
		)
		return err
	})
}

func (service *Service) persistCaseLongitudinalIndexExactV1(
	ctx context.Context,
	input domaincaseentity.ThreadCaseContextRecordInputV1,
) (domaincaseentity.ThreadCaseContextRecord, error) {
	record, err := domaincaseentity.NewCaseLongitudinalIndexRecordV1(input)
	if err != nil {
		return domaincaseentity.ThreadCaseContextRecord{}, ErrPrivateStateIntegrity
	}
	if record.Generation > 1 {
		previous, resolveErr := service.store.ResolveLatestCaseLongitudinalContext(ctx, input.SecurityContext)
		if resolveErr != nil || previous.Generation+1 != record.Generation ||
			domaincaseentity.ValidateThreadCaseContextEvolutionV1(previous, record) != nil {
			return domaincaseentity.ThreadCaseContextRecord{}, ErrPrivateStateIntegrity
		}
	}
	if err := service.store.PutThreadContextIfAbsent(ctx, record); err != nil {
		return domaincaseentity.ThreadCaseContextRecord{}, privateStateErrorV1(err)
	}
	return record, nil
}

func (service *Service) classifyCaseContinuityTransitionExactV1(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	relation string,
	sourceThreadID string,
	references []domaincaseentity.ReferenceV1,
) (CaseContinuityTransitionV1, domaincaseentity.ThreadCaseContextRecord, error) {
	relation = strings.TrimSpace(relation)
	if relation != "" && relation != "primary" && relation != "fork" {
		return "", domaincaseentity.ThreadCaseContextRecord{}, ErrPrivateStateIntegrity
	}
	current, currentErr := service.store.ResolveLatestThreadContextForScope(
		ctx,
		securityContext,
		securityContext.ThreadID,
	)
	sourceThreadID = strings.TrimSpace(sourceThreadID)
	if sourceThreadID != "" {
		if sourceThreadID == securityContext.ThreadID {
			return "", domaincaseentity.ThreadCaseContextRecord{}, ErrPrivateStateIntegrity
		}
		source, sourceErr := service.store.ResolveLatestThreadContextForScope(ctx, securityContext, sourceThreadID)
		if sourceErr != nil || !referenceSetContainsV1(source.EntityReferences, references) {
			return "", domaincaseentity.ThreadCaseContextRecord{}, ErrPrivateStateIntegrity
		}
		if relation != "fork" && relation != "primary" {
			return "", domaincaseentity.ThreadCaseContextRecord{}, ErrPrivateStateIntegrity
		}
		if errors.Is(currentErr, caseentityport.ErrNotFound) {
			if relation == "fork" {
				return CaseContinuityForkV1, domaincaseentity.ThreadCaseContextRecord{}, nil
			}
			return CaseContinuityResumeV1, domaincaseentity.ThreadCaseContextRecord{}, nil
		}
		if currentErr != nil {
			return "", domaincaseentity.ThreadCaseContextRecord{}, privateStateErrorV1(currentErr)
		}
		if current.CurrentContextEpoch < securityContext.ContextEpoch {
			return CaseContinuityCompactionV1, current, nil
		}
		if current.CurrentContextEpoch > securityContext.ContextEpoch {
			return "", domaincaseentity.ThreadCaseContextRecord{}, ErrPrivateStateIntegrity
		}
		return CaseContinuityRestartV1, current, nil
	}
	if relation == "fork" {
		return "", domaincaseentity.ThreadCaseContextRecord{}, ErrPrivateStateIntegrity
	}
	if errors.Is(currentErr, caseentityport.ErrNotFound) {
		return CaseContinuityIndependentV1, domaincaseentity.ThreadCaseContextRecord{}, nil
	}
	if currentErr != nil {
		return "", domaincaseentity.ThreadCaseContextRecord{}, privateStateErrorV1(currentErr)
	}
	if current.CurrentContextEpoch < securityContext.ContextEpoch {
		return CaseContinuityCompactionV1, current, nil
	}
	if current.CurrentContextEpoch > securityContext.ContextEpoch {
		return "", domaincaseentity.ThreadCaseContextRecord{}, ErrPrivateStateIntegrity
	}
	return CaseContinuityRestartV1, current, nil
}

func (service *Service) persistCurrentThreadContinuityExactV1(ctx context.Context, securityContext domainsecurity.TurnSecurityContext, references []domaincaseentity.ReferenceV1, index domaincaseentity.ThreadCaseContextRecord, transition CaseContinuityTransitionV1) (domaincaseentity.ThreadCaseContextRecord, error) {
	record, write, err := service.prepareCurrentThreadContinuityExactV1(ctx, securityContext, references, index, transition)
	if err != nil {
		return domaincaseentity.ThreadCaseContextRecord{}, err
	}
	if write {
		if err := service.store.PutThreadContextIfAbsent(ctx, record); err != nil {
			return domaincaseentity.ThreadCaseContextRecord{}, ErrPrivateStateIntegrity
		}
	}
	return record, nil
}

func (service *Service) prepareCurrentThreadContinuityExactV1(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	references []domaincaseentity.ReferenceV1,
	index domaincaseentity.ThreadCaseContextRecord,
	transition CaseContinuityTransitionV1,
) (domaincaseentity.ThreadCaseContextRecord, bool, error) {
	if !domaincaseentity.IsCaseLongitudinalIndexRecordV1(index) ||
		index.TenantID != securityContext.TenantID || index.UserID != securityContext.UserID ||
		index.CaseID != securityContext.CaseID || index.CaseBindingHash != securityContext.CaseBindingHash ||
		index.CurrentDatasetSnapshotID != securityContext.DatasetSnapshotID ||
		index.CurrentContextEpoch != securityContext.ContextEpoch ||
		!referenceSetContainsV1(index.EntityReferences, references) {
		return domaincaseentity.ThreadCaseContextRecord{}, false, ErrPrivateStateIntegrity
	}
	displayBindings := caseAcceptedDisplayBindingsForReferencesV1(index.DisplayBindings, references)
	previous, previousErr := service.store.ResolveLatestThreadContextForScope(ctx, securityContext, securityContext.ThreadID)
	newThread := transition == CaseContinuityIndependentV1 || transition == CaseContinuityResumeV1 || transition == CaseContinuityForkV1
	if newThread {
		if !errors.Is(previousErr, caseentityport.ErrNotFound) {
			return domaincaseentity.ThreadCaseContextRecord{}, false, ErrPrivateStateIntegrity
		}
		record, err := domaincaseentity.NewThreadCaseContextRecordV1(domaincaseentity.ThreadCaseContextRecordInputV1{
			SecurityContext: securityContext, Generation: 1, EntityReferences: references,
			Snapshots: index.Snapshots, Claims: index.Claims, Evidence: index.Evidence,
			OpenQuestionReferences: index.OpenQuestionReferences, DataGapReferences: index.DataGapReferences,
			Continuations: index.Continuations, DisplayBindings: displayBindings,
		})
		if err != nil {
			return domaincaseentity.ThreadCaseContextRecord{}, false, ErrPrivateStateIntegrity
		}
		return record, true, nil
	}
	if previousErr != nil {
		return domaincaseentity.ThreadCaseContextRecord{}, false, privateStateErrorV1(previousErr)
	}
	references = unionCaseEntityReferencesV1(previous.EntityReferences, references)
	if sameCaseEntityReferencesV1(references, previous.EntityReferences) &&
		previous.CurrentDatasetSnapshotID == securityContext.DatasetSnapshotID &&
		previous.CurrentContextEpoch == securityContext.ContextEpoch &&
		reflect.DeepEqual(previous.Snapshots, index.Snapshots) &&
		reflect.DeepEqual(previous.Claims, index.Claims) &&
		reflect.DeepEqual(previous.Evidence, index.Evidence) &&
		reflect.DeepEqual(previous.OpenQuestionReferences, index.OpenQuestionReferences) &&
		reflect.DeepEqual(previous.DataGapReferences, index.DataGapReferences) &&
		reflect.DeepEqual(previous.Continuations, index.Continuations) &&
		reflect.DeepEqual(previous.DisplayBindings, displayBindings) {
		return previous, false, nil
	}
	record, err := domaincaseentity.NewThreadCaseContextRecordV1(domaincaseentity.ThreadCaseContextRecordInputV1{
		SecurityContext: securityContext, Generation: previous.Generation + 1, PreviousRecordDigest: previous.RecordDigest,
		EntityReferences: references, Snapshots: index.Snapshots, Claims: index.Claims, Evidence: index.Evidence,
		OpenQuestionReferences: index.OpenQuestionReferences, DataGapReferences: index.DataGapReferences,
		Continuations: index.Continuations, DisplayBindings: displayBindings,
	})
	if err != nil || domaincaseentity.ValidateThreadCaseContextEvolutionV1(previous, record) != nil {
		return domaincaseentity.ThreadCaseContextRecord{}, false, ErrPrivateStateIntegrity
	}
	return record, true, nil
}

func evolveCaseLongitudinalCurrentnessV1(
	previous domaincaseentity.ThreadCaseContextRecord,
	securityContext domainsecurity.TurnSecurityContext,
) ([]domaincaseentity.CaseSnapshotStateV1, []domaincaseentity.CaseClaimStateV1, []domaincaseentity.CaseEvidenceStateV1, []domaincaseentity.CaseContinuationStateV1, []domaincaseentity.CaseAcceptedDisplayBindingV1, error) {
	snapshots := append([]domaincaseentity.CaseSnapshotStateV1(nil), previous.Snapshots...)
	claims := make([]domaincaseentity.CaseClaimStateV1, len(previous.Claims))
	for index, claim := range previous.Claims {
		claims[index] = claim
		if claim.TypedState != nil {
			typedState := *claim.TypedState
			claims[index].TypedState = &typedState
		}
		claims[index].EvidenceReferences = append([]string(nil), claim.EvidenceReferences...)
		claims[index].CounterEvidenceReferences = append([]string(nil), claim.CounterEvidenceReferences...)
	}
	evidence := append([]domaincaseentity.CaseEvidenceStateV1(nil), previous.Evidence...)
	continuations := append([]domaincaseentity.CaseContinuationStateV1(nil), previous.Continuations...)
	displayBindings := cloneCaseAcceptedDisplayBindingsV1(previous.DisplayBindings)
	if previous.CurrentDatasetSnapshotID == securityContext.DatasetSnapshotID {
		if securityContext.ContextEpoch < previous.CurrentContextEpoch {
			return nil, nil, nil, nil, nil, ErrPrivateStateIntegrity
		}
		for index := range snapshots {
			if snapshots[index].DatasetSnapshotID == securityContext.DatasetSnapshotID {
				snapshots[index].ContextEpoch = securityContext.ContextEpoch
			}
		}
		return snapshots, claims, evidence, continuations, displayBindings, nil
	}
	if securityContext.ContextEpoch <= previous.CurrentContextEpoch {
		return nil, nil, nil, nil, nil, ErrPrivateStateIntegrity
	}
	for index := range snapshots {
		if snapshots[index].Currentness == domaincaseentity.SnapshotCurrentV1 {
			snapshots[index].Currentness = domaincaseentity.SnapshotStaleV1
		}
	}
	for index := range claims {
		if claims[index].Currentness == domaincaseentity.SnapshotCurrentV1 {
			claims[index].Currentness = domaincaseentity.SnapshotStaleV1
		}
	}
	for index := range evidence {
		if evidence[index].Currentness == domaincaseentity.SnapshotCurrentV1 {
			evidence[index].Currentness = domaincaseentity.SnapshotStaleV1
		}
	}
	for index := range continuations {
		if continuations[index].Currentness == domaincaseentity.SnapshotCurrentV1 {
			continuations[index].Currentness = domaincaseentity.SnapshotStaleV1
		}
	}
	for index := range displayBindings {
		if displayBindings[index].Currentness == domaincaseentity.SnapshotCurrentV1 {
			displayBindings[index].Currentness = domaincaseentity.SnapshotStaleV1
		}
	}
	snapshots = append(snapshots, domaincaseentity.CaseSnapshotStateV1{
		DatasetSnapshotID: securityContext.DatasetSnapshotID,
		ContextEpoch:      securityContext.ContextEpoch,
		Currentness:       domaincaseentity.SnapshotCurrentV1,
	})
	return snapshots, claims, evidence, continuations, displayBindings, nil
}

func unionCaseEntityReferencesV1(
	left, right []domaincaseentity.ReferenceV1,
) []domaincaseentity.ReferenceV1 {
	seen := make(map[domaincaseentity.ReferenceV1]struct{}, len(left)+len(right))
	for _, reference := range append(append([]domaincaseentity.ReferenceV1(nil), left...), right...) {
		seen[reference] = struct{}{}
	}
	result := make([]domaincaseentity.ReferenceV1, 0, len(seen))
	for reference := range seen {
		result = append(result, reference)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func cloneCaseAcceptedDisplayBindingsV1(
	values []domaincaseentity.CaseAcceptedDisplayBindingV1,
) []domaincaseentity.CaseAcceptedDisplayBindingV1 {
	if values == nil {
		return nil
	}
	cloned := make([]domaincaseentity.CaseAcceptedDisplayBindingV1, len(values))
	for index, value := range values {
		cloned[index] = value
		cloned[index].ClaimBindings = append(
			[]domaincaseentity.CaseAcceptedDisplayClaimBindingV1(nil), value.ClaimBindings...,
		)
		cloned[index].EvidenceReceiptBindings = append(
			[]domaincaseentity.CaseAcceptedDisplayEvidenceBindingV1(nil), value.EvidenceReceiptBindings...,
		)
	}
	return cloned
}

func caseAcceptedDisplayBindingsForReferencesV1(
	bindings []domaincaseentity.CaseAcceptedDisplayBindingV1,
	references []domaincaseentity.ReferenceV1,
) []domaincaseentity.CaseAcceptedDisplayBindingV1 {
	if bindings == nil {
		return nil
	}
	allowed := make(map[domaincaseentity.ReferenceV1]struct{}, len(references))
	for _, reference := range references {
		allowed[reference] = struct{}{}
	}
	selected := make([]domaincaseentity.CaseAcceptedDisplayBindingV1, 0, len(bindings))
	for _, binding := range bindings {
		if _, found := allowed[binding.EntityReference]; found {
			selected = append(selected, binding)
		}
	}
	return cloneCaseAcceptedDisplayBindingsV1(selected)
}

func unionCaseEntityIdentityStatesV1(
	left, right []domaincaseentity.CaseEntityIdentityStateV1,
) ([]domaincaseentity.CaseEntityIdentityStateV1, error) {
	byReference := make(map[domaincaseentity.ReferenceV1]domaincaseentity.CaseEntityIdentityStateV1, len(left)+len(right))
	byAlias := make(map[domaincaseentity.ModelEntityAliasV1]domaincaseentity.ReferenceV1, len(left)+len(right))
	for _, identity := range append(append([]domaincaseentity.CaseEntityIdentityStateV1(nil), left...), right...) {
		alias, err := domaincaseentity.NewModelEntityAliasV1(identity.EntityType, identity.StableOrdinal)
		if err != nil || domaincaseentity.ValidateReferenceV1(string(identity.Reference)) != nil {
			return nil, ErrPrivateStateIntegrity
		}
		if previous, found := byReference[identity.Reference]; found && previous != identity {
			return nil, ErrPrivateStateIntegrity
		}
		if previous, found := byAlias[alias]; found && previous != identity.Reference {
			return nil, ErrPrivateStateIntegrity
		}
		byReference[identity.Reference] = identity
		byAlias[alias] = identity.Reference
	}
	result := make([]domaincaseentity.CaseEntityIdentityStateV1, 0, len(byReference))
	for _, identity := range byReference {
		result = append(result, identity)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Reference < result[j].Reference })
	return result, nil
}

func sameCaseEntityReferencesV1(left, right []domaincaseentity.ReferenceV1) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func referenceSetContainsV1(all, selected []domaincaseentity.ReferenceV1) bool {
	set := make(map[domaincaseentity.ReferenceV1]struct{}, len(all))
	for _, reference := range all {
		set[reference] = struct{}{}
	}
	for _, reference := range selected {
		if _, found := set[reference]; !found {
			return false
		}
	}
	return true
}

func (service *Service) resolveBindingRecordByReferenceV1(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	reference domaincaseentity.ReferenceV1,
) (domaincaseentity.CaseEntityBindingRecord, error) {
	if service == nil || service.store == nil || service.keyed == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		domaincaseentity.ValidateReferenceV1(string(reference)) != nil {
		return domaincaseentity.CaseEntityBindingRecord{}, ErrPrivateStateUnavailable
	}
	var resolved domaincaseentity.CaseEntityBindingRecord
	for _, entityType := range []string{
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1,
	} {
		record, err := service.resolveBindingRecordV1(
			ctx,
			securityContext,
			entityType,
			reference,
		)
		switch {
		case err == nil:
			if resolved.Reference != "" {
				return domaincaseentity.CaseEntityBindingRecord{}, ErrPrivateStateIntegrity
			}
			resolved = record
		case errors.Is(err, ErrReferenceNotFound):
			continue
		default:
			return domaincaseentity.CaseEntityBindingRecord{}, err
		}
	}
	if resolved.Reference == "" {
		return domaincaseentity.CaseEntityBindingRecord{}, ErrReferenceNotFound
	}
	return resolved, nil
}

func (service *Service) resolveBindingRecordV1(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	entityType string,
	reference domaincaseentity.ReferenceV1,
) (domaincaseentity.CaseEntityBindingRecord, error) {
	if service == nil || service.store == nil || service.keyed == nil {
		return domaincaseentity.CaseEntityBindingRecord{}, ErrPrivateStateUnavailable
	}
	bindingKey, err := domaincaseentity.CaseEntityBindingLookupKeyV1(
		securityContext,
		entityType,
		reference,
	)
	if err != nil {
		return domaincaseentity.CaseEntityBindingRecord{}, ErrInvalidReferenceInput
	}
	record, err := service.store.ResolveBinding(ctx, bindingKey)
	if err != nil {
		if errors.Is(err, caseentityport.ErrNotFound) {
			return domaincaseentity.CaseEntityBindingRecord{}, ErrReferenceNotFound
		}
		return domaincaseentity.CaseEntityBindingRecord{}, privateStateErrorV1(err)
	}
	if record.EntityType != entityType || record.Reference != reference ||
		service.verifyBindingRecordV1(ctx, securityContext, record) != nil {
		return domaincaseentity.CaseEntityBindingRecord{}, ErrPrivateStateIntegrity
	}
	return record, nil
}

func (service *Service) verifyBindingRecordV1(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	record domaincaseentity.CaseEntityBindingRecord,
) error {
	if service == nil || service.keyed == nil ||
		domaincaseentity.ValidateCaseEntityBindingRecordV1(record) != nil ||
		record.TenantID != securityContext.TenantID ||
		record.UserID != securityContext.UserID ||
		record.CaseID != securityContext.CaseID ||
		record.CaseBindingHash != securityContext.CaseBindingHash {
		return ErrPrivateStateIntegrity
	}
	var expected domaincaseentity.ReferenceV1
	err := record.UseCanonicalValueV1(func(canonicalValue string) error {
		var deriveErr error
		expected, deriveErr = service.deriveCanonicalReferenceV1(
			ctx,
			securityContext,
			record.EntityType,
			canonicalValue,
		)
		return deriveErr
	})
	if err != nil || expected != record.Reference {
		return ErrPrivateStateIntegrity
	}
	return nil
}

func validateReferenceInputV1(
	securityContext domainsecurity.TurnSecurityContext,
	financialAccountType string,
	sourceExactValue string,
) (string, error) {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		!validFinancialAccountTypeV1(financialAccountType) {
		return "", ErrInvalidReferenceInput
	}
	canonicalValue, err := domaincontrolledaccount.CanonicalFinancialAccountTextV2(sourceExactValue)
	if err != nil {
		return "", ErrInvalidReferenceInput
	}
	return canonicalValue, nil
}

func (service *Service) deriveCanonicalReferenceV1(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	financialAccountType string,
	canonicalValue string,
) (domaincaseentity.ReferenceV1, error) {
	if service == nil || service.keyed == nil {
		return "", ErrReferenceDerivation
	}
	payload, err := json.Marshal(referencePayloadV1{
		CanonicalValue:  canonicalValue,
		Canonicalizer:   domaincontrolledaccount.ControlledAccountFinancialCanonicalizationV1,
		CaseBindingHash: securityContext.CaseBindingHash,
		CaseID:          securityContext.CaseID,
		EntityType:      financialAccountType,
		TenantID:        securityContext.TenantID,
		UserID:          securityContext.UserID,
	})
	if err != nil {
		return "", ErrReferenceDerivation
	}
	keyedDigest, err := service.keyed.KeyedPayloadHash(ctx, referenceDigestPurposeV1, payload)
	if err != nil {
		return "", ErrReferenceDerivation
	}
	reference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(keyedDigest)
	if err != nil {
		return "", ErrReferenceDerivation
	}
	return reference, nil
}

// Field order is lexicographic by JSON name so json.Marshal produces the exact
// canonical object spelling required by pendingwork.KeyedPayloadHash.
type referencePayloadV1 struct {
	CanonicalValue  string `json:"canonicalValue"`
	Canonicalizer   string `json:"canonicalizer"`
	CaseBindingHash string `json:"caseBindingHash"`
	CaseID          string `json:"caseId"`
	EntityType      string `json:"entityType"`
	TenantID        string `json:"tenantId"`
	UserID          string `json:"userId"`
}

func validFinancialAccountTypeV1(value string) bool {
	return domaincaseentity.IsFinancialEntityTypeV1(value)
}

func privateStateErrorV1(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, caseentityport.ErrConflict):
		return ErrPrivateStateConflict
	case errors.Is(err, caseentityport.ErrNotFound):
		return ErrPrivateStateNotFound
	default:
		return ErrPrivateStateIntegrity
	}
}
