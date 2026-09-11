package fundscsvadmission

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	datasetsnapshotapp "analytix.local/runtime-go/internal/app/datasetsnapshot"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	datasetsnapshotv2fixture "analytix.local/runtime-go/internal/testsupport/datasetsnapshotv2fixture"
)

type cleaningSnapshotAuthorityStubV1 struct {
	current    datasetsnapshotport.ResolvedSnapshotV2
	admitValue datasetsnapshotport.ResolvedSnapshotV2
	admitErr   error
	admitCalls int
	expected   string
}

func (stub *cleaningSnapshotAuthorityStubV1) AdmitAfterExactV2(
	ctx context.Context,
	input datasetsnapshotapp.AdmitAfterInputV2,
) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	stub.expected = input.ExpectedCurrentDatasetSnapshotID
	return stub.AdmitExactV2(ctx, input.AdmitInputV2)
}

func (stub *cleaningSnapshotAuthorityStubV1) AdmitExactV2(
	_ context.Context,
	input datasetsnapshotapp.AdmitInputV2,
) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	stub.admitCalls++
	if input.ManifestReference.ByteLength == 0 || input.FundsProducerReference.ByteLength == 0 ||
		input.AcceptedAt.IsZero() {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrMismatch
	}
	return stub.admitValue, stub.admitErr
}

func (stub *cleaningSnapshotAuthorityStubV1) ResolveWitnessedV2(
	_ context.Context,
	input datasetsnapshotport.ResolveInputV2,
) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	if datasetsnapshotport.ValidateResolvedSnapshotV2(stub.current) != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrUnavailable
	}
	if input.ExpectedDatasetSnapshotID != "" &&
		input.ExpectedDatasetSnapshotID != stub.current.Record.DatasetSnapshotID {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrStale
	}
	return stub.current, nil
}

func TestCleaningAdmissionCASFailurePreservesOldHeadAndPostCASLossIsOutcomeUnknown(t *testing.T) {
	workspace := "/synthetic/cleaning-case"
	observation := importObservationForTestV1(t, workspace, "case-cleaning")
	oldHead := cleaningResolvedSnapshotV1(t, observation, "old-head")
	output := cleaningResolvedSnapshotV1(t, observation, "output-head")
	summary := domainevidence.FundsCanonicalCSVAdmissionSummaryV1{
		ManifestSHA256: domainsecurity.SHA256Hex([]byte("manifest")), ManifestByteLength: 128,
		ProducerSHA256: domainsecurity.SHA256Hex([]byte("producer")), ProducerByteLength: 256,
		SourceArtifactSHA256: domainsecurity.SHA256Hex([]byte("source")), SourceRowCount: 2,
	}

	t.Run("pre-CAS failure", func(t *testing.T) {
		snapshots := &cleaningSnapshotAuthorityStubV1{
			current: oldHead, admitErr: errors.New("CAS rejected before linearization"),
		}
		service := &ServiceV1{
			observer: &importObserverStubV1{observation: observation}, snapshots: snapshots,
			now: func() time.Time { return time.Date(2026, 8, 22, 2, 0, 0, 0, time.UTC) },
		}
		status, err := service.admitComposedCleaningV1(
			context.Background(), workspace, observation, summary,
			output.Record.DatasetSnapshotID, oldHead.Record.DatasetSnapshotID,
		)
		if err != nil || status != CleaningCommitStatusPreCASFailedV1 || snapshots.admitCalls != 1 || snapshots.expected != oldHead.Record.DatasetSnapshotID ||
			snapshots.current.Record.DatasetSnapshotID != oldHead.Record.DatasetSnapshotID {
			t.Fatalf("pre-CAS failure changed current head: status=%q current=%q err=%v", status, snapshots.current.Record.DatasetSnapshotID, err)
		}
	})

	t.Run("unreconciled CAS failure remains outcome unknown", func(t *testing.T) {
		thirdHead := cleaningResolvedSnapshotV1(t, observation, "third-head")
		snapshots := &cleaningSnapshotAuthorityStubV1{
			current: thirdHead, admitErr: errors.New("CAS result cannot be reconciled"),
		}
		service := &ServiceV1{
			observer: &importObserverStubV1{observation: observation}, snapshots: snapshots,
			now: func() time.Time { return time.Date(2026, 8, 22, 2, 0, 0, 0, time.UTC) },
		}
		status, err := service.admitComposedCleaningV1(
			context.Background(), workspace, observation, summary,
			output.Record.DatasetSnapshotID, oldHead.Record.DatasetSnapshotID,
		)
		if err != nil || status != CleaningCommitStatusOutcomeUnknownV1 ||
			snapshots.current.Record.DatasetSnapshotID != thirdHead.Record.DatasetSnapshotID {
			t.Fatalf("unreconciled CAS failure was downgraded: status=%q current=%q err=%v", status, snapshots.current.Record.DatasetSnapshotID, err)
		}
	})

	t.Run("post-CAS response loss", func(t *testing.T) {
		snapshots := &cleaningSnapshotAuthorityStubV1{
			current: output, admitErr: errors.New("response lost after CAS"),
		}
		service := &ServiceV1{
			observer: &importObserverStubV1{observation: observation}, snapshots: snapshots,
			now: func() time.Time { return time.Date(2026, 8, 22, 2, 0, 0, 0, time.UTC) },
		}
		status, err := service.admitComposedCleaningV1(
			context.Background(), workspace, observation, summary,
			output.Record.DatasetSnapshotID, oldHead.Record.DatasetSnapshotID,
		)
		if err != nil || status != CleaningCommitStatusOutcomeUnknownV1 || snapshots.admitCalls != 1 ||
			snapshots.expected != oldHead.Record.DatasetSnapshotID ||
			snapshots.current.Record.DatasetSnapshotID != output.Record.DatasetSnapshotID {
			t.Fatalf("post-CAS loss invented rollback: status=%q current=%q err=%v", status, snapshots.current.Record.DatasetSnapshotID, err)
		}
	})
}

func cleaningResolvedSnapshotV1(
	t *testing.T,
	observation domainsecurity.CaseBindingObservationV1,
	material string,
) datasetsnapshotport.ResolvedSnapshotV2 {
	t.Helper()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{byte(len(material) + 17)}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	resolved, err := datasetsnapshotv2fixture.NewResolvedSnapshotV2(datasetsnapshotv2fixture.ResolvedInput{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		Observation: observation, Material: material,
		InstallationID: domainsecurity.SHA256Hex([]byte("cleaning-installation")),
		AcceptedAt:     time.Date(2026, 8, 22, 1, 0, 0, 0, time.UTC),
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
		Sign: func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}
