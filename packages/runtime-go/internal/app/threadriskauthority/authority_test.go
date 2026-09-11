package threadriskauthority

import (
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	monotonicheadport "analytix.local/runtime-go/internal/ports/monotonichead"
	storeport "analytix.local/runtime-go/internal/ports/threadriskauthority"
)

func TestGenerationZeroFirstAdvanceAndExactCurrent(t *testing.T) {
	fixture := newAuthorityFixture(t)

	empty, err := fixture.authority.Current(context.Background(), "thread-a")
	if err != nil {
		t.Fatal(err)
	}
	if empty.HasIndex || empty.Found || empty.Observation.Checkpoint.Generation != 0 {
		t.Fatalf("generation zero must not invent local authority: %#v", empty)
	}

	first, err := fixture.authority.ResolveOrRaise(context.Background(), fixture.generalInput("thread-a", fixture.workspace, 1))
	if err != nil {
		t.Fatal(err)
	}
	if !first.HasIndex || !first.Found || first.Index.Generation != 1 || first.Policy.RiskClass != domainsecurity.RiskClassGeneral ||
		first.Index.IndexDigest != first.Observation.Checkpoint.CurrentStateDigest ||
		first.RiskAuthorityBinding.ObservationDigest != first.Observation.ObservationDigest {
		t.Fatalf("first witnessed head is incomplete: %#v", first)
	}
	if fixture.policies.listCalls.Load() != 0 {
		t.Fatal("witnessed authority must never list policies to infer current")
	}

	current, err := fixture.authority.ResolveCurrent(context.Background(), "thread-a")
	if err != nil {
		t.Fatal(err)
	}
	if !equalIndex(current.Index, first.Index) || !equalPolicy(current.Policy, first.Policy) ||
		current.Request.ChallengeNonce == first.Request.ChallengeNonce {
		t.Fatal("current did not use an exact index with a fresh challenge")
	}
}

func TestGenerationZeroAfterRestartRejectsExistingProjection(t *testing.T) {
	fixture := newAuthorityFixture(t)
	fixture.witness.mu.Lock()
	emptyCheckpoint := fixture.witness.checkpoint
	fixture.witness.mu.Unlock()

	committed, err := fixture.authority.ResolveOrRaise(context.Background(), fixture.generalInput("thread-a", fixture.workspace, 1))
	if err != nil {
		t.Fatal(err)
	}
	fixture.witness.mu.Lock()
	fixture.witness.checkpoint = emptyCheckpoint
	fixture.witness.mu.Unlock()

	restarted, err := New(fixture.config(91))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Current(context.Background(), "thread-a"); !errors.Is(err, ErrIntegrity) ||
		!errors.Is(err, monotonicheadport.ErrEquivocation) {
		t.Fatalf("fresh generation zero replay accepted a non-empty durable projection: %v", err)
	}
	if fixture.projection.current().IndexDigest != committed.Index.IndexDigest {
		t.Fatal("generation zero replay mutated the existing projection")
	}
}

func TestSameGenerationCheckpointEquivocationAfterRestartIsRejected(t *testing.T) {
	fixture := newAuthorityFixture(t)
	committed, err := fixture.authority.ResolveOrRaise(context.Background(), fixture.generalInput("thread-a", fixture.workspace, 1))
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := committed.Observation.Checkpoint
	fork, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: checkpoint.InstallationID, EnrollmentID: checkpoint.EnrollmentID, Namespace: checkpoint.Namespace,
		Generation: checkpoint.Generation, CurrentStateDigest: checkpoint.CurrentStateDigest,
		PreviousStateDigest: checkpoint.PreviousStateDigest, PreviousCheckpointDigest: checkpoint.PreviousCheckpointDigest,
		FenceNonce: domainsecurity.SHA256Hex([]byte("restart-fork-fence")), MutationID: checkpoint.MutationID,
		WitnessKeyID: checkpoint.WitnessKeyID, WitnessPublicKey: fixture.witnessKey.Public().(ed25519.PublicKey),
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.witnessKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	fixture.witness.mu.Lock()
	fixture.witness.checkpoint = fork
	fixture.witness.mu.Unlock()
	restarted, err := New(fixture.config(92))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Current(context.Background(), "thread-a"); !errors.Is(err, ErrIntegrity) ||
		!errors.Is(err, monotonicheadport.ErrCheckpointFloorConflict) || !errors.Is(err, monotonicheadport.ErrEquivocation) {
		t.Fatalf("same-generation checkpoint fork survived restart: %v", err)
	}
}

func TestCheckpointFloorFailureClassification(t *testing.T) {
	tests := []struct {
		name             string
		failure          error
		wantUnavailable  bool
		wantIntegrity    bool
		wantEquivocation bool
	}{
		{name: "unavailable", failure: monotonicheadport.ErrCheckpointFloorUnavailable, wantUnavailable: true},
		{name: "indeterminate", failure: monotonicheadport.ErrCheckpointFloorIndeterminate, wantUnavailable: true},
		{name: "bootstrap", failure: monotonicheadport.ErrCheckpointFloorBootstrap, wantIntegrity: true},
		{name: "conflict", failure: monotonicheadport.ErrCheckpointFloorConflict, wantIntegrity: true, wantEquivocation: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAuthorityFixture(t)
			fixture.checkpointFloor.failure = test.failure
			_, err := fixture.authority.Current(context.Background(), "thread-a")
			if !errors.Is(err, test.failure) || errors.Is(err, ErrUnavailable) != test.wantUnavailable ||
				errors.Is(err, ErrIntegrity) != test.wantIntegrity || errors.Is(err, monotonicheadport.ErrEquivocation) != test.wantEquivocation {
				t.Fatalf("checkpoint floor failure was misclassified: %v", err)
			}
		})
	}
}

func TestResolveOrRaiseIsStrictlyMonotonic(t *testing.T) {
	fixture := newAuthorityFixture(t)
	general, err := fixture.authority.ResolveOrRaise(context.Background(), fixture.generalInput("thread-a", fixture.workspace, 1))
	if err != nil {
		t.Fatal(err)
	}
	caseHead, err := fixture.authority.ResolveOrRaise(context.Background(), fixture.caseInput("thread-a", fixture.workspace, 2))
	if err != nil {
		t.Fatal(err)
	}
	if caseHead.Policy.RiskClass != domainsecurity.RiskClassCase || caseHead.Index.Generation != general.Index.Generation+1 {
		t.Fatal("general-to-case did not advance exactly one generation")
	}

	downgrade, err := fixture.authority.ResolveOrRaise(context.Background(), fixture.generalInput("thread-a", fixture.workspace, 3))
	if err != nil || downgrade.Policy.RiskClass != domainsecurity.RiskClassCase || downgrade.Index.IndexDigest != caseHead.Index.IndexDigest {
		t.Fatal("a lower request must return the current case head without downgrade")
	}
	if _, err := fixture.authority.ResolveOrRaise(context.Background(), fixture.caseInput("thread-a", fixture.workspace+"-moved", 4)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("case workspace move was not rejected: %v", err)
	}

	other := newAuthorityFixture(t)
	first, err := other.authority.ResolveOrRaise(context.Background(), other.generalInput("thread-b", other.workspace, 1))
	if err != nil {
		t.Fatal(err)
	}
	moved, err := other.authority.ResolveOrRaise(context.Background(), other.generalInput("thread-b", other.workspace+"-moved", 2))
	if err != nil {
		t.Fatal(err)
	}
	if moved.Policy.WorkspaceRealPath == first.Policy.WorkspaceRealPath || moved.Policy.PredecessorPolicyDigest != first.Policy.PolicyDigest {
		t.Fatal("general workspace move was not represented by a signed successor")
	}
}

func TestValidateCurrentAllowsUnrelatedAdvanceAndRejectsSameThreadAdvance(t *testing.T) {
	fixture := newAuthorityFixture(t)
	threadA, err := fixture.authority.ResolveOrRaise(context.Background(), fixture.generalInput("thread-a", fixture.workspace, 1))
	if err != nil {
		t.Fatal(err)
	}
	securityContext := fixture.generalContext(t, "thread-a", fixture.workspace, threadA, 1)

	if _, err := fixture.authority.ResolveOrRaise(context.Background(), fixture.generalInput("thread-b", fixture.workspace+"-b", 2)); err != nil {
		t.Fatal(err)
	}
	if err := fixture.authority.ValidateCurrent(context.Background(), securityContext); err != nil {
		t.Fatalf("unrelated global index advance killed an unchanged thread: %v", err)
	}

	if _, err := fixture.authority.ResolveOrRaise(context.Background(), fixture.caseInput("thread-a", fixture.workspace, 3)); err != nil {
		t.Fatal(err)
	}
	if err := fixture.authority.ValidateCurrent(context.Background(), securityContext); !errors.Is(err, ErrCurrentThreadChanged) {
		t.Fatalf("same-thread risk advance did not invalidate old TSC: %v", err)
	}
}

func TestValidateCurrentAllowsBoundaryOnlyOrdinaryEffectWithoutCaseFactAuthority(t *testing.T) {
	fixture := newAuthorityFixture(t)
	head, err := fixture.authority.ResolveOrRaise(
		context.Background(),
		fixture.caseInput("thread-boundary", fixture.workspace, 1),
	)
	if err != nil {
		t.Fatal(err)
	}
	observationDigest := domainsecurity.SHA256Hex([]byte("missing-case-binding"))
	publication, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest:   head.Policy.PolicyDigest,
		RiskClass:                domainsecurity.RiskClassCase,
		Disposition:              domainsecurity.PublicationDispositionCaseBoundaryOnly,
		CaseBindingState:         domainsecurity.CaseBindingStateMissing,
		BindingObservationDigest: observationDigest,
		BlockerCode:              domainsecurity.PublicationBlockerCaseBindingMissing,
	})
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-boundary", TurnID: "turn-boundary", WorkspaceRealPath: fixture.workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: domainsecurity.UnboundCaseID, CaseBindingHash: domainsecurity.UnboundCaseBindingHash(fixture.workspace),
		DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID, SourceManifestHash: domainsecurity.EmptySourceManifestHash,
		ContextEpoch: 1, IssuedAt: time.Date(2026, 7, 12, 1, 0, 0, 0, time.UTC),
		PublicationPolicy: publication, RiskAuthorityBinding: head.RiskAuthorityBinding,
	})
	if err != nil {
		t.Fatal(err)
	}
	if domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) == nil {
		t.Fatal("boundary-only context acquired case/provider execution authority")
	}
	if err := domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(securityContext); err != nil {
		t.Fatalf("boundary-only context lost ordinary-effect authority: %v", err)
	}
	if err := fixture.authority.ValidateCurrent(context.Background(), securityContext); err != nil {
		t.Fatalf("unchanged witnessed risk authority blocked the ordinary effect: %v", err)
	}
}

func TestValidateCurrentRejectsMissingAndFakeObservationBundle(t *testing.T) {
	fixture := newAuthorityFixture(t)
	head, err := fixture.authority.ResolveOrRaise(context.Background(), fixture.generalInput("thread-a", fixture.workspace, 1))
	if err != nil {
		t.Fatal(err)
	}
	securityContext := fixture.generalContext(t, "thread-a", fixture.workspace, head, 1)

	fixture.observations.delete(head.Observation.ObservationDigest)
	if err := fixture.authority.ValidateCurrent(context.Background(), securityContext); !errors.Is(err, ErrHistoricalObservationAbsent) {
		t.Fatalf("missing exact observation bundle was accepted: %v", err)
	}

	fixture.observations.putRaw(storeport.ObservationBundle{
		Index: head.Index, Request: head.Request, Observation: head.Observation,
	})
	fixture.observations.mutate(head.Observation.ObservationDigest, func(bundle *storeport.ObservationBundle) {
		bundle.Request.ChallengeNonce = domainsecurity.SHA256Hex([]byte("forged-challenge"))
	})
	if err := fixture.authority.ValidateCurrent(context.Background(), securityContext); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("fake observation bundle was accepted: %v", err)
	}
}

func TestLocalRollbackAndProjectionCannotSelectCurrent(t *testing.T) {
	fixture := newAuthorityFixture(t)
	first, err := fixture.authority.ResolveOrRaise(context.Background(), fixture.generalInput("thread-a", fixture.workspace, 1))
	if err != nil {
		t.Fatal(err)
	}
	indexSnapshot := fixture.indexes.snapshot()
	policySnapshot := fixture.policies.snapshot()
	observationSnapshot := fixture.observations.snapshot()

	latest, err := fixture.authority.ResolveOrRaise(context.Background(), fixture.caseInput("thread-a", fixture.workspace, 2))
	if err != nil {
		t.Fatal(err)
	}
	fixture.projection.set(first.Index)
	current, err := fixture.authority.Current(context.Background(), "thread-a")
	if err != nil || current.Index.IndexDigest != latest.Index.IndexDigest || fixture.projection.current().IndexDigest != latest.Index.IndexDigest {
		t.Fatalf("old projection influenced current: %v", err)
	}

	fixture.indexes.delete(latest.Index.IndexDigest)
	if _, err := fixture.authority.Current(context.Background(), "thread-a"); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("tail deletion did not fail closed: %v", err)
	}
	fixture.indexes.restore(indexSnapshot)
	fixture.policies.restore(policySnapshot)
	fixture.observations.restore(observationSnapshot)
	fixture.projection.set(first.Index)
	if _, err := fixture.authority.Current(context.Background(), "thread-a"); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("whole local directory rollback downgraded witness current: %v", err)
	}
}

func TestMissingOrCorruptPolicyAndIndexChainsFailClosed(t *testing.T) {
	fixture := newAuthorityFixture(t)
	first, err := fixture.authority.ResolveOrRaise(context.Background(), fixture.generalInput("thread-a", fixture.workspace, 1))
	if err != nil {
		t.Fatal(err)
	}
	latest, err := fixture.authority.ResolveOrRaise(context.Background(), fixture.caseInput("thread-a", fixture.workspace, 2))
	if err != nil {
		t.Fatal(err)
	}

	fixture.policies.delete(first.Policy.PolicyDigest)
	if _, err := fixture.authority.Current(context.Background(), "thread-a"); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("detached policy predecessor was accepted: %v", err)
	}
	fixture.policies.putRaw(first.Policy)
	fixture.indexes.delete(first.Index.IndexDigest)
	if _, err := fixture.authority.Current(context.Background(), "thread-a"); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("detached index predecessor was accepted: %v", err)
	}
	fixture.indexes.putRaw(first.Index)
	fixture.indexes.mutate(first.Index.IndexDigest, func(index *domainsecurity.ThreadRiskAuthorityIndexV1) {
		index.MutationID = domainsecurity.SHA256Hex([]byte("corrupt-index"))
	})
	if _, err := fixture.authority.Current(context.Background(), "thread-a"); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("corrupt index chain was accepted: %v", err)
	}
	_ = latest
}

func TestAdvanceCASConflictIsNotLocallyGuessed(t *testing.T) {
	fixture := newAuthorityFixture(t)
	fixture.witness.setAdvanceFault(witnessFaultCASConflict)
	_, err := fixture.authority.ResolveOrRaise(context.Background(), fixture.generalInput("thread-a", fixture.workspace, 1))
	if !errors.Is(err, monotonicheadport.ErrCASConflict) {
		t.Fatalf("CAS conflict was not preserved: %v", err)
	}
	if fixture.witness.generation() != 0 {
		t.Fatal("CAS conflict changed witness head")
	}
}

func TestIndeterminateAdvanceReconcilesOnlyByFreshObserve(t *testing.T) {
	t.Run("committed", func(t *testing.T) {
		fixture := newAuthorityFixture(t)
		fixture.witness.setAdvanceFault(witnessFaultIndeterminateCommitted)
		head, err := fixture.authority.ResolveOrRaise(context.Background(), fixture.generalInput("thread-a", fixture.workspace, 1))
		if err != nil || !head.HasIndex || head.Index.Generation != 1 {
			t.Fatalf("committed indeterminate advance did not reconcile: %v", err)
		}
		if fixture.witness.advanceCalls.Load() != 1 {
			t.Fatal("indeterminate reconciliation resent Advance")
		}
	})

	t.Run("not_committed", func(t *testing.T) {
		fixture := newAuthorityFixture(t)
		fixture.witness.setAdvanceFault(witnessFaultIndeterminateUncommitted)
		_, err := fixture.authority.ResolveOrRaise(context.Background(), fixture.generalInput("thread-a", fixture.workspace, 1))
		if !errors.Is(err, monotonicheadport.ErrIndeterminate) || !errors.Is(err, ErrAdvanceNotCommitted) {
			t.Fatalf("uncommitted outcome was not typed: %v", err)
		}
		if fixture.witness.advanceCalls.Load() != 1 {
			t.Fatal("uncommitted indeterminate advance was resent")
		}
	})

	t.Run("third_head", func(t *testing.T) {
		fixture := newAuthorityFixture(t)
		fixture.witness.setAdvanceFault(witnessFaultIndeterminateThirdHead)
		_, err := fixture.authority.ResolveOrRaise(context.Background(), fixture.generalInput("thread-a", fixture.workspace, 1))
		if !errors.Is(err, monotonicheadport.ErrEquivocation) || !errors.Is(err, ErrIntegrity) {
			t.Fatalf("third head was not classified as equivocation: %v", err)
		}
	})
}

func TestWitnessEquivocationAndContextCancellationFailClosed(t *testing.T) {
	fixture := newAuthorityFixture(t)
	if _, err := fixture.authority.ResolveOrRaise(context.Background(), fixture.generalInput("thread-a", fixture.workspace, 1)); err != nil {
		t.Fatal(err)
	}
	fixture.witness.equivocateSameGeneration()
	if _, err := fixture.authority.Current(context.Background(), "thread-a"); !errors.Is(err, monotonicheadport.ErrEquivocation) {
		t.Fatalf("same-generation witness equivocation was accepted: %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	other := newAuthorityFixture(t)
	if _, err := other.authority.Current(canceled, "thread-a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled current returned %v", err)
	}
	if _, err := other.authority.ResolveOrRaise(canceled, other.generalInput("thread-a", other.workspace, 1)); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled mutation returned %v", err)
	}
}

func TestConcurrentRemoteCASHasOneWinner(t *testing.T) {
	fixture := newAuthorityFixture(t)
	fixture.witness.barrierFirstAdvances(2)
	second, err := New(fixture.config(91))
	if err != nil {
		t.Fatal(err)
	}

	var successes atomic.Int32
	var conflicts atomic.Int32
	var wait sync.WaitGroup
	for _, authority := range []*Authority{fixture.authority, second} {
		wait.Add(1)
		go func(candidate *Authority) {
			defer wait.Done()
			_, resolveErr := candidate.ResolveOrRaise(context.Background(), fixture.generalInput("thread-a", fixture.workspace, 1))
			switch {
			case resolveErr == nil:
				successes.Add(1)
			case errors.Is(resolveErr, monotonicheadport.ErrCASConflict):
				conflicts.Add(1)
			default:
				t.Errorf("unexpected concurrent result: %v", resolveErr)
			}
		}(authority)
	}
	wait.Wait()
	if successes.Load() != 1 || conflicts.Load() != 1 || fixture.witness.generation() != 1 || fixture.witness.advanceCalls.Load() != 2 {
		t.Fatalf("remote CAS results: successes=%d conflicts=%d generation=%d advanceCalls=%d", successes.Load(), conflicts.Load(), fixture.witness.generation(), fixture.witness.advanceCalls.Load())
	}
}

type authorityFixture struct {
	t               *testing.T
	installation    *testInstallationAuthority
	witnessKey      ed25519.PrivateKey
	witness         *testWitness
	indexes         *memoryIndexStore
	observations    *memoryObservationStore
	policies        *memoryPolicyStore
	projection      *memoryProjection
	checkpointFloor *memoryCheckpointFloor
	authority       *Authority
	installationID  string
	enrollmentID    string
	workspace       string
}

func newAuthorityFixture(t *testing.T) *authorityFixture {
	t.Helper()
	installation := newTestInstallationAuthority(11)
	_, witnessPrivate, err := ed25519.GenerateKey(&repeatReader{value: 22})
	if err != nil {
		t.Fatal(err)
	}
	fixture := &authorityFixture{
		t:               t,
		installation:    installation,
		witnessKey:      witnessPrivate,
		indexes:         newMemoryIndexStore(),
		observations:    newMemoryObservationStore(),
		policies:        newMemoryPolicyStore(),
		projection:      &memoryProjection{},
		checkpointFloor: &memoryCheckpointFloor{},
		installationID:  domainsecurity.SHA256Hex([]byte("test-installation")),
		enrollmentID:    domainsecurity.SHA256Hex([]byte("test-enrollment")),
		workspace:       "/private/tmp/analytix-thread-risk-test",
	}
	fixture.witness = newTestWitness(t, fixture.installationID, fixture.enrollmentID, witnessPrivate)
	fixture.authority, err = New(fixture.config(44))
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}

func (fixture *authorityFixture) config(randomStart byte) Config {
	witnessPublic := fixture.witnessKey.Public().(ed25519.PublicKey)
	return Config{
		InstallationID:   fixture.installationID,
		EnrollmentID:     fixture.enrollmentID,
		Namespace:        domainsecurity.ThreadRiskAuthorityNamespaceV1,
		Authority:        fixture.installation,
		WitnessKeyID:     domainsecurity.SHA256Hex(witnessPublic),
		WitnessPublicKey: witnessPublic,
		Random:           &counterReader{next: uint64(randomStart)},
		Witness:          fixture.witness,
		CheckpointFloor:  fixture.checkpointFloor,
		Indexes:          fixture.indexes,
		Observations:     fixture.observations,
		Projection:       fixture.projection,
		Policies:         fixture.policies,
	}
}

func (fixture *authorityFixture) generalInput(threadID, workspace string, tick int) ResolveOrRaiseInput {
	return ResolveOrRaiseInput{
		ThreadID:          threadID,
		WorkspaceRealPath: workspace,
		RequestedRisk:     domainsecurity.RiskClassGeneral,
		Origin:            domainsecurity.RiskPolicyOriginGeneralWorkspace,
		SignalsDigest:     domainsecurity.SHA256Hex([]byte("general-signals")),
		IssuedAt:          time.Date(2026, 7, 12, 0, 0, tick, 0, time.UTC),
	}
}

func (fixture *authorityFixture) caseInput(threadID, workspace string, tick int) ResolveOrRaiseInput {
	return ResolveOrRaiseInput{
		ThreadID:          threadID,
		WorkspaceRealPath: workspace,
		RequestedRisk:     domainsecurity.RiskClassCase,
		Origin:            domainsecurity.RiskPolicyOriginValidCaseBinding,
		SignalsDigest:     domainsecurity.SHA256Hex([]byte("case-signals")),
		IssuedAt:          time.Date(2026, 7, 12, 0, 0, tick, 0, time.UTC),
	}
}

func (fixture *authorityFixture) generalContext(t *testing.T, threadID, workspace string, head Head, epoch uint64) domainsecurity.TurnSecurityContext {
	t.Helper()
	publication, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest:   head.Policy.PolicyDigest,
		RiskClass:                domainsecurity.RiskClassGeneral,
		Disposition:              domainsecurity.PublicationDispositionGeneralOutput,
		CaseBindingState:         domainsecurity.CaseBindingStateMissing,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("missing-binding")),
		BlockerCode:              domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID:             threadID,
		TurnID:               "turn-1",
		WorkspaceRealPath:    workspace,
		TenantID:             domainsecurity.LocalTenantID,
		UserID:               domainsecurity.LocalUserID,
		CaseID:               domainsecurity.UnboundCaseID,
		CaseBindingHash:      domainsecurity.UnboundCaseBindingHash(workspace),
		DatasetSnapshotID:    domainsecurity.NoDatasetSnapshotID,
		SourceManifestHash:   domainsecurity.EmptySourceManifestHash,
		ContextEpoch:         epoch,
		IssuedAt:             time.Date(2026, 7, 12, 1, 0, 0, 0, time.UTC),
		PublicationPolicy:    publication,
		RiskAuthorityBinding: head.RiskAuthorityBinding,
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

type testInstallationAuthority struct {
	private ed25519.PrivateKey
	public  ed25519.PublicKey
}

func newTestInstallationAuthority(seed byte) *testInstallationAuthority {
	public, private, _ := ed25519.GenerateKey(&repeatReader{value: seed})
	return &testInstallationAuthority{private: private, public: public}
}

func (authority *testInstallationAuthority) KeyID() string {
	return domainsecurity.SHA256Hex(authority.public)
}
func (authority *testInstallationAuthority) PublicKey() []byte {
	return append([]byte(nil), authority.public...)
}
func (authority *testInstallationAuthority) Sign(ctx context.Context, body []byte) ([]byte, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return ed25519.Sign(authority.private, body), nil
}
func (authority *testInstallationAuthority) VerifyTrusted(ctx context.Context, keyID string, publicKey, body, signature []byte) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if keyID != authority.KeyID() || !ed25519.PublicKey(publicKey).Equal(authority.public) || !ed25519.Verify(authority.public, body, signature) {
		return errors.New("untrusted")
	}
	return nil
}

type repeatReader struct{ value byte }

func (reader *repeatReader) Read(body []byte) (int, error) {
	for position := range body {
		body[position] = reader.value
	}
	return len(body), nil
}

type counterReader struct {
	mu   sync.Mutex
	next uint64
}

func (reader *counterReader) Read(body []byte) (int, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	reader.next++
	for position := range body {
		body[position] = byte(position*37 + 11)
	}
	if len(body) >= 8 {
		binary.LittleEndian.PutUint64(body[:8], reader.next)
	}
	return len(body), nil
}

type witnessFault string

const (
	witnessFaultNone                     witnessFault = ""
	witnessFaultCASConflict              witnessFault = "cas_conflict"
	witnessFaultIndeterminateCommitted   witnessFault = "indeterminate_committed"
	witnessFaultIndeterminateUncommitted witnessFault = "indeterminate_uncommitted"
	witnessFaultIndeterminateThirdHead   witnessFault = "indeterminate_third_head"
)

type testWitness struct {
	t                     *testing.T
	mu                    sync.Mutex
	private               ed25519.PrivateKey
	public                ed25519.PublicKey
	checkpoint            domainsecurity.MonotonicHeadCheckpointV1
	fault                 witnessFault
	advanceCalls          atomic.Int32
	advanceBarrierCount   int
	advanceBarrierRelease chan struct{}
}

func newTestWitness(t *testing.T, installationID, enrollmentID string, private ed25519.PrivateKey) *testWitness {
	t.Helper()
	public := private.Public().(ed25519.PublicKey)
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID:     installationID,
		EnrollmentID:       enrollmentID,
		Namespace:          domainsecurity.ThreadRiskAuthorityNamespaceV1,
		Generation:         0,
		CurrentStateDigest: domainsecurity.SHA256Hex([]byte("enrolled-empty-risk-authority")),
		FenceNonce:         domainsecurity.SHA256Hex([]byte("enrollment-fence")),
		WitnessKeyID:       domainsecurity.SHA256Hex(public),
		WitnessPublicKey:   public,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(private, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	return &testWitness{t: t, private: private, public: public, checkpoint: checkpoint}
}

func (witness *testWitness) Observe(ctx context.Context, request domainsecurity.MonotonicHeadObserveRequestV1) (domainsecurity.MonotonicHeadObservationV1, error) {
	if ctx.Err() != nil {
		return domainsecurity.MonotonicHeadObservationV1{}, ctx.Err()
	}
	witness.mu.Lock()
	checkpoint := witness.checkpoint
	witness.mu.Unlock()
	return domainsecurity.NewMonotonicHeadObservationV1(request, checkpoint, func(message []byte) ([]byte, error) {
		return ed25519.Sign(witness.private, message), nil
	})
}

func (witness *testWitness) Advance(ctx context.Context, request domainsecurity.MonotonicHeadAdvanceRequestV1) (domainsecurity.MonotonicHeadAdvanceReceiptV1, error) {
	call := int(witness.advanceCalls.Add(1))
	if ctx.Err() != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, ctx.Err()
	}
	witness.mu.Lock()
	barrierCount := witness.advanceBarrierCount
	barrierRelease := witness.advanceBarrierRelease
	witness.mu.Unlock()
	if barrierCount > 0 && call <= barrierCount {
		if call == barrierCount {
			close(barrierRelease)
		}
		select {
		case <-barrierRelease:
		case <-ctx.Done():
			return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, ctx.Err()
		}
	}
	witness.mu.Lock()
	defer witness.mu.Unlock()
	fault := witness.fault
	witness.fault = witnessFaultNone
	if fault == witnessFaultCASConflict {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonicheadport.ErrCASConflict
	}
	previous := witness.checkpoint
	if request.ExpectedGeneration != previous.Generation || request.ExpectedCheckpointDigest != previous.CheckpointDigest ||
		request.ExpectedStateDigest != previous.CurrentStateDigest || request.ExpectedFenceNonce != previous.FenceNonce {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonicheadport.ErrCASConflict
	}
	if fault == witnessFaultIndeterminateUncommitted {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonicheadport.ErrIndeterminate
	}
	nextState := request.NextStateDigest
	if fault == witnessFaultIndeterminateThirdHead {
		nextState = domainsecurity.SHA256Hex([]byte("incompatible-third-head"))
	}
	next, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID:           previous.InstallationID,
		EnrollmentID:             previous.EnrollmentID,
		Namespace:                previous.Namespace,
		Generation:               request.NextGeneration,
		CurrentStateDigest:       nextState,
		PreviousStateDigest:      previous.CurrentStateDigest,
		PreviousCheckpointDigest: previous.CheckpointDigest,
		FenceNonce:               domainsecurity.SHA256Hex([]byte("fence:" + request.MutationID + ":" + nextState)),
		MutationID:               request.MutationID,
		WitnessKeyID:             domainsecurity.SHA256Hex(witness.public),
		WitnessPublicKey:         witness.public,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(witness.private, message), nil })
	if err != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, err
	}
	witness.checkpoint = next
	if fault == witnessFaultIndeterminateCommitted || fault == witnessFaultIndeterminateThirdHead {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonicheadport.ErrIndeterminate
	}
	receipt, err := domainsecurity.NewMonotonicHeadAdvanceReceiptV1(request, next, func(message []byte) ([]byte, error) {
		return ed25519.Sign(witness.private, message), nil
	})
	return receipt, err
}

func (witness *testWitness) setAdvanceFault(fault witnessFault) {
	witness.mu.Lock()
	witness.fault = fault
	witness.mu.Unlock()
}
func (witness *testWitness) generation() uint64 {
	witness.mu.Lock()
	defer witness.mu.Unlock()
	return witness.checkpoint.Generation
}
func (witness *testWitness) barrierFirstAdvances(count int) {
	witness.mu.Lock()
	witness.advanceBarrierCount = count
	witness.advanceBarrierRelease = make(chan struct{})
	witness.mu.Unlock()
}
func (witness *testWitness) equivocateSameGeneration() {
	witness.mu.Lock()
	defer witness.mu.Unlock()
	previous := witness.checkpoint
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID:           previous.InstallationID,
		EnrollmentID:             previous.EnrollmentID,
		Namespace:                previous.Namespace,
		Generation:               previous.Generation,
		CurrentStateDigest:       previous.CurrentStateDigest,
		PreviousStateDigest:      previous.PreviousStateDigest,
		PreviousCheckpointDigest: previous.PreviousCheckpointDigest,
		FenceNonce:               domainsecurity.SHA256Hex([]byte("equivocated-fence")),
		MutationID:               previous.MutationID,
		WitnessKeyID:             domainsecurity.SHA256Hex(witness.public),
		WitnessPublicKey:         witness.public,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(witness.private, message), nil })
	if err != nil {
		witness.t.Fatal(err)
	}
	witness.checkpoint = checkpoint
}

type memoryIndexStore struct {
	mu      sync.Mutex
	records map[string]domainsecurity.ThreadRiskAuthorityIndexV1
}

func newMemoryIndexStore() *memoryIndexStore {
	return &memoryIndexStore{records: make(map[string]domainsecurity.ThreadRiskAuthorityIndexV1)}
}
func (store *memoryIndexStore) PutIfAbsent(ctx context.Context, index domainsecurity.ThreadRiskAuthorityIndexV1) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if current, found := store.records[index.IndexDigest]; found && !equalIndex(current, index) {
		return errors.New("conflict")
	}
	store.records[index.IndexDigest] = index
	return nil
}
func (store *memoryIndexStore) Resolve(ctx context.Context, digest string) (domainsecurity.ThreadRiskAuthorityIndexV1, error) {
	if ctx.Err() != nil {
		return domainsecurity.ThreadRiskAuthorityIndexV1{}, ctx.Err()
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	index, found := store.records[digest]
	if !found {
		return domainsecurity.ThreadRiskAuthorityIndexV1{}, errors.New("absent")
	}
	return index, nil
}
func (store *memoryIndexStore) snapshot() map[string]domainsecurity.ThreadRiskAuthorityIndexV1 {
	store.mu.Lock()
	defer store.mu.Unlock()
	copyRecords := make(map[string]domainsecurity.ThreadRiskAuthorityIndexV1, len(store.records))
	for digest, index := range store.records {
		copyRecords[digest] = index
	}
	return copyRecords
}
func (store *memoryIndexStore) restore(records map[string]domainsecurity.ThreadRiskAuthorityIndexV1) {
	store.mu.Lock()
	store.records = records
	store.mu.Unlock()
}
func (store *memoryIndexStore) delete(digest string) {
	store.mu.Lock()
	delete(store.records, digest)
	store.mu.Unlock()
}
func (store *memoryIndexStore) putRaw(index domainsecurity.ThreadRiskAuthorityIndexV1) {
	store.mu.Lock()
	store.records[index.IndexDigest] = index
	store.mu.Unlock()
}
func (store *memoryIndexStore) mutate(digest string, change func(*domainsecurity.ThreadRiskAuthorityIndexV1)) {
	store.mu.Lock()
	index := store.records[digest]
	change(&index)
	store.records[digest] = index
	store.mu.Unlock()
}

type memoryPolicyStore struct {
	mu        sync.Mutex
	records   map[string]domainsecurity.ThreadRiskPolicyV1
	listCalls atomic.Int32
}

func newMemoryPolicyStore() *memoryPolicyStore {
	return &memoryPolicyStore{records: make(map[string]domainsecurity.ThreadRiskPolicyV1)}
}
func (store *memoryPolicyStore) PutIfAbsent(ctx context.Context, policy domainsecurity.ThreadRiskPolicyV1) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if current, found := store.records[policy.PolicyDigest]; found && !equalPolicy(current, policy) {
		return errors.New("conflict")
	}
	store.records[policy.PolicyDigest] = policy
	return nil
}
func (store *memoryPolicyStore) Resolve(ctx context.Context, digest string) (domainsecurity.ThreadRiskPolicyV1, error) {
	if ctx.Err() != nil {
		return domainsecurity.ThreadRiskPolicyV1{}, ctx.Err()
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	policy, found := store.records[digest]
	if !found {
		return domainsecurity.ThreadRiskPolicyV1{}, errors.New("absent")
	}
	return policy, nil
}
func (store *memoryPolicyStore) List(ctx context.Context) ([]domainsecurity.ThreadRiskPolicyV1, error) {
	store.listCalls.Add(1)
	return nil, errors.New("list forbidden")
}
func (store *memoryPolicyStore) HasRecords(context.Context) (bool, error) {
	return false, errors.New("scan forbidden")
}
func (store *memoryPolicyStore) snapshot() map[string]domainsecurity.ThreadRiskPolicyV1 {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make(map[string]domainsecurity.ThreadRiskPolicyV1, len(store.records))
	for digest, policy := range store.records {
		result[digest] = policy
	}
	return result
}
func (store *memoryPolicyStore) restore(records map[string]domainsecurity.ThreadRiskPolicyV1) {
	store.mu.Lock()
	store.records = records
	store.mu.Unlock()
}
func (store *memoryPolicyStore) delete(digest string) {
	store.mu.Lock()
	delete(store.records, digest)
	store.mu.Unlock()
}
func (store *memoryPolicyStore) putRaw(policy domainsecurity.ThreadRiskPolicyV1) {
	store.mu.Lock()
	store.records[policy.PolicyDigest] = policy
	store.mu.Unlock()
}

type memoryObservationStore struct {
	mu      sync.Mutex
	records map[string]storeport.ObservationBundle
}

func newMemoryObservationStore() *memoryObservationStore {
	return &memoryObservationStore{records: make(map[string]storeport.ObservationBundle)}
}
func (store *memoryObservationStore) PutIfAbsent(ctx context.Context, bundle storeport.ObservationBundle) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	digest := bundle.Observation.ObservationDigest
	if current, found := store.records[digest]; found && !equalBundle(current, bundle) {
		return errors.New("conflict")
	}
	store.records[digest] = bundle
	return nil
}
func (store *memoryObservationStore) Resolve(ctx context.Context, digest string) (storeport.ObservationBundle, error) {
	if ctx.Err() != nil {
		return storeport.ObservationBundle{}, ctx.Err()
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	bundle, found := store.records[digest]
	if !found {
		return storeport.ObservationBundle{}, errors.New("absent")
	}
	return bundle, nil
}
func (store *memoryObservationStore) snapshot() map[string]storeport.ObservationBundle {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make(map[string]storeport.ObservationBundle, len(store.records))
	for digest, bundle := range store.records {
		result[digest] = bundle
	}
	return result
}
func (store *memoryObservationStore) restore(records map[string]storeport.ObservationBundle) {
	store.mu.Lock()
	store.records = records
	store.mu.Unlock()
}
func (store *memoryObservationStore) delete(digest string) {
	store.mu.Lock()
	delete(store.records, digest)
	store.mu.Unlock()
}
func (store *memoryObservationStore) putRaw(bundle storeport.ObservationBundle) {
	store.mu.Lock()
	store.records[bundle.Observation.ObservationDigest] = bundle
	store.mu.Unlock()
}
func (store *memoryObservationStore) mutate(digest string, change func(*storeport.ObservationBundle)) {
	store.mu.Lock()
	bundle := store.records[digest]
	change(&bundle)
	store.records[digest] = bundle
	store.mu.Unlock()
}

type memoryProjection struct {
	mu    sync.Mutex
	index domainsecurity.ThreadRiskAuthorityIndexV1
}

type memoryCheckpointFloor struct {
	mu         sync.Mutex
	checkpoint *domainsecurity.MonotonicHeadCheckpointV1
	failure    error
}

func (floor *memoryCheckpointFloor) ProjectWitnessSelected(ctx context.Context, selected domainsecurity.MonotonicHeadCheckpointV1) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	floor.mu.Lock()
	defer floor.mu.Unlock()
	if floor.failure != nil {
		return floor.failure
	}
	if floor.checkpoint == nil {
		if selected.Generation != 0 {
			return monotonicheadport.ErrCheckpointFloorBootstrap
		}
		copy := selected
		floor.checkpoint = &copy
		return nil
	}
	if *floor.checkpoint == selected {
		return nil
	}
	if domainsecurity.ValidateMonotonicHeadCheckpointDirectSuccessorV1(*floor.checkpoint, selected) != nil {
		return monotonicheadport.ErrCheckpointFloorConflict
	}
	copy := selected
	floor.checkpoint = &copy
	return nil
}

func (projection *memoryProjection) ValidateWitnessEmpty(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	projection.mu.Lock()
	defer projection.mu.Unlock()
	if projection.index.Generation != 0 {
		return errors.New("non-empty projection")
	}
	return nil
}

func (projection *memoryProjection) ProjectWitnessSelected(ctx context.Context, index domainsecurity.ThreadRiskAuthorityIndexV1) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	projection.set(index)
	return nil
}
func (projection *memoryProjection) set(index domainsecurity.ThreadRiskAuthorityIndexV1) {
	projection.mu.Lock()
	projection.index = index
	projection.mu.Unlock()
}
func (projection *memoryProjection) current() domainsecurity.ThreadRiskAuthorityIndexV1 {
	projection.mu.Lock()
	defer projection.mu.Unlock()
	return projection.index
}

var _ io.Reader = (*counterReader)(nil)
