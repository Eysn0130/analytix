package steering

import (
	"math"
	"testing"
)

func TestVersionNumericBounds(t *testing.T) {
	for _, input := range []any{-float64(math.MinInt), math.NaN(), math.Inf(1), float64(1.5)} {
		if isExactJSONInteger(input) || exactIntField(map[string]any{"version": input}, "version") != 0 {
			t.Errorf("invalid version accepted: %v", input)
		}
	}
}
