//go:build darwin || linux

package runtimeapp

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	datasetsnapshotstore "analytix.local/runtime-go/internal/adapters/outbound/datasetsnapshot"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	fundsquerysourceadapter "analytix.local/runtime-go/internal/adapters/outbound/fundsquerysource"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	datasetsnapshotapp "analytix.local/runtime-go/internal/app/datasetsnapshot"
	fundsquerysourceapp "analytix.local/runtime-go/internal/app/fundsquerysource"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	fixturev2 "analytix.local/runtime-go/internal/testsupport/datasetsnapshotv2fixture"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

// The manifest successor is synthetic; this tests actual protected owner/store
// admission and disk reopen, not a cleaning transform, turn epoch, or process/UI restart.
func TestRuntimeHostLocalExpectedPredecessorPersistsSuccessorAndRejectsStaleAfterReopen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	config := Config{DataDir: t.TempDir(), DurableTempDir: t.TempDir(), UserDataDir: t.TempDir()}
	privateRoot := filepath.Join(config.DataDir, "private")
	snapshotRoot := filepath.Join(privateRoot, "dataset-snapshot-authority")
	access, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(privateRoot, "authority", "final-answer-ed25519-v1.json")
	key, err := finalauthority.OpenOrCreateFileAuthority(keyPath, false)
	if err != nil {
		t.Fatal(err)
	}
	freeze := func() *persistencefs.RootAuthority {
		t.Helper()
		root, err := persistencefs.FreezeRootAuthority(persistencefs.RootSet{
			DataDir: config.DataDir, DurableDir: config.DurableTempDir,
		})
		if err != nil {
			t.Fatal(err)
		}
		return root
	}
	stores, err := datasetsnapshotstore.OpenStoresV2(snapshotRoot, access)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := newRuntimeHostLocalEvidenceOwnersV1(config, freeze(), access, key, stores,
		runtimeHostLocalProfileAdmitV1, func(context.Context) error { return nil })
	if err != nil {
		_ = stores.Close()
		t.Fatal(err)
	}
	defer func() {
		if owner != nil {
			if err := owner.Close(); err != nil {
				t.Error(err)
			}
		}
		if stores != nil {
			if err := stores.Close(); err != nil {
				t.Error(err)
			}
		}
	}()
	if err := owner.EnsureForFundsImport(ctx); err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	runtimeSharedEvidenceWriteCaseBindingV2(t, workspace)
	observation, err := (filestore.CaseBindingReader{}).Observe(workspace)
	if err != nil {
		t.Fatal(err)
	}
	manifest, producer, materials := runtimeSharedEvidenceBoundMaterialsV2(t, workspace, observation, key.KeyID(), key)
	putMaterials := func(values map[datasetsnapshotport.MaterialKindV2]map[string][]byte) {
		t.Helper()
		materialCAS, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(
			filepath.Join(snapshotRoot, "materials"), 16*1024*1024, access)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := materialCAS.Close(); err != nil {
				t.Error(err)
			}
		}()
		for _, records := range values {
			for address, body := range records {
				if err := materialCAS.PutIfAbsent(ctx, address, body); err != nil && !errors.Is(err, os.ErrExist) {
					t.Fatal(err)
				}
			}
		}
	}
	putMaterials(materials)
	input := datasetsnapshotapp.AdmitInputV2{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		Observation: observation, ManifestReference: manifest, FundsProducerReference: producer,
		AcceptedAt: time.Date(2026, 9, 29, 8, 0, 0, 0, time.UTC),
	}
	first, err := owner.AdmitExactV2(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	manifestInput, err := fixturev2.CloneManifestInputV2(first)
	if err != nil {
		t.Fatal(err)
	}
	manifestInput.AnalyticalDuckDB = first.Manifest.AnalyticalDuckDB
	manifestInput.FundsProducerContentManifest.SourceRevision++
	producerBody, err := domainsecurity.FundsProducerContentManifestV1Bytes(manifestInput.FundsProducerContentManifest)
	if err != nil {
		t.Fatal(err)
	}
	secondManifest, err := domainsecurity.NewDatasetSnapshotManifestV2(manifestInput)
	if err != nil {
		t.Fatal(err)
	}
	manifestBody, err := domainsecurity.DatasetSnapshotManifestV2Bytes(secondManifest)
	if err != nil {
		t.Fatal(err)
	}
	producerDigest, manifestDigest := domainsecurity.SHA256Hex(producerBody), domainsecurity.SHA256Hex(manifestBody)
	putMaterials(map[datasetsnapshotport.MaterialKindV2]map[string][]byte{
		datasetsnapshotport.MaterialFundsProducerContentV1: {producerDigest: producerBody},
		datasetsnapshotport.MaterialSnapshotManifestV2:     {manifestDigest: manifestBody},
	})
	input.ManifestReference = datasetsnapshotport.ExactMaterialReferenceV2{Address: manifestDigest, SHA256: manifestDigest, ByteLength: uint64(len(manifestBody))}
	input.FundsProducerReference = datasetsnapshotport.ExactMaterialReferenceV2{Address: producerDigest, SHA256: producerDigest, ByteLength: uint64(len(producerBody))}
	input.AcceptedAt = input.AcceptedAt.Add(time.Hour)
	afterInput := datasetsnapshotapp.AdmitAfterInputV2{AdmitInputV2: input, ExpectedCurrentDatasetSnapshotID: first.Record.DatasetSnapshotID}
	parent, found, err := owner.heads.Current(ctx)
	if err != nil || !found {
		t.Fatalf("successor predecessor head is unavailable: %v", err)
	}
	second, err := owner.AdmitAfterExactV2(ctx, afterInput)
	if err != nil || second.Record.DatasetSnapshotID == first.Record.DatasetSnapshotID ||
		second.Record.PredecessorRecordDigest != first.Record.RecordDigest {
		t.Fatalf("actual host owner did not admit the exact successor: %v", err)
	}
	before, found, err := owner.heads.Current(ctx)
	if err != nil || !found || before.DatasetSnapshotCount != parent.DatasetSnapshotCount+1 ||
		before.Generation != parent.Generation+1 || before.DatasetSnapshotIndexDigest == parent.DatasetSnapshotIndexDigest ||
		before.EvidenceRegistryCount != parent.EvidenceRegistryCount || before.PublicationCount != parent.PublicationCount ||
		before.EvidenceRegistryIndexDigest != parent.EvidenceRegistryIndexDigest || before.PublicationIndexDigest != parent.PublicationIndexDigest {
		t.Fatalf("successor changed the wrong signed child root: %v", err)
	}
	beforeFiles := runtimeHostLocalCleaningStoreBytesV2(t, snapshotRoot)
	if _, err := owner.AdmitAfterExactV2(ctx, afterInput); !errors.Is(err, datasetsnapshotport.ErrStale) {
		t.Fatalf("stale predecessor retry was not refused: %v", err)
	}
	if !reflect.DeepEqual(beforeFiles, runtimeHostLocalCleaningStoreBytesV2(t, snapshotRoot)) {
		t.Fatal("stale predecessor retry wrote a dataset record, index or material")
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	owner = nil
	if err := stores.Close(); err != nil {
		t.Fatal(err)
	}
	stores = nil
	access, err = privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	key, err = finalauthority.OpenOrCreateFileAuthority(keyPath, false)
	if err != nil {
		t.Fatal(err)
	}
	stores, err = datasetsnapshotstore.OpenStoresV2(snapshotRoot, access)
	if err != nil {
		t.Fatal(err)
	}
	owner, err = newRuntimeHostLocalEvidenceOwnersV1(config, freeze(), access, key, stores,
		runtimeHostLocalProfileResumeV1, func(context.Context) error { t.Fatal("reopen re-entered fresh admission"); return nil })
	if err != nil {
		t.Fatal(err)
	}
	resolve := datasetsnapshotport.ResolveInputV2{TenantID: input.TenantID, UserID: input.UserID,
		Observation: observation, ExpectedDatasetSnapshotID: second.Record.DatasetSnapshotID}
	selected, err := owner.ResolveWitnessedV2(ctx, resolve)
	if err != nil || selected.Record != second.Record || selected.Manifest != second.Manifest {
		t.Fatalf("disk reopen did not preserve the exact selected successor: %v", err)
	}
	resolve.ExpectedDatasetSnapshotID = first.Record.DatasetSnapshotID
	if _, err := owner.ResolveWitnessedV2(ctx, resolve); !errors.Is(err, datasetsnapshotport.ErrStale) {
		t.Fatalf("disk reopen accepted the old snapshot as current: %v", err)
	}
	if _, err := owner.AdmitAfterExactV2(ctx, afterInput); !errors.Is(err, datasetsnapshotport.ErrStale) {
		t.Fatalf("disk reopen accepted the old expected predecessor: %v", err)
	}
	hostSource, err := fundsquerysourceadapter.NewHostExactSource(config.UserDataDir)
	if err != nil {
		t.Fatal(err)
	}
	query, err := fundsquerysourceapp.NewService(owner, filestore.CaseBindingReader{}, stores.Materials, hostSource)
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := query.ResolveCurrentLocalDisplay(ctx, workspace, input.TenantID, input.UserID)
	if err != nil || descriptor.DatasetSnapshotID != second.Record.DatasetSnapshotID || descriptor.SourceManifestHash != second.Record.SourceManifestHash {
		t.Fatalf("query source did not follow the reopened successor: %v", err)
	}
	after, found, err := owner.heads.Current(ctx)
	if err != nil || !found || after != before || !reflect.DeepEqual(beforeFiles, runtimeHostLocalCleaningStoreBytesV2(t, snapshotRoot)) {
		t.Fatalf("reopen or stale requests changed the signed head or dataset store: %v", err)
	}
}

func runtimeHostLocalCleaningStoreBytesV2(t *testing.T, root string) map[string]string {
	t.Helper()
	values := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		values[relative] = domainsecurity.SHA256Hex(body)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return values
}
