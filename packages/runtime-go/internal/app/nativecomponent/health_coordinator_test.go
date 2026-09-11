package nativecomponent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	effectgateapp "analytix.local/runtime-go/internal/app/effectgate"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	appthread "analytix.local/runtime-go/internal/app/thread"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	appturn "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	"analytix.local/runtime-go/internal/contracts"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	nativecomponentport "analytix.local/runtime-go/internal/ports/nativecomponent"
)

func TestNativeHealthPrivateRecordsSettleGrantWithoutEvents(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	outcome, err := fixture.coordinator.ProbeDataEngine(
		context.Background(), fixture.securityContext,
	)
	if err != nil || outcome != (HealthOutcome{SchemaVersion: 1, Status: HealthStatusReady, Code: HealthCodeReady}) {
		t.Fatalf("health outcome=%#v err=%v", outcome, err)
	}
	if fixture.runner.calls != 1 {
		t.Fatalf("native runner calls = %d", fixture.runner.calls)
	}
	thread, err := fixture.store.GetThread(fixture.securityContext.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, ok := appmodel.TurnByID(thread, fixture.securityContext.TurnID)
	if !ok {
		t.Fatal("health turn is unavailable")
	}
	items := healthTurnItems(turn)
	if len(items) != 2 {
		t.Fatalf("health records = %#v", items)
	}
	callItem, _ := items[0].(map[string]any)
	resultItem, _ := items[1].(map[string]any)
	grant, err := domainsecurity.ParseExecutionGrant(callItem["executionGrant"])
	if err != nil {
		t.Fatal(err)
	}
	registry, err := executiongrantapp.RegistryFromThread(
		fixture.securityContext.ThreadID, thread, fixture.securityContext.TurnID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := domainsecurity.VerifyExecutionGrantMembership(
		registry, fixture.securityContext.ThreadID, fixture.securityContext.TurnID, grant, domainsecurity.GrantRegistrySettled,
	); err != nil {
		t.Fatalf("health grant was not settled: %v", err)
	}
	projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(resultItem["output"])
	if err != nil || projection.Code != "tool_completed" || projection.FactAnswerAllowed || projection.EvidenceAuthority ||
		callItem["status"] != "completed" || resultItem["hostEvidenceSettlement"] != nil {
		t.Fatalf("health public records are unsafe: call=%#v result=%#v projection=%#v err=%v", callItem, resultItem, projection, err)
	}
	encoded, _ := json.Marshal(items)
	for _, forbidden := range []string{nativeReadyResult().RegistryDigest, "stdout", "stderr", "executable", "pid"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("private native value %q persisted: %s", forbidden, encoded)
		}
	}
}

func TestNativeHealthAcceptsCanonicalDurableCallWithoutInternalHostKind(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	fixture.store.dropCallToolKind = true
	outcome, err := fixture.coordinator.ProbeDataEngine(context.Background(), fixture.securityContext)
	if err != nil || outcome != (HealthOutcome{SchemaVersion: 1, Status: HealthStatusReady, Code: HealthCodeReady}) {
		t.Fatalf("canonical durable health outcome=%#v err=%v", outcome, err)
	}
	thread, err := fixture.store.GetThread(fixture.securityContext.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, ok := appmodel.TurnByID(thread, fixture.securityContext.TurnID)
	if !ok {
		t.Fatal("canonical durable health turn is unavailable")
	}
	items := healthTurnItems(turn)
	callItem, _ := items[0].(map[string]any)
	if _, present := callItem["toolKind"]; present {
		t.Fatalf("canonical durable health call retained internal host kind: %#v", callItem)
	}
}

func TestNativeHealthNeverEntersCasePublicHistoryProviderHistoryOrSidecar(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	if _, err := fixture.coordinator.ProbeDataEngine(
		context.Background(), fixture.securityContext,
	); err != nil {
		t.Fatal(err)
	}
	thread, err := fixture.store.GetThread(fixture.securityContext.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	public, err := appthread.ProjectPublicThread(thread)
	if err != nil {
		t.Fatal(err)
	}
	turn, ok := appmodel.TurnByID(public, fixture.securityContext.TurnID)
	if !ok || len(healthTurnItems(turn)) != 0 {
		t.Fatalf("native health records reached case public history: %#v", public)
	}
	if messages := appmodel.ProviderHistoryMessagesFromThread(thread); len(messages) != 0 {
		t.Fatalf("native health records reached provider history: %#v", messages)
	}
	if sidecar, _, err := appthread.MissingMessageSidecarItems(thread, nil); err != nil || len(sidecar) != 0 {
		t.Fatalf("native health records reached messages sidecar: items=%#v err=%v", sidecar, err)
	}
}

func TestNativeHealthDuplicateCallRejectedWithoutSecondSpawn(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	if _, err := fixture.coordinator.ProbeDataEngine(
		context.Background(), fixture.securityContext,
	); err != nil {
		t.Fatal(err)
	}
	outcome, err := fixture.coordinator.ProbeDataEngine(
		context.Background(), fixture.securityContext,
	)
	if !errors.Is(err, ErrHealthDuplicate) || outcome.Status != HealthStatusUnavailable || fixture.runner.calls != 1 {
		t.Fatalf("duplicate outcome=%#v err=%v runnerCalls=%d", outcome, err, fixture.runner.calls)
	}
	thread, _ := fixture.store.GetThread(fixture.securityContext.ThreadID)
	turn, _ := appmodel.TurnByID(thread, fixture.securityContext.TurnID)
	if len(healthTurnItems(turn)) != 2 {
		t.Fatalf("duplicate health call changed durable records: %#v", turn["items"])
	}
}

func TestNativeHealthEnsureExactSettledReadyIsIdempotent(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	first, firstErr := fixture.coordinator.ensureDataEngineReady(context.Background(), fixture.securityContext)
	second, secondErr := fixture.coordinator.ensureDataEngineReady(context.Background(), fixture.securityContext)
	want := HealthOutcome{SchemaVersion: HealthOutcomeSchemaVersion, Status: HealthStatusReady, Code: HealthCodeReady}
	if firstErr != nil || secondErr != nil || first != want || second != want || fixture.runner.calls != 1 {
		t.Fatalf("idempotent health first=%#v firstErr=%v second=%#v secondErr=%v runnerCalls=%d", first, firstErr, second, secondErr, fixture.runner.calls)
	}
	thread, _ := fixture.store.GetThread(fixture.securityContext.ThreadID)
	turn, _ := appmodel.TurnByID(thread, fixture.securityContext.TurnID)
	if len(healthTurnItems(turn)) != 2 {
		t.Fatalf("idempotent readback changed durable records: %#v", turn["items"])
	}
}

func TestNativeHealthEnsureSettledNonReadyNeverRetries(t *testing.T) {
	t.Run("unavailable", func(t *testing.T) {
		fixture := newNativeHealthCoordinatorFixture(t)
		fixture.coordinator.dependencies.Service = nil
		if _, err := fixture.coordinator.ensureDataEngineReady(context.Background(), fixture.securityContext); !errors.Is(err, ErrHealthUnavailableSettled) {
			t.Fatalf("first unavailable health error = %v", err)
		}
		outcome, err := fixture.coordinator.ensureDataEngineReady(context.Background(), fixture.securityContext)
		if !errors.Is(err, ErrHealthUnavailableSettled) || outcome.Status != HealthStatusUnavailable || fixture.runner.calls != 0 {
			t.Fatalf("unavailable readback outcome=%#v err=%v runnerCalls=%d", outcome, err, fixture.runner.calls)
		}
	})

	t.Run("failed", func(t *testing.T) {
		fixture := newNativeHealthCoordinatorFixture(t)
		fixture.runner.err = errors.New("private native failure")
		fixture.runner.result = domainnative.Result{}
		if _, err := fixture.coordinator.ensureDataEngineReady(context.Background(), fixture.securityContext); !errors.Is(err, ErrHealthExecutionUnsafe) {
			t.Fatalf("first failed health error = %v", err)
		}
		outcome, err := fixture.coordinator.ensureDataEngineReady(context.Background(), fixture.securityContext)
		if !errors.Is(err, ErrHealthExecutionUnsafe) || outcome.Status != HealthStatusUnavailable || fixture.runner.calls != 1 {
			t.Fatalf("failed readback outcome=%#v err=%v runnerCalls=%d", outcome, err, fixture.runner.calls)
		}
	})
}

func TestNativeHealthEnsureRejectsActiveTornTamperedAndCrossContextRecords(t *testing.T) {
	t.Run("active", func(t *testing.T) {
		fixture := newNativeHealthCoordinatorFixture(t)
		persistActiveNativeHealthCall(t, fixture)
		outcome, err := fixture.coordinator.ensureDataEngineReady(context.Background(), fixture.securityContext)
		if !errors.Is(err, ErrHealthSettlement) || outcome.Status != HealthStatusUnavailable || fixture.runner.calls != 0 {
			t.Fatalf("active readback outcome=%#v err=%v runnerCalls=%d", outcome, err, fixture.runner.calls)
		}
	})

	t.Run("torn", func(t *testing.T) {
		fixture := newNativeHealthCoordinatorFixture(t)
		fixture.store.dropResultWrites = true
		if _, err := fixture.coordinator.ensureDataEngineReady(context.Background(), fixture.securityContext); !errors.Is(err, ErrHealthSettlement) {
			t.Fatalf("first torn health error = %v", err)
		}
		fixture.store.dropResultWrites = false
		outcome, err := fixture.coordinator.ensureDataEngineReady(context.Background(), fixture.securityContext)
		if !errors.Is(err, ErrHealthSettlement) || outcome.Status != HealthStatusUnavailable || fixture.runner.calls != 1 {
			t.Fatalf("torn readback outcome=%#v err=%v runnerCalls=%d", outcome, err, fixture.runner.calls)
		}
	})

	t.Run("tampered", func(t *testing.T) {
		fixture := newNativeHealthCoordinatorFixture(t)
		if _, err := fixture.coordinator.ensureDataEngineReady(context.Background(), fixture.securityContext); err != nil {
			t.Fatal(err)
		}
		fixture.store.mu.Lock()
		turn := fixture.store.thread["turns"].([]any)[0].(map[string]any)
		turn["items"].([]any)[1].(map[string]any)["unexpected"] = "tampered"
		fixture.store.mu.Unlock()
		outcome, err := fixture.coordinator.ensureDataEngineReady(context.Background(), fixture.securityContext)
		if !errors.Is(err, ErrHealthSettlement) || outcome.Status != HealthStatusUnavailable || fixture.runner.calls != 1 {
			t.Fatalf("tampered readback outcome=%#v err=%v runnerCalls=%d", outcome, err, fixture.runner.calls)
		}
	})

	t.Run("cross_context", func(t *testing.T) {
		fixture := newNativeHealthCoordinatorFixture(t)
		if _, err := fixture.coordinator.ensureDataEngineReady(context.Background(), fixture.securityContext); err != nil {
			t.Fatal(err)
		}
		next := nativeAdmissionContext(
			t, fixture.securityContext.ThreadID, fixture.securityContext.TurnID,
			time.Date(2026, 7, 16, 15, 0, 1, 0, time.UTC),
		)
		nextRecord := turnsecurityapp.PublicRecord(next)
		fixture.store.mu.Lock()
		fixture.store.thread["securityState"] = nextRecord
		fixture.store.thread["turns"].([]any)[0].(map[string]any)["securityContext"] = nextRecord
		fixture.store.mu.Unlock()
		outcome, err := fixture.coordinator.ensureDataEngineReady(context.Background(), next)
		if err == nil || outcome.Status != HealthStatusUnavailable || fixture.runner.calls != 1 {
			t.Fatalf("cross-context readback outcome=%#v err=%v runnerCalls=%d", outcome, err, fixture.runner.calls)
		}
	})
}

func TestNativeHealthFailureSettlesWithoutPersistingRawError(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	fixture.runner.err = errors.New("PRIVATE_NATIVE_STDERR_SENTINEL")
	fixture.runner.result = domainnative.Result{}
	outcome, err := fixture.coordinator.ProbeDataEngine(
		context.Background(), fixture.securityContext,
	)
	if !errors.Is(err, ErrHealthExecutionUnsafe) || errors.Is(err, ErrHealthUnavailableSettled) ||
		outcome.Status != HealthStatusUnavailable || fixture.runner.calls != 1 {
		t.Fatalf("failure outcome=%#v err=%v runnerCalls=%d", outcome, err, fixture.runner.calls)
	}
	thread, _ := fixture.store.GetThread(fixture.securityContext.ThreadID)
	turn, _ := appmodel.TurnByID(thread, fixture.securityContext.TurnID)
	items := healthTurnItems(turn)
	if len(items) != 2 {
		t.Fatalf("failure records = %#v", items)
	}
	resultItem, _ := items[1].(map[string]any)
	projection, parseErr := domaintoolresult.ParsePublicToolResultProjectionV1(resultItem["output"])
	if parseErr != nil || projection.Code != "tool_failed" || projection.FactAnswerAllowed || projection.EvidenceAuthority {
		t.Fatalf("failure projection=%#v err=%v", projection, parseErr)
	}
	registry, registryErr := executiongrantapp.RegistryFromThread(
		fixture.securityContext.ThreadID, thread, fixture.securityContext.TurnID,
	)
	grant, grantErr := domainsecurity.ParseExecutionGrant(items[0].(map[string]any)["executionGrant"])
	if registryErr != nil || grantErr != nil || domainsecurity.VerifyExecutionGrantMembership(
		registry, fixture.securityContext.ThreadID, fixture.securityContext.TurnID, grant, domainsecurity.GrantRegistrySettled,
	) != nil {
		t.Fatalf("failed health grant remained open: registryErr=%v grantErr=%v registry=%#v", registryErr, grantErr, registry)
	}
	if encoded, _ := json.Marshal(items); strings.Contains(string(encoded), "PRIVATE_NATIVE_STDERR_SENTINEL") {
		t.Fatalf("raw native failure persisted: %s", encoded)
	}
}

func TestNativeHealthCleanCapabilityUnavailableIsTheOnlySettledFailureThatAllowsContinuation(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	fixture.runner.err = nativecomponentport.ErrUnavailable
	fixture.runner.result = domainnative.Result{}
	outcome, err := fixture.coordinator.ProbeDataEngine(context.Background(), fixture.securityContext)
	if err != ErrHealthUnavailableSettled || outcome.Status != HealthStatusUnavailable || fixture.runner.calls != 1 {
		t.Fatalf("clean unavailable outcome=%#v err=%v runnerCalls=%d", outcome, err, fixture.runner.calls)
	}
}

func TestNativeHealthUnsafeRunnerFailureClassesNeverBecomeSettledUnavailable(t *testing.T) {
	for _, runErr := range []error{
		nativecomponentport.ErrRequestInvalid,
		nativecomponentport.ErrRegistryInvalid,
		nativecomponentport.ErrProtocolInvalid,
		nativecomponentport.ErrTerminationUnconfirmed,
		errors.Join(nativecomponentport.ErrUnavailable, nativecomponentport.ErrTerminationUnconfirmed),
		errors.Join(nativecomponentport.ErrUnavailable, errors.New("mixed unknown runner failure")),
		errors.New("unknown native runner failure"),
	} {
		t.Run(runErr.Error(), func(t *testing.T) {
			fixture := newNativeHealthCoordinatorFixture(t)
			fixture.runner.err = runErr
			fixture.runner.result = domainnative.Result{}
			outcome, err := fixture.coordinator.ProbeDataEngine(context.Background(), fixture.securityContext)
			if !errors.Is(err, ErrHealthExecutionUnsafe) || errors.Is(err, ErrHealthUnavailableSettled) ||
				outcome.Status != HealthStatusUnavailable || fixture.runner.calls != 1 {
				t.Fatalf("unsafe outcome=%#v err=%v runnerCalls=%d", outcome, err, fixture.runner.calls)
			}
		})
	}
}

func TestNativeHealthConcurrentEnsureNeverAdmitsUnfinishedProbe(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	runner := &blockingHealthRunner{entered: make(chan struct{}), release: make(chan struct{})}
	fixture.coordinator.dependencies.Service.dependencies.Runner = runner
	firstDone := make(chan error, 1)
	go func() {
		_, err := fixture.coordinator.ensureDataEngineReady(context.Background(), fixture.securityContext)
		firstDone <- err
	}()
	select {
	case <-runner.entered:
	case <-time.After(time.Second):
		t.Fatal("first native health execution did not start")
	}
	secondOutcome, secondErr := fixture.coordinator.ensureDataEngineReady(context.Background(), fixture.securityContext)
	if !errors.Is(secondErr, ErrHealthDuplicate) || secondOutcome.Status != HealthStatusUnavailable {
		t.Fatalf("concurrent duplicate outcome=%#v err=%v", secondOutcome, secondErr)
	}
	close(runner.release)
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("first health execution failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("first native health execution did not finish")
	}
	if calls := runner.calls.Load(); calls != 1 {
		t.Fatalf("concurrent native runner calls = %d", calls)
	}
	thirdOutcome, thirdErr := fixture.coordinator.ensureDataEngineReady(context.Background(), fixture.securityContext)
	if thirdErr != nil || thirdOutcome.Status != HealthStatusReady || runner.calls.Load() != 1 {
		t.Fatalf("settled readback outcome=%#v err=%v runnerCalls=%d", thirdOutcome, thirdErr, runner.calls.Load())
	}
}

func TestNativeHealthReadyRequiresDurableSettlementReadback(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	fixture.store.dropResultWrites = true
	outcome, err := fixture.coordinator.ProbeDataEngine(
		context.Background(), fixture.securityContext,
	)
	if !errors.Is(err, ErrHealthSettlement) || outcome.Status != HealthStatusUnavailable || fixture.runner.calls != 1 {
		t.Fatalf("missing readback outcome=%#v err=%v runnerCalls=%d", outcome, err, fixture.runner.calls)
	}
	thread, _ := fixture.store.GetThread(fixture.securityContext.ThreadID)
	turn, _ := appmodel.TurnByID(thread, fixture.securityContext.TurnID)
	if len(healthTurnItems(turn)) != 1 {
		t.Fatalf("dropped result unexpectedly became durable: %#v", turn["items"])
	}
}

func TestNativeHealthUnavailableModePersistsSettledMetadataOnlyResult(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	fixture.coordinator.dependencies.Service = nil
	outcome, err := fixture.coordinator.ProbeDataEngine(context.Background(), fixture.securityContext)
	if !errors.Is(err, ErrHealthUnavailableSettled) ||
		outcome != (HealthOutcome{SchemaVersion: 1, Status: HealthStatusUnavailable, Code: HealthCodeUnavailable}) {
		t.Fatalf("unavailable outcome=%#v err=%v", outcome, err)
	}
	if fixture.runner.calls != 0 {
		t.Fatalf("unavailable mode invoked native runner %d times", fixture.runner.calls)
	}
	thread, readErr := fixture.store.GetThread(fixture.securityContext.ThreadID)
	turn, ok := appmodel.TurnByID(thread, fixture.securityContext.TurnID)
	if readErr != nil || !ok {
		t.Fatalf("read settled unavailable turn: ok=%t err=%v", ok, readErr)
	}
	items := healthTurnItems(turn)
	if len(items) != 2 {
		t.Fatalf("unavailable health records = %#v", items)
	}
	callItem, _ := items[0].(map[string]any)
	resultItem, _ := items[1].(map[string]any)
	grant, grantErr := domainsecurity.ParseExecutionGrant(callItem["executionGrant"])
	registry, registryErr := executiongrantapp.RegistryFromThread(
		fixture.securityContext.ThreadID, thread, fixture.securityContext.TurnID,
	)
	if grantErr != nil || registryErr != nil || domainsecurity.VerifyExecutionGrantMembership(
		registry,
		fixture.securityContext.ThreadID,
		fixture.securityContext.TurnID,
		grant,
		domainsecurity.GrantRegistrySettled,
	) != nil {
		t.Fatalf("unavailable grant was not durably settled: grantErr=%v registryErr=%v", grantErr, registryErr)
	}
	projection, projectionErr := domaintoolresult.ParsePublicToolResultProjectionV1(resultItem["output"])
	if projectionErr != nil ||
		projection.ProjectionKind != domaintoolresult.ProjectionHostStatus ||
		projection.Disclosure != domaintoolresult.MetadataOnlyDisclosure ||
		projection.MessageKey != "tool_blocked" ||
		projection.Status != "blocked" ||
		projection.Code != "tool_source_unavailable" ||
		!projection.PrivatePayloadWithheld ||
		projection.FactAnswerAllowed ||
		projection.EvidenceAuthority ||
		resultItem["isError"] != true ||
		resultItem["hostEvidenceSettlement"] != nil {
		t.Fatalf("unsafe unavailable settlement: result=%#v projection=%#v err=%v", resultItem, projection, projectionErr)
	}
}

func TestNativeHealthUnavailableRequiresDurableSettlementReadback(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	fixture.coordinator.dependencies.Service = nil
	fixture.store.dropResultWrites = true
	outcome, err := fixture.coordinator.ProbeDataEngine(context.Background(), fixture.securityContext)
	if !errors.Is(err, ErrHealthSettlement) || errors.Is(err, ErrHealthUnavailableSettled) ||
		outcome.Status != HealthStatusUnavailable || fixture.runner.calls != 0 {
		t.Fatalf("unavailable readback outcome=%#v err=%v runnerCalls=%d", outcome, err, fixture.runner.calls)
	}
	thread, _ := fixture.store.GetThread(fixture.securityContext.ThreadID)
	turn, _ := appmodel.TurnByID(thread, fixture.securityContext.TurnID)
	if len(healthTurnItems(turn)) != 1 {
		t.Fatalf("dropped unavailable result became durable: %#v", turn["items"])
	}
}

func TestNativeHealthPreSettlementFailureNeverReturnsSettledUnavailable(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	fixture.coordinator.dependencies.DurableAuthority = nil
	outcome, err := fixture.coordinator.ProbeDataEngine(context.Background(), fixture.securityContext)
	if !errors.Is(err, ErrHealthAuthorityInvalid) || errors.Is(err, ErrHealthUnavailableSettled) ||
		outcome.Status != HealthStatusUnavailable || fixture.runner.calls != 0 {
		t.Fatalf("pre-settlement outcome=%#v err=%v runnerCalls=%d", outcome, err, fixture.runner.calls)
	}
	thread, _ := fixture.store.GetThread(fixture.securityContext.ThreadID)
	turn, _ := appmodel.TurnByID(thread, fixture.securityContext.TurnID)
	if len(healthTurnItems(turn)) != 0 {
		t.Fatalf("pre-settlement failure wrote records: %#v", turn["items"])
	}
}

type nativeHealthCoordinatorFixture struct {
	securityContext domainsecurity.TurnSecurityContext
	store           *nativeHealthStoreStub
	runner          *nativeRunnerStub
	coordinator     *HealthCoordinator
}

func newNativeHealthCoordinatorFixture(t *testing.T) nativeHealthCoordinatorFixture {
	t.Helper()
	now := time.Date(2026, 7, 16, 15, 0, 0, 0, time.UTC)
	securityContext := nativeAdmissionContext(t, "thread-native-health", "turn-native-health", now)
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	store := &nativeHealthStoreStub{thread: map[string]any{
		"id": securityContext.ThreadID, "workspace": securityContext.WorkspaceRealPath, "securityState": securityRecord,
		"turns": []any{map[string]any{
			"id": securityContext.TurnID, "threadId": securityContext.ThreadID, "status": "running",
			"securityContext": securityRecord, "items": []any{},
		}},
	}}
	runner := &nativeRunnerStub{result: nativeReadyResult()}
	effectGate := effectgateapp.New()
	service := NewService(Dependencies{
		Threads: store, DurableAuthority: parsedNativeAuthority{}, LiveAuthority: nativeLiveAuthority{},
		HealthOnlyAuthority: nativeHealthOnlyAuthorityV1(),
		AcquireEffect:       effectGate.AcquireEffect, Runner: runner, Now: func() time.Time { return now },
	})
	coordinator := NewHealthCoordinator(HealthDependencies{
		Store: store, DurableAuthority: parsedNativeAuthority{}, AcquireEffect: effectGate.AcquireEffect,
		Service: service, Now: func() time.Time { return now },
	})
	return nativeHealthCoordinatorFixture{
		securityContext: securityContext, store: store, runner: runner, coordinator: coordinator,
	}
}

func persistActiveNativeHealthCall(t *testing.T, fixture nativeHealthCoordinatorFixture) {
	t.Helper()
	policy, ok := domainnative.Policy(domainnative.ComponentDataEngine, "health")
	if !ok {
		t.Fatal("native health policy is unavailable")
	}
	canonicalArguments, ok := domainnative.CanonicalArgumentsV1(policy.ComponentID, policy.Operation)
	if !ok {
		t.Fatal("native health arguments are unavailable")
	}
	call := healthToolCall(fixture.securityContext, policy, canonicalArguments)
	issuedAt := fixture.coordinator.dependencies.Now().UTC()
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: fixture.securityContext, Provider: domainnative.NativeProvider, ServerIdentity: domainnative.NativeServerIdentity,
		ToolName: call.Name, ToolCallID: call.ID, ConnectionEpoch: 0,
		ArgsHash: domainsecurity.CanonicalJSONHash(call.Arguments), SchemaHash: policy.SchemaHash,
		ScopeHash: domainnative.ScopeHash(fixture.securityContext, policy.ComponentID, policy.Operation), ReadOnly: policy.ReadOnly,
		ApprovalState: "not_required", IssuedAt: issuedAt, ExpiresAt: issuedAt.Add(healthGrantTTL),
	})
	callItem, _, err := appturn.ToolCallReadyRecords(appturn.ToolCallReadyInput{
		ThreadID: fixture.securityContext.ThreadID, TurnID: fixture.securityContext.TurnID,
		ItemID: healthToolCallItemID(fixture.securityContext.TurnID, call), CreatedAt: issuedAt.Format(time.RFC3339Nano),
		Call: call, ToolKind: toolcatalogapp.ToolKind(call.Name), Context: fixture.securityContext, Grant: grant,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.AppendItemToTurn(fixture.securityContext.ThreadID, fixture.securityContext.TurnID, callItem); err != nil {
		t.Fatal(err)
	}
}

type nativeHealthStoreStub struct {
	mu               sync.Mutex
	thread           map[string]any
	dropResultWrites bool
	dropCallToolKind bool
}

type blockingHealthRunner struct {
	calls   atomic.Int32
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (runner *blockingHealthRunner) Execute(context.Context, domainnative.Request) (domainnative.Result, error) {
	runner.calls.Add(1)
	runner.once.Do(func() { close(runner.entered) })
	<-runner.release
	return nativeReadyResult(), nil
}

func (store *nativeHealthStoreStub) GetThread(threadID string) (map[string]any, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if strings.TrimSpace(contracts.StringField(store.thread, "id")) != strings.TrimSpace(threadID) {
		return nil, errors.New("thread unavailable")
	}
	return contracts.CloneMap(store.thread), nil
}

func (store *nativeHealthStoreStub) AppendItemToTurn(threadID, turnID string, item map[string]any) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.dropResultWrites && strings.TrimSpace(contracts.StringField(item, "kind")) == "tool_result" {
		return nil
	}
	if strings.TrimSpace(contracts.StringField(store.thread, "id")) != strings.TrimSpace(threadID) {
		return errors.New("thread unavailable")
	}
	item = contracts.CloneMap(item)
	if store.dropCallToolKind && strings.TrimSpace(contracts.StringField(item, "kind")) == "tool_call" {
		delete(item, "toolKind")
	}
	if err := appturn.ValidateSecurityBoundAppend(store.thread, turnID, item); err != nil {
		return err
	}
	next, ok := appturn.AppendItemToTurn(appturn.AppendItemInput{
		Thread: store.thread, TurnID: turnID, Item: item, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if !ok {
		return errors.New("turn unavailable")
	}
	store.thread = next
	return nil
}

func (store *nativeHealthStoreStub) PatchTurnItemStatus(threadID, turnID, itemID, status string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if strings.TrimSpace(contracts.StringField(store.thread, "id")) != strings.TrimSpace(threadID) {
		return errors.New("thread unavailable")
	}
	if err := appturn.ValidatePatchTurnItemStatus(store.thread, turnID); err != nil {
		return err
	}
	next, ok := appturn.PatchTurnItemStatus(appturn.PatchItemStatusInput{
		Thread: store.thread, TurnID: turnID, ItemID: itemID, Status: status,
		FinishedAt: time.Now().UTC().Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if !ok {
		return fmt.Errorf("item %s unavailable", itemID)
	}
	store.thread = next
	return nil
}
