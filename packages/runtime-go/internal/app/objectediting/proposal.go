package objectediting

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	ordinary "analytix.local/runtime-go/internal/domain/ordinaryprojection"
	identityport "analytix.local/runtime-go/internal/ports/identity"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
)

const (
	MaxSelectionBytes      = 64 << 10
	MaxPatchBytes          = 64 << 10
	MaxPatchParts          = 256
	MaxProtectedSpans      = 64
	MaxScopesPerSession    = 8
	MaxProposalsPerSession = 16
	MaxProposalOperations  = 128
)

var (
	ErrDraftStale      = errors.New("object_editing_draft_stale")
	ErrScope           = errors.New("object_editing_scope_invalid")
	ErrProposal        = errors.New("object_editing_proposal_invalid")
	ErrProjection      = errors.New("object_editing_projection_unavailable")
	ErrProtected       = errors.New("object_editing_protected_span_invalid")
	proposalToken      = regexp.MustCompile(`^[a-f0-9]{48}$`)
	proposalHash       = regexp.MustCompile(`^[a-f0-9]{64}$`)
	proposalOperation  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$`)
	proposalThread     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	placeholderLiteral = regexp.MustCompile(`(?i)(?:PERSON|ACCOUNT|PHONE|EMAIL|ADDRESS|IDENTITY|ID_CARD|BANK_CARD|COMPANY|ORGANIZATION|DEVICE|IP|MAC|NUMBER)_[0-9]+`)
)

// ProjectionInput is Core-only. The injected authority must authorize ThreadID
// and Purpose for Principal/ObjectID and find ALL protected ranges in Text,
// including credentials. It must not call back into this Service (mu is held).
// The existing privacy/secret projection machinery can back this adapter, but
// frontend "already redacted" claims never implement this authority.
type ScopeAuthority struct {
	Principal                   identitydomain.PrincipalV1
	ObjectID, ThreadID, Purpose string
	Workspace, Path             string
}
type ProjectionInput struct {
	ScopeAuthority
	Text string
}

// ProtectedRange uses original UTF-8 byte offsets, not projected-text offsets.
type ProtectedRange struct{ StartByte, EndByte int }
type TrustedSelectionProjector interface {
	ValidateCurrent(context.Context, ScopeAuthority) error
	AuthorizeAndProject(context.Context, ProjectionInput) ([]ProtectedRange, error)
}

// NewWithProjector is trusted composition only. New leaves capture unavailable;
// callers cannot supply a projector through a transport request.
func NewWithProjector(identity identityport.Authority, files fileport.Files, projector TrustedSelectionProjector) *Service {
	return &Service{identity: identity, files: files, sessions: make(map[string]session), edits: make(map[string]*editingState), projector: projector}
}

// Every exported value below belongs to protected-local transport. Draft text,
// private proposals and scope mappings are memory-only and are not history data.
type Draft struct {
	ObjectID     string `json:"objectId"`
	BaseRevision string `json:"baseRevision"`
	Version      string `json:"version"`
	Content      string `json:"content"`
}
type UpdateDraftInput struct {
	SessionID       string `json:"sessionId"`
	BaseRevision    string `json:"baseRevision"`
	ExpectedVersion string `json:"expectedVersion"`
	Content         string `json:"content"`
}
type UTF16Range struct {
	Start int `json:"start"`
	End   int `json:"end"`
}
type CaptureScopeInput struct {
	SessionID    string     `json:"sessionId"`
	DraftVersion string     `json:"draftVersion"`
	ThreadID     string     `json:"threadId"`
	Purpose      string     `json:"purpose"`
	Range        UTF16Range `json:"range"`
}

// Parts are a tagged union. A protected reference has no caller-supplied text;
// a literal cannot contain a reference or a placeholder to be expanded later.
type PatchPart struct {
	Kind         string `json:"kind"`
	Text         string `json:"text,omitempty"`
	ProtectedRef string `json:"protectedRef,omitempty"`
}
type Scope struct {
	ScopeID      string      `json:"scopeId"`
	ObjectID     string      `json:"objectId"`
	ThreadID     string      `json:"threadId"`
	Purpose      string      `json:"purpose"`
	DraftVersion string      `json:"draftVersion"`
	BaseRevision string      `json:"baseRevision"`
	Range        UTF16Range  `json:"range"`
	Parts        []PatchPart `json:"parts"`
	Current      bool        `json:"current"`
}
type ScopeBinding struct {
	SessionID    string `json:"sessionId"`
	ScopeID      string `json:"scopeId"`
	ThreadID     string `json:"threadId"`
	Purpose      string `json:"purpose"`
	DraftVersion string `json:"draftVersion"`
}
type ProposeInput struct {
	ScopeBinding
	OperationID string      `json:"operationId"`
	Parts       []PatchPart `json:"parts"`
}
type Proposal struct {
	ProposalID   string      `json:"proposalId"`
	ScopeID      string      `json:"scopeId"`
	DraftVersion string      `json:"draftVersion"`
	Parts        []PatchPart `json:"parts"`
	Status       string      `json:"status"`
}
type DecisionInput struct {
	ScopeBinding
	OperationID string `json:"operationId"`
	ProposalID  string `json:"proposalId"`
}

// A replay returns the same decision, not an old draft snapshot that could
// overwrite later input. ReadDraft retrieves the current working copy.
type Decision struct {
	ProposalID   string `json:"proposalId"`
	Status       string `json:"status"`
	DraftVersion string `json:"draftVersion"`
}
type protectedSpan struct{ token, text string }
type capturedScope struct {
	view               Scope
	startByte, endByte int
	spans              []protectedSpan
	revoked            bool
}
type proposalOperationRecord struct {
	digest, proposalID string
	decision           Decision
}
type editingState struct {
	draft                            Draft
	scopes                           map[string]*capturedScope
	proposals                        map[string]*Proposal
	operations                       map[string]proposalOperationRecord
	lastCommitID, lastCommitRevision string
}

func opaqueProposalID() (string, error) {
	var bytes [24]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", ErrUnavailable
	}
	return hex.EncodeToString(bytes[:]), nil
}
func validDraftText(text string) bool {
	if len(text) > fileport.MaxTextBytes || !utf8.ValidString(text) {
		return false
	}
	for _, char := range text {
		if char < 0x20 && char != '\n' && char != '\r' && char != '\t' {
			return false
		}
	}
	return true
}
func validPurpose(purpose string) bool { return purpose == "discuss" || purpose == "edit" }
func (s *Service) validateProposalPrincipal(ctx context.Context, current session) error {
	if ctx.Err() != nil || s.identity.ValidateCurrent(ctx, current.principal) != nil {
		return ErrUnavailable
	}
	return nil
}

// UpdateDraft explicitly captures a local working copy; it does not write a
// file, invoke a provider, or receive individual keystrokes automatically.
func (s *Service) UpdateDraft(ctx context.Context, input UpdateDraftInput) (Draft, error) {
	if s == nil {
		return Draft{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.currentLocked(ctx, input.SessionID)
	if err != nil {
		return Draft{}, err
	}
	if !proposalHash.MatchString(input.BaseRevision) || !validDraftText(input.Content) {
		return Draft{}, fileport.ErrInvalidInput
	}
	state := s.edits[input.SessionID]
	// Check draft CAS before reading or rebasing. Failed capture retains the old
	// working copy and every scope; only a successful replacement invalidates it.
	if (state == nil && input.ExpectedVersion != "") || (state != nil && input.ExpectedVersion != state.draft.Version) {
		return Draft{}, ErrDraftStale
	}
	doc, err := s.files.Read(ctx, current.workspace, current.path)
	if err != nil {
		return Draft{}, ErrUnavailable
	}
	if doc.Encoding == "office-base64" || !validDraftText(doc.Content) {
		return Draft{}, fileport.ErrNotText
	}
	if doc.Revision != input.BaseRevision {
		return Draft{}, ErrDraftStale
	}
	if state == nil {
		state = &editingState{scopes: make(map[string]*capturedScope), proposals: make(map[string]*Proposal), operations: make(map[string]proposalOperationRecord)}
	}
	version, err := opaqueProposalID()
	if err != nil {
		return Draft{}, err
	}
	if err := s.validateProposalPrincipal(ctx, current); err != nil {
		return Draft{}, err
	}
	invalidateEditingScopes(state)
	state.draft = Draft{current.objectID, input.BaseRevision, version, strings.Clone(input.Content)}
	s.edits[input.SessionID] = state
	return state.draft, nil
}
func (s *Service) ReadDraft(ctx context.Context, sessionID string) (Draft, error) {
	if s == nil {
		return Draft{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.currentLocked(ctx, sessionID); err != nil {
		return Draft{}, err
	}
	state := s.edits[sessionID]
	if state == nil {
		return Draft{}, ErrDraftStale
	}
	return state.draft, nil
}

// utf16ByteRange rejects offsets inside a supplementary Unicode character.
func utf16ByteRange(text string, selection UTF16Range) (int, int, bool) {
	if !utf8.ValidString(text) || selection.Start < 0 || selection.End < selection.Start {
		return 0, 0, false
	}
	start, end, units := -1, -1, 0
	for offset, char := range text {
		if units == selection.Start {
			start = offset
		}
		if units == selection.End {
			end = offset
		}
		units++
		if char > 0xffff {
			units++
		}
	}
	if units == selection.Start {
		start = len(text)
	}
	if units == selection.End {
		end = len(text)
	}
	return start, end, start >= 0 && end >= start
}
func proposalLiteralSafe(text string) bool {
	if !validDraftText(text) || placeholderLiteral.MatchString(text) || strings.Contains(strings.ToLower(text), "protected_") {
		return false
	}
	// Ordinary projection canonicalizes whitespace-only strings to empty. Such
	// separators contain no protected data and must retain their original bytes.
	return strings.TrimSpace(text) == "" || ordinary.ProjectTextV1(text) == text
}
func cloneParts(parts []PatchPart) []PatchPart {
	cloned := make([]PatchPart, len(parts))
	for index, part := range parts {
		cloned[index] = PatchPart{Kind: strings.Clone(part.Kind), Text: strings.Clone(part.Text), ProtectedRef: strings.Clone(part.ProtectedRef)}
	}
	return cloned
}
func scopeView(scope *capturedScope) Scope {
	view := scope.view
	view.Parts = cloneParts(view.Parts)
	return view
}
func proposalView(proposal *Proposal) Proposal {
	result := *proposal
	result.Parts = cloneParts(result.Parts)
	return result
}

func (s *Service) CaptureScope(ctx context.Context, input CaptureScopeInput) (Scope, error) {
	if s == nil {
		return Scope{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.currentLocked(ctx, input.SessionID)
	if err != nil {
		return Scope{}, err
	}
	state := s.edits[input.SessionID]
	if state == nil || input.DraftVersion != state.draft.Version {
		return Scope{}, ErrDraftStale
	}
	if !proposalThread.MatchString(input.ThreadID) || !validPurpose(input.Purpose) {
		return Scope{}, fileport.ErrInvalidInput
	}
	if s.projector == nil {
		return Scope{}, ErrProjection
	}
	if len(state.scopes) >= MaxScopesPerSession {
		return Scope{}, ErrCapacity
	}
	start, end, valid := utf16ByteRange(state.draft.Content, input.Range)
	if !valid || end-start > MaxSelectionBytes {
		return Scope{}, fileport.ErrInvalidInput
	}
	selected := state.draft.Content[start:end]
	ranges, err := s.projector.AuthorizeAndProject(ctx, ProjectionInput{current.scopeAuthority(input.ThreadID, input.Purpose), selected})
	if err != nil || len(ranges) > MaxProtectedSpans {
		return Scope{}, ErrProjection
	}
	id, err := opaqueProposalID()
	if err != nil {
		return Scope{}, err
	}
	scope := &capturedScope{view: Scope{ScopeID: id, ObjectID: current.objectID, ThreadID: input.ThreadID, Purpose: input.Purpose, DraftVersion: state.draft.Version, BaseRevision: state.draft.BaseRevision, Range: input.Range, Parts: []PatchPart{}, Current: true}, startByte: start, endByte: end}
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
		scope.view.Parts = append(scope.view.Parts, PatchPart{Kind: "literal", Text: strings.Clone(text)})
		return true
	}
	for _, span := range ranges {
		if span.StartByte < cursor || span.EndByte <= span.StartByte || span.EndByte > len(selected) || !utf8.ValidString(selected[:span.StartByte]) || !utf8.ValidString(selected[:span.EndByte]) {
			return Scope{}, ErrProjection
		}
		if !appendLiteral(selected[cursor:span.StartByte]) {
			return Scope{}, ErrProjection
		}
		spanID, err := opaqueProposalID()
		if err != nil {
			return Scope{}, err
		}
		reference := "protected_" + spanID
		scope.spans = append(scope.spans, protectedSpan{reference, strings.Clone(selected[span.StartByte:span.EndByte])})
		scope.view.Parts = append(scope.view.Parts, PatchPart{Kind: "protected", ProtectedRef: reference})
		cursor = span.EndByte
	}
	if !appendLiteral(selected[cursor:]) || !proposalLiteralSafe(literals.String()) {
		return Scope{}, ErrProjection
	}
	for _, span := range scope.spans {
		if strings.Contains(literals.String(), span.text) {
			return Scope{}, ErrProjection
		}
	}
	if err := s.validateScopeAuthority(ctx, current, scope); err != nil {
		return Scope{}, err
	}
	// Acquire protection before exposing an edit scope. The host callback takes
	// the same mutation lease as every file tool and checks the disk baseline
	// inside that lease. No Renderer flag can bypass it.
	if input.Purpose == "edit" && s.beginManagedCapture != nil && s.managedReleases[current.id] == nil {
		release, err := s.beginManagedCapture(ctx, current.id, current.path, func() error {
			doc, readErr := s.files.Read(ctx, current.workspace, current.path)
			if readErr != nil || doc.Revision != state.draft.BaseRevision {
				return ErrDraftStale
			}
			return nil
		})
		if err != nil {
			return Scope{}, ErrProjection
		}
		if err := s.validateScopeAuthority(ctx, current, scope); err != nil {
			release()
			return Scope{}, err
		}
		s.managedReleases[current.id] = release
	}
	state.scopes[id] = scope
	return scopeView(scope), nil
}

func boundScope(state *editingState, binding ScopeBinding, requireCurrent bool) (*capturedScope, error) {
	if state == nil {
		return nil, ErrScope
	}
	scope := state.scopes[binding.ScopeID]
	if scope == nil || scope.revoked || scope.view.ThreadID != binding.ThreadID || scope.view.Purpose != binding.Purpose || scope.view.DraftVersion != binding.DraftVersion {
		return nil, ErrScope
	}
	if requireCurrent && (!scope.view.Current || state.draft.Version != binding.DraftVersion) {
		return nil, ErrDraftStale
	}
	return scope, nil
}
func (s *Service) ReadScope(ctx context.Context, binding ScopeBinding) (Scope, error) {
	if s == nil {
		return Scope{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.currentLocked(ctx, binding.SessionID)
	if err != nil {
		return Scope{}, err
	}
	scope, err := boundScope(s.edits[binding.SessionID], binding, false)
	if err != nil {
		return Scope{}, err
	}
	if err := s.validateScopeAuthority(ctx, current, scope); err != nil {
		return Scope{}, err
	}
	return scopeView(scope), nil
}
func (s *Service) RevokeScope(ctx context.Context, binding ScopeBinding) error {
	if s == nil {
		return ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.currentLocked(ctx, binding.SessionID); err != nil {
		return err
	}
	state := s.edits[binding.SessionID]
	if state == nil {
		return ErrScope
	}
	scope := state.scopes[binding.ScopeID]
	if scope == nil || scope.view.ThreadID != binding.ThreadID || scope.view.Purpose != binding.Purpose || scope.view.DraftVersion != binding.DraftVersion {
		return ErrScope
	}
	scope.revoked = true
	scope.view.Current = false
	scope.spans = nil
	return nil
}
func invalidateEditingScopes(state *editingState) {
	for _, scope := range state.scopes {
		scope.view.Current = false
		scope.spans = nil
	}
}

// Literal assembly is contextual and typed; no global placeholder substitution
// exists. Every source protected span must appear exactly once, in source order.
func renderProposal(scope *capturedScope, parts []PatchPart) (string, error) {
	if len(parts) > MaxPatchParts {
		return "", ErrProtected
	}
	var output, literals strings.Builder
	nextProtected := 0
	for _, part := range parts {
		switch part.Kind {
		case "literal":
			if part.Text == "" || part.ProtectedRef != "" || !proposalLiteralSafe(part.Text) {
				return "", ErrProtected
			}
			for _, span := range scope.spans {
				if strings.Contains(part.Text, span.text) {
					return "", ErrProtected
				}
			}
			output.WriteString(part.Text)
			literals.WriteString(part.Text)
		case "protected":
			if part.Text != "" || nextProtected >= len(scope.spans) || part.ProtectedRef != scope.spans[nextProtected].token {
				return "", ErrProtected
			}
			output.WriteString(scope.spans[nextProtected].text)
			nextProtected++
		default:
			return "", ErrProtected
		}
		if output.Len() > MaxPatchBytes {
			return "", ErrProtected
		}
	}
	if nextProtected != len(scope.spans) || !proposalLiteralSafe(literals.String()) {
		return "", ErrProtected
	}
	for _, span := range scope.spans {
		if strings.Contains(literals.String(), span.text) {
			return "", ErrProtected
		}
	}
	return output.String(), nil
}
func validScopeBinding(binding ScopeBinding) bool {
	return proposalToken.MatchString(binding.SessionID) && proposalToken.MatchString(binding.ScopeID) && proposalToken.MatchString(binding.DraftVersion) && proposalThread.MatchString(binding.ThreadID) && validPurpose(binding.Purpose)
}
func boundedPatch(parts []PatchPart) bool {
	if len(parts) > MaxPatchParts {
		return false
	}
	total := 0
	for _, part := range parts {
		if len(part.Kind) > 16 || len(part.ProtectedRef) > 58 || len(part.Text) > MaxPatchBytes {
			return false
		}
		total += len(part.Text)
		if total > MaxPatchBytes {
			return false
		}
	}
	return true
}
func (s *Service) validateScopeAuthority(ctx context.Context, current session, scope *capturedScope) error {
	if s.projector == nil || s.projector.ValidateCurrent(ctx, current.scopeAuthority(scope.view.ThreadID, scope.view.Purpose)) != nil {
		return ErrProjection
	}
	return s.validateProposalPrincipal(ctx, current)
}

func (current session) scopeAuthority(threadID, purpose string) ScopeAuthority {
	return ScopeAuthority{Principal: current.principal, ObjectID: current.objectID,
		ThreadID: threadID, Purpose: purpose, Workspace: current.workspace, Path: current.path}
}

func operationDigest(kind string, input any) string {
	body, _ := json.Marshal(input)
	digest := sha256.Sum256(append([]byte(kind+"\x00"), body...))
	return hex.EncodeToString(digest[:])
}
func (s *Service) Propose(ctx context.Context, input ProposeInput) (Proposal, error) {
	if s == nil {
		return Proposal{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.currentLocked(ctx, input.SessionID)
	if err != nil {
		return Proposal{}, err
	}
	if !proposalOperation.MatchString(input.OperationID) || !validScopeBinding(input.ScopeBinding) || !boundedPatch(input.Parts) {
		return Proposal{}, fileport.ErrInvalidInput
	}
	state := s.edits[input.SessionID]
	if state == nil {
		return Proposal{}, ErrDraftStale
	}
	scope, err := boundScope(state, input.ScopeBinding, false)
	if err != nil {
		return Proposal{}, err
	}
	if err := s.validateScopeAuthority(ctx, current, scope); err != nil {
		return Proposal{}, err
	}
	digest := operationDigest("propose", input)
	if previous, exists := state.operations[input.OperationID]; exists {
		if previous.digest != digest {
			return Proposal{}, fileport.ErrOperationMismatch
		}
		return proposalView(state.proposals[previous.proposalID]), nil
	}
	if len(state.proposals) >= MaxProposalsPerSession || len(state.operations) >= MaxProposalOperations {
		return Proposal{}, ErrCapacity
	}
	scope, err = boundScope(state, input.ScopeBinding, true)
	if err != nil {
		return Proposal{}, err
	}
	if scope.view.Purpose != "edit" {
		return Proposal{}, ErrScope
	}
	replacement, err := renderProposal(scope, input.Parts)
	if err != nil {
		return Proposal{}, err
	}
	if len(state.draft.Content)-(scope.endByte-scope.startByte)+len(replacement) > fileport.MaxTextBytes {
		return Proposal{}, fileport.ErrTooLarge
	}
	id, err := opaqueProposalID()
	if err != nil {
		return Proposal{}, err
	}
	if err := s.validateScopeAuthority(ctx, current, scope); err != nil {
		return Proposal{}, err
	}
	proposal := &Proposal{ProposalID: id, ScopeID: input.ScopeID, DraftVersion: input.DraftVersion, Parts: cloneParts(input.Parts), Status: "proposed"}
	state.proposals[id] = proposal
	state.operations[input.OperationID] = proposalOperationRecord{digest: digest, proposalID: id}
	return proposalView(proposal), nil
}
func (s *Service) Accept(ctx context.Context, input DecisionInput) (Decision, error) {
	return s.decide(ctx, input, true)
}
func (s *Service) Reject(ctx context.Context, input DecisionInput) (Decision, error) {
	return s.decide(ctx, input, false)
}
func (s *Service) decide(ctx context.Context, input DecisionInput, accept bool) (Decision, error) {
	if s == nil {
		return Decision{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.currentLocked(ctx, input.SessionID)
	if err != nil {
		return Decision{}, err
	}
	if !proposalOperation.MatchString(input.OperationID) || !proposalToken.MatchString(input.ProposalID) || !validScopeBinding(input.ScopeBinding) {
		return Decision{}, fileport.ErrInvalidInput
	}
	state := s.edits[input.SessionID]
	if state == nil {
		return Decision{}, ErrDraftStale
	}
	scope, err := boundScope(state, input.ScopeBinding, false)
	if err != nil {
		return Decision{}, err
	}
	if err := s.validateScopeAuthority(ctx, current, scope); err != nil {
		return Decision{}, err
	}
	kind := "reject"
	if accept {
		kind = "accept"
	}
	digest := operationDigest(kind, input)
	if previous, exists := state.operations[input.OperationID]; exists {
		if previous.digest != digest {
			return Decision{}, fileport.ErrOperationMismatch
		}
		return previous.decision, nil
	}
	if len(state.operations) >= MaxProposalOperations {
		return Decision{}, ErrCapacity
	}
	scope, err = boundScope(state, input.ScopeBinding, accept)
	if err != nil {
		return Decision{}, err
	}
	proposal := state.proposals[input.ProposalID]
	if proposal == nil || proposal.ScopeID != input.ScopeID || proposal.DraftVersion != input.DraftVersion || proposal.Status != "proposed" {
		return Decision{}, ErrProposal
	}
	decision := Decision{ProposalID: proposal.ProposalID, Status: "rejected", DraftVersion: state.draft.Version}
	content := state.draft.Content
	if accept {
		if scope.view.Purpose != "edit" {
			return Decision{}, ErrScope
		}
		replacement, err := renderProposal(scope, proposal.Parts)
		if err != nil {
			return Decision{}, err
		}
		content = state.draft.Content[:scope.startByte] + replacement + state.draft.Content[scope.endByte:]
		if !validDraftText(content) {
			return Decision{}, fileport.ErrTooLarge
		}
		version, err := opaqueProposalID()
		if err != nil {
			return Decision{}, err
		}
		decision.Status = "accepted"
		decision.DraftVersion = version
	}
	if err := s.validateScopeAuthority(ctx, current, scope); err != nil {
		return Decision{}, err
	}
	if accept {
		state.draft.Content = content
		state.draft.Version = decision.DraftVersion
		invalidateEditingScopes(state)
	}
	proposal.Status = decision.Status
	state.operations[input.OperationID] = proposalOperationRecord{digest: digest, proposalID: proposal.ProposalID, decision: decision}
	return decision, nil
}

// Called only after an existing CAS receipt verifies the current disk revision.
// A replay of the same confirmation is read-only with respect to new scopes.
func (s *Service) observeEditingCommitLocked(sessionID string, receipt fileport.Receipt, content string) {
	state := s.edits[sessionID]
	if state == nil || (state.lastCommitID == receipt.OperationID && state.lastCommitRevision == receipt.Revision) {
		return
	}
	invalidateEditingScopes(state)
	state.lastCommitID, state.lastCommitRevision = receipt.OperationID, receipt.Revision
	if state.draft.Content == content {
		state.draft.BaseRevision = receipt.Revision
	}
}
