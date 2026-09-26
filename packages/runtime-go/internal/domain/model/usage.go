package model

import (
	"math"
	"strings"
)

const maxPublicTokenCount = 1_000_000_000

func NormalizeUsage(input Usage) Usage {
	prompt := boundedTokenCount(input.PromptTokens)
	completion := boundedTokenCount(input.CompletionTokens)
	reasoning := boundedTokenCount(input.ReasoningTokens)
	if reasoning > completion {
		reasoning = 0
	}
	total := boundedTokenCount(input.TotalTokens)
	if total < prompt+completion {
		total = prompt + completion
	}
	hit := boundedTokenCount(input.CacheHitTokens)
	miss := boundedTokenCount(input.CacheMissTokens)
	if hit+miss > prompt {
		hit, miss = 0, 0
	}
	hasHit := input.HasCacheHit
	hasMiss := input.HasCacheMiss
	if !hasHit && !hasMiss && hit == 0 && miss == 0 {
		hasHit, hasMiss = false, false
	}
	rate := 0.0
	if hit+miss > 0 {
		rate = float64(hit) / float64(hit+miss)
	}
	currency := strings.ToUpper(strings.TrimSpace(input.Currency))
	if currency != "USD" && currency != "CNY" {
		currency = ""
	}
	return Usage{
		MessagesInput: input.MessagesInput, UsagePresenceKnown: input.UsagePresenceKnown, HasPromptTokens: input.HasPromptTokens, HasCompletionTokens: input.HasCompletionTokens,
		PromptTokens: prompt, CompletionTokens: completion, ReasoningTokens: reasoning, TotalTokens: total,
		CacheHitTokens: hit, CacheMissTokens: miss, CacheHitRate: rate, HasCacheHit: hasHit, HasCacheMiss: hasMiss,
		FinishReason: NormalizeFinishReason(input.FinishReason), CostUSD: boundedNonNegativeFloat(input.CostUSD),
		CostCNY: boundedNonNegativeFloat(input.CostCNY), CacheSavingsUSD: boundedNonNegativeFloat(input.CacheSavingsUSD),
		CacheSavingsCNY: boundedNonNegativeFloat(input.CacheSavingsCNY), Currency: currency, PriceConfigured: input.PriceConfigured,
	}
}

func NormalizeFinishReason(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "stop", "end_turn", "completed", "complete":
		return "stop"
	case "length", "max_tokens", "max_output_tokens":
		return "length"
	case "tool_calls", "tool_use":
		return "tool_calls"
	case "content_filter", "content_filtered", "safety":
		return "content_filter"
	default:
		return "unknown"
	}
}

func boundedTokenCount(value int) int {
	if value < 0 || value > maxPublicTokenCount {
		return 0
	}
	return value
}

func boundedNonNegativeFloat(value float64) float64 {
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) || value > 1e12 {
		return 0
	}
	return value
}
