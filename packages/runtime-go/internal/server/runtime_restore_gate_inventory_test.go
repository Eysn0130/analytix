package server

import (
	"context"
	"testing"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	controlapp "analytix.local/runtime-go/internal/app/control"
	gatecontinuationapp "analytix.local/runtime-go/internal/app/gatecontinuation"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestRestartReconcilesGateRequestInventoryCrashCutsWithoutExecution(t *testing.T) {
	for _, test := range []struct {
		name      string
		withItem  bool
		withEvent bool
	}{
		{name: "receipt_only"},
		{name: "item_only", withItem: true},
		{name: "event_only", withEvent: true},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			handler, threadID, fixture, provider := installGateInventoryCrashCut(t, test.withItem, test.withEvent)
			if err := handler.restoreRuntimeState(); err != nil {
				t.Fatalf("first restart: %v", err)
			}
			if err := handler.restoreRuntimeState(); err != nil {
				t.Fatalf("idempotent restart: %v", err)
			}
			if provider.calls.Load() != 0 {
				t.Fatalf("restart resumed provider execution: %d", provider.calls.Load())
			}
			_, disposition, err := handler.continuations.ResolveTrustedDisposition(context.Background(), fixture.GateID)
			if err != nil || disposition.Status != domaincontinuation.StatusRestartInvalid || disposition.ReasonCode != "restart_nonresumable" {
				t.Fatalf("restart disposition = %#v err=%v", disposition, err)
			}
			thread, err := handler.store.GetThread(threadID)
			if err != nil {
				t.Fatal(err)
			}
			turn, status, found := gatecontinuationapp.RestartInventoryTurnV1(thread, fixture.Pending.TurnID)
			if !found || status != "aborted" {
				t.Fatalf("stale owner turn status=%q found=%v", status, found)
			}
			itemFound, itemStatus, err := gatecontinuationapp.InspectRestartGateRequestItemV1(turn, mustGateProjection(t, handler, fixture).Item)
			if err != nil || !itemFound || itemStatus != "expired" {
				t.Fatalf("gate item found=%v status=%q err=%v", itemFound, itemStatus, err)
			}
			replay, err := handler.store.LoadEventsSince(threadID, 0)
			if err != nil {
				t.Fatal(err)
			}
			requested, resolved := 0, 0
			for _, event := range replay.Events {
				if gatecontinuationapp.PendingGateRequestBaseKeyV1(event) == gatecontinuationapp.PendingGateRequestBaseKeyV1(mustGateProjection(t, handler, fixture).Event) {
					requested++
				}
				if stringField(event, "kind") == "approval_resolved" && stringField(event, "approvalId") == fixture.GateID {
					resolved++
				}
			}
			if requested != 1 || resolved != 1 {
				t.Fatalf("gate event counts requested=%d resolved=%d events=%#v", requested, resolved, replay.Events)
			}
		})
	}
}

func TestRestartPositiveGateDispositionRepairsProjectionWithoutExecution(t *testing.T) {
	handler, threadID, fixture, provider := installGateInventoryCrashCut(t, false, false)
	if err := handler.continuations.DisposePendingHost(
		fixture.GateID, domaincontinuation.StatusAllowed, "approval_allowed", time.Now().UTC(), fixture.Pending,
	); err != nil {
		t.Fatal(err)
	}
	if err := handler.restoreRuntimeState(); err != nil {
		t.Fatal(err)
	}
	if provider.calls.Load() != 0 {
		t.Fatalf("positive restart disposition resumed provider execution: %d", provider.calls.Load())
	}
	thread, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, status, found := gatecontinuationapp.RestartInventoryTurnV1(thread, fixture.Pending.TurnID)
	if !found || status != "aborted" {
		t.Fatalf("owner turn status=%q found=%v", status, found)
	}
	itemFound, itemStatus, err := gatecontinuationapp.InspectRestartGateRequestItemV1(turn, mustGateProjection(t, handler, fixture).Item)
	if err != nil || !itemFound || itemStatus != "allowed" {
		t.Fatalf("allowed projection found=%v status=%q err=%v", itemFound, itemStatus, err)
	}
	replay, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	resolved := 0
	for _, event := range replay.Events {
		if stringField(event, "kind") == "approval_resolved" && stringField(event, "approvalId") == fixture.GateID && stringField(event, "status") == "allowed" {
			resolved++
		}
	}
	if resolved != 1 {
		t.Fatalf("allowed resolution count=%d events=%#v", resolved, replay.Events)
	}
}

func installGateInventoryCrashCut(
	t *testing.T,
	withItem, withEvent bool,
) (*runtimeServerHandler, string, pausedTransitionApprovalFixture, *countingImmediateWorkspaceProvider) {
	t.Helper()
	workspace := workspacetest.New(t)
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "provider-a", BaseURL: "https://provider.invalid", APIKey: "test-only",
		Model: "model-a", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	provider := &countingImmediateWorkspaceProvider{}
	handler.provider = provider
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "gate inventory crash cut", "workspace": workspace,
		"providerId": "provider-a", "model": "model-a",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	now := time.Now().UTC()
	securityContext, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: context.Background(), Authority: handler.turnSecurity, Reader: filestore.CaseBindingReader{}, Thread: thread,
		ThreadID: threadID, TurnID: "turn-inventory-cut", Workspace: workspace,
		Principal: testIdentityPrincipal(), IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	pending := pausedApprovalForTransition(t, securityContext, now)
	gateID := controlapp.SecureGateID(
		domaincontinuation.KindApproval, pending.ThreadID, pending.TurnID,
		pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID,
	)
	receipt, err := handler.continuations.IssuePendingHost(domaincontinuation.KindApproval, gateID, "item_"+gateID, pending, now)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := gatecontinuationapp.ProjectVerifiedGateRequestV1(receipt)
	if err != nil {
		t.Fatal(err)
	}
	items := []any{}
	if withItem {
		items = append(items, projection.Item)
	}
	epochState, err := contextepochapp.BootstrapState(
		threadID, securityContext.ContextEpoch,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(securityContext)}, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := handler.store.AppendTurnToThread(threadID, map[string]any{
		"id": pending.TurnID, "threadId": threadID, "status": "running",
		"createdAt": now.Format(time.RFC3339Nano), "securityContext": securityRecord, "items": items,
	}, "provider-a", map[string]any{
		"securityState": securityRecord, "contextEpochState": contextepochapp.PublicState(epochState),
	}); err != nil {
		t.Fatal(err)
	}
	if withEvent {
		if _, _, err := handler.store.RecordEvent(projection.Event); err != nil {
			t.Fatal(err)
		}
	}
	handler.gates = controlapp.NewGateRegistry[runtimePendingToolCall]()
	handler.gate = controlapp.NewApprovalUserInputManager()
	return handler, threadID, pausedTransitionApprovalFixture{
		GateID: gateID, ItemID: projection.Record.ItemID, Pending: pending, SecurityContext: securityContext,
	}, provider
}

func mustGateProjection(
	t *testing.T,
	handler *runtimeServerHandler,
	fixture pausedTransitionApprovalFixture,
) gatecontinuationapp.GateRequestProjectionV1 {
	t.Helper()
	receipt, err := handler.continuations.VerifyOpen(context.Background(), fixture.GateID, time.Now().UTC())
	if err != nil {
		// After restart the receipt is consumed; resolve the immutable pair.
		receipt, _, err = handler.continuations.ResolveTrustedDisposition(context.Background(), fixture.GateID)
	}
	if err != nil {
		t.Fatal(err)
	}
	projection, err := gatecontinuationapp.ProjectVerifiedGateRequestV1(receipt)
	if err != nil {
		t.Fatal(err)
	}
	return projection
}
