package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
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
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
	provider "analytix.local/runtime-go/internal/provider"
	datasetsnapshotv2fixture "analytix.local/runtime-go/internal/testsupport/datasetsnapshotv2fixture"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type admissionFailureCountingProvider struct {
	calls atomic.Int64
}

func (*admissionFailureCountingProvider) RequiresDurablePipelineStagesV1() {}

func (provider *admissionFailureCountingProvider) Stream(context.Context, domainmodel.Request) (domainmodel.Result, error) {
	provider.calls.Add(1)
	return domainmodel.Result{}, errors.New("provider must not run after admission failure")
}

type admissionFailureRiskAuthority struct {
	privateKey ed25519.PrivateKey

	mu        sync.Mutex
	policy    domainsecurity.ThreadRiskPolicyV1
	contracts securitycontexttest.RiskAuthorityContracts
}

func newAdmissionFailureRiskAuthority() *admissionFailureRiskAuthority {
	return &admissionFailureRiskAuthority{
		privateKey: ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x61}, ed25519.SeedSize)),
	}
}

func (authority *admissionFailureRiskAuthority) ResolveOrRaise(_ context.Context, input threadriskauthorityapp.ResolveOrRaiseInput) (threadriskauthorityapp.Head, error) {
	if input.RequestedRisk != domainsecurity.RiskClassCase {
		return threadriskauthorityapp.Head{}, errors.New("admission fixture requires case risk")
	}
	publicKey := authority.privateKey.Public().(ed25519.PublicKey)
	policy, err := domainsecurity.NewThreadRiskPolicyV1(domainsecurity.ThreadRiskPolicyInputV1{
		ThreadID: input.ThreadID, WorkspaceRealPath: input.WorkspaceRealPath,
		RiskClass: input.RequestedRisk, Origin: input.Origin, SignalsDigest: input.SignalsDigest,
		IssuedAt: input.IssuedAt, AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) {
		return ed25519.Sign(authority.privateKey, message), nil
	})
	if err != nil {
		return threadriskauthorityapp.Head{}, err
	}
	contracts, err := securitycontexttest.WitnessedRiskAuthorityContracts(
		input.ThreadID, input.WorkspaceRealPath, input.RequestedRisk, policy.PolicyDigest,
	)
	if err != nil {
		return threadriskauthorityapp.Head{}, err
	}
	authority.mu.Lock()
	authority.policy = policy
	authority.contracts = contracts
	authority.mu.Unlock()
	return threadriskauthorityapp.Head{
		HasIndex: true, Index: contracts.Index, Request: contracts.Request, Observation: contracts.Observation,
		RiskAuthorityBinding: contracts.Binding, Policy: policy, Found: true,
	}, nil
}

func (authority *admissionFailureRiskAuthority) ValidateCurrent(_ context.Context, securityContext domainsecurity.TurnSecurityContext) error {
	authority.mu.Lock()
	policy := authority.policy
	contracts := authority.contracts
	authority.mu.Unlock()
	if domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) != nil ||
		policy.ThreadID != securityContext.ThreadID || policy.WorkspaceRealPath != securityContext.WorkspaceRealPath ||
		domainsecurity.ValidateTurnPublicationPolicyForThreadRiskPolicyV1(securityContext.PublicationPolicy, policy) != nil ||
		domainsecurity.ValidateWitnessedRiskAuthorityBindingV1(
			securityContext.RiskAuthorityBinding, contracts.Index, contracts.Request, contracts.Observation,
		) != nil {
		return errors.New("current witnessed risk authority does not match the admitted turn")
	}
	return nil
}

type admissionFailureSnapshotAuthority struct {
	record   domainsecurity.DatasetSnapshotAuthorityRecordV1
	resolved datasetsnapshotport.ResolvedSnapshotV2
	calls    atomic.Int64
}

func (authority *admissionFailureSnapshotAuthority) ResolveWitnessedV2(ctx context.Context, input datasetsnapshotport.ResolveInputV2) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	authority.calls.Add(1)
	if ctx == nil || ctx.Err() != nil ||
		domainsecurity.ValidateDatasetSnapshotAuthorityRecordForBindingV2(
			authority.resolved.Record, input.TenantID, input.UserID, input.Observation,
		) != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrMismatch
	}
	if input.ExpectedDatasetSnapshotID != "" && input.ExpectedDatasetSnapshotID != authority.resolved.Record.DatasetSnapshotID {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrStale
	}
	return authority.resolved, nil
}

func (authority *admissionFailureSnapshotAuthority) ResolveWitnessed(ctx context.Context, input datasetsnapshotport.ResolveInput) (domainsecurity.DatasetSnapshotAuthorityRecordV1, error) {
	authority.calls.Add(1)
	if ctx == nil || ctx.Err() != nil ||
		domainsecurity.ValidateDatasetSnapshotAuthorityRecordForBindingV1(authority.record, input.Observation) != nil ||
		authority.record.TenantID != input.TenantID || authority.record.UserID != input.UserID {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, errors.New("current dataset snapshot does not match the admitted case binding")
	}
	return authority.record, nil
}

type admissionFailureMCP struct {
	probeCalls         atomic.Int64
	currentValidations atomic.Int64

	mu      sync.Mutex
	current domainsecurity.VerifiedSourceProbe
	revoked bool
}

func (*admissionFailureMCP) Connect()                    {}
func (*admissionFailureMCP) Disconnect()                 {}
func (*admissionFailureMCP) Diagnostics() map[string]any { return map[string]any{} }
func (*admissionFailureMCP) ServerDiagnostics() []any    { return nil }
func (*admissionFailureMCP) Search(string) []string      { return nil }
func (*admissionFailureMCP) CallTool(string, bool, ...map[string]any) map[string]any {
	return map[string]any{"isError": true}
}
func (*admissionFailureMCP) RefreshCatalog() map[string]any   { return map[string]any{} }
func (*admissionFailureMCP) RestartReconnect() map[string]any { return map[string]any{} }
func (*admissionFailureMCP) Tools() []string                  { return nil }
func (*admissionFailureMCP) MCPToolAdvertisementSnapshotV1(domainsecurity.TurnSecurityContext) []domainmcp.ToolAdvertisementV1 {
	return nil
}
func (*admissionFailureMCP) Prompts() []any                                  { return nil }
func (*admissionFailureMCP) Resources() []any                                { return nil }
func (*admissionFailureMCP) ToolReadOnlyHint(string) bool                    { return true }
func (*admissionFailureMCP) ToolInputSchema(string) (json.RawMessage, bool)  { return nil, false }
func (*admissionFailureMCP) ToolOutputSchema(string) (json.RawMessage, bool) { return nil, false }
func (*admissionFailureMCP) ToolDescription(string) (string, bool)           { return "", false }

func (source *admissionFailureMCP) ProbeCaseSource(_ context.Context, input sourceprobeport.Input) (domainsecurity.VerifiedSourceProbe, error) {
	source.probeCalls.Add(1)
	checkedAt := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	response := domainsecurity.SourceProbeResponse{
		Version: domainsecurity.SourceProbeVersion, ServerName: "analytix_funds", ServerVersion: "0.16.15",
		CaseID: input.CaseID, CaseBindingHash: input.CaseBindingHash, DatasetSnapshotID: input.DatasetSnapshotID,
		Ready: true, ReadOnly: true, CheckedAt: checkedAt.Format(time.RFC3339Nano),
	}
	identity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		input.ServerID, response.ServerName, response.ServerVersion,
		domainsecurity.SHA256Hex([]byte("turn-start-admission-failure-source-instance")), 11,
	)
	if err != nil {
		return domainsecurity.VerifiedSourceProbe{}, err
	}
	probe, err := domainsecurity.NewVerifiedSourceProbe(domainsecurity.VerifiedSourceProbeInput{
		ServerID: input.ServerID, ServerIdentity: identity, ConnectionEpoch: 11,
		CatalogFingerprint: domainsecurity.SHA256Hex([]byte("turn-start-admission-failure-catalog")),
		SpecFingerprint:    domainsecurity.SHA256Hex([]byte("turn-start-admission-failure-spec")),
		ThreadID:           input.ThreadID, TurnID: input.TurnID, ContextEpoch: input.ContextEpoch,
		ContextDigest: input.ContextDigest, DatasetSnapshotID: input.DatasetSnapshotID,
		CheckedAt: checkedAt, Response: response,
	})
	if err != nil {
		return domainsecurity.VerifiedSourceProbe{}, err
	}
	source.mu.Lock()
	source.current = probe
	source.revoked = false
	source.mu.Unlock()
	return probe, nil
}

func (source *admissionFailureMCP) ValidateCurrentProbe(ctx context.Context, serverID string, securityContext domainsecurity.TurnSecurityContext) error {
	source.currentValidations.Add(1)
	if ctx == nil || ctx.Err() != nil {
		return errors.New("current case source validation context is unavailable")
	}
	source.mu.Lock()
	probe := source.current
	revoked := source.revoked
	source.mu.Unlock()
	if revoked || serverID != probe.ServerID || !turnsecurityapp.SourceDiscoveryMatchesContext(probe, securityContext) {
		return errors.New("current case source connection was revoked")
	}
	return nil
}

func (source *admissionFailureMCP) revokeCurrentProbe() {
	source.mu.Lock()
	source.revoked = true
	source.mu.Unlock()
}

func TestCaseAdmissionFailureCommitsSignedTurnAndHostBoundaryBeforeRestart(t *testing.T) {
	durableRoot := t.TempDir()
	dataRoot := t.TempDir()
	privateRoot := t.TempDir()
	if err := os.Chmod(privateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	caseKeyRoot := filepath.Join(privateRoot, "case-key")
	finalKeyRoot := filepath.Join(privateRoot, "final-key")
	for _, root := range []string{caseKeyRoot, finalKeyRoot} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	workspace := writeThreadMutationCaseBinding(t)
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: dataRoot,
		ProviderID: "initial-provider", BaseURL: "https://provider.invalid", APIKey: "initial-key",
		Model: "initial-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)

	caseAuthorityPath := filepath.Join(caseKeyRoot, "authority.json")
	caseSigner, err := finalauthorityadapter.OpenOrCreateFileAuthority(caseAuthorityPath, false)
	if err != nil {
		t.Fatal(err)
	}
	caseAuthorityRoot := filepath.Join(privateRoot, "case-authority-records")
	caseStore, err := newServerTestCaseThreadStore(t, caseAuthorityRoot)
	if err != nil {
		t.Fatal(err)
	}
	caseAuthority, err := casethreadapp.NewRegistry(context.Background(), caseSigner, caseStore)
	if err != nil {
		t.Fatal(err)
	}
	handler.caseThreads = caseAuthority
	handler.store.SetCaseThreadAuthority(caseAuthority)

	finalSigner, err := finalauthorityadapter.OpenOrCreateFileAuthority(
		filepath.Join(finalKeyRoot, "authority.json"), false,
	)
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
		"title": "case admission failure", "workspace": workspace,
		"providerId": "broken-provider", "model": "broken-model",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	observer := filestore.CaseBindingReader{}
	observation, err := observer.Observe(workspace)
	if err != nil || observation.State != domainsecurity.CaseBindingStateValid {
		t.Fatalf("case binding fixture is not valid: observation=%#v err=%v", observation, err)
	}
	snapshotAuthority := newAdmissionFailureSnapshotAuthority(t, observation)
	handler.turnSecurity = turnsecurityapp.WorkspaceSecurityAuthority{Identity: testIdentityAuthority(),
		Observer: observer, RiskAuthority: newAdmissionFailureRiskAuthority(), SnapshotAuthority: snapshotAuthority, SnapshotAuthorityV2: snapshotAuthority,
	}
	source := &admissionFailureMCP{}
	handler.mcp = source
	countingProvider := &admissionFailureCountingProvider{}
	handler.provider = countingProvider
	handler.providerConfig = provider.NewRuntimeProviderConfigSet(provider.RuntimeProviderConfigInput{
		DefaultProviderID: "broken-provider", DefaultBaseURL: "https://provider.invalid",
		DefaultEndpointFormat: "chat_completions", DefaultModel: "broken-model",
		// Deliberately no API key: provider admission must fail after the
		// signed case context has been registered and the live source probed.
	})

	response, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
		Prompt: "perform the evidence-ready case analysis", ProviderID: "broken-provider", Model: "broken-model",
	})
	if err != nil {
		t.Fatalf("case admission failure did not close through the host boundary: %v", err)
	}
	turnID := stringField(response, "turnId")
	if turnID == "" {
		t.Fatalf("case admission failure lost the durable turn identity: %#v", response)
	}
	if calls := countingProvider.calls.Load(); calls != 0 {
		t.Fatalf("provider ran after provider-config admission failed: calls=%d", calls)
	}
	if calls := source.probeCalls.Load(); calls != 1 {
		t.Fatalf("case-ready turn did not run exactly one current source probe: calls=%d", calls)
	}
	if calls := snapshotAuthority.calls.Load(); calls < 2 {
		t.Fatalf("snapshot authority was not revalidated before commit: calls=%d", calls)
	}

	reloaded, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turns := listAny(reloaded["turns"])
	if len(turns) != 1 {
		t.Fatalf("admission failure did not leave exactly one durable turn: %#v", turns)
	}
	turn, _ := turns[0].(map[string]any)
	if stringField(turn, "id") != turnID || stringField(turn, "status") != "failed" {
		t.Fatalf("admission failure turn is not durably terminal: %#v", turn)
	}
	securityContext, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) != nil ||
		securityContext.CaseID != observation.CaseID || securityContext.DatasetSnapshotID != snapshotAuthority.resolved.Record.DatasetSnapshotID {
		t.Fatalf("admission failure lost the exact evidence-ready security context: context=%#v err=%v", securityContext, err)
	}
	state, ok, err := contextepochapp.StateFromThread(reloaded)
	if err != nil || !ok || state.AcceptedSnapshot.Epoch != securityContext.ContextEpoch {
		t.Fatalf("admission failure did not commit the exact context epoch: state=%#v ok=%t err=%v", state, ok, err)
	}
	committed, ok := caseAuthority.CommittedContext(threadID, turnID)
	if !ok || committed.SecurityContext != securityContext || committed.EpochState.StateDigest != state.StateDigest ||
		!caseAuthority.ContainsContext(securityContext) {
		t.Fatalf("signed case authority was staged but not committed: committed=%#v ok=%t", committed, ok)
	}

	acceptedFinal, err := domainevidence.ParseAcceptedFinalRecord(turn["acceptedFinal"])
	if err != nil || acceptedFinal.TerminalReason != string(evidenceapp.TerminalProviderFailure) ||
		acceptedFinal.Variant != domainevidence.NeedsEvidenceAnswer || acceptedFinal.RegistrySequence != 0 ||
		acceptedFinal.ContextDigest != securityContext.ContextDigest || acceptedFinal.FinalGateVersion != domainevidence.FinalEvidenceGateVersion {
		t.Fatalf("admission failure bypassed the host Final Evidence Gate: accepted=%#v err=%v", acceptedFinal, err)
	}
	privateFinal, err := privateFinals.Resolve(context.Background(), acceptedFinal.RecordDigest)
	if err != nil || domainevidence.ValidateFinalAnswerEnvelope(privateFinal.Envelope) != nil ||
		privateFinal.Envelope.ContextDigest != securityContext.ContextDigest ||
		len(privateFinal.Envelope.Claims) != 0 || len(privateFinal.Envelope.EvidenceReceiptIDs) != 0 {
		t.Fatalf("private host boundary is not an evidence-free provider-failure envelope: final=%#v err=%v", privateFinal, err)
	}
	expectedText, err := domainevidence.RenderFinalAnswer(privateFinal.Envelope)
	if err != nil {
		t.Fatal(err)
	}
	if finalText := acceptedFinalAssistantText(turn); finalText != expectedText {
		t.Fatalf("public final was not the deterministic host rendering: got=%q want=%q", finalText, expectedText)
	}

	restartedStore, err := NewTempDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	restartedSigner, err := finalauthorityadapter.OpenOrCreateFileAuthority(caseAuthorityPath, true)
	if err != nil {
		t.Fatal(err)
	}
	restartedCaseStore, err := newServerTestCaseThreadStore(t, caseAuthorityRoot)
	if err != nil {
		t.Fatal(err)
	}
	restartedAuthority, err := casethreadapp.NewRegistry(context.Background(), restartedSigner, restartedCaseStore)
	if err != nil {
		t.Fatal(err)
	}
	restartedStore.SetCaseThreadAuthority(restartedAuthority)
	restartedThread, err := restartedStore.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	if err := casethreadapp.ValidateRestartState(restartedAuthority, threadID, restartedThread); err != nil {
		t.Fatalf("restart treated the committed admission-failure turn as orphaned staged authority: %v", err)
	}
	inventory, err := casethreadapp.PreflightRestartInventory(restartedAuthority, restartedStore)
	if err != nil || len(inventory.Quarantined) != 0 {
		t.Fatalf("restart preflight quarantined the committed admission-failure turn: inventory=%#v err=%v", inventory, err)
	}
	if err := casethreadapp.ApplyRestartInventory(restartedAuthority, inventory); err != nil || !restartedAuthority.CanExecute(threadID) {
		t.Fatalf("restart did not preserve executable case authority: canExecute=%t err=%v", restartedAuthority.CanExecute(threadID), err)
	}
	restartedCommitted, ok := restartedAuthority.CommittedContext(threadID, turnID)
	if !ok || restartedCommitted.SecurityContext != securityContext || restartedCommitted.EpochState.StateDigest != state.StateDigest {
		t.Fatalf("restart lost the committed signed context: committed=%#v ok=%t", restartedCommitted, ok)
	}
}

func newAdmissionFailureSnapshotAuthority(t *testing.T, observation domainsecurity.CaseBindingObservationV1) *admissionFailureSnapshotAuthority {
	t.Helper()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x62}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	record, err := domainsecurity.NewDatasetSnapshotAuthorityRecordV1(domainsecurity.DatasetSnapshotAuthorityRecordInputV1{
		InstallationID: domainsecurity.SHA256Hex([]byte("turn-start-admission-failure-installation")),
		TenantID:       domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		WorkspaceRealPath: observation.WorkspaceRealPath, CaseID: observation.CaseID,
		CaseBindingHash: observation.CaseBindingHash, BindingObservationDigest: observation.ObservationDigest,
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("turn-start-admission-failure-source-manifest")),
		RawManifestSHA256:  domainsecurity.SHA256Hex([]byte("turn-start-admission-failure-raw-manifest")),
		ParserVersion:      "turn-start-admission-failure-parser-v1",
		AcceptedAt:         time.Date(2026, 7, 12, 11, 0, 0, 0, time.UTC),
		AuthorityKeyID:     domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := datasetsnapshotv2fixture.NewResolvedSnapshotV2(datasetsnapshotv2fixture.ResolvedInput{
		InstallationID: domainsecurity.SHA256Hex([]byte("turn-start-admission-failure-installation")),
		TenantID:       domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		Observation: observation, Material: "turn-start-admission-failure",
		AcceptedAt:     time.Date(2026, 7, 12, 11, 0, 0, 0, time.UTC),
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
		Sign: func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return &admissionFailureSnapshotAuthority{record: record, resolved: resolved}
}

func acceptedFinalAssistantText(turn map[string]any) string {
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		if stringField(item, "kind") == "assistant_text" &&
			(item["acceptedFinal"] != nil || item["acceptedFinalView"] != nil) {
			return stringField(item, "text")
		}
	}
	return ""
}
