package model

import (
	"math"
	"testing"
)

func TestNormalizeUsageRejectsProviderControlledFinishReasonAndInvalidNumbers(t *testing.T) {
	usage := NormalizeUsage(Usage{
		PromptTokens: 100, CompletionTokens: 20, ReasoningTokens: 200, TotalTokens: -1,
		CacheHitTokens: 200, CacheMissTokens: 10, CacheHitRate: math.NaN(), FinishReason: "SOL_PRIVATE_TRACE_7C",
		CostUSD: math.Inf(1), CostCNY: -1, Currency: "SOL_PRIVATE_TRACE_7C", HasCacheHit: true, HasCacheMiss: true,
	})
	if usage.FinishReason != "unknown" || usage.TotalTokens != 120 || usage.ReasoningTokens != 0 ||
		usage.CacheHitTokens != 0 || usage.CacheMissTokens != 0 || usage.CacheHitRate != 0 || usage.CostUSD != 0 || usage.CostCNY != 0 || usage.Currency != "" {
		t.Fatalf("provider usage was not normalized: %#v", usage)
	}
}

func TestNormalizeFinishReasonUsesClosedVocabulary(t *testing.T) {
	tests := map[string]string{"end_turn": "stop", "max_tokens": "length", "tool_use": "tool_calls", "safety": "content_filter", "private": "unknown"}
	for input, want := range tests {
		if got := NormalizeFinishReason(input); got != want {
			t.Fatalf("finish reason %q: got %q want %q", input, got, want)
		}
	}
}
