package usage

import (
	appcache "analytix.local/runtime-go/internal/app/cachetelemetry"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainterminaltelemetry "analytix.local/runtime-go/internal/domain/terminaltelemetry"
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

func TestProviderAttemptCostsIncludeFailureRetryAndCancellationWithoutMixingCurrencies(t *testing.T) {
	first := cacheObservationFixture(1, domaincache.ProviderCallStatusFailed, 20, 80, "2026-07-13T00:00:00Z", "2026-07-13T00:00:01Z")
	second := cacheObservationFixture(2, domaincache.ProviderCallStatusSucceeded, 80, 20, "2026-07-13T00:00:01Z", "2026-07-13T00:00:02Z")
	third := cacheObservationFixture(3, domaincache.ProviderCallStatusCancelled, 0, 0, "2026-07-13T00:00:02Z", "2026-07-13T00:00:03Z")
	first.Usage.EstimatedCost = domaincache.EstimatedCostV1{Known: true, Currency: "USD", NanoUnits: 12}
	second.Usage.EstimatedCost = domaincache.EstimatedCostV1{Known: true, Currency: "CNY", NanoUnits: 30}
	observations := []domaincache.ProviderCallObservationV1{first, second, third, second}
	d := ProviderAttemptDiagnostics(observations)
	if d["providerAttemptCount"] != uint64(3) || d["providerCostKnownAttemptCount"] != uint64(2) || d["providerKnownCostUsdAttemptCount"] != uint64(1) || d["providerKnownCostCnyAttemptCount"] != uint64(1) || d["providerCostEstimateComplete"] != false || d["providerKnownCostUsdNanos"] != uint64(12) || d["providerKnownCostCnyNanos"] != uint64(30) {
		t.Fatalf("cost accounting drift: %#v", d)
	}
	projected := domainterminaltelemetry.NewTerminalTelemetryV1(domainmodel.Usage{CostUSD: 999, PriceConfigured: true}, d).PublicUsageMap()
	if projected["priceConfigured"] != false || projected["costUsd"] == 999 {
		t.Fatalf("unknown attempt exposed last-attempt cost as total: %#v", projected)
	}
	if projected["costEstimateStatus"] != "partial" || projected["costUsd"] != float64(0) || projected["costCny"] != float64(0) {
		t.Fatalf("partial known subtotal was lost: %#v", projected)
	}
	third.Usage.EstimatedCost = domaincache.EstimatedCostV1{Known: true, Currency: "USD", NanoUnits: 0}
	d = ProviderAttemptDiagnostics([]domaincache.ProviderCallObservationV1{first, second, third})
	projected = domainterminaltelemetry.NewTerminalTelemetryV1(domainmodel.Usage{}, d).PublicUsageMap()
	if projected["priceConfigured"] != true || projected["costUsd"] != float64(12)/1e9 || projected["costCny"] != float64(30)/1e9 {
		t.Fatalf("known total unavailable: %#v", projected)
	}
	if projected["costEstimateStatus"] != "complete" {
		t.Fatalf("complete currency provenance unavailable: %#v", projected)
	}
}

func TestConversationToolTimeUsesDurableInventoryAndExactEventPairs(t *testing.T) {
	const threadID, turnID = "thread-tools", "turn-tools"
	items := []any{}
	events := []map[string]any{}
	for index, callID := range []string{"call_host_" + strings.Repeat("a", 64), "call_host_" + strings.Repeat("b", 64)} {
		callItemID := domaintoolcall.ToolCallItemIDV1(turnID, callID)
		resultID := "result-" + callID
		call := map[string]any{"id": callItemID, "kind": "tool_call", "callId": callID, "threadId": threadID, "turnId": turnID}
		result := map[string]any{"id": resultID, "kind": "tool_result", "callId": callID, "threadId": threadID, "turnId": turnID, "status": "completed", "isError": false}
		items = append(items, call, result)
		events = append(events, map[string]any{"kind": "tool_call_ready", "threadId": threadID, "turnId": turnID, "itemId": callItemID, "callId": callID, "seq": index*2 + 1, "timestamp": "2026-07-14T00:00:00Z"}, map[string]any{"kind": "tool_call_finished", "threadId": threadID, "turnId": turnID, "itemId": resultID, "item": result, "seq": index*2 + 2, "timestamp": "2026-07-14T00:00:02Z"})
	}
	thread := map[string]any{"id": threadID, "turns": []any{map[string]any{"id": turnID, "items": items}}}
	assert := func(input []map[string]any, want ToolWallTimeV1) {
		t.Helper()
		got, err := conversationToolTimeV1(thread, input, turnID)
		if err != nil || got != want {
			t.Fatalf("tool wall time = %#v, want %#v: %v", got, want, err)
		}
	}
	assert(events, ToolWallTimeV1{ReportedValue: 4000, KnownCallCount: 2, CallCount: 2, Complete: true})
	assert(append(append([]map[string]any{}, events...), events[0]), ToolWallTimeV1{ReportedValue: 4000, KnownCallCount: 2, CallCount: 2, Complete: true})
	assert(nil, ToolWallTimeV1{CallCount: 2})
	assert(events[1:], ToolWallTimeV1{ReportedValue: 2000, KnownCallCount: 1, CallCount: 2})
	partial := map[string]any{}
	for key, value := range events[0] {
		partial[key] = value
	}
	partial["partial"] = true
	delete(partial, "itemId")
	assert(append([]map[string]any{partial}, events[1:]...), ToolWallTimeV1{ReportedValue: 2000, KnownCallCount: 1, CallCount: 2})
	legacyReady := map[string]any{}
	for key, value := range events[0] {
		legacyReady[key] = value
	}
	delete(legacyReady, "itemId")
	assert(append([]map[string]any{legacyReady}, events[1:]...), ToolWallTimeV1{ReportedValue: 2000, KnownCallCount: 1, CallCount: 2})
	legacyReady["seq"] = 9
	assert(append(append([]map[string]any{}, events...), legacyReady), ToolWallTimeV1{ReportedValue: 4000, KnownCallCount: 2, CallCount: 2, Complete: true})
	wrongTurn := map[string]any{}
	for key, value := range events[0] {
		wrongTurn[key] = value
	}
	wrongTurn["turnId"] = "other-turn"
	assert(append([]map[string]any{wrongTurn}, events[1:]...), ToolWallTimeV1{ReportedValue: 2000, KnownCallCount: 1, CallCount: 2})
	repeated := map[string]any{}
	for key, value := range events[0] {
		repeated[key] = value
	}
	repeated["seq"] = 10
	assert(append(append([]map[string]any{}, events...), repeated), ToolWallTimeV1{ReportedValue: 2000, KnownCallCount: 1, CallCount: 2})
	repeated["seq"] = events[0]["seq"]
	repeated["timestamp"] = time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := conversationToolTimeV1(thread, append(append([]map[string]any{}, events...), repeated), turnID); err == nil {
		t.Fatal("divergent replay must fail closed")
	}
	projection, err := BuildConversationUsageV1(thread, events, nil)
	if err != nil || projection.Billing.InventoryComplete || projection.Metrics.StepCount != nil || projection.Metrics.TurnCount != nil || !reflect.DeepEqual(projection.Metrics.ToolMs, ToolWallTimeV1{ReportedValue: 4000, KnownCallCount: 2, CallCount: 2, Complete: true}) {
		t.Fatalf("legacy usage fabricated provider zero: %#v err=%v", projection, err)
	}
	logicalEvents := []map[string]any{
		logicalMetricEventFixtureV1("post_start", "", 5, map[string]any{"logicalStepMetricsVersion": float64(1)}),
		logicalMetricEventFixtureV1("logical_step_started", "f13e1680-e522-4d40-bc18-40dde3bb66ea", 6, nil),
		logicalMetricEventFixtureV1("logical_step_closed", "f13e1680-e522-4d40-bc18-40dde3bb66ea", 7, nil),
		logicalMetricEventFixtureV1("post_start", "", 8, map[string]any{"maxModelSteps": float64(2)}),
	}
	for _, event := range logicalEvents {
		event["threadId"], event["turnId"] = threadID, turnID
	}
	mixed, err := BuildConversationUsageV1(thread, append(append([]map[string]any{}, events...), logicalEvents...), nil)
	if err != nil || mixed.Metrics.TurnCount != nil || mixed.Metrics.StepCount != nil || mixed.Metrics.LLMMs.Complete || mixed.Metrics.LLMMs.ReportedValue != 4000 || mixed.Metrics.DecodeTokens != 20 || mixed.Metrics.ToolMs != projection.Metrics.ToolMs {
		t.Fatalf("mixed coverage lost independently proven metrics: %#v err=%v", mixed, err)
	}

}

func TestConversationUsageVerifiedEmptyThreadHasCanonicalMetrics(t *testing.T) {
	got, err := BuildConversationUsageV1(map[string]any{"id": "thread-empty", "turns": []any{}}, nil, nil)
	if err != nil || got.Metrics.TurnCount == nil || *got.Metrics.TurnCount != 0 || got.Metrics.StepCount == nil || *got.Metrics.StepCount != 0 || !got.Metrics.LLMMs.Complete || !got.Metrics.ToolMs.Complete {
		t.Fatalf("empty metrics are not canonical: %#v err=%v", got, err)
	}
	if _, err := BuildConversationUsageV1(nil, nil, nil); err == nil {
		t.Fatal("absent thread cannot prove empty usage")
	}
}

func logicalMetricEventFixtureV1(stage, id string, seq int, details map[string]any) map[string]any {
	if details == nil {
		details = map[string]any{"schemaVersion": float64(1), "logicalStepId": id}
		if stage == "logical_step_closed" {
			details["assistantMessageAssembled"] = true
			details["llmMs"] = float64(4000)
			details["ttftMs"] = float64(200)
			details["decodeMs"] = float64(3800)
			details["decodeTokens"] = float64(20)
		}
	}
	return map[string]any{"kind": "pipeline_stage", "threadId": "thread", "turnId": "turn", "stage": stage, "details": details, "seq": seq, "timestamp": time.Date(2026, 10, 12, 0, 0, seq, 0, time.UTC).Format(time.RFC3339Nano)}
}
func TestConversationLogicalMetricsUseClosedAssembledFactsV1(t *testing.T) {
	id1, id2 := "f13e1680-e522-4d40-bc18-40dde3bb66ea", "f13e1680-e522-4d40-bc18-40dde3bb66eb"
	marker := logicalMetricEventFixtureV1("post_start", "", 1, map[string]any{"logicalStepMetricsVersion": float64(1)})
	start := logicalMetricEventFixtureV1("logical_step_started", id1, 2, nil)
	close := logicalMetricEventFixtureV1("logical_step_closed", id1, 3, nil)
	pending := logicalMetricEventFixtureV1("logical_step_started", id2, 4, nil)
	for _, test := range []struct {
		name         string
		events       []map[string]any
		turns, steps uint64
		known        bool
		llm, output  uint64
	}{
		{"empty", []map[string]any{marker}, 0, 0, true, 0, 0}, {"pending", []map[string]any{marker, start}, 0, 0, true, 0, 0},
		{"closed and pending", []map[string]any{marker, start, close, pending}, 1, 1, true, 4000, 20},
		{"replay duplicate", []map[string]any{marker, start, close, close, start}, 1, 1, true, 4000, 20},
		{"legacy", nil, 0, 0, false, 0, 0},
		{"unmarked facts", []map[string]any{start, close}, 0, 0, false, 4000, 20},
		{"mixed", []map[string]any{logicalMetricEventFixtureV1("post_start", "", 6, map[string]any{"maxModelSteps": float64(2)}), marker, start, close}, 0, 0, false, 4000, 20},
		{"unmatched close", []map[string]any{marker, close}, 0, 0, false, 0, 0},
		{"nonassembled", []map[string]any{marker, start, logicalMetricEventFixtureV1("logical_step_closed", id1, 3, map[string]any{"schemaVersion": float64(1), "logicalStepId": id1, "assistantMessageAssembled": false})}, 1, 1, true, 0, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := logicalStepMetricsV1("thread", "turn", test.events)
			if err != nil {
				t.Fatal(err)
			}
			if test.known {
				if got.TurnCount == nil || got.StepCount == nil || *got.TurnCount != test.turns || *got.StepCount != test.steps {
					t.Fatalf("closed counts: %#v", got)
				}
			} else if got.TurnCount != nil || got.StepCount != nil {
				t.Fatal("unknown counts fabricated")
			}
			if got.LLMMs.ReportedValue != test.llm || got.DecodeTokens != test.output {
				t.Fatalf("assembled metrics: %#v", got)
			}
			if test.name == "nonassembled" && got.LLMMs.KnownStepCount != 0 {
				t.Fatal("failed close invented known duration")
			}
		})
	}
	conflict := logicalMetricEventFixtureV1("logical_step_closed", id1, 3, nil)
	conflict["details"].(map[string]any)["decodeTokens"] = float64(21)
	got, err := logicalStepMetricsV1("thread", "turn", []map[string]any{marker, start, close, conflict})
	if err != nil || got.StepCount != nil || got.DecodeTokens != 0 {
		t.Fatalf("conflicting replay selected an arbitrary sample: %#v %v", got, err)
	}
	secondClose := logicalMetricEventFixtureV1("logical_step_closed", id2, 5, nil)
	got, err = logicalStepMetricsV1("thread", "turn", []map[string]any{marker, start, close, pending, secondClose})
	if err != nil || got.TurnCount == nil || *got.TurnCount != 1 || *got.StepCount != 2 || got.DecodeTokens != 40 {
		t.Fatalf("multi-step turn multiplied turns: %#v %v", got, err)
	}
	for _, badSeq := range []int{0, 1, 6} {
		bad := logicalMetricEventFixtureV1("logical_step_closed", id1, badSeq, nil)
		bad["details"].(map[string]any)["private"] = "PRIVATE"
		for _, records := range [][]map[string]any{{bad, marker, start, close, pending, secondClose}, {marker, start, close, pending, secondClose, bad}} {
			got, err := logicalStepMetricsV1("thread", "turn", records)
			if err != nil || got.TurnCount != nil || got.StepCount != nil || got.LLMMs.ReportedValue != 4000 || got.DecodeTokens != 20 {
				t.Fatalf("bad UUID record ordering changed independent samples: %#v err=%v", got, err)
			}
		}
	}

}
func TestConversationBillingRoutesRequireEveryAttemptAndScopeV1(t *testing.T) {
	known := appcache.ProviderBillingV1{InventoryComplete: true, AttemptCount: 1, Routes: []domaincache.ProviderPublicAttributionV1{{Provider: "deepseek", Model: "deepseek-chat"}}}
	unknown := appcache.ProviderBillingV1{InventoryComplete: true, AttemptCount: 1}
	for _, parts := range [][]appcache.ProviderBillingV1{{known, unknown, known}, {unknown, known}, {known, known}} {
		target := appcache.ProviderBillingV1{InventoryComplete: true}
		for _, part := range parts {
			if err := addConversationBillingV1(&target, part); err != nil {
				t.Fatal(err)
			}
		}
		want := len(parts) == 2 && len(parts[0].Routes) > 0
		if (len(target.Routes) > 0) != want || target.AttemptCount != uint64(len(parts)) {
			t.Fatalf("partial route revived or billing lost: %#v", target)
		}
	}
}

func TestConversationAssemblyDoesNotCountUsageOrBackfillLegacyV1(t *testing.T) {
	id := "f13e1680-e522-4d40-bc18-40dde3bb66ea"
	thread := map[string]any{"id": "thread", "turns": []any{map[string]any{"id": "turn", "threadId": "thread", "items": []any{}}}}
	marker := logicalMetricEventFixtureV1("post_start", "", 1, map[string]any{"logicalStepMetricsVersion": float64(1)})
	start := logicalMetricEventFixtureV1("logical_step_started", id, 2, nil)
	assembly := logicalMetricEventFixtureV1("logical_step_assembled", id, 3, nil)
	closed := logicalMetricEventFixtureV1("logical_step_closed", id, 4, nil)
	for _, history := range [][]map[string]any{nil, {marker, start}, {marker, start, closed}} {
		before, err := BuildConversationUsageV1(thread, history, nil)
		if err != nil {
			t.Fatal(err)
		}
		withAssembly := append(append([]map[string]any{}, history...), assembly, assembly)
		after, err := BuildConversationUsageV1(thread, withAssembly, nil)
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatalf("assembly changed billing/metrics or backfilled legacy facts: before=%#v after=%#v err=%v", before, after, err)
		}
	}
	legacy := []map[string]any{start, closed}
	before := append([]map[string]any{}, legacy...)
	projection, err := BuildConversationUsageV1(thread, legacy, nil)
	if err != nil || projection.Metrics.StepCount != nil || projection.Metrics.TurnCount != nil || !reflect.DeepEqual(legacy, before) {
		t.Fatal("legacy unknown coverage was fabricated or durable history mutated")
	}
	for _, event := range legacy {
		if event["stage"] == "logical_step_assembled" {
			t.Fatal("legacy history invented an assembly boundary")
		}
	}
}

type conversationUsageObserverFuncV1 func(context.Context, []domainsecurity.TurnSecurityContext) ([]appcache.TurnInventoryV1, error)

func (observe conversationUsageObserverFuncV1) ObserveTurnInventoriesV1(ctx context.Context, contexts []domainsecurity.TurnSecurityContext) ([]appcache.TurnInventoryV1, error) {
	return observe(ctx, contexts)
}

func TestObserveConversationUsagePreservesHistoricalScopeAndFailsClosed(t *testing.T) {
	const threadID, turnID = "thread-usage", "turn-compacted"
	frozen, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: t.TempDir(), ContextEpoch: 1,
		IssuedAt: time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	thread := map[string]any{"id": threadID, "turns": []any{}}
	committed := []domainsecurity.TurnSecurityContext{frozen, frozen}
	for _, test := range []struct {
		name      string
		snapshots []appcache.TurnInventoryV1
		err       error
		want      string
	}{
		{"observer error", nil, errors.New("private observer detail"), "conversation provider usage inventory is unavailable"},
		{"missing inventory", nil, nil, "conversation provider usage inventory is incomplete"},
		{"unsealed inventory", []appcache.TurnInventoryV1{{}}, nil, "conversation provider inventory scope is invalid or duplicated"},
	} {
		t.Run(test.name, func(t *testing.T) {
			observer := conversationUsageObserverFuncV1(func(_ context.Context, contexts []domainsecurity.TurnSecurityContext) ([]appcache.TurnInventoryV1, error) {
				if len(contexts) != 1 || contexts[0].ContextDigest != frozen.ContextDigest {
					t.Fatalf("committed historical scope lost or duplicated: %#v", contexts)
				}
				return test.snapshots, test.err
			})
			if _, err := ObserveConversationUsageV1(context.Background(), thread, nil, committed, observer); err == nil || err.Error() != test.want {
				t.Fatalf("inventory did not fail closed: %v", err)
			}
		})
	}
	if len(thread["turns"].([]any)) != 0 {
		t.Fatal("historical read view mutated the durable thread")
	}
	if _, err := ObserveConversationUsageV1(context.Background(), thread, nil, nil, nil); err == nil {
		t.Fatal("missing observer accepted")
	}
	invalid := map[string]any{"id": threadID, "turns": []any{map[string]any{"id": "other-turn", "securityContext": frozen}}}
	if _, err := ConversationUsageContextsV1(invalid, nil); err == nil {
		t.Fatal("foreign turn scope accepted")
	}
	thread["generalTerminalPublicationArchive"] = map[string]any{}
	if _, err := ConversationUsageHistoryV1(thread, nil, nil); err == nil {
		t.Fatal("invalid historical terminal membership accepted")
	}
}
