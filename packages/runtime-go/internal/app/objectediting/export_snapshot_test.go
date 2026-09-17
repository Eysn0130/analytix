package objectediting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	fileport "analytix.local/runtime-go/internal/ports/objectediting"
)

func TestExportSnapshotBindsCurrentThreadDraftAndDiskWithoutWriting(t *testing.T) {
	_, identity, files := fixture(t)
	projector := &testSelectionProjector{}
	s := NewWithProjector(identity, files, projector)
	ctx := context.Background()
	opened, err := s.Open(ctx, "/workspace", "file.md")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := s.UpdateDraft(ctx, UpdateDraftInput{SessionID: opened.SessionID, BaseRevision: opened.Revision, Content: "未保存草稿"})
	if err != nil {
		t.Fatal(err)
	}
	input := ExportSnapshotInput{opened.SessionID, opened.ObjectID, "thread-1", opened.Revision, draft.Version}
	snapshot, err := s.ReadExportSnapshot(ctx, input)
	digest := sha256.Sum256([]byte(draft.Content))
	if err != nil || snapshot.Content != draft.Content || snapshot.ContentDigest != hex.EncodeToString(digest[:]) || snapshot.Workspace != "/workspace" || snapshot.Path != opened.Path {
		t.Fatalf("snapshot: %#v %v", snapshot, err)
	}
	if files.commits != 0 || files.doc.Content != "local original" || projector.calls != 0 || len(s.edits[opened.SessionID].scopes) != 0 {
		t.Fatal("export wrote or created a model scope")
	}
	for _, change := range []func(*ExportSnapshotInput){
		func(v *ExportSnapshotInput) { v.ThreadID = "other-thread" },
		func(v *ExportSnapshotInput) { v.ObjectID = strings.Repeat("d", 64) },
		func(v *ExportSnapshotInput) { v.DraftVersion = strings.Repeat("d", 48) },
		func(v *ExportSnapshotInput) { v.BaseRevision = strings.Repeat("d", 64) },
	} {
		bad := input
		change(&bad)
		if value, err := s.ReadExportSnapshot(ctx, bad); err == nil || value.Content != "" {
			t.Fatal("foreign binding exposed text")
		}
	}
	files.doc.Revision = strings.Repeat("e", 64)
	if _, err := s.ReadExportSnapshot(ctx, input); !errors.Is(err, fileport.ErrConflict) {
		t.Fatalf("external change: %v", err)
	}
	files.doc.Revision = opened.Revision
	files.onRead = func() { projector.denied = true }
	if _, err := s.ReadExportSnapshot(ctx, input); !errors.Is(err, ErrProjection) {
		t.Fatalf("revoked during read: %v", err)
	}
	files.onRead = nil
	projector.denied = false
	identity.unavailable = true
	if _, err := s.ReadExportSnapshot(ctx, input); err == nil {
		t.Fatal("stale principal accepted")
	}
	identity.unavailable = false
	if err := s.Close(ctx, opened.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadExportSnapshot(ctx, input); !errors.Is(err, ErrSession) {
		t.Fatalf("closed session: %v", err)
	}
}

func TestExportDraftCanRebaseAfterLaterSaveButRejectsStaleCASAndExternalDisk(t *testing.T) {
	s, _, files, _, opened, draft, binding, _ := proposalFixture(t)
	ctx := context.Background()
	input := ExportSnapshotInput{opened.SessionID, opened.ObjectID, "thread-1", opened.Revision, draft.Version}
	if _, err := s.ReadExportSnapshot(ctx, input); err != nil {
		t.Fatal(err)
	}
	receipt, err := s.Commit(ctx, opened.SessionID, "save-next", opened.Revision, "saved B")
	if err != nil {
		t.Fatal(err)
	}
	update := UpdateDraftInput{SessionID: opened.SessionID, BaseRevision: receipt.Revision, ExpectedVersion: draft.Version, Content: "saved B"}
	next, err := s.UpdateDraft(ctx, update)
	if err != nil || next.BaseRevision != receipt.Revision || next.Version == draft.Version {
		t.Fatalf("rebase: %#v %v", next, err)
	}
	input.BaseRevision = next.BaseRevision
	input.DraftVersion = next.Version
	if got, err := s.ReadExportSnapshot(ctx, input); err != nil || got.Content != "saved B" {
		t.Fatalf("second export: %v", err)
	}
	if scope, err := s.ReadScope(ctx, binding); err != nil || scope.Current {
		t.Fatalf("old scope remained current: %v", err)
	}
	if _, err := s.UpdateDraft(ctx, update); !errors.Is(err, ErrDraftStale) {
		t.Fatal("stale CAS accepted")
	}
	update.ExpectedVersion = next.Version
	update.Content = "retained C"
	files.doc.Revision = strings.Repeat("f", 64)
	if _, err := s.UpdateDraft(ctx, update); !errors.Is(err, ErrDraftStale) {
		t.Fatal("external disk accepted")
	}
	got, _ := s.ReadDraft(ctx, opened.SessionID)
	if got != next {
		t.Fatal("failed capture replaced the previous draft")
	}
}
