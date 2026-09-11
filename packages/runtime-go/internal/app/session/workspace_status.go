package session

import (
	"context"
	"strings"

	"analytix.local/runtime-go/internal/ports"
)

type WorkspaceStatusService struct {
	Probe ports.WorkspaceStatusProbe
}

func (s WorkspaceStatusService) Status(ctx context.Context, workspace string) map[string]any {
	if s.Probe == nil {
		return WorkspaceStatusMap(ports.WorkspaceStatusResult{
			Path:            strings.TrimSpace(workspace),
			Exists:          false,
			IsGitRepository: false,
		})
	}
	return WorkspaceStatusMap(s.Probe.WorkspaceStatus(ctx, workspace))
}

func WorkspaceStatusMap(status ports.WorkspaceStatusResult) map[string]any {
	return map[string]any{
		"path":            status.Path,
		"exists":          status.Exists,
		"isGitRepository": status.IsGitRepository,
		"branch":          nullableStringPointer(status.Branch),
		"headSha":         nullableStringPointer(status.HeadSHA),
		"isDirty":         nullableBoolPointer(status.IsDirty),
		"fileChangeCount": nullableIntPointer(status.FileChangeCount),
		"checkedAt":       status.CheckedAt,
	}
}

func nullableStringPointer(value *string) any {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	return strings.TrimSpace(*value)
}

func nullableBoolPointer(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableIntPointer(value *int) any {
	if value == nil {
		return nil
	}
	return float64(*value)
}
