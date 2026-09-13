package server

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"

	domainevent "analytix.local/runtime-go/internal/domain/event"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestAcceptedFinalBatchSlowSubscriberNeverReceivesPrefix(t *testing.T) {
	store := &DurableEventSessionStore{
		subscribers: map[string]map[chan map[string]any]struct{}{},
	}
	const threadID = "thread-accepted-final-batch"
	slow := make(chan map[string]any, 1)
	slow <- map[string]any{"kind": "queued-progress"}
	store.subscribers[threadID] = map[chan map[string]any]struct{}{slow: {}}
	batch := acceptedFinalDeliveryBatchForServerTest(t, threadID, 1)

	store.publishAcceptedFinalBatchNoLock(threadID, batch)

	if _, subscribed := store.subscribers[threadID]; subscribed {
		t.Fatal("slow subscriber remained live after rejecting the atomic bundle")
	}
	if event, ok := <-slow; !ok || event["kind"] != "queued-progress" {
		t.Fatalf("slow subscriber lost its already queued event: event=%#v open=%t", event, ok)
	}
	if event, ok := <-slow; ok || event != nil {
		t.Fatalf("slow subscriber received an accepted-final prefix: event=%#v open=%t", event, ok)
	}
}

func TestAcceptedFinalBatchReadySubscriberReceivesWholeBundle(t *testing.T) {
	store := &DurableEventSessionStore{
		subscribers: map[string]map[chan map[string]any]struct{}{},
	}
	const threadID = "thread-accepted-final-batch-ready"
	ready := make(chan map[string]any, 1)
	store.subscribers[threadID] = map[chan map[string]any]struct{}{ready: {}}
	batch := acceptedFinalDeliveryBatchForServerTest(t, threadID, 7)

	store.publishAcceptedFinalBatchNoLock(threadID, batch)

	select {
	case got := <-ready:
		parsed, err := domainevent.ParseAcceptedFinalDeliveryBatchV2(got)
		if err != nil || parsed.BatchID != batch.BatchID || len(parsed.Events) != len(batch.Events) {
			t.Fatalf("live delivery unit = %#v, err=%v", got, err)
		}
	default:
		t.Fatal("accepted-final batch was not delivered")
	}
	select {
	case extra := <-ready:
		t.Fatalf("bundle delivered an extra event: %#v", extra)
	default:
	}
}

func TestAcceptedFinalCursorInsideRangeReplaysWholeBatch(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{"title": "accepted-final cursor"}, workspacetest.New(t))
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	batch := acceptedFinalDeliveryBatchForServerTest(t, threadID, 1)
	if err := store.eventLog.AppendEventsAtomic(threadID, batch.Events); err != nil {
		t.Fatal(err)
	}

	for _, cursor := range []int{batch.FirstSeq, batch.LastSeq - 1} {
		replay, err := store.LoadPublicEventsSince(threadID, cursor)
		if err != nil {
			t.Fatalf("load public replay at cursor %d: %v", cursor, err)
		}
		if !reflect.DeepEqual(replay.Events, batch.Events) {
			t.Fatalf("cursor %d replayed a partial accepted-final batch: got=%#v want=%#v", cursor, replay.Events, batch.Events)
		}
	}

	replay, err := store.LoadPublicEventsSince(threadID, batch.LastSeq)
	if err != nil {
		t.Fatal(err)
	}
	if len(replay.Events) != 0 {
		t.Fatalf("cursor at the accepted-final batch tail replayed delivered events: %#v", replay.Events)
	}
}

func acceptedFinalDeliveryBatchForServerTest(t *testing.T, threadID string, firstSeq int) domainevent.AcceptedFinalDeliveryBatchV2 {
	t.Helper()
	commitID := digestServerBatchTest("commit-" + threadID)
	events := make([]map[string]any, 0, 3)
	for index, candidate := range []struct{ slot, kind string }{
		{"assistant-final", "item_completed"}, {"usage", "usage"}, {"terminal", "turn_completed"},
	} {
		event := map[string]any{
			"kind": candidate.kind, "threadId": threadID, "turnId": "turn-1", "seq": float64(firstSeq + index),
			"timestamp": "2026-07-18T00:00:00Z", "acceptedFinalDigest": commitID, "publicationCommitId": commitID,
			"publicationEventId": digestServerBatchTest("analytix.accepted-final-event/v1\x00" + commitID + "\x00" + candidate.slot),
			"publicationSlot":    candidate.slot,
		}
		switch candidate.slot {
		case "assistant-final":
			itemID := "item-turn-1-assistant"
			event["itemId"] = itemID
			event["item"] = map[string]any{
				"id": itemID, "threadId": threadID, "turnId": "turn-1", "role": "assistant",
				"kind": "assistant_text", "text": "verified boundary", "status": "completed",
				"acceptedFinalView": map[string]any{
					"schemaVersion": json.Number("3"), "acceptedFinalDigest": commitID,
					"publicationState": "accepted", "variant": "GeneralGuidanceAnswer", "terminalReason": "success",
					"blockerCode": "", "coverageStatus": "guidance_only", "checkedScopeDigest": "",
					"missingScopeCount": json.Number("0"), "claimCount": json.Number("0"), "claimTypes": []any{},
					"receiptMetadata": map[string]any{
						"projection": "masked_metadata_only", "count": json.Number("0"),
						"setDigest": digestServerBatchTest("empty-receipts"), "citations": []any{},
					},
					"noHitWording": "", "acceptedAt": "2026-07-18T00:00:00Z",
				},
			}
		case "usage":
			event["usageFinalStatus"] = "completed"
		case "terminal":
			event["status"] = "completed"
			event["terminalReason"] = "success"
		}
		payload := cloneMap(event)
		delete(payload, "seq")
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		event["publicationPayloadDigest"] = digestServerBatchBytes(body)
		events = append(events, event)
	}
	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	seal, err := domainevent.NewAcceptedFinalDeliverySealForEventsV2(
		events,
		digestServerBatchTest("accepted-final-disposition-"+threadID),
		digestServerBatchTest("terminal-disposition-"+threadID),
		digestServerBatchBytes(publicKey),
		publicKey,
		func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := domainevent.NewAcceptedFinalDeliveryBatchV2(events, seal)
	if err != nil {
		t.Fatal(err)
	}
	return batch
}

func digestServerBatchTest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func digestServerBatchBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
