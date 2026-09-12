package server

import (
	"context"
	"testing"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
)

func TestDurablePrimaryReaderRequiresOneCompositionBinding(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{"title": "synthetic primary"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id := stringField(thread, "id")
	if _, err := store.ReadPrimaryThreadSnapshotV1(context.Background(), id); err == nil {
		t.Fatal("unbound store acquired primary authority")
	}
	if _, err := store.ReadCommittedEventLogSHA256V1(context.Background(), id); err == nil {
		t.Fatal("unbound store acquired event-log authority")
	}
	if store.BindPrimaryThreadReaderV1(nil) == nil {
		t.Fatal("nil reader bound")
	}
	reader, err := finalauthorityadapter.NewAcceptedFinalCASReader(store.root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BindPrimaryThreadReaderV1(reader); err != nil {
		t.Fatal(err)
	}
	if store.BindPrimaryThreadReaderV1(reader) == nil {
		t.Fatal("primary authority was rebound")
	}
	actual, err := store.ReadPrimaryThreadSnapshotV1(context.Background(), id)
	if err != nil || actual.ThreadID != id || actual.ThreadFileSHA256 == "" {
		t.Fatalf("bound primary could not be observed: %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.ReadPrimaryThreadSnapshotV1(cancelled, id); err == nil {
		t.Fatal("cancelled read acquired primary authority")
	}
}
