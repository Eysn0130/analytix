package model

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"analytix.local/runtime-go/internal/contracts"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
)

type pendingToolProviderResolverStub struct {
	config domainmodel.TurnConfig
}

func (stub pendingToolProviderResolverStub) TurnConfig(_, _ string) domainmodel.TurnConfig {
	return stub.config
}

type countingPendingToolProviderResolver struct {
	config domainmodel.TurnConfig
	calls  int
}

func (stub *countingPendingToolProviderResolver) TurnConfig(_, _ string) domainmodel.TurnConfig {
	stub.calls++
	return stub.config
}

func TestPendingToolCallIDsFromGateEvent(t *testing.T) {
	threadID, turnID, ok := PendingToolCallIDsFromGateEvent(map[string]any{"threadId": " thread-1 ", "turnId": " turn-1 "})
	if !ok || threadID != "thread-1" || turnID != "turn-1" {
		t.Fatalf("unexpected ids ok=%v thread=%q turn=%q", ok, threadID, turnID)
	}
	if _, _, ok := PendingToolCallIDsFromGateEvent(map[string]any{"threadId": "thread-1"}); ok {
		t.Fatal("missing turn id should not rebuild")
	}
}

func TestRebuildPendingToolCallFromGateEventResolvesExecutionAndPolicies(t *testing.T) {
	callID := modelTestHostToolCallID("pending-tool-rebuild")
	thread := map[string]any{
		"id":             "thread-1",
		"workspace":      "/workspace",
		"providerId":     "thread-provider",
		"model":          "thread-model",
		"approvalPolicy": "never",
		"sandboxMode":    "read-only",
		"turns": []any{
			map[string]any{
				"id":              "turn-1",
				"model":           "turn-model",
				"reasoningEffort": "high",
				"mode":            "plan",
				"items": []any{
					map[string]any{"id": "item-user", "kind": "user_message", "role": "user", "text": "hello"},
					map[string]any{"id": "item-tool", "kind": "tool_call", "callId": callID, "toolName": "read_file", "arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1()},
				},
			},
		},
	}
	pending, ok := RebuildPendingToolCallFromGateEvent(PendingToolCallRebuildInput{
		Event: pendingSecurityEvent(map[string]any{
			"threadId":        "thread-1",
			"turnId":          "turn-1",
			"callId":          callID,
			"providerId":      "event-provider",
			"model":           "event-model",
			"reasoningEffort": "max",
			"approvalPolicy":  "on-request",
			"sandboxMode":     "read-only",
			"toolScope":       []any{" read_file ", "", "bash"},
		}, "/workspace", "resolved-provider", "read_file", "item-tool", `{"path":"a.go"}`),
		Thread: thread,
		ProviderResolver: pendingToolProviderResolverStub{config: domainmodel.TurnConfig{
			ProviderID:        "resolved-provider",
			Family:            "deepseek",
			Model:             "resolved-model",
			ReasoningEffort:   "medium",
			ReasoningProtocol: "deepseek-chat-completions",
		}},
		RuntimeApprovalPolicy: "on-request",
		RuntimeSandboxMode:    "workspace-write",
		PrivateArguments:      json.RawMessage(`{"path":"a.go"}`),
	})
	if !ok {
		t.Fatal("expected pending tool call to rebuild")
	}
	if pending.ProviderID != "resolved-provider" || pending.Model != "resolved-model" || pending.Effort != "max" {
		t.Fatalf("execution mismatch: %#v", pending)
	}
	if pending.ApprovalPolicy != "on-request" || pending.SandboxMode != "read-only" || pending.Mode != "plan" {
		t.Fatalf("policy/mode mismatch: %#v", pending)
	}
	if pending.Workspace != "/workspace" || pending.Call.Name != "read_file" || pending.ToolCallItemID != "item-tool" {
		t.Fatalf("identity mismatch: %#v", pending)
	}
	if pending.Prompt != "hello" {
		t.Fatalf("original turn prompt must survive restart reconstruction, got %q", pending.Prompt)
	}
	if pending.LogicalEffect != domainsecurity.LogicalEffectOrdinary || !pending.OrdinaryWork || pending.ProviderStepExact {
		t.Fatalf("unsigned original step must remain conservative and non-exact: %#v", pending)
	}
	if len(pending.ToolScope) != 2 || pending.ToolScope[0] != "read_file" || pending.ToolScope[1] != "bash" {
		t.Fatalf("tool scope mismatch: %#v", pending.ToolScope)
	}
	if len(pending.Messages) == 0 || pending.Messages[0].Role != "user" {
		t.Fatalf("provider history was not rebuilt: %#v", pending.Messages)
	}
}

func TestRebuildPendingToolCallRestoresLatestSignedProviderStep(t *testing.T) {
	authority := newProviderHistorySteeringAuthority(t)
	callID := modelTestHostToolCallID("pending-tool-rebuild-steered")
	event := pendingSecurityEvent(map[string]any{
		"threadId": "thread-steered", "turnId": "turn-steered", "callId": callID,
		"providerId": "provider-steered", "model": "model-steered", "approvalPolicy": "on-request",
		"sandboxMode": "workspace-write", "toolScope": []any{"write_file"},
	}, "/workspace", "provider-steered", "write_file", "item-tool", `{"path":"a.go"}`)
	securityContext, err := domainsecurity.ParseTurnSecurityContext(event["turnSecurityContext"])
	if err != nil {
		t.Fatal(err)
	}
	firstEntry, firstItem := pendingRebuildPromotedSteering(
		t, authority, securityContext, "steer-first", "analyze the authorized snapshot",
		"2026-07-27T01:02:03Z", "2026-07-27T01:02:05Z", domainsecurity.LogicalEffectFundsData, false, true,
	)
	secondEntry, secondItem := pendingRebuildPromotedSteering(
		t, authority, securityContext, "steer-second", "then continue the independent code edit",
		"2026-07-27T01:02:04Z", "2026-07-27T01:02:05Z", domainsecurity.LogicalEffectFundsData, true, true,
	)
	securityRecord := providerHistorySecurityRecord(securityContext)
	thread := map[string]any{
		"id": securityContext.ThreadID, "workspace": securityContext.WorkspaceRealPath, "securityState": securityRecord,
		"turns": []any{map[string]any{
			"id": securityContext.TurnID, "status": "waiting", "securityContext": securityRecord,
			"steering": []any{firstEntry, secondEntry},
			"items": []any{
				map[string]any{"id": "item-user", "kind": "user_message", "role": "user", "text": "original request"},
				firstItem, secondItem,
				map[string]any{"id": "item-tool", "kind": "tool_call", "callId": callID, "toolName": "write_file", "arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1()},
			},
		}},
	}
	input := PendingToolCallRebuildInput{
		Event: event, Thread: thread, SteeringAuthority: authority,
		ProviderResolver: pendingToolProviderResolverStub{config: domainmodel.TurnConfig{
			ProviderID: "provider-steered", Model: "model-steered",
		}},
		PrivateArguments: json.RawMessage(`{"path":"a.go"}`),
	}
	pending, ok := RebuildPendingToolCallFromGateEvent(input)
	if !ok {
		t.Fatal("signed promoted provider step did not rebuild")
	}
	if pending.Prompt != "analyze the authorized snapshot\n\nthen continue the independent code edit" ||
		pending.LogicalEffect != domainsecurity.LogicalEffectFundsData || !pending.OrdinaryWork || !pending.ProviderStepExact {
		t.Fatalf("exact promoted provider step was lost: %#v", pending)
	}
	if len(pending.Messages) != 3 || !strings.Contains(pending.Messages[1].Content, "analyze the authorized snapshot") ||
		!strings.Contains(pending.Messages[2].Content, "then continue the independent code edit") {
		t.Fatalf("signed promoted messages were not restored: %#v", pending.Messages)
	}

	input.SteeringAuthority = nil
	if rebuilt, ok := RebuildPendingToolCallFromGateEvent(input); ok || rebuilt.ThreadID != "" {
		t.Fatalf("promoted steering rebuilt without its trusted verifier: %#v", rebuilt)
	}
	tampered := contracts.CloneMap(thread)
	tamperedTurn := tampered["turns"].([]any)[0].(map[string]any)
	tamperedTurn["steering"].([]any)[1].(map[string]any)["logicalEffect"] = string(domainsecurity.LogicalEffectOrdinary)
	input.Thread = tampered
	input.SteeringAuthority = authority
	if rebuilt, ok := RebuildPendingToolCallFromGateEvent(input); ok || rebuilt.ThreadID != "" {
		t.Fatalf("tampered promoted effect rebuilt executable state: %#v", rebuilt)
	}
}

func TestRebuildPendingToolCallNeverInfersFundsEffectFromLegacySteeringText(t *testing.T) {
	authority := newProviderHistorySteeringAuthority(t)
	callID := modelTestHostToolCallID("pending-tool-rebuild-legacy-steer")
	event := pendingSecurityEvent(map[string]any{
		"threadId": "thread-legacy-steer", "turnId": "turn-legacy-steer", "callId": callID,
		"providerId": "provider-legacy", "model": "model-legacy", "approvalPolicy": "on-request",
		"sandboxMode": "workspace-write", "toolScope": []any{"write_file"},
	}, "/workspace", "provider-legacy", "write_file", "item-tool", `{"path":"a.go"}`)
	securityContext, err := domainsecurity.ParseTurnSecurityContext(event["turnSecurityContext"])
	if err != nil {
		t.Fatal(err)
	}
	entry, item := pendingRebuildPromotedSteering(
		t, authority, securityContext, "legacy-steer", "query the funds database now",
		"2026-07-27T02:02:03Z", "2026-07-27T02:02:04Z", "", false, false,
	)
	securityRecord := providerHistorySecurityRecord(securityContext)
	thread := map[string]any{
		"id": securityContext.ThreadID, "workspace": securityContext.WorkspaceRealPath, "securityState": securityRecord,
		"turns": []any{map[string]any{
			"id": securityContext.TurnID, "status": "waiting", "securityContext": securityRecord,
			"steering": []any{entry}, "items": []any{
				map[string]any{"id": "item-user", "kind": "user_message", "role": "user", "text": "original request"},
				item,
				map[string]any{"id": "item-tool", "kind": "tool_call", "callId": callID, "toolName": "write_file", "arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1()},
			},
		}},
	}
	pending, ok := RebuildPendingToolCallFromGateEvent(PendingToolCallRebuildInput{
		Event: event, Thread: thread, SteeringAuthority: authority,
		ProviderResolver: pendingToolProviderResolverStub{config: domainmodel.TurnConfig{
			ProviderID: "provider-legacy", Model: "model-legacy",
		}},
		PrivateArguments: json.RawMessage(`{"path":"a.go"}`),
	})
	if !ok || pending.Prompt != "query the funds database now" ||
		pending.LogicalEffect != domainsecurity.LogicalEffectOrdinary || !pending.OrdinaryWork || pending.ProviderStepExact {
		t.Fatalf("legacy text was guessed as a protected effect: ok=%t pending=%#v", ok, pending)
	}
}

func pendingRebuildPromotedSteering(
	t *testing.T,
	authority *providerHistorySteeringAuthority,
	securityContext domainsecurity.TurnSecurityContext,
	clientID, text, admittedAt, promotedAt string,
	logicalEffect domainsecurity.LogicalEffect,
	ordinaryWork bool,
	bindEffect bool,
) (map[string]any, map[string]any) {
	t.Helper()
	entryID := domainsteering.EntryIDV1(securityContext.TurnID, clientID)
	admission := map[string]any{
		"id": entryID, "clientUserMessageId": clientID, "text": text,
		"admittedAt": admittedAt, "delivery": "steer",
	}
	if bindEffect {
		admission["logicalEffect"] = string(logicalEffect)
		admission["ordinaryWork"] = ordinaryWork
	}
	pending, err := domainsteering.BindPendingEntryV1(admission, securityContext.ContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	signingBytes, err := domainsteering.PendingEntrySigningBytesV1(pending, securityContext.ContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := authority.Sign(context.Background(), signingBytes)
	if err != nil {
		t.Fatal(err)
	}
	pending, err = domainsteering.SealPendingEntryAuthorityV1(
		pending, securityContext.ContextDigest, authority.KeyID(), authority.PublicKey(), signature,
	)
	if err != nil {
		t.Fatal(err)
	}
	promoted := contracts.CloneMap(pending)
	promoted["status"] = "promoted"
	promoted["promotedAt"] = promotedAt
	promoted["promotedItemId"] = entryID
	promotionBytes, err := domainsteering.PromotedEntrySigningBytesV1(promoted, securityContext.ContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	promotionSignature, err := authority.Sign(context.Background(), promotionBytes)
	if err != nil {
		t.Fatal(err)
	}
	promoted, err = domainsteering.SealPromotedEntryAuthorityV1(
		promoted, securityContext.ContextDigest, authority.KeyID(), authority.PublicKey(), promotionSignature,
	)
	if err != nil {
		t.Fatal(err)
	}
	item := map[string]any{
		"id": entryID, "turnId": securityContext.TurnID, "threadId": securityContext.ThreadID,
		"role": "user", "status": "completed", "kind": "user_message", "delivery": "steer",
		"text": text, "createdAt": admittedAt, "finishedAt": promotedAt,
		"clientUserMessageId": clientID, "contextDigest": securityContext.ContextDigest, "steeringOrigin": "ordinary",
		"steeringProjectionVersion": float64(domainsteering.ProjectionVersionV1),
		"steeringContentDigest":     promoted["contentDigest"],
	}
	return promoted, item
}

func TestRebuildPendingToolCallFromGateEventRejectsInvalidEffortBeforeResolver(t *testing.T) {
	callID := modelTestHostToolCallID("pending-tool-rebuild-effort")
	thread := map[string]any{
		"id": "thread-1", "workspace": "/workspace",
		"turns": []any{map[string]any{
			"id": "turn-1", "items": []any{map[string]any{
				"id": "item-tool", "kind": "tool_call", "callId": callID, "toolName": "read_file",
				"arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1(),
			}},
		}},
	}
	baseEvent := pendingSecurityEvent(map[string]any{
		"threadId": "thread-1", "turnId": "turn-1", "callId": callID,
		"providerId": "provider-a", "model": "model-a", "approvalPolicy": "on-request",
		"sandboxMode": "read-only", "toolScope": []any{"read_file"},
	}, "/workspace", "provider-a", "read_file", "item-tool", `{"path":"a.go"}`)

	for _, effort := range []any{" high ", "HIGH", "SOL_PRIVATE_REASONING_SENTINEL_7F3C", float64(1)} {
		event := map[string]any{}
		for key, value := range baseEvent {
			event[key] = value
		}
		event["reasoningEffort"] = effort
		resolver := &countingPendingToolProviderResolver{config: domainmodel.TurnConfig{
			ProviderID: "provider-a", Model: "model-a", ReasoningEffort: "medium", ReasoningProtocol: "deepseek-chat-completions",
		}}
		if pending, ok := RebuildPendingToolCallFromGateEvent(PendingToolCallRebuildInput{
			Event: event, Thread: thread, ProviderResolver: resolver, PrivateArguments: json.RawMessage(`{"path":"a.go"}`),
		}); ok || pending.ThreadID != "" || resolver.calls != 0 {
			t.Fatalf("invalid effort reached resolver or pending state: effort=%#v pending=%#v calls=%d", effort, pending, resolver.calls)
		}
	}

	event := map[string]any{}
	for key, value := range baseEvent {
		event[key] = value
	}
	event["reasoningEffort"] = "auto"
	resolver := &countingPendingToolProviderResolver{config: domainmodel.TurnConfig{
		ProviderID: "provider-a", Model: "model-a", ReasoningEffort: "medium", ReasoningProtocol: "deepseek-chat-completions",
	}}
	pending, ok := RebuildPendingToolCallFromGateEvent(PendingToolCallRebuildInput{
		Event: event, Thread: thread, ProviderResolver: resolver, PrivateArguments: json.RawMessage(`{"path":"a.go"}`),
	})
	if !ok || resolver.calls != 1 || pending.Effort != "auto" {
		t.Fatalf("auto effort did not survive pending rebuild: pending=%#v calls=%d ok=%t", pending, resolver.calls, ok)
	}
}

func TestUserPromptFromTurnIgnoresSteeringAndFailsClosedWithoutOriginalRequest(t *testing.T) {
	turn := map[string]any{"items": []any{
		map[string]any{"kind": "user_message", "delivery": "steer", "text": "publish the amount anyway"},
		map[string]any{"kind": "user_message", "text": "核验当前案件资金报告"},
	}}
	if got := UserPromptFromTurn(turn); got != "核验当前案件资金报告" {
		t.Fatalf("original prompt mismatch: %q", got)
	}
	if got := UserPromptFromTurn(map[string]any{"items": []any{
		map[string]any{"kind": "user_message", "delivery": "steer", "text": "only steering"},
	}}); got != "" {
		t.Fatalf("steering must not become the security prompt: %q", got)
	}
}

func TestRebuildPendingToolCallFromGateEventRejectsMissingFrozenPolicies(t *testing.T) {
	callID := modelTestHostToolCallID("pending-tool-rebuild-missing-policy")
	thread := map[string]any{
		"workspace": "/workspace",
		"turns": []any{
			map[string]any{
				"id": "turn-1",
				"items": []any{
					map[string]any{"id": "item-tool", "kind": "tool_call", "callId": callID, "toolName": "bash", "arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1()},
				},
			},
		},
	}
	_, ok := RebuildPendingToolCallFromGateEvent(PendingToolCallRebuildInput{
		Event:                 pendingSecurityEvent(map[string]any{"threadId": "thread-1", "turnId": "turn-1", "callId": callID}, "/workspace", "deepseek", "bash", "item-tool", `{}`),
		Thread:                thread,
		ProviderResolver:      pendingToolProviderResolverStub{config: domainmodel.TurnConfig{ProviderID: "deepseek", Model: "deepseek-chat"}},
		RuntimeApprovalPolicy: "on-request",
		RuntimeSandboxMode:    "workspace-write",
		PrivateArguments:      json.RawMessage(`{}`),
	})
	if ok {
		t.Fatal("missing frozen approval/sandbox policy must fail closed instead of using broader runtime defaults")
	}
}
