package server

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	toolsideeffect "analytix.local/runtime-go/internal/adapters/outbound/toolsideeffect"
	goalapp "analytix.local/runtime-go/internal/app/goal"
	appmodel "analytix.local/runtime-go/internal/app/model"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestDurableTodoWriteSnapshotV1ObservesSameMutationAndFailure(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{"title": "synthetic snapshot"}, workspacetest.New(t))
	if err != nil {
		t.Fatal(err)
	}
	id := stringField(thread, "id")
	first, err := store.CommitTodoWriteSnapshotV1(id, []any{map[string]any{"id": "todo-1", "content": "One", "status": "pending"}})
	if err != nil || first.Before != nil || first.After == nil {
		t.Fatal("first committed observation unavailable", err)
	}
	second, err := store.CommitTodoWriteSnapshotV1(id, []any{map[string]any{"id": "todo-1", "content": "Two", "status": "in_progress"}})
	if err != nil || stringField(listAny(second.Before["items"])[0].(map[string]any), "content") != "One" || stringField(listAny(second.After["items"])[0].(map[string]any), "content") != "Two" {
		t.Fatal("snapshot is not mutation before/after", err)
	}
	listAny(first.After["items"])[0].(map[string]any)["content"] = "forged"
	current, err := store.GetTodos(id)
	if err != nil || stringField(listAny(current["items"])[0].(map[string]any), "content") != "Two" {
		t.Fatal("snapshot borrowed primary state", err)
	}
	failed, err := store.CommitTodoWriteSnapshotV1(id, []any{map[string]any{"id": "todo-1", "content": "One", "status": "in_progress"}, map[string]any{"id": "todo-1", "content": "Two", "status": "in_progress"}})
	if err == nil || failed.Before != nil || failed.After != nil {
		t.Fatal("failed mutation returned successful observation")
	}
}

func TestDurableTodoWriteSnapshotV1PersistenceErrorIsNotSuccessfulObservation(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{"title": "synthetic persistence failure"}, workspacetest.New(t))
	if err != nil {
		t.Fatal(err)
	}
	id := stringField(thread, "id")
	if _, err := store.SetTodos(id, []any{map[string]any{"id": "todo-1", "content": "Before", "status": "pending"}}); err != nil {
		t.Fatal(err)
	}
	metadata := store.metadataPath(id)
	if err := os.Rename(metadata, metadata+".synthetic-backup"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(metadata, 0700); err != nil {
		t.Fatal(err)
	}
	events := 0
	service := goalapp.ToolService{Store: runtimeGoalToolStore{store: store}, RecordTodosUpdated: func(string, map[string]any) { events++ }}
	result := service.TodoWrite(id, map[string]any{"todos": []any{map[string]any{"id": "todo-1", "content": "After partial persistence", "status": "pending"}}})
	if !result.IsError || result.ProtectedCapture != nil || events != 0 {
		t.Fatal("persistence failure emitted success observation/event")
	}
	// Existing persistence writes primary before metadata. Prove the failure
	// can have an effect while the capture still refuses to claim success.
	body, err := os.ReadFile(store.threadPath(id))
	if err != nil {
		t.Fatal(err)
	}
	primary := map[string]any{}
	if json.Unmarshal(body, &primary) != nil {
		t.Fatal("primary unreadable")
	}
	if stringField(listAny(primary["todos"].(map[string]any)["items"])[0].(map[string]any), "content") != "After partial persistence" {
		t.Fatal("fixture did not reach post-primary persistence failure")
	}
}

func TestDurablePreparedCompleteStepCommitsGoalAndTodoAtomically(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{"title": "goal atomic"}, workspacetest.New(t))
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
	thread, err := store.CreateThread(map[string]any{"title": "todo cas"}, workspacetest.New(t))
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
	thread, err := store.CreateThread(map[string]any{"title": "todo lifecycle"}, workspacetest.New(t))
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

func TestDurableParallelTodoAdmissionCaptureOperationsAndColdReopen(t *testing.T) {
	root := t.TempDir()
	store, err := NewTempDurableEventSessionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{"title": "parallel synthetic todos"}, workspacetest.New(t))
	if err != nil {
		t.Fatal(err)
	}
	id := stringField(thread, "id")
	before, err := store.SetTodos(id, []any{
		map[string]any{"id": "first", "content": "Before", "status": "in_progress"},
		map[string]any{"id": "second", "content": "Second", "status": "in_progress"},
	})
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"todos": []any{
		map[string]any{"id": "first", "content": "After", "status": "in_progress"},
		map[string]any{"id": "second", "content": "Second", "status": "in_progress"},
		map[string]any{"id": "third", "content": "Third", "status": "pending"},
	}}
	body, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	events := 0
	service := goalapp.ToolService{Store: runtimeGoalToolStore{store: store}, RecordTodosUpdated: func(string, map[string]any) { events++ }}
	prepared, err := toolsideeffect.Prepare(context.Background(), appmodel.PendingToolCall{
		ThreadID: id, TurnID: "turn-parallel", Call: domainmodel.ToolCall{ID: "call-parallel", Name: "todo_write", Arguments: body},
		ExecutionGrant: domainsecurity.ExecutionGrant{ToolName: "todo_write"},
	}, time.Now(), toolsideeffect.Dependencies{GoalService: service})
	if err != nil || prepared.Rejection != nil || prepared.SemanticIdentity.ArgsHash == "" {
		t.Fatalf("owner admission rejected parallel request: %#v err=%v", prepared, err)
	}
	result := service.TodoWrite(id, args)
	if result.IsError || result.ProtectedCapture == nil || events != 1 {
		t.Fatal("real durable write did not produce one successful observation")
	}
	capture := result.ProtectedCapture
	if domaintoolresult.ValidateProtectedCaptureV1(*capture) != nil || capture.Todo.Baseline.Availability != "known" ||
		capture.Todo.Baseline.Todos == nil || capture.Todo.Baseline.Todos.Items[0].Content != "Before" || capture.Todo.Todos.Items[0].Content != "After" {
		t.Fatal("same-mutation before/after was not captured")
	}
	for _, list := range []domaintoolresult.ProtectedTodoListV1{*capture.Todo.Baseline.Todos, capture.Todo.Todos} {
		if list.Items[0].Status != "in_progress" || list.Items[1].Status != "in_progress" {
			t.Fatal("capture lost parallel state")
		}
	}
	if stringField(listAny(before["items"])[0].(map[string]any), "content") != "Before" {
		t.Fatal("write borrowed the predecessor")
	}
	for _, op := range []map[string]any{
		{"op": "start", "id": "third"}, {"op": "fail", "id": "first", "statusReasonCode": "tool_failed"}, {"op": "retry", "id": "first"},
	} {
		if result := service.TodoOps(id, map[string]any{"ops": []any{op}}); result.IsError {
			t.Fatalf("parallel operation failed: %#v", result.Output)
		}
	}
	if _, err := store.SetGoal(id, map[string]any{"objective": "Complete all parallel work", "status": "active"}); err != nil {
		t.Fatal(err)
	}
	pending := goalapp.PendingToolContext{ThreadID: id, TurnID: "turn-parallel", ToolCallID: "complete-parallel"}
	if result := service.CompleteStep(pending, map[string]any{"step_index": float64(3), "evidence": []any{"checked"}}); result.IsError {
		t.Fatalf("complete_step failed: %#v", result.Output)
	}
	reopened, err := NewTempDurableEventSessionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := reopened.GetTodos(id)
	if err != nil {
		t.Fatal(err)
	}
	items := listAny(restored["items"])
	for index, status := range []string{"in_progress", "in_progress", "completed"} {
		if stringField(items[index].(map[string]any), "status") != status {
			t.Fatalf("cold reopen lost parallel state: %#v", restored)
		}
	}
	if stringField(items[0].(map[string]any), "statusReasonCode") != "" {
		t.Fatal("retry retained terminal reason")
	}
	if capture.Todo.Todos.Items[2].Status != "pending" || capture.Todo.Todos.Items[0].Status != "in_progress" {
		t.Fatal("later operations changed retained capture")
	}
	service.Store = runtimeGoalToolStore{store: reopened}
	if result := service.UpdateGoal(pending, map[string]any{"status": "complete"}); !result.IsError || stringField(result.Output.(map[string]any), "code") != "goal_todos_incomplete" {
		t.Fatal("cold-reopened parallel work allowed goal closure")
	}
}
