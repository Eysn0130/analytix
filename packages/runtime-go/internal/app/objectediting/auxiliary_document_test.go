package objectediting

import (
	"context"
	"errors"
	"strings"
	"testing"

	fileport "analytix.local/runtime-go/internal/ports/objectediting"
)

type auxiliaryDocumentProjector struct {
	scopes   []ScopeAuthority
	denyPath string
	denied   bool
	thread   string
}

func (p *auxiliaryDocumentProjector) ValidateCurrent(_ context.Context, scope ScopeAuthority) error {
	p.scopes = append(p.scopes, scope)
	if scope.Path == p.denyPath || p.denied || scope.ThreadID != p.thread || scope.Purpose != "discuss" {
		return ErrProjection
	}
	return nil
}
func (p *auxiliaryDocumentProjector) FreezeSelectionAuthority(ctx context.Context, scope ScopeAuthority) (string, error) {
	return "frozen-current-authority", p.ValidateCurrent(ctx, scope)
}
func (*auxiliaryDocumentProjector) AuthorizeAndProject(context.Context, ProjectionInput) ([]ProtectedRange, error) {
	panic("auxiliary must not capture/project a draft")
}

func TestAuxiliaryDocumentRechecksRevisionSessionAndAuthorityWithoutEditing(t *testing.T) {
	s, identity, files := fixture(t)
	projector := &auxiliaryDocumentProjector{thread: "thread"}
	s.projector = projector
	opened, err := s.Open(context.Background(), "/workspace", "file.md")
	if err != nil {
		t.Fatal(err)
	}
	input := AuxiliaryDocumentInput{opened.SessionID, opened.ObjectID, opened.Revision, "thread"}
	read := func() error { _, err := s.ValidateAuxiliaryDocument(context.Background(), input); return err }
	if err := read(); err != nil {
		t.Fatal(err)
	}
	if len(s.edits) != 0 || len(s.managedReleases) != 0 || files.commits != 0 {
		t.Fatal("validation acquired editing ownership")
	}
	files.doc.Revision = strings.Repeat("c", 64)
	if !errors.Is(read(), fileport.ErrConflict) {
		t.Fatal("stale revision admitted")
	}
	files.doc.Revision = opened.Revision
	input.ThreadID = "other"
	if !errors.Is(read(), ErrProjection) {
		t.Fatal("wrong thread admitted")
	}
	input.ThreadID = "thread"
	projector.denied = true
	if !errors.Is(read(), ErrProjection) {
		t.Fatal("revoked scope admitted")
	}
	projector.denied = false
	files.onRead = func() { identity.unavailable = true }
	if read() == nil {
		t.Fatal("identity revocation during read admitted")
	}
	files.onRead = nil
	identity.unavailable = false
	if err := s.Close(context.Background(), opened.SessionID); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(read(), ErrSession) {
		t.Fatal("closed session admitted")
	}
}

func TestAuxiliaryPathAuthorizesBeforeReadAndCanonicalIdentityWithoutSession(t *testing.T) {
	s, _, files := fixture(t)
	projector := &auxiliaryDocumentProjector{thread: "thread", denied: true}
	s.projector = projector
	reads := 0
	files.onRead = func() { reads++ }
	if _, err := s.ReadAuxiliaryDocument(context.Background(), "thread", "/workspace", "alias.md"); err == nil || reads != 0 {
		t.Fatal("unauthorized source read")
	}
	projector.denied = false
	projector.denyPath = files.doc.Path
	if _, err := s.ReadAuxiliaryDocument(context.Background(), "thread", "/workspace", "alias.md"); err == nil || reads != 1 {
		t.Fatal("canonical path was not reauthorized")
	}
	projector.denyPath = ""
	authority, err := s.ReadAuxiliaryDocument(context.Background(), "thread", "/workspace", "alias.md")
	if err != nil || authority.Path != files.doc.Path || authority.BaseRevision != files.doc.Revision {
		t.Fatalf("canonical authority missing: %v", err)
	}
	if len(s.sessions) != 0 || len(s.edits) != 0 || len(s.managedReleases) != 0 || files.commits != 0 {
		t.Fatal("path validation created editing state")
	}
}
