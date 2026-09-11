package subagent

import (
	"errors"
	"strings"
	"testing"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestResolveExecutionInheritsSourceIdentityWhenNoOverride(t *testing.T) {
	resolver := newExecutionResolverStub(map[string]domainmodel.TurnConfig{
		"parent": {ProviderID: "parent", Model: "parent-model", EndpointFormat: "chat_completions", BaseURL: "https://parent.example"},
		"source": {ProviderID: "source", Model: "source-model", EndpointFormat: "responses", BaseURL: "https://source.example"},
	})
	result, err := ResolveExecution(ExecutionInput{
		Request: TaskRequest{},
		Parent:  ParentExecution{ProviderID: "parent", Model: "parent-model", Effort: "low", Workspace: "/parent"},
		Source: domainjob.Record{
			ProviderID:     "source",
			Model:          "source-model",
			EndpointFormat: "responses",
			Variant:        "fast",
			Effort:         "high",
			Workspace:      "/source",
		},
		HasSource: true,
		Resolver:  resolver,
	})

	if err != nil {
		t.Fatalf("ResolveExecution returned error: %v", err)
	}
	if result.ProviderID != "source" ||
		result.Model != "source-model" ||
		result.EndpointFormat != "responses" ||
		result.Variant != "fast" ||
		result.Effort != "high" ||
		result.Workspace != "/source" ||
		!result.SourceExecutionInherited {
		t.Fatalf("source execution mismatch: %#v", result)
	}
	if result.Config.ProviderID != "source" || result.Config.Model != "source-model" {
		t.Fatalf("resolved config mismatch: %#v", result.Config)
	}
}

func TestResolveExecutionPreservesExplicitProviderModelAndEndpoint(t *testing.T) {
	resolver := newExecutionResolverStub(map[string]domainmodel.TurnConfig{
		"parent":   {ProviderID: "parent", Model: "parent-model", EndpointFormat: "chat_completions", BaseURL: "https://parent.example"},
		"explicit": {ProviderID: "explicit", Model: "explicit-model", EndpointFormat: "chat_completions", BaseURL: "https://explicit.example"},
	})
	result, err := ResolveExecution(ExecutionInput{
		Request: TaskRequest{
			ProviderID:       "explicit",
			Model:            "explicit-model",
			EndpointFormat:   "responses",
			Variant:          "accurate",
			Effort:           "medium",
			ProviderExplicit: true,
			ModelExplicit:    true,
			EndpointExplicit: true,
			VariantExplicit:  true,
		},
		Parent:    ParentExecution{ProviderID: "parent", Model: "parent-model", Effort: "low", Workspace: "/parent"},
		Source:    domainjob.Record{ProviderID: "source", Model: "source-model", EndpointFormat: "chat_completions", Variant: "fast"},
		HasSource: true,
		Resolver:  resolver,
	})

	if err != nil {
		t.Fatalf("ResolveExecution returned error: %v", err)
	}
	if result.ProviderID != "explicit" ||
		result.Model != "explicit-model" ||
		result.EndpointFormat != "responses" ||
		result.Variant != "accurate" ||
		result.Effort != "medium" ||
		result.Workspace != "/parent" ||
		!result.ExplicitExecutionOverride ||
		result.SourceExecutionInherited {
		t.Fatalf("explicit execution mismatch: %#v", result)
	}
}

func TestResolveExecutionRejectsMissingProviderAndInvalidModel(t *testing.T) {
	resolver := newExecutionResolverStub(map[string]domainmodel.TurnConfig{
		"parent": {ProviderID: "parent", Model: "parent-model", EndpointFormat: "chat_completions"},
	})
	if _, err := ResolveExecution(ExecutionInput{
		Request:  TaskRequest{ProviderID: "missing", ProviderExplicit: true},
		Resolver: resolver,
	}); err == nil || err.Error() != "The selected provider is not configured for this runtime." {
		t.Fatalf("missing provider error mismatch: %v", err)
	}

	resolver.validateErr = errors.New("invalid model")
	if _, err := ResolveExecution(ExecutionInput{
		Request:  TaskRequest{},
		Parent:   ParentExecution{ProviderID: "parent", Model: "parent-model"},
		Resolver: resolver,
	}); err == nil || err.Error() != "invalid model" {
		t.Fatalf("invalid model error mismatch: %v", err)
	}
}

func TestResolveExecutionRejectsInvalidReasoningEffortWithoutReflection(t *testing.T) {
	const sentinel = "SOL_PRIVATE_REASONING_SENTINEL_7F3C"
	resolver := newExecutionResolverStub(map[string]domainmodel.TurnConfig{
		"parent": {ProviderID: "parent", Model: "parent-model", EndpointFormat: "chat_completions"},
	})
	for name, input := range map[string]ExecutionInput{
		"request": {Request: TaskRequest{Effort: sentinel}, Parent: ParentExecution{ProviderID: "parent", Model: "parent-model"}, Resolver: resolver},
		"parent":  {Parent: ParentExecution{ProviderID: "parent", Model: "parent-model", Effort: sentinel}, Resolver: resolver},
		"source": {
			Parent: ParentExecution{ProviderID: "parent", Model: "parent-model"},
			Source: domainjob.Record{Effort: sentinel}, HasSource: true, Resolver: resolver,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ResolveExecution(input); err == nil || strings.Contains(err.Error(), sentinel) {
				t.Fatalf("invalid reasoning effort was accepted or reflected: %v", err)
			}
		})
	}
}

type executionResolverStub struct {
	configs     map[string]domainmodel.TurnConfig
	validateErr error
}

func newExecutionResolverStub(configs map[string]domainmodel.TurnConfig) *executionResolverStub {
	return &executionResolverStub{configs: configs}
}

func (s *executionResolverStub) HasProvider(providerID string) bool {
	if providerID == "" {
		return true
	}
	_, ok := s.configs[providerID]
	return ok
}

func (s *executionResolverStub) TurnConfigForExecution(providerID string, providerModel string) domainmodel.TurnConfig {
	config, ok := s.configs[providerID]
	if !ok {
		return domainmodel.TurnConfig{ProviderID: providerID, Model: providerModel}
	}
	if providerModel != "" {
		config.Model = providerModel
	}
	return config
}

func (s *executionResolverStub) ValidateExecutionModel(string, string) error {
	return s.validateErr
}
