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
	gateReceipts := make([]domaincontinuation.Receipt, 0, len(result.Receipts))
	for _, receipt := range result.Receipts {
		if receipt.Payload.UserInputDraft != nil {
			continue
		}
		if receipt.Payload.UserInputReplyCapture != nil {
			if _, err := service.resolveLateCaptureSource(ctx, receipt); err != nil {
				return TrustedInventoryV1{}, err
			}
			delete(result.Dispositions, receipt.Payload.GateID)
			continue
		}
		gateReceipts = append(gateReceipts, receipt)
	}
	result.Receipts = gateReceipts
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
	dispositions := map[string]domaincontinuation.Disposition{}
	seenDispositions := map[string]bool{}
	if err := store.VisitDispositions(ctx, func(disposition domaincontinuation.Disposition) error {
		if seenDispositions[disposition.GateID] {
			return errors.New("continuation disposition inventory repeats a gate")
		}
		seenDispositions[disposition.GateID] = true
		dispositions[disposition.GateID] = disposition
		receipt, found := receipts[disposition.GateID]
		if !found || receipt.Payload.UserInputDraft != nil || disposition.ReceiptID != receipt.ReceiptID || disposition.Kind != receipt.Payload.Kind {
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
	if err := verifyUserInputDraftInventoryV1(receipts); err != nil {
		return err
	}
	for _, receipt := range receipts {
		if receipt.Payload.UserInputDraft != nil {
			continue
		}
		if link := receipt.Payload.UserInputReplyCapture; link != nil {
			source, exists := receipts[link.SourceInputID]
			sourceDisposition, continued := dispositions[link.SourceInputID]
			if !exists || !continued || domaincontinuation.ValidateUserInputReplyCaptureSourceV1(receipt, source) != nil || sourceDisposition.Status != domaincontinuation.StatusContinued || !domaincontinuation.ValidUserInputContinuedReasonV1(source.Payload.UserInputTimedWait, sourceDisposition.ReasonCode) {
				return errors.New("late capture inventory has no original continued authority")
			}
			capturedAt, capturedErr := time.Parse(time.RFC3339Nano, link.CapturedAt)
			continuedAt, continuedErr := time.Parse(time.RFC3339Nano, sourceDisposition.DisposedAt)
			if capturedErr != nil || continuedErr != nil || capturedAt.Before(continuedAt) {
				return errors.New("late capture inventory predates continued authority")
			}
			if disposition, complete := dispositions[receipt.Payload.GateID]; complete {
				if disposition.UserInputReplySourceID != link.SourceInputID || disposition.UserInputSubmission == nil || disposition.DisposedAt != link.CapturedAt || validateUserInputSubmissionForReceipt(source, *disposition.UserInputSubmission) != nil {
					return errors.New("late capture inventory submission conflicts")
				}
			}
		} else {
			if disposition, exists := dispositions[receipt.Payload.GateID]; exists {
				if disposition.UserInputReplySourceID != "" || (disposition.Status == domaincontinuation.StatusContinued && !domaincontinuation.ValidUserInputContinuedReasonV1(receipt.Payload.UserInputTimedWait, disposition.ReasonCode)) {
					return errors.New("ordinary continuation inventory has invalid timed linkage")
				}
				if disposition.UserInputSubmission != nil && validateUserInputSubmissionForReceipt(receipt, *disposition.UserInputSubmission) != nil {
					return errors.New("continuation inventory submission conflicts")
				}
			}
		}
	}
	return nil
}

// The private draft kind is verified before filtering and cannot become a
// reconstructed Gate. Detached, branched, repeated or over-capacity chains
// reject the complete inventory rather than downgrading to a partial view.
func verifyUserInputDraftInventoryV1(receipts map[string]domaincontinuation.Receipt) error {
	bySource := map[string][]domaincontinuation.Receipt{}
	for _, receipt := range receipts {
		if link := receipt.Payload.UserInputDraft; link != nil {
			source, found := receipts[link.SourceInputID]
			if !found || domaincontinuation.ValidateUserInputDraftSourceV1(receipt, source) != nil || ValidateUserInputDraftForReceiptV1(source, link.Snapshot) != nil {
				return ErrUserInputDraftInvalidV1
			}
			bySource[link.SourceInputID] = append(bySource[link.SourceInputID], receipt)
		}
	}
	for sourceID, drafts := range bySource {
		source := receipts[sourceID]
		var parent *string
		seen := map[string]bool{}
		retained := 0
		at, err := time.Parse(time.RFC3339Nano, source.Payload.IssuedAt)
		if err != nil {
			return ErrUserInputDraftInvalidV1
		}
		for count := 0; ; count++ {
			receipt, exists := receipts[domaincontinuation.UserInputDraftSlotGateIDV1(source.ReceiptID, parent)]
			if !exists {
				if count != len(drafts) {
					return ErrUserInputDraftInvalidV1
				}
				break
			}
			link := receipt.Payload.UserInputDraft
			if link == nil || link.SourceInputID != sourceID || !equalDraftParentV1(link.ParentRevision, parent) || seen[link.Revision] || count >= domaincontinuation.MaxUserInputDraftRevisionsV1 {
				return ErrUserInputDraftInvalidV1
			}
			savedAt, err := time.Parse(time.RFC3339Nano, receipt.Payload.IssuedAt)
			if err != nil || savedAt.Before(at) {
				return ErrUserInputDraftInvalidV1
			}
			retained += draftSnapshotBytesV1(link.Snapshot)
			if retained > domaincontinuation.MaxUserInputDraftRetainedBytesV1 {
				return ErrUserInputDraftCapacityV1
			}
			seen[link.Revision] = true
			value := link.Revision
			parent = &value
			at = savedAt
		}
	}
	return nil
}
