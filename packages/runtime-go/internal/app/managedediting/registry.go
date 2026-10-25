// Package managedediting protects explicitly captured editing baselines from
// runtime-owned mutation paths. It is not OS containment of external processes.
package managedediting

import (
	"context"
	"errors"
	"regexp"
	"sync"

	"analytix.local/runtime-go/internal/app/workspacemutation"
	filesport "analytix.local/runtime-go/internal/ports/managedediting"
)

var (
	ErrUnavailable          = errors.New("managed_editing_unavailable")
	ErrUnsafePath           = errors.New("managed_editing_unsafe_path")
	ErrSessionMismatch      = errors.New("managed_editing_session_mismatch")
	ErrMutationBlocked      = errors.New("managed_editing_mutation_blocked")
	ErrCaptureActive        = errors.New("managed_editing_capture_active")
	ErrCaptureFailed        = errors.New("managed_editing_capture_failed")
	ErrUncontainedExecution = errors.New("managed_editing_uncontained_execution")
	sessionPattern          = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)
)

type capturePurpose uint8

const (
	strictCapture capturePurpose = iota + 1
	manualCapture
)

type capture struct {
	purpose   capturePurpose
	path      string
	identity  filesport.Identity
	ancestors []filesport.Identity
	refs      int
}

// Registry must be composed once per runtime with the same Coordinator used by
// all mutations and checkpoint restore. It never creates another coordinator.
// Lock order is Coordinator -> mu. Releases take only mu, never Coordinator.
type Registry struct {
	coordinator    *workspacemutation.Coordinator
	files          filesport.Files
	mu             sync.Mutex
	captures       map[string]*capture
	opaqueObserved bool
}

func New(coordinator *workspacemutation.Coordinator, files filesport.Files) *Registry {
	return &Registry{coordinator: coordinator, files: files, captures: make(map[string]*capture)}
}

// Capture retains a reference until its idempotent release is called. A second
// capture for the same session/path/inode is an independent reference; rebinding
// a live session to a different path or externally replaced inode is rejected.
func (r *Registry) Capture(ctx context.Context, sessionID, canonicalAbsolutePath string) (func(), error) {
	return r.withCapture(ctx, sessionID, canonicalAbsolutePath, nil, strictCapture)
}

// WithCapture registers the lease and calls readBaseline while still holding the
// shared Coordinator. This makes registration + reading atomic against governed
// mutations. The callback must only read: do not acquire Coordinator recursively
// or call Capture/BeginOpaque. On callback failure the new reference is released;
// callers receive a fixed error, never the private callback cause.
func (r *Registry) WithCapture(ctx context.Context, sessionID, canonicalAbsolutePath string, readBaseline func() error) (func(), error) {
	if readBaseline == nil {
		return nil, ErrCaptureFailed
	}
	return r.withCapture(ctx, sessionID, canonicalAbsolutePath, readBaseline, strictCapture)
}

// WithManualCapture is trusted Core admission for an explicit user transaction.
// It preserves file identity, the shared coordinator and governed-mutation guard,
// but makes no exclusivity promise against previously started external writers.
// Their changes must conflict at the existing native version/CAS boundary.
func (r *Registry) WithManualCapture(ctx context.Context, sessionID, path string, readBaseline func() error) (func(), error) {
	if readBaseline == nil {
		return nil, ErrCaptureFailed
	}
	return r.withCapture(ctx, sessionID, path, readBaseline, manualCapture)
}

func (r *Registry) withCapture(ctx context.Context, sessionID, path string, readBaseline func() error, purpose capturePurpose) (func(), error) {
	if r == nil || r.coordinator == nil || r.files == nil || ctx == nil || !sessionPattern.MatchString(sessionID) || (purpose != strictCapture && purpose != manualCapture) {
		return nil, ErrUnavailable
	}
	unlock, err := r.coordinator.Acquire(ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer unlock()
	if ctx.Err() != nil {
		return nil, ErrUnavailable
	}
	r.mu.Lock()
	if purpose == strictCapture && r.opaqueObserved {
		r.mu.Unlock()
		return nil, ErrUncontainedExecution
	}
	identity, ancestors, err := r.files.Inspect(path, false)
	if err != nil || identity == nil || !identity.Regular() || !identity.SingleLink() {
		r.mu.Unlock()
		return nil, ErrUnsafePath
	}
	for _, held := range r.captures {
		if held.purpose != purpose && (held.path == path || r.files.SameFile(held.identity, identity)) {
			r.mu.Unlock()
			return nil, ErrCaptureActive
		}
	}
	current := r.captures[sessionID]
	if current != nil {
		if current.purpose != purpose || current.path != path || !r.files.SameFile(current.identity, identity) {
			r.mu.Unlock()
			return nil, ErrSessionMismatch
		}
	} else {
		current = &capture{purpose: purpose, path: path, identity: identity, ancestors: ancestors}
		r.captures[sessionID] = current
	}
	current.refs++
	r.mu.Unlock()
	var once sync.Once
	release := func() {
		once.Do(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			if r.captures[sessionID] != current {
				return
			}
			current.refs--
			if current.refs == 0 {
				delete(r.captures, sessionID)
			}
		})
	}
	complete := false
	// Also remove the new lease if a trusted callback panics; the coordinator's
	// deferred unlock still runs. Panic handling belongs to the existing caller.
	defer func() {
		if !complete {
			release()
		}
	}()
	if readBaseline != nil && readBaseline() != nil {
		return nil, ErrCaptureFailed
	}
	if ctx.Err() != nil {
		return nil, ErrUnavailable
	}
	complete = true
	return release, nil
}

// CheckMutation MUST run with the injected Coordinator already held, continuing
// through the actual mutation. Supplying every source/destination path is the
// caller's responsibility. No capture means this guard does not restrict tools.
// Missing destinations are allowed only beneath verified non-symlink prefixes.
func (r *Registry) CheckMutation(paths ...string) error {
	if r == nil || r.coordinator == nil {
		return ErrUnavailable
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.captures) == 0 {
		return nil
	}
	if r.files == nil {
		return ErrUnavailable
	}
	if len(paths) == 0 {
		return ErrUnsafePath
	}
	for _, path := range paths {
		for _, current := range r.captures {
			// Keep protecting the original name even after external removal/replacement.
			if r.files.Contains(path, current.path) {
				return ErrMutationBlocked
			}
		}
		identity, _, err := r.files.Inspect(path, true)
		if err != nil {
			return ErrUnsafePath
		}
		for _, current := range r.captures {
			latest, parents, err := r.files.Inspect(current.path, false)
			if err != nil || latest == nil || !latest.Regular() {
				return ErrUnsafePath
			}
			if identity == nil {
				continue
			}
			if r.files.SameFile(identity, current.identity) || r.files.SameFile(identity, latest) {
				return ErrMutationBlocked
			}
			// Compare both captured and current ancestors, covering case/volume aliases
			// and externally renamed directories without trusting path spelling alone.
			for _, parent := range current.ancestors {
				if r.files.SameFile(identity, parent) {
					return ErrMutationBlocked
				}
			}
			for _, parent := range parents {
				if r.files.SameFile(identity, parent) {
					return ErrMutationBlocked
				}
			}
		}
	}
	return nil
}

// BeginOpaque records that an uncontained process/tool may continue writing
// after its invocation returns. There is deliberately no decrement/reset API.
// Repeated opaque tools remain usable. Strict Agent capture stays unavailable;
// explicit manual transactions retain optimistic version/CAS semantics.
func (r *Registry) BeginOpaque(ctx context.Context) error {
	if r == nil || r.coordinator == nil || ctx == nil {
		return ErrUnavailable
	}
	unlock, err := r.coordinator.Acquire(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer unlock()
	if ctx.Err() != nil {
		return ErrUnavailable
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.captures) > 0 {
		return ErrCaptureActive
	}
	r.opaqueObserved = true
	return nil
}

func (r *Registry) HasCaptures() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.captures) > 0
}
