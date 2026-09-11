package threadsummaryindexfs

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	contracts "analytix.local/runtime-go/internal/contracts"
)

const preservedStoreFirstV1 = " {\"schemaVersion\":1, \"threadId\":\"held\", \"summary\":{\"id\":\"held\",\"title\":\"HISTORY\"}}\r\n"
const preservedStoreLastV1 = "\t{\"schemaVersion\":1,\"threadId\":\"held\",\"deleted\":true,\"summary\":{\"id\":\"held\",\"title\":\"DELETED\"}}"

func newPreservedStoreV1(t *testing.T, absent bool, read func(string) (map[string]any, error)) (*Store, *sync.Mutex) {
	t.Helper()
	root := t.TempDir()
	for _, id := range []string{"active", "held"} {
		if err := os.MkdirAll(filepath.Join(root, "threads", id), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, "thread_summaries.jsonl")
	if !absent {
		body := preservedStoreFirstV1 + `{"schemaVersion":1,"threadId":"active","summary":{"id":"active","title":"OLD"}}` + "\n" + preservedStoreLastV1
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	preserved, err := PreparePreservedRecordsV1(context.Background(), path, []string{"held"})
	if err != nil {
		t.Fatal(err)
	}
	owner := &sync.Mutex{}
	store, err := New(Dependencies{Root: root, Owner: owner, ReadThread: read, RestartPreservation: preserved,
		Now: func() time.Time { return time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, owner
}

func independentSummaryThreadV1(title string) map[string]any {
	return map[string]any{"id": "active", "title": title, "status": "idle", "updatedAt": "2026-09-07T00:00:00Z", "turns": []any{}}
}

func assertPreservedStoreV1(t *testing.T, store *Store, title string) []byte {
	t.Helper()
	body, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	lines, err := parsePreservedSummaryLinesV1(body)
	if err != nil {
		t.Fatal(err)
	}
	held := store.restartPreserved.held(lines)
	if len(held) != 2 || held[0] != preservedStoreFirstV1 || held[1] != preservedStoreLastV1 || !bytes.HasSuffix(body, []byte(preservedStoreLastV1)) {
		t.Fatal("writer changed held history, deletion row, or final framing")
	}
	store.owner.Lock()
	summaries, exists, loadErr := store.loadSummariesOwnerLocked()
	store.owner.Unlock()
	if loadErr != nil || !exists || len(summaries) != 1 || contracts.StringField(summaries[0], "title") != title {
		t.Fatalf("independent last writer lost or deleted held row resurrected: summaries=%v exists=%v err=%v", summaries, exists, loadErr)
	}
	if err := store.restartPreserved.Revalidate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestStorePreservationAppendAndRebuildKeepHeldHistory(t *testing.T) {
	heldReads := 0
	store, owner := newPreservedStoreV1(t, false, func(id string) (map[string]any, error) {
		if id == "held" {
			heldReads++
			return nil, errors.New("held primary must not be normalized for rebuild")
		}
		return independentSummaryThreadV1("CANONICAL"), nil
	})
	before, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	owner.Lock()
	denied := store.AppendOwnerLocked(map[string]any{"id": "held", "title": "REPLACEMENT"})
	owner.Unlock()
	after, err := os.ReadFile(store.Path())
	if err != nil || !errors.Is(denied, ErrRestartPreserved) || !bytes.Equal(before, after) {
		t.Fatal("held append was not denied without effects")
	}
	for _, title := range []string{"NEW", "LATEST"} {
		owner.Lock()
		appendErr := store.AppendOwnerLocked(independentSummaryThreadV1(title))
		owner.Unlock()
		if appendErr != nil {
			t.Fatal(appendErr)
		}
		assertPreservedStoreV1(t, store, title)
	}
	var rebuilt []byte
	for iteration := 0; iteration < 2; iteration++ {
		if err := store.Ensure(); err != nil {
			t.Fatal(err)
		}
		body := assertPreservedStoreV1(t, store, "CANONICAL")
		if iteration == 0 {
			rebuilt = body
		} else if !bytes.Equal(body, rebuilt) {
			t.Fatal("repeat rebuild changed stable index")
		}
	}
	if heldReads != 0 {
		t.Fatal("rebuild read held primary")
	}
}

func TestStorePreservationDeferredAppendWinsOverStaleRebuild(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	store, owner := newPreservedStoreV1(t, false, func(id string) (map[string]any, error) {
		if id == "held" {
			return nil, errors.New("held read")
		}
		close(entered)
		<-release
		return independentSummaryThreadV1("STALE"), nil
	})
	done := make(chan error, 1)
	go func() { done <- store.Ensure() }()
	<-entered
	owner.Lock()
	appendErr := store.AppendOwnerLocked(independentSummaryThreadV1("CONCURRENT"))
	heldErr := store.AppendOwnerLocked(map[string]any{"id": "held", "title": "FORGED"})
	owner.Unlock()
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if appendErr != nil || !errors.Is(heldErr, ErrRestartPreserved) {
		t.Fatalf("unexpected append results: %v, %v", appendErr, heldErr)
	}
	assertPreservedStoreV1(t, store, "CONCURRENT")
}

func TestStorePreservationMissingIndexBackfillDoesNotInventHeldRows(t *testing.T) {
	var heldReads atomic.Int32
	store, _ := newPreservedStoreV1(t, true, func(id string) (map[string]any, error) {
		if id == "held" {
			heldReads.Add(1)
			return map[string]any{"id": "held"}, nil
		}
		return independentSummaryThreadV1("BACKFILLED"), nil
	})
	rows, _, err := store.List(false, true, true, "", 10)
	if err != nil || len(rows) != 0 {
		t.Fatalf("missing index read: %v %v", rows, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for store.Stats().BackfillActive && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	stats := store.Stats()
	if stats.BackfillActive || stats.Backfills != 1 || heldReads.Load() != 0 {
		t.Fatalf("backfill did not preserve absent held rows: %+v", stats)
	}
	rows, _, err = store.List(false, true, true, "", 10)
	if err != nil || len(rows) != 1 || contracts.StringField(rows[0], "id") != "active" {
		t.Fatalf("backfill result: %v %v", rows, err)
	}
	if err := store.restartPreserved.Revalidate(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestStorePreservationRejectsChangedHeldRowsBeforeIndexWrite(t *testing.T) {
	for _, operation := range []string{"append", "ensure"} {
		t.Run(operation, func(t *testing.T) {
			store, owner := newPreservedStoreV1(t, false, func(string) (map[string]any, error) { return independentSummaryThreadV1("NEW"), nil })
			changed := []byte(" " + preservedStoreFirstV1 + preservedStoreLastV1)
			if err := os.WriteFile(store.Path(), changed, 0o600); err != nil {
				t.Fatal(err)
			}
			var err error
			if operation == "append" {
				owner.Lock()
				err = store.AppendOwnerLocked(independentSummaryThreadV1("NEW"))
				owner.Unlock()
			} else {
				err = store.Ensure()
			}
			if err == nil {
				t.Fatal("stale scope overwrote changed index")
			}
			after, readErr := os.ReadFile(store.Path())
			if readErr != nil || !bytes.Equal(changed, after) {
				t.Fatal("refusal changed index")
			}
		})
	}
	store, owner := newPreservedStoreV1(t, false, func(string) (map[string]any, error) { return nil, nil })
	if other, err := New(Dependencies{Root: t.TempDir(), Owner: owner, ReadThread: store.readThread, RestartPreservation: store.restartPreserved}); err == nil || other != nil {
		t.Fatal("scope from another store accepted")
	}
}
