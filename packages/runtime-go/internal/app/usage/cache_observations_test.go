package usage

import (
	"strings"
	"testing"

	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
)

func TestProviderAttemptDiagnosticsAggregatesEverySettledAttempt(t *testing.T) {
	first := cacheObservationFixture(1, domaincache.ProviderCallStatusFailed, 20, 80, "2026-07-13T00:00:00Z", "2026-07-13T00:00:01Z")
	second := cacheObservationFixture(2, domaincache.ProviderCallStatusSucceeded, 80, 20, "2026-07-13T00:00:01Z", "2026-07-13T00:00:02Z")
	diagnostics := ProviderAttemptDiagnostics([]domaincache.ProviderCallObservationV1{first, second})
	if diagnostics["providerAttemptTelemetryValid"] != true || diagnostics["providerAttemptCount"] != uint64(2) ||
		diagnostics["providerLogicalCallCount"] != uint64(1) {
		t.Fatalf("unexpected provider-attempt aggregate: %#v", diagnostics)
	}
	statuses := diagnostics["providerAttemptStatuses"].(map[string]any)
	if statuses["failed"] != uint64(1) || statuses["succeeded"] != uint64(1) {
		t.Fatalf("terminal attempts were not counted exactly: %#v", statuses)
	}
	hit := diagnostics["providerAttemptCacheHitTokens"].(map[string]any)
	miss := diagnostics["providerAttemptCacheMissTokens"].(map[string]any)
	if hit["complete"] != true || hit["value"] != uint64(100) || miss["value"] != uint64(100) {
		t.Fatalf("cache usage did not include every attempt: hit=%#v miss=%#v", hit, miss)
	}
	rate := diagnostics["providerAttemptCacheRate"].(map[string]any)
	if rate["known"] != true || rate["numerator"] != uint64(100) || rate["denominator"] != uint64(200) {
		t.Fatalf("cache rate is not the exact aggregate ratio: %#v", rate)
	}
	if _, exposed := diagnostics["providerAttemptReasoningTokens"]; exposed {
		t.Fatalf("public cache diagnostics must not expose reasoning metadata: %#v", diagnostics)
	}
}

func TestProviderAttemptDiagnosticsNeverCollapsesUnknownIntoZero(t *testing.T) {
	unknown := cacheObservationFixture(1, domaincache.ProviderCallStatusFailed, 0, 0, "2026-07-13T00:00:00Z", "2026-07-13T00:00:01Z")
	unknown.Usage.CacheHitTokens = domaincache.TokenCountV1{}
	unknown.Usage.CacheMissTokens = domaincache.TokenCountV1{}
	known := cacheObservationFixture(2, domaincache.ProviderCallStatusSucceeded, 0, 0, "2026-07-13T00:00:01Z", "2026-07-13T00:00:02Z")
	diagnostics := ProviderAttemptDiagnostics([]domaincache.ProviderCallObservationV1{unknown, known})
	hit := diagnostics["providerAttemptCacheHitTokens"].(map[string]any)
	if hit["complete"] != false || hit["knownObservationCount"] != uint64(1) || hit["observationCount"] != uint64(2) {
		t.Fatalf("unknown cache usage was collapsed into explicit zero: %#v", hit)
	}
	if _, exists := hit["value"]; exists {
		t.Fatalf("incomplete cache usage published a numeric value: %#v", hit)
	}
	if diagnostics["providerAttemptCacheRate"].(map[string]any)["known"] != false {
		t.Fatalf("incomplete cache usage published a rate: %#v", diagnostics)
	}
}

func TestProviderAttemptDiagnosticsFailsClosedOnInvalidObservation(t *testing.T) {
	invalid := cacheObservationFixture(1, domaincache.ProviderCallStatusSucceeded, 1, 0, "2026-07-13T00:00:00Z", "2026-07-13T00:00:01Z")
	invalid.Usage.CacheHitTokens = domaincache.TokenCountV1{Known: false, Value: 1}
	diagnostics := ProviderAttemptDiagnostics([]domaincache.ProviderCallObservationV1{invalid})
	if diagnostics["providerAttemptTelemetryValid"] != false || diagnostics["providerAttemptTelemetryError"] != "invalid_or_incomplete" || len(diagnostics) != 3 {
		t.Fatalf("invalid observation did not fail closed: %#v", diagnostics)
	}
}

func cacheObservationFixture(attempt uint32, status domaincache.ProviderCallStatusV1, hit, miss uint64, startedAt, settledAt string) domaincache.ProviderCallObservationV1 {
	return domaincache.ProviderCallObservationV1{
		SchemaVersion: domaincache.ProviderCallObservationV1SchemaVersion,
		Shape: domaincache.CacheVisibleShapeV1{
			SchemaVersion: domaincache.CacheVisibleShapeV1SchemaVersion, LogicalCallHMAC: strings.Repeat("a", 64),
			Attempt: attempt, ProviderFamily: domaincache.ProviderFamilyDeepSeek, ModelHMAC: strings.Repeat("b", 64),
			Endpoint: domaincache.EndpointFormatChatCompletions, EndpointHMAC: strings.Repeat("c", 64),
			WireBodyHMAC: strings.Repeat("d", 64), CredentialScopeHMAC: strings.Repeat("e", 64),
			ProviderConfigHMAC: strings.Repeat("f", 64), DigestEpoch: 1, StartedAt: startedAt,
		},
		Status: status,
		Usage: domaincache.ProviderUsageV1{
			InputTokens: domaincache.TokenCountV1{Known: true, Value: hit + miss}, OutputTokens: domaincache.TokenCountV1{Known: true, Value: 1},
			CacheHitTokens: domaincache.TokenCountV1{Known: true, Value: hit}, CacheMissTokens: domaincache.TokenCountV1{Known: true, Value: miss},
			ReasoningTokens: domaincache.TokenCountV1{},
		},
		SettledAt: settledAt,
	}
}
