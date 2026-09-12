//go:build !darwin

package process

import (
	"os/exec"
	"testing"
)

func TestGitStatusProbeUnsupportedContainmentIsUnavailable(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatal("git is required for the containment contract")
	}
	repository := initGitRepo(t)
	if staged, err := NewGitStatusProbe(t.TempDir()).PathHasStagedChanges(repository, "notes.txt"); staged || err != ErrGitStatusUnavailable {
		t.Fatal("unavailable containment was treated as a clean index")
	}
}
