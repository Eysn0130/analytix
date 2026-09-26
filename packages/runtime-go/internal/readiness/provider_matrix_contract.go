//go:build !analytix_prod

package readiness

import (
	"context"
	"net/http"
	"strings"

	provider "analytix.local/runtime-go/internal/provider"
	providerscript "analytix.local/runtime-go/internal/testsupport/providerscript"
)

func RunProviderReadinessMatrix(ctx context.Context, env map[string]string, httpClient *http.Client) (RuntimeProviderMatrixResult, error) {
	if env == nil {
		env = environMap()
	}
	fixture := providerscript.NewScriptedProviderServer()
	defer fixture.Close()
	if httpClient == nil {
		httpClient = fixture.Client()
	}
	fixtureCases := runtimeProviderCases(fixture.URL)
	fixtureClient := provider.NewHTTPProviderClient(fixture.Client())
	fixtureProbes := make([]RuntimeReadinessProbe, 0, len(fixtureCases))
	for _, item := range fixtureCases {
		result, err := fixtureClient.Stream(ctx, providerRequestForCase(item, "test-provider-key"))
		if err != nil {
			return RuntimeProviderMatrixResult{}, err
		}
		fixtureProbes = append(fixtureProbes, providerProbeFromResult(item, result, false, "passed", "fixture provider matrix probe"))
	}

	credentialedClient := provider.NewHTTPProviderClient(httpClient)
	credentialed := make([]RuntimeReadinessProbe, 0, len(fixtureCases))
	for _, item := range runtimeProviderCases("") {
		apiKey := strings.TrimSpace(env[item.apiKeyEnv])
		baseURL := strings.TrimSpace(env[item.baseURLEnv])
		model := strings.TrimSpace(env[item.modelEnv])
		if apiKey == "" || baseURL == "" || model == "" {
			credentialed = append(credentialed, RuntimeReadinessProbe{
				ID:             item.id,
				Family:         item.family,
				EndpointFormat: item.endpointFormat,
				Status:         "skipped",
				Skipped:        true,
				Credentialed:   true,
				Message:        "missing env-gated credential/baseUrl/model; skipped without counting as pass",
			})
			continue
		}
		item.baseURL = baseURL
		item.model = model
		result, err := credentialedClient.Stream(ctx, providerRequestForCase(item, apiKey))
		if err != nil {
			credentialed = append(credentialed, RuntimeReadinessProbe{
				ID:             item.id,
				Family:         item.family,
				EndpointFormat: item.endpointFormat,
				Status:         "failed",
				Credentialed:   true,
				Message:        err.Error(),
			})
			continue
		}
		credentialed = append(credentialed, providerProbeFromResult(item, result, true, "passed", "credentialed provider probe passed"))
	}

	deepseek := fixtureProbes[0]
	return RuntimeProviderMatrixResult{
		SchemaVersion:              1,
		ProviderMatrixScaffold:     true,
		FixtureMatrixRequired:      true,
		CredentialedMatrixEnvGated: true,
		ReadsRealAPIKeysByDefault:  false,
		DeepSeekCacheBenchmarkFormat: map[string]any{
			"schemaVersion":       1,
			"provider":            deepseek.Family,
			"endpointFormat":      deepseek.EndpointFormat,
			"prefixHash":          deepseek.PrefixShape.PrefixHash,
			"systemHash":          deepseek.PrefixShape.SystemHash,
			"toolsHash":           deepseek.PrefixShape.ToolsHash,
			"cacheHitTokens":      deepseek.Usage.CacheHitTokens,
			"cacheMissTokens":     deepseek.Usage.CacheMissTokens,
			"cacheHitRate":        deepseek.Usage.CacheHitRate,
			"dynamicStateCheck":   deepseek.PrefixShape.DynamicStateCheck,
			"recordedAtFieldName": "recordedAt",
		},
		FixtureProbes:      fixtureProbes,
		CredentialedProbes: credentialed,
	}, nil
}

func providerRequestForCase(item runtimeProviderCase, apiKey string) provider.Request {
	return provider.Request{
		ProviderID:        item.id + "-runtime-readiness",
		Family:            item.family,
		EndpointFormat:    item.endpointFormat,
		ReasoningProtocol: item.reasoningProtocol,
		BaseURL:           item.baseURL,
		Model:             item.model,
		APIKey:            apiKey,
		ReasoningEffort:   "high",
		SystemPrompt:      "You are analytix.",
		Messages: []provider.Message{
			{Role: "system", Content: "You are analytix."},
			{Role: "user", Content: "Run runtime provider matrix contract probe."},
		},
		Tools: providerscript.DefaultProductionCandidateTools(),
	}
}

func providerProbeFromResult(item runtimeProviderCase, result provider.Result, credentialed bool, status string, message string) RuntimeReadinessProbe {
	return RuntimeReadinessProbe{
		ID:                item.id,
		Family:            result.Family,
		EndpointFormat:    result.EndpointFormat,
		Status:            status,
		Credentialed:      credentialed,
		RequestURL:        result.RequestURL,
		RequestBodyFields: result.RequestBodyFields,
		Usage:             result.Usage,
		PrefixShape:       result.PrefixShape,
		Message:           message,
	}
}
