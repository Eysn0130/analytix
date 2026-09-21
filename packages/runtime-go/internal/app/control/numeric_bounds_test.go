package control

import (
	"math"
	"testing"
)

func TestInterruptCountRejectsOutOfRangeFloat(t *testing.T) {
	for _, input := range []any{-float64(math.MinInt), math.Inf(1), math.NaN(), -1, float64(0.5)} {
		if got, ok := nonNegativeInterruptCount(input); ok {
			t.Errorf("invalid count accepted: %v -> %d", input, got)
		}
	}
	for _, input := range []any{int(7), int64(7), float64(7), 0} {
		if _, ok := nonNegativeInterruptCount(input); !ok {
			t.Errorf("valid count rejected: %v", input)
		}
	}
}
