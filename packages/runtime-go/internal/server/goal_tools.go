package server

import (
	"context"
	"errors"
	"os"

	toolsideeffect "analytix.local/runtime-go/internal/adapters/outbound/toolsideeffect"
	appgoal "analytix.local/runtime-go/internal/app/goal"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func (h *runtimeServerHandler) executeRuntimeGetGoalTool(ctx context.Context, pending runtimePendingToolCall) (any, bool) {
	result := h.runtimeGoalToolService().GetGoal(pending.ThreadID)
	captureRuntimeGoalResultV1(ctx, result)
	return result.Output, result.IsError
}

func (h *runtimeServerHandler) executeRuntimeCreateGoalTool(ctx context.Context, pending runtimePendingToolCall, args map[string]any) (any, bool) {
	result := h.runtimeGoalToolService().CreateGoal(appgoal.PendingToolContextFrom(pending), args)
	captureRuntimeGoalResultV1(ctx, result)
	return result.Output, result.IsError
}

func (h *runtimeServerHandler) executeRuntimeCompleteStepTool(ctx context.Context, pending runtimePendingToolCall, args map[string]any) (any, bool) {
	prepared, ok := toolsideeffect.CompleteStep(ctx)
	if !ok {
		return map[string]any{"code": "goal_prepared_authority_invalid", "error": "complete_step prepared owner authority is unavailable"}, true
	}
	result := h.runtimeGoalToolService().ExecutePreparedCompleteStep(appgoal.PendingToolContextFrom(pending), prepared)
	if result.IsError {
		captureRuntimeGoalResultV1(ctx, result)
	}
	return result.Output, result.IsError
}

func (h *runtimeServerHandler) executeRuntimeUpdateGoalTool(ctx context.Context, pending runtimePendingToolCall, args map[string]any) (any, bool) {
	prepared, ok := toolsideeffect.UpdateGoal(ctx)
	if !ok {
		return map[string]any{"code": "goal_prepared_authority_invalid", "error": "update_goal prepared owner authority is unavailable"}, true
	}
	result := h.runtimeGoalToolService().ExecutePreparedUpdateGoal(appgoal.PendingToolContextFrom(pending), prepared)
	captureRuntimeGoalResultV1(ctx, result)
	return result.Output, result.IsError
}

func (h *runtimeServerHandler) executeRuntimeTodoOpsTool(ctx context.Context, pending runtimePendingToolCall, args map[string]any) (any, bool) {
	prepared, ok := toolsideeffect.TodoOps(ctx)
	if !ok {
		return map[string]any{"code": "todo_prepared_authority_invalid", "error": "todo_ops prepared owner authority is unavailable"}, true
	}
	result := h.runtimeGoalToolService().ExecutePreparedTodoOps(pending.ThreadID, prepared)
	if result.IsError {
		captureRuntimeGoalResultV1(ctx, result)
	}
	return result.Output, result.IsError
}

func (h *runtimeServerHandler) executeRuntimeTodoListTool(ctx context.Context, pending runtimePendingToolCall) (any, bool) {
	result := h.runtimeGoalToolService().TodoList(pending.ThreadID)
	if result.IsError {
		captureRuntimeGoalResultV1(ctx, result)
	}
	return result.Output, result.IsError
}

func (h *runtimeServerHandler) executeRuntimeTodoWriteTool(ctx context.Context, pending runtimePendingToolCall, args map[string]any) (any, bool) {
	result := h.runtimeGoalToolService().TodoWrite(pending.ThreadID, args)
	captureRuntimeGoalResultV1(ctx, result)
	return result.Output, result.IsError
}

func captureRuntimeGoalResultV1(ctx context.Context, result appgoal.ToolResult) {
	if result.IsError && result.ProtectedError != nil {
		domaintoolresult.CaptureProtectedToolErrorV1(ctx, result.ProtectedError)
	}
	if !result.IsError && result.ProtectedCapture != nil {
		domaintoolresult.CaptureProtectedToolResultV1(ctx, *result.ProtectedCapture)
	}
}

func (h *runtimeServerHandler) runtimeGoalToolService() appgoal.ToolService {
	return appgoal.ToolService{
		Store: runtimeGoalToolStore{store: h.store, driver: h.goalDriver},
		RecordGoalUpdated: func(threadID string, goal map[string]any) {
			_, _, _ = h.store.RecordEvent(appgoal.BuildUpdatedEvent(threadID, goal))
			if h.goalDriver != nil {
				h.goalDriver.Observe(threadID, goal)
			}
		},
		RecordTodosUpdated: func(threadID string, todos map[string]any) {
			_, _, _ = h.store.RecordEvent(appgoal.BuildTodosUpdatedEvent(threadID, todos))
		},
	}
}

type runtimeGoalToolStore struct {
	store  *DurableEventSessionStore
	driver *appgoal.RoundDriverV1
}

func (s runtimeGoalToolStore) GetGoal(threadID string) (map[string]any, error) {
	goal, err := s.store.GetGoal(threadID)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return goal, err
}

func (s runtimeGoalToolStore) SetGoal(threadID string, patch map[string]any) (map[string]any, error) {
	if s.driver != nil {
		return s.driver.Mutate(threadID, patch, func() (map[string]any, error) { return s.store.SetGoal(threadID, patch) })
	}
	return s.store.SetGoal(threadID, patch)
}

func (s runtimeGoalToolStore) AppendGoalEvidence(threadID string, entry map[string]any) (map[string]any, map[string]any, error) {
	return s.store.AppendGoalEvidence(threadID, entry)
}

func (s runtimeGoalToolStore) GetTodos(threadID string) (map[string]any, error) {
	todos, err := s.store.GetTodos(threadID)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return todos, err
}

func (s runtimeGoalToolStore) SetTodos(threadID string, items []any) (map[string]any, error) {
	return s.store.SetTodos(threadID, items)
}

func (s runtimeGoalToolStore) CommitTodoWriteSnapshotV1(threadID string, items []any) (appgoal.TodoWriteCommittedSnapshotV1, error) {
	return s.store.CommitTodoWriteSnapshotV1(threadID, items)
}

func (s runtimeGoalToolStore) GetThread(threadID string) (map[string]any, error) {
	thread, err := s.store.GetThread(threadID)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return thread, err
}

func (s runtimeGoalToolStore) CommitPreparedCompleteStep(request appgoal.PreparedCompleteStepCommit) (appgoal.PreparedCompleteStepCommitResult, error) {
	return s.store.CommitPreparedCompleteStep(request)
}

func (s runtimeGoalToolStore) CommitPreparedUpdateGoal(request appgoal.PreparedUpdateGoalCommit) (map[string]any, error) {
	return s.store.CommitPreparedUpdateGoal(request)
}

func (s runtimeGoalToolStore) CommitPreparedTodoOps(request appgoal.PreparedTodoOpsCommit) (map[string]any, error) {
	return s.store.CommitPreparedTodoOps(request)
}
