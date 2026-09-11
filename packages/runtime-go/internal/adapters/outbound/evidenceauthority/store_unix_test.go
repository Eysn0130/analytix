//go:build darwin || linux

package evidenceauthority

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestStoresRejectSymlinkHardlinkAndUnsafePermissions(t *testing.T) {
	t.Run("bundle-symlink", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "bundles")
		store, err := newTestBundleStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		bundle := newEvidenceAuthorityStoreFixture(91).firstBundle(t, "bundle-symlink")
		if err := store.PutIfAbsent(context.Background(), bundle); err != nil {
			t.Fatal(err)
		}
		path := rawEvidenceCASRecordPath(root, bundle.RecordDigest)
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		external := filepath.Join(t.TempDir(), "external.json")
		if err := os.WriteFile(external, canonicalEvidenceBundleBody(t, bundle), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(external, path); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Resolve(context.Background(), bundle.RecordDigest); err == nil {
			t.Fatal("bundle CAS followed a symlink")
		}
	})

	t.Run("observation-hardlink", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "observations")
		store, err := newTestObservationStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		fixture := newEvidenceAuthorityStoreFixture(92)
		bundle := fixture.observationBundle(t, fixture.firstBundle(t, "observation-hardlink"), "observation-hardlink")
		if err := store.PutIfAbsent(context.Background(), bundle); err != nil {
			t.Fatal(err)
		}
		path := rawEvidenceCASRecordPath(root, bundle.Observation.ObservationDigest)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		external := filepath.Join(t.TempDir(), "external.json")
		if err := os.WriteFile(external, body, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(external, path); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Resolve(context.Background(), bundle.Observation.ObservationDigest); err == nil {
			t.Fatal("observation CAS accepted a multi-link record")
		}
	})

	t.Run("root-permissions", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "bundles")
		store, err := newTestBundleStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		bundle := newEvidenceAuthorityStoreFixture(93).firstBundle(t, "unsafe-root-mode")
		if err := store.PutIfAbsent(context.Background(), bundle); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(root, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
		if _, err := store.Resolve(context.Background(), bundle.RecordDigest); err == nil {
			t.Fatal("bundle store accepted unsafe root permissions")
		}
	})

	t.Run("record-permissions", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "observations")
		store, err := newTestObservationStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		fixture := newEvidenceAuthorityStoreFixture(94)
		bundle := fixture.observationBundle(t, fixture.firstBundle(t, "unsafe-record-mode"), "unsafe-record-mode")
		if err := store.PutIfAbsent(context.Background(), bundle); err != nil {
			t.Fatal(err)
		}
		path := rawEvidenceCASRecordPath(root, bundle.Observation.ObservationDigest)
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Resolve(context.Background(), bundle.Observation.ObservationDigest); err == nil {
			t.Fatal("observation store accepted unsafe record permissions")
		}
	})

	t.Run("root-symlink", func(t *testing.T) {
		realRoot := filepath.Join(t.TempDir(), "real")
		if err := os.Mkdir(realRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(realRoot, link); err != nil {
			t.Fatal(err)
		}
		if _, err := newTestBundleStore(t, link); err == nil {
			t.Fatal("bundle store accepted a symlink root")
		}
		if _, err := newTestObservationStore(t, link); err == nil {
			t.Fatal("observation store accepted a symlink root")
		}
		if _, err := NewProjection(link); err == nil {
			t.Fatal("projection accepted a symlink root")
		}
	})

	t.Run("unknown-inventory", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "bundles")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "attacker"), []byte("attacker"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := newTestBundleStore(t, root); err == nil {
			t.Fatal("bundle store accepted unknown inventory")
		}
	})
}

func TestStoresRejectRecordTruncationAfterOpen(t *testing.T) {
	t.Run("bundle", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "bundles")
		store, err := newTestBundleStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		bundle := newEvidenceAuthorityStoreFixture(95).firstBundle(t, "truncate-bundle")
		if err := store.PutIfAbsent(context.Background(), bundle); err != nil {
			t.Fatal(err)
		}
		body := canonicalEvidenceBundleBody(t, bundle)
		replaceEvidenceFile(t, rawEvidenceCASRecordPath(root, bundle.RecordDigest), body[:len(body)/2])
		if _, err := store.Resolve(context.Background(), bundle.RecordDigest); err == nil {
			t.Fatal("bundle store accepted a record truncated after startup")
		}
	})

	t.Run("observation", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "observations")
		store, err := newTestObservationStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		fixture := newEvidenceAuthorityStoreFixture(96)
		bundle := fixture.observationBundle(t, fixture.firstBundle(t, "truncate-observation"), "truncate-observation")
		if err := store.PutIfAbsent(context.Background(), bundle); err != nil {
			t.Fatal(err)
		}
		body, err := evidenceObservationBytes(bundle)
		if err != nil {
			t.Fatal(err)
		}
		replaceEvidenceFile(t, rawEvidenceCASRecordPath(root, bundle.Observation.ObservationDigest), body[:len(body)/2])
		if _, err := store.Resolve(context.Background(), bundle.Observation.ObservationDigest); err == nil {
			t.Fatal("observation store accepted a record truncated after startup")
		}
	})
}

func TestProjectionRealExactRestartIdempotentAdvanceSkipAndResidueReconcile(t *testing.T) {
	fixture := newEvidenceAuthorityStoreFixture(97)
	first := fixture.firstBundle(t, "real-projection")
	second := fixture.nextBundle(t, first, "dataset", "real-projection-2")
	third := fixture.nextBundle(t, second, "registry", "real-projection-3")
	fourth := fixture.nextBundle(t, third, "publication", "real-projection-4")
	fifth := fixture.nextBundle(t, fourth, "dataset", "real-projection-5")
	parent := t.TempDir()
	root := filepath.Join(parent, "projection")
	bundleStore, err := newTestBundleStore(t, filepath.Join(parent, "bundles"))
	if err != nil {
		t.Fatal(err)
	}
	for _, bundle := range []domainevidence.EvidenceAuthorityBundleV1{first, second, third, fourth, fifth} {
		if err := bundleStore.PutIfAbsent(context.Background(), bundle); err != nil {
			t.Fatal(err)
		}
	}
	projection, err := NewProjection(root, bundleStore)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.ProjectWitnessSelected(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := projection.ProjectWitnessSelected(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if err := projection.ProjectWitnessSelected(context.Background(), fourth); err != nil {
		t.Fatalf("witness-selected generation skip failed: %v", err)
	}
	if err := projection.ProjectWitnessSelected(context.Background(), fourth); err != nil {
		t.Fatalf("idempotent projection failed: %v", err)
	}
	restarted, err := NewProjection(root, bundleStore)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.ProjectWitnessSelected(context.Background(), fourth); err != nil {
		t.Fatalf("restarted projection did not verify exact current bytes: %v", err)
	}
	fourthBody := canonicalEvidenceBundleBody(t, fourth)
	storedFourth, err := os.ReadFile(filepath.Join(root, evidenceWitnessProjectionName))
	if err != nil || !bytes.Equal(storedFourth, fourthBody) {
		t.Fatalf("projection did not preserve exact canonical bytes: err=%v", err)
	}
	if domainsecurity.SHA256Hex(storedFourth) == fourth.RecordDigest {
		t.Fatal("projection body digest was conflated with bundle RecordDigest")
	}

	// Only the independently selected fifth bundle's complete canonical-body
	// SHA is supplied to reconciliation; no local generation scan chooses it.
	residue := filepath.Join(root, "."+evidenceWitnessProjectionName+"-00112233445566778899aabb.tmp")
	if err := os.WriteFile(residue, canonicalEvidenceBundleBody(t, fifth), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restarted.ProjectWitnessSelected(context.Background(), fifth); err != nil {
		t.Fatalf("exact witness-selected residue reconciliation failed: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != evidenceWitnessProjectionName {
		t.Fatalf("projection residue was not deterministically reconciled: entries=%v err=%v", entries, err)
	}

	path := filepath.Join(root, evidenceWitnessProjectionName)
	replaceEvidenceFile(t, path, []byte(`{"generation":999,"current":true}`))
	err = restarted.ProjectWitnessSelected(context.Background(), fifth)
	if !errors.Is(err, ErrProjectionConflict) {
		t.Fatalf("attacker target did not fail closed: %v", err)
	}
	body, readErr := os.ReadFile(path)
	if readErr != nil || string(body) != `{"generation":999,"current":true}` {
		t.Fatal("attacker target was silently replaced")
	}
}

func TestProjectionRealRejectsRollbackAndSameGenerationForkWithoutMutation(t *testing.T) {
	fixture := newEvidenceAuthorityStoreFixture(98)
	first := fixture.firstBundle(t, "real-conflict")
	second := fixture.nextBundle(t, first, "dataset", "real-conflict-2")
	fork := fixture.nextBundle(t, first, "registry", "real-conflict-fork")
	root := filepath.Join(t.TempDir(), "projection")
	projection, err := NewProjection(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.ProjectWitnessSelected(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := projection.ProjectWitnessSelected(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	want := canonicalEvidenceBundleBody(t, second)
	if err := projection.ProjectWitnessSelected(context.Background(), first); !errors.Is(err, ErrProjectionConflict) {
		t.Fatalf("rollback returned %v", err)
	}
	if err := projection.ProjectWitnessSelected(context.Background(), fork); !errors.Is(err, ErrProjectionConflict) {
		t.Fatalf("same-generation fork returned %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, evidenceWitnessProjectionName))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("conflict mutated projection: err=%v", err)
	}
}

func TestProjectionRejectsSymlinkHardlinkPermissionsAndUnknownResidue(t *testing.T) {
	fixture := newEvidenceAuthorityStoreFixture(101)
	first := fixture.firstBundle(t, "projection-files")
	second := fixture.nextBundle(t, first, "dataset", "projection-files-2")

	t.Run("symlink", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "projection")
		projection, err := NewProjection(root)
		if err != nil {
			t.Fatal(err)
		}
		if err := projection.ProjectWitnessSelected(context.Background(), first); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, evidenceWitnessProjectionName)
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		external := filepath.Join(t.TempDir(), "external.json")
		if err := os.WriteFile(external, canonicalEvidenceBundleBody(t, first), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(external, path); err != nil {
			t.Fatal(err)
		}
		if err := projection.ProjectWitnessSelected(context.Background(), second); err == nil {
			t.Fatal("projection followed a symlink target")
		}
	})

	t.Run("hardlink", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "projection")
		projection, err := NewProjection(root)
		if err != nil {
			t.Fatal(err)
		}
		if err := projection.ProjectWitnessSelected(context.Background(), first); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, evidenceWitnessProjectionName)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		external := filepath.Join(t.TempDir(), "external.json")
		if err := os.WriteFile(external, body, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(external, path); err != nil {
			t.Fatal(err)
		}
		if err := projection.ProjectWitnessSelected(context.Background(), second); err == nil {
			t.Fatal("projection accepted a multi-link target")
		}
	})

	t.Run("permissions", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "projection")
		projection, err := NewProjection(root)
		if err != nil {
			t.Fatal(err)
		}
		if err := projection.ProjectWitnessSelected(context.Background(), first); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(root, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
		if err := projection.ProjectWitnessSelected(context.Background(), second); err == nil {
			t.Fatal("projection accepted unsafe root permissions")
		}
	})

	t.Run("unknown-residue", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "projection")
		projection, err := NewProjection(root)
		if err != nil {
			t.Fatal(err)
		}
		if err := projection.ProjectWitnessSelected(context.Background(), first); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "attacker.tmp"), []byte("attacker"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := projection.ProjectWitnessSelected(context.Background(), second); err == nil {
			t.Fatal("projection accepted an unknown residue name")
		}
	})

	t.Run("unselected-valid-residue", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "projection")
		projection, err := NewProjection(root)
		if err != nil {
			t.Fatal(err)
		}
		if err := projection.ProjectWitnessSelected(context.Background(), first); err != nil {
			t.Fatal(err)
		}
		residue := filepath.Join(root, "."+evidenceWitnessProjectionName+"-ffeeddccbbaa998877665544.tmp")
		if err := os.WriteFile(residue, []byte("attacker"), 0o600); err != nil {
			t.Fatal(err)
		}
		err = projection.ProjectWitnessSelected(context.Background(), second)
		if !errors.Is(err, ErrProjectionIndeterminate) {
			t.Fatalf("unselected residue did not quarantine projection: %v", err)
		}
		if _, statErr := os.Stat(residue); statErr != nil {
			t.Fatal("unselected residue was silently cleaned")
		}
	})
}
