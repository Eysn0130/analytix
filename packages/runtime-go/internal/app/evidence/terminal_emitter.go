package evidence

import (
	"context"
	"errors"
)

// PersistCaseTerminalBoundary is the only production call edge from a case
// terminal emitter into the finalizer. Keeping that edge explicit lets the
// architecture test reject new direct finalizer calls outside decorators.
func PersistCaseTerminalBoundary(
	ctx context.Context,
	finalizer CasePublicationFinalizer,
	input PersistCaseBoundaryInput,
) (PersistCaseBoundaryResult, error) {
	if finalizer == nil || !validTerminalReason(input.TerminalReason) || !productionTurnTerminalReasonV1(input.TerminalReason) {
		return PersistCaseBoundaryResult{}, errors.New("case terminal emitter authority is invalid")
	}
	return finalizer.PersistBoundary(ctx, input)
}
