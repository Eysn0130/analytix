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

	datasetsnapshotstore "analytix.local/runtime-go/internal/adapters/outbound/datasetsnapshot"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	datasetsnapshotapp "analytix.local/runtime-go/internal/app/datasetsnapshot"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	datasetport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	startupport "analytix.local/runtime-go/internal/ports/startup"
	fixturev2 "analytix.local/runtime-go/internal/testsupport/datasetsnapshotv2fixture"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

// Provision a real local synthetic dataset and complete stored witness
// exchanges. Admission is fixture setup, never a prerequisite imposed by the
// original-history observer or the UNKNOWN preservation guard.
func runtimeAssociatedOriginalFixtureV1(t *testing.T) (*runtimeChildIdentityStartupV1, string, string) {
	t.Helper()
	ctx := context.Background()
	fixture, config := runtimeWitnessedRegistryConfigV2(t)
	private := filepath.Join(config.DataDir, "private")
	access, err := privatecastest.NewAccessAuthority(private)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := datasetsnapshotstore.OpenStoresV2(filepath.Join(private, "dataset-snapshot-authority"), access)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := openRuntimeSharedEvidenceStoresV2(config.DataDir, access)
	if err != nil {
		t.Fatal(err)
	}
	composition, configured, err := newRuntimeSharedEvidenceDatasetSnapshotV2(ctx, config, fixture.Authority, stores, evidence, access, filestore.CaseBindingReader{}, nil)
	if err != nil || !configured || composition.snapshot == nil || composition.evidence == nil {
		t.Fatalf("associated fixture composition: %v", err)
	}
	if _, err := composition.evidence.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	runtimeSharedEvidenceWriteCaseBindingV2(t, workspace)
	observation, err := (filestore.CaseBindingReader{}).Observe(workspace)
	if err != nil {
		t.Fatal(err)
	}
	manifest, producer, materials := runtimeSharedEvidenceBoundMaterialsV2(t, observation.WorkspaceRealPath, observation, fixture.InstallationID, fixture.Authority)
	materialRoot := filepath.Join(private, "dataset-snapshot-authority", "materials")
	materialStore, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(materialRoot, 16<<20, access)
	if err != nil {
		t.Fatal(err)
	}
	for _, entries := range materials {
		for digest, body := range entries {
			if err := materialStore.PutIfAbsent(ctx, digest, body); err != nil && !errors.Is(err, os.ErrExist) {
				t.Fatal(err)
			}
		}
	}
	resolved, err := composition.snapshot.AdmitExactV2(ctx, datasetsnapshotapp.AdmitInputV2{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, Observation: observation,
		ManifestReference: manifest, FundsProducerReference: producer, AcceptedAt: time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := errors.Join(materialStore.Close(), stores.Close(), evidence.Close()); err != nil {
		t.Fatal(err)
	}
	input := domainsecurity.TurnSecurityContextInput{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		ThreadID: "thread-report-held", TurnID: "turn-report-held", WorkspaceRealPath: observation.WorkspaceRealPath,
		CaseID: observation.CaseID, CaseBindingHash: observation.CaseBindingHash, DatasetSnapshotID: resolved.Record.DatasetSnapshotID,
		SourceManifestHash: resolved.Record.SourceManifestHash, ContextEpoch: 1, IssuedAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC),
	}
	frozen, err := securitycontexttest.CaseExecutionContextV2(input)
	if err != nil {
		t.Fatal(err)
	}
	input.PublicationPolicy, err = domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: frozen.PublicationPolicy.ThreadRiskPolicyDigest, RiskClass: domainsecurity.RiskClassCase,
		Disposition: domainsecurity.PublicationDispositionCaseEvidenceGate, CaseBindingState: domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: observation.ObservationDigest, BlockerCode: domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	input.RiskAuthorityBinding = frozen.RiskAuthorityBinding
	frozen, err = domainsecurity.NewTurnSecurityContextV2(input)
	if err != nil {
		t.Fatal(err)
	}
	roots, err := persistencefs.ResolveRootSet(config.DataDir, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	core, _ := runtimeReportPreservationAtRootsFixtureV1(t, roots, false, true, false, frozen)
	core.originalRegistryTrust, err = prepareRuntimeOriginalRegistryTrustV2(ctx, config, core.verification)
	if err != nil {
		t.Fatal(err)
	}
	var nested string
	for digest := range materials[datasetport.MaterialRawContentChunkV1] {
		nested = filepath.Join(materialRoot, digest[:2], digest+".json")
		break
	}
	if nested == "" {
		t.Fatal("fixture lost nested raw chunk")
	}
	return core, filepath.Join(materialRoot, resolved.Record.ManifestSHA256[:2], resolved.Record.ManifestSHA256+".json"), nested
}

func runtimeAssociatedSignedPlanV1(t *testing.T, core *runtimeChildIdentityStartupV1, preserved runtimeReportRestartPreservationV1, mutate func(startupport.PersistenceRootsV1) error) error {
	t.Helper()
	ctx := context.Background()
	snapshot, err := persistencefs.NewStartupSnapshotReaderWithOriginalCreateResiduesV1(core.roots, core.originalCreateProofV1()).CaptureManagedSnapshotV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("associated semantic original material"))
	baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	root, err := persistencefs.FreezeRootAuthority(core.roots)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(core.roots)
	if err != nil {
		t.Fatal(err)
	}
	builder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(core.roots, root, journal, preserved, core.originalCreateProofV1())
	prepared, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		if err := mutate(stage); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(stage.DataDir, "private", "a-associated-independent.bin"), []byte(`{"independent":true}`), 0o600)
	})
	if prepared != nil {
		defer prepared.Close()
		if err == nil {
			runtimeAssociatedRequireMarkerV1(t, prepared.Plan(), "data/private/a-associated-independent.bin")
			err = prepared.Apply(ctx)
		}
	}
	if err == nil {
		body, readErr := os.ReadFile(filepath.Join(core.roots.DataDir, "private", "a-associated-independent.bin"))
		if readErr != nil || string(body) != `{"independent":true}` {
			t.Fatalf("independent managed state was not applied: %v", readErr)
		}
	}
	return err
}

func runtimeAssociatedRequireMarkerV1(t *testing.T, plan domainstartup.SemanticStartupPlanV1, marker string) {
	t.Helper()
	for _, operation := range plan.Operations {
		if operation.Path == marker && operation.After.Type == domainstartup.ManagedEntryTypeFile && operation.After.SHA256 == domainsecurity.SHA256Hex([]byte(`{"independent":true}`)) {
			return
		}
	}
	t.Fatal("independent fixture marker is outside the signed managed program")
}

func runtimeAssociatedExpectRejectionV1(t *testing.T, core *runtimeChildIdentityStartupV1, preserved runtimeReportRestartPreservationV1, mutate func(startupport.PersistenceRootsV1) error) {
	t.Helper()
	before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	err := runtimeAssociatedSignedPlanV1(t, core, preserved, mutate)
	if err == nil || !strings.Contains(err.Error(), "original associated") {
		t.Fatalf("associated guard did not reject before effects: %v", err)
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
		t.Fatal("associated rejection ran after managed effects")
	}
}

func TestRuntimeOriginalAssociatedMaterialCannotBeRemovedBySignedSemanticPlan(t *testing.T) {
	ctx := context.Background()
	core, material, _ := runtimeAssociatedOriginalFixtureV1(t)
	signer, signErr := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
	if signErr != nil {
		t.Fatal(signErr)
	}

	preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	for _, guard := range preserved.associated.owners {
		if guard.unavailable {
			t.Fatal("real associated fixture is not a complete available historical graph")
		}
	}
	relative, _ := filepath.Rel(core.roots.DataDir, material)
	runtimeAssociatedExpectRejectionV1(t, core, preserved, func(stage startupport.PersistenceRootsV1) error {
		return os.Remove(filepath.Join(stage.DataDir, relative))
	})
	runtimeAssociatedExpectRejectionV1(t, core, preserved, func(stage startupport.PersistenceRootsV1) error {
		return os.Chmod(filepath.Join(stage.DataDir, relative), 0o400)
	})
	for name, entry := range preserved.associated.owners["evidence-authority"].original {
		if !entry.Directory && strings.HasPrefix(name, "observations/") {
			runtimeAssociatedExpectRejectionV1(t, core, preserved, func(stage startupport.PersistenceRootsV1) error {
				return os.Remove(filepath.Join(stage.DataDir, "private", "evidence-authority", name))
			})
			break
		}
	}
	// A new valid genesis sibling pointing at an existing held bundle must not
	// evade the guard merely because the bundle itself is already original.
	inventory, unavailable, err := observeRuntimeAssociatedDomainV1(ctx, core, "dataset-snapshot-authority", preserved.associated.owners["dataset-snapshot-authority"].original)
	if err != nil || unavailable || len(inventory.Indexes) != 1 {
		t.Fatalf("original index fixture: %v", err)
	}
	var original domainsecurity.DatasetSnapshotIndexV1
	for _, index := range inventory.Indexes {
		original = index
	}
	index, err := domainsecurity.NewDatasetSnapshotIndexV1(domainsecurity.DatasetSnapshotIndexInputV1{
		InstallationID: original.InstallationID, EnrollmentID: original.EnrollmentID, Generation: 1,
		PreviousIndexDigest: domainsecurity.DatasetSnapshotIndexGenesisDigestV1(), MutationID: domainsecurity.SHA256Hex([]byte("new held genesis sibling")),
		Binding: original.Binding, SnapshotRecordDigest: original.SnapshotRecordDigest, AuthorityKeyID: core.verification.KeyID(), AuthorityPublicKey: core.verification.PublicKey(),
	}, func(body []byte) ([]byte, error) { return signer.Sign(ctx, body) })
	if err != nil {
		t.Fatal(err)
	}
	body, _ := domainsecurity.DatasetSnapshotIndexV1Bytes(index)
	candidate := preserved.associated.owners["dataset-snapshot-authority"].original.cloneV1()
	candidate["indexes/"+index.IndexDigest[:2]] = finalauthority.SecurePrivateCASOriginalEntryV1{Directory: true, Mode: 0o700}
	candidate["indexes/"+index.IndexDigest[:2]+"/"+index.IndexDigest+".json"] = finalauthority.SecurePrivateCASOriginalEntryV1{Mode: 0o600, Body: body}
	if _, unavailable, err := observeRuntimeAssociatedDomainV1(ctx, core, "dataset-snapshot-authority", candidate); err != nil || unavailable {
		t.Fatalf("new held index must first be a valid historical branch: %v", err)
	}
	runtimeAssociatedExpectRejectionV1(t, core, preserved, func(stage startupport.PersistenceRootsV1) error {
		file := filepath.Join(stage.DataDir, "private", "dataset-snapshot-authority", "indexes", index.IndexDigest[:2], index.IndexDigest+".json")
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			return err
		}
		return os.WriteFile(file, body, 0o600)
	})
}

func TestRuntimeOriginalAssociatedNestedAbsenceIsPreservedWithoutReadmission(t *testing.T) {
	core, _, nested := runtimeAssociatedOriginalFixtureV1(t)
	body, err := os.ReadFile(nested)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(nested); err != nil {
		t.Fatal(err)
	}
	preserved, err := prepareRuntimeReportRestartPreservationV1(context.Background(), core)
	if err != nil {
		t.Fatalf("historical missing nested material became a startup gate: %v", err)
	}
	if !preserved.associated.owners["dataset-snapshot-authority"].unavailable {
		t.Fatal("missing nested material produced a complete graph")
	}
	relative, _ := filepath.Rel(core.roots.DataDir, nested)
	runtimeAssociatedExpectRejectionV1(t, core, preserved, func(stage startupport.PersistenceRootsV1) error {
		return os.WriteFile(filepath.Join(stage.DataDir, relative), body, 0o600)
	})
	if err := runtimeAssociatedSignedPlanV1(t, core, preserved, func(startupport.PersistenceRootsV1) error { return nil }); err != nil {
		t.Fatalf("ordinary independent state could not preserve original absence: %v", err)
	}
	if _, err := os.Stat(nested); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("original nested absence changed")
	}
}

func TestRuntimeOriginalAssociatedAllowsIndependentV2DatasetWithSameBinding(t *testing.T) {
	ctx := context.Background()
	core, _, _ := runtimeAssociatedOriginalFixtureV1(t)
	signer, signErr := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
	if signErr != nil {
		t.Fatal(signErr)
	}

	preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	guard := preserved.associated.owners["dataset-snapshot-authority"]
	inventory, unavailable, err := observeRuntimeAssociatedDomainV1(ctx, core, "dataset-snapshot-authority", guard.original)
	if err != nil || unavailable {
		t.Fatalf("original graph unavailable: %v", err)
	}
	var previous domainsecurity.DatasetSnapshotIndexV1
	for _, index := range inventory.Indexes {
		previous = index
	}
	admission, err := fixturev2.NewAcceptedSlotRetainedAdmissionV1(fixturev2.AcceptedSlotRetainedAdmissionInputV1{
		Binding: previous.Binding, OriginalExact: "synthetic-account-002", Canonical: "synthetic-account-002", Field: domainevidence.AcceptedSlotSourceFieldAccountV1,
		InstallationID: previous.InstallationID, AcceptedAt: time.Date(2026, 7, 31, 8, 0, 0, 0, time.UTC), AuthorityKeyID: core.verification.KeyID(), AuthorityPublicKey: core.verification.PublicKey(),
		Sign: func(body []byte) ([]byte, error) { return signer.Sign(ctx, body) },
	})
	if err != nil {
		t.Fatal(err)
	}
	var record domainsecurity.DatasetSnapshotAuthorityRecordV2
	err = admission.Snapshot.Manifest.WithExactFundsProducerAuthorityAdmissionV2(admission.Snapshot.FundsProducerContent, func(issuer domainsecurity.DatasetSnapshotAuthoritySealedAdmissionV2) error {
		var err error
		record, err = issuer.Issue(domainsecurity.DatasetSnapshotAuthoritySealedIssueInputV2{
			InstallationID: previous.InstallationID, AcceptedAt: time.Date(2026, 7, 31, 8, 0, 0, 0, time.UTC), PredecessorRecordDigest: previous.SnapshotRecordDigest,
			AuthorityKeyID: core.verification.KeyID(), AuthorityPublicKey: core.verification.PublicKey(),
		}, func(body []byte) ([]byte, error) { return signer.Sign(ctx, body) })
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Binding != previous.Binding || record.DatasetSnapshotID == inventory.Records[previous.SnapshotRecordDigest].V2.DatasetSnapshotID {
		t.Fatal("independent same-binding fixture did not change dataset")
	}
	index, err := domainsecurity.NewDatasetSnapshotIndexV1(domainsecurity.DatasetSnapshotIndexInputV1{
		InstallationID: previous.InstallationID, EnrollmentID: previous.EnrollmentID, Generation: previous.Generation + 1, PreviousIndexDigest: previous.IndexDigest,
		MutationID: domainsecurity.SHA256Hex([]byte("independent V2 dataset append")), Binding: record.Binding, SnapshotRecordDigest: record.RecordDigest,
		AuthorityKeyID: core.verification.KeyID(), AuthorityPublicKey: core.verification.PublicKey(),
	}, func(body []byte) ([]byte, error) { return signer.Sign(ctx, body) })
	if err != nil {
		t.Fatal(err)
	}
	err = runtimeAssociatedSignedPlanV1(t, core, preserved, func(stage startupport.PersistenceRootsV1) error {
		private := filepath.Join(stage.DataDir, "private")
		access, err := privatecastest.NewAccessAuthority(private)
		if err != nil {
			return err
		}
		stores, err := datasetsnapshotstore.OpenStoresV2(filepath.Join(private, "dataset-snapshot-authority"), access)
		if err != nil {
			return err
		}
		defer stores.Close()
		materials, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(private, "dataset-snapshot-authority", "materials"), 16<<20, access)
		if err != nil {
			return err
		}
		defer materials.Close()
		for _, values := range admission.Materials {
			for digest, body := range values {
				if err := materials.PutIfAbsent(ctx, digest, body); err != nil && !errors.Is(err, os.ErrExist) {
					return err
				}
			}
		}
		if err := stores.AuthorityBundles.PutIfAbsent(ctx, datasetport.AuthorityBundleV2{Record: record, Manifest: admission.Snapshot.Manifest}); err != nil {
			return err
		}
		return stores.Indexes.PutIfAbsent(ctx, index)
	})
	if err != nil {
		t.Fatalf("valid independent V2 append was blocked: %v", err)
	}
	current, err := prepareRuntimeAssociatedSemanticPreservationV1(ctx, core, preserved.report)
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.validateV1(ctx, core, "dataset-snapshot-authority", current.owners["dataset-snapshot-authority"].original, preserved.report, nil); err != nil {
		t.Fatal(err)
	}
	if len(current.owners["dataset-snapshot-authority"].original) <= len(guard.original) {
		t.Fatal("independent append did not reach original inventory after restart")
	}
}

func TestRuntimeOriginalAssociatedEmptyInitializedOwnersNeedNoEnrollment(t *testing.T) {
	core, _ := runtimeReportPreservationFixtureV1(t, false, true, false)
	for _, owner := range runtimeAssociatedOwnersV1 {
		for _, root := range runtimePrivateCASExpectedRoots(core.roots.DataDir, owner) {
			if err := os.MkdirAll(root, 0o700); err != nil {
				t.Fatal(err)
			}
		}
	}
	if core.originalRegistryTrust != nil {
		t.Fatal("fixture unexpectedly enrolled")
	}
	preserved, err := prepareRuntimeReportRestartPreservationV1(context.Background(), core)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtimeAssociatedSignedPlanV1(t, core, preserved, func(startupport.PersistenceRootsV1) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

type cancelAfterAssociatedOwnerCreateV1 struct {
	context.Context
	cancel context.CancelFunc
	owner  string
}

func (ctx cancelAfterAssociatedOwnerCreateV1) Err() error {
	if info, err := os.Stat(ctx.owner); err == nil && info.IsDir() {
		ctx.cancel()
	}
	return ctx.Context.Err()
}

func TestRuntimeOriginalAssociatedDirectoryPrefixResumesThroughActualRoot(t *testing.T) {
	for _, owner := range runtimeAssociatedOwnersV1 {
		t.Run(owner, func(t *testing.T) {
			ctx := context.Background()
			_, config := runtimeWitnessedRegistryConfigV2(t)
			roots, err := resolveRuntimePersistenceRoots(config)
			if err != nil {
				t.Fatal(err)
			}
			core, primary := runtimeReportPreservationAtRootsFixtureV1(t, roots, false, true, false)
			core.originalRegistryTrust, err = prepareRuntimeOriginalRegistryTrustV2(ctx, config, core.verification)
			if err != nil {
				t.Fatal(err)
			}
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := persistencefs.NewStartupSnapshotReader(roots).CaptureManagedSnapshotV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			digest := domainsecurity.SHA256Hex([]byte("associated original directory prefix"))
			baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
			if err != nil {
				t.Fatal(err)
			}
			root, err := persistencefs.FreezeRootAuthority(roots)
			if err != nil {
				t.Fatal(err)
			}
			journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(roots)
			if err != nil {
				t.Fatal(err)
			}
			builder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(roots, root, journal, preserved)
			prepared, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				for _, leaf := range runtimePrivateCASExpectedRoots(stage.DataDir, owner) {
					if err := os.MkdirAll(leaf, 0o700); err != nil {
						return err
					}
				}
				return os.WriteFile(filepath.Join(stage.DataDir, "private", "zz-associated-prefix-independent.bin"), []byte(`{"independent":true}`), 0o600)
			})
			if err != nil {
				t.Fatal(err)
			}
			defer prepared.Close()
			runtimeAssociatedRequireMarkerV1(t, prepared.Plan(), "data/private/zz-associated-prefix-independent.bin")
			ownerRoot := filepath.Join(roots.DataDir, "private", owner)
			cancelled, cancel := context.WithCancel(ctx)
			defer cancel()
			err = prepared.Apply(cancelAfterAssociatedOwnerCreateV1{Context: cancelled, cancel: cancel, owner: ownerRoot})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("did not reach signed physical directory cut: %v", err)
			}
			if _, err := os.Stat(ownerRoot); err != nil {
				t.Fatal("owner directory was not installed")
			}
			for _, leaf := range runtimePrivateCASExpectedRoots(roots.DataDir, owner) {
				if _, err := os.Stat(leaf); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("cut did not retain partial owner")
				}
			}
			retained := []string{filepath.Dir(primary)}
			for _, receipt := range core.pendingInventory.Receipts {
				for _, leaf := range []string{"receipts", "dispositions"} {
					retained = append(retained, filepath.Join(roots.DataDir, "private", "pending-work", leaf, receipt.WorkID[:2], receipt.WorkID+".json"))
				}
			}
			original := startupWholeTreeRecordMapForTest(t, retained...)
			for restart := 0; restart < 2; restart++ {
				handler, err := NewRuntimeServerHandlerE(config)
				if err != nil {
					t.Fatalf("actual root rejected independent signed owner prefix at restart %d: %v", restart, err)
				}
				shutdownOwnedRuntimeHandler(t, handler)
				if !reflect.DeepEqual(original, startupWholeTreeRecordMapForTest(t, retained...)) {
					t.Fatal("associated prefix recovery changed original UNKNOWN graph")
				}
			}
			if body, err := os.ReadFile(filepath.Join(roots.DataDir, "private", "zz-associated-prefix-independent.bin")); err != nil || string(body) != `{"independent":true}` {
				t.Fatal("independent signed Final was not recovered")
			}
		})
	}
}

func TestRuntimeOriginalAssociatedMissingDatasetCannotAppearThroughSemanticPlan(t *testing.T) {
	ctx := context.Background()
	core, _, _ := runtimeAssociatedOriginalFixtureV1(t)
	observed, err := datasetsnapshotstore.PrepareOriginalObservationV1(ctx, filepath.Join(core.roots.DataDir, "private", "dataset-snapshot-authority"), core.access)
	if err != nil {
		t.Fatal(err)
	}
	files, err := observed.SnapshotOriginalFilesV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	candidate := runtimeOriginalSemanticFilesV1(files)
	if _, unavailable, err := observeRuntimeAssociatedDomainV1(ctx, core, "dataset-snapshot-authority", candidate); err != nil || unavailable {
		t.Fatalf("late dataset fixture lacks valid full graph: %v", err)
	}
	// Remove only these enumerated synthetic fixture records. Empty directories
	// remain original; no historical/current dataset is invented by observation.
	for name, entry := range files {
		if !entry.Directory {
			if err := os.Remove(filepath.Join(core.roots.DataDir, "private", "dataset-snapshot-authority", name)); err != nil {
				t.Fatal(err)
			}
		}
	}
	preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	runtimeAssociatedExpectRejectionV1(t, core, preserved, func(stage startupport.PersistenceRootsV1) error {
		for name, entry := range files {
			if !entry.Directory {
				if err := os.WriteFile(filepath.Join(stage.DataDir, "private", "dataset-snapshot-authority", name), entry.Body, os.FileMode(entry.Mode)); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
