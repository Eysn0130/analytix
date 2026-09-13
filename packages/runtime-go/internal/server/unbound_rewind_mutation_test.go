package server

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestUnboundRewindCommitsReplacementEpochAndRejectsStaleBindingAfterRestart(t *testing.T) {
	durableRoot := t.TempDir()
	workspace := t.TempDir()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: t.TempDir(),
		ProviderID: "rewind-provider", BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: "rewind-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	handler.provider = immediateMutationWriterProvider{}
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "rewind", "workspace": workspace, "providerId": "rewind-provider", "model": "rewind-model",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	response, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{Prompt: "complete before rewind"})
	if err != nil {
		t.Fatal(err)
	}
	turnID := stringField(response, "turnId")
	before, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	oldContext, found, err := turnsecurityapp.LatestContext(before)
	if err != nil || !found || oldContext.TurnID != turnID {
		t.Fatalf("completed turn current authority mismatch: context=%#v found=%t err=%v", oldContext, found, err)
	}
	beforeRaw := rawGeneralTerminalThreadForTest(t, handler.store, threadID)
	beforeEvents, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	beforeEntries, err := turnapp.PreflightGeneralTerminalPublicationInventoryV1(beforeRaw, beforeEvents.Events)
	if err != nil || len(beforeEntries) != 1 || beforeEntries[0].ArchivedOnly ||
		beforeEntries[0].State != turnapp.GeneralTerminalPublicationCompleteV1 {
		t.Fatalf("completed turn did not establish a live canonical terminal archive: entries=%#v err=%v", beforeEntries, err)
	}
	terminalCommitDigest := beforeEntries[0].Commit.CommitDigest
	probeAt := time.Date(2026, 7, 15, 2, 0, 0, 0, time.UTC)
	probePrepared, err := threadapp.PrepareRewindMutation(beforeRaw, threadID, turnID, oldContext, probeAt)
	if err != nil {
		t.Fatal(err)
	}
	probeBaseline, err := threadapp.MutationBaselineDigest(beforeRaw)
	if err != nil {
		t.Fatal(err)
	}
	probeStored := cloneMap(beforeRaw)
	probeResult, probeErr := threadapp.CommitRewindMutationWithReadWrite(threadapp.RewindMutationCommitRequest{
		ThreadID: threadID, RewindTurnID: turnID, Stamp: probePrepared.Stamp,
		ExpectedBaselineDigest: probeBaseline, ExpectedContextDigest: probePrepared.SecurityContext.ContextDigest,
		CurrentContext: oldContext,
	}, func(string) (map[string]any, error) {
		return cloneMap(probeStored), nil
	}, func(target map[string]any) error {
		probeStored = cloneMap(target)
		delete(probeStored, turnapp.GeneralTerminalPublicationArchiveFieldV1)
		return nil
	})
	if probeErr == nil || probeResult.Committed {
		t.Fatalf("rewind readback accepted a target that lost the terminal archive: result=%#v err=%v", probeResult, probeErr)
	}
	callID := serverTestHostToolCallID("stale_rewind")
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: oldContext, Provider: "host", ServerIdentity: "host:builtin", ToolName: "task", ToolCallID: callID,
		ConnectionEpoch: 0, ArgsHash: domainsecurity.SHA256Hex([]byte("args")), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required",
		IssuedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Minute),
	})
	staleBinding, err := domainjob.NewSecurityBinding(oldContext, grant, callID)
	if err != nil {
		t.Fatal(err)
	}
	rewind, err := handler.runtimeThreadService().Rewind(context.Background(), threadID, turnID)
	if err != nil {
		t.Fatal(err)
	}
	authorityTurnID := stringField(rewind, "authorityTurnId")
	if authorityTurnID == "" || authorityTurnID == turnID {
		t.Fatalf("rewind did not return replacement authority: %#v", rewind)
	}
	after, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turns := listAny(after["turns"])
	if len(turns) != 1 || stringField(turns[0].(map[string]any), "id") != authorityTurnID || stringField(turns[0].(map[string]any), "kind") != "rewind_transition" {
		t.Fatalf("rewind did not atomically replace removed history with host authority: %#v", turns)
	}
	afterRaw := rawGeneralTerminalThreadForTest(t, handler.store, threadID)
	afterEvents, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	afterEntries, err := turnapp.PreflightGeneralTerminalPublicationInventoryV1(afterRaw, afterEvents.Events)
	if err != nil || len(afterEntries) != 1 || !afterEntries[0].ArchivedOnly ||
		afterEntries[0].State != turnapp.GeneralTerminalPublicationCompleteV1 ||
		afterEntries[0].Commit.CommitDigest != terminalCommitDigest {
		t.Fatalf("rewind did not retain the exact archived terminal authority: entries=%#v err=%v", afterEntries, err)
	}
	assertGeneralTerminalUsageIndexExactlyOnce(t, handler.store, threadID, afterEntries[0].Events)
	if plans, err := handler.store.PreflightGeneralTerminalPublicationRecoveryV1(); err != nil ||
		len(plans) != 1 || len(plans[0].RepairTurns) != 0 {
		t.Fatalf("rewind left a terminal recovery gap: plans=%#v err=%v", plans, err)
	}
	current, found, err := turnsecurityapp.LatestContext(after)
	if err != nil || !found || current.TurnID != authorityTurnID || current.ContextEpoch <= oldContext.ContextEpoch {
		t.Fatalf("rewind replacement context mismatch: context=%#v found=%t err=%v", current, found, err)
	}
	state, ok, err := contextepochapp.StateFromThread(after)
	if err != nil || !ok || state.AcceptedSnapshot.Epoch != current.ContextEpoch {
		t.Fatalf("rewind split context epoch authority: state=%#v ok=%t err=%v", state, ok, err)
	}
	staleCtx, staleCancel := context.WithCancel(context.Background())
	if handler.runtimeSubagentState().RegisterBoundBackgroundJob("stale-rewind-result", staleBinding, staleCancel) {
		t.Fatal("old rewind binding was admitted after replacement epoch commit")
	}
	select {
	case <-staleCtx.Done():
	default:
		t.Fatal("rejected stale rewind result was not cancelled")
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
	if err != nil || !found || restartedContext != current {
		t.Fatalf("restart lost rewind replacement context: context=%#v found=%t err=%v", restartedContext, found, err)
	}
	restartedState, ok, err := contextepochapp.StateFromThread(restartedThread)
	if err != nil || !ok || restartedState.StateDigest != state.StateDigest {
		t.Fatalf("restart lost rewind epoch state: state=%#v ok=%t err=%v", restartedState, ok, err)
	}
	restartedRaw := rawGeneralTerminalThreadForTest(t, restarted, threadID)
	restartedTurns := listAny(restartedRaw["turns"])
	if len(restartedTurns) != 1 || stringField(restartedTurns[0].(map[string]any), "id") != authorityTurnID ||
		stringField(restartedTurns[0].(map[string]any), "kind") != "rewind_transition" {
		t.Fatalf("restart lost the exact durable rewind authority: %#v", restartedTurns)
	}
	restartedEvents, err := restarted.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	restartedEntries, err := turnapp.PreflightGeneralTerminalPublicationInventoryV1(restartedRaw, restartedEvents.Events)
	if err != nil || len(restartedEntries) != 1 || !restartedEntries[0].ArchivedOnly ||
		restartedEntries[0].State != turnapp.GeneralTerminalPublicationCompleteV1 ||
		restartedEntries[0].Commit.CommitDigest != terminalCommitDigest {
		t.Fatalf("restart lost rewound terminal archive: entries=%#v err=%v", restartedEntries, err)
	}
	if plans, err := restarted.PreflightGeneralTerminalPublicationRecoveryV1(); err != nil ||
		len(plans) != 1 || len(plans[0].RepairTurns) != 0 {
		t.Fatalf("restart found a rewound terminal recovery gap: plans=%#v err=%v", plans, err)
	}
	eventBytesBeforePrimaryLoss, err := os.ReadFile(restarted.eventsPath(threadID))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(restarted.threadPath(threadID)); err != nil {
		t.Fatal(err)
	}
	recovered, err := NewTempDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	recoveredThread, err := recovered.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	recoveredTurns := listAny(recoveredThread["turns"])
	// Sidecar fallback is a public read view. The internal authority marker is
	// retained in the primary above, but must not become a prompt-less public
	// conversation turn or authorize recovery after the primary is lost.
	if len(recoveredTurns) != 0 {
		t.Fatalf("sidecar recovery exposed internal authority or resurrected rewound history: %#v", recoveredTurns)
	}
	if domainevent.ContainsPrivateTerminalAuthority(recoveredThread) {
		t.Fatalf("public sidecar recovered private terminal authority: %#v", recoveredThread)
	}
	if plans, err := recovered.PreflightGeneralTerminalPublicationRecoveryV1(); err == nil || plans != nil {
		t.Fatalf("missing primary authority with terminal markers did not fail closed: plans=%#v err=%v", plans, err)
	}
	eventBytesAfterPrimaryLoss, err := os.ReadFile(recovered.eventsPath(threadID))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(eventBytesBeforePrimaryLoss, eventBytesAfterPrimaryLoss) {
		t.Fatal("missing-primary preflight synthesized or rewrote terminal events")
	}
}
