package server

import (
	"context"
	"errors"
	"testing"
	"time"

	controlapp "analytix.local/runtime-go/internal/app/control"
	goalapp "analytix.local/runtime-go/internal/app/goal"
	turnstartapp "analytix.local/runtime-go/internal/app/turnstart"
	domaingoal "analytix.local/runtime-go/internal/domain/goal"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestGoalPrimaryWriteSerializesForegroundReservationRevocation(t *testing.T) {
	for _, humanFirst := range []bool{true, false} {
		name := "primary_first"
		if humanFirst {
			name = "human_first"
		}
		t.Run(name, func(t *testing.T) {
			store, err := NewTempDurableEventSessionStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			workspace := workspacetest.New(t)
			thread, err := store.CreateThread(map[string]any{"title": "Goal atomic fixture"}, workspace)
			if err != nil {
				t.Fatal(err)
			}
			threadID := stringField(thread, "id")
			sent := make(chan *goalapp.RoundAttemptV1, 1)
			driver := goalapp.NewRoundDriverV1(goalapp.DriverDependenciesV1{ReadGoal: store.GetGoal, Ready: func(string) bool { return true }, Send: func(_ context.Context, _ string, a *goalapp.RoundAttemptV1) error { sent <- a; return nil }})
			defer driver.Close(context.Background())
			patch := map[string]any{"operation": "create", "objective": "Synthetic atomic objective", "maxGoalRounds": 2}
			if _, err := driver.Mutate(threadID, patch, func() (map[string]any, error) { return store.SetGoal(threadID, patch) }); err != nil {
				t.Fatal(err)
			}
			var attempt *goalapp.RoundAttemptV1
			select {
			case attempt = <-sent:
			case <-time.After(2 * time.Second):
				t.Fatal("Goal attempt not prepared")
			}
			controller := controlapp.NewController(nil)
			reservation, release, err := controller.PrepareGoalForegroundWithWitnessV1(context.Background(), threadID)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			if err := controller.RegisterTurnCancelWithError(threadID, "goal-turn", func() {}); err != nil {
				t.Fatal(err)
			}
			defer controller.UnregisterTurnCancel(threadID, "goal-turn")
			releaseTransition, err := controller.ReserveThreadTransition(threadID)
			if err != nil {
				t.Fatal(err)
			}
			defer releaseTransition()
			if err := reservation.Validate(context.Background(), threadID); err != nil {
				t.Fatal(err)
			}
			if err := attempt.Validate(threadID, attempt.Prompt()); err != nil {
				t.Fatal(err)
			}
			before, digest, err := store.ReadThreadStartBaseline(threadID)
			if err != nil {
				t.Fatal(err)
			}
			now := "2026-10-11T00:00:00Z"
			turn := map[string]any{"id": "goal-turn", "threadId": threadID, "status": "running", "createdAt": now, "items": []any{map[string]any{"id": "goal-user", "kind": "user_message", "role": "user", "status": "completed", "threadId": threadID, "turnId": "goal-turn", "text": attempt.Prompt(), "createdAt": now, "finishedAt": now, "source": domaingoal.RoundSourceRecordV1(attempt.Source())}}}
			if humanFirst {
				_, releaseHuman, err := controller.PrepareForegroundWithWitness(context.Background(), threadID)
				if err != nil {
					t.Fatal(err)
				}
				if err := controller.RegisterTurnCancelWithError(threadID, "human-turn", func() {}); !errors.Is(err, controlapp.ErrTurnExecutionConflict) {
					t.Fatal("human bypassed Goal transition", err)
				}
				releaseHuman()
			}
			writeCalls := 0
			authorize := func(write func() error) error {
				return reservation.WithCurrentV1(context.Background(), threadID, func() error {
					writeCalls++
					if humanFirst {
						return write()
					}
					started, prepared := make(chan struct{}), make(chan func(), 1)
					go func() {
						close(started)
						_, releaseHuman, err := controller.PrepareForegroundWithWitness(context.Background(), threadID)
						if err != nil {
							prepared <- nil
						} else {
							prepared <- releaseHuman
						}
					}()
					<-started
					select {
					case releaseHuman := <-prepared:
						if releaseHuman != nil {
							releaseHuman()
						}
						t.Fatal("human revoked reservation inside atomic primary write")
					case <-time.After(25 * time.Millisecond):
					}
					// Join after WithCurrentV1 releases Controller.mu, without
					// taking it recursively inside the protected write callback.
					t.Cleanup(func() {
						select {
						case releaseHuman := <-prepared:
							if releaseHuman == nil {
								t.Error("human preparation failed")
							} else {
								releaseHuman()
							}
						case <-time.After(2 * time.Second):
							t.Error("human remained blocked after primary write")
						}
					})
					return write()
				})
			}
			outcome, appendErr := store.AppendGoalTurnIfBaselineV1(threadID, turn, "", nil, workspace, digest, attempt, attempt.Prompt(), authorize)
			after, _, err := store.ReadThreadStartBaseline(threadID)
			if err != nil {
				t.Fatal(err)
			}
			if humanFirst {
				if appendErr == nil || outcome.Committed || writeCalls != 0 || goalapp.SemanticStateHashV1(before) != goalapp.SemanticStateHashV1(after) {
					t.Fatal("revoked foreground admission changed primary Goal/source/count/history", outcome, appendErr, writeCalls)
				}
			} else if appendErr != nil || !outcome.Committed || writeCalls != 1 || !turnstartapp.GoalPrimaryWitnessV1(after, turn, attempt.Source(), attempt.Prompt()) {
				t.Fatal("protected primary did not admit exactly one actual Goal round", outcome, appendErr, writeCalls)
			}
		})
	}
}
