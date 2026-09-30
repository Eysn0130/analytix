package datasetsnapshot

import (
	"context"
	"errors"
	"reflect"
	"sync"

	domainhost "analytix.local/runtime-go/internal/domain/hostcurrentness"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
)

// WithCurrentSelectionV2 issues a callback-scoped host-local capability. A
// selected local head is never represented as a witnessed FreshHead.
func (service *HostLocalSealedServiceV2) WithCurrentSelectionV2(
	ctx context.Context,
	input datasetsnapshotport.ResolveInputV2,
	securityContext domainsecurity.TurnSecurityContext,
	callback func(datasetsnapshotport.CurrentSelectionV2, datasetsnapshotport.CurrentSelectionCapabilityV2) error,
) error {
	if service == nil || ctx == nil || callback == nil {
		return datasetsnapshotport.ErrUnavailable
	}
	if err := validateCurrentResolveInputV2(input, securityContext); err != nil {
		return errors.Join(datasetsnapshotport.ErrMismatch, err)
	}
	binding, err := resolveBinding(datasetsnapshotport.ResolveInput{
		TenantID: input.TenantID, UserID: input.UserID, Observation: input.Observation,
	})
	if err != nil {
		return errors.Join(datasetsnapshotport.ErrMismatch, err)
	}
	head, err := service.currentHostHead(ctx)
	if err != nil {
		return err
	}
	state, err := service.loadVersionedChainFromRootV2(ctx, head.DatasetSnapshotIndexDigest,
		head.DatasetSnapshotCount, true, service.modeCommitmentDigest, &binding, input)
	if err != nil {
		return err
	}
	if state.selected == nil || state.selected.record.V2 == nil || state.headIndex == nil ||
		state.selected.record.V2.DatasetSnapshotID != input.ExpectedDatasetSnapshotID {
		return datasetsnapshotport.ErrStale
	}
	resolved, err := service.resolveHostNodeMaterialV2(ctx, *state.selected, input)
	if err != nil {
		return err
	}
	if err := service.confirmHostDatasetHead(ctx, head); err != nil {
		return err
	}
	selection := datasetsnapshotport.CurrentSelectionV2{
		HostLocalHead: &head, DatasetIndexPath: append([]domainsecurity.DatasetSnapshotIndexV1(nil), state.indexPath...),
		SelectedIndex: state.selected.index, Snapshot: resolved,
	}
	selection.SelectionDigest, err = currentSelectionDigestV2(selection)
	if err != nil || service.validateHostCurrentSelectionV2(selection, input, securityContext) != nil {
		return errors.Join(datasetsnapshotport.ErrCorrupt, err)
	}
	capability := &hostLocalCurrentSelectionCapabilityV2{
		active: true, ctx: ctx, service: service, input: input,
		securityContext: securityContext, selection: cloneCurrentSelectionV2(selection),
	}
	capability.condition = sync.NewCond(&capability.mu)
	defer capability.close()
	if err := callback(cloneCurrentSelectionV2(selection), capability); err != nil {
		return err
	}
	return ctx.Err()
}

func (service *HostLocalSealedServiceV2) validateHostCurrentSelectionV2(
	selection datasetsnapshotport.CurrentSelectionV2,
	input datasetsnapshotport.ResolveInputV2,
	securityContext domainsecurity.TurnSecurityContext,
) error {
	if service == nil || validateCurrentResolveInputV2(input, securityContext) != nil ||
		!reflect.DeepEqual(selection.Head, evidenceauthorityport.FreshHead{}) ||
		selection.HostLocalHead == nil ||
		domainhost.ValidateHeadV1(*selection.HostLocalHead) != nil ||
		selection.HostLocalHead.InstallationID != service.installationID ||
		selection.HostLocalHead.RootBindingDigest != service.rootBindingDigest ||
		selection.HostLocalHead.AuthorityKeyID != service.authorityKeyID ||
		len(selection.DatasetIndexPath) == 0 || len(selection.DatasetIndexPath) > maxDatasetSnapshotIndexDepth ||
		uint64(len(selection.DatasetIndexPath)) != selection.HostLocalHead.DatasetSnapshotCount ||
		selection.HostLocalHead.DatasetSnapshotIndexDigest != selection.DatasetIndexPath[0].IndexDigest ||
		selection.Snapshot.Record.DatasetSnapshotID != securityContext.DatasetSnapshotID ||
		selection.Snapshot.Record.SourceManifestHash != securityContext.SourceManifestHash ||
		selection.Snapshot.Manifest.SourceManifestHash != securityContext.SourceManifestHash ||
		selection.Snapshot.Record.Binding != selection.Snapshot.Manifest.Binding ||
		selection.SelectedIndex.Binding != selection.Snapshot.Record.Binding ||
		selection.SelectedIndex.SnapshotRecordDigest != selection.Snapshot.Record.RecordDigest ||
		domainsecurity.ValidateDatasetSnapshotAuthorityRecordForFundsProducerContentV2(
			selection.Snapshot.Record, selection.Snapshot.Manifest, selection.Snapshot.FundsProducerContent,
		) != nil ||
		domainsecurity.ValidateDatasetSnapshotIndexNodeForHostLocalManifestV2(
			selection.SelectedIndex, selection.Snapshot.Record, selection.Snapshot.Manifest,
			selection.Snapshot.FundsProducerContent, input.TenantID, input.UserID, input.Observation,
			service.installationID, service.modeCommitmentDigest, service.authorityKeyID, service.authorityKey,
		) != nil {
		return datasetsnapshotport.ErrMismatch
	}
	seen := make(map[string]bool, len(selection.DatasetIndexPath))
	selectedCount := 0
	for offset, index := range selection.DatasetIndexPath {
		if seen[index.IndexDigest] || index.Generation != selection.HostLocalHead.DatasetSnapshotCount-uint64(offset) ||
			domainsecurity.ValidateDatasetSnapshotIndexForHostLocalV2(index, service.installationID,
				service.modeCommitmentDigest, service.authorityKeyID, service.authorityKey) != nil {
			return datasetsnapshotport.ErrCorrupt
		}
		seen[index.IndexDigest] = true
		if index == selection.SelectedIndex {
			selectedCount++
		}
		if offset == 0 {
			if domainsecurity.ValidateDatasetSnapshotIndexHostLocalRootV2(index,
				selection.HostLocalHead.DatasetSnapshotIndexDigest, selection.HostLocalHead.DatasetSnapshotCount) != nil {
				return datasetsnapshotport.ErrCorrupt
			}
		} else if domainsecurity.ValidateDatasetSnapshotIndexHostLocalTransitionV2(index,
			selection.DatasetIndexPath[offset-1]) != nil {
			return datasetsnapshotport.ErrCorrupt
		}
	}
	if selectedCount != 1 || selection.DatasetIndexPath[len(selection.DatasetIndexPath)-1].PreviousIndexDigest !=
		domainsecurity.DatasetSnapshotIndexGenesisDigestV1() {
		return datasetsnapshotport.ErrCorrupt
	}
	digest, err := currentSelectionDigestV2(selection)
	if err != nil || selection.SelectionDigest != digest {
		return errors.Join(datasetsnapshotport.ErrCorrupt, err)
	}
	return nil
}

type hostLocalCurrentSelectionCapabilityV2 struct {
	mu                 sync.Mutex
	condition          *sync.Cond
	active             bool
	activeUses         uint64
	registryEffectUsed bool
	ctx                context.Context
	service            *HostLocalSealedServiceV2
	input              datasetsnapshotport.ResolveInputV2
	securityContext    domainsecurity.TurnSecurityContext
	selection          datasetsnapshotport.CurrentSelectionV2
}

func (capability *hostLocalCurrentSelectionCapabilityV2) UseExact(
	selection datasetsnapshotport.CurrentSelectionV2,
	securityContext domainsecurity.TurnSecurityContext,
	use func(context.Context) error,
) error {
	return capability.useExact(selection, securityContext, use, false)
}

func (capability *hostLocalCurrentSelectionCapabilityV2) UsePostNativeExact(
	selection datasetsnapshotport.CurrentSelectionV2,
	securityContext domainsecurity.TurnSecurityContext,
	use func(context.Context) error,
) error {
	return capability.useExact(selection, securityContext, use, true)
}

func (capability *hostLocalCurrentSelectionCapabilityV2) useExact(
	selection datasetsnapshotport.CurrentSelectionV2,
	securityContext domainsecurity.TurnSecurityContext,
	use func(context.Context) error,
	postNative bool,
) error {
	if capability == nil || use == nil || !capability.beginUse(postNative) {
		return datasetsnapshotport.ErrUnavailable
	}
	defer capability.endUse()
	if securityContext != capability.securityContext || selection.SelectionDigest != capability.selection.SelectionDigest ||
		capability.service.validateHostCurrentSelectionV2(selection, capability.input, securityContext) != nil ||
		!reflect.DeepEqual(selection, capability.selection) {
		return datasetsnapshotport.ErrMismatch
	}
	lease, cancel := context.WithCancel(capability.ctx)
	defer cancel()
	if err := capability.service.confirmHostDatasetHead(lease, *capability.selection.HostLocalHead); err != nil {
		return err
	}
	useErr := use(lease)
	currentErr := capability.service.confirmHostDatasetHead(lease, *capability.selection.HostLocalHead)
	return errors.Join(useErr, currentErr, lease.Err())
}

func (capability *hostLocalCurrentSelectionCapabilityV2) beginUse(postNative bool) bool {
	capability.mu.Lock()
	defer capability.mu.Unlock()
	if !capability.active || capability.ctx.Err() != nil || postNative && capability.activeUses != 1 {
		return false
	}
	capability.activeUses++
	return true
}

func (capability *hostLocalCurrentSelectionCapabilityV2) endUse() {
	capability.mu.Lock()
	capability.activeUses--
	if capability.activeUses == 0 {
		capability.condition.Broadcast()
	}
	capability.mu.Unlock()
}

func (capability *hostLocalCurrentSelectionCapabilityV2) close() {
	capability.mu.Lock()
	capability.active = false
	for capability.activeUses > 0 {
		capability.condition.Wait()
	}
	capability.mu.Unlock()
}

func (*hostLocalCurrentSelectionCapabilityV2) MarshalJSON() ([]byte, error) {
	return nil, errors.New("host-local dataset snapshot capability is not serializable")
}

// WithRetainedSelectionV2 finds one exact historical node only on the
// currently selected host-local index path. The current head remains live
// across the complete retained material read and callback.
func (service *HostLocalSealedServiceV2) WithRetainedSelectionV2(
	ctx context.Context,
	input datasetsnapshotport.RetainedSelectionInputV2,
	callback func(context.Context, datasetsnapshotport.RetainedSelectionV2) error,
) error {
	if service == nil || ctx == nil || callback == nil || validateRetainedSelectionInputV2(input) != nil {
		return datasetsnapshotport.ErrMismatch
	}
	return service.WithCurrentSelectionV2(ctx, input.CurrentResolveInput, input.CurrentSecurityContext,
		func(current datasetsnapshotport.CurrentSelectionV2,
			capability datasetsnapshotport.CurrentSelectionCapabilityV2) error {
			return capability.UseExact(current, input.CurrentSecurityContext, func(lease context.Context) error {
				var selected *datasetsnapshotport.RetainedSelectionV2
				for _, index := range current.DatasetIndexPath {
					node, err := service.resolveVersionedNodeV2(lease, index, true, service.modeCommitmentDigest)
					if err != nil {
						return err
					}
					if node.record.V2 == nil || node.record.V2.DatasetSnapshotID != input.RetainedDatasetSnapshotID {
						continue
					}
					if node.bundle == nil || node.record.V2.SourceManifestHash != input.RetainedSourceManifestHash ||
						node.bundle.Manifest.SourceManifestHash != input.RetainedSourceManifestHash {
						return datasetsnapshotport.ErrMismatch
					}
					resolved, err := service.resolveHostNodeMaterialV2(lease, node, input.CurrentResolveInput)
					if err != nil {
						return err
					}
					retained := datasetsnapshotport.RetainedSelectionV2{
						Current: cloneCurrentSelectionV2(current), SelectedIndex: index, Snapshot: resolved,
					}
					selected = &retained
					break
				}
				if selected == nil {
					return datasetsnapshotport.ErrNotFound
				}
				if err := callback(lease, cloneRetainedSelectionV2(*selected)); err != nil {
					return err
				}
				node, err := service.resolveVersionedNodeV2(lease, selected.SelectedIndex, true,
					service.modeCommitmentDigest)
				if err != nil {
					return err
				}
				readback, err := service.resolveHostNodeMaterialV2(lease, node, input.CurrentResolveInput)
				if err != nil || !reflect.DeepEqual(readback, selected.Snapshot) {
					return errors.Join(datasetsnapshotport.ErrCorrupt, err)
				}
				return nil
			})
		})
}

// Historical witnessed observations cannot select a host-local profile.
func (*HostLocalSealedServiceV2) WithHistoricalFactSelectionV2(context.Context,
	datasetsnapshotport.ResolveInputV2, domainsecurity.TurnSecurityContext,
	evidenceauthorityport.FreshHead, func(datasetsnapshotport.CurrentSelectionV2) error) error {
	return datasetsnapshotport.ErrUnavailable
}
