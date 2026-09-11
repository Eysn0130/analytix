package authorityadvancefs

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	domainauthority "analytix.local/runtime-go/internal/domain/authorityadvance"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	storeport "analytix.local/runtime-go/internal/ports/authorityadvance"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

type journalFixtureV2 struct {
	installationID string
	enrollmentID   string
	mutationID     string
	authorityKeyID string
	authorityPub   ed25519.PublicKey
	authorityPriv  ed25519.PrivateKey
	witnessKeyID   string
	witnessPub     ed25519.PublicKey
	witnessPriv    ed25519.PrivateKey
	enrollment     domainsecurity.MonotonicHeadCheckpointV1
}

func TestAuthorityAdvanceStorePersistsExactIntentAndSettlement(t *testing.T) {
	fixture := newJournalFixtureV2(t, "exact")
	intent := fixture.intent(t, "policy-one")
	settlement := fixture.settlement(t, intent)
	store, err := newAuthorityAdvanceTestStore(t, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutIntentIfAbsent(context.Background(), intent); err != nil {
		t.Fatal(err)
	}
	if err := store.PutIntentIfAbsent(context.Background(), intent); err != nil {
		t.Fatalf("byte-identical intent replay rejected: %v", err)
	}
	storedIntent, err := store.ResolveIntent(context.Background(), intent.MutationID)
	if err != nil || !sameIntentBytesV2(storedIntent, intent) {
		t.Fatalf("intent readback mismatch: %#v err=%v", storedIntent, err)
	}
	if err := store.PutSettlementIfAbsent(context.Background(), settlement); err != nil {
		t.Fatal(err)
	}
	if err := store.PutSettlementIfAbsent(context.Background(), settlement); err != nil {
		t.Fatalf("byte-identical settlement replay rejected: %v", err)
	}
	storedSettlement, err := store.ResolveSettlement(context.Background(), intent.MutationID)
	if err != nil || !sameSettlementBytesV2(storedSettlement, settlement) {
		t.Fatalf("settlement readback mismatch: %#v err=%v", storedSettlement, err)
	}
}

func TestAuthorityAdvanceStoreRejectsMutationReuseWithDifferentBytes(t *testing.T) {
	fixture := newJournalFixtureV2(t, "conflict")
	first := fixture.intent(t, "policy-one")
	different := fixture.intent(t, "policy-two")
	store, err := newAuthorityAdvanceTestStore(t, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutIntentIfAbsent(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := store.PutIntentIfAbsent(context.Background(), different); !errors.Is(err, storeport.ErrConflict) {
		t.Fatalf("same mutation ID conflict classification = %v", err)
	}
	stored, err := store.ResolveIntent(context.Background(), first.MutationID)
	if err != nil || !sameIntentBytesV2(stored, first) {
		t.Fatal("conflicting write replaced the original intent")
	}
}

func TestAuthorityAdvanceStoreRejectsCorruptedMutationRecord(t *testing.T) {
	fixture := newJournalFixtureV2(t, "corrupt")
	intent := fixture.intent(t, "policy")
	root := t.TempDir()
	store, err := newAuthorityAdvanceTestStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutIntentIfAbsent(context.Background(), intent); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "v2", "intents", intent.MutationID[:2], intent.MutationID+".json")
	if err := os.WriteFile(path, []byte(`{"attacker":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveIntent(context.Background(), intent.MutationID); !errors.Is(err, storeport.ErrCorrupt) {
		t.Fatalf("corrupted intent record classification = %v", err)
	}
}

func TestAuthorityAdvanceStoreConcurrentExactReplayIsIdempotent(t *testing.T) {
	fixture := newJournalFixtureV2(t, "concurrent")
	intent := fixture.intent(t, "policy")
	root := t.TempDir()
	first, err := newAuthorityAdvanceTestStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newAuthorityAdvanceTestStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	errorsFound := make(chan error, 16)
	for index := 0; index < 16; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			store := first
			if index%2 == 1 {
				store = second
			}
			errorsFound <- store.PutIntentIfAbsent(context.Background(), intent)
		}(index)
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatalf("concurrent exact intent replay: %v", err)
		}
	}
}

func TestAuthorityAdvanceStoreConcurrentMutationConflictHasOneImmutableWinner(t *testing.T) {
	fixture := newJournalFixtureV2(t, "cross-store-conflict")
	left := fixture.intent(t, "left-policy")
	right := fixture.intent(t, "right-policy")
	root := t.TempDir()
	leftStore, err := newAuthorityAdvanceTestStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	rightStore, err := newAuthorityAdvanceTestStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		results <- leftStore.PutIntentIfAbsent(context.Background(), left)
	}()
	go func() {
		<-start
		results <- rightStore.PutIntentIfAbsent(context.Background(), right)
	}()
	close(start)
	firstErr := <-results
	secondErr := <-results
	if (firstErr == nil) == (secondErr == nil) {
		t.Fatalf("different bytes under one mutation must have exactly one winner: left/right errors = %v / %v", firstErr, secondErr)
	}
	loser := firstErr
	if loser == nil {
		loser = secondErr
	}
	if !errors.Is(loser, storeport.ErrConflict) {
		t.Fatalf("concurrent mutation loser classification = %v", loser)
	}
	stored, err := leftStore.ResolveIntent(context.Background(), left.MutationID)
	if err != nil {
		t.Fatal(err)
	}
	if !sameIntentBytesV2(stored, left) && !sameIntentBytesV2(stored, right) {
		t.Fatal("concurrent no-replace winner bytes were altered")
	}
}

func TestAuthorityAdvancePreparedRecoveryAndInventoryBindExactJournalGraph(t *testing.T) {
	fixture := newJournalFixtureV2(t, "prepared-recovery")
	intent := fixture.intent(t, "policy")
	settlement := fixture.settlement(t, intent)
	root := t.TempDir()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutIntentIfAbsent(context.Background(), intent); err != nil {
		t.Fatal(err)
	}
	if err := store.PutSettlementIfAbsent(context.Background(), settlement); err != nil {
		t.Fatal(err)
	}
	if present, err := store.HasRecords(context.Background()); err != nil || !present {
		t.Fatalf("journal inventory presence = %v, %v", present, err)
	}
	intentCount, settlementCount := 0, 0
	if err := store.VisitIntents(context.Background(), func(actual domainauthority.MonotonicAdvanceIntentV2) error {
		intentCount++
		if !sameIntentBytesV2(actual, intent) {
			return errors.New("visited a different intent")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.VisitSettlements(context.Background(), func(actual domainauthority.MonotonicAdvanceSettlementV2) error {
		settlementCount++
		if !sameSettlementBytesV2(actual, settlement) {
			return errors.New("visited a different settlement")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if intentCount != 1 || settlementCount != 1 {
		t.Fatalf("journal inventory counts = %d/%d", intentCount, settlementCount)
	}
	prepared, err := PrepareRecoveryV1(context.Background(), root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.ValidateSemantics(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Revalidate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if plans := prepared.SecurePrivateCASRecoveryPlansV2(); len(plans) != 2 {
		t.Fatalf("prepared journal roots = %d, want 2", len(plans))
	}
}

func TestAuthorityAdvancePreparedRecoveryRejectsOrphanSettlement(t *testing.T) {
	fixture := newJournalFixtureV2(t, "orphan-settlement")
	intent := fixture.intent(t, "policy")
	settlement := fixture.settlement(t, intent)
	root := t.TempDir()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutSettlementIfAbsent(context.Background(), settlement); err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareRecoveryV1(context.Background(), root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.ValidateSemantics(context.Background()); err == nil {
		t.Fatal("orphan authority advance settlement passed prepared recovery")
	}
}

func newAuthorityAdvanceTestStore(t *testing.T, root string) (*Store, error) {
	t.Helper()
	mutation, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		return nil, err
	}
	return NewStore(root, mutation)
}

func newJournalFixtureV2(t *testing.T, label string) journalFixtureV2 {
	t.Helper()
	authoritySeed := sha256.Sum256([]byte("journal-authority:" + label))
	authorityPrivate := ed25519.NewKeyFromSeed(authoritySeed[:])
	authorityPublic := authorityPrivate.Public().(ed25519.PublicKey)
	witnessSeed := sha256.Sum256([]byte("journal-witness:" + label))
	witnessPrivate := ed25519.NewKeyFromSeed(witnessSeed[:])
	witnessPublic := witnessPrivate.Public().(ed25519.PublicKey)
	fixture := journalFixtureV2{
		installationID: domainsecurity.SHA256Hex([]byte("journal-installation:" + label)),
		enrollmentID:   domainsecurity.SHA256Hex([]byte("journal-enrollment:" + label)),
		mutationID:     domainsecurity.SHA256Hex([]byte("journal-mutation:" + label)),
		authorityKeyID: domainsecurity.SHA256Hex(authorityPublic),
		authorityPub:   authorityPublic,
		authorityPriv:  authorityPrivate,
		witnessKeyID:   domainsecurity.SHA256Hex(witnessPublic),
		witnessPub:     witnessPublic,
		witnessPriv:    witnessPrivate,
	}
	enrollment, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: fixture.installationID, EnrollmentID: fixture.enrollmentID,
		Namespace:          domainsecurity.ThreadRiskAuthorityNamespaceV1,
		CurrentStateDigest: domainsecurity.SHA256Hex([]byte("journal-enrollment-state:" + label)),
		FenceNonce:         domainsecurity.SHA256Hex([]byte("journal-enrollment-fence:" + label)),
		WitnessKeyID:       fixture.witnessKeyID, WitnessPublicKey: fixture.witnessPub,
	}, fixture.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	fixture.enrollment = enrollment
	return fixture
}

func (fixture journalFixtureV2) intent(t *testing.T, policyLabel string) domainauthority.MonotonicAdvanceIntentV2 {
	t.Helper()
	index, err := domainsecurity.NewThreadRiskAuthorityIndexV1(domainsecurity.ThreadRiskAuthorityIndexInputV1{
		InstallationID: fixture.installationID, EnrollmentID: fixture.enrollmentID,
		Namespace: domainsecurity.ThreadRiskAuthorityNamespaceV1, Generation: 1,
		Entries: []domainsecurity.ThreadRiskAuthorityEntryV1{{
			ThreadID: "thread-journal", WorkspaceRealPath: "/workspace/journal",
			RiskClass:           domainsecurity.RiskClassGeneral,
			CurrentPolicyDigest: domainsecurity.SHA256Hex([]byte(policyLabel)),
		}},
		MutationID:     fixture.mutationID,
		AuthorityKeyID: fixture.authorityKeyID, AuthorityPublicKey: fixture.authorityPub,
	}, fixture.authoritySign)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := domainauthority.NewThreadRiskGenesisTransitionBindingV2(fixture.enrollment, index)
	if err != nil {
		t.Fatal(err)
	}
	request, err := domainsecurity.NewMonotonicHeadAdvanceRequestV1(domainsecurity.MonotonicHeadAdvanceRequestInputV1{
		InstallationID: fixture.installationID, EnrollmentID: fixture.enrollmentID,
		Namespace:          domainsecurity.ThreadRiskAuthorityNamespaceV1,
		ExpectedGeneration: fixture.enrollment.Generation, ExpectedCheckpointDigest: fixture.enrollment.CheckpointDigest,
		ExpectedStateDigest: fixture.enrollment.CurrentStateDigest, NextGeneration: index.Generation,
		NextStateDigest: index.IndexDigest, ExpectedFenceNonce: fixture.enrollment.FenceNonce,
		MutationID: fixture.mutationID, AuthorityKeyID: fixture.authorityKeyID, AuthorityPublicKey: fixture.authorityPub,
	}, fixture.authoritySign)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := domainauthority.NewMonotonicAdvanceIntentV2(domainauthority.MonotonicAdvanceIntentInputV2{
		Root: domainauthority.AdvanceRootThreadRiskV2, PreviousCheckpoint: fixture.enrollment,
		AdvanceRequest: request, Transition: binding,
		AuthorityKeyID: fixture.authorityKeyID, AuthorityPublicKey: fixture.authorityPub,
	}, fixture.authoritySign)
	if err != nil {
		t.Fatal(err)
	}
	return intent
}

func (fixture journalFixtureV2) settlement(
	t *testing.T,
	intent domainauthority.MonotonicAdvanceIntentV2,
) domainauthority.MonotonicAdvanceSettlementV2 {
	t.Helper()
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: fixture.installationID, EnrollmentID: fixture.enrollmentID,
		Namespace: domainsecurity.ThreadRiskAuthorityNamespaceV1, Generation: 1,
		CurrentStateDigest:       intent.AdvanceRequest.NextStateDigest,
		PreviousStateDigest:      fixture.enrollment.CurrentStateDigest,
		PreviousCheckpointDigest: fixture.enrollment.CheckpointDigest,
		FenceNonce:               domainsecurity.SHA256Hex([]byte("journal-committed-fence:" + intent.MutationID)),
		MutationID:               intent.MutationID, WitnessKeyID: fixture.witnessKeyID, WitnessPublicKey: fixture.witnessPub,
	}, fixture.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domainsecurity.NewMonotonicHeadAdvanceReceiptV1(intent.AdvanceRequest, checkpoint, fixture.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	settlement, err := domainauthority.NewCommittedMonotonicAdvanceSettlementV2(intent, receipt, fixture.authoritySign)
	if err != nil {
		t.Fatal(err)
	}
	return settlement
}

func (fixture journalFixtureV2) authoritySign(message []byte) ([]byte, error) {
	return ed25519.Sign(fixture.authorityPriv, message), nil
}

func (fixture journalFixtureV2) witnessSign(message []byte) ([]byte, error) {
	return ed25519.Sign(fixture.witnessPriv, message), nil
}

func sameIntentBytesV2(left, right domainauthority.MonotonicAdvanceIntentV2) bool {
	leftBody, leftErr := domainauthority.MonotonicAdvanceIntentV2Bytes(left)
	rightBody, rightErr := domainauthority.MonotonicAdvanceIntentV2Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}

func sameSettlementBytesV2(left, right domainauthority.MonotonicAdvanceSettlementV2) bool {
	leftBody, leftErr := domainauthority.MonotonicAdvanceSettlementV2Bytes(left)
	rightBody, rightErr := domainauthority.MonotonicAdvanceSettlementV2Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}
