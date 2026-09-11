package continuation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	continuationstoreport "analytix.local/runtime-go/internal/ports/continuationstore"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

type TrustedInventoryV1 struct {
	Receipts     []domaincontinuation.Receipt
	Dispositions map[string]domaincontinuation.Disposition
}

// TrustedInventoryV1 returns an all-or-nothing installation-pinned snapshot
// for startup reconciliation. A partial visit is never returned as authority.
func (service *Service) TrustedInventoryV1(ctx context.Context) (TrustedInventoryV1, error) {
	if !service.Available() || ctx == nil {
		return TrustedInventoryV1{}, ErrAuthorityUnavailable
	}
	inventoryStore, ok := service.store.(continuationstoreport.InventoryStore)
	if !ok {
		return TrustedInventoryV1{}, errors.New("continuation inventory store is unavailable")
	}
	if err := VerifyTrustedInventoryV1(ctx, inventoryStore, service.authority); err != nil {
		return TrustedInventoryV1{}, err
	}
	result := TrustedInventoryV1{Dispositions: map[string]domaincontinuation.Disposition{}}
	if err := inventoryStore.VisitReceipts(ctx, func(receipt domaincontinuation.Receipt) error {
		verified, err := service.verifyReceipt(ctx, receipt)
		if err != nil {
			return err
		}
		result.Receipts = append(result.Receipts, verified)
		return nil
	}); err != nil {
		return TrustedInventoryV1{}, err
	}
	if err := inventoryStore.VisitDispositions(ctx, func(disposition domaincontinuation.Disposition) error {
		receipt, found := trustedReceiptForGate(result.Receipts, disposition.GateID)
		if !found {
			return errors.New("continuation disposition inventory lost its receipt")
		}
		if err := service.verifyDisposition(ctx, disposition, receipt); err != nil {
			return err
		}
		if _, duplicate := result.Dispositions[disposition.GateID]; duplicate {
			return errors.New("continuation disposition inventory repeats a gate")
		}
		result.Dispositions[disposition.GateID] = disposition
		return nil
	}); err != nil {
		return TrustedInventoryV1{}, err
	}
	return result, nil
}

func trustedReceiptForGate(receipts []domaincontinuation.Receipt, gateID string) (domaincontinuation.Receipt, bool) {
	for _, receipt := range receipts {
		if receipt.Payload.GateID == gateID {
			return receipt, true
		}
	}
	return domaincontinuation.Receipt{}, false
}

// VerifyTrustedInventoryV1 pins every private approval/user-input receipt and
// disposition to the current installation authority before any continuation
// can be reconstructed. Structural self-signatures and non-empty ids are not
// installation authority.
func VerifyTrustedInventoryV1(
	ctx context.Context,
	store continuationstoreport.InventoryStore,
	authority finalauthorityport.Authority,
) error {
	if ctx == nil || store == nil || authority == nil {
		return errors.New("continuation trusted inventory dependencies are required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	receipts := map[string]domaincontinuation.Receipt{}
	if err := store.VisitReceipts(ctx, func(receipt domaincontinuation.Receipt) error {
		if _, duplicate := receipts[receipt.Payload.GateID]; duplicate {
			return errors.New("continuation receipt inventory repeats a gate")
		}
		keyID, publicKey, signature, err := domaincontinuation.AuthorityMaterial(receipt)
		if err != nil || keyID != authority.KeyID() || !bytes.Equal(publicKey, authority.PublicKey()) ||
			authority.VerifyTrusted(ctx, keyID, publicKey, domaincontinuation.ReceiptSigningBytes(receipt), signature) != nil {
			return errors.New("continuation receipt lacks current installation authority")
		}
		receipts[receipt.Payload.GateID] = receipt
		return nil
	}); err != nil {
		return fmt.Errorf("verify continuation receipt inventory: %w", err)
	}
	seenDispositions := map[string]bool{}
	if err := store.VisitDispositions(ctx, func(disposition domaincontinuation.Disposition) error {
		if seenDispositions[disposition.GateID] {
			return errors.New("continuation disposition inventory repeats a gate")
		}
		seenDispositions[disposition.GateID] = true
		receipt, found := receipts[disposition.GateID]
		if !found || disposition.ReceiptID != receipt.ReceiptID || disposition.Kind != receipt.Payload.Kind {
			return errors.New("continuation disposition has no exact receipt authority")
		}
		disposedAt, disposedErr := time.Parse(time.RFC3339Nano, disposition.DisposedAt)
		issuedAt, issuedErr := time.Parse(time.RFC3339Nano, receipt.Payload.IssuedAt)
		if disposedErr != nil || issuedErr != nil || disposedAt.Before(issuedAt) {
			return errors.New("continuation disposition predates its receipt authority")
		}
		keyID, publicKey, signature, err := domaincontinuation.DispositionAuthorityMaterial(disposition)
		if err != nil || keyID != authority.KeyID() || !bytes.Equal(publicKey, authority.PublicKey()) ||
			authority.VerifyTrusted(ctx, keyID, publicKey, domaincontinuation.DispositionSigningBytes(disposition), signature) != nil {
			return errors.New("continuation disposition lacks current installation authority")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("verify continuation disposition inventory: %w", err)
	}
	return nil
}
