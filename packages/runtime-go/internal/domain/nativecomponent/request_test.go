package nativecomponent

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/testsupport/toolidentity"
)

const nativeComponentTestResolvedAccountKeyV1 = "private-account-key-flow"

func TestNativeComponentRequestCannotSupplyPathHashOrCwd(t *testing.T) {
	typeOfRequest := reflect.TypeOf(Request{})
	fields := make([]string, 0, typeOfRequest.NumField())
	for index := 0; index < typeOfRequest.NumField(); index++ {
		fields = append(fields, typeOfRequest.Field(index).Name)
	}
	want := []string{"ComponentID", "Operation", "AccountFlowArguments", "Context", "Grant", "Deadline"}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("native request fields = %v, want payload-free %v", fields, want)
	}
}

func TestAnalyzeAccountFlowsRequestIsOneClosedHostPrivateContract(t *testing.T) {
	now := time.Date(2026, 7, 27, 1, 0, 0, 0, time.UTC)
	securityContext := nativeComponentTestContext(t, "case-flow", now)
	arguments := nativeComponentTestAccountFlowArguments(t, securityContext)
	request := nativeComponentTestAccountFlowRequest(t, securityContext, arguments, now)
	if err := ValidateRequest(request, now); err != nil {
		t.Fatalf("validate typed flow request: %v", err)
	}
	policy, ok := Policy(ComponentDataEngine, OperationFundsAnalyzeAccountFlows)
	if !ok || policy.ReadOnly != true || policy.MaxDuration != 30*time.Second ||
		ToolName(policy.ComponentID, policy.Operation) != "native__data_engine__funds_analyze_account_flows" {
		t.Fatalf("fixed flow policy unavailable or drifted: %#v", policy)
	}
	if _, ok := ParseToolName("native__data_engine__duckdb_query"); ok {
		t.Fatal("generic DuckDB query entered the native policy registry")
	}
	for _, value := range []any{AnalyzeAccountFlowsArgumentsInputV1{}, Request{}} {
		typeOfValue := reflect.TypeOf(value)
		for index := 0; index < typeOfValue.NumField(); index++ {
			name := strings.ToLower(typeOfValue.Field(index).Name)
			if strings.Contains(name, "path") || strings.Contains(name, "sql") || strings.Contains(name, "json") {
				t.Fatalf("typed flow contract contains generic escape field %q", typeOfValue.Field(index).Name)
			}
		}
	}
	if _, err := json.Marshal(request); err == nil {
		t.Fatal("host-private flow request became generically JSON serializable")
	}
	privateInput := NewAnalyzeAccountFlowsArgumentsInputV1(
		AnalyzeAccountFlowsArgumentsInputV1{},
		nativeComponentTestResolvedAccountKeyV1,
	)
	if _, err := json.Marshal(privateInput); err == nil {
		t.Fatal("host-private flow constructor input became generically JSON serializable")
	}
	formatted := fmt.Sprintf("%#v %#v", request, privateInput)
	if strings.Contains(formatted, nativeComponentTestResolvedAccountKeyV1) ||
		strings.Contains(formatted, arguments.expectedDuckDBContentSnapshotDigest) ||
		strings.Contains(formatted, arguments.expectedDuckDBSnapshotManifestSHA256) ||
		strings.Contains(formatted, arguments.expectedMaterializationIdentity) ||
		!strings.Contains(formatted, "[REDACTED]") {
		t.Fatalf("host-private request formatting leaked its resolved key: %s", formatted)
	}
	type inputWithoutMethods AnalyzeAccountFlowsArgumentsInputV1
	type argumentsWithoutMethods AnalyzeAccountFlowsArgumentsV1
	aliases := []any{inputWithoutMethods(privateInput), argumentsWithoutMethods(arguments)}
	for _, alias := range aliases {
		body, err := json.Marshal(alias)
		if err != nil {
			t.Fatalf("defined-type JSON failed: %v", err)
		}
		if strings.Contains(string(body), nativeComponentTestResolvedAccountKeyV1) {
			t.Fatalf("defined-type JSON leaked resolved account key: %s", body)
		}
		for _, verb := range []string{"%v", "%+v", "%#v"} {
			formatted := fmt.Sprintf(verb, alias)
			if strings.Contains(formatted, nativeComponentTestResolvedAccountKeyV1) {
				t.Fatalf("defined-type %s formatting leaked resolved account key: %s", verb, formatted)
			}
		}
	}
}

func TestAnalyzeAccountFlowsRequestBindsTypedArgumentsToCurrentContextAndGrant(t *testing.T) {
	now := time.Date(2026, 7, 27, 2, 0, 0, 0, time.UTC)
	securityContext := nativeComponentTestContext(t, "case-flow", now)
	arguments := nativeComponentTestAccountFlowArguments(t, securityContext)
	request := nativeComponentTestAccountFlowRequest(t, securityContext, arguments, now)

	missing := request
	missing.AccountFlowArguments = nil
	if err := ValidateRequest(missing, now); !errors.Is(err, ErrRequestInvalid) {
		t.Fatalf("missing typed arguments error = %v, want %v", err, ErrRequestInvalid)
	}

	otherContext := nativeComponentTestContext(t, "case-other", now)
	mismatched := request
	mismatched.Context = otherContext
	if err := ValidateRequest(mismatched, now); !errors.Is(err, ErrGrantInvalid) {
		// The durable grant is checked before the typed argument/context pair;
		// either boundary must fail before execution.
		t.Fatalf("cross-case typed request error = %v, want %v", err, ErrGrantInvalid)
	}

	tamperedArguments := arguments
	tamperedArguments.scanCap++
	tampered := request
	tampered.AccountFlowArguments = &tamperedArguments
	if err := ValidateRequest(tampered, now); !errors.Is(err, ErrGrantInvalid) {
		t.Fatalf("typed argument/grant hash mismatch error = %v, want %v", err, ErrGrantInvalid)
	}

	baseHash := AnalyzeAccountFlowsArgumentsHashV1(arguments)
	for name, mutate := range map[string]func(*AnalyzeAccountFlowsArgumentsV1){
		"DuckDB content snapshot": func(value *AnalyzeAccountFlowsArgumentsV1) {
			value.expectedDuckDBContentSnapshotDigest = strings.Repeat("7", 64)
		},
		"DuckDB snapshot manifest": func(value *AnalyzeAccountFlowsArgumentsV1) {
			value.expectedDuckDBSnapshotManifestSHA256 = strings.Repeat("8", 64)
		},
		"materialization identity": func(value *AnalyzeAccountFlowsArgumentsV1) {
			value.expectedMaterializationIdentity = accountFlowMaterializationIdentityPrefixV1 + strings.Repeat("9", 64)
		},
	} {
		t.Run(name+" is grant-bound", func(t *testing.T) {
			changed := arguments
			mutate(&changed)
			changedHash := AnalyzeAccountFlowsArgumentsHashV1(changed)
			if changedHash == "" || changedHash == baseHash {
				t.Fatalf("host provenance expectation did not change arguments hash: %q", changedHash)
			}
			tampered := request
			tampered.AccountFlowArguments = &changed
			if err := ValidateRequest(tampered, now); !errors.Is(err, ErrGrantInvalid) {
				t.Fatalf("host provenance/grant mismatch error = %v, want %v", err, ErrGrantInvalid)
			}
		})
	}

	invalidContextBinding := arguments
	invalidContextBinding.contextEpoch++
	invalid := request
	invalid.AccountFlowArguments = &invalidContextBinding
	if err := ValidateRequest(invalid, now); !errors.Is(err, ErrContextMismatch) {
		t.Fatalf("typed context mismatch error = %v, want %v", err, ErrContextMismatch)
	}
}

func TestAnalyzeAccountFlowsArgumentsRequireCanonicalSnapshotAndMaterializationExpectations(t *testing.T) {
	securityContext := nativeComponentTestContext(t, "case-flow", time.Date(2026, 7, 27, 2, 30, 0, 0, time.UTC))
	valid := nativeComponentTestAccountFlowArgumentsInput(securityContext)
	for name, mutate := range map[string]func(*AnalyzeAccountFlowsArgumentsInputV1){
		"missing DuckDB content snapshot": func(input *AnalyzeAccountFlowsArgumentsInputV1) {
			input.ExpectedDuckDBContentSnapshotDigest = ""
		},
		"noncanonical DuckDB content snapshot": func(input *AnalyzeAccountFlowsArgumentsInputV1) {
			input.ExpectedDuckDBContentSnapshotDigest = strings.ToUpper(input.ExpectedDuckDBContentSnapshotDigest)
		},
		"missing DuckDB snapshot manifest": func(input *AnalyzeAccountFlowsArgumentsInputV1) {
			input.ExpectedDuckDBSnapshotManifestSHA256 = ""
		},
		"noncanonical DuckDB snapshot manifest": func(input *AnalyzeAccountFlowsArgumentsInputV1) {
			input.ExpectedDuckDBSnapshotManifestSHA256 = strings.ToUpper(input.ExpectedDuckDBSnapshotManifestSHA256)
		},
		"wrong materialization identity": func(input *AnalyzeAccountFlowsArgumentsInputV1) {
			input.ExpectedMaterializationIdentity = "txn_daily_snapshot:v11:" + strings.Repeat("a", 64)
		},
		"noncanonical materialization identity": func(input *AnalyzeAccountFlowsArgumentsInputV1) {
			input.ExpectedMaterializationIdentity = accountFlowMaterializationIdentityPrefixV1 + strings.Repeat("A", 64)
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := valid
			mutate(&input)
			if arguments, err := NewAnalyzeAccountFlowsArgumentsV1(input); !errors.Is(err, ErrRequestInvalid) || !reflect.DeepEqual(arguments, AnalyzeAccountFlowsArgumentsV1{}) {
				t.Fatalf("invalid host provenance expectation survived: arguments=%#v err=%v", arguments, err)
			}
		})
	}
}

func TestNativeComponentRequestBindsCaseTurnSnapshotAndCurrentGrant(t *testing.T) {
	now := time.Date(2026, 7, 16, 1, 0, 0, 0, time.UTC)
	caseA := nativeComponentTestContext(t, "case-a", now)
	request := nativeComponentTestRequest(t, caseA, ComponentDataEngine, "health", now, "not_required")
	if err := ValidateRequest(request, now); err != nil {
		t.Fatalf("validate current request: %v", err)
	}
	caseB := nativeComponentTestContext(t, "case-b", now)
	request.Context = caseB
	if err := ValidateRequest(request, now); !errors.Is(err, ErrGrantInvalid) {
		t.Fatalf("cross-case request error = %v, want %v", err, ErrGrantInvalid)
	}
	request.Context = caseA
	request.Deadline = request.Deadline.Add(time.Hour)
	if err := ValidateRequest(request, now); !errors.Is(err, ErrGrantInvalid) {
		t.Fatalf("deadline beyond grant error = %v, want %v", err, ErrGrantInvalid)
	}
}

func TestNativeComponentUnbootstrappedWriteOperationsRemainUnavailable(t *testing.T) {
	now := time.Date(2026, 7, 16, 2, 0, 0, 0, time.UTC)
	context := nativeComponentTestContext(t, "case-write", now)
	request := nativeComponentTestRequest(t, context, ComponentDataEngine, "health", now, "not_required")
	for _, operation := range []string{"duckdb.open_write_session", "duckdb.execute", "cleaning.clean_all"} {
		request.Operation = operation
		if err := ValidateRequest(request, now); !errors.Is(err, ErrOperationUnavailable) {
			t.Fatalf("operation %s error = %v, want %v", operation, err, ErrOperationUnavailable)
		}
	}
}

func TestNativeComponentRequestRejectsNonCanonicalArgumentHashAndUnknownOperation(t *testing.T) {
	now := time.Date(2026, 7, 16, 3, 0, 0, 0, time.UTC)
	context := nativeComponentTestContext(t, "case-query", now)
	request := nativeComponentTestRequest(t, context, ComponentDataEngine, "health", now, "not_required")
	for _, hostile := range []string{
		`{"unknown":true}`,
		`{"workingDirectory":"/tmp/attacker"}`,
		`{"nested":{"value":true}}`,
	} {
		request.Grant = nativeComponentTestGrant(t, context, ComponentDataEngine, "health", now, "not_required", domainsecurity.CanonicalJSONHash([]byte(hostile)))
		if err := ValidateRequest(request, now); !errors.Is(err, ErrGrantInvalid) {
			t.Fatalf("hostile argument hash for %s error = %v, want %v", hostile, err, ErrGrantInvalid)
		}
	}
	request = nativeComponentTestRequest(t, context, ComponentDataEngine, "health", now, "not_required")
	request.Operation = "arbitrary.exec"
	if err := ValidateRequest(request, now); !errors.Is(err, ErrOperationUnavailable) {
		t.Fatalf("unknown operation error = %v, want %v", err, ErrOperationUnavailable)
	}
}

func TestNativeHealthCanonicalArgumentsAreHostFixed(t *testing.T) {
	arguments, ok := CanonicalArgumentsV1(ComponentDataEngine, "health")
	if !ok || arguments != `{}` {
		t.Fatalf("canonical health arguments = %q ok=%v", arguments, ok)
	}
	for _, operation := range []string{OperationFundsAnalyzeAccountFlows, "query", "execute", "arbitrary.exec"} {
		if arguments, ok := CanonicalArgumentsV1(ComponentDataEngine, operation); ok || arguments != "" {
			t.Fatalf("unregistered operation %q obtained arguments %q", operation, arguments)
		}
	}
}

func nativeComponentTestAccountFlowArguments(
	t *testing.T,
	securityContext domainsecurity.TurnSecurityContext,
) AnalyzeAccountFlowsArgumentsV1 {
	t.Helper()
	arguments, err := NewAnalyzeAccountFlowsArgumentsV1(nativeComponentTestAccountFlowArgumentsInput(securityContext))
	if err != nil {
		t.Fatal(err)
	}
	return arguments
}

func nativeComponentTestAccountFlowArgumentsInput(
	securityContext domainsecurity.TurnSecurityContext,
) AnalyzeAccountFlowsArgumentsInputV1 {
	return NewAnalyzeAccountFlowsArgumentsInputV1(AnalyzeAccountFlowsArgumentsInputV1{
		CaseID:                               securityContext.CaseID,
		DatasetSnapshotID:                    securityContext.DatasetSnapshotID,
		ContextEpoch:                         securityContext.ContextEpoch,
		ContextDigest:                        securityContext.ContextDigest,
		CaseBindingHash:                      securityContext.CaseBindingHash,
		ExpectedProducerContentID:            domainsecurity.FundsProducerContentIDPrefixV1 + strings.Repeat("4", 64),
		ExpectedProducerManifestSHA256:       strings.Repeat("5", 64),
		ExpectedDuckDBContentSnapshotDigest:  strings.Repeat("b", 64),
		ExpectedDuckDBSnapshotManifestSHA256: strings.Repeat("c", 64),
		ExpectedMaterializationIdentity:      accountFlowMaterializationIdentityPrefixV1 + strings.Repeat("a", 64),
		SubjectAlias:                         "acct:1",
		SubjectRef:                           "cer1_" + strings.Repeat("a", 64),
		SubjectResolutionDigest:              strings.Repeat("6", 64),
		StartInclusive:                       "2026-01-01T08:00:00+08:00",
		EndInclusive:                         "2026-01-01T08:02:00+08:00",
		EvidenceRowLimit:                     10,
		DatasetUTCOffsetMinutes:              480,
		ExpectedCurrency:                     "CNY",
		MinorUnitScale:                       AccountFlowMinorUnitScaleV1,
		ScanCap:                              100,
	}, nativeComponentTestResolvedAccountKeyV1)
}

func nativeComponentTestAccountFlowRequest(
	t *testing.T,
	securityContext domainsecurity.TurnSecurityContext,
	arguments AnalyzeAccountFlowsArgumentsV1,
	now time.Time,
) Request {
	t.Helper()
	policy, ok := Policy(ComponentDataEngine, OperationFundsAnalyzeAccountFlows)
	if !ok {
		t.Fatal("missing fixed flow policy")
	}
	grant := nativeComponentTestGrant(
		t,
		securityContext,
		ComponentDataEngine,
		OperationFundsAnalyzeAccountFlows,
		now,
		"not_required",
		AnalyzeAccountFlowsArgumentsHashV1(arguments),
	)
	return Request{
		ComponentID: ComponentDataEngine, Operation: OperationFundsAnalyzeAccountFlows,
		AccountFlowArguments: &arguments, Context: securityContext, Grant: grant,
		Deadline: now.Add(policy.MaxDuration),
	}
}

func nativeComponentTestRequest(
	t *testing.T,
	context domainsecurity.TurnSecurityContext,
	componentID string,
	operation string,
	now time.Time,
	approval string,
) Request {
	t.Helper()
	policy, ok := Policy(componentID, operation)
	if !ok {
		t.Fatalf("missing policy for %s/%s", componentID, operation)
	}
	canonicalArguments, ok := CanonicalArgumentsV1(componentID, operation)
	if !ok {
		t.Fatalf("missing canonical arguments for %s/%s", componentID, operation)
	}
	grant := nativeComponentTestGrant(t, context, componentID, operation, now, approval, domainsecurity.CanonicalJSONHash([]byte(canonicalArguments)))
	return Request{
		ComponentID: componentID, Operation: operation, Context: context, Grant: grant,
		Deadline: now.Add(policy.MaxDuration),
	}
}

func nativeComponentTestGrant(
	t *testing.T,
	context domainsecurity.TurnSecurityContext,
	componentID string,
	operation string,
	now time.Time,
	approval string,
	argsHash string,
) domainsecurity.ExecutionGrant {
	t.Helper()
	policy, ok := Policy(componentID, operation)
	if !ok {
		t.Fatalf("missing policy for %s/%s", componentID, operation)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: context, Provider: NativeProvider, ServerIdentity: NativeServerIdentity,
		ToolName: ToolName(componentID, operation), ToolCallID: toolidentity.MustHostToolCallIDV1("native-component:" + context.ContextDigest + ":" + componentID + ":" + operation), ConnectionEpoch: 0,
		ArgsHash: argsHash, SchemaHash: policy.SchemaHash,
		ScopeHash: ScopeHash(context, componentID, operation), ReadOnly: policy.ReadOnly,
		ApprovalState: approval, IssuedAt: now, ExpiresAt: now.Add(5 * time.Minute),
	})
	return grant
}

func nativeComponentTestContext(t *testing.T, caseID string, now time.Time) domainsecurity.TurnSecurityContext {
	t.Helper()
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: domainsecurity.SHA256Hex([]byte("policy:" + caseID)),
		RiskClass:              domainsecurity.RiskClassCase, Disposition: domainsecurity.PublicationDispositionCaseEvidenceGate,
		CaseBindingState:         domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("binding-observation:" + caseID)),
		BlockerCode:              domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding := domainsecurity.RiskAuthorityBindingV1{
		SchemaVersion: domainsecurity.RiskAuthorityBindingSchemaVersion,
		Purpose:       domainsecurity.RiskAuthorityBindingPurpose,
		State:         domainsecurity.RiskAuthorityBindingStateWitnessed,
		IndexDigest:   domainsecurity.SHA256Hex([]byte("index:" + caseID)), Generation: 1,
		CheckpointDigest:  domainsecurity.SHA256Hex([]byte("checkpoint:" + caseID)),
		ObservationDigest: domainsecurity.SHA256Hex([]byte("observation:" + caseID)),
	}
	context, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-" + caseID, TurnID: "turn-" + caseID,
		WorkspaceRealPath: "/workspace/" + caseID, TenantID: "local", UserID: "local", CaseID: caseID,
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("binding:" + caseID)),
		DatasetSnapshotID:  domainsecurity.DatasetSnapshotIDPrefixV2 + domainsecurity.SHA256Hex([]byte("snapshot:"+caseID)),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest:" + caseID)),
		ContextEpoch:       1, IssuedAt: now, PublicationPolicy: policy, RiskAuthorityBinding: binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	return context
}
