package filestore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListDirReturnsStableEntryShapeAndLimit(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o700); err != nil {
		t.Fatalf("mkdir dir: %v", err)
	}
	result, err := ListDir(root, 1)
	if err != nil {
		t.Fatalf("list dir: %v", err)
	}
	if len(result.Entries) != 1 || len(result.Names) != 1 {
		t.Fatalf("limited result = %#v", result)
	}
	if !result.Truncated {
		t.Fatalf("expected truncated result")
	}
	if result.Entries[0].Name == "" || result.Entries[0].DisplayName == "" || result.Entries[0].Kind == "" {
		t.Fatalf("entry shape = %#v", result.Entries[0])
	}
}

func TestListDirFailureSuggestsCurrentDesktopForOtherMacUser(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("home directory unavailable")
	}
	currentUser := filepath.Base(home)
	otherUser := currentUser + "_not_current"
	output, isError := ExecuteListTool(WorkspaceSearchToolInput{
		Workspace:   t.TempDir(),
		Args:        map[string]any{"path": filepath.Join("/Users", otherUser, "Desktop")},
		SandboxMode: "danger-full-access",
	})
	if !isError {
		t.Fatalf("expected missing other-user Desktop to fail")
	}
	body, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("expected map output, got %#v", output)
	}
	if body["code"] != "list_failed" || body["suggested_path"] != filepath.Join(home, "Desktop") {
		t.Fatalf("missing current Desktop suggestion: %#v", body)
	}
	hint, _ := body["hint"].(string)
	if !strings.Contains(hint, "current user's Desktop") {
		t.Fatalf("missing desktop hint: %#v", body)
	}
}

func TestFindFilesStopsAtLimitBeforeProtectedDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o600); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("b"), 0o600); err != nil {
		t.Fatalf("write b: %v", err)
	}
	protectedDir := filepath.Join(root, "protected")
	if err := os.Mkdir(protectedDir, 0o700); err != nil {
		t.Fatalf("mkdir protected: %v", err)
	}
	if err := os.WriteFile(filepath.Join(protectedDir, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	protectedVisited := false
	result, err := FindFiles(FindFilesRequest{
		Root:  root,
		Limit: 1,
		RelativePath: func(path string) string {
			rel, _ := filepath.Rel(root, path)
			return filepath.ToSlash(rel)
		},
		ProtectedDir: func(path string) (string, bool) {
			if path == protectedDir {
				protectedVisited = true
				return "protected", true
			}
			return "", false
		},
		Match: func(relativePath string, name string) bool {
			return strings.HasSuffix(name, ".txt") && !strings.Contains(relativePath, "secret")
		},
	})
	if err != nil {
		t.Fatalf("find files: %v", err)
	}
	if len(result.Matches) != 1 {
		t.Fatalf("matches = %#v", result.Matches)
	}
	if !result.LimitReached {
		t.Fatalf("expected limit reached")
	}
	if result.Matches[0].RelativePath != "a.txt" || protectedVisited || len(result.SkippedProtected) != 0 {
		t.Fatalf("limit did not stop before later traversal: result=%#v protectedVisited=%v", result, protectedVisited)
	}
}

func TestFindFilesMissingRootReturnsTraversalError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-root")
	callbacks := 0
	result, err := FindFiles(FindFilesRequest{
		Root:         missing,
		Limit:        10,
		ProtectedDir: func(string) (string, bool) { callbacks++; return "", false },
		Match:        func(string, string) bool { callbacks++; return true },
	})
	var pathError *os.PathError
	if !errors.Is(err, os.ErrNotExist) || !errors.As(err, &pathError) || pathError.Path != missing {
		t.Fatalf("missing root error was swallowed or replaced: %v", err)
	}
	if callbacks != 0 || len(result.Matches) != 0 || len(result.SkippedProtected) != 0 || result.LimitReached {
		t.Fatalf("missing root produced successful traversal facts: result=%#v callbacks=%d", result, callbacks)
	}
}

func TestFindFilesChildReadDirFailureReturnsErrorWithPartialMatches(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"00-before.txt", "02-after.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("synthetic"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	child := filepath.Join(root, "01-disappears")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	removals := 0
	result, err := FindFiles(FindFilesRequest{
		Root:  root,
		Limit: 10,
		RelativePath: func(path string) string {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				t.Fatal(err)
			}
			return filepath.ToSlash(rel)
		},
		ProtectedDir: func(path string) (string, bool) {
			if path == child {
				removals++
				if removals != 1 {
					t.Fatal("directory admission callback repeated after ReadDir failure")
				}
				// WalkDir has already observed the directory entry but has not
				// opened it yet. Removing this empty, test-owned directory makes
				// the subsequent real ReadDir fail without permission assumptions.
				if err := os.Remove(child); err != nil {
					t.Fatal(err)
				}
			}
			return "", false
		},
		Match: func(_ string, name string) bool { return strings.HasSuffix(name, ".txt") },
	})
	var pathError *os.PathError
	if !errors.Is(err, os.ErrNotExist) || !errors.As(err, &pathError) || pathError.Path != child {
		t.Fatalf("child ReadDir error was swallowed or replaced: %v", err)
	}
	if removals != 1 || len(result.Matches) != 1 || result.Matches[0].RelativePath != "00-before.txt" || result.LimitReached || len(result.SkippedProtected) != 0 {
		t.Fatalf("partial traversal was treated as a complete success: result=%#v removals=%d", result, removals)
	}
}

func TestFindFilesProtectedSkipDirReturnsNoErrorAndContinues(t *testing.T) {
	root := t.TempDir()
	protected := filepath.Join(root, "01-protected")
	if err := os.Mkdir(protected, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(root, "00-before.txt"), filepath.Join(protected, "secret.txt"), filepath.Join(root, "02-after.txt")} {
		if err := os.WriteFile(path, []byte("synthetic"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	protectedVisits, descendantVisits := 0, 0
	result, err := FindFiles(FindFilesRequest{
		Root:  root,
		Limit: 10,
		RelativePath: func(path string) string {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				t.Fatal(err)
			}
			return filepath.ToSlash(rel)
		},
		ProtectedDir: func(path string) (string, bool) {
			if path == protected {
				protectedVisits++
				return "01-protected", true
			}
			return "", false
		},
		Match: func(relativePath string, name string) bool {
			if strings.HasPrefix(relativePath, "01-protected/") {
				descendantVisits++
			}
			return strings.HasSuffix(name, ".txt")
		},
	})
	if err != nil {
		t.Fatalf("intentional SkipDir became a traversal error: %v", err)
	}
	if protectedVisits != 1 || descendantVisits != 0 || len(result.SkippedProtected) != 1 || result.SkippedProtected[0] != "01-protected" || result.LimitReached {
		t.Fatalf("protected directory was not actively skipped: result=%#v protectedVisits=%d descendantVisits=%d", result, protectedVisits, descendantVisits)
	}
	if len(result.Matches) != 2 || result.Matches[0].RelativePath != "00-before.txt" || result.Matches[1].RelativePath != "02-after.txt" {
		t.Fatalf("SkipDir did not preserve ordinary traversal on both sides: %#v", result.Matches)
	}
}

func TestNormalizeSearchGlobPatterns(t *testing.T) {
	workspace := t.TempDir()
	if got, ok := NormalizeWorkspaceGlobPattern(workspace, workspace+`/src\**\*.go`); !ok || got != "src/**/*.go" {
		t.Fatalf("NormalizeWorkspaceGlobPattern absolute = %q, %v", got, ok)
	}
	if got, ok := NormalizeWorkspaceGlobPattern(workspace, workspace+"/../*.go"); ok {
		t.Fatalf("NormalizeWorkspaceGlobPattern escape = %q, true", got)
	}
	if got, ok := NormalizeRelativeSearchGlob("src/**/*.go"); !ok || got != "src/**/*.go" {
		t.Fatalf("NormalizeRelativeSearchGlob relative = %q, %v", got, ok)
	}
	if got, ok := NormalizeRelativeSearchGlob("/tmp/*.go"); ok {
		t.Fatalf("NormalizeRelativeSearchGlob absolute = %q, true", got)
	}
}

func TestGlobFilesSkipsProtectedAndNamedDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "node_modules"), 0o700); err != nil {
		t.Fatalf("mkdir node_modules: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "node_modules", "hit.txt"), []byte("hit"), 0o600); err != nil {
		t.Fatalf("write skipped file: %v", err)
	}
	protectedDir := filepath.Join(root, "protected")
	if err := os.Mkdir(protectedDir, 0o700); err != nil {
		t.Fatalf("mkdir protected: %v", err)
	}
	if err := os.WriteFile(filepath.Join(protectedDir, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "keep.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatalf("write keep: %v", err)
	}
	result, err := GlobFiles(GlobFilesRequest{
		Context: context.Background(),
		Root:    root,
		Limit:   10,
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
		SkipDirName: func(name string) bool {
			return name == "node_modules"
		},
		Match: func(relativePath string) bool {
			return strings.HasSuffix(relativePath, ".txt")
		},
	})
	if err != nil {
		t.Fatalf("glob files: %v", err)
	}
	if len(result.Matches) != 1 || result.Matches[0].RelativePath != "keep.txt" {
		t.Fatalf("matches = %#v", result.Matches)
	}
	if len(result.SkippedDirs) != 1 || result.SkippedDirs[0] != "node_modules" {
		t.Fatalf("skipped dirs = %#v", result.SkippedDirs)
	}
	if len(result.SkippedProtected) != 1 || result.SkippedProtected[0] != "protected" {
		t.Fatalf("skipped protected = %#v", result.SkippedProtected)
	}
}
