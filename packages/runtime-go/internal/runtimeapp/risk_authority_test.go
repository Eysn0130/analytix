package runtimeapp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	threadriskpolicystore "analytix.local/runtime-go/internal/adapters/outbound/threadriskpolicy"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appidentity "analytix.local/runtime-go/internal/app/identity"
	threadriskauthorityapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

type fakeLocalRiskMarkerAuthority struct{}

func (fakeLocalRiskMarkerAuthority) KeyID() string {
	return domainsecurity.SHA256Hex([]byte("fake-local-marker"))
}
func (fakeLocalRiskMarkerAuthority) PublicKey() []byte { return []byte("not-an-ed25519-authority") }
func (fakeLocalRiskMarkerAuthority) Sign(context.Context, []byte) ([]byte, error) {
	return []byte("forged-local-signature"), nil
}
func (fakeLocalRiskMarkerAuthority) VerifyTrusted(context.Context, string, []byte, []byte, []byte) error {
	return errors.New("fake local marker is never trusted")
}

func TestRuntimeGeneralOnlyAuthorityAdmitsOrdinaryTurnAndRevalidatesAfterRestart(t *testing.T) {
	workspace := t.TempDir()
	bankCSV := filepath.Join(workspace, "bank.csv")
	if err := os.WriteFile(bankCSV, []byte("account,amount\n6222020202020202020,999999.99\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := newRuntimeThreadRiskAuthority(Config{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	identityAuthority, err := appidentity.NewInstallationLocalAuthority(
		domainsecurity.SHA256Hex([]byte("runtime-general-test-installation")),
	)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := identityAuthority.ResolveCurrent(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: context.Background(), Authority: turnsecurityapp.WorkspaceSecurityAuthority{Identity: identityAuthority,
			Observer: filestore.CaseBindingReader{}, RiskAuthority: first,
		},
		Thread: map[string]any{}, ThreadID: "thread-runtime-general", TurnID: "turn-runtime-general",
		Workspace: workspace, Principal: principal, IssuedAt: time.Now().UTC(),
	})
	if err != nil || !domainsecurity.TurnSecurityContextUsesHostGeneralOnlyRisk(securityContext) ||
		domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) != nil {
		t.Fatalf("runtime composition did not admit ordinary general turn: context=%#v err=%v", securityContext, err)
	}
	if securityContext.DatasetSnapshotID != domainsecurity.NoDatasetSnapshotID ||
		securityContext.SourceManifestHash != domainsecurity.EmptySourceManifestHash ||
		securityContext.RiskAuthorityBinding.IndexDigest != "" {
		t.Fatalf("general-only runtime composition carried snapshot/witness authority: %#v", securityContext)
	}
	callEntropy := sha256.Sum256([]byte("runtime-general-only-bank-read"))
	callID, err := domainmodel.NewHostToolCallIDV1(callEntropy[:])
	if err != nil {
		t.Fatal(err)
	}
	call := domainmodel.ToolCall{ID: callID, Name: "read_file", Arguments: json.RawMessage(`{"path":"` + bankCSV + `"}`)}
	schemas := []domainmodel.ToolSchema{{
		Name:       "read_file",
		Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`),
	}}
	if _, err := executiongrantapp.IssueProvider(
		securityContext, "provider", call, schemas, []string{"read_file"}, true, "not_required", 0, "", time.Now().UTC(),
	); err != nil {
		t.Fatalf("unavailable case authority disabled an ordinary workspace read: %v", err)
	}
	fundsName := "mcp__analytix_funds__count_case_rows"
	fundsCallEntropy := sha256.Sum256([]byte("runtime-fallback-funds-read"))
	fundsCallID, err := domainmodel.NewHostToolCallIDV1(fundsCallEntropy[:])
	if err != nil {
		t.Fatal(err)
	}
	fundsIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"analytix_funds", "analytix_funds", "1.0.0",
		domainsecurity.SHA256Hex([]byte("runtime-fallback-funds-instance")), 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	fundsCall := domainmodel.ToolCall{ID: fundsCallID, Name: fundsName, Arguments: json.RawMessage(`{}`)}
	if _, err := executiongrantapp.IssueProvider(
		securityContext, "provider", fundsCall,
		[]domainmodel.ToolSchema{{Name: fundsName, Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`)}},
		[]string{fundsName}, true, "not_required", 1, fundsIdentity, time.Now().UTC(),
	); err == nil || err.Error() != "execution_grant_case_authority_required" {
		t.Fatalf("general-only authority granted a funds effect without case authority: %v", err)
	}
	// A caller-provided local signer/JSON-marker analogue cannot mint or alter
	// this policy. Production composition ignores it unless an independently
	// enrolled witness is explicitly available.
	restarted, err := newRuntimeThreadRiskAuthority(Config{}, fakeLocalRiskMarkerAuthority{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := turnsecurityapp.ValidateCurrent(turnsecurityapp.CurrentValidationInput{
		OperationContext: context.Background(), Identity: identityAuthority,
		Observer: filestore.CaseBindingReader{}, RiskAuthority: restarted,
		Context: securityContext, Workspace: workspace,
	}); err != nil {
		t.Fatalf("restarted runtime could not deterministically revalidate general-only authority: %v", err)
	}
}

func TestRuntimeProductionRiskCompositionUsesInstalledHostPolicyAndOnePrivateCAS(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	installation, err := finalauthorityadapter.OpenOrCreateFileAuthority(
		filepath.Join(root, "installation-authority.key"),
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	policyRoot := filepath.Join(root, "thread-risk-policy")
	access, err := privatecastest.NewAccessAuthority(policyRoot)
	if err != nil {
		t.Fatal(err)
	}
	policies, err := threadriskpolicystore.NewStore(policyRoot, access)
	if err != nil {
		t.Fatal(err)
	}
	composed, err := newRuntimeThreadRiskAuthority(Config{}, installation, policies)
	if err != nil {
		t.Fatal(err)
	}
	host, ok := composed.(*threadriskauthorityapp.HostPolicyAuthority)
	if !ok {
		t.Fatalf("production risk composition = %T, want *HostPolicyAuthority", composed)
	}

	workspace := t.TempDir()
	metadata := filepath.Join(workspace, ".analytix")
	if err := os.Mkdir(metadata, 0o755); err != nil {
		t.Fatal(err)
	}
	bindingBody, err := json.Marshal(map[string]any{
		"version": 1, "workspaceRoot": workspace, "caseId": "case_host_policy_001",
		"source": "analytix-data-analysis",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metadata, "case-project.json"), bindingBody, 0o644); err != nil {
		t.Fatalf("write case binding: %v", err)
	}
	observation, err := (filestore.CaseBindingReader{}).Observe(workspace)
	if err != nil || observation.State != domainsecurity.CaseBindingStateValid {
		t.Fatalf("observe exact case binding: observation=%#v err=%v", observation, err)
	}
	head, err := host.ResolveOrRaise(ctx, threadriskauthorityapp.ResolveOrRaiseInput{
		ThreadID: "thread-runtime-host-policy", WorkspaceRealPath: observation.WorkspaceRealPath,
		BindingObservation: observation, RequestedRisk: domainsecurity.RiskClassCase,
		Origin:        domainsecurity.RiskPolicyOriginValidCaseBinding,
		SignalsDigest: domainsecurity.SHA256Hex([]byte("runtime-host-policy-signals")),
		IssuedAt:      time.Now().UTC().Round(0),
	})
	if err != nil || !head.Found || head.HasIndex ||
		head.RiskAuthorityBinding.State != domainsecurity.RiskAuthorityBindingStateHostPolicy {
		t.Fatalf("host policy case head = %#v, err=%v", head, err)
	}
	if exists, readErr := policies.HasRecords(ctx); readErr != nil || !exists {
		t.Fatalf("host policy was not persisted in the one private CAS: exists=%v err=%v", exists, readErr)
	}
	restarted, err := newRuntimeThreadRiskAuthority(Config{}, installation, policies)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := restarted.(*threadriskauthorityapp.HostPolicyAuthority); !ok {
		t.Fatalf("restart risk composition = %T, want *HostPolicyAuthority", restarted)
	}

	ordinaryWorkspace := t.TempDir()
	ordinaryObservation, err := (filestore.CaseBindingReader{}).Observe(ordinaryWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := restarted.ResolveOrRaise(ctx, threadriskauthorityapp.ResolveOrRaiseInput{
		ThreadID: "thread-runtime-host-policy-ordinary", WorkspaceRealPath: ordinaryObservation.WorkspaceRealPath,
		BindingObservation: ordinaryObservation, RequestedRisk: domainsecurity.RiskClassGeneral,
		Origin:        domainsecurity.RiskPolicyOriginGeneralWorkspace,
		SignalsDigest: domainsecurity.SHA256Hex([]byte("runtime-host-policy-ordinary")),
		IssuedAt:      time.Now().UTC().Round(0),
	})
	if err != nil || ordinary.RiskAuthorityBinding.State != domainsecurity.RiskAuthorityBindingStateHostGeneralOnly {
		t.Fatalf("ordinary authority was not additive: head=%#v err=%v", ordinary, err)
	}
}
