// Package objectediting owns protected-local text editing sessions. Local
// keystrokes stay in the editor; only explicit reads and commits cross Core.
package objectediting

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"

	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	identityport "analytix.local/runtime-go/internal/ports/identity"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
)

var (
	ErrUnavailable = errors.New("object_editing_unavailable")
	ErrSession     = errors.New("object_editing_session_invalid")
	ErrCapacity    = errors.New("object_editing_capacity")
)

type Opened struct {
	SessionID string `json:"sessionId"`
	ObjectID  string `json:"objectId"`
	Path      string `json:"path"`
	Content   string `json:"content"`
	Revision  string `json:"revision"`
}

type session struct {
	id        string
	objectID  string
	workspace string
	path      string
	principal identitydomain.PrincipalV1
}

type Service struct {
	identity            identityport.Authority
	files               fileport.Files
	mu                  sync.Mutex
	sessions            map[string]session
	edits               map[string]*editingState
	projector           TrustedSelectionProjector
	beginManagedCapture func(context.Context, string, string, func() error) (func(), error)
	managedReleases     map[string]func()
}

// BindHost is trusted startup composition, never a transport operation.
// Binding after an editing session has been opened is rejected.
func (s *Service) BindHost(projector TrustedSelectionProjector, capture func(context.Context, string, string, func() error) (func(), error)) error {
	if s == nil || projector == nil || capture == nil {
		return ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sessions) != 0 || s.projector != nil || s.beginManagedCapture != nil {
		return ErrUnavailable
	}
	s.projector, s.beginManagedCapture = projector, capture
	s.managedReleases = make(map[string]func())
	return nil
}

func New(identity identityport.Authority, files fileport.Files) *Service {
	return NewWithProjector(identity, files, nil)
}

func (s *Service) principal(ctx context.Context) (identitydomain.PrincipalV1, error) {
	if s == nil || s.identity == nil || s.files == nil || ctx == nil || ctx.Err() != nil {
		return identitydomain.PrincipalV1{}, ErrUnavailable
	}
	p, err := s.identity.ResolveCurrent(ctx)
	if err != nil || identitydomain.ValidatePrincipalV1(p) != nil || s.identity.ValidateCurrent(ctx, p) != nil {
		return identitydomain.PrincipalV1{}, ErrUnavailable
	}
	return p, nil
}

// Open resolves identity in Core and authorizes the actual file through the
// filesystem port. No caller-supplied principal, digest or installed flag grants
// access. These bytes may return only through the typed protected-local lane.
func (s *Service) Open(ctx context.Context, workspace, path string) (Opened, error) {
	p, err := s.principal(ctx)
	if err != nil {
		return Opened{}, err
	}
	doc, err := s.files.Read(ctx, workspace, path)
	if err != nil {
		return Opened{}, err
	}
	if s.identity.ValidateCurrent(ctx, p) != nil {
		return Opened{}, ErrUnavailable
	}
	digest := sha256.Sum256([]byte("analytix.object-editing/v1\x00" + p.PrincipalDigest + "\x00" + doc.Workspace + "\x00" + doc.IdentityPath))
	objectID := hex.EncodeToString(digest[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	// Reopening an existing object does not accumulate sessions. The caller's
	// base revision remains explicit, so a stale view cannot overwrite another.
	for _, current := range s.sessions {
		if current.objectID == objectID && identitydomain.SamePrincipalV1(current.principal, p) {
			return Opened{current.id, objectID, doc.Path, doc.Content, doc.Revision}, nil
		}
	}
	if len(s.sessions) >= 64 {
		return Opened{}, ErrCapacity
	}
	var token [24]byte
	if _, err := rand.Read(token[:]); err != nil {
		return Opened{}, ErrUnavailable
	}
	id := hex.EncodeToString(token[:])
	s.sessions[id] = session{id, objectID, doc.Workspace, doc.Path, p}
	return Opened{id, objectID, doc.Path, doc.Content, doc.Revision}, nil
}

// currentLocked is called with mu held. Close and accepted commits are ordered
// by the same lock; revoking a session cannot race a new write through it.
func (s *Service) currentLocked(ctx context.Context, id string) (session, error) {
	p, err := s.principal(ctx)
	if err != nil {
		return session{}, err
	}
	current, ok := s.sessions[id]
	if !ok || !identitydomain.SamePrincipalV1(current.principal, p) {
		return session{}, ErrSession
	}
	return current, nil
}

func (s *Service) Commit(ctx context.Context, id, operationID, baseRevision, content string) (fileport.Receipt, error) {
	if s == nil {
		return fileport.Receipt{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.currentLocked(ctx, id)
	if err != nil {
		return fileport.Receipt{}, err
	}
	receipt, err := s.files.Commit(ctx, fileport.CommitInput{
		Workspace: current.workspace, Path: current.path, BaseRevision: baseRevision,
		Content: content, OperationID: operationID, ObjectIdentity: current.objectID,
	})
	if err != nil {
		return receipt, err
	}
	return s.verifyCurrentReceipt(ctx, current, receipt)
}

func (s *Service) Status(ctx context.Context, id, operationID string) (fileport.Receipt, error) {
	if s == nil {
		return fileport.Receipt{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.currentLocked(ctx, id)
	if err != nil {
		return fileport.Receipt{}, err
	}
	receipt, err := s.files.Status(ctx, current.objectID, operationID, current.workspace, current.path)
	if err != nil {
		return receipt, err
	}
	return s.verifyCurrentReceipt(ctx, current, receipt)
}

// A durable receipt proves a past commit, not that an external editor has left
// that revision on disk. Never turn a historical replay into a current Saved UI.
func (s *Service) verifyCurrentReceipt(ctx context.Context, current session, receipt fileport.Receipt) (fileport.Receipt, error) {
	if receipt.Status != fileport.StatusCommitted {
		return receipt, nil
	}
	doc, err := s.files.Read(ctx, current.workspace, current.path)
	if err != nil || s.identity.ValidateCurrent(ctx, current.principal) != nil {
		receipt.Status = fileport.StatusUnknown
		return receipt, fileport.ErrPersistence
	}
	if doc.Revision != receipt.Revision {
		receipt.Status = fileport.StatusConflict
		return receipt, fileport.ErrConflict
	}
	s.observeEditingCommitLocked(current.id, receipt, doc.Content)
	return receipt, nil
}

// Close revokes the object session. It does not write, remove or clear a file.
func (s *Service) Close(ctx context.Context, id string) error {
	if s == nil {
		return ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.currentLocked(ctx, id); err != nil {
		return err
	}
	delete(s.sessions, id)
	delete(s.edits, id)
	if release := s.managedReleases[id]; release != nil {
		release()
		delete(s.managedReleases, id)
	}
	return nil
}
