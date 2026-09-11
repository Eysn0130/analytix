package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sessionapp "analytix.local/runtime-go/internal/app/session"
	"analytix.local/runtime-go/internal/ports"
)

type workspaceStatusProbeStub struct {
	workspace string
}

func (s *workspaceStatusProbeStub) WorkspaceStatus(ctx context.Context, workspace string) ports.WorkspaceStatusResult {
	s.workspace = workspace
	branch := "main"
	return ports.WorkspaceStatusResult{
		Path:            workspace,
		Exists:          true,
		IsGitRepository: true,
		Branch:          &branch,
		CheckedAt:       "2026-07-02T00:00:00Z",
	}
}

func TestWorkspaceStatusHandlersContract(t *testing.T) {
	probe := &workspaceStatusProbeStub{}
	handler := WorkspaceStatusHandlers{Service: sessionapp.WorkspaceStatusService{Probe: probe}}
	recorder := httptest.NewRecorder()
	handler.HandleStatus(recorder, httptest.NewRequest(http.MethodGet, "/v1/workspace/status?path=/tmp/repo", nil))
	if recorder.Code != http.StatusOK || probe.workspace != "/tmp/repo" || !strings.Contains(recorder.Body.String(), `"isGitRepository":true`) {
		t.Fatalf("workspace status contract mismatch: status=%d workspace=%q body=%s", recorder.Code, probe.workspace, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.HandleStatus(recorder, httptest.NewRequest(http.MethodPost, "/v1/workspace/status", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method contract mismatch: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
