package server

import (
	"os"
	"time"

	goalapp "analytix.local/runtime-go/internal/app/goal"
	threadapp "analytix.local/runtime-go/internal/app/thread"
)

func (s *DurableEventSessionStore) GetGoal(threadID string) (map[string]any, error) {
	return s.readGoalTodoField(threadID, "goal")
}

func (s *DurableEventSessionStore) SetGoal(threadID string, patch map[string]any) (map[string]any, error) {
	thread, err := s.mutateGoalTodoThread(threadID, func(thread map[string]any, now string) error {
		goal, _ := thread["goal"].(map[string]any)
		todos, _ := thread["todos"].(map[string]any)
		next, err := goalapp.ApplyStatePatch(goalapp.StatePatchInput{
			ThreadID: threadID, Existing: goal, Patch: patch, Todos: todos, Now: now,
		})
		if err == nil {
			thread["goal"] = next
		}
		return err
	})
	return clonedGoalTodoField(thread, "goal"), err
}

func (s *DurableEventSessionStore) AppendGoalEvidence(threadID string, entry map[string]any) (map[string]any, map[string]any, error) {
	var record map[string]any
	thread, err := s.mutateGoalTodoThread(threadID, func(thread map[string]any, now string) error {
		goal, _ := thread["goal"].(map[string]any)
		next, appended, err := goalapp.AppendEvidence(goalapp.EvidenceAppendInput{
			ThreadID: threadID, Goal: goal, Entry: entry, Now: now,
		})
		if err == nil {
			thread["goal"], record = next, appended
		}
		return err
	})
	return clonedGoalTodoField(thread, "goal"), cloneMap(record), err
}

func (s *DurableEventSessionStore) ClearGoal(threadID string) (bool, error) {
	return s.clearGoalTodoField(threadID, "goal")
}

func (s *DurableEventSessionStore) GetTodos(threadID string) (map[string]any, error) {
	return s.readGoalTodoField(threadID, "todos")
}

func (s *DurableEventSessionStore) SetTodos(threadID string, items []any) (map[string]any, error) {
	thread, err := s.mutateGoalTodoThread(threadID, threadapp.TodoReplacementMutation(threadID, items))
	return clonedGoalTodoField(thread, "todos"), err
}

func (s *DurableEventSessionStore) ClearTodos(threadID string) (bool, error) {
	var existed bool
	_, err := s.mutateGoalTodoThread(threadID, threadapp.TodoClearMutation(&existed))
	return existed, err
}

func (s *DurableEventSessionStore) CommitPreparedCompleteStep(request goalapp.PreparedCompleteStepCommit) (goalapp.PreparedCompleteStepCommitResult, error) {
	var result goalapp.PreparedCompleteStepCommitResult
	_, err := s.mutateGoalTodoThread(request.ThreadID, func(thread map[string]any, now string) error {
		goal, _ := thread["goal"].(map[string]any)
		todos, _ := thread["todos"].(map[string]any)
		if goalapp.SemanticStateHashV1(goal) != request.ExpectedGoalHash || goalapp.SemanticStateHashV1(todos) != request.ExpectedTodoHash {
			return goalapp.ErrPreparedStateStale
		}
		nextGoal, entry, err := goalapp.AppendEvidence(goalapp.EvidenceAppendInput{
			ThreadID: request.ThreadID, Goal: goal, Entry: request.Entry, Now: now,
		})
		if err != nil {
			return err
		}
		nextTodos := todos
		if request.NextTodoItems != nil {
			nextTodos, err = threadapp.NormalizeTodos(request.ThreadID, request.NextTodoItems, now)
			if err != nil {
				return err
			}
		}
		if len(request.GoalPatch) > 0 {
			nextGoal, err = goalapp.ApplyStatePatch(goalapp.StatePatchInput{
				ThreadID: request.ThreadID, Existing: nextGoal, Patch: request.GoalPatch, Todos: nextTodos, Now: now,
			})
			if err != nil {
				return err
			}
		}
		thread["goal"] = nextGoal
		if request.NextTodoItems != nil {
			thread["todos"] = nextTodos
		}
		result = goalapp.PreparedCompleteStepCommitResult{Goal: cloneMap(nextGoal), Entry: cloneMap(entry), Todos: cloneMap(nextTodos)}
		return nil
	})
	return result, err
}

func (s *DurableEventSessionStore) CommitPreparedUpdateGoal(request goalapp.PreparedUpdateGoalCommit) (map[string]any, error) {
	thread, err := s.mutateGoalTodoThread(request.ThreadID, func(thread map[string]any, now string) error {
		goal, _ := thread["goal"].(map[string]any)
		todos, _ := thread["todos"].(map[string]any)
		if goalapp.SemanticStateHashV1(goal) != request.ExpectedGoalHash ||
			(request.ExpectedTodoHash != "" && goalapp.SemanticStateHashV1(todos) != request.ExpectedTodoHash) {
			return goalapp.ErrPreparedStateStale
		}
		next, err := goalapp.ApplyStatePatch(goalapp.StatePatchInput{
			ThreadID: request.ThreadID, Existing: goal, Patch: request.Patch, Todos: todos, Now: now,
		})
		if err == nil {
			thread["goal"] = next
		}
		return err
	})
	return clonedGoalTodoField(thread, "goal"), err
}

func (s *DurableEventSessionStore) CommitPreparedTodoOps(request goalapp.PreparedTodoOpsCommit) (map[string]any, error) {
	thread, err := s.mutateGoalTodoThread(request.ThreadID, func(thread map[string]any, now string) error {
		current, _ := thread["todos"].(map[string]any)
		if goalapp.SemanticStateHashV1(current) != request.ExpectedTodoHash {
			return goalapp.ErrPreparedStateStale
		}
		todos, err := threadapp.NormalizeTodos(request.ThreadID, request.NextTodoItems, now)
		if err == nil {
			thread["todos"] = todos
		}
		return err
	})
	return clonedGoalTodoField(thread, "todos"), err
}

func (s *DurableEventSessionStore) readGoalTodoField(threadID string, field string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	thread, err := s.readThreadNoLock(threadID)
	if err != nil || thread == nil {
		if err == nil {
			err = os.ErrNotExist
		}
		return nil, err
	}
	return clonedGoalTodoField(thread, field), nil
}

func (s *DurableEventSessionStore) clearGoalTodoField(threadID string, field string) (bool, error) {
	var existed bool
	_, err := s.mutateGoalTodoThread(threadID, func(thread map[string]any, _ string) error {
		_, existed = thread[field]
		delete(thread, field)
		return nil
	})
	return existed, err
}

func (s *DurableEventSessionStore) mutateGoalTodoThread(threadID string, mutate func(map[string]any, string) error) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	thread, err := s.readThreadNoLock(threadID)
	if err != nil {
		return nil, err
	}
	if thread == nil {
		return nil, os.ErrNotExist
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := mutate(thread, now); err != nil {
		return nil, err
	}
	thread["updatedAt"] = now
	if err := s.upsertThreadNoLock(thread, false); err != nil {
		return nil, err
	}
	return cloneMap(thread), nil
}

func clonedGoalTodoField(thread map[string]any, field string) map[string]any {
	value, _ := thread[field].(map[string]any)
	if value == nil {
		return nil
	}
	return cloneMap(value)
}
