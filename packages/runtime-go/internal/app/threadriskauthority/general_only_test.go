package threadriskauthority

import (
	"context"
	"errors"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type generalOnlyObserverStub struct {
	observation domainsecurity.CaseBindingObservationV1
	err         error
	calls       int
}

func (stub *generalOnlyObserverStub) Observe(string) (domainsecurity.CaseBindingObservationV1, error) {
	stub.calls++
	return stub.observation, stub.err
}

func TestGeneralOnlyAuthorityResolvesAndRevalidatesAcrossRestart(t *testing.T) {
	observation := generalOnlyObservation(t, "/workspace/general", domainsecurity.CaseBindingStateMissing)
	observer := &generalOnlyObserverStub{observation: observation}
	first, err := NewGeneralOnlyAuthority(observer)
	if err != nil {
		t.Fatal(err)
	}
	input := ResolveOrRaiseInput{
		ThreadID: "thread-general", WorkspaceRealPath: observation.WorkspaceRealPath,
		BindingObservation: observation, RequestedRisk: domainsecurity.RiskClassGeneral,
		Origin:        domainsecurity.RiskPolicyOriginGeneralWorkspace,
		SignalsDigest: domainsecurity.SHA256Hex([]byte("safe-general-signals")), IssuedAt: time.Now().UTC(),
	}
	head, err := first.ResolveOrRaise(context.Background(), input)
	if err != nil || !head.Found || head.HasIndex || head.Policy != (domainsecurity.ThreadRiskPolicyV1{}) ||
		head.RiskAuthorityBinding.State != domainsecurity.RiskAuthorityBindingStateHostGeneralOnly {
		t.Fatalf("general-only resolution mismatch: head=%#v err=%v", head, err)
	}
	publication, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: head.GeneralOnlyPolicy.PolicyDigest, RiskClass: domainsecurity.RiskClassGeneral,
		Disposition: domainsecurity.PublicationDispositionGeneralOutput, CaseBindingState: domainsecurity.CaseBindingStateMissing,
		BindingObservationDigest: observation.ObservationDigest, BlockerCode: domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: input.ThreadID, TurnID: "turn-general", WorkspaceRealPath: observation.WorkspaceRealPath,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: domainsecurity.UnboundCaseID, CaseBindingHash: domainsecurity.UnboundCaseBindingHash(observation.WorkspaceRealPath),
		DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID, SourceManifestHash: domainsecurity.EmptySourceManifestHash,
		ContextEpoch: 1, IssuedAt: time.Now().UTC(), PublicationPolicy: publication, RiskAuthorityBinding: head.RiskAuthorityBinding,
	})
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := NewGeneralOnlyAuthority(observer)
	if err != nil || restarted.ValidateCurrent(context.Background(), securityContext) != nil {
		t.Fatalf("restart-stable general-only authority rejected current context: %v", err)
	}
	if observer.calls < 2 {
		t.Fatalf("general-only authority did not perform fresh observations: calls=%d", observer.calls)
	}
	observer.observation = generalOnlyObservation(t, observation.WorkspaceRealPath, domainsecurity.CaseBindingStateValid)
	if err := restarted.ValidateCurrent(context.Background(), securityContext); !errors.Is(err, ErrGeneralOnlyDenied) {
		t.Fatalf("case binding appearing after issuance did not revoke general authority: %v", err)
	}
}

func TestGeneralOnlyAuthorityRejectsEveryElevationSignalAndIndeterminateBinding(t *testing.T) {
	missing := generalOnlyObservation(t, "/workspace/general", domainsecurity.CaseBindingStateMissing)
	authority, err := NewGeneralOnlyAuthority(&generalOnlyObserverStub{observation: missing})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []ResolveOrRaiseInput{
		{ThreadID: "thread-case", WorkspaceRealPath: missing.WorkspaceRealPath, BindingObservation: missing, RequestedRisk: domainsecurity.RiskClassCase, Origin: domainsecurity.RiskPolicyOriginLexicalGuard},
		{ThreadID: "thread-funds", WorkspaceRealPath: missing.WorkspaceRealPath, BindingObservation: missing, RequestedRisk: domainsecurity.RiskClassCase, Origin: domainsecurity.RiskPolicyOriginProtectedDataGuard},
		{ThreadID: "thread-resume", WorkspaceRealPath: missing.WorkspaceRealPath, BindingObservation: missing, RequestedRisk: domainsecurity.RiskClassCase, Origin: domainsecurity.RiskPolicyOriginSignedCaseLineage},
	} {
		input.SignalsDigest = domainsecurity.SHA256Hex([]byte(input.ThreadID))
		input.IssuedAt = time.Now().UTC()
		if _, err := authority.ResolveOrRaise(context.Background(), input); !errors.Is(err, ErrGeneralOnlyDenied) {
			t.Fatalf("general-only authority accepted risk elevation: input=%#v err=%v", input, err)
		}
	}
	for _, state := range []string{
		domainsecurity.CaseBindingStateValid, domainsecurity.CaseBindingStateInvalid,
		domainsecurity.CaseBindingStateUnreadable, domainsecurity.CaseBindingStateUnstable,
		domainsecurity.CaseBindingStateWorkspaceMissing, domainsecurity.CaseBindingStateNotApplicable,
	} {
		observation := generalOnlyObservation(t, missing.WorkspaceRealPath, state)
		input := ResolveOrRaiseInput{
			ThreadID: "thread-" + state, WorkspaceRealPath: missing.WorkspaceRealPath,
			BindingObservation: observation, RequestedRisk: domainsecurity.RiskClassGeneral,
			Origin:        domainsecurity.RiskPolicyOriginGeneralWorkspace,
			SignalsDigest: domainsecurity.SHA256Hex([]byte(state)), IssuedAt: time.Now().UTC(),
		}
		if _, err := authority.ResolveOrRaise(context.Background(), input); !errors.Is(err, ErrGeneralOnlyDenied) {
			t.Fatalf("general-only authority accepted non-missing binding state %q: %v", state, err)
		}
	}
}

func generalOnlyObservation(t *testing.T, workspace, state string) domainsecurity.CaseBindingObservationV1 {
	t.Helper()
	input := domainsecurity.CaseBindingObservationInputV1{WorkspaceRealPath: workspace, State: state}
	if state == domainsecurity.CaseBindingStateValid {
		input.CaseID = "case-general-only-test"
		input.BindingSHA256 = domainsecurity.SHA256Hex([]byte("case-binding-file"))
		input.CaseBindingHash = domainsecurity.SHA256Hex([]byte("case-binding-hash"))
	}
	observation, err := domainsecurity.NewCaseBindingObservationV1(input)
	if err != nil {
		t.Fatal(err)
	}
	return observation
}
