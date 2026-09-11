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
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestAccessStorePersistsOneExactReceiptAndDispositionPerAccess(t *testing.T) {
	root := filepath.Join(t.TempDir(), "controlled-access")
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
	if err := store.ReserveAccessReceipt(context.Background(), receipt); !errors.Is(err, piiauthorizationport.ErrAlreadyReserved) {
		t.Fatalf("existing access receipt reacquired release authority: %v", err)
	}
	for range 2 {
		if err := store.PutAccessDispositionIfAbsent(context.Background(), disposition); err != nil {
			t.Fatal(err)
		}
	}
	storedReceipt, err := store.ResolveAccessReceipt(context.Background(), receipt.AccessID)
	if err != nil || storedReceipt.RecordDigest != receipt.RecordDigest {
		t.Fatalf("access receipt readback failed: stored=%#v err=%v", storedReceipt, err)
	}
	storedDisposition, err := store.ResolveAccessDisposition(context.Background(), receipt.AccessID)
	if err != nil || storedDisposition.RecordDigest != disposition.RecordDigest {
		t.Fatalf("access disposition readback failed: stored=%#v err=%v", storedDisposition, err)
	}
	if has, err := store.HasRecords(context.Background()); err != nil || !has {
		t.Fatalf("access inventory was not detected: has=%v err=%v", has, err)
	}
	receipts, dispositions := 0, 0
	if err := store.VisitAccessReceipts(context.Background(), func(candidate domainpii.ControlledArtifactAccessReceiptV1) error {
		receipts++
		if candidate.AccessID != receipt.AccessID {
			t.Fatalf("unexpected access receipt: %#v", candidate)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.VisitAccessDispositions(context.Background(), func(candidate domainpii.ControlledArtifactAccessDispositionV1) error {
		dispositions++
		if candidate.AccessID != receipt.AccessID {
			t.Fatalf("unexpected access disposition: %#v", candidate)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if receipts != 1 || dispositions != 1 {
		t.Fatalf("access inventory cardinality changed: receipts=%d dispositions=%d", receipts, dispositions)
	}
	if _, err := store.ResolveAccessReceipt(context.Background(), domainsecurity.SHA256Hex([]byte("missing-access"))); !errors.Is(err, piiauthorizationport.ErrNotFound) {
		t.Fatalf("missing access did not fail closed: %v", err)
	}
}

func TestAccessStoreRejectsConflictingTerminalDisposition(t *testing.T) {
	root := filepath.Join(t.TempDir(), "controlled-access-conflict")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewAccessStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	receipt, delivered := accessStoreFixtureV1(t)
	if err := store.ReserveAccessReceipt(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
	if err := store.PutAccessDispositionIfAbsent(context.Background(), delivered); err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x45}, ed25519.SeedSize))
	failed, err := domainpii.NewControlledArtifactAccessDispositionV1(
		receipt, domainpii.ControlledArtifactAccessDispositionFailedV1, domainpii.ControlledArtifactAccessReasonHostReleaseFailedV1, 0,
		time.Date(2026, 7, 16, 11, 4, 0, 0, time.UTC), receipt.AuthorityKeyID, privateKey.Public().(ed25519.PublicKey),
		func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutAccessDispositionIfAbsent(context.Background(), failed); !errors.Is(err, piiauthorizationport.ErrConflict) {
		t.Fatalf("conflicting access disposition was not rejected: %v", err)
	}
}

func TestAccessStoreConcurrentReservationHasExactlyOneWinner(t *testing.T) {
	root := filepath.Join(t.TempDir(), "controlled-access-concurrent")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewAccessStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	receipt, _ := accessStoreFixtureV1(t)
	var winners atomic.Int64
	var alreadyReserved atomic.Int64
	start := make(chan struct{})
	var wait sync.WaitGroup
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			err := store.ReserveAccessReceipt(context.Background(), receipt)
			switch {
			case err == nil:
				winners.Add(1)
			case errors.Is(err, piiauthorizationport.ErrAlreadyReserved):
				alreadyReserved.Add(1)
			default:
				t.Errorf("unexpected reservation result: %v", err)
			}
		}()
	}
	close(start)
	wait.Wait()
	if winners.Load() != 1 || alreadyReserved.Load() != 15 {
		t.Fatalf("reservation winners=%d already_reserved=%d, want 1/15", winners.Load(), alreadyReserved.Load())
	}
}

func TestAccessStoreRejectsChangedReceiptForAlreadyReservedUseSlot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "controlled-access-use-slot")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewAccessStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	first, _ := accessStoreFixtureV1(t)
	if err := store.ReserveAccessReceipt(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	changed := first
	changed.RequestedAt = time.Date(2026, 7, 16, 11, 2, 0, 1, time.UTC).Format(time.RFC3339Nano)
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x45}, ed25519.SeedSize))
	changed, err = domainpii.NewControlledArtifactAccessReceiptV1(domainpii.ControlledArtifactAccessReceiptInputV1{
		SecurityContext: accessStoreSecurityContextV1(t),
		AccessAction:    changed.AccessAction, ControlledHandleDigest: changed.ControlledHandleDigest,
		UseSlotDigest: changed.UseSlotDigest, RendererPrincipalDigest: changed.RendererPrincipalDigest,
		RendererGeneration: changed.RendererGeneration, BackendGeneration: changed.BackendGeneration,
		AccessPolicyDigest: changed.AccessPolicyDigest, RetentionPolicyDigest: changed.RetentionPolicyDigest,
		PublicationCommitDigest: changed.PublicationCommitDigest, PublicationReceiptDigest: changed.PublicationReceiptDigest,
		PIIProjectionDigest: changed.PIIProjectionDigest, PIIAuthorizationDigest: changed.PIIAuthorizationDigest,
		ClaimLedgerDigest: changed.ClaimLedgerDigest, TargetIdentityDigest: changed.TargetIdentityDigest,
		ArtifactSHA256: changed.ArtifactSHA256, ArtifactByteLength: changed.ArtifactByteLength, MediaType: changed.MediaType,
		RequestedAt:     time.Date(2026, 7, 16, 11, 2, 0, 1, time.UTC),
		AuthorizedUntil: time.Date(2026, 7, 16, 11, 10, 0, 0, time.UTC),
		AuthorityKeyID:  changed.AuthorityKeyID, AuthorityPublicKey: privateKey.Public().(ed25519.PublicKey),
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	if changed.AccessID != first.AccessID {
		t.Fatalf("changed receipt escaped its use-slot reservation key: first=%s changed=%s", first.AccessID, changed.AccessID)
	}
	if err := store.ReserveAccessReceipt(context.Background(), changed); !errors.Is(err, piiauthorizationport.ErrConflict) {
		t.Fatalf("changed receipt reacquired an already reserved use slot: %v", err)
	}
}

func TestAccessStoreCloseIsConcurrentAndIdempotent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "controlled-access-close")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewAccessStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errorsOut := make(chan error, 16)
	var wait sync.WaitGroup
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			errorsOut <- store.Close()
		}()
	}
	close(start)
	wait.Wait()
	close(errorsOut)
	for err := range errorsOut {
		if err != nil {
			t.Fatalf("controlled access store close was not concurrent and idempotent: %v", err)
		}
	}
	if _, err := store.HasRecords(context.Background()); err == nil {
		t.Fatal("closed controlled access store retained live private-CAS authority")
	}
}

func accessStoreFixtureV1(t *testing.T) (domainpii.ControlledArtifactAccessReceiptV1, domainpii.ControlledArtifactAccessDispositionV1) {
	return accessStoreFixtureWithAuthoritySeedV1(t, 0x45)
}

func accessStoreFixtureWithAuthoritySeedV1(
	t *testing.T,
	authoritySeed byte,
) (domainpii.ControlledArtifactAccessReceiptV1, domainpii.ControlledArtifactAccessDispositionV1) {
	t.Helper()
	grant := storeTestGrant(t)
	securityContext := accessStoreSecurityContextV1(t)
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{authoritySeed}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	receipt, err := domainpii.NewControlledArtifactAccessReceiptV1(domainpii.ControlledArtifactAccessReceiptInputV1{
		SecurityContext: securityContext, AccessAction: domainpii.ControlledArtifactAccessActionExportV1,
		ControlledHandleDigest:   domainsecurity.SHA256Hex([]byte("pii-access-store-handle")),
		UseSlotDigest:            domainsecurity.SHA256Hex([]byte("pii-access-store-use-slot")),
		RendererPrincipalDigest:  domainsecurity.SHA256Hex([]byte("pii-access-store-renderer")),
		RendererGeneration:       3,
		BackendGeneration:        5,
		AccessPolicyDigest:       grant.AccessPolicyDigest,
		RetentionPolicyDigest:    grant.RetentionPolicyDigest,
		PublicationCommitDigest:  domainsecurity.SHA256Hex([]byte("pii-access-store-commit")),
		PublicationReceiptDigest: domainsecurity.SHA256Hex([]byte("pii-access-store-receipt")),
		PIIProjectionDigest:      domainsecurity.SHA256Hex([]byte("pii-access-store-projection")),
		PIIAuthorizationDigest:   grant.RecordDigest, ClaimLedgerDigest: grant.ClaimLedgerDigest,
		TargetIdentityDigest: grant.TargetIdentityDigest, ArtifactSHA256: grant.ProjectedContentSHA256,
		ArtifactByteLength: 4096, MediaType: domainpii.ControlledPIIArtifactMediaTypeV1,
		RequestedAt:     time.Date(2026, 7, 16, 11, 2, 0, 0, time.UTC),
		AuthorizedUntil: time.Date(2026, 7, 16, 11, 10, 0, 0, time.UTC),
		AuthorityKeyID:  domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := domainpii.NewControlledArtifactAccessDispositionV1(
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

func accessStoreSecurityContextV1(t *testing.T) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-pii-store", TurnID: "turn-pii-store", WorkspaceRealPath: "/workspace/pii-store",
		CaseID: "case-pii-store", CaseBindingHash: domainsecurity.SHA256Hex([]byte("pii-store-binding")),
		ContextEpoch: 2, IssuedAt: time.Date(2026, 7, 16, 11, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}
