//go:build !analytix_prod

package providerscript

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"

	provider "analytix.local/runtime-go/internal/provider"
)

func RunLocalProviderContract(ctx context.Context) (map[string]any, error) {
	fake := NewScriptedProviderServer()
	defer fake.Close()

	client := provider.NewHTTPProviderClient(fake.Client())
	requests := []provider.Request{
		{
			ProviderID:        "deepseek-test-local",
			Family:            "deepseek",
			EndpointFormat:    "chat_completions",
			BaseURL:           fake.URL + "/deepseek/v1",
			APIKey:            "test-provider-key",
			Model:             "deepseek-chat",
			ReasoningEffort:   "high",
			ReasoningProtocol: "deepseek-chat-completions",
			SystemPrompt:      "You are analytix.",
			Messages: []provider.Message{
				{Role: "system", Content: "You are analytix."},
				{Role: "user", Content: "Say hi."},
			},
			Tools: DefaultProductionCandidateTools(),
		},
		{
			ProviderID:     "openai-compatible-test-local",
			Family:         "openai-compatible",
			EndpointFormat: "chat_completions",
			BaseURL:        fake.URL + "/openai/v1",
			APIKey:         "test-provider-key",
			Model:          "gpt-compatible",
			SystemPrompt:   "You are analytix.",
			Messages: []provider.Message{
				{Role: "system", Content: "You are analytix."},
				{Role: "user", Content: "Say hi."},
			},
			Tools: DefaultProductionCandidateTools(),
		},
		{
			ProviderID:     "anthropic-compatible-test-local",
			Family:         "anthropic-compatible",
			EndpointFormat: "messages",
			BaseURL:        fake.URL + "/anthropic",
			APIKey:         "test-provider-key",
			Model:          "claude-compatible",
			SystemPrompt:   "You are analytix.",
			Messages: []provider.Message{
				{Role: "system", Content: "You are analytix."},
				{Role: "user", Content: "Say hi."},
			},
			Tools: DefaultProductionCandidateTools(),
		},
	}
	results := make([]provider.Result, 0, len(requests))
	for _, request := range requests {
		result, err := client.Stream(ctx, request)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	deepseek := results[0]
	equivalent := requests[0]
	equivalent.Messages = append(equivalent.Messages, provider.Message{Role: "user", Content: "Dynamic turn text not included in stable prefix."})
	equivalentShape := provider.CapturePrefixShape(equivalent)
	families := []string{}
	cacheByFamily := map[string]provider.Usage{}
	for _, result := range results {
		families = append(families, result.Family)
		cacheByFamily[result.Family] = result.Usage
	}
	return map[string]any{
		"runtimeGoContractParitySlice":   true,
		"providerFamiliesCovered":        sameStringSet(families, []string{"deepseek", "openai-compatible", "anthropic-compatible"}),
		"families":                       families,
		"contractReplayProviderServer":   true,
		"readsRealAPIKeys":               false,
		"externalNetworkUsed":            false,
		"requestShapeCount":              len(results),
		"streamCompletedCount":           countCompleted(results),
		"usageByFamily":                  cacheByFamily,
		"deepseekCacheHitTokens":         deepseek.Usage.CacheHitTokens,
		"deepseekCacheMissTokens":        deepseek.Usage.CacheMissTokens,
		"deepseekCacheHitRate":           deepseek.Usage.CacheHitRate,
		"deepseekCacheFieldsConsistent":  deepseek.Usage.CacheHitTokens == 700 && deepseek.Usage.CacheMissTokens == 300,
		"openaiCompatibleCacheHitTokens": cacheByFamily["openai-compatible"].CacheHitTokens,
		"anthropicCacheHitTokens":        cacheByFamily["anthropic-compatible"].CacheHitTokens,
		"stablePrefixEquivalent":         deepseek.PrefixShape.PrefixHash == equivalentShape.PrefixHash,
		"dynamicStateInStablePrefix":     false,
		"results":                        results,
	}, nil
}

func DefaultProductionCandidateTools() []provider.ToolSchema {
	return []provider.ToolSchema{
		{
			Name:        "read_file",
			Description: "Read a workspace file.",
			Parameters:  []byte(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
		},
	}
}

func NewScriptedProviderServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		switch {
		case stringsHasSuffix(r.URL.Path, "/chat/completions") && bytes.Contains(body, []byte(`"thinking"`)):
			writeFakeSSE(w, []string{
				`data: {"choices":[{"delta":{"reasoning_content":"thought "}}]}`,
				`data: {"choices":[{"delta":{"content":"deepseek hello"},"finish_reason":"stop"}]}`,
				`data: {"choices":[],"usage":{"prompt_tokens":1000,"completion_tokens":40,"total_tokens":1040,"prompt_cache_hit_tokens":700,"prompt_cache_miss_tokens":300,"prompt_tokens_details":{"cached_tokens":700},"completion_tokens_details":{"reasoning_tokens":12}}}`,
				`data: [DONE]`,
			})
		case stringsHasSuffix(r.URL.Path, "/chat/completions"):
			writeFakeSSE(w, []string{
				`data: {"choices":[{"delta":{"content":"openai compatible hello"},"finish_reason":"stop"}]}`,
				`data: {"choices":[],"usage":{"prompt_tokens":400,"completion_tokens":30,"total_tokens":430,"prompt_tokens_details":{"cached_tokens":300},"completion_tokens_details":{"reasoning_tokens":3}}}`,
				`data: [DONE]`,
			})
		case stringsHasSuffix(r.URL.Path, "/responses"):
			writeFakeSSE(w, []string{
				`event: response.output_text.delta`,
				`data: {"type":"response.output_text.delta","delta":"openai responses hello"}`,
				`event: response.completed`,
				`data: {"type":"response.completed","response":{"usage":{"input_tokens":360,"output_tokens":28,"total_tokens":388,"input_tokens_details":{"cached_tokens":240},"output_tokens_details":{"reasoning_tokens":4}}}}`,
			})
		case stringsHasSuffix(r.URL.Path, "/v1/messages"):
			writeFakeSSE(w, []string{
				`event: message_start`,
				`data: {"type":"message_start","message":{"usage":{"input_tokens":50,"cache_creation_input_tokens":200,"cache_read_input_tokens":1000}}}`,
				`event: content_block_delta`,
				`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"anthropic compatible hello"}}`,
				`event: message_delta`,
				`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":20}}`,
				`event: message_stop`,
				`data: {"type":"message_stop"}`,
			})
		case stringsHasSuffix(r.URL.Path, "/custom-endpoint"):
			writeFakeSSE(w, []string{
				`data: {"choices":[{"delta":{"content":"custom endpoint hello"},"finish_reason":"stop"}]}`,
				`data: {"choices":[],"usage":{"prompt_tokens":300,"completion_tokens":25,"total_tokens":325,"prompt_tokens_details":{"cached_tokens":200},"completion_tokens_details":{"reasoning_tokens":2}}}`,
				`data: [DONE]`,
			})
		default:
			http.Error(w, "unknown fake provider path", http.StatusNotFound)
		}
	}))
}

func writeFakeSSE(w http.ResponseWriter, frames []string) {
	for _, frame := range frames {
		_, _ = w.Write([]byte(frame))
		_, _ = w.Write([]byte("\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}
}

func countCompleted(results []provider.Result) int {
	count := 0
	for _, result := range results {
		if result.StreamCompleted {
			count++
		}
	}
	return count
}

func sameStringSet(actual []string, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	seen := map[string]int{}
	for _, value := range actual {
		seen[value]++
	}
	for _, value := range expected {
		seen[value]--
		if seen[value] < 0 {
			return false
		}
	}
	return true
}

func stringsHasSuffix(value string, suffix string) bool {
	return len(value) >= len(suffix) && value[len(value)-len(suffix):] == suffix
}
