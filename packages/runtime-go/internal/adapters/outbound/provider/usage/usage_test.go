package usage

import (
	"strings"
	"testing"

	appmodel "analytix.local/runtime-go/internal/app/model"
	appusage "analytix.local/runtime-go/internal/app/usage"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestFromTokensTracksCacheBreakdown(t *testing.T) {
	usage := FromTokens(100, 25, 0, 7, 40, 60, true)
	if usage.PromptTokens != 100 || usage.CompletionTokens != 25 || usage.TotalTokens != 125 {
		t.Fatalf("unexpected totals: %#v", usage)
	}
	if usage.ReasoningTokens != 7 || usage.CacheHitTokens != 40 || usage.CacheMissTokens != 60 {
		t.Fatalf("unexpected token breakdown: %#v", usage)
	}
	if usage.CacheHitRate != 0.4 || !usage.HasCacheTelemetry() {
		t.Fatalf("unexpected cache telemetry: %#v", usage)
	}
}

func TestApplyPricingHandlesCacheSavingsAndCurrencies(t *testing.T) {
	usage := FromTokens(1000, 500, 1500, 0, 250, 750, true)
	priced := ApplyPricing(usage, &domainmodel.Pricing{CacheHit: 1, Input: 3, Output: 6, Currency: "CNY"})
	if !priced.PriceConfigured || priced.Currency != "CNY" {
		t.Fatalf("pricing metadata missing: %#v", priced)
	}
	if priced.CostCNY <= 0 || priced.CostUSD != 0 || priced.CacheSavingsCNY <= 0 || priced.CacheSavingsUSD != 0 {
		t.Fatalf("expected CNY and USD cost/savings: %#v", priced)
	}

	unpriced := ApplyPricing(usage, nil)
	if unpriced.PriceConfigured || unpriced.CostUSD != 0 || unpriced.Currency != "" {
		t.Fatalf("nil pricing should leave usage cost unknown: %#v", unpriced)
	}
}

func TestCacheDiagnosticsReportsPrefixAndToolSourceDrift(t *testing.T) {
	previous := domainmodel.PrefixShape{
		PrefixHash:      "prefix-a",
		SystemHash:      "system-a",
		ToolsHash:       "tools-a",
		ToolSourcesHash: "sources-a",
		ProviderID:      "provider-a",
		EndpointFormat:  "chat_completions",
		Model:           "model-a",
	}
	current := domainmodel.Result{
		ProviderID:     "provider-a",
		Family:         "deepseek",
		EndpointFormat: "chat_completions",
		Usage:          FromTokens(100, 10, 110, 0, 25, 75, true),
		PrefixShape: domainmodel.PrefixShape{
			PrefixHash:      "prefix-b",
			SystemHash:      "system-a",
			ToolsHash:       "tools-b",
			ToolSourcesHash: "sources-b",
			ProviderID:      "provider-a",
			EndpointFormat:  "chat_completions",
			Model:           "model-a",
		},
		HasFirstTokenLatency: true,
		FirstTokenLatencyMs:  42,
	}
	diagnostics := CacheDiagnosticsWithPrevious(previous, current)
	if diagnostics["prefixChanged"] != true || diagnostics["toolSourceChanged"] != true {
		t.Fatalf("expected prefix and tool source drift: %#v", diagnostics)
	}
	if diagnostics["cacheHitTokens"] != 25 || diagnostics["cacheMissTokens"] != 75 {
		t.Fatalf("expected DeepSeek cache token diagnostics: %#v", diagnostics)
	}
	reasons, _ := diagnostics["prefixChangeReasons"].([]string)
	if !contains(reasons, "tools") {
		t.Fatalf("expected tools prefix reason: %#v", diagnostics)
	}
	toolSourceReasons, _ := diagnostics["toolSourceChangeReasons"].([]string)
	if !contains(toolSourceReasons, "tool_sources") {
		t.Fatalf("expected tool_sources reason: %#v", diagnostics)
	}
	if diagnostics["firstTokenLatencyMs"] != int64(42) {
		t.Fatalf("expected first token latency: %#v", diagnostics)
	}
}

func TestProductionCacheDiagnosticsPassClosedTerminalProjection(t *testing.T) {
	shape := appmodel.CapturePrefixShape(domainmodel.Request{
		ProviderID: "deepseek", Family: "deepseek", EndpointFormat: "chat_completions",
		Model: "deepseek-v4-pro", Route: "direct_answer", SystemPrompt: "stable system prefix",
	})
	providerUsage := FromTokens(100, 5, 105, 0, 20, 80, true)
	diagnostics := CacheDiagnostics(domainmodel.Result{
		ProviderID: "deepseek", Family: "deepseek", EndpointFormat: "chat_completions",
		Usage: providerUsage, PrefixShape: shape,
		HasFirstTokenLatency: true, FirstTokenLatencyMs: 42,
		HasDuration: true, DurationMs: 87,
	})
	projection := appusage.NewTerminalTelemetryV1(providerUsage, diagnostics)
	got := projection.PublicCacheDiagnosticsMap()
	if got["terminalCacheDiagnosticsDisposition"] != nil || got["route"] != "direct_answer" ||
		got["toolCount"] != uint64(0) || got["firstTokenLatencyMs"] != uint64(42) || got["durationMs"] != uint64(87) {
		t.Fatalf("production cache diagnostics did not survive the closed terminal projection: %#v", got)
	}
	for _, key := range []string{"systemHash", "toolsHash", "prefixHash", "prefixItemsHash", "toolSourcesHash"} {
		digest, ok := got[key].(string)
		if !ok || len(digest) != 64 || strings.ToLower(digest) != digest {
			t.Fatalf("production %s is not a full lowercase SHA-256 digest: %#v", key, got[key])
		}
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
