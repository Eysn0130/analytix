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

func TestGoalDriverFinishedBeforeSendReturnRechecksReadyAndBudget(t *testing.T) {
	goal := lifecycleGoalV1(t)
	goal["maxGoalRounds"] = 2
	var mu sync.Mutex
	var ready atomic.Bool
	ready.Store(true)
	var sends, budgetBlocks atomic.Int32
	sent := make(chan *RoundAttemptV1, 2)
	notReady := make(chan struct{}, 1)
	allowSendReturn := make(chan struct{})
	firstSendReturning := make(chan struct{})
	var release sync.Once
	driver := NewRoundDriverV1(DriverDependenciesV1{
		ReadGoal: func(string) (map[string]any, error) {
			mu.Lock()
			defer mu.Unlock()
			return contracts.CloneMap(goal), nil
		},
		Ready: func(string) bool {
			value := ready.Load()
			if !value {
				select {
				case notReady <- struct{}{}:
				default:
				}
			}
			return value
		},
		Send: func(_ context.Context, _ string, a *RoundAttemptV1) error {
			index := sends.Add(1)
			if err := a.WithAdmission(func() (bool, error) {
				mu.Lock()
				defer mu.Unlock()
				goal["roundsStarted"] = a.Source().Round
				return true, nil
			}); err != nil {
				return err
			}
			if index == 1 {
				ready.Store(false)
				defer func() {
					ready.Store(true)
					close(firstSendReturning)
				}()
			}
			sent <- a
			if index == 1 {
				<-allowSendReturn
			}
			return nil
		},
		BlockBudget: func(_ string, expected domaingoal.RefV1) (map[string]any, error) {
			mu.Lock()
			defer mu.Unlock()
			ref, maximum, started, known := domaingoal.CountedRecordV1(goal)
			if !known || ref != expected || started != maximum {
				return nil, errors.New("budget block did not match the admitted rounds")
			}
			next, err := ApplyStatePatch(StatePatchInput{Existing: goal, Patch: map[string]any{"status": "blocked", "blockedReason": "Goal round budget exhausted"}, Now: "2026-10-18T00:00:00Z"})
			if err == nil {
				goal = next
				budgetBlocks.Add(1)
			}
			return contracts.CloneMap(goal), err
		},
	})
	defer func() {
		release.Do(func() { close(allowSendReturn) })
		_ = driver.Close(context.Background())
	}()
	patch := goalRefPatchV1(goal, "resume")
	if _, err := driver.Mutate("thread-goal", patch, func() (map[string]any, error) {
		mu.Lock()
		defer mu.Unlock()
		next, err := ApplyLifecycleV1(StatePatchInput{Existing: goal, Patch: patch, Now: "2026-10-18T00:00:00Z"})
		if err == nil {
			goal = next
		}
		return next, err
	}); err != nil {
		t.Fatal(err)
	}
	first := goalDriverWaitV1(t, sent)
	if first.Source().Round != 1 {
		t.Fatal("first admission did not consume round one")
	}
	driver.Finished("thread-goal", first, false)
	select {
	case <-notReady:
	default:
		t.Fatal("Finished did not observe preparation still holding readiness")
	}
	select {
	case <-firstSendReturning:
		t.Fatal("Send returned before the Finished barrier")
	default:
	}
	drained := make(chan struct{})
	go func() { driver.workers.Wait(); close(drained) }()
	release.Do(func() { close(allowSendReturn) })
	goalDriverWaitV1(t, drained)
	var second *RoundAttemptV1
	select {
	case second = <-sent:
	default:
		t.Fatalf("successful Send return lost the Finished wake: sends=%d", sends.Load())
	}
	if !ready.Load() || second.Source().Round != 2 || second.Validate("thread-goal", second.Prompt()) != nil || sends.Load() != 2 {
		t.Fatal("readiness recheck did not admit exactly round two")
	}
	driver.Finished("thread-goal", second, false)
	mu.Lock()
	snapshot := contracts.CloneMap(goal)
	mu.Unlock()
	_, maximum, started, known := domaingoal.CountedRecordV1(snapshot)
	if !known || maximum != 2 || started != 2 || snapshot["status"] != "blocked" || budgetBlocks.Load() != 1 ||
		driver.Activation("thread-goal", snapshot).Activation != "disarmed" || sends.Load() != 2 {
		t.Fatal("wake recheck exceeded or failed to block the round budget", snapshot, sends.Load(), budgetBlocks.Load())
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
