package evidence

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	gateprojection "analytix.local/runtime-go/internal/app/gateprojection"
	appturn "analytix.local/runtime-go/internal/app/turn"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	acceptedfinaleventport "analytix.local/runtime-go/internal/ports/acceptedfinalevent"
)

type AcceptedFinalEventReservationWorkV1 = acceptedfinaleventport.ReservationWork

type FinalPublicationEventIO struct {
	ReadThread           func(context.Context, appturn.AcceptedFinalCompletionStore, domainevidence.PrivateAcceptedFinalRecord) (map[string]any, error)
	ReadCASObservation   func(context.Context, appturn.AcceptedFinalCompletionStore, domainevidence.PrivateAcceptedFinalRecord) (domainevidence.AcceptedFinalCASObservationV1, error)
	LoadEvents           func(context.Context, appturn.AcceptedFinalCompletionStore, string) ([]map[string]any, error)
	AppendEvents         func(context.Context, appturn.AcceptedFinalCompletionStore, []map[string]any) ([]map[string]any, error)
	Readback             acceptedfinaleventport.Readback
	WithEventReservation func(
		context.Context,
		appturn.AcceptedFinalCompletionStore,
		string,
		string,
		AcceptedFinalEventReservationWorkV1,
	) error
	ActivateAndPublishEvents func(
		context.Context,
		appturn.AcceptedFinalCompletionStore,
		[]map[string]any,
		domainevent.AcceptedFinalDeliverySealV1,
		func() error,
	) error
	// InventoryReadConcurrency opts a known-concurrency-safe adapter into
	// bounded parallel event replay during one read-only startup preflight.
	// Zero and one retain sequential reads for adapters without that contract.
	InventoryReadConcurrency int
}

func (io FinalPublicationEventIO) validForPlanning() bool {
	return io.ReadThread != nil && io.LoadEvents != nil
}

func (io FinalPublicationEventIO) validForApply() bool {
	return io.validForPlanning() && io.AppendEvents != nil
}

func (io FinalPublicationEventIO) validForLiveDelivery() bool {
	return io.validForApply() && io.Readback != nil && io.WithEventReservation != nil &&
		io.ActivateAndPublishEvents != nil
}

// AcceptedFinalEventReconciliationPlan is an immutable, non-authoritative
// repair plan. Planning performs every read-side integrity check without
// appending an event; applying revalidates the live log before each write.
type AcceptedFinalEventReconciliationPlan struct {
	privateRecord domainevidence.PrivateAcceptedFinalRecord
	publication   appturn.AcceptedFinalPublicationPlan
	auditOnly     bool
	factAuthority appturn.FactFinalMutationAuthority
}

// ReconcileAcceptedFinalEventInventory is the only startup reconciliation
// entrypoint. It completes a whole-inventory dry run before the first append,
// then applies repairable suffixes and validates the complete readback.
func ReconcileAcceptedFinalEventInventory(ctx context.Context, io FinalPublicationEventIO, store appturn.AcceptedFinalCompletionStore, reader AcceptedFinalPublicReader, inventory FinalAuthorityInventory) error {
	plans, err := PreflightAcceptedFinalEventReconciliations(ctx, io, store, reader, inventory.Committed)
	if err != nil {
		return err
	}
	return ApplyAcceptedFinalEventReconciliationInventory(ctx, io, store, reader, plans)
}

// ApplyAcceptedFinalEventReconciliationInventory revalidates every preflighted
// plan before writing and then verifies the complete event inventory. Startup
// callers must obtain all plans before applying any other persistence repair.
func ApplyAcceptedFinalEventReconciliationInventory(ctx context.Context, io FinalPublicationEventIO, store appturn.AcceptedFinalCompletionStore, reader AcceptedFinalPublicReader, plans []AcceptedFinalEventReconciliationPlan) error {
	committed := make([]domainevidence.PrivateAcceptedFinalRecord, 0, len(plans))
	for _, plan := range plans {
		committed = append(committed, plan.privateRecord)
	}
	return ApplyAcceptedFinalEventReconciliationInventoryWithQuarantine(ctx, io, store, reader, plans, committed, nil, nil)
}

// ApplyAcceptedFinalEventReconciliationInventoryWithQuarantine applies only
// terminal-complete plans. Quarantined public winners are revalidated as
// audit-only records but are never repaired into publication events.
func ApplyAcceptedFinalEventReconciliationInventoryWithQuarantine(
	ctx context.Context,
	io FinalPublicationEventIO,
	store appturn.AcceptedFinalCompletionStore,
	reader AcceptedFinalPublicReader,
	plans []AcceptedFinalEventReconciliationPlan,
	committed []domainevidence.PrivateAcceptedFinalRecord,
	legacyQuarantined []domainevidence.PrivateAcceptedFinalRecord,
	auditOnlyRecords []domainevidence.PrivateAcceptedFinalRecord,
) error {
	if !io.validForApply() || store == nil || reader == nil {
		return errors.New("accepted final event reconciliation dependencies are invalid")
	}
	for _, plan := range plans {
		if err := ApplyAcceptedFinalEventReconciliation(ctx, io, store, plan); err != nil {
			return err
		}
	}
	_, err := PreflightAcceptedFinalEventReconciliationsWithQuarantine(
		ctx, io, store, reader, committed, legacyQuarantined, auditOnlyRecords,
	)
	return err
}

// PreflightAcceptedFinalEventReconciliations validates the complete public
// event inventory and simulates every repair in memory. It returns no plan
// unless the fully projected replay for every thread is authoritative.
func PreflightAcceptedFinalEventReconciliations(ctx context.Context, io FinalPublicationEventIO, store appturn.AcceptedFinalCompletionStore, reader AcceptedFinalPublicReader, committed []domainevidence.PrivateAcceptedFinalRecord) ([]AcceptedFinalEventReconciliationPlan, error) {
	return PreflightAcceptedFinalEventReconciliationsWithQuarantine(ctx, io, store, reader, committed, nil, nil)
}

const maxAcceptedFinalInventoryReadConcurrencyV1 = 8

type acceptedFinalInventorySnapshotV1 struct {
	thread map[string]any
	events []map[string]any
}

func captureAcceptedFinalInventorySnapshotsV1(
	ctx context.Context,
	io FinalPublicationEventIO,
	store appturn.AcceptedFinalCompletionStore,
	reader AcceptedFinalPublicReader,
	threadIDs []string,
) (map[string]acceptedFinalInventorySnapshotV1, error) {
	for index, threadID := range threadIDs {
		if index > 0 && threadID == threadIDs[index-1] {
			return nil, errors.New("accepted final reconciliation inventory contains a duplicate thread")
		}
	}
	snapshots := make([]acceptedFinalInventorySnapshotV1, len(threadIDs))
	for index, threadID := range threadIDs {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		thread, err := reader.GetThread(threadID)
		if err != nil {
			return nil, err
		}
		snapshots[index].thread = thread
	}
	concurrency := io.InventoryReadConcurrency
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > maxAcceptedFinalInventoryReadConcurrencyV1 {
		concurrency = maxAcceptedFinalInventoryReadConcurrencyV1
	}
	if concurrency > len(threadIDs) {
		concurrency = len(threadIDs)
	}
	loadErrors := make([]error, len(threadIDs))
	if concurrency == 1 {
		for index, threadID := range threadIDs {
			if ctx != nil {
				if err := ctx.Err(); err != nil {
					loadErrors[index] = err
					continue
				}
			}
			snapshots[index].events, loadErrors[index] = io.LoadEvents(ctx, store, threadID)
		}
	} else if concurrency > 1 {
		jobs := make(chan int)
		var workers sync.WaitGroup
		workers.Add(concurrency)
		for worker := 0; worker < concurrency; worker++ {
			go func() {
				defer workers.Done()
				for index := range jobs {
					if ctx != nil {
						if err := ctx.Err(); err != nil {
							loadErrors[index] = err
							continue
						}
					}
					snapshots[index].events, loadErrors[index] = io.LoadEvents(ctx, store, threadIDs[index])
				}
			}()
		}
		for index := range threadIDs {
			jobs <- index
		}
		close(jobs)
		workers.Wait()
	}
	byThread := make(map[string]acceptedFinalInventorySnapshotV1, len(threadIDs))
	for index, threadID := range threadIDs {
		if loadErrors[index] != nil {
			return nil, loadErrors[index]
		}
		byThread[threadID] = snapshots[index]
	}
	return byThread, nil
}

// PreflightAcceptedFinalEventReconciliationsWithQuarantine requires an
// explicit terminal-complete publication allowlist. Quarantined winners may
// retain a validated raw audit prefix, but missing events are never repaired
// and none of their markers can be mistaken for a committed publication.
func PreflightAcceptedFinalEventReconciliationsWithQuarantine(
	ctx context.Context,
	io FinalPublicationEventIO,
	store appturn.AcceptedFinalCompletionStore,
	reader AcceptedFinalPublicReader,
	committed []domainevidence.PrivateAcceptedFinalRecord,
	legacyQuarantined []domainevidence.PrivateAcceptedFinalRecord,
	auditOnlyRecords []domainevidence.PrivateAcceptedFinalRecord,
) ([]AcceptedFinalEventReconciliationPlan, error) {
	if !io.validForPlanning() || store == nil || reader == nil {
		return nil, errors.New("accepted final event reconciliation dependencies are invalid")
	}
	threadIDs, err := reader.AllThreadIDs()
	if err != nil {
		return nil, err
	}
	sort.Strings(threadIDs)
	snapshots, err := captureAcceptedFinalInventorySnapshotsV1(ctx, io, store, reader, threadIDs)
	if err != nil {
		return nil, err
	}
	byThread := map[string][]AcceptedFinalEventReconciliationPlan{}
	byDigest := map[string]AcceptedFinalEventReconciliationPlan{}
	referencedThreads := map[string]bool{}
	add := func(privateRecord domainevidence.PrivateAcceptedFinalRecord, repair, auditOnly bool) error {
		threadID := privateRecord.AcceptedFinal.ThreadID
		snapshot, ok := snapshots[threadID]
		if !ok {
			return errors.New("accepted final reconciliation inventory references a missing thread")
		}
		plan, err := planAcceptedFinalEventReconciliationFromSnapshot(
			privateRecord, nil, auditOnly, snapshot.thread, snapshot.events,
		)
		if err != nil {
			return err
		}
		digest := privateRecord.AcceptedFinal.RecordDigest
		if _, duplicate := byDigest[digest]; duplicate {
			return errors.New("accepted final reconciliation inventory contains a duplicate commit")
		}
		byDigest[digest] = plan
		referencedThreads[threadID] = true
		if repair {
			byThread[threadID] = append(byThread[threadID], plan)
		}
		return nil
	}
	for _, privateRecord := range committed {
		if err := add(privateRecord, true, false); err != nil {
			return nil, err
		}
	}
	for _, privateRecord := range legacyQuarantined {
		if err := add(privateRecord, false, true); err != nil {
			return nil, err
		}
	}
	for _, privateRecord := range auditOnlyRecords {
		if err := add(privateRecord, false, true); err != nil {
			return nil, err
		}
	}
	replayAuthority, err := newAcceptedFinalReplayAuthorityV1(committed, legacyQuarantined, auditOnlyRecords)
	if err != nil {
		return nil, err
	}
	if validator, ok := reader.(caseCompactionAuthorityReaderV1); ok {
		replayAuthority.validateCaseCompactionTurn = validator.ValidateCaseCompactionAuthorityTurnV1
	}
	seenThread := map[string]bool{}
	ordered := make([]AcceptedFinalEventReconciliationPlan, 0, len(committed))
	for _, threadID := range threadIDs {
		if seenThread[threadID] {
			return nil, errors.New("accepted final reconciliation inventory contains a duplicate thread")
		}
		seenThread[threadID] = true
		snapshot := snapshots[threadID]
		thread := snapshot.thread
		events := snapshot.events
		if err := validateFinalPublicationEventSequence(events); err != nil {
			return nil, err
		}
		if err := validateInventoryPublicationMarkers(events, byDigest); err != nil {
			return nil, err
		}
		threadPlans, projected, err := projectAcceptedFinalEventRepairs(events, byThread[threadID])
		if err != nil {
			return nil, err
		}
		if err := validateAcceptedFinalEventReplayWithAuthority(thread, projected, replayAuthority); err != nil {
			return nil, err
		}
		ordered = append(ordered, threadPlans...)
	}
	for threadID := range referencedThreads {
		if !seenThread[threadID] {
			return nil, errors.New("accepted final reconciliation inventory references a missing thread")
		}
	}
	return ordered, nil
}

func validateInventoryPublicationMarkers(events []map[string]any, allowed map[string]AcceptedFinalEventReconciliationPlan) error {
	for _, event := range events {
		commitID := strings.TrimSpace(authorityString(event, "publicationCommitId"))
		acceptedDigest := strings.TrimSpace(authorityString(event, "acceptedFinalDigest"))
		if commitID == "" && acceptedDigest == "" {
			continue
		}
		if commitID == "" || acceptedDigest == "" || commitID != acceptedDigest {
			return errors.New("accepted final replay contains a partial or mismatched publication marker")
		}
		plan, ok := allowed[commitID]
		if !ok {
			return errors.New("accepted final replay contains an unknown or uncommitted publication marker")
		}
		eventID := strings.TrimSpace(authorityString(event, "publicationEventId"))
		matched := false
		for _, expected := range plan.publication.Events {
			if expected.EventID != eventID {
				continue
			}
			matched = validatePublicationEvent(event, commitID, expected) == nil
			break
		}
		if !matched {
			return errors.New("accepted final replay contains an event outside its committed manifest")
		}
	}
	return nil
}

func projectAcceptedFinalEventRepairs(events []map[string]any, plans []AcceptedFinalEventReconciliationPlan) ([]AcceptedFinalEventReconciliationPlan, []map[string]any, error) {
	type pendingPlan struct {
		plan   AcceptedFinalEventReconciliationPlan
		prefix int
	}
	complete := make([]pendingPlan, 0, len(plans))
	partial := make([]pendingPlan, 0, 1)
	missing := make([]pendingPlan, 0, len(plans))
	for _, plan := range plans {
		prefix, err := inspectAcceptedFinalEventPlan(events, plan.privateRecord.AcceptedFinal, plan.publication)
		if err != nil {
			return nil, nil, err
		}
		pending := pendingPlan{plan: plan, prefix: prefix}
		switch {
		case prefix == len(plan.publication.Events):
			complete = append(complete, pending)
		case prefix > 0:
			partial = append(partial, pending)
		default:
			missing = append(missing, pending)
		}
	}
	if len(partial) > 1 {
		return nil, nil, errors.New("accepted final replay contains multiple partial publication suffixes")
	}
	sort.Slice(missing, func(left int, right int) bool {
		leftRecord := missing[left].plan.privateRecord.AcceptedFinal
		rightRecord := missing[right].plan.privateRecord.AcceptedFinal
		if leftRecord.AcceptedAt != rightRecord.AcceptedAt {
			return leftRecord.AcceptedAt < rightRecord.AcceptedAt
		}
		return leftRecord.RecordDigest < rightRecord.RecordDigest
	})
	application := append(partial, missing...)
	projected := clonePublicationEvents(events)
	nextSeq := 1
	if len(projected) != 0 {
		lastSeq, _ := contracts.NumericSeq(projected[len(projected)-1]["seq"])
		nextSeq = lastSeq + 1
	}
	for _, pending := range application {
		for _, expected := range pending.plan.publication.Events[pending.prefix:] {
			event := contracts.CloneMap(expected.Draft)
			event["seq"] = nextSeq
			nextSeq++
			projected = append(projected, event)
		}
	}
	ordered := make([]AcceptedFinalEventReconciliationPlan, 0, len(plans))
	for _, pending := range application {
		ordered = append(ordered, pending.plan)
	}
	for _, pending := range complete {
		ordered = append(ordered, pending.plan)
	}
	return ordered, projected, nil
}

func clonePublicationEvents(events []map[string]any) []map[string]any {
	cloned := make([]map[string]any, 0, len(events))
	for _, event := range events {
		cloned = append(cloned, contracts.CloneMap(event))
	}
	return cloned
}

func PlanAcceptedFinalEventReconciliation(ctx context.Context, io FinalPublicationEventIO, store appturn.AcceptedFinalCompletionStore, privateRecord domainevidence.PrivateAcceptedFinalRecord) (AcceptedFinalEventReconciliationPlan, error) {
	return planAcceptedFinalEventReconciliation(ctx, io, store, privateRecord, nil, false)
}

func PlanAcceptedFinalEventReconciliationWithAuthority(
	ctx context.Context,
	io FinalPublicationEventIO,
	store appturn.AcceptedFinalCompletionStore,
	privateRecord domainevidence.PrivateAcceptedFinalRecord,
	factAuthority appturn.FactFinalMutationAuthority,
) (AcceptedFinalEventReconciliationPlan, error) {
	return planAcceptedFinalEventReconciliation(ctx, io, store, privateRecord, factAuthority, false)
}

func planAcceptedFinalEventReconciliation(
	ctx context.Context,
	io FinalPublicationEventIO,
	store appturn.AcceptedFinalCompletionStore,
	privateRecord domainevidence.PrivateAcceptedFinalRecord,
	factAuthority appturn.FactFinalMutationAuthority,
	auditOnly bool,
) (AcceptedFinalEventReconciliationPlan, error) {
	if !io.validForPlanning() || store == nil {
		return AcceptedFinalEventReconciliationPlan{}, errors.New("accepted final event reconciliation dependencies are invalid")
	}
	if _, err := acceptedFinalEventReconciliationValidator(privateRecord, factAuthority, auditOnly); err != nil {
		return AcceptedFinalEventReconciliationPlan{}, err
	}
	thread, err := io.ReadThread(ctx, store, privateRecord)
	if err != nil {
		return AcceptedFinalEventReconciliationPlan{}, errors.New("accepted final event reconciliation has no matching public commit")
	}
	events, err := io.LoadEvents(ctx, store, privateRecord.AcceptedFinal.ThreadID)
	if err != nil {
		return AcceptedFinalEventReconciliationPlan{}, err
	}
	return planAcceptedFinalEventReconciliationFromSnapshot(
		privateRecord, factAuthority, auditOnly, thread, events,
	)
}

func acceptedFinalEventReconciliationValidator(
	privateRecord domainevidence.PrivateAcceptedFinalRecord,
	factAuthority appturn.FactFinalMutationAuthority,
	auditOnly bool,
) (func(domainevidence.PrivateAcceptedFinalRecord) error, error) {
	validate := func(record domainevidence.PrivateAcceptedFinalRecord) error {
		return appturn.ValidatePrivateAcceptedFinalMutationAuthority(record, factAuthority)
	}
	if auditOnly {
		if factAuthority != nil {
			return nil, errors.New("audit-only accepted final cannot carry fact mutation authority")
		}
		validate = domainevidence.ValidatePrivateAcceptedFinalAuditAuthority
	}
	if validate(privateRecord) != nil {
		return nil, errors.New("accepted final event reconciliation dependencies are invalid")
	}
	return validate, nil
}

func planAcceptedFinalEventReconciliationFromSnapshot(
	privateRecord domainevidence.PrivateAcceptedFinalRecord,
	factAuthority appturn.FactFinalMutationAuthority,
	auditOnly bool,
	thread map[string]any,
	events []map[string]any,
) (AcceptedFinalEventReconciliationPlan, error) {
	validate, err := acceptedFinalEventReconciliationValidator(privateRecord, factAuthority, auditOnly)
	if err != nil {
		return AcceptedFinalEventReconciliationPlan{}, err
	}
	publication, err := buildAcceptedFinalPreflightPlan(privateRecord)
	if err != nil {
		return AcceptedFinalEventReconciliationPlan{}, err
	}
	if validateAcceptedFinalThread(thread, privateRecord, validate) != nil {
		return AcceptedFinalEventReconciliationPlan{}, errors.New("accepted final event reconciliation has no matching public commit")
	}
	if _, err := inspectAcceptedFinalEventPlan(events, privateRecord.AcceptedFinal, publication); err != nil {
		return AcceptedFinalEventReconciliationPlan{}, err
	}
	return AcceptedFinalEventReconciliationPlan{
		privateRecord: privateRecord, publication: publication,
		auditOnly: auditOnly, factAuthority: factAuthority,
	}, nil
}

func ApplyAcceptedFinalEventReconciliation(ctx context.Context, io FinalPublicationEventIO, store appturn.AcceptedFinalCompletionStore, reconciliation AcceptedFinalEventReconciliationPlan) error {
	if !io.validForApply() || store == nil || reconciliation.auditOnly ||
		appturn.ValidatePrivateAcceptedFinalMutationAuthority(
			reconciliation.privateRecord, reconciliation.factAuthority,
		) != nil || len(reconciliation.publication.Events) == 0 {
		return errors.New("accepted final event reconciliation dependencies are invalid")
	}
	current, err := PlanAcceptedFinalEventReconciliationWithAuthority(
		ctx, io, store, reconciliation.privateRecord, reconciliation.factAuthority,
	)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current.publication, reconciliation.publication) {
		return errors.New("accepted final event reconciliation plan changed before apply")
	}
	if err := reconcileAcceptedFinalEventPlan(
		ctx, io, store, reconciliation.privateRecord, reconciliation.publication, reconciliation.factAuthority,
	); err != nil {
		return err
	}
	return nil
}

func ReconcileAcceptedFinalEvents(ctx context.Context, io FinalPublicationEventIO, store appturn.AcceptedFinalCompletionStore, privateRecord domainevidence.PrivateAcceptedFinalRecord) error {
	return ReconcileAcceptedFinalEventsWithAuthority(ctx, io, store, privateRecord, nil)
}

func ReconcileAcceptedFinalEventsWithAuthority(
	ctx context.Context,
	io FinalPublicationEventIO,
	store appturn.AcceptedFinalCompletionStore,
	privateRecord domainevidence.PrivateAcceptedFinalRecord,
	factAuthority appturn.FactFinalMutationAuthority,
) error {
	plan, err := PlanAcceptedFinalEventReconciliationWithAuthority(ctx, io, store, privateRecord, factAuthority)
	if err != nil {
		return err
	}
	return ApplyAcceptedFinalEventReconciliation(ctx, io, store, plan)
}

func LoadVerifiedAcceptedFinalEvents(ctx context.Context, io FinalPublicationEventIO, store appturn.AcceptedFinalCompletionStore, privateRecord domainevidence.PrivateAcceptedFinalRecord) ([]map[string]any, error) {
	return LoadVerifiedAcceptedFinalEventsWithAuthority(ctx, io, store, privateRecord, nil)
}

func LoadVerifiedAcceptedFinalEventsWithAuthority(
	ctx context.Context,
	io FinalPublicationEventIO,
	store appturn.AcceptedFinalCompletionStore,
	privateRecord domainevidence.PrivateAcceptedFinalRecord,
	factAuthority appturn.FactFinalMutationAuthority,
) ([]map[string]any, error) {
	if !io.validForPlanning() || store == nil ||
		appturn.ValidatePrivateAcceptedFinalMutationAuthority(privateRecord, factAuthority) != nil {
		return nil, errors.New("accepted final publication readback dependencies are invalid")
	}
	plan, err := appturn.BuildAcceptedFinalPublicationPlan(privateRecord.AcceptedFinal, privateRecord.RenderedText, privateRecord.PublicationIntent)
	if err != nil {
		return nil, err
	}
	events, err := io.LoadEvents(ctx, store, privateRecord.AcceptedFinal.ThreadID)
	if err != nil {
		return nil, err
	}
	verified, err := publicationEventsForCommit(events, privateRecord.AcceptedFinal.RecordDigest)
	if err != nil || len(verified) != len(plan.Events) {
		return nil, errors.New("accepted final event manifest readback is incomplete")
	}
	ordered := make([]map[string]any, 0, len(plan.Events))
	previousSeq := 0
	for _, expected := range plan.Events {
		actual := verified[expected.EventID]
		if err := validatePublicationEvent(actual, privateRecord.AcceptedFinal.RecordDigest, expected); err != nil {
			return nil, err
		}
		seq, ok := contracts.NumericSeq(actual["seq"])
		if !ok || (previousSeq != 0 && seq != previousSeq+1) {
			return nil, errors.New("accepted final publication event readback is not contiguous")
		}
		previousSeq = seq
		ordered = append(ordered, contracts.CloneMap(actual))
	}
	return ordered, nil
}

// FinalizeAcceptedFinalEventDeliveryV1 is the only live accepted-final event
// path. It keeps the same-thread reservation across reconciliation, exact
// durable readback, pre-sign verification, fallible delivery preparation,
// projection activation, and the no-error live commit.
func FinalizeAcceptedFinalEventDeliveryV1(
	ctx context.Context,
	io FinalPublicationEventIO,
	store appturn.AcceptedFinalCompletionStore,
	privateRecord domainevidence.PrivateAcceptedFinalRecord,
	projectionLease *gateprojection.StagedFinalProjectionLeaseV1,
	factAuthority appturn.FactFinalMutationAuthority,
) error {
	if !io.validForLiveDelivery() || store == nil || projectionLease == nil ||
		appturn.ValidatePrivateAcceptedFinalMutationAuthority(privateRecord, factAuthority) != nil {
		return errors.New("accepted final live delivery dependencies are invalid")
	}
	return io.WithEventReservation(
		ctx,
		store,
		privateRecord.SecurityContext.ThreadID,
		privateRecord.AcceptedFinal.RecordDigest,
		func(reservedContext context.Context) error {
			if err := ReconcileAcceptedFinalEventsWithAuthority(
				reservedContext, io, store, privateRecord, factAuthority,
			); err != nil {
				return err
			}
			verifiedEvents, err := LoadVerifiedAcceptedFinalEventsWithAuthority(
				reservedContext, io, store, privateRecord, factAuthority,
			)
			if err != nil {
				return err
			}
			deliverySeal, err := projectionLease.SealAcceptedFinalDelivery(reservedContext, verifiedEvents)
			if err != nil {
				return err
			}
			return io.ActivateAndPublishEvents(
				reservedContext, store, verifiedEvents, deliverySeal, projectionLease.Activate,
			)
		},
	)
}

func reconcileAcceptedFinalEventPlan(
	ctx context.Context,
	io FinalPublicationEventIO,
	store appturn.AcceptedFinalCompletionStore,
	privateRecord domainevidence.PrivateAcceptedFinalRecord,
	plan appturn.AcceptedFinalPublicationPlan,
	factAuthority appturn.FactFinalMutationAuthority,
) error {
	record := privateRecord.AcceptedFinal
	events, err := io.LoadEvents(ctx, store, record.ThreadID)
	if err != nil {
		return err
	}
	prefixLength, err := inspectAcceptedFinalEventPlan(events, record, plan)
	if err != nil {
		return err
	}
	missing := plan.Events[prefixLength:]
	if len(missing) > 0 {
		drafts := make([]map[string]any, 0, len(missing))
		for _, expected := range missing {
			drafts = append(drafts, contracts.CloneMap(expected.Draft))
		}
		var written []map[string]any
		err := appturn.UsePrivateAcceptedFinalMutationAuthority(privateRecord, factAuthority, func() error {
			var appendErr error
			written, appendErr = io.AppendEvents(ctx, store, drafts)
			return appendErr
		})
		if err != nil {
			return err
		}
		if len(written) != len(missing) {
			return errors.New("accepted final event bundle write is incomplete")
		}
		for index, expected := range missing {
			if err := validatePublicationEvent(written[index], record.RecordDigest, expected); err != nil {
				return err
			}
		}
	}
	reloaded, err := io.LoadEvents(ctx, store, record.ThreadID)
	if err != nil {
		return err
	}
	verified, err := publicationEventsForCommit(reloaded, record.RecordDigest)
	if err != nil || len(verified) != len(plan.Events) {
		return errors.New("accepted final event manifest readback is incomplete")
	}
	if err := validateFinalPublicationEventSequence(reloaded); err != nil {
		return err
	}
	previousSeq := 0
	for _, expected := range plan.Events {
		actual := verified[expected.EventID]
		if err := validatePublicationEvent(actual, record.RecordDigest, expected); err != nil {
			return err
		}
		seq, _ := contracts.NumericSeq(actual["seq"])
		if previousSeq != 0 && seq != previousSeq+1 {
			return errors.New("accepted final publication event manifest is not contiguous in slot order")
		}
		previousSeq = seq
	}
	return nil
}

func inspectAcceptedFinalEventPlan(events []map[string]any, record domainevidence.AcceptedFinalRecord, plan appturn.AcceptedFinalPublicationPlan) (int, error) {
	if err := validateFinalPublicationEventSequence(events); err != nil {
		return 0, err
	}
	existing, err := publicationEventsForCommit(events, record.RecordDigest)
	if err != nil {
		return 0, err
	}
	prefixLength := 0
	previousSeq := 0
	missing := false
	for _, expected := range plan.Events {
		current, ok := existing[expected.EventID]
		if !ok {
			missing = true
			continue
		}
		if missing {
			return 0, errors.New("accepted final publication event manifest has a non-prefix repair gap")
		}
		if err := validatePublicationEvent(current, record.RecordDigest, expected); err != nil {
			return 0, err
		}
		seq, _ := contracts.NumericSeq(current["seq"])
		if previousSeq != 0 && seq != previousSeq+1 {
			return 0, errors.New("accepted final publication event manifest is not contiguous in slot order")
		}
		previousSeq = seq
		prefixLength++
	}
	if len(existing) != prefixLength {
		return 0, errors.New("accepted final event manifest contains unexpected publication events")
	}
	if prefixLength > 0 && prefixLength < len(plan.Events) {
		lastSeq, _ := contracts.NumericSeq(events[len(events)-1]["seq"])
		if previousSeq != lastSeq {
			return 0, errors.New("accepted final partial publication bundle is not the event-log suffix")
		}
	}
	return prefixLength, nil
}

func validateFinalPublicationEventSequence(events []map[string]any) error {
	previous := 0
	for _, event := range events {
		seq, ok := contracts.NumericSeq(event["seq"])
		if !ok || seq <= 0 || (previous != 0 && seq != previous+1) {
			return errors.New("accepted final event log sequence is not positive and contiguous")
		}
		previous = seq
	}
	return nil
}

func publicationEventsForCommit(events []map[string]any, commitID string) (map[string]map[string]any, error) {
	out := map[string]map[string]any{}
	for _, event := range events {
		eventCommitID := strings.TrimSpace(authorityString(event, "publicationCommitId"))
		acceptedDigest := strings.TrimSpace(authorityString(event, "acceptedFinalDigest"))
		if eventCommitID == "" && acceptedDigest == "" {
			continue
		}
		if eventCommitID == "" || acceptedDigest == "" || eventCommitID != acceptedDigest {
			return nil, errors.New("accepted final replay contains a partial or mismatched publication marker")
		}
		if eventCommitID != commitID {
			continue
		}
		eventID := strings.TrimSpace(authorityString(event, "publicationEventId"))
		if eventID == "" {
			return nil, errors.New("accepted final replay contains an unmanifested publication event")
		}
		if _, duplicate := out[eventID]; duplicate {
			return nil, errors.New("accepted final replay contains a duplicate publication event")
		}
		out[eventID] = event
	}
	return out, nil
}

func validatePublicationEvent(actual map[string]any, commitID string, expected appturn.AcceptedFinalPublicationEvent) error {
	if actual == nil || authorityString(actual, "publicationCommitId") != commitID ||
		authorityString(actual, "acceptedFinalDigest") != commitID || authorityString(actual, "publicationEventId") != expected.EventID ||
		authorityString(actual, "publicationSlot") != expected.Slot ||
		authorityString(actual, "publicationPayloadDigest") != expected.PayloadDigest ||
		appturn.AcceptedFinalPublicationPayloadDigest(actual) != expected.PayloadDigest {
		return fmt.Errorf("accepted final publication event %s is inconsistent", expected.Slot)
	}
	return nil
}

func validateCommittedAcceptedFinalThread(thread map[string]any, privateRecord domainevidence.PrivateAcceptedFinalRecord) error {
	return validateAcceptedFinalThread(thread, privateRecord, domainevidence.ValidatePrivateAcceptedFinalPublicationAuthority)
}

func validateAcceptedFinalThread(thread map[string]any, privateRecord domainevidence.PrivateAcceptedFinalRecord, validate func(domainevidence.PrivateAcceptedFinalRecord) error) error {
	if validate == nil || validate(privateRecord) != nil || authorityString(thread, "id") != privateRecord.SecurityContext.ThreadID {
		return errors.New("accepted final public thread identity is invalid")
	}
	expected := privateRecord.AcceptedFinal
	for _, turn := range authorityTurns(thread) {
		if authorityString(turn, "id") != expected.TurnID {
			continue
		}
		record, err := domainevidence.ParseAcceptedFinalRecord(turn["acceptedFinal"])
		securityContext, contextErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
		expectedStatus, statusOK := domainevidence.FinalAnswerTerminalStatus(expected.TerminalReason)
		if err != nil || contextErr != nil || !reflect.DeepEqual(record, expected) || !reflect.DeepEqual(securityContext, privateRecord.SecurityContext) ||
			!statusOK || authorityString(turn, "status") != expectedStatus || validateAcceptedFinalItems(turn, &expected) != nil ||
			validateAcceptedFinalPublicationItems(turn, privateRecord) != nil {
			return errors.New("accepted final public turn authority is invalid")
		}
		plan, err := buildAcceptedFinalPreflightPlan(privateRecord)
		if err != nil {
			return err
		}
		if discard, required := plan.TurnFields["discard"]; required && turn["discard"] != discard {
			return errors.New("accepted final public turn discard state is invalid")
		}
		return nil
	}
	return errors.New("accepted final public turn is missing")
}
