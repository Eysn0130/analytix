package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestDurableCaseCheckpointEventsPersistAndPublishMetadataOnly(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{"title": "case checkpoint audit", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	securityContext := newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: "turn-case-checkpoint-audit", WorkspaceRealPath: workspace, CaseID: "case-checkpoint-audit",
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("case-checkpoint-audit-binding")),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("case-checkpoint-audit-manifest")),
		ContextEpoch:       5, IssuedAt: time.Date(2026, 7, 15, 4, 0, 0, 0, time.UTC),
	})
	contextBody, _ := json.Marshal(securityContext)
	contextRecord := map[string]any{}
	if err := json.Unmarshal(contextBody, &contextRecord); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": securityContext.TurnID, "threadId": threadID, "status": "completed", "securityContext": contextRecord, "items": []any{},
	}, "deepseek", map[string]any{"securityState": contextRecord}); err != nil {
		t.Fatal(err)
	}

	checkpointID := domaincheckpointref.RuntimeID("workspace/checkpoint:durable-case")
	planID := domaincheckpointref.PlanID(checkpointID)
	rescueID := domaincheckpointref.RescueID(checkpointID)
	applyID := domaincheckpointref.ApplyID(checkpointID)
	createdAt := "2026-07-15T04:00:01Z"
	rawEvents := []map[string]any{
		{
			"kind": "checkpoint_captured", "threadId": threadID, "turnId": securityContext.TurnID,
			"checkpoint": map[string]any{
				"schemaVersion": float64(1), "checkpointId": checkpointID, "threadId": threadID,
				"turnId": securityContext.TurnID, "createdAt": createdAt, "status": "captured",
				"changedFileCount": float64(1), "snapshotStorage": "runtime_private_cas",
			},
		},
		{
			"kind": "checkpoint_rewind_rescue_created", "threadId": threadID,
			"rescue": map[string]any{
				"schemaVersion": float64(1), "rescueId": rescueID, "planId": planID, "checkpointId": checkpointID,
				"threadId": threadID, "createdAt": createdAt, "fileCount": float64(1), "storage": "runtime_private_sidecar",
			},
		},
		{
			"kind": "checkpoint_rewind_applied", "threadId": threadID,
			"apply": map[string]any{
				"schemaVersion": float64(1), "applyId": applyID, "planId": planID, "checkpointId": checkpointID,
				"threadId": threadID, "createdAt": createdAt, "scope": "code", "status": "applied",
				"destructive": true, "fileCount": float64(1), "conversationStatus": "not_requested",
				"summary": map[string]any{
					"fileAppliedCount": float64(1), "fileNoopCount": float64(0), "fileManualReviewCount": float64(0),
					"fileBlockedCount": float64(0), "fileFailedCount": float64(0),
				},
			},
		},
	}
	live, unsubscribe := store.SubscribeEvents(threadID)
	defer unsubscribe()
	for _, raw := range rawEvents {
		recorded, _, err := store.RecordEvent(raw)
		if err != nil {
			t.Fatalf("record case checkpoint audit: %v", err)
		}
		assertCaseCheckpointAuditIsPublicMetadataOnly(t, recorded, checkpointID, planID, rescueID, applyID)
		select {
		case event := <-live:
			assertCaseCheckpointAuditIsPublicMetadataOnly(t, event, checkpointID, planID, rescueID, applyID)
		case <-time.After(time.Second):
			t.Fatal("case checkpoint audit was not published after durable append")
		}
	}
	replay, err := store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	checkpointCount := 0
	for _, event := range replay.Events {
		if strings.HasPrefix(stringField(event, "kind"), "checkpoint_") {
			checkpointCount++
			assertCaseCheckpointAuditIsPublicMetadataOnly(t, event, checkpointID, planID, rescueID, applyID)
		}
	}
	if checkpointCount != len(rawEvents) {
		t.Fatalf("durable case checkpoint audit count mismatch: got=%d want=%d", checkpointCount, len(rawEvents))
	}
}

func assertCaseCheckpointAuditIsPublicMetadataOnly(t *testing.T, event map[string]any, privateValues ...string) {
	t.Helper()
	body, _ := json.Marshal(event)
	for _, forbidden := range append(privateValues,
		"checkpointId", "planId", "applyId", "rescueId", "workspace", "relativePath", "beforeHash", "afterHash", "content",
		"executionGrant", "publicationReceipt", "publicationAuthority") {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("durable case checkpoint audit leaked %q: %s", forbidden, body)
		}
	}
	if !strings.Contains(string(body), `"projectionKind":"checkpoint_status"`) ||
		!strings.Contains(string(body), `"disclosure":"metadata_only"`) ||
		!strings.Contains(string(body), `"factAnswerAllowed":false`) ||
		!strings.Contains(string(body), `"evidenceAuthority":false`) {
		t.Fatalf("durable case checkpoint audit is not closed metadata: %s", body)
	}
}
