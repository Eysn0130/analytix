package loop

import (
	"encoding/json"
	"strings"
)

const DefaultModelStepLimit = 64

type StepLimitConfig struct {
	DefaultMaxModelSteps    *int
	UserGlobalMaxModelSteps *int
	PlannerMaxModelSteps    *int
	HeadlessMaxModelSteps   *int
}

func LoadStepLimitConfig(document map[string]any) StepLimitConfig {
	var settings StepLimitConfig
	for _, raw := range StepLimitConfigMaps(document) {
		settings = MergeStepLimitConfig(settings, raw)
	}
	return settings
}

func StepLimitConfigMaps(document map[string]any) []map[string]any {
	out := []map[string]any{}
	if direct, ok := document["stepLimits"].(map[string]any); ok {
		out = append(out, direct)
	}
	if runtimeConfig, ok := document["runtime"].(map[string]any); ok {
		if nested, ok := runtimeConfig["stepLimits"].(map[string]any); ok {
			out = append(out, nested)
		}
		if tuning, ok := runtimeConfig["runtimeTuning"].(map[string]any); ok {
			if nested, ok := tuning["stepLimits"].(map[string]any); ok {
				out = append(out, nested)
			}
		}
	}
	if capabilities, ok := document["capabilities"].(map[string]any); ok {
		if nested, ok := capabilities["stepLimits"].(map[string]any); ok {
			out = append(out, nested)
		}
	}
	return out
}

func MergeStepLimitConfig(base StepLimitConfig, raw map[string]any) StepLimitConfig {
	if value := optionalNonNegativeConfigInt(raw, "defaultMaxModelSteps", "default_max_model_steps"); value != nil {
		base.DefaultMaxModelSteps = value
	}
	if value := optionalNonNegativeConfigInt(raw, "userGlobalMaxModelSteps", "user_global_max_model_steps"); value != nil {
		base.UserGlobalMaxModelSteps = value
	}
	if value := optionalNonNegativeConfigInt(raw, "plannerMaxModelSteps", "planner_max_model_steps"); value != nil {
		base.PlannerMaxModelSteps = value
	}
	if value := optionalNonNegativeConfigInt(raw, "headlessMaxModelSteps", "headless_max_model_steps"); value != nil {
		base.HeadlessMaxModelSteps = value
	}
	return base
}

func OptionalThreadMaxModelSteps(thread map[string]any) *int {
	if thread == nil {
		return nil
	}
	stepLimits, _ := thread["runtimeStepLimits"].(map[string]any)
	if stepLimits == nil {
		return nil
	}
	value, ok := numericAny(stepLimits["maxModelSteps"])
	if !ok || value <= 0 {
		return nil
	}
	normalized := value
	return &normalized
}

func ResolveModelStepLimit(config StepLimitConfig, mode string, disableUserInput bool, threadMaxModelSteps *int, turnMaxModelSteps *int) int {
	limit := boundedStepLimit(config.DefaultMaxModelSteps, DefaultModelStepLimit)
	if userGlobal, ok := positiveStepLimit(config.UserGlobalMaxModelSteps); ok {
		limit = userGlobal
	}
	if disableUserInput {
		if headless, ok := positiveStepLimit(config.HeadlessMaxModelSteps); ok {
			limit = headless
		}
	}
	if normalizeMode(mode) == "plan" {
		if planner, ok := positiveStepLimit(config.PlannerMaxModelSteps); ok {
			limit = planner
		}
	}
	limit = boundedStepLimit(threadMaxModelSteps, limit)
	limit = boundedStepLimit(turnMaxModelSteps, limit)
	return limit
}

func optionalNonNegativeConfigInt(raw map[string]any, keys ...string) *int {
	for _, key := range keys {
		value, ok := numericAny(raw[key])
		if ok && value >= 0 {
			normalized := value
			return &normalized
		}
	}
	return nil
}

func boundedStepLimit(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	if *value <= 0 {
		return fallback
	}
	return *value
}

func positiveStepLimit(value *int) (int, bool) {
	if value == nil || *value <= 0 {
		return 0, false
	}
	return *value, true
}

func normalizeMode(value string) string {
	switch strings.TrimSpace(value) {
	case "agent", "plan":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func numericAny(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed), true
		}
	}
	return 0, false
}
