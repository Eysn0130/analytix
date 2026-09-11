package subagent

import (
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestBuildChildRunStartRequestMaterializesLineageAndExecution(t *testing.T) {
	parentSteps := 12
	request, err := BuildChildRunStartRequest(ChildRunStartInput{
		ParentGoalID:        "goal-1",
		ParentGoalObjective: "ship runtime",
		ParentThreadID:      "thread-parent",
		ParentTurnID:        "turn-parent",
		ParentToolItemID:    "item-call-1",
		ParentToolCallID:    "call-1",
		ChildThreadID:       "thread-child",
		FallbackName:        "task",
		SourceRef:           "child-source",
		Request: TaskRequest{
			Name:            "reviewer",
			Label:           "Review",
			Prompt:          "Inspect files",
			ProfileName:     "reviewer-profile",
			ToolPolicy:      "readOnly",
			ParallelGroupID: "group-1",
			ParallelIndex:   2,
			ContinueFrom:    "child-source",
			IsolationMode:   "worktree",
		},
		Execution: ExecutionResult{
			ProviderID:               "deepseek",
			Model:                    "deepseek-chat",
			EndpointFormat:           "chat_completions",
			Variant:                  "fast",
			Effort:                   "medium",
			Workspace:                "/workspace",
			Config:                   domainmodel.TurnConfig{BaseURL: "https://api.deepseek.com"},
			ProfileExecutionOverride: true,
		},
		ParentMaxModelSteps: &parentSteps,
		ToolScope:           []string{"read_file"},
		ToolSchemaHash:      "tool-hash",
		SystemPromptHash:    "prompt-hash",
		MaxChildRuns:        4,
	})

	if err != nil {
		t.Fatalf("BuildChildRunStartRequest returned error: %v", err)
	}
	if request.ParentGoalID != "goal-1" ||
		request.ParentThreadID != "thread-parent" ||
		request.ParentTurnID != "turn-parent" ||
		request.ParentToolItemID != "item-call-1" ||
		request.ParentToolCallID != "call-1" ||
		request.ChildThreadID != "thread-child" ||
		request.Kind != "subagent" ||
		request.Name != "reviewer" ||
		request.Label != "Review" ||
		request.Prompt != "Inspect files" ||
		request.SourceRef != "child-source" ||
		request.Status != "queued" {
		t.Fatalf("lineage mismatch: %#v", request)
	}
	if request.ProviderID != "deepseek" ||
		request.Model != "deepseek-chat" ||
		request.EndpointFormat != "" ||
		request.Variant != "fast" ||
		request.ModelSource != "subagent-profile" ||
		request.ModelExecution["providerId"] != "deepseek" ||
		request.ModelExecution["modelId"] != "deepseek-chat" ||
		request.Effort != "medium" ||
		request.Workspace != "/workspace" {
		t.Fatalf("execution mismatch: %#v", request)
	}
	if request.ModelExecution["endpointFormat"] != nil || request.ModelExecution["baseUrlFingerprint"] != nil ||
		request.ModelExecution["customFullEndpointFingerprint"] != nil {
		t.Fatalf("child run persisted Registry route authority: %#v", request.ModelExecution)
	}
	if request.MaxModelSteps == nil || *request.MaxModelSteps != 12 {
		t.Fatalf("max steps mismatch: %#v", request.MaxModelSteps)
	}
	if request.ProfileSource != "profile:reviewer-profile" ||
		request.ToolPolicy != "readOnly" ||
		len(request.ToolScope) != 1 ||
		request.ToolScope[0] != "read_file" ||
		request.ToolSchemaHash != "tool-hash" ||
		request.SystemPromptHash != "prompt-hash" ||
		!request.DefaultModelInherited ||
		request.ParallelGroupID != "group-1" ||
		request.ParallelIndex != 2 ||
		request.ContinueFrom != "child-source" ||
		request.IsolationMode != "worktree" ||
		request.MergeStatus != "not_requested" ||
		request.MaxChildRuns != 4 ||
		!request.MaxChildRunsSet {
		t.Fatalf("policy/profile mismatch: %#v", request)
	}
}
