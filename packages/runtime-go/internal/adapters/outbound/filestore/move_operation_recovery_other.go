//go:build !darwin && !linux

package filestore

import (
	"context"
	"errors"

	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
	checkpointfileport "analytix.local/runtime-go/internal/ports/checkpointfile"
)

func prepareConditionalMoveOperationRecoveryPlatform(
	context.Context,
	CheckpointOperationObserver,
	domaincheckpoint.OperationGroupIntentV2,
) (checkpointfileport.PreparedOperationRecovery, error) {
	return nil, errors.New("conditional move startup recovery is unavailable on this platform")
}
