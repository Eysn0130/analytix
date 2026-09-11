package loop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	domainfailure "analytix.local/runtime-go/internal/domain/failure"
)

type TurnFailureError struct {
	Message  string
	Code     string
	Details  map[string]any
	Severity string
}

func (err TurnFailureError) Error() string {
	return err.Message
}

func StepLimitExceededError(maxSteps int) error {
	return TurnFailureError{
		Message:  fmt.Sprintf("Turn stopped after %d model steps without reaching a final response.", maxSteps),
		Code:     "turn_step_limit_exceeded",
		Details:  map[string]any{"maxModelSteps": float64(maxSteps)},
		Severity: "error",
	}
}

func StepLimitFinalAnswerPrompt(maxSteps int) string {
	return fmt.Sprintf(
		"The turn has reached its configured budget of %d model steps. Do not call more tools. Provide the best final answer now, summarizing completed work, remaining uncertainty, and any safe next steps.",
		maxSteps,
	)
}

func InterruptedStreamCanRecover(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var forbidden providerRetryForbiddenError
	if errors.As(err, &forbidden) && forbidden.ProviderRetryForbidden() {
		return false
	}
	var publicFailure publicFailureError
	if errors.As(err, &publicFailure) && publicFailure.PublicFailureRecord().Code() == domainfailure.CodeProviderStreamInterrupted {
		return true
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "stream stalled"),
		strings.Contains(message, "connection reset"),
		strings.Contains(message, "unexpected eof"),
		strings.Contains(message, "ended before completion"),
		strings.Contains(message, "ended before completing tool calls"),
		strings.Contains(message, "use of closed network connection"),
		strings.Contains(message, "response body closed"):
		return true
	default:
		return false
	}
}

// InterruptedStreamCanRecoverWithContext gives host cancellation/deadline
// authority precedence over provider transport classification. A provider may
// report DeadlineExceeded for its own still-live transport budget, but an
// expired parent turn must never receive a recovery prompt or second request.
func InterruptedStreamCanRecoverWithContext(ctx context.Context, err error) bool {
	return ctx != nil && ctx.Err() == nil && InterruptedStreamCanRecover(err)
}

func InterruptedStreamRecoveryPrompt(hasPartialText bool, hadPartialTool bool) string {
	switch {
	case hadPartialTool:
		return "The previous assistant response was interrupted while a tool call was streaming. Its draft and partial tool call were discarded and were never shown to the user. Continue the same task now. If a tool is still needed, issue a fresh complete tool call from scratch; do not rely on partial arguments. The eventual final response must be complete and self-contained."
	case hasPartialText:
		return "The previous assistant response was interrupted during streaming. Its partial text was discarded and was never shown to the user. Produce a complete, self-contained replacement response from the beginning; do not continue from or refer to the discarded draft."
	default:
		return "The previous assistant response was interrupted before a publishable answer was completed. Produce a complete, self-contained replacement response now."
	}
}

type retryableProviderError interface {
	Retryable() bool
}

type providerRetryForbiddenError interface {
	ProviderRetryForbidden() bool
}

type retryAfterProviderError interface {
	RetryAfterDelay() time.Duration
}

type diagnosticProviderError interface {
	Diagnostics() map[string]any
}

type publicFailureError interface {
	PublicFailureRecord() domainfailure.Record
}

func ProviderErrorLooksRetryable(err error) bool {
	if err == nil {
		return false
	}
	var forbidden providerRetryForbiddenError
	if errors.As(err, &forbidden) && forbidden.ProviderRetryForbidden() {
		return false
	}
	var retryable retryableProviderError
	if errors.As(err, &retryable) {
		return retryable.Retryable()
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "returned 408"),
		strings.Contains(msg, "returned 409"),
		strings.Contains(msg, "returned 425"),
		strings.Contains(msg, "returned 429"),
		strings.Contains(msg, "returned 500"),
		strings.Contains(msg, "returned 502"),
		strings.Contains(msg, "returned 503"),
		strings.Contains(msg, "returned 504"),
		strings.Contains(msg, "timeout"),
		strings.Contains(msg, "temporarily unavailable"),
		strings.Contains(msg, "connection reset"),
		strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "fetch failed"),
		strings.Contains(msg, "eof"):
		return true
	default:
		return false
	}
}

func ProviderRetryDelay(err error) time.Duration {
	var retryAfter retryAfterProviderError
	if errors.As(err, &retryAfter) {
		return retryAfter.RetryAfterDelay()
	}
	return 0
}

func ProviderErrorDiagnostic(err error) map[string]any {
	diagnostics := collectProviderDiagnostics(err)
	if len(diagnostics) == 0 {
		return nil
	}
	return projectProviderDiagnostic(diagnostics)
}

// collectProviderDiagnostics walks the existing error chain, including
// errors.Join, so a dispatch/stage wrapper cannot hide the adapter's safe
// provider kind/status fields. It does not inspect Error() text and therefore
// never promotes raw provider payloads, URLs, credentials, or PII.
func collectProviderDiagnostics(err error) map[string]any {
	if err == nil {
		return nil
	}
	merged := map[string]any{}
	var visit func(error)
	visit = func(current error) {
		if current == nil {
			return
		}
		var diagnostic diagnosticProviderError
		if errors.As(current, &diagnostic) {
			for key, value := range diagnostic.Diagnostics() {
				if _, exists := merged[key]; !exists {
					merged[key] = value
				}
			}
		}
		switch unwrapped := current.(type) {
		case interface{ Unwrap() []error }:
			for _, child := range unwrapped.Unwrap() {
				visit(child)
			}
		case interface{ Unwrap() error }:
			visit(unwrapped.Unwrap())
		}
	}
	visit(err)
	return merged
}

func SanitizeProviderRetryMessage(err error) string {
	return PublicFailureForError(err).Message()
}

func PublicFailureForError(err error) domainfailure.Record {
	var publicFailure publicFailureError
	if errors.As(err, &publicFailure) {
		return domainfailure.Normalize(publicFailure.PublicFailureRecord())
	}
	code := publicFailureCode(err)
	details := ProviderErrorDiagnostic(err)
	var typed TurnFailureError
	if errors.As(err, &typed) {
		code = strings.TrimSpace(typed.Code)
		if strings.HasPrefix(code, "tool_pending_continuation_") {
			code = "tool_pending_continuation_invalid"
		}
		details = typed.Details
	}
	return domainfailure.New(code, details)
}

func publicFailureCode(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return domainfailure.CodeTurnCancelled
	case errors.Is(err, context.DeadlineExceeded):
		return domainfailure.CodeProviderTimeout
	}
	diagnostic := ProviderErrorDiagnostic(err)
	status := diagnosticNumber(diagnostic, "status")
	kind, _ := diagnostic["kind"].(string)
	switch {
	case kind == "auth":
		return domainfailure.CodeProviderAuthenticationFailed
	case kind == "rate_limit":
		return domainfailure.CodeProviderRateLimited
	case kind == "insufficient_balance":
		return domainfailure.CodeProviderInsufficientBalance
	case kind == "invalid_model":
		return domainfailure.CodeProviderModelInvalid
	case status == 404:
		return domainfailure.CodeProviderEndpointNotFound
	case kind == "request":
		return domainfailure.CodeProviderRequestRejected
	case kind == "network":
		return domainfailure.CodeProviderNetworkUnavailable
	case kind == "server" || status >= 500:
		return domainfailure.CodeProviderUnavailable
	case InterruptedStreamCanRecover(err):
		return domainfailure.CodeProviderStreamInterrupted
	case len(diagnostic) > 0:
		return domainfailure.CodeProviderError
	default:
		return domainfailure.CodeTurnFailed
	}
}

func projectProviderDiagnostic(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	projected := map[string]any{}
	for _, key := range []string{"providerId", "family", "endpointFormat", "status", "kind", "hasApiKey", "authStatus", "retryable", "retryAfterMs", "attempt"} {
		if value, exists := input[key]; exists {
			projected[key] = value
		}
	}
	projected = domainfailure.New(domainfailure.CodeProviderError, projected).Details()
	if stage, ok := closedProviderFailureStage(input["failureStage"]); ok {
		if projected == nil {
			projected = map[string]any{}
		}
		projected["failureStage"] = stage
	}
	if dispatchState, ok := closedProviderDispatchState(input["dispatchState"]); ok {
		if projected == nil {
			projected = map[string]any{}
		}
		projected["dispatchState"] = dispatchState
	}
	return projected
}

func closedProviderFailureStage(value any) (string, bool) {
	stage, ok := value.(string)
	if !ok || strings.TrimSpace(stage) != stage {
		return "", false
	}
	switch stage {
	case "request_validation", "request_build", "pre_send_body_audit", "telemetry_begin",
		"transport_before_observed_send", "transport_after_observed_send", "local_admission_config",
		"callback_projection", "unclassified":
		return stage, true
	default:
		return "", false
	}
}

func closedProviderDispatchState(value any) (string, bool) {
	state, ok := value.(string)
	if !ok || strings.TrimSpace(state) != state {
		return "", false
	}
	switch state {
	case "not_sent", "sent", "indeterminate":
		return state, true
	default:
		return "", false
	}
}

func diagnosticNumber(input map[string]any, key string) float64 {
	if input == nil {
		return 0
	}
	value, _ := input[key].(float64)
	return value
}
