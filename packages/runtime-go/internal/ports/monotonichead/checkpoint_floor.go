package monotonichead

import (
	"context"
	"errors"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var (
	ErrCheckpointFloorBootstrap     = errors.New("monotonic checkpoint floor bootstrap mismatch")
	ErrCheckpointFloorConflict      = errors.New("monotonic checkpoint floor conflicts with witness")
	ErrCheckpointFloorIndeterminate = errors.New("monotonic checkpoint floor is indeterminate")
	ErrCheckpointFloorUnavailable   = errors.New("monotonic checkpoint floor is unavailable")
)

// CheckpointFloor persists a lower bound selected by fresh witness
// observations. It can reject rollback but can never select a current head;
// callers must still perform a fresh Observe before every protected action.
type CheckpointFloor interface {
	ProjectWitnessSelected(context.Context, domainsecurity.MonotonicHeadCheckpointV1) error
}
