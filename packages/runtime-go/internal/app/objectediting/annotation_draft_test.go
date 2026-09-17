package objectediting

import (
	"context"
	"errors"
	"strings"
	"testing"

	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
)

type annotationTestFiles struct {
	*testFiles
	seen            fileport.AnnotationDraftTarget
	write           fileport.AnnotationDraftWriteInput
	result          fileport.AnnotationDraft
	onAnnotation    func()
	annotationCalls int
}

func (f *annotationTestFiles) ReadAnnotationDraft(_ context.Context, target fileport.AnnotationDraftTarget) (fileport.AnnotationDraft, error) {
	f.seen = target
	f.annotationCalls++
	if f.onAnnotation != nil {
		f.onAnnotation()
	}
	out := f.result
	out.ObjectID = target.ObjectIdentity
	out.ThreadID = target.ThreadID
	return out, nil
}
func (f *annotationTestFiles) WriteAnnotationDraft(ctx context.Context, in fileport.AnnotationDraftWriteInput) (fileport.AnnotationDraft, error) {
	f.write = in
	f.result = fileport.AnnotationDraft{Note: in.Note, SourceRevision: in.SourceRevision, DraftRevision: strings.Repeat("e", 64), UpdatedAt: "2026-09-15T00:00:00Z"}
	return f.ReadAnnotationDraft(ctx, in.AnnotationDraftTarget)
}

func TestAnnotationDraftServiceBindsCurrentCoreSession(t *testing.T) {
	_, identity, base := fixture(t)
	files := &annotationTestFiles{testFiles: base}
	s := New(identity, files)
	ctx := context.Background()
	opened, err := s.Open(ctx, "/workspace", "file.md")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := s.WriteAnnotationDraft(ctx, opened.SessionID, "thread-a", "", "private note", strings.Repeat("f", 64))
	if err != nil || draft.Note != "private note" || files.seen.ObjectIdentity != opened.ObjectID || files.seen.Path != base.doc.Path || files.seen.Workspace != base.doc.Workspace || files.seen.ThreadID != "thread-a" || base.commits != 0 {
		t.Fatal("bound write", draft, files.seen, err)
	}
	restarted := New(identity, files)
	if _, err := restarted.ReadAnnotationDraft(ctx, opened.SessionID, "thread-a"); !errors.Is(err, ErrSession) {
		t.Fatal("old session", err)
	}
	reopened, err := restarted.Open(ctx, "/workspace", "file.md")
	if err != nil {
		t.Fatal(err)
	}
	got, err := restarted.ReadAnnotationDraft(ctx, reopened.SessionID, "thread-a")
	if err != nil || got != draft {
		t.Fatal("rebind", got, err)
	}
	identity.principal, _ = identitydomain.NewPrincipalV1(strings.Repeat("d", 64), "local", "local")
	if got, err := restarted.ReadAnnotationDraft(ctx, reopened.SessionID, "thread-a"); !errors.Is(err, ErrSession) || got.Note != "" {
		t.Fatal("revoked identity", got, err)
	}
}

func TestAnnotationDraftServiceRevalidatesBeforeReturningText(t *testing.T) {
	for _, write := range []bool{false, true} {
		_, identity, base := fixture(t)
		files := &annotationTestFiles{testFiles: base, result: fileport.AnnotationDraft{Note: "private note"}}
		s := New(identity, files)
		opened, err := s.Open(context.Background(), "/workspace", "file.md")
		if err != nil {
			t.Fatal(err)
		}
		files.onAnnotation = func() { identity.unavailable = true }
		var got fileport.AnnotationDraft
		if write {
			got, err = s.WriteAnnotationDraft(context.Background(), opened.SessionID, "thread-a", "", "private note", strings.Repeat("f", 64))
		} else {
			got, err = s.ReadAnnotationDraft(context.Background(), opened.SessionID, "thread-a")
		}
		if !errors.Is(err, ErrUnavailable) || got.Note != "" {
			t.Fatal("late identity change disclosed note", got, err)
		}
	}
}
