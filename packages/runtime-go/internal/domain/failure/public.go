package failure

import (
	"bytes"
	"encoding/json"
	"math"
	"regexp"
	"strings"
)

const (
	CodeTurnFailed                     = "turn_failed"
	CodeTurnCancelled                  = "turn_cancelled"
	CodeProviderError                  = "provider_error"
	CodeProviderAuthenticationFailed   = "provider_authentication_failed"
	CodeProviderRateLimited            = "provider_rate_limited"
	CodeProviderInsufficientBalance    = "provider_insufficient_balance"
	CodeProviderEndpointNotFound       = "provider_endpoint_not_found"
	CodeProviderRequestRejected        = "provider_request_rejected"
	CodeProviderUnavailable            = "provider_unavailable"
	CodeProviderNetworkUnavailable     = "provider_network_unavailable"
	CodeProviderTimeout                = "provider_timeout"
	CodeProviderStreamInterrupted      = "provider_stream_interrupted"
	CodeProviderModelInvalid           = "provider_model_invalid"
	CodeProviderNotConfigured          = "provider_not_configured"
	CodeProviderToolArgumentsInvalid   = "provider_tool_arguments_invalid"
	CodeProviderReasoningMarkupInvalid = "provider_reasoning_markup_invalid"
	CodeProviderEmptyFinal             = "provider_empty_final"
	CodeAttachmentFallbackTooLarge     = "attachment_text_fallback_too_large"
	CodeHostResponseEventFailed        = "host_response_event_failed"
	CodeHostCandidateLifecycleFailed   = "host_candidate_lifecycle_failed"
	CodeHostCandidateAuthorityFailed   = "host_candidate_authority_failed"
	CodeHostCandidateSteeringFailed    = "host_candidate_steering_failed"
	CodeHostCandidateProjectionFailed  = "host_candidate_projection_failed"
	CodeHostCandidatePublicationFailed = "host_candidate_publication_failed"
)

var (
	codePattern   = regexp.MustCompile(`^[a-z][a-z0-9_]{0,95}$`)
	sha256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

// Record is the only public failure payload that may cross the runtime event,
// HTTP, persistence, and desktop boundaries. Its fields are private so callers
// cannot attach provider text, PII, reasoning, URLs, or arbitrary diagnostics.
type Record struct {
	code     string
	message  string
	severity string
	details  map[string]any
}

// Error carries a closed public failure record through internal call chains
// without retaining provider text, PII, reasoning, URLs, or caller-authored
// messages. Internal diagnostics that need more detail must stay on a private
// observer channel and may never be reconstructed from Error().
type Error struct {
	record Record
}

func NewError(code string, details map[string]any) error {
	return Error{record: New(code, details)}
}

func (err Error) Error() string {
	return err.record.Message()
}

func (err Error) PublicFailureRecord() Record {
	return Normalize(err.record)
}

func New(code string, details map[string]any) Record {
	code = normalizeCode(code)
	return Record{
		code:     code,
		message:  messageForCode(code),
		severity: severityForCode(code),
		details:  projectDetails(details),
	}
}

func Normalize(record Record) Record {
	if record.code == "" || record.message == "" || record.severity == "" {
		return New(CodeTurnFailed, nil)
	}
	return New(record.code, record.details)
}

func (record Record) Code() string {
	return Normalize(record).code
}

func (record Record) Message() string {
	return Normalize(record).message
}

func (record Record) Severity() string {
	return Normalize(record).severity
}

func (record Record) Details() map[string]any {
	normalized := Normalize(record)
	if len(normalized.details) == 0 {
		return nil
	}
	out := make(map[string]any, len(normalized.details))
	for key, value := range normalized.details {
		out[key] = value
	}
	return out
}

// ValidatePublicDetails accepts only the exact closed projection emitted by
// Record.Details. Unknown keys, non-canonical spellings, fractional counters,
// and values that would be silently removed or normalized fail closed.
func ValidatePublicDetails(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	projected := projectDetails(input)
	if len(projected) != len(input) {
		return false
	}
	inputBody, inputErr := json.Marshal(input)
	projectedBody, projectedErr := json.Marshal(projected)
	return inputErr == nil && projectedErr == nil && bytes.Equal(inputBody, projectedBody)
}

// ValidateToolNotAdvertisedDetails accepts the one exact diagnostic shape
// allowed to accompany a public tool_not_advertised terminal. The values bind
// hashes and closed enums only; no provider-supplied tool name is retained.
func ValidateToolNotAdvertisedDetails(input map[string]any) bool {
	if len(input) != 10 || !ValidatePublicDetails(input) {
		return false
	}
	for _, key := range []string{
		"rejectedToolNormalizedNameSha256",
		"rejectedToolCategory",
		"promptRoute",
		"loopStep",
		"advertisedToolCount",
		"advertisedToolManifestHash",
		"advertisedNameSetSortedHash",
		"providerRequestToolManifestHash",
		"runToolStepManifestHash",
		"providerRequestRunToolStepManifestSame",
	} {
		if _, present := input[key]; !present {
			return false
		}
	}
	providerHash, _ := input["providerRequestToolManifestHash"].(string)
	runToolStepHash, _ := input["runToolStepManifestHash"].(string)
	same, ok := input["providerRequestRunToolStepManifestSame"].(bool)
	return ok && same == (providerHash == runToolStepHash)
}

func normalizeCode(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if !codePattern.MatchString(code) || !allowedCode(code) {
		return CodeTurnFailed
	}
	return code
}

func allowedCode(code string) bool {
	switch code {
	case CodeTurnFailed, CodeTurnCancelled, CodeProviderError, CodeProviderAuthenticationFailed,
		CodeProviderRateLimited, CodeProviderInsufficientBalance, CodeProviderEndpointNotFound,
		CodeProviderRequestRejected, CodeProviderUnavailable, CodeProviderNetworkUnavailable,
		CodeProviderTimeout, CodeProviderStreamInterrupted, CodeProviderModelInvalid,
		CodeProviderNotConfigured, CodeProviderToolArgumentsInvalid, CodeProviderReasoningMarkupInvalid,
		CodeProviderEmptyFinal,
		CodeAttachmentFallbackTooLarge,
		CodeHostResponseEventFailed, CodeHostCandidateLifecycleFailed,
		CodeHostCandidateAuthorityFailed, CodeHostCandidateSteeringFailed,
		CodeHostCandidateProjectionFailed, CodeHostCandidatePublicationFailed,
		"publication_receipt_required", "validation_error", "runtime_restarted",
		"turn_recovery_boundary", "approval_denied", "input_cancelled",
		"turn_step_limit_exceeded", "turn_security_authority_unavailable", "turn_security_context_invalid",
		"turn_security_workspace_mismatch", "turn_security_case_binding_mismatch", "turn_security_dataset_snapshot_mismatch",
		"turn_security_risk_policy_mismatch", "tool_call_identity_invalid", "tool_not_advertised",
		"tool_schema_missing", "tool_schema_invalid", "tool_private_arguments", "tool_source_unavailable",
		"tool_invalid_arguments_storm", "tool_failure_storm", "execution_grant_rejected", "source_probe_unavailable",
		"tool_pending_continuation_invalid", "provider_stream_failed", "context_window_hard_limit":
		return true
	}
	return false
}

func messageForCode(code string) string {
	switch code {
	case CodeTurnCancelled:
		return "The turn was cancelled before a verified response was available."
	case CodeProviderAuthenticationFailed:
		return "Provider authentication failed. Check the configured credential."
	case CodeProviderRateLimited:
		return "The provider rate limit was reached. Retry after the bounded delay."
	case CodeProviderInsufficientBalance:
		return "The provider account balance or credit is insufficient."
	case CodeProviderEndpointNotFound:
		return "The model endpoint was not found. Check Provider Base URL and Endpoint format."
	case CodeProviderRequestRejected:
		return "The provider rejected the model request. Response content was withheld."
	case CodeProviderUnavailable:
		return "The model provider is temporarily unavailable."
	case CodeProviderNetworkUnavailable:
		return "The model provider could not be reached."
	case CodeProviderTimeout:
		return "The provider request timed out before a verified response was available."
	case CodeProviderStreamInterrupted:
		return "The provider stream was interrupted before a verified response was available."
	case CodeProviderModelInvalid:
		return "The selected model is not configured for this provider."
	case CodeProviderNotConfigured:
		return "The selected provider is not configured for this runtime."
	case CodeProviderToolArgumentsInvalid:
		return "The provider supplied incomplete or invalid tool arguments; execution was blocked."
	case CodeProviderReasoningMarkupInvalid:
		return "The provider returned invalid private-reasoning markup; the response was blocked."
	case CodeProviderEmptyFinal:
		return "The provider returned no final response after the bounded recovery attempt."
	case CodeAttachmentFallbackTooLarge:
		return "The attachment cannot be represented within the bounded text fallback."
	case CodeHostResponseEventFailed:
		return "The provider response completed, but the host could not durably record its closed response stage."
	case CodeHostCandidateLifecycleFailed:
		return "The provider response completed, but the host candidate lifecycle could not continue."
	case CodeHostCandidateAuthorityFailed:
		return "The provider response completed, but current host candidate authority was unavailable."
	case CodeHostCandidateSteeringFailed:
		return "The provider response completed, but current host steering could not be linearized."
	case CodeHostCandidateProjectionFailed:
		return "The provider response completed, but the host could not compile a safe candidate projection."
	case CodeHostCandidatePublicationFailed:
		return "The provider response completed, but the host could not publish the terminal candidate."
	case "runtime_restarted":
		return "The runtime restarted before this turn completed. The turn was marked aborted so the thread can continue."
	case "turn_recovery_boundary":
		return "The bounded recovery path ended without a verified final response."
	case "approval_denied":
		return "The requested operation was denied; no unverified final response was published."
	case "input_cancelled":
		return "The requested input was cancelled; no unverified final response was published."
	case "turn_step_limit_exceeded":
		return "The turn reached its configured model-step limit."
	case "tool_not_advertised":
		return "The provider requested a tool that was not advertised for this turn."
	case "tool_call_identity_invalid":
		return "The provider supplied an invalid or duplicate tool-call identity."
	case "tool_private_arguments":
		return "The provider supplied private reasoning in tool arguments; execution was blocked."
	case "tool_source_unavailable", "source_probe_unavailable":
		return "The required source is not available for the current run."
	case "publication_receipt_required":
		return "Formal artifact publication requires current host publication authority."
	case "tool_invalid_arguments_storm":
		return "Repeated invalid tool arguments caused the host to stop the turn."
	case "tool_failure_storm":
		return "Repeated tool failures caused the host to stop the turn."
	case "validation_error":
		return "The request did not satisfy the current host contract."
	case "context_window_hard_limit":
		return "The bounded request still exceeds the model context window after safe compaction; no provider request was sent."
	}
	switch {
	case strings.HasPrefix(code, "turn_security_"), strings.HasPrefix(code, "execution_grant_"):
		return "Current host security authority rejected the operation."
	case strings.HasPrefix(code, "tool_schema_"):
		return "The tool schema is unavailable or invalid for the current turn."
	case code == "tool_pending_continuation_invalid":
		return "Pending tool continuation failed current host revalidation."
	case strings.HasPrefix(code, "tool_"):
		return "The host rejected or could not complete the tool operation."
	case strings.HasPrefix(code, "approval_"):
		return "The operation did not receive current host approval."
	case strings.HasPrefix(code, "source_"):
		return "The required source is unavailable or no longer current."
	default:
		return "The turn failed before a verified response was available."
	}
}

func severityForCode(code string) string {
	if code == CodeTurnCancelled || code == "runtime_restarted" || code == "turn_recovery_boundary" ||
		code == "approval_denied" || code == "input_cancelled" || code == "source_probe_unavailable" {
		return "warning"
	}
	return "error"
}

func projectDetails(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := map[string]any{}
	for key, value := range input {
		switch key {
		case "status":
			if number, ok := boundedNumber(value, 0, 599); ok {
				out[key] = number
			}
		case "retryAfterMs":
			if number, ok := boundedNumber(value, 0, 900_000); ok {
				out[key] = number
			}
		case "attempt", "maxAttempt", "recoveryAttempt", "maxRecoveryAttempt", "maxRecoveryAttempts", "maxModelSteps", "stormCount", "loopStep", "advertisedToolCount":
			if number, ok := boundedNumber(value, 0, 1_000_000); ok {
				out[key] = number
			}
		case "retryable", "hasApiKey", "executed", "partialToolStarted", "visibleRecovery", "recoveryExhausted", "providerRequestRunToolStepManifestSame":
			if boolean, ok := value.(bool); ok {
				out[key] = boolean
			}
		case "rejectedToolNormalizedNameSha256", "advertisedToolManifestHash", "advertisedNameSetSortedHash", "providerRequestToolManifestHash", "runToolStepManifestHash":
			if digest := stringValue(value); sha256Pattern.MatchString(digest) {
				out[key] = digest
			}
		case "rejectedToolCategory":
			switch stringValue(value) {
			case "known_builtin_not_advertised", "known_mcp_not_advertised", "known_alias_not_advertised", "unknown_provider_name":
				out[key] = stringValue(value)
			}
		case "promptRoute":
			switch stringValue(value) {
			case "direct_answer", "light_agent", "tool_agent", "subagent_agent":
				out[key] = stringValue(value)
			}
		case "authStatus":
			if stringValue(value) == "none" || stringValue(value) == "required" {
				out[key] = stringValue(value)
			}
		case "endpointFormat":
			switch stringValue(value) {
			case "chat_completions", "responses", "messages", "custom_endpoint":
				out[key] = stringValue(value)
			}
		case "failureStage":
			switch stringValue(value) {
			case "request_validation", "request_build", "pre_send_body_audit", "telemetry_begin",
				"transport_before_observed_send", "transport_after_observed_send", "local_admission_config",
				"callback_projection", "unclassified":
				out[key] = stringValue(value)
			}
		case "dispatchState":
			switch stringValue(value) {
			case "not_sent", "sent", "indeterminate":
				out[key] = stringValue(value)
			}
		case "kind":
			switch stringValue(value) {
			case "auth", "rate_limit", "insufficient_balance", "request", "server", "network", "http", "invalid_model", "unknown":
				out[key] = stringValue(value)
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func boundedNumber(value any, minimum, maximum float64) (float64, bool) {
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
	case uint:
		number = float64(typed)
	case uint64:
		number = float64(typed)
	case uint32:
		number = float64(typed)
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		number = parsed
	default:
		return 0, false
	}
	return number, !math.IsNaN(number) && !math.IsInf(number, 0) &&
		!math.Signbit(number) && math.Trunc(number) == number && number >= minimum && number <= maximum &&
		number <= 9_007_199_254_740_991
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}
