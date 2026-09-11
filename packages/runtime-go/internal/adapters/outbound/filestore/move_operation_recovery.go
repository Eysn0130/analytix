package filestore

import (
	"context"

	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
	checkpointfileport "analytix.local/runtime-go/internal/ports/checkpointfile"
)

var _ checkpointfileport.OperationRecoveryPlanner = CheckpointOperationObserver{}

func (observer CheckpointOperationObserver) PrepareOperationRecovery(
	ctx context.Context,
	intent domaincheckpoint.OperationGroupIntentV2,
) (checkpointfileport.PreparedOperationRecovery, error) {
	return prepareConditionalMoveOperationRecoveryPlatform(ctx, observer, intent)
}
