package compat

import (
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestBuildHTTPRequestRejectsInvalidReasoningEffortForEveryEndpointShape(t *testing.T) {
	for _, request := range []domainmodel.Request{
		baseRequest("chat_completions", "https://api.example/v1", nil),
		baseRequest("responses", "https://api.example/v1", nil),
		baseRequest("messages", "https://api.anthropic.com/v1", nil),
		baseRequest("custom_endpoint", "https://provider.example/chat/completions", nil),
		baseRequest("custom_endpoint", "https://provider.example/responses", nil),
		baseRequest("custom_endpoint", "https://provider.example/messages", nil),
	} {
		request.ReasoningEffort = " high "
		prepared, err := BuildHTTPRequest(request)
		if !errors.Is(err, domainmodel.ErrInvalidReasoningEffort) || prepared.RequestURL != "" || prepared.Body != nil || prepared.Headers != nil {
			t.Fatalf("invalid effort must stop before request construction: prepared=%#v err=%v", prepared, err)
		}
	}
}

func TestBuildHTTPRequestRejectsNegativeOutputBudgetForEveryEndpointShape(t *testing.T) {
	for _, shape := range []string{"chat_completions", "responses", "messages"} {
		for _, format := range []string{shape, "custom_endpoint"} {
			request := baseRequest(format, "https://provider.example/"+shape, nil)
			request.MaxOutputTokens = -1
			prepared, err := BuildHTTPRequest(request)
			if err == nil || prepared.RequestURL != "" || prepared.Body != nil || prepared.Headers != nil {
				t.Fatalf("negative output budget must stop before request construction: shape=%s format=%s", shape, format)
			}
		}
	}
}

func TestBuildHTTPRequestPreservesEndpointFamilies(t *testing.T) {
	cases := []struct {
		name             string
		request          domainmodel.Request
		wantURL          string
		wantBodyFields   []string
		wantHeaderFields []string
	}{
		{
			name: "chat completions",
			request: baseRequest("chat_completions", "https://api.example/v1", []domainmodel.ToolSchema{{
				Name:        "read",
				Description: "Read a file",
				Parameters:  json.RawMessage(`{"type":"object","required":["path"],"properties":{"path":{"type":"string"}}}`),
			}}),
			wantURL:          "https://api.example/v1/chat/completions",
			wantBodyFields:   []string{"messages", "model", "stream", "stream_options", "tools"},
			wantHeaderFields: []string{"Accept", "Authorization", "Content-Type"},
		},
		{
			name:             "official DeepSeek root uses versioned chat route",
			request:          baseRequest("chat_completions", "https://api.deepseek.com", nil),
			wantURL:          "https://api.deepseek.com/v1/chat/completions",
			wantBodyFields:   []string{"messages", "model", "stream", "stream_options"},
			wantHeaderFields: []string{"Accept", "Authorization", "Content-Type"},
		},
		{
			name:             "responses",
			request:          baseRequest("responses", "https://api.example/v1", nil),
			wantURL:          "https://api.example/v1/responses",
			wantBodyFields:   []string{"input", "model", "stream"},
			wantHeaderFields: []string{"Accept", "Authorization", "Content-Type"},
		},
		{
			name:             "anthropic messages",
			request:          baseRequest("messages", "https://api.anthropic.com/v1", nil),
			wantURL:          "https://api.anthropic.com/v1/messages",
			wantBodyFields:   []string{"max_tokens", "messages", "model", "stream"},
			wantHeaderFields: []string{"Accept", "Content-Type", "anthropic-version", "x-api-key"},
		},
		{
			name:             "custom full endpoint",
			request:          baseRequest("custom_endpoint", "https://provider.example/custom/v1/chat/completions", nil),
			wantURL:          "https://provider.example/custom/v1/chat/completions",
			wantBodyFields:   []string{"messages", "model", "stream", "stream_options"},
			wantHeaderFields: []string{"Accept", "Authorization", "Content-Type"},
		},
		{
			name:             "custom full endpoint responses",
			request:          baseRequest("custom_endpoint", "https://provider.example/openai/v1/responses?api-version=2026-01-01", nil),
			wantURL:          "https://provider.example/openai/v1/responses?api-version=2026-01-01",
			wantBodyFields:   []string{"input", "model", "stream"},
			wantHeaderFields: []string{"Accept", "Authorization", "Content-Type"},
		},
		{
			name:             "custom full endpoint anthropic messages",
			request:          baseRequest("custom_endpoint", "https://provider.example/anthropic/messages", nil),
			wantURL:          "https://provider.example/anthropic/messages",
			wantBodyFields:   []string{"max_tokens", "messages", "model", "stream"},
			wantHeaderFields: []string{"Accept", "Authorization", "Content-Type", "anthropic-version", "x-api-key"},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			prepared, err := BuildHTTPRequest(tt.request)
			if err != nil {
				t.Fatalf("BuildHTTPRequest failed: %v", err)
			}
			if prepared.RequestURL != tt.wantURL {
				t.Fatalf("request URL mismatch: got %q want %q", prepared.RequestURL, tt.wantURL)
			}
			if got := sortedAnyMapKeys(prepared.Body); !reflect.DeepEqual(got, tt.wantBodyFields) {
				t.Fatalf("body fields mismatch: got %#v want %#v", got, tt.wantBodyFields)
			}
			if got := sortedStringMapKeys(prepared.Headers); !reflect.DeepEqual(got, tt.wantHeaderFields) {
				t.Fatalf("header fields mismatch: got %#v want %#v", got, tt.wantHeaderFields)
			}
		})
	}
}

func TestBuildHTTPRequestRequiresModelProviderCredentials(t *testing.T) {
	if _, err := BuildHTTPRequest(domainmodel.Request{
		ProviderID:     "provider-a",
		EndpointFormat: "chat_completions",
		BaseURL:        "https://api.example/v1",
		Model:          "model-a",
	}); err == nil {
		t.Fatalf("expected missing api key error")
	}
	if _, err := BuildHTTPRequest(domainmodel.Request{
		ProviderID:     "provider-a",
		EndpointFormat: "chat_completions",
		APIKey:         "key",
		BaseURL:        "https://api.example/v1",
	}); err == nil {
		t.Fatalf("expected missing model error")
	}
}

func baseRequest(endpointFormat string, baseURL string, tools []domainmodel.ToolSchema) domainmodel.Request {
	return domainmodel.Request{
		ProviderID:     "provider-a",
		Family:         "openai-compatible",
		EndpointFormat: endpointFormat,
		BaseURL:        baseURL,
		APIKey:         "test-key",
		Model:          "model-a",
		Messages: []domainmodel.Message{{
			Role:    "user",
			Content: "hello",
		}},
		Tools: tools,
	}
}

func sortedAnyMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
