package loop

import (
	"encoding/json"
	"math"
	"strconv"
	"testing"
)

func TestStepLimitsRejectInvalidNumericInput(t *testing.T) {
	bad := []any{float64(1.5), math.NaN(), math.Inf(1), math.Inf(-1), -float64(math.MinInt), math.Nextafter(float64(math.MinInt), math.Inf(-1)), json.Number("9223372036854775808")}
	for _, input := range bad {
		config := LoadStepLimitConfig(map[string]any{"stepLimits": map[string]any{"defaultMaxModelSteps": input}})
		if got := ResolveModelStepLimit(config, "agent", false, nil, nil); got != DefaultModelStepLimit {
			t.Errorf("invalid input changed fallback: %v -> %d", input, got)
		}
		if got := OptionalThreadMaxModelSteps(map[string]any{"runtimeStepLimits": map[string]any{"maxModelSteps": input}}); got != nil {
			t.Errorf("invalid thread limit accepted: %v -> %d", input, *got)
		}
	}
	for _, input := range []any{int(7), int64(7), float64(7), json.Number("7")} {
		if got := OptionalThreadMaxModelSteps(map[string]any{"runtimeStepLimits": map[string]any{"maxModelSteps": input}}); got == nil || *got != 7 {
			t.Errorf("valid thread limit rejected: %v", input)
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
