package subagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	turnapp "analytix.local/runtime-go/internal/app/turn"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	"analytix.local/runtime-go/internal/jobs"
	subagentstartupport "analytix.local/runtime-go/internal/ports/subagentstartup"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type startupRecoveryThreadStoreStub struct {
	thread      map[string]any
	getErr      error
	patchErr    error
	appendErr   error
	patchCalls  int
	appendCalls int
}

func (store *startupRecoveryThreadStoreStub) GetThread(string) (map[string]any, error) {
	if store.getErr != nil {
		return nil, store.getErr
	}
	return store.thread, nil
}

func (store *startupRecoveryThreadStoreStub) PatchTurnItemStatus(_, turnID, itemID, status string) error {
	store.patchCalls++
	if store.patchErr != nil {
		return store.patchErr
	}
	turn, ok := FindTurn(store.thread, turnID)
	if !ok {
		return errors.New("turn missing")
	}
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		if mapString(item, "id") == itemID {
			item["status"] = status
			return nil
		}
	}
	return errors.New("item missing")
}

func (store *startupRecoveryThreadStoreStub) AppendItemToTurn(_, turnID string, item map[string]any) error {
	store.appendCalls++
	if store.appendErr != nil {
		return store.appendErr
	}
	turn, ok := FindTurn(store.thread, turnID)
	if !ok {
		return errors.New("turn missing")
	}
	turn["items"] = append(listAny(turn["items"]), item)
	return nil
}

func (store *startupRecoveryThreadStoreStub) EnsureRecoveredParentToolSettlementExact(
	input subagentstartupport.RecoveredParentToolSettlementInput,
) (subagentstartupport.RecoveredParentToolSettlementResult, error) {
	if store.patchErr != nil {
		return "", store.patchErr
	}
	if store.appendErr != nil {
		return "", store.appendErr
	}
	next, changed, err := turnapp.EnsureRecoveredToolSettlementExact(turnapp.RecoveredToolSettlementInput{
		Thread: store.thread, ThreadID: input.Record.ParentThreadID, TurnID: input.Record.ParentTurnID,
		ToolCallItemID: input.ToolCallItemID, CallID: input.CallID, ToolName: input.ToolName,
		Status: input.Status, Timestamp: input.Timestamp, ResultItem: input.ResultItem,
	})
	if errors.Is(err, turnapp.ErrTurnItemIdentityConflict) {
		return subagentstartupport.RecoveredParentToolSettlementConflict, nil
	}
	if err != nil {
		return "", err
	}
	if !changed {
		return subagentstartupport.RecoveredParentToolSettlementExistingExact, nil
	}
	store.thread = next
	store.patchCalls++
	store.appendCalls++
	return subagentstartupport.RecoveredParentToolSettlementInserted, nil
}

func (store *startupRecoveryThreadStoreStub) EnsureBackgroundDeliveryItemExact(_, turnID string, item map[string]any) (BackgroundDeliveryEnsureItemResultV1, error) {
	if store.appendErr != nil {
		return "", store.appendErr
	}
	turn, ok := FindTurn(store.thread, turnID)
	if !ok {
		return "", errors.New("turn missing")
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
	store.appendCalls++
	turn["items"] = append(listAny(turn["items"]), item)
	return BackgroundDeliveryItemInsertedV1, nil
}

type startupRecoveryJobStoreStub struct {
	record      domainjob.Record
	all         []domainjob.Record
	loadErr     error
	updateErrAt int
	updateCalls int
	preserved   bool
	preserveErr error
}

type startupRecoveryInstallBarrier struct {
	*jobs.Manager
	afterQuery  func(domainjob.Record, bool, error)
	afterUpdate func(domainjob.Record, error)
}

func (store startupRecoveryInstallBarrier) UpdateChildRun(id string, request domainjob.UpdateRequest) (domainjob.Record, error) {
	record, err := store.Manager.UpdateChildRun(id, request)
	if store.afterUpdate != nil {
		store.afterUpdate(record, err)
	}
	return record, err
}

func (store startupRecoveryInstallBarrier) RestartPreservesChildRunV1(record domainjob.Record) (bool, error) {
	held, err := store.Manager.RestartPreservesChildRunV1(record)
	store.afterQuery(record, held, err)
	return held, err
}

func TestStartupRecoverySettledLedgerCannotRacePreservationInstallation(t *testing.T) {
	for _, mode := range []string{"startup", "update_replay"} {
		for _, installed := range []bool{false, true} {
			t.Run(mode+"/"+map[bool]string{false: "late_install_rejected", true: "held_before_recovery"}[installed], func(t *testing.T) {
				fixture, thread := startupRecoveryFixture(t)
				root := t.TempDir()
				initial, err := jobs.NewManager(root)
				if err != nil {
					t.Fatal(err)
				}
				record, err := initial.StartChildRun(domainjob.StartRequest{
					ParentGoalID: "goal-restart", ParentThreadID: fixture.ParentThreadID, ParentTurnID: fixture.ParentTurnID,
					ParentToolItemID: fixture.ParentToolItemID, ParentToolCallID: fixture.ParentToolCallID,
					ChildThreadID: "thread-child-restart", Kind: "subagent", Status: "completed", Background: true,
				})
				if err != nil {
					t.Fatal(err)
				}
				_, err = initial.UpdateChildRun(record.ID, domainjob.UpdateRequest{
					CompletionDeliveryID: "delivery-restart", CompletionDeliveryStatus: "skipped",
				})
				if err != nil {
					t.Fatal(err)
				}
				manager, err := jobs.NewManager(root)
				if err != nil {
					t.Fatal(err)
				}
				record, err = manager.LoadChildRun(record.ID)
				if err != nil {
					t.Fatal(err)
				}
				preserve := func() error {
					return manager.PreserveRestartScopeV1(context.Background(), []string{record.ParentThreadID}, []domainjob.Record{record})
				}
				if installed {
					if err := preserve(); err != nil {
						t.Fatal(err)
					}
				}
				before, err := jobs.BuildChildRunInventoryV1(root)
				if err != nil {
					t.Fatal(err)
				}
				queries, events := 0, 0
				store := startupRecoveryInstallBarrier{Manager: manager, afterQuery: func(_ domainjob.Record, held bool, err error) {
					queries++
					if err != nil || held != installed {
						t.Fatalf("unexpected recovery admission: held=%t err=%v", held, err)
					}
					if !installed && preserve() == nil {
						t.Error("hold installed between recovery query and parent ledger write")
					}
				}}
				store.afterUpdate = func(_ domainjob.Record, err error) {
					queries++
					if installed {
						if !errors.Is(err, jobs.ErrRestartPreserved) {
							t.Errorf("held delivery replay returned success: %v", err)
						}
						return
					}
					if err != nil {
						t.Fatalf("ordinary delivery replay failed: %v", err)
					}
					if preserve() == nil {
						t.Error("hold installed after delivery replay and before parent ledger write")
					}
				}
				threads := &startupRecoveryThreadStoreStub{thread: thread}
				service := NewStartupRecoveryService(StartupRecoveryDependencies{
					Threads: threads, Jobs: store, Security: &startupRecoverySecurityStub{},
					RecordEvent: func(map[string]any, string) { events++ },
				})
				if mode == "startup" {
					if err := service.RecoverPendingDeliveries(); err != nil {
						t.Fatal(err)
					}
				} else {
					updated := service.delivery.UpdateCompletion(record.ID, record.CompletionDeliveryID, record.CompletionDeliveryItemID, record.CompletionDeliveryStatus, record.CompletionDeliveryReason, "")
					if (updated.ID == "") != installed {
						t.Errorf("delivery replay return disagrees with preservation: empty=%t held=%t", updated.ID == "", installed)
					}
				}
				wantEffects := 1
				if installed {
					wantEffects = 0
				}
				if queries != 1 || events != wantEffects || threads.appendCalls != wantEffects || threads.patchCalls != 0 {
					t.Fatalf("unexpected recovery effects: queries=%d events=%d appends=%d patches=%d", queries, events, threads.appendCalls, threads.patchCalls)
				}
				if err := jobs.ValidateChildRunInventoryV1(root, before); err != nil {
					t.Fatalf("settled repair changed original job inventory: %v", err)
				}
			})
		}
	}
}

func (store *startupRecoveryJobStoreStub) RestartPreservesChildRunV1(domainjob.Record) (bool, error) {
	return store.preserved, store.preserveErr
}

func (store *startupRecoveryJobStoreStub) AllRecords() []domainjob.Record {
	return append([]domainjob.Record(nil), store.all...)
}

func (store *startupRecoveryJobStoreStub) LoadChildRun(string) (domainjob.Record, error) {
	if store.loadErr != nil {
		return domainjob.Record{}, store.loadErr
	}
	return store.record, nil
}

func (store *startupRecoveryJobStoreStub) UpdateChildRun(_ string, request domainjob.UpdateRequest) (domainjob.Record, error) {
	store.updateCalls++
	if store.updateErrAt == store.updateCalls {
		return domainjob.Record{}, errors.New("persist job failed")
	}
	if request.CompletionDeliveryID != "" {
		store.record.CompletionDeliveryID = request.CompletionDeliveryID
	}
	if request.CompletionDeliveryItemID != "" {
		store.record.CompletionDeliveryItemID = request.CompletionDeliveryItemID
	}
	if request.CompletionDeliveryStatus != "" {
		store.record.CompletionDeliveryStatus = request.CompletionDeliveryStatus
	}
	if request.CompletionDeliveryReason != "" {
		store.record.CompletionDeliveryReason = request.CompletionDeliveryReason
	}
	if request.RecoveryStatus != "" {
		store.record.RecoveryStatus = request.RecoveryStatus
	}
	if request.RecoveryReason != "" {
		store.record.RecoveryReason = request.RecoveryReason
	}
	return store.record, nil
}

type startupRecoverySecurityStub struct {
	blocker       string
	deadLetterErr error
	deadLetters   []string
}

func (security *startupRecoverySecurityStub) Blocker(domainjob.Record) string {
	return security.blocker
}

func (security *startupRecoverySecurityStub) BlockerAt(domainjob.Record, time.Time) string {
	return security.blocker
}

func (security *startupRecoverySecurityStub) MarkDeadLetter(_ domainjob.Record, reason string) error {
	security.deadLetters = append(security.deadLetters, reason)
	return security.deadLetterErr
}

func TestStartupRecoveryRejectsStaleEpochBeforeParentMutation(t *testing.T) {
	record, thread := startupRecoveryFixture(t)
	threads := &startupRecoveryThreadStoreStub{thread: thread}
	jobs := &startupRecoveryJobStoreStub{record: record}
	security := &startupRecoverySecurityStub{blocker: "parent_security_context_mismatch"}
	service := NewStartupRecoveryService(StartupRecoveryDependencies{Threads: threads, Jobs: jobs, Security: security})

	if err := service.RecoverInterrupted([]domainjob.Record{record}); err != nil {
		t.Fatal(err)
	}
	if jobs.updateCalls != 0 || threads.patchCalls != 0 || threads.appendCalls != 0 {
		t.Fatalf("stale recovery mutated authority: jobs=%d patch=%d append=%d", jobs.updateCalls, threads.patchCalls, threads.appendCalls)
	}
	if len(security.deadLetters) != 1 || security.deadLetters[0] != "parent_security_context_mismatch" {
		t.Fatalf("stale recovery did not dead-letter deterministically: %#v", security.deadLetters)
	}
}

func TestStartupRecoveryPreservedJobsNeverEnterDispositionOrDelivery(t *testing.T) {
	for _, operation := range []string{"interrupted", "pending_delivery", "already_delivered"} {
		t.Run(operation, func(t *testing.T) {
			record, thread := startupRecoveryFixture(t)
			if operation == "already_delivered" {
				record.Status = "completed"
				record.CompletionDeliveryStatus = "delivered"
				record.AutoContinueParent = true
			}
			threads := &startupRecoveryThreadStoreStub{thread: thread}
			jobs := &startupRecoveryJobStoreStub{record: record, all: []domainjob.Record{record}, preserved: true}
			security := &startupRecoverySecurityStub{blocker: "held authority must not become a dead letter"}
			service := NewStartupRecoveryService(StartupRecoveryDependencies{Threads: threads, Jobs: jobs, Security: security})
			var err error
			if operation == "interrupted" {
				err = service.RecoverInterrupted([]domainjob.Record{record})
			} else {
				err = service.RecoverPendingDeliveries()
			}
			if err != nil || jobs.updateCalls != 0 || len(security.deadLetters) != 0 || threads.patchCalls != 0 || threads.appendCalls != 0 {
				t.Fatalf("held job entered restart effects: updates=%d deadLetters=%d patches=%d appends=%d err=%v", jobs.updateCalls, len(security.deadLetters), threads.patchCalls, threads.appendCalls, err)
			}
		})
	}
}

func TestStartupRecoveryPropagatesPreservationFailureBeforeEffects(t *testing.T) {
	for _, pending := range []bool{false, true} {
		record, thread := startupRecoveryFixture(t)
		injected := errors.New("preserved Core dependency changed")
		threads := &startupRecoveryThreadStoreStub{thread: thread}
		jobs := &startupRecoveryJobStoreStub{record: record, all: []domainjob.Record{record}, preserveErr: injected}
		security := &startupRecoverySecurityStub{}
		service := NewStartupRecoveryService(StartupRecoveryDependencies{Threads: threads, Jobs: jobs, Security: security})
		var err error
		if pending {
			err = service.RecoverPendingDeliveries()
		} else {
			err = service.RecoverInterrupted([]domainjob.Record{record})
		}
		if !errors.Is(err, injected) || jobs.updateCalls != 0 || len(security.deadLetters) != 0 || threads.patchCalls != 0 || threads.appendCalls != 0 {
			t.Fatalf("preservation failure did not stop effects: %v", err)
		}
	}
}

func TestStartupRecoveryRejectsMalformedHostIdentityBeforeParentMutation(t *testing.T) {
	record, thread := startupRecoveryFixture(t)
	record.ParentToolCallID = "provider_call_6222020202020202020"
	turn, _ := FindTurn(thread, record.ParentTurnID)
	item, _ := listAny(turn["items"])[0].(map[string]any)
	item["callId"] = record.ParentToolCallID
	item["id"] = "item_provider_owned"
	record.ParentToolItemID = "item_provider_owned"
	threads := &startupRecoveryThreadStoreStub{thread: thread}
	jobs := &startupRecoveryJobStoreStub{record: record}
	security := &startupRecoverySecurityStub{}
	service := NewStartupRecoveryService(StartupRecoveryDependencies{Threads: threads, Jobs: jobs, Security: security})

	if err := service.RecoverInterrupted([]domainjob.Record{record}); err != nil {
		t.Fatal(err)
	}
	if jobs.updateCalls != 0 || threads.patchCalls != 0 || threads.appendCalls != 0 {
		t.Fatalf("malformed identity mutated authority: jobs=%d patch=%d append=%d", jobs.updateCalls, threads.patchCalls, threads.appendCalls)
	}
	if len(security.deadLetters) != 1 || security.deadLetters[0] != "parent_tool_identity_invalid" {
		t.Fatalf("malformed identity dead-letter mismatch: %#v", security.deadLetters)
	}
}

func TestStartupRecoveryRejectsArchivedParentBeforeMutation(t *testing.T) {
	for _, test := range []struct {
		name    string
		archive func(map[string]any)
	}{
		{name: "status", archive: func(thread map[string]any) { thread["status"] = "archived" }},
		{name: "legacy flag", archive: func(thread map[string]any) { thread["archived"] = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			record, thread := startupRecoveryFixture(t)
			record.SecurityBinding = &domainjob.SecurityBinding{}
			test.archive(thread)
			durableAt, err := time.Parse(time.RFC3339Nano, record.RecoveryUpdatedAt)
			if err != nil {
				t.Fatal(err)
			}
			if blocker := RecoveredParentSettlementBlockerAt(record, thread, durableAt); blocker != "parent_thread_archived" {
				t.Fatalf("archived durable blocker=%q", blocker)
			}
			threads := &startupRecoveryThreadStoreStub{thread: thread}
			jobs := &startupRecoveryJobStoreStub{record: record}
			security := &startupRecoverySecurityStub{}
			events := 0
			service := NewStartupRecoveryService(StartupRecoveryDependencies{
				Threads: threads, Jobs: jobs, Security: security,
				RecordEvent: func(map[string]any, string) { events++ },
			})

			if err := service.RecoverInterrupted([]domainjob.Record{record}); err != nil {
				t.Fatal(err)
			}
			if jobs.updateCalls != 0 || threads.patchCalls != 0 || threads.appendCalls != 0 || events != 0 {
				t.Fatalf("archived recovery mutated authority: jobs=%d patch=%d append=%d events=%d",
					jobs.updateCalls, threads.patchCalls, threads.appendCalls, events)
			}
			if len(security.deadLetters) != 1 || security.deadLetters[0] != "parent_thread_archived" {
				t.Fatalf("archived recovery dead-letter mismatch: %#v", security.deadLetters)
			}
		})
	}
}

func TestStartupRecoveryDeadLettersMissingParentAndTurn(t *testing.T) {
	for _, test := range []struct {
		name    string
		prepare func(domainjob.Record, map[string]any) (domainjob.Record, map[string]any)
		reason  string
	}{
		{
			name: "parent thread",
			prepare: func(record domainjob.Record, _ map[string]any) (domainjob.Record, map[string]any) {
				return record, nil
			},
			reason: "parent_thread_missing",
		},
		{
			name: "parent turn",
			prepare: func(record domainjob.Record, thread map[string]any) (domainjob.Record, map[string]any) {
				thread["turns"] = []any{}
				return record, thread
			},
			reason: "parent_turn_missing",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			record, thread := startupRecoveryFixture(t)
			record, thread = test.prepare(record, thread)
			threads := &startupRecoveryThreadStoreStub{thread: thread}
			jobs := &startupRecoveryJobStoreStub{record: record}
			security := &startupRecoverySecurityStub{}
			service := NewStartupRecoveryService(StartupRecoveryDependencies{Threads: threads, Jobs: jobs, Security: security})

			if err := service.RecoverInterrupted([]domainjob.Record{record}); err != nil {
				t.Fatal(err)
			}
			if jobs.updateCalls != 0 || threads.patchCalls != 0 || threads.appendCalls != 0 {
				t.Fatalf("missing parent mutated authority: jobs=%d patch=%d append=%d", jobs.updateCalls, threads.patchCalls, threads.appendCalls)
			}
			if len(security.deadLetters) != 1 || security.deadLetters[0] != test.reason {
				t.Fatalf("missing parent dead-letter mismatch: %#v", security.deadLetters)
			}
		})
	}
}

func TestStartupRecoveryExistingToolResultIsDeduplicated(t *testing.T) {
	record, thread := startupRecoveryFixture(t)
	records, err := RecoveredJobToolResult(RecoveredJobToolResultInput{
		ThreadID: record.ParentThreadID, TurnID: record.ParentTurnID, Record: record,
		Message: "runtime restarted", CallID: record.ParentToolCallID, ToolName: "task",
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, _ := FindTurn(thread, record.ParentTurnID)
	call, _ := listAny(turn["items"])[0].(map[string]any)
	call["status"] = records.Status
	call["finishedAt"] = RecoveredJobSettlementTimestamp(record)
	turn["items"] = append(listAny(turn["items"]), records.ResultItem)
	threads := &startupRecoveryThreadStoreStub{thread: thread}
	service := NewStartupRecoveryService(StartupRecoveryDependencies{
		Threads: threads, Jobs: &startupRecoveryJobStoreStub{record: record}, Security: &startupRecoverySecurityStub{},
	})

	if err := service.settleParentTool(record, "runtime restarted"); err != nil {
		t.Fatal(err)
	}
	if threads.patchCalls != 0 || threads.appendCalls != 0 {
		t.Fatalf("existing result was mutated or duplicated: patch=%d append=%d", threads.patchCalls, threads.appendCalls)
	}
}

func TestStartupRecoveryPropagatesPersistenceFailures(t *testing.T) {
	record, thread := startupRecoveryFixture(t)
	for _, test := range []struct {
		name       string
		threads    *startupRecoveryThreadStoreStub
		jobs       *startupRecoveryJobStoreStub
		settleOnly bool
		want       string
	}{
		{
			name: "job retry", threads: &startupRecoveryThreadStoreStub{thread: thread},
			jobs: &startupRecoveryJobStoreStub{record: record, updateErrAt: 1}, want: "persist job failed",
		},
		{
			name: "delivery retry", threads: &startupRecoveryThreadStoreStub{thread: thread},
			jobs: &startupRecoveryJobStoreStub{record: record, updateErrAt: 2}, want: "persist recovered job retry ledger failed",
		},
		{
			name: "parent status", threads: &startupRecoveryThreadStoreStub{thread: thread, patchErr: errors.New("persist parent status failed")},
			jobs: &startupRecoveryJobStoreStub{record: record}, settleOnly: true, want: "persist parent status failed",
		},
		{
			name: "parent result", threads: &startupRecoveryThreadStoreStub{thread: thread, appendErr: errors.New("persist parent result failed")},
			jobs: &startupRecoveryJobStoreStub{record: record}, settleOnly: true, want: "persist parent result failed",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := NewStartupRecoveryService(StartupRecoveryDependencies{
				Threads: test.threads, Jobs: test.jobs, Security: &startupRecoverySecurityStub{},
				Now: func() time.Time { return time.Date(2026, 7, 20, 1, 2, 3, 0, time.UTC) },
			})
			var err error
			if test.settleOnly {
				err = service.settleParentTool(record, "runtime restarted")
			} else {
				err = service.RecoverInterrupted([]domainjob.Record{record})
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("persistence failure not propagated: %v", err)
			}
		})
	}
}

func TestStartupRecoveryPropagatesDeadLetterPersistenceFailure(t *testing.T) {
	record, thread := startupRecoveryFixture(t)
	security := &startupRecoverySecurityStub{
		blocker:       "parent_security_context_mismatch",
		deadLetterErr: errors.New("persist dead letter failed"),
	}
	service := NewStartupRecoveryService(StartupRecoveryDependencies{
		Threads:  &startupRecoveryThreadStoreStub{thread: thread},
		Jobs:     &startupRecoveryJobStoreStub{record: record},
		Security: security,
	})
	if err := service.RecoverInterrupted([]domainjob.Record{record}); err == nil || !strings.Contains(err.Error(), "persist dead letter failed") {
		t.Fatalf("dead-letter persistence failure not propagated: %v", err)
	}
}

func startupRecoveryFixture(t *testing.T) (domainjob.Record, map[string]any) {
	t.Helper()
	callID, err := domainmodel.NewHostToolCallIDV1(bytes.Repeat([]byte{0x6a}, domainmodel.HostToolCallIDEntropyBytesV1))
	if err != nil {
		t.Fatal(err)
	}
	const threadID = "thread-startup-recovery"
	const turnID = "turn-startup-recovery"
	itemID := domaintoolcall.ToolCallItemIDV1(turnID, callID)
	thread := map[string]any{
		"id": threadID,
		"turns": []any{map[string]any{
			"id": turnID, "threadId": threadID,
			"items": []any{map[string]any{
				"id": itemID, "threadId": threadID, "turnId": turnID,
				"kind": "tool_call", "toolName": "task", "callId": callID, "status": "running",
			}},
		}},
	}
	return domainjob.Record{
		ID: "job-startup-recovery", ParentThreadID: threadID, ParentTurnID: turnID,
		ParentToolItemID: itemID, ParentToolCallID: callID, Kind: "subagent",
		Status: string(domainjob.StatusInterrupted), Background: true,
		RecoveryUpdatedAt: "2026-07-20T01:02:03Z",
	}, thread
}

func TestBackgroundCompletionExactReplaySurvivesLaterCaseLineage(t *testing.T) {
	for _, mode := range []string{"exact", "duplicate", "partial", "changed_timestamp", "sequence_gap", "case_parent", "invalid_context"} {
		t.Run(mode, func(t *testing.T) {
			record, thread := startupRecoveryFixture(t)
			frozen, err := securitytest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
				ThreadID: record.ParentThreadID, TurnID: record.ParentTurnID, WorkspaceRealPath: t.TempDir(),
				TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
				ContextEpoch: 1, IssuedAt: time.Unix(1_700_000_000, 0).UTC(),
			})
			if err != nil {
				t.Fatal(err)
			}
			body, _ := json.Marshal(frozen)
			var contextRecord map[string]any
			if err := json.Unmarshal(body, &contextRecord); err != nil {
				t.Fatal(err)
			}
			parent, _ := FindTurn(thread, record.ParentTurnID)
			parent["securityContext"] = contextRecord
			record.Status = "completed"
			record.CompletionDeliveryStatus = "skipped"
			record.CompletionDeliveryID = "delivery-r131"
			record.CompletionDeliveryItemID = "delivery-item-r131"
			record.CompletionDeliveryAt = "2026-07-20T01:02:03Z"
			drafts := CanonicalBackgroundCompletionLifecycleEventsV1(BuildDurableJobLifecycleEventsV1(
				record.ParentThreadID, record.ParentTurnID, record.ParentToolItemID, record.ParentToolCallID, "task", record,
			), record)
			if len(drafts) != 3 {
				t.Fatal("fixture omitted the complete background lifecycle")
			}
			replay := make([]map[string]any, 0, len(drafts))
			for i, draft := range drafts {
				projected, err := turnapp.SanitizeCaseEventPublication(thread, draft)
				if err != nil {
					t.Fatal(err)
				}
				projected["seq"] = float64(i + 10)
				replay = append(replay, projected)
			}
			thread["historyAuthority"] = "case_boundary_only_v1"
			switch mode {
			case "duplicate":
				replay = append(replay, cloneMap(replay[0]))
			case "partial":
				replay = replay[1:]
			case "changed_timestamp":
				replay[0]["timestamp"] = "2026-07-20T01:02:04Z"
			case "sequence_gap":
				replay[1]["seq"] = float64(99)
			case "invalid_context":
				contextRecord["contextEpoch"] = float64(2)
			case "case_parent":
				caseContext, err := securitytest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
					ThreadID: record.ParentThreadID, TurnID: record.ParentTurnID, WorkspaceRealPath: frozen.WorkspaceRealPath,
					TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, ContextEpoch: 1, IssuedAt: time.Unix(1_700_000_000, 0).UTC(),
				})
				if err != nil {
					t.Fatal(err)
				}
				body, _ = json.Marshal(caseContext)
				if err := json.Unmarshal(body, &contextRecord); err != nil {
					t.Fatal(err)
				}
				parent["securityContext"] = contextRecord
			}
			before, _ := json.Marshal(replay)
			mutations := 0
			operations := BackgroundCompletionLifecycleExactOperationsV1{
				LoadEvents:          func(string) ([]map[string]any, error) { return replay, nil },
				PublicationReserved: func(string) bool { return false },
				ReadThread:          func(string) (map[string]any, error) { return thread, nil },
				ProjectThread:       func(_ string, value map[string]any) (map[string]any, error) { return value, nil },
				NextSequence:        func(string) (int, error) { mutations++; return 100, nil },
				Persist:             func(string, []map[string]any, bool) error { mutations++; return nil },
				MissingThread:       errors.New("missing"), SetHighest: func(string, int) { mutations++ },
				Publish: func(string, []map[string]any) { mutations++ },
			}
			err = ReconcileBackgroundCompletionLifecycleExactV1(drafts, operations)
			if (mode == "exact") != (err == nil) {
				t.Fatalf("mode=%s recovery err=%v", mode, err)
			}
			after, _ := json.Marshal(replay)
			if mutations != 0 || !bytes.Equal(before, after) {
				t.Fatal("replay reconciliation wrote or published history")
			}
		})
	}
}
