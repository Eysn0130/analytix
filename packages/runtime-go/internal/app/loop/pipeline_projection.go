package loop

import (
	"errors"
	"math"
	"strings"
)

var errInvalidPipelineTelemetry = errors.New("pipeline telemetry is not a closed public record")

type pipelineValueKind uint8

const (
	pipelineNumber pipelineValueKind = iota + 1
	pipelineBoolean
	pipelineApprovalPolicy
	pipelineSandboxMode
	pipelinePromptRoute
	pipelineEndpointFormat
	pipelineStopReason
	pipelineAdmissionReason
)

var pipelineDetailSchema = map[string]map[string]pipelineValueKind{
	"setup": {
		"workspaceBound": pipelineBoolean, "caseBound": pipelineBoolean,
	},
	"pre_start": {
		"approvalPolicy": pipelineApprovalPolicy, "sandboxMode": pipelineSandboxMode,
	},
	"post_start": {
		"maxModelSteps": pipelineNumber,
	},
	"input_received": {
		"stepIndex": pipelineNumber, "promptBytes": pipelineNumber,
	},
	"input_cached": pipelineMessageCountSchema(),
	"input_routed": {
		"promptRoute": pipelinePromptRoute, "toolCount": pipelineNumber, "planActive": pipelineBoolean, "subagentDepth": pipelineNumber,
	},
	"input_compressed": pipelineMessageCountSchema(),
	"input_remembered": {
		"memoryCount": pipelineNumber, "contextInstructionCount": pipelineNumber,
	},
	"provider_admission_rejected": {
		"reasonCode": pipelineAdmissionReason, "projectedRequestTokens": pipelineNumber,
		"hardThresholdTokens": pipelineNumber, "providerAttemptCount": pipelineNumber,
	},
	"pre_send":  pipelineProviderRequestSchema(false),
	"post_send": pipelineProviderRequestSchema(true),
	"response_received": {
		"stopReason": pipelineStopReason, "toolCallCount": pipelineNumber, "streamCompleted": pipelineBoolean,
		"durationMs": pipelineNumber, "firstTokenLatencyMs": pipelineNumber,
	},
}

var pipelineTraceSchema = map[string]struct{}{
	"provider_request_build_started_at": {},
	"provider_request_pre_send_at":      {},
	"provider_request_sent_at":          {},
	"provider_response_headers_at":      {},
}

func pipelineMessageCountSchema() map[string]pipelineValueKind {
	return map[string]pipelineValueKind{
		"historyItems": pipelineNumber, "messageCount": pipelineNumber, "contextInstructionCount": pipelineNumber,
		"systemBytes": pipelineNumber, "userBytes": pipelineNumber, "assistantBytes": pipelineNumber, "toolBytes": pipelineNumber,
	}
}

func pipelineProviderRequestSchema(postSend bool) map[string]pipelineValueKind {
	schema := pipelineMessageCountSchema()
	for key, kind := range map[string]pipelineValueKind{
		"attempt": pipelineNumber, "physicalAttempt": pipelineNumber, "toolCount": pipelineNumber, "toolSchemaBytes": pipelineNumber,
		"logicalSequence": pipelineNumber, "outerAttempt": pipelineNumber, "laneCallSequence": pipelineNumber,
		"requestBodyBytes": pipelineNumber, "hasStreamOptionsIncludeUsage": pipelineBoolean,
		"endpointFormat": pipelineEndpointFormat, "provider_request_build_started_at": pipelineNumber,
		"provider_request_pre_send_at": pipelineNumber,
	} {
		schema[key] = kind
	}
	if postSend {
		schema["provider_request_sent_at"] = pipelineNumber
		for key := range map[string]struct{}{
			"status": {}, "provider_response_headers_at": {}, "provider_response_headers_wait_ms": {},
		} {
			schema[key] = pipelineNumber
		}
	}
	return schema
}

func projectPipelineTelemetry(stage string, details map[string]any, trace map[string]any, traceEnabled bool) (map[string]any, map[string]any, error) {
	stage = strings.TrimSpace(stage)
	schema, ok := pipelineDetailSchema[stage]
	if !ok {
		return nil, nil, errInvalidPipelineTelemetry
	}
	projected, err := projectPipelineValues(details, schema)
	if err != nil {
		return nil, nil, err
	}
	if !traceEnabled || len(trace) == 0 {
		return projected, nil, nil
	}
	projectedTrace := make(map[string]any, len(trace))
	for key, value := range trace {
		if _, ok := pipelineTraceSchema[key]; !ok {
			return nil, nil, errInvalidPipelineTelemetry
		}
		number, ok := pipelineFiniteNumber(value)
		if !ok || number < 0 {
			return nil, nil, errInvalidPipelineTelemetry
		}
		projectedTrace[key] = number
	}
	return projected, projectedTrace, nil
}

func projectPipelineValues(input map[string]any, schema map[string]pipelineValueKind) (map[string]any, error) {
	if len(input) == 0 {
		return nil, nil
	}
	projected := make(map[string]any, len(input))
	for key, value := range input {
		kind, ok := schema[key]
		if !ok {
			return nil, errInvalidPipelineTelemetry
		}
		projectedValue, ok := projectPipelineValue(kind, value)
		if !ok {
			return nil, errInvalidPipelineTelemetry
		}
		projected[key] = projectedValue
	}
	return projected, nil
}

func projectPipelineValue(kind pipelineValueKind, value any) (any, bool) {
	switch kind {
	case pipelineNumber:
		number, ok := pipelineFiniteNumber(value)
		return number, ok && number >= 0 && number <= 1e15
	case pipelineBoolean:
		boolean, ok := value.(bool)
		return boolean, ok
	case pipelineApprovalPolicy:
		return pipelineEnum(value, "always", "auto", "on-request", "untrusted", "suggest", "never")
	case pipelineSandboxMode:
		return pipelineEnum(value, "read-only", "workspace-write", "danger-full-access", "external-sandbox")
	case pipelinePromptRoute:
		return pipelineEnum(value, "direct_answer", "light_agent", "tool_agent", "subagent_agent")
	case pipelineEndpointFormat:
		return pipelineEnum(value, "chat_completions", "responses", "messages", "custom_endpoint")
	case pipelineStopReason:
		return normalizePipelineStopReason(value), true
	case pipelineAdmissionReason:
		return pipelineEnum(value, "context_window_hard_limit")
	default:
		return nil, false
	}
}

func pipelineEnum(value any, allowed ...string) (any, bool) {
	text, ok := value.(string)
	if !ok {
		return nil, false
	}
	text = strings.TrimSpace(text)
	for _, candidate := range allowed {
		if text == candidate {
			return text, true
		}
	}
	return nil, false
}

func normalizePipelineStopReason(value any) string {
	text, _ := value.(string)
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "stop", "end_turn", "completed", "complete":
		return "stop"
	case "length", "max_tokens", "max_output_tokens":
		return "length"
	case "tool_calls", "tool_use":
		return "tool_calls"
	case "content_filter", "content_filtered", "safety":
		return "content_filter"
	default:
		return "unknown"
	}
}

func pipelineFiniteNumber(value any) (float64, bool) {
	var number float64
	switch typed := value.(type) {
	case float64:
		number = typed
	case float32:
		number = float64(typed)
	case int:
		number = float64(typed)
	case int64:
		number = float64(typed)
	case int32:
		number = float64(typed)
	default:
		return 0, false
	}
	return number, !math.IsNaN(number) && !math.IsInf(number, 0)
}
