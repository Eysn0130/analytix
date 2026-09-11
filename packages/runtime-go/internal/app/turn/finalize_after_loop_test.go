package turn

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	appusage "analytix.local/runtime-go/internal/app/usage"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type finalizeStoreStub struct {
	status         string
	statusErr      error
	appendItems    []map[string]any
	atomicItems    []map[string]any
	events         []map[string]any
	finishChanged  bool
	finishStatus   string
	finishErr      error
	terminalFields map[string]any
	bundleCalls    int
}

func TestGeneralCompletionPublicationInputCannotCarryProviderPayload(t *testing.T) {
	for _, inputType := range []reflect.Type{
		reflect.TypeOf(CommitCompletionInput{}),
		reflect.TypeOf(FinalizeAfterLoopInput{}),
	} {
		for _, forbidden := range []string{"AssistantText", "Events", "Result", "Usage", "CacheDiagnostics"} {
			if _, ok := inputType.FieldByName(forbidden); ok {
				t.Fatalf("%s restored forbidden provider payload field %s", inputType.Name(), forbidden)
			}
		}
	}
}

func TestEveryGeneralCandidateTerminalReasonPublishesOnlyHostBoundary(t *testing.T) {
	for _, disposition := range domainterminal.AllDispositionsV1() {
		if !disposition.CandidateAllowed {
			continue
		}
		t.Run(disposition.Reason, func(t *testing.T) {
			store := &finalizeStoreStub{status: "running", finishChanged: true}
			input := FinalizeAfterLoopInput{
				Store: store, SecurityContext: generalCompletionContext(t), ThreadID: "thr_1", TurnID: "turn_1",
				TerminalReason: disposition.Reason, Now: "2026-01-02T03:04:05Z",
			}
			if err := FinalizeAfterLoop(input); err != nil {
				t.Fatal(err)
			}
			if len(store.atomicItems) != 1 || store.atomicItems[0]["text"] != GeneralProviderFinalQuarantinedText {
				t.Fatalf("candidate terminal published non-host text: %#v", store.atomicItems)
			}
		})
	}
}

func (stub *finalizeStoreStub) TurnStatus(_, _ string) (string, error) {
	if stub.statusErr != nil {
		return "", stub.statusErr
	}
	return stub.status, nil
}

func (stub *finalizeStoreStub) FinishTurnIfActiveWithItemsAndFields(_, _, status string, items []map[string]any, fields map[string]any) (bool, string, error) {
	if stub.finishErr != nil {
		return false, "", stub.finishErr
	}
	stub.atomicItems = append(stub.atomicItems, items...)
	stub.appendItems = append(stub.appendItems, items...)
	stub.terminalFields = fields
	if stub.finishStatus != "" {
		return stub.finishChanged, stub.finishStatus, nil
	}
	return stub.finishChanged, status, nil
}

func (stub *finalizeStoreStub) RecordGeneralTerminalEventBundle(_, _ string) ([]map[string]any, error) {
	stub.bundleCalls++
	if len(stub.atomicItems) > 0 {
		stub.events = append(stub.events, map[string]any{"kind": "item_completed"})
	}
	publication, _ := stub.terminalFields["generalTerminalPublication"].(map[string]any)
	usage, _ := publication["usageEvent"].(map[string]any)
	stub.events = append(stub.events, usage, map[string]any{"kind": "turn_completed"})
	return stub.events, nil
}

func TestFinalizeAfterLoopPublishesOnlyHostQuarantineBoundary(t *testing.T) {
	store := &finalizeStoreStub{status: "running", finishChanged: true}
	err := FinalizeAfterLoop(FinalizeAfterLoopInput{
		Store:           store,
		SecurityContext: generalCompletionContext(t),
		ThreadID:        "thr_1",
		TurnID:          "turn_1",
		Model:           "gpt-5",
		Now:             "2026-01-02T03:04:05Z",
		Telemetry: appusage.NewTerminalTelemetryV1(domainmodel.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		}, map[string]any{"prefixHash": "abc"}),
		UsageSource: "subagent",
		ChildRunID:  "child_1",
	})
	if err != nil {
		t.Fatalf("finalize after loop: %v", err)
	}
	if len(store.appendItems) != 1 || store.appendItems[0]["text"] != GeneralProviderFinalQuarantinedText {
		t.Fatalf("assistant item mismatch: %#v", store.appendItems)
	}
	if len(store.atomicItems) != 1 {
		t.Fatalf("completion must attach assistant text only inside the terminal CAS: atomicItems=%#v", store.atomicItems)
	}
	publicationBody, _ := json.Marshal(store.terminalFields["generalTerminalPublication"])
	if store.bundleCalls != 1 || store.terminalFields["generalTerminalCASBinding"] == nil ||
		store.terminalFields["generalTerminalPublication"] == nil || string(publicationBody) == "" {
		t.Fatalf("completion did not commit and reconcile one terminal outbox: fields=%#v calls=%d", store.terminalFields, store.bundleCalls)
	}
	if kinds := eventKinds(store.events); len(kinds) != 3 || kinds[0] != "item_completed" || kinds[1] != "usage" || kinds[2] != "turn_completed" {
		t.Fatalf("event order mismatch: %#v", kinds)
	}
	usage := store.events[1]
	if usage["usageSource"] != "subagent" || usage["childRunId"] != "child_1" {
		t.Fatalf("usage lineage mismatch: %#v", usage)
	}
}

func TestFinalizeAfterLoopInputCannotCarryReasoningOrProviderText(t *testing.T) {
	store := &finalizeStoreStub{status: "running", finishChanged: true}
	err := FinalizeAfterLoop(FinalizeAfterLoopInput{
		Store:           store,
		SecurityContext: generalCompletionContext(t),
		ThreadID:        "thr_1",
		TurnID:          "turn_1",
		Model:           "gpt-5",
		Now:             "2026-01-02T03:04:05Z",
	})
	if err != nil {
		t.Fatalf("finalize after loop: %v", err)
	}
	if len(store.appendItems) != 1 || store.appendItems[0]["kind"] != "assistant_text" {
		t.Fatalf("reasoning must not be persisted as a turn item: %#v", store.appendItems)
	}
	if store.appendItems[0]["text"] != GeneralProviderFinalQuarantinedText {
		t.Fatalf("provider final was not replaced by the host boundary: %#v", store.appendItems)
	}
	if len(store.atomicItems) != 1 {
		t.Fatalf("completion must use the atomic terminal write: atomicItems=%#v", store.atomicItems)
	}
	if kinds := eventKinds(store.events); len(kinds) != 3 ||
		kinds[0] != "item_completed" ||
		kinds[1] != "usage" ||
		kinds[2] != "turn_completed" {
		t.Fatalf("event order mismatch: %#v", kinds)
	}
	for _, event := range store.events {
		if event["kind"] == "assistant_reasoning_delta" {
			t.Fatalf("reasoning event must not be recorded: %#v", store.events)
		}
	}
}

func TestFinalizeAfterLoopPreservesAllowedReasonAndRejectsRecoveryCandidate(t *testing.T) {
	store := &finalizeStoreStub{status: "running", finishChanged: true}
	if err := FinalizeAfterLoop(FinalizeAfterLoopInput{
		Store: store, SecurityContext: generalCompletionContext(t), ThreadID: "thr_1", TurnID: "turn_1",
		TerminalReason: "approval", Now: "2026-01-02T03:04:05Z",
	}); err != nil {
		t.Fatal(err)
	}
	commit, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(store.terminalFields["generalTerminalPublication"])
	if err != nil || commit.TerminalReason != "approval" || commit.TerminalStatus != "completed" {
		t.Fatalf("allowed terminal reason was not bound: commit=%#v err=%v", commit, err)
	}

	blocked := &finalizeStoreStub{status: "running", finishChanged: true}
	err = FinalizeAfterLoop(FinalizeAfterLoopInput{
		Store: blocked, SecurityContext: generalCompletionContext(t), ThreadID: "thr_1", TurnID: "turn_1",
		TerminalReason: "recovery", Now: "2026-01-02T03:04:05Z",
	})
	if err == nil || len(blocked.atomicItems) != 0 || blocked.terminalFields != nil || blocked.bundleCalls != 0 {
		t.Fatalf("recovery candidate reached terminal CAS: fields=%#v items=%#v calls=%d err=%v", blocked.terminalFields, blocked.atomicItems, blocked.bundleCalls, err)
	}
}

func TestFinalizeAfterLoopPublishesBoundaryEvenWithoutProviderText(t *testing.T) {
	store := &finalizeStoreStub{status: "running", finishChanged: true}
	err := FinalizeAfterLoop(FinalizeAfterLoopInput{
		Store:           store,
		SecurityContext: generalCompletionContext(t),
		ThreadID:        "thr_1",
		TurnID:          "turn_1",
		Model:           "gpt-5",
		Now:             "2026-01-02T03:04:05Z",
	})
	if err != nil {
		t.Fatalf("finalize empty text: %v", err)
	}
	if len(store.appendItems) != 1 || store.appendItems[0]["text"] != GeneralProviderFinalQuarantinedText {
		t.Fatalf("successful provider completion must publish only the host boundary: %#v", store.appendItems)
	}
	if kinds := eventKinds(store.events); len(kinds) != 3 || kinds[0] != "item_completed" || kinds[1] != "usage" || kinds[2] != "turn_completed" {
		t.Fatalf("host boundary events mismatch: %#v", kinds)
	}
}

func TestFinalizeAfterLoopSkipsTerminalTurn(t *testing.T) {
	store := &finalizeStoreStub{status: "aborted"}
	if err := FinalizeAfterLoop(FinalizeAfterLoopInput{Store: store, ThreadID: "thr_1", TurnID: "turn_1"}); err != nil {
		t.Fatalf("terminal finalize: %v", err)
	}
	if len(store.appendItems) != 0 || len(store.events) != 0 {
		t.Fatalf("terminal turn should not be mutated: items=%#v events=%#v", store.appendItems, store.events)
	}
}

func TestFinalizeAfterLoopPropagatesStoreErrors(t *testing.T) {
	statusErr := errors.New("status failed")
	err := FinalizeAfterLoop(FinalizeAfterLoopInput{
		Store:    &finalizeStoreStub{statusErr: statusErr},
		ThreadID: "thr_1",
		TurnID:   "turn_1",
	})
	if !errors.Is(err, statusErr) {
		t.Fatalf("expected status error, got %v", err)
	}
	finishErr := errors.New("finish failed")
	err = FinalizeAfterLoop(FinalizeAfterLoopInput{
		Store:           &finalizeStoreStub{status: "running", finishErr: finishErr},
		SecurityContext: generalCompletionContext(t),
		ThreadID:        "thr_1",
		TurnID:          "turn_1",
	})
	if !errors.Is(err, finishErr) {
		t.Fatalf("expected finish error, got %v", err)
	}
}

func generalCompletionContext(t *testing.T) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := testsecurity.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr_1", TurnID: "turn_1", WorkspaceRealPath: "/workspace", ContextEpoch: 1, IssuedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func eventKinds(events []map[string]any) []string {
	kinds := make([]string, 0, len(events))
	for _, event := range events {
		kind, _ := event["kind"].(string)
		kinds = append(kinds, kind)
	}
	return kinds
}
