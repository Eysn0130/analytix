package turn

import (
	"context"
	"errors"
	"fmt"
	"testing"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

type generalTerminalRecoveryStoreStub struct {
	tx             *generalTerminalRecoveryTransactionStub
	exclusiveCalls int
	exclusive      bool
}

func (store *generalTerminalRecoveryStoreStub) WithGeneralTerminalRecoveryExclusiveV1(
	ctx context.Context,
	run func(recoveryport.TransactionV1) error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if store.exclusive || run == nil || store.tx == nil {
		return errors.New("invalid exclusive recovery transaction")
	}
	store.exclusive = true
	store.exclusiveCalls++
	store.tx.owner = store
	defer func() { store.exclusive = false }()
	return run(store.tx)
}

type generalTerminalRecoveryTransactionStub struct {
	owner       *generalTerminalRecoveryStoreStub
	threadIDs   []string
	threads     map[string]map[string]any
	replays     map[string]recoveryport.ReplaySnapshotV1
	loadErrors  map[string]error
	loadCalls   int
	recordHook  func(string, string) error
	recordCalls int
	settlements int
	batchCalls  int
}

func (tx *generalTerminalRecoveryTransactionStub) requireExclusive() error {
	if tx == nil || tx.owner == nil || !tx.owner.exclusive {
		return errors.New("recovery operation escaped the exclusive transaction")
	}
	return nil
}

func (tx *generalTerminalRecoveryTransactionStub) ThreadIDs() ([]string, error) {
	return append([]string(nil), tx.threadIDs...), tx.requireExclusive()
}

func (tx *generalTerminalRecoveryTransactionStub) ReadCanonicalThread(threadID string) (map[string]any, error) {
	if err := tx.requireExclusive(); err != nil {
		return nil, err
	}
	return contracts.CloneMap(tx.threads[threadID]), nil
}

func (tx *generalTerminalRecoveryTransactionStub) PublicationThreadView(_ string, thread map[string]any) (map[string]any, error) {
	if err := tx.requireExclusive(); err != nil {
		return nil, err
	}
	return contracts.CloneMap(thread), nil
}

func (tx *generalTerminalRecoveryTransactionStub) LoadEvents(threadID string) (recoveryport.ReplaySnapshotV1, error) {
	if err := tx.requireExclusive(); err != nil {
		return recoveryport.ReplaySnapshotV1{}, err
	}
	tx.loadCalls++
	return tx.replays[threadID], tx.loadErrors[threadID]
}

func (tx *generalTerminalRecoveryTransactionStub) ObserveEvents(ctx context.Context, threadID string) (recoveryport.ReplaySnapshotV1, error) {
	if err := ctx.Err(); err != nil {
		return recoveryport.ReplaySnapshotV1{}, err
	}
	return tx.LoadEvents(threadID)
}

func (tx *generalTerminalRecoveryTransactionStub) RecordTerminalBundle(threadID, turnID string) error {
	if err := tx.requireExclusive(); err != nil {
		return err
	}
	tx.recordCalls++
	if tx.recordHook != nil {
		return tx.recordHook(threadID, turnID)
	}
	return nil
}

func (tx *generalTerminalRecoveryTransactionStub) SettleTerminalUsage(map[string]any) error {
	if err := tx.requireExclusive(); err != nil {
		return err
	}
	tx.settlements++
	return nil
}

func (tx *generalTerminalRecoveryTransactionStub) SettleTerminalUsageBatch(events []map[string]any) error {
	if err := tx.requireExclusive(); err != nil {
		return err
	}
	tx.batchCalls++
	tx.settlements += len(events)
	return nil
}

func TestGeneralTerminalRecoveryPreflightAndApplyUseOneExclusiveBaseline(t *testing.T) {
	tx := &generalTerminalRecoveryTransactionStub{
		threadIDs: []string{"thread-a"},
		threads: map[string]map[string]any{
			"thread-a": {"id": "thread-a", "turns": []any{}},
		},
		replays: map[string]recoveryport.ReplaySnapshotV1{
			"thread-a": {Events: []map[string]any{}, Replayable: true},
		},
	}
	store := &generalTerminalRecoveryStoreStub{tx: tx}
	plans, err := PreflightGeneralTerminalPublicationRecoveryV1(context.Background(), store)
	if err != nil || len(plans) != 1 || plans[0].ThreadID != "thread-a" || plans[0].RepairTurns == nil || len(plans[0].RepairTurns) != 0 ||
		!domainsecurity.IsSHA256Hex(plans[0].ThreadDigest) || !domainsecurity.IsSHA256Hex(plans[0].EventsDigest) {
		t.Fatalf("preflight plan = %#v err=%v", plans, err)
	}
	if err := ApplyGeneralTerminalPublicationRecoveryV1(context.Background(), store, plans); err != nil {
		t.Fatal(err)
	}
	if store.exclusiveCalls != 2 || store.exclusive || tx.recordCalls != 0 || tx.settlements != 0 {
		t.Fatalf("recovery transaction lifecycle = calls:%d held:%t records:%d settlements:%d", store.exclusiveCalls, store.exclusive, tx.recordCalls, tx.settlements)
	}

	tx.threads["thread-a"]["title"] = "baseline drift"
	if err := ApplyGeneralTerminalPublicationRecoveryV1(context.Background(), store, plans); err == nil {
		t.Fatal("changed canonical thread baseline was accepted")
	}
	if tx.recordCalls != 0 {
		t.Fatal("baseline drift caused a partial recovery write")
	}
}

func TestStartupGeneralTerminalRecoveryStrictlyLoadsEachThreadOnce(t *testing.T) {
	const threadCount = 963
	tx := &generalTerminalRecoveryTransactionStub{
		threadIDs: make([]string, 0, threadCount),
		threads:   make(map[string]map[string]any, threadCount),
		replays:   make(map[string]recoveryport.ReplaySnapshotV1, threadCount),
	}
	for index := range threadCount {
		threadID := fmt.Sprintf("thread-%04d", index)
		tx.threadIDs = append(tx.threadIDs, threadID)
		tx.threads[threadID] = map[string]any{"id": threadID, "turns": []any{}}
		tx.replays[threadID] = recoveryport.ReplaySnapshotV1{Events: []map[string]any{}, Replayable: true}
	}
	store := &generalTerminalRecoveryStoreStub{tx: tx}
	if err := RecoverGeneralTerminalPublicationsAtStartupV1(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	if store.exclusiveCalls != 1 || tx.loadCalls != threadCount || tx.recordCalls != 0 || tx.settlements != 0 {
		t.Fatalf(
			"startup recovery = exclusive:%d loads:%d records:%d settlements:%d",
			store.exclusiveCalls, tx.loadCalls, tx.recordCalls, tx.settlements,
		)
	}
}

func TestStartupGeneralTerminalRecoveryValidatesAllThreadsBeforeMutation(t *testing.T) {
	tx := &generalTerminalRecoveryTransactionStub{
		threadIDs: []string{"thread-a", "thread-b", "thread-c"},
		threads: map[string]map[string]any{
			"thread-a": {"id": "thread-a", "turns": []any{}},
			"thread-b": {"id": "thread-b", "turns": []any{}},
			"thread-c": {"id": "thread-c", "turns": []any{}},
		},
		replays: map[string]recoveryport.ReplaySnapshotV1{
			"thread-a": {Events: []map[string]any{}, Replayable: true},
			"thread-b": {Events: []map[string]any{}, Replayable: true},
			"thread-c": {Events: []map[string]any{}, Replayable: false},
		},
	}
	store := &generalTerminalRecoveryStoreStub{tx: tx}
	if err := RecoverGeneralTerminalPublicationsAtStartupV1(context.Background(), store); err == nil {
		t.Fatal("unreplayable final thread was accepted")
	}
	if tx.loadCalls != 3 || tx.recordCalls != 0 || tx.settlements != 0 {
		t.Fatalf("failed full preflight mutated state: loads=%d records=%d settlements=%d", tx.loadCalls, tx.recordCalls, tx.settlements)
	}
}

func TestStartupGeneralTerminalRecoverySettlesCompleteUsageWithoutReload(t *testing.T) {
	thread, commit := committedGeneralTerminalPublicationFixture(t, GeneralProviderFinalQuarantinedText)
	archive, err := NextGeneralTerminalPublicationArchiveV1(
		thread, domainturnterminal.GeneralTerminalPublicationCommitV1Map(commit),
	)
	if err != nil {
		t.Fatal(err)
	}
	thread[GeneralTerminalPublicationArchiveFieldV1] = archive
	events, err := PrepareGeneralTerminalEventBundleV1(thread, commit.TurnID, commit, 1)
	if err != nil {
		t.Fatal(err)
	}
	tx := &generalTerminalRecoveryTransactionStub{
		threadIDs: []string{commit.ThreadID},
		threads:   map[string]map[string]any{commit.ThreadID: thread},
		replays: map[string]recoveryport.ReplaySnapshotV1{
			commit.ThreadID: {Events: events, Replayable: true},
		},
	}
	store := &generalTerminalRecoveryStoreStub{tx: tx}
	if err := RecoverGeneralTerminalPublicationsAtStartupV1(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	if tx.loadCalls != 1 || tx.recordCalls != 0 || tx.settlements != 1 || tx.batchCalls != 1 {
		t.Fatalf("complete startup inventory = loads:%d records:%d settlements:%d batches:%d", tx.loadCalls, tx.recordCalls, tx.settlements, tx.batchCalls)
	}
}

func TestStartupGeneralTerminalRecoveryReadsBackOnlyRepairedThread(t *testing.T) {
	thread, commit := committedGeneralTerminalPublicationFixture(t, GeneralProviderFinalQuarantinedText)
	archive, err := NextGeneralTerminalPublicationArchiveV1(
		thread, domainturnterminal.GeneralTerminalPublicationCommitV1Map(commit),
	)
	if err != nil {
		t.Fatal(err)
	}
	thread[GeneralTerminalPublicationArchiveFieldV1] = archive
	events, err := PrepareGeneralTerminalEventBundleV1(thread, commit.TurnID, commit, 1)
	if err != nil {
		t.Fatal(err)
	}
	tx := &generalTerminalRecoveryTransactionStub{
		threadIDs: []string{commit.ThreadID},
		threads:   map[string]map[string]any{commit.ThreadID: thread},
		replays: map[string]recoveryport.ReplaySnapshotV1{
			commit.ThreadID: {Events: []map[string]any{}, Replayable: true},
		},
	}
	tx.recordHook = func(threadID, turnID string) error {
		if threadID != commit.ThreadID || turnID != commit.TurnID {
			return errors.New("unexpected repair identity")
		}
		tx.replays[threadID] = recoveryport.ReplaySnapshotV1{Events: events, Replayable: true}
		return nil
	}
	store := &generalTerminalRecoveryStoreStub{tx: tx}
	if err := RecoverGeneralTerminalPublicationsAtStartupV1(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	if tx.loadCalls != 2 || tx.recordCalls != 1 || tx.settlements != 1 || tx.batchCalls != 1 {
		t.Fatalf("repaired startup inventory = loads:%d records:%d settlements:%d batches:%d", tx.loadCalls, tx.recordCalls, tx.settlements, tx.batchCalls)
	}
}

func TestGeneralTerminalRecoveryRejectsInvalidAuthorityAndInventory(t *testing.T) {
	if plans, err := PreflightGeneralTerminalPublicationRecoveryV1(context.Background(), nil); err == nil || plans != nil {
		t.Fatalf("nil recovery store was accepted: plans=%#v err=%v", plans, err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	store := &generalTerminalRecoveryStoreStub{tx: &generalTerminalRecoveryTransactionStub{}}
	if _, err := PreflightGeneralTerminalPublicationRecoveryV1(cancelled, store); !errors.Is(err, context.Canceled) || store.exclusiveCalls != 0 {
		t.Fatalf("cancelled preflight entered the store: calls=%d err=%v", store.exclusiveCalls, err)
	}

	store.tx.threadIDs = []string{"thread-b", "thread-a"}
	if _, err := PreflightGeneralTerminalPublicationRecoveryV1(context.Background(), store); err == nil {
		t.Fatal("noncanonical recovery inventory was accepted")
	}
	store.tx.threadIDs = []string{"thread-a"}
	invalid := []GeneralTerminalPublicationRecoveryPlanV1{{
		ThreadID: "thread-a", ThreadDigest: domainsecurity.SHA256Hex([]byte("thread")),
		EventsDigest: domainsecurity.SHA256Hex([]byte("events")), RepairTurns: nil,
	}}
	if err := ApplyGeneralTerminalPublicationRecoveryV1(context.Background(), store, invalid); err == nil {
		t.Fatal("recovery plan with a nil repair inventory was accepted")
	}
}

func TestGeneralTerminalUsageEventSelectsOnlyExactUsageSlot(t *testing.T) {
	events := []map[string]any{
		{"kind": "usage", "generalTerminalSlot": "terminal"},
		{"kind": "turn_completed", "generalTerminalSlot": "usage"},
		{"kind": "usage", "generalTerminalSlot": "usage", "seq": 3},
	}
	if event := GeneralTerminalUsageEventV1(events); event == nil || event["seq"] != 3 {
		t.Fatalf("exact terminal usage event was not selected: %#v", event)
	}
}
