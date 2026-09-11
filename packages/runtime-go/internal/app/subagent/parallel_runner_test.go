package subagent

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRunParallelTasksExecutesDependencyWavesAndAggregates(t *testing.T) {
	tasks := []ParallelTaskRequest{
		{Index: 0, ID: "a", Request: TaskRequest{ID: "a", Prompt: "alpha"}},
		{Index: 1, ID: "b", Request: TaskRequest{ID: "b", Prompt: "beta", DependsOn: []string{"a"}}},
	}
	var mu sync.Mutex
	seen := []string{}
	result := RunParallelTasks(context.Background(), tasks, time.Unix(0, 42), func(_ context.Context, request TaskRequest) RunResult {
		request.AcquireQueued <- struct{}{}
		mu.Lock()
		seen = append(seen, request.ID+":"+request.Prompt)
		mu.Unlock()
		return RunResult{Output: map[string]any{"summary": "summary-" + request.ID}}
	})
	if result.IsError || result.Output["status"] != "completed" || result.Output["parallelGroupId"] != "parallel-42" || result.Output["taskCount"] != float64(2) {
		t.Fatalf("parallel result mismatch: %#v", result)
	}
	if len(result.Results) != 2 || stringValue(result.Results[0].Output, "summary") != "summary-a" {
		t.Fatalf("child results should stay in input order: %#v", result.Results)
	}
	if len(seen) != 2 || seen[0] != "a:alpha" || !strings.Contains(seen[1], "Dependency results:") || !strings.Contains(seen[1], "a: summary-a") {
		t.Fatalf("dependency handoff mismatch: %#v", seen)
	}
}

func TestRunParallelTasksReportsChildFailureAndCycle(t *testing.T) {
	tasks := []ParallelTaskRequest{
		{Index: 0, ID: "a", Request: TaskRequest{ID: "a", Prompt: "alpha"}},
		{Index: 1, ID: "b", Request: TaskRequest{ID: "b", Prompt: "beta"}},
	}
	result := RunParallelTasks(context.Background(), tasks, time.Unix(0, 1), func(_ context.Context, request TaskRequest) RunResult {
		request.AcquireQueued <- struct{}{}
		return RunResult{Output: map[string]any{"summary": request.ID}, IsError: request.ID == "b"}
	})
	if !result.IsError || result.Output["status"] != "failed" {
		t.Fatalf("parallel failures should aggregate: %#v", result)
	}

	cycle := []ParallelTaskRequest{
		{Index: 0, ID: "a", Request: TaskRequest{ID: "a", DependsOn: []string{"b"}}},
		{Index: 1, ID: "b", Request: TaskRequest{ID: "b", DependsOn: []string{"a"}}},
	}
	cycleResult := RunParallelTasks(context.Background(), cycle, time.Unix(0, 1), func(context.Context, TaskRequest) RunResult {
		t.Fatal("runner should not be called for dependency cycle")
		return RunResult{}
	})
	if !cycleResult.IsError || stringValue(cycleResult.Output, "code") != "validation_error" {
		t.Fatalf("dependency cycle should validate: %#v", cycleResult)
	}
}
