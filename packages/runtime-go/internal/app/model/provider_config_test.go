package model

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestRuntimeProviderConfigSetRejectsUnknownOrMalformedEndpointConfiguration(t *testing.T) {
	tests := []struct {
		name  string
		input domainmodel.RuntimeProviderConfigInput
	}{
		{name: "default endpoint", input: domainmodel.RuntimeProviderConfigInput{DefaultEndpointFormat: "typoo"}},
		{name: "malformed providers JSON", input: domainmodel.RuntimeProviderConfigInput{ModelProvidersJSON: `{`}},
		{name: "unknown providers JSON field", input: domainmodel.RuntimeProviderConfigInput{ModelProvidersJSON: `{"providers":[],"unexpected":true}`}},
		{name: "provider endpoint", input: domainmodel.RuntimeProviderConfigInput{ModelProvidersJSON: `{"providers":[{"id":"p","baseUrl":"https://p.example","endpointFormat":"typoo"}]}`}},
		{name: "model profile endpoint", input: domainmodel.RuntimeProviderConfigInput{ModelProvidersJSON: `{"providers":[{"id":"p","baseUrl":"https://p.example","models":["m"],"modelProfiles":{"m":{"endpointFormat":"typoo"}}}]}`}},
		{name: "model profile reasoning protocol", input: domainmodel.RuntimeProviderConfigInput{ModelProvidersJSON: `{"providers":[{"id":"p","baseUrl":"https://p.example","models":["m"],"modelProfiles":{"m":{"reasoning":{"requestProtocol":"deepseek-by-name"}}}}]}`}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			set := NewRuntimeProviderConfigSet(test.input)
			if err := set.ConfigurationError(); err == nil || !strings.Contains(err.Error(), "provider configuration error") {
				t.Fatalf("invalid provider configuration was accepted: %v", err)
			}
			if _, err := set.ResolveTurnExecution(TurnExecutionInput{}); err == nil {
				t.Fatalf("invalid provider configuration reached turn execution")
			}
		})
	}
}

func TestRuntimeProviderConfigSetAcceptsDesktopCapabilityMetadata(t *testing.T) {
	set := NewRuntimeProviderConfigSet(domainmodel.RuntimeProviderConfigInput{
		DefaultProviderID: "deepseek",
		DefaultModel:      "deepseek-v4-pro",
		ModelProvidersJSON: `{
			"defaultProviderId":"deepseek",
			"providers":[{
				"id":"deepseek",
				"baseUrl":"http://127.0.0.1:18998",
				"modelProxyUrl":"http://127.0.0.1:18999",
				"endpointFormat":"chat_completions",
				"models":["deepseek-v4-pro"],
				"modelProfiles":{
					"deepseek-v4-pro":{
						"contextWindowTokens":1000000,
						"inputModalities":["text"],
						"outputModalities":["text"],
						"supportsToolCalling":true,
						"messageParts":["text"],
						"reasoning":{
							"supportedEfforts":["off","high","max"],
							"defaultEffort":"high",
							"requestProtocol":"deepseek-chat-completions"
						}
					}
				}
			}]
		}`,
	})
	if err := set.ConfigurationError(); err != nil {
		t.Fatalf("desktop capability metadata was rejected: %v", err)
	}
	config := set.TurnConfigForExecution("deepseek", "deepseek-v4-pro")
	if config.Model != "deepseek-v4-pro" || config.ProxyURL != "http://127.0.0.1:18999" || config.ContextWindowTokens != 1000000 ||
		!hasString(config.InputModalities, "text") || !hasString(config.MessageParts, "text") {
		t.Fatalf("desktop capability metadata did not preserve the executable profile: %#v", config)
	}
}

func TestDeepSeekReasoningProtocolRequiresExactHostAuthority(t *testing.T) {
	tests := []struct {
		name             string
		providersJSON    string
		providerID       string
		model            string
		wantProtocol     string
		wantFamily       string
		wantReasoningEff bool
	}{
		{
			name:          "official default receives host protocol",
			providersJSON: `{"defaultProviderId":"deepseek","providers":[{"id":"deepseek","baseUrl":"https://api.deepseek.com/v1","apiKey":"test-key","endpointFormat":"chat_completions","models":["deepseek-chat"]}]}`,
			providerID:    "deepseek", model: "deepseek-chat", wantProtocol: "deepseek-chat-completions", wantFamily: "deepseek", wantReasoningEff: true,
		},
		{
			name:          "explicit Messages profile reaches TurnConfig",
			providersJSON: `{"providers":[{"id":"messages","baseUrl":"https://api.deepseek.com/anthropic","apiKey":"test-key","endpointFormat":"messages","models":["deepseek-flash"],"modelProfiles":{"deepseek-flash":{"reasoning":{"requestProtocol":"deepseek-messages"}}}}]}`,
			providerID:    "messages", model: "deepseek-flash", wantProtocol: "deepseek-messages", wantFamily: "anthropic-compatible", wantReasoningEff: true,
		},
		{
			name:          "provider label cannot authorize protocol",
			providersJSON: `{"providers":[{"id":"deepseek","baseUrl":"https://models.example/v1","apiKey":"test-key","endpointFormat":"chat_completions","models":["deepseek-chat"]}]}`,
			providerID:    "deepseek", model: "deepseek-chat", wantFamily: "deepseek",
		},
		{
			name:          "url substring cannot authorize protocol",
			providersJSON: `{"providers":[{"id":"proxy","baseUrl":"https://deepseek.example/v1","apiKey":"test-key","endpointFormat":"chat_completions","models":["model"]}]}`,
			providerID:    "proxy", model: "model", wantFamily: "deepseek",
		},
		{
			name:          "model substring cannot authorize protocol",
			providersJSON: `{"providers":[{"id":"proxy","baseUrl":"https://models.example/v1","apiKey":"test-key","endpointFormat":"chat_completions","models":["deepseek-compatible"]}]}`,
			providerID:    "proxy", model: "deepseek-compatible", wantFamily: "deepseek",
		},
		{
			name:          "explicit profile authorizes neutral proxy",
			providersJSON: `{"providers":[{"id":"proxy","baseUrl":"https://models.example/v1","apiKey":"test-key","endpointFormat":"chat_completions","models":["model"],"modelProfiles":{"model":{"reasoning":{"requestProtocol":"deepseek-chat-completions"}}}}]}`,
			providerID:    "proxy", model: "model", wantProtocol: "deepseek-chat-completions", wantFamily: "openai-compatible", wantReasoningEff: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			set := NewRuntimeProviderConfigSet(domainmodel.RuntimeProviderConfigInput{ModelProvidersJSON: test.providersJSON})
			if err := set.ConfigurationError(); err != nil {
				t.Fatalf("valid provider configuration was rejected: %v", err)
			}
			config := set.TurnConfig(test.providerID, test.model)
			if config.ReasoningProtocol != test.wantProtocol || config.Family != test.wantFamily {
				t.Fatalf("reasoning authority mismatch: %#v", config)
			}
			if got := SupportsReasoningEffort(config); got != test.wantReasoningEff {
				t.Fatalf("reasoning effort authority mismatch: got %v want %v", got, test.wantReasoningEff)
			}
		})
	}
}

func TestRuntimeProviderConfigSetRejectsUnknownTurnEndpointOverride(t *testing.T) {
	set := NewRuntimeProviderConfigSet(domainmodel.RuntimeProviderConfigInput{
		DefaultBaseURL: "https://api.deepseek.com", DefaultAPIKey: "test-key", DefaultModel: "deepseek-chat",
	})
	if err := set.ConfigurationError(); err != nil {
		t.Fatalf("valid provider configuration was rejected: %v", err)
	}
	if _, err := set.ResolveTurnExecution(TurnExecutionInput{RequestEndpointFormat: "typoo"}); err == nil {
		t.Fatalf("unknown turn endpoint override was accepted")
	}
}

func TestRuntimeProviderConfigSetRejectsBoundedProviderModelMismatch(t *testing.T) {
	set := NewRuntimeProviderConfigSet(domainmodel.RuntimeProviderConfigInput{
		ModelProvidersJSON: `{
			"defaultProviderId": "deepseek",
			"providers": [{
				"id": "deepseek",
				"apiKey": "sk-test",
				"baseUrl": "https://api.deepseek.com/v1",
				"endpointFormat": "chat_completions",
				"models": ["deepseek-v4-pro"]
			}]
		}`,
	})

	if err := set.ValidateExecutionModel("deepseek", "deepseek-v4-pro"); err != nil {
		t.Fatalf("configured model should validate: %v", err)
	}
	err := set.ValidateExecutionModel("deepseek", "mimo-v2-pro")
	if err == nil {
		t.Fatal("expected invalid model error")
	}
	var modelErr *ExecutionModelError
	if !errors.As(err, &modelErr) {
		t.Fatalf("expected ExecutionModelError, got %T: %v", err, err)
	}
	if modelErr.Kind != "invalid_model" || modelErr.ProviderID != "deepseek" || modelErr.Family != "deepseek" {
		t.Fatalf("unexpected provider error: %#v", modelErr)
	}
}

func TestRuntimeProviderConfigSetPreservesOpenCatalogExecutionOverride(t *testing.T) {
	set := NewRuntimeProviderConfigSet(domainmodel.RuntimeProviderConfigInput{
		ModelProvidersJSON: `{
			"defaultProviderId": "custom-open",
			"providers": [{
				"id": "custom-open",
				"apiKey": "sk-test",
				"baseUrl": "https://models.example/v1",
				"endpointFormat": "chat_completions"
			}]
		}`,
	})

	if err := set.ValidateExecutionModel("custom-open", "child-special"); err != nil {
		t.Fatalf("open provider catalogs must allow explicit live models: %v", err)
	}
	config := set.TurnConfigForExecution("custom-open", "child-special")
	if config.ProviderID != "custom-open" || config.Model != "child-special" || config.Family != "openai-compatible" {
		t.Fatalf("open execution config should preserve explicit live model: %#v", config)
	}
}

func TestRuntimeProviderConfigSetResolvesTurnExecutionFromThreadDefaults(t *testing.T) {
	set := NewRuntimeProviderConfigSet(domainmodel.RuntimeProviderConfigInput{
		ModelProvidersJSON: `{
			"defaultProviderId": "custom-open",
			"providers": [{
				"id": "custom-open",
				"apiKey": "sk-test",
				"baseUrl": "https://models.example/v1",
				"endpointFormat": "chat_completions",
				"models": ["thread-model"]
			}]
		}`,
	})

	resolved, err := set.ResolveTurnExecution(TurnExecutionInput{
		ThreadProviderID: "custom-open",
		ThreadModel:      "thread-model",
	})
	if err != nil {
		t.Fatalf("resolve thread execution: %v", err)
	}
	if resolved.ProviderID != "custom-open" || resolved.Model != "thread-model" || resolved.Config.Model != "thread-model" {
		t.Fatalf("unexpected resolved execution: %#v", resolved)
	}
	if resolved.ThreadModel != "thread-model" {
		t.Fatalf("thread model not recorded: %#v", resolved)
	}
}

func TestRuntimeProviderConfigSetUsesConfiguredDefaultModelInsteadOfFirstProviderModel(t *testing.T) {
	set := NewRuntimeProviderConfigSet(domainmodel.RuntimeProviderConfigInput{
		DefaultModel: "selected-model",
		ModelProvidersJSON: `{
			"defaultProviderId": "custom-open",
			"providers": [{
				"id": "custom-open",
				"apiKey": "sk-test",
				"baseUrl": "https://models.example/v1",
				"endpointFormat": "chat_completions",
				"models": ["first-model", "selected-model"],
				"modelProfiles": {
					"first-model": {"contextWindowTokens": 100000},
					"selected-model": {"contextWindowTokens": 200000}
				}
			}]
		}`,
	})

	config := set.TurnConfig("", "")
	if config.Model != "selected-model" || config.ContextWindowTokens != 200000 {
		t.Fatalf("default turn config should use configured selected model: %#v", config)
	}
	resolved, err := set.ResolveTurnExecution(TurnExecutionInput{})
	if err != nil {
		t.Fatalf("resolve default execution: %v", err)
	}
	if resolved.Model != "selected-model" || resolved.Config.ContextWindowTokens != 200000 {
		t.Fatalf("default execution should use configured selected model: %#v", resolved)
	}
}

func TestRuntimeProviderConfigSetPrefersFlashForFreshDeepSeekCatalog(t *testing.T) {
	set := NewRuntimeProviderConfigSet(domainmodel.RuntimeProviderConfigInput{
		ModelProvidersJSON: `{
			"defaultProviderId": "deepseek",
			"providers": [{
				"id": "deepseek",
				"apiKey": "sk-test",
				"baseUrl": "https://api.deepseek.com/v1",
				"endpointFormat": "chat_completions",
				"models": ["deepseek-v4-pro", "deepseek-v4-flash"]
			}]
		}`,
	})

	config := set.TurnConfig("", "")
	if config.Model != "deepseek-v4-flash" {
		t.Fatalf("fresh DeepSeek catalog should prefer v4 flash: %#v", config)
	}
}

func TestRuntimeProviderConfigSetResolveTurnExecutionPreservesExplicitOpenModel(t *testing.T) {
	set := NewRuntimeProviderConfigSet(domainmodel.RuntimeProviderConfigInput{
		ModelProvidersJSON: `{
			"defaultProviderId": "custom-open",
			"providers": [{
				"id": "custom-open",
				"apiKey": "sk-test",
				"baseUrl": "https://models.example/v1",
				"endpointFormat": "chat_completions"
			}]
		}`,
	})

	resolved, err := set.ResolveTurnExecution(TurnExecutionInput{
		RequestProviderID:     "custom-open",
		RequestModel:          "child-special",
		RequestEndpointFormat: "messages",
	})
	if err != nil {
		t.Fatalf("resolve explicit execution: %v", err)
	}
	if resolved.Model != "child-special" || resolved.Config.Model != "child-special" {
		t.Fatalf("explicit open model was not preserved: %#v", resolved)
	}
	if resolved.Config.EndpointFormat != "messages" || resolved.Config.Family != "anthropic-compatible" {
		t.Fatalf("endpoint override was not applied: %#v", resolved.Config)
	}
}

func TestRuntimeProviderConfigSetInfersVisionForUnprofiledLikelyMultimodalModels(t *testing.T) {
	set := NewRuntimeProviderConfigSet(domainmodel.RuntimeProviderConfigInput{
		ModelProvidersJSON: `{
			"defaultProviderId": "custom-open",
			"providers": [{
				"id": "custom-open",
				"apiKey": "sk-test",
				"baseUrl": "https://models.example/v1",
				"endpointFormat": "chat_completions",
				"models": ["gpt-4o-mini", "custom-vision-model", "gpt-4o-audio-preview"]
			}, {
				"id": "responses",
				"apiKey": "sk-test",
				"baseUrl": "https://models.example/v1",
				"endpointFormat": "responses",
				"models": ["custom-omni-model"]
			}, {
				"id": "profiled",
				"apiKey": "sk-test",
				"baseUrl": "https://models.example/v1",
				"endpointFormat": "chat_completions",
				"models": ["gpt-4o-mini"],
				"modelProfiles": {
					"gpt-4o-mini": {
						"inputModalities": ["text"],
						"messageParts": ["text"]
					}
				}
			}]
		}`,
	})

	chatConfig := set.TurnConfig("custom-open", "gpt-4o-mini")
	if !chatConfig.SupportsImageInput ||
		!hasString(chatConfig.InputModalities, "image") ||
		!hasString(chatConfig.MessageParts, "image_url") {
		t.Fatalf("expected chat-completions vision defaults: %#v", chatConfig)
	}

	visionConfig := set.TurnConfig("custom-open", "custom-vision-model")
	if !visionConfig.SupportsImageInput || !hasString(visionConfig.MessageParts, "image_url") {
		t.Fatalf("expected token-based vision default: %#v", visionConfig)
	}

	responsesConfig := set.TurnConfig("responses", "custom-omni-model")
	if !responsesConfig.SupportsImageInput || !hasString(responsesConfig.MessageParts, "input_image") {
		t.Fatalf("expected responses vision defaults: %#v", responsesConfig)
	}

	audioConfig := set.TurnConfig("custom-open", "gpt-4o-audio-preview")
	if audioConfig.SupportsImageInput || hasString(audioConfig.InputModalities, "image") {
		t.Fatalf("audio model should not be inferred as vision: %#v", audioConfig)
	}

	profiledConfig := set.TurnConfig("profiled", "gpt-4o-mini")
	if profiledConfig.SupportsImageInput || !hasString(profiledConfig.MessageParts, "text") || hasString(profiledConfig.MessageParts, "image_url") {
		t.Fatalf("explicit text-only profile should win over heuristic: %#v", profiledConfig)
	}
}

func TestRuntimeProviderConfigSetResolveTurnExecutionAppliesSupportedEffortOnly(t *testing.T) {
	set := NewRuntimeProviderConfigSet(domainmodel.RuntimeProviderConfigInput{
		ModelProvidersJSON: `{
			"defaultProviderId": "xiaomi",
			"providers": [{
				"id": "xiaomi",
				"apiKey": "sk-test",
				"baseUrl": "https://api.xiaomimimo.com/v1",
				"endpointFormat": "chat_completions",
				"models": ["mimo-v2.5-pro"],
				"modelProfiles": {
					"mimo-v2.5-pro": {
						"reasoning": {
							"requestProtocol": "mimo-chat-completions"
						}
					}
				}
			}, {
				"id": "plain",
				"apiKey": "sk-test",
				"baseUrl": "https://plain.example/v1",
				"endpointFormat": "chat_completions",
				"models": ["plain-model"]
			}]
		}`,
	})

	mimo, err := set.ResolveTurnExecution(TurnExecutionInput{
		RequestProviderID: "xiaomi",
		RequestModel:      "mimo-v2.5-pro",
		RequestEffort:     "low",
	})
	if err != nil {
		t.Fatalf("resolve mimo execution: %v", err)
	}
	if mimo.Effort != "low" || mimo.Config.ReasoningEffort != "low" {
		t.Fatalf("supported effort not applied: %#v", mimo)
	}

	plain, err := set.ResolveTurnExecution(TurnExecutionInput{
		RequestProviderID: "plain",
		RequestModel:      "plain-model",
		RequestEffort:     "low",
	})
	if err != nil {
		t.Fatalf("resolve plain execution: %v", err)
	}
	if plain.Effort != "" || plain.Config.ReasoningEffort != "" {
		t.Fatalf("unsupported effort should be ignored: %#v", plain)
	}
}

func TestRuntimeProviderConfigSetResolvesProfileAliasWithoutDeepSeekDefaults(t *testing.T) {
	set := NewRuntimeProviderConfigSet(domainmodel.RuntimeProviderConfigInput{
		ModelProvidersJSON: `{
			"defaultProviderId": "xiaomi",
			"providers": [{
				"id": "xiaomi",
				"apiKey": "sk-test",
				"baseUrl": "https://api.xiaomimimo.com/v1",
				"endpointFormat": "chat_completions",
				"models": ["mimo-v2.5-pro"],
				"modelProfiles": {
					"mimo-v2.5-pro": {
						"aliases": ["mimo-v2.5-pro-ultraspeed"],
						"inputModalities": ["text"],
						"messageParts": ["text"],
						"reasoning": {
							"supportedEfforts": ["off", "low", "medium", "high"],
							"defaultEffort": "high",
							"requestProtocol": "mimo-chat-completions"
						},
						"price": {"input": 0.15, "output": 0.6, "currency": "CNY"}
					}
				}
			}]
		}`,
	})

	config := set.TurnConfig("xiaomi", "mimo-v2.5-pro-ultraspeed")
	if config.ProviderID != "xiaomi" ||
		config.Model != "mimo-v2.5-pro" ||
		config.Family != "openai-compatible" ||
		config.ReasoningProtocol != "mimo-chat-completions" ||
		!reflect.DeepEqual(config.ReasoningSupportedEfforts, []string{"off", "low", "medium", "high"}) ||
		config.ReasoningDefaultEffort != "high" ||
		config.ReasoningEffort != "high" {
		t.Fatalf("provider config should canonicalize MiMo alias: %#v", config)
	}
	if config.DeepSeekPrefixEnhancement || config.CacheTelemetrySupported {
		t.Fatalf("MiMo alias should not inherit DeepSeek behavior: %#v", config)
	}
	if config.Pricing == nil || config.Pricing.Output != 0.6 || config.Pricing.Currency != "CNY" {
		t.Fatalf("MiMo alias should use canonical profile pricing: %#v", config.Pricing)
	}
}

func TestRuntimeProviderConfigSetRejectsInvalidOrUnsupportedReasoningEffortExactly(t *testing.T) {
	set := NewRuntimeProviderConfigSet(domainmodel.RuntimeProviderConfigInput{
		ModelProvidersJSON: `{
			"defaultProviderId":"p",
			"providers":[{
				"id":"p","apiKey":"sk-test","baseUrl":"https://p.example/v1","models":["m"],
				"modelProfiles":{"m":{"reasoning":{"requestProtocol":"openai-responses","supportedEfforts":["low"]}}}
			}]
		}`,
	})
	for _, effort := range []string{" ", " high ", "HIGH", "adaptive", "minimal", "xhigh", "SOL_PRIVATE_REASONING_SENTINEL_7F3C"} {
		_, err := set.ResolveTurnExecution(TurnExecutionInput{RequestProviderID: "p", RequestModel: "m", RequestEffort: effort})
		if !errors.Is(err, domainmodel.ErrInvalidReasoningEffort) ||
			(effort == "SOL_PRIVATE_REASONING_SENTINEL_7F3C" && strings.Contains(err.Error(), effort)) {
			t.Fatalf("invalid effort must fail with the static sentinel without reflection: effort=%q err=%v", effort, err)
		}
	}
	if _, err := set.ResolveTurnExecution(TurnExecutionInput{RequestProviderID: "p", RequestModel: "m", RequestEffort: "high"}); !errors.Is(err, ErrUnsupportedReasoningEffort) {
		t.Fatalf("legal but unsupported effort must fail closed: %v", err)
	}
	result, err := set.ResolveTurnExecution(TurnExecutionInput{RequestProviderID: "p", RequestModel: "m", RequestEffort: "low"})
	if err != nil || result.Effort != "low" {
		t.Fatalf("supported exact effort should pass: result=%#v err=%v", result, err)
	}
}

func TestRuntimeProviderConfigSetRejectsMalformedReasoningEffortConfiguration(t *testing.T) {
	tests := []string{
		`{"providers":[{"id":"p","apiKey":"k","baseUrl":"https://p.example","models":["m"],"modelProfiles":{"m":{"reasoning":{"supportedEfforts":[" high "]}}}}]}`,
		`{"providers":[{"id":"p","apiKey":"k","baseUrl":"https://p.example","models":["m"],"modelProfiles":{"m":{"reasoning":{"supportedEfforts":["high","high"]}}}}]}`,
		`{"providers":[{"id":"p","apiKey":"k","baseUrl":"https://p.example","models":["m"],"modelProfiles":{"m":{"reasoning":{"supportedEfforts":["low"],"defaultEffort":"high"}}}}]}`,
		`{"providers":[{"id":"p","apiKey":"k","baseUrl":"https://p.example","models":["m"],"modelProfiles":{"m":{"reasoning":{"defaultEffort":"HIGH"}}}}]}`,
	}
	for _, raw := range tests {
		set := NewRuntimeProviderConfigSet(domainmodel.RuntimeProviderConfigInput{ModelProvidersJSON: raw})
		if set.ConfigurationError() == nil {
			t.Fatalf("malformed reasoning effort configuration must fail closed: %s", raw)
		}
	}
}
