package model

import (
	"errors"
	"testing"

	domainsteering "analytix.local/runtime-go/internal/domain/steering"
)

type steeringPromotionStoreStub struct {
	entries               []map[string]any
	items                 []map[string]any
	events                []map[string]any
	err                   error
	expectedContextDigest string
	prefix                []domainsteering.PendingEntryExpectationV1
}

func (stub *steeringPromotionStoreStub) PromotePendingSteeringEntriesForContext(_, _, expectedContextDigest string) ([]map[string]any, []map[string]any, error) {
	stub.expectedContextDigest = expectedContextDigest
	return stub.entries, stub.items, stub.err
}

func (stub *steeringPromotionStoreStub) PromotePendingSteeringEntryPrefixForContext(
	_, _, expectedContextDigest string,
	expected []domainsteering.PendingEntryExpectationV1,
) ([]map[string]any, []map[string]any, error) {
	stub.expectedContextDigest = expectedContextDigest
	stub.prefix = append([]domainsteering.PendingEntryExpectationV1(nil), expected...)
	return stub.entries, stub.items, stub.err
}

func (stub *steeringPromotionStoreStub) RecordEvent(event map[string]any) (map[string]any, []string, error) {
	stub.events = append(stub.events, event)
	return event, nil, nil
}

func TestPromoteSteeringForProviderRecordsEventsAndMessages(t *testing.T) {
	const contextDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	store := &steeringPromotionStoreStub{
		entries: []map[string]any{{"id": "steer_1", "displayText": "visible"}},
		items: []map[string]any{{
			"id":       "item_steer_1",
			"threadId": "thread_1",
			"turnId":   "turn_1",
			"text":     "continue now",
		}},
	}
	messages, err := PromoteSteeringForProvider(store, "thread_1", "turn_1", contextDigest)
	if err != nil {
		t.Fatalf("promote steering: %v", err)
	}
	if len(messages) != 1 || messages[0].Role != "user" || messages[0].Content == "" {
		t.Fatalf("unexpected provider messages: %#v", messages)
	}
	if len(store.events) != 1 || store.events[0]["kind"] != "item_created" || store.events[0]["itemId"] != "item_steer_1" {
		t.Fatalf("unexpected item-created events: %#v", store.events)
	}
	if store.expectedContextDigest != contextDigest {
		t.Fatalf("expected frozen context digest, got %q", store.expectedContextDigest)
	}
}

func TestPromoteSteeringForProviderWithEntriesReturnsAdmissionMetadata(t *testing.T) {
	store := &steeringPromotionStoreStub{
		entries: []map[string]any{{
			"id":             "steer_1",
			"jobId":          "job_1",
			"childRunId":     "job_1",
			"steerMessageId": "steer_1",
		}},
		items: []map[string]any{{
			"id":       "item_steer_1",
			"threadId": "thread_1",
			"turnId":   "turn_1",
			"text":     "continue now",
		}},
	}
	messages, entries, items, err := PromoteSteeringForProviderWithEntries(store, "thread_1", "turn_1", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("promote steering: %v", err)
	}
	if len(messages) != 1 || messages[0].Role != "user" {
		t.Fatalf("unexpected promoted messages: %#v", messages)
	}
	if len(entries) != 1 || entries[0]["jobId"] != "job_1" || entries[0]["steerMessageId"] != "steer_1" {
		t.Fatalf("unexpected promoted entries: %#v", entries)
	}
	if len(items) != 1 || items[0]["id"] != "item_steer_1" {
		t.Fatalf("unexpected promoted items: %#v", items)
	}
}

func TestPromoteSteeringPrefixForProviderWithEntriesUsesExactCASInput(t *testing.T) {
	const contextDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	expected := []domainsteering.PendingEntryExpectationV1{{
		ID: "steer_1", ContentDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}}
	store := &steeringPromotionStoreStub{
		entries: []map[string]any{{"id": "steer_1"}},
		items: []map[string]any{{
			"id": "item_steer_1", "threadId": "thread_1", "turnId": "turn_1", "text": "continue now",
		}},
	}
	messages, entries, items, err := PromoteSteeringPrefixForProviderWithEntries(
		store, "thread_1", "turn_1", contextDigest, expected,
	)
	if err != nil || len(messages) != 1 || len(entries) != 1 || len(items) != 1 ||
		len(store.prefix) != 1 || store.prefix[0] != expected[0] || store.expectedContextDigest != contextDigest {
		t.Fatalf("exact prefix was not preserved: messages=%#v entries=%#v items=%#v prefix=%#v err=%v", messages, entries, items, store.prefix, err)
	}
}

func TestPromoteSteeringPrefixForProviderRejectsInvalidCASBeforeStore(t *testing.T) {
	const contextDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for name, expected := range map[string][]domainsteering.PendingEntryExpectationV1{
		"empty": nil,
		"missing id": {{
			ContentDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		}},
		"invalid digest": {{ID: "steer_1", ContentDigest: "invalid"}},
	} {
		t.Run(name, func(t *testing.T) {
			store := &steeringPromotionStoreStub{}
			messages, entries, items, err := PromoteSteeringPrefixForProviderWithEntries(
				store, "thread_1", "turn_1", contextDigest, expected,
			)
			if err == nil || len(messages) != 0 || len(entries) != 0 || len(items) != 0 || store.expectedContextDigest != "" {
				t.Fatalf("invalid prefix reached store: messages=%#v entries=%#v items=%#v store=%#v err=%v", messages, entries, items, store, err)
			}
		})
	}
}

func TestPromoteSteeringForProviderPropagatesStoreErrors(t *testing.T) {
	want := errors.New("turn inactive")
	if _, err := PromoteSteeringForProvider(&steeringPromotionStoreStub{err: want}, "thread_1", "turn_1", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); !errors.Is(err, want) {
		t.Fatalf("expected store error, got %v", err)
	}
	if messages, err := PromoteSteeringForProvider(nil, "thread_1", "turn_1", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err == nil || messages != nil {
		t.Fatalf("nil store should fail closed, messages=%#v err=%v", messages, err)
	}
	store := &steeringPromotionStoreStub{}
	if messages, err := PromoteSteeringForProvider(store, "thread_1", "turn_1", ""); err == nil || messages != nil {
		t.Fatalf("invalid context digest should fail closed, messages=%#v err=%v", messages, err)
	}
	if store.expectedContextDigest != "" {
		t.Fatalf("invalid context reached persistence: %q", store.expectedContextDigest)
	}
}
