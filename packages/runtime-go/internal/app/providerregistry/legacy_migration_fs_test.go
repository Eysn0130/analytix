package providerregistry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	providerregistryfs "analytix.local/runtime-go/internal/adapters/outbound/providerregistryfs"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
)

func TestFreshFilesystemRegistryStoreRecoversLegacyMigrationCrashPoints(t *testing.T) {
	t.Parallel()

	points := []FaultPoint{
		FaultAfterMigrationRecoveryPrepared,
		FaultAfterMigrationRecoverySecretDurable,
		FaultAfterMigrationRecoverySecretDurableRecorded,
		FaultAfterMigrationRecoveryReadbackVerified,
		FaultAfterMigrationRecoveryVerifiedRecorded,
	}
	for _, point := range points {
		point := point
		t.Run(string(point), func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			dataDir := t.TempDir()
			firstStore, err := providerregistryfs.New(dataDir)
			if err != nil {
				t.Fatalf("providerregistryfs.New(first) error = %v", err)
			}
			initialState := newMemoryRegistryStore().snapshot()
			if err := firstStore.WithExclusive(ctx, func(transaction registryport.Transaction) error {
				state, err := transaction.Load(ctx)
				initialState = state
				return err
			}); err != nil {
				t.Fatalf("initial filesystem Load() error = %v", err)
			}
			secrets := newMemorySecretStore()
			candidate := legacyMigrationCandidateWithArtifactsFor(
				initialState,
				"migration-filesystem-alpha",
				"provider-filesystem-migration",
				"source-filesystem",
			)
			interrupted := mustManager(t, firstStore, secrets, faultOnce(point))
			_, err = interrupted.PrepareLegacyMigrationRecovery(ctx, candidate)
			if !errors.Is(err, ErrInterrupted) {
				t.Fatalf("PrepareLegacyMigrationRecovery() error = %v, want interrupted", err)
			}
			if err := firstStore.Close(); err != nil {
				t.Fatalf("Close(first) error = %v", err)
			}

			restartedStore, err := providerregistryfs.New(dataDir)
			if err != nil {
				t.Fatalf("providerregistryfs.New(restarted) error = %v", err)
			}
			restarted := mustManager(t, restartedStore, secrets, nil)
			if point == FaultAfterMigrationRecoveryPrepared {
				for attempt := 0; attempt < 2; attempt++ {
					if err := restarted.Recover(ctx); !errors.Is(err, registryport.ErrVerification) {
						t.Fatalf("fresh filesystem Recover() missing secret error = %v, want verification", err)
					}
				}
				for attempt := 0; attempt < 2; attempt++ {
					if _, err := restarted.PrepareLegacyMigrationRecovery(ctx, candidate); !errors.Is(err, registryport.ErrVerification) {
						t.Fatalf("fresh filesystem exact replay without protected candidate error = %v, want verification", err)
					}
				}
				if err := restartedStore.WithExclusive(ctx, func(transaction registryport.Transaction) error {
					state, err := transaction.Load(ctx)
					if err != nil {
						return err
					}
					recovery := state.LegacyMigrationRecoveries[candidate.MigrationID]
					if recovery.Phase != "prepared" || len(state.Providers) != 0 ||
						secrets.preparedCount() != 1 || secrets.recordCount() != 0 {
						t.Fatalf("missing protected candidate replay changed filesystem state: %#v", state)
					}
					return nil
				}); err != nil {
					t.Fatalf("prepared filesystem Load() error = %v", err)
				}
				if err := restartedStore.Close(); err != nil {
					t.Fatalf("Close(restarted prepared state) error = %v", err)
				}
				return
			} else if err := restarted.Recover(ctx); err != nil {
				t.Fatalf("fresh filesystem Recover() error = %v", err)
			}
			if err := restarted.Recover(ctx); err != nil {
				t.Fatalf("repeat filesystem Recover() error = %v", err)
			}
			if err := restartedStore.Close(); err != nil {
				t.Fatalf("Close(restarted) error = %v", err)
			}

			secondRestartStore, err := providerregistryfs.New(dataDir)
			if err != nil {
				t.Fatalf("providerregistryfs.New(second restart) error = %v", err)
			}
			if err := mustManager(t, secondRestartStore, secrets, nil).Recover(ctx); err != nil {
				t.Fatalf("second fresh filesystem Recover() error = %v", err)
			}
			if err := secondRestartStore.WithExclusive(ctx, func(transaction registryport.Transaction) error {
				state, err := transaction.Load(ctx)
				if err != nil {
					return err
				}
				recovery := state.LegacyMigrationRecoveries[candidate.MigrationID]
				if recovery.Phase != "verified" || len(state.Transactions) != 0 || len(state.Providers) != 0 ||
					!secrets.hasActive(recovery.RecoveryCredentialRef) {
					t.Fatalf("restarted filesystem Registry = %#v", state)
				}
				return nil
			}); err != nil {
				t.Fatalf("final filesystem Load() error = %v", err)
			}

			registryBytes, err := os.ReadFile(filepath.Join(dataDir, "private", "provider-registry", "registry.v1.json"))
			if err != nil {
				t.Fatalf("ReadFile(Registry) error = %v", err)
			}
			protectedValues := [][]byte{candidate.Credential}
			for _, locator := range candidate.ActiveCredentialLocators {
				protectedValues = append(protectedValues, []byte(locator))
			}
			for _, artifact := range candidate.RollbackCredentialArtifacts {
				protectedValues = append(protectedValues, artifact.Credential)
				for _, locator := range artifact.Locators {
					protectedValues = append(protectedValues, []byte(locator))
				}
			}
			for _, protected := range protectedValues {
				if bytes.Contains(registryBytes, protected) {
					t.Fatalf("filesystem Registry contains protected v2 value of length %d", len(protected))
				}
			}
		})
	}
}

func TestFreshFilesystemRegistryStoreResumesExplicitLegacyMigrationRollback(t *testing.T) {
	t.Parallel()

	t.Run("source restoration pending", func(t *testing.T) {
		ctx := context.Background()
		dataDir, store, secrets, candidate, committed := committedFilesystemLegacyMigration(
			t, "source-pending",
		)
		interrupted := mustManager(
			t, store, secrets, faultOnce(FaultAfterMigrationRollbackSourceRestoreRecorded),
		)
		if _, err := interrupted.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
			MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
			SourceSHA256:                 candidate.SourceSHA256,
			SourcePhysicalIdentitySHA256: candidate.SourcePhysicalIdentitySHA256,
			RecoveryCredentialRef:        committed.RecoveryCredentialRef,
			Confirmation:                 LegacyMigrationRollbackConfirmation,
		}); !errors.Is(err, ErrInterrupted) {
			t.Fatalf("BeginLegacyMigrationRollback() error = %v, want interrupted", err)
		}
		if err := store.Close(); err != nil {
			t.Fatalf("Close(interrupted) error = %v", err)
		}

		restartedStore, err := providerregistryfs.New(dataDir)
		if err != nil {
			t.Fatalf("providerregistryfs.New(restarted) error = %v", err)
		}
		restarted := mustManager(t, restartedStore, secrets, nil)
		for attempt := 0; attempt < 2; attempt++ {
			if err := restarted.Recover(ctx); err != nil {
				t.Fatalf("Recover(%d) error = %v", attempt+1, err)
			}
			begin, err := restarted.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
				MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
				SourceSHA256:                 candidate.SourceSHA256,
				SourcePhysicalIdentitySHA256: candidate.SourcePhysicalIdentitySHA256,
				RecoveryCredentialRef:        committed.RecoveryCredentialRef,
				Confirmation:                 LegacyMigrationRollbackConfirmation,
			})
			if err != nil || begin.Status != LegacyMigrationRollbackStatusCleanedSourceRequired {
				t.Fatalf("BeginLegacyMigrationRollback(replay %d) = %#v, %v", attempt+1, begin, err)
			}
		}
		state, err := restarted.Snapshot(ctx)
		if err != nil {
			t.Fatalf("Snapshot(restarted) error = %v", err)
		}
		if state.LegacyMigrationRecoveries[candidate.MigrationID].Phase !=
			domainregistry.LegacyMigrationRecoveryPhaseRollbackCleanedSourcePending || len(state.Providers) != 1 {
			t.Fatalf("restarted source-pending state = %#v", state)
		}
		if err := restartedStore.Close(); err != nil {
			t.Fatalf("Close(restarted) error = %v", err)
		}
	})

	t.Run("rollback committed before finalization", func(t *testing.T) {
		ctx := context.Background()
		dataDir, store, secrets, candidate, committed := committedFilesystemLegacyMigration(
			t, "rollback-committed",
		)
		manager := mustManager(t, store, secrets, nil)
		begin, err := manager.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
			MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
			SourceSHA256:                 candidate.SourceSHA256,
			SourcePhysicalIdentitySHA256: candidate.SourcePhysicalIdentitySHA256,
			RecoveryCredentialRef:        committed.RecoveryCredentialRef,
			Confirmation:                 LegacyMigrationRollbackConfirmation,
		})
		if err != nil {
			t.Fatalf("BeginLegacyMigrationRollback() error = %v", err)
		}
		if begin.Status != LegacyMigrationRollbackStatusCleanedSourceRequired {
			t.Fatalf("BeginLegacyMigrationRollback() result = %#v", begin)
		}
		before, err := manager.Snapshot(ctx)
		if err != nil {
			t.Fatalf("Snapshot(before rollback) error = %v", err)
		}
		interrupted := mustManager(
			t, store, secrets, faultOnce(FaultAfterMigrationRollbackCommittedRecorded),
		)
		if _, err := legacyMigrationTestCommitRollbackWithLiveSourceAuthority(
			t, ctx, interrupted, before, candidate, committed.RecoveryCredentialRef,
		); !errors.Is(err, ErrInterrupted) {
			t.Fatalf("CommitLegacyMigrationRollback() error = %v, want interrupted", err)
		}
		if err := store.Close(); err != nil {
			t.Fatalf("Close(interrupted) error = %v", err)
		}

		restartedStore, err := providerregistryfs.New(dataDir)
		if err != nil {
			t.Fatalf("providerregistryfs.New(restarted) error = %v", err)
		}
		restarted := mustManager(t, restartedStore, secrets, nil)
		if err := restarted.Recover(ctx); err != nil {
			t.Fatalf("Recover() error = %v", err)
		}
		for attempt := 0; attempt < 2; attempt++ {
			result, err := legacyMigrationTestCommitRollbackWithLiveSourceAuthority(
				t, ctx, restarted, before, candidate, committed.RecoveryCredentialRef,
			)
			if err != nil || result.Status != LegacyMigrationRollbackStatusCommittedRecoveryRetained {
				t.Fatalf("CommitLegacyMigrationRollback(replay %d) = %#v, %v", attempt+1, result, err)
			}
		}
		state, err := restarted.Snapshot(ctx)
		if err != nil {
			t.Fatalf("Snapshot(restarted) error = %v", err)
		}
		if state.LegacyMigrationRecoveries[candidate.MigrationID].Phase !=
			"rollback-committed-recovery-retained" || len(state.Providers) != 0 ||
			!secrets.hasActive(committed.Provider.CredentialRef) ||
			!secrets.hasActive(string(committed.RecoveryCredentialRef)) {
			t.Fatalf("restarted rollback-committed state = %#v", state)
		}
		if err := restartedStore.Close(); err != nil {
			t.Fatalf("Close(restarted) error = %v", err)
		}
	})
}

func TestFreshFilesystemRegistryDiscoversActionableLegacyMigrationRecoveryDescriptors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataDir := t.TempDir()
	secrets := newMemorySecretStore()
	func() {
		store, err := providerregistryfs.New(dataDir)
		if err != nil {
			t.Fatalf("providerregistryfs.New(setup) error = %v", err)
		}
		defer func() {
			if closeErr := store.Close(); closeErr != nil {
				t.Fatalf("Close(setup) error = %v", closeErr)
			}
		}()
		manager := mustManager(t, store, secrets, nil)
		sourceLocator := "current:analytix-settings.json"
		sourceSnapshot := []byte(`{"provider":{"providers":[{"id":"alpha","apiKey":"synthetic-inventory-alpha"},{"id":"beta","apiKey":"synthetic-inventory-beta"}]}}`)
		digest := sha256.Sum256(sourceSnapshot)
		sourceSHA256 := hex.EncodeToString(digest[:])
		var anchorCandidate LegacyMigrationCandidate
		for index, providerID := range []string{"provider-inventory-alpha", "provider-inventory-beta"} {
			before, err := manager.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			candidate := legacyMigrationCandidateFor(
				before, "migration-inventory-"+providerID, providerID, "inventory-source",
				"synthetic-inventory-"+providerID,
			)
			candidate.SourceLocator = sourceLocator
			candidate.SourceSHA256 = sourceSHA256
			candidate.SourceSnapshot = bytes.Clone(sourceSnapshot)
			candidate.ActiveCredentialLocators = []string{
				fmt.Sprintf("%s:provider.providers[%d].apiKey", sourceLocator, index),
			}
			if index == 0 {
				anchorCandidate = candidate
			} else {
				legacyMigrationTestSharePhysicalSource(t, anchorCandidate, &candidate)
			}
			recovery, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
			if err != nil {
				t.Fatalf("PrepareLegacyMigrationRecovery(%d) error = %v", index, err)
			}
			if _, err := manager.CommitVerifiedLegacyMigrationRecovery(
				ctx, legacyMigrationCommitCommandFor(before, candidate, recovery),
			); err != nil {
				t.Fatalf("CommitVerifiedLegacyMigrationRecovery(%d) error = %v", index, err)
			}
		}
	}()

	restartedStore, err := providerregistryfs.New(dataDir)
	if err != nil {
		t.Fatalf("providerregistryfs.New(restarted) error = %v", err)
	}
	defer func() {
		if closeErr := restartedStore.Close(); closeErr != nil {
			t.Fatalf("Close(restarted) error = %v", closeErr)
		}
	}()
	restarted := mustManager(t, restartedStore, secrets, nil)
	inventory, err := restarted.InventoryLegacyMigrationRecoveries(ctx)
	if err != nil {
		t.Fatalf("InventoryLegacyMigrationRecoveries() error = %v", err)
	}
	if len(inventory.Recoveries) != 2 ||
		inventory.Recoveries[0].SourceLocator != "current:analytix-settings.json" ||
		inventory.Recoveries[0].SourceSHA256 != inventory.Recoveries[1].SourceSHA256 ||
		inventory.Recoveries[0].CommitOrder >= inventory.Recoveries[1].CommitOrder ||
		inventory.Recoveries[0].RecoveryCredentialRef == "" ||
		inventory.Recoveries[1].RecoveryCredentialRef == "" {
		t.Fatalf("fresh inventory = %#v", inventory)
	}
	for _, descriptor := range inventory.Recoveries {
		begin, err := restarted.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
			MigrationID: descriptor.MigrationID, SourceLocator: descriptor.SourceLocator,
			SourceSHA256: descriptor.SourceSHA256, RecoveryCredentialRef: descriptor.RecoveryCredentialRef,
			SourcePhysicalIdentitySHA256: descriptor.SourcePhysicalIdentitySHA256,
			Confirmation:                 LegacyMigrationRollbackConfirmation,
		})
		if err != nil || begin.Status != LegacyMigrationRollbackStatusCleanedSourceRequired {
			t.Fatalf("BeginLegacyMigrationRollback(discovered %q) = %#v, %v", descriptor.MigrationID, begin, err)
		}
	}
	for index := len(inventory.Recoveries) - 1; index >= 0; index-- {
		descriptor := inventory.Recoveries[index]
		before, err := restarted.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		candidate := LegacyMigrationCandidate{
			MigrationID: descriptor.MigrationID, SourceLocator: descriptor.SourceLocator,
			SourceSHA256:                 descriptor.SourceSHA256,
			SourcePhysicalIdentitySHA256: descriptor.SourcePhysicalIdentitySHA256,
			ExpectedCleanedSourceSHA256:  descriptor.ExpectedCleanedSourceSHA256,
			Provider:                     domainregistry.ProviderInput{ID: descriptor.ProviderID},
		}
		if _, err := legacyMigrationTestCommitRollbackWithLiveSourceAuthority(
			t, ctx, restarted, before, candidate, descriptor.RecoveryCredentialRef,
		); err != nil {
			t.Fatalf("CommitLegacyMigrationRollback(discovered %q) error = %v", descriptor.MigrationID, err)
		}
	}
	anchor := inventory.Recoveries[0]
	anchorCandidate := LegacyMigrationCandidate{
		MigrationID: anchor.MigrationID, SourceLocator: anchor.SourceLocator,
		SourceSHA256:                 anchor.SourceSHA256,
		SourcePhysicalIdentitySHA256: anchor.SourcePhysicalIdentitySHA256,
		ExpectedCleanedSourceSHA256:  anchor.ExpectedCleanedSourceSHA256,
		Provider:                     domainregistry.ProviderInput{ID: anchor.ProviderID},
	}
	finalized, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
		t, ctx, restarted, anchorCandidate, anchor.RecoveryCredentialRef,
		anchor.ExpectedCleanedSourceSHA256,
	)
	if err != nil || finalized.Outcome != LegacyMigrationFinalizationOutcomeRolledBack {
		t.Fatalf("FinalizeLegacyMigrationRecovery(discovered group) = %#v, %v", finalized, err)
	}
	repeated, err := restarted.FinalizeLegacyMigrationRecovery(ctx, FinalizeLegacyMigrationRecoveryCommand{
		MigrationID: anchor.MigrationID, SourceLocator: anchor.SourceLocator,
		SourceSHA256: anchor.SourceSHA256, VerifiedSourceSHA256: anchor.ExpectedCleanedSourceSHA256,
		SourcePhysicalIdentitySHA256: anchor.SourcePhysicalIdentitySHA256,
		VerifiedSource:               bytes.Clone(legacyMigrationTestCleanedSource),
		Confirmation:                 LegacyMigrationFinalizeConfirmation,
	})
	if err != nil || repeated.Status != LegacyMigrationFinalizationStatusAlreadyFinalized {
		t.Fatalf("FinalizeLegacyMigrationRecovery(repeat discovered group) = %#v, %v", repeated, err)
	}
	after, err := restarted.InventoryLegacyMigrationRecoveries(ctx)
	if err != nil || len(after.Recoveries) != 2 {
		t.Fatalf("InventoryLegacyMigrationRecoveries(after finalization) = %#v, %v", after, err)
	}
}

func TestFreshFilesystemRegistryStoreRecoversLegacyMigrationFinalizationCrashPoints(t *testing.T) {
	t.Parallel()

	for _, point := range []FaultPoint{
		FaultAfterMigrationFinalizingRecorded,
		FaultAfterMigrationFinalizationWinnerDeleteIntent,
		FaultAfterMigrationFinalizationWinnerTombstoned,
		FaultAfterMigrationFinalizationWinnerDeleted,
		FaultAfterMigrationFinalizedRecorded,
	} {
		point := point
		t.Run(string(point), func(t *testing.T) {
			ctx := context.Background()
			dataDir, store, secrets, candidate, committed := committedFilesystemLegacyMigration(
				t, "finalize-"+string(point),
			)
			manager := mustManager(t, store, secrets, nil)
			begin, err := manager.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
				MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
				SourceSHA256:                 candidate.SourceSHA256,
				SourcePhysicalIdentitySHA256: candidate.SourcePhysicalIdentitySHA256,
				RecoveryCredentialRef:        committed.RecoveryCredentialRef,
				Confirmation:                 LegacyMigrationRollbackConfirmation,
			})
			if err != nil {
				t.Fatalf("BeginLegacyMigrationRollback() error = %v", err)
			}
			if begin.Status != LegacyMigrationRollbackStatusCleanedSourceRequired {
				t.Fatalf("BeginLegacyMigrationRollback() result = %#v", begin)
			}
			before, err := manager.Snapshot(ctx)
			if err != nil {
				t.Fatalf("Snapshot(before rollback) error = %v", err)
			}
			if _, err := legacyMigrationTestCommitRollbackWithLiveSourceAuthority(
				t, ctx, manager, before, candidate, committed.RecoveryCredentialRef,
			); err != nil {
				t.Fatalf("CommitLegacyMigrationRollback() error = %v", err)
			}
			interrupted := mustManager(t, store, secrets, faultOnce(point))
			if _, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
				t, ctx, interrupted, candidate, committed.RecoveryCredentialRef,
				candidate.ExpectedCleanedSourceSHA256,
			); !errors.Is(err, ErrInterrupted) {
				t.Fatalf("FinalizeLegacyMigrationRecovery() error = %v, want interrupted", err)
			}
			if err := store.Close(); err != nil {
				t.Fatalf("Close(interrupted) error = %v", err)
			}

			restartedStore, err := providerregistryfs.New(dataDir)
			if err != nil {
				t.Fatalf("providerregistryfs.New(restarted) error = %v", err)
			}
			restarted := mustManager(t, restartedStore, secrets, nil)
			if err := restarted.Recover(ctx); err != nil {
				t.Fatalf("Recover(without live source authority) error = %v", err)
			}
			if registryState, snapshotErr := restarted.Snapshot(ctx); snapshotErr != nil {
				t.Fatal(snapshotErr)
			} else if registryState.LegacyMigrationRecoveries[candidate.MigrationID].Phase !=
				domainregistry.LegacyMigrationRecoveryPhaseRollbackRecoveryRetained {
				if _, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
					t, ctx, restarted, candidate, committed.RecoveryCredentialRef,
					candidate.ExpectedCleanedSourceSHA256,
				); err != nil {
					t.Fatalf("FinalizeLegacyMigrationRecovery(fresh live authority) error = %v", err)
				}
			}
			repeated, err := restarted.FinalizeLegacyMigrationRecovery(
				ctx,
				legacyMigrationTestFinalizeCommand(
					t, candidate, "", candidate.ExpectedCleanedSourceSHA256,
				),
			)
			if err != nil || repeated.Status != LegacyMigrationFinalizationStatusAlreadyFinalized ||
				repeated.Outcome != LegacyMigrationFinalizationOutcomeRolledBack {
				t.Fatalf("FinalizeLegacyMigrationRecovery(repeat) = %#v, %v", repeated, err)
			}
			if secrets.exists(committed.Provider.CredentialRef) ||
				!secrets.hasActive(string(committed.RecoveryCredentialRef)) {
				t.Fatal("fresh rollback finalization did not retain only protected recovery")
			}
			if err := restartedStore.Close(); err != nil {
				t.Fatalf("Close(restarted) error = %v", err)
			}
		})
	}
}

func committedFilesystemLegacyMigration(
	t *testing.T,
	suffix string,
) (string, *providerregistryfs.Store, *memorySecretStore, LegacyMigrationCandidate, CommitVerifiedLegacyMigrationRecoveryResult) {
	t.Helper()
	ctx := context.Background()
	dataDir := t.TempDir()
	store, err := providerregistryfs.New(dataDir)
	if err != nil {
		t.Fatalf("providerregistryfs.New() error = %v", err)
	}
	secrets := newMemorySecretStore()
	manager := mustManager(t, store, secrets, nil)
	initial, err := manager.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot(initial) error = %v", err)
	}
	candidate := legacyMigrationCandidateWithSourceSnapshotFor(
		initial, "migration-fs-"+suffix, "provider-fs-"+suffix, "source-fs-"+suffix,
	)
	recovery, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
	if err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
	}
	committed, err := manager.CommitVerifiedLegacyMigrationRecovery(
		ctx, legacyMigrationCommitCommandFor(initial, candidate, recovery),
	)
	if err != nil {
		t.Fatalf("CommitVerifiedLegacyMigrationRecovery() error = %v", err)
	}
	return dataDir, store, secrets, candidate, committed
}

func TestCompetingFilesystemRegistryManagersHaveOneLegacyMigrationRecoveryWinner(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataDir := t.TempDir()
	firstStore, err := providerregistryfs.New(dataDir)
	if err != nil {
		t.Fatalf("providerregistryfs.New(first) error = %v", err)
	}
	secondStore, err := providerregistryfs.New(dataDir)
	if err != nil {
		t.Fatalf("providerregistryfs.New(second) error = %v", err)
	}
	initialState := newMemoryRegistryStore().snapshot()
	if err := firstStore.WithExclusive(ctx, func(transaction registryport.Transaction) error {
		state, err := transaction.Load(ctx)
		initialState = state
		return err
	}); err != nil {
		t.Fatalf("initial filesystem Load() error = %v", err)
	}
	secrets := newMemorySecretStore()
	managers := []*Manager{
		mustManager(t, firstStore, secrets, nil),
		mustManager(t, secondStore, secrets, nil),
	}
	candidates := []LegacyMigrationCandidate{
		legacyMigrationCandidateFor(
			initialState, "migration-filesystem-competing-alpha", "provider-filesystem-competing",
			"source-filesystem-alpha", "synthetic-filesystem-alpha",
		),
		legacyMigrationCandidateFor(
			initialState, "migration-filesystem-competing-beta", "provider-filesystem-competing",
			"source-filesystem-beta", "synthetic-filesystem-beta",
		),
	}
	start := make(chan struct{})
	errorsSeen := make(chan error, 2)
	var waitGroup sync.WaitGroup
	for index := range managers {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			<-start
			_, err := managers[index].PrepareLegacyMigrationRecovery(ctx, candidates[index])
			errorsSeen <- err
		}(index)
	}
	close(start)
	waitGroup.Wait()
	close(errorsSeen)
	var successes, conflicts int
	for err := range errorsSeen {
		if err == nil {
			successes++
		} else if errors.Is(err, registryport.ErrConflict) {
			conflicts++
		} else {
			t.Fatalf("competing filesystem PrepareLegacyMigrationRecovery() error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 || secrets.recordCount() != 1 || secrets.preparedCount() != 1 {
		t.Fatalf("competing filesystem outcome successes=%d conflicts=%d records=%d prepared=%d",
			successes, conflicts, secrets.recordCount(), secrets.preparedCount())
	}
	if err := secondStore.WithExclusive(ctx, func(transaction registryport.Transaction) error {
		state, err := transaction.Load(ctx)
		if err != nil {
			return err
		}
		if len(state.LegacyMigrationRecoveries) != 1 || len(state.Transactions) != 0 || len(state.Providers) != 0 {
			t.Fatalf("final filesystem Registry = %#v", state)
		}
		return nil
	}); err != nil {
		t.Fatalf("final filesystem Load() error = %v", err)
	}
}

func TestFreshFilesystemRegistryStoreReconcilesCommittedRetainedLegacyMigrationWinner(t *testing.T) {
	t.Parallel()

	points := []FaultPoint{
		FaultAfterCandidateDurableRecorded,
		FaultAfterReadbackVerified,
		FaultBeforeMigrationRecoveryCommittedRetainedRecord,
		FaultAfterMigrationRecoveryCommittedRetainedRecorded,
	}
	for _, point := range points {
		point := point
		t.Run(string(point), func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			dataDir := t.TempDir()
			firstStore, err := providerregistryfs.New(dataDir)
			if err != nil {
				t.Fatalf("providerregistryfs.New(first) error = %v", err)
			}
			var initialState = newMemoryRegistryStore().snapshot()
			if err := firstStore.WithExclusive(ctx, func(transaction registryport.Transaction) error {
				state, err := transaction.Load(ctx)
				initialState = state
				return err
			}); err != nil {
				t.Fatalf("initial filesystem Load() error = %v", err)
			}
			secrets := newMemorySecretStore()
			candidate := legacyMigrationCandidateWithArtifactsFor(
				initialState,
				"migration-filesystem-commit",
				"provider-filesystem-commit",
				"source-filesystem-commit",
			)
			base := mustManager(t, firstStore, secrets, nil)
			recovery, err := base.PrepareLegacyMigrationRecovery(ctx, candidate)
			if err != nil {
				t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
			}
			command := legacyMigrationCommitCommandFor(initialState, candidate, recovery)
			interrupted := mustManager(t, firstStore, secrets, faultOnce(point))
			if _, err := interrupted.CommitVerifiedLegacyMigrationRecovery(ctx, command); !errors.Is(err, ErrInterrupted) {
				t.Fatalf("CommitVerifiedLegacyMigrationRecovery() error = %v, want interrupted", err)
			}
			if err := firstStore.Close(); err != nil {
				t.Fatalf("Close(first) error = %v", err)
			}

			restartedStore, err := providerregistryfs.New(dataDir)
			if err != nil {
				t.Fatalf("providerregistryfs.New(restarted) error = %v", err)
			}
			restarted := mustManager(t, restartedStore, secrets, nil)
			if err := restarted.Recover(ctx); err != nil {
				t.Fatalf("fresh filesystem Recover() error = %v", err)
			}
			result, err := restarted.CommitVerifiedLegacyMigrationRecovery(ctx, command)
			if err != nil {
				t.Fatalf("repeated filesystem CommitVerifiedLegacyMigrationRecovery() error = %v", err)
			}
			if result.Status != LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained ||
				!result.SafeToRemoveLegacyPlaintext {
				t.Fatalf("filesystem committed result = %#v", result)
			}
			if err := restarted.Recover(ctx); err != nil {
				t.Fatalf("repeat filesystem Recover() error = %v", err)
			}
			if err := restartedStore.WithExclusive(ctx, func(transaction registryport.Transaction) error {
				state, err := transaction.Load(ctx)
				if err != nil {
					return err
				}
				winner := state.Providers[candidate.Provider.ID]
				storedRecovery := state.LegacyMigrationRecoveries[candidate.MigrationID]
				if len(state.Providers) != 1 || len(state.Transactions) != 0 ||
					storedRecovery.Phase != "provider-committed-recovery-retained" ||
					winner.CredentialRef != storedRecovery.CommittedProviderCredentialRef ||
					winner.CredentialRef == storedRecovery.RecoveryCredentialRef ||
					!secrets.hasActive(winner.CredentialRef) ||
					!secrets.hasActive(storedRecovery.RecoveryCredentialRef) {
					t.Fatalf("restarted filesystem Registry = %#v", state)
				}
				return nil
			}); err != nil {
				t.Fatalf("final filesystem Load() error = %v", err)
			}
			registryBytes, err := os.ReadFile(filepath.Join(dataDir, "private", "provider-registry", "registry.v1.json"))
			if err != nil {
				t.Fatalf("ReadFile(Registry) error = %v", err)
			}
			protectedValues := [][]byte{candidate.Credential}
			for _, locator := range candidate.ActiveCredentialLocators {
				protectedValues = append(protectedValues, []byte(locator))
			}
			for _, artifact := range candidate.RollbackCredentialArtifacts {
				protectedValues = append(protectedValues, artifact.Credential)
				for _, locator := range artifact.Locators {
					protectedValues = append(protectedValues, []byte(locator))
				}
			}
			for _, protected := range protectedValues {
				if bytes.Contains(registryBytes, protected) {
					t.Fatalf("filesystem Registry contains protected v2 value of length %d", len(protected))
				}
			}
			_, tombstones, deletes := secrets.callCounts()
			if tombstones != 0 || deletes != 0 {
				t.Fatalf("filesystem retained authority cleanup calls tombstones=%d deletes=%d", tombstones, deletes)
			}
			if err := restartedStore.Close(); err != nil {
				t.Fatalf("Close(restarted) error = %v", err)
			}
		})
	}
}

func TestFreshFilesystemRegistryStoreRecoversSubsequentLegacyMigrationCommitCrashPoints(t *testing.T) {
	t.Parallel()

	points := []FaultPoint{
		FaultBeforeMigrationProviderTransactionPrepared,
		FaultAfterTransactionPrepared,
		FaultAfterCandidateDurable,
		FaultAfterCandidateDurableRecorded,
		FaultAfterMetadataCommitted,
		FaultAfterReadbackVerified,
		FaultBeforeMigrationRecoveryCommittedRetainedRecord,
		FaultAfterMigrationRecoveryCommittedRetainedRecorded,
	}
	for _, point := range points {
		point := point
		t.Run(string(point), func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			dataDir := t.TempDir()
			firstStore, err := providerregistryfs.New(dataDir)
			if err != nil {
				t.Fatalf("providerregistryfs.New(first) error = %v", err)
			}
			secrets := newMemorySecretStore()
			base := mustManager(t, firstStore, secrets, nil)
			initial, err := base.Snapshot(ctx)
			if err != nil {
				t.Fatalf("initial Snapshot() error = %v", err)
			}
			alpha := legacyMigrationCandidateFor(
				initial, "migration-filesystem-crash-alpha", "provider-filesystem-crash-alpha",
				"filesystem-shared-crash-source", "synthetic-filesystem-crash-alpha-not-a-real-key",
			)
			alphaRecovery, err := base.PrepareLegacyMigrationRecovery(ctx, alpha)
			if err != nil {
				t.Fatalf("PrepareLegacyMigrationRecovery(alpha) error = %v", err)
			}
			if _, err := base.CommitVerifiedLegacyMigrationRecovery(
				ctx, legacyMigrationCommitCommandFor(initial, alpha, alphaRecovery),
			); err != nil {
				t.Fatalf("CommitVerifiedLegacyMigrationRecovery(alpha) error = %v", err)
			}
			retainedState, err := base.Snapshot(ctx)
			if err != nil {
				t.Fatalf("retained Snapshot() error = %v", err)
			}
			retainedProvider := retainedState.Providers[alpha.Provider.ID]
			retainedRecovery := retainedState.LegacyMigrationRecoveries[alpha.MigrationID]
			beta := legacyMigrationCandidateFor(
				retainedState, "migration-filesystem-crash-beta", "provider-filesystem-crash-beta",
				"filesystem-shared-crash-source", "synthetic-filesystem-crash-beta-not-a-real-key",
			)
			betaRecovery, err := base.PrepareLegacyMigrationRecovery(ctx, beta)
			if err != nil {
				t.Fatalf("PrepareLegacyMigrationRecovery(beta) error = %v", err)
			}
			command := legacyMigrationCommitCommandFor(retainedState, beta, betaRecovery)
			interrupted := mustManager(t, firstStore, secrets, faultOnce(point))
			if _, err := interrupted.CommitVerifiedLegacyMigrationRecovery(ctx, command); !errors.Is(err, ErrInterrupted) {
				t.Fatalf("CommitVerifiedLegacyMigrationRecovery(beta) error = %v, want interrupted", err)
			}
			if err := firstStore.Close(); err != nil {
				t.Fatalf("Close(first) error = %v", err)
			}

			restartedStore, err := providerregistryfs.New(dataDir)
			if err != nil {
				t.Fatalf("providerregistryfs.New(restarted) error = %v", err)
			}
			restarted := mustManager(t, restartedStore, secrets, nil)
			if err := restarted.Recover(ctx); err != nil {
				t.Fatalf("fresh filesystem Recover() error = %v", err)
			}
			for attempt := 0; attempt < 2; attempt++ {
				result, err := restarted.CommitVerifiedLegacyMigrationRecovery(ctx, command)
				if err != nil {
					t.Fatalf("repeated filesystem beta commit %d error = %v", attempt+1, err)
				}
				if result.Status != LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained ||
					!result.SafeToRemoveLegacyPlaintext {
					t.Fatalf("repeated filesystem beta commit %d result = %#v", attempt+1, result)
				}
			}
			state, err := restarted.Snapshot(ctx)
			if err != nil {
				t.Fatalf("restarted Snapshot() error = %v", err)
			}
			betaProvider := state.Providers[beta.Provider.ID]
			storedBetaRecovery := state.LegacyMigrationRecoveries[beta.MigrationID]
			if state.Revision != 2 || len(state.Providers) != 2 || len(state.Transactions) != 0 ||
				len(state.LegacyMigrationRecoveries) != 2 || state.SelectedProviderID != alpha.Provider.ID ||
				state.Providers[alpha.Provider.ID].CredentialRef != retainedProvider.CredentialRef ||
				state.LegacyMigrationRecoveries[alpha.MigrationID].RecoveryCredentialRef != retainedRecovery.RecoveryCredentialRef ||
				storedBetaRecovery.Phase != "provider-committed-recovery-retained" ||
				betaProvider.CredentialRef != storedBetaRecovery.CommittedProviderCredentialRef ||
				betaProvider.CredentialRef == retainedProvider.CredentialRef ||
				betaProvider.CredentialRef == storedBetaRecovery.RecoveryCredentialRef ||
				!secrets.hasActive(retainedProvider.CredentialRef) ||
				!secrets.hasActive(retainedRecovery.RecoveryCredentialRef) ||
				!secrets.hasActive(betaProvider.CredentialRef) ||
				!secrets.hasActive(storedBetaRecovery.RecoveryCredentialRef) ||
				secrets.recordCount() != 4 {
				t.Fatalf("restarted subsequent filesystem state = %#v records=%d", state, secrets.recordCount())
			}
			registryBytes, err := os.ReadFile(filepath.Join(dataDir, "private", "provider-registry", "registry.v1.json"))
			if err != nil {
				t.Fatalf("ReadFile(Registry) error = %v", err)
			}
			if bytes.Contains(registryBytes, alpha.Credential) || bytes.Contains(registryBytes, beta.Credential) {
				t.Fatal("restarted subsequent filesystem Registry contains protected payload material")
			}
			if err := restartedStore.Close(); err != nil {
				t.Fatalf("Close(restarted) error = %v", err)
			}
		})
	}
}

func TestFreshFilesystemRegistryStoreReplaysCommittedRetainedLegacyMigrationPrepare(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataDir := t.TempDir()
	secrets := newMemorySecretStore()
	firstStore, err := providerregistryfs.New(dataDir)
	if err != nil {
		t.Fatalf("providerregistryfs.New(first) error = %v", err)
	}
	first := mustManager(t, firstStore, secrets, nil)
	initial, err := first.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot(initial) error = %v", err)
	}
	candidate := legacyMigrationCandidateWithArtifactsFor(
		initial,
		"migration-filesystem-committed-retained-replay",
		"provider-filesystem-committed-retained-replay",
		"source-filesystem-committed-retained-replay",
	)
	recovery, err := first.PrepareLegacyMigrationRecovery(ctx, candidate)
	if err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
	}
	committed, err := first.CommitVerifiedLegacyMigrationRecovery(
		ctx,
		legacyMigrationCommitCommandFor(initial, candidate, recovery),
	)
	if err != nil {
		t.Fatalf("CommitVerifiedLegacyMigrationRecovery() error = %v", err)
	}
	before, err := first.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot(committed) error = %v", err)
	}
	candidate.Expected = expectedFor(before, candidate.Provider.ID)
	registryPath := filepath.Join(dataDir, "private", "provider-registry", "registry.v1.json")
	beforeBytes, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("ReadFile(before replay) error = %v", err)
	}
	if err := firstStore.Close(); err != nil {
		t.Fatalf("Close(first) error = %v", err)
	}

	restartedStore, err := providerregistryfs.New(dataDir)
	if err != nil {
		t.Fatalf("providerregistryfs.New(restarted) error = %v", err)
	}
	restarted := mustManager(t, restartedStore, secrets, nil)
	beforeReads, beforeTombstones, beforeDeletes := secrets.callCounts()
	for attempt := 0; attempt < 2; attempt++ {
		result, err := restarted.PrepareLegacyMigrationRecovery(ctx, cloneLegacyMigrationCandidate(candidate))
		if err != nil {
			t.Fatalf("fresh PrepareLegacyMigrationRecovery(replay %d) error = %v", attempt+1, err)
		}
		if result.Status != LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained ||
			result.MigrationID != committed.MigrationID ||
			result.RecoveryCredentialRef != committed.RecoveryCredentialRef ||
			result.SafeToProceedWithProviderMigration {
			t.Fatalf("fresh PrepareLegacyMigrationRecovery(replay %d) result = %#v", attempt+1, result)
		}
	}
	replayReads, replayTombstones, replayDeletes := secrets.callCounts()
	if replayReads != beforeReads+4 || replayTombstones != beforeTombstones || replayDeletes != beforeDeletes {
		t.Fatalf("fresh filesystem replay secret calls reads=%d->%d tombstones=%d->%d deletes=%d->%d",
			beforeReads, replayReads, beforeTombstones, replayTombstones, beforeDeletes, replayDeletes)
	}
	after, err := restarted.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot(after replay) error = %v", err)
	}
	afterBytes, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("ReadFile(after replay) error = %v", err)
	}
	afterReads, afterTombstones, afterDeletes := secrets.callCounts()
	if !reflect.DeepEqual(before, after) || !bytes.Equal(beforeBytes, afterBytes) ||
		secrets.preparedCount() != 2 || secrets.recordCount() != 2 ||
		afterReads < replayReads || afterTombstones != beforeTombstones || afterDeletes != beforeDeletes {
		t.Fatalf("fresh filesystem replay changed authority: before=%#v after=%#v reads=%d->%d tombstones=%d->%d deletes=%d->%d prepared=%d records=%d",
			before, after, beforeReads, afterReads, beforeTombstones, afterTombstones, beforeDeletes, afterDeletes,
			secrets.preparedCount(), secrets.recordCount())
	}
	if err := restartedStore.Close(); err != nil {
		t.Fatalf("Close(restarted) error = %v", err)
	}
}

func TestFreshFilesystemRegistryStoresPreserveMultipleCommittedRetainedLegacyMigrations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataDir := t.TempDir()
	secrets := newMemorySecretStore()
	selectedProviderID := ""
	winnerRefs := make(map[string]string)
	recoveryRefs := make(map[string]string)
	credentials := make(map[string][]byte)
	sharedSourceSnapshot := []byte(`{"provider":{"providers":[{"id":"provider-filesystem-shared-alpha","apiKey":"synthetic-alpha"},{"id":"provider-filesystem-shared-beta","apiKey":"synthetic-beta"},{"id":"provider-filesystem-shared-gamma","apiKey":"synthetic-gamma"}]}}`)
	sharedSourceDigest := sha256.Sum256(sharedSourceSnapshot)
	sharedSourceSHA256 := hex.EncodeToString(sharedSourceDigest[:])
	sharedCleanedDigest := sha256.Sum256([]byte(`{"provider":{"providers":[{"id":"provider-filesystem-shared-alpha"},{"id":"provider-filesystem-shared-beta"},{"id":"provider-filesystem-shared-gamma"}]}}`))
	sharedExpectedCleanedSourceSHA256 := hex.EncodeToString(sharedCleanedDigest[:])
	for index, migration := range []struct {
		migrationID string
		providerID  string
		credential  string
	}{
		{migrationID: "migration-filesystem-shared-alpha", providerID: "provider-filesystem-shared-alpha", credential: "synthetic-filesystem-shared-alpha-not-a-real-key"},
		{migrationID: "migration-filesystem-shared-beta", providerID: "provider-filesystem-shared-beta", credential: "synthetic-filesystem-shared-beta-not-a-real-key"},
		{migrationID: "migration-filesystem-shared-gamma", providerID: "provider-filesystem-shared-gamma", credential: "synthetic-filesystem-shared-gamma-not-a-real-key"},
	} {
		store, err := providerregistryfs.New(dataDir)
		if err != nil {
			t.Fatalf("providerregistryfs.New(%d) error = %v", index+1, err)
		}
		manager := mustManager(t, store, secrets, nil)
		before, err := manager.Snapshot(ctx)
		if err != nil {
			t.Fatalf("fresh Manager Snapshot(%d) error = %v", index+1, err)
		}
		candidate := legacyMigrationCandidateFor(
			before,
			migration.migrationID,
			migration.providerID,
			"filesystem-shared-settings-source",
			migration.credential,
		)
		candidate.SourceSnapshot = bytes.Clone(sharedSourceSnapshot)
		candidate.SourceSHA256 = sharedSourceSHA256
		candidate.ExpectedCleanedSourceSHA256 = sharedExpectedCleanedSourceSHA256
		candidate.ActiveCredentialLocators = []string{
			fmt.Sprintf("current:analytix-settings.json:provider.providers[%d].apiKey", index),
		}
		recovery, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
		if err != nil {
			t.Fatalf("PrepareLegacyMigrationRecovery(%d) error = %v", index+1, err)
		}
		command := legacyMigrationCommitCommandFor(before, candidate, recovery)
		result, err := manager.CommitVerifiedLegacyMigrationRecovery(ctx, command)
		if err != nil {
			t.Fatalf("CommitVerifiedLegacyMigrationRecovery(%d) error = %v", index+1, err)
		}
		if result.Status != LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained ||
			!result.SafeToRemoveLegacyPlaintext {
			t.Fatalf("CommitVerifiedLegacyMigrationRecovery(%d) result = %#v", index+1, result)
		}
		if repeated, err := manager.CommitVerifiedLegacyMigrationRecovery(ctx, command); err != nil ||
			repeated.Status != LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained ||
			!repeated.SafeToRemoveLegacyPlaintext {
			t.Fatalf("repeated CommitVerifiedLegacyMigrationRecovery(%d) result = %#v error = %v",
				index+1, repeated, err)
		}
		if err := manager.Recover(ctx); err != nil {
			t.Fatalf("fresh Manager Recover(%d) error = %v", index+1, err)
		}
		state, err := manager.Snapshot(ctx)
		if err != nil {
			t.Fatalf("post-commit Snapshot(%d) error = %v", index+1, err)
		}
		if index == 0 {
			selectedProviderID = candidate.Provider.ID
		}
		winner := state.Providers[candidate.Provider.ID]
		storedRecovery := state.LegacyMigrationRecoveries[candidate.MigrationID]
		winnerRefs[candidate.Provider.ID] = winner.CredentialRef
		recoveryRefs[candidate.Provider.ID] = storedRecovery.RecoveryCredentialRef
		credentials[candidate.Provider.ID] = bytes.Clone(candidate.Credential)
		if state.Revision != uint64(index+1) || len(state.Providers) != index+1 ||
			len(state.Transactions) != 0 || len(state.LegacyMigrationRecoveries) != index+1 ||
			state.SelectedProviderID != selectedProviderID || winner.CredentialRef == "" ||
			winner.CredentialRef == storedRecovery.RecoveryCredentialRef ||
			storedRecovery.Phase != "provider-committed-recovery-retained" ||
			storedRecovery.CommittedProviderCredentialRef != winner.CredentialRef {
			t.Fatalf("filesystem migration %d state = %#v", index+1, state)
		}
		for providerID, winnerRef := range winnerRefs {
			recoveryRef := recoveryRefs[providerID]
			if state.Providers[providerID].CredentialRef != winnerRef ||
				state.LegacyMigrationRecoveries["migration-filesystem-shared-"+providerID[len("provider-filesystem-shared-"):]].RecoveryCredentialRef != recoveryRef ||
				!secrets.hasActive(winnerRef) || !secrets.hasActive(recoveryRef) {
				t.Fatalf("filesystem migration %d lost retained refs for %q", index+1, providerID)
			}
			plaintext, err := secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
				CredentialRef: secretstoreport.CredentialRef(winnerRef),
				Purpose:       "provider-api-key",
				Consumer:      RegistryReadbackConsumer,
			})
			if err != nil || !bytes.Equal(plaintext, credentials[providerID]) {
				clear(plaintext)
				t.Fatalf("filesystem migration %d winner readback for %q error = %v", index+1, providerID, err)
			}
			clear(plaintext)
		}
		if err := store.Close(); err != nil {
			t.Fatalf("Close(%d) error = %v", index+1, err)
		}
	}

	finalStore, err := providerregistryfs.New(dataDir)
	if err != nil {
		t.Fatalf("providerregistryfs.New(final) error = %v", err)
	}
	finalManager := mustManager(t, finalStore, secrets, nil)
	if err := finalManager.Recover(ctx); err != nil {
		t.Fatalf("final fresh Manager Recover() error = %v", err)
	}
	finalState, err := finalManager.Snapshot(ctx)
	if err != nil {
		t.Fatalf("final fresh Manager Snapshot() error = %v", err)
	}
	if finalState.Revision != 3 || len(finalState.Providers) != 3 ||
		len(finalState.LegacyMigrationRecoveries) != 3 || finalState.SelectedProviderID != selectedProviderID ||
		secrets.preparedCount() != 6 || secrets.recordCount() != 6 {
		t.Fatalf("final fresh filesystem state = %#v prepared=%d records=%d",
			finalState, secrets.preparedCount(), secrets.recordCount())
	}
	registryBytes, err := os.ReadFile(filepath.Join(dataDir, "private", "provider-registry", "registry.v1.json"))
	if err != nil {
		t.Fatalf("ReadFile(final Registry) error = %v", err)
	}
	for _, credential := range credentials {
		if bytes.Contains(registryBytes, credential) {
			t.Fatal("final filesystem Registry contains a synthetic credential canary")
		}
	}
	if err := finalStore.Close(); err != nil {
		t.Fatalf("Close(final) error = %v", err)
	}
}
