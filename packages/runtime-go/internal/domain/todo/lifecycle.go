package todo

import (
	"fmt"
	"strings"
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
	StatusCanceled   Status = "canceled"
)

type StatusReasonCode string

const (
	ReasonExecutionFailed    StatusReasonCode = "execution_failed"
	ReasonDependencyFailed   StatusReasonCode = "dependency_failed"
	ReasonVerificationFailed StatusReasonCode = "verification_failed"
	ReasonTimeout            StatusReasonCode = "timeout"
	ReasonToolFailed         StatusReasonCode = "tool_failed"
	ReasonSubagentFailed     StatusReasonCode = "subagent_failed"
	ReasonRuntimeFailed      StatusReasonCode = "runtime_failed"
	ReasonUserCanceled       StatusReasonCode = "user_canceled"
	ReasonParentCanceled     StatusReasonCode = "parent_canceled"
	ReasonRuntimeCanceled    StatusReasonCode = "runtime_canceled"
	ReasonSuperseded         StatusReasonCode = "superseded"
)

var failureReasons = map[StatusReasonCode]struct{}{
	ReasonExecutionFailed: {}, ReasonDependencyFailed: {}, ReasonVerificationFailed: {},
	ReasonTimeout: {}, ReasonToolFailed: {}, ReasonSubagentFailed: {}, ReasonRuntimeFailed: {},
}

var cancellationReasons = map[StatusReasonCode]struct{}{
	ReasonUserCanceled: {}, ReasonParentCanceled: {}, ReasonRuntimeCanceled: {}, ReasonSuperseded: {},
}

func ParseStatus(value string) (Status, error) {
	status := Status(strings.TrimSpace(value))
	switch status {
	case StatusPending, StatusInProgress, StatusCompleted, StatusFailed, StatusCanceled:
		return status, nil
	default:
		return "", fmt.Errorf("invalid todo status %q", value)
	}
}

func ValidateStatusReason(status Status, value string) error {
	reason := StatusReasonCode(strings.TrimSpace(value))
	switch status {
	case StatusFailed:
		if _, ok := failureReasons[reason]; !ok {
			return fmt.Errorf("failed todo requires a valid failure statusReasonCode")
		}
	case StatusCanceled:
		if _, ok := cancellationReasons[reason]; !ok {
			return fmt.Errorf("canceled todo requires a valid cancellation statusReasonCode")
		}
	default:
		if reason != "" {
			return fmt.Errorf("todo status %q prohibits statusReasonCode", status)
		}
	}
	return nil
}

func IsRetainedTerminal(status Status) bool {
	return status == StatusFailed || status == StatusCanceled
}

func IsUnfinished(status Status) bool {
	return status != StatusCompleted
}

func ValidateOperationTransition(operation string, current Status, next Status) error {
	operation = strings.TrimSpace(operation)
	valid := false
	switch operation {
	case "start":
		valid = current == StatusPending && next == StatusInProgress || current == StatusInProgress && next == StatusInProgress
	case "done":
		valid = (current == StatusPending || current == StatusInProgress) && next == StatusCompleted || current == StatusCompleted && next == StatusCompleted
	case "fail":
		valid = (current == StatusPending || current == StatusInProgress) && next == StatusFailed
	case "cancel":
		valid = (current == StatusPending || current == StatusInProgress) && next == StatusCanceled
	case "retry":
		valid = IsRetainedTerminal(current) && next == StatusInProgress
	case "note":
		valid = current == next
	}
	if !valid {
		return fmt.Errorf("invalid todo transition %q: %s -> %s", operation, current, next)
	}
	return nil
}
