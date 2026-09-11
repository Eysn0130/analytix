package continuationstore_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	continuationstore "analytix.local/runtime-go/internal/adapters/outbound/continuationstore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestLegacyContinuationMigrationPreservesExactPrivateAuthority(t *testing.T) {
	root := filepath.Join(t.TempDir(), "gate-continuations")
	authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(t.TempDir(), "authority", "final.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 16, 13, 0, 0, 0, time.UTC)
	receipt, disposition := signedContinuationFixture(
		t, authority, now, json.RawMessage(`{"account":"6222020202020202020","path":"case-a.csv"}`),
	)
	legacyReceiptPath, _ := writeLegacyContinuationFixture(t, root, receipt, disposition)
	legacyReceiptBody, err := os.ReadFile(legacyReceiptPath)
	if err != nil || !bytes.Contains(legacyReceiptBody, []byte("6222020202020202020")) {
		t.Fatalf("legacy private authority fixture lost the exact account: err=%v", err)
	}
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := continuationstore.MigrateLegacyV1ForSemanticStage(context.Background(), root, access); err != nil {
		t.Fatalf("migrate legacy continuation authority: %v", err)
	}
	for _, partition := range []string{"receipts", "dispositions"} {
		if _, err := os.Lstat(filepath.Join(root, partition)); !os.IsNotExist(err) {
			t.Fatalf("legacy partition %q survived migration: %v", partition, err)
		}
	}
	store, err := continuationstore.NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	migratedReceipt, err := store.ResolveReceipt(context.Background(), receipt.Payload.GateID)
	if err != nil {
		t.Fatal(err)
	}
	migratedDisposition, err := store.ResolveDisposition(context.Background(), receipt.Payload.GateID)
	if err != nil {
		t.Fatal(err)
	}
	wantReceipt, _ := domaincontinuation.ReceiptBytes(receipt)
	gotReceipt, _ := domaincontinuation.ReceiptBytes(migratedReceipt)
	wantDisposition, _ := domaincontinuation.DispositionBytes(disposition)
	gotDisposition, _ := domaincontinuation.DispositionBytes(migratedDisposition)
	if !bytes.Equal(gotReceipt, wantReceipt) || !bytes.Equal(gotDisposition, wantDisposition) {
		t.Fatal("legacy continuation migration changed signed authority bytes")
	}
	if !bytes.Contains(gotReceipt, []byte("6222020202020202020")) {
		t.Fatal("controlled private continuation authority redacted the authorized exact account")
	}
	if err := continuationstore.MigrateLegacyV1ForSemanticStage(context.Background(), root, access); err != nil {
		t.Fatalf("idempotent migration failed: %v", err)
	}
}

func TestLegacyContinuationMigrationRejectsUnknownInventoryWithoutRetiringLegacy(t *testing.T) {
	root := filepath.Join(t.TempDir(), "gate-continuations")
	authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(t.TempDir(), "authority", "final.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	receipt, disposition := signedContinuationFixture(t, authority, time.Date(2026, 7, 16, 14, 0, 0, 0, time.UTC), json.RawMessage(`{"path":"case-a.csv"}`))
	receiptPath, _ := writeLegacyContinuationFixture(t, root, receipt, disposition)
	unknownPath := filepath.Join(filepath.Dir(receiptPath), ".unknown")
	if err := os.WriteFile(unknownPath, []byte("unsafe"), 0o600); err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := continuationstore.MigrateLegacyV1ForSemanticStage(context.Background(), root, access); err == nil {
		t.Fatal("unknown legacy continuation inventory was migrated")
	}
	if _, err := os.Stat(receiptPath); err != nil {
		t.Fatalf("failed migration retired legacy authority: %v", err)
	}
	if _, err := os.Stat(unknownPath); err != nil {
		t.Fatalf("failed migration removed unknown evidence: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "receipts-v2")); !os.IsNotExist(err) {
		t.Fatalf("failed preflight created a v2 authority root: %v", err)
	}
}

func TestLegacyContinuationMigrationRejectsV2ConflictAndKeepsLegacy(t *testing.T) {
	root := filepath.Join(t.TempDir(), "gate-continuations")
	current, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(t.TempDir(), "authority", "current.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(t.TempDir(), "authority", "foreign.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 16, 15, 0, 0, 0, time.UTC)
	payload := servicePayloadWithPrivateArguments(t, now, json.RawMessage(`{"account":"6222020202020202020"}`))
	currentReceipt := signContinuationReceipt(t, current, payload)
	foreignReceipt := signContinuationReceipt(t, foreign, payload)
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := continuationstore.NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutReceiptIfAbsent(context.Background(), foreignReceipt); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	legacyReceiptPath, _ := writeLegacyContinuationFixture(t, root, currentReceipt, domaincontinuation.Disposition{})
	if err := continuationstore.MigrateLegacyV1ForSemanticStage(context.Background(), root, access); err == nil {
		t.Fatal("conflicting v2 continuation authority was overwritten")
	}
	if _, err := os.Stat(legacyReceiptPath); err != nil {
		t.Fatalf("conflicting migration retired legacy authority: %v", err)
	}
	stored, err := continuationstore.NewStoreContext(context.Background(), root, access)
	if err == nil {
		_ = stored.Close()
		t.Fatal("live continuation store opened while legacy authority remained")
	}
}

func TestLegacyContinuationMigrationRequiresSemanticStageAuthority(t *testing.T) {
	root := filepath.Join(t.TempDir(), "gate-continuations")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := continuationstore.MigrateLegacyV1ForSemanticStage(
		context.Background(), root, nonStageContinuationAccess{access: access},
	); err == nil {
		t.Fatal("live-style private CAS authority invoked destructive legacy migration")
	}
}

func TestContinuationRecoveryRejectsHiddenOwnerEntry(t *testing.T) {
	root := filepath.Join(t.TempDir(), "gate-continuations")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".hidden"), []byte("unsafe"), 0o600); err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := continuationstore.PrepareRecoveryV1(context.Background(), root, access); err == nil {
		t.Fatal("hidden continuation owner entry escaped prepared recovery validation")
	}
}

func TestContinuationPreparedRecoveryRejectsOwnerSwapWithOriginalV2Leaves(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "gate-continuations")
	access, err := privatecastest.NewAccessAuthority(parent)
	if err != nil {
		t.Fatal(err)
	}
	store, err := continuationstore.NewStoreContext(context.Background(), root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	prepared, err := continuationstore.PrepareRecoveryV1(context.Background(), root, access)
	if err != nil {
		t.Fatal(err)
	}
	backup := root + ".original"
	if err := os.Rename(root, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, partition := range []string{"receipts-v2", "dispositions-v2"} {
		if err := os.Rename(filepath.Join(backup, partition), filepath.Join(root, partition)); err != nil {
			t.Fatal(err)
		}
	}
	if err := prepared.Revalidate(context.Background()); err == nil {
		t.Fatal("continuation owner replacement passed while both original V2 leaf identities were preserved")
	}
}

func signedContinuationFixture(
	t *testing.T,
	authority *finalauthority.FileAuthority,
	now time.Time,
	arguments json.RawMessage,
) (domaincontinuation.Receipt, domaincontinuation.Disposition) {
	t.Helper()
	receipt := signContinuationReceipt(t, authority, servicePayloadWithPrivateArguments(t, now, arguments))
	disposition, err := domaincontinuation.NewDisposition(
		receipt, domaincontinuation.StatusAllowed, "approval_allowed", now.Add(2*time.Minute),
		authority.KeyID(), authority.PublicKey(), func(message []byte) ([]byte, error) {
			return authority.Sign(context.Background(), message)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return receipt, disposition
}

func signContinuationReceipt(
	t *testing.T,
	authority *finalauthority.FileAuthority,
	payload domaincontinuation.Payload,
) domaincontinuation.Receipt {
	t.Helper()
	receipt, err := domaincontinuation.NewReceipt(
		payload, authority.KeyID(), authority.PublicKey(), func(message []byte) ([]byte, error) {
			return authority.Sign(context.Background(), message)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func writeLegacyContinuationFixture(
	t *testing.T,
	root string,
	receipt domaincontinuation.Receipt,
	disposition domaincontinuation.Disposition,
) (string, string) {
	t.Helper()
	digest := strings.TrimPrefix(strings.TrimPrefix(receipt.Payload.GateID, "appr_"), "input_")
	receiptPath := filepath.Join(root, "receipts", digest[:2], receipt.Payload.GateID, "receipt.json")
	receiptBody, err := domaincontinuation.ReceiptBytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	writeLegacyContinuationRecord(t, root, receiptPath, receiptBody)
	if disposition.GateID == "" {
		return receiptPath, ""
	}
	dispositionPath := filepath.Join(root, "dispositions", digest[:2], receipt.Payload.GateID, "disposition.json")
	dispositionBody, err := domaincontinuation.DispositionBytes(disposition)
	if err != nil {
		t.Fatal(err)
	}
	writeLegacyContinuationRecord(t, root, dispositionPath, dispositionBody)
	return receiptPath, dispositionPath
}

func writeLegacyContinuationRecord(t *testing.T, root, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	for current := filepath.Dir(path); current != filepath.Dir(root); current = filepath.Dir(current) {
		if err := os.Chmod(current, 0o700); err != nil {
			t.Fatal(err)
		}
		if current == root {
			break
		}
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
}

type nonStageContinuationAccess struct {
	access *privatecastest.AccessAuthority
}

func (access nonStageContinuationAccess) IsSemanticStagePrivateCASAccessAuthority() bool {
	return false
}

func (access nonStageContinuationAccess) WithPrivateCASAccess(
	ctx context.Context,
	requestedRoot string,
	use func(privatecasport.RootBinding) error,
) error {
	return access.access.WithPrivateCASAccess(ctx, requestedRoot, use)
}
