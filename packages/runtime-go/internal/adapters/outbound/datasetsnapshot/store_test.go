package datasetsnapshot

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

type datasetSnapshotStoreFixture struct {
	private        ed25519.PrivateKey
	public         ed25519.PublicKey
	installationID string
	enrollmentID   string
	record         domainsecurity.DatasetSnapshotAuthorityRecordV1
	index          domainsecurity.DatasetSnapshotIndexV1
}

func newDatasetSnapshotStoreFixture(t *testing.T) datasetSnapshotStoreFixture {
	t.Helper()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x53}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	installationID := domainsecurity.SHA256Hex([]byte("dataset-store-installation"))
	enrollmentID := domainsecurity.SHA256Hex([]byte("dataset-store-enrollment"))
	observation, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: "/workspace/case-a", State: domainsecurity.CaseBindingStateValid, CaseID: "case-a",
		BindingSHA256:   domainsecurity.SHA256Hex([]byte("dataset-store-binding-file")),
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("dataset-store-binding")),
	})
	if err != nil {
		t.Fatal(err)
	}
	sign := func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil }
	record, err := domainsecurity.NewDatasetSnapshotAuthorityRecordV1(domainsecurity.DatasetSnapshotAuthorityRecordInputV1{
		InstallationID: installationID, TenantID: "tenant-a", UserID: "user-a",
		WorkspaceRealPath: observation.WorkspaceRealPath, CaseID: observation.CaseID,
		CaseBindingHash: observation.CaseBindingHash, BindingObservationDigest: observation.ObservationDigest,
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("dataset-store-source-manifest")),
		RawManifestSHA256:  domainsecurity.SHA256Hex([]byte("dataset-store-raw-manifest")), ParserVersion: "parser/v1",
		AcceptedAt:     time.Date(2026, 7, 13, 6, 0, 0, 0, time.UTC),
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := domainsecurity.DatasetSnapshotBindingKeyFromRecordV1(record)
	if err != nil {
		t.Fatal(err)
	}
	index, err := domainsecurity.NewDatasetSnapshotIndexV1(domainsecurity.DatasetSnapshotIndexInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Generation: 1,
		PreviousIndexDigest: domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
		MutationID:          domainsecurity.SHA256Hex([]byte("dataset-store-mutation")),
		Binding:             binding, SnapshotRecordDigest: record.RecordDigest,
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	return datasetSnapshotStoreFixture{
		private: privateKey, public: publicKey, installationID: installationID, enrollmentID: enrollmentID,
		record: record, index: index,
	}
}

func newTestRecordStore(t *testing.T, root string) (*RecordStore, error) {
	t.Helper()
	mutation, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		return nil, err
	}
	return NewRecordStore(root, mutation)
}

func newTestIndexStore(t *testing.T, root string) (*IndexStore, error) {
	t.Helper()
	mutation, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		return nil, err
	}
	return NewIndexStore(root, mutation)
}

func TestDatasetSnapshotCASStoresRoundTripRestartAndConcurrentIdempotence(t *testing.T) {
	fixture := newDatasetSnapshotStoreFixture(t)
	ctx := context.Background()
	recordRoot := filepath.Join(t.TempDir(), "records")
	indexRoot := filepath.Join(t.TempDir(), "indexes")
	records, err := newTestRecordStore(t, recordRoot)
	if err != nil {
		t.Fatal(err)
	}
	indexes, err := newTestIndexStore(t, indexRoot)
	if err != nil {
		t.Fatal(err)
	}

	var wait sync.WaitGroup
	errorsOut := make(chan error, 32)
	for range 16 {
		wait.Add(2)
		go func() {
			defer wait.Done()
			errorsOut <- records.PutIfAbsent(ctx, fixture.record)
		}()
		go func() {
			defer wait.Done()
			errorsOut <- indexes.PutIfAbsent(ctx, fixture.index)
		}()
	}
	wait.Wait()
	close(errorsOut)
	for err := range errorsOut {
		if err != nil {
			t.Fatal(err)
		}
	}
	resolvedRecord, err := records.Resolve(ctx, fixture.record.RecordDigest)
	if err != nil || resolvedRecord != fixture.record {
		t.Fatalf("record CAS round trip failed: record=%#v err=%v", resolvedRecord, err)
	}
	resolvedIndex, err := indexes.Resolve(ctx, fixture.index.IndexDigest)
	if err != nil || resolvedIndex != fixture.index {
		t.Fatalf("index CAS round trip failed: index=%#v err=%v", resolvedIndex, err)
	}
	restartedRecords, err := newTestRecordStore(t, recordRoot)
	if err != nil {
		t.Fatal(err)
	}
	restartedIndexes, err := newTestIndexStore(t, indexRoot)
	if err != nil {
		t.Fatal(err)
	}
	if record, err := restartedRecords.Resolve(ctx, fixture.record.RecordDigest); err != nil || record != fixture.record {
		t.Fatalf("restarted record store lost content address: record=%#v err=%v", record, err)
	}
	if index, err := restartedIndexes.Resolve(ctx, fixture.index.IndexDigest); err != nil || index != fixture.index {
		t.Fatalf("restarted index store lost content address: index=%#v err=%v", index, err)
	}
	unknown := domainsecurity.SHA256Hex([]byte("unknown-dataset-store-record"))
	if _, err := records.Resolve(ctx, unknown); err == nil {
		t.Fatal("unknown record digest was resolved")
	}
	if _, err := indexes.Resolve(ctx, unknown); err == nil {
		t.Fatal("unknown index digest was resolved")
	}
}

func TestDatasetSnapshotStoresExposeNoLocalAuthoritySelector(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(&RecordStore{}), reflect.TypeOf(&IndexStore{})} {
		for _, name := range []string{"List", "Current", "ResolveCurrent", "ResolveWitnessed", "Active", "Latest"} {
			if _, found := typ.MethodByName(name); found {
				t.Fatalf("%s exposes forbidden local authority-selection method %s", typ, name)
			}
		}
	}
	if _, err := NewRecordStore("", nil); err == nil {
		t.Fatal("empty record root was accepted")
	}
	if _, err := NewIndexStore(" ", nil); err == nil {
		t.Fatal("non-canonical index root was accepted")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	fixture := newDatasetSnapshotStoreFixture(t)
	records, _ := newTestRecordStore(t, filepath.Join(t.TempDir(), "records"))
	if err := records.PutIfAbsent(canceled, fixture.record); !errors.Is(err, context.Canceled) {
		t.Fatalf("record store hid cancellation: %v", err)
	}
}
