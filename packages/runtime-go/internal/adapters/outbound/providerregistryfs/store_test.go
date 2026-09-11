package providerregistryfs

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
)

func TestStoreStrictPrivateKeyFreeRoundTrip(t *testing.T) {
	t.Parallel()

	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx := context.Background()
	var committed domainregistry.Registry
	if err := store.WithExclusive(ctx, func(transaction registryport.Transaction) error {
		state, err := transaction.Load(ctx)
		if err != nil {
			return err
		}
		provider := syntheticProvider()
		state.Providers[provider.ID] = provider
		state.SelectedProviderID = provider.ID
		state.Revision++
		if err := transaction.Commit(ctx, state); err != nil {
			return err
		}
		committed = state.Clone()
		return nil
	}); err != nil {
		t.Fatalf("WithExclusive(commit) error = %v", err)
	}
	content, err := os.ReadFile(store.registryPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, forbidden := range []string{"synthetic-provider-secret-marker", "apiKey", "accessToken", "rawBody"} {
		if bytes.Contains(content, []byte(forbidden)) {
			t.Fatalf("Registry bytes contain forbidden material %q", forbidden)
		}
	}
	if err := store.WithExclusive(ctx, func(transaction registryport.Transaction) error {
		loaded, err := transaction.Load(ctx)
		if err != nil {
			return err
		}
		loadedBytes, _ := domainregistry.Marshal(loaded)
		committedBytes, _ := domainregistry.Marshal(committed)
		if !bytes.Equal(loadedBytes, committedBytes) {
			t.Fatal("Store round trip changed Registry state")
		}
		return nil
	}); err != nil {
		t.Fatalf("WithExclusive(load) error = %v", err)
	}
}

func TestStoreConcurrentFirstOpenAcrossInstancesUsesOneIncarnation(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	stores := make([]*Store, 12)
	for index := range stores {
		store, err := New(dataDir)
		if err != nil {
			t.Fatalf("New(%d) error = %v", index, err)
		}
		stores[index] = store
	}
	start := make(chan struct{})
	incarnations := make(chan string, len(stores))
	errorsSeen := make(chan error, len(stores))
	var waitGroup sync.WaitGroup
	for _, store := range stores {
		waitGroup.Add(1)
		go func(store *Store) {
			defer waitGroup.Done()
			<-start
			err := store.WithExclusive(context.Background(), func(transaction registryport.Transaction) error {
				state, err := transaction.Load(context.Background())
				if err == nil {
					incarnations <- state.Incarnation
				}
				return err
			})
			errorsSeen <- err
		}(store)
	}
	close(start)
	waitGroup.Wait()
	close(incarnations)
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatalf("concurrent first Load() error = %v", err)
		}
	}
	winner := ""
	for incarnation := range incarnations {
		if winner == "" {
			winner = incarnation
		}
		if incarnation != winner {
			t.Fatalf("incarnation=%q want one winner %q", incarnation, winner)
		}
	}
	if !domainregistry.ValidIncarnation(winner) {
		t.Fatalf("winner incarnation is invalid: %q", winner)
	}
}

func TestStoreAtomicFailuresPreservePriorCommittedBytes(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		hooks *commitHooks
	}{
		{name: "before replace", hooks: &commitHooks{beforeReplace: func() error { return errors.New("synthetic failure") }}},
		{name: "after replace before parent sync", hooks: &commitHooks{afterReplaceBeforeDirectorySync: func() error { return errors.New("synthetic failure") }}},
		{name: "after parent sync before decision", hooks: &commitHooks{afterReplaceDirectorySync: func() error { return errors.New("synthetic failure") }}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			dataDir := t.TempDir()
			base, err := New(dataDir)
			if err != nil {
				t.Fatalf("New(base) error = %v", err)
			}
			ctx := context.Background()
			if err := base.WithExclusive(ctx, func(transaction registryport.Transaction) error {
				_, err := transaction.Load(ctx)
				return err
			}); err != nil {
				t.Fatalf("initialize error = %v", err)
			}
			before, err := os.ReadFile(base.registryPath)
			if err != nil {
				t.Fatalf("ReadFile(before) error = %v", err)
			}
			failing, err := newWithHooks(dataDir, testCase.hooks)
			if err != nil {
				t.Fatalf("newWithHooks() error = %v", err)
			}
			err = failing.WithExclusive(ctx, func(transaction registryport.Transaction) error {
				state, err := transaction.Load(ctx)
				if err != nil {
					return err
				}
				state.Revision++
				return transaction.Commit(ctx, state)
			})
			if !errors.Is(err, registryport.ErrPersistence) {
				t.Fatalf("failing Commit() error = %v, want persistence failure", err)
			}
			after, err := os.ReadFile(base.registryPath)
			if err != nil {
				t.Fatalf("ReadFile(after) error = %v", err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("returned atomic failure changed prior committed bytes")
			}
			for _, suffix := range []string{".commit-journal", ".previous", ".committed"} {
				if _, statErr := os.Lstat(filepath.Join(base.directory, "."+registryFileName+suffix)); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("atomic failure retained recovery artifact %s", suffix)
				}
			}
		})
	}
}

func TestStoreCommitMarkerSyncFailureRollsBackBeforeReturn(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	base, err := New(dataDir)
	if err != nil {
		t.Fatalf("New(base) error = %v", err)
	}
	ctx := context.Background()
	if err := base.WithExclusive(ctx, func(transaction registryport.Transaction) error {
		_, err := transaction.Load(ctx)
		return err
	}); err != nil {
		t.Fatalf("initialize error = %v", err)
	}
	prior, err := os.ReadFile(base.registryPath)
	if err != nil {
		t.Fatalf("ReadFile(prior) error = %v", err)
	}
	failing, err := newWithHooks(dataDir, &commitHooks{observe: func(point commitFaultPoint) error {
		if point == faultCommitMarkerDirectorySync {
			return errors.New("synthetic commit-marker sync failure")
		}
		return nil
	}})
	if err != nil {
		t.Fatalf("newWithHooks() error = %v", err)
	}
	err = failing.WithExclusive(ctx, func(transaction registryport.Transaction) error {
		state, err := transaction.Load(ctx)
		if err != nil {
			return err
		}
		state.Revision++
		return transaction.Commit(ctx, state)
	})
	if !errors.Is(err, registryport.ErrPersistence) {
		t.Fatalf("Commit(marker sync failure) error = %v, want persistence failure", err)
	}
	after, err := os.ReadFile(base.registryPath)
	if err != nil || !bytes.Equal(prior, after) {
		t.Fatal("returned commit-marker failure promoted the candidate")
	}
	failing.hooks = nil
	for index, reader := range []*Store{failing, mustNewStore(t, dataDir)} {
		if err := reader.WithExclusive(ctx, func(transaction registryport.Transaction) error {
			loaded, err := transaction.Load(ctx)
			if err != nil {
				return err
			}
			content, _ := domainregistry.Marshal(loaded)
			if !bytes.Equal(content, prior) {
				t.Fatalf("reader %d promoted the failed candidate", index)
			}
			return nil
		}); err != nil {
			t.Fatalf("reader %d Load() error = %v", index, err)
		}
	}
}

func TestStorePreLinearizationConfirmationFailureCannotPromoteWithoutPriorEvidence(t *testing.T) {
	for _, freshFirst := range []bool{false, true} {
		name := "same-process-first"
		if freshFirst {
			name = "fresh-restart-first"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			dataDir := t.TempDir()
			base := mustNewStore(t, dataDir)
			var prior domainregistry.Registry
			if err := base.WithExclusive(ctx, func(transaction registryport.Transaction) error {
				loaded, err := transaction.Load(ctx)
				prior = loaded
				return err
			}); err != nil {
				t.Fatalf("initialize error = %v", err)
			}
			candidate := prior.Clone()
			candidate.Revision++
			_, backupPath, markerPath := recoveryPaths(base.registryPath)
			journalPath, _, _ := recoveryPaths(base.registryPath)
			failing, err := newWithHooks(dataDir, &commitHooks{observe: func(point commitFaultPoint) error {
				switch point {
				case faultCommitConfirmationDirectorySync:
					if err := removePrivateFileIfPresent(backupPath, 32<<20); err != nil {
						return err
					}
					return errors.New("synthetic commit-confirmation directory sync failure")
				case faultRollbackBackupReestablished:
					return errors.New("synthetic rollback backup reestablishment failure")
				case faultRollbackRequiredReestablished:
					return errors.New("synthetic rollback evidence reestablishment failure")
				}
				return nil
			}})
			if err != nil {
				t.Fatalf("newWithHooks() error = %v", err)
			}
			commitErr := failing.WithExclusive(ctx, func(transaction registryport.Transaction) error {
				return transaction.Commit(ctx, candidate)
			})
			if !errors.Is(commitErr, registryport.ErrPersistence) {
				t.Fatalf("Commit(pre-linearization confirmation failure) error = %v, want persistence failure", commitErr)
			}
			failing.hooks = nil
			fresh := mustNewStore(t, dataDir)
			readers := []*Store{failing, fresh}
			if freshFirst {
				readers = []*Store{fresh, failing}
			}
			candidateBytes, _ := domainregistry.Marshal(candidate)
			for index, reader := range readers {
				loadErr := reader.WithExclusive(ctx, func(transaction registryport.Transaction) error {
					_, err := transaction.Load(ctx)
					return err
				})
				if !errors.Is(loadErr, registryport.ErrPersistence) {
					t.Fatalf("reader %d Load() error = %v, want fail-closed persistence failure", index, loadErr)
				}
				target, err := os.ReadFile(base.registryPath)
				if err != nil || !bytes.Equal(target, candidateBytes) {
					t.Fatalf("reader %d changed ambiguous target bytes", index)
				}
				if _, err := os.Lstat(journalPath); err != nil {
					t.Fatalf("reader %d discarded prior-present journal: %v", index, err)
				}
				if _, err := os.Lstat(markerPath); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("reader %d retained promotable commit marker", index)
				}
			}
		})
	}
}

func TestStorePostLinearizationCleanupFaultReturnsCommittedDecision(t *testing.T) {
	t.Parallel()

	for _, freshFirst := range []bool{false, true} {
		name := "same-process-first"
		if freshFirst {
			name = "fresh-restart-first"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			dataDir := t.TempDir()
			base := mustNewStore(t, dataDir)
			var prior domainregistry.Registry
			if err := base.WithExclusive(ctx, func(transaction registryport.Transaction) error {
				loaded, err := transaction.Load(ctx)
				prior = loaded
				return err
			}); err != nil {
				t.Fatalf("initialize error = %v", err)
			}
			candidate := prior.Clone()
			candidate.Revision++
			failing, err := newWithHooks(dataDir, &commitHooks{observe: func(point commitFaultPoint) error {
				switch point {
				case faultCommitBackupCleanup:
					return errors.New("synthetic post-linearization cleanup failure")
				case faultRollbackBackupReestablished:
					return errors.New("synthetic rollback backup reestablishment failure")
				case faultRollbackRequiredReestablished:
					return errors.New("synthetic rollback evidence reestablishment failure")
				}
				return nil
			}})
			if err != nil {
				t.Fatalf("newWithHooks() error = %v", err)
			}
			if err := failing.WithExclusive(ctx, func(transaction registryport.Transaction) error {
				return transaction.Commit(ctx, candidate)
			}); err != nil {
				t.Fatalf("Commit(post-linearization cleanup failure) error = %v, want committed success", err)
			}
			candidateBytes, _ := domainregistry.Marshal(candidate)
			failing.hooks = nil
			fresh := mustNewStore(t, dataDir)
			readers := []*Store{failing, fresh}
			if freshFirst {
				readers = []*Store{fresh, failing}
			}
			for index, reader := range readers {
				if err := reader.WithExclusive(ctx, func(transaction registryport.Transaction) error {
					loaded, err := transaction.Load(ctx)
					if err != nil {
						return err
					}
					loadedBytes, _ := domainregistry.Marshal(loaded)
					if !bytes.Equal(loadedBytes, candidateBytes) {
						t.Fatalf("reader %d changed the durable committed decision", index)
					}
					return nil
				}); err != nil {
					t.Fatalf("reader %d Load() error = %v", index, err)
				}
			}
		})
	}
}

func TestStoreDecisionProtocolFaultMatrix(t *testing.T) {
	for _, priorPresent := range []bool{false, true} {
		priorName := "prior-absent"
		if priorPresent {
			priorName = "prior-present"
		}
		for _, testCase := range []struct {
			name        string
			faults      map[commitFaultPoint]bool
			wantSuccess bool
		}{
			{name: "rollback-required establishment", faults: map[commitFaultPoint]bool{faultRollbackRequiredEstablished: true}},
			{name: "commit-marker sync", faults: map[commitFaultPoint]bool{faultCommitMarkerDirectorySync: true}},
			{name: "rollback-required re-establishment", faults: map[commitFaultPoint]bool{
				faultCommitMarkerDirectorySync: true, faultRollbackRequiredReestablished: true,
			}},
			{name: "backup cleanup", faults: map[commitFaultPoint]bool{faultCommitBackupCleanup: true}, wantSuccess: true},
			{name: "journal cleanup", faults: map[commitFaultPoint]bool{faultCommitJournalCleanup: true}, wantSuccess: true},
			{name: "cleanup directory sync", faults: map[commitFaultPoint]bool{faultCommitCleanupDirectorySync: true}, wantSuccess: true},
			{name: "commit marker removal", faults: map[commitFaultPoint]bool{faultCommitMarkerRemoval: true}, wantSuccess: true},
			{name: "commit marker removal sync", faults: map[commitFaultPoint]bool{faultCommitMarkerRemovalSync: true}, wantSuccess: true},
		} {
			t.Run(priorName+"/"+testCase.name, func(t *testing.T) {
				ctx := context.Background()
				dataDir := t.TempDir()
				base := mustNewStore(t, dataDir)
				var prior domainregistry.Registry
				var priorBytes []byte
				if priorPresent {
					if err := base.WithExclusive(ctx, func(transaction registryport.Transaction) error {
						loaded, err := transaction.Load(ctx)
						prior = loaded
						return err
					}); err != nil {
						t.Fatalf("initialize error = %v", err)
					}
					priorBytes, _ = domainregistry.Marshal(prior)
				}
				candidate, err := domainregistry.NewRegistry("inc_" + strings.Repeat("R", 43))
				if err != nil {
					t.Fatalf("NewRegistry() error = %v", err)
				}
				if priorPresent {
					candidate = prior.Clone()
					candidate.Revision++
				}
				fired := make(map[commitFaultPoint]bool)
				failing, err := newWithHooks(dataDir, &commitHooks{observe: func(point commitFaultPoint) error {
					if testCase.faults[point] && !fired[point] {
						fired[point] = true
						return errors.New("synthetic decision-protocol fault")
					}
					return nil
				}})
				if err != nil {
					t.Fatalf("newWithHooks() error = %v", err)
				}
				commitErr := failing.WithExclusive(ctx, func(transaction registryport.Transaction) error {
					return transaction.Commit(ctx, candidate)
				})
				if testCase.wantSuccess {
					if commitErr != nil {
						t.Fatalf("Commit() error = %v, want committed success", commitErr)
					}
				} else if !errors.Is(commitErr, registryport.ErrPersistence) {
					t.Fatalf("Commit() error = %v, want persistence failure", commitErr)
				}
				failing.hooks = nil
				var sameProcess domainregistry.Registry
				if err := failing.WithExclusive(ctx, func(transaction registryport.Transaction) error {
					loaded, err := transaction.Load(ctx)
					sameProcess = loaded
					return err
				}); err != nil {
					t.Fatalf("same-process Load() error = %v", err)
				}
				fresh := mustNewStore(t, dataDir)
				var restarted domainregistry.Registry
				if err := fresh.WithExclusive(ctx, func(transaction registryport.Transaction) error {
					loaded, err := transaction.Load(ctx)
					restarted = loaded
					return err
				}); err != nil {
					t.Fatalf("fresh Load() error = %v", err)
				}
				sameBytes, _ := domainregistry.Marshal(sameProcess)
				restartedBytes, _ := domainregistry.Marshal(restarted)
				if !bytes.Equal(sameBytes, restartedBytes) {
					t.Fatal("same-process and fresh restart resolved different decisions")
				}
				candidateBytes, _ := domainregistry.Marshal(candidate)
				if testCase.wantSuccess {
					if !bytes.Equal(sameBytes, candidateBytes) {
						t.Fatal("returned success did not preserve the committed candidate")
					}
				} else if priorPresent {
					if !bytes.Equal(sameBytes, priorBytes) {
						t.Fatal("returned failure later promoted the candidate")
					}
				} else if sameProcess.Incarnation == candidate.Incarnation {
					t.Fatal("prior-absent failure later promoted the candidate")
				}
				for _, artifact := range []string{
					rollbackRequiredPath(base.registryPath),
					filepath.Join(base.directory, "."+registryFileName+".commit-journal"),
					filepath.Join(base.directory, "."+registryFileName+".previous"),
					filepath.Join(base.directory, "."+registryFileName+".committed"),
				} {
					if _, err := os.Lstat(artifact); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("decision recovery retained artifact %s", filepath.Base(artifact))
					}
				}
			})
		}
	}
}

func TestStoreOrphanBackupRequiresProofBeforeCleanup(t *testing.T) {
	for _, targetEqual := range []bool{true, false} {
		name := "target-different"
		if targetEqual {
			name = "target-equal"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			store := mustNewStore(t, t.TempDir())
			var prior domainregistry.Registry
			if err := store.WithExclusive(ctx, func(transaction registryport.Transaction) error {
				loaded, err := transaction.Load(ctx)
				prior = loaded
				return err
			}); err != nil {
				t.Fatalf("initialize error = %v", err)
			}
			targetBytes, _ := domainregistry.Marshal(prior)
			backupBytes := append([]byte(nil), targetBytes...)
			if !targetEqual {
				backupState := prior.Clone()
				backupState.Revision++
				backupBytes, _ = domainregistry.Marshal(backupState)
			}
			_, backupPath, _ := recoveryPaths(store.registryPath)
			if err := createExclusivePrivateFile(backupPath, backupBytes); err != nil {
				t.Fatalf("create orphan backup error = %v", err)
			}
			err := store.WithExclusive(ctx, func(transaction registryport.Transaction) error {
				_, err := transaction.Load(ctx)
				return err
			})
			if targetEqual {
				if err != nil {
					t.Fatalf("Load(equal orphan backup) error = %v", err)
				}
				if _, statErr := os.Lstat(backupPath); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("equal redundant backup retained: %v", statErr)
				}
				return
			}
			if !errors.Is(err, registryport.ErrPersistence) {
				t.Fatalf("Load(different orphan backup) error = %v, want persistence failure", err)
			}
			afterTarget, targetErr := os.ReadFile(store.registryPath)
			afterBackup, backupErr := os.ReadFile(backupPath)
			if targetErr != nil || backupErr != nil || !bytes.Equal(afterTarget, targetBytes) || !bytes.Equal(afterBackup, backupBytes) {
				t.Fatal("ambiguous orphan backup recovery changed target or backup bytes")
			}
		})
	}
}

func TestStorePriorAbsentEvidenceRejectsUnexpectedBackup(t *testing.T) {
	for _, evidence := range []string{"journal", "rollback-required"} {
		t.Run(evidence, func(t *testing.T) {
			ctx := context.Background()
			store := mustNewStore(t, t.TempDir())
			if err := ensureRegistryDirectory(store.dataDir, store.directory); err != nil {
				t.Fatalf("ensureRegistryDirectory() error = %v", err)
			}
			candidate, _ := domainregistry.NewRegistry("inc_" + strings.Repeat("T", 43))
			candidateBytes, _ := domainregistry.Marshal(candidate)
			if err := os.WriteFile(store.registryPath, candidateBytes, 0o600); err != nil {
				t.Fatalf("WriteFile(candidate) error = %v", err)
			}
			journalPath, backupPath, _ := recoveryPaths(store.registryPath)
			backupBytes := []byte("synthetic-prior-bytes")
			if err := createExclusivePrivateFile(backupPath, backupBytes); err != nil {
				t.Fatalf("create unexpected backup error = %v", err)
			}
			if evidence == "journal" {
				if err := createExclusivePrivateFile(journalPath, journalPriorAbsent); err != nil {
					t.Fatalf("create journal error = %v", err)
				}
			} else if err := createExclusivePrivateFile(rollbackRequiredPath(store.registryPath), rollbackPriorAbsent); err != nil {
				t.Fatalf("create rollback evidence error = %v", err)
			}
			err := store.WithExclusive(ctx, func(transaction registryport.Transaction) error {
				_, err := transaction.Load(ctx)
				return err
			})
			if !errors.Is(err, registryport.ErrPersistence) {
				t.Fatalf("Load(prior-absent+backup) error = %v, want persistence failure", err)
			}
			afterTarget, targetErr := os.ReadFile(store.registryPath)
			afterBackup, backupErr := os.ReadFile(backupPath)
			if targetErr != nil || backupErr != nil || !bytes.Equal(afterTarget, candidateBytes) || !bytes.Equal(afterBackup, backupBytes) {
				t.Fatal("prior-absent ambiguity changed target or backup bytes")
			}
		})
	}
}

func TestStoreReturnedSuccessRemainsLoadableWhenCommittedCleanupReplays(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataDir := t.TempDir()
	store := mustNewStore(t, dataDir)
	var prior domainregistry.Registry
	if err := store.WithExclusive(ctx, func(transaction registryport.Transaction) error {
		loaded, err := transaction.Load(ctx)
		prior = loaded
		return err
	}); err != nil {
		t.Fatalf("initialize error = %v", err)
	}
	priorBytes, _ := domainregistry.Marshal(prior)
	candidate := prior.Clone()
	candidate.Revision++
	if err := store.WithExclusive(ctx, func(transaction registryport.Transaction) error {
		return transaction.Commit(ctx, candidate)
	}); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	candidateBytes, _ := domainregistry.Marshal(candidate)
	journalPath, backupPath, markerPath := recoveryPaths(store.registryPath)
	if err := createExclusivePrivateFile(backupPath, priorBytes); err != nil {
		t.Fatalf("create backup error = %v", err)
	}
	if err := createExclusivePrivateFile(journalPath, journalPriorPresent); err != nil {
		t.Fatalf("create journal error = %v", err)
	}
	if err := createExclusivePrivateFile(markerPath, commitMarker); err != nil {
		t.Fatalf("create marker error = %v", err)
	}
	for index, reader := range []*Store{store, mustNewStore(t, dataDir)} {
		if err := reader.WithExclusive(ctx, func(transaction registryport.Transaction) error {
			loaded, err := transaction.Load(ctx)
			if err != nil {
				return err
			}
			loadedBytes, _ := domainregistry.Marshal(loaded)
			if !bytes.Equal(loadedBytes, candidateBytes) {
				t.Fatalf("reader %d did not preserve the returned-success candidate", index)
			}
			return nil
		}); err != nil {
			t.Fatalf("reader %d Load() error = %v", index, err)
		}
	}
}

func TestStoreRollbackRequiredPrecedesVisibleCommitMarkerWithoutJournal(t *testing.T) {
	for _, priorPresent := range []bool{false, true} {
		name := "prior-absent"
		if priorPresent {
			name = "prior-present"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			dataDir := t.TempDir()
			store := mustNewStore(t, dataDir)
			if err := ensureRegistryDirectory(store.dataDir, store.directory); err != nil {
				t.Fatalf("ensureRegistryDirectory() error = %v", err)
			}
			var priorBytes []byte
			if priorPresent {
				if err := store.WithExclusive(ctx, func(transaction registryport.Transaction) error {
					_, err := transaction.Load(ctx)
					return err
				}); err != nil {
					t.Fatalf("initialize error = %v", err)
				}
				priorBytes, _ = os.ReadFile(store.registryPath)
			}
			candidate, _ := domainregistry.NewRegistry("inc_" + strings.Repeat("S", 43))
			candidateBytes, _ := domainregistry.Marshal(candidate)
			if err := os.WriteFile(store.registryPath, candidateBytes, 0o600); err != nil {
				t.Fatalf("WriteFile(candidate) error = %v", err)
			}
			if priorPresent {
				_, backupPath, _ := recoveryPaths(store.registryPath)
				if err := createExclusivePrivateFile(backupPath, priorBytes); err != nil {
					t.Fatalf("create backup error = %v", err)
				}
			}
			rollbackEvidence := rollbackPriorAbsent
			if priorPresent {
				rollbackEvidence = rollbackPriorPresent
			}
			if err := createExclusivePrivateFile(rollbackRequiredPath(store.registryPath), rollbackEvidence); err != nil {
				t.Fatalf("create rollback marker error = %v", err)
			}
			_, _, markerPath := recoveryPaths(store.registryPath)
			if err := createExclusivePrivateFile(markerPath, commitMarker); err != nil {
				t.Fatalf("create commit marker error = %v", err)
			}
			var loaded domainregistry.Registry
			if err := store.WithExclusive(ctx, func(transaction registryport.Transaction) error {
				state, err := transaction.Load(ctx)
				loaded = state
				return err
			}); err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			loadedBytes, _ := domainregistry.Marshal(loaded)
			if priorPresent {
				if !bytes.Equal(loadedBytes, priorBytes) {
					t.Fatal("visible commit marker overrode rollback-required prior bytes")
				}
			} else if loaded.Incarnation == candidate.Incarnation {
				t.Fatal("visible commit marker promoted a prior-absent candidate")
			}
		})
	}
}

func TestStoreRejectsConflictingRollbackEvidenceWithoutChangingTarget(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := mustNewStore(t, t.TempDir())
	var prior domainregistry.Registry
	if err := store.WithExclusive(ctx, func(transaction registryport.Transaction) error {
		loaded, err := transaction.Load(ctx)
		prior = loaded
		return err
	}); err != nil {
		t.Fatalf("initialize error = %v", err)
	}
	priorBytes, _ := domainregistry.Marshal(prior)
	candidate := prior.Clone()
	candidate.Revision++
	candidateBytes, _ := domainregistry.Marshal(candidate)
	if err := os.WriteFile(store.registryPath, candidateBytes, 0o600); err != nil {
		t.Fatalf("WriteFile(candidate) error = %v", err)
	}
	journalPath, backupPath, markerPath := recoveryPaths(store.registryPath)
	if err := createExclusivePrivateFile(backupPath, priorBytes); err != nil {
		t.Fatalf("create backup error = %v", err)
	}
	if err := createExclusivePrivateFile(journalPath, journalPriorPresent); err != nil {
		t.Fatalf("create journal error = %v", err)
	}
	if err := createExclusivePrivateFile(rollbackRequiredPath(store.registryPath), rollbackPriorAbsent); err != nil {
		t.Fatalf("create conflicting rollback marker error = %v", err)
	}
	if err := createExclusivePrivateFile(markerPath, commitMarker); err != nil {
		t.Fatalf("create commit marker error = %v", err)
	}
	err := store.WithExclusive(ctx, func(transaction registryport.Transaction) error {
		_, err := transaction.Load(ctx)
		return err
	})
	if !errors.Is(err, registryport.ErrPersistence) {
		t.Fatalf("Load(conflicting evidence) error = %v, want persistence failure", err)
	}
	after, err := os.ReadFile(store.registryPath)
	if err != nil || !bytes.Equal(after, candidateBytes) {
		t.Fatal("conflicting decision evidence changed target bytes")
	}
}

func TestStoreReadbackMismatchRollsBackWithoutLeakingPath(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	base, err := New(dataDir)
	if err != nil {
		t.Fatalf("New(base) error = %v", err)
	}
	ctx := context.Background()
	if err := base.WithExclusive(ctx, func(transaction registryport.Transaction) error {
		_, err := transaction.Load(ctx)
		return err
	}); err != nil {
		t.Fatalf("initialize error = %v", err)
	}
	before, err := os.ReadFile(base.registryPath)
	if err != nil {
		t.Fatalf("ReadFile(before) error = %v", err)
	}
	failing, err := newWithHooks(dataDir, &commitHooks{})
	if err != nil {
		t.Fatalf("newWithHooks() error = %v", err)
	}
	failing.hooks.afterReplaceDirectorySync = func() error {
		return os.WriteFile(failing.registryPath, []byte("synthetic-readback-mismatch"), 0o600)
	}
	err = failing.WithExclusive(ctx, func(transaction registryport.Transaction) error {
		state, err := transaction.Load(ctx)
		if err != nil {
			return err
		}
		state.Revision++
		return transaction.Commit(ctx, state)
	})
	if !errors.Is(err, registryport.ErrPersistence) || strings.Contains(err.Error(), dataDir) {
		t.Fatalf("Commit(readback mismatch) error = %v", err)
	}
	after, err := os.ReadFile(base.registryPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("readback mismatch did not restore exact prior bytes")
	}
}

func TestStoreRejectsMalformedUnknownVersionAndPhaseWithoutOverwrite(t *testing.T) {
	t.Parallel()

	for _, content := range []string{
		`{"version":2,"revision":0,"incarnation":"inc_` + strings.Repeat("a", 43) + `","providers":{},"transactions":{}}`,
		`{"version":1,"revision":0,"incarnation":"inc_` + strings.Repeat("a", 43) + `","providers":{},"transactions":{"txn_` + strings.Repeat("B", 43) + `":{"version":1,"id":"txn_` + strings.Repeat("B", 43) + `","operation":"connect","phase":"unknown","providerId":"provider-alpha","fence":{"registryRevision":0,"registryIncarnation":"inc_` + strings.Repeat("a", 43) + `"},"recovery":{"version":1,"attempts":0,"lastObserved":"unknown"}}}}`,
		`{"version":1`,
	} {
		dataDir := t.TempDir()
		store, err := New(dataDir)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		if err := ensureRegistryDirectory(store.dataDir, store.directory); err != nil {
			t.Fatalf("ensureRegistryDirectory() error = %v", err)
		}
		before := []byte(content)
		if err := os.WriteFile(store.registryPath, before, 0o600); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		err = store.WithExclusive(context.Background(), func(transaction registryport.Transaction) error {
			_, err := transaction.Load(context.Background())
			return err
		})
		if !errors.Is(err, registryport.ErrPersistence) {
			t.Fatalf("Load(malformed) error = %v, want persistence failure", err)
		}
		after, readErr := os.ReadFile(store.registryPath)
		if readErr != nil || !bytes.Equal(before, after) {
			t.Fatal("malformed Registry was overwritten")
		}
	}
}

func TestStoreRejectsCredentialBearingURLWithoutOverwritingCommittedBytes(t *testing.T) {
	t.Parallel()

	store := mustNewStore(t, t.TempDir())
	ctx := context.Background()
	if err := store.WithExclusive(ctx, func(transaction registryport.Transaction) error {
		state, err := transaction.Load(ctx)
		if err != nil {
			return err
		}
		provider := syntheticProvider()
		state.Providers[provider.ID] = provider
		state.SelectedProviderID = provider.ID
		state.Revision++
		return transaction.Commit(ctx, state)
	}); err != nil {
		t.Fatalf("initialize Provider error = %v", err)
	}
	before, err := os.ReadFile(store.registryPath)
	if err != nil {
		t.Fatalf("ReadFile(before) error = %v", err)
	}
	err = store.WithExclusive(ctx, func(transaction registryport.Transaction) error {
		state, err := transaction.Load(ctx)
		if err != nil {
			return err
		}
		provider := state.Providers["provider-alpha"]
		provider.Endpoint = "https://provider.invalid/v1?custom=synthetic-marker"
		state.Providers[provider.ID] = provider
		state.Revision++
		return transaction.Commit(ctx, state)
	})
	if !errors.Is(err, registryport.ErrInvalidRequest) {
		t.Fatalf("Commit(credential-bearing URL) error = %v, want invalid request", err)
	}
	after, err := os.ReadFile(store.registryPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("rejected credential-bearing URL overwrote prior Registry bytes")
	}
}

func syntheticProvider() domainregistry.Provider {
	return domainregistry.Provider{
		ID: "provider-alpha", Kind: "openai-compatible", Endpoint: "https://provider.invalid/v1",
		Models: []string{"model-alpha"}, MediaModels: []string{"media-alpha"},
		SelectedModel: "model-alpha", SelectedMedia: "media-alpha", SelectedRoutes: []string{"primary"},
		CredentialRef: "cred_" + strings.Repeat("B", 43), CredentialPurpose: "provider-api-key",
		Revision: 1, Generation: 1,
		Incarnation: "inc_" + strings.Repeat("b", 43),
	}
}

func mustNewStore(t *testing.T, dataDir string) *Store {
	t.Helper()
	store, err := New(dataDir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return store
}
