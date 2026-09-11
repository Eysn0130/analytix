package subagent

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

type RunResult struct {
	Output  map[string]any
	IsError bool
}

type ParallelRunResult struct {
	Output  map[string]any
	Results []RunResult
	IsError bool
}

type TaskRunner func(context.Context, TaskRequest) RunResult

func RunParallelTasks(ctx context.Context, tasks []ParallelTaskRequest, now time.Time, runner TaskRunner) ParallelRunResult {
	if runner == nil {
		return ParallelRunResult{Output: ValidationErrorResponse("parallel_tasks runner is required"), IsError: true}
	}
	groupID := fmt.Sprintf("parallel-%d", now.UTC().UnixNano())
	results := make([]RunResult, len(tasks))
	completed := map[string]bool{}
	completedSummaries := map[string]string{}
	remaining := map[string]ParallelTaskRequest{}
	for _, task := range tasks {
		task.Request.ParallelGroupID = groupID
		task.Request.ParallelIndex = task.Index + 1
		remaining[task.ID] = task
	}
	for len(remaining) > 0 {
		ready := []ParallelTaskRequest{}
		for _, task := range remaining {
			if ParallelDependenciesComplete(task.Request.DependsOn, completed) {
				ready = append(ready, task)
			}
		}
		if len(ready) == 0 {
			return ParallelRunResult{Output: ValidationErrorResponse("parallel_tasks dependency cycle detected"), IsError: true}
		}
		sort.SliceStable(ready, func(i, j int) bool { return ready[i].Index < ready[j].Index })
		var wg sync.WaitGroup
		for _, task := range ready {
			delete(remaining, task.ID)
			request := task.Request
			if dependencyContext := ParallelDependencyContext(request.DependsOn, completedSummaries); dependencyContext != "" {
				originalPrompt := request.Prompt
				request.Prompt = request.Prompt + "\n\nDependency results:\n" + dependencyContext
				request.parallelDependencyPrompt = &parallelDependencyPromptV1{original: originalPrompt, expanded: request.Prompt}
			}
			acquireQueued := make(chan struct{})
			request.AcquireQueued = acquireQueued
			index := task.Index
			wg.Add(1)
			go func() {
				defer wg.Done()
				result := runner(ctx, request)
				result.Output = PublicChildOutputProjection(result.Output)
				results[index] = result
			}()
			select {
			case <-acquireQueued:
			case <-ctx.Done():
			}
		}
		wg.Wait()
		for _, task := range ready {
			completed[task.ID] = true
			completedSummaries[task.ID] = DependencySummary(results[task.Index].Output)
		}
	}
	tasksOutput := make([]any, 0, len(results))
	isError := false
	errorFlags := make([]bool, 0, len(results))
	for _, result := range results {
		tasksOutput = append(tasksOutput, result.Output)
		isError = isError || result.IsError
		errorFlags = append(errorFlags, result.IsError)
	}
	return ParallelRunResult{
		Output: map[string]any{
			"kind":                         "parallel_tasks",
			"status":                       ParallelStatusFromErrors(errorFlags),
			"parallelGroupId":              groupID,
			"taskCount":                    float64(len(results)),
			"tasks":                        tasksOutput,
			"topLevelSubagentRouteExposed": false,
		},
		Results: results,
		IsError: isError,
	}
}
