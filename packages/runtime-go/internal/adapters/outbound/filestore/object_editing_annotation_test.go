//go:build darwin || linux

package filestore

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	editing "analytix.local/runtime-go/internal/ports/objectediting"
)

func annotationFixture(t *testing.T, kind string) (*ObjectEditingFiles, editing.AnnotationDraftWriteInput) {
	t.Helper()
	s, workspace, path := officeEditingNativeFixture(t, kind, officeEditingNativeBytes(t, kind, "Original"))
	return s, editing.AnnotationDraftWriteInput{AnnotationDraftTarget: editing.AnnotationDraftTarget{Workspace: workspace, Path: path, ObjectIdentity: strings.Repeat("a", 64), ThreadID: "thread-a"}, Note: "保留备注 😀", SourceRevision: strings.Repeat("b", 64)}
}

func TestAnnotationDraftRestartIsolationCASAndClear(t *testing.T) {
	for _, kind := range []string{"docx", "xlsx", "pptx"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			s, in := annotationFixture(t, kind)
			before, _ := os.ReadFile(in.Path)
			empty, err := s.ReadAnnotationDraft(ctx, in.AnnotationDraftTarget)
			if err != nil || empty != (editing.AnnotationDraft{ObjectID: in.ObjectIdentity, ThreadID: in.ThreadID}) {
				t.Fatal("initial", empty, err)
			}
			saved, err := s.WriteAnnotationDraft(ctx, in)
			if err != nil || saved.DraftRevision == "" || saved.Note != in.Note || saved.SourceRevision != in.SourceRevision || saved.UpdatedAt == "" {
				t.Fatal("write", saved, err)
			}
			body, _ := os.ReadFile(s.annotationPath(in.ObjectIdentity, in.ThreadID))
			if saved.DraftRevision != digestAtomicText(body) {
				t.Fatal("revision is not the exact private record hash")
			}
			restarted, err := NewOfficeObjectEditingFiles(s.receiptRoot, nil, kind)
			if err != nil {
				t.Fatal(err)
			}
			read, err := restarted.ReadAnnotationDraft(ctx, in.AnnotationDraftTarget)
			if err != nil || read != saved {
				t.Fatal("restart", read, err)
			}
			for _, different := range []editing.AnnotationDraftTarget{
				{Workspace: in.Workspace, Path: in.Path, ObjectIdentity: in.ObjectIdentity, ThreadID: "thread-b"},
				{Workspace: in.Workspace, Path: in.Path, ObjectIdentity: strings.Repeat("c", 64), ThreadID: in.ThreadID},
			} {
				other, err := restarted.ReadAnnotationDraft(ctx, different)
				if err != nil || other.Note != "" || other.DraftRevision != "" {
					t.Fatal("isolation", other, err)
				}
			}
			writes := 0
			restarted.replaceJournal = func(r atomicTextReplaceRequest) error { writes++; return atomicReplaceText(r) }
			again, err := restarted.WriteAnnotationDraft(ctx, in)
			if err != nil || again != saved || writes != 0 {
				t.Fatal("lost ack replay wrote", again, err, writes)
			}
			stale := in
			stale.Note = "stale replacement"
			if _, err := restarted.WriteAnnotationDraft(ctx, stale); !errors.Is(err, editing.ErrConflict) {
				t.Fatal("CAS", err)
			}
			clear := in
			clear.ExpectedDraftRevision = saved.DraftRevision
			clear.Note = ""
			cleared, err := restarted.WriteAnnotationDraft(ctx, clear)
			if err != nil || cleared.Note != "" || cleared.DraftRevision == "" || cleared.DraftRevision == saved.DraftRevision {
				t.Fatal("clear", cleared, err)
			}
			if _, err := restarted.WriteAnnotationDraft(ctx, in); !errors.Is(err, editing.ErrConflict) {
				t.Fatal("old write revived cleared note", err)
			}
			if again, err := restarted.WriteAnnotationDraft(ctx, clear); err != nil || again != cleared {
				t.Fatal("clear replay", again, err)
			}
			officeEditingAssertBytes(t, in.Path, before)
		})
	}
}

func TestAnnotationDraftLostWriteAcknowledgement(t *testing.T) {
	s, in := annotationFixture(t, "docx")
	writes := 0
	s.replaceJournal = func(r atomicTextReplaceRequest) error {
		writes++
		if err := atomicReplaceText(r); err != nil {
			return err
		}
		return errors.New("synthetic lost acknowledgement")
	}
	if got, err := s.WriteAnnotationDraft(context.Background(), in); !errors.Is(err, editing.ErrPersistence) || got.Note != "" {
		t.Fatal("uncertain result", got, err)
	}
	got, err := s.WriteAnnotationDraft(context.Background(), in)
	if err != nil || got.Note != in.Note || writes != 1 {
		t.Fatal("replay", got, err, writes)
	}
}

func TestAnnotationDraftRejectsPrivateTampering(t *testing.T) {
	for _, mode := range []string{"hardlink", "symlink", "permissions", "extra-key", "path-binding", "oversize", "root"} {
		t.Run(mode, func(t *testing.T) {
			s, in := annotationFixture(t, "docx")
			if _, err := s.WriteAnnotationDraft(context.Background(), in); err != nil {
				t.Fatal(err)
			}
			path := s.annotationPath(in.ObjectIdentity, in.ThreadID)
			body, _ := os.ReadFile(path)
			var err error
			switch mode {
			case "hardlink":
				err = os.Link(path, path+".link")
			case "symlink":
				if err = os.Rename(path, path+".original"); err == nil {
					err = os.Symlink(path+".original", path)
				}
			case "permissions":
				err = os.Chmod(path, 0644)
			case "extra-key":
				err = os.WriteFile(path, append(body[:len(body)-1], []byte(`,"token":"old-token"}`)...), 0600)
			case "path-binding":
				err = os.WriteFile(path, []byte(strings.Replace(string(body), `"pathBinding":"`, `"pathBinding":"bad`, 1)), 0600)
			case "oversize":
				err = os.WriteFile(path, []byte(strings.Repeat("x", editing.MaxAnnotationRecordBytes+1)), 0600)
			case "root":
				err = os.Chmod(s.receiptRoot, 0755)
			}
			if err != nil {
				t.Fatal(err)
			}
			if got, err := s.ReadAnnotationDraft(context.Background(), in.AnnotationDraftTarget); err == nil || got.Note != "" {
				t.Fatal("tampered read disclosed note", got, err)
			}
			if got, err := s.WriteAnnotationDraft(context.Background(), in); err == nil || got.Note != "" {
				t.Fatal("tampered write accepted", got, err)
			}
		})
	}
}

func TestAnnotationDraftUnicodeAndEscapedSize(t *testing.T) {
	for _, note := range []string{strings.Repeat("中", 4096), strings.Repeat("😀", 2048), strings.Repeat("\x00", 4096)} {
		s, in := annotationFixture(t, "docx")
		in.Note = note
		got, err := s.WriteAnnotationDraft(context.Background(), in)
		if err != nil || got.Note != note {
			t.Fatal("valid boundary", err)
		}
	}
	for _, note := range []string{strings.Repeat("中", 4097), strings.Repeat("😀", 2048) + "a", string([]byte{0xff})} {
		s, in := annotationFixture(t, "docx")
		in.Note = note
		if _, err := s.WriteAnnotationDraft(context.Background(), in); !errors.Is(err, editing.ErrInvalidInput) {
			t.Fatal("invalid boundary", err)
		}
	}
}
