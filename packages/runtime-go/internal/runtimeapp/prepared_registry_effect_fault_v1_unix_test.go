//go:build darwin || linux

package runtimeapp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	datasetsnapshotstore "analytix.local/runtime-go/internal/adapters/outbound/datasetsnapshot"
	evidenceregistrystore "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	settlementstore "analytix.local/runtime-go/internal/adapters/outbound/evidencesettlement"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	datasetsnapshotapp "analytix.local/runtime-go/internal/app/datasetsnapshot"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	evidenceauthorityapp "analytix.local/runtime-go/internal/app/evidenceauthority"
	evidenceregistryapp "analytix.local/runtime-go/internal/app/evidenceregistry"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

// Fault adapters forward to the real enrolled Authority and secure stores.
// They inject failures at exact boundaries, never manufacture a successful
// after-head or commit. The ordinary owner is closed before this composition.
func TestPreparedRegistryEffectActualCASAndReadbackFailures(t *testing.T) {
	for _, fault := range []string{"CAS conflict", "CAS committed return error", "capsule post-CAS readback", "index pre-CAS readback", "different coordinator owner"} {
		t.Run(fault, func(t *testing.T) {
			fixture := newPreparedRegistryEffectFixtureV1(t)
			if err := fixture.composition.registryOwner.Close(); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			privateRoot := filepath.Join(fixture.config.DataDir, "private")
			access, err := privatecastest.NewAccessAuthority(privateRoot)
			if err != nil {
				t.Fatal(err)
			}
			snapshots, err := datasetsnapshotstore.OpenStoresV2(filepath.Join(privateRoot, "dataset-snapshot-authority"), access)
			if err != nil {
				t.Fatal(err)
			}
			defer snapshots.Close()
			indexes, err := evidenceregistrystore.NewAuthorityIndexStoreV2(filepath.Join(privateRoot, "evidence-registry", "indexes"), access)
			if err != nil {
				t.Fatal(err)
			}
			defer indexes.Close()
			capsules, err := evidenceregistrystore.NewAuthorityCapsuleStoreV2(filepath.Join(privateRoot, "evidence-registry", "capsules"), access)
			if err != nil {
				t.Fatal(err)
			}
			defer capsules.Close()
			enrolled, configured, err := loadRuntimeSharedEvidenceEnrollmentV2(ctx, fixture.config, fixture.witness.Authority)
			if err != nil || !configured {
				t.Fatal("enrolled authority configuration missing")
			}
			coordinator := &preparedRegistryFaultCoordinatorV1{Authority: fixture.composition.evidence, fault: fault}
			sealed, err := datasetsnapshotapp.NewSealedV2(datasetsnapshotapp.SealedConfigV2{InstallationID: enrolled.projection.InstallationID, EnrollmentID: enrolled.projection.Enrollment.EnrollmentID, Authority: fixture.witness.Authority, Coordinator: coordinator, LegacyRecords: snapshots.LegacyRecords, Bundles: snapshots.AuthorityBundles, Indexes: snapshots.Indexes, Materials: snapshots.Materials})
			if err != nil {
				t.Fatal(err)
			}
			registryCoordinator := coordinator
			if fault == "different coordinator owner" {
				registryCoordinator = &preparedRegistryFaultCoordinatorV1{Authority: fixture.composition.evidence}
			}
			registry, err := evidenceregistryapp.New(evidenceregistryapp.Config{InstallationID: enrolled.projection.InstallationID, EnrollmentID: enrolled.projection.Enrollment.EnrollmentID, Authority: fixture.witness.Authority, WitnessKeyID: enrolled.projection.Enrollment.WitnessKeyID, WitnessKey: enrolled.witnessKey, Coordinator: registryCoordinator, WitnessChain: fixture.composition.evidence, Indexes: &preparedRegistryFaultIndexesV1{AuthorityIndexStore: indexes, coordinator: coordinator}, Capsules: &preparedRegistryFaultCapsulesV1{AuthorityCapsuleStore: capsules, coordinator: coordinator}, DatasetAuthority: sealed, BindingObserver: filestore.CaseBindingReader{}})
			if err != nil {
				t.Fatal(err)
			}
			var prepared domainevidence.PreparedEvidenceSettlement
			var input registryport.CommitPreparedInput
			var restartReader preparedRegistryRestartReaderV1
			err = sealed.WithCurrentSelectionV2(ctx, fixture.resolve, fixture.securityContext, func(selection datasetsnapshotport.CurrentSelectionV2, cap datasetsnapshotport.CurrentSelectionCapabilityV2) error {
				prepared = fixture.prepare(t, selection)
				restartReader = persistPreparedRegistryRestartFixtureV1(t, fixture.config, prepared)
				input = preparedRegistryCommitInputV1(prepared)
				marker, err := domainevidence.NewHostEvidenceSettlementMarker(prepared)
				if err != nil {
					t.Fatal(err)
				}
				return cap.(datasetsnapshotport.RegistryCommitCapabilityV1).UseExactRegistryCommit(selection, fixture.securityContext, prepared, marker, func(lease context.Context) error {
					_, commitErr := registry.CommitPrepared(lease, input)
					if commitErr == nil {
						t.Fatal("fault boundary unexpectedly reported a successful commit")
					}
					// Even a caller swallowing the error or replaying the receipt
					// cannot manufacture the operation's missing CAS/readback proof.
					coordinator.disabled = true
					_, _ = registry.Replay(lease, fixture.securityContext)
					return nil
				})
			})
			if err == nil {
				t.Fatal("failed actual CAS/readback was accepted as an exact effect")
			}
			head, err := fixture.composition.evidence.ObserveFresh(ctx)
			if err != nil {
				t.Fatal(err)
			}
			expected := uint64(0)
			if fault == "CAS committed return error" || fault == "capsule post-CAS readback" {
				expected = 1
			}
			if head.Bundle.EvidenceRegistryCount != expected || coordinator.committed != (expected == 1) {
				t.Fatal("failure classification differs from actual witnessed commit")
			}
			if fault == "CAS conflict" && !coordinator.conflicted {
				t.Fatal("real Coordinator CAS conflict was not exercised")
			}
			coordinator.disabled = true
			observed, err := registry.Replay(ctx, fixture.securityContext)
			if err != nil {
				t.Fatal(err)
			}
			_, found, err := domainevidence.ResolvePreparedSettlementIssue(observed, prepared)
			if err != nil || found != (expected == 1) {
				t.Fatal("capsule-only residue was confused with a committed issue")
			}
			if expected == 0 {
				return
			}
			before := startupWholeTreeDigest(t, filepath.Join(privateRoot, "evidence-registry"))
			if err := indexes.Close(); err != nil {
				t.Fatal(err)
			}
			if err := capsules.Close(); err != nil {
				t.Fatal(err)
			}
			reopened := runtimeWitnessedRegistryDirectCompositionV2(t, fixture.witness, fixture.config)
			if reopened.registryOwner == nil || reopened.registry == nil {
				t.Fatal("committed-but-reported-error state did not reopen")
			}
			defer reopened.registryOwner.Close()
			receipt, err := reopened.registryOwner.CommitPrepared(ctx, input)
			if err != nil || receipt.ReceiptID != prepared.ReceiptID {
				t.Fatal("exact committed retry failed after restart")
			}
			after, err := reopened.evidence.ObserveFresh(ctx)
			if err != nil || after.Bundle != head.Bundle || before != startupWholeTreeDigest(t, filepath.Join(privateRoot, "evidence-registry")) {
				t.Fatal("restart duplicated issuance after actual CAS failure report")
			}
			settlements, err := settlementstore.NewStore(filepath.Join(privateRoot, "evidence-settlements"))
			if err != nil {
				t.Fatal(err)
			}
			issuer := evidenceapp.Issuer{Registry: reopened.registryOwner, Authority: fixture.witness.Authority, SettlementStore: settlements}
			preserved := startupWholeTreeDigest(t, filepath.Join(privateRoot, "evidence-registry"), filepath.Join(privateRoot, "evidence-settlements"), filepath.Dir(restartReader.path))
			inventory, err := evidenceapp.PreflightEvidenceSettlementInventory(ctx, restartReader, issuer)
			if err != nil || len(inventory.Pending) != 0 || len(inventory.Quarantined) != 1 {
				t.Fatalf("committed Host preparation lost audit-only restart classification: %v", err)
			}
			if err := evidenceapp.ApplyEvidenceSettlementReconciliationInventory(ctx, restartReader, issuer, inventory); err != nil {
				t.Fatal(err)
			}
			finalHead, err := reopened.evidence.ObserveFresh(ctx)
			if err != nil || finalHead.Bundle != head.Bundle || preserved != startupWholeTreeDigest(t, filepath.Join(privateRoot, "evidence-registry"), filepath.Join(privateRoot, "evidence-settlements"), filepath.Dir(restartReader.path)) {
				t.Fatal("startup reconciliation rewrote committed authority or duplicated issuance")
			}
		})
	}
}

// This lower-owner fixture persists its already synthetic, genuinely signed
// preparation and exact marker for the existing startup audit. It does not
// stand in for the native execution/settlement/public-final test.
func persistPreparedRegistryRestartFixtureV1(t *testing.T, config Config, prepared domainevidence.PreparedEvidenceSettlement) preparedRegistryRestartReaderV1 {
	t.Helper()
	body, err := domainevidence.PreparedEvidenceSettlementBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	preparedDir := filepath.Join(config.DataDir, "private", "evidence-settlements", "prepared", prepared.SettlementID[:2])
	if err := os.MkdirAll(preparedDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(preparedDir, prepared.SettlementID+".json"), body, 0600); err != nil {
		t.Fatal(err)
	}
	marker, err := domainevidence.NewHostEvidenceSettlementMarker(prepared)
	if err != nil {
		t.Fatal(err)
	}
	thread := map[string]any{"id": prepared.SecurityContext.ThreadID, "turns": []any{map[string]any{
		"id": prepared.SecurityContext.TurnID, "securityContext": prepared.SecurityContext,
		"items": []any{map[string]any{"id": prepared.ResultItemID, "hostEvidenceSettlement": marker}},
	}}}
	body, err = json.Marshal(thread)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(config.ProductionDurableRoot, "b1-synthetic-restart")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	reader := preparedRegistryRestartReaderV1{path: filepath.Join(root, "thread.json"), threadID: prepared.SecurityContext.ThreadID}
	if err := os.WriteFile(reader.path, body, 0600); err != nil {
		t.Fatal(err)
	}
	return reader
}

type preparedRegistryRestartReaderV1 struct{ path, threadID string }

func (reader preparedRegistryRestartReaderV1) AllThreadIDs() ([]string, error) {
	return []string{reader.threadID}, nil
}

func (reader preparedRegistryRestartReaderV1) GetThread(threadID string) (map[string]any, error) {
	if threadID != reader.threadID {
		return nil, errors.New("synthetic restart thread is unavailable")
	}
	body, err := os.ReadFile(reader.path)
	if err != nil {
		return nil, err
	}
	var thread map[string]any
	err = json.Unmarshal(body, &thread)
	return thread, err
}

type preparedRegistryFaultCoordinatorV1 struct {
	*evidenceauthorityapp.Authority
	fault                           string
	disabled, committed, conflicted bool
}

func (coordinator *preparedRegistryFaultCoordinatorV1) AdvanceEvidenceRegistry(ctx context.Context, input evidenceauthorityport.RegistryAdvanceInput) (evidenceauthorityport.FreshHead, error) {
	if !coordinator.disabled && coordinator.fault == "CAS conflict" {
		if _, err := coordinator.Authority.AdvancePublication(ctx, evidenceauthorityport.PublicationAdvanceInput{ExpectedBundleDigest: input.ExpectedBundleDigest, NextIndexDigest: domainsecurity.SHA256Hex([]byte("intervening-real-publication-CAS"))}); err != nil {
			return evidenceauthorityport.FreshHead{}, err
		}
	}
	head, err := coordinator.Authority.AdvanceEvidenceRegistry(ctx, input)
	if err != nil {
		coordinator.conflicted = errors.Is(err, evidenceauthorityapp.ErrCurrentChanged)
		return head, err
	}
	coordinator.committed = true
	if !coordinator.disabled && coordinator.fault == "CAS committed return error" {
		return evidenceauthorityport.FreshHead{}, errors.New("synthetic failure after actual enrolled registry CAS")
	}
	return head, nil
}

type preparedRegistryFaultIndexesV1 struct {
	registryport.AuthorityIndexStore
	coordinator *preparedRegistryFaultCoordinatorV1
}

func (store *preparedRegistryFaultIndexesV1) Resolve(ctx context.Context, digest string) (domainevidence.EvidenceRegistryAuthorityIndexV2, error) {
	if !store.coordinator.disabled && store.coordinator.fault == "index pre-CAS readback" {
		return domainevidence.EvidenceRegistryAuthorityIndexV2{}, errors.New("synthetic index readback failure before CAS")
	}
	return store.AuthorityIndexStore.Resolve(ctx, digest)
}

type preparedRegistryFaultCapsulesV1 struct {
	registryport.AuthorityCapsuleStore
	coordinator *preparedRegistryFaultCoordinatorV1
}

func (store *preparedRegistryFaultCapsulesV1) Resolve(ctx context.Context, digest string) (domainevidence.EvidenceRegistryAuthorityCapsule, error) {
	if !store.coordinator.disabled && store.coordinator.committed && store.coordinator.fault == "capsule post-CAS readback" {
		return domainevidence.EvidenceRegistryAuthorityCapsule{}, errors.New("synthetic capsule readback failure after actual CAS")
	}
	return store.AuthorityCapsuleStore.Resolve(ctx, digest)
}
