package subagent

import (
	"strings"

	modelapp "analytix.local/runtime-go/internal/app/model"
	domainjob "analytix.local/runtime-go/internal/domain/job"
)

type ChildRunStartInput struct {
	ParentGoalID          string
	ParentGoalObjective   string
	ParentThreadID        string
	ParentTurnID          string
	ParentToolItemID      string
	ParentToolCallID      string
	SecurityBinding       *domainjob.SecurityBinding
	ChildThreadID         string
	FallbackName          string
	SourceRef             string
	Request               TaskRequest
	Execution             ExecutionResult
	ParentMaxModelSteps   *int
	ToolScope             []string
	ToolSchemaHash        string
	DelegatedToolManifest *domainjob.DelegatedToolManifestV1
	SystemPromptHash      string
	MaxChildRuns          int
}

func BuildChildRunStartRequest(input ChildRunStartInput) (domainjob.StartRequest, error) {
	request := input.Request
	execution := input.Execution
	maxSteps := ResolvedMaxSteps(request, input.ParentMaxModelSteps)
	modelSource := ModelSource(execution.ExplicitExecutionOverride, execution.ProfileExecutionOverride, execution.SourceExecutionInherited)
	modelExecution, err := modelapp.ExecutionRef(execution.ProviderID, execution.Model, execution.Variant, "", "", modelSource)
	if err != nil {
		return domainjob.StartRequest{}, err
	}
	return domainjob.StartRequest{
		ParentGoalID:          input.ParentGoalID,
		ParentGoalObjective:   input.ParentGoalObjective,
		ParentThreadID:        input.ParentThreadID,
		ParentTurnID:          input.ParentTurnID,
		ParentToolItemID:      input.ParentToolItemID,
		ParentToolCallID:      input.ParentToolCallID,
		SecurityBinding:       domainjob.CloneSecurityBinding(input.SecurityBinding),
		CaseDelegation:        domainjob.CloneCaseDelegationContextV1(request.CaseDelegation),
		ChildThreadID:         input.ChildThreadID,
		Kind:                  "subagent",
		Name:                  firstNonEmptyAnyString(request.Name, input.FallbackName),
		Label:                 request.Label,
		Prompt:                request.Prompt,
		Workspace:             execution.Workspace,
		Model:                 execution.Model,
		ProviderID:            execution.ProviderID,
		EndpointFormat:        "",
		Variant:               execution.Variant,
		ModelSource:           modelSource,
		ModelExecution:        modelExecution,
		Effort:                execution.Effort,
		MaxModelSteps:         &maxSteps,
		ProfileName:           request.ProfileName,
		ProfileSource:         ProfileSource(request),
		ToolPolicy:            request.ToolPolicy,
		ToolScope:             input.ToolScope,
		ToolSchemaHash:        input.ToolSchemaHash,
		DelegatedToolManifest: domainjob.CloneDelegatedToolManifestV1(input.DelegatedToolManifest),
		SystemPromptHash:      input.SystemPromptHash,
		SkillPackageDigest:    request.SkillPackageDigest,
		ProfileMode:           request.ProfileMode,
		ProfileDescription:    request.ProfileDescription,
		ProfileColor:          request.ProfileColor,
		ProfileIcon:           request.ProfileIcon,
		ReturnFormat:          request.ReturnFormat,
		TokenBudget:           request.TokenBudget,
		TimeBudgetMs:          request.TimeBudgetMS,
		DefaultModelInherited: strings.TrimSpace(request.Model) == "",
		ParallelGroupID:       request.ParallelGroupID,
		ParallelIndex:         request.ParallelIndex,
		ContinueFrom:          request.ContinueFrom,
		ForkFrom:              request.ForkFrom,
		SourceRef:             input.SourceRef,
		Background:            request.RunInBackground,
		AutoContinueParent:    request.AutoContinueParent,
		IsolationMode:         request.IsolationMode,
		MergeStatus:           mergeStatusForRequest(request),
		Status:                string(domainjob.StatusQueued),
		MaxChildRuns:          input.MaxChildRuns,
		MaxChildRunsSet:       true,
	}, nil
}

func mergeStatusForRequest(request TaskRequest) string {
	if strings.TrimSpace(request.IsolationMode) == string(domainjob.IsolationWorktree) {
		return "not_requested"
	}
	return ""
}
