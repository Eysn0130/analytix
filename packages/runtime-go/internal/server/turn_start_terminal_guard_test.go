package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

type asyncTurnTerminalGuardFailureProvider struct {
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	failure     error
}

func (*asyncTurnTerminalGuardFailureProvider) RequiresDurablePipelineStagesV1() {}

func (provider *asyncTurnTerminalGuardFailureProvider) Stream(ctx context.Context, request domainmodel.Request) (domainmodel.Result, error) {
	if err := emitTestDurableProviderPipelinePairV1(request); err != nil {
		return domainmodel.Result{}, err
	}
	provider.startedOnce.Do(func() { close(provider.started) })
	select {
	case <-provider.release:
		return domainmodel.Result{}, provider.failure
	case <-ctx.Done():
		return domainmodel.Result{}, ctx.Err()
	}
}

func TestAsyncTurnFailurePersistenceStderrUsesFixedProjection(t *testing.T) {
	const expectedThreadID = "thr_durable_13900000006"
	const expectedTurnID = "turn_13900000007"
	const providerErrorSentinel = "provider failed: /private/provider-pii-13900000008"
	const recordErrorSentinel = "terminal append failed: /private/record-pii-13900000009"

	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "guard-provider", BaseURL: "https://provider.invalid", APIKey: "test-key", Model: "guard-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	handler.store.mu.Lock()
	meta, err := handler.store.readMetaNoLock()
	if err == nil {
		meta.ThreadCounter = 13900000005
		err = handler.store.writeMetaNoLock(meta)
	}
	handler.store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	handler.turnSeq = 13900000006

	workspace := t.TempDir()
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "Async terminal guard", "workspace": workspace, "providerId": "guard-provider", "model": "guard-model",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	if threadID != expectedThreadID {
		t.Fatalf("hostile thread identity fixture mismatch: got=%q want=%q", threadID, expectedThreadID)
	}

	provider := &asyncTurnTerminalGuardFailureProvider{
		started: make(chan struct{}), release: make(chan struct{}), failure: errors.New(providerErrorSentinel),
	}
	handler.provider = provider
	var asyncObservations []AsyncTurnObservationV1
	handler.asyncTurnObserver = func(observation AsyncTurnObservationV1) {
		asyncObservations = append(asyncObservations, observation)
	}
	var appendAttempts atomic.Int64
	handler.store.generalTerminalAtomicAppend = func(string, []map[string]any) error {
		appendAttempts.Add(1)
		return errors.New(recordErrorSentinel)
	}

	var turnID string
	diagnostic := captureRuntimeStderrForTest(t, func() {
		response, startErr := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
			Prompt: "exercise the real async terminal guard", Async: true,
		})
		if startErr != nil {
			t.Fatal(startErr)
		}
		turnID = stringField(response, "turnId")
		if turnID != expectedTurnID {
			t.Fatalf("hostile turn identity fixture mismatch: got=%q want=%q", turnID, expectedTurnID)
		}
		select {
		case <-provider.started:
		case <-time.After(15 * time.Second):
			t.Fatal("provider did not enter the real async turn path")
		}
		close(provider.release)
		waitContext, cancelWait := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancelWait()
		if waitErr := handler.runtimeControl().WaitForTurnOperations(waitContext); waitErr != nil {
			t.Fatalf("async turn operation did not finish: %v", waitErr)
		}
	})

	// WaitForTurnOperations succeeds even when failure recording failed. The
	// private observer must report both errors before that drain returns.
	if len(asyncObservations) != 2 || asyncObservations[0].Stage != "started" || asyncObservations[1].Stage != "finished" {
		t.Fatal("complete async operation was not observed before drain")
	}
	observed := asyncObservations[1]
	if observed.ThreadID != threadID || observed.TurnID != turnID || observed.CompletionPhase != "loop" ||
		observed.CompletionErrorClass == "none" || observed.CompletionErrorClass == "" ||
		observed.FailureRecordErrorClass == "none" || observed.FailureRecordErrorClass == "" || observed.TerminalStatus != "failed" {
		t.Fatal("complete async observer missed the injected execution or failure-record error")
	}

	reloaded, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	if status := stringField(reloaded, "status"); status != "idle" {
		t.Fatalf("async failure did not restore idle thread state: status=%q thread=%#v", status, reloaded)
	}
	turns := listAny(reloaded["turns"])
	if len(turns) != 1 {
		t.Fatalf("expected one durable async turn, got %#v", turns)
	}
	turn, _ := turns[0].(map[string]any)
	if stringField(turn, "id") != turnID || stringField(turn, "status") != "failed" ||
		turn["generalTerminalCASBinding"] == nil || turn["generalTerminalPublication"] == nil {
		t.Fatalf("async failure terminal CAS changed: %#v", turn)
	}
	items := listAny(turn["items"])
	if len(items) != 2 || stringField(items[0].(map[string]any), "kind") != "user_message" ||
		stringField(items[1].(map[string]any), "kind") != "error" {
		t.Fatalf("async failure terminal item changed: %#v", items)
	}
	if attempts := appendAttempts.Load(); attempts != 1 {
		t.Fatalf("async failure persistence attempt changed: attempts=%d want=1", attempts)
	}
	replay, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	foundStart := false
	for _, event := range replay.Events {
		if stringField(event, "turnId") != turnID {
			continue
		}
		switch stringField(event, "kind") {
		case "turn_started":
			foundStart = true
		case "turn_failed":
			t.Fatalf("failed terminal append unexpectedly published an event: %#v", replay.Events)
		}
	}
	if !foundStart {
		t.Fatalf("real async turn start evidence missing: %#v", replay.Events)
	}
	if !handler.runtimeControl().RegisterTurnCancel(threadID, turnID, func() {}) {
		t.Fatal("async failure retained its cancellation registration")
	}
	if !handler.runtimeControl().UnregisterTurnCancel(threadID, turnID) {
		t.Fatal("cleanup probe cancellation registration was not removable")
	}

	const expected = "[analytix] event=ANALYTIX_RUNTIME_ASYNC_TURN_FAILURE_RECORD_FAILED\n"
	if diagnostic != expected {
		t.Fatalf("async turn failure stderr projection mismatch: got=%q want=%q", diagnostic, expected)
	}
	for _, sentinel := range []string{threadID, turnID, providerErrorSentinel, recordErrorSentinel, "/private/"} {
		if strings.Contains(diagnostic, sentinel) {
			t.Fatalf("async turn failure stderr leaked hostile sentinel %q: %q", sentinel, diagnostic)
		}
	}
}

func TestPostTerminalGoalLineageFailureStderrUsesFixedProjection(t *testing.T) {
	const jobPathSentinel = "customer-pii-13900000058-goal-lineage"
	dataDir := filepath.Join(t.TempDir(), jobPathSentinel)
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: dataDir,
		ProviderID: "goal-lineage-provider", BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: "goal-lineage-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	provider := &countingImmediateWorkspaceProvider{}
	handler.provider = provider

	workspace := t.TempDir()
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "Post-terminal goal lineage", "workspace": workspace,
		"providerId": "goal-lineage-provider", "model": "goal-lineage-model",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	if _, err := handler.store.SetGoal(threadID, map[string]any{
		"objective": "preserve the completed turn", "status": "active", "strictCompletion": true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "child-runs"), []byte("block child-run persistence\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var response map[string]any
	diagnostic := captureRuntimeStderrForTest(t, func() {
		response, err = handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
			Prompt: "complete ordinary work before recording goal lineage",
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	turnID := stringField(response, "turnId")
	if stringField(response, "status") != "completed" || turnID == "" {
		t.Fatalf("goal-lineage failure changed the completed response: %#v", response)
	}
	if calls := provider.calls.Load(); calls != 1 {
		t.Fatalf("goal-lineage failure changed ordinary provider work: calls=%d want=1", calls)
	}

	reloaded, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turns := listAny(reloaded["turns"])
	if stringField(reloaded, "status") != "idle" || len(turns) != 1 {
		t.Fatalf("goal-lineage failure changed terminal thread state: %#v", reloaded)
	}
	turn, _ := turns[0].(map[string]any)
	if stringField(turn, "id") != turnID || stringField(turn, "status") != "completed" ||
		turn["generalTerminalCASBinding"] == nil || turn["generalTerminalPublication"] == nil {
		t.Fatalf("goal-lineage failure changed the committed terminal: %#v", turn)
	}
	if records := handler.jobs.AllRecords(); len(records) != 0 {
		t.Fatalf("failed goal-lineage persistence retained a child run: %#v", records)
	}
	replay, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	terminalEvents := 0
	for _, event := range replay.Events {
		if stringField(event, "turnId") != turnID {
			continue
		}
		if stringField(event, "kind") == "turn_completed" {
			terminalEvents++
		}
		if stringField(event, "kind") == "pipeline_stage" && stringField(event, "label") == "internal goal child-run lineage" {
			t.Fatalf("failed goal lineage published a child-run event: %#v", event)
		}
	}
	if terminalEvents != 1 {
		t.Fatalf("goal-lineage failure changed terminal publication count: got=%d events=%#v", terminalEvents, replay.Events)
	}

	const expected = "[analytix] event=ANALYTIX_RUNTIME_POST_TERMINAL_GOAL_LINEAGE_FAILED\n"
	if diagnostic != expected {
		t.Fatalf("post-terminal goal-lineage stderr projection mismatch: got=%q want=%q", diagnostic, expected)
	}
	for _, sentinel := range []string{jobPathSentinel, threadID, turnID, "child-runs", "/private/"} {
		if strings.Contains(diagnostic, sentinel) {
			t.Fatalf("post-terminal goal-lineage stderr leaked hostile sentinel %q: %q", sentinel, diagnostic)
		}
	}
}

func TestPostAppendPreLoopFailureClosesTurn(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "guard-provider", BaseURL: "https://provider.invalid", APIKey: "test-key", Model: "guard-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "Start guard", "workspace": t.TempDir(), "providerId": "guard-provider", "model": "guard-model",
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	injected := false
	handler.store.beforeRecordEventHook = func(event map[string]any) error {
		if !injected && stringField(event, "kind") == "turn_started" {
			injected = true
			return errors.New("injected post-append turn_started failure")
		}
		return nil
	}
	if _, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{Prompt: "must close after append"}); err == nil {
		t.Fatal("injected post-append failure should be returned")
	}
	if !injected {
		t.Fatal("fault injection did not reach the post-append cut")
	}
	reloaded, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turns := listAny(reloaded["turns"])
	if len(turns) != 1 {
		t.Fatalf("expected one durable turn, got %#v", turns)
	}
	turn, _ := turns[0].(map[string]any)
	if status := stringField(turn, "status"); status == "running" || status == "queued" || status == "waiting" {
		t.Fatalf("post-append failure left an active turn: %#v", turn)
	}
	replay, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	foundTerminal := false
	for _, event := range replay.Events {
		if stringField(event, "turnId") == stringField(turn, "id") && stringField(event, "kind") == "turn_failed" {
			foundTerminal = true
		}
	}
	if !foundTerminal {
		t.Fatalf("post-append failure did not persist terminal evidence: %#v", replay.Events)
	}
}

func TestTurnStartEventPairDoesNotPublishAPartialPrefix(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "guard-provider", BaseURL: "https://provider.invalid", APIKey: "test-key", Model: "guard-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "Atomic start guard", "workspace": t.TempDir(), "providerId": "guard-provider", "model": "guard-model",
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	injected := false
	handler.store.beforeRecordEventHook = func(event map[string]any) error {
		if !injected && stringField(event, "kind") == "item_created" {
			injected = true
			return errors.New("injected user item start-event failure")
		}
		return nil
	}
	if _, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{Prompt: "start atomically"}); err == nil {
		t.Fatal("injected user item start-event failure should be returned")
	}
	if !injected {
		t.Fatal("fault injection did not reach the user item start event")
	}
	replay, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range replay.Events {
		switch stringField(event, "kind") {
		case "turn_started", "item_created":
			t.Fatalf("turn start published a partial event prefix: %#v", replay.Events)
		}
	}
}
