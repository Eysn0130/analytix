package server

import (
	"strings"
	"testing"

	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	"analytix.local/runtime-go/internal/jobs"
)

func TestRuntimeServerRejectsNonSubagentContinueSource(t *testing.T) {
	err := subagentapp.ValidateSource(jobs.Record{
		ID:             "job-shell",
		Kind:           "background-shell",
		Status:         "completed",
		ParentThreadID: "parent",
		ChildThreadID:  "child",
		Workspace:      "/workspace",
		Model:          "deepseek-chat",
		Effort:         "medium",
		ToolScope:      []string{"read"},
	}, runtimeSubagentTaskRequest{
		ContinueFrom: "job-shell",
	}, subagentapp.SourceValidation{
		ParentThreadID: "parent",
		Workspace:      "/workspace",
		ProviderID:     "deepseek",
		Model:          "deepseek-chat",
		EndpointFormat: "chat_completions",
		Effort:         "medium",
		ToolScope:      []string{"read"},
	})
	if err == nil || !strings.Contains(err.Error(), "not a subagent transcript") {
		t.Fatalf("non-subagent child-run source should be rejected, got %v", err)
	}
}

func TestRuntimeServerRejectsSubagentContinueSourceExecutionDrift(t *testing.T) {
	err := subagentapp.ValidateSource(jobs.Record{
		ID:             "job-child",
		Kind:           "subagent",
		Status:         "completed",
		ParentThreadID: "parent",
		ChildThreadID:  "child",
		Workspace:      "/workspace",
		ProviderID:     "source-provider",
		Model:          "source-model",
		EndpointFormat: "chat_completions",
		Variant:        "stable",
		Effort:         "medium",
		ToolScope:      []string{"read"},
	}, runtimeSubagentTaskRequest{
		ContinueFrom: "job-child",
	}, subagentapp.SourceValidation{
		ParentThreadID: "parent",
		Workspace:      "/workspace",
		ProviderID:     "other-provider",
		Model:          "source-model",
		EndpointFormat: "chat_completions",
		Variant:        "stable",
		Effort:         "medium",
		ToolScope:      []string{"read"},
	})
	if err == nil || !strings.Contains(err.Error(), "uses provider") {
		t.Fatalf("subagent source execution drift should be rejected, got %v", err)
	}
}
