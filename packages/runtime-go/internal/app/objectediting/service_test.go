package objectediting

import (
	"context"
	"errors"
	"strings"
	"testing"

	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	identityport "analytix.local/runtime-go/internal/ports/identity"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
)

type testIdentity struct {
	principal   identitydomain.PrincipalV1
	unavailable bool
}

func (i *testIdentity) ResolveCurrent(context.Context) (identitydomain.PrincipalV1, error) {
	if i.unavailable {
		return identitydomain.PrincipalV1{}, identityport.ErrUnavailable
	}
	return i.principal, nil
}
func (i *testIdentity) ValidateCurrent(_ context.Context, p identitydomain.PrincipalV1) error {
	if i.unavailable || !identitydomain.SamePrincipalV1(p, i.principal) {
		return identityport.ErrMismatch
	}
	return nil
}

type testFiles struct {
	doc       fileport.Document
	receipt   fileport.Receipt
	input     fileport.CommitInput
	commits   int
	readError error
	onRead    func()
}

func (f *testFiles) Read(context.Context, string, string) (fileport.Document, error) {
	if f.onRead != nil {
		f.onRead()
	}
	return f.doc, f.readError
}
func (f *testFiles) Commit(_ context.Context, in fileport.CommitInput) (fileport.Receipt, error) {
	f.commits++
	f.input = in
	f.doc.Content = in.Content
	f.doc.Revision = f.receipt.Revision
	f.receipt.OperationID = in.OperationID
	return f.receipt, nil
}
func (f *testFiles) Status(context.Context, string, string, string, string) (fileport.Receipt, error) {
	return f.receipt, nil
}
func fixture(t *testing.T) (*Service, *testIdentity, *testFiles) {
	t.Helper()
	p, err := identitydomain.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	identity := &testIdentity{principal: p}
	files := &testFiles{doc: fileport.Document{Workspace: "/workspace", IdentityPath: "/workspace/file.md", Path: "/workspace/file.md", Content: "local original", Revision: strings.Repeat("b", 64)}, receipt: fileport.Receipt{OperationID: "save_1234", Revision: strings.Repeat("c", 64), Status: fileport.StatusCommitted, SavedAt: "2026-09-14T04:00:00Z"}}
	return New(identity, files), identity, files
}

func TestSessionBindsHostPrincipalObjectAndExplicitRevision(t *testing.T) {
	s, identity, files := fixture(t)
	ctx := context.Background()
	opened, err := s.Open(ctx, "/workspace", "file.md")
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Open(ctx, "/workspace", "file.md")
	if err != nil || opened.SessionID != again.SessionID {
		t.Fatal("same object did not retain its session")
	}
	if _, err := s.Commit(ctx, opened.SessionID, "save_1234", opened.Revision, "new draft"); err != nil {
		t.Fatal(err)
	}
	if files.input.Path != opened.Path || files.input.ObjectIdentity != opened.ObjectID || files.input.BaseRevision != opened.Revision {
		t.Fatal("commit lost the bound object or requested revision")
	}
	identity.principal, _ = identitydomain.NewPrincipalV1(strings.Repeat("d", 64), "local", "local")
	if _, err := s.Commit(ctx, opened.SessionID, "save_5678", opened.Revision, "forbidden"); !errors.Is(err, ErrSession) {
		t.Fatalf("changed identity accepted: %v", err)
	}
	if files.commits != 1 {
		t.Fatal("revoked principal wrote a file")
	}
}

func TestClosedOrRestartedSessionCannotWrite(t *testing.T) {
	s, identity, files := fixture(t)
	ctx := context.Background()
	opened, err := s.Open(ctx, "/workspace", "file.md")
	if err != nil {
		t.Fatal(err)
	}
	restarted := New(identity, files)
	if _, err := restarted.Status(ctx, opened.SessionID, "save_1234"); !errors.Is(err, ErrSession) {
		t.Fatal("old session survived restart")
	}
	reopened, err := restarted.Open(ctx, "/workspace", "file.md")
	if err != nil || reopened.ObjectID != opened.ObjectID || reopened.SessionID == opened.SessionID {
		t.Fatal("rebind did not retain only durable object identity")
	}
	if err := s.Close(ctx, opened.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Commit(ctx, opened.SessionID, "save_1234", opened.Revision, "no"); !errors.Is(err, ErrSession) {
		t.Fatal("closed session accepted")
	}
	if files.commits != 0 {
		t.Fatal("session close wrote a file")
	}
}

func TestHistoricalCommitReceiptCannotClaimExternalVersionSaved(t *testing.T) {
	s, _, files := fixture(t)
	ctx := context.Background()
	opened, err := s.Open(ctx, "/workspace", "file.md")
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := s.Status(ctx, opened.SessionID, "save_1234")
	if !errors.Is(err, fileport.ErrConflict) || receipt.Status != fileport.StatusConflict {
		t.Fatalf("historical receipt reported current saved: %#v %v", receipt, err)
	}
	files.readError = errors.New("unsafe local error")
	receipt, err = s.Status(ctx, opened.SessionID, "save_1234")
	if !errors.Is(err, fileport.ErrPersistence) || receipt.Status != fileport.StatusUnknown {
		t.Fatal("failed confirmation reported saved")
	}
}

func TestIdentityLossDuringReadDoesNotReturnOriginalBytes(t *testing.T) {
	s, identity, files := fixture(t)
	files.onRead = func() { identity.unavailable = true }
	opened, err := s.Open(context.Background(), "/workspace", "file.md")
	if !errors.Is(err, ErrUnavailable) || opened.Content != "" || len(s.sessions) != 0 {
		t.Fatal("original escaped after authority loss")
	}
}
