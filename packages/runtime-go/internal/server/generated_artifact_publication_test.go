package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	generationapp "analytix.local/runtime-go/internal/app/documentgeneration"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	"analytix.local/runtime-go/internal/provider"
)

func TestGeneratedArtifactTypedReceiptSettlesDurablyAndReplaysSSE(t *testing.T) {
	for _, kind := range []string{"docx", "xlsx", "pptx"} {
		t.Run(kind, func(t *testing.T) { testGeneratedArtifactPublication(t, kind) })
	}
}
func testGeneratedArtifactPublication(t *testing.T, kind string) {
	const decimalHash = "93887a2fccaf8b3b1b51350c0abfc65390966231128774816dce99bc362505e4"
	receipt := generationapp.CreatedReceipt{
		ArtifactID: decimalHash, Kind: kind, ContentHash: decimalHash, ByteSize: 1200,
		SavedAt: "2026-09-12T12:35:15.123456789Z",
	}
	durableRoot, workspace := t.TempDir(), t.TempDir()
	store, err := NewTempDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	handler := &runtimeServerHandler{store: store, publicProjector: threadapp.EnsurePublicProjector(nil)}
	configureServerGeneralExecution(t, handler)
	thread, err := store.CreateThread(map[string]any{"title": "Generated artifact", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID, turnID := stringField(thread, "id"), "turn_generated_artifact"
	now := time.Now().UTC()
	securityContext, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: context.Background(), Authority: handler.turnSecurity, Thread: thread,
		ThreadID: threadID, TurnID: turnID, Workspace: workspace,
		Principal: testIdentityPrincipal(), IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	generationArgs := map[string]any{"kind": kind, "path": "report." + kind}
	switch kind {
	case "docx":
		generationArgs["markdown"] = "# Report"
	case "xlsx":
		generationArgs["workbook"] = map[string]any{"sheets": []any{map[string]any{"id": "s", "name": "数据", "cells": []any{map[string]any{"address": "A1", "type": "number", "value": 42}}}}}
	case "pptx":
		generationArgs["presentation"] = map[string]any{"slides": []any{map[string]any{"id": "s", "objects": []any{map[string]any{"id": "t", "kind": "text", "x": 0, "y": 0, "w": 1, "h": 1, "text": "Report"}}}}}
	}
	call := provider.ToolCall{
		ID: serverTestHostToolCallID("generated-artifact-publication"), Name: "generate_office_document",
		Arguments: json.RawMessage(mustHostedJSON(t, generationArgs)),
	}
	args, err := domainsecurity.DecodeCanonicalJSONObject(call.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := generationapp.ParseRequest(args); err != nil {
		t.Fatalf("fixture generation request is invalid: %v", err)
	}
	scope := []string{call.Name}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "test-provider", ServerIdentity: "host:builtin",
		ToolName: call.Name, ToolCallID: call.ID, ArgsHash: executiongrantapp.ArgumentsHash(call.Arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("generated-artifact-schema")),
		ScopeHash:  executiongrantapp.ScopeHash(scope), ReadOnly: false, ApprovalState: "not_required", IssuedAt: now,
	})
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	epochState, err := contextepochapp.BootstrapState(threadID, securityContext.ContextEpoch,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(securityContext)}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "createdAt": now.Format(time.RFC3339Nano),
		"securityContext": securityRecord, "items": []any{},
	}, "test-provider", map[string]any{"securityState": securityRecord, "contextEpochState": contextepochapp.PublicState(epochState)}); err != nil {
		t.Fatal(err)
	}
	callItemID, err := handler.persistToolCallReady(context.Background(), threadID, turnID, call, 1, securityContext, grant)
	if err != nil {
		t.Fatal(err)
	}
	pending := runtimePendingToolCall{
		ThreadID: threadID, TurnID: turnID, Workspace: workspace, ProviderID: "test-provider",
		Call: call, ToolCallItemID: callItemID, SecurityContext: securityContext, ExecutionGrant: grant,
		ToolScope: scope, ApprovalPolicy: "auto", SandboxMode: "workspace-write",
	}
	if _, err := executiongrantapp.AuthorizePendingRegistry(context.Background(), handler.turnSecurity,
		filestore.CaseBindingReader{}, store, nil, pending, "", time.Now().UTC()); err != nil {
		t.Fatalf("fixture execution authority is invalid: %v", err)
	}
	settled, err := handler.settleRuntimeToolOutput(context.Background(), pending, receipt, false)
	if err != nil {
		t.Fatalf("typed artifact settlement failed: %v", err)
	}
	want := &domaintoolresult.ArtifactStatusV1{
		ArtifactID: receipt.ArtifactID, Kind: receipt.Kind, ContentHash: receipt.ContentHash,
		ByteSize: receipt.ByteSize, SavedAt: receipt.SavedAt,
	}
	assertArtifact := func(output any) {
		t.Helper()
		projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(output)
		if err != nil || projection.ProjectionKind != domaintoolresult.ProjectionArtifactStatus || !reflect.DeepEqual(projection.Artifact, want) {
			t.Fatalf("artifact metadata did not survive settlement/publication: %v", err)
		}
	}
	if settled.IsError {
		t.Fatalf("typed receipt became an error settlement: %#v", settled.Output)
	}
	assertArtifact(settled.Output)
	if !strings.Contains(settled.Message.Content, receipt.ArtifactID) {
		t.Fatal("current provider reply lost the created artifact receipt")
	}
	for _, restart := range []bool{false, true} {
		if restart {
			store, err = NewTempDurableEventSessionStore(durableRoot)
			if err != nil {
				t.Fatal(err)
			}
			handler.store = store
		}
		stored, err := store.GetThread(threadID)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, rawTurn := range stored["turns"].([]any) {
			for _, rawItem := range rawTurn.(map[string]any)["items"].([]any) {
				item := rawItem.(map[string]any)
				if stringField(item, "kind") == "tool_result" && stringField(item, "callId") == call.ID {
					assertArtifact(item["output"])
					found = true
				}
			}
		}
		if !found {
			t.Fatal("durable artifact result missing")
		}
		response := httptest.NewRecorder()
		handler.handleThreadEvents(response, httptest.NewRequest("GET", "/events?since_seq=0", nil), threadID)
		found = false
		for _, line := range strings.Split(response.Body.String(), "\n") {
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			var event map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
				t.Fatal(err)
			}
			if stringField(event, "kind") != "tool_call_finished" {
				continue
			}
			item, _ := event["item"].(map[string]any)
			if stringField(item, "callId") == call.ID {
				assertArtifact(item["output"])
				found = true
			}
		}
		if !found {
			t.Fatalf("artifact missing from SSE replay (restart=%v, HTTP=%d)", restart, response.Code)
		}
	}
}
