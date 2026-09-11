package loop

import (
	"context"
	"errors"
	"testing"

	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
)

func TestCandidateTerminalHandoffUsesHostDisposition(t *testing.T) {
	if err := RequireCandidateTerminalHandoff(false, nil, nil); err != nil {
		t.Fatalf("fixed disposition required a candidate handoff: %v", err)
	}
	if err := RequireCandidateTerminalHandoff(true, context.Background(), func() {}); err != nil {
		t.Fatalf("complete candidate handoff rejected: %v", err)
	}
	if err := RequireCandidateTerminalHandoff(true, context.Background(), nil); !errors.Is(err, ErrCandidateTerminalHandoffRequired) {
		t.Fatalf("candidate disposition without handoff error = %v", err)
	}
}

func TestTakeCandidateTerminalHandoffConsumesOrRejectsClaim(t *testing.T) {
	fallback := context.Background()
	if got, release, err := TakeCandidateTerminalHandoff(fallback, nil); err != nil || got != fallback || release != nil {
		t.Fatalf("nil result handoff = (%v, %t, %v)", got, release != nil, err)
	}

	released := false
	result := RuntimeAgentLoopResult{ReleaseCandidateTerminal: func() { released = true }}
	if _, release, err := TakeCandidateTerminalHandoff(fallback, &result); err == nil || release != nil || !released {
		t.Fatalf("release-only handoff was not rejected and released: release=%t released=%t err=%v", release != nil, released, err)
	}
	if result.CandidateTerminalContext != nil || result.ReleaseCandidateTerminal != nil {
		t.Fatalf("invalid handoff remained reusable: %#v", result)
	}

	released = false
	terminalContext := context.WithValue(fallback, struct{}{}, "terminal")
	result = RuntimeAgentLoopResult{CandidateTerminalContext: terminalContext, ReleaseCandidateTerminal: func() { released = true }}
	got, release, err := TakeCandidateTerminalHandoff(fallback, &result)
	if err != nil || got != terminalContext || release == nil || released || result.CandidateTerminalContext != nil || result.ReleaseCandidateTerminal != nil {
		t.Fatalf("complete handoff was not transferred exactly once: context=%v release=%t result=%#v err=%v", got, release != nil, result, err)
	}
	release()
	if !released {
		t.Fatal("transferred handoff did not retain its release capability")
	}
}

func TestCandidateTerminalHandoffValidatesOrdinaryProvenance(t *testing.T) {
	terminalContext := context.Background()
	release := func() {}
	for _, result := range []RuntimeAgentLoopResult{
		{CandidateOrdinaryWork: true},
		{CandidateInputClass: RuntimeCandidateInputClassOrdinaryOnly},
		{CandidateOrdinaryWork: true, CandidateInputClass: RuntimeCandidateInputClassOrdinaryOnly},
	} {
		if err := RequireCandidateTerminalHandoffForResult(true, result, terminalContext, release); err == nil {
			t.Fatalf("ordinary provenance without a typed slot was accepted: %#v", result)
		}
	}

	slot, err := domainordinaryresult.NewResultSlotV1("Updated the ordinary source file and ran its focused test.")
	if err != nil {
		t.Fatal(err)
	}
	ordinary := RuntimeAgentLoopResult{
		CandidateOrdinaryWork: true,
		CandidateInputClass:   RuntimeCandidateInputClassOrdinaryOnly,
		OrdinaryResult:        &slot,
	}
	if err := RequireCandidateTerminalHandoffForResult(true, ordinary, terminalContext, release); err != nil {
		t.Fatalf("complete typed ordinary candidate was rejected: %v", err)
	}
	if err := RequireCandidateTerminalHandoffForResult(false, ordinary, nil, nil); !errors.Is(err, ErrCandidateTerminalHandoffRequired) {
		t.Fatalf("typed ordinary candidate lost its handoff requirement: %v", err)
	}
	caseCandidate := RuntimeAgentLoopResult{CandidateUsesCaseData: true, CandidateOrdinaryWork: true}
	if err := RequireCandidateTerminalHandoffForResult(true, caseCandidate, terminalContext, release); err != nil {
		t.Fatalf("case-data candidate without an ordinary slot was rejected: %v", err)
	}
}
