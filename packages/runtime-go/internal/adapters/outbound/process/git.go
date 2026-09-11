package process

import (
	"context"
	"os"
	"os/exec"

	childenv "analytix.local/runtime-go/internal/adapters/outbound/childenv"
	processsandbox "analytix.local/runtime-go/internal/adapters/outbound/processsandbox"
)

type GitStatusProbe struct {
	protectedReadDirs []string
}

func NewGitStatusProbe(protectedReadDirs ...string) GitStatusProbe {
	return GitStatusProbe{
		protectedReadDirs: append([]string(nil), protectedReadDirs...),
	}
}

func (p GitStatusProbe) PathHasStagedChanges(workspace string, relativePath string) bool {
	exe, err := exec.LookPath("git")
	if err != nil {
		return false
	}
	cmd, err := processsandbox.CommandContext(
		context.Background(),
		exe,
		[]string{"-C", workspace, "diff", "--cached", "--quiet", "--", relativePath},
		processsandbox.FilesystemPolicy{
			DenyRoots: append([]string(nil), p.protectedReadDirs...),
		},
	)
	if err != nil {
		return false
	}
	cmd.Env = childenv.Sanitized(os.Environ())
	if err = cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
			return true
		}
	}
	return false
}
