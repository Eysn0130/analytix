package model

import (
	"errors"
	"testing"
)

func TestProjectReasoningEffortV1UsesClosedExactValues(t *testing.T) {
	for _, value := range []string{"", "auto", "off", "low", "medium", "high", "max"} {
		projected, valid := ProjectReasoningEffortV1(value)
		if !valid || projected != value || ValidateReasoningEffortV1(value) != nil {
			t.Fatalf("expected %q to be a valid exact reasoning effort, got projected=%q valid=%v", value, projected, valid)
		}
	}

	for _, value := range []string{
		" high ", "HIGH", "minimal", "xhigh", "unknown",
		"SOL_PRIVATE_REASONING_SENTINEL_7F3C",
		"<think>private chain of thought</think>",
		`{"effort":"high","reasoning":"private"}`,
	} {
		projected, valid := ProjectReasoningEffortV1(value)
		err := ValidateReasoningEffortV1(value)
		if valid || projected != "" || !errors.Is(err, ErrInvalidReasoningEffort) {
			t.Fatalf("expected malformed reasoning effort to be rejected without projection: %q", value)
		}
	}
}
