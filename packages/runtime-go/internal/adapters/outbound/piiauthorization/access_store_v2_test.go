package piiauthorization

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestAccessStoreV2PersistsExactProjectedDeliveryAuthority(t *testing.T) {
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
	receipt, disposition := accessStoreFixtureV2(t, "projected-outcome-v2-a")
	if err := store.ReserveAccessReceiptV2(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
	if err := store.ReserveAccessReceiptV2(context.Background(), receipt); !errors.Is(err, piiauthorizationport.ErrAlreadyReserved) {
		t.Fatalf("exact V2 replay reacquired release authority: %v", err)
	}
	if err := store.PutAccessDispositionIfAbsentV2(context.Background(), disposition); err != nil {
		t.Fatal(err)
	}
	storedReceipt, err := store.ResolveAccessReceiptV2(context.Background(), receipt.AccessID)
	if err != nil || storedReceipt.RecordDigest != receipt.RecordDigest ||
		storedReceipt.DeliveryID != receipt.DeliveryID || storedReceipt.DeliveryOutcomeRecordDigest != receipt.DeliveryOutcomeRecordDigest {
		t.Fatalf("V2 receipt readback lost delivery authority: stored=%#v err=%v", storedReceipt, err)
	}
	storedDisposition, err := store.ResolveAccessDispositionV2(context.Background(), receipt.AccessID)
	if err != nil || storedDisposition.RecordDigest != disposition.RecordDigest ||
		storedDisposition.DeliveryOutcomeRecordDigest != receipt.DeliveryOutcomeRecordDigest {
		t.Fatalf("V2 disposition readback lost delivery authority: stored=%#v err=%v", storedDisposition, err)
	}
	if has, err := store.HasRecordsV2(context.Background()); err != nil || !has {
		t.Fatalf("V2 access inventory was not detected: has=%v err=%v", has, err)
	}
}

func TestAccessStoreV2RejectsOutcomeRebindingForReservedUseSlot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "controlled-artifact-access-v2-rebinding")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewAccessStoreV2(root, access)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	first, _ := accessStoreFixtureV2(t, "projected-outcome-v2-first")
	second, _ := accessStoreFixtureV2(t, "projected-outcome-v2-replacement")
	if first.AccessID != second.AccessID || first.RecordDigest == second.RecordDigest {
		t.Fatalf("fixture did not model one slot across changed outcomes: first=%#v second=%#v", first, second)
	}
	if err := store.ReserveAccessReceiptV2(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := store.ReserveAccessReceiptV2(context.Background(), second); !errors.Is(err, piiauthorizationport.ErrConflict) {
		t.Fatalf("changed projected winner rebound a reserved use slot: %v", err)
	}
}

func TestAccessStoreV2ConcurrentOutcomesHaveOneReservationWinner(t *testing.T) {
	root := filepath.Join(t.TempDir(), "controlled-artifact-access-v2-concurrent")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewAccessStoreV2(root, access)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	first, _ := accessStoreFixtureV2(t, "projected-outcome-v2-concurrent-a")
	second, _ := accessStoreFixtureV2(t, "projected-outcome-v2-concurrent-b")
	var winners atomic.Int64
	var terminalRejects atomic.Int64
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := range 16 {
		candidate := first
		if index%2 == 1 {
			candidate = second
		}
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			err := store.ReserveAccessReceiptV2(context.Background(), candidate)
			switch {
			case err == nil:
				winners.Add(1)
			case errors.Is(err, piiauthorizationport.ErrAlreadyReserved), errors.Is(err, piiauthorizationport.ErrConflict):
				terminalRejects.Add(1)
			default:
				t.Errorf("unexpected V2 reservation result: %v", err)
			}
		}()
	}
	close(start)
	wait.Wait()
	if winners.Load() != 1 || terminalRejects.Load() != 15 {
		t.Fatalf("V2 reservation winners=%d terminal_rejects=%d, want 1/15", winners.Load(), terminalRejects.Load())
	}
}

func TestAccessStoreV2NeverInterpretsLegacyJournalBytes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "controlled-artifact-access-mixed-version")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := NewAccessStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	legacyReceipt, _ := accessStoreFixtureV1(t)
	if err := legacy.ReserveAccessReceipt(context.Background(), legacyReceipt); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	v2, err := NewAccessStoreV2(root, access)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = v2.Close() })
	if _, err := v2.ResolveAccessReceiptV2(context.Background(), legacyReceipt.AccessID); !errors.Is(err, piiauthorizationport.ErrCorrupt) {
		t.Fatalf("V2 store interpreted a legacy receipt as V2 authority: %v", err)
	}
	if err := v2.VisitAccessReceiptsV2(context.Background(), func(domainpii.ControlledArtifactAccessReceiptV2) error {
		return nil
	}); !errors.Is(err, piiauthorizationport.ErrCorrupt) {
		t.Fatalf("V2 inventory accepted legacy journal bytes: %v", err)
	}
}

func accessStoreFixtureV2(
	t *testing.T,
	outcome string,
) (domainpii.ControlledArtifactAccessReceiptV2, domainpii.ControlledArtifactAccessDispositionV2) {
	t.Helper()
	grant := storeTestGrant(t)
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x45}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	handleDigest, _ := domainpii.ControlledAccessHandleDigestV2("pii-access-store-handle-token-v2")
	useSlotDigest, _ := domainpii.ControlledAccessUseSlotDigestV2("pii-access-store-use-slot-token-v2")
	principalDigest, _ := domainpii.ControlledAccessRendererPrincipalDigestV2("pii-access-store-renderer-token-v2")
	receipt, err := domainpii.NewControlledArtifactAccessReceiptV2(domainpii.ControlledArtifactAccessReceiptInputV2{
		SecurityContext: accessStoreSecurityContextV1(t), AccessAction: domainpii.ControlledArtifactAccessActionExportV1,
		ControlledHandleDigest: handleDigest, UseSlotDigest: useSlotDigest,
		RendererPrincipalDigest: principalDigest, RendererGeneration: 3, BackendGeneration: 5,
		AccessPolicyDigest: grant.AccessPolicyDigest, RetentionPolicyDigest: grant.RetentionPolicyDigest,
		DeliveryID:                  domainsecurity.SHA256Hex([]byte("pii-access-store-delivery-v2")),
		DeliveryOutcomeRecordDigest: domainsecurity.SHA256Hex([]byte(outcome)),
		PublicationCommitDigest:     domainsecurity.SHA256Hex([]byte("pii-access-store-commit-v2")),
		PublicationReceiptDigest:    domainsecurity.SHA256Hex([]byte("pii-access-store-receipt-v2")),
		PIIProjectionDigest:         domainsecurity.SHA256Hex([]byte("pii-access-store-projection-v2")),
		PIIAuthorizationDigest:      grant.RecordDigest, ClaimLedgerDigest: grant.ClaimLedgerDigest,
		TargetIdentityDigest:        grant.TargetIdentityDigest,
		ReleaseTargetIdentityDigest: domainsecurity.SHA256Hex([]byte("pii-access-store-release-target-v2")),
		ArtifactSHA256:              grant.ProjectedContentSHA256,
		ArtifactByteLength:          4096, MediaType: domainpii.ControlledPIIArtifactMediaTypeV1,
		RequestedAt:     time.Date(2026, 7, 16, 11, 2, 0, 0, time.UTC),
		AuthorizedUntil: time.Date(2026, 7, 16, 11, 10, 0, 0, time.UTC),
		AuthorityKeyID:  domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := domainpii.NewControlledArtifactAccessDispositionV2(
		receipt, domainpii.ControlledArtifactAccessDispositionHostReleaseCommittedV1,
		domainpii.ControlledArtifactAccessReasonHostReleaseCommittedV1, receipt.ArtifactByteLength,
		time.Date(2026, 7, 16, 11, 3, 0, 0, time.UTC), domainsecurity.SHA256Hex(publicKey), publicKey,
		func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	return receipt, disposition
}
