package toolcatalog

import (
	"testing"

	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func TestToolFailureProgressMessageUsesClosedProjectionCode(t *testing.T) {
	projection := publicHostStatusProjection("failed", "tool_failed", "workspace_escape")
	message := ToolFailureProgressMessage(domaintoolresult.PublicToolResultProjectionRecordV1(projection))
	if message != "tool failed: workspace_escape" {
		t.Fatalf("unexpected progress message: %q", message)
	}
}

func TestToolFailureProgressMessageRejectsRawError(t *testing.T) {
	message := ToolFailureProgressMessage(map[string]any{
		"code": "workspace_escape", "error": "ACCOUNT_SENTINEL_6222020202020202020",
	})
	if message != "tool failed" {
		t.Fatalf("raw error reached progress projection: %q", message)
	}
}
