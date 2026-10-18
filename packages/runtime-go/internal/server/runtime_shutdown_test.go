package server

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	controlapp "analytix.local/runtime-go/internal/app/control"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	gatecontinuationapp "analytix.local/runtime-go/internal/app/gatecontinuation"
	nativecomponentapp "analytix.local/runtime-go/internal/app/nativecomponent"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	appturn "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	providerpkg "analytix.local/runtime-go/internal/provider"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestRuntimeServerShutdownCancelsActiveExecutionAndDisconnectsMCP(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        t.TempDir(),
	}).(*runtimeServerHandler)
	mcp := &shutdownMCPStub{disconnected: make(chan struct{})}
	handler.mcp = mcp

	turnCtx, cancelTurn := context.WithCancel(context.Background())
	defer cancelTurn()
	handler.runtimeControl().RegisterTurnCancel("thr_shutdown", "turn_shutdown", cancelTurn)
	backgroundCtx, cancelBackground := context.WithCancel(context.Background())
	defer cancelBackground()
	handler.runtimeSubagentState().RegisterBackgroundJob("job_shutdown", cancelBackground)
	backgroundStopped := make(chan struct{})
	go func() {
		<-backgroundCtx.Done()
		handler.runtimeSubagentState().UnregisterBackgroundJob("job_shutdown")
		close(backgroundStopped)
	}()

	if err := handler.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown returned error: %v", err)
	}
	select {
	case <-turnCtx.Done():
	case <-time.After(2 * time.Second):
		t.Fatalf("shutdown did not cancel active turn")
	}
	select {
	case <-backgroundStopped:
	case <-time.After(2 * time.Second):
		t.Fatalf("shutdown did not wait for background job cleanup")
	}
	select {
	case <-mcp.disconnected:
	case <-time.After(2 * time.Second):
		t.Fatalf("shutdown did not disconnect MCP manager")
	}
}

func TestRuntimeServerShutdownRetriesPausedApprovalTerminalClaim(t *testing.T) {
	workspace := workspacetest.New(t)
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), Host: "127.0.0.1", DataDir: t.TempDir(),
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "shutdown paused approval", "workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	fixture := installPausedTransitionApproval(
		t, handler, thread, workspace, "turn-shutdown-paused-approval", "shutdown-provider",
	)

	injected := errors.New("injected shutdown approval cancellation event failure")
	injectedOnce := false
	handler.store.beforeRecordEventHook = func(event map[string]any) error {
		if !injectedOnce && stringField(event, "kind") == "approval_resolved" && stringField(event, "approvalId") == fixture.GateID {
			injectedOnce = true
			return injected
		}
		return nil
	}
	if err := handler.Shutdown(context.Background()); !errors.Is(err, injected) {
		t.Fatalf("first shutdown did not return the gate settlement blocker: %v", err)
	}
	if !injectedOnce {
		t.Fatal("shutdown did not reach paused gate cancellation persistence")
	}
	if _, _, exists, hasPending := handler.gates.PeekApproval(fixture.GateID); !exists || hasPending {
		t.Fatalf("failed shutdown exposed a partially settled gate: exists=%v pending=%v", exists, hasPending)
	}
	if claims := handler.gates.DrainAll(); len(claims) != 1 || claims[0].ID != fixture.GateID {
		t.Fatalf("failed shutdown did not retain its exact terminal claim: %#v", claims)
	}
	lateApproval, err := handler.approveRuntimeTool(context.Background(), controlapp.ApprovalDecision{
		ApprovalID: fixture.GateID, Decision: "allow",
	})
	if err != nil || lateApproval.StatusCode != 409 || lateApproval.Body["code"] != "gate_continuation_unavailable" {
		t.Fatalf("partially settled shutdown claim accepted user continuation: result=%#v err=%v", lateApproval, err)
	}

	handler.store.beforeRecordEventHook = nil
	if err := handler.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown terminal claim retry failed: %v", err)
	}
	if claims := handler.gates.DrainAll(); len(claims) != 0 {
		t.Fatalf("successful shutdown left a terminal claim: %#v", claims)
	}
	_, disposition, err := handler.continuations.ResolveTrustedDisposition(context.Background(), fixture.GateID)
	if err != nil || disposition.Status != domaincontinuation.StatusInterrupted || disposition.ReasonCode != "runtime_shutdown" {
		t.Fatalf("shutdown gate disposition = %#v err=%v", disposition, err)
	}
	reloaded, err := handler.store.GetThread(stringField(thread, "id"))
	if err != nil {
		t.Fatal(err)
	}
	oldStatus := ""
	for _, rawTurn := range listAny(reloaded["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		if stringField(turn, "id") == fixture.Pending.TurnID {
			oldStatus = stringField(turn, "status")
			break
		}
	}
	if oldStatus != "aborted" {
		t.Fatalf("shutdown did not close paused turn through the fixed terminal gate: status=%q thread=%#v", oldStatus, reloaded)
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
		t.Fatalf("shutdown gate/terminal ordering invalid: resolved=%d terminal=%d events=%#v", resolvedSeq, terminalSeq, replay.Events)
	}
}

func TestRuntimeServerShutdownReturnsBlockerWhenBackgroundJobNeverUnregisters(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), Host: "127.0.0.1", DataDir: t.TempDir(),
	}).(*runtimeServerHandler)
	jobCtx, cancelJob := context.WithCancel(context.Background())
	handler.runtimeSubagentState().RegisterBackgroundJob("job_never_unregisters", cancelJob)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := handler.Shutdown(ctx)
	if !errors.Is(err, subagentapp.ErrBackgroundJobStopTimeout) {
		t.Fatalf("shutdown did not propagate the unstopped-job blocker: %v", err)
	}
	select {
	case <-jobCtx.Done():
	default:
		t.Fatal("shutdown did not request cancellation before returning the blocker")
	}
	if ids := handler.runtimeSubagentState().ActiveBackgroundJobIDs(); len(ids) != 1 || ids[0] != "job_never_unregisters" {
		t.Fatalf("timeout must not unregister an unproven job: %#v", ids)
	}
}

func TestRuntimeServerShutdownWaitsForOwnedFinalizationBeforeDisconnectingMCP(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), Host: "127.0.0.1", DataDir: t.TempDir(),
	}).(*runtimeServerHandler)
	mcp := &shutdownMCPStub{disconnected: make(chan struct{})}
	handler.mcp = mcp
	finishOperation, admitted := handler.runtimeControl().BeginTurnOperation()
	if !admitted {
		t.Fatal("expected owned finalization admission")
	}
	turnCtx, cancelTurn := context.WithCancel(context.Background())
	defer cancelTurn()
	if !handler.runtimeControl().RegisterTurnCancel("thr_shutdown_wait", "turn_shutdown_wait", cancelTurn) {
		t.Fatal("expected active turn registration")
	}
	cancelObserved := make(chan struct{})
	releaseFinalization := make(chan struct{})
	go func() {
		<-turnCtx.Done()
		close(cancelObserved)
		<-releaseFinalization
		handler.runtimeControl().UnregisterTurnCancel("thr_shutdown_wait", "turn_shutdown_wait")
		finishOperation()
	}()
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- handler.Shutdown(context.Background()) }()
	select {
	case <-cancelObserved:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel the owned turn")
	}
	select {
	case err := <-shutdownDone:
		t.Fatalf("shutdown returned before owned finalization completed: %v", err)
	case <-mcp.disconnected:
		t.Fatal("MCP disconnected before owned finalization completed")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseFinalization)
	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("shutdown returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not finish after owned finalization completed")
	}
	select {
	case <-mcp.disconnected:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not disconnect MCP after finalization")
	}
}

func TestRuntimeServerShutdownClosesNativeAfterDrainAndRetriesFailure(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), Host: "127.0.0.1", DataDir: t.TempDir(),
	}).(*runtimeServerHandler)
	lifecycle := &shutdownNativeLifecycle{failures: 1, called: make(chan struct{}, 2)}
	owner := &shutdownNativeOwner{lifecycle: lifecycle}
	ordinaryHealth := nativecomponentapp.LiveAuthorityFunc(func(
		ctx context.Context,
		_ domainsecurity.TurnSecurityContext,
	) error {
		if ctx == nil || ctx.Err() != nil {
			return nativecomponentapp.ErrAuthorityInvalid
		}
		return nil
	})
	nativeAuthority, err := nativecomponentapp.NewReadyRuntimeAuthority(nativecomponentapp.RuntimeAuthorityDependencies{
		Health: nativecomponentapp.HealthDependencies{
			Store: handler.store, DurableAuthority: shutdownNativeDurableAuthority{},
			AcquireEffect: func(
				ctx context.Context,
				_ domainsecurity.TurnSecurityContext,
			) (context.Context, func(), error) {
				return ctx, func() {}, nil
			},
			Now: time.Now,
		},
		LiveAuthority: nativecomponentapp.LiveAuthorityFunc(func(
			context.Context,
			domainsecurity.TurnSecurityContext,
		) error {
			return nil
		}),
		HealthOnlyAuthority: nativecomponentapp.NewHealthOnlyCurrentnessV1(
			ordinaryHealth,
			owner.ValidateCurrentDataEngine,
		),
		Owner: owner,
	})
	if err != nil {
		t.Fatalf("new shutdown native authority: %v", err)
	}
	handler.nativeAuthority = nativeAuthority
	finishOperation, admitted := handler.runtimeControl().BeginTurnOperation()
	if !admitted {
		t.Fatal("expected turn operation admission")
	}
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- handler.Shutdown(context.Background()) }()
	select {
	case <-lifecycle.called:
		t.Fatal("native lifecycle closed before turn operations drained")
	case <-time.After(50 * time.Millisecond):
	}
	finishOperation()
	select {
	case err := <-shutdownDone:
		if err == nil {
			t.Fatal("native close failure was not propagated")
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not finish after turn drain")
	}
	if lifecycle.Calls() != 1 {
		t.Fatalf("native close calls after first shutdown = %d", lifecycle.Calls())
	}
	if err := handler.Shutdown(context.Background()); err != nil {
		t.Fatalf("native close retry failed: %v", err)
	}
	if lifecycle.Calls() != 2 {
		t.Fatalf("native close calls after retry = %d", lifecycle.Calls())
	}
}

func TestRuntimeServerShutdownCancelsBackgroundShellAndMarksJobTerminal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("background shell process-group shutdown uses the POSIX shell path in this contract test")
	}
	workspace := filepath.Join(workspacetest.New(t), "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        t.TempDir(),
	}).(*runtimeServerHandler)
	thread, err := handler.store.CreateThread(map[string]any{
		"id": "thr_shutdown_bg", "title": "Shutdown background shell", "workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_shutdown_bg"
	now := time.Now().UTC()
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		SourceManifestHash: turnsecurityapp.SourceManifestHash(nil), ContextEpoch: 1, IssuedAt: now,
	})
	arguments := map[string]any{
		"command": "sleep 20 & echo $! > child.pid; printf started; wait", "run_in_background": true, "timeout": float64(120),
	}
	argumentBody, err := json.Marshal(arguments)
	if err != nil {
		t.Fatal(err)
	}
	call := providerpkg.ToolCall{ID: serverTestHostToolCallID("call_shutdown_bg"), Name: "bash", Arguments: argumentBody}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "test-provider", ServerIdentity: "host:builtin", ToolName: call.Name,
		ToolCallID: call.ID, ArgsHash: domainsecurity.CanonicalJSONHash(argumentBody), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: false, ApprovalState: "approved", IssuedAt: now,
	})
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := handler.store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "createdAt": now.Format(time.RFC3339Nano),
		"securityContext": securityRecord, "items": []any{},
	}, "test-provider", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.persistToolCallReady(context.Background(), threadID, turnID, call, 0, securityContext, grant); err != nil {
		t.Fatal(err)
	}
	if err := handler.runtimeSubagentState().ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), securityContext, time.Second); err != nil {
		t.Fatal(err)
	}
	pending := runtimePendingToolCall{
		ThreadID:         threadID,
		TurnID:           turnID,
		Workspace:        workspace,
		ToolCallItemID:   "item_tool_shutdown_bg",
		ApprovalPolicy:   "auto",
		SandboxMode:      "danger-full-access",
		Call:             call,
		ProviderID:       "test-provider",
		Model:            "test-model",
		SubagentDepth:    0,
		DisableUserInput: true,
		SecurityContext:  securityContext,
		ExecutionGrant:   grant,
	}
	output, isError := handler.executeBashRuntimeTool(context.Background(), pending, arguments)
	if isError {
		t.Fatalf("start background shell failed: %#v", output)
	}
	jobID := stringField(output.(map[string]any), "jobId")
	if jobID == "" {
		t.Fatalf("background shell start should return job id: %#v", output)
	}
	var childPID string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if bytes, readErr := os.ReadFile(filepath.Join(workspace, "child.pid")); readErr == nil {
			childPID = strings.TrimSpace(string(bytes))
			if childPID != "" {
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	if childPID == "" {
		t.Fatalf("background shell did not start and record child pid")
	}
	startedRecord, err := handler.jobs.Load(jobID)
	if err != nil {
		t.Fatalf("load started background shell: %v", err)
	}
	if startedRecord.Output != "" || startedRecord.Error != "" {
		t.Fatalf("security-bound background shell must not persist process output: %#v", startedRecord)
	}

	if err := handler.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown returned error: %v", err)
	}
	var recordStatus string
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		record, err := handler.jobs.Load(jobID)
		if err == nil {
			recordStatus = record.Status
			if recordStatus == "killed" {
				if strings.TrimSpace(record.FinishedAt) == "" {
					t.Fatalf("terminal background shell should persist finishedAt: %#v", record)
				}
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	if recordStatus != "killed" {
		t.Fatalf("shutdown-cancelled background shell should be terminal, status=%q", recordStatus)
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if exec.Command("kill", "-0", childPID).Run() != nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("background shell child process %s survived runtime shutdown", childPID)
}

type shutdownMCPStub struct {
	once         sync.Once
	disconnected chan struct{}
}

type shutdownNativeLifecycle struct {
	mu       sync.Mutex
	failures int
	calls    int
	called   chan struct{}
}

type shutdownNativeDurableAuthority struct{}

func (shutdownNativeDurableAuthority) ValidateCurrent(
	_ string,
	thread map[string]any,
) (domainsecurity.TurnSecurityContext, error) {
	return domainsecurity.ParseTurnSecurityContext(thread["securityState"])
}

type shutdownNativeOwner struct {
	lifecycle *shutdownNativeLifecycle
}

func (*shutdownNativeOwner) Execute(
	context.Context,
	domainnative.Request,
) (domainnative.Result, error) {
	return domainnative.Result{}, nil
}

func (*shutdownNativeOwner) ValidateCurrentDataEngine(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil {
		return nativecomponentapp.ErrAuthorityInvalid
	}
	return nil
}

func (owner *shutdownNativeOwner) Close() error {
	if owner == nil || owner.lifecycle == nil {
		return errors.New("native lifecycle is unavailable")
	}
	return owner.lifecycle.Close()
}

func (lifecycle *shutdownNativeLifecycle) Close() error {
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	lifecycle.calls++
	lifecycle.called <- struct{}{}
	if lifecycle.failures > 0 {
		lifecycle.failures--
		return errors.New("injected native lifecycle failure")
	}
	return nil
}

func (lifecycle *shutdownNativeLifecycle) Calls() int {
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	return lifecycle.calls
}

func (m *shutdownMCPStub) Connect() {}

func (m *shutdownMCPStub) Diagnostics() map[string]any { return map[string]any{} }

func (m *shutdownMCPStub) ServerDiagnostics() []any { return nil }

func (m *shutdownMCPStub) Search(query string) []string { return nil }

func (m *shutdownMCPStub) CallTool(toolName string, approved bool, arguments ...map[string]any) map[string]any {
	return map[string]any{}
}

func (m *shutdownMCPStub) RefreshCatalog() map[string]any { return map[string]any{} }

func (m *shutdownMCPStub) RestartReconnect() map[string]any { return map[string]any{} }

func (m *shutdownMCPStub) Disconnect() {
	m.once.Do(func() {
		close(m.disconnected)
	})
}

func (m *shutdownMCPStub) Tools() []string { return nil }

func (m *shutdownMCPStub) MCPToolAdvertisementSnapshotV1(domainsecurity.TurnSecurityContext) []domainmcp.ToolAdvertisementV1 {
	return nil
}

func (m *shutdownMCPStub) Prompts() []any { return nil }

func (m *shutdownMCPStub) Resources() []any { return nil }

func (m *shutdownMCPStub) ToolReadOnlyHint(toolName string) bool { return false }

func (m *shutdownMCPStub) ToolInputSchema(toolName string) (json.RawMessage, bool) {
	return nil, false
}

func (m *shutdownMCPStub) ToolOutputSchema(toolName string) (json.RawMessage, bool) {
	return nil, false
}

func (m *shutdownMCPStub) ToolDescription(toolName string) (string, bool) {
	return "", false
}

func shutdownNativeQuestionFixture(t *testing.T, toolName string) (*runtimeServerHandler, runtimePendingToolCall, string) {
	t.Helper()
	h, pending, _, _ := protectedSeamFixtureV1(t, toolName, map[string]any{"prompt": "Continue?", "questions": []any{map[string]any{"question": "Continue?"}}})
	pending.ProviderConfig = domainmodel.TurnConfig{ProviderID: "synthetic", Model: "synthetic", APIKey: "test-only", BaseURL: "https://provider.invalid", EndpointFormat: "chat_completions"}
	pending.LogicalEffect, pending.OrdinaryWork = domainsecurity.LogicalEffectOrdinary, true
	pending.ProviderStepExact, pending.OrdinaryResultInputIsolated = true, true
	pending.ProviderNamespace = domaincontinuation.NewProviderContinuationNamespaceV1("turn", "", 1, nil)
	if pending.ExecutionGrant.ReadOnly || pending.ExecutionGrant.ServerIdentity != "host:builtin" {
		t.Fatal("Question fixture did not use the real native non-readonly policy")
	}
	id, err := h.requestRuntimeUserInput(context.Background(), pending)
	if err != nil {
		t.Fatal(err)
	}
	return h, pending, id
}

func assertShutdownQuestionSettled(t *testing.T, h *runtimeServerHandler, pending runtimePendingToolCall, inputID string, terminal bool) {
	t.Helper()
	thread, err := h.store.GetThread(pending.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := executiongrantapp.RegistryFromThread(pending.ThreadID, thread, pending.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, pending.ExecutionGrant.GrantID)
	if !found || entry.Status != domainsecurity.GrantRegistrySettled {
		t.Fatalf("Question grant remained open: found=%v status=%s", found, entry.Status)
	}
	_, disposition, err := h.continuations.ResolveTrustedDisposition(context.Background(), inputID)
	if err != nil || disposition.Status != domaincontinuation.StatusInterrupted {
		t.Fatalf("Question signed cancellation unavailable: disposition=%#v err=%v", disposition, err)
	}
	turn, status, found := gatecontinuationapp.RestartInventoryTurnV1(thread, pending.TurnID)
	if !found || (terminal && status != "aborted") || (!terminal && status != "running") {
		t.Fatalf("Question turn status=%q terminal=%v found=%v", status, terminal, found)
	}
	results := 0
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		if stringField(item, "kind") != "tool_result" || stringField(item, "callId") != pending.Call.ID {
			continue
		}
		projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(item["output"])
		if err != nil || projection.Code != "tool_cancelled" || projection.Status != "cancelled" || !projection.PrivatePayloadWithheld || projection.FactAnswerAllowed || projection.EvidenceAuthority ||
			stringField(item, "createdAt") != disposition.DisposedAt || stringField(item, "finishedAt") != disposition.DisposedAt || stringField(item, "executionGrantId") != pending.ExecutionGrant.GrantID {
			t.Fatalf("Question cancellation lost signed metadata binding: item=%#v err=%v", item, err)
		}
		results++
	}
	replay, err := h.store.LoadEventsSince(pending.ThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	resolvedSeq, resultSeq, terminalSeq, resultEvents, terminals := 0, 0, 0, 0, 0
	for _, event := range replay.Events {
		if stringField(event, "turnId") != pending.TurnID {
			continue
		}
		switch stringField(event, "kind") {
		case "user_input_resolved":
			resolvedSeq = transitionGateEventSeq(event["seq"])
		case "tool_call_finished":
			item, _ := event["item"].(map[string]any)
			if stringField(item, "callId") == pending.Call.ID {
				resultEvents++
				resultSeq = transitionGateEventSeq(event["seq"])
			}
		case "turn_aborted":
			terminals++
			terminalSeq = transitionGateEventSeq(event["seq"])
		}
	}
	if results != 1 || resultEvents != 1 || resolvedSeq == 0 || resultSeq <= resolvedSeq || (terminal && (terminals != 1 || terminalSeq <= resultSeq)) || (!terminal && terminals != 0) {
		t.Fatalf("Question settlement ordering/identity changed: results=%d events=%d terminals=%d resolution=%d result=%d terminal=%d", results, resultEvents, terminals, resolvedSeq, resultSeq, terminalSeq)
	}
}

func TestShutdownNativeQuestionSettlesGrantBeforeTerminalAndRestart(t *testing.T) {
	for _, toolName := range []string{"user_input", "request_user_input"} {
		t.Run(toolName, func(t *testing.T) {
			h, pending, inputID := shutdownNativeQuestionFixture(t, toolName)
			provider := &countingImmediateWorkspaceProvider{}
			h.provider = provider
			if err := h.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
			assertShutdownQuestionSettled(t, h, pending, inputID, true)
			if err := h.restoreRuntimeState(); err != nil {
				t.Fatalf("closed Question failed durable restart audit: %v", err)
			}
			if err := h.restoreRuntimeState(); err != nil {
				t.Fatalf("closed Question failed repeated restart audit: %v", err)
			}
			if provider.calls.Load() != 0 {
				t.Fatal("Question shutdown or restart resumed the provider")
			}
		})
	}
}

func TestShutdownNativeQuestionRetriesRealResultAndTerminalCrashCuts(t *testing.T) {
	for _, cut := range []string{"result_ack_lost", "before_terminal", "terminal_ack_lost"} {
		t.Run(cut, func(t *testing.T) {
			h, pending, inputID := shutdownNativeQuestionFixture(t, "request_user_input")
			deps := h.runtimeGateContinuationDependencies()
			injected := errors.New("synthetic Question crash cut")
			resultOwner, terminalOwner := deps.SettleTerminalQuestion, deps.RecordFailure
			resultCalls, terminalCalls := 0, 0
			deps.SettleTerminalQuestion = func(ctx context.Context, gate controlapp.DrainedGate[runtimePendingToolCall]) error {
				resultCalls++
				if err := resultOwner(ctx, gate); err != nil {
					return err
				}
				if cut == "result_ack_lost" && resultCalls == 1 {
					return injected
				}
				return nil
			}
			deps.RecordFailure = func(pending runtimePendingToolCall, cause error) error {
				terminalCalls++
				if cut == "before_terminal" && terminalCalls == 1 {
					return injected
				}
				if err := terminalOwner(pending, cause); err != nil {
					return err
				}
				if cut == "terminal_ack_lost" && terminalCalls == 1 {
					return injected
				}
				return nil
			}
			drained := h.gates.DrainAll()
			if err := gatecontinuationapp.CloseDrainedForShutdown(context.Background(), drained, deps); !errors.Is(err, injected) {
				t.Fatalf("first close did not preserve crash cut: %v", err)
			}
			assertShutdownQuestionSettled(t, h, pending, inputID, cut == "terminal_ack_lost")
			if !h.gates.RestoreDrained(drained) {
				t.Fatal("retain failed terminal claim")
			}
			retry := h.gates.DrainAll()
			if err := gatecontinuationapp.CloseDrainedForShutdown(context.Background(), retry, deps); err != nil {
				t.Fatalf("Question retry reauthorized a settled grant or duplicated a result: %v", err)
			}
			if !h.gates.CommitDrained(retry) {
				t.Fatal("commit exact retry claim")
			}
			assertShutdownQuestionSettled(t, h, pending, inputID, true)
			if cut == "terminal_ack_lost" && terminalCalls != 2 {
				t.Fatal("terminal ACK retry did not revisit the idempotent terminal outbox")
			}
		})
	}
}

func TestTerminalQuestionResultEventReadsBackLostAckAndRejectsConflicts(t *testing.T) {
	h, pending, inputID := shutdownNativeQuestionFixture(t, "request_user_input")
	disposition, err := h.continuations.DisposePendingHostDisposition(inputID, domaincontinuation.StatusInterrupted, "runtime_shutdown", time.Now().UTC(), pending)
	if err != nil {
		t.Fatal(err)
	}
	projection := toolcatalogapp.BuildPublicToolResultProjectionV1(pending.Call.Name, map[string]any{"code": "tool_cancelled"}, true)
	records, err := toolcatalogapp.SettleToolResult(toolcatalogapp.ToolResultInput{
		ThreadID: pending.ThreadID, TurnID: pending.TurnID, Call: pending.Call,
		CreatedAt: disposition.DisposedAt, FinishedAt: disposition.DisposedAt, Projection: projection, IsError: true,
		ContextDigest: pending.SecurityContext.ContextDigest, ContextEpoch: pending.SecurityContext.ContextEpoch, ExecutionGrantID: pending.ExecutionGrant.GrantID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.store.EnsureOrdinaryToolResultSettlementExact(context.Background(), pending.ThreadID, pending.TurnID, pending.ToolCallItemID, records.ResultItem); err != nil {
		t.Fatal(err)
	}
	deps := h.runtimeGateEventOutboxDependencies()
	record := deps.RecordEvent
	writes := 0
	deps.RecordEvent = func(event map[string]any) error {
		writes++
		if err := record(event); err != nil {
			return err
		}
		return errors.New("synthetic event acknowledgement lost after durable write")
	}
	if err := recordTerminalQuestionToolResultEventV1(records.Event, deps); err != nil {
		t.Fatalf("exact event readback did not resolve lost ACK: %v", err)
	}
	if err := recordTerminalQuestionToolResultEventV1(records.Event, deps); err != nil || writes != 1 {
		t.Fatalf("event retry duplicated its append: writes=%d err=%v", writes, err)
	}
	load := deps.LoadEvents
	deps.LoadEvents = func(threadID string) ([]map[string]any, error) {
		events, err := load(threadID)
		if err != nil {
			return nil, err
		}
		for _, event := range events {
			if stringField(event, "kind") == "tool_call_finished" {
				return append(events, event), nil
			}
		}
		return events, nil
	}
	if err := recordTerminalQuestionToolResultEventV1(records.Event, deps); err == nil || writes != 1 {
		t.Fatal("duplicate exact result events were accepted")
	}
	deps.LoadEvents = load
	conflicting := map[string]any{}
	body, _ := json.Marshal(records.Event)
	_ = json.Unmarshal(body, &conflicting)
	conflicting["itemId"] = "different_result_for_the_same_call"
	if err := recordTerminalQuestionToolResultEventV1(conflicting, deps); err == nil || writes != 1 {
		t.Fatal("same call with a conflicting result event identity was appended")
	}
}

func TestTerminalQuestionResultRepairsEventAfterOnlyPrimaryResultCommitted(t *testing.T) {
	h, pending, inputID := shutdownNativeQuestionFixture(t, "request_user_input")
	disposition, err := h.continuations.DisposePendingHostDisposition(inputID, domaincontinuation.StatusInterrupted, "runtime_shutdown", time.Now().UTC(), pending)
	if err != nil {
		t.Fatal(err)
	}
	records, err := toolcatalogapp.SettleToolResult(toolcatalogapp.ToolResultInput{
		ThreadID: pending.ThreadID, TurnID: pending.TurnID, Call: pending.Call,
		CreatedAt: disposition.DisposedAt, FinishedAt: disposition.DisposedAt,
		Projection: toolcatalogapp.BuildPublicToolResultProjectionV1(pending.Call.Name, map[string]any{"code": "tool_cancelled"}, true), IsError: true,
		ContextDigest: pending.SecurityContext.ContextDigest, ContextEpoch: pending.SecurityContext.ContextEpoch, ExecutionGrantID: pending.ExecutionGrant.GrantID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.store.EnsureOrdinaryToolResultSettlementExact(context.Background(), pending.ThreadID, pending.TurnID, pending.ToolCallItemID, records.ResultItem); err != nil {
		t.Fatal(err)
	}
	if err := h.Shutdown(context.Background()); err != nil {
		t.Fatalf("primary-only result did not repair its event before terminal: %v", err)
	}
	assertShutdownQuestionSettled(t, h, pending, inputID, true)
}

func TestTerminalQuestionPreservesDurableNormalResultAndBlocksUnsettledSubmission(t *testing.T) {
	for _, withResult := range []bool{false, true} {
		t.Run(map[bool]string{false: "unsettled_submission", true: "settled_submission"}[withResult], func(t *testing.T) {
			h, pending, inputID := shutdownNativeQuestionFixture(t, "request_user_input")
			text := "synthetic answer"
			var args map[string]any
			if err := json.Unmarshal(pending.Call.Arguments, &args); err != nil {
				t.Fatal(err)
			}
			questions := toolcatalogapp.UserInputQuestions(args, inputID, "Continue?")
			answers, submission, err := controlapp.NormalizeAndCaptureUserInputAnswers(controlapp.UserInputResponse{
				InputID: inputID, StructuredAnswers: []controlapp.UserInputAnswer{{ID: inputID + "_1", Kind: "answer", OptionIDs: []string{}, CustomText: &text}},
			}, questions, "")
			if err != nil || submission == nil {
				t.Fatalf("capture real Core Question submission: %v", err)
			}
			if _, err := h.continuations.DisposePendingUserInputHost(inputID, time.Now().UTC(), pending, *submission); err != nil {
				t.Fatal(err)
			}
			if withResult {
				output := appturn.UserInputResolvedOutputForQuestions("submitted", answers, false, inputID, questions)
				if _, err := h.persistRuntimeToolResultAndMessage(context.Background(), pending, output, false); err != nil {
					t.Fatal(err)
				}
			}
			err = h.Shutdown(context.Background())
			if (err == nil) != withResult {
				t.Fatalf("submission terminalization ignored durable settlement: withResult=%v err=%v", withResult, err)
			}
			thread, err := h.store.GetThread(pending.ThreadID)
			if err != nil {
				t.Fatal(err)
			}
			turn, status, _ := gatecontinuationapp.RestartInventoryTurnV1(thread, pending.TurnID)
			if (withResult && status != "aborted") || (!withResult && status != "running") {
				t.Fatalf("submission turn status=%q withResult=%v", status, withResult)
			}
			results := 0
			for _, raw := range listAny(turn["items"]) {
				item, _ := raw.(map[string]any)
				if stringField(item, "kind") != "tool_result" || stringField(item, "callId") != pending.Call.ID {
					continue
				}
				projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(item["output"])
				if err != nil || projection.Status != "completed" {
					t.Fatalf("normal result was replaced by cancellation: %#v err=%v", item, err)
				}
				results++
			}
			if (withResult && results != 1) || (!withResult && results != 0) {
				t.Fatalf("submission result count=%d withResult=%v", results, withResult)
			}
			_, disposition, err := h.continuations.ResolveTrustedDisposition(context.Background(), inputID)
			if err != nil || disposition.Status != domaincontinuation.StatusSubmitted {
				t.Fatalf("submission disposition was replaced: %#v err=%v", disposition, err)
			}
		})
	}
}

func TestTerminalQuestionRejectsWrongSignedContextAndMissingDispatch(t *testing.T) {
	h, pending, inputID := shutdownNativeQuestionFixture(t, "request_user_input")
	drained := h.gates.DrainAll()
	if len(drained) != 1 {
		t.Fatal("missing native Question claim")
	}
	gate := drained[0]
	if err := h.continuations.DisposePendingHost(inputID, domaincontinuation.StatusInterrupted, "runtime_shutdown", time.Now().UTC(), pending); err != nil {
		t.Fatal(err)
	}
	gate.TerminalStatus, gate.TerminalReasonCode = domaincontinuation.StatusInterrupted, "runtime_shutdown"
	gate.Pending.SecurityContext.ContextDigest = domainsecurity.SHA256Hex([]byte("different signed context"))
	if err := h.persistTerminalQuestionResult(context.Background(), gate); err == nil {
		t.Fatal("Question cancellation accepted a different signed context")
	}
	unstarted := pending
	unstarted.Call.ID = serverTestHostToolCallID("question_without_original_dispatch")
	unstarted.ToolCallItemID = domaintoolcall.ToolCallItemIDV1(pending.TurnID, unstarted.Call.ID)
	now := time.Now().UTC()
	unstarted.ExecutionGrant = domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: pending.SecurityContext, Provider: pending.ProviderID, ServerIdentity: "host:builtin",
		ToolName: pending.Call.Name, ToolCallID: unstarted.Call.ID, ArgsHash: pending.ExecutionGrant.ArgsHash,
		SchemaHash: pending.ExecutionGrant.SchemaHash, ScopeHash: pending.ExecutionGrant.ScopeHash,
		ApprovalState: "not_required", ReadOnly: false, IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	unstartedID := controlapp.SecureGateID(domaincontinuation.KindUserInput, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, unstarted.ExecutionGrant.GrantID, unstarted.Call.ID)
	receipt, err := h.continuations.IssuePendingHost(domaincontinuation.KindUserInput, unstartedID, "item_"+unstartedID, unstarted, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.continuations.DisposePendingHost(unstartedID, domaincontinuation.StatusInterrupted, "runtime_shutdown", now, unstarted); err != nil {
		t.Fatal(err)
	}
	gate.ID, gate.Pending = unstartedID, unstarted
	gate.Record = controlapp.GateRecord{ThreadID: pending.ThreadID, TurnID: pending.TurnID, ItemID: "item_" + unstartedID, ToolName: pending.Call.Name, ContinuationReceiptID: receipt.ReceiptID}
	if err := h.persistTerminalQuestionResult(context.Background(), gate); err == nil {
		t.Fatal("Question cancellation accepted a signed call without original dispatch")
	}
	thread, err := h.store.GetThread(pending.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := executiongrantapp.RegistryFromThread(pending.ThreadID, thread, pending.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, pending.ExecutionGrant.GrantID)
	if !found || entry.Status != domainsecurity.GrantRegistryActive {
		t.Fatalf("rejected Question cancellation changed grant state: found=%v status=%s", found, entry.Status)
	}
}

func TestTerminalQuestionRepairsNormalResultEventBeforeTerminalAfterWriteFailure(t *testing.T) {
	h, pending, inputID := shutdownNativeQuestionFixture(t, "request_user_input")
	provider := &countingImmediateWorkspaceProvider{}
	h.provider = provider
	injected := errors.New("synthetic normal Question result event write failure")
	var originalEvent map[string]any
	eventAttempts := 0
	h.store.beforeRecordEventHook = func(event map[string]any) error {
		item, _ := event["item"].(map[string]any)
		if stringField(event, "kind") == "tool_call_finished" && stringField(item, "callId") == pending.Call.ID {
			eventAttempts++
			if originalEvent == nil {
				body, _ := json.Marshal(event)
				_ = json.Unmarshal(body, &originalEvent)
			}
			return injected
		}
		return nil
	}
	text := "synthetic answer"
	result, err := h.respondRuntimeUserInput(context.Background(), controlapp.UserInputResponse{
		InputID: inputID, StructuredAnswers: []controlapp.UserInputAnswer{{ID: inputID + "_1", Kind: "answer", OptionIDs: []string{}, CustomText: &text}},
	})
	if err != nil || result.StatusCode < 500 || eventAttempts == 0 {
		t.Fatalf("normal Question result did not preserve event write failure: result=%#v attempts=%d err=%v", result, eventAttempts, err)
	}
	thread, err := h.store.GetThread(pending.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	_, status, found := gatecontinuationapp.RestartInventoryTurnV1(thread, pending.TurnID)
	if !found || status != "running" {
		t.Fatalf("missing normal result event did not block terminal: status=%q found=%v", status, found)
	}
	original, err := executiongrantapp.DurableSettlementFromThread(pending.ThreadID, thread, pending.TurnID, toolcatalogapp.ToolResultItemID(pending.TurnID, pending.Call.ID), pending.ExecutionGrant)
	if err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(original.ResultItem)
	if err != nil {
		t.Fatal(err)
	}
	h.store.beforeRecordEventHook = nil
	if err := h.Shutdown(context.Background()); err != nil {
		t.Fatalf("normal Question result event retry failed: %v", err)
	}
	thread, err = h.store.GetThread(pending.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	settled, err := executiongrantapp.DurableSettlementFromThread(pending.ThreadID, thread, pending.TurnID, toolcatalogapp.ToolResultItemID(pending.TurnID, pending.Call.ID), pending.ExecutionGrant)
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(settled.ResultItem)
	if err != nil || string(before) != string(after) {
		t.Fatalf("event retry replaced the original result or timestamps: err=%v", err)
	}
	replay, err := h.store.LoadEventsSince(pending.ThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	resultEvents, terminals, resultSeq, terminalSeq := 0, 0, 0, 0
	for _, event := range replay.Events {
		if stringField(event, "turnId") != pending.TurnID {
			continue
		}
		if stringField(event, "kind") == "tool_call_finished" && stringField(event, "itemId") == stringField(original.ResultItem, "id") {
			if !gatecontinuationapp.ExactGateEventProjectionV1(event, originalEvent) {
				t.Fatal("repaired normal result event differs from the original public projection")
			}
			resultEvents++
			resultSeq = transitionGateEventSeq(event["seq"])
		}
		if stringField(event, "kind") == "turn_aborted" {
			terminals++
			terminalSeq = transitionGateEventSeq(event["seq"])
		}
	}
	if resultEvents != 1 || terminals != 1 || resultSeq == 0 || terminalSeq <= resultSeq || provider.calls.Load() != 0 {
		t.Fatalf("normal result event/terminal ordering or continuation changed: events=%d terminals=%d result=%d terminal=%d provider=%d", resultEvents, terminals, resultSeq, terminalSeq, provider.calls.Load())
	}
}

func TestShutdownQuestionRepairsTerminalBundleAfterCASWriteBeforeAppendFailure(t *testing.T) {
	h, pending, inputID := shutdownNativeQuestionFixture(t, "request_user_input")
	injected := errors.New("synthetic Question terminal bundle append failure")
	bundleAttempts := 0
	h.store.generalTerminalAtomicAppend = func(string, []map[string]any) error {
		bundleAttempts++
		return injected
	}
	if err := h.Shutdown(context.Background()); !errors.Is(err, injected) || bundleAttempts != 1 {
		t.Fatalf("first shutdown did not preserve terminal bundle blocker: attempts=%d err=%v", bundleAttempts, err)
	}
	thread, err := h.store.GetThread(pending.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, status, found := gatecontinuationapp.RestartInventoryTurnV1(thread, pending.TurnID)
	if !found || status != "aborted" || turn["generalTerminalCASBinding"] == nil || turn["generalTerminalPublication"] == nil {
		t.Fatalf("first shutdown did not commit the terminal winner: status=%q found=%v", status, found)
	}
	winner, err := json.Marshal(turn["generalTerminalPublication"])
	if err != nil {
		t.Fatal(err)
	}
	replay, err := h.store.LoadEventsSince(pending.ThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := appturn.PreflightGeneralTerminalPublicationInventoryV1(thread, replay.Events)
	if err != nil {
		t.Fatal(err)
	}
	entry, found := appturn.GeneralTerminalPublicationEntryForTurnV1(inventory, pending.TurnID)
	if !found || entry.State != appturn.GeneralTerminalPublicationMissingV1 {
		t.Fatalf("terminal append cut was not retained as missing: found=%v state=%s", found, entry.State)
	}
	h.store.generalTerminalAtomicAppend = nil
	if err := h.Shutdown(context.Background()); err != nil {
		t.Fatalf("second shutdown did not replay the winner and repair its terminal bundle: %v", err)
	}
	assertShutdownQuestionSettled(t, h, pending, inputID, true)
	thread, err = h.store.GetThread(pending.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, _, _ = gatecontinuationapp.RestartInventoryTurnV1(thread, pending.TurnID)
	after, err := json.Marshal(turn["generalTerminalPublication"])
	if err != nil || string(winner) != string(after) {
		t.Fatalf("terminal retry replaced the first winner: err=%v", err)
	}
	replay, err = h.store.LoadEventsSince(pending.ThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err = appturn.PreflightGeneralTerminalPublicationInventoryV1(thread, replay.Events)
	if err != nil {
		t.Fatal(err)
	}
	entry, found = appturn.GeneralTerminalPublicationEntryForTurnV1(inventory, pending.TurnID)
	if !found || entry.State != appturn.GeneralTerminalPublicationCompleteV1 || len(entry.Events) != 3 {
		t.Fatalf("terminal retry did not close one exact bundle: found=%v state=%s events=%d", found, entry.State, len(entry.Events))
	}
	seen := map[string]bool{}
	for _, event := range entry.Events {
		id := stringField(event, "generalTerminalEventId")
		if id == "" || seen[id] {
			t.Fatal("terminal retry duplicated a bundle event identity")
		}
		seen[id] = true
	}
}

func TestWorkspacePatchSettlesNativeQuestionInsideReservedWriterBeforeNewEpoch(t *testing.T) {
	h, pending, inputID := shutdownNativeQuestionFixture(t, "request_user_input")
	provider := &countingImmediateWorkspaceProvider{}
	h.provider = provider
	workspace := workspacetest.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	patched, err := h.runtimeThreadService().Patch(ctx, pending.ThreadID, map[string]any{"workspace": workspace})
	if err != nil {
		t.Fatalf("workspace writer could not close its native Question before epoch commit: %v", err)
	}
	if err := ctx.Err(); err != nil {
		t.Fatalf("workspace Question settlement exceeded the writer deadline: %v", err)
	}
	realWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil || stringField(patched, "workspace") != realWorkspace {
		t.Fatalf("workspace patch did not return the new workspace: workspace=%q err=%v", stringField(patched, "workspace"), err)
	}
	if patched["securityState"] != nil || patched["contextEpochState"] != nil {
		t.Fatal("workspace patch exposed private epoch authority")
	}
	assertShutdownQuestionSettled(t, h, pending, inputID, true)
	thread, err := h.store.GetThread(pending.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	current, found, err := turnsecurityapp.LatestContext(thread)
	if err != nil || !found || current.ContextEpoch <= pending.SecurityContext.ContextEpoch || current.ContextDigest == pending.SecurityContext.ContextDigest || current.WorkspaceRealPath != realWorkspace {
		t.Fatalf("workspace patch did not commit a new epoch after Question settlement: current=%#v found=%v err=%v", current, found, err)
	}
	turn, _, found := gatecontinuationapp.RestartInventoryTurnV1(thread, pending.TurnID)
	if !found {
		t.Fatal("workspace mutation lost the original Question turn")
	}
	originalContext, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || originalContext != pending.SecurityContext {
		t.Fatalf("workspace mutation rebound the original Question context: context=%#v err=%v", originalContext, err)
	}
	durable, err := executiongrantapp.DurableSettlementFromThread(pending.ThreadID, thread, pending.TurnID, toolcatalogapp.ToolResultItemID(pending.TurnID, pending.Call.ID), pending.ExecutionGrant)
	if err != nil || stringField(durable.ResultItem, "contextDigest") != pending.SecurityContext.ContextDigest {
		t.Fatalf("Question result was not settled against its original grant/context: err=%v", err)
	}
	_, disposition, err := h.continuations.ResolveTrustedDisposition(context.Background(), inputID)
	if err != nil || disposition.Status != domaincontinuation.StatusInterrupted || disposition.ReasonCode != "security_context_transition" {
		t.Fatalf("workspace mutation changed the signed Question close intent: disposition=%#v err=%v", disposition, err)
	}
	if provider.calls.Load() != 0 {
		t.Fatal("workspace Question settlement resumed the provider")
	}
}
