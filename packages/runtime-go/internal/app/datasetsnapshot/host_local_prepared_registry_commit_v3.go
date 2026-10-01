package datasetsnapshot

import (
	"context"
	"errors"
	"reflect"

	registryapp "analytix.local/runtime-go/internal/app/evidenceregistry"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
)

// UseExactRegistryCommit authorizes one real host-local registry CAS while
// the shared child-writer serializer is held. Its returned signed head is
// checked against the unchanged selected DSV2 graph.
func (capability *hostLocalCurrentSelectionCapabilityV2) UseExactRegistryCommit(
	selection datasetsnapshotport.CurrentSelectionV2,
	securityContext domainsecurity.TurnSecurityContext,
	prepared domainevidence.PreparedEvidenceSettlement,
	marker domainevidence.HostEvidenceSettlementMarker,
	use func(context.Context) error,
) error {
	if capability == nil || use == nil || !capability.beginRegistryEffect() {
		return datasetsnapshotport.ErrUnavailable
	}
	defer capability.endUse()
	if securityContext != capability.securityContext || selection.HostLocalHead == nil ||
		selection.SelectionDigest != capability.selection.SelectionDigest ||
		capability.service.validateHostCurrentSelectionV2(selection, capability.input, securityContext) != nil ||
		!reflect.DeepEqual(selection, capability.selection) {
		return datasetsnapshotport.ErrMismatch
	}
	contentDigest, err := datasetsnapshotport.CanonicalCurrentSelectionContentDigestV2(selection)
	if err != nil || domainevidence.ValidatePreparedEvidenceSettlementForCurrentHostAuthorityV2(
		prepared, securityContext, prepared.SourceProbe, selection.SelectionDigest, contentDigest) != nil {
		return errors.Join(datasetsnapshotport.ErrMismatch, err)
	}
	lease, cancel := context.WithCancel(capability.ctx)
	defer cancel()
	if err := capability.service.confirmHostDatasetHead(lease, *capability.selection.HostLocalHead); err != nil {
		return err
	}
	after, err := registryapp.WithHostLocalPreparedCommitEffectV3(lease,
		capability.service.heads, *capability.selection.HostLocalHead, prepared, marker, use)
	if err != nil {
		return err
	}
	if after.DatasetSnapshotIndexDigest != selection.HostLocalHead.DatasetSnapshotIndexDigest ||
		after.DatasetSnapshotCount != selection.HostLocalHead.DatasetSnapshotCount ||
		capability.service.confirmHostDatasetHead(lease, after) != nil {
		return datasetsnapshotport.ErrStale
	}
	node, err := capability.service.resolveVersionedNodeV2(lease,
		selection.SelectedIndex, true, capability.service.modeCommitmentDigest)
	if err != nil {
		return err
	}
	resolved, err := capability.service.resolveHostNodeMaterialV2(lease, node, capability.input)
	if err != nil || !reflect.DeepEqual(resolved, selection.Snapshot) {
		return errors.Join(datasetsnapshotport.ErrCorrupt, err)
	}
	return lease.Err()
}

func (capability *hostLocalCurrentSelectionCapabilityV2) beginRegistryEffect() bool {
	capability.mu.Lock()
	defer capability.mu.Unlock()
	if !capability.active || capability.ctx.Err() != nil || capability.activeUses != 0 ||
		capability.registryEffectUsed {
		return false
	}
	capability.registryEffectUsed = true
	capability.activeUses++
	return true
}
