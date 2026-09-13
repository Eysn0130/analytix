package server

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	controlapp "analytix.local/runtime-go/internal/app/control"
	gatecontinuationapp "analytix.local/runtime-go/internal/app/gatecontinuation"
	appmodel "analytix.local/runtime-go/internal/app/model"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestRestartReconcilesSignedGateResolutionWithoutContinuation(t *testing.T) {
	tests := []struct {
		name, kind, dispositionStatus, reason, publicStatus string
	}{
		{"approval_allowed", domaincontinuation.KindApproval, domaincontinuation.StatusAllowed, "approval_allowed", "allowed"},
		{"approval_denied", domaincontinuation.KindApproval, domaincontinuation.StatusDenied, "approval_denied", "denied"},
		{"input_submitted", domaincontinuation.KindUserInput, domaincontinuation.StatusSubmitted, "user_input_submitted", "submitted"},
		{"input_cancelled", domaincontinuation.KindUserInput, domaincontinuation.StatusCancelled, "user_input_cancelled", "cancelled"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			workspace := workspacetest.New(t)
			handler := NewRuntimeServerHandler(RuntimeServerConfig{
				RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
				ProviderID: "provider-a", BaseURL: "https://provider.invalid", APIKey: "test-only",
				Model: "model-a", EndpointFormat: "chat_completions",
			}).(*runtimeServerHandler)
			configureServerGeneralExecution(t, handler)
			provider := &countingImmediateWorkspaceProvider{}
			handler.provider = provider
			thread, err := handler.store.CreateThread(map[string]any{
				"title": "signed gate crash " + test.name, "workspace": workspace,
				"providerId": "provider-a", "model": "model-a",
			}, workspace)
			if err != nil {
				t.Fatal(err)
			}
			var fixture pausedTransitionApprovalFixture
			if test.kind == domaincontinuation.KindApproval {
				fixture = installPausedTransitionApproval(t, handler, thread, workspace, "turn-"+test.name, "provider-a")
			} else {
				fixture = installPausedRestartUserInput(t, handler, thread, workspace, "turn-"+test.name, "provider-a")
			}
			if err := handler.continuations.DisposePendingHost(
				fixture.GateID, test.dispositionStatus, test.reason, time.Now().UTC(), fixture.Pending,
			); err != nil {
				t.Fatal(err)
			}

			// Simulate a crash after the immutable private disposition but before
			// any public resolution projection or continuation side effect.
			handler.gates = controlapp.NewGateRegistry[runtimePendingToolCall]()
			handler.gate = controlapp.NewApprovalUserInputManager()
			injected := errors.New("injected signed resolution event failure")
			injectedOnce := false
			handler.store.beforeRecordEventHook = func(event map[string]any) error {
				if !injectedOnce && stringField(event, "kind") == restartedResolutionEventKind(test.kind) &&
					restartedResolutionEventID(test.kind, event) == fixture.GateID {
					injectedOnce = true
					return injected
				}
				return nil
			}
			if err := handler.restoreRuntimeState(); !errors.Is(err, injected) {
				t.Fatalf("first restart did not retain the projection outbox: %v", err)
			}
			assertRestartGateTurnStillActiveWithoutTerminal(t, handler, stringField(thread, "id"), fixture.Pending.TurnID)
			_, disposition, err := handler.continuations.ResolveTrustedDisposition(context.Background(), fixture.GateID)
			if err != nil || disposition.Status != test.dispositionStatus || disposition.ReasonCode != test.reason {
				t.Fatalf("signed disposition changed during failed reconciliation: %#v err=%v", disposition, err)
			}
			if provider.calls.Load() != 0 {
				t.Fatalf("restart invoked provider continuation: calls=%d", provider.calls.Load())
			}
			if _, err := os.Stat(filepath.Join(workspace, "a.txt")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("restart re-executed a tool side effect: %v", err)
			}

			handler.store.beforeRecordEventHook = nil
			if err := handler.restoreRuntimeState(); err != nil {
				t.Fatalf("projection retry failed: %v", err)
			}
			assertRestartResolutionProjection(
				t, handler, stringField(thread, "id"), fixture, test.kind, test.publicStatus,
			)
			if provider.calls.Load() != 0 {
				t.Fatalf("projection retry invoked provider continuation: calls=%d", provider.calls.Load())
			}
		})
	}
}

func TestRestartDoesNotPublishTerminalBeforeGateCancellationEvent(t *testing.T) {
	workspace := workspacetest.New(t)
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "provider-a", BaseURL: "https://provider.invalid", APIKey: "test-only",
		Model: "model-a", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "crashed paused approval", "workspace": workspace,
		"providerId": "provider-a", "model": "model-a",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	fixture := installPausedTransitionApproval(
		t, handler, thread, workspace, "turn-crashed-paused-approval", "provider-a",
	)

	// Simulate process memory loss without invoking Shutdown: only durable
	// thread/events and the signed continuation store survive.
	handler.gates = controlapp.NewGateRegistry[runtimePendingToolCall]()
	handler.gate = controlapp.NewApprovalUserInputManager()
	injected := errors.New("injected restarted gate cancellation event failure")
	injectedOnce := false
	handler.store.beforeRecordEventHook = func(event map[string]any) error {
		if !injectedOnce && stringField(event, "kind") == "approval_resolved" && stringField(event, "approvalId") == fixture.GateID {
			injectedOnce = true
			return injected
		}
		return nil
	}
	if err := handler.restoreRuntimeState(); !errors.Is(err, injected) {
		t.Fatalf("restart did not fail closed on the missing gate cancellation event: %v", err)
	}
	if !injectedOnce {
		t.Fatal("restart reconciliation did not reach the gate cancellation event")
	}
	_, disposition, err := handler.continuations.ResolveTrustedDisposition(context.Background(), fixture.GateID)
	if err != nil || disposition.Status != domaincontinuation.StatusRestartInvalid {
		t.Fatalf("restart did not retain its signed non-resumable disposition: %#v err=%v", disposition, err)
	}
	assertRestartGateTurnStillActiveWithoutTerminal(t, handler, stringField(thread, "id"), fixture.Pending.TurnID)

	handler.store.beforeRecordEventHook = nil
	if err := handler.restoreRuntimeState(); err != nil {
		t.Fatalf("restart reconciliation retry failed: %v", err)
	}
	reloaded, err := handler.store.GetThread(stringField(thread, "id"))
	if err != nil {
		t.Fatal(err)
	}
	status := ""
	for _, rawTurn := range listAny(reloaded["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		if stringField(turn, "id") == fixture.Pending.TurnID {
			status = stringField(turn, "status")
			break
		}
	}
	if status != "aborted" {
		t.Fatalf("restart retry did not close the stale turn through the fixed terminal gate: status=%q", status)
	}
	lateApproval, err := handler.approveRuntimeTool(context.Background(), controlapp.ApprovalDecision{
		ApprovalID: fixture.GateID, Decision: "allow",
	})
	if err != nil || lateApproval.StatusCode != 409 || lateApproval.Body["code"] != "gate_continuation_unavailable" {
		t.Fatalf("restarted gate became executable: result=%#v err=%v", lateApproval, err)
	}
	replay, err := handler.store.LoadEventsSince(stringField(thread, "id"), 0)
	if err != nil {
		t.Fatal(err)
	}
	resolvedSeq, terminalSeq := 0, 0
	for _, event := range replay.Events {
		if stringField(event, "kind") == "approval_resolved" && stringField(event, "approvalId") == fixture.GateID {
			resolvedSeq = transitionGateEventSeq(event["seq"])
		}
		if stringField(event, "kind") == "turn_aborted" && stringField(event, "turnId") == fixture.Pending.TurnID {
			terminalSeq = transitionGateEventSeq(event["seq"])
		}
	}
	if resolvedSeq <= 0 || terminalSeq <= resolvedSeq {
		t.Fatalf("restart published terminal before gate cancellation: resolved=%d terminal=%d events=%#v", resolvedSeq, terminalSeq, replay.Events)
	}
}

func assertRestartGateTurnStillActiveWithoutTerminal(
	t *testing.T,
	handler *runtimeServerHandler,
	threadID, turnID string,
) {
	t.Helper()
	thread, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	status := ""
	for _, rawTurn := range listAny(thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		if stringField(turn, "id") == turnID {
			status = stringField(turn, "status")
			break
		}
	}
	if status != "running" {
		t.Fatalf("restart published a terminal before gate reconciliation: status=%q thread=%#v", status, thread)
	}
	replay, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range replay.Events {
		if stringField(event, "turnId") == turnID {
			switch stringField(event, "kind") {
			case "turn_aborted", "turn_failed", "turn_completed":
				t.Fatalf("restart emitted terminal before gate reconciliation: %#v", event)
			}
		}
	}
}

func restartedResolutionEventKind(kind string) string {
	if kind == domaincontinuation.KindApproval {
		return "approval_resolved"
	}
	return "user_input_resolved"
}

func restartedResolutionEventID(kind string, event map[string]any) string {
	if kind == domaincontinuation.KindApproval {
		return stringField(event, "approvalId")
	}
	return stringField(event, "inputId")
}

func assertRestartResolutionProjection(
	t *testing.T,
	handler *runtimeServerHandler,
	threadID string,
	fixture pausedTransitionApprovalFixture,
	kind, publicStatus string,
) {
	t.Helper()
	thread, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turnStatus, itemStatus := "", ""
	for _, rawTurn := range listAny(thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		if stringField(turn, "id") != fixture.Pending.TurnID {
			continue
		}
		turnStatus = stringField(turn, "status")
		for _, rawItem := range listAny(turn["items"]) {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "id") == fixture.ItemID {
				itemStatus = stringField(item, "status")
			}
		}
	}
	if turnStatus != "aborted" || itemStatus != publicStatus {
		t.Fatalf("restart projection mismatch: turn=%q item=%q thread=%#v", turnStatus, itemStatus, thread)
	}
	replay, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	resolutionCount, resolutionSeq, terminalSeq := 0, 0, 0
	for _, event := range replay.Events {
		if stringField(event, "kind") == restartedResolutionEventKind(kind) &&
			restartedResolutionEventID(kind, event) == fixture.GateID {
			resolutionCount++
			if stringField(event, "status") != publicStatus {
				t.Fatalf("restart emitted the wrong public resolution: %#v", event)
			}
			resolutionSeq = transitionGateEventSeq(event["seq"])
		}
		if stringField(event, "kind") == "turn_aborted" && stringField(event, "turnId") == fixture.Pending.TurnID {
			terminalSeq = transitionGateEventSeq(event["seq"])
		}
	}
	if resolutionCount != 1 || resolutionSeq <= 0 || terminalSeq <= resolutionSeq {
		t.Fatalf("restart resolution ordering mismatch: count=%d resolution=%d terminal=%d events=%#v", resolutionCount, resolutionSeq, terminalSeq, replay.Events)
	}
	var late controlapp.ActionResult
	if kind == domaincontinuation.KindApproval {
		late, err = handler.approveRuntimeTool(context.Background(), controlapp.ApprovalDecision{ApprovalID: fixture.GateID, Decision: "allow"})
	} else {
		late, err = handler.respondRuntimeUserInput(context.Background(), controlapp.UserInputResponse{InputID: fixture.GateID, Answers: []map[string]string{{"id": "confirm", "value": "yes"}}})
	}
	if err != nil || late.StatusCode != 409 || late.Body["code"] != "gate_continuation_unavailable" {
		t.Fatalf("reconciled gate became executable: result=%#v err=%v", late, err)
	}
	if err := handler.restoreRuntimeState(); err != nil {
		t.Fatalf("closed signed projection failed restart audit: %v", err)
	}
}

func installPausedRestartUserInput(
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
	call := domainmodel.ToolCall{
		ID: serverTestHostToolCallID("call-paused-input"), Name: "request_user_input",
		Arguments: json.RawMessage(`{"questions":[{"id":"confirm","question":"Continue?"}]}`),
	}
	scope := []string{call.Name}
	scopeBody, _ := json.Marshal(scope)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: providerID, ServerIdentity: "host:builtin", ToolName: call.Name,
		ToolCallID: call.ID, ArgsHash: domainsecurity.CanonicalJSONHash(call.Arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("schema-paused-input")), ScopeHash: domainsecurity.SHA256Hex(scopeBody),
		ReadOnly: true, ApprovalState: "not_required", IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	providerConfig := domainmodel.TurnConfig{
		ProviderID: providerID, Model: "model-a", APIKey: "test-only", BaseURL: "https://provider.invalid",
		EndpointFormat: "chat_completions",
	}
	pending := appmodel.PendingToolCall{
		ThreadID: threadID, TurnID: turnID, Workspace: securityContext.WorkspaceRealPath,
		ProviderConfig: providerConfig, ProviderID: providerConfig.ProviderID, Model: providerConfig.Model, Prompt: "continue",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", Call: call,
		LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true, ProviderStepExact: true,
		CaseSourceUnavailable: false, OrdinaryResultInputIsolated: true,
		ToolCallItemID: domaintoolcall.ToolCallItemIDV1(turnID, call.ID),
		ToolScope:      scope, SecurityContext: securityContext, ExecutionGrant: grant,
		ProviderNamespace: domaincontinuation.NewProviderContinuationNamespaceV1("turn", "", 1, nil),
	}
	gateID := controlapp.SecureGateID(
		domaincontinuation.KindUserInput, threadID, turnID, securityContext.ContextDigest, grant.GrantID, call.ID,
	)
	itemID := "item_" + gateID
	receipt, err := handler.continuations.IssuePendingHost(domaincontinuation.KindUserInput, gateID, itemID, pending, now)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := gatecontinuationapp.ProjectVerifiedGateRequestV1(receipt)
	if err != nil {
		t.Fatal(err)
	}
	item, requestedEvent := projection.Item, projection.Event
	epochState, err := contextepochapp.BootstrapState(
		threadID, securityContext.ContextEpoch,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(securityContext)}, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := handler.store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "createdAt": now.Format(time.RFC3339Nano),
		"securityContext": securityRecord, "items": []any{item},
	}, providerID, map[string]any{
		"securityState": securityRecord, "contextEpochState": contextepochapp.PublicState(epochState),
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := handler.store.RecordEvent(requestedEvent); err != nil {
		t.Fatal(err)
	}
	record := projection.Record
	if !handler.gates.RegisterPendingUserInput(gateID, record, pending) {
		t.Fatal("register paused user input")
	}
	handler.gate.RequestUserInput(gateID, projection.ManagerLabel)
	return pausedTransitionApprovalFixture{
		GateID: gateID, ItemID: itemID, Pending: pending, SecurityContext: securityContext,
	}
}
