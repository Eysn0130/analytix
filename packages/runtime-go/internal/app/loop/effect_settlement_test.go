package loop

import (
	"context"
	"errors"
	"testing"
	"time"

	effectgateapp "analytix.local/runtime-go/internal/app/effectgate"
	appmodel "analytix.local/runtime-go/internal/app/model"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestToolEffectLeaseCoversBlockedDurableSettlement(t *testing.T) {
	gate := effectgateapp.New()
	pending := effectSettlementPending(t)
	effectReturned := make(chan struct{})
	settlementEntered := make(chan struct{})
	allowSettlement := make(chan struct{})
	transactionDone := make(chan error, 1)
	go func() {
		_, err := ExecuteAndSettleTool(context.Background(), pending, gate.AcquireEffect,
			func(context.Context, appmodel.PendingToolCall, any) (any, bool) {
				close(effectReturned)
				return map[string]any{"executed": true}, false
			}, nil, nil,
			func(_ context.Context, _ appmodel.PendingToolCall, output any, isError bool) (SettledToolExecution, error) {
				close(settlementEntered)
				<-allowSettlement
				return SettledToolExecution{Output: output, IsError: isError, Message: domainmodel.Message{Role: "tool"}}, nil
			}, nil)
		transactionDone <- err
	}()
	<-effectReturned
	<-settlementEntered
	transitionAcquired := startEffectSettlementTransition(gate, pending.SecurityContext)
	assertTransitionBlocked(t, transitionAcquired, "effect lease ended before durable settlement")
	close(allowSettlement)
	if err := <-transactionDone; err != nil {
		t.Fatal(err)
	}
	releaseTransition(t, transitionAcquired)
}

func TestToolEffectLeaseCoversSettlementFailure(t *testing.T) {
	gate := effectgateapp.New()
	pending := effectSettlementPending(t)
	settlementEntered := make(chan struct{})
	allowFailure := make(chan struct{})
	wantErr := errors.New("durable settlement failed")
	transactionDone := make(chan error, 1)
	go func() {
		_, err := ExecuteAndSettleTool(context.Background(), pending, gate.AcquireEffect,
			func(context.Context, appmodel.PendingToolCall, any) (any, bool) {
				return map[string]any{"executed": true}, false
			}, nil, nil,
			func(_ context.Context, _ appmodel.PendingToolCall, output any, isError bool) (SettledToolExecution, error) {
				close(settlementEntered)
				<-allowFailure
				return SettledToolExecution{Output: output, IsError: isError}, wantErr
			}, nil)
		transactionDone <- err
	}()
	<-settlementEntered
	transitionAcquired := startEffectSettlementTransition(gate, pending.SecurityContext)
	assertTransitionBlocked(t, transitionAcquired, "settlement failure released effect authority early")
	close(allowFailure)
	if err := <-transactionDone; !errors.Is(err, wantErr) {
		t.Fatalf("settlement error mismatch: %v", err)
	}
	releaseTransition(t, transitionAcquired)
}

func TestToolTransformFailureCannotSkipDurableSettlement(t *testing.T) {
	gate := effectgateapp.New()
	pending := effectSettlementPending(t)
	wantErr := errors.New("loop guard persistence failed")
	settled := false
	_, err := ExecuteAndSettleTool(context.Background(), pending, gate.AcquireEffect,
		func(context.Context, appmodel.PendingToolCall, any) (any, bool) {
			return map[string]any{"executed": true}, false
		}, nil,
		func(output any, isError bool) (any, bool, error) {
			return output, isError, wantErr
		},
		func(_ context.Context, _ appmodel.PendingToolCall, output any, isError bool) (SettledToolExecution, error) {
			settled = true
			return SettledToolExecution{Output: output, IsError: isError, Message: domainmodel.Message{Role: "tool"}}, nil
		}, nil)
	if !errors.Is(err, wantErr) || !settled {
		t.Fatalf("transform failure skipped settlement: settled=%v err=%v", settled, err)
	}
}

func TestDurableSettlementDowngradeIsReturnedToTheLoop(t *testing.T) {
	gate := effectgateapp.New()
	pending := effectSettlementPending(t)
	settled, err := ExecuteAndSettleTool(context.Background(), pending, gate.AcquireEffect,
		func(context.Context, appmodel.PendingToolCall, any) (any, bool) {
			return map[string]any{"executed": true}, false
		}, nil, nil,
		func(context.Context, appmodel.PendingToolCall, any, bool) (SettledToolExecution, error) {
			return SettledToolExecution{
				Output:  map[string]any{"code": "execution_grant_context_mismatch", "executed": false},
				IsError: true,
				Message: domainmodel.Message{Role: "tool", Content: "execution_grant_context_mismatch"},
			}, nil
		}, nil)
	if err != nil || !settled.IsError || settled.Output.(map[string]any)["executed"] != false {
		t.Fatalf("durable settlement downgrade was lost: settled=%#v err=%v", settled, err)
	}
	if code := settled.AuthorityFailureCode(); code != "execution_grant_context_mismatch" {
		t.Fatalf("settlement authority downgrade code mismatch: %q", code)
	}
}

func TestCancelledSyntheticToolOutputStillSettlesUnderEffectLease(t *testing.T) {
	gate := effectgateapp.New()
	pending := effectSettlementPending(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	settlementEntered := make(chan struct{})
	allowSettlement := make(chan struct{})
	transactionDone := make(chan error, 1)
	go func() {
		_, err := SettleToolOutput(ctx, pending, gate.AcquireEffect, map[string]any{"code": "cancelled"}, true,
			func(settlementCtx context.Context, _ appmodel.PendingToolCall, output any, isError bool) (SettledToolExecution, error) {
				if settlementCtx.Err() != nil {
					return SettledToolExecution{Output: output, IsError: isError}, settlementCtx.Err()
				}
				close(settlementEntered)
				<-allowSettlement
				return SettledToolExecution{Output: output, IsError: isError, Message: domainmodel.Message{Role: "tool"}}, nil
			})
		transactionDone <- err
	}()
	<-settlementEntered
	transitionAcquired := startEffectSettlementTransition(gate, pending.SecurityContext)
	assertTransitionBlocked(t, transitionAcquired, "cancelled settlement did not retain effect authority")
	close(allowSettlement)
	if err := <-transactionDone; err != nil {
		t.Fatal(err)
	}
	releaseTransition(t, transitionAcquired)
}

func TestCancellationAfterEffectCannotCancelDurableSettlement(t *testing.T) {
	gate := effectgateapp.New()
	pending := effectSettlementPending(t)
	ctx, cancel := context.WithCancel(context.Background())
	settlementCalled := false
	_, err := ExecuteAndSettleTool(ctx, pending, gate.AcquireEffect,
		func(context.Context, appmodel.PendingToolCall, any) (any, bool) {
			cancel()
			return map[string]any{"code": "cancelled", "executed": true}, true
		}, nil, nil,
		func(settlementCtx context.Context, _ appmodel.PendingToolCall, output any, isError bool) (SettledToolExecution, error) {
			settlementCalled = true
			if settlementCtx.Err() != nil {
				return SettledToolExecution{Output: output, IsError: isError}, settlementCtx.Err()
			}
			return SettledToolExecution{Output: output, IsError: isError, Message: domainmodel.Message{Role: "tool"}}, nil
		}, nil)
	if err != nil || !settlementCalled {
		t.Fatalf("post-effect cancellation skipped durable settlement: called=%v err=%v", settlementCalled, err)
	}
}

func effectSettlementPending(t *testing.T) appmodel.PendingToolCall {
	t.Helper()
	securityContext, err := testsecurity.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-effect-settlement", TurnID: "turn-effect-settlement", WorkspaceRealPath: t.TempDir(),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("effect-settlement-manifest")), ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return appmodel.PendingToolCall{ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, SecurityContext: securityContext}
}

func assertTransitionBlocked(t *testing.T, acquired <-chan func(), message string) {
	t.Helper()
	select {
	case release := <-acquired:
		if release != nil {
			release()
		}
		t.Fatal(message)
	case <-time.After(25 * time.Millisecond):
	}
}

func startEffectSettlementTransition(gate *effectgateapp.Gate, securityContext domainsecurity.TurnSecurityContext) <-chan func() {
	started := make(chan struct{})
	acquired := make(chan func(), 1)
	go func() {
		close(started)
		release, _ := gate.AcquireTransition(context.Background(), securityContext)
		acquired <- release
	}()
	<-started
	return acquired
}

func releaseTransition(t *testing.T, acquired <-chan func()) {
	t.Helper()
	select {
	case release := <-acquired:
		if release == nil {
			t.Fatal("transition failed after tool settlement")
		}
		release()
	case <-time.After(time.Second):
		t.Fatal("transition did not resume after tool settlement")
	}
}

func TestAccountFlowSettlementDeadlinePolicyV1(t *testing.T) {
	for _, name := range []string{"native", "read_only_batch", "cancelled_batch", "other_tool", "similar_tool_name", "invalid_grant", "missing_source", "expired_carrier", "missing_carrier", "tampered_raw", "error_output", "override", "preflight_dispatch", "pure_synthetic"} {
		t.Run(name, func(t *testing.T) {
			base, output, raw, semantic := providerAttemptAccountFlowFixtureV1(t)
			pending := loopPendingToolCall(t, base.SecurityContext, base.Call.Name, string(base.Call.Arguments), true, time.Now().UTC())
			source := &providerAttemptFundsSourceV1{semantic: semantic, active: true}
			var sourceOwner any = source
			var override any
			var dispatch ToolDispatchBoundary
			isError := false
			want := toolSettlementTimeout
			switch name {
			case "native", "read_only_batch":
				want = accountFlowSettlementTimeoutV1
			case "cancelled_batch":
				output = map[string]any{"executed": false, "code": "cancelled"}
				isError = true
			case "other_tool":
				pending = loopPendingToolCall(t, base.SecurityContext, "mcp__analytix_funds__count_case_rows", string(base.Call.Arguments), true, time.Now().UTC())
			case "similar_tool_name":
				pending = loopPendingToolCall(t, base.SecurityContext, base.Call.Name+"_extra", string(base.Call.Arguments), true, time.Now().UTC())
			case "invalid_grant":
				pending.ExecutionGrant.ContextDigest = domainsecurity.SHA256Hex([]byte("other-context"))
			case "missing_source":
				sourceOwner = nil
			case "expired_carrier":
				source.active = false
			case "missing_carrier":
				delete(output, domainmcp.HostRawToolResultKey)
			case "tampered_raw":
				raw.RawSHA256 = domainsecurity.SHA256Hex([]byte("other-raw-result"))
				output[domainmcp.HostRawToolResultKey] = raw
			case "error_output":
				isError = true
			case "override":
				override = map[string]any{"executed": true}
			case "preflight_dispatch":
				dispatch = func(ctx context.Context, _ appmodel.PendingToolCall, run ToolDispatchRun) (SettledToolExecution, error) {
					return run(ctx, &ToolDispatchOverride{Output: output})
				}
			}
			type valueKey struct{}
			ctx, cancel := context.WithCancel(context.WithValue(context.Background(), valueKey{}, "preserved"))
			defer cancel()
			released, executed := false, false
			acquire := func(ctx context.Context, _ domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
				return ctx, func() { released = true }, nil
			}
			var settledContext context.Context
			settle := func(current context.Context, _ appmodel.PendingToolCall, _ any, _ bool) (SettledToolExecution, error) {
				cancel()
				settledContext = current
				deadline, ok := current.Deadline()
				remaining := time.Until(deadline)
				if !ok || remaining > want || remaining < want-time.Second || current.Err() != nil || current.Value(valueKey{}) != "preserved" || released {
					t.Fatalf("settlement policy/lifetime mismatch: deadline=%v remaining=%v want=%v cancelled=%v released=%v", ok, remaining, want, current.Err() != nil, released)
				}
				return SettledToolExecution{}, nil
			}
			var err error
			if name == "read_only_batch" || name == "cancelled_batch" {
				_, err = SettleReadOnlyBatchToolOutputV1(ctx, pending, acquire, output, isError, settle, sourceOwner)
			} else if name == "pure_synthetic" {
				_, err = SettleToolOutput(ctx, pending, acquire, output, false, settle)
			} else {
				_, err = executeAndSettleToolWithAccountFlowSourceV1(ctx, pending, acquire,
					func(context.Context, appmodel.PendingToolCall, any) (any, bool) {
						executed = true
						cancel()
						return output, isError
					}, override, nil, settle, dispatch, sourceOwner)
			}
			if err != nil || !released || settledContext == nil || !errors.Is(settledContext.Err(), context.Canceled) {
				t.Fatal("settlement did not close its owned deadline and release authority on return")
			}
			if executed != (name != "preflight_dispatch" && name != "pure_synthetic" && name != "read_only_batch" && name != "cancelled_batch") {
				t.Fatal("synthetic outcome crossed actual execution")
			}
			if source.discardInvocations != 0 {
				t.Fatal("budget selection consumed or discarded evidence")
			}
		})
	}
}

func TestAccountFlowSettlementLeaseSpansSingleDeadlineAndReturnV1(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "success"
		if fail {
			name = "failure"
		}
		t.Run(name, func(t *testing.T) {
			base, output, _, semantic := providerAttemptAccountFlowFixtureV1(t)
			pending := loopPendingToolCall(t, base.SecurityContext, base.Call.Name, string(base.Call.Arguments), true, time.Now().UTC())
			gate := effectgateapp.New()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			entered := make(chan context.Context, 1)
			continueSettlement := make(chan struct{})
			done := make(chan error, 1)
			wantErr := errors.New("controlled settlement failure")
			go func() {
				_, err := ExecuteAndSettleWithSideEffectIntent(SideEffectIntentExecutionInput{
					Context: ctx, Pending: pending, Acquire: gate.AcquireEffect,
					AccountFlowSource: &providerAttemptFundsSourceV1{semantic: semantic, active: true},
					Execute: func(context.Context, appmodel.PendingToolCall, any) (any, bool) {
						cancel()
						return output, false
					},
					Settle: func(current context.Context, _ appmodel.PendingToolCall, _ any, _ bool) (SettledToolExecution, error) {
						deadline, ok := current.Deadline()
						if !ok || current.Err() != nil || time.Until(deadline) < accountFlowSettlementTimeoutV1-time.Second {
							return SettledToolExecution{}, errors.New("native settlement has no detached owned budget")
						}
						entered <- current
						<-continueSettlement
						after, _ := current.Deadline()
						if !after.Equal(deadline) || current.Err() != nil {
							return SettledToolExecution{}, errors.New("settlement phase renewed or lost its original budget")
						}
						if fail {
							return SettledToolExecution{}, wantErr
						}
						return SettledToolExecution{}, nil
					},
				})
				done <- err
			}()
			var settlementContext context.Context
			select {
			case settlementContext = <-entered:
			case err := <-done:
				t.Fatalf("settlement did not enter: %v", err)
			case <-time.After(2 * time.Second):
				t.Fatal("settlement entry timed out")
			}
			transition := startEffectSettlementTransition(gate, pending.SecurityContext)
			assertTransitionBlocked(t, transition, "native receipt settlement released authority between phases")
			close(continueSettlement)
			if err := <-done; (fail && !errors.Is(err, wantErr)) || (!fail && err != nil) {
				t.Fatalf("settlement result changed: %v", err)
			}
			releaseTransition(t, transition)
			if !errors.Is(settlementContext.Err(), context.Canceled) {
				t.Fatal("completed native settlement retained a live detached context")
			}
		})
	}
}
