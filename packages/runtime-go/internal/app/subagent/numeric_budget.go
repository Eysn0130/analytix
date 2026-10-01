package subagent

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
)

var errInvalidBudget = errors.New("subagent budget must be an integer within the supported range")

func numericAny(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		if strconv.IntSize == 32 && (typed < math.MinInt32 || typed > math.MaxInt32) {
			return 0, false
		}
		return int(typed), true
	case float64:
		// The upper bound is exclusive: float64(MaxInt64) rounds up to 2^63.
		bound := math.Exp2(float64(strconv.IntSize - 1))
		if math.IsNaN(typed) || typed < -bound || typed >= bound || math.Trunc(typed) != typed {
			return 0, false
		}
		return int(typed), true
	case json.Number:
		return exactJSONInteger(string(typed))
	default:
		return 0, false
	}
}

// exactJSONInteger preserves integral decimal/exponent forms without a float
// round trip. Expansion is bounded to an int's decimal width, even for a huge
// exponent, so untrusted numbers cannot trigger exponent-sized allocations.
func exactJSONInteger(value string) (int, bool) {
	if value == "" || strings.TrimSpace(value) != value || (value[0] != '-' && (value[0] < '0' || value[0] > '9')) || !json.Valid([]byte(value)) {
		return 0, false
	}
	mantissa := value
	exponent := 0
	var exponentErr error
	if index := strings.IndexAny(value, "eE"); index >= 0 {
		mantissa = value[:index]
		exponent, exponentErr = strconv.Atoi(value[index+1:])
	}
	negative := strings.HasPrefix(mantissa, "-")
	mantissa = strings.TrimPrefix(mantissa, "-")
	fractionDigits := 0
	if index := strings.IndexByte(mantissa, '.'); index >= 0 {
		fractionDigits = len(mantissa) - index - 1
		mantissa = mantissa[:index] + mantissa[index+1:]
	}
	digits := strings.TrimLeft(mantissa, "0")
	if digits == "" {
		return 0, true
	}
	if exponentErr != nil {
		return 0, false
	}
	if exponent >= fractionDigits {
		zeros := exponent - fractionDigits
		if zeros > 19 || len(digits) > 19-zeros {
			return 0, false
		}
		digits += strings.Repeat("0", zeros)
	} else {
		// Check before subtracting to avoid overflow for very negative exponents.
		if exponent < fractionDigits-len(digits) {
			return 0, false
		}
		cut := len(digits) - (fractionDigits - exponent)
		if strings.Trim(digits[cut:], "0") != "" {
			return 0, false
		}
		digits = digits[:cut]
		if len(digits) > 19 {
			return 0, false
		}
	}
	if negative {
		digits = "-" + digits
	}
	parsed, err := strconv.ParseInt(digits, 10, strconv.IntSize)
	return int(parsed), err == nil
}

// budgetInteger distinguishes absent/null values from explicit invalid input.
// The first non-null alias remains authoritative.
func budgetInteger(values ...any) (int, bool, error) {
	value := firstNonNilValue(values...)
	if value == nil {
		return 0, false, nil
	}
	parsed, ok := numericAny(value)
	if !ok {
		return 0, true, errInvalidBudget
	}
	return parsed, true, nil
}

func validTimeBudgetMS(value int) bool {
	return value <= 0 || int64(value) <= math.MaxInt64/int64(time.Millisecond)
}

func budgetSecondsToMS(seconds int) (int, error) {
	if seconds <= 0 {
		// Nonpositive budgets remain disabled. Preserve their ordinary stored
		// value without allowing an extreme negative value to wrap positive.
		if seconds < math.MinInt/1000 {
			return seconds, nil
		}
		return seconds * 1000, nil
	}
	if seconds > math.MaxInt/1000 || int64(seconds) > math.MaxInt64/int64(time.Second) {
		return 0, errInvalidBudget
	}
	return seconds * 1000, nil
}
