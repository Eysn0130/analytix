package datasetsnapshot

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	monotonicheadport "analytix.local/runtime-go/internal/ports/monotonichead"
)

func TestDatasetSnapshotServiceAcceptResolveInterleavedAndIdempotent(t *testing.T) {
	fixture := newDatasetSnapshotServiceFixture(t)
	ctx := context.Background()
	caseA := datasetSnapshotServiceObservation(t, "/workspace/case-a", "case-a", "a")
	caseB := datasetSnapshotServiceObservation(t, "/workspace/case-b", "case-b", "b")
	acceptedAt := time.Date(2026, 7, 13, 2, 0, 0, 0, time.UTC)

	recordA1, err := fixture.service.Accept(ctx, datasetSnapshotAcceptInput(caseA, "a-v1", acceptedAt))
	if err != nil {
		t.Fatal(err)
	}
	recordB1, err := fixture.service.Accept(ctx, datasetSnapshotAcceptInput(caseB, "b-v1", acceptedAt.Add(time.Minute)))
	if err != nil {
		t.Fatal(err)
	}
	resolvedA, err := fixture.service.ResolveWitnessed(ctx, datasetsnapshotport.ResolveInput{
		TenantID: "tenant-a", UserID: "user-a", Observation: caseA,
	})
	if err != nil || resolvedA.RecordDigest != recordA1.RecordDigest {
		t.Fatalf("interleaved case A resolved incorrectly: record=%#v err=%v", resolvedA, err)
	}
	resolvedB, err := fixture.service.ResolveWitnessed(ctx, datasetsnapshotport.ResolveInput{
		TenantID: "tenant-a", UserID: "user-a", Observation: caseB,
	})
	if err != nil || resolvedB.RecordDigest != recordB1.RecordDigest {
		t.Fatalf("interleaved case B resolved incorrectly: record=%#v err=%v", resolvedB, err)
	}

	before := fixture.coordinator.successfulAdvances
	idempotent, err := fixture.service.Accept(ctx, datasetSnapshotAcceptInput(caseA, "a-v1", acceptedAt.Add(2*time.Minute)))
	if err != nil || idempotent.RecordDigest != recordA1.RecordDigest || fixture.coordinator.successfulAdvances != before {
		t.Fatalf("identical content minted another authority record: record=%#v advances=%d err=%v", idempotent, fixture.coordinator.successfulAdvances, err)
	}

	recordA2, err := fixture.service.Accept(ctx, datasetSnapshotAcceptInput(caseA, "a-v2", acceptedAt.Add(3*time.Minute)))
	if err != nil || recordA2.PredecessorRecordDigest != recordA1.RecordDigest || recordA2.DatasetSnapshotID == recordA1.DatasetSnapshotID {
		t.Fatalf("explicit per-binding successor is invalid: record=%#v err=%v", recordA2, err)
	}
	resolvedA, err = fixture.service.ResolveWitnessed(ctx, datasetsnapshotport.ResolveInput{
		TenantID: "tenant-a", UserID: "user-a", Observation: caseA,
	})
	if err != nil || resolvedA.RecordDigest != recordA2.RecordDigest {
		t.Fatalf("latest case A snapshot was not selected: record=%#v err=%v", resolvedA, err)
	}
	if _, err := fixture.service.Accept(ctx, datasetSnapshotAcceptInput(caseA, "a-v1", acceptedAt.Add(4*time.Minute))); !errors.Is(err, datasetsnapshotport.ErrStale) {
		t.Fatalf("historical A -> B -> A content replay was not rejected: %v", err)
	}
	if _, err := fixture.service.ResolveWitnessed(ctx, datasetsnapshotport.ResolveInput{
		TenantID: "tenant-other", UserID: "user-a", Observation: caseA,
	}); !errors.Is(err, datasetsnapshotport.ErrUnavailable) {
		t.Fatalf("cross-tenant lookup exposed or selected another tenant snapshot: %v", err)
	}
}

func TestDatasetSnapshotServiceRejectsGlobalReplayAndPerBindingLineageDamage(t *testing.T) {
	t.Run("non-adjacent record replay", func(t *testing.T) {
		fixture := newDatasetSnapshotServiceFixture(t)
		ctx := context.Background()
		observation := datasetSnapshotServiceObservation(t, "/workspace/case-a", "case-a", "replay")
		at := time.Date(2026, 7, 13, 3, 0, 0, 0, time.UTC)
		first, err := fixture.service.Accept(ctx, datasetSnapshotAcceptInput(observation, "v1", at))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.service.Accept(ctx, datasetSnapshotAcceptInput(observation, "v2", at.Add(time.Minute))); err != nil {
			t.Fatal(err)
		}
		head := fixture.coordinator.currentBundle()
		previous, err := fixture.indexes.Resolve(ctx, head.DatasetSnapshotIndexDigest)
		if err != nil {
			t.Fatal(err)
		}
		replay := fixture.signedIndex(t, previous.Generation+1, previous.IndexDigest, first.RecordDigest, previous.Binding, "replay-old-record")
		fixture.indexes.records[replay.IndexDigest] = replay
		if _, err := fixture.coordinator.AdvanceDatasetSnapshot(ctx, evidenceauthorityport.DatasetAdvanceInput{
			ExpectedBundleDigest: head.RecordDigest, NextIndexDigest: replay.IndexDigest,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.service.ResolveWitnessed(ctx, datasetsnapshotport.ResolveInput{
			TenantID: "tenant-a", UserID: "user-a", Observation: observation,
		}); !errors.Is(err, datasetsnapshotport.ErrCorrupt) {
			t.Fatalf("non-adjacent snapshot record replay was accepted: %v", err)
		}
	})

	t.Run("wrong per-binding predecessor", func(t *testing.T) {
		fixture := newDatasetSnapshotServiceFixture(t)
		ctx := context.Background()
		observation := datasetSnapshotServiceObservation(t, "/workspace/case-a", "case-a", "lineage")
		at := time.Date(2026, 7, 13, 3, 30, 0, 0, time.UTC)
		first, err := fixture.service.Accept(ctx, datasetSnapshotAcceptInput(observation, "v1", at))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.service.Accept(ctx, datasetSnapshotAcceptInput(observation, "v2", at.Add(time.Minute))); err != nil {
			t.Fatal(err)
		}
		head := fixture.coordinator.currentBundle()
		previous, _ := fixture.indexes.Resolve(ctx, head.DatasetSnapshotIndexDigest)
		brokenRecord := fixture.signedRecord(t, observation, "v3", first.RecordDigest, at.Add(2*time.Minute))
		fixture.records.records[brokenRecord.RecordDigest] = brokenRecord
		brokenIndex := fixture.signedIndex(t, previous.Generation+1, previous.IndexDigest, brokenRecord.RecordDigest, previous.Binding, "wrong-predecessor")
		fixture.indexes.records[brokenIndex.IndexDigest] = brokenIndex
		if _, err := fixture.coordinator.AdvanceDatasetSnapshot(ctx, evidenceauthorityport.DatasetAdvanceInput{
			ExpectedBundleDigest: head.RecordDigest, NextIndexDigest: brokenIndex.IndexDigest,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.service.ResolveWitnessed(ctx, datasetsnapshotport.ResolveInput{
			TenantID: "tenant-a", UserID: "user-a", Observation: observation,
		}); !errors.Is(err, datasetsnapshotport.ErrCorrupt) {
			t.Fatalf("wrong per-binding predecessor was accepted: %v", err)
		}
	})
}

func TestDatasetSnapshotServiceFailsClosedOnMissingCASStaleAdvanceAndCountMismatch(t *testing.T) {
	t.Run("missing index", func(t *testing.T) {
		fixture := newDatasetSnapshotServiceFixture(t)
		observation := datasetSnapshotServiceObservation(t, "/workspace/case-a", "case-a", "missing")
		if _, err := fixture.service.Accept(context.Background(), datasetSnapshotAcceptInput(
			observation, "v1", time.Date(2026, 7, 13, 4, 0, 0, 0, time.UTC),
		)); err != nil {
			t.Fatal(err)
		}
		delete(fixture.indexes.records, fixture.coordinator.currentBundle().DatasetSnapshotIndexDigest)
		if _, err := fixture.service.ResolveWitnessed(context.Background(), datasetsnapshotport.ResolveInput{
			TenantID: "tenant-a", UserID: "user-a", Observation: observation,
		}); !errors.Is(err, datasetsnapshotport.ErrCorrupt) {
			t.Fatalf("missing witness-selected CAS index did not fail closed: %v", err)
		}
	})

	t.Run("stale expected bundle", func(t *testing.T) {
		fixture := newDatasetSnapshotServiceFixture(t)
		fixture.coordinator.advanceErr = monotonicheadport.ErrCASConflict
		observation := datasetSnapshotServiceObservation(t, "/workspace/case-a", "case-a", "stale")
		if _, err := fixture.service.Accept(context.Background(), datasetSnapshotAcceptInput(
			observation, "v1", time.Date(2026, 7, 13, 4, 30, 0, 0, time.UTC),
		)); !errors.Is(err, datasetsnapshotport.ErrStale) {
			t.Fatalf("stale shared bundle CAS was not typed: %v", err)
		}
		if fixture.coordinator.currentBundle().DatasetSnapshotCount != 0 {
			t.Fatal("failed CAS advanced dataset authority")
		}
	})

	t.Run("bundle count and index generation mismatch", func(t *testing.T) {
		fixture := newDatasetSnapshotServiceFixture(t)
		ctx := context.Background()
		observation := datasetSnapshotServiceObservation(t, "/workspace/case-a", "case-a", "count")
		at := time.Date(2026, 7, 13, 5, 0, 0, 0, time.UTC)
		first, err := fixture.service.Accept(ctx, datasetSnapshotAcceptInput(observation, "v1", at))
		if err != nil {
			t.Fatal(err)
		}
		head := fixture.coordinator.currentBundle()
		previous, _ := fixture.indexes.Resolve(ctx, head.DatasetSnapshotIndexDigest)
		invalidGeneration := fixture.signedIndex(t, 3, previous.IndexDigest, first.RecordDigest, previous.Binding, "count-mismatch")
		fixture.indexes.records[invalidGeneration.IndexDigest] = invalidGeneration
		if _, err := fixture.coordinator.AdvanceDatasetSnapshot(ctx, evidenceauthorityport.DatasetAdvanceInput{
			ExpectedBundleDigest: head.RecordDigest, NextIndexDigest: invalidGeneration.IndexDigest,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.service.ResolveWitnessed(ctx, datasetsnapshotport.ResolveInput{
			TenantID: "tenant-a", UserID: "user-a", Observation: observation,
		}); !errors.Is(err, datasetsnapshotport.ErrCorrupt) {
			t.Fatalf("bundle count/index generation mismatch was accepted: %v", err)
		}
	})
}

type datasetSnapshotServiceFixture struct {
	authority   *datasetSnapshotTestAuthority
	coordinator *datasetSnapshotTestCoordinator
	records     *datasetSnapshotMemoryRecordStore
	indexes     *datasetSnapshotMemoryIndexStore
	service     *Service
}

func newDatasetSnapshotServiceFixture(t *testing.T) *datasetSnapshotServiceFixture {
	t.Helper()
	authority := newDatasetSnapshotTestAuthority(0x31)
	coordinator := newDatasetSnapshotTestCoordinator(t, authority)
	records := &datasetSnapshotMemoryRecordStore{records: map[string]domainsecurity.DatasetSnapshotAuthorityRecordV1{}}
	indexes := &datasetSnapshotMemoryIndexStore{records: map[string]domainsecurity.DatasetSnapshotIndexV1{}}
	service, err := New(Config{
		InstallationID: coordinator.installationID, EnrollmentID: coordinator.enrollmentID,
		Authority: authority, Coordinator: coordinator, Records: records, Indexes: indexes,
		Random: &datasetSnapshotCounterReader{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &datasetSnapshotServiceFixture{authority: authority, coordinator: coordinator, records: records, indexes: indexes, service: service}
}

func (fixture *datasetSnapshotServiceFixture) signedIndex(
	t *testing.T,
	generation uint64,
	previous, recordDigest string,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	label string,
) domainsecurity.DatasetSnapshotIndexV1 {
	t.Helper()
	index, err := domainsecurity.NewDatasetSnapshotIndexV1(domainsecurity.DatasetSnapshotIndexInputV1{
		InstallationID: fixture.coordinator.installationID, EnrollmentID: fixture.coordinator.enrollmentID,
		Generation: generation, PreviousIndexDigest: previous,
		MutationID: domainsecurity.SHA256Hex([]byte("dataset-snapshot-test-index:" + label)),
		Binding:    binding, SnapshotRecordDigest: recordDigest,
		AuthorityKeyID: fixture.authority.KeyID(), AuthorityPublicKey: fixture.authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	return index
}

func (fixture *datasetSnapshotServiceFixture) signedRecord(
	t *testing.T,
	observation domainsecurity.CaseBindingObservationV1,
	material, predecessor string,
	acceptedAt time.Time,
) domainsecurity.DatasetSnapshotAuthorityRecordV1 {
	t.Helper()
	record, err := domainsecurity.NewDatasetSnapshotAuthorityRecordV1(domainsecurity.DatasetSnapshotAuthorityRecordInputV1{
		InstallationID: fixture.coordinator.installationID, TenantID: "tenant-a", UserID: "user-a",
		WorkspaceRealPath: observation.WorkspaceRealPath, CaseID: observation.CaseID,
		CaseBindingHash: observation.CaseBindingHash, BindingObservationDigest: observation.ObservationDigest,
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source:" + material)),
		RawManifestSHA256:  domainsecurity.SHA256Hex([]byte("raw:" + material)), ParserVersion: "parser/v1",
		AcceptedAt: acceptedAt, PredecessorRecordDigest: predecessor,
		AuthorityKeyID: fixture.authority.KeyID(), AuthorityPublicKey: fixture.authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func datasetSnapshotAcceptInput(observation domainsecurity.CaseBindingObservationV1, material string, acceptedAt time.Time) AcceptInput {
	return AcceptInput{
		TenantID: "tenant-a", UserID: "user-a", Observation: observation,
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source:" + material)),
		RawManifestSHA256:  domainsecurity.SHA256Hex([]byte("raw:" + material)),
		ParserVersion:      "parser/v1", AcceptedAt: acceptedAt,
	}
}

func datasetSnapshotServiceObservation(t *testing.T, workspace, caseID, label string) domainsecurity.CaseBindingObservationV1 {
	t.Helper()
	observation, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: workspace, State: domainsecurity.CaseBindingStateValid, CaseID: caseID,
		BindingSHA256:   domainsecurity.SHA256Hex([]byte("binding-file:" + label)),
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-canonical:" + label)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

type datasetSnapshotTestAuthority struct {
	private ed25519.PrivateKey
	public  ed25519.PublicKey
}

func newDatasetSnapshotTestAuthority(seed byte) *datasetSnapshotTestAuthority {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize))
	return &datasetSnapshotTestAuthority{private: privateKey, public: privateKey.Public().(ed25519.PublicKey)}
}

func (authority *datasetSnapshotTestAuthority) KeyID() string {
	return domainsecurity.SHA256Hex(authority.public)
}
func (authority *datasetSnapshotTestAuthority) PublicKey() []byte {
	return append([]byte(nil), authority.public...)
}
func (authority *datasetSnapshotTestAuthority) Sign(ctx context.Context, body []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ed25519.Sign(authority.private, body), nil
}
func (authority *datasetSnapshotTestAuthority) VerifyTrusted(ctx context.Context, keyID string, publicKey, body, signature []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if keyID != authority.KeyID() || !bytes.Equal(publicKey, authority.public) || !ed25519.Verify(authority.public, body, signature) {
		return errors.New("untrusted test authority")
	}
	return nil
}

type datasetSnapshotTestCoordinator struct {
	mu                 sync.Mutex
	authority          *datasetSnapshotTestAuthority
	witnessPrivate     ed25519.PrivateKey
	witnessPublic      ed25519.PublicKey
	installationID     string
	enrollmentID       string
	bundle             domainevidence.EvidenceAuthorityBundleV1
	checkpoint         domainsecurity.MonotonicHeadCheckpointV1
	observeCount       uint64
	successfulAdvances int
	advanceErr         error
}

func newDatasetSnapshotTestCoordinator(t *testing.T, authority *datasetSnapshotTestAuthority) *datasetSnapshotTestCoordinator {
	t.Helper()
	witnessPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x77}, ed25519.SeedSize))
	coordinator := &datasetSnapshotTestCoordinator{
		authority: authority, witnessPrivate: witnessPrivate, witnessPublic: witnessPrivate.Public().(ed25519.PublicKey),
		installationID: domainsecurity.SHA256Hex([]byte("dataset-service-installation")),
		enrollmentID:   domainsecurity.SHA256Hex([]byte("dataset-service-enrollment")),
	}
	bundle, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID: coordinator.installationID, EnrollmentID: coordinator.enrollmentID, Generation: 1,
		MutationID:                  domainsecurity.SHA256Hex([]byte("dataset-service-bundle-genesis")),
		DatasetSnapshotIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
		EvidenceRegistryIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
		PublicationIndexDigest:      domainpublication.PublicationIndexGenesisDigestV1(),
		AuthorityKeyID:              authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := coordinator.checkpointFor(bundle, domainsecurity.MonotonicHeadCheckpointV1{})
	if err != nil {
		t.Fatal(err)
	}
	coordinator.bundle = bundle
	coordinator.checkpoint = checkpoint
	return coordinator
}

func (coordinator *datasetSnapshotTestCoordinator) ObserveFresh(ctx context.Context) (evidenceauthorityport.FreshHead, error) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	return coordinator.observeLocked()
}

func (coordinator *datasetSnapshotTestCoordinator) AdvanceDatasetSnapshot(ctx context.Context, input evidenceauthorityport.DatasetAdvanceInput) (evidenceauthorityport.FreshHead, error) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	if coordinator.advanceErr != nil {
		return evidenceauthorityport.FreshHead{}, coordinator.advanceErr
	}
	if input.ExpectedBundleDigest != coordinator.bundle.RecordDigest {
		return evidenceauthorityport.FreshHead{}, monotonicheadport.ErrCASConflict
	}
	previous := coordinator.bundle
	next, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID: previous.InstallationID, EnrollmentID: previous.EnrollmentID,
		Generation: previous.Generation + 1, PreviousBundleDigest: previous.RecordDigest,
		MutationID:                 domainsecurity.SHA256Hex([]byte(fmt.Sprintf("dataset-service-bundle-mutation:%d", previous.Generation+1))),
		DatasetSnapshotIndexDigest: input.NextIndexDigest, DatasetSnapshotCount: previous.DatasetSnapshotCount + 1,
		EvidenceRegistryIndexDigest: previous.EvidenceRegistryIndexDigest, EvidenceRegistryCount: previous.EvidenceRegistryCount,
		PublicationIndexDigest: previous.PublicationIndexDigest, PublicationCount: previous.PublicationCount,
		AuthorityKeyID: coordinator.authority.KeyID(), AuthorityPublicKey: coordinator.authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return coordinator.authority.Sign(ctx, message) })
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	checkpoint, err := coordinator.checkpointFor(next, coordinator.checkpoint)
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	coordinator.bundle = next
	coordinator.checkpoint = checkpoint
	coordinator.successfulAdvances++
	return coordinator.observeLocked()
}

func (coordinator *datasetSnapshotTestCoordinator) currentBundle() domainevidence.EvidenceAuthorityBundleV1 {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	return coordinator.bundle
}

func (coordinator *datasetSnapshotTestCoordinator) observeLocked() (evidenceauthorityport.FreshHead, error) {
	coordinator.observeCount++
	request, err := domainsecurity.NewMonotonicHeadObserveRequestV1(domainsecurity.MonotonicHeadObserveRequestInputV1{
		InstallationID: coordinator.installationID, EnrollmentID: coordinator.enrollmentID,
		Namespace:      domainevidence.EvidenceAuthorityBundleWitnessNamespaceV1,
		ChallengeNonce: domainsecurity.SHA256Hex([]byte(fmt.Sprintf("dataset-service-challenge:%d", coordinator.observeCount))),
		AuthorityKeyID: coordinator.authority.KeyID(), AuthorityPublicKey: coordinator.authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return coordinator.authority.Sign(context.Background(), message) })
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	observation, err := domainsecurity.NewMonotonicHeadObservationV1(request, coordinator.checkpoint, func(message []byte) ([]byte, error) {
		return ed25519.Sign(coordinator.witnessPrivate, message), nil
	})
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	return evidenceauthorityport.FreshHead{HasBundle: true, Bundle: coordinator.bundle, Request: request, Observation: observation}, nil
}

func (coordinator *datasetSnapshotTestCoordinator) checkpointFor(
	bundle domainevidence.EvidenceAuthorityBundleV1,
	previous domainsecurity.MonotonicHeadCheckpointV1,
) (domainsecurity.MonotonicHeadCheckpointV1, error) {
	previousState := domainsecurity.SHA256Hex([]byte("dataset-service-enrolled-state"))
	previousCheckpoint := domainsecurity.SHA256Hex([]byte("dataset-service-enrolled-checkpoint"))
	if previous.Generation != 0 {
		previousState = previous.CurrentStateDigest
		previousCheckpoint = previous.CheckpointDigest
	}
	return domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: coordinator.installationID, EnrollmentID: coordinator.enrollmentID,
		Namespace:  domainevidence.EvidenceAuthorityBundleWitnessNamespaceV1,
		Generation: bundle.Generation, CurrentStateDigest: bundle.RecordDigest,
		PreviousStateDigest: previousState, PreviousCheckpointDigest: previousCheckpoint,
		FenceNonce:   domainsecurity.SHA256Hex([]byte(fmt.Sprintf("dataset-service-fence:%d", bundle.Generation))),
		MutationID:   bundle.MutationID,
		WitnessKeyID: domainsecurity.SHA256Hex(coordinator.witnessPublic), WitnessPublicKey: coordinator.witnessPublic,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(coordinator.witnessPrivate, message), nil })
}

type datasetSnapshotMemoryRecordStore struct {
	records map[string]domainsecurity.DatasetSnapshotAuthorityRecordV1
}

func (store *datasetSnapshotMemoryRecordStore) PutIfAbsent(_ context.Context, record domainsecurity.DatasetSnapshotAuthorityRecordV1) error {
	if current, ok := store.records[record.RecordDigest]; ok && current != record {
		return errors.New("record conflict")
	}
	store.records[record.RecordDigest] = record
	return nil
}
func (store *datasetSnapshotMemoryRecordStore) Resolve(_ context.Context, digest string) (domainsecurity.DatasetSnapshotAuthorityRecordV1, error) {
	record, ok := store.records[digest]
	if !ok {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, errors.New("record absent")
	}
	return record, nil
}

type datasetSnapshotMemoryIndexStore struct {
	records map[string]domainsecurity.DatasetSnapshotIndexV1
}

func (store *datasetSnapshotMemoryIndexStore) PutIfAbsent(_ context.Context, index domainsecurity.DatasetSnapshotIndexV1) error {
	if current, ok := store.records[index.IndexDigest]; ok && current != index {
		return errors.New("index conflict")
	}
	store.records[index.IndexDigest] = index
	return nil
}
func (store *datasetSnapshotMemoryIndexStore) Resolve(_ context.Context, digest string) (domainsecurity.DatasetSnapshotIndexV1, error) {
	index, ok := store.records[digest]
	if !ok {
		return domainsecurity.DatasetSnapshotIndexV1{}, errors.New("index absent")
	}
	return index, nil
}

type datasetSnapshotCounterReader struct {
	mu      sync.Mutex
	counter byte
}

func (reader *datasetSnapshotCounterReader) Read(body []byte) (int, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	reader.counter++
	for index := range body {
		body[index] = reader.counter + byte(index*17)
	}
	return len(body), nil
}
