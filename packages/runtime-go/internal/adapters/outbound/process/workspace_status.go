package process

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"analytix.local/runtime-go/internal/ports"
)

type WorkspaceStatusProbe struct {
	CommandProbe      ports.CommandProbe
	protectedReadDirs []string
	Timeout           time.Duration
}

func NewWorkspaceStatusProbe(protectedReadDirs ...string) WorkspaceStatusProbe {
	return WorkspaceStatusProbe{
		CommandProbe:      NewCommandProbe(protectedReadDirs...),
		protectedReadDirs: append([]string(nil), protectedReadDirs...),
		Timeout:           2 * time.Second,
	}
}

func (p WorkspaceStatusProbe) WorkspaceStatus(ctx context.Context, workspace string) ports.WorkspaceStatusResult {
	checkedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if strings.TrimSpace(workspace) == "" {
		return ports.WorkspaceStatusResult{
			Path:            "",
			Exists:          false,
			IsGitRepository: false,
			CheckedAt:       checkedAt,
		}
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		abs = workspace
	}
	if _, err := os.Stat(abs); err != nil {
		return ports.WorkspaceStatusResult{
			Path:            abs,
			Exists:          false,
			IsGitRepository: false,
			CheckedAt:       checkedAt,
		}
	}
	inside, ok := p.gitOutput(ctx, abs, "rev-parse", "--is-inside-work-tree")
	if !ok || strings.TrimSpace(inside) != "true" {
		return ports.WorkspaceStatusResult{
			Path:            abs,
			Exists:          true,
			IsGitRepository: false,
			CheckedAt:       checkedAt,
		}
	}
	status := ports.WorkspaceStatusResult{
		Path:            abs,
		Exists:          true,
		IsGitRepository: true,
		CheckedAt:       checkedAt,
	}
	if branch, ok := p.gitOutput(ctx, abs, "rev-parse", "--abbrev-ref", "HEAD"); ok {
		status.Branch = stringPtr(branch)
	}
	if head, ok := p.gitOutput(ctx, abs, "rev-parse", "HEAD"); ok {
		status.HeadSHA = stringPtr(head)
	}
	if porcelain, ok := p.gitOutput(ctx, abs, "status", "--porcelain"); ok {
		changeCount := countStatusLines(porcelain)
		status.IsDirty = boolPtr(changeCount > 0)
		status.FileChangeCount = intPtr(changeCount)
	}
	return status
}

func (p WorkspaceStatusProbe) gitOutput(ctx context.Context, workspace string, args ...string) (string, bool) {
	probe := p.CommandProbe
	if probe == nil {
		probe = NewCommandProbe(p.protectedReadDirs...)
	}
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result := probe.ProbeCommand(probeCtx, ports.CommandProbeRequest{
		Binary: "git",
		Args:   append([]string{"-C", workspace}, args...),
	})
	if !result.Found || result.Error != "" || result.TimedOut {
		return "", false
	}
	return strings.TrimSpace(result.Stdout), true
}

func countStatusLines(status string) int {
	count := 0
	for _, line := range strings.Split(status, "\n") {
		if strings.TrimSpace(line) != "" {
			count += 1
		}
	}
	return count
}

func stringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

func intPtr(value int) *int {
	return &value
}
