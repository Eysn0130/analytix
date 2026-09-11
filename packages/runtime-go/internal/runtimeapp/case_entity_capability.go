package runtimeapp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	caseentitystore "analytix.local/runtime-go/internal/adapters/outbound/caseentity"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
)

type runtimeCaseEntityCapabilityV1 struct {
	store       *caseentitystore.Store
	hasRecords  bool
	semanticUse bool
}

// openRuntimeCaseEntityCapabilityV1 separates preservation from semantic use.
// Structurally authenticated but semantically invalid case-only records remain
// untouched; the ordinary Agent starts without the case-entity capability.
func openRuntimeCaseEntityCapabilityV1(
	ctx context.Context,
	root string,
	access finalauthority.SecurePrivateCASRecoveryAccessAuthority,
) (runtimeCaseEntityCapabilityV1, error) {
	prepared, err := caseentitystore.PrepareRecoveryV1(ctx, root, access)
	if err != nil {
		return runtimeCaseEntityCapabilityV1{}, fmt.Errorf("prepare runtime case entity capability: %w", err)
	}
	recovery := runtimePreparedPrivateCASOwnerRecovery(prepared)
	semanticUse := true
	if err := prepared.ValidateSemantics(ctx); err != nil {
		if _, domainOnly := err.(*finalauthority.DomainRecordUnavailableError); !domainOnly {
			return runtimeCaseEntityCapabilityV1{}, fmt.Errorf("validate runtime case entity capability: %w", err)
		}
		if contextErr := ctx.Err(); contextErr != nil {
			return runtimeCaseEntityCapabilityV1{}, contextErr
		}
		boundary, boundaryErr := newRuntimeCaseEntityBoundaryRecoveryV1(prepared)
		if boundaryErr != nil {
			return runtimeCaseEntityCapabilityV1{}, fmt.Errorf("prepare runtime case entity boundary: %w", boundaryErr)
		}
		recovery = boundary
		semanticUse = false
	}
	if err := recovery.Revalidate(ctx); err != nil {
		return runtimeCaseEntityCapabilityV1{}, fmt.Errorf("revalidate runtime case entity capability: %w", err)
	}
	hasRecords, err := runtimePreparedPrivateCASHasRecordsV1(ctx, recovery)
	if err != nil {
		return runtimeCaseEntityCapabilityV1{}, fmt.Errorf("inventory runtime case entity capability: %w", err)
	}
	if !semanticUse {
		return runtimeCaseEntityCapabilityV1{hasRecords: hasRecords}, nil
	}
	store, err := caseentitystore.NewStoreContext(ctx, filepath.Clean(root), access)
	if err != nil {
		return runtimeCaseEntityCapabilityV1{}, fmt.Errorf("open runtime case entity capability: %w", err)
	}
	return runtimeCaseEntityCapabilityV1{
		store: store, hasRecords: hasRecords, semanticUse: true,
	}, nil
}

func (capability *runtimeCaseEntityCapabilityV1) Close() error {
	if capability == nil || capability.store == nil {
		return nil
	}
	err := capability.store.Close()
	if err == nil {
		capability.store = nil
		capability.semanticUse = false
	}
	return err
}

func runtimePreparedPrivateCASHasRecordsV1(
	ctx context.Context,
	prepared runtimePreparedPrivateCASOwnerRecovery,
) (bool, error) {
	if prepared == nil {
		return false, errors.New("runtime private CAS owner inventory is unavailable")
	}
	found := false
	for _, plan := range prepared.SecurePrivateCASRecoveryPlansV2() {
		if plan == nil {
			return false, errors.New("runtime private CAS owner inventory has an invalid leaf")
		}
		err := plan.VisitCommittedFiles(ctx, func(finalauthority.SecurePrivateCASFile) error {
			found = true
			return nil
		})
		if err != nil {
			return false, err
		}
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return false, err
	}
	return found, nil
}
