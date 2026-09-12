package runtimeapp

import (
	"encoding/json"
	"strings"

	providerregistryapp "analytix.local/runtime-go/internal/app/providerregistry"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	"analytix.local/runtime-go/internal/provider"
)

// Desktop already projects these key-free tariffs for usage display. This
// decoder deliberately has no credential, selection, model catalog, alias,
// or execution-capability fields. Route metadata only identifies a tariff;
// it never supplies a route to the execution resolver.
type providerRegistryPricingV1 struct {
	ID            string                          `json:"id"`
	Endpoint      string                          `json:"baseUrl"`
	Protocol      string                          `json:"endpointFormat"`
	Price         *domainmodel.Pricing            `json:"price"`
	Prices        map[string]*domainmodel.Pricing `json:"prices"`
	ModelProfiles map[string]struct {
		Price *domainmodel.Pricing `json:"price"`
	} `json:"modelProfiles"`
}

func newProviderRegistryExecutionResolverWithPricingV1(manager *providerregistryapp.Manager, metadata string) *providerRegistryExecutionResolverV1 {
	resolver := newProviderRegistryExecutionResolverV1(manager)
	var projection struct {
		Providers []providerRegistryPricingV1 `json:"providers"`
	}
	if json.Unmarshal([]byte(metadata), &projection) == nil {
		resolver.pricing = projection.Providers
	}
	var modelProjection struct {
		Providers []providerRegistryModelMetadataV1 `json:"providers"`
	}
	if json.Unmarshal([]byte(metadata), &modelProjection) == nil {
		resolver.modelMetadata = modelProjection.Providers
	}
	return resolver
}

func (resolver *providerRegistryExecutionResolverV1) applyPricingV1(selected domainregistry.Provider, result *provider.TurnExecutionResult) {
	// This runs only after Registry model admission. Tariff keys cannot enlarge
	// its catalog, canonicalize a caller alias, or select another Provider.
	if result == nil || result.ProviderID != selected.ID {
		return
	}
	bounded := false
	for _, model := range selected.Models {
		bounded = bounded || model == result.Model
	}
	if !bounded {
		return
	}
	var exact *providerRegistryPricingV1
	for index := range resolver.pricing {
		candidate := &resolver.pricing[index]
		protocol, _, err := domainmodel.ParseEndpointFormat(candidate.Protocol)
		if err != nil {
			continue
		}
		if protocol == "" {
			protocol = "chat_completions"
		}
		if strings.TrimSpace(candidate.ID) != selected.ID ||
			strings.TrimRight(strings.TrimSpace(candidate.Endpoint), "/") != result.Config.BaseURL ||
			protocol != result.Config.EndpointFormat {
			continue
		}
		if exact != nil {
			return // Ambiguous metadata cannot attribute a tariff to this route.
		}
		exact = candidate
	}
	if exact == nil {
		return
	}
	// Reuse the existing tariff resolver, including per-model precedence,
	// explicit zero prices and built-in defaults. Only the already admitted
	// exact model participates; none of its execution profile is copied.
	metadata := domainmodel.ModelProviderConfig{
		ID: selected.ID, BaseURL: result.Config.BaseURL, EndpointFormat: result.Config.EndpointFormat,
		Models: []string{result.Model}, Price: exact.Price,
		Prices: map[string]*domainmodel.Pricing{result.Model: exact.Prices[result.Model]},
		ModelProfiles: map[string]domainmodel.ModelProviderProfile{
			result.Model: {Price: exact.ModelProfiles[result.Model].Price},
		},
	}
	encoded, err := json.Marshal(domainmodel.ModelProvidersConfig{DefaultProviderID: selected.ID, Providers: []domainmodel.ModelProviderConfig{metadata}})
	if err != nil {
		return
	}
	config := provider.NewRuntimeProviderConfigSet(provider.RuntimeProviderConfigInput{ModelProvidersJSON: string(encoded)})
	if config.ConfigurationError() != nil {
		return
	}
	result.Config.Pricing = config.TurnConfig(selected.ID, result.Model).Pricing
}
