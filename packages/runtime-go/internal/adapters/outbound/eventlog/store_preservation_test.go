package eventlog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	domainevent "analytix.local/runtime-go/internal/domain/event"
)

func eventStorePreservationFixtureV1(t *testing.T, interrupted bool) (string, *SemanticRestartPreservationV1) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "threads", "held"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeMigrationFixtureJSON(t, filepath.Join(root, "threads", "held", "thread.json"), map[string]any{"id": "held", "turns": []any{}})
	writer := NewStore(root)
	if err := writer.AppendEvent("held", map[string]any{"seq": float64(1), "kind": "tool_call_ready", "threadId": "held", "turnId": "turn-1"}); err != nil {
		t.Fatal(err)
	}
	if interrupted {
		writer.atomicAppendCut = func(cut string) error {
			if cut == atomicBundleCutJournalCommitted {
				return errors.New("synthetic interrupted bundle")
			}
			return nil
		}
		if err := writer.AppendEventsAtomic("held", []map[string]any{{"seq": float64(2), "kind": "tool_call_ready", "threadId": "held", "turnId": "turn-1"}}); err == nil {
			t.Fatal("crash cut was not reached")
		}
	}
	preserved, err := PrepareSemanticRestartPreservationV1(context.Background(), root, filepath.Join(root, "thread_summaries.jsonl"), []string{"held"})
	if err != nil {
		t.Fatal(err)
	}
	return root, preserved
}

func TestEventStoreRestartPreservationDeniesEveryWriteAndRecoveryEntry(t *testing.T) {
	for _, operation := range []string{"single", "atomic", "frontier", "raw", "fork_source", "fork_target", "load", "single_recovery", "bundle_recovery"} {
		t.Run(operation, func(t *testing.T) {
			root, preserved := eventStorePreservationFixtureV1(t, false)
			store, err := NewStoreWithPreservationV1(root, preserved)
			if err != nil {
				t.Fatal(err)
			}
			before := semanticPreservationBytesV1(t, root)
			event := map[string]any{"seq": float64(2), "kind": "tool_call_ready", "threadId": "held", "turnId": "turn-1"}
			switch operation {
			case "single":
				err = store.appendSingleEventAtomic("held", event)
			case "atomic":
				err = store.AppendEventsAtomic("held", []map[string]any{event})
			case "frontier":
				_, frontier, observeErr := store.ObserveSinceWithFrontier(context.Background(), "held", 0)
				if observeErr != nil {
					t.Fatal(observeErr)
				}
				err = store.AppendEventsAtomicAtFrontier("held", []map[string]any{event}, frontier)
			case "raw":
				_, _, err = store.AppendRawLine("held", `{"seq":2,"kind":"tool_call_ready","threadId":"held","turnId":"turn-1"}`)
			case "fork_source":
				_, err = store.Fork("held", "new", domainevent.Seq(1))
			case "fork_target":
				_, err = store.Fork("missing", "held", domainevent.Seq(1))
			case "load":
				_, err = store.LoadSince("held", 0)
			case "single_recovery":
				_, err = store.recoverAtomicSingle("held")
			case "bundle_recovery":
				_, err = store.recoverAtomicBundle("held")
			}
			if !errors.Is(err, ErrRestartPreserved) {
				t.Errorf("entry failed to deny held ID: %v", err)
			}
			assertSemanticOriginalBytesV1(t, root, before)
		})
	}
}

func TestEventStoreRestartPreservationLeavesActualPendingBundleUntouched(t *testing.T) {
	root, preserved := eventStorePreservationFixtureV1(t, true)
	store, err := NewStoreWithPreservationV1(root, preserved)
	if err != nil {
		t.Fatal(err)
	}
	before := semanticPreservationBytesV1(t, root)
	if _, err := store.LoadSince("held", 0); !errors.Is(err, ErrRestartPreserved) {
		t.Errorf("held recovery accepted actual pending bundle: %v", err)
	}
	assertSemanticOriginalBytesV1(t, root, before)
}

func TestEventStoreRestartPreservationRetainsIndependentWritesAndReadOnlyObservation(t *testing.T) {
	root, preserved := eventStorePreservationFixtureV1(t, false)
	store, err := NewStoreWithPreservationV1(root, preserved)
	if err != nil {
		t.Fatal(err)
	}
	before := semanticPreservationBytesV1(t, filepath.Join(root, "threads", "held"))
	if err := store.AppendEvent("active", map[string]any{"seq": float64(1), "kind": "tool_call_ready", "threadId": "active", "turnId": "turn-1"}); err != nil {
		t.Fatal(err)
	}
	if loaded, err := store.LoadSince("active", 0); err != nil || len(loaded.Events) != 1 {
		t.Fatalf("independent event store failed: %v", err)
	}
	if observed, _, err := store.ObserveSinceWithFrontier(context.Background(), "held", 0); err != nil || len(observed.Events) != 1 {
		t.Fatalf("read-only committed observation failed: %v", err)
	}
	assertSemanticOriginalBytesV1(t, filepath.Join(root, "threads", "held"), before)
	if _, err := NewStoreWithPreservationV1(t.TempDir(), preserved); err == nil {
		t.Fatal("foreign root accepted")
	}
}
