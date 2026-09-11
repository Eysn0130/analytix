package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const ignoredCancellationFinalSentinel = "OLD_WORKSPACE_FINAL_MUST_NOT_PUBLISH_927451"
const foregroundProviderEntryTimeout = 15 * time.Second

type ignoredCancellationProvider struct {
	entered   chan struct{}
	cancelled chan struct{}
	release   chan struct{}
	onceEnter sync.Once
	onceStop  sync.Once
	text      string
}

type countingImmediateWorkspaceProvider struct {
	calls atomic.Int64
}

func (*countingImmediateWorkspaceProvider) RequiresDurablePipelineStagesV1() {}

func (*ignoredCancellationProvider) RequiresDurablePipelineStagesV1() {}

func (provider *countingImmediateWorkspaceProvider) Stream(ctx context.Context, request domainmodel.Request) (domainmodel.Result, error) {
	provider.calls.Add(1)
	return (immediateMutationWriterProvider{}).Stream(ctx, request)
}

func (provider *ignoredCancellationProvider) Stream(ctx context.Context, request domainmodel.Request) (domainmodel.Result, error) {
	if err := emitTestDurableProviderPipelinePairV1(request); err != nil {
		return domainmodel.Result{}, err
	}
	provider.onceEnter.Do(func() { close(provider.entered) })
	select {
	case <-ctx.Done():
		provider.onceStop.Do(func() { close(provider.cancelled) })
		<-provider.release
	case <-provider.release:
	}
	text := provider.text
	if text == "" {
		text = ignoredCancellationFinalSentinel
	}
	return domainmodel.Result{
		ProviderID: request.ProviderID, EndpointFormat: request.EndpointFormat, StreamCompleted: true,
		Chunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: text}},
	}, nil
}

func TestTitlePatchDuringForegroundTurnDoesNotCancelExecution(t *testing.T) {
	workspace := t.TempDir()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "title-provider", BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: "title-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	provider := &ignoredCancellationProvider{
		entered: make(chan struct{}), cancelled: make(chan struct{}), release: make(chan struct{}), text: "TITLE_PATCH_TURN_COMPLETED_613579",
	}
	handler.provider = provider
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "before", "workspace": workspace, "providerId": "title-provider", "model": "title-model",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	if _, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{Prompt: "keep running during title patch", Async: true}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.entered:
	case <-time.After(foregroundProviderEntryTimeout):
		t.Fatal("provider did not enter its foreground stream")
	}
	if _, err := handler.runtimeThreadService().Patch(context.Background(), threadID, map[string]any{"title": "after"}); err != nil {
		t.Fatalf("title patch failed during foreground execution: %v", err)
	}
	select {
	case <-provider.cancelled:
		t.Fatal("ordinary metadata patch cancelled the foreground turn")
	default:
	}
	if handler.runtimeControl().ActiveTurnCount() != 1 {
		t.Fatal("ordinary metadata patch released foreground execution ownership")
	}
	close(provider.release)
	waitCtx, cancelWait := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelWait()
	if err := handler.runtimeControl().WaitForThreadTurns(waitCtx, threadID); err != nil {
		t.Fatalf("foreground turn did not finish after title patch: %v", err)
	}
	reloaded, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(reloaded)
	if stringField(reloaded, "title") != "after" || !strings.Contains(string(body), provider.text) ||
		strings.Contains(string(body), turnapp.GeneralProviderFinalQuarantinedText) {
		t.Fatalf("foreground turn or title patch was lost: %s", body)
	}
	turns := listAny(reloaded["turns"])
	if len(turns) != 1 || stringField(turns[0].(map[string]any), "status") != "completed" {
		t.Fatalf("foreground turn did not complete normally: %#v", turns)
	}
}

func TestWorkspacePatchCancelsAndWaitsForForegroundProviderBeforeRebind(t *testing.T) {
	workspaceA := t.TempDir()
	workspaceB := t.TempDir()
	durableRoot := t.TempDir()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: t.TempDir(),
		ProviderID: "blocking-provider", BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: "blocking-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	provider := &ignoredCancellationProvider{entered: make(chan struct{}), cancelled: make(chan struct{}), release: make(chan struct{})}
	handler.provider = provider
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "foreground rebind", "workspace": workspaceA, "providerId": "blocking-provider", "model": "blocking-model",
	}, workspaceA)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	if _, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{Prompt: "block then ignore cancellation", Async: true}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.entered:
	case <-time.After(foregroundProviderEntryTimeout):
		t.Fatal("provider did not enter its foreground stream")
	}

	patchDone := make(chan error, 1)
	go func() {
		_, patchErr := handler.runtimeThreadService().Patch(context.Background(), threadID, map[string]any{"workspace": workspaceB})
		patchDone <- patchErr
	}()
	select {
	case <-provider.cancelled:
	case err := <-patchDone:
		t.Fatalf("workspace patch returned before cancelling the provider: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("workspace patch did not cancel the active foreground provider")
	}
	select {
	case err := <-patchDone:
		t.Fatalf("workspace patch returned before foreground terminal ownership settled: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(provider.release)
	select {
	case err := <-patchDone:
		if err != nil {
			t.Fatalf("workspace patch failed after the provider released: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("workspace patch remained blocked after foreground terminal ownership settled")
	}

	reloaded, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	expectedWorkspace, err := filepath.EvalSymlinks(workspaceB)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stringField(reloaded, "workspace")) != expectedWorkspace {
		t.Fatalf("workspace rebind was not committed: %#v", reloaded)
	}
	body, err := json.Marshal(reloaded)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), ignoredCancellationFinalSentinel) {
		t.Fatalf("provider output returned after cancellation was persisted into the rebound thread: %s", body)
	}
	turns := listAny(reloaded["turns"])
	if len(turns) < 2 || stringField(turns[len(turns)-1].(map[string]any), "kind") != "workspace_transition" {
		t.Fatalf("workspace transition is not the durable high-water: %#v", turns)
	}
	providerFinishedAt, err := time.Parse(time.RFC3339Nano, stringField(turns[0].(map[string]any), "finishedAt"))
	if err != nil {
		t.Fatal(err)
	}
	rebindUpdatedAt, err := time.Parse(time.RFC3339Nano, stringField(reloaded, "updatedAt"))
	if err != nil || rebindUpdatedAt.Before(providerFinishedAt) {
		t.Fatalf("workspace commit timestamp precedes foreground terminal persistence: finished=%s updated=%s err=%v", providerFinishedAt, rebindUpdatedAt, err)
	}
	restarted, err := NewTempDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	restartedThread, err := restarted.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	restartedContext, found, err := turnsecurityapp.LatestContext(restartedThread)
	if err != nil || !found || restartedContext.WorkspaceRealPath != expectedWorkspace {
		t.Fatalf("restart lost workspace security authority: context=%#v found=%t err=%v", restartedContext, found, err)
	}
	restartedEpoch, ok, err := contextepochapp.StateFromThread(restartedThread)
	if err != nil || !ok || restartedEpoch.AcceptedSnapshot.Epoch != restartedContext.ContextEpoch {
		t.Fatalf("restart split workspace and epoch authority: state=%#v ok=%t err=%v", restartedEpoch, ok, err)
	}
}

func TestWorkspacePatchAdvancesAuthorityForNextProviderDispatch(t *testing.T) {
	workspaceA := t.TempDir()
	workspaceB := t.TempDir()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "rebind-provider", BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: "rebind-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	handler.provider = immediateMutationWriterProvider{}
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "provider after rebind", "workspace": workspaceA, "providerId": "rebind-provider", "model": "rebind-model",
	}, workspaceA)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	patched, err := handler.runtimeThreadService().Patch(context.Background(), threadID, map[string]any{"workspace": workspaceB})
	if err != nil {
		t.Fatal(err)
	}
	expectedWorkspace, err := filepath.EvalSymlinks(workspaceB)
	if err != nil {
		t.Fatal(err)
	}
	if patched["securityState"] != nil || patched["contextEpochState"] != nil {
		t.Fatalf("workspace patch exposed private authority in its public response: %#v", patched)
	}
	reboundThread, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	rebound, found, err := turnsecurityapp.LatestContext(reboundThread)
	if err != nil || !found || rebound.WorkspaceRealPath != expectedWorkspace || rebound.ContextEpoch < 2 {
		t.Fatalf("workspace rebind did not commit current authority: context=%#v found=%t err=%v", rebound, found, err)
	}
	if _, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
		Prompt: "complete after the workspace authority rebind", ProviderID: "rebind-provider", Model: "rebind-model",
	}); err != nil {
		t.Fatalf("rebound workspace authority did not reach provider dispatch: %v", err)
	}
	reloaded, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turns := listAny(reloaded["turns"])
	if len(turns) < 2 {
		t.Fatalf("workspace transition and provider turn were not durable: %#v", turns)
	}
	lastTurn, _ := turns[len(turns)-1].(map[string]any)
	frozen, err := domainsecurity.ParseTurnSecurityContext(lastTurn["securityContext"])
	if err != nil || frozen.WorkspaceRealPath != expectedWorkspace || frozen.ContextEpoch < rebound.ContextEpoch || stringField(lastTurn, "status") != "completed" {
		t.Fatalf("provider turn did not retain rebound authority: turn=%#v context=%#v err=%v", lastTurn, frozen, err)
	}
}

func TestResearchTurnMaterializesMissingWorkspaceBeforeAuthorityFreeze(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	researchWorkspace := filepath.Join(parent, "missing-research-workspace")
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "research-provider", BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: "research-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	provider := &countingImmediateWorkspaceProvider{}
	handler.provider = provider
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "research missing workspace", "workspace": researchWorkspace,
		"providerId": "research-provider", "model": "research-model",
	}, researchWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	if _, err := os.Stat(researchWorkspace); !os.IsNotExist(err) {
		t.Fatalf("thread creation unexpectedly materialized the target: %v", err)
	}
	if _, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
		Prompt: "/goal --research verify the missing workspace authority", ProviderID: "research-provider", Model: "research-model",
	}); err != nil {
		t.Fatalf("research turn failed after a missing workspace rebind: %v", err)
	}
	if provider.calls.Load() != 1 {
		t.Fatalf("provider dispatch count mismatch after stable freeze: %d", provider.calls.Load())
	}
	reloaded, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turns := listAny(reloaded["turns"])
	lastTurn, _ := turns[len(turns)-1].(map[string]any)
	frozen, err := domainsecurity.ParseTurnSecurityContext(lastTurn["securityContext"])
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := (filestore.CaseBindingReader{}).Observe(researchWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	if frozen.WorkspaceRealPath != researchWorkspace ||
		frozen.PublicationPolicy.BindingObservationDigest != fresh.ObservationDigest ||
		fresh.State != domainsecurity.CaseBindingStateMissing || stringField(lastTurn, "status") != "completed" {
		t.Fatalf("research turn did not retain the post-materialization authority: context=%#v observation=%#v turn=%#v", frozen, fresh, lastTurn)
	}
	epoch, ok, err := contextepochapp.StateFromThread(reloaded)
	if err != nil || !ok || epoch.AcceptedSnapshot.Epoch != frozen.ContextEpoch {
		t.Fatalf("research turn split epoch and workspace authority: epoch=%#v ok=%t context=%#v err=%v", epoch, ok, frozen, err)
	}
	if _, err := os.Stat(filepath.Join(researchWorkspace, ".analytix", "autoresearch", threadID, "progress.json")); err != nil {
		t.Fatalf("research state was not created after execution authority became valid: %v", err)
	}
	replay, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	turnStartedIndex, researchAuditIndex := -1, -1
	for index, event := range replay.Events {
		switch stringField(event, "kind") {
		case "turn_started":
			turnStartedIndex = index
		case "autoresearch_state_audit":
			researchAuditIndex = index
		}
	}
	if turnStartedIndex < 0 || researchAuditIndex <= turnStartedIndex {
		t.Fatalf("research audit event was not durably ordered after turn start: events=%#v", replay.Events)
	}
}

func TestWorkspaceRebindToMissingTargetCannotMintGeneralAuthority(t *testing.T) {
	workspace := t.TempDir()
	missingWorkspace := filepath.Join(t.TempDir(), "missing-rebind-target")
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "rebind-provider", BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: "rebind-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "missing rebind target", "workspace": workspace,
		"providerId": "rebind-provider", "model": "rebind-model",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handler.runtimeThreadService().Patch(context.Background(), stringField(thread, "id"), map[string]any{
		"workspace": missingWorkspace,
	}); err == nil {
		t.Fatal("missing workspace rebind minted a general publication authority")
	}
	if _, err := os.Stat(missingWorkspace); !os.IsNotExist(err) {
		t.Fatalf("failed workspace rebind changed the missing target: %v", err)
	}
}
