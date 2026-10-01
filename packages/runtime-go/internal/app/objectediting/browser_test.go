package objectediting

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	identity "analytix.local/runtime-go/internal/domain/identity"
)

type browserProjector struct {
	testSelectionProjector
	binding string
}

func (p *browserProjector) ResolveImageWorkspace(_ context.Context, _ identity.PrincipalV1, thread string) (string, error) {
	if thread != "thread-1" {
		return "", ErrScope
	}
	return "/workspace", nil
}
func (p *browserProjector) FreezeSelectionAuthority(ctx context.Context, a ScopeAuthority) (string, error) {
	return p.binding, p.ValidateCurrent(ctx, a)
}
func (p *browserProjector) ValidateCurrent(ctx context.Context, a ScopeAuthority) error {
	if a.SecurityBinding != "" && a.SecurityBinding != p.binding {
		return ErrScope
	}
	return p.testSelectionProjector.ValidateCurrent(ctx, a)
}
func browserFixture(t *testing.T) (*Service, *browserProjector, BrowserScope) {
	t.Helper()
	_, identity, files := fixture(t)
	p := &browserProjector{binding: "current"}
	s := NewWithProjector(identity, files, p)
	v, err := s.CaptureBrowser(context.Background(), BrowserCapture{"thread-1", strings.Repeat("a", 48), strings.Repeat("b", 48), "Hello Alice"})
	if err != nil {
		t.Fatal(err)
	}
	return s, p, v
}
func browserRead(t *testing.T, s *Service, v BrowserScope) (string, <-chan error) {
	t.Helper()
	out := make(chan error, 1)
	go func() {
		value, found, err := s.ReadBrowserScopeForModel(context.Background(), v.ThreadID, v.ScopeID)
		if err == nil {
			raw, _ := json.Marshal(value)
			if !found || strings.Contains(string(raw), "Alice") || !strings.Contains(string(raw), "Hello") {
				err = errors.New("projection contract failed")
			}
		}
		out <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	nonce, err := s.NextBrowserCheck(ctx, v)
	if err != nil || nonce == "" {
		t.Fatal("no read challenge", err)
	}
	return nonce, out
}
func TestBrowserReadRequiresFreshOneUseMainProof(t *testing.T) {
	s, _, v := browserFixture(t)
	nonce, out := browserRead(t, s, v)
	if s.AnswerBrowserCheck(context.Background(), v, strings.Repeat("f", 48), true) == nil {
		t.Fatal("unissued nonce")
	}
	if err := s.AnswerBrowserCheck(context.Background(), v, nonce, true); err != nil {
		t.Fatal(err)
	}
	if err := <-out; err != nil {
		t.Fatal(err)
	}
	next, out := browserRead(t, s, v)
	if next == nonce || s.AnswerBrowserCheck(context.Background(), v, nonce, true) == nil {
		t.Fatal("replayed nonce")
	}
	if err := s.AnswerBrowserCheck(context.Background(), v, next, true); err != nil {
		t.Fatal(err)
	}
	if err := <-out; err != nil {
		t.Fatal(err)
	}
}
func TestBrowserAuthorityAndReadInvalidation(t *testing.T) {
	for _, mode := range []string{"wrong-thread", "changed-source", "revoke", "authority", "expiry", "binding", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			s, p, v := browserFixture(t)
			if mode == "wrong-thread" {
				if _, found, err := s.ReadBrowserScopeForModel(context.Background(), "other", v.ScopeID); !found || err == nil {
					t.Fatal("cross thread")
				}
				return
			}
			if mode == "binding" {
				p.binding = "new"
				if _, _, err := s.ReadBrowserScopeForModel(context.Background(), v.ThreadID, v.ScopeID); err == nil {
					t.Fatal("binding survived")
				}
				return
			}
			if mode == "cancel" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				if _, _, err := s.ReadBrowserScopeForModel(ctx, v.ThreadID, v.ScopeID); err == nil {
					t.Fatal("cancel ignored")
				}
				return
			}
			nonce, out := browserRead(t, s, v)
			switch mode {
			case "changed-source":
				if err := s.AnswerBrowserCheck(context.Background(), v, nonce, false); err != nil {
					t.Fatal(err)
				}
			case "revoke":
				if err := s.RevokeBrowser(context.Background(), v); err != nil {
					t.Fatal(err)
				}
			case "authority":
				s.mu.Lock()
				p.denied = true
				s.mu.Unlock()
				if s.AnswerBrowserCheck(context.Background(), v, nonce, true) == nil {
					t.Fatal("revoked authority answered")
				}
				s.mu.Lock()
				s.dropBrowserLocked(v.ScopeID)
				s.mu.Unlock()
			case "expiry":
				s.mu.Lock()
				s.browser[v.ScopeID].expires = time.Now().Add(-time.Second)
				s.mu.Unlock()
				if s.HasBrowserScopes() {
					t.Fatal("expired scope")
				}
			}
			if err := <-out; err == nil {
				t.Fatal("invalid read delivered")
			}
		})
	}
}
func TestBrowserConcurrentReadsAndCaptureBudgets(t *testing.T) {
	s, _, v := browserFixture(t)
	nonce, out := browserRead(t, s, v)
	if _, _, err := s.ReadBrowserScopeForModel(context.Background(), v.ThreadID, v.ScopeID); !errors.Is(err, ErrCapacity) {
		t.Fatal("concurrent read admitted")
	}
	if s.AnswerBrowserCheck(context.Background(), v, nonce, true) != nil {
		t.Fatal("answer")
	}
	if err := <-out; err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"", strings.Repeat("x", 4097), strings.Repeat("😀", 2049), string([]byte{0xff})} {
		if _, err := s.CaptureBrowser(context.Background(), BrowserCapture{v.ThreadID, v.DocumentID, v.SelectionID, text}); err == nil {
			t.Fatal("invalid text admitted")
		}
	}
}
