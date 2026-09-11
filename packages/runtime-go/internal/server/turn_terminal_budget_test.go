package server

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	controlapp "analytix.local/runtime-go/internal/app/control"
	effectgateapp "analytix.local/runtime-go/internal/app/effectgate"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	apploop "analytix.local/runtime-go/internal/app/loop"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
)

type terminalBudgetValueKeyV1 struct{}

func terminalBudgetFixtureV1(t *testing.T, general bool) (*runtimeServerHandler, *effectgateapp.Gate, domainsecurity.TurnSecurityContext, context.Context, func(), context.CancelFunc) {
	t.Helper()
	frozen := caseTerminalSecurityContext(t)
	if general {
		frozen = newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
			ThreadID: frozen.ThreadID, TurnID: frozen.TurnID, WorkspaceRealPath: frozen.WorkspaceRealPath,
			ContextEpoch: 1, IssuedAt: time.Now().UTC(),
		})
	}
	gate := effectgateapp.New()
	operation, cancel := context.WithCancel(context.WithValue(context.Background(), terminalBudgetValueKeyV1{}, "preserved"))
	t.Cleanup(cancel)
	control := controlapp.NewController(runtimeControlDriver{})
	if !control.RegisterTurnCancel(frozen.ThreadID, frozen.TurnID, cancel) {
		t.Fatal("terminal budget fixture execution owner is unavailable")
	}
	t.Cleanup(func() { control.UnregisterTurnCancel(frozen.ThreadID, frozen.TurnID) })
	effect, releaseEffect, err := gate.AcquireEffect(operation, frozen)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseEffect)
	claimed, releaseClaim, err := control.AcquireCandidateTerminalForLoop(effect, frozen.ThreadID, frozen.TurnID, frozen.ContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	release := func() { once.Do(func() { releaseClaim(); releaseEffect() }) }
	t.Cleanup(release)
	return &runtimeServerHandler{control: control}, gate, frozen, claimed, release, cancel
}

func TestRuntimeTerminalBudgetFollowsNormalizedDispositionV1(t *testing.T) {
	for _, disposition := range domainterminal.AllDispositionsV1() {
		t.Run(disposition.Reason, func(t *testing.T) {
			h, _, frozen, claimed, release, _ := terminalBudgetFixtureV1(t, false)
			result := runtimeAgentLoopResult{CandidateUsesCaseData: true}
			origin := time.Now().Add(-2 * time.Second)
			ctx, cancel := h.newRuntimeTerminalContextV1(claimed, origin, frozen, result, evidenceapp.TerminalReason(disposition.Reason), release)
			defer cancel()
			want := 15 * time.Second
			if disposition.CandidateAllowed {
				want = 60 * time.Second
			}
			deadline, ok := ctx.Deadline()
			if !ok || !deadline.Equal(origin.Add(want)) {
				t.Fatal("terminal disposition changed its closed budget or original deadline origin")
			}
			if !h.control.HasLiveCandidateTerminalForLoop(claimed, frozen.ThreadID, frozen.TurnID, frozen.ContextDigest) {
				t.Fatal("budget selection consumed the terminal claim")
			}
		})
	}
}

func TestRuntimeTerminalBudgetRejectsIneligibleHandoffsV1(t *testing.T) {
	for _, name := range []string{"general", "invalid_case", "ordinary", "paused", "source_unavailable", "missing_context", "missing_release", "released", "consumed", "other_controller", "wrong_turn", "wrong_digest", "cancelled", "recovered_initial", "recovered_continuation", "elapsed"} {
		t.Run(name, func(t *testing.T) {
			h, _, frozen, claimed, release, cancelOperation := terminalBudgetFixtureV1(t, name == "general")
			result := runtimeAgentLoopResult{CandidateUsesCaseData: true}
			reason := evidenceapp.TerminalSuccess
			origin := time.Now().Add(-2 * time.Second)
			switch name {
			case "invalid_case":
				frozen.ContextDigest = "invalid"
			case "ordinary":
				result.CandidateUsesCaseData = false
			case "paused":
				result.Paused = true
			case "source_unavailable":
				result.CaseSourceUnavailable = true
			case "missing_context":
				claimed = context.Background()
			case "missing_release":
				release = nil
			case "released":
				release()
			case "consumed":
				consumedRelease, err := h.control.AcquireCandidateTerminal(claimed, frozen.ThreadID, frozen.TurnID, frozen.ContextDigest)
				if err != nil {
					t.Fatal(err)
				}
				consumedRelease()
			case "other_controller":
				h.control = controlapp.NewController(runtimeControlDriver{})
			case "wrong_turn":
				if h.control.HasLiveCandidateTerminalForLoop(claimed, frozen.ThreadID, "other-turn", frozen.ContextDigest) {
					t.Fatal("live claim crossed turn identity")
				}
				frozen.TurnID = "other-turn"
			case "wrong_digest":
				if h.control.HasLiveCandidateTerminalForLoop(claimed, frozen.ThreadID, frozen.TurnID, "other-digest") {
					t.Fatal("live claim crossed context identity")
				}
				frozen.ContextDigest = "other-digest"
			case "cancelled":
				cancelOperation()
			case "recovered_initial":
				result.TerminalRecoveryKind = apploop.RuntimeTerminalRecoveryApplied
				reason = evidenceapp.TerminalReasonForRuntimeCompletion(result, false)
			case "recovered_continuation":
				result.TerminalRecoveryKind = apploop.RuntimeTerminalRecoveryApplied
				reason = evidenceapp.TerminalReasonForRuntimeResult(result, evidenceapp.TerminalResume)
			case "elapsed":
				origin = time.Now().Add(-61 * time.Second)
			}
			ctx, cancel := h.newRuntimeTerminalContextV1(claimed, origin, frozen, result, reason, release)
			defer cancel()
			want := 15 * time.Second
			if name == "elapsed" {
				want = 60 * time.Second
				if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
					t.Fatal("elapsed terminal budget was restarted")
				}
			} else if ctx.Err() != nil {
				t.Fatal("fixed terminal budget did not shield prior cancellation")
			}
			deadline, ok := ctx.Deadline()
			if !ok || !deadline.Equal(origin.Add(want)) {
				t.Fatal("ineligible terminal handoff selected an extended or renewed budget")
			}
		})
	}
}

func TestRuntimeTerminalBudgetRetainsLeaseAcrossCompleteTailV1(t *testing.T) {
	for _, reason := range []evidenceapp.TerminalReason{evidenceapp.TerminalSuccess, evidenceapp.TerminalResume} {
		for _, failed := range []bool{false, true} {
			name := string(reason) + "/success"
			if failed {
				name = string(reason) + "/failure"
			}
			t.Run(name, func(t *testing.T) {
				h, gate, frozen, claimed, release, cancelOperation := terminalBudgetFixtureV1(t, false)
				result := runtimeAgentLoopResult{CandidateUsesCaseData: true, CandidateTerminalContext: claimed, ReleaseCandidateTerminal: release}
				parent, releaseTail, err := takeRuntimeCandidateTerminal(context.Background(), &result)
				if err != nil || releaseTail == nil || result.CandidateTerminalContext != nil || result.ReleaseCandidateTerminal != nil {
					t.Fatal("terminal handoff was not transferred exactly once")
				}
				origin := time.Now().Add(-time.Second)
				ctx, cancel := h.newRuntimeTerminalContextV1(parent, origin, frozen, result, reason, releaseTail)
				defer cancel()
				deadline, _ := ctx.Deadline()
				if !deadline.Equal(origin.Add(60 * time.Second)) {
					t.Fatal("initial/continuation candidate budget differs")
				}
				cancelOperation()
				if ctx.Err() != nil || ctx.Value(terminalBudgetValueKeyV1{}) != "preserved" {
					t.Fatal("late cancellation lost protected terminal context")
				}
				// Final publication consumes the existing terminal token, while its
				// outer effect lease must remain held through the longitudinal append.
				consumedRelease, err := h.control.AcquireCandidateTerminal(ctx, frozen.ThreadID, frozen.TurnID, frozen.ContextDigest)
				if err != nil {
					t.Fatal(err)
				}
				consumedRelease()
				transitionCtx, cancelTransition := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancelTransition()
				transition := make(chan error, 1)
				go func() {
					unlock, err := gate.AcquireTransition(transitionCtx, frozen)
					if unlock != nil {
						unlock()
					}
					transition <- err
				}()
				select {
				case err := <-transition:
					t.Fatalf("authority transition crossed final-to-longitudinal tail: %v", err)
				case <-time.After(25 * time.Millisecond):
				}
				complete := func() error {
					defer releaseTail()
					defer cancel()
					after, _ := ctx.Deadline()
					if !after.Equal(deadline) || ctx.Err() != nil {
						return errors.New("terminal append renewed or lost its original budget")
					}
					if failed {
						return context.DeadlineExceeded
					}
					return nil
				}
				if err := complete(); (failed && !errors.Is(err, context.DeadlineExceeded)) || (!failed && err != nil) {
					t.Fatalf("terminal completion/failure changed: %v", err)
				}
				if err := <-transition; err != nil || !errors.Is(ctx.Err(), context.Canceled) {
					t.Fatal("terminal return failed to release the lease and owned deadline")
				}
			})
		}
	}
}
