package piiauthorization

import (
	"context"
	"path/filepath"
	"testing"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestControlledAccessPreparedRecoveryAcceptsClosedAndOpenReceipts(t *testing.T) {
	for _, test := range []struct {
		name   string
		closed bool
	}{
		{name: "closed", closed: true},
		{name: "open crash cut", closed: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "controlled-artifact-access")
			access, err := privatecastest.NewAccessAuthority(root)
			if err != nil {
				t.Fatal(err)
			}
			store, err := NewAccessStore(root, access)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			receipt, disposition := accessStoreFixtureV1(t)
			if err := store.ReserveAccessReceipt(context.Background(), receipt); err != nil {
				t.Fatal(err)
			}
			if test.closed {
				if err := store.PutAccessDispositionIfAbsent(context.Background(), disposition); err != nil {
					t.Fatal(err)
				}
			}
			prepared, err := PrepareAccessRecoveryV1(context.Background(), root, access)
			if err != nil {
				t.Fatal(err)
			}
			if err := prepared.ValidateSemantics(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := prepared.Revalidate(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestControlledAccessPreparedRecoveryRejectsOrphanDisposition(t *testing.T) {
	root := filepath.Join(t.TempDir(), "controlled-artifact-access")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	receipts, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(root, "access-receipts"), domainpii.MaxControlledArtifactAccessRecordBytesV1, access,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = receipts.Close() })
	dispositions, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(root, "access-dispositions"), domainpii.MaxControlledArtifactAccessRecordBytesV1, access,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dispositions.Close() })
	_, disposition := accessStoreFixtureV1(t)
	body, err := domainpii.ControlledArtifactAccessDispositionV1Bytes(disposition)
	if err != nil {
		t.Fatal(err)
	}
	if err := dispositions.PutIfAbsent(context.Background(), disposition.AccessID, body); err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareAccessRecoveryV1(context.Background(), root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.ValidateSemantics(context.Background()); err == nil {
		t.Fatal("controlled access recovery accepted an orphan terminal disposition")
	}
}

func TestControlledAccessPreparedRecoveryRejectsSameAccessIDDifferentAuthorityClosure(t *testing.T) {
	root := filepath.Join(t.TempDir(), "controlled-artifact-access")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	receipts, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(root, "access-receipts"), domainpii.MaxControlledArtifactAccessRecordBytesV1, access,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = receipts.Close() })
	dispositions, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(root, "access-dispositions"), domainpii.MaxControlledArtifactAccessRecordBytesV1, access,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dispositions.Close() })
	receiptA, _ := accessStoreFixtureWithAuthoritySeedV1(t, 0x45)
	receiptB, dispositionB := accessStoreFixtureWithAuthoritySeedV1(t, 0x46)
	if receiptA.AccessID != receiptB.AccessID || receiptA.RecordDigest == receiptB.RecordDigest {
		t.Fatalf(
			"fixture did not preserve one semantic access across distinct authorities: accessA=%s accessB=%s recordA=%s recordB=%s",
			receiptA.AccessID, receiptB.AccessID, receiptA.RecordDigest, receiptB.RecordDigest,
		)
	}
	receiptBody, err := domainpii.ControlledArtifactAccessReceiptV1Bytes(receiptA)
	if err != nil {
		t.Fatal(err)
	}
	if err := receipts.PutIfAbsent(context.Background(), receiptA.AccessID, receiptBody); err != nil {
		t.Fatal(err)
	}
	dispositionBody, err := domainpii.ControlledArtifactAccessDispositionV1Bytes(dispositionB)
	if err != nil {
		t.Fatal(err)
	}
	if err := dispositions.PutIfAbsent(context.Background(), dispositionB.AccessID, dispositionBody); err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareAccessRecoveryV1(context.Background(), root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.ValidateSemantics(context.Background()); err == nil {
		t.Fatal("controlled access recovery accepted a terminal signed by a different authority")
	}
}
