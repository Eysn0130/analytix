package server

import (
	"encoding/json"
	"errors"
	"os"
	"time"

	goalapp "analytix.local/runtime-go/internal/app/goal"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	domaingoal "analytix.local/runtime-go/internal/domain/goal"
	"analytix.local/runtime-go/internal/domain/jsonstrict"
)

func (s *DurableEventSessionStore) GetGoal(threadID string) (map[string]any, error) {
	return s.readGoalTodoField(threadID, "goal")
}

func (s *DurableEventSessionStore) SetGoal(threadID string, patch map[string]any) (map[string]any, error) {
	thread, err := s.mutateGoalLifecycleThreadV1(threadID, func(thread map[string]any, now string) error {
		goal, _ := thread["goal"].(map[string]any)
		todos, _ := thread["todos"].(map[string]any)
		next, err := goalapp.ApplyLifecycleV1(goalapp.StatePatchInput{
			ThreadID: threadID, Existing: goal, Patch: patch, Todos: todos, Now: now,
		})
		if err == nil {
			if next == nil {
				delete(thread, "goal")
			} else {
				thread["goal"] = next
			}
			operation := stringField(patch, "operation")
			if operation == "" {
				operation = "legacy_update"
			}
			goalapp.AppendHistoryV1(thread, goal, next, operation, now)
		}
		return err
	})
	return clonedGoalTodoField(thread, "goal"), err
}

func (s *DurableEventSessionStore) mutateGoalLifecycleThreadV1(threadID string, mutate func(map[string]any, string) error) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	thread, err := s.readThreadNoLock(threadID)
	if err != nil {
		return nil, err
	}
	if thread == nil {
		return nil, os.ErrNotExist
	}
	beforeGoal, beforeHistory := goalapp.SemanticStateHashV1(thread["goal"]), goalapp.SemanticStateHashV1(thread["goalHistory"])
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := mutate(thread, now); err != nil {
		return nil, err
	}
	thread["updatedAt"] = now
	if err := s.upsertThreadNoLock(thread, false); err != nil {
		primary, readErr := s.goalPrimaryReadV1(threadID)
		if readErr == nil && goalapp.SemanticStateHashV1(primary["goal"]) == goalapp.SemanticStateHashV1(thread["goal"]) && goalapp.SemanticStateHashV1(primary["goalHistory"]) == goalapp.SemanticStateHashV1(thread["goalHistory"]) {
			return cloneMap(primary), errors.Join(goalapp.ErrGoalCommittedRecoveryV1, err)
		}
		if readErr == nil && goalapp.SemanticStateHashV1(primary["goal"]) == beforeGoal && goalapp.SemanticStateHashV1(primary["goalHistory"]) == beforeHistory {
			return nil, err
		}
		return nil, errors.Join(goalapp.ErrGoalCommitUnknownV1, err)
	}
	return cloneMap(thread), nil
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
	var existed bool
	_, err := s.mutateGoalTodoThread(threadID, func(thread map[string]any, now string) error {
		goal, _ := thread["goal"].(map[string]any)
		if _, _, _, counted := domaingoal.CountedRecordV1(goal); counted {
			return goalapp.ErrGoalRefConflictV1
		}
		existed = goal != nil
		if existed {
			delete(thread, "goal")
			goalapp.AppendHistoryV1(thread, goal, nil, "legacy_clear", now)
		}
		return nil
	})
	return existed, err
}

func (s *DurableEventSessionStore) GetTodos(threadID string) (map[string]any, error) {
	return s.readGoalTodoField(threadID, "todos")
}

func (s *DurableEventSessionStore) SetTodos(threadID string, items []any) (map[string]any, error) {
	thread, err := s.mutateGoalTodoThread(threadID, threadapp.TodoReplacementMutation(threadID, items))
	return clonedGoalTodoField(thread, "todos"), err
}

func (s *DurableEventSessionStore) CommitTodoWriteSnapshotV1(threadID string, items []any) (goalapp.TodoWriteCommittedSnapshotV1, error) {
	var before map[string]any
	committed, err := s.mutateGoalTodoThread(threadID, func(thread map[string]any, now string) error {
		before = clonedGoalTodoField(thread, "todos")
		return threadapp.TodoReplacementMutation(threadID, items)(thread, now)
	})
	// A later sidecar failure may follow a successful primary write. It is an
	// uncertain outcome, never a successful display observation or rollback.
	if err != nil {
		return goalapp.TodoWriteCommittedSnapshotV1{}, err
	}
	return goalapp.TodoWriteCommittedSnapshotV1{Before: before, After: clonedGoalTodoField(committed, "todos")}, nil
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
		goalapp.AppendHistoryV1(thread, goal, nextGoal, "evidence", now)
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
			goalapp.AppendHistoryV1(thread, goal, next, "model_update", now)
		}
		return err
	})
	return clonedGoalTodoField(thread, "goal"), err
}

// GoalPrimaryReadV1 bypasses recovery sidecars: admission witnesses must be
// established from the authoritative primary, never a fallback projection.
func (s *DurableEventSessionStore) goalPrimaryReadV1(threadID string) (map[string]any, error) {
	data, err := os.ReadFile(s.threadPath(threadID))
	if err != nil {
		return nil, err
	}
	if err := jsonstrict.Validate(data, jsonstrict.Options{RequireObject: true}); err != nil {
		return nil, errors.New("goal primary readback is ambiguous")
	}
	var thread map[string]any
	if err := json.Unmarshal(data, &thread); err != nil || thread == nil {
		return nil, errors.New("goal primary readback is invalid")
	}
	if stringField(thread, "id") != threadID {
		return nil, errors.New("goal primary route identity mismatch")
	}
	return thread, nil
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
