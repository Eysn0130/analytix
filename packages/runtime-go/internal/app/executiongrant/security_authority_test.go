package executiongrant

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"sync"
	"testing"
	"time"

	threadriskauthorityapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	datasetsnapshotv2fixture "analytix.local/runtime-go/internal/testsupport/datasetsnapshotv2fixture"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type executionGrantSecurityFixture struct {
	Authority turnsecurityapp.WorkspaceSecurityAuthority
	Context   domainsecurity.TurnSecurityContext
	Reader    executionGrantWorkspaceStub
	Observer  *executionGrantObserver
	Risk      *executionGrantRiskAuthority
	Snapshot  *executionGrantSnapshotAuthority
}

func newExecutionGrantCaseFixture(t *testing.T, threadID, turnID, workspace, caseID string, now time.Time) *executionGrantSecurityFixture {
	t.Helper()
	observation := executionGrantCaseObservation(t, workspace, caseID, "binding:"+caseID)
	observer := &executionGrantObserver{observation: observation}
	risk := newExecutionGrantRiskAuthority()
	snapshot := newExecutionGrantSnapshotAuthority(t, observation, "initial", now)
	authority := turnsecurityapp.WorkspaceSecurityAuthority{
		Identity: testIdentityAuthority(), Observer: observer, RiskAuthority: risk,
		SnapshotAuthority: snapshot, SnapshotAuthorityV2: snapshot,
	}
	securityContext, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: context.Background(), Authority: authority, Thread: map[string]any{},
		ThreadID: threadID, TurnID: turnID, Workspace: workspace,
		Principal: testIdentityPrincipal(), IssuedAt: now,
	})
	if err != nil {
		t.Fatalf("freeze witnessed case authority: %v", err)
	}
	if err := domainsecurity.ValidateTurnSecurityContextForExecution(securityContext); err != nil ||
		!domainsecurity.TurnSecurityContextAllowsCaseEvidence(securityContext) {
		t.Fatalf("case fixture did not produce executable V2 authority: context=%#v err=%v", securityContext, err)
	}
	return &executionGrantSecurityFixture{
		Authority: authority, Context: securityContext,
		Reader: executionGrantWorkspaceStub{
			binding: domainsecurity.CaseBinding{
				Version: 1, CaseID: observation.CaseID, WorkspaceRealPath: observation.WorkspaceRealPath,
				BindingSHA256: observation.BindingSHA256, CaseBindingHash: observation.CaseBindingHash,
			},
			realPath: observation.WorkspaceRealPath,
		},
		Observer: observer, Risk: risk, Snapshot: snapshot,
	}
}

func newExecutionGrantBoundaryFixture(t *testing.T, threadID, turnID, workspace, caseID string, now time.Time) *executionGrantSecurityFixture {
	t.Helper()
	observation := executionGrantCaseObservation(t, workspace, caseID, "binding:"+caseID)
	observer := &executionGrantObserver{observation: observation}
	risk := newExecutionGrantRiskAuthority()
	snapshot := newExecutionGrantSnapshotAuthority(t, observation, "unavailable", now)
	snapshot.err = datasetsnapshotport.ErrUnavailable
	authority := turnsecurityapp.WorkspaceSecurityAuthority{
		Identity: testIdentityAuthority(), Observer: observer, RiskAuthority: risk,
		SnapshotAuthority: snapshot, SnapshotAuthorityV2: snapshot,
	}
	securityContext, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: context.Background(), Authority: authority, Thread: map[string]any{},
		ThreadID: threadID, TurnID: turnID, Workspace: workspace,
		Principal: testIdentityPrincipal(), IssuedAt: now,
	})
	if err != nil {
		t.Fatalf("freeze witnessed boundary authority: %v", err)
	}
	if err := domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(securityContext); err != nil ||
		domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) == nil ||
		!domainsecurity.TurnSecurityContextIsBoundaryOnly(securityContext) ||
		securityContext.RiskAuthorityBinding.State != domainsecurity.RiskAuthorityBindingStateWitnessed {
		t.Fatalf("boundary fixture did not preserve witnessed ordinary authority: context=%#v err=%v", securityContext, err)
	}
	return &executionGrantSecurityFixture{
		Authority: authority, Context: securityContext,
		Reader: executionGrantWorkspaceStub{
			binding: domainsecurity.CaseBinding{
				Version: 1, CaseID: observation.CaseID, WorkspaceRealPath: observation.WorkspaceRealPath,
				BindingSHA256: observation.BindingSHA256, CaseBindingHash: observation.CaseBindingHash,
			},
			realPath: observation.WorkspaceRealPath,
		},
		Observer: observer, Risk: risk, Snapshot: snapshot,
	}
}

func newExecutionGrantGeneralFixture(t *testing.T, threadID, turnID, workspace string, now time.Time) *executionGrantSecurityFixture {
	t.Helper()
	observation, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: workspace, State: domainsecurity.CaseBindingStateMissing,
	})
	if err != nil {
		t.Fatal(err)
	}
	observer := &executionGrantObserver{observation: observation}
	risk := newExecutionGrantRiskAuthority()
	authority := turnsecurityapp.WorkspaceSecurityAuthority{
		Identity: testIdentityAuthority(), Observer: observer, RiskAuthority: risk,
	}
	securityContext, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: context.Background(), Authority: authority, Thread: map[string]any{},
		ThreadID: threadID, TurnID: turnID, Workspace: workspace,
		Principal: testIdentityPrincipal(), IssuedAt: now,
	})
	if err != nil {
		t.Fatalf("freeze witnessed general authority: %v", err)
	}
	if err := domainsecurity.ValidateTurnSecurityContextForExecution(securityContext); err != nil ||
		!domainsecurity.TurnSecurityContextIsGeneral(securityContext) {
		t.Fatalf("general fixture did not produce executable V2 authority: context=%#v err=%v", securityContext, err)
	}
	return &executionGrantSecurityFixture{
		Authority: authority, Context: securityContext,
		Reader: executionGrantWorkspaceStub{realPath: workspace}, Observer: observer, Risk: risk,
	}
}

func executionGrantCaseObservation(t *testing.T, workspace, caseID, material string) domainsecurity.CaseBindingObservationV1 {
	t.Helper()
	observation, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: workspace, State: domainsecurity.CaseBindingStateValid, CaseID: caseID,
		BindingSHA256:   domainsecurity.SHA256Hex([]byte("binding-file:\x00" + material)),
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-canonical:\x00" + material)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

type executionGrantObserver struct {
	mu          sync.RWMutex
	observation domainsecurity.CaseBindingObservationV1
	err         error
}

func (observer *executionGrantObserver) Observe(string) (domainsecurity.CaseBindingObservationV1, error) {
	observer.mu.RLock()
	defer observer.mu.RUnlock()
	return observer.observation, observer.err
}

func (observer *executionGrantObserver) set(observation domainsecurity.CaseBindingObservationV1) {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	observer.observation = observation
}

type executionGrantRiskAuthority struct {
	mu        sync.RWMutex
	private   ed25519.PrivateKey
	public    ed25519.PublicKey
	policy    domainsecurity.ThreadRiskPolicyV1
	contracts securitycontexttest.RiskAuthorityContracts
	stale     bool
}

func newExecutionGrantRiskAuthority() *executionGrantRiskAuthority {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x6a}, ed25519.SeedSize))
	return &executionGrantRiskAuthority{private: privateKey, public: privateKey.Public().(ed25519.PublicKey)}
}

func (authority *executionGrantRiskAuthority) ResolveOrRaise(ctx context.Context, input threadriskauthorityapp.ResolveOrRaiseInput) (threadriskauthorityapp.Head, error) {
	if ctx == nil || ctx.Err() != nil {
		if ctx == nil {
			return threadriskauthorityapp.Head{}, errors.New("test risk witness context is absent")
		}
		return threadriskauthorityapp.Head{}, ctx.Err()
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if authority.policy.PolicyDigest != "" && authority.policy.ThreadID == input.ThreadID &&
		authority.policy.WorkspaceRealPath == input.WorkspaceRealPath &&
		(authority.policy.RiskClass == input.RequestedRisk || authority.policy.RiskClass == domainsecurity.RiskClassCase) {
		return authority.headLocked(), nil
	}
	predecessor := authority.policy.PolicyDigest
	policy, err := domainsecurity.NewThreadRiskPolicyV1(domainsecurity.ThreadRiskPolicyInputV1{
		ThreadID: input.ThreadID, WorkspaceRealPath: input.WorkspaceRealPath, RiskClass: input.RequestedRisk,
		Origin: input.Origin, SignalsDigest: input.SignalsDigest, PredecessorPolicyDigest: predecessor,
		IssuedAt: input.IssuedAt, AuthorityKeyID: domainsecurity.SHA256Hex(authority.public), AuthorityPublicKey: authority.public,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(authority.private, message), nil })
	if err != nil {
		return threadriskauthorityapp.Head{}, err
	}
	contracts, err := securitycontexttest.WitnessedRiskAuthorityContracts(
		policy.ThreadID, policy.WorkspaceRealPath, policy.RiskClass, policy.PolicyDigest,
	)
	if err != nil {
		return threadriskauthorityapp.Head{}, err
	}
	authority.policy = policy
	authority.contracts = contracts
	authority.stale = false
	return authority.headLocked(), nil
}

func (authority *executionGrantRiskAuthority) ValidateCurrent(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) error {
	if ctx == nil || ctx.Err() != nil {
		if ctx == nil {
			return errors.New("test risk witness context is absent")
		}
		return ctx.Err()
	}
	authority.mu.RLock()
	defer authority.mu.RUnlock()
	if authority.stale || authority.policy.PolicyDigest == "" ||
		securityContext.ThreadID != authority.policy.ThreadID ||
		securityContext.WorkspaceRealPath != authority.policy.WorkspaceRealPath ||
		domainsecurity.ValidateTurnPublicationPolicyForThreadRiskPolicyV1(securityContext.PublicationPolicy, authority.policy) != nil ||
		domainsecurity.ValidateWitnessedRiskAuthorityBindingV1(
			securityContext.RiskAuthorityBinding, authority.contracts.Index, authority.contracts.Request, authority.contracts.Observation,
		) != nil {
		return errors.New("test current risk witness mismatch")
	}
	return nil
}

func (authority *executionGrantRiskAuthority) headLocked() threadriskauthorityapp.Head {
	return threadriskauthorityapp.Head{
		HasIndex: true, Index: authority.contracts.Index, Request: authority.contracts.Request,
		Observation: authority.contracts.Observation, RiskAuthorityBinding: authority.contracts.Binding,
		Policy: authority.policy, Found: true,
	}
}

func (authority *executionGrantRiskAuthority) markStale() {
	authority.mu.Lock()
	defer authority.mu.Unlock()
	authority.stale = true
}

type executionGrantSnapshotAuthority struct {
	mu       sync.RWMutex
	record   domainsecurity.DatasetSnapshotAuthorityRecordV1
	resolved datasetsnapshotport.ResolvedSnapshotV2
	private  ed25519.PrivateKey
	public   ed25519.PublicKey
	err      error
}

func newExecutionGrantSnapshotAuthority(t *testing.T, observation domainsecurity.CaseBindingObservationV1, material string, now time.Time) *executionGrantSnapshotAuthority {
	t.Helper()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x73}, ed25519.SeedSize))
	authority := &executionGrantSnapshotAuthority{private: privateKey, public: privateKey.Public().(ed25519.PublicKey)}
	authority.record = authority.newRecord(t, observation, material, now)
	authority.resolved = authority.newResolvedV2(t, observation, material, now)
	return authority
}

func (authority *executionGrantSnapshotAuthority) ResolveWitnessedV2(ctx context.Context, input datasetsnapshotport.ResolveInputV2) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	if ctx == nil || ctx.Err() != nil {
		if ctx == nil {
			return datasetsnapshotport.ResolvedSnapshotV2{}, errors.New("test snapshot authority context is absent")
		}
		return datasetsnapshotport.ResolvedSnapshotV2{}, ctx.Err()
	}
	authority.mu.RLock()
	defer authority.mu.RUnlock()
	if authority.err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, authority.err
	}
	if domainsecurity.ValidateDatasetSnapshotAuthorityRecordForBindingV2(
		authority.resolved.Record, input.TenantID, input.UserID, input.Observation,
	) != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrMismatch
	}
	if input.ExpectedDatasetSnapshotID != "" && input.ExpectedDatasetSnapshotID != authority.resolved.Record.DatasetSnapshotID {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrStale
	}
	return authority.resolved, nil
}

func (authority *executionGrantSnapshotAuthority) ResolveWitnessed(ctx context.Context, input datasetsnapshotport.ResolveInput) (domainsecurity.DatasetSnapshotAuthorityRecordV1, error) {
	if ctx == nil || ctx.Err() != nil {
		if ctx == nil {
			return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, errors.New("test snapshot authority context is absent")
		}
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, ctx.Err()
	}
	authority.mu.RLock()
	defer authority.mu.RUnlock()
	if authority.record.TenantID != input.TenantID || authority.record.UserID != input.UserID ||
		domainsecurity.ValidateDatasetSnapshotAuthorityRecordForBindingV1(authority.record, input.Observation) != nil {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, errors.New("test snapshot authority binding mismatch")
	}
	return authority.record, authority.err
}

func (authority *executionGrantSnapshotAuthority) replace(t *testing.T, observation domainsecurity.CaseBindingObservationV1, material string, now time.Time) {
	t.Helper()
	record := authority.newRecord(t, observation, material, now)
	resolved := authority.newResolvedV2(t, observation, material, now)
	authority.mu.Lock()
	defer authority.mu.Unlock()
	authority.record = record
	authority.resolved = resolved
	authority.err = nil
}

func (authority *executionGrantSnapshotAuthority) newResolvedV2(t *testing.T, observation domainsecurity.CaseBindingObservationV1, material string, now time.Time) datasetsnapshotport.ResolvedSnapshotV2 {
	t.Helper()
	resolved, err := datasetsnapshotv2fixture.NewResolvedSnapshotV2(datasetsnapshotv2fixture.ResolvedInput{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		Observation: observation, Material: material,
		InstallationID: domainsecurity.SHA256Hex([]byte("execution-grant-test-installation")),
		AcceptedAt:     now,
		AuthorityKeyID: domainsecurity.SHA256Hex(authority.public), AuthorityPublicKey: authority.public,
		Sign: func(message []byte) ([]byte, error) { return ed25519.Sign(authority.private, message), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func (authority *executionGrantSnapshotAuthority) newRecord(t *testing.T, observation domainsecurity.CaseBindingObservationV1, material string, now time.Time) domainsecurity.DatasetSnapshotAuthorityRecordV1 {
	t.Helper()
	installationID := domainsecurity.SHA256Hex([]byte("execution-grant-test-installation"))
	input := domainsecurity.DatasetSnapshotAuthorityRecordInputV1{
		InstallationID: installationID, TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		WorkspaceRealPath: observation.WorkspaceRealPath, CaseID: observation.CaseID,
		CaseBindingHash: observation.CaseBindingHash, BindingObservationDigest: observation.ObservationDigest,
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-manifest:\x00" + material)),
		RawManifestSHA256:  domainsecurity.SHA256Hex([]byte("raw-manifest:\x00" + material)),
		ParserVersion:      "execution-grant-test-parser/1.0.0", AcceptedAt: now,
		AuthorityKeyID: domainsecurity.SHA256Hex(authority.public), AuthorityPublicKey: authority.public,
	}
	record, err := domainsecurity.NewDatasetSnapshotAuthorityRecordV1(input, func(message []byte) ([]byte, error) {
		return ed25519.Sign(authority.private, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return record
}
