//go:build darwin || linux

package runtimeapp

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	privatecasrecoverytest "analytix.local/runtime-go/internal/testsupport/privatecasrecovery"
)

func TestRuntimeAttachmentPrivateCASRecoveryPreservesOriginalResidue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	core, intent, _ := runtimeHeldAttachmentIntentFixtureV1(t)
	residue := filepath.Join(core.roots.DataDir, "private", "attachment-authority", "upload-intents", intent.UploadID[:2], "."+intent.UploadID+".json-held.tmp")
	if err := os.WriteFile(residue, []byte("opaque original held upload residue"), 0o600); err != nil {
		t.Fatal(err)
	}
	preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	var selected []runtimePrivateCASOwnerRecovery
	for _, owner := range runtimePrivateCASOwnerRecoveries(core.roots.DataDir, core.access) {
		if owner.name == "attachment-authority" {
			selected = append(selected, owner)
		}
	}
	prepared, err := prepareAndValidateRuntimePrivateCASOwners(ctx, selected, true, nil, preserved)
	if err != nil || len(prepared) != 1 {
		t.Fatalf("attachment recovery preparation: %v", err)
	}
	before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	if err := privatecasrecoverytest.ApplyV4(ctx, "original-held-attachment", prepared[0]); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
		t.Fatal("private CAS recovery changed original held attachment residue or graph")
	}
	store, err := preserved.OpenAttachmentRestartStoreV1(ctx, filepath.Join(core.roots.DataDir, "private", "attachment-authority"), core.access)
	if err != nil {
		t.Fatal(err)
	}
	if found, err := store.HasRecords(ctx); err != nil || !found {
		t.Fatalf("original residue prevented read-only runtime activation: %v", err)
	}
	if got, err := store.ResolveUploadIntent(ctx, intent.UploadID); err != nil || !reflect.DeepEqual(got, intent) {
		t.Fatalf("original held intent read after recovery: %v", err)
	}
}

func TestRuntimeAttachmentRestartWritesIndependentBesidePreservedResidue(t *testing.T) {
	for _, kind := range []string{"plain", "linked"} {
		t.Run(kind, func(t *testing.T) { testRuntimeAttachmentIndependentResidueV1(t, kind == "linked") })
	}
}

func testRuntimeAttachmentIndependentResidueV1(t *testing.T, linked bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	core, held, _ := runtimeHeldAttachmentIntentFixtureV1(t)
	residue := filepath.Join(core.roots.DataDir, "private", "attachment-authority", "upload-intents", held.UploadID[:2], "."+held.UploadID+".json-held.tmp")
	var residueErr error
	if linked {
		residueErr = os.Link(filepath.Join(filepath.Dir(residue), held.UploadID+".json"), residue)
	} else {
		residueErr = os.WriteFile(residue, []byte("immutable original held residue"), 0o600)
	}
	if residueErr != nil {
		t.Fatal(residueErr)
	}
	preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	// Startup retires the pre-recovery live generations before opening the
	// preserved owner. An existing strict generation must never be upgraded by
	// merely discovering a residue through a second constructor.
	var selected []runtimePrivateCASOwnerRecovery
	for _, owner := range runtimePrivateCASOwnerRecoveries(core.roots.DataDir, core.access) {
		if owner.name == "attachment-authority" {
			selected = append(selected, owner)
		}
	}
	prepared, err := prepareAndValidateRuntimePrivateCASOwners(ctx, selected, true, nil, preserved)
	if err != nil || len(prepared) != 1 {
		t.Fatalf("attachment recovery preparation: %v", err)
	}
	if err := privatecasrecoverytest.ApplyV4(ctx, "independent-beside-held-attachment", prepared[0]); err != nil {
		t.Fatal(err)
	}
	store, err := preserved.OpenAttachmentRestartStoreV1(ctx, filepath.Join(core.roots.DataDir, "private", "attachment-authority"), core.access)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := domainattachment.NewOwnerRecordV1(domainattachment.OwnerRecordInputV1{OwnerNonce: "00000000000000000000000000000002", BlobSHA256: held.Owner.BlobSHA256, ByteSize: held.Owner.ByteSize, MIMEType: held.Owner.MIMEType, ThreadID: "thread-independent", WorkspaceRealPath: held.Owner.WorkspaceRealPath, ProjectionSHA256: held.Owner.ProjectionSHA256, CreatedAt: time.Unix(1, 0).UTC()})
	if err != nil {
		t.Fatal(err)
	}
	intent, err := domainattachment.NewUploadIntentV1(owner, held.MetadataSHA256)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(residue)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutUploadIntentIfAbsent(ctx, intent); err != nil {
		t.Fatalf("preserved residue blocked independent same-owner write: %v", err)
	}
	if got, err := store.ResolveUploadIntent(ctx, intent.UploadID); err != nil || !reflect.DeepEqual(got, intent) {
		t.Fatalf("independent intent readback: %v", err)
	}
	after, err := os.ReadFile(residue)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("independent write changed held residue: %v", err)
	}
}

func TestRuntimeAttachmentOriginalEmptyShardSurvivesOrphanRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	core, heldIntent, _ := runtimeHeldAttachmentIntentFixtureV1(t)
	attachmentRoot := filepath.Join(core.roots.DataDir, "private", "attachment-authority")
	held := filepath.Join(attachmentRoot, "owners", "ab")
	if err := os.Mkdir(held, 0o700); err != nil {
		t.Fatal(err)
	}
	independent := filepath.Join(core.roots.DataDir, "private", "case-thread-authority", "ac")
	if err := os.MkdirAll(independent, 0o700); err != nil {
		t.Fatal(err)
	}
	preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	before := startupWholeTreeRecordMapForTest(t, attachmentRoot, core.roots.DurableDir)
	if err := recoverRuntimePrivateCASOrphanTopology(ctx, core.roots.DataDir, core.access, preserved); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, attachmentRoot, core.roots.DurableDir)) {
		t.Fatal("early orphan recovery deleted original held attachment topology before its later guard")
	}
	if _, err := os.Lstat(independent); !os.IsNotExist(err) {
		t.Fatalf("independent orphan recovery was blocked: %v", err)
	}
	if err := preserved.validateAttachmentSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	var selected []runtimePrivateCASOwnerRecovery
	for _, owner := range runtimePrivateCASOwnerRecoveries(core.roots.DataDir, core.access) {
		if owner.name == "attachment-authority" {
			selected = append(selected, owner)
		}
	}
	prepared, err := prepareAndValidateRuntimePrivateCASOwners(ctx, selected, true, nil, preserved)
	if err != nil || len(prepared) != 1 {
		t.Fatalf("original empty-shard body preparation: %v", err)
	}
	if err := privatecasrecoverytest.ApplyV4(ctx, "original-empty-attachment-shard", prepared[0]); err != nil {
		t.Fatal(err)
	}
	store, err := preserved.OpenAttachmentRestartStoreV1(ctx, attachmentRoot, core.access)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := domainattachment.NewOwnerRecordV1(domainattachment.OwnerRecordInputV1{OwnerNonce: "00000000000000000000000000000003", BlobSHA256: heldIntent.Owner.BlobSHA256, ByteSize: heldIntent.Owner.ByteSize, MIMEType: heldIntent.Owner.MIMEType, ThreadID: "thread-independent", WorkspaceRealPath: heldIntent.Owner.WorkspaceRealPath, ProjectionSHA256: heldIntent.Owner.ProjectionSHA256, CreatedAt: time.Unix(1, 0).UTC()})
	if err != nil {
		t.Fatal(err)
	}
	intent, err := domainattachment.NewUploadIntentV1(owner, heldIntent.MetadataSHA256)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutUploadIntentIfAbsent(ctx, intent); err != nil {
		t.Fatalf("original empty shard blocked independent same-owner write: %v", err)
	}
	if got, err := store.ResolveUploadIntent(ctx, intent.UploadID); err != nil || !reflect.DeepEqual(got, intent) {
		t.Fatalf("independent write readback beside original empty shard: %v", err)
	}
	if info, err := os.Stat(held); err != nil || !info.IsDir() {
		t.Fatalf("independent write lost original empty shard: %v", err)
	}
}
