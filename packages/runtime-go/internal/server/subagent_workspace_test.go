package server

import (
	"testing"

	workspacefs "analytix.local/runtime-go/internal/adapters/outbound/workspacefs"
)

func TestRuntimeSubagentWorkspaceUsesAdapterAuthority(t *testing.T) {
	if resolved, err := workspacefs.ValidateSubagentWorkspace(t.TempDir()); err != nil || resolved == "" {
		t.Fatalf("workspace authority adapter unavailable: resolved=%q err=%v", resolved, err)
	}
}
