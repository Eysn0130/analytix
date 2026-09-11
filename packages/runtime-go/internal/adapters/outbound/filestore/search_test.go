package filestore

import (
	"context"
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

func TestFindFilesSkipsProtectedDirectoriesAndStopsAtLimit(t *testing.T) {
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
	result, err := FindFiles(FindFilesRequest{
		Root:  root,
		Limit: 1,
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
