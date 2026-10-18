package control

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestForegroundAuxiliaryWitnessIsBoundRevocableAndNeverWireAuthority(t *testing.T) {
	c := NewController(nil)
	current := controlGeneralContext(t, "thread", "turn")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	witness, release, err := c.PrepareForegroundWithWitness(ctx, "thread")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := json.Marshal(witness); err == nil {
		t.Fatal("reservation serialized")
	}
	if _, _, err := c.BeginAuxiliary(ctx, "thread"); err == nil {
		t.Fatal("generic auxiliary bypassed reservation")
	}
	if _, _, err := c.BeginAuxiliaryForForeground(ctx, current, nil); err == nil {
		t.Fatal("missing witness admitted")
	}
	if _, _, err := NewController(nil).BeginAuxiliaryForForeground(ctx, current, witness); err == nil {
		t.Fatal("other controller admitted")
	}
	other := controlGeneralContext(t, "other", "turn")
	if _, _, err := c.BeginAuxiliaryForForeground(ctx, other, witness); err == nil {
		t.Fatal("other thread admitted")
	}
	_, finish, err := c.BeginAuxiliaryForForeground(ctx, current, witness)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.BeginAuxiliaryForForeground(ctx, current, witness); err == nil {
		t.Fatal("concurrent auxiliary admitted")
	}
	finish()
	release()
	if witness.Validate(ctx, "thread") == nil {
		t.Fatal("released witness survived")
	}
	witness, release, err = c.PrepareForegroundWithWitness(ctx, "thread")
	if err != nil {
		t.Fatal(err)
	}
	next, releaseNext, err := c.PrepareForegroundWithWitness(ctx, "thread")
	if err != nil {
		t.Fatal(err)
	}
	if witness.Validate(ctx, "thread") == nil || next.Validate(ctx, "thread") == nil {
		t.Fatal("overlapping preparation admitted")
	}
	release()
	if next.Validate(ctx, "thread") != nil {
		t.Fatal("current sole reservation was lost")
	}
	cancel()
	if next.Validate(context.Background(), "thread") == nil {
		t.Fatal("canceled admission survived")
	}
	releaseNext()
}

func TestAuxiliaryReservationExcludesForegroundAndWaitsForClosure(t *testing.T) {
	c := NewController(nil)
	ctx, finish, err := c.BeginAuxiliary(context.Background(), "thread")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.BeginAuxiliary(context.Background(), "thread"); !errors.Is(err, ErrTurnExecutionConflict) {
		t.Fatal("parallel auxiliary admitted")
	}
	ready := make(chan func(), 1)
	go func() {
		release, err := c.PrepareForeground(context.Background(), "thread")
		if err == nil {
			ready <- release
		}
	}()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("foreground did not cancel auxiliary")
	}
	select {
	case <-ready:
		t.Fatal("foreground passed before closure")
	default:
	}
	if _, _, err := c.BeginAuxiliary(context.Background(), "thread"); err == nil {
		t.Fatal("canceled producer released its slot before closure")
	}
	finish()
	var release func()
	select {
	case release = <-ready:
	case <-time.After(time.Second):
		t.Fatal("foreground did not resume after closure")
	}
	if _, _, err := c.BeginAuxiliary(context.Background(), "thread"); err == nil {
		t.Fatal("baseline/registration gap admitted auxiliary")
	}
	if err := c.RegisterTurnCancelWithError("thread", "turn", func() {}); err != nil {
		t.Fatal(err)
	}
	release()
	if _, _, err := c.BeginAuxiliary(context.Background(), "thread"); err == nil {
		t.Fatal("foreground owner admitted auxiliary")
	}
	c.UnregisterTurnCancel("thread", "turn")
	_, done, err := c.BeginAuxiliary(context.Background(), "thread")
	if err != nil {
		t.Fatal(err)
	}
	done()
}

func TestAuxiliaryRegistrationAndShutdownCannotEscapeClosure(t *testing.T) {
	for _, registerTail := range []bool{false, true} {
		t.Run(map[bool]string{false: "immediate", true: "tail"}[registerTail], func(t *testing.T) {
			c := NewController(nil)
			ctx, finish, err := c.BeginAuxiliary(context.Background(), "thread")
			if err != nil {
				t.Fatal(err)
			}
			if registerTail {
				result := make(chan error, 1)
				go func() {
					result <- c.RegisterTurnCancelAfterThreadTail(context.Background(), "thread", "turn", func() {})
				}()
				select {
				case <-ctx.Done():
				case <-time.After(time.Second):
					t.Fatal("tail did not cancel")
				}
				select {
				case <-result:
					t.Fatal("tail skipped closure")
				default:
				}
				finish()
				if err := <-result; err != nil {
					t.Fatal(err)
				}
				c.UnregisterTurnCancel("thread", "turn")
			} else {
				if err := c.RegisterTurnCancelWithError("thread", "turn", func() {}); !errors.Is(err, ErrTurnExecutionConflict) {
					t.Fatal("immediate registration crossed auxiliary")
				}
				if ctx.Err() == nil {
					t.Fatal("immediate registration did not cancel")
				}
				finish()
			}
		})
	}
	c := NewController(nil)
	ctx, finish, err := c.BeginAuxiliary(context.Background(), "thread")
	if err != nil {
		t.Fatal(err)
	}
	if c.BeginShutdown() != 1 || ctx.Err() == nil {
		t.Fatal("shutdown missed auxiliary")
	}
	wait, cancel := context.WithCancel(context.Background())
	cancel()
	if c.WaitForTurnOperations(wait) == nil {
		t.Fatal("shutdown forgot pending closure")
	}
	finish()
	if c.WaitForTurnOperations(context.Background()) != nil {
		t.Fatal("shutdown did not settle")
	}
}
