package server

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	controlapp "analytix.local/runtime-go/internal/app/control"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type cancelAfterCurrentObservation struct {
	delegate filestore.CaseBindingReader
	cancel   context.CancelFunc
	once     sync.Once
}

func (observer *cancelAfterCurrentObservation) Observe(workspace string) (domainsecurity.CaseBindingObservationV1, error) {
	observation, err := observer.delegate.Observe(workspace)
	if err == nil && observer.cancel != nil {
		observer.once.Do(observer.cancel)
	}
	return observation, err
}

func TestContinuationLateCancelPersistsOneFixedTerminalWithoutCandidateBytes(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
	}).(*runtimeServerHandler)
	workspace := t.TempDir()
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "late continuation cancel", "workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn-late-continuation-cancel"
	riskAuthority := newServerTestRiskAuthority()
	securityContext, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: context.Background(), Authority: turnsecurityapp.WorkspaceSecurityAuthority{Identity: testIdentityAuthority(),
			Observer: filestore.CaseBindingReader{}, RiskAuthority: riskAuthority,
		},
		Thread: thread, ThreadID: threadID, TurnID: turnID, Workspace: workspace,
		Principal: testIdentityPrincipal(), IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := handler.store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running",
		"createdAt": time.Now().UTC().Format(time.RFC3339Nano),
		"items":     []any{}, "steering": []any{}, "prompt": "continue",
		"securityContext": securityRecord,
	}, "model", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatal(err)
	}
	caseAuthority := &caseThreadAuthorityStub{threads: map[string]bool{}}
	handler.caseThreads = caseAuthority
	handler.store.SetCaseThreadAuthority(caseAuthority)
	operationContext, cancelOperation := context.WithCancel(context.Background())
	handler.turnSecurity = turnsecurityapp.WorkspaceSecurityAuthority{
		Identity: testIdentityAuthority(),
		Observer: filestore.CaseBindingReader{},
		RiskAuthority: riskAuthority,
	}
	if !handler.runtimeControl().RegisterTurnCancel(threadID, turnID, cancelOperation) {
		t.Fatal("register continuation owner")
	}
	defer handler.runtimeControl().UnregisterTurnCancel(threadID, turnID)
	candidateContext, releaseCandidate, err := handler.runtimePublicationAuthority().AcquireCurrentCandidateTerminalForLoop(
		operationContext, securityContext, false,
	)
	if err != nil || candidateContext == nil || releaseCandidate == nil {
		t.Fatalf("acquire continuation candidate terminal: context=%t release=%t err=%v", candidateContext != nil, releaseCandidate != nil, err)
	}
	defer releaseCandidate()
	cancelOperation()

	const sentinel = "甲公司支付2645472元。张三是李四的堂兄。"
	pending := runtimePendingToolCall{
		ThreadID: threadID, TurnID: turnID, Model: "model", Workspace: workspace,
		Prompt: "continue", SecurityContext: securityContext,
	}
	loopResult := runtimeAgentLoopResult{
		AssistantText: sentinel, CandidateTerminalContext: candidateContext, ReleaseCandidateTerminal: releaseCandidate,
	}
	if err := handler.finalizeRuntimeTurnAfterLoop(operationContext, pending, loopResult, evidenceapp.TerminalSuccess); err != nil {
		t.Fatal(err)
	}
	// A repeated consumed-gate completion is first-winner CAS only.
	loopResult.CandidateTerminalContext = nil
	loopResult.ReleaseCandidateTerminal = nil
	if err := handler.finalizeRuntimeTurnAfterLoop(operationContext, pending, loopResult, evidenceapp.TerminalSuccess); err != nil {
		t.Fatal(err)
	}

	reloaded, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(reloaded)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), sentinel) {
		t.Fatalf("cancelled provider candidate reached durable state: %s", body)
	}
	turns := listAny(reloaded["turns"])
	if len(turns) != 1 || stringField(turns[0].(map[string]any), "status") != "aborted" {
		t.Fatalf("late cancellation did not produce one aborted terminal: %#v", turns)
	}
	replay, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	aborted := 0
	for _, event := range replay.Events {
		if stringField(event, "turnId") == turnID && stringField(event, "kind") == "turn_aborted" {
			aborted++
		}
	}
	if aborted != 1 {
		t.Fatalf("terminal bundle count = %d, want 1", aborted)
	}
}

func TestStartTurnRejectsSameThreadTerminalTailBeforeAppendOrProvider(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "tail-provider", BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: "tail-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	provider := &countingImmediateWorkspaceProvider{}
	handler.provider = provider
	workspace := t.TempDir()
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "terminal tail admission", "workspace": workspace,
		"providerId": "tail-provider", "model": "tail-model",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	releaseTail, err := handler.runtimeControl().AcquireHostTerminal(context.Background(), threadID, "turn-old", "digest-old")
	if err != nil || releaseTail == nil {
		t.Fatalf("acquire old terminal tail: %v", err)
	}
	if _, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{Prompt: "must wait for old tail"}); !errors.Is(err, controlapp.ErrTurnExecutionConflict) {
		t.Fatalf("same-thread terminal tail admission error = %v", err)
	}
	before, err := handler.store.GetThread(threadID)
	if err != nil || len(listAny(before["turns"])) != 0 || provider.calls.Load() != 0 {
		t.Fatalf("rejected start reached durable/provider boundary: thread=%#v calls=%d err=%v", before, provider.calls.Load(), err)
	}
	releaseTail()
	if _, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{Prompt: "start after tail"}); err != nil {
		t.Fatal(err)
	}
	after, err := handler.store.GetThread(threadID)
	if err != nil || len(listAny(after["turns"])) != 1 || provider.calls.Load() != 1 {
		t.Fatalf("start did not resume after terminal tail: thread=%#v calls=%d err=%v", after, provider.calls.Load(), err)
	}
}
