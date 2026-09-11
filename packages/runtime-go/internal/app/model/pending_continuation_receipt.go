package model

import (
	"encoding/json"
	"errors"
	"strings"

	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

type StrictPendingToolProviderResolver interface {
	TurnConfig(providerID, model string) domainmodel.TurnConfig
	HasProvider(providerID string) bool
	ValidateExecutionModel(providerID string, model string) error
}

func RebuildPendingToolCallFromReceipt(receipt domaincontinuation.Receipt, thread map[string]any, resolver StrictPendingToolProviderResolver) (PendingToolCall, error) {
	return RebuildPendingToolCallFromReceiptWithSteeringAuthority(receipt, thread, resolver, nil)
}

func RebuildPendingToolCallFromReceiptWithSteeringAuthority(
	receipt domaincontinuation.Receipt,
	thread map[string]any,
	resolver StrictPendingToolProviderResolver,
	steeringAuthority finalauthorityport.Verifier,
) (PendingToolCall, error) {
	if err := domaincontinuation.ValidateReceiptForExecution(receipt); err != nil || thread == nil || resolver == nil {
		return PendingToolCall{}, errors.New("continuation receipt payload is invalid")
	}
	payload := receipt.Payload
	if providerHistoryStringField(thread, "id") != "" && providerHistoryStringField(thread, "id") != payload.ThreadID {
		return PendingToolCall{}, errors.New("continuation thread identity mismatch")
	}
	turn, ok := TurnByID(thread, payload.TurnID)
	if !ok || (providerHistoryStringField(turn, "status") != "running" && providerHistoryStringField(turn, "status") != "waiting") {
		return PendingToolCall{}, errors.New("continuation turn is inactive")
	}
	if !resolver.HasProvider(payload.ProviderID) || resolver.ValidateExecutionModel(payload.ProviderID, payload.Model) != nil {
		return PendingToolCall{}, errors.New("continuation provider or model is unavailable")
	}
	providerConfig := resolver.TurnConfig(payload.ProviderID, payload.Model)
	if payload.ReasoningEffort != "" && SupportsReasoningEffort(providerConfig) {
		providerConfig.ReasoningEffort = payload.ReasoningEffort
	}
	if providerConfig.ProviderID != payload.ProviderID || providerConfig.Model != payload.Model || strings.TrimSpace(providerConfig.APIKey) == "" ||
		ProviderContinuationRouteHash(providerConfig) != payload.ProviderRouteHash {
		return PendingToolCall{}, errors.New("continuation provider execution route changed")
	}
	if err := validateContinuationThreadRecords(receipt, thread, turn); err != nil {
		return PendingToolCall{}, err
	}
	event := continuationPayloadEvent(payload)
	pending, ok := BuildPendingToolCall(PendingToolCallInput{
		Event: event, Thread: thread, Turn: turn, ProviderConfig: providerConfig, ProviderID: payload.ProviderID, Model: payload.Model,
		Effort: payload.ReasoningEffort, Workspace: providerHistoryStringField(thread, "workspace"), ApprovalPolicy: payload.ApprovalPolicy,
		SandboxMode: payload.SandboxMode, Mode: payload.Mode, ToolScope: append([]string(nil), payload.ToolScope...),
		PrivateArguments:  append(json.RawMessage(nil), payload.Arguments...),
		Messages:          ProviderHistoryMessagesFromThreadWithSteeringAuthority(thread, steeringAuthority),
		SteeringAuthority: steeringAuthority,
	})
	if !ok || strings.TrimSpace(pending.Prompt) == "" {
		return PendingToolCall{}, errors.New("continuation pending tool reconstruction failed")
	}
	if domaincontinuation.CanonicalProviderStepPromptSHA256(pending.Prompt) != payload.ProviderStepPromptSHA256 {
		return PendingToolCall{}, errors.New("continuation provider-step prompt binding mismatch")
	}
	pending.LogicalEffect = payload.LogicalEffect
	pending.OrdinaryWork = payload.OrdinaryWork
	pending.ProviderStepExact = payload.ProviderStepExact
	pending.CaseSourceUnavailable = payload.CaseSourceUnavailable
	pending.OrdinaryResultInputIsolated = payload.OrdinaryResultInputIsolated
	pending.PrivateProtocolObserved = payload.PrivateProtocolObserved
	pending.PriorSettledToolRefs = append([]domainsecurity.SettledToolReference(nil), payload.PriorSettledToolRefs...)
	pending.ProviderNamespace = domaincontinuation.CloneProviderContinuationNamespaceV1(payload.ProviderNamespace)
	pending.TerminalRecoveryKind = payload.TerminalRecoveryKind
	return pending, nil
}

func validateContinuationThreadRecords(receipt domaincontinuation.Receipt, thread, turn map[string]any) error {
	payload := receipt.Payload
	currentContext, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if err != nil || currentContext.ContextDigest != payload.SecurityContext.ContextDigest {
		return errors.New("continuation current thread context mismatch")
	}
	turnContext, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || turnContext != payload.SecurityContext {
		return errors.New("continuation frozen turn context mismatch")
	}
	callID, toolName, itemID, ok := ToolCallIdentityFromTurn(turn, payload.CallID, payload.ToolCallItemID, payload.ToolName)
	if !ok || itemID != payload.ToolCallItemID || callID != payload.CallID || toolName != payload.ToolName ||
		domainsecurity.CanonicalJSONHash(payload.Arguments) != payload.ExecutionGrant.ArgsHash {
		return errors.New("continuation tool call record mismatch")
	}
	gateFound := false
	for _, raw := range anySlice(turn["items"]) {
		item, ok := raw.(map[string]any)
		if !ok || providerHistoryStringField(item, "id") != payload.ItemID {
			continue
		}
		gateFound = true
		if providerHistoryStringField(item, "status") != "pending" || providerHistoryStringField(item, "continuationReceiptId") != receipt.ReceiptID {
			return errors.New("continuation gate item is not pending or receipt-bound")
		}
		if payload.Kind == domaincontinuation.KindApproval {
			if providerHistoryStringField(item, "kind") != "approval" || providerHistoryStringField(item, "approvalId") != payload.GateID ||
				providerHistoryStringField(item, "toolName") != payload.ToolName {
				return errors.New("continuation approval item mismatch")
			}
		} else if providerHistoryStringField(item, "kind") != "user_input" || providerHistoryStringField(item, "inputId") != payload.GateID ||
			providerHistoryStringField(item, "prompt") != continuationUserInputPrompt(payload.Arguments) {
			return errors.New("continuation user-input item mismatch")
		}
	}
	if !gateFound {
		return errors.New("continuation gate item is missing")
	}
	return nil
}

func continuationUserInputPrompt(arguments json.RawMessage) string {
	args := map[string]any{}
	_ = json.Unmarshal(arguments, &args)
	for _, key := range []string{"prompt", "question"} {
		if value, ok := args[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "User input required"
}

func continuationPayloadEvent(payload domaincontinuation.Payload) map[string]any {
	event := map[string]any{
		"threadId": payload.ThreadID, "turnId": payload.TurnID, "callId": payload.CallID, "toolName": payload.ToolName,
		"toolCallItemId": payload.ToolCallItemID, "providerId": payload.ProviderID, "model": payload.Model,
		"workspaceCheckpointId": payload.WorkspaceCheckpointID, "approvalPolicy": payload.ApprovalPolicy, "sandboxMode": payload.SandboxMode,
		"subagentDepth": float64(payload.SubagentDepth), "reasoningEffort": payload.ReasoningEffort, "mode": payload.Mode,
		"disableUserInput": payload.DisableUserInput, "toolScope": stringListAny(payload.ToolScope),
		"turnSecurityContext": continuationRecord(payload.SecurityContext), "executionGrant": continuationRecord(payload.ExecutionGrant),
	}
	if payload.MaxModelSteps != nil {
		event["maxModelSteps"] = float64(*payload.MaxModelSteps)
	}
	if payload.EffectiveMaxModelSteps != nil {
		event["effectiveMaxModelSteps"] = float64(*payload.EffectiveMaxModelSteps)
	}
	if len(payload.GUIPlan) > 0 {
		var plan map[string]any
		if json.Unmarshal(payload.GUIPlan, &plan) == nil {
			event["guiPlan"] = plan
		}
	}
	return event
}

func continuationRecord(value any) map[string]any {
	body, _ := json.Marshal(value)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}

func stringListAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}
