package subagent

import (
	"math"
	"testing"
)

func TestVersionNumericBounds(t *testing.T) {
	for _, input := range []any{-float64(math.MinInt), math.NaN(), math.Inf(1), float64(1.5)} {
		if _, ok := exactTaskJobSteerVersion(input); ok {
			t.Errorf("invalid version accepted: %v", input)
		}
	}
}
