package runtimeapp

import (
	"context"
	"errors"
	"path/filepath"

	authorityadvancefs "analytix.local/runtime-go/internal/adapters/outbound/authorityadvancefs"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	authorityadvanceapp "analytix.local/runtime-go/internal/app/authorityadvance"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

type runtimeAuthorityAdvanceStartupV2 struct {
	prepared      *authorityadvancefs.PreparedRecoveryV1
	inventory     *authorityadvancefs.PreparedInventoryV2
	revalidateKey func(context.Context) error
}

// The Core journal must be complete and signed by the current local authority
// before recovery can alter any owner. This does not retry an unsettled intent.
func prepareRuntimeAuthorityAdvanceStartupV2(ctx context.Context, roots persistencefs.RootSet, rootAuthority *persistencefs.RootAuthority, access finalauthority.SecurePrivateCASRecoveryAccessAuthority, installation *finalauthority.AnchoredFileAuthority) (*runtimeAuthorityAdvanceStartupV2, error) {
	prepared, err := authorityadvancefs.PrepareRecoveryV1(ctx, filepath.Join(roots.DataDir, "private", "authority-advance"), access)
	if err != nil {
		return nil, err
	}
	inventory, err := prepared.SnapshotInventory(ctx)
	if err != nil {
		return nil, err
	}
	result := &runtimeAuthorityAdvanceStartupV2{prepared: prepared, inventory: inventory}
	if inventory.HasRecords() {
		var verification authorityport.Authority
		if installation == nil {
			key, err := finalauthority.OpenExistingFileVerificationV1(rootAuthority)
			if err != nil {
				return nil, err
			}
			verification, result.revalidateKey = key, key.Revalidate
		} else {
			verification, result.revalidateKey = installation, installation.ValidateCurrentInstallation
		}
		if err := authorityadvanceapp.VerifyTrustedInventoryV2(ctx, inventory, inventory, verification); err != nil {
			return nil, err
		}
	}
	if err := result.revalidate(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func (prepared *runtimeAuthorityAdvanceStartupV2) revalidate(ctx context.Context) error {
	if prepared == nil || prepared.prepared == nil || prepared.inventory == nil {
		return errors.New("Core authority advance startup inventory is unavailable")
	}
	if prepared.revalidateKey != nil {
		if err := prepared.revalidateKey(ctx); err != nil {
			return err
		}
	}
	return prepared.prepared.Revalidate(ctx)
}
