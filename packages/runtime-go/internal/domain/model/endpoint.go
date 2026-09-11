package model

import (
	"errors"
	"strings"
)

var ErrInvalidEndpointFormat = errors.New("model endpoint format is invalid")

// ParseEndpointFormat separates an omitted value, which an owning boundary
// may default, from an unknown supplied value, which must fail closed.
func ParseEndpointFormat(value string) (format string, supplied bool, err error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", false, nil
	}
	normalized := strings.Trim(strings.ToLower(trimmed), "/")
	normalized = strings.ReplaceAll(normalized, "-", "_")
	switch normalized {
	case "chat", "chat_completions", "v1/chat/completions", "chat/completions":
		return "chat_completions", true, nil
	case "response", "responses", "v1/responses":
		return "responses", true, nil
	case "message", "messages", "anthropic_message", "anthropic_messages", "anthropic/messages", "v1/messages":
		return "messages", true, nil
	case "custom", "custom_endpoint", "custom_full_path", "full_path", "full_url":
		return "custom_endpoint", true, nil
	default:
		return "", true, ErrInvalidEndpointFormat
	}
}

func NormalizeEndpointFormat(value string) string {
	format, supplied, err := ParseEndpointFormat(value)
	if err != nil {
		return ""
	}
	if !supplied {
		return "chat_completions"
	}
	return format
}

func OptionalEndpointFormat(value string) string {
	format, supplied, err := ParseEndpointFormat(value)
	if err != nil || !supplied {
		return ""
	}
	return format
}

func ProviderFamily(providerID string, baseURL string, model string, endpointFormat string) string {
	joined := strings.ToLower(providerID + " " + baseURL + " " + model)
	switch {
	case NormalizeEndpointFormat(endpointFormat) == "custom_endpoint":
		return "custom_endpoint"
	case NormalizeEndpointFormat(endpointFormat) == "messages" || strings.Contains(joined, "anthropic") || strings.Contains(joined, "claude"):
		return "anthropic-compatible"
	case strings.Contains(joined, "deepseek"):
		return "deepseek"
	default:
		return "openai-compatible"
	}
}
