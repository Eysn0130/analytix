package server

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	goalapp "analytix.local/runtime-go/internal/app/goal"
	threadapp "analytix.local/runtime-go/internal/app/thread"
)

func TestDurablePreparedCompleteStepCommitsGoalAndTodoAtomically(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{"title": "goal atomic"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	goal, err := store.SetGoal(threadID, map[string]any{"objective": "finish", "status": "active", "strictCompletion": true})
	if err != nil {
		t.Fatal(err)
	}
	todos, err := store.SetTodos(threadID, []any{map[string]any{"id": "todo_1", "content": "Run tests", "status": "in_progress"}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.CommitPreparedCompleteStep(goalapp.PreparedCompleteStepCommit{
		ThreadID: threadID, ExpectedGoalHash: goalapp.SemanticStateHashV1(goal), ExpectedTodoHash: goalapp.SemanticStateHashV1(todos),
		Entry:         map[string]any{"step": "Run tests", "evidence": []any{"verified"}, "evidenceDetails": []any{map[string]any{"kind": "manual"}}},
		NextTodoItems: []any{map[string]any{"id": "todo_1", "content": "Run tests", "status": "completed"}},
		GoalPatch:     map[string]any{"selfCheckRequired": true, "selfCheckCompleted": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(listAny(result.Goal["evidenceLedger"])) != 1 || result.Goal["selfCheckCompleted"] != true ||
		stringField(listAny(result.Todos["items"])[0].(map[string]any), "status") != "completed" {
		t.Fatalf("atomic goal/todo result mismatch: %#v", result)
	}
	_, err = store.CommitPreparedCompleteStep(goalapp.PreparedCompleteStepCommit{
		ThreadID: threadID, ExpectedGoalHash: goalapp.SemanticStateHashV1(goal), ExpectedTodoHash: goalapp.SemanticStateHashV1(todos),
		Entry: map[string]any{"step": "Run tests", "evidence": []any{"duplicate"}},
	})
	if !errors.Is(err, goalapp.ErrPreparedStateStale) {
		t.Fatalf("stale complete_step was not rejected: %v", err)
	}
	reloaded, err := store.GetGoal(threadID)
	if err != nil || len(listAny(reloaded["evidenceLedger"])) != 1 {
		t.Fatalf("stale commit partially wrote evidence: goal=%#v err=%v", reloaded, err)
	}
}

func TestDurablePreparedTodoOpsCASHasOneConcurrentWinner(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{"title": "todo cas"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	todos, err := store.SetTodos(threadID, []any{map[string]any{"id": "todo_1", "content": "One", "status": "pending"}})
	if err != nil {
		t.Fatal(err)
	}
	expected := goalapp.SemanticStateHashV1(todos)
	var winners atomic.Int32
	var stale atomic.Int32
	var group sync.WaitGroup
	for _, status := range []string{"in_progress", "completed"} {
		status := status
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := store.CommitPreparedTodoOps(goalapp.PreparedTodoOpsCommit{
				ThreadID: threadID, ExpectedTodoHash: expected,
				NextTodoItems: []any{map[string]any{"id": "todo_1", "content": "One", "status": status}},
			})
			if err == nil {
				winners.Add(1)
			} else if errors.Is(err, goalapp.ErrPreparedStateStale) {
				stale.Add(1)
			}
		}()
	}
	group.Wait()
	if winners.Load() != 1 || stale.Load() != 1 {
		t.Fatalf("prepared todo CAS winners=%d stale=%d", winners.Load(), stale.Load())
	}
}

func TestDurableTodoTerminalStateSurvivesRestartAndRejectsReplacementBypasses(t *testing.T) {
	root := t.TempDir()
	store, err := NewTempDurableEventSessionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{"title": "todo lifecycle"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	todos, err := store.SetTodos(threadID, []any{
		map[string]any{"id": "todo_1", "content": "Run tool", "status": "in_progress"},
	})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := store.CommitPreparedTodoOps(goalapp.PreparedTodoOpsCommit{
		ThreadID: threadID, ExpectedTodoHash: goalapp.SemanticStateHashV1(todos),
		NextTodoItems: []any{map[string]any{
			"id": "todo_1", "content": "Run tool", "status": "failed", "statusReasonCode": "tool_failed",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetTodos(threadID, []any{
		map[string]any{"id": "todo_1", "content": "Run tool", "status": "in_progress"},
	}); err == nil {
		t.Fatal("full replacement bypassed explicit retry")
	}
	if cleared, err := store.ClearTodos(threadID); !errors.Is(err, threadapp.ErrTodoTerminalAuditRetention) || cleared {
		t.Fatalf("terminal audit was cleared: cleared=%t err=%v", cleared, err)
	}
	reopened, err := NewTempDurableEventSessionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := reopened.GetTodos(threadID)
	if err != nil {
		t.Fatal(err)
	}
	item := listAny(restored["items"])[0].(map[string]any)
	if stringField(item, "status") != "failed" || stringField(item, "statusReasonCode") != "tool_failed" {
		t.Fatalf("restart lost terminal todo state: %#v", restored)
	}
	retried, err := reopened.CommitPreparedTodoOps(goalapp.PreparedTodoOpsCommit{
		ThreadID: threadID, ExpectedTodoHash: goalapp.SemanticStateHashV1(restored),
		NextTodoItems: []any{map[string]any{"id": "todo_1", "content": "Run tool", "status": "in_progress"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	retriedItem := listAny(retried["items"])[0].(map[string]any)
	if stringField(retriedItem, "status") != "in_progress" || stringField(retriedItem, "statusReasonCode") != "" {
		t.Fatalf("explicit retry did not clear terminal reason: %#v", retried)
	}
	if goalapp.SemanticStateHashV1(failed) == goalapp.SemanticStateHashV1(retried) {
		t.Fatal("retry did not create an auditable durable state change")
	}
}
