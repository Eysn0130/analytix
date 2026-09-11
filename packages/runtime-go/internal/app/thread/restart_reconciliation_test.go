package thread

import (
	"context"
	"errors"
	"testing"
	"time"

	controlapp "analytix.local/runtime-go/internal/app/control"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	continuationstoreport "analytix.local/runtime-go/internal/ports/continuationstore"
)

type restartGateAuthorityStubV1 struct {
	disposition    domaincontinuation.Disposition
	dispositionErr error
	reason         string
	disposedStatus string
	disposedReason string
	disposedAt     time.Time
	authorizeErr   error
}

func (*restartGateAuthorityStubV1) ValidateRestartRecordHost(
	gateID, receiptID, kind, threadID, turnID, itemID string,
) error {
	return nil
}

func (stub *restartGateAuthorityStubV1) ResolveTrustedDispositionForAudit(
	context.Context,
	string,
) (domaincontinuation.Receipt, domaincontinuation.Disposition, error) {
	return domaincontinuation.Receipt{}, stub.disposition, stub.dispositionErr
}

func (stub *restartGateAuthorityStubV1) RestartDispositionReason(
	gateID, receiptID, kind, threadID, turnID, itemID string,
	thread map[string]any,
	resolver appmodel.StrictPendingToolProviderResolver,
	authorize func(appmodel.PendingToolCall) error,
) string {
	stub.authorizeErr = authorize(appmodel.PendingToolCall{})
	return stub.reason
}

func (stub *restartGateAuthorityStubV1) DisposeHost(
	gateID, status, reasonCode string,
	at time.Time,
) error {
	stub.disposedStatus, stub.disposedReason, stub.disposedAt = status, reasonCode, at
	return nil
}

type restartProviderResolverStubV1 struct{}

func (restartProviderResolverStubV1) TurnConfig(providerID, model string) domainmodel.TurnConfig {
	return domainmodel.TurnConfig{}
}

func (restartProviderResolverStubV1) HasProvider(string) bool {
	return true
}

func (restartProviderResolverStubV1) ValidateExecutionModel(string, string) error {
	return nil
}

func TestRestartGateReconciliationProjectsTrustedDispositionV1(t *testing.T) {
	authority := &restartGateAuthorityStubV1{disposition: domaincontinuation.Disposition{
		Status: domaincontinuation.StatusAllowed, ReasonCode: "approval_allowed",
	}}
	record := controlapp.GateRecord{
		ThreadID: "thread-1", TurnID: "turn-1", ItemID: "item-1",
		ToolName: "read", ContinuationReceiptID: "receipt-1",
	}
	var patchedStatus string
	var resolution map[string]any
	err := RevalidateAndCloseRestartedGateV1(
		domaincontinuation.KindApproval,
		"approval-1",
		map[string]any{"turns": []any{map[string]any{
			"id": "turn-1", "items": []any{map[string]any{"id": "item-1", "status": "pending"}},
		}}},
		controlapp.PendingGateState[appmodel.PendingToolCall]{Record: record},
		RestartGateReconciliationDependenciesV1{
			Continuations: authority, ProviderResolver: restartProviderResolverStubV1{},
			AuthorizePending: func(context.Context, appmodel.PendingToolCall, string, time.Time) error { return nil },
			PatchTurnItemStatus: func(threadID, turnID, itemID, status string) error {
				patchedStatus = status
				return nil
			},
			RecordResolution: func(event map[string]any) error {
				resolution = event
				return nil
			},
			RecordCancellations: func([]controlapp.PendingGateCancellation, string) error {
				return errors.New("unexpected cancellation")
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if patchedStatus != "allowed" || resolution["kind"] != "approval_resolved" ||
		resolution["status"] != "allowed" || authority.disposedStatus != "" {
		t.Fatalf("trusted disposition projection mismatch: patch=%q event=%#v authority=%#v", patchedStatus, resolution, authority)
	}
}

func TestRestartGateReconciliationClosesMissingDispositionV1(t *testing.T) {
	now := time.Date(2026, 7, 26, 15, 0, 0, 0, time.UTC)
	authority := &restartGateAuthorityStubV1{
		dispositionErr: continuationstoreport.ErrNotFound,
		reason:         "restart_authority_rejected",
	}
	record := controlapp.GateRecord{
		ThreadID: "thread-2", TurnID: "turn-2", ItemID: "item-2",
		ToolName: "read", ContinuationReceiptID: "receipt-2",
	}
	var approvalState, patchedStatus, cancellationReason string
	err := RevalidateAndCloseRestartedGateV1(
		domaincontinuation.KindApproval,
		"approval-2",
		map[string]any{"turns": []any{}},
		controlapp.PendingGateState[appmodel.PendingToolCall]{Record: record},
		RestartGateReconciliationDependenciesV1{
			Continuations: authority, ProviderResolver: restartProviderResolverStubV1{},
			AuthorizePending: func(_ context.Context, _ appmodel.PendingToolCall, state string, at time.Time) error {
				approvalState = state
				if !at.Equal(now) {
					t.Fatalf("authorization time=%s want %s", at, now)
				}
				return nil
			},
			PatchTurnItemStatus: func(threadID, turnID, itemID, status string) error {
				patchedStatus = status
				return nil
			},
			RecordResolution: func(map[string]any) error { return errors.New("unexpected resolution") },
			RecordCancellations: func(cancellations []controlapp.PendingGateCancellation, reason string) error {
				if len(cancellations) != 1 || cancellations[0].ID != "approval-2" {
					t.Fatalf("cancellations=%#v", cancellations)
				}
				cancellationReason = reason
				return nil
			},
			CurrentTime: func() time.Time { return now },
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if authority.authorizeErr != nil || approvalState != "pending" ||
		authority.disposedStatus != domaincontinuation.StatusRestartInvalid ||
		authority.disposedReason != authority.reason || !authority.disposedAt.Equal(now) ||
		patchedStatus != "expired" || cancellationReason != authority.reason {
		t.Fatalf(
			"restart invalid closure mismatch: state=%q patch=%q cancellation=%q authority=%#v",
			approvalState, patchedStatus, cancellationReason, authority,
		)
	}
}

func TestAbortStaleRuntimeTurnsAfterRestartDispatchesByExactStateV1(t *testing.T) {
	terminalKey := controlapp.TurnKey("thread-1", "turn-terminal")
	activeKey := controlapp.TurnKey("thread-1", "turn-active")
	outcomes := map[string]executiongrantapp.RestartGrantOutcomesV1{
		terminalKey: {"grant-terminal": {}},
		activeKey:   {"grant-active": {}},
	}
	var reconciled, aborted string
	err := AbortStaleRuntimeTurnsAfterRestartV1(
		[]string{"thread-1"},
		map[string]bool{controlapp.TurnKey("thread-1", "turn-gated"): true},
		outcomes,
		RestartTurnReconciliationDependenciesV1{
			GetThread: func(string) (map[string]any, error) {
				return map[string]any{"turns": []any{
					map[string]any{"id": "turn-owned", "status": "running"},
					map[string]any{"id": "turn-gated", "status": "waiting"},
					map[string]any{"id": "turn-terminal", "status": "completed"},
					map[string]any{"id": "turn-active", "status": "running"},
				}}, nil
			},
			OwnsRestartTurn: func(_, turnID string) bool { return turnID == "turn-owned" },
			ReconcileTerminal: func(_, turnID string, got executiongrantapp.RestartGrantOutcomesV1) error {
				reconciled = turnID
				return nil
			},
			AbortActive: func(_, turnID string, got executiongrantapp.RestartGrantOutcomesV1) error {
				aborted = turnID
				return nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if reconciled != "turn-terminal" || aborted != "turn-active" || len(outcomes) != 0 {
		t.Fatalf("restart dispatch mismatch: reconciled=%q aborted=%q outcomes=%#v", reconciled, aborted, outcomes)
	}
}

func TestAbortStaleRuntimeTurnsAfterRestartRejectsGateDispatchConflictV1(t *testing.T) {
	key := controlapp.TurnKey("thread-1", "turn-gated")
	err := AbortStaleRuntimeTurnsAfterRestartV1(
		[]string{"thread-1"},
		map[string]bool{key: true},
		map[string]executiongrantapp.RestartGrantOutcomesV1{
			key: {"grant-1": {}},
		},
		RestartTurnReconciliationDependenciesV1{
			GetThread: func(string) (map[string]any, error) {
				return map[string]any{"turns": []any{map[string]any{"id": "turn-gated", "status": "waiting"}}}, nil
			},
			OwnsRestartTurn:   func(string, string) bool { return false },
			ReconcileTerminal: func(string, string, executiongrantapp.RestartGrantOutcomesV1) error { return nil },
			AbortActive:       func(string, string, executiongrantapp.RestartGrantOutcomesV1) error { return nil },
		},
	)
	if err == nil || err.Error() != "outcome-unknown write dispatch conflicts with a resumable gate" {
		t.Fatalf("gate dispatch conflict did not fail closed: %v", err)
	}
}
