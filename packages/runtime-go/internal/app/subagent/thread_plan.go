package subagent

import (
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

type ChildThreadPlanInput struct {
	Request               TaskRequest
	DistinctNameCandidate string
	PendingCallID         string
	ProviderID            string
	Model                 string
	EndpointFormat        string
	Effort                string
	Workspace             string
	ApprovalPolicy        string
	SandboxMode           string
}

func ChildThreadTitle(input ChildThreadPlanInput) string {
	titleLabel := DisplayNameCandidate(input.DistinctNameCandidate)
	if titleLabel == "" {
		request := input.Request
		titleLabel = GeneratedNickname(ParallelIndexSeed(request.ParallelIndex), request.ID, input.PendingCallID, request.Name, request.Label, request.ProfileName)
	}
	return "Child agent: " + titleLabel
}

func ForkedChildThreadTitle(title string, request TaskRequest) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Child agent: Subagent"
	}
	titleSuffix := " fork"
	if strings.TrimSpace(request.ContinueFrom) != "" {
		titleSuffix = " continuation"
	}
	return title + titleSuffix
}

func ChildThreadCreatePatch(input ChildThreadPlanInput) map[string]any {
	request := input.Request
	effort, _ := domainmodel.ProjectReasoningEffortV1(input.Effort)
	return map[string]any{
		"title":           ChildThreadTitle(input),
		"workspace":       strings.TrimSpace(input.Workspace),
		"model":           strings.TrimSpace(input.Model),
		"providerId":      strings.TrimSpace(input.ProviderID),
		"reasoningEffort": effort,
		"mode":            "agent",
		"approvalPolicy":  ApprovalPolicy(request.ToolPolicy, input.ApprovalPolicy),
		"sandboxMode":     strings.TrimSpace(input.SandboxMode),
	}
}
