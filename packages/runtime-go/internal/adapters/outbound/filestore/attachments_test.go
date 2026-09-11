package filestore

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPersistentAttachmentStoreConstructorAndInvalidInputDoNotCreateDirectories(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	store, err := NewPersistentAttachmentStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dataDir, "attachments")
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("attachment constructor mutated storage: %v", err)
	}
	if _, err := store.Create(map[string]any{"mimeType": "text/plain"}); err == nil {
		t.Fatal("invalid attachment was accepted")
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid attachment created storage: %v", err)
	}
}

func TestPersistentAttachmentStorePersistsMetadataContentAndScope(t *testing.T) {
	store, err := NewPersistentAttachmentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	body := map[string]any{
		"name":          "note.txt",
		"mimeType":      "text/plain",
		"dataBase64":    base64.StdEncoding.EncodeToString([]byte("hello")),
		"threadId":      "thread_a",
		"workspace":     workspace,
		"localFilePath": filepath.Join(workspace, "note.txt"),
	}
	created, err := store.Create(body)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := created["id"].(string)
	if id == "" || created["scope"] != "thread" || created["localFilePath"] != filepath.Join(workspace, "note.txt") {
		t.Fatalf("attachment metadata mismatch: %#v", created)
	}

	metadata, found, err := store.Metadata(id)
	if err != nil {
		t.Fatal(err)
	}
	if !found || metadata["name"] != "note.txt" {
		t.Fatalf("metadata should persist: found=%v metadata=%#v", found, metadata)
	}
	contentMetadata, dataBase64, found, err := store.Content(id)
	if err != nil {
		t.Fatal(err)
	}
	if !found || contentMetadata["mimeType"] != "text/plain" || dataBase64 != body["dataBase64"] {
		t.Fatalf("content should persist: found=%v metadata=%#v data=%q", found, contentMetadata, dataBase64)
	}

	diagnostics := store.Diagnostics()
	if diagnostics["count"] != float64(1) || diagnostics["totalBytes"] != float64(5) {
		t.Fatalf("diagnostics mismatch: %#v", diagnostics)
	}
}

func TestPersistentAttachmentStoreRejectsContentHashMismatch(t *testing.T) {
	dataDir := t.TempDir()
	store, err := NewPersistentAttachmentStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create(map[string]any{
		"name": "evidence.txt", "mimeType": "text/plain",
		"dataBase64": base64.StdEncoding.EncodeToString([]byte("original")),
		"threadId":   "thread_integrity", "workspace": t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	id, _ := created["id"].(string)
	if err := os.WriteFile(store.contentPath(id), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.Content(id); !errors.Is(err, ErrAttachmentContentIntegrity) {
		t.Fatalf("tampered attachment content was accepted: %v", err)
	}
}

func TestAttachmentMetadataCollisionNeverDeletesPreexistingFile(t *testing.T) {
	store, err := NewPersistentAttachmentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.ensureWriteDirsNoLock(); err != nil {
		t.Fatal(err)
	}
	id := "att_000000000000000000000000"
	sentinel := []byte("preexisting metadata")
	if err := os.WriteFile(store.metadataPath(id), sentinel, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.contentPath(id), []byte("new transaction content"), 0o600); err != nil {
		t.Fatal(err)
	}
	metadataCreated, err := store.writeMetadataExclusiveNoLock(id, map[string]any{"id": id})
	if err == nil || metadataCreated {
		t.Fatalf("metadata collision was not classified before cleanup: created=%v err=%v", metadataCreated, err)
	}
	if err := store.removeUploadFilesNoLock(id, metadataCreated, true); err != nil {
		t.Fatal(err)
	}
	retained, err := os.ReadFile(store.metadataPath(id))
	if err != nil || string(retained) != string(sentinel) {
		t.Fatalf("cleanup deleted metadata owned by another transaction: body=%q err=%v", retained, err)
	}
	if _, err := os.Lstat(store.contentPath(id)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cleanup did not remove content created by this transaction: %v", err)
	}
}

func TestSameBlobHasDistinctCaseScopedAttachmentIDs(t *testing.T) {
	workspace := t.TempDir()
	writeCaseBindingFixture(t, workspace, map[string]any{
		"version": 1, "workspaceRoot": workspace, "caseId": "case_attachment_a", "source": "analytix-data-analysis",
	})
	store, err := NewPersistentAttachmentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{
		"name": "case.csv", "mimeType": "text/csv",
		"dataBase64": base64.StdEncoding.EncodeToString([]byte("amount\n1")),
		"threadId":   "thread_attachment", "workspace": workspace,
		"scope": "global",
	}
	created, err := store.Create(body)
	if err != nil {
		t.Fatal(err)
	}
	if created["scope"] != "thread" {
		t.Fatalf("caller widened attachment scope: %#v", created)
	}
	owner, ok := created["ownerRecord"].(map[string]any)
	if !ok {
		t.Fatalf("case-bound upload is missing its observed binding: %#v", created)
	}
	binding, _ := owner["caseBindingObservation"].(map[string]any)
	if binding["caseId"] != "case_attachment_a" {
		t.Fatalf("wrong case binding was persisted: %#v", owner)
	}

	writeCaseBindingFixture(t, workspace, map[string]any{
		"version": 1, "workspaceRoot": workspace, "caseId": "case_attachment_b", "source": "analytix-data-analysis",
	})
	reused, err := store.Create(body)
	if err != nil {
		t.Fatal(err)
	}
	if reused["id"] == created["id"] {
		t.Fatalf("same blob reused one owner id across cases: first=%#v second=%#v", created, reused)
	}
	reusedOwner, _ := reused["ownerRecord"].(map[string]any)
	reusedBinding, _ := reusedOwner["caseBindingObservation"].(map[string]any)
	if reusedBinding["caseId"] != "case_attachment_b" {
		t.Fatalf("second upload did not receive its own case owner: %#v", reused)
	}
}

func TestPersistentAttachmentStoreDetectsPNGAndRejectsMismatchedMime(t *testing.T) {
	store, err := NewPersistentAttachmentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data := pngHeader(16, 24)
	created, err := store.Create(map[string]any{
		"name":       "image.png",
		"mimeType":   "application/octet-stream",
		"dataBase64": base64.StdEncoding.EncodeToString(data),
		"threadId":   "thread_image",
		"workspace":  t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if created["mimeType"] != "image/png" || created["width"] != float64(16) || created["height"] != float64(24) {
		t.Fatalf("png metadata mismatch: %#v", created)
	}

	if _, err := store.Create(map[string]any{
		"name":       "bad.jpg",
		"mimeType":   "image/jpeg",
		"dataBase64": base64.StdEncoding.EncodeToString(data),
		"threadId":   "thread_image",
		"workspace":  t.TempDir(),
	}); err == nil {
		t.Fatal("expected mismatched declared MIME type to be rejected")
	}
}

func TestPersistentAttachmentStoreValidatesTextFallback(t *testing.T) {
	store, err := NewPersistentAttachmentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Create(map[string]any{
		"name":       "large.txt",
		"mimeType":   "text/plain",
		"dataBase64": base64.StdEncoding.EncodeToString([]byte("hello")),
		"threadId":   "thread_fallback",
		"workspace":  t.TempDir(),
		"textFallback": map[string]any{
			"mimeType":   "image/webp",
			"dataBase64": "not-base64",
		},
	})
	if err == nil {
		t.Fatal("expected invalid fallback dataBase64 to be rejected")
	}
}

func pngHeader(width int, height int) []byte {
	data := []byte{
		0x89, 0x50, 0x4e, 0x47,
		0x0d, 0x0a, 0x1a, 0x0a,
		0, 0, 0, 0x0d,
		'I', 'H', 'D', 'R',
		0, 0, 0, 0,
		0, 0, 0, 0,
	}
	data[16] = byte(width >> 24)
	data[17] = byte(width >> 16)
	data[18] = byte(width >> 8)
	data[19] = byte(width)
	data[20] = byte(height >> 24)
	data[21] = byte(height >> 16)
	data[22] = byte(height >> 8)
	data[23] = byte(height)
	return data
}
