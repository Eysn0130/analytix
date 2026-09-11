package subagent

import (
	"errors"
	"strings"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

// ChildWorkspaceAdmissionInput contains only host-resolved workspace
// authority. Provider arguments are untrusted and must be resolved before
// they reach this check.
type ChildWorkspaceAdmissionInput struct {
	Request                 TaskRequest
	ParentWorkspaceRealPath string
	ChildWorkspaceRealPath  string
}

// ValidateChildWorkspaceAdmission rejects a foreground child before any
// durable job or child-thread record can be created when its workspace differs
// from the frozen parent authority. A cross-workspace child must use the
// runtime-owned detached/background path; worktree isolation already requires
// that path through ValidateWorktreeIsolationRequest.
func ValidateChildWorkspaceAdmission(input ChildWorkspaceAdmissionInput) error {
	parent := strings.TrimSpace(input.ParentWorkspaceRealPath)
	child := strings.TrimSpace(input.ChildWorkspaceRealPath)
	if parent == "" || child == "" {
		return errors.New("subagent workspace authority is unavailable")
	}
	if parent == child {
		return nil
	}
	if input.Request.RunInBackground {
		return nil
	}
	if strings.TrimSpace(input.Request.IsolationMode) == string(domainjob.IsolationWorktree) {
		return errors.New("cross-workspace worktree isolation requires the host background path")
	}
	return errors.New("foreground subagent workspace must match the frozen parent workspace; use run_in_background=true or host worktree isolation for cross-workspace execution")
}
