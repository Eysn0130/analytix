//go:build darwin

package electronlegacytask

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDarwinDenyDeleteACLBlocksBeforeRetirementJournal(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, BackgroundTaskFileV1)
	writeTestFile(t, target, []byte("opaque"))
	command := exec.Command("/bin/chmod", "+a", "everyone deny delete", target)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("add Darwin deny-delete ACL: %v: %s", err, output)
	}
	t.Cleanup(func() {
		if output, err := exec.Command("/bin/chmod", "-N", target).CombinedOutput(); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Errorf("clear Darwin target ACL: %v: %s", err, output)
		}
	})
	store := newTestStore(t, root)
	if _, err := store.Prepare(context.Background()); err == nil {
		t.Fatal("deny-delete ACL reached retirement planning")
	}
	assertFileBody(t, target, []byte("opaque"))
	if _, err := os.Lstat(filepath.Join(root, journalDirectoryV1)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deny-delete ACL created retirement journal: %v", err)
	}
}
