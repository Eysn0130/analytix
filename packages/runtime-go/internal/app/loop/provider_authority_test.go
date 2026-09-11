package loop

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	threadriskauthorityapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	identityport "analytix.local/runtime-go/internal/ports/identity"
	datasetsnapshotv2fixture "analytix.local/runtime-go/internal/testsupport/datasetsnapshotv2fixture"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestValidateProviderAttemptAuthoritySeparatesFundsSourceFromCaseDataAuthority(t *testing.T) {
	fixture := newProviderAttemptAuthorityFixture(t)

	t.Run("case attachment does not depend on funds source", func(t *testing.T) {
		input := fixture.input
		input.CaseDataEffect = true
		input.FundsDataEffect = false
		input.Source = nil
		if err := ValidateProviderAttemptAuthority(input); err != nil {
			t.Fatalf("validate case attachment authority: %v", err)
		}
	})

	t.Run("funds effect cannot omit case-data authority", func(t *testing.T) {
		source := &providerAuthoritySourceStub{}
		input := fixture.input
		input.CaseDataEffect = false
		input.FundsDataEffect = true
		input.Source = source
		err := ValidateProviderAttemptAuthority(input)
		requireProviderAuthorityFailureCode(t, err, "turn_security_risk_policy_mismatch")
		if source.calls != 0 {
			t.Fatalf("funds source was consulted before case-data authority: calls=%d", source.calls)
		}
	})

	t.Run("funds effect revalidates exact source", func(t *testing.T) {
		source := &providerAuthoritySourceStub{}
		input := fixture.input
		input.CaseDataEffect = true
		input.FundsDataEffect = true
		input.Source = source
		if err := ValidateProviderAttemptAuthority(input); err != nil {
			t.Fatalf("validate funds provider authority: %v", err)
		}
		if source.calls != 1 || source.serverID != "analytix_funds" ||
			source.contextDigest != fixture.input.SecurityContext.ContextDigest {
			t.Fatalf("funds source validation was not exact: %#v", source)
		}
	})

	t.Run("funds effect fails closed without current source", func(t *testing.T) {
		input := fixture.input
		input.CaseDataEffect = true
		input.FundsDataEffect = true
		input.Source = nil
		requireProviderAuthorityFailureCode(t, ValidateProviderAttemptAuthority(input), "source_probe_unavailable")
	})
}

type providerAttemptAuthorityFixture struct {
	input ProviderAttemptAuthorityInput
}

func newProviderAttemptAuthorityFixture(t *testing.T) providerAttemptAuthorityFixture {
	t.Helper()
	workspace := t.TempDir()
	threadID := "thread-provider-authority"
	turnID := "turn-provider-authority"
	caseID := "case-provider-authority"
	issuedAt := time.Date(2026, 7, 27, 13, 0, 0, 0, time.UTC)
	observation, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: workspace,
		State:             domainsecurity.CaseBindingStateValid,
		CaseID:            caseID,
		BindingSHA256:     domainsecurity.SHA256Hex([]byte("provider-authority-marker")),
		CaseBindingHash:   domainsecurity.SHA256Hex([]byte("provider-authority-binding")),
	})
	if err != nil {
		t.Fatal(err)
	}
	installationID := domainsecurity.SHA256Hex([]byte("provider-authority-installation"))
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x2d}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	resolved, err := datasetsnapshotv2fixture.NewResolvedSnapshotV2(datasetsnapshotv2fixture.ResolvedInput{
		TenantID: domainidentity.LocalTenantID, UserID: domainidentity.LocalUserID,
		Observation: observation, Material: "provider-authority-snapshot",
		InstallationID: installationID, AcceptedAt: issuedAt,
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
		Sign: func(message []byte) ([]byte, error) {
			return ed25519.Sign(privateKey, message), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	riskPolicyDigest := domainsecurity.SHA256Hex([]byte("provider-authority-risk-policy"))
	publication, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest:   riskPolicyDigest,
		RiskClass:                domainsecurity.RiskClassCase,
		Disposition:              domainsecurity.PublicationDispositionCaseEvidenceGate,
		CaseBindingState:         domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: observation.ObservationDigest,
		BlockerCode:              domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	riskBinding, err := securitycontexttest.WitnessedRiskBinding(
		threadID, workspace, domainsecurity.RiskClassCase, riskPolicyDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		TenantID: domainidentity.LocalTenantID, UserID: domainidentity.LocalUserID,
		CaseID: caseID, CaseBindingHash: observation.CaseBindingHash,
		DatasetSnapshotID:  resolved.Record.DatasetSnapshotID,
		SourceManifestHash: resolved.Record.SourceManifestHash,
		ContextEpoch:       1, IssuedAt: issuedAt,
		PublicationPolicy: publication, RiskAuthorityBinding: riskBinding,
	})
	if err != nil {
		t.Fatal(err)
	}
	principal, err := domainidentity.NewPrincipalV1(
		installationID, domainidentity.LocalTenantID, domainidentity.LocalUserID,
	)
	if err != nil {
		t.Fatal(err)
	}
	return providerAttemptAuthorityFixture{input: ProviderAttemptAuthorityInput{
		OperationContext: context.Background(),
		Authority: structProviderWorkspaceAuthority(
			providerAuthorityIdentityStub{principal: principal},
			providerAuthorityObserverStub{workspace: workspace, observation: observation},
			providerAuthorityRiskStub{contextDigest: securityContext.ContextDigest},
			providerAuthoritySnapshotStub{resolved: resolved, observation: observation},
		),
		SecurityContext: securityContext,
		Workspace:       workspace,
		CaseLineage:     providerAuthorityCaseLineageStub(true),
	}}
}

func structProviderWorkspaceAuthority(
	identity providerAuthorityIdentityStub,
	observer providerAuthorityObserverStub,
	risk providerAuthorityRiskStub,
	snapshot providerAuthoritySnapshotStub,
) turnsecurityapp.WorkspaceSecurityAuthority {
	return turnsecurityapp.WorkspaceSecurityAuthority{
		Identity: identity, Observer: observer, RiskAuthority: risk, SnapshotAuthorityV2: snapshot,
	}
}

type providerAuthorityIdentityStub struct {
	principal domainidentity.PrincipalV1
}

func (stub providerAuthorityIdentityStub) ResolveCurrent(context.Context) (domainidentity.PrincipalV1, error) {
	return stub.principal, nil
}

func (stub providerAuthorityIdentityStub) ValidateCurrent(_ context.Context, principal domainidentity.PrincipalV1) error {
	if !domainidentity.SamePrincipalV1(stub.principal, principal) {
		return identityport.ErrMismatch
	}
	return nil
}

type providerAuthorityObserverStub struct {
	workspace   string
	observation domainsecurity.CaseBindingObservationV1
}

func (stub providerAuthorityObserverStub) Observe(workspace string) (domainsecurity.CaseBindingObservationV1, error) {
	if workspace != stub.workspace {
		return domainsecurity.CaseBindingObservationV1{}, errors.New("workspace mismatch")
	}
	return stub.observation, nil
}

type providerAuthorityRiskStub struct {
	contextDigest string
}

func (providerAuthorityRiskStub) ResolveOrRaise(
	context.Context,
	threadriskauthorityapp.ResolveOrRaiseInput,
) (threadriskauthorityapp.Head, error) {
	return threadriskauthorityapp.Head{}, errors.New("unexpected risk resolution")
}

func (stub providerAuthorityRiskStub) ValidateCurrent(_ context.Context, securityContext domainsecurity.TurnSecurityContext) error {
	if securityContext.ContextDigest != stub.contextDigest {
		return errors.New("risk context mismatch")
	}
	return nil
}

type providerAuthoritySnapshotStub struct {
	resolved    datasetsnapshotport.ResolvedSnapshotV2
	observation domainsecurity.CaseBindingObservationV1
}

func (stub providerAuthoritySnapshotStub) ResolveWitnessedV2(
	_ context.Context,
	input datasetsnapshotport.ResolveInputV2,
) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	record := stub.resolved.Record
	if input.TenantID != record.Binding.TenantID || input.UserID != record.Binding.UserID ||
		input.Observation != stub.observation ||
		input.ExpectedDatasetSnapshotID != record.DatasetSnapshotID {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrMismatch
	}
	return stub.resolved, nil
}

type providerAuthorityCaseLineageStub bool

func (stub providerAuthorityCaseLineageStub) IsCaseThread(string) bool {
	return bool(stub)
}

type providerAuthoritySourceStub struct {
	calls         int
	serverID      string
	contextDigest string
}

func (stub *providerAuthoritySourceStub) ValidateCurrentProbe(
	_ context.Context,
	serverID string,
	securityContext domainsecurity.TurnSecurityContext,
) error {
	stub.calls++
	stub.serverID = serverID
	stub.contextDigest = securityContext.ContextDigest
	return nil
}

func requireProviderAuthorityFailureCode(t *testing.T, err error, want string) {
	t.Helper()
	var failure TurnFailureError
	if !errors.As(err, &failure) || failure.Code != want {
		t.Fatalf("provider authority failure = %#v, want code %q", err, want)
	}
}
