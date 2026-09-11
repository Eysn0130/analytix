package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	"analytix.local/runtime-go/internal/ports"
)

func TestTaskJobServiceWaitOutputAndKill(t *testing.T) {
	const privateSentinel = "ZXQ_PRIV_7F3C9A2D_41B6"
	repo := newTaskJobRepoStub([]domainjob.Record{{
		ID:             "job-a",
		Kind:           "background-shell",
		ParentThreadID: "thread-a",
		Status:         "running",
		Background:     true,
		Output:         "abcdef " + privateSentinel,
	}, {
		ID:             "job-b",
		Kind:           "background-shell",
		ParentThreadID: "thread-a",
		Status:         "running",
		Background:     true,
		Output:         "still running",
	}})
	state := NewRuntimeState()
	state.SetOutputCursor("thread-a", "job-a", 17)
	svc := NewService(Dependencies{Jobs: repo, State: state})

	go func() {
		time.Sleep(20 * time.Millisecond)
		_, _ = repo.UpdateChildRun("job-a", domainjob.UpdateRequest{Status: "completed", Output: "abcdef done " + privateSentinel})
	}()
	waitStarted := time.Now()
	wait := svc.WaitTaskJobs("thread-a", TaskJobWaitRequest{JobIDs: []string{"job-a"}, TimeoutMS: 200}, true)
	if wait.IsError || time.Since(waitStarted) < 10*time.Millisecond {
		t.Fatalf("wait should block until the job becomes terminal: %#v", wait)
	}
	jobs, _ := wait.Response["jobs"].([]any)
	if len(jobs) != 1 || stringValue(jobs[0].(map[string]any), "status") != "completed" {
		t.Fatalf("wait result should include completed job: %#v", wait.Response)
	}

	first := svc.OutputTaskJob("thread-a", TaskJobOutputRequest{JobID: "job-a", Limit: 2, Filter: "[", Since: "invalid"}, true)
	second := svc.OutputTaskJob("thread-a", TaskJobOutputRequest{JobID: "job-a", Offset: 999, Limit: 2, Tail: true}, true)
	if first.IsError || second.IsError {
		t.Fatalf("task output should return the fixed withheld response without parsing private bytes: first=%#v second=%#v", first, second)
	}
	if first.Response["schemaVersion"] != 1 || first.Response["availability"] != "withheld" || first.Response["jobId"] != "job-a" ||
		second.Response["schemaVersion"] != 1 || second.Response["availability"] != "withheld" || second.Response["jobId"] != "job-a" {
		t.Fatalf("task output did not use the versioned withheld envelope: first=%#v second=%#v", first.Response, second.Response)
	}
	for _, response := range []map[string]any{first.Response, second.Response} {
		encoded, _ := json.Marshal(response)
		if strings.Contains(string(encoded), privateSentinel) {
			t.Fatalf("task output leaked private persisted bytes: %s", encoded)
		}
		for _, forbidden := range []string{"output", "offset", "nextOffset", "outputBytes", "truncated", "error"} {
			if _, ok := response[forbidden]; ok {
				t.Fatalf("task output exposed %q: %#v", forbidden, response)
			}
		}
	}
	if cursor := state.OutputCursor("thread-a", "job-a"); cursor != 17 {
		t.Fatalf("withheld output advanced the private cursor: %d", cursor)
	}

	canceled := false
	cancelCtx, cancel := context.WithCancel(context.Background())
	state.RegisterBackgroundJob("job-b", func() {
		canceled = true
		cancel()
		state.UnregisterBackgroundJob("job-b")
	})
	kill := svc.KillTaskJob("thread-a", TaskJobKillRequest{JobID: "job-b", Reason: "stop requested"})
	if kill.IsError || !canceled || cancelCtx.Err() == nil {
		t.Fatalf("kill should cancel registered background job: %#v canceled=%v", kill, canceled)
	}
	killed, _ := repo.LoadChildRun("job-b")
	if killed.Status != string(domainjob.StatusKilled) || killed.Error != "" || killed.FailureCode != domainjob.FailureChildKilled {
		t.Fatalf("kill should persist terminal status: %#v", killed)
	}
}

func TestTaskJobKillTimeoutDoesNotClaimKilledAndLaterCompletionConverges(t *testing.T) {
	repo := newTaskJobRepoStub([]domainjob.Record{{
		ID: "job-stuck", ParentThreadID: "thread-a", Status: string(domainjob.StatusRunning), Background: true,
	}})
	state := NewRuntimeState()
	jobCtx, cancel := context.WithCancel(context.Background())
	state.RegisterBackgroundJob("job-stuck", cancel)
	svc := NewService(Dependencies{Jobs: repo, State: state})

	result := svc.killTaskJobWithWait("thread-a", TaskJobKillRequest{JobID: "job-stuck", Reason: "stop requested"}, 20*time.Millisecond)
	if !result.IsError || result.ErrorCode != TaskJobErrorConflict || !errors.Is(result.Err, ErrBackgroundJobStopTimeout) {
		t.Fatalf("unacknowledged kill did not return a conflict blocker: %#v", result)
	}
	loaded, err := repo.LoadChildRun("job-stuck")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != string(domainjob.StatusRunning) {
		t.Fatalf("kill timeout falsely persisted a killed status: %#v", loaded)
	}
	select {
	case <-jobCtx.Done():
	default:
		t.Fatal("kill timeout did not at least deliver cancellation")
	}

	if _, err := repo.UpdateChildRun("job-stuck", domainjob.UpdateRequest{Status: string(domainjob.StatusCompleted), Output: "real terminal"}); err != nil {
		t.Fatal(err)
	}
	state.UnregisterBackgroundJob("job-stuck")
	loaded, err = repo.LoadChildRun("job-stuck")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != string(domainjob.StatusCompleted) || loaded.Output != "real terminal" {
		t.Fatalf("real completion did not converge after the timeout: %#v", loaded)
	}
}

func TestTaskJobParentThreadIDFromArgs(t *testing.T) {
	if got := TaskJobParentThreadIDFromArgs(map[string]any{"threadId": " thread-a "}); got != "thread-a" {
		t.Fatalf("threadId should parse and trim: %q", got)
	}
	if got := TaskJobParentThreadIDFromArgs(map[string]any{"parentThreadId": " parent ", "threadId": "thread-a"}); got != "parent" {
		t.Fatalf("parentThreadId should take precedence: %q", got)
	}
}

func TestTaskJobServiceRejectsForbiddenParent(t *testing.T) {
	repo := newTaskJobRepoStub([]domainjob.Record{{
		ID:             "job-a",
		ParentThreadID: "thread-a",
		Status:         "completed",
	}})
	svc := NewService(Dependencies{Jobs: repo, State: NewRuntimeState()})

	result := svc.OutputTaskJob("thread-b", TaskJobOutputRequest{JobID: "job-a"}, true)
	if !result.IsError || result.ErrorCode != TaskJobErrorNotFound {
		t.Fatalf("output should reject parent mismatch: %#v", result)
	}
}

func TestTaskJobServiceListsAndBuildsBackgroundContext(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	repo := newTaskJobRepoStub([]domainjob.Record{{
		ID:             "foreground",
		ParentThreadID: "thread-a",
		Status:         "completed",
		Output:         "ignored",
		UpdatedAt:      now.Format(time.RFC3339Nano),
	}, {
		ID:             "background",
		ParentThreadID: "thread-a",
		Kind:           "background-shell",
		Status:         "running",
		Background:     true,
		Output:         "secret running output",
		Error:          "still running",
		UpdatedAt:      now.Add(-time.Minute).Format(time.RFC3339Nano),
	}})
	svc := NewService(Dependencies{Jobs: repo})

	records, err := svc.ListTaskJobs("thread-a")
	if err != nil || len(records) != 2 {
		t.Fatalf("list task jobs mismatch: records=%#v err=%v", records, err)
	}
	backgroundOnly := true
	list := svc.ListTaskJobViews("thread-a", TaskJobListRequest{Status: "running", Background: &backgroundOnly, Limit: 1})
	if list.IsError {
		t.Fatalf("list task job views should succeed: %#v", list)
	}
	jobs, _ := list.Response["jobs"].([]any)
	if len(jobs) != 1 {
		t.Fatalf("list task job views should include one filtered job: %#v", list.Response)
	}
	jobView, _ := jobs[0].(map[string]any)
	if stringValue(jobView, "id") != "background" || jobView["outputWithheld"] != true || jobView["canReadOutput"] != false {
		t.Fatalf("list task job views should include only the closed metadata projection: %#v", list.Response)
	}
	note, filtered, err := svc.BackgroundJobContextNote("thread-a", now)
	if err != nil || len(filtered) != 1 || filtered[0].ID != "background" {
		t.Fatalf("background context should include only background records: note=%q filtered=%#v err=%v", note, filtered, err)
	}
	if !strings.Contains(note, "<background-jobs>") || !strings.Contains(note, "terminal=false") ||
		strings.Contains(note, "secret running output") || strings.Contains(note, "stalled=") || strings.Contains(note, "heartbeatStatus=") {
		t.Fatalf("background context note should be closed metadata-only for running jobs:\n%s", note)
	}
}

func TestTaskJobServiceQuarantinesChildTodoProjectionBeforeReadsOrWrites(t *testing.T) {
	record := domainjob.Record{
		ID:             "job-child-todos",
		ParentThreadID: "thread-parent",
		ParentTurnID:   "turn-parent",
		Kind:           "subagent",
		Status:         string(domainjob.StatusCompleted),
		ChildThreadID:  "thread-child",
		ChildTurnID:    "turn-child",
		Background:     true,
		Output:         `{"evidence":[{"id":"ev-child-1","summary":"child verified task"}]}`,
		ReturnFormat:   "evidence",
	}
	repo := newTaskJobRepoStub([]domainjob.Record{record})
	todos := newChildTodoStoreStub(map[string]map[string]any{
		"thread-parent": {
			"threadId": "thread-parent",
			"items": []any{
				map[string]any{"id": "parent-todo", "content": "Parent stays pending", "status": "pending"},
			},
		},
		"thread-child": {
			"threadId":  "thread-child",
			"updatedAt": "2026-07-07T00:00:00Z",
			"items": []any{
				map[string]any{"id": "child-1", "content": "Gather facts", "status": "completed", "evidenceIds": []any{"ev-child-1"}, "parentTodoRef": "parent-todo"},
				map[string]any{"id": "child-2", "content": "Write summary", "status": "blocked"},
			},
		},
	})
	svc := NewService(Dependencies{Jobs: repo, ChildTodos: todos})
	before := todos.clone("thread-parent")

	result := svc.ProjectChildTodosTaskJob("thread-parent", TaskJobChildTodoProjectRequest{JobID: record.ID, ClientRequestID: "project-1"})
	if !result.IsError || result.ErrorCode != TaskJobErrorConflict ||
		stringValue(result.Response, "code") != "validation_error" || len(result.Response) != 1 {
		t.Fatalf("unbound child todo projection should be quarantined: %#v", result)
	}
	if !sameMapValue(before, todos.clone("thread-parent")) {
		t.Fatalf("parent todos should not be mutated: before=%#v after=%#v", before, todos.clone("thread-parent"))
	}
	loaded, _ := repo.LoadChildRun(record.ID)
	if len(loaded.ChildTodoLists) != 0 || len(loaded.ChildTodoProjections) != 0 {
		t.Fatalf("quarantined child todo projection wrote durable child content: %#v", loaded)
	}

	listResult := svc.ListTaskJobViews("thread-parent", TaskJobListRequest{})
	jobs, _ := listResult.Response["jobs"].([]any)
	jobView, _ := jobs[0].(map[string]any)
	if jobView["outputWithheld"] != true || jobView["canReadOutput"] != false || jobView["childTodoCount"] != nil {
		t.Fatalf("legacy unbound list view must expose only the closed metadata projection: %#v", jobView)
	}
	output := svc.OutputTaskJob("thread-parent", TaskJobOutputRequest{JobID: record.ID}, false)
	if output.IsError || output.Response["schemaVersion"] != 1 || output.Response["availability"] != "withheld" ||
		output.Response["jobId"] != record.ID || output.Response["outputWithheld"] != true ||
		output.Response["childTodoCompletedCount"] != nil {
		t.Fatalf("legacy unbound output must use the closed withheld response: %#v", output)
	}
}

func TestTaskJobServiceRejectsInvalidChildTodoProjection(t *testing.T) {
	record := domainjob.Record{
		ID:             "job-child-todos",
		ParentThreadID: "thread-parent",
		Kind:           "subagent",
		Status:         string(domainjob.StatusCompleted),
		ChildThreadID:  "thread-child",
		Background:     true,
	}
	repo := newTaskJobRepoStub([]domainjob.Record{record})
	svc := NewService(Dependencies{Jobs: repo, ChildTodos: newChildTodoStoreStub(map[string]map[string]any{
		"thread-child": {"threadId": "thread-child", "items": []any{}},
	})})
	if result := svc.ProjectChildTodosTaskJob("other-parent", TaskJobChildTodoProjectRequest{JobID: record.ID}); !result.IsError || result.ErrorCode != TaskJobErrorNotFound {
		t.Fatalf("cross-parent projection should reject: %#v", result)
	}
	if result := svc.ProjectChildTodosTaskJob("thread-parent", TaskJobChildTodoProjectRequest{JobID: "missing"}); !result.IsError || result.ErrorCode != TaskJobErrorNotFound {
		t.Fatalf("missing job projection should reject: %#v", result)
	}
	running := record
	running.ID = "job-running"
	running.Status = string(domainjob.StatusRunning)
	repo.records[running.ID] = running
	if result := svc.ProjectChildTodosTaskJob("thread-parent", TaskJobChildTodoProjectRequest{JobID: running.ID}); !result.IsError || result.ErrorCode != TaskJobErrorConflict {
		t.Fatalf("running child projection should reject: %#v", result)
	}
}

func TestTaskJobServiceQuarantinesChildTodoProjectionReject(t *testing.T) {
	record := domainjob.Record{
		ID:             "job-child-todos",
		ParentThreadID: "thread-parent",
		Kind:           "subagent",
		Status:         string(domainjob.StatusCompleted),
		ChildThreadID:  "thread-child",
		Background:     true,
		ChildTodoLists: []domainjob.ChildTodoList{{
			ID:             "list-1",
			ParentThreadID: "thread-parent",
			ChildThreadID:  "thread-child",
			ChildRunID:     "job-child-todos",
			JobID:          "job-child-todos",
			Scope:          TodoScopeChild,
		}},
		ChildTodoProjections: []domainjob.ChildTodoProjection{{
			ID:              "projection-1",
			ParentThreadID:  "thread-parent",
			ChildThreadID:   "thread-child",
			ChildRunID:      "job-child-todos",
			JobID:           "job-child-todos",
			ChildTodoListID: "list-1",
			Status:          ChildTodoProjectionProposed,
			ProjectedItems:  []domainjob.ChildTodoProjectionItem{{ID: "child-1", Content: "Done", Status: "completed"}},
		}},
	}
	repo := newTaskJobRepoStub([]domainjob.Record{record})
	todos := newChildTodoStoreStub(map[string]map[string]any{
		"thread-parent": {
			"threadId": "thread-parent",
			"items":    []any{map[string]any{"id": "parent-todo", "content": "Parent stays pending", "status": "pending"}},
		},
	})
	svc := NewService(Dependencies{Jobs: repo, ChildTodos: todos})
	before := todos.clone("thread-parent")

	result := svc.RejectChildTodoProjectionTaskJob("thread-parent", TaskJobChildTodoRejectRequest{JobID: record.ID, ProjectionID: "projection-1"})
	if !result.IsError || result.ErrorCode != TaskJobErrorConflict {
		t.Fatalf("unbound child todo reject should be quarantined: %#v", result)
	}
	loaded, _ := repo.LoadChildRun(record.ID)
	if loaded.ChildTodoProjections[0].Status != ChildTodoProjectionProposed || strings.TrimSpace(loaded.ChildTodoProjections[0].RejectedAt) != "" {
		t.Fatalf("quarantined projection reject mutated durable state: %#v", loaded.ChildTodoProjections)
	}
	if !sameMapValue(before, todos.clone("thread-parent")) {
		t.Fatalf("projection reject should not mutate parent todos: before=%#v after=%#v", before, todos.clone("thread-parent"))
	}
	if result := svc.RejectChildTodoProjectionTaskJob("other-parent", TaskJobChildTodoRejectRequest{JobID: record.ID, ProjectionID: "projection-1"}); !result.IsError || result.ErrorCode != TaskJobErrorNotFound {
		t.Fatalf("cross-parent reject should reject: %#v", result)
	}
}

func TestTaskJobServiceChildTodoProjectionCannotCompleteParentGoal(t *testing.T) {
	if err := ParentGoalNotCompletedByChildProjectionError(); err == nil || !strings.Contains(err.Error(), "cannot complete the parent goal") {
		t.Fatalf("child todo projection must remain a parent-owned proposal: %v", err)
	}
}

func TestTaskJobServiceQuarantinesMappedChildTodoProjectionAccept(t *testing.T) {
	record := taskJobChildTodoProjectionRecord("job-child-todos", "thread-parent")
	repo := newTaskJobRepoStub([]domainjob.Record{record})
	todos := newChildTodoStoreStub(map[string]map[string]any{
		"thread-parent": {
			"threadId":  "thread-parent",
			"updatedAt": "2026-07-07T00:00:00Z",
			"items": []any{
				map[string]any{"id": "parent-todo", "content": "Parent todo", "status": "pending", "createdAt": "2026-07-07T00:00:00Z", "updatedAt": "2026-07-07T00:00:00Z"},
			},
		},
	})
	events := &taskJobEventRecorderStub{}
	svc := NewService(Dependencies{Jobs: repo, ChildTodos: todos, Events: events})

	result := svc.AcceptChildTodoProjectionTaskJob("thread-parent", TaskJobChildTodoAcceptRequest{
		JobID:                        record.ID,
		ProjectionID:                 "projection-1",
		ApprovalID:                   "approval-1",
		ClientRequestID:              "accept-1",
		ExpectedParentTodosUpdatedAt: "2026-07-07T00:00:00Z",
	})
	if !result.IsError || result.ErrorCode != TaskJobErrorConflict {
		t.Fatalf("unbound mapped child todo accept should be quarantined: %#v", result)
	}
	parent := todos.clone("thread-parent")
	items := listAny(parent["items"])
	if len(items) != 1 || stringValue(items[0].(map[string]any), "status") != "pending" {
		t.Fatalf("quarantined accept mutated parent todo: %#v", parent)
	}
	loaded, _ := repo.LoadChildRun(record.ID)
	if loaded.ChildTodoProjections[0].Status != ChildTodoProjectionProposed || loaded.ChildTodoProjections[0].AcceptedAt != "" {
		t.Fatalf("quarantined accept mutated projection: %#v", loaded.ChildTodoProjections)
	}
	if len(loaded.ProjectionDecisions) != 0 {
		t.Fatalf("quarantined accept persisted an unbound decision: %#v", loaded)
	}
	if events.count("todos_updated") != 0 {
		t.Fatalf("quarantined accept emitted todos_updated")
	}

	repeated := svc.AcceptChildTodoProjectionTaskJob("thread-parent", TaskJobChildTodoAcceptRequest{
		JobID:                        record.ID,
		ProjectionID:                 "projection-1",
		ApprovalID:                   "approval-1",
		ClientRequestID:              "accept-1",
		ExpectedParentTodosUpdatedAt: "stale-is-ignored-for-idempotent-replay",
	})
	if !repeated.IsError || repeated.ErrorCode != TaskJobErrorConflict {
		t.Fatalf("repeated unbound accept should remain quarantined: %#v", repeated)
	}
	if events.count("todos_updated") != 0 {
		t.Fatalf("repeated accept should not duplicate todos_updated, got %d", events.count("todos_updated"))
	}
}

func TestTaskJobServiceQuarantinesUnmappedChildTodoProjectionAccept(t *testing.T) {
	record := taskJobChildTodoProjectionRecord("job-child-todos", "thread-parent")
	record.ChildTodoProjections[0].ProjectedItems[0].ParentTodoRef = ""
	repo := newTaskJobRepoStub([]domainjob.Record{record})
	todos := newChildTodoStoreStub(map[string]map[string]any{
		"thread-parent": {
			"threadId":  "thread-parent",
			"updatedAt": "2026-07-07T00:00:00Z",
			"items":     []any{map[string]any{"id": "parent-todo", "content": "Parent todo", "status": "pending"}},
		},
	})
	events := &taskJobEventRecorderStub{}
	svc := NewService(Dependencies{Jobs: repo, ChildTodos: todos, Events: events})
	before := todos.clone("thread-parent")

	result := svc.AcceptChildTodoProjectionTaskJob("thread-parent", TaskJobChildTodoAcceptRequest{
		JobID:                        record.ID,
		ProjectionID:                 "projection-1",
		ApprovalID:                   "approval-1",
		ClientRequestID:              "accept-unmapped",
		ExpectedParentTodosUpdatedAt: "2026-07-07T00:00:00Z",
	})
	if !result.IsError || result.ErrorCode != TaskJobErrorConflict {
		t.Fatalf("unbound unmapped accept should be quarantined: %#v", result)
	}
	if !sameMapValue(before, todos.clone("thread-parent")) {
		t.Fatalf("unmapped accept should not mutate parent todos: before=%#v after=%#v", before, todos.clone("thread-parent"))
	}
	loaded, _ := repo.LoadChildRun(record.ID)
	if len(loaded.ProjectionDecisions) != 0 {
		t.Fatalf("quarantined unmapped accept persisted a decision: %#v", loaded.ProjectionDecisions)
	}
	if events.count("todos_updated") != 0 {
		t.Fatalf("unmapped accept should not emit todos_updated")
	}
}

func TestTaskJobServiceAcceptChildTodoProjectionRejectsUnsafeRequests(t *testing.T) {
	record := taskJobChildTodoProjectionRecord("job-child-todos", "thread-parent")
	rejected := record
	rejected.ID = "job-child-todos-rejected"
	rejected.ChildTodoProjections = []domainjob.ChildTodoProjection{record.ChildTodoProjections[0]}
	rejected.ChildTodoProjections[0].Status = ChildTodoProjectionRejected
	repo := newTaskJobRepoStub([]domainjob.Record{record, rejected})
	todos := newChildTodoStoreStub(map[string]map[string]any{
		"thread-parent": {
			"threadId":  "thread-parent",
			"updatedAt": "2026-07-07T00:00:00Z",
			"items":     []any{map[string]any{"id": "parent-todo", "content": "Parent todo", "status": "pending"}},
		},
	})
	svc := NewService(Dependencies{Jobs: repo, ChildTodos: todos})
	valid := TaskJobChildTodoAcceptRequest{
		JobID:                        record.ID,
		ProjectionID:                 "projection-1",
		ApprovalID:                   "approval-1",
		ExpectedParentTodosUpdatedAt: "2026-07-07T00:00:00Z",
	}

	if result := svc.AcceptChildTodoProjectionTaskJob("thread-parent", TaskJobChildTodoAcceptRequest{JobID: record.ID, ProjectionID: "projection-1", ExpectedParentTodosUpdatedAt: "2026-07-07T00:00:00Z"}); !result.IsError || result.ErrorCode != TaskJobErrorValidation {
		t.Fatalf("accept without approval should reject: %#v", result)
	}
	if result := svc.AcceptChildTodoProjectionTaskJob("other-parent", valid); !result.IsError || result.ErrorCode != TaskJobErrorNotFound {
		t.Fatalf("cross-parent accept should reject: %#v", result)
	}
	missing := valid
	missing.ProjectionID = "missing"
	if result := svc.AcceptChildTodoProjectionTaskJob("thread-parent", missing); !result.IsError || result.ErrorCode != TaskJobErrorConflict {
		t.Fatalf("missing projection accept should reject: %#v", result)
	}
	rejectedRequest := valid
	rejectedRequest.JobID = rejected.ID
	if result := svc.AcceptChildTodoProjectionTaskJob("thread-parent", rejectedRequest); !result.IsError || result.ErrorCode != TaskJobErrorConflict {
		t.Fatalf("rejected projection accept should reject: %#v", result)
	}
	stale := valid
	stale.ExpectedParentTodosUpdatedAt = "2026-07-07T00:01:00Z"
	if result := svc.AcceptChildTodoProjectionTaskJob("thread-parent", stale); !result.IsError || result.ErrorCode != TaskJobErrorConflict {
		t.Fatalf("stale parent todos should reject: %#v", result)
	}
}

func taskJobChildTodoProjectionRecord(id string, parentThreadID string) domainjob.Record {
	return domainjob.Record{
		ID:             id,
		ParentThreadID: parentThreadID,
		Kind:           "subagent",
		Status:         string(domainjob.StatusCompleted),
		ChildThreadID:  "thread-child",
		Background:     true,
		ChildTodoLists: []domainjob.ChildTodoList{{
			ID:             "list-1",
			ParentThreadID: parentThreadID,
			ChildThreadID:  "thread-child",
			ChildRunID:     id,
			JobID:          id,
			Scope:          TodoScopeChild,
			Items: []domainjob.ChildTodoItem{{
				ID:            "child-1",
				Content:       "Finish child work",
				Status:        "completed",
				EvidenceIDs:   []string{"ev-child-1"},
				ParentTodoRef: "parent-todo",
			}},
		}},
		ChildTodoProjections: []domainjob.ChildTodoProjection{{
			ID:              "projection-1",
			ParentThreadID:  parentThreadID,
			ChildThreadID:   "thread-child",
			ChildRunID:      id,
			JobID:           id,
			ChildTodoListID: "list-1",
			Status:          ChildTodoProjectionProposed,
			ProjectedItems: []domainjob.ChildTodoProjectionItem{{
				ID:            "child-1",
				Content:       "Finish child work",
				Status:        "completed",
				EvidenceIDs:   []string{"ev-child-1"},
				ParentTodoRef: "parent-todo",
			}},
			EvidenceIDs: []string{"ev-child-1"},
			Summary:     "1 child todos: 1 completed, 0 in progress, 0 pending, 0 blocked, 0 canceled",
			CreatedAt:   "2026-07-07T00:00:00Z",
		}},
	}
}

func TestTaskJobServicePauseResumeAndKillPausedChild(t *testing.T) {
	record := domainjob.Record{
		ID:             "job-pause",
		ParentGoalID:   "goal-pause",
		ParentThreadID: "thread-a",
		Kind:           "subagent",
		Status:         string(domainjob.StatusRunning),
		ChildThreadID:  "child-a",
		Background:     true,
	}
	record.PauseState = taskJobStubPauseState(record)
	repo := newTaskJobRepoStub([]domainjob.Record{record})
	state := NewRuntimeState()
	ctx, cancel := context.WithCancel(context.Background())
	state.RegisterBackgroundJob(record.ID, cancel)
	defer state.UnregisterBackgroundJob(record.ID)
	svc := NewService(Dependencies{Jobs: repo, State: state})

	pause := svc.PauseTaskJob("thread-a", TaskJobPauseRequest{JobID: record.ID, ClientRequestID: "pause_1"})
	if pause.IsError || stringValue(pause.Response, "status") != "requested" {
		t.Fatalf("pause should be requested: %#v", pause)
	}
	loaded, _ := repo.LoadChildRun(record.ID)
	if loaded.Status != string(domainjob.StatusPauseRequested) || loaded.PauseState.PauseRequestID != "pause_1" {
		t.Fatalf("pause request should persist: %#v", loaded)
	}

	paused := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := state.WaitIfBackgroundJobPauseRequested(ctx, record.ID, BackgroundJobPauseCallbacks{
			OnPaused: func(snapshot BackgroundJobPauseSnapshot) error {
				latest, loadErr := repo.LoadChildRun(record.ID)
				if loadErr != nil {
					return loadErr
				}
				request := LatestRequestedPause(latest)
				token := BuildResumeToken(latest, request, snapshot.PausedAt)
				if _, _, err := repo.MarkPauseRequestPaused(record.ID, request.ID, snapshot.PausedAt.Format(time.RFC3339Nano), token); err != nil {
					return err
				}
				close(paused)
				return nil
			},
			OnResumed: func(snapshot BackgroundJobPauseSnapshot) error {
				if _, _, err := repo.MarkPauseRequestResumed(record.ID, snapshot.PauseRequestID, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
					return err
				}
				_, _, err := repo.CompletePauseResume(record.ID, snapshot.PauseRequestID)
				return err
			},
		})
		done <- err
	}()
	select {
	case <-paused:
	case <-time.After(time.Second):
		t.Fatal("pause should admit at safe boundary")
	}
	loaded, _ = repo.LoadChildRun(record.ID)
	if loaded.Status != string(domainjob.StatusPaused) || !loaded.PauseState.CanResume {
		t.Fatalf("pause should mark child paused: %#v", loaded)
	}

	resume := svc.ResumeTaskJob("thread-a", TaskJobResumeRequest{JobID: record.ID})
	if resume.IsError || stringValue(resume.Response, "status") != "resume_requested" {
		t.Fatalf("resume should be requested: %#v", resume)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("paused runner should resume cleanly: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("resume should unblock paused runner")
	}
	loaded, _ = repo.LoadChildRun(record.ID)
	if loaded.Status != string(domainjob.StatusRunning) {
		t.Fatalf("resume should restore running state: %#v", loaded)
	}

	pause = svc.PauseTaskJob("thread-a", TaskJobPauseRequest{JobID: record.ID, ClientRequestID: "pause_2"})
	if pause.IsError {
		t.Fatalf("second pause should request: %#v", pause)
	}
	pausedAgain := make(chan struct{})
	go func() {
		defer state.UnregisterBackgroundJob(record.ID)
		_, err := state.WaitIfBackgroundJobPauseRequested(ctx, record.ID, BackgroundJobPauseCallbacks{
			OnPaused: func(snapshot BackgroundJobPauseSnapshot) error {
				latest, _ := repo.LoadChildRun(record.ID)
				request := LatestRequestedPause(latest)
				token := BuildResumeToken(latest, request, snapshot.PausedAt)
				_, _, err := repo.MarkPauseRequestPaused(record.ID, request.ID, snapshot.PausedAt.Format(time.RFC3339Nano), token)
				close(pausedAgain)
				return err
			},
		})
		done <- err
	}()
	select {
	case <-pausedAgain:
	case <-time.After(time.Second):
		t.Fatal("second pause should admit")
	}
	kill := svc.KillTaskJob("thread-a", TaskJobKillRequest{JobID: record.ID, Reason: "stop paused"})
	if kill.IsError {
		t.Fatalf("kill paused should succeed: %#v", kill)
	}
	loaded, _ = repo.LoadChildRun(record.ID)
	if loaded.Status != string(domainjob.StatusKilled) {
		t.Fatalf("kill paused should persist killed: %#v", loaded)
	}
}

func TestResumeFailureCannotSplitDurableAndRunnerState(t *testing.T) {
	record := domainjob.Record{
		ID: "job-resume-fail", ParentGoalID: "goal-resume-fail", ParentThreadID: "thread-a",
		Kind: "subagent", Status: string(domainjob.StatusRunning), ChildThreadID: "child-a", Background: true,
	}
	record.PauseState = taskJobStubPauseState(record)
	repo := newTaskJobRepoStub([]domainjob.Record{record})
	now := time.Now().UTC()
	record, request, err := repo.AddPauseRequest(record.ID, domainjob.PauseRequest{
		ID: "pause-resume-fail", ParentThreadID: record.ParentThreadID, ChildRunID: record.ID, JobID: record.ID,
		Status: "requested", RequestedAt: now.Add(-time.Second).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	record, request, err = repo.MarkPauseRequestPaused(record.ID, request.ID, now.Format(time.RFC3339Nano), domainjob.ResumeToken{
		ResumeToken: "resume-fail", IssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(time.Hour).Format(time.RFC3339Nano),
		ChildRunID: record.ID, ParentThreadID: record.ParentThreadID,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := &failingResumePauseRuntime{snapshot: BackgroundJobPauseSnapshot{
		JobID: record.ID, PauseRequestID: request.ID, Status: "paused",
	}}
	svc := NewService(Dependencies{Jobs: repo, PauseRuntime: runtime})
	result := svc.ResumeTaskJob(record.ParentThreadID, TaskJobResumeRequest{JobID: record.ID})
	if !result.IsError || result.ErrorCode != TaskJobErrorConflict || runtime.resumeCalls != 1 {
		t.Fatalf("resume signal failure did not fail closed: %#v runtime=%#v", result, runtime)
	}
	loaded, err := repo.LoadChildRun(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	latest := LatestPauseRequest(loaded)
	if loaded.Status != string(domainjob.StatusPaused) || latest.Status != "paused" || !loaded.PauseState.Paused || !loaded.PauseState.CanResume {
		t.Fatalf("failed resume split durable pause state: %#v", loaded)
	}
}

func TestTaskJobServicePauseRejectsUnsafeRequests(t *testing.T) {
	record := domainjob.Record{
		ID:             "job-pause",
		ParentGoalID:   "goal-pause",
		ParentThreadID: "thread-a",
		Kind:           "subagent",
		Status:         string(domainjob.StatusRunning),
		ChildThreadID:  "child-a",
		Background:     true,
	}
	record.PauseState = taskJobStubPauseState(record)
	repo := newTaskJobRepoStub([]domainjob.Record{record})
	svc := NewService(Dependencies{Jobs: repo, State: NewRuntimeState()})
	if result := svc.PauseTaskJob("thread-b", TaskJobPauseRequest{JobID: record.ID}); !result.IsError || result.ErrorCode != TaskJobErrorNotFound {
		t.Fatalf("cross-parent pause should reject: %#v", result)
	}
	if result := svc.PauseTaskJob("thread-a", TaskJobPauseRequest{JobID: "missing"}); !result.IsError || result.ErrorCode != TaskJobErrorNotFound {
		t.Fatalf("missing job pause should reject: %#v", result)
	}
	if result := svc.PauseTaskJob("thread-a", TaskJobPauseRequest{JobID: record.ID}); !result.IsError || result.ErrorCode != TaskJobErrorConflict {
		t.Fatalf("inactive runner pause should reject without fake pause: %#v", result)
	}
	if _, err := repo.UpdateChildRun(record.ID, domainjob.UpdateRequest{Status: string(domainjob.StatusCompleted)}); err != nil {
		t.Fatalf("complete child: %v", err)
	}
	if result := svc.PauseTaskJob("thread-a", TaskJobPauseRequest{JobID: record.ID}); !result.IsError || result.ErrorCode != TaskJobErrorConflict {
		t.Fatalf("terminal pause should reject: %#v", result)
	}
}

func TestTaskJobServiceReviewsWorktreeIsolation(t *testing.T) {
	record := domainjob.Record{
		ID:             "job-review",
		ParentThreadID: "thread-a",
		Kind:           "subagent",
		Status:         string(domainjob.StatusCompleted),
		ChildThreadID:  "child-review",
		Background:     true,
		IsolationMode:  "worktree",
		WorktreePath:   "/tmp/analytix/subagent-worktrees/thread-a/job-review",
		WorktreeBranch: "codex/subagent/thread-a/job-review",
		BaseCommit:     "base",
		CurrentCommit:  "base",
		MergeStatus:    "not_requested",
	}
	repo := newTaskJobRepoStub([]domainjob.Record{record})
	before, _ := repo.LoadChildRun(record.ID)
	manager := &taskJobReviewWorktreeManager{
		isolation: domainjob.WorktreeIsolation{
			IsolationMode:  "worktree",
			WorktreePath:   record.WorktreePath,
			WorktreeBranch: record.WorktreeBranch,
			BaseCommit:     "base",
			CurrentCommit:  "current",
			ChangedFiles: []domainjob.ChangedFile{
				{Path: "src/app.go", Status: "M"},
				{Path: "src/new.go", Status: "??"},
			},
			DiffSummary: "2 files changed",
			MergeStatus: "not_requested",
		},
	}
	svc := NewService(Dependencies{Jobs: repo, WorktreeManager: manager})

	result := svc.ReviewTaskJobIsolation("thread-a", TaskJobIsolationReviewRequest{JobID: "job-review", ClientRequestID: "review-1"})
	assertWorktreeIsolationQuarantined(t, result)
	loaded, _ := repo.LoadChildRun("job-review")
	if !reflect.DeepEqual(loaded, before) || manager.calls != 0 {
		t.Fatalf("quarantined review touched manager or lineage: loaded=%#v calls=%d", loaded, manager.calls)
	}
}

func TestTaskJobServiceIsolationReviewRejectsUnsafeJobs(t *testing.T) {
	records := []domainjob.Record{{
		ID:             "plain-job",
		ParentThreadID: "thread-a",
		Kind:           "subagent",
		Status:         string(domainjob.StatusCompleted),
		ChildThreadID:  "child-plain",
		Background:     true,
	}}
	repo := newTaskJobRepoStub(records)
	svc := NewService(Dependencies{Jobs: repo, WorktreeManager: &taskJobReviewWorktreeManager{}})
	if result := svc.ReviewTaskJobIsolation("thread-b", TaskJobIsolationReviewRequest{JobID: "plain-job"}); !result.IsError || result.ErrorCode != TaskJobErrorNotFound {
		t.Fatalf("cross-parent review should reject: %#v", result)
	}
	if result := svc.ReviewTaskJobIsolation("thread-a", TaskJobIsolationReviewRequest{JobID: "missing"}); !result.IsError || result.ErrorCode != TaskJobErrorNotFound {
		t.Fatalf("missing job review should reject: %#v", result)
	}
	if result := svc.ReviewTaskJobIsolation("thread-a", TaskJobIsolationReviewRequest{JobID: "plain-job"}); !result.IsError || result.ErrorCode != TaskJobErrorValidation {
		t.Fatalf("non-isolated job review should reject: %#v", result)
	}
}

func TestTaskJobServiceRejectsWorktreeIsolationResult(t *testing.T) {
	record := taskJobReviewedIsolationRecord("job-reject", "thread-a")
	repo := newTaskJobRepoStub([]domainjob.Record{record})
	before, _ := repo.LoadChildRun(record.ID)
	svc := NewService(Dependencies{Jobs: repo, WorktreeManager: &taskJobReviewWorktreeManager{}})

	result := svc.RejectTaskJobIsolation("thread-a", TaskJobIsolationRejectRequest{JobID: record.ID, ClientRequestID: "reject-1", Reason: "not needed"})
	assertWorktreeIsolationQuarantined(t, result)
	loaded, _ := repo.LoadChildRun(record.ID)
	if !reflect.DeepEqual(loaded, before) {
		t.Fatalf("quarantined reject wrote a merge decision: %#v", loaded)
	}
	if result := svc.RejectTaskJobIsolation("thread-b", TaskJobIsolationRejectRequest{JobID: record.ID}); !result.IsError || result.ErrorCode != TaskJobErrorNotFound {
		t.Fatalf("cross-parent reject should fail: %#v", result)
	}
	if result := svc.RejectTaskJobIsolation("thread-a", TaskJobIsolationRejectRequest{JobID: "missing"}); !result.IsError || result.ErrorCode != TaskJobErrorNotFound {
		t.Fatalf("missing reject should fail: %#v", result)
	}
	notReviewed := taskJobReviewedIsolationRecord("job-not-reviewed", "thread-a")
	notReviewed.MergeStatus = "not_requested"
	repo.mu.Lock()
	repo.records[notReviewed.ID] = notReviewed
	repo.mu.Unlock()
	if result := svc.RejectTaskJobIsolation("thread-a", TaskJobIsolationRejectRequest{JobID: notReviewed.ID}); !result.IsError || result.ErrorCode != TaskJobErrorConflict {
		t.Fatalf("not-reviewed reject should fail: %#v", result)
	}
	plain := domainjob.Record{ID: "plain-reject", ParentThreadID: "thread-a", Kind: "subagent", Status: string(domainjob.StatusCompleted), ChildThreadID: "child", Background: true}
	repo.mu.Lock()
	repo.records[plain.ID] = plain
	repo.mu.Unlock()
	if result := svc.RejectTaskJobIsolation("thread-a", TaskJobIsolationRejectRequest{JobID: plain.ID}); !result.IsError || result.ErrorCode != TaskJobErrorValidation {
		t.Fatalf("non-isolated reject should fail: %#v", result)
	}
}

func TestTaskJobServiceCleanupWorktreeIsolation(t *testing.T) {
	record := taskJobReviewedIsolationRecord("job-cleanup", "thread-a")
	record.MergeStatus = "rejected"
	repo := newTaskJobRepoStub([]domainjob.Record{record})
	before, _ := repo.LoadChildRun(record.ID)
	manager := &taskJobReviewWorktreeManager{receipt: domainjob.CleanupReceipt{
		ID:           "cleanup-1",
		WorktreePath: record.WorktreePath,
		Branch:       record.WorktreeBranch,
		Removed:      true,
		CreatedAt:    time.Date(2026, 7, 7, 1, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
	}}
	svc := NewService(Dependencies{Jobs: repo, WorktreeManager: manager})

	result := svc.CleanupTaskJobIsolation("thread-a", TaskJobIsolationCleanupRequest{JobID: record.ID, ClientRequestID: "cleanup-1"})
	assertWorktreeIsolationQuarantined(t, result)
	loaded, _ := repo.LoadChildRun(record.ID)
	if !reflect.DeepEqual(loaded, before) || manager.calls != 0 {
		t.Fatalf("quarantined cleanup touched manager or lineage: loaded=%#v calls=%d", loaded, manager.calls)
	}
}

func TestTaskJobServiceCleanupAcceptedWorktreeIsolation(t *testing.T) {
	record := taskJobReviewedIsolationRecord("job-accepted-cleanup", "thread-a")
	record.MergeStatus = "accepted"
	record.AcceptDecisions = []domainjob.AcceptDecision{{
		ID:                 "accept-1",
		ParentThreadID:     "thread-a",
		ChildRunID:         record.ID,
		JobID:              record.ID,
		ApprovalID:         "approval-1",
		BaseCommit:         record.BaseCommit,
		AppliedPatchDigest: "sha256:abc",
		CreatedAt:          time.Date(2026, 7, 7, 3, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
	}}
	repo := newTaskJobRepoStub([]domainjob.Record{record})
	before, _ := repo.LoadChildRun(record.ID)
	manager := &taskJobReviewWorktreeManager{receipt: domainjob.CleanupReceipt{
		ID:           "cleanup-accepted-1",
		WorktreePath: record.WorktreePath,
		Branch:       record.WorktreeBranch,
		Removed:      true,
		CreatedAt:    time.Date(2026, 7, 7, 3, 5, 0, 0, time.UTC).Format(time.RFC3339Nano),
	}}
	svc := NewService(Dependencies{Jobs: repo, WorktreeManager: manager})

	result := svc.CleanupTaskJobIsolation("thread-a", TaskJobIsolationCleanupRequest{JobID: record.ID, ClientRequestID: "cleanup-accepted-1"})
	assertWorktreeIsolationQuarantined(t, result)
	loaded, _ := repo.LoadChildRun(record.ID)
	if !reflect.DeepEqual(loaded, before) || manager.calls != 0 {
		t.Fatalf("quarantined accepted cleanup touched manager or lineage: loaded=%#v calls=%d", loaded, manager.calls)
	}
}

func TestTaskJobServiceCleanupRejectsUnsafeJobs(t *testing.T) {
	running := taskJobReviewedIsolationRecord("job-running", "thread-a")
	running.Status = string(domainjob.StatusRunning)
	nonOwned := taskJobReviewedIsolationRecord("job-non-owned", "thread-a")
	nonOwned.WorktreeBranch = "main"
	notReviewed := taskJobReviewedIsolationRecord("job-not-reviewed", "thread-a")
	notReviewed.MergeStatus = "not_requested"
	acceptedWithoutReceipt := taskJobReviewedIsolationRecord("job-accepted-no-receipt", "thread-a")
	acceptedWithoutReceipt.MergeStatus = "accepted"
	cleaned := taskJobReviewedIsolationRecord("job-cleaned", "thread-a")
	cleaned.MergeStatus = "cleaned"
	cleaned.CleanupReceipts = []domainjob.CleanupReceipt{{
		ID:           "cleanup-existing",
		WorktreePath: cleaned.WorktreePath,
		Branch:       cleaned.WorktreeBranch,
		Removed:      true,
		CreatedAt:    time.Date(2026, 7, 7, 4, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
	}}
	repo := newTaskJobRepoStub([]domainjob.Record{running, nonOwned, notReviewed, acceptedWithoutReceipt, cleaned})
	svc := NewService(Dependencies{Jobs: repo, WorktreeManager: &taskJobReviewWorktreeManager{}})

	if result := svc.CleanupTaskJobIsolation("thread-a", TaskJobIsolationCleanupRequest{JobID: running.ID}); !result.IsError || result.ErrorCode != TaskJobErrorConflict {
		t.Fatalf("running cleanup should reject: %#v", result)
	}
	if result := svc.CleanupTaskJobIsolation("thread-b", TaskJobIsolationCleanupRequest{JobID: notReviewed.ID}); !result.IsError || result.ErrorCode != TaskJobErrorNotFound {
		t.Fatalf("cross-parent cleanup should reject: %#v", result)
	}
	if result := svc.CleanupTaskJobIsolation("thread-a", TaskJobIsolationCleanupRequest{JobID: "missing"}); !result.IsError || result.ErrorCode != TaskJobErrorNotFound {
		t.Fatalf("missing cleanup should reject: %#v", result)
	}
	if result := svc.CleanupTaskJobIsolation("thread-a", TaskJobIsolationCleanupRequest{JobID: nonOwned.ID}); !result.IsError || result.ErrorCode != TaskJobErrorValidation {
		t.Fatalf("non-owned cleanup should reject: %#v", result)
	}
	if result := svc.CleanupTaskJobIsolation("thread-a", TaskJobIsolationCleanupRequest{JobID: notReviewed.ID}); !result.IsError || result.ErrorCode != TaskJobErrorConflict {
		t.Fatalf("not-reviewed cleanup should reject: %#v", result)
	}
	if result := svc.CleanupTaskJobIsolation("thread-a", TaskJobIsolationCleanupRequest{JobID: acceptedWithoutReceipt.ID}); !result.IsError || result.ErrorCode != TaskJobErrorConflict {
		t.Fatalf("accepted cleanup without accept decision should reject: %#v", result)
	}
	if result := svc.CleanupTaskJobIsolation("thread-a", TaskJobIsolationCleanupRequest{JobID: cleaned.ID}); !result.IsError || result.ErrorCode != TaskJobErrorConflict {
		t.Fatalf("already-cleaned cleanup should deterministically reject: %#v", result)
	}
}

func TestTaskJobServiceAcceptsReviewedWorktreeIsolation(t *testing.T) {
	record := taskJobReviewedIsolationRecord("job-accept", "thread-a")
	repo := newTaskJobRepoStub([]domainjob.Record{record})
	before, _ := repo.LoadChildRun(record.ID)
	manager := &taskJobReviewWorktreeManager{
		acceptIsolation: domainjob.WorktreeIsolation{
			IsolationMode:  "worktree",
			WorktreePath:   record.WorktreePath,
			WorktreeBranch: record.WorktreeBranch,
			BaseCommit:     record.BaseCommit,
			CurrentCommit:  record.CurrentCommit,
			ChangedFiles:   record.ChangedFiles,
			DiffSummary:    record.DiffSummary,
			MergeStatus:    "accepted",
		},
		acceptDecision: domainjob.AcceptDecision{
			ID:                 "accept-1",
			ParentThreadID:     "thread-a",
			ChildRunID:         record.ID,
			JobID:              record.ID,
			MergeRequestID:     "merge-1",
			ApprovalID:         "approval-1",
			BaseCommit:         record.BaseCommit,
			ParentHeadBefore:   record.BaseCommit,
			ParentHeadAfter:    record.BaseCommit,
			ChangedFiles:       record.ChangedFiles,
			AppliedPatchDigest: "sha256:abc",
			CreatedAt:          time.Date(2026, 7, 7, 2, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		},
	}
	svc := NewService(Dependencies{Jobs: repo, WorktreeManager: manager})

	result := svc.AcceptTaskJobIsolation("thread-a", TaskJobIsolationAcceptRequest{
		JobID:           record.ID,
		MergeRequestID:  "merge-1",
		ApprovalID:      "approval-1",
		ParentWorkspace: "/parent",
	})
	assertWorktreeIsolationQuarantined(t, result)
	loaded, _ := repo.LoadChildRun(record.ID)
	if !reflect.DeepEqual(loaded, before) || manager.calls != 0 {
		t.Fatalf("quarantined accept touched manager or lineage: loaded=%#v calls=%d", loaded, manager.calls)
	}
}

func TestTaskJobServiceAcceptRejectsUnsafeJobs(t *testing.T) {
	reviewed := taskJobReviewedIsolationRecord("job-reviewed", "thread-a")
	notReviewed := taskJobReviewedIsolationRecord("job-not-reviewed", "thread-a")
	notReviewed.MergeStatus = "not_requested"
	conflicted := taskJobReviewedIsolationRecord("job-conflicted", "thread-a")
	conflicted.MergeStatus = "conflicted"
	conflicted.ChangedFiles = []domainjob.ChangedFile{{Path: "src/app.go", Status: "UU"}}
	plain := domainjob.Record{ID: "plain-accept", ParentThreadID: "thread-a", Kind: "subagent", Status: string(domainjob.StatusCompleted), ChildThreadID: "child", Background: true}
	repo := newTaskJobRepoStub([]domainjob.Record{reviewed, notReviewed, conflicted, plain})
	svc := NewService(Dependencies{Jobs: repo, WorktreeManager: &taskJobReviewWorktreeManager{}})

	valid := TaskJobIsolationAcceptRequest{JobID: reviewed.ID, ApprovalID: "approval-1", ParentWorkspace: "/parent"}
	if result := svc.AcceptTaskJobIsolation("thread-b", valid); !result.IsError || result.ErrorCode != TaskJobErrorNotFound {
		t.Fatalf("cross-parent accept should fail: %#v", result)
	}
	if result := svc.AcceptTaskJobIsolation("thread-a", TaskJobIsolationAcceptRequest{JobID: plain.ID, ApprovalID: "approval-1", ParentWorkspace: "/parent"}); !result.IsError || result.ErrorCode != TaskJobErrorValidation {
		t.Fatalf("non-isolated accept should fail: %#v", result)
	}
	if result := svc.AcceptTaskJobIsolation("thread-a", TaskJobIsolationAcceptRequest{JobID: notReviewed.ID, ApprovalID: "approval-1", ParentWorkspace: "/parent"}); !result.IsError || result.ErrorCode != TaskJobErrorConflict {
		t.Fatalf("not-reviewed accept should fail: %#v", result)
	}
	if result := svc.AcceptTaskJobIsolation("thread-a", TaskJobIsolationAcceptRequest{JobID: conflicted.ID, ApprovalID: "approval-1", ParentWorkspace: "/parent"}); !result.IsError || result.ErrorCode != TaskJobErrorConflict {
		t.Fatalf("conflicted accept should fail: %#v", result)
	}
	assertWorktreeIsolationQuarantined(t, svc.AcceptTaskJobIsolation("thread-a", TaskJobIsolationAcceptRequest{JobID: reviewed.ID, ParentWorkspace: "/parent"}))
	assertWorktreeIsolationQuarantined(t, svc.AcceptTaskJobIsolation("thread-a", TaskJobIsolationAcceptRequest{JobID: reviewed.ID, ApprovalID: "approval-1"}))
}

func TestTaskJobServiceConflictReportWorktreeIsolation(t *testing.T) {
	record := taskJobReviewedIsolationRecord("job-conflict-report", "thread-a")
	record.MergeStatus = "conflicted"
	record.ChangedFiles = []domainjob.ChangedFile{{Path: "src/app.go", Status: "UU"}}
	manager := &taskJobReviewWorktreeManager{
		repairIsolation: domainjob.WorktreeIsolation{
			IsolationMode:  "worktree",
			WorktreePath:   record.WorktreePath,
			WorktreeBranch: record.WorktreeBranch,
			BaseCommit:     record.BaseCommit,
			CurrentCommit:  "child-head",
			ChangedFiles:   record.ChangedFiles,
			DiffSummary:    "1 conflict",
			MergeStatus:    "conflicted",
		},
		conflictReport: domainjob.ConflictReport{
			ID:                     "conflict-1",
			ParentThreadID:         "thread-a",
			ChildRunID:             record.ID,
			JobID:                  record.ID,
			BaseCommit:             record.BaseCommit,
			ParentHeadAtConflict:   "parent-head",
			ChildHeadAtConflict:    "child-head",
			SourcePatchDigest:      "sha256:source",
			ConflictFiles:          record.ChangedFiles,
			ConflictSummary:        "1 conflict",
			CreatedAt:              time.Date(2026, 7, 7, 5, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
			TouchedParentWorkspace: false,
		},
	}
	repo := newTaskJobRepoStub([]domainjob.Record{record})
	before, _ := repo.LoadChildRun(record.ID)
	svc := NewService(Dependencies{Jobs: repo, WorktreeManager: manager})

	result := svc.ConflictReportTaskJobIsolation("thread-a", TaskJobIsolationConflictReportRequest{
		JobID:           record.ID,
		ClientRequestID: "conflict-1",
		ParentWorkspace: "/parent",
	})
	assertWorktreeIsolationQuarantined(t, result)
	loaded, _ := repo.LoadChildRun(record.ID)
	if !reflect.DeepEqual(loaded, before) || manager.calls != 0 {
		t.Fatalf("quarantined conflict report touched manager or lineage: loaded=%#v calls=%d", loaded, manager.calls)
	}
}

func TestTaskJobServiceConflictReportRejectsUnsafeJobs(t *testing.T) {
	conflicted := taskJobReviewedIsolationRecord("job-conflicted", "thread-a")
	conflicted.MergeStatus = "conflicted"
	notConflicted := taskJobReviewedIsolationRecord("job-clean", "thread-a")
	plain := domainjob.Record{ID: "plain-conflict", ParentThreadID: "thread-a", Kind: "subagent", Status: string(domainjob.StatusCompleted), ChildThreadID: "child", Background: true}
	repo := newTaskJobRepoStub([]domainjob.Record{conflicted, notConflicted, plain})
	svc := NewService(Dependencies{Jobs: repo, WorktreeManager: &taskJobReviewWorktreeManager{}})
	request := TaskJobIsolationConflictReportRequest{JobID: conflicted.ID, ParentWorkspace: "/parent"}

	if result := svc.ConflictReportTaskJobIsolation("thread-b", request); !result.IsError || result.ErrorCode != TaskJobErrorNotFound {
		t.Fatalf("cross-parent conflict report should reject: %#v", result)
	}
	if result := svc.ConflictReportTaskJobIsolation("thread-a", TaskJobIsolationConflictReportRequest{JobID: plain.ID, ParentWorkspace: "/parent"}); !result.IsError || result.ErrorCode != TaskJobErrorValidation {
		t.Fatalf("non-isolated conflict report should reject: %#v", result)
	}
	if result := svc.ConflictReportTaskJobIsolation("thread-a", TaskJobIsolationConflictReportRequest{JobID: notConflicted.ID, ParentWorkspace: "/parent"}); !result.IsError || result.ErrorCode != TaskJobErrorConflict {
		t.Fatalf("non-conflicted conflict report should reject: %#v", result)
	}
	assertWorktreeIsolationQuarantined(t, svc.ConflictReportTaskJobIsolation("thread-a", TaskJobIsolationConflictReportRequest{JobID: conflicted.ID}))
}

func TestTaskJobServiceRepairCheckAndAcceptWorktreeIsolation(t *testing.T) {
	record := taskJobReviewedIsolationRecord("job-repair", "thread-a")
	record.MergeStatus = "conflicted"
	record.ConflictReports = []domainjob.ConflictReport{{
		ID:                   "conflict-1",
		ParentThreadID:       "thread-a",
		ChildRunID:           record.ID,
		JobID:                record.ID,
		BaseCommit:           record.BaseCommit,
		ParentHeadAtConflict: "parent-head",
		ChildHeadAtConflict:  "child-head",
		ConflictFiles:        record.ChangedFiles,
		CreatedAt:            time.Date(2026, 7, 7, 5, 5, 0, 0, time.UTC).Format(time.RFC3339Nano),
	}}
	manager := &taskJobReviewWorktreeManager{
		repairIsolation: domainjob.WorktreeIsolation{
			IsolationMode:  "worktree",
			WorktreePath:   record.WorktreePath,
			WorktreeBranch: record.WorktreeBranch,
			BaseCommit:     record.BaseCommit,
			CurrentCommit:  record.CurrentCommit,
			ChangedFiles:   []domainjob.ChangedFile{{Path: "src/app.go", Status: "M"}},
			DiffSummary:    record.DiffSummary,
			MergeStatus:    "repair_checked",
		},
		repairReview: domainjob.RepairPatchReview{
			ID:                 "repair-review-1",
			ConflictReportID:   "conflict-1",
			ParentThreadID:     "thread-a",
			ChildRunID:         record.ID,
			JobID:              record.ID,
			RepairPatch:        "patch",
			RepairPatchDigest:  "sha256:repair",
			ExpectedParentHead: "parent-head",
			ChangedFiles:       []domainjob.ChangedFile{{Path: "src/app.go", Status: "M"}},
			DryRunStatus:       "clean",
			CreatedAt:          time.Date(2026, 7, 7, 5, 6, 0, 0, time.UTC).Format(time.RFC3339Nano),
		},
		repairDecision: domainjob.RepairDecision{
			ID:                 "repair-decision-1",
			RepairReviewID:     "repair-review-1",
			ApprovalID:         "approval-1",
			ParentHeadBefore:   "parent-head",
			ParentHeadAfter:    "parent-head",
			AppliedPatchDigest: "sha256:repair",
			CreatedAt:          time.Date(2026, 7, 7, 5, 7, 0, 0, time.UTC).Format(time.RFC3339Nano),
		},
	}
	repo := newTaskJobRepoStub([]domainjob.Record{record})
	before, _ := repo.LoadChildRun(record.ID)
	svc := NewService(Dependencies{Jobs: repo, WorktreeManager: manager})

	check := svc.RepairCheckTaskJobIsolation("thread-a", TaskJobIsolationRepairCheckRequest{
		JobID:            record.ID,
		ConflictReportID: "conflict-1",
		RepairPatch:      "patch",
		ParentWorkspace:  "/parent",
	})
	assertWorktreeIsolationQuarantined(t, check)
	loaded, _ := repo.LoadChildRun(record.ID)
	if !reflect.DeepEqual(loaded, before) || manager.calls != 0 {
		t.Fatalf("quarantined repair check touched manager or lineage: loaded=%#v calls=%d", loaded, manager.calls)
	}
	checked := before
	checked.MergeStatus = "repair_checked"
	checked.RepairPatchReviews = []domainjob.RepairPatchReview{manager.repairReview}
	repo.mu.Lock()
	repo.records[record.ID] = checked
	repo.mu.Unlock()
	accept := svc.RepairAcceptTaskJobIsolation("thread-a", TaskJobIsolationRepairAcceptRequest{
		JobID:           record.ID,
		RepairReviewID:  "repair-review-1",
		ApprovalID:      "approval-1",
		ParentWorkspace: "/parent",
	})
	assertWorktreeIsolationQuarantined(t, accept)
	loaded, _ = repo.LoadChildRun(record.ID)
	if !reflect.DeepEqual(loaded, checked) || manager.calls != 0 {
		t.Fatalf("quarantined repair accept touched manager or lineage: loaded=%#v calls=%d", loaded, manager.calls)
	}
}

func TestTaskJobServiceRepairCheckAndAcceptRejectUnsafeJobs(t *testing.T) {
	record := taskJobReviewedIsolationRecord("job-repair-unsafe", "thread-a")
	record.MergeStatus = "conflicted"
	record.ConflictReports = []domainjob.ConflictReport{{
		ID:                   "conflict-1",
		ParentThreadID:       "thread-a",
		ChildRunID:           record.ID,
		JobID:                record.ID,
		ParentHeadAtConflict: "parent-head",
	}}
	cleanReview := domainjob.RepairPatchReview{
		ID:                 "repair-review-1",
		ConflictReportID:   "conflict-1",
		ParentThreadID:     "thread-a",
		ChildRunID:         record.ID,
		JobID:              record.ID,
		RepairPatch:        "patch",
		ExpectedParentHead: "parent-head",
		DryRunStatus:       "clean",
	}
	checked := record
	checked.ID = "job-repair-checked"
	checked.MergeStatus = "repair_checked"
	checked.RepairPatchReviews = []domainjob.RepairPatchReview{cleanReview}
	cleaned := checked
	cleaned.ID = "job-repair-cleaned"
	cleaned.MergeStatus = "cleaned"
	plain := domainjob.Record{ID: "plain-repair", ParentThreadID: "thread-a", Kind: "subagent", Status: string(domainjob.StatusCompleted), ChildThreadID: "child", Background: true}
	repo := newTaskJobRepoStub([]domainjob.Record{record, checked, cleaned, plain})
	svc := NewService(Dependencies{Jobs: repo, WorktreeManager: &taskJobReviewWorktreeManager{}})

	checkRequest := TaskJobIsolationRepairCheckRequest{JobID: record.ID, ConflictReportID: "conflict-1", RepairPatch: "patch", ParentWorkspace: "/parent"}
	if result := svc.RepairCheckTaskJobIsolation("thread-b", checkRequest); !result.IsError || result.ErrorCode != TaskJobErrorNotFound {
		t.Fatalf("cross-parent repair check should reject: %#v", result)
	}
	if result := svc.RepairCheckTaskJobIsolation("thread-a", TaskJobIsolationRepairCheckRequest{JobID: plain.ID, ConflictReportID: "conflict-1", RepairPatch: "patch", ParentWorkspace: "/parent"}); !result.IsError || result.ErrorCode != TaskJobErrorValidation {
		t.Fatalf("non-isolated repair check should reject: %#v", result)
	}
	assertWorktreeIsolationQuarantined(t, svc.RepairCheckTaskJobIsolation("thread-a", TaskJobIsolationRepairCheckRequest{JobID: record.ID, ConflictReportID: "missing", RepairPatch: "patch", ParentWorkspace: "/parent"}))
	assertWorktreeIsolationQuarantined(t, svc.RepairCheckTaskJobIsolation("thread-a", TaskJobIsolationRepairCheckRequest{JobID: record.ID, ConflictReportID: "conflict-1", RepairPatch: "patch"}))

	if result := svc.RepairAcceptTaskJobIsolation("thread-b", TaskJobIsolationRepairAcceptRequest{JobID: checked.ID, RepairReviewID: "repair-review-1", ApprovalID: "approval-1", ParentWorkspace: "/parent"}); !result.IsError || result.ErrorCode != TaskJobErrorNotFound {
		t.Fatalf("cross-parent repair accept should reject: %#v", result)
	}
	if result := svc.RepairAcceptTaskJobIsolation("thread-a", TaskJobIsolationRepairAcceptRequest{JobID: plain.ID, RepairReviewID: "repair-review-1", ApprovalID: "approval-1", ParentWorkspace: "/parent"}); !result.IsError || result.ErrorCode != TaskJobErrorValidation {
		t.Fatalf("non-isolated repair accept should reject: %#v", result)
	}
	assertWorktreeIsolationQuarantined(t, svc.RepairAcceptTaskJobIsolation("thread-a", TaskJobIsolationRepairAcceptRequest{JobID: checked.ID, RepairReviewID: "missing", ApprovalID: "approval-1", ParentWorkspace: "/parent"}))
	assertWorktreeIsolationQuarantined(t, svc.RepairAcceptTaskJobIsolation("thread-a", TaskJobIsolationRepairAcceptRequest{JobID: checked.ID, RepairReviewID: "repair-review-1", ParentWorkspace: "/parent"}))
	assertWorktreeIsolationQuarantined(t, svc.RepairAcceptTaskJobIsolation("thread-a", TaskJobIsolationRepairAcceptRequest{JobID: checked.ID, RepairReviewID: "repair-review-1", ApprovalID: "approval-1"}))
	if result := svc.RepairAcceptTaskJobIsolation("thread-a", TaskJobIsolationRepairAcceptRequest{JobID: cleaned.ID, RepairReviewID: "repair-review-1", ApprovalID: "approval-1", ParentWorkspace: "/parent"}); !result.IsError || result.ErrorCode != TaskJobErrorConflict {
		t.Fatalf("cleaned repair accept should reject: %#v", result)
	}
}

func TestTaskJobHTTPServiceDoesNotResolveWorkspaceBeforeWorktreeAuthority(t *testing.T) {
	accept := taskJobReviewedIsolationRecord("job-http-accept", "thread-a")
	conflict := taskJobReviewedIsolationRecord("job-http-conflict", "thread-a")
	conflict.MergeStatus = "conflicted"
	repairCheck := taskJobReviewedIsolationRecord("job-http-repair-check", "thread-a")
	repairCheck.MergeStatus = "conflicted"
	repairAccept := taskJobReviewedIsolationRecord("job-http-repair-accept", "thread-a")
	repairAccept.MergeStatus = "repair_checked"
	repo := newTaskJobRepoStub([]domainjob.Record{accept, conflict, repairCheck, repairAccept})
	resolverCalls := 0
	httpService := NewTaskJobHTTPService(TaskJobHTTPServiceDeps{
		Service: NewService(Dependencies{Jobs: repo, WorktreeManager: &taskJobReviewWorktreeManager{}}),
		ParentWorkspace: func(string) string {
			resolverCalls++
			return "/PRIVATE/PARENT/WORKSPACE"
		},
	})
	results := []TaskJobServiceResult{
		httpService.AcceptTaskJobIsolation("thread-a", TaskJobIsolationAcceptRequest{JobID: accept.ID, ApprovalID: "forged-approval"}),
		httpService.ConflictReportTaskJobIsolation("thread-a", TaskJobIsolationConflictReportRequest{JobID: conflict.ID}),
		httpService.RepairCheckTaskJobIsolation("thread-a", TaskJobIsolationRepairCheckRequest{JobID: repairCheck.ID, ConflictReportID: "forged-report", RepairPatch: "PRIVATE PATCH"}),
		httpService.RepairAcceptTaskJobIsolation("thread-a", TaskJobIsolationRepairAcceptRequest{JobID: repairAccept.ID, RepairReviewID: "forged-review", ApprovalID: "forged-approval"}),
	}
	for _, result := range results {
		assertWorktreeIsolationQuarantined(t, result)
		encoded, err := json.Marshal(result.Response)
		if err != nil {
			t.Fatalf("encode fixed response: %v", err)
		}
		if strings.Contains(string(encoded), "forged") || strings.Contains(string(encoded), "PRIVATE") {
			t.Fatalf("fixed response reflected untrusted mutation input: %s", encoded)
		}
	}
	if resolverCalls != 0 {
		t.Fatalf("worktree authority blocker called parent workspace resolver %d times", resolverCalls)
	}
}

func TestTaskJobHTTPRecoveryDoesNotReflectCallerReasonOrPersistRawError(t *testing.T) {
	const sentinel = "PRIVATE_RECOVERY_REASON account 6222020202020202020"
	record := domainjob.Record{
		ID: "job-stale-recovery", Kind: "background-shell", ParentThreadID: "thread-a", ParentTurnID: "turn-a",
		Status: string(domainjob.StatusRunning), Background: true, StaleAfterMs: 1,
		LastHeartbeatAt: "2020-01-01T00:00:00Z", UpdatedAt: "2020-01-01T00:00:00Z",
	}
	repo := newTaskJobRepoStub([]domainjob.Record{record})
	httpJobs := taskJobHTTPStoreStub{taskJobRepoStub: repo}
	var lifecycleMessage string
	httpService := NewTaskJobHTTPService(TaskJobHTTPServiceDeps{
		Service: NewService(Dependencies{Jobs: repo}),
		Jobs:    httpJobs,
		RecordLifecycle: func(_ domainjob.Record, _ string, message string) {
			lifecycleMessage = message
		},
	})

	result := httpService.RecoverTaskJob("thread-a", TaskJobRecoverRequest{JobID: record.ID, Reason: sentinel})
	encoded, err := json.Marshal(result.Response)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || result.Record.Status != string(domainjob.StatusInterrupted) || result.Record.Error != "" ||
		result.Record.FailureCode != domainjob.FailureChildInterrupted {
		t.Fatalf("stale recovery did not persist a closed terminal state: %#v", result)
	}
	if lifecycleMessage != domainjob.FailureChildInterrupted || strings.Contains(string(encoded), sentinel) ||
		result.Response["reason"] != domainjob.OperationalReasonWithheld {
		t.Fatalf("recovery reflected caller-controlled failure text: response=%s lifecycle=%q", encoded, lifecycleMessage)
	}
}

type taskJobRepoStub struct {
	mu      sync.Mutex
	records map[string]domainjob.Record
}

type taskJobHTTPStoreStub struct {
	*taskJobRepoStub
}

func (taskJobHTTPStoreStub) QueueSteerMessage(string, domainjob.SteerQueueAuthorityV1, domainjob.SteerMessage) (domainjob.Record, domainjob.SteerMessage, error) {
	return domainjob.Record{}, domainjob.SteerMessage{}, errors.New("unexpected steer queue")
}

func (taskJobHTTPStoreStub) SettleSteerPromotionExact(string, domainjob.SteerMessage, domainjob.SteerPromotionSettlementV1) (domainjob.Record, domainjob.SteerMessage, error) {
	return domainjob.Record{}, domainjob.SteerMessage{}, errors.New("unexpected steer settlement")
}

func (taskJobHTTPStoreStub) RejectSteerMessage(string, domainjob.SteerMessage, string) (domainjob.Record, domainjob.SteerMessage, error) {
	return domainjob.Record{}, domainjob.SteerMessage{}, errors.New("unexpected steer rejection")
}

type failingResumePauseRuntime struct {
	snapshot    BackgroundJobPauseSnapshot
	resumeCalls int
}

func (r *failingResumePauseRuntime) RequestBackgroundJobPause(string, string, time.Time) (BackgroundJobPauseSnapshot, bool) {
	return BackgroundJobPauseSnapshot{}, false
}

func (r *failingResumePauseRuntime) ClearBackgroundJobPauseRequest(string, string) bool {
	return false
}

func (r *failingResumePauseRuntime) BackgroundJobPauseSnapshot(string) (BackgroundJobPauseSnapshot, bool) {
	return r.snapshot, true
}

func (r *failingResumePauseRuntime) ResumeBackgroundJob(string, string) (BackgroundJobPauseSnapshot, bool) {
	r.resumeCalls++
	return BackgroundJobPauseSnapshot{}, false
}

func newTaskJobRepoStub(records []domainjob.Record) *taskJobRepoStub {
	out := &taskJobRepoStub{records: map[string]domainjob.Record{}}
	for _, record := range records {
		record.PauseState = taskJobStubPauseState(record)
		out.records[record.ID] = record
	}
	return out
}

func (s *taskJobRepoStub) StartChildRun(request domainjob.StartRequest) (domainjob.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record := domainjob.Record{
		ID:                    request.ParentThreadID + "-job",
		ParentGoalID:          request.ParentGoalID,
		ParentThreadID:        request.ParentThreadID,
		ParentTurnID:          request.ParentTurnID,
		ParentToolItemID:      request.ParentToolItemID,
		ParentToolCallID:      request.ParentToolCallID,
		ChildThreadID:         request.ChildThreadID,
		ChildTurnID:           request.ChildTurnID,
		Kind:                  request.Kind,
		Status:                request.Status,
		Output:                request.Output,
		FailureCode:           domainjob.NormalizeFailureCode(request.FailureCode, request.Status, strings.TrimSpace(request.Error) != ""),
		Usage:                 cloneTaskJobStubUsage(request.Usage),
		ToolInvocations:       request.ToolInvocations,
		Background:            request.Background,
		ProfileName:           request.ProfileName,
		ProfileSource:         request.ProfileSource,
		ToolPolicy:            request.ToolPolicy,
		ProfileMode:           request.ProfileMode,
		ReturnFormat:          request.ReturnFormat,
		QueuedAt:              request.QueuedAt,
		DefaultModelInherited: request.DefaultModelInherited,
	}
	record.PauseState = taskJobStubPauseState(record)
	s.records[record.ID] = record
	return record, nil
}

func (s *taskJobRepoStub) LoadChildRun(id string) (domainjob.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[id]
	if !ok {
		return domainjob.Record{}, errTaskJobRepoStubNotFound{}
	}
	return record, nil
}

func (s *taskJobRepoStub) UpdateChildRun(id string, request domainjob.UpdateRequest) (domainjob.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[id]
	if !ok {
		return domainjob.Record{}, errTaskJobRepoStubNotFound{}
	}
	if request.Status != "" {
		record.Status = request.Status
	}
	if request.Output != "" {
		record.Output = request.Output
	}
	record.Error = ""
	if request.FailureCode != "" {
		record.FailureCode = request.FailureCode
	}
	if request.ChildThreadID != "" {
		record.ChildThreadID = request.ChildThreadID
	}
	if request.ChildTurnID != "" {
		record.ChildTurnID = request.ChildTurnID
	}
	if request.Workspace != "" {
		record.Workspace = request.Workspace
	}
	if request.Usage != nil {
		record.Usage = cloneTaskJobStubUsage(request.Usage)
	}
	if request.ToolInvocations != nil {
		record.ToolInvocations = *request.ToolInvocations
	}
	if request.Isolation != nil {
		record.IsolationMode = strings.TrimSpace(firstNonEmptyAnyString(request.Isolation.IsolationMode, record.IsolationMode))
		record.WorktreePath = strings.TrimSpace(firstNonEmptyAnyString(request.Isolation.WorktreePath, record.WorktreePath))
		record.WorktreeBranch = strings.TrimSpace(firstNonEmptyAnyString(request.Isolation.WorktreeBranch, record.WorktreeBranch))
		record.BaseCommit = strings.TrimSpace(firstNonEmptyAnyString(request.Isolation.BaseCommit, record.BaseCommit))
		record.CurrentCommit = strings.TrimSpace(firstNonEmptyAnyString(request.Isolation.CurrentCommit, record.CurrentCommit))
		record.DiffSummary = strings.TrimSpace(firstNonEmptyAnyString(request.Isolation.DiffSummary, record.DiffSummary))
		record.MergeStatus = strings.TrimSpace(firstNonEmptyAnyString(request.Isolation.MergeStatus, record.MergeStatus))
		if request.Isolation.ChangedFiles != nil {
			record.ChangedFiles = append([]domainjob.ChangedFile(nil), request.Isolation.ChangedFiles...)
		}
	}
	if request.MergeDecision != nil {
		record.MergeDecisions = append(append([]domainjob.MergeDecision(nil), record.MergeDecisions...), *request.MergeDecision)
	}
	if request.CleanupReceipt != nil {
		record.CleanupReceipts = append(append([]domainjob.CleanupReceipt(nil), record.CleanupReceipts...), *request.CleanupReceipt)
	}
	if request.AcceptDecision != nil {
		record.AcceptDecisions = append(append([]domainjob.AcceptDecision(nil), record.AcceptDecisions...), *request.AcceptDecision)
	}
	if request.ConflictReport != nil {
		record.ConflictReports = append(append([]domainjob.ConflictReport(nil), record.ConflictReports...), *request.ConflictReport)
	}
	if request.RepairPatchReview != nil {
		record.RepairPatchReviews = append(append([]domainjob.RepairPatchReview(nil), record.RepairPatchReviews...), *request.RepairPatchReview)
	}
	if request.RepairDecision != nil {
		record.RepairDecisions = append(append([]domainjob.RepairDecision(nil), record.RepairDecisions...), *request.RepairDecision)
	}
	if request.ChildTodoList != nil {
		record.ChildTodoLists = appendOrReplaceTaskJobStubChildTodoList(record.ChildTodoLists, *request.ChildTodoList)
	}
	if request.ChildTodoProjection != nil {
		record.ChildTodoProjections = appendOrReplaceTaskJobStubChildTodoProjection(record.ChildTodoProjections, *request.ChildTodoProjection)
	}
	if request.ProjectionDecision != nil {
		record.ProjectionDecisions = appendOrReplaceTaskJobStubProjectionDecision(record.ProjectionDecisions, *request.ProjectionDecision)
	}
	if taskJobStubTerminalStatus(record.Status) {
		record.PauseRequests = expireTaskJobStubPauseRequests(record.PauseRequests, "child run terminal: "+record.Status)
	}
	record.PauseState = taskJobStubPauseState(record)
	s.records[id] = record
	return record, nil
}

func (s *taskJobRepoStub) AddPauseRequest(id string, request domainjob.PauseRequest) (domainjob.Record, domainjob.PauseRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[id]
	if !ok {
		return domainjob.Record{}, domainjob.PauseRequest{}, errTaskJobRepoStubNotFound{}
	}
	for _, existing := range record.PauseRequests {
		if existing.ID == request.ID {
			return record, existing, nil
		}
	}
	if request.Status == "" {
		request.Status = "requested"
	}
	if request.ChildRunID == "" {
		request.ChildRunID = record.ID
	}
	if request.JobID == "" {
		request.JobID = record.ID
	}
	record.PauseRequests = append(append([]domainjob.PauseRequest(nil), record.PauseRequests...), request)
	switch request.Status {
	case "requested":
		record.Status = string(domainjob.StatusPauseRequested)
	case "paused":
		record.Status = string(domainjob.StatusPaused)
	case "resumed":
		record.Status = string(domainjob.StatusResumeRequested)
	}
	record.PauseState = taskJobStubPauseState(record)
	s.records[id] = record
	return record, request, nil
}

func (s *taskJobRepoStub) MarkPauseRequestPaused(id string, requestID string, pausedAt string, token domainjob.ResumeToken) (domainjob.Record, domainjob.PauseRequest, error) {
	return s.updatePauseRequest(id, requestID, "paused", pausedAt, "", token)
}

func (s *taskJobRepoStub) MarkPauseResumeRequested(id string, requestID string) (domainjob.Record, domainjob.PauseRequest, error) {
	return s.transitionPauseMainStatus(id, requestID, string(domainjob.StatusPaused), string(domainjob.StatusResumeRequested), "paused")
}

func (s *taskJobRepoStub) RollbackPauseResumeRequested(id string, requestID string) (domainjob.Record, domainjob.PauseRequest, error) {
	return s.transitionPauseMainStatus(id, requestID, string(domainjob.StatusResumeRequested), string(domainjob.StatusPaused), "paused")
}

func (s *taskJobRepoStub) MarkPauseRequestResumed(id string, requestID string, resumedAt string) (domainjob.Record, domainjob.PauseRequest, error) {
	return s.updatePauseRequest(id, requestID, "resumed", resumedAt, "", domainjob.ResumeToken{})
}

func (s *taskJobRepoStub) CompletePauseResume(id string, requestID string) (domainjob.Record, domainjob.PauseRequest, error) {
	return s.transitionPauseMainStatus(id, requestID, string(domainjob.StatusResuming), string(domainjob.StatusRunning), "resumed")
}

func (s *taskJobRepoStub) transitionPauseMainStatus(id string, requestID string, fromStatus string, toStatus string, requestStatus string) (domainjob.Record, domainjob.PauseRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[id]
	if !ok || strings.TrimSpace(record.Status) != fromStatus {
		return record, domainjob.PauseRequest{}, errTaskJobRepoStubNotFound{}
	}
	for _, request := range record.PauseRequests {
		if request.ID != requestID || request.Status != requestStatus {
			continue
		}
		record.Status = toStatus
		record.PauseState = taskJobStubPauseState(record)
		s.records[id] = record
		return record, request, nil
	}
	return record, domainjob.PauseRequest{}, errTaskJobRepoStubNotFound{}
}

func (s *taskJobRepoStub) RejectPauseRequest(id string, request domainjob.PauseRequest, reason string) (domainjob.Record, domainjob.PauseRequest, error) {
	request.Status = "rejected"
	request.RejectedReason = reason
	return s.AddPauseRequest(id, request)
}

func (s *taskJobRepoStub) ExpirePauseRequest(id string, requestID string, expiredAt string, reason string) (domainjob.Record, domainjob.PauseRequest, error) {
	return s.updatePauseRequest(id, requestID, "expired", expiredAt, reason, domainjob.ResumeToken{})
}

func (s *taskJobRepoStub) updatePauseRequest(id string, requestID string, status string, at string, reason string, token domainjob.ResumeToken) (domainjob.Record, domainjob.PauseRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[id]
	if !ok {
		return domainjob.Record{}, domainjob.PauseRequest{}, errTaskJobRepoStubNotFound{}
	}
	requests := append([]domainjob.PauseRequest(nil), record.PauseRequests...)
	for index := range requests {
		if requests[index].ID != requestID {
			continue
		}
		requests[index].Status = status
		switch status {
		case "paused":
			requests[index].PausedAt = at
			requests[index].ResumeToken = token.ResumeToken
			requests[index].ResumeTokenIssuedAt = token.IssuedAt
			requests[index].ResumeTokenExpiresAt = token.ExpiresAt
			record.Status = string(domainjob.StatusPaused)
		case "resumed":
			requests[index].ResumedAt = at
			record.Status = string(domainjob.StatusResuming)
		case "expired":
			requests[index].RejectedReason = reason
			record.Status = string(domainjob.StatusRunning)
		}
		record.PauseRequests = requests
		record.PauseState = taskJobStubPauseState(record)
		s.records[id] = record
		return record, requests[index], nil
	}
	return record, domainjob.PauseRequest{}, errTaskJobRepoStubNotFound{}
}

func (s *taskJobRepoStub) List(parentThreadID string) ([]domainjob.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domainjob.Record{}
	for _, record := range s.records {
		if record.ParentThreadID == parentThreadID {
			out = append(out, record)
		}
	}
	return out, nil
}

func taskJobStubPauseState(record domainjob.Record) domainjob.ChildRunPauseState {
	state := domainjob.ChildRunPauseState{}
	for _, request := range record.PauseRequests {
		switch strings.TrimSpace(request.Status) {
		case "requested", "paused", "resumed":
			state.PauseRequestID = request.ID
			state.Status = request.Status
		}
		if request.RequestedAt > state.RequestedAt {
			state.RequestedAt = request.RequestedAt
		}
		if request.PausedAt > state.LastPausedAt {
			state.LastPausedAt = request.PausedAt
		}
		if request.ResumedAt > state.LastResumedAt {
			state.LastResumedAt = request.ResumedAt
		}
		if request.ResumeTokenIssuedAt > state.ResumeTokenIssuedAt {
			state.ResumeTokenIssuedAt = request.ResumeTokenIssuedAt
		}
		if request.ResumeTokenExpiresAt > state.ResumeTokenExpiresAt {
			state.ResumeTokenExpiresAt = request.ResumeTokenExpiresAt
		}
		if request.PausedAt != "" {
			state.PauseCount++
		}
	}
	status := strings.TrimSpace(record.Status)
	state.Paused = status == string(domainjob.StatusPaused)
	state.CanPause = record.Background && !taskJobStubTerminalStatus(status) && strings.TrimSpace(record.ChildThreadID) != "" &&
		(status == string(domainjob.StatusQueued) || status == string(domainjob.StatusRunning))
	state.CanResume = record.Background && !taskJobStubTerminalStatus(status) && strings.TrimSpace(record.ChildThreadID) != "" &&
		status == string(domainjob.StatusPaused)
	return state
}

func taskJobStubTerminalStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "completed", "failed", "aborted", "interrupted", "killed", "canceled", "timeout":
		return true
	default:
		return false
	}
}

func expireTaskJobStubPauseRequests(values []domainjob.PauseRequest, reason string) []domainjob.PauseRequest {
	out := append([]domainjob.PauseRequest(nil), values...)
	for index := range out {
		switch strings.TrimSpace(out[index].Status) {
		case "requested", "paused":
			out[index].Status = "expired"
			out[index].RejectedReason = reason
		}
	}
	return out
}

func cloneTaskJobStubUsage(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

type childTodoStoreStub struct {
	mu    sync.Mutex
	todos map[string]map[string]any
}

func newChildTodoStoreStub(todos map[string]map[string]any) *childTodoStoreStub {
	return &childTodoStoreStub{todos: todos}
}

func (s *childTodoStoreStub) GetTodos(threadID string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneTaskJobStubAnyMap(s.todos[threadID]), nil
}

func (s *childTodoStoreStub) SetTodos(threadID string, items []any) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := cloneTaskJobStubAnyMap(s.todos[threadID])
	if current == nil {
		current = map[string]any{"threadId": threadID}
	}
	current["threadId"] = threadID
	current["items"] = cloneTaskJobStubAnyList(items)
	current["updatedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	s.todos[threadID] = current
	return cloneTaskJobStubAnyMap(current), nil
}

func (s *childTodoStoreStub) clone(threadID string) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneTaskJobStubAnyMap(s.todos[threadID])
}

func cloneTaskJobStubAnyList(values []any) []any {
	out := make([]any, 0, len(values))
	for _, item := range values {
		switch typed := item.(type) {
		case map[string]any:
			out = append(out, cloneTaskJobStubAnyMap(typed))
		default:
			out = append(out, item)
		}
	}
	return out
}

func cloneTaskJobStubAnyMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		switch typed := item.(type) {
		case []any:
			next := make([]any, len(typed))
			copy(next, typed)
			out[key] = next
		case map[string]any:
			out[key] = cloneTaskJobStubAnyMap(typed)
		default:
			out[key] = item
		}
	}
	return out
}

func appendOrReplaceTaskJobStubChildTodoList(values []domainjob.ChildTodoList, value domainjob.ChildTodoList) []domainjob.ChildTodoList {
	out := append([]domainjob.ChildTodoList(nil), values...)
	for index := range out {
		if out[index].ID == value.ID {
			out[index] = value
			return out
		}
	}
	return append(out, value)
}

func appendOrReplaceTaskJobStubChildTodoProjection(values []domainjob.ChildTodoProjection, value domainjob.ChildTodoProjection) []domainjob.ChildTodoProjection {
	out := append([]domainjob.ChildTodoProjection(nil), values...)
	for index := range out {
		if out[index].ID == value.ID {
			out[index] = value
			return out
		}
	}
	return append(out, value)
}

func appendOrReplaceTaskJobStubProjectionDecision(values []domainjob.ProjectionDecision, value domainjob.ProjectionDecision) []domainjob.ProjectionDecision {
	out := append([]domainjob.ProjectionDecision(nil), values...)
	for index := range out {
		if out[index].ID == value.ID {
			out[index] = value
			return out
		}
	}
	return append(out, value)
}

type taskJobEventRecorderStub struct {
	mu     sync.Mutex
	events []map[string]any
}

func (s *taskJobEventRecorderStub) RecordEvent(event map[string]any) (map[string]any, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, cloneTaskJobStubAnyMap(event))
	return cloneTaskJobStubAnyMap(event), nil, nil
}

func (s *taskJobEventRecorderStub) count(kind string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, event := range s.events {
		if stringValue(event, "kind") == kind {
			count++
		}
	}
	return count
}

func sameMapValue(first map[string]any, second map[string]any) bool {
	return reflect.DeepEqual(first, second)
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	default:
		return 0
	}
}

type errTaskJobRepoStubNotFound struct{}

func (errTaskJobRepoStubNotFound) Error() string {
	return "not found"
}

type taskJobReviewWorktreeManager struct {
	isolation       domainjob.WorktreeIsolation
	receipt         domainjob.CleanupReceipt
	acceptIsolation domainjob.WorktreeIsolation
	acceptDecision  domainjob.AcceptDecision
	conflictReport  domainjob.ConflictReport
	repairIsolation domainjob.WorktreeIsolation
	repairReview    domainjob.RepairPatchReview
	repairDecision  domainjob.RepairDecision
	err             error
	calls           int
}

func (m *taskJobReviewWorktreeManager) CreateSubagentWorktree(context.Context, ports.WorktreeCreateRequest) (domainjob.WorktreeIsolation, error) {
	return domainjob.WorktreeIsolation{}, nil
}

func (m *taskJobReviewWorktreeManager) SummarizeSubagentWorktree(context.Context, domainjob.WorktreeIsolation) (domainjob.WorktreeIsolation, error) {
	m.calls++
	if m.err != nil {
		return domainjob.WorktreeIsolation{}, m.err
	}
	return m.isolation, nil
}

func (m *taskJobReviewWorktreeManager) CleanupSubagentWorktree(context.Context, domainjob.WorktreeIsolation) (domainjob.CleanupReceipt, error) {
	m.calls++
	if m.err != nil {
		return domainjob.CleanupReceipt{}, m.err
	}
	return m.receipt, nil
}

func (m *taskJobReviewWorktreeManager) AcceptSubagentWorktree(context.Context, ports.WorktreeAcceptRequest) (domainjob.WorktreeIsolation, domainjob.AcceptDecision, error) {
	m.calls++
	if m.err != nil {
		return domainjob.WorktreeIsolation{}, domainjob.AcceptDecision{}, m.err
	}
	return m.acceptIsolation, m.acceptDecision, nil
}

func (m *taskJobReviewWorktreeManager) ReportSubagentWorktreeConflict(context.Context, ports.WorktreeConflictReportRequest) (domainjob.WorktreeIsolation, domainjob.ConflictReport, error) {
	m.calls++
	if m.err != nil {
		return domainjob.WorktreeIsolation{}, domainjob.ConflictReport{}, m.err
	}
	return m.repairIsolation, m.conflictReport, nil
}

func (m *taskJobReviewWorktreeManager) CheckSubagentRepairPatch(context.Context, ports.WorktreeRepairCheckRequest) (domainjob.WorktreeIsolation, domainjob.RepairPatchReview, error) {
	m.calls++
	if m.err != nil {
		return domainjob.WorktreeIsolation{}, domainjob.RepairPatchReview{}, m.err
	}
	return m.repairIsolation, m.repairReview, nil
}

func (m *taskJobReviewWorktreeManager) AcceptSubagentRepairPatch(context.Context, ports.WorktreeRepairAcceptRequest) (domainjob.WorktreeIsolation, domainjob.RepairDecision, error) {
	m.calls++
	if m.err != nil {
		return domainjob.WorktreeIsolation{}, domainjob.RepairDecision{}, m.err
	}
	return m.repairIsolation, m.repairDecision, nil
}

func assertWorktreeIsolationQuarantined(t *testing.T, result TaskJobServiceResult) {
	t.Helper()
	if !result.IsError || result.ErrorCode != TaskJobErrorWorktreeIsolationAuthorityRequired ||
		stringValue(result.Response, "code") != "worktree_isolation_authority_required" ||
		stringValue(result.Response, "message") != "Worktree isolation controls require host-issued durable authority." ||
		len(result.Response) != 2 {
		t.Fatalf("worktree isolation operation was not quarantined: %#v", result)
	}
}

func taskJobReviewedIsolationRecord(jobID, parentThreadID string) domainjob.Record {
	return domainjob.Record{
		ID:             jobID,
		ParentThreadID: parentThreadID,
		Kind:           "subagent",
		Status:         string(domainjob.StatusCompleted),
		ChildThreadID:  "child-" + jobID,
		Background:     true,
		IsolationMode:  "worktree",
		WorktreePath:   "/tmp/analytix/subagent-worktrees/" + parentThreadID + "/" + jobID,
		WorktreeBranch: "codex/subagent/" + parentThreadID + "/" + jobID,
		BaseCommit:     "base",
		CurrentCommit:  "current",
		MergeStatus:    "review_requested",
		ChangedFiles:   []domainjob.ChangedFile{{Path: "src/app.go", Status: "M"}},
	}
}
