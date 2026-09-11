//go:build darwin || linux

package finalauthority

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestAcceptedFinalCASReaderRejectsSymlinkRootsAndThreadAncestors(t *testing.T) {
	base := t.TempDir()
	realRoot := filepath.Join(base, "real")
	writeAcceptedFinalCASTestThread(t, realRoot, "thr_cas_symlink", "turn_cas_symlink", "running")
	rootLink := filepath.Join(base, "durable-link")
	if err := os.Symlink(realRoot, rootLink); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAcceptedFinalCASReader(rootLink); err == nil {
		t.Fatal("CAS reader accepted a symlink durable root")
	}

	reader, err := NewAcceptedFinalCASReader(realRoot)
	if err != nil {
		t.Fatal(err)
	}
	threadDirectory := filepath.Join(realRoot, "threads", "thr_cas_symlink")
	if err := os.Rename(threadDirectory, threadDirectory+"-original"); err != nil {
		t.Fatal(err)
	}
	escape := filepath.Join(base, "escape-thread")
	writeAcceptedFinalCASTestThread(t, filepath.Join(base, "escape-root"), "thr_cas_symlink", "turn_cas_symlink", "completed")
	if err := os.Rename(filepath.Join(base, "escape-root", "threads", "thr_cas_symlink"), escape); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(escape, threadDirectory); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadAcceptedFinalCASObservation(context.Background(), "thr_cas_symlink", "turn_cas_symlink"); err == nil {
		t.Fatal("CAS reader followed a symlink thread ancestor")
	}
}
