package control

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	appturn "analytix.local/runtime-go/internal/app/turn"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	casepublication "analytix.local/runtime-go/internal/testsupport/casepublication"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type interruptStoreStub struct {
	changed      bool
	status       string
	finishErr    error
	recordErr    error
	bundleErr    error
	finishItems  []map[string]any
	finishFields map[string]any
	events       []map[string]any
	threadID     string
	turnID       string
	security     domainsecurity.TurnSecurityContext
	publishCalls int
}

func (stub *interruptStoreStub) FinishTurnIfActiveWithItemsAndFields(threadID, turnID, status string, items []map[string]any, fields map[string]any) (bool, string, error) {
	stub.threadID = threadID
	stub.turnID = turnID
	if stub.finishErr != nil {
		return false, "", stub.finishErr
	}
	if stub.changed {
		stub.finishItems = items
		stub.finishFields = fields
	}
	if stub.status != "" {
		status = stub.status
	}
	stub.status = status
	return stub.changed, status, nil
}

func (stub *interruptStoreStub) FinishTurnIfActiveWithAcceptedFinalAuthority(
	threadID, turnID, status string,
	items []map[string]any,
	fields map[string]any,
	privateFinal domainevidence.PrivateAcceptedFinalRecord,
	factAuthority appturn.FactFinalMutationAuthority,
) (bool, string, error) {
	if _, err := appturn.ValidateAcceptedFinalCASAuthority(
		threadID, turnID, status, items, fields, privateFinal, factAuthority,
	); err != nil {
		return false, "", err
	}
	return stub.FinishTurnIfActiveWithItemsAndFields(threadID, turnID, status, items, fields)
}

func (stub *interruptStoreStub) GetThread(threadID string) (map[string]any, error) {
	expectedThreadID := stub.threadID
	if expectedThreadID == "" {
		expectedThreadID = stub.security.ThreadID
	}
	if threadID != expectedThreadID {
		return nil, errors.New("test interrupted thread is unavailable")
	}
	turnID := stub.turnID
	if turnID == "" {
		turnID = stub.security.TurnID
	}
	contextBody, _ := json.Marshal(stub.security)
	securityContext := map[string]any{}
	_ = json.Unmarshal(contextBody, &securityContext)
	items := make([]any, 0, len(stub.finishItems))
	for _, item := range stub.finishItems {
		items = append(items, item)
	}
	status := stub.status
	turn := map[string]any{
		"id": turnID, "status": status, "securityContext": securityContext, "items": items,
	}
	thread := map[string]any{"id": threadID, "securityState": securityContext, "turns": []any{turn}}
	if stub.finishFields == nil {
		turn["status"] = "running"
	} else {
		for key, value := range stub.finishFields {
			turn[key] = value
		}
		if stub.finishFields["acceptedFinal"] != nil {
			record, err := domainevidence.ParseAcceptedFinalRecord(stub.finishFields["acceptedFinal"])
			if err != nil {
				return nil, err
			}
			turn["finishedAt"] = record.AcceptedAt
		}
		if stub.finishFields["generalTerminalPublication"] != nil {
			commit, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(stub.finishFields["generalTerminalPublication"])
			if err != nil {
				return nil, err
			}
			archive, err := domainturnterminal.NewGeneralTerminalPublicationArchiveV1([]domainturnterminal.GeneralTerminalPublicationCommitV1{commit})
			if err != nil {
				return nil, err
			}
			turn["finishedAt"] = commit.CommittedAt
			thread[domainturnterminal.GeneralTerminalPublicationArchiveFieldV1] = domainturnterminal.GeneralTerminalPublicationArchiveV1Map(archive)
		}
	}
	return thread, nil
}

func TestInterruptActiveTurnRetryReturnsPersistedInterruptMetadata(t *testing.T) {
	securityContext := controlGeneralContext(t, "thr_retry", "turn_retry")
	store := &interruptStoreStub{changed: true, security: securityContext}
	first, err := InterruptActiveTurn(InterruptActiveTurnInput{
		Store: store, ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		Discard: true, Cancelled: true, CaseContext: securityContext,
	})
	if err != nil || first.Response["discard"] != true || first.Response["cancelled"] != true {
		t.Fatalf("first interrupt metadata = %#v err=%v", first, err)
	}
	store.changed = false
	retry, err := InterruptActiveTurn(InterruptActiveTurnInput{
		Store: store, ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		Discard: false, Cancelled: false, CaseContext: securityContext,
	})
	if err != nil || retry.Changed || retry.Response["discard"] != true || retry.Response["cancelled"] != true || !retry.Cancelled {
		t.Fatalf("retry lost persisted interrupt metadata = %#v err=%v", retry, err)
	}
}

func (stub *interruptStoreStub) RecordEvent(event map[string]any) (map[string]any, []string, error) {
	stub.events = append(stub.events, event)
	return event, []string{"persist", "publish"}, stub.recordErr
}

func (stub *interruptStoreStub) RecordGeneralTerminalEventBundle(_, _ string) ([]map[string]any, error) {
	stub.publishCalls++
	return nil, stub.bundleErr
}

func (stub *interruptStoreStub) RecordAcceptedFinalEventBundle(bundle []map[string]any) ([]map[string]any, error) {
	if stub.recordErr != nil {
		return nil, stub.recordErr
	}
	recorded := make([]map[string]any, 0, len(bundle))
	for index, event := range bundle {
		item := contracts.CloneMap(event)
		item["seq"] = float64(len(stub.events) + index + 1)
		recorded = append(recorded, item)
	}
	stub.events = append(stub.events, recorded...)
	return recorded, nil
}

func TestInterruptActiveTurnFinishesAndRecordsEvents(t *testing.T) {
	securityContext := controlGeneralContext(t, "thr_1", "turn_1")
	store := &interruptStoreStub{changed: true, security: securityContext}
	result, err := InterruptActiveTurn(InterruptActiveTurnInput{
		Store:       store,
		ThreadID:    "thr_1",
		TurnID:      "turn_1",
		Cancelled:   true,
		CaseContext: securityContext,
		TerminalItems: []map[string]any{{
			"id": "item_1",
		}},
		GateCancellations: []PendingGateCancellation{
			ApprovalGateCancellation("appr_1", GateRecord{ThreadID: "thr_1", TurnID: "turn_1", ItemID: "item_appr", ToolName: "write"}),
		},
	})
	if err != nil {
		t.Fatalf("interrupt active turn: %v", err)
	}
	if result.Status != "aborted" || !result.Changed || !result.Cancelled {
		t.Fatalf("result mismatch: %#v", result)
	}
	if store.finishFields["discard"] != false || len(store.finishItems) != 1 ||
		stringField(store.finishItems[0], "kind") != "error" || store.publishCalls != 1 {
		t.Fatalf("finish input mismatch: fields=%#v items=%#v", store.finishFields, store.finishItems)
	}
	if kinds := interruptEventKinds(store.events); len(kinds) != 1 || kinds[0] != "approval_resolved" {
		t.Fatalf("event order mismatch: %#v", kinds)
	}
}

func TestInterruptActiveTurnSkipsTerminalEventsWhenUnchanged(t *testing.T) {
	securityContext := controlGeneralContext(t, "thr_1", "turn_1")
	store := &interruptStoreStub{changed: true, security: securityContext}
	seed, err := appturn.CommitGeneralFailureTerminal(appturn.PersistFailureInput{
		Store: store, SecurityContext: securityContext, TerminalReason: "source_unavailable",
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		Failure: domainfailure.New("source_probe_unavailable", nil), FinishedAt: "2026-07-15T00:00:00Z",
	})
	if err != nil || !seed.Changed || seed.Status != "completed" {
		t.Fatalf("seed completed terminal: result=%#v err=%v", seed, err)
	}
	store.changed = false
	beforeItems, beforeFields, beforePublishCalls := store.finishItems, store.finishFields, store.publishCalls
	result, err := InterruptActiveTurn(InterruptActiveTurnInput{
		Store:       store,
		ThreadID:    "thr_1",
		TurnID:      "turn_1",
		Discard:     true,
		CaseContext: securityContext,
	})
	if err != nil {
		t.Fatalf("interrupt unchanged turn: %v", err)
	}
	if result.Status != "completed" || result.Cancelled {
		t.Fatalf("unchanged result mismatch: %#v", result)
	}
	if result.Changed || result.Response["discard"] != false || result.Response["cancelled"] != false {
		t.Fatalf("unchanged interrupt claimed request effects: %#v", result)
	}
	if len(store.events) != 0 {
		t.Fatalf("unchanged turn should not record terminal events: %#v", store.events)
	}
	if store.publishCalls != beforePublishCalls+1 || fmt.Sprint(store.finishItems) != fmt.Sprint(beforeItems) ||
		fmt.Sprint(store.finishFields) != fmt.Sprint(beforeFields) {
		t.Fatalf("interrupt changed the completed winner: calls=%d items=%#v fields=%#v", store.publishCalls, store.finishItems, store.finishFields)
	}
}

func TestInterruptActiveTurnReturnsFinishErrorAndCollectsRecordError(t *testing.T) {
	finishErr := errors.New("finish failed")
	securityContext := controlGeneralContext(t, "thr_1", "turn_1")
	_, err := InterruptActiveTurn(InterruptActiveTurnInput{
		Store:       &interruptStoreStub{finishErr: finishErr, security: securityContext},
		ThreadID:    "thr_1",
		TurnID:      "turn_1",
		CaseContext: securityContext,
	})
	if !errors.Is(err, finishErr) {
		t.Fatalf("expected finish error, got %v", err)
	}
	recordErr := errors.New("record failed")
	result, err := InterruptActiveTurn(InterruptActiveTurnInput{
		Store:       &interruptStoreStub{changed: true, recordErr: recordErr, security: securityContext},
		ThreadID:    "thr_1",
		TurnID:      "turn_1",
		CaseContext: securityContext,
		GateCancellations: []PendingGateCancellation{
			ApprovalGateCancellation("appr_1", GateRecord{ThreadID: "thr_1", TurnID: "turn_1", ItemID: "item_appr", ToolName: "write"}),
		},
	})
	if err != nil {
		t.Fatalf("record errors should be returned in result, got %v", err)
	}
	if !errors.Is(result.RecordError, recordErr) {
		t.Fatalf("expected record error in result, got %v", result.RecordError)
	}
}

func controlGeneralContext(t *testing.T, threadID, turnID string) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/workspace",
		ContextEpoch: 2, IssuedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func TestInterruptCaseTurnUsesFinalEvidenceGateAndDropsDraft(t *testing.T) {
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr_case", TurnID: "turn_case", WorkspaceRealPath: "/workspace", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-a"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 2, IssuedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &interruptStoreStub{
		changed: true, security: securityContext,
		threadID: securityContext.ThreadID, turnID: securityContext.TurnID,
	}
	finalizer, err := casepublication.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	result, err := InterruptActiveTurn(InterruptActiveTurnInput{
		Store: store, ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		CaseContext: securityContext, CaseFinalizer: finalizer, AcceptedAt: time.Unix(2, 0),
		TerminalEvents: []map[string]any{{"kind": "assistant_text_delta", "delta": "伪造金额 420 万元"}},
		GateCancellations: []PendingGateCancellation{
			ApprovalGateCancellation("appr_case", GateRecord{
				ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
				ItemID: "item_appr_case", ToolName: "write_report",
			}),
		},
		GateCancellationsSettled: true,
	})
	if err != nil || result.Status != "aborted" || store.finishFields["acceptedFinal"] == nil || len(store.finishItems) != 2 {
		t.Fatalf("case interrupt finalization mismatch: result=%#v items=%#v fields=%#v err=%v", result, store.finishItems, store.finishFields, err)
	}
	encoded, _ := json.Marshal(store.finishItems)
	if strings.Contains(string(encoded), "420 万元") || strings.Contains(string(encoded), "伪造金额") {
		t.Fatalf("case interrupt materialized provider draft: %s", encoded)
	}
	if kinds := interruptEventKinds(store.events); len(kinds) != 4 || kinds[len(kinds)-1] != "turn_aborted" {
		t.Fatalf("case interrupt did not publish one gated terminal sequence: %#v", kinds)
	}
	terminal := store.events[len(store.events)-1]
	if terminal["cancelled"] != true || fmt.Sprint(terminal["cancelledPendingGates"]) != "1" {
		t.Fatalf("paused case gate was not included in accepted interrupt metadata: %#v", terminal)
	}
}

func TestInterruptCaseRetryRestoresCanonicalPrivateIntentAfterEventGap(t *testing.T) {
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr_case_retry", TurnID: "turn_case_retry", WorkspaceRealPath: "/workspace", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-a"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 2, IssuedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("accepted-final event gap")
	store := &interruptStoreStub{
		changed: true, recordErr: injected, security: securityContext,
		threadID: securityContext.ThreadID, turnID: securityContext.TurnID,
	}
	finalizer, err := casepublication.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := InterruptActiveTurn(InterruptActiveTurnInput{
		Store: store, ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		CaseContext: securityContext, CaseFinalizer: finalizer, AcceptedAt: time.Unix(2, 0),
		Discard: true, Cancelled: true, GateCancellationsSettled: true,
	})
	if !errors.Is(err, injected) || first.Response != nil || store.finishFields["acceptedFinal"] == nil {
		t.Fatalf("first case interrupt did not preserve a recoverable CAS winner: result=%#v fields=%#v err=%v", first, store.finishFields, err)
	}
	store.changed = false
	store.recordErr = nil
	retry, err := InterruptActiveTurn(InterruptActiveTurnInput{
		Store: store, ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		CaseContext: securityContext, CaseFinalizer: finalizer, AcceptedAt: time.Unix(99, 0),
		Discard: false, Cancelled: false, GateCancellationsSettled: true,
	})
	if err != nil || retry.Changed || retry.Status != "aborted" || !retry.Cancelled ||
		retry.Response["discard"] != true || retry.Response["cancelled"] != true {
		t.Fatalf("case interrupt retry did not use the original private terminal intent: result=%#v err=%v", retry, err)
	}
	if kinds := interruptEventKinds(store.events); len(kinds) != 4 || kinds[len(kinds)-1] != "turn_aborted" {
		t.Fatalf("case interrupt event gap was not reconciled exactly once: %#v", kinds)
	}
}

func interruptEventKinds(events []map[string]any) []string {
	kinds := make([]string, 0, len(events))
	for _, event := range events {
		kind, _ := event["kind"].(string)
		kinds = append(kinds, kind)
	}
	return kinds
}
