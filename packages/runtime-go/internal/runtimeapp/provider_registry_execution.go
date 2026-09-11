package runtimeapp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	providerregistryapp "analytix.local/runtime-go/internal/app/providerregistry"
	visionbridgeapp "analytix.local/runtime-go/internal/app/visionbridge"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	provider "analytix.local/runtime-go/internal/provider"
)

type providerRegistryExecutionResolverV1 struct {
	manager *providerregistryapp.Manager
}

// ResolveVisionExecution resolves the selected committed media model and its
// protected credential only for one bounded Vision Bridge Provider effect.
// The returned lease owns the secret-bearing config and exact currentness
// fence; callers must clear it as soon as the effect settles.
func (resolver *providerRegistryExecutionResolverV1) ResolveVisionExecution(
	ctx context.Context,
) (visionbridgeapp.ExecutionLease, error) {
	if resolver == nil || resolver.manager == nil || ctx == nil || ctx.Err() != nil {
		return visionbridgeapp.ExecutionLease{}, errors.New("provider execution authority is unavailable")
	}
	resolution, err := resolver.manager.ResolveSelectedMediaForExecution(ctx)
	if err != nil {
		return visionbridgeapp.ExecutionLease{}, errors.New("provider execution authority is unavailable")
	}
	resolvedProvider := resolution.Provider.Clone()
	resolvedProvider.SelectedModel = resolvedProvider.SelectedMedia
	resolvedProvider.Models = append([]string(nil), resolvedProvider.MediaModels...)
	result, err := resolver.resolveTurnExecutionV1(provider.TurnExecutionInput{
		RequestProviderID: resolvedProvider.ID,
		RequestModel:      resolvedProvider.SelectedModel,
	}, resolvedProvider, string(resolution.Credential))
	if err != nil {
		resolution.Clear()
		return visionbridgeapp.ExecutionLease{}, err
	}
	result.Config.SupportsImageInput = true
	result.Config.InputModalities = []string{"text", "image"}
	result.Config.MessageParts = []string{"text", "image_url"}
	authority := resolution.Authority()
	return visionbridgeapp.NewExecutionLease(
		result.Config,
		func(currentContext context.Context) error {
			if err := resolver.manager.ValidateMediaExecutionCurrent(currentContext, authority); err != nil {
				return errors.New("provider execution authority changed")
			}
			return nil
		},
		resolution.Clear,
	), nil
}

func newProviderRegistryExecutionResolverV1(
	manager *providerregistryapp.Manager,
) *providerRegistryExecutionResolverV1 {
	return &providerRegistryExecutionResolverV1{manager: manager}
}

func (resolver *providerRegistryExecutionResolverV1) ResolveTurnExecution(
	ctx context.Context,
	input provider.TurnExecutionInput,
) (provider.TurnExecutionResult, error) {
	if resolver == nil || resolver.manager == nil || ctx == nil || ctx.Err() != nil {
		return provider.TurnExecutionResult{}, errors.New("provider execution authority is unavailable")
	}
	resolution, err := resolver.manager.ResolveSelectedForExecution(ctx)
	if err != nil {
		return provider.TurnExecutionResult{}, errors.New("provider execution authority is unavailable")
	}
	defer resolution.Clear()
	result, err := resolver.resolveTurnExecutionV1(input, resolution.Provider, string(resolution.Credential))
	if err != nil {
		return provider.TurnExecutionResult{}, err
	}
	authority := resolution.Authority()
	result.Authority = provider.TurnExecutionAuthority{
		RegistryRevision: authority.RegistryRevision, RegistryIncarnation: authority.RegistryIncarnation,
		ProviderID: authority.ProviderID, ProviderRevision: authority.ProviderRevision,
		ProviderGeneration: authority.ProviderGeneration, ProviderIncarnation: authority.ProviderIncarnation,
		ProviderCredentialRef: authority.ProviderCredentialRef, ProviderCredentialPurpose: authority.ProviderCredentialPurpose,
	}
	return result, nil
}

// ResolveTurnIntent returns only key-free current Provider/model intent. The
// protected credential is resolved later inside the physical Provider effect.
func (resolver *providerRegistryExecutionResolverV1) ResolveTurnIntent(
	ctx context.Context,
	input provider.TurnExecutionInput,
) (provider.TurnExecutionResult, error) {
	if resolver == nil || resolver.manager == nil || ctx == nil || ctx.Err() != nil {
		return provider.TurnExecutionResult{}, errors.New("provider execution authority is unavailable")
	}
	resolution, err := resolver.manager.ResolveSelectedIntent(ctx)
	if err != nil {
		return provider.TurnExecutionResult{}, errors.New("provider execution authority is unavailable")
	}
	result, err := resolver.resolveTurnExecutionV1(input, resolution.Provider, "provider-registry-intent-only")
	if err != nil {
		return provider.TurnExecutionResult{}, err
	}
	result.Config.APIKey = ""
	return result, nil
}

func (resolver *providerRegistryExecutionResolverV1) ValidateTurnExecutionCurrent(
	ctx context.Context,
	authority provider.TurnExecutionAuthority,
) error {
	if resolver == nil || resolver.manager == nil || ctx == nil || ctx.Err() != nil {
		return errors.New("provider execution authority is unavailable")
	}
	err := resolver.manager.ValidateExecutionCurrent(ctx, providerregistryapp.ExecutionAuthority{
		RegistryRevision: authority.RegistryRevision, RegistryIncarnation: authority.RegistryIncarnation,
		ProviderID: authority.ProviderID, ProviderRevision: authority.ProviderRevision,
		ProviderGeneration: authority.ProviderGeneration, ProviderIncarnation: authority.ProviderIncarnation,
		ProviderCredentialRef: authority.ProviderCredentialRef, ProviderCredentialPurpose: authority.ProviderCredentialPurpose,
	})
	if err != nil {
		return errors.New("provider execution authority changed")
	}
	return nil
}

func (resolver *providerRegistryExecutionResolverV1) resolveTurnExecutionV1(
	input provider.TurnExecutionInput,
	resolvedProvider domainregistry.Provider,
	credential string,
) (provider.TurnExecutionResult, error) {

	endpointFormat, err := providerRegistryEndpointFormatV1(resolvedProvider.Kind)
	if err != nil {
		return provider.TurnExecutionResult{}, err
	}
	providerMetadata := domainmodel.ModelProviderConfig{}
	providerMetadata.ID = resolvedProvider.ID
	providerMetadata.APIKey = credential
	providerMetadata.BaseURL = resolvedProvider.Endpoint
	providerMetadata.ModelProxyURL = resolvedProvider.Proxy
	providerMetadata.EndpointFormat = endpointFormat
	providerMetadata.Models = append([]string(nil), resolvedProvider.Models...)
	configured := domainmodel.ModelProvidersConfig{
		DefaultProviderID: resolvedProvider.ID,
		Providers:         []domainmodel.ModelProviderConfig{providerMetadata},
	}
	encoded, err := json.Marshal(configured)
	if err != nil {
		return provider.TurnExecutionResult{}, errors.New("provider execution authority is invalid")
	}
	defer clear(encoded)
	config := provider.NewRuntimeProviderConfigSet(provider.RuntimeProviderConfigInput{
		DefaultProviderID:  resolvedProvider.ID,
		DefaultModel:       resolvedProvider.SelectedModel,
		ModelProvidersJSON: string(encoded),
	})
	if err := config.ConfigurationError(); err != nil {
		return provider.TurnExecutionResult{}, errors.New("provider execution authority is invalid")
	}
	input.RequestProviderID = resolvedProvider.ID
	input.ThreadProviderID = ""
	// Provider endpoint protocol is Registry route authority, not caller intent.
	// Legacy thread/subagent records may still carry the old field, but it must
	// never override the selected committed Registry winner.
	input.RequestEndpointFormat = ""
	return config.ResolveTurnExecution(input)
}

func providerRegistryEndpointFormatV1(kind string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "openai-compatible", "deepseek", "chat-completions", "chat_completions":
		return "chat_completions", nil
	case "openai-responses", "responses":
		return "responses", nil
	case "anthropic-compatible", "anthropic-messages", "messages":
		return "messages", nil
	case "custom-endpoint", "custom_endpoint":
		return "custom_endpoint", nil
	default:
		return "", errors.New("provider execution authority is invalid")
	}
}
