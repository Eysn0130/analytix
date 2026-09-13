package server

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	controlapp "analytix.local/runtime-go/internal/app/control"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

type registerBarrierCaseAuthority struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	mu      sync.Mutex
	staged  map[string]domainsecurity.TurnSecurityContext
	commits map[string]domaincontextepoch.State
}

func (authority *registerBarrierCaseAuthority) Register(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) error {
	authority.once.Do(func() { close(authority.entered) })
	select {
	case <-authority.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	authority.mu.Lock()
	authority.staged[securityContext.TurnID] = securityContext
	authority.mu.Unlock()
	return nil
}

func (authority *registerBarrierCaseAuthority) Commit(_ context.Context, securityContext domainsecurity.TurnSecurityContext, state domaincontextepoch.State, _ time.Time) error {
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if authority.staged[securityContext.TurnID] != securityContext {
		return errors.New("case authority commit lacks staged context")
	}
	authority.commits[securityContext.TurnID] = state
	return nil
}

func (authority *registerBarrierCaseAuthority) Derive(context.Context, string, string, string) error {
	return nil
}

func (authority *registerBarrierCaseAuthority) IsCaseThread(string) bool { return true }

func (authority *registerBarrierCaseAuthority) ContainsContext(securityContext domainsecurity.TurnSecurityContext) bool {
	authority.mu.Lock()
	defer authority.mu.Unlock()
	_, ok := authority.commits[securityContext.TurnID]
	return ok && authority.staged[securityContext.TurnID] == securityContext
}

func (authority *registerBarrierCaseAuthority) ContextTurnIDs(string) []string {
	authority.mu.Lock()
	defer authority.mu.Unlock()
	ids := make([]string, 0, len(authority.commits))
	for turnID := range authority.commits {
		ids = append(ids, turnID)
	}
	return ids
}

func (*registerBarrierCaseAuthority) CanExecute(string) bool              { return true }
func (*registerBarrierCaseAuthority) ReplaceQuarantine(map[string]string) {}

func (*registerBarrierCaseAuthority) WithRestartRecoveryV1(_ string, recover func() error) error {
	return recover()
}

func (authority *registerBarrierCaseAuthority) CommittedContext(_ string, turnID string) (casethreadapp.CommittedContext, bool) {
	authority.mu.Lock()
	defer authority.mu.Unlock()
	securityContext, contextOK := authority.staged[turnID]
	state, stateOK := authority.commits[turnID]
	return casethreadapp.CommittedContext{SecurityContext: securityContext, EpochState: state}, contextOK && stateOK
}

func (authority *registerBarrierCaseAuthority) CommittedContexts() []casethreadapp.CommittedContext {
	authority.mu.Lock()
	defer authority.mu.Unlock()
	contexts := make([]casethreadapp.CommittedContext, 0, len(authority.commits))
	for turnID, state := range authority.commits {
		contexts = append(contexts, casethreadapp.CommittedContext{SecurityContext: authority.staged[turnID], EpochState: state})
	}
	return contexts
}

type immediateMutationWriterProvider struct{}

func (immediateMutationWriterProvider) RequiresDurablePipelineStagesV1() {}

func (immediateMutationWriterProvider) Stream(_ context.Context, request domainmodel.Request) (domainmodel.Result, error) {
	if err := emitTestDurableProviderPipelinePairV1(request); err != nil {
		return domainmodel.Result{}, err
	}
	chunk := domainmodel.Chunk{Kind: domainmodel.ChunkText, Text: "writer barrier completion"}
	if request.OnChunk != nil {
		if err := request.OnChunk(chunk); err != nil {
			return domainmodel.Result{}, err
		}
	}
	return domainmodel.Result{ProviderID: request.ProviderID, EndpointFormat: request.EndpointFormat, Chunks: []domainmodel.Chunk{chunk}, StreamCompleted: true}, nil
}

func TestRegisterRequiredToAppendHoldsWriterAgainstPatchAndDelete(t *testing.T) {
	workspace := writeThreadMutationCaseBinding(t)
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "writer-provider", BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: "writer-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	authority := &registerBarrierCaseAuthority{
		entered: make(chan struct{}), release: make(chan struct{}), staged: map[string]domainsecurity.TurnSecurityContext{},
		commits: map[string]domaincontextepoch.State{},
	}
	handler.caseThreads = authority
	handler.store.SetCaseThreadAuthority(authority)
	configureServerCaseExecution(t, handler, workspace)
	handler.provider = immediateMutationWriterProvider{}
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "before", "workspace": workspace, "providerId": "writer-provider", "model": "writer-model",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	startDone := make(chan error, 1)
	go func() {
		_, startErr := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{Prompt: "hold register authority", Async: true})
		startDone <- startErr
	}()
	select {
	case <-authority.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("turn start did not reach RegisterRequired")
	}

	patchCtx, cancelPatch := context.WithTimeout(context.Background(), 50*time.Millisecond)
	_, patchErr := handler.runtimeThreadService().Patch(patchCtx, threadID, map[string]any{"title": "must-wait"})
	cancelPatch()
	if !errors.Is(patchErr, context.DeadlineExceeded) || !errors.Is(patchErr, threadapp.ErrThreadMutationTransition) {
		t.Fatalf("PATCH crossed the RegisterRequired -> Append writer: %v", patchErr)
	}
	deleteCtx, cancelDelete := context.WithTimeout(context.Background(), 50*time.Millisecond)
	_, deleteErr := handler.runtimeThreadService().Delete(deleteCtx, threadID)
	cancelDelete()
	if !errors.Is(deleteErr, threadapp.ErrThreadMutationTransition) ||
		(!errors.Is(deleteErr, context.DeadlineExceeded) && !errors.Is(deleteErr, controlapp.ErrThreadTransition)) {
		t.Fatalf("DELETE crossed the RegisterRequired -> Append writer: %v", deleteErr)
	}
	stagedThread, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	if stringField(stagedThread, "title") != "before" || len(listAny(stagedThread["turns"])) != 0 {
		t.Fatalf("metadata mutation or append crossed staged authority: %#v", stagedThread)
	}

	close(authority.release)
	select {
	case err := <-startDone:
		if err != nil {
			t.Fatalf("turn start failed after RegisterRequired released: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("turn start remained blocked after RegisterRequired released")
	}
	if _, err := handler.runtimeThreadService().Patch(context.Background(), threadID, map[string]any{"title": "after"}); err != nil {
		t.Fatalf("PATCH failed after the staged authority committed: %v", err)
	}
	if _, err := handler.runtimeThreadService().Patch(context.Background(), threadID, map[string]any{"workspace": t.TempDir()}); !errors.Is(err, threadapp.ErrCaseWorkspaceSignedRebind) {
		t.Fatalf("case workspace rebind did not fail closed after append: %v", err)
	}
	if _, err := handler.runtimeThreadService().Delete(context.Background(), threadID); !errors.Is(err, threadapp.ErrCaseDeleteSignedTombstone) {
		t.Fatalf("case DELETE did not fail closed after append: %v", err)
	}
	if _, err := handler.runtimeThreadService().Rewind(context.Background(), threadID, "turn_1"); !errors.Is(err, threadapp.ErrAcceptedFinalRewind) {
		t.Fatalf("accepted-final rewind did not fail closed after append: %v", err)
	}
	committedThread, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	if stringField(committedThread, "title") != "after" || len(listAny(committedThread["turns"])) == 0 {
		t.Fatalf("committed authority was lost after queued mutations: %#v", committedThread)
	}
}

func writeThreadMutationCaseBinding(t *testing.T) string {
	t.Helper()
	workspace := workspacetest.New(t)
	realPath, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(realPath, ".analytix")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{
		"version": 1, "workspaceRoot": realPath, "caseId": "case_writer_barrier", "source": "analytix-data-analysis",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "case-project.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	return realPath
}

func (*registerBarrierCaseAuthority) RestartPreservesThreadV1(string) bool { return false }
