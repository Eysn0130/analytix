package server

import (
	"context"
	"errors"
	"strings"

	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	apploop "analytix.local/runtime-go/internal/app/loop"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
)

// AsyncTurnObservationV1 is an in-process regression observation, never a
// public event or persisted record. IDs are private correlation only. The
// observer must not log them or alter runtime execution.
type AsyncTurnObservationV1 struct {
	ThreadID                 string
	TurnID                   string
	Stage                    string
	CompletionPhase          string
	CompletionErrorClass     string
	FailureRecordErrorClass  string
	CompletionDetailClass    string
	FailureRecordDetailClass string
	TerminalStatus           string
}

type asyncTurnPhaseContextKeyV1 struct{}
type asyncTurnPhaseObserverContextKeyV1 struct{}

func setAsyncTurnCompletionPhaseV1(ctx context.Context, phase string) {
	if ctx == nil {
		return
	}
	if current, ok := ctx.Value(asyncTurnPhaseContextKeyV1{}).(*string); ok {
		*current = phase
	}
	if observe, ok := ctx.Value(asyncTurnPhaseObserverContextKeyV1{}).(func(string)); ok {
		observe(phase)
	}
}

func asyncTurnErrorClassV1(err error) string {
	if err == nil {
		return "none"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline_exceeded"
	}
	if errors.Is(err, caseentityapp.ErrPrivateStateIntegrity) {
		return "case_private_state_integrity"
	}
	var public interface{ PublicFailureRecord() domainfailure.Record }
	if errors.As(err, &public) {
		return domainfailure.Normalize(public.PublicFailureRecord()).Code()
	}
	var terminal interface{ TerminalFailureCode() string }
	if errors.As(err, &terminal) {
		return asyncTurnTypedErrorClassV1(terminal.TerminalFailureCode())
	}
	var failure apploop.TurnFailureError
	if errors.As(err, &failure) {
		return asyncTurnTypedErrorClassV1(failure.Code)
	}
	return "unclassified"
}

func asyncTurnTypedErrorClassV1(code string) string {
	canonical := strings.ToLower(strings.TrimSpace(code))
	closed := domainfailure.New(canonical, nil).Code()
	if canonical != closed {
		return "unclassified"
	}
	return closed
}

func (h *runtimeServerHandler) asyncTurnTerminalStatusV1(input runtimeAgentLoopInput) string {
	status, _, err := h.runtimeTurnTerminalStatus(input.ThreadID, input.TurnID)
	if err != nil {
		return "unobservable"
	}
	switch status {
	case "completed", "failed", "aborted", "cancelled", "interrupted", "running", "waiting":
		return status
	default:
		return "unobservable"
	}
}
