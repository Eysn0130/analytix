package runtimeapp

import (
	"context"
	"errors"
	"path"
	"reflect"
	"strings"

	evidencestore "analytix.local/runtime-go/internal/adapters/outbound/evidenceauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

// Frozen before the first recovery writer. A later retired journal or newly
// installed witness exchange cannot grant Original historical qualification.
type runtimeOriginalFinalHistoryV1 struct {
	core                             *runtimeChildIdentityStartupV1
	preserved                        runtimeReportRestartPreservationV1
	registry                         *registryport.OriginalHistoryV2
	evidence                         evidencestore.OriginalGraphV1
	registryRecords, evidenceRecords runtimeOriginalSemanticFilesV1
	finals                           runtimeAcceptedFinalSemanticInventoryV1
	settlementHistory                *runtimeOriginalSettlementHistoryV1
}

type runtimeFinalHistoryQualificationKeyV1 struct{}

// A non-nil token also records an unqualified Original. Internal stage and
// activation recursion cannot turn their newly applied records into Original.
type runtimeFinalHistoryQualificationV1 struct {
	history *runtimeOriginalFinalHistoryV1
}

// The configuration-free maintenance entry has no independently enrolled
// historical reader. It may settle unrelated transactions, but cannot change
// immutable history records without that qualification. Normal configured
// startup owns the qualified recovery path.
func rejectUnqualifiedFinalHistoryRecoveryV1(ctx context.Context, roots persistencefs.RootSet, originalCreates *runtimeOriginalCreateStartupV1) (resultErr error) {
	journal, err := persistencefs.ObserveAuthenticatedSemanticJournalV1(ctx, roots, originalCreates.proofV1())
	if err != nil {
		return err
	}
	if journal == nil {
		return ctx.Err()
	}
	defer func() { resultErr = errors.Join(resultErr, journal.Revalidate(ctx)) }()
	for _, operation := range journal.OperationsV1() {
		if operation.Before == operation.After {
			continue
		}
		name := path.Base(operation.Path)
		if !strings.HasSuffix(name, ".json") || !domainsecurity.IsSHA256Hex(strings.TrimSuffix(name, ".json")) {
			continue
		}
		for _, owner := range []string{
			"data/private/evidence-registry/indexes/", "data/private/evidence-registry/capsules/",
			"data/private/evidence-authority/bundles/", "data/private/evidence-authority/observations/",
			"data/private/accepted-finals/records/", "data/private/accepted-finals/dispositions/",
			"data/private/evidence-settlements/prepared/",
		} {
			if strings.HasPrefix(operation.Path, owner) {
				return errors.New("semantic history recovery requires configured Original final authority")
			}
		}
	}
	return ctx.Err()
}

func bindRuntimeOriginalFinalHistoryV1(ctx context.Context, core *runtimeChildIdentityStartupV1, preserved runtimeReportRestartPreservationV1) (context.Context, *runtimeOriginalFinalHistoryV1, error) {
	if qualification, ok := ctx.Value(runtimeFinalHistoryQualificationKeyV1{}).(*runtimeFinalHistoryQualificationV1); ok {
		if qualification.history == nil {
			return ctx, nil, nil
		}
		rebound := *qualification.history
		rebound.core, rebound.preserved = core, preserved
		if err := rebound.ValidateSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
			return ctx, nil, err
		}
		return ctx, &rebound, nil
	}
	history, err := prepareRuntimeOriginalFinalHistoryV1(ctx, core, preserved)
	if err != nil {
		return ctx, nil, err
	}
	return context.WithValue(ctx, runtimeFinalHistoryQualificationKeyV1{}, &runtimeFinalHistoryQualificationV1{history: history}), history, nil
}

func prepareRuntimeOriginalFinalHistoryV1(ctx context.Context, core *runtimeChildIdentityStartupV1, preserved runtimeReportRestartPreservationV1) (result *runtimeOriginalFinalHistoryV1, resultErr error) {
	if core == nil || core.originalRegistryTrust == nil || preserved.report == nil {
		return nil, nil
	}
	finals, _, observation, err := readRuntimeAcceptedFinalSemanticInventoryV1(ctx, core, preserved.report)
	if err != nil {
		return nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, observation.Revalidate(ctx))
		if resultErr != nil {
			result = nil
		}
	}()
	selected := runtimeAcceptedFinalSemanticInventoryV1{records: map[string]domainevidence.PrivateAcceptedFinalRecord{}, dispositions: map[string]domainevidence.AcceptedFinalDispositionRecord{}}
	for digest, record := range finals.records {
		if record.RegistryHead.Sequence == 0 || domainevidence.FinalAnswerVariantRequiresPublicationSnapshotProof(record.Envelope.Variant) || record.AcceptedFinal.FactFinalWitnessAdmission != nil {
			continue
		}
		selected.records[digest] = record
		if disposition, found := finals.dispositions[digest]; found {
			selected.dispositions[digest] = disposition
		}
	}
	closed := preserved.history != nil && (preserved.history.closedFailed || preserved.history.closedCompletedPrefix || (preserved.history.mixedClosed != nil && len(preserved.history.mixedClosed.Attempts) != 0))
	if len(selected.records) == 0 && !closed {
		return nil, nil
	}
	graphs, err := observeRuntimeOriginalFinalGraphsV1(ctx, core, preserved)
	if err != nil {
		return nil, err
	}
	if graphs.registry == nil {
		// Original domain unavailability/legacy classification retains the
		// existing contract and acquires no offline historical qualification.
		return nil, nil
	}
	defer func() { resultErr = errors.Join(resultErr, graphs.revalidate(ctx)) }()
	for _, record := range selected.records {
		if _, err := replayRuntimeOriginalBoundaryPrefixV1(ctx, record, graphs.registry, graphs.evidence); err != nil {
			return nil, err
		}
	}
	frozen := &runtimeOriginalFinalHistoryV1{
		core: core, preserved: preserved, registry: graphs.registry, evidence: graphs.evidence,
		registryRecords: runtimeImmutableFinalRegistryRecordsV1(graphs.registryOriginal),
		evidenceRecords: runtimeReportCommittedFilesV1(graphs.evidenceOriginal), finals: selected,
	}
	frozen.settlementHistory, err = prepareRuntimeOriginalSettlementHistoryV1(ctx, frozen, graphs)
	if err != nil {
		return nil, err
	}
	if err := frozen.ValidateSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
		return nil, err
	}
	return frozen, nil
}

func runtimeImmutableFinalRegistryRecordsV1(files runtimeOriginalSemanticFilesV1) runtimeOriginalSemanticFilesV1 {
	selected := runtimeOriginalSemanticFilesV1{}
	for name, entry := range runtimeReportCommittedFilesV1(files) {
		if strings.HasPrefix(name, "indexes/") || strings.HasPrefix(name, "capsules/") {
			selected[name] = entry
		}
	}
	return selected
}

func (frozen *runtimeOriginalFinalHistoryV1) ValidateSemanticOperationsV1(ctx context.Context, operations []domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error), noWriteOperationID string) (resultErr error) {
	if frozen == nil {
		return nil
	}
	graphs, err := observeRuntimeOriginalFinalGraphsV1(ctx, frozen.core, frozen.preserved)
	if err != nil {
		return err
	}
	if graphs.registry == nil {
		return errors.New("original nonzero final registry became unavailable")
	}
	defer func() { resultErr = errors.Join(resultErr, graphs.revalidate(ctx)) }()
	finalsOriginal, finalsCandidate, finalsObservation, err := readRuntimeAcceptedFinalSemanticInventoryV1(ctx, frozen.core, frozen.preserved.report)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, finalsObservation.Revalidate(ctx)) }()
	for _, operation := range finalsObservation.journal.OperationsV1() {
		leaf, id, record, _, err := acceptedFinalSemanticAddressV1(operation)
		if err != nil {
			return err
		}
		if record {
			if err := finalsCandidate.applyV1(operation, leaf, id, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
				return finalsObservation.journal.ReadAfterV1(ctx, operation)
			}); err != nil {
				return err
			}
		}
	}
	for _, operation := range operations {
		if operation.OperationID == noWriteOperationID && noWriteOperationID != "" {
			continue
		}
		for _, owner := range []string{runtimeRegistrySemanticRootV1, "data/private/evidence-authority", runtimeAcceptedFinalSemanticRootV1} {
			if err := validateRuntimeOriginalSemanticAncestorV1(owner, operation); err != nil {
				return err
			}
		}
		if name, owned := registrySemanticRelativeV1(operation); owned {
			if err := graphs.registryFinal.applyV1(operation, name, readAfter); err != nil {
				return err
			}
		}
		if name, owned := runtimeAssociatedRelativeV1("evidence-authority", operation); owned {
			if err := graphs.evidenceFinal.applyV1(operation, name, readAfter); err != nil {
				return err
			}
		}
		leaf, id, record, _, err := acceptedFinalSemanticAddressV1(operation)
		if err != nil {
			return err
		}
		if record {
			if err := finalsCandidate.applyV1(operation, leaf, id, readAfter); err != nil {
				return err
			}
		}
	}
	for _, pair := range []struct {
		original, candidate, retained runtimeOriginalSemanticFilesV1
	}{
		{graphs.registryOriginal, graphs.registryFinal, frozen.registryRecords},
		{graphs.evidenceOriginal, graphs.evidenceFinal, frozen.evidenceRecords},
	} {
		for name, expected := range pair.retained {
			if !reflect.DeepEqual(pair.original[name], expected) || !reflect.DeepEqual(pair.candidate[name], expected) {
				return errors.New("semantic candidate changed original nonzero final history material")
			}
		}
	}
	for digest, expected := range frozen.finals.records {
		if !reflect.DeepEqual(finalsOriginal.records[digest], expected) || !reflect.DeepEqual(finalsCandidate.records[digest], expected) {
			return errors.New("semantic candidate changed original nonzero boundary final")
		}
	}
	for digest, expected := range frozen.finals.dispositions {
		if !reflect.DeepEqual(finalsOriginal.dispositions[digest], expected) || !reflect.DeepEqual(finalsCandidate.dispositions[digest], expected) {
			return errors.New("semantic candidate changed original nonzero boundary disposition")
		}
	}
	if err := finalsCandidate.verifyV1(ctx, frozen.core.verification, true); err != nil {
		return err
	}
	candidateRegistry, err := parseRuntimeOriginalRegistryInventoryV1(ctx, frozen.core, graphs.registryFinal, graphs.contexts)
	if err != nil || candidateRegistry.v2 == nil || candidateRegistry.unavailable {
		return errors.Join(errors.New("nonzero final candidate registry is invalid"), err)
	}
	if frozen.settlementHistory == nil {
		return errors.New("original final settlement support is unqualified")
	}
	if _, _, err := frozen.settlementHistory.validateV1(ctx, frozen, graphs, candidateRegistry.v2, operations, readAfter, noWriteOperationID); err != nil {
		return err
	}
	trust := frozen.core.originalRegistryTrust
	if _, err := evidencestore.ParseOriginalGraphV1(ctx, graphs.evidenceFinal, trust.projection.InstallationID, trust.projection.Enrollment.EnrollmentID, trust.projection.Enrollment.WitnessKeyID, trust.witnessKey, frozen.core.verification); err != nil {
		return err
	}
	return ctx.Err()
}
