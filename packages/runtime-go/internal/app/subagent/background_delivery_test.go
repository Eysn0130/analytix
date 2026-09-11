package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

type backgroundDeliveryExactStoreStub struct {
	thread map[string]any
}

type backgroundAutoContinueJobStoreStub struct {
	record domainjob.Record
	now    string
}

func (store *backgroundAutoContinueJobStoreStub) LoadChildRun(id string) (domainjob.Record, error) {
	if id != store.record.ID {
		return domainjob.Record{}, errors.New("job not found")
	}
	return store.record, nil
}

func (store *backgroundAutoContinueJobStoreStub) UpdateChildRun(id string, request domainjob.UpdateRequest) (domainjob.Record, error) {
	if id != store.record.ID {
		return domainjob.Record{}, errors.New("job not found")
	}
	if request.AutoContinueStatus != "" {
		store.record.AutoContinueStatus = request.AutoContinueStatus
		store.record.AutoContinueReason = request.AutoContinueReason
		store.record.AutoContinueError = ""
		store.record.AutoContinueUpdatedAt = store.now
		if store.record.AutoContinueUpdatedAt == "" {
			store.record.AutoContinueUpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		store.record.UpdatedAt = store.record.AutoContinueUpdatedAt
	}
	if request.AutoContinueTurnID != "" {
		store.record.AutoContinueTurnID = request.AutoContinueTurnID
	}
	return store.record, nil
}

func (store *backgroundAutoContinueJobStoreStub) AllRecords() []domainjob.Record {
	return []domainjob.Record{store.record}
}

func (store *backgroundAutoContinueJobStoreStub) ReserveBackgroundAutoContinueV1(
	id string,
	turnID string,
	gate func(domainjob.Record, []domainjob.Record) string,
) (domainjob.Record, bool, error) {
	if id != store.record.ID || store.record.AutoContinueStatus != "" {
		return store.record, false, nil
	}
	reason := gate(store.record, []domainjob.Record{store.record})
	if reason != "" {
		store.record.AutoContinueStatus = "skipped"
		store.record.AutoContinueReason = reason
		return store.record, false, nil
	}
	store.record.AutoContinueStatus = "starting"
	store.record.AutoContinueTurnID = turnID
	store.record.AutoContinueUpdatedAt = store.now
	if store.record.AutoContinueUpdatedAt == "" {
		store.record.AutoContinueUpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	store.record.UpdatedAt = store.record.AutoContinueUpdatedAt
	return store.record, true, nil
}

type backgroundAutoContinueStarterStub struct {
	newCalls   int
	startCalls int
	state      BackgroundAutoContinueTurnStateV1
}

func (starter *backgroundAutoContinueStarterStub) NewTurnID(domainjob.Record) (string, error) {
	starter.newCalls++
	return "turn_case_auto_continue", nil
}

func (starter *backgroundAutoContinueStarterStub) StartReservedTurn(_ context.Context, record domainjob.Record) (string, error) {
	starter.startCalls++
	return record.AutoContinueTurnID, nil
}

func (starter *backgroundAutoContinueStarterStub) InspectReservedTurn(domainjob.Record) (BackgroundAutoContinueTurnStateV1, error) {
	if starter.state != "" {
		return starter.state, nil
	}
	return BackgroundAutoContinueTurnMissingV1, nil
}

type backgroundNonContinuableCompletionAuthorityStub struct {
	receipt domainjob.ChildCompletionReceiptV1
}

func (stub backgroundNonContinuableCompletionAuthorityStub) Rehydrate(context.Context, domainjob.Record) (VerifiedChildCompletion, error) {
	receipt := stub.receipt
	receipt.CanContinueParent = false
	return VerifiedChildCompletion{receipt: receipt, trusted: true}, nil
}

func (store *backgroundDeliveryExactStoreStub) GetThread(string) (map[string]any, error) {
	return store.thread, nil
}

func (store *backgroundDeliveryExactStoreStub) EnsureBackgroundDeliveryItemExact(_, turnID string, item map[string]any) (BackgroundDeliveryEnsureItemResultV1, error) {
	turn, ok := FindTurn(store.thread, turnID)
	if !ok {
		return "", nil
	}
	for _, raw := range listAny(turn["items"]) {
		existing, _ := raw.(map[string]any)
		if mapString(existing, "id") != mapString(item, "id") {
			continue
		}
		if reflect.DeepEqual(existing, item) {
			return BackgroundDeliveryItemExistingExactV1, nil
		}
		return BackgroundDeliveryItemConflictV1, nil
	}
	turn["items"] = append(listAny(turn["items"]), item)
	return BackgroundDeliveryItemInsertedV1, nil
}

func TestBackgroundDeliveryAutoContinueGateFailsClosedWithoutSecurityAuthority(t *testing.T) {
	service := BackgroundDeliveryService{}
	record := domainjob.Record{SecurityBinding: &domainjob.SecurityBinding{}}
	if got := service.AutoContinueGate(record); got != "job_security_authority_unavailable" {
		t.Fatalf("auto-continue security blocker=%q", got)
	}
}

func TestBackgroundDeliveryAutoContinueGateAllowsOrdinaryWithoutCaseReceipt(t *testing.T) {
	service := BackgroundDeliveryService{
		SecurityBlocker: func(domainjob.Record) string { return "" },
	}
	record := domainjob.Record{SecurityBinding: &domainjob.SecurityBinding{ParentCaseID: "unbound"}}
	if got := service.AutoContinueGate(record); got != "" {
		t.Fatalf("auto-continue gate=%q", got)
	}
}

func TestBackgroundDeliveryAutoContinueGateConsumesTrustedCaseReceipt(t *testing.T) {
	fixture := newChildCompletionFixture(t)
	verified, err := fixture.authority.Issue(context.Background(), fixture.record, fixture.child.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := verified.ReceiptForPersistence()
	if err != nil {
		t.Fatal(err)
	}
	record := fixture.record
	record.Status = string(domainjob.StatusCompleted)
	record.Background = true
	record.AutoContinueParent = true
	record.ChildTurnID = fixture.child.TurnID
	record.ChildCompletionReceipt = receipt
	record.CompletionDeliveryStatus = "delivered"
	service := BackgroundDeliveryService{
		SecurityBlocker:     func(domainjob.Record) string { return "" },
		CompletionAuthority: fixture.authority,
	}
	if got := service.AutoContinueGate(record); got != "" {
		t.Fatalf("trusted case auto-continue gate=%q", got)
	}
}

func TestBackgroundDeliveryAutoContinueIgnoresCallerTerminalStatusUntilDurableCompletion(t *testing.T) {
	jobs := &backgroundAutoContinueJobStoreStub{record: domainjob.Record{
		ID: "job-running", ParentThreadID: "thread-parent", ParentTurnID: "turn-parent",
		Status: "running", Background: true, AutoContinueParent: true,
		SecurityBinding: &domainjob.SecurityBinding{ParentCaseID: "unbound"},
	}}
	threads := &backgroundDeliveryExactStoreStub{thread: map[string]any{
		"id": "thread-parent", "status": "idle", "turns": []any{map[string]any{
			"id": "turn-parent", "status": "completed", "items": []any{},
		}},
	}}
	starter := &backgroundAutoContinueStarterStub{}
	service := BackgroundDeliveryService{
		Threads: threads, Jobs: jobs, SecurityBlocker: func(domainjob.Record) string { return "" },
		AutoContinueStarter: starter,
	}
	service.MaybeAutoContinue(jobs.record, "completed", "HOSTILE_CALLER_TERMINAL")
	if jobs.record.AutoContinueStatus != "" || starter.newCalls != 0 || starter.startCalls != 0 {
		t.Fatalf("caller terminal status consumed the durable gate: record=%#v starter=%#v", jobs.record, starter)
	}
}

func newCaseBackgroundAutoContinueServiceV1(
	t *testing.T,
) (BackgroundDeliveryService, *backgroundAutoContinueJobStoreStub, *backgroundAutoContinueStarterStub, *backgroundDeliveryExactStoreStub, childCompletionFixture) {
	t.Helper()
	fixture := newChildCompletionFixture(t)
	verified, err := fixture.authority.Issue(context.Background(), fixture.record, fixture.child.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := verified.ReceiptForPersistence()
	if err != nil {
		t.Fatal(err)
	}
	fixture.thread["status"] = "idle"
	parentTurn, ok := FindTurn(fixture.thread, fixture.parent.TurnID)
	if !ok {
		t.Fatal("case parent turn is missing")
	}
	parentTurn["status"] = "completed"
	threads := &backgroundDeliveryExactStoreStub{thread: fixture.thread}
	record := fixture.record
	record.Status = string(domainjob.StatusCompleted)
	record.Background = true
	record.AutoContinueParent = true
	record.ChildTurnID = fixture.child.TurnID
	record.ChildCompletionReceipt = receipt
	record.CompletionDeliveryID = "delivery_case_job_1"
	record.CompletionDeliveryItemID = "item_case_job_1"
	record.CompletionDeliveryStatus = "delivered"
	record.CompletionDeliveryAt = fixture.issuedAt.Format(time.RFC3339Nano)
	record.FinishedAt = fixture.issuedAt.Format(time.RFC3339Nano)
	record.UpdatedAt = fixture.issuedAt.Format(time.RFC3339Nano)
	jobs := &backgroundAutoContinueJobStoreStub{record: record, now: fixture.issuedAt.Format(time.RFC3339Nano)}
	starter := &backgroundAutoContinueStarterStub{}
	authorizer := JobSecurityAuthorizer{Threads: threads, Jobs: jobs}
	service := BackgroundDeliveryService{
		Threads: threads, Jobs: jobs,
		SecurityBlocker: func(record domainjob.Record) string {
			return authorizer.BlockerAt(record, fixture.issuedAt)
		},
		SecurityBlockerAt:   authorizer.HistoricalBlockerAt,
		CompletionAuthority: fixture.authority,
		AutoContinueStarter: starter,
	}
	return service, jobs, starter, threads, fixture
}

func TestBackgroundDeliveryCaseCompletionReceiptStartsProductionServiceVertical(t *testing.T) {
	service, jobs, starter, threads, _ := newCaseBackgroundAutoContinueServiceV1(t)
	service.MaybeAutoContinue(jobs.record, jobs.record.Status, "ignored child payload")
	if jobs.record.AutoContinueStatus != "started" || jobs.record.AutoContinueTurnID != "turn_case_auto_continue" ||
		starter.newCalls != 1 || starter.startCalls != 1 {
		t.Fatalf("trusted case completion did not start: record=%#v starter=%#v", jobs.record, starter)
	}
	parent, _ := FindTurn(threads.thread, jobs.record.ParentTurnID)
	started := 0
	for _, raw := range listAny(parent["items"]) {
		item, _ := raw.(map[string]any)
		args, _ := item["arguments"].(map[string]any)
		diagnostics, _ := args["diagnostics"].(map[string]any)
		if mapString(diagnostics, "notificationKind") == "background_job_auto_continue" &&
			mapString(diagnostics, "autoContinueStatus") == "started" {
			started++
		}
	}
	if started != 1 {
		t.Fatalf("case auto-continue started lifecycle count=%d parent=%#v", started, parent)
	}
}

func TestBackgroundDeliveryCaseCompletionRejectionsRemainSideEffectFreeAcrossDuplicateAndRecovery(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		mutate func(*testing.T, *BackgroundDeliveryService, *backgroundAutoContinueJobStoreStub, *backgroundDeliveryExactStoreStub, childCompletionFixture)
	}{
		{
			name: "missing receipt", reason: "child_completion_receipt_required",
			mutate: func(_ *testing.T, _ *BackgroundDeliveryService, jobs *backgroundAutoContinueJobStoreStub, _ *backgroundDeliveryExactStoreStub, _ childCompletionFixture) {
				jobs.record.ChildCompletionReceipt = nil
			},
		},
		{
			name: "tampered receipt", reason: "child_completion_receipt_invalid",
			mutate: func(_ *testing.T, _ *BackgroundDeliveryService, jobs *backgroundAutoContinueJobStoreStub, _ *backgroundDeliveryExactStoreStub, _ childCompletionFixture) {
				receipt := *jobs.record.ChildCompletionReceipt
				receipt.ReceiptDigest = strings.Repeat("f", 64)
				jobs.record.ChildCompletionReceipt = &receipt
			},
		},
		{
			name: "trusted not continuable", reason: "child_completion_not_continuable",
			mutate: func(t *testing.T, service *BackgroundDeliveryService, jobs *backgroundAutoContinueJobStoreStub, _ *backgroundDeliveryExactStoreStub, fixture childCompletionFixture) {
				receipt, err := domainjob.NewChildCompletionReceiptV1(domainjob.ChildCompletionReceiptInputV1{
					ChildRunID: jobs.record.ID, SecurityBinding: jobs.record.SecurityBinding,
					ParentContext: fixture.parent, ChildContext: fixture.child, AcceptedFinal: fixture.privateFinal.AcceptedFinal,
					CanContinueParent: false, IssuedAt: fixture.issuedAt,
					AuthorityKeyID: fixture.host.KeyID(), AuthorityPublicKey: fixture.host.PublicKey(),
				}, func(message []byte) ([]byte, error) { return fixture.host.Sign(context.Background(), message) })
				if err != nil {
					t.Fatal(err)
				}
				jobs.record.ChildCompletionReceipt = &receipt
				service.CompletionAuthority = backgroundNonContinuableCompletionAuthorityStub{receipt: receipt}
			},
		},
		{
			name: "current epoch drift", reason: "parent_security_context_mismatch",
			mutate: func(_ *testing.T, _ *BackgroundDeliveryService, _ *backgroundAutoContinueJobStoreStub, threads *backgroundDeliveryExactStoreStub, fixture childCompletionFixture) {
				threads.thread["securityState"] = jobSecurityContractRecord(fixture.child)
			},
		},
		{
			name: "current grant expired", reason: "parent_execution_grant_expired",
			mutate: func(t *testing.T, service *BackgroundDeliveryService, _ *backgroundAutoContinueJobStoreStub, threads *backgroundDeliveryExactStoreStub, fixture childCompletionFixture) {
				expiresAt := mustJobSecurityTime(t, fixture.grant.ExpiresAt)
				authorizer := JobSecurityAuthorizer{Threads: threads}
				service.SecurityBlocker = func(record domainjob.Record) string { return authorizer.BlockerAt(record, expiresAt) }
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, jobs, starter, threads, fixture := newCaseBackgroundAutoContinueServiceV1(t)
			test.mutate(t, &service, jobs, threads, fixture)
			before, _ := json.Marshal(threads.thread)
			service.MaybeAutoContinue(jobs.record, jobs.record.Status, "HOSTILE_CHILD_OUTPUT_/Users/private_13800138000")
			if jobs.record.AutoContinueStatus != "skipped" || jobs.record.AutoContinueReason != test.reason {
				t.Fatalf("case rejection mismatch: %#v", jobs.record)
			}
			service.MaybeAutoContinue(jobs.record, jobs.record.Status, "duplicate")
			if err := service.RecoverAutoContinue(jobs.record); err != nil {
				t.Fatal(err)
			}
			after, _ := json.Marshal(threads.thread)
			if !reflect.DeepEqual(before, after) || starter.newCalls != 0 || starter.startCalls != 0 {
				t.Fatalf("case rejection gained a side effect: parent=%t starter=%#v", reflect.DeepEqual(before, after), starter)
			}
		})
	}
}

func TestBackgroundDeliveryCompletionBlockerFailsClosedWithoutTurnStore(t *testing.T) {
	if got := (BackgroundDeliveryService{}).CompletionBlocker("thread", "turn"); got != "runtime_store_unavailable" {
		t.Fatalf("completion blocker=%q", got)
	}
}

func TestBackgroundDeliveryLedgerFieldsUseDurableRecord(t *testing.T) {
	tests := []struct {
		name      string
		status    string
		reason    string
		timestamp string
	}{
		{name: "pending", status: "pending", reason: "runtime_startup_recovery", timestamp: "2026-08-23T01:00:00Z"},
		{name: "retry", status: "retry", reason: "completion_delivery_retry", timestamp: "2026-08-23T01:00:00Z"},
		{name: "delivered", status: "delivered", reason: "", timestamp: "2026-08-23T02:00:00Z"},
		{name: "dead letter", status: "dead_letter", reason: "parent_thread_missing", timestamp: "2026-08-23T03:00:00Z"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := domainjob.Record{
				CompletionDeliveryStatus: test.status,
				CompletionDeliveryReason: test.reason,
				CompletionDeliveryError:  "durable error withheld",
				UpdatedAt:                "2026-08-23T01:00:00Z",
				CompletionDeliveryAt:     "2026-08-23T02:00:00Z",
				CompletionDeadLetterAt:   "2026-08-23T03:00:00Z",
			}
			fields := backgroundDeliveryLedgerFields(record)
			if fields.status != test.status || fields.reason != test.reason ||
				fields.errorText != "durable error withheld" || fields.timestamp != test.timestamp {
				t.Fatalf("durable ledger fields mismatch: %#v", fields)
			}
		})
	}
}

func TestBackgroundAutoContinueStartedRepairUsesStableDurableProjectionWithoutHostilePayload(t *testing.T) {
	thread := map[string]any{"id": "thread-auto", "turns": []any{map[string]any{
		"id": "turn-auto", "items": []any{},
	}}}
	store := &backgroundDeliveryExactStoreStub{thread: thread}
	events := []map[string]any{}
	service := BackgroundDeliveryService{
		Threads:           store,
		SecurityBlocker:   func(domainjob.Record) string { return "" },
		SecurityBlockerAt: func(domainjob.Record, time.Time) string { return "" },
		RecordEvent:       func(event map[string]any, _ string) { events = append(events, event) },
	}
	const durableTime = "2026-08-23T07:00:00Z"
	const canary = "CALLER_PII_13800138000_/Users/private/case_6222020000000000000_LONG_REF"
	record := domainjob.Record{
		ID: "job-auto", ParentThreadID: "thread-auto", ParentTurnID: "turn-auto",
		Kind: canary, Label: canary, ChildThreadID: canary, ChildTurnID: canary, ArtifactPath: canary, Output: canary,
		Status: string(domainjob.StatusCompleted), Background: true, AutoContinueParent: true,
		CompletionDeliveryStatus: "delivered", CompletionDeliveryID: "delivery-auto",
		CompletionDeliveryItemID: "item-delivery-auto", CompletionDeliveryAt: durableTime,
		AutoContinueStatus: "started", AutoContinueTurnID: "turn-continuation",
		AutoContinueUpdatedAt: durableTime, UpdatedAt: "2026-08-23T07:30:00Z",
	}
	if err := service.RepairAutoContinueNotice(record); err != nil {
		t.Fatal(err)
	}
	if err := service.RepairAutoContinueNotice(record); err != nil {
		t.Fatal(err)
	}
	turn, _ := FindTurn(thread, "turn-auto")
	items := listAny(turn["items"])
	if len(items) != 1 || len(events) != 2 {
		t.Fatalf("exact auto notice items=%d events=%d", len(items), len(events))
	}
	item, _ := items[0].(map[string]any)
	if mapString(item, "createdAt") != durableTime || mapString(item, "finishedAt") != durableTime {
		t.Fatalf("auto notice used unstable time: %#v", item)
	}
	payload, _ := json.Marshal([]any{item, events})
	if strings.Contains(string(payload), canary) || strings.Contains(string(payload), "13800138000") ||
		strings.Contains(string(payload), "/Users/private") || strings.Contains(string(payload), "6222020000000000000") {
		t.Fatalf("hostile auto notice payload leaked: %s", payload)
	}
	item["summary"] = "preexisting-conflict"
	if err := service.RepairAutoContinueNotice(record); err == nil || !strings.Contains(err.Error(), "identity conflicts") {
		t.Fatalf("same-id auto notice conflict did not fail closed: %v", err)
	}
	if len(events) != 2 || mapString(item, "summary") != "preexisting-conflict" {
		t.Fatalf("auto notice conflict overwrote item or emitted events: item=%#v events=%#v", item, events)
	}
}

func TestRecoveredExactAutoContinueUsesDeliveryAuthorityTimeAndStartedDisplayTime(t *testing.T) {
	const deliveryTime = "2026-08-23T07:00:00Z"
	const startedTime = "2026-08-23T09:00:00Z"
	thread := map[string]any{"id": "thread-auto", "turns": []any{map[string]any{
		"id": "turn-parent", "status": "completed", "items": []any{},
	}}}
	threads := &backgroundDeliveryExactStoreStub{thread: thread}
	jobs := &backgroundAutoContinueJobStoreStub{record: domainjob.Record{
		ID: "job-auto", ParentThreadID: "thread-auto", ParentTurnID: "turn-parent",
		Status: string(domainjob.StatusCompleted), Background: true, AutoContinueParent: true,
		AutoContinueStatus: "starting", AutoContinueTurnID: "turn-continuation",
		CompletionDeliveryID: "delivery-auto", CompletionDeliveryItemID: "item-auto",
		CompletionDeliveryStatus: "delivered", CompletionDeliveryAt: deliveryTime,
		SecurityBinding: &domainjob.SecurityBinding{ParentCaseID: "unbound"},
	}, now: startedTime}
	starter := &backgroundAutoContinueStarterStub{state: BackgroundAutoContinueTurnExactV1}
	authorityTimes := []string{}
	service := BackgroundDeliveryService{
		Threads: threads, Jobs: jobs, SecurityBlocker: func(domainjob.Record) string { return "" },
		SecurityBlockerAt: func(_ domainjob.Record, at time.Time) string {
			authorityTimes = append(authorityTimes, at.UTC().Format(time.RFC3339Nano))
			if !at.UTC().Before(time.Date(2026, 8, 23, 8, 0, 0, 0, time.UTC)) {
				return "parent_execution_grant_expired"
			}
			return ""
		},
		AutoContinueStarter: starter,
	}
	if err := service.RecoverAutoContinue(jobs.record); err != nil {
		t.Fatal(err)
	}
	if jobs.record.AutoContinueStatus != "started" || starter.newCalls != 0 || starter.startCalls != 0 {
		t.Fatalf("exact recovered turn restarted work: record=%#v starter=%#v", jobs.record, starter)
	}
	parent, _ := FindTurn(thread, "turn-parent")
	items := listAny(parent["items"])
	if len(items) != 1 || len(authorityTimes) != 1 || authorityTimes[0] != deliveryTime {
		t.Fatalf("started notice authority time mismatch: items=%#v authorityTimes=%#v", items, authorityTimes)
	}
	item, _ := items[0].(map[string]any)
	if mapString(item, "createdAt") != startedTime || mapString(item, "finishedAt") != startedTime {
		t.Fatalf("started notice lost display time: %#v", item)
	}
	if err := service.RecoverAutoContinue(jobs.record); err != nil {
		t.Fatal(err)
	}
	if len(listAny(parent["items"])) != 1 || len(authorityTimes) != 2 || authorityTimes[1] != deliveryTime {
		t.Fatalf("exact started notice repair was not stable: items=%#v authorityTimes=%#v", parent["items"], authorityTimes)
	}
}
