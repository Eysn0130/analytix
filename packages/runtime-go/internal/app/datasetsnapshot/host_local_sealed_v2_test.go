package datasetsnapshot

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainhost "analytix.local/runtime-go/internal/domain/hostcurrentness"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	fixturev2 "analytix.local/runtime-go/internal/testsupport/datasetsnapshotv2fixture"
)

type hostLocalHeadCoordinatorFixture struct {
	mu          sync.Mutex
	admissionMu sync.Mutex
	genesis     domainhost.HeadV1
	current     domainhost.HeadV1
}

func (fixture *hostLocalHeadCoordinatorFixture) Current(context.Context) (domainhost.HeadV1, bool, error) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.current, true, nil
}

func (fixture *hostLocalHeadCoordinatorFixture) CurrentModeCommitment(context.Context) (domainhost.HeadV1, error) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.genesis, nil
}

func (fixture *hostLocalHeadCoordinatorFixture) AdvanceExact(_ context.Context, expected, next domainhost.HeadV1) error {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.current != expected || domainhost.ValidateHeadTransitionV1(expected, next) != nil {
		return errors.New("host-local head was stale or invalid")
	}
	fixture.current = next
	return nil
}

func (fixture *hostLocalHeadCoordinatorFixture) WithProtectedMutation(ctx context.Context, use func(context.Context, evidenceauthorityport.HostLocalMutation) error) error {
	fixture.admissionMu.Lock()
	defer fixture.admissionMu.Unlock()
	return use(ctx, fixture)
}

func TestHostLocalSealedV2AdmitResolveRestartAndModeRejection(t *testing.T) {
	ctx := context.Background()
	fixture := newSealedServiceFixtureV2(t)
	genesis, err := domainhost.NewHeadV1(domainhost.HeadInputV1{
		InstallationID:              fixture.coordinator.installationID,
		RootBindingDigest:           domainsecurity.SHA256Hex([]byte("protected roots")),
		MutationID:                  domainsecurity.SHA256Hex([]byte("host-local genesis")),
		DatasetSnapshotIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
		EvidenceRegistryIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
		PublicationIndexDigest:      domainpublication.PublicationIndexGenesisDigestV1(),
		AuthorityKeyID:              fixture.authority.KeyID(), AuthorityPublicKey: fixture.authority.PublicKey(),
	}, func(body []byte) ([]byte, error) { return fixture.authority.Sign(ctx, body) })
	if err != nil {
		t.Fatal(err)
	}
	heads := &hostLocalHeadCoordinatorFixture{genesis: genesis, current: genesis}
	random := &datasetSnapshotCounterReader{}
	open := func() *HostLocalSealedServiceV2 {
		t.Helper()
		service, err := NewHostLocalSealedV2(ctx, HostLocalSealedConfigV2{
			InstallationID: fixture.coordinator.installationID,
			Authority:      fixture.authority, Heads: heads, LegacyRecords: fixture.legacy,
			Bundles: fixture.bundles, Indexes: fixture.indexes, Materials: fixture.materials,
			Random: random,
		})
		if err != nil {
			t.Fatal(err)
		}
		return service
	}
	service := open()
	resolved, err := service.AdmitExactV2(ctx, fixture.admitInput())
	if err != nil || resolved.Record.DatasetSnapshotID == "" {
		t.Fatalf("host-local DSV2 admission failed: %v", err)
	}
	if heads.current.DatasetSnapshotCount != 1 || heads.current.EvidenceRegistryCount != 0 {
		t.Fatal("host-local admission advanced a wrong child")
	}
	index := fixture.indexes.records[heads.current.DatasetSnapshotIndexDigest]
	if index.SchemaVersion != domainsecurity.DatasetSnapshotIndexHostLocalSchemaVersionV2 ||
		index.ModeCommitmentDigest != genesis.RecordDigest || index.EnrollmentID != "" ||
		domainsecurity.ValidateDatasetSnapshotIndexV1(index) == nil {
		t.Fatal("host-local DSV2 index was mislabelled as witnessed")
	}
	restarted := open()
	current, err := restarted.ResolveWitnessedV2(ctx, fixture.resolveInput(resolved))
	if err != nil || current.Record != resolved.Record || current.Manifest != resolved.Manifest {
		t.Fatalf("host-local DSV2 restart readback lost exact material: %v", err)
	}
	securityContext := fixture.securityContext(t, resolved, "host-local-current-selection")
	var leaked datasetsnapshotport.CurrentSelectionCapabilityV2
	var selected datasetsnapshotport.CurrentSelectionV2
	if err := restarted.WithCurrentSelectionV2(ctx, fixture.resolveInput(resolved), securityContext,
		func(selection datasetsnapshotport.CurrentSelectionV2, capability datasetsnapshotport.CurrentSelectionCapabilityV2) error {
			if selection.HostLocalHead == nil || selection.HostLocalHead.RecordDigest != heads.current.RecordDigest ||
				selection.Head.HasBundle || selection.SelectedIndex.Mode != "host_local" {
				t.Fatal("host-local current selection was confused with a witnessed selection")
			}
			leaked, selected = capability, selection
			return capability.UseExact(selection, securityContext, func(context.Context) error { return nil })
		}); err != nil {
		t.Fatalf("host-local exact current selection failed: %v", err)
	}
	if leaked == nil || leaked.UseExact(selected, securityContext, func(context.Context) error { return nil }) == nil {
		t.Fatal("host-local current-selection capability survived its callback")
	}
	producer := fixture.producer
	producer.SourceRevision++
	producerBody, err := domainsecurity.FundsProducerContentManifestV1Bytes(producer)
	if err != nil {
		t.Fatal(err)
	}
	producerDigest := domainsecurity.SHA256Hex(producerBody)
	fixture.materials.values[datasetsnapshotport.MaterialFundsProducerContentV1][producerDigest] = producerBody
	manifestInput, err := fixturev2.CloneManifestInputV2(resolved)
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
	nextInput := fixture.admitInput()
	nextInput.ManifestReference = exactReferenceV2(manifestDigest, uint64(len(manifestBody)))
	nextInput.FundsProducerReference = exactReferenceV2(producerDigest, uint64(len(producerBody)))
	nextInput.AcceptedAt = time.Date(2026, 7, 21, 9, 30, 0, 0, time.UTC)
	newer, err := restarted.AdmitExactV2(ctx, nextInput)
	if err != nil || newer.Record.DatasetSnapshotID == resolved.Record.DatasetSnapshotID {
		t.Fatalf("host-local second exact DSV2 admission failed: %v", err)
	}
	newerContext := fixture.securityContext(t, newer, "host-local-retained-selection")
	retainedCalls := 0
	if err := restarted.WithRetainedSelectionV2(ctx, datasetsnapshotport.RetainedSelectionInputV2{
		CurrentResolveInput: fixture.resolveInput(newer), CurrentSecurityContext: newerContext,
		RetainedDatasetSnapshotID:  resolved.Record.DatasetSnapshotID,
		RetainedSourceManifestHash: resolved.Record.SourceManifestHash,
	}, func(_ context.Context, retained datasetsnapshotport.RetainedSelectionV2) error {
		retainedCalls++
		if retained.Snapshot.Record != resolved.Record || retained.Current.Snapshot.Record != newer.Record {
			t.Fatal("host-local retained selection returned a detached material graph")
		}
		return nil
	}); err != nil || retainedCalls != 1 {
		t.Fatalf("host-local retained current-path read failed: calls=%d err=%v", retainedCalls, err)
	}
	wrong := fixture.resolveInput(resolved)
	wrong.ExpectedDatasetSnapshotID = "dsv2_" + domainsecurity.SHA256Hex([]byte("other snapshot"))
	if _, err := restarted.ResolveWitnessedV2(ctx, wrong); err == nil {
		t.Fatal("host-local DSV2 accepted caller-selected stale snapshot")
	}
	index.Mode = "witnessed"
	fixture.indexes.records[heads.current.DatasetSnapshotIndexDigest] = index
	if _, err := restarted.ResolveWitnessedV2(ctx, fixture.resolveInput(resolved)); err == nil {
		t.Fatal("host-local DSV2 accepted cross-labelled current index")
	}
}
