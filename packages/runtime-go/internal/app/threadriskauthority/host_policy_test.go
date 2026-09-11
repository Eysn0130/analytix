package threadriskauthority

import (
	"context"
	"errors"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestHostPolicyAuthorityIssuesRestartReadableCaseRiskWithoutWitness(t *testing.T) {
	workspace := "/workspace/host-policy-case"
	observation := generalOnlyObservation(t, workspace, domainsecurity.CaseBindingStateValid)
	observer := &generalOnlyObserverStub{observation: observation}
	installationAuthority := newTestInstallationAuthority(0xa7)
	policies := newMemoryPolicyStore()
	authority, err := NewHostPolicyAuthority(observer, installationAuthority, policies)
	if err != nil {
		t.Fatal(err)
	}
	issuedAt := time.Now().UTC().Round(0)
	input := ResolveOrRaiseInput{
		ThreadID: "thread-host-policy", WorkspaceRealPath: workspace,
		BindingObservation: observation, RequestedRisk: domainsecurity.RiskClassCase,
		Origin:        domainsecurity.RiskPolicyOriginValidCaseBinding,
		SignalsDigest: domainsecurity.SHA256Hex([]byte("host-policy-case-signals")),
		IssuedAt:      issuedAt,
	}
	head, err := authority.ResolveOrRaise(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if head.HasIndex || !head.Found || head.Index.IndexDigest != "" ||
		head.Request != (domainsecurity.MonotonicHeadObserveRequestV1{}) ||
		head.Observation != (domainsecurity.MonotonicHeadObservationV1{}) ||
		head.RiskAuthorityBinding.State != domainsecurity.RiskAuthorityBindingStateHostPolicy ||
		head.RiskAuthorityBinding.ThreadPolicyDigest != head.Policy.PolicyDigest ||
		domainsecurity.ValidateHostPolicyRiskAuthorityBindingV1(
			head.RiskAuthorityBinding,
			head.Policy,
			observation,
		) != nil {
		t.Fatalf("host-policy head mismatch: %#v", head)
	}
	publication, err := domainsecurity.NewTurnPublicationPolicyV1(
		domainsecurity.TurnPublicationPolicyInputV1{
			ThreadRiskPolicyDigest:   head.Policy.PolicyDigest,
			RiskClass:                domainsecurity.RiskClassCase,
			Disposition:              domainsecurity.PublicationDispositionCaseEvidenceGate,
			CaseBindingState:         domainsecurity.CaseBindingStateValid,
			BindingObservationDigest: observation.ObservationDigest,
			BlockerCode:              domainsecurity.PublicationBlockerNone,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(
		domainsecurity.TurnSecurityContextInput{
			ThreadID: input.ThreadID, TurnID: "turn-host-policy",
			WorkspaceRealPath: workspace,
			TenantID:          domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
			CaseID: observation.CaseID, CaseBindingHash: observation.CaseBindingHash,
			DatasetSnapshotID: domainsecurity.DatasetSnapshotIDPrefixV2 +
				domainsecurity.SHA256Hex([]byte("host-policy-snapshot")),
			SourceManifestHash: domainsecurity.SHA256Hex([]byte("host-policy-source")),
			ContextEpoch:       1, IssuedAt: issuedAt,
			PublicationPolicy: publication, RiskAuthorityBinding: head.RiskAuthorityBinding,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil {
		t.Fatal("host-policy TSCV2 did not retain case-fact structural authority")
	}
	restarted, err := NewHostPolicyAuthority(observer, installationAuthority, policies)
	if err != nil || restarted.ValidateCurrent(context.Background(), securityContext) != nil {
		t.Fatalf("restart could not revalidate exact host policy: %v", err)
	}
	policies.delete(head.Policy.PolicyDigest)
	if err := restarted.ValidateCurrent(context.Background(), securityContext); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("missing exact policy CAS was accepted: %v", err)
	}
}

func TestHostPolicyAuthorityRevokesOnlyCaseEffectWhenBindingChanges(t *testing.T) {
	workspace := "/workspace/host-policy-revocation"
	observation := generalOnlyObservation(t, workspace, domainsecurity.CaseBindingStateValid)
	observer := &generalOnlyObserverStub{observation: observation}
	authority, err := NewHostPolicyAuthority(
		observer,
		newTestInstallationAuthority(0xb8),
		newMemoryPolicyStore(),
	)
	if err != nil {
		t.Fatal(err)
	}
	head, err := authority.ResolveOrRaise(context.Background(), ResolveOrRaiseInput{
		ThreadID: "thread-revoked-case", WorkspaceRealPath: workspace,
		BindingObservation: observation, RequestedRisk: domainsecurity.RiskClassCase,
		Origin:        domainsecurity.RiskPolicyOriginValidCaseBinding,
		SignalsDigest: domainsecurity.SHA256Hex([]byte("case-before-revocation")),
		IssuedAt:      time.Now().UTC().Round(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	observer.observation = generalOnlyObservation(t, workspace, domainsecurity.CaseBindingStateMissing)
	publication, err := domainsecurity.NewTurnPublicationPolicyV1(
		domainsecurity.TurnPublicationPolicyInputV1{
			ThreadRiskPolicyDigest:   head.Policy.PolicyDigest,
			RiskClass:                domainsecurity.RiskClassCase,
			Disposition:              domainsecurity.PublicationDispositionCaseEvidenceGate,
			CaseBindingState:         domainsecurity.CaseBindingStateValid,
			BindingObservationDigest: observation.ObservationDigest,
			BlockerCode:              domainsecurity.PublicationBlockerNone,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(
		domainsecurity.TurnSecurityContextInput{
			ThreadID: "thread-revoked-case", TurnID: "turn-revoked-case",
			WorkspaceRealPath: workspace,
			TenantID:          domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
			CaseID: observation.CaseID, CaseBindingHash: observation.CaseBindingHash,
			DatasetSnapshotID: domainsecurity.DatasetSnapshotIDPrefixV2 +
				domainsecurity.SHA256Hex([]byte("revoked-snapshot")),
			SourceManifestHash: domainsecurity.SHA256Hex([]byte("revoked-source")),
			ContextEpoch:       1, IssuedAt: time.Now().UTC().Round(0),
			PublicationPolicy: publication, RiskAuthorityBinding: head.RiskAuthorityBinding,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := authority.ValidateCurrent(context.Background(), securityContext); !errors.Is(err, ErrHostPolicyDenied) {
		t.Fatalf("changed case binding retained protected authority: %v", err)
	}
	generalInput := ResolveOrRaiseInput{
		ThreadID: "thread-ordinary-after-revocation", WorkspaceRealPath: workspace,
		BindingObservation: observer.observation,
		RequestedRisk:      domainsecurity.RiskClassGeneral,
		Origin:             domainsecurity.RiskPolicyOriginGeneralWorkspace,
		SignalsDigest:      domainsecurity.SHA256Hex([]byte("ordinary-after-revocation")),
		IssuedAt:           time.Now().UTC().Round(0),
	}
	general, err := authority.ResolveOrRaise(context.Background(), generalInput)
	if err != nil || general.RiskAuthorityBinding.State !=
		domainsecurity.RiskAuthorityBindingStateHostGeneralOnly {
		t.Fatalf("ordinary authority was disabled by case revocation: head=%#v err=%v", general, err)
	}
}
