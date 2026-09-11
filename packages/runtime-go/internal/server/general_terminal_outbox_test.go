package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	controlapp "analytix.local/runtime-go/internal/app/control"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	usageapp "analytix.local/runtime-go/internal/app/usage"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestGeneralTerminalOutboxTreatsCommittedAtomicAppendErrorAsAckLoss(t *testing.T) {
	store, threadID, turnID, securityContext := newGeneralTerminalOutboxFixture(t)
	store.generalTerminalAtomicAppend = func(id string, events []map[string]any) error {
		if err := store.eventLog.AppendEvents(id, events, true); err != nil {
			return err
		}
		return &eventlog.AtomicAppendError{Committed: true, Cause: errors.New("simulated post-replace acknowledgement loss")}
	}
	result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
		Store: store, SecurityContext: securityContext, ThreadID: threadID, TurnID: turnID,
		Model: "deepseek", CreatedAt: "2026-07-15T00:00:00Z",
		FinishedAt: "2026-07-15T00:00:00Z", Telemetry: usageapp.NewTerminalTelemetryV1(domainmodel.Usage{
			PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15,
		}, nil),
	})
	if err != nil || !result.Changed {
		t.Fatalf("committed append acknowledgement loss was not recovered: result=%#v err=%v", result, err)
	}
	replay, err := store.LoadEventsSince(threadID, 0)
	if err != nil || len(replay.Events) != 3 {
		t.Fatalf("committed readback is incomplete: replay=%#v err=%v", replay, err)
	}
	assertGeneralTerminalEventIDsExactlyOnce(t, replay.Events)
	assertGeneralTerminalUsageIndexExactlyOnce(t, store, threadID, replay.Events)
}

func TestStartupGeneralTerminalRecoveryStrictlyReplaysEachAuthorityFreeThreadOnce(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	for range 64 {
		if _, err := store.CreateThread(map[string]any{"title": "legacy ordinary", "workspace": workspace}, workspace); err != nil {
			t.Fatal(err)
		}
	}
	store.eventReplayReadCount = 0
	if err := turnapp.RecoverGeneralTerminalPublicationsAtStartupV1(context.Background(), store); err != nil {
		t.Fatalf("startup authority-free inventory: %v", err)
	}
	if store.eventReplayReadCount != 64 {
		t.Fatalf("startup recovery performed %d strict event replays, want exactly 64", store.eventReplayReadCount)
	}
}

func TestStartupGeneralTerminalRecoveryPreservesHeldOutboxAndRecoversIndependent(t *testing.T) {
	for _, fault := range []string{"", "held_drift", "independent_corruption"} {
		t.Run("fault="+fault, func(t *testing.T) {
			ctx := context.Background()
			store, heldID, heldTurn, heldContext := newGeneralTerminalOutboxFixture(t)
			independentID, independentTurn, independentContext := addGeneralTerminalOutboxFixture(t, store, "independent")
			store.generalTerminalAtomicAppend = func(string, []map[string]any) error { return errors.New("synthetic outbox cut") }
			for _, frozen := range []domainsecurity.TurnSecurityContext{heldContext, independentContext} {
				result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{Store: store, SecurityContext: frozen, ThreadID: frozen.ThreadID, TurnID: frozen.TurnID, Model: "deepseek", CreatedAt: "2026-07-15T00:00:00Z", FinishedAt: "2026-07-15T00:00:00Z", Telemetry: usageapp.NewTerminalTelemetryV1(domainmodel.Usage{}, nil)})
				if err == nil || !result.Changed {
					t.Fatalf("outbox cut was not reached: changed=%t err=%v", result.Changed, err)
				}
			}
			preserved, err := eventlog.PrepareSemanticRestartPreservationV1(ctx, store.root, store.threadSummaryIndex.Path(), []string{heldID})
			if err != nil {
				t.Fatal(err)
			}
			reopened, err := newDurableEventSessionStoreWithPreservationV1(store.root, durableStoreModeTemp, false, preserved)
			if err != nil {
				t.Fatal(err)
			}
			if fault != "" {
				id := heldID
				if fault == "independent_corruption" {
					id = independentID
				}
				if err := os.WriteFile(reopened.eventsPath(id), []byte("{\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			before := runtimeRestoreFileDigestsV1(t, store.root)
			heldBefore := runtimeRestoreFileDigestsV1(t, store.threadDir(heldID))
			err = turnapp.RecoverGeneralTerminalPublicationsAtStartupV1(ctx, reopened)
			if fault != "" {
				if err == nil || !reflect.DeepEqual(before, runtimeRestoreFileDigestsV1(t, store.root)) {
					t.Fatalf("invalid startup inventory was admitted or changed state: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("held outbox blocked independent startup recovery: %v", err)
			}
			if !reflect.DeepEqual(heldBefore, runtimeRestoreFileDigestsV1(t, store.threadDir(heldID))) {
				t.Fatal("startup completed or rewrote held outbox")
			}
			if _, err := reopened.RecordGeneralTerminalEventBundle(heldID, heldTurn); err == nil {
				t.Fatal("startup released held outbox for explicit recovery")
			}
			replay, err := reopened.LoadEventsSince(independentID, 0)
			if err != nil || len(replay.Events) != 3 || stringField(replay.Events[0], "turnId") != independentTurn {
				t.Fatalf("independent startup did not restore exact outbox: events=%d err=%v", len(replay.Events), err)
			}
			assertGeneralTerminalEventIDsExactlyOnce(t, replay.Events)
			assertGeneralTerminalUsageIndexExactlyOnce(t, reopened, independentID, replay.Events)
			if _, err := os.Lstat(reopened.usageIndex.ThreadPath(heldID)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("held outbox acquired a usage projection: %v", err)
			}
			global, err := filestore.ReadJSONLFileRecords[usageIndexRecord](reopened.usageIndex.Path(), nil)
			if err != nil || len(global) != 1 || global[0].ThreadID != independentID {
				t.Fatalf("startup usage settlement was not limited to the independent outbox: count=%d err=%v", len(global), err)
			}
			secondScope, err := eventlog.PrepareSemanticRestartPreservationV1(ctx, store.root, store.threadSummaryIndex.Path(), []string{heldID})
			if err != nil {
				t.Fatal(err)
			}
			second, err := newDurableEventSessionStoreWithPreservationV1(store.root, durableStoreModeTemp, false, secondScope)
			if err != nil {
				t.Fatal(err)
			}
			settledBefore := runtimeRestoreFileDigestsV1(t, store.root)
			if err := turnapp.RecoverGeneralTerminalPublicationsAtStartupV1(ctx, second); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(settledBefore, runtimeRestoreFileDigestsV1(t, store.root)) || !reflect.DeepEqual(heldBefore, runtimeRestoreFileDigestsV1(t, store.threadDir(heldID))) {
				t.Fatal("second fresh startup rewrote held state or duplicated independent settlement")
			}
		})
	}
}

func TestStartupGeneralTerminalRecoveryNeverBypassesStrictEventValidation(t *testing.T) {
	for name, body := range map[string][]byte{
		"unicode escaped authority key":       []byte(`{"seq":1,"kind":"user_message","threadId":"THREAD_ID","text":"ordinary","generalTerminal\u0053lot":"usage"}` + "\n"),
		"canonical and escaped duplicate key": []byte(`{"seq":1,"kind":"user_message","threadId":"THREAD_ID","text":"ordinary","generalTerminalSlot":"usage","generalTerminal\u0053lot":"usage"}` + "\n"),
		"malformed json":                      []byte("{\n"),
		"duplicate sequence": []byte(
			`{"seq":1,"kind":"user_message","threadId":"THREAD_ID","text":"first"}` + "\n" +
				`{"seq":1,"kind":"user_message","threadId":"THREAD_ID","text":"second"}` + "\n",
		),
	} {
		t.Run(name, func(t *testing.T) {
			store, err := NewTempDurableEventSessionStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			thread, err := store.CreateThread(map[string]any{"title": "strict recovery"}, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			threadID := stringField(thread, "id")
			body = bytes.ReplaceAll(body, []byte("THREAD_ID"), []byte(threadID))
			if err := os.WriteFile(store.eventsPath(threadID), body, 0o600); err != nil {
				t.Fatal(err)
			}
			before := append([]byte(nil), body...)
			if err := turnapp.RecoverGeneralTerminalPublicationsAtStartupV1(context.Background(), store); err == nil {
				t.Fatal("invalid event stream passed startup recovery")
			}
			after, err := os.ReadFile(store.eventsPath(threadID))
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("rejected event stream was mutated: err=%v", err)
			}
		})
	}
}

func TestResolveCommittedGeneralTerminalRequiresExactCompleteBundle(t *testing.T) {
	store, threadID, turnID, securityContext := newGeneralTerminalOutboxFixture(t)
	result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
		Store: store, SecurityContext: securityContext, ThreadID: threadID, TurnID: turnID,
		Model: "deepseek", CreatedAt: "2026-07-15T00:00:00Z", FinishedAt: "2026-07-15T00:00:00Z",
		Telemetry: usageapp.NewTerminalTelemetryV1(domainmodel.Usage{}, nil),
	})
	if err != nil || !result.Changed {
		t.Fatalf("commit general child terminal: result=%#v err=%v", result, err)
	}
	primary, err := finalauthorityadapter.NewAcceptedFinalCASReader(store.root)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := turnapp.ResolveCommittedGeneralTerminalV1(context.Background(), store, primary, threadID, turnID)
	if err != nil || resolved.SecurityContext != securityContext || resolved.Commit.CommitDigest != result.Publication.CommitDigest ||
		resolved.Commit.TerminalStatus != "completed" || resolved.Commit.TerminalReason != "success" {
		t.Fatalf("resolve exact general terminal: resolution=%#v err=%v", resolved, err)
	}

	cutStore, cutThreadID, cutTurnID, cutContext := newGeneralTerminalOutboxFixture(t)
	cutStore.generalTerminalAtomicAppend = func(string, []map[string]any) error { return errors.New("injected append cut") }
	cutResult, cutErr := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
		Store: cutStore, SecurityContext: cutContext, ThreadID: cutThreadID, TurnID: cutTurnID,
		Model: "deepseek", CreatedAt: "2026-07-15T00:00:00Z", FinishedAt: "2026-07-15T00:00:00Z",
		Telemetry: usageapp.NewTerminalTelemetryV1(domainmodel.Usage{}, nil),
	})
	if cutErr == nil || !cutResult.Changed {
		t.Fatalf("expected terminal CAS/event cut: result=%#v err=%v", cutResult, cutErr)
	}
	cutPrimary, err := finalauthorityadapter.NewAcceptedFinalCASReader(cutStore.root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := turnapp.ResolveCommittedGeneralTerminalV1(context.Background(), cutStore, cutPrimary, cutThreadID, cutTurnID); err == nil {
		t.Fatal("incomplete general terminal event bundle was resolved")
	}
}

func TestGeneralTerminalOutboxRecoversCASAppendGapExactlyOnce(t *testing.T) {
	store, threadID, turnID, securityContext := newGeneralTerminalOutboxFixture(t)
	store.beforePersistEventHook = func() {
		if err := os.MkdirAll(store.eventsPath(threadID), 0o700); err != nil {
			t.Fatalf("install event append failure: %v", err)
		}
	}
	result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
		Store: store, SecurityContext: securityContext, ThreadID: threadID, TurnID: turnID,
		Model: "deepseek", CreatedAt: "2026-07-15T00:00:00Z",
		FinishedAt: "2026-07-15T00:00:00Z", Telemetry: usageapp.NewTerminalTelemetryV1(domainmodel.Usage{
			PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15,
		}, nil),
	})
	if err == nil || !result.Changed {
		t.Fatalf("expected CAS winner plus append failure: result=%#v err=%v", result, err)
	}
	raw := rawGeneralTerminalThreadForTest(t, store, threadID)
	turn := raw["turns"].([]any)[0].(map[string]any)
	if stringField(turn, "status") != "completed" || turn["generalTerminalPublication"] == nil ||
		turn["generalTerminalCASBinding"] == nil || len(listAny(turn["items"])) != 1 {
		t.Fatalf("terminal CAS did not preserve its recovery outbox: %#v", turn)
	}
	store.beforePersistEventHook = nil
	if err := os.Remove(store.eventsPath(threadID)); err != nil {
		t.Fatalf("restore event log path: %v", err)
	}
	live, unsubscribe := store.SubscribeEvents(threadID)
	defer unsubscribe()
	events, err := store.RecordGeneralTerminalEventBundle(threadID, turnID)
	if err != nil || len(events) != 3 {
		t.Fatalf("recover exact outbox: events=%#v err=%v", events, err)
	}
	for index, expected := range events {
		select {
		case observed := <-live:
			if !reflect.DeepEqual(observed, expected) {
				t.Fatalf("live event %d diverged from durable readback: observed=%#v expected=%#v", index, observed, expected)
			}
		case <-time.After(time.Second):
			t.Fatalf("recovered event %d was not published", index)
		}
	}
	beforeBytes, err := os.ReadFile(store.eventsPath(threadID))
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.RecordGeneralTerminalEventBundle(threadID, turnID)
	if err != nil || !reflect.DeepEqual(events, again) {
		t.Fatalf("idempotent reconcile diverged: first=%#v again=%#v err=%v", events, again, err)
	}
	select {
	case duplicate := <-live:
		t.Fatalf("same-process reconcile republished a duplicate: %#v", duplicate)
	case <-time.After(50 * time.Millisecond):
	}
	afterBytes, _ := os.ReadFile(store.eventsPath(threadID))
	if !reflect.DeepEqual(beforeBytes, afterBytes) {
		t.Fatal("idempotent reconcile rewrote the durable event log")
	}
	assertGeneralTerminalEventIDsExactlyOnce(t, events)
	assertGeneralTerminalUsageIndexExactlyOnce(t, store, threadID, events)

	reopened, err := NewTempDurableEventSessionStore(store.root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.RecordGeneralTerminalEventBundle(threadID, turnID); err != nil {
		t.Fatalf("restart reconcile failed: %v", err)
	}
	reopenedBytes, _ := os.ReadFile(reopened.eventsPath(threadID))
	if !reflect.DeepEqual(beforeBytes, reopenedBytes) {
		t.Fatal("restart reconcile duplicated or rewrote the exact bundle")
	}
	assertGeneralTerminalUsageIndexExactlyOnce(t, reopened, threadID, events)
}

func TestGeneralFailureRetryReconcilesCASAppendGap(t *testing.T) {
	store, threadID, turnID, securityContext := newGeneralTerminalOutboxFixture(t)
	injected := errors.New("injected general terminal append failure")
	store.generalTerminalAtomicAppend = func(string, []map[string]any) error { return injected }
	input := turnapp.PersistFailureInput{
		Store: store, SecurityContext: securityContext, TerminalReason: "cancel",
		ThreadID: threadID, TurnID: turnID, FinishedAt: "2026-07-15T00:00:00Z",
		Failure: domainfailure.New(domainfailure.CodeTurnCancelled, nil), Model: "deepseek",
		Interrupt: &turnapp.GeneralTerminalInterruptMetadata{Cancelled: true},
	}
	first, err := turnapp.CommitGeneralFailureTerminal(input)
	if !errors.Is(err, injected) || !first.Changed {
		t.Fatalf("first CAS/append cut = result:%#v err:%v", first, err)
	}
	store.generalTerminalAtomicAppend = nil
	second, err := turnapp.CommitGeneralFailureTerminal(input)
	if err != nil || second.Changed || second.Status != "aborted" {
		t.Fatalf("retry did not reconcile canonical winner = result:%#v err:%v", second, err)
	}
	replay, err := store.LoadEventsSince(threadID, 0)
	if err != nil || len(replay.Events) != 3 {
		t.Fatalf("reconciled bundle = %#v err=%v", replay.Events, err)
	}
	assertGeneralTerminalEventIDsExactlyOnce(t, replay.Events)
	assertGeneralTerminalUsageIndexExactlyOnce(t, store, threadID, replay.Events)
}

func TestInterruptRetryRestoresCanonicalMetadataAfterTerminalEventGap(t *testing.T) {
	store, threadID, turnID, securityContext := newGeneralTerminalOutboxFixture(t)
	injected := errors.New("injected interrupt terminal append failure")
	store.generalTerminalAtomicAppend = func(string, []map[string]any) error { return injected }
	first, err := controlapp.InterruptActiveTurn(controlapp.InterruptActiveTurnInput{
		Store: store, ThreadID: threadID, TurnID: turnID, Discard: true, Cancelled: true,
		FinishedAt: "2026-07-15T00:00:00Z", CaseContext: securityContext,
	})
	if !errors.Is(err, injected) || first.Changed {
		t.Fatalf("first interrupt should expose only the recoverable event gap: result=%#v err=%v", first, err)
	}
	raw := rawGeneralTerminalThreadForTest(t, store, threadID)
	turn := raw["turns"].([]any)[0].(map[string]any)
	if stringField(turn, "status") != "aborted" || turn["discard"] != true || turn["cancelled"] != true {
		t.Fatalf("first interrupt CAS did not retain canonical metadata: %#v", turn)
	}

	store.generalTerminalAtomicAppend = nil
	retry, err := controlapp.InterruptActiveTurn(controlapp.InterruptActiveTurnInput{
		Store: store, ThreadID: threadID, TurnID: turnID, Discard: false, Cancelled: false,
		FinishedAt: "2026-07-15T00:00:00Z", CaseContext: securityContext,
	})
	if err != nil || retry.Changed || retry.Status != "aborted" || !retry.Cancelled ||
		retry.Response["discard"] != true || retry.Response["cancelled"] != true {
		t.Fatalf("retry did not return the first canonical interrupt metadata: result=%#v err=%v", retry, err)
	}
	replay, err := store.LoadEventsSince(threadID, 0)
	if err != nil || len(replay.Events) != 3 {
		t.Fatalf("interrupt event-gap reconciliation mismatch: events=%#v err=%v", replay.Events, err)
	}
	assertGeneralTerminalEventIDsExactlyOnce(t, replay.Events)
}

func TestInterruptRetryRestoresCanonicalMetadataAfterTerminalCASAckGap(t *testing.T) {
	store, threadID, turnID, securityContext := newGeneralTerminalOutboxFixture(t)
	indexPath := store.threadSummaryIndex.Path()
	if err := os.Remove(indexPath); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.Mkdir(indexPath, 0o700); err != nil {
		t.Fatal(err)
	}
	first, err := controlapp.InterruptActiveTurn(controlapp.InterruptActiveTurnInput{
		Store: store, ThreadID: threadID, TurnID: turnID, Discard: true, Cancelled: true,
		FinishedAt: "2026-07-15T00:00:00Z", CaseContext: securityContext,
	})
	if err == nil || first.Response != nil {
		t.Fatalf("first interrupt should expose the post-CAS sidecar acknowledgement gap: result=%#v err=%v", first, err)
	}
	raw := rawGeneralTerminalThreadForTest(t, store, threadID)
	turn := raw["turns"].([]any)[0].(map[string]any)
	if stringField(turn, "status") != "aborted" || turn["discard"] != true || turn["cancelled"] != true {
		t.Fatalf("post-CAS acknowledgement gap lost canonical interrupt metadata: %#v", turn)
	}
	if err := os.Remove(indexPath); err != nil {
		t.Fatal(err)
	}

	retry, err := controlapp.InterruptActiveTurn(controlapp.InterruptActiveTurnInput{
		Store: store, ThreadID: threadID, TurnID: turnID, Discard: false, Cancelled: false,
		FinishedAt: "2026-07-15T00:00:00Z", CaseContext: securityContext,
	})
	if err != nil || retry.Changed || retry.Status != "aborted" || !retry.Cancelled ||
		retry.Response["discard"] != true || retry.Response["cancelled"] != true {
		t.Fatalf("CAS acknowledgement retry did not restore canonical metadata: result=%#v err=%v", retry, err)
	}
	replay, err := store.LoadEventsSince(threadID, 0)
	if err != nil || len(replay.Events) != 3 {
		t.Fatalf("CAS acknowledgement retry terminal bundle mismatch: events=%#v err=%v", replay.Events, err)
	}
	assertGeneralTerminalEventIDsExactlyOnce(t, replay.Events)
}

func TestEveryGeneralTerminalStatusRecoversCASAppendGapExactlyOnce(t *testing.T) {
	tests := []struct {
		name             string
		reason           string
		failure          domainfailure.Record
		expectedStatus   string
		expectedKind     string
		expectedItemKind string
		interrupt        *turnapp.GeneralTerminalInterruptMetadata
	}{
		{
			name: "source boundary", reason: "source_unavailable", failure: domainfailure.New("source_probe_unavailable", nil),
			expectedStatus: "completed", expectedKind: "turn_completed", expectedItemKind: "assistant_text",
		},
		{
			name: "provider failure", reason: "provider_failure", failure: domainfailure.New(domainfailure.CodeProviderError, nil),
			expectedStatus: "failed", expectedKind: "turn_failed", expectedItemKind: "error",
		},
		{
			name: "cancel", reason: "cancel", failure: domainfailure.New(domainfailure.CodeTurnCancelled, nil),
			expectedStatus: "aborted", expectedKind: "turn_aborted", expectedItemKind: "error",
			interrupt: &turnapp.GeneralTerminalInterruptMetadata{Discard: true, Cancelled: true, CancelledPendingGates: 2},
		},
		{
			name: "restart", reason: "restart", failure: domainfailure.New("runtime_restarted", nil),
			expectedStatus: "aborted", expectedKind: "turn_aborted", expectedItemKind: "error",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, threadID, turnID, securityContext := newGeneralTerminalOutboxFixture(t)
			store.beforePersistEventHook = func() {
				if err := os.MkdirAll(store.eventsPath(threadID), 0o700); err != nil {
					t.Fatalf("install event append failure: %v", err)
				}
			}
			result, err := turnapp.CommitGeneralFailureTerminal(turnapp.PersistFailureInput{
				Store: store, SecurityContext: securityContext, TerminalReason: test.reason,
				ThreadID: threadID, TurnID: turnID, FinishedAt: "2026-07-15T00:00:00Z",
				Failure: test.failure, Model: "deepseek", Interrupt: test.interrupt,
				Events: []map[string]any{{
					"kind": "assistant_text_delta", "threadId": threadID, "turnId": turnID,
					"delta": "PROVIDER_DRAFT_MUST_NOT_SURVIVE",
				}},
			})
			if err == nil || !result.Changed || result.Status != test.expectedStatus {
				t.Fatalf("expected recoverable CAS gap: result=%#v err=%v", result, err)
			}
			raw := rawGeneralTerminalThreadForTest(t, store, threadID)
			turn := raw["turns"].([]any)[0].(map[string]any)
			items := listAny(turn["items"])
			if stringField(turn, "status") != test.expectedStatus || turn["generalTerminalPublication"] == nil ||
				len(items) != 1 || stringField(items[0].(map[string]any), "kind") != test.expectedItemKind ||
				strings.Contains(string(mustJSONForGeneralTerminalTest(t, turn)), "PROVIDER_DRAFT_MUST_NOT_SURVIVE") {
				t.Fatalf("terminal CAS winner is not closed: %#v", turn)
			}
			store.beforePersistEventHook = nil
			if err := os.Remove(store.eventsPath(threadID)); err != nil {
				t.Fatalf("restore event log path: %v", err)
			}
			plans, err := store.PreflightGeneralTerminalPublicationRecoveryV1()
			if err != nil || len(plans) != 1 || len(plans[0].RepairTurns) != 1 ||
				plans[0].RepairTurns[0].TurnID != turnID {
				t.Fatalf("terminal crash gap was not preflighted: plans=%#v err=%v", plans, err)
			}
			if err := store.ApplyGeneralTerminalPublicationRecoveryV1(plans); err != nil {
				t.Fatalf("recover terminal bundle: %v", err)
			}
			replay, err := store.LoadEventsSince(threadID, 0)
			if err != nil || len(replay.Events) != 3 ||
				stringField(replay.Events[2], "kind") != test.expectedKind ||
				stringField(replay.Events[2], "terminalReason") != test.reason ||
				strings.Contains(string(mustJSONForGeneralTerminalTest(t, replay.Events)), "PROVIDER_DRAFT_MUST_NOT_SURVIVE") {
				t.Fatalf("recovered terminal bundle mismatch: replay=%#v err=%v", replay, err)
			}
			assertGeneralTerminalEventIDsExactlyOnce(t, replay.Events)
			assertGeneralTerminalUsageIndexExactlyOnce(t, store, threadID, replay.Events)
			before, _ := os.ReadFile(store.eventsPath(threadID))
			settledPlans, err := store.PreflightGeneralTerminalPublicationRecoveryV1()
			if err != nil || len(settledPlans) != 1 || len(settledPlans[0].RepairTurns) != 0 {
				t.Fatalf("settled bundle did not produce an exact recovery baseline: plans=%#v err=%v", settledPlans, err)
			}
			if err := store.ApplyGeneralTerminalPublicationRecoveryV1(settledPlans); err != nil {
				t.Fatalf("idempotent recovery failed: %v", err)
			}
			after, _ := os.ReadFile(store.eventsPath(threadID))
			if !reflect.DeepEqual(before, after) {
				t.Fatal("idempotent recovery duplicated or rewrote the terminal bundle")
			}
		})
	}
}

func mustJSONForGeneralTerminalTest(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestGeneralTerminalOutboxRejectsStaleEventFrontierAndRecovers(t *testing.T) {
	store, threadID, turnID, securityContext := newGeneralTerminalOutboxFixture(t)
	interloper := eventlog.NewStore(store.root)
	store.beforePersistEventHook = func() {
		if err := interloper.AppendEvent(threadID, map[string]any{
			"seq": float64(1), "kind": "tool_progress", "threadId": threadID, "turnId": turnID,
			"status": "running",
		}); err != nil {
			t.Fatalf("append interleaved event: %v", err)
		}
	}
	result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
		Store: store, SecurityContext: securityContext, ThreadID: threadID, TurnID: turnID,
		Model: "deepseek", CreatedAt: "2026-07-15T00:00:00Z",
		FinishedAt: "2026-07-15T00:00:00Z", Telemetry: usageapp.NewTerminalTelemetryV1(domainmodel.Usage{
			PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15,
		}, nil),
	})
	if !errors.Is(err, eventlog.ErrEventLogFrontierChanged) || !result.Changed {
		t.Fatalf("stale event frontier did not preserve only the CAS winner: result=%#v err=%v", result, err)
	}
	store.beforePersistEventHook = nil
	beforeRecovery, err := store.LoadEventsSince(threadID, 0)
	if err != nil || len(beforeRecovery.Events) != 1 || stringField(beforeRecovery.Events[0], "status") != "running" {
		t.Fatalf("stale bundle append changed the interleaved winner: replay=%#v err=%v", beforeRecovery, err)
	}
	plans, err := store.PreflightGeneralTerminalPublicationRecoveryV1()
	if err != nil || len(plans) != 1 || len(plans[0].RepairTurns) != 1 ||
		plans[0].RepairTurns[0].TurnID != turnID {
		t.Fatalf("stale frontier crash gap was not recoverable: plans=%#v err=%v", plans, err)
	}
	if err := store.ApplyGeneralTerminalPublicationRecoveryV1(plans); err != nil {
		t.Fatalf("recover after stale frontier rejection: %v", err)
	}
	replay, err := store.LoadEventsSince(threadID, 0)
	if err != nil || len(replay.Events) != 4 {
		t.Fatalf("recovered stale-frontier bundle is incomplete: replay=%#v err=%v", replay, err)
	}
	terminalEvents := make([]map[string]any, 0, 3)
	for _, event := range replay.Events {
		if stringField(event, "generalTerminalEventId") != "" {
			terminalEvents = append(terminalEvents, event)
		}
	}
	assertGeneralTerminalEventIDsExactlyOnce(t, terminalEvents)
	assertGeneralTerminalUsageIndexExactlyOnce(t, store, threadID, terminalEvents)
}

func TestGeneralTerminalStartupRecoveryRepairsCompleteBundleUsageIndexCrashCuts(t *testing.T) {
	for _, crashCut := range []string{"both-missing", "global-only-missing", "thread-only-missing"} {
		t.Run(crashCut, func(t *testing.T) {
			store, threadID, turnID, securityContext := newGeneralTerminalOutboxFixture(t)
			result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
				Store: store, SecurityContext: securityContext, ThreadID: threadID, TurnID: turnID,
				Model: "deepseek", CreatedAt: "2026-07-15T00:00:00Z",
				FinishedAt: "2026-07-15T00:00:00Z", Telemetry: usageapp.NewTerminalTelemetryV1(domainmodel.Usage{
					PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15,
				}, nil),
			})
			if err != nil || !result.Changed {
				t.Fatalf("commit exact terminal bundle: result=%#v err=%v", result, err)
			}
			eventBytes, err := os.ReadFile(store.eventsPath(threadID))
			if err != nil {
				t.Fatal(err)
			}
			switch crashCut {
			case "both-missing":
				if err := os.Remove(store.usageIndex.Path()); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(store.usageIndex.ThreadPath(threadID)); err != nil {
					t.Fatal(err)
				}
			case "global-only-missing":
				if err := os.Remove(store.usageIndex.Path()); err != nil {
					t.Fatal(err)
				}
			case "thread-only-missing":
				if err := os.Remove(store.usageIndex.ThreadPath(threadID)); err != nil {
					t.Fatal(err)
				}
			}

			reopened, err := NewTempDurableEventSessionStore(store.root)
			if err != nil {
				t.Fatal(err)
			}
			plans, err := reopened.PreflightGeneralTerminalPublicationRecoveryV1()
			if err != nil || len(plans) != 1 || len(plans[0].RepairTurns) != 0 {
				t.Fatalf("complete bundle was not retained as a derived-state recovery plan: plans=%#v err=%v", plans, err)
			}
			if err := reopened.ApplyGeneralTerminalPublicationRecoveryV1(plans); err != nil {
				t.Fatalf("repair complete bundle usage index: %v", err)
			}
			replay, err := reopened.LoadEventsSince(threadID, 0)
			if err != nil {
				t.Fatal(err)
			}
			assertGeneralTerminalUsageIndexExactlyOnce(t, reopened, threadID, replay.Events)
			if err := reopened.ApplyGeneralTerminalPublicationRecoveryV1(plans); err != nil {
				t.Fatalf("idempotent complete bundle settlement failed: %v", err)
			}
			assertGeneralTerminalUsageIndexExactlyOnce(t, reopened, threadID, replay.Events)
			afterEventBytes, err := os.ReadFile(reopened.eventsPath(threadID))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(eventBytes, afterEventBytes) {
				t.Fatal("usage index settlement rewrote the canonical event bundle")
			}
		})
	}
}

func TestGeneralTerminalStartupRecoveryRebuildsMatchingButWrongUsageIndex(t *testing.T) {
	store, threadID, turnID, securityContext := newGeneralTerminalOutboxFixture(t)
	if result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
		Store: store, SecurityContext: securityContext, ThreadID: threadID, TurnID: turnID,
		Model: "deepseek", CreatedAt: "2026-07-15T00:00:00Z",
		FinishedAt: "2026-07-15T00:00:00Z", Telemetry: usageapp.NewTerminalTelemetryV1(domainmodel.Usage{
			PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15,
		}, nil),
	}); err != nil || !result.Changed {
		t.Fatalf("commit exact terminal bundle: result=%#v err=%v", result, err)
	}
	replay, err := store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	usage := turnapp.GeneralTerminalUsageEventV1(replay.Events)
	seq, _ := numericSeq(usage["seq"])
	store.mu.Lock()
	global, globalErr := filestore.ReadJSONLFileRecords[usageapp.IndexRecord](store.usageIndex.Path(), nil)
	threadRecords, threadErr := filestore.ReadJSONLFileRecords[usageapp.IndexRecord](store.usageIndex.ThreadPath(threadID), nil)
	if globalErr == nil && threadErr == nil {
		for index := range global {
			if global[index].ThreadID == threadID && global[index].Seq == seq {
				global[index].Usage.TotalTokens += 999
			}
		}
		for index := range threadRecords {
			if threadRecords[index].Seq == seq {
				threadRecords[index].Usage.TotalTokens += 999
			}
		}
		globalErr = filestore.WriteJSONLFileAtomic(store.usageIndex.Path(), ".wrong-global-*.tmp", global)
		if globalErr == nil {
			threadErr = filestore.WriteJSONLFileAtomic(
				store.usageIndex.ThreadPath(threadID), ".wrong-thread-*.tmp", threadRecords,
			)
		}
	}
	store.mu.Unlock()
	if globalErr != nil || threadErr != nil {
		t.Fatal(errors.Join(globalErr, threadErr))
	}
	plans, err := store.PreflightGeneralTerminalPublicationRecoveryV1()
	if err != nil || len(plans) != 1 || len(plans[0].RepairTurns) != 0 {
		t.Fatalf("preflight complete bundle: plans=%#v err=%v", plans, err)
	}
	if err := store.ApplyGeneralTerminalPublicationRecoveryV1(plans); err != nil {
		t.Fatalf("rebuild matching but wrong usage indexes: %v", err)
	}
	assertGeneralTerminalUsageIndexExactlyOnce(t, store, threadID, replay.Events)
}

func TestGeneralTerminalOutboxRejectsPartialBundleWithoutRepair(t *testing.T) {
	store, threadID, turnID, securityContext := newGeneralTerminalOutboxFixture(t)
	store.beforePersistEventHook = func() { _ = os.MkdirAll(store.eventsPath(threadID), 0o700) }
	result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
		Store: store, SecurityContext: securityContext, ThreadID: threadID, TurnID: turnID,
		Model: "deepseek", CreatedAt: "2026-07-15T00:00:00Z",
		FinishedAt: "2026-07-15T00:00:00Z",
	})
	if err == nil || !result.Changed {
		t.Fatalf("expected append gap: result=%#v err=%v", result, err)
	}
	store.beforePersistEventHook = nil
	if err := os.Remove(store.eventsPath(threadID)); err != nil {
		t.Fatal(err)
	}
	thread := rawGeneralTerminalThreadForTest(t, store, threadID)
	entries, err := turnapp.PreflightGeneralTerminalPublicationInventoryV1(thread, nil)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := turnapp.GeneralTerminalPublicationEntryForTurnV1(entries, turnID)
	if !ok {
		t.Fatal("missing canonical outbox entry")
	}
	bundle, err := turnapp.PrepareGeneralTerminalEventBundleV1(thread, turnID, entry.Commit, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.eventLog.AppendEvent(threadID, bundle[0]); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(store.eventsPath(threadID))
	if _, err := store.RecordGeneralTerminalEventBundle(threadID, turnID); err == nil {
		t.Fatal("partial durable bundle was repaired instead of failing closed")
	}
	after, _ := os.ReadFile(store.eventsPath(threadID))
	if !reflect.DeepEqual(before, after) {
		t.Fatal("partial bundle failure mutated the event log")
	}
}

func TestGeneralTerminalCASCannotExtendEarlierMissingArchiveCommit(t *testing.T) {
	store, threadID, firstTurnID, firstContext := newGeneralTerminalOutboxFixture(t)
	store.beforePersistEventHook = func() { _ = os.MkdirAll(store.eventsPath(threadID), 0o700) }
	firstResult, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
		Store: store, SecurityContext: firstContext, ThreadID: threadID, TurnID: firstTurnID,
		Model: "deepseek", CreatedAt: "2026-07-15T00:00:00Z",
		FinishedAt: "2026-07-15T00:00:00Z",
	})
	if err == nil || !firstResult.Changed {
		t.Fatalf("expected first terminal crash gap: result=%#v err=%v", firstResult, err)
	}
	store.beforePersistEventHook = nil
	if err := os.Remove(store.eventsPath(threadID)); err != nil {
		t.Fatal(err)
	}
	secondContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: "turn_general_outbox_second", WorkspaceRealPath: firstContext.WorkspaceRealPath,
		SourceManifestHash: firstContext.SourceManifestHash, ContextEpoch: firstContext.ContextEpoch,
		IssuedAt: time.Unix(11, 0).UTC(),
	})
	secondRecord := turnsecurityapp.PublicRecord(secondContext)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": secondContext.TurnID, "threadId": threadID, "status": "running",
		"securityContext": secondRecord, "items": []any{},
	}, "deepseek", map[string]any{"securityState": secondRecord}); err != nil {
		t.Fatal(err)
	}
	secondResult, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
		Store: store, SecurityContext: secondContext, ThreadID: threadID, TurnID: secondContext.TurnID,
		Model: "deepseek", CreatedAt: "2026-07-15T00:00:01Z",
		FinishedAt: "2026-07-15T00:00:01Z",
	})
	if err == nil || secondResult.Changed {
		t.Fatalf("second terminal CAS extended an unsettled archive: result=%#v err=%v", secondResult, err)
	}
	thread := rawGeneralTerminalThreadForTest(t, store, threadID)
	entries, err := turnapp.PreflightGeneralTerminalPublicationInventoryV1(thread, nil)
	if err != nil || len(entries) != 1 || entries[0].TurnID != firstTurnID ||
		entries[0].State != turnapp.GeneralTerminalPublicationMissingV1 {
		t.Fatalf("rejected second CAS changed the original crash gap: entries=%#v err=%v", entries, err)
	}
}

func TestGeneralTerminalOutboxRecognizesExactBundleOutsideCurrentTail(t *testing.T) {
	store, threadID, turnID, securityContext := newGeneralTerminalOutboxFixture(t)
	if result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
		Store: store, SecurityContext: securityContext, ThreadID: threadID, TurnID: turnID,
		Model: "deepseek", CreatedAt: "2026-07-15T00:00:00Z",
		FinishedAt: "2026-07-15T00:00:00Z",
	}); err != nil || !result.Changed {
		t.Fatalf("commit exact bundle: result=%#v err=%v", result, err)
	}
	if _, _, err := store.RecordEvent(map[string]any{
		"kind": "pipeline_stage", "threadId": threadID, "turnId": turnID,
		"jobId": "job-late", "stage": "background_job_completed", "timestamp": "2026-07-15T00:00:01Z",
	}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(store.eventsPath(threadID))
	reopened, err := NewTempDurableEventSessionStore(store.root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.RecordGeneralTerminalEventBundle(threadID, turnID); err != nil {
		t.Fatalf("non-tail exact bundle was rejected: %v", err)
	}
	after, _ := os.ReadFile(reopened.eventsPath(threadID))
	if !reflect.DeepEqual(before, after) {
		t.Fatal("non-tail reconcile duplicated the bundle")
	}
}

func TestGenericEventPathsRejectGeneralTerminalMarkersAndAssistantText(t *testing.T) {
	store, threadID, turnID, securityContext := newGeneralTerminalOutboxFixture(t)
	if result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
		Store: store, SecurityContext: securityContext, ThreadID: threadID, TurnID: turnID,
		Model: "deepseek", CreatedAt: "2026-07-15T00:00:00Z",
		FinishedAt: "2026-07-15T00:00:00Z",
	}); err != nil || !result.Changed {
		t.Fatalf("commit exact bundle: result=%#v err=%v", result, err)
	}
	before, _ := os.ReadFile(store.eventsPath(threadID))
	replay, err := store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range replay.Events {
		kind := stringField(event, "kind")
		item, _ := event["item"].(map[string]any)
		if kind == "item_completed" && stringField(item, "kind") == "assistant_text" {
			candidate := cloneMap(event)
			delete(candidate, "seq")
			if _, _, err := store.RecordEvent(candidate); !errors.Is(err, domainevent.ErrAssistantDraftPersistence) {
				t.Fatalf("exact canonical assistant event bypassed the dedicated bundle API: %v", err)
			}
		}
		if kind == "usage" || kind == "turn_completed" {
			candidate := cloneMap(event)
			delete(candidate, "seq")
			for field := range candidate {
				if len(field) >= len("generalTerminal") && field[:len("generalTerminal")] == "generalTerminal" {
					delete(candidate, field)
				}
			}
			if _, _, err := store.RecordEvent(candidate); !errors.Is(err, domainevent.ErrAssistantDraftPersistence) {
				t.Fatalf("unmarked duplicate %s bypassed the canonical outbox: %v", kind, err)
			}
		}
	}
	for _, field := range []string{
		"generalTerminalCommitId", "generalTerminalEventId", "generalTerminalSlot",
		"generalTerminalPayloadDigest", "generalTerminalAuthorityKind", "generalTerminalAuthorityDigest",
		"acceptedFinal", "acceptedFinalView", "acceptedFinalDigest", "publicationCommitId", "publicationEventId",
		"publicationSlot", "publicationPayloadDigest",
	} {
		draft := map[string]any{
			"kind": "pipeline_stage", "threadId": threadID, "turnId": turnID, "stage": "response_received", field: "forged",
		}
		if _, _, err := store.RecordEvent(draft); !errors.Is(err, domainevent.ErrAssistantDraftPersistence) {
			t.Fatalf("generic event accepted marker %s: %v", field, err)
		}
	}
	nested := map[string]any{
		"kind": "pipeline_stage", "threadId": threadID, "turnId": turnID, "stage": "response_received",
		"details": map[string]any{"publicationCommitId": "forged"},
	}
	if _, _, err := store.RecordEvent(nested); !errors.Is(err, domainevent.ErrAssistantDraftPersistence) {
		t.Fatalf("nested marker bypassed generic sink: %v", err)
	}
	rawCandidate := map[string]any{
		"kind": "pipeline_stage", "threadId": threadID, "turnId": turnID, "stage": "response_received", "seq": float64(999),
		"generalTerminalCommitId": "forged",
	}
	rawBody, _ := json.Marshal(rawCandidate)
	if err := store.AppendRawEventLine(threadID, string(rawBody)); !errors.Is(err, domainevent.ErrAssistantDraftPersistence) {
		t.Fatalf("raw JSON marker bypassed generic sink: %v", err)
	}
	after, _ := os.ReadFile(store.eventsPath(threadID))
	if !reflect.DeepEqual(before, after) {
		t.Fatal("rejected generic publication bypass mutated the event log")
	}
}

func TestGeneralTerminalRecoveryRejectsBaselineDriftBeforeAppend(t *testing.T) {
	store, threadID, turnID, securityContext := newGeneralTerminalOutboxFixture(t)
	store.beforePersistEventHook = func() { _ = os.MkdirAll(store.eventsPath(threadID), 0o700) }
	result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
		Store: store, SecurityContext: securityContext, ThreadID: threadID, TurnID: turnID,
		Model: "deepseek", CreatedAt: "2026-07-15T00:00:00Z",
		FinishedAt: "2026-07-15T00:00:00Z",
	})
	if err == nil || !result.Changed {
		t.Fatalf("expected append gap: result=%#v err=%v", result, err)
	}
	store.beforePersistEventHook = nil
	if err := os.Remove(store.eventsPath(threadID)); err != nil {
		t.Fatal(err)
	}
	plans, err := store.PreflightGeneralTerminalPublicationRecoveryV1()
	if err != nil || len(plans) != 1 {
		t.Fatalf("preflight missing outbox: plans=%#v err=%v", plans, err)
	}
	if err := store.ApplyGeneralTerminalPublicationRecoveryV1(nil); err == nil {
		t.Fatal("truncated recovery inventory bypassed the all-thread baseline")
	}
	if _, _, err := store.RecordEvent(map[string]any{
		"kind": "tool_result_upload_wait", "threadId": threadID, "turnId": turnID,
		"status": "waiting", "timestamp": "2026-07-15T00:00:01Z",
	}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(store.eventsPath(threadID))
	if err := store.ApplyGeneralTerminalPublicationRecoveryV1(plans); err == nil {
		t.Fatal("changed event frontier was applied from a stale recovery plan")
	}
	after, _ := os.ReadFile(store.eventsPath(threadID))
	if !reflect.DeepEqual(before, after) {
		t.Fatal("stale recovery plan mutated the event log before rejecting drift")
	}
}

func TestGeneralTerminalRecoveryRejectsThreadAddedAfterPreflight(t *testing.T) {
	store, threadID, turnID, securityContext := newGeneralTerminalOutboxFixture(t)
	store.beforePersistEventHook = func() { _ = os.MkdirAll(store.eventsPath(threadID), 0o700) }
	result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
		Store: store, SecurityContext: securityContext, ThreadID: threadID, TurnID: turnID,
		Model: "deepseek", CreatedAt: "2026-07-15T00:00:00Z",
		FinishedAt: "2026-07-15T00:00:00Z",
	})
	if err == nil || !result.Changed {
		t.Fatalf("expected append gap: result=%#v err=%v", result, err)
	}
	store.beforePersistEventHook = nil
	if err := os.Remove(store.eventsPath(threadID)); err != nil {
		t.Fatal(err)
	}
	plans, err := store.PreflightGeneralTerminalPublicationRecoveryV1()
	if err != nil || len(plans) != 1 {
		t.Fatalf("preflight missing outbox: plans=%#v err=%v", plans, err)
	}
	if _, err := store.CreateThread(map[string]any{"title": "added after preflight"}, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyGeneralTerminalPublicationRecoveryV1(plans); err == nil {
		t.Fatal("new thread after preflight did not invalidate the all-thread baseline")
	}
	if _, err := os.Stat(store.eventsPath(threadID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale recovery inventory mutated the missing bundle: %v", err)
	}
}

func TestGeneralTerminalRecoveryPreflightsAllThreadsBeforeRepair(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	firstThread, firstTurn, firstContext := addGeneralTerminalOutboxFixture(t, store, "first")
	secondThread, secondTurn, secondContext := addGeneralTerminalOutboxFixture(t, store, "second")
	for _, fixture := range []struct {
		threadID string
		turnID   string
		context  domainsecurity.TurnSecurityContext
	}{
		{firstThread, firstTurn, firstContext},
		{secondThread, secondTurn, secondContext},
	} {
		store.beforePersistEventHook = func() { _ = os.MkdirAll(store.eventsPath(fixture.threadID), 0o700) }
		result, commitErr := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
			Store: store, SecurityContext: fixture.context, ThreadID: fixture.threadID, TurnID: fixture.turnID,
			Model: "deepseek", CreatedAt: "2026-07-15T00:00:00Z",
			FinishedAt: "2026-07-15T00:00:00Z",
		})
		if commitErr == nil || !result.Changed {
			t.Fatalf("expected append gap for %s: result=%#v err=%v", fixture.threadID, result, commitErr)
		}
		store.beforePersistEventHook = nil
		if err := os.Remove(store.eventsPath(fixture.threadID)); err != nil {
			t.Fatal(err)
		}
	}
	secondCanonical := rawGeneralTerminalThreadForTest(t, store, secondThread)
	entries, err := turnapp.PreflightGeneralTerminalPublicationInventoryV1(secondCanonical, nil)
	if err != nil {
		t.Fatal(err)
	}
	secondEntry, ok := turnapp.GeneralTerminalPublicationEntryForTurnV1(entries, secondTurn)
	if !ok {
		t.Fatal("second canonical outbox missing")
	}
	partial, err := turnapp.PrepareGeneralTerminalEventBundleV1(secondCanonical, secondTurn, secondEntry.Commit, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.eventLog.AppendEvent(secondThread, partial[0]); err != nil {
		t.Fatal(err)
	}
	if plans, err := store.PreflightGeneralTerminalPublicationRecoveryV1(); err == nil || plans != nil {
		t.Fatalf("partial second thread passed all-inventory preflight: plans=%#v err=%v", plans, err)
	}
	if _, err := os.Stat(store.eventsPath(firstThread)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preflight repaired the first thread before discovering the second corruption: %v", err)
	}
}

func TestGeneralTerminalRecoveryRejectsDuplicateRootArchiveKeyBeforeMutation(t *testing.T) {
	store, threadID, turnID, securityContext := newGeneralTerminalOutboxFixture(t)
	if result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
		Store: store, SecurityContext: securityContext, ThreadID: threadID, TurnID: turnID,
		Model: "deepseek", CreatedAt: "2026-07-15T00:00:00Z",
		FinishedAt: "2026-07-15T00:00:00Z",
	}); err != nil || !result.Changed {
		t.Fatalf("commit exact terminal bundle: result=%#v err=%v", result, err)
	}
	threadPath := store.threadPath(threadID)
	canonical, err := os.ReadFile(threadPath)
	if err != nil {
		t.Fatal(err)
	}
	needle := []byte(`"generalTerminalPublicationArchive":`)
	if bytes.Count(canonical, needle) != 1 {
		t.Fatalf("canonical archive field count is not one: %d", bytes.Count(canonical, needle))
	}
	ambiguous := bytes.Replace(canonical, needle, []byte(
		`"generalTerminalPublicationArchive":{"forged":true},"generalTerminalPublicationArchive":`,
	), 1)
	if err := os.WriteFile(threadPath, ambiguous, 0o600); err != nil {
		t.Fatal(err)
	}
	eventBytes, err := os.ReadFile(store.eventsPath(threadID))
	if err != nil {
		t.Fatal(err)
	}
	plans, err := store.PreflightGeneralTerminalPublicationRecoveryV1()
	if err == nil || plans != nil || !strings.Contains(err.Error(), "duplicate JSON object key") {
		t.Fatalf("duplicate root archive key was not rejected: plans=%#v err=%v", plans, err)
	}
	afterThread, _ := os.ReadFile(threadPath)
	afterEvents, _ := os.ReadFile(store.eventsPath(threadID))
	if !bytes.Equal(ambiguous, afterThread) || !bytes.Equal(eventBytes, afterEvents) {
		t.Fatal("duplicate archive preflight mutated canonical persistence")
	}
}

func TestGeneralTerminalRecoveryRejectsDuplicateMarkerKeyBeforeMutation(t *testing.T) {
	store, threadID, turnID, securityContext := newGeneralTerminalOutboxFixture(t)
	if result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
		Store: store, SecurityContext: securityContext, ThreadID: threadID, TurnID: turnID,
		Model: "deepseek", CreatedAt: "2026-07-15T00:00:00Z",
		FinishedAt: "2026-07-15T00:00:00Z",
	}); err != nil || !result.Changed {
		t.Fatalf("commit exact terminal bundle: result=%#v err=%v", result, err)
	}
	replay, err := store.LoadEventsSince(threadID, 0)
	if err != nil || len(replay.Events) == 0 {
		t.Fatalf("load canonical terminal bundle: replay=%#v err=%v", replay, err)
	}
	commitID := stringField(replay.Events[0], "generalTerminalCommitId")
	path := store.eventsPath(threadID)
	canonical, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	needle := []byte(`"generalTerminalCommitId":"` + commitID + `"`)
	if bytes.Count(canonical, needle) == 0 {
		t.Fatal("canonical terminal marker was not found")
	}
	ambiguous := bytes.Replace(canonical, needle, []byte(
		`"generalTerminalCommitId":"forged","generalTerminalCommitId":"`+commitID+`"`,
	), 1)
	if err := os.WriteFile(path, ambiguous, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadEventsSince(threadID, 0)
	if err != nil || len(loaded.Events) != 2 || len(loaded.Diagnostics) != 1 {
		t.Fatalf("duplicate marker line did not return sanitized internal replay diagnostics: loaded=%#v err=%v", loaded, err)
	}
	diagnostic := loaded.Diagnostics[0]
	if diagnostic.Path != filepath.ToSlash(filepath.Join("threads", threadID, "events.jsonl")) ||
		diagnostic.Line != 1 || diagnostic.Error != "invalid_json" || diagnostic.Preview != "" {
		t.Fatalf("duplicate marker diagnostic exposed or omitted metadata: %#v", diagnostic)
	}
	plans, err := store.PreflightGeneralTerminalPublicationRecoveryV1()
	if err == nil || plans != nil {
		t.Fatalf("duplicate marker inventory was not rejected: plans=%#v err=%v", plans, err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(ambiguous, after) {
		t.Fatal("duplicate marker preflight mutated the event log")
	}
}

func newGeneralTerminalOutboxFixture(
	t *testing.T,
) (*DurableEventSessionStore, string, string, domainsecurity.TurnSecurityContext) {
	t.Helper()
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	threadID, turnID, securityContext := addGeneralTerminalOutboxFixture(t, store, "default")
	return store, threadID, turnID, securityContext
}

func addGeneralTerminalOutboxFixture(
	t *testing.T,
	store *DurableEventSessionStore,
	suffix string,
) (string, string, domainsecurity.TurnSecurityContext) {
	t.Helper()
	workspace := t.TempDir()
	thread, err := store.CreateThread(map[string]any{"title": "General outbox", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_general_outbox_" + suffix
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1,
		IssuedAt: time.Unix(10, 0).UTC(),
	})
	contextRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "securityContext": contextRecord, "items": []any{},
	}, "deepseek", map[string]any{"securityState": contextRecord}); err != nil {
		t.Fatal(err)
	}
	return threadID, turnID, securityContext
}

func rawGeneralTerminalThreadForTest(t *testing.T, store *DurableEventSessionStore, threadID string) map[string]any {
	t.Helper()
	store.mu.Lock()
	defer store.mu.Unlock()
	thread, err := store.readThreadNoLock(threadID)
	if err != nil {
		t.Fatal(err)
	}
	return thread
}

func assertGeneralTerminalEventIDsExactlyOnce(t *testing.T, events []map[string]any) {
	t.Helper()
	seen := map[string]int{}
	for _, event := range events {
		seen[stringField(event, "generalTerminalEventId")]++
	}
	if len(seen) != len(events) {
		t.Fatalf("event IDs are not unique: %#v", seen)
	}
	for eventID, count := range seen {
		if eventID == "" || count != 1 {
			t.Fatalf("event ID %q count=%d", eventID, count)
		}
	}
}

func assertGeneralTerminalUsageIndexExactlyOnce(
	t *testing.T,
	store *DurableEventSessionStore,
	threadID string,
	events []map[string]any,
) {
	t.Helper()
	usage := turnapp.GeneralTerminalUsageEventV1(events)
	if stringField(usage, "threadId") != threadID {
		t.Fatalf("terminal usage event belongs to another thread: %#v", usage)
	}
	if err := store.usageIndex.VerifyTerminalEvent(usage); err != nil {
		t.Fatalf("usage index is not exactly once: %v", err)
	}
}
