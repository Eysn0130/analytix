package subagent

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"testing"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

func TestNumericAnyRequiresExactRepresentableInteger(t *testing.T) {
	for _, value := range []any{
		1.5, math.NaN(), math.Inf(1), math.Inf(-1),
		math.Exp2(float64(strconv.IntSize - 1)),
		math.Nextafter(-math.Exp2(float64(strconv.IntSize-1)), math.Inf(-1)),
		json.Number("1.5"), json.Number("1.00000000000000001"),
		json.Number("9223372036854775808"), json.Number("-9223372036854775809"),
		json.Number("1e1000000000"), json.Number("1e-1000000000"),
		json.Number("1e-9223372036854775808"), json.Number("1e9223372036854775807"),
		json.Number("01"), json.Number(" 1"), "12",
	} {
		if _, ok := numericAny(value); ok {
			t.Errorf("accepted invalid integer %v (%T)", value, value)
		}
	}
	for _, tc := range []struct {
		value any
		want  int
	}{
		{int(12), 12}, {int64(-12), -12}, {float64(12), 12},
		{-math.Exp2(float64(strconv.IntSize - 1)), math.MinInt},
		{json.Number("12.0"), 12}, {json.Number("1e3"), 1000},
		{json.Number("1200e-2"), 12}, {json.Number("0.00120e4"), 12},
		{json.Number("120000000000000000000000000000000e-31"), 12},
		{json.Number("-0e999999999999999999999999"), 0},
		{json.Number(strconv.Itoa(math.MaxInt)), math.MaxInt},
		{json.Number(strconv.Itoa(math.MinInt)), math.MinInt},
	} {
		if got, ok := numericAny(tc.value); !ok || got != tc.want {
			t.Errorf("numericAny(%v) = %d, %v; want %d, true", tc.value, got, ok, tc.want)
		}
	}
	if strconv.IntSize == 64 {
		want, _ := strconv.Atoi("9007199254740993")
		if got, ok := numericAny(json.Number("9007199254740993")); !ok || got != want {
			t.Errorf("integer lost precision: got %d, %v; want %d", got, ok, want)
		}
	}
}

func TestTaskBudgetRejectsInvalidExplicitValues(t *testing.T) {
	for _, key := range []string{"max_steps", "maxSteps", "token_budget", "tokenBudget", "time_budget_ms", "timeBudgetMs", "time_budget_seconds", "timeBudgetSeconds"} {
		for _, value := range []any{1.5, json.Number("1.00000000000000001"), json.Number("9223372036854775808"), "invalid"} {
			if _, err := TaskRequestFromArgs("task", map[string]any{"prompt": "synthetic", key: value}); err == nil {
				t.Errorf("%s accepted invalid budget %v", key, value)
			}
		}
	}
	for _, key := range []string{"time_budget_ms", "time_budget_seconds"} {
		if _, err := TaskRequestFromArgs("task", map[string]any{"prompt": "synthetic", key: int(^uint(0) >> 1)}); err == nil && strconv.IntSize == 64 {
			t.Errorf("%s accepted duration overflow", key)
		}
	}
	if _, err := TaskRequestFromArgs("task", map[string]any{"prompt": "synthetic", "max_steps": "invalid", "maxSteps": 2}); err == nil {
		t.Fatal("invalid preferred alias silently fell back")
	}
}

func TestTaskBudgetPreservesUnsetAliasAndNonpositiveSemantics(t *testing.T) {
	request, err := TaskRequestFromArgs("task", map[string]any{"prompt": "synthetic", "max_steps": nil, "maxSteps": -1, "token_budget": 0, "tokenBudget": 5, "time_budget_ms": 0, "time_budget_seconds": json.Number("2e0")})
	if err != nil || !request.MaxStepsSet || request.MaxSteps != -1 || request.TokenBudgetSet || !request.TimeBudgetMSSet || request.TimeBudgetMS != 2000 {
		t.Fatalf("compatibility changed: %+v, %v", request, err)
	}
	request, err = TaskRequestFromArgs("task", map[string]any{"prompt": "synthetic", "max_steps": 0, "time_budget_ms": -1, "time_budget_seconds": -1})
	if err != nil || !request.MaxStepsSet || request.MaxSteps != 0 || request.TimeBudgetMSSet {
		t.Fatalf("nonpositive semantics changed: %+v, %v", request, err)
	}
}

func TestProfileBudgetRejectsInvalidExplicitValues(t *testing.T) {
	for _, key := range []string{"maxSteps", "max_steps", "tokenBudget", "token_budget", "timeBudgetMs", "time_budget_ms", "timeBudgetSeconds", "time_budget_seconds"} {
		for _, value := range []any{1.5, "invalid", json.Number("9223372036854775808")} {
			_, err := LoadProfileSettings(map[string]any{"subagents": map[string]any{"profiles": map[string]any{"synthetic": map[string]any{key: value}}}}, true)
			if err == nil {
				t.Errorf("profile %s accepted %v", key, value)
			}
		}
	}
	for _, key := range []string{"maxParallel", "max_parallel", "maxChildRuns", "max_child_runs"} {
		if _, err := LoadProfileSettings(map[string]any{"subagents": map[string]any{key: 1.5}}, true); err == nil {
			t.Errorf("settings %s accepted fractional budget", key)
		}
	}
	settings, err := LoadProfileSettings(map[string]any{"subagents": map[string]any{"profiles": map[string]any{"synthetic": map[string]any{"maxSteps": 0, "tokenBudget": -1, "timeBudgetMs": 0, "timeBudgetSeconds": 2}}}}, true)
	profile := settings.Profiles["synthetic"]
	if err != nil || profile.MaxStepsSet || profile.TokenBudgetSet || profile.TimeBudgetMSSet {
		t.Fatalf("profile nonpositive semantics changed: %+v, %v", profile, err)
	}
	settings = MergeProfileSettings(DefaultProfileSettings(), map[string]any{"profiles": map[string]any{"synthetic": map[string]any{"maxSteps": "invalid", "max_steps": 2}}}, "config")
	if _, err := ApplyProfile(TaskRequest{Prompt: "synthetic", ProfileName: "synthetic"}, settings); err == nil {
		t.Fatal("profile application silently accepted an invalid preferred alias")
	}
}

func TestTimeBudgetDurationBoundaries(t *testing.T) {
	maxMS := min(int64(math.MaxInt), int64(math.MaxInt64)/int64(time.Millisecond))
	maxSeconds := min(int64(math.MaxInt)/1000, int64(math.MaxInt64)/int64(time.Second))
	for _, tc := range []struct {
		requestKey, profileKey string
		value                  int64
		wantMS                 int
		valid                  bool
	}{
		{"time_budget_ms", "timeBudgetMs", maxMS, int(maxMS), true},
		{"time_budget_ms", "timeBudgetMs", maxMS + 1, 0, false},
		{"time_budget_seconds", "timeBudgetSeconds", maxSeconds, int(maxSeconds * 1000), true},
		{"time_budget_seconds", "timeBudgetSeconds", maxSeconds + 1, 0, false},
	} {
		value := json.Number(strconv.FormatInt(tc.value, 10))
		request, err := TaskRequestFromArgs("task", map[string]any{"prompt": "synthetic", tc.requestKey: value})
		if (err == nil) != tc.valid || tc.valid && (!request.TimeBudgetMSSet || request.TimeBudgetMS != tc.wantMS) {
			t.Errorf("request %s=%s: got %d, set=%v, err=%v", tc.requestKey, value, request.TimeBudgetMS, request.TimeBudgetMSSet, err)
		}
		settings, err := LoadProfileSettings(map[string]any{"subagents": map[string]any{"profiles": map[string]any{"synthetic": map[string]any{tc.profileKey: value}}}}, true)
		profile := settings.Profiles["synthetic"]
		if (err == nil) != tc.valid || tc.valid && (!profile.TimeBudgetMSSet || profile.TimeBudgetMS != tc.wantMS) {
			t.Errorf("profile %s=%s: got %d, set=%v, err=%v", tc.profileKey, value, profile.TimeBudgetMS, profile.TimeBudgetMSSet, err)
		}
	}
	for _, value := range []int{0, -1, math.MinInt} {
		settings, err := LoadProfileSettings(map[string]any{"subagents": map[string]any{"maxParallel": value, "maxChildRuns": value, "profiles": map[string]any{"synthetic": map[string]any{"timeBudgetSeconds": value}}}}, true)
		if err != nil || settings.MaxParallel != value || settings.MaxChildRuns != value || settings.Profiles["synthetic"].TimeBudgetMSSet {
			t.Errorf("nonpositive profile budget changed: %d, err=%v", value, err)
		}
	}
}

func TestTaskJobWaitClampsSecondsBeforeMultiplication(t *testing.T) {
	for _, tc := range []struct{ seconds, want int }{{int(^uint(0) >> 1), DefaultTaskJobWaitTimeoutMS}, {-int(^uint(0)>>1) - 1, 0}, {0, 0}, {1, 1000}} {
		request := TaskJobWaitRequestFromArgs(map[string]any{"timeout_seconds": tc.seconds})
		if request.TimeoutMS != tc.want {
			t.Errorf("seconds %d produced %d; want %d", tc.seconds, request.TimeoutMS, tc.want)
		}
	}
}

func TestCompleteTaskRejectsDurationOverflowBeforeDriverEffects(t *testing.T) {
	if strconv.IntSize != 64 {
		t.Skip("int milliseconds cannot overflow time.Duration on a 32-bit host")
	}
	record := domainjob.Record{ID: "child-budget", ChildThreadID: "thread-child", Status: string(domainjob.StatusRunning), Effort: "medium"}
	driver := newCompletionDriverStub(record)
	result := CompleteTask(context.Background(), CompleteTaskInput{
		Request: TaskRequest{Prompt: "synthetic", TimeBudgetMS: int(^uint(0) >> 1), TimeBudgetMSSet: true},
		Record:  record, Effort: "medium", Driver: driver, StartAuthority: NewBackgroundJobStartBarrier(),
	})
	if !result.IsError || len(driver.startRequests) != 0 || driver.updates != 0 || len(driver.progress) != 0 {
		t.Fatalf("overflow reached driver: error=%v starts=%d updates=%d progress=%d", result.IsError, len(driver.startRequests), driver.updates, len(driver.progress))
	}
}
