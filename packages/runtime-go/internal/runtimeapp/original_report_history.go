package runtimeapp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"sort"

	authorityadvancefs "analytix.local/runtime-go/internal/adapters/outbound/authorityadvancefs"
	evidencestore "analytix.local/runtime-go/internal/adapters/outbound/evidenceauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	publicationstore "analytix.local/runtime-go/internal/adapters/outbound/reportpublication"
	pendingapp "analytix.local/runtime-go/internal/app/pendingwork"
	publicationapp "analytix.local/runtime-go/internal/app/reportpublication"
	domainauthority "analytix.local/runtime-go/internal/domain/authorityadvance"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpending "analytix.local/runtime-go/internal/domain/pendingwork"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	authorityport "analytix.local/runtime-go/internal/ports/authorityadvance"
	evidenceport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

// This is historical audit authority over complete Original records. The
// returned authority cannot recover, sign, or release a report.
type runtimeOriginalReportHistoryV1 struct {
	plan                  publicationapp.RestartPlanV1
	authority             *publicationapp.ProjectedDeliveryAuthorityV1
	revalidate            func(context.Context) error
	semantic              *runtimeCompletedReportSemanticPreservationV1
	controlled            *runtimeCompletedControlledAccessHistoryV2
	deferred              *runtimeDeferredReportHistoryV1
	closedCompletedPrefix bool
	mixedClosed           *publicationapp.RestartPlanV1
	closedFailed          bool
	closedResults         map[string]string
}

func (history *runtimeOriginalReportHistoryV1) matchesCompletedPlanV1(plan publicationapp.RestartPlanV1) bool {
	if history == nil || len(history.plan.Attempts) == 0 || !reflect.DeepEqual(history.plan, plan) {
		return false
	}
	for _, entry := range plan.Attempts {
		if entry.State != publicationapp.RestartAttemptDeliveryProjectionV1 && entry.State != publicationapp.RestartAttemptDeliveryRejectionV1 {
			return false
		}
	}
	return true
}

func prepareRuntimeOriginalReportHistoryV1(ctx context.Context, core *runtimeChildIdentityStartupV1, publication *runtimePublicationSemanticPreservationV1, advance *runtimeAuthorityAdvanceStartupV2, scope *pendingapp.ReportRestartScopeV1) (result *runtimeOriginalReportHistoryV1, resultErr error) {
	defer func() {
		if resultErr != nil {
			result = nil
		}
	}()
	if ctx == nil || core == nil || core.verification == nil || core.primaries == nil || publication == nil || publication.unavailable || advance == nil || scope == nil {
		return nil, errors.New("original report history dependencies are unavailable")
	}
	revalidateCore := func(ctx context.Context) error {
		return errors.Join(core.revalidate(ctx), core.originalRegistryTrust.Revalidate(ctx), advance.revalidate(ctx), scope.RevalidatePrimary(ctx), context.Cause(ctx))
	}
	if err := revalidateCore(ctx); err != nil {
		return nil, err
	}
	original, final, publicationObservation, err := publication.readV1(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, publicationObservation.Revalidate(ctx), revalidateCore(ctx))
	}()
	if err := errors.Join(publication.validateEndpointV1(ctx, original), publication.validateEndpointV1(ctx, final)); err != nil {
		return nil, err
	}
	// This Core journal has no semantic restart writer. Prove that its prepared
	// physical inventory is also Original, instead of treating a signed Final
	// addition or replacement as historical input.
	journal, err := persistencefs.ObserveAuthenticatedSemanticJournalV1(ctx, core.roots, core.originalCreateProofV1())
	if err != nil {
		return nil, err
	}
	revalidateJournal := func(ctx context.Context) error {
		if journal != nil {
			return journal.Revalidate(ctx)
		}
		current, err := persistencefs.ObserveAuthenticatedSemanticJournalV1(ctx, core.roots, core.originalCreateProofV1())
		if err != nil {
			return err
		}
		if current != nil {
			return errors.New("semantic journal appeared after Original report observation")
		}
		return context.Cause(ctx)
	}
	defer func() { resultErr = errors.Join(resultErr, revalidateJournal(ctx)) }()
	for _, operation := range journal.OperationsV1() {
		if err := validateRuntimeOriginalSemanticAncestorV1("data/private/authority-advance", operation); err != nil {
			return nil, err
		}
		if _, owned := runtimeAssociatedRelativeV1("authority-advance", operation); owned {
			return nil, errors.New("original authority advance journal was changed by semantic startup")
		}
	}
	associatedOriginal, associatedFinal, _, associatedObservation, err := readRuntimeAssociatedSemanticInventoryV1(ctx, core, scope, nil)
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, associatedObservation.Revalidate(ctx)) }()
	originalHistory, err := prepareRuntimeReportHistoryEndpointV1(ctx, core, publication, advance, core.pendingInventory, original["report-publication"], associatedOriginal["evidence-authority"], core.primaries)
	if err != nil {
		return nil, err
	}
	finalPending := core.pendingInventory
	if core.pendingSemantic != nil {
		finalPending = core.pendingSemantic.final
	}
	finalHistory, err := prepareRuntimeReportHistoryEndpointV1(ctx, core, publication, advance, finalPending, final["report-publication"], associatedFinal["evidence-authority"], core.primaries)
	if err != nil {
		return nil, err
	}
	// Final is independently checked, but cannot supply an Original record or
	// release authority. Both endpoint audits read only Original primaries.
	originalHistory.revalidate = func(ctx context.Context) error {
		return errors.Join(revalidateCore(ctx), publicationObservation.Revalidate(ctx), associatedObservation.Revalidate(ctx), revalidateJournal(ctx))
	}
	if originalHistory.matchesCompletedPlanV1(originalHistory.plan) {
		if !originalHistory.matchesCompletedPlanV1(finalHistory.plan) {
			return nil, errors.New("completed report historical plan changed in signed Final")
		}
		originalHistory.controlled, err = prepareRuntimeCompletedControlledAccessHistoryV2(ctx, core, publication, originalHistory, finalHistory, original, final)
		if err != nil {
			return nil, err
		}
		originalHistory.semantic, err = prepareRuntimeCompletedReportSemanticPreservationV1(ctx, core, publication, advance, originalHistory, original, final, associatedOriginal, associatedFinal)
		if err != nil {
			return nil, err
		}
	}
	if _, _, mixed := runtimeMixedClosedDeferredPlansV1(originalHistory.plan); mixed {
		if err := prepareRuntimeMixedReportHistoryV1(ctx, core, publication, advance, scope, originalHistory, finalHistory, original, final, associatedOriginal, associatedFinal); err != nil {
			return nil, err
		}
	}
	if originalHistory.mixedClosed == nil && runtimeReportPlanIsDeferredV1(originalHistory.plan) {
		originalHistory.deferred, err = prepareRuntimeDeferredReportHistoryV1(ctx, core, publication, advance, scope, originalHistory.plan, finalHistory.plan, original, final)
		if err != nil {
			return nil, err
		}
	}
	if originalHistory.mixedClosed == nil && runtimeReportPlanIsClosedFailedV1(originalHistory.plan) {
		if !originalHistory.matchesClosedFailedPlanV1(finalHistory.plan) || !reflect.DeepEqual(original, final) {
			return nil, errors.New("closed report Original/Final prefix changed")
		}
		for _, owner := range []string{"controlled-artifact-access", "controlled-artifact-access-v2"} {
			if len(runtimeReportCommittedFilesV1(original[owner])) != 0 {
				return nil, errRuntimeReportRestartReconciliationRequired
			}
		}
		if err := validateRuntimeClosedFailedCoreV1(ctx, core, publication, advance, originalHistory.plan, core.pendingInventory, core.primaries); err != nil {
			return nil, err
		}
		originalHistory.closedFailed = true
		if err := originalHistory.captureClosedResultsV1(ctx, core.primaries); err != nil {
			return nil, err
		}
		originalHistory.semantic, err = prepareRuntimeCompletedReportSemanticPreservationV1(ctx, core, publication, advance, originalHistory, original, final, associatedOriginal, associatedFinal)
		if err != nil {
			return nil, err
		}
	}
	if runtimeReportPlanIsClosedCompletedPrefixV1(originalHistory.plan) {
		if !originalHistory.matchesClosedCompletedPrefixPlanV1(finalHistory.plan) || !reflect.DeepEqual(original, final) {
			return nil, errors.New("closed completed report Original/Final prefix changed")
		}
		for _, owner := range []string{"controlled-artifact-access", "controlled-artifact-access-v2"} {
			if len(runtimeReportCommittedFilesV1(original[owner])) != 0 {
				return nil, errRuntimeReportRestartReconciliationRequired
			}
		}
		if err := validateRuntimeClosedCompletedCoreV1(ctx, core, publication, advance, originalHistory.plan, core.pendingInventory, core.primaries); err != nil {
			return nil, err
		}
		originalHistory.closedCompletedPrefix = true
		originalHistory.semantic, err = prepareRuntimeCompletedReportSemanticPreservationV1(ctx, core, publication, advance, originalHistory, original, final, associatedOriginal, associatedFinal)
		if err != nil {
			return nil, err
		}
	}
	return originalHistory, nil
}

func prepareRuntimeReportHistoryEndpointV1(ctx context.Context, core *runtimeChildIdentityStartupV1, publication *runtimePublicationSemanticPreservationV1, advance *runtimeAuthorityAdvanceStartupV2, pending pendingapp.TrustedInventoryV1, files, evidenceFiles runtimeOriginalSemanticFilesV1, primaries recoveryport.PrimaryThreadReaderV1) (*runtimeOriginalReportHistoryV1, error) {
	check := func(id, key string) error {
		if err := core.revalidateKey(ctx); err != nil {
			return err
		}
		if id != core.verification.KeyID() || key != base64.RawURLEncoding.EncodeToString(core.verification.PublicKey()) {
			return errors.New("original report belongs to another current installation key")
		}
		return nil
	}
	readers, err := publicationstore.ParseOriginalReadersV1(ctx, files, publication.installation, check, publication.creationProofV1("report-publication"))
	if err != nil {
		return nil, err
	}
	dispositions := make([]domainpending.PendingWorkDispositionV1, 0, len(pending.Dispositions))
	for _, disposition := range pending.Dispositions {
		dispositions = append(dispositions, disposition)
	}
	sort.Slice(dispositions, func(i, j int) bool { return dispositions[i].WorkID < dispositions[j].WorkID })
	completed, err := pendingapp.PrepareCompletedReportSnapshotV1(ctx, pending.Receipts, dispositions, core.verification, primaries)
	if err != nil {
		return nil, err
	}
	if err := publicationapp.VerifyTrustedInventoryV1(ctx, readers.Attempts, readers.Receipts, readers.Indexes, readers.Selections, readers.Commits, readers.Decisions, readers.GrantSettlements, readers.StageCompletions, readers.DeliveryOutcomes, core.verification); err != nil {
		return nil, err
	}
	plan, err := publicationapp.PreflightRestartV1(ctx, publicationapp.RestartPreflightConfigV1{
		Pending: pending, Attempts: readers.Attempts, Receipts: readers.Receipts, Indexes: readers.Indexes,
		Commits: readers.Commits, Selections: readers.Selections, Decisions: readers.Decisions, GrantSettlements: readers.GrantSettlements,
		StageCompletions: readers.StageCompletions, DeliveryOutcomes: readers.DeliveryOutcomes, Ledgers: readers.Ledgers,
		Projections: readers.PIIProjections, Inspections: readers.Inspections, Threads: runtimeOriginalReportThreadsV1{ctx, primaries},
		Intents: advance.inventory, Settlements: advance.inventory, Authority: core.verification,
	})
	if err != nil {
		return nil, err
	}
	trust := core.originalRegistryTrust
	projection := trust.projection
	parseEvidence := func(files runtimeOriginalSemanticFilesV1) (evidencestore.OriginalGraphV1, error) {
		return evidencestore.ParseOriginalGraphV1(ctx, files, projection.InstallationID, projection.Enrollment.EnrollmentID, projection.Enrollment.WitnessKeyID, trust.witnessKey, core.verification)
	}
	graph, err := parseEvidence(evidenceFiles)
	if err != nil {
		return nil, err
	}
	advanceReader := runtimeOriginalReportAdvanceV1{advance.inventory}
	historical, err := publicationapp.NewHistoricalProjectedDeliveryAuthorityV1(publicationapp.ProjectedDeliveryAuthorityConfigV1{
		InstallationID: projection.InstallationID, EnrollmentID: projection.Enrollment.EnrollmentID, WitnessKeyID: projection.Enrollment.WitnessKeyID, WitnessKey: trust.witnessKey,
		Attempts: readers.Attempts, Receipts: readers.Receipts, Indexes: readers.Indexes, Selections: readers.Selections, Commits: readers.Commits,
		Decisions: readers.Decisions, GrantSettlements: readers.GrantSettlements, StageCompletions: readers.StageCompletions, DeliveryOutcomes: readers.DeliveryOutcomes,
		Ledgers: readers.Ledgers, Projections: readers.PIIProjections, Inspections: readers.Inspections, Artifacts: readers.Artifacts, ControlledMetadata: readers.Artifacts,
		Pending: completed, Authority: core.verification, Intents: advanceReader, Settlements: advanceReader,
		Bundles: runtimeOriginalReportBundlesV1(graph.Bundles), Observations: runtimeOriginalReportObservationsV1(graph.Observations),
	})
	if err != nil {
		return nil, err
	}
	for _, entry := range plan.Attempts {
		if entry.Intent != nil {
			if entry.Intent == nil || entry.Intent.Transition.EvidenceBundle == nil || entry.Index == nil {
				return nil, errors.New("report intent lacks its publication transition")
			}
			transition := entry.Intent.Transition.EvidenceBundle
			previous, previousFound := graph.Bundles[entry.Attempt.ExpectedEvidenceBundleDigest]
			next, nextFound := graph.Bundles[entry.Attempt.NextEvidenceBundleDigest]
			if !previousFound || !nextFound || !reflect.DeepEqual(previous, transition.PreviousBundle) || !reflect.DeepEqual(next, transition.NextBundle) ||
				previous.PublicationIndexDigest != entry.Attempt.ExpectedPublicationIndexDigest || previous.PublicationCount != entry.Attempt.ExpectedPublicationCount ||
				domainpublication.ValidatePublicationIndexWitnessRootV1(*entry.Index, next.PublicationIndexDigest, next.PublicationCount) != nil {
				return nil, errors.New("report intent lacks its exact Original transition bundles")
			}
		}
		if entry.Selection != nil {
			if entry.Selection == nil || entry.Candidate == nil || entry.Index == nil || entry.Settlement == nil {
				return nil, errors.New("original report selection lost its complete prefix")
			}
			selection := *entry.Selection
			previous, previousFound := graph.Bundles[selection.PreviousEvidenceBundleDigest]
			committed, committedFound := graph.Bundles[selection.CommittedEvidenceBundleDigest]
			observed, observationFound := graph.Observations[selection.ObservationDigest]
			if !previousFound || !committedFound || !observationFound || !reflect.DeepEqual(observed.Bundle, committed) ||
				observed.Request.RequestDigest != selection.ObserveRequestDigest || observed.Observation.ObservationDigest != selection.ObservationDigest {
				return nil, errors.New("original report selection lacks its exact stored witness exchange")
			}
			if err := domainpublication.ValidatePublicationCommitSelectionExactV1(selection, domainpublication.PublicationCommitSelectionInputV1{
				Attempt: entry.Attempt, SettlementDigest: entry.Settlement.RecordDigest,
				CommitInput: domainpublication.PublicationCommitReceiptInputV1{
					PreviousBundle: previous, CommittedBundle: committed, ObserveRequest: observed.Request, Observation: observed.Observation,
					Candidate: *entry.Candidate, Index: *entry.Index, InstallationID: projection.InstallationID, EnrollmentID: projection.Enrollment.EnrollmentID,
					AuthorityKeyID: core.verification.KeyID(), AuthorityPublicKey: core.verification.PublicKey(),
					WitnessKeyID: projection.Enrollment.WitnessKeyID, WitnessPublicKey: trust.witnessKey,
				},
			}); err != nil {
				return nil, errors.Join(errors.New("original report selection witness material is invalid"), err)
			}
		}
		if entry.DeliveryOutcome == nil {
			continue
		}
		if entry.Commit == nil {
			return nil, errors.New("original report outcome lost its commit")
		}
		frozen := entry.Stage.Context
		selector := publicationport.HistoricalProjectedDeliverySelectorV1{
			DeliveryID: domainpublication.ReportDeliveryOutcomeID(*entry.DeliveryOutcome), OutcomeRecordDigest: domainpublication.ReportDeliveryOutcomeRecordDigest(*entry.DeliveryOutcome),
			PublicationCommitDigest: entry.Commit.RecordDigest, ThreadID: frozen.ThreadID, TurnID: frozen.TurnID, ContextDigest: frozen.ContextDigest,
			CaseBindingHash: frozen.CaseBindingHash, ContextEpoch: frozen.ContextEpoch, DatasetSnapshotID: frozen.DatasetSnapshotID, SourceManifestHash: frozen.SourceManifestHash,
		}
		if err := historical.VerifyTrustedHistoricalDeliveryOutcomeV1(ctx, selector); err != nil {
			return nil, err
		}
	}

	return &runtimeOriginalReportHistoryV1{plan: plan, authority: historical}, nil
}

type runtimeOriginalReportThreadsV1 struct {
	ctx       context.Context
	primaries recoveryport.PrimaryThreadReaderV1
}

func (reader runtimeOriginalReportThreadsV1) GetThread(id string) (map[string]any, error) {
	snapshot, err := reader.primaries.ReadPrimaryThreadSnapshotV1(reader.ctx, id)
	return snapshot.Thread, err
}

type runtimeOriginalReportAdvanceV1 struct {
	inventory *authorityadvancefs.PreparedInventoryV2
}

func (reader runtimeOriginalReportAdvanceV1) ResolveIntent(ctx context.Context, id string) (result domainauthority.MonotonicAdvanceIntentV2, resultErr error) {
	found := false
	err := reader.inventory.VisitIntents(ctx, func(record domainauthority.MonotonicAdvanceIntentV2) error {
		if record.MutationID == id {
			result = record
			found = true
		}
		return nil
	})
	if err != nil {
		return domainauthority.MonotonicAdvanceIntentV2{}, err
	}
	if !found {
		return result, authorityport.ErrNotFound
	}
	return result, nil
}
func (reader runtimeOriginalReportAdvanceV1) ResolveSettlement(ctx context.Context, id string) (result domainauthority.MonotonicAdvanceSettlementV2, resultErr error) {
	found := false
	err := reader.inventory.VisitSettlements(ctx, func(record domainauthority.MonotonicAdvanceSettlementV2) error {
		if record.MutationID == id {
			result = record
			found = true
		}
		return nil
	})
	if err != nil {
		return domainauthority.MonotonicAdvanceSettlementV2{}, err
	}
	if !found {
		return result, authorityport.ErrNotFound
	}
	return result, nil
}

type runtimeOriginalReportBundlesV1 map[string]domainevidence.EvidenceAuthorityBundleV1

func (reader runtimeOriginalReportBundlesV1) Resolve(ctx context.Context, id string) (domainevidence.EvidenceAuthorityBundleV1, error) {
	record, found := reader[id]
	return cloneRuntimeOriginalReportValueV1(ctx, record, found)
}

type runtimeOriginalReportObservationsV1 map[string]evidenceport.ObservationBundle

func (reader runtimeOriginalReportObservationsV1) Resolve(ctx context.Context, id string) (evidenceport.ObservationBundle, error) {
	record, found := reader[id]
	return cloneRuntimeOriginalReportValueV1(ctx, record, found)
}
func cloneRuntimeOriginalReportValueV1[T any](ctx context.Context, record T, found bool) (result T, resultErr error) {
	if ctx == nil {
		return result, errors.New("original report evidence context is unavailable")
	}
	if err := context.Cause(ctx); err != nil {
		return result, err
	}
	if !found {
		return result, errors.New("original report stored witness material is missing")
	}
	body, err := json.Marshal(record)
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return result, err
	}
	return result, context.Cause(ctx)
}
