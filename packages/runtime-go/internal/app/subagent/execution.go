package subagent

import (
	"errors"
	"strings"

	modelapp "analytix.local/runtime-go/internal/app/model"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

type ExecutionResolver interface {
	HasProvider(providerID string) bool
	TurnConfigForExecution(providerID string, providerModel string) domainmodel.TurnConfig
	ValidateExecutionModel(providerID string, model string) error
}

type ParentExecution struct {
	ProviderID string
	Model      string
	Effort     string
	Workspace  string
}

type ExecutionInput struct {
	Request   TaskRequest
	Parent    ParentExecution
	Source    domainjob.Record
	HasSource bool
	Resolver  ExecutionResolver
}

type ExecutionResult struct {
	ProviderID                string
	Model                     string
	EndpointFormat            string
	Variant                   string
	Effort                    string
	Workspace                 string
	Config                    domainmodel.TurnConfig
	ExplicitExecutionOverride bool
	ProfileExecutionOverride  bool
	SourceExecutionInherited  bool
}

func ResolveExecution(input ExecutionInput) (ExecutionResult, error) {
	resolver := input.Resolver
	if resolver == nil {
		return ExecutionResult{}, errors.New("subagent execution resolver is required")
	}
	request := input.Request
	parent := input.Parent
	source := input.Source
	if err := domainmodel.ValidateReasoningEffortV1(request.Effort); err != nil {
		return ExecutionResult{}, err
	}
	if err := domainmodel.ValidateReasoningEffortV1(parent.Effort); err != nil {
		return ExecutionResult{}, err
	}
	if input.HasSource {
		if err := domainmodel.ValidateReasoningEffortV1(source.Effort); err != nil {
			return ExecutionResult{}, err
		}
	}
	explicitExecutionOverride := request.ModelExplicit || request.ProviderExplicit || request.EndpointExplicit || request.VariantExplicit
	profileExecutionOverride := request.ProfileExecutionConfigured
	requestedModel := strings.TrimSpace(request.Model)
	if requestedModel == "" && strings.TrimSpace(request.ProviderID) != "" && request.ProviderID != parent.ProviderID {
		requestedModel = ""
	} else if requestedModel == "" {
		requestedModel = parent.Model
	}
	providerID := firstNonEmptyAnyString(request.ProviderID, parent.ProviderID)
	if providerID != "" && !resolver.HasProvider(providerID) {
		return ExecutionResult{}, domainfailure.NewError(domainfailure.CodeProviderNotConfigured, nil)
	}
	resolvedConfig := applyEndpointOverride(resolver.TurnConfigForExecution(providerID, requestedModel), request.EndpointFormat)
	providerID = resolvedConfig.ProviderID
	model := resolvedConfig.Model
	endpointFormat := resolvedConfig.EndpointFormat
	variant := strings.TrimSpace(request.Variant)
	effort := firstNonEmptyAnyString(request.Effort, parent.Effort)
	workspace := firstNonEmptyAnyString(request.Workspace, parent.Workspace)
	sourceExecutionInherited := false
	if input.HasSource {
		if !explicitExecutionOverride && !profileExecutionOverride && strings.TrimSpace(source.ProviderID) != "" {
			providerID = source.ProviderID
		}
		if !explicitExecutionOverride && !profileExecutionOverride && strings.TrimSpace(source.EndpointFormat) != "" {
			endpointFormat = source.EndpointFormat
		}
		if !explicitExecutionOverride && !profileExecutionOverride && strings.TrimSpace(source.Model) != "" {
			model = source.Model
		}
		if !explicitExecutionOverride && !profileExecutionOverride && strings.TrimSpace(source.Variant) != "" {
			variant = source.Variant
		}
		if !explicitExecutionOverride && !profileExecutionOverride && (source.ProviderID != "" || source.Model != "" || source.EndpointFormat != "" || source.Variant != "") {
			sourceExecutionInherited = true
		}
		if strings.TrimSpace(request.Effort) == "" && strings.TrimSpace(source.Effort) != "" {
			effort = source.Effort
		}
		if strings.TrimSpace(request.Workspace) == "" && strings.TrimSpace(source.Workspace) != "" {
			workspace = source.Workspace
		}
	}
	if providerID != "" && !resolver.HasProvider(providerID) {
		return ExecutionResult{}, domainfailure.NewError(domainfailure.CodeProviderNotConfigured, nil)
	}
	resolvedConfig = applyEndpointOverride(resolver.TurnConfigForExecution(providerID, model), endpointFormat)
	providerID = resolvedConfig.ProviderID
	model = resolvedConfig.Model
	endpointFormat = resolvedConfig.EndpointFormat
	if err := resolver.ValidateExecutionModel(providerID, model); err != nil {
		return ExecutionResult{}, err
	}
	if err := domainmodel.ValidateReasoningEffortV1(effort); err != nil {
		return ExecutionResult{}, err
	}
	return ExecutionResult{
		ProviderID:                providerID,
		Model:                     model,
		EndpointFormat:            endpointFormat,
		Variant:                   variant,
		Effort:                    effort,
		Workspace:                 workspace,
		Config:                    resolvedConfig,
		ExplicitExecutionOverride: explicitExecutionOverride,
		ProfileExecutionOverride:  profileExecutionOverride,
		SourceExecutionInherited:  sourceExecutionInherited,
	}, nil
}

func applyEndpointOverride(config domainmodel.TurnConfig, endpointFormat string) domainmodel.TurnConfig {
	if endpointOverride := modelapp.OptionalEndpointFormat(endpointFormat); endpointOverride != "" {
		config.EndpointFormat = endpointOverride
		config.Family = modelapp.ProviderFamily(config.ProviderID, config.BaseURL, config.Model, config.EndpointFormat)
		config.CacheTelemetrySupported = config.Family == "deepseek"
		config.DeepSeekPrefixEnhancement = config.Family == "deepseek"
	}
	return config
}
