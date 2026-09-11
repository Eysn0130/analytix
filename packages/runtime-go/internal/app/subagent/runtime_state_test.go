package subagent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestRuntimeStatePauseBlocksUntilResume(t *testing.T) {
	state := NewRuntimeState()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state.RegisterBackgroundJob("job_pause", cancel)
	defer state.UnregisterBackgroundJob("job_pause")

	paused := make(chan BackgroundJobPauseSnapshot, 1)
	resumed := make(chan BackgroundJobPauseSnapshot, 1)
	done := make(chan error, 1)
	if _, ok := state.RequestBackgroundJobPause("job_pause", "pause_1", time.Now().UTC()); !ok {
		t.Fatal("pause request should target active job")
	}
	go func() {
		_, err := state.WaitIfBackgroundJobPauseRequested(ctx, "job_pause", BackgroundJobPauseCallbacks{
			OnPaused: func(snapshot BackgroundJobPauseSnapshot) error {
				paused <- snapshot
				return nil
			},
			OnResumed: func(snapshot BackgroundJobPauseSnapshot) error {
				resumed <- snapshot
				return nil
			},
		})
		done <- err
	}()

	select {
	case snapshot := <-paused:
		if snapshot.PauseRequestID != "pause_1" || snapshot.Status != "paused" {
			t.Fatalf("paused snapshot mismatch: %#v", snapshot)
		}
	case <-time.After(time.Second):
		t.Fatal("pause was not admitted at safe boundary")
	}
	select {
	case err := <-done:
		t.Fatalf("pause should block until resume, got %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	if _, ok := state.ResumeBackgroundJob("job_pause", "pause_1"); !ok {
		t.Fatal("resume should unblock paused job")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("wait should finish cleanly after resume: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("resume did not unblock pause")
	}
	select {
	case snapshot := <-resumed:
		if snapshot.PauseRequestID != "pause_1" || snapshot.Status != "resuming" {
			t.Fatalf("resumed snapshot mismatch: %#v", snapshot)
		}
	default:
		t.Fatal("resume callback was not called")
	}
}

func TestRuntimeStateKillPausedJobDoesNotResume(t *testing.T) {
	state := NewRuntimeState()
	ctx, cancel := context.WithCancel(context.Background())
	state.RegisterBackgroundJob("job_pause", cancel)
	defer state.UnregisterBackgroundJob("job_pause")

	resumed := false
	done := make(chan error, 1)
	_, _ = state.RequestBackgroundJobPause("job_pause", "pause_1", time.Now().UTC())
	go func() {
		_, err := state.WaitIfBackgroundJobPauseRequested(ctx, "job_pause", BackgroundJobPauseCallbacks{
			OnResumed: func(BackgroundJobPauseSnapshot) error {
				resumed = true
				return nil
			},
		})
		done <- err
	}()
	time.Sleep(25 * time.Millisecond)
	if !state.CancelBackgroundJob("job_pause") {
		t.Fatal("cancel should find active job")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("kill should return context canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("kill did not unblock paused job")
	}
	if resumed {
		t.Fatal("kill must not run resume callback")
	}
}

func TestRuntimeStateCaseSwitchCancelsAndWaitsForBoundJob(t *testing.T) {
	state := NewRuntimeState()
	contextA, binding := runtimeStateSecurityFixture(t, "thread_a", "turn_a", "case_a", "snapshot_a", 1)
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), contextA, time.Second); err != nil {
		t.Fatal(err)
	}
	jobCtx, cancel := context.WithCancel(context.Background())
	if !state.RegisterBoundBackgroundJob("job_case_a", binding, cancel) {
		t.Fatal("current case job registration was rejected")
	}
	stopped := make(chan struct{})
	go func() {
		<-jobCtx.Done()
		state.UnregisterBackgroundJob("job_case_a")
		close(stopped)
	}()
	contextB, _ := runtimeStateSecurityFixture(t, "thread_a", "turn_b", "case_b", "snapshot_b", 2)
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), contextB, time.Second); err != nil {
		t.Fatalf("case switch did not wait for old job: %v", err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("new case authority returned before old job stopped")
	}
}

func TestRuntimeStateNewTurnCancelsOldExactTurnMutator(t *testing.T) {
	state := NewRuntimeState()
	contextA, binding := runtimeStateSecurityFixture(t, "thread_a", "turn_a", "case_a", "snapshot_a", 3)
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), contextA, time.Second); err != nil {
		t.Fatal(err)
	}
	jobCtx, cancel := context.WithCancel(context.Background())
	if !state.RegisterBoundBackgroundJob("job_turn_a", binding, cancel) {
		t.Fatal("old turn mutator registration was rejected")
	}
	stopped := make(chan struct{})
	go func() {
		<-jobCtx.Done()
		state.UnregisterBackgroundJob("job_turn_a")
		close(stopped)
	}()
	contextB := runtimeStateDerivedContextV2(t, contextA, contextA.ThreadID, "turn_b", contextA.ContextEpoch)
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), contextB, time.Second); err != nil {
		t.Fatalf("new exact turn did not cancel the old mutator: %v", err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("new turn authority returned before old exact-turn mutator stopped")
	}
}

func TestCancelBoundBackgroundJobsForParentThreadAndWaitIsPrincipalScoped(t *testing.T) {
	state := NewRuntimeState()
	contextA, bindingA := runtimeStateSecurityFixtureForPrincipal(
		t, "tenant-a", "user-a", "shared-thread", "turn-a", "case-a", "snapshot-a", 1,
	)
	contextB, bindingB := runtimeStateSecurityFixtureForPrincipal(
		t, "tenant-b", "user-b", "shared-thread", "turn-b", "case-b", "snapshot-b", 1,
	)
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), contextA, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), contextB, time.Second); err != nil {
		t.Fatal(err)
	}
	jobCtxA, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	if !state.RegisterBoundBackgroundJob("job-a", bindingA, cancelA) {
		t.Fatal("tenant A background job registration failed")
	}
	jobCtxB, cancelB := context.WithCancel(context.Background())
	defer cancelB()
	if !state.RegisterBoundBackgroundJob("job-b", bindingB, cancelB) {
		t.Fatal("tenant B background job registration failed")
	}
	defer state.UnregisterBackgroundJob("job-b")

	releaseTail := make(chan struct{})
	go func() {
		<-jobCtxA.Done()
		<-releaseTail
		state.UnregisterBackgroundJob("job-a")
	}()
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- state.CancelBoundBackgroundJobsForParentThreadAndWait(
			context.Background(), contextA.ThreadID, contextA.TenantID, contextA.UserID, time.Second,
		)
	}()
	select {
	case <-jobCtxA.Done():
	case <-time.After(time.Second):
		t.Fatal("same-principal parent background job was not canceled")
	}
	select {
	case err := <-waitDone:
		t.Fatalf("parent-thread cancellation returned before completion tail unregister: %v", err)
	default:
	}
	select {
	case <-jobCtxB.Done():
		t.Fatal("same thread text in another principal namespace was canceled")
	default:
	}
	close(releaseTail)
	select {
	case err := <-waitDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("parent-thread cancellation did not observe final unregister")
	}
}

func TestCancelBoundBackgroundJobsForParentThreadAndWaitFailsClosedOnMissingUnregister(t *testing.T) {
	state := NewRuntimeState()
	current, binding := runtimeStateSecurityFixture(t, "thread-a", "turn-a", "case-a", "snapshot-a", 1)
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), current, time.Second); err != nil {
		t.Fatal(err)
	}
	jobCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const jobID = "job-missing-unregister"
	if !state.RegisterBoundBackgroundJob(jobID, binding, cancel) {
		t.Fatal("background job registration failed")
	}
	defer state.UnregisterBackgroundJob(jobID)
	if err := state.CancelBoundBackgroundJobsForParentThreadAndWait(
		context.Background(), current.ThreadID, current.TenantID, current.UserID, 20*time.Millisecond,
	); err == nil || !strings.Contains(err.Error(), "did not stop") {
		t.Fatalf("missing unregister did not fail closed: %v", err)
	}
	select {
	case <-jobCtx.Done():
	default:
		t.Fatal("timeout path did not request cancellation")
	}
	if ids := state.ActiveBackgroundJobIDs(); len(ids) != 1 || ids[0] != jobID {
		t.Fatalf("timeout path falsely acknowledged unregister: %#v", ids)
	}
}

func TestRuntimeStateTransitionAbortNeverPublishesTentativeAuthority(t *testing.T) {
	state := NewRuntimeState()
	contextA, oldBinding := runtimeStateSecurityFixture(t, "thread_a", "turn_a", "case_a", "snapshot_a", 1)
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), contextA, time.Second); err != nil {
		t.Fatal(err)
	}
	contextB, newBinding := runtimeStateSecurityFixture(t, "thread_a", "turn_b", "case_b", "snapshot_b", 2)
	transition, err := state.BeginSecurityContextTransition(context.Background(), contextB)
	if err != nil {
		t.Fatal(err)
	}
	oldJobCtx, oldCancel := context.WithCancel(context.Background())
	if state.RegisterBoundBackgroundJob("job_during_transition", oldBinding, oldCancel) {
		t.Fatal("late job registered while its thread/workspace transition was pending")
	}
	select {
	case <-oldJobCtx.Done():
	default:
		t.Fatal("rejected transition-time job was not canceled")
	}
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := transition.Prepare(canceledCtx, contextB, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled transition prepare did not fail closed: %v", err)
	}
	transition.Abort()

	oldJobCtx, oldCancel = context.WithCancel(context.Background())
	if !state.RegisterBoundBackgroundJob("job_after_abort_old", oldBinding, oldCancel) {
		t.Fatal("abort did not restore old durable admission authority")
	}
	state.UnregisterBackgroundJob("job_after_abort_old")
	oldCancel()
	newJobCtx, newCancel := context.WithCancel(context.Background())
	if state.RegisterBoundBackgroundJob("job_after_abort_new", newBinding, newCancel) {
		t.Fatal("abort published tentative new authority")
	}
	select {
	case <-newJobCtx.Done():
	default:
		t.Fatal("rejected tentative-authority job was not canceled")
	}
	threadKey := securityAuthorityMapKey("thread", contextA.TenantID, contextA.UserID, contextA.ThreadID)
	if current := state.currentByThread[threadKey]; current != contextA {
		t.Fatalf("abort changed current thread authority: %#v", current)
	}
}

func TestRuntimeStateRejectsLateOldContextRegistration(t *testing.T) {
	state := NewRuntimeState()
	_, oldBinding := runtimeStateSecurityFixture(t, "thread_a", "turn_a", "case_a", "snapshot_a", 1)
	contextB, _ := runtimeStateSecurityFixture(t, "thread_a", "turn_b", "case_b", "snapshot_b", 2)
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), contextB, time.Second); err != nil {
		t.Fatal(err)
	}
	jobCtx, cancel := context.WithCancel(context.Background())
	if state.RegisterBoundBackgroundJob("job_late", oldBinding, cancel) {
		t.Fatal("late old-context job registration was accepted")
	}
	select {
	case <-jobCtx.Done():
	default:
		t.Fatal("rejected late job was not canceled")
	}
}

func TestRuntimeStateDoesNotCancelUnrelatedThreadWithSameWorkspaceAuthority(t *testing.T) {
	state := NewRuntimeState()
	contextA, binding := runtimeStateSecurityFixture(t, "thread_a", "turn_a", "case_a", "snapshot_a", 3)
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), contextA, time.Second); err != nil {
		t.Fatal(err)
	}
	jobCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if !state.RegisterBoundBackgroundJob("job_a", binding, cancel) {
		t.Fatal("current job registration was rejected")
	}
	defer state.UnregisterBackgroundJob("job_a")
	contextB := runtimeStateDerivedContextV2(t, contextA, "thread_b", "turn_b", 99)
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), contextB, time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case <-jobCtx.Done():
		t.Fatal("same workspace authority on another thread incorrectly canceled the job")
	default:
	}
}

func TestRuntimeStateNamespacesThreadAndWorkspaceAuthorityByPrincipal(t *testing.T) {
	state := NewRuntimeState()
	contextA, bindingA := runtimeStateSecurityFixtureForPrincipal(t, "tenant-a", "user-a", "shared-thread", "turn-a", "case-a", "snapshot-a", 1)
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), contextA, time.Second); err != nil {
		t.Fatal(err)
	}
	jobCtxA, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	if !state.RegisterBoundBackgroundJob("job-a", bindingA, cancelA) {
		t.Fatal("tenant A job registration was rejected")
	}
	defer state.UnregisterBackgroundJob("job-a")

	contextB, bindingB := runtimeStateSecurityFixtureForPrincipal(t, "tenant-b", "user-b", "shared-thread", "turn-b", "case-b", "snapshot-b", 2)
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), contextB, time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case <-jobCtxA.Done():
		t.Fatal("tenant B authority canceled tenant A job sharing only path/thread text")
	default:
	}
	_, cancelB := context.WithCancel(context.Background())
	defer cancelB()
	if !state.RegisterBoundBackgroundJob("job-b", bindingB, cancelB) {
		t.Fatal("tenant B job registration collided with tenant A authority namespace")
	}
	state.UnregisterBackgroundJob("job-b")
}

func TestRuntimeStateFailsClosedWhenOldJobDoesNotStop(t *testing.T) {
	state := NewRuntimeState()
	contextA, binding := runtimeStateSecurityFixture(t, "thread_a", "turn_a", "case_a", "snapshot_a", 1)
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), contextA, time.Second); err != nil {
		t.Fatal(err)
	}
	_, cancel := context.WithCancel(context.Background())
	if !state.RegisterBoundBackgroundJob("job_stuck", binding, cancel) {
		t.Fatal("current job registration was rejected")
	}
	defer state.UnregisterBackgroundJob("job_stuck")
	contextB, _ := runtimeStateSecurityFixture(t, "thread_a", "turn_b", "case_b", "snapshot_b", 2)
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), contextB, 20*time.Millisecond); err == nil {
		t.Fatal("new case authority ignored a job that did not acknowledge cancellation")
	}
}

func TestCancelBackgroundJobAndWaitTimeoutKeepsUnregisteredJobUnproven(t *testing.T) {
	state := NewRuntimeState()
	jobCtx, cancel := context.WithCancel(context.Background())
	startBarrier, registered := state.RegisterBackgroundJobWithStartBarrier("job_never_unregisters", cancel)
	if !registered || startBarrier == nil {
		t.Fatal("background job registration failed")
	}
	found, stopped, err := state.CancelBackgroundJobAndWait("job_never_unregisters", 20*time.Millisecond)
	if !found || stopped || !errors.Is(err, ErrBackgroundJobStopTimeout) {
		t.Fatalf("timeout result must distinguish found from stopped: found=%t stopped=%t err=%v", found, stopped, err)
	}
	select {
	case <-jobCtx.Done():
	default:
		t.Fatal("cancel request was not delivered")
	}
	if ids := state.ActiveBackgroundJobIDs(); len(ids) != 1 || ids[0] != "job_never_unregisters" {
		t.Fatalf("timeout falsely unregistered the job: %#v", ids)
	}
	started := false
	if err := startBarrier.StartIfActive(context.Background(), func() error {
		started = true
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled start authority was not closed: %v", err)
	}
	if started {
		t.Fatal("cancel that won before start still ran the start callback")
	}
	state.UnregisterBackgroundJob("job_never_unregisters")
}

func TestCancelBackgroundJobAndWaitReportsAcknowledgedStop(t *testing.T) {
	state := NewRuntimeState()
	jobCtx, cancel := context.WithCancel(context.Background())
	if _, registered := state.RegisterBackgroundJobWithStartBarrier("job_stops", cancel); !registered {
		t.Fatal("background job registration failed")
	}
	go func() {
		<-jobCtx.Done()
		state.UnregisterBackgroundJob("job_stops")
	}()
	found, stopped, err := state.CancelBackgroundJobAndWait("job_stops", time.Second)
	if !found || !stopped || err != nil {
		t.Fatalf("acknowledged stop result mismatch: found=%t stopped=%t err=%v", found, stopped, err)
	}
}

func TestCancelBackgroundJobsAndWaitReportsPartialStopWithoutUnregisteringTimeout(t *testing.T) {
	state := NewRuntimeState()
	stoppingCtx, stopCancel := context.WithCancel(context.Background())
	state.RegisterBackgroundJob("job_stops", stopCancel)
	go func() {
		<-stoppingCtx.Done()
		state.UnregisterBackgroundJob("job_stops")
	}()
	_, stuckCancel := context.WithCancel(context.Background())
	state.RegisterBackgroundJob("job_stuck", stuckCancel)

	found, stopped, err := state.CancelBackgroundJobsAndWait(20 * time.Millisecond)
	if found != 2 || stopped != 1 || !errors.Is(err, ErrBackgroundJobStopTimeout) {
		t.Fatalf("partial stop summary mismatch: found=%d stopped=%d err=%v", found, stopped, err)
	}
	ids := state.ActiveBackgroundJobIDs()
	if len(ids) != 1 || ids[0] != "job_stuck" {
		t.Fatalf("partial timeout removed the unproven job: %#v", ids)
	}
	state.UnregisterBackgroundJob("job_stuck")
}

func TestCancelBackgroundJobsAndWaitClosesLateAdmission(t *testing.T) {
	state := NewRuntimeState()
	found, stopped, err := state.CancelBackgroundJobsAndWait(time.Second)
	if found != 0 || stopped != 0 || err != nil {
		t.Fatalf("empty shutdown summary mismatch: found=%d stopped=%d err=%v", found, stopped, err)
	}
	jobCtx, cancel := context.WithCancel(context.Background())
	if startBarrier, registered := state.RegisterBackgroundJobWithStartBarrier("job_after_shutdown", cancel); registered || startBarrier != nil {
		t.Fatal("background admission reopened after shutdown cancellation began")
	}
	select {
	case <-jobCtx.Done():
	default:
		t.Fatal("late background admission was rejected without canceling its context")
	}
}

func TestBackgroundJobStartWinnerLinearizesBeforeCancel(t *testing.T) {
	state := NewRuntimeState()
	jobCtx, cancel := context.WithCancel(context.Background())
	startBarrier, registered := state.RegisterBackgroundJobWithStartBarrier("job_start_wins", cancel)
	if !registered {
		t.Fatal("background job registration failed")
	}
	startEntered := make(chan struct{})
	releaseStart := make(chan struct{})
	startDone := make(chan error, 1)
	go func() {
		startDone <- startBarrier.StartIfActive(jobCtx, func() error {
			close(startEntered)
			<-releaseStart
			return nil
		})
	}()
	<-startEntered
	if found := state.CancelBackgroundJob("job_start_wins"); !found {
		t.Fatal("cancel lost the registered control")
	}
	select {
	case <-jobCtx.Done():
	default:
		t.Fatal("cancel was not delivered while the authorized start was in flight")
	}
	select {
	case err := <-startDone:
		t.Fatalf("authorized start returned before the deterministic release: %v", err)
	default:
	}
	close(releaseStart)
	if err := <-startDone; err != nil {
		t.Fatalf("start winner failed unexpectedly: %v", err)
	}
	state.UnregisterBackgroundJob("job_start_wins")
}

func TestContextTransitionWinsAfterLastCheckAndBeforeBackgroundStart(t *testing.T) {
	state := NewRuntimeState()
	contextA, binding := runtimeStateSecurityFixture(t, "thread_a", "turn_a", "case_a", "snapshot_a", 1)
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), contextA, time.Second); err != nil {
		t.Fatal(err)
	}
	jobCtx, cancel := context.WithCancel(context.Background())
	startBarrier, registered := state.RegisterBoundBackgroundJobWithStartBarrier("job_transition_wins", binding, cancel)
	if !registered {
		t.Fatal("bound background job registration failed")
	}
	afterLastCheck := make(chan struct{})
	releaseStartAttempt := make(chan struct{})
	startResult := make(chan error, 1)
	startCalled := make(chan struct{}, 1)
	go func() {
		close(afterLastCheck)
		<-releaseStartAttempt
		err := startBarrier.StartIfActive(jobCtx, func() error {
			startCalled <- struct{}{}
			return nil
		})
		state.UnregisterBackgroundJob("job_transition_wins")
		startResult <- err
	}()
	<-afterLastCheck
	contextB, _ := runtimeStateSecurityFixture(t, "thread_a", "turn_b", "case_b", "snapshot_b", 2)
	transitionDone := make(chan error, 1)
	go func() {
		transitionDone <- state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), contextB, time.Second)
	}()
	select {
	case <-jobCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("context transition did not close old job start authority")
	}
	close(releaseStartAttempt)
	if err := <-startResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("old job start was not rejected after transition cancellation won: %v", err)
	}
	if err := <-transitionDone; err != nil {
		t.Fatalf("context transition failed after canceled job acknowledged stop: %v", err)
	}
	select {
	case <-startCalled:
		t.Fatal("old job started after the new context transition won")
	default:
	}
}

func TestBoundBackgroundAdmissionRegistersBeforeTransferAndCleansExactlyOnce(t *testing.T) {
	state := NewRuntimeState()
	securityContext, binding := runtimeStateSecurityFixture(t, "thread_a", "turn_a", "case_a", "snapshot_a", 1)
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), securityContext, time.Second); err != nil {
		t.Fatal(err)
	}
	control, jobCtx, err := BeginOptionalBoundBackgroundAdmission(context.Background(), true, state, "job_transfer", binding)
	if err != nil || len(state.ActiveBackgroundJobIDs()) != 1 {
		t.Fatalf("bound admission did not register before preparation: control=%#v err=%v", control, err)
	}
	cleanup := control.TransferCleanup()
	control.Close()
	select {
	case <-jobCtx.Done():
		t.Fatal("ownership transfer canceled the job before goroutine cleanup")
	default:
	}
	cleanup()
	cleanup()
	select {
	case <-jobCtx.Done():
	default:
		t.Fatal("transferred cleanup did not cancel the job")
	}
	if len(state.ActiveBackgroundJobIDs()) != 0 {
		t.Fatal("transferred cleanup left a registered background control")
	}
}

func TestBoundChildAdmissionPreservesForegroundContextAndDetachesBackground(t *testing.T) {
	state := NewRuntimeState()
	securityContext, binding := runtimeStateSecurityFixture(t, "thread_context", "turn_context", "case_context", "snapshot_context", 1)
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), securityContext, time.Second); err != nil {
		t.Fatal(err)
	}
	parent, cancelParent := context.WithCancel(context.Background())
	foreground, foregroundCtx, err := BeginBoundChildAdmission(parent, false, state, "job_foreground_context", binding)
	if err != nil {
		t.Fatal(err)
	}
	background, backgroundCtx, err := BeginBoundChildAdmission(parent, true, state, "job_background_context", binding)
	if err != nil {
		foreground.Close()
		t.Fatal(err)
	}
	cancelParent()
	select {
	case <-foregroundCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("foreground child did not preserve parent cancellation")
	}
	select {
	case <-backgroundCtx.Done():
		t.Fatal("background child inherited the provider tool context")
	default:
	}
	foreground.Close()
	background.Close()
}

func TestRuntimeStateExposesOrdinaryEffectWithoutWeakeningStrictEffect(t *testing.T) {
	state := NewRuntimeState()
	securityContext := runtimeStateWitnessedBoundaryContext(t, "thread-ordinary", "turn-ordinary", 1)
	_, release, err := state.AcquireOrdinaryContextEffect(context.Background(), securityContext)
	if err != nil {
		t.Fatalf("runtime state rejected witnessed boundary ordinary effect: %v", err)
	}
	release()
	if _, _, err := state.AcquireContextEffect(context.Background(), securityContext); err == nil {
		t.Fatal("runtime state strict effect admitted a boundary-only context")
	}
	_, release, err = state.AcquireContextEffectForAuthority(context.Background(), securityContext, false)
	if err != nil {
		t.Fatalf("effect selector rejected witnessed boundary ordinary effect: %v", err)
	}
	release()
	if _, _, err := state.AcquireContextEffectForAuthority(context.Background(), securityContext, true); err == nil {
		t.Fatal("effect selector admitted case data under boundary-only authority")
	}
}

func runtimeStateSecurityFixture(t *testing.T, threadID, turnID, caseID, snapshot string, epoch uint64) (domainsecurity.TurnSecurityContext, *domainjob.SecurityBinding) {
	return runtimeStateSecurityFixtureForPrincipal(t, "", "", threadID, turnID, caseID, snapshot, epoch)
}

func runtimeStateWitnessedBoundaryContext(t *testing.T, threadID, turnID string, epoch uint64) domainsecurity.TurnSecurityContext {
	t.Helper()
	workspace := "/workspace/shared"
	policyDigest := domainsecurity.SHA256Hex([]byte("runtime-state-boundary-risk:\x00" + threadID))
	publication, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest:   policyDigest,
		RiskClass:                domainsecurity.RiskClassCase,
		Disposition:              domainsecurity.PublicationDispositionCaseBoundaryOnly,
		CaseBindingState:         domainsecurity.CaseBindingStateMissing,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("runtime-state-boundary-binding:\x00" + threadID)),
		BlockerCode:              domainsecurity.PublicationBlockerCaseBindingMissing,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := securitycontexttest.WitnessedRiskBinding(
		threadID, workspace, domainsecurity.RiskClassCase, policyDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: domainsecurity.UnboundCaseID, CaseBindingHash: domainsecurity.UnboundCaseBindingHash(workspace),
		DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID, SourceManifestHash: domainsecurity.EmptySourceManifestHash,
		ContextEpoch: epoch, IssuedAt: time.Now().UTC(), PublicationPolicy: publication, RiskAuthorityBinding: binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func runtimeStateSecurityFixtureForPrincipal(t *testing.T, tenantID, userID, threadID, turnID, caseID, snapshot string, epoch uint64) (domainsecurity.TurnSecurityContext, *domainjob.SecurityBinding) {
	t.Helper()
	now := time.Now().UTC()
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/workspace/shared", TenantID: tenantID, UserID: userID, CaseID: caseID,
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-" + caseID)), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID(snapshot),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-" + snapshot)), ContextEpoch: epoch, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider_1", ServerIdentity: "host:builtin", ToolName: "task", ToolCallID: subagentTestHostToolCallID("runtime-state-" + turnID),
		ArgsHash: domainsecurity.SHA256Hex([]byte("args")), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	binding, err := domainjob.NewSecurityBinding(securityContext, grant, grant.ToolCallID)
	if err != nil {
		t.Fatal(err)
	}
	return securityContext, binding
}

func runtimeStateDerivedContextV2(t *testing.T, parent domainsecurity.TurnSecurityContext, threadID, turnID string, epoch uint64) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: parent.WorkspaceRealPath,
		TenantID: parent.TenantID, UserID: parent.UserID, CaseID: parent.CaseID, CaseBindingHash: parent.CaseBindingHash,
		DatasetSnapshotID: parent.DatasetSnapshotID, SourceManifestHash: parent.SourceManifestHash, ContextEpoch: epoch,
		IssuedAt: time.Now().UTC(), PublicationPolicy: parent.PublicationPolicy, RiskAuthorityBinding: parent.RiskAuthorityBinding,
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}
