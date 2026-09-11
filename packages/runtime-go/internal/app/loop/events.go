package loop

import (
	"strings"
	"time"

	domainfailure "analytix.local/runtime-go/internal/domain/failure"
)

type AssistantDeltaEventInput struct {
	ThreadID  string
	TurnID    string
	EventKind string
	ItemKind  string
	Text      string
	CreatedAt string
	Trace     map[string]any
}

type PipelineStageEventInput struct {
	ThreadID  string
	TurnID    string
	Stage     string
	Details   map[string]any
	Timestamp string
	Trace     map[string]any
}

type ProviderRetryingEventInput struct {
	ThreadID           string
	TurnID             string
	Attempt            int
	MaxAttempts        int
	Message            string
	ProviderDiagnostic map[string]any
}

type StreamInterruptedRecoveryEventInput struct {
	ThreadID           string
	TurnID             string
	Attempt            int
	MaxAttempts        int
	PartialToolStarted bool
	Message            string
	ProviderDiagnostic map[string]any
}

type EmptyFinalRecoveryEventInput struct {
	ThreadID    string
	TurnID      string
	Attempt     int
	MaxAttempts int
}

func BuildAssistantDeltaEvent(input AssistantDeltaEventInput) map[string]any {
	itemID := "item_" + input.TurnID + "_" + input.ItemKind
	item := map[string]any{
		"id":        itemID,
		"turnId":    input.TurnID,
		"threadId":  input.ThreadID,
		"role":      "assistant",
		"status":    "running",
		"createdAt": input.CreatedAt,
		"kind":      input.ItemKind,
		"text":      input.Text,
	}
	event := map[string]any{
		"kind":     input.EventKind,
		"threadId": input.ThreadID,
		"turnId":   input.TurnID,
		"itemId":   itemID,
		"item":     item,
	}
	if len(input.Trace) > 0 {
		event["trace"] = cloneEventMap(input.Trace)
	}
	return event
}

func BuildPipelineStageEvent(input PipelineStageEventInput) map[string]any {
	event := map[string]any{
		"kind":     "pipeline_stage",
		"threadId": input.ThreadID,
		"turnId":   input.TurnID,
		"stage":    input.Stage,
		"label":    pipelineStageLabel(input.Stage),
	}
	if input.Timestamp != "" {
		event["timestamp"] = input.Timestamp
	}
	if len(input.Details) > 0 {
		event["details"] = cloneEventMap(input.Details)
	}
	if len(input.Trace) > 0 {
		event["trace"] = cloneEventMap(input.Trace)
	}
	return event
}

func pipelineStageLabel(stage string) string {
	switch stage {
	case "setup":
		return "Setup"
	case "pre_start":
		return "Pre-Start"
	case "post_start":
		return "Post-Start"
	case "input_received":
		return "Input Received"
	case "input_cached":
		return "Input Cached"
	case "input_routed":
		return "Input Routed"
	case "input_compressed":
		return "Input Compressed"
	case "input_remembered":
		return "Input Remembered"
	case "pre_send":
		return "Pre-Send"
	case "post_send":
		return "Post-Send"
	case "response_received":
		return "Response Received"
	case "provider_admission_rejected":
		return "Provider admission rejected"
	default:
		return strings.TrimSpace(stage)
	}
}

func UnixMillis(t time.Time) float64 {
	return float64(t.UTC().UnixNano()) / float64(time.Millisecond)
}

func WithTurnStartedTrace(event map[string]any, startedAt string, fallback time.Time, enabled bool) map[string]any {
	if !enabled {
		return event
	}
	traced := cloneEventMap(event)
	trace := map[string]any{}
	if current, ok := event["trace"].(map[string]any); ok {
		trace = cloneEventMap(current)
	}
	at := fallback.UTC()
	if parsed, err := time.Parse(time.RFC3339Nano, startedAt); err == nil {
		at = parsed
	}
	trace["turn_started_at"] = UnixMillis(at)
	traced["trace"] = trace
	return traced
}

func BuildToolCallPartialEvent(threadID, turnID, callID, toolName string) map[string]any {
	return map[string]any{
		"kind":       "tool_call_ready",
		"threadId":   threadID,
		"turnId":     turnID,
		"callId":     callID,
		"toolName":   toolName,
		"readyCount": float64(1),
		"partial":    true,
	}
}

func BuildProviderRetryingEvent(input ProviderRetryingEventInput) map[string]any {
	details := providerPipelineDetails(input.ProviderDiagnostic)
	details["message"] = domainfailure.New(domainfailure.CodeProviderUnavailable, details).Message()
	return map[string]any{
		"kind":       "pipeline_stage",
		"threadId":   input.ThreadID,
		"turnId":     input.TurnID,
		"stage":      "provider_retrying",
		"label":      "Retrying provider stream",
		"attempt":    float64(input.Attempt),
		"maxAttempt": float64(input.MaxAttempts),
		"details":    details,
	}
}

func BuildStreamInterruptedRecoveryEvent(input StreamInterruptedRecoveryEventInput) map[string]any {
	details := providerPipelineDetails(input.ProviderDiagnostic)
	details["message"] = domainfailure.New(domainfailure.CodeProviderStreamInterrupted, details).Message()
	details["visibleRecovery"] = true
	details["recoveryKind"] = "interrupted_stream"
	details["recoveryAttempt"] = float64(input.Attempt)
	details["maxRecoveryAttempt"] = float64(input.MaxAttempts)
	details["partialToolStarted"] = input.PartialToolStarted
	return map[string]any{
		"kind":       "pipeline_stage",
		"threadId":   input.ThreadID,
		"turnId":     input.TurnID,
		"stage":      "provider_retrying",
		"label":      "Recovering interrupted provider stream",
		"attempt":    float64(input.Attempt),
		"maxAttempt": float64(input.MaxAttempts),
		"details":    details,
	}
}

func BuildStepLimitFinalAnswerRecoveryEvent(threadID, turnID string, maxSteps int) map[string]any {
	return map[string]any{
		"kind":     "pipeline_stage",
		"threadId": threadID,
		"turnId":   turnID,
		"stage":    "step_limit_finalizing",
		"label":    "Model step budget reached",
		"details": map[string]any{
			"visibleRecovery": true,
			"recoveryKind":    "step_limit_final_answer",
			"maxModelSteps":   float64(maxSteps),
		},
	}
}

func BuildProviderErrorEvent(threadID, turnID string, failure domainfailure.Record, providerDiagnostic map[string]any) map[string]any {
	failure = domainfailure.Normalize(failure)
	details := providerPipelineDetails(providerDiagnostic)
	details["reasonCode"] = failure.Code()
	details["message"] = failure.Message()
	return map[string]any{
		"kind":     "pipeline_stage",
		"threadId": threadID,
		"turnId":   turnID,
		"stage":    "provider_error",
		"label":    "Provider stream failed",
		"details":  details,
	}
}

func BuildEmptyFinalRecoveryEvent(input EmptyFinalRecoveryEventInput) map[string]any {
	return map[string]any{
		"kind":     "pipeline_stage",
		"threadId": input.ThreadID,
		"turnId":   input.TurnID,
		"stage":    "empty_final_recovered",
		"label":    "Provider returned an empty final response",
		"details": map[string]any{
			"visibleRecovery":     true,
			"recoveryKind":        "empty_final",
			"recoveryAttempt":     float64(input.Attempt),
			"maxRecoveryAttempts": float64(input.MaxAttempts),
		},
	}
}

func BuildEmptyFinalRecoveryExhaustedEvent(input EmptyFinalRecoveryEventInput) map[string]any {
	return map[string]any{
		"kind":     "pipeline_stage",
		"threadId": input.ThreadID,
		"turnId":   input.TurnID,
		"stage":    "provider_error",
		"label":    "Provider returned an empty final response",
		"details": map[string]any{
			"message":             "provider returned empty final response after recovery",
			"reasonCode":          domainfailure.CodeProviderEmptyFinal,
			"visibleRecovery":     true,
			"recoveryKind":        "empty_final",
			"recoveryAttempt":     float64(input.Attempt),
			"maxRecoveryAttempts": float64(input.MaxAttempts),
			"recoveryExhausted":   true,
		},
	}
}

func BuildLoopGuardEvent(threadID, turnID, toolName string, count int, guardKind string) map[string]any {
	return map[string]any{
		"kind":     "pipeline_stage",
		"threadId": threadID,
		"turnId":   turnID,
		"stage":    "loop_guard",
		"label":    "Loop guard nudged the model",
		"details": map[string]any{
			"toolName":        toolName,
			"guardKind":       guardKind,
			"stormCount":      float64(count),
			"visibleRecovery": true,
		},
	}
}

func providerPipelineDetails(providerDiagnostic map[string]any) map[string]any {
	details := map[string]any{}
	if projected := projectProviderDiagnostic(providerDiagnostic); len(projected) > 0 {
		details["providerError"] = projected
	}
	return details
}

func cloneEventMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
