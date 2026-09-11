package evidence

import (
	"errors"
	"reflect"
	"strings"
)

func ValidateCoverageDescriptor(value map[string]any) error {
	return validateCoverageDescriptor(value, 0)
}

func validateCoverageDescriptor(value map[string]any, depth int) error {
	if depth > 12 {
		return errors.New("coverage descriptor exceeds maximum depth")
	}
	for key, child := range value {
		switch key {
		case "partial", "partialCoverage", "partial_coverage", "truncated", "hasMore", "has_more", "support_payload_truncated", "coverageConflict",
			"complete", "coverageComplete", "coverage_complete", "paginationComplete", "pagination_complete", "source_coverage_complete":
			if _, ok := child.(bool); !ok {
				return errors.New("coverage boolean marker has invalid type")
			}
		case "coverageStatus", "coverage_status", "paginationCompleteness", "pagination_completeness", "status":
			text, ok := child.(string)
			if !ok || coverageStatusRank(text) == 0 {
				return errors.New("coverage status marker is invalid")
			}
		default:
			return errors.New("coverage descriptor contains unknown field")
		}
	}
	return nil
}

// MergeConservativeCoverage combines untrusted coverage descriptors without
// allowing a later complete/false assertion to erase an earlier incomplete
// signal. Conflicting unknown fields are retained from the first channel and
// marked as a host-observed coverage conflict.
func MergeConservativeCoverage(current map[string]any, next map[string]any) map[string]any {
	out := cloneCoverageMap(current)
	if out == nil {
		out = map[string]any{}
	}
	for key, value := range next {
		existing, exists := out[key]
		if !exists {
			out[key] = cloneCoverageValue(value)
			continue
		}
		merged, compatible := mergeCoverageValue(key, existing, value)
		out[key] = merged
		if !compatible {
			out["coverageConflict"] = true
		}
	}
	return out
}

func mergeCoverageValue(key string, current, next any) (any, bool) {
	if reflect.DeepEqual(current, next) {
		return cloneCoverageValue(current), true
	}
	if left, ok := current.(map[string]any); ok {
		if right, ok := next.(map[string]any); ok {
			return MergeConservativeCoverage(left, right), true
		}
	}
	if left, ok := current.(bool); ok {
		if right, ok := next.(bool); ok {
			switch key {
			case "partial", "partialCoverage", "partial_coverage", "truncated", "hasMore", "has_more", "support_payload_truncated", "coverageConflict":
				return left || right, true
			case "complete", "coverageComplete", "coverage_complete", "paginationComplete", "pagination_complete", "source_coverage_complete":
				return left && right, true
			}
		}
	}
	if left, ok := current.(string); ok && coverageStatusKey(key) {
		if right, ok := next.(string); ok {
			if coverageStatusRank(right) > coverageStatusRank(left) {
				return right, true
			}
			return left, true
		}
	}
	return cloneCoverageValue(current), false
}

func coverageStatusKey(key string) bool {
	switch key {
	case "coverageStatus", "coverage_status", "paginationCompleteness", "pagination_completeness", "status":
		return true
	default:
		return false
	}
}

func coverageStatusRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "partial", "incomplete", "truncated":
		return 3
	case "unknown":
		return 2
	case "complete", "full":
		return 1
	default:
		return 0
	}
}

func cloneCoverageMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, child := range value {
		out[key] = cloneCoverageValue(child)
	}
	return out
}

func cloneCoverageValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneCoverageMap(typed)
	case []any:
		out := make([]any, len(typed))
		for index, child := range typed {
			out[index] = cloneCoverageValue(child)
		}
		return out
	default:
		return value
	}
}
