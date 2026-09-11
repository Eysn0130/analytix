package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestForegroundHandoffBodyIsParentMemoryOnlyAndCannotRehydrateContinuationAuthority(t *testing.T) {
	securityContext, err := securitycontexttest.HostGeneralOnlyExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "foreground-parent", TurnID: "foreground-parent-turn", WorkspaceRealPath: t.TempDir(),
		ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	call := domainmodel.ToolCall{
		ID: serverTestHostToolCallID("foreground-parent"), Name: toolcatalogapp.ForegroundTaskToolName,
		Arguments: json.RawMessage(`{"prompt":"bounded question","max_steps":2,"token_budget":512,"time_budget_ms":30000}`),
	}
	pending := runtimePendingToolCall{Call: call, SecurityContext: securityContext}
	output := map[string]any{
		"kind": "subagent_task", "childRunId": "child-run", "jobId": "child-run", "status": "completed",
		"result":                  "parent-memory-only body",
		"handoffReceiptDigest":    domainsecurity.SHA256Hex([]byte("receipt")),
		"submissionDigest":        domainsecurity.SHA256Hex([]byte("submission")),
		"privacyProjectionDigest": domainsecurity.SHA256Hex([]byte("privacy")),
		"factAnswerAllowed":       false, "evidenceAuthority": false,
		"parentGoalCompletionAllowed": false, "parentTodoCompletionAllowed": false,
	}
	persistable := runtimePersistableToolOutput(pending, output)
	persistableJSON, _ := json.Marshal(persistable)
	if strings.Contains(string(persistableJSON), "parent-memory-only body") || !strings.Contains(string(persistableJSON), "foreground_handoff_projection_invalid") {
		t.Fatalf("forged foreground body entered persistable projection: %s", persistableJSON)
	}
	projection, modelContent := (&runtimeServerHandler{}).prepareRuntimeToolResultForModel(context.Background(), pending, output, false)
	projectionJSON, _ := json.Marshal(projection)
	if strings.Contains(string(projectionJSON), "parent-memory-only body") || strings.Contains(modelContent, "parent-memory-only body") ||
		!strings.Contains(modelContent, "foreground_handoff_projection_invalid") {
		t.Fatalf("unwitnessed foreground body crossed settlement: projection=%s model=%s", projectionJSON, modelContent)
	}

	forged := cloneMap(output)
	delete(forged, "parentTodoCompletionAllowed")
	_, forgedContent := (&runtimeServerHandler{}).prepareRuntimeToolResultForModel(context.Background(), pending, forged, false)
	if strings.Contains(forgedContent, "parent-memory-only body") || !strings.Contains(forgedContent, "foreground_handoff_projection_invalid") {
		t.Fatalf("malformed foreground output reached parent memory: %s", forgedContent)
	}
	injected := cloneMap(output)
	injected["debug"] = "second private body"
	injectedProjection, injectedContent := (&runtimeServerHandler{}).prepareRuntimeToolResultForModel(context.Background(), pending, injected, false)
	injectedJSON, _ := json.Marshal(injectedProjection)
	if strings.Contains(string(injectedJSON), "second private body") || strings.Contains(injectedContent, "second private body") ||
		!strings.Contains(injectedContent, "foreground_handoff_projection_invalid") {
		t.Fatalf("unknown foreground field crossed the closed projection: projection=%s model=%s", injectedJSON, injectedContent)
	}
}
