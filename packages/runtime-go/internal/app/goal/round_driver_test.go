package goal

import (
	"analytix.local/runtime-go/internal/contracts"
	domaingoal "analytix.local/runtime-go/internal/domain/goal"
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func goalDriverWaitV1[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("Goal driver did not reach expected seam")
		var zero T
		return zero
	}
}
func TestGoalDriverColdReadAndEditDoNotArm(t *testing.T) {
	goal := lifecycleGoalV1(t)
	sent := make(chan *RoundAttemptV1, 1)
	driver := NewRoundDriverV1(DriverDependenciesV1{ReadGoal: func(string) (map[string]any, error) { return goal, nil }, Ready: func(string) bool { return true }, Send: func(_ context.Context, _ string, a *RoundAttemptV1) error { sent <- a; return nil }})
	defer driver.Close(context.Background())
	if driver.Activation("thread-goal", goal).Activation != "disarmed" {
		t.Fatal("cold goal armed")
	}
	patch := goalRefPatchV1(goal, "edit")
	patch["objective"] = "Changed cold objective"
	_, err := driver.Mutate("thread-goal", patch, func() (map[string]any, error) {
		next, err := ApplyLifecycleV1(StatePatchInput{Existing: goal, Patch: patch})
		if err == nil {
			goal = next
		}
		return next, err
	})
	if err != nil {
		t.Fatal(err)
	}
	driver.Kick("thread-goal")
	select {
	case <-sent:
		t.Fatal("cold edit started a turn")
	default:
	}
}
func TestGoalDriverDeferredIdleClaimRetainsActivationAndWake(t *testing.T) {
	goal := lifecycleGoalV1(t)
	var mu sync.Mutex
	var ready atomic.Bool
	ready.Store(true)
	busy := errors.New("synthetic idle claim conflict")
	var sends atomic.Int32
	deferred := make(chan struct{}, 1)
	admitted := make(chan *RoundAttemptV1, 1)
	driver := NewRoundDriverV1(DriverDependenciesV1{
		ReadGoal: func(string) (map[string]any, error) {
			mu.Lock()
			defer mu.Unlock()
			return contracts.CloneMap(goal), nil
		},
		Ready: func(string) bool { return ready.Load() }, Deferred: func(err error) bool { return errors.Is(err, busy) },
		Send: func(_ context.Context, _ string, a *RoundAttemptV1) error {
			if sends.Add(1) == 1 {
				ready.Store(false)
				return busy
			}
			admitted <- a
			return nil
		},
		Publish: func(_ string, a *domaingoal.ActivationV1) {
			if sends.Load() == 1 && a != nil && a.Activation == "armed" {
				select {
				case deferred <- struct{}{}:
				default:
				}
			}
		},
	})
	defer driver.Close(context.Background())
	patch := goalRefPatchV1(goal, "resume")
	_, err := driver.Mutate("thread-goal", patch, func() (map[string]any, error) {
		mu.Lock()
		defer mu.Unlock()
		next, err := ApplyLifecycleV1(StatePatchInput{Existing: goal, Patch: patch})
		if err == nil {
			goal = next
		}
		return next, err
	})
	if err != nil {
		t.Fatal(err)
	}
	goalDriverWaitV1(t, deferred)
	mu.Lock()
	snapshot := contracts.CloneMap(goal)
	mu.Unlock()
	_, _, started, known := domaingoal.CountedRecordV1(snapshot)
	if driver.Activation("thread-goal", snapshot).Activation != "armed" || !known || started != 0 {
		t.Fatal("rejected idle claim consumed activation or round", snapshot)
	}
	ready.Store(true)
	driver.Kick("thread-goal")
	a := goalDriverWaitV1(t, admitted)
	if a.Source().Round != 1 || a.Validate("thread-goal", a.Prompt()) != nil {
		t.Fatal("deferred wake lost exact source")
	}
}
func TestGoalDriverUnknownLifecycleCommitStopsOwnedExecution(t *testing.T) {
	goal := lifecycleGoalV1(t)
	bound := make(chan struct{}, 1)
	cancelled := make(chan struct{}, 1)
	driver := NewRoundDriverV1(DriverDependenciesV1{ReadGoal: func(string) (map[string]any, error) { return goal, nil }, Ready: func(string) bool { return true },
		Send: func(ctx context.Context, _ string, a *RoundAttemptV1) error {
			if err := a.BindExecutionCancel(func() {
				select {
				case cancelled <- struct{}{}:
				default:
				}
			}); err != nil {
				return err
			}
			bound <- struct{}{}
			<-ctx.Done()
			return ctx.Err()
		},
	})
	defer driver.Close(context.Background())
	patch := goalRefPatchV1(goal, "resume")
	_, err := driver.Mutate("thread-goal", patch, func() (map[string]any, error) {
		next, err := ApplyLifecycleV1(StatePatchInput{Existing: goal, Patch: patch})
		if err == nil {
			goal = next
		}
		return next, err
	})
	if err != nil {
		t.Fatal(err)
	}
	goalDriverWaitV1(t, bound)
	_, err = driver.Mutate("thread-goal", goalRefPatchV1(goal, "pause"), func() (map[string]any, error) { return nil, ErrGoalCommitUnknownV1 })
	if !errors.Is(err, ErrGoalCommitUnknownV1) {
		t.Fatal(err)
	}
	goalDriverWaitV1(t, cancelled)
	if driver.Activation("thread-goal", goal).Activation != "disarmed" {
		t.Fatal("unknown commit left goal armed")
	}
}

func TestGoalDriverInterruptedEditedAttemptStopsButOldAttemptCannotStopResume(t *testing.T) {
	for _, newerResume := range []bool{false, true} {
		t.Run(fmt.Sprint("newer_resume_", newerResume), func(t *testing.T) {
			goal := lifecycleGoalV1(t)
			var mu sync.Mutex
			sent := make(chan *RoundAttemptV1, 4)
			driver := NewRoundDriverV1(DriverDependenciesV1{
				ReadGoal: func(string) (map[string]any, error) {
					mu.Lock()
					defer mu.Unlock()
					return contracts.CloneMap(goal), nil
				},
				Ready: func(string) bool { return true },
				Send:  func(_ context.Context, _ string, a *RoundAttemptV1) error { sent <- a; return nil },
			})
			defer driver.Close(context.Background())
			mutate := func(operation string) {
				t.Helper()
				mu.Lock()
				patch := goalRefPatchV1(goal, operation)
				mu.Unlock()
				if operation == "edit" {
					patch["objective"] = "Edited running objective"
				}
				_, err := driver.Mutate("thread-goal", patch, func() (map[string]any, error) {
					mu.Lock()
					defer mu.Unlock()
					next, err := ApplyLifecycleV1(StatePatchInput{Existing: goal, Patch: patch})
					if err == nil {
						goal = next
					}
					return next, err
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			mutate("resume")
			old := goalDriverWaitV1(t, sent)
			if err := old.WithAdmission(func() (bool, error) {
				mu.Lock()
				defer mu.Unlock()
				goal["roundsStarted"] = old.Source().Round
				return true, nil
			}); err != nil {
				t.Fatal(err)
			}
			mutate("edit")
			current := old
			if newerResume {
				mutate("pause")
				mutate("resume")
				current = goalDriverWaitV1(t, sent)
			}
			driver.Finished("thread-goal", old, true)
			mu.Lock()
			snapshot := contracts.CloneMap(goal)
			mu.Unlock()
			if newerResume {
				if current == old || driver.Activation("thread-goal", snapshot).Activation != "armed" || current.Validate("thread-goal", current.Prompt()) != nil {
					t.Fatal("old completion stopped the new Resume attempt")
				}
			} else {
				if driver.Activation("thread-goal", snapshot).Activation != "disarmed" {
					t.Fatal("interrupted edited attempt remained armed")
				}
				driver.mu.Lock()
				running := driver.live["thread-goal"].attempt
				driver.mu.Unlock()
				if running != nil {
					t.Fatal("interrupted edited attempt started another round")
				}
			}
		})
	}
}
