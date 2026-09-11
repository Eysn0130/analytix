package filestore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWritePrivateFileAtomicReplacesWithPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "thread.json")
	if err := WritePrivateFileAtomic(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := WritePrivateFileAtomic(path, []byte("second")); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "second" {
		t.Fatalf("private atomic replacement mismatch: body=%q err=%v", body, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("private file permissions mismatch: info=%v err=%v", info, err)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".thread.json-*.tmp"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("private atomic writer left temporary files: %#v err=%v", matches, err)
	}
}
