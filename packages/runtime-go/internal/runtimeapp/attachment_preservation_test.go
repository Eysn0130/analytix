//go:build darwin || linux

package runtimeapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	attachmentport "analytix.local/runtime-go/internal/ports/attachmentauthority"
)

type runtimeAttachmentRestartFactoryForTestV1 interface {
	OpenAttachmentRestartStoreV1(context.Context, string, finalauthority.SecurePrivateCASRecoveryAccessAuthority) (attachmentport.UploadInventoryStore, error)
}

func TestRuntimeAttachmentRestartFactoryRetainsOriginalAbsence(t *testing.T) {
	ctx := context.Background()
	core, _ := runtimeReportPreservationFixtureV1(t, false, true, false)
	preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	factory, ok := any(preserved).(runtimeAttachmentRestartFactoryForTestV1)
	if !ok {
		t.Fatal("runtime attachment original store factory is unavailable")
	}
	root := filepath.Join(core.roots.DataDir, "private", "attachment-authority")
	before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	store, err := factory.OpenAttachmentRestartStoreV1(ctx, root, core.access)
	if err != nil {
		t.Fatal(err)
	}
	if found, err := store.HasRecords(ctx); err != nil || found {
		t.Fatalf("absent original attachment inventory: %v", err)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("original attachment activation created absent owner")
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
		t.Fatal("original attachment activation changed files")
	}
}

func TestRuntimeAttachmentRestartFactoryPreservesHeldWritesAndIndependentUpload(t *testing.T) {
	ctx := context.Background()
	core, held, _ := runtimeHeldAttachmentIntentFixtureV1(t)
	preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	factory, ok := any(preserved).(runtimeAttachmentRestartFactoryForTestV1)
	if !ok {
		t.Fatal("runtime attachment original store factory is unavailable")
	}
	store, err := factory.OpenAttachmentRestartStoreV1(ctx, filepath.Join(core.roots.DataDir, "private", "attachment-authority"), core.access)
	if err != nil {
		t.Fatal(err)
	}
	guard, ok := store.(attachmentport.RestartPreservationV1)
	if !ok || !guard.RestartPreservesThreadV1(held.Owner.ThreadID) {
		t.Fatal("actual runtime store lost original denial scope")
	}
	closed, err := domainattachment.NewUploadDispositionV1(held, domainattachment.UploadDispositionQuarantinedV1, "restart_missing_upload_files", "", "", time.Unix(2, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	for _, write := range []func() error{
		func() error { return store.PutOwnerIfAbsent(ctx, held.Owner) },
		func() error { return store.CommitOwnerForOpenUpload(ctx, held) },
		func() error { return store.PutUploadIntentIfAbsent(ctx, held) },
		func() error { return store.PutUploadDispositionIfAbsent(ctx, closed) },
	} {
		if err := write(); !errors.Is(err, attachmentport.ErrRestartPreserved) {
			t.Fatalf("held direct write cause: %v", err)
		}
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
		t.Fatal("held direct write changed original bytes")
	}
	owner, err := domainattachment.NewOwnerRecordV1(domainattachment.OwnerRecordInputV1{OwnerNonce: "00000000000000000000000000000002", BlobSHA256: held.Owner.BlobSHA256, ByteSize: held.Owner.ByteSize, MIMEType: held.Owner.MIMEType, ThreadID: "thread-independent", WorkspaceRealPath: held.Owner.WorkspaceRealPath, ProjectionSHA256: held.Owner.ProjectionSHA256, CreatedAt: time.Unix(1, 0).UTC()})
	if err != nil {
		t.Fatal(err)
	}
	intent, err := domainattachment.NewUploadIntentV1(owner, held.MetadataSHA256)
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := domainattachment.NewUploadDispositionV1(intent, domainattachment.UploadDispositionQuarantinedV1, "restart_missing_upload_files", "", "", time.Unix(2, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutUploadDispositionIfAbsent(ctx, disposition); !errors.Is(err, attachmentport.ErrCorrupt) || !errors.Is(err, attachmentport.ErrNotFound) {
		t.Fatalf("missing disposition parent lost corruption cause: %v", err)
	}
	if err := store.PutUploadIntentIfAbsent(ctx, intent); err != nil {
		t.Fatal(err)
	}
	if err := store.PutUploadDispositionIfAbsent(ctx, disposition); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveUploadDisposition(ctx, held.UploadID); !errors.Is(err, attachmentport.ErrNotFound) {
		t.Fatalf("held upload acquired a disposition: %v", err)
	}
	if got, err := store.ResolveUploadDisposition(ctx, intent.UploadID); err != nil || got.RecordDigest != disposition.RecordDigest {
		t.Fatalf("independent upload did not persist: %v", err)
	}
}
