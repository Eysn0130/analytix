package evidence

import (
	"context"
	"errors"
	"strings"

	apploop "analytix.local/runtime-go/internal/app/loop"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
)

type TerminalDisposition struct {
	Status   string
	Code     string
	Message  string
	Severity string
}

func GeneralTerminalHostFailure(reason TerminalReason) domainfailure.Record {
	record, ok := domainterminal.GeneralFailureRecordV1(string(reason))
	if !ok {
		return domainfailure.New(domainfailure.CodeTurnFailed, nil)
	}
	return record
}

func DispositionForTerminalReason(reason TerminalReason) (TerminalDisposition, error) {
	closed, ok := domainterminal.FailureProjectionV1(string(reason))
	if !ok {
		return TerminalDisposition{}, errors.New("terminal reason has no closed disposition")
	}
	return TerminalDisposition{
		Status: closed.Status, Code: closed.Code, Message: closed.Message, Severity: closed.Severity,
	}, nil
}

func TerminalReasonForFailure(cause error) TerminalReason {
	if errors.Is(cause, context.DeadlineExceeded) {
		return TerminalTimeout
	}
	if errors.Is(cause, context.Canceled) {
		return TerminalCancel
	}
	code := ""
	var typed apploop.TurnFailureError
	if errors.As(cause, &typed) {
		code = strings.ToLower(strings.TrimSpace(typed.Code))
	} else {
		var coded interface{ TerminalFailureCode() string }
		if errors.As(cause, &coded) {
			code = strings.ToLower(strings.TrimSpace(coded.TerminalFailureCode()))
		}
	}
	if code != "" {
		switch {
		case strings.Contains(code, "step_limit"):
			return TerminalStepLimit
		case strings.Contains(code, "semantic"):
			return TerminalSemanticFailure
		case code == "context_window_hard_limit":
			return TerminalSemanticFailure
		case strings.Contains(code, "pending_continuation"):
			return TerminalSourceUnavailable
		case strings.Contains(code, "source_unavailable"), strings.Contains(code, "server_identity"),
			strings.Contains(code, "server_mismatch"), strings.Contains(code, "connection_epoch"),
			strings.Contains(code, "source_probe"), strings.Contains(code, "schema_unavailable"):
			return TerminalSourceUnavailable
		case strings.Contains(code, "tool"):
			return TerminalToolFailure
		case strings.Contains(code, "stream"):
			return TerminalStreamAbort
		case strings.HasPrefix(code, "host_"):
			return TerminalSemanticFailure
		}
	}
	if code == "" {
		code = strings.ToLower(strings.TrimSpace(apploop.PublicFailureForError(cause).Code()))
		if code == "context_window_hard_limit" {
			return TerminalSemanticFailure
		}
	}
	if apploop.InterruptedStreamCanRecover(cause) {
		return TerminalStreamAbort
	}
	return TerminalProviderFailure
}

// TerminalReasonForRuntimeResult preserves recovery as a sticky downgrade only
// for terminal paths that could otherwise carry evidence. A later hard
// failure/source boundary remains the more specific fail-closed reason.
func TerminalReasonForRuntimeResult(result apploop.RuntimeAgentLoopResult, fallback TerminalReason) TerminalReason {
	if !terminalAllowsEvidence(fallback) {
		return fallback
	}
	switch result.TerminalRecoveryKind {
	case apploop.RuntimeTerminalRecoveryNone:
		return fallback
	case apploop.RuntimeTerminalRecoveryApplied:
		return TerminalRecovery
	case apploop.RuntimeTerminalRecoveryStepLimit:
		return TerminalStepLimit
	default:
		return TerminalSemanticFailure
	}
}

func TerminalReasonForRuntimeCompletion(result apploop.RuntimeAgentLoopResult, sourceUnavailable bool) TerminalReason {
	fallback := TerminalSuccess
	if sourceUnavailable {
		fallback = TerminalSourceUnavailable
	}
	return TerminalReasonForRuntimeResult(result, fallback)
}
