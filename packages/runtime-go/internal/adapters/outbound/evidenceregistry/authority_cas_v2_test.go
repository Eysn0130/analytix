package evidenceregistry

import (
	"context"
	"path/filepath"
	"testing"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestEvidenceRegistryAuthorityV2CASPersistsOnlyByContentAddress(t *testing.T) {
	root := t.TempDir()
	mutation, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	authority := storeTestAuthority(t, filepath.Join(root, "authority"))
	securityContext := storeTestContext(t)
	registry, err := domainevidence.NewEvidenceReceiptRegistry(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	material := storeTestMaterial(t)
	proof := storeTestProof()
	registry, _, err = domainevidence.RegisterEvidenceReceipt(
		registry, storeTestDraft(t, securityContext, material), material, proof, storeTestTime(),
	)
	if err != nil {
		t.Fatal(err)
	}
	capsule, err := domainevidence.NewEvidenceRegistryAuthorityCapsule(
		securityContext, registry, authority.KeyID(), authority.PublicKey(),
		func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) },
	)
	if err != nil {
		t.Fatal(err)
	}
	index, err := domainevidence.NewEvidenceRegistryAuthorityIndexV2(domainevidence.EvidenceRegistryAuthorityIndexInputV2{
		InstallationID: domainsecurity.SHA256Hex([]byte("registry-v2-cas-installation")),
		EnrollmentID:   domainsecurity.SHA256Hex([]byte("registry-v2-cas-enrollment")), Generation: 1,
		PreviousIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
		MutationID:          domainsecurity.SHA256Hex([]byte("registry-v2-cas-mutation")),
	}, capsule, authority.KeyID(), authority.PublicKey(), func(message []byte) ([]byte, error) {
		return authority.Sign(context.Background(), message)
	})
	if err != nil {
		t.Fatal(err)
	}
	capsules, err := NewAuthorityCapsuleStoreV2(filepath.Join(root, "capsules"), mutation)
	if err != nil {
		t.Fatal(err)
	}
	indexes, err := NewAuthorityIndexStoreV2(filepath.Join(root, "indexes"), mutation)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := capsules.PutIfAbsent(context.Background(), capsule); err != nil {
			t.Fatal(err)
		}
		if err := indexes.PutIfAbsent(context.Background(), index); err != nil {
			t.Fatal(err)
		}
	}
	reopenedCapsules, err := NewAuthorityCapsuleStoreV2(filepath.Join(root, "capsules"), mutation)
	if err != nil {
		t.Fatal(err)
	}
	reopenedIndexes, err := NewAuthorityIndexStoreV2(filepath.Join(root, "indexes"), mutation)
	if err != nil {
		t.Fatal(err)
	}
	resolvedCapsule, err := reopenedCapsules.Resolve(context.Background(), capsule.RecordDigest)
	if err != nil || resolvedCapsule.RecordDigest != capsule.RecordDigest {
		t.Fatalf("capsule CAS readback mismatch: capsule=%#v err=%v", resolvedCapsule, err)
	}
	resolvedIndex, err := reopenedIndexes.Resolve(context.Background(), index.IndexDigest)
	if err != nil || resolvedIndex.IndexDigest != index.IndexDigest || !domainevidence.EvidenceRegistryAuthorityIndexEntryMatchesCapsuleV2(resolvedIndex, resolvedCapsule) {
		t.Fatalf("index CAS readback mismatch: index=%#v err=%v", resolvedIndex, err)
	}
	if _, err := reopenedIndexes.Resolve(context.Background(), domainsecurity.SHA256Hex([]byte("missing-index"))); err == nil {
		t.Fatal("missing witness-selected index was invented from local inventory")
	}
}
