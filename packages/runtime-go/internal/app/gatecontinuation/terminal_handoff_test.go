package gatecontinuation

import (
	"context"
	"errors"
	"testing"

	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	apploop "analytix.local/runtime-go/internal/app/loop"
	appmodel "analytix.local/runtime-go/internal/app/model"
)

func TestFinalizeHeldRuntimeLoopFailureConsumesTerminalHandoff(t *testing.T) {
	cause := errors.New("ordinary result projection failed")
	released := false
	finalizeCalls := 0
	result := apploop.RuntimeAgentLoopResult{
		CandidateTerminalContext: context.Background(),
		ReleaseCandidateTerminal: func() { released = true },
	}
	held, err := finalizeHeldRuntimeLoopFailure(
		context.Background(), appmodel.PendingToolCall{ThreadID: "thread-1", TurnID: "turn-1"}, result, cause,
		Dependencies{Finalize: func(
			_ context.Context,
			pending appmodel.PendingToolCall,
			candidate apploop.RuntimeAgentLoopResult,
			reason evidenceapp.TerminalReason,
		) error {
			finalizeCalls++
			if pending.ThreadID != "thread-1" || pending.TurnID != "turn-1" ||
				candidate.CandidateTerminalContext == nil || candidate.ReleaseCandidateTerminal == nil ||
				reason != evidenceapp.TerminalReasonForFailure(cause) {
				t.Fatalf("claimed terminal failure was not forwarded exactly: pending=%#v result=%#v reason=%q", pending, candidate, reason)
			}
			candidate.ReleaseCandidateTerminal()
			return nil
		}},
	)
	if err != nil || !held || finalizeCalls != 1 || !released {
		t.Fatalf("held terminal failure was not consumed: held=%v calls=%d released=%v err=%v", held, finalizeCalls, released, err)
	}
}

func TestFinalizeHeldRuntimeLoopFailureReleasesWhenFinalizerIsMissing(t *testing.T) {
	released := false
	held, err := finalizeHeldRuntimeLoopFailure(
		context.Background(), appmodel.PendingToolCall{}, apploop.RuntimeAgentLoopResult{
			ReleaseCandidateTerminal: func() { released = true },
		}, errors.New("failure"), Dependencies{},
	)
	if !held || err == nil || !released {
		t.Fatalf("missing finalizer stranded terminal handoff: held=%v released=%v err=%v", held, released, err)
	}

	held, err = finalizeHeldRuntimeLoopFailure(
		context.Background(), appmodel.PendingToolCall{}, apploop.RuntimeAgentLoopResult{}, errors.New("failure"), Dependencies{},
	)
	if held || err != nil {
		t.Fatalf("result without a terminal handoff was treated as held: held=%v err=%v", held, err)
	}
}
