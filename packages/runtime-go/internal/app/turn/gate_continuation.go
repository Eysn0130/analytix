package turn

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type GateContinuationInput struct {
	CallID                      string
	ToolName                    string
	ToolCallItemID              string
	ProviderID                  string
	Model                       string
	ProviderRouteHash           string
	WorkspaceCheckpointID       string
	ApprovalPolicy              string
	SandboxMode                 string
	SubagentDepth               int
	ReasoningEffort             string
	Mode                        string
	GUIPlan                     map[string]any
	DisableUserInput            bool
	MaxModelSteps               *int
	EffectiveMaxModelSteps      *int
	ToolScope                   []string
	LogicalEffect               domainsecurity.LogicalEffect
	OrdinaryWork                bool
	ProviderStepExact           bool
	CaseSourceUnavailable       bool
	OrdinaryResultInputIsolated bool
	ProviderStepPromptSHA256    string
	SecurityContext             domainsecurity.TurnSecurityContext
	ExecutionGrant              domainsecurity.ExecutionGrant
}

func GateContinuationInputFromPendingTool(pending appmodel.PendingToolCall) GateContinuationInput {
	return GateContinuationInput{
		CallID:                      pending.Call.ID,
		ToolName:                    pending.Call.Name,
		ToolCallItemID:              pending.ToolCallItemID,
		ProviderID:                  pending.ProviderID,
		Model:                       pending.Model,
		ProviderRouteHash:           appmodel.ProviderContinuationRouteHash(pending.ProviderConfig),
		WorkspaceCheckpointID:       pending.WorkspaceCheckpointID,
		ApprovalPolicy:              pending.ApprovalPolicy,
		SandboxMode:                 pending.SandboxMode,
		SubagentDepth:               pending.SubagentDepth,
		ReasoningEffort:             pending.Effort,
		Mode:                        pending.Mode,
		GUIPlan:                     pending.GUIPlan,
		DisableUserInput:            pending.DisableUserInput,
		MaxModelSteps:               pending.MaxModelSteps,
		EffectiveMaxModelSteps:      pending.EffectiveMaxModelSteps,
		ToolScope:                   pending.ToolScope,
		LogicalEffect:               pending.LogicalEffect,
		OrdinaryWork:                pending.OrdinaryWork,
		ProviderStepExact:           pending.ProviderStepExact,
		CaseSourceUnavailable:       pending.CaseSourceUnavailable,
		OrdinaryResultInputIsolated: pending.OrdinaryResultInputIsolated,
		ProviderStepPromptSHA256:    domaincontinuation.CanonicalProviderStepPromptSHA256(pending.Prompt),
		SecurityContext:             pending.SecurityContext,
		ExecutionGrant:              pending.ExecutionGrant,
	}
}

func GateContinuationFields(input GateContinuationInput) map[string]any {
	fields := map[string]any{
		"callId":                strings.TrimSpace(input.CallID),
		"toolName":              strings.TrimSpace(input.ToolName),
		"toolCallItemId":        strings.TrimSpace(input.ToolCallItemID),
		"providerId":            strings.TrimSpace(input.ProviderID),
		"model":                 strings.TrimSpace(input.Model),
		"providerRouteHash":     strings.TrimSpace(input.ProviderRouteHash),
		"workspaceCheckpointId": strings.TrimSpace(input.WorkspaceCheckpointID),
		"approvalPolicy":        strings.TrimSpace(input.ApprovalPolicy),
		"sandboxMode":           strings.TrimSpace(input.SandboxMode),
		"subagentDepth":         float64(input.SubagentDepth),
	}
	if effort, valid := domainmodel.ProjectReasoningEffortV1(input.ReasoningEffort); valid && effort != "" {
		fields["reasoningEffort"] = effort
	}
	if strings.TrimSpace(input.Mode) != "" {
		fields["mode"] = strings.TrimSpace(input.Mode)
	}
	if input.GUIPlan != nil {
		fields["guiPlan"] = cloneContinuationMap(input.GUIPlan)
	}
	if input.DisableUserInput {
		fields["disableUserInput"] = true
	}
	if input.MaxModelSteps != nil {
		fields["maxModelSteps"] = float64(*input.MaxModelSteps)
	}
	if input.EffectiveMaxModelSteps != nil {
		fields["effectiveMaxModelSteps"] = float64(*input.EffectiveMaxModelSteps)
	}
	if len(input.ToolScope) > 0 {
		fields["toolScope"] = continuationStringListAny(input.ToolScope)
	}
	if domainsecurity.ValidateLogicalEffect(input.LogicalEffect) == nil {
		fields["logicalEffect"] = string(input.LogicalEffect)
	}
	if input.OrdinaryWork {
		fields["ordinaryWork"] = true
	}
	if input.ProviderStepExact {
		fields["providerStepExact"] = true
	}
	if input.CaseSourceUnavailable {
		fields["caseSourceUnavailable"] = true
	}
	if input.OrdinaryResultInputIsolated {
		fields["ordinaryResultInputIsolated"] = true
	}
	if domainsecurity.IsSHA256Hex(input.ProviderStepPromptSHA256) {
		fields["providerStepPromptSha256"] = input.ProviderStepPromptSHA256
	}
	fields["turnSecurityContext"] = continuationContractRecord(input.SecurityContext)
	fields["executionGrant"] = continuationContractRecord(input.ExecutionGrant)
	return fields
}

func GateContinuationPayload(kind string, gateID string, itemID string, pending appmodel.PendingToolCall, issuedAt time.Time) (domaincontinuation.Payload, error) {
	guiPlan, err := domaincontinuation.CanonicalGUIPlan(pending.GUIPlan)
	if err != nil {
		return domaincontinuation.Payload{}, err
	}
	arguments, err := domaincontinuation.CanonicalPrivateToolArgumentsV2(pending.Call.Arguments)
	if err != nil {
		return domaincontinuation.Payload{}, err
	}
	if issuedAt.IsZero() {
		issuedAt = time.Now().UTC()
	}
	payload := domaincontinuation.Payload{
		Version: domaincontinuation.ContractVersion, Kind: strings.TrimSpace(kind), GateID: strings.TrimSpace(gateID),
		ThreadID: pending.ThreadID, TurnID: pending.TurnID, ItemID: strings.TrimSpace(itemID), CallID: pending.Call.ID,
		ToolName: pending.Call.Name, ToolCallItemID: pending.ToolCallItemID, Arguments: append(json.RawMessage(nil), arguments...),
		ProviderID: pending.ProviderID, Model: pending.Model,
		ProviderRouteHash: appmodel.ProviderContinuationRouteHash(pending.ProviderConfig), WorkspaceCheckpointID: pending.WorkspaceCheckpointID,
		ApprovalPolicy: pending.ApprovalPolicy, SandboxMode: pending.SandboxMode, SubagentDepth: pending.SubagentDepth,
		ReasoningEffort: pending.Effort, Mode: pending.Mode, GUIPlan: guiPlan, DisableUserInput: pending.DisableUserInput,
		MaxModelSteps: cloneContinuationInt(pending.MaxModelSteps), EffectiveMaxModelSteps: cloneContinuationInt(pending.EffectiveMaxModelSteps),
		ToolScope: append([]string(nil), pending.ToolScope...), PriorSettledToolRefs: append([]domainsecurity.SettledToolReference(nil), pending.PriorSettledToolRefs...),
		LogicalEffect: pending.LogicalEffect, OrdinaryWork: pending.OrdinaryWork, ProviderStepExact: pending.ProviderStepExact,
		CaseSourceUnavailable: pending.CaseSourceUnavailable, OrdinaryResultInputIsolated: pending.OrdinaryResultInputIsolated,
		PrivateProtocolObserved:  pending.PrivateProtocolObserved,
		ProviderStepPromptSHA256: domaincontinuation.CanonicalProviderStepPromptSHA256(pending.Prompt),
		ProviderNamespace:        domaincontinuation.CloneProviderContinuationNamespaceV1(pending.ProviderNamespace), TerminalRecoveryKind: pending.TerminalRecoveryKind,
		SecurityContext: pending.SecurityContext, ExecutionGrant: pending.ExecutionGrant,
		IssuedAt: issuedAt.UTC().Format(time.RFC3339Nano),
	}
	if err := domaincontinuation.ValidatePayloadForExecution(payload); err != nil {
		return domaincontinuation.Payload{}, err
	}
	if err := executiongrantapp.ValidateExecutionGrantForCall(
		payload.SecurityContext,
		payload.ExecutionGrant,
		domainmodel.ToolCall{ID: payload.CallID, Name: payload.ToolName, Arguments: payload.Arguments},
	); err != nil {
		return domaincontinuation.Payload{}, err
	}
	return payload, nil
}

func ValidatePendingContinuationReceipt(receipt domaincontinuation.Receipt, pending appmodel.PendingToolCall) error {
	issuedAt, err := time.Parse(time.RFC3339Nano, receipt.Payload.IssuedAt)
	if err != nil {
		return errors.New("continuation receipt issue time is invalid")
	}
	expected, err := GateContinuationPayload(receipt.Payload.Kind, receipt.Payload.GateID, receipt.Payload.ItemID, pending, issuedAt)
	if err != nil || !reflect.DeepEqual(expected, receipt.Payload) {
		return errors.New("continuation receipt does not match the in-memory pending authority")
	}
	return nil
}

func cloneContinuationInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func GateContinuationUnavailableResponse(kind string, id string) map[string]any {
	kind = strings.TrimSpace(kind)
	id = strings.TrimSpace(id)
	return map[string]any{
		"code":    "gate_continuation_unavailable",
		"kind":    kind,
		"id":      id,
		"message": "pending gate continuation is unavailable after runtime restart; start a new turn to retry",
	}
}

func GateContinuationTerminalResponse(kind string, id string, status string) map[string]any {
	kind = strings.TrimSpace(kind)
	id = strings.TrimSpace(id)
	status = strings.TrimSpace(status)
	return map[string]any{
		"code":       "gate_continuation_terminal_turn",
		"kind":       kind,
		"id":         id,
		"turnStatus": status,
		"message":    "pending gate continuation is unavailable because the turn is already " + status,
	}
}

func cloneContinuationMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func continuationStringListAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, strings.TrimSpace(value))
		}
	}
	return out
}

func continuationContractRecord(value any) map[string]any {
	body, _ := json.Marshal(value)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}
