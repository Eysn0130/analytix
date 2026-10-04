package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	evidenceregistry "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	gateprojection "analytix.local/runtime-go/internal/app/gateprojection"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestPatchTurnItemStatusRejectsAcceptedFinalWithoutWriting(t *testing.T) {
	root := workspacetest.New(t)
	durableRoot := filepath.Join(root, "durable")
	privateRoot := filepath.Join(root, "private")
	store, err := NewTempDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{"title": "immutable accepted final"}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_immutable_final"
	securityContext := newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace, CaseID: "case-immutable-final",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")),
		ContextEpoch: 5, IssuedAt: time.Unix(30, 0),
	})
	contextBody, err := json.Marshal(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	contextRecord := map[string]any{}
	if err := json.Unmarshal(contextBody, &contextRecord); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "securityContext": contextRecord,
		"items": []any{map[string]any{
			"id": "tool_immutable", "threadId": threadID, "turnId": turnID,
			"kind": "tool_call", "status": "completed",
		}},
	}, "deepseek", map[string]any{"securityState": contextRecord}); err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(privateRoot, "authority", "final-answer-ed25519-v1.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := evidenceregistry.NewStore(filepath.Join(privateRoot, "evidence-registry"), authority)
	if err != nil {
		t.Fatal(err)
	}
	privateStore, err := newServerTestPrivateFinalStore(t, filepath.Join(privateRoot, "accepted-finals"))
	if err != nil {
		t.Fatal(err)
	}
	casReader, err := finalauthority.NewAcceptedFinalCASReader(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	eventIO := durableAcceptedFinalEventIO(store, casReader)
	index := gateprojection.NewTrustedFinalProjectionIndexWithReadback(authority, eventIO.Readback)
	finalizer := evidenceapp.NewCasePublicationFinalizerWithPublicationSnapshots(
		registry, registry, authority, privateStore, eventIO,
		newServerTestTurnTerminalCoordinator(t, authority, privateStore), nil, index,
	)
	ordinarySlot, err := domainordinaryresult.NewResultSlotV1("Updated the immutability test fixture.")
	if err != nil {
		t.Fatal(err)
	}
	result, err := finalizer.PersistBoundary(context.Background(), evidenceapp.PersistCaseBoundaryInput{
		Store: store, Context: securityContext, TerminalReason: evidenceapp.TerminalSuccess,
		OrdinaryResult: &ordinarySlot, CaseSlotIntent: evidenceapp.CaseSlotNotRequestedV1,
		ThreadID: threadID, TurnID: turnID, AcceptedAt: time.Unix(31, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	finalTurn := primaryTurnByID(t, before, turnID)
	acceptedFinal, ok := finalTurn["acceptedFinal"].(map[string]any)
	if stringField(finalTurn, "status") != "completed" || !ok ||
		stringField(acceptedFinal, "recordDigest") != result.Persistence.AcceptedFinal.RecordDigest {
		t.Fatal("fixture did not persist the completed accepted final")
	}
	completedTool := false
	items, _ := finalTurn["items"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if stringField(item, "id") == "tool_immutable" && stringField(item, "kind") == "tool_call" && stringField(item, "status") == "completed" {
			completedTool = true
		}
	}
	if !completedTool {
		t.Fatal("fixture did not preserve the completed tool")
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
