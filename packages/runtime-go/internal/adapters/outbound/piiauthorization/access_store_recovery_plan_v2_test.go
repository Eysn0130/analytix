package piiauthorization

import (
	"context"
	"path/filepath"
	"testing"

	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestControlledAccessV2PreparedRecoveryAcceptsClosedAndCrashOpenReceipts(t *testing.T) {
	for _, test := range []struct {
		name   string
		closed bool
	}{
		{name: "closed", closed: true},
		{name: "open crash cut", closed: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "controlled-artifact-access-v2")
			access, err := privatecastest.NewAccessAuthority(root)
			if err != nil {
				t.Fatal(err)
			}
			store, err := NewAccessStoreV2(root, access)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			receipt, disposition := accessStoreFixtureV2(t, "prepared-recovery-v2")
			if err := store.ReserveAccessReceiptV2(context.Background(), receipt); err != nil {
				t.Fatal(err)
			}
			if test.closed {
				if err := store.PutAccessDispositionIfAbsentV2(context.Background(), disposition); err != nil {
					t.Fatal(err)
				}
			}
			prepared, err := PrepareAccessRecoveryV2(context.Background(), root, access)
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

func TestControlledAccessV2PreparedRecoveryRejectsLegacyJournalBytes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "controlled-artifact-access-v2")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := NewAccessStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	receipt, _ := accessStoreFixtureV1(t)
	if err := legacy.ReserveAccessReceipt(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareAccessRecoveryV2(context.Background(), root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.ValidateSemantics(context.Background()); err == nil {
		t.Fatal("V2 prepared recovery accepted legacy V1 authority bytes")
	}
}
