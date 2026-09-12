package runtimeapp

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	"analytix.local/runtime-go/internal/provider"
)

func TestProviderRegistryPricingUsesExactCommittedRouteAndExistingPrecedence(t *testing.T) {
	selected := domainregistry.Provider{ID: "priced", Kind: "openai-compatible", Endpoint: "https://priced.invalid/v1", Models: []string{"model"}, SelectedModel: "model"}
	price := func(input float64) *domainmodel.Pricing {
		return &domainmodel.Pricing{Input: input, Output: input, Currency: "USD"}
	}
	for _, mode := range []string{"provider", "profile", "per-model", "explicit-zero", "missing", "foreign-id", "foreign-endpoint", "foreign-protocol", "ambiguous", "other-model"} {
		t.Run(mode, func(t *testing.T) {
			declaration := map[string]any{"id": selected.ID, "baseUrl": selected.Endpoint, "endpointFormat": "chat_completions", "price": price(1)}
			want := price(1)
			switch mode {
			case "profile", "per-model", "explicit-zero":
				declaration["modelProfiles"] = map[string]any{"model": map[string]any{"price": price(2)}}
				want = price(2)
				if mode != "profile" {
					want = price(3)
					if mode == "explicit-zero" {
						want = price(0)
					}
					declaration["prices"] = map[string]any{"model": want}
				}
			case "missing":
				delete(declaration, "price")
				want = nil
			case "foreign-id":
				declaration["id"], want = "other", nil
			case "foreign-endpoint":
				declaration["baseUrl"], want = "https://other.invalid/v1", nil
			case "foreign-protocol":
				declaration["endpointFormat"], want = "messages", nil
			case "ambiguous":
				want = nil
			case "other-model":
				delete(declaration, "price")
				declaration["prices"] = map[string]any{"other": price(9)}
				declaration["modelProfiles"] = map[string]any{"other": map[string]any{"price": price(8), "aliases": []string{"model"}}}
				want = nil
			}
			declarations := []any{declaration}
			if mode == "ambiguous" {
				declarations = append(declarations, declaration)
			}
			body, err := json.Marshal(map[string]any{"providers": declarations})
			if err != nil {
				t.Fatal(err)
			}
			resolver := newProviderRegistryExecutionResolverWithPricingV1(nil, string(body))
			result, err := resolver.resolveTurnExecutionV1(provider.TurnExecutionInput{RequestModel: "model"}, selected, "synthetic-committed")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(result.Config.Pricing, want) {
				t.Fatalf("tariff attribution or precedence differs: got=%#v want=%#v", result.Config.Pricing, want)
			}
			usage := provider.ApplyPricing(provider.Usage{PromptTokens: 100, CompletionTokens: 10}, result.Config.Pricing)
			if usage.PriceConfigured != (want != nil) {
				t.Fatal("missing and explicit-zero tariff states collapsed")
			}
		})
	}
}

func TestProviderRegistryPricingCannotSupplyExecutionAuthority(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	writeProviderRegistrySyntheticFallbackMasterKeyV1(t, dataDir, 'p')
	authority, err := openProviderRegistryAuthorityV1(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Close()
	selected := connectProviderRegistryTestWinnerV1(t, ctx, authority.Manager(), "registry-priced", "synthetic-registry-credential")
	body, err := json.Marshal(map[string]any{
		"defaultProviderId": "legacy-selected",
		"providers": []any{map[string]any{
			"id": selected.ID, "baseUrl": selected.Endpoint, "endpointFormat": "chat_completions",
			"apiKey": "synthetic-legacy-credential", "modelProxyUrl": "https://legacy-proxy.invalid",
			"models": []string{"legacy-model"}, "selectedModel": "legacy-model", "price": map[string]any{"input": 2, "output": 3, "currency": "USD"},
			"modelProfiles": map[string]any{
				"model-k4": map[string]any{"endpointFormat": "messages", "inputModalities": []string{"image"}, "contextWindowTokens": 999},
				// Only a Registry-admitted canonical model may own aliases or
				// reasoning. This external profile must supply neither.
				"legacy-model": map[string]any{"aliases": []string{"legacy-alias"}, "reasoning": map[string]any{"requestProtocol": "openai-responses"}},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	resolver := newProviderRegistryExecutionResolverWithPricingV1(authority.Manager(), string(body))
	baseline, err := newProviderRegistryExecutionResolverV1(authority.Manager()).ResolveTurnExecution(ctx, provider.TurnExecutionInput{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := resolver.ResolveTurnExecution(ctx, provider.TurnExecutionInput{RequestProviderID: "legacy-selected", RequestEndpointFormat: "messages", RequestModel: "model-k4"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Config.Pricing == nil || result.Config.Pricing.Input != 2 {
		t.Fatal("exact selected route lost its key-free tariff")
	}
	result.Config.Pricing = baseline.Config.Pricing
	if !reflect.DeepEqual(result, baseline) {
		t.Fatal("pricing metadata changed committed execution configuration or authority")
	}
	for _, model := range []string{"legacy-alias", "legacy-model"} {
		if _, err := resolver.ResolveTurnExecution(ctx, provider.TurnExecutionInput{RequestModel: model}); err == nil {
			t.Fatal("pricing metadata expanded the committed model catalog")
		}
	}
	if err := authority.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.ResolveTurnExecution(ctx, provider.TurnExecutionInput{}); err == nil {
		t.Fatal("tariff metadata supplied authority after the Registry closed")
	}
}
