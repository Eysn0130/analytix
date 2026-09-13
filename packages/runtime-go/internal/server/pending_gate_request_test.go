package server

import (
	"testing"

	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestRecordPendingGateRequestIsIdempotentAndRejectsConflicts(t *testing.T) {
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
		"kind": "approval_requested", "threadId": threadID, "turnId": "turn-a",
		"itemId": "item-a", "approvalId": "approval-a", "status": "pending",
		"toolName": "write_file", "approvalPolicy": "on-request", "sandboxMode": "workspace-write",
		"summary": "Approve write_file", "continuationReceiptId": "receipt-a",
	}
	if err := handler.recordPendingGateRequest(event); err != nil {
		t.Fatal(err)
	}
	if err := handler.recordPendingGateRequest(event); err != nil {
		t.Fatalf("exact request retry was not idempotent: %v", err)
	}
	replay, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil || len(replay.Events) != 1 {
		t.Fatalf("exact retry duplicated the event: events=%#v err=%v", replay.Events, err)
	}
	for name, patch := range map[string]map[string]any{
		"item":    {"itemId": "item-other"},
		"receipt": {"continuationReceiptId": "receipt-other"},
		"tool":    {"toolName": "other_tool", "summary": "Approve other_tool"},
	} {
		conflicting := cloneMap(event)
		for key, value := range patch {
			conflicting[key] = value
		}
		if err := handler.recordPendingGateRequest(conflicting); err == nil {
			t.Fatalf("%s conflict was accepted: %#v", name, conflicting)
		}
	}
	for _, key := range []string{"threadId", "turnId", "approvalId"} {
		invalid := cloneMap(event)
		delete(invalid, key)
		if err := handler.recordPendingGateRequest(invalid); err == nil {
			t.Fatalf("request without %s was accepted", key)
		}
	}
}
