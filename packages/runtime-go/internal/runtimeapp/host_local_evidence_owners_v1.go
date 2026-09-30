package runtimeapp

import (
	"context"
	"errors"
	"path/filepath"
	"sync"

	datasetsnapshotstore "analytix.local/runtime-go/internal/adapters/outbound/datasetsnapshot"
	evidenceauthorityhostlocal "analytix.local/runtime-go/internal/adapters/outbound/evidenceauthorityhostlocal"
	evidenceregistrystore "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	datasetsnapshotapp "analytix.local/runtime-go/internal/app/datasetsnapshot"
	evidenceregistryapp "analytix.local/runtime-go/internal/app/evidenceregistry"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceregistryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

// This composition is inert on ordinary startup. Only EnsureForFundsImport
// may install generation zero; reads may open an already committed profile.
type runtimeHostLocalEvidenceOwnersV1 struct {
	mu            sync.Mutex
	idle          *sync.Cond
	active        uint64
	config        Config
	rootAuthority *persistencefs.RootAuthority
	access        finalauthority.SecurePrivateCASRecoveryAccessAuthority
	authority     finalauthorityport.Authority
	stores        *datasetsnapshotstore.StoresV2
	decision      runtimeHostLocalProfileDecisionV1
	checkFresh    func(context.Context) error
	heads         *evidenceauthorityhostlocal.HostLocalStore
	snapshot      *datasetsnapshotapp.HostLocalSealedServiceV2
	registry      *evidenceregistryapp.HostLocalServiceV3
	closeRegistry func() error
	closed        bool
}

var _ runtimeEvidenceRegistryAuthority = (*runtimeHostLocalEvidenceOwnersV1)(nil)
var _ datasetsnapshotport.CurrentAuthorityV2 = (*runtimeHostLocalEvidenceOwnersV1)(nil)
var _ datasetsnapshotport.AuthorityV2 = (*runtimeHostLocalEvidenceOwnersV1)(nil)

func newRuntimeHostLocalEvidenceOwnersV1(config Config,
	rootAuthority *persistencefs.RootAuthority,
	access finalauthority.SecurePrivateCASRecoveryAccessAuthority,
	authority finalauthorityport.Authority,
	stores *datasetsnapshotstore.StoresV2,
	decision runtimeHostLocalProfileDecisionV1,
	checkFresh func(context.Context) error) (*runtimeHostLocalEvidenceOwnersV1, error) {
	if rootAuthority == nil || access == nil || authority == nil || stores == nil ||
		decision == runtimeHostLocalProfileUnavailableV1 || checkFresh == nil {
		return nil, errors.New("runtime host-local evidence composition is unavailable")
	}
	owner := &runtimeHostLocalEvidenceOwnersV1{
		config: config, rootAuthority: rootAuthority, access: access,
		authority: authority, stores: stores, decision: decision, checkFresh: checkFresh,
	}
	owner.idle = sync.NewCond(&owner.mu)
	return owner, nil
}

func (owner *runtimeHostLocalEvidenceOwnersV1) EnsureForFundsImport(ctx context.Context) error {
	if owner == nil || ctx == nil || ctx.Err() != nil {
		return errRuntimeCaseEvidenceAuthorityUnavailableV1
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	return owner.ensureLocked(ctx, true)
}

func (owner *runtimeHostLocalEvidenceOwnersV1) ensureRead(ctx context.Context) error {
	if owner == nil || ctx == nil || ctx.Err() != nil {
		return errRuntimeCaseEvidenceAuthorityUnavailableV1
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	return owner.ensureLocked(ctx, false)
}

func (owner *runtimeHostLocalEvidenceOwnersV1) ensureLocked(ctx context.Context, fundsImport bool) error {
	if owner.closed {
		return errRuntimeCaseEvidenceAuthorityUnavailableV1
	}
	if owner.snapshot != nil && owner.registry != nil {
		return nil
	}
	if owner.decision == runtimeHostLocalProfileAdmitV1 {
		if !fundsImport || owner.checkFresh(ctx) != nil {
			return errRuntimeCaseEvidenceAuthorityUnavailableV1
		}
	}
	selector, history, err := runtimeHostLocalProfileRootsPresentV1(owner.config.DataDir)
	if err != nil || owner.decision == runtimeHostLocalProfileAdmitV1 && (selector || history) ||
		owner.decision == runtimeHostLocalProfileResumeV1 && (!selector || !history) {
		return errors.Join(errRuntimeCaseEvidenceAuthorityUnavailableV1, err)
	}
	if owner.heads == nil {
		owner.heads, err = openRuntimeHostLocalProfileV1(ctx, owner.config, owner.rootAuthority,
			owner.access, owner.authority, owner.decision)
		if err != nil {
			return err
		}
		owner.decision = runtimeHostLocalProfileResumeV1
	}
	if owner.snapshot == nil {
		owner.snapshot, err = datasetsnapshotapp.NewHostLocalSealedV2(ctx,
			datasetsnapshotapp.HostLocalSealedConfigV2{
				InstallationID: owner.authority.KeyID(), Authority: owner.authority, Heads: owner.heads,
				LegacyRecords: owner.stores.LegacyRecords, Bundles: owner.stores.AuthorityBundles,
				Indexes: owner.stores.Indexes, Materials: owner.stores.Materials,
			})
		if err != nil {
			return err
		}
	}
	if owner.registry == nil {
		registryRoot := filepath.Join(owner.config.DataDir, "private", "evidence-registry")
		prepared, prepareErr := evidenceregistrystore.PrepareRecoveryV2(ctx, registryRoot, owner.access)
		if prepareErr != nil {
			return prepareErr
		}
		semanticErr := prepared.ValidateSemantics(ctx)
		allowed := semanticErr == nil &&
			(prepared.HostLocalV3ActivationAllowed() || prepared.FreshImportInventoryV2())
		if !allowed && semanticErr != nil {
			head, found, headErr := owner.heads.Current(ctx)
			partialEmpty, partialErr := prepared.HostLocalEmptyPartialInventoryAllowed(ctx)
			if headErr == nil && found && head.EvidenceRegistryCount == 0 &&
				head.EvidenceRegistryIndexDigest == domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2() &&
				partialErr == nil && partialEmpty {
				allowed = true
			}
		}
		if !allowed {
			return errors.Join(errRuntimeCaseEvidenceAuthorityUnavailableV1, semanticErr)
		}
		if err := prepared.RevalidatePhysicalV2(ctx); err != nil {
			return err
		}
		indexes, err := evidenceregistrystore.NewAuthorityIndexStoreV2(filepath.Join(registryRoot, "indexes"), owner.access)
		if err != nil {
			return err
		}
		capsules, err := evidenceregistrystore.NewAuthorityCapsuleStoreV2(filepath.Join(registryRoot, "capsules"), owner.access)
		if err != nil {
			return errors.Join(err, indexes.Close())
		}
		registry, err := evidenceregistryapp.NewHostLocalV3(ctx, evidenceregistryapp.HostLocalConfigV3{
			InstallationID: owner.authority.KeyID(), Authority: owner.authority,
			Heads: owner.heads, Indexes: indexes, Capsules: capsules, DatasetAuthority: owner.snapshot,
		})
		if err != nil {
			return errors.Join(err, capsules.Close(), indexes.Close())
		}
		owner.registry = registry
		owner.closeRegistry = func() error { return errors.Join(capsules.Close(), indexes.Close()) }
	}
	return nil
}

func (owner *runtimeHostLocalEvidenceOwnersV1) Close() error {
	if owner == nil {
		return nil
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	owner.closed = true
	for owner.active != 0 {
		owner.idle.Wait()
	}
	var err error
	if owner.closeRegistry != nil {
		err = errors.Join(err, owner.closeRegistry())
		owner.closeRegistry = nil
	}
	if owner.heads != nil {
		err = errors.Join(err, owner.heads.Close())
		owner.heads = nil
	}
	owner.registry, owner.snapshot = nil, nil
	return err
}

func (owner *runtimeHostLocalEvidenceOwnersV1) CaseEvidenceAuthorityUnavailableV1() bool {
	owner.mu.Lock()
	defer owner.mu.Unlock()
	return owner.closed || owner.registry == nil
}

// A borrow pins the immutable service pointers and their CAS handles across
// callbacks without holding owner.mu. Nested snapshot-to-registry operations
// therefore acquire another borrow; Close waits for all of them to finish.
func (owner *runtimeHostLocalEvidenceOwnersV1) borrow(ctx context.Context) (
	*datasetsnapshotapp.HostLocalSealedServiceV2,
	*evidenceregistryapp.HostLocalServiceV3,
	func(), error) {
	if err := owner.ensureRead(ctx); err != nil {
		return nil, nil, nil, err
	}
	owner.mu.Lock()
	if owner.closed || owner.snapshot == nil || owner.registry == nil {
		owner.mu.Unlock()
		return nil, nil, nil, errRuntimeCaseEvidenceAuthorityUnavailableV1
	}
	owner.active++
	snapshot, registry := owner.snapshot, owner.registry
	owner.mu.Unlock()
	return snapshot, registry, func() {
		owner.mu.Lock()
		owner.active--
		if owner.active == 0 {
			owner.idle.Broadcast()
		}
		owner.mu.Unlock()
	}, nil
}

func (owner *runtimeHostLocalEvidenceOwnersV1) AdmitExactV2(ctx context.Context, input datasetsnapshotapp.AdmitInputV2) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	snapshot, _, release, err := owner.borrow(ctx)
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	defer release()
	return snapshot.AdmitExactV2(ctx, input)
}

func (owner *runtimeHostLocalEvidenceOwnersV1) AdmitAfterExactV2(ctx context.Context, input datasetsnapshotapp.AdmitAfterInputV2) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	snapshot, _, release, err := owner.borrow(ctx)
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	defer release()
	return snapshot.AdmitAfterExactV2(ctx, input)
}

func (owner *runtimeHostLocalEvidenceOwnersV1) ResolveWitnessedV2(ctx context.Context, input datasetsnapshotport.ResolveInputV2) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	snapshot, _, release, err := owner.borrow(ctx)
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	defer release()
	return snapshot.ResolveWitnessedV2(ctx, input)
}

func (owner *runtimeHostLocalEvidenceOwnersV1) WithCurrentSelectionV2(ctx context.Context, input datasetsnapshotport.ResolveInputV2,
	securityContext domainsecurity.TurnSecurityContext,
	use func(datasetsnapshotport.CurrentSelectionV2, datasetsnapshotport.CurrentSelectionCapabilityV2) error) error {
	snapshot, _, release, err := owner.borrow(ctx)
	if err != nil {
		return err
	}
	defer release()
	return snapshot.WithCurrentSelectionV2(ctx, input, securityContext, use)
}

func (owner *runtimeHostLocalEvidenceOwnersV1) WithRetainedSelectionV2(ctx context.Context,
	input datasetsnapshotport.RetainedSelectionInputV2,
	use func(context.Context, datasetsnapshotport.RetainedSelectionV2) error) error {
	snapshot, _, release, err := owner.borrow(ctx)
	if err != nil {
		return err
	}
	defer release()
	return snapshot.WithRetainedSelectionV2(ctx, input, use)
}

func (owner *runtimeHostLocalEvidenceOwnersV1) CommitPrepared(ctx context.Context, input evidenceregistryport.CommitPreparedInput) (domainevidence.EvidenceReceipt, error) {
	_, registry, release, err := owner.borrow(ctx)
	if err != nil {
		return domainevidence.EvidenceReceipt{}, err
	}
	defer release()
	return registry.CommitPrepared(ctx, input)
}

func (owner *runtimeHostLocalEvidenceOwnersV1) Resolve(ctx context.Context, query evidenceregistryport.MembershipQuery) (domainevidence.RegisteredEvidence, error) {
	_, registry, release, err := owner.borrow(ctx)
	if err != nil {
		return domainevidence.RegisteredEvidence{}, err
	}
	defer release()
	return registry.Resolve(ctx, query)
}

func (owner *runtimeHostLocalEvidenceOwnersV1) Revoke(ctx context.Context, input evidenceregistryport.RevokeInput) error {
	_, registry, release, err := owner.borrow(ctx)
	if err != nil {
		return err
	}
	defer release()
	return registry.Revoke(ctx, input)
}

func (owner *runtimeHostLocalEvidenceOwnersV1) Replay(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) (domainevidence.EvidenceReceiptRegistry, error) {
	_, registry, release, err := owner.borrow(ctx)
	if err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, err
	}
	defer release()
	return registry.Replay(ctx, securityContext)
}

func (owner *runtimeHostLocalEvidenceOwnersV1) ReplayAt(ctx context.Context, securityContext domainsecurity.TurnSecurityContext, sequence uint64) (domainevidence.EvidenceReceiptRegistry, error) {
	_, registry, release, err := owner.borrow(ctx)
	if err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, err
	}
	defer release()
	return registry.ReplayAt(ctx, securityContext, sequence)
}

func (owner *runtimeHostLocalEvidenceOwnersV1) WithLockedSnapshot(ctx context.Context, securityContext domainsecurity.TurnSecurityContext,
	use func(domainevidence.EvidenceReceiptRegistry) error) error {
	_, registry, release, err := owner.borrow(ctx)
	if err != nil {
		return err
	}
	defer release()
	return registry.WithLockedSnapshot(ctx, securityContext, use)
}

func (owner *runtimeHostLocalEvidenceOwnersV1) HasRecords(ctx context.Context) (bool, error) {
	_, registry, release, err := owner.borrow(ctx)
	if err != nil {
		return false, err
	}
	defer release()
	return registry.HasRecords(ctx)
}

func (owner *runtimeHostLocalEvidenceOwnersV1) ListRegistries(ctx context.Context, contexts []domainsecurity.TurnSecurityContext) ([]evidenceregistryport.InventoryRecord, error) {
	_, registry, release, err := owner.borrow(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return registry.ListRegistries(ctx, contexts)
}
