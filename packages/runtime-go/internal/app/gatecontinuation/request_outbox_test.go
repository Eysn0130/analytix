package gatecontinuation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	continuationapp "analytix.local/runtime-go/internal/app/continuation"
	controlapp "analytix.local/runtime-go/internal/app/control"
	appmodel "analytix.local/runtime-go/internal/app/model"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	continuationstoreport "analytix.local/runtime-go/internal/ports/continuationstore"
)

func TestGateRequestReceiptRetryReusesExactAuthority(t *testing.T) {
	now := time.Now().UTC()
	pending := cancellationPendingFixture(t, domaincontinuation.KindApproval, now)
	store := newCancellationGateStore()
	service := continuationapp.NewService(newCancellationAuthority(), store)
	gateID := controlapp.SecureGateID(
		domaincontinuation.KindApproval, pending.ThreadID, pending.TurnID,
		pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID,
	)
	itemID := "item_" + gateID
	first, err := service.IssueOrResolvePendingHost(domaincontinuation.KindApproval, gateID, itemID, pending, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.IssueOrResolvePendingHost(domaincontinuation.KindApproval, gateID, itemID, pending, now.Add(time.Minute))
	if err != nil || second.ReceiptID != first.ReceiptID || second.Payload.IssuedAt != first.Payload.IssuedAt {
		t.Fatalf("exact receipt retry = %#v err=%v", second, err)
	}
	conflict := pending
	conflict.Model = "different-model"
	if _, err := service.IssueOrResolvePendingHost(domaincontinuation.KindApproval, gateID, itemID, conflict, now.Add(2*time.Minute)); err == nil {
		t.Fatal("same gate id accepted different pending authority")
	}
}

func TestConcurrentGateRequestReceiptRetriesShareOneOpenAuthority(t *testing.T) {
	now := time.Now().UTC()
	pending := cancellationPendingFixture(t, domaincontinuation.KindApproval, now)
	store := newCancellationGateStore()
	service := continuationapp.NewService(newCancellationAuthority(), store)
	gateID := controlapp.SecureGateID(
		domaincontinuation.KindApproval, pending.ThreadID, pending.TurnID,
		pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID,
	)
	itemID := "item_" + gateID
	type result struct {
		receiptID string
		err       error
	}
	results := make(chan result, 32)
	var workers sync.WaitGroup
	for index := 0; index < 32; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			receipt, err := service.IssueOrResolvePendingHost(
				domaincontinuation.KindApproval, gateID, itemID, pending, now.Add(time.Minute),
			)
			results <- result{receiptID: receipt.ReceiptID, err: err}
		}()
	}
	workers.Wait()
	close(results)
	receiptID := ""
	for candidate := range results {
		if candidate.err != nil || candidate.receiptID == "" {
			t.Fatalf("concurrent receipt result=%#v", candidate)
		}
		if receiptID == "" {
			receiptID = candidate.receiptID
		} else if candidate.receiptID != receiptID {
			t.Fatalf("concurrent receipts diverged: %q != %q", candidate.receiptID, receiptID)
		}
	}
	if _, err := store.ResolveDisposition(context.Background(), gateID); !errors.Is(err, continuationstoreport.ErrNotFound) {
		t.Fatalf("concurrent exact retry disposed the winning receipt: %v", err)
	}
}

func TestGateDispositionAckLossReturnsFirstSignedAuthority(t *testing.T) {
	now := time.Now().UTC()
	pending := cancellationPendingFixture(t, domaincontinuation.KindApproval, now)
	store := newCancellationGateStore()
	service := continuationapp.NewService(newCancellationAuthority(), store)
	gateID := controlapp.SecureGateID(
		domaincontinuation.KindApproval, pending.ThreadID, pending.TurnID,
		pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID,
	)
	if _, err := service.IssueOrResolvePendingHost(
		domaincontinuation.KindApproval, gateID, "item_"+gateID, pending, now,
	); err != nil {
		t.Fatal(err)
	}
	first, err := service.DisposePendingHostDisposition(
		gateID, domaincontinuation.StatusAllowed, "approval_allowed", now.Add(time.Minute), pending,
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.DisposePendingHostDisposition(
		gateID, domaincontinuation.StatusAllowed, "approval_allowed", now.Add(2*time.Minute), pending,
	)
	if err != nil || second != first {
		t.Fatalf("exact disposition retry = %#v, want %#v, err=%v", second, first, err)
	}
	if _, err := service.DisposePendingHostDisposition(
		gateID, domaincontinuation.StatusDenied, "approval_denied", now.Add(3*time.Minute), pending,
	); !errors.Is(err, continuationapp.ErrReceiptConsumed) {
		t.Fatalf("opposite disposition retry error = %v, want ErrReceiptConsumed", err)
	}
}

func TestApprovedGrantIsDeterministicFromDisposition(t *testing.T) {
	now := time.Date(2026, 7, 17, 9, 0, 0, 0, time.UTC)
	pending := cancellationPendingFixture(t, domaincontinuation.KindApproval, now)
	store := newCancellationGateStore()
	service := continuationapp.NewService(newCancellationAuthority(), store)
	gateID := controlapp.SecureGateID(
		domaincontinuation.KindApproval, pending.ThreadID, pending.TurnID,
		pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID,
	)
	if _, err := service.IssueOrResolvePendingHost(
		domaincontinuation.KindApproval, gateID, "item_"+gateID, pending, now,
	); err != nil {
		t.Fatal(err)
	}
	disposition, err := service.DisposePendingHostDisposition(
		gateID, domaincontinuation.StatusAllowed, "approval_allowed", now.Add(time.Minute), pending,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, firstDisposition, first, err := service.ResolveApprovedGrantHost(gateID, pending)
	if err != nil {
		t.Fatal(err)
	}
	_, secondDisposition, second, err := service.ResolveApprovedGrantHost(gateID, pending)
	if err != nil || firstDisposition != disposition || secondDisposition != disposition || first != second || first.IssuedAt != disposition.DisposedAt {
		t.Fatalf("approved grant was not disposition-deterministic: first=%#v second=%#v disposition=%#v err=%v", first, second, disposition, err)
	}
}

func TestApprovalTransitionRequiresSignedAllowedDisposition(t *testing.T) {
	now := time.Date(2026, 7, 17, 9, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name   string
		status string
		reason string
		tamper bool
	}{
		{name: "denied", status: domaincontinuation.StatusDenied, reason: "approval_denied"},
		{name: "tampered", status: domaincontinuation.StatusAllowed, reason: "approval_allowed", tamper: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			pending := cancellationPendingFixture(t, domaincontinuation.KindApproval, now)
			store := newCancellationGateStore()
			service := continuationapp.NewService(newCancellationAuthority(), store)
			gateID := controlapp.SecureGateID(
				domaincontinuation.KindApproval, pending.ThreadID, pending.TurnID,
				pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID,
			)
			if _, err := service.IssueOrResolvePendingHost(
				domaincontinuation.KindApproval, gateID, "item_"+gateID, pending, now,
			); err != nil {
				t.Fatal(err)
			}
			if _, err := service.DisposePendingHostDisposition(gateID, test.status, test.reason, now.Add(time.Minute), pending); err != nil {
				t.Fatal(err)
			}
			if test.tamper {
				store.mu.Lock()
				disposition := store.dispositions[gateID]
				disposition.DisposedAt = now.Add(2 * time.Minute).Format(time.RFC3339Nano)
				store.dispositions[gateID] = disposition
				store.mu.Unlock()
			}
			if _, _, _, err := service.ResolveApprovedGrantHost(gateID, pending); err == nil {
				t.Fatal("non-allowed or untrusted disposition authorized an approved grant")
			}
		})
	}
}

func TestGateRequestOutboxExactRetryDoesNotDuplicateProjection(t *testing.T) {
	for _, kind := range []string{domaincontinuation.KindApproval, domaincontinuation.KindUserInput} {
		kind := kind
		t.Run(kind, func(t *testing.T) {
			now := time.Now().UTC()
			pending := cancellationPendingFixture(t, kind, now)
			baseStore := newCancellationGateStore()
			service := continuationapp.NewService(newCancellationAuthority(), baseStore)
			gateID := controlapp.SecureGateID(kind, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
			receipt, err := service.IssueOrResolvePendingHost(kind, gateID, "item_"+gateID, pending, now)
			if err != nil {
				t.Fatal(err)
			}
			projection, err := ProjectVerifiedGateRequestV1(receipt)
			if err != nil {
				t.Fatal(err)
			}
			store := &requestProjectionStore{cancellationGateStore: baseStore, items: map[string]map[string]any{}}
			registry := controlapp.NewGateRegistry[appmodel.PendingToolCall]()
			manager := controlapp.NewApprovalUserInputManager()
			var eventMu sync.Mutex
			var event map[string]any
			eventWrites := 0
			dependencies := Dependencies{
				Store: store, Registry: registry, Manager: manager,
				RecordRequest: func(candidate map[string]any) error {
					eventMu.Lock()
					defer eventMu.Unlock()
					if event == nil {
						event = cloneRequestTestMap(candidate)
						eventWrites++
						return nil
					}
					if !exactRequestTestJSON(event, candidate) {
						return errors.New("event projection conflict")
					}
					return nil
				},
			}
			if err := persistAndActivateGateRequest(projection, pending, dependencies); err != nil {
				t.Fatal(err)
			}
			if err := persistAndActivateGateRequest(projection, pending, dependencies); err != nil {
				t.Fatalf("exact outbox retry: %v", err)
			}
			if store.ItemCount() != 1 || eventWrites != 1 {
				t.Fatalf("duplicate projection: items=%d events=%d", store.ItemCount(), eventWrites)
			}
			if kind == domaincontinuation.KindApproval {
				if _, ok := registry.ClaimApproval(gateID); !ok {
					t.Fatal("approval was not activated")
				}
			} else if _, ok := registry.ClaimUserInput(gateID); !ok {
				t.Fatal("user input was not activated")
			}
			if len(manager.Replay()) != 1 {
				t.Fatalf("manager projection duplicated: %#v", manager.Replay())
			}
		})
	}
}

func TestGateRequestProjectionFailureRetainsNonExecutableReservation(t *testing.T) {
	now := time.Now().UTC()
	pending := cancellationPendingFixture(t, domaincontinuation.KindApproval, now)
	baseStore := newCancellationGateStore()
	service := continuationapp.NewService(newCancellationAuthority(), baseStore)
	gateID := controlapp.SecureGateID(domaincontinuation.KindApproval, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
	receipt, err := service.IssueOrResolvePendingHost(domaincontinuation.KindApproval, gateID, "item_"+gateID, pending, now)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := ProjectVerifiedGateRequestV1(receipt)
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected item failure")
	store := &requestProjectionStore{cancellationGateStore: baseStore, ensureErr: injected, items: map[string]map[string]any{}}
	store.thread = map[string]any{"turns": []any{map[string]any{
		"id": pending.TurnID, "items": []any{},
	}}}
	registry := controlapp.NewGateRegistry[appmodel.PendingToolCall]()
	manager := controlapp.NewApprovalUserInputManager()
	err = persistAndActivateGateRequest(projection, pending, Dependencies{
		Store: store, Registry: registry, Manager: manager,
		RecordRequest: func(map[string]any) error { t.Fatal("event ran after item failure"); return nil },
	})
	if !errors.Is(err, injected) {
		t.Fatalf("projection failure = %v", err)
	}
	if _, ok := registry.ClaimApproval(gateID); ok {
		t.Fatal("failed projection became executable")
	}
	drained := registry.DrainForTurn(pending.ThreadID, pending.TurnID)
	if len(drained) != 1 || drained[0].ID != gateID || drained[0].Pending.Call.ID != pending.Call.ID {
		t.Fatalf("failed reservation was not retained for terminal settlement: %#v", drained)
	}
	store.mu.Lock()
	store.ensureErr = nil
	store.mu.Unlock()
	requestEvents, cancellationEvents := 0, 0
	terminalDependencies := Dependencies{
		Store: store, Registry: registry, Manager: manager, Continuations: service,
		RecordRequest: func(event map[string]any) error {
			if event["kind"] != "approval_requested" {
				t.Fatalf("request event = %#v", event)
			}
			requestEvents++
			return nil
		},
		RecordCancellations: func(cancellations []controlapp.PendingGateCancellation, reason string) error {
			if len(cancellations) != 1 || reason != "turn_failed" {
				t.Fatalf("cancellations=%#v reason=%q", cancellations, reason)
			}
			cancellationEvents++
			return nil
		},
	}
	if _, err := SettleDrainedForTerminal(drained, "turn_failed", terminalDependencies); err != nil {
		t.Fatalf("terminal outbox repair failed: %v", err)
	}
	if requestEvents != 1 || cancellationEvents != 1 {
		t.Fatalf("terminal projections request=%d cancellation=%d", requestEvents, cancellationEvents)
	}
	_, disposition, err := service.ResolveTrustedDisposition(context.Background(), gateID)
	if err != nil || disposition.Status != domaincontinuation.StatusInterrupted || disposition.ReasonCode != "turn_failed" {
		t.Fatalf("terminal disposition=%#v err=%v", disposition, err)
	}
	if status := store.ItemStatus(projection.Record.ItemID); status != "expired" {
		t.Fatalf("terminal item status=%q", status)
	}
}

type requestProjectionStore struct {
	*cancellationGateStore
	mu        sync.Mutex
	items     map[string]map[string]any
	ensureErr error
	thread    map[string]any
}

func (store *requestProjectionStore) EnsureGateRequestItemExact(_, _ string, item map[string]any) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.ensureErr != nil {
		return store.ensureErr
	}
	id, _ := item["id"].(string)
	if existing, ok := store.items[id]; ok {
		if exactRequestTestJSON(existing, item) {
			return nil
		}
		return errors.New("item projection conflict")
	}
	store.items[id] = cloneRequestTestMap(item)
	if store.thread != nil {
		turns, _ := store.thread["turns"].([]any)
		for _, rawTurn := range turns {
			turn, _ := rawTurn.(map[string]any)
			items, _ := turn["items"].([]any)
			turn["items"] = append(items, cloneRequestTestMap(item))
		}
	}
	return nil
}

func (store *requestProjectionStore) GetThread(string) (map[string]any, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.thread == nil {
		return store.cancellationGateStore.GetThread("")
	}
	return cloneRequestTestMap(store.thread), nil
}

func (store *requestProjectionStore) PatchTurnItemStatus(_, _, itemID, status string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	item, ok := store.items[itemID]
	if !ok {
		return errors.New("item unavailable")
	}
	item["status"] = status
	turns, _ := store.thread["turns"].([]any)
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			candidate, _ := rawItem.(map[string]any)
			if candidate["id"] == itemID {
				candidate["status"] = status
			}
		}
	}
	return nil
}

func (store *requestProjectionStore) ItemCount() int {
	store.mu.Lock()
	defer store.mu.Unlock()
	return len(store.items)
}

func (store *requestProjectionStore) ItemStatus(itemID string) string {
	store.mu.Lock()
	defer store.mu.Unlock()
	status, _ := store.items[itemID]["status"].(string)
	return status
}

func cloneRequestTestMap(input map[string]any) map[string]any {
	body, _ := json.Marshal(input)
	var output map[string]any
	_ = json.Unmarshal(body, &output)
	return output
}

func exactRequestTestJSON(left, right map[string]any) bool {
	leftBody, leftErr := json.Marshal(left)
	rightBody, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}
