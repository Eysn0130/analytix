package model

import (
	"encoding/json"
	"math"
	"strings"

	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

type PendingToolCall struct {
	ThreadID              string
	TurnID                string
	ProviderConfig        domainmodel.TurnConfig
	ProviderID            string
	Model                 string
	Effort                string
	Workspace             string
	Prompt                string
	LogicalEffect         domainsecurity.LogicalEffect
	OrdinaryWork          bool
	ProviderStepExact     bool
	CaseSourceUnavailable bool
	// OrdinaryResultInputIsolated is host-owned provider-lane provenance. It is
	// excluded from public history and provider payloads, but an exact paused
	// gate binds the boolean in its private signed continuation receipt so a
	// same-process continuation cannot broaden the provider input lane.
	OrdinaryResultInputIsolated bool `json:"-"`
	AttachmentIDs               []string
	AttachmentPlanDigest        string
	WorkspaceCheckpointID       string
	Mode                        string
	GUIPlan                     map[string]any
	ApprovalPolicy              string
	SandboxMode                 string
	DisableUserInput            bool
	MaxModelSteps               *int
	EffectiveMaxModelSteps      *int
	Messages                    []domainmodel.Message
	PrivateProtocolSession      *domainmodel.PrivateProtocolSession     `json:"-"`
	AnthropicCapsule            *domainmodel.AnthropicThinkingCapsule   `json:"-"`
	PrivateProtocolCapsules     []*domainmodel.AnthropicThinkingCapsule `json:"-"`
	// PrivateProtocolObserved is a non-secret durable fact bound by a signed
	// gate continuation receipt. It tells pause/resume and restart to compile
	// safe semantic history without replaying process-local protocol bytes.
	PrivateProtocolObserved bool `json:"-"`
	ProviderNamespace       domaincontinuation.ProviderContinuationNamespaceV1
	TerminalRecoveryKind    domaincontinuation.TerminalRecoveryKindV1
	PriorSettledToolRefs    []domainsecurity.SettledToolReference
	Call                    domainmodel.ToolCall
	ToolCallItemID          string
	ToolScope               []string
	SubagentDepth           int
	SecurityContext         domainsecurity.TurnSecurityContext
	ExecutionGrant          domainsecurity.ExecutionGrant
	ApprovalTransition      *domainsecurity.ApprovalGrantTransitionV1 `json:"-"`
}

type PendingToolCallInput struct {
	Event             map[string]any
	Thread            map[string]any
	Turn              map[string]any
	ProviderConfig    domainmodel.TurnConfig
	ProviderID        string
	Model             string
	Effort            string
	Workspace         string
	ApprovalPolicy    string
	SandboxMode       string
	Mode              string
	Messages          []domainmodel.Message
	ToolScope         []string
	PrivateArguments  json.RawMessage
	SteeringAuthority finalauthorityport.Verifier
}

type PendingToolThreadStore interface {
	GetThread(string) (map[string]any, error)
}

func TurnByID(thread map[string]any, turnID string) (map[string]any, bool) {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return nil, false
	}
	for _, raw := range anySlice(thread["turns"]) {
		turn, ok := raw.(map[string]any)
		if ok && providerHistoryStringField(turn, "id") == turnID {
			return turn, true
		}
	}
	return nil, false
}

func ToolCallIdentityFromTurn(turn map[string]any, callID string, itemID string, toolName string) (string, string, string, bool) {
	callID = strings.TrimSpace(callID)
	itemID = strings.TrimSpace(itemID)
	toolName = strings.TrimSpace(toolName)
	if callID == "" || itemID == "" {
		return "", "", "", false
	}
	for _, raw := range anySlice(turn["items"]) {
		item, ok := raw.(map[string]any)
		if !ok || providerHistoryStringField(item, "kind") != "tool_call" {
			continue
		}
		if providerHistoryStringField(item, "callId") != callID || providerHistoryStringField(item, "id") != itemID {
			continue
		}
		if toolName != "" && providerHistoryStringField(item, "toolName") != toolName {
			return "", "", "", false
		}
		if _, err := domaintoolcall.ParsePublicToolCallArgumentsProjectionV1(item["arguments"]); err != nil {
			return "", "", "", false
		}
		return callID, providerHistoryStringField(item, "toolName"), itemID, true
	}
	return "", "", "", false
}

func BuildPendingToolCall(input PendingToolCallInput) (PendingToolCall, bool) {
	effort, effortValid := domainmodel.ProjectReasoningEffortV1(input.Effort)
	if !effortValid || effort != input.Effort {
		return PendingToolCall{}, false
	}
	callID := strings.TrimSpace(providerHistoryStringField(input.Event, "callId"))
	requestedItemID := strings.TrimSpace(providerHistoryStringField(input.Event, "toolCallItemId"))
	toolName := strings.TrimSpace(providerHistoryStringField(input.Event, "toolName"))
	resolvedCallID, resolvedToolName, itemID, ok := ToolCallIdentityFromTurn(input.Turn, callID, requestedItemID, toolName)
	if !ok {
		return PendingToolCall{}, false
	}
	arguments := append(json.RawMessage(nil), input.PrivateArguments...)
	if len(arguments) == 0 || domainsecurity.CanonicalJSONHash(arguments) == "" {
		return PendingToolCall{}, false
	}
	call := domainmodel.ToolCall{ID: resolvedCallID, Name: resolvedToolName, Arguments: arguments}
	subagentDepth := 0
	if depth, ok := numericAny(input.Event["subagentDepth"]); ok && depth > 0 {
		subagentDepth = depth
	}
	guiPlan := mapFieldClone(input.Event, "guiPlan")
	if guiPlan == nil {
		guiPlan = mapFieldClone(input.Turn, "guiPlan")
	}
	securityValue := input.Event["turnSecurityContext"]
	if securityValue == nil {
		securityValue = input.Turn["securityContext"]
	}
	securityContext, securityErr := domainsecurity.ParseTurnSecurityContext(securityValue)
	executionGrant, grantErr := domainsecurity.ParseExecutionGrant(input.Event["executionGrant"])
	if securityErr != nil || grantErr != nil || domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(securityContext) != nil ||
		domainsecurity.ValidateExecutionGrantForOrdinaryContext(executionGrant, securityContext) != nil ||
		securityContext.ThreadID != strings.TrimSpace(providerHistoryStringField(input.Event, "threadId")) ||
		securityContext.TurnID != strings.TrimSpace(providerHistoryStringField(input.Event, "turnId")) || executionGrant.ContextDigest != securityContext.ContextDigest ||
		executionGrant.TurnID != securityContext.TurnID || executionGrant.Provider != strings.TrimSpace(input.ProviderID) || executionGrant.ToolName != call.Name ||
		executionGrant.ToolCallID != call.ID || executionGrant.ArgsHash != domainsecurity.CanonicalJSONHash(call.Arguments) {
		return PendingToolCall{}, false
	}
	prompt, logicalEffect, ordinaryWork, providerStepExact, ok := pendingProviderStepFromTurnWithSteeringAuthority(
		input.Turn, itemID, securityContext, input.SteeringAuthority,
	)
	if !ok {
		return PendingToolCall{}, false
	}
	providerConfig := input.ProviderConfig
	providerConfig.APIKey = ""
	providerConfig.ProxyURL = ""
	return PendingToolCall{
		ThreadID:               strings.TrimSpace(providerHistoryStringField(input.Event, "threadId")),
		TurnID:                 strings.TrimSpace(providerHistoryStringField(input.Event, "turnId")),
		ProviderConfig:         providerConfig,
		ProviderID:             strings.TrimSpace(input.ProviderID),
		Model:                  strings.TrimSpace(input.Model),
		Effort:                 effort,
		Workspace:              strings.TrimSpace(input.Workspace),
		Prompt:                 prompt,
		LogicalEffect:          logicalEffect,
		OrdinaryWork:           ordinaryWork,
		ProviderStepExact:      providerStepExact,
		AttachmentIDs:          AttachmentIDsFromTurn(input.Turn),
		WorkspaceCheckpointID:  firstNonEmptyAnyString(input.Event["workspaceCheckpointId"], input.Turn["workspaceCheckpointId"]),
		Mode:                   strings.TrimSpace(input.Mode),
		GUIPlan:                guiPlan,
		ApprovalPolicy:         strings.TrimSpace(input.ApprovalPolicy),
		SandboxMode:            strings.TrimSpace(input.SandboxMode),
		DisableUserInput:       BoolFieldWithFallback(input.Event, input.Turn, "disableUserInput"),
		MaxModelSteps:          OptionalIntField(input.Event, input.Turn, "maxModelSteps"),
		EffectiveMaxModelSteps: OptionalIntField(input.Event, input.Turn, "effectiveMaxModelSteps"),
		Messages:               CloneProviderMessages(input.Messages),
		Call:                   call,
		ToolCallItemID:         itemID,
		ToolScope:              append([]string(nil), input.ToolScope...),
		SubagentDepth:          subagentDepth,
		SecurityContext:        securityContext,
		ExecutionGrant:         executionGrant,
	}, true
}

func PendingToolWorkspace(pending PendingToolCall, store PendingToolThreadStore) string {
	workspace := strings.TrimSpace(pending.Workspace)
	if workspace == "" && store != nil && strings.TrimSpace(pending.ThreadID) != "" {
		if thread, err := store.GetThread(pending.ThreadID); err == nil && thread != nil {
			workspace = strings.TrimSpace(providerHistoryStringField(thread, "workspace"))
		}
	}
	return workspace
}

// UserPromptFromTurn recovers the immutable user request that opened a turn.
// Steering messages may follow it, but they must never replace the request
// used to derive the turn's security policy during approval or input resume.
func UserPromptFromTurn(turn map[string]any) string {
	for _, raw := range anySlice(turn["items"]) {
		item, ok := raw.(map[string]any)
		if !ok || providerHistoryStringField(item, "kind") != "user_message" {
			continue
		}
		if providerHistoryStringField(item, "delivery") == "steer" {
			continue
		}
		if prompt := providerHistoryRawStringField(item, "text"); strings.TrimSpace(prompt) != "" {
			return prompt
		}
	}
	return ""
}

func AttachmentIDsFromTurn(turn map[string]any) []string {
	for _, raw := range anySlice(turn["items"]) {
		item, ok := raw.(map[string]any)
		if !ok || providerHistoryStringField(item, "kind") != "user_message" ||
			providerHistoryStringField(item, "delivery") == "steer" {
			continue
		}
		return StringList(item["attachmentIds"])
	}
	return []string{}
}

func OptionalIntField(primary map[string]any, fallback map[string]any, key string) *int {
	if value, ok := numericAny(primary[key]); ok {
		return &value
	}
	if value, ok := numericAny(fallback[key]); ok {
		return &value
	}
	return nil
}

func BoolFieldWithFallback(primary map[string]any, fallback map[string]any, key string) bool {
	if value, ok := primary[key].(bool); ok {
		return value
	}
	value, _ := fallback[key].(bool)
	return value
}

func anySlice(value any) []any {
	values, _ := value.([]any)
	return values
}

func numericAny(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		if typed < math.MinInt || typed > math.MaxInt {
			return 0, false
		}
		return int(typed), true
	case float64:
		// Check before conversion; MaxInt rounds up in float64 on 64-bit hosts.
		if math.IsNaN(typed) || typed < float64(math.MinInt) || typed >= -float64(math.MinInt) || math.Trunc(typed) != typed {
			return 0, false
		}
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil || parsed < math.MinInt || parsed > math.MaxInt {
			return 0, false
		}
		return int(parsed), true
	default:
		return 0, false
	}
}

func firstNonEmptyAny(values ...any) any {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return value
			}
		case nil:
			continue
		default:
			return value
		}
	}
	return nil
}

func firstNonEmptyAnyString(values ...any) string {
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			continue
		}
		if text = strings.TrimSpace(text); text != "" {
			return text
		}
	}
	return ""
}

func mapFieldClone(record map[string]any, key string) map[string]any {
	value, _ := record[key].(map[string]any)
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for childKey, childValue := range value {
		out[childKey] = childValue
	}
	return out
}
