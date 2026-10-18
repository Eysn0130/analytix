package model

import (
	"crypto/ed25519"
	"encoding/json"
	"strings"
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
	receipt, thread, resolver, prompt := pendingContinuationApprovalFixtureV1(t)
	payload := receipt.Payload
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

func pendingContinuationApprovalFixtureV1(t *testing.T, definitions ...*domainmodel.ToolDefinitionSnapshotV1) (domaincontinuation.Receipt, map[string]any, strictContinuationProviderResolverStub, string) {
	t.Helper()
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
	if len(definitions) != 0 {
		payload.ModelVisibleToolDefinition = definitions[0]
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
	return receipt, thread, resolver, prompt
}

func TestRebuildPausedPendingFromPersistedReceiptRetainsOriginalDefinitionV1(t *testing.T) {
	for _, definition := range []*domainmodel.ToolDefinitionSnapshotV1{
		nil,
		{SchemaJSON: `{"name":"write_file","description":"ACTUAL_PAUSED_REQUEST","parameters":{"type":"object"}}`},
		{Truncated: true},
	} {
		receipt, thread, resolver, _ := pendingContinuationApprovalFixtureV1(t, definition)
		body, err := domaincontinuation.ReceiptBytes(receipt)
		if err != nil {
			t.Fatal(err)
		}
		restartedReceipt, err := domaincontinuation.ParseReceipt(body)
		if err != nil {
			t.Fatal(err)
		}
		// Restart loses the same-process pending and all model request/catalog
		// objects. Rebuild from the signed bytes and persisted waiting thread.
		restartedThread := continuationCloneRecord(thread)
		pending, err := RebuildPendingToolCallFromReceipt(restartedReceipt, restartedThread, resolver)
		if err != nil {
			t.Fatal(err)
		}
		if definition == nil {
			if pending.ModelVisibleToolDefinition != nil {
				t.Fatal("old missing definition was backfilled during restart")
			}
			continue
		}
		if pending.ModelVisibleToolDefinition == nil || *pending.ModelVisibleToolDefinition != *definition {
			t.Fatal("restart lost the original request definition")
		}
		restartedReceipt.Payload.ModelVisibleToolDefinition.SchemaJSON = "later mutation"
		if *pending.ModelVisibleToolDefinition != *definition {
			t.Fatal("rebuilt pending retained a mutable receipt pointer")
		}
	}
}

func TestRebuildPendingToolCallRetainsPriorSignedPendingHistoryWithoutReexecutingIt(t *testing.T) {
	receipt, thread, resolver, _ := pendingContinuationApprovalFixtureV1(t)
	timeout := 120
	historical, audit := pendingHistoryFixtureV1(t, receipt.Payload.ThreadID, &timeout)
	thread["turns"] = append(historical["turns"].([]any), thread["turns"].([]any)...)
	pending, err := RebuildPendingToolCallFromReceiptWithSteeringAuthority(receipt, thread, resolver, nil, audit)
	if err != nil || pending.Call.ID != receipt.Payload.CallID || pending.Call.ID == audit.receipt.Payload.CallID {
		t.Fatal("history became another execution or changed the current call", err)
	}
	var prior []domainmodel.Message
	for _, message := range pending.Messages {
		if len(message.ToolCalls) == 1 && message.ToolCalls[0].ID == audit.receipt.Payload.CallID || message.ToolCallID == audit.receipt.Payload.CallID {
			prior = append(prior, message)
		}
	}
	assertOriginalPendingHistoryV1(t, prior, audit.receipt.Payload.CallID)
	if rebuilt, err := RebuildPendingToolCallFromReceiptWithSteeringAuthority(audit.receipt, historical, resolver, nil, audit); err == nil || rebuilt.ThreadID != "" {
		t.Fatal("continued audit receipt reopened execution")
	}
}

func TestRebuildPendingRetainsSignedOutputCapAcrossRestart(t *testing.T) {
	receipt, thread, resolver, _ := pendingContinuationApprovalFixtureV1(t)
	payload := receipt.Payload
	payload.OutputCapLogicalSequence = 1
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err = domaincontinuation.NewReceipt(payload, domainsecurity.SHA256Hex(publicKey), publicKey, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	thread["turns"].([]any)[0].(map[string]any)["items"].([]any)[2].(map[string]any)["continuationReceiptId"] = receipt.ReceiptID
	body, _ := domaincontinuation.ReceiptBytes(receipt)
	restartedReceipt, err := domaincontinuation.ParseReceipt(body)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := RebuildPendingToolCallFromReceipt(restartedReceipt, continuationCloneRecord(thread), resolver)
	if err != nil || pending.OutputCapLogicalSequence != 1 || pending.ProviderNamespace.ProviderCallSequence != 1 {
		t.Fatalf("restart lost cap: %#v %v", pending, err)
	}
}

func TestSubmittedQuestionEffectRestoresOnlyBoundIDsAfterCompleteContentProjection(t *testing.T) {
	receipt, _, _, _ := pendingContinuationApprovalFixtureV1(t)
	current := receipt.Payload.SecurityContext
	qid := "input_" + strings.Repeat("a", 26) + "123456789012" + strings.Repeat("a", 26) + "_1"
	content := ToolResultContent(map[string]any{"status": "submitted", "answerCount": float64(1),
		"answers": []map[string]string{{"id": qid, "label": "account 6222020000000000000", "value": qid}}})
	message := domainmodel.Message{Role: "tool", Name: "request_user_input", ToolCallID: receipt.Payload.CallID, Content: content}
	// Same-package fixture isolates projection behavior; the external-package
	// privacy regression separately proves the signed audit minting boundary.
	message.PrivateUserInputPendingBinding = pendingUserInputProviderBindingV1{
		threadID: current.ThreadID, contextDigest: current.ContextDigest, sourceTurnID: current.TurnID,
		receiptID: receipt.ReceiptID, callID: message.ToolCallID, toolName: message.Name,
		role: message.Role, name: message.Name, outerCallID: message.ToolCallID, content: message.Content,
		submittedQuestionIDs: ToolResultContent([]string{qid}),
	}
	if _, err := PendingUserInputProviderMessageV1(message, current); err == nil {
		t.Fatal("pending-only helper returned unprojected submitted answers")
	}
	for _, closed := range []bool{false, true} {
		source := CloneProviderMessages(CloneProviderMessages([]domainmodel.Message{message}))[0]
		if closed {
			messages := DeepSeekReasoningSafeHistory([]domainmodel.Message{{Role: "assistant", ToolCalls: []domainmodel.ToolCall{{ID: source.ToolCallID, Name: source.Name}}}, source})
			if len(messages) != 1 {
				t.Fatal("submitted continuation was lost during protocol closure")
			}
			source = messages[0]
		}
		scanned := false
		projected, err := UserInputProviderMessageForEffectV1(source, current, func(raw string) (string, error) {
			scanned = raw == content
			return strings.ReplaceAll(strings.ReplaceAll(raw, qid, "[ACCOUNT]"), "6222020000000000000", "[ACCOUNT]"), nil
		})
		if err != nil || !scanned || projected.PrivateUserInputPendingBinding != nil {
			t.Fatal("complete submitted content was not scanned before identity restoration", err)
		}
		inner := projected.Content
		if closed {
			var valid bool
			inner, valid = CompletedPrivateProtocolSafeHistoryContentV1(inner, "request_user_input")
			if !valid {
				t.Fatal("submitted protocol closure is not canonical")
			}
		}
		_, answers, valid := submittedUserInputProviderContentV1(inner)
		if !valid || answers[0]["id"] != qid || answers[0]["value"] != "[ACCOUNT]" || answers[0]["label"] != "account [ACCOUNT]" {
			t.Fatal("identity restoration also restored an untrusted answer string")
		}
	}
	if message.Content != content || message.PrivateUserInputPendingBinding == nil {
		t.Fatal("clone or projection changed the immutable source carrier")
	}
	if _, err := UserInputProviderMessageForEffectV1(message, current, nil); err == nil {
		t.Fatal("submitted answer escaped without its final effect projection")
	}
}
