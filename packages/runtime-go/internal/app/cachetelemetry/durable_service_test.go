package cachetelemetry

import (
	"bytes"
	"context"
	"crypto/ed25519"
	cryptorand "crypto/rand"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	cachetelemetryport "analytix.local/runtime-go/internal/ports/cachetelemetry"
	cachetelemetrystoreport "analytix.local/runtime-go/internal/ports/cachetelemetrystore"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestDurableServiceCommitsTrustedCrossRestartLedgerWithoutRawSecrets(t *testing.T) {
	fixture := newDurableServiceFixtureV1(t)
	observedAuthority := &observingProviderAuthorityV1{Authority: fixture.authority}
	service, err := NewDurableService(observedAuthority, fixture.store)
	if err != nil {
		t.Fatal(err)
	}
	firstInput := fixture.registration(t, 1, 1, domaincachetelemetry.ProviderChannelPrimary, "2026-07-14T00:00:00Z")
	first, err := service.BeginAttempt(context.Background(), firstInput)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SettleAttempt(context.Background(), cachetelemetryport.AttemptSettlementInputV1{
		Handle: first, DispatchState: domaincachetelemetry.ProviderDispatchStateSent,
		Status: domaincachetelemetry.ProviderCallStatusSucceeded, Usage: completeProviderUsageV1(),
		SafeReasonCode: "provider_succeeded", SettledAt: durableServiceTimeV1(t, "2026-07-14T00:00:01Z"),
	}); err != nil {
		t.Fatal(err)
	}

	reopenedStore := fixture.reopenStore(t)
	restarted, err := NewDurableService(observedAuthority, reopenedStore)
	if err != nil {
		t.Fatal(err)
	}
	secondInput := fixture.registration(t, 1, 2, domaincachetelemetry.ProviderChannelPrimary, "2026-07-14T00:00:02Z")
	second, err := restarted.BeginAttempt(context.Background(), secondInput)
	if err != nil {
		t.Fatal(err)
	}
	if first.Intent.Shape.LogicalCallHMAC != second.Intent.Shape.LogicalCallHMAC ||
		first.Intent.Shape.DigestEpoch != second.Intent.Shape.DigestEpoch || second.Intent.Shape.Attempt != 2 ||
		second.Intent.LogicalCallOrdinal != first.Intent.LogicalCallOrdinal || second.Intent.ChannelOrdinal != first.Intent.ChannelOrdinal {
		t.Fatalf("provider telemetry identity did not survive restart: first=%#v second=%#v", first.Intent, second.Intent)
	}
	if err := restarted.SettleAttempt(context.Background(), cachetelemetryport.AttemptSettlementInputV1{
		Handle: second, DispatchState: domaincachetelemetry.ProviderDispatchStateSent,
		Status: domaincachetelemetry.ProviderCallStatusSucceeded, Usage: completeProviderUsageV1(),
		SafeReasonCode: "provider_succeeded", SettledAt: durableServiceTimeV1(t, "2026-07-14T00:00:03Z"),
	}); err != nil {
		t.Fatal(err)
	}
	closure, err := restarted.CloseTurn(context.Background(), fixture.securityContext, domaincachetelemetry.ProviderTurnTerminalSuccessV1, durableServiceTimeV1(t, "2026-07-14T00:00:04Z"))
	if err != nil {
		t.Fatal(err)
	}
	if closure.IntentCount != 2 || closure.SettlementCount != 2 || !closure.BenchmarkEligible {
		t.Fatalf("unexpected trusted provider turn closure: %#v", closure)
	}
	if err := VerifyTrustedInventoryV1(context.Background(), reopenedStore, observedAuthority); err != nil {
		t.Fatal(err)
	}
	for _, signed := range observedAuthority.Messages() {
		assertProviderTelemetrySecretsAbsentV1(t, signed)
	}
	durableBytes, err := json.Marshal(fixture.store.snapshot())
	if err != nil {
		t.Fatal(err)
	}
	assertProviderTelemetrySecretsAbsentV1(t, durableBytes)
}

func TestDurableServiceFullInventoryUsesOneCrossLeafStoreVisit(t *testing.T) {
	fixture := newDurableServiceFixtureV1(t)
	store := &countingProviderTelemetryInventoryStoreV1{memoryProviderTelemetryStoreV1: fixture.store}
	service, err := NewDurableService(fixture.authority, store)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := service.fullInventoryV1(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.inventoryVisits != 1 || store.intentVisits != 0 || store.settlementVisits != 0 || store.closureVisits != 0 {
		t.Fatalf(
			"full inventory visits = fused:%d intents:%d settlements:%d closures:%d",
			store.inventoryVisits, store.intentVisits, store.settlementVisits, store.closureVisits,
		)
	}
}

func TestDurableServiceBeginAttemptUsesCommittedIntentWithoutRedundantRead(t *testing.T) {
	fixture := newDurableServiceFixtureV1(t)
	store := &countingProviderTelemetryInventoryStoreV1{memoryProviderTelemetryStoreV1: fixture.store}
	service, err := NewDurableService(fixture.authority, store)
	if err != nil {
		t.Fatal(err)
	}
	input := fixture.registration(t, 1, 1, domaincachetelemetry.ProviderChannelPrimary, "2026-07-14T00:00:00Z")
	if _, err := service.BeginAttempt(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if store.inventoryVisits != 1 || store.intentReads != 0 {
		t.Fatalf("begin attempt observations = inventory:%d intent-reads:%d", store.inventoryVisits, store.intentReads)
	}
}

func TestDurableServiceSettleAttemptUsesCommittedAuthorityWithoutRedundantReads(t *testing.T) {
	fixture := newDurableServiceFixtureV1(t)
	store := &countingProviderTelemetryInventoryStoreV1{memoryProviderTelemetryStoreV1: fixture.store}
	service, err := NewDurableService(fixture.authority, store)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := service.BeginAttempt(
		context.Background(),
		fixture.registration(t, 1, 1, domaincachetelemetry.ProviderChannelPrimary, "2026-07-14T00:00:00Z"),
	)
	if err != nil {
		t.Fatal(err)
	}
	store.intentReads = 0
	store.settlementReads = 0
	if err := service.SettleAttempt(context.Background(), cachetelemetryport.AttemptSettlementInputV1{
		Handle: handle, DispatchState: domaincachetelemetry.ProviderDispatchStateSent,
		Status: domaincachetelemetry.ProviderCallStatusSucceeded, Usage: completeProviderUsageV1(),
		SafeReasonCode: "provider_succeeded", SettledAt: durableServiceTimeV1(t, "2026-07-14T00:00:01Z"),
	}); err != nil {
		t.Fatal(err)
	}
	if store.intentReads != 0 || store.settlementReads != 0 {
		t.Fatalf("settle attempt redundant reads = intent:%d settlement:%d", store.intentReads, store.settlementReads)
	}
}

func TestDurableServiceCloseTurnUsesCommittedAuthorityWithoutRedundantReads(t *testing.T) {
	fixture := newDurableServiceFixtureV1(t)
	store := &countingProviderTelemetryInventoryStoreV1{memoryProviderTelemetryStoreV1: fixture.store}
	service, err := NewDurableService(fixture.authority, store)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := service.BeginAttempt(
		context.Background(),
		fixture.registration(t, 1, 1, domaincachetelemetry.ProviderChannelPrimary, "2026-07-14T00:00:00Z"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SettleAttempt(context.Background(), cachetelemetryport.AttemptSettlementInputV1{
		Handle: handle, DispatchState: domaincachetelemetry.ProviderDispatchStateSent,
		Status: domaincachetelemetry.ProviderCallStatusSucceeded, Usage: completeProviderUsageV1(),
		SafeReasonCode: "provider_succeeded", SettledAt: durableServiceTimeV1(t, "2026-07-14T00:00:01Z"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CloseTurn(
		context.Background(), fixture.securityContext,
		domaincachetelemetry.ProviderTurnTerminalSuccessV1,
		durableServiceTimeV1(t, "2026-07-14T00:00:02Z"),
	); err != nil {
		t.Fatal(err)
	}
	if store.inventoryVisits != 1 || store.closureReads != 0 {
		t.Fatalf("attempt lifecycle closure observations = inventory:%d closure-reads:%d", store.inventoryVisits, store.closureReads)
	}
}

func TestDurableServiceReusesTrustedProcessInventoryAcrossAttemptLifecycle(t *testing.T) {
	fixture := newDurableServiceFixtureV1(t)
	store := &countingProviderTelemetryInventoryStoreV1{memoryProviderTelemetryStoreV1: fixture.store}
	service, err := NewDurableService(fixture.authority, store)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.BeginAttempt(
		context.Background(),
		fixture.registration(t, 1, 1, domaincachetelemetry.ProviderChannelPrimary, "2026-07-14T00:00:00Z"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SettleAttempt(context.Background(), cachetelemetryport.AttemptSettlementInputV1{
		Handle: first, DispatchState: domaincachetelemetry.ProviderDispatchStateSent,
		Status: domaincachetelemetry.ProviderCallStatusSucceeded, Usage: completeProviderUsageV1(),
		SafeReasonCode: "provider_succeeded", SettledAt: durableServiceTimeV1(t, "2026-07-14T00:00:01Z"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.BeginAttempt(
		context.Background(),
		fixture.registration(t, 1, 2, domaincachetelemetry.ProviderChannelPrimary, "2026-07-14T00:00:02Z"),
	); err != nil {
		t.Fatal(err)
	}
	if store.inventoryVisits != 1 {
		t.Fatalf("attempt lifecycle reloaded an unchanged trusted inventory: visits=%d", store.inventoryVisits)
	}
}

func TestDurableServiceAdmitsWitnessedBoundaryOnlyForOrdinaryProviderEffect(t *testing.T) {
	fixture := newDurableServiceFixtureV1(t)
	service, err := NewDurableService(fixture.authority, fixture.store)
	if err != nil {
		t.Fatal(err)
	}
	input := fixture.registration(t, 1, 1, domaincachetelemetry.ProviderChannelPrimary, "2026-07-14T00:00:00Z")
	input.SecurityContext = providerTelemetryWitnessedBoundaryContextV1(t)
	input.OrdinaryEffect = true
	if _, err := service.BeginAttempt(context.Background(), input); err != nil {
		t.Fatalf("witnessed boundary ordinary provider telemetry was rejected: %v", err)
	}

	strict := input
	strict.LogicalSequence = 2
	strict.OrdinaryEffect = false
	if _, err := service.BeginAttempt(context.Background(), strict); err == nil {
		t.Fatal("witnessed boundary telemetry acquired strict case execution authority")
	}

	quarantined, err := securitycontexttest.BoundaryOnlyContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-cache-boundary-quarantined", TurnID: "turn-cache-boundary-quarantined",
		WorkspaceRealPath: "/workspace/cache-boundary-quarantined", ContextEpoch: 1,
		IssuedAt: durableServiceTimeV1(t, "2026-07-14T00:00:00Z"),
	})
	if err != nil {
		t.Fatal(err)
	}
	blocked := input
	blocked.LogicalSequence = 3
	blocked.SecurityContext = quarantined
	if _, err := service.BeginAttempt(context.Background(), blocked); err == nil {
		t.Fatal("quarantined boundary telemetry acquired ordinary effect authority")
	}
}

func TestDurableServiceObservesOnlyExistingTrustedTurnClosure(t *testing.T) {
	fixture := newDurableServiceFixtureV1(t)
	service, err := NewDurableService(fixture.authority, fixture.store)
	if err != nil {
		t.Fatal(err)
	}
	if closure, found, err := service.ObserveTurnClosureV1(context.Background(), fixture.securityContext); err != nil || found || closure.ClosureID != "" {
		t.Fatalf("missing provider closure was not a read-only miss: closure=%#v found=%v err=%v", closure, found, err)
	}
	want, err := service.CloseTurn(context.Background(), fixture.securityContext, domaincachetelemetry.ProviderTurnTerminalSuccessV1, durableServiceTimeV1(t, "2026-07-14T00:00:01Z"))
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := service.ObserveTurnClosureV1(context.Background(), fixture.securityContext)
	if err != nil || !found || got != want {
		t.Fatalf("trusted provider closure observation mismatch: got=%#v found=%v err=%v", got, found, err)
	}
}

func TestVisitTurnClosuresDoesNotHoldServiceLockDuringCallback(t *testing.T) {
	fixture := newDurableServiceFixtureV1(t)
	service, err := NewDurableService(fixture.authority, fixture.store)
	if err != nil {
		t.Fatal(err)
	}
	want, err := service.CloseTurn(
		context.Background(), fixture.securityContext, domaincachetelemetry.ProviderTurnTerminalSuccessV1,
		durableServiceTimeV1(t, "2026-07-14T00:00:01Z"),
	)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- service.VisitTurnClosuresV1(context.Background(), func(visited domaincachetelemetry.ProviderTurnClosureV1) error {
			observed, found, observeErr := service.ObserveTurnClosureV1(context.Background(), fixture.securityContext)
			if observeErr != nil || !found || observed != want || visited != want {
				return errors.New("provider closure callback could not re-enter the service read path")
			}
			return nil
		})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("provider closure visitor held the service lock while invoking its callback")
	}
}

func TestDurableServiceRestartSettlementIsIndeterminateUnknownAndIdempotent(t *testing.T) {
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
		t.Fatalf("restart plan mismatch: %#v err=%v", plan, err)
	}
	settlement := plan.Settlements[0]
	if settlement.IntentID != handle.Intent.IntentID || settlement.DispatchState != domaincachetelemetry.ProviderDispatchStateIndeterminate ||
		settlement.Observation.Status != domaincachetelemetry.ProviderCallStatusRestartInterrupted ||
		settlement.Observation.Usage != (domaincachetelemetry.ProviderUsageV1{}) {
		t.Fatalf("restart settlement upgraded unknown authority: %#v", settlement)
	}
	if err := service.ApplyRestartSettlements(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if err := service.ApplyRestartSettlements(context.Background(), plan); err != nil {
		t.Fatalf("exact restart settlement replay was not idempotent: %v", err)
	}
	closure, err := service.CloseTurn(context.Background(), fixture.securityContext, domaincachetelemetry.ProviderTurnTerminalRestartV1, durableServiceTimeV1(t, "2026-07-14T00:00:06Z"))
	if err != nil {
		t.Fatal(err)
	}
	if closure.BenchmarkEligible {
		t.Fatal("restart-interrupted turn became cache benchmark eligible")
	}
	if plan, err := service.PlanRestartSettlements(context.Background(), durableServiceTimeV1(t, "2026-07-14T00:00:07Z")); err != nil || len(plan.Settlements) != 0 {
		t.Fatalf("settled restart intent remained orphaned: %#v err=%v", plan, err)
	}
}

func TestDurableServiceRejectsWrongInstallationAuthorityAndLateClosedTurnAttempt(t *testing.T) {
	fixture := newDurableServiceFixtureV1(t)
	service, err := NewDurableService(fixture.authority, fixture.store)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := service.BeginAttempt(context.Background(), fixture.registration(t, 1, 1, domaincachetelemetry.ProviderChannelPrimary, "2026-07-14T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SettleAttempt(context.Background(), cachetelemetryport.AttemptSettlementInputV1{
		Handle: handle, DispatchState: domaincachetelemetry.ProviderDispatchStateSent,
		Status: domaincachetelemetry.ProviderCallStatusSucceeded, Usage: completeProviderUsageV1(),
		SafeReasonCode: "provider_succeeded", SettledAt: durableServiceTimeV1(t, "2026-07-14T00:00:01Z"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CloseTurn(context.Background(), fixture.securityContext, domaincachetelemetry.ProviderTurnTerminalSuccessV1, durableServiceTimeV1(t, "2026-07-14T00:00:02Z")); err != nil {
		t.Fatal(err)
	}
	late := fixture.registration(t, 2, 1, domaincachetelemetry.ProviderChannelPrimary, "2026-07-14T00:00:03Z")
	if _, err := service.BeginAttempt(context.Background(), late); err == nil || !providerRetryForbiddenV1(err) {
		t.Fatalf("closed provider turn accepted late attempt or returned retryable failure: %v", err)
	}

	otherAuthority := newMemoryProviderAuthorityV1(t)
	if err := VerifyTrustedInventoryV1(context.Background(), fixture.store, otherAuthority); err == nil {
		t.Fatal("self-signed provider ledger passed a different installation authority")
	}
}

func TestDurableServiceClosureAcceptsFrozenV2ContextsAndConflictsFailClosed(t *testing.T) {
	fixture := newDurableServiceFixtureV1(t)
	service, err := NewDurableService(fixture.authority, fixture.store)
	if err != nil {
		t.Fatal(err)
	}
	issuedAt := durableServiceTimeV1(t, "2026-07-14T00:00:00Z")
	general, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-general", TurnID: "turn-general", WorkspaceRealPath: "/workspace/general",
		ContextEpoch: 1, IssuedAt: issuedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := securitycontexttest.BoundaryOnlyContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-boundary", TurnID: "turn-boundary", WorkspaceRealPath: "/workspace/boundary",
		ContextEpoch: 1, IssuedAt: issuedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, securityContext := range map[string]domainsecurity.TurnSecurityContext{
		"general": general, "case": fixture.securityContext, "boundary": boundary,
	} {
		t.Run(name, func(t *testing.T) {
			closedAt := durableServiceTimeV1(t, "2026-07-14T00:00:01Z")
			first, err := service.CloseTurn(context.Background(), securityContext, domaincachetelemetry.ProviderTurnTerminalCancelV1, closedAt)
			if err != nil || first.IntentCount != 0 || first.SettlementCount != 0 {
				t.Fatalf("frozen V2 context did not produce an empty closure: closure=%#v err=%v", first, err)
			}
			replayed, err := service.CloseTurn(context.Background(), securityContext, domaincachetelemetry.ProviderTurnTerminalCancelV1, closedAt.Add(time.Minute))
			if err != nil || replayed != first {
				t.Fatalf("same-reason closure was not idempotent: first=%#v replay=%#v err=%v", first, replayed, err)
			}
			if _, err := service.CloseTurn(context.Background(), securityContext, domaincachetelemetry.ProviderTurnTerminalSuccessV1, closedAt); err == nil || !providerRetryForbiddenV1(err) {
				t.Fatalf("different terminal reason did not fail closed: %v", err)
			}
		})
	}

	legacy := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-v1", TurnID: "turn-v1", WorkspaceRealPath: "/workspace/v1",
		ContextEpoch: 1, IssuedAt: issuedAt,
	})
	if _, err := service.CloseTurn(context.Background(), legacy, domaincachetelemetry.ProviderTurnTerminalRestartV1, issuedAt.Add(time.Minute)); err == nil || !providerRetryForbiddenV1(err) {
		t.Fatalf("audit-only V1 context closed a live provider turn: %v", err)
	}
	if _, err := service.CloseTurn(context.Background(), general, "completed", issuedAt.Add(time.Minute)); err == nil || !providerRetryForbiddenV1(err) {
		t.Fatalf("free-form terminal reason was accepted: %v", err)
	}
}

func TestDurableServiceCachedClosureNeverReplacesMissingDurableAuthority(t *testing.T) {
	fixture := newDurableServiceFixtureV1(t)
	service, err := NewDurableService(fixture.authority, fixture.store)
	if err != nil {
		t.Fatal(err)
	}
	closedAt := durableServiceTimeV1(t, "2026-07-14T00:00:01Z")
	closure, err := service.CloseTurn(
		context.Background(),
		fixture.securityContext,
		domaincachetelemetry.ProviderTurnTerminalCancelV1,
		closedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.store.mu.Lock()
	delete(fixture.store.closures, closure.TurnBindingHMAC)
	fixture.store.mu.Unlock()

	if _, err := service.CloseTurn(
		context.Background(),
		fixture.securityContext,
		domaincachetelemetry.ProviderTurnTerminalCancelV1,
		closedAt.Add(time.Minute),
	); err == nil || !providerRetryForbiddenV1(err) {
		t.Fatalf("process-local closure cache replaced missing durable authority: %v", err)
	}
	fixture.store.mu.Lock()
	_, recreated := fixture.store.closures[closure.TurnBindingHMAC]
	fixture.store.mu.Unlock()
	if recreated {
		t.Fatal("cached closure silently recreated missing durable authority")
	}
}

func TestDurableServiceSeparatesProviderChannelsAndInvalidInputNeverCommits(t *testing.T) {
	fixture := newDurableServiceFixtureV1(t)
	service, err := NewDurableService(fixture.authority, fixture.store)
	if err != nil {
		t.Fatal(err)
	}
	invalid := fixture.registration(t, 0, 1, domaincachetelemetry.ProviderChannelPrimary, "2026-07-14T00:00:00Z")
	if _, err := service.BeginAttempt(context.Background(), invalid); err == nil || !providerRetryForbiddenV1(err) {
		t.Fatalf("invalid provider attempt did not fail non-retryably: %v", err)
	}
	if has, err := fixture.store.HasRecords(context.Background()); err != nil || has {
		t.Fatalf("invalid provider attempt committed authority: has=%v err=%v", has, err)
	}
	primary, err := service.BeginAttempt(context.Background(), fixture.registration(t, 1, 1, domaincachetelemetry.ProviderChannelPrimary, "2026-07-14T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SettleAttempt(context.Background(), cachetelemetryport.AttemptSettlementInputV1{
		Handle: primary, DispatchState: domaincachetelemetry.ProviderDispatchStateSent,
		Status: domaincachetelemetry.ProviderCallStatusSucceeded, Usage: completeProviderUsageV1(),
		SafeReasonCode: "provider_succeeded", SettledAt: durableServiceTimeV1(t, "2026-07-14T00:00:01Z"),
	}); err != nil {
		t.Fatal(err)
	}
	vision, err := service.BeginAttempt(context.Background(), fixture.registration(t, 2, 1, domaincachetelemetry.ProviderChannelAttachmentVision, "2026-07-14T00:00:02Z"))
	if err != nil {
		t.Fatal(err)
	}
	if vision.Intent.LogicalCallOrdinal != 2 || vision.Intent.ChannelOrdinal != 1 || primary.Intent.ChannelOrdinal != 1 {
		t.Fatalf("provider lane ordinals were mixed: primary=%#v vision=%#v", primary.Intent, vision.Intent)
	}
}

func TestProviderAttemptRejectsUsageSourceChildRunMismatch(t *testing.T) {
	for name, mutate := range map[string]func(*cachetelemetryport.AttemptRegistrationInputV1){
		"subagent_without_child": func(input *cachetelemetryport.AttemptRegistrationInputV1) {
			input.UsageSource = domaincachetelemetry.ProviderUsageSourceSubagent
			input.ChildRunID = nil
		},
		"turn_with_child": func(input *cachetelemetryport.AttemptRegistrationInputV1) {
			input.UsageSource = domaincachetelemetry.ProviderUsageSourceTurn
			input.ChildRunID = []byte("child-run")
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newDurableServiceFixtureV1(t)
			service, err := NewDurableService(fixture.authority, fixture.store)
			if err != nil {
				t.Fatal(err)
			}
			input := fixture.registration(t, 1, 1, domaincachetelemetry.ProviderChannelPrimary, "2026-07-14T00:00:00Z")
			mutate(&input)
			if _, err := service.BeginAttempt(context.Background(), input); err == nil || !providerRetryForbiddenV1(err) {
				t.Fatalf("mismatched provider namespace did not fail non-retryably: %v", err)
			}
			if has, err := fixture.store.HasRecords(context.Background()); err != nil || has {
				t.Fatalf("mismatched provider namespace committed authority: has=%v err=%v", has, err)
			}
		})
	}

	fixture := newDurableServiceFixtureV1(t)
	service, err := NewDurableService(fixture.authority, fixture.store)
	if err != nil {
		t.Fatal(err)
	}
	valid := fixture.registration(t, 1, 1, domaincachetelemetry.ProviderChannelPrimary, "2026-07-14T00:00:00Z")
	valid.UsageSource = domaincachetelemetry.ProviderUsageSourceSubagent
	valid.ChildRunID = []byte("child-run")
	if _, err := service.BeginAttempt(context.Background(), valid); err != nil {
		t.Fatalf("valid subagent provider namespace was rejected: %v", err)
	}
}

type durableServiceFixtureV1 struct {
	store           *memoryProviderTelemetryStoreV1
	authority       finalauthorityport.Authority
	securityContext domainsecurity.TurnSecurityContext
}

func newDurableServiceFixtureV1(t *testing.T) durableServiceFixtureV1 {
	t.Helper()
	store := newMemoryProviderTelemetryStoreV1()
	authority := newMemoryProviderAuthorityV1(t)
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-private", TurnID: "turn-private", WorkspaceRealPath: "/Users/sun/Projects/case-a",
		CaseID: "case-a", CaseBindingHash: domainsecurity.SHA256Hex([]byte("case-binding")),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("snapshot-private"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-manifest")), ContextEpoch: 4,
		IssuedAt: durableServiceTimeV1(t, "2026-07-13T23:59:00Z"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return durableServiceFixtureV1{store: store, authority: authority, securityContext: securityContext}
}

func (fixture durableServiceFixtureV1) reopenStore(t *testing.T) *memoryProviderTelemetryStoreV1 {
	t.Helper()
	return fixture.store
}

func (fixture durableServiceFixtureV1) registration(t *testing.T, logicalSequence uint64, physicalAttempt uint32, channel domaincachetelemetry.ProviderChannelV1, startedAt string) cachetelemetryport.AttemptRegistrationInputV1 {
	t.Helper()
	laneCallSequence := uint32(0)
	if channel != domaincachetelemetry.ProviderChannelPrimary {
		laneCallSequence = 1
	}
	return cachetelemetryport.AttemptRegistrationInputV1{
		SecurityContext: fixture.securityContext, UsageSource: domaincachetelemetry.ProviderUsageSourceTurn,
		ChildRunID: nil, Channel: channel, LogicalSequence: logicalSequence, OuterAttempt: 1,
		LaneCallSequence: laneCallSequence, PhysicalAttempt: physicalAttempt,
		ProviderFamily: domaincachetelemetry.ProviderFamilyDeepSeek,
		EndpointFormat: domaincachetelemetry.EndpointFormatChatCompletions,
		Model:          []byte("deepseek-chat"), Endpoint: []byte("https://api.example.test/v1/chat?token=secret"),
		WireBody:        []byte(`{"prompt":"合成样例事件甲","card":"6222021234567890123"}`),
		WireHeaders:     []byte(`{"Authorization":"Bearer sk-provider-secret"}`),
		CredentialScope: []byte("provider-a\x00sk-provider-secret"),
		ProviderConfig:  []byte(`{"baseUrl":"https://api.example.test/v1","model":"deepseek-chat"}`),
		StartedAt:       durableServiceTimeV1(t, startedAt),
	}
}

func providerTelemetryWitnessedBoundaryContextV1(t *testing.T) domainsecurity.TurnSecurityContext {
	t.Helper()
	const (
		threadID  = "thread-cache-boundary"
		turnID    = "turn-cache-boundary"
		workspace = "/workspace/cache-boundary"
	)
	policyDigest := domainsecurity.SHA256Hex([]byte("cache-boundary-risk-policy:\x00" + threadID))
	publication, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: policyDigest, RiskClass: domainsecurity.RiskClassCase,
		Disposition:              domainsecurity.PublicationDispositionCaseBoundaryOnly,
		CaseBindingState:         domainsecurity.CaseBindingStateMissing,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("cache-boundary-binding:\x00" + threadID)),
		BlockerCode:              domainsecurity.PublicationBlockerCaseBindingMissing,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := securitycontexttest.WitnessedRiskBinding(
		threadID, workspace, domainsecurity.RiskClassCase, policyDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: domainsecurity.UnboundCaseID, CaseBindingHash: domainsecurity.UnboundCaseBindingHash(workspace),
		DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID, SourceManifestHash: domainsecurity.EmptySourceManifestHash,
		ContextEpoch: 1, IssuedAt: durableServiceTimeV1(t, "2026-07-14T00:00:00Z"),
		PublicationPolicy: publication, RiskAuthorityBinding: binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func completeProviderUsageV1() domaincachetelemetry.ProviderUsageV1 {
	return domaincachetelemetry.ProviderUsageV1{
		InputTokens:     domaincachetelemetry.TokenCountV1{Known: true, Value: 100},
		OutputTokens:    domaincachetelemetry.TokenCountV1{Known: true, Value: 20},
		CacheHitTokens:  domaincachetelemetry.TokenCountV1{Known: true, Value: 90},
		CacheMissTokens: domaincachetelemetry.TokenCountV1{Known: true, Value: 10},
		ReasoningTokens: domaincachetelemetry.TokenCountV1{Known: false},
	}
}

func durableServiceTimeV1(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

type memoryProviderTelemetryStoreV1 struct {
	mu          sync.Mutex
	intents     map[string]domaincachetelemetry.ProviderAttemptIntentV1
	settlements map[string]domaincachetelemetry.ProviderAttemptSettlementV1
	closures    map[string]domaincachetelemetry.ProviderTurnClosureV1
}

type countingProviderTelemetryInventoryStoreV1 struct {
	*memoryProviderTelemetryStoreV1
	inventoryVisits  int
	intentReads      int
	settlementReads  int
	closureReads     int
	intentVisits     int
	settlementVisits int
	closureVisits    int
}

func (store *countingProviderTelemetryInventoryStoreV1) ReadIntent(
	ctx context.Context,
	id string,
) (domaincachetelemetry.ProviderAttemptIntentV1, error) {
	store.intentReads++
	return store.memoryProviderTelemetryStoreV1.ReadIntent(ctx, id)
}

func (store *countingProviderTelemetryInventoryStoreV1) ReadSettlement(
	ctx context.Context,
	id string,
) (domaincachetelemetry.ProviderAttemptSettlementV1, error) {
	store.settlementReads++
	return store.memoryProviderTelemetryStoreV1.ReadSettlement(ctx, id)
}

func (store *countingProviderTelemetryInventoryStoreV1) ReadClosure(
	ctx context.Context,
	turnBinding string,
) (domaincachetelemetry.ProviderTurnClosureV1, error) {
	store.closureReads++
	return store.memoryProviderTelemetryStoreV1.ReadClosure(ctx, turnBinding)
}

func (store *countingProviderTelemetryInventoryStoreV1) VisitInventory(
	ctx context.Context,
	visitIntent func(domaincachetelemetry.ProviderAttemptIntentV1) error,
	visitSettlement func(domaincachetelemetry.ProviderAttemptSettlementV1) error,
	visitClosure func(domaincachetelemetry.ProviderTurnClosureV1) error,
) error {
	store.inventoryVisits++
	return store.memoryProviderTelemetryStoreV1.VisitInventory(ctx, visitIntent, visitSettlement, visitClosure)
}

func (store *countingProviderTelemetryInventoryStoreV1) VisitIntents(
	ctx context.Context,
	visit func(domaincachetelemetry.ProviderAttemptIntentV1) error,
) error {
	store.intentVisits++
	return store.memoryProviderTelemetryStoreV1.VisitIntents(ctx, visit)
}

func (store *countingProviderTelemetryInventoryStoreV1) VisitSettlements(
	ctx context.Context,
	visit func(domaincachetelemetry.ProviderAttemptSettlementV1) error,
) error {
	store.settlementVisits++
	return store.memoryProviderTelemetryStoreV1.VisitSettlements(ctx, visit)
}

func (store *countingProviderTelemetryInventoryStoreV1) VisitClosures(
	ctx context.Context,
	visit func(domaincachetelemetry.ProviderTurnClosureV1) error,
) error {
	store.closureVisits++
	return store.memoryProviderTelemetryStoreV1.VisitClosures(ctx, visit)
}

type memoryProviderTelemetrySnapshotV1 struct {
	Intents     []domaincachetelemetry.ProviderAttemptIntentV1     `json:"intents"`
	Settlements []domaincachetelemetry.ProviderAttemptSettlementV1 `json:"settlements"`
	Closures    []domaincachetelemetry.ProviderTurnClosureV1       `json:"closures"`
}

func newMemoryProviderTelemetryStoreV1() *memoryProviderTelemetryStoreV1 {
	return &memoryProviderTelemetryStoreV1{
		intents:     make(map[string]domaincachetelemetry.ProviderAttemptIntentV1),
		settlements: make(map[string]domaincachetelemetry.ProviderAttemptSettlementV1),
		closures:    make(map[string]domaincachetelemetry.ProviderTurnClosureV1),
	}
}

func (store *memoryProviderTelemetryStoreV1) PutIntentIfAbsent(_ context.Context, intent domaincachetelemetry.ProviderAttemptIntentV1) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, closed := store.closures[intent.TurnBindingHMAC]; closed {
		return errors.New("provider telemetry turn is closed")
	}
	if existing, exists := store.intents[intent.IntentID]; exists {
		if existing == intent {
			return nil
		}
		return errors.New("provider intent conflicts with existing authority")
	}
	store.intents[intent.IntentID] = intent
	return nil
}

func (store *memoryProviderTelemetryStoreV1) CommitIntentIfAbsent(
	ctx context.Context,
	intent domaincachetelemetry.ProviderAttemptIntentV1,
) (domaincachetelemetry.ProviderAttemptIntentV1, error) {
	if err := store.PutIntentIfAbsent(ctx, intent); err != nil {
		return domaincachetelemetry.ProviderAttemptIntentV1{}, err
	}
	return intent, nil
}

func (store *memoryProviderTelemetryStoreV1) ReadIntent(_ context.Context, id string) (domaincachetelemetry.ProviderAttemptIntentV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	intent, exists := store.intents[id]
	if !exists {
		return domaincachetelemetry.ProviderAttemptIntentV1{}, cachetelemetrystoreport.ErrNotFound
	}
	return intent, nil
}

func (store *memoryProviderTelemetryStoreV1) VisitIntents(_ context.Context, visit func(domaincachetelemetry.ProviderAttemptIntentV1) error) error {
	store.mu.Lock()
	values := make([]domaincachetelemetry.ProviderAttemptIntentV1, 0, len(store.intents))
	for _, value := range store.intents {
		values = append(values, value)
	}
	store.mu.Unlock()
	sort.Slice(values, func(i, j int) bool { return values[i].IntentID < values[j].IntentID })
	for _, value := range values {
		if err := visit(value); err != nil {
			return err
		}
	}
	return nil
}

func (store *memoryProviderTelemetryStoreV1) VisitInventory(
	_ context.Context,
	visitIntent func(domaincachetelemetry.ProviderAttemptIntentV1) error,
	visitSettlement func(domaincachetelemetry.ProviderAttemptSettlementV1) error,
	visitClosure func(domaincachetelemetry.ProviderTurnClosureV1) error,
) error {
	store.mu.Lock()
	intents := make([]domaincachetelemetry.ProviderAttemptIntentV1, 0, len(store.intents))
	for _, value := range store.intents {
		intents = append(intents, value)
	}
	settlements := make([]domaincachetelemetry.ProviderAttemptSettlementV1, 0, len(store.settlements))
	for _, value := range store.settlements {
		settlements = append(settlements, value)
	}
	closures := make([]domaincachetelemetry.ProviderTurnClosureV1, 0, len(store.closures))
	for _, value := range store.closures {
		closures = append(closures, value)
	}
	store.mu.Unlock()
	sort.Slice(intents, func(i, j int) bool { return intents[i].IntentID < intents[j].IntentID })
	sort.Slice(settlements, func(i, j int) bool { return settlements[i].IntentID < settlements[j].IntentID })
	sort.Slice(closures, func(i, j int) bool { return closures[i].TurnBindingHMAC < closures[j].TurnBindingHMAC })
	for _, value := range intents {
		if err := visitIntent(value); err != nil {
			return err
		}
	}
	for _, value := range settlements {
		if err := visitSettlement(value); err != nil {
			return err
		}
	}
	for _, value := range closures {
		if err := visitClosure(value); err != nil {
			return err
		}
	}
	return nil
}

func (store *memoryProviderTelemetryStoreV1) PutSettlementIfAbsent(_ context.Context, settlement domaincachetelemetry.ProviderAttemptSettlementV1) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, exists := store.intents[settlement.IntentID]; !exists {
		return errors.New("provider settlement has no intent")
	}
	if _, closed := store.closures[settlement.TurnBindingHMAC]; closed {
		return errors.New("provider telemetry turn is closed")
	}
	if existing, exists := store.settlements[settlement.IntentID]; exists {
		if existing == settlement {
			return nil
		}
		return errors.New("provider settlement conflicts with existing authority")
	}
	store.settlements[settlement.IntentID] = settlement
	return nil
}

func (store *memoryProviderTelemetryStoreV1) CommitSettlementIfAbsent(
	ctx context.Context,
	settlement domaincachetelemetry.ProviderAttemptSettlementV1,
) (domaincachetelemetry.ProviderAttemptSettlementV1, error) {
	if err := store.PutSettlementIfAbsent(ctx, settlement); err != nil {
		return domaincachetelemetry.ProviderAttemptSettlementV1{}, err
	}
	return settlement, nil
}

func (store *memoryProviderTelemetryStoreV1) ReadSettlement(_ context.Context, intentID string) (domaincachetelemetry.ProviderAttemptSettlementV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	settlement, exists := store.settlements[intentID]
	if !exists {
		return domaincachetelemetry.ProviderAttemptSettlementV1{}, cachetelemetrystoreport.ErrNotFound
	}
	return settlement, nil
}

func (store *memoryProviderTelemetryStoreV1) VisitSettlements(_ context.Context, visit func(domaincachetelemetry.ProviderAttemptSettlementV1) error) error {
	store.mu.Lock()
	values := make([]domaincachetelemetry.ProviderAttemptSettlementV1, 0, len(store.settlements))
	for _, value := range store.settlements {
		values = append(values, value)
	}
	store.mu.Unlock()
	sort.Slice(values, func(i, j int) bool { return values[i].IntentID < values[j].IntentID })
	for _, value := range values {
		if err := visit(value); err != nil {
			return err
		}
	}
	return nil
}

func (store *memoryProviderTelemetryStoreV1) PutClosureIfAbsent(ctx context.Context, closure domaincachetelemetry.ProviderTurnClosureV1) error {
	persisted, err := store.CommitClosureIfAbsent(ctx, closure)
	if err != nil {
		return err
	}
	if persisted != closure {
		return errors.New("provider turn closure conflicts with existing authority")
	}
	return nil
}

func (store *memoryProviderTelemetryStoreV1) CommitClosureIfAbsent(_ context.Context, closure domaincachetelemetry.ProviderTurnClosureV1) (domaincachetelemetry.ProviderTurnClosureV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if existing, exists := store.closures[closure.TurnBindingHMAC]; exists {
		return existing, nil
	}
	store.closures[closure.TurnBindingHMAC] = closure
	return closure, nil
}

func (store *memoryProviderTelemetryStoreV1) ReadClosure(_ context.Context, turnBinding string) (domaincachetelemetry.ProviderTurnClosureV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	closure, exists := store.closures[turnBinding]
	if !exists {
		return domaincachetelemetry.ProviderTurnClosureV1{}, cachetelemetrystoreport.ErrNotFound
	}
	return closure, nil
}

func (store *memoryProviderTelemetryStoreV1) VisitClosures(_ context.Context, visit func(domaincachetelemetry.ProviderTurnClosureV1) error) error {
	store.mu.Lock()
	values := make([]domaincachetelemetry.ProviderTurnClosureV1, 0, len(store.closures))
	for _, value := range store.closures {
		values = append(values, value)
	}
	store.mu.Unlock()
	sort.Slice(values, func(i, j int) bool { return values[i].TurnBindingHMAC < values[j].TurnBindingHMAC })
	for _, value := range values {
		if err := visit(value); err != nil {
			return err
		}
	}
	return nil
}

func (store *memoryProviderTelemetryStoreV1) HasRecords(context.Context) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return len(store.intents) > 0 || len(store.settlements) > 0 || len(store.closures) > 0, nil
}

func (store *memoryProviderTelemetryStoreV1) snapshot() memoryProviderTelemetrySnapshotV1 {
	snapshot := memoryProviderTelemetrySnapshotV1{}
	_ = store.VisitIntents(context.Background(), func(value domaincachetelemetry.ProviderAttemptIntentV1) error {
		snapshot.Intents = append(snapshot.Intents, value)
		return nil
	})
	_ = store.VisitSettlements(context.Background(), func(value domaincachetelemetry.ProviderAttemptSettlementV1) error {
		snapshot.Settlements = append(snapshot.Settlements, value)
		return nil
	})
	_ = store.VisitClosures(context.Background(), func(value domaincachetelemetry.ProviderTurnClosureV1) error {
		snapshot.Closures = append(snapshot.Closures, value)
		return nil
	})
	return snapshot
}

type memoryProviderAuthorityV1 struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	keyID      string
}

func newMemoryProviderAuthorityV1(t *testing.T) *memoryProviderAuthorityV1 {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(cryptorand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &memoryProviderAuthorityV1{
		privateKey: privateKey,
		publicKey:  publicKey,
		keyID:      domainsecurity.SHA256Hex(publicKey),
	}
}

func (authority *memoryProviderAuthorityV1) KeyID() string { return authority.keyID }

func (authority *memoryProviderAuthorityV1) PublicKey() []byte {
	return append([]byte(nil), authority.publicKey...)
}

func (authority *memoryProviderAuthorityV1) Sign(ctx context.Context, message []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ed25519.Sign(authority.privateKey, message), nil
}

func (authority *memoryProviderAuthorityV1) VerifyTrusted(ctx context.Context, keyID string, publicKey, message, signature []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if keyID != authority.keyID || !bytes.Equal(publicKey, authority.publicKey) || !ed25519.Verify(authority.publicKey, message, signature) {
		return errors.New("provider telemetry signature is not trusted")
	}
	return nil
}

type observingProviderAuthorityV1 struct {
	finalauthorityport.Authority
	mu       sync.Mutex
	messages [][]byte
}

func (authority *observingProviderAuthorityV1) Sign(ctx context.Context, message []byte) ([]byte, error) {
	authority.mu.Lock()
	authority.messages = append(authority.messages, append([]byte(nil), message...))
	authority.mu.Unlock()
	return authority.Authority.Sign(ctx, message)
}

func (authority *observingProviderAuthorityV1) Messages() [][]byte {
	authority.mu.Lock()
	defer authority.mu.Unlock()
	result := make([][]byte, len(authority.messages))
	for index := range authority.messages {
		result[index] = append([]byte(nil), authority.messages[index]...)
	}
	return result
}

func providerRetryForbiddenV1(err error) bool {
	var marker interface{ ProviderRetryForbidden() bool }
	return errors.As(err, &marker) && marker.ProviderRetryForbidden()
}

func assertProviderTelemetrySecretsAbsentV1(t *testing.T, body []byte) {
	t.Helper()
	for _, forbidden := range [][]byte{
		[]byte("合成样例事件甲"), []byte("6222021234567890123"), []byte("sk-provider-secret"),
		[]byte("https://api.example.test/v1/chat?token=secret"), []byte("/Users/sun/Projects/case-a"),
		[]byte(`"caseId":"case-a"`), []byte(`"reasoning_content"`),
	} {
		if bytes.Contains(body, forbidden) {
			t.Fatalf("provider telemetry authority leaked %q in %s", forbidden, body)
		}
	}
}
