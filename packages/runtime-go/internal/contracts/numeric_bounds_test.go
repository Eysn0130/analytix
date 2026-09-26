package contracts

import (
	"encoding/json"
	"math"
	"strconv"
	"testing"
)

func TestNumericSeqBounds(t *testing.T) {
	bad := []any{float64(0.5), math.NaN(), math.Inf(1), math.Inf(-1), -float64(math.MinInt), math.Nextafter(float64(math.MinInt), math.Inf(-1)), json.Number("9223372036854775808"), json.Number("-9223372036854775809"), json.Number("1e3"), json.Number("1.0"), nil}
	for _, input := range bad {
		if got, ok := NumericSeq(input); ok {
			t.Errorf("out-of-range or nonintegral seq accepted: %v -> %d", input, got)
		}
	}
	for _, input := range []any{int(7), int64(7), float64(7), json.Number("7")} {
		if got, ok := NumericSeq(input); !ok || got != 7 {
			t.Errorf("valid seq rejected: %v -> %d, %v", input, got, ok)
		}
	}
}

func TestNumericHostUpperBoundary(t *testing.T) {
	for _, input := range []any{math.MaxInt, int64(math.MaxInt), json.Number(strconv.Itoa(math.MaxInt)), math.Trunc(math.Nextafter(-float64(math.MinInt), 0))} {
		if got, ok := NumericSeq(input); !ok || got <= 0 {
			t.Errorf("valid upper boundary rejected: %v -> %d, %v", input, got, ok)
		}
	}
	if strconv.IntSize == 32 {
		for _, input := range []any{int64(2147483648), json.Number("2147483648")} {
			if _, ok := NumericSeq(input); ok {
				t.Errorf("narrowing conversion accepted: %v", input)
			}
		}
	}
}

func TestNumericBoundaryCompatibility(t *testing.T) {
	upper := math.Trunc(math.Nextafter(-float64(math.MinInt), 0))
	for _, tc := range []struct {
		input any
		want  int
		valid bool
	}{
		{math.MinInt, math.MinInt, true},
		{int64(math.MinInt), math.MinInt, true},
		{json.Number(strconv.Itoa(math.MinInt)), math.MinInt, true},
		{float64(math.MinInt), math.MinInt, false},
		{math.Nextafter(float64(math.MinInt), 0), int(math.Ceil(math.Nextafter(float64(math.MinInt), 0))), false},
		{upper, int(upper), true},
		{math.Nextafter(-float64(math.MinInt), math.Inf(1)), 0, false},
	} {
		if got, ok := NumericSeq(tc.input); ok != tc.valid || ok && got != tc.want {
			t.Errorf("integer(%v) = %d, %v; want %d, %v", tc.input, got, ok, tc.want, tc.valid)
		}
	}
	if strconv.IntSize == 64 {
		want, _ := strconv.Atoi("9007199254740993")
		if got, ok := NumericSeq(json.Number("9007199254740993")); !ok || got != want {
			t.Fatalf("exact JSON integer lost precision: %d, %v", got, ok)
		}
	}
}
