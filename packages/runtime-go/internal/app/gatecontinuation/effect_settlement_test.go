package gatecontinuation

import (
	"context"
	"testing"
	"time"

	continuationapp "analytix.local/runtime-go/internal/app/continuation"
	controlapp "analytix.local/runtime-go/internal/app/control"
	effectgateapp "analytix.local/runtime-go/internal/app/effectgate"
	apploop "analytix.local/runtime-go/internal/app/loop"
	appmodel "analytix.local/runtime-go/internal/app/model"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestApprovedToolKeepsEffectLeaseThroughSettlement(t *testing.T) {
	gate := effectgateapp.New()
	now := time.Now().UTC()
	original := cancellationPendingFixture(t, domaincontinuation.KindApproval, now)
	store := newCancellationGateStore()
	continuations := continuationapp.NewService(newCancellationAuthority(), store)
	approvalID := controlapp.SecureGateID(
		domaincontinuation.KindApproval, original.ThreadID, original.TurnID,
		original.SecurityContext.ContextDigest, original.ExecutionGrant.GrantID, original.Call.ID,
	)
	_, err := continuations.IssueOrResolvePendingHost(
		domaincontinuation.KindApproval, approvalID, "item_"+approvalID, original, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := continuations.DisposePendingHostDisposition(
		approvalID, domaincontinuation.StatusAllowed, "approval_allowed", now.Add(time.Second), original,
	); err != nil {
		t.Fatal(err)
	}
	_, _, approvedGrant, err := continuations.ResolveApprovedGrantHost(approvalID, original)
	if err != nil {
		t.Fatal(err)
	}
	pending := original
	pending.ExecutionGrant = approvedGrant
	effectReturned := make(chan struct{})
	settlementEntered := make(chan struct{})
	allowSettlement := make(chan struct{})
	approvalDone := make(chan error, 1)
	dependencies := Dependencies{
		Continuations: continuations,
		ExecuteAndSettle: func(ctx context.Context, pending appmodel.PendingToolCall) (apploop.SettledToolExecution, error) {
			return apploop.ExecuteAndSettleTool(ctx, pending, gate.AcquireEffect,
				func(context.Context, appmodel.PendingToolCall, any) (any, bool) {
					close(effectReturned)
					return map[string]any{"executed": true}, false
				}, nil, nil,
				func(_ context.Context, _ appmodel.PendingToolCall, output any, isError bool) (apploop.SettledToolExecution, error) {
					close(settlementEntered)
					<-allowSettlement
					return apploop.SettledToolExecution{Output: output, IsError: isError, Message: domainmodel.Message{Role: "tool"}}, nil
				}, nil)
		},
	}
	go func() {
		_, err := executeApprovedTool(context.Background(), approvalID, original, pending, dependencies)
		approvalDone <- err
	}()
	<-effectReturned
	<-settlementEntered
	transitionAcquired := make(chan func(), 1)
	transitionStarted := make(chan struct{})
	go func() {
		close(transitionStarted)
		release, _ := gate.AcquireTransition(context.Background(), original.SecurityContext)
		transitionAcquired <- release
	}()
	<-transitionStarted
	select {
	case release := <-transitionAcquired:
		if release != nil {
			release()
		}
		t.Fatal("approval released effect authority before durable settlement")
	case <-time.After(25 * time.Millisecond):
	}
	close(allowSettlement)
	if err := <-approvalDone; err != nil {
		t.Fatal(err)
	}
	select {
	case release := <-transitionAcquired:
		if release == nil {
			t.Fatal("transition failed after approved tool settlement")
		}
		release()
	case <-time.After(time.Second):
		t.Fatal("transition did not resume after approved tool settlement")
	}
}
