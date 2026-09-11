package model

import (
	"crypto/ed25519"
	"encoding/json"
	"testing"
	"time"

	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type strictContinuationProviderResolverStub struct {
	config domainmodel.TurnConfig
}

func (stub strictContinuationProviderResolverStub) TurnConfig(_, _ string) domainmodel.TurnConfig {
	return stub.config
}

func (stub strictContinuationProviderResolverStub) HasProvider(providerID string) bool {
	return providerID == stub.config.ProviderID
}

func (stub strictContinuationProviderResolverStub) ValidateExecutionModel(providerID, model string) error {
	if providerID != stub.config.ProviderID || model != stub.config.Model {
		return errPendingContinuationTestProvider
	}
	return nil
}

var errPendingContinuationTestProvider = &pendingContinuationTestError{}

type pendingContinuationTestError struct{}

func (*pendingContinuationTestError) Error() string { return "test provider mismatch" }

func TestRebuildPendingToolCallUsesReceiptProviderStepBindingAndPromptDigest(t *testing.T) {
	now := time.Date(2026, 7, 28, 6, 7, 8, 0, time.UTC)
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-rebuild-receipt", TurnID: "turn-rebuild-receipt", WorkspaceRealPath: "/workspace",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	callID := modelTestHostToolCallID("pending-continuation-receipt")
	toolName := "write_file"
	arguments := json.RawMessage(`{"path":"main.go"}`)
	toolScope := []string{toolName}
	scopeBody, err := json.Marshal(toolScope)
	if err != nil {
		t.Fatal(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-a", ServerIdentity: "host:builtin", ToolName: toolName, ToolCallID: callID,
		ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex(scopeBody), ApprovalState: "pending", IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	gateID := domaincontinuation.GateID(
		domaincontinuation.KindApproval, securityContext.ThreadID, securityContext.TurnID,
		securityContext.ContextDigest, grant.GrantID, callID,
	)
	toolItemID := domaintoolcall.ToolCallItemIDV1(securityContext.TurnID, callID)
	prompt := "continue the exact ordinary edit"
	providerConfig := domainmodel.TurnConfig{
		ProviderID: "provider-a", Model: "model-a", APIKey: "test-only", BaseURL: "https://provider.invalid",
		EndpointFormat: "chat_completions",
	}
	payload := domaincontinuation.Payload{
		Version: domaincontinuation.ContractVersionV3, Kind: domaincontinuation.KindApproval, GateID: gateID,
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, ItemID: "item_" + gateID,
		CallID: callID, ToolName: toolName, ToolCallItemID: toolItemID, Arguments: arguments,
		ProviderID: providerConfig.ProviderID, Model: providerConfig.Model, ProviderRouteHash: ProviderContinuationRouteHash(providerConfig),
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", ToolScope: toolScope,
		LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true, ProviderStepExact: true,
		CaseSourceUnavailable: true, OrdinaryResultInputIsolated: true, PrivateProtocolObserved: true,
		ProviderStepPromptSHA256: domaincontinuation.CanonicalProviderStepPromptSHA256(prompt),
		ProviderNamespace:        domaincontinuation.NewProviderContinuationNamespaceV1("turn", "", 1, nil),
		SecurityContext:          securityContext, ExecutionGrant: grant, IssuedAt: now.Add(time.Second).Format(time.RFC3339Nano),
	}
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domaincontinuation.NewReceipt(
		payload, domainsecurity.SHA256Hex(publicKey), publicKey,
		func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	securityRecord := continuationRecord(securityContext)
	thread := map[string]any{
		"id": securityContext.ThreadID, "workspace": securityContext.WorkspaceRealPath, "securityState": securityRecord,
		"turns": []any{map[string]any{
			"id": securityContext.TurnID, "status": "waiting", "securityContext": securityRecord,
			"items": []any{
				map[string]any{"id": "item-user", "kind": "user_message", "role": "user", "text": prompt},
				map[string]any{
					"id": toolItemID, "kind": "tool_call", "callId": callID, "toolName": toolName,
					"arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1(),
				},
				map[string]any{
					"id": payload.ItemID, "kind": "approval", "approvalId": gateID, "status": "pending",
					"toolName": toolName, "continuationReceiptId": receipt.ReceiptID,
				},
			},
		}},
	}
	resolver := strictContinuationProviderResolverStub{config: providerConfig}
	pending, err := RebuildPendingToolCallFromReceipt(receipt, thread, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Prompt != prompt || pending.LogicalEffect != payload.LogicalEffect ||
		pending.OrdinaryWork != payload.OrdinaryWork || !pending.ProviderStepExact ||
		!pending.CaseSourceUnavailable || !pending.OrdinaryResultInputIsolated ||
		!pending.PrivateProtocolObserved {
		t.Fatalf("receipt provider-step binding was not restored exactly: %#v", pending)
	}

	tamperedThread := continuationCloneRecord(thread)
	tamperedTurn := tamperedThread["turns"].([]any)[0].(map[string]any)
	tamperedTurn["items"].([]any)[0].(map[string]any)["text"] = "continue a different edit"
	if rebuilt, err := RebuildPendingToolCallFromReceipt(receipt, tamperedThread, resolver); err == nil || rebuilt.ThreadID != "" {
		t.Fatalf("changed provider-step prompt rebuilt receipt authority: pending=%#v err=%v", rebuilt, err)
	}
}

func continuationCloneRecord(value map[string]any) map[string]any {
	body, _ := json.Marshal(value)
	cloned := map[string]any{}
	_ = json.Unmarshal(body, &cloned)
	return cloned
}
