package attachmentauthority_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	attachmentauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/attachmentauthority"
	"analytix.local/runtime-go/internal/adapters/outbound/filestore"
	attachmentauthorityapp "analytix.local/runtime-go/internal/app/attachmentauthority"
	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestAttachmentUploadCommitsAcrossPersistentStoreAndPrivateAuthority(t *testing.T) {
	workspace := t.TempDir()
	dataDir := t.TempDir()
	authority, attachments := persistentAttachmentStores(t, dataDir)
	service := &attachmentauthorityapp.Service{
		Threads:  attachmentThreadReader{thread: map[string]any{"id": "thread-a", "workspace": workspace}},
		Bindings: filestore.CaseBindingReader{}, Owners: authority, Uploads: authority,
	}
	metadata, err := service.Upload(context.Background(), map[string]any{
		"name": "statement.txt", "mimeType": "text/plain", "dataBase64": "ZXZpZGVuY2U=",
		"threadId": "thread-a", "workspace": workspace,
	}, attachments)
	if err != nil {
		t.Fatalf("persistent attachment upload failed: %v", err)
	}
	if metadata["id"] == "" {
		t.Fatalf("persistent attachment upload returned no id: %#v", metadata)
	}
}

func TestAttachmentUploadRejectsMissingWorkspaceBeforeIntentOrFiles(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "missing-workspace")
	authority, attachments := persistentAttachmentStores(t, dataDir)
	service := &attachmentauthorityapp.Service{
		Threads:  attachmentThreadReader{thread: map[string]any{"id": "thread-a", "workspace": workspace}},
		Bindings: filestore.CaseBindingReader{}, Owners: authority, Uploads: authority,
	}
	if _, err := service.Upload(context.Background(), map[string]any{
		"name": "statement.txt", "mimeType": "text/plain", "dataBase64": "ZXZpZGVuY2U=",
		"threadId": "thread-a", "workspace": workspace,
	}, attachments); !errors.Is(err, domainattachment.ErrUploadBindingUnsafe) {
		t.Fatalf("missing workspace classification = %v", err)
	}
	hasRecords, err := authority.HasRecords(context.Background())
	if err != nil || hasRecords {
		t.Fatalf("missing-workspace upload mutated private authority: hasRecords=%v err=%v", hasRecords, err)
	}
	if diagnostics := attachments.Diagnostics(); diagnostics["count"] != float64(0) || diagnostics["totalBytes"] != float64(0) {
		t.Fatalf("missing-workspace upload wrote attachment files: %#v", diagnostics)
	}
}

func persistentAttachmentStores(t *testing.T, dataDir string) (*attachmentauthorityadapter.Store, *filestore.PersistentAttachmentStore) {
	t.Helper()
	privateRoot := filepath.Join(dataDir, "private", "attachment-authority")
	access, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := attachmentauthorityadapter.NewStore(privateRoot, access)
	if err != nil {
		t.Fatal(err)
	}
	attachments, err := filestore.NewPersistentAttachmentStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	return authority, attachments
}

type attachmentThreadReader struct{ thread map[string]any }

func (reader attachmentThreadReader) GetThread(string) (map[string]any, error) {
	return reader.thread, nil
}
