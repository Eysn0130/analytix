package turn

import (
	"encoding/json"
	"math"
	"testing"
)

func TestTurnMetadataRejectsOutOfRangeNumbers(t *testing.T) {
	for _, input := range []any{-float64(math.MinInt), math.Inf(1), math.NaN(), float64(0.5)} {
		if got, ok := exactNonNegativeInterruptCountV1(input); ok {
			t.Errorf("invalid interrupt count accepted: %v -> %d", input, got)
		}
		if got := caseCompactionNumeric(input); got != -1 {
			t.Errorf("invalid compaction number accepted: %v -> %d", input, got)
		}
	}
	for _, input := range []any{math.Exp2(63), math.Inf(1), math.NaN(), float64(0.5)} {
		if got, ok := positiveCaseCompactionInteger(input); ok {
			t.Errorf("invalid replacedTokens accepted: %v -> %d", input, got)
		}
	}
	for _, input := range []any{int(7), int64(7), float64(7), json.Number("7")} {
		if got, ok := exactNonNegativeInterruptCountV1(input); !ok || got != 7 {
			t.Errorf("valid count rejected: %v", input)
		}
		if got := caseCompactionNumeric(input); got != 7 {
			t.Errorf("valid compaction number rejected: %v", input)
		}
		if got, ok := positiveCaseCompactionInteger(input); !ok || got != 7 {
			t.Errorf("valid replacedTokens rejected: %v", input)
		}
	}
}
