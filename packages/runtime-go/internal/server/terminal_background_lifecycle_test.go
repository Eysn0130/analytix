package server

import (
	"context"
	"reflect"
	"testing"

	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	"analytix.local/runtime-go/internal/jobs"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestRuntimeBashTerminalProgressCommitsCompleteReplayableLifecycle(t *testing.T) {
	for _, status := range []string{"completed", "killed", "failed", "timeout", "interrupted"} {
		t.Run(status, func(t *testing.T) {
			root, jobRoot := t.TempDir(), t.TempDir()
			store, err := NewTempDurableEventSessionStore(root)
			if err != nil {
				t.Fatal(err)
			}
			workspace := workspacetest.New(t)
			thread, err := store.CreateThread(map[string]any{"title": "Background shell lifecycle"}, workspace)
			if err != nil {
				t.Fatal(err)
			}
			threadID, turnID := stringField(thread, "id"), "turn_bash_lifecycle"
			parent := appendServerBoundJobParent(t, store, threadID, turnID, workspace, "background-shell")
			manager, err := jobs.NewManager(jobRoot)
			if err != nil {
				t.Fatal(err)
			}
			record, err := manager.StartChildRun(jobs.StartRequest{
				ParentGoalID:   "goal_bash_lifecycle",
				ParentThreadID: threadID, ParentTurnID: turnID,
				ParentToolItemID: parent.ItemID, ParentToolCallID: parent.CallID,
				Kind: "background-shell", Label: "Background shell", Status: status,
				Background: true, SecurityBinding: parent.Binding,
			})
			if err != nil {
				t.Fatal(err)
			}
			handler := &runtimeServerHandler{store: store, jobs: manager}
			pending := runtimePendingToolCall{
				ThreadID: threadID, TurnID: turnID, ToolCallItemID: parent.ItemID,
				Call: domainmodel.ToolCall{ID: parent.CallID, Name: "bash"},
			}
			handler.runtimeBashToolCallbacks(pending).OnBackgroundProgress(record, status, "private shell diagnostic")
			current, err := manager.LoadChildRun(record.ID)
			if err != nil {
				t.Fatal(err)
			}
			callbackReplay, err := store.LoadEventsSince(threadID, 0)
			if err != nil {
				t.Fatal(err)
			}
			assertCanonicalAdmittedBackgroundLifecycleEvents(t, callbackReplay.Events, current, "private shell diagnostic")
			// Neither an old caller record nor a delayed output label may rewrite
			// the terminal fact already committed by the job authority.
			stale := record
			stale.Status = "running"
			handler.runtimeBashToolCallbacks(pending).OnBackgroundProgress(stale, status, "stale diagnostic")
			handler.runtimeBashToolCallbacks(pending).OnBackgroundProgress(record, "running", "late output")
			result, err := subagentapp.RecordBackgroundJobLifecycleV1(handler.runtimeBackgroundDeliveryService(), store, record)
			if err != nil || result.Blocker != "" {
				t.Fatalf("terminal progress conflicts with canonical lifecycle: blocker=%q err=%v", result.Blocker, err)
			}
			before, err := store.LoadEventsSince(threadID, 0)
			if err != nil {
				t.Fatal(err)
			}
			assertCanonicalAdmittedBackgroundLifecycleEvents(t, before.Events, result.Record, "private shell diagnostic")
			if !reflect.DeepEqual(callbackReplay.Events, before.Events) {
				t.Fatal("duplicate or stale callback changed the committed terminal lifecycle")
			}
			if err := handler.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
			// Reload both persisted authorities, then replay the same terminal fact.
			reloaded, err := NewTempDurableEventSessionStore(root)
			if err != nil {
				t.Fatal(err)
			}
			manager, err = jobs.NewManager(jobRoot)
			if err != nil {
				t.Fatal(err)
			}
			restarted := &runtimeServerHandler{store: reloaded, jobs: manager}
			result, err = subagentapp.RecordBackgroundJobLifecycleV1(restarted.runtimeBackgroundDeliveryService(), reloaded, record)
			if err != nil || result.Blocker != "" {
				t.Fatalf("reloaded terminal lifecycle was not exact: blocker=%q err=%v", result.Blocker, err)
			}
			after, err := reloaded.LoadEventsSince(threadID, 0)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before.Events, after.Events) {
				t.Fatal("terminal lifecycle replay changed committed events")
			}
		})
	}
}
