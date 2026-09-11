package persistencefs_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	startupport "analytix.local/runtime-go/internal/ports/startup"
)

func originalCreateSnapshotFixtureV1(t *testing.T) (persistencefs.RootSet, *persistencefs.RootAuthority, *finalauthority.PreparedSecurePrivateCASOriginalCreateResiduesV1, []string, []os.FileInfo) {
	t.Helper()
	ctx := context.Background()
	base := t.TempDir()
	roots := persistencefs.RootSet{DataDir: filepath.Join(base, "data"), DurableDir: filepath.Join(base, "durable")}
	owner := filepath.Join(roots.DataDir, "private", "attachment-authority")
	for _, name := range []string{roots.DurableDir, "owners", "use-receipts", "use-dispositions", "upload-intents", "upload-dispositions"} {
		if !filepath.IsAbs(name) {
			name = filepath.Join(owner, name)
		}
		if err := os.MkdirAll(name, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	names := []string{
		filepath.Join(filepath.Dir(owner), domainprivatecas.CreateDirectoryResidueNameV1("attachment-authority")),
		filepath.Join(owner, domainprivatecas.CreateDirectoryResidueNameV1("owners")),
		filepath.Join(owner, "owners", domainprivatecas.CreateDirectoryResidueNameV1("ab")),
	}
	before := make([]os.FileInfo, len(names))
	for index, name := range names {
		if err := os.Mkdir(name, 0o700); err != nil {
			t.Fatal(err)
		}
		var err error
		before[index], err = os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
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
	creates, err := finalauthority.PrepareSecurePrivateCASCreateResidueRecoveryV1(ctx, roots.DataDir, access)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := creates.OriginalCreateResiduesV1(ctx, "private/attachment-authority")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := persistencefs.CaptureStrictContext(ctx, roots); err == nil {
		t.Fatal("ordinary full snapshot accepted creation residue")
	}
	return roots, root, proof, names, before
}

func TestOriginalCreateResiduesSurviveSemanticSnapshotAndApply(t *testing.T) {
	ctx := context.Background()
	roots, root, proof, names, before := originalCreateSnapshotFixtureV1(t)
	reader := persistencefs.NewStartupSnapshotReaderWithOriginalCreateResiduesV1(roots, proof)
	snapshot, err := reader.CaptureManagedSnapshotV1(ctx)
	if err != nil {
		t.Fatalf("proved original creation directories blocked full snapshot: %v", err)
	}
	digest := domainsecurity.SHA256Hex([]byte("original-create-semantic-roundtrip"))
	baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(roots)
	if err != nil {
		t.Fatal(err)
	}
	builder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(roots, root, journal, nil, proof)
	plan, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		return os.MkdirAll(filepath.Join(stage.DataDir, "memory", "independent"), 0o700)
	})
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
	for index, name := range names {
		after, err := os.Stat(name)
		if err != nil || !os.SameFile(before[index], after) || before[index].Mode() != after.Mode() || !before[index].ModTime().Equal(after.ModTime()) {
			t.Fatalf("semantic roundtrip changed original creation directory: %v", err)
		}
	}
	if err := proof.Revalidate(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestOriginalCreateSnapshotRejectsProofAndStageDrift(t *testing.T) {
	for _, kind := range []string{"new stage residue", "removed stage residue", "replaced stage residue", "nonempty stage residue", "new original residue", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			roots, root, proof, names, _ := originalCreateSnapshotFixtureV1(t)
			reader := persistencefs.NewStartupSnapshotReaderWithOriginalCreateResiduesV1(roots, proof)
			if kind == "canceled" {
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				if _, err := reader.CaptureManagedSnapshotV1(canceled); !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation cause lost: %v", err)
				}
				return
			}
			if kind == "new original residue" {
				name := filepath.Join(roots.DataDir, "private", "attachment-authority", domainprivatecas.CreateDirectoryResidueNameV1("use-receipts"))
				if err := os.Mkdir(name, 0o700); err != nil {
					t.Fatal(err)
				}
				if _, err := reader.CaptureManagedSnapshotV1(ctx); err == nil {
					t.Fatal("snapshot adopted a new original residue")
				}
				return
			}
			snapshot, err := reader.CaptureManagedSnapshotV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			digest := domainsecurity.SHA256Hex([]byte("original-create-stage-negative"))
			baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
			if err != nil {
				t.Fatal(err)
			}
			journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(roots)
			if err != nil {
				t.Fatal(err)
			}
			builder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(roots, root, journal, nil, proof)
			plan, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				relative, err := filepath.Rel(roots.DataDir, names[0])
				if err != nil {
					return err
				}
				target := filepath.Join(stage.DataDir, relative)
				switch kind {
				case "new stage residue":
					return os.Mkdir(filepath.Join(stage.DataDir, "private", "attachment-authority", domainprivatecas.CreateDirectoryResidueNameV1("use-receipts")), 0o700)
				case "removed stage residue":
					return os.Remove(target)
				case "replaced stage residue":
					if err := os.Rename(target, target+"-held-original"); err != nil {
						return err
					}
					return os.Mkdir(target, 0o700)
				case "nonempty stage residue":
					return os.WriteFile(filepath.Join(target, "unexpected"), []byte("synthetic"), 0o600)
				}
				return errors.New("unknown synthetic stage cut")
			})
			if err == nil {
				if plan != nil {
					_ = plan.Close()
				}
				t.Fatal("semantic simulation adopted changed stage residue")
			}
			if err := proof.Revalidate(ctx); err != nil {
				t.Fatalf("stage rejection changed original directories: %v", err)
			}
		})
	}
}

func TestOriginalCreateResiduesAllowAuthenticatedInterruptedObservation(t *testing.T) {
	for _, cut := range []string{"after_operation", "after_operation_journal", "after_journal_retirement_tombstone", "after_journal_retired"} {
		t.Run(cut, func(t *testing.T) {
			ctx := context.Background()
			roots, root, proof, _, _ := originalCreateSnapshotFixtureV1(t)
			reader := persistencefs.NewStartupSnapshotReaderWithOriginalCreateResiduesV1(roots, proof)
			snapshot, err := reader.CaptureManagedSnapshotV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			digest := domainsecurity.SHA256Hex([]byte("original-create-interrupted"))
			baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
			if err != nil {
				t.Fatal(err)
			}
			journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(roots)
			if err != nil {
				t.Fatal(err)
			}
			builder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(roots, root, journal, nil, proof)
			cause := errors.New("synthetic original-create interruption")
			persistencefs.SetOriginalCreateSemanticFaultForTestV1(builder, func(phase string, index int) error {
				if phase == cut && (index == 0 || index == -1) {
					return cause
				}
				return nil
			})
			plan, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				return os.MkdirAll(filepath.Join(stage.DataDir, "memory", "independent"), 0o700)
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := plan.Apply(ctx); !errors.Is(err, cause) {
				t.Fatalf("interruption cause lost: %v", err)
			}
			if cut == "after_operation" || cut == "after_operation_journal" {
				observation, err := persistencefs.ObserveAuthenticatedSemanticJournalV1(ctx, roots, proof)
				if err != nil || observation == nil {
					t.Fatalf("typed original proof blocked signed interrupted observation: %v", err)
				}
				if err := observation.Revalidate(ctx); err != nil {
					t.Fatal(err)
				}
				if _, err := persistencefs.ObserveAuthenticatedSemanticJournalV1(ctx, roots); err == nil {
					t.Fatal("ordinary journal observation borrowed the original proof")
				}
			}
			freshRoot, err := persistencefs.FreezeRootAuthority(roots)
			if err != nil {
				t.Fatal(err)
			}
			if err := persistencefs.NewSemanticPlanBuilderWithAuthorities(roots, freshRoot, journal).RecoverAuthenticatedExisting(ctx); err == nil {
				t.Fatal("ordinary recovery borrowed original creation proof")
			}
			fresh := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(roots, freshRoot, journal, nil, proof)
			if err := fresh.RecoverAuthenticatedExisting(ctx); err != nil {
				t.Fatal(err)
			}
			if err := plan.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := reader.CaptureManagedSnapshotV1(ctx); err != nil {
				t.Fatal(err)
			}
			if err := proof.Revalidate(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type originalCreateFinalReadbackFailureV1 struct {
	privatecasport.OriginalCreateResiduesV1
	fail  bool
	cause error
}

func (proof *originalCreateFinalReadbackFailureV1) Revalidate(ctx context.Context) error {
	if proof.fail {
		return proof.cause
	}
	return proof.OriginalCreateResiduesV1.Revalidate(ctx)
}

func TestOriginalCreateFinalReadbackPreservesFailureCause(t *testing.T) {
	for _, kind := range []string{"canceled", "proof unavailable"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			roots, root, original, _, _ := originalCreateSnapshotFixtureV1(t)
			proof := &originalCreateFinalReadbackFailureV1{OriginalCreateResiduesV1: original, cause: errors.New("synthetic original proof unavailable")}
			expected := proof.cause
			if kind == "canceled" {
				expected = context.Canceled
			}
			snapshot, err := persistencefs.NewStartupSnapshotReaderWithOriginalCreateResiduesV1(roots, proof).CaptureManagedSnapshotV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			digest := domainsecurity.SHA256Hex([]byte("original-create-final-cause"))
			baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
			if err != nil {
				t.Fatal(err)
			}
			journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(roots)
			if err != nil {
				t.Fatal(err)
			}
			builder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(roots, root, journal, nil, proof)
			plan, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				return os.MkdirAll(filepath.Join(stage.DataDir, "memory", "independent"), 0o700)
			})
			if err != nil {
				t.Fatal(err)
			}
			last := len(plan.Plan().Operations) - 1
			if last < 0 {
				t.Fatal("synthetic plan did not contain operations")
			}
			persistencefs.SetOriginalCreateSemanticFaultForTestV1(builder, func(phase string, index int) error {
				if phase == "after_operation_journal" && index == last {
					if kind == "canceled" {
						cancel()
					} else {
						proof.fail = true
					}
				}
				return nil
			})
			if err := plan.Apply(ctx); !errors.Is(err, expected) {
				t.Fatalf("final readback lost exact failure cause: %v", err)
			}
		})
	}
}
