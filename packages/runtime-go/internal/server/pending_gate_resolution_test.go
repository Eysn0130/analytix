package server

import (
	"testing"

	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestRecordPendingGateResolutionIsIdempotentAndRejectsConflicts(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "provider-a", BaseURL: "https://provider.invalid", APIKey: "test-only",
		Model: "model-a", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	workspace := workspacetest.New(t)
	thread, err := handler.store.CreateThread(map[string]any{"workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	event := map[string]any{
		"kind": "approval_resolved", "threadId": threadID, "turnId": "turn-a",
		"itemId": "item-a", "approvalId": "approval-a", "status": "allowed",
	}
	if err := handler.recordPendingGateResolution(event); err != nil {
		t.Fatal(err)
	}
	if err := handler.recordPendingGateResolution(event); err != nil {
		t.Fatalf("exact resolution retry was not idempotent: %v", err)
	}
	replay, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil || len(replay.Events) != 1 {
		t.Fatalf("exact retry duplicated the event: events=%#v err=%v", replay.Events, err)
	}
	for name, patch := range map[string]map[string]any{
		"status":      {"status": "denied"},
		"cancelledBy": {"cancelledBy": "runtime_shutdown"},
		"item":        {"itemId": "item-other"},
	} {
		conflicting := make(map[string]any, len(event)+1)
		for key, value := range event {
			conflicting[key] = value
		}
		for key, value := range patch {
			conflicting[key] = value
		}
		if err := handler.recordPendingGateResolution(conflicting); err == nil {
			t.Fatalf("%s conflict was accepted: %#v", name, conflicting)
		}
	}
	for _, key := range []string{"threadId", "turnId", "itemId", "approvalId", "status"} {
		invalid := make(map[string]any, len(event))
		for field, value := range event {
			invalid[field] = value
		}
		delete(invalid, key)
		if err := handler.recordPendingGateResolution(invalid); err == nil {
			t.Fatalf("resolution without %s was accepted", key)
		}
	}
}
