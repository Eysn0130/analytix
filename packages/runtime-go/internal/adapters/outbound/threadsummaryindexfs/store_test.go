package threadsummaryindexfs

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	contracts "analytix.local/runtime-go/internal/contracts"
)

func TestStoreRehydratesForgedHintFromCanonicalThread(t *testing.T) {
	root := t.TempDir()
	threadID := "thread-a"
	if err := os.MkdirAll(filepath.Join(root, "threads", threadID), 0o700); err != nil {
		t.Fatal(err)
	}
	owner := &sync.Mutex{}
	canonical := map[string]any{
		"id": threadID, "title": "Canonical", "status": "idle", "updatedAt": "2026-07-18T00:00:00Z",
		"turns": []any{map[string]any{"id": "turn-1", "items": []any{map[string]any{"kind": "user_message", "text": "CANONICAL_PREVIEW"}}}},
	}
	store := newThreadSummaryStoreForTest(t, root, owner, func(string) (map[string]any, error) {
		return contracts.CloneMap(canonical), nil
	})
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	forged := store.recordForThread(map[string]any{
		"id": threadID, "title": "Forged", "status": "idle", "updatedAt": "9999-01-01T00:00:00Z",
		"turns": []any{map[string]any{"id": "turn-forged", "items": []any{map[string]any{"kind": "user_message", "text": "FORGED_PREVIEW"}}}},
	})
	if err := filestore.AppendJSONLRecord(store.Path(), forged); err != nil {
		t.Fatal(err)
	}
	summaries, ok, err := store.List(false, true, true, "", 10)
	if err != nil || !ok || len(summaries) != 1 {
		t.Fatalf("summary list mismatch: summaries=%#v ok=%v err=%v", summaries, ok, err)
	}
	if contracts.StringField(summaries[0], "title") != "Canonical" || contracts.StringField(summaries[0], "preview") != "CANONICAL_PREVIEW" {
		t.Fatalf("forged summary hint crossed canonical rehydration: %#v", summaries[0])
	}
	if err := os.WriteFile(store.Path(), []byte(`{"threadId":"thread-a","threadId":"forged"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if summaries, _, err := store.List(false, true, true, "", 10); err == nil || summaries != nil {
		t.Fatalf("corrupt summary index was trusted: summaries=%#v err=%v", summaries, err)
	}
}

func TestStoreBackfillDoesNotLoseConcurrentSummaryAppend(t *testing.T) {
	root := t.TempDir()
	threadID := "thread-a"
	if err := os.MkdirAll(filepath.Join(root, "threads", threadID), 0o700); err != nil {
		t.Fatal(err)
	}
	owner := &sync.Mutex{}
	oldThread := map[string]any{"id": threadID, "title": "Old", "status": "idle", "updatedAt": "2026-07-18T00:00:00Z", "turns": []any{}}
	newThread := map[string]any{"id": threadID, "title": "New", "status": "idle", "updatedAt": "2026-07-18T00:01:00Z", "turns": []any{}}
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	store := newThreadSummaryStoreForTest(t, root, owner, func(string) (map[string]any, error) {
		captured := contracts.CloneMap(oldThread)
		once.Do(func() { close(entered) })
		<-release
		return captured, nil
	})
	done := make(chan error, 1)
	go func() { done <- store.Ensure() }()
	<-entered
	owner.Lock()
	appendErr := store.AppendOwnerLocked(newThread)
	owner.Unlock()
	if appendErr != nil {
		t.Fatalf("append concurrent summary: %v", appendErr)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("summary backfill: %v", err)
	}
	records, err := filestore.ReadJSONLFileRecords[record](store.Path(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || contracts.StringField(records[0].Summary, "title") != "New" {
		t.Fatalf("concurrent summary append was overwritten by stale backfill: %#v", records)
	}
}

func TestStoreAppendReportsDerivedIndexAcknowledgementFailure(t *testing.T) {
	root := t.TempDir()
	threadID := "thread-a"
	if err := os.MkdirAll(filepath.Join(root, "threads", threadID), 0o700); err != nil {
		t.Fatal(err)
	}
	owner := &sync.Mutex{}
	store := newThreadSummaryStoreForTest(t, root, owner, func(string) (map[string]any, error) {
		return map[string]any{"id": threadID, "turns": []any{}}, nil
	})
	if err := os.Mkdir(store.Path(), 0o700); err != nil {
		t.Fatal(err)
	}
	owner.Lock()
	err := store.AppendOwnerLocked(map[string]any{"id": threadID, "turns": []any{}})
	owner.Unlock()
	if err == nil {
		t.Fatal("summary append acknowledgement failure was hidden")
	}
}

func newThreadSummaryStoreForTest(
	t *testing.T,
	root string,
	owner sync.Locker,
	readThread func(string) (map[string]any, error),
) *Store {
	t.Helper()
	store, err := New(Dependencies{
		Root: root, Owner: owner, ReadThread: readThread,
		Now: func() time.Time { return time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return store
}
