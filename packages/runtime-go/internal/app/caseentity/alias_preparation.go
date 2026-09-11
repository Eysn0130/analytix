package caseentity

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"sync/atomic"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
)

type PreparedCaseLongitudinalAliasesV1 struct {
	state *preparedCaseLongitudinalAliasesStateV1
}
type preparedCaseLongitudinalAliasesStateV1 struct {
	owner    *Service
	input    UseCaseLongitudinalAliasesInputV1
	prepared caseAliasPreparationV1
	consumed atomic.Bool
}

type caseAliasPreparationV1 struct {
	index, thread           domaincaseentity.ThreadCaseContextRecord
	writeIndex, writeThread bool
	transition              CaseContinuityTransitionV1
	payload                 caseAliasProjectionPayloadV1
}

type caseAliasProjectionPayloadV1 struct {
	references   []domaincaseentity.ReferenceV1
	aliases      []domaincaseentity.ModelEntityAliasV1
	text         string
	descriptors  []providerIngressEntityDescriptorV1
	longitudinal ProviderIngressLongitudinalStateV1
}

func (PreparedCaseLongitudinalAliasesV1) String() string {
	return "PreparedCaseLongitudinalAliasesV1{private:[REDACTED]}"
}
func (plan PreparedCaseLongitudinalAliasesV1) GoString() string { return plan.String() }
func (PreparedCaseLongitudinalAliasesV1) MarshalJSON() ([]byte, error) {
	return nil, errors.New("case alias preparation cannot be serialized")
}
func (*PreparedCaseLongitudinalAliasesV1) UnmarshalJSON([]byte) error {
	return errors.New("case alias preparation cannot be deserialized")
}

// PrepareCaseLongitudinalAliasesV1 validates the exact current private index,
// bindings and transition without writing either index or thread continuity.
// A preview deliberately carries no persisted PrivateRecordReferenceV1.
func (service *Service) PrepareCaseLongitudinalAliasesV1(ctx context.Context, input UseCaseLongitudinalAliasesInputV1) (PreparedCaseLongitudinalAliasesV1, error) {
	input.Aliases = append([]domaincaseentity.ModelEntityAliasV1(nil), input.Aliases...)
	var prepared caseAliasPreparationV1
	err := service.withCurrentPrivateStateV1(ctx, input.SecurityContext, func(leaseContext context.Context) error {
		var err error
		prepared, err = service.prepareCaseLongitudinalAliasesExactV1(leaseContext, input)
		return err
	})
	if err != nil {
		return PreparedCaseLongitudinalAliasesV1{}, err
	}
	return PreparedCaseLongitudinalAliasesV1{state: &preparedCaseLongitudinalAliasesStateV1{owner: service, input: input, prepared: prepared}}, nil
}

func (plan PreparedCaseLongitudinalAliasesV1) InspectSelectionV1(use func(CaseLongitudinalAliasSelectionV1) error) error {
	if plan.state == nil || use == nil || plan.state.consumed.Load() {
		return ErrPrivateStateUnavailable
	}
	if err := use(plan.state.prepared.selectionV1(false)); err != nil {
		return ErrPrivateStateUnavailable
	}
	return nil
}

// ApplyPreparedCaseLongitudinalAliasesV1 re-prepares against the current head
// and proves the same selected semantics before any CAS. Sibling preparations
// may have extended this thread: retain the original union/evolution rules.
// The two existing CAS writes are not a new atomic transaction. An ambiguous
// prefix burns this process plan and never gains an automatic retry.
func (service *Service) ApplyPreparedCaseLongitudinalAliasesV1(ctx context.Context, plan PreparedCaseLongitudinalAliasesV1, use func(CaseLongitudinalAliasSelectionV1) error) error {
	state := plan.state
	if state == nil || state.owner != service || state.consumed.Load() || use == nil {
		return ErrPrivateStateUnavailable
	}
	return service.withCurrentPrivateStateV1(ctx, state.input.SecurityContext, func(leaseContext context.Context) error {
		current, err := service.prepareCaseLongitudinalAliasesExactV1(leaseContext, state.input)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(current.payload, state.prepared.payload) {
			return ErrPrivateStateIntegrity
		}
		if err := leaseContext.Err(); err != nil {
			return err
		}
		if !state.consumed.CompareAndSwap(false, true) {
			return ErrPrivateStateUnavailable
		}
		if current.writeIndex {
			if err := service.store.PutThreadContextIfAbsent(leaseContext, current.index); err != nil {
				return privateStateErrorV1(err)
			}
		}
		if current.writeThread {
			if err := service.store.PutThreadContextIfAbsent(leaseContext, current.thread); err != nil {
				return privateStateErrorV1(err)
			}
		}
		if err := use(current.selectionV1(true)); err != nil {
			return ErrPrivateStateUnavailable
		}
		return nil
	})
}

func (prepared caseAliasPreparationV1) selectionV1(persisted bool) CaseLongitudinalAliasSelectionV1 {
	payload := prepared.payload
	selection := CaseLongitudinalAliasSelectionV1{Transition: prepared.transition,
		References: append([]domaincaseentity.ReferenceV1(nil), payload.references...), Aliases: append([]domaincaseentity.ModelEntityAliasV1(nil), payload.aliases...),
		ProviderProjection: newProviderIngressProjectionWithLongitudinalStateV1(payload.text, payload.longitudinal, payload.descriptors...),
	}
	if persisted {
		selection.Record = PrivateRecordReferenceV1{RecordID: prepared.thread.StorageKey, RecordDigest: prepared.thread.RecordDigest}
	}
	return selection
}

func (service *Service) prepareCaseLongitudinalAliasesExactV1(ctx context.Context, input UseCaseLongitudinalAliasesInputV1) (caseAliasPreparationV1, error) {
	if len(input.Aliases) == 0 || len(input.Aliases) > maxAccountIngressUniqueValuesV1 {
		return caseAliasPreparationV1{}, ErrPrivateStateUnavailable
	}
	index, writeIndex, err := service.prepareCaseLongitudinalIndexCurrentExactV1(ctx, input.SecurityContext)
	if err != nil {
		return caseAliasPreparationV1{}, err
	}
	indexAliases := make(map[domaincaseentity.ModelEntityAliasV1]domaincaseentity.ReferenceV1, len(index.EntityIdentities))
	for _, identity := range index.EntityIdentities {
		alias, aliasErr := domaincaseentity.NewModelEntityAliasV1(identity.EntityType, identity.StableOrdinal)
		if aliasErr != nil {
			return caseAliasPreparationV1{}, ErrPrivateStateIntegrity
		}
		indexAliases[alias] = identity.Reference
	}
	aliases := append([]domaincaseentity.ModelEntityAliasV1(nil), input.Aliases...)
	sort.Slice(aliases, func(left, right int) bool { return aliases[left] < aliases[right] })
	references := make([]domaincaseentity.ReferenceV1, 0, len(aliases))
	descriptors := make([]providerIngressEntityDescriptorV1, 0, len(aliases))
	for aliasIndex, alias := range aliases {
		if aliasIndex > 0 && alias == aliases[aliasIndex-1] {
			return caseAliasPreparationV1{}, ErrPrivateStateIntegrity
		}
		entityType, ordinal, parseErr := domaincaseentity.ParseModelEntityAliasV1(string(alias))
		if parseErr != nil {
			return caseAliasPreparationV1{}, ErrPrivateStateIntegrity
		}
		record, resolveErr := service.store.ResolveBindingByStableOrdinal(
			ctx,
			input.SecurityContext,
			entityType,
			ordinal,
		)
		if resolveErr != nil || service.verifyBindingRecordV1(ctx, input.SecurityContext, record) != nil {
			return caseAliasPreparationV1{}, ErrPrivateStateIntegrity
		}
		if indexedReference, admitted := indexAliases[alias]; !admitted || indexedReference != record.Reference ||
			record.EntityType != entityType || record.StableOrdinal != ordinal {
			return caseAliasPreparationV1{}, ErrPrivateStateIntegrity
		}
		references = append(references, record.Reference)
		descriptors = append(descriptors, providerIngressEntityDescriptorV1{
			reference: record.Reference, alias: alias, entityType: entityType,
			financialAccountType: entityType,
		})
	}

	transition, currentThread, transitionErr := service.classifyCaseContinuityTransitionExactV1(
		ctx,
		input.SecurityContext,
		input.Relation,
		input.SourceThreadID,
		references,
	)
	if transitionErr != nil || input.ExpectedTransition != "" && transition != input.ExpectedTransition {
		return caseAliasPreparationV1{}, ErrPrivateStateIntegrity
	}
	threadReferences := references
	if transition == CaseContinuityRestartV1 || transition == CaseContinuityCompactionV1 {
		threadReferences = unionCaseEntityReferencesV1(currentThread.EntityReferences, references)
	}
	threadRecord, writeThread, err := service.prepareCurrentThreadContinuityExactV1(
		ctx,
		input.SecurityContext,
		threadReferences,
		index,
		transition,
	)
	if err != nil {
		return caseAliasPreparationV1{}, err
	}
	providerText, providerTextErr := caseLongitudinalProviderTextV1(input.ProviderText, aliases)
	if providerTextErr != nil {
		return caseAliasPreparationV1{}, providerTextErr
	}
	longitudinalState, longitudinalErr := providerIngressLongitudinalStateFromIndexV1(index)
	if longitudinalErr != nil {
		return caseAliasPreparationV1{}, longitudinalErr
	}
	return caseAliasPreparationV1{index: index, thread: threadRecord, writeIndex: writeIndex, writeThread: writeThread, transition: transition,
		payload: caseAliasProjectionPayloadV1{references: references, aliases: aliases, text: providerText, descriptors: descriptors, longitudinal: longitudinalState}}, nil
}
