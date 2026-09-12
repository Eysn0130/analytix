package process

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	childenv "analytix.local/runtime-go/internal/adapters/outbound/childenv"
	processsandbox "analytix.local/runtime-go/internal/adapters/outbound/processsandbox"
)

// ErrGitStatusUnavailable never exposes subprocess output or private paths.
// Callers must not interpret an unavailable probe as a clean index.
var ErrGitStatusUnavailable = errors.New("git status check is unavailable")

type GitStatusProbe struct {
	protectedReadDirs []string
}

func NewGitStatusProbe(protectedReadDirs ...string) GitStatusProbe {
	return GitStatusProbe{
		protectedReadDirs: append([]string(nil), protectedReadDirs...),
	}
}

func (p GitStatusProbe) PathHasStagedChanges(workspace string, relativePath string) (bool, error) {
	// Repository-location overrides could redirect a check away from the file
	// being restored. Never use them as evidence that this workspace is clean.
	for _, key := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_CEILING_DIRECTORIES", "GIT_DISCOVERY_ACROSS_FILESYSTEM"} {
		if os.Getenv(key) != "" {
			return false, ErrGitStatusUnavailable
		}
	}
	located, err := checkpointGitRepositoryPresent(workspace)
	if err != nil || !located {
		return false, err
	}
	exe, err := exec.LookPath("git")
	if err != nil {
		return false, ErrGitStatusUnavailable
	}
	cmd, err := processsandbox.CommandContext(context.Background(), exe,
		[]string{"-C", workspace, "diff", "--cached", "--quiet", "--", relativePath},
		processsandbox.FilesystemPolicy{DenyRoots: append([]string(nil), p.protectedReadDirs...)})
	if err != nil {
		return false, ErrGitStatusUnavailable
	}
	cmd.Env = childenv.Sanitized(os.Environ())
	if err = cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
			return true, nil
		}
		return false, ErrGitStatusUnavailable
	}
	return false, nil
}

// Only a complete, successful ancestor inventory proves an ordinary directory
// is outside Git. A .git file also counts: linked worktrees resolve through it.
// Bare-repository markers remain conservative candidates rather than clean.
func checkpointGitRepositoryPresent(workspace string) (bool, error) {
	if workspace == "" {
		return false, ErrGitStatusUnavailable
	}
	root, err := filepath.Abs(workspace)
	if err != nil {
		return false, ErrGitStatusUnavailable
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return false, ErrGitStatusUnavailable
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return false, ErrGitStatusUnavailable
	}
	for {
		if _, err := os.Lstat(filepath.Join(root, ".git")); err == nil {
			return true, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, ErrGitStatusUnavailable
		}
		bare := true
		for _, marker := range []string{"HEAD", "objects"} {
			if _, err := os.Lstat(filepath.Join(root, marker)); errors.Is(err, os.ErrNotExist) {
				bare = false
			} else if err != nil {
				return false, ErrGitStatusUnavailable
			}
		}
		if bare {
			return true, nil
		}
		parent := filepath.Dir(root)
		if parent == root {
			return false, nil
		}
		root = parent
	}
}
