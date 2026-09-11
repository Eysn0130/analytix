package filestore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGitIgnoreMatcherAppliesNestedRulesAndNegation(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("build/\n*.log\n!important.log\nsrc/**/*.tmp\n"), 0o600); err != nil {
		t.Fatalf("write gitignore: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src", "feature"), 0o700); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", ".gitignore"), []byte("local.txt\n"), 0o600); err != nil {
		t.Fatalf("write nested gitignore: %v", err)
	}
	matcher := NewGitIgnoreMatcher(root, filepath.Join(root, "src"))
	cases := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{filepath.Join(root, "build"), true, true},
		{filepath.Join(root, "run.log"), false, true},
		{filepath.Join(root, "important.log"), false, false},
		{filepath.Join(root, "src", "feature", "scratch.tmp"), false, true},
		{filepath.Join(root, "src", "local.txt"), false, true},
		{filepath.Join(root, "src", "keep.txt"), false, false},
	}
	for _, tc := range cases {
		if got := matcher.Ignored(tc.path, tc.isDir); got != tc.want {
			t.Fatalf("Ignored(%q, %v) = %v, want %v", tc.path, tc.isDir, got, tc.want)
		}
	}
}

func TestDirsBetweenAndPathUnderOrEqual(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dirs := DirsBetween(root, dir)
	if len(dirs) != 3 || dirs[0] != root || dirs[2] != dir {
		t.Fatalf("unexpected dirs: %#v", dirs)
	}
	if !PathUnderOrEqual(root, dir) {
		t.Fatalf("expected %q under %q", dir, root)
	}
	if PathUnderOrEqual(dir, root) {
		t.Fatalf("did not expect %q under %q", root, dir)
	}
}
