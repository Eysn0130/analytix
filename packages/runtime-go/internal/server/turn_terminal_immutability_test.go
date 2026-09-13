package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	turnapp "analytix.local/runtime-go/internal/app/turn"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestPatchTurnItemStatusRejectsAcceptedFinalWithoutWriting(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{"title": "immutable accepted final"}, workspacetest.New(t))
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_immutable_final"
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "completed",
		"acceptedFinal": map[string]any{"recordDigest": "host-signed-authority"},
		"items": []any{map[string]any{
			"id": "tool_immutable", "threadId": threadID, "turnId": turnID,
			"kind": "tool_call", "status": "completed",
		}},
	}, "deepseek", nil); err != nil {
		t.Fatal(err)
	}
	before, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	beforeBody, _ := json.Marshal(before)
	if err := store.PatchTurnItemStatus(threadID, turnID, "tool_immutable", "failed"); !errors.Is(err, turnapp.ErrAcceptedFinalImmutable) {
		t.Fatalf("late accepted-final patch was not rejected: %v", err)
	}
	after, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	afterBody, _ := json.Marshal(after)
	if !bytes.Equal(afterBody, beforeBody) {
		t.Fatalf("rejected accepted-final patch changed durable thread: before=%s after=%s", beforeBody, afterBody)
	}
}
