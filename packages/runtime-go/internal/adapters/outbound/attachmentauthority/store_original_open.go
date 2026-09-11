package attachmentauthority

import (
	"context"
	"errors"
	"path/filepath"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
)

// OpenPreservingOriginalResiduesV1 opens all five existing leaves from the
// same complete owner observation. It neither creates missing topology nor
// recovers residue. The runtime retains held-scope and current-key authority.
func (prepared *PreparedRecoveryV1) OpenPreservingOriginalResiduesV1(ctx context.Context) (store *Store, resultErr error) {
	if prepared == nil || prepared.owner == nil || !prepared.owner.Present() {
		return nil, errors.New("attachment original opening requires a present complete owner")
	}
	for _, plan := range prepared.SecurePrivateCASRecoveryPlansV2() {
		if plan == nil || !plan.Present() {
			return nil, errors.New("attachment original opening requires every leaf to be present")
		}
	}
	if err := prepared.ValidateSemantics(ctx); err != nil {
		return nil, err
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return nil, err
	}
	opened := make([]*finalauthority.SecurePrivateCAS, 0, 5)
	defer func() {
		resultErr = errors.Join(resultErr, prepared.Revalidate(ctx))
		if resultErr != nil {
			for _, leaf := range opened {
				resultErr = errors.Join(resultErr, leaf.Close())
			}
			store = nil
		}
	}()
	candidate := &Store{root: prepared.root}
	for _, plan := range prepared.SecurePrivateCASRecoveryPlansV2() {
		leaf, err := plan.OpenPreservingOriginalResiduesV1(ctx)
		if err != nil {
			return nil, classifyAuthorityIntegrityError(err)
		}
		opened = append(opened, leaf)
		switch filepath.Base(plan.RootPath()) {
		case "owners":
			candidate.ownerCAS = leaf
		case "use-receipts":
			candidate.receiptCAS = leaf
		case "use-dispositions":
			candidate.dispositionCAS = leaf
		case "upload-intents":
			candidate.uploadIntentCAS = leaf
		case "upload-dispositions":
			candidate.uploadDispositionCAS = leaf
		default:
			return nil, errors.New("attachment original opening has an unknown leaf")
		}
	}
	if len(opened) != 5 || candidate.ownerCAS == nil || candidate.receiptCAS == nil || candidate.dispositionCAS == nil || candidate.uploadIntentCAS == nil || candidate.uploadDispositionCAS == nil {
		return nil, errors.New("attachment original opening is incomplete")
	}
	if err := candidate.validateInventory(ctx); err != nil {
		return nil, classifyAuthorityIntegrityError(err)
	}
	return candidate, nil
}
