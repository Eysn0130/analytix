package nativecomponent

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	caseentityport "analytix.local/runtime-go/internal/ports/caseentity"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	currentdatasettest "analytix.local/runtime-go/internal/testsupport/currentdataset"
	fundsquerysourcefixture "analytix.local/runtime-go/internal/testsupport/fundsquerysourcefixture"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type accountFlowPostNativeCurrentnessTestV1 = func(
	context.Context,
	domainsecurity.TurnSecurityContext,
	domainfundsquerysource.DescriptorV1,
	func(context.Context) error,
) error

func nativeComponentTestToolCallID(seed string) string {
	entropy := sha256.Sum256([]byte("analytix.native-component-test-tool-call/v1\x00" + seed))
	identity, err := domainmodel.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		panic(err)
	}
	return identity
}

func TestNativeComponentExecuteInputCannotSupplyAuthorityOrProcessConfiguration(t *testing.T) {
	typeOfInput := reflect.TypeOf(ExecuteInput{})
	fields := make([]string, 0, typeOfInput.NumField())
	for index := 0; index < typeOfInput.NumField(); index++ {
		fields = append(fields, typeOfInput.Field(index).Name)
	}
	want := []string{"ThreadID", "TurnID", "GrantID"}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("execute input fields = %v, want %v", fields, want)
	}
}

func TestNativeComponentCanonicalHealthDoesNotEnterGeneralLiveAuthority(t *testing.T) {
	fixture := newNativeAdmissionFixture(t, "not_required", time.Minute)
	runner := &nativeRunnerStub{result: nativeReadyResult()}
	generalLiveCalls := 0
	service := NewService(Dependencies{
		Threads:          &sequenceThreadReader{threads: []map[string]any{fixture.thread}},
		DurableAuthority: parsedNativeAuthority{},
		LiveAuthority: LiveAuthorityFunc(func(
			context.Context,
			domainsecurity.TurnSecurityContext,
		) error {
			generalLiveCalls++
			return ErrAuthorityInvalid
		}),
		AcquireEffect: nativeEffectLease,
		Runner:        runner,
		Now:           func() time.Time { return fixture.now },
	})

	result, err := service.Execute(context.Background(), fixture.input)
	if !errors.Is(err, ErrAuthorityInvalid) || result != (domainnative.Result{}) ||
		generalLiveCalls != 0 || runner.calls != 0 {
		t.Fatalf(
			"canonical health entered general full currentness: result=%#v err=%v general_live=%d runner=%d",
			result,
			err,
			generalLiveCalls,
			runner.calls,
		)
	}
}

func TestNativeComponentCanonicalHealthRevalidatesMetadataAuthorityAtEveryPhase(t *testing.T) {
	fixture := newNativeAdmissionFixture(t, "not_required", time.Minute)
	order := make([]string, 0, 7)
	runner := &nativeRunnerStub{result: nativeReadyResult()}
	generalLiveCalls := 0
	ordinaryEffectCalls := 0
	dataEngineCalls := 0
	service := NewService(Dependencies{
		Threads:          &sequenceThreadReader{threads: []map[string]any{fixture.thread}},
		DurableAuthority: parsedNativeAuthority{},
		LiveAuthority: LiveAuthorityFunc(func(
			context.Context,
			domainsecurity.TurnSecurityContext,
		) error {
			generalLiveCalls++
			return ErrAuthorityInvalid
		}),
		HealthOnlyAuthority: NewHealthOnlyCurrentnessV1(
			LiveAuthorityFunc(func(context.Context, domainsecurity.TurnSecurityContext) error {
				ordinaryEffectCalls++
				order = append(order, fmt.Sprintf("ordinary:%d", ordinaryEffectCalls))
				return nil
			}),
			func(context.Context) error {
				dataEngineCalls++
				order = append(order, fmt.Sprintf("data_engine:%d", dataEngineCalls))
				return nil
			},
		),
		AcquireEffect: nativeEffectLease,
		Runner:        runner,
		Now:           func() time.Time { return fixture.now },
	})
	runner.onExecute = func() { order = append(order, "runner") }

	result, err := service.Execute(context.Background(), fixture.input)
	wantOrder := []string{
		"ordinary:1", "data_engine:1",
		"ordinary:2", "data_engine:2",
		"runner",
		"ordinary:3", "data_engine:3",
	}
	if err != nil || result != nativeReadyResult() || !reflect.DeepEqual(order, wantOrder) ||
		generalLiveCalls != 0 || ordinaryEffectCalls != 3 || dataEngineCalls != 3 || runner.calls != 1 {
		t.Fatalf(
			"health metadata admission order invalid: result=%#v err=%v order=%v general=%d ordinary=%d data_engine=%d runner=%d",
			result,
			err,
			order,
			generalLiveCalls,
			ordinaryEffectCalls,
			dataEngineCalls,
			runner.calls,
		)
	}
}

func TestNativeComponentCanonicalHealthFailsClosedOnMetadataDriftAtEveryPhase(t *testing.T) {
	tests := []struct {
		name              string
		failOrdinaryAt    int
		failDataEngineAt  int
		wantOrdinaryCalls int
		wantDataCalls     int
		wantRunnerCalls   int
		wantAcquireCalls  int
		wantReleaseCalls  int
	}{
		{name: "ordinary before effect", failOrdinaryAt: 1, wantOrdinaryCalls: 1},
		{name: "data engine before effect", failDataEngineAt: 1, wantOrdinaryCalls: 1, wantDataCalls: 1},
		{name: "ordinary inside effect", failOrdinaryAt: 2, wantOrdinaryCalls: 2, wantDataCalls: 1, wantAcquireCalls: 1, wantReleaseCalls: 1},
		{name: "data engine inside effect", failDataEngineAt: 2, wantOrdinaryCalls: 2, wantDataCalls: 2, wantAcquireCalls: 1, wantReleaseCalls: 1},
		{name: "ordinary after runner", failOrdinaryAt: 3, wantOrdinaryCalls: 3, wantDataCalls: 2, wantRunnerCalls: 1, wantAcquireCalls: 1, wantReleaseCalls: 1},
		{name: "data engine after runner", failDataEngineAt: 3, wantOrdinaryCalls: 3, wantDataCalls: 3, wantRunnerCalls: 1, wantAcquireCalls: 1, wantReleaseCalls: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newNativeAdmissionFixture(t, "not_required", time.Minute)
			runner := &nativeRunnerStub{result: nativeReadyResult()}
			generalLiveCalls := 0
			ordinaryEffectCalls := 0
			dataEngineCalls := 0
			acquireCalls := 0
			releaseCalls := 0
			service := NewService(Dependencies{
				Threads:          &sequenceThreadReader{threads: []map[string]any{fixture.thread}},
				DurableAuthority: parsedNativeAuthority{},
				LiveAuthority: LiveAuthorityFunc(func(
					context.Context,
					domainsecurity.TurnSecurityContext,
				) error {
					generalLiveCalls++
					return ErrAuthorityInvalid
				}),
				HealthOnlyAuthority: NewHealthOnlyCurrentnessV1(
					LiveAuthorityFunc(func(context.Context, domainsecurity.TurnSecurityContext) error {
						ordinaryEffectCalls++
						if ordinaryEffectCalls == test.failOrdinaryAt {
							return ErrAuthorityInvalid
						}
						return nil
					}),
					func(context.Context) error {
						dataEngineCalls++
						if dataEngineCalls == test.failDataEngineAt {
							return ErrAuthorityInvalid
						}
						return nil
					},
				),
				AcquireEffect: func(
					ctx context.Context,
					_ domainsecurity.TurnSecurityContext,
				) (context.Context, func(), error) {
					acquireCalls++
					return ctx, func() { releaseCalls++ }, nil
				},
				Runner: runner,
				Now:    func() time.Time { return fixture.now },
			})

			result, err := service.Execute(context.Background(), fixture.input)
			if !errors.Is(err, ErrAuthorityInvalid) || result != (domainnative.Result{}) ||
				generalLiveCalls != 0 || ordinaryEffectCalls != test.wantOrdinaryCalls ||
				dataEngineCalls != test.wantDataCalls || runner.calls != test.wantRunnerCalls ||
				acquireCalls != test.wantAcquireCalls || releaseCalls != test.wantReleaseCalls {
				t.Fatalf(
					"metadata drift escaped: result=%#v err=%v general=%d ordinary=%d data_engine=%d runner=%d acquire=%d release=%d",
					result,
					err,
					generalLiveCalls,
					ordinaryEffectCalls,
					dataEngineCalls,
					runner.calls,
					acquireCalls,
					releaseCalls,
				)
			}
		})
	}
}

func TestNativeComponentHealthOnlyCurrentnessRequiresCompleteCanonicalAdmission(t *testing.T) {
	fixture := newNativeAdmissionFixture(t, "not_required", time.Minute)
	healthPolicy, ok := domainnative.Policy(domainnative.ComponentDataEngine, "health")
	if !ok {
		t.Fatal("native health policy missing")
	}
	flowPolicy, ok := domainnative.Policy(
		domainnative.ComponentDataEngine,
		domainnative.OperationFundsAnalyzeAccountFlows,
	)
	if !ok {
		t.Fatal("native account-flow policy missing")
	}
	tests := []struct {
		name    string
		payload json.RawMessage
		mutate  func(*domainsecurity.ExecutionGrantInput)
	}{
		{
			name:    "non canonical arguments",
			payload: json.RawMessage(`{"unexpected":true}`),
			mutate: func(input *domainsecurity.ExecutionGrantInput) {
				input.ArgsHash = domainsecurity.CanonicalJSONHash([]byte(`{"unexpected":true}`))
			},
		},
		{
			name:    "wrong component",
			payload: json.RawMessage(`{}`),
			mutate: func(input *domainsecurity.ExecutionGrantInput) {
				input.ToolName = "native__analysis_compute__health"
			},
		},
		{
			name:    "non health operation",
			payload: json.RawMessage(`{}`),
			mutate: func(input *domainsecurity.ExecutionGrantInput) {
				input.ToolName = domainnative.ToolName(flowPolicy.ComponentID, flowPolicy.Operation)
				input.SchemaHash = flowPolicy.SchemaHash
				input.ScopeHash = domainnative.ScopeHash(
					fixture.securityContext,
					flowPolicy.ComponentID,
					flowPolicy.Operation,
				)
			},
		},
		{
			name:    "writable grant",
			payload: json.RawMessage(`{}`),
			mutate: func(input *domainsecurity.ExecutionGrantInput) {
				input.ReadOnly = false
			},
		},
		{
			name:    "wrong schema",
			payload: json.RawMessage(`{}`),
			mutate: func(input *domainsecurity.ExecutionGrantInput) {
				input.SchemaHash = domainsecurity.SHA256Hex([]byte("wrong-health-schema"))
			},
		},
		{
			name:    "wrong scope",
			payload: json.RawMessage(`{}`),
			mutate: func(input *domainsecurity.ExecutionGrantInput) {
				input.ScopeHash = domainsecurity.SHA256Hex([]byte("wrong-health-scope"))
			},
		},
		{
			name:    "wrong provider grant",
			payload: json.RawMessage(`{}`),
			mutate: func(input *domainsecurity.ExecutionGrantInput) {
				input.Provider = "provider_untrusted"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			grantInput := domainsecurity.ExecutionGrantInput{
				Context: fixture.securityContext, Provider: domainnative.NativeProvider,
				ServerIdentity: domainnative.NativeServerIdentity,
				ToolName:       domainnative.ToolName(healthPolicy.ComponentID, healthPolicy.Operation),
				ToolCallID:     nativeComponentTestToolCallID("health-closed-" + test.name),
				ArgsHash:       domainsecurity.CanonicalJSONHash(test.payload),
				SchemaHash:     healthPolicy.SchemaHash,
				ScopeHash: domainnative.ScopeHash(
					fixture.securityContext,
					healthPolicy.ComponentID,
					healthPolicy.Operation,
				),
				ReadOnly: true, ApprovalState: "not_required",
				IssuedAt: fixture.now, ExpiresAt: fixture.now.Add(time.Minute),
			}
			test.mutate(&grantInput)
			grant := domainsecurity.NewExecutionGrant(grantInput)
			thread := nativeAdmissionThread(t, fixture.securityContext, grant, test.payload)
			healthCalls := 0
			generalCalls := 0
			runner := &nativeRunnerStub{result: nativeReadyResult()}
			service := NewService(Dependencies{
				Threads:          &sequenceThreadReader{threads: []map[string]any{thread}},
				DurableAuthority: parsedNativeAuthority{},
				LiveAuthority: LiveAuthorityFunc(func(
					context.Context,
					domainsecurity.TurnSecurityContext,
				) error {
					generalCalls++
					return nil
				}),
				HealthOnlyAuthority: LiveAuthorityFunc(func(
					context.Context,
					domainsecurity.TurnSecurityContext,
				) error {
					healthCalls++
					return nil
				}),
				AcquireEffect: nativeEffectLease,
				Runner:        runner,
				Now:           func() time.Time { return fixture.now },
			})
			input := ExecuteInput{
				ThreadID: fixture.securityContext.ThreadID,
				TurnID:   fixture.securityContext.TurnID,
				GrantID:  grant.GrantID,
			}
			result, err := service.Execute(context.Background(), input)
			if !errors.Is(err, ErrGrantInvalid) || result != (domainnative.Result{}) ||
				healthCalls != 0 || generalCalls != 0 || runner.calls != 0 {
				t.Fatalf(
					"invalid health admission reached currentness/runner: result=%#v err=%v health=%d general=%d runner=%d",
					result,
					err,
					healthCalls,
					generalCalls,
					runner.calls,
				)
			}
		})
	}
}

func TestNativeComponentDSV2DriftPermitsOnlyMetadataHealth(t *testing.T) {
	healthFixture := newNativeAdmissionFixture(t, "not_required", time.Minute)
	healthRunner := &nativeRunnerStub{result: nativeReadyResult()}
	healthService := healthFixture.service(healthRunner)
	healthService.dependencies.LiveAuthority = nativeLiveAuthority{err: ErrAuthorityInvalid}

	if result, err := healthService.Execute(context.Background(), healthFixture.input); err != nil ||
		result != nativeReadyResult() || healthRunner.calls != 1 {
		t.Fatalf("DSV2-only drift blocked metadata health: result=%#v err=%v runner=%d", result, err, healthRunner.calls)
	}

	flowFixture := newNativeFlowAdmissionFixture(t)
	flowRunner := &nativeRunnerStub{copyFlowSource: true}
	flowService := flowFixture.service(flowRunner)
	flowGeneralCalls := 0
	flowHealthOnlyCalls := 0
	flowService.dependencies.LiveAuthority = LiveAuthorityFunc(func(
		context.Context,
		domainsecurity.TurnSecurityContext,
	) error {
		flowGeneralCalls++
		return ErrAuthorityInvalid
	})
	flowService.dependencies.HealthOnlyAuthority = LiveAuthorityFunc(func(
		context.Context,
		domainsecurity.TurnSecurityContext,
	) error {
		flowHealthOnlyCalls++
		return nil
	})
	sourceCalls := 0
	subjectCalls := 0
	counterpartyCalls := 0
	consumerCalls := 0
	flowService.dependencies.UseCurrentAccountFlowSource = func(
		context.Context,
		domainsecurity.TurnSecurityContext,
		func(
			context.Context,
			domainfundsquerysource.DescriptorV1,
			fundsquerysourceport.ExactReadLease,
			domainnative.AccountFlowSourceRowResolverV1,
			accountFlowPostNativeCurrentnessTestV1,
		) error,
	) error {
		sourceCalls++
		return nil
	}
	flowService.dependencies.ResolveAccountFlowSubject = func(
		context.Context,
		domainsecurity.TurnSecurityContext,
		domainfundsquerysource.DescriptorV1,
		domaincaseentity.ModelEntityAliasV1,
		ValidateCurrentAccountFlowCallback,
		func(AccountFlowSubjectResolutionV1) error,
	) error {
		subjectCalls++
		return nil
	}
	flowService.dependencies.ResolveAccountFlowCounterparty = func(
		context.Context,
		domainsecurity.TurnSecurityContext,
		domainfundsquerysource.DescriptorV1,
		string,
		string,
		ValidateCurrentAccountFlowCallback,
		func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error,
	) error {
		counterpartyCalls++
		return nil
	}
	result, err := flowService.AnalyzeAccountFlows(
		context.Background(),
		flowFixture.input,
		func(domainnative.AccountFlowHostEvidenceProjectionV1) error {
			consumerCalls++
			return nil
		},
	)
	if !errors.Is(err, ErrAuthorityInvalid) ||
		!reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) ||
		flowGeneralCalls != 1 || flowHealthOnlyCalls != 0 || flowRunner.flowCalls != 0 ||
		sourceCalls != 0 || subjectCalls != 0 || counterpartyCalls != 0 ||
		consumerCalls != 0 || flowFixture.source.CopyCount() != 0 {
		t.Fatalf(
			"metadata health minted Funds readiness: result=%#v err=%v general=%d health_only=%d native=%d source=%d subject=%d counterparty=%d consumer=%d copies=%d",
			result,
			err,
			flowGeneralCalls,
			flowHealthOnlyCalls,
			flowRunner.flowCalls,
			sourceCalls,
			subjectCalls,
			counterpartyCalls,
			consumerCalls,
			flowFixture.source.CopyCount(),
		)
	}
}

func TestNativeComponentRequiresExactActiveDurableGrantAndHostDeadline(t *testing.T) {
	fixture := newNativeAdmissionFixture(t, "not_required", time.Minute)
	runner := &nativeRunnerStub{result: nativeReadyResult()}
	service := fixture.service(runner)
	if _, err := service.admit(context.Background(), fixture.input, nil, nil, fixture.now); err != nil {
		t.Fatalf("fixture admission failed: %v", err)
	}

	result, err := service.Execute(context.Background(), fixture.input)
	if err != nil || result != nativeReadyResult() {
		t.Fatalf("execute result=%#v err=%v", result, err)
	}
	policy, ok := domainnative.Policy(domainnative.ComponentDataEngine, "health")
	if !ok {
		t.Fatal("native health policy is unavailable")
	}
	if runner.calls != 1 || runner.request.Grant != fixture.grant || runner.request.Context != fixture.securityContext ||
		runner.request.ComponentID != domainnative.ComponentDataEngine || runner.request.Operation != "health" ||
		!runner.request.Deadline.Equal(fixture.now.Add(policy.MaxDuration)) {
		t.Fatalf("runner received non-host authority: calls=%d request=%#v", runner.calls, runner.request)
	}

	fake := fixture.input
	fake.GrantID = domainsecurity.SHA256Hex([]byte("self-reported-grant"))
	if result, err := service.Execute(context.Background(), fake); !errors.Is(err, ErrGrantInvalid) || result != (domainnative.Result{}) || runner.calls != 1 {
		t.Fatalf("unknown grant reached runner: result=%#v err=%v calls=%d", result, err, runner.calls)
	}
}

func TestNativeComponentDurableJSONCannotSubstituteForCallbackScopedFlowArguments(t *testing.T) {
	now := time.Date(2026, 7, 27, 7, 0, 0, 0, time.UTC)
	securityContext := nativeAdmissionContext(t, "thread-flow", "turn-flow", now)
	privateAccountKey := "host-private-account-key"
	argumentsInput := domainnative.NewAnalyzeAccountFlowsArgumentsInputV1(domainnative.AnalyzeAccountFlowsArgumentsInputV1{
		CaseID: securityContext.CaseID, DatasetSnapshotID: securityContext.DatasetSnapshotID,
		ContextEpoch: securityContext.ContextEpoch, ContextDigest: securityContext.ContextDigest,
		CaseBindingHash:                      securityContext.CaseBindingHash,
		ExpectedProducerContentID:            domainsecurity.FundsProducerContentIDPrefixV1 + strings.Repeat("4", 64),
		ExpectedProducerManifestSHA256:       strings.Repeat("5", 64),
		ExpectedDuckDBContentSnapshotDigest:  strings.Repeat("b", 64),
		ExpectedDuckDBSnapshotManifestSHA256: strings.Repeat("c", 64),
		ExpectedMaterializationIdentity:      "txn_daily_snapshot:v12:" + strings.Repeat("a", 64),
		SubjectAlias:                         "acct:1",
		SubjectRef:                           "cer1_" + strings.Repeat("a", 64),
		SubjectResolutionDigest:              strings.Repeat("6", 64),
		StartInclusive:                       "2026-01-01T00:00:00Z", EndInclusive: "2026-01-01T00:02:00Z",
		EvidenceRowLimit: 10, DatasetUTCOffsetMinutes: 0, ExpectedCurrency: "CNY",
		MinorUnitScale: domainnative.AccountFlowMinorUnitScaleV1, ScanCap: 100,
	}, privateAccountKey)
	arguments, err := domainnative.NewAnalyzeAccountFlowsArgumentsV1(argumentsInput)
	if err != nil {
		t.Fatal(err)
	}
	policy, ok := domainnative.Policy(domainnative.ComponentDataEngine, domainnative.OperationFundsAnalyzeAccountFlows)
	if !ok {
		t.Fatal("fixed flow policy missing")
	}
	// This is a hostile durability fixture: even an exact private JSON copy in
	// a tool-call item must not become the callback-scoped native argument.
	payload, err := json.Marshal(map[string]any{
		"caseId": argumentsInput.CaseID, "datasetSnapshotId": argumentsInput.DatasetSnapshotID,
		"contextEpoch": argumentsInput.ContextEpoch, "contextDigest": argumentsInput.ContextDigest,
		"caseBindingHash":                argumentsInput.CaseBindingHash,
		"expectedProducerContentId":      argumentsInput.ExpectedProducerContentID,
		"expectedProducerManifestSha256": argumentsInput.ExpectedProducerManifestSHA256,
		"subjectAlias":                   argumentsInput.SubjectAlias,
		"subjectRef":                     argumentsInput.SubjectRef, "resolvedAccountKey": privateAccountKey,
		"subjectResolutionDigest": argumentsInput.SubjectResolutionDigest,
		"startInclusive":          "2026-01-01T00:00:00.000000Z", "endInclusive": "2026-01-01T00:02:00.000000Z",
		"evidenceRowLimit": 10, "datasetUtcOffsetMinutes": 0, "expectedCurrency": "CNY",
		"minorUnitScale": 2, "scanCap": 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: domainnative.NativeProvider, ServerIdentity: domainnative.NativeServerIdentity,
		ToolName: domainnative.ToolName(policy.ComponentID, policy.Operation), ToolCallID: nativeComponentTestToolCallID("flow"),
		ArgsHash: domainnative.AnalyzeAccountFlowsArgumentsHashV1(arguments), SchemaHash: policy.SchemaHash,
		ScopeHash: domainnative.ScopeHash(securityContext, policy.ComponentID, policy.Operation), ReadOnly: true,
		ApprovalState: "not_required", IssuedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	thread := nativeAdmissionThread(t, securityContext, grant, payload)
	runner := &nativeRunnerStub{result: nativeReadyResult()}
	service := NewService(Dependencies{
		Threads: &sequenceThreadReader{threads: []map[string]any{thread}}, DurableAuthority: parsedNativeAuthority{},
		LiveAuthority: nativeLiveAuthority{}, AcquireEffect: nativeEffectLease, Runner: runner,
		Now: func() time.Time { return now },
	})
	result, runErr := service.Execute(context.Background(), ExecuteInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, GrantID: grant.GrantID,
	})
	if !errors.Is(runErr, ErrGrantInvalid) || result != (domainnative.Result{}) || runner.calls != 0 {
		t.Fatalf("durable JSON reached fixed flow runner: result=%#v err=%v calls=%d", result, runErr, runner.calls)
	}
}

func TestNativeComponentAccountFlowInjectsOnlyCallbackArgumentsAndRejectsInvalidTypedResult(t *testing.T) {
	fixture := newNativeFlowAdmissionFixture(t)
	runner := &nativeRunnerStub{copyFlowSource: true}
	service := fixture.service(runner)

	result, err := service.AnalyzeAccountFlows(context.Background(), fixture.input, nativeFlowEvidenceConsumerV1())
	if !errors.Is(err, ErrResultInvalid) || !reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) {
		t.Fatalf("invalid typed result survived: result=%#v err=%v", result, err)
	}
	if runner.flowCalls != 1 || runner.request.AccountFlowArguments == nil ||
		runner.request.Grant != fixture.nativeGrant || runner.request.Grant == fixture.grant ||
		runner.request.Context != fixture.securityContext ||
		runner.flowDescriptor != fixture.descriptor || runner.flowSource != fixture.source ||
		fixture.source.CopyCount() != 1 {
		t.Fatalf(
			"flow runner did not receive exact host inputs: calls=%d request=%#v descriptor=%#v sourceCopies=%d",
			runner.flowCalls,
			runner.request,
			runner.flowDescriptor,
			fixture.source.CopyCount(),
		)
	}
	if domainnative.ValidateRequest(runner.request, fixture.now) != nil {
		t.Fatal("runner received a request that was not reconstructed from the durable grant")
	}
}

func TestNativeComponentAccountFlowReturnsOnlyProviderSafeSemanticResult(t *testing.T) {
	fixture := newNativeFlowAdmissionFixture(t)
	want := nativeFlowValidEvidenceResult(t, fixture.arguments)
	runner := &nativeRunnerStub{copyFlowSource: true, flowResult: want}
	service := fixture.service(runner)
	privateResolver := service.dependencies.ResolveAccountFlowCounterparty
	privateInstitutionCalls := 0
	service.dependencies.ResolveAccountFlowCounterparty = func(
		ctx context.Context,
		securityContext domainsecurity.TurnSecurityContext,
		descriptor domainfundsquerysource.DescriptorV1,
		sourceExactAccount string,
		bankInstitution string,
		validateCurrent ValidateCurrentAccountFlowCallback,
		consume func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error,
	) error {
		privateInstitutionCalls++
		if bankInstitution != "Analytix Test Bank" {
			return errors.New("host-private institution drifted")
		}
		return privateResolver(
			ctx,
			securityContext,
			descriptor,
			sourceExactAccount,
			bankInstitution,
			validateCurrent,
			func(reference domaincaseentity.ReferenceV1, display domaincaseentity.DisplayLabelV1) error {
				if domaincaseentity.ValidateDisplayLabelV1(display) != nil || display.Institution != bankInstitution {
					return errors.New("protected-local institution display drifted")
				}
				return consume(reference, display)
			},
		)
	}
	consumerCalls := 0
	summaryCalls := 0
	rowCalls := 0
	consumeEvidence := func(projection domainnative.AccountFlowHostEvidenceProjectionV1) error {
		consumerCalls++
		return projection.UseExactV1(
			func(
				subjectRef string,
				_ string,
				_ uint64,
				_, _, _, _, _, _ string,
				_ uint8,
				_, _, _ string,
				_ uint64,
				_, _ bool,
				_ uint64,
				_ domainnative.AccountFlowProviderSemanticCoverageV1,
				_, _ string,
			) error {
				summaryCalls++
				if subjectRef != fixture.arguments.SubjectRef {
					return errors.New("evidence subject reference mismatch")
				}
				return nil
			},
			func(_ int, subjectRef, sourceRecordID, sourceFileID string, sourceRowNumber uint64, _ string, _ string, _ string, _ string, _ uint8) error {
				rowCalls++
				if subjectRef != fixture.arguments.SubjectRef || !strings.HasPrefix(sourceRecordID, "srow1_") ||
					strings.TrimSpace(sourceFileID) == "" || sourceRowNumber == 0 {
					return errors.New("evidence row binding mismatch")
				}
				return nil
			},
		)
	}

	result, err := service.AnalyzeAccountFlows(context.Background(), fixture.input, consumeEvidence)
	if err != nil || result.SubjectAlias != fixture.input.Intent.SubjectAlias || result.InflowMinor != "1000" ||
		result.OutflowMinor != "300" || result.NetMinor != "700" || result.TransactionCount != 2 ||
		result.EvidenceTransactionCount != 2 || len(result.Transactions) != 2 ||
		!result.CounterpartySemanticsComplete || result.QueryHash != want.QueryHash ||
		result.ResultHash != want.ResultHash ||
		consumerCalls != 1 || summaryCalls != 1 || rowCalls != 2 || privateInstitutionCalls != 2 ||
		*fixture.resolverCalls != 2 ||
		runner.flowCalls != 1 || fixture.source.CopyCount() != 1 {
		t.Fatalf(
			"provider semantic projection failed: result=%#v err=%v consumer=%d summary=%d rows=%d institutions=%d resolver=%d calls=%d copies=%d",
			result,
			err,
			consumerCalls,
			summaryCalls,
			rowCalls,
			privateInstitutionCalls,
			*fixture.resolverCalls,
			runner.flowCalls,
			fixture.source.CopyCount(),
		)
	}
	providerJSON, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(providerJSON), result.SubjectAlias) != 1 || strings.Contains(string(providerJSON), fixture.arguments.SubjectRef) {
		t.Fatalf("provider semantic lost alias or leaked authority reference: %s", providerJSON)
	}
	for index, row := range result.Transactions {
		if row.Counterparty.Status != domainnative.AccountFlowCounterpartyResolvedV1 ||
			row.Counterparty.Alias == "" ||
			!strings.HasPrefix(row.EvidenceRef, "srow1_") ||
			(index != 0 && row.Counterparty.Alias != result.Transactions[0].Counterparty.Alias) {
			t.Fatalf("provider counterparty semantics drifted: %#v", result.Transactions)
		}
	}
	for _, forbidden := range []string{
		"6222021234567890123",
		"6217009876543210987",
		"private-source:/cases/secret.duckdb",
		"Private A&B Counterparty",
		"Analytix Test Bank",
		"Private Bank",
		"duckdb",
		"SELECT ",
	} {
		if strings.Contains(string(providerJSON), forbidden) {
			t.Fatalf("provider semantic leaked %q: %s", forbidden, providerJSON)
		}
	}
}

func TestNativeComponentAccountFlowDoesNotRepeatFullDatasetAdmissionInsideLiveSourceCallback(t *testing.T) {
	fixture := newNativeFlowAdmissionFixture(t)
	runner := &nativeRunnerStub{
		copyFlowSource: true,
		flowResult:     nativeFlowValidEvidenceResult(t, fixture.arguments),
	}
	service := fixture.service(runner)
	originalUseCurrent := service.dependencies.UseCurrentAccountFlowSource
	originalCallbackCurrent := service.dependencies.ValidateCurrentAccountFlowCallback
	insideSourceCallback := false
	fullDatasetAdmissionsInsideCallback := 0
	fullDatasetAdmissions := 0
	fullDatasetAdmissionsAfterSource := -1
	fullDatasetAdmissionsAtConsumer := -1
	callbackCurrentnessValidations := 0
	service.dependencies.ValidateCurrentAccountFlowCallback = func(
		ctx context.Context,
		securityContext domainsecurity.TurnSecurityContext,
	) error {
		callbackCurrentnessValidations++
		return originalCallbackCurrent(ctx, securityContext)
	}
	service.dependencies.LiveAuthority = LiveAuthorityFunc(func(
		_ context.Context,
		_ domainsecurity.TurnSecurityContext,
	) error {
		fullDatasetAdmissions++
		if insideSourceCallback {
			fullDatasetAdmissionsInsideCallback++
			return errors.New("full DSV2 inventory re-entered inside the exact source callback")
		}
		return nil
	})
	service.dependencies.UseCurrentAccountFlowSource = func(
		ctx context.Context,
		securityContext domainsecurity.TurnSecurityContext,
		use func(
			context.Context,
			domainfundsquerysource.DescriptorV1,
			fundsquerysourceport.ExactReadLease,
			domainnative.AccountFlowSourceRowResolverV1,
			accountFlowPostNativeCurrentnessTestV1,
		) error,
	) error {
		err := originalUseCurrent(
			ctx,
			securityContext,
			func(
				sourceCtx context.Context,
				descriptor domainfundsquerysource.DescriptorV1,
				source fundsquerysourceport.ExactReadLease,
				resolveSourceRow domainnative.AccountFlowSourceRowResolverV1,
				revalidatePostNative accountFlowPostNativeCurrentnessTestV1,
			) error {
				insideSourceCallback = true
				defer func() { insideSourceCallback = false }()
				return use(sourceCtx, descriptor, source, resolveSourceRow, revalidatePostNative)
			},
		)
		fullDatasetAdmissionsAfterSource = fullDatasetAdmissions
		return err
	}

	consumerCalls := 0
	result, err := service.AnalyzeAccountFlows(
		context.Background(),
		fixture.input,
		func(projection domainnative.AccountFlowHostEvidenceProjectionV1) error {
			consumerCalls++
			fullDatasetAdmissionsAtConsumer = fullDatasetAdmissions
			return nativeFlowEvidenceConsumerV1()(projection)
		},
	)
	if err != nil || result.SubjectAlias != fixture.input.Intent.SubjectAlias ||
		fullDatasetAdmissionsInsideCallback != 0 || fullDatasetAdmissionsAfterSource != 2 ||
		fullDatasetAdmissionsAtConsumer != 3 || fullDatasetAdmissions != 4 ||
		callbackCurrentnessValidations == 0 || runner.flowCalls != 1 || consumerCalls != 1 {
		t.Fatalf(
			"account flow currentness ordering drifted: result=%#v err=%v full_inside=%d full_source=%d full_consumer=%d full_total=%d callback=%d calls=%d consumer=%d",
			result,
			err,
			fullDatasetAdmissionsInsideCallback,
			fullDatasetAdmissionsAfterSource,
			fullDatasetAdmissionsAtConsumer,
			fullDatasetAdmissions,
			callbackCurrentnessValidations,
			runner.flowCalls,
			consumerCalls,
		)
	}
}

func TestNativeComponentAccountFlowSharedHeadDriftAfterNativeDiscardsProvisionalResult(t *testing.T) {
	fixture := newNativeFlowAdmissionFixture(t)
	runner := &nativeRunnerStub{
		copyFlowSource: true,
		flowResult:     nativeFlowValidEvidenceResult(t, fixture.arguments),
	}
	service := fixture.service(runner)
	originalUseCurrent := service.dependencies.UseCurrentAccountFlowSource
	originalCallbackCurrent := service.dependencies.ValidateCurrentAccountFlowCallback
	postNativeUses := 0
	callbackValidations := 0
	callbackValidationsAtPostNative := -1
	service.dependencies.ValidateCurrentAccountFlowCallback = func(
		ctx context.Context,
		securityContext domainsecurity.TurnSecurityContext,
	) error {
		callbackValidations++
		return originalCallbackCurrent(ctx, securityContext)
	}
	service.dependencies.UseCurrentAccountFlowSource = func(
		ctx context.Context,
		securityContext domainsecurity.TurnSecurityContext,
		use func(
			context.Context,
			domainfundsquerysource.DescriptorV1,
			fundsquerysourceport.ExactReadLease,
			domainnative.AccountFlowSourceRowResolverV1,
			accountFlowPostNativeCurrentnessTestV1,
		) error,
	) error {
		return originalUseCurrent(
			ctx,
			securityContext,
			func(
				sourceContext context.Context,
				descriptor domainfundsquerysource.DescriptorV1,
				source fundsquerysourceport.ExactReadLease,
				resolveSourceRow domainnative.AccountFlowSourceRowResolverV1,
				_ accountFlowPostNativeCurrentnessTestV1,
			) error {
				return use(
					sourceContext,
					descriptor,
					source,
					resolveSourceRow,
					func(
						context.Context,
						domainsecurity.TurnSecurityContext,
						domainfundsquerysource.DescriptorV1,
						func(context.Context) error,
					) error {
						postNativeUses++
						callbackValidationsAtPostNative = callbackValidations
						return datasetsnapshotport.ErrStale
					},
				)
			},
		)
	}
	consumerCalls := 0
	result, err := service.AnalyzeAccountFlows(
		context.Background(),
		fixture.input,
		func(domainnative.AccountFlowHostEvidenceProjectionV1) error {
			consumerCalls++
			return nil
		},
	)
	if !errors.Is(err, ErrAuthorityInvalid) ||
		!reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) ||
		postNativeUses != 1 || callbackValidationsAtPostNative < 1 ||
		callbackValidations != callbackValidationsAtPostNative || consumerCalls != 0 ||
		*fixture.resolverCalls != 0 || runner.flowCalls != 1 || fixture.source.CopyCount() != 1 {
		t.Fatalf(
			"post-native shared-head drift escaped: result=%#v err=%v currentness=%d validations=%d/%d consumer=%d resolver=%d calls=%d copies=%d",
			result,
			err,
			postNativeUses,
			callbackValidationsAtPostNative,
			callbackValidations,
			consumerCalls,
			*fixture.resolverCalls,
			runner.flowCalls,
			fixture.source.CopyCount(),
		)
	}
}

func TestNativeComponentAccountFlowLiveAuthorityDriftAfterNativeReachesNoProjectionOrConsumer(t *testing.T) {
	fixture := newNativeFlowAdmissionFixture(t)
	runner := &nativeRunnerStub{
		copyFlowSource: true,
		flowResult:     nativeFlowValidEvidenceResult(t, fixture.arguments),
	}
	service := fixture.service(runner)
	originalCallbackCurrent := service.dependencies.ValidateCurrentAccountFlowCallback
	callbackValidations := 0
	service.dependencies.ValidateCurrentAccountFlowCallback = func(
		ctx context.Context,
		securityContext domainsecurity.TurnSecurityContext,
	) error {
		callbackValidations++
		if runner.flowCalls == 1 {
			return errors.New("principal/risk/case binding changed during native execution")
		}
		return originalCallbackCurrent(ctx, securityContext)
	}
	counterpartyCalls := 0
	originalCounterparty := service.dependencies.ResolveAccountFlowCounterparty
	service.dependencies.ResolveAccountFlowCounterparty = func(
		ctx context.Context,
		securityContext domainsecurity.TurnSecurityContext,
		descriptor domainfundsquerysource.DescriptorV1,
		sourceExactAccount string,
		bankInstitution string,
		validateCurrent ValidateCurrentAccountFlowCallback,
		consume func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error,
	) error {
		counterpartyCalls++
		return originalCounterparty(
			ctx,
			securityContext,
			descriptor,
			sourceExactAccount,
			bankInstitution,
			validateCurrent,
			consume,
		)
	}
	consumerCalls := 0
	result, err := service.AnalyzeAccountFlows(
		context.Background(),
		fixture.input,
		func(domainnative.AccountFlowHostEvidenceProjectionV1) error {
			consumerCalls++
			return nil
		},
	)
	if !errors.Is(err, ErrAuthorityInvalid) ||
		!reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) ||
		callbackValidations == 0 || counterpartyCalls != 0 || consumerCalls != 0 ||
		*fixture.resolverCalls != 0 || runner.flowCalls != 1 || fixture.source.CopyCount() != 1 {
		t.Fatalf(
			"post-native live-authority drift escaped: result=%#v err=%v validations=%d counterparty=%d consumer=%d resolver=%d calls=%d copies=%d",
			result,
			err,
			callbackValidations,
			counterpartyCalls,
			consumerCalls,
			*fixture.resolverCalls,
			runner.flowCalls,
			fixture.source.CopyCount(),
		)
	}
}

func TestNativeComponentAccountFlowCounterpartyUnavailabilityIsRowScopedPartial(t *testing.T) {
	fixture := newNativeFlowAdmissionFixture(t)
	runner := &nativeRunnerStub{
		copyFlowSource: true,
		flowResult:     nativeFlowValidEvidenceResult(t, fixture.arguments),
	}
	service := fixture.service(runner)
	resolverCalls := 0
	service.dependencies.ResolveAccountFlowCounterparty = func(
		context.Context,
		domainsecurity.TurnSecurityContext,
		domainfundsquerysource.DescriptorV1,
		string,
		string,
		ValidateCurrentAccountFlowCallback,
		func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error,
	) error {
		resolverCalls++
		return domainnative.ErrAccountFlowCounterpartySemanticUnavailableV1
	}
	result, err := service.AnalyzeAccountFlows(
		context.Background(),
		fixture.input,
		nativeFlowEvidenceConsumerV1(),
	)
	if err != nil {
		t.Fatalf("row-scoped counterparty gap blocked the bounded analysis: %v", err)
	}
	if resolverCalls != 2 || result.CounterpartySemanticsComplete ||
		result.Coverage.State != domainnative.AccountFlowCoveragePartialV1 ||
		len(result.Coverage.Gaps) != 1 ||
		result.Coverage.Gaps[0] != domainnative.AccountFlowGapCounterpartyResolutionV1 {
		t.Fatalf("counterparty gap was promoted or hidden: calls=%d result=%#v", resolverCalls, result)
	}
	for _, row := range result.Transactions {
		if row.Counterparty.Status != domainnative.AccountFlowCounterpartyUnresolvedV1 ||
			row.Counterparty.Alias != "" || row.Counterparty.EntityType != "" || row.Counterparty.AccountType != "" {
			t.Fatalf("unavailable counterparty leaked identity semantics: %#v", row.Counterparty)
		}
	}
}

func TestNativeComponentAccountFlowEvidenceConsumerFailureFailsClosedOnce(t *testing.T) {
	fixture := newNativeFlowAdmissionFixture(t)
	runner := &nativeRunnerStub{
		copyFlowSource: true,
		flowResult:     nativeFlowValidEvidenceResult(t, fixture.arguments),
	}
	service := fixture.service(runner)
	const privateFailure = "private-evidence-consumer:/cases/secret.duckdb:6222021234567890123"
	consumerCalls := 0
	result, err := service.AnalyzeAccountFlows(
		context.Background(),
		fixture.input,
		func(domainnative.AccountFlowHostEvidenceProjectionV1) error {
			consumerCalls++
			return errors.New(privateFailure)
		},
	)
	if !errors.Is(err, ErrResultInvalid) || strings.Contains(err.Error(), privateFailure) ||
		!reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) ||
		consumerCalls != 1 || *fixture.resolverCalls != 2 || runner.flowCalls != 1 || fixture.source.CopyCount() != 1 {
		t.Fatalf(
			"consumer failure was not closed: result=%#v err=%v consumer=%d resolver=%d calls=%d copies=%d",
			result,
			err,
			consumerCalls,
			*fixture.resolverCalls,
			runner.flowCalls,
			fixture.source.CopyCount(),
		)
	}
}

func TestNativeComponentAccountFlowEvidenceConsumerPanicBurnsCapture(t *testing.T) {
	fixture := newNativeFlowAdmissionFixture(t)
	runner := &nativeRunnerStub{
		copyFlowSource: true,
		flowResult:     nativeFlowValidEvidenceResult(t, fixture.arguments),
	}
	consumerCalls := 0
	var retained domainnative.AccountFlowHostEvidenceProjectionV1
	result, err := fixture.service(runner).AnalyzeAccountFlows(
		context.Background(),
		fixture.input,
		func(projection domainnative.AccountFlowHostEvidenceProjectionV1) error {
			consumerCalls++
			retained = projection
			panic("private evidence consumer panic")
		},
	)
	retainedCalls := 0
	retainedErr := retained.UseExactV1(
		func(
			string, string, uint64, string, string, string, string, string, string, uint8,
			string, string, string, uint64, bool, bool, uint64,
			domainnative.AccountFlowProviderSemanticCoverageV1, string, string,
		) error {
			retainedCalls++
			return nil
		},
		func(int, string, string, string, uint64, string, string, string, string, uint8) error {
			retainedCalls++
			return nil
		},
	)
	if !errors.Is(err, ErrResultInvalid) ||
		!reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) ||
		consumerCalls != 1 || retainedCalls != 0 || !errors.Is(retainedErr, domainnative.ErrResultInvalid) {
		t.Fatalf(
			"consumer panic escaped or retained capture: result=%#v err=%v consumer=%d retained=%d/%v",
			result,
			err,
			consumerCalls,
			retainedCalls,
			retainedErr,
		)
	}
}

func TestNativeComponentAccountFlowSourceResolverDriftIsTypedAndPrivate(t *testing.T) {
	fixture := newNativeFlowAdmissionFixture(t)
	runner := &nativeRunnerStub{
		copyFlowSource: true,
		flowResult:     nativeFlowValidEvidenceResult(t, fixture.arguments),
	}
	service := fixture.service(runner)
	useCurrent := service.dependencies.UseCurrentAccountFlowSource
	const privateFailure = "private-resolver-drift:/cases/secret.duckdb:6222021234567890123"
	resolverCalls := 0
	service.dependencies.UseCurrentAccountFlowSource = func(
		ctx context.Context,
		securityContext domainsecurity.TurnSecurityContext,
		use func(
			context.Context,
			domainfundsquerysource.DescriptorV1,
			fundsquerysourceport.ExactReadLease,
			domainnative.AccountFlowSourceRowResolverV1,
			accountFlowPostNativeCurrentnessTestV1,
		) error,
	) error {
		return useCurrent(
			ctx,
			securityContext,
			func(
				sourceCtx context.Context,
				descriptor domainfundsquerysource.DescriptorV1,
				source fundsquerysourceport.ExactReadLease,
				_ domainnative.AccountFlowSourceRowResolverV1,
				revalidatePostNative accountFlowPostNativeCurrentnessTestV1,
			) error {
				return use(
					sourceCtx,
					descriptor,
					source,
					func(string, uint64, func(string) error) error {
						resolverCalls++
						return errors.Join(fundsquerysourceport.ErrMismatch, errors.New(privateFailure))
					},
					revalidatePostNative,
				)
			},
		)
	}
	consumerCalls := 0
	result, err := service.AnalyzeAccountFlows(
		context.Background(),
		fixture.input,
		func(projection domainnative.AccountFlowHostEvidenceProjectionV1) error {
			consumerCalls++
			return nativeFlowEvidenceConsumerV1()(projection)
		},
	)
	if err != fundsquerysourceport.ErrMismatch || strings.Contains(err.Error(), privateFailure) ||
		!reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) ||
		resolverCalls != 1 || consumerCalls != 0 || runner.flowCalls != 1 || fixture.source.CopyCount() != 1 {
		t.Fatalf(
			"resolver drift was not closed: result=%#v err=%v resolver=%d consumer=%d calls=%d copies=%d",
			result,
			err,
			resolverCalls,
			consumerCalls,
			runner.flowCalls,
			fixture.source.CopyCount(),
		)
	}
}

func TestNativeComponentAccountFlowRepeatedSourceCallbackConsumesEvidenceOnce(t *testing.T) {
	fixture := newNativeFlowAdmissionFixture(t)
	runner := &nativeRunnerStub{
		copyFlowSource: true,
		flowResult:     nativeFlowValidEvidenceResult(t, fixture.arguments),
	}
	service := fixture.service(runner)
	useCurrent := service.dependencies.UseCurrentAccountFlowSource
	service.dependencies.UseCurrentAccountFlowSource = func(
		ctx context.Context,
		securityContext domainsecurity.TurnSecurityContext,
		use func(
			context.Context,
			domainfundsquerysource.DescriptorV1,
			fundsquerysourceport.ExactReadLease,
			domainnative.AccountFlowSourceRowResolverV1,
			accountFlowPostNativeCurrentnessTestV1,
		) error,
	) error {
		return useCurrent(
			ctx,
			securityContext,
			func(
				sourceCtx context.Context,
				descriptor domainfundsquerysource.DescriptorV1,
				source fundsquerysourceport.ExactReadLease,
				resolveSourceRow domainnative.AccountFlowSourceRowResolverV1,
				revalidatePostNative accountFlowPostNativeCurrentnessTestV1,
			) error {
				if err := use(sourceCtx, descriptor, source, resolveSourceRow, revalidatePostNative); err != nil {
					return err
				}
				return use(sourceCtx, descriptor, source, resolveSourceRow, revalidatePostNative)
			},
		)
	}
	consumerCalls := 0
	result, err := service.AnalyzeAccountFlows(
		context.Background(),
		fixture.input,
		func(projection domainnative.AccountFlowHostEvidenceProjectionV1) error {
			consumerCalls++
			return nativeFlowEvidenceConsumerV1()(projection)
		},
	)
	if !errors.Is(err, ErrAuthorityInvalid) ||
		!reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) ||
		consumerCalls != 0 || *fixture.resolverCalls != 2 || runner.flowCalls != 1 || fixture.source.CopyCount() != 1 {
		t.Fatalf(
			"repeated callback duplicated evidence: result=%#v err=%v consumer=%d resolver=%d calls=%d copies=%d",
			result,
			err,
			consumerCalls,
			*fixture.resolverCalls,
			runner.flowCalls,
			fixture.source.CopyCount(),
		)
	}
}

func TestNativeComponentAccountFlowProjectionCancellationIsPreserved(t *testing.T) {
	fixture := newNativeFlowAdmissionFixture(t)
	runner := &nativeRunnerStub{
		copyFlowSource: true,
		flowResult:     nativeFlowValidEvidenceResult(t, fixture.arguments),
	}
	consumerCalls := 0
	result, err := fixture.service(runner).AnalyzeAccountFlows(
		context.Background(),
		fixture.input,
		func(domainnative.AccountFlowHostEvidenceProjectionV1) error {
			consumerCalls++
			return context.Canceled
		},
	)
	if !errors.Is(err, context.Canceled) ||
		!reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) ||
		consumerCalls != 1 || *fixture.resolverCalls != 2 || runner.flowCalls != 1 || fixture.source.CopyCount() != 1 {
		t.Fatalf(
			"projection cancellation drifted: result=%#v err=%v consumer=%d resolver=%d calls=%d copies=%d",
			result,
			err,
			consumerCalls,
			*fixture.resolverCalls,
			runner.flowCalls,
			fixture.source.CopyCount(),
		)
	}
}

func TestNativeComponentAccountFlowWaitsForOuterExactUsePostcheck(t *testing.T) {
	fixture := newNativeFlowAdmissionFixture(t)
	want := nativeFlowValidNoHitResult(t, fixture.arguments)
	runner := &nativeRunnerStub{copyFlowSource: true, flowResult: want}
	service := fixture.service(runner)
	useCurrent := service.dependencies.UseCurrentAccountFlowSource
	const privateFailure = "private-source-postcheck:/cases/secret.duckdb:6222021234567890123"
	service.dependencies.UseCurrentAccountFlowSource = func(
		ctx context.Context,
		securityContext domainsecurity.TurnSecurityContext,
		use func(
			context.Context,
			domainfundsquerysource.DescriptorV1,
			fundsquerysourceport.ExactReadLease,
			domainnative.AccountFlowSourceRowResolverV1,
			accountFlowPostNativeCurrentnessTestV1,
		) error,
	) error {
		if err := useCurrent(ctx, securityContext, use); err != nil {
			return err
		}
		return errors.New(privateFailure)
	}

	consumerCalls := 0
	result, err := service.AnalyzeAccountFlows(
		context.Background(),
		fixture.input,
		func(projection domainnative.AccountFlowHostEvidenceProjectionV1) error {
			consumerCalls++
			return nativeFlowEvidenceConsumerV1()(projection)
		},
	)
	if !errors.Is(err, ErrAuthorityInvalid) || strings.Contains(err.Error(), privateFailure) ||
		!reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) ||
		consumerCalls != 0 || runner.flowCalls != 1 || fixture.source.CopyCount() != 1 {
		t.Fatalf(
			"provisional result escaped failed outer postcheck: result=%#v err=%v consumer=%d calls=%d copies=%d",
			result,
			err,
			consumerCalls,
			runner.flowCalls,
			fixture.source.CopyCount(),
		)
	}
}

func TestNativeComponentAccountFlowRejectsMismatchedOpaqueResolutionInsideEffect(t *testing.T) {
	fixture := newNativeFlowAdmissionFixture(t)
	_, err := fixture.caseEntityService.BindReferenceV1(
		context.Background(),
		caseentityapp.NewDeriveReferenceInputV1(
			fixture.securityContext,
			domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			"6217009876543210987",
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	var mismatch AccountFlowSubjectResolutionV1
	mismatchCalls := 0
	err = NewPersistentAccountFlowSubjectResolver(fixture.caseEntityService)(
		context.Background(),
		fixture.securityContext,
		fixture.descriptor,
		"acct:2",
		func(context.Context, domainsecurity.TurnSecurityContext) error { return nil },
		func(value AccountFlowSubjectResolutionV1) error {
			mismatchCalls++
			mismatch = value
			return nil
		},
	)
	if err != nil || mismatchCalls != 1 {
		t.Fatal(err)
	}
	runner := &nativeRunnerStub{copyFlowSource: true}
	service := fixture.service(runner)
	type effectLeaseKey struct{}
	releases := 0
	service.dependencies.AcquireEffect = func(
		ctx context.Context,
		_ domainsecurity.TurnSecurityContext,
	) (context.Context, func(), error) {
		return context.WithValue(ctx, effectLeaseKey{}, true), func() { releases++ }, nil
	}
	resolverCalls := 0
	service.dependencies.ResolveAccountFlowSubject = func(
		ctx context.Context,
		_ domainsecurity.TurnSecurityContext,
		_ domainfundsquerysource.DescriptorV1,
		_ domaincaseentity.ModelEntityAliasV1,
		_ ValidateCurrentAccountFlowCallback,
		use func(AccountFlowSubjectResolutionV1) error,
	) error {
		resolverCalls++
		if leased, _ := ctx.Value(effectLeaseKey{}).(bool); !leased {
			t.Fatal("resolver did not receive the acquired effect context")
		}
		return use(mismatch)
	}

	result, runErr := service.AnalyzeAccountFlows(context.Background(), fixture.input, nativeFlowEvidenceConsumerV1())
	if !errors.Is(runErr, ErrAuthorityInvalid) || !reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) ||
		resolverCalls != 1 || releases != 1 || runner.flowCalls != 0 || fixture.source.CopyCount() != 0 {
		t.Fatalf(
			"mismatched resolution crossed the effect boundary: result=%#v err=%v resolutions=%d releases=%d calls=%d copies=%d",
			result,
			runErr,
			resolverCalls,
			releases,
			runner.flowCalls,
			fixture.source.CopyCount(),
		)
	}
}

func TestNativeComponentAccountFlowRejectsLateAndConcurrentSubjectCallbacks(t *testing.T) {
	t.Run("late first callback", func(t *testing.T) {
		fixture := newNativeFlowAdmissionFixture(t)
		runner := &nativeRunnerStub{}
		service := fixture.service(runner)
		var retained func(AccountFlowSubjectResolutionV1) error
		service.dependencies.ResolveAccountFlowSubject = func(
			_ context.Context,
			_ domainsecurity.TurnSecurityContext,
			_ domainfundsquerysource.DescriptorV1,
			_ domaincaseentity.ModelEntityAliasV1,
			_ ValidateCurrentAccountFlowCallback,
			use func(AccountFlowSubjectResolutionV1) error,
		) error {
			retained = use
			return nil
		}
		consumerCalls := 0
		result, err := service.AnalyzeAccountFlows(
			context.Background(),
			fixture.input,
			func(domainnative.AccountFlowHostEvidenceProjectionV1) error {
				consumerCalls++
				return nil
			},
		)
		lateErr := retained(fixture.resolution)
		if !errors.Is(err, ErrAuthorityInvalid) || !errors.Is(lateErr, ErrAuthorityInvalid) ||
			!reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) ||
			consumerCalls != 0 || runner.flowCalls != 0 || fixture.source.CopyCount() != 0 {
			t.Fatalf(
				"late subject callback survived: result=%#v err=%v late=%v consumer=%d calls=%d copies=%d",
				result,
				err,
				lateErr,
				consumerCalls,
				runner.flowCalls,
				fixture.source.CopyCount(),
			)
		}
	})

	t.Run("concurrent callbacks", func(t *testing.T) {
		fixture := newNativeFlowAdmissionFixture(t)
		runner := &nativeRunnerStub{}
		service := fixture.service(runner)
		callbackErrors := make(chan error, 2)
		service.dependencies.ResolveAccountFlowSubject = func(
			_ context.Context,
			_ domainsecurity.TurnSecurityContext,
			_ domainfundsquerysource.DescriptorV1,
			_ domaincaseentity.ModelEntityAliasV1,
			_ ValidateCurrentAccountFlowCallback,
			use func(AccountFlowSubjectResolutionV1) error,
		) error {
			start := make(chan struct{})
			for range 2 {
				go func() {
					<-start
					callbackErrors <- use(fixture.resolution)
				}()
			}
			close(start)
			<-callbackErrors
			<-callbackErrors
			return nil
		}
		consumerCalls := 0
		result, err := service.AnalyzeAccountFlows(
			context.Background(),
			fixture.input,
			func(domainnative.AccountFlowHostEvidenceProjectionV1) error {
				consumerCalls++
				return nil
			},
		)
		if !errors.Is(err, ErrAuthorityInvalid) ||
			!reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) ||
			consumerCalls != 0 || runner.flowCalls != 0 || fixture.source.CopyCount() != 0 {
			t.Fatalf(
				"concurrent subject callbacks survived: result=%#v err=%v consumer=%d calls=%d copies=%d",
				result,
				err,
				consumerCalls,
				runner.flowCalls,
				fixture.source.CopyCount(),
			)
		}
	})

	t.Run("outer close waits for in-flight subject callback", func(t *testing.T) {
		barrier := newCallbackUseBarrier()
		if !barrier.begin() {
			t.Fatal("subject callback did not enter active barrier")
		}
		type closeResult struct {
			attempts  uint32
			completed bool
		}
		closeDone := make(chan closeResult, 1)
		go func() {
			attempts, completed := barrier.close()
			closeDone <- closeResult{attempts: attempts, completed: completed}
		}()

		deadline := time.Now().Add(2 * time.Second)
		for {
			barrier.mu.Lock()
			active := barrier.active
			barrier.mu.Unlock()
			if !active {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("outer subject scope did not deactivate in-flight callback")
			}
			time.Sleep(time.Millisecond)
		}
		select {
		case result := <-closeDone:
			t.Fatalf("outer subject scope returned before callback ended: %#v", result)
		default:
		}
		committed := barrier.commit(func() {
			t.Fatal("inactive subject callback committed provisional resolution")
		})
		if committed {
			t.Fatal("inactive subject callback commit reported success")
		}
		barrier.end(false)
		select {
		case result := <-closeDone:
			if result.attempts != 1 || result.completed {
				t.Fatalf("in-flight subject close state drifted: %#v", result)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("outer subject scope did not finish after callback ended")
		}
	})
}

func TestNativeComponentAccountFlowRejectsLateAndInFlightSourceCallbacks(t *testing.T) {
	t.Run("late first callback", func(t *testing.T) {
		fixture := newNativeFlowAdmissionFixture(t)
		runner := &nativeRunnerStub{}
		service := fixture.service(runner)
		var retained func(
			context.Context,
			domainfundsquerysource.DescriptorV1,
			fundsquerysourceport.ExactReadLease,
			domainnative.AccountFlowSourceRowResolverV1,
			accountFlowPostNativeCurrentnessTestV1,
		) error
		service.dependencies.UseCurrentAccountFlowSource = func(
			_ context.Context,
			_ domainsecurity.TurnSecurityContext,
			use func(
				context.Context,
				domainfundsquerysource.DescriptorV1,
				fundsquerysourceport.ExactReadLease,
				domainnative.AccountFlowSourceRowResolverV1,
				accountFlowPostNativeCurrentnessTestV1,
			) error,
		) error {
			retained = use
			return nil
		}
		consumerCalls := 0
		result, err := service.AnalyzeAccountFlows(
			context.Background(),
			fixture.input,
			func(domainnative.AccountFlowHostEvidenceProjectionV1) error {
				consumerCalls++
				return nil
			},
		)
		lateErr := retained(
			context.Background(),
			fixture.descriptor,
			fixture.source,
			nativeFlowSourceRowResolverV1,
			nativeFlowPostNativeCurrentnessV1(fixture.securityContext, fixture.descriptor),
		)
		if !errors.Is(err, ErrAuthorityInvalid) || !errors.Is(lateErr, ErrAuthorityInvalid) ||
			!reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) ||
			consumerCalls != 0 || runner.flowCalls != 0 || fixture.source.CopyCount() != 0 {
			t.Fatalf(
				"late source callback survived: result=%#v err=%v late=%v consumer=%d calls=%d copies=%d",
				result,
				err,
				lateErr,
				consumerCalls,
				runner.flowCalls,
				fixture.source.CopyCount(),
			)
		}
	})

	t.Run("concurrent callbacks", func(t *testing.T) {
		fixture := newNativeFlowAdmissionFixture(t)
		runner := &nativeRunnerStub{
			copyFlowSource: true,
			flowResult:     nativeFlowValidEvidenceResult(t, fixture.arguments),
		}
		service := fixture.service(runner)
		service.dependencies.UseCurrentAccountFlowSource = func(
			ctx context.Context,
			_ domainsecurity.TurnSecurityContext,
			use func(
				context.Context,
				domainfundsquerysource.DescriptorV1,
				fundsquerysourceport.ExactReadLease,
				domainnative.AccountFlowSourceRowResolverV1,
				accountFlowPostNativeCurrentnessTestV1,
			) error,
		) error {
			revalidatePostNative := nativeFlowPostNativeCurrentnessV1(
				fixture.securityContext,
				fixture.descriptor,
			)
			start := make(chan struct{})
			callbackErrors := make(chan error, 2)
			for range 2 {
				go func() {
					<-start
					callbackErrors <- use(
						ctx,
						fixture.descriptor,
						fixture.source,
						nativeFlowSourceRowResolverV1,
						revalidatePostNative,
					)
				}()
			}
			close(start)
			<-callbackErrors
			<-callbackErrors
			return nil
		}
		consumerCalls := 0
		result, err := service.AnalyzeAccountFlows(
			context.Background(),
			fixture.input,
			func(domainnative.AccountFlowHostEvidenceProjectionV1) error {
				consumerCalls++
				return nil
			},
		)
		if !errors.Is(err, ErrAuthorityInvalid) ||
			!reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) ||
			consumerCalls != 0 || runner.flowCalls != 1 || fixture.source.CopyCount() != 1 {
			t.Fatalf(
				"concurrent source callbacks survived: result=%#v err=%v consumer=%d calls=%d copies=%d",
				result,
				err,
				consumerCalls,
				runner.flowCalls,
				fixture.source.CopyCount(),
			)
		}
	})

	t.Run("outer return waits for in-flight callback", func(t *testing.T) {
		fixture := newNativeFlowAdmissionFixture(t)
		flowStarted := make(chan struct{})
		flowRelease := make(chan struct{})
		runner := &nativeRunnerStub{
			copyFlowSource: true,
			flowResult:     nativeFlowValidEvidenceResult(t, fixture.arguments),
			flowStarted:    flowStarted,
			flowRelease:    flowRelease,
		}
		service := fixture.service(runner)
		callbackDone := make(chan error, 1)
		service.dependencies.UseCurrentAccountFlowSource = func(
			ctx context.Context,
			_ domainsecurity.TurnSecurityContext,
			use func(
				context.Context,
				domainfundsquerysource.DescriptorV1,
				fundsquerysourceport.ExactReadLease,
				domainnative.AccountFlowSourceRowResolverV1,
				accountFlowPostNativeCurrentnessTestV1,
			) error,
		) error {
			go func() {
				callbackDone <- use(
					ctx,
					fixture.descriptor,
					fixture.source,
					nativeFlowSourceRowResolverV1,
					nativeFlowPostNativeCurrentnessV1(fixture.securityContext, fixture.descriptor),
				)
			}()
			<-flowStarted
			return nil
		}
		type outcome struct {
			result domainnative.AccountFlowProviderSemanticResultV1
			err    error
		}
		consumerCalls := 0
		outcomes := make(chan outcome, 1)
		go func() {
			result, err := service.AnalyzeAccountFlows(
				context.Background(),
				fixture.input,
				func(domainnative.AccountFlowHostEvidenceProjectionV1) error {
					consumerCalls++
					return nil
				},
			)
			outcomes <- outcome{result: result, err: err}
		}()
		select {
		case got := <-outcomes:
			t.Fatalf("outer returned before in-flight source callback closed: %#v", got)
		case <-time.After(100 * time.Millisecond):
		}
		close(flowRelease)
		got := <-outcomes
		callbackErr := <-callbackDone
		if !errors.Is(got.err, ErrAuthorityInvalid) || !errors.Is(callbackErr, ErrAuthorityInvalid) ||
			!reflect.DeepEqual(got.result, domainnative.AccountFlowProviderSemanticResultV1{}) ||
			consumerCalls != 0 || runner.flowCalls != 1 || fixture.source.CopyCount() != 1 {
			t.Fatalf(
				"in-flight source callback crossed outer return: result=%#v err=%v callback=%v consumer=%d calls=%d copies=%d",
				got.result,
				got.err,
				callbackErr,
				consumerCalls,
				runner.flowCalls,
				fixture.source.CopyCount(),
			)
		}
	})
}

func TestNativeComponentPersistentSubjectResolutionSharesLiveSourceCallbackUnderOneEffectLease(t *testing.T) {
	fixture := newNativeFlowAdmissionFixture(t)
	runner := &nativeRunnerStub{copyFlowSource: true}
	service := fixture.service(runner)
	type sourceLeaseKey struct{}
	type effectLeaseKey struct{}
	bindingResolved := false
	sourceCallbackActive := false
	sourceCallbacks := 0
	useCurrent := service.dependencies.UseCurrentAccountFlowSource
	service.dependencies.UseCurrentAccountFlowSource = func(
		ctx context.Context,
		securityContext domainsecurity.TurnSecurityContext,
		use func(
			context.Context,
			domainfundsquerysource.DescriptorV1,
			fundsquerysourceport.ExactReadLease,
			domainnative.AccountFlowSourceRowResolverV1,
			accountFlowPostNativeCurrentnessTestV1,
		) error,
	) error {
		if effectActive, _ := ctx.Value(effectLeaseKey{}).(bool); !effectActive {
			t.Fatal("source use escaped the account-flow effect lease")
		}
		if bindingResolved {
			t.Fatal("persistent subject resolution completed before exact-source callback")
		}
		return useCurrent(
			ctx,
			securityContext,
			func(
				sourceCtx context.Context,
				descriptor domainfundsquerysource.DescriptorV1,
				source fundsquerysourceport.ExactReadLease,
				resolveSourceRow domainnative.AccountFlowSourceRowResolverV1,
				revalidatePostNative accountFlowPostNativeCurrentnessTestV1,
			) error {
				sourceCallbacks++
				sourceCallbackActive = true
				defer func() { sourceCallbackActive = false }()
				err := use(
					context.WithValue(sourceCtx, sourceLeaseKey{}, true),
					descriptor,
					source,
					resolveSourceRow,
					revalidatePostNative,
				)
				if !bindingResolved {
					t.Fatal("exact-source callback completed without persistent subject resolution")
				}
				return err
			},
		)
	}
	service.dependencies.AcquireEffect = func(
		ctx context.Context,
		_ domainsecurity.TurnSecurityContext,
	) (context.Context, func(), error) {
		if active, _ := ctx.Value(sourceLeaseKey{}).(bool); active {
			t.Fatal("effect lease was nested under the exact-source callback")
		}
		return context.WithValue(ctx, effectLeaseKey{}, true), func() {}, nil
	}
	resolveCalls := 0
	fixture.caseEntityStore.onResolveBinding = func(ctx context.Context) {
		resolveCalls++
		if sourceActive, _ := ctx.Value(sourceLeaseKey{}).(bool); !sourceActive || !sourceCallbackActive {
			t.Fatal("persistent binding resolution escaped exact-source callback")
		}
		if effectActive, _ := ctx.Value(effectLeaseKey{}).(bool); !effectActive {
			t.Fatal("persistent binding resolution escaped effect callback")
		}
		bindingResolved = true
	}

	result, err := service.AnalyzeAccountFlows(context.Background(), fixture.input, nativeFlowEvidenceConsumerV1())
	if !errors.Is(err, ErrResultInvalid) ||
		!reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) ||
		!bindingResolved || sourceCallbackActive || resolveCalls == 0 || sourceCallbacks != 1 ||
		runner.flowCalls != 1 || fixture.source.CopyCount() != 1 {
		t.Fatalf(
			"persistent resolver/source sequencing failed: result=%#v err=%v resolutions=%d sources=%d calls=%d copies=%d",
			result,
			err,
			resolveCalls,
			sourceCallbacks,
			runner.flowCalls,
			fixture.source.CopyCount(),
		)
	}
}

func TestNativeComponentAccountFlowRevalidatesHostOwnedOuterCatalogOnEveryAdmission(t *testing.T) {
	fixture := newNativeFlowAdmissionFixture(t)
	want := nativeFlowValidNoHitResult(t, fixture.arguments)
	runner := &nativeRunnerStub{copyFlowSource: true, flowResult: want}
	service := fixture.service(runner)
	validationCalls := 0
	service.dependencies.ValidateCurrentAccountFlowOuterGrant = func(
		_ context.Context,
		securityContext domainsecurity.TurnSecurityContext,
		grant domainsecurity.ExecutionGrant,
	) error {
		validationCalls++
		if securityContext != fixture.securityContext || grant.Provider != fixture.grant.Provider ||
			grant.SchemaHash != fixture.grant.SchemaHash || grant.ScopeHash != fixture.grant.ScopeHash ||
			grant != fixture.grant {
			return errors.New("current host catalog mismatch")
		}
		return nil
	}

	result, err := service.AnalyzeAccountFlows(context.Background(), fixture.input, nativeFlowEvidenceConsumerV1())
	if err != nil || result.SubjectAlias != fixture.input.Intent.SubjectAlias || result.TransactionCount != want.TransactionCount || validationCalls != 8 {
		t.Fatalf("outer catalog validations=%d result=%#v err=%v", validationCalls, result, err)
	}

	for _, field := range []string{"provider", "schema", "scope"} {
		t.Run(field, func(t *testing.T) {
			localFixture := newNativeFlowAdmissionFixture(t)
			localRunner := &nativeRunnerStub{copyFlowSource: true, flowResult: nativeFlowValidNoHitResult(t, localFixture.arguments)}
			localService := localFixture.service(localRunner)
			const privateCatalogError = "private-current-catalog:6222021234567890123"
			localService.dependencies.ValidateCurrentAccountFlowOuterGrant = func(
				_ context.Context,
				_ domainsecurity.TurnSecurityContext,
				grant domainsecurity.ExecutionGrant,
			) error {
				matches := true
				switch field {
				case "provider":
					matches = grant.Provider == "rotated-provider"
				case "schema":
					matches = grant.SchemaHash == nativeFlowDigest("rotated-schema")
				case "scope":
					matches = grant.ScopeHash == nativeFlowDigest("rotated-scope")
				}
				if !matches {
					return errors.New(privateCatalogError)
				}
				return nil
			}
			result, err := localService.AnalyzeAccountFlows(context.Background(), localFixture.input, nativeFlowEvidenceConsumerV1())
			if !errors.Is(err, ErrGrantInvalid) || strings.Contains(err.Error(), privateCatalogError) ||
				!reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) ||
				localRunner.flowCalls != 0 || localFixture.source.CopyCount() != 0 {
				t.Fatalf("stale %s catalog authority survived: result=%#v err=%v", field, result, err)
			}
		})
	}

	t.Run("consumer-pre admission rejects rotated catalog", func(t *testing.T) {
		localFixture := newNativeFlowAdmissionFixture(t)
		localRunner := &nativeRunnerStub{
			copyFlowSource: true,
			flowResult:     nativeFlowValidNoHitResult(t, localFixture.arguments),
		}
		localService := localFixture.service(localRunner)
		validationCalls := 0
		const privateCatalogError = "private-rotated-catalog:6222021234567890123"
		localService.dependencies.ValidateCurrentAccountFlowOuterGrant = func(
			_ context.Context,
			_ domainsecurity.TurnSecurityContext,
			_ domainsecurity.ExecutionGrant,
		) error {
			validationCalls++
			if validationCalls == 7 {
				return errors.New(privateCatalogError)
			}
			return nil
		}

		consumerCalls := 0
		result, err := localService.AnalyzeAccountFlows(
			context.Background(),
			localFixture.input,
			func(projection domainnative.AccountFlowHostEvidenceProjectionV1) error {
				consumerCalls++
				return nativeFlowEvidenceConsumerV1()(projection)
			},
		)
		if !errors.Is(err, ErrAuthorityInvalid) || strings.Contains(err.Error(), privateCatalogError) ||
			!reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) ||
			validationCalls != 7 || consumerCalls != 0 || localRunner.flowCalls != 1 || localFixture.source.CopyCount() != 1 {
			t.Fatalf(
				"late catalog rotation released a provisional result: result=%#v err=%v validations=%d consumer=%d calls=%d copies=%d",
				result,
				err,
				validationCalls,
				consumerCalls,
				localRunner.flowCalls,
				localFixture.source.CopyCount(),
			)
		}
	})
}

func TestNativeComponentAccountFlowPostConsumerFailureBurnsCaptureWithoutEffects(t *testing.T) {
	for _, test := range []struct {
		name        string
		configure   func(*Service, *int, context.CancelFunc)
		want        error
		validations int
	}{
		{
			name: "post-consumer admission",
			configure: func(service *Service, validationCalls *int, _ context.CancelFunc) {
				service.dependencies.ValidateCurrentAccountFlowOuterGrant = func(
					_ context.Context,
					_ domainsecurity.TurnSecurityContext,
					_ domainsecurity.ExecutionGrant,
				) error {
					(*validationCalls)++
					if *validationCalls == 8 {
						return errors.New("private-post-consumer-catalog:/cases/secret.duckdb")
					}
					return nil
				}
			},
			want:        ErrAuthorityInvalid,
			validations: 8,
		},
		{
			name: "post-consumer cancellation",
			configure: func(service *Service, validationCalls *int, _ context.CancelFunc) {
				service.dependencies.ValidateCurrentAccountFlowOuterGrant = func(
					_ context.Context,
					_ domainsecurity.TurnSecurityContext,
					_ domainsecurity.ExecutionGrant,
				) error {
					(*validationCalls)++
					return nil
				}
			},
			want:        context.Canceled,
			validations: 7,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newNativeFlowAdmissionFixture(t)
			runner := &nativeRunnerStub{
				copyFlowSource: true,
				flowResult:     nativeFlowValidEvidenceResult(t, fixture.arguments),
			}
			service := fixture.service(runner)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			validationCalls := 0
			test.configure(service, &validationCalls, cancel)
			consumerCalls := 0
			var retained domainnative.AccountFlowHostEvidenceProjectionV1
			result, err := service.AnalyzeAccountFlows(
				ctx,
				fixture.input,
				func(projection domainnative.AccountFlowHostEvidenceProjectionV1) error {
					consumerCalls++
					retained = projection
					if consumeErr := nativeFlowEvidenceConsumerV1()(projection); consumeErr != nil {
						return consumeErr
					}
					if test.name == "post-consumer cancellation" {
						cancel()
					}
					return nil
				},
			)
			settlementCalls := 0
			persistenceCalls := 0
			publicationCalls := 0
			if err == nil {
				settlementCalls++
				persistenceCalls++
				publicationCalls++
			}
			retainedSummaryCalls := 0
			retainedRowCalls := 0
			retainedErr := retained.UseExactV1(
				func(
					string, string, uint64, string, string, string, string, string, string, uint8,
					string, string, string, uint64, bool, bool, uint64,
					domainnative.AccountFlowProviderSemanticCoverageV1, string, string,
				) error {
					retainedSummaryCalls++
					return nil
				},
				func(int, string, string, string, uint64, string, string, string, string, uint8) error {
					retainedRowCalls++
					return nil
				},
			)
			if !errors.Is(err, test.want) ||
				!reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) ||
				validationCalls != test.validations || consumerCalls != 1 ||
				!errors.Is(retainedErr, domainnative.ErrResultInvalid) ||
				retainedSummaryCalls != 0 || retainedRowCalls != 0 ||
				settlementCalls != 0 || persistenceCalls != 0 || publicationCalls != 0 {
				t.Fatalf(
					"post-consumer failure escaped: result=%#v err=%v validations=%d consumer=%d retained=%v/%d/%d effects=%d/%d/%d",
					result,
					err,
					validationCalls,
					consumerCalls,
					retainedErr,
					retainedSummaryCalls,
					retainedRowCalls,
					settlementCalls,
					persistenceCalls,
					publicationCalls,
				)
			}
		})
	}
}

func TestNativeComponentAccountFlowSanitizesPrivateDependencyErrorsAndReleasesEffectOnce(t *testing.T) {
	const privateFailure = "private-account:6222021234567890123:/cases/secret.duckdb"

	t.Run("source composition", func(t *testing.T) {
		fixture := newNativeFlowAdmissionFixture(t)
		runner := &nativeRunnerStub{}
		service := fixture.service(runner)
		service.dependencies.UseCurrentAccountFlowSource = func(
			context.Context,
			domainsecurity.TurnSecurityContext,
			func(
				context.Context,
				domainfundsquerysource.DescriptorV1,
				fundsquerysourceport.ExactReadLease,
				domainnative.AccountFlowSourceRowResolverV1,
				accountFlowPostNativeCurrentnessTestV1,
			) error,
		) error {
			return errors.New(privateFailure)
		}
		result, err := service.AnalyzeAccountFlows(context.Background(), fixture.input, nativeFlowEvidenceConsumerV1())
		if !errors.Is(err, ErrAuthorityInvalid) || strings.Contains(err.Error(), privateFailure) ||
			!reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) || runner.flowCalls != 0 {
			t.Fatalf("private source composition error escaped: result=%#v err=%v", result, err)
		}
	})

	for _, test := range []struct {
		name      string
		configure func(*Service, *nativeFlowAdmissionFixture, *nativeRunnerStub)
		want      error
	}{
		{
			name: "resolver",
			configure: func(service *Service, _ *nativeFlowAdmissionFixture, _ *nativeRunnerStub) {
				service.dependencies.ResolveAccountFlowSubject = func(
					context.Context,
					domainsecurity.TurnSecurityContext,
					domainfundsquerysource.DescriptorV1,
					domaincaseentity.ModelEntityAliasV1,
					ValidateCurrentAccountFlowCallback,
					func(AccountFlowSubjectResolutionV1) error,
				) error {
					return errors.New(privateFailure)
				}
			},
			want: ErrAuthorityInvalid,
		},
		{
			name: "runner",
			configure: func(_ *Service, _ *nativeFlowAdmissionFixture, runner *nativeRunnerStub) {
				runner.flowErr = errors.New(privateFailure)
			},
			want: ErrAccountFlowExecutionFailed,
		},
		{
			name: "source copy",
			configure: func(_ *Service, fixture *nativeFlowAdmissionFixture, runner *nativeRunnerStub) {
				fixture.source.SetFailure(errors.New(privateFailure))
				runner.copyFlowSource = true
			},
			want: ErrAccountFlowExecutionFailed,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newNativeFlowAdmissionFixture(t)
			runner := &nativeRunnerStub{}
			service := fixture.service(runner)
			releases := 0
			service.dependencies.AcquireEffect = func(
				ctx context.Context,
				_ domainsecurity.TurnSecurityContext,
			) (context.Context, func(), error) {
				return ctx, func() { releases++ }, nil
			}
			test.configure(service, &fixture, runner)
			result, err := service.AnalyzeAccountFlows(context.Background(), fixture.input, nativeFlowEvidenceConsumerV1())
			if !errors.Is(err, test.want) || strings.Contains(err.Error(), privateFailure) ||
				!reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) || releases != 1 {
				t.Fatalf("private %s error escaped: result=%#v err=%v releases=%d", test.name, result, err, releases)
			}
		})
	}
}

func TestNativeComponentAccountFlowBindsExactProviderIntentBeforePrivateEffects(t *testing.T) {
	fixture := newNativeFlowAdmissionFixture(t)
	fixture.input.Intent.EndInclusive = "2026-01-01T00:03:00.000000Z"
	runner := &nativeRunnerStub{copyFlowSource: true}
	service := fixture.service(runner)
	effectCalls := 0
	resolutionCalls := 0
	service.dependencies.AcquireEffect = func(
		ctx context.Context,
		_ domainsecurity.TurnSecurityContext,
	) (context.Context, func(), error) {
		effectCalls++
		return ctx, func() {}, nil
	}
	service.dependencies.ResolveAccountFlowSubject = func(
		context.Context,
		domainsecurity.TurnSecurityContext,
		domainfundsquerysource.DescriptorV1,
		domaincaseentity.ModelEntityAliasV1,
		ValidateCurrentAccountFlowCallback,
		func(AccountFlowSubjectResolutionV1) error,
	) error {
		resolutionCalls++
		return nil
	}

	result, err := service.AnalyzeAccountFlows(context.Background(), fixture.input, nativeFlowEvidenceConsumerV1())
	if !errors.Is(err, ErrGrantInvalid) || !reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) ||
		effectCalls != 0 || resolutionCalls != 0 || runner.flowCalls != 0 || fixture.source.CopyCount() != 0 {
		t.Fatalf(
			"unbound intent reached private effects: result=%#v err=%v effects=%d resolutions=%d calls=%d copies=%d",
			result, err, effectCalls, resolutionCalls, runner.flowCalls, fixture.source.CopyCount(),
		)
	}
}

func TestNativeComponentAccountFlowInputCannotSupplyPrivateSubjectResolution(t *testing.T) {
	fixture := newNativeFlowAdmissionFixture(t)
	typeOfInput := reflect.TypeOf(AnalyzeAccountFlowsInput{})
	fields := make([]string, 0, typeOfInput.NumField())
	for index := 0; index < typeOfInput.NumField(); index++ {
		fields = append(fields, typeOfInput.Field(index).Name)
	}
	want := []string{"ExecuteInput", "Intent"}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("account-flow input fields=%v want=%v", fields, want)
	}
	private := fixture.resolution
	if _, err := json.Marshal(private); !errors.Is(err, ErrRequestInvalid) ||
		strings.Contains(private.String(), "6222021234567890123") ||
		strings.Contains(private.GoString(), fixture.arguments.SubjectResolutionDigest) {
		t.Fatal("host-private subject resolution became serializable or printable")
	}
	resolutionType := reflect.TypeOf(AccountFlowSubjectResolutionV1{})
	for index := 0; index < resolutionType.NumField(); index++ {
		if resolutionType.Field(index).IsExported() {
			t.Fatalf("host-private resolution field %q is exported", resolutionType.Field(index).Name)
		}
	}
	type definedResolutionV1 AccountFlowSubjectResolutionV1
	defined := definedResolutionV1(private)
	definedBody, definedErr := json.Marshal(defined)
	definedFormat := fmt.Sprintf("%#v", defined)
	if definedErr != nil || strings.Contains(string(definedBody), "6222021234567890123") ||
		strings.Contains(definedFormat, "6222021234567890123") ||
		strings.Contains(definedFormat, fixture.arguments.SubjectResolutionDigest) {
		t.Fatalf(
			"defined-type conversion exposed host-private resolution: body=%q format=%q err=%v",
			definedBody,
			definedFormat,
			definedErr,
		)
	}
}

func TestNativeComponentAccountFlowRejectsRegisteredNativeImplementationGrant(t *testing.T) {
	fixture := newNativeFlowAdmissionFixture(t)
	thread := nativeAdmissionThread(
		t,
		fixture.securityContext,
		fixture.nativeGrant,
		json.RawMessage(`{}`),
	)
	turn := thread["turns"].([]any)[0].(map[string]any)
	callItem := turn["items"].([]any)[0].(map[string]any)
	callItem["arguments"] = domaintoolcall.PublicToolCallArgumentsProjectionRecordV1()
	fixture.input.ExecuteInput.GrantID = fixture.nativeGrant.GrantID
	runner := &nativeRunnerStub{copyFlowSource: true}
	service := fixture.service(runner)
	service.dependencies.Threads = &sequenceThreadReader{threads: []map[string]any{thread}}

	result, err := service.AnalyzeAccountFlows(context.Background(), fixture.input, nativeFlowEvidenceConsumerV1())
	if !errors.Is(err, ErrGrantInvalid) ||
		!reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) ||
		runner.flowCalls != 0 || fixture.source.CopyCount() != 0 {
		t.Fatalf(
			"registered native implementation grant reached funds effect: result=%#v err=%v calls=%d copies=%d",
			result,
			err,
			runner.flowCalls,
			fixture.source.CopyCount(),
		)
	}
}

func TestNativeComponentAccountFlowRejectsHostSourceDescriptorMismatchBeforeRunner(t *testing.T) {
	fixture := newNativeFlowAdmissionFixture(t)
	mismatchInput := fixture.descriptorInput
	mismatchInput.CaseID = "case-other"
	mismatch, err := domainfundsquerysource.NewDescriptorV1(mismatchInput)
	if err != nil {
		t.Fatal(err)
	}
	runner := &nativeRunnerStub{copyFlowSource: true}
	effectCalls := 0
	releases := 0
	service := fixture.service(runner)
	service.dependencies.UseCurrentAccountFlowSource = func(
		ctx context.Context,
		securityContext domainsecurity.TurnSecurityContext,
		use func(
			context.Context,
			domainfundsquerysource.DescriptorV1,
			fundsquerysourceport.ExactReadLease,
			domainnative.AccountFlowSourceRowResolverV1,
			accountFlowPostNativeCurrentnessTestV1,
		) error,
	) error {
		if securityContext != fixture.securityContext {
			return ErrAuthorityInvalid
		}
		return use(
			ctx,
			mismatch,
			fixture.source,
			nativeFlowSourceRowResolverV1,
			nativeFlowPostNativeCurrentnessV1(fixture.securityContext, mismatch),
		)
	}
	service.dependencies.AcquireEffect = func(
		ctx context.Context,
		_ domainsecurity.TurnSecurityContext,
	) (context.Context, func(), error) {
		effectCalls++
		return ctx, func() { releases++ }, nil
	}

	consumerCalls := 0
	result, err := service.AnalyzeAccountFlows(
		context.Background(),
		fixture.input,
		func(domainnative.AccountFlowHostEvidenceProjectionV1) error {
			consumerCalls++
			return nil
		},
	)
	if !errors.Is(err, ErrAuthorityInvalid) || !reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) ||
		consumerCalls != 0 || runner.flowCalls != 0 || effectCalls != 1 || releases != 1 || fixture.source.CopyCount() != 0 {
		t.Fatalf(
			"mismatched host descriptor reached runner/evidence: result=%#v err=%v consumer=%d calls=%d effects=%d releases=%d copies=%d",
			result,
			err,
			consumerCalls,
			runner.flowCalls,
			effectCalls,
			releases,
			fixture.source.CopyCount(),
		)
	}
}

func TestNativeComponentAccountFlowSubjectCurrentnessFailureReachesNeitherNativeNorEvidence(t *testing.T) {
	tests := []struct {
		name                string
		invokeSubjectUse    bool
		wantSubjectUseCalls int
	}{
		{name: "pre lookup revoked"},
		{name: "post lookup revoked", invokeSubjectUse: true, wantSubjectUseCalls: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newNativeFlowAdmissionFixture(t)
			runner := &nativeRunnerStub{copyFlowSource: true}
			service := fixture.service(runner)
			resolverCalls := 0
			subjectUseCalls := 0
			service.dependencies.ResolveAccountFlowSubject = func(
				_ context.Context,
				_ domainsecurity.TurnSecurityContext,
				_ domainfundsquerysource.DescriptorV1,
				_ domaincaseentity.ModelEntityAliasV1,
				_ ValidateCurrentAccountFlowCallback,
				use func(AccountFlowSubjectResolutionV1) error,
			) error {
				resolverCalls++
				if test.invokeSubjectUse {
					subjectUseCalls++
					if err := use(fixture.resolution); err != nil {
						return err
					}
				}
				return caseentityapp.ErrPrivateStateUnavailable
			}
			consumerCalls := 0
			result, err := service.AnalyzeAccountFlows(
				context.Background(),
				fixture.input,
				func(domainnative.AccountFlowHostEvidenceProjectionV1) error {
					consumerCalls++
					return nil
				},
			)
			if !errors.Is(err, ErrAuthorityInvalid) ||
				!reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) ||
				resolverCalls != 1 || subjectUseCalls != test.wantSubjectUseCalls ||
				consumerCalls != 0 || runner.flowCalls != 0 || fixture.source.CopyCount() != 0 {
				t.Fatalf(
					"subject currentness failure escaped: result=%#v err=%v resolver=%d subjectUse=%d consumer=%d calls=%d copies=%d",
					result,
					err,
					resolverCalls,
					subjectUseCalls,
					consumerCalls,
					runner.flowCalls,
					fixture.source.CopyCount(),
				)
			}
		})
	}
}

func TestNativeComponentAccountFlowRevalidatesGrantAfterRunBeforeTypedResult(t *testing.T) {
	fixture := newNativeFlowAdmissionFixture(t)
	staleContext := nativeAdmissionContext(
		t,
		fixture.securityContext.ThreadID,
		"turn-flow-stale",
		fixture.now.Add(time.Second),
	)
	stale := nativeAdmissionThread(t, staleContext, fixture.grant, json.RawMessage(`{}`))
	runner := &nativeRunnerStub{copyFlowSource: true}
	service := fixture.service(runner)
	service.dependencies.Threads = &sequenceThreadReader{
		threads: []map[string]any{fixture.thread, fixture.thread, fixture.thread, fixture.thread, stale},
	}

	result, err := service.AnalyzeAccountFlows(context.Background(), fixture.input, nativeFlowEvidenceConsumerV1())
	if !errors.Is(err, ErrAuthorityInvalid) || errors.Is(err, ErrResultInvalid) ||
		!reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) || runner.flowCalls != 1 || fixture.source.CopyCount() != 1 {
		t.Fatalf(
			"stale authority did not dominate typed result: result=%#v err=%v calls=%d copies=%d",
			result,
			err,
			runner.flowCalls,
			fixture.source.CopyCount(),
		)
	}
}

func TestNativeComponentAccountFlowUnavailableDoesNotDisableHealthExecute(t *testing.T) {
	fixture := newNativeAdmissionFixture(t, "not_required", time.Minute)
	runner := &nativeRunnerStub{result: nativeReadyResult()}
	service := fixture.service(runner)
	if result, err := service.Execute(context.Background(), fixture.input); err != nil || result != nativeReadyResult() {
		t.Fatalf("health execute was disabled with flow: result=%#v err=%v", result, err)
	}
	flowFixture := newNativeFlowAdmissionFixture(t)
	result, err := service.AnalyzeAccountFlows(context.Background(), flowFixture.input, nativeFlowEvidenceConsumerV1())
	if !errors.Is(err, ErrUnavailable) || !reflect.DeepEqual(result, domainnative.AccountFlowProviderSemanticResultV1{}) || runner.calls != 1 {
		t.Fatalf("flow unavailability affected health: result=%#v err=%v healthCalls=%d", result, err, runner.calls)
	}
}

func TestNativeComponentPendingRevokedAndSettledGrantsNeverSpawn(t *testing.T) {
	for _, state := range []string{"pending", "revoked", "settled"} {
		t.Run(state, func(t *testing.T) {
			fixture := newNativeAdmissionFixture(t, map[string]string{"pending": "pending", "revoked": "pending", "settled": "not_required"}[state], time.Minute)
			turn := fixture.thread["turns"].([]any)[0].(map[string]any)
			if state == "revoked" {
				approved, err := executiongrantapp.Approve(fixture.grant, fixture.now.Add(time.Second))
				if err != nil {
					t.Fatal(err)
				}
				transitionAuthority, err := domainsecurity.NewApprovalGrantTransitionV1(domainsecurity.ApprovalGrantTransitionInputV1{
					Context: fixture.securityContext, ApprovalID: "appr_native", ApprovalItemID: "item_appr_native",
					ContinuationReceiptID:     domainsecurity.SHA256Hex([]byte("native-receipt")),
					ContinuationDispositionID: domainsecurity.SHA256Hex([]byte("native-disposition")),
					PendingGrant:              fixture.grant, ApprovedGrant: approved, TransitionedAt: fixture.now.Add(time.Second),
				})
				if err != nil {
					t.Fatal(err)
				}
				transition, _, err := executiongrantapp.ApprovalTransitionRecords(
					fixture.securityContext, transitionAuthority, fixture.grant, approved,
				)
				if err != nil {
					t.Fatal(err)
				}
				turn["items"] = append(turn["items"].([]any), transition)
			}
			if state == "settled" {
				turn["items"] = append(turn["items"].([]any), nativeResultItem(fixture, fixture.now.Add(time.Second)))
			}
			runner := &nativeRunnerStub{result: nativeReadyResult()}
			result, err := fixture.service(runner).Execute(context.Background(), fixture.input)
			if !errors.Is(err, ErrGrantInvalid) || result != (domainnative.Result{}) || runner.calls != 0 {
				t.Fatalf("state %s reached runner: result=%#v err=%v calls=%d", state, result, err, runner.calls)
			}
		})
	}
}

func TestNativeComponentRevalidatesInsideLeaseBeforeSpawn(t *testing.T) {
	fixture := newNativeAdmissionFixture(t, "not_required", time.Minute)
	stale := nativeAdmissionThread(t, nativeAdmissionContext(t, "thread-native", "turn-stale", fixture.now.Add(time.Second)), fixture.grant, fixture.payload)
	reader := &sequenceThreadReader{threads: []map[string]any{fixture.thread, stale}}
	runner := &nativeRunnerStub{result: nativeReadyResult()}
	service := NewService(Dependencies{
		Threads: reader, DurableAuthority: parsedNativeAuthority{}, LiveAuthority: nativeLiveAuthority{},
		HealthOnlyAuthority: nativeHealthOnlyAuthorityV1(),
		AcquireEffect:       nativeEffectLease, Runner: runner, Now: func() time.Time { return fixture.now },
	})

	result, err := service.Execute(context.Background(), fixture.input)
	if !errors.Is(err, ErrAuthorityInvalid) || result != (domainnative.Result{}) || runner.calls != 0 {
		t.Fatalf("stale lease admission reached runner: result=%#v err=%v calls=%d", result, err, runner.calls)
	}
}

func TestNativeComponentDiscardsLateResultAfterAuthorityChanges(t *testing.T) {
	fixture := newNativeAdmissionFixture(t, "not_required", time.Minute)
	staleContext := nativeAdmissionContext(t, fixture.securityContext.ThreadID, "turn-stale", fixture.now.Add(time.Second))
	stale := nativeAdmissionThread(t, staleContext, fixture.grant, fixture.payload)
	reader := &sequenceThreadReader{threads: []map[string]any{fixture.thread, fixture.thread, stale}}
	runner := &nativeRunnerStub{result: nativeReadyResult()}
	service := NewService(Dependencies{
		Threads: reader, DurableAuthority: parsedNativeAuthority{}, LiveAuthority: nativeLiveAuthority{},
		HealthOnlyAuthority: nativeHealthOnlyAuthorityV1(),
		AcquireEffect:       nativeEffectLease, Runner: runner, Now: func() time.Time { return fixture.now },
	})

	result, err := service.Execute(context.Background(), fixture.input)
	if !errors.Is(err, ErrAuthorityInvalid) || result != (domainnative.Result{}) || runner.calls != 1 {
		t.Fatalf("late stale result survived: result=%#v err=%v calls=%d", result, err, runner.calls)
	}
}

func TestNativeComponentDiscardsResultWhenGrantExpiresDuringExecution(t *testing.T) {
	fixture := newNativeAdmissionFixture(t, "not_required", 2*time.Second)
	clock := &sequenceClock{times: []time.Time{fixture.now, fixture.now, fixture.now.Add(3 * time.Second)}}
	runner := &nativeRunnerStub{result: nativeReadyResult()}
	service := NewService(Dependencies{
		Threads:          &sequenceThreadReader{threads: []map[string]any{fixture.thread}},
		DurableAuthority: parsedNativeAuthority{}, LiveAuthority: nativeLiveAuthority{},
		HealthOnlyAuthority: nativeHealthOnlyAuthorityV1(),
		AcquireEffect:       nativeEffectLease, Runner: runner, Now: clock.Now,
	})
	result, err := service.Execute(context.Background(), fixture.input)
	if !errors.Is(err, ErrAuthorityInvalid) || result != (domainnative.Result{}) || runner.calls != 1 {
		t.Fatalf("expired late result survived: result=%#v err=%v calls=%d", result, err, runner.calls)
	}
}

type nativeAdmissionFixture struct {
	now             time.Time
	payload         json.RawMessage
	securityContext domainsecurity.TurnSecurityContext
	grant           domainsecurity.ExecutionGrant
	thread          map[string]any
	input           ExecuteInput
}

type nativeFlowAdmissionFixture struct {
	now               time.Time
	securityContext   domainsecurity.TurnSecurityContext
	caseEntityService *caseentityapp.Service
	caseEntityStore   *nativeCaseEntityStoreStub
	arguments         domainnative.AnalyzeAccountFlowsArgumentsInputV1
	resolution        AccountFlowSubjectResolutionV1
	descriptorInput   domainfundsquerysource.DescriptorInputV1
	descriptor        domainfundsquerysource.DescriptorV1
	grant             domainsecurity.ExecutionGrant
	nativeGrant       domainsecurity.ExecutionGrant
	thread            map[string]any
	source            *nativeExactSourceStub
	resolverCalls     *int
	input             AnalyzeAccountFlowsInput
}

func newNativeFlowAdmissionFixture(t *testing.T) nativeFlowAdmissionFixture {
	t.Helper()
	now := time.Date(2026, 7, 27, 7, 0, 0, 0, time.UTC)
	currentDataset := currentdatasettest.NewHarness()
	securityContext := nativeFlowAdmissionContext(
		t,
		currentDataset,
		"thread-flow",
		"turn-flow",
		now,
	)
	caseEntityStore := newNativeCaseEntityStoreStub()
	caseEntityService := caseentityapp.NewPersistentService(
		&nativeCaseEntityDigester{key: []byte("native-flow-installation-key")},
		caseEntityStore,
		currentDataset,
		currentDataset,
		currentDataset.ValidateCurrent,
	)
	subjectRef, err := caseEntityService.BindReferenceV1(
		context.Background(),
		caseentityapp.NewDeriveReferenceInputV1(
			securityContext,
			domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			"6222021234567890123",
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	bindingRecord := caseEntityStore.bindingsByReference[subjectRef]
	argumentsInput := domainnative.NewAnalyzeAccountFlowsArgumentsInputV1(domainnative.AnalyzeAccountFlowsArgumentsInputV1{
		CaseID: securityContext.CaseID, DatasetSnapshotID: securityContext.DatasetSnapshotID,
		ContextEpoch: securityContext.ContextEpoch, ContextDigest: securityContext.ContextDigest,
		CaseBindingHash:                      securityContext.CaseBindingHash,
		ExpectedProducerContentID:            domainsecurity.FundsProducerContentIDPrefixV1 + strings.Repeat("4", 64),
		ExpectedProducerManifestSHA256:       strings.Repeat("5", 64),
		ExpectedDuckDBContentSnapshotDigest:  strings.Repeat("b", 64),
		ExpectedDuckDBSnapshotManifestSHA256: strings.Repeat("c", 64),
		ExpectedMaterializationIdentity:      "txn_daily_snapshot:v12:" + strings.Repeat("a", 64),
		SubjectAlias:                         "acct:1",
		SubjectRef:                           string(subjectRef),
		SubjectResolutionDigest:              bindingRecord.RecordDigest,
		StartInclusive:                       "2026-01-01T00:00:00.000000Z",
		EndInclusive:                         "2026-01-01T00:02:00.000000Z",
		EvidenceRowLimit:                     10,
		DatasetUTCOffsetMinutes:              0,
		ExpectedCurrency:                     "CNY",
		MinorUnitScale:                       domainnative.AccountFlowMinorUnitScaleV1,
		ScanCap:                              domainnative.AccountFlowMaximumScanRowsV1,
	}, "6222021234567890123")
	arguments, err := domainnative.NewAnalyzeAccountFlowsArgumentsV1(argumentsInput)
	if err != nil {
		t.Fatal(err)
	}
	descriptorInput := domainfundsquerysource.DescriptorInputV1{
		SnapshotRecordDigest:                   nativeFlowDigest("snapshot-record"),
		DatasetSnapshotID:                      securityContext.DatasetSnapshotID,
		SourceManifestHash:                     securityContext.SourceManifestHash,
		CaseID:                                 securityContext.CaseID,
		CaseBindingHash:                        securityContext.CaseBindingHash,
		DatasetBindingDigest:                   nativeFlowDigest("dataset-binding"),
		BindingObservationDigest:               securityContext.PublicationPolicy.BindingObservationDigest,
		FundsProducerContentID:                 argumentsInput.ExpectedProducerContentID,
		FundsProducerContentManifestSHA256:     argumentsInput.ExpectedProducerManifestSHA256,
		FundsProducerContentManifestByteLength: 1024,
		DuckDBSHA256:                           nativeFlowDigest("duckdb"),
		DuckDBByteLength:                       4096,
		DuckDBContentSnapshotDigest:            argumentsInput.ExpectedDuckDBContentSnapshotDigest,
		DuckDBSnapshotManifestSHA256:           argumentsInput.ExpectedDuckDBSnapshotManifestSHA256,
		MaterializationIdentity:                argumentsInput.ExpectedMaterializationIdentity,
		SchemaDigest:                           domainfundsquerysource.FixedFundsAnalyticalSchemaDigestV1(),
		DatasetUTCOffsetMinutes:                0,
		ExpectedCurrency:                       "CNY",
		MinorUnitScale:                         domainnative.AccountFlowMinorUnitScaleV1,
		QueryProfileDigest:                     domainfundsquerysource.FixedFundsQueryProfileDigestV1(),
	}
	descriptor, err := domainfundsquerysource.NewDescriptorV1(descriptorInput)
	if err != nil {
		t.Fatal(err)
	}
	var resolution AccountFlowSubjectResolutionV1
	resolutionCalls := 0
	err = NewPersistentAccountFlowSubjectResolver(caseEntityService)(
		context.Background(),
		securityContext,
		descriptor,
		"acct:1",
		func(context.Context, domainsecurity.TurnSecurityContext) error { return nil },
		func(value AccountFlowSubjectResolutionV1) error {
			resolutionCalls++
			resolution = value
			return nil
		},
	)
	if err != nil || resolutionCalls != 1 {
		t.Fatal(err)
	}
	policy, ok := domainnative.Policy(
		domainnative.ComponentDataEngine,
		domainnative.OperationFundsAnalyzeAccountFlows,
	)
	if !ok {
		t.Fatal("fixed flow policy missing")
	}
	const connectionEpoch = uint64(17)
	serverIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"analytix_funds",
		"analytix_funds",
		"0.16.16+evidence",
		nativeFlowDigest("connection-instance"),
		connectionEpoch,
	)
	if err != nil {
		t.Fatal(err)
	}
	toolCallID := nativeComponentTestToolCallID("flow-execute")
	intent := domainnative.AnalyzeAccountFlowsProviderIntentV1{
		SubjectAlias: argumentsInput.SubjectAlias, StartInclusive: argumentsInput.StartInclusive,
		EndInclusive: argumentsInput.EndInclusive, EvidenceRowLimit: argumentsInput.EvidenceRowLimit,
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider_1", ServerIdentity: serverIdentity,
		ToolName: fundsAnalyzeAccountFlowsToolName, ToolCallID: toolCallID,
		ConnectionEpoch: connectionEpoch, ArgsHash: domainnative.AnalyzeAccountFlowsProviderIntentHashV1(intent),
		SchemaHash: nativeFlowDigest("provider-schema"), ScopeHash: nativeFlowDigest("provider-scope"),
		ReadOnly: true, ApprovalState: "not_required", IssuedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	nativeGrant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: domainnative.NativeProvider, ServerIdentity: domainnative.NativeServerIdentity,
		ToolName: domainnative.ToolName(policy.ComponentID, policy.Operation), ToolCallID: toolCallID,
		ArgsHash: domainnative.AnalyzeAccountFlowsArgumentsHashV1(arguments), SchemaHash: policy.SchemaHash,
		ScopeHash: domainnative.ScopeHash(securityContext, policy.ComponentID, policy.Operation), ReadOnly: policy.ReadOnly,
		ApprovalState: "not_required", IssuedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	thread := nativeAdmissionThread(t, securityContext, grant, json.RawMessage(`{}`))
	turn := thread["turns"].([]any)[0].(map[string]any)
	callItem := turn["items"].([]any)[0].(map[string]any)
	callItem["arguments"] = domaintoolcall.PublicToolCallArgumentsProjectionRecordV1()
	source := newNativeExactSourceStub()
	resolverCalls := 0
	return nativeFlowAdmissionFixture{
		now: now, securityContext: securityContext, caseEntityService: caseEntityService,
		caseEntityStore: caseEntityStore, arguments: argumentsInput,
		descriptorInput: descriptorInput, descriptor: descriptor, grant: grant, nativeGrant: nativeGrant,
		thread: thread, source: source, resolverCalls: &resolverCalls, resolution: resolution,
		input: AnalyzeAccountFlowsInput{
			ExecuteInput: ExecuteInput{
				ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, GrantID: grant.GrantID,
			},
			Intent: intent,
		},
	}
}

func (fixture nativeFlowAdmissionFixture) service(runner *nativeRunnerStub) *Service {
	persistentResolver := NewPersistentAccountFlowSubjectResolver(fixture.caseEntityService)
	persistentCounterpartyResolver := NewPersistentAccountFlowCounterpartyResolver(fixture.caseEntityService)
	return NewService(Dependencies{
		Threads: &sequenceThreadReader{threads: []map[string]any{fixture.thread}}, DurableAuthority: parsedNativeAuthority{},
		LiveAuthority: nativeLiveAuthority{}, AcquireEffect: nativeEffectLease,
		Runner: runner, AccountFlowRunner: runner,
		UseCurrentAccountFlowSource: func(
			ctx context.Context,
			securityContext domainsecurity.TurnSecurityContext,
			use func(
				context.Context,
				domainfundsquerysource.DescriptorV1,
				fundsquerysourceport.ExactReadLease,
				domainnative.AccountFlowSourceRowResolverV1,
				accountFlowPostNativeCurrentnessTestV1,
			) error,
		) error {
			if securityContext != fixture.securityContext {
				return ErrAuthorityInvalid
			}
			return use(
				ctx,
				fixture.descriptor,
				fixture.source,
				func(sourceFileID string, sourceRowNumber uint64, consume func(string) error) error {
					(*fixture.resolverCalls)++
					return nativeFlowSourceRowResolverV1(sourceFileID, sourceRowNumber, consume)
				},
				nativeFlowPostNativeCurrentnessV1(securityContext, fixture.descriptor),
			)
		},
		ResolveAccountFlowSubject: func(
			ctx context.Context,
			securityContext domainsecurity.TurnSecurityContext,
			descriptor domainfundsquerysource.DescriptorV1,
			alias domaincaseentity.ModelEntityAliasV1,
			validateCurrent ValidateCurrentAccountFlowCallback,
			use func(AccountFlowSubjectResolutionV1) error,
		) error {
			return persistentResolver(ctx, securityContext, descriptor, alias, validateCurrent, use)
		},
		ResolveAccountFlowCounterparty: persistentCounterpartyResolver,
		ValidateCurrentAccountFlowCallback: func(
			_ context.Context,
			securityContext domainsecurity.TurnSecurityContext,
		) error {
			if securityContext != fixture.securityContext {
				return ErrAuthorityInvalid
			}
			return nil
		},
		ValidateCurrentAccountFlowOuterGrant: func(
			_ context.Context,
			securityContext domainsecurity.TurnSecurityContext,
			grant domainsecurity.ExecutionGrant,
		) error {
			if securityContext != fixture.securityContext || grant != fixture.grant {
				return ErrGrantInvalid
			}
			return nil
		},
		Now: func() time.Time { return fixture.now },
	})
}

func nativeFlowDigest(material string) string {
	return domainsecurity.SHA256Hex([]byte("native-flow-admission-test:\x00" + material))
}

func nativeFlowPostNativeCurrentnessV1(
	securityContext domainsecurity.TurnSecurityContext,
	descriptor domainfundsquerysource.DescriptorV1,
) accountFlowPostNativeCurrentnessTestV1 {
	var mu sync.Mutex
	used := false
	return func(
		validationContext context.Context,
		currentContext domainsecurity.TurnSecurityContext,
		currentDescriptor domainfundsquerysource.DescriptorV1,
		continueValidation func(context.Context) error,
	) error {
		mu.Lock()
		if used || validationContext == nil || continueValidation == nil ||
			currentContext != securityContext || currentDescriptor != descriptor {
			mu.Unlock()
			return ErrAuthorityInvalid
		}
		used = true
		mu.Unlock()
		if err := validationContext.Err(); err != nil {
			return err
		}
		return continueValidation(validationContext)
	}
}

func nativeFlowSourceRowResolverV1(
	sourceFileID string,
	sourceRowNumber uint64,
	consume func(string) error,
) error {
	if sourceFileID == "" || sourceRowNumber == 0 || consume == nil {
		return ErrAuthorityInvalid
	}
	return consume(
		"srow1_" + nativeFlowDigest(fmt.Sprintf("%s:%d", sourceFileID, sourceRowNumber)),
	)
}

func nativeFlowEvidenceConsumerV1() domainnative.AccountFlowHostEvidenceProjectionConsumerV1 {
	return func(projection domainnative.AccountFlowHostEvidenceProjectionV1) error {
		return projection.UseExactV1(
			func(
				string,
				string,
				uint64,
				string,
				string,
				string,
				string,
				string,
				string,
				uint8,
				string,
				string,
				string,
				uint64,
				bool,
				bool,
				uint64,
				domainnative.AccountFlowProviderSemanticCoverageV1,
				string,
				string,
			) error {
				return nil
			},
			func(int, string, string, string, uint64, string, string, string, string, uint8) error {
				return nil
			},
		)
	}
}

func newNativeAdmissionFixture(t *testing.T, approval string, ttl time.Duration) nativeAdmissionFixture {
	t.Helper()
	now := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	securityContext := nativeAdmissionContext(t, "thread-native", "turn-native", now)
	payload := json.RawMessage(`{}`)
	policy, ok := domainnative.Policy(domainnative.ComponentDataEngine, "health")
	if !ok {
		t.Fatal("native health policy missing")
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: domainnative.NativeProvider, ServerIdentity: domainnative.NativeServerIdentity,
		ToolName: domainnative.ToolName(policy.ComponentID, policy.Operation), ToolCallID: nativeComponentTestToolCallID("health"),
		ArgsHash: domainsecurity.CanonicalJSONHash(payload), SchemaHash: policy.SchemaHash,
		ScopeHash: domainnative.ScopeHash(securityContext, policy.ComponentID, policy.Operation), ReadOnly: policy.ReadOnly,
		ApprovalState: approval, IssuedAt: now, ExpiresAt: now.Add(ttl),
	})
	thread := nativeAdmissionThread(t, securityContext, grant, payload)
	return nativeAdmissionFixture{
		now: now, payload: payload, securityContext: securityContext, grant: grant, thread: thread,
		input: ExecuteInput{ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, GrantID: grant.GrantID},
	}
}

func (fixture nativeAdmissionFixture) service(runner *nativeRunnerStub) *Service {
	return NewService(Dependencies{
		Threads:          &sequenceThreadReader{threads: []map[string]any{fixture.thread}},
		DurableAuthority: parsedNativeAuthority{}, LiveAuthority: nativeLiveAuthority{},
		HealthOnlyAuthority: nativeHealthOnlyAuthorityV1(),
		AcquireEffect:       nativeEffectLease, Runner: runner, Now: func() time.Time { return fixture.now },
	})
}

func nativeAdmissionContext(t *testing.T, threadID, turnID string, now time.Time) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := securitytest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/cases/native", CaseID: "case-native",
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("binding-native")),
		DatasetSnapshotID:  securitytest.DatasetSnapshotID("snapshot-native"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-native")), ContextEpoch: 1, IssuedAt: now,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func nativeFlowAdmissionContext(
	t *testing.T,
	harness *currentdatasettest.Harness,
	threadID string,
	turnID string,
	now time.Time,
) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := harness.NewCaseContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/cases/native", CaseID: "case-native",
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("binding-native")),
		DatasetSnapshotID:  securitytest.DatasetSnapshotID("snapshot-native"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-native")), ContextEpoch: 1, IssuedAt: now,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func nativeAdmissionThread(t *testing.T, securityContext domainsecurity.TurnSecurityContext, grant domainsecurity.ExecutionGrant, payload json.RawMessage) map[string]any {
	t.Helper()
	contextRecord := turnsecurityapp.PublicRecord(securityContext)
	grantBody, _ := json.Marshal(grant)
	grantRecord := map[string]any{}
	if err := json.Unmarshal(grantBody, &grantRecord); err != nil {
		t.Fatal(err)
	}
	arguments := map[string]any{}
	if err := json.Unmarshal(payload, &arguments); err != nil {
		t.Fatal(err)
	}
	return map[string]any{
		"id": securityContext.ThreadID, "workspace": securityContext.WorkspaceRealPath, "securityState": contextRecord,
		"turns": []any{map[string]any{
			"id": securityContext.TurnID, "threadId": securityContext.ThreadID, "status": "running", "securityContext": contextRecord,
			"items": []any{map[string]any{
				"id": domaintoolcall.ToolCallItemIDV1(securityContext.TurnID, grant.ToolCallID), "kind": "tool_call", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
				"toolName": grant.ToolName, "callId": grant.ToolCallID, "arguments": arguments, "createdAt": grant.IssuedAt,
				"contextDigest": securityContext.ContextDigest, "contextEpoch": float64(securityContext.ContextEpoch),
				"executionGrantId": grant.GrantID, "executionGrant": grantRecord,
			}},
		}},
	}
}

func nativeResultItem(fixture nativeAdmissionFixture, at time.Time) map[string]any {
	return map[string]any{
		"id": "item-native-result", "kind": "tool_result", "role": "tool", "status": "completed",
		"threadId": fixture.securityContext.ThreadID, "turnId": fixture.securityContext.TurnID,
		"toolName": fixture.grant.ToolName, "callId": fixture.grant.ToolCallID,
		"createdAt": at.Format(time.RFC3339Nano), "finishedAt": at.Format(time.RFC3339Nano),
		"contextDigest": fixture.securityContext.ContextDigest, "contextEpoch": float64(fixture.securityContext.ContextEpoch),
		"executionGrantId": fixture.grant.GrantID,
	}
}

func nativeReadyResult() domainnative.Result {
	return domainnative.Result{
		SchemaVersion: domainnative.ResultSchemaVersion, ComponentID: domainnative.ComponentDataEngine,
		Operation: "health", Status: "ready", RegistryDigest: domainsecurity.SHA256Hex([]byte("native-registry")),
	}
}

type sequenceThreadReader struct {
	mu      sync.Mutex
	threads []map[string]any
	reads   int
}

func (reader *sequenceThreadReader) GetThread(string) (map[string]any, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	if len(reader.threads) == 0 {
		return nil, errors.New("thread unavailable")
	}
	index := reader.reads
	if index >= len(reader.threads) {
		index = len(reader.threads) - 1
	}
	reader.reads++
	return reader.threads[index], nil
}

type parsedNativeAuthority struct{}

func (parsedNativeAuthority) ValidateCurrent(_ string, thread map[string]any) (domainsecurity.TurnSecurityContext, error) {
	return domainsecurity.ParseTurnSecurityContext(thread["securityState"])
}

type nativeLiveAuthority struct{ err error }

func (authority nativeLiveAuthority) ValidateCurrent(context.Context, domainsecurity.TurnSecurityContext) error {
	return authority.err
}

func nativeHealthOnlyAuthorityV1() LiveAuthority {
	return NewHealthOnlyCurrentnessV1(
		nativeLiveAuthority{},
		func(context.Context) error { return nil },
	)
}

func nativeEffectLease(ctx context.Context, _ domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
	return ctx, func() {}, nil
}

type nativeRunnerStub struct {
	calls          int
	flowCalls      int
	flowStarted    chan struct{}
	flowRelease    <-chan struct{}
	request        domainnative.Request
	result         domainnative.Result
	flowResult     domainnative.AnalyzeAccountFlowsResultV1
	flowDescriptor domainfundsquerysource.DescriptorV1
	flowSource     fundsquerysourceport.ExactReadLease
	copyFlowSource bool
	err            error
	flowErr        error
	closeFn        func() error
	onExecute      func()
}

func (runner *nativeRunnerStub) Close() error {
	if runner.closeFn != nil {
		return runner.closeFn()
	}
	return nil
}

func (runner *nativeRunnerStub) Execute(_ context.Context, request domainnative.Request) (domainnative.Result, error) {
	runner.calls++
	runner.request = request
	if runner.onExecute != nil {
		runner.onExecute()
	}
	return runner.result, runner.err
}

func (runner *nativeRunnerStub) AnalyzeAccountFlows(
	ctx context.Context,
	request domainnative.Request,
	descriptor domainfundsquerysource.DescriptorV1,
	source fundsquerysourceport.ExactReadLease,
) (domainnative.AnalyzeAccountFlowsResultV1, error) {
	runner.flowCalls++
	if runner.flowStarted != nil {
		close(runner.flowStarted)
	}
	if runner.flowRelease != nil {
		<-runner.flowRelease
	}
	runner.request = request
	runner.flowDescriptor = descriptor
	runner.flowSource = source
	if runner.copyFlowSource {
		destination, err := fundsquerysourcefixture.NewUnlinkedDestinationV1("")
		if err != nil {
			return domainnative.AnalyzeAccountFlowsResultV1{}, err
		}
		defer destination.Close()
		if err := source.CopyExactTo(ctx, destination); err != nil {
			return domainnative.AnalyzeAccountFlowsResultV1{}, err
		}
	}
	return runner.flowResult, runner.flowErr
}

type nativeExactSourceStub struct {
	*fundsquerysourcefixture.ExactReadLeaseV1
	copies int
}

func newNativeExactSourceStub() *nativeExactSourceStub {
	source := &nativeExactSourceStub{}
	source.ExactReadLeaseV1 = fundsquerysourcefixture.NewExactReadLeaseV1WithCounter(
		[]byte("exact-duckdb"),
		&source.copies,
	)
	return source
}

type nativeCaseEntityDigester struct {
	key []byte
}

func (digester *nativeCaseEntityDigester) KeyedPayloadHash(
	_ context.Context,
	purpose string,
	payload []byte,
) (string, error) {
	mac := hmac.New(sha256.New, digester.key)
	_, _ = mac.Write([]byte(purpose))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

type nativeCaseEntityStoreStub struct {
	bindings            map[string]domaincaseentity.CaseEntityBindingRecord
	bindingsByReference map[domaincaseentity.ReferenceV1]domaincaseentity.CaseEntityBindingRecord
	ingress             map[string]domaincaseentity.CaseIngressRecord
	threadContext       map[string]domaincaseentity.ThreadCaseContextRecord
	onResolveBinding    func(context.Context)
}

func newNativeCaseEntityStoreStub() *nativeCaseEntityStoreStub {
	return &nativeCaseEntityStoreStub{
		bindings:            make(map[string]domaincaseentity.CaseEntityBindingRecord),
		bindingsByReference: make(map[domaincaseentity.ReferenceV1]domaincaseentity.CaseEntityBindingRecord),
		ingress:             make(map[string]domaincaseentity.CaseIngressRecord),
		threadContext:       make(map[string]domaincaseentity.ThreadCaseContextRecord),
	}
}

func (store *nativeCaseEntityStoreStub) EnsureBinding(
	_ context.Context,
	input domaincaseentity.CaseEntityBindingRecordInputV1,
) (domaincaseentity.CaseEntityBindingRecord, error) {
	key, err := domaincaseentity.CaseEntityBindingLookupKeyV1(input.SecurityContext, input.EntityType, input.Reference)
	if err != nil {
		return domaincaseentity.CaseEntityBindingRecord{}, caseentityport.ErrIntegrity
	}
	if current, found := store.bindings[key]; found {
		expected, expectedErr := domaincaseentity.NewCaseEntityBindingRecordV1(
			domaincaseentity.WithCaseEntityBindingStableOrdinalV1(input, current.StableOrdinal),
		)
		if expectedErr == nil && current.RecordDigest == expected.RecordDigest {
			return current, nil
		}
		return domaincaseentity.CaseEntityBindingRecord{}, caseentityport.ErrConflict
	}
	var maxOrdinal uint32
	for _, current := range store.bindings {
		if current.TenantID == input.SecurityContext.TenantID && current.UserID == input.SecurityContext.UserID &&
			current.CaseID == input.SecurityContext.CaseID && current.CaseBindingHash == input.SecurityContext.CaseBindingHash &&
			current.StableOrdinal > maxOrdinal {
			maxOrdinal = current.StableOrdinal
		}
	}
	record, err := domaincaseentity.NewCaseEntityBindingRecordV1(
		domaincaseentity.WithCaseEntityBindingStableOrdinalV1(input, maxOrdinal+1),
	)
	if err != nil {
		return domaincaseentity.CaseEntityBindingRecord{}, caseentityport.ErrIntegrity
	}
	store.bindings[record.BindingKey] = record
	store.bindingsByReference[record.Reference] = record
	return record, nil
}

func (store *nativeCaseEntityStoreStub) ResolveBinding(
	ctx context.Context,
	key string,
) (domaincaseentity.CaseEntityBindingRecord, error) {
	if store.onResolveBinding != nil {
		store.onResolveBinding(ctx)
	}
	record, found := store.bindings[key]
	if !found {
		return domaincaseentity.CaseEntityBindingRecord{}, caseentityport.ErrNotFound
	}
	return record, nil
}

func (store *nativeCaseEntityStoreStub) ResolveBindingByStableOrdinal(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	entityType string,
	stableOrdinal uint32,
) (domaincaseentity.CaseEntityBindingRecord, error) {
	if store.onResolveBinding != nil {
		store.onResolveBinding(ctx)
	}
	var resolved domaincaseentity.CaseEntityBindingRecord
	for _, record := range store.bindings {
		if record.TenantID != securityContext.TenantID || record.UserID != securityContext.UserID ||
			record.CaseID != securityContext.CaseID || record.CaseBindingHash != securityContext.CaseBindingHash ||
			record.EntityType != entityType || record.StableOrdinal != stableOrdinal {
			continue
		}
		if resolved.RecordDigest != "" {
			return domaincaseentity.CaseEntityBindingRecord{}, caseentityport.ErrIntegrity
		}
		resolved = record
	}
	if resolved.RecordDigest == "" {
		return domaincaseentity.CaseEntityBindingRecord{}, caseentityport.ErrNotFound
	}
	return resolved, nil
}

func (store *nativeCaseEntityStoreStub) PutIngressIfAbsent(
	_ context.Context,
	record domaincaseentity.CaseIngressRecord,
) error {
	if current, found := store.ingress[record.IngressID]; found {
		if current.RecordDigest == record.RecordDigest {
			return nil
		}
		return caseentityport.ErrConflict
	}
	store.ingress[record.IngressID] = record
	return nil
}

func (store *nativeCaseEntityStoreStub) ResolveIngress(
	_ context.Context,
	key string,
) (domaincaseentity.CaseIngressRecord, error) {
	record, found := store.ingress[key]
	if !found {
		return domaincaseentity.CaseIngressRecord{}, caseentityport.ErrNotFound
	}
	return record, nil
}

func (store *nativeCaseEntityStoreStub) PutThreadContextIfAbsent(
	_ context.Context,
	record domaincaseentity.ThreadCaseContextRecord,
) error {
	if current, found := store.threadContext[record.StorageKey]; found {
		if current.RecordDigest == record.RecordDigest {
			return nil
		}
		return caseentityport.ErrConflict
	}
	store.threadContext[record.StorageKey] = record
	return nil
}

func (store *nativeCaseEntityStoreStub) ResolveThreadContext(
	_ context.Context,
	key string,
) (domaincaseentity.ThreadCaseContextRecord, error) {
	record, found := store.threadContext[key]
	if !found {
		return domaincaseentity.ThreadCaseContextRecord{}, caseentityport.ErrNotFound
	}
	return record, nil
}

func (store *nativeCaseEntityStoreStub) ResolveLatestCaseLongitudinalContext(
	_ context.Context,
	securityContext domainsecurity.TurnSecurityContext,
) (domaincaseentity.ThreadCaseContextRecord, error) {
	return store.latestThreadContextForScopeV1(securityContext, "", true)
}

func (store *nativeCaseEntityStoreStub) ResolveLatestThreadContextForScope(
	_ context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	threadID string,
) (domaincaseentity.ThreadCaseContextRecord, error) {
	return store.latestThreadContextForScopeV1(securityContext, threadID, false)
}

func (store *nativeCaseEntityStoreStub) latestThreadContextForScopeV1(
	securityContext domainsecurity.TurnSecurityContext,
	threadID string,
	caseLongitudinal bool,
) (domaincaseentity.ThreadCaseContextRecord, error) {
	var latest domaincaseentity.ThreadCaseContextRecord
	for _, record := range store.threadContext {
		if record.TenantID != securityContext.TenantID || record.UserID != securityContext.UserID ||
			record.CaseID != securityContext.CaseID || record.CaseBindingHash != securityContext.CaseBindingHash ||
			domaincaseentity.IsCaseLongitudinalIndexRecordV1(record) != caseLongitudinal ||
			(!caseLongitudinal && record.ThreadID != threadID) {
			continue
		}
		if record.Generation > latest.Generation {
			latest = record
		}
	}
	if latest.Generation == 0 {
		return domaincaseentity.ThreadCaseContextRecord{}, caseentityport.ErrNotFound
	}
	return latest, nil
}

type nativeFlowCoverageWire struct {
	State                  string   `json:"state"`
	Gaps                   []string `json:"gaps"`
	NormalizedSnapshotRows uint64   `json:"normalizedSnapshotRows"`
	AcceptedSnapshotRows   uint64   `json:"acceptedSnapshotRows"`
	RejectedSnapshotRows   uint64   `json:"rejectedSnapshotRows"`
	DuplicateSnapshotRows  uint64   `json:"duplicateSnapshotRows"`
	UntimedSubjectRows     uint64   `json:"untimedSubjectRows"`
	ObservedMatchingRows   uint64   `json:"observedMatchingRows"`
}

type nativeFlowProvenanceWire struct {
	DatasetSnapshotID              string `json:"datasetSnapshotId"`
	ContextEpoch                   uint64 `json:"contextEpoch"`
	ContextDigest                  string `json:"contextDigest"`
	CaseBindingHash                string `json:"caseBindingHash"`
	ExpectedProducerContentID      string `json:"expectedProducerContentId"`
	ExpectedProducerManifestSHA256 string `json:"expectedProducerManifestSha256"`
	SubjectResolutionDigest        string `json:"subjectResolutionDigest"`
	DuckDBContentSnapshotDigest    string `json:"duckdbContentSnapshotDigest"`
	DuckDBSnapshotManifestSHA256   string `json:"duckdbSnapshotManifestSha256"`
	MaterializationIdentity        string `json:"materializationIdentity"`
	SourceSignature                string `json:"sourceSignature"`
	ResultSignature                string `json:"resultSignature"`
	ProducerContentID              string `json:"producerContentId"`
	ProducerManifestSHA256         string `json:"producerManifestSha256"`
	QueryContract                  string `json:"queryContract"`
	QuerySQLHash                   string `json:"querySqlHash"`
}

type nativeFlowEvidenceWire struct {
	SubjectRef       string  `json:"subjectRef"`
	SourceFileID     string  `json:"sourceFileId"`
	SourceRowNumber  uint64  `json:"sourceRowNumber"`
	CounterpartyKey  *string `json:"counterpartyKey"`
	CounterpartyName *string `json:"counterpartyName"`
	CounterpartyBank *string `json:"counterpartyBank"`
	OccurredAt       string  `json:"occurredAt"`
	Direction        string  `json:"direction"`
	AmountMinor      string  `json:"amountMinor"`
	Currency         string  `json:"currency"`
	MinorUnitScale   uint8   `json:"minorUnitScale"`
}

type nativeFlowResultWire struct {
	SubjectRef              string                   `json:"subjectRef"`
	StartInclusive          string                   `json:"startInclusive"`
	EndInclusive            string                   `json:"endInclusive"`
	Timezone                string                   `json:"timezone"`
	Currency                string                   `json:"currency"`
	MinorUnitScale          uint8                    `json:"minorUnitScale"`
	InflowMinor             string                   `json:"inflowMinor"`
	OutflowMinor            string                   `json:"outflowMinor"`
	NetMinor                string                   `json:"netMinor"`
	TransactionCount        uint64                   `json:"transactionCount"`
	AggregateComplete       bool                     `json:"aggregateComplete"`
	EvidenceRowsComplete    bool                     `json:"evidenceRowsComplete"`
	Coverage                nativeFlowCoverageWire   `json:"coverage"`
	Provenance              nativeFlowProvenanceWire `json:"provenance"`
	Currentness             string                   `json:"currentness"`
	SemanticProjectionState string                   `json:"semanticProjectionState"`
	EvidenceRows            []nativeFlowEvidenceWire `json:"evidenceRows"`
	QueryHash               string                   `json:"queryHash"`
	ResultHash              string                   `json:"resultHash"`
}

func nativeFlowValidNoHitResult(
	t *testing.T,
	input domainnative.AnalyzeAccountFlowsArgumentsInputV1,
) domainnative.AnalyzeAccountFlowsResultV1 {
	return nativeFlowValidResult(t, input, false)
}

func nativeFlowValidEvidenceResult(
	t *testing.T,
	input domainnative.AnalyzeAccountFlowsArgumentsInputV1,
) domainnative.AnalyzeAccountFlowsResultV1 {
	return nativeFlowValidResult(t, input, true)
}

func nativeFlowValidResult(
	t *testing.T,
	input domainnative.AnalyzeAccountFlowsArgumentsInputV1,
	withEvidence bool,
) domainnative.AnalyzeAccountFlowsResultV1 {
	t.Helper()
	start, err := time.Parse(time.RFC3339Nano, input.StartInclusive)
	if err != nil {
		t.Fatal(err)
	}
	end, err := time.Parse(time.RFC3339Nano, input.EndInclusive)
	if err != nil {
		t.Fatal(err)
	}
	startInclusive := start.UTC().Format("2006-01-02T15:04:05.000000Z")
	endInclusive := end.UTC().Format("2006-01-02T15:04:05.000000Z")
	sourceSignature := strings.TrimPrefix(input.ExpectedMaterializationIdentity, "txn_daily_snapshot:v12:")
	evidenceRows := []nativeFlowEvidenceWire{}
	coverage := domainnative.AccountFlowCoverageV1{
		State: domainnative.AccountFlowCoverageObservedNoHitPendingBindingV1,
		Gaps:  []string{},
	}
	inflowMinor := "0"
	outflowMinor := "0"
	netMinor := "0"
	transactionCount := uint64(0)
	if withEvidence {
		counterpartyKey := "6217009876543210987"
		counterpartyName := "Private A&B Counterparty"
		counterpartyBank := "Analytix Test Bank"
		inflowMinor = "1000"
		outflowMinor = "300"
		netMinor = "700"
		transactionCount = 2
		coverage = domainnative.AccountFlowCoverageV1{
			State:                  domainnative.AccountFlowCoverageCompleteV1,
			Gaps:                   []string{},
			NormalizedSnapshotRows: 2,
			AcceptedSnapshotRows:   2,
			ObservedMatchingRows:   2,
		}
		evidenceRows = []nativeFlowEvidenceWire{
			{
				SubjectRef: input.SubjectRef, SourceFileID: "private-source:/cases/secret.duckdb", SourceRowNumber: 1001,
				CounterpartyKey: &counterpartyKey, CounterpartyName: &counterpartyName,
				CounterpartyBank: &counterpartyBank,
				OccurredAt:       "2026-01-01T00:00:00.000000Z",
				Direction:        domainnative.AccountFlowDirectionInflowV1,
				AmountMinor:      "1000", Currency: input.ExpectedCurrency, MinorUnitScale: input.MinorUnitScale,
			},
			{
				SubjectRef: input.SubjectRef, SourceFileID: "private-source:/cases/secret.duckdb", SourceRowNumber: 1002,
				CounterpartyKey: &counterpartyKey, CounterpartyName: &counterpartyName,
				CounterpartyBank: &counterpartyBank,
				OccurredAt:       "2026-01-01T00:01:00.000000Z",
				Direction:        domainnative.AccountFlowDirectionOutflowV1,
				AmountMinor:      "300", Currency: input.ExpectedCurrency, MinorUnitScale: input.MinorUnitScale,
			},
		}
	}
	result := domainnative.AnalyzeAccountFlowsResultV1{
		SubjectRef: input.SubjectRef, StartInclusive: startInclusive, EndInclusive: endInclusive,
		Timezone: nativeFlowTimezone(input.DatasetUTCOffsetMinutes), Currency: input.ExpectedCurrency,
		MinorUnitScale: input.MinorUnitScale, InflowMinor: inflowMinor, OutflowMinor: outflowMinor,
		NetMinor: netMinor, TransactionCount: transactionCount, AggregateComplete: true, EvidenceRowsComplete: true,
		Coverage: coverage,
		Provenance: domainnative.AccountFlowProvenanceV1{
			DatasetSnapshotID: input.DatasetSnapshotID, ContextEpoch: input.ContextEpoch,
			ContextDigest: input.ContextDigest, CaseBindingHash: input.CaseBindingHash,
			ExpectedProducerContentID:      input.ExpectedProducerContentID,
			ExpectedProducerManifestSHA256: input.ExpectedProducerManifestSHA256,
			SubjectResolutionDigest:        input.SubjectResolutionDigest,
			DuckDBContentSnapshotDigest:    input.ExpectedDuckDBContentSnapshotDigest,
			DuckDBSnapshotManifestSHA256:   input.ExpectedDuckDBSnapshotManifestSHA256,
			MaterializationIdentity:        input.ExpectedMaterializationIdentity,
			SourceSignature:                sourceSignature,
			ResultSignature:                strings.Repeat("d", 64),
			ProducerContentID:              input.ExpectedProducerContentID,
			ProducerManifestSHA256:         input.ExpectedProducerManifestSHA256,
			QueryContract:                  domainnative.AccountFlowQueryContractV1,
			QuerySQLHash:                   "d06b98ac697a24f6cca52d33ead6839ad9d2eed6ce9147f967d1a0a4e79a5b51",
		},
		Currentness:             domainnative.AccountFlowCurrentnessHostRevalidationRequiredV1,
		SemanticProjectionState: domainnative.AccountFlowSemanticHostResolutionRequiredV1,
	}
	queryBody := nativeFlowJSON(t, struct {
		CaseBindingHash                string `json:"caseBindingHash"`
		ContextDigest                  string `json:"contextDigest"`
		ContextEpoch                   uint64 `json:"contextEpoch"`
		Contract                       string `json:"contract"`
		DatasetSnapshotID              string `json:"datasetSnapshotId"`
		DatasetUTCOffsetMinutes        int16  `json:"datasetUtcOffsetMinutes"`
		EndInclusive                   string `json:"endInclusive"`
		EvidenceRowLimit               uint32 `json:"evidenceRowLimit"`
		ExpectedCurrency               string `json:"expectedCurrency"`
		ExpectedProducerContentID      string `json:"expectedProducerContentId"`
		ExpectedProducerManifestSHA256 string `json:"expectedProducerManifestSha256"`
		MinorUnitScale                 uint8  `json:"minorUnitScale"`
		QuerySQLHash                   string `json:"querySqlHash"`
		ScanCap                        uint32 `json:"scanCap"`
		StartInclusive                 string `json:"startInclusive"`
		SubjectRef                     string `json:"subjectRef"`
		SubjectResolutionDigest        string `json:"subjectResolutionDigest"`
	}{
		CaseBindingHash: input.CaseBindingHash, ContextDigest: input.ContextDigest,
		ContextEpoch: input.ContextEpoch, Contract: domainnative.AccountFlowQueryContractV1,
		DatasetSnapshotID: input.DatasetSnapshotID, DatasetUTCOffsetMinutes: input.DatasetUTCOffsetMinutes,
		EndInclusive: endInclusive, EvidenceRowLimit: input.EvidenceRowLimit,
		ExpectedCurrency: input.ExpectedCurrency, ExpectedProducerContentID: input.ExpectedProducerContentID,
		ExpectedProducerManifestSHA256: input.ExpectedProducerManifestSHA256, MinorUnitScale: input.MinorUnitScale,
		QuerySQLHash: result.Provenance.QuerySQLHash, ScanCap: input.ScanCap, StartInclusive: startInclusive,
		SubjectRef: input.SubjectRef, SubjectResolutionDigest: input.SubjectResolutionDigest,
	})
	result.QueryHash = nativeFlowFramedHash(
		"analytix.account-flow-query-hash/v1",
		[]byte(result.Provenance.DuckDBContentSnapshotDigest),
		queryBody,
	)
	coverageWire := nativeFlowCoverageWire{
		State: result.Coverage.State, Gaps: result.Coverage.Gaps,
		NormalizedSnapshotRows: result.Coverage.NormalizedSnapshotRows,
		AcceptedSnapshotRows:   result.Coverage.AcceptedSnapshotRows,
		RejectedSnapshotRows:   result.Coverage.RejectedSnapshotRows,
		DuplicateSnapshotRows:  result.Coverage.DuplicateSnapshotRows,
		UntimedSubjectRows:     result.Coverage.UntimedSubjectRows,
		ObservedMatchingRows:   result.Coverage.ObservedMatchingRows,
	}
	provenance := nativeFlowProvenanceWire{
		DatasetSnapshotID: result.Provenance.DatasetSnapshotID, ContextEpoch: result.Provenance.ContextEpoch,
		ContextDigest: result.Provenance.ContextDigest, CaseBindingHash: result.Provenance.CaseBindingHash,
		ExpectedProducerContentID:      result.Provenance.ExpectedProducerContentID,
		ExpectedProducerManifestSHA256: result.Provenance.ExpectedProducerManifestSHA256,
		SubjectResolutionDigest:        result.Provenance.SubjectResolutionDigest,
		DuckDBContentSnapshotDigest:    result.Provenance.DuckDBContentSnapshotDigest,
		DuckDBSnapshotManifestSHA256:   result.Provenance.DuckDBSnapshotManifestSHA256,
		MaterializationIdentity:        result.Provenance.MaterializationIdentity,
		SourceSignature:                result.Provenance.SourceSignature,
		ResultSignature:                result.Provenance.ResultSignature,
		ProducerContentID:              result.Provenance.ProducerContentID,
		ProducerManifestSHA256:         result.Provenance.ProducerManifestSHA256,
		QueryContract:                  result.Provenance.QueryContract,
		QuerySQLHash:                   result.Provenance.QuerySQLHash,
	}
	resultBody := nativeFlowJSON(t, struct {
		SubjectRef              string                   `json:"subjectRef"`
		StartInclusive          string                   `json:"startInclusive"`
		EndInclusive            string                   `json:"endInclusive"`
		Timezone                string                   `json:"timezone"`
		Currency                string                   `json:"currency"`
		MinorUnitScale          uint8                    `json:"minorUnitScale"`
		InflowMinor             string                   `json:"inflowMinor"`
		OutflowMinor            string                   `json:"outflowMinor"`
		NetMinor                string                   `json:"netMinor"`
		TransactionCount        uint64                   `json:"transactionCount"`
		AggregateComplete       bool                     `json:"aggregateComplete"`
		EvidenceRowsComplete    bool                     `json:"evidenceRowsComplete"`
		Coverage                nativeFlowCoverageWire   `json:"coverage"`
		Provenance              nativeFlowProvenanceWire `json:"provenance"`
		Currentness             string                   `json:"currentness"`
		SemanticProjectionState string                   `json:"semanticProjectionState"`
		EvidenceRows            []nativeFlowEvidenceWire `json:"evidenceRows"`
		QueryHash               string                   `json:"queryHash"`
	}{
		SubjectRef: result.SubjectRef, StartInclusive: result.StartInclusive, EndInclusive: result.EndInclusive,
		Timezone: result.Timezone, Currency: result.Currency, MinorUnitScale: result.MinorUnitScale,
		InflowMinor: result.InflowMinor, OutflowMinor: result.OutflowMinor, NetMinor: result.NetMinor,
		TransactionCount: result.TransactionCount, AggregateComplete: result.AggregateComplete,
		EvidenceRowsComplete: result.EvidenceRowsComplete, Coverage: coverageWire, Provenance: provenance,
		Currentness: result.Currentness, SemanticProjectionState: result.SemanticProjectionState,
		EvidenceRows: evidenceRows, QueryHash: result.QueryHash,
	})
	result.ResultHash = nativeFlowFramedHash("analytix.account-flow-result-hash/v1", resultBody)
	arguments, err := domainnative.NewAnalyzeAccountFlowsArgumentsV1(input)
	if err != nil {
		t.Fatal("test typed result arguments are invalid")
	}
	parsed, err := domainnative.ParseAnalyzeAccountFlowsResultV1(
		nativeFlowJSON(t, nativeFlowResultWire{
			SubjectRef: result.SubjectRef, StartInclusive: result.StartInclusive, EndInclusive: result.EndInclusive,
			Timezone: result.Timezone, Currency: result.Currency, MinorUnitScale: result.MinorUnitScale,
			InflowMinor: result.InflowMinor, OutflowMinor: result.OutflowMinor, NetMinor: result.NetMinor,
			TransactionCount: result.TransactionCount, AggregateComplete: result.AggregateComplete,
			EvidenceRowsComplete: result.EvidenceRowsComplete, Coverage: coverageWire, Provenance: provenance,
			Currentness: result.Currentness, SemanticProjectionState: result.SemanticProjectionState,
			EvidenceRows: evidenceRows, QueryHash: result.QueryHash, ResultHash: result.ResultHash,
		}),
		arguments,
	)
	if err != nil {
		t.Fatal("test typed result fixture is invalid")
	}
	return parsed
}

func nativeFlowTimezone(offsetMinutes int16) string {
	if offsetMinutes == 0 {
		return "Z"
	}
	absolute := int(offsetMinutes)
	sign := "+"
	if absolute < 0 {
		sign = "-"
		absolute = -absolute
	}
	return fmt.Sprintf("%s%02d:%02d", sign, absolute/60, absolute%60)
}

func nativeFlowJSON(t *testing.T, value any) []byte {
	t.Helper()
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		t.Fatal(err)
	}
	return bytes.TrimSuffix(output.Bytes(), []byte{'\n'})
}

func nativeFlowFramedHash(domain string, values ...[]byte) string {
	hasher := sha256.New()
	for _, value := range append([][]byte{[]byte(domain)}, values...) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = hasher.Write(length[:])
		_, _ = hasher.Write(value)
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

type sequenceClock struct {
	mu    sync.Mutex
	times []time.Time
	index int
}

func (clock *sequenceClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	if len(clock.times) == 0 {
		return time.Time{}
	}
	index := clock.index
	if index >= len(clock.times) {
		index = len(clock.times) - 1
	}
	clock.index++
	return clock.times[index]
}
