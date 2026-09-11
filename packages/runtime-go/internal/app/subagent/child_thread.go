package subagent

import (
	"errors"
	"strings"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

type ChildThreadStore interface {
	ForkThread(threadID string, request map[string]any) (map[string]any, error)
	CreateThread(request map[string]any, fallbackWorkspace string) (map[string]any, error)
	PatchThread(threadID string, patch map[string]any) (map[string]any, error)
}

type PrepareChildThreadInput struct {
	Store                 ChildThreadStore
	ParentThreadID        string
	PendingCallID         string
	Request               TaskRequest
	Source                domainjob.Record
	HasSource             bool
	DistinctNameCandidate string
	ProviderID            string
	Model                 string
	EndpointFormat        string
	Effort                string
	Workspace             string
	ApprovalPolicy        string
	SandboxMode           string
}

func PrepareChildThread(input PrepareChildThreadInput) (string, error) {
	if err := domainmodel.ValidateReasoningEffortV1(input.Effort); err != nil {
		return "", err
	}
	if input.Store == nil {
		return "", errors.New("child thread store is required")
	}
	if input.HasSource && SecurityBoundChildOutput(input.Source) && (input.Request.ContinueFrom != "" || input.Request.ForkFrom != "") {
		return "", errors.New("security_bound_source_output_unavailable")
	}
	if input.Request.ContinueFrom != "" && input.HasSource && input.Source.ParentThreadID == input.ParentThreadID {
		return input.Source.ChildThreadID, nil
	}
	threadPlan := ChildThreadPlanInput{
		Request:               input.Request,
		DistinctNameCandidate: input.DistinctNameCandidate,
		PendingCallID:         input.PendingCallID,
		ProviderID:            input.ProviderID,
		Model:                 input.Model,
		EndpointFormat:        input.EndpointFormat,
		Effort:                input.Effort,
		Workspace:             input.Workspace,
		ApprovalPolicy:        input.ApprovalPolicy,
		SandboxMode:           input.SandboxMode,
	}
	title := ChildThreadTitle(threadPlan)
	if input.HasSource && (input.Request.ForkFrom != "" || input.Request.ContinueFrom != "") {
		fork, err := input.Store.ForkThread(input.Source.ChildThreadID, map[string]any{
			"relation":       "side",
			"parentThreadId": input.ParentThreadID,
			"title":          ForkedChildThreadTitle(title, input.Request),
		})
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(firstNonEmptyAnyString(fork["id"])), nil
	}
	createPatch := ChildThreadCreatePatch(threadPlan)
	createPatch["relation"] = "side"
	createPatch["parentThreadId"] = strings.TrimSpace(input.ParentThreadID)
	thread, err := input.Store.CreateThread(createPatch, input.Workspace)
	if err != nil {
		return "", err
	}
	threadID := strings.TrimSpace(firstNonEmptyAnyString(thread["id"]))
	if threadID == "" {
		return "", errors.New("created child thread without id")
	}
	if _, err := input.Store.PatchThread(threadID, map[string]any{"relation": "side", "parentThreadId": strings.TrimSpace(input.ParentThreadID)}); err != nil {
		return "", err
	}
	return threadID, nil
}
