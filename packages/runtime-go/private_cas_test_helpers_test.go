package runtimego

import (
	"context"
	"path/filepath"
	"testing"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	pendingworkstore "analytix.local/runtime-go/internal/adapters/outbound/pendingworkstore"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

const runtimeTestPendingWorkRecordBytes = 1024 * 1024

func newRuntimeTestPendingWorkStore(t *testing.T, root string) (*pendingworkstore.Store, error) {
	t.Helper()
	mutation, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		return nil, err
	}
	return pendingworkstore.NewStore(root, mutation)
}

func newRuntimeHostBoundPendingWorkStore(
	t *testing.T,
	dataDir string,
	access finalauthority.SecurePrivateCASAccessAuthority,
) (*pendingworkstore.Store, error) {
	t.Helper()
	root := filepath.Join(dataDir, "private", "pending-work")
	return pendingworkstore.NewStore(root, access)
}

func seedRuntimePendingWorkReceipt(
	t *testing.T,
	dataDir string,
	access finalauthority.SecurePrivateCASAccessAuthority,
	receipt domainpendingwork.PendingWorkReceiptV1,
) {
	t.Helper()
	root := filepath.Join(dataDir, "private", "pending-work")
	body, err := domainpendingwork.PendingWorkReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	receipts, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(root, "receipts"), runtimeTestPendingWorkRecordBytes, access,
	)
	if err != nil {
		t.Fatal(err)
	}
	dispositions, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(root, "dispositions"), runtimeTestPendingWorkRecordBytes, access,
	)
	if err != nil {
		_ = receipts.Close()
		t.Fatal(err)
	}
	if err := receipts.PutIfAbsent(context.Background(), receipt.WorkID, body); err != nil {
		_ = receipts.Close()
		_ = dispositions.Close()
		t.Fatal(err)
	}
	if err := receipts.Close(); err != nil {
		_ = dispositions.Close()
		t.Fatal(err)
	}
	if err := dispositions.Close(); err != nil {
		t.Fatal(err)
	}
}
