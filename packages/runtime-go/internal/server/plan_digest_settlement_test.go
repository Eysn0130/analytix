package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	provider "analytix.local/runtime-go/internal/provider"
)

func TestCreatePlanPhysicalWriteSettlesDecimalRunDigestAndSurvivesRestart(t *testing.T) {
	const (
		markdown     = "# Generated plan\n\nValidation nonce: 30"
		expectedHash = "93887a2fccaf8b3b1b51350c0abfc65390966231128774816dce99bc362505e4"
		relativePath = ".analytixsdd/plan/validation.md"
	)
	digest := sha256.Sum256([]byte(markdown))
	if actual := hex.EncodeToString(digest[:]); actual != expectedHash {
		t.Fatalf("regression fixture digest changed: %s", actual)
	}

	durableRoot := t.TempDir()
	store, err := NewTempDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	handler := &runtimeServerHandler{store: store}
	thread, err := store.CreateThread(map[string]any{
		"id": "thread_plan_digest_settlement", "title": "Plan digest settlement", "workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_plan_digest_settlement"
	now := time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC)
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		SourceManifestHash: turnsecurityapp.SourceManifestHash(nil), ContextEpoch: 1, IssuedAt: now,
	})
	args := map[string]any{
		"markdown": markdown, "operation": "draft", "plan_relative_path": relativePath,
		"source_request": "record the accepted implementation plan",
	}
	argumentBody, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	call := provider.ToolCall{
		ID: serverTestHostToolCallID("plan-digest-settlement"), Name: "create_plan", Arguments: argumentBody,
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "test-provider", ServerIdentity: "host:builtin",
		ToolName: call.Name, ToolCallID: call.ID, ArgsHash: domainsecurity.CanonicalJSONHash(argumentBody),
		SchemaHash: domainsecurity.SHA256Hex([]byte("create-plan-schema")),
		ScopeHash:  domainsecurity.SHA256Hex([]byte("create-plan-scope")),
		ReadOnly:   false, ApprovalState: "not_required", IssuedAt: now,
	})
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "createdAt": now.Format(time.RFC3339Nano),
		"securityContext": securityRecord, "items": []any{},
	}, "test-provider", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatal(err)
	}
	toolCallItemID, err := handler.persistToolCallReady(
		context.Background(), threadID, turnID, call, 1, securityContext, grant,
	)
	if err != nil {
		t.Fatal(err)
	}

	prepared, failure, failed := filestore.PrepareCreatePlanTool(filestore.CreatePlanToolInput{
		Workspace: workspace, Mode: "plan", SandboxMode: "workspace-write", Args: args,
		Now: func() time.Time { return now },
	})
	if failed {
		t.Fatalf("prepare create_plan failed: %#v", failure)
	}
	output, isError := filestore.ExecutePreparedCreatePlanTool(prepared)
	if isError {
		t.Fatalf("physical create_plan failed: %#v", output)
	}
	projection := toolcatalogapp.BuildPublicToolResultProjectionV1(call.Name, output, false)
	if projection.ProjectionKind != domaintoolresult.ProjectionPlanStatus || projection.Status != "completed" {
		t.Fatalf("create_plan did not produce a completed plan projection: %#v", projection)
	}
	pending := runtimePendingToolCall{
		ThreadID: threadID, TurnID: turnID, Workspace: workspace, Mode: "plan",
		ApprovalPolicy: "auto", SandboxMode: "workspace-write", ProviderID: "test-provider",
		Call: call, ToolCallItemID: toolCallItemID, SecurityContext: securityContext, ExecutionGrant: grant,
	}
	if err := handler.persistRuntimeToolResult(pending, projection, evidenceapp.PreparedToolSettlement{
		Output: output, IsError: false,
	}); err != nil {
		stored, _ := store.GetThread(threadID)
		resultPresent := false
		storedTurns, _ := stored["turns"].([]any)
		if len(storedTurns) > 0 {
			storedTurn, _ := storedTurns[0].(map[string]any)
			storedItems, _ := storedTurn["items"].([]any)
			for _, raw := range storedItems {
				item, _ := raw.(map[string]any)
				resultPresent = resultPresent || stringField(item, "kind") == "tool_result"
			}
		}
		t.Fatalf("persist create_plan result after physical write: %v (durableResult=%t)", err, resultPresent)
	}
	replay, err := store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(replay.Diagnostics) != 0 {
		t.Fatalf("persisted plan settlement produced replay diagnostics: %#v", replay.Diagnostics)
	}
	if body, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(relativePath))); err != nil || string(body) != markdown {
		t.Fatalf("physical plan bytes mismatch: err=%v", err)
	}

	assertRestartedPlanDigestV1(t, store, threadID, turnID, call.ID, expectedHash)
	reopened, err := NewTempDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	assertRestartedPlanDigestV1(t, reopened, threadID, turnID, call.ID, expectedHash)
}

func assertRestartedPlanDigestV1(
	t *testing.T,
	store *DurableEventSessionStore,
	threadID, turnID, callID, expectedHash string,
) {
	t.Helper()
	thread, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turns, _ := thread["turns"].([]any)
	turn, _ := turns[0].(map[string]any)
	items, _ := turn["items"].([]any)
	wantedID := domaintoolresult.ToolResultItemIDV1(turnID, callID)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if stringField(item, "id") != wantedID {
			continue
		}
		projection, parseErr := domaintoolresult.ParsePublicToolResultProjectionV1(item["output"])
		if parseErr != nil {
			t.Fatalf("durable plan projection is not closed: %v", parseErr)
		}
		if projection.Plan == nil || projection.Plan.ContentHash != expectedHash {
			t.Fatalf("durable plan digest changed: %#v", projection.Plan)
		}
		return
	}
	t.Fatalf("durable result %s was not found", wantedID)
}
