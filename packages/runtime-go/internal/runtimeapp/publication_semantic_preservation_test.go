//go:build darwin || linux

package runtimeapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	startupport "analytix.local/runtime-go/internal/ports/startup"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestRuntimeUnavailablePublicationOriginalCannotBeDeletedBySignedRecovery(t *testing.T) {
	for _, held := range []bool{true, false} {
		name := "ordinary"
		if held {
			name = "held-unknown"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			_, config := runtimeWitnessedRegistryConfigV2(t)
			handler, err := NewRuntimeServerHandlerE(config)
			if err != nil {
				t.Fatal(err)
			}
			shutdownOwnedRuntimeHandler(t, handler)
			roots, err := resolveRuntimePersistenceRoots(config)
			if err != nil {
				t.Fatal(err)
			}
			if held {
				runtimeReportPreservationAtRootsFixtureV1(t, roots, false, true, false)
			}
			access, err := privatecastest.NewAccessAuthority(filepath.Join(roots.DataDir, "private"))
			if err != nil {
				t.Fatal(err)
			}
			address := domainsecurity.SHA256Hex([]byte("synthetic unavailable original PII grant"))
			relative := filepath.Join("private", "pii-authorization", "grants", address[:2], address+".json")
			cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(roots.DataDir, "private", "pii-authorization", "grants"), 2<<20, access)
			if err != nil {
				t.Fatal(err)
			}
			if err := cas.PutIfAbsent(ctx, address, []byte(`{"domainCanary":"`+runtimeOptionalDomainPrivateCanary+`"}`)); err != nil {
				t.Fatal(err)
			}
			if err := cas.Close(); err != nil {
				t.Fatal(err)
			}
			prefix := filepath.Join("private", "a-publication-cut.bin")
			remaining := filepath.Join("private", "z-publication-remaining.bin")
			for _, file := range []string{prefix, remaining} {
				if err := os.WriteFile(filepath.Join(roots.DataDir, file), []byte("synthetic original independent file"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := persistencefs.NewStartupSnapshotReader(roots).CaptureManagedSnapshotV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			digest := domainsecurity.SHA256Hex([]byte("synthetic unavailable publication signed cut"))
			baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
			if err != nil {
				t.Fatal(err)
			}
			// Model an already signed transaction produced before this guard.
			builder := persistencefs.NewSemanticPlanBuilder(roots)
			prepared, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				for _, file := range []string{prefix, relative, remaining} {
					if err := os.Remove(filepath.Join(stage.DataDir, file)); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			defer prepared.Close()
			operations := prepared.Plan().Operations
			if len(operations) != 3 || operations[0].Path != "data/"+filepath.ToSlash(prefix) || operations[1].Path != "data/"+filepath.ToSlash(relative) || operations[2].Path != "data/"+filepath.ToSlash(remaining) {
				t.Fatal("fixture does not contain the exact managed signed removal sequence")
			}
			cancelled, cancel := context.WithCancel(ctx)
			defer cancel()
			cut := cancelAfterPendingRemovalContextV1{Context: cancelled, cancel: cancel, removedPaths: []string{filepath.Join(roots.DataDir, prefix)}, remainingPath: filepath.Join(roots.DataDir, relative)}
			if err := prepared.Apply(cut); !errors.Is(err, context.Canceled) {
				t.Fatalf("signed pre-grant cut failed: %v", err)
			}
			if _, err := os.Stat(filepath.Join(roots.DataDir, prefix)); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("signed prefix was not applied")
			}
			if _, err := os.Stat(filepath.Join(roots.DataDir, relative)); err != nil {
				t.Fatal("original grant was lost before root recovery")
			}
			before := startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)
			handler, err = NewRuntimeServerHandlerE(config)
			if err == nil {
				shutdownOwnedRuntimeHandler(t, handler)
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)) {
				t.Fatal("actual root signed recovery changed unavailable original or remaining managed state")
			}
			if err == nil {
				t.Fatal("actual root accepted a signed rewrite of unavailable original publication history")
			}
			lease, err := AcquireRuntimePersistenceLease(config)
			if err != nil {
				t.Fatal(err)
			}
			err = RecoverAuthenticatedExistingSemanticStartupWithPersistenceLeaseContextE(ctx, lease)
			if closeErr := lease.Close(); closeErr != nil {
				t.Fatal(closeErr)
			}
			if err == nil || !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)) {
				t.Fatal("standalone maintenance recovery bypassed original publication preservation")
			}
		})
	}
}

// This fixture has no held Core work. Its only trust comes from the enrolled
// installation and native physical observations used by the actual root.
func runtimePublicationOriginalFixtureV1(t *testing.T, withGrant bool) (*runtimeChildIdentityStartupV1, Config, string, []byte, string, []byte, *runtimePublicationSemanticPreservationV1) {
	t.Helper()
	ctx := context.Background()
	fixture, config := runtimeWitnessedRegistryConfigV2(t)
	roots, err := resolveRuntimePersistenceRoots(config)
	if err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(filepath.Join(roots.DataDir, "private"))
	if err != nil {
		t.Fatal(err)
	}
	for _, owner := range runtimePublicationOwnersV1 {
		for _, root := range runtimePrivateCASExpectedRoots(roots.DataDir, owner) {
			if err := os.MkdirAll(root, 0o700); err != nil {
				t.Fatal(err)
			}
		}
	}
	grant, ledger := runtimeOptionalPIIGrantLedgerFixture(t, fixture.Authority)
	grantBody, err := domainpii.PIIProjectionGrantV1Bytes(grant)
	if err != nil {
		t.Fatal(err)
	}
	ledgerBody, err := domainpublication.ClaimLedgerV1Bytes(ledger)
	if err != nil {
		t.Fatal(err)
	}
	grantPath := filepath.Join("private", "pii-authorization", "grants", grant.RecordDigest[:2], grant.RecordDigest+".json")
	ledgerPath := filepath.Join("private", "report-publication", "claim-ledgers", ledger.LedgerDigest[:2], ledger.LedgerDigest+".json")
	if withGrant {
		cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(roots.DataDir, "private", "pii-authorization", "grants"), 2<<20, access)
		if err != nil {
			t.Fatal(err)
		}
		if err := errors.Join(cas.PutIfAbsent(ctx, grant.RecordDigest, grantBody), cas.Close()); err != nil {
			t.Fatal(err)
		}
	}
	rootAuthority, err := persistencefs.FreezeRootAuthority(roots)
	if err != nil {
		t.Fatal(err)
	}
	installation, err := loadRuntimeOptionalDomainInstallation(ctx, config, roots, rootAuthority)
	if err != nil {
		t.Fatal(err)
	}
	core := &runtimeChildIdentityStartupV1{roots: roots, access: access}
	preserved, err := prepareRuntimePublicationSemanticPreservationV1(ctx, core, installation)
	if err != nil {
		t.Fatal(err)
	}
	if preserved.unavailable != withGrant {
		t.Fatal("fixture original availability is incorrect")
	}
	return core, config, grantPath, grantBody, ledgerPath, ledgerBody, preserved
}

func TestRuntimeUnavailablePublicationFreezesWholeOriginalClosure(t *testing.T) {
	core, _, grantPath, _, ledgerPath, ledgerBody, guard := runtimePublicationOriginalFixtureV1(t, true)
	before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	for _, scenario := range []string{"remove", "replace", "file-mode", "owner-mode", "fill-missing-ledger", "add-opaque-residue", "remove-empty-dependent-leaf"} {
		t.Run(scenario, func(t *testing.T) {
			err := runtimeAssociatedSignedPlanV1(t, core, runtimeReportRestartPreservationV1{publication: guard}, func(stage startupport.PersistenceRootsV1) error {
				switch scenario {
				case "remove":
					return os.Remove(filepath.Join(stage.DataDir, grantPath))
				case "replace":
					return os.WriteFile(filepath.Join(stage.DataDir, grantPath), []byte(`{}`), 0o600)
				case "file-mode":
					return os.Chmod(filepath.Join(stage.DataDir, grantPath), 0o400)
				case "owner-mode":
					return os.Chmod(filepath.Join(stage.DataDir, "private", "pii-authorization"), 0o750)
				case "fill-missing-ledger":
					if err := os.MkdirAll(filepath.Dir(filepath.Join(stage.DataDir, ledgerPath)), 0o700); err != nil {
						return err
					}
					return os.WriteFile(filepath.Join(stage.DataDir, ledgerPath), ledgerBody, 0o600)
				case "add-opaque-residue":
					return os.WriteFile(filepath.Join(stage.DataDir, "private", "pii-authorization", "grants", ".unbound.tmp"), []byte("synthetic residue"), 0o600)
				default:
					return os.Remove(filepath.Join(stage.DataDir, "private", "controlled-artifact-access", "access-dispositions"))
				}
			})
			if err == nil || !strings.Contains(err.Error(), "original publication") {
				t.Fatalf("unavailable original mutation was not rejected: %v", err)
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("guard rejection changed managed original state")
			}
		})
	}
	if err := runtimeAssociatedSignedPlanV1(t, core, runtimeReportRestartPreservationV1{publication: guard}, func(startupport.PersistenceRootsV1) error { return nil }); err != nil {
		t.Fatalf("unavailable original blocked independent managed write: %v", err)
	}
}

func TestRuntimeHealthyPublicationAllowsIndependentCompleteGraph(t *testing.T) {
	core, _, grantPath, grantBody, ledgerPath, ledgerBody, guard := runtimePublicationOriginalFixtureV1(t, false)
	err := runtimeAssociatedSignedPlanV1(t, core, runtimeReportRestartPreservationV1{publication: guard}, func(stage startupport.PersistenceRootsV1) error {
		for file, body := range map[string][]byte{grantPath: grantBody, ledgerPath: ledgerBody} {
			if err := os.MkdirAll(filepath.Dir(filepath.Join(stage.DataDir, file)), 0o700); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(stage.DataDir, file), body, 0o600); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("healthy original blocked complete independent grant/ledger graph: %v", err)
	}
	fresh, err := prepareRuntimePublicationSemanticPreservationV1(context.Background(), core, guard.installation)
	if err != nil || fresh.unavailable {
		t.Fatalf("independent final graph did not reobserve healthy: %v", err)
	}
}

func TestRuntimePublicationSignedAddedDependencyCannotHealOriginal(t *testing.T) {
	ctx := context.Background()
	core, config, _, _, ledgerPath, ledgerBody, guard := runtimePublicationOriginalFixtureV1(t, true)
	snapshot, err := persistencefs.NewStartupSnapshotReader(core.roots).CaptureManagedSnapshotV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("synthetic signed dependency healing prefix"))
	baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join("private", "zz-publication-unapplied.bin")
	prepared, err := persistencefs.NewSemanticPlanBuilder(core.roots).Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(stage.DataDir, ledgerPath)), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(stage.DataDir, ledgerPath), ledgerBody, 0o600); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(stage.DataDir, marker), []byte(`{"independent":true}`), 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	runtimeAssociatedRequireMarkerV1(t, prepared.Plan(), "data/"+filepath.ToSlash(marker))
	cancelled, cancel := context.WithCancel(ctx)
	defer cancel()
	cut := cancelAfterTerminalDispositionContextV1{Context: cancelled, cancel: cancel, dispositionPath: filepath.Join(core.roots.DataDir, ledgerPath), intentPath: filepath.Join(core.roots.DataDir, marker)}
	if err := prepared.Apply(cut); !errors.Is(err, context.Canceled) {
		t.Fatalf("signed dependency prefix did not reach cut: %v", err)
	}
	if body, err := os.ReadFile(cut.dispositionPath); err != nil || !reflect.DeepEqual(body, ledgerBody) {
		t.Fatal("exact dependency was not physically installed")
	}
	physical, err := prepareRuntimeOptionalDomainCapabilitiesForOwnersV1(ctx, core.roots.DataDir, core.access, guard.installation, runtimePublicationDomainOwner)
	if err != nil {
		t.Fatal(err)
	}
	for _, owner := range runtimePublicationOwnersV1 {
		if !physical[owner].available {
			t.Fatal("physical dependency prefix is not healthy")
		}
	}
	before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	handler, err := NewRuntimeServerHandlerE(config)
	if err == nil {
		shutdownOwnedRuntimeHandler(t, handler)
		t.Fatal("physical healthy prefix concealed unavailable Original")
	}
	if !strings.Contains(err.Error(), "original publication") {
		t.Fatalf("rejection did not reach original publication proof: %v", err)
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
		t.Fatal("original rejection ran after remaining signed write")
	}
}

func TestRuntimePublicationSignedDeletedBeforeCannotBecomeEmptyOriginal(t *testing.T) {
	ctx := context.Background()
	core, config, grantPath, _, _, _, _ := runtimePublicationOriginalFixtureV1(t, true)
	remaining := filepath.Join("private", "zz-publication-remove-after.bin")
	if err := os.WriteFile(filepath.Join(core.roots.DataDir, remaining), []byte("synthetic retained independent original"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := persistencefs.NewStartupSnapshotReader(core.roots).CaptureManagedSnapshotV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("synthetic signed deleted publication Before"))
	baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := persistencefs.NewSemanticPlanBuilder(core.roots).Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		if err := os.Remove(filepath.Join(stage.DataDir, grantPath)); err != nil {
			return err
		}
		return os.Remove(filepath.Join(stage.DataDir, remaining))
	})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	operations := prepared.Plan().Operations
	if len(operations) != 2 || operations[0].Path != "data/"+filepath.ToSlash(grantPath) || operations[1].Path != "data/"+filepath.ToSlash(remaining) {
		t.Fatal("fixture does not contain exact managed removal program")
	}
	cancelled, cancel := context.WithCancel(ctx)
	defer cancel()
	cut := cancelAfterPendingRemovalContextV1{Context: cancelled, cancel: cancel, removedPaths: []string{filepath.Join(core.roots.DataDir, grantPath)}, remainingPath: filepath.Join(core.roots.DataDir, remaining)}
	if err := prepared.Apply(cut); !errors.Is(err, context.Canceled) {
		t.Fatalf("signed deleted-Before cut failed: %v", err)
	}
	if _, err := os.Stat(cut.removedPaths[0]); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("fixture did not lose actual Before bytes")
	}
	before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	handler, err := NewRuntimeServerHandlerE(config)
	if err == nil {
		shutdownOwnedRuntimeHandler(t, handler)
		t.Fatal("lost Before became empty healthy Original")
	}
	if !strings.Contains(err.Error(), "original publication Before bytes are unavailable") {
		t.Fatalf("lost Before did not reach original proof refusal: %v", err)
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
		t.Fatal("lost Before rejection ran after remaining signed removal")
	}
}

func TestRuntimePublicationOriginalRequiresExistingLocalKeyWithoutEnrollment(t *testing.T) {
	for _, foreign := range []bool{true, false} {
		name := "local"
		if foreign {
			name = "foreign"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			core, _, _, _, _, _, _ := runtimePublicationOriginalFixtureV1(t, false)
			keyPath := filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json")
			if foreign {
				keyPath = filepath.Join(t.TempDir(), "synthetic-foreign-key.json")
			}
			key, err := finalauthority.OpenOrCreateFileAuthority(keyPath, !foreign)
			if err != nil {
				t.Fatal(err)
			}
			grant, ledger := runtimeOptionalPIIGrantLedgerFixture(t, key)
			grantBody, err := domainpii.PIIProjectionGrantV1Bytes(grant)
			if err != nil {
				t.Fatal(err)
			}
			ledgerBody, err := domainpublication.ClaimLedgerV1Bytes(ledger)
			if err != nil {
				t.Fatal(err)
			}
			for _, record := range []struct {
				owner, leaf, digest string
				body                []byte
			}{{"pii-authorization", "grants", grant.RecordDigest, grantBody}, {"report-publication", "claim-ledgers", ledger.LedgerDigest, ledgerBody}} {
				cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(core.roots.DataDir, "private", record.owner, record.leaf), 2<<20, core.access)
				if err != nil {
					t.Fatal(err)
				}
				if err := errors.Join(cas.PutIfAbsent(ctx, record.digest, record.body), cas.Close()); err != nil {
					t.Fatal(err)
				}
			}
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			guard, err := prepareRuntimePublicationSemanticPreservationV1(ctx, core, nil)
			if foreign && err == nil {
				t.Fatal("complete foreign-signed Original was considered healthy without the existing local key")
			}
			if !foreign && (err != nil || guard.unavailable) {
				t.Fatalf("existing local-key healthy Original lost compatibility: %v", err)
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("local-key observation changed raw original state")
			}
		})
	}
}

func TestRuntimeUnavailablePublicationPreservesOriginalEmptyShard(t *testing.T) {
	core, config, _, _, _, _, _ := runtimePublicationOriginalFixtureV1(t, true)
	shard := filepath.Join(core.roots.DataDir, "private", "pii-authorization", "grants", "ff")
	if err := os.Mkdir(shard, 0o700); err != nil {
		t.Fatal(err)
	}
	original := startupWholeTreeRecordMapForTest(t, filepath.Join(core.roots.DataDir, "private", "pii-authorization"))
	handler, err := NewRuntimeServerHandlerE(config)
	if err == nil {
		shutdownOwnedRuntimeHandler(t, handler)
	}
	if !reflect.DeepEqual(original, startupWholeTreeRecordMapForTest(t, filepath.Join(core.roots.DataDir, "private", "pii-authorization"))) {
		t.Fatal("ordinary startup deleted an unavailable original empty shard")
	}
	if err != nil {
		t.Fatalf("preserved unavailable original prevented ordinary startup: %v", err)
	}
}

func runtimePublicationCreateDirectoriesV1(t *testing.T, core *runtimeChildIdentityStartupV1, partial bool) (owners, creates []string) {
	t.Helper()
	for _, owner := range runtimePublicationOwnersV1 {
		root := filepath.Join(core.roots.DataDir, "private", owner)
		owners = append(owners, root)
		leaves := runtimePrivateCASExpectedRoots(core.roots.DataDir, owner)
		paths := []string{
			filepath.Join(filepath.Dir(root), domainprivatecas.CreateDirectoryResidueNameV1(owner)),
			filepath.Join(root, domainprivatecas.CreateDirectoryResidueNameV1(filepath.Base(leaves[0]))),
			filepath.Join(leaves[0], domainprivatecas.CreateDirectoryResidueNameV1("ab")),
		}
		for _, p := range paths {
			if err := os.Mkdir(p, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		creates = append(creates, paths...)
		owners = append(owners, paths[0])
		if partial && owner != "pii-authorization" {
			for _, leaf := range leaves[1:] {
				if err := os.Remove(leaf); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	return owners, creates
}

func TestRuntimeUnavailablePublicationRetainsFull4CreatesAndEmptyPartial(t *testing.T) {
	for _, partial := range []bool{false, true} {
		name := "complete"
		if partial {
			name = "empty-partial-dependencies"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			core, config, _, _, _, _, initialGuard := runtimePublicationOriginalFixtureV1(t, true)
			owners, creates := runtimePublicationCreateDirectoriesV1(t, core, partial)
			var err error
			core.originalCreates, err = prepareRuntimeOriginalCreateStartupV1(ctx, core.roots, core.access)
			if err != nil {
				t.Fatal(err)
			}
			guard, err := prepareRuntimePublicationSemanticPreservationV1(ctx, core, initialGuard.installation)
			if err != nil || !guard.unavailable {
				t.Fatalf("original C/partial observation: %v", err)
			}
			original := startupWholeTreeRecordMapForTest(t, owners...)
			t.Run("stage-preservation", func(t *testing.T) {
				for _, directory := range creates {
					relative, err := filepath.Rel(core.roots.DataDir, directory)
					if err != nil {
						t.Fatal(err)
					}
					err = runtimeAssociatedSignedPlanV1(t, core, runtimeReportRestartPreservationV1{publication: guard}, func(stage startupport.PersistenceRootsV1) error {
						return os.Remove(filepath.Join(stage.DataDir, relative))
					})
					if err == nil {
						t.Fatalf("stage deletion of original C directory was accepted: %s", relative)
					}
					if !reflect.DeepEqual(original, startupWholeTreeRecordMapForTest(t, owners...)) {
						t.Fatal("rejected stage changed original creation topology")
					}
				}
				if err := runtimeAssociatedSignedPlanV1(t, core, runtimeReportRestartPreservationV1{publication: guard}, func(startupport.PersistenceRootsV1) error { return nil }); err != nil {
					t.Fatalf("independent stage with original C/partial: %v", err)
				}
			})
			t.Run("root-restarts", func(t *testing.T) {
				independent := filepath.Join(core.roots.DataDir, "private", domainprivatecas.CreateDirectoryResidueNameV1("attachment-authority"))
				if err := os.Mkdir(independent, 0o700); err != nil {
					t.Fatal(err)
				}
				for restart := 0; restart < 2; restart++ {
					handler, err := NewRuntimeServerHandlerE(config)
					if err == nil {
						shutdownOwnedRuntimeHandler(t, handler)
					}
					if !reflect.DeepEqual(original, startupWholeTreeRecordMapForTest(t, owners...)) {
						t.Fatal("actual root changed unavailable original C/partial topology")
					}
					if err != nil {
						t.Fatalf("actual ordinary restart %d with frozen C/partial: %v", restart, err)
					}
					if _, err := os.Lstat(independent); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("independent unheld original creation residue was not recovered: %v", err)
					}
				}
			})
		})
	}
}

func TestRuntimeHealthyPublicationEmptyPartialCreatesRecoverNormally(t *testing.T) {
	core, config, _, _, _, _, _ := runtimePublicationOriginalFixtureV1(t, false)
	_, creates := runtimePublicationCreateDirectoriesV1(t, core, true)
	handler, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("healthy empty partial/C startup: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, handler)
	for _, directory := range creates {
		if _, err := os.Lstat(directory); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("healthy original C was not recovered: %v", err)
		}
	}
}

// This fixture models a journal signed by the pre-preservation producer. Its
// stage can remove the original temp; actual startup still uses native proofs.
type runtimeLegacyPlainJournalProducerV1 struct {
	privatecasport.OriginalPlainResiduesV1
}

func (runtimeLegacyPlainJournalProducerV1) ObserveCopiedOriginalPlainResiduesV1(context.Context, string, privatecasport.RecoveryAccessAuthority) (privatecasport.OriginalPlainResiduesV1, error) {
	return nil, nil
}

func TestRuntimeHealthyPublicationPlainResidueRecovery(t *testing.T) {
	for _, route := range []string{"native-cleanup", "signed-root", "signed-maintenance"} {
		t.Run(route, func(t *testing.T) {
			ctx := context.Background()
			_, config := runtimeWitnessedRegistryConfigV2(t)
			initial, err := NewRuntimeServerHandlerE(config)
			if err != nil {
				t.Fatal(err)
			}
			shutdownOwnedRuntimeHandler(t, initial)
			roots, err := resolveRuntimePersistenceRoots(config)
			if err != nil {
				t.Fatal(err)
			}
			access, err := privatecastest.NewAccessAuthority(filepath.Join(roots.DataDir, "private"))
			if err != nil {
				t.Fatal(err)
			}
			digest := domainsecurity.SHA256Hex([]byte("healthy original plain report temp"))
			residue := filepath.Join(roots.DataDir, "private", "report-publication", "attempts", digest[:2], "."+digest+".json-original.tmp")
			if err := os.MkdirAll(filepath.Dir(residue), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(residue, []byte("healthy opaque report write interrupted"), 0o600); err != nil {
				t.Fatal(err)
			}
			if route != "native-cleanup" {
				prefix := filepath.Join(roots.DataDir, "private", "a-legacy-plain-prefix.bin")
				if err := os.WriteFile(prefix, []byte("independent signed prefix"), 0o600); err != nil {
					t.Fatal(err)
				}
				original, err := prepareRuntimeOriginalCreateStartupV1(ctx, roots, access)
				if err != nil {
					t.Fatal(err)
				}
				if original.plain == nil {
					t.Fatal("legacy journal fixture has no native plain observation")
				}
				producer := runtimeLegacyPlainJournalProducerV1{original.plainProofV1()}
				legacyContext := persistencefs.WithOriginalPlainResiduesV1(ctx, producer)
				snapshot, err := persistencefs.NewStartupSnapshotReader(roots).CaptureManagedSnapshotV1(legacyContext)
				if err != nil {
					t.Fatal(err)
				}
				configuration := domainsecurity.SHA256Hex([]byte("healthy original plain legacy recovery"))
				baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, configuration, time.Unix(1, 0).UTC())
				if err != nil {
					t.Fatal(err)
				}
				builder := persistencefs.NewSemanticPlanBuilder(roots)
				plan, err := builder.Prepare(legacyContext, baseline, configuration, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
					for _, source := range []string{prefix, residue} {
						relative, err := filepath.Rel(roots.DataDir, source)
						if err != nil {
							return err
						}
						if err := os.Remove(filepath.Join(stage.DataDir, relative)); err != nil {
							return err
						}
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				defer plan.Close()
				if len(plan.Plan().Operations) != 2 || plan.Plan().Operations[0].Path != "data/private/a-legacy-plain-prefix.bin" {
					t.Fatal("legacy plain journal does not have the intended exact prefix")
				}
				cancelled, cancel := context.WithCancel(legacyContext)
				defer cancel()
				cut := cancelAfterPendingRemovalContextV1{Context: cancelled, cancel: cancel, removedPaths: []string{prefix}, remainingPath: residue}
				if err := plan.Apply(cut); !errors.Is(err, context.Canceled) {
					t.Fatalf("legacy signed prefix did not stop at the intended cut: %v", err)
				}
				if _, err := os.Stat(residue); err != nil {
					t.Fatal("legacy prefix lost original plain bytes")
				}
			}
			if route == "signed-maintenance" {
				lease, err := AcquireRuntimePersistenceLease(config)
				if err != nil {
					t.Fatal(err)
				}
				recovered := RecoverAuthenticatedExistingSemanticStartupWithPersistenceLeaseContextE(ctx, lease)
				if err := errors.Join(recovered, lease.Close()); err != nil {
					t.Fatalf("healthy signed plain maintenance failed: %v", err)
				}
			} else {
				handler, err := NewRuntimeServerHandlerE(config)
				if handler != nil {
					shutdownOwnedRuntimeHandler(t, handler)
				}
				if err != nil {
					t.Fatalf("healthy plain root recovery failed: %v", err)
				}
			}
			if _, err := os.Stat(residue); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("healthy original plain residue was not recovered: %v", err)
			}
		})
	}
}
