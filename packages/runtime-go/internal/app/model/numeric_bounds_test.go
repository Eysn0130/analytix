package model

import (
	"encoding/json"
	"math"
	"strconv"
	"testing"
)

func TestOptionalIntFieldRejectsInvalidNumericInput(t *testing.T) {
	bad := []any{float64(1.5), math.NaN(), math.Inf(1), math.Inf(-1), -float64(math.MinInt), math.Nextafter(float64(math.MinInt), math.Inf(-1)), json.Number("9223372036854775808")}
	for _, input := range bad {
		if got := OptionalIntField(map[string]any{"maxModelSteps": input}, map[string]any{"maxModelSteps": 9}, "maxModelSteps"); got == nil || *got != 9 {
			t.Errorf("invalid primary changed fallback: %v -> %v", input, got)
		}
	}
	for _, input := range []any{int(7), int64(7), float64(7), json.Number("7")} {
		if got := OptionalIntField(map[string]any{"maxModelSteps": input}, nil, "maxModelSteps"); got == nil || *got != 7 {
			t.Errorf("valid primary rejected: %v", input)
		}
	}
}

func TestNumericHostUpperBoundary(t *testing.T) {
	for _, input := range []any{math.MaxInt, int64(math.MaxInt), json.Number(strconv.Itoa(math.MaxInt)), math.Trunc(math.Nextafter(-float64(math.MinInt), 0))} {
		if got, ok := numericAny(input); !ok || got <= 0 {
			t.Errorf("valid upper boundary rejected: %v -> %d, %v", input, got, ok)
		}
	}
	if strconv.IntSize == 32 {
		for _, input := range []any{int64(2147483648), json.Number("2147483648")} {
			if _, ok := numericAny(input); ok {
				t.Errorf("narrowing conversion accepted: %v", input)
			}
		}
	}
}
