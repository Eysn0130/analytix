package cachetelemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	cachetelemetryport "analytix.local/runtime-go/internal/ports/cachetelemetry"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func preserveTelemetryRestartForTestV1(t *testing.T, service *DurableService, contexts []domainsecurity.TurnSecurityContext) {
	t.Helper()
	if err := service.PreserveRestartContextsV1(context.Background(), contexts); err != nil {
		t.Fatal(err)
	}
}

func telemetryRestartBytesV1(t *testing.T, fixture durableServiceFixtureV1) []byte {
	t.Helper()
	body, err := json.Marshal(fixture.store.snapshot())
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestDurableRestartPreservationBlocksEveryWriter(t *testing.T) {
	for _, operation := range []string{"plan", "begin", "settle", "close", "apply_preexisting_plan"} {
		t.Run(operation, func(t *testing.T) {
			fixture := newDurableServiceFixtureV1(t)
			service, err := NewDurableService(fixture.authority, fixture.store)
			if err != nil {
				t.Fatal(err)
			}
			handle, err := service.BeginAttempt(context.Background(), fixture.registration(t, 1, 1, domaincachetelemetry.ProviderChannelPrimary, "2026-07-14T00:00:00Z"))
			if err != nil {
				t.Fatal(err)
			}
			plan, err := service.PlanRestartSettlements(context.Background(), durableServiceTimeV1(t, "2026-07-14T00:00:05Z"))
			if err != nil || len(plan.Settlements) != 1 {
				t.Fatalf("original open intent was not observed: count=%d err=%v", len(plan.Settlements), err)
			}
			before := telemetryRestartBytesV1(t, fixture)
			preserveTelemetryRestartForTestV1(t, service, []domainsecurity.TurnSecurityContext{fixture.securityContext})
			switch operation {
			case "plan":
				filtered, planErr := service.PlanRestartSettlements(context.Background(), durableServiceTimeV1(t, "2026-07-14T00:00:06Z"))
				if planErr != nil || len(filtered.Settlements) != 0 {
					t.Fatalf("preserved intent was reclassified: count=%d err=%v", len(filtered.Settlements), planErr)
				}
			case "begin":
				_, err = service.BeginAttempt(context.Background(), fixture.registration(t, 2, 1, domaincachetelemetry.ProviderChannelPrimary, "2026-07-14T00:00:06Z"))
			case "settle":
				err = service.SettleAttempt(context.Background(), cachetelemetryport.AttemptSettlementInputV1{
					Handle: handle, DispatchState: domaincachetelemetry.ProviderDispatchStateSent,
					Status: domaincachetelemetry.ProviderCallStatusSucceeded, Usage: completeProviderUsageV1(),
					SafeReasonCode: "provider_succeeded", SettledAt: durableServiceTimeV1(t, "2026-07-14T00:00:06Z"),
				})
			case "close":
				_, err = service.CloseTurn(context.Background(), fixture.securityContext, domaincachetelemetry.ProviderTurnTerminalRestartV1, durableServiceTimeV1(t, "2026-07-14T00:00:06Z"))
			case "apply_preexisting_plan":
				err = service.ApplyRestartSettlements(context.Background(), plan)
			}
			if operation != "plan" && (!errors.Is(err, ErrRestartPreserved) || !providerRetryForbiddenV1(err)) {
				t.Errorf("preserved %s did not fail without retry: %v", operation, err)
			}
			if !bytes.Equal(before, telemetryRestartBytesV1(t, fixture)) {
				t.Error("preserved telemetry authority bytes changed")
			}
		})
	}
}

func TestDurableRestartPreservationKeepsIndependentOrdinaryTurnUsable(t *testing.T) {
	fixture := newDurableServiceFixtureV1(t)
	service, err := NewDurableService(fixture.authority, fixture.store)
	if err != nil {
		t.Fatal(err)
	}
	held, err := service.BeginAttempt(context.Background(), fixture.registration(t, 1, 1, domaincachetelemetry.ProviderChannelPrimary, "2026-07-14T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	preserveTelemetryRestartForTestV1(t, service, []domainsecurity.TurnSecurityContext{fixture.securityContext})
	ordinary, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-independent", TurnID: "turn-independent", WorkspaceRealPath: "/workspace/independent",
		ContextEpoch: 1, IssuedAt: durableServiceTimeV1(t, "2026-07-14T00:00:01Z"),
	})
	if err != nil {
		t.Fatal(err)
	}
	input := fixture.registration(t, 1, 1, domaincachetelemetry.ProviderChannelPrimary, "2026-07-14T00:00:02Z")
	input.SecurityContext = ordinary
	input.OrdinaryEffect = true
	independent, err := service.BeginAttempt(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := service.PlanRestartSettlements(context.Background(), durableServiceTimeV1(t, "2026-07-14T00:00:05Z"))
	if err != nil || len(plan.Settlements) != 1 || plan.Settlements[0].IntentID != independent.Intent.IntentID {
		t.Fatalf("independent restart plan is not exact: count=%d err=%v", len(plan.Settlements), err)
	}
	if err := service.ApplyRestartSettlements(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CloseTurn(context.Background(), ordinary, domaincachetelemetry.ProviderTurnTerminalRestartV1, durableServiceTimeV1(t, "2026-07-14T00:00:06Z")); err != nil {
		t.Fatal(err)
	}
	snapshot := fixture.store.snapshot()
	if len(snapshot.Intents) != 2 || len(snapshot.Settlements) != 1 || len(snapshot.Closures) != 1 || snapshot.Settlements[0].IntentID == held.Intent.IntentID {
		t.Fatal("independent work changed held authority")
	}
}

func TestDurableRestartPreservationRejectsInvalidScopeAndCannotBeReleased(t *testing.T) {
	for _, fault := range []string{"invalid_context", "cancelled", "foreign_authority", "corrupt_intent"} {
		t.Run(fault, func(t *testing.T) {
			fixture := newDurableServiceFixtureV1(t)
			service, err := NewDurableService(fixture.authority, fixture.store)
			if err != nil {
				t.Fatal(err)
			}
			handle, err := service.BeginAttempt(context.Background(), fixture.registration(t, 1, 1, domaincachetelemetry.ProviderChannelPrimary, "2026-07-14T00:00:00Z"))
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			frozen := fixture.securityContext
			switch fault {
			case "invalid_context":
				frozen.TurnID = "different-turn"
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			case "foreign_authority":
				service, err = NewDurableService(newMemoryProviderAuthorityV1(t), fixture.store)
				if err != nil {
					t.Fatal(err)
				}
			case "corrupt_intent":
				mutated := handle.Intent
				mutated.ChannelOrdinal++
				fixture.store.intents[mutated.IntentID] = mutated
			}
			before := telemetryRestartBytesV1(t, fixture)
			if err := service.PreserveRestartContextsV1(ctx, []domainsecurity.TurnSecurityContext{frozen}); err == nil || !providerRetryForbiddenV1(err) {
				t.Fatalf("invalid preservation scope accepted or retryable: %v", err)
			}
			if service.restartPreserved != nil || !bytes.Equal(before, telemetryRestartBytesV1(t, fixture)) {
				t.Fatal("failed scope installation changed authority or installed partial hold")
			}
		})
	}
	fixture := newDurableServiceFixtureV1(t)
	service, err := NewDurableService(fixture.authority, fixture.store)
	if err != nil {
		t.Fatal(err)
	}
	preserveTelemetryRestartForTestV1(t, service, []domainsecurity.TurnSecurityContext{fixture.securityContext})
	if err := service.PreserveRestartContextsV1(context.Background(), nil); err == nil {
		t.Fatal("empty replacement released preservation")
	}
	if _, err := service.BeginAttempt(context.Background(), fixture.registration(t, 1, 1, domaincachetelemetry.ProviderChannelPrimary, "2026-07-14T00:00:00Z")); !errors.Is(err, ErrRestartPreserved) {
		t.Fatalf("held context became executable: %v", err)
	}
}

func TestDurableRestartPreservationPreflightsHeldSuffixBeforeAnyWrite(t *testing.T) {
	fixture := newDurableServiceFixtureV1(t)
	service, err := NewDurableService(fixture.authority, fixture.store)
	if err != nil {
		t.Fatal(err)
	}
	// A fresh canonical context makes a second independent signed turn binding.
	other, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-other", TurnID: "turn-other", WorkspaceRealPath: "/workspace/other",
		ContextEpoch: 1, IssuedAt: durableServiceTimeV1(t, "2026-07-14T00:00:00Z"),
	})
	if err != nil {
		t.Fatal(err)
	}
	contexts := map[string]domainsecurity.TurnSecurityContext{}
	for _, frozen := range []domainsecurity.TurnSecurityContext{fixture.securityContext, other} {
		input := fixture.registration(t, 1, 1, domaincachetelemetry.ProviderChannelPrimary, "2026-07-14T00:00:01Z")
		input.SecurityContext = frozen
		input.OrdinaryEffect = true
		handle, err := service.BeginAttempt(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		contexts[handle.Intent.IntentID] = frozen
	}
	plan, err := service.PlanRestartSettlements(context.Background(), durableServiceTimeV1(t, "2026-07-14T00:00:05Z"))
	if err != nil || len(plan.Settlements) != 2 {
		t.Fatalf("two-intent plan missing: count=%d err=%v", len(plan.Settlements), err)
	}
	preserveTelemetryRestartForTestV1(t, service, []domainsecurity.TurnSecurityContext{contexts[plan.Settlements[1].IntentID]})
	before := telemetryRestartBytesV1(t, fixture)
	if err := service.ApplyRestartSettlements(context.Background(), plan); !errors.Is(err, ErrRestartPreserved) {
		t.Fatalf("held suffix was not rejected: %v", err)
	}
	if !bytes.Equal(before, telemetryRestartBytesV1(t, fixture)) {
		t.Fatal("stale plan wrote its unheld prefix before rejecting held suffix")
	}
}
