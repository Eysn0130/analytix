package control

import (
	"context"
	"errors"
	"testing"
	"time"
)

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
