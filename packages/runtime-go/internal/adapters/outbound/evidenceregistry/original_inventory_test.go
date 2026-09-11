package evidenceregistry

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func originalRegistryInventoryForTest(t *testing.T, ctx context.Context, root string, contexts []domainsecurity.TurnSecurityContext, authority finalauthorityport.Authority) ([]registryport.InventoryRecord, error) {
	t.Helper()
	access, err := privatecastest.NewAccessAuthority(filepath.Dir(root))
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareRecoveryV2(ctx, root, access)
	if err != nil {
		return nil, err
	}
	return prepared.SnapshotOriginalLegacyInventoryV1(ctx, contexts, authority)
}

func TestOriginalRegistryInventoryPreservesAbsenceAndDoesNotCreateLock(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	root := filepath.Join(base, "registry")
	authority := storeTestAuthority(t, root)
	if records, err := originalRegistryInventoryForTest(t, ctx, root, nil, authority); err != nil || len(records) != 0 {
		t.Fatalf("original absent registry: records=%d err=%v", len(records), err)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("original registry absence changed: %v", err)
	}
	store, err := NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	frozen := storeTestContext(t)
	material := storeTestMaterial(t)
	if _, err := store.CommitPrepared(ctx, registryport.CommitPreparedInput{Context: frozen, Draft: storeTestDraft(t, frozen, material), CanonicalEvidence: material, SettlementProof: storeTestProof(), RegisteredAt: storeTestTime()}); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(root, ".registry.lock")
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{store.registryPath(frozen), store.registryAuthoritySealPath(frozen)} {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	if records, err := originalRegistryInventoryForTest(t, ctx, root, []domainsecurity.TurnSecurityContext{frozen}, authority); err != nil || len(records) != 1 || records[0].Registry.Sequence != 1 {
		t.Fatalf("original signed registry with missing projections: records=%d err=%v", len(records), err)
	}
	for _, path := range []string{lockPath, store.registryPath(frozen), store.registryAuthoritySealPath(frozen)} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("original observation created %s: %v", filepath.Base(path), err)
		}
	}
}

type originalRegistryContextAuthority struct {
	finalauthorityport.Authority
	want         context.Context
	cause        error
	seen         bool
	beforeVerify func()
}

func (authority *originalRegistryContextAuthority) VerifyTrusted(ctx context.Context, keyID string, publicKey, body, signature []byte) error {
	if ctx != authority.want {
		return errors.New("registry verifier lost original context")
	}
	authority.seen = true
	if authority.beforeVerify != nil {
		authority.beforeVerify()
	}
	if authority.cause != nil {
		return authority.cause
	}
	return authority.Authority.VerifyTrusted(ctx, keyID, publicKey, body, signature)
}

func TestOriginalRegistryInventoryRetainsVerificationContextAndCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root := filepath.Join(t.TempDir(), "registry")
	base := storeTestAuthority(t, root)
	store, err := NewStore(root, base)
	if err != nil {
		t.Fatal(err)
	}
	frozen := storeTestContext(t)
	material := storeTestMaterial(t)
	if _, err := store.CommitPrepared(ctx, registryport.CommitPreparedInput{Context: frozen, Draft: storeTestDraft(t, frozen, material), CanonicalEvidence: material, SettlementProof: storeTestProof(), RegisteredAt: storeTestTime()}); err != nil {
		t.Fatal(err)
	}
	cause := errors.New("original installation verification unavailable")
	authority := &originalRegistryContextAuthority{Authority: base, want: ctx, cause: cause}
	if records, err := originalRegistryInventoryForTest(t, ctx, root, []domainsecurity.TurnSecurityContext{frozen}, authority); !errors.Is(err, cause) || len(records) != 0 || !authority.seen {
		t.Fatalf("original verifier evidence was flattened: records=%d seen=%v err=%v", len(records), authority.seen, err)
	}
}

func originalRegistryTreeForTest(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[rel] = info.Mode().String()
		if info.Mode().IsRegular() {
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			result[rel] += ":" + domainsecurity.SHA256Hex(body)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestOriginalRegistryInventoryRejectsIncompleteOrChangedObservation(t *testing.T) {
	for _, scenario := range []string{"missing-context", "duplicate-context", "unknown-file", "changed-index", "index-verifier-enoent", "cancelled-during-verify", "mode-preserved"} {
		t.Run(scenario, func(t *testing.T) {
			if scenario == "mode-preserved" && runtime.GOOS == "windows" {
				t.Skip("Unix mode bits")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			root := filepath.Join(t.TempDir(), "registry")
			base := storeTestAuthority(t, root)
			store, err := NewStore(root, base)
			if err != nil {
				t.Fatal(err)
			}
			frozen := storeTestContext(t)
			if _, err := store.CommitPrepared(ctx, storeTestPreparedInputForLabel(t, frozen, "original", "100")); err != nil {
				t.Fatal(err)
			}
			contexts := []domainsecurity.TurnSecurityContext{frozen}
			if scenario == "missing-context" {
				contexts = nil
			}
			if scenario == "duplicate-context" {
				contexts = append(contexts, frozen)
			}
			if scenario == "unknown-file" {
				if err := os.WriteFile(filepath.Join(root, "unknown"), []byte("synthetic"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "mode-preserved" {
				if err := os.Chmod(root, 0o500); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
			}
			access, err := privatecastest.NewAccessAuthority(filepath.Dir(root))
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := PrepareRecoveryV2(ctx, root, access)
			if err != nil {
				t.Fatal(err)
			}
			before := originalRegistryTreeForTest(t, root)
			authority := &originalRegistryContextAuthority{Authority: base, want: ctx}
			if scenario == "index-verifier-enoent" {
				authority.cause = os.ErrNotExist
			}
			if scenario == "cancelled-during-verify" {
				authority.beforeVerify = cancel
			}
			if scenario == "changed-index" {
				changed := false
				authority.beforeVerify = func() {
					if changed {
						return
					}
					changed = true
					path := filepath.Join(root, evidenceRegistryAuthorityIndexFile)
					body, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil {
						t.Fatal(err)
					}
				}
			}
			records, err := prepared.SnapshotOriginalLegacyInventoryV1(ctx, contexts, authority)
			if scenario == "mode-preserved" {
				if err != nil || len(records) != 1 {
					t.Fatalf("original mode-preserving read: records=%d err=%v", len(records), err)
				}
			} else if err == nil || records != nil {
				t.Fatalf("invalid original observation returned usable inventory: records=%d err=%v", len(records), err)
			}
			if scenario == "index-verifier-enoent" && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("lost original verification ENOENT: %v", err)
			}
			if scenario == "cancelled-during-verify" && !errors.Is(err, context.Canceled) {
				t.Fatalf("lost cancellation: %v", err)
			}
			if scenario != "changed-index" && !reflect.DeepEqual(before, originalRegistryTreeForTest(t, root)) {
				t.Fatal("original observation changed bytes or modes")
			}
		})
	}
}

type originalRegistryReadCloserForTest struct{ readErr, closeErr error }

func (file originalRegistryReadCloserForTest) Read([]byte) (int, error) { return 0, file.readErr }
func (file originalRegistryReadCloserForTest) Close() error             { return file.closeErr }

func TestOriginalRegistryReadRetainsReadAndCloseFailures(t *testing.T) {
	readErr, closeErr := errors.New("original read unavailable"), errors.New("original close unavailable")
	body, err := readOriginalRegistryBodyV1(originalRegistryReadCloserForTest{readErr, closeErr}, 1)
	if body != nil || !errors.Is(err, readErr) || !errors.Is(err, closeErr) {
		t.Fatalf("lost physical failure: body=%d err=%v", len(body), err)
	}
}

func TestOriginalRegistryPartialGraphNeedsBothCompleteEndpoints(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "registry")
	authority := storeTestAuthority(t, root)
	store, err := NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	frozen := storeTestContext(t)
	if _, err := store.CommitPrepared(ctx, storeTestPreparedInputForLabel(t, frozen, "first", "100")); err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(filepath.Dir(root))
	if err != nil {
		t.Fatal(err)
	}
	beforePlan, err := PrepareRecoveryV2(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	before, err := beforePlan.SnapshotOriginalLegacyFilesV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPrepared(ctx, storeTestPreparedInputForLabel(t, frozen, "second", "200")); err != nil {
		t.Fatal(err)
	}
	afterPlan, err := PrepareRecoveryV2(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	after, err := afterPlan.SnapshotOriginalLegacyFilesV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []map[string]OriginalLegacyEntryV1{before, after} {
		if records, err := ParseOriginalLegacyInventoryV1(ctx, endpoint, []domainsecurity.TurnSecurityContext{frozen}, authority); err != nil || len(records) != 1 {
			t.Fatalf("complete endpoint: records=%d err=%v", len(records), err)
		}
	}
	partial := map[string]OriginalLegacyEntryV1{}
	for name, file := range before {
		partial[name] = file
	}
	partial[evidenceRegistryAuthorityIndexFile] = after[evidenceRegistryAuthorityIndexFile]
	if err := ValidateOriginalLegacyFilesV1(ctx, partial, authority); err != nil {
		t.Fatalf("individually authenticated physical records: %v", err)
	}
	if records, err := ParseOriginalLegacyInventoryV1(ctx, partial, []domainsecurity.TurnSecurityContext{frozen}, authority); err == nil || records != nil {
		t.Fatal("partial graph became complete registry authority")
	}
}

func TestOriginalRegistryCandidateKeepsPhysicalBoundsAndCaseUniqueness(t *testing.T) {
	ctx := context.Background()
	authority := storeTestAuthority(t, filepath.Join(t.TempDir(), "registry"))
	for _, scenario := range []string{"entries", "aggregate-bytes", "case-alias"} {
		t.Run(scenario, func(t *testing.T) {
			files := map[string]OriginalLegacyEntryV1{".": {Directory: true, Mode: 0o700}}
			switch scenario {
			case "entries":
				for i := 0; i <= maxRegistryOwnerEntries; i++ {
					files[".registry-authority-index-"+strconv.Itoa(i)+".tmp"] = OriginalLegacyEntryV1{Mode: 0o600}
				}
			case "aggregate-bytes":
				files[".registry-authority-index-a.tmp"] = OriginalLegacyEntryV1{Mode: 0o600, Body: make([]byte, maxRegistrySiblingBytes+1)}
			case "case-alias":
				files[".registry-authority-index-a.tmp"] = OriginalLegacyEntryV1{Mode: 0o600}
				files[".registry-authority-index-A.tmp"] = OriginalLegacyEntryV1{Mode: 0o600}
			}
			if err := ValidateOriginalLegacyFilesV1(ctx, files, authority); err == nil {
				t.Fatal("raw candidate bypassed physical owner constraints")
			}
			if records, err := ParseOriginalLegacyInventoryV1(ctx, files, nil, authority); err == nil || records != nil {
				t.Fatal("complete candidate bypassed physical owner constraints")
			}
		})
	}
}
