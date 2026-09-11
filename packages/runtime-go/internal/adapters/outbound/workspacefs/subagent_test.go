package workspacefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateSubagentWorkspaceRequiresExistingDirectoryAndReturnsRealPath(t *testing.T) {
	root := t.TempDir()
	resolved, err := ValidateSubagentWorkspace(root)
	if err != nil {
		t.Fatalf("existing directory should validate: %v", err)
	}
	expectedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != filepath.Clean(expectedRoot) {
		t.Fatalf("workspace should be cleaned: %q", resolved)
	}

	file := filepath.Join(root, "not-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := ValidateSubagentWorkspace(file); err == nil {
		t.Fatal("file workspace should be rejected")
	}
	if _, err := ValidateSubagentWorkspace(filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing workspace should be rejected")
	}
}

func TestValidateSubagentWorkspaceCanonicalizesSymlink(t *testing.T) {
	root := t.TempDir()
	realWorkspace := filepath.Join(root, "real")
	if err := os.Mkdir(realWorkspace, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(realWorkspace, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	resolved, err := ValidateSubagentWorkspace(alias)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := filepath.EvalSymlinks(realWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != expected {
		t.Fatalf("workspace alias did not resolve to frozen real path: got=%q want=%q", resolved, expected)
	}
}
