package runtimeapp

import (
	"encoding/json"
	"reflect"
	"testing"

	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	"analytix.local/runtime-go/internal/provider"
)

func TestProviderRegistryBoundedModelMetadataPreservesReasoningAndAlias(t *testing.T) {
	selected := domainregistry.Provider{ID: "selected", Kind: "openai-compatible", Endpoint: "https://selected.invalid/v1", Proxy: "http://committed-proxy.invalid", Models: []string{"canonical"}, SelectedModel: "canonical"}
	metadata := map[string]any{"id": selected.ID, "baseUrl": selected.Endpoint, "endpointFormat": "chat_completions", "apiKey": "synthetic-stale", "modelProxyUrl": "http://stale.invalid", "models": []string{"external"},
		"modelProfiles": map[string]any{"canonical": map[string]any{
			"aliases": []string{"display-alias"}, "endpointFormat": "messages", "inputModalities": []string{"image"}, "contextWindowTokens": 999,
			"reasoning": map[string]any{"requestProtocol": "deepseek-chat-completions", "supportedEfforts": []string{"off", "high", "max"}, "defaultEffort": "high"},
		}}}
	for _, requested := range []string{"canonical", "display-alias"} {
		t.Run(requested, func(t *testing.T) {
			resolver := registryModelMetadataResolverForTestV1(t, metadata)
			result, err := resolver.resolveTurnExecutionV1(provider.TurnExecutionInput{RequestProviderID: "external", RequestModel: requested, RequestEndpointFormat: "responses"}, selected, "synthetic-committed")
			if err != nil {
				t.Fatal(err)
			}
			if result.ProviderID != selected.ID || result.Model != "canonical" || result.Config.BaseURL != selected.Endpoint || result.Config.ProxyURL != selected.Proxy || result.Config.APIKey != "synthetic-committed" || result.Config.EndpointFormat != "chat_completions" {
				t.Fatal("model metadata changed committed Provider authority")
			}
			if result.Config.ReasoningProtocol != "deepseek-chat-completions" || result.Effort != "high" || !reflect.DeepEqual(result.Config.ReasoningSupportedEfforts, []string{"off", "high", "max"}) {
				t.Fatal("selected canonical model lost its reasoning profile")
			}
			if result.Config.SupportsImageInput || result.Config.ContextWindowTokens != 0 {
				t.Fatal("unrelated model capabilities escaped the bounded projection")
			}
			if _, err := resolver.resolveTurnExecutionV1(provider.TurnExecutionInput{RequestModel: requested, RequestEffort: "low"}, selected, "synthetic-committed"); err == nil {
				t.Fatal("unsupported profile effort was accepted")
			}
		})
	}
}

func TestProviderRegistryBoundedModelMetadataRejectsExpansionAndAmbiguity(t *testing.T) {
	for _, mode := range []string{"unregistered", "foreign-route", "foreign-provider", "foreign-protocol", "alias-conflict", "canonical-conflict", "duplicate-route", "invalid-reasoning"} {
		t.Run(mode, func(t *testing.T) {
			selected := domainregistry.Provider{ID: "selected", Kind: "openai-compatible", Endpoint: "https://selected.invalid/v1", Models: []string{"canonical", "second"}, SelectedModel: "canonical"}
			profiles := map[string]any{"canonical": map[string]any{"aliases": []string{"display-alias"}}}
			metadata := map[string]any{"id": selected.ID, "baseUrl": selected.Endpoint, "endpointFormat": "chat_completions", "modelProfiles": profiles}
			request := "display-alias"
			switch mode {
			case "unregistered":
				delete(profiles, "canonical")
				profiles["external"] = map[string]any{"aliases": []string{"display-alias"}}
			case "foreign-route":
				metadata["baseUrl"] = "https://external.invalid/v1"
			case "foreign-provider":
				metadata["id"] = "external"
			case "foreign-protocol":
				metadata["endpointFormat"] = "messages"
			case "alias-conflict":
				profiles["second"] = map[string]any{"aliases": []string{"DISPLAY-ALIAS"}}
			case "canonical-conflict":
				profiles["second"] = map[string]any{"aliases": []string{"canonical"}}
				request = "canonical"
			case "invalid-reasoning":
				profiles["canonical"] = map[string]any{"reasoning": map[string]any{"requestProtocol": "unknown-protocol"}}
				request = "canonical"
			}
			declarations := []map[string]any{metadata}
			if mode == "duplicate-route" {
				declarations = append(declarations, metadata)
				request = "canonical"
			}
			resolver := registryModelMetadataResolverForTestV1(t, declarations...)
			if _, err := resolver.resolveTurnExecutionV1(provider.TurnExecutionInput{RequestModel: request}, selected, "synthetic-committed"); err == nil {
				t.Fatal("untrusted or ambiguous model metadata was admitted")
			}
			if mode == "unregistered" {
				if _, err := resolver.resolveTurnExecutionV1(provider.TurnExecutionInput{RequestModel: "external"}, selected, "synthetic-committed"); err == nil {
					t.Fatal("external profile key expanded the Registry model catalog")
				}
			}
		})
	}
}

func registryModelMetadataResolverForTestV1(t *testing.T, declarations ...map[string]any) *providerRegistryExecutionResolverV1 {
	t.Helper()
	body, err := json.Marshal(map[string]any{"defaultProviderId": "external", "providers": declarations})
	if err != nil {
		t.Fatal(err)
	}
	return newProviderRegistryExecutionResolverWithPricingV1(nil, string(body))
}
