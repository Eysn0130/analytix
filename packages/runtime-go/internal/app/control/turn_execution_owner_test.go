package control

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// These barriers observe entry into Wait's select; elapsed time is never used
// to decide whether an execution owner has actually released its registration.
type turnExecutionOwnerWaitContextV1 struct {
	context.Context
	entered chan struct{}
	once    sync.Once
}

func (ctx *turnExecutionOwnerWaitContextV1) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.entered) })
	return ctx.Context.Done()
}

func receiveTurnExecutionOwnerTestV1[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(time.Second):
		t.Fatal("execution owner channel did not progress (deadlock guard)")
		var zero T
		return zero
	}
}

func assertTurnExecutionOwnerOpenV1(t *testing.T, owner *TurnExecutionOwnerV1) {
	t.Helper()
	select {
	case <-owner.Done():
		t.Fatal("execution owner was released before actual cleanup")
	default:
	}
}

func registerTurnExecutionOwnerTestV1(t *testing.T, c *Controller, ctx context.Context, threadID, turnID string, cancel context.CancelFunc) (context.Context, *TurnExecutionOwnerV1) {
	t.Helper()
	ownedCtx, owner, err := c.RegisterTurnCancelOwnedV1(ctx, threadID, turnID, cancel)
	if err != nil || owner == nil || ownedCtx == nil {
		t.Fatalf("register owned execution: ctx=%v owner=%v err=%v", ownedCtx != nil, owner != nil, err)
	}
	t.Cleanup(func() { c.ReleaseTurnExecutionOwnerV1(owner) })
	return ownedCtx, owner
}

func TestTurnExecutionOwnerV1BoundedWaitExpiresWithoutReleasingProducer(t *testing.T) {
	c := NewController(nil)
	_, owner := registerTurnExecutionOwnerTestV1(t, c, context.Background(), "thread", "turn", func() { t.Error("wait cancelled producer") })
	if err := owner.WaitBounded(context.Background(), time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("bounded handoff did not expire: %v", err)
	}
	assertTurnExecutionOwnerOpenV1(t, owner)
	if err := owner.WaitBounded(context.Background(), 0); !errors.Is(err, ErrTurnExecutionConflict) {
		t.Fatalf("invalid handoff duration accepted: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.ReleaseTurnExecutionOwnerV1(owner)
	if err := owner.WaitBounded(ctx, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("both-ready handoff ignored cancellation: %v", err)
	}
}

func TestTurnExecutionOwnerV1InvalidRegistrationHasNoSideEffects(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name      string
		ctx       context.Context
		threadID  string
		turnID    string
		nilCancel bool
	}{
		{name: "nil context", threadID: "thread", turnID: "turn"},
		{name: "cancelled context", ctx: cancelledCtx, threadID: "thread", turnID: "turn"},
		{name: "empty thread", ctx: context.Background(), turnID: "turn"},
		{name: "empty turn", ctx: context.Background(), threadID: "thread"},
		{name: "nil cancel", ctx: context.Background(), threadID: "thread", turnID: "turn", nilCancel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewController(&stubDriver{})
			cancelCalled := false
			cancelExecution := context.CancelFunc(func() { cancelCalled = true })
			if tc.nilCancel {
				cancelExecution = nil
			}
			_, owner, err := c.RegisterTurnCancelOwnedV1(tc.ctx, tc.threadID, tc.turnID, cancelExecution)
			if err == nil || owner != nil || c.ActiveTurnCount() != 0 || cancelCalled {
				t.Fatalf("invalid registration changed authority: owner=%v err=%v active=%d cancel=%v", owner != nil, err, c.ActiveTurnCount(), cancelCalled)
			}
			if err := c.RegisterTurnCancelWithError("thread", "turn", func() {}); err != nil {
				t.Fatalf("invalid registration left a conflicting reservation: %v", err)
			}
			if !c.UnregisterTurnCancel("thread", "turn") {
				t.Fatal("legacy cleanup failed")
			}
		})
	}
}

func TestTurnExecutionOwnerV1CaptureRequiresExactLivePrivateContext(t *testing.T) {
	c := NewController(&stubDriver{})
	other := NewController(&stubDriver{})
	ownedCtx, owner := registerTurnExecutionOwnerTestV1(t, c, context.Background(), "thread", "turn", func() {})
	var nilOwner *TurnExecutionOwnerV1
	if nilOwner.MatchesTurn("thread", "turn") || (&TurnExecutionOwnerV1{}).MatchesTurn("thread", "turn") {
		t.Fatal("nil or zero identity matched a turn")
	}
	if !owner.MatchesTurn(" thread ", " turn ") || owner.MatchesTurn("other", "turn") || owner.MatchesTurn("thread", "other") || owner.MatchesTurn("", "turn") {
		t.Fatal("immutable identity did not match exactly normalized thread and turn")
	}
	if captured, err := c.CaptureTurnExecutionOwnerV1(ownedCtx, "thread", "turn"); err != nil || captured != owner {
		t.Fatalf("exact owned context did not capture its owner: captured=%v err=%v", captured == owner, err)
	}
	cancelledCtx, cancel := context.WithCancel(ownedCtx)
	cancel()
	for _, tc := range []struct {
		name       string
		controller *Controller
		ctx        context.Context
		threadID   string
		turnID     string
	}{
		{name: "nil context", controller: c, threadID: "thread", turnID: "turn"},
		{name: "background context", controller: c, ctx: context.Background(), threadID: "thread", turnID: "turn"},
		{name: "other controller", controller: other, ctx: ownedCtx, threadID: "thread", turnID: "turn"},
		{name: "other thread", controller: c, ctx: ownedCtx, threadID: "other", turnID: "turn"},
		{name: "other turn", controller: c, ctx: ownedCtx, threadID: "thread", turnID: "other"},
		{name: "cancelled context", controller: c, ctx: cancelledCtx, threadID: "thread", turnID: "turn"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if captured, err := tc.controller.CaptureTurnExecutionOwnerV1(tc.ctx, tc.threadID, tc.turnID); err == nil || captured != nil {
				t.Fatalf("unproven context captured owner: captured=%v err=%v", captured != nil, err)
			}
		})
	}
	if !c.ReleaseTurnExecutionOwnerV1(owner) {
		t.Fatal("exact owner release failed")
	}
	if !owner.MatchesTurn("thread", "turn") {
		t.Fatal("release removed immutable identity needed by its captured gate")
	}
	if captured, err := c.CaptureTurnExecutionOwnerV1(ownedCtx, "thread", "turn"); err == nil || captured != nil {
		t.Fatalf("released proof remained valid: captured=%v err=%v", captured != nil, err)
	}
	newCtx, newOwner := registerTurnExecutionOwnerTestV1(t, c, context.Background(), "thread", "turn", func() {})
	if captured, err := c.CaptureTurnExecutionOwnerV1(ownedCtx, "thread", "turn"); err == nil || captured != nil {
		t.Fatalf("expired proof crossed same-key owner replacement: captured=%v err=%v", captured != nil, err)
	}
	if captured, err := c.CaptureTurnExecutionOwnerV1(newCtx, "thread", "turn"); err != nil || captured != newOwner {
		t.Fatalf("new owner proof was lost: captured=%v err=%v", captured == newOwner, err)
	}
}

func TestTurnExecutionOwnerV1LegacyAndOwnedReleaseRemainSeparate(t *testing.T) {
	c := NewController(&stubDriver{})
	if err := c.RegisterTurnCancelWithError("thread", "turn", func() {}); err != nil {
		t.Fatal(err)
	}
	if owner, err := c.CaptureTurnExecutionOwnerV1(context.Background(), "thread", "turn"); err == nil || owner != nil {
		t.Fatalf("legacy registration supplied capturable proof: owner=%v err=%v", owner != nil, err)
	}
	// Even an in-package fixture that obtains the legacy identity cannot turn
	// that identity into the private proof reserved for an owned registration.
	c.mu.Lock()
	legacyOwner := c.turnOwners[turnKey{threadID: "thread", turnID: "turn"}]
	c.mu.Unlock()
	if legacyOwner == nil {
		t.Fatal("legacy registration did not retain its own cleanup identity")
	}
	legacyCtx := context.WithValue(context.Background(), turnExecutionOwnerContextKeyV1{}, legacyOwner)
	if owner, err := c.CaptureTurnExecutionOwnerV1(legacyCtx, "thread", "turn"); err == nil || owner != nil {
		t.Fatalf("legacy identity was upgraded to an owned context proof: owner=%v err=%v", owner != nil, err)
	}
	if !c.UnregisterTurnCancel("thread", "turn") || c.UnregisterTurnCancel("thread", "turn") {
		t.Fatal("legacy unregister was not a one-time release")
	}
	ownedCtx, owner := registerTurnExecutionOwnerTestV1(t, c, context.Background(), "thread", "turn", func() {})
	if c.UnregisterTurnCancel("thread", "turn") || c.ActiveTurnCount() != 1 {
		t.Fatal("key-only compatibility cleanup released an owned registration")
	}
	assertTurnExecutionOwnerOpenV1(t, owner)
	if captured, err := c.CaptureTurnExecutionOwnerV1(ownedCtx, "thread", "turn"); err != nil || captured != owner {
		t.Fatalf("legacy cleanup damaged owned proof: captured=%v err=%v", captured == owner, err)
	}
	if c.RegisterTurnCancel("thread", "turn", func() {}) {
		t.Fatal("legacy registration replaced the live owned execution")
	}
}

func TestTurnExecutionOwnerV1StaleReleaseCannotClearReplacementOrCancellation(t *testing.T) {
	c := NewController(&stubDriver{})
	other := NewController(&stubDriver{})
	_, oldOwner := registerTurnExecutionOwnerTestV1(t, c, context.Background(), "thread", "turn", func() {})
	if other.ReleaseTurnExecutionOwnerV1(oldOwner) || c.ReleaseTurnExecutionOwnerV1(nil) || c.ReleaseTurnExecutionOwnerV1(&TurnExecutionOwnerV1{}) {
		t.Fatal("foreign, nil or zero identity released execution authority")
	}
	assertTurnExecutionOwnerOpenV1(t, oldOwner)
	if !c.ReleaseTurnExecutionOwnerV1(oldOwner) {
		t.Fatal("first exact release failed")
	}
	receiveTurnExecutionOwnerTestV1(t, oldOwner.Done())
	cancelRequested := make(chan struct{})
	_, newOwner := registerTurnExecutionOwnerTestV1(t, c, context.Background(), "thread", "turn", func() { close(cancelRequested) })
	if !c.CancelRegisteredTurn("thread", "turn") {
		t.Fatal("replacement owner did not accept cancellation intent")
	}
	receiveTurnExecutionOwnerTestV1(t, cancelRequested)
	if c.ReleaseTurnExecutionOwnerV1(oldOwner) || c.UnregisterTurnCancel("thread", "turn") {
		t.Fatal("stale or key-only cleanup released replacement owner")
	}
	assertTurnExecutionOwnerOpenV1(t, newOwner)
	c.mu.Lock()
	_, reservationPresent := c.cancelReservations[turnKey{threadID: "thread", turnID: "turn"}]
	c.mu.Unlock()
	if !reservationPresent || c.ActiveTurnCount() != 1 {
		t.Fatal("stale cleanup cleared replacement cancellation reservation")
	}
	if !c.ReleaseTurnExecutionOwnerV1(newOwner) || c.ReleaseTurnExecutionOwnerV1(newOwner) {
		t.Fatal("replacement exact release was not idempotent")
	}
	receiveTurnExecutionOwnerTestV1(t, newOwner.Done())
	c.mu.Lock()
	_, reservationPresent = c.cancelReservations[turnKey{threadID: "thread", turnID: "turn"}]
	c.mu.Unlock()
	if reservationPresent || c.ActiveTurnCount() != 0 {
		t.Fatal("actual replacement release retained its cancellation reservation")
	}
}

func TestTurnExecutionOwnerV1WaitRequiresActualReleaseAndDoesNotCancelWorker(t *testing.T) {
	for _, useController := range []bool{false, true} {
		name := "owner wait"
		if useController {
			name = "controller wait"
		}
		t.Run(name, func(t *testing.T) {
			c := NewController(&stubDriver{})
			workerCtx, cancelWorker := context.WithCancel(context.Background())
			defer cancelWorker()
			_, owner := registerTurnExecutionOwnerTestV1(t, c, workerCtx, "thread", "turn", cancelWorker)
			wait := owner.Wait
			if useController {
				wait = func(ctx context.Context) error { return c.WaitTurnExecutionOwnerV1(ctx, owner) }
			}
			waitCtx, cancelWait := context.WithCancel(context.Background())
			defer cancelWait()
			barrier := &turnExecutionOwnerWaitContextV1{Context: waitCtx, entered: make(chan struct{})}
			waited := make(chan error, 1)
			go func() { waited <- wait(barrier) }()
			receiveTurnExecutionOwnerTestV1(t, barrier.entered)
			cancelWait()
			if err := receiveTurnExecutionOwnerTestV1(t, waited); !errors.Is(err, context.Canceled) {
				t.Fatalf("wait cancellation = %v", err)
			}
			if workerCtx.Err() != nil {
				t.Fatal("canceling wait canceled execution")
			}
			assertTurnExecutionOwnerOpenV1(t, owner)
			barrier = &turnExecutionOwnerWaitContextV1{Context: context.Background(), entered: make(chan struct{})}
			go func() { waited <- wait(barrier) }()
			receiveTurnExecutionOwnerTestV1(t, barrier.entered)
			cancelWorker()
			assertTurnExecutionOwnerOpenV1(t, owner)
			select {
			case err := <-waited:
				t.Fatalf("cancelFunc alone completed owner wait: %v", err)
			default:
			}
			if !c.ReleaseTurnExecutionOwnerV1(owner) {
				t.Fatal("actual owner cleanup failed")
			}
			if err := receiveTurnExecutionOwnerTestV1(t, waited); err != nil {
				t.Fatalf("wait after actual cleanup = %v", err)
			}
			if err := wait(context.Background()); err != nil {
				t.Fatalf("released owner did not remain waitable: %v", err)
			}
		})
	}
}

func TestTurnExecutionOwnerV1WaitRejectsUnprovenAndSelfWait(t *testing.T) {
	c := NewController(&stubDriver{})
	other := NewController(&stubDriver{})
	ownedCtx, owner := registerTurnExecutionOwnerTestV1(t, c, context.Background(), "thread", "turn", func() {})
	var nilOwner *TurnExecutionOwnerV1
	zeroOwner := &TurnExecutionOwnerV1{}
	for _, tc := range []struct {
		name string
		wait func() error
	}{
		{name: "owner nil context", wait: func() error { return owner.Wait(nil) }},
		{name: "nil owner", wait: func() error { return nilOwner.Wait(context.Background()) }},
		{name: "zero owner", wait: func() error { return zeroOwner.Wait(context.Background()) }},
		{name: "controller nil owner", wait: func() error { return c.WaitTurnExecutionOwnerV1(context.Background(), nil) }},
		{name: "controller zero owner", wait: func() error { return c.WaitTurnExecutionOwnerV1(context.Background(), zeroOwner) }},
		{name: "foreign controller", wait: func() error { return other.WaitTurnExecutionOwnerV1(context.Background(), owner) }},
		{name: "owner self wait", wait: func() error { return owner.Wait(ownedCtx) }},
		{name: "controller self wait", wait: func() error { return c.WaitTurnExecutionOwnerV1(ownedCtx, owner) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			waited := make(chan error, 1)
			go func() { waited <- tc.wait() }()
			if err := receiveTurnExecutionOwnerTestV1(t, waited); err == nil {
				t.Fatal("invalid or self wait was accepted")
			}
			assertTurnExecutionOwnerOpenV1(t, owner)
		})
	}
}

func TestRunRegisteredTurnV1CarriesOwnedProofAndReleasesExactOwner(t *testing.T) {
	c := NewController(&stubDriver{})
	entered := make(chan *TurnExecutionOwnerV1, 1)
	finish := make(chan struct{})
	var finishOnce sync.Once
	finishRun := func() { finishOnce.Do(func() { close(finish) }) }
	t.Cleanup(finishRun)
	returned := make(chan ActionResult, 1)
	go func() {
		returned <- c.RunRegisteredTurn(context.Background(), "thread", "turn", func(ctx context.Context) ActionResult {
			owner, err := c.CaptureTurnExecutionOwnerV1(ctx, "thread", "turn")
			if err != nil || owner == nil {
				entered <- nil
				return ActionResult{StatusCode: 500}
			}
			entered <- owner
			<-finish
			return ActionResult{StatusCode: 207}
		})
	}()
	owner := receiveTurnExecutionOwnerTestV1(t, entered)
	if owner == nil {
		t.Fatal("RunRegisteredTurn callback did not receive owned proof")
	}
	if c.UnregisterTurnCancel("thread", "turn") {
		t.Fatal("legacy cleanup released running continuation")
	}
	called := false
	conflict := c.RunRegisteredTurn(context.Background(), "thread", "turn", func(context.Context) ActionResult {
		called = true
		return ActionResult{StatusCode: 200}
	})
	if called || conflict.StatusCode != 409 {
		t.Fatalf("conflicting continuation ran: called=%v result=%#v", called, conflict)
	}
	assertTurnExecutionOwnerOpenV1(t, owner)
	finishRun()
	if result := receiveTurnExecutionOwnerTestV1(t, returned); result.StatusCode != 207 {
		t.Fatalf("callback result was changed: %#v", result)
	}
	receiveTurnExecutionOwnerTestV1(t, owner.Done())
	if c.ActiveTurnCount() != 0 || c.ReleaseTurnExecutionOwnerV1(owner) {
		t.Fatal("RunRegisteredTurn retained or double-released its execution")
	}
	_, replacement := registerTurnExecutionOwnerTestV1(t, c, context.Background(), "thread", "turn", func() {})
	if c.ReleaseTurnExecutionOwnerV1(owner) {
		t.Fatal("continuation's stale identity released replacement")
	}
	assertTurnExecutionOwnerOpenV1(t, replacement)
}
