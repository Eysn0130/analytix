package event

import (
	"encoding/json"
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

func TestVersionBoundaryCompatibility(t *testing.T) {
	upper := math.Trunc(math.Nextafter(-float64(math.MinInt), 0))
	for _, tc := range []struct {
		input any
		want  int
	}{
		{math.MaxInt, math.MaxInt}, {float64(math.MinInt), math.MinInt}, {upper, int(upper)}, {math.Copysign(0, -1), 0},
	} {
		if got := generalCompactionNumeric(tc.input); got != tc.want {
			t.Errorf("version boundary changed: %v -> %d; want %d", tc.input, got, tc.want)
		}
	}
	for _, input := range []any{nil, math.Inf(-1), math.Nextafter(float64(math.MinInt), math.Inf(-1)), json.Number("1")} {
		if got := generalCompactionNumeric(input); got != -1 {
			t.Errorf("invalid/unsupported version accepted: %v", input)
		}
	}
}
