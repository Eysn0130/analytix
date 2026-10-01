package usage

import (
	"math"
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func FromTokens(prompt, completion, total, reasoning, hit, miss int, hasCacheTelemetry bool) domainmodel.Usage {
	if total == 0 {
		total = prompt + completion
	}
	rate := 0.0
	if hit+miss > 0 {
		rate = float64(hit) / float64(hit+miss)
	}
	return domainmodel.Usage{
		PromptTokens:     prompt,
		CompletionTokens: completion,
		ReasoningTokens:  reasoning,
		TotalTokens:      total,
		CacheHitTokens:   hit,
		CacheMissTokens:  miss,
		CacheHitRate:     rate,
		HasCacheHit:      hasCacheTelemetry,
		HasCacheMiss:     hasCacheTelemetry,
	}
}

func ApplyPricing(usage domainmodel.Usage, pricing *domainmodel.Pricing) domainmodel.Usage {
	if pricing == nil {
		return usage
	}
	if pricing.Input < 0 || pricing.Output < 0 || pricing.CacheHit < 0 || math.IsNaN(pricing.Input+pricing.Output+pricing.CacheHit) || math.IsInf(pricing.Input+pricing.Output+pricing.CacheHit, 0) {
		usage.PriceConfigured = false
		usage.CostUSD = 0
		usage.CostCNY = 0
		return usage
	}
	usage.CostCNY, usage.CostUSD = 0, 0
	usage.CacheSavingsCNY, usage.CacheSavingsUSD = 0, 0
	usage.Currency = ""
	hit := usage.CacheHitTokens
	miss := usage.CacheMissTokens
	if hit+miss == 0 && usage.PromptTokens > 0 {
		miss = usage.PromptTokens
	} else if miss == 0 && hit > 0 && usage.PromptTokens > hit {
		miss = usage.PromptTokens - hit
	}
	cost := (float64(hit)*pricing.CacheHit +
		float64(miss)*pricing.Input +
		float64(usage.CompletionTokens)*pricing.Output) / 1e6
	savings := 0.0
	if pricing.Input > pricing.CacheHit {
		savings = float64(hit) * (pricing.Input - pricing.CacheHit) / 1e6
	}
	switch strings.ToLower(strings.TrimSpace(pricing.Currency)) {
	case "cny", "rmb", "¥", "￥":
		usage.PriceConfigured = true
		usage.CostCNY = cost
		usage.CacheSavingsCNY = savings
		usage.Currency = "CNY"
	case "usd", "$":
		usage.PriceConfigured = true
		usage.CostUSD = cost
		usage.CacheSavingsUSD = savings
		usage.Currency = "USD"
	default:
		// No exchange-rate or currency authority: do not fabricate a USD bill.
		usage.PriceConfigured = false
	}
	return usage
}

func CacheDiagnostics(result domainmodel.Result) map[string]any {
	return CacheDiagnosticsWithPrevious(domainmodel.PrefixShape{}, result)
}

func CacheDiagnosticsWithPrevious(previous domainmodel.PrefixShape, result domainmodel.Result) map[string]any {
	shape := result.PrefixShape
	cacheTelemetryPresent := result.Usage.HasCacheTelemetry()
	providerNativeCacheTelemetry := cacheTelemetryPresent
	cacheTelemetrySupported := result.Family == "deepseek" || providerNativeCacheTelemetry
	prefixChangeReasons := prefixShapeChangeReasons(previous, shape)
	toolSourceChanged := (previous.ToolsHash != "" && previous.ToolsHash != shape.ToolsHash) ||
		(previous.ToolSourcesHash != "" && previous.ToolSourcesHash != shape.ToolSourcesHash)
	toolSourceChangeReasons := []string{}
	if previous.ToolsHash != "" && previous.ToolsHash != shape.ToolsHash {
		toolSourceChangeReasons = append(toolSourceChangeReasons, "tools")
	}
	if previous.ToolSourcesHash != "" && previous.ToolSourcesHash != shape.ToolSourcesHash {
		toolSourceChangeReasons = append(toolSourceChangeReasons, "tool_sources")
	}
	toolSourceIDs := shape.ToolSourceIDs
	if len(toolSourceIDs) == 0 && shape.ToolsHash != "" {
		toolSourceIDs = []string{"builtin"}
	}
	toolSourcesHash := shape.ToolSourcesHash
	if toolSourcesHash == "" {
		toolSourcesHash = shape.ToolsHash
	}
	modelObservation := "not_reported"
	if result.ResponseObservedModel != "" {
		modelObservation = "differs_resolved"
		if result.ResponseObservedModel == shape.Model {
			modelObservation = "matches_resolved"
		}
	}
	diagnostics := map[string]any{
		"responseModelObservation":     modelObservation,
		"prefixHash":                   shape.PrefixHash,
		"prefixChanged":                len(prefixChangeReasons) > 0,
		"prefixChangeReasons":          prefixChangeReasons,
		"toolSourceChanged":            toolSourceChanged,
		"toolSourceChangeReasons":      toolSourceChangeReasons,
		"systemHash":                   shape.SystemHash,
		"prefixItemsHash":              shape.PrefixItemsHash,
		"toolsHash":                    shape.ToolsHash,
		"toolSchemaTokens":             shape.ToolSchemaTokens,
		"dynamicStateCheck":            "not_checked",
		"toolSchemaEstimator":          "utf8_bytes_div4",
		"toolCount":                    shape.ToolCount,
		"toolSourcesHash":              toolSourcesHash,
		"toolSourceIds":                toolSourceIDs,
		"cacheTelemetrySupported":      cacheTelemetrySupported,
		"cacheTelemetryPresent":        cacheTelemetryPresent,
		"providerNativeCacheTelemetry": providerNativeCacheTelemetry,
		"provider":                     result.Family,
		"providerId":                   result.ProviderID,
		"endpointFormat":               result.EndpointFormat,
		"model":                        shape.Model,
	}
	if shape.Route != "" {
		diagnostics["route"] = shape.Route
	}
	if result.HasFirstTokenLatency {
		diagnostics["firstTokenLatencyMs"] = result.FirstTokenLatencyMs
	}
	if result.HasDuration {
		diagnostics["durationMs"] = result.DurationMs
	}
	if cacheTelemetryPresent {
		diagnostics["cacheHitTokens"] = result.Usage.CacheHitTokens
		diagnostics["cacheMissTokens"] = result.Usage.CacheMissTokens
		diagnostics["cacheHitRate"] = result.Usage.CacheHitRate
	}
	if cacheTelemetryPresent {
		diagnostics["cacheTelemetrySource"] = "provider_usage"
	}
	return diagnostics
}

func prefixShapeChangeReasons(previous domainmodel.PrefixShape, current domainmodel.PrefixShape) []string {
	if previous.PrefixHash == "" {
		return []string{}
	}
	reasons := []string{}
	if previous.SystemHash != "" && previous.SystemHash != current.SystemHash {
		reasons = append(reasons, "system")
	}
	if previous.ToolsHash != "" && previous.ToolsHash != current.ToolsHash {
		reasons = append(reasons, "tools")
	}
	if previous.PrefixItemsHash != "" && previous.PrefixItemsHash != current.PrefixItemsHash {
		reasons = append(reasons, "prefix")
	}
	if previous.Provider != "" && previous.Provider != current.Provider {
		reasons = append(reasons, "provider")
	}
	if previous.ProviderID != "" && previous.ProviderID != current.ProviderID {
		reasons = append(reasons, "provider_id")
	}
	if previous.EndpointFormat != "" && previous.EndpointFormat != current.EndpointFormat {
		reasons = append(reasons, "endpoint_format")
	}
	if previous.Model != "" && previous.Model != current.Model {
		reasons = append(reasons, "model")
	}
	if len(reasons) == 0 && previous.PrefixHash != current.PrefixHash {
		reasons = append(reasons, "prefix")
	}
	return reasons
}
