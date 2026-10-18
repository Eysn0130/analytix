package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	appmodel "analytix.local/runtime-go/internal/app/model"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	appusage "analytix.local/runtime-go/internal/app/usage"
	contracts "analytix.local/runtime-go/internal/contracts"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

func seedOrdinarySummaryTurnsForTest(t *testing.T, f inlineCompletionFixture) domainsecurity.TurnSecurityContext {
	t.Helper()
	return seedOrdinarySummaryTurnsRangeForTest(t, f, 0, 4)
}

func seedOrdinarySummaryTurnsRangeForTest(t *testing.T, f inlineCompletionFixture, first, end int) domainsecurity.TurnSecurityContext {
	t.Helper()
	h := f.handler
	h.providerUsage = f.service.closer.(appusage.ProviderUsageObserverV1)
	h.threads = nil
	var current domainsecurity.TurnSecurityContext
	for index := first; index < end; index++ {
		thread, err := h.store.GetThread(f.request.ThreadID)
		if err != nil {
			t.Fatal(err)
		}
		at := time.Now().UTC()
		current, err = turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{Context: context.Background(), Authority: h.turnSecurity,
			Thread: thread, ThreadID: f.request.ThreadID, TurnID: fmt.Sprintf("turn_summary_%d", index), Workspace: f.workspace,
			Principal: testIdentityPrincipal(), IssuedAt: at})
		if err != nil {
			t.Fatal(err)
		}
		epoch, found, err := contextepochapp.StateFromThread(thread)
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			epoch, err = contextepochapp.BootstrapState(f.request.ThreadID, current.ContextEpoch, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(current)}, at)
			if err != nil {
				t.Fatal(err)
			}
		}
		stamp := at.Format(time.RFC3339Nano)
		if err := h.store.AppendTurnToThread(f.request.ThreadID, map[string]any{"id": current.TurnID, "threadId": f.request.ThreadID,
			"status": "running", "prompt": "Continue this task.", "createdAt": stamp, "securityContext": turnsecurityapp.PublicRecord(current),
			"items": []any{map[string]any{"id": fmt.Sprintf("item_summary_%d", index), "threadId": f.request.ThreadID, "turnId": current.TurnID,
				"kind": "user_message", "role": "user", "status": "completed", "createdAt": stamp,
				"text": fmt.Sprintf("SOURCE_%d ", index) + strings.Repeat("Continue the safe task using the recorded decisions. ", 200)}}},
			"openai", map[string]any{"securityState": turnsecurityapp.PublicRecord(current), "contextEpochState": contextepochapp.PublicState(epoch)}); err != nil {
			t.Fatal(err)
		}
		if result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{Store: h.store, SecurityContext: current,
			ThreadID: f.request.ThreadID, TurnID: current.TurnID, Model: "fixture-model", CreatedAt: stamp, FinishedAt: stamp}); err != nil || !result.Changed {
			t.Fatalf("seed terminal: result=%#v err=%v", result, err)
		}
	}
	return current
}

func TestOrdinarySummaryProductionProviderAndRestartClosure(t *testing.T) {
	t.Setenv("ANALYTIX_THREAD_TRACE", "1")
	var requestBody map[string]any
	f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"The user wants the safe task continued using recorded decisions.\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	})
	current := seedOrdinarySummaryTurnsForTest(t, f)
	// Simulate interruption after canonical CAS but before completion append.
	f.handler.store.beforeRecordEventHook = func(event map[string]any) error {
		if stringField(event, "kind") == "compaction_completed" {
			return errors.New("synthetic completion append failure")
		}
		return nil
	}
	if _, err := f.handler.runtimeThreadService().Compact(context.Background(), current.ThreadID, "manual"); err == nil {
		t.Fatal("completion append fault was ignored")
	}
	if f.calls.Load() != 1 {
		t.Fatalf("physical Provider count=%d", f.calls.Load())
	}
	body, _ := json.Marshal(requestBody)
	if !strings.Contains(string(body), "SOURCE_0") || !strings.Contains(string(body), "SOURCE_1") || strings.Contains(string(body), "SOURCE_2") || strings.Contains(string(body), "SOURCE_3") || requestBody["tools"] != nil {
		t.Fatalf("summary did not use the selected closed history")
	}
	thread, err := f.handler.store.GetThread(current.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	turn := listAny(thread["turns"])[0].(map[string]any)
	item := listAny(turn["items"])[0].(map[string]any)
	// Each completed source turn owns its user item and real terminal item.
	if !domainevent.ValidGeneralCompactionProviderHistoryItemV5(item) || item["summaryKind"] != "conversation" || item["shadowedItemCount"] != float64(4) {
		t.Fatalf("real summary marker invalid: %#v", item)
	}
	f.handler.store.beforeRecordEventHook = nil
	reopened, err := newDurableEventSessionStore(f.durableRoot, f.handler.store.mode, false)
	if err != nil {
		t.Fatal(err)
	}
	reopened.SetCaseThreadAuthority(f.handler.caseThreads)
	if err := reopened.RecoverOrdinaryCompactions(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := reopened.RecoverOrdinaryCompactions(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.calls.Load() != 1 {
		t.Fatal("recovery reissued Provider")
	}
	replay, err := reopened.LoadEventsSince(current.ThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	completed := 0
	for _, event := range replay.Events {
		if stringField(event, "kind") == "compaction_completed" {
			completed++
			projected, visible, err := threadapp.ProjectPublicThreadEvent(current.ThreadID, thread, event)
			if err != nil || !visible || projected["summary"] != item["summary"] {
				t.Fatalf("recovered summary event unavailable: visible=%t err=%v", visible, err)
			}
		}
	}
	if completed != 1 {
		t.Fatalf("recovery closure count=%d", completed)
	}
	// Exercise the production HTTP/SSE boundary against the recovered store,
	// not only the ordinary public-event projection above.
	f.handler.store = reopened
	wire := ordinaryCompactionTraceReplay(t, f, current.ThreadID)
	if len(wire["compaction_started"]) != 1 || len(wire["compaction_completed"]) != 1 || len(wire["compaction_failed"]) != 0 {
		t.Fatal("trace-enabled replay lost the actual compaction lifecycle")
	}
	completion := wire["compaction_completed"][0]
	if completion["itemId"] != item["id"] || completion["turnId"] != turn["id"] || completion["summary"] != item["summary"] ||
		completion["checkpointDigest"] != item["checkpointDigest"] {
		t.Fatal("trace-enabled replay changed the committed summary identity")
	}
}

func TestOrdinarySummaryProductionCappedResponsePreservesPrimary(t *testing.T) {
	t.Setenv("ANALYTIX_THREAD_TRACE", "1")
	f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Incomplete summary\"},\"finish_reason\":\"length\"}]}\n\ndata: [DONE]\n\n")
	})
	current := seedOrdinarySummaryTurnsForTest(t, f)
	before, _ := f.handler.store.GetThread(current.ThreadID)
	digest, _ := threadapp.CompactionBaselineDigest(before)
	if response, err := f.handler.runtimeThreadService().Compact(context.Background(), current.ThreadID, "manual"); err == nil || response != nil {
		t.Fatalf("capped summary accepted: %#v err=%v", response, err)
	}
	after, _ := f.handler.store.GetThread(current.ThreadID)
	afterDigest, _ := threadapp.CompactionBaselineDigest(after)
	if digest != afterDigest || f.calls.Load() != 1 {
		t.Fatalf("capped summary changed history or retried: equal=%t calls=%d", digest == afterDigest, f.calls.Load())
	}
	wire := ordinaryCompactionTraceReplay(t, f, current.ThreadID)
	if len(wire["compaction_started"]) != 1 || len(wire["compaction_failed"]) != 1 || len(wire["compaction_completed"]) != 0 ||
		wire["compaction_failed"][0]["reasonCode"] != "summary_unavailable" {
		t.Fatal("trace-enabled replay changed or dropped the actual failed summary")
	}
}

// These are actual persisted ordinary-compaction events produced above. The
// desktop contract is closed: transport tracing cannot append top-level fields.
func ordinaryCompactionTraceReplay(t *testing.T, f inlineCompletionFixture, threadID string) map[string][]map[string]any {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/v1/threads/"+threadID+"/events?since_seq=0", nil)
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("trace-enabled compaction SSE status=%d", recorder.Code)
	}
	result := map[string][]map[string]any{}
	for _, line := range strings.Split(recorder.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatal(err)
		}
		kind := stringField(event, "kind")
		if kind != "compaction_started" && kind != "compaction_completed" && kind != "compaction_failed" {
			continue
		}
		if event["schemaVersion"] != float64(3) {
			t.Fatal("ordinary compaction changed wire version")
		}
		if _, found := event["trace"]; found {
			t.Fatal("SSE trace widened a closed ordinary-compaction event: " + kind)
		}
		result[kind] = append(result[kind], event)
	}
	return result
}

func TestOrdinarySummaryInterruptedStartRemainsUnknownWithoutProvider(t *testing.T) {
	h, threadID, _ := newUnboundCompactionFixture(t)
	before, _ := h.store.GetThread(threadID)
	digest, _ := threadapp.CompactionBaselineDigest(before)
	_, _, err := h.store.RecordEvent(map[string]any{"kind": "compaction_started", "schemaVersion": float64(3), "threadId": threadID,
		"turnId": "turn_compaction_interrupted", "itemId": "compaction_interrupted", "compactionId": "compaction_interrupted", "auto": false, "startedAt": time.Now().UTC().Format(time.RFC3339Nano)})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.store.RecoverOrdinaryCompactions(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, _ := h.store.GetThread(threadID)
	afterDigest, _ := threadapp.CompactionBaselineDigest(after)
	if digest != afterDigest {
		t.Fatal("interrupted recovery replaced history")
	}
	replay, _ := h.store.LoadEventsSince(threadID, 0)
	last := replay.Events[len(replay.Events)-1]
	if stringField(last, "kind") != "compaction_started" || last["compactionId"] != "compaction_interrupted" {
		t.Fatalf("missing evidence fabricated a terminal result: %#v", last)
	}
}

func TestOrdinarySummaryCaseThreadMakesZeroProviderCalls(t *testing.T) {
	f := newInlineCompletionFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("case summary reached Provider") })
	current := seedOrdinarySummaryTurnsForTest(t, f)
	thread, err := f.handler.store.GetThread(current.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	frozen := newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{ThreadID: current.ThreadID, TurnID: current.TurnID, WorkspaceRealPath: current.WorkspaceRealPath, ContextEpoch: current.ContextEpoch})
	thread["securityState"] = turnsecurityapp.PublicRecord(frozen)
	if text, err := f.handler.summarizeOrdinaryCompaction(context.Background(), threadapp.OrdinarySummaryInput{Thread: thread, Messages: nil}); err == nil || text != "" || f.calls.Load() != 0 {
		t.Fatalf("case authority widened: textAvailable=%t err=%v physicalCalls=%d", text != "", err, f.calls.Load())
	}
}

func TestOrdinarySummaryIncomingCaseRiskBeforeHardLimitMakesZeroProviderCalls(t *testing.T) {
	for _, risk := range []string{"intent", "lexical"} {
		t.Run(risk, func(t *testing.T) {
			f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"The user wants the safe task continued using recorded decisions.\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			})
			current := seedOrdinarySummaryTurnsForTest(t, f)
			thread, err := f.handler.store.GetThread(current.ThreadID)
			if err != nil {
				t.Fatal(err)
			}
			config := f.handler.providerConfig.TurnConfig(stringField(thread, "providerId"), stringField(thread, "model"))
			thresholds := appmodel.ResolveContextThresholdsV1(config.ContextWindowTokens)
			prompt := strings.Repeat("Continue the recorded task. ", 22000)
			prefix := cloneMap(thread)
			prefix["turns"] = listAny(thread["turns"])[:2]
			messages, err := f.handler.projectOrdinaryCompactionHistory(context.Background(), prefix)
			if err != nil || appmodel.EstimateMessagesTokensV1(messages) >= thresholds.HardThresholdTokens || appmodel.EstimateTextTokensV1(prompt) < thresholds.HardThresholdTokens {
				t.Fatalf("risk-order fixture is not under-summary/over-foreground budget: err=%v", err)
			}
			request := startRuntimeTurnRequest{Prompt: prompt}
			if risk == "intent" {
				request.RiskIntent = domainsecurity.RiskClassCase
			} else {
				request.Prompt += " 当前案件的交易金额是2645472.00元，请核实。"
			}
			_, err = f.handler.startRuntimeTurn(context.Background(), current.ThreadID, request)
			if err == nil {
				t.Fatal("case hard-limit request was admitted")
			}
			if f.calls.Load() != 0 {
				t.Fatalf("incoming case risk reached summary before foreground admission: physicalCalls=%d", f.calls.Load())
			}
		})
	}
}

func TestOrdinarySummaryReceiptBarrierAppendFailureRetryCompetitionAndRestart(t *testing.T) {
	f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Continue the safe task using recorded decisions.\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	})
	current := seedOrdinarySummaryTurnsForTest(t, f)
	store := f.handler.store
	store.beforeRecordEventHook = func(event map[string]any) error {
		if isOrdinaryCompactionCompletionV3(event) {
			return errors.New("synthetic completion append fault")
		}
		return nil
	}
	if _, err := f.handler.runtimeThreadService().Compact(context.Background(), current.ThreadID, "manual"); err == nil {
		t.Fatal("initial append fault ignored")
	}
	committed, _ := store.GetThread(current.ThreadID)
	oldTurn := listAny(committed["turns"])[0].(map[string]any)
	oldCompleted, err := threadapp.OrdinaryCompactionCompletedEventV3(committed, stringField(oldTurn, "id"))
	if err != nil {
		t.Fatal(err)
	}
	// Later foreground turns advance the high-water; the older snapshot remains
	// history evidence and is never installed as current execution authority.
	seedOrdinarySummaryTurnsRangeForTest(t, f, 4, 8)
	before, _ := store.GetThread(current.ThreadID)
	digest, _ := threadapp.CompactionBaselineDigest(before)
	if _, err := f.handler.runtimeThreadService().Compact(context.Background(), current.ThreadID, "manual"); err == nil {
		t.Fatal("barrier append fault ignored")
	}
	after, _ := store.GetThread(current.ThreadID)
	afterDigest, _ := threadapp.CompactionBaselineDigest(after)
	if afterDigest != digest {
		t.Fatal("failed receipt barrier removed primary marker")
	}
	store.beforeRecordEventHook = nil
	// Normal completion retries compete with a later compaction barrier through
	// the same owner lock and must reuse exactly the same durable receipt.
	var wg sync.WaitGroup
	failures := make(chan error, 4)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _, err := store.RecordEvent(oldCompleted); failures <- err }()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := f.handler.runtimeThreadService().Compact(context.Background(), current.ThreadID, "manual")
		failures <- err
	}()
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	latest, _ := store.GetThread(current.ThreadID)
	if _, err := threadapp.OrdinaryCompactionCompletedEventV3(latest, stringField(oldTurn, "id")); err == nil {
		t.Fatal("old marker was not replaced")
	}
	reopened, err := newDurableEventSessionStore(f.durableRoot, store.mode, false)
	if err != nil {
		t.Fatal(err)
	}
	reopened.SetCaseThreadAuthority(f.handler.caseThreads)
	for i := 0; i < 2; i++ {
		if err := reopened.RecoverOrdinaryCompactions(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	replay, err := reopened.LoadEventsSince(current.ThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	oldCount := 0
	for _, event := range replay.Events {
		if stringField(event, "compactionId") == stringField(oldCompleted, "compactionId") && isOrdinaryCompactionCompletionV3(event) {
			oldCount++
		}
	}
	if oldCount != 1 || f.calls.Load() != 3 {
		t.Fatalf("receipt duplicated or recovery called Provider: old=%d physical=%d", oldCount, f.calls.Load())
	}
	conflict := cloneMap(oldCompleted)
	conflict["checkpointDigest"] = strings.Repeat("a", 64)
	if _, _, err := reopened.RecordEvent(conflict); err == nil {
		t.Fatal("conflicting completion receipt accepted")
	}
}

func TestOrdinarySummaryReceiptBarrierCASLoserDoesNotPublish(t *testing.T) {
	f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Continue the safe task using recorded decisions.\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	})
	current := seedOrdinarySummaryTurnsForTest(t, f)
	store := f.handler.store
	store.beforeRecordEventHook = func(event map[string]any) error {
		if isOrdinaryCompactionCompletionV3(event) {
			return errors.New("synthetic append fault")
		}
		return nil
	}
	if _, err := f.handler.runtimeThreadService().Compact(context.Background(), current.ThreadID, "manual"); err == nil {
		t.Fatal("append fault ignored")
	}
	seedOrdinarySummaryTurnsRangeForTest(t, f, 4, 8)
	before, _ := store.GetThread(current.ThreadID)
	prepared, err := threadapp.PrepareCompaction(before, current.ThreadID, "manual", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	changed := cloneMap(before)
	changed["title"] = "concurrent mutation"
	store.mu.Lock()
	err = store.upsertThreadNoLock(changed, false)
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	store.beforeRecordEventHook = nil
	if result, err := store.CommitCompaction(prepared.CommitRequest()); !errors.Is(err, threadapp.ErrCompactionBaselineConflict) || result.Committed {
		t.Fatalf("CAS loser committed: result=%#v err=%v", result, err)
	}
	replay, err := store.LoadEventsSince(current.ThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range replay.Events {
		if isOrdinaryCompactionCompletionV3(event) {
			t.Fatal("CAS loser triggered success publication")
		}
	}
}

func TestOrdinarySummaryReceiptRetryUsesCommittedReplayAndRestoresHighWater(t *testing.T) {
	f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Continue the safe task using recorded decisions.\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	})
	current := seedOrdinarySummaryTurnsForTest(t, f)
	store := f.handler.store
	live, unsubscribe := store.SubscribeEvents(current.ThreadID)
	defer unsubscribe()
	committedFault := false
	store.beforeRecordEventHook = func(event map[string]any) error {
		if !isOrdinaryCompactionCompletionV3(event) {
			return nil
		}
		// Inject an append-return fault only after the real adapter has written
		// this exact event. The owner must use disk replay rather than retrying
		// from its stale high-water. No new production fault framework is added.
		durable := cloneMap(event)
		durable["seq"] = float64(store.highestSeqByThread[current.ThreadID] + 1)
		durable["timestamp"] = time.Now().UTC().Format(time.RFC3339Nano)
		if err := store.eventLog.AppendEvent(current.ThreadID, durable); err != nil {
			return err
		}
		committedFault = true
		store.beforeRecordEventHook = nil
		return &eventlog.AtomicAppendError{Committed: true, Cause: errors.New("synthetic committed append return fault")}
	}
	if _, err := f.handler.runtimeThreadService().Compact(context.Background(), current.ThreadID, "manual"); err != nil {
		t.Fatal(err)
	}
	if !committedFault {
		t.Fatal("committed append fault was not exercised")
	}
	replay, err := store.LoadEventsSince(current.ThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var receipt map[string]any
	for _, event := range replay.Events {
		if isOrdinaryCompactionCompletionV3(event) {
			receipt = event
		}
	}
	seq, _ := numericSeq(receipt["seq"])
	store.mu.Lock()
	store.highestSeqByThread[current.ThreadID] = seq - 1
	store.mu.Unlock()
	retry := cloneMap(receipt)
	retry["seq"], retry["timestamp"] = float64(seq+100), time.Now().UTC().Format(time.RFC3339Nano)
	recorded, _, err := store.RecordEvent(retry)
	if err != nil {
		t.Fatal(err)
	}
	actual, _ := numericSeq(recorded["seq"])
	if actual != seq {
		t.Fatal("retry allocated a new completion sequence")
	}
	next, _, err := store.RecordEvent(map[string]any{"kind": "usage", "threadId": current.ThreadID, "turnId": "turn_receipt_retry"})
	if err != nil {
		t.Fatal(err)
	}
	nextSeq, _ := numericSeq(next["seq"])
	if nextSeq <= seq {
		t.Fatal("committed append retry failed to restore sequence high-water")
	}
	// The real live subscriber must receive the durable completion before the
	// later sequence. A replay-only receipt can otherwise be skipped when that
	// later delivery advances the SSE cursor. Retries must not publish it twice.
	var liveSequences []int
drainLive:
	for {
		select {
		case event, ok := <-live:
			if !ok {
				t.Fatal("receipt subscriber disconnected despite available capacity")
			}
			if isOrdinaryCompactionCompletionV3(event) || stringField(event, "turnId") == "turn_receipt_retry" {
				liveSeq, _ := numericSeq(event["seq"])
				liveSequences = append(liveSequences, liveSeq)
			}
		default:
			break drainLive
		}
	}
	if len(liveSequences) != 2 || liveSequences[0] != seq || liveSequences[1] != nextSeq {
		t.Fatalf("durable completion did not publish exactly once before later event: got=%v want=[%d %d]", liveSequences, seq, nextSeq)
	}
	// A receipt retry must also preserve already reserved pending sequences.
	store.mu.Lock()
	store.pendingEvents[current.ThreadID] = map[int]map[string]any{nextSeq + 1: {"kind": "usage", "threadId": current.ThreadID, "seq": float64(nextSeq + 1)}}
	store.highestSeqByThread[current.ThreadID] = nextSeq + 1
	store.mu.Unlock()
	if _, _, err := store.RecordEvent(retry); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	reserved := store.highestSeqByThread[current.ThreadID]
	delete(store.pendingEvents, current.ThreadID)
	store.highestSeqByThread[current.ThreadID] = nextSeq
	store.mu.Unlock()
	if reserved != nextSeq+1 {
		t.Fatal("receipt retry lowered the pending sequence frontier")
	}
	select {
	case event := <-live:
		t.Fatalf("receipt retry produced another live side effect: kind=%s", stringField(event, "kind"))
	default:
	}
}

func TestOrdinarySummaryReceiptNormalAppendPublishesOnceWithoutRetryEffects(t *testing.T) {
	f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Continue the safe task using recorded decisions.\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	})
	current := seedOrdinarySummaryTurnsForTest(t, f)
	store := f.handler.store
	live, unsubscribe := store.SubscribeEvents(current.ThreadID)
	defer unsubscribe()
	if _, err := f.handler.runtimeThreadService().Compact(context.Background(), current.ThreadID, "manual"); err != nil {
		t.Fatal(err)
	}
	var receipt map[string]any
drainLive:
	for {
		select {
		case event, ok := <-live:
			if !ok {
				t.Fatal("normal receipt subscriber unexpectedly disconnected")
			}
			if isOrdinaryCompactionCompletionV3(event) {
				if receipt != nil {
					t.Fatal("normal append published the completion twice")
				}
				receipt = event
			}
		default:
			break drainLive
		}
	}
	if receipt == nil {
		t.Fatal("normal append did not publish its completion")
	}
	if _, _, err := store.RecordEvent(receipt); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-live:
		t.Fatalf("normal receipt retry produced another live side effect: kind=%s", stringField(event, "kind"))
	default:
	}
	if f.calls.Load() != 1 {
		t.Fatal("receipt retry invoked Provider again")
	}
}

func TestOrdinarySummaryReceiptBarrierClosesOldCheckpointAfterEpochAdvance(t *testing.T) {
	f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Continue the safe task using recorded decisions.\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	})
	current := seedOrdinarySummaryTurnsForTest(t, f)
	store := f.handler.store
	store.beforeRecordEventHook = func(event map[string]any) error {
		if isOrdinaryCompactionCompletionV3(event) {
			return errors.New("synthetic append fault")
		}
		return nil
	}
	if _, err := f.handler.runtimeThreadService().Compact(context.Background(), current.ThreadID, "manual"); err == nil {
		t.Fatal("append fault ignored")
	}
	seedOrdinarySummaryTurnsRangeForTest(t, f, 4, 8)
	thread, _ := store.GetThread(current.ThreadID)
	markerTurn := listAny(thread["turns"])[0].(map[string]any)
	old, err := threadapp.OrdinaryCompactionCompletedEventV3(thread, stringField(markerTurn, "id"))
	if err != nil {
		t.Fatal(err)
	}
	state, err := domaincontextepoch.ParseState(thread["contextEpochState"])
	if err != nil {
		t.Fatal(err)
	}
	beforeEpoch := state.AcceptedSnapshot.Epoch
	state, _, err = contextepochapp.Upsert(state, domaincontextepoch.SourceEntry{Version: 1, SourceID: "synthetic-history-diagnostics", Kind: "diagnostic", Digest: strings.Repeat("b", 64), Sequence: 1, TrustState: domaincontextepoch.TrustTrusted, PromptBoundary: domaincontextepoch.BoundaryDiagnosticsOnly, ActivationState: domaincontextepoch.ActivationInactive}, false)
	if err != nil {
		t.Fatal(err)
	}
	thread["contextEpochState"] = contextepochapp.PublicState(state)
	current, _ = domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	start, err := contextepochapp.PrepareTurn(contextepochapp.PrepareTurnInput{Thread: thread, SecurityContext: current, At: time.Now().UTC()})
	if err != nil || start.State.AcceptedSnapshot.Epoch <= beforeEpoch {
		t.Fatalf("fixture did not advance existing epoch: err=%v", err)
	}
	// Advance through a new admitted turn. The immutable terminal outbox for
	// the earlier completed turn retains its exact frozen context.
	at := time.Now().UTC()
	frozen, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{Context: context.Background(), Authority: f.handler.turnSecurity,
		Thread: thread, ThreadID: current.ThreadID, TurnID: "turn_summary_epoch", Workspace: f.workspace, Principal: testIdentityPrincipal(), IssuedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	start, err = contextepochapp.PrepareTurn(contextepochapp.PrepareTurnInput{Thread: thread, SecurityContext: frozen, At: at})
	if err != nil || start.State.AcceptedSnapshot.Epoch <= beforeEpoch {
		t.Fatalf("new turn epoch did not advance: err=%v", err)
	}
	current = start.SecurityContext
	stamp := at.Format(time.RFC3339Nano)
	turn := map[string]any{"id": current.TurnID, "threadId": current.ThreadID, "status": "running", "createdAt": stamp,
		"securityContext": turnsecurityapp.PublicRecord(current), "contextEpochSnapshot": contextepochapp.PublicSnapshot(start.State.AcceptedSnapshot),
		"items": []any{map[string]any{"id": "item_summary_epoch", "threadId": current.ThreadID, "turnId": current.TurnID, "kind": "user_message", "role": "user", "status": "completed", "createdAt": stamp, "text": "Continue the recorded task."}}}
	if err := store.AppendTurnToThread(current.ThreadID, turn, "openai", map[string]any{"securityState": turnsecurityapp.PublicRecord(current), "contextEpochState": contextepochapp.PublicState(start.State)}); err != nil {
		t.Fatal(err)
	}
	if result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{Store: store, SecurityContext: current, ThreadID: current.ThreadID, TurnID: current.TurnID, Model: "fixture-model", CreatedAt: stamp, FinishedAt: stamp}); err != nil || !result.Changed {
		t.Fatalf("epoch turn terminal: err=%v", err)
	}
	actual, _ := store.GetThread(current.ThreadID)
	if _, err := threadapp.OrdinaryCompactionCompletedEventV3(actual, stringField(markerTurn, "id")); err != nil {
		t.Fatal("later epoch invalidated actual old checkpoint", err)
	}
	prepared, err := threadapp.PrepareCompaction(actual, current.ThreadID, "manual", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	store.beforeRecordEventHook = nil
	if result, err := store.CommitCompaction(prepared.CommitRequest()); err != nil || !result.Committed {
		t.Fatalf("barrier CAS failed: err=%v", err)
	}
	replay, err := store.LoadEventsSince(current.ThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range replay.Events {
		if isOrdinaryCompactionCompletionV3(event) && stringField(event, "compactionId") == stringField(old, "compactionId") {
			if !sameOrdinaryCompletionFacts(event, old) {
				t.Fatal("later epoch invented old completion facts")
			}
			count++
		}
	}
	if count != 1 || f.calls.Load() != 1 {
		t.Fatalf("epoch barrier receipt=%d Provider=%d", count, f.calls.Load())
	}
}

func TestOrdinarySummaryDerivedForkResumeReachesPhysicalProviderWithoutParentAuthority(t *testing.T) {
	for _, mode := range []string{"manual-fork", "manual-prefix-fork", "automatic-resume"} {
		t.Run(mode, func(t *testing.T) {
			const decision = "AMBER_ORBIT_DECISION_ONLY_IN_SUMMARY"
			var bodies []string
			var bodiesMu sync.Mutex
			f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				bodiesMu.Lock()
				isSummary := len(bodies) == 0
				bodies = append(bodies, string(body))
				bodiesMu.Unlock()
				text := "The inherited task can continue."
				if isSummary {
					text = "Keep the accepted old decision: " + decision + "."
				}
				encoded, _ := json.Marshal(text)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%s},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", encoded)
			})
			parentContext := seedOrdinarySummaryTurnsForTest(t, f)
			if mode == "automatic-resume" {
				reservation, release, err := f.handler.runtimeControl().PrepareForegroundWithWitness(context.Background(), parentContext.ThreadID)
				if err != nil {
					t.Fatal(err)
				}
				result, err := f.handler.runtimeThreadService().AutoCompactBeforeTurnV1(context.Background(), threadapp.AutoCompactionInputV1{
					Reservation: reservation, ThreadID: parentContext.ThreadID, Prompt: "Continue the chosen plan.", ContextWindowTokens: 16000, MainThread: true,
				})
				release()
				if err != nil || !result.Compacted {
					t.Fatalf("automatic source was not actually compacted: compacted=%v err=%v", result.Compacted, err)
				}
			} else if _, err := f.handler.runtimeThreadService().Compact(context.Background(), parentContext.ThreadID, "manual"); err != nil {
				t.Fatal(err)
			}
			parent, err := f.handler.store.GetThread(parentContext.ThreadID)
			if err != nil {
				t.Fatal(err)
			}
			parentContext, err = domainsecurity.ParseTurnSecurityContext(parent["securityState"])
			if err != nil {
				t.Fatal(err)
			}
			markerTurn := listAny(parent["turns"])[0].(map[string]any)
			marker := listAny(markerTurn["items"])[0].(map[string]any)
			if !strings.Contains(stringField(marker, "summary"), decision) {
				t.Fatal("source has no real summary-only decision")
			}
			for _, raw := range listAny(parent["turns"])[1:] {
				for _, rawItem := range listAny(raw.(map[string]any)["items"]) {
					itemBody, _ := json.Marshal(rawItem)
					if strings.Contains(string(itemBody), decision) {
						t.Fatal("decision is also present in retained ordinary history")
					}
				}
			}
			// A structurally valid synthetic old grant is actual private source
			// data here; derivation must strip it rather than copy its permission.
			oldCallID, err := domainsecurity.NewHostToolCallIDV1([]byte(strings.Repeat("p", domainsecurity.HostToolCallIDEntropyBytesV1)))
			if err != nil {
				t.Fatal(err)
			}
			oldGrant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{Context: parentContext, Provider: "host", ServerIdentity: "host:builtin", ToolName: "read_task_history", ToolCallID: oldCallID,
				ConnectionEpoch: 0, ArgsHash: domainsecurity.SHA256Hex([]byte("fixture args")), SchemaHash: domainsecurity.SHA256Hex([]byte("fixture schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("fixture scope")), ReadOnly: true, ApprovalState: "approved", IssuedAt: time.Now().UTC()})
			if err := domainsecurity.ValidateExecutionGrant(oldGrant); err != nil {
				t.Fatal(err)
			}
			grantBody, err := json.Marshal(oldGrant)
			if err != nil {
				t.Fatal(err)
			}
			var grantRecord map[string]any
			if err := json.Unmarshal(grantBody, &grantRecord); err != nil {
				t.Fatal(err)
			}
			parent["executionGrant"], parent["executionGrantId"] = grantRecord, oldGrant.GrantID
			markerTurn["executionGrant"], markerTurn["executionGrantId"] = grantRecord, oldGrant.GrantID
			f.handler.store.mu.Lock()
			err = f.handler.store.upsertThreadNoLock(parent, false)
			f.handler.store.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			// Valid display bytes are not sufficient source authority: both
			// existing builders must verify the actual committed source before
			// producing a historical data binding.
			for _, missing := range []string{"securityContext", "contextEpochSnapshot"} {
				unverified := contracts.CloneMap(parent)
				delete(listAny(unverified["turns"])[0].(map[string]any), missing)
				for _, builder := range []string{"fork", "resume"} {
					var rejected map[string]any
					var err error
					if builder == "fork" {
						rejected, err = threadapp.BuildFork(threadapp.ForkInput{Source: unverified, ForkID: "unverified_fork", ParentThreadID: parentContext.ThreadID, Now: time.Now().UTC().Format(time.RFC3339Nano)})
					} else {
						rejected, err = threadapp.BuildResume(threadapp.ResumeInput{Source: unverified, ThreadID: "unverified_resume", SessionID: "unverified_session", Now: time.Now().UTC().Format(time.RFC3339Nano)})
					}
					if err != nil {
						t.Fatal(err)
					}
					turn := listAny(rejected["turns"])[0].(map[string]any)
					if turn[domainthread.CompactionHistoryDerivationFieldV1] != nil {
						t.Fatalf("%s copied unverified source authority without %s", builder, missing)
					}
					if !strings.Contains(stringField(listAny(turn["items"])[0].(map[string]any), "summary"), decision) {
						t.Fatal("unavailable source lost its valid historical display data")
					}
				}
			}
			var child map[string]any
			if mode == "automatic-resume" {
				resumed, err := f.handler.store.ResumeSession(parentContext.ThreadID, map[string]any{})
				if err != nil {
					t.Fatal(err)
				}
				child, err = f.handler.store.GetThread(stringField(resumed, "thread_id"))
				if err != nil {
					t.Fatal(err)
				}
			} else {
				request := map[string]any{}
				if mode == "manual-prefix-fork" {
					request["turnId"] = stringField(markerTurn, "id")
				}
				child, err = f.handler.store.ForkThread(parentContext.ThreadID, request)
				if err != nil {
					t.Fatal(err)
				}
			}
			assertNoParentAuthority := func(thread map[string]any) {
				t.Helper()
				for _, key := range []string{"securityState", "contextEpochState", "executionGrant", "executionGrantId", "contextDigest"} {
					if thread[key] != nil {
						t.Fatalf("derived shell copied parent authority field %s", key)
					}
				}
				for _, raw := range listAny(thread["turns"]) {
					turn := raw.(map[string]any)
					for _, key := range []string{"securityContext", "contextEpochSnapshot", "executionGrant", "executionGrantId", "contextDigest"} {
						if turn[key] != nil {
							t.Fatalf("derived history copied parent turn authority field %s", key)
						}
					}
				}
			}
			assertNoParentAuthority(child)
			startAndCheck := func(threadID string) map[string]any {
				t.Helper()
				bodiesMu.Lock()
				before := len(bodies)
				bodiesMu.Unlock()
				if _, err := f.handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{Prompt: "Continue the chosen plan."}); err != nil {
					t.Fatal("normal new-turn admission failed", err)
				}
				bodiesMu.Lock()
				requests := append([]string(nil), bodies[before:]...)
				bodiesMu.Unlock()
				if len(requests) != 1 || !strings.Contains(requests[0], decision) {
					t.Fatalf("%s normal physical Provider request lost the summary-only decision: physical=%d", mode, len(requests))
				}
				if mode == "automatic-resume" && (!strings.Contains(requests[0], "hist1_") || !strings.Contains(requests[0], "read_task_history")) {
					t.Fatal("automatic continuation did not reach the physical request as bounded original-source references")
				}
				if strings.Contains(requests[0], parentContext.ContextDigest) || strings.Contains(requests[0], oldGrant.GrantID) || strings.Contains(requests[0], "compactionHistoryDerivation") {
					t.Fatal("Provider request acquired private parent authority or derivation metadata")
				}
				current, err := f.handler.store.GetThread(threadID)
				if err != nil {
					t.Fatal(err)
				}
				frozen, err := domainsecurity.ParseTurnSecurityContext(current["securityState"])
				if err != nil || frozen.ThreadID != threadID || frozen.ContextDigest == parentContext.ContextDigest || frozen.ContextEpoch >= parentContext.ContextEpoch {
					t.Fatal("new turn did not receive its own normal child context/epoch")
				}
				for _, raw := range listAny(current["turns"]) {
					turn := raw.(map[string]any)
					items := listAny(turn["items"])
					if len(items) != 1 {
						continue
					}
					item, _ := items[0].(map[string]any)
					if !domainevent.ValidGeneralCompactionProviderHistoryItemV5(item) {
						continue
					}
					if !appmodel.GeneralCompactionDerivedHistoryBoundV5(current, item) || appmodel.GeneralCompactionCheckpointBoundV5(current, item) {
						t.Fatal("derived data did not use its own binding or revived a native execution checkpoint")
					}
					if _, err := threadapp.OrdinaryCompactionCompletedEventV3(current, stringField(turn, "id")); err == nil {
						t.Fatal("derived history manufactured a new native compaction completion")
					}
				}
				public, err := threadapp.ProjectPublicThread(current)
				publicBody, _ := json.Marshal(public)
				if err != nil || strings.Contains(string(publicBody), "compactionHistoryDerivation") || strings.Contains(string(publicBody), oldGrant.GrantID) {
					t.Fatal("private derivation or the parent grant entered public history", err)
				}
				return current
			}
			child = startAndCheck(stringField(child, "id"))
			bodiesMu.Lock()
			summaryInstructionsOnWire := strings.Contains(bodies[0], "Summarize the conversation")
			bodiesMu.Unlock()
			if !summaryInstructionsOnWire {
				t.Fatal("actual summary request omitted its system instructions")
			}
			// Display bytes without the verified export binding cannot enter
			// model history; changing its scope or replaying parent authority also
			// fails closed without invoking another Provider.
			for _, fault := range []string{"missing-data-binding", "changed-data-scope", "parent-context"} {
				copy := contracts.CloneMap(child)
				turn := listAny(copy["turns"])[0].(map[string]any)
				switch fault {
				case "missing-data-binding":
					delete(turn, domainthread.CompactionHistoryDerivationFieldV1)
				case "changed-data-scope":
					proof, _ := domainthread.ParseCompactionHistoryDerivationV1(turn[domainthread.CompactionHistoryDerivationFieldV1])
					proof.ScopeDigest = strings.Repeat("0", 64)
					turn[domainthread.CompactionHistoryDerivationFieldV1] = domainthread.CompactionHistoryDerivationMapV1(proof)
				case "parent-context":
					copy["securityState"] = turnsecurityapp.PublicRecord(parentContext)
				}
				for _, message := range appmodel.ProviderHistoryFromThread(copy) {
					if strings.Contains(message.Content, decision) {
						t.Fatalf("%s resurrected summary data without its child binding", fault)
					}
				}
			}
			if mode == "automatic-resume" {
				childItem := listAny(listAny(child["turns"])[0].(map[string]any)["items"])[0].(map[string]any)
				snapshot, err := domainthread.ParseTaskContinuationSnapshotV1(childItem["taskContinuation"])
				scope, ok := appmodel.ContinuationSourceScopeV1(child)
				if err != nil || snapshot.UserHistory == nil || !ok || snapshot.UserHistory.ScopeDigest != scope || !appmodel.ContinuationSourceReferencesRequiredV1(child) {
					t.Fatal("automatic continuation sources did not bind the actual child data scope")
				}
				frozen, _ := domainsecurity.ParseTurnSecurityContext(child["securityState"])
				page, err := turnapp.ReadTaskContinuationSourceV1(child, frozen, snapshot.UserHistory.Sources[0].Reference, 0, 80)
				if err != nil || !strings.HasPrefix(stringField(page, "text"), "SOURCE_0 ") {
					t.Fatal("child bounded source reader lost the original source", err)
				}
				// The same data projection survives cold replay and another real fork;
				// a parent checkpoint or old execution witness is never restored.
				reopened, err := newDurableEventSessionStore(f.durableRoot, f.handler.store.mode, false)
				if err != nil {
					t.Fatal(err)
				}
				reopened.SetCaseThreadAuthority(f.handler.caseThreads)
				f.handler.store, f.handler.threads = reopened, nil
				grandchild, err := reopened.ForkThread(stringField(child, "id"), map[string]any{})
				if err != nil {
					t.Fatal(err)
				}
				assertNoParentAuthority(grandchild)
				grandchild = startAndCheck(stringField(grandchild, "id"))
				grandchildItem := listAny(listAny(grandchild["turns"])[0].(map[string]any)["items"])[0].(map[string]any)
				grandSnapshot, err := domainthread.ParseTaskContinuationSnapshotV1(grandchildItem["taskContinuation"])
				grandFrozen, _ := domainsecurity.ParseTurnSecurityContext(grandchild["securityState"])
				if err != nil || grandSnapshot.UserHistory == nil {
					t.Fatal("re-fork lost the typed source snapshot", err)
				}
				if _, err := turnapp.ReadTaskContinuationSourceV1(grandchild, grandFrozen, grandSnapshot.UserHistory.Sources[0].Reference, 0, 80); err != nil {
					t.Fatal("re-fork lost bounded source readability", err)
				}
			}
		})
	}
}
