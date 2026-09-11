package evidence

import (
	"testing"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	apploop "analytix.local/runtime-go/internal/app/loop"
)

type hostTerminalFailureCodeStub string

func (failure hostTerminalFailureCodeStub) Error() string { return "closed host boundary failure" }

func (failure hostTerminalFailureCodeStub) TerminalFailureCode() string { return string(failure) }

func TestToolSourceUnavailableMapsToSourceUnavailable(t *testing.T) {
	for _, code := range []string{
		"tool_source_unavailable", "execution_grant_source_unavailable", "execution_grant_server_identity_invalid",
		"execution_grant_server_mismatch", "execution_grant_connection_epoch_mismatch", "mcp_source_probe_mismatch", "execution_grant_schema_unavailable",
		"tool_pending_continuation_invalid",
	} {
		if got := TerminalReasonForFailure(apploop.TurnFailureError{Code: code}); got != TerminalSourceUnavailable {
			t.Fatalf("code %q mapped to %q, want %q", code, got, TerminalSourceUnavailable)
		}
	}
	if got := TerminalReasonForFailure(executiongrantapp.ValidationError{Code: "execution_grant_connection_epoch_mismatch"}); got != TerminalSourceUnavailable {
		t.Fatalf("typed execution-grant rejection mapped to %q", got)
	}
	if got := TerminalReasonForFailure(apploop.TurnFailureError{Code: "tool_failed"}); got != TerminalToolFailure {
		t.Fatalf("ordinary tool failure mapped to %q", got)
	}
}

func TestPostProviderHostFailuresMapToSemanticFailure(t *testing.T) {
	for _, code := range []string{
		"host_response_event_failed",
		"host_candidate_lifecycle_failed",
		"host_candidate_authority_failed",
		"host_candidate_steering_failed",
		"host_candidate_projection_failed",
		"host_candidate_publication_failed",
	} {
		if got := TerminalReasonForFailure(hostTerminalFailureCodeStub(code)); got != TerminalSemanticFailure {
			t.Fatalf("closed host failure %q mapped to %q, want %q", code, got, TerminalSemanticFailure)
		}
	}
}

func TestPreProviderContextHardLimitMapsToSemanticFailure(t *testing.T) {
	if got := TerminalReasonForFailure(hostTerminalFailureCodeStub("context_window_hard_limit")); got != TerminalSemanticFailure {
		t.Fatalf("pre-provider context admission mapped to %q, want %q", got, TerminalSemanticFailure)
	}
}

func TestRuntimeRecoveryCannotUpgradeEvidenceBearingTerminal(t *testing.T) {
	if got := TerminalReasonForRuntimeCompletion(apploop.RuntimeAgentLoopResult{}, false); got != TerminalSuccess {
		t.Fatalf("zero-value non-recovery completion mapped to %q", got)
	}
	for _, fallback := range []TerminalReason{TerminalSuccess, TerminalApproval, TerminalUserInput, TerminalResume, TerminalBackgroundCompletion} {
		if got := TerminalReasonForRuntimeResult(apploop.RuntimeAgentLoopResult{
			TerminalRecoveryKind: apploop.RuntimeTerminalRecoveryApplied,
		}, fallback); got != TerminalRecovery {
			t.Fatalf("recovery upgraded through %q to %q", fallback, got)
		}
		if got := TerminalReasonForRuntimeResult(apploop.RuntimeAgentLoopResult{
			TerminalRecoveryKind: apploop.RuntimeTerminalRecoveryStepLimit,
		}, fallback); got != TerminalStepLimit {
			t.Fatalf("step-limit recovery upgraded through %q to %q", fallback, got)
		}
	}
	if got := TerminalReasonForRuntimeResult(apploop.RuntimeAgentLoopResult{
		TerminalRecoveryKind: apploop.RuntimeTerminalRecoveryApplied,
	}, TerminalSourceUnavailable); got != TerminalSourceUnavailable {
		t.Fatalf("specific fail-closed reason was replaced by recovery: %q", got)
	}
	if got := TerminalReasonForRuntimeResult(apploop.RuntimeAgentLoopResult{
		TerminalRecoveryKind: "model_claimed_success",
	}, TerminalSuccess); got != TerminalSemanticFailure {
		t.Fatalf("unknown recovery state failed open as %q", got)
	}
	if got := TerminalReasonForRuntimeCompletion(apploop.RuntimeAgentLoopResult{}, true); got != TerminalSourceUnavailable {
		t.Fatalf("source-unavailable completion mapped to %q", got)
	}
}
