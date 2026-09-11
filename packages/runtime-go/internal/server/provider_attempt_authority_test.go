package server

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	evidenceregistryadapter "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	threadriskauthorityapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	provider "analytix.local/runtime-go/internal/provider"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type providerAttemptPostAppendObserver struct {
	store    *DurableEventSessionStore
	threadID string
	current  domainsecurity.CaseBindingObservationV1
	stale    domainsecurity.CaseBindingObservationV1

	preAppendCalls  atomic.Int64
	postAppendCalls atomic.Int64
}

func (observer *providerAttemptPostAppendObserver) Observe(string) (domainsecurity.CaseBindingObservationV1, error) {
	if observer == nil || observer.store == nil || strings.TrimSpace(observer.threadID) == "" {
		return domainsecurity.CaseBindingObservationV1{}, errors.New("provider-attempt observer is unavailable")
	}
	thread, err := observer.store.GetThread(observer.threadID)
	if err != nil {
		return domainsecurity.CaseBindingObservationV1{}, err
	}
	if len(listAny(thread["turns"])) > 0 {
		observer.postAppendCalls.Add(1)
		return observer.stale, nil
	}
	observer.preAppendCalls.Add(1)
	return observer.current, nil
}

type providerAttemptRevokingCaseAuthority struct {
	casethreadapp.CommittedAuthority
	source  *admissionFailureMCP
	commits atomic.Int64
}

type finalizationStaleRiskAuthority struct {
	base  *serverTestRiskAuthority
	stale atomic.Bool
}

func (authority *finalizationStaleRiskAuthority) ResolveOrRaise(ctx context.Context, input threadriskauthorityapp.ResolveOrRaiseInput) (threadriskauthorityapp.Head, error) {
	return authority.base.ResolveOrRaise(ctx, input)
}

func (authority *finalizationStaleRiskAuthority) ValidateCurrent(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) error {
	if authority.stale.Load() {
		return errors.New("current general risk head changed before final publication")
	}
	return authority.base.ValidateCurrent(ctx, securityContext)
}

type finalizationStalingProvider struct {
	authority *finalizationStaleRiskAuthority
	calls     atomic.Int64
	text      string
}

func (*finalizationStalingProvider) RequiresDurablePipelineStagesV1() {}

func (provider *finalizationStalingProvider) Stream(_ context.Context, request domainmodel.Request) (domainmodel.Result, error) {
	if err := emitTestDurableProviderPipelinePairV1(request); err != nil {
		return domainmodel.Result{}, err
	}
	provider.calls.Add(1)
	chunk := domainmodel.Chunk{Kind: domainmodel.ChunkText, Text: provider.text}
	if request.OnChunk != nil {
		if err := request.OnChunk(chunk); err != nil {
			return domainmodel.Result{}, err
		}
	}
	provider.authority.stale.Store(true)
	return domainmodel.Result{
		ProviderID: request.ProviderID, EndpointFormat: request.EndpointFormat,
		Chunks: []domainmodel.Chunk{chunk}, StreamCompleted: true,
	}, nil
}

type providerAttemptPostAppendRiskAuthority struct {
	base     *visionPostCommitRiskAuthority
	store    *DurableEventSessionStore
	threadID string

	preAppendValidations  atomic.Int64
	postAppendValidations atomic.Int64
}

func (authority *providerAttemptPostAppendRiskAuthority) ResolveOrRaise(ctx context.Context, input threadriskauthorityapp.ResolveOrRaiseInput) (threadriskauthorityapp.Head, error) {
	if authority == nil || authority.base == nil {
		return threadriskauthorityapp.Head{}, errors.New("provider-attempt risk authority is unavailable")
	}
	return authority.base.ResolveOrRaise(ctx, input)
}

func (authority *providerAttemptPostAppendRiskAuthority) ValidateCurrent(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) error {
	if authority == nil || authority.base == nil || authority.store == nil || strings.TrimSpace(authority.threadID) == "" {
		return errors.New("provider-attempt risk authority is unavailable")
	}
	thread, err := authority.store.GetThread(authority.threadID)
	if err != nil {
		return err
	}
	if len(listAny(thread["turns"])) > 0 {
		authority.postAppendValidations.Add(1)
		return errors.New("current witnessed risk head was superseded after append")
	}
	authority.preAppendValidations.Add(1)
	return authority.base.ValidateCurrent(ctx, securityContext)
}

func (authority *providerAttemptRevokingCaseAuthority) Commit(ctx context.Context, securityContext domainsecurity.TurnSecurityContext, state domaincontextepoch.State, at time.Time) error {
	if authority == nil || authority.CommittedAuthority == nil || authority.source == nil {
		return errors.New("provider-attempt case commit authority is unavailable")
	}
	if err := authority.CommittedAuthority.Commit(ctx, securityContext, state, at); err != nil {
		return err
	}
	authority.commits.Add(1)
	authority.source.revokeCurrentProbe()
	return nil
}

func TestValidateCurrentProviderAttemptRejectsPostAppendGeneralAuthorityDrift(t *testing.T) {
	workspace := t.TempDir()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "provider-attempt-general", BaseURL: "https://provider.invalid/v1", APIKey: "provider-key",
		Model: "general-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	handler.caseThreads = &caseThreadAuthorityStub{}
	countingProvider := &admissionFailureCountingProvider{}
	handler.provider = countingProvider

	thread, err := handler.store.CreateThread(map[string]any{
		"title": "general provider authority drift", "workspace": workspace,
		"providerId": "provider-attempt-general", "model": "general-model",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	baseObserver := filestore.CaseBindingReader{}
	current, err := baseObserver.Observe(workspace)
	if err != nil || current.State != domainsecurity.CaseBindingStateMissing {
		t.Fatalf("general workspace observation is invalid: observation=%#v err=%v", current, err)
	}
	stale, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: current.WorkspaceRealPath + "-changed", State: domainsecurity.CaseBindingStateMissing,
	})
	if err != nil {
		t.Fatal(err)
	}
	observer := &providerAttemptPostAppendObserver{
		store: handler.store, threadID: threadID, current: current, stale: stale,
	}
	handler.turnSecurity = turnsecurityapp.WorkspaceSecurityAuthority{Identity: testIdentityAuthority(),
		Observer: observer, RiskAuthority: newVisionPostCommitRiskAuthority(),
	}

	_, startErr := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
		Prompt: "Summarize this generic note.", ProviderID: "provider-attempt-general", Model: "general-model",
	})
	if startErr == nil {
		t.Fatal("post-append observer drift must reject the provider attempt")
	}
	if calls := countingProvider.calls.Load(); calls != 0 {
		t.Fatalf("stale general authority reached Provider.Stream: calls=%d", calls)
	}
	if observer.preAppendCalls.Load() < 2 || observer.postAppendCalls.Load() != 1 {
		t.Fatalf("observer drift did not occur only after durable append: pre=%d post=%d",
			observer.preAppendCalls.Load(), observer.postAppendCalls.Load())
	}

	reloaded, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turns := listAny(reloaded["turns"])
	if len(turns) != 1 {
		t.Fatalf("authority drift did not preserve exactly one durable turn: %#v", turns)
	}
	turn, _ := turns[0].(map[string]any)
	if stringField(turn, "status") != "failed" {
		t.Fatalf("general authority drift did not deterministically close the turn: startErr=%v turn=%#v", startErr, turn)
	}
	securityContext, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) != nil ||
		!domainsecurity.TurnSecurityContextIsGeneral(securityContext) || securityContext.ThreadID != threadID {
		t.Fatalf("failed general turn lost its exact admitted V2 authority: context=%#v err=%v", securityContext, err)
	}
	errorItem := providerAttemptErrorItem(turn)
	if stringField(errorItem, "code") != "provider_error" ||
		stringField(errorItem, "message") != "The turn failed before a verified response was available." {
		t.Fatalf("general authority drift terminal is not deterministic: %#v", errorItem)
	}
}

func TestValidateCurrentProviderAttemptRejectsPostAppendGeneralRiskWitnessStaleness(t *testing.T) {
	workspace := t.TempDir()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "provider-attempt-risk", BaseURL: "https://provider.invalid/v1", APIKey: "provider-key",
		Model: "general-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	handler.caseThreads = &caseThreadAuthorityStub{}
	countingProvider := &admissionFailureCountingProvider{}
	handler.provider = countingProvider
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "general risk witness stale", "workspace": workspace,
		"providerId": "provider-attempt-risk", "model": "general-model",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	riskAuthority := &providerAttemptPostAppendRiskAuthority{
		base: newVisionPostCommitRiskAuthority(), store: handler.store, threadID: threadID,
	}
	handler.turnSecurity = turnsecurityapp.WorkspaceSecurityAuthority{Identity: testIdentityAuthority(),
		Observer: filestore.CaseBindingReader{}, RiskAuthority: riskAuthority,
	}

	_, startErr := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
		Prompt: "Summarize this generic note.", ProviderID: "provider-attempt-risk", Model: "general-model",
	})
	if startErr == nil {
		t.Fatal("post-append risk witness staleness must reject the provider attempt")
	}
	if calls := countingProvider.calls.Load(); calls != 0 {
		t.Fatalf("stale general risk witness reached Provider.Stream: calls=%d", calls)
	}
	if riskAuthority.preAppendValidations.Load() < 2 || riskAuthority.postAppendValidations.Load() != 1 {
		t.Fatalf("risk witness did not become stale only after durable append: pre=%d post=%d",
			riskAuthority.preAppendValidations.Load(), riskAuthority.postAppendValidations.Load())
	}
	reloaded, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turns := listAny(reloaded["turns"])
	if len(turns) != 1 {
		t.Fatalf("risk witness staleness did not preserve exactly one durable turn: %#v", turns)
	}
	turn, _ := turns[0].(map[string]any)
	if stringField(turn, "status") != "failed" {
		t.Fatalf("risk witness staleness did not deterministically close the turn: startErr=%v turn=%#v", startErr, turn)
	}
	securityContext, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) != nil ||
		!domainsecurity.TurnSecurityContextIsGeneral(securityContext) || securityContext.ThreadID != threadID {
		t.Fatalf("failed risk-witness turn lost its exact admitted V2 authority: context=%#v err=%v", securityContext, err)
	}
	errorItem := providerAttemptErrorItem(turn)
	if stringField(errorItem, "code") != "provider_error" ||
		stringField(errorItem, "message") != "The turn failed before a verified response was available." {
		t.Fatalf("risk witness staleness terminal is not deterministic: %#v", errorItem)
	}
}

func TestGeneralTerminalCASRevalidatesCurrentRiskAfterProviderReturns(t *testing.T) {
	workspace := t.TempDir()
	durableRoot := t.TempDir()
	dataDir := t.TempDir()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: dataDir,
		ProviderID: "provider-final-risk", BaseURL: "https://provider.invalid/v1", APIKey: "provider-key",
		Model: "general-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	base := newServerTestRiskAuthority()
	riskAuthority := &finalizationStaleRiskAuthority{base: base}
	handler.turnSecurity = turnsecurityapp.WorkspaceSecurityAuthority{Identity: testIdentityAuthority(),
		Observer: filestore.CaseBindingReader{}, RiskAuthority: riskAuthority,
	}
	caseAuthority := &caseThreadAuthorityStub{threads: map[string]bool{}}
	handler.caseThreads = caseAuthority
	handler.store.SetCaseThreadAuthority(caseAuthority)
	const sentinel = "甲公司向乙公司交付2645472件材料。张三与李四曾在同一项目工作。"
	if domainsecurity.ContainsProtectedCaseFactCandidate(sentinel) {
		t.Fatal("regression sentinel must bypass the defense-in-depth text guard to exercise host provenance")
	}
	malicious := &finalizationStalingProvider{authority: riskAuthority, text: sentinel}
	handler.provider = malicious
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "general final authority race", "workspace": workspace,
		"providerId": "provider-final-risk", "model": "general-model",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	if _, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
		Prompt: "Summarize this generic note.", ProviderID: "provider-final-risk", Model: "general-model",
	}); err == nil {
		t.Fatal("stale post-provider risk authority reached general terminal CAS")
	}
	if malicious.calls.Load() != 1 {
		t.Fatalf("provider calls = %d", malicious.calls.Load())
	}
	reloaded, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(reloaded)
	if strings.Contains(string(body), sentinel) {
		t.Fatalf("rejected provider candidate entered durable thread: %s", body)
	}
	replay, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	events, _ := json.Marshal(replay.Events)
	if strings.Contains(string(events), sentinel) {
		t.Fatalf("rejected provider candidate entered event replay: %s", events)
	}
}

func TestRevokedCaseProbeSettlesSourceUnavailableThroughFinalEvidenceGate(t *testing.T) {
	durableRoot := t.TempDir()
	privateRoot := t.TempDir()
	if err := os.Chmod(privateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	workspace := writeThreadMutationCaseBinding(t)
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: t.TempDir(),
		ProviderID: "provider-attempt-case", BaseURL: "https://provider.invalid/v1", APIKey: "provider-key",
		Model: "case-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)

	caseKeyRoot := filepath.Join(privateRoot, "case-key")
	finalKeyRoot := filepath.Join(privateRoot, "final-key")
	for _, root := range []string{caseKeyRoot, finalKeyRoot} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	caseSigner, err := finalauthorityadapter.OpenOrCreateFileAuthority(filepath.Join(caseKeyRoot, "authority.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	caseStore, err := newServerTestCaseThreadStore(t, filepath.Join(privateRoot, "case-authority-records"))
	if err != nil {
		t.Fatal(err)
	}
	caseRegistry, err := casethreadapp.NewRegistry(context.Background(), caseSigner, caseStore)
	if err != nil {
		t.Fatal(err)
	}
	source := &admissionFailureMCP{}
	caseAuthority := &providerAttemptRevokingCaseAuthority{CommittedAuthority: caseRegistry, source: source}
	handler.caseThreads = caseAuthority
	handler.store.SetCaseThreadAuthority(caseAuthority)

	finalSigner, err := finalauthorityadapter.OpenOrCreateFileAuthority(filepath.Join(finalKeyRoot, "authority.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	evidenceRegistry, err := evidenceregistryadapter.NewStore(filepath.Join(privateRoot, "evidence-registry"), finalSigner)
	if err != nil {
		t.Fatal(err)
	}
	privateFinals, err := newServerTestPrivateFinalStore(t, filepath.Join(privateRoot, "accepted-finals"))
	if err != nil {
		t.Fatal(err)
	}
	casReader, err := finalauthorityadapter.NewAcceptedFinalCASReader(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	handler.caseFinalizer = evidenceapp.NewCasePublicationFinalizerWithAuthority(
		evidenceRegistry, evidenceRegistry, finalSigner, privateFinals,
		durableAcceptedFinalEventIO(handler.store, casReader), newServerTestTurnTerminalCoordinator(t, finalSigner, privateFinals),
	)

	thread, err := handler.store.CreateThread(map[string]any{
		"title": "case provider probe revoked", "workspace": workspace,
		"providerId": "provider-attempt-case", "model": "case-model",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	observer := filestore.CaseBindingReader{}
	observation, err := observer.Observe(workspace)
	if err != nil || observation.State != domainsecurity.CaseBindingStateValid {
		t.Fatalf("case binding fixture is invalid: observation=%#v err=%v", observation, err)
	}
	snapshotAuthority := newAdmissionFailureSnapshotAuthority(t, observation)
	handler.turnSecurity = turnsecurityapp.WorkspaceSecurityAuthority{Identity: testIdentityAuthority(),
		Observer: observer, RiskAuthority: newAdmissionFailureRiskAuthority(), SnapshotAuthority: snapshotAuthority, SnapshotAuthorityV2: snapshotAuthority,
	}
	handler.mcp = source
	countingProvider := &admissionFailureCountingProvider{}
	handler.provider = countingProvider
	handler.providerConfig = provider.NewRuntimeProviderConfigSet(provider.RuntimeProviderConfigInput{
		DefaultProviderID: "provider-attempt-case", DefaultBaseURL: "https://provider.invalid/v1",
		DefaultAPIKey: "provider-key", DefaultEndpointFormat: "chat_completions", DefaultModel: "case-model",
	})

	_, startErr := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
		Prompt: "分析当前案件账户在指定期间的流入、流出、净额和交易笔数。", RiskIntent: domainsecurity.RiskClassCase,
		ProviderID: "provider-attempt-case", Model: "case-model",
	})
	if startErr != nil {
		t.Fatalf("revoked case source should settle a host boundary without failing ordinary runtime state: %v", startErr)
	}
	if calls := countingProvider.calls.Load(); calls != 0 {
		t.Fatalf("revoked case source reached Provider.Stream: calls=%d", calls)
	}
	if caseAuthority.commits.Load() != 1 || source.probeCalls.Load() != 1 || source.currentValidations.Load() != 0 {
		t.Fatalf("case source was not revoked at the exact append/commit cut: commits=%d probes=%d validations=%d",
			caseAuthority.commits.Load(), source.probeCalls.Load(), source.currentValidations.Load())
	}

	reloaded, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turns := listAny(reloaded["turns"])
	if len(turns) != 1 {
		t.Fatalf("revoked case probe did not preserve exactly one durable turn: %#v", turns)
	}
	turn, _ := turns[0].(map[string]any)
	if stringField(turn, "status") != "completed" {
		t.Fatalf("revoked case probe did not deterministically close the turn: %#v", turn)
	}
	securityContext, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) != nil ||
		!domainsecurity.TurnSecurityContextAllowsCaseEvidence(securityContext) || securityContext.ThreadID != threadID {
		t.Fatalf("failed case turn lost its exact evidence-ready V2 authority: context=%#v err=%v", securityContext, err)
	}
	state, ok, err := contextepochapp.StateFromThread(reloaded)
	if err != nil || !ok || state.AcceptedSnapshot.Epoch != securityContext.ContextEpoch {
		t.Fatalf("revoked case probe lost its committed epoch: state=%#v ok=%t err=%v", state, ok, err)
	}
	committed, ok := caseRegistry.CommittedContext(threadID, securityContext.TurnID)
	if !ok || committed.SecurityContext != securityContext || committed.EpochState.StateDigest != state.StateDigest ||
		!caseAuthority.ContainsContext(securityContext) {
		t.Fatalf("case probe was revoked before exact case authority commit: committed=%#v ok=%t", committed, ok)
	}

	acceptedFinal, err := domainevidence.ParseAcceptedFinalRecord(turn["acceptedFinal"])
	if err != nil || acceptedFinal.TerminalReason != string(evidenceapp.TerminalSourceUnavailable) ||
		acceptedFinal.Variant != domainevidence.SourceUnavailableAnswer || acceptedFinal.ContextDigest != securityContext.ContextDigest ||
		acceptedFinal.FinalGateVersion != domainevidence.FinalEvidenceGateVersion || acceptedFinal.RegistrySequence != 0 {
		t.Fatalf("revoked case source bypassed the Final Evidence Gate: accepted=%#v err=%v", acceptedFinal, err)
	}
	privateFinal, err := privateFinals.Resolve(context.Background(), acceptedFinal.RecordDigest)
	if err != nil || domainevidence.ValidateFinalAnswerEnvelope(privateFinal.Envelope) != nil ||
		privateFinal.Envelope.ContextDigest != securityContext.ContextDigest ||
		len(privateFinal.Envelope.Claims) != 0 || len(privateFinal.Envelope.EvidenceReceiptIDs) != 0 {
		t.Fatalf("revoked source final is not an evidence-free host boundary: final=%#v err=%v", privateFinal, err)
	}
	expectedText, err := domainevidence.RenderFinalAnswer(privateFinal.Envelope)
	if err != nil {
		t.Fatal(err)
	}
	if finalText := acceptedFinalAssistantText(turn); finalText != expectedText {
		t.Fatalf("revoked source final was not deterministic host rendering: got=%q want=%q", finalText, expectedText)
	}
	if strings.Contains(expectedText, "provider.invalid") || strings.Contains(expectedText, "case-model") {
		t.Fatalf("host boundary leaked provider-controlled diagnostics: %q", expectedText)
	}
}

func TestValidateCurrentProviderAttemptKeepsOrdinaryWorkAfterDatasetRevocation(t *testing.T) {
	workspace := writeThreadMutationCaseBinding(t)
	observer := filestore.CaseBindingReader{}
	observation, err := observer.Observe(workspace)
	if err != nil || observation.State != domainsecurity.CaseBindingStateValid {
		t.Fatalf("case binding fixture is invalid: observation=%#v err=%v", observation, err)
	}
	snapshot := newAdmissionFailureSnapshotAuthority(t, observation)
	risk := newServerTestRiskAuthority()
	authority := turnsecurityapp.WorkspaceSecurityAuthority{
		Identity: testIdentityAuthority(), Observer: observer, RiskAuthority: risk,
		SnapshotAuthority: snapshot, SnapshotAuthorityV2: snapshot,
	}
	securityContext, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: context.Background(), Authority: authority, Thread: map[string]any{},
		ThreadID: "thread-additive-provider", TurnID: "turn-additive-provider", Workspace: workspace,
		Principal: testIdentityPrincipal(), IssuedAt: time.Date(2026, 7, 26, 16, 0, 0, 0, time.UTC),
	})
	if err != nil || !domainsecurity.TurnSecurityContextAllowsCaseEvidence(securityContext) {
		t.Fatalf("case provider fixture is invalid: context=%#v err=%v", securityContext, err)
	}
	snapshot.resolved.Record.DatasetSnapshotID = securitycontexttest.DatasetSnapshotID("revoked-provider-snapshot")
	handler := &runtimeServerHandler{turnSecurity: authority}
	ordinary := runtimeAgentLoopInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Request:         startRuntimeTurnRequest{Prompt: "修改当前源码中的注释并运行普通单元测试。"},
		SecurityContext: securityContext,
	}
	if err := handler.validateCurrentProviderAttempt(context.Background(), ordinary, false, false); err != nil {
		t.Fatalf("dataset revocation disabled an ordinary provider effect: %v", err)
	}
	caseRequest := ordinary
	caseRequest.Request.Prompt = "分析当前案件账户的流入、流出、净额和交易笔数。"
	if err := handler.validateCurrentProviderAttempt(context.Background(), caseRequest, true, false); err == nil {
		t.Fatal("dataset revocation still authorized a case-data provider effect")
	}
}

func providerAttemptErrorItem(turn map[string]any) map[string]any {
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		if stringField(item, "kind") == "error" {
			return item
		}
	}
	return nil
}
