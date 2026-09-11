//go:build !analytix_prod

package livelocal

import (
	"errors"
	"strconv"
	"strings"
	"time"

	contracts "analytix.local/runtime-go/internal/contracts"
)

func RunDurableReplayContract(provider map[string]any, store *DurableEventSessionStore) (map[string]any, error) {
	if store == nil {
		return nil, errors.New("durable store is required")
	}
	if provider == nil {
		return nil, errors.New("provider contract result is required")
	}
	// Ordinary-history privacy projection treats long digit runs as possible
	// account identifiers. Keep the unique conformance fixture ID visibly
	// host-generated without creating a PII-shaped decimal timestamp.
	stamp := strconv.FormatInt(time.Now().UnixNano(), 36)
	threadID := "thr_go_contract_" + strings.Join(strings.Split(stamp, ""), "x")
	turnID := "turn_go_contract"
	drafts := []map[string]any{
		{"threadId": threadID, "kind": "turn_started", "turnId": turnID, "status": "running"},
		{"threadId": threadID, "kind": "pipeline_stage", "turnId": turnID, "stage": "response_received"},
		{"threadId": threadID, "kind": "usage", "turnId": turnID, "usage": map[string]any{
			"provider":          "deepseek",
			"cacheHitTokens":    provider["deepseekCacheHitTokens"],
			"cacheMissTokens":   provider["deepseekCacheMissTokens"],
			"cacheHitRate":      provider["deepseekCacheHitRate"],
			"endpointFormat":    "chat_completions",
			"liveLocalProvider": true,
		}},
		{"threadId": threadID, "kind": "turn_completed", "turnId": turnID, "status": "completed"},
	}
	orders := [][]string{}
	if err := ensureFixtureThread(store, threadID); err != nil {
		return nil, err
	}
	for _, draft := range drafts {
		_, order, err := store.RecordEvent(draft)
		if err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}
	replay, err := store.LoadEventsSince(threadID, 0)
	if err != nil {
		return nil, err
	}
	recovered, err := store.RecoveredState(threadID)
	if err != nil {
		return nil, err
	}
	highestSeq, err := store.HighestSeq(threadID)
	if err != nil {
		return nil, err
	}
	eventKinds := contracts.EventKinds(replay.Events)
	return map[string]any{
		"runtimeGoContractParitySlice":  true,
		"durableReplayFromEventSink":    true,
		"threadId":                      threadID,
		"eventCount":                    len(replay.Events),
		"highestSeq":                    highestSeq,
		"eventKinds":                    eventKinds,
		"recoveredState":                recovered,
		"usageCacheAccountingPersisted": contracts.ContainsString(eventKinds, "usage"),
		"allPersistBeforePublish":       contracts.AllPersistBeforePublish(orders),
		"sseReplayFrameCount":           len(replay.Events),
		"diagnostics":                   replay.Diagnostics,
	}, nil
}
