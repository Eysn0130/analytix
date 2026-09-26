package subagent

import (
	"encoding/json"
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

func TestVersionBoundaryCompatibility(t *testing.T) {
	upper := math.Trunc(math.Nextafter(-float64(math.MinInt), 0))
	for _, tc := range []struct {
		input any
		want  int
	}{
		{math.MaxInt, math.MaxInt}, {float64(math.MinInt), math.MinInt}, {upper, int(upper)}, {math.Copysign(0, -1), 0},
	} {
		if got, ok := exactTaskJobSteerVersion(tc.input); !ok || got != tc.want {
			t.Errorf("version boundary changed: %v -> %d; want %d", tc.input, got, tc.want)
		}
	}
	for _, input := range []any{nil, math.Inf(-1), math.Nextafter(float64(math.MinInt), math.Inf(-1)), json.Number("1")} {
		if _, ok := exactTaskJobSteerVersion(input); ok {
			t.Errorf("invalid/unsupported version accepted: %v", input)
		}
	}
}
