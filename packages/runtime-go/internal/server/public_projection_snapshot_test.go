package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	threadapp "analytix.local/runtime-go/internal/app/thread"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	"analytix.local/runtime-go/internal/contracts"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
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
	created, err := store.CreateThread(map[string]any{"workspace": workspacetest.New(t)}, "")
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

// A synthetic ordinary entry goes through real durable Host admission and the
// normal HTTP detail route. This does not claim native or Provider acceptance.
func TestPublicSteeringDetailHasClosedDTOAndRetainsPrivateAuthority(t *testing.T) {
	store, err := NewProductionDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	configureDurableSteeringAuthority(t, store)
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{"title": "Synthetic steering detail", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID, turnID := contracts.StringField(thread, "id"), "turn_public_steering_detail"
	frozen := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		ContextEpoch: 1, IssuedAt: time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC),
	})
	record := turnsecurityapp.PublicRecord(frozen)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "prompt": "synthetic",
		"createdAt": "2026-10-09T00:00:00Z", "startedAt": "2026-10-09T00:00:00Z",
		"items": []any{}, "steering": []any{}, "securityContext": record,
	}, "", map[string]any{"securityState": record}); err != nil {
		t.Fatal(err)
	}
	const clientID = "00000000-0000-4000-8000-0000000000a1"
	entryID := domainsteering.EntryIDV1(turnID, clientID)
	if _, err := store.AdmitSteeringEntryForContext(threadID, turnID, turnID, frozen.ContextDigest, map[string]any{
		"id": entryID, "clientUserMessageId": clientID, "text": "synthetic steering",
		"admittedAt": "2026-10-09T00:00:01Z", "delivery": "steer",
	}); err != nil {
		t.Fatal(err)
	}
	before, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	privateEntry := before["turns"].([]any)[0].(map[string]any)["steering"].([]any)[0].(map[string]any)
	if contracts.StringField(privateEntry, "authoritySignature") == "" ||
		domainsteering.ValidatePendingEntryForContextV1(privateEntry, frozen.ContextDigest) != nil {
		t.Fatal("fixture did not retain real signed Host admission")
	}
	before = contracts.CloneMap(before)
	handler := &runtimeServerHandler{store: store, runtimeToken: DefaultRuntimeToken, publicProjector: threadapp.EnsurePublicProjector(nil)}
	request := httptest.NewRequest(http.MethodGet, "/v1/threads/"+threadID, nil)
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("normal thread detail code=%d want=200", response.Code)
	}
	var detail map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	publicEntry := detail["turns"].([]any)[0].(map[string]any)["steering"].([]any)[0]
	expected := map[string]any{
		"id": entryID, "clientUserMessageId": clientID, "text": "synthetic steering",
		"admittedAt": "2026-10-09T00:00:01Z", "status": "pending",
		"attachmentIds": []any{}, "fileReferences": []any{},
	}
	if !reflect.DeepEqual(publicEntry, expected) {
		t.Fatal("complete HTTP detail steering does not equal the closed wire DTO")
	}
	for _, key := range []string{
		"delivery", "projectionVersion", "contentDigest", "contextDigest", "authoritySignature", "authorityPublicKey", "authorityKeyId",
		"userInputId", "userInputCaptureReceiptId", "userInputReplyIdentity",
	} {
		if strings.Contains(response.Body.String(), "\""+key+"\":") {
			t.Fatalf("HTTP detail leaked private field %s", key)
		}
	}
	after, err := store.GetThread(threadID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("public detail changed durable private authority", err)
	}
}
