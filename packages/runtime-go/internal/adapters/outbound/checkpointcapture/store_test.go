package checkpointcapture_test

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	checkpointcapture "analytix.local/runtime-go/internal/adapters/outbound/checkpointcapture"
	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	"analytix.local/runtime-go/internal/contracts"
	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
)

type captureHarness struct {
	log       *eventlog.Store
	mu        sync.Mutex
	highest   map[string]int
	thread    map[string]any
	published int
	persist   func(string, []map[string]any, bool) error
}

func newCaptureHarness(t *testing.T, root string) (*captureHarness, *checkpointapp.CapturedEventReconciler) {
	t.Helper()
	harness := &captureHarness{
		log: eventlog.NewStore(root), highest: map[string]int{},
		thread: map[string]any{"id": "thread-capture", "turns": []any{}},
	}
	harness.persist = harness.log.AppendEvents
	store := checkpointcapture.NewStore(
		&harness.mu, harness.log, harness.highest,
		func(string) (map[string]any, error) { return contracts.CloneMap(harness.thread), nil },
		func(_ string, thread map[string]any) (map[string]any, error) { return thread, nil },
		func(_ map[string]any, event map[string]any) (map[string]any, error) {
			return contracts.CloneMap(event), nil
		},
		harness.log.LoadSince,
		func(threadID string) (int, error) {
			highest, err := harness.log.HighestSeq(threadID)
			return highest + 1, err
		},
		func(threadID string, events []map[string]any, atomic bool) error {
			return harness.persist(threadID, events, atomic)
		},
		func(string, map[string]any) { harness.published++ },
	)
	return harness, checkpointapp.NewCapturedEventReconciler(store)
}

func TestCapturedEventReconcilerPersistsExactlyOnceAcrossRestartAndPublishesOnce(t *testing.T) {
	root := t.TempDir()
	harness, reconciler := newCaptureHarness(t, root)
	event := capturedEvent("thread-capture", strings.Repeat("a", 64), "captured")

	if err := reconciler.ReconcileCheckpointCapturedEventForStartup(event, false); err == nil {
		t.Fatal("read-only startup accepted a missing checkpoint capture event")
	}
	if result, err := harness.log.LoadSince("thread-capture", 0); err != nil || len(result.Events) != 0 {
		t.Fatalf("read-only startup mutated persistence: events=%d err=%v", len(result.Events), err)
	}
	if err := reconciler.ReconcileCheckpointCapturedEventForStartup(event, true); err != nil {
		t.Fatalf("append missing checkpoint capture event: %v", err)
	}
	if err := reconciler.ReconcileCheckpointCapturedEventForStartup(event, true); err != nil {
		t.Fatalf("idempotent startup reconciliation: %v", err)
	}

	restartedHarness, restarted := newCaptureHarness(t, root)
	if err := restarted.ReconcileCheckpointCapturedEventForStartup(event, false); err != nil {
		t.Fatalf("restart readback rejected exact durable event: %v", err)
	}
	if err := restarted.ReconcileCheckpointCapturedEvent(event); err != nil {
		t.Fatalf("runtime publish existing durable event: %v", err)
	}
	if err := restarted.ReconcileCheckpointCapturedEvent(event); err != nil {
		t.Fatalf("idempotent runtime publish: %v", err)
	}
	result, err := restartedHarness.log.LoadSince("thread-capture", 0)
	if err != nil || len(result.Diagnostics) != 0 || len(result.Events) != 1 {
		t.Fatalf("exactly-once durable event failed: events=%d diagnostics=%v err=%v", len(result.Events), result.Diagnostics, err)
	}
	if restartedHarness.published != 1 {
		t.Fatalf("runtime event published %d times, want 1", restartedHarness.published)
	}
}

func TestCapturedEventReconcilerTreatsCommittedACKLossAsSuccessAndConflictsAsFailure(t *testing.T) {
	t.Run("ack-lost-readback", func(t *testing.T) {
		harness, reconciler := newCaptureHarness(t, t.TempDir())
		harness.persist = func(threadID string, events []map[string]any, atomic bool) error {
			if err := harness.log.AppendEvents(threadID, events, atomic); err != nil {
				return err
			}
			return errors.New("append acknowledgement lost")
		}
		if err := reconciler.ReconcileCheckpointCapturedEventForStartup(capturedEvent("thread-capture", strings.Repeat("b", 64), "captured"), true); err != nil {
			t.Fatalf("committed append acknowledgement loss did not reconcile: %v", err)
		}
	})

	for _, test := range []struct {
		name   string
		seed   func(map[string]any) []map[string]any
		needle string
	}{
		{
			name: "duplicate",
			seed: func(event map[string]any) []map[string]any {
				first, second := contracts.CloneMap(event), contracts.CloneMap(event)
				first["seq"], first["timestamp"] = float64(1), "2026-07-15T00:00:00Z"
				second["seq"], second["timestamp"] = float64(2), "2026-07-15T00:00:01Z"
				return []map[string]any{first, second}
			},
			needle: "conflicts",
		},
		{
			name: "payload-mismatch",
			seed: func(event map[string]any) []map[string]any {
				mismatch := contracts.CloneMap(event)
				payload := contracts.CloneMap(mismatch["checkpoint"].(map[string]any))
				payload["status"] = "different"
				payload["capturePayloadDigest"] = domaincheckpointref.CapturedPayloadDigest(payload)
				mismatch["checkpoint"], mismatch["seq"], mismatch["timestamp"] = payload, float64(1), "2026-07-15T00:00:00Z"
				return []map[string]any{mismatch}
			},
			needle: "conflicts",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			harness, reconciler := newCaptureHarness(t, t.TempDir())
			event := capturedEvent("thread-capture", strings.Repeat("c", 64), "captured")
			if err := harness.log.AppendEventsAtomic("thread-capture", test.seed(event)); err != nil {
				t.Fatalf("seed durable conflict: %v", err)
			}
			if err := reconciler.ReconcileCheckpointCapturedEvent(event); err == nil || !strings.Contains(err.Error(), test.needle) {
				t.Fatalf("durable conflict was not rejected: %v", err)
			}
			if harness.published != 0 {
				t.Fatalf("conflicting event was published %d times", harness.published)
			}
		})
	}
}

func TestCapturedEventReconcilerFailsClosedOnStrictJSONDiagnostics(t *testing.T) {
	root := t.TempDir()
	harness, reconciler := newCaptureHarness(t, root)
	const sentinel = "PRIVATE_MALFORMED_CHECKPOINT_SENTINEL"
	body := `{"seq":1,"kind":"usage","threadId":"thread-capture"}` + "\n" +
		`{"private":"` + sentinel + `"` + "\n"
	if err := os.MkdirAll(harness.log.ThreadDir("thread-capture"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(harness.log.EventsPath("thread-capture"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(harness.log.EventsPath("thread-capture"))
	if err != nil {
		t.Fatal(err)
	}

	err = reconciler.ReconcileCheckpointCapturedEvent(
		capturedEvent("thread-capture", strings.Repeat("f", 64), "captured"),
	)
	if err == nil || !strings.Contains(err.Error(), "checkpoint captured event log contains invalid records") {
		t.Fatalf("checkpoint reconciliation did not fail closed on replay diagnostics: %v", err)
	}
	if strings.Contains(err.Error(), sentinel) {
		t.Fatalf("checkpoint reconciliation exposed rejected record bytes: %v", err)
	}
	if harness.published != 0 {
		t.Fatalf("checkpoint event was published %d times", harness.published)
	}
	after, readErr := os.ReadFile(harness.log.EventsPath("thread-capture"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(before) {
		t.Fatalf("failed checkpoint reconciliation changed durable bytes:\nbefore=%q\nafter=%q", before, after)
	}
}

func capturedEvent(threadID, frontierDigest, status string) map[string]any {
	payload := map[string]any{
		"schemaVersion": float64(1), "captureEventId": domaincheckpointref.CaptureEventID(frontierDigest), "status": status,
	}
	payload["capturePayloadDigest"] = domaincheckpointref.CapturedPayloadDigest(payload)
	return map[string]any{"kind": "checkpoint_captured", "threadId": threadID, "turnId": "turn-capture", "checkpoint": payload}
}
