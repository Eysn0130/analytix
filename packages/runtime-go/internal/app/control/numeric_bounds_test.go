package control

import (
	"encoding/json"
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

func TestInterruptCountBoundaryCompatibility(t *testing.T) {
	upper := math.Trunc(math.Nextafter(-float64(math.MinInt), 0))
	for _, tc := range []struct {
		input any
		want  int
	}{
		{int64(math.MaxInt), math.MaxInt}, {upper, int(upper)}, {math.Copysign(0, -1), 0},
	} {
		if got, ok := nonNegativeInterruptCount(tc.input); !ok || got != tc.want {
			t.Errorf("count(%v) = %d, %v; want %d", tc.input, got, ok, tc.want)
		}
	}
	for _, input := range []any{nil, math.Inf(-1), math.Nextafter(0, -1), json.Number("7"), int64(-1)} {
		if _, ok := nonNegativeInterruptCount(input); ok {
			t.Errorf("unsupported/negative count accepted: %v", input)
		}
	}
}
