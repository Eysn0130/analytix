package objectediting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	fileport "analytix.local/runtime-go/internal/ports/objectediting"
)

// ExportSnapshotInput names an existing local draft, never a model scope.
type ExportSnapshotInput struct {
	SessionID    string `json:"sessionId"`
	ObjectID     string `json:"objectId"`
	ThreadID     string `json:"threadId"`
	BaseRevision string `json:"baseRevision"`
	DraftVersion string `json:"draftVersion"`
}

// ExportSnapshot is protected-local Main transport only. Its digest identifies
// bytes; authorization is rechecked against the live session and thread.
type ExportSnapshot struct {
	ExportSnapshotInput
	ContentDigest string `json:"contentDigest"`
	Workspace     string `json:"workspace"`
	Path          string `json:"path"`
	Content       string `json:"content"`
}

func (s *Service) ReadExportSnapshot(ctx context.Context, input ExportSnapshotInput) (ExportSnapshot, error) {
	if s == nil {
		return ExportSnapshot{}, ErrUnavailable
	}
	if !proposalToken.MatchString(input.SessionID) || !proposalHash.MatchString(input.ObjectID) ||
		!proposalThread.MatchString(input.ThreadID) || !proposalHash.MatchString(input.BaseRevision) || !proposalToken.MatchString(input.DraftVersion) {
		return ExportSnapshot{}, fileport.ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.currentLocked(ctx, input.SessionID)
	if err != nil {
		return ExportSnapshot{}, err
	}
	if current.objectID != input.ObjectID {
		return ExportSnapshot{}, ErrSession
	}
	// Reuse current Core thread/read authority, without capturing a model scope,
	// projecting text, taking an edit lease, or creating a second owner.
	authorize := func() error {
		if s.projector == nil || s.projector.ValidateCurrent(ctx, current.scopeAuthority(input.ThreadID, "discuss")) != nil {
			return ErrProjection
		}
		return s.validateProposalPrincipal(ctx, current)
	}
	if err := authorize(); err != nil {
		return ExportSnapshot{}, err
	}
	state := s.edits[input.SessionID]
	if state == nil || state.draft.ObjectID != input.ObjectID || state.draft.BaseRevision != input.BaseRevision || state.draft.Version != input.DraftVersion {
		return ExportSnapshot{}, ErrDraftStale
	}
	doc, err := s.files.Read(ctx, current.workspace, current.path)
	if err != nil {
		return ExportSnapshot{}, err
	}
	if doc.Encoding == "office-base64" || !validDraftText(doc.Content) || !validDraftText(state.draft.Content) {
		return ExportSnapshot{}, fileport.ErrNotText
	}
	if doc.Revision != input.BaseRevision {
		return ExportSnapshot{}, fileport.ErrConflict
	}
	if err := authorize(); err != nil {
		return ExportSnapshot{}, err
	}
	content := strings.Clone(state.draft.Content)
	digest := sha256.Sum256([]byte(content))
	return ExportSnapshot{input, hex.EncodeToString(digest[:]), current.workspace, current.path, content}, nil
}
