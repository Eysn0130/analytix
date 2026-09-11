package usage

import (
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestProviderUsageMapKeepsUnknownCacheTelemetryUnknown(t *testing.T) {
	out := ProviderUsageMap(domainmodel.Usage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120})
	if _, ok := out["cacheHitTokens"]; ok {
		t.Fatalf("unexpected cacheHitTokens for provider without cache telemetry: %#v", out)
	}
	if out["cacheHitRate"] != nil || out["cacheableTokenHitRate"] != nil || out["totalInputTokenHitRate"] != nil {
		t.Fatalf("expected nil cache rates for unknown telemetry: %#v", out)
	}
}

func TestProviderUsageMapReportsCacheBreakdown(t *testing.T) {
	out := ProviderUsageMap(domainmodel.Usage{
		PromptTokens:     100,
		CompletionTokens: 25,
		TotalTokens:      125,
		CacheHitTokens:   30,
		CacheMissTokens:  70,
		HasCacheHit:      true,
		HasCacheMiss:     true,
		PriceConfigured:  true,
	})
	if out["cacheHitTokens"] != 30 || out["cacheMissTokens"] != 70 || out["cachedTokens"] != 30 {
		t.Fatalf("unexpected cache token fields: %#v", out)
	}
	if got := out["cacheHitRate"]; got != 0.3 {
		t.Fatalf("unexpected cacheHitRate: %#v", got)
	}
	if got := out["totalInputTokenHitRate"]; got != 0.3 {
		t.Fatalf("unexpected totalInputTokenHitRate: %#v", got)
	}
	if out["priceConfigured"] != true {
		t.Fatalf("expected priceConfigured true: %#v", out)
	}
}
