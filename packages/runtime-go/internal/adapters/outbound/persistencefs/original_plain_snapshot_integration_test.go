package persistencefs_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	startupport "analytix.local/runtime-go/internal/ports/startup"
)

func originalPlainSnapshotFixtureV1(t *testing.T) (persistencefs.RootSet, *persistencefs.RootAuthority, *finalauthority.OriginalPlainResiduesV1, string) {
	t.Helper()
	roots := persistencefs.RootSet{DataDir: t.TempDir(), DurableDir: t.TempDir()}
	owner := filepath.Join(roots.DataDir, "private", "report-publication")
	leaves := []finalauthority.SecurePrivateCASOwnerLeafV1{}
	for _, spec := range domainprivatecas.RuntimeRootSpecsV1() {
		if filepath.Dir(spec.RelativeCASRoot) != "report-publication" {
			continue
		}
		leaf := filepath.Base(spec.RelativeCASRoot)
		leaves = append(leaves, finalauthority.SecurePrivateCASOwnerLeafV1{Name: leaf, MaxBytes: 4096})
		if err := os.MkdirAll(filepath.Join(owner, leaf), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	shard := filepath.Join(owner, "attempts", "aa")
	if err := os.Mkdir(shard, 0o700); err != nil {
		t.Fatal(err)
	}
	residue := filepath.Join(shard, "."+strings.Repeat("a", 64)+".json-original.tmp")
	if err := os.WriteFile(residue, []byte("original opaque uncommitted bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := persistencefs.FreezeRootAuthority(roots)
	if err != nil {
		t.Fatal(err)
	}
	access, err := persistencefs.NewSemanticStagePrivateCASAccessAuthority(roots, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := access.Close(); err != nil {
			t.Error(err)
		}
	})
	observed, err := finalauthority.PrepareOriginalFixedOwnerObservationV1(context.Background(), owner, leaves, access)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := finalauthority.FreezeOriginalPlainResiduesV1(context.Background(), []*finalauthority.OriginalFixedOwnerObservationV1{observed})
	if err != nil || proof == nil {
		t.Fatalf("native plain proof unavailable: %v", err)
	}
	return roots, root, proof, residue
}

func TestOriginalPlainSnapshotRequiresExactPhysicalProof(t *testing.T) {
	for _, fault := range []string{"none", "no-proof", "wrong-root", "body", "mode", "additional-file", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			roots, _, proof, residue := originalPlainSnapshotFixtureV1(t)
			ctx := persistencefs.WithOriginalPlainResiduesV1(context.Background(), proof)
			switch fault {
			case "no-proof":
				ctx = context.Background()
			case "wrong-root":
				roots.DataDir = t.TempDir()
			case "body":
				if err := os.WriteFile(residue, []byte("changed opaque bytes"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "mode":
				if err := os.Chmod(residue, 0o400); err != nil {
					t.Fatal(err)
				}
			case "additional-file":
				if err := os.WriteFile(strings.Replace(residue, "original.tmp", "additional.tmp", 1), []byte("new opaque bytes"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			snapshot, err := persistencefs.CaptureStrictContext(ctx, roots)
			if fault != "none" {
				if err == nil {
					t.Fatal("unproved or changed plain residue was accepted")
				}
				if fault == "cancelled" && !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			state := proof.FileStatesV1()[0]
			found := false
			for _, entry := range snapshot.Entries {
				if entry.Path == "data/"+state.RelativePath {
					found = entry.Type == "file" && entry.Mode == state.Mode && entry.Size == state.Size && entry.SHA256 == state.SHA256
				}
			}
			if !found {
				t.Fatal("original plain file was omitted from the managed denominator")
			}
		})
	}
}

func TestOriginalPlainResiduesBindCopiedStageBeforeSimulation(t *testing.T) {
	for _, mutation := range []string{"independent", "replace-original", "add-residue"} {
		t.Run(mutation, func(t *testing.T) {
			roots, root, proof, residue := originalPlainSnapshotFixtureV1(t)
			ctx := persistencefs.WithOriginalPlainResiduesV1(context.Background(), proof)
			reader := persistencefs.NewStartupSnapshotReader(roots)
			snapshot, err := reader.CaptureManagedSnapshotV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			digest := domainsecurity.SHA256Hex([]byte("original-plain-semantic-roundtrip"))
			baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
			if err != nil {
				t.Fatal(err)
			}
			journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(roots)
			if err != nil {
				t.Fatal(err)
			}
			builder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(roots, root, journal, nil)
			plan, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				if mutation == "independent" {
					return os.MkdirAll(filepath.Join(stage.DataDir, "memory", "independent"), 0o700)
				}
				file := filepath.Join(stage.DataDir, filepath.FromSlash(proof.FileStatesV1()[0].RelativePath))
				if mutation == "add-residue" {
					file = strings.Replace(file, "original.tmp", "additional.tmp", 1)
				}
				return os.WriteFile(file, []byte("unauthorized staged bytes"), 0o600)
			})
			if mutation != "independent" {
				if plan != nil {
					_ = plan.Close()
				}
				if err == nil {
					t.Fatal("stage re-observation adopted a new original file set")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if err := plan.Apply(ctx); err != nil {
					t.Fatal(err)
				}
				if err := plan.Close(); err != nil {
					t.Fatal(err)
				}
				if err := builder.RecoverAuthenticatedExisting(ctx); err != nil {
					t.Fatal(err)
				}
				if _, err := reader.CaptureManagedSnapshotV1(ctx); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(filepath.Join(roots.DataDir, "memory", "independent")); err != nil {
					t.Fatal(err)
				}
			}
			body, err := os.ReadFile(residue)
			if err != nil || domainsecurity.SHA256Hex(body) != proof.FileStatesV1()[0].SHA256 {
				t.Fatal("original plain bytes changed")
			}
		})
	}
}
