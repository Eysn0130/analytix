package authorityadvance

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"reflect"
	"sync"
	"testing"

	domainauthority "analytix.local/runtime-go/internal/domain/authorityadvance"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	journalport "analytix.local/runtime-go/internal/ports/authorityadvance"
	monotonicheadport "analytix.local/runtime-go/internal/ports/monotonichead"
)

type coordinatorOperationLogV1 struct {
	mu    sync.Mutex
	items []string
}

func (log *coordinatorOperationLogV1) add(item string) {
	log.mu.Lock()
	defer log.mu.Unlock()
	log.items = append(log.items, item)
}

func (log *coordinatorOperationLogV1) snapshot() []string {
	log.mu.Lock()
	defer log.mu.Unlock()
	return append([]string(nil), log.items...)
}

type coordinatorAuthorityV1 struct {
	private ed25519.PrivateKey
	public  ed25519.PublicKey
	keyID   string
}

func (authority coordinatorAuthorityV1) KeyID() string { return authority.keyID }
func (authority coordinatorAuthorityV1) PublicKey() []byte {
	return append([]byte(nil), authority.public...)
}
func (authority coordinatorAuthorityV1) Sign(ctx context.Context, message []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ed25519.Sign(authority.private, message), nil
}
func (authority coordinatorAuthorityV1) VerifyTrusted(
	ctx context.Context,
	keyID string,
	publicKey, message, signature []byte,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if keyID != authority.keyID || !bytes.Equal(publicKey, authority.public) ||
		!ed25519.Verify(authority.public, message, signature) {
		return errors.New("untrusted authority signature")
	}
	return nil
}

type coordinatorWitnessV1 struct {
	log             *coordinatorOperationLogV1
	mu              sync.Mutex
	calls           int
	requests        []domainsecurity.MonotonicHeadAdvanceRequestV1
	receipt         domainsecurity.MonotonicHeadAdvanceReceiptV1
	errors          []error
	resolveCalls    int
	resolveRequests []domainsecurity.MonotonicHeadMutationResolveRequestV1
	resolveErrors   []error
	resolve         func(domainsecurity.MonotonicHeadMutationResolveRequestV1) (domainsecurity.MonotonicHeadMutationResolutionV1, error)
	entered         chan struct{}
	release         chan struct{}
}

func (witness *coordinatorWitnessV1) Observe(
	context.Context,
	domainsecurity.MonotonicHeadObserveRequestV1,
) (domainsecurity.MonotonicHeadObservationV1, error) {
	return domainsecurity.MonotonicHeadObservationV1{}, errors.New("observe is outside the commit coordinator")
}

func (witness *coordinatorWitnessV1) Advance(
	ctx context.Context,
	request domainsecurity.MonotonicHeadAdvanceRequestV1,
) (domainsecurity.MonotonicHeadAdvanceReceiptV1, error) {
	witness.log.add("witness.advance")
	if witness.entered != nil {
		select {
		case witness.entered <- struct{}{}:
		default:
		}
	}
	if witness.release != nil {
		select {
		case <-witness.release:
		case <-ctx.Done():
			return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, ctx.Err()
		}
	}
	witness.mu.Lock()
	defer witness.mu.Unlock()
	witness.calls++
	witness.requests = append(witness.requests, request)
	if len(witness.errors) != 0 {
		err := witness.errors[0]
		witness.errors = witness.errors[1:]
		if err != nil {
			return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, err
		}
	}
	return witness.receipt, nil
}

func (witness *coordinatorWitnessV1) ResolveMutation(
	_ context.Context,
	request domainsecurity.MonotonicHeadMutationResolveRequestV1,
) (domainsecurity.MonotonicHeadMutationResolutionV1, error) {
	witness.log.add("witness.resolve")
	witness.mu.Lock()
	defer witness.mu.Unlock()
	witness.resolveCalls++
	witness.resolveRequests = append(witness.resolveRequests, request)
	if len(witness.resolveErrors) != 0 {
		err := witness.resolveErrors[0]
		witness.resolveErrors = witness.resolveErrors[1:]
		if err != nil {
			return domainsecurity.MonotonicHeadMutationResolutionV1{}, err
		}
	}
	if witness.resolve == nil {
		return domainsecurity.MonotonicHeadMutationResolutionV1{}, monotonicheadport.ErrUnavailable
	}
	return witness.resolve(request)
}

type coordinatorIntentStoreV2 struct {
	log        *coordinatorOperationLogV1
	records    map[string]domainauthority.MonotonicAdvanceIntentV2
	putErr     error
	resolveErr error
}

func (store *coordinatorIntentStoreV2) PutIntentIfAbsent(
	_ context.Context,
	intent domainauthority.MonotonicAdvanceIntentV2,
) error {
	store.log.add("intent.put")
	if store.putErr != nil {
		return store.putErr
	}
	if existing, ok := store.records[intent.MutationID]; ok {
		if !equalIntentV2(existing, intent) {
			return errors.New("conflicting intent")
		}
		return nil
	}
	store.records[intent.MutationID] = intent
	return nil
}

func (store *coordinatorIntentStoreV2) ResolveIntent(
	_ context.Context,
	mutationID string,
) (domainauthority.MonotonicAdvanceIntentV2, error) {
	store.log.add("intent.resolve")
	if store.resolveErr != nil {
		return domainauthority.MonotonicAdvanceIntentV2{}, store.resolveErr
	}
	record, ok := store.records[mutationID]
	if !ok {
		return domainauthority.MonotonicAdvanceIntentV2{}, journalport.ErrNotFound
	}
	return record, nil
}

type coordinatorSettlementStoreV2 struct {
	log     *coordinatorOperationLogV1
	records map[string]domainauthority.MonotonicAdvanceSettlementV2
	putErr  error
}

func (store *coordinatorSettlementStoreV2) PutSettlementIfAbsent(
	_ context.Context,
	settlement domainauthority.MonotonicAdvanceSettlementV2,
) error {
	store.log.add("settlement.put")
	if store.putErr != nil {
		return store.putErr
	}
	if existing, ok := store.records[settlement.MutationID]; ok {
		if !equalSettlementV2(existing, settlement) {
			return errors.New("conflicting settlement")
		}
		return nil
	}
	store.records[settlement.MutationID] = settlement
	return nil
}

func (store *coordinatorSettlementStoreV2) ResolveSettlement(
	_ context.Context,
	mutationID string,
) (domainauthority.MonotonicAdvanceSettlementV2, error) {
	store.log.add("settlement.resolve")
	record, ok := store.records[mutationID]
	if !ok {
		return domainauthority.MonotonicAdvanceSettlementV2{}, journalport.ErrNotFound
	}
	return record, nil
}

type coordinatorRiskStoreV1 struct {
	log     *coordinatorOperationLogV1
	records map[string]domainsecurity.ThreadRiskAuthorityIndexV1
}

type coordinatorEvidenceStoreV1 struct {
	log     *coordinatorOperationLogV1
	records map[string]domainevidence.EvidenceAuthorityBundleV1
}

func (store *coordinatorEvidenceStoreV1) PutIfAbsent(
	context.Context,
	domainevidence.EvidenceAuthorityBundleV1,
) error {
	return errors.New("coordinator candidate store is read-only")
}

func (store *coordinatorEvidenceStoreV1) Resolve(
	_ context.Context,
	digest string,
) (domainevidence.EvidenceAuthorityBundleV1, error) {
	store.log.add("candidate.resolve")
	record, ok := store.records[digest]
	if !ok {
		return domainevidence.EvidenceAuthorityBundleV1{}, errors.New("candidate not found")
	}
	return record, nil
}

func (store *coordinatorRiskStoreV1) PutIfAbsent(
	context.Context,
	domainsecurity.ThreadRiskAuthorityIndexV1,
) error {
	return errors.New("coordinator candidate store is read-only")
}

func (store *coordinatorRiskStoreV1) Resolve(
	_ context.Context,
	digest string,
) (domainsecurity.ThreadRiskAuthorityIndexV1, error) {
	store.log.add("candidate.resolve")
	record, ok := store.records[digest]
	if !ok {
		return domainsecurity.ThreadRiskAuthorityIndexV1{}, errors.New("candidate not found")
	}
	return record, nil
}

type coordinatorFixtureV1 struct {
	coordinator  *Coordinator
	config       Config
	intent       domainauthority.MonotonicAdvanceIntentV2
	index        domainsecurity.ThreadRiskAuthorityIndexV1
	witness      *coordinatorWitnessV1
	intents      *coordinatorIntentStoreV2
	settlements  *coordinatorSettlementStoreV2
	riskIndexes  *coordinatorRiskStoreV1
	log          *coordinatorOperationLogV1
	authority    coordinatorAuthorityV1
	witnessKey   ed25519.PublicKey
	witnessPriv  ed25519.PrivateKey
	enrollmentID string
}

func TestIntentMustPersistBeforeWitnessDispatch(t *testing.T) {
	fixture := newCoordinatorFixtureV1(t, "ordered")
	result, err := fixture.coordinator.CommitExact(context.Background(), fixture.intent)
	if err != nil {
		t.Fatal(err)
	}
	if result.Replayed || result.Settlement.Kind != domainauthority.MonotonicAdvanceSettlementCommittedV2 {
		t.Fatalf("unexpected commit result: %#v", result)
	}
	want := []string{
		"candidate.resolve", "intent.resolve", "settlement.resolve", "intent.put", "intent.resolve", "settlement.resolve",
		"witness.advance", "settlement.put", "settlement.resolve",
	}
	if got := fixture.log.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commit order = %v, want %v", got, want)
	}
}

func TestCoordinatorAdmissionWaitHonorsContextCancellation(t *testing.T) {
	fixture := newCoordinatorFixtureV1(t, "admission-cancel")
	fixture.witness.entered = make(chan struct{}, 1)
	fixture.witness.release = make(chan struct{})
	firstResult := make(chan error, 1)
	go func() {
		_, err := fixture.coordinator.CommitExact(context.Background(), fixture.intent)
		firstResult <- err
	}()
	<-fixture.witness.entered

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fixture.coordinator.CommitExact(canceled, fixture.intent); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled admission wait returned %v", err)
	}
	close(fixture.witness.release)
	if err := <-firstResult; err != nil {
		t.Fatalf("admitted commit failed after release: %v", err)
	}
}

func TestCanceledRecoverNeverAcquiresIdleAdmission(t *testing.T) {
	fixture := newCoordinatorFixtureV1(t, "recover-canceled-idle")
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fixture.coordinator.RecoverExact(canceled, fixture.intent.MutationID); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled idle recovery returned %v", err)
	}
	if got := fixture.log.snapshot(); len(got) != 0 {
		t.Fatalf("canceled idle recovery touched dependencies: %v", got)
	}
}

func TestOrphanSettlementCannotReconstructDeletedIntent(t *testing.T) {
	fixture := newCoordinatorFixtureV1(t, "orphan-settlement")
	settlement, err := domainauthority.NewCommittedMonotonicAdvanceSettlementV2(
		fixture.intent,
		fixture.witness.receipt,
		func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.authority.private, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.settlements.records[fixture.intent.MutationID] = settlement
	if _, err := fixture.coordinator.CommitExact(context.Background(), fixture.intent); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("orphan settlement classification = %v", err)
	}
	if len(fixture.intents.records) != 0 || fixture.witness.calls != 0 {
		t.Fatal("orphan settlement reconstructed a deleted intent or dispatched witness")
	}
}

func TestGenesisEvidenceAuthorityAdvanceUsesSameDurableCoordinator(t *testing.T) {
	base := newCoordinatorFixtureV1(t, "evidence")
	log := &coordinatorOperationLogV1{}
	installationID := base.intent.InstallationID
	enrollmentID := base.enrollmentID
	witnessKeyID := domainsecurity.SHA256Hex(base.witnessKey)
	enrollment, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID,
		Namespace:          domainsecurity.EvidenceRegistryAuthorityNamespaceV1,
		CurrentStateDigest: domainsecurity.SHA256Hex([]byte("evidence-enrollment-state")),
		FenceNonce:         domainsecurity.SHA256Hex([]byte("evidence-enrollment-fence")),
		WitnessKeyID:       witnessKeyID, WitnessPublicKey: base.witnessKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(base.witnessPriv, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	mutationID := domainsecurity.SHA256Hex([]byte("evidence-genesis-mutation"))
	bundle, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Generation: 1, MutationID: mutationID,
		DatasetSnapshotIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
		EvidenceRegistryIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
		PublicationIndexDigest:      domainpublication.PublicationIndexGenesisDigestV1(),
		AuthorityKeyID:              base.authority.keyID, AuthorityPublicKey: base.authority.public,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(base.authority.private, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	binding, err := domainauthority.NewEvidenceAuthorityGenesisTransitionBindingV2(enrollment, bundle)
	if err != nil {
		t.Fatal(err)
	}
	request, err := domainsecurity.NewMonotonicHeadAdvanceRequestV1(domainsecurity.MonotonicHeadAdvanceRequestInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID,
		Namespace:          domainsecurity.EvidenceRegistryAuthorityNamespaceV1,
		ExpectedGeneration: 0, ExpectedCheckpointDigest: enrollment.CheckpointDigest,
		ExpectedStateDigest: enrollment.CurrentStateDigest, NextGeneration: 1, NextStateDigest: bundle.RecordDigest,
		ExpectedFenceNonce: enrollment.FenceNonce, MutationID: mutationID,
		AuthorityKeyID: base.authority.keyID, AuthorityPublicKey: base.authority.public,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(base.authority.private, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	intent, err := domainauthority.NewMonotonicAdvanceIntentV2(domainauthority.MonotonicAdvanceIntentInputV2{
		Root: domainauthority.AdvanceRootEvidenceGenesisV2, PreviousCheckpoint: enrollment,
		AdvanceRequest: request, Transition: binding,
		AuthorityKeyID: base.authority.keyID, AuthorityPublicKey: base.authority.public,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(base.authority.private, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID,
		Namespace: domainsecurity.EvidenceRegistryAuthorityNamespaceV1, Generation: 1,
		CurrentStateDigest: bundle.RecordDigest, PreviousStateDigest: enrollment.CurrentStateDigest,
		PreviousCheckpointDigest: enrollment.CheckpointDigest,
		FenceNonce:               domainsecurity.SHA256Hex([]byte("evidence-committed-fence")), MutationID: mutationID,
		WitnessKeyID: witnessKeyID, WitnessPublicKey: base.witnessKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(base.witnessPriv, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domainsecurity.NewMonotonicHeadAdvanceReceiptV1(
		request, checkpoint, func(message []byte) ([]byte, error) { return ed25519.Sign(base.witnessPriv, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	witness := &coordinatorWitnessV1{log: log, receipt: receipt}
	intents := &coordinatorIntentStoreV2{log: log, records: make(map[string]domainauthority.MonotonicAdvanceIntentV2)}
	settlements := &coordinatorSettlementStoreV2{log: log, records: make(map[string]domainauthority.MonotonicAdvanceSettlementV2)}
	bundles := &coordinatorEvidenceStoreV1{
		log: log, records: map[string]domainevidence.EvidenceAuthorityBundleV1{bundle.RecordDigest: bundle},
	}
	coordinator, err := New(Config{
		InstallationID: installationID, EnrollmentID: enrollmentID,
		Namespace: domainsecurity.EvidenceRegistryAuthorityNamespaceV1,
		Authority: base.authority, WitnessKeyID: witnessKeyID, WitnessPublicKey: base.witnessKey,
		Witness: witness, Intents: intents, Settlements: settlements, EvidenceBundles: bundles,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.CommitExact(context.Background(), intent)
	if err != nil || result.Settlement.Kind != domainauthority.MonotonicAdvanceSettlementCommittedV2 {
		t.Fatalf("evidence genesis commit = %#v err=%v", result, err)
	}
}

func TestCandidateMustExistBeforeAdvanceIntent(t *testing.T) {
	fixture := newCoordinatorFixtureV1(t, "candidate-missing")
	delete(fixture.riskIndexes.records, fixture.index.IndexDigest)
	if _, err := fixture.coordinator.CommitExact(context.Background(), fixture.intent); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("missing candidate classification = %v", err)
	}
	if fixture.witness.calls != 0 || len(fixture.intents.records) != 0 {
		t.Fatal("missing candidate persisted intent or dispatched witness")
	}
}

func TestIntentPersistenceFailurePreventsWitnessDispatch(t *testing.T) {
	fixture := newCoordinatorFixtureV1(t, "intent-failure")
	fixture.intents.putErr = errors.New("durability failure")
	if _, err := fixture.coordinator.CommitExact(context.Background(), fixture.intent); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("intent persistence classification = %v", err)
	}
	if fixture.witness.calls != 0 {
		t.Fatal("witness dispatched before durable intent")
	}
}

func TestIntentJournalConflictIsIntegrityNotAvailability(t *testing.T) {
	fixture := newCoordinatorFixtureV1(t, "intent-conflict-classification")
	fixture.intents.putErr = journalport.ErrConflict
	_, err := fixture.coordinator.CommitExact(context.Background(), fixture.intent)
	if !errors.Is(err, ErrIntegrity) || errors.Is(err, ErrUnavailable) || fixture.witness.calls != 0 {
		t.Fatalf("intent journal conflict classification = %v calls=%d", err, fixture.witness.calls)
	}
}

func TestCandidateIntegritySurvivesIntentJournalUnavailability(t *testing.T) {
	fixture := newCoordinatorFixtureV1(t, "candidate-integrity-and-journal-unavailable")
	fixture.riskIndexes.records[fixture.index.IndexDigest] = domainsecurity.ThreadRiskAuthorityIndexV1{}
	fixture.intents.resolveErr = journalport.ErrUnavailable
	_, err := fixture.coordinator.CommitExact(context.Background(), fixture.intent)
	if !errors.Is(err, ErrIntegrity) || !errors.Is(err, ErrUnavailable) || fixture.witness.calls != 0 {
		t.Fatalf("combined candidate/journal classification = %v calls=%d", err, fixture.witness.calls)
	}
}

func TestIndeterminateAdvanceLeavesOnlyExactIntent(t *testing.T) {
	fixture := newCoordinatorFixtureV1(t, "indeterminate")
	fixture.witness.errors = []error{monotonicheadport.ErrIndeterminate}
	if _, err := fixture.coordinator.CommitExact(context.Background(), fixture.intent); !errors.Is(err, ErrUnresolved) {
		t.Fatalf("indeterminate classification = %v", err)
	}
	if len(fixture.intents.records) != 1 || len(fixture.settlements.records) != 0 || fixture.witness.calls != 1 {
		t.Fatal("indeterminate advance did not retain exactly one intent and zero settlements")
	}
	stored := fixture.intents.records[fixture.intent.MutationID]
	if !equalIntentV2(stored, fixture.intent) {
		t.Fatal("indeterminate intent changed")
	}
}

func TestRecoverMissingDurableCandidateIsIntegrityIncident(t *testing.T) {
	fixture := newCoordinatorFixtureV1(t, "recover-missing-candidate")
	fixture.witness.errors = []error{monotonicheadport.ErrIndeterminate}
	if _, err := fixture.coordinator.CommitExact(context.Background(), fixture.intent); !errors.Is(err, ErrUnresolved) {
		t.Fatal(err)
	}
	delete(fixture.riskIndexes.records, fixture.index.IndexDigest)
	_, err := fixture.coordinator.RecoverExact(context.Background(), fixture.intent.MutationID)
	if !errors.Is(err, ErrUnresolved) || !errors.Is(err, ErrIntegrity) || errors.Is(err, ErrUnavailable) {
		t.Fatalf("missing durable candidate classification = %v", err)
	}
}

func TestCommitRetryMissingDurableCandidateIsIntegrityIncident(t *testing.T) {
	fixture := newCoordinatorFixtureV1(t, "commit-retry-missing-candidate")
	fixture.witness.errors = []error{monotonicheadport.ErrIndeterminate}
	if _, err := fixture.coordinator.CommitExact(context.Background(), fixture.intent); !errors.Is(err, ErrUnresolved) {
		t.Fatal(err)
	}
	delete(fixture.riskIndexes.records, fixture.index.IndexDigest)
	_, err := fixture.coordinator.CommitExact(context.Background(), fixture.intent)
	if !errors.Is(err, ErrUnresolved) || !errors.Is(err, ErrIntegrity) || errors.Is(err, ErrUnavailable) {
		t.Fatalf("missing durable candidate commit retry classification = %v", err)
	}
}

func TestInvalidSuccessReceiptRemainsIndeterminate(t *testing.T) {
	fixture := newCoordinatorFixtureV1(t, "invalid-success-receipt")
	fixture.witness.receipt = domainsecurity.MonotonicHeadAdvanceReceiptV1{}
	if _, err := fixture.coordinator.CommitExact(context.Background(), fixture.intent); !errors.Is(err, ErrUnresolved) || !errors.Is(err, monotonicheadport.ErrIndeterminate) {
		t.Fatalf("invalid success receipt classification = %v", err)
	}
	if len(fixture.intents.records) != 1 || len(fixture.settlements.records) != 0 {
		t.Fatal("invalid success receipt produced a terminal settlement")
	}
}

func TestJoinedWitnessIntegrityErrorsPreserveEveryClassification(t *testing.T) {
	for _, witnessErr := range []error{
		errors.Join(monotonicheadport.ErrIndeterminate, monotonicheadport.ErrInvalidReceipt),
		errors.Join(monotonicheadport.ErrIndeterminate, monotonicheadport.ErrMutationConflict),
		errors.Join(monotonicheadport.ErrIndeterminate, monotonicheadport.ErrEquivocation),
	} {
		fixture := newCoordinatorFixtureV1(t, domainsecurity.SHA256Hex([]byte(witnessErr.Error())))
		fixture.witness.errors = []error{witnessErr}
		_, err := fixture.coordinator.CommitExact(context.Background(), fixture.intent)
		if !errors.Is(err, ErrUnresolved) || !errors.Is(err, ErrIntegrity) ||
			!errors.Is(err, monotonicheadport.ErrIndeterminate) {
			t.Fatalf("joined witness integrity classification = %v", err)
		}
	}
}

func TestRecoverExactQueriesMutationWithoutBlindAdvanceReplay(t *testing.T) {
	fixture := newCoordinatorFixtureV1(t, "recover")
	fixture.witness.errors = []error{monotonicheadport.ErrIndeterminate}
	if _, err := fixture.coordinator.CommitExact(context.Background(), fixture.intent); !errors.Is(err, ErrUnresolved) {
		t.Fatalf("initial indeterminate classification = %v", err)
	}
	recovered, err := fixture.coordinator.RecoverExact(context.Background(), fixture.intent.MutationID)
	if err != nil || !recovered.Replayed || recovered.Settlement.Kind != domainauthority.MonotonicAdvanceSettlementCommittedV2 {
		t.Fatalf("exact committed mutation was not recovered: result=%#v err=%v", recovered, err)
	}
	if fixture.witness.calls != 1 || len(fixture.witness.requests) != 1 || fixture.witness.resolveCalls != 1 || len(fixture.witness.resolveRequests) != 1 {
		t.Fatal("unresolved recovery blindly replayed the witness request")
	}
	replayed, err := fixture.coordinator.RecoverExact(context.Background(), fixture.intent.MutationID)
	if err != nil || !replayed.Replayed || fixture.witness.calls != 1 || fixture.witness.resolveCalls != 1 {
		t.Fatalf("durable settlement replay dispatched witness: result=%#v err=%v", replayed, err)
	}
	settlementBody, _ := domainauthority.MonotonicAdvanceSettlementV2Bytes(recovered.Settlement)
	replayedBody, _ := domainauthority.MonotonicAdvanceSettlementV2Bytes(replayed.Settlement)
	if !bytes.Equal(settlementBody, replayedBody) {
		t.Fatal("durable settlement replay was not byte-identical")
	}
}

func TestRecoverExactSignedAbsentNeverRetriesOrSettles(t *testing.T) {
	fixture := newCoordinatorFixtureV1(t, "recover-absent")
	fixture.witness.errors = []error{monotonicheadport.ErrIndeterminate}
	if _, err := fixture.coordinator.CommitExact(context.Background(), fixture.intent); !errors.Is(err, ErrUnresolved) {
		t.Fatal(err)
	}
	fixture.witness.resolve = func(request domainsecurity.MonotonicHeadMutationResolveRequestV1) (domainsecurity.MonotonicHeadMutationResolutionV1, error) {
		return domainsecurity.NewMonotonicHeadMutationResolutionV1(
			request,
			fixture.intent.PreviousCheckpoint,
			nil,
			func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.witnessPriv, message), nil },
		)
	}
	if _, err := fixture.coordinator.RecoverExact(context.Background(), fixture.intent.MutationID); !errors.Is(err, ErrUnresolved) {
		t.Fatalf("signed absent recovery classification = %v", err)
	}
	if fixture.witness.calls != 1 || fixture.witness.resolveCalls != 1 || len(fixture.settlements.records) != 0 {
		t.Fatalf("signed absent response retried or settled: advance=%d resolve=%d settlements=%d", fixture.witness.calls, fixture.witness.resolveCalls, len(fixture.settlements.records))
	}
}

func TestMutationIDReuseWithDifferentRequestFailsClosed(t *testing.T) {
	fixture := newCoordinatorFixtureV1(t, "mutation-conflict")
	fixture.witness.errors = []error{monotonicheadport.ErrIndeterminate}
	if _, err := fixture.coordinator.CommitExact(context.Background(), fixture.intent); !errors.Is(err, ErrUnresolved) {
		t.Fatal(err)
	}
	different := newCoordinatorIntentForPolicyV1(t, fixture, "different-policy")
	fixture.riskIndexes.records[different.Transition.ThreadRiskGenesis.FirstIndex.IndexDigest] =
		different.Transition.ThreadRiskGenesis.FirstIndex
	if _, err := fixture.coordinator.CommitExact(context.Background(), different); err == nil {
		t.Fatal("same mutation ID with different request was accepted")
	}
	if fixture.witness.calls != 1 || len(fixture.settlements.records) != 0 {
		t.Fatal("conflicting intent dispatched or settled")
	}
}

func TestWitnessCommitWithoutSettlementNeverReturnsSuccess(t *testing.T) {
	fixture := newCoordinatorFixtureV1(t, "settlement-failure")
	fixture.settlements.putErr = errors.New("settlement disk failure")
	if _, err := fixture.coordinator.CommitExact(context.Background(), fixture.intent); !errors.Is(err, ErrCommittedUnsettled) {
		t.Fatalf("committed-unsettled classification = %v", err)
	}
	if fixture.witness.calls != 1 || len(fixture.settlements.records) != 0 {
		t.Fatal("settlement failure did not remain fail-closed")
	}
}

func TestSettlementJournalConflictIsCommittedUnsettledIntegrity(t *testing.T) {
	fixture := newCoordinatorFixtureV1(t, "settlement-conflict-classification")
	fixture.settlements.putErr = journalport.ErrConflict
	_, err := fixture.coordinator.CommitExact(context.Background(), fixture.intent)
	if !errors.Is(err, ErrCommittedUnsettled) || !errors.Is(err, ErrIntegrity) || errors.Is(err, ErrUnavailable) {
		t.Fatalf("settlement journal conflict classification = %v", err)
	}
}

func TestCASConflictCannotBecomeSupersededWithoutCompleteRange(t *testing.T) {
	fixture := newCoordinatorFixtureV1(t, "cas-conflict")
	fixture.witness.errors = []error{monotonicheadport.ErrCASConflict}
	if _, err := fixture.coordinator.CommitExact(context.Background(), fixture.intent); !errors.Is(err, monotonicheadport.ErrCASConflict) || !errors.Is(err, ErrUnresolved) {
		t.Fatalf("CAS conflict classification = %v", err)
	}
	if len(fixture.intents.records) != 1 || len(fixture.settlements.records) != 0 {
		t.Fatal("CAS conflict produced a terminal settlement without committed range")
	}
}

func TestWitnessIntegrityConflictsRemainUnresolvedIntegrityIncidents(t *testing.T) {
	for _, witnessErr := range []error{monotonicheadport.ErrMutationConflict, monotonicheadport.ErrEquivocation} {
		fixture := newCoordinatorFixtureV1(t, witnessErr.Error())
		fixture.witness.errors = []error{witnessErr}
		_, err := fixture.coordinator.CommitExact(context.Background(), fixture.intent)
		if !errors.Is(err, witnessErr) || !errors.Is(err, ErrUnresolved) || !errors.Is(err, ErrIntegrity) {
			t.Fatalf("witness integrity conflict classification = %v", err)
		}
	}
}

func newCoordinatorFixtureV1(t *testing.T, label string) coordinatorFixtureV1 {
	t.Helper()
	log := &coordinatorOperationLogV1{}
	authoritySeed := sha256.Sum256([]byte("coordinator-authority:" + label))
	authorityPrivate := ed25519.NewKeyFromSeed(authoritySeed[:])
	authorityPublic := authorityPrivate.Public().(ed25519.PublicKey)
	authority := coordinatorAuthorityV1{
		private: authorityPrivate, public: authorityPublic, keyID: domainsecurity.SHA256Hex(authorityPublic),
	}
	witnessSeed := sha256.Sum256([]byte("coordinator-witness:" + label))
	witnessPrivate := ed25519.NewKeyFromSeed(witnessSeed[:])
	witnessPublic := witnessPrivate.Public().(ed25519.PublicKey)
	installationID := domainsecurity.SHA256Hex([]byte("coordinator-installation:" + label))
	enrollmentID := domainsecurity.SHA256Hex([]byte("coordinator-enrollment:" + label))
	enrollment, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID,
		Namespace: domainsecurity.ThreadRiskAuthorityNamespaceV1, Generation: 0,
		CurrentStateDigest: domainsecurity.SHA256Hex([]byte("coordinator-enrollment-state:" + label)),
		FenceNonce:         domainsecurity.SHA256Hex([]byte("coordinator-enrollment-fence:" + label)),
		WitnessKeyID:       domainsecurity.SHA256Hex(witnessPublic), WitnessPublicKey: witnessPublic,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(witnessPrivate, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	mutationID := domainsecurity.SHA256Hex([]byte("coordinator-mutation:" + label))
	index := coordinatorRiskIndexV1(t, authority, installationID, enrollmentID, mutationID, "policy:"+label)
	binding, err := domainauthority.NewThreadRiskGenesisTransitionBindingV2(enrollment, index)
	if err != nil {
		t.Fatal(err)
	}
	request, err := domainsecurity.NewMonotonicHeadAdvanceRequestV1(domainsecurity.MonotonicHeadAdvanceRequestInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID,
		Namespace:          domainsecurity.ThreadRiskAuthorityNamespaceV1,
		ExpectedGeneration: 0, ExpectedCheckpointDigest: enrollment.CheckpointDigest,
		ExpectedStateDigest: enrollment.CurrentStateDigest, NextGeneration: 1, NextStateDigest: index.IndexDigest,
		ExpectedFenceNonce: enrollment.FenceNonce, MutationID: mutationID,
		AuthorityKeyID: authority.keyID, AuthorityPublicKey: authority.public,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(authority.private, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	intent, err := domainauthority.NewMonotonicAdvanceIntentV2(domainauthority.MonotonicAdvanceIntentInputV2{
		Root: domainauthority.AdvanceRootThreadRiskV2, PreviousCheckpoint: enrollment,
		AdvanceRequest: request, Transition: binding,
		AuthorityKeyID: authority.keyID, AuthorityPublicKey: authority.public,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(authority.private, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	nextCheckpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID,
		Namespace: domainsecurity.ThreadRiskAuthorityNamespaceV1, Generation: 1,
		CurrentStateDigest: index.IndexDigest, PreviousStateDigest: enrollment.CurrentStateDigest,
		PreviousCheckpointDigest: enrollment.CheckpointDigest,
		FenceNonce:               domainsecurity.SHA256Hex([]byte("coordinator-next-fence:" + label)),
		MutationID:               mutationID, WitnessKeyID: domainsecurity.SHA256Hex(witnessPublic), WitnessPublicKey: witnessPublic,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(witnessPrivate, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domainsecurity.NewMonotonicHeadAdvanceReceiptV1(
		request,
		nextCheckpoint,
		func(message []byte) ([]byte, error) { return ed25519.Sign(witnessPrivate, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	witness := &coordinatorWitnessV1{log: log, receipt: receipt}
	witness.resolve = func(resolveRequest domainsecurity.MonotonicHeadMutationResolveRequestV1) (domainsecurity.MonotonicHeadMutationResolutionV1, error) {
		return domainsecurity.NewMonotonicHeadMutationResolutionV1(
			resolveRequest,
			receipt.Checkpoint,
			&domainsecurity.MonotonicHeadCommittedMutationV1{AdvanceRequest: request, AdvanceReceipt: receipt},
			func(message []byte) ([]byte, error) { return ed25519.Sign(witnessPrivate, message), nil },
		)
	}
	intents := &coordinatorIntentStoreV2{log: log, records: make(map[string]domainauthority.MonotonicAdvanceIntentV2)}
	settlements := &coordinatorSettlementStoreV2{log: log, records: make(map[string]domainauthority.MonotonicAdvanceSettlementV2)}
	riskIndexes := &coordinatorRiskStoreV1{
		log: log, records: map[string]domainsecurity.ThreadRiskAuthorityIndexV1{index.IndexDigest: index},
	}
	config := Config{
		InstallationID: installationID, EnrollmentID: enrollmentID,
		Namespace: domainsecurity.ThreadRiskAuthorityNamespaceV1,
		Authority: authority, WitnessKeyID: domainsecurity.SHA256Hex(witnessPublic), WitnessPublicKey: witnessPublic,
		Witness: witness, Intents: intents, Settlements: settlements, RiskIndexes: riskIndexes,
	}
	coordinator, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	return coordinatorFixtureV1{
		coordinator: coordinator, config: config, intent: intent, index: index, witness: witness,
		intents: intents, settlements: settlements, riskIndexes: riskIndexes, log: log,
		authority: authority, witnessKey: witnessPublic, witnessPriv: witnessPrivate, enrollmentID: enrollmentID,
	}
}

func coordinatorRiskIndexV1(
	t *testing.T,
	authority coordinatorAuthorityV1,
	installationID, enrollmentID, mutationID, policyLabel string,
) domainsecurity.ThreadRiskAuthorityIndexV1 {
	t.Helper()
	index, err := domainsecurity.NewThreadRiskAuthorityIndexV1(domainsecurity.ThreadRiskAuthorityIndexInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID,
		Namespace: domainsecurity.ThreadRiskAuthorityNamespaceV1, Generation: 1,
		Entries: []domainsecurity.ThreadRiskAuthorityEntryV1{{
			ThreadID: "thread-coordinator", WorkspaceRealPath: "/workspace/coordinator",
			RiskClass:           domainsecurity.RiskClassGeneral,
			CurrentPolicyDigest: domainsecurity.SHA256Hex([]byte(policyLabel)),
		}},
		MutationID: mutationID, AuthorityKeyID: authority.keyID, AuthorityPublicKey: authority.public,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(authority.private, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	return index
}

func newCoordinatorIntentForPolicyV1(
	t *testing.T,
	fixture coordinatorFixtureV1,
	policyLabel string,
) domainauthority.MonotonicAdvanceIntentV2 {
	t.Helper()
	enrollment := fixture.intent.PreviousCheckpoint
	index := coordinatorRiskIndexV1(
		t,
		fixture.authority,
		fixture.intent.InstallationID,
		fixture.enrollmentID,
		fixture.intent.MutationID,
		policyLabel,
	)
	binding, err := domainauthority.NewThreadRiskGenesisTransitionBindingV2(enrollment, index)
	if err != nil {
		t.Fatal(err)
	}
	request, err := domainsecurity.NewMonotonicHeadAdvanceRequestV1(domainsecurity.MonotonicHeadAdvanceRequestInputV1{
		InstallationID: fixture.intent.InstallationID, EnrollmentID: fixture.enrollmentID,
		Namespace:          domainsecurity.ThreadRiskAuthorityNamespaceV1,
		ExpectedGeneration: enrollment.Generation, ExpectedCheckpointDigest: enrollment.CheckpointDigest,
		ExpectedStateDigest: enrollment.CurrentStateDigest, NextGeneration: index.Generation, NextStateDigest: index.IndexDigest,
		ExpectedFenceNonce: enrollment.FenceNonce, MutationID: fixture.intent.MutationID,
		AuthorityKeyID: fixture.authority.keyID, AuthorityPublicKey: fixture.authority.public,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.authority.private, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	intent, err := domainauthority.NewMonotonicAdvanceIntentV2(domainauthority.MonotonicAdvanceIntentInputV2{
		Root: domainauthority.AdvanceRootThreadRiskV2, PreviousCheckpoint: enrollment,
		AdvanceRequest: request, Transition: binding,
		AuthorityKeyID: fixture.authority.keyID, AuthorityPublicKey: fixture.authority.public,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.authority.private, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	return intent
}
