package nativecomponent

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	appmodel "analytix.local/runtime-go/internal/app/model"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	nativecomponentport "analytix.local/runtime-go/internal/ports/nativecomponent"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestRuntimeAuthorityUnavailableModeSettlesAndBlocksProvider(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	dependencies := fixture.coordinator.dependencies
	dependencies.Service = nil
	authority, err := NewUnavailableRuntimeAuthority(dependencies)
	if err != nil {
		t.Fatalf("new unavailable runtime authority: %v", err)
	}
	if authority.AccountFlowsAvailable() {
		t.Fatal("unavailable runtime advertised additive account-flow capability")
	}
	if err := authority.PrepareCaseTurn(context.Background(), fixture.securityContext); !errors.Is(err, ErrHealthUnavailableSettled) {
		t.Fatalf("unavailable case turn was admitted: %v", err)
	}
	if fixture.runner.calls != 0 {
		t.Fatalf("unavailable runtime invoked runner %d times", fixture.runner.calls)
	}
	thread, _ := fixture.store.GetThread(fixture.securityContext.ThreadID)
	turn, _ := appmodel.TurnByID(thread, fixture.securityContext.TurnID)
	if len(healthTurnItems(turn)) != 2 {
		t.Fatalf("unavailable result was not durably settled: %#v", turn["items"])
	}
}

func TestRuntimeAuthorityExactSettledReadyPrepareIsIdempotent(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	health := fixture.coordinator.dependencies
	health.Service = nil
	authority, err := NewReadyRuntimeAuthority(RuntimeAuthorityDependencies{
		Health: health, LiveAuthority: nativeLiveAuthority{},
		HealthOnlyAuthority: nativeHealthOnlyAuthorityV1(), Owner: fixture.runner,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := authority.PrepareCaseTurn(context.Background(), fixture.securityContext); err != nil {
		t.Fatalf("first prepare: %v", err)
	}
	if err := authority.PrepareCaseTurn(context.Background(), fixture.securityContext); err != nil {
		t.Fatalf("settled-ready readback: %v", err)
	}
	if fixture.runner.calls != 1 {
		t.Fatalf("idempotent prepare native calls = %d", fixture.runner.calls)
	}
	if result, flowErr := authority.AnalyzeAccountFlows(context.Background(), AnalyzeAccountFlowsInput{}, nativeFlowEvidenceConsumerV1()); !errors.Is(flowErr, ErrUnavailable) || !reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) {
		t.Fatalf("missing flow-only composition affected ready authority: result=%#v err=%v", result, flowErr)
	}
	thread, _ := fixture.store.GetThread(fixture.securityContext.ThreadID)
	turn, _ := appmodel.TurnByID(thread, fixture.securityContext.TurnID)
	if len(healthTurnItems(turn)) != 2 {
		t.Fatalf("idempotent prepare changed durable records: %#v", turn["items"])
	}
}

func TestRuntimeAuthorityNilNeverSkipsExecutableCaseTurn(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	var authority *RuntimeAuthority
	if err := authority.PrepareCaseTurn(context.Background(), fixture.securityContext); !errors.Is(err, ErrRuntimeAuthorityUnavailable) {
		t.Fatalf("nil runtime authority error = %v", err)
	}
}

func TestRuntimeAuthorityScopesAccountFlowUnavailabilityToTypedCall(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	dependencies := fixture.coordinator.dependencies
	dependencies.Service = nil
	authority, err := NewUnavailableRuntimeAuthority(dependencies)
	if err != nil {
		t.Fatal(err)
	}
	result, err := authority.AnalyzeAccountFlows(context.Background(), AnalyzeAccountFlowsInput{}, nativeFlowEvidenceConsumerV1())
	if !errors.Is(err, ErrUnavailable) || !reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) {
		t.Fatalf("unavailable typed flow result=%#v err=%v", result, err)
	}
	general, contextErr := securitytest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-general-native", TurnID: "turn-general-native", WorkspaceRealPath: "/workspace/general-native",
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, ContextEpoch: 1,
	})
	if contextErr != nil {
		t.Fatal(contextErr)
	}
	if err := authority.PrepareCaseTurn(context.Background(), general); err != nil {
		t.Fatalf("typed flow unavailability blocked general Agent turn: %v", err)
	}
}

func TestRuntimeAuthorityForwardsTypedAccountFlowThroughReadyService(t *testing.T) {
	fixture := newNativeFlowAdmissionFixture(t)
	runner := &nativeRunnerStub{copyFlowSource: true}
	authority := &RuntimeAuthority{service: fixture.service(runner)}
	if !authority.AccountFlowsAvailable() {
		t.Fatal("fully composed account-flow service was not advertised as available")
	}
	result, err := authority.AnalyzeAccountFlows(context.Background(), fixture.input, nativeFlowEvidenceConsumerV1())
	if !errors.Is(err, ErrResultInvalid) || !reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) ||
		runner.flowCalls != 1 || fixture.source.copies != 1 {
		t.Fatalf("ready typed flow was not forwarded: result=%#v err=%v calls=%d copies=%d", result, err, runner.flowCalls, fixture.source.copies)
	}
}

func TestRuntimeAuthorityBoundaryOnlyDoesNotRequireNativeAuthority(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	boundary, err := securitytest.BoundaryOnlyContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: fixture.securityContext.ThreadID, TurnID: fixture.securityContext.TurnID,
		WorkspaceRealPath: fixture.securityContext.WorkspaceRealPath,
		TenantID:          fixture.securityContext.TenantID, UserID: fixture.securityContext.UserID,
		ContextEpoch: fixture.securityContext.ContextEpoch,
	})
	if err != nil {
		t.Fatalf("build boundary-only context: %v", err)
	}
	var authority *RuntimeAuthority
	if err := authority.PrepareCaseTurn(context.Background(), boundary); err != nil {
		t.Fatalf("boundary-only context required native authority: %v", err)
	}
}

func TestRuntimeAuthorityCanceledContextNeverRunsOrSettles(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	dependencies := fixture.coordinator.dependencies
	dependencies.Service = nil
	authority, err := NewUnavailableRuntimeAuthority(dependencies)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := authority.PrepareCaseTurn(ctx, fixture.securityContext); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled prepare error = %v", err)
	}
	thread, _ := fixture.store.GetThread(fixture.securityContext.ThreadID)
	turn, _ := appmodel.TurnByID(thread, fixture.securityContext.TurnID)
	if len(healthTurnItems(turn)) != 0 {
		t.Fatalf("canceled context wrote health records: %#v", turn["items"])
	}
}

func TestRuntimeAuthorityCloseRetriesOwnerFailure(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	health := fixture.coordinator.dependencies
	health.Service = nil
	health.Now = time.Now
	closeCalls := 0
	fixture.runner.closeFn = func() error {
		closeCalls++
		if closeCalls == 1 {
			return errors.New("injected owner close failure")
		}
		return nil
	}
	authority, err := NewReadyRuntimeAuthority(RuntimeAuthorityDependencies{
		Health:              health,
		LiveAuthority:       nativeLiveAuthority{},
		HealthOnlyAuthority: nativeHealthOnlyAuthorityV1(),
		Owner:               fixture.runner,
	})
	if err != nil {
		t.Fatalf("new ready runtime authority: %v", err)
	}
	if err := authority.Close(); err == nil {
		t.Fatal("first owner close failure was swallowed")
	}
	if err := authority.Close(); err != nil {
		t.Fatalf("retry owner close: %v", err)
	}
	if closeCalls != 2 {
		t.Fatalf("owner close calls = %d", closeCalls)
	}
}

func TestRuntimeAuthorityUnsafeNativeFailureBlocksProviderAfterSettlement(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	health := fixture.coordinator.dependencies
	health.Service = nil
	fixture.runner.err = nativecomponentport.ErrTerminationUnconfirmed
	fixture.runner.result = domainnative.Result{}
	authority, err := NewReadyRuntimeAuthority(RuntimeAuthorityDependencies{
		Health: health, LiveAuthority: nativeLiveAuthority{},
		HealthOnlyAuthority: nativeHealthOnlyAuthorityV1(), Owner: fixture.runner,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = authority.PrepareCaseTurn(context.Background(), fixture.securityContext)
	if !errors.Is(err, ErrHealthExecutionUnsafe) || errors.Is(err, ErrHealthUnavailableSettled) {
		t.Fatalf("unsafe native failure did not block provider: %v", err)
	}
	thread, _ := fixture.store.GetThread(fixture.securityContext.ThreadID)
	turn, _ := appmodel.TurnByID(thread, fixture.securityContext.TurnID)
	if len(healthTurnItems(turn)) != 2 {
		t.Fatalf("unsafe native failure was not safely settled: %#v", turn["items"])
	}
}

func TestRuntimeAuthorityContainedNativeFailureOnlyDisablesCaseCapability(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	health := fixture.coordinator.dependencies
	health.Service = nil
	fixture.runner.err = nativecomponentport.ErrProtocolInvalid
	fixture.runner.result = domainnative.Result{}
	authority, err := NewReadyRuntimeAuthority(RuntimeAuthorityDependencies{
		Health: health, LiveAuthority: nativeLiveAuthority{},
		HealthOnlyAuthority: nativeHealthOnlyAuthorityV1(), Owner: fixture.runner,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = authority.PrepareCaseTurn(context.Background(), fixture.securityContext)
	if !errors.Is(err, ErrHealthExecutionUnsafe) || !errors.Is(err, ErrHealthUnavailableSettled) {
		t.Fatalf("contained case-only failure did not preserve ordinary lane: %v", err)
	}
}

func TestRuntimeAuthorityDerivesOptionalAccountFlowRunnerFromLifecycleOwner(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	health := fixture.coordinator.dependencies
	health.Service = nil
	authority, err := NewReadyRuntimeAuthority(RuntimeAuthorityDependencies{
		Health: health, LiveAuthority: nativeLiveAuthority{},
		HealthOnlyAuthority: nativeHealthOnlyAuthorityV1(), Owner: fixture.runner,
	})
	if err != nil {
		t.Fatal(err)
	}
	if authority.service.dependencies.AccountFlowRunner != fixture.runner {
		t.Fatal("account-flow execution was not bound to the lifecycle owner")
	}
}

func TestRuntimeAuthorityCancellationDuringNativeExecutionIsPreservedAndSettled(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	health := fixture.coordinator.dependencies
	health.Service = nil
	health.Now = time.Now
	owner := &cancelingNativeOwner{entered: make(chan struct{})}
	authority, err := NewReadyRuntimeAuthority(RuntimeAuthorityDependencies{
		Health: health, LiveAuthority: nativeLiveAuthority{},
		HealthOnlyAuthority: nativeHealthOnlyAuthorityV1(), Owner: owner,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- authority.PrepareCaseTurn(ctx, fixture.securityContext) }()
	select {
	case <-owner.entered:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("native execution did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation was reclassified: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled native execution did not return")
	}
	thread, _ := fixture.store.GetThread(fixture.securityContext.ThreadID)
	turn, _ := appmodel.TurnByID(thread, fixture.securityContext.TurnID)
	if len(healthTurnItems(turn)) != 2 {
		t.Fatalf("canceled native execution left grant open: %#v", turn["items"])
	}
}

func TestRuntimeAuthorityCancellationCannotMaskUnconfirmedTermination(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	health := fixture.coordinator.dependencies
	health.Service = nil
	health.Now = time.Now
	owner := &cancelingNativeOwner{entered: make(chan struct{}), errAfterCancel: nativecomponentport.ErrTerminationUnconfirmed}
	authority, err := NewReadyRuntimeAuthority(RuntimeAuthorityDependencies{
		Health: health, LiveAuthority: nativeLiveAuthority{},
		HealthOnlyAuthority: nativeHealthOnlyAuthorityV1(), Owner: owner,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- authority.PrepareCaseTurn(ctx, fixture.securityContext) }()
	select {
	case <-owner.entered:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("native execution did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, ErrHealthExecutionUnsafe) || !errors.Is(err, nativecomponentport.ErrTerminationUnconfirmed) ||
			errors.Is(err, ErrHealthUnavailableSettled) {
			t.Fatalf("termination failure was masked by cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("termination failure did not return")
	}
}

func TestRuntimeAuthorityEffectWaitDeadlineIsPreservedWithoutGrant(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	health := fixture.coordinator.dependencies
	health.Service = nil
	health.AcquireEffect = func(ctx context.Context, _ domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
		<-ctx.Done()
		return ctx, nil, ctx.Err()
	}
	authority, err := NewUnavailableRuntimeAuthority(health)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err = authority.PrepareCaseTurn(ctx, fixture.securityContext)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("effect wait deadline was reclassified: %v", err)
	}
	thread, _ := fixture.store.GetThread(fixture.securityContext.ThreadID)
	turn, _ := appmodel.TurnByID(thread, fixture.securityContext.TurnID)
	if len(healthTurnItems(turn)) != 0 {
		t.Fatalf("timed-out effect wait persisted a grant: %#v", turn["items"])
	}
}

func TestRuntimeAuthorityConstructorsRejectTypedNilDependencies(t *testing.T) {
	fixture := newNativeHealthCoordinatorFixture(t)
	health := fixture.coordinator.dependencies
	health.Service = nil
	var nilStore *nativeHealthStoreStub
	health.Store = nilStore
	if authority, err := NewUnavailableRuntimeAuthority(health); authority != nil || !errors.Is(err, ErrRuntimeAuthorityUnavailable) {
		t.Fatalf("typed-nil store survived: authority=%#v err=%v", authority, err)
	}

	health = fixture.coordinator.dependencies
	health.Service = nil
	var nilOwner *nativeRunnerStub
	if authority, err := NewReadyRuntimeAuthority(RuntimeAuthorityDependencies{
		Health: health, LiveAuthority: nativeLiveAuthority{},
		HealthOnlyAuthority: nativeHealthOnlyAuthorityV1(), Owner: nilOwner,
	}); authority != nil || !errors.Is(err, ErrRuntimeAuthorityUnavailable) {
		t.Fatalf("typed-nil owner survived: authority=%#v err=%v", authority, err)
	}
	var nilLive *typedNilLiveAuthority
	if authority, err := NewReadyRuntimeAuthority(RuntimeAuthorityDependencies{
		Health: health, LiveAuthority: nilLive,
		HealthOnlyAuthority: nativeHealthOnlyAuthorityV1(), Owner: fixture.runner,
	}); authority != nil || !errors.Is(err, ErrRuntimeAuthorityUnavailable) {
		t.Fatalf("typed-nil live authority survived: authority=%#v err=%v", authority, err)
	}
	if authority, err := NewReadyRuntimeAuthority(RuntimeAuthorityDependencies{
		Health: health, LiveAuthority: nativeLiveAuthority{}, Owner: fixture.runner,
	}); authority != nil || !errors.Is(err, ErrRuntimeAuthorityUnavailable) {
		t.Fatalf("nil health-only authority survived: authority=%#v err=%v", authority, err)
	}
	var nilHealthOnly *typedNilLiveAuthority
	if authority, err := NewReadyRuntimeAuthority(RuntimeAuthorityDependencies{
		Health: health, LiveAuthority: nativeLiveAuthority{},
		HealthOnlyAuthority: nilHealthOnly, Owner: fixture.runner,
	}); authority != nil || !errors.Is(err, ErrRuntimeAuthorityUnavailable) {
		t.Fatalf("typed-nil health-only authority survived: authority=%#v err=%v", authority, err)
	}
}

type cancelingNativeOwner struct {
	entered        chan struct{}
	errAfterCancel error
}

func (owner *cancelingNativeOwner) Execute(ctx context.Context, _ domainnative.Request) (domainnative.Result, error) {
	close(owner.entered)
	<-ctx.Done()
	if owner.errAfterCancel != nil {
		return domainnative.Result{}, owner.errAfterCancel
	}
	return domainnative.Result{}, ctx.Err()
}

func (*cancelingNativeOwner) AnalyzeAccountFlows(
	context.Context,
	domainnative.Request,
	domainfundsquerysource.DescriptorV1,
	fundsquerysourceport.ExactReadLease,
) (domainnative.AnalyzeAccountFlowsResultV1, error) {
	return domainnative.AnalyzeAccountFlowsResultV1{}, nativecomponentport.ErrUnavailable
}

func (*cancelingNativeOwner) Close() error { return nil }

type typedNilLiveAuthority struct{}

func (*typedNilLiveAuthority) ValidateCurrent(context.Context, domainsecurity.TurnSecurityContext) error {
	return nil
}
