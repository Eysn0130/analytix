package runtimeapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	registrystore "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	datasetsnapshotapp "analytix.local/runtime-go/internal/app/datasetsnapshot"
	evidenceauthorityapp "analytix.local/runtime-go/internal/app/evidenceauthority"
	evidenceregistryapp "analytix.local/runtime-go/internal/app/evidenceregistry"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

func newRuntimeFreshImportRegistryV1(
	root string,
	access finalauthority.SecurePrivateCASRecoveryAccessAuthority,
	authority *evidenceauthorityapp.Authority,
	snapshots *datasetsnapshotapp.SealedServiceV2,
	initial *registrystore.PreparedRecoveryV2,
	open func() (*evidenceregistryapp.Service, func() error, error),
) *runtimeImportActivatedRegistryV1 {
	return &runtimeImportActivatedRegistryV1{activate: func(ctx context.Context, binding domainsecurity.CaseBindingObservationV1, snapshotID string) (_ *evidenceregistryapp.Service, _ func() error, resultErr error) {
		if initial == nil || initial.RevalidatePhysicalV2(ctx) != nil {
			return nil, nil, errRuntimeCaseEvidenceAuthorityUnavailableV1
		}
		if domainsecurity.ValidateCaseBindingObservationV1(binding) != nil || !domainsecurity.IsDatasetSnapshotIDV2Syntax(snapshotID) {
			return nil, nil, errRuntimeCaseEvidenceAuthorityUnavailableV1
		}
		resolve := datasetsnapshotport.ResolveInputV2{TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, Observation: binding, ExpectedDatasetSnapshotID: snapshotID}
		selected, err := snapshots.ResolveWitnessedV2(ctx, resolve)
		if err != nil || selected.Record.DatasetSnapshotID != snapshotID {
			return nil, nil, errRuntimeCaseEvidenceAuthorityUnavailableV1
		}
		prepared, err := registrystore.PrepareRecoveryV2(ctx, root, access)
		if err != nil {
			return nil, nil, err
		}
		if err := prepared.ValidateSemantics(ctx); err != nil || !prepared.FreshImportInventoryV2() {
			return nil, nil, errors.Join(errRuntimeCaseEvidenceAuthorityUnavailableV1, err)
		}
		// Pin the existing container (or its parent while absent) across the
		// intended empty CAS creation. CAS opens themselves use the frozen
		// access authority and no-link, handle-relative traversal.
		pinnedPath := root
		if !prepared.HasStateV2() {
			pinnedPath = filepath.Dir(root)
		}
		before, err := os.Lstat(pinnedPath)
		if err != nil || !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
			return nil, nil, errRuntimeCaseEvidenceAuthorityUnavailableV1
		}
		pin, err := os.Open(pinnedPath)
		if err != nil {
			return nil, nil, err
		}
		defer pin.Close()
		held, err := pin.Stat()
		if err != nil || !os.SameFile(before, held) {
			return nil, nil, errRuntimeCaseEvidenceAuthorityUnavailableV1
		}
		var registry *evidenceregistryapp.Service
		var closeStores func() error
		defer func() {
			if resultErr != nil && closeStores != nil {
				resultErr = errors.Join(resultErr, closeStores())
			}
		}()
		err = authority.WithFreshHeadChallenge(ctx, func(head evidenceauthorityport.FreshHead, challenge func(context.Context) error) error {
			if !head.HasBundle || head.Bundle.EvidenceRegistryCount != 0 || head.Bundle.EvidenceRegistryIndexDigest != domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2() {
				return errRuntimeCaseEvidenceAuthorityUnavailableV1
			}
			if err := prepared.RevalidatePhysicalV2(ctx); err != nil {
				return err
			}
			if err := challenge(ctx); err != nil {
				return err
			}
			var openErr error
			registry, closeStores, openErr = open()
			if openErr != nil {
				return openErr
			}
			after, err := os.Lstat(pinnedPath)
			if err != nil || !after.IsDir() || after.Mode()&os.ModeSymlink != 0 || !os.SameFile(held, after) {
				return errRuntimeCaseEvidenceAuthorityUnavailableV1
			}
			complete, err := registrystore.PrepareRecoveryV2(ctx, root, access)
			if err != nil {
				return err
			}
			if err := complete.ValidateSemantics(ctx); err != nil || !complete.FreshImportInventoryV2() {
				return errors.Join(errRuntimeCaseEvidenceAuthorityUnavailableV1, err)
			}
			if err := complete.RevalidatePhysicalV2(ctx); err != nil {
				return err
			}
			current, err := snapshots.ResolveWitnessedV2(ctx, resolve)
			if err != nil || current.Record.RecordDigest != selected.Record.RecordDigest {
				return errRuntimeCaseEvidenceAuthorityUnavailableV1
			}
			return challenge(ctx)
		})
		if err != nil {
			return nil, nil, err
		}
		return registry, closeStores, nil
	}}
}

// One composition-owned indirection is shared by every registry consumer.
// Reads never activate it. Only the confirmed-import callback may publish a
// fully validated Service; Close waits for its active calls before closing CAS.
type runtimeImportActivatedRegistryV1 struct {
	mu          sync.RWMutex
	registry    *evidenceregistryapp.Service
	closeStores func() error
	activate    func(context.Context, domainsecurity.CaseBindingObservationV1, string) (*evidenceregistryapp.Service, func() error, error)
	closed      bool
}

var _ runtimeEvidenceRegistryAuthority = (*runtimeImportActivatedRegistryV1)(nil)
var _ registryport.WitnessedSnapshotAuthority = (*runtimeImportActivatedRegistryV1)(nil)
var _ registryport.FactFinalWitnessIssuer = (*runtimeImportActivatedRegistryV1)(nil)
var _ registryport.FactFinalWitnessVerifier = (*runtimeImportActivatedRegistryV1)(nil)

func (owner *runtimeImportActivatedRegistryV1) ActivateAfterImport(ctx context.Context, binding domainsecurity.CaseBindingObservationV1, snapshotID string) error {
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if owner.closed || ctx == nil || ctx.Err() != nil {
		return errRuntimeCaseEvidenceAuthorityUnavailableV1
	}
	if owner.registry != nil {
		return nil
	}
	if owner.activate == nil {
		return errRuntimeCaseEvidenceAuthorityUnavailableV1
	}
	registry, closeStores, err := owner.activate(ctx, binding, snapshotID)
	if err != nil {
		return err
	}
	if registry == nil || closeStores == nil {
		if closeStores != nil {
			_ = closeStores()
		}
		return errRuntimeCaseEvidenceAuthorityUnavailableV1
	}
	owner.registry, owner.closeStores = registry, closeStores
	return nil
}

func (owner *runtimeImportActivatedRegistryV1) CaseEvidenceAuthorityUnavailableV1() bool {
	owner.mu.RLock()
	defer owner.mu.RUnlock()
	return owner.closed || owner.registry == nil
}

func (owner *runtimeImportActivatedRegistryV1) Close() error {
	owner.mu.Lock()
	defer owner.mu.Unlock()
	owner.closed = true
	if owner.closeStores == nil {
		return nil
	}
	if err := owner.closeStores(); err != nil {
		return err
	}
	owner.closeStores = nil
	return nil
}

func (owner *runtimeImportActivatedRegistryV1) current() (*evidenceregistryapp.Service, func(), error) {
	owner.mu.RLock()
	if owner.closed || owner.registry == nil {
		owner.mu.RUnlock()
		return nil, nil, errRuntimeCaseEvidenceAuthorityUnavailableV1
	}
	return owner.registry, owner.mu.RUnlock, nil
}

func (owner *runtimeImportActivatedRegistryV1) CommitPrepared(ctx context.Context, input registryport.CommitPreparedInput) (domainevidence.EvidenceReceipt, error) {
	registry, release, err := owner.current()
	if err != nil {
		return domainevidence.EvidenceReceipt{}, err
	}
	defer release()
	return registry.CommitPrepared(ctx, input)
}

func (owner *runtimeImportActivatedRegistryV1) Resolve(ctx context.Context, query registryport.MembershipQuery) (domainevidence.RegisteredEvidence, error) {
	registry, release, err := owner.current()
	if err != nil {
		return domainevidence.RegisteredEvidence{}, err
	}
	defer release()
	return registry.Resolve(ctx, query)
}

func (owner *runtimeImportActivatedRegistryV1) Revoke(ctx context.Context, input registryport.RevokeInput) error {
	registry, release, err := owner.current()
	if err != nil {
		return err
	}
	defer release()
	return registry.Revoke(ctx, input)
}

func (owner *runtimeImportActivatedRegistryV1) Replay(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) (domainevidence.EvidenceReceiptRegistry, error) {
	registry, release, err := owner.current()
	if err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, err
	}
	defer release()
	return registry.Replay(ctx, securityContext)
}

func (owner *runtimeImportActivatedRegistryV1) ReplayAt(ctx context.Context, securityContext domainsecurity.TurnSecurityContext, sequence uint64) (domainevidence.EvidenceReceiptRegistry, error) {
	registry, release, err := owner.current()
	if err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, err
	}
	defer release()
	return registry.ReplayAt(ctx, securityContext, sequence)
}

func (owner *runtimeImportActivatedRegistryV1) WithLockedSnapshot(ctx context.Context, securityContext domainsecurity.TurnSecurityContext, use func(domainevidence.EvidenceReceiptRegistry) error) error {
	registry, release, err := owner.current()
	if err != nil {
		return err
	}
	defer release()
	return registry.WithLockedSnapshot(ctx, securityContext, use)
}

func (owner *runtimeImportActivatedRegistryV1) WithWitnessedSnapshot(ctx context.Context, securityContext domainsecurity.TurnSecurityContext, use func(registryport.WitnessedSnapshot) error) error {
	registry, release, err := owner.current()
	if err != nil {
		return err
	}
	defer release()
	return registry.WithWitnessedSnapshot(ctx, securityContext, use)
}

func (owner *runtimeImportActivatedRegistryV1) WithWitnessedSnapshotAuthority(ctx context.Context, securityContext domainsecurity.TurnSecurityContext, use func(registryport.WitnessedSnapshot, registryport.WitnessedSnapshotCapability) error) error {
	registry, release, err := owner.current()
	if err != nil {
		return err
	}
	defer release()
	return registry.WithWitnessedSnapshotAuthority(ctx, securityContext, use)
}

func (owner *runtimeImportActivatedRegistryV1) WithFactFinalWitnessAuthority(ctx context.Context, request registryport.FactFinalWitnessRequest, use func(registryport.FactFinalWitnessCapability) error) error {
	registry, release, err := owner.current()
	if err != nil {
		return err
	}
	defer release()
	return registry.WithFactFinalWitnessAuthority(ctx, request, use)
}

func (owner *runtimeImportActivatedRegistryV1) VerifyFactFinalWitnessCurrent(ctx context.Context, record domainevidence.PrivateAcceptedFinalRecord) error {
	registry, release, err := owner.current()
	if err != nil {
		return err
	}
	defer release()
	return registry.VerifyFactFinalWitnessCurrent(ctx, record)
}

func (owner *runtimeImportActivatedRegistryV1) ListRegistries(ctx context.Context, contexts []domainsecurity.TurnSecurityContext) ([]registryport.InventoryRecord, error) {
	registry, release, err := owner.current()
	if err != nil {
		return nil, err
	}
	defer release()
	return registry.ListRegistries(ctx, contexts)
}

func (owner *runtimeImportActivatedRegistryV1) HasRecords(ctx context.Context) (bool, error) {
	registry, release, err := owner.current()
	if err != nil {
		return false, err
	}
	defer release()
	return registry.HasRecords(ctx)
}

func (owner *runtimeImportActivatedRegistryV1) WithRecoveredFactFinalWitness(ctx context.Context, record domainevidence.PrivateAcceptedFinalRecord, use func(registryport.FactFinalWitnessCapability) error) error {
	registry, release, err := owner.current()
	if err != nil {
		return err
	}
	defer release()
	return registry.WithRecoveredFactFinalWitness(ctx, record, use)
}
