package effectgate

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestEffectGateTransitionWaitsForInFlightEffect(t *testing.T) {
	gate := New()
	securityContext := effectGateContext("thread-a", "/workspace/a", "case-a", 1)
	_, releaseEffect, err := gate.AcquireEffect(context.Background(), securityContext)
	if err != nil {
		t.Fatal(err)
	}
	transitionStarted := make(chan struct{})
	transitionAcquired := make(chan func(), 1)
	go func() {
		close(transitionStarted)
		release, err := gate.AcquireTransition(context.Background(), securityContext)
		if err != nil {
			transitionAcquired <- nil
			return
		}
		transitionAcquired <- release
	}()
	<-transitionStarted
	select {
	case <-transitionAcquired:
		t.Fatal("authority transition crossed an in-flight effect")
	case <-time.After(25 * time.Millisecond):
	}
	releaseEffect()
	select {
	case release := <-transitionAcquired:
		if release == nil {
			t.Fatal("authority transition failed after effect release")
		}
		release()
	case <-time.After(time.Second):
		t.Fatal("authority transition did not resume")
	}
}

func TestEffectGateEffectWaitsForWinningTransition(t *testing.T) {
	gate := New()
	securityContext := effectGateContext("thread-a", "/workspace/a", "case-a", 1)
	releaseTransition, err := gate.AcquireTransition(context.Background(), securityContext)
	if err != nil {
		t.Fatal(err)
	}
	effectAcquired := make(chan func(), 1)
	go func() {
		_, release, err := gate.AcquireEffect(context.Background(), securityContext)
		if err != nil {
			effectAcquired <- nil
			return
		}
		effectAcquired <- release
	}()
	select {
	case <-effectAcquired:
		t.Fatal("effect crossed a winning authority transition")
	case <-time.After(25 * time.Millisecond):
	}
	releaseTransition()
	select {
	case release := <-effectAcquired:
		if release == nil {
			t.Fatal("effect failed after transition release")
		}
		release()
	case <-time.After(time.Second):
		t.Fatal("effect did not resume after transition")
	}
}

func TestEffectGatePreContextTransitionIsConcurrencyOnly(t *testing.T) {
	gate := New()
	transition := TransitionScope{
		ThreadID: "thread-pre-context", WorkspaceRealPath: "/workspace/pre-context",
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
	}
	releaseTransition, err := gate.AcquireTransitionScope(context.Background(), transition)
	if err != nil {
		t.Fatal(err)
	}
	securityContext := effectGateContext(transition.ThreadID, transition.WorkspaceRealPath, "case-a", 1)
	effectDone := make(chan func(), 1)
	go func() {
		_, release, acquireErr := gate.AcquireEffect(context.Background(), securityContext)
		if acquireErr != nil {
			effectDone <- nil
			return
		}
		effectDone <- release
	}()
	select {
	case <-effectDone:
		t.Fatal("effect crossed a pre-context transition writer")
	case <-time.After(25 * time.Millisecond):
	}
	releaseTransition()
	select {
	case release := <-effectDone:
		if release == nil {
			t.Fatal("effect failed after pre-context transition release")
		}
		release()
	case <-time.After(time.Second):
		t.Fatal("effect remained blocked after pre-context transition release")
	}

	effectCtx, releaseEffect, err := gate.AcquireEffect(context.Background(), securityContext)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseEffect()
	if release, err := gate.AcquireTransitionScope(effectCtx, transition); err == nil {
		if release != nil {
			release()
		}
		t.Fatal("provider effect lease upgraded through a pre-context transition")
	}
	transition.ThreadID = " thread-pre-context"
	if release, err := gate.AcquireTransitionScope(context.Background(), transition); err == nil {
		if release != nil {
			release()
		}
		t.Fatal("non-canonical pre-context transition identity was accepted")
	}
}

func TestEffectGateTransitionBarrierReservesWriterBeforeCancellation(t *testing.T) {
	gate := New()
	securityContext := effectGateContext("thread-barrier", "/workspace/barrier", "case-a", 1)
	_, releaseEffect, err := gate.AcquireEffect(context.Background(), securityContext)
	if err != nil {
		t.Fatal(err)
	}
	barrierEntered := make(chan struct{})
	releaseBarrier := make(chan struct{})
	type result struct {
		release func()
		err     error
	}
	transitionDone := make(chan result, 1)
	go func() {
		release, acquireErr := gate.AcquireTransitionScopeWithBarrier(context.Background(), TransitionScope{
			ThreadID: securityContext.ThreadID, WorkspaceRealPath: securityContext.WorkspaceRealPath,
			TenantID: securityContext.TenantID, UserID: securityContext.UserID,
		}, func() error {
			close(barrierEntered)
			<-releaseBarrier
			return nil
		})
		transitionDone <- result{release: release, err: acquireErr}
	}()
	select {
	case <-barrierEntered:
	case <-time.After(time.Second):
		t.Fatal("transition barrier did not run while old effect was active")
	}
	lateEffect := make(chan error, 1)
	go func() {
		_, release, acquireErr := gate.AcquireEffect(context.Background(), securityContext)
		if release != nil {
			release()
		}
		lateEffect <- acquireErr
	}()
	select {
	case err := <-lateEffect:
		t.Fatalf("late effect crossed reserved writer: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	releaseEffect()
	close(releaseBarrier)
	var transition result
	select {
	case transition = <-transitionDone:
		if transition.err != nil || transition.release == nil {
			t.Fatalf("reserved transition failed: %v", transition.err)
		}
	case <-time.After(time.Second):
		t.Fatal("reserved transition did not acquire after old effect settled")
	}
	select {
	case err := <-lateEffect:
		t.Fatalf("late effect crossed active writer: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	transition.release()
	select {
	case err := <-lateEffect:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("late effect did not resume after transition")
	}
}

func TestEffectGateChildPreContextTransitionBorrowsOnlyExactParentWorkspace(t *testing.T) {
	gate := New()
	parent := effectGateContext("thread-parent-pre-context", "/workspace/shared-pre-context", "case-a", 3)
	parentCtx, releaseParent, err := gate.AcquireEffect(context.Background(), parent)
	if err != nil {
		t.Fatal(err)
	}
	child := TransitionScope{
		ThreadID: "thread-child-pre-context", WorkspaceRealPath: parent.WorkspaceRealPath,
		TenantID: parent.TenantID, UserID: parent.UserID,
	}
	releaseChild, err := gate.AcquireDelegatedChildTransitionScope(parentCtx, child, parent.ContextDigest)
	if err != nil {
		t.Fatalf("exact child pre-context transition was rejected: %v", err)
	}

	// The child transition retains the parent workspace lease even if the
	// outer caller releases its own reference first.
	releaseParent()
	workspaceWriter := make(chan func(), 1)
	go func() {
		changed := effectGateContext("thread-workspace-writer", parent.WorkspaceRealPath, "case-b", 1)
		release, _ := gate.AcquireTransition(context.Background(), changed)
		workspaceWriter <- release
	}()
	select {
	case <-workspaceWriter:
		t.Fatal("workspace writer crossed a live child pre-context transition")
	case <-time.After(25 * time.Millisecond):
	}
	releaseChild()
	select {
	case release := <-workspaceWriter:
		if release == nil {
			t.Fatal("workspace writer failed after child transition release")
		}
		release()
	case <-time.After(time.Second):
		t.Fatal("workspace writer remained blocked after child transition release")
	}

	assertRejected := func(name string, transition TransitionScope) {
		t.Helper()
		ctx, release, acquireErr := gate.AcquireEffect(context.Background(), parent)
		if acquireErr != nil {
			t.Fatalf("%s: acquire parent: %v", name, acquireErr)
		}
		defer release()
		if childRelease, childErr := gate.AcquireDelegatedChildTransitionScope(ctx, transition, parent.ContextDigest); childErr == nil {
			if childRelease != nil {
				childRelease()
			}
			t.Fatalf("%s child scope was accepted", name)
		}
	}
	assertRejected("same thread", TransitionScope{
		ThreadID: parent.ThreadID, WorkspaceRealPath: parent.WorkspaceRealPath, TenantID: parent.TenantID, UserID: parent.UserID,
	})
	assertRejected("cross workspace", TransitionScope{
		ThreadID: child.ThreadID, WorkspaceRealPath: "/workspace/other", TenantID: parent.TenantID, UserID: parent.UserID,
	})
	assertRejected("cross tenant", TransitionScope{
		ThreadID: child.ThreadID, WorkspaceRealPath: parent.WorkspaceRealPath, TenantID: "tenant-other", UserID: parent.UserID,
	})
	assertRejected("cross user", TransitionScope{
		ThreadID: child.ThreadID, WorkspaceRealPath: parent.WorkspaceRealPath, TenantID: parent.TenantID, UserID: "user-other",
	})
	ctx, release, err := gate.AcquireEffect(context.Background(), parent)
	if err != nil {
		t.Fatalf("acquire digest-mismatch parent: %v", err)
	}
	if childRelease, childErr := gate.AcquireDelegatedChildTransitionScope(ctx, child, domainsecurity.SHA256Hex([]byte("other-parent"))); childErr == nil {
		if childRelease != nil {
			childRelease()
		}
		t.Fatal("child pre-context transition with a mismatched parent digest was accepted")
	}
	release()
	if release, err := gate.AcquireDelegatedChildTransitionScope(context.Background(), child, parent.ContextDigest); err == nil {
		if release != nil {
			release()
		}
		t.Fatal("child pre-context transition without a parent effect lease was accepted")
	}
}

func TestEffectGateNestedBatchLeaseCannotDeadlockBehindQueuedWriter(t *testing.T) {
	gate := New()
	securityContext := effectGateContext("thread-a", "/workspace/a", "case-a", 1)
	leaseCtx, releaseEffect, err := gate.AcquireEffect(context.Background(), securityContext)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseEffect()
	writerWaiting := make(chan struct{})
	writerDone := make(chan func(), 1)
	go func() {
		close(writerWaiting)
		release, err := gate.AcquireTransition(context.Background(), securityContext)
		if err != nil {
			writerDone <- nil
			return
		}
		writerDone <- release
	}()
	<-writerWaiting
	waitForEffectGateWriter(t, gate, securityContext)
	_, releaseNested, err := gate.AcquireEffect(leaseCtx, securityContext)
	if err != nil {
		t.Fatalf("nested batch member did not reuse its host lease: %v", err)
	}
	releaseNested()
	releaseEffect()
	select {
	case release := <-writerDone:
		if release == nil {
			t.Fatal("queued writer failed")
		}
		release()
	case <-time.After(time.Second):
		t.Fatal("queued writer remained deadlocked")
	}
}

func TestEffectGateNestedReferenceKeepsOuterAuthorityLocked(t *testing.T) {
	gate := New()
	securityContext := effectGateContext("thread-a", "/workspace/a", "case-a", 1)
	leaseCtx, releaseOuter, err := gate.AcquireEffect(context.Background(), securityContext)
	if err != nil {
		t.Fatal(err)
	}
	_, releaseNested, err := gate.AcquireEffect(leaseCtx, securityContext)
	if err != nil {
		t.Fatal(err)
	}
	releaseOuter()
	transitionDone := make(chan func(), 1)
	go func() {
		release, _ := gate.AcquireTransition(context.Background(), securityContext)
		transitionDone <- release
	}()
	select {
	case <-transitionDone:
		t.Fatal("outer release invalidated a live nested effect reference")
	case <-time.After(25 * time.Millisecond):
	}
	releaseNested()
	select {
	case release := <-transitionDone:
		if release == nil {
			t.Fatal("transition failed after the final nested reference was released")
		}
		release()
	case <-time.After(time.Second):
		t.Fatal("transition remained blocked after the final nested reference was released")
	}
}

func TestEffectGateNestedChildTransitionBorrowsMatchingWorkspaceLease(t *testing.T) {
	gate := New()
	parent := effectGateContext("thread-parent", "/workspace/shared", "case-a", 4)
	parentCtx, releaseParent, err := gate.AcquireEffect(context.Background(), parent)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseParent()
	changed := effectGateContext("thread-switch", parent.WorkspaceRealPath, "case-b", 1)
	writerDone := make(chan func(), 1)
	go func() {
		release, _ := gate.AcquireTransition(context.Background(), changed)
		writerDone <- release
	}()
	waitForEffectGateWriter(t, gate, changed)
	child := effectGateChildContext(parent, "thread-child")
	releaseChildTransition, err := gate.AcquireTransition(parentCtx, child)
	if err != nil {
		t.Fatalf("nested child transition could not borrow the matching workspace lease: %v", err)
	}
	releaseChildTransition()
	_, releaseChildEffect, err := gate.AcquireEffect(parentCtx, child)
	if err != nil {
		t.Fatalf("nested child effect could not borrow the matching workspace lease: %v", err)
	}
	releaseChildEffect()
	select {
	case <-writerDone:
		t.Fatal("workspace-changing writer crossed the parent effect lease")
	default:
	}
	releaseParent()
	release := <-writerDone
	if release == nil {
		t.Fatal("workspace-changing writer failed after parent release")
	}
	release()
}

func TestEffectGateQueuedChildTransitionCannotCreateParentChildDeadlock(t *testing.T) {
	gate := New()
	parent := effectGateContext("thread-parent", "/workspace/shared", "case-a", 4)
	parentCtx, releaseParent, err := gate.AcquireEffect(context.Background(), parent)
	if err != nil {
		t.Fatal(err)
	}
	child := effectGateChildContext(parent, "thread-child")
	queuedTransition := make(chan func(), 1)
	go func() {
		release, _ := gate.AcquireTransition(context.Background(), child)
		queuedTransition <- release
	}()
	waitForEffectGateWriterOnKey(t, gate, "workspace\x00"+child.TenantID+"\x00"+child.UserID+"\x00"+child.WorkspaceRealPath)

	nestedDone := make(chan error, 1)
	go func() {
		release, acquireErr := gate.AcquireTransition(parentCtx, child)
		if acquireErr == nil {
			release()
		}
		nestedDone <- acquireErr
	}()
	select {
	case acquireErr := <-nestedDone:
		if acquireErr != nil {
			t.Fatalf("nested child transition failed: %v", acquireErr)
		}
	case <-time.After(time.Second):
		t.Fatal("queued child transition created a workspace/thread lock-order deadlock")
	}
	select {
	case <-queuedTransition:
		t.Fatal("independent child transition crossed the parent workspace lease")
	default:
	}
	releaseParent()
	select {
	case release := <-queuedTransition:
		if release == nil {
			t.Fatal("queued child transition failed after parent release")
		}
		release()
	case <-time.After(time.Second):
		t.Fatal("queued child transition remained blocked after parent release")
	}
}

func TestEffectGateBorrowedChildEffectRetainsParentWorkspaceAuthority(t *testing.T) {
	gate := New()
	parent := effectGateContext("thread-parent", "/workspace/shared", "case-a", 4)
	parentCtx, releaseParent, err := gate.AcquireEffect(context.Background(), parent)
	if err != nil {
		t.Fatal(err)
	}
	child := effectGateChildContext(parent, "thread-child")
	_, releaseChild, err := gate.AcquireEffect(parentCtx, child)
	if err != nil {
		t.Fatal(err)
	}
	releaseParent()
	changed := effectGateContext("thread-switch", parent.WorkspaceRealPath, "case-b", 1)
	transitionDone := make(chan func(), 1)
	go func() {
		release, _ := gate.AcquireTransition(context.Background(), changed)
		transitionDone <- release
	}()
	select {
	case <-transitionDone:
		t.Fatal("parent workspace authority was released while a borrowed child effect was live")
	case <-time.After(25 * time.Millisecond):
	}
	releaseChild()
	select {
	case release := <-transitionDone:
		if release == nil {
			t.Fatal("transition failed after borrowed child release")
		}
		release()
	case <-time.After(time.Second):
		t.Fatal("transition remained blocked after borrowed child release")
	}
}

func TestEffectGateWorkspaceTransitionBlocksOtherThreadEffect(t *testing.T) {
	gate := New()
	contextA := effectGateContext("thread-a", "/workspace/shared", "case-a", 1)
	contextB := effectGateContext("thread-b", "/workspace/shared", "case-b", 9)
	_, releaseEffect, err := gate.AcquireEffect(context.Background(), contextA)
	if err != nil {
		t.Fatal(err)
	}
	transitionDone := make(chan func(), 1)
	go func() {
		release, _ := gate.AcquireTransition(context.Background(), contextB)
		transitionDone <- release
	}()
	select {
	case <-transitionDone:
		t.Fatal("shared-workspace transition crossed another thread's effect")
	case <-time.After(25 * time.Millisecond):
	}
	releaseEffect()
	release := <-transitionDone
	if release == nil {
		t.Fatal("shared-workspace transition failed")
	}
	release()
}

func TestWorkspaceRebindWriterWaitsForOldAndNewWorkspaceEffects(t *testing.T) {
	gate := New()
	previous := effectGateContext("thread-rebind", "/workspace/a", "case-a", 1)
	target := effectGateContext("thread-rebind", "/workspace/b", "case-b", 2)
	oldEffect := effectGateContext("thread-old-effect", previous.WorkspaceRealPath, "case-a", 1)
	newEffect := effectGateContext("thread-new-effect", target.WorkspaceRealPath, "case-b", 1)
	_, releaseOld, err := gate.AcquireEffect(context.Background(), oldEffect)
	if err != nil {
		t.Fatal(err)
	}
	_, releaseNew, err := gate.AcquireEffect(context.Background(), newEffect)
	if err != nil {
		releaseOld()
		t.Fatal(err)
	}
	acquired := make(chan func(), 1)
	go func() {
		release, acquireErr := gate.AcquireWorkspaceRebindTransition(context.Background(), previous, target)
		if acquireErr != nil {
			acquired <- nil
			return
		}
		acquired <- release
	}()
	select {
	case <-acquired:
		t.Fatal("workspace rebind crossed effects in both workspace authorities")
	case <-time.After(25 * time.Millisecond):
	}
	releaseOld()
	select {
	case <-acquired:
		t.Fatal("workspace rebind crossed the target workspace effect")
	case <-time.After(25 * time.Millisecond):
	}
	releaseNew()
	select {
	case release := <-acquired:
		if release == nil {
			t.Fatal("workspace rebind failed after both effects settled")
		}
		release()
	case <-time.After(time.Second):
		t.Fatal("workspace rebind remained blocked after both effects settled")
	}
}

func TestOppositeWorkspaceRebindWritersUseOneGlobalLockOrder(t *testing.T) {
	gate := New()
	aToBPrevious := effectGateContext("thread-a", "/workspace/a", "case-a", 1)
	aToBTarget := effectGateContext("thread-a", "/workspace/b", "case-b", 2)
	bToAPrevious := effectGateContext("thread-b", "/workspace/b", "case-b", 1)
	bToATarget := effectGateContext("thread-b", "/workspace/a", "case-a", 2)
	type result struct {
		release func()
		err     error
	}
	results := make(chan result, 2)
	go func() {
		release, err := gate.AcquireWorkspaceRebindTransition(context.Background(), aToBPrevious, aToBTarget)
		results <- result{release: release, err: err}
	}()
	go func() {
		release, err := gate.AcquireWorkspaceRebindTransition(context.Background(), bToAPrevious, bToATarget)
		results <- result{release: release, err: err}
	}()
	var first result
	select {
	case first = <-results:
		if first.err != nil || first.release == nil {
			t.Fatalf("first opposite rebind failed: %v", first.err)
		}
	case <-time.After(time.Second):
		t.Fatal("opposite rebind writers deadlocked before either acquired the union")
	}
	select {
	case second := <-results:
		if second.release != nil {
			second.release()
		}
		first.release()
		t.Fatal("both opposite rebind writers held the same workspace union")
	case <-time.After(25 * time.Millisecond):
	}
	first.release()
	select {
	case second := <-results:
		if second.err != nil || second.release == nil {
			t.Fatalf("second opposite rebind failed: %v", second.err)
		}
		second.release()
	case <-time.After(time.Second):
		t.Fatal("second opposite rebind stayed deadlocked after the first released")
	}
}

func TestEffectGateCanceledWaiterDoesNotPoisonScope(t *testing.T) {
	gate := New()
	securityContext := effectGateContext("thread-a", "/workspace/a", "case-a", 1)
	releaseTransition, err := gate.AcquireTransition(context.Background(), securityContext)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := gate.AcquireEffect(ctx, securityContext); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled effect waiter was not rejected: %v", err)
	}
	releaseTransition()
	_, releaseEffect, err := gate.AcquireEffect(context.Background(), securityContext)
	if err != nil {
		t.Fatalf("canceled waiter poisoned the gate: %v", err)
	}
	releaseEffect()
}

func TestEffectGateCanceledNestedLeaseCannotRetainAuthority(t *testing.T) {
	gate := New()
	securityContext := effectGateContext("thread-a", "/workspace/a", "case-a", 1)
	leaseCtx, releaseEffect, err := gate.AcquireEffect(context.Background(), securityContext)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseEffect()
	canceledCtx, cancel := context.WithCancel(leaseCtx)
	cancel()
	if _, _, err := gate.AcquireEffect(canceledCtx, securityContext); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled nested effect retained host authority: %v", err)
	}
	child := effectGateChildContext(securityContext, "thread-child")
	if _, _, err := gate.AcquireEffect(canceledCtx, child); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled child effect borrowed host authority: %v", err)
	}
	if _, err := gate.AcquireTransition(canceledCtx, child); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled child transition borrowed host authority: %v", err)
	}
}

func TestEffectGateCancellationBeforeMutexGrantFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name    string
		acquire func(*scopeLock, context.Context) error
	}{
		{name: "read", acquire: func(lock *scopeLock, ctx context.Context) error { return lock.acquireRead(ctx) }},
		{name: "write", acquire: func(lock *scopeLock, ctx context.Context) error { return lock.acquireWrite(ctx) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			lock := &scopeLock{changed: make(chan struct{})}
			lock.mu.Lock()
			ctx, cancel := context.WithCancel(context.Background())
			result := make(chan error, 1)
			started := make(chan struct{})
			go func() {
				close(started)
				result <- test.acquire(lock, ctx)
			}()
			<-started
			runtime.Gosched()
			cancel()
			lock.mu.Unlock()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("canceled %s lease was granted: %v", test.name, err)
				}
			case <-time.After(time.Second):
				t.Fatalf("canceled %s lease did not return", test.name)
			}
			lock.mu.Lock()
			defer lock.mu.Unlock()
			if lock.readers != 0 || lock.writer || lock.waitingWriters != 0 {
				t.Fatalf("canceled %s lease changed lock authority: %#v", test.name, lock)
			}
		})
	}
}

func TestEffectGateReclaimsOnlyIdleScopeLocks(t *testing.T) {
	gate := New()
	for index := 0; index < 128; index++ {
		securityContext := effectGateContext(
			fmt.Sprintf("thread-%d", index), fmt.Sprintf("/workspace/%d", index), fmt.Sprintf("case-%d", index), 1,
		)
		_, release, err := gate.AcquireEffect(context.Background(), securityContext)
		if err != nil {
			t.Fatal(err)
		}
		release()
	}
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if len(gate.scopes) != 0 {
		t.Fatalf("idle effect-gate scopes leaked: %d", len(gate.scopes))
	}
}

func TestAuditOnlyV1CannotAcquireEffectOrTransitionLease(t *testing.T) {
	legacy := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-v1", TurnID: "turn-v1", WorkspaceRealPath: "/workspace", ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	gate := New()
	if _, _, err := gate.AcquireEffect(context.Background(), legacy); err == nil {
		t.Fatal("audit-only V1 acquired an effect lease")
	}
	if _, _, err := gate.AcquireOrdinaryEffect(context.Background(), legacy); err == nil {
		t.Fatal("audit-only V1 acquired an ordinary effect lease")
	}
	if _, err := gate.AcquireTransition(context.Background(), legacy); err == nil {
		t.Fatal("audit-only V1 acquired a context transition writer")
	}
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if len(gate.scopes) != 0 {
		t.Fatalf("rejected V1 allocated effect-gate scope state: %d", len(gate.scopes))
	}
}

func TestOrdinaryEffectGateAdmitsWitnessedBoundaryWithoutStrictUpgrade(t *testing.T) {
	gate := New()
	boundary := effectGateWitnessedBoundaryContext("thread-boundary", "/workspace/boundary", 1)
	leaseCtx, release, err := gate.AcquireOrdinaryEffect(context.Background(), boundary)
	if err != nil {
		t.Fatalf("witnessed boundary ordinary effect was rejected: %v", err)
	}
	if _, _, err := gate.AcquireEffect(context.Background(), boundary); err == nil {
		t.Fatal("witnessed boundary acquired a strict effect lease")
	}
	transitionCtx, cancelTransition := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelTransition()
	if _, err := gate.AcquireTransition(transitionCtx, boundary); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ordinary effect did not serialize its authority transition: %v", err)
	}
	if _, nestedRelease, err := gate.AcquireOrdinaryEffect(leaseCtx, boundary); err != nil {
		t.Fatalf("nested ordinary effect was rejected: %v", err)
	} else {
		nestedRelease()
	}
	release()
	releaseTransition, err := gate.AcquireTransition(context.Background(), boundary)
	if err != nil {
		t.Fatalf("released ordinary effect retained its transition lock: %v", err)
	}
	releaseTransition()

	quarantined, err := securitycontexttest.BoundaryOnlyContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-quarantined", TurnID: "turn-quarantined", WorkspaceRealPath: "/workspace/quarantined",
		ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := gate.AcquireOrdinaryEffect(context.Background(), quarantined); err == nil {
		t.Fatal("quarantined boundary acquired an ordinary effect lease")
	}
}

func TestOrdinaryEffectLeaseCannotUpgradeNestedStrictAuthority(t *testing.T) {
	gate := New()
	securityContext := effectGateContext("thread-no-upgrade", "/workspace/no-upgrade", "case-no-upgrade", 1)
	leaseCtx, release, err := gate.AcquireOrdinaryEffect(context.Background(), securityContext)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, _, err := gate.AcquireEffect(leaseCtx, securityContext); err == nil ||
		!strings.Contains(err.Error(), "cannot upgrade") {
		t.Fatalf("ordinary lease upgraded through strict nested re-entry: %v", err)
	}
}

func TestMetadataScopeReadCoexistsWithEffectAndBlocksTransition(t *testing.T) {
	gate := New()
	securityContext := effectGateContext("thread-metadata", "/workspace/metadata", "case-metadata", 1)
	scope := TransitionScope{
		ThreadID: securityContext.ThreadID, WorkspaceRealPath: securityContext.WorkspaceRealPath,
		TenantID: securityContext.TenantID, UserID: securityContext.UserID,
	}
	_, releaseEffect, err := gate.AcquireEffect(context.Background(), securityContext)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseEffect()
	releaseMetadata, err := gate.AcquireTransitionScopeRead(context.Background(), scope)
	if err != nil {
		t.Fatalf("metadata read did not coexist with the active effect: %v", err)
	}
	transitionCtx, cancelTransition := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelTransition()
	if _, err := gate.AcquireTransitionScope(transitionCtx, scope); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("transition crossed the metadata/effect readers: %v", err)
	}
	releaseMetadata()
}

func TestMetadataScopeReadWaitsForPreContextTransition(t *testing.T) {
	gate := New()
	scope := TransitionScope{
		ThreadID: "thread-metadata-wait", WorkspaceRealPath: "/workspace/metadata-wait",
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
	}
	releaseTransition, err := gate.AcquireTransitionScope(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	metadataCtx, cancelMetadata := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelMetadata()
	if _, err := gate.AcquireTransitionScopeRead(metadataCtx, scope); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("metadata read crossed the pre-context writer: %v", err)
	}
	releaseTransition()
}

func waitForEffectGateWriter(t *testing.T, gate *Gate, securityContext domainsecurity.TurnSecurityContext) {
	t.Helper()
	keys, _, err := gateKeys(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		locks := lookupEffectGateScopeLocks(gate, keys)
		for _, lock := range locks {
			if lock == nil {
				continue
			}
			lock.mu.Lock()
			waiting := lock.waitingWriters > 0 || lock.writer
			lock.mu.Unlock()
			if waiting {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("writer did not reach the effect-gate queue")
		}
		runtime.Gosched()
	}
}

func waitForEffectGateWriterOnKey(t *testing.T, gate *Gate, key string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		locks := lookupEffectGateScopeLocks(gate, []string{key})
		lock := locks[0]
		if lock == nil {
			if time.Now().After(deadline) {
				t.Fatal("writer did not reach the requested effect-gate scope")
			}
			runtime.Gosched()
			continue
		}
		lock.mu.Lock()
		waiting := lock.waitingWriters > 0 || lock.writer
		lock.mu.Unlock()
		if waiting {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("writer did not reach the requested effect-gate scope")
		}
		runtime.Gosched()
	}
}

func lookupEffectGateScopeLocks(gate *Gate, keys []string) []*scopeLock {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	locks := make([]*scopeLock, len(keys))
	for index, key := range keys {
		locks[index] = gate.scopes[key]
	}
	return locks
}

func effectGateContext(threadID, workspace, caseID string, epoch uint64) domainsecurity.TurnSecurityContext {
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: "turn-" + threadID, WorkspaceRealPath: workspace, CaseID: caseID,
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-" + caseID)), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-" + caseID),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-" + caseID)), ContextEpoch: epoch, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		panic(err)
	}
	return securityContext
}

func effectGateWitnessedBoundaryContext(threadID, workspace string, epoch uint64) domainsecurity.TurnSecurityContext {
	policyDigest := domainsecurity.SHA256Hex([]byte("boundary-risk-policy:\x00" + threadID + "\x00" + workspace))
	publication, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest:   policyDigest,
		RiskClass:                domainsecurity.RiskClassCase,
		Disposition:              domainsecurity.PublicationDispositionCaseBoundaryOnly,
		CaseBindingState:         domainsecurity.CaseBindingStateMissing,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("boundary-binding:\x00" + threadID)),
		BlockerCode:              domainsecurity.PublicationBlockerCaseBindingMissing,
	})
	if err != nil {
		panic(err)
	}
	binding, err := securitycontexttest.WitnessedRiskBinding(
		threadID, workspace, domainsecurity.RiskClassCase, policyDigest,
	)
	if err != nil {
		panic(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: "turn-" + threadID, WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: domainsecurity.UnboundCaseID, CaseBindingHash: domainsecurity.UnboundCaseBindingHash(workspace),
		DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID, SourceManifestHash: domainsecurity.EmptySourceManifestHash,
		ContextEpoch: epoch, IssuedAt: time.Now().UTC(), PublicationPolicy: publication, RiskAuthorityBinding: binding,
	})
	if err != nil {
		panic(err)
	}
	return securityContext
}

func effectGateChildContext(parent domainsecurity.TurnSecurityContext, threadID string) domainsecurity.TurnSecurityContext {
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: "turn-" + threadID, WorkspaceRealPath: parent.WorkspaceRealPath,
		TenantID: parent.TenantID, UserID: parent.UserID, CaseID: parent.CaseID, CaseBindingHash: parent.CaseBindingHash,
		DatasetSnapshotID: parent.DatasetSnapshotID, SourceManifestHash: parent.SourceManifestHash, ContextEpoch: 1, IssuedAt: time.Now().UTC(),
		PublicationPolicy: parent.PublicationPolicy, RiskAuthorityBinding: parent.RiskAuthorityBinding,
	})
	if err != nil {
		panic(err)
	}
	return securityContext
}
