package subagent

import (
	"context"
	"sync"
	"testing"
	"time"

	effectgateapp "analytix.local/runtime-go/internal/app/effectgate"
)

type childAdmissionWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (ctx *childAdmissionWaitContext) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.waiting) })
	return ctx.Context.Done()
}

func TestBoundChildAdmissionWaitsForSiblingTransitionAndRevalidates(t *testing.T) {
	for _, disposition := range []string{"commit", "abort", "changed_workspace", "cancel", "shutdown"} {
		t.Run(disposition, func(t *testing.T) {
			state := NewRuntimeState()
			parent, binding := runtimeStateSecurityFixture(t, "parent", "parent_turn", "case", "snapshot", 1)
			if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), parent, time.Second); err != nil {
				t.Fatal(err)
			}
			effectCtx, release, err := state.AcquireContextEffect(context.Background(), parent)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			transition, err := state.BeginDelegatedChildSecurityScopeTransition(effectCtx, effectgateapp.TransitionScope{
				ThreadID: "left_child", WorkspaceRealPath: parent.WorkspaceRealPath, TenantID: parent.TenantID, UserID: parent.UserID,
			}, parent.ContextDigest)
			if err != nil {
				t.Fatal(err)
			}
			defer transition.Abort()
			base, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx := &childAdmissionWaitContext{Context: base, waiting: make(chan struct{})}
			type result struct {
				control *BoundChildAdmission
				err     error
			}
			done := make(chan result, 1)
			go func() {
				control, _, err := BeginBoundChildAdmission(ctx, true, state, "right_job", binding)
				done <- result{control, err}
			}()
			select {
			case <-ctx.waiting:
			case got := <-done:
				if got.control != nil {
					got.control.Close()
				}
				t.Fatalf("sibling admission did not wait for the active transition: %v", got.err)
			case <-time.After(time.Second):
				t.Fatal("sibling admission did not reach its wait boundary")
			}
			select {
			case got := <-done:
				if got.control != nil {
					got.control.Close()
				}
				t.Fatalf("sibling admitted before transition closure: %v", got.err)
			default:
			}
			switch disposition {
			case "abort":
				transition.Abort()
			case "cancel":
				cancel()
			case "shutdown":
				if _, _, err := state.CancelBackgroundJobsAndWait(time.Second); err != nil {
					t.Fatal(err)
				}
			default:
				snapshot := "snapshot"
				if disposition == "changed_workspace" {
					snapshot = "new_snapshot"
				}
				child, _ := runtimeStateSecurityFixture(t, "left_child", "left_turn", "case", snapshot, 1)
				if err := transition.Prepare(context.Background(), child, time.Second); err != nil {
					t.Fatal(err)
				}
				if err := transition.Commit(); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case got := <-done:
				if got.control != nil {
					defer got.control.Close()
				}
				wantSuccess := disposition == "commit" || disposition == "abort"
				if (got.err == nil && got.control != nil) != wantSuccess {
					t.Fatalf("admission after %s: %v", disposition, got.err)
				}
				if !wantSuccess && len(state.ActiveBackgroundJobIDs()) != 0 {
					t.Fatal("rejected admission left a background control")
				}
			case <-time.After(time.Second):
				t.Fatal("transition closure or cancellation did not settle admission")
			}
		})
	}
}

func TestBoundChildAdmissionStillRejectsOrdinaryAuthorityTransitions(t *testing.T) {
	for _, threadID := range []string{"parent", "other_thread"} {
		t.Run(threadID, func(t *testing.T) {
			state := NewRuntimeState()
			parent, binding := runtimeStateSecurityFixture(t, "parent", "parent_turn", "case", "snapshot", 1)
			if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), parent, time.Second); err != nil {
				t.Fatal(err)
			}
			transition, err := state.BeginSecurityScopeTransition(context.Background(), effectgateapp.TransitionScope{
				ThreadID: threadID, WorkspaceRealPath: parent.WorkspaceRealPath, TenantID: parent.TenantID, UserID: parent.UserID,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer transition.Abort()
			control, _, err := BeginBoundChildAdmission(context.Background(), true, state, "right_job", binding)
			if control != nil {
				control.Close()
			}
			if err == nil || len(state.ActiveBackgroundJobIDs()) != 0 {
				t.Fatal("ordinary authority transition admitted a new child")
			}
		})
	}
}
