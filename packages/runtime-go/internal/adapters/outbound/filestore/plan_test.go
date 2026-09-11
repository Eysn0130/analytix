package filestore

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestExistingMarkdownRelativePathsAndResolveWorkspaceRelativePath(t *testing.T) {
	root := t.TempDir()
	planDir := filepath.Join(root, ".analytixsdd", "plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "one.md"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "ignore.txt"), []byte("ignore"), 0o644); err != nil {
		t.Fatal(err)
	}
	existing := ExistingMarkdownRelativePaths(root, ".analytixsdd/plan")
	if !existing[".analytixsdd/plan/one.md"] || existing[".analytixsdd/plan/ignore.txt"] {
		t.Fatalf("existing plan paths mismatch: %#v", existing)
	}
	resolved, ok := ResolveWorkspaceRelativePath(root, ".analytixsdd/plan/two.md")
	if !ok || resolved != filepath.Join(root, ".analytixsdd", "plan", "two.md") {
		t.Fatalf("resolved path mismatch: %q %t", resolved, ok)
	}
	if escaped, ok := ResolveWorkspaceRelativePath(root, "../escape.md"); ok {
		t.Fatalf("escaped path resolved: %q", escaped)
	}
}

func TestWriteFileAtomicRejectsSymlinkAncestorWithoutWritingTarget(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "linked")
	if err := os.Symlink(outside, link); err != nil {
		if runtime.GOOS == "windows" {
			return
		}
		t.Fatal(err)
	}
	target := filepath.Join(outside, "plan.md")
	if _, err := WriteFileAtomic(filepath.Join(link, "plan.md"), []byte("must-not-write"), time.Now); err == nil {
		t.Fatal("plan write followed a replaceable symlink ancestor")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("rejected plan write changed the symlink target: %v", err)
	}
}

func TestWriteFileAtomicWritesAndReturnsTimestamp(t *testing.T) {
	root := t.TempDir()
	stamp := time.Date(2026, 7, 3, 1, 2, 3, 4, time.UTC)
	savedAt, err := WriteFileAtomic(filepath.Join(root, "nested", "plan.md"), []byte("body"), func() time.Time { return stamp })
	if err != nil {
		t.Fatalf("write atomic: %v", err)
	}
	if savedAt != "2026-07-03T01:02:03.000000004Z" {
		t.Fatalf("timestamp mismatch: %s", savedAt)
	}
	data, err := os.ReadFile(filepath.Join(root, "nested", "plan.md"))
	if err != nil || string(data) != "body" {
		t.Fatalf("read written file: %q %v", data, err)
	}
}
