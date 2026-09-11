package usageindexfs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	usageapp "analytix.local/runtime-go/internal/app/usage"
)

type usageFaultReaderV1 struct{ err error }

func (reader usageFaultReaderV1) Read([]byte) (int, error) { return 0, reader.err }

func TestUsageIndexPreservesReadFailureAfterMalformedPartialLine(t *testing.T) {
	for _, body := range []string{"{", "{\n", "{\n{\"threadId\":\"independent\"}\n"} {
		t.Run(body, func(t *testing.T) {
			failure := errors.New("synthetic usage index read failure")
			reader := io.MultiReader(strings.NewReader(body), usageFaultReaderV1{err: failure})
			records, err := readUsageIndexV1(reader, "independent")
			if !errors.Is(err, failure) || errors.Is(err, errRepairableProjection) || records != nil {
				t.Fatalf("physical error became malformed projection: records=%d err=%v", len(records), err)
			}
		})
	}
}

func usagePreservationFixtureV1(t *testing.T, missing bool) (string, *RestartPreservationV1, []byte) {
	t.Helper()
	root := t.TempDir()
	for _, path := range []string{"threads/held", "threads/independent", "usage_events/threads"} {
		if err := os.MkdirAll(filepath.Join(root, path), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	encoded, err := json.Marshal(usageapp.IndexRecord{ThreadID: "held", TurnID: "turn-held", Seq: 1, Usage: usageapp.Snapshot{TotalTokens: 12}})
	if err != nil {
		t.Fatal(err)
	}
	raw := append(append(append([]byte("\t"), encoded...), '\r', '\n'), encoded...)
	if err := os.WriteFile(filepath.Join(root, "usage_events", "index.jsonl"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if !missing {
		if err := os.WriteFile(filepath.Join(root, "usage_events", "threads", "held.jsonl"), raw, 0o640); err != nil {
			t.Fatal(err)
		}
	}
	preserved, err := PrepareRestartPreservationV1(context.Background(), root, []string{"held", "reserved-absent"})
	if err != nil {
		t.Fatal(err)
	}
	return root, preserved, raw
}

func newUsagePreservationStoreV1(t *testing.T, root string, preserved *RestartPreservationV1, load func(string, int) (eventlog.LoadResult, error), before func(string)) *Store {
	t.Helper()
	store, err := New(Dependencies{Root: root, Owner: &sync.Mutex{}, RestartPreservation: preserved,
		ReadThread: func(id string) (map[string]any, error) {
			if preserved.OwnsThread(id) {
				return nil, errors.New("held thread was read for usage projection")
			}
			return map[string]any{"id": id, "model": "deepseek"}, nil
		}, LoadEvents: load, BeforeBuildThread: before,
		PendingUsageEventsOwnerLocked: func(string) []map[string]any { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestUsageRestartPreservesRawRowsAndMissingFilesAcrossIndependentWriters(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "original_file", true: "original_absence"}[missing], func(t *testing.T) {
			root, preserved, raw := usagePreservationFixtureV1(t, missing)
			store := newUsagePreservationStoreV1(t, root, preserved, func(id string, _ int) (eventlog.LoadResult, error) {
				if id != "independent" {
					return eventlog.LoadResult{}, errors.New("held events were loaded")
				}
				return eventlog.LoadResult{Events: []map[string]any{usageEvent(id, "turn-1", 1, 10, 1)}}, nil
			}, nil)
			if err := store.Ensure(); err != nil {
				t.Fatal(err)
			}
			event := usageEvent("held", "turn-held", 1, 12, 1)
			for name, run := range map[string]func() error{
				"append": func() error {
					store.owner.Lock()
					defer store.owner.Unlock()
					return store.AppendEventOwnerLocked(event)
				},
				"settle": func() error {
					store.owner.Lock()
					defer store.owner.Unlock()
					return store.SettleTerminalEventOwnerLocked(event)
				},
				"verify":      func() error { return store.VerifyTerminalEvent(event) },
				"read_held":   func() error { _, err := store.LoadRecords("held"); return err },
				"read_global": func() error { _, err := store.LoadRecords(""); return err },
			} {
				if err := run(); !errors.Is(err, ErrRestartPreserved) {
					t.Fatalf("%s admitted held usage authority: %v", name, err)
				}
			}
			store.owner.Lock()
			err := store.AppendEventOwnerLocked(usageEvent("independent", "turn-2", 2, 15, 2))
			store.owner.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			if records, err := store.LoadRecords("independent"); err != nil || len(records) != 2 {
				t.Fatalf("independent usage is unavailable: count=%d err=%v", len(records), err)
			}
			body, err := os.ReadFile(store.Path())
			if err != nil {
				t.Fatal(err)
			}
			var held bytes.Buffer
			for _, line := range bytes.SplitAfter(body, []byte("\n")) {
				if len(bytes.TrimSpace(line)) == 0 {
					continue
				}
				var record usageapp.IndexRecord
				if err := json.Unmarshal(line, &record); err != nil {
					t.Fatal(err)
				}
				if record.ThreadID == "held" {
					held.Write(line)
				}
			}
			if !bytes.Equal(held.Bytes(), raw) {
				t.Fatalf("held CRLF, duplicate rows or unterminated EOF changed: %v", err)
			}
			if err := preserved.Revalidate(context.Background(), root); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUsageRestartDeferredAppendKeepsHeldRowsAndIndependentEvents(t *testing.T) {
	root, preserved, _ := usagePreservationFixtureV1(t, false)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var eventsMu sync.Mutex
	events := []map[string]any{usageEvent("independent", "turn-1", 1, 10, 1)}
	store := newUsagePreservationStoreV1(t, root, preserved, func(id string, _ int) (eventlog.LoadResult, error) {
		if id != "independent" {
			return eventlog.LoadResult{}, errors.New("held events were loaded")
		}
		eventsMu.Lock()
		defer eventsMu.Unlock()
		return eventlog.LoadResult{Events: cloneEvents(events)}, nil
	}, func(string) { once.Do(func() { close(entered); <-release }) })
	done := make(chan error, 1)
	go func() { done <- store.Ensure() }()
	<-entered
	store.owner.Lock()
	heldErr := store.AppendEventOwnerLocked(usageEvent("held", "turn-held", 9, 99, 1))
	deferredHeld := len(store.deferredEvents)
	store.owner.Unlock()
	independent := usageEvent("independent", "turn-2", 2, 15, 2)
	eventsMu.Lock()
	events = append(events, independent)
	eventsMu.Unlock()
	store.owner.Lock()
	appendErr := store.AppendEventOwnerLocked(independent)
	store.owner.Unlock()
	close(release)
	buildErr := <-done
	if !errors.Is(heldErr, ErrRestartPreserved) || deferredHeld != 0 || appendErr != nil || buildErr != nil {
		t.Fatalf("deferred preservation failed: held=%v count=%d append=%v build=%v", heldErr, deferredHeld, appendErr, buildErr)
	}
	if records, err := store.LoadRecords("independent"); err != nil || len(records) != 2 {
		t.Fatalf("independent deferred event was lost or duplicated: count=%d err=%v", len(records), err)
	}
	if err := preserved.Revalidate(context.Background(), root); err != nil {
		t.Fatal(err)
	}
}

func TestUsageRestartRejectsOriginalDriftBeforeIndependentEffects(t *testing.T) {
	root, preserved, raw := usagePreservationFixtureV1(t, true)
	store := newUsagePreservationStoreV1(t, root, preserved, func(string, int) (eventlog.LoadResult, error) {
		t.Error("usage loaded events after original absence changed")
		return eventlog.LoadResult{}, nil
	}, nil)
	if err := os.WriteFile(store.ThreadPath("held"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Ensure(); err == nil {
		t.Fatal("original held absence was replaced by future usage")
	}
	body, err := os.ReadFile(store.Path())
	if err != nil || !bytes.Equal(body, raw) {
		t.Fatalf("refused rebuild changed global rows: %v", err)
	}
}

func TestUsageRestartSettlementPreservesCanonicalReadErrorsWithoutRebuild(t *testing.T) {
	for _, failure := range []error{errors.New("synthetic canonical read I/O failure"), context.Canceled} {
		t.Run(failure.Error(), func(t *testing.T) {
			root, preserved, _ := usagePreservationFixtureV1(t, false)
			event := usageEvent("independent", "turn-1", 1, 10, 1)
			store := newUsagePreservationStoreV1(t, root, preserved, func(string, int) (eventlog.LoadResult, error) {
				return eventlog.LoadResult{Events: []map[string]any{event}}, nil
			}, nil)
			if err := store.Ensure(); err != nil {
				t.Fatal(err)
			}
			global, err := os.ReadFile(store.Path())
			if err != nil {
				t.Fatal(err)
			}
			independent, err := os.ReadFile(store.ThreadPath("independent"))
			if err != nil {
				t.Fatal(err)
			}
			originalRead := store.readThread
			calls := 0
			store.readThread = func(id string) (map[string]any, error) {
				calls++
				if calls == 1 {
					return nil, failure
				}
				return originalRead(id)
			}
			before := store.backfills
			store.owner.Lock()
			err = store.SettleTerminalEventOwnerLocked(event)
			store.owner.Unlock()
			if !errors.Is(err, failure) || calls != 1 || store.backfills != before {
				t.Fatalf("canonical error became rebuild success: calls=%d backfills=%d err=%v", calls, store.backfills-before, err)
			}
			afterGlobal, globalErr := os.ReadFile(store.Path())
			afterIndependent, independentErr := os.ReadFile(store.ThreadPath("independent"))
			if globalErr != nil || independentErr != nil || !bytes.Equal(global, afterGlobal) || !bytes.Equal(independent, afterIndependent) {
				t.Fatal("failed canonical observation rewrote usage projections")
			}
		})
	}
}

func TestUsageTerminalSettlementDistinguishesProjectionDamageFromReadFailure(t *testing.T) {
	for _, scenario := range []string{"malformed_global", "malformed_thread", "divergent", "missing_global_unreadable_thread", "unreadable_global_missing_thread", "malformed_global_unreadable_thread", "unreadable_global_malformed_thread"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			id := "independent"
			if err := os.MkdirAll(filepath.Join(root, "threads", id), 0o700); err != nil {
				t.Fatal(err)
			}
			event := usageEvent(id, "turn-1", 1, 10, 1)
			store := newUsageIndexStoreForTest(t, root, &sync.Mutex{}, map[string]map[string]any{id: {"id": id, "model": "deepseek"}}, func(string, int) (eventlog.LoadResult, error) {
				return eventlog.LoadResult{Events: []map[string]any{event}}, nil
			}, nil, nil)
			if err := store.Ensure(); err != nil {
				t.Fatal(err)
			}
			write := func(path string, body []byte) {
				t.Helper()
				if err := os.WriteFile(path, body, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			remove := func(path string) {
				t.Helper()
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			unreadable := ""
			switch scenario {
			case "malformed_global":
				write(store.Path(), []byte("{\n"))
			case "malformed_thread":
				write(store.ThreadPath(id), []byte("{\n"))
			case "divergent":
				write(store.ThreadPath(id), []byte("\n"))
			case "missing_global_unreadable_thread", "malformed_global_unreadable_thread":
				if scenario == "missing_global_unreadable_thread" {
					remove(store.Path())
				} else {
					write(store.Path(), []byte("{\n"))
				}
				unreadable = store.ThreadPath(id)
			case "unreadable_global_missing_thread", "unreadable_global_malformed_thread":
				if scenario == "unreadable_global_missing_thread" {
					remove(store.ThreadPath(id))
				} else {
					write(store.ThreadPath(id), []byte("{\n"))
				}
				unreadable = store.Path()
			}
			if unreadable != "" {
				remove(unreadable)
				if err := os.Mkdir(unreadable, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			before := store.backfills
			readCalls := 0
			read := store.readThread
			store.readThread = func(id string) (map[string]any, error) { readCalls++; return read(id) }
			store.owner.Lock()
			err := store.SettleTerminalEventOwnerLocked(event)
			store.owner.Unlock()
			if unreadable != "" {
				var pathErr *os.PathError
				if !errors.As(err, &pathErr) || pathErr.Path != unreadable || errors.Is(err, errRepairableProjection) || readCalls != 0 || store.backfills != before {
					t.Fatalf("physical read failure became repair: calls=%d backfills=%d err=%v", readCalls, store.backfills-before, err)
				}
				if info, err := os.Lstat(unreadable); err != nil || !info.IsDir() {
					t.Fatal("failed read changed the original path")
				}
			} else if err != nil || store.backfills != before+1 {
				t.Fatalf("derived damage was not repaired: backfills=%d err=%v", store.backfills-before, err)
			} else if err := store.VerifyTerminalEvent(event); err != nil {
				t.Fatalf("repaired projection is not exact: %v", err)
			}
		})
	}
}

func TestUsageActiveSettlementPreservesHeldRowsAndReadFailures(t *testing.T) {
	root, preserved, _ := usagePreservationFixtureV1(t, false)
	event := usageEvent("independent", "turn-1", 1, 10, 1)
	store := newUsagePreservationStoreV1(t, root, preserved, func(id string, _ int) (eventlog.LoadResult, error) {
		if id != "independent" {
			return eventlog.LoadResult{}, errors.New("held canonical events were read")
		}
		return eventlog.LoadResult{Events: []map[string]any{event}}, nil
	}, nil)
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var readerTicket, rejectCanonical atomic.Bool
	cause := &os.PathError{Op: "read", Path: "/private/synthetic-usage-read", Err: os.ErrPermission}
	read := store.readThread
	store.readThread = func(id string) (map[string]any, error) {
		if rejectCanonical.Load() {
			return nil, cause
		}
		return read(id)
	}
	store.beforeBuildThread = func(string) {
		if readerTicket.CompareAndSwap(false, true) {
			close(entered)
			if !awaitUsageRebuildSignalV1(release) {
				t.Error("preserved reader barrier expired")
			}
		}
	}
	done := make(chan error, 1)
	go func() { defer close(finished); done <- store.Ensure() }()
	defer func() {
		rejectCanonical.Store(false)
		closeUsageRebuildSignalV1(release)
		if !awaitUsageRebuildSignalV1(finished) {
			t.Error("preserved reader did not drain")
		}
	}()
	if !awaitUsageRebuildSignalV1(entered) {
		t.Fatal("preserved reader did not start")
	}
	func() {
		store.owner.Lock()
		defer store.owner.Unlock()
		before := store.backfills
		heldErr := store.SettleTerminalEventOwnerLocked(usageEvent("held", "held-turn", 9, 99, 1))
		if !errors.Is(heldErr, ErrRestartPreserved) || len(store.deferredEvents) != 0 {
			t.Fatal("active settlement admitted or queued a held thread")
		}
		rejectCanonical.Store(true)
		err := store.SettleTerminalEventOwnerLocked(event)
		rejectCanonical.Store(false)
		if !errors.Is(err, cause) || errors.Is(err, errRepairableProjection) || store.backfills != before {
			t.Error("active canonical read failure was swallowed or rebuilt")
		}
		// A real projection read error still fails before repair. The separately
		// held reader is released only after this test restores its own fault.
		path := store.ThreadPath("independent")
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal("could not retain independent projection")
		}
		if err := os.Remove(path); err != nil {
			t.Fatal("could not install isolated read failure")
		}
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal("could not install isolated read failure")
		}
		err = store.SettleTerminalEventOwnerLocked(event)
		if err == nil || errors.Is(err, errRepairableProjection) || store.backfills != before {
			t.Error("active physical read failure was swallowed or rebuilt")
		}
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.IsDir() {
			t.Fatal("failed settlement changed the unreadable projection")
		}
		if err := os.Remove(path); err != nil {
			t.Fatal("could not remove isolated read fault")
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal("could not restore independent projection")
		}
	}()
	close(release)
	if !awaitUsageRebuildSignalV1(finished) {
		t.Fatal("preserved reader did not finish")
	}
	if err := <-done; err != nil {
		t.Fatal("preserved reader failed after fault restoration")
	}
	if err := preserved.Revalidate(context.Background(), root); err != nil {
		t.Fatal("held raw rows changed during active settlement")
	}
	if err := store.VerifyTerminalEvent(event); err != nil {
		t.Fatal("independent usage was not exact after reader writeback")
	}
}
