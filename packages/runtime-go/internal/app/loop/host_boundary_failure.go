package loop

import (
	"context"
	"errors"

	domainfailure "analytix.local/runtime-go/internal/domain/failure"
)

type HostBoundaryFailurePhase uint8

const (
	HostResponseEventFailure HostBoundaryFailurePhase = iota + 1
	HostCandidateLifecycleFailure
	HostCandidateAuthorityFailure
	HostCandidateSteeringFailure
	HostCandidateProjectionFailure
	HostCandidatePublicationFailure
)

type hostBoundaryFailure struct {
	code  string
	cause error
}

func (failure hostBoundaryFailure) Error() string {
	return domainfailure.New(failure.code, nil).Message()
}

func (failure hostBoundaryFailure) Unwrap() error { return failure.cause }

func (failure hostBoundaryFailure) TerminalFailureCode() string { return failure.code }

func (failure hostBoundaryFailure) PublicFailureRecord() domainfailure.Record {
	return domainfailure.New(failure.code, nil)
}

// WrapHostBoundaryFailure attaches one closed, fact-free host phase to an
// otherwise untyped internal error. Existing typed provider, tool, security,
// cancellation, and timeout failures keep their narrower public contract.
// Raw causes remain process-private and Error returns only the fixed message.
func WrapHostBoundaryFailure(phase HostBoundaryFailurePhase, cause error) error {
	if cause == nil {
		return nil
	}
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return cause
	}
	var public interface{ PublicFailureRecord() domainfailure.Record }
	if errors.As(cause, &public) {
		return cause
	}
	var terminal interface{ TerminalFailureCode() string }
	if errors.As(cause, &terminal) {
		return cause
	}
	code := hostBoundaryFailureCode(phase)
	if code == "" {
		code = domainfailure.CodeHostCandidateLifecycleFailed
	}
	return hostBoundaryFailure{code: code, cause: cause}
}

func hostBoundaryFailureCode(phase HostBoundaryFailurePhase) string {
	switch phase {
	case HostResponseEventFailure:
		return domainfailure.CodeHostResponseEventFailed
	case HostCandidateLifecycleFailure:
		return domainfailure.CodeHostCandidateLifecycleFailed
	case HostCandidateAuthorityFailure:
		return domainfailure.CodeHostCandidateAuthorityFailed
	case HostCandidateSteeringFailure:
		return domainfailure.CodeHostCandidateSteeringFailed
	case HostCandidateProjectionFailure:
		return domainfailure.CodeHostCandidateProjectionFailed
	case HostCandidatePublicationFailure:
		return domainfailure.CodeHostCandidatePublicationFailed
	default:
		return ""
	}
}
