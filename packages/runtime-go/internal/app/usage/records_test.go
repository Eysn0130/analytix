package usage

import (
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
