package loop

import "testing"

func ptr(value int) *int {
	return &value
}

func TestStepLimitConfigMapsPreservePrecedenceOrder(t *testing.T) {
	document := map[string]any{
		"stepLimits": map[string]any{"defaultMaxModelSteps": 3},
		"runtime": map[string]any{
			"stepLimits": map[string]any{"defaultMaxModelSteps": 5},
			"runtimeTuning": map[string]any{
				"stepLimits": map[string]any{"plannerMaxModelSteps": 7},
			},
		},
		"capabilities": map[string]any{
			"stepLimits": map[string]any{"headlessMaxModelSteps": 11},
		},
	}

	var config StepLimitConfig
	for _, raw := range StepLimitConfigMaps(document) {
		config = MergeStepLimitConfig(config, raw)
	}

	if config.DefaultMaxModelSteps == nil || *config.DefaultMaxModelSteps != 5 {
		t.Fatalf("runtime.stepLimits should override top-level stepLimits: %#v", config.DefaultMaxModelSteps)
	}
	if config.PlannerMaxModelSteps == nil || *config.PlannerMaxModelSteps != 7 {
		t.Fatalf("runtime.runtimeTuning.stepLimits should merge planner limit: %#v", config.PlannerMaxModelSteps)
	}
	if config.HeadlessMaxModelSteps == nil || *config.HeadlessMaxModelSteps != 11 {
		t.Fatalf("capabilities.stepLimits should merge headless limit: %#v", config.HeadlessMaxModelSteps)
	}
}

func TestResolveModelStepLimitPrecedenceAndZeroFallsBackToBoundedLimit(t *testing.T) {
	config := StepLimitConfig{
		DefaultMaxModelSteps:    ptr(8),
		UserGlobalMaxModelSteps: ptr(6),
		PlannerMaxModelSteps:    ptr(4),
		HeadlessMaxModelSteps:   ptr(2),
	}

	if got := ResolveModelStepLimit(config, "agent", false, nil, nil); got != 6 {
		t.Fatalf("global user limit should override default, got %d", got)
	}
	if got := ResolveModelStepLimit(config, "plan", false, nil, nil); got != 4 {
		t.Fatalf("planner limit should override global limit for plan mode, got %d", got)
	}
	if got := ResolveModelStepLimit(config, "agent", true, nil, nil); got != 2 {
		t.Fatalf("headless limit should override global limit when user input is disabled, got %d", got)
	}
	if got := ResolveModelStepLimit(config, "plan", true, ptr(3), nil); got != 3 {
		t.Fatalf("thread limit should override mode-specific limit, got %d", got)
	}
	if got := ResolveModelStepLimit(config, "plan", true, ptr(3), ptr(0)); got != 3 {
		t.Fatalf("turn maxModelSteps=0 must not disable the bounded thread limit, got %d", got)
	}
}

func TestOptionalThreadMaxModelSteps(t *testing.T) {
	thread := map[string]any{"runtimeStepLimits": map[string]any{"maxModelSteps": float64(9)}}
	value := OptionalThreadMaxModelSteps(thread)
	if value == nil || *value != 9 {
		t.Fatalf("expected thread max model steps 9, got %#v", value)
	}
	if value := OptionalThreadMaxModelSteps(map[string]any{"runtimeStepLimits": map[string]any{"maxModelSteps": -1}}); value != nil {
		t.Fatalf("negative thread max model steps should be ignored: %#v", value)
	}
	if value := OptionalThreadMaxModelSteps(map[string]any{"runtimeStepLimits": map[string]any{"maxModelSteps": 0}}); value != nil {
		t.Fatalf("zero thread max model steps must not create an unbounded loop: %#v", value)
	}
}
