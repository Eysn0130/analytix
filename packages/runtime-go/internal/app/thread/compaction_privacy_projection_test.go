package thread

import (
	"strings"
	"testing"

	domainevent "analytix.local/runtime-go/internal/domain/event"
)

func TestOrdinaryCompactionProjectionPreservesClosedReasoningExclusionProof(t *testing.T) {
	item := map[string]any{
		"id": "compaction_test", "turnId": "turn_compaction", "threadId": "thread_compaction",
		"role": "system", "status": "completed", "kind": "compaction",
		"summary": "Prior conversation compacted.", "replacedTokens": float64(12), "auto": false,
		"pinnedConstraints": []any{"user: preserve recent turns"},
		"sourceDigest":      "17c1a04c4aa4d01077a6d5818030f42c89cdcd3f034e7c18fdb20ec827e133ef",
		"digestMarker":      "sha256:17c1a04c4aa4", "sourceItemIds": []any{"item_safe"},
		"schemaVersion": float64(3), "reasoningExcluded": true, "assistantProseExcluded": true,
		"toolPayloadsExcluded": true, "caseFactsExcluded": true, "providerHistoryProjectionVersion": float64(1),
	}
	item["reasoningExclusionProof"] = domainevent.ReasoningExclusionProof(item)
	thread := map[string]any{
		"id": "thread_compaction", "status": "idle", "turns": []any{map[string]any{
			"id": "turn_compaction", "threadId": "thread_compaction", "items": []any{item},
		}},
	}

	projected, err := ProjectPublicThread(thread)
	if err != nil {
		t.Fatal(err)
	}
	turn := projected["turns"].([]any)[0].(map[string]any)
	publicItem := turn["items"].([]any)[0].(map[string]any)
	if err := domainevent.ValidatePublicRecord(publicItem); err != nil {
		t.Fatalf("projected compaction proof is not self-consistent: %#v err=%v", publicItem, err)
	}
	if publicItem["reasoningExcluded"] != true || publicItem["assistantProseExcluded"] != true ||
		publicItem["toolPayloadsExcluded"] != true || publicItem["caseFactsExcluded"] != true ||
		publicItem["providerHistoryProjectionVersion"] != float64(1) {
		t.Fatalf("projected compaction omitted closed exclusion metadata: %#v", publicItem)
	}
}

func TestOrdinaryCompactionCompletedEventRequiresExactDurableItemProof(t *testing.T) {
	thread, event := ordinaryCompactionCompletedFixture(t)

	projected, visible, err := ProjectPublicThreadEvent("thread_compaction", thread, event)
	if err != nil || !visible {
		t.Fatalf("valid compaction completion was hidden: %#v visible=%t err=%v", projected, visible, err)
	}
	if projected["reasoningExcluded"] != true || projected["schemaVersion"] != float64(2) ||
		projected["reasoningExclusionProof"] != event["reasoningExclusionProof"] {
		t.Fatalf("compaction completion omitted its closed exclusion proof: %#v", projected)
	}
	if err := domainevent.ValidatePublicRecord(projected); err != nil {
		t.Fatalf("projected compaction completion is not public-safe: %v", err)
	}
}

func TestOrdinaryCompactionCompletedEventRejectsMissingTamperedOrUnboundProof(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any, map[string]any)
	}{
		{name: "missing proof", mutate: func(_ map[string]any, event map[string]any) {
			delete(event, "reasoningExclusionProof")
		}},
		{name: "tampered proof", mutate: func(_ map[string]any, event map[string]any) {
			event["reasoningExclusionProof"] = "sha256:" + strings.Repeat("f", 64)
		}},
		{name: "mismatched summary", mutate: func(_ map[string]any, event map[string]any) {
			event["summary"] = "caller-authored compacted facts"
		}},
		{name: "mismatched source", mutate: func(_ map[string]any, event map[string]any) {
			event["sourceDigest"] = strings.Repeat("b", 64)
		}},
		{name: "unbound item", mutate: func(thread map[string]any, _ map[string]any) {
			turn := thread["turns"].([]any)[0].(map[string]any)
			turn["items"] = []any{}
		}},
		{name: "damaged durable item", mutate: func(thread map[string]any, _ map[string]any) {
			turn := thread["turns"].([]any)[0].(map[string]any)
			item := turn["items"].([]any)[0].(map[string]any)
			item["reasoningExclusionProof"] = "sha256:" + strings.Repeat("c", 64)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			thread, event := ordinaryCompactionCompletedFixture(t)
			test.mutate(thread, event)
			projected, visible, err := ProjectPublicThreadEvent("thread_compaction", thread, event)
			if err != nil || visible || projected != nil {
				t.Fatalf("invalid compaction completion crossed SSE: %#v visible=%t err=%v", projected, visible, err)
			}
		})
	}
}

func ordinaryCompactionCompletedFixture(t *testing.T) (map[string]any, map[string]any) {
	t.Helper()
	sourceDigest := strings.Repeat("a", 64)
	item := map[string]any{
		"id": "compaction_test", "turnId": "turn_compaction", "threadId": "thread_compaction",
		"role": "system", "status": "completed", "createdAt": "2026-07-20T00:00:00Z",
		"finishedAt": "2026-07-20T00:00:00Z", "kind": "compaction",
		"summary": domainevent.GeneralCompactionSummaryTextV3, "replacedTokens": float64(12), "auto": false,
		"pinnedConstraints": []any{"user: preserve recent turns"},
		"sourceDigest":      sourceDigest, "digestMarker": "sha256:" + sourceDigest[:12], "sourceItemIds": []any{"item_safe"},
		"schemaVersion": float64(3), "reasoningExcluded": true, "assistantProseExcluded": true,
		"toolPayloadsExcluded": true, "caseFactsExcluded": true, "providerHistoryProjectionVersion": float64(1),
	}
	item["reasoningExclusionProof"] = domainevent.ReasoningExclusionProof(item)
	thread := map[string]any{
		"id": "thread_compaction", "status": "idle", "turns": []any{map[string]any{
			"id": "turn_compaction", "threadId": "thread_compaction", "items": []any{item},
		}},
	}
	event := map[string]any{
		"kind": "compaction_completed", "threadId": "thread_compaction", "turnId": "turn_compaction",
		"itemId": "compaction_test", "summary": item["summary"], "replacedTokens": item["replacedTokens"],
		"auto": item["auto"], "pinnedConstraints": item["pinnedConstraints"], "sourceDigest": item["sourceDigest"],
		"digestMarker": item["digestMarker"], "sourceItemIds": item["sourceItemIds"],
		"schemaVersion": float64(2), "reasoningExcluded": true,
		"reasoningExclusionProof": item["reasoningExclusionProof"],
	}
	return thread, event
}
