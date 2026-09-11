package loop

import (
	"context"
	"errors"
)

var ErrCandidateTerminalHandoffRequired = errors.New("runtime candidate terminal handoff is required")

// TakeCandidateTerminalHandoff transfers the process-local terminal claim out
// of a loop result exactly once. A partial handoff is invalid; any release
// capability that can still be honored is consumed before returning failure.
func TakeCandidateTerminalHandoff(
	fallback context.Context,
	result *RuntimeAgentLoopResult,
) (context.Context, func(), error) {
	if result == nil {
		return fallback, nil, nil
	}
	terminalContext := result.CandidateTerminalContext
	release := result.ReleaseCandidateTerminal
	result.CandidateTerminalContext = nil
	result.ReleaseCandidateTerminal = nil
	if terminalContext == nil && release == nil {
		return fallback, nil, nil
	}
	if terminalContext == nil || release == nil {
		if release != nil {
			release()
		}
		return fallback, nil, errors.New("runtime candidate terminal handoff is invalid")
	}
	return terminalContext, release, nil
}

// RequireCandidateTerminalHandoff validates the transport capability against
// a host-owned disposition decision without importing terminal/evidence policy
// into the loop package.
func RequireCandidateTerminalHandoff(
	candidateAllowed bool,
	terminalContext context.Context,
	release func(),
) error {
	if !candidateAllowed {
		return nil
	}
	if terminalContext == nil || release == nil {
		return ErrCandidateTerminalHandoffRequired
	}
	return nil
}

func RequireCandidateTerminalHandoffForResult(
	candidateAllowed bool,
	result RuntimeAgentLoopResult,
	terminalContext context.Context,
	release func(),
) error {
	ordinaryCandidate := RuntimeResultCarriesOrdinaryCandidate(result)
	if result.OrdinaryResult != nil && !ordinaryCandidate {
		return errors.New("runtime ordinary candidate terminal handoff is invalid")
	}
	if candidateAllowed && !result.CandidateUsesCaseData && !ordinaryCandidate &&
		(result.CandidateOrdinaryWork || result.CandidateInputClass != "") {
		return errors.New("runtime ordinary candidate terminal provenance is incomplete")
	}
	if ordinaryCandidate && (terminalContext == nil || release == nil) {
		return ErrCandidateTerminalHandoffRequired
	}
	return RequireCandidateTerminalHandoff(candidateAllowed, terminalContext, release)
}
