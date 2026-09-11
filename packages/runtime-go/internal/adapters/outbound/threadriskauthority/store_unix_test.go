//go:build darwin || linux

package threadriskauthority

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStoresRejectSymlinkHardlinkAndUnsafePermissions(t *testing.T) {
	t.Run("index-symlink", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "indexes")
		store, err := newTestRiskIndexStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		index := newAuthorityStoreFixture(91).firstIndex(t, "index-symlink")
		if err := store.PutIfAbsent(context.Background(), index); err != nil {
			t.Fatal(err)
		}
		path := rawCASRecordPath(root, index.IndexDigest)
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		external := filepath.Join(t.TempDir(), "external.json")
		if err := os.WriteFile(external, canonicalIndexBody(t, index), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(external, path); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Resolve(context.Background(), index.IndexDigest); err == nil {
			t.Fatal("index CAS followed a symlink")
		}
	})

	t.Run("observation-hardlink", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "observations")
		store, err := newTestRiskObservationStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		fixture := newAuthorityStoreFixture(92)
		bundle := fixture.bundle(t, fixture.firstIndex(t, "observation-hardlink"), "observation-hardlink")
		if err := store.PutIfAbsent(context.Background(), bundle); err != nil {
			t.Fatal(err)
		}
		path := rawCASRecordPath(root, bundle.Observation.ObservationDigest)
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
		root := filepath.Join(t.TempDir(), "indexes")
		store, err := newTestRiskIndexStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		index := newAuthorityStoreFixture(93).firstIndex(t, "unsafe-mode")
		if err := store.PutIfAbsent(context.Background(), index); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(root, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
		if _, err := store.Resolve(context.Background(), index.IndexDigest); err == nil {
			t.Fatal("index store accepted unsafe root permissions")
		}
	})

	t.Run("record-permissions", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "indexes")
		store, err := newTestRiskIndexStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		index := newAuthorityStoreFixture(98).firstIndex(t, "unsafe-record-mode")
		if err := store.PutIfAbsent(context.Background(), index); err != nil {
			t.Fatal(err)
		}
		path := rawCASRecordPath(root, index.IndexDigest)
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Resolve(context.Background(), index.IndexDigest); err == nil {
			t.Fatal("index store accepted unsafe record permissions")
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
		if _, err := newTestRiskIndexStore(t, link); err == nil {
			t.Fatal("index store accepted a symlink root")
		}
		if _, err := newTestRiskObservationStore(t, link); err == nil {
			t.Fatal("observation store accepted a symlink root")
		}
	})

	t.Run("unknown-inventory", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "indexes")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "attacker"), []byte("attacker"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := newTestRiskIndexStore(t, root); err == nil {
			t.Fatal("index store accepted unknown inventory")
		}
	})
}

func TestIndexStoreRejectsRecordTruncationAfterOpen(t *testing.T) {
	root := filepath.Join(t.TempDir(), "indexes")
	store, err := newTestRiskIndexStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	index := newAuthorityStoreFixture(94).firstIndex(t, "truncate")
	if err := store.PutIfAbsent(context.Background(), index); err != nil {
		t.Fatal(err)
	}
	path := rawCASRecordPath(root, index.IndexDigest)
	body := canonicalIndexBody(t, index)
	replaceWithFile(t, path, body[:len(body)/2])
	if _, err := store.Resolve(context.Background(), index.IndexDigest); err == nil {
		t.Fatal("index store accepted a record truncated after startup")
	}
}

func TestProjectionRealExactReplaceResidueReconcileAndTamperRejection(t *testing.T) {
	fixture := newAuthorityStoreFixture(95)
	first := fixture.firstIndex(t, "real-projection")
	second := fixture.nextIndex(t, first, "real-projection-2")
	third := fixture.nextIndex(t, second, "real-projection-3")
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
	if err := projection.ProjectWitnessSelected(context.Background(), second); err != nil {
		t.Fatalf("idempotent projection failed: %v", err)
	}
	restarted, err := NewProjection(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.ProjectWitnessSelected(context.Background(), second); err != nil {
		t.Fatalf("restarted projection did not verify exact current bytes: %v", err)
	}

	// A crash residue is never chosen locally. The wrapper supplies only the
	// SHA-256 of the exact witness-selected third index to ReconcileExact.
	residue := filepath.Join(root, "."+witnessSelectedProjectionName+"-00112233445566778899aabb.tmp")
	if err := os.WriteFile(residue, canonicalIndexBody(t, third), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restarted.ProjectWitnessSelected(context.Background(), third); err != nil {
		t.Fatalf("exact witness-selected residue reconciliation failed: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != witnessSelectedProjectionName {
		t.Fatalf("projection residue was not deterministically reconciled: entries=%v err=%v", entries, err)
	}

	t.Run("attacker-target", func(t *testing.T) {
		path := filepath.Join(root, witnessSelectedProjectionName)
		replaceWithFile(t, path, []byte(`{"generation":999,"current":true}`))
		err := restarted.ProjectWitnessSelected(context.Background(), third)
		if !errors.Is(err, ErrProjectionConflict) {
			t.Fatalf("attacker target did not fail closed: %v", err)
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil || string(body) != `{"generation":999,"current":true}` {
			t.Fatal("attacker target was silently replaced")
		}
	})
}

func TestProjectionRejectsSymlinkHardlinkPermissionsAndUnknownResidue(t *testing.T) {
	fixture := newAuthorityStoreFixture(96)
	first := fixture.firstIndex(t, "projection-files")
	second := fixture.nextIndex(t, first, "projection-files-2")

	t.Run("symlink", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "projection")
		projection, err := NewProjection(root)
		if err != nil {
			t.Fatal(err)
		}
		if err := projection.ProjectWitnessSelected(context.Background(), first); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, witnessSelectedProjectionName)
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		external := filepath.Join(t.TempDir(), "external.json")
		if err := os.WriteFile(external, canonicalIndexBody(t, first), 0o600); err != nil {
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
		path := filepath.Join(root, witnessSelectedProjectionName)
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
		residue := filepath.Join(root, "."+witnessSelectedProjectionName+"-ffeeddccbbaa998877665544.tmp")
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
