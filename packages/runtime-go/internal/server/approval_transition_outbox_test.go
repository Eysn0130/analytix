package server

import (
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestRecordApprovalGrantTransitionIsExactAndRejectsConflicts(t *testing.T) {
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
		"kind": "execution_grant_approved", "threadId": threadID, "turnId": "turn-a",
		"approvalId": "appr-a", "approvalTransitionId": domainsecurity.SHA256Hex([]byte("transition-a")),
		"continuationReceiptId":     domainsecurity.SHA256Hex([]byte("receipt-a")),
		"continuationDispositionId": domainsecurity.SHA256Hex([]byte("disposition-a")),
		"itemId":                    "item-a", "executionGrantId": domainsecurity.SHA256Hex([]byte("grant-a")),
	}
	if err := handler.recordApprovalGrantTransition(event); err != nil {
		t.Fatal(err)
	}
	if err := handler.recordApprovalGrantTransition(event); err != nil {
		t.Fatalf("exact transition retry was not idempotent: %v", err)
	}
	replay, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil || len(replay.Events) != 1 {
		t.Fatalf("exact retry duplicated the event: events=%#v err=%v", replay.Events, err)
	}
	conflict := cloneMap(event)
	conflict["approvalTransitionId"] = domainsecurity.SHA256Hex([]byte("transition-b"))
	if err := handler.recordApprovalGrantTransition(conflict); err == nil {
		t.Fatal("same approval accepted a conflicting grant transition")
	}
}
