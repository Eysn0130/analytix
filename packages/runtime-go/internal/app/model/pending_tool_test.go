package model

import (
	"encoding/json"
	"testing"
	"time"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestPendingToolCallRestoresOnlyByExactCallAndItemID(t *testing.T) {
	turn := map[string]any{
		"items": []any{
			map[string]any{
				"id":        "item_other",
				"kind":      "tool_call",
				"callId":    "call_other",
				"toolName":  "bash",
				"arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1(),
			},
			map[string]any{
				"id":        "item_target",
				"kind":      "tool_call",
				"callId":    "call_target",
				"toolName":  "read_file",
				"arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1(),
			},
		},
	}
	callID, toolName, itemID, ok := ToolCallIdentityFromTurn(turn, "call_target", "item_target", "read_file")
	if !ok || itemID != "item_target" || callID != "call_target" || toolName != "read_file" {
		t.Fatalf("unexpected restored public tool identity ok=%v item=%q call=%q tool=%q", ok, itemID, callID, toolName)
	}
}

func TestPendingToolCallRejectsMissingCallOrItemID(t *testing.T) {
	turn := map[string]any{
		"items": []any{
			map[string]any{
				"id":        "item_named",
				"kind":      "tool_call",
				"callId":    "call_named",
				"toolName":  "wait",
				"arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1(),
			},
		},
	}
	if callID, toolName, itemID, ok := ToolCallIdentityFromTurn(turn, "", "item_named", "wait"); ok {
		t.Fatalf("missing call id must fail closed, item=%q call=%q tool=%q", itemID, callID, toolName)
	}
	if callID, toolName, itemID, ok := ToolCallIdentityFromTurn(turn, "call_named", "", "wait"); ok {
		t.Fatalf("missing item id must fail closed, item=%q call=%q tool=%q", itemID, callID, toolName)
	}
	if callID, toolName, itemID, ok := ToolCallIdentityFromTurn(turn, "call_named", "item_other", "wait"); ok {
		t.Fatalf("mismatched item id must fail closed, item=%q call=%q tool=%q", itemID, callID, toolName)
	}
}

func TestTurnByIDAndGateFieldFallbacks(t *testing.T) {
	thread := map[string]any{
		"turns": []any{
			map[string]any{"id": "turn_1"},
			map[string]any{"id": "turn_2"},
		},
	}
	if turn, ok := TurnByID(thread, " turn_2 "); !ok || turn["id"] != "turn_2" {
		t.Fatalf("unexpected turn lookup ok=%v turn=%#v", ok, turn)
	}
	primary := map[string]any{"enabled": true}
	fallback := map[string]any{"enabled": false, "limit": json.Number("12")}
	if !BoolFieldWithFallback(primary, fallback, "enabled") {
		t.Fatal("primary bool should win")
	}
	limit := OptionalIntField(primary, fallback, "limit")
	if limit == nil || *limit != 12 {
		t.Fatalf("fallback numeric value mismatch: %#v", limit)
	}
}

func TestBuildPendingToolCallFromGateEvent(t *testing.T) {
	callID := modelTestHostToolCallID("pending-tool-build")
	thread := map[string]any{
		"turns": []any{
			map[string]any{
				"id":                    "turn-1",
				"workspaceCheckpointId": "checkpoint-turn",
				"guiPlan":               map[string]any{"source": "turn"},
				"items": []any{
					map[string]any{
						"id":        "item_tool",
						"kind":      "tool_call",
						"callId":    callID,
						"toolName":  "edit_file",
						"arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1(),
					},
				},
			},
		},
	}
	turn, ok := TurnByID(thread, "turn-1")
	if !ok {
		t.Fatal("turn should exist")
	}
	event := map[string]any{
		"threadId":               "thread-1",
		"turnId":                 "turn-1",
		"callId":                 callID,
		"workspaceCheckpointId":  "checkpoint-event",
		"disableUserInput":       true,
		"maxModelSteps":          float64(3),
		"effectiveMaxModelSteps": json.Number("5"),
		"subagentDepth":          float64(2),
	}
	event = pendingSecurityEvent(event, "/workspace", "deepseek", "edit_file", "item_tool", `{"path":"main.go"}`)
	messages := []domainmodel.Message{{Role: "user", Content: "hello"}}
	pending, ok := BuildPendingToolCall(PendingToolCallInput{
		Event:  event,
		Thread: thread,
		Turn:   turn,
		ProviderConfig: domainmodel.TurnConfig{
			ProviderID: "deepseek", Model: "deepseek-chat", APIKey: "synthetic-pending-credential",
			ProxyURL: "http://stale-proxy.invalid",
		},
		ProviderID:       "deepseek",
		Model:            "deepseek-chat",
		Effort:           "high",
		Workspace:        "/workspace",
		ApprovalPolicy:   "on-request",
		SandboxMode:      "workspace-write",
		Mode:             "agent",
		Messages:         messages,
		ToolScope:        []string{"edit_file"},
		PrivateArguments: json.RawMessage(`{"path":"main.go"}`),
	})
	if !ok || pending.ThreadID != "thread-1" || pending.TurnID != "turn-1" || pending.Call.Name != "edit_file" || pending.ToolCallItemID != "item_tool" {
		t.Fatalf("pending identity mismatch ok=%v pending=%#v", ok, pending)
	}
	if pending.WorkspaceCheckpointID != "checkpoint-event" || !pending.DisableUserInput || pending.MaxModelSteps == nil || *pending.MaxModelSteps != 3 {
		t.Fatalf("pending event fields mismatch: %#v", pending)
	}
	if pending.EffectiveMaxModelSteps == nil || *pending.EffectiveMaxModelSteps != 5 || pending.SubagentDepth != 2 {
		t.Fatalf("pending numeric fields mismatch: %#v", pending)
	}
	if pending.GUIPlan["source"] != "turn" || len(pending.Messages) != 1 || len(pending.ToolScope) != 1 {
		t.Fatalf("pending cloned fields mismatch: %#v", pending)
	}
	if pending.ProviderConfig.APIKey != "" || pending.ProviderConfig.ProxyURL != "" {
		t.Fatal("pending tool call retained physical Provider authority")
	}
	messages[0].Content = "mutated"
	if pending.Messages[0].Content != "hello" {
		t.Fatalf("messages should be cloned: %#v", pending.Messages)
	}
}

func TestBuildPendingToolCallRejectsAuditOnlyV1Authority(t *testing.T) {
	thread := map[string]any{"turns": []any{map[string]any{
		"id": "turn-v1", "items": []any{map[string]any{
			"id": "item-v1", "kind": "tool_call", "callId": "call-v1", "toolName": "read_file", "arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1(),
		}},
	}}}
	turn, _ := TurnByID(thread, "turn-v1")
	event := map[string]any{"threadId": "thread-v1", "turnId": "turn-v1", "callId": "call-v1"}
	legacy := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-v1", TurnID: "turn-v1", WorkspaceRealPath: "/workspace", ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: legacy, Provider: "provider-v1", ServerIdentity: "host:builtin", ToolName: "read_file", ToolCallID: "call-v1",
		ArgsHash: domainsecurity.CanonicalJSONHash([]byte(`{"path":"a.txt"}`)), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: time.Now().UTC(),
	})
	event["turnSecurityContext"] = mapRecordForPendingToolTest(legacy)
	event["executionGrant"] = mapRecordForPendingToolTest(grant)
	event["toolCallItemId"] = "item-v1"
	event["toolName"] = "read_file"
	if _, ok := BuildPendingToolCall(PendingToolCallInput{
		Event: event, Thread: thread, Turn: turn, ProviderID: "provider-v1", Workspace: "/workspace", ToolScope: []string{"read_file"},
		PrivateArguments: json.RawMessage(`{"path":"a.txt"}`),
	}); ok {
		t.Fatal("audit-only V1 context rebuilt an executable pending tool call")
	}
}

func pendingSecurityEvent(event map[string]any, workspace string, providerID string, toolName string, itemID string, arguments string) map[string]any {
	now := time.Now().UTC()
	context, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: event["threadId"].(string), TurnID: event["turnId"].(string), WorkspaceRealPath: workspace,
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("test-manifest")), ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		panic(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: context, Provider: providerID, ServerIdentity: "host:builtin", ToolName: toolName,
		ToolCallID: event["callId"].(string), ArgsHash: domainsecurity.CanonicalJSONHash([]byte(arguments)),
		SchemaHash: domainsecurity.SHA256Hex([]byte("test-schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("test-scope")),
		ApprovalState: "pending", IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	contextBody, _ := json.Marshal(context)
	grantBody, _ := json.Marshal(grant)
	var contextRecord map[string]any
	var grantRecord map[string]any
	_ = json.Unmarshal(contextBody, &contextRecord)
	_ = json.Unmarshal(grantBody, &grantRecord)
	event["turnSecurityContext"] = contextRecord
	event["executionGrant"] = grantRecord
	event["toolCallItemId"] = itemID
	return event
}

func mapRecordForPendingToolTest(value any) map[string]any {
	body, _ := json.Marshal(value)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}
