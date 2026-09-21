package event

import (
	"math"
	"testing"
)

func TestVersionNumericBounds(t *testing.T) {
	for _, input := range []any{-float64(math.MinInt), math.NaN(), math.Inf(1), float64(1.5)} {
		if got := generalCompactionNumeric(input); got != -1 {
			t.Errorf("invalid version accepted: %v -> %d", input, got)
		}
	}
}
