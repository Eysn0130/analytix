package turn

import (
	"encoding/json"
	"math"
	"strconv"
	"testing"
)

func TestTurnMetadataRejectsOutOfRangeNumbers(t *testing.T) {
	for _, input := range []any{-float64(math.MinInt), math.Inf(1), math.NaN(), float64(0.5)} {
		if got, ok := exactNonNegativeInterruptCountV1(input); ok {
			t.Errorf("invalid interrupt count accepted: %v -> %d", input, got)
		}
		if got := caseCompactionNumeric(input); got != -1 {
			t.Errorf("invalid compaction number accepted: %v -> %d", input, got)
		}
	}
	for _, input := range []any{math.Exp2(63), math.Inf(1), math.NaN(), float64(0.5)} {
		if got, ok := positiveCaseCompactionInteger(input); ok {
			t.Errorf("invalid replacedTokens accepted: %v -> %d", input, got)
		}
	}
	for _, input := range []any{int(7), int64(7), float64(7), json.Number("7")} {
		if got, ok := exactNonNegativeInterruptCountV1(input); !ok || got != 7 {
			t.Errorf("valid count rejected: %v", input)
		}
		if got := caseCompactionNumeric(input); got != 7 {
			t.Errorf("valid compaction number rejected: %v", input)
		}
		if got, ok := positiveCaseCompactionInteger(input); !ok || got != 7 {
			t.Errorf("valid replacedTokens rejected: %v", input)
		}
	}
}

func TestTurnNumericBoundaryCompatibility(t *testing.T) {
	upper := math.Trunc(math.Nextafter(-float64(math.MinInt), 0))
	for _, tc := range []struct {
		input any
		want  int
	}{
		{int64(math.MaxInt), math.MaxInt}, {json.Number(strconv.Itoa(math.MaxInt)), math.MaxInt},
		{upper, int(upper)}, {math.Copysign(0, -1), 0}, {json.Number("-0"), 0},
	} {
		if got, ok := exactNonNegativeInterruptCountV1(tc.input); !ok || got != tc.want {
			t.Errorf("interrupt boundary changed: %v -> %d, %v", tc.input, got, ok)
		}
		if got := caseCompactionNumeric(tc.input); got != tc.want {
			t.Errorf("compaction boundary changed: %v -> %d", tc.input, got)
		}
	}
	for _, input := range []any{nil, math.Inf(-1), json.Number("1e3"), json.Number("1.0"), json.Number("9223372036854775808"), json.Number("-9223372036854775809")} {
		if _, ok := exactNonNegativeInterruptCountV1(input); ok {
			t.Errorf("invalid count accepted: %v", input)
		}
		if got := caseCompactionNumeric(input); got != -1 {
			t.Errorf("invalid compaction input accepted: %v", input)
		}
		if _, ok := positiveCaseCompactionInteger(input); ok {
			t.Errorf("invalid positive integer accepted: %v", input)
		}
	}
	for _, input := range []any{math.MinInt, int64(math.MinInt), float64(math.MinInt), json.Number(strconv.Itoa(math.MinInt))} {
		if got := caseCompactionNumeric(input); got != math.MinInt {
			t.Errorf("valid signed lower boundary rejected: %v -> %d", input, got)
		}
		if _, ok := exactNonNegativeInterruptCountV1(input); ok {
			t.Errorf("negative count accepted: %v", input)
		}
	}
	for _, input := range []any{0, math.Copysign(0, -1), json.Number("-0"), -1} {
		if _, ok := positiveCaseCompactionInteger(input); ok {
			t.Errorf("nonpositive replacedTokens accepted: %v", input)
		}
	}
	for _, tc := range []struct {
		input any
		want  int64
	}{
		{int64(math.MaxInt64), math.MaxInt64}, {json.Number("9223372036854775807"), math.MaxInt64},
		{math.Nextafter(math.Exp2(63), 0), 9223372036854774784}, {json.Number("9007199254740993"), 9007199254740993},
	} {
		if got, ok := positiveCaseCompactionInteger(tc.input); !ok || got != tc.want {
			t.Errorf("int64 boundary changed: %v -> %d, %v", tc.input, got, ok)
		}
	}
	for _, record := range []map[string]any{{"discard": false, "cancelled": false}, {"discard": false, "cancelled": false, "cancelledPendingGates": nil}} {
		if _, err := interruptMetadataFromRecordV1(record); err == nil {
			t.Error("absent/null interrupt count accepted")
		}
	}
}
