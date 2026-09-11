//go:build darwin || linux

package electronlegacytask

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAuthenticatedPlanPrecedesTargetProtection(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, BackgroundTaskFileV1)
	writeTestFile(t, target, []byte("opaque"))
	if err := os.Chmod(target, 0o644); err != nil {
		t.Fatal(err)
	}
	store := newTestStore(t, root)
	plan, err := store.Prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cut := errors.New("cut after target protection")
	store.hooks.afterTargetProtected = func() error { return cut }
	if err := plan.Apply(context.Background()); !errors.Is(err, cut) {
		t.Fatalf("apply cut = %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("protected target mode = %#o", info.Mode().Perm())
	}
	if _, err := os.Lstat(filepath.Join(root, journalDirectoryV1, journalPlanFileV1)); err != nil {
		t.Fatalf("authenticated plan was not durable before protection cut: %v", err)
	}
}
