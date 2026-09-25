package usageindexfs

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	usageapp "analytix.local/runtime-go/internal/app/usage"
	contracts "analytix.local/runtime-go/internal/contracts"
)

func TestStoreBuildsCumulativeUsageAndRejectsCorruptIndex(t *testing.T) {
	root := t.TempDir()
	threadID := "thread-a"
	if err := os.MkdirAll(filepath.Join(root, "threads", threadID), 0o700); err != nil {
		t.Fatal(err)
	}
	owner := &sync.Mutex{}
	events := []map[string]any{
		usageEvent(threadID, "turn-1", 1, 10, 1),
		usageEvent(threadID, "turn-2", 2, 15, 2),
	}
	store := newUsageIndexStoreForTest(t, root, owner, map[string]map[string]any{
		threadID: {"id": threadID, "model": "deepseek", "providerId": "deepseek"},
	}, func(string, int) (eventlog.LoadResult, error) {
		return eventlog.LoadResult{Events: cloneEvents(events)}, nil
	}, nil, nil)
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	records, err := store.LoadRecords(threadID)
	if err != nil || len(records) != 2 {
		t.Fatalf("usage records mismatch: records=%#v err=%v", records, err)
	}
	if records[0].Usage.TotalTokens != 10 || records[1].Usage.TotalTokens != 5 || records[1].Usage.Turns != 1 {
		t.Fatalf("cumulative usage was not converted to exact deltas: %#v", records)
	}
	if err := os.WriteFile(store.Path(), []byte(`{"threadId":"thread-a","threadId":"forged"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if records, err := store.LoadRecords(""); err == nil || records != nil {
		t.Fatalf("strictly corrupt index was trusted or silently rebuilt: records=%#v err=%v", records, err)
	}
}

func TestStoreUsageRebuildMergesConcurrentCanonicalEventExactlyOnce(t *testing.T) {
	root := t.TempDir()
	threadID := "thread-a"
	if err := os.MkdirAll(filepath.Join(root, "threads", threadID), 0o700); err != nil {
		t.Fatal(err)
	}
	owner := &sync.Mutex{}
	var eventsMu sync.Mutex
	events := []map[string]any{usageEvent(threadID, "turn-1", 1, 10, 1)}
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	store := newUsageIndexStoreForTest(t, root, owner, map[string]map[string]any{
		threadID: {"id": threadID, "model": "deepseek"},
	}, func(string, int) (eventlog.LoadResult, error) {
		eventsMu.Lock()
		defer eventsMu.Unlock()
		return eventlog.LoadResult{Events: cloneEvents(events)}, nil
	}, nil, func(string) {
		once.Do(func() { close(entered) })
		<-release
	})
	done := make(chan error, 1)
	go func() { done <- store.Ensure() }()
	<-entered
	concurrent := usageEvent(threadID, "turn-2", 2, 15, 2)
	eventsMu.Lock()
	events = append(events, contracts.CloneMap(concurrent))
	eventsMu.Unlock()
	owner.Lock()
	appendErr := store.AppendEventOwnerLocked(concurrent)
	owner.Unlock()
	if appendErr != nil {
		t.Fatalf("concurrent usage append: %v", appendErr)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("usage rebuild: %v", err)
	}
	global, err := filestore.ReadJSONLFileRecords[usageapp.IndexRecord](store.Path(), nil)
	if err != nil {
		t.Fatal(err)
	}
	perThread, err := filestore.ReadJSONLFileRecords[usageapp.IndexRecord](store.ThreadPath(threadID), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(global) != 2 || len(perThread) != 2 || global[1].Seq != 2 || perThread[1].Seq != 2 {
		t.Fatalf("concurrent usage event was lost or duplicated: global=%#v thread=%#v", global, perThread)
	}
}

func TestStoreTerminalSettlementRebuildsDivergentProjection(t *testing.T) {
	root := t.TempDir()
	threadID := "thread-a"
	if err := os.MkdirAll(filepath.Join(root, "threads", threadID), 0o700); err != nil {
		t.Fatal(err)
	}
	owner := &sync.Mutex{}
	event := usageEvent(threadID, "turn-1", 1, 10, 1)
	store := newUsageIndexStoreForTest(t, root, owner, map[string]map[string]any{
		threadID: {"id": threadID, "model": "deepseek"},
	}, func(string, int) (eventlog.LoadResult, error) {
		return eventlog.LoadResult{Events: []map[string]any{contracts.CloneMap(event)}}, nil
	}, nil, nil)
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(store.ThreadPath(threadID)); err != nil {
		t.Fatal(err)
	}
	owner.Lock()
	err := store.SettleTerminalEventOwnerLocked(event)
	owner.Unlock()
	if err != nil {
		t.Fatalf("terminal settlement did not rebuild divergent projection: %v", err)
	}
	if err := store.VerifyTerminalEvent(event); err != nil {
		t.Fatalf("terminal usage was not exact after rebuild: %v", err)
	}
}

func TestStoreBatchTerminalSettlementReadsCanonicalThreadOnceAndRepairs(t *testing.T) {
	root := t.TempDir()
	threadID := "thread-batch"
	if err := os.MkdirAll(filepath.Join(root, "threads", threadID), 0o700); err != nil {
		t.Fatal(err)
	}
	owner := &sync.Mutex{}
	events := []map[string]any{
		usageEvent(threadID, "turn-1", 1, 10, 1),
		usageEvent(threadID, "turn-2", 2, 15, 2),
	}
	var reads atomic.Int32
	store, err := New(Dependencies{
		Root: root, Owner: owner,
		ReadThread: func(string) (map[string]any, error) {
			reads.Add(1)
			return map[string]any{"id": threadID, "model": "deepseek"}, nil
		},
		LoadEvents: func(string, int) (eventlog.LoadResult, error) {
			return eventlog.LoadResult{Events: cloneEvents(events)}, nil
		},
		PendingUsageEventsOwnerLocked: func(string) []map[string]any { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	reads.Store(0)
	owner.Lock()
	err = store.SettleTerminalEventsOwnerLocked(events)
	owner.Unlock()
	if err != nil || reads.Load() != 1 {
		t.Fatalf("batch settlement did not share its canonical read: reads=%d err=%v", reads.Load(), err)
	}
	if err := os.Remove(store.ThreadPath(threadID)); err != nil {
		t.Fatal(err)
	}
	owner.Lock()
	err = store.SettleTerminalEventsOwnerLocked(events)
	owner.Unlock()
	if err != nil {
		t.Fatalf("batch settlement did not repair the derived projection: %v", err)
	}
	for _, event := range events {
		if err := store.VerifyTerminalEvent(event); err != nil {
			t.Fatalf("batch repair lost an exact terminal usage record: %v", err)
		}
	}
}

func newUsageIndexStoreForTest(
	t *testing.T,
	root string,
	owner sync.Locker,
	threads map[string]map[string]any,
	load func(string, int) (eventlog.LoadResult, error),
	pending func(string) []map[string]any,
	before func(string),
) *Store {
	t.Helper()
	if pending == nil {
		pending = func(string) []map[string]any { return nil }
	}
	store, err := New(Dependencies{
		Root: root, Owner: owner,
		ReadThread: func(threadID string) (map[string]any, error) {
			thread := threads[threadID]
			if thread == nil {
				return nil, errors.New("thread not found")
			}
			return contracts.CloneMap(thread), nil
		},
		LoadEvents: load, PendingUsageEventsOwnerLocked: pending, BeforeBuildThread: before,
		Now: func() time.Time { return time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func usageEvent(threadID, turnID string, seq, totalTokens, turns int) map[string]any {
	return map[string]any{
		"kind": "usage", "threadId": threadID, "turnId": turnID, "seq": seq,
		"timestamp": "2026-07-18T00:00:00Z",
		"usage":     map[string]any{"totalTokens": totalTokens, "turns": turns},
	}
}

func cloneEvents(events []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(events))
	for _, event := range events {
		out = append(out, contracts.CloneMap(event))
	}
	return out
}

func TestStoreLoadsNewThreadBeforeFirstUsageAndThenItsFirstRecord(t *testing.T) {
	root := t.TempDir()
	id := "thread-empty"
	if err := os.MkdirAll(filepath.Join(root, "threads", id), 0o700); err != nil {
		t.Fatal(err)
	}
	events := []map[string]any{}
	store := newUsageIndexStoreForTest(t, root, &sync.Mutex{}, map[string]map[string]any{id: {"id": id, "model": "synthetic"}}, func(string, int) (eventlog.LoadResult, error) {
		return eventlog.LoadResult{Events: cloneEvents(events)}, nil
	}, nil, nil)
	for i := 0; i < 2; i++ {
		records, err := store.LoadRecords(id)
		if err != nil || len(records) != 0 {
			t.Fatalf("new thread usage must be empty: count=%d err=%v", len(records), err)
		}
	}
	events = append(events, usageEvent(id, "turn-1", 1, 7, 1))
	store.owner.Lock()
	err := store.AppendEventOwnerLocked(events[0])
	store.owner.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.LoadRecords(id)
	if err != nil || len(records) != 1 || records[0].Usage.TotalTokens != 7 {
		t.Fatalf("first usage record was lost: records=%#v err=%v", records, err)
	}
}

func TestStoreLoadsEmptyIndependentUsageWithoutReadingHeldRows(t *testing.T) {
	root, preserved, _ := usagePreservationFixtureV1(t, false)
	store := newUsagePreservationStoreV1(t, root, preserved, func(id string, _ int) (eventlog.LoadResult, error) {
		if id != "independent" {
			t.Fatal("empty usage read reached held canonical events")
		}
		return eventlog.LoadResult{}, nil
	}, nil)
	records, err := store.LoadRecords("independent")
	if err != nil || len(records) != 0 {
		t.Fatalf("empty independent usage unavailable: count=%d err=%v", len(records), err)
	}
	if _, err := store.LoadRecords("held"); !errors.Is(err, ErrRestartPreserved) {
		t.Fatalf("held usage admitted: %v", err)
	}
}

func TestStoreTerminalSettlementDuringReaderRebuild(t *testing.T) {
	for _, scenario := range []string{"stale", "overlap", "leading_noop", "overlapping_noop"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			id := "thread-rebuild"
			if err := os.MkdirAll(filepath.Join(root, "threads", id), 0o700); err != nil {
				t.Fatal(err)
			}
			owner := &sync.Mutex{}
			var eventsMu sync.Mutex
			events := []map[string]any{}
			if scenario == "stale" || scenario == "overlap" {
				events = append(events, usageEvent(id, "turn-1", 1, 10, 1))
			}
			entered, capture, captured, release := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
			finished := make(chan struct{})
			var readerTicket atomic.Bool
			store := newUsageIndexStoreForTest(t, root, owner, map[string]map[string]any{
				id: {"id": id, "model": "synthetic"},
			}, func(string, int) (eventlog.LoadResult, error) {
				reader := readerTicket.CompareAndSwap(false, true)
				if reader {
					close(entered)
					if !awaitUsageRebuildSignalV1(capture) {
						return eventlog.LoadResult{}, errors.New("reader capture barrier expired")
					}
				}
				eventsMu.Lock()
				snapshot := cloneEvents(events)
				eventsMu.Unlock()
				if reader {
					close(captured)
					if !awaitUsageRebuildSignalV1(release) {
						return eventlog.LoadResult{}, errors.New("reader writeback barrier expired")
					}
				}
				return eventlog.LoadResult{Events: snapshot}, nil
			}, nil, nil)
			defer func() {
				closeUsageRebuildSignalV1(capture)
				closeUsageRebuildSignalV1(release)
				if !awaitUsageRebuildSignalV1(finished) {
					t.Error("reader rebuild did not drain")
				}
			}()
			done := make(chan error, 1)
			go func() { defer close(finished); done <- store.Ensure() }()
			if !awaitUsageRebuildSignalV1(entered) {
				t.Fatal("reader did not reach active rebuild")
			}
			appendCanonical := func(event map[string]any, terminal bool) {
				t.Helper()
				owner.Lock()
				defer owner.Unlock()
				eventsMu.Lock()
				events = append(events, contracts.CloneMap(event))
				eventsMu.Unlock()
				if terminal {
					for range 2 {
						if err := store.SettleTerminalEventOwnerLocked(event); err != nil {
							t.Errorf("terminal settlement during benign reader rebuild failed: %v", err)
						}
					}
				} else if err := store.AppendEventOwnerLocked(event); err != nil {
					t.Errorf("canonical usage enqueue failed: %v", err)
				}
			}
			if scenario == "overlap" {
				appendCanonical(usageEvent(id, "turn-2", 2, 15, 2), false)
			}
			if scenario == "overlapping_noop" {
				appendCanonical(usageEvent(id, "zero", 1, 0, 0), false)
				appendCanonical(usageEvent(id, "turn-1", 2, 0, 1), false)
				appendCanonical(usageEvent(id, "turn-2", 3, 0, 2), false)
			}
			close(capture)
			if !awaitUsageRebuildSignalV1(captured) {
				t.Fatal("reader did not retain its old canonical prefix")
			}
			if scenario == "leading_noop" {
				appendCanonical(usageEvent(id, "zero", 1, 0, 0), false)
				appendCanonical(usageEvent(id, "turn-1", 2, 10, 1), true)
			}
			if scenario == "stale" || scenario == "leading_noop" {
				seq := 2
				if scenario == "leading_noop" {
					seq = 3
				}
				appendCanonical(usageEvent(id, "turn-2", seq, 15, 2), true)
			}
			lastSeq, lastTotal := 3, 22
			if scenario == "leading_noop" || scenario == "overlapping_noop" {
				lastSeq = 4
			}
			if scenario == "overlapping_noop" {
				lastTotal = 0
			}
			last := usageEvent(id, "turn-3", lastSeq, lastTotal, 3)
			appendCanonical(last, true)
			close(release)
			if !awaitUsageRebuildSignalV1(finished) {
				t.Fatal("reader rebuild did not finish")
			}
			if err := <-done; err != nil {
				t.Fatalf("reader rebuild failed: %v", err)
			}
			global, globalErr := filestore.ReadJSONLFileRecords[usageapp.IndexRecord](store.Path(), nil)
			rows, rowsErr := filestore.ReadJSONLFileRecords[usageapp.IndexRecord](store.ThreadPath(id), nil)
			if globalErr != nil || rowsErr != nil || len(global) != 3 || len(rows) != 3 {
				t.Fatalf("stale reader lost or duplicated settled usage: global=%d thread=%d readErrors=%t", len(global), len(rows), globalErr != nil || rowsErr != nil)
			}
			wantDeltas := []int{10, 5, 7}
			if scenario == "overlapping_noop" {
				wantDeltas = []int{0, 0, 0}
			}
			for i, row := range rows {
				if !recordEqual(global[i], row) || row.Usage.TotalTokens != wantDeltas[i] || row.Usage.Turns != 1 {
					t.Errorf("stale reader changed exact usage delta at ordinal %d: tokens=%d turns=%d", i+1, row.Usage.TotalTokens, row.Usage.Turns)
				}
			}
			if rows[2].RawUsage.TotalTokens != lastTotal || rows[2].RawUsage.Turns != 3 {
				t.Error("stale reader changed latest raw usage")
			}
			if err := store.VerifyTerminalEvent(last); err != nil {
				t.Errorf("terminal usage was not exact after external reader writeback: %v", err)
			}
		})
	}
}

func awaitUsageRebuildSignalV1(signal <-chan struct{}) bool {
	select {
	case <-signal:
		return true
	case <-time.After(10 * time.Second):
		return false
	}
}

func closeUsageRebuildSignalV1(signal chan struct{}) {
	select {
	case <-signal:
	default:
		close(signal)
	}
}
