package pendingwork

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestApprovedToolDispatchLeasePersistsBeforeExternalCallAndIsOneShot(t *testing.T) {
	fixture, request := approvedDispatchFixture(t)
	lease, err := fixture.service.BeginApprovedToolDispatch(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := fixture.store.ReadReceipt(context.Background(), lease.WorkID())
	if err != nil || receipt.Kind != domainpendingwork.KindApprovedToolDispatch || len(receipt.GrantMembers) != 1 ||
		receipt.GrantMembers[0].GrantID != request.Pending.ExecutionGrant.GrantID {
		t.Fatalf("dispatch receipt was not durable before send: receipt=%#v err=%v", receipt, err)
	}
	if _, err := fixture.service.BeginApprovedToolDispatch(context.Background(), request); !errors.Is(err, ErrWorkAlreadyOpen) {
		t.Fatalf("open write dispatch was reissued: %v", err)
	}
	changed := request
	changed.Pending.Call.Arguments = json.RawMessage(`{"member":"changed"}`)
	if err := fixture.service.VerifyApprovedToolDispatchAtSend(
		context.Background(), lease, changed, request.IssuedAt.Add(time.Second),
	); !errors.Is(err, ErrOperationMismatch) {
		t.Fatalf("changed dispatch reused the lease: %v", err)
	}
	if err := fixture.service.VerifyApprovedToolDispatchAtSend(
		context.Background(), lease, request, request.IssuedAt.Add(time.Second),
	); err != nil {
		t.Fatalf("exact dispatch failed at-send verification: %v", err)
	}
	if err := fixture.service.VerifyApprovedToolDispatchAtSend(
		context.Background(), lease, request, request.IssuedAt.Add(time.Second),
	); !errors.Is(err, ErrDispatchAlreadyClaimed) {
		t.Fatalf("copied lease authorized a second send: %v", err)
	}
}

func TestApprovedToolDispatchCompletesOnlyAfterDurableResult(t *testing.T) {
	fixture, request := approvedDispatchFixture(t)
	lease, err := fixture.service.BeginApprovedToolDispatch(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.VerifyApprovedToolDispatchAtSend(
		context.Background(), lease, request, request.IssuedAt.Add(time.Second),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.CloseApprovedToolDispatchAfterSettlement(
		context.Background(), lease, request, request.IssuedAt.Add(2*time.Second),
	); err == nil {
		t.Fatal("dispatch completed before an exact durable tool result")
	}
	fixture.addResult(request.Pending.ExecutionGrant, true, "durable error outcome", request.IssuedAt.Add(2*time.Second))
	disposition, err := fixture.service.CloseApprovedToolDispatchAfterSettlement(
		context.Background(), lease, request, request.IssuedAt.Add(3*time.Second),
	)
	if err != nil || disposition.Status != domainpendingwork.StatusCompleted || disposition.ReasonCode != "tool_outcome_durable" {
		t.Fatalf("durably classified dispatch did not complete: disposition=%#v err=%v", disposition, err)
	}
	repeated, err := fixture.service.CloseApprovedToolDispatchAfterSettlement(
		context.Background(), lease, request, request.IssuedAt.Add(4*time.Second),
	)
	if err != nil || repeated.DispositionID != disposition.DispositionID {
		t.Fatalf("completed dispatch close was not idempotent: repeated=%#v err=%v", repeated, err)
	}
}

func TestRestartOpenApprovedDispatchProducesOutcomeUnknownAndCannotResend(t *testing.T) {
	fixture, request := approvedDispatchFixture(t)
	lease, err := fixture.service.BeginApprovedToolDispatch(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.VerifyApprovedToolDispatchAtSend(
		context.Background(), lease, request, request.IssuedAt.Add(time.Second),
	); err != nil {
		t.Fatal(err)
	}
	restarted := NewService(fixture.authority, fixture.store, fixture.threads)
	dispositions, err := restarted.CloseAllOpenOnRestart(context.Background(), request.IssuedAt.Add(2*time.Second))
	if err != nil || len(dispositions) != 1 || dispositions[0].WorkID != lease.WorkID() ||
		dispositions[0].Status != domainpendingwork.StatusOutcomeUnknown || dispositions[0].ReasonCode != "tool_outcome_unknown_after_restart" {
		t.Fatalf("open dispatch was not conservatively classified: dispositions=%#v err=%v", dispositions, err)
	}
	if _, err := restarted.BeginApprovedToolDispatch(context.Background(), request); !errors.Is(err, ErrWorkClosed) {
		t.Fatalf("outcome-unknown dispatch became resendable: %v", err)
	}
	if err := restarted.VerifyApprovedToolDispatchAtSend(
		context.Background(), lease, request, request.IssuedAt.Add(3*time.Second),
	); !errors.Is(err, ErrWorkClosed) {
		t.Fatalf("restarted dispatch retained execution authority: %v", err)
	}
	if repeated, err := restarted.CloseAllOpenOnRestart(context.Background(), request.IssuedAt.Add(3*time.Second)); err != nil || len(repeated) != 0 {
		t.Fatalf("repeated restart changed a prior disposition: dispositions=%#v err=%v", repeated, err)
	}
	inventory, err := restarted.TrustedInventoryV1(context.Background())
	if err != nil || len(inventory.Receipts) != 1 || len(inventory.Dispositions) != 1 {
		t.Fatalf("trusted inventory lost prior outcome-unknown authority: inventory=%#v err=%v", inventory, err)
	}
	unknown, err := RestartOutcomeUnknownGrantsV1(inventory)
	if err != nil || len(unknown) != 1 || unknown[0].GrantID != request.Pending.ExecutionGrant.GrantID ||
		unknown[0].Disposition.DispositionID != dispositions[0].DispositionID {
		t.Fatalf("prior outcome-unknown authority did not survive restart: unknown=%#v err=%v", unknown, err)
	}
	inventory.Receipts[0].GrantMembers[0].GrantID = domainsecurity.SHA256Hex([]byte("caller mutation"))
	secondInventory, err := restarted.TrustedInventoryV1(context.Background())
	if err != nil || secondInventory.Receipts[0].GrantMembers[0].GrantID != request.Pending.ExecutionGrant.GrantID {
		t.Fatalf("trusted inventory exposed store-owned grant members: inventory=%#v err=%v", secondInventory, err)
	}
}

func TestApprovedToolDispatchWorkIdentitySurvivesUnrelatedRegistryGrowth(t *testing.T) {
	fixture, request := approvedDispatchFixture(t)
	unrelated := fixture.addGrant(t, "unrelated_read_before_dispatch", true, "not_required", request.IssuedAt.Add(-time.Second))
	lease, err := fixture.service.BeginApprovedToolDispatch(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	fixture.addResult(unrelated, false, "unrelated durable result", request.IssuedAt.Add(time.Second))
	if err := fixture.service.VerifyApprovedToolDispatchAtSend(
		context.Background(), lease, request, request.IssuedAt.Add(2*time.Second),
	); err != nil {
		t.Fatalf("unrelated settlement invalidated the exact approved dispatch: %v", err)
	}
	if _, err := fixture.service.BeginApprovedToolDispatch(context.Background(), request); !errors.Is(err, ErrWorkAlreadyOpen) {
		t.Fatalf("registry growth minted another write dispatch lease: %v", err)
	}
	receipts, err := fixture.store.ListReceipts(context.Background())
	if err != nil || len(receipts) != 1 || receipts[0].WorkID != lease.WorkID() {
		t.Fatalf("registry growth changed stable dispatch identity: receipts=%#v err=%v", receipts, err)
	}
}

func TestRestartPreservesKnownDurableApprovedDispatchOutcome(t *testing.T) {
	fixture, request := approvedDispatchFixture(t)
	lease, err := fixture.service.BeginApprovedToolDispatch(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.VerifyApprovedToolDispatchAtSend(
		context.Background(), lease, request, request.IssuedAt.Add(time.Second),
	); err != nil {
		t.Fatal(err)
	}
	fixture.addResult(request.Pending.ExecutionGrant, false, "known durable outcome", request.IssuedAt.Add(2*time.Second))
	restarted := NewService(fixture.authority, fixture.store, fixture.threads)
	dispositions, err := restarted.CloseAllOpenOnRestart(context.Background(), request.IssuedAt.Add(3*time.Second))
	if err != nil || len(dispositions) != 1 || dispositions[0].Status != domainpendingwork.StatusCompleted ||
		dispositions[0].ReasonCode != "tool_outcome_durable" {
		t.Fatalf("known durable outcome was downgraded at restart: dispositions=%#v err=%v", dispositions, err)
	}
	inventory, err := restarted.TrustedInventoryV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := RestartOutcomeUnknownGrantsV1(inventory)
	if err != nil || len(unknown) != 0 {
		t.Fatalf("known durable outcome became outcome-unknown: unknown=%#v err=%v", unknown, err)
	}
}

func TestTrustedInventoryRejectsCompletedDispatchAfterDurableResultRemoval(t *testing.T) {
	fixture, request := approvedDispatchFixture(t)
	lease, err := fixture.service.BeginApprovedToolDispatch(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.VerifyApprovedToolDispatchAtSend(
		context.Background(), lease, request, request.IssuedAt.Add(time.Second),
	); err != nil {
		t.Fatal(err)
	}
	fixture.addResult(request.Pending.ExecutionGrant, false, "known durable outcome", request.IssuedAt.Add(2*time.Second))
	if _, err := fixture.service.CloseApprovedToolDispatchAfterSettlement(
		context.Background(), lease, request, request.IssuedAt.Add(3*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.TrustedInventoryV1(context.Background()); err != nil {
		t.Fatalf("intact completed dispatch inventory was rejected: %v", err)
	}
	thread := fixture.threads.threads[fixture.securityContext.ThreadID]
	turn := thread["turns"].([]any)[0].(map[string]any)
	items := turn["items"].([]any)
	retained := make([]any, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if mapString(item, "kind") == "tool_result" && mapString(item, "executionGrantId") == request.Pending.ExecutionGrant.GrantID {
			continue
		}
		retained = append(retained, raw)
	}
	turn["items"] = retained
	if _, err := fixture.service.TrustedInventoryV1(context.Background()); err == nil {
		t.Fatal("completed dispatch remained trusted after its durable result was removed")
	}
}

func approvedDispatchFixture(t *testing.T) (*serviceFixture, ApprovedToolDispatchRequest) {
	return approvedDispatchFixtureForTool(t, "write_file")
}

func approvedDispatchFixtureForTool(t *testing.T, tool string) (*serviceFixture, ApprovedToolDispatchRequest) {
	t.Helper()
	fixture := newServiceFixture(t)
	pendingGrant := fixture.addGrant(t, tool, false, "pending", fixture.now)
	transitionedAt := fixture.now.Add(time.Second)
	approvedGrant, err := executiongrantapp.Approve(pendingGrant, transitionedAt)
	if err != nil {
		t.Fatal(err)
	}
	transition, err := domainsecurity.NewApprovalGrantTransitionV1(domainsecurity.ApprovalGrantTransitionInputV1{
		Context: fixture.securityContext, ApprovalID: "appr_dispatch", ApprovalItemID: "item_appr_dispatch",
		ContinuationReceiptID:     domainsecurity.SHA256Hex([]byte("dispatch-receipt")),
		ContinuationDispositionID: domainsecurity.SHA256Hex([]byte("dispatch-disposition")),
		PendingGrant:              pendingGrant, ApprovedGrant: approvedGrant, TransitionedAt: transitionedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err := executiongrantapp.ApprovalTransitionRecords(
		fixture.securityContext, transition, pendingGrant, approvedGrant,
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.appendItem(item)
	fixture.grantArguments[approvedGrant.GrantID] = fixture.grantArguments[pendingGrant.GrantID]
	pending := fixture.pendingCall(approvedGrant)
	pending.ApprovalTransition = &transition
	return fixture, ApprovedToolDispatchRequest{Pending: pending, IssuedAt: fixture.now.Add(2 * time.Second)}
}
