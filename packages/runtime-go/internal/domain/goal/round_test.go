package goal

import (
	"encoding/json"
	"math"
	"strconv"
	"testing"
)

func TestIntegerV1Bounds(t *testing.T) {
	maximum := int64(math.MaxInt)
	if maximum > 1<<53-1 {
		maximum = 1<<53 - 1
	}
	for _, test := range []struct {
		name    string
		value   any
		minimum int
		want    int
		valid   bool
	}{
		{"int", int(7), 1, 7, true},
		{"int64", int64(7), 1, 7, true},
		{"float64", float64(7), 1, 7, true},
		{"json_integer", json.Number("7"), 1, 7, true},
		{"zero_count", 0, 0, 0, true},
		{"negative_zero_count", math.Copysign(0, -1), 0, 0, true},
		{"zero_revision", 0, 1, 0, false},
		{"negative", -1, 0, 0, false},
		{"fraction", 1.5, 1, 0, false},
		{"float32", float32(7), 1, 0, false},
		{"json_decimal", json.Number("7.0"), 1, 0, false},
		{"json_exponent", json.Number("7e1"), 1, 0, false},
		{"maximum_int64", maximum, 1, int(maximum), true},
		{"maximum_float", float64(maximum), 1, int(maximum), true},
		{"maximum_json", json.Number(strconv.FormatInt(maximum, 10)), 1, int(maximum), true},
		{"above_maximum", maximum + 1, 1, 0, false},
		{"above_maximum_json", json.Number(strconv.FormatInt(maximum+1, 10)), 1, 0, false},
		{"above_safe_integer", float64(1 << 53), 1, 0, false},
		{"host_int_upper", -float64(math.MinInt), 1, 0, false},
		{"nan", math.NaN(), 0, 0, false},
		{"positive_infinity", math.Inf(1), 0, 0, false},
		{"negative_infinity", math.Inf(-1), 0, 0, false},
		{"huge", 1e100, 0, 0, false},
		{"negative_huge", -1e100, 0, 0, false},
		{"nil", nil, 0, 0, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got, ok := IntegerV1(test.value, test.minimum); got != test.want || ok != test.valid {
				t.Fatalf("integer(%v, %d) = %d, %v; want %d, %v", test.value, test.minimum, got, ok, test.want, test.valid)
			}
		})
	}
}
