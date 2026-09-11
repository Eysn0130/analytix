package subagent

import (
	"errors"
	"fmt"
	"strings"

	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

type SideEffectProjectionInput struct {
	Enabled                 bool
	Settings                ProfileSettings
	Request                 TaskRequest
	Parent                  ParentExecution
	ParentWorkspaceRealPath string
	CallName                string
	MaxModelSteps           *int
	EffectiveMaxModelSteps  *int
	Resolver                ExecutionResolver
	ResolveSource           func(TaskRequest) (domainjob.Record, bool, error)
	ResolveSourceLabel      func(domainjob.Record) string
	CanonicalizeWorkspace   func(string) (string, error)
	MCPAdvertisements       []toolcatalogapp.MCPToolAdvertisementV1
	AllowedToolScope        func(TaskRequest, []string) []string
	ToolSchemas             func([]string, string, []toolcatalogapp.MCPToolAdvertisementV1) []domainmodel.ToolSchema
}

// ResolveSideEffectProjection applies the same effective profile, execution,
// workspace, and delegated-tool owners used by a physical child run. Raw
// aliases and unused request fields cannot mint a second semantic intent.
func ResolveSideEffectProjection(input SideEffectProjectionInput) (map[string]any, error) {
	if !input.Enabled {
		return nil, errors.New("subagents are disabled in Analytix runtime settings")
	}
	request, err := ApplyProfile(input.Request, input.Settings)
	if err != nil {
		return nil, err
	}
	if request.ContinueFrom != "" && request.ForkFrom != "" {
		return nil, errors.New("continue_from and fork_from are mutually exclusive")
	}
	if err := ValidateWorktreeIsolationRequest(request); err != nil {
		return nil, err
	}
	if input.ResolveSource == nil || input.CanonicalizeWorkspace == nil || input.AllowedToolScope == nil || input.ToolSchemas == nil {
		return nil, errors.New("subagent side-effect owner is unavailable")
	}
	source, hasSource, err := input.ResolveSource(request)
	if err != nil {
		return nil, err
	}
	if hasSource && !request.LabelExplicit && input.ResolveSourceLabel != nil {
		sourceLabel := input.ResolveSourceLabel(source)
		if request.ContinueFrom != "" {
			request.Label = sourceLabel
		} else if request.ForkFrom != "" {
			request.Label = strings.TrimSpace(sourceLabel + " fork")
		}
		if !request.NameExplicit {
			request.Name = sourceLabel
		}
	}
	execution, err := ResolveExecution(ExecutionInput{
		Request: request, Parent: input.Parent, Source: source, HasSource: hasSource, Resolver: input.Resolver,
	})
	if err != nil {
		return nil, err
	}
	if request.ForegroundHandoff {
		if err := ValidateForegroundHandoffExecution(request, execution, input.Parent); err != nil {
			return nil, err
		}
	}
	workspace, err := input.CanonicalizeWorkspace(execution.Workspace)
	if err != nil {
		return nil, err
	}
	parentWorkspace, err := input.CanonicalizeWorkspace(input.ParentWorkspaceRealPath)
	if err != nil {
		return nil, errors.New("frozen parent workspace authority is unavailable")
	}
	if err := ValidateChildWorkspaceAdmission(ChildWorkspaceAdmissionInput{
		Request: request, ParentWorkspaceRealPath: parentWorkspace, ChildWorkspaceRealPath: workspace,
	}); err != nil {
		return nil, err
	}
	allowed := input.AllowedToolScope(request, toolcatalogapp.MCPToolNamesFromAdvertisementsV1(input.MCPAdvertisements))
	if strings.HasPrefix(strings.TrimSpace(request.ProfileName), "skill:") {
		request, err = RestrictSkillToolsToHost(request, allowed)
		if err != nil {
			return nil, err
		}
	} else if dropped := DroppedRequestedTools(request, allowed); len(dropped) > 0 {
		return nil, fmt.Errorf("subagent tools not available under toolPolicy %q: %s", request.ToolPolicy, strings.Join(dropped, ", "))
	}
	toolScope := ToolScope(request.Tools, allowed)
	toolSchemas := input.ToolSchemas(toolScope, request.Prompt, input.MCPAdvertisements)
	toolSchemaHash := toolcatalogapp.ToolSchemaHash(toolSchemas)
	if _, err := toolcatalogapp.BuildDelegatedToolManifestV1(toolScope, toolSchemas, input.MCPAdvertisements); err != nil {
		return nil, err
	}
	return (TaskRunPreparation{Request: request, Source: source, Execution: execution, Workspace: workspace,
		ToolScope: toolScope, ToolSchemaHash: toolSchemaHash, SystemPromptHash: domainmodel.StringHash(SystemPromptForRequest(request)),
	}).SideEffectProjectionV1(input.CallName, ParentMaxModelSteps(input.MaxModelSteps, input.EffectiveMaxModelSteps)), nil
}

// SideEffectProjectionV1 preserves the semantic identity owner while allowing
// the runtime to sign the exact preparation admitted for child production.
func (preparation TaskRunPreparation) SideEffectProjectionV1(callName string, parentMaxSteps *int) map[string]any {
	request, source, execution := preparation.Request, preparation.Source, preparation.Execution
	workspace, toolScope, toolSchemaHash := preparation.Workspace, preparation.ToolScope, preparation.ToolSchemaHash
	return map[string]any{
		"id": request.ID, "dependsOn": request.DependsOn,
		"name": firstNonEmptySideEffectString(request.Name, callName), "label": request.Label, "prompt": request.Prompt,
		"workspace": workspace, "providerId": execution.ProviderID, "model": execution.Model,
		"variant": execution.Variant, "effort": execution.Effort,
		"toolPolicy": request.ToolPolicy, "toolScope": toolScope, "toolSchemaHash": toolSchemaHash,
		"systemPromptHash": preparation.SystemPromptHash,
		"maxSteps":         ResolvedMaxSteps(request, parentMaxSteps),
		"tokenBudget":      request.TokenBudget, "timeBudgetMs": request.TimeBudgetMS, "returnFormat": request.ReturnFormat,
		"runInBackground": request.RunInBackground, "autoContinueParent": request.AutoContinueParent,
		"isolationMode": request.IsolationMode, "continueFrom": request.ContinueFrom, "forkFrom": request.ForkFrom,
		"sourceRef": source.ID, "skillPackageDigest": request.SkillPackageDigest,
		"profileName": request.ProfileName, "profileMode": request.ProfileMode,
		"profileDescription": request.ProfileDescription, "profileColor": request.ProfileColor, "profileIcon": request.ProfileIcon,
	}
}

func firstNonEmptySideEffectString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
