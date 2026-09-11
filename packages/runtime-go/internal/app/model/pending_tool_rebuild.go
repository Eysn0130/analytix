package model

import (
	"encoding/json"
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

type PendingToolProviderResolver interface {
	TurnConfig(providerID, model string) domainmodel.TurnConfig
}

type PendingToolCallRebuildInput struct {
	Event                 map[string]any
	Thread                map[string]any
	ProviderResolver      PendingToolProviderResolver
	RuntimeApprovalPolicy string
	RuntimeSandboxMode    string
	PrivateArguments      json.RawMessage
	SteeringAuthority     finalauthorityport.Verifier
}

func PendingToolCallIDsFromGateEvent(event map[string]any) (string, string, bool) {
	threadID := strings.TrimSpace(providerHistoryStringField(event, "threadId"))
	turnID := strings.TrimSpace(providerHistoryStringField(event, "turnId"))
	return threadID, turnID, threadID != "" && turnID != ""
}

func RebuildPendingToolCallFromGateEvent(input PendingToolCallRebuildInput) (PendingToolCall, bool) {
	_, turnID, ok := PendingToolCallIDsFromGateEvent(input.Event)
	if !ok || input.Thread == nil || input.ProviderResolver == nil {
		return PendingToolCall{}, false
	}
	turn, ok := TurnByID(input.Thread, turnID)
	if !ok {
		return PendingToolCall{}, false
	}
	providerID := firstNonEmptyAnyString(input.Event["providerId"], input.Thread["providerId"])
	model := firstNonEmptyAnyString(input.Event["model"], turn["model"], input.Thread["model"])
	effort, effortValid := pendingReasoningEffortFromRecords(input.Event, turn)
	if !effortValid {
		return PendingToolCall{}, false
	}
	providerConfig := input.ProviderResolver.TurnConfig(providerID, model)
	if effort != "" && SupportsReasoningEffort(providerConfig) {
		providerConfig.ReasoningEffort = effort
	}
	providerID = providerConfig.ProviderID
	model = providerConfig.Model
	effort, effortValid = domainmodel.ProjectReasoningEffortV1(providerConfig.ReasoningEffort)
	if !effortValid || effort != providerConfig.ReasoningEffort {
		return PendingToolCall{}, false
	}
	approvalPolicy := normalizePendingApprovalPolicy(providerHistoryStringField(input.Event, "approvalPolicy"))
	sandboxMode := normalizePendingSandboxMode(providerHistoryStringField(input.Event, "sandboxMode"))
	if approvalPolicy == "" || sandboxMode == "" {
		return PendingToolCall{}, false
	}
	return BuildPendingToolCall(PendingToolCallInput{
		Event:             input.Event,
		Thread:            input.Thread,
		Turn:              turn,
		ProviderConfig:    providerConfig,
		ProviderID:        providerID,
		Model:             model,
		Effort:            effort,
		Workspace:         providerHistoryStringField(input.Thread, "workspace"),
		ApprovalPolicy:    approvalPolicy,
		SandboxMode:       sandboxMode,
		Mode:              normalizePendingTurnMode(firstNonEmptyAnyString(input.Event["mode"], turn["mode"])),
		Messages:          ProviderHistoryMessagesFromThreadWithSteeringAuthority(input.Thread, input.SteeringAuthority),
		ToolScope:         StringList(input.Event["toolScope"]),
		PrivateArguments:  append(json.RawMessage(nil), input.PrivateArguments...),
		SteeringAuthority: input.SteeringAuthority,
	})
}

func ResolvePendingApprovalPolicy(value string, runtimeDefault string) string {
	return firstPendingPolicy(normalizePendingApprovalPolicy, value, runtimeDefault)
}

func ResolvePendingSandboxMode(value string, runtimeDefault string) string {
	return firstPendingPolicy(normalizePendingSandboxMode, value, runtimeDefault)
}

func StringList(value any) []string {
	raw := []any{}
	switch typed := value.(type) {
	case []any:
		raw = typed
	case []string:
		raw = make([]any, 0, len(typed))
		for _, item := range typed {
			raw = append(raw, item)
		}
	default:
		return []string{}
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}

func firstPendingPolicy(normalize func(string) string, values ...string) string {
	for _, value := range values {
		if normalized := normalize(value); normalized != "" {
			return normalized
		}
	}
	return ""
}

func normalizePendingTurnMode(value string) string {
	switch strings.TrimSpace(value) {
	case "agent", "plan":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func pendingReasoningEffortFromRecords(records ...map[string]any) (string, bool) {
	for _, record := range records {
		if record == nil {
			continue
		}
		raw, present := record["reasoningEffort"]
		if !present {
			continue
		}
		value, ok := raw.(string)
		if !ok {
			return "", false
		}
		canonical, valid := domainmodel.ProjectReasoningEffortV1(value)
		if !valid || canonical != value {
			return "", false
		}
		if canonical != "" {
			return canonical, true
		}
	}
	return "", true
}

func normalizePendingApprovalPolicy(value string) string {
	switch strings.TrimSpace(value) {
	case "always", "auto", "on-request", "untrusted", "suggest", "never":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func normalizePendingSandboxMode(value string) string {
	switch strings.TrimSpace(value) {
	case "read-only", "workspace-write", "danger-full-access", "external-sandbox":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}
