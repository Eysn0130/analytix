package datasetsnapshot

import (
	"context"
	"errors"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainhost "analytix.local/runtime-go/internal/domain/hostcurrentness"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	fixturev2 "analytix.local/runtime-go/internal/testsupport/datasetsnapshotv2fixture"
)

func TestHostLocalSealedV2ExpectedPredecessorControlsCleaningAdmission(t *testing.T) {
	service, fixture, heads := hostLocalExpectedPredecessorFixtureV2(t)
	ctx := context.Background()
	first, err := service.AdmitExactV2(ctx, fixture.admitInput())
	if err != nil {
		t.Fatal(err)
	}
	input := hostLocalExpectedSuccessorInputV2(t, fixture, first, 2)
	cleaned, err := service.AdmitAfterExactV2(ctx, AdmitAfterInputV2{
		AdmitInputV2: input, ExpectedCurrentDatasetSnapshotID: first.Record.DatasetSnapshotID,
	})
	if err != nil {
		t.Fatalf("cleaning with the exact current predecessor failed: %v", err)
	}
	if cleaned.Record.DatasetSnapshotID == first.Record.DatasetSnapshotID ||
		cleaned.Record.PredecessorRecordDigest != first.Record.RecordDigest ||
		heads.current.DatasetSnapshotCount != 2 || heads.current.EvidenceRegistryCount != 0 ||
		heads.current.PublicationCount != 0 {
		t.Fatal("cleaning did not preserve the exact predecessor and single-child transition")
	}
	before := heads.current
	if _, err := service.AdmitAfterExactV2(ctx, AdmitAfterInputV2{
		AdmitInputV2: input, ExpectedCurrentDatasetSnapshotID: first.Record.DatasetSnapshotID,
	}); !errors.Is(err, datasetsnapshotport.ErrStale) {
		t.Fatalf("a retry with an old predecessor was not refused: %v", err)
	}
	if heads.current != before || len(fixture.indexes.records) != 2 || len(fixture.bundles.records) != 2 {
		t.Fatal("refused stale cleaning wrote an inert candidate or changed the selected head")
	}
	restarted := hostLocalExpectedOpenV2(t, fixture, heads)
	selected, err := restarted.ResolveWitnessedV2(ctx, fixture.resolveInput(cleaned))
	if err != nil || selected.Record != cleaned.Record || selected.Manifest != cleaned.Manifest {
		t.Fatalf("cleaned snapshot could not be resolved through the production authority: %v", err)
	}
}

func TestHostLocalSealedV2ExpectedPredecessorRejectsForeignScopeAndCancellation(t *testing.T) {
	service, fixture, heads := hostLocalExpectedPredecessorFixtureV2(t)
	first, err := service.AdmitExactV2(context.Background(), fixture.admitInput())
	if err != nil {
		t.Fatal(err)
	}
	input := hostLocalExpectedSuccessorInputV2(t, fixture, first, 2)
	before := heads.current
	for _, scope := range []string{"tenant", "user", "case", "binding"} {
		t.Run(scope, func(t *testing.T) {
			foreign := input
			if scope == "tenant" {
				foreign.TenantID += "-other"
			} else if scope == "user" {
				foreign.UserID += "-other"
			} else {
				observation := foreign.Observation
				if scope == "case" {
					observation.CaseID += "-other"
				} else {
					observation.BindingSHA256 = domainsecurity.SHA256Hex([]byte("other binding"))
					observation.CaseBindingHash = domainsecurity.SHA256Hex([]byte("other marker"))
				}
				foreign.Observation, err = domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
					WorkspaceRealPath: observation.WorkspaceRealPath, State: observation.State,
					CaseID: observation.CaseID, BindingSHA256: observation.BindingSHA256,
					CaseBindingHash: observation.CaseBindingHash,
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := service.AdmitAfterExactV2(context.Background(), AdmitAfterInputV2{
				AdmitInputV2: foreign, ExpectedCurrentDatasetSnapshotID: first.Record.DatasetSnapshotID,
			}); !errors.Is(err, datasetsnapshotport.ErrStale) && !errors.Is(err, datasetsnapshotport.ErrMismatch) {
				t.Fatalf("foreign-scope predecessor was not refused: %v", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.AdmitAfterExactV2(ctx, AdmitAfterInputV2{
		AdmitInputV2: input, ExpectedCurrentDatasetSnapshotID: first.Record.DatasetSnapshotID,
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled cleaning did not preserve cancellation: %v", err)
	}
	if heads.current != before || len(fixture.indexes.records) != 1 || len(fixture.bundles.records) != 1 {
		t.Fatal("foreign scope or cancelled cleaning changed protected state")
	}
}

func TestHostLocalSealedV2ExpectedPredecessorRejectsMissingAndMalformedInput(t *testing.T) {
	service, fixture, heads := hostLocalExpectedPredecessorFixtureV2(t)
	expected := domainsecurity.DatasetSnapshotIDPrefixV2 + domainsecurity.SHA256Hex([]byte("missing snapshot"))
	if _, err := service.AdmitAfterExactV2(context.Background(), AdmitAfterInputV2{
		AdmitInputV2: fixture.admitInput(), ExpectedCurrentDatasetSnapshotID: expected,
	}); !errors.Is(err, datasetsnapshotport.ErrStale) {
		t.Fatalf("cleaning without an admitted predecessor was not refused: %v", err)
	}
	for _, invalid := range []string{"", "dsv2_not-a-digest", " " + expected} {
		if _, err := service.AdmitAfterExactV2(context.Background(), AdmitAfterInputV2{
			AdmitInputV2: fixture.admitInput(), ExpectedCurrentDatasetSnapshotID: invalid,
		}); !errors.Is(err, datasetsnapshotport.ErrMismatch) {
			t.Fatalf("invalid expected predecessor was not rejected: %v", err)
		}
	}
	if heads.current != heads.genesis || len(fixture.indexes.records) != 0 || len(fixture.bundles.records) != 0 {
		t.Fatal("rejected predecessor input caused a protected write")
	}
}

func TestHostLocalSealedV2ExpectedPredecessorSerializesCompetingCleaning(t *testing.T) {
	service, fixture, heads := hostLocalExpectedPredecessorFixtureV2(t)
	first, err := service.AdmitExactV2(context.Background(), fixture.admitInput())
	if err != nil {
		t.Fatal(err)
	}
	inputs := []AdmitInputV2{
		hostLocalExpectedSuccessorInputV2(t, fixture, first, 2),
		hostLocalExpectedSuccessorInputV2(t, fixture, first, 3),
	}
	start := make(chan struct{})
	results := make(chan error, len(inputs))
	for _, input := range inputs {
		go func(input AdmitInputV2) {
			<-start
			_, err := service.AdmitAfterExactV2(context.Background(), AdmitAfterInputV2{
				AdmitInputV2: input, ExpectedCurrentDatasetSnapshotID: first.Record.DatasetSnapshotID,
			})
			results <- err
		}(input)
	}
	close(start)
	success, stale := 0, 0
	for range inputs {
		switch err := <-results; {
		case err == nil:
			success++
		case errors.Is(err, datasetsnapshotport.ErrStale):
			stale++
		default:
			t.Fatalf("unexpected competing cleaning result: %v", err)
		}
	}
	if success != 1 || stale != 1 || heads.current.DatasetSnapshotCount != 2 ||
		len(fixture.indexes.records) != 2 || len(fixture.bundles.records) != 2 {
		t.Fatalf("competing cleaning did not commit exactly once without residue: success=%d stale=%d", success, stale)
	}
}

func hostLocalExpectedPredecessorFixtureV2(t *testing.T) (*HostLocalSealedServiceV2, *sealedServiceFixtureV2, *hostLocalHeadCoordinatorFixture) {
	t.Helper()
	fixture := newSealedServiceFixtureV2(t)
	genesis, err := domainhost.NewHeadV1(domainhost.HeadInputV1{
		InstallationID:              fixture.coordinator.installationID,
		RootBindingDigest:           domainsecurity.SHA256Hex([]byte("protected roots")),
		MutationID:                  domainsecurity.SHA256Hex([]byte("host-local genesis")),
		DatasetSnapshotIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
		EvidenceRegistryIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
		PublicationIndexDigest:      domainpublication.PublicationIndexGenesisDigestV1(),
		AuthorityKeyID:              fixture.authority.KeyID(), AuthorityPublicKey: fixture.authority.PublicKey(),
	}, func(body []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), body) })
	if err != nil {
		t.Fatal(err)
	}
	heads := &hostLocalHeadCoordinatorFixture{genesis: genesis, current: genesis}
	return hostLocalExpectedOpenV2(t, fixture, heads), fixture, heads
}

func hostLocalExpectedOpenV2(t *testing.T, fixture *sealedServiceFixtureV2, heads *hostLocalHeadCoordinatorFixture) *HostLocalSealedServiceV2 {
	t.Helper()
	service, err := NewHostLocalSealedV2(context.Background(), HostLocalSealedConfigV2{
		InstallationID: fixture.coordinator.installationID, Authority: fixture.authority,
		Heads: heads, LegacyRecords: fixture.legacy, Bundles: fixture.bundles,
		Indexes: fixture.indexes, Materials: fixture.materials, Random: &datasetSnapshotCounterReader{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func hostLocalExpectedSuccessorInputV2(t *testing.T, fixture *sealedServiceFixtureV2,
	predecessor datasetsnapshotport.ResolvedSnapshotV2, revision uint64) AdmitInputV2 {
	t.Helper()
	producer := fixture.producer
	producer.SourceRevision = revision
	producerBody, err := domainsecurity.FundsProducerContentManifestV1Bytes(producer)
	if err != nil {
		t.Fatal(err)
	}
	producerDigest := domainsecurity.SHA256Hex(producerBody)
	fixture.materials.values[datasetsnapshotport.MaterialFundsProducerContentV1][producerDigest] = producerBody
	manifestInput, err := fixturev2.CloneManifestInputV2(predecessor)
	if err != nil {
		t.Fatal(err)
	}
	manifestInput.FundsProducerContentManifest = producer
	manifest, err := domainsecurity.NewDatasetSnapshotManifestV2(manifestInput)
	if err != nil {
		t.Fatal(err)
	}
	manifestBody, err := domainsecurity.DatasetSnapshotManifestV2Bytes(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest := domainsecurity.SHA256Hex(manifestBody)
	fixture.materials.values[datasetsnapshotport.MaterialSnapshotManifestV2][manifestDigest] = manifestBody
	input := fixture.admitInput()
	input.ManifestReference = exactReferenceV2(manifestDigest, uint64(len(manifestBody)))
	input.FundsProducerReference = exactReferenceV2(producerDigest, uint64(len(producerBody)))
	input.AcceptedAt = time.Date(2026, 7, 21, 9, 30, 0, 0, time.UTC)
	return input
}
