package turn

import (
	"crypto/ed25519"
	"encoding/json"
	"testing"
	"time"

	appmodel "analytix.local/runtime-go/internal/app/model"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
)

func TestGateContinuationFieldsCarriesExecutionAndGateState(t *testing.T) {
	maxSteps := 5
	effective := 4
	guiPlan := map[string]any{"phase": "review"}
	fields := GateContinuationFields(GateContinuationInput{
		CallID:                      " call_1 ",
		ToolName:                    " bash ",
		ToolCallItemID:              " item_1 ",
		ProviderID:                  " provider_a ",
		Model:                       " model_a ",
		WorkspaceCheckpointID:       " checkpoint_1 ",
		ApprovalPolicy:              " on-request ",
		SandboxMode:                 " workspace-write ",
		SubagentDepth:               2,
		ReasoningEffort:             "auto",
		Mode:                        " primary ",
		GUIPlan:                     guiPlan,
		DisableUserInput:            true,
		MaxModelSteps:               &maxSteps,
		EffectiveMaxModelSteps:      &effective,
		ToolScope:                   []string{" read_file ", "", "bash"},
		LogicalEffect:               domainsecurity.LogicalEffectOrdinary,
		OrdinaryWork:                true,
		ProviderStepExact:           true,
		CaseSourceUnavailable:       true,
		OrdinaryResultInputIsolated: true,
		ProviderStepPromptSHA256:    domainsecurity.SHA256Hex([]byte("exact provider step")),
	})
	guiPlan["phase"] = "mutated"
	if fields["callId"] != "call_1" ||
		fields["toolName"] != "bash" ||
		fields["providerId"] != "provider_a" ||
		fields["model"] != "model_a" ||
		fields["reasoningEffort"] != "auto" ||
		fields["subagentDepth"] != float64(2) ||
		fields["disableUserInput"] != true ||
		fields["maxModelSteps"] != float64(5) ||
		fields["effectiveMaxModelSteps"] != float64(4) {
		t.Fatalf("gate continuation fields mismatch: %#v", fields)
	}
	if fields["logicalEffect"] != "ordinary" || fields["ordinaryWork"] != true ||
		fields["providerStepExact"] != true || fields["caseSourceUnavailable"] != true ||
		fields["ordinaryResultInputIsolated"] != true ||
		fields["providerStepPromptSha256"] != domainsecurity.SHA256Hex([]byte("exact provider step")) {
		t.Fatalf("provider-step continuation fields mismatch: %#v", fields)
	}
	if _, persistedPrompt := fields["prompt"]; persistedPrompt {
		t.Fatalf("provider-step prompt text entered continuation fields: %#v", fields)
	}
	copiedPlan, _ := fields["guiPlan"].(map[string]any)
	if copiedPlan["phase"] != "review" {
		t.Fatalf("gui plan should be copied, got %#v", copiedPlan)
	}
	scope, _ := fields["toolScope"].([]any)
	if len(scope) != 2 || scope[0] != "read_file" || scope[1] != "bash" {
		t.Fatalf("tool scope mismatch: %#v", scope)
	}
	invalid := GateContinuationFields(GateContinuationInput{ReasoningEffort: " high "})
	if _, exists := invalid["reasoningEffort"]; exists {
		t.Fatalf("invalid reasoning effort was normalized into gate metadata: %#v", invalid)
	}
}

func TestGateContinuationInputFromPendingTool(t *testing.T) {
	maxSteps := 8
	effective := 6
	input := GateContinuationInputFromPendingTool(appmodel.PendingToolCall{
		Prompt:                      "  exact provider step  ",
		ProviderID:                  "provider_a",
		Model:                       "model_a",
		Effort:                      "high",
		WorkspaceCheckpointID:       "checkpoint_1",
		ApprovalPolicy:              "on-request",
		SandboxMode:                 "workspace-write",
		Mode:                        "agent",
		GUIPlan:                     map[string]any{"phase": "impl"},
		DisableUserInput:            true,
		MaxModelSteps:               &maxSteps,
		EffectiveMaxModelSteps:      &effective,
		Call:                        domainmodel.ToolCall{ID: "call_1", Name: "bash"},
		ToolCallItemID:              "item_1",
		ToolScope:                   []string{"read_file", "bash"},
		SubagentDepth:               2,
		LogicalEffect:               domainsecurity.LogicalEffectOrdinary,
		OrdinaryWork:                true,
		ProviderStepExact:           true,
		CaseSourceUnavailable:       true,
		OrdinaryResultInputIsolated: true,
	})
	if input.CallID != "call_1" ||
		input.ToolName != "bash" ||
		input.ProviderID != "provider_a" ||
		input.Model != "model_a" ||
		input.ReasoningEffort != "high" ||
		input.WorkspaceCheckpointID != "checkpoint_1" ||
		input.ApprovalPolicy != "on-request" ||
		input.SandboxMode != "workspace-write" ||
		input.Mode != "agent" ||
		!input.DisableUserInput ||
		input.MaxModelSteps == nil ||
		*input.MaxModelSteps != 8 ||
		input.EffectiveMaxModelSteps == nil ||
		*input.EffectiveMaxModelSteps != 6 ||
		len(input.ToolScope) != 2 ||
		input.SubagentDepth != 2 ||
		input.LogicalEffect != domainsecurity.LogicalEffectOrdinary || !input.OrdinaryWork ||
		!input.ProviderStepExact || !input.CaseSourceUnavailable || !input.OrdinaryResultInputIsolated ||
		input.ProviderStepPromptSHA256 != domainsecurity.SHA256Hex([]byte("exact provider step")) {
		t.Fatalf("pending continuation input mismatch: %#v", input)
	}
}

func TestGateContinuationPayloadBindsExactProviderStepAndSameProcessPending(t *testing.T) {
	pending, now := gateContinuationPendingFixture(t)
	pending.PrivateProtocolObserved = true
	gateID := domaincontinuation.GateID(
		domaincontinuation.KindApproval, pending.ThreadID, pending.TurnID,
		pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID,
	)
	itemID := "item_" + gateID
	payload, err := GateContinuationPayload(domaincontinuation.KindApproval, gateID, itemID, pending, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	wantPromptHash := domainsecurity.SHA256Hex([]byte("continue the exact ordinary edit"))
	if payload.LogicalEffect != domainsecurity.LogicalEffectOrdinary || !payload.OrdinaryWork ||
		!payload.ProviderStepExact || !payload.CaseSourceUnavailable || !payload.OrdinaryResultInputIsolated ||
		!payload.PrivateProtocolObserved || payload.ProviderStepPromptSHA256 != wantPromptHash {
		t.Fatalf("provider-step receipt payload mismatch: %#v", payload)
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
	if err := ValidatePendingContinuationReceipt(receipt, pending); err != nil {
		t.Fatalf("exact same-process pending continuation was rejected: %v", err)
	}

	changed := pending
	changed.Prompt = "continue a different ordinary edit"
	if err := ValidatePendingContinuationReceipt(receipt, changed); err == nil {
		t.Fatal("changed provider-step prompt reused a continuation receipt")
	}
	changed = pending
	changed.LogicalEffect = domainsecurity.LogicalEffectFundsData
	if err := ValidatePendingContinuationReceipt(receipt, changed); err == nil {
		t.Fatal("changed logical effect reused a continuation receipt")
	}
	changed = pending
	changed.OrdinaryResultInputIsolated = false
	if err := ValidatePendingContinuationReceipt(receipt, changed); err == nil {
		t.Fatal("changed input-isolation provenance reused a continuation receipt")
	}
	changed = pending
	changed.PrivateProtocolObserved = false
	if err := ValidatePendingContinuationReceipt(receipt, changed); err == nil {
		t.Fatal("changed private-protocol observation reused a continuation receipt")
	}

	invalid := pending
	invalid.ProviderStepExact = false
	if _, err := GateContinuationPayload(domaincontinuation.KindApproval, gateID, itemID, invalid, now.Add(time.Second)); err == nil {
		t.Fatal("non-exact provider step minted a continuation payload")
	}
	invalid = pending
	invalid.Prompt = " \n\t "
	if _, err := GateContinuationPayload(domaincontinuation.KindApproval, gateID, itemID, invalid, now.Add(time.Second)); err == nil {
		t.Fatal("empty provider-step prompt minted a continuation payload")
	}
}

func gateContinuationPendingFixture(t *testing.T) (appmodel.PendingToolCall, time.Time) {
	t.Helper()
	now := time.Date(2026, 7, 28, 1, 2, 3, 0, time.UTC)
	securityContext := newTurnExecutionContextV2(t, "thread-gate-binding", "turn-gate-binding", "/workspace", 1, now)
	call := domainmodel.ToolCall{
		ID: turnTestHostToolCallID("gate-binding"), Name: "write_file",
		Arguments: json.RawMessage(`{"path":"main.go","content":"package main"}`),
	}
	toolScope := []string{call.Name}
	scopeBody, err := json.Marshal(toolScope)
	if err != nil {
		t.Fatal(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-a", ServerIdentity: "host:builtin",
		ToolName: call.Name, ToolCallID: call.ID, ArgsHash: domainsecurity.CanonicalJSONHash(call.Arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("schema")), ScopeHash: domainsecurity.SHA256Hex(scopeBody),
		ApprovalState: "pending", IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	return appmodel.PendingToolCall{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		ProviderConfig: domainmodel.TurnConfig{
			ProviderID: "provider-a", Model: "model-a", EndpointFormat: "chat_completions", BaseURL: "https://provider.invalid",
		},
		ProviderID: "provider-a", Model: "model-a", Prompt: "  continue the exact ordinary edit  ",
		LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true, ProviderStepExact: true,
		CaseSourceUnavailable: true, OrdinaryResultInputIsolated: true,
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write",
		ProviderNamespace: domaincontinuation.NewProviderContinuationNamespaceV1("turn", "", 1, nil),
		Call:              call, ToolCallItemID: domaintoolcall.ToolCallItemIDV1(securityContext.TurnID, call.ID),
		ToolScope: toolScope, SecurityContext: securityContext, ExecutionGrant: grant,
	}, now
}

func TestGateContinuationResponsesAreTypedAndTrimmed(t *testing.T) {
	unavailable := GateContinuationUnavailableResponse(" approval ", " approval_1 ")
	if unavailable["code"] != "gate_continuation_unavailable" ||
		unavailable["kind"] != "approval" ||
		unavailable["id"] != "approval_1" {
		t.Fatalf("unavailable response mismatch: %#v", unavailable)
	}

	terminal := GateContinuationTerminalResponse(" user_input ", " input_1 ", " cancelled ")
	if terminal["code"] != "gate_continuation_terminal_turn" ||
		terminal["kind"] != "user_input" ||
		terminal["id"] != "input_1" ||
		terminal["turnStatus"] != "cancelled" {
		t.Fatalf("terminal response mismatch: %#v", terminal)
	}
}
