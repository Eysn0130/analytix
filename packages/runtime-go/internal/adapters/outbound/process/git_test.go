package process

import (
	"errors"
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
	if staged, err := probe.PathHasStagedChanges(repository, "README.md"); err != nil || !staged {
		t.Fatal("ordinary contained staged-change check failed")
	}

	protectedConfig := filepath.Join(protectedRoot, "protected-gitconfig")
	if err := os.WriteFile(protectedConfig, []byte("[user]\n\tname = must-not-escape\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", protectedConfig)
	if staged, err := probe.PathHasStagedChanges(repository, "README.md"); staged || !errors.Is(err, ErrGitStatusUnavailable) {
		t.Fatal("git status probe did not fail closed when git attempted to read protected configuration")
	}
}

func TestGitStatusProbeDistinguishesCleanStagedAndUnavailable(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatal("git is required for the staged-change contract")
	}
	probe := NewGitStatusProbe()
	if staged, err := probe.PathHasStagedChanges(t.TempDir(), "notes.txt"); err != nil || staged {
		t.Fatal("confirmed non-repository workspace was not clean")
	}
	if staged, err := NewGitStatusProbe(t.TempDir()).PathHasStagedChanges(t.TempDir(), "notes.txt"); err != nil || staged {
		t.Fatal("non-repository workspace unnecessarily required a child process")
	}
	repository := initGitRepo(t)
	if staged, err := probe.PathHasStagedChanges(repository, "README.md"); err != nil || staged {
		t.Fatal("clean tracked file was not clean")
	}
	nested := filepath.Join(repository, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "notes.txt"), []byte("staged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", "nested/notes.txt")
	if staged, err := probe.PathHasStagedChanges(nested, "notes.txt"); err != nil || !staged {
		t.Fatal("parent repository staged change was not detected")
	}
	if err := os.WriteFile(filepath.Join(repository, ".git", "index"), []byte("invalid-index-private-marker"), 0o600); err != nil {
		t.Fatal(err)
	}
	if staged, err := probe.PathHasStagedChanges(nested, "notes.txt"); staged || err != ErrGitStatusUnavailable {
		t.Fatal("unreadable index was not reported as unavailable")
	}
}

func TestGitStatusProbeUnknownRepositoryInventoryIsUnavailable(t *testing.T) {
	workspace := t.TempDir()
	if staged, err := NewGitStatusProbe().PathHasStagedChanges(filepath.Join(workspace, "absent"), "notes.txt"); staged || err != ErrGitStatusUnavailable {
		t.Fatal("missing workspace was treated as a clean index")
	}
	if err := os.WriteFile(filepath.Join(workspace, ".git"), []byte("unreadable-gitdir-private-marker"), 0o600); err != nil {
		t.Fatal(err)
	}
	if staged, err := NewGitStatusProbe().PathHasStagedChanges(workspace, "notes.txt"); staged || err != ErrGitStatusUnavailable {
		t.Fatal("invalid linked-repository marker was treated as a clean index")
	}
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "private-repository"))
	if staged, err := NewGitStatusProbe().PathHasStagedChanges(t.TempDir(), "notes.txt"); staged || err != ErrGitStatusUnavailable {
		t.Fatal("repository environment override was treated as a clean index")
	}
}

func TestGitStatusProbeMissingExecutableIsUnavailable(t *testing.T) {
	repository := initGitRepo(t)
	t.Setenv("PATH", t.TempDir())
	if staged, err := NewGitStatusProbe().PathHasStagedChanges(repository, "notes.txt"); staged || err != ErrGitStatusUnavailable {
		t.Fatal("missing git executable was treated as a clean index")
	}
}
