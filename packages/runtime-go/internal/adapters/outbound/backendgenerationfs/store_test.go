package backendgenerationfs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"testing"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	backendapp "analytix.local/runtime-go/internal/app/backendgeneration"
)

func TestSecureStoreV1PersistsContinuousAllocationsAcrossLeaseRestart(t *testing.T) {
	userData := t.TempDir()
	root := filepath.Join(userData, "private", "runtime-sidecar-authority-v1", "allocations")
	first := consumeWithFreshLeaseV1(t, userData, root, bytes.NewReader(bytes.Repeat([]byte{0x11}, 32)))
	second := consumeWithFreshLeaseV1(t, userData, root, bytes.NewReader(bytes.Repeat([]byte{0x22}, 32)))
	if first.Record.Generation != 1 || second.Record.Generation != 2 ||
		second.Record.PreviousRecordDigest != first.RecordDigest {
		t.Fatalf("persistent allocation chain changed across lease restart: first=%#v second=%#v", first, second)
	}
}

func TestSecureStoreV1CompositeLeaseExcludesSecondAllocatorProcessScope(t *testing.T) {
	userData := t.TempDir()
	roots, err := persistencefs.ResolveRootSet(userData, userData)
	if err != nil {
		t.Fatal(err)
	}
	first, err := persistencefs.AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if second, err := persistencefs.AcquireCompositeLease(roots); !errors.Is(err, persistencefs.ErrPersistenceInUse) {
		if second != nil {
			_ = second.Close()
		}
		t.Fatalf("second allocator scope acquired the same authority: %v", err)
	}
}

func TestSecureStoreV1RecoveryRejectsCommittedNonAllocationRecord(t *testing.T) {
	ctx := context.Background()
	userData := t.TempDir()
	root := filepath.Join(userData, "private", "runtime-sidecar-authority-v1", "allocations")
	lease := acquireAllocatorLeaseV1(t, userData)
	prepared, err := PrepareRecoveryV1(ctx, root, lease)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.ValidateSemantics(ctx); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Revalidate(ctx); err != nil {
		t.Fatal(err)
	}
	ensureTestJournalAuthorityV1(t, ctx, lease, prepared)
	journal, err := persistencefs.NewPrivateCASRecoveryJournalV1(lease)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.ApplyV4(ctx, journal); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStoreV1(ctx, root, lease)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"schemaVersion":1,"purpose":"not-an-allocation"}`)
	digestBytes := sha256.Sum256(body)
	digest := hex.EncodeToString(digestBytes[:])
	provisional, err := store.cas.PutIfAbsentWithAdditionReceipt(ctx, digest, body)
	if err != nil {
		t.Fatal(err)
	}
	finalized, err := store.cas.FinalizeCommittedAdditions(
		ctx, []finalauthority.SecurePrivateCASAdditionReceiptV2{provisional},
	)
	if err != nil || len(finalized) != 1 || store.cas.VerifyCommittedAddition(ctx, finalized[0]) != nil {
		t.Fatalf("failed to create committed corruption fixture: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}

	lease = acquireAllocatorLeaseV1(t, userData)
	defer lease.Close()
	prepared, err = PrepareRecoveryV1(ctx, root, lease)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.ValidateSemantics(ctx); err == nil {
		t.Fatal("committed non-allocation record survived semantic recovery validation")
	}
}

func consumeWithFreshLeaseV1(
	t *testing.T,
	userData string,
	root string,
	random *bytes.Reader,
) backendapp.ConsumedAllocationV1 {
	t.Helper()
	ctx := context.Background()
	lease := acquireAllocatorLeaseV1(t, userData)
	prepared, err := PrepareRecoveryV1(ctx, root, lease)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.ValidateSemantics(ctx); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Revalidate(ctx); err != nil {
		t.Fatal(err)
	}
	ensureTestJournalAuthorityV1(t, ctx, lease, prepared)
	journal, err := persistencefs.NewPrivateCASRecoveryJournalV1(lease)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.ApplyV4(ctx, journal); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStoreV1(ctx, root, lease)
	if err != nil {
		t.Fatal(err)
	}
	allocator, err := backendapp.NewAllocatorV1(store, random)
	if err != nil {
		t.Fatal(err)
	}
	result, err := allocator.Consume(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	return result
}

func ensureTestJournalAuthorityV1(
	t *testing.T,
	ctx context.Context,
	lease *persistencefs.CompositeLease,
	owner persistencefs.JournalAuthorityPreflightOwnerV1,
) {
	t.Helper()
	prepared, err := persistencefs.PrepareJournalAuthorityBootstrapV1(ctx, lease, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, present, err := prepared.BindExistingV1(ctx); err != nil {
		t.Fatal(err)
	} else if !present {
		if _, err := prepared.CreateV1(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

func acquireAllocatorLeaseV1(t *testing.T, userData string) *persistencefs.CompositeLease {
	t.Helper()
	roots, err := persistencefs.ResolveRootSet(userData, userData)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := persistencefs.AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	return lease
}
