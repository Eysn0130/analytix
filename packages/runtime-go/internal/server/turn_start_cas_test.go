package server

import (
	"errors"
	"testing"

	threadapp "analytix.local/runtime-go/internal/app/thread"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnstartapp "analytix.local/runtime-go/internal/app/turnstart"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestDurableTurnStartCASRejectsStalePreparedAndSecondRunningTurn(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{"title": "turn CAS", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	_, baseline, err := store.ReadThreadStartBaseline(threadID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": "turn-winner", "threadId": threadID, "status": "running", "items": []any{},
	}, "provider-a", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendTurnToThreadIfBaseline(threadID, map[string]any{
		"id": "turn-stale", "threadId": threadID, "status": "running", "items": []any{},
	}, "provider-a", map[string]any{}, workspace, baseline); !errors.Is(err, turnstartapp.ErrBaselineConflict) {
		t.Fatalf("stale prepared turn was not rejected: %v", err)
	}
	reloaded, current, err := store.ReadThreadStartBaseline(threadID)
	if err != nil || len(listAny(reloaded["turns"])) != 1 {
		t.Fatalf("stale CAS changed durable turns: thread=%#v err=%v", reloaded, err)
	}
	if err := store.AppendTurnToThreadIfBaseline(threadID, map[string]any{
		"id": "turn-second", "threadId": threadID, "status": "running", "items": []any{},
	}, "provider-a", map[string]any{}, workspace, current); !errors.Is(err, turnstartapp.ErrBaselineConflict) {
		t.Fatalf("second running turn crossed the high-water check: %v", err)
	}
}

func TestOutputCapTerminalCASWinnerSurvivesDurableHistoryAndLateLoser(t *testing.T) {
	store, threadID, turnID, securityContext := newGeneralTerminalOutboxFixture(t)
	input := turnapp.CommitCompletionInput{Store: store, SecurityContext: securityContext, ThreadID: threadID, TurnID: turnID, TerminalReason: "success",
		FinishedAt: "2026-10-10T00:00:00Z", EndReason: &domainterminal.EndReason{Kind: domainterminal.EndReasonMaxTokens}, EmptyOutputCap: true}
	winner, err := turnapp.CommitCompletedTurn(input)
	if err != nil || !winner.Changed {
		t.Fatalf("cap CAS did not commit: %#v %v", winner, err)
	}
	raw := rawGeneralTerminalThreadForTest(t, store, threadID)
	projected, err := threadapp.ProjectPublicThread(raw)
	if err != nil {
		t.Fatal(err)
	}
	publicTurn := projected["turns"].([]any)[0].(map[string]any)
	if publicTurn["endReason"] == nil || len(listAny(publicTurn["items"])) != 0 {
		t.Fatal("history lost cap or invented assistant")
	}
	replay, err := store.LoadEventsSince(threadID, 0)
	if err != nil || len(replay.Events) != 2 || replay.Events[1]["endReason"] == nil {
		t.Fatalf("durable two-slot cap replay lost: %#v %v", replay, err)
	}
	assertGeneralTerminalEventIDsExactlyOnce(t, replay.Events)
	input.EndReason = nil
	input.EmptyOutputCap = false
	loser, err := turnapp.CommitCompletedTurn(input)
	if err != nil || loser.Changed {
		t.Fatal("late ordinary winner replaced the committed cap")
	}
	replayAfter, err := store.LoadEventsSince(threadID, 0)
	if err != nil || len(replayAfter.Events) != len(replay.Events) || replayAfter.Events[1]["endReason"] == nil {
		t.Fatal("late loser rewrote terminal history")
	}
}
