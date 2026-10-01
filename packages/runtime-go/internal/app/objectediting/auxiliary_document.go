package objectediting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode/utf8"

	fileport "analytix.local/runtime-go/internal/ports/objectediting"
)

type AuxiliaryDocumentInput struct {
	SessionID, ObjectID, BaseRevision, ThreadID string
}

// AuxiliaryDocumentAuthority is process-local evidence, never an HTTP grant.
// It contains no document/draft bytes and cannot create an editing lease.
type AuxiliaryDocumentAuthority struct {
	Workspace, Path, ObjectID, BaseRevision, SecurityBinding string
}

func (s *Service) ValidateAuxiliaryDocument(ctx context.Context, input AuxiliaryDocumentInput) (AuxiliaryDocumentAuthority, error) {
	if s == nil {
		return AuxiliaryDocumentAuthority{}, ErrUnavailable
	}
	if !proposalToken.MatchString(input.SessionID) || !proposalHash.MatchString(input.ObjectID) ||
		!proposalHash.MatchString(input.BaseRevision) || !proposalThread.MatchString(input.ThreadID) {
		return AuxiliaryDocumentAuthority{}, fileport.ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.currentLocked(ctx, input.SessionID)
	if err != nil {
		return AuxiliaryDocumentAuthority{}, err
	}
	if current.objectID != input.ObjectID {
		return AuxiliaryDocumentAuthority{}, ErrSession
	}
	freezer, ok := s.projector.(SelectionAuthorityFreezer)
	if !ok {
		return AuxiliaryDocumentAuthority{}, ErrProjection
	}
	scope := current.scopeAuthority(input.ThreadID, "discuss")
	binding, err := freezer.FreezeSelectionAuthority(ctx, scope)
	if err != nil || binding == "" {
		return AuxiliaryDocumentAuthority{}, ErrProjection
	}
	scope.SecurityBinding = binding
	doc, err := s.files.Read(ctx, current.workspace, current.path)
	if err != nil {
		return AuxiliaryDocumentAuthority{}, err
	}
	if doc.Encoding == "office-base64" || !validDraftText(doc.Content) {
		return AuxiliaryDocumentAuthority{}, fileport.ErrNotText
	}
	if doc.Revision != input.BaseRevision {
		return AuxiliaryDocumentAuthority{}, fileport.ErrConflict
	}
	digest := sha256.Sum256([]byte("analytix.object-editing/v1\x00" + current.principal.PrincipalDigest + "\x00" + doc.Workspace + "\x00" + doc.IdentityPath))
	if hex.EncodeToString(digest[:]) != current.objectID {
		return AuxiliaryDocumentAuthority{}, ErrSession
	}
	if s.projector.ValidateCurrent(ctx, scope) != nil || s.validateProposalPrincipal(ctx, current) != nil {
		return AuxiliaryDocumentAuthority{}, ErrProjection
	}
	return AuxiliaryDocumentAuthority{current.workspace, current.path, current.objectID, doc.Revision, binding}, nil
}

// ReadAuxiliaryDocument serves editors without an object session. The workspace
// is host-selected from the primary thread; no session or edit lease is minted.
// Closing a view is owned by its request cancellation, not session revocation.
func (s *Service) ReadAuxiliaryDocument(ctx context.Context, threadID, workspace, path string) (AuxiliaryDocumentAuthority, error) {
	if s == nil {
		return AuxiliaryDocumentAuthority{}, ErrUnavailable
	}
	if !proposalThread.MatchString(threadID) || strings.TrimSpace(path) == "" || len(path) > 4096 || !utf8.ValidString(path) || strings.ContainsRune(path, 0) {
		return AuxiliaryDocumentAuthority{}, fileport.ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	principal, err := s.principal(ctx)
	if err != nil {
		return AuxiliaryDocumentAuthority{}, err
	}
	if s.projector == nil {
		return AuxiliaryDocumentAuthority{}, ErrProjection
	}
	scope := ScopeAuthority{Principal: principal, ThreadID: threadID, Workspace: workspace, Path: path, ObjectID: "auxiliary-document", Purpose: "discuss"}
	if s.projector.ValidateCurrent(ctx, scope) != nil {
		return AuxiliaryDocumentAuthority{}, ErrProjection
	}
	doc, err := s.files.Read(ctx, workspace, path)
	if err != nil {
		return AuxiliaryDocumentAuthority{}, err
	}
	if doc.Encoding == "office-base64" || !validDraftText(doc.Content) {
		return AuxiliaryDocumentAuthority{}, fileport.ErrNotText
	}
	if !proposalHash.MatchString(doc.Revision) {
		return AuxiliaryDocumentAuthority{}, fileport.ErrConflict
	}
	digest := sha256.Sum256([]byte("analytix.object-editing/v1\x00" + principal.PrincipalDigest + "\x00" + doc.Workspace + "\x00" + doc.IdentityPath))
	scope.ObjectID, scope.Workspace, scope.Path = hex.EncodeToString(digest[:]), doc.Workspace, doc.Path
	freezer, ok := s.projector.(SelectionAuthorityFreezer)
	if !ok {
		return AuxiliaryDocumentAuthority{}, ErrProjection
	}
	binding, err := freezer.FreezeSelectionAuthority(ctx, scope)
	if err != nil || binding == "" {
		return AuxiliaryDocumentAuthority{}, ErrProjection
	}
	scope.SecurityBinding = binding
	if s.projector.ValidateCurrent(ctx, scope) != nil || s.identity.ValidateCurrent(ctx, principal) != nil || ctx.Err() != nil {
		return AuxiliaryDocumentAuthority{}, ErrProjection
	}
	return AuxiliaryDocumentAuthority{doc.Workspace, doc.Path, scope.ObjectID, doc.Revision, binding}, nil
}
