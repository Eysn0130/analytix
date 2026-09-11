package loop

import (
	"context"
	"errors"
	"strings"
	"testing"

	domainfailure "analytix.local/runtime-go/internal/domain/failure"
)

type existingTerminalFailureCodeStub struct{ code string }

func (failure existingTerminalFailureCodeStub) Error() string { return "existing terminal failure" }

func (failure existingTerminalFailureCodeStub) TerminalFailureCode() string { return failure.code }

func TestWrapHostBoundaryFailureProjectsOnlyClosedPhaseCodes(t *testing.T) {
	tests := []struct {
		phase HostBoundaryFailurePhase
		code  string
	}{
		{HostResponseEventFailure, domainfailure.CodeHostResponseEventFailed},
		{HostCandidateLifecycleFailure, domainfailure.CodeHostCandidateLifecycleFailed},
		{HostCandidateAuthorityFailure, domainfailure.CodeHostCandidateAuthorityFailed},
		{HostCandidateSteeringFailure, domainfailure.CodeHostCandidateSteeringFailed},
		{HostCandidateProjectionFailure, domainfailure.CodeHostCandidateProjectionFailed},
		{HostCandidatePublicationFailure, domainfailure.CodeHostCandidatePublicationFailed},
	}
	for _, test := range tests {
		t.Run(test.code, func(t *testing.T) {
			cause := errors.New("PRIVATE_HOST_CAUSE_SENTINEL")
			wrapped := WrapHostBoundaryFailure(test.phase, cause)
			if !errors.Is(wrapped, cause) || strings.Contains(wrapped.Error(), "PRIVATE_HOST_CAUSE_SENTINEL") {
				t.Fatalf("host failure did not retain only its private cause identity: %v", wrapped)
			}
			public := PublicFailureForError(wrapped)
			if public.Code() != test.code || public.Details() != nil || public.Message() != wrapped.Error() {
				t.Fatalf("host failure projection mismatch: %#v", public)
			}
		})
	}
}

func TestWrapHostBoundaryFailurePreservesNarrowerTypedFailures(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		if wrapped := WrapHostBoundaryFailure(HostCandidateAuthorityFailure, cause); !errors.Is(wrapped, cause) {
			t.Fatalf("context failure was replaced: cause=%T wrapped=%T", cause, wrapped)
		}
	}
	public := domainfailure.NewError(domainfailure.CodeProviderReasoningMarkupInvalid, nil)
	if wrapped := WrapHostBoundaryFailure(HostCandidateAuthorityFailure, public); PublicFailureForError(wrapped).Code() !=
		domainfailure.CodeProviderReasoningMarkupInvalid {
		t.Fatalf("public provider failure was replaced: %v", wrapped)
	}
	terminal := existingTerminalFailureCodeStub{code: "turn_security_context_invalid"}
	wrapped := WrapHostBoundaryFailure(HostCandidateAuthorityFailure, terminal)
	var coded interface{ TerminalFailureCode() string }
	if !errors.As(wrapped, &coded) || coded.TerminalFailureCode() != terminal.code {
		t.Fatalf("terminal security failure was replaced: %v", wrapped)
	}
}
