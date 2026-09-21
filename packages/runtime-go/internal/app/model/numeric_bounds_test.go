package model

import (
	"encoding/json"
	"math"
	"strconv"
	"testing"
)

func TestOptionalIntFieldRejectsInvalidNumericInput(t *testing.T) {
	bad := []any{float64(1.5), math.NaN(), math.Inf(1), math.Inf(-1), -float64(math.MinInt), math.Nextafter(float64(math.MinInt), math.Inf(-1)), json.Number("9223372036854775808"), json.Number("-9223372036854775809"), json.Number("1e3"), json.Number("1.0"), nil}
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
		{float64(math.MinInt), math.MinInt, true},
		{math.Nextafter(float64(math.MinInt), 0), int(math.Ceil(math.Nextafter(float64(math.MinInt), 0))), strconv.IntSize == 64},
		{upper, int(upper), true},
		{math.Nextafter(-float64(math.MinInt), math.Inf(1)), 0, false},
		{math.Copysign(0, -1), 0, true},
		{json.Number("-0"), 0, true},
	} {
		if got, ok := numericAny(tc.input); ok != tc.valid || ok && got != tc.want {
			t.Errorf("integer(%v) = %d, %v; want %d, %v", tc.input, got, ok, tc.want, tc.valid)
		}
	}
	if strconv.IntSize == 64 {
		want, _ := strconv.Atoi("9007199254740993")
		if got, ok := numericAny(json.Number("9007199254740993")); !ok || got != want {
			t.Fatalf("exact JSON integer lost precision: %d, %v", got, ok)
		}
	}
}

func TestOptionalIntFieldNullAndZeroCompatibility(t *testing.T) {
	for _, primary := range []map[string]any{nil, {}, {"limit": nil}} {
		if got := OptionalIntField(primary, map[string]any{"limit": 9}, "limit"); got == nil || *got != 9 {
			t.Fatalf("absent/null primary lost fallback: %v", got)
		}
		if got := OptionalIntField(primary, nil, "limit"); got != nil {
			t.Fatalf("absent/null values became an integer: %d", *got)
		}
	}
	if got := OptionalIntField(map[string]any{"limit": math.Copysign(0, -1)}, map[string]any{"limit": 9}, "limit"); got == nil || *got != 0 {
		t.Fatalf("valid zero failed to override fallback: %v", got)
	}
}
