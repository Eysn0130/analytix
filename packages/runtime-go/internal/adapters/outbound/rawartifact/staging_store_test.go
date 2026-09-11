//go:build !analytix_prod

package rawartifact

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	rawartifactport "analytix.local/runtime-go/internal/ports/rawartifact"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	rawartifactfixture "analytix.local/runtime-go/internal/testsupport/rawartifactfixture"
)

func TestRawArtifactStagingStoreExactRoundTripRestartAndConcurrentIdempotence(t *testing.T) {
	hierarchy, err := rawartifactfixture.BuildV1("store", 2)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "raw-artifact-staging")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStagingStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := stageRawArtifactHierarchyForTestV1(context.Background(), store, hierarchy); err != nil {
		t.Fatal(err)
	}
	manifestCanonical, _ := domainevidence.RawArtifactManifestV1Bytes(hierarchy.Manifest)
	manifestReference := rawartifactport.ExactObjectReferenceV1{
		Digest: hierarchy.Manifest.ManifestDigest, SHA256: domainsecurity.SHA256Hex(manifestCanonical),
		ByteLength: uint64(len(manifestCanonical)),
	}
	resolvedManifest, err := store.ResolveManifestExact(context.Background(), manifestReference)
	if err != nil || resolvedManifest.ManifestDigest != hierarchy.Manifest.ManifestDigest {
		t.Fatalf("manifest exact resolve failed: manifest=%#v err=%v", resolvedManifest, err)
	}
	if _, err := store.ResolveAcquisitionIntentExact(context.Background(), hierarchy.Manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveManifestPageExact(context.Background(), hierarchy.Manifest.PageDescriptors[0]); err != nil {
		t.Fatal(err)
	}
	entryCanonical, _ := domainevidence.RawArtifactEntryV1Bytes(hierarchy.Entries[0])
	if _, err := store.ResolveEntryExact(context.Background(), rawartifactport.ExactObjectReferenceV1{
		Digest: hierarchy.Entries[0].EntryDigest, SHA256: domainsecurity.SHA256Hex(entryCanonical),
		ByteLength: uint64(len(entryCanonical)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveSourceLocatorExact(context.Background(), hierarchy.Entries[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveContentRootExact(context.Background(), hierarchy.Entries[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveContentIndexPageExact(
		context.Background(), hierarchy.ContentRoots[0].IndexPageDescriptors[0],
	); err != nil {
		t.Fatal(err)
	}
	if body, err := store.ResolveChunkExact(
		context.Background(), hierarchy.ContentPages[0].ChunkDescriptors[0],
	); err != nil || !bytes.Equal(body, hierarchy.ChunkBodies[0]) {
		t.Fatalf("chunk exact resolve failed: bytes=%d err=%v", len(body), err)
	}
	wrongPhysical := manifestReference
	wrongPhysical.SHA256 = domainsecurity.SHA256Hex([]byte("other-manifest-body"))
	if _, err := store.ResolveManifestExact(context.Background(), wrongPhysical); err == nil {
		t.Fatal("manifest resolved through a mismatched physical SHA")
	}

	var wait sync.WaitGroup
	errorsOut := make(chan error, 32)
	for range 16 {
		wait.Add(2)
		go func() {
			defer wait.Done()
			errorsOut <- store.StageChunkExact(context.Background(), hierarchy.ContentPages[0].ChunkDescriptors[0], hierarchy.ChunkBodies[0])
		}()
		go func() {
			defer wait.Done()
			errorsOut <- store.StageManifestExact(context.Background(), hierarchy.Manifest)
		}()
	}
	wait.Wait()
	close(errorsOut)
	for err := range errorsOut {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewStagingStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restarted.Close() }()
	if err := stageRawArtifactHierarchyForTestV1(context.Background(), restarted, hierarchy); err != nil {
		t.Fatalf("restarted exact staging was not idempotent: %v", err)
	}
	manifestBody, err := restarted.manifests.Read(context.Background(), hierarchy.Manifest.ManifestDigest)
	expectedManifestBody, expectedErr := domainevidence.RawArtifactManifestV1Bytes(hierarchy.Manifest)
	if err != nil || expectedErr != nil || !bytes.Equal(manifestBody, expectedManifestBody) {
		t.Fatalf("restarted manifest CAS readback failed: bytes=%d err=%v", len(manifestBody), err)
	}
}

func TestRawArtifactStagingStoreRejectsCorruptionCancellationAndSelectors(t *testing.T) {
	hierarchy, err := rawartifactfixture.BuildV1("store-hostile", 1)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "raw-artifact-staging")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStagingStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	corrupt := append([]byte(nil), hierarchy.ChunkBodies[0]...)
	corrupt[0] ^= 0xff
	if err := store.StageChunkExact(context.Background(), hierarchy.ContentPages[0].ChunkDescriptors[0], corrupt); err == nil {
		t.Fatal("corrupt chunk entered raw artifact private CAS")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.StageManifestExact(canceled, hierarchy.Manifest); !errors.Is(err, context.Canceled) {
		t.Fatalf("staging store hid cancellation: %v", err)
	}
	manifestBody, _ := domainevidence.RawArtifactManifestV1Bytes(hierarchy.Manifest)
	if _, err := store.ResolveManifestExact(canceled, rawartifactport.ExactObjectReferenceV1{
		Digest: hierarchy.Manifest.ManifestDigest, SHA256: domainsecurity.SHA256Hex(manifestBody), ByteLength: uint64(len(manifestBody)),
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("exact reader hid cancellation: %v", err)
	}
	for _, name := range []string{"List", "Current", "Latest", "Active", "ResolveCurrent", "ResolveLatest"} {
		if _, found := reflect.TypeOf(store).MethodByName(name); found {
			t.Fatalf("raw artifact staging store exposes forbidden selector %s", name)
		}
	}
	if _, err := NewStagingStore(" ", nil); err == nil {
		t.Fatal("invalid raw artifact staging root was accepted")
	}
}

func stageRawArtifactHierarchyForTestV1(
	ctx context.Context,
	store *StagingStore,
	hierarchy rawartifactfixture.HierarchyV1,
) error {
	if err := store.StageAcquisitionIntentExact(ctx, hierarchy.Intent); err != nil {
		return err
	}
	for index := range hierarchy.Entries {
		if err := store.StageSourceLocatorExact(ctx, hierarchy.Locators[index]); err != nil {
			return err
		}
		if err := store.StageChunkExact(ctx, hierarchy.ContentPages[index].ChunkDescriptors[0], hierarchy.ChunkBodies[index]); err != nil {
			return err
		}
		if err := store.StageContentIndexPageExact(ctx, hierarchy.ContentPages[index]); err != nil {
			return err
		}
		if err := store.StageContentRootExact(ctx, hierarchy.ContentRoots[index]); err != nil {
			return err
		}
		if err := store.StageEntryExact(ctx, hierarchy.Entries[index]); err != nil {
			return err
		}
	}
	for _, page := range hierarchy.ManifestPages {
		if err := store.StageManifestPageExact(ctx, page); err != nil {
			return err
		}
	}
	return store.StageManifestExact(ctx, hierarchy.Manifest)
}
