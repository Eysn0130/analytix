package electronlegacytask

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
)

func TestOpaqueLegacyElectronTaskRetirementNeverDecodesPayload(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, BackgroundTaskFileV1)
	unrelated := filepath.Join(root, "settings.json")
	body := []byte("not-json\nprivate reasoning and raw provider output\x00")
	writeTestFile(t, target, body)
	writeTestFile(t, unrelated, []byte("unrelated"))
	store := newTestStore(t, root)

	plan, err := store.Prepare(context.Background())
	if err != nil || !plan.Present() {
		t.Fatalf("prepare opaque payload: present=%v err=%v", plan != nil && plan.Present(), err)
	}
	if err := plan.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertCommittedRetirement(t, store, target)
	assertFileBody(t, unrelated, []byte("unrelated"))
}

func TestAbsentLegacyElectronOwnerCreatesNothing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing-user-data")
	store := newTestStore(t, root)
	observation, err := store.Observe(context.Background())
	if err != nil || observation.TargetPresent() || observation.JournalPresent() {
		t.Fatalf("absent observation = %#v, %v", observation, err)
	}
	plan, err := store.Prepare(context.Background())
	if err != nil || plan.Present() {
		t.Fatalf("absent prepare: present=%v err=%v", plan != nil && plan.Present(), err)
	}
	if err := plan.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("absent owner root was created: %v", err)
	}
}

func TestLegacyElectronStoreRequiresExactLeaseAuthority(t *testing.T) {
	root := t.TempDir()
	if _, err := NewStore(nil, root); err == nil {
		t.Fatal("store accepted missing lease authority")
	}
	store := newTestStore(t, root)
	other := filepath.Join(filepath.Dir(root), "other-owner")
	if _, err := NewStore(store.lease, other); err == nil {
		t.Fatal("store accepted an unregistered owner root")
	}
}

func TestLegacyElectronOwnerRejectsSymlinkAndHardlinkWithoutMutation(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		root := t.TempDir()
		external := filepath.Join(t.TempDir(), "external")
		writeTestFile(t, external, []byte("external"))
		target := filepath.Join(root, BackgroundTaskFileV1)
		if err := os.Symlink(external, target); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		store := newTestStore(t, root)
		if _, err := store.Prepare(context.Background()); err == nil {
			t.Fatal("symlink target was accepted")
		}
		assertFileBody(t, external, []byte("external"))
	})

	t.Run("hardlink", func(t *testing.T) {
		root := t.TempDir()
		external := filepath.Join(t.TempDir(), "external")
		writeTestFile(t, external, []byte("external"))
		target := filepath.Join(root, BackgroundTaskFileV1)
		if err := os.Link(external, target); err != nil {
			t.Skipf("hardlink unavailable: %v", err)
		}
		store := newTestStore(t, root)
		if _, err := store.Prepare(context.Background()); err == nil {
			t.Fatal("hard-linked target was accepted")
		}
		assertFileBody(t, external, []byte("external"))
	})
}

func TestLegacyElectronOwnerReplacementRacesFailClosed(t *testing.T) {
	t.Run("target before detach", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, BackgroundTaskFileV1)
		held := filepath.Join(root, "held-original")
		writeTestFile(t, target, []byte("original"))
		store := newTestStore(t, root)
		plan, err := store.Prepare(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		store.hooks.beforeDetach = func() error {
			if err := os.Rename(target, held); err != nil {
				return err
			}
			return os.WriteFile(target, []byte("replacement"), 0o600)
		}
		if err := plan.Apply(context.Background()); err == nil {
			t.Fatal("same-path target replacement was accepted")
		}
		assertFileBody(t, target, []byte("replacement"))
		assertFileBody(t, held, []byte("original"))
	})

	t.Run("quarantine before deletion", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, BackgroundTaskFileV1)
		writeTestFile(t, target, []byte("original"))
		store := newTestStore(t, root)
		plan, err := store.Prepare(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		journal := filepath.Join(root, journalDirectoryV1)
		payload := filepath.Join(journal, journalQuarantineFileV1)
		held := filepath.Join(journal, "held-original")
		store.hooks.beforeQuarantineRemove = func() error {
			if err := os.Rename(payload, held); err != nil {
				return err
			}
			return os.WriteFile(payload, []byte("replacement"), 0o600)
		}
		if err := plan.Apply(context.Background()); err == nil {
			t.Fatal("same-path quarantine replacement was accepted")
		}
		assertFileBody(t, payload, []byte("replacement"))
		assertFileBody(t, held, []byte("original"))
	})
}

func TestLegacyElectronOwnerCrashCutsRecoverMonotonically(t *testing.T) {
	stages := []struct {
		name string
		set  func(*storeHooks, error)
	}{
		{"after_journal_create", func(h *storeHooks, cut error) { h.afterJournalCreated = func() error { return cut } }},
		{"after_target_protected", func(h *storeHooks, cut error) { h.afterTargetProtected = func() error { return cut } }},
		{"plan_after_temp_sync", atomicCut("plan_after_temp_sync")},
		{"plan_after_publish", atomicCut("plan_after_publish")},
		{"plan_after_directory_sync", atomicCut("plan_after_directory_sync")},
		{"after_journal_prepare", func(h *storeHooks, cut error) { h.afterJournalPrepared = func() error { return cut } }},
		{"protection_after_temp_sync", atomicCut("protection_after_temp_sync")},
		{"protection_after_publish", atomicCut("protection_after_publish")},
		{"protection_after_directory_sync", atomicCut("protection_after_directory_sync")},
		{"after_protection_prepare", func(h *storeHooks, cut error) { h.afterProtectionPrepared = func() error { return cut } }},
		{"before_detach", func(h *storeHooks, cut error) { h.beforeDetach = func() error { return cut } }},
		{"after_detach", func(h *storeHooks, cut error) { h.afterDetach = func() error { return cut } }},
		{"before_quarantine_remove", func(h *storeHooks, cut error) { h.beforeQuarantineRemove = func() error { return cut } }},
		{"removal_intent_after_temp_sync", atomicCut("removal_intent_after_temp_sync")},
		{"removal_intent_after_publish", atomicCut("removal_intent_after_publish")},
		{"removal_intent_after_directory_sync", atomicCut("removal_intent_after_directory_sync")},
		{"after_removal_prepare", func(h *storeHooks, cut error) { h.afterRemovalPrepared = func() error { return cut } }},
		{"remove_after_unlink", func(h *storeHooks, cut error) {
			h.removeFileFault = func(stage string) error {
				if stage == "after_unlink" {
					return cut
				}
				return nil
			}
		}},
		{"after_quarantine_remove", func(h *storeHooks, cut error) { h.afterQuarantineRemoved = func() error { return cut } }},
		{"before_commit", func(h *storeHooks, cut error) { h.beforeCommit = func() error { return cut } }},
		{"commit_after_temp_sync", atomicCut("commit_after_temp_sync")},
		{"commit_after_publish", atomicCut("commit_after_publish")},
		{"commit_after_directory_sync", atomicCut("commit_after_directory_sync")},
		{"after_commit_prepare", func(h *storeHooks, cut error) { h.afterCommitPrepared = func() error { return cut } }},
	}
	for _, stage := range stages {
		t.Run(stage.name, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, BackgroundTaskFileV1)
			writeTestFile(t, target, []byte("opaque"))
			store := newTestStore(t, root)
			plan, err := store.Prepare(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			cutErr := errors.New("simulated crash cut")
			stage.set(&store.hooks, cutErr)
			if err := plan.Apply(context.Background()); !errors.Is(err, cutErr) {
				t.Fatalf("apply cut = %v", err)
			}
			restarted, err := NewStore(store.lease, root)
			if err != nil {
				t.Fatal(err)
			}
			settleRetirement(t, restarted)
			assertCommittedRetirement(t, restarted, target)
		})
	}
}

func TestLegacyElectronOwnerRecoveryReloadsInstallationAuthorityInNewProcess(t *testing.T) {
	managed := t.TempDir()
	dataRoot := filepath.Join(managed, "data")
	durableRoot := filepath.Join(managed, "durable")
	ownerRoot := filepath.Join(managed, "owner")
	for _, path := range []string{dataRoot, durableRoot, ownerRoot} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(ownerRoot, BackgroundTaskFileV1)
	writeTestFile(t, target, []byte("opaque"))

	run := func(mode string) {
		t.Helper()
		command := exec.Command(os.Args[0], "-test.run=^TestLegacyElectronOwnerProcessHelper$")
		command.Env = append(os.Environ(),
			"ANALYTIX_ELECTRON_RETIREMENT_HELPER="+mode,
			"ANALYTIX_ELECTRON_RETIREMENT_DATA="+dataRoot,
			"ANALYTIX_ELECTRON_RETIREMENT_DURABLE="+durableRoot,
			"ANALYTIX_ELECTRON_RETIREMENT_OWNER="+ownerRoot,
		)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("helper %s failed: %v\n%s", mode, err, output)
		}
	}

	run("cut_after_detach")
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("first process did not detach the exact target: %v", err)
	}
	assertFileBody(
		t,
		filepath.Join(ownerRoot, journalDirectoryV1, journalQuarantineFileV1),
		[]byte("opaque"),
	)
	run("recover")
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("restarted process restored or retained the target: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(ownerRoot, journalDirectoryV1, journalCommitFileV1)); err != nil {
		t.Fatalf("restarted process did not publish the signed commit: %v", err)
	}
}

func TestLegacyElectronOwnerProcessHelper(t *testing.T) {
	mode := os.Getenv("ANALYTIX_ELECTRON_RETIREMENT_HELPER")
	if mode == "" {
		return
	}
	dataRoot := os.Getenv("ANALYTIX_ELECTRON_RETIREMENT_DATA")
	durableRoot := os.Getenv("ANALYTIX_ELECTRON_RETIREMENT_DURABLE")
	ownerRoot := os.Getenv("ANALYTIX_ELECTRON_RETIREMENT_OWNER")
	roots, err := persistencefs.ResolveRootSet(dataRoot, durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := persistencefs.AcquireCompositeLeaseWithSeparateOwnerRoots(roots, ownerRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := lease.Close(); err != nil {
			t.Errorf("close helper lease: %v", err)
		}
	}()
	prepareTestJournalAuthority(t, lease, ownerRoot)
	store, err := NewStore(lease, ownerRoot)
	if err != nil {
		t.Fatal(err)
	}
	switch mode {
	case "cut_after_detach":
		plan, err := store.Prepare(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		store.hooks.afterDetach = func() error { return errors.New("process cut after detach") }
		if err := plan.Apply(context.Background()); err == nil {
			t.Fatal("helper did not stop after detach")
		}
	case "recover":
		observation, err := store.Observe(context.Background())
		if err != nil || !observation.RecoveryRequired() {
			t.Fatalf("restart observation = %#v, %v", observation, err)
		}
		if err := store.Recover(context.Background()); err != nil {
			t.Fatal(err)
		}
		observation, err = store.Observe(context.Background())
		if err != nil || !observation.Committed() || observation.RecoveryRequired() {
			t.Fatalf("restart completion = %#v, %v", observation, err)
		}
	default:
		t.Fatalf("unknown helper mode %q", mode)
	}
}

func TestTargetReappearanceAroundCommitNeverReturnsSuccess(t *testing.T) {
	for _, stage := range []struct {
		name string
		set  func(*storeHooks, string)
	}{
		{"before commit write", func(hooks *storeHooks, target string) {
			hooks.beforeCommit = func() error { return os.WriteFile(target, []byte("new task"), 0o600) }
		}},
		{"after commit write", func(hooks *storeHooks, target string) {
			hooks.afterCommitPrepared = func() error { return os.WriteFile(target, []byte("new task"), 0o600) }
		}},
	} {
		t.Run(stage.name, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, BackgroundTaskFileV1)
			writeTestFile(t, target, []byte("old task"))
			store := newTestStore(t, root)
			plan, err := store.Prepare(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			stage.set(&store.hooks, target)
			if err := plan.Apply(context.Background()); err == nil {
				t.Fatal("target reappearance around commit returned success")
			}
			assertFileBody(t, target, []byte("new task"))
			observation, observeErr := store.Observe(context.Background())
			if observeErr == nil && !observation.RecoveryRequired() {
				t.Fatal("target reappearance around commit produced a settled observation")
			}
		})
	}
}

func TestProtectionProofPrecedesTargetDetach(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, BackgroundTaskFileV1)
	writeTestFile(t, target, []byte("opaque"))
	store := newTestStore(t, root)
	plan, err := store.Prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cut := errors.New("cut after durable protection proof")
	store.hooks.afterProtectionPrepared = func() error { return cut }
	if err := plan.Apply(context.Background()); !errors.Is(err, cut) {
		t.Fatalf("apply cut = %v", err)
	}
	journal := filepath.Join(root, journalDirectoryV1)
	if _, err := os.Lstat(filepath.Join(journal, journalProtectionFileV1)); err != nil {
		t.Fatalf("protection proof was not durable before cut: %v", err)
	}
	assertFileBody(t, target, []byte("opaque"))
	if _, err := os.Lstat(filepath.Join(journal, journalQuarantineFileV1)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target detached before durable protection proof: %v", err)
	}
	restarted, err := NewStore(store.lease, root)
	if err != nil {
		t.Fatal(err)
	}
	settleRetirement(t, restarted)
	assertCommittedRetirement(t, restarted, target)
}

func TestUnsignedSelfConsistentPlanNeverAuthorizesTargetMutation(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, BackgroundTaskFileV1)
	writeTestFile(t, target, []byte("opaque"))
	store := newTestStore(t, root)
	plan, err := store.Prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cut := errors.New("cut after durable plan")
	store.hooks.afterJournalPrepared = func() error { return cut }
	if err := plan.Apply(context.Background()); !errors.Is(err, cut) {
		t.Fatalf("apply cut = %v", err)
	}
	planPath := filepath.Join(root, journalDirectoryV1, journalPlanFileV1)
	body, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	record, err := decodeStrict[journalPlanV1](body, maxJournalBytesV1)
	clear(body)
	if err != nil {
		t.Fatal(err)
	}
	record.AuthoritySignature = ""
	forged, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(planPath, forged, 0o600); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewStore(store.lease, root)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := restarted.Observe(context.Background())
	if err != nil || !observation.RecoveryRequired() || observation.Committed() {
		t.Fatalf("unsigned plan observation = %#v, %v", observation, err)
	}
	if err := restarted.Recover(context.Background()); err != nil {
		t.Fatalf("safe unauthenticated cleanup failed: %v", err)
	}
	assertFileBody(t, target, []byte("opaque"))
	if _, err := os.Lstat(filepath.Join(root, journalDirectoryV1)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsigned journal remained or authorized progress: %v", err)
	}
}

func TestPlanSignatureCannotAuthorizeProtectionStage(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, BackgroundTaskFileV1)
	writeTestFile(t, target, []byte("opaque"))
	store := newTestStore(t, root)
	plan, err := store.Prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cut := errors.New("cut after durable protection proof")
	store.hooks.afterProtectionPrepared = func() error { return cut }
	if err := plan.Apply(context.Background()); !errors.Is(err, cut) {
		t.Fatalf("apply cut = %v", err)
	}
	journal := filepath.Join(root, journalDirectoryV1)
	planBody, err := os.ReadFile(filepath.Join(journal, journalPlanFileV1))
	if err != nil {
		t.Fatal(err)
	}
	planRecord, err := decodeStrict[journalPlanV1](planBody, maxJournalBytesV1)
	clear(planBody)
	if err != nil {
		t.Fatal(err)
	}
	proofPath := filepath.Join(journal, journalProtectionFileV1)
	proofBody, err := os.ReadFile(proofPath)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := decodeStrict[journalProtectionV1](proofBody, maxJournalBytesV1)
	clear(proofBody)
	if err != nil {
		t.Fatal(err)
	}
	proof.AuthoritySignature = planRecord.AuthoritySignature
	forged, err := json.Marshal(proof)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(proofPath, forged, 0o600); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewStore(store.lease, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Recover(context.Background()); err == nil {
		t.Fatal("plan-domain signature authorized a protection proof")
	}
	assertFileBody(t, target, []byte("opaque"))
	if _, err := os.Lstat(filepath.Join(journal, journalQuarantineFileV1)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target detached after cross-domain signature replay: %v", err)
	}
}

func TestForgedProtectionProofAndProtectedStateDriftFailClosed(t *testing.T) {
	t.Run("proof target mismatch", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, BackgroundTaskFileV1)
		writeTestFile(t, target, []byte("opaque"))
		store := newTestStore(t, root)
		plan, err := store.Prepare(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		cut := errors.New("cut after durable protection proof")
		store.hooks.afterProtectionPrepared = func() error { return cut }
		if err := plan.Apply(context.Background()); !errors.Is(err, cut) {
			t.Fatalf("apply cut = %v", err)
		}
		proofPath := filepath.Join(root, journalDirectoryV1, journalProtectionFileV1)
		body, err := os.ReadFile(proofPath)
		if err != nil {
			t.Fatal(err)
		}
		proof, err := decodeStrict[journalProtectionV1](body, maxJournalBytesV1)
		clear(body)
		if err != nil {
			t.Fatal(err)
		}
		proof.ProtectedTarget.SecurityDigest = sha256Hex([]byte("forged protected state"))
		proof.ProofDigest = digestJournalProtection(proof)
		forged, err := json.Marshal(proof)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(proofPath, forged, 0o600); err != nil {
			t.Fatal(err)
		}
		restarted, err := NewStore(store.lease, root)
		if err != nil {
			t.Fatal(err)
		}
		if err := restarted.Recover(context.Background()); err == nil {
			t.Fatal("self-consistent protection proof for the wrong state was accepted")
		}
		assertFileBody(t, target, []byte("opaque"))
		if _, err := os.Lstat(filepath.Join(root, journalDirectoryV1, journalQuarantineFileV1)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("target detached after forged protection proof: %v", err)
		}
	})

	t.Run("target security drift", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, BackgroundTaskFileV1)
		writeTestFile(t, target, []byte("opaque"))
		store := newTestStore(t, root)
		plan, err := store.Prepare(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		cut := errors.New("cut after durable protection proof")
		store.hooks.afterProtectionPrepared = func() error { return cut }
		if err := plan.Apply(context.Background()); !errors.Is(err, cut) {
			t.Fatalf("apply cut = %v", err)
		}
		if err := os.Chmod(target, 0o640); err != nil {
			t.Fatal(err)
		}
		restarted, err := NewStore(store.lease, root)
		if err != nil {
			t.Fatal(err)
		}
		if err := restarted.Recover(context.Background()); err == nil {
			t.Fatal("target security drift after protection proof was accepted")
		}
		assertFileBody(t, target, []byte("opaque"))
		if _, err := os.Lstat(filepath.Join(root, journalDirectoryV1, journalQuarantineFileV1)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("drifted target entered quarantine: %v", err)
		}
	})
}

func TestRemovalIntentPrecedesPayloadDeletion(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, BackgroundTaskFileV1)
	writeTestFile(t, target, []byte("opaque"))
	store := newTestStore(t, root)
	plan, err := store.Prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cut := errors.New("cut after durable removal intent")
	store.hooks.afterRemovalPrepared = func() error { return cut }
	if err := plan.Apply(context.Background()); !errors.Is(err, cut) {
		t.Fatalf("apply cut = %v", err)
	}
	journal := filepath.Join(root, journalDirectoryV1)
	if _, err := os.Lstat(filepath.Join(journal, journalRemovalIntentFileV1)); err != nil {
		t.Fatalf("removal intent was not durable before cut: %v", err)
	}
	assertFileBody(t, filepath.Join(journal, journalQuarantineFileV1), []byte("opaque"))
	restarted, err := NewStore(store.lease, root)
	if err != nil {
		t.Fatal(err)
	}
	settleRetirement(t, restarted)
	assertCommittedRetirement(t, restarted, target)
}

func TestEmptyAndPartialJournalResidueRecoversDeterministically(t *testing.T) {
	t.Run("empty journal", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, BackgroundTaskFileV1)
		writeTestFile(t, target, []byte("opaque"))
		if err := os.Mkdir(filepath.Join(root, journalDirectoryV1), 0o700); err != nil {
			t.Fatal(err)
		}
		store := newTestStore(t, root)
		settleRetirement(t, store)
		assertCommittedRetirement(t, store, target)
	})

	t.Run("partial plan", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, BackgroundTaskFileV1)
		writeTestFile(t, target, []byte("opaque"))
		journal := filepath.Join(root, journalDirectoryV1)
		if err := os.Mkdir(journal, 0o700); err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(journal, journalPlanFileV1), []byte("{"))
		store := newTestStore(t, root)
		settleRetirement(t, store)
		assertCommittedRetirement(t, store, target)
	})

	t.Run("invalid published commit fails closed", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, BackgroundTaskFileV1)
		writeTestFile(t, target, []byte("opaque"))
		store := newTestStore(t, root)
		plan, err := store.Prepare(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		cut := errors.New("cut before commit")
		store.hooks.beforeCommit = func() error { return cut }
		if err := plan.Apply(context.Background()); !errors.Is(err, cut) {
			t.Fatalf("apply cut = %v", err)
		}
		writeTestFile(t, filepath.Join(root, journalDirectoryV1, journalCommitFileV1), []byte("{"))
		restarted, err := NewStore(store.lease, root)
		if err != nil {
			t.Fatal(err)
		}
		if err := restarted.Recover(context.Background()); err == nil {
			t.Fatal("invalid published commit was repaired instead of rejected")
		}
		assertFileBody(t, filepath.Join(root, journalDirectoryV1, journalCommitFileV1), []byte("{"))
		if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("target unexpectedly reappeared after invalid commit: %v", err)
		}
	})
}

func TestUnauthenticatedJournalPayloadNeverBecomesBackgroundTask(t *testing.T) {
	for _, planBody := range [][]byte{nil, []byte("{")} {
		name := "missing_plan"
		if planBody != nil {
			name = "invalid_plan"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			journal := filepath.Join(root, journalDirectoryV1)
			if err := os.Mkdir(journal, 0o700); err != nil {
				t.Fatal(err)
			}
			payload := filepath.Join(journal, journalQuarantineFileV1)
			writeTestFile(t, payload, []byte("attacker-controlled task bytes"))
			if planBody != nil {
				writeTestFile(t, filepath.Join(journal, journalPlanFileV1), planBody)
			}

			store := newTestStore(t, root)
			observation, err := store.Observe(context.Background())
			if err != nil || !observation.RecoveryRequired() {
				t.Fatalf("unauthenticated observation = %#v, %v", observation, err)
			}
			if err := store.Recover(context.Background()); err == nil {
				t.Fatal("unauthenticated journal payload was accepted")
			}
			if _, err := os.Lstat(filepath.Join(root, BackgroundTaskFileV1)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unauthenticated payload became a background task: %v", err)
			}
			assertFileBody(t, payload, []byte("attacker-controlled task bytes"))
		})
	}
}

func TestMissingRemovalIntentCannotCommitAliasedPayload(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, BackgroundTaskFileV1)
	writeTestFile(t, target, []byte("opaque"))
	store := newTestStore(t, root)
	plan, err := store.Prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cut := errors.New("cut after detach")
	store.hooks.afterDetach = func() error { return cut }
	if err := plan.Apply(context.Background()); !errors.Is(err, cut) {
		t.Fatalf("apply cut = %v", err)
	}
	payload := filepath.Join(root, journalDirectoryV1, journalQuarantineFileV1)
	alias := filepath.Join(root, "late-payload-alias")
	if err := os.Link(payload, alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(payload); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewStore(store.lease, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Recover(context.Background()); err == nil {
		t.Fatal("payload absence without a host removal intent was committed")
	}
	if _, err := os.Lstat(filepath.Join(root, journalDirectoryV1, journalCommitFileV1)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("commit appeared without removal intent: %v", err)
	}
	assertFileBody(t, alias, []byte("opaque"))
}

func TestRootAndAncestorReplacementInvalidateFrozenAuthority(t *testing.T) {
	for _, test := range []struct {
		name string
		swap func(root string) error
	}{
		{"root", func(root string) error {
			if err := os.Rename(root, root+"-held"); err != nil {
				return err
			}
			return os.Mkdir(root, 0o700)
		}},
		{"ancestor", func(root string) error {
			parent := filepath.Dir(root)
			if err := os.Rename(parent, parent+"-held"); err != nil {
				return err
			}
			return os.MkdirAll(root, 0o700)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			parent := filepath.Join(t.TempDir(), "parent")
			root := filepath.Join(parent, "owner")
			if err := os.MkdirAll(root, 0o700); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(root, BackgroundTaskFileV1)
			writeTestFile(t, target, []byte("opaque"))
			store := newTestStore(t, root)
			plan, err := store.Prepare(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if err := test.swap(root); err != nil {
				t.Skipf("directory replacement unavailable while pinned: %v", err)
			}
			if err := plan.Validate(context.Background()); err == nil {
				t.Fatal("replaced root authority remained live")
			}
			if _, err := os.Lstat(filepath.Join(root, journalDirectoryV1)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("replacement root was mutated: %v", err)
			}
		})
	}
}

func TestCommittedLegacyElectronOwnerRejectsTargetReappearance(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, BackgroundTaskFileV1)
	writeTestFile(t, target, []byte("old"))
	store := newTestStore(t, root)
	plan, err := store.Prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, target, []byte("new"))
	if _, err := store.Observe(context.Background()); err == nil {
		t.Fatal("target reappearance after commit was accepted")
	}
	assertFileBody(t, target, []byte("new"))
}

func TestCommittedJournalCannotReplayUnderAnotherOwner(t *testing.T) {
	sourceRoot := t.TempDir()
	sourceTarget := filepath.Join(sourceRoot, BackgroundTaskFileV1)
	writeTestFile(t, sourceTarget, []byte("opaque"))
	sourceStore := newTestStore(t, sourceRoot)
	plan, err := sourceStore.Prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}

	destinationRoot := t.TempDir()
	sourceJournal := filepath.Join(sourceRoot, journalDirectoryV1)
	destinationJournal := filepath.Join(destinationRoot, journalDirectoryV1)
	if err := os.Mkdir(destinationJournal, 0o700); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(sourceJournal)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		body, err := os.ReadFile(filepath.Join(sourceJournal, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(destinationJournal, entry.Name()), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	destinationStore := newTestStore(t, destinationRoot)
	if _, err := destinationStore.Observe(context.Background()); err == nil {
		t.Fatal("self-consistent committed journal replayed under another owner authority")
	}
}

func TestUnknownOwnerJournalObjectBlocksBeforeTargetMutation(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, BackgroundTaskFileV1)
	writeTestFile(t, target, []byte("old"))
	journal := filepath.Join(root, journalDirectoryV1)
	if err := os.Mkdir(journal, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(journal, "unknown"), []byte("x"))
	store := newTestStore(t, root)
	if _, err := store.Observe(context.Background()); err == nil {
		t.Fatal("unknown journal object was accepted")
	}
	assertFileBody(t, target, []byte("old"))
}

func TestAuthenticatedRecoverySemanticFailureCreatesNoFurtherMutation(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, BackgroundTaskFileV1)
	held := filepath.Join(root, "held-original")
	writeTestFile(t, target, []byte("old"))
	store := newTestStore(t, root)
	plan, err := store.Prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cut := errors.New("stop after durable plan")
	store.hooks.afterJournalPrepared = func() error { return cut }
	if err := plan.Apply(context.Background()); !errors.Is(err, cut) {
		t.Fatalf("apply cut = %v", err)
	}
	if err := os.Rename(target, held); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, target, []byte("replacement"))
	journal := filepath.Join(root, journalDirectoryV1)
	invalidCommit := filepath.Join(journal, journalCommitFileV1)
	temp := filepath.Join(journal, ".committed.json-0123456789abcdef0123456789abcdef.tmp")
	writeTestFile(t, invalidCommit, []byte("{"))
	writeTestFile(t, temp, []byte("partial"))

	restarted, err := NewStore(store.lease, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Recover(context.Background()); err == nil {
		t.Fatal("mismatched authenticated recovery was accepted")
	}
	assertFileBody(t, target, []byte("replacement"))
	assertFileBody(t, held, []byte("old"))
	assertFileBody(t, invalidCommit, []byte("{"))
	assertFileBody(t, temp, []byte("partial"))
}

func atomicCut(stage string) func(*storeHooks, error) {
	return func(hooks *storeHooks, cut error) {
		hooks.atomicWriteFault = func(current string) error {
			if current == stage {
				return cut
			}
			return nil
		}
	}
}

func newTestStore(t *testing.T, root string) *Store {
	t.Helper()
	managed := t.TempDir()
	data := filepath.Join(managed, "data")
	durable := filepath.Join(managed, "durable")
	for _, path := range []string{data, durable} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	roots, err := persistencefs.ResolveRootSet(data, durable)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(roots); err != nil {
		t.Fatal(err)
	}
	lease, err := persistencefs.AcquireCompositeLeaseWithSeparateOwnerRoots(roots, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := lease.Close(); err != nil {
			t.Errorf("close lease: %v", err)
		}
	})
	if _, present, err := lease.OpenExistingJournalAuthorityV1(); err != nil || !present {
		t.Fatalf("bind test journal authority: present=%v, err=%v", present, err)
	}
	store, err := NewStore(lease, root)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

type electronStoreJournalPreflightOwnerV2 struct {
	lease       *persistencefs.CompositeLease
	root        string
	observation PreAuthorityObservationV2
}

func (owner *electronStoreJournalPreflightOwnerV2) ObserveJournalAuthorityPreflightV1(
	ctx context.Context,
) (string, error) {
	observation, err := ObserveBeforeJournalAuthorityV2(ctx, owner.lease, owner.root)
	if err != nil {
		return "", err
	}
	owner.observation = observation
	return observation.Digest(), nil
}

func (owner *electronStoreJournalPreflightOwnerV2) ValidateJournalAuthorityPreflightV1(
	ctx context.Context,
	expected string,
) error {
	if expected == "" || expected != owner.observation.Digest() {
		return errors.New("test Electron journal-authority preflight changed")
	}
	return ValidateBeforeJournalAuthorityV2(ctx, owner.lease, owner.root, owner.observation)
}

func prepareTestJournalAuthority(
	t *testing.T,
	lease *persistencefs.CompositeLease,
	root string,
) {
	t.Helper()
	owner := &electronStoreJournalPreflightOwnerV2{lease: lease, root: root}
	prepared, err := persistencefs.PrepareJournalAuthorityBootstrapV1(
		context.Background(), lease, owner,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, present, err := prepared.BindExistingV1(context.Background()); err != nil {
		t.Fatal(err)
	} else if !present {
		if owner.observation.JournalPresent() {
			t.Fatal("test Electron journal has no existing installation authority")
		}
		if _, err := prepared.CreateV1(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}

func settleRetirement(t *testing.T, store *Store) {
	t.Helper()
	observation, err := store.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if observation.RecoveryRequired() {
		if err := store.Recover(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	observation, err = store.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if observation.Committed() {
		return
	}
	plan, err := store.Prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func assertCommittedRetirement(t *testing.T, store *Store, target string) {
	t.Helper()
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retired target remains: %v", err)
	}
	journal := filepath.Join(store.Root(), journalDirectoryV1)
	if _, err := os.Lstat(filepath.Join(journal, journalCommitFileV1)); err != nil {
		t.Fatalf("durable commit is missing: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(journal, journalRemovalIntentFileV1)); err != nil {
		t.Fatalf("durable removal intent is missing: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(journal, journalProtectionFileV1)); err != nil {
		t.Fatalf("durable protection proof is missing: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(journal, journalQuarantineFileV1)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("quarantine payload remains: %v", err)
	}
	observation, err := store.Observe(context.Background())
	if err != nil || !observation.Committed() || observation.TargetPresent() || observation.RecoveryRequired() {
		t.Fatalf("committed observation = %#v, %v", observation, err)
	}
}

func writeTestFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertFileBody(t *testing.T, path string, expected []byte) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(body, expected) {
		t.Fatalf("file %s = %q, %v", path, body, err)
	}
}
