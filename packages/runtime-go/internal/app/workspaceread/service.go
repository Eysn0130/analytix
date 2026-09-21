// Package workspaceread binds optional retrieval to the existing Core thread
// authority. A binding is only a freshness assertion, never a permission token.
package workspaceread

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	fileport "analytix.local/runtime-go/internal/ports/workspaceread"
)

type Scope struct {
	ThreadID  string
	Workspace string
	Binding   string
}

type Authority interface {
	Current(context.Context, string) (Scope, error)
}

type Service struct {
	Files     fileport.Files
	authority Authority
}

// BindAuthority is called once during production composition, before serving.
func (s *Service) BindAuthority(authority Authority) error {
	if s == nil || authority == nil || s.authority != nil {
		return fileport.ErrUnavailable
	}
	s.authority = authority
	return nil
}

type Request struct {
	Action     string `json:"action"`
	ThreadID   string `json:"threadId"`
	Binding    string `json:"binding,omitempty"`
	IncludePDF bool   `json:"includePdf,omitempty"`
}

var threadIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

// Valid checks values at the service boundary. The HTTP adapter separately
// requires the exact field set and JSON types for each action.
func (r Request) Valid() bool {
	if !threadIDPattern.MatchString(r.ThreadID) {
		return false
	}
	switch r.Action {
	case "authorize":
		return r.Binding == "" && !r.IncludePDF
	case "validate":
		return len(r.Binding) == 64 && domainsecurity.IsSHA256Hex(r.Binding) && !r.IncludePDF
	case "scan":
		return len(r.Binding) == 64 && domainsecurity.IsSHA256Hex(r.Binding)
	default:
		return false
	}
}

type Snapshot struct {
	ThreadID  string          `json:"threadId"`
	Workspace string          `json:"workspace"`
	Binding   string          `json:"binding"`
	Files     []fileport.File `json:"files,omitempty"`
}

func binding(scope Scope, root fileport.Root) string {
	body, _ := json.Marshal(struct {
		Scope Scope
		Root  fileport.Root
	}{scope, root})
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func (s *Service) Read(ctx context.Context, request Request) (Snapshot, error) {
	if s == nil || s.Files == nil || s.authority == nil || ctx == nil || ctx.Err() != nil || !request.Valid() {
		return Snapshot{}, fileport.ErrUnavailable
	}
	scope, err := s.authority.Current(ctx, request.ThreadID)
	if err != nil || scope.ThreadID != request.ThreadID || scope.Binding == "" || scope.Workspace == "" {
		return Snapshot{}, fileport.ErrUnavailable
	}
	root, err := s.Files.InspectRoot(ctx, scope.Workspace)
	if err != nil || root.Workspace == "" || root.Identity == "" || root.Policy == "" {
		return Snapshot{}, fileport.ErrUnavailable
	}
	expected := binding(scope, root)
	if request.Action != "authorize" && request.Binding != expected {
		return Snapshot{}, fileport.ErrUnavailable
	}
	current := func() error {
		if ctx.Err() != nil {
			return fileport.ErrUnavailable
		}
		now, err := s.authority.Current(ctx, request.ThreadID)
		if err != nil || now != scope {
			return fileport.ErrUnavailable
		}
		actual, err := s.Files.InspectRoot(ctx, scope.Workspace)
		if err != nil || actual != root {
			return fileport.ErrUnavailable
		}
		return nil
	}
	if current() != nil {
		return Snapshot{}, fileport.ErrUnavailable
	}
	// Keep the trusted thread spelling for the renderer assertion; root identity
	// and every scan still use the adapter-canonical root (including macOS aliases).
	result := Snapshot{ThreadID: scope.ThreadID, Workspace: scope.Workspace, Binding: expected}
	if request.Action == "scan" {
		budget := 250 * time.Millisecond
		if request.IncludePDF {
			budget = 2500 * time.Millisecond
		}
		result.Files, err = s.Files.Scan(ctx, root, request.IncludePDF, budget, current)
		if err != nil {
			return Snapshot{}, fileport.ErrUnavailable
		}
	}
	if current() != nil {
		return Snapshot{}, fileport.ErrUnavailable
	}
	return result, nil
}
