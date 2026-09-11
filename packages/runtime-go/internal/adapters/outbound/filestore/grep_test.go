package filestore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrepScanCollectsContextAndSkipsGitIgnoredFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.txt\n"), 0o600); err != nil {
		t.Fatalf("write gitignore: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "keep.txt"), []byte("before\nneedle here\nafter\n"), 0o600); err != nil {
		t.Fatalf("write keep: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "ignored.txt"), []byte("needle hidden\n"), 0o600); err != nil {
		t.Fatalf("write ignored: %v", err)
	}
	result, err := GrepScan(context.Background(), GrepScanRequest{
		Workspace:    root,
		Root:         root,
		Limit:        10,
		ContextLines: 1,
		Match: func(line string) (bool, int) {
			index := strings.Index(line, "needle")
			return index >= 0, index + 1
		},
		RelativePath: func(path string) string {
			rel, _ := filepath.Rel(root, path)
			return filepath.ToSlash(rel)
		},
		HiddenEntryName: func(name string) bool {
			return strings.HasPrefix(name, ".")
		},
	})
	if err != nil {
		t.Fatalf("grep scan: %v", err)
	}
	if len(result.Matches) != 1 {
		t.Fatalf("matches = %#v", result.Matches)
	}
	match := result.Matches[0]
	if match.RelativePath != "keep.txt" || match.Line != 2 || match.Column != 1 || match.Text != "needle here" {
		t.Fatalf("unexpected match: %#v", match)
	}
	if len(match.ContextBefore) != 1 || match.ContextBefore[0] != "before" {
		t.Fatalf("context_before = %#v", match.ContextBefore)
	}
	if len(match.ContextAfter) != 1 || match.ContextAfter[0] != "after" {
		t.Fatalf("context_after = %#v", match.ContextAfter)
	}
}

func TestGrepScanSkipsProtectedDirectory(t *testing.T) {
	root := t.TempDir()
	protectedDir := filepath.Join(root, "protected")
	if err := os.Mkdir(protectedDir, 0o700); err != nil {
		t.Fatalf("mkdir protected: %v", err)
	}
	if err := os.WriteFile(filepath.Join(protectedDir, "secret.txt"), []byte("needle\n"), 0o600); err != nil {
		t.Fatalf("write protected file: %v", err)
	}
	result, err := GrepScan(context.Background(), GrepScanRequest{
		Workspace: root,
		Root:      root,
		Limit:     10,
		Match: func(line string) (bool, int) {
			return strings.Contains(line, "needle"), 1
		},
		RelativePath: func(path string) string {
			rel, _ := filepath.Rel(root, path)
			return filepath.ToSlash(rel)
		},
		ProtectedDir: func(path string) (string, bool) {
			if path == protectedDir {
				return "protected", true
			}
			return "", false
		},
	})
	if err != nil {
		t.Fatalf("grep scan: %v", err)
	}
	if len(result.Matches) != 0 {
		t.Fatalf("matches = %#v", result.Matches)
	}
	if len(result.SkippedProtected) != 1 || result.SkippedProtected[0] != "protected" {
		t.Fatalf("skipped protected = %#v", result.SkippedProtected)
	}
}
