package objectediting

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
)

type nativeRecoveryTestFiles struct {
	*testFiles
	prepare  fileport.NativeChangeInput
	commit   fileport.NativeCommitInput
	undo     fileport.NativeUndoInput
	resume   fileport.NativeUndoInput
	query    []string
	calls    int
	onNative func()
}

func (f *nativeRecoveryTestFiles) observed() {
	f.calls++
	if f.onNative != nil {
		f.onNative()
	}
}
func (f *nativeRecoveryTestFiles) PrepareNativeChange(_ context.Context, in fileport.NativeChangeInput) (fileport.NativeChangeStatus, error) {
	f.prepare = in
	f.observed()
	return fileport.NativeChangeStatus{ChangeID: in.Draft.ChangeID, BeforeText: "private-original"}, nil
}
func (f *nativeRecoveryTestFiles) NativeRecovery(_ context.Context, object, workspace, path, thread string) (fileport.NativeRecovery, error) {
	f.query = []string{object, workspace, path, thread}
	f.observed()
	return fileport.NativeRecovery{Current: &fileport.NativeChangeStatus{BeforeText: "private-original"}}, nil
}
func (f *nativeRecoveryTestFiles) CommitNativeChange(_ context.Context, in fileport.NativeCommitInput) (fileport.Receipt, error) {
	f.commit = in
	f.doc.Revision = f.receipt.Revision
	f.observed()
	return f.receipt, nil
}
func (f *nativeRecoveryTestFiles) UndoNativeChange(_ context.Context, in fileport.NativeUndoInput) (fileport.Receipt, error) {
	f.undo = in
	f.doc.Revision = f.receipt.Revision
	f.observed()
	return f.receipt, nil
}
func (f *nativeRecoveryTestFiles) ResumeNativeChange(_ context.Context, in fileport.NativeUndoInput) (fileport.Receipt, error) {
	f.resume = in
	f.doc.Revision = f.receipt.Revision
	f.observed()
	return f.receipt, nil
}
func (f *nativeRecoveryTestFiles) CancelNativeChange(_ context.Context, in fileport.NativeUndoInput) (fileport.NativeRecovery, error) {
	f.undo = in
	f.observed()
	return fileport.NativeRecovery{Current: &fileport.NativeChangeStatus{BeforeText: "private-original"}}, nil
}
func nativeRecoveryServiceFixture(t *testing.T) (*Service, *testIdentity, *nativeRecoveryTestFiles, Opened) {
	t.Helper()
	_, identity, base := fixture(t)
	files := &nativeRecoveryTestFiles{testFiles: base}
	s := New(identity, files)
	opened, err := s.Open(context.Background(), "/workspace", "file.md")
	if err != nil {
		t.Fatal(err)
	}
	return s, identity, files, opened
}
func nativeRecoveryDraft() fileport.NativeChangeDraft {
	return fileport.NativeChangeDraft{ChangeID: strings.Repeat("e", 64), ThreadID: "thread_main", ProposalID: strings.Repeat("f", 48), BaseRevision: strings.Repeat("b", 64), BeforeText: "original", AfterText: "replacement"}
}

func TestNativeRecoveryServiceUsesSessionAuthorityAndReopens(t *testing.T) {
	s, identity, files, opened := nativeRecoveryServiceFixture(t)
	ctx := context.Background()
	draft := nativeRecoveryDraft()
	if _, err := s.PrepareNativeChange(ctx, opened.SessionID, draft); err != nil {
		t.Fatal(err)
	}
	if files.prepare.Workspace != "/workspace" || files.prepare.Path != opened.Path || files.prepare.ObjectIdentity != opened.ObjectID || files.prepare.Draft != draft {
		t.Fatal("prepare lost session authority", files.prepare)
	}
	if _, err := s.NativeRecovery(ctx, opened.SessionID, "thread_main"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(files.query, []string{opened.ObjectID, "/workspace", opened.Path, "thread_main"}) {
		t.Fatal("recovery binding changed", files.query)
	}
	if _, err := s.CommitNativeChange(ctx, opened.SessionID, "thread_main", draft.ChangeID, "native_save_fixed", draft.BaseRevision, "new native bytes"); err != nil {
		t.Fatal(err)
	}
	if files.commit.Path != opened.Path || files.commit.Workspace != "/workspace" || files.commit.ObjectIdentity != opened.ObjectID || files.commit.ChangeID != draft.ChangeID || files.commit.OperationID != "native_save_fixed" || files.commit.ThreadID != "thread_main" || files.commit.Content != "new native bytes" {
		t.Fatal("native commit binding changed", files.commit)
	}
	if _, err := s.UndoNativeChange(ctx, opened.SessionID, "thread_main", draft.ChangeID, files.receipt.Revision); err != nil {
		t.Fatal(err)
	}
	if files.undo.Path != opened.Path || files.undo.ObjectIdentity != opened.ObjectID || files.undo.ThreadID != "thread_main" || files.undo.ChangeID != draft.ChangeID {
		t.Fatal("undo lost Core target", files.undo)
	}
	if _, err := s.ResumeNativeChange(ctx, opened.SessionID, "thread_main", draft.ChangeID, opened.Revision); err != nil {
		t.Fatal(err)
	}
	if files.resume.Path != opened.Path || files.resume.Workspace != "/workspace" || files.resume.ObjectIdentity != opened.ObjectID || files.resume.ThreadID != "thread_main" || files.resume.ChangeID != draft.ChangeID || files.resume.BaseRevision != opened.Revision {
		t.Fatal("resume lost Core target authority", files.resume)
	}
	if files.commits != 0 {
		t.Fatal("native operation used generic commit")
	}
	restarted := New(identity, files)
	if _, err := restarted.NativeRecovery(ctx, opened.SessionID, "thread_main"); !errors.Is(err, ErrSession) {
		t.Fatal("old session resumed", err)
	}
	reopened, err := restarted.Open(ctx, "/workspace", "file.md")
	if err != nil || reopened.SessionID == opened.SessionID || reopened.ObjectID != opened.ObjectID {
		t.Fatal("reopen did not rebind", err)
	}
	if _, err := restarted.NativeRecovery(ctx, reopened.SessionID, "thread_main"); err != nil {
		t.Fatal(err)
	}
}

func TestNativeRecoveryServiceMissingOwnerAndRevokedSessionFailClosed(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []string{"missing owner", "revoked principal", "closed"} {
		t.Run(mode, func(t *testing.T) {
			s, identity, files, opened := nativeRecoveryServiceFixture(t)
			want := ErrSession
			if mode == "missing owner" {
				s.files = files.testFiles
				want = ErrUnavailable
			}
			if mode == "revoked principal" {
				identity.principal, _ = identitydomain.NewPrincipalV1(strings.Repeat("e", 64), "local", "local")
			}
			if mode == "closed" {
				if err := s.Close(ctx, opened.SessionID); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.PrepareNativeChange(ctx, opened.SessionID, nativeRecoveryDraft()); !errors.Is(err, want) {
				t.Fatal(err)
			}
			if _, err := s.NativeRecovery(ctx, opened.SessionID, "thread_main"); !errors.Is(err, want) {
				t.Fatal(err)
			}
			if _, err := s.CommitNativeChange(ctx, opened.SessionID, "thread_main", strings.Repeat("e", 64), "native_save_fixed", opened.Revision, "bytes"); !errors.Is(err, want) {
				t.Fatal(err)
			}
			if _, err := s.UndoNativeChange(ctx, opened.SessionID, "thread_main", strings.Repeat("e", 64), opened.Revision); !errors.Is(err, want) {
				t.Fatal(err)
			}
			if _, err := s.ResumeNativeChange(ctx, opened.SessionID, "thread_main", strings.Repeat("e", 64), opened.Revision); !errors.Is(err, want) {
				t.Fatal(err)
			}
			if _, err := s.CancelNativeChange(ctx, opened.SessionID, "thread_main", strings.Repeat("e", 64), opened.Revision); !errors.Is(err, want) {
				t.Fatal(err)
			}
			if files.calls != 0 || files.commits != 0 {
				t.Fatal("unavailable authority reached storage")
			}
		})
	}
}

func TestNativeRecoveryServiceRevalidatesIdentityAfterStorage(t *testing.T) {
	for _, operation := range []string{"prepare", "recovery", "commit", "undo", "cancel", "resume"} {
		t.Run(operation, func(t *testing.T) {
			s, identity, files, opened := nativeRecoveryServiceFixture(t)
			ctx := context.Background()
			files.onNative = func() { identity.unavailable = true }
			switch operation {
			case "cancel":
				out, err := s.CancelNativeChange(ctx, opened.SessionID, "thread_main", strings.Repeat("e", 64), opened.Revision)
				if !errors.Is(err, ErrUnavailable) || out.Current != nil || out.Pending != nil {
					t.Fatal("private cancellation result escaped revoked identity", err)
				}
			case "prepare":
				out, err := s.PrepareNativeChange(ctx, opened.SessionID, nativeRecoveryDraft())
				if !errors.Is(err, ErrUnavailable) || out != (fileport.NativeChangeStatus{}) {
					t.Fatal("private prepared data escaped revoked identity", out, err)
				}
			case "recovery":
				out, err := s.NativeRecovery(ctx, opened.SessionID, "thread_main")
				if !errors.Is(err, ErrUnavailable) || out.Current != nil || out.Pending != nil {
					t.Fatal("private recovery escaped revoked identity", err)
				}
			case "commit":
				out, err := s.CommitNativeChange(ctx, opened.SessionID, "thread_main", strings.Repeat("e", 64), "native_save_fixed", opened.Revision, "bytes")
				if !errors.Is(err, fileport.ErrPersistence) || out.Status != fileport.StatusUnknown {
					t.Fatal("commit confirmed after identity loss", out, err)
				}
			case "resume":
				out, err := s.ResumeNativeChange(ctx, opened.SessionID, "thread_main", strings.Repeat("e", 64), opened.Revision)
				if !errors.Is(err, fileport.ErrPersistence) || out.Status != fileport.StatusUnknown {
					t.Fatal("resume confirmed after identity loss", out, err)
				}
			case "undo":
				out, err := s.UndoNativeChange(ctx, opened.SessionID, "thread_main", strings.Repeat("e", 64), opened.Revision)
				if !errors.Is(err, fileport.ErrPersistence) || out.Status != fileport.StatusUnknown {
					t.Fatal("undo confirmed after identity loss", out, err)
				}
			}
		})
	}
}
