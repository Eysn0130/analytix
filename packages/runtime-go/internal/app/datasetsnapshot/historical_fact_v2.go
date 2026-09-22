package datasetsnapshot

import (
	"context"
	"errors"
	"reflect"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
)

// WithHistoricalFactSelectionV2 verifies original publication material against
// both its witnessed head and the current retained chain. It issues no current
// selection capability, so it cannot authorize a new query or registry commit.
// The caller separately revalidates current principal, case and risk permission.
func (service *SealedServiceV2) WithHistoricalFactSelectionV2(ctx context.Context, input datasetsnapshotport.ResolveInputV2, securityContext domainsecurity.TurnSecurityContext, historical evidenceauthorityport.FreshHead, use func(datasetsnapshotport.CurrentSelectionV2) error) error {
	if service == nil || ctx == nil || use == nil || validateCurrentResolveInputV2(input, securityContext) != nil || service.validateHead(historical) != nil {
		return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("historical fact dataset input is invalid"))
	}
	binding, err := resolveBinding(datasetsnapshotport.ResolveInput{TenantID: input.TenantID, UserID: input.UserID, Observation: input.Observation})
	if err != nil {
		return err
	}
	current, err := service.observeFresh(ctx)
	if err != nil {
		return err
	}
	resolve := func() (datasetsnapshotport.CurrentSelectionV2, error) {
		state, err := service.loadVersionedChainV2(ctx, current, &binding, input)
		if err != nil {
			return datasetsnapshotport.CurrentSelectionV2{}, err
		}
		found := false
		for _, index := range state.indexPath {
			if index.IndexDigest == historical.Bundle.DatasetSnapshotIndexDigest && index.Generation == historical.Bundle.DatasetSnapshotCount {
				found = true
				break
			}
		}
		if !found {
			return datasetsnapshotport.CurrentSelectionV2{}, datasetsnapshotport.ErrStale
		}
		original, err := service.loadVersionedChainV2(ctx, historical, &binding, input)
		if err != nil {
			return datasetsnapshotport.CurrentSelectionV2{}, err
		}
		if original.selected == nil || original.selected.record.V2 == nil || original.selected.record.V2.DatasetSnapshotID != input.ExpectedDatasetSnapshotID {
			return datasetsnapshotport.CurrentSelectionV2{}, datasetsnapshotport.ErrMismatch
		}
		snapshot, err := service.resolveNodeMaterialV2(ctx, *original.selected, input)
		if err != nil {
			return datasetsnapshotport.CurrentSelectionV2{}, err
		}
		selection := datasetsnapshotport.CurrentSelectionV2{Head: historical, DatasetIndexPath: append([]domainsecurity.DatasetSnapshotIndexV1(nil), original.indexPath...), SelectedIndex: original.selected.index, Snapshot: snapshot}
		selection.SelectionDigest, err = currentSelectionDigestV2(selection)
		if err == nil {
			err = service.validateCurrentSelectionV2(selection, input, securityContext)
		}
		return selection, err
	}
	selection, err := resolve()
	if err != nil {
		return err
	}
	if _, err = service.confirmSharedEvidenceHeadV2(ctx, current); err != nil {
		return err
	}
	useErr := use(cloneCurrentSelectionV2(selection))
	after, afterErr := resolve()
	if afterErr == nil && !reflect.DeepEqual(selection, after) {
		afterErr = datasetsnapshotport.ErrCorrupt
	}
	_, currentErr := service.confirmSharedEvidenceHeadV2(ctx, current)
	return errors.Join(useErr, afterErr, currentErr, ctx.Err())
}
