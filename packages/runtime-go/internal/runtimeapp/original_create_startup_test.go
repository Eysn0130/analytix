//go:build darwin || linux

package runtimeapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	startupport "analytix.local/runtime-go/internal/ports/startup"
)

func TestRuntimeOriginalCreateStartupBootstrapsCompositeJournalWithoutChangingOriginal(t *testing.T) {
	ctx := context.Background()
	core, _, _ := runtimeHeldAttachmentIntentFixtureV1(t)
	var names []string
	var infos []os.FileInfo
	for parent, component := range map[string]string{
		"private":                             "attachment-authority",
		"private/attachment-authority":        "owners",
		"private/attachment-authority/owners": "ab",
	} {
		name := filepath.Join(core.roots.DataDir, parent, domainprivatecas.CreateDirectoryResidueNameV1(component))
		if err := os.Mkdir(name, 0o700); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		names, infos = append(names, name), append(infos, info)
	}
	lease, err := persistencefs.AcquireCompositeLeaseWithSeparateOwnerRoots(core.roots, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if _, held := lease.FrozenJournalAuthority(); held {
		t.Fatal("fixture already has journal authority")
	}
	original, err := prepareRuntimeOriginalCreateStartupV1(ctx, core.roots, lease)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := ensureRuntimeJournalAuthorityAfterManagedPreflight(ctx, core.roots, lease, original)
	if err != nil || journal == nil {
		t.Fatalf("original creation proof did not reach composite bootstrap: %v", err)
	}
	root, held := lease.FrozenAuthority()
	if !held {
		t.Fatal("root authority was lost")
	}
	fresh, err := prepareRuntimeChildIdentityStartupV1(ctx, core.roots, root, lease, nil, original)
	if err != nil {
		t.Fatal(err)
	}
	preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, fresh)
	if err != nil {
		t.Fatal(err)
	}
	retained, err := original.retainedV1(ctx, preserved)
	if err != nil || retained == nil || retained.attachment != original.attachment || preserved.attachments.createResidues != original.attachment || retained.combined == nil || len(retained.combined.OwnerRootsV1()) != 1 || retained.combined.OwnerRootsV1()[0] != original.attachment.OwnerRootV1() {
		t.Fatalf("root/Core/held observation replaced original proof: %v", err)
	}
	for i, name := range names {
		info, err := os.Stat(name)
		if err != nil || !os.SameFile(infos[i], info) || infos[i].Mode() != info.Mode() || !infos[i].ModTime().Equal(info.ModTime()) {
			t.Fatalf("journal bootstrap changed original creation directory: %v", err)
		}
	}
	added := filepath.Join(core.roots.DataDir, "private", "attachment-authority", "owners", domainprivatecas.CreateDirectoryResidueNameV1("cd"))
	if err := os.Mkdir(added, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareRuntimeChildIdentityStartupV1(ctx, core.roots, root, lease, nil, original); err == nil {
		t.Fatal("later Core observation adopted a newly added creation directory")
	}
}

func TestRuntimeOriginalCreateStandaloneRecoveryRetainsUncleanedProof(t *testing.T) {
	ctx := context.Background()
	roots, err := persistencefs.ResolveRootSet(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	owner := filepath.Join(roots.DataDir, "private", "attachment-authority")
	for _, leaf := range []string{"owners", "use-receipts", "use-dispositions", "upload-intents", "upload-dispositions"} {
		if err := os.MkdirAll(filepath.Join(owner, leaf), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	residue := filepath.Join(owner, "owners", domainprivatecas.CreateDirectoryResidueNameV1("ab"))
	if err := os.Mkdir(residue, 0o700); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(residue)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := persistencefs.AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	root, held := lease.FrozenAuthority()
	if !held {
		t.Fatal("root authority unavailable")
	}
	journal, held := lease.FrozenJournalAuthority()
	if !held {
		t.Fatal("journal authority unavailable")
	}
	original, err := prepareRuntimeOriginalCreateStartupV1(ctx, roots, lease)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := persistencefs.NewStartupSnapshotReaderWithOriginalCreateResiduesV1(roots, original.proofV1()).CaptureManagedSnapshotV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("original-create-standalone-recovery"))
	baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	builder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(roots, root, journal, nil, original.proofV1())
	plan, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		for _, name := range []string{"a-independent", "z-independent"} {
			if err := os.MkdirAll(filepath.Join(stage.DataDir, "memory", name), 0o700); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	canceled, cancel := context.WithCancel(ctx)
	defer cancel()
	cut := cancelAfterTerminalDispositionContextV1{Context: canceled, cancel: cancel, dispositionPath: filepath.Join(roots.DataDir, "memory", "a-independent"), intentPath: filepath.Join(roots.DataDir, "memory", "z-independent")}
	if err := plan.Apply(cut); !errors.Is(err, context.Canceled) {
		t.Fatalf("independent directory program did not reach real interruption: %v", err)
	}
	if _, err := os.Stat(cut.dispositionPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cut.intentPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("interruption did not precede final independent directory")
	}
	if err := RecoverAuthenticatedExistingSemanticStartupWithPersistenceLeaseContextE(ctx, lease); err != nil {
		t.Fatalf("standalone authenticated recovery lost still-present original proof: %v", err)
	}
	if _, err := os.Stat(cut.intentPath); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(residue)
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("standalone recovery changed original creation directory: %v", err)
	}
}

func TestRuntimeOriginalCreateHeldPrepareActivateAndRestart(t *testing.T) {
	core, held, _ := runtimeHeldAttachmentIntentFixtureV1(t)
	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: core.roots.DataDir, DurableTempDir: core.roots.DurableDir}
	var names []string
	var infos []os.FileInfo
	for parent, component := range map[string]string{
		"private":                             "attachment-authority",
		"private/attachment-authority":        "owners",
		"private/attachment-authority/owners": "ab",
	} {
		name := filepath.Join(core.roots.DataDir, parent, domainprivatecas.CreateDirectoryResidueNameV1(component))
		if err := os.Mkdir(name, 0o700); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		names, infos = append(names, name), append(infos, info)
	}
	before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	assertOriginal := func() {
		t.Helper()
		after := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
		for name, original := range before {
			if !semanticOriginalRecordUnchangedForTestV1(original, after[name]) {
				t.Fatalf("startup changed original record at %s", name)
			}
		}
		for i, name := range names {
			info, err := os.Stat(name)
			if err != nil || !os.SameFile(infos[i], info) || infos[i].Mode() != info.Mode() || !infos[i].ModTime().Equal(info.ModTime()) {
				t.Fatalf("actual startup changed original creation directory: %v", err)
			}
		}
		disposition := filepath.Join(core.roots.DataDir, "private", "attachment-authority", "upload-dispositions", held.UploadID[:2], held.UploadID+".json")
		if _, err := os.Stat(disposition); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("startup disposed original held upload intent")
		}
	}
	lease, err := AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	prepared, err := PrepareRuntimeServerStartupWithPersistenceLeaseE(config, lease)
	if err != nil {
		t.Fatalf("actual held startup preparation: %v", err)
	}
	if prepared.originalCreates == nil || prepared.originalCreates.attachment == nil || len(prepared.originalCreates.attachment.RelativePathsV1()) != 3 {
		t.Fatal("prepared activation omitted original creation proof")
	}
	assertOriginal()
	handler, err := prepared.Activate(config)
	if err != nil {
		t.Fatalf("actual held startup activation: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, handler)
	assertOriginal()
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("actual held startup fresh restart: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, restarted)
	assertOriginal()
	// Every original held file is compared above; no test-only alternate key,
	// new grant, provider request or recovery permission enters this chain.
}

func TestRuntimeOriginalCreateStartupDoesNotRetainUnheldCleanup(t *testing.T) {
	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: t.TempDir(), DurableTempDir: t.TempDir()}
	initial, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	shutdownOwnedRuntimeHandler(t, initial)
	residue := filepath.Join(config.DataDir, "private", domainprivatecas.CreateDirectoryResidueNameV1("attachment-authority"))
	if err := os.Mkdir(residue, 0o700); err != nil {
		t.Fatal(err)
	}
	lease, err := AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	prepared, err := PrepareRuntimeServerStartupWithPersistenceLeaseE(config, lease)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.originalCreates == nil || prepared.originalCreates.attachment != nil {
		t.Fatal("unheld cleanup retained its pre-recovery original creation proof")
	}
	if _, err := os.Stat(residue); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unheld creation residue was not recovered: %v", err)
	}
	handler, err := prepared.Activate(config)
	if err != nil {
		t.Fatalf("stale original creation proof blocked activation: %v", err)
	}
	if lifecycle, ok := handler.(interface{ Shutdown(context.Context) error }); ok {
		defer lifecycle.Shutdown(context.Background())
	}
}
