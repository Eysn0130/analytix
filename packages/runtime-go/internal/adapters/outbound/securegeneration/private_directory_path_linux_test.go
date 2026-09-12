//go:build linux

package securegeneration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateDirectoryDetachedRequiresUnlinkedInodeNotNameSuffix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "still-linked (deleted)")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	directory, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	if detached, err := privateDirectoryDetached(int(directory.Fd())); err != nil || detached {
		t.Fatalf("live directory classified as detached: detached=%v err=%v", detached, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if detached, err := privateDirectoryDetached(int(directory.Fd())); err != nil || !detached {
		t.Fatalf("unlinked directory not recognized: detached=%v err=%v", detached, err)
	}
}
