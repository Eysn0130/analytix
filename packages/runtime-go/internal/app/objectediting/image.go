package objectediting

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"reflect"
	"strings"
	"unicode/utf8"

	identity "analytix.local/runtime-go/internal/domain/identity"
	files "analytix.local/runtime-go/internal/ports/objectediting"
)

// ImageWorkspaceResolver is trusted host composition, not caller authority.
// It resolves a live primary thread and principal before any source file read.
type ImageWorkspaceResolver interface {
	ResolveImageWorkspace(context.Context, identity.PrincipalV1, string) (string, error)
}
type OpenedImage struct {
	SessionID      string `json:"sessionId"`
	ObjectID       string `json:"objectId"`
	ThreadID       string `json:"threadId"`
	Path           string `json:"path"`
	SourceRevision string `json:"sourceRevision"`
	MIMEType       string `json:"mimeType"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	DataBase64     string `json:"dataBase64"`
}
type ImageAnnotationWrite struct {
	SessionID                  string                        `json:"sessionId"`
	ThreadID                   string                        `json:"threadId"`
	SourceRevision             string                        `json:"sourceRevision"`
	ExpectedAnnotationRevision string                        `json:"expectedAnnotationRevision"`
	Regions                    []files.ImageAnnotationRegion `json:"regions"`
}
type ImageScopeCapture struct {
	RegionID           string `json:"regionId"`
	SessionID          string `json:"sessionId"`
	ThreadID           string `json:"threadId"`
	SourceRevision     string `json:"sourceRevision"`
	AnnotationRevision string `json:"annotationRevision"`
}
type ImageScopeBinding struct {
	SessionID string `json:"sessionId"`
	ThreadID  string `json:"threadId"`
	ScopeID   string `json:"scopeId"`
}
type ImageScope struct {
	RegionID           string            `json:"regionId"`
	Kind               string            `json:"kind"`
	SessionID          string            `json:"sessionId"`
	ScopeID            string            `json:"scopeId"`
	ObjectID           string            `json:"objectId"`
	ThreadID           string            `json:"threadId"`
	SourceRevision     string            `json:"sourceRevision"`
	AnnotationRevision string            `json:"annotationRevision"`
	Width              int               `json:"width"`
	Height             int               `json:"height"`
	Region             files.ImageRegion `json:"region"`
	Purpose            string            `json:"purpose"`
	Editable           bool              `json:"editable"`
	Current            bool              `json:"current"`
}

func imageObjectID(p identity.PrincipalV1, doc files.ImageDocument) string {
	digest := sha256.Sum256([]byte("analytix.object-editing/v1\x00" + p.PrincipalDigest + "\x00" + doc.Workspace + "\x00" + doc.IdentityPath))
	return hex.EncodeToString(digest[:])
}
func (s *Service) imageWorkspace(ctx context.Context, p identity.PrincipalV1, thread string) (string, error) {
	resolver, ok := s.projector.(ImageWorkspaceResolver)
	if !ok || !proposalThread.MatchString(thread) {
		return "", ErrProjection
	}
	return resolver.ResolveImageWorkspace(ctx, p, thread)
}
func (s *Service) OpenImage(ctx context.Context, thread, path string) (OpenedImage, error) {
	if s == nil || strings.TrimSpace(path) == "" || !utf8.ValidString(path) || len(path) > 4096 || strings.ContainsRune(path, 0) {
		return OpenedImage{}, files.ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.principal(ctx)
	if err != nil {
		return OpenedImage{}, err
	}
	workspace, err := s.imageWorkspace(ctx, p, thread)
	if err != nil {
		return OpenedImage{}, err
	}
	port, ok := s.files.(files.ImageFiles)
	if !ok {
		return OpenedImage{}, ErrUnavailable
	}
	scope := ScopeAuthority{Principal: p, ThreadID: thread, Workspace: workspace, Path: path, ObjectID: "image-snapshot", Purpose: "discuss"}
	if s.projector.ValidateCurrent(ctx, scope) != nil {
		return OpenedImage{}, ErrProjection
	}
	doc, err := port.ReadImage(ctx, workspace, path)
	if err != nil {
		return OpenedImage{}, err
	}
	object := imageObjectID(p, doc)
	scope.ObjectID, scope.Workspace, scope.Path = object, doc.Workspace, doc.Path
	if s.projector.ValidateCurrent(ctx, scope) != nil || s.identity.ValidateCurrent(ctx, p) != nil || ctx.Err() != nil {
		return OpenedImage{}, ErrProjection
	}
	id := ""
	for _, current := range s.sessions {
		if current.objectID == object && identity.SamePrincipalV1(current.principal, p) {
			if !current.image {
				return OpenedImage{}, ErrSession
			}
			id = current.id
			break
		}
	}
	if id == "" {
		if len(s.sessions) >= 64 {
			return OpenedImage{}, ErrCapacity
		}
		id, err = opaqueProposalID()
		if err != nil {
			return OpenedImage{}, err
		}
		s.sessions[id] = session{id: id, objectID: object, workspace: doc.Workspace, path: doc.Path, principal: p, image: true}
	}
	return OpenedImage{id, object, thread, doc.Path, doc.Revision, doc.MIMEType, doc.Width, doc.Height, base64.StdEncoding.EncodeToString(doc.Bytes)}, nil
}
func (s *Service) currentImageLocked(ctx context.Context, id, thread string) (session, files.ImageFiles, error) {
	current, err := s.currentSessionLocked(ctx, id)
	if err != nil {
		return session{}, nil, err
	}
	port, ok := s.files.(files.ImageFiles)
	if !current.image || !ok {
		return session{}, nil, ErrSession
	}
	if _, err := s.imageWorkspace(ctx, current.principal, thread); err != nil {
		return session{}, nil, err
	}
	if s.projector.ValidateCurrent(ctx, current.scopeAuthority(thread, "discuss")) != nil {
		return session{}, nil, ErrProjection
	}
	return current, port, nil
}
func (s *Service) imageAnnotationLocked(ctx context.Context, id, thread string) (session, files.ImageAnnotation, error) {
	current, port, err := s.currentImageLocked(ctx, id, thread)
	if err != nil {
		return current, files.ImageAnnotation{}, err
	}
	value, err := port.ReadImageAnnotation(ctx, files.AnnotationDraftTarget{Workspace: current.workspace, Path: current.path, ObjectIdentity: current.objectID, ThreadID: thread})
	if err != nil {
		return current, files.ImageAnnotation{}, err
	}
	if value.ObjectID != current.objectID || value.ThreadID != thread || s.projector.ValidateCurrent(ctx, current.scopeAuthority(thread, "discuss")) != nil || s.validateProposalPrincipal(ctx, current) != nil {
		return current, files.ImageAnnotation{}, ErrProjection
	}
	return current, value, nil
}
func (s *Service) ReadImageAnnotation(ctx context.Context, id, thread string) (files.ImageAnnotation, error) {
	if s == nil {
		return files.ImageAnnotation{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, value, err := s.imageAnnotationLocked(ctx, id, thread)
	return value, err
}
func (s *Service) WriteImageAnnotation(ctx context.Context, input ImageAnnotationWrite) (files.ImageAnnotation, error) {
	if s == nil {
		return files.ImageAnnotation{}, ErrUnavailable
	}
	if !files.ValidImageAnnotationWrite(input.ExpectedAnnotationRevision, input.SourceRevision, input.Regions) {
		return files.ImageAnnotation{}, files.ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, port, err := s.currentImageLocked(ctx, input.SessionID, input.ThreadID)
	if err != nil {
		return files.ImageAnnotation{}, err
	}
	// Even an uncertain acknowledgement revokes previous captures. Re-read and
	// recapture the persisted annotation; never reuse a potentially old note.
	if state := s.edits[input.SessionID]; state != nil {
		for id, scope := range state.scopes {
			if scope.image != nil && scope.image.ThreadID == input.ThreadID {
				delete(state.scopes, id)
			}
		}
	}
	value, err := port.WriteImageAnnotation(ctx, files.ImageAnnotationWriteInput{AnnotationDraftTarget: files.AnnotationDraftTarget{Workspace: current.workspace, Path: current.path, ObjectIdentity: current.objectID, ThreadID: input.ThreadID}, ExpectedAnnotationRevision: input.ExpectedAnnotationRevision, SourceRevision: input.SourceRevision, Regions: input.Regions})
	if err != nil {
		return files.ImageAnnotation{}, err
	}
	if value.ObjectID != current.objectID || value.ThreadID != input.ThreadID || !value.Current || s.projector.ValidateCurrent(ctx, current.scopeAuthority(input.ThreadID, "discuss")) != nil || s.validateProposalPrincipal(ctx, current) != nil {
		return files.ImageAnnotation{}, ErrProjection
	}
	return value, nil
}
func (s *Service) CaptureImageScope(ctx context.Context, input ImageScopeCapture) (ImageScope, error) {
	if s == nil {
		return ImageScope{}, ErrUnavailable
	}
	if !proposalHash.MatchString(input.SourceRevision) || !proposalHash.MatchString(input.AnnotationRevision) || !files.ValidImageRegionID(input.RegionID) {
		return ImageScope{}, files.ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, note, err := s.imageAnnotationLocked(ctx, input.SessionID, input.ThreadID)
	if err != nil {
		return ImageScope{}, err
	}
	selected, found := imageAnnotationRegion(note, input.RegionID)
	if !note.Current || !found || note.SourceRevision != input.SourceRevision || note.AnnotationRevision != input.AnnotationRevision {
		return ImageScope{}, ErrDraftStale
	}
	freezer, ok := s.projector.(SelectionAuthorityFreezer)
	if !ok {
		return ImageScope{}, ErrProjection
	}
	authority := current.scopeAuthority(input.ThreadID, "discuss")
	binding, err := freezer.FreezeSelectionAuthority(ctx, authority)
	if err != nil || binding == "" {
		return ImageScope{}, ErrProjection
	}
	authority.SecurityBinding = binding
	ranges, err := s.projector.AuthorizeAndProject(ctx, ProjectionInput{ScopeAuthority: authority, Text: selected.Note})
	if err != nil || len(ranges) > MaxProtectedSpans {
		return ImageScope{}, ErrProjection
	}
	parts, err := imageNoteParts(selected.Note, ranges)
	if err != nil {
		return ImageScope{}, err
	}
	if s.projector.ValidateCurrent(ctx, authority) != nil || s.validateProposalPrincipal(ctx, current) != nil {
		return ImageScope{}, ErrProjection
	}
	state := s.edits[input.SessionID]
	if state == nil {
		state = &editingState{scopes: make(map[string]*capturedScope)}
		s.edits[input.SessionID] = state
	}
	if len(state.scopes) >= MaxScopesPerSession {
		return ImageScope{}, ErrCapacity
	}
	id, err := opaqueProposalID()
	if err != nil {
		return ImageScope{}, err
	}
	view := ImageScope{Kind: "image-region", SessionID: input.SessionID, ScopeID: id, ObjectID: current.objectID, ThreadID: input.ThreadID, SourceRevision: note.SourceRevision, AnnotationRevision: note.AnnotationRevision, Width: note.Width, Height: note.Height, RegionID: selected.RegionID, Region: selected.Region, Purpose: "discuss", Editable: false, Current: true}
	state.scopes[id] = &capturedScope{image: &view, imageBinding: binding, view: Scope{Parts: parts}}
	if _, err := s.imageScopeLocked(ctx, ImageScopeBinding{SessionID: input.SessionID, ThreadID: input.ThreadID, ScopeID: id}); err != nil {
		delete(state.scopes, id)
		return ImageScope{}, err
	}
	return view, nil
}
func imageNoteParts(note string, ranges []ProtectedRange) ([]PatchPart, error) {
	parts := []PatchPart{}
	cursor := 0
	var literals strings.Builder
	appendLiteral := func(text string) bool {
		if text == "" {
			return true
		}
		if !proposalLiteralSafe(text) {
			return false
		}
		literals.WriteString(text)
		parts = append(parts, PatchPart{Kind: "literal", Text: strings.Clone(text)})
		return true
	}
	for _, span := range ranges {
		if span.StartByte < cursor || span.EndByte <= span.StartByte || span.EndByte > len(note) || !utf8.ValidString(note[:span.StartByte]) || !utf8.ValidString(note[:span.EndByte]) || !appendLiteral(note[cursor:span.StartByte]) {
			return nil, ErrProjection
		}
		id, err := opaqueProposalID()
		if err != nil {
			return nil, err
		}
		parts = append(parts, PatchPart{Kind: "protected", ProtectedRef: "protected_" + id})
		cursor = span.EndByte
	}
	if !appendLiteral(note[cursor:]) || !proposalLiteralSafe(literals.String()) {
		return nil, ErrProjection
	}
	for _, span := range ranges {
		if strings.Contains(literals.String(), note[span.StartByte:span.EndByte]) {
			return nil, ErrProjection
		}
	}
	return parts, nil
}
func (s *Service) imageScopeLocked(ctx context.Context, input ImageScopeBinding) (*capturedScope, error) {
	state := s.edits[input.SessionID]
	if state == nil {
		return nil, ErrScope
	}
	scope := state.scopes[input.ScopeID]
	if scope == nil || scope.image == nil || scope.revoked || !scope.image.Current || scope.image.ThreadID != input.ThreadID {
		return nil, ErrScope
	}
	current, note, err := s.imageAnnotationLocked(ctx, input.SessionID, input.ThreadID)
	selected, found := imageAnnotationRegion(note, scope.image.RegionID)
	if err != nil || !note.Current || !found || note.AnnotationRevision != scope.image.AnnotationRevision || note.SourceRevision != scope.image.SourceRevision || note.Width != scope.image.Width || note.Height != scope.image.Height || !reflect.DeepEqual(selected.Region, scope.image.Region) {
		scope.revoked = true
		scope.image.Current = false
		scope.view.Parts = nil
		return nil, ErrDraftStale
	}
	authority := current.scopeAuthority(input.ThreadID, "discuss")
	authority.SecurityBinding = scope.imageBinding
	if s.projector.ValidateCurrent(ctx, authority) != nil || s.validateProposalPrincipal(ctx, current) != nil {
		scope.revoked = true
		scope.image.Current = false
		scope.view.Parts = nil
		return nil, ErrProjection
	}
	return scope, nil
}
func (s *Service) ReadImageScope(ctx context.Context, input ImageScopeBinding) (ImageScope, error) {
	if s == nil {
		return ImageScope{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := s.imageScopeLocked(ctx, input)
	if err != nil {
		return ImageScope{}, err
	}
	return *scope.image, nil
}
func (s *Service) RevokeImageScope(ctx context.Context, input ImageScopeBinding) error {
	if s == nil {
		return ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, _, err := s.currentImageLocked(ctx, input.SessionID, input.ThreadID); err != nil {
		return err
	}
	state := s.edits[input.SessionID]
	if state == nil {
		return ErrScope
	}
	scope := state.scopes[input.ScopeID]
	if scope == nil || scope.image == nil || scope.image.ThreadID != input.ThreadID {
		return ErrScope
	}
	scope.revoked = true
	scope.image.Current = false
	scope.view.Parts = nil
	return nil
}
func (s *Service) HasImageScopes() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, state := range s.edits {
		for _, scope := range state.scopes {
			if scope.image != nil && scope.image.Current && !scope.revoked {
				return true
			}
		}
	}
	return false
}

// ReadImageScopeForModel is called only by the current Core tool owner. Missing
// scopes may belong to another existing owner; known stale scopes never fall back.
func (s *Service) ReadImageScopeForModel(ctx context.Context, thread, id string) (map[string]any, bool, error) {
	if s == nil {
		return nil, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for sessionID, state := range s.edits {
		candidate := state.scopes[id]
		if candidate == nil || candidate.image == nil {
			continue
		}
		scope, err := s.imageScopeLocked(ctx, ImageScopeBinding{SessionID: sessionID, ThreadID: thread, ScopeID: id})
		if err != nil {
			return nil, true, err
		}
		return map[string]any{"kind": "image-region", "scopeId": id, "editable": false, "region": scope.image.Region, "dimensions": map[string]int{"width": scope.image.Width, "height": scope.image.Height}, "noteParts": cloneParts(scope.view.Parts), "imageObservation": "unavailable"}, true, nil
	}
	return nil, false, nil
}

func imageAnnotationRegion(annotation files.ImageAnnotation, id string) (files.ImageAnnotationRegion, bool) {
	for _, item := range annotation.Regions {
		if item.RegionID == id {
			return item, true
		}
	}
	return files.ImageAnnotationRegion{}, false
}
