package evidence

import (
	"context"
	"errors"
	"reflect"
	"testing"

	appturn "analytix.local/runtime-go/internal/app/turn"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

type finalEventPreservationStubV1 struct {
	input PreservedFinalEventInventoryV1
	err   error
}

func TestPreservedFinalEventsGuardIndependentAppendAfterLastRead(t *testing.T) {
	for _, fault := range []bool{false, true} {
		t.Run(map[bool]string{false: "independent_repair", true: "late_original_failure"}[fault], func(t *testing.T) {
			held, heldThread, heldPublication, _, _ := historicalFactAuditFixture(t)
			observer := finalEventPreservationFixtureV1(t, held, heldThread, acceptedFinalPublicationEventsForTest(heldPublication, 1))
			_, template := evidenceIssuerFixture(t)
			frozen := newEvidenceCaseContextV2(t, domainsecurity.TurnSecurityContextInput{ThreadID: "thread-independent", TurnID: "turn-independent", WorkspaceRealPath: template.Context.WorkspaceRealPath, TenantID: template.Context.TenantID, UserID: template.Context.UserID, CaseID: "case-independent", CaseBindingHash: domainsecurity.SHA256Hex([]byte("independent-binding")), DatasetSnapshotID: template.Context.DatasetSnapshotID, SourceManifestHash: template.Context.SourceManifestHash, ContextEpoch: 1, IssuedAt: evidenceIssuerTime()})
			finalizer, _, _, privateStore := newTestCasePublicationFinalizer()
			store := &caseTerminalStoreStub{}
			if _, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{Store: store, Context: frozen, TerminalReason: TerminalSuccess, ThreadID: frozen.ThreadID, TurnID: frozen.TurnID, AcceptedAt: evidenceIssuerTime()}); err != nil {
				t.Fatal(err)
			}
			records, err := privateStore.List(context.Background())
			if err != nil || len(records) != 1 {
				t.Fatalf("independent final fixture: %v", err)
			}
			thread := acceptedFinalReaderForStore(frozen, store).threads[frozen.ThreadID]
			reader := acceptedFinalPublicReaderStub{threads: map[string]map[string]any{frozen.ThreadID: thread}}
			events := []map[string]any{}
			appends := 0
			io := finalPublicationInventoryEventIO(thread, &events, &appends)
			audit := []domainevidence.PrivateAcceptedFinalRecord{held}
			plans, err := PreflightAcceptedFinalEventsWithPreservationV1(context.Background(), io, store, reader, records, nil, audit, observer)
			if err != nil || len(plans) != 1 {
				t.Fatalf("independent repair planning: %v", err)
			}
			load, loads := io.LoadEvents, 0
			failure := errors.New("synthetic original changed during last read")
			io.LoadEvents = func(ctx context.Context, store appturn.AcceptedFinalCompletionStore, id string) ([]map[string]any, error) {
				loaded, err := load(ctx, store, id)
				loads++
				if fault && loads == 3 {
					observer.err = failure
				}
				return loaded, err
			}
			err = ApplyAcceptedFinalEventsWithPreservationV1(context.Background(), io, store, reader, plans, records, nil, audit, observer)
			if fault {
				if !errors.Is(err, failure) || appends != 0 || len(events) != 0 {
					t.Fatalf("late original failure reached append: appends=%d events=%d err=%v", appends, len(events), err)
				}
			} else {
				if err != nil || appends != 1 || len(events) == 0 {
					t.Fatalf("independent repair failed: appends=%d err=%v", appends, err)
				}
				second, err := PreflightAcceptedFinalEventsWithPreservationV1(context.Background(), io, store, reader, records, nil, audit, observer)
				if err != nil {
					t.Fatal(err)
				}
				if err := ApplyAcceptedFinalEventsWithPreservationV1(context.Background(), io, store, reader, second, records, nil, audit, observer); err != nil || appends != 1 {
					t.Fatalf("second independent pass rewrote events: appends=%d err=%v", appends, err)
				}
			}
		})
	}
}

func (stub *finalEventPreservationStubV1) ObserveOriginalFinalEventInventoryV1(ctx context.Context) (PreservedFinalEventInventoryV1, error) {
	if err := ctx.Err(); err != nil {
		return PreservedFinalEventInventoryV1{}, err
	}
	return stub.input, stub.err
}

func (*finalEventPreservationStubV1) ValidateCaseCompactionAuthorityTurnV1(string, map[string]any, map[string]any) error {
	return errors.New("synthetic compaction authority unavailable")
}

func finalEventPreservationFixtureV1(t *testing.T, record domainevidence.PrivateAcceptedFinalRecord, thread map[string]any, events []map[string]any) *finalEventPreservationStubV1 {
	t.Helper()
	reader := acceptedFinalPublicReaderStub{threads: map[string]map[string]any{record.SecurityContext.ThreadID: thread}}
	cas, err := reader.ReadAcceptedFinalCASObservation(context.Background(), record.SecurityContext.ThreadID, record.SecurityContext.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	return &finalEventPreservationStubV1{input: PreservedFinalEventInventoryV1{
		Threads:    []PreservedFinalEventThreadV1{{Primary: recoveryport.PrimaryThreadSnapshotV1{ThreadID: record.SecurityContext.ThreadID, ThreadFileSHA256: cas.ThreadFileSHA256, Thread: thread}, Events: events, CAS: map[string]domainevidence.AcceptedFinalCASObservationV1{record.SecurityContext.TurnID: cas}}},
		Classified: FinalAuthorityInventory{AuditOnlyPublicWinners: []domainevidence.PrivateAcceptedFinalRecord{record}},
		Preserved:  []domainevidence.PrivateAcceptedFinalRecord{record},
	}}
}

func TestHeldHistoricalFinalNeedsOriginalInventoryOutsideExecutableReader(t *testing.T) {
	record, thread, publication, _, _ := historicalFactAuditFixture(t)
	events := acceptedFinalPublicationEventsForTest(publication, 1)
	appends := 0
	io := finalPublicationInventoryEventIO(thread, &events, &appends)
	executable := acceptedFinalPublicReaderStub{threads: map[string]map[string]any{}}
	observer := finalEventPreservationFixtureV1(t, record, thread, events)
	plans, err := PreflightAcceptedFinalEventsWithPreservationV1(context.Background(), io, &caseTerminalStoreStub{}, executable, nil, nil, []domainevidence.PrivateAcceptedFinalRecord{record}, observer)
	if err != nil || len(plans) != 0 || appends != 0 {
		t.Fatalf("held original winner blocked independent inventory or acquired a repair: plans=%d appends=%d err=%v", len(plans), appends, err)
	}
}

func TestPreservedFinalEventsValidateCompleteHistoryWithoutRepair(t *testing.T) {
	for _, scenario := range []string{"missing", "partial", "complete", "unknown_marker", "unmanifested_assistant", "unmanifested_terminal", "unknown_turn", "missing_turn_draft", "thread_private_reasoning", "numeric_marker", "orphan_event_marker", "foreign_thread", "duplicate_private", "missing_private", "duplicate_classification", "missing_classification", "lost_cas", "changed_cas", "scope_error", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			record, thread, publication, _, _ := historicalFactAuditFixture(t)
			events := acceptedFinalPublicationEventsForTest(publication, len(publication.Events))
			if scenario == "missing" {
				events = nil
			} else if scenario == "partial" {
				events = events[:1]
			}
			observer := finalEventPreservationFixtureV1(t, record, thread, events)
			var failure error
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch scenario {
			case "unknown_marker":
				events[0]["publicationCommitId"] = "unknown"
			case "unmanifested_assistant", "unmanifested_terminal", "unknown_turn":
				kind := "item_completed"
				if scenario == "unmanifested_terminal" {
					kind = "turn_completed"
				}
				extra := map[string]any{"kind": kind, "threadId": record.SecurityContext.ThreadID, "turnId": record.SecurityContext.TurnID, "seq": float64(len(events) + 1), "item": map[string]any{"kind": "assistant_text", "text": "synthetic unbound"}}
				if scenario == "unknown_turn" {
					extra["turnId"] = "unknown"
				}
				events = append(events, extra)
				observer.input.Threads[0].Events = events
			case "foreign_thread":
				events[0]["threadId"] = "foreign"
			case "missing_turn_draft", "thread_private_reasoning":
				extra := map[string]any{"kind": "assistant_text_delta", "threadId": record.SecurityContext.ThreadID, "seq": float64(len(events) + 1), "text": "synthetic unbound"}
				if scenario == "thread_private_reasoning" {
					extra["kind"] = "thread_updated"
					extra["reasoning_content"] = "synthetic private reasoning"
				}
				events = append(events, extra)
				observer.input.Threads[0].Events = events
			case "numeric_marker", "orphan_event_marker":
				extra := map[string]any{"kind": "pipeline_stage", "threadId": record.SecurityContext.ThreadID, "seq": float64(len(events) + 1)}
				if scenario == "numeric_marker" {
					extra["publicationCommitId"] = float64(123)
					extra["acceptedFinalDigest"] = float64(123)
				} else {
					extra["publicationEventId"] = "orphan"
				}
				events = append(events, extra)
				observer.input.Threads[0].Events = events
			case "duplicate_private":
				observer.input.Preserved = append(observer.input.Preserved, record)
			case "missing_private":
				observer.input.Preserved = nil
			case "duplicate_classification":
				observer.input.Classified.NotCommitted = []domainevidence.PrivateAcceptedFinalRecord{record}
			case "missing_classification":
				observer.input.Classified = FinalAuthorityInventory{}
			case "lost_cas":
				observer.input.Threads[0].CAS = nil
			case "changed_cas":
				cas := observer.input.Threads[0].CAS[record.SecurityContext.TurnID]
				cas.ThreadFileSHA256 = "changed"
				observer.input.Threads[0].CAS[record.SecurityContext.TurnID] = cas
			case "scope_error":
				failure = errors.New("synthetic original scope I/O")
				observer.err = failure
			case "cancelled":
				cancel()
				failure = context.Canceled
			}
			before := cloneEventMaps(events)
			appends := 0
			io := finalPublicationInventoryEventIO(thread, &events, &appends)
			executable := acceptedFinalPublicReaderStub{threads: map[string]map[string]any{}}
			audit := []domainevidence.PrivateAcceptedFinalRecord{record}
			plans, err := PreflightAcceptedFinalEventsWithPreservationV1(ctx, io, &caseTerminalStoreStub{}, executable, nil, nil, audit, observer)
			positive := scenario == "missing" || scenario == "partial" || scenario == "complete"
			if positive {
				if err != nil || len(plans) != 0 {
					t.Fatalf("original prefix was not retained: %v", err)
				}
				if err = ApplyAcceptedFinalEventsWithPreservationV1(ctx, io, &caseTerminalStoreStub{}, executable, plans, nil, nil, audit, observer); err != nil {
					t.Fatal(err)
				}
			} else if err == nil || failure != nil && !errors.Is(err, failure) {
				t.Fatalf("invalid original inventory accepted or cause lost: %v", err)
			}
			if appends != 0 || !reflect.DeepEqual(before, events) {
				t.Fatal("original held events were rewritten")
			}
		})
	}
}
