package thread

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
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type mutationWorkspaceObserver struct {
	observations map[string]domainsecurity.CaseBindingObservationV1
}

func (stub mutationWorkspaceObserver) Observe(workspace string) (domainsecurity.CaseBindingObservationV1, error) {
	observation, found := stub.observations[workspace]
	if !found {
		return domainsecurity.CaseBindingObservationV1{}, errors.New("workspace observation missing")
	}
	return observation, nil
}

func (stub mutationWorkspaceObserver) ReadOptional(workspace string) (domainsecurity.CaseBinding, bool, error) {
	observation, err := stub.Observe(workspace)
	if err != nil || observation.State != domainsecurity.CaseBindingStateValid {
		return domainsecurity.CaseBinding{}, false, err
	}
	return domainsecurity.CaseBinding{
		CaseID: observation.CaseID, WorkspaceRealPath: observation.WorkspaceRealPath,
		BindingSHA256: observation.BindingSHA256, CaseBindingHash: observation.CaseBindingHash,
	}, true, nil
}

func (stub mutationWorkspaceObserver) WorkspaceRealPath(workspace string) (string, error) {
	observation, err := stub.Observe(workspace)
	return observation.WorkspaceRealPath, err
}

type mutationRiskAuthority struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	current    map[string]mutationRiskHead
	byDigest   map[string]mutationRiskHead
}

type mutationRiskHead struct {
	policy    domainsecurity.ThreadRiskPolicyV1
	contracts securitycontexttest.RiskAuthorityContracts
}

type mutationOrdinaryCurrentRiskAuthority struct{}

func (mutationOrdinaryCurrentRiskAuthority) ResolveOrRaise(context.Context, threadriskauthorityapp.ResolveOrRaiseInput) (threadriskauthorityapp.Head, error) {
	return threadriskauthorityapp.Head{}, errors.New("risk resolution is not used for an existing mutation context")
}

func (mutationOrdinaryCurrentRiskAuthority) ValidateCurrent(context.Context, domainsecurity.TurnSecurityContext) error {
	return nil
}

func newMutationRiskAuthority() *mutationRiskAuthority {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{73}, ed25519.SeedSize))
	return &mutationRiskAuthority{
		privateKey: privateKey, publicKey: privateKey.Public().(ed25519.PublicKey),
		current: map[string]mutationRiskHead{}, byDigest: map[string]mutationRiskHead{},
	}
}

func (authority *mutationRiskAuthority) ResolveOrRaise(_ context.Context, input threadriskauthorityapp.ResolveOrRaiseInput) (threadriskauthorityapp.Head, error) {
	current, found := authority.current[input.ThreadID]
	if found && current.policy.WorkspaceRealPath == input.WorkspaceRealPath &&
		(current.policy.RiskClass == input.RequestedRisk || current.policy.RiskClass == domainsecurity.RiskClassCase) {
		return mutationThreadRiskAuthorityHead(current), nil
	}
	if found && current.policy.RiskClass == domainsecurity.RiskClassCase && current.policy.WorkspaceRealPath != input.WorkspaceRealPath {
		return threadriskauthorityapp.Head{}, errors.New("case workspace requires explicit signed rebind")
	}
	predecessor := ""
	if found {
		predecessor = current.policy.PolicyDigest
	}
	policy, err := domainsecurity.NewThreadRiskPolicyV1(domainsecurity.ThreadRiskPolicyInputV1{
		ThreadID: input.ThreadID, WorkspaceRealPath: input.WorkspaceRealPath, RiskClass: input.RequestedRisk,
		Origin: input.Origin, SignalsDigest: input.SignalsDigest, PredecessorPolicyDigest: predecessor,
		IssuedAt: input.IssuedAt, AuthorityKeyID: domainsecurity.SHA256Hex(authority.publicKey), AuthorityPublicKey: authority.publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(authority.privateKey, message), nil })
	if err != nil {
		return threadriskauthorityapp.Head{}, err
	}
	contracts, err := securitycontexttest.WitnessedRiskAuthorityContracts(
		policy.ThreadID, policy.WorkspaceRealPath, policy.RiskClass, policy.PolicyDigest,
	)
	if err != nil {
		return threadriskauthorityapp.Head{}, err
	}
	head := mutationRiskHead{policy: policy, contracts: contracts}
	authority.current[input.ThreadID] = head
	authority.byDigest[policy.PolicyDigest] = head
	return mutationThreadRiskAuthorityHead(head), nil
}

func (authority *mutationRiskAuthority) ValidateCurrent(_ context.Context, securityContext domainsecurity.TurnSecurityContext) error {
	head, found := authority.byDigest[securityContext.PublicationPolicy.ThreadRiskPolicyDigest]
	current, currentFound := authority.current[securityContext.ThreadID]
	if !found || !currentFound || current.policy.PolicyDigest != head.policy.PolicyDigest ||
		domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) != nil ||
		head.policy.ThreadID != securityContext.ThreadID || head.policy.WorkspaceRealPath != securityContext.WorkspaceRealPath ||
		domainsecurity.ValidateThreadRiskPolicyV1(head.policy) != nil ||
		domainsecurity.ValidateWitnessedRiskAuthorityBindingV1(
			securityContext.RiskAuthorityBinding,
			head.contracts.Index,
			head.contracts.Request,
			head.contracts.Observation,
		) != nil {
		return errors.New("risk policy context mismatch")
	}
	return domainsecurity.ValidateTurnPublicationPolicyForThreadRiskPolicyV1(securityContext.PublicationPolicy, head.policy)
}

func mutationThreadRiskAuthorityHead(head mutationRiskHead) threadriskauthorityapp.Head {
	return threadriskauthorityapp.Head{
		HasIndex: true,
		Index:    head.contracts.Index, Request: head.contracts.Request, Observation: head.contracts.Observation,
		RiskAuthorityBinding: head.contracts.Binding, Policy: head.policy, Found: true,
	}
}

func TestGeneralWorkspaceRebindUsesSignedV2Successor(t *testing.T) {
	at := time.Date(2026, 7, 12, 20, 0, 0, 0, time.UTC)
	reader := mutationWorkspaceObserver{observations: map[string]domainsecurity.CaseBindingObservationV1{
		"/workspace/a": mutationObservation(t, "/workspace/a", domainsecurity.CaseBindingStateMissing, "", ""),
		"/workspace/b": mutationObservation(t, "/workspace/b", domainsecurity.CaseBindingStateMissing, "", ""),
	}}
	riskAuthority := newMutationRiskAuthority()
	authority := turnsecurityapp.WorkspaceSecurityAuthority{Identity: testIdentityAuthority(), Observer: reader, RiskAuthority: riskAuthority}
	scopes, err := FreezeWorkspaceRebindScopes(map[string]any{
		"id": "thread-rebind-v2", "workspace": "/workspace/a",
	}, "thread-rebind-v2", "/workspace/b", reader, testIdentityPrincipal(), at, authority)
	if err != nil {
		t.Fatal(err)
	}
	if !domainsecurity.TurnSecurityContextIsGeneral(scopes.Previous) || !domainsecurity.TurnSecurityContextIsGeneral(scopes.Target) ||
		scopes.Previous.Version != domainsecurity.TurnSecurityContextVersionV2 || scopes.Target.Version != domainsecurity.TurnSecurityContextVersionV2 ||
		scopes.Previous.PublicationPolicy.ThreadRiskPolicyDigest == scopes.Target.PublicationPolicy.ThreadRiskPolicyDigest {
		t.Fatalf("workspace rebind did not use a signed V2 successor: %#v", scopes)
	}
	prepared, err := PrepareWorkspaceMutation(map[string]any{"id": "thread-rebind-v2", "workspace": "/workspace/a"}, scopes)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.SecurityContext.ContextEpoch <= scopes.Previous.ContextEpoch || !domainsecurity.TurnSecurityContextIsGeneral(prepared.SecurityContext) ||
		prepared.SecurityContext.PublicationPolicy != scopes.Target.PublicationPolicy {
		t.Fatalf("workspace mutation lost V2 policy authority: %#v", prepared)
	}
}

func TestCaseWorkspaceMoveAndLegacyV1FailClosed(t *testing.T) {
	at := time.Date(2026, 7, 12, 21, 0, 0, 0, time.UTC)
	bindingHash := domainsecurity.SHA256Hex([]byte("case-binding"))
	reader := mutationWorkspaceObserver{observations: map[string]domainsecurity.CaseBindingObservationV1{
		"/workspace/case":    mutationObservation(t, "/workspace/case", domainsecurity.CaseBindingStateValid, "case-1234", bindingHash),
		"/workspace/general": mutationObservation(t, "/workspace/general", domainsecurity.CaseBindingStateMissing, "", ""),
	}}
	authority := turnsecurityapp.WorkspaceSecurityAuthority{Identity: testIdentityAuthority(), Observer: reader, RiskAuthority: newMutationRiskAuthority()}
	if _, err := FreezeWorkspaceRebindScopes(map[string]any{
		"id": "thread-case-rebind", "workspace": "/workspace/case",
	}, "thread-case-rebind", "/workspace/general", reader, testIdentityPrincipal(), at, authority); err == nil {
		t.Fatalf("case workspace move did not fail closed: %v", err)
	}
	legacy := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-v1-mutation", TurnID: "turn-v1", WorkspaceRealPath: "/workspace/general", IssuedAt: at,
	})
	if _, err := FreezeThreadMutationScope(map[string]any{
		"id": legacy.ThreadID, "workspace": "/workspace/general", "securityState": turnsecurityapp.PublicRecord(legacy),
	}, legacy.ThreadID, reader, testIdentityPrincipal(), at, authority); err == nil {
		t.Fatal("legacy V1 context authorized a thread mutation")
	}
}

func TestCaseMetadataMutationDoesNotRequireCurrentDatasetSnapshot(t *testing.T) {
	at := time.Date(2026, 7, 12, 21, 30, 0, 0, time.UTC)
	workspace := "/workspace/case-metadata"
	caseID := "case-metadata"
	bindingHash := domainsecurity.SHA256Hex([]byte("case-metadata-binding"))
	observation := mutationObservation(t, workspace, domainsecurity.CaseBindingStateValid, caseID, bindingHash)
	reader := mutationWorkspaceObserver{observations: map[string]domainsecurity.CaseBindingObservationV1{workspace: observation}}
	policyDigest := domainsecurity.SHA256Hex([]byte("case-metadata-risk-policy"))
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: policyDigest, RiskClass: domainsecurity.RiskClassCase,
		Disposition: domainsecurity.PublicationDispositionCaseEvidenceGate, CaseBindingState: domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: observation.ObservationDigest, BlockerCode: domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	riskBinding, err := securitycontexttest.WitnessedRiskBinding("thread-case-metadata", workspace, domainsecurity.RiskClassCase, policyDigest)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-case-metadata", TurnID: "turn-case-metadata", WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: caseID, CaseBindingHash: bindingHash,
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("case-metadata"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("case-metadata-manifest")), ContextEpoch: 4, IssuedAt: at,
		PublicationPolicy: policy, RiskAuthorityBinding: riskBinding,
	})
	if err != nil {
		t.Fatal(err)
	}
	authority := turnsecurityapp.WorkspaceSecurityAuthority{
		Identity: testIdentityAuthority(), Observer: reader, RiskAuthority: mutationOrdinaryCurrentRiskAuthority{},
		// Dataset authorities are deliberately absent: metadata mutation is an
		// ordinary effect and must not reacquire protected snapshot capability.
	}
	current, err := FreezeThreadMutationScope(map[string]any{
		"id": securityContext.ThreadID, "workspace": workspace,
		"securityState": turnsecurityapp.PublicRecord(securityContext),
	}, securityContext.ThreadID, reader, testIdentityPrincipal(), at.Add(time.Second), authority)
	if err != nil {
		t.Fatal(err)
	}
	if current.ContextDigest != securityContext.ContextDigest {
		t.Fatalf("metadata mutation changed its existing security context: %#v", current)
	}
}

func TestFreezeThreadMutationScopeRejectsForeignHostPrincipal(t *testing.T) {
	foreign, err := domainidentity.NewPrincipalV1(
		domainsecurity.SHA256Hex([]byte("foreign-thread-mutation-installation")),
		domainidentity.LocalTenantID,
		domainidentity.LocalUserID,
	)
	if err != nil {
		t.Fatal(err)
	}
	reader := mutationWorkspaceObserver{}
	authority := turnsecurityapp.WorkspaceSecurityAuthority{
		Identity: testIdentityAuthority(), Observer: reader, RiskAuthority: newMutationRiskAuthority(),
	}
	if _, err := FreezeThreadMutationScope(
		map[string]any{"id": "thread-foreign-principal", "workspace": "/workspace/foreign-principal"},
		"thread-foreign-principal", reader, foreign, time.Now().UTC(), authority,
	); err == nil || err.Error() != "turn_security_identity_mismatch" {
		t.Fatalf("foreign principal error = %v, want turn_security_identity_mismatch", err)
	}
}

func TestBoundaryDerivedMutationTargetsNeverBecomeGeneral(t *testing.T) {
	at := time.Date(2026, 7, 12, 22, 0, 0, 0, time.UTC)
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: domainsecurity.SHA256Hex([]byte("risk-case-boundary")), RiskClass: domainsecurity.RiskClassCase,
		Disposition: domainsecurity.PublicationDispositionCaseBoundaryOnly, CaseBindingState: domainsecurity.CaseBindingStateMissing,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("observation-missing")),
		BlockerCode:              domainsecurity.PublicationBlockerCaseBindingMissing,
	})
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-boundary", TurnID: "turn-boundary", WorkspaceRealPath: "/workspace/boundary",
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, CaseID: domainsecurity.UnboundCaseID,
		CaseBindingHash: domainsecurity.UnboundCaseBindingHash("/workspace/boundary"), DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID,
		SourceManifestHash: domainsecurity.EmptySourceManifestHash, ContextEpoch: 3, IssuedAt: at, PublicationPolicy: policy,
		RiskAuthorityBinding: domainsecurity.NewQuarantinedRiskAuthorityBindingV1(),
	})
	if err != nil {
		t.Fatal(err)
	}
	target := MutationInvalidationTarget(boundary, "delete", at.Add(time.Second))
	if !domainsecurity.TurnSecurityContextIsBoundaryOnly(target) || target.PublicationPolicy != boundary.PublicationPolicy {
		t.Fatalf("boundary invalidation target downgraded risk: %#v", target)
	}
	if _, err := PrepareRewindMutation(map[string]any{
		"id": boundary.ThreadID, "workspace": boundary.WorkspaceRealPath, "turns": []any{},
	}, boundary.ThreadID, "turn-missing", boundary, at); err == nil {
		t.Fatal("boundary-only context authorized rewind")
	}
}

func mutationObservation(t *testing.T, workspace, state, caseID, bindingHash string) domainsecurity.CaseBindingObservationV1 {
	t.Helper()
	input := domainsecurity.CaseBindingObservationInputV1{WorkspaceRealPath: workspace, State: state}
	if state == domainsecurity.CaseBindingStateValid {
		input.CaseID = caseID
		input.BindingSHA256 = domainsecurity.SHA256Hex([]byte("binding-body:" + caseID))
		input.CaseBindingHash = bindingHash
	}
	observation, err := domainsecurity.NewCaseBindingObservationV1(input)
	if err != nil {
		t.Fatal(err)
	}
	return observation
}
