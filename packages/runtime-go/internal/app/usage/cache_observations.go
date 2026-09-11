package usage

import (
	appcache "analytix.local/runtime-go/internal/app/cachetelemetry"
	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainterminaltelemetry "analytix.local/runtime-go/internal/domain/terminaltelemetry"
)

const ProviderAttemptTelemetrySchemaV1 = domainterminaltelemetry.ProviderAttemptTelemetrySchemaV1

// ProviderAttemptDiagnostics validates and aggregates every physical provider
// attempt. It emits numeric counters and closed status only; HMAC identities,
// request bytes, endpoints, credentials, model names, provider text, and
// reasoning have no public representation here.
func ProviderAttemptDiagnostics(observations []domaincache.ProviderCallObservationV1) map[string]any {
	if len(observations) == 0 {
		return nil
	}
	service := appcache.NewService()
	for _, observation := range observations {
		if err := service.Begin(observation.Shape); err != nil {
			return invalidProviderAttemptDiagnostics()
		}
		if err := service.Settle(observation); err != nil {
			return invalidProviderAttemptDiagnostics()
		}
	}
	aggregate, err := service.Aggregate()
	if err != nil {
		return invalidProviderAttemptDiagnostics()
	}
	return map[string]any{
		"providerAttemptTelemetrySchema": ProviderAttemptTelemetrySchemaV1,
		"providerAttemptTelemetryValid":  true,
		"providerLogicalCallCount":       aggregate.LogicalCallCount,
		"providerAttemptCount":           aggregate.AttemptCount,
		"providerAttemptStatuses": map[string]any{
			"succeeded": aggregate.Statuses.Succeeded, "failed": aggregate.Statuses.Failed,
			"cancelled": aggregate.Statuses.Cancelled, "timedOut": aggregate.Statuses.TimedOut,
			"streamAborted": aggregate.Statuses.StreamAborted,
		},
		"providerAttemptInputTokens":     aggregateTokenDiagnostics(aggregate.InputTokens),
		"providerAttemptOutputTokens":    aggregateTokenDiagnostics(aggregate.OutputTokens),
		"providerAttemptCacheHitTokens":  aggregateTokenDiagnostics(aggregate.CacheHitTokens),
		"providerAttemptCacheMissTokens": aggregateTokenDiagnostics(aggregate.CacheMissTokens),
		"providerAttemptCacheRate": map[string]any{
			"known": aggregate.CacheRate.Known, "numerator": aggregate.CacheRate.Numerator,
			"denominator": aggregate.CacheRate.Denominator,
		},
	}
}

func invalidProviderAttemptDiagnostics() map[string]any {
	return map[string]any{
		"providerAttemptTelemetrySchema": ProviderAttemptTelemetrySchemaV1,
		"providerAttemptTelemetryValid":  false,
		"providerAttemptTelemetryError":  "invalid_or_incomplete",
	}
}

func aggregateTokenDiagnostics(count appcache.AggregatedTokenCountV1) map[string]any {
	result := map[string]any{
		"complete": count.Complete, "knownObservationCount": count.KnownObservationCount,
		"observationCount": count.ObservationCount,
	}
	if count.Complete {
		result["value"] = count.Value
	}
	return result
}
