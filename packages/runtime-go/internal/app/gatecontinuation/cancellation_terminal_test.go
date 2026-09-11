package gatecontinuation

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	continuationapp "analytix.local/runtime-go/internal/app/continuation"
	controlapp "analytix.local/runtime-go/internal/app/control"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	apploop "analytix.local/runtime-go/internal/app/loop"
	appmodel "analytix.local/runtime-go/internal/app/model"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	continuationstoreport "analytix.local/runtime-go/internal/ports/continuationstore"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func gateContinuationTestToolCallID(seed string) string {
	entropy := sha256.Sum256([]byte("analytix.gate-continuation-test-tool-call/v1\x00" + seed))
	identity, err := domainmodel.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		panic(err)
	}
	return identity
}

func TestApprovalAndUserInputLateCancellationCloseOneFixedTerminal(t *testing.T) {
	for _, kind := range []string{domaincontinuation.KindApproval, domaincontinuation.KindUserInput} {
		kind := kind
		t.Run(kind, func(t *testing.T) {
			now := time.Now().UTC()
			pending := cancellationPendingFixture(t, kind, now)
			registry := controlapp.NewGateRegistry[appmodel.PendingToolCall]()
			manager := controlapp.NewApprovalUserInputManager()
			store := newCancellationGateStore()
			continuations := continuationapp.NewService(newCancellationAuthority(), store)
			gateID := controlapp.SecureGateID(kind, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
			itemID := "item_" + gateID
			receipt, err := continuations.IssuePendingHost(kind, gateID, itemID, pending, now)
			if err != nil {
				t.Fatal(err)
			}
			record := controlapp.GateRecord{
				ThreadID: pending.ThreadID, TurnID: pending.TurnID, ItemID: itemID,
				ToolName: pending.Call.Name, ContinuationReceiptID: receipt.ReceiptID,
			}
			if kind == domaincontinuation.KindApproval {
				if !registry.RegisterPendingApproval(gateID, record, pending) {
					t.Fatal("register approval gate")
				}
				manager.RequestApproval(gateID, pending.Call.Name)
			} else {
				if !registry.RegisterPendingUserInput(gateID, record, pending) {
					t.Fatal("register user-input gate")
				}
				manager.RequestUserInput(gateID, "Continue?")
			}

			operationContext, cancelOperation := context.WithCancel(context.Background())
			getThreadCalls := 0
			store.getThread = func(string) (map[string]any, error) {
				getThreadCalls++
				if getThreadCalls == 1 {
					cancelOperation()
					return nil, errors.New("current authority observation interrupted")
				}
				return map[string]any{"turns": []any{map[string]any{
					"id": pending.TurnID,
					"items": []any{map[string]any{
						"id": itemID, "status": "pending",
					}},
				}}}, nil
			}
			var failureCalls, finalizeCalls, executionCalls, completionCalls, cancellationCalls int
			var failureCause error
			dependencies := Dependencies{
				Store: store, Registry: registry, Manager: manager, Continuations: continuations,
				TerminalStatus:  func(string, string) (string, bool, error) { return "", false, nil },
				ValidateCatalog: func(appmodel.PendingToolCall) error { return nil },
				ExecuteAndSettle: func(context.Context, appmodel.PendingToolCall) (apploop.SettledToolExecution, error) {
					executionCalls++
					return apploop.SettledToolExecution{}, nil
				},
				Complete: func(context.Context, appmodel.PendingToolCall, any, bool) (apploop.RuntimeAgentLoopResult, error) {
					completionCalls++
					return apploop.RuntimeAgentLoopResult{AssistantText: "CANDIDATE_SENTINEL"}, nil
				},
				CompleteSettled: func(context.Context, appmodel.PendingToolCall, apploop.SettledToolExecution) (apploop.RuntimeAgentLoopResult, error) {
					completionCalls++
					return apploop.RuntimeAgentLoopResult{AssistantText: "CANDIDATE_SENTINEL"}, nil
				},
				RecordFailure: func(_ appmodel.PendingToolCall, cause error) error {
					failureCalls++
					failureCause = cause
					return nil
				},
				Finalize: func(context.Context, appmodel.PendingToolCall, apploop.RuntimeAgentLoopResult, evidenceapp.TerminalReason) error {
					finalizeCalls++
					return nil
				},
				RecordCancellations: func(cancellations []controlapp.PendingGateCancellation, reason string) error {
					if len(cancellations) != 1 || reason != "turn_cancelled" {
						t.Fatalf("cancellation record = %#v reason=%q", cancellations, reason)
					}
					cancellationCalls++
					return nil
				},
			}

			var result controlapp.ActionResult
			if kind == domaincontinuation.KindApproval {
				result = Approve(operationContext, controlapp.ApprovalDecision{ApprovalID: gateID, Decision: "allow"}, dependencies)
			} else {
				result = RespondUserInput(operationContext, controlapp.UserInputResponse{InputID: gateID, Answers: []map[string]string{{"id": "confirm", "value": "yes"}}}, dependencies)
			}
			if result.StatusCode != statusConflict || result.Body["code"] != "turn_cancelled" {
				t.Fatalf("late cancellation result = %#v", result)
			}
			if failureCalls != 1 || !errors.Is(failureCause, context.Canceled) || finalizeCalls != 0 {
				t.Fatalf("terminal calls: failure=%d cause=%v finalize=%d", failureCalls, failureCause, finalizeCalls)
			}
			if executionCalls != 0 || completionCalls != 0 || cancellationCalls != 1 {
				t.Fatalf("continuation leaked: execute=%d complete=%d cancellation=%d", executionCalls, completionCalls, cancellationCalls)
			}
			disposition, err := store.ResolveDisposition(context.Background(), gateID)
			if err != nil || disposition.Status != domaincontinuation.StatusInterrupted || disposition.ReasonCode != "turn_cancelled" {
				t.Fatalf("disposition = %#v err=%v", disposition, err)
			}

			if kind == domaincontinuation.KindApproval {
				result = Approve(context.Background(), controlapp.ApprovalDecision{ApprovalID: gateID, Decision: "allow"}, dependencies)
			} else {
				result = RespondUserInput(context.Background(), controlapp.UserInputResponse{InputID: gateID}, dependencies)
			}
			if result.StatusCode != statusConflict || result.Body["code"] != "gate_continuation_unavailable" {
				t.Fatalf("consumed gate retry result = %#v", result)
			}
			if failureCalls != 1 || executionCalls != 0 || completionCalls != 0 || cancellationCalls != 1 {
				t.Fatalf("consumed gate retried work: failure=%d execute=%d complete=%d cancellation=%d", failureCalls, executionCalls, completionCalls, cancellationCalls)
			}
		})
	}
}

func TestTerminalManagerUnknownFailureStopsBeforeTerminalSettlement(t *testing.T) {
	manager := failingTerminalManager{status: 500, body: map[string]any{"code": "manager_failed"}}
	for _, kind := range []string{"approval", "user_input"} {
		if err := settleTerminalManager(manager, kind, "gate"); err == nil {
			t.Fatalf("%s manager failure was treated as settled", kind)
		}
	}
}

func TestSettleDrainedRejectsReceiptPendingMismatchBeforeSideEffects(t *testing.T) {
	now := time.Now().UTC()
	pending := cancellationPendingFixture(t, domaincontinuation.KindApproval, now)
	store := newCancellationGateStore()
	continuations := continuationapp.NewService(newCancellationAuthority(), store)
	gateID := controlapp.SecureGateID(
		domaincontinuation.KindApproval, pending.ThreadID, pending.TurnID,
		pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID,
	)
	itemID := "item_" + gateID
	receipt, err := continuations.IssuePendingHost(domaincontinuation.KindApproval, gateID, itemID, pending, now)
	if err != nil {
		t.Fatal(err)
	}
	corrupted := pending
	corrupted.Model = "model-from-another-claim"
	manager := &countingTerminalManager{}
	cancellationCalls := 0
	_, err = SettleDrainedForTerminal([]controlapp.DrainedGate[appmodel.PendingToolCall]{
		{
			Kind: "approval", ID: gateID, ClaimToken: 1,
			Record: controlapp.GateRecord{
				ThreadID: pending.ThreadID, TurnID: pending.TurnID, ItemID: itemID,
				ToolName: pending.Call.Name, ContinuationReceiptID: receipt.ReceiptID,
			},
			Pending: corrupted,
		},
	}, "runtime_shutdown", Dependencies{
		Store: store, Manager: manager, Continuations: continuations,
		RecordCancellations: func([]controlapp.PendingGateCancellation, string) error {
			cancellationCalls++
			return nil
		},
	})
	if err == nil {
		t.Fatal("mismatched signed pending authority was accepted")
	}
	if manager.Calls() != 0 || cancellationCalls != 0 || store.PatchCalls() != 0 || store.EventCalls() != 0 {
		t.Fatalf(
			"mismatched batch produced side effects: manager=%d cancellations=%d patches=%d events=%d",
			manager.Calls(), cancellationCalls, store.PatchCalls(), store.EventCalls(),
		)
	}
	if _, err := store.ResolveDisposition(context.Background(), gateID); !errors.Is(err, continuationstoreport.ErrNotFound) {
		t.Fatalf("mismatched batch consumed its signed receipt: %v", err)
	}
}

func TestSignedResolutionCannotBeDowngradedByTerminalSettlement(t *testing.T) {
	tests := []struct {
		kind, signedStatus, reason, publicStatus, decision string
	}{
		{domaincontinuation.KindApproval, domaincontinuation.StatusAllowed, "approval_allowed", "allowed", "allow"},
		{domaincontinuation.KindApproval, domaincontinuation.StatusDenied, "approval_denied", "denied", "deny"},
		{domaincontinuation.KindUserInput, domaincontinuation.StatusSubmitted, "user_input_submitted", "submitted", ""},
		{domaincontinuation.KindUserInput, domaincontinuation.StatusCancelled, "user_input_cancelled", "cancelled", ""},
	}
	for _, test := range tests {
		test := test
		t.Run(test.kind+"_"+test.signedStatus, func(t *testing.T) {
			now := time.Now().UTC()
			pending := cancellationPendingFixture(t, test.kind, now)
			store := newCancellationGateStore()
			continuations := continuationapp.NewService(newCancellationAuthority(), store)
			manager := controlapp.NewApprovalUserInputManager()
			gateID := controlapp.SecureGateID(test.kind, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
			itemID := "item_" + gateID
			receipt, err := continuations.IssuePendingHost(test.kind, gateID, itemID, pending, now)
			if err != nil {
				t.Fatal(err)
			}
			record := controlapp.GateRecord{
				ThreadID: pending.ThreadID, TurnID: pending.TurnID, ItemID: itemID,
				ToolName: pending.Call.Name, Prompt: "Continue?", ContinuationReceiptID: receipt.ReceiptID,
			}
			store.getThread = func(string) (map[string]any, error) { return gateThreadFixture(record, "pending"), nil }
			if test.kind == domaincontinuation.KindApproval {
				manager.RequestApproval(gateID, pending.Call.Name)
			} else {
				manager.RequestUserInput(gateID, "Continue?")
			}
			if err := continuations.DisposePendingHost(gateID, test.signedStatus, test.reason, now, pending); err != nil {
				t.Fatal(err)
			}
			resolutionCalls, cancellationCalls := 0, 0
			cancellations, err := SettleDrainedForTerminal(
				[]controlapp.DrainedGate[appmodel.PendingToolCall]{
					{Kind: test.kind, ID: gateID, ClaimToken: 1, Record: record, Pending: pending},
				},
				"runtime_shutdown",
				Dependencies{
					Store: store, Manager: manager, Continuations: continuations,
					RecordResolution: func(event map[string]any) error {
						if stringFieldForGateTest(event, "status") != test.publicStatus {
							t.Fatalf("public resolution = %#v", event)
						}
						resolutionCalls++
						return nil
					},
					RecordCancellations: func([]controlapp.PendingGateCancellation, string) error {
						cancellationCalls++
						return nil
					},
				},
			)
			if err != nil || len(cancellations) != 0 || resolutionCalls != 1 || cancellationCalls != 0 || store.PatchCalls() != 1 {
				t.Fatalf("signed resolution settlement mismatch: cancellations=%#v resolution=%d cancellation=%d patches=%d err=%v", cancellations, resolutionCalls, cancellationCalls, store.PatchCalls(), err)
			}
			_, disposition, err := continuations.ResolveTrustedDisposition(context.Background(), gateID)
			if err != nil || disposition.Status != test.signedStatus || disposition.ReasonCode != test.reason {
				t.Fatalf("signed disposition was downgraded: %#v err=%v", disposition, err)
			}
			if test.kind == domaincontinuation.KindApproval {
				status, decision, exists := manager.ApprovalDisposition(gateID)
				if !exists || status != test.publicStatus || decision != test.decision {
					t.Fatalf("approval manager projection = status=%q decision=%q exists=%v", status, decision, exists)
				}
			} else if status, exists := manager.UserInputDisposition(gateID); !exists || status != test.publicStatus {
				t.Fatalf("user-input manager projection = status=%q exists=%v", status, exists)
			}
		})
	}
}

func TestTerminalManagerConflictRequiresExactDisposition(t *testing.T) {
	now := time.Now().UTC()
	pending := cancellationPendingFixture(t, domaincontinuation.KindApproval, now)
	store := newCancellationGateStore()
	continuations := continuationapp.NewService(newCancellationAuthority(), store)
	manager := controlapp.NewApprovalUserInputManager()
	gateID := controlapp.SecureGateID(domaincontinuation.KindApproval, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
	itemID := "item_" + gateID
	receipt, err := continuations.IssuePendingHost(domaincontinuation.KindApproval, gateID, itemID, pending, now)
	if err != nil {
		t.Fatal(err)
	}
	record := controlapp.GateRecord{ThreadID: pending.ThreadID, TurnID: pending.TurnID, ItemID: itemID, ToolName: pending.Call.Name, ContinuationReceiptID: receipt.ReceiptID}
	manager.RequestApproval(gateID, pending.Call.Name)
	if status, _ := manager.ResolveApproval(gateID, "allow"); status != statusOK {
		t.Fatal("pre-resolve approval")
	}
	cancellationCalls := 0
	_, err = SettleDrainedForTerminal(
		[]controlapp.DrainedGate[appmodel.PendingToolCall]{{Kind: "approval", ID: gateID, ClaimToken: 1, Record: record, Pending: pending}},
		"runtime_shutdown",
		Dependencies{
			Store: store, Manager: manager, Continuations: continuations,
			RecordCancellations: func([]controlapp.PendingGateCancellation, string) error {
				cancellationCalls++
				return nil
			},
		},
	)
	if err == nil || cancellationCalls != 0 || store.PatchCalls() != 0 || store.EventCalls() != 0 {
		t.Fatalf("opposite manager state was accepted: cancellations=%d patches=%d events=%d err=%v", cancellationCalls, store.PatchCalls(), store.EventCalls(), err)
	}
	if _, err := store.ResolveDisposition(context.Background(), gateID); !errors.Is(err, continuationstoreport.ErrNotFound) {
		t.Fatalf("manager conflict consumed the continuation receipt: %v", err)
	}
}

func TestResolutionEventFailureRetainsNonExecutableClaimForHostRetry(t *testing.T) {
	now := time.Now().UTC()
	pending := cancellationPendingFixture(t, domaincontinuation.KindApproval, now)
	store := newCancellationGateStore()
	continuations := continuationapp.NewService(newCancellationAuthority(), store)
	manager := controlapp.NewApprovalUserInputManager()
	registry := controlapp.NewGateRegistry[appmodel.PendingToolCall]()
	gateID := controlapp.SecureGateID(domaincontinuation.KindApproval, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
	itemID := "item_" + gateID
	receipt, err := continuations.IssuePendingHost(domaincontinuation.KindApproval, gateID, itemID, pending, now)
	if err != nil {
		t.Fatal(err)
	}
	record := controlapp.GateRecord{ThreadID: pending.ThreadID, TurnID: pending.TurnID, ItemID: itemID, ToolName: pending.Call.Name, ContinuationReceiptID: receipt.ReceiptID}
	store.getThread = func(string) (map[string]any, error) { return gateThreadFixture(record, "pending"), nil }
	if !registry.RegisterPendingApproval(gateID, record, pending) {
		t.Fatal("register approval")
	}
	manager.RequestApproval(gateID, pending.Call.Name)
	claim, ok := registry.ClaimApproval(gateID)
	if !ok {
		t.Fatal("claim approval")
	}
	if err := continuations.DisposePendingHost(gateID, domaincontinuation.StatusAllowed, "approval_allowed", now, pending); err != nil {
		t.Fatal(err)
	}
	claim, ok = registry.MarkClaimDisposition(claim, domaincontinuation.StatusAllowed, "approval_allowed")
	if !ok {
		t.Fatal("mark signed disposition")
	}
	drained, ok := registry.PromoteClaimToTerminal(claim)
	if !ok {
		t.Fatal("promote resolution claim")
	}
	injected := errors.New("injected resolution event failure")
	failResolution := true
	resolutionCalls, failureCalls := 0, 0
	dependencies := Dependencies{
		Store: store, Registry: registry, Manager: manager, Continuations: continuations,
		RecordResolution: func(map[string]any) error {
			resolutionCalls++
			if failResolution {
				return injected
			}
			return nil
		},
		RecordCancellations: func([]controlapp.PendingGateCancellation, string) error { return nil },
		RecordFailure: func(appmodel.PendingToolCall, error) error {
			failureCalls++
			return nil
		},
		Finalize: func(context.Context, appmodel.PendingToolCall, apploop.RuntimeAgentLoopResult, evidenceapp.TerminalReason) error {
			return nil
		},
	}
	if err := CloseDrainedForTerminal(context.Background(), []controlapp.DrainedGate[appmodel.PendingToolCall]{drained}, "resolved_continuation_failed", injected, dependencies); !errors.Is(err, injected) {
		t.Fatalf("first resolution settlement did not retain its failure: %v", err)
	}
	if _, _, _, hasPending := registry.PeekApproval(gateID); hasPending || !registry.RestoreDrained([]controlapp.DrainedGate[appmodel.PendingToolCall]{drained}) || failureCalls != 0 || store.PatchCalls() != 0 {
		t.Fatalf("failed projection did not retain a non-executable claim: pending=%v failures=%d patches=%d", hasPending, failureCalls, store.PatchCalls())
	}
	failResolution = false
	retry := registry.DrainForTurn(pending.ThreadID, pending.TurnID)
	if len(retry) != 1 {
		t.Fatalf("host retry claim = %#v", retry)
	}
	if err := CloseDrainedForTerminal(context.Background(), retry, "runtime_shutdown", context.Canceled, dependencies); err != nil {
		t.Fatalf("host retry failed: %v", err)
	}
	if !registry.CommitDrained(retry) || resolutionCalls != 2 || failureCalls != 1 || store.PatchCalls() != 1 {
		t.Fatalf("host retry settlement mismatch: resolutions=%d failures=%d patches=%d", resolutionCalls, failureCalls, store.PatchCalls())
	}
}

func TestDeniedAndCancelledPersistFailureCannotPublishSuccessTerminal(t *testing.T) {
	for _, kind := range []string{domaincontinuation.KindApproval, domaincontinuation.KindUserInput} {
		kind := kind
		t.Run(kind, func(t *testing.T) {
			now := time.Now().UTC()
			pending := cancellationPendingFixture(t, kind, now)
			store := newCancellationGateStore()
			continuations := continuationapp.NewService(newCancellationAuthority(), store)
			manager := controlapp.NewApprovalUserInputManager()
			registry := controlapp.NewGateRegistry[appmodel.PendingToolCall]()
			gateID := controlapp.SecureGateID(kind, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
			itemID := "item_" + gateID
			receipt, err := continuations.IssuePendingHost(kind, gateID, itemID, pending, now)
			if err != nil {
				t.Fatal(err)
			}
			record := controlapp.GateRecord{ThreadID: pending.ThreadID, TurnID: pending.TurnID, ItemID: itemID, ToolName: pending.Call.Name, Prompt: "Continue?", ContinuationReceiptID: receipt.ReceiptID}
			store.getThread = func(string) (map[string]any, error) { return gateThreadFixture(record, "pending"), nil }
			if kind == domaincontinuation.KindApproval {
				registry.RegisterPendingApproval(gateID, record, pending)
				manager.RequestApproval(gateID, pending.Call.Name)
			} else {
				registry.RegisterPendingUserInput(gateID, record, pending)
				manager.RequestUserInput(gateID, "Continue?")
			}
			persistFailure := errors.New("injected tool-result persistence failure")
			successFinalizeCalls, failureCalls := 0, 0
			dependencies := Dependencies{
				Store: store, Registry: registry, Manager: manager, Continuations: continuations,
				TerminalStatus:    func(string, string) (string, bool, error) { return "running", false, nil },
				PersistToolResult: func(context.Context, appmodel.PendingToolCall, any, bool) error { return persistFailure },
				Finalize: func(context.Context, appmodel.PendingToolCall, apploop.RuntimeAgentLoopResult, evidenceapp.TerminalReason) error {
					successFinalizeCalls++
					return nil
				},
				RecordFailure: func(appmodel.PendingToolCall, error) error {
					failureCalls++
					return nil
				},
				RecordResolution:    func(map[string]any) error { return nil },
				RecordCancellations: func([]controlapp.PendingGateCancellation, string) error { return nil },
			}
			var result controlapp.ActionResult
			if kind == domaincontinuation.KindApproval {
				result = Approve(context.Background(), controlapp.ApprovalDecision{ApprovalID: gateID, Decision: "deny"}, dependencies)
			} else {
				result = RespondUserInput(context.Background(), controlapp.UserInputResponse{InputID: gateID, Cancelled: true}, dependencies)
			}
			if result.StatusCode != statusInternal || successFinalizeCalls != 0 || failureCalls != 1 {
				t.Fatalf("persistence failure published a success terminal: result=%#v finalize=%d failures=%d", result, successFinalizeCalls, failureCalls)
			}
		})
	}
}

func gateThreadFixture(record controlapp.GateRecord, status string) map[string]any {
	return map[string]any{"turns": []any{map[string]any{
		"id":    record.TurnID,
		"items": []any{map[string]any{"id": record.ItemID, "status": status}},
	}}}
}

func stringFieldForGateTest(value map[string]any, key string) string {
	text, _ := value[key].(string)
	return text
}

type failingTerminalManager struct {
	status int
	body   map[string]any
}

type countingTerminalManager struct {
	mu    sync.Mutex
	calls int
}

func (*countingTerminalManager) RequestApproval(string, string)            {}
func (*countingTerminalManager) EnsureApprovalPending(string, string) bool { return true }
func (manager *countingTerminalManager) ResolveApproval(string, string) (int, map[string]any) {
	manager.mu.Lock()
	manager.calls++
	manager.mu.Unlock()
	return statusOK, map[string]any{"status": "denied"}
}
func (*countingTerminalManager) RequestUserInput(string, string)            {}
func (*countingTerminalManager) EnsureUserInputPending(string, string) bool { return true }
func (manager *countingTerminalManager) SubmitUserInput(string, []map[string]string) (int, map[string]any) {
	manager.mu.Lock()
	manager.calls++
	manager.mu.Unlock()
	return statusOK, map[string]any{"status": "submitted"}
}
func (manager *countingTerminalManager) CancelUserInput(string) (int, map[string]any) {
	manager.mu.Lock()
	manager.calls++
	manager.mu.Unlock()
	return statusOK, map[string]any{"status": "cancelled"}
}
func (manager *countingTerminalManager) Calls() int {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.calls
}

func (manager failingTerminalManager) RequestApproval(string, string)            {}
func (manager failingTerminalManager) EnsureApprovalPending(string, string) bool { return true }
func (manager failingTerminalManager) ResolveApproval(string, string) (int, map[string]any) {
	return manager.status, manager.body
}
func (manager failingTerminalManager) RequestUserInput(string, string)            {}
func (manager failingTerminalManager) EnsureUserInputPending(string, string) bool { return true }
func (manager failingTerminalManager) SubmitUserInput(string, []map[string]string) (int, map[string]any) {
	return manager.status, manager.body
}
func (manager failingTerminalManager) CancelUserInput(string) (int, map[string]any) {
	return manager.status, manager.body
}

func cancellationPendingFixture(t *testing.T, kind string, now time.Time) appmodel.PendingToolCall {
	t.Helper()
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-" + kind, TurnID: "turn-" + kind, WorkspaceRealPath: t.TempDir(),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-" + kind)), ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	call := domainmodel.ToolCall{ID: gateContinuationTestToolCallID(kind), Name: "write_file", Arguments: json.RawMessage(`{"path":"a.txt"}`)}
	approvalState := "pending"
	if kind == domaincontinuation.KindUserInput {
		call.Name = "request_user_input"
		call.Arguments = json.RawMessage(`{"questions":[{"id":"confirm","question":"Continue?"}]}`)
		approvalState = "not_required"
	}
	scope := []string{call.Name}
	scopeBody, _ := json.Marshal(scope)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-a", ServerIdentity: "host:builtin", ToolName: call.Name, ToolCallID: call.ID,
		ArgsHash: domainsecurity.CanonicalJSONHash(call.Arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema-" + kind)),
		ScopeHash: domainsecurity.SHA256Hex(scopeBody), ReadOnly: kind == domaincontinuation.KindUserInput,
		ApprovalState: approvalState, IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	providerConfig := domainmodel.TurnConfig{
		ProviderID: "provider-a", Model: "model-a", APIKey: "test-only", BaseURL: "https://provider.invalid",
		EndpointFormat: "openai-chat-completions",
	}
	return appmodel.PendingToolCall{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: securityContext.WorkspaceRealPath,
		ProviderConfig: providerConfig, ProviderID: providerConfig.ProviderID, Model: providerConfig.Model, Prompt: "continue",
		LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true, ProviderStepExact: true,
		OrdinaryResultInputIsolated: true,
		ApprovalPolicy:              "on-request", SandboxMode: "workspace-write", Call: call,
		ToolCallItemID: domaintoolcall.ToolCallItemIDV1(securityContext.TurnID, call.ID),
		ToolScope:      scope, SecurityContext: securityContext, ExecutionGrant: grant,
		ProviderNamespace: domaincontinuation.NewProviderContinuationNamespaceV1("turn", "", 1, nil),
	}
}

type cancellationAuthority struct {
	private ed25519.PrivateKey
	public  ed25519.PublicKey
	keyID   string
}

func newCancellationAuthority() *cancellationAuthority {
	private := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x42}, ed25519.SeedSize))
	public := private.Public().(ed25519.PublicKey)
	return &cancellationAuthority{private: private, public: public, keyID: domainsecurity.SHA256Hex(public)}
}

func (authority *cancellationAuthority) KeyID() string { return authority.keyID }
func (authority *cancellationAuthority) PublicKey() []byte {
	return append([]byte(nil), authority.public...)
}
func (authority *cancellationAuthority) Sign(_ context.Context, message []byte) ([]byte, error) {
	return ed25519.Sign(authority.private, message), nil
}
func (authority *cancellationAuthority) VerifyTrusted(_ context.Context, keyID string, publicKey, message, signature []byte) error {
	if keyID != authority.keyID || !bytes.Equal(publicKey, authority.public) || !ed25519.Verify(authority.public, message, signature) {
		return errors.New("untrusted test signature")
	}
	return nil
}

type cancellationGateStore struct {
	mu           sync.Mutex
	receipts     map[string]domaincontinuation.Receipt
	dispositions map[string]domaincontinuation.Disposition
	getThread    func(string) (map[string]any, error)
	patchCalls   int
	eventCalls   int
}

func newCancellationGateStore() *cancellationGateStore {
	return &cancellationGateStore{
		receipts: map[string]domaincontinuation.Receipt{}, dispositions: map[string]domaincontinuation.Disposition{},
	}
}

func (store *cancellationGateStore) GetThread(threadID string) (map[string]any, error) {
	if store.getThread != nil {
		return store.getThread(threadID)
	}
	return nil, errors.New("thread unavailable")
}
func (*cancellationGateStore) AppendItemToTurn(string, string, map[string]any) error { return nil }
func (*cancellationGateStore) EnsureGateRequestItemExact(string, string, map[string]any) error {
	return nil
}

func (store *cancellationGateStore) PatchTurnItemStatus(string, string, string, string) error {
	store.mu.Lock()
	store.patchCalls++
	store.mu.Unlock()
	return nil
}
func (store *cancellationGateStore) RecordEvent(event map[string]any) (map[string]any, []string, error) {
	store.mu.Lock()
	store.eventCalls++
	store.mu.Unlock()
	return event, nil, nil
}

func (store *cancellationGateStore) PatchCalls() int {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.patchCalls
}

func (store *cancellationGateStore) EventCalls() int {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.eventCalls
}
func (store *cancellationGateStore) PutReceiptIfAbsent(_ context.Context, receipt domaincontinuation.Receipt) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, exists := store.receipts[receipt.Payload.GateID]; exists {
		return errors.New("receipt exists")
	}
	store.receipts[receipt.Payload.GateID] = receipt
	return nil
}
func (store *cancellationGateStore) ResolveReceipt(_ context.Context, gateID string) (domaincontinuation.Receipt, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	receipt, exists := store.receipts[gateID]
	if !exists {
		return domaincontinuation.Receipt{}, continuationstoreport.ErrNotFound
	}
	return receipt, nil
}
func (store *cancellationGateStore) PutDispositionIfAbsent(_ context.Context, disposition domaincontinuation.Disposition) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, exists := store.dispositions[disposition.GateID]; exists {
		return errors.New("disposition exists")
	}
	store.dispositions[disposition.GateID] = disposition
	return nil
}
func (store *cancellationGateStore) ResolveDisposition(_ context.Context, gateID string) (domaincontinuation.Disposition, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	disposition, exists := store.dispositions[gateID]
	if !exists {
		return domaincontinuation.Disposition{}, continuationstoreport.ErrNotFound
	}
	return disposition, nil
}
func (store *cancellationGateStore) HasRecords(context.Context) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return len(store.receipts) > 0 || len(store.dispositions) > 0, nil
}
