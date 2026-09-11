package subagent

import "testing"

func TestChildThreadPlanBuildsTitleAndCreatePatch(t *testing.T) {
	input := ChildThreadPlanInput{
		Request:               TaskRequest{Name: "Reviewer", Label: "Reviewer", ProfileName: "reviewer", ToolPolicy: "readOnly"},
		DistinctNameCandidate: "Reviewer",
		PendingCallID:         "call_1",
		ProviderID:            "openai",
		Model:                 "gpt-5",
		EndpointFormat:        "responses",
		Effort:                "high",
		Workspace:             "/work",
		ApprovalPolicy:        "on-request",
		SandboxMode:           "workspace-write",
	}
	if title := ChildThreadTitle(input); title != "Child agent: Reviewer" {
		t.Fatalf("title mismatch: %q", title)
	}
	patch := ChildThreadCreatePatch(input)
	if patch["title"] != "Child agent: Reviewer" ||
		patch["providerId"] != "openai" ||
		patch["model"] != "gpt-5" ||
		patch["approvalPolicy"] != "never" ||
		patch["sandboxMode"] != "workspace-write" {
		t.Fatalf("child thread patch mismatch: %#v", patch)
	}
	if _, ok := patch["endpointFormat"]; ok {
		t.Fatalf("child thread persisted Registry route authority: %#v", patch)
	}
}

func TestChildThreadPlanFallbacksAndForkTitles(t *testing.T) {
	input := ChildThreadPlanInput{
		Request:       TaskRequest{ID: "task_2", ParallelIndex: 2},
		PendingCallID: "call_1",
	}
	if title := ChildThreadTitle(input); title != "Child agent: Nash" {
		t.Fatalf("fallback title mismatch: %q", title)
	}
	if title := ForkedChildThreadTitle("Child agent: Nash", TaskRequest{ForkFrom: "job_1"}); title != "Child agent: Nash fork" {
		t.Fatalf("fork title mismatch: %q", title)
	}
	if title := ForkedChildThreadTitle("Child agent: Nash", TaskRequest{ContinueFrom: "job_1"}); title != "Child agent: Nash continuation" {
		t.Fatalf("continuation title mismatch: %q", title)
	}
}
