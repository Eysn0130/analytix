package finalauthority

import (
	"context"
	"errors"
	"path/filepath"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

// EmptyPartialOwnerProofV1 proves only that one catalog owner has incomplete
// topology and no committed material. It cannot apply recovery or open a store.
// Startup must discard it after recovery and collect the repaired owner anew.
type EmptyPartialOwnerProofV1 struct {
	prepared     *PreparedSecurePrivateCASDirectoryRecoveryV1
	ownerRoot    string
	ownerBinding privatecasport.RootBinding
}

// A nil proof with no error means absent or non-partial topology. The caller
// must then prepare the normal owner; it is not evidence of empty inventory.
func PrepareEmptyPartialOwnerProofV1(ctx context.Context, dataDir, recoveryGroupID string, access SecurePrivateCASRecoveryAccessAuthority) (*EmptyPartialOwnerProofV1, error) {
	if !filepath.IsAbs(dataDir) || filepath.Clean(dataDir) != dataDir {
		return nil, errors.New("private CAS empty partial owner data root is invalid")
	}
	owner := ""
	for _, group := range domainprivatecas.RecoverableOwnerDirectoryGroupsV1() {
		if group.RecoveryGroupID == recoveryGroupID && !group.DeferOrphanRecovery {
			owner = group.OwnerRelativePath
			break
		}
	}
	if owner == "" {
		return nil, errors.New("private CAS empty partial owner is not an eligible catalog group")
	}
	ownerRoot := filepath.Join(dataDir, filepath.FromSlash(owner))
	var ownerBinding privatecasport.RootBinding
	if err := withExistingPrivateCASAccess(ctx, access, ownerRoot, func(binding privatecasport.RootBinding) error {
		ownerBinding = binding
		return nil
	}); err != nil {
		return nil, err
	}
	prepared, err := prepareSecurePrivateCASDirectoryRecoveryV1(ctx, dataDir, access, privateCASDirectoryRecoveryOrphanTopologyV1, owner)
	if err != nil {
		return nil, err
	}
	proof := &EmptyPartialOwnerProofV1{prepared: prepared, ownerRoot: ownerRoot, ownerBinding: ownerBinding}
	if err := proof.revalidateOwnerAccess(ctx); err != nil {
		return nil, err
	}
	if !privateCASPreparedPlanProvesEmptyPartialOwnerV1(prepared.plan, owner) {
		return nil, nil
	}
	return proof, nil
}

func (proof *EmptyPartialOwnerProofV1) Revalidate(ctx context.Context) error {
	if proof == nil || proof.prepared == nil || proof.prepared.ownerScope == "" || !privateCASPreparedPlanProvesEmptyPartialOwnerV1(proof.prepared.plan, proof.prepared.ownerScope) {
		return errors.New("private CAS empty partial owner proof is unavailable")
	}
	if err := proof.revalidateOwnerAccess(ctx); err != nil {
		return err
	}
	if err := proof.prepared.Revalidate(ctx); err != nil {
		return err
	}
	return proof.revalidateOwnerAccess(ctx)
}

func (proof *EmptyPartialOwnerProofV1) revalidateOwnerAccess(ctx context.Context) error {
	return withExistingPrivateCASAccess(ctx, proof.prepared.access, proof.ownerRoot, func(binding privatecasport.RootBinding) error {
		if binding != proof.ownerBinding {
			return errors.New("private CAS empty partial owner access binding changed")
		}
		return nil
	})
}
