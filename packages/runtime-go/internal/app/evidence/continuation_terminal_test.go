package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	apploop "analytix.local/runtime-go/internal/app/loop"
	appmodel "analytix.local/runtime-go/internal/app/model"
	appturn "analytix.local/runtime-go/internal/app/turn"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type continuationTerminalStoreStub struct {
	status      string
	items       []map[string]any
	fields      map[string]any
	bundleCalls int
}

func (stub *continuationTerminalStoreStub) TurnStatus(_, _ string) (string, error) {
	if stub.status == "" {
		return "running", nil
	}
	return stub.status, nil
}

func (stub *continuationTerminalStoreStub) FinishTurnIfActiveWithItemsAndFields(_, _, status string, items []map[string]any, fields map[string]any) (bool, string, error) {
	stub.status = status
	stub.items = items
	stub.fields = fields
	return true, status, nil
}

func (stub *continuationTerminalStoreStub) RecordGeneralTerminalEventBundle(_, _ string) ([]map[string]any, error) {
	stub.bundleCalls++
	return nil, nil
}

func (stub *continuationTerminalStoreStub) RecordEvent(event map[string]any) (map[string]any, []string, error) {
	return event, nil, nil
}

func TestPersistContinuationRecoveryPublishesOnlyHostBoundary(t *testing.T) {
	securityContext, err := testsecurity.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-continuation", TurnID: "turn-continuation", WorkspaceRealPath: "/workspace",
		ContextEpoch: 2, IssuedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &continuationTerminalStoreStub{}
	sentinel := "RECOVERY_MODEL_DRAFT_MUST_NOT_PUBLISH"
	err = PersistContinuation(context.Background(), PersistContinuationInput{
		Store: store,
		Pending: appmodel.PendingToolCall{
			ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
			Model: "deepseek", SecurityContext: securityContext,
		},
		LoopResult: apploop.RuntimeAgentLoopResult{
			AssistantText: sentinel, TerminalRecoveryKind: apploop.RuntimeTerminalRecoveryApplied,
		},
		TerminalReason: TerminalApproval,
		AcceptedAt:     time.Unix(2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(store.items)
	commit, parseErr := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(store.fields["generalTerminalPublication"])
	if parseErr != nil || store.status != "completed" || store.bundleCalls != 1 || len(store.items) != 1 ||
		strings.Contains(string(encoded), sentinel) || commit.TerminalReason != "recovery" || commit.TerminalStatus != "completed" {
		t.Fatalf("recovery was not downgraded to a host boundary: status=%s items=%s commit=%#v calls=%d err=%v", store.status, encoded, commit, store.bundleCalls, parseErr)
	}
	if text, ok := store.items[0]["text"].(string); !ok || text != appturn.GeneralProviderFinalQuarantinedText {
		t.Fatalf("recovery boundary text is not deterministic: %#v", store.items[0])
	}
}

func TestPersistContinuationClassifiesExactOrdinaryLaneWithoutACaseSlot(t *testing.T) {
	securityContext, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-case-ordinary-continuation", TurnID: "turn-case-ordinary-continuation",
		WorkspaceRealPath: "/workspace/case", ContextEpoch: 2, IssuedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	slot, err := domainordinaryresult.NewResultSlotV1("The ordinary read completed.")
	if err != nil {
		t.Fatal(err)
	}
	observed := PersistCaseBoundaryInput{}
	err = PersistContinuation(context.Background(), PersistContinuationInput{
		Pending: appmodel.PendingToolCall{
			ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Model: "deepseek",
			SecurityContext: securityContext, ProviderStepExact: true,
			LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true,
		},
		LoopResult: apploop.RuntimeAgentLoopResult{
			OrdinaryResult: &slot, CandidateOrdinaryWork: true,
			CandidateInputClass: apploop.RuntimeCandidateInputClassOrdinaryOnly,
		},
		TerminalReason: TerminalSuccess, AcceptedAt: time.Unix(2, 0),
		FinalizeCase: func(_ context.Context, input PersistCaseBoundaryInput) (PersistCaseBoundaryResult, error) {
			observed = input
			return PersistCaseBoundaryResult{}, nil
		},
	})
	if err != nil || observed.CaseSlotIntent != CaseSlotNotRequestedV1 || observed.OrdinaryResult == nil ||
		observed.OrdinaryResult.ResultDigest != slot.ResultDigest {
		t.Fatalf("exact ordinary continuation acquired a case slot: input=%#v err=%v", observed, err)
	}
}

func TestPersistContinuationCancellationCarriesExplicitInterruptMetadata(t *testing.T) {
	securityContext, err := testsecurity.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-continuation-cancel", TurnID: "turn-continuation-cancel", WorkspaceRealPath: "/workspace",
		ContextEpoch: 2, IssuedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &continuationTerminalStoreStub{}
	err = PersistContinuation(context.Background(), PersistContinuationInput{
		Store: store,
		Pending: appmodel.PendingToolCall{
			ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
			Model: "deepseek", SecurityContext: securityContext,
		},
		TerminalReason: TerminalCancel,
		AcceptedAt:     time.Unix(2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	commit, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(store.fields["generalTerminalPublication"])
	if err != nil || store.status != "aborted" || store.fields["discard"] != false ||
		store.fields["cancelled"] != true || store.fields["cancelledPendingGates"] != 0 ||
		commit.TerminalEvent["discard"] != false || commit.TerminalEvent["cancelled"] != true ||
		commit.TerminalEvent["cancelledPendingGates"] != json.Number("0") {
		t.Fatalf("continuation cancellation lost explicit interrupt authority: status=%q fields=%#v commit=%#v err=%v", store.status, store.fields, commit, err)
	}
}

func TestPersistContinuationCandidateRequiresCurrentHostGeneralFinalizer(t *testing.T) {
	securityContext, err := testsecurity.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-current-general", TurnID: "turn-current-general", WorkspaceRealPath: "/workspace",
		ContextEpoch: 2, IssuedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &continuationTerminalStoreStub{}
	input := PersistContinuationInput{
		Store: store,
		Pending: appmodel.PendingToolCall{
			ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
			Model: "deepseek", SecurityContext: securityContext,
		},
		LoopResult:     apploop.RuntimeAgentLoopResult{AssistantText: "candidate must remain private"},
		TerminalReason: TerminalSuccess,
		AcceptedAt:     time.Unix(2, 0),
	}
	if err := PersistContinuation(context.Background(), input); err == nil || store.status != "" || len(store.items) != 0 {
		t.Fatalf("candidate bypassed missing current host finalizer: status=%q items=%#v err=%v", store.status, store.items, err)
	}
	called := false
	input.GeneralOperationContext = context.Background()
	input.FinalizeGeneral = func(_ context.Context, finalInput appturn.FinalizeAfterLoopInput) error {
		called = true
		if finalInput.SecurityContext != securityContext {
			t.Fatalf("continuation changed the frozen finalization input: %#v", finalInput)
		}
		return errors.New("current general authority rejected")
	}
	if err := PersistContinuation(context.Background(), input); err == nil || !called || store.status != "" || len(store.items) != 0 {
		t.Fatalf("continuation did not delegate exclusively to current host authority: called=%t status=%q items=%#v err=%v", called, store.status, store.items, err)
	}
	cancelledContext, cancel := context.WithCancel(context.Background())
	cancel()
	called = false
	input.GeneralOperationContext = cancelledContext
	input.FinalizeGeneral = func(context.Context, appturn.FinalizeAfterLoopInput) error {
		called = true
		return nil
	}
	if err := PersistContinuation(context.Background(), input); !errors.Is(err, context.Canceled) || called || store.status != "" || len(store.items) != 0 {
		t.Fatalf("cancelled continuation reached general finalizer: called=%t status=%q items=%#v err=%v", called, store.status, store.items, err)
	}
}

func TestPersistContinuationCandidateQuarantinesProviderDraft(t *testing.T) {
	securityContext, err := testsecurity.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-continuation-quarantine", TurnID: "turn-continuation-quarantine", WorkspaceRealPath: "/workspace",
		ContextEpoch: 3, IssuedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	const sentinel = "甲公司支付2645472元。张三是李四的堂兄。"
	store := &continuationTerminalStoreStub{}
	input := PersistContinuationInput{
		Store: store,
		Pending: appmodel.PendingToolCall{
			ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
			Model: "deepseek", SecurityContext: securityContext,
		},
		LoopResult:              apploop.RuntimeAgentLoopResult{AssistantText: sentinel},
		TerminalReason:          TerminalResume,
		AcceptedAt:              time.Unix(2, 0),
		GeneralOperationContext: context.Background(),
		FinalizeGeneral: func(_ context.Context, finalInput appturn.FinalizeAfterLoopInput) error {
			return appturn.FinalizeAfterLoop(finalInput)
		},
	}
	if err := PersistContinuation(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(store.items)
	if store.status != "completed" || store.bundleCalls != 1 || len(store.items) != 1 ||
		strings.Contains(string(encoded), sentinel) || store.items[0]["text"] != appturn.GeneralProviderFinalQuarantinedText {
		t.Fatalf("continuation provider draft reached publication: status=%q items=%s calls=%d", store.status, encoded, store.bundleCalls)
	}
}
