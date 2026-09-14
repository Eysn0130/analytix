package managedediting

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"analytix.local/runtime-go/internal/app/workspacemutation"
)

func fixture(t *testing.T) (*Registry, *workspacemutation.Coordinator, string, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "documents")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "document.txt")
	if err := os.WriteFile(path, []byte("private baseline"), 0600); err != nil {
		t.Fatal(err)
	}
	coordinator := workspacemutation.NewCoordinator()
	return New(coordinator), coordinator, root, path
}
func captureFile(t *testing.T, r *Registry, session, path string) func() {
	t.Helper()
	release, err := r.Capture(context.Background(), session, path)
	if err != nil {
		t.Fatalf("capture failed: %v", err)
	}
	t.Cleanup(release)
	return release
}
func checkMutation(t *testing.T, r *Registry, c *workspacemutation.Coordinator, paths ...string) error {
	t.Helper()
	unlock, err := c.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	return r.CheckMutation(paths...)
}

func TestRegistryProtectsPathAncestorsAndAllowsOtherMutations(t *testing.T) {
	r, c, root, path := fixture(t)
	if err := checkMutation(t, r, c, "relative", filepath.Join(root, "missing")); err != nil {
		t.Fatal("no-capture tools restricted")
	}
	release := captureFile(t, r, "session-1", path)
	for _, target := range []string{path, filepath.Dir(path), root, filepath.VolumeName(root) + string(filepath.Separator)} {
		if !errors.Is(checkMutation(t, r, c, target), ErrMutationBlocked) {
			t.Fatal("captured target or ancestor allowed")
		}
	}
	other := filepath.Join(root, "other.txt")
	if err := os.WriteFile(other, []byte("other"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{other, filepath.Join(filepath.Dir(path), "new.txt"), filepath.Join(root, "new", "nested.txt")} {
		if err := checkMutation(t, r, c, target); err != nil {
			t.Fatalf("unrelated mutation blocked: %v", err)
		}
	}
	if !errors.Is(checkMutation(t, r, c, other, path), ErrMutationBlocked) {
		t.Fatal("multi-path destination missed")
	}
	if !errors.Is(checkMutation(t, r, c, "relative"), ErrUnsafePath) {
		t.Fatal("ambiguous path allowed")
	}
	release()
	release()
	if r.HasCaptures() || checkMutation(t, r, c, path) != nil {
		t.Fatal("released capture still blocks mutation")
	}
}

func TestRegistryCaptureReferenceAndSessionBinding(t *testing.T) {
	r, c, root, path := fixture(t)
	first := captureFile(t, r, "session-1", path)
	second := captureFile(t, r, "session-1", path)
	other := filepath.Join(root, "other.txt")
	_ = os.WriteFile(other, []byte("other"), 0600)
	if release, err := r.Capture(context.Background(), "session-1", other); release != nil || !errors.Is(err, ErrSessionMismatch) {
		t.Fatal("session rebound")
	}
	first()
	first()
	if !r.HasCaptures() || !errors.Is(checkMutation(t, r, c, path), ErrMutationBlocked) {
		t.Fatal("first release removed second lease")
	}
	second()
	if r.HasCaptures() {
		t.Fatal("references not released")
	}
	replacement := captureFile(t, r, "session-1", other)
	first()
	second()
	if !r.HasCaptures() {
		t.Fatal("old releases removed replacement lease")
	}
	replacement()
}

func TestRegistryRejectsHardlinksAndSymlinkPaths(t *testing.T) {
	r, c, root, path := fixture(t)
	hard := filepath.Join(root, "hard.txt")
	if err := os.Link(path, hard); err != nil {
		t.Fatal(err)
	}
	if release, err := r.Capture(context.Background(), "hard", path); release != nil || !errors.Is(err, ErrUnsafePath) {
		t.Fatal("multiply-linked capture allowed")
	}
	if err := os.Remove(hard); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(root, "symbol.txt")
	if err := os.Symlink(path, symlink); err != nil {
		t.Fatal(err)
	}
	parentAlias := filepath.Join(root, "alias")
	if err := os.Symlink(filepath.Dir(path), parentAlias); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{symlink, filepath.Join(parentAlias, filepath.Base(path)), filepath.Dir(path), filepath.Join(root, "missing"), filepath.Dir(path) + string(filepath.Separator) + ".." + string(filepath.Separator) + "documents" + string(filepath.Separator) + filepath.Base(path)} {
		if release, err := r.Capture(context.Background(), "unsafe", target); release != nil || !errors.Is(err, ErrUnsafePath) {
			t.Fatal("unsafe capture allowed")
		}
	}
	captureFile(t, r, "session-1", path)
	if err := os.Link(path, hard); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(checkMutation(t, r, c, hard), ErrMutationBlocked) {
		t.Fatal("hardlink identity alias allowed")
	}
	for _, target := range []string{symlink, filepath.Join(parentAlias, filepath.Base(path)), filepath.Join(parentAlias, "not-created.txt")} {
		if !errors.Is(checkMutation(t, r, c, target), ErrUnsafePath) {
			t.Fatal("symbolic mutation alias allowed")
		}
	}
}

func TestRegistryExternalReplacementKeepsNameAndBothIdentitiesProtected(t *testing.T) {
	r, c, root, path := fixture(t)
	captureFile(t, r, "session-1", path)
	old := filepath.Join(root, "renamed-original.txt")
	if err := os.Rename(path, old); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("external replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	newAlias := filepath.Join(root, "replacement-alias.txt")
	if err := os.Link(path, newAlias); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{path, old, newAlias} {
		if !errors.Is(checkMutation(t, r, c, target), ErrMutationBlocked) {
			t.Fatal("external replacement lost protection")
		}
	}
	if release, err := r.Capture(context.Background(), "session-1", path); release != nil || err == nil {
		t.Fatal("replaced inode silently rebound")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(checkMutation(t, r, c, path), ErrMutationBlocked) {
		t.Fatal("missing target name unprotected")
	}
	if !errors.Is(checkMutation(t, r, c, filepath.Join(root, "other.txt")), ErrUnsafePath) {
		t.Fatal("missing captured identity failed open")
	}
}

func TestRegistryWithCaptureHoldsSharedCoordinatorThroughBaselineRead(t *testing.T) {
	r, c, _, path := fixture(t)
	entered := make(chan struct{})
	allowRead := make(chan struct{})
	finished := make(chan error, 1)
	var release func()
	go func() {
		var err error
		release, err = r.WithCapture(context.Background(), "session-1", path, func() error {
			if !r.HasCaptures() {
				return errors.New("capture absent during read")
			}
			close(entered)
			<-allowRead
			_, err := os.ReadFile(path)
			return err
		})
		finished <- err
	}()
	<-entered
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Acquire(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal("baseline callback did not retain coordinator")
	}
	waiting := make(chan struct{})
	mutated := make(chan error, 1)
	go func() {
		close(waiting)
		unlock, err := c.Acquire(context.Background())
		if err == nil {
			err = r.CheckMutation(path)
			unlock()
		}
		mutated <- err
	}()
	<-waiting
	select {
	case <-mutated:
		t.Fatal("mutation passed while read holds coordinator")
	default:
	}
	close(allowRead)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := <-mutated; !errors.Is(err, ErrMutationBlocked) {
		t.Fatal("mutation did not observe registered capture")
	}
	// Release must not acquire Coordinator in reverse order.
	unlock, err := c.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	go func() { release(); close(released) }()
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("release attempted coordinator recursion")
	}
	unlock()
}

func TestRegistryCallbackFailureClosesLeaseWithoutPrivateCause(t *testing.T) {
	r, c, _, path := fixture(t)
	release, err := r.WithCapture(context.Background(), "session-1", path, func() error { return errors.New("private content /sensitive/path") })
	if release != nil || !errors.Is(err, ErrCaptureFailed) || strings.Contains(err.Error(), "private") {
		t.Fatal("callback cause escaped")
	}
	if r.HasCaptures() {
		t.Fatal("failed callback retained capture")
	}
	unlock, err := c.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	// A failed extra reference must leave an existing reference intact.
	captureFile(t, r, "session-1", path)
	_, err = r.WithCapture(context.Background(), "session-1", path, func() error { return errors.New("private") })
	if !errors.Is(err, ErrCaptureFailed) || !r.HasCaptures() {
		t.Fatal("failed extra reference removed existing capture")
	}
}

func TestRegistryOpaqueObservationPersistsWithoutBlockingOtherTools(t *testing.T) {
	r, c, _, path := fixture(t)
	release := captureFile(t, r, "session-1", path)
	if !errors.Is(r.BeginOpaque(context.Background()), ErrCaptureActive) {
		t.Fatal("opaque tool started during capture")
	}
	release()
	// A denied start must not contaminate the registry.
	release = captureFile(t, r, "session-2", path)
	release()
	if err := r.BeginOpaque(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := r.BeginOpaque(context.Background()); err != nil {
		t.Fatal("opaque tools permanently disabled")
	}
	if r.HasCaptures() || checkMutation(t, r, c, path) != nil {
		t.Fatal("other tools restricted without captures")
	}
	if release, err := r.Capture(context.Background(), "session-3", path); release != nil || !errors.Is(err, ErrUncontainedExecution) {
		t.Fatal("opaque invocation return reset observed state")
	}
}

func TestRegistryCaptureOpaqueRaceHasOneWinner(t *testing.T) {
	_, _, _, path := fixture(t)
	for index := 0; index < 32; index++ {
		r := New(workspacemutation.NewCoordinator())
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		var release func()
		var captureErr, opaqueErr error
		go func() { defer wg.Done(); <-start; release, captureErr = r.Capture(context.Background(), "race", path) }()
		go func() { defer wg.Done(); <-start; opaqueErr = r.BeginOpaque(context.Background()) }()
		close(start)
		wg.Wait()
		if captureErr == nil {
			if !errors.Is(opaqueErr, ErrCaptureActive) {
				t.Fatal("opaque execution raced through live capture")
			}
			release()
		} else if !errors.Is(captureErr, ErrUncontainedExecution) || opaqueErr != nil {
			t.Fatal("capture did not observe earlier opaque execution")
		}
	}
}

func TestRegistryCancelledWaitAndUnavailableCoordinator(t *testing.T) {
	r, c, _, path := fixture(t)
	unlock, err := c.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if release, err := r.Capture(ctx, "session", path); release != nil || !errors.Is(err, ErrUnavailable) {
		t.Fatal("cancelled capture allowed")
	}
	if !errors.Is(r.BeginOpaque(ctx), ErrUnavailable) {
		t.Fatal("cancelled opaque allowed")
	}
	unlock()
	if r.HasCaptures() {
		t.Fatal("cancelled request retained capture")
	}
	unavailable := New(nil)
	if _, err := unavailable.Capture(context.Background(), "session", path); !errors.Is(err, ErrUnavailable) {
		t.Fatal("replacement coordinator created")
	}
	if !errors.Is(unavailable.CheckMutation(path), ErrUnavailable) || !errors.Is(unavailable.BeginOpaque(context.Background()), ErrUnavailable) {
		t.Fatal("unavailable registry failed open")
	}
}
