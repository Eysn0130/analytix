package server

import (
	"errors"
	"testing"

	threadapp "analytix.local/runtime-go/internal/app/thread"
	"analytix.local/runtime-go/internal/contracts"
)

type ownerLockedPublicProjectorV1 struct {
	threadapp.PublicProjector
	store *DurableEventSessionStore
}

func (projector ownerLockedPublicProjectorV1) ProjectThread(thread map[string]any) (map[string]any, error) {
	// Final persistence also requires this mutex. A projection outside it can
	// combine an earlier running turn with a later committed final index.
	if projector.store.mu.TryLock() {
		projector.store.mu.Unlock()
		return nil, errors.New("public projection escaped the durable snapshot owner")
	}
	return contracts.CloneMap(thread), nil
}

func TestPublicThreadProjectionSharesDurableMutationSnapshot(t *testing.T) {
	store, err := NewProductionDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateThread(map[string]any{"workspace": t.TempDir()}, "")
	if err != nil {
		t.Fatal(err)
	}
	threadID := contracts.StringField(created, "id")
	if _, _, err := store.RecordEvent(map[string]any{"kind": "thread_created", "threadId": threadID, "status": created["status"]}); err != nil {
		t.Fatal(err)
	}
	service := threadapp.NewService(threadapp.Dependencies{
		Repository:      store,
		PublicProjector: ownerLockedPublicProjectorV1{PublicProjector: threadapp.EnsurePublicProjector(nil), store: store},
		UsageSnapshot: func(string) (map[string]any, error) {
			if !store.mu.TryLock() {
				return nil, errors.New("usage enrichment reentered the durable snapshot owner")
			}
			store.mu.Unlock()
			return map[string]any{}, nil
		},
	})
	detail, err := service.Get(threadID)
	if err != nil {
		t.Fatal(err)
	}
	if seq, ok := contracts.NumericSeq(detail["latestSeq"]); !ok || seq != 1 {
		t.Fatal("public snapshot omitted its aligned replay cursor")
	}
	listed, err := service.List(threadapp.ListInput{})
	if err != nil || len(listed) != 1 {
		t.Fatalf("list did not use the same owner-serialized projection: %v", err)
	}
}
