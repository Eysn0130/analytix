package job

import (
	"errors"
	"strings"
)

const (
	StatusCanceled        Status = "canceled"
	StatusTimeout         Status = "timeout"
	PublicStatusUnknownV1        = "unknown"
)

// ValidateStatusV1 is the sole durable child/job lifecycle allowlist. Status
// text is control data, never provider-authored display text.
func ValidateStatusV1(value string) error {
	if !ValidStatusV1(value) {
		return errors.New("job status is outside the closed lifecycle allowlist")
	}
	return nil
}

func ValidStatusV1(value string) bool {
	switch Status(strings.TrimSpace(value)) {
	case StatusQueued, StatusRunning, StatusPauseRequested, StatusPaused,
		StatusResumeRequested, StatusResuming, StatusCompleted, StatusFailed,
		StatusAborted, StatusInterrupted, StatusKilled, StatusCanceled, StatusTimeout:
		return true
	default:
		return false
	}
}

// PublicStatusV1 maps untrusted or corrupt status text to one fixed public
// value instead of reflecting it into event stages, messages, or history.
func PublicStatusV1(value string) string {
	value = strings.TrimSpace(value)
	if !ValidStatusV1(value) {
		return PublicStatusUnknownV1
	}
	return value
}

func PublicKindV1(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "background-shell", "subagent", "child-run", "parallel-child-run", "task", "parallel_task", "bash":
		return value
	default:
		return "unknown"
	}
}

func TerminalStatusV1(value string) bool {
	switch Status(strings.TrimSpace(value)) {
	case StatusCompleted, StatusFailed, StatusAborted, StatusInterrupted,
		StatusKilled, StatusCanceled, StatusTimeout:
		return true
	default:
		return false
	}
}

func ExternallyStoppedStatusV1(value string) bool {
	switch Status(strings.TrimSpace(value)) {
	case StatusAborted, StatusInterrupted, StatusKilled, StatusCanceled:
		return true
	default:
		return false
	}
}
