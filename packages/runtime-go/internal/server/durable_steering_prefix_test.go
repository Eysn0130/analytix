package server

import (
	"errors"
	"testing"
	"time"

	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
)

func TestDurableSteeringExactPrefixCASLeavesNewerTailPending(t *testing.T) {
	store, err := NewProductionDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	configureDurableSteeringAuthority(t, store)
	thread, err := store.CreateThread(
		map[string]any{"title": "Exact steering prefix", "workspace": "/workspace/steering-prefix"},
		"/workspace/steering-prefix",
	)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn-steering-prefix"
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/workspace/steering-prefix",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("steering-prefix-manifest")),
		ContextEpoch:       1, IssuedAt: time.Date(2026, 7, 27, 2, 0, 0, 0, time.UTC),
	})
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "prompt": "start",
		"steering": []any{}, "items": []any{}, "createdAt": "2026-07-27T02:00:00Z", "startedAt": "2026-07-27T02:00:00Z",
		"securityContext": securityRecord,
	}, "", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatal(err)
	}

	admitted := make([]map[string]any, 0, 3)
	for index, input := range []struct {
		effect domainsecurity.LogicalEffect
		text   string
	}{
		{domainsecurity.LogicalEffectOrdinary, "ordinary one"},
		{domainsecurity.LogicalEffectOrdinary, "ordinary two"},
		{domainsecurity.LogicalEffectCaseData, "case tail"},
	} {
		clientID := "client-steering-prefix-" + string(rune('1'+index))
		entry, admitErr := store.AdmitSteeringEntryForContext(
			threadID, turnID, turnID, securityContext.ContextDigest,
			map[string]any{
				"id": domainsteering.EntryIDV1(turnID, clientID), "clientUserMessageId": clientID,
				"text": input.text, "admittedAt": "2026-07-27T02:00:01Z", "delivery": "steer",
				"logicalEffect": string(input.effect), "ordinaryWork": input.effect == domainsecurity.LogicalEffectOrdinary,
			},
		)
		if admitErr != nil {
			t.Fatal(admitErr)
		}
		admitted = append(admitted, entry)
	}
	expected := make([]domainsteering.PendingEntryExpectationV1, 0, 2)
	for _, entry := range admitted[:2] {
		candidate, expectationErr := domainsteering.NewPendingEntryExpectationV1(entry)
		if expectationErr != nil {
			t.Fatal(expectationErr)
		}
		expected = append(expected, candidate)
	}
	promoted, items, err := store.PromotePendingSteeringEntryPrefixForContext(
		threadID, turnID, securityContext.ContextDigest, expected,
	)
	if err != nil || len(promoted) != 2 || len(items) != 2 {
		t.Fatalf("exact prefix promotion failed: promoted=%#v items=%#v err=%v", promoted, items, err)
	}
	pending, err := store.PendingSteeringEntriesForContext(threadID, turnID, securityContext.ContextDigest)
	if err != nil || len(pending) != 1 || stringField(pending[0], "id") != stringField(admitted[2], "id") {
		t.Fatalf("prefix promotion consumed newer tail: pending=%#v err=%v", pending, err)
	}

	staleEntries, staleItems, err := store.PromotePendingSteeringEntryPrefixForContext(
		threadID, turnID, securityContext.ContextDigest, expected,
	)
	if !errors.Is(err, domainsteering.ErrPendingPrefixMismatch) || len(staleEntries) != 0 || len(staleItems) != 0 {
		t.Fatalf("stale prefix CAS did not fail closed: entries=%#v items=%#v err=%v", staleEntries, staleItems, err)
	}
	pending, err = store.PendingSteeringEntriesForContext(threadID, turnID, securityContext.ContextDigest)
	if err != nil || len(pending) != 1 || stringField(pending[0], "id") != stringField(admitted[2], "id") {
		t.Fatalf("failed CAS changed durable pending tail: pending=%#v err=%v", pending, err)
	}

	lastEntries, lastItems, err := store.PromotePendingSteeringEntriesForContext(
		threadID, turnID, securityContext.ContextDigest,
	)
	if err != nil || len(lastEntries) != 1 || len(lastItems) != 1 ||
		stringField(lastEntries[0], "id") != stringField(admitted[2], "id") {
		t.Fatalf("legacy promote-all wrapper did not preserve the remaining tail: entries=%#v items=%#v err=%v", lastEntries, lastItems, err)
	}
}
