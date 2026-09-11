//go:build darwin || linux

package evidenceregistry

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestRecoveryV2RejectsSpecialUnknownPhysicalInventory(t *testing.T) {
	root := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(root, "unknown-fifo"), 0o600); err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareRecoveryV2(context.Background(), root, access); err == nil {
		t.Fatal("special evidence-registry sibling was frozen as an optional semantic blocker")
	}
}

func TestHasStateRejectsNamedAndNestedUnsafePhysicalInventory(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		createParent bool
		seed         func(*testing.T, string, string)
	}{
		{
			name: "legacy-lock-symlink", createParent: true,
			seed: func(t *testing.T, _, root string) {
				t.Helper()
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(t.TempDir(), "outside-lock")
				if err := os.WriteFile(target, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filepath.Join(root, ".registry.lock")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "legacy-lock-special", createParent: true,
			seed: func(t *testing.T, _, root string) {
				t.Helper()
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := syscall.Mkfifo(filepath.Join(root, ".registry.lock"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "root-symlink", createParent: true,
			seed: func(t *testing.T, _, root string) {
				t.Helper()
				target := t.TempDir()
				if err := os.Symlink(target, root); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "root-special", createParent: true,
			seed: func(t *testing.T, _, root string) {
				t.Helper()
				if err := syscall.Mkfifo(root, 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "ancestor-symlink",
			seed: func(t *testing.T, base, _ string) {
				t.Helper()
				target := t.TempDir()
				if err := os.Symlink(target, filepath.Join(base, "private")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "ancestor-special",
			seed: func(t *testing.T, base, _ string) {
				t.Helper()
				if err := syscall.Mkfifo(filepath.Join(base, "private"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "child-symlink", createParent: true,
			seed: func(t *testing.T, _, root string) {
				t.Helper()
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatal(err)
				}
				target := t.TempDir()
				if err := os.Symlink(target, filepath.Join(root, "indexes")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "child-special", createParent: true,
			seed: func(t *testing.T, _, root string) {
				t.Helper()
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := syscall.Mkfifo(filepath.Join(root, "indexes"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			base := t.TempDir()
			root := filepath.Join(base, "private", "evidence-registry")
			if testCase.createParent {
				if err := os.MkdirAll(filepath.Dir(root), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			testCase.seed(t, base, root)
			access, err := privatecastest.NewAccessAuthority(base)
			if err != nil {
				t.Fatal(err)
			}
			if state, err := HasState(context.Background(), root, access); err == nil || state {
				t.Fatalf("unsafe state probe = %t, %v; want fatal", state, err)
			}
		})
	}
}

func TestRecoveryV2PhysicalRevalidationRejectsOwnerReplacementAfterProbe(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "evidence-registry")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unknown"), []byte("frozen"), 0o600); err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(base)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareRecoveryV2(context.Background(), root, access)
	if err != nil {
		t.Fatal(err)
	}
	displaced := filepath.Join(base, "displaced-registry")
	if err := os.Rename(root, displaced); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unknown"), []byte("frozen"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := prepared.RevalidatePhysicalV2(context.Background()); err == nil {
		t.Fatal("owner replacement between state probe and composition survived physical revalidation")
	}
}

func TestRecoveryV2RejectsLinkedUnknownPhysicalInventory(t *testing.T) {
	for _, testCase := range []struct {
		name string
		seed func(*testing.T, string)
	}{
		{
			name: "symlink",
			seed: func(t *testing.T, root string) {
				t.Helper()
				target := filepath.Join(t.TempDir(), "outside")
				if err := os.WriteFile(target, []byte("outside"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filepath.Join(root, "unknown-link")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "hardlink",
			seed: func(t *testing.T, root string) {
				t.Helper()
				first := filepath.Join(root, "unknown-first")
				if err := os.WriteFile(first, []byte("linked"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(first, filepath.Join(root, "unknown-second")); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			testCase.seed(t, root)
			access, err := privatecastest.NewAccessAuthority(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := PrepareRecoveryV2(context.Background(), root, access); err == nil {
				t.Fatal("linked evidence-registry sibling was frozen as an optional semantic blocker")
			}
		})
	}
}

func TestRecoveryV2RevalidationRejectsSameSizeUnknownContentRewrite(t *testing.T) {
	for _, testCase := range []struct {
		name string
		path func(string) string
	}{
		{name: "direct", path: func(root string) string { return filepath.Join(root, "unknown-regular") }},
		{name: "nested", path: func(root string) string { return filepath.Join(root, "thread-frozen-v1", "record") }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			target := testCase.path(root)
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
				t.Fatal(err)
			}
			access, err := privatecastest.NewAccessAuthority(root)
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := PrepareRecoveryV2(context.Background(), root, access)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte("rewritte"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := prepared.RevalidatePhysicalV2(context.Background()); err == nil {
				t.Fatal("same-size unknown content rewrite survived handle-relative physical revalidation")
			}
		})
	}
}

func TestRecoveryV2RejectsUnsafeNestedUnknownPhysicalInventory(t *testing.T) {
	for _, testCase := range []struct {
		name string
		seed func(*testing.T, string)
	}{
		{
			name: "symlink",
			seed: func(t *testing.T, directory string) {
				t.Helper()
				target := filepath.Join(t.TempDir(), "outside")
				if err := os.WriteFile(target, []byte("outside"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filepath.Join(directory, "link")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "hardlink",
			seed: func(t *testing.T, directory string) {
				t.Helper()
				first := filepath.Join(directory, "first")
				if err := os.WriteFile(first, []byte("linked"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(first, filepath.Join(directory, "second")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "special",
			seed: func(t *testing.T, directory string) {
				t.Helper()
				if err := syscall.Mkfifo(filepath.Join(directory, "fifo"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "case-alias",
			seed: func(t *testing.T, directory string) {
				t.Helper()
				for _, name := range []string{"record", "Record"} {
					if err := os.WriteFile(filepath.Join(directory, name), []byte(name), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				entries, err := os.ReadDir(directory)
				if err != nil {
					t.Fatal(err)
				}
				if len(entries) != 2 {
					t.Skip("host filesystem is case-insensitive")
				}
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			directory := filepath.Join(root, "thread-frozen-v1")
			if err := os.Mkdir(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			testCase.seed(t, directory)
			access, err := privatecastest.NewAccessAuthority(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := PrepareRecoveryV2(context.Background(), root, access); err == nil {
				t.Fatal("unsafe nested evidence-registry inventory was physically frozen")
			}
		})
	}
}

func TestRecoveryV2RejectsPerIdentityFirstSequenceAndExtensionGaps(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		sequences []uint64
	}{
		{name: "first-sequence-gap", sequences: []uint64{2}},
		{name: "extension-gap", sequences: []uint64{1, 3}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root, access := seedRecoveryV2IdentityLineage(t, testCase.sequences)
			prepared, err := PrepareRecoveryV2(context.Background(), root, access)
			if err != nil {
				t.Fatalf("prepare physically valid hostile lineage: %v", err)
			}
			if err := prepared.ValidateSemantics(context.Background()); err == nil {
				t.Fatalf("per-identity registry sequence gap was admitted: %#v", testCase.sequences)
			}
		})
	}
}

func TestRecoveryV2FreshRestartReenablesWitnessedCapabilityAfterExactSemanticRepair(t *testing.T) {
	root, access := seedRecoveryV2IdentityLineage(t, []uint64{1})
	indexFiles, err := filepath.Glob(filepath.Join(root, "indexes", "*", "*.json"))
	if err != nil || len(indexFiles) != 1 {
		t.Fatalf("seeded recovery V2 index inventory = %#v, %v; want one record", indexFiles, err)
	}
	indexPath := indexFiles[0]
	canonicalIndex, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	rootBefore, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	indexesBefore, err := os.Stat(filepath.Join(root, "indexes"))
	if err != nil {
		t.Fatal(err)
	}
	capsulesBefore, err := os.Stat(filepath.Join(root, "capsules"))
	if err != nil {
		t.Fatal(err)
	}
	indexBefore, err := os.Stat(indexPath)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(indexPath, []byte(`{"corrupt":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	blocked, err := PrepareRecoveryV2(context.Background(), root, access)
	if err != nil {
		t.Fatalf("semantic corruption escaped optional recovery isolation: %v", err)
	}
	if err := blocked.ValidateSemantics(context.Background()); err == nil {
		t.Fatal("semantically corrupt V2 authority was activated")
	}
	if blocked.WitnessedV2ActivationAllowed() {
		t.Fatal("semantically corrupt V2 authority enabled witnessed capability")
	}
	if !blocked.HasStateV2() {
		t.Fatal("semantically corrupt V2 authority was misclassified as absent state")
	}
	if err := blocked.RevalidatePhysicalV2(context.Background()); err != nil {
		t.Fatalf("stable corrupt bytes were misclassified as a physical process-safety fault: %v", err)
	}

	if err := os.WriteFile(indexPath, canonicalIndex, 0o600); err != nil {
		t.Fatal(err)
	}
	repaired, err := PrepareRecoveryV2(context.Background(), root, access)
	if err != nil {
		t.Fatalf("fresh recovery preparation after exact repair: %v", err)
	}
	if err := repaired.ValidateSemantics(context.Background()); err != nil {
		t.Fatalf("fresh recovery semantics after exact repair: %v", err)
	}
	if err := repaired.Revalidate(context.Background()); err != nil {
		t.Fatalf("fresh recovery physical identity after exact repair: %v", err)
	}
	if !repaired.WitnessedV2ActivationAllowed() || repaired.AuthorityKnownEmptyV2() {
		t.Fatalf("fresh repaired V2 capability remained disabled: witnessed=%t empty=%t",
			repaired.WitnessedV2ActivationAllowed(), repaired.AuthorityKnownEmptyV2())
	}

	for name, before := range map[string]os.FileInfo{
		"root": rootBefore, "indexes": indexesBefore, "capsules": capsulesBefore, "index": indexBefore,
	} {
		path := map[string]string{
			"root": root, "indexes": filepath.Join(root, "indexes"),
			"capsules": filepath.Join(root, "capsules"), "index": indexPath,
		}[name]
		after, statErr := os.Stat(path)
		if statErr != nil || !os.SameFile(before, after) {
			t.Fatalf("%s physical identity changed during semantic repair: %v", name, statErr)
		}
	}
}

func seedRecoveryV2IdentityLineage(
	t *testing.T,
	sequences []uint64,
) (string, *privatecastest.AccessAuthority) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "evidence-registry")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	indexes, err := NewAuthorityIndexStoreV2(filepath.Join(root, "indexes"), access)
	if err != nil {
		t.Fatal(err)
	}
	capsules, err := NewAuthorityCapsuleStoreV2(filepath.Join(root, "capsules"), access)
	if err != nil {
		t.Fatal(err)
	}
	securityContext := storeTestContext(t)
	registry, err := domainevidence.NewEvidenceReceiptRegistry(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyID := domainsecurity.SHA256Hex(publicKey)
	sign := func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil }
	installationID := domainsecurity.SHA256Hex([]byte("recovery-v2-sequence-installation"))
	enrollmentID := domainsecurity.SHA256Hex([]byte("recovery-v2-sequence-enrollment"))
	previousIndexDigest := domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2()
	sequenceIndex := 0
	for ordinal := uint64(1); sequenceIndex < len(sequences); ordinal++ {
		input := storeTestPreparedInputForLabel(
			t, securityContext, "recovery-sequence-"+strconv.FormatUint(ordinal, 10), strconv.FormatUint(ordinal*100, 10),
		)
		registry, _, err = domainevidence.RegisterEvidenceReceipt(
			registry, input.Draft, input.CanonicalEvidence, input.SettlementProof, input.RegisteredAt,
		)
		if err != nil {
			t.Fatal(err)
		}
		if registry.Sequence != sequences[sequenceIndex] {
			continue
		}
		capsule, err := domainevidence.NewEvidenceRegistryAuthorityCapsule(
			securityContext, registry, keyID, publicKey, sign,
		)
		if err != nil {
			t.Fatal(err)
		}
		index, err := domainevidence.NewEvidenceRegistryAuthorityIndexV2(
			domainevidence.EvidenceRegistryAuthorityIndexInputV2{
				InstallationID:      installationID,
				EnrollmentID:        enrollmentID,
				Generation:          uint64(sequenceIndex + 1),
				PreviousIndexDigest: previousIndexDigest,
				MutationID: domainsecurity.SHA256Hex([]byte(
					"recovery-v2-sequence-mutation-" + strconv.Itoa(sequenceIndex+1),
				)),
			},
			capsule, keyID, publicKey, sign,
		)
		if err != nil {
			t.Fatal(err)
		}
		if err := capsules.PutIfAbsent(context.Background(), capsule); err != nil {
			t.Fatal(err)
		}
		if err := indexes.PutIfAbsent(context.Background(), index); err != nil {
			t.Fatal(err)
		}
		previousIndexDigest = index.IndexDigest
		sequenceIndex++
	}
	return root, access
}
