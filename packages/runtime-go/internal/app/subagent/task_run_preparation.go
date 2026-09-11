package subagent

import (
	"errors"
	"fmt"
	"strings"

	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type TaskRunPreparationInput struct {
	Enabled                  bool
	Settings                 ProfileSettings
	Request                  TaskRequest
	SystemPromptHint         string
	Parent                   ParentExecution
	ParentWorkspaceRealPath  string
	ParentThreadID           string
	SecurityContext          domainsecurity.TurnSecurityContext
	Resolver                 ExecutionResolver
	LockSource               func(string) (func(), error)
	ResolveSource            func(TaskRequest) (domainjob.Record, bool, error)
	ResolveSourceLabel       func(domainjob.Record) string
	CanonicalizeWorkspace    func(string) (string, error)
	ResolveMCPAdvertisements func() []toolcatalogapp.MCPToolAdvertisementV1
	AllowedToolScope         func(TaskRequest, []string) []string
	ToolSchemas              func([]string, string, []toolcatalogapp.MCPToolAdvertisementV1) []domainmodel.ToolSchema
	ThreadIsDescendantOf     func(string, string) (bool, error)
}

type TaskRunPreparation struct {
	Request               TaskRequest
	Source                domainjob.Record
	HasSource             bool
	Execution             ExecutionResult
	Workspace             string
	ToolScope             []string
	ToolSchemaHash        string
	DelegatedToolManifest *domainjob.DelegatedToolManifestV1
	SystemPromptHash      string
	sourceLockScope       string
	releaseSourceLock     func()
}

func (preparation *TaskRunPreparation) ReleaseSourceLock() {
	if preparation == nil || preparation.releaseSourceLock == nil {
		return
	}
	preparation.releaseSourceLock()
	preparation.releaseSourceLock = nil
}

func (preparation *TaskRunPreparation) TransferSourceLock() func() {
	if preparation == nil || preparation.releaseSourceLock == nil {
		return func() {}
	}
	release := preparation.releaseSourceLock
	preparation.releaseSourceLock = nil
	return release
}

func (preparation *TaskRunPreparation) SourceLockHeld() bool {
	return preparation != nil && preparation.releaseSourceLock != nil
}

func (preparation *TaskRunPreparation) SourceLockScope() string {
	if preparation == nil {
		return ""
	}
	return preparation.sourceLockScope
}

// PrepareTaskRun resolves the host-owned profile, source, execution,
// workspace, delegated tools, and replay authority before the server creates a
// durable child run. A successfully returned source lock remains owned by the
// preparation until the caller releases or transfers it.
func PrepareTaskRun(input TaskRunPreparationInput) (preparation TaskRunPreparation, err error) {
	preparation.Request = input.Request
	defer func() {
		if err != nil {
			preparation.ReleaseSourceLock()
		}
	}()
	if !input.Enabled {
		return preparation, errors.New("subagents are disabled in Analytix runtime settings")
	}
	preparation.Request, err = ApplyProfile(preparation.Request, input.Settings)
	if err != nil {
		return preparation, err
	}
	preparation.Request = WithSystemPromptHint(preparation.Request, input.SystemPromptHint)
	request := preparation.Request
	if request.ContinueFrom != "" && request.ForkFrom != "" {
		return preparation, errors.New("continue_from and fork_from are mutually exclusive")
	}
	if ref := strings.TrimSpace(firstNonEmptyAnyString(request.ContinueFrom, request.ForkFrom)); ref != "" {
		if input.LockSource == nil {
			return preparation, errors.New("subagent source lock owner is unavailable")
		}
		preparation.releaseSourceLock, err = input.LockSource(ref)
		if err != nil {
			return preparation, err
		}
		if strings.TrimSpace(request.ContinueFrom) != "" {
			preparation.sourceLockScope = "continue"
		} else {
			preparation.sourceLockScope = "fork"
		}
	}
	if input.ResolveSource == nil || input.CanonicalizeWorkspace == nil || input.ResolveMCPAdvertisements == nil || input.AllowedToolScope == nil || input.ToolSchemas == nil {
		return preparation, errors.New("subagent task preparation owner is unavailable")
	}
	preparation.Source, preparation.HasSource, err = input.ResolveSource(request)
	if err != nil {
		return preparation, err
	}
	if preparation.HasSource && !request.LabelExplicit && input.ResolveSourceLabel != nil {
		sourceLabel := input.ResolveSourceLabel(preparation.Source)
		if request.ContinueFrom != "" {
			request.Label = sourceLabel
		} else if request.ForkFrom != "" {
			request.Label = strings.TrimSpace(sourceLabel + " fork")
		}
		if !request.NameExplicit {
			request.Name = sourceLabel
		}
		preparation.Request = request
	}
	preparation.Execution, err = ResolveExecution(ExecutionInput{
		Request: request, Parent: input.Parent, Source: preparation.Source, HasSource: preparation.HasSource, Resolver: input.Resolver,
	})
	if err != nil {
		return preparation, err
	}
	if request.ForegroundHandoff {
		if err = ValidateForegroundHandoffExecution(request, preparation.Execution, input.Parent); err != nil {
			return preparation, err
		}
	}
	if err = ValidateWorktreeIsolationRequest(request); err != nil {
		return preparation, err
	}
	preparation.Workspace, err = input.CanonicalizeWorkspace(preparation.Execution.Workspace)
	if err != nil {
		return preparation, err
	}
	preparation.Execution.Workspace = preparation.Workspace
	parentWorkspace, workspaceErr := input.CanonicalizeWorkspace(input.ParentWorkspaceRealPath)
	if workspaceErr != nil {
		return preparation, errors.New("frozen parent workspace authority is unavailable: " + workspaceErr.Error())
	}
	if err = ValidateChildWorkspaceAdmission(ChildWorkspaceAdmissionInput{
		Request: request, ParentWorkspaceRealPath: parentWorkspace, ChildWorkspaceRealPath: preparation.Workspace,
	}); err != nil {
		return preparation, err
	}
	advertisements := input.ResolveMCPAdvertisements()
	allowedTools := input.AllowedToolScope(request, toolcatalogapp.MCPToolNamesFromAdvertisementsV1(advertisements))
	if strings.HasPrefix(strings.TrimSpace(request.ProfileName), "skill:") {
		preparation.Request, err = RestrictSkillToolsToHost(request, allowedTools)
		if err != nil {
			return preparation, err
		}
		request = preparation.Request
	} else if dropped := DroppedRequestedTools(request, allowedTools); len(dropped) > 0 {
		return preparation, fmt.Errorf("subagent tools not available under toolPolicy %q: %s", request.ToolPolicy, strings.Join(dropped, ", "))
	}
	preparation.ToolScope = ToolScope(request.Tools, allowedTools)
	toolSchemas := input.ToolSchemas(preparation.ToolScope, request.Prompt, advertisements)
	preparation.ToolSchemaHash = toolcatalogapp.ToolSchemaHash(toolSchemas)
	preparation.DelegatedToolManifest, err = toolcatalogapp.BuildDelegatedToolManifestV1(preparation.ToolScope, toolSchemas, advertisements)
	if err != nil {
		return preparation, errors.New("subagent delegated tool manifest is unavailable: " + err.Error())
	}
	preparation.SystemPromptHash = domainmodel.StringHash(SystemPromptForRequest(request))
	if preparation.HasSource {
		allowAncestorSource := false
		if strings.TrimSpace(preparation.Source.ParentThreadID) != "" && preparation.Source.ParentThreadID != input.ParentThreadID {
			if input.ThreadIsDescendantOf == nil {
				return preparation, errors.New("subagent source ancestry owner is unavailable")
			}
			allowAncestorSource, err = input.ThreadIsDescendantOf(input.ParentThreadID, preparation.Source.ParentThreadID)
			if err != nil {
				return preparation, err
			}
		}
		if err = ValidateSource(preparation.Source, request, SourceValidation{
			ParentThreadID: input.ParentThreadID, AllowAncestorSource: allowAncestorSource,
			Workspace: preparation.Workspace, ProviderID: preparation.Execution.ProviderID, Model: preparation.Execution.Model,
			EndpointFormat: "", Variant: preparation.Execution.Variant, Effort: preparation.Execution.Effort,
			ToolScope: preparation.ToolScope, ToolSchemaHash: preparation.ToolSchemaHash, SystemPromptHash: preparation.SystemPromptHash,
			SkillPackageDigest: request.SkillPackageDigest,
		}); err != nil {
			return preparation, err
		}
		securityContextMatches := domainjob.SecurityBindingMatchesCaseEpoch(preparation.Source.SecurityBinding, input.SecurityContext)
		if allowAncestorSource {
			securityContextMatches = domainjob.SecurityBindingMatchesCaseEpochScope(preparation.Source.SecurityBinding, input.SecurityContext)
		}
		if !securityContextMatches {
			return preparation, errors.New("subagent source belongs to a different frozen case context")
		}
	}
	return preparation, nil
}
