package checkpointauthority

import (
	"context"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	privatecasrecoverytest "analytix.local/runtime-go/internal/testsupport/privatecasrecovery"
)

func recoverIfPresentV4ForTest(
	ctx context.Context,
	root string,
	access privatecasport.RecoveryAccessAuthority,
) error {
	prepared, err := PrepareRecoveryV1(ctx, root, access)
	if err != nil {
		return err
	}
	if err := prepared.ValidateSemantics(ctx); err != nil {
		return err
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return err
	}
	if _, err := prepared.ApplyLegacyCheckpointQuarantineMigrationV1(ctx); err != nil {
		return err
	}
	refreshed, err := PrepareRecoveryV1(ctx, root, access)
	if err != nil {
		return err
	}
	if err := refreshed.ValidateSemantics(ctx); err != nil {
		return err
	}
	if err := refreshed.Revalidate(ctx); err != nil {
		return err
	}
	return privatecasrecoverytest.ApplyV4(ctx, "checkpoint-authority-test", refreshed)
}
