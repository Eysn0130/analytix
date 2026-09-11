//go:build darwin || linux || windows

package evidenceregistry

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestOriginalRegistryV2RetainsUnwitnessedSiblingGraphAndStandaloneCapsule(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "evidence-registry")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthorityadapter.OpenOrCreateFileAuthority(filepath.Join(t.TempDir(), "authority.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	indexes, err := NewAuthorityIndexStoreV2(filepath.Join(root, "indexes"), access)
	if err != nil {
		t.Fatal(err)
	}
	defer indexes.cas.Close()
	capsules, err := NewAuthorityCapsuleStoreV2(filepath.Join(root, "capsules"), access)
	if err != nil {
		t.Fatal(err)
	}
	defer capsules.cas.Close()
	frozen := storeTestContext(t)
	registry, err := domainevidence.NewEvidenceReceiptRegistry(frozen)
	if err != nil {
		t.Fatal(err)
	}
	installation := domainsecurity.SHA256Hex([]byte("independent-installation"))
	enrollment := domainsecurity.SHA256Hex([]byte("independent-enrollment"))
	sign := func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) }
	var first, sibling domainevidence.EvidenceRegistryAuthorityIndexV2
	for ordinal, label := range []string{"first", "sibling", "standalone"} {
		input := storeTestPreparedInputForLabel(t, frozen, label, "100")
		registry, _, err = domainevidence.RegisterEvidenceReceipt(registry, input.Draft, input.CanonicalEvidence, input.SettlementProof, input.RegisteredAt)
		if err != nil {
			t.Fatal(err)
		}
		capsule, err := domainevidence.NewEvidenceRegistryAuthorityCapsule(frozen, registry, authority.KeyID(), authority.PublicKey(), sign)
		if err != nil {
			t.Fatal(err)
		}
		if err := capsules.PutIfAbsent(ctx, capsule); err != nil {
			t.Fatal(err)
		}
		if ordinal == 2 {
			continue
		}
		previous := domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2()
		if ordinal == 1 {
			previous = first.IndexDigest
		}
		inputIndex := domainevidence.EvidenceRegistryAuthorityIndexInputV2{InstallationID: installation, EnrollmentID: enrollment, Generation: uint64(ordinal + 1), PreviousIndexDigest: previous, MutationID: domainsecurity.SHA256Hex([]byte(label))}
		index, err := domainevidence.NewEvidenceRegistryAuthorityIndexV2(inputIndex, capsule, authority.KeyID(), authority.PublicKey(), sign)
		if err != nil {
			t.Fatal(err)
		}
		if err := indexes.PutIfAbsent(ctx, index); err != nil {
			t.Fatal(err)
		}
		if ordinal == 0 {
			first = index
		} else {
			inputIndex.MutationID = domainsecurity.SHA256Hex([]byte("retry-after-witness-refused"))
			sibling, err = domainevidence.NewEvidenceRegistryAuthorityIndexV2(inputIndex, capsule, authority.KeyID(), authority.PublicKey(), sign)
			if err != nil {
				t.Fatal(err)
			}
			if err := indexes.PutIfAbsent(ctx, sibling); err != nil {
				t.Fatal(err)
			}
		}
	}
	prepared, err := PrepareRecoveryV2(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	files, err := prepared.SnapshotOriginalFilesV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := ParseOriginalGraphV2(ctx, files, []domainsecurity.TurnSecurityContext{frozen}, installation, enrollment, authority)
	if err != nil || len(graph.Indexes) != 3 || len(graph.Capsules) != 3 {
		t.Fatalf("complete original sibling/standalone graph was not retained: indexes=%d capsules=%d err=%v", len(graph.Indexes), len(graph.Capsules), err)
	}
	if graph.Indexes[sibling.IndexDigest] != sibling || graph.Indexes[first.IndexDigest] != first {
		t.Fatal("original graph selected a local tip or dropped a sibling")
	}
	for _, fault := range []string{"foreign enrollment", "foreign key", "missing context", "missing predecessor", "canceled"} {
		t.Run(fault, func(t *testing.T) {
			candidate := map[string]OriginalLegacyEntryV1{}
			for name, value := range files {
				candidate[name] = value
			}
			callContext, callEnrollment, verifier := ctx, enrollment, authority
			contexts := []domainsecurity.TurnSecurityContext{frozen}
			switch fault {
			case "foreign enrollment":
				callEnrollment = domainsecurity.SHA256Hex([]byte("foreign-enrollment"))
			case "foreign key":
				verifier, err = finalauthorityadapter.OpenOrCreateFileAuthority(filepath.Join(t.TempDir(), "other.json"), false)
				if err != nil {
					t.Fatal(err)
				}
			case "missing context":
				contexts = nil
			case "missing predecessor":
				delete(candidate, "indexes/"+first.IndexDigest[:2]+"/"+first.IndexDigest+".json")
			case "canceled":
				var cancel context.CancelFunc
				callContext, cancel = context.WithCancel(ctx)
				cancel()
			}
			graph, err := ParseOriginalGraphV2(callContext, candidate, contexts, installation, callEnrollment, verifier)
			if err == nil || graph.Indexes != nil || graph.Capsules != nil || fault == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("original graph lost refusal/cause or returned a partial graph: %v", err)
			}
		})
	}
	after, err := prepared.SnapshotOriginalFilesV1(ctx)
	if err != nil || !reflect.DeepEqual(files, after) {
		t.Fatalf("original graph observation changed bytes or modes: %v", err)
	}
}
