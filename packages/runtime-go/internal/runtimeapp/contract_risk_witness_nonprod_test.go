//go:build !analytix_prod

package runtimeapp

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	threadriskauthorityapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	monotonicheadport "analytix.local/runtime-go/internal/ports/monotonichead"
)

func TestContractThreadRiskAuthorityEnabledOnlyForExplicitTempMode(t *testing.T) {
	installation := newContractRiskTestInstallationAuthority()
	tests := []struct {
		name    string
		config  Config
		enabled bool
	}{
		{name: "missing durable temp root", config: Config{}},
		{name: "production root wins", config: Config{DurableTempDir: t.TempDir(), ProductionDurableRoot: t.TempDir()}},
		{name: "candidate root wins", config: Config{DurableTempDir: t.TempDir(), CandidateDurableRoot: t.TempDir()}},
		{name: "explicit contract temp root", config: Config{DurableTempDir: t.TempDir()}, enabled: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authority, enabled, err := newContractThreadRiskAuthority(test.config, installation)
			if err != nil {
				t.Fatal(err)
			}
			if enabled != test.enabled || (enabled && authority == nil) || (!enabled && authority != nil) {
				t.Fatalf("factory result authority=%T enabled=%v", authority, enabled)
			}
		})
	}

	if authority, enabled, err := newContractThreadRiskAuthority(
		Config{DurableTempDir: t.TempDir()}, nil,
	); err == nil || !enabled || authority != nil {
		t.Fatalf("enabled temp mode accepted missing installation authority: authority=%T enabled=%v err=%v", authority, enabled, err)
	}
}

func TestContractThreadRiskAuthorityExactGeneralCaseAndNoDowngrade(t *testing.T) {
	authority, enabled, err := newContractThreadRiskAuthority(
		Config{DurableTempDir: t.TempDir()}, newContractRiskTestInstallationAuthority(),
	)
	if err != nil || !enabled || authority == nil {
		t.Fatalf("factory failed: authority=%T enabled=%v err=%v", authority, enabled, err)
	}
	workspace := "/private/tmp/analytix-contract-risk"
	general, err := authority.ResolveOrRaise(context.Background(), contractRiskInput(
		"thread-contract", workspace, domainsecurity.RiskClassGeneral, 1,
	))
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := authority.ResolveOrRaise(context.Background(), contractRiskInput(
		"thread-contract", workspace, domainsecurity.RiskClassGeneral, 2,
	))
	if err != nil {
		t.Fatal(err)
	}
	if !general.HasIndex || !general.Found || general.Index.Generation != 1 ||
		general.Policy.RiskClass != domainsecurity.RiskClassGeneral ||
		repeated.Index.IndexDigest != general.Index.IndexDigest ||
		repeated.Policy.PolicyDigest != general.Policy.PolicyDigest ||
		repeated.Request.ChallengeNonce == general.Request.ChallengeNonce {
		t.Fatalf("repeated resolution was not an exact freshly witnessed head: first=%#v repeated=%#v", general, repeated)
	}

	caseHead, err := authority.ResolveOrRaise(context.Background(), contractRiskInput(
		"thread-contract", workspace, domainsecurity.RiskClassCase, 3,
	))
	if err != nil {
		t.Fatal(err)
	}
	if caseHead.Policy.RiskClass != domainsecurity.RiskClassCase || caseHead.Index.Generation != 2 ||
		caseHead.Policy.PredecessorPolicyDigest != general.Policy.PolicyDigest {
		t.Fatalf("general-to-case transition was not an exact successor: %#v", caseHead)
	}

	downgrade, err := authority.ResolveOrRaise(context.Background(), contractRiskInput(
		"thread-contract", workspace, domainsecurity.RiskClassGeneral, 4,
	))
	if err != nil {
		t.Fatal(err)
	}
	if downgrade.Policy.RiskClass != domainsecurity.RiskClassCase ||
		downgrade.Policy.PolicyDigest != caseHead.Policy.PolicyDigest ||
		downgrade.Index.IndexDigest != caseHead.Index.IndexDigest {
		t.Fatalf("case risk downgraded: %#v", downgrade)
	}
}

func TestContractMonotonicWitnessExactMutationAndCAS(t *testing.T) {
	installation := newContractRiskTestInstallationAuthority()
	witnessSeed := sha256.Sum256([]byte("analytix.contract-risk-witness-test/key/v1"))
	witnessPrivate := ed25519.NewKeyFromSeed(witnessSeed[:])
	installationID := domainsecurity.SHA256Hex([]byte("analytix.contract-risk-witness-test/installation/v1"))
	enrollmentID := domainsecurity.SHA256Hex([]byte("analytix.contract-risk-witness-test/enrollment/v1"))
	witness, err := newContractMonotonicWitness(
		installationID, enrollmentID, installation.KeyID(), installation.PublicKey(), witnessPrivate,
	)
	if err != nil {
		t.Fatal(err)
	}

	checkpoint := contractRiskObserveCheckpoint(t, witness, installation, installationID, enrollmentID, "initial")
	firstRequest := contractRiskAdvanceRequest(
		t, installation, installationID, enrollmentID, checkpoint,
		domainsecurity.SHA256Hex([]byte("state-1")), domainsecurity.SHA256Hex([]byte("mutation-1")),
	)
	firstReceipt, err := witness.Advance(context.Background(), firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := witness.Advance(context.Background(), firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, _ := domainsecurity.MonotonicHeadAdvanceReceiptV1Bytes(firstReceipt)
	replayedBytes, _ := domainsecurity.MonotonicHeadAdvanceReceiptV1Bytes(replayed)
	if !bytes.Equal(firstBytes, replayedBytes) {
		t.Fatal("identical mutation retry did not return the exact receipt")
	}

	conflictingRequest := contractRiskAdvanceRequest(
		t, installation, installationID, enrollmentID, checkpoint,
		domainsecurity.SHA256Hex([]byte("other-state")), firstRequest.MutationID,
	)
	if _, err := witness.Advance(context.Background(), conflictingRequest); !errors.Is(err, monotonicheadport.ErrMutationConflict) {
		t.Fatalf("mutation ID reuse with different bytes returned %v", err)
	}

	secondRequest := contractRiskAdvanceRequest(
		t, installation, installationID, enrollmentID, firstReceipt.Checkpoint,
		domainsecurity.SHA256Hex([]byte("state-2")), domainsecurity.SHA256Hex([]byte("mutation-2")),
	)
	secondReceipt, err := witness.Advance(context.Background(), secondRequest)
	if err != nil || secondReceipt.Checkpoint.Generation != 2 ||
		secondReceipt.Checkpoint.PreviousCheckpointDigest != firstReceipt.Checkpoint.CheckpointDigest {
		t.Fatalf("second exact CAS failed: receipt=%#v err=%v", secondReceipt, err)
	}

	staleRequest := contractRiskAdvanceRequest(
		t, installation, installationID, enrollmentID, checkpoint,
		domainsecurity.SHA256Hex([]byte("stale-state")), domainsecurity.SHA256Hex([]byte("mutation-stale")),
	)
	if _, err := witness.Advance(context.Background(), staleRequest); !errors.Is(err, monotonicheadport.ErrCASConflict) {
		t.Fatalf("stale compare-and-swap returned %v", err)
	}
}

func TestContractThreadRiskAuthorityConcurrentIndexChain(t *testing.T) {
	authority, enabled, err := newContractThreadRiskAuthority(
		Config{DurableTempDir: t.TempDir()}, newContractRiskTestInstallationAuthority(),
	)
	if err != nil || !enabled || authority == nil {
		t.Fatalf("factory failed: authority=%T enabled=%v err=%v", authority, enabled, err)
	}

	const workers = 24
	errCh := make(chan error, workers)
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		worker := worker
		wait.Add(1)
		go func() {
			defer wait.Done()
			threadID := fmt.Sprintf("thread-concurrent-%02d", worker)
			workspace := fmt.Sprintf("/private/tmp/analytix-contract-risk/%02d", worker)
			head, resolveErr := authority.ResolveOrRaise(context.Background(), contractRiskInput(
				threadID, workspace, domainsecurity.RiskClassGeneral, worker+1,
			))
			if resolveErr != nil {
				errCh <- resolveErr
				return
			}
			if !head.HasIndex || !head.Found || head.Policy.ThreadID != threadID {
				errCh <- errors.New("concurrent resolution returned an incomplete head")
			}
		}()
	}
	wait.Wait()
	close(errCh)
	for resolveErr := range errCh {
		t.Error(resolveErr)
	}
	if t.Failed() {
		return
	}
	current, err := authority.(*threadriskauthorityapp.Authority).Current(context.Background(), "thread-concurrent-00")
	if err != nil {
		t.Fatal(err)
	}
	if current.Index.Generation != workers || len(current.Index.Entries) != workers || !current.Found {
		t.Fatalf("concurrent mutations did not form one complete index chain: generation=%d entries=%d found=%v",
			current.Index.Generation, len(current.Index.Entries), current.Found)
	}
}

func contractRiskInput(threadID, workspace, risk string, tick int) threadriskauthorityapp.ResolveOrRaiseInput {
	origin := domainsecurity.RiskPolicyOriginGeneralWorkspace
	if risk == domainsecurity.RiskClassCase {
		origin = domainsecurity.RiskPolicyOriginLexicalGuard
	}
	return threadriskauthorityapp.ResolveOrRaiseInput{
		ThreadID:          threadID,
		WorkspaceRealPath: workspace,
		RequestedRisk:     risk,
		Origin:            origin,
		SignalsDigest:     domainsecurity.SHA256Hex([]byte("signals:" + threadID + ":" + risk)),
		IssuedAt:          time.Date(2026, 7, 12, 0, 0, tick, 0, time.UTC),
	}
}

func contractRiskObserveCheckpoint(
	t *testing.T,
	witness monotonicheadport.Witness,
	installation finalauthorityport.Authority,
	installationID string,
	enrollmentID string,
	label string,
) domainsecurity.MonotonicHeadCheckpointV1 {
	t.Helper()
	request, err := domainsecurity.NewMonotonicHeadObserveRequestV1(domainsecurity.MonotonicHeadObserveRequestInputV1{
		InstallationID:     installationID,
		EnrollmentID:       enrollmentID,
		Namespace:          domainsecurity.ThreadRiskAuthorityNamespaceV1,
		ChallengeNonce:     domainsecurity.SHA256Hex([]byte("challenge:" + label)),
		AuthorityKeyID:     installation.KeyID(),
		AuthorityPublicKey: installation.PublicKey(),
	}, func(message []byte) ([]byte, error) {
		return installation.Sign(context.Background(), message)
	})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := witness.Observe(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return observation.Checkpoint
}

func contractRiskAdvanceRequest(
	t *testing.T,
	installation finalauthorityport.Authority,
	installationID string,
	enrollmentID string,
	previous domainsecurity.MonotonicHeadCheckpointV1,
	nextState string,
	mutationID string,
) domainsecurity.MonotonicHeadAdvanceRequestV1 {
	t.Helper()
	request, err := domainsecurity.NewMonotonicHeadAdvanceRequestV1(domainsecurity.MonotonicHeadAdvanceRequestInputV1{
		InstallationID:           installationID,
		EnrollmentID:             enrollmentID,
		Namespace:                domainsecurity.ThreadRiskAuthorityNamespaceV1,
		ExpectedGeneration:       previous.Generation,
		ExpectedCheckpointDigest: previous.CheckpointDigest,
		ExpectedStateDigest:      previous.CurrentStateDigest,
		NextGeneration:           previous.Generation + 1,
		NextStateDigest:          nextState,
		ExpectedFenceNonce:       previous.FenceNonce,
		MutationID:               mutationID,
		AuthorityKeyID:           installation.KeyID(),
		AuthorityPublicKey:       installation.PublicKey(),
	}, func(message []byte) ([]byte, error) {
		return installation.Sign(context.Background(), message)
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

type contractRiskTestInstallationAuthority struct {
	private ed25519.PrivateKey
	public  ed25519.PublicKey
}

func newContractRiskTestInstallationAuthority() *contractRiskTestInstallationAuthority {
	seed := sha256.Sum256([]byte("analytix.contract-risk-test-installation-authority/v1"))
	private := ed25519.NewKeyFromSeed(seed[:])
	return &contractRiskTestInstallationAuthority{
		private: private,
		public:  append(ed25519.PublicKey(nil), private.Public().(ed25519.PublicKey)...),
	}
}

func (authority *contractRiskTestInstallationAuthority) KeyID() string {
	return domainsecurity.SHA256Hex(authority.public)
}

func (authority *contractRiskTestInstallationAuthority) PublicKey() []byte {
	return append([]byte(nil), authority.public...)
}

func (authority *contractRiskTestInstallationAuthority) Sign(ctx context.Context, message []byte) ([]byte, error) {
	if err := contractRiskContextError(ctx); err != nil {
		return nil, err
	}
	return ed25519.Sign(authority.private, message), nil
}

func (authority *contractRiskTestInstallationAuthority) VerifyTrusted(
	ctx context.Context,
	keyID string,
	publicKey []byte,
	message []byte,
	signature []byte,
) error {
	if err := contractRiskContextError(ctx); err != nil {
		return err
	}
	if keyID != authority.KeyID() || !bytes.Equal(publicKey, authority.public) || !ed25519.Verify(authority.public, message, signature) {
		return errors.New("untrusted contract test signature")
	}
	return nil
}

var _ finalauthorityport.Authority = (*contractRiskTestInstallationAuthority)(nil)
