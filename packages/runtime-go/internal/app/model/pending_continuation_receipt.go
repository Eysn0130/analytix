package model

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
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
	continuations ...ContinuationHistoryAuthorityV1,
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
		PrivateArguments:           append(json.RawMessage(nil), payload.Arguments...),
		ModelVisibleToolDefinition: domainmodel.CloneToolDefinitionSnapshotV1(payload.ModelVisibleToolDefinition),
		Messages:                   ProviderHistoryMessagesFromThreadWithSteeringAuthority(thread, steeringAuthority, continuations...),
		SteeringAuthority:          steeringAuthority,
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
	pending.OutputCapLogicalSequence = payload.OutputCapLogicalSequence
	pending.UserInputTimedWait = domaincontinuation.CloneUserInputTimedWaitV1(payload.UserInputTimedWait)
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
			providerHistoryStringField(item, "prompt") != continuationUserInputPrompt(payload.ToolName, payload.Arguments) {
			return errors.New("continuation user-input item mismatch")
		}
	}
	if !gateFound {
		return errors.New("continuation gate item is missing")
	}
	return nil
}

func continuationUserInputPrompt(toolName string, arguments json.RawMessage) string {
	if toolName == "exit_plan_mode" {
		return domaincontinuation.PlanReviewPromptV1
	}
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

// ContinuationHistoryAuthorityV1 resolves current-installation verified audit
// records. It does not authorize execution, renew a grant, or recreate a Gate.
// The existing Continuation service implements this read-only boundary.
type ContinuationHistoryAuthorityV1 interface {
	ResolveTrustedDispositionForAudit(context.Context, string) (domaincontinuation.Receipt, domaincontinuation.Disposition, error)
}

func pendingUserInputHistoryFromContinuationsV1(thread, turn map[string]any, authority ContinuationHistoryAuthorityV1) map[string]domainmodel.Message {
	contentByCall := map[string]domainmodel.Message{}
	if authority == nil {
		return contentByCall
	}
	ambiguous := map[string]bool{}
	for _, raw := range anySlice(turn["items"]) {
		item, ok := raw.(map[string]any)
		if !ok || providerHistoryRawStringField(item, "kind") != "user_input" || providerHistoryRawStringField(item, "status") != "continued" {
			continue
		}
		inputID := providerHistoryRawStringField(item, "inputId")
		if inputID == "" {
			continue
		}
		receipt, disposition, err := authority.ResolveTrustedDispositionForAudit(context.Background(), inputID)
		if err != nil {
			continue
		}
		content, ok := continuedUserInputPendingHistoryV1(receipt, disposition, thread, turn, item)
		if !ok {
			continue
		}
		callID := receipt.Payload.CallID
		if _, exists := contentByCall[callID]; exists || ambiguous[callID] {
			delete(contentByCall, callID)
			ambiguous[callID] = true
			continue
		}
		current, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
		if err != nil {
			continue
		}
		message := domainmodel.Message{Role: "tool", Name: receipt.Payload.ToolName, ToolCallID: callID, Content: content}
		bound, err := bindPendingUserInputProviderMessageV1(message, receipt, current)
		if err != nil {
			continue
		}
		contentByCall[callID] = bound
	}
	return contentByCall
}

// Reconstruct only the settled original request's bounded pending semantics.
// The locator item has no callId: all call/result matching comes from the
// verified receipt. Current turn execution validation remains a separate path;
// an old audit record never renews or consumes its execution grant.
func continuedUserInputPendingHistoryV1(receipt domaincontinuation.Receipt, disposition domaincontinuation.Disposition, thread, turn, gate map[string]any) (string, bool) {
	payload := receipt.Payload
	if !trustedContinuedUserInputV1(receipt, disposition) ||
		providerHistoryRawStringField(thread, "id") != payload.ThreadID || providerHistoryRawStringField(turn, "id") != payload.TurnID ||
		providerHistoryRawStringField(gate, "id") != payload.ItemID || providerHistoryRawStringField(gate, "kind") != "user_input" || providerHistoryRawStringField(gate, "inputId") != payload.GateID || providerHistoryRawStringField(gate, "status") != "continued" || providerHistoryRawStringField(gate, "continuationReceiptId") != receipt.ReceiptID {
		return "", false
	}
	frozen, ok := providerHistoryTurnContext(turn)
	issuedAt, issuedErr := time.Parse(time.RFC3339Nano, payload.IssuedAt)
	disposedAt, disposedErr := time.Parse(time.RFC3339Nano, disposition.DisposedAt)
	if !ok || frozen != payload.SecurityContext || issuedErr != nil || disposedErr != nil || disposedAt.Before(issuedAt) {
		return "", false
	}
	var call, result map[string]any
	callCount, resultCount, gateCount := 0, 0, 0
	for _, raw := range anySlice(turn["items"]) {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if providerHistoryRawStringField(item, "id") == payload.ItemID {
			gateCount++
		}
		if providerHistoryRawStringField(item, "callId") != payload.CallID {
			continue
		}
		switch providerHistoryRawStringField(item, "kind") {
		case "tool_call":
			callCount++
			call = item
		case "tool_result":
			resultCount++
			result = item
		}
	}
	if callCount != 1 || resultCount != 1 || gateCount != 1 ||
		!providerHistoryCurrentToolCallIdentityV1(payload.TurnID, call) || !providerHistoryCurrentToolResultIdentityV1(payload.TurnID, result) ||
		providerHistoryRawStringField(call, "id") != payload.ToolCallItemID || providerHistoryRawStringField(call, "toolName") != payload.ToolName || providerHistoryRawStringField(result, "toolName") != payload.ToolName ||
		providerHistoryRawStringField(result, "threadId") != payload.ThreadID || providerHistoryRawStringField(result, "status") != "completed" || result["isError"] != false {
		return "", false
	}
	projectedResult := domaintoolresult.PublicToolResultItemRecordV1(result)
	if projectedResult["status"] != "completed" || projectedResult["isError"] != false || projectedResult["lifecycleStatusWithheld"] == true {
		return "", false
	}
	if _, err := domaintoolcall.ParsePublicToolCallArgumentsProjectionV1(call["arguments"]); err != nil {
		return "", false
	}
	grant, err := domainsecurity.ParseExecutionGrant(call["executionGrant"])
	if err != nil || grant != payload.ExecutionGrant {
		return "", false
	}
	for _, item := range []map[string]any{call, result} {
		epoch, valid := providerHistoryUint64V1(item["contextEpoch"])
		if !valid || epoch != frozen.ContextEpoch || providerHistoryRawStringField(item, "contextDigest") != frozen.ContextDigest || providerHistoryRawStringField(item, "executionGrantId") != grant.GrantID {
			return "", false
		}
	}
	// This is the existing first-result message, not stored Question/answer
	// text or a late-answer envelope. Public result persistence stays closed.
	return ToolResultContent(map[string]any{"pending": true, "callId": payload.CallID, "message": "The question remains open for a later answer. Continue independent work; no answer or approval was supplied."}), true
}

const pendingUserInputProviderMessageV1 = "The question remains open for a later answer. Continue independent work; no answer or approval was supplied."

// All fields are immutable process-local values; JSON cannot recreate this
// concrete carrier. Source execution authority remains separate from current
// request scope, so verified old history never renews its grant.
type pendingUserInputProviderBindingV1 struct {
	threadID, contextDigest, sourceTurnID, receiptID, callID, toolName string
	role, name, outerCallID, content                                   string
	submittedQuestionIDs                                               string
}

func pendingUserInputProviderContentV1(callID string) string {
	return ToolResultContent(map[string]any{"pending": true, "callId": callID, "message": pendingUserInputProviderMessageV1})
}

func trustedContinuedUserInputV1(receipt domaincontinuation.Receipt, disposition domaincontinuation.Disposition) bool {
	payload := receipt.Payload
	issued, issuedErr := time.Parse(time.RFC3339Nano, payload.IssuedAt)
	disposed, disposedErr := time.Parse(time.RFC3339Nano, disposition.DisposedAt)
	return domaincontinuation.ValidateReceiptForExecution(receipt) == nil &&
		domaincontinuation.ValidateDisposition(disposition) == nil &&
		payload.Kind == domaincontinuation.KindUserInput &&
		(payload.ToolName == "request_user_input" || payload.ToolName == "user_input") &&
		payload.UserInputTimedWait != nil && payload.UserInputTimedWait.TimeoutSeconds != -1 &&
		payload.UserInputReplyCapture == nil && payload.UserInputDraft == nil &&
		domainsecurity.TurnSecurityContextIsGeneral(payload.SecurityContext) &&
		disposition.GateID == payload.GateID && disposition.ReceiptID == receipt.ReceiptID &&
		disposition.Kind == payload.Kind && disposition.Status == domaincontinuation.StatusContinued &&
		disposition.ReasonCode == "user_input_continued" && disposition.UserInputSubmission == nil &&
		disposition.UserInputReplySourceID == "" && issuedErr == nil && disposedErr == nil && !disposed.Before(issued)
}

// BindPendingUserInputProviderMessageV1 admits only the real settled original
// pending or submitted call, read through current-installation audit authority.
// Equal-looking JSON or a self-carried receipt is never a minting source.
func BindPendingUserInputProviderMessageV1(
	ctx context.Context,
	authority ContinuationHistoryAuthorityV1,
	pending PendingToolCall,
	message *domainmodel.Message,
) (bool, error) {
	// This binding matches the existing General pending-history contract.
	// Other ordinary-effect Question lanes retain their prior privacy path.
	if message == nil || !domainsecurity.TurnSecurityContextIsGeneral(pending.SecurityContext) ||
		(pending.Call.Name != "request_user_input" && pending.Call.Name != "user_input") {
		return false, nil
	}
	submitted := message.Content != pendingUserInputProviderContentV1(pending.Call.ID)
	if submitted {
		var record map[string]any
		if json.Unmarshal([]byte(message.Content), &record) != nil || record["status"] != "submitted" {
			return false, nil
		}
	}
	if ctx == nil || authority == nil {
		return true, errors.New("pending Question audit authority is unavailable")
	}
	gateID := domaincontinuation.GateID(domaincontinuation.KindUserInput, pending.ThreadID, pending.TurnID,
		pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
	receipt, disposition, err := authority.ResolveTrustedDispositionForAudit(ctx, gateID)
	payload := receipt.Payload
	trusted := trustedContinuedUserInputV1(receipt, disposition)
	if submitted {
		trusted = trustedSubmittedUserInputV1(receipt, disposition)
	}
	if err != nil || !trusted ||
		payload.GateID != gateID || payload.ThreadID != pending.ThreadID || payload.TurnID != pending.TurnID ||
		payload.CallID != pending.Call.ID || payload.ToolName != pending.Call.Name || payload.ToolCallItemID != pending.ToolCallItemID ||
		payload.SecurityContext != pending.SecurityContext || payload.ExecutionGrant != pending.ExecutionGrant ||
		domainsecurity.CanonicalJSONHash(payload.Arguments) != domainsecurity.CanonicalJSONHash(pending.Call.Arguments) ||
		payload.ProviderID != pending.ProviderID || payload.Model != pending.Model ||
		payload.ProviderRouteHash != ProviderContinuationRouteHash(pending.ProviderConfig) ||
		payload.ProviderStepPromptSHA256 != domaincontinuation.CanonicalProviderStepPromptSHA256(pending.Prompt) ||
		payload.LogicalEffect != pending.LogicalEffect || payload.OrdinaryWork != pending.OrdinaryWork ||
		payload.ProviderStepExact != pending.ProviderStepExact || payload.OrdinaryResultInputIsolated != pending.OrdinaryResultInputIsolated ||
		payload.ApprovalPolicy != pending.ApprovalPolicy || payload.SandboxMode != pending.SandboxMode ||
		!reflect.DeepEqual(payload.ToolScope, pending.ToolScope) ||
		!reflect.DeepEqual(payload.ProviderNamespace, pending.ProviderNamespace) ||
		payload.OutputCapLogicalSequence != pending.OutputCapLogicalSequence ||
		!reflect.DeepEqual(payload.UserInputTimedWait, pending.UserInputTimedWait) {
		return true, errors.New("pending Question signed source mismatch")
	}
	if submitted {
		bound, err := bindSubmittedUserInputProviderMessageV1(*message, receipt, disposition, pending.SecurityContext)
		if err != nil {
			return true, err
		}
		*message = bound
		return true, nil
	}
	bound, err := bindPendingUserInputProviderMessageV1(*message, receipt, pending.SecurityContext)
	if err != nil {
		return true, err
	}
	*message = bound
	return true, nil
}

func pendingUserInputProviderMessageShapeV1(message domainmodel.Message) bool {
	return len(message.Parts) == 0 && len(message.ToolCalls) == 0 &&
		message.PrivateAttachmentPlanDigest == "" && message.PrivateUserInputReplyBinding == nil &&
		!message.PrivateUserInputReplyRequired && message.PrivateProviderSemanticBinding == nil &&
		message.PrivateProviderReferenceBinding == nil
}

func bindPendingUserInputProviderMessageV1(message domainmodel.Message, receipt domaincontinuation.Receipt, current domainsecurity.TurnSecurityContext) (domainmodel.Message, error) {
	payload := receipt.Payload
	if domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(current) != nil ||
		!domainsecurity.TurnSecurityContextIsGeneral(current) || current.ThreadID != payload.ThreadID ||
		message.PrivateUserInputPendingBinding != nil || !pendingUserInputProviderMessageShapeV1(message) ||
		message.Role != "tool" || message.Name != payload.ToolName || message.ToolCallID != payload.CallID ||
		message.Content != pendingUserInputProviderContentV1(payload.CallID) {
		return message, errors.New("pending Question model message mismatch")
	}
	message.PrivateUserInputPendingBinding = pendingUserInputProviderBindingV1{
		threadID: current.ThreadID, contextDigest: current.ContextDigest,
		sourceTurnID: payload.TurnID, receiptID: receipt.ReceiptID, callID: payload.CallID, toolName: payload.ToolName,
		role: message.Role, name: message.Name, outerCallID: message.ToolCallID, content: message.Content,
	}
	return message, nil
}

func pendingUserInputProviderBindingMatchesV1(message domainmodel.Message, binding pendingUserInputProviderBindingV1) bool {
	return pendingUserInputProviderMessageShapeV1(message) &&
		message.Role == binding.role && message.Name == binding.name &&
		message.ToolCallID == binding.outerCallID && message.Content == binding.content
}

// PendingUserInputProviderMessageV1 validates and rebuilds the exact bounded
// protocol message for one current request. It exposes no reusable carrier.
func PendingUserInputProviderMessageV1(message domainmodel.Message, current domainsecurity.TurnSecurityContext) (domainmodel.Message, error) {
	binding, ok := message.PrivateUserInputPendingBinding.(pendingUserInputProviderBindingV1)
	if !ok || binding.submittedQuestionIDs != "" || domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(current) != nil ||
		!domainsecurity.TurnSecurityContextIsGeneral(current) || binding.threadID != current.ThreadID ||
		binding.contextDigest != current.ContextDigest || !pendingUserInputProviderBindingMatchesV1(message, binding) {
		return domainmodel.Message{}, errors.New("pending Question provider identity is unavailable")
	}
	content := pendingUserInputProviderContentV1(binding.callID)
	if binding.role == "user" {
		var valid bool
		content, valid = CompletedPrivateProtocolSafeHistoryContentV1(message.Content, binding.toolName)
		if !valid || content != pendingUserInputProviderContentV1(binding.callID) || binding.name != "" || binding.outerCallID != "" {
			return domainmodel.Message{}, errors.New("pending Question private history identity is unavailable")
		}
		content = binding.content
	} else if binding.role != "tool" || binding.name != binding.toolName || binding.outerCallID != binding.callID {
		return domainmodel.Message{}, errors.New("pending Question protocol identity is unavailable")
	}
	return domainmodel.Message{Role: binding.role, Name: binding.name, ToolCallID: binding.outerCallID, Content: content}, nil
}

func rebindPendingUserInputSafeHistoryV1(source domainmodel.Message, projected *domainmodel.Message) {
	if projected == nil || source.PrivateUserInputPendingBinding == nil {
		return
	}
	// Keep a present but invalid origin fail-closed across role conversion.
	// Only a verified exact source below may replace its wire binding.
	projected.PrivateUserInputPendingBinding = source.PrivateUserInputPendingBinding
	binding, ok := source.PrivateUserInputPendingBinding.(pendingUserInputProviderBindingV1)
	if !ok || !pendingUserInputProviderBindingMatchesV1(source, binding) ||
		binding.role != "tool" || source.Name != binding.toolName || source.ToolCallID != binding.callID ||
		(binding.submittedQuestionIDs == "" && source.Content != pendingUserInputProviderContentV1(binding.callID)) ||
		projected.Role != "user" || projected.Name != "" || projected.ToolCallID != "" ||
		!pendingUserInputProviderMessageShapeV1(*projected) {
		return
	}
	content, valid := CompletedPrivateProtocolSafeHistoryContentV1(projected.Content, binding.toolName)
	if !valid || content != source.Content {
		return
	}
	binding.role, binding.name, binding.outerCallID, binding.content = projected.Role, projected.Name, projected.ToolCallID, projected.Content
	projected.PrivateUserInputPendingBinding = binding
}

// Submitted identity is limited to the current settled continuation. Historical
// public records never reconstruct submitted answers from this private audit.
func trustedSubmittedUserInputV1(receipt domaincontinuation.Receipt, disposition domaincontinuation.Disposition) bool {
	payload := receipt.Payload
	issued, issuedErr := time.Parse(time.RFC3339Nano, payload.IssuedAt)
	disposed, disposedErr := time.Parse(time.RFC3339Nano, disposition.DisposedAt)
	return domaincontinuation.ValidateReceiptForExecution(receipt) == nil &&
		domaincontinuation.ValidateDisposition(disposition) == nil &&
		payload.Kind == domaincontinuation.KindUserInput &&
		(payload.ToolName == "request_user_input" || payload.ToolName == "user_input") &&
		payload.UserInputReplyCapture == nil && payload.UserInputDraft == nil &&
		domainsecurity.TurnSecurityContextIsGeneral(payload.SecurityContext) &&
		disposition.GateID == payload.GateID && disposition.ReceiptID == receipt.ReceiptID && disposition.Kind == payload.Kind &&
		disposition.Status == domaincontinuation.StatusSubmitted && disposition.ReasonCode == "user_input_submitted" &&
		disposition.UserInputSubmission != nil && disposition.UserInputReplySourceID == "" &&
		issuedErr == nil && disposedErr == nil && !disposed.Before(issued)
}

// The closed result shape matches the current host continuation output. Every
// answer string, including JSON-valued selectedOptions, remains untrusted text.
func submittedUserInputProviderContentV1(content string) (map[string]any, []map[string]any, bool) {
	var record map[string]any
	if json.Unmarshal([]byte(content), &record) != nil || len(record) != 3 ||
		record["status"] != "submitted" || ToolResultContent(record) != content {
		return nil, nil, false
	}
	rawAnswers, ok := record["answers"].([]any)
	if !ok || len(rawAnswers) < 1 || len(rawAnswers) > 3 || record["answerCount"] != float64(len(rawAnswers)) {
		return nil, nil, false
	}
	answers := make([]map[string]any, 0, len(rawAnswers))
	for _, raw := range rawAnswers {
		answer, ok := raw.(map[string]any)
		if !ok || len(answer) < 3 || len(answer) > 6 {
			return nil, nil, false
		}
		for key, value := range answer {
			if key != "id" && key != "kind" && key != "label" && key != "value" && key != "selectedOptions" && key != "customText" {
				return nil, nil, false
			}
			if _, ok := value.(string); !ok {
				return nil, nil, false
			}
		}
		if answer["id"] == nil || answer["label"] == nil || answer["value"] == nil {
			return nil, nil, false
		}
		answers = append(answers, answer)
	}
	return record, answers, true
}

func bindSubmittedUserInputProviderMessageV1(message domainmodel.Message, receipt domaincontinuation.Receipt, disposition domaincontinuation.Disposition, current domainsecurity.TurnSecurityContext) (domainmodel.Message, error) {
	payload := receipt.Payload
	if !trustedSubmittedUserInputV1(receipt, disposition) || current != payload.SecurityContext ||
		message.PrivateUserInputPendingBinding != nil || !pendingUserInputProviderMessageShapeV1(message) ||
		message.Role != "tool" || message.Name != payload.ToolName || message.ToolCallID != payload.CallID {
		return message, errors.New("submitted Question model message mismatch")
	}
	_, answers, ok := submittedUserInputProviderContentV1(message.Content)
	var records []map[string]any
	if !ok || json.Unmarshal(disposition.UserInputSubmission.Answers, &records) != nil || len(records) != len(answers) {
		return message, errors.New("submitted Question answer identity mismatch")
	}
	allowed := make(map[string]bool, len(records))
	for _, record := range records {
		id, _ := record["id"].(string)
		if disposition.UserInputSubmission.Format == "legacy" {
			id, _ = record["questionId"].(string)
		}
		allowed[id] = true
	}
	ids := make([]string, 0, len(answers))
	for _, answer := range answers {
		id, _ := answer["id"].(string)
		if !allowed[id] {
			return message, errors.New("submitted Question answer identity mismatch")
		}
		delete(allowed, id)
		ids = append(ids, id)
	}
	if len(allowed) != 0 {
		return message, errors.New("submitted Question answer identity mismatch")
	}
	message.PrivateUserInputPendingBinding = pendingUserInputProviderBindingV1{
		threadID: current.ThreadID, contextDigest: current.ContextDigest,
		sourceTurnID: payload.TurnID, receiptID: receipt.ReceiptID, callID: payload.CallID, toolName: payload.ToolName,
		role: message.Role, name: message.Name, outerCallID: message.ToolCallID, content: message.Content,
		submittedQuestionIDs: ToolResultContent(ids),
	}
	return message, nil
}

// UserInputProviderMessageForEffectV1 validates the immutable source message,
// projects the complete submitted result, then restores only signed Core IDs.
// The callback is the final effect's ordinary JSON and text privacy projection.
func UserInputProviderMessageForEffectV1(message domainmodel.Message, current domainsecurity.TurnSecurityContext, project func(string) (string, error)) (domainmodel.Message, error) {
	binding, ok := message.PrivateUserInputPendingBinding.(pendingUserInputProviderBindingV1)
	if !ok || binding.submittedQuestionIDs == "" {
		return PendingUserInputProviderMessageV1(message, current)
	}
	if project == nil || domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(current) != nil ||
		!domainsecurity.TurnSecurityContextIsGeneral(current) || binding.threadID != current.ThreadID ||
		binding.contextDigest != current.ContextDigest || binding.sourceTurnID != current.TurnID ||
		!pendingUserInputProviderBindingMatchesV1(message, binding) {
		return domainmodel.Message{}, errors.New("submitted Question provider identity is unavailable")
	}
	content := message.Content
	if binding.role == "user" {
		var valid bool
		content, valid = CompletedPrivateProtocolSafeHistoryContentV1(content, binding.toolName)
		if !valid || binding.name != "" || binding.outerCallID != "" {
			return domainmodel.Message{}, errors.New("submitted Question private history identity is unavailable")
		}
	} else if binding.role != "tool" || binding.name != binding.toolName || binding.outerCallID != binding.callID {
		return domainmodel.Message{}, errors.New("submitted Question protocol identity is unavailable")
	}
	projected, err := project(content)
	if err != nil {
		return domainmodel.Message{}, err
	}
	record, answers, valid := submittedUserInputProviderContentV1(projected)
	var ids []string
	if !valid || json.Unmarshal([]byte(binding.submittedQuestionIDs), &ids) != nil || len(ids) != len(answers) {
		return domainmodel.Message{}, errors.New("submitted Question projection shape is unavailable")
	}
	for index, id := range ids {
		answers[index]["id"] = id
	}
	content = ToolResultContent(record)
	if binding.role == "user" {
		content = privateProtocolSafeHistoryPrefixV1 + ToolResultContent(deepSeekHistoricalToolResult{Tool: binding.toolName, Status: "completed", Content: content})
	}
	return domainmodel.Message{Role: binding.role, Name: binding.name, ToolCallID: binding.outerCallID, Content: content}, nil
}
