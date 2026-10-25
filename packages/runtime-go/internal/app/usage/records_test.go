package usage

import (
	"encoding/json"
	"math"
	"strconv"
	"testing"
	"time"
)

func TestParseWindowDateCountMatchesAcrossDST(t *testing.T) {
	window, err := ParseWindow(WindowParams{
		From:     "2026-03-08",
		To:       "2026-03-09",
		Timezone: "America/New_York",
		Label:    "daily usage",
	})
	if err != nil {
		t.Fatalf("parse window: %v", err)
	}
	if window.Days != 2 || window.From != "2026-03-08" || window.To != "2026-03-09" || window.Timezone != "America/New_York" {
		t.Fatalf("usage window should use date-only inclusive day count: %#v", window)
	}
}

func TestResponsesAggregateCacheSourcesAndProviders(t *testing.T) {
	records := []Record{
		{
			ThreadID:    "thread-a",
			Model:       "deepseek-chat",
			Provider:    "deepseek",
			CompletedAt: "2026-05-01T10:00:00Z",
			Usage: Snapshot{
				PromptTokens:       100,
				CompletionTokens:   10,
				TotalTokens:        110,
				CacheHitTokens:     80,
				CacheMissTokens:    20,
				HasCacheHitTokens:  true,
				HasCacheMissTokens: true,
				Turns:              1,
			},
		},
		{
			ThreadID:    "thread-a",
			Model:       "deepseek-chat",
			Provider:    "deepseek",
			CompletedAt: "2026-05-01T10:05:00Z",
			Usage: Snapshot{
				PromptTokens:       50,
				CompletionTokens:   5,
				ReasoningTokens:    2,
				TotalTokens:        55,
				CacheHitTokens:     30,
				CacheMissTokens:    20,
				HasCacheHitTokens:  true,
				HasCacheMissTokens: true,
				CacheMissReasons:   []string{"tool_catalog_changed"},
				CacheSuggestions:   []string{"Keep tool schemas stable."},
				Turns:              1,
			},
		},
		{
			ThreadID:    "thread-b",
			Model:       "gpt-4o",
			Provider:    "openai",
			CompletedAt: "2026-05-01T10:10:00Z",
			UsageSource: SourceSubagent,
			ChildRunID:  "job-1",
			Usage: Snapshot{
				PromptTokens:       20,
				CompletionTokens:   4,
				TotalTokens:        24,
				CacheHitTokens:     10,
				CacheMissTokens:    10,
				HasCacheHitTokens:  true,
				HasCacheMissTokens: true,
				Turns:              1,
			},
		},
	}

	threadResponse := ThreadResponse(records)
	threadA := bucketByStringField(t, threadResponse, "thread_id", "thread-a")
	if threadA["input_tokens"] != 150 || threadA["cache_hit_tokens"] != 110 || threadA["cache_miss_tokens"] != 40 || threadA["provider"] != "deepseek" {
		t.Fatalf("thread bucket mismatch: %#v", threadA)
	}
	if !containsAnyString(listAnyForTest(threadA["last_cache_miss_reasons"]), "tool_catalog_changed") ||
		!containsAnyString(listAnyForTest(threadA["last_cache_suggestions"]), "Keep tool schemas stable.") {
		t.Fatalf("latest cache diagnostics missing: %#v", threadA)
	}

	window := Window{From: "2026-05-01", To: "2026-05-01", Timezone: "UTC", Location: time.UTC, Days: 1}
	dayTotals := mapField(DailyResponse(records, window), "totals")
	if dayTotals["total_tokens"] != 189 || dayTotals["thread_count"] != 2 || dayTotals["active_days"] != 1 {
		t.Fatalf("daily totals mismatch: %#v", dayTotals)
	}
	modelResponse := ModelResponse(records, window)
	if bucketByStringField(t, modelResponse, "model", "deepseek-chat")["total_tokens"] != 165 {
		t.Fatalf("deepseek model bucket mismatch: %#v", modelResponse)
	}
	runtimeResponse := RuntimeResponse(records, []string{"thread-a", "thread-b", "thread-empty"})
	if len(listAnyForTest(runtimeResponse["perThread"])) != 3 {
		t.Fatalf("runtime response must include all listed threads: %#v", runtimeResponse)
	}
	subagent := sourceBucket(t, runtimeResponse, SourceSubagent)
	if !containsAnyString(listAnyForTest(subagent["childRunIds"]), "job-1") {
		t.Fatalf("subagent childRunIds missing: %#v", subagent)
	}
}

func TestCostCoverageKeepsKnownZeroCurrencyAndUnknownTurnsSeparate(t *testing.T) {
	if got := (Snapshot{Turns: 1, CostEstimateStatus: "none"}).Map()["costEstimateStatus"]; got != "unknown" {
		t.Fatalf("nonempty usage was marked as no cost attempts: %v", got)
	}
	records := []Record{
		{ThreadID: "usd", Model: "m", CompletedAt: "2026-05-01T10:00:00Z", Usage: Snapshot{
			Turns: 1, PriceConfigured: true, CostEstimateStatus: "complete", CostKnownCurrencies: []string{"USD"},
		}},
		{ThreadID: "cny", Model: "m", CompletedAt: "2026-05-01T10:01:00Z", Usage: Snapshot{
			Turns: 1, PriceConfigured: true, CostEstimateStatus: "complete", CostKnownCurrencies: []string{"CNY"},
		}},
		{ThreadID: "unknown", Model: "m", CompletedAt: "2026-05-02T10:00:00Z", Usage: Snapshot{Turns: 1}},
	}
	window := Window{From: "2026-05-01", To: "2026-05-03", Timezone: "UTC", Location: time.UTC, Days: 3}
	threads := ThreadResponse(records)
	usd := bucketByStringField(t, threads, "thread_id", "usd")
	cny := bucketByStringField(t, threads, "thread_id", "cny")
	if usd["cost_estimate_status"] != "complete" || !containsAnyString(listAnyForTest(usd["cost_known_currencies"]), "USD") ||
		cny["cost_estimate_status"] != "complete" || !containsAnyString(listAnyForTest(cny["cost_known_currencies"]), "CNY") {
		t.Fatalf("zero-priced currency provenance was lost: usd=%#v cny=%#v", usd, cny)
	}
	day := DailyResponse(records, window)
	first := bucketByStringField(t, day, "date", "2026-05-01")
	if first["cost_estimate_status"] != "complete" || len(listAnyForTest(first["cost_known_currencies"])) != 2 || first["cost_usd"] != float64(0) || first["cost_cny"] != float64(0) {
		t.Fatalf("mixed known zero currencies were not kept separate: %#v", first)
	}
	empty := bucketByStringField(t, day, "date", "2026-05-03")
	if empty["cost_estimate_status"] != "none" || empty["price_configured"] != false {
		t.Fatalf("empty day was not neutral: %#v", empty)
	}
	for _, aggregate := range []map[string]any{mapField(day, "totals"), mapField(ModelResponse(records, window), "totals"), mapField(RuntimeResponse(records, []string{"usd", "cny", "unknown"}), "total")} {
		status := aggregate["cost_estimate_status"]
		if status == nil {
			status = aggregate["costEstimateStatus"]
		}
		if status != "partial" {
			t.Fatalf("known plus unknown turns became complete: %#v", aggregate)
		}
		if aggregate["cost_usd"] != nil && aggregate["cost_usd"] != float64(0) ||
			aggregate["costUsd"] != nil && aggregate["costUsd"] != float64(0) {
			t.Fatalf("partial subtotal was published as a full cost: %#v", aggregate)
		}
	}
}

func bucketByStringField(t *testing.T, response map[string]any, field string, value string) map[string]any {
	t.Helper()
	for _, raw := range listAnyForTest(response["buckets"]) {
		bucket, _ := raw.(map[string]any)
		if bucket != nil && bucket[field] == value {
			return bucket
		}
	}
	t.Fatalf("usage bucket %s=%s missing in %#v", field, value, response)
	return nil
}

func sourceBucket(t *testing.T, response map[string]any, source string) map[string]any {
	t.Helper()
	for _, raw := range listAnyForTest(response["bySource"]) {
		bucket, _ := raw.(map[string]any)
		if bucket != nil && bucket["source"] == source {
			return bucket
		}
	}
	t.Fatalf("usage source %s missing in %#v", source, response)
	return nil
}

func mapField(record map[string]any, key string) map[string]any {
	value, _ := record[key].(map[string]any)
	if value == nil {
		return map[string]any{}
	}
	return value
}

func listAnyForTest(value any) []any {
	if items, ok := value.([]any); ok {
		return items
	}
	if items, ok := value.([]string); ok {
		out := make([]any, len(items))
		for index, item := range items {
			out[index] = item
		}
		return out
	}
	return []any{}
}

func containsAnyString(items []any, expected string) bool {
	for _, item := range items {
		if text, ok := item.(string); ok && text == expected {
			return true
		}
	}
	return false
}

func TestConversationProjectionDoesNotChangeLegacyCounters(t *testing.T) {
	legacy := []Record{{ThreadID: "thread-known", Model: "model", Provider: "provider", CompletedAt: "2026-07-14T00:00:00Z", Usage: Snapshot{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12, Turns: 1}}}
	projection := ConversationUsageV1{SchemaVersion: "conversation-usage.v1"}
	combined := append(append([]Record{}, legacy...), Record{ThreadID: "thread-open", ProjectionOnly: true, Conversation: &projection})
	if SumRecords(combined).TotalTokens != 12 || len(BySourceResponse(combined)) != len(BySourceResponse(legacy)) {
		t.Fatal("projection changed legacy usage")
	}
	response := ThreadResponse(combined)
	if response["totals"].(map[string]any)["thread_count"] != 1 || len(response["buckets"].([]any)) != 2 {
		t.Fatalf("legacy totals drift: %#v", response)
	}
	for _, raw := range response["buckets"].([]any) {
		row := raw.(map[string]any)
		if row["thread_id"] == "thread-open" && row["conversationUsage"] != &projection {
			t.Fatal("open usage projection was lost")
		}
	}
}

func TestSnapshotFromMapIntegerBounds(t *testing.T) {
	precise := int64(1<<53 + 1)
	preciseWant := 0
	if precise <= int64(math.MaxInt) {
		preciseWant = int(precise)
	}
	safeMaximum := int64(1<<53 - 1)
	safeWant := 0
	if safeMaximum <= int64(math.MaxInt) {
		safeWant = int(safeMaximum)
	}
	upper := -float64(math.MinInt)
	lastFloat := math.Trunc(math.Nextafter(upper, 0))
	for _, test := range []struct {
		name  string
		value any
		want  int
	}{
		{"int", int(7), 7},
		{"int64", int64(7), 7},
		{"float64", float64(7), 7},
		{"float32", float32(7), 7},
		{"json_integer", json.Number("7"), 7},
		{"zero", 0, 0},
		{"negative", int64(-1), 0},
		{"negative_float", -1.5, 0},
		{"fraction", 7.75, 7},
		{"float32_fraction", float32(7.75), 7},
		{"json_fraction", json.Number("7.75"), 7},
		{"json_exponent", json.Number("7e1"), 70},
		{"json_fraction_exponent", json.Number("7.75e1"), 77},
		{"safe_maximum", safeMaximum, safeWant},
		{"precise_int64", precise, preciseWant},
		{"precise_json", json.Number(strconv.FormatInt(precise, 10)), preciseWant},
		{"host_maximum_int", math.MaxInt, math.MaxInt},
		{"host_maximum_int64", int64(math.MaxInt), math.MaxInt},
		{"host_maximum_json", json.Number(strconv.Itoa(math.MaxInt)), math.MaxInt},
		{"last_host_float", lastFloat, int(lastFloat)},
		{"host_int_upper", upper, 0},
		{"host_int_upper_json", json.Number(strconv.FormatUint(uint64(math.MaxInt)+1, 10)), 0},
		{"nan", math.NaN(), 0},
		{"positive_infinity", math.Inf(1), 0},
		{"negative_infinity", math.Inf(-1), 0},
		{"huge", 1e100, 0},
		{"huge_json_exponent", json.Number("1e100"), 0},
		{"nil", nil, 0},
		{"string", "7", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := SnapshotFromMap(map[string]any{"promptTokens": test.value, "totalTokens": test.value})
			if snapshot.PromptTokens != test.want || snapshot.TotalTokens != test.want {
				t.Fatalf("usage integer(%v) = prompt %d, total %d; want %d", test.value, snapshot.PromptTokens, snapshot.TotalTokens, test.want)
			}
		})
	}
}
