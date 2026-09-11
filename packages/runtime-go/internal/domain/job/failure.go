package job

import "strings"

const (
	FailureChildExecutionFailed = "child_execution_failed"
	FailureChildInterrupted     = "child_interrupted"
	FailureChildKilled          = "child_killed"
	FailureChildAborted         = "child_aborted"
	FailureChildCanceled        = "child_canceled"
	FailureChildTimeout         = "child_timeout"
	FailureChildUnknown         = "child_unknown_failure"
)

func NormalizeFailureCode(code, status string, failureObserved bool) string {
	code = strings.TrimSpace(code)
	if code != "" && ValidFailureCode(code) {
		return code
	}
	switch strings.TrimSpace(status) {
	case string(StatusFailed):
		return FailureChildExecutionFailed
	case string(StatusInterrupted):
		return FailureChildInterrupted
	case string(StatusKilled):
		return FailureChildKilled
	case string(StatusAborted):
		return FailureChildAborted
	case string(StatusCanceled):
		return FailureChildCanceled
	case string(StatusTimeout):
		return FailureChildTimeout
	default:
		if failureObserved {
			return FailureChildUnknown
		}
		return ""
	}
}

func ValidFailureCode(code string) bool {
	switch strings.TrimSpace(code) {
	case "", FailureChildExecutionFailed, FailureChildInterrupted, FailureChildKilled, FailureChildAborted,
		FailureChildCanceled, FailureChildTimeout, FailureChildUnknown:
		return true
	default:
		return false
	}
}
