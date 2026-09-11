package datasetsnapshot

import (
	"context"
	"errors"

	registryapp "analytix.local/runtime-go/internal/app/evidenceregistry"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
)

// UseExactRegistryCommit keeps the immutable source graph exact while
// accepting only the registry Service's own single CAS. UseExact remains the
// unchanged read-only operation. This capability is never advanced for reuse.
func (capability *currentSelectionCapabilityV2) UseExactRegistryCommit(
	selection datasetsnapshotport.CurrentSelectionV2,
	securityContext domainsecurity.TurnSecurityContext,
	prepared domainevidence.PreparedEvidenceSettlement,
	marker domainevidence.HostEvidenceSettlementMarker,
	use func(context.Context) error,
) error {
	if capability == nil || use == nil || !capability.beginRegistryEffectV1() {
		return errors.Join(datasetsnapshotport.ErrUnavailable, errors.New("dataset registry effect capability is inactive"))
	}
	defer capability.endUse()
	if capability.service == nil || securityContext != capability.securityContext ||
		selection.SelectionDigest != capability.selection.SelectionDigest ||
		capability.service.validateCurrentSelectionV2(selection, capability.input, securityContext) != nil {
		return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset registry effect selection is not exact"))
	}
	contentDigest, err := datasetsnapshotport.CanonicalCurrentSelectionContentDigestV2(selection)
	if err != nil || domainevidence.ValidatePreparedEvidenceSettlementForCurrentHostAuthorityV2(
		prepared, securityContext, prepared.SourceProbe, selection.SelectionDigest, contentDigest,
	) != nil {
		return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset registry effect prepared graph is not exact"))
	}
	coordinator, ok := capability.service.coordinator.(evidenceauthorityport.RegistryCoordinator)
	if !ok {
		return datasetsnapshotport.ErrUnavailable
	}
	lease, cancel := context.WithCancel(capability.ctx)
	defer cancel()
	if _, err := capability.service.confirmSharedEvidenceHeadV2(lease, capability.selection.Head); err != nil {
		return err
	}
	after, err := registryapp.WithPreparedCommitEffectV1(lease, coordinator, capability.selection.Head, prepared, marker, use)
	if err != nil {
		return err
	}
	// The head is returned directly by the concrete registry operation, never
	// by use. Fresh observation must match that exact signed CAS result.
	materialErr := capability.service.verifyCurrentSelectionMaterialV2(lease, selection, capability.input)
	_, currentErr := capability.service.confirmSharedEvidenceHeadV2(lease, after)
	return errors.Join(currentErr, materialErr, lease.Err())
}

func (capability *currentSelectionCapabilityV2) beginRegistryEffectV1() bool {
	capability.mu.Lock()
	defer capability.mu.Unlock()
	if !capability.active || capability.ctx == nil || capability.ctx.Err() != nil || capability.activeUses != 0 || capability.registryEffectUsed {
		return false
	}
	capability.registryEffectUsed = true
	capability.activeUses++
	return true
}
