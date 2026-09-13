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
	nativecomponentapp "analytix.local/runtime-go/internal/app/nativecomponent"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
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
