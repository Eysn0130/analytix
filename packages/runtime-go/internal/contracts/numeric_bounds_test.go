package contracts

import (
	"encoding/json"
	"math"
	"strconv"
	"testing"
)

func TestNumericSeqBounds(t *testing.T) {
	bad := []any{float64(0.5), math.NaN(), math.Inf(1), math.Inf(-1), -float64(math.MinInt), math.Nextafter(float64(math.MinInt), math.Inf(-1)), json.Number("9223372036854775808")}
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
