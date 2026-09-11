package turnterminal

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	appturn "analytix.local/runtime-go/internal/app/turn"
	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	turnterminalstoreport "analytix.local/runtime-go/internal/ports/turnterminalstore"
	terminaltest "analytix.local/runtime-go/internal/testsupport/turnterminal"
)

type coordinatorTraceV1 struct {
	mu      sync.Mutex
	entries []string
}

func (trace *coordinatorTraceV1) add(value string) {
	trace.mu.Lock()
	trace.entries = append(trace.entries, value)
	trace.mu.Unlock()
}

func (trace *coordinatorTraceV1) snapshot() []string {
	trace.mu.Lock()
	defer trace.mu.Unlock()
	return append([]string(nil), trace.entries...)
}

type memoryTerminalStoreV1 struct {
	trace       *coordinatorTraceV1
	intent      *domainturnterminal.TurnTerminalIntentV1
	disposition *domainturnterminal.TurnTerminalDispositionV1
}

func (store *memoryTerminalStoreV1) PutIntentIfAbsent(_ context.Context, intent domainturnterminal.TurnTerminalIntentV1) error {
	store.trace.add("intent_put")
	if store.intent != nil && *store.intent != intent {
		return errors.New("intent conflict")
	}
	copy := intent
	store.intent = &copy
	return nil
}
func (store *memoryTerminalStoreV1) ReadIntent(_ context.Context, _ string) (domainturnterminal.TurnTerminalIntentV1, error) {
	store.trace.add("intent_read")
	if store.intent == nil {
		return domainturnterminal.TurnTerminalIntentV1{}, turnterminalstoreport.ErrNotFound
	}
	return *store.intent, nil
}
func (store *memoryTerminalStoreV1) VisitIntents(_ context.Context, visit func(domainturnterminal.TurnTerminalIntentV1) error) error {
	if store.intent != nil {
		return visit(*store.intent)
	}
	return nil
}
func (store *memoryTerminalStoreV1) PutDispositionIfAbsent(_ context.Context, disposition domainturnterminal.TurnTerminalDispositionV1) error {
	store.trace.add("terminal_disposition_put")
	if store.disposition != nil && *store.disposition != disposition {
		return errors.New("disposition conflict")
	}
	copy := disposition
	store.disposition = &copy
	return nil
}
func (store *memoryTerminalStoreV1) ReadDisposition(_ context.Context, _ string) (domainturnterminal.TurnTerminalDispositionV1, error) {
	store.trace.add("terminal_disposition_read")
	if store.disposition == nil {
		return domainturnterminal.TurnTerminalDispositionV1{}, turnterminalstoreport.ErrNotFound
	}
	return *store.disposition, nil
}
func (store *memoryTerminalStoreV1) VisitDispositions(_ context.Context, visit func(domainturnterminal.TurnTerminalDispositionV1) error) error {
	if store.disposition != nil {
		return visit(*store.disposition)
	}
	return nil
}
func (store *memoryTerminalStoreV1) HasRecords(context.Context) (bool, error) {
	return store.intent != nil || store.disposition != nil, nil
}

type memoryPrivateFinalStoreV1 struct {
	trace       *coordinatorTraceV1
	private     domainevidence.PrivateAcceptedFinalRecord
	disposition *domainevidence.AcceptedFinalDispositionRecord
}

func (store *memoryPrivateFinalStoreV1) PutIfAbsent(_ context.Context, record domainevidence.PrivateAcceptedFinalRecord) error {
	store.private = record
	return nil
}
func (store *memoryPrivateFinalStoreV1) Resolve(_ context.Context, digest string) (domainevidence.PrivateAcceptedFinalRecord, error) {
	store.trace.add("private_final_read")
	if store.private.AcceptedFinal.RecordDigest != digest {
		return domainevidence.PrivateAcceptedFinalRecord{}, errors.New("private final is missing")
	}
	return store.private, nil
}
func (store *memoryPrivateFinalStoreV1) List(context.Context) ([]domainevidence.PrivateAcceptedFinalRecord, error) {
	return []domainevidence.PrivateAcceptedFinalRecord{store.private}, nil
}
func (store *memoryPrivateFinalStoreV1) HasRecords(context.Context) (bool, error) { return true, nil }
func (store *memoryPrivateFinalStoreV1) PutDispositionIfAbsent(_ context.Context, disposition domainevidence.AcceptedFinalDispositionRecord) error {
	store.trace.add("accepted_disposition_put")
	if store.disposition != nil && *store.disposition != disposition {
		return errors.New("accepted disposition conflict")
	}
	copy := disposition
	store.disposition = &copy
	return nil
}
func (store *memoryPrivateFinalStoreV1) ResolveDisposition(_ context.Context, digest string) (domainevidence.AcceptedFinalDispositionRecord, error) {
	store.trace.add("accepted_disposition_read")
	if store.disposition == nil || store.disposition.AcceptedFinalDigest != digest {
		return domainevidence.AcceptedFinalDispositionRecord{}, errors.New("accepted disposition is missing")
	}
	return *store.disposition, nil
}
func (store *memoryPrivateFinalStoreV1) ListDispositions(context.Context) ([]domainevidence.AcceptedFinalDispositionRecord, error) {
	if store.disposition == nil {
		return nil, nil
	}
	return []domainevidence.AcceptedFinalDispositionRecord{*store.disposition}, nil
}

type memoryCompletionCASV1 struct {
	trace       *coordinatorTraceV1
	context     domainsecurity.TurnSecurityContext
	current     domainsecurity.TurnSecurityContext
	winner      domainevidence.AcceptedFinalRecord
	publicCalls int
	publicErr   error
}

func (store *memoryCompletionCASV1) FinishTurnIfActiveWithItemsAndFields(_, _ string, status string,
	_ []map[string]any, fields map[string]any) (bool, string, error) {
	store.trace.add("public_cas")
	store.publicCalls++
	if store.publicErr != nil {
		return false, "in_progress", store.publicErr
	}
	if store.winner.RecordDigest != "" {
		return false, status, nil
	}
	winner, err := domainevidence.ParseAcceptedFinalRecord(fields["acceptedFinal"])
	if err != nil {
		return false, "in_progress", err
	}
	store.winner = winner
	return true, status, nil
}
func (store *memoryCompletionCASV1) FinishTurnIfActiveWithAcceptedFinalAuthority(
	threadID, turnID, status string,
	items []map[string]any,
	fields map[string]any,
	privateFinal domainevidence.PrivateAcceptedFinalRecord,
	factAuthority appturn.FactFinalMutationAuthority,
) (bool, string, error) {
	if domainevidence.ValidatePrivateAcceptedFinalPublicationAuthority(privateFinal) != nil {
		return false, "in_progress", errors.New("test private final is invalid")
	}
	if domainevidence.FinalAnswerRequiresPublicationSnapshotProof(privateFinal.Envelope) {
		if factAuthority == nil || factAuthority.UseExact(privateFinal, func() error { return nil }) != nil {
			return false, "in_progress", errors.New("test fact authority is invalid")
		}
	} else if factAuthority != nil {
		return false, "in_progress", errors.New("test boundary final carried fact authority")
	}
	return store.FinishTurnIfActiveWithItemsAndFields(threadID, turnID, status, items, fields)
}
func (store *memoryCompletionCASV1) RecordEvent(event map[string]any) (map[string]any, []string, error) {
	return event, nil, nil
}
func (store *memoryCompletionCASV1) ReadAcceptedFinalCASObservation(_ context.Context, _, _ string) (domainevidence.AcceptedFinalCASObservationV1, error) {
	store.trace.add("cas_observe")
	status := "running"
	if store.winner.RecordDigest != "" {
		status = "completed"
	}
	current := store.current
	if current.ContextDigest == "" {
		current = store.context
	}
	observation := domainevidence.AcceptedFinalCASObservationV1{
		ThreadID: store.context.ThreadID, TurnID: store.context.TurnID, Status: status,
		FrozenContext: store.context, CurrentContext: current,
		ThreadFileSHA256:     domainsecurity.SHA256Hex([]byte("thread:" + store.winner.RecordDigest)),
		TurnProjectionSHA256: domainsecurity.SHA256Hex([]byte("turn:" + store.winner.RecordDigest)),
	}
	if store.winner.RecordDigest != "" {
		observation.HasWinner = true
		observation.Winner = store.winner
	}
	return domainevidence.NewAcceptedFinalCASObservationV1(observation)
}

type memoryProviderCloserV1 struct {
	trace    *coordinatorTraceV1
	closure  domaincachetelemetry.ProviderTurnClosureV1
	err      error
	calls    int
	reason   domaincachetelemetry.ProviderTurnTerminalReasonV1
	closedAt time.Time
}

func (closer *memoryProviderCloserV1) CloseTurn(_ context.Context, _ domainsecurity.TurnSecurityContext,
	reason domaincachetelemetry.ProviderTurnTerminalReasonV1, closedAt time.Time) (domaincachetelemetry.ProviderTurnClosureV1, error) {
	closer.trace.add("provider_close")
	closer.calls++
	closer.reason = reason
	closer.closedAt = closedAt
	return closer.closure, closer.err
}

func (closer *memoryProviderCloserV1) ObserveTurnClosureV1(_ context.Context, _ domainsecurity.TurnSecurityContext) (domaincachetelemetry.ProviderTurnClosureV1, bool, error) {
	if closer.err != nil {
		return domaincachetelemetry.ProviderTurnClosureV1{}, false, closer.err
	}
	if closer.calls == 0 {
		return domaincachetelemetry.ProviderTurnClosureV1{}, false, nil
	}
	return closer.closure, true, nil
}

func (closer *memoryProviderCloserV1) VisitTurnClosuresV1(_ context.Context, visit func(domaincachetelemetry.ProviderTurnClosureV1) error) error {
	if closer.err != nil {
		return closer.err
	}
	if closer.calls == 0 {
		return nil
	}
	return visit(closer.closure)
}

func TestCoordinatorCommitsIntentClosurePublicCASAndDispositionInOrder(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	trace := &coordinatorTraceV1{}
	terminals := &memoryTerminalStoreV1{trace: trace}
	privateFinals := &memoryPrivateFinalStoreV1{trace: trace, private: fixture.PrivateFinal}
	completion := &memoryCompletionCASV1{trace: trace, context: fixture.Context}
	closer := &memoryProviderCloserV1{trace: trace, closure: fixture.Closure}
	coordinator, err := NewCoordinator(newTrustedInventoryAuthorityV1(fixture.PrivateKey), privateFinals, terminals, closer)
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.CommitV1(context.Background(), CommitInputV1{
		CompletionStore: completion, CASReader: completion, PrivateFinal: fixture.PrivateFinal,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Intent.AcceptedFinalDigest != fixture.PrivateFinal.AcceptedFinal.RecordDigest ||
		result.ProviderClosure.ClosureID != fixture.Closure.ClosureID ||
		result.AcceptedFinalDisposition.State != domainevidence.AcceptedFinalCommitted ||
		result.TerminalDisposition.AcceptedFinalDispositionDigest != result.AcceptedFinalDisposition.RecordDigest {
		t.Fatalf("terminal transaction result mismatch: %#v", result)
	}
	acceptedAt, _ := time.Parse(time.RFC3339Nano, fixture.PrivateFinal.AcceptedFinal.AcceptedAt)
	if closer.reason != fixture.Intent.TerminalReasonCode || !closer.closedAt.Equal(acceptedAt) {
		t.Fatalf("provider closure binding mismatch: reason=%s time=%s", closer.reason, closer.closedAt)
	}
	assertTraceOrderV1(t, trace.snapshot(), "intent_put", "provider_close", "public_cas", "accepted_disposition_put", "terminal_disposition_put")
	if completion.publicCalls != 1 || closer.calls != 1 {
		t.Fatalf("terminal transaction effect counts: public=%d closure=%d", completion.publicCalls, closer.calls)
	}
	if _, err := coordinator.CommitV1(context.Background(), CommitInputV1{
		CompletionStore: completion, CASReader: completion, PrivateFinal: fixture.PrivateFinal,
	}); err != nil {
		t.Fatalf("exact terminal transaction replay failed: %v", err)
	}
	if terminals.intent == nil || terminals.disposition == nil || privateFinals.disposition == nil {
		t.Fatal("terminal transaction replay lost authority")
	}
}

func TestCoordinatorProviderClosureFailurePreventsPublicCAS(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	trace := &coordinatorTraceV1{}
	terminals := &memoryTerminalStoreV1{trace: trace}
	privateFinals := &memoryPrivateFinalStoreV1{trace: trace, private: fixture.PrivateFinal}
	completion := &memoryCompletionCASV1{trace: trace, context: fixture.Context}
	closer := &memoryProviderCloserV1{trace: trace, err: errors.New("closure unavailable")}
	coordinator, err := NewCoordinator(newTrustedInventoryAuthorityV1(fixture.PrivateKey), privateFinals, terminals, closer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.CommitV1(context.Background(), CommitInputV1{
		CompletionStore: completion, CASReader: completion, PrivateFinal: fixture.PrivateFinal,
	}); err == nil {
		t.Fatal("provider closure failure allowed terminal commit")
	}
	if completion.publicCalls != 0 || terminals.intent == nil || terminals.disposition != nil || privateFinals.disposition != nil {
		t.Fatalf("closure failure effects: public=%d intent=%v terminal=%v accepted=%v",
			completion.publicCalls, terminals.intent != nil, terminals.disposition != nil, privateFinals.disposition != nil)
	}
}

func TestCoordinatorDifferentExistingWinnerPreventsIntentAndClosure(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	trace := &coordinatorTraceV1{}
	terminals := &memoryTerminalStoreV1{trace: trace}
	privateFinals := &memoryPrivateFinalStoreV1{trace: trace, private: fixture.PrivateFinal}
	completion := &memoryCompletionCASV1{trace: trace, context: fixture.Context, winner: fixture.PrivateFinal.AcceptedFinal}
	other, err := domainevidence.NewAcceptedFinalRecord(domainevidence.AcceptedFinalRecordInput{
		Context: fixture.Context, Envelope: fixture.PrivateFinal.Envelope, RenderedText: fixture.PrivateFinal.RenderedText,
		RegistryHead:        fixture.PrivateFinal.RegistryHead,
		PrivateRecordDigest: domainsecurity.SHA256Hex([]byte("other-private-final")),
		AcceptedAt:          terminaltest.FixtureTimeV1().Add(time.Hour),
		AuthorityKeyID:      domainsecurity.SHA256Hex(fixture.PublicKey), AuthorityPublicKey: fixture.PublicKey,
	}, fixture.Sign)
	if err != nil {
		t.Fatal(err)
	}
	completion.winner = other
	closer := &memoryProviderCloserV1{trace: trace, closure: fixture.Closure}
	coordinator, err := NewCoordinator(newTrustedInventoryAuthorityV1(fixture.PrivateKey), privateFinals, terminals, closer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.CommitV1(context.Background(), CommitInputV1{
		CompletionStore: completion, CASReader: completion, PrivateFinal: fixture.PrivateFinal,
	}); err == nil {
		t.Fatal("different existing public winner allowed terminal intent")
	}
	if terminals.intent != nil || closer.calls != 0 || completion.publicCalls != 0 {
		t.Fatalf("different winner effects: intent=%v closure=%d public=%d", terminals.intent != nil, closer.calls, completion.publicCalls)
	}
}

func TestCoordinatorRejectsPublicWinnerWithoutPriorTerminalIntent(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	trace := &coordinatorTraceV1{}
	terminals := &memoryTerminalStoreV1{trace: trace}
	privateFinals := &memoryPrivateFinalStoreV1{trace: trace, private: fixture.PrivateFinal}
	completion := &memoryCompletionCASV1{trace: trace, context: fixture.Context, winner: fixture.PrivateFinal.AcceptedFinal}
	closer := &memoryProviderCloserV1{trace: trace, closure: fixture.Closure}
	coordinator, err := NewCoordinator(newTrustedInventoryAuthorityV1(fixture.PrivateKey), privateFinals, terminals, closer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.CommitV1(context.Background(), CommitInputV1{
		CompletionStore: completion, CASReader: completion, PrivateFinal: fixture.PrivateFinal,
	}); err == nil {
		t.Fatal("legacy public winner was backfilled with a synthetic terminal chain")
	}
	if terminals.intent != nil || closer.calls != 0 || completion.publicCalls != 0 {
		t.Fatalf("legacy public winner was mutated: intent=%v closure=%d public=%d", terminals.intent != nil, closer.calls, completion.publicCalls)
	}
}

func assertTraceOrderV1(t *testing.T, trace []string, ordered ...string) {
	t.Helper()
	position := -1
	for _, expected := range ordered {
		found := -1
		for index := position + 1; index < len(trace); index++ {
			if trace[index] == expected {
				found = index
				break
			}
		}
		if found < 0 {
			t.Fatalf("trace lacks ordered step %q after %d: %v", expected, position, trace)
		}
		position = found
	}
}

var _ appturn.AcceptedFinalCompletionStore = (*memoryCompletionCASV1)(nil)
