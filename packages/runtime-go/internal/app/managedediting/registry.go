// Package managedediting protects explicitly captured editing baselines from
// runtime-owned mutation paths. It is not OS containment of external processes.
package managedediting

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"analytix.local/runtime-go/internal/app/workspacemutation"
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

type capture struct {
	path      string
	identity  os.FileInfo
	ancestors []os.FileInfo
	refs      int
}

// Registry must be composed once per runtime with the same Coordinator used by
// all mutations and checkpoint restore. It never creates another coordinator.
// Lock order is Coordinator -> mu. Releases take only mu, never Coordinator.
type Registry struct {
	coordinator    *workspacemutation.Coordinator
	mu             sync.Mutex
	captures       map[string]*capture
	opaqueObserved bool
}

func New(coordinator *workspacemutation.Coordinator) *Registry {
	return &Registry{coordinator: coordinator, captures: make(map[string]*capture)}
}

// Capture retains a reference until its idempotent release is called. A second
// capture for the same session/path/inode is an independent reference; rebinding
// a live session to a different path or externally replaced inode is rejected.
func (r *Registry) Capture(ctx context.Context, sessionID, canonicalAbsolutePath string) (func(), error) {
	return r.withCapture(ctx, sessionID, canonicalAbsolutePath, nil)
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
	return r.withCapture(ctx, sessionID, canonicalAbsolutePath, readBaseline)
}

func (r *Registry) withCapture(ctx context.Context, sessionID, path string, readBaseline func() error) (func(), error) {
	if r == nil || r.coordinator == nil || ctx == nil || !sessionPattern.MatchString(sessionID) {
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
	if r.opaqueObserved {
		r.mu.Unlock()
		return nil, ErrUncontainedExecution
	}
	identity, ancestors, err := inspectPath(path, false)
	if err != nil || identity == nil || !identity.Mode().IsRegular() || !singleLink(identity) {
		r.mu.Unlock()
		return nil, ErrUnsafePath
	}
	current := r.captures[sessionID]
	if current != nil {
		if current.path != path || !os.SameFile(current.identity, identity) {
			r.mu.Unlock()
			return nil, ErrSessionMismatch
		}
	} else {
		current = &capture{path: path, identity: identity, ancestors: ancestors}
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
	if len(paths) == 0 {
		return ErrUnsafePath
	}
	for _, path := range paths {
		for _, current := range r.captures {
			// Keep protecting the original name even after external removal/replacement.
			if containsPath(path, current.path) {
				return ErrMutationBlocked
			}
		}
		identity, _, err := inspectPath(path, true)
		if err != nil {
			return ErrUnsafePath
		}
		for _, current := range r.captures {
			latest, parents, err := inspectPath(current.path, false)
			if err != nil || latest == nil || !latest.Mode().IsRegular() {
				return ErrUnsafePath
			}
			if identity == nil {
				continue
			}
			if os.SameFile(identity, current.identity) || os.SameFile(identity, latest) {
				return ErrMutationBlocked
			}
			// Compare both captured and current ancestors, covering case/volume aliases
			// and externally renamed directories without trusting path spelling alone.
			for _, parent := range current.ancestors {
				if os.SameFile(identity, parent) {
					return ErrMutationBlocked
				}
			}
			for _, parent := range parents {
				if os.SameFile(identity, parent) {
					return ErrMutationBlocked
				}
			}
		}
	}
	return nil
}

// BeginOpaque records that an uncontained process/tool may continue writing
// after its invocation returns. There is deliberately no decrement/reset API.
// Repeated opaque tools remain usable, but capture requires a fresh runtime
// registry after normal application restart if no containment proof exists.
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

func containsPath(parent, path string) bool {
	if !filepath.IsAbs(parent) || filepath.Clean(parent) != parent {
		return false
	}
	relative, err := filepath.Rel(parent, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

// inspectPath rejects symlinks in every existing component, including the final
// component. It never normalizes an ambiguous caller path into an allowed path.
func inspectPath(path string, allowMissing bool) (os.FileInfo, []os.FileInfo, error) {
	if path == "" || strings.ContainsRune(path, 0) || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, nil, ErrUnsafePath
	}
	root := filepath.VolumeName(path) + string(filepath.Separator)
	current := root
	info, err := os.Lstat(current)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, ErrUnsafePath
	}
	var ancestors []os.FileInfo
	if path == root {
		return info, ancestors, nil
	}
	components := strings.Split(strings.TrimPrefix(path, root), string(filepath.Separator))
	for index, component := range components {
		ancestors = append(ancestors, info)
		current = filepath.Join(current, component)
		info, err = os.Lstat(current)
		if err != nil {
			if allowMissing && errors.Is(err, os.ErrNotExist) {
				return nil, ancestors, nil
			}
			return nil, nil, ErrUnsafePath
		}
		if info.Mode()&os.ModeSymlink != 0 || (index < len(components)-1 && !info.IsDir()) {
			return nil, nil, ErrUnsafePath
		}
		resolved, err := filepath.EvalSymlinks(current)
		if err != nil || resolved != current {
			return nil, nil, ErrUnsafePath
		}
	}
	return info, ancestors, nil
}

// Unix Stat_t exposes Nlink; some other FileInfo implementations expose
// NumberOfLinks. When the platform cannot prove nlink==1 we fail closed rather
// than equate os.SameFile (known identity comparison) with absence of aliases.
func singleLink(info os.FileInfo) bool {
	value := reflect.ValueOf(info.Sys())
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return false
	}
	for _, name := range []string{"Nlink", "NumberOfLinks"} {
		field := value.FieldByName(name)
		switch field.Kind() {
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return field.Uint() == 1
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return field.Int() == 1
		}
	}
	return false
}
