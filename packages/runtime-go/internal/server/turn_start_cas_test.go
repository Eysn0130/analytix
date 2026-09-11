package server

import (
	"errors"
	"testing"

	turnstartapp "analytix.local/runtime-go/internal/app/turnstart"
)

func TestDurableTurnStartCASRejectsStalePreparedAndSecondRunningTurn(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
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
