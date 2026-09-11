//go:build !analytix_prod

package provider

import "strings"

func RuntimeTurnConfig(providerID, model, contractProviderBaseURL string) TurnConfig {
	rawProviderID := strings.TrimSpace(providerID)
	rawModel := strings.TrimSpace(model)
	base := strings.TrimRight(contractProviderBaseURL, "/")
	if rawProviderID == "" {
		rawProviderID = "deepseek-test-local"
	}
	lowerID := strings.ToLower(rawProviderID)
	lowerModel := strings.ToLower(rawModel)
	config := TurnConfig{
		ProviderID:                rawProviderID,
		Family:                    "deepseek",
		EndpointFormat:            "chat_completions",
		BaseURL:                   base + "/deepseek/v1",
		APIKey:                    "test-provider-key",
		Model:                     rawModel,
		ReasoningEffort:           "high",
		ReasoningProtocol:         "deepseek-chat-completions",
		CacheTelemetrySupported:   true,
		DeepSeekPrefixEnhancement: true,
		InputModalities:           []string{"text"},
		MessageParts:              []string{"text"},
	}
	switch {
	case strings.Contains(lowerID, "custom") ||
		strings.Contains(lowerID, "zai") ||
		strings.Contains(lowerID, "zhipu") ||
		strings.Contains(lowerModel, "custom"):
		config.Family = "custom_endpoint"
		config.EndpointFormat = "custom_endpoint"
		config.BaseURL = base + "/custom-endpoint"
		config.CacheTelemetrySupported = false
		config.DeepSeekPrefixEnhancement = false
		config.ReasoningEffort = ""
		config.ReasoningProtocol = ""
		if config.Model == "" {
			config.Model = "custom-compatible"
		}
	case strings.Contains(lowerID, "anthropic") ||
		strings.Contains(lowerID, "claude") ||
		strings.Contains(lowerModel, "claude"):
		config.Family = "anthropic-compatible"
		config.EndpointFormat = "messages"
		config.BaseURL = base + "/anthropic"
		config.CacheTelemetrySupported = false
		config.DeepSeekPrefixEnhancement = false
		config.ReasoningEffort = ""
		config.ReasoningProtocol = ""
		if config.Model == "" {
			config.Model = "claude-compatible"
		}
	case strings.Contains(lowerID, "openai") ||
		strings.Contains(lowerID, "gpt") ||
		strings.Contains(lowerModel, "gpt"):
		config.Family = "openai-compatible"
		config.EndpointFormat = "chat_completions"
		config.BaseURL = base + "/openai/v1"
		config.CacheTelemetrySupported = false
		config.DeepSeekPrefixEnhancement = false
		config.ReasoningEffort = ""
		config.ReasoningProtocol = ""
		if config.Model == "" {
			config.Model = "gpt-compatible"
		}
	default:
		if config.Model == "" {
			config.Model = "deepseek-chat"
		}
	}
	config.Pricing = defaultDeepSeekPricing(config.Model)
	return config
}
