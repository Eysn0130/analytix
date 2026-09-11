package turnsecurity

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	threadriskauthorityapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestResolveRiskPublicationIsHostDerivedAndMonotonic(t *testing.T) {
	at := time.Date(2026, 7, 12, 16, 0, 0, 0, time.UTC)
	authority := &riskPolicyAuthorityStub{}
	missing := riskObservation(t, "/workspace", domainsecurity.CaseBindingStateMissing, "", "")
	general, err := ResolveRiskPublication(context.Background(), RiskPolicyResolutionInput{
		Authority: authority, ThreadID: "thread-risk", WorkspaceRealPath: "/workspace", Binding: missing, IssuedAt: at,
	})
	if err != nil || general.Publication.Disposition != domainsecurity.PublicationDispositionGeneralOutput ||
		general.ThreadPolicy.RiskClass != domainsecurity.RiskClassGeneral {
		t.Fatalf("general host policy failed: result=%#v err=%v", general, err)
	}
	boundary, err := ResolveRiskPublication(context.Background(), RiskPolicyResolutionInput{
		Authority: authority, ThreadID: "thread-risk", WorkspaceRealPath: "/workspace", Binding: missing,
		RiskIntent: domainsecurity.RiskClassCase, IssuedAt: at.Add(time.Second),
	})
	if err != nil || boundary.Publication.Disposition != domainsecurity.PublicationDispositionCaseBoundaryOnly ||
		boundary.Publication.BlockerCode != domainsecurity.PublicationBlockerCaseBindingMissing ||
		boundary.ThreadPolicy.RiskClass != domainsecurity.RiskClassCase {
		t.Fatalf("case boundary policy failed: result=%#v err=%v", boundary, err)
	}
	noDowngrade, err := ResolveRiskPublication(context.Background(), RiskPolicyResolutionInput{
		Authority: authority, ThreadID: "thread-risk", WorkspaceRealPath: "/workspace", Binding: missing, IssuedAt: at.Add(2 * time.Second),
	})
	if err != nil || noDowngrade.ThreadPolicy.RiskClass != domainsecurity.RiskClassCase ||
		noDowngrade.Publication.Disposition != domainsecurity.PublicationDispositionCaseBoundaryOnly {
		t.Fatalf("case risk downgraded: result=%#v err=%v", noDowngrade, err)
	}
	if _, err := ResolveRiskPublication(context.Background(), RiskPolicyResolutionInput{
		Authority: authority, ThreadID: "thread-risk", WorkspaceRealPath: "/workspace", Binding: missing,
		RiskIntent: domainsecurity.RiskClassGeneral, IssuedAt: at.Add(3 * time.Second),
	}); err == nil {
		t.Fatal("caller supplied a risk downgrade intent")
	}
}

func TestContextChangingInputRaisesCaseRiskWithoutCallerIntent(t *testing.T) {
	at := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	authority := &riskPolicyAuthorityStub{}
	missing := riskObservation(t, "/workspace", domainsecurity.CaseBindingStateMissing, "", "")
	result, err := ResolveRiskPublication(context.Background(), RiskPolicyResolutionInput{
		Authority: authority, ThreadID: "thread-context-input", WorkspaceRealPath: "/workspace", Binding: missing,
		ContextChangingInput: true, IssuedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RequestedRisk != domainsecurity.RiskClassCase || result.Origin != domainsecurity.RiskPolicyOriginHostInputContextChange ||
		result.Publication.Disposition != domainsecurity.PublicationDispositionCaseBoundaryOnly ||
		result.Publication.BlockerCode != domainsecurity.PublicationBlockerCaseBindingMissing {
		t.Fatalf("host context-changing input did not raise a deterministic case boundary: %#v", result)
	}
}

func TestProtectedCaseDataRaisesCaseRiskWithoutLexicalIntent(t *testing.T) {
	at := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	authority := &riskPolicyAuthorityStub{}
	missing := riskObservation(t, "/workspace", domainsecurity.CaseBindingStateMissing, "", "")
	result, err := ResolveRiskPublication(context.Background(), RiskPolicyResolutionInput{
		Authority: authority, ThreadID: "thread-protected-data", WorkspaceRealPath: "/workspace", Binding: missing,
		ProtectedCaseData: true, IssuedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RequestedRisk != domainsecurity.RiskClassCase || result.Origin != domainsecurity.RiskPolicyOriginProtectedDataGuard ||
		result.Publication.Disposition != domainsecurity.PublicationDispositionCaseBoundaryOnly ||
		result.Publication.BlockerCode != domainsecurity.PublicationBlockerCaseBindingMissing {
		t.Fatalf("protected data did not raise a deterministic case boundary: %#v", result)
	}
}

func TestResolveRiskPublicationClassifiesBindingStatesAndChanges(t *testing.T) {
	at := time.Date(2026, 7, 12, 17, 0, 0, 0, time.UTC)
	for state, blocker := range map[string]string{
		domainsecurity.CaseBindingStateInvalid:          domainsecurity.PublicationBlockerCaseBindingInvalid,
		domainsecurity.CaseBindingStateUnreadable:       domainsecurity.PublicationBlockerCaseBindingUnreadable,
		domainsecurity.CaseBindingStateUnstable:         domainsecurity.PublicationBlockerCaseBindingUnstable,
		domainsecurity.CaseBindingStateWorkspaceMissing: domainsecurity.PublicationBlockerCaseWorkspaceUnavailable,
	} {
		t.Run(state, func(t *testing.T) {
			authority := &riskPolicyAuthorityStub{}
			observation := riskObservation(t, "/workspace", state, "", "")
			result, err := ResolveRiskPublication(context.Background(), RiskPolicyResolutionInput{
				Authority: authority, ThreadID: "thread-" + state, WorkspaceRealPath: "/workspace", Binding: observation,
				RiskIntent: domainsecurity.RiskClassCase, IssuedAt: at,
			})
			if err != nil || result.Publication.Disposition != domainsecurity.PublicationDispositionCaseBoundaryOnly ||
				result.Publication.BlockerCode != blocker {
				t.Fatalf("binding state policy mismatch: result=%#v err=%v", result, err)
			}
		})
	}
	authority := &riskPolicyAuthorityStub{}
	valid := riskObservation(t, "/workspace", domainsecurity.CaseBindingStateValid, "case-new", domainsecurity.SHA256Hex([]byte("binding-new")))
	previous := riskContext(t, "thread-changed", "case-old", domainsecurity.SHA256Hex([]byte("binding-old")))
	changed, err := ResolveRiskPublication(context.Background(), RiskPolicyResolutionInput{
		Authority: authority, ThreadID: previous.ThreadID, WorkspaceRealPath: "/workspace", Binding: valid,
		PreviousContext: &previous, IssuedAt: at,
	})
	if err != nil || changed.Publication.CaseBindingState != domainsecurity.CaseBindingStateChangedUnaccepted ||
		changed.Publication.BlockerCode != domainsecurity.PublicationBlockerCaseBindingChanged {
		t.Fatalf("changed binding was accepted without rebind authority: result=%#v err=%v", changed, err)
	}
}

func TestWorkspaceUnavailableCannotMintGeneralRiskWithoutCallerIntent(t *testing.T) {
	at := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	for _, state := range []string{
		domainsecurity.CaseBindingStateWorkspaceMissing,
		domainsecurity.CaseBindingStateNotApplicable,
	} {
		t.Run(state, func(t *testing.T) {
			authority := &riskPolicyAuthorityStub{}
			observation := riskObservation(t, "/workspace", state, "", "")
			result, err := ResolveRiskPublication(context.Background(), RiskPolicyResolutionInput{
				Authority: authority, ThreadID: "thread-no-general-" + state,
				WorkspaceRealPath: "/workspace", Binding: observation, IssuedAt: at,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.ThreadPolicy.RiskClass != domainsecurity.RiskClassCase ||
				result.Publication.Disposition != domainsecurity.PublicationDispositionCaseBoundaryOnly {
				t.Fatalf("non-missing binding state minted general authority: %#v", result)
			}
			if state == domainsecurity.CaseBindingStateWorkspaceMissing &&
				result.Publication.BlockerCode != domainsecurity.PublicationBlockerCaseWorkspaceUnavailable {
				t.Fatalf("workspace-unavailable blocker = %q", result.Publication.BlockerCode)
			}
			if state == domainsecurity.CaseBindingStateNotApplicable &&
				result.Publication.BlockerCode != domainsecurity.PublicationBlockerCasePolicyCorrupt {
				t.Fatalf("not-applicable blocker = %q", result.Publication.BlockerCode)
			}
		})
	}
}

func TestResolveRiskPublicationRejectsCallerPolicyMaterial(t *testing.T) {
	authority := &riskPolicyAuthorityStub{err: errors.New("authority failed")}
	observation := riskObservation(t, "/workspace", domainsecurity.CaseBindingStateMissing, "", "")
	if _, err := ResolveRiskPublication(context.Background(), RiskPolicyResolutionInput{
		Authority: authority, ThreadID: "thread", WorkspaceRealPath: "/workspace", Binding: observation,
		RiskIntent: "case_evidence_gate", IssuedAt: time.Now().UTC(),
	}); err == nil {
		t.Fatal("caller supplied a publication disposition as risk intent")
	}
}

type riskPolicyAuthorityStub struct {
	current    domainsecurity.ThreadRiskPolicyV1
	head       threadriskauthorityapp.Head
	err        error
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
}

func (stub *riskPolicyAuthorityStub) ResolveOrRaise(_ context.Context, input threadriskauthorityapp.ResolveOrRaiseInput) (threadriskauthorityapp.Head, error) {
	if stub.err != nil {
		return threadriskauthorityapp.Head{}, stub.err
	}
	if stub.current.RiskClass == domainsecurity.RiskClassCase {
		return stub.head, nil
	}
	if stub.privateKey == nil {
		stub.privateKey = ed25519.NewKeyFromSeed(bytes.Repeat([]byte{91}, ed25519.SeedSize))
		stub.publicKey = stub.privateKey.Public().(ed25519.PublicKey)
	}
	predecessor := stub.current.PolicyDigest
	policy, err := domainsecurity.NewThreadRiskPolicyV1(domainsecurity.ThreadRiskPolicyInputV1{
		ThreadID: input.ThreadID, WorkspaceRealPath: input.WorkspaceRealPath, RiskClass: input.RequestedRisk,
		Origin: input.Origin, SignalsDigest: input.SignalsDigest, PredecessorPolicyDigest: predecessor,
		IssuedAt: input.IssuedAt, AuthorityKeyID: domainsecurity.SHA256Hex(stub.publicKey), AuthorityPublicKey: stub.publicKey,
	}, func(message []byte) ([]byte, error) {
		return ed25519.Sign(stub.privateKey, message), nil
	})
	if err != nil {
		return threadriskauthorityapp.Head{}, err
	}
	stub.current = policy
	contracts, err := securitycontexttest.WitnessedRiskAuthorityContracts(
		policy.ThreadID, policy.WorkspaceRealPath, policy.RiskClass, policy.PolicyDigest,
	)
	if err != nil {
		return threadriskauthorityapp.Head{}, err
	}
	stub.head = threadriskauthorityapp.Head{
		HasIndex: true, Index: contracts.Index, Request: contracts.Request, Observation: contracts.Observation,
		RiskAuthorityBinding: contracts.Binding, Policy: policy, Found: true,
	}
	return stub.head, nil
}

func (stub *riskPolicyAuthorityStub) ValidateCurrent(_ context.Context, securityContext domainsecurity.TurnSecurityContext) error {
	if stub.current.PolicyDigest == "" || securityContext.PublicationPolicy.ThreadRiskPolicyDigest != stub.current.PolicyDigest ||
		securityContext.PublicationPolicy.RiskClass != stub.current.RiskClass || securityContext.ThreadID != stub.current.ThreadID ||
		securityContext.WorkspaceRealPath != stub.current.WorkspaceRealPath {
		return errors.New("test risk authority head mismatch")
	}
	return nil
}

func riskObservation(t *testing.T, workspace, state, caseID, bindingHash string) domainsecurity.CaseBindingObservationV1 {
	t.Helper()
	input := domainsecurity.CaseBindingObservationInputV1{WorkspaceRealPath: workspace, State: state}
	if state == domainsecurity.CaseBindingStateValid {
		input.CaseID = caseID
		input.BindingSHA256 = domainsecurity.SHA256Hex([]byte("binding-bytes:" + caseID))
		input.CaseBindingHash = bindingHash
	}
	observation, err := domainsecurity.NewCaseBindingObservationV1(input)
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func riskContext(t *testing.T, threadID, caseID, bindingHash string) domainsecurity.TurnSecurityContext {
	t.Helper()
	publication, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: domainsecurity.SHA256Hex([]byte("old-policy")), RiskClass: domainsecurity.RiskClassCase,
		Disposition: domainsecurity.PublicationDispositionCaseEvidenceGate, CaseBindingState: domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("old-observation")), BlockerCode: domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: "turn-old", WorkspaceRealPath: "/workspace", TenantID: domainsecurity.LocalTenantID,
		UserID: domainsecurity.LocalUserID, CaseID: caseID, CaseBindingHash: bindingHash,
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("snapshot-old"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-old")), ContextEpoch: 1,
		IssuedAt: time.Date(2026, 7, 12, 1, 0, 0, 0, time.UTC), PublicationPolicy: publication,
		RiskAuthorityBinding: mustWitnessedRiskBinding(t, threadID, "/workspace", domainsecurity.RiskClassCase, publication.ThreadRiskPolicyDigest),
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func mustWitnessedRiskBinding(t *testing.T, threadID, workspace, riskClass, policyDigest string) domainsecurity.RiskAuthorityBindingV1 {
	t.Helper()
	binding, err := securitycontexttest.WitnessedRiskBinding(threadID, workspace, riskClass, policyDigest)
	if err != nil {
		t.Fatal(err)
	}
	return binding
}
