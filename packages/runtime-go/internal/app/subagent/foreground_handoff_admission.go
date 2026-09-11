package subagent

import (
	"errors"
	"strings"

	modelapp "analytix.local/runtime-go/internal/app/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	ForegroundHandoffMaxSteps       = 4
	ForegroundHandoffMaxTokenBudget = 4096
	ForegroundHandoffMaxTimeBudget  = 60000
	ForegroundHandoffGeneralV1      = "general"
	ForegroundHandoffCaseTypedV1    = "case_typed"
)

func BindForegroundHandoffPendingRequest(pending modelapp.PendingToolCall, request TaskRequest) (TaskRequest, error) {
	if !IsForegroundHandoffPendingToolCallV1(pending) {
		return request, nil
	}
	return BindForegroundHandoffRequest(pending.SecurityContext, request)
}

// BindForegroundHandoffRequest converts an already strictly-decoded task
// request into an optional bounded child-result handoff. Provider input cannot
// set the marker directly; other ordinary subagent requests keep their normal
// host policy instead of being forced into this shape.
func BindForegroundHandoffRequest(parent domainsecurity.TurnSecurityContext, request TaskRequest) (TaskRequest, error) {
	kind := ""
	switch {
	case domainsecurity.TurnSecurityContextUsesHostGeneralOnlyRisk(parent):
		kind = ForegroundHandoffGeneralV1
	case domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(parent) == nil:
		kind = ForegroundHandoffCaseTypedV1
	default:
		return request, errors.New("foreground child handoff requires current host authority")
	}
	request.ForegroundHandoff = true
	request.ForegroundHandoffKind = kind
	request.ToolPolicy = "readOnly"
	request.ToolPolicySet = true
	request.ToolPolicySource = "host-" + kind
	request.Tools = []string{toolcatalogForegroundSubmitToolName}
	request.ReturnFormat = "summary"
	if err := ValidateForegroundHandoffRequest(request); err != nil {
		return TaskRequest{}, err
	}
	return request, nil
}

func ValidateForegroundHandoffRequest(request TaskRequest) error {
	if !request.ForegroundHandoff || strings.TrimSpace(request.Prompt) == "" || len([]byte(request.Prompt)) > 8192 ||
		(request.ForegroundHandoffKind != ForegroundHandoffGeneralV1 && request.ForegroundHandoffKind != ForegroundHandoffCaseTypedV1) ||
		!request.MaxStepsSet || request.MaxSteps < 1 || request.MaxSteps > ForegroundHandoffMaxSteps ||
		!request.TokenBudgetSet || request.TokenBudget < 1 || request.TokenBudget > ForegroundHandoffMaxTokenBudget ||
		!request.TimeBudgetMSSet || request.TimeBudgetMS < 1 || request.TimeBudgetMS > ForegroundHandoffMaxTimeBudget ||
		request.RunInBackground || request.AutoContinueParent || strings.TrimSpace(request.IsolationMode) != "" ||
		strings.TrimSpace(request.ContinueFrom) != "" || strings.TrimSpace(request.ForkFrom) != "" ||
		strings.TrimSpace(request.Workspace) != "" || strings.TrimSpace(request.ProviderID) != "" ||
		strings.TrimSpace(request.Model) != "" || strings.TrimSpace(request.EndpointFormat) != "" ||
		strings.TrimSpace(request.Variant) != "" || strings.TrimSpace(request.Effort) != "" ||
		strings.TrimSpace(request.ProfileName) != "" || len(request.BlockedTools) != 0 ||
		len(request.BlockedMCPServers) != 0 || len(request.BlockedSkills) != 0 {
		return errors.New("foreground child handoff request exceeds its closed authority")
	}
	if len(request.Tools) != 1 || request.Tools[0] != toolcatalogForegroundSubmitToolName {
		return errors.New("foreground child handoff tool scope is invalid")
	}
	return nil
}

func ValidateForegroundHandoffExecution(request TaskRequest, execution ExecutionResult, parent ParentExecution) error {
	if err := ValidateForegroundHandoffRequest(request); err != nil {
		return err
	}
	if strings.TrimSpace(execution.ProviderID) == "" || execution.ProviderID != strings.TrimSpace(parent.ProviderID) ||
		execution.Model != strings.TrimSpace(parent.Model) || execution.Workspace != strings.TrimSpace(parent.Workspace) ||
		execution.ExplicitExecutionOverride || execution.ProfileExecutionOverride || execution.SourceExecutionInherited {
		return errors.New("foreground child handoff execution differs from its parent authority")
	}
	return nil
}
