package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	usageindexfs "analytix.local/runtime-go/internal/adapters/outbound/usageindexfs"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	appturn "analytix.local/runtime-go/internal/app/turn"
	usageapp "analytix.local/runtime-go/internal/app/usage"
)

func TestUsageHeldThreadReadRemainsUnavailableAtPublicBoundary(t *testing.T) {
	root := t.TempDir()
	original, err := NewTempDurableEventSessionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, title := range []string{"Held usage", "Independent usage"} {
		thread, err := original.CreateThread(map[string]any{"title": title}, "")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, stringField(thread, "id"))
	}
	held, independent := ids[0], ids[1]
	for _, id := range ids {
		recordUsageEvent(t, &runtimeServerHandler{store: original}, id, "turn_usage_1", "2026-05-01T10:00:00Z", "synthetic", "synthetic", map[string]any{"totalTokens": 3, "turns": 1})
	}
	if err := original.EnsureUsageIndex(); err != nil {
		t.Fatal(err)
	}
	preserved, err := eventlog.PrepareSemanticRestartPreservationV1(context.Background(), root, original.threadSummaryIndex.Path(), []string{held})
	if err != nil {
		t.Fatal(err)
	}
	store, err := newDurableEventSessionStoreWithPreservationV1(root, durableStoreModeTemp, false, preserved)
	if err != nil {
		t.Fatal(err)
	}
	handler := &runtimeServerHandler{store: store}
	before := runtimeRestoreFileDigestsV1(t, root)
	if _, _, err := (runtimeSubagentCompletionDriver{handler: handler}).TurnCompletion(held, "turn_usage_1"); !errors.Is(err, casethreadapp.ErrRestartPreserved) {
		t.Errorf("held original primary became child settlement input: %v", err)
	}
	if records, err := handler.runtimeUsageService().Records(held); records != nil || !errors.Is(err, usageindexfs.ErrRestartPreserved) {
		t.Errorf("held usage read became empty success: records=%v err=%v", records, err)
	}
	if thread, err := handler.runtimeThreadService().Get(held); thread != nil || !errors.Is(err, eventlog.ErrRestartPreserved) {
		t.Errorf("held event frontier became successful thread summary: returned=%v err=%v", thread != nil, err)
	}
	if diagnostics, err := (runtimeSubagentCompletionDriver{handler: handler}).CacheDiagnostics(held, "turn_usage_1"); diagnostics != nil || !errors.Is(err, eventlog.ErrRestartPreserved) {
		t.Errorf("held event read became empty cache diagnostics: returned=%v err=%v", diagnostics != nil, err)
	}
	if _, err := (runtimeSubagentCompletionDriver{handler: handler}).CacheDiagnostics(independent, "turn_usage_1"); err != nil {
		t.Fatalf("independent cache observation failed: %v", err)
	}
	summary := httptest.NewRecorder()
	handler.handleThreadRecord(summary, httptest.NewRequest(http.MethodGet, "/v1/threads/"+held, nil), held)
	if summary.Code < 400 || summary.Code == http.StatusNotFound {
		t.Errorf("held summary returned success or missing: status=%d", summary.Code)
	}
	for _, id := range []string{held, independent, "thread_missing_usage"} {
		recorder := httptest.NewRecorder()
		handler.handleUsage(recorder, httptest.NewRequest(http.MethodGet, "/v1/usage?group_by=thread&thread_id="+id, nil))
		if id == held {
			if recorder.Code < 400 || recorder.Code == http.StatusNotFound {
				t.Errorf("held usage returned successful empty or missing projection: status=%d", recorder.Code)
			}
		} else if recorder.Code != http.StatusOK {
			t.Errorf("ordinary/missing usage control failed: id=%s status=%d", id, recorder.Code)
		}
	}
	if !reflect.DeepEqual(before, runtimeRestoreFileDigestsV1(t, root)) {
		t.Fatal("usage observation changed held source")
	}
}

func TestUsagePrimaryReadErrorDoesNotBecomeMissing(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{"title": "Synthetic unreadable primary"}, "")
	if err != nil {
		t.Fatal(err)
	}
	id := stringField(thread, "id")
	if err := os.WriteFile(store.threadPath(id), []byte(`{"id":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetThread(id); err == nil {
		t.Fatal("fixture primary must fail its actual read")
	}
	before := runtimeRestoreFileDigestsV1(t, store.root)
	if present, err := (runtimeUsageRepository{store: store}).ThreadExists(id); present || err == nil {
		t.Errorf("primary read error became missing: present=%v err=%v", present, err)
	}
	handler := &runtimeServerHandler{store: store}
	recorder := httptest.NewRecorder()
	handler.handleUsage(recorder, httptest.NewRequest(http.MethodGet, "/v1/usage?group_by=thread&thread_id="+id, nil))
	if recorder.Code < 400 || recorder.Code == http.StatusNotFound {
		t.Errorf("primary read error returned success or missing usage: status=%d", recorder.Code)
	}
	if present, err := (runtimeUsageRepository{store: store}).ThreadExists("thread_missing_usage"); present || err != nil {
		t.Fatalf("ordinary missing control changed: present=%v err=%v", present, err)
	}
	if !reflect.DeepEqual(before, runtimeRestoreFileDigestsV1(t, store.root)) {
		t.Fatal("failed usage observation changed its source")
	}
}

func TestRuntimeTurnSequenceAcceptsProductAndLegacyTurnIDs(t *testing.T) {
	for _, tc := range []struct {
		id  string
		seq int
		ok  bool
	}{
		{id: "turn_1", seq: 1, ok: true},
		{id: "turn_42", seq: 42, ok: true},
		{id: "turn_d0242_7", seq: 7, ok: true},
		{id: "turn_d0242_bad", ok: false},
		{id: "item_turn_1", ok: false},
	} {
		seq, ok := appturn.SequenceID(tc.id)
		if seq != tc.seq || ok != tc.ok {
			t.Fatalf("SequenceID(%q) = (%d, %v), want (%d, %v)", tc.id, seq, ok, tc.seq, tc.ok)
		}
	}
}

func TestUsageAggregationCoversDeltaCumulativeOldCacheAndMixedProviderModel(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   "usage-token",
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		Insecure:       true,
	})
	runtime, ok := handler.(*runtimeServerHandler)
	if !ok {
		t.Fatalf("handler type mismatch: %T", handler)
	}

	deepseekThread, err := runtime.store.CreateThread(map[string]any{
		"title":      "Go delta usage",
		"workspace":  "/tmp/usage",
		"model":      "deepseek-chat",
		"providerId": "deepseek",
	}, runtime.dataDir)
	if err != nil {
		t.Fatalf("create deepseek thread: %v", err)
	}
	deepseekID := stringField(deepseekThread, "id")
	recordUsageEvent(t, runtime, deepseekID, "turn_delta_1", "2026-05-01T10:00:00Z", "deepseek-chat", "deepseek", map[string]any{
		"promptTokens":     100,
		"completionTokens": 10,
		"reasoningTokens":  4,
		"totalTokens":      110,
		"cacheHitTokens":   80,
		"cacheMissTokens":  20,
		"cacheHitRate":     0.8,
		"turns":            1,
	})
	recordUsageEvent(t, runtime, deepseekID, "turn_delta_2", "2026-05-01T10:05:00Z", "deepseek-chat", "deepseek", map[string]any{
		"promptTokens":           50,
		"completionTokens":       5,
		"reasoningTokens":        2,
		"totalTokens":            55,
		"cacheHitTokens":         30,
		"cacheMissTokens":        20,
		"cacheHitRate":           0.6,
		"cacheableTokenHitRate":  0.6,
		"totalInputTokenHitRate": 0.6,
		"cacheMissReasons":       []string{"tool_catalog_changed"},
		"cacheSuggestions":       []string{"Keep MCP and Skill tools stable within a thread."},
		"turns":                  1,
	})

	openAIThread, err := runtime.store.CreateThread(map[string]any{
		"title":      "Kun cumulative usage",
		"workspace":  "/tmp/usage",
		"model":      "gpt-4o",
		"providerId": "openai",
	}, runtime.dataDir)
	if err != nil {
		t.Fatalf("create openai thread: %v", err)
	}
	openAIID := stringField(openAIThread, "id")
	recordUsageEvent(t, runtime, openAIID, "turn_cumulative_1", "2026-05-01T11:00:00Z", "gpt-4o", "openai", map[string]any{
		"promptTokens":     10,
		"completionTokens": 1,
		"totalTokens":      11,
		"cachedTokens":     5,
		"cacheHitRate":     nil,
		"turns":            1,
	})
	recordUsageEvent(t, runtime, openAIID, "turn_cumulative_2", "2026-05-01T11:05:00Z", "gpt-4o", "openai", map[string]any{
		"promptTokens":     30,
		"completionTokens": 3,
		"reasoningTokens":  1,
		"totalTokens":      33,
		"cacheHitTokens":   20,
		"cacheMissTokens":  10,
		"cacheHitRate":     20.0 / 30.0,
		"turns":            2,
	})

	records, err := runtime.runtimeUsageService().Records("")
	if err != nil {
		t.Fatalf("load usage records: %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("usage records should preserve two Go delta and two Kun cumulative-derived records: %#v", records)
	}

	threadResponse := usageapp.ThreadResponse(records)
	deepseekBucket := usageBucketByStringField(t, threadResponse, "thread_id", deepseekID)
	if deepseekBucket["input_tokens"] != 150 ||
		deepseekBucket["output_tokens"] != 15 ||
		deepseekBucket["reasoning_tokens"] != 6 ||
		deepseekBucket["cached_tokens"] != 110 ||
		deepseekBucket["cache_hit_tokens"] != 110 ||
		deepseekBucket["cache_miss_tokens"] != 40 ||
		deepseekBucket["total_tokens"] != 165 ||
		deepseekBucket["turns"] != 2 ||
		deepseekBucket["provider"] != "deepseek" {
		t.Fatalf("deepseek delta bucket mismatch: %#v", deepseekBucket)
	}
	if rate, ok := deepseekBucket["last_turn_cache_hit_rate"].(float64); !ok || rate != 0.6 {
		t.Fatalf("deepseek last turn cache rate mismatch: %#v", deepseekBucket)
	}
	if rate, ok := deepseekBucket["last_turn_cacheable_hit_rate"].(float64); !ok || rate != 0.6 {
		t.Fatalf("deepseek last turn cacheable rate mismatch: %#v", deepseekBucket)
	}
	if rate, ok := deepseekBucket["last_turn_total_input_hit_rate"].(float64); !ok || rate != 0.6 {
		t.Fatalf("deepseek last turn total input rate mismatch: %#v", deepseekBucket)
	}
	if !containsAnyString(listAny(deepseekBucket["last_cache_miss_reasons"]), "tool_catalog_changed") ||
		!containsAnyString(listAny(deepseekBucket["last_cache_suggestions"]), "Keep MCP and Skill tools stable within a thread.") {
		t.Fatalf("deepseek latest cache diagnostics missing: %#v", deepseekBucket)
	}
	openAIBucket := usageBucketByStringField(t, threadResponse, "thread_id", openAIID)
	if openAIBucket["input_tokens"] != 30 ||
		openAIBucket["output_tokens"] != 3 ||
		openAIBucket["reasoning_tokens"] != 1 ||
		openAIBucket["cached_tokens"] != 20 ||
		openAIBucket["cache_hit_tokens"] != 20 ||
		openAIBucket["cache_miss_tokens"] != 10 ||
		openAIBucket["total_tokens"] != 33 ||
		openAIBucket["turns"] != 2 ||
		openAIBucket["provider"] != "openai" {
		t.Fatalf("openai cumulative bucket mismatch: %#v", openAIBucket)
	}
	if rate, ok := openAIBucket["cache_hit_rate"].(float64); !ok || rate != 20.0/30.0 {
		t.Fatalf("old cachedTokens-only event must not be guessed as cache hit telemetry: %#v", openAIBucket)
	}
	if boolField(deepseekBucket, "price_configured") || boolField(openAIBucket, "price_configured") {
		t.Fatalf("unpriced usage buckets must not pretend provider pricing is configured: deepseek=%#v openai=%#v", deepseekBucket, openAIBucket)
	}

	window := usageapp.Window{From: "2026-05-01", To: "2026-05-01", Timezone: "UTC", Location: time.UTC, Days: 1}
	dayResponse := usageapp.DailyResponse(records, window)
	dayTotals := mapFieldAny(dayResponse, "totals")
	if dayTotals["total_tokens"] != 198 ||
		dayTotals["thread_count"] != 2 ||
		dayTotals["active_days"] != 1 {
		t.Fatalf("daily usage totals mismatch: %#v", dayResponse)
	}
	modelResponse := usageapp.ModelResponse(records, window)
	deepseekModel := usageBucketByStringField(t, modelResponse, "model", "deepseek-chat")
	openAIModel := usageBucketByStringField(t, modelResponse, "model", "gpt-4o")
	if deepseekModel["provider"] != "deepseek" || deepseekModel["total_tokens"] != 165 {
		t.Fatalf("deepseek model bucket mismatch: %#v", deepseekModel)
	}
	if openAIModel["provider"] != "openai" || openAIModel["total_tokens"] != 33 {
		t.Fatalf("openai model bucket mismatch: %#v", openAIModel)
	}
}

func TestUsageWindowDateCountMatchesKunAcrossDST(t *testing.T) {
	window, err := usageapp.ParseWindow(usageapp.WindowParams{
		From:     "2026-03-08",
		To:       "2026-03-09",
		Timezone: "America/New_York",
		Label:    "daily usage",
	})
	if err != nil {
		t.Fatalf("usage window should parse across DST: %v", err)
	}
	if window.Days != 2 || window.From != "2026-03-08" || window.To != "2026-03-09" || window.Timezone != "America/New_York" {
		t.Fatalf("usage window should use date-only inclusive day count: %#v", window)
	}
}

func TestUsageRuntimeResponseAttributesSubagentSource(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   "usage-token",
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		Insecure:       true,
	})
	runtime, ok := handler.(*runtimeServerHandler)
	if !ok {
		t.Fatalf("handler type mismatch: %T", handler)
	}
	thread, err := runtime.store.CreateThread(map[string]any{
		"title":      "Subagent usage",
		"workspace":  "/tmp/usage",
		"model":      "deepseek-chat",
		"providerId": "deepseek",
	}, runtime.dataDir)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	threadID := stringField(thread, "id")
	recordUsageEvent(t, runtime, threadID, "turn_parent", "2026-05-01T10:00:00Z", "deepseek-chat", "deepseek", map[string]any{
		"promptTokens":     12,
		"completionTokens": 3,
		"totalTokens":      15,
		"turns":            1,
	})
	_, _, err = runtime.store.RecordEvent(map[string]any{
		"kind":        "usage",
		"threadId":    threadID,
		"turnId":      "turn_child",
		"timestamp":   "2026-05-01T10:01:00Z",
		"model":       "deepseek-chat",
		"usageSource": "subagent",
		"childRunId":  "job-1",
		"usage": map[string]any{
			"promptTokens":     20,
			"completionTokens": 4,
			"totalTokens":      24,
			"cacheHitTokens":   10,
			"cacheMissTokens":  10,
			"turns":            1,
		},
		"cacheDiagnostics": map[string]any{"providerId": "deepseek"},
	})
	if err != nil {
		t.Fatalf("record subagent usage event: %v", err)
	}
	usageService := runtime.runtimeUsageService()
	records, err := usageService.Records("")
	if err != nil {
		t.Fatalf("load usage records: %v", err)
	}
	response, err := usageService.RuntimeResponse(records)
	if err != nil {
		t.Fatal(err)
	}
	parent := usageSourceBucket(t, response, "turn")
	if mapFieldAny(parent, "usage")["totalTokens"] != 15 {
		t.Fatalf("parent source usage mismatch: %#v", parent)
	}
	subagent := usageSourceBucket(t, response, "subagent")
	usage := mapFieldAny(subagent, "usage")
	if usage["totalTokens"] != 24 || usage["cacheHitTokens"] != 10 {
		t.Fatalf("subagent source usage/cache mismatch: %#v", subagent)
	}
	if !containsAnyString(listAny(subagent["childRunIds"]), "job-1") {
		t.Fatalf("subagent source should retain childRunId: %#v", subagent)
	}
}

func TestUsageEventsIndexBackfillAvoidsPerRequestThreadEventReplay(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   "usage-token",
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		Insecure:       true,
	})
	runtime, ok := handler.(*runtimeServerHandler)
	if !ok {
		t.Fatalf("handler type mismatch: %T", handler)
	}

	for threadIndex := 0; threadIndex < 25; threadIndex++ {
		thread, err := runtime.store.CreateThread(map[string]any{
			"title":      "Large history usage",
			"workspace":  "/tmp/usage",
			"model":      "deepseek-chat",
			"providerId": "deepseek",
		}, runtime.dataDir)
		if err != nil {
			t.Fatalf("create thread %d: %v", threadIndex, err)
		}
		threadID := stringField(thread, "id")
		for eventIndex := 0; eventIndex < 40; eventIndex++ {
			event := map[string]any{
				"seq":       float64(eventIndex + 1),
				"kind":      "usage",
				"threadId":  threadID,
				"turnId":    "turn_large",
				"timestamp": "2026-05-01T10:00:00Z",
				"model":     "deepseek-chat",
				"usage": map[string]any{
					"promptTokens":     10,
					"completionTokens": 1,
					"totalTokens":      11,
					"cacheHitTokens":   7,
					"cacheMissTokens":  3,
					"turns":            1,
				},
				"cacheDiagnostics": map[string]any{
					"providerId": "deepseek",
				},
			}
			line, _ := json.Marshal(event)
			if err := runtime.store.AppendRawEventLine(threadID, string(line)); err != nil {
				t.Fatalf("append raw usage event: %v", err)
			}
		}
	}

	if err := runtime.store.EnsureUsageIndex(); err != nil {
		t.Fatalf("backfill usage index: %v", err)
	}
	eventReplayReadsAfterBackfill := runtime.store.EventReplayReadCount()
	usageIndexReadsBefore := runtime.store.UsageIndexReadCount()
	records, err := runtime.runtimeUsageService().Records("")
	if err != nil {
		t.Fatalf("usage records: %v", err)
	}
	if len(records) != 1000 {
		t.Fatalf("expected indexed usage records for large history, got %d", len(records))
	}
	if runtime.store.EventReplayReadCount() != eventReplayReadsAfterBackfill {
		t.Fatalf("usage request replayed thread events: before=%d after=%d", eventReplayReadsAfterBackfill, runtime.store.EventReplayReadCount())
	}
	if runtime.store.UsageIndexReadCount() != usageIndexReadsBefore+1 {
		t.Fatalf("usage request should read usage_events index exactly once")
	}
}

func TestUsageIndexBackfillHandlesLargeDurableEventLines(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   "usage-large-line-token",
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		Insecure:       true,
	})
	runtime, ok := handler.(*runtimeServerHandler)
	if !ok {
		t.Fatalf("handler type mismatch: %T", handler)
	}
	thread, err := runtime.store.CreateThread(map[string]any{
		"title":      "Large JSONL line usage",
		"workspace":  "/tmp/usage-large-line",
		"model":      "deepseek-chat",
		"providerId": "deepseek",
	}, runtime.dataDir)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	threadID := stringField(thread, "id")
	largeText := strings.Repeat("large-token-", 32*1024)
	largeEvent := map[string]any{
		"seq":       1,
		"kind":      "pipeline_stage",
		"threadId":  threadID,
		"turnId":    "turn_large_line",
		"timestamp": "2026-05-01T10:00:00Z",
		"stage":     "response_received",
		"details":   map[string]any{"padding": largeText},
	}
	line, err := json.Marshal(largeEvent)
	if err != nil {
		t.Fatalf("marshal large event: %v", err)
	}
	if len(line) <= 64*1024 {
		t.Fatalf("test event must exceed bufio.Scanner default token limit, got %d bytes", len(line))
	}
	if err := runtime.store.AppendRawEventLine(threadID, string(line)); err != nil {
		t.Fatalf("append large event line: %v", err)
	}
	usageLine, _ := json.Marshal(map[string]any{
		"seq":       2,
		"kind":      "usage",
		"threadId":  threadID,
		"turnId":    "turn_large_line",
		"timestamp": "2026-05-01T10:00:01Z",
		"model":     "deepseek-chat",
		"usage": map[string]any{
			"promptTokens":     12,
			"completionTokens": 3,
			"totalTokens":      15,
			"cacheHitTokens":   9,
			"cacheMissTokens":  3,
			"turns":            1,
		},
	})
	if err := runtime.store.AppendRawEventLine(threadID, string(usageLine)); err != nil {
		t.Fatalf("append usage event line: %v", err)
	}

	if err := runtime.store.EnsureUsageIndex(); err != nil {
		t.Fatalf("backfill usage index with large event line: %v", err)
	}
	replay, err := runtime.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatalf("load replay with large event line: %v", err)
	}
	if len(replay.Events) != 2 {
		t.Fatalf("large event replay count mismatch: %d", len(replay.Events))
	}
	firstDetails, _ := replay.Events[0]["details"].(map[string]any)
	if stringField(replay.Events[0], "kind") != "pipeline_stage" || stringField(firstDetails, "padding") != largeText {
		t.Fatalf("large event replay mismatch: kind=%q paddingBytes=%d", stringField(replay.Events[0], "kind"), len(stringField(firstDetails, "padding")))
	}
	records, err := runtime.runtimeUsageService().Records(threadID)
	if err != nil {
		t.Fatalf("usage records: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one usage record after large-line backfill, got %d", len(records))
	}
}

func TestUsageIndexBackfillDoesNotBlockConcurrentUsageEventWrites(t *testing.T) {
	store, err := NewProductionDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("create production store: %v", err)
	}
	runtime := &runtimeServerHandler{store: store, dataDir: t.TempDir()}
	thread, err := store.CreateThread(map[string]any{
		"title":      "Concurrent usage index",
		"workspace":  "/tmp/usage-concurrent",
		"model":      "deepseek-chat",
		"providerId": "deepseek",
	}, runtime.dataDir)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	threadID := stringField(thread, "id")
	recordUsageEvent(t, runtime, threadID, "turn_before_backfill", "2026-05-01T10:00:00Z", "deepseek-chat", "deepseek", map[string]any{
		"promptTokens":     10,
		"completionTokens": 2,
		"totalTokens":      12,
		"turns":            1,
	})

	scanStarted := make(chan struct{}, 1)
	releaseScan := make(chan struct{})
	store.beforeUsageIndexThreadHook = func(id string) {
		if id != threadID {
			return
		}
		select {
		case scanStarted <- struct{}{}:
			<-releaseScan
		default:
		}
	}
	backfillDone := make(chan error, 1)
	go func() {
		backfillDone <- store.EnsureUsageIndex()
	}()
	select {
	case <-scanStarted:
	case <-time.After(time.Second):
		t.Fatalf("usage index backfill did not reach thread scan")
	}

	writeEnteredOwner := make(chan struct{}, 1)
	store.beforeRecordEventHook = func(event map[string]any) error {
		if stringField(event, "turnId") == "turn_during_backfill" {
			select {
			case writeEnteredOwner <- struct{}{}:
			default:
			}
		}
		return nil
	}
	writeDone := make(chan error, 1)
	go func() {
		_, _, err := store.RecordEvent(map[string]any{
			"kind":      "usage",
			"threadId":  threadID,
			"turnId":    "turn_during_backfill",
			"timestamp": "2026-05-01T10:01:00Z",
			"model":     "deepseek-chat",
			"usage": map[string]any{
				"promptTokens":     20,
				"completionTokens": 4,
				"totalTokens":      24,
				"turns":            1,
			},
			"cacheDiagnostics": map[string]any{
				"providerId": "deepseek",
				"model":      "deepseek-chat",
			},
		})
		writeDone <- err
	}()
	ownerAcquiredDuringScan := false
	select {
	case <-writeEnteredOwner:
		ownerAcquiredDuringScan = true
	case <-time.After(15 * time.Second):
	}
	close(releaseScan)
	var writeErr error
	writeFinished := false
	select {
	case err := <-writeDone:
		writeErr = err
		writeFinished = true
	case <-time.After(15 * time.Second):
	}
	var backfillErr error
	backfillFinished := false
	select {
	case err := <-backfillDone:
		backfillErr = err
		backfillFinished = true
	case <-time.After(15 * time.Second):
	}
	if !ownerAcquiredDuringScan {
		t.Fatal("concurrent usage event did not acquire the owner lock while usage index backfill was scanning")
	}
	if !writeFinished {
		t.Fatal("concurrent usage event did not finish after releasing the backfill scan")
	}
	if writeErr != nil {
		t.Fatalf("record concurrent usage event: %v", writeErr)
	}
	if !backfillFinished {
		t.Fatal("usage index backfill did not finish after releasing its scan")
	}
	if backfillErr != nil {
		t.Fatalf("backfill usage index: %v", backfillErr)
	}

	records, err := store.LoadUsageIndexRecords(threadID)
	if err != nil {
		t.Fatalf("load usage records: %v", err)
	}
	turnIDs := map[string]bool{}
	for _, record := range records {
		turnIDs[record.TurnID] = true
	}
	if !turnIDs["turn_before_backfill"] || !turnIDs["turn_during_backfill"] {
		t.Fatalf("usage index should include pre-existing and concurrent usage events, got %#v", records)
	}
}

func recordUsageEvent(
	t *testing.T,
	runtime *runtimeServerHandler,
	threadID string,
	turnID string,
	timestamp string,
	model string,
	providerID string,
	usage map[string]any,
) {
	t.Helper()
	_, _, err := runtime.store.RecordEvent(map[string]any{
		"kind":      "usage",
		"threadId":  threadID,
		"turnId":    turnID,
		"timestamp": timestamp,
		"model":     model,
		"usage":     usage,
		"cacheDiagnostics": map[string]any{
			"providerId": providerID,
			"model":      model,
		},
	})
	if err != nil {
		t.Fatalf("record usage event: %v", err)
	}
}

func usageBucketByStringField(t *testing.T, response map[string]any, field string, value string) map[string]any {
	t.Helper()
	for _, raw := range listAny(response["buckets"]) {
		bucket, _ := raw.(map[string]any)
		if bucket != nil && stringField(bucket, field) == value {
			return bucket
		}
	}
	t.Fatalf("usage bucket %s=%s missing in %#v", field, value, response)
	return nil
}

func usageSourceBucket(t *testing.T, response map[string]any, source string) map[string]any {
	t.Helper()
	for _, raw := range listAny(response["bySource"]) {
		bucket, _ := raw.(map[string]any)
		if bucket != nil && stringField(bucket, "source") == source {
			return bucket
		}
	}
	t.Fatalf("usage source %s missing in %#v", source, response)
	return nil
}

func containsAnyString(items []any, expected string) bool {
	for _, item := range items {
		if text, ok := item.(string); ok && text == expected {
			return true
		}
	}
	return false
}

func mapFieldAny(record map[string]any, key string) map[string]any {
	value, _ := record[key].(map[string]any)
	if value == nil {
		return map[string]any{}
	}
	return value
}
