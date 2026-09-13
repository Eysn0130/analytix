package server

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	controlapp "analytix.local/runtime-go/internal/app/control"
	appmodel "analytix.local/runtime-go/internal/app/model"
	appturn "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestWorkspacePatchClosesPausedApprovalBeforeEpochCommit(t *testing.T) {
	workspaceA := workspacetest.New(t)
	workspaceB := workspacetest.New(t)
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	thread, err := handler.store.CreateThread(map[string]any{"title": "paused gate rebind", "workspace": workspaceA}, workspaceA)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	now := time.Now().UTC()
	securityContext, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: context.Background(), Authority: handler.turnSecurity, Reader: filestore.CaseBindingReader{}, Thread: thread,
		ThreadID: threadID, TurnID: "turn-paused-approval", Workspace: workspaceA,
		Principal: testIdentityPrincipal(), IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	pending := pausedApprovalForTransition(t, securityContext, now)
	gateID := controlapp.SecureGateID(
		domaincontinuation.KindApproval, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest,
		pending.ExecutionGrant.GrantID, pending.Call.ID,
	)
	itemID := "item_" + gateID
	receipt, err := handler.continuations.IssuePendingHost(domaincontinuation.KindApproval, gateID, itemID, pending, now)
	if err != nil {
		t.Fatal(err)
	}
	item, requestedEvent := appturn.ApprovalRequestRecords(appturn.ApprovalRequestInput{
		ThreadID: pending.ThreadID, TurnID: pending.TurnID, ItemID: itemID, ApprovalID: gateID,
		CreatedAt: now.Format(time.RFC3339Nano), ToolName: pending.Call.Name,
		ApprovalPolicy: pending.ApprovalPolicy, SandboxMode: pending.SandboxMode, ContinuationReceiptID: receipt.ReceiptID,
	})
	epochState, err := contextepochapp.BootstrapState(
		threadID, securityContext.ContextEpoch,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(securityContext)}, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := handler.store.AppendTurnToThread(threadID, map[string]any{
		"id": pending.TurnID, "threadId": threadID, "status": "running", "createdAt": now.Format(time.RFC3339Nano),
		"securityContext": securityRecord, "items": []any{item},
	}, "provider-a", map[string]any{
		"securityState": securityRecord, "contextEpochState": contextepochapp.PublicState(epochState),
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := handler.store.RecordEvent(requestedEvent); err != nil {
		t.Fatal(err)
	}
	record := controlapp.GateRecord{
		ThreadID: threadID, TurnID: pending.TurnID, ItemID: itemID,
		ToolName: pending.Call.Name, ContinuationReceiptID: receipt.ReceiptID,
	}
	if !handler.gates.RegisterPendingApproval(gateID, record, pending) {
		t.Fatal("register paused approval")
	}
	handler.gate.RequestApproval(gateID, pending.Call.Name)

	injected := errors.New("injected paused gate cancellation event failure")
	injectedOnce := false
	handler.store.beforeRecordEventHook = func(event map[string]any) error {
		if !injectedOnce && stringField(event, "kind") == "approval_resolved" && stringField(event, "approvalId") == gateID {
			injectedOnce = true
			return injected
		}
		return nil
	}
	if _, err := handler.runtimeThreadService().Patch(context.Background(), threadID, map[string]any{"workspace": workspaceB}); !errors.Is(err, injected) {
		t.Fatalf("first partial gate settlement did not block mutation: %v", err)
	}
	if !injectedOnce {
		t.Fatal("gate cancellation fault was not reached")
	}
	if _, _, exists, hasPending := handler.gates.PeekApproval(gateID); !exists || hasPending {
		t.Fatalf("failed settlement was exposed as executable pending: exists=%v pending=%v", exists, hasPending)
	}
	if claims := handler.gates.DrainForThread(threadID); len(claims) != 1 || claims[0].ID != gateID {
		t.Fatalf("failed settlement claim was not retained for host retry: %#v", claims)
	}
	lateApproval, err := handler.approveRuntimeTool(context.Background(), controlapp.ApprovalDecision{ApprovalID: gateID, Decision: "allow"})
	if err != nil || lateApproval.StatusCode != 409 || lateApproval.Body["code"] != "gate_continuation_unavailable" {
		t.Fatalf("partially settled claim accepted user retry: result=%#v err=%v", lateApproval, err)
	}
	beforeRetry, err := handler.store.GetThread(threadID)
	if err != nil || stringField(beforeRetry, "workspace") == workspaceB {
		t.Fatalf("failed gate settlement advanced workspace: workspace=%q err=%v", stringField(beforeRetry, "workspace"), err)
	}
	handler.store.beforeRecordEventHook = nil
	patched, err := handler.runtimeThreadService().Patch(context.Background(), threadID, map[string]any{"workspace": workspaceB})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, exists, hasPending := handler.gates.PeekApproval(gateID); !exists || hasPending {
		t.Fatalf("paused approval remained executable: exists=%v pending=%v", exists, hasPending)
	}
	_, disposition, err := handler.continuations.ResolveTrustedDisposition(context.Background(), gateID)
	if err != nil || disposition.Status != domaincontinuation.StatusInterrupted || disposition.ReasonCode != "security_context_transition" {
		t.Fatalf("gate disposition = %#v err=%v", disposition, err)
	}
	realWorkspaceB, err := filepath.EvalSymlinks(workspaceB)
	if err != nil || stringField(patched, "workspace") != realWorkspaceB {
		t.Fatalf("workspace mutation did not commit after gate closure: workspace=%q err=%v", stringField(patched, "workspace"), err)
	}
	replay, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	resolvedSeq, terminalSeq := 0, 0
	for _, event := range replay.Events {
		if stringField(event, "kind") == "approval_resolved" && stringField(event, "approvalId") == gateID {
			resolvedSeq = transitionGateEventSeq(event["seq"])
		}
		if stringField(event, "kind") == "turn_aborted" && stringField(event, "turnId") == pending.TurnID {
			terminalSeq = transitionGateEventSeq(event["seq"])
		}
	}
	if resolvedSeq <= 0 || terminalSeq <= resolvedSeq {
		t.Fatalf("gate/terminal ordering invalid: resolved=%d terminal=%d events=%#v", resolvedSeq, terminalSeq, replay.Events)
	}
	if patched["securityState"] != nil || patched["contextEpochState"] != nil {
		t.Fatalf("workspace patch exposed private authority in its public response: %#v", patched)
	}
	committed, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	latest, found, err := turnsecurityapp.LatestContext(committed)
	if err != nil || !found || latest.ContextEpoch <= securityContext.ContextEpoch || latest.WorkspaceRealPath != realWorkspaceB {
		t.Fatalf("epoch committed before exact gate closure authority: latest=%#v found=%v err=%v", latest, found, err)
	}
}

func TestStartTurnClosesOldPausedApprovalBeforeNewEpochAdmission(t *testing.T) {
	workspace := workspacetest.New(t)
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "start-provider", BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: "start-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	provider := &countingImmediateWorkspaceProvider{}
	handler.provider = provider
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "paused gate before next turn", "workspace": workspace,
		"providerId": "start-provider", "model": "start-model",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	now := time.Now().UTC()
	securityContext, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: context.Background(), Authority: handler.turnSecurity, Reader: filestore.CaseBindingReader{}, Thread: thread,
		ThreadID: threadID, TurnID: "turn-paused-before-start", Workspace: workspace,
		Principal: testIdentityPrincipal(), IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	pending := pausedApprovalForTransition(t, securityContext, now)
	gateID := controlapp.SecureGateID(
		domaincontinuation.KindApproval, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest,
		pending.ExecutionGrant.GrantID, pending.Call.ID,
	)
	itemID := "item_" + gateID
	receipt, err := handler.continuations.IssuePendingHost(domaincontinuation.KindApproval, gateID, itemID, pending, now)
	if err != nil {
		t.Fatal(err)
	}
	item, requestedEvent := appturn.ApprovalRequestRecords(appturn.ApprovalRequestInput{
		ThreadID: pending.ThreadID, TurnID: pending.TurnID, ItemID: itemID, ApprovalID: gateID,
		CreatedAt: now.Format(time.RFC3339Nano), ToolName: pending.Call.Name,
		ApprovalPolicy: pending.ApprovalPolicy, SandboxMode: pending.SandboxMode, ContinuationReceiptID: receipt.ReceiptID,
	})
	epochState, err := contextepochapp.BootstrapState(
		threadID, securityContext.ContextEpoch,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(securityContext)}, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := handler.store.AppendTurnToThread(threadID, map[string]any{
		"id": pending.TurnID, "threadId": threadID, "status": "running", "createdAt": now.Format(time.RFC3339Nano),
		"securityContext": securityRecord, "items": []any{item},
	}, "start-provider", map[string]any{
		"securityState": securityRecord, "contextEpochState": contextepochapp.PublicState(epochState),
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := handler.store.RecordEvent(requestedEvent); err != nil {
		t.Fatal(err)
	}
	record := controlapp.GateRecord{
		ThreadID: threadID, TurnID: pending.TurnID, ItemID: itemID,
		ToolName: pending.Call.Name, ContinuationReceiptID: receipt.ReceiptID,
	}
	if !handler.gates.RegisterPendingApproval(gateID, record, pending) {
		t.Fatal("register paused approval")
	}
	handler.gate.RequestApproval(gateID, pending.Call.Name)

	settlementBlocked := make(chan struct{})
	settlementRelease := make(chan struct{})
	settlementReleased := false
	defer func() {
		if !settlementReleased {
			close(settlementRelease)
		}
	}()
	handler.store.beforeRecordEventHook = func(event map[string]any) error {
		if stringField(event, "kind") == "approval_resolved" && stringField(event, "approvalId") == gateID {
			close(settlementBlocked)
			<-settlementRelease
		}
		return nil
	}
	type startResult struct {
		response map[string]any
		err      error
	}
	startDone := make(chan startResult, 1)
	go func() {
		response, startErr := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
			Prompt:     "start only after the previous paused authority is closed",
			ProviderID: "start-provider", Model: "start-model",
		})
		startDone <- startResult{response: response, err: startErr}
	}()
	select {
	case <-settlementBlocked:
	case <-time.After(serverPositiveTestTimeout):
		t.Fatal("new turn did not block on old approval settlement")
	}
	if provider.calls.Load() != 0 {
		t.Fatalf("provider dispatched before old approval settlement: %d", provider.calls.Load())
	}
	select {
	case result := <-startDone:
		t.Fatalf("new turn returned before old approval terminal settlement: response=%#v err=%v", result.response, result.err)
	default:
	}
	lateDuringSettlement, err := handler.approveRuntimeTool(context.Background(), controlapp.ApprovalDecision{
		ApprovalID: gateID, Decision: "allow",
	})
	if err != nil || lateDuringSettlement.StatusCode != 409 || lateDuringSettlement.Body["code"] != "gate_continuation_unavailable" {
		t.Fatalf("terminal-claimed approval was executable during start barrier: result=%#v err=%v", lateDuringSettlement, err)
	}
	close(settlementRelease)
	settlementReleased = true
	var started map[string]any
	select {
	case result := <-startDone:
		if result.err != nil {
			t.Fatal(result.err)
		}
		started = result.response
	case <-time.After(serverPositiveTestTimeout):
		t.Fatal("new turn did not continue after old approval settlement")
	}
	handler.store.beforeRecordEventHook = nil
	if provider.calls.Load() != 1 {
		t.Fatalf("provider was dispatched before or more than once after gate quiescence: %d", provider.calls.Load())
	}
	newTurnID := stringField(started, "turnId")
	if newTurnID == "" || newTurnID == pending.TurnID {
		t.Fatalf("new turn identity is invalid: response=%#v", started)
	}
	if _, _, exists, hasPending := handler.gates.PeekApproval(gateID); !exists || hasPending {
		t.Fatalf("old approval remained executable after new turn admission: exists=%v pending=%v", exists, hasPending)
	}
	lateAfterStart, err := handler.approveRuntimeTool(context.Background(), controlapp.ApprovalDecision{
		ApprovalID: gateID, Decision: "allow",
	})
	if err != nil || lateAfterStart.StatusCode != 409 || lateAfterStart.Body["code"] != "gate_continuation_unavailable" {
		t.Fatalf("settled approval was executable after new turn admission: result=%#v err=%v", lateAfterStart, err)
	}
	if claims := handler.gates.DrainForThread(threadID); len(claims) != 0 {
		t.Fatalf("old approval terminal claim was not committed before new turn admission: %#v", claims)
	}
	_, disposition, err := handler.continuations.ResolveTrustedDisposition(context.Background(), gateID)
	if err != nil || disposition.Status != domaincontinuation.StatusInterrupted || disposition.ReasonCode != "turn_start_security_transition" {
		t.Fatalf("old gate disposition = %#v err=%v", disposition, err)
	}

	reloaded, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	oldStatus, newStatus := "", ""
	var newContext domainsecurity.TurnSecurityContext
	for _, rawTurn := range listAny(reloaded["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		switch stringField(turn, "id") {
		case pending.TurnID:
			oldStatus = stringField(turn, "status")
		case newTurnID:
			newStatus = stringField(turn, "status")
			newContext, err = domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if oldStatus != "aborted" || newStatus != "completed" {
		t.Fatalf("old/new turn terminal states are invalid: old=%q new=%q thread=%#v", oldStatus, newStatus, reloaded)
	}
	if newContext.ContextEpoch <= securityContext.ContextEpoch || newContext.ContextDigest == securityContext.ContextDigest {
		t.Fatalf("new turn reused the paused gate epoch: old=%#v new=%#v", securityContext, newContext)
	}
	realWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if newContext.ThreadID != threadID || newContext.TurnID != newTurnID || newContext.WorkspaceRealPath != realWorkspace {
		t.Fatalf("new turn froze the wrong post-quiescence scope: context=%#v workspace=%q", newContext, realWorkspace)
	}
	currentEpoch, ok, err := contextepochapp.StateFromThread(reloaded)
	if err != nil || !ok || currentEpoch.AcceptedSnapshot.Epoch != newContext.ContextEpoch {
		t.Fatalf("new turn and durable epoch authority diverged: state=%#v ok=%v context=%#v err=%v", currentEpoch, ok, newContext, err)
	}

	replay, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	resolvedSeq, oldTerminalSeq, newStartedSeq := 0, 0, 0
	for _, event := range replay.Events {
		switch stringField(event, "kind") {
		case "approval_resolved":
			if stringField(event, "approvalId") == gateID {
				resolvedSeq = transitionGateEventSeq(event["seq"])
			}
		case "turn_aborted":
			if stringField(event, "turnId") == pending.TurnID {
				oldTerminalSeq = transitionGateEventSeq(event["seq"])
			}
		case "turn_started":
			if stringField(event, "turnId") == newTurnID {
				newStartedSeq = transitionGateEventSeq(event["seq"])
			}
		}
	}
	if resolvedSeq <= 0 || oldTerminalSeq <= resolvedSeq || newStartedSeq <= oldTerminalSeq {
		t.Fatalf(
			"gate settlement/new admission ordering invalid: resolved=%d oldTerminal=%d newStarted=%d events=%#v",
			resolvedSeq, oldTerminalSeq, newStartedSeq, replay.Events,
		)
	}
}

func TestStartTurnWaitsForSameThreadBackgroundCompletionTailBeforeBaseline(t *testing.T) {
	workspace := workspacetest.New(t)
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "background-tail-provider", BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: "background-tail-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	provider := &countingImmediateWorkspaceProvider{}
	handler.provider = provider
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "before background completion tail", "workspace": workspace,
		"providerId": "background-tail-provider", "model": "background-tail-model",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	now := time.Now().UTC()
	current, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: context.Background(), Authority: handler.turnSecurity, Reader: filestore.CaseBindingReader{}, Thread: thread,
		ThreadID: threadID, TurnID: "turn-before-background-tail", Workspace: workspace,
		Principal: testIdentityPrincipal(), IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	epochState, err := contextepochapp.BootstrapState(
		threadID, current.ContextEpoch,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(current)}, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	securityRecord := turnsecurityapp.PublicRecord(current)
	if err := handler.store.AppendTurnToThread(threadID, map[string]any{
		"id": current.TurnID, "threadId": threadID, "status": "running", "createdAt": now.Format(time.RFC3339Nano),
		"securityContext": securityRecord, "items": []any{},
	}, "background-tail-provider", map[string]any{
		"securityState": securityRecord, "contextEpochState": contextepochapp.PublicState(epochState),
	}); err != nil {
		t.Fatal(err)
	}
	finishedAt := now.Add(time.Second).Format(time.RFC3339Nano)
	if result, err := appturn.CommitCompletedTurn(appturn.CommitCompletionInput{
		Store: handler.store, SecurityContext: current, ThreadID: threadID, TurnID: current.TurnID,
		Model: "background-tail-model", CreatedAt: finishedAt, FinishedAt: finishedAt,
	}); err != nil || !result.Changed {
		t.Fatalf("commit previous turn: result=%#v err=%v", result, err)
	}
	state := handler.runtimeSubagentState()
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), current, time.Second); err != nil {
		t.Fatal(err)
	}
	binding := compactionJobBinding(t, current)
	jobCtx, cancelJob := context.WithCancel(context.Background())
	const jobID = "job-background-completion-tail"
	if !state.RegisterBoundBackgroundJob(jobID, binding, cancelJob) {
		t.Fatal("background completion control registration failed")
	}
	replacementCtx, cancelReplacement := context.WithCancel(context.Background())
	const replacementJobID = "job-background-completion-tail-late-admission"
	defer func() {
		cancelJob()
		state.UnregisterBackgroundJob(jobID)
		cancelReplacement()
		state.UnregisterBackgroundJob(replacementJobID)
	}()
	tailDone := make(chan error, 1)
	replacementTailDone := make(chan error, 1)
	go func() {
		<-jobCtx.Done()
		_, tailErr := handler.store.PatchThread(threadID, map[string]any{"title": "background completion tail settled"})
		if tailErr == nil && !state.RegisterBoundBackgroundJob(replacementJobID, binding, cancelReplacement) {
			tailErr = errors.New("late background completion control registration failed")
		}
		state.UnregisterBackgroundJob(jobID)
		tailDone <- tailErr
	}()
	go func() {
		<-replacementCtx.Done()
		_, tailErr := handler.store.PatchThread(threadID, map[string]any{"title": "late background completion tail settled"})
		state.UnregisterBackgroundJob(replacementJobID)
		replacementTailDone <- tailErr
	}()

	started, startErr := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
		Prompt:     "start only after the old background completion tail is durable",
		ProviderID: "background-tail-provider", Model: "background-tail-model",
	})
	select {
	case tailErr := <-tailDone:
		if tailErr != nil {
			t.Fatalf("background completion tail mutation failed: %v", tailErr)
		}
	case <-time.After(serverPositiveTestTimeout):
		t.Fatal("new turn did not cancel and wait for the old background completion tail")
	}
	select {
	case tailErr := <-replacementTailDone:
		if tailErr != nil {
			t.Fatalf("late background completion tail mutation failed: %v", tailErr)
		}
	case <-time.After(serverPositiveTestTimeout):
		t.Fatal("new turn did not close a background control admitted during the barrier window")
	}
	if startErr != nil {
		t.Fatalf("new turn conflicted with the acknowledged background completion tail: %v", startErr)
	}
	if stringField(started, "turnId") == "" || provider.calls.Load() != 1 {
		t.Fatalf("new turn did not execute exactly once after background settlement: response=%#v providerCalls=%d", started, provider.calls.Load())
	}
	reloaded, err := handler.store.GetThread(threadID)
	if err != nil || stringField(reloaded, "title") != "late background completion tail settled" {
		t.Fatalf("new turn did not retain the pre-baseline background tail: thread=%#v err=%v", reloaded, err)
	}
}

func transitionGateEventSeq(value any) int {
	switch number := value.(type) {
	case int:
		return number
	case int64:
		return int(number)
	case float64:
		return int(number)
	default:
		return 0
	}
}

func pausedApprovalForTransition(t *testing.T, securityContext domainsecurity.TurnSecurityContext, now time.Time) appmodel.PendingToolCall {
	t.Helper()
	call := domainmodel.ToolCall{ID: serverTestHostToolCallID("call-paused-approval"), Name: "write_file", Arguments: json.RawMessage(`{"path":"a.txt"}`)}
	scope := []string{call.Name}
	scopeBody, _ := json.Marshal(scope)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-a", ServerIdentity: "host:builtin", ToolName: call.Name,
		ToolCallID: call.ID, ArgsHash: domainsecurity.CanonicalJSONHash(call.Arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("schema-paused-approval")), ScopeHash: domainsecurity.SHA256Hex(scopeBody),
		ReadOnly: false, ApprovalState: "pending", IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	providerConfig := domainmodel.TurnConfig{
		ProviderID: "provider-a", Model: "model-a", APIKey: "test-only", BaseURL: "https://provider.invalid",
		EndpointFormat: "chat_completions",
	}
	return appmodel.PendingToolCall{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: securityContext.WorkspaceRealPath,
		ProviderConfig: providerConfig, ProviderID: providerConfig.ProviderID, Model: providerConfig.Model, Prompt: "continue",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", Call: call,
		LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true, ProviderStepExact: true,
		CaseSourceUnavailable: false, OrdinaryResultInputIsolated: true,
		ToolCallItemID: domaintoolcall.ToolCallItemIDV1(securityContext.TurnID, call.ID),
		ToolScope:      scope, SecurityContext: securityContext, ExecutionGrant: grant,
		ProviderNamespace: domaincontinuation.NewProviderContinuationNamespaceV1("turn", "", 1, nil),
	}
}

type pausedTransitionApprovalFixture struct {
	GateID          string
	ItemID          string
	Pending         appmodel.PendingToolCall
	SecurityContext domainsecurity.TurnSecurityContext
}

func installPausedTransitionApproval(
	t *testing.T,
	handler *runtimeServerHandler,
	thread map[string]any,
	workspace, turnID, providerID string,
) pausedTransitionApprovalFixture {
	t.Helper()
	threadID := stringField(thread, "id")
	now := time.Now().UTC()
	securityContext, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: context.Background(), Authority: handler.turnSecurity, Reader: filestore.CaseBindingReader{}, Thread: thread,
		ThreadID: threadID, TurnID: turnID, Workspace: workspace,
		Principal: testIdentityPrincipal(), IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	pending := pausedApprovalForTransition(t, securityContext, now)
	gateID := controlapp.SecureGateID(
		domaincontinuation.KindApproval, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest,
		pending.ExecutionGrant.GrantID, pending.Call.ID,
	)
	itemID := "item_" + gateID
	receipt, err := handler.continuations.IssuePendingHost(domaincontinuation.KindApproval, gateID, itemID, pending, now)
	if err != nil {
		t.Fatal(err)
	}
	item, requestedEvent := appturn.ApprovalRequestRecords(appturn.ApprovalRequestInput{
		ThreadID: pending.ThreadID, TurnID: pending.TurnID, ItemID: itemID, ApprovalID: gateID,
		CreatedAt: now.Format(time.RFC3339Nano), ToolName: pending.Call.Name,
		ApprovalPolicy: pending.ApprovalPolicy, SandboxMode: pending.SandboxMode, ContinuationReceiptID: receipt.ReceiptID,
	})
	epochState, err := contextepochapp.BootstrapState(
		threadID, securityContext.ContextEpoch,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(securityContext)}, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := handler.store.AppendTurnToThread(threadID, map[string]any{
		"id": pending.TurnID, "threadId": threadID, "status": "running", "createdAt": now.Format(time.RFC3339Nano),
		"securityContext": securityRecord, "items": []any{item},
	}, providerID, map[string]any{
		"securityState": securityRecord, "contextEpochState": contextepochapp.PublicState(epochState),
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := handler.store.RecordEvent(requestedEvent); err != nil {
		t.Fatal(err)
	}
	record := controlapp.GateRecord{
		ThreadID: threadID, TurnID: pending.TurnID, ItemID: itemID,
		ToolName: pending.Call.Name, ContinuationReceiptID: receipt.ReceiptID,
	}
	if !handler.gates.RegisterPendingApproval(gateID, record, pending) {
		t.Fatal("register paused approval")
	}
	handler.gate.RequestApproval(gateID, pending.Call.Name)
	return pausedTransitionApprovalFixture{
		GateID: gateID, ItemID: itemID, Pending: pending, SecurityContext: securityContext,
	}
}
