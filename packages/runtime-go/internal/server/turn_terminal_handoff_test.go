package server

import (
	"context"
	"errors"
	"testing"

	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	apploop "analytix.local/runtime-go/internal/app/loop"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
)

func TestRuntimeCandidateTerminalHandoffFollowsClosedDisposition(t *testing.T) {
	for _, disposition := range domainterminal.AllDispositionsV1() {
		disposition := disposition
		t.Run(disposition.Reason, func(t *testing.T) {
			reason := evidenceapp.TerminalReason(disposition.Reason)
			err := requireRuntimeCandidateTerminalHandoff(reason, context.Background(), nil)
			if disposition.CandidateAllowed {
				if !errors.Is(err, errRuntimeCandidateTerminalHandoffRequired) {
					t.Fatalf("candidate disposition without handoff error = %v", err)
				}
				if err := requireRuntimeCandidateTerminalHandoff(reason, context.Background(), func() {}); err != nil {
					t.Fatalf("complete candidate handoff rejected: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("fixed disposition required a candidate handoff: %v", err)
			}
		})
	}
}

func TestTakeRuntimeCandidateTerminalRejectsPartialHandoff(t *testing.T) {
	released := false
	result := runtimeAgentLoopResult{ReleaseCandidateTerminal: func() { released = true }}
	if _, release, err := takeRuntimeCandidateTerminal(context.Background(), &result); err == nil || release != nil || !released {
		t.Fatalf("release-only handoff was not rejected and released: release=%t released=%t err=%v", release != nil, released, err)
	}
	result = runtimeAgentLoopResult{CandidateTerminalContext: context.Background()}
	if _, release, err := takeRuntimeCandidateTerminal(context.Background(), &result); err == nil || release != nil {
		t.Fatalf("context-only handoff was not rejected: release=%t err=%v", release != nil, err)
	}
}

func TestRuntimeCandidateTerminalHandoffRejectsIncompleteLiveOrdinaryProvenance(t *testing.T) {
	terminalContext := context.Background()
	release := func() {}
	for _, disposition := range domainterminal.AllDispositionsV1() {
		if !disposition.CandidateAllowed {
			continue
		}
		for _, result := range []runtimeAgentLoopResult{
			{CandidateOrdinaryWork: true},
			{CandidateInputClass: apploop.RuntimeCandidateInputClassOrdinaryOnly},
			{CandidateOrdinaryWork: true, CandidateInputClass: apploop.RuntimeCandidateInputClassOrdinaryOnly},
		} {
			if err := requireRuntimeCandidateTerminalHandoffForResult(
				evidenceapp.TerminalReason(disposition.Reason), result, terminalContext, release,
			); err == nil {
				t.Fatalf("%s accepted ordinary provenance without a typed slot: %#v", disposition.Reason, result)
			}
		}
	}

	caseCandidate := runtimeAgentLoopResult{
		CandidateUsesCaseData: true,
		CandidateOrdinaryWork: true,
	}
	if err := requireRuntimeCandidateTerminalHandoffForResult(
		evidenceapp.TerminalSuccess, caseCandidate, terminalContext, release,
	); err != nil {
		t.Fatalf("case-data candidate without an ordinary slot was rejected: %v", err)
	}

	legacyCandidate := runtimeAgentLoopResult{}
	if err := requireRuntimeCandidateTerminalHandoffForResult(
		evidenceapp.TerminalSuccess, legacyCandidate, terminalContext, release,
	); err != nil {
		t.Fatalf("legacy candidate without typed ordinary provenance was rejected: %v", err)
	}

	fixedOrdinary := runtimeAgentLoopResult{
		CandidateOrdinaryWork: true,
		CandidateInputClass:   apploop.RuntimeCandidateInputClassOrdinaryOnly,
	}
	if err := requireRuntimeCandidateTerminalHandoffForResult(
		evidenceapp.TerminalSourceUnavailable, fixedOrdinary, nil, nil,
	); err != nil {
		t.Fatalf("fixed recovery path was forced through live candidate publication: %v", err)
	}
}

func TestRuntimeCandidateTerminalHandoffAcceptsCompleteTypedOrdinaryProvenance(t *testing.T) {
	slot, err := domainordinaryresult.NewResultSlotV1("Updated the ordinary source file and ran its focused test.")
	if err != nil {
		t.Fatal(err)
	}
	result := runtimeAgentLoopResult{
		CandidateOrdinaryWork: true,
		CandidateInputClass:   apploop.RuntimeCandidateInputClassOrdinaryOnly,
		OrdinaryResult:        &slot,
	}
	if err := requireRuntimeCandidateTerminalHandoffForResult(
		evidenceapp.TerminalSuccess, result, context.Background(), func() {},
	); err != nil {
		t.Fatalf("complete typed ordinary candidate was rejected: %v", err)
	}
}

func TestRuntimeCaseSlotIntentIsTurnCapabilityScoped(t *testing.T) {
	slot, err := domainordinaryresult.NewResultSlotV1("The ordinary read completed.")
	if err != nil {
		t.Fatal(err)
	}
	ordinary := runtimeAgentLoopResult{
		CandidateOrdinaryWork: true,
		CandidateInputClass:   apploop.RuntimeCandidateInputClassOrdinaryOnly,
		OrdinaryResult:        &slot,
	}
	tests := map[string]struct {
		policy            apploop.CaseFundAnalysisPolicy
		sourceUnavailable bool
		reportRequested   bool
		result            runtimeAgentLoopResult
		want              evidenceapp.CaseSlotPublicationIntentV1
	}{
		"ordinary candidate": {
			result: ordinary, want: evidenceapp.CaseSlotNotRequestedV1,
		},
		"ordinary candidate with legacy source boundary": {
			sourceUnavailable: true, result: ordinary, want: evidenceapp.CaseSlotNotRequestedV1,
		},
		"active funds capability": {
			policy: apploop.CaseFundAnalysisPolicy{Active: true}, result: ordinary, want: evidenceapp.CaseSlotRequestedV1,
		},
		"source boundary without ordinary candidate": {
			sourceUnavailable: true, want: evidenceapp.CaseSlotRequestedV1,
		},
		"report slot": {
			reportRequested: true, result: ordinary, want: evidenceapp.CaseSlotRequestedV1,
		},
		"missing provenance": {
			result: runtimeAgentLoopResult{OrdinaryResult: &slot}, want: evidenceapp.CaseSlotRequestedV1,
		},
	}
	for name, test := range tests {
		got := evidenceapp.CaseSlotPublicationIntentForTurnV1(
			apploop.RuntimeResultCarriesOrdinaryCandidate(test.result), test.policy.Active, test.reportRequested,
		)
		if got != test.want {
			t.Fatalf("%s case-slot intent=%q want=%q", name, got, test.want)
		}
	}
}
