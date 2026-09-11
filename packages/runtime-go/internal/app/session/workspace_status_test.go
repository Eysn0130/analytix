package session

import (
	"context"
	"testing"

	"analytix.local/runtime-go/internal/ports"
)

type workspaceStatusProbeStub struct {
	result ports.WorkspaceStatusResult
}

func (s workspaceStatusProbeStub) WorkspaceStatus(context.Context, string) ports.WorkspaceStatusResult {
	return s.result
}

func TestWorkspaceStatusServiceMapsNullableFields(t *testing.T) {
	branch := "main"
	head := "abc123"
	dirty := true
	changes := 2
	status := WorkspaceStatusService{
		Probe: workspaceStatusProbeStub{result: ports.WorkspaceStatusResult{
			Path:            "/tmp/repo",
			Exists:          true,
			IsGitRepository: true,
			Branch:          &branch,
			HeadSHA:         &head,
			IsDirty:         &dirty,
			FileChangeCount: &changes,
			CheckedAt:       "2026-07-02T00:00:00Z",
		}},
	}.Status(context.Background(), "/tmp/repo")
	if status["branch"] != "main" || status["headSha"] != "abc123" || status["isDirty"] != true || status["fileChangeCount"] != float64(2) {
		t.Fatalf("workspace status mapping mismatch: %#v", status)
	}
}

func TestWorkspaceStatusServiceHandlesMissingProbe(t *testing.T) {
	status := WorkspaceStatusService{}.Status(context.Background(), "/tmp/missing")
	if status["path"] != "/tmp/missing" || status["exists"] != false || status["branch"] != nil || status["isDirty"] != nil {
		t.Fatalf("missing probe status mismatch: %#v", status)
	}
}
