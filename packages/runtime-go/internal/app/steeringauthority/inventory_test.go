package steeringauthority

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type authorityInventoryReaderStub struct {
	ids     []string
	threads map[string]map[string]any
	err     error
}

func (reader authorityInventoryReaderStub) AllThreadIDs() ([]string, error) {
	return append([]string(nil), reader.ids...), reader.err
}

func (reader authorityInventoryReaderStub) GetThread(id string) (map[string]any, error) {
	thread, ok := reader.threads[id]
	if !ok {
		return nil, errors.New("missing thread")
	}
	return thread, nil
}

func TestPublicSteeringAuthorityInventoryIsInstallationBound(t *testing.T) {
	authority := newAuthorityStub(0x41)
	service := NewService(authority)
	threadID := "thread-1"
	pendingContext := inventorySecurityContext(t, threadID, "turn-pending", 1)
	promotedContext := inventorySecurityContext(t, threadID, "turn-promoted", 2)
	cancelledContext := inventorySecurityContext(t, threadID, "turn-cancelled", 3)
	pending := sealedInventoryPending(t, service, pendingContext, "client-pending", "pending guidance")
	promotedPending := sealedInventoryPending(t, service, promotedContext, "client-promoted", "promoted guidance")
	promotedTurn, promotedEntries, promotedItems, err := domainsteering.PromoteTurnEntriesV1(
		threadID, promotedContext.TurnID,
		map[string]any{"steering": []any{promotedPending}, "items": []any{}},
		promotedContext.ContextDigest, "2026-07-18T01:02:04Z",
		func(entry map[string]any, digest string) error {
			return service.Verify(context.Background(), entry, digest)
		},
		func(entry map[string]any, digest string) (map[string]any, error) {
			return service.Promote(context.Background(), entry, digest)
		},
	)
	if err != nil || len(promotedEntries) != 1 || len(promotedItems) != 1 {
		t.Fatalf("promote inventory entry: entries=%#v items=%#v err=%v", promotedEntries, promotedItems, err)
	}
	cancelled := sealedInventoryPending(t, service, cancelledContext, "client-cancelled", "cancelled guidance")
	cancelled["status"] = "cancelled"
	cancelled["cancelledAt"] = "2026-07-18T01:02:05Z"
	cancelled["cancelReason"] = "aborted"
	reader := authorityInventoryReaderStub{
		ids: []string{threadID},
		threads: map[string]map[string]any{threadID: {
			"id": threadID,
			"turns": []any{
				map[string]any{"id": pendingContext.TurnID, "securityContext": inventorySecurityRecord(t, pendingContext), "steering": []any{pending}, "items": []any{}},
				map[string]any{"id": promotedContext.TurnID, "securityContext": inventorySecurityRecord(t, promotedContext), "steering": promotedTurn["steering"], "items": promotedTurn["items"]},
				map[string]any{"id": cancelledContext.TurnID, "securityContext": inventorySecurityRecord(t, cancelledContext), "steering": []any{cancelled}, "items": []any{}},
			},
		}},
	}
	if found, err := HasPublicAuthorityStateV1(reader); err != nil || !found {
		t.Fatalf("signed steering inventory was not discovered: found=%v err=%v", found, err)
	}
	if err := VerifyTrustedInventoryV1(context.Background(), reader, service); err != nil {
		t.Fatalf("trusted steering inventory failed verification: %v", err)
	}
	if err := VerifyTrustedInventoryV1(context.Background(), reader, NewService(newAuthorityStub(0x57))); err == nil {
		t.Fatal("foreign installation verified steering inventory")
	}
}

func inventorySecurityContext(t *testing.T, threadID, turnID string, epoch uint64) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/workspace",
		ContextEpoch: epoch, IssuedAt: time.Date(2026, 7, 18, 1, 2, int(epoch), 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func inventorySecurityRecord(t *testing.T, securityContext domainsecurity.TurnSecurityContext) map[string]any {
	t.Helper()
	body, err := json.Marshal(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	record := map[string]any{}
	if err := json.Unmarshal(body, &record); err != nil {
		t.Fatal(err)
	}
	return record
}

func sealedInventoryPending(
	t *testing.T,
	service *Service,
	securityContext domainsecurity.TurnSecurityContext,
	clientID, text string,
) map[string]any {
	t.Helper()
	pending, err := domainsteering.BindPendingEntryV1(map[string]any{
		"id": domainsteering.EntryIDV1(securityContext.TurnID, clientID), "clientUserMessageId": clientID,
		"text": text, "admittedAt": "2026-07-18T01:02:03Z", "delivery": "steer",
	}, securityContext.ContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	pending, err = service.Seal(context.Background(), pending, securityContext.ContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	return pending
}

func TestPublicSteeringAuthorityInventoryRejectsPartialMaterialAndCorruptShape(t *testing.T) {
	partial := authorityInventoryReaderStub{
		ids: []string{"thread-1"},
		threads: map[string]map[string]any{"thread-1": {
			"turns": []any{map[string]any{"steering": []any{map[string]any{"authorityKeyId": domainsecurity.SHA256Hex([]byte("key"))}}}},
		}},
	}
	if _, err := HasPublicAuthorityStateV1(partial); err == nil {
		t.Fatal("partial steering authority material was accepted")
	}
	unsigned := authorityInventoryReaderStub{
		ids: []string{"thread-1"},
		threads: map[string]map[string]any{"thread-1": {
			"turns": []any{map[string]any{"steering": []any{map[string]any{"status": "cancelled"}}}},
		}},
	}
	if found, err := HasPublicAuthorityStateV1(unsigned); err != nil || found {
		t.Fatalf("unsigned inert legacy state became authority: found=%v err=%v", found, err)
	}
	if err := VerifyTrustedInventoryV1(context.Background(), unsigned, NewService(newAuthorityStub(0x41))); err != nil {
		t.Fatalf("unsigned inert legacy state should not claim installation authority: %v", err)
	}
}

func TestPublicSteeringAuthorityInventoryRejectsDetachedOrDuplicatePromotion(t *testing.T) {
	service := NewService(newAuthorityStub(0x61))
	for name, mutate := range map[string]func(map[string]any){
		"missing promoted item": func(turn map[string]any) { turn["items"] = []any{} },
		"unknown promoted item field": func(turn map[string]any) {
			turn["items"].([]any)[0].(map[string]any)["reasoning"] = "private reasoning"
		},
		"mismatched frozen context": func(turn map[string]any) {
			turn["securityContext"].(map[string]any)["turnId"] = "turn-elsewhere"
		},
		"duplicate promoted entry": func(turn map[string]any) {
			entries := turn["steering"].([]any)
			turn["steering"] = append(entries, copyAuthorityEntry(entries[0].(map[string]any)))
		},
	} {
		t.Run(name, func(t *testing.T) {
			reader := promotedInventoryFixture(t, service)
			turn := reader.threads["thread-promoted"]["turns"].([]any)[0].(map[string]any)
			mutate(turn)
			if found, err := HasPublicAuthorityStateV1(reader); err == nil {
				t.Fatalf("corrupt signed promotion inventory was accepted: found=%v", found)
			}
			if err := VerifyTrustedInventoryV1(context.Background(), reader, service); err == nil {
				t.Fatal("corrupt signed promotion inventory verified")
			}
		})
	}
}

func promotedInventoryFixture(t *testing.T, service *Service) authorityInventoryReaderStub {
	t.Helper()
	threadID := "thread-promoted"
	securityContext := inventorySecurityContext(t, threadID, "turn-promoted", 1)
	pending := sealedInventoryPending(t, service, securityContext, "client-promoted", "promoted guidance")
	turn, entries, items, err := domainsteering.PromoteTurnEntriesV1(
		threadID, securityContext.TurnID,
		map[string]any{"steering": []any{pending}, "items": []any{}},
		securityContext.ContextDigest, "2026-07-18T01:02:04Z",
		func(entry map[string]any, digest string) error {
			return service.Verify(context.Background(), entry, digest)
		},
		func(entry map[string]any, digest string) (map[string]any, error) {
			return service.Promote(context.Background(), entry, digest)
		},
	)
	if err != nil || len(entries) != 1 || len(items) != 1 {
		t.Fatalf("build promoted inventory fixture: entries=%#v items=%#v err=%v", entries, items, err)
	}
	turn["id"] = securityContext.TurnID
	turn["securityContext"] = inventorySecurityRecord(t, securityContext)
	return authorityInventoryReaderStub{
		ids: []string{threadID},
		threads: map[string]map[string]any{threadID: {
			"id": threadID, "turns": []any{turn},
		}},
	}
}

func copyAuthorityEntry(entry map[string]any) map[string]any {
	out := make(map[string]any, len(entry)+3)
	for key, value := range entry {
		out[key] = value
	}
	return out
}
