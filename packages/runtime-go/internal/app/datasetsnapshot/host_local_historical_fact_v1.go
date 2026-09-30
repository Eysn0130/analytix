package datasetsnapshot

import (
	"context"
	"errors"
	"reflect"

	domainhost "analytix.local/runtime-go/internal/domain/hostcurrentness"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
)

func (service *HostLocalSealedServiceV2) WithHostLocalHistoricalFactSelectionV1(ctx context.Context, input datasetsnapshotport.ResolveInputV2,
	securityContext domainsecurity.TurnSecurityContext, historical domainhost.HeadV1, use func(datasetsnapshotport.CurrentSelectionV2) error) error {
	if service == nil || ctx == nil || use == nil || validateCurrentResolveInputV2(input, securityContext) != nil ||
		domainhost.ValidateHeadForInstallationV1(historical, service.installationID, service.rootBindingDigest, service.authorityKeyID, service.authorityKey) != nil {
		return datasetsnapshotport.ErrMismatch
	}
	binding, err := resolveBinding(datasetsnapshotport.ResolveInput{TenantID: input.TenantID, UserID: input.UserID, Observation: input.Observation})
	if err != nil {
		return err
	}
	current, err := service.currentHostHead(ctx)
	if err != nil {
		return err
	}
	resolve := func() (datasetsnapshotport.CurrentSelectionV2, error) {
		retained, err := service.loadVersionedChainFromRootV2(ctx, current.DatasetSnapshotIndexDigest, current.DatasetSnapshotCount, true, service.modeCommitmentDigest, &binding, input)
		if err != nil {
			return datasetsnapshotport.CurrentSelectionV2{}, err
		}
		found := false
		for _, index := range retained.indexPath {
			if index.IndexDigest == historical.DatasetSnapshotIndexDigest && index.Generation == historical.DatasetSnapshotCount {
				found = true
				break
			}
		}
		if !found {
			return datasetsnapshotport.CurrentSelectionV2{}, datasetsnapshotport.ErrStale
		}
		original, err := service.loadVersionedChainFromRootV2(ctx, historical.DatasetSnapshotIndexDigest, historical.DatasetSnapshotCount, true, service.modeCommitmentDigest, &binding, input)
		if err != nil {
			return datasetsnapshotport.CurrentSelectionV2{}, err
		}
		if original.selected == nil || original.selected.record.V2 == nil || original.selected.record.V2.DatasetSnapshotID != input.ExpectedDatasetSnapshotID {
			return datasetsnapshotport.CurrentSelectionV2{}, datasetsnapshotport.ErrMismatch
		}
		snapshot, err := service.resolveHostNodeMaterialV2(ctx, *original.selected, input)
		if err != nil {
			return datasetsnapshotport.CurrentSelectionV2{}, err
		}
		selection := datasetsnapshotport.CurrentSelectionV2{HostLocalHead: &historical, DatasetIndexPath: append([]domainsecurity.DatasetSnapshotIndexV1(nil), original.indexPath...), SelectedIndex: original.selected.index, Snapshot: snapshot}
		selection.SelectionDigest, err = currentSelectionDigestV2(selection)
		if err == nil {
			err = service.validateHostCurrentSelectionV2(selection, input, securityContext)
		}
		return selection, err
	}
	selection, err := resolve()
	if err != nil {
		return err
	}
	if err := service.confirmHostDatasetHead(ctx, current); err != nil {
		return err
	}
	useErr := use(cloneCurrentSelectionV2(selection))
	after, afterErr := resolve()
	if afterErr == nil && !reflect.DeepEqual(after, selection) {
		afterErr = datasetsnapshotport.ErrCorrupt
	}
	return errors.Join(useErr, afterErr, service.confirmHostDatasetHead(ctx, current), ctx.Err())
}
