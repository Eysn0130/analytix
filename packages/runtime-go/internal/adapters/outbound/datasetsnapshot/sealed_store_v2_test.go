package datasetsnapshot

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	fixturev2 "analytix.local/runtime-go/internal/testsupport/datasetsnapshotv2fixture"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestDatasetSnapshotMaterialStoreV2ExactReadOnlyPrivateCAS(t *testing.T) {
	fixture, err := fixturev2.Load()
	if err != nil {
		t.Fatal(err)
	}
	body := fixture.Materials[datasetsnapshotport.MaterialRawContentChunkV1]
	var digest string
	var raw []byte
	for digest, raw = range body {
	}
	root := filepath.Join(t.TempDir(), "raw-chunks")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	cas, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(root, 16<<20, access)
	if err != nil {
		t.Fatal(err)
	}
	defer cas.Close()
	if err := cas.PutIfAbsent(context.Background(), digest, raw); err != nil {
		t.Fatal(err)
	}
	store, err := NewMaterialStoreV2(map[datasetsnapshotport.MaterialKindV2]*finalauthorityadapter.SecurePrivateCAS{
		datasetsnapshotport.MaterialRawContentChunkV1: cas,
	})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := store.ResolveExact(context.Background(), datasetsnapshotport.MaterialRawContentChunkV1,
		datasetsnapshotport.ExactMaterialReferenceV2{Address: digest, SHA256: digest, ByteLength: uint64(len(raw))})
	if err != nil || !bytes.Equal(resolved, raw) {
		t.Fatalf("exact private CAS material did not round trip: %v", err)
	}
	if _, err := store.ResolveExact(context.Background(), datasetsnapshotport.MaterialRawContentChunkV1,
		datasetsnapshotport.ExactMaterialReferenceV2{Address: digest, SHA256: digest, ByteLength: uint64(len(raw) + 1)}); !errors.Is(err, datasetsnapshotport.ErrCorrupt) {
		t.Fatalf("wrong exact length was not rejected: %v", err)
	}
	if _, err := store.ResolveExact(context.Background(), datasetsnapshotport.MaterialParsedPageV1,
		datasetsnapshotport.ExactMaterialReferenceV2{Address: digest, SHA256: digest, ByteLength: uint64(len(raw))}); err == nil {
		t.Fatal("unconfigured material kind was resolved")
	}
}

func TestDatasetSnapshotMaterialStoreV2CanonicalCSVClosedKindRoundTrip(t *testing.T) {
	expectedKinds := []datasetsnapshotport.MaterialKindV2{
		datasetsnapshotport.MaterialParsedIdentityV1,
		datasetsnapshotport.MaterialSourceRowRecordV1,
	}
	configuredKinds := materialKindsV2()
	for _, expected := range expectedKinds {
		count := 0
		for _, configured := range configuredKinds {
			if configured == expected {
				count++
			}
		}
		if count != 1 || !validMaterialKindV2(expected) {
			t.Fatalf("canonical CSV material kind is not configured exactly once: kind=%q count=%d", expected, count)
		}
	}

	root := filepath.Join(t.TempDir(), "canonical-csv-materials")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	cas, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(root, maxAdmissionMaterialBytesV2, access)
	if err != nil {
		t.Fatal(err)
	}
	defer cas.Close()
	store, err := NewMaterialStoreV2(map[datasetsnapshotport.MaterialKindV2]*finalauthorityadapter.SecurePrivateCAS{
		datasetsnapshotport.MaterialParsedIdentityV1:  cas,
		datasetsnapshotport.MaterialSourceRowRecordV1: cas,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range expectedKinds {
		body := []byte("opaque-canonical-csv-material:" + kind)
		digest := domainsecurity.SHA256Hex(body)
		if err := cas.PutIfAbsent(context.Background(), digest, body); err != nil {
			t.Fatal(err)
		}
		resolved, err := store.ResolveExact(context.Background(), kind, datasetsnapshotport.ExactMaterialReferenceV2{
			Address: digest, SHA256: digest, ByteLength: uint64(len(body)),
		})
		if err != nil || !bytes.Equal(resolved, body) {
			t.Fatalf("canonical CSV material did not round trip through the shared private CAS: kind=%q err=%v", kind, err)
		}
	}

	unknown := datasetsnapshotport.MaterialKindV2("unknown-canonical-csv-material-v1")
	if validMaterialKindV2(unknown) {
		t.Fatal("unknown material kind entered the closed production vocabulary")
	}
	if _, err := NewMaterialStoreV2(map[datasetsnapshotport.MaterialKindV2]*finalauthorityadapter.SecurePrivateCAS{
		unknown: cas,
	}); err == nil {
		t.Fatal("unknown material kind was configured")
	}
}

func TestDatasetSnapshotAuthorityBundleStoreV2AtomicConcurrentRestart(t *testing.T) {
	fixture, err := fixturev2.Load()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := domainsecurity.ParseDatasetSnapshotManifestV2(
		fixture.Materials[datasetsnapshotport.MaterialSnapshotManifestV2][fixture.ManifestReference.Address],
	)
	if err != nil {
		t.Fatal(err)
	}
	producer, err := domainsecurity.ParseFundsProducerContentManifestV1(
		fixture.Materials[datasetsnapshotport.MaterialFundsProducerContentV1][fixture.ProducerReference.Address],
	)
	if err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x68}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	var record domainsecurity.DatasetSnapshotAuthorityRecordV2
	err = manifest.WithExactFundsProducerAuthorityAdmissionV2(producer, func(
		issuer domainsecurity.DatasetSnapshotAuthoritySealedAdmissionV2,
	) error {
		issued, issueErr := issuer.Issue(domainsecurity.DatasetSnapshotAuthoritySealedIssueInputV2{
			InstallationID: domainsecurity.SHA256Hex([]byte("sealed-bundle-installation")),
			AcceptedAt:     time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC),
			AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
		}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
		record = issued
		return issueErr
	})
	if err != nil {
		t.Fatal(err)
	}
	bundle := datasetsnapshotport.AuthorityBundleV2{Record: record, Manifest: manifest}
	root := filepath.Join(t.TempDir(), "bundles")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	cas, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(root, maxAuthorityBundleBytesV2, access)
	if err != nil {
		t.Fatal(err)
	}
	defer cas.Close()
	store, err := NewAuthorityBundleStoreV2(cas)
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	errorsOut := make(chan error, 16)
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errorsOut <- store.PutIfAbsent(context.Background(), bundle)
		}()
	}
	wait.Wait()
	close(errorsOut)
	for err := range errorsOut {
		if err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := store.Resolve(context.Background(), record.RecordDigest)
	if err != nil || resolved.Record != record || resolved.Manifest != manifest {
		t.Fatalf("atomic authority bundle round trip failed: %#v err=%v", resolved, err)
	}
	raw, err := cas.Read(context.Background(), record.RecordDigest)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseAuthorityBundleV2(record.RecordDigest, raw)
	if err != nil || parsed.Record != record || parsed.Manifest != manifest {
		t.Fatalf("single CAS body did not contain an exact record/manifest pair: %v", err)
	}
	restartAccess, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	restartedCAS, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(root, maxAuthorityBundleBytesV2, restartAccess)
	if err != nil {
		t.Fatal(err)
	}
	defer restartedCAS.Close()
	restarted, err := NewAuthorityBundleStoreV2(restartedCAS)
	if err != nil {
		t.Fatal(err)
	}
	if value, err := restarted.Resolve(context.Background(), record.RecordDigest); err != nil || value.Record != record {
		t.Fatalf("restarted bundle store lost immutable content: %#v err=%v", value, err)
	}
	if _, err := restarted.Resolve(context.Background(), domainsecurity.SHA256Hex([]byte("missing-bundle"))); !errors.Is(err, datasetsnapshotport.ErrNotFound) {
		t.Fatalf("missing bundle was not typed: %v", err)
	}
}
