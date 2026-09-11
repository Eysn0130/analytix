package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
)

func TestExactHostFundsUsesInProcessTransportWithoutStartingPackagedJavaScript(t *testing.T) {
	spec := hostFundsServerSpecFixtureV1(t)
	if err := os.MkdirAll(filepath.Dir(spec.Command), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec.Command, []byte("#!/bin/sh\nexit 97\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NODE_OPTIONS", "--require=/ambient/node-hook-must-not-run.cjs")
	host, err := NewHostFundsServerSpecV1(spec, hostFundsStaticAdmissionInputFixtureV1(t, spec.ExpectedServerVersion))
	if err != nil {
		t.Fatal(err)
	}
	manager := NewProductionManagerWithOptions(nil, ProductionManagerOptions{
		DatasetAuthority: unavailableHostFundsDatasetAuthorityV2{},
		HostFundsServer:  host,
	})
	manager.Connect()
	client, ok := manager.clients["analytix_funds"].(*hostFundsTransportClientV1)
	if !ok || client == nil {
		t.Fatalf("exact host funds binding did not create the in-process transport: %#v", manager.failures)
	}
	if tools := manager.Tools(); len(tools) != 1 ||
		tools[0] != CanonicalToolName("analytix_funds", hostFundsCountToolNameV1) {
		t.Fatalf("in-process host funds catalog mismatch: %#v", tools)
	}
	identity := client.ObservedServerIdentity()
	if identity.ProtocolVersion != MCPProtocolVersion || identity.Name != "analytix_funds" ||
		identity.Version != hostFundsTestPackageVersionV1 {
		t.Fatalf("in-process host funds identity mismatch: %#v", identity)
	}
	manager.RefreshCatalog()
	if _, ok := manager.clients["analytix_funds"].(*hostFundsTransportClientV1); !ok {
		t.Fatalf("refresh fell back to a generic funds transport: %#v", manager.failures)
	}
	manager.RestartReconnect()
	if _, ok := manager.clients["analytix_funds"].(*hostFundsTransportClientV1); !ok {
		t.Fatalf("restart fell back to a generic funds transport: %#v", manager.failures)
	}
	manager.Disconnect()
}

func TestGenericTransportsRejectHighRiskFundsBeforeAnyIO(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	if _, err := newMCPTransportClient(ServerSpec{
		ID: "analytix_funds", Transport: "http", URL: server.URL,
	}, "", nil); err == nil || requests.Load() != 0 {
		t.Fatalf("generic HTTP transport reached high-risk funds I/O: err=%v requests=%d", err, requests.Load())
	}

	marker := filepath.Join(t.TempDir(), "stdio-started")
	if _, err := newMCPTransportClient(ServerSpec{
		ID: "analytix-fund-analysis", Transport: "stdio", Command: "/bin/sh",
		Args: []string{"-c", "touch " + marker},
	}, "", nil); err == nil {
		t.Fatal("generic stdio transport accepted a high-risk funds server")
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("generic stdio transport executed a high-risk funds command: %v", err)
	}
}

func TestHostFundsInProcessIdentityCatalogProbeCountEvidenceAndCaptureContracts(t *testing.T) {
	client := newHostFundsTransportClientForTestV1(t)
	defer client.Close()

	catalog, err := client.ListToolCatalogContext(context.Background())
	if err != nil || !exactHostFundsRuntimeToolCatalogV1(catalog.Tools, len(catalog.Quarantined), true, true) {
		t.Fatalf("fixed in-process catalog mismatch: err=%v catalog=%#v", err, catalog)
	}
	prompts, err := client.ListPrompts()
	if err != nil || len(prompts) != 1 || prompts[0].Name != hostFundsBoundaryPromptNameV1 ||
		prompts[0].Description != "返回固定资金来源不可用边界；不接受案件文本或敏感个人信息参数。" ||
		len(prompts[0].Arguments) != 0 {
		t.Fatalf("fixed in-process prompt metadata drifted: err=%v prompts=%#v", err, prompts)
	}
	resources, err := client.ListResources()
	if err != nil || len(resources) != 1 || resources[0].URI != hostFundsBoundaryResourceURIV1 ||
		resources[0].Name != "资金事实来源不可用边界" ||
		resources[0].Description != "固定能力边界；不包含案件事实、敏感个人信息或报告内容。" ||
		resources[0].MimeType != "text/plain" {
		t.Fatalf("fixed in-process resource metadata drifted: err=%v resources=%#v", err, resources)
	}
	projection := hostFundsProjectionForTransportTestV1(t)
	probeParams, err := sourceProbeNativeParamsV2(projection)
	if err != nil {
		t.Fatal(err)
	}
	probe, err := client.CallNativeContext(context.Background(), "analytix/sourceProbe", probeParams)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseFundsSourceProbeResponseV2(
		probe, projection, "analytix_funds", hostFundsTestPackageVersionV1,
	); err != nil {
		t.Fatalf("fixed source probe failed existing validator: %v", err)
	}

	arguments := map[string]any{"table_name": domainsecurity.FundsCountProjectionTableV2}
	callParams, err := fundsCountToolNativeParamsV2(arguments, projection)
	if err != nil {
		t.Fatal(err)
	}
	called, err := client.CallNativeLosslessContext(context.Background(), "tools/call", callParams)
	if err != nil || !domainmcp.ValidLosslessToolResult(called) {
		t.Fatalf("fixed count result lost exact bytes: err=%v result=%#v", err, called)
	}
	structured, ok := called.Value.(map[string]any)
	if !ok || structured["semanticStatus"] != "success" ||
		structured["purpose"] != fundsCountToolOutcomePurposeV2 {
		t.Fatalf("fixed count result mismatch: %#v", called.Value)
	}
	observedProjection, err := domainsecurity.ParseFundsCountProjectionV2(structured["data"])
	if err != nil || observedProjection != projection {
		t.Fatalf("fixed count projection drifted: observed=%#v err=%v", observedProjection, err)
	}

	evidenceParams, err := fundsEvidenceReadNativeParamsV2(projection)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := client.CallNativeLosslessContext(
		context.Background(), fundsEvidenceReadMethod, evidenceParams,
	)
	if err != nil || !domainmcp.ValidLosslessToolResult(evidence) {
		t.Fatalf("fixed evidence result lost exact bytes: err=%v result=%#v", err, evidence)
	}
	var candidate struct {
		SchemaVersion      int                                   `json:"schemaVersion"`
		Purpose            string                                `json:"purpose"`
		ServerName         string                                `json:"serverName"`
		ServerVersion      string                                `json:"serverVersion"`
		ToolName           string                                `json:"toolName"`
		Projection         domainsecurity.FundsCountProjectionV2 `json:"projection"`
		PaginationComplete bool                                  `json:"paginationComplete"`
		ReadOnly           bool                                  `json:"readOnly"`
	}
	if err := json.Unmarshal(evidence.RawResult, &candidate); err != nil ||
		candidate.SchemaVersion != 2 ||
		candidate.Purpose != "analytix.funds.count-case-rows-evidence-candidate/v2" ||
		candidate.ServerName != "analytix_funds" || candidate.ServerVersion != hostFundsTestPackageVersionV1 ||
		candidate.ToolName != hostFundsCountToolNameV1 || candidate.Projection != projection ||
		!candidate.PaginationComplete || !candidate.ReadOnly {
		t.Fatalf("fixed evidence contract mismatch: candidate=%#v err=%v", candidate, err)
	}

	unsafe := hostFundsAccountFlowArgumentsForTransportTestV1()
	unsafe["subject_alias"] = "6217000012345678901"
	if _, err := client.CallToolContext(context.Background(), hostFundsAccountFlowToolNameV1, unsafe); err == nil ||
		strings.Contains(err.Error(), "6217000012345678901") {
		t.Fatalf("unsafe account-flow identifier was accepted or reflected: %v", err)
	}
	unsafe = hostFundsAccountFlowArgumentsForTransportTestV1()
	unsafe["sql"] = "SELECT * FROM secret"
	if _, err := client.CallToolContext(context.Background(), hostFundsAccountFlowToolNameV1, unsafe); err == nil ||
		strings.Contains(strings.ToLower(err.Error()), "select") {
		t.Fatalf("unsafe account-flow SQL was accepted or reflected: %v", err)
	}
	if _, err := client.CallToolContext(context.Background(), hostFundsCountToolNameV1, arguments); err == nil {
		t.Fatal("authority-free count call bypassed the host projection")
	}
}

func TestHostFundsManagerCountEvidenceUsesInProcessContract(t *testing.T) {
	fixture := newHostEvidenceManagerFixture(t, true)
	client, err := newHostFundsTransportClientV1(
		fixture.manager.hostFundsServer,
		fixture.spec,
		fixture.manager.hostFundsFingerprint,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	fixture.manager.mu.Lock()
	fixture.manager.clients[fixture.spec.ID] = client
	fixture.manager.removeServerToolsNoLock(fixture.spec.ID)
	runtimeTools := hostFundsRuntimeToolCatalogV1(true, false)
	registerErr := fixture.manager.registerToolsNoLock(fixture.spec, runtimeTools)
	if registerErr == nil {
		fixture.manager.sourceCatalogs[fixture.spec.ID] = sourceProbeCatalogFingerprintWithIssues(runtimeTools, nil)
		fixture.manager.refreshCatalogStateNoLock()
	}
	fixture.manager.mu.Unlock()
	if registerErr != nil {
		t.Fatal(registerErr)
	}
	probe, err := fixture.manager.ProbeCaseSource(
		context.Background(), fixture.probeInput(fixture.securityContext),
	)
	if err != nil {
		t.Fatal(err)
	}
	countArguments := map[string]any{"table_name": domainsecurity.FundsCountProjectionTableV2}
	countToolName := CanonicalToolName("analytix_funds", hostFundsCountToolNameV1)
	countEnvelope := sourceProbeHostContext(
		t, fixture.manager, fixture.securityContext, countToolName, probe.ConnectionEpoch, countArguments,
	)
	countResult := fixture.manager.CallToolSecurityBoundContext(
		context.Background(), countToolName, true, countEnvelope, countArguments,
	)
	countProjection, ok := countResult["result"].(map[string]any)
	if countResult["executed"] != true || countResult["semanticStatus"] != "success" || !ok ||
		countProjection["purpose"] != fundsCountToolOutcomePurposeV2 ||
		countProjection["semanticStatus"] != "success" {
		t.Fatalf("manager count did not use the fixed in-process result: %#v", countResult)
	}
	_, countGrant, err := countEnvelope.Authority()
	if err != nil {
		t.Fatal(err)
	}
	countArgumentBytes, err := json.Marshal(countArguments)
	if err != nil {
		t.Fatal(err)
	}
	evidenceSeen := false
	if err := fixture.manager.WithCurrentEvidenceReadAuthority(
		context.Background(),
		sourceprobeport.EvidenceReadInput{
			Context: fixture.securityContext, Binding: fixture.binding,
			Grant: countGrant, Arguments: countArgumentBytes,
		},
		func(
			current domainsecurity.VerifiedSourceProbe,
			raw domainmcp.LosslessToolResult,
			capability sourceprobeport.HostEvidenceCapability,
		) error {
			if current.ProbeDigest != probe.ProbeDigest || capability == nil ||
				!domainmcp.ValidLosslessToolResult(raw) ||
				!bytes.Contains(raw.RawResult, []byte("analytix.funds.count-case-rows-evidence-candidate/v2")) {
				return errors.New("in-process evidence result is invalid")
			}
			evidenceSeen = true
			return nil
		},
	); err != nil || !evidenceSeen {
		t.Fatalf("manager evidence read did not use the fixed in-process result: seen=%v err=%v", evidenceSeen, err)
	}
}

func TestHostFundsManagerExecutesProviderSafeAccountFlowWithPrivateEvidenceCarrier(t *testing.T) {
	fixture := newHostEvidenceManagerFixture(t, true)
	var currentGrant domainsecurity.ExecutionGrant
	var calls atomic.Int64
	fixture.manager.accountFlowExecutor = func(
		ctx context.Context,
		input AccountFlowExecutionInput,
		consumeSummary domainnative.AccountFlowHostEvidenceSummaryConsumerV1,
		consumeRow domainnative.AccountFlowHostEvidenceRowConsumerV1,
	) (domainnative.AccountFlowProviderSemanticResultV1, error) {
		calls.Add(1)
		if fixture.manager.ValidateCurrentAccountFlowOuterGrant(ctx, fixture.securityContext, currentGrant) != nil ||
			input.GrantID != currentGrant.GrantID {
			return domainnative.AccountFlowProviderSemanticResultV1{}, errors.New("outer grant changed")
		}
		semantic := domainnative.AccountFlowProviderSemanticResultV1{
			SubjectAlias: input.Intent.SubjectAlias, StartInclusive: input.Intent.StartInclusive,
			EndInclusive: input.Intent.EndInclusive, Timezone: "Z", Currency: "CNY",
			MinorUnitScale: domainnative.AccountFlowMinorUnitScaleV1,
			InflowMinor:    "100", OutflowMinor: "0", NetMinor: "100", TransactionCount: 1,
			EvidenceTransactionCount: 1, EvidenceRowLimit: input.Intent.EvidenceRowLimit,
			AggregateComplete: true, EvidenceRowsComplete: true, CounterpartySemanticsComplete: false,
			Currentness: domainnative.AccountFlowProviderCurrentnessCurrentV1,
			Coverage: domainnative.AccountFlowProviderSemanticCoverageV1{
				State:                  domainnative.AccountFlowCoveragePartialV1,
				Gaps:                   []string{domainnative.AccountFlowGapCounterpartyResolutionV1},
				NormalizedSnapshotRows: 1, AcceptedSnapshotRows: 1, ObservedMatchingRows: 1,
			},
			QueryHash:  domainsecurity.SHA256Hex([]byte("account-flow-query")),
			ResultHash: domainsecurity.SHA256Hex([]byte("account-flow-result")),
			Transactions: []domainnative.AccountFlowProviderSemanticTransactionV1{{
				EvidenceRef:  "srow1_" + domainsecurity.SHA256Hex([]byte("account-flow-row")),
				Counterparty: domainnative.AccountFlowProviderCounterpartyV1{Status: domainnative.AccountFlowCounterpartyUnresolvedV1},
				OccurredAt:   "2026-01-05T10:30:00.000000Z", Direction: domainnative.AccountFlowDirectionInflowV1,
				AmountMinor: "100", Currency: "CNY", MinorUnitScale: domainnative.AccountFlowMinorUnitScaleV1,
			}},
		}
		outcome, err := domainnative.NewAccountFlowProviderOutcomeV1(
			semantic.SubjectAlias, semantic.AggregateComplete, semantic.EvidenceRowsComplete, semantic.QueryHash, semantic.ResultHash,
		)
		if err != nil {
			return domainnative.AccountFlowProviderSemanticResultV1{}, err
		}
		semantic.Outcome = outcome
		if err := consumeSummary(
			"cer1_"+strings.Repeat("a", 64), fixture.securityContext.DatasetSnapshotID,
			fixture.securityContext.ContextEpoch, fixture.securityContext.ContextDigest,
			fixture.securityContext.CaseBindingHash, semantic.StartInclusive, semantic.EndInclusive,
			semantic.Timezone, semantic.Currency, semantic.MinorUnitScale,
			semantic.InflowMinor, semantic.OutflowMinor, semantic.NetMinor,
			semantic.TransactionCount, semantic.AggregateComplete, semantic.EvidenceRowsComplete,
			semantic.EvidenceTransactionCount, semantic.Coverage, semantic.QueryHash, semantic.ResultHash,
		); err != nil {
			return domainnative.AccountFlowProviderSemanticResultV1{}, err
		}
		row := semantic.Transactions[0]
		if err := consumeRow(
			0, "cer1_"+strings.Repeat("a", 64), row.EvidenceRef, "0123456789abcdefabcd", 7,
			row.OccurredAt, row.Direction, row.AmountMinor, row.Currency, row.MinorUnitScale,
		); err != nil {
			return domainnative.AccountFlowProviderSemanticResultV1{}, err
		}
		return semantic, nil
	}
	client, err := newHostFundsTransportClientV1(
		fixture.manager.hostFundsServer, fixture.spec, fixture.manager.hostFundsFingerprint, true,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	replacementTools := hostFundsRuntimeToolCatalogV1(true, true)
	replacementCatalogDigest := sourceProbeCatalogFingerprintWithIssues(replacementTools, nil)
	fixture.manager.mu.Lock()
	fixture.manager.beginHostFundsSourceReadSetupNoLock()
	specFingerprint := fixture.manager.specFingerprints[fixture.spec.ID]
	serverIdentity := fixture.manager.serverIdentities[fixture.spec.ID]
	connectionEpoch := fixture.manager.connectionEpochs[fixture.spec.ID]
	activated := fixture.manager.activateHostFundsSourceReadNoLock(
		specFingerprint,
		serverIdentity,
		replacementCatalogDigest,
		connectionEpoch,
	)
	var registerErr error
	if activated {
		fixture.manager.removeServerToolsNoLock(fixture.spec.ID)
		registerErr = fixture.manager.registerToolsNoLock(fixture.spec, replacementTools)
	}
	if activated && registerErr == nil {
		fixture.manager.sourceCatalogs[fixture.spec.ID] = replacementCatalogDigest
		fixture.manager.clients[fixture.spec.ID] = client
		fixture.manager.refreshCatalogStateNoLock()
	}
	fixture.manager.mu.Unlock()
	if !activated {
		t.Fatal("replacement Funds source-read authority was not activated")
	}
	if registerErr != nil {
		t.Fatal(registerErr)
	}
	probe, err := fixture.manager.ProbeCaseSource(context.Background(), fixture.probeInput(fixture.securityContext))
	if err != nil {
		t.Fatal(err)
	}
	arguments := hostFundsAccountFlowArgumentsForTransportTestV1()
	toolName := CanonicalToolName("analytix_funds", hostFundsAccountFlowToolNameV1)
	envelope := sourceProbeHostContext(
		t, fixture.manager, fixture.securityContext, toolName, probe.ConnectionEpoch, arguments,
	)
	_, currentGrant, err = envelope.Authority()
	if err != nil {
		t.Fatal(err)
	}
	result := fixture.manager.CallToolSecurityBoundContext(context.Background(), toolName, true, envelope, arguments)
	if result["executed"] != true || result["semanticStatus"] != "partial" || calls.Load() != 1 {
		t.Fatalf("account-flow execution mismatch: result=%#v calls=%d", result, calls.Load())
	}
	lossless, ok := result[domainmcp.HostRawToolResultKey].(domainmcp.LosslessToolResult)
	if !ok || !domainmcp.ValidLosslessToolResult(lossless) || lossless.HostPrivate == nil {
		t.Fatalf("account-flow private carrier missing: %#v", result)
	}
	semantic, ok := fixture.manager.HostFundsAccountFlowProviderSemanticV1(lossless)
	if !ok || semantic.SubjectAlias != "card:1" || semantic.NetMinor != "100" ||
		semantic.TransactionCount != 1 || len(semantic.Transactions) != 1 ||
		semantic.Currentness != domainnative.AccountFlowProviderCurrentnessCurrentV1 ||
		semantic.Outcome.TypedSlotEligibility != domainevidence.AccountFlowOutcomeTypedSlotPendingFinalGateV1 ||
		semantic.Outcome.LocalDisplayCompletion != domainevidence.AccountFlowOutcomeDisplayNotRequestedV1 ||
		semantic.Outcome.FactAnswerAllowed ||
		semantic.Outcome.SourceFieldReference.Field != domainevidence.AcceptedSlotSourceFieldCardV1 ||
		domainevidence.ValidateAccountFlowTypedSourceFieldReferenceV1(
			semantic.Outcome.SourceFieldReference, semantic.QueryHash, semantic.ResultHash,
		) != nil || !bytes.Contains(lossless.RawResult, []byte(`"schemaVersion":3`)) ||
		!bytes.Contains(lossless.RawResult, []byte(`"purpose":"analytix.funds-account-flow-analysis/v3"`)) {
		t.Fatalf("provider-safe semantic mismatch: %#v ok=%v", semantic, ok)
	}
	restarted := lossless
	restarted.HostPrivate = nil
	if err := fixture.manager.ConsumeHostFundsAccountFlowEvidenceV1(
		restarted,
		func(string, string, uint64, string, string, string, string, string, string, uint8, string, string, string, uint64, bool, bool, uint64, domainnative.AccountFlowProviderSemanticCoverageV1, string, string) error {
			return nil
		},
		func(int, string, string, string, uint64, string, string, string, string, uint8) error {
			return nil
		},
	); !errors.Is(err, errHostFundsRequestInvalidV1) {
		t.Fatalf("restart reconstruction without the private carrier was consumable: %v", err)
	}
	const consumerCount = 64
	var successes atomic.Int64
	var rejected atomic.Int64
	var summaryCalls atomic.Int64
	var rowCalls atomic.Int64
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := 0; index < consumerCount; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			err := fixture.manager.ConsumeHostFundsAccountFlowEvidenceV1(
				lossless,
				func(string, string, uint64, string, string, string, string, string, string, uint8, string, string, string, uint64, bool, bool, uint64, domainnative.AccountFlowProviderSemanticCoverageV1, string, string) error {
					summaryCalls.Add(1)
					return nil
				},
				func(index int, _ string, _ string, sourceFileID string, sourceRowNumber uint64, _ string, _ string, _ string, _ string, _ uint8) error {
					if index != 0 || sourceFileID != "0123456789abcdefabcd" || sourceRowNumber != 7 {
						return errors.New("private evidence changed")
					}
					rowCalls.Add(1)
					return nil
				},
			)
			if err == nil {
				successes.Add(1)
				return
			}
			if !errors.Is(err, errHostFundsRequestInvalidV1) {
				t.Errorf("concurrent private carrier returned an unexpected error: %v", err)
			}
			rejected.Add(1)
		}()
	}
	close(start)
	wait.Wait()
	if successes.Load() != 1 || rejected.Load() != consumerCount-1 ||
		summaryCalls.Load() != 1 || rowCalls.Load() != 1 {
		t.Fatalf(
			"private evidence was not consumed exactly once under race: successes=%d rejected=%d summaries=%d rows=%d",
			successes.Load(), rejected.Load(), summaryCalls.Load(), rowCalls.Load(),
		)
	}
	if err := fixture.manager.ConsumeHostFundsAccountFlowEvidenceV1(
		lossless,
		func(string, string, uint64, string, string, string, string, string, string, uint8, string, string, string, uint64, bool, bool, uint64, domainnative.AccountFlowProviderSemanticCoverageV1, string, string) error {
			return nil
		},
		func(int, string, string, string, uint64, string, string, string, string, uint8) error {
			return nil
		},
	); !errors.Is(err, errHostFundsRequestInvalidV1) {
		t.Fatalf("burned private evidence carrier was replayable: %v", err)
	}
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"0123456789abcdefabcd", "cer1_", "/private/duckdb", "select * from", "reversemap", "sourceexactvalue"} {
		if bytes.Contains(bytes.ToLower(body), []byte(strings.ToLower(forbidden))) {
			t.Fatalf("public account-flow result leaked %q: %s", forbidden, body)
		}
	}
}

func TestHostFundsInProcessTransportFingerprintMismatchNeverFallsBack(t *testing.T) {
	spec := hostFundsServerSpecFixtureV1(t)
	host, err := NewHostFundsServerSpecV1(spec, hostFundsStaticAdmissionInputFixtureV1(t, spec.ExpectedServerVersion))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newHostFundsTransportClientV1(host, spec, strings.Repeat("f", 64)); err == nil {
		t.Fatal("mismatched host funds fingerprint created an in-process client")
	}
	manager := NewProductionManagerWithOptions(nil, ProductionManagerOptions{
		DatasetAuthority: unavailableHostFundsDatasetAuthorityV2{}, HostFundsServer: host,
	})
	manager.hostFundsFingerprint = ""
	manager.Connect()
	defer manager.Disconnect()
	if len(manager.clients) != 0 || len(manager.Tools()) != 0 ||
		!strings.Contains(manager.failures["analytix_funds"], "exact host-owned in-process binding") {
		t.Fatalf("missing fingerprint fell back to a generic transport: clients=%#v failures=%#v", manager.clients, manager.failures)
	}
}

func TestHostFundsInProcessCloseAndCancellationAreConcurrentSafe(t *testing.T) {
	client := newHostFundsTransportClientForTestV1(t)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.ListToolCatalogContext(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled catalog call did not stop: %v", err)
	}
	if _, err := client.CallNativeContext(cancelled, "analytix/sourceProbe", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled native call did not stop: %v", err)
	}

	var wait sync.WaitGroup
	errorsSeen := make(chan error, 128)
	for index := 0; index < 128; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := client.ListToolCatalogContext(context.Background())
			if err != nil && !errors.Is(err, errHostFundsTransportClosedV1) {
				errorsSeen <- err
			}
		}()
	}
	client.Close()
	client.Close()
	wait.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		t.Fatalf("concurrent close returned an unexpected error: %v", err)
	}
	if identity := client.ObservedServerIdentity(); identity != (domainmcp.ServerIdentity{}) {
		t.Fatalf("closed transport retained observed identity: %#v", identity)
	}
	if _, err := client.ListTools(); !errors.Is(err, errHostFundsTransportClosedV1) {
		t.Fatalf("closed transport remained usable: %v", err)
	}
}

func newHostFundsTransportClientForTestV1(t *testing.T) *hostFundsTransportClientV1 {
	t.Helper()
	spec := hostFundsServerSpecFixtureV1(t)
	host, err := NewHostFundsServerSpecV1(spec, hostFundsStaticAdmissionInputFixtureV1(t, spec.ExpectedServerVersion))
	if err != nil {
		t.Fatal(err)
	}
	client, err := newHostFundsTransportClientV1(host, spec, SpecFingerprint(spec))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func hostFundsProjectionForTransportTestV1(t *testing.T) domainsecurity.FundsCountProjectionV2 {
	t.Helper()
	manifestDigest := strings.Repeat("b", 64)
	projection, err := domainsecurity.NewFundsCountProjectionV2(domainsecurity.FundsCountProjectionInputV2{
		TurnSecurityContextDigest:          strings.Repeat("a", 64),
		DatasetSnapshotID:                  domainsecurity.DatasetSnapshotIDPrefixV2 + manifestDigest,
		DatasetSelectionDigest:             strings.Repeat("c", 64),
		DatasetRecordDigest:                strings.Repeat("d", 64),
		DatasetManifestDigest:              manifestDigest,
		FundsProducerContentID:             "fpc1_" + strings.Repeat("e", 64),
		FundsProducerContentManifestSHA256: strings.Repeat("f", 64),
		DetailContentSHA256:                strings.Repeat("1", 64),
		RowCount:                           42,
	})
	if err != nil {
		t.Fatal(err)
	}
	return projection
}

func hostFundsAccountFlowArgumentsForTransportTestV1() map[string]any {
	return map[string]any{
		"subject_alias":      "card:1",
		"start_inclusive":    "2026-01-01T00:00:00.000000Z",
		"end_inclusive":      "2026-01-31T23:59:59.999000Z",
		"evidence_row_limit": 100,
	}
}
