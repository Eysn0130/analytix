package process

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGitStatusProbeContainsProtectedGitConfigurationAndKeepsOrdinaryStagedChecks(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("packaged macOS process containment seam")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	repository := initGitRepo(t)
	if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("staged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", "README.md")

	protectedRoot := t.TempDir()
	probe := NewGitStatusProbe(protectedRoot)
	if !probe.PathHasStagedChanges(repository, "README.md") {
		t.Fatal("ordinary contained staged-change check failed")
	}

	protectedConfig := filepath.Join(protectedRoot, "protected-gitconfig")
	if err := os.WriteFile(protectedConfig, []byte("[user]\n\tname = must-not-escape\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", protectedConfig)
	if probe.PathHasStagedChanges(repository, "README.md") {
		t.Fatal("git status probe did not fail closed when git attempted to read protected configuration")
	}
}
