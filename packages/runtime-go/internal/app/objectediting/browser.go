package objectediting

import (
	"context"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

// Browser selections use the existing identity, thread authority and projector.
// Only the bearer-protected Main transport may register engine-derived text or
// answer a read challenge. Neither the renderer nor a model chooses authority.
type BrowserCapture struct {
	ThreadID    string `json:"threadId"`
	DocumentID  string `json:"documentId"`
	SelectionID string `json:"selectionId"`
	Text        string `json:"text"`
}
type BrowserScope struct {
	ScopeID     string `json:"scopeId"`
	Workspace   string `json:"workspace"`
	ThreadID    string `json:"threadId"`
	DocumentID  string `json:"documentId"`
	SelectionID string `json:"selectionId"`
}
type browserSelection struct {
	view      BrowserScope
	authority ScopeAuthority
	parts     []PatchPart
	expires   time.Time
	checks    chan string
	pending   string
	result    chan bool
}

func (s *Service) CaptureBrowser(ctx context.Context, in BrowserCapture) (BrowserScope, error) {
	if s == nil || !proposalToken.MatchString(in.DocumentID) || !proposalToken.MatchString(in.SelectionID) ||
		!utf8.ValidString(in.Text) || strings.TrimSpace(in.Text) == "" || len(in.Text) > 16384 || len(utf16.Encode([]rune(in.Text))) > 4096 {
		return BrowserScope{}, ErrScope
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.principal(ctx)
	if err != nil {
		return BrowserScope{}, err
	}
	workspace, err := s.imageWorkspace(ctx, p, in.ThreadID)
	if err != nil {
		return BrowserScope{}, err
	}
	a := ScopeAuthority{Principal: p, ThreadID: in.ThreadID, Workspace: workspace, ObjectID: in.DocumentID, Path: "browser-selection", Purpose: "discuss"}
	freezer, ok := s.projector.(SelectionAuthorityFreezer)
	if !ok {
		return BrowserScope{}, ErrProjection
	}
	a.SecurityBinding, err = freezer.FreezeSelectionAuthority(ctx, a)
	if err != nil || a.SecurityBinding == "" {
		return BrowserScope{}, ErrProjection
	}
	ranges, err := s.projector.AuthorizeAndProject(ctx, ProjectionInput{ScopeAuthority: a, Text: in.Text})
	if err != nil {
		return BrowserScope{}, err
	}
	parts, err := imageNoteParts(in.Text, ranges)
	if err != nil || s.projector.ValidateCurrent(ctx, a) != nil {
		return BrowserScope{}, ErrProjection
	}
	if s.browser == nil {
		s.browser = make(map[string]*browserSelection)
	}
	s.pruneBrowserLocked()
	if len(s.browser) >= 64 {
		return BrowserScope{}, ErrCapacity
	}
	id, err := opaqueProposalID()
	if err != nil {
		return BrowserScope{}, err
	}
	v := BrowserScope{id, workspace, in.ThreadID, in.DocumentID, in.SelectionID}
	s.browser[id] = &browserSelection{view: v, authority: a, parts: parts, expires: time.Now().Add(5 * time.Minute), checks: make(chan string, 1)}
	return v, nil
}

func (s *Service) dropBrowserLocked(id string) {
	if b := s.browser[id]; b != nil {
		if b.result != nil {
			select {
			case b.result <- false:
			default:
			}
		}
		delete(s.browser, id)
	}
}
func (s *Service) pruneBrowserLocked() {
	for id, b := range s.browser {
		if !time.Now().Before(b.expires) {
			s.dropBrowserLocked(id)
		}
	}
}
func (s *Service) browserCurrentLocked(ctx context.Context, v BrowserScope) (*browserSelection, error) {
	s.pruneBrowserLocked()
	b := s.browser[v.ScopeID]
	if b == nil || b.view != v || ctx.Err() != nil || s.identity.ValidateCurrent(ctx, b.authority.Principal) != nil || s.projector.ValidateCurrent(ctx, b.authority) != nil {
		return nil, ErrScope
	}
	return b, nil
}
func (s *Service) RevokeBrowser(ctx context.Context, v BrowserScope) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.browserCurrentLocked(ctx, v); err != nil {
		return err
	}
	s.dropBrowserLocked(v.ScopeID)
	return nil
}

// NextBrowserCheck is a bounded private long poll, not a grant or a source read.
func (s *Service) NextBrowserCheck(ctx context.Context, v BrowserScope) (string, error) {
	s.mu.Lock()
	b, err := s.browserCurrentLocked(ctx, v)
	s.mu.Unlock()
	if err != nil {
		return "", err
	}
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case id := <-b.checks:
		return id, nil
	case <-ctx.Done():
		return "", ctx.Err()
	case <-timer.C:
		return "", nil
	}
}
func (s *Service) AnswerBrowserCheck(ctx context.Context, v BrowserScope, nonce string, current bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := s.browserCurrentLocked(ctx, v)
	if err != nil {
		return err
	}
	if nonce == "" || b.pending != nonce || b.result == nil {
		return ErrScope
	}
	// Consume once. An old answer can never settle a later read of this scope.
	b.pending = ""
	b.result <- current
	return nil
}

// ValidateBrowserScope binds every private transport field before issuing a
// challenge; a foreign binding must not consume a legitimate pending read.
func (s *Service) ValidateBrowserScope(ctx context.Context, v BrowserScope) error {
	s.mu.Lock()
	_, err := s.browserCurrentLocked(ctx, v)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	_, found, err := s.ReadBrowserScopeForModel(ctx, v.ThreadID, v.ScopeID)
	if !found {
		return ErrScope
	}
	return err
}
func (s *Service) ReadBrowserScopeForModel(ctx context.Context, thread, id string) (any, bool, error) {
	if s == nil {
		return nil, false, nil
	}
	s.mu.Lock()
	s.pruneBrowserLocked()
	b := s.browser[id]
	if b == nil {
		s.mu.Unlock()
		return nil, false, nil
	}
	if thread != b.view.ThreadID {
		s.mu.Unlock()
		return nil, true, ErrScope
	}
	if _, err := s.browserCurrentLocked(ctx, b.view); err != nil {
		s.mu.Unlock()
		return nil, true, err
	}
	if b.result != nil {
		s.mu.Unlock()
		return nil, true, ErrCapacity
	}
	nonce, err := opaqueProposalID()
	if err != nil {
		s.mu.Unlock()
		return nil, true, err
	}
	result := make(chan bool, 1)
	b.pending = nonce
	b.result = result
	// A timed-out undelivered challenge must not starve the next read.
	select {
	case <-b.checks:
	default:
	}
	b.checks <- nonce
	s.mu.Unlock()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	current := false
	select {
	case current = <-result:
	case <-ctx.Done():
	case <-timer.C:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.browser[id] != b || b.result != result {
		return nil, true, ErrScope
	}
	b.pending = ""
	b.result = nil
	if !current {
		s.dropBrowserLocked(id)
		return nil, true, ErrScope
	}
	if _, err := s.browserCurrentLocked(ctx, b.view); err != nil {
		return nil, true, err
	}
	return map[string]any{"kind": "browser-selection", "scopeId": id, "editable": false, "parts": cloneParts(b.parts), "source": "selected text only; page visual revision is not established"}, true, nil
}
func (s *Service) HasBrowserScopes() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneBrowserLocked()
	return len(s.browser) > 0
}
