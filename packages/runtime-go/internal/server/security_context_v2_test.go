package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"sync"
	"testing"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	threadriskauthorityapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func newServerGeneralContextV2(t *testing.T, input domainsecurity.TurnSecurityContextInput) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(input)
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func newServerCaseContextV2(t *testing.T, input domainsecurity.TurnSecurityContextInput) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := securitycontexttest.CaseExecutionContextV2(input)
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

// serverTestRiskAuthority is a test-only witnessed authority. Production
// composition must use an independently enrolled monotonic witness and must
// never acquire authority from this fixture.
type serverTestRiskAuthority struct {
	privateKey ed25519.PrivateKey

	mu        sync.Mutex
	policies  map[string]domainsecurity.ThreadRiskPolicyV1
	contracts map[string]securitycontexttest.RiskAuthorityContracts
}

func newServerTestRiskAuthority() *serverTestRiskAuthority {
	return &serverTestRiskAuthority{
		privateKey: ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x73}, ed25519.SeedSize)),
		policies:   map[string]domainsecurity.ThreadRiskPolicyV1{},
		contracts:  map[string]securitycontexttest.RiskAuthorityContracts{},
	}
}

func (authority *serverTestRiskAuthority) ResolveOrRaise(
	ctx context.Context,
	input threadriskauthorityapp.ResolveOrRaiseInput,
) (threadriskauthorityapp.Head, error) {
	if authority == nil || ctx == nil || ctx.Err() != nil {
		return threadriskauthorityapp.Head{}, errors.New("server test risk authority is unavailable")
	}
	if input.RequestedRisk != domainsecurity.RiskClassGeneral && input.RequestedRisk != domainsecurity.RiskClassCase {
		return threadriskauthorityapp.Head{}, errors.New("server test risk authority class is invalid")
	}
	authority.mu.Lock()
	existingPolicy, policyExists := authority.policies[input.ThreadID]
	existingContracts, contractsExist := authority.contracts[input.ThreadID]
	authority.mu.Unlock()
	if policyExists && contractsExist && existingPolicy.RiskClass == domainsecurity.RiskClassCase &&
		input.RequestedRisk == domainsecurity.RiskClassCase &&
		existingPolicy.WorkspaceRealPath == input.WorkspaceRealPath {
		return threadriskauthorityapp.Head{
			HasIndex: true, Index: existingContracts.Index, Request: existingContracts.Request,
			Observation: existingContracts.Observation, RiskAuthorityBinding: existingContracts.Binding,
			Policy: existingPolicy, Found: true,
		}, nil
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
	authority.policies[input.ThreadID] = policy
	authority.contracts[input.ThreadID] = contracts
	authority.mu.Unlock()
	return threadriskauthorityapp.Head{
		HasIndex: true, Index: contracts.Index, Request: contracts.Request, Observation: contracts.Observation,
		RiskAuthorityBinding: contracts.Binding, Policy: policy, Found: true,
	}, nil
}

func (authority *serverTestRiskAuthority) Current(
	ctx context.Context,
	threadID string,
) (threadriskauthorityapp.Head, error) {
	if authority == nil || ctx == nil || ctx.Err() != nil {
		return threadriskauthorityapp.Head{}, errors.New("server test risk authority is unavailable")
	}
	authority.mu.Lock()
	policy, policyOK := authority.policies[threadID]
	contracts, contractsOK := authority.contracts[threadID]
	authority.mu.Unlock()
	if !policyOK || !contractsOK {
		return threadriskauthorityapp.Head{}, errors.New("server test risk authority thread is absent")
	}
	return threadriskauthorityapp.Head{
		HasIndex: true, Index: contracts.Index, Request: contracts.Request, Observation: contracts.Observation,
		RiskAuthorityBinding: contracts.Binding, Policy: policy, Found: true,
	}, nil
}

func (authority *serverTestRiskAuthority) ValidateCurrent(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
) error {
	if authority == nil || ctx == nil || ctx.Err() != nil ||
		domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(securityContext) != nil {
		return errors.New("server test current risk authority is unavailable")
	}
	authority.mu.Lock()
	policy, policyOK := authority.policies[securityContext.ThreadID]
	contracts, contractsOK := authority.contracts[securityContext.ThreadID]
	authority.mu.Unlock()
	if !policyOK || !contractsOK || policy.WorkspaceRealPath != securityContext.WorkspaceRealPath ||
		domainsecurity.ValidateTurnPublicationPolicyForThreadRiskPolicyV1(securityContext.PublicationPolicy, policy) != nil ||
		domainsecurity.ValidateWitnessedRiskAuthorityBindingV1(
			securityContext.RiskAuthorityBinding, contracts.Index, contracts.Request, contracts.Observation,
		) != nil {
		return errors.New("server test current risk authority does not match the frozen turn")
	}
	return nil
}

func configureServerGeneralExecution(t *testing.T, handler *runtimeServerHandler) {
	t.Helper()
	if handler == nil {
		t.Fatal("server handler is unavailable")
	}
	handler.turnSecurity = turnsecurityapp.WorkspaceSecurityAuthority{Identity: testIdentityAuthority(),
		Observer: filestore.CaseBindingReader{}, RiskAuthority: newServerTestRiskAuthority(),
	}
	if handler.caseThreads == nil {
		authority := &caseThreadAuthorityStub{threads: map[string]bool{}}
		handler.caseThreads = authority
		handler.store.SetCaseThreadAuthority(authority)
	}
	// The compatibility constructor already built the thread service from its
	// fail-closed dependencies. Rebuild it from the explicit test authority.
	handler.threads = nil
}

func configureServerCaseExecution(t *testing.T, handler *runtimeServerHandler, workspace string) *admissionFailureMCP {
	t.Helper()
	observer := filestore.CaseBindingReader{}
	observation, err := observer.Observe(workspace)
	if err != nil || observation.State != domainsecurity.CaseBindingStateValid {
		t.Fatalf("server case execution fixture is invalid: observation=%#v err=%v", observation, err)
	}
	snapshot := newAdmissionFailureSnapshotAuthority(t, observation)
	source := &admissionFailureMCP{}
	handler.turnSecurity = turnsecurityapp.WorkspaceSecurityAuthority{Identity: testIdentityAuthority(),
		Observer: observer, RiskAuthority: newServerTestRiskAuthority(), SnapshotAuthority: snapshot, SnapshotAuthorityV2: snapshot,
	}
	handler.mcp = source
	handler.threads = nil
	return source
}
