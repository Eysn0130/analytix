package pendingwork

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appmodel "analytix.local/runtime-go/internal/app/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsideeffectidentity "analytix.local/runtime-go/internal/domain/sideeffectidentity"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
)

func TestSideEffectIntentRejectsOpenAndClosedDuplicatesAcrossChangedGrants(t *testing.T) {
	fixture := newServiceFixture(t)
	firstGrant := fixture.addGrant(t, "write_file", false, "not_required", fixture.now)
	firstRequest := sideEffectIntentRequestForTest(fixture.pendingCall(firstGrant), fixture.now.Add(time.Second))
	lease, err := fixture.service.BeginSideEffectIntent(context.Background(), firstRequest)
	if err != nil {
		t.Fatal(err)
	}

	secondGrant := addEquivalentSideEffectGrant(t, fixture, firstGrant, firstGrant.ToolName, "second", fixture.grantArguments[firstGrant.GrantID], fixture.now.Add(2*time.Second))
	secondRequest := sideEffectIntentRequestForTest(fixture.pendingCall(secondGrant), fixture.now.Add(3*time.Second))
	_, err = fixture.service.BeginSideEffectIntent(context.Background(), secondRequest)
	assertSideEffectDuplicate(t, err, "open", firstGrant.GrantID)

	if err := fixture.service.VerifySideEffectIntentAtSend(
		context.Background(), lease, firstRequest, fixture.now.Add(4*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	fixture.addResult(firstGrant, false, "first durable outcome", fixture.now.Add(5*time.Second))
	if _, err := fixture.service.CloseSideEffectIntentAfterSettlement(
		context.Background(), lease, firstRequest, fixture.now.Add(6*time.Second),
	); err != nil {
		t.Fatal(err)
	}

	thirdGrant := addEquivalentSideEffectGrant(t, fixture, firstGrant, firstGrant.ToolName, "third", fixture.grantArguments[firstGrant.GrantID], fixture.now.Add(7*time.Second))
	thirdRequest := sideEffectIntentRequestForTest(fixture.pendingCall(thirdGrant), fixture.now.Add(8*time.Second))
	_, err = fixture.service.BeginSideEffectIntent(context.Background(), thirdRequest)
	assertSideEffectDuplicate(t, err, "closed", firstGrant.GrantID)

	receipts, err := fixture.store.ListReceipts(context.Background())
	if err != nil || len(receipts) != 1 || receipts[0].WorkID != lease.WorkID() {
		t.Fatalf("semantic duplicate created another intent: receipts=%#v err=%v", receipts, err)
	}
}

func TestSideEffectIntentConcurrentClaimantsHaveOneWinner(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, "write_file", false, "not_required", fixture.now)
	request := sideEffectIntentRequestForTest(fixture.pendingCall(grant), fixture.now.Add(time.Second))

	var winners atomic.Int32
	var duplicates atomic.Int32
	var unexpected atomic.Int32
	var group sync.WaitGroup
	for range 12 {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := fixture.service.BeginSideEffectIntent(context.Background(), request); err == nil {
				winners.Add(1)
			} else if errors.Is(err, ErrWorkAlreadyOpen) {
				duplicates.Add(1)
			} else {
				unexpected.Add(1)
			}
		}()
	}
	group.Wait()
	if winners.Load() != 1 || duplicates.Load() != 11 || unexpected.Load() != 0 {
		t.Fatalf("exclusive side-effect CAS winners=%d duplicates=%d unexpected=%d", winners.Load(), duplicates.Load(), unexpected.Load())
	}
}

func TestSideEffectIntentChangedArgumentsCanClaimSeparately(t *testing.T) {
	fixture := newServiceFixture(t)
	firstGrant := fixture.addGrant(t, "write_file", false, "not_required", fixture.now)
	firstRequest := sideEffectIntentRequestForTest(fixture.pendingCall(firstGrant), fixture.now.Add(time.Second))
	first, err := fixture.service.BeginSideEffectIntent(context.Background(), firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	changedArgs := map[string]any{"member": "different destination"}
	secondGrant := addEquivalentSideEffectGrant(t, fixture, firstGrant, firstGrant.ToolName, "different-args", changedArgs, fixture.now.Add(2*time.Second))
	secondRequest := sideEffectIntentRequestForTest(fixture.pendingCall(secondGrant), fixture.now.Add(3*time.Second))
	second, err := fixture.service.BeginSideEffectIntent(context.Background(), secondRequest)
	if err != nil || second.WorkID() == first.WorkID() {
		t.Fatalf("different semantic arguments did not receive independent authority: first=%q second=%q err=%v", first.WorkID(), second.WorkID(), err)
	}
}

func TestWitnessedBoundarySideEffectIntentAllowsOrdinaryWriteOnly(t *testing.T) {
	issuedAt := time.Date(2026, 7, 27, 13, 15, 0, 0, time.UTC)
	securityContext := mustPendingWorkWitnessedBoundaryContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-side-effect-boundary", TurnID: "turn-side-effect-boundary",
		WorkspaceRealPath: "/workspace/side-effect-boundary", ContextEpoch: 1, IssuedAt: issuedAt,
	})
	ordinary := pendingWorkCallForContext(t, securityContext, "write_file", false, "not_required", issuedAt)
	if _, err := sideEffectIntentSemanticAuthority(sideEffectIntentRequestForTest(ordinary, issuedAt.Add(time.Second))); err != nil {
		t.Fatalf("ordinary write intent was blocked by unavailable case authority: %v", err)
	}

	protected := pendingWorkCallForContext(t, securityContext, "mcp__analytix_funds__run_full_case_analysis", false, "not_required", issuedAt)
	if _, err := sideEffectIntentSemanticAuthority(sideEffectIntentRequestForTest(protected, issuedAt.Add(time.Second))); !errors.Is(err, ErrGrantAuthority) {
		t.Fatalf("protected case artifact intent escaped boundary-only authority: %v", err)
	}
}

func TestSideEffectIntentRejectsToolAndArgumentAliasBypass(t *testing.T) {
	fixture := newServiceFixture(t)
	seed := fixture.addGrant(t, "read", true, "not_required", fixture.now)
	firstArgs := map[string]any{"path": "a.txt", "oldText": "before", "newText": "after"}
	firstGrant := addEquivalentSideEffectGrant(t, fixture, seed, "edit", "alias-first", firstArgs, fixture.now.Add(time.Second))
	firstRequest := sideEffectIntentRequestForTest(fixture.pendingCall(firstGrant), fixture.now.Add(2*time.Second))
	if _, err := fixture.service.BeginSideEffectIntent(context.Background(), firstRequest); err != nil {
		t.Fatal(err)
	}
	secondArgs := map[string]any{"path": "a.txt", "old_string": "before", "new_string": "after", "replace_all": false}
	secondGrant := addEquivalentSideEffectGrant(t, fixture, seed, "edit_file", "alias-second", secondArgs, fixture.now.Add(3*time.Second))
	secondRequest := sideEffectIntentRequestForTest(fixture.pendingCall(secondGrant), fixture.now.Add(4*time.Second))
	secondRequest.SemanticIdentity = firstRequest.SemanticIdentity
	_, err := fixture.service.BeginSideEffectIntent(context.Background(), secondRequest)
	assertSideEffectDuplicate(t, err, "open", firstGrant.GrantID)
}

func TestSideEffectIntentApprovedGrantRevalidatesDurableTransitionAtSend(t *testing.T) {
	fixture, approved := approvedDispatchFixture(t)
	request := sideEffectIntentRequestForTest(approved.Pending, approved.IssuedAt)
	lease, err := fixture.service.BeginSideEffectIntent(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	tampered := request
	transition := *tampered.Pending.ApprovalTransition
	transition.TransitionID = domainsecurity.SHA256Hex([]byte("tampered transition"))
	tampered.Pending.ApprovalTransition = &transition
	if err := fixture.service.VerifySideEffectIntentAtSend(
		context.Background(), lease, tampered, request.IssuedAt.Add(time.Second),
	); err == nil {
		t.Fatal("tampered approval transition retained side-effect execution authority")
	}
	if err := fixture.service.VerifySideEffectIntentAtSend(
		context.Background(), lease, request, request.IssuedAt.Add(time.Second),
	); err != nil {
		t.Fatalf("exact durable approval transition was rejected: %v", err)
	}
}

func TestRestartOpenNotRequiredSideEffectIntentIsOutcomeUnknownAndCannotResend(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, "write_file", false, "not_required", fixture.now)
	request := sideEffectIntentRequestForTest(fixture.pendingCall(grant), fixture.now.Add(time.Second))
	lease, err := fixture.service.BeginSideEffectIntent(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.VerifySideEffectIntentAtSend(
		context.Background(), lease, request, fixture.now.Add(2*time.Second),
	); err != nil {
		t.Fatal(err)
	}

	restarted := NewService(fixture.authority, fixture.store, fixture.threads)
	dispositions, err := restarted.CloseAllOpenOnRestart(context.Background(), fixture.now.Add(3*time.Second))
	if err != nil || len(dispositions) != 1 || dispositions[0].Status != domainpendingwork.StatusOutcomeUnknown ||
		dispositions[0].ReasonCode != "tool_outcome_unknown_after_restart" {
		t.Fatalf("not-required write was not conservatively classified: dispositions=%#v err=%v", dispositions, err)
	}
	if _, err := restarted.BeginSideEffectIntent(context.Background(), request); !errors.Is(err, ErrWorkClosed) {
		t.Fatalf("outcome-unknown side effect became resendable: %v", err)
	}
}

func TestRestartPreservesKnownDurableSideEffectIntentOutcome(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, "write_file", false, "not_required", fixture.now)
	request := sideEffectIntentRequestForTest(fixture.pendingCall(grant), fixture.now.Add(time.Second))
	lease, err := fixture.service.BeginSideEffectIntent(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.VerifySideEffectIntentAtSend(
		context.Background(), lease, request, fixture.now.Add(2*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	fixture.addResult(grant, false, "known durable outcome", fixture.now.Add(3*time.Second))
	restarted := NewService(fixture.authority, fixture.store, fixture.threads)
	dispositions, err := restarted.CloseAllOpenOnRestart(context.Background(), fixture.now.Add(4*time.Second))
	if err != nil || len(dispositions) != 1 || dispositions[0].Status != domainpendingwork.StatusCompleted ||
		dispositions[0].ReasonCode != "tool_outcome_durable" {
		t.Fatalf("known side-effect outcome was downgraded at restart: dispositions=%#v err=%v", dispositions, err)
	}
	inventory, err := restarted.TrustedInventoryV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := RestartOutcomeUnknownGrantsV1(inventory)
	if err != nil || len(unknown) != 0 {
		t.Fatalf("known side-effect outcome became outcome-unknown: unknown=%#v err=%v", unknown, err)
	}
}

func assertSideEffectDuplicate(t *testing.T, err error, status, firstGrantID string) {
	t.Helper()
	var duplicate SideEffectIntentDuplicateError
	if !errors.As(err, &duplicate) || duplicate.IntentStatus != status || duplicate.FirstGrantID != firstGrantID {
		t.Fatalf("duplicate classification=%#v err=%v want status=%q firstGrant=%q", duplicate, err, status, firstGrantID)
	}
}

func sideEffectIntentRequestForTest(pending appmodel.PendingToolCall, issuedAt time.Time) SideEffectIntentRequest {
	toolName := pending.Call.Name
	switch toolName {
	case "write":
		toolName = "write_file"
	case "edit":
		toolName = "edit_file"
	case "delegate_task":
		toolName = "task"
	case "todo_patch":
		toolName = "todo_ops"
	}
	semanticHash := domainsecurity.SHA256Hex(append([]byte(toolName+"\x00"), pending.Call.Arguments...))
	return SideEffectIntentRequest{
		Pending: pending, IssuedAt: issuedAt,
		SemanticIdentity: domainsideeffectidentity.IdentityV1{
			SchemaVersion: domainsideeffectidentity.SchemaVersionV1, ToolName: toolName, ArgsHash: semanticHash,
		},
	}
}

func addEquivalentSideEffectGrant(
	t *testing.T,
	fixture *serviceFixture,
	base domainsecurity.ExecutionGrant,
	toolName string,
	callLabel string,
	arguments map[string]any,
	issuedAt time.Time,
) domainsecurity.ExecutionGrant {
	t.Helper()
	argumentBytes, err := json.Marshal(arguments)
	if err != nil {
		t.Fatal(err)
	}
	callID := pendingWorkTestToolCallID(toolName + "-" + callLabel)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: fixture.securityContext, Provider: base.Provider, ServerIdentity: base.ServerIdentity,
		ConnectionEpoch: base.ConnectionEpoch, ToolName: toolName, ToolCallID: callID,
		ArgsHash: domainsecurity.CanonicalJSONHash(argumentBytes), SchemaHash: base.SchemaHash, ScopeHash: base.ScopeHash,
		ReadOnly: false, ApprovalState: "not_required", IssuedAt: issuedAt, ExpiresAt: fixture.now.Add(20 * time.Minute),
	})
	if err := domainsecurity.ValidateExecutionGrant(grant); err != nil {
		t.Fatal(err)
	}
	fixture.appendItem(map[string]any{
		"id": domaintoolcall.ToolCallItemIDV1(fixture.securityContext.TurnID, callID), "kind": "tool_call", "role": "assistant", "status": "completed",
		"threadId": fixture.securityContext.ThreadID, "turnId": fixture.securityContext.TurnID,
		"toolName": grant.ToolName, "callId": grant.ToolCallID, "arguments": arguments, "createdAt": grant.IssuedAt,
		"contextDigest": fixture.securityContext.ContextDigest, "contextEpoch": float64(fixture.securityContext.ContextEpoch),
		"executionGrantId": grant.GrantID, "executionGrant": mapRecord(grant),
	})
	fixture.grantArguments[grant.GrantID] = arguments
	return grant
}
