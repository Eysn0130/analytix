package mcp

import (
	"context"
	"errors"
	"reflect"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
)

func (capability *managerHostEvidenceCapabilityV2) UseExactRegistryCommit(
	securityContext domainsecurity.TurnSecurityContext,
	probe domainsecurity.VerifiedSourceProbe,
	selection datasetsnapshotport.CurrentSelectionV2,
	prepared domainevidence.PreparedEvidenceSettlement,
	marker domainevidence.HostEvidenceSettlementMarker,
	use func(context.Context) error,
) error {
	if capability == nil {
		return errors.New("host registry effect capability is inactive")
	}
	capability.mu.RLock()
	defer capability.mu.RUnlock()
	effect, ok := capability.datasetCapability.(datasetsnapshotport.RegistryCommitCapabilityV1)
	if !ok || !capability.active || capability.ctx == nil || capability.ctx.Err() != nil || use == nil ||
		securityContext != capability.admission.Context ||
		!reflect.DeepEqual(probe, capability.admission.Probe) ||
		!reflect.DeepEqual(selection, capability.admission.Selection) ||
		!reflect.DeepEqual(prepared.SourceProbe, probe) || capability.manager == nil {
		return errors.New("host registry effect does not authorize this source binding")
	}
	return effect.UseExactRegistryCommit(cloneCurrentDatasetSelectionV2(selection), securityContext, prepared, marker,
		func(lease context.Context) error {
			if lease == nil || lease.Err() != nil || capability.ctx.Err() != nil ||
				!capability.manager.sourceEvidenceAdmissionIsCurrent(capability.serverID, capability.admission) {
				return errors.New("host registry effect source admission is no longer current")
			}
			if err := use(lease); err != nil {
				return err
			}
			if lease.Err() != nil || capability.ctx.Err() != nil ||
				!capability.manager.sourceEvidenceAdmissionIsCurrent(capability.serverID, capability.admission) {
				return errors.New("host registry effect source admission changed during use")
			}
			return nil
		})
}
