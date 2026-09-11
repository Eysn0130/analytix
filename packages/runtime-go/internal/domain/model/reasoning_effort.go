package model

import "errors"

var ErrInvalidReasoningEffort = errors.New("reasoning effort is invalid")

// ProjectReasoningEffortV1 is the single persisted and public authority for
// reasoning-effort metadata. Matching is deliberately exact: boundary code
// must not turn whitespace, casing, aliases, or attacker-controlled text into
// a valid value.
func ProjectReasoningEffortV1(raw string) (string, bool) {
	switch raw {
	case "", "auto", "off", "low", "medium", "high", "max":
		return raw, true
	default:
		return "", false
	}
}

// ValidateReasoningEffortV1 rejects malformed metadata without reflecting the
// untrusted value into logs, API errors, or durable state.
func ValidateReasoningEffortV1(raw string) error {
	if _, valid := ProjectReasoningEffortV1(raw); !valid {
		return ErrInvalidReasoningEffort
	}
	return nil
}
