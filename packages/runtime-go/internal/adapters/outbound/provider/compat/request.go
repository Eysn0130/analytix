package compat

import (
	"errors"
	"fmt"
	"strings"

	provideranthropic "analytix.local/runtime-go/internal/adapters/outbound/provider/anthropic"
	provideropenai "analytix.local/runtime-go/internal/adapters/outbound/provider/openai"
	providerresponses "analytix.local/runtime-go/internal/adapters/outbound/provider/responses"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

type PreparedHTTPRequest struct {
	RequestURL string
	Body       map[string]any
	Headers    map[string]string
}

func BuildHTTPRequest(request domainmodel.Request) (PreparedHTTPRequest, error) {
	if request.MaxOutputTokens < 0 {
		return PreparedHTTPRequest{}, errors.New("maxOutputTokens must be non-negative")
	}
	if err := domainmodel.ValidateReasoningEffortV1(request.ReasoningEffort); err != nil {
		return PreparedHTTPRequest{}, err
	}
	if strings.TrimSpace(request.BaseURL) == "" {
		return PreparedHTTPRequest{}, errors.New("baseUrl is required")
	}
	if strings.TrimSpace(request.Model) == "" {
		return PreparedHTTPRequest{}, errors.New("model is required")
	}
	format := request.EndpointFormat
	if format == "" {
		format = "chat_completions"
	}
	apiKey := strings.TrimSpace(request.APIKey)
	if apiKey == "" {
		return PreparedHTTPRequest{}, fmt.Errorf("apiKey is required for provider %s", firstNonEmpty(request.ProviderID, request.Family, "model provider"))
	}
	switch format {
	case "chat_completions":
		body, err := provideropenai.ChatCompletionsBody(request)
		if err != nil {
			return PreparedHTTPRequest{}, err
		}
		return PreparedHTTPRequest{
			RequestURL: AppendEndpointPath(request.BaseURL, "/v1/chat/completions"),
			Body:       body,
			Headers:    provideropenai.BearerStreamHeaders(apiKey),
		}, nil
	case "responses":
		return PreparedHTTPRequest{
			RequestURL: AppendEndpointPath(request.BaseURL, "/v1/responses"),
			Body:       providerresponses.Body(request),
			Headers:    provideropenai.BearerStreamHeaders(apiKey),
		}, nil
	case "messages":
		body, err := provideranthropic.MessagesBody(request)
		if err != nil {
			return PreparedHTTPRequest{}, err
		}
		return PreparedHTTPRequest{
			RequestURL: AppendEndpointPath(strings.TrimSuffix(request.BaseURL, "/v1"), "/v1/messages"),
			Body:       body,
			Headers:    provideranthropic.StreamHeaders(apiKey, false),
		}, nil
	case "custom_endpoint":
		switch CustomEndpointRequestShape(request.BaseURL) {
		case "responses":
			return PreparedHTTPRequest{
				RequestURL: request.BaseURL,
				Body:       providerresponses.Body(request),
				Headers:    provideropenai.BearerStreamHeaders(apiKey),
			}, nil
		case "messages":
			body, err := provideranthropic.MessagesBody(request)
			if err != nil {
				return PreparedHTTPRequest{}, err
			}
			return PreparedHTTPRequest{
				RequestURL: request.BaseURL,
				Body:       body,
				Headers:    provideranthropic.StreamHeaders(apiKey, true),
			}, nil
		default:
			body, err := provideropenai.ChatCompletionsBody(request)
			if err != nil {
				return PreparedHTTPRequest{}, err
			}
			return PreparedHTTPRequest{
				RequestURL: request.BaseURL,
				Body:       body,
				Headers:    provideropenai.BearerStreamHeaders(apiKey),
			}, nil
		}
	default:
		return PreparedHTTPRequest{}, fmt.Errorf("unsupported endpoint format %q", format)
	}
}

func AppendEndpointPath(baseURL string, versionedPath string) string {
	trimmed := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(trimmed, "/v1") && strings.HasPrefix(versionedPath, "/v1/") {
		return trimmed + strings.TrimPrefix(versionedPath, "/v1")
	}
	return trimmed + versionedPath
}

func CustomEndpointRequestShape(baseURL string) string {
	return domainmodel.CustomEndpointRequestShape(baseURL)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
