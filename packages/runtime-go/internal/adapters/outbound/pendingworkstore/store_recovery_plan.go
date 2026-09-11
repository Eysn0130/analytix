package pendingworkstore

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
)

type PreparedRecoveryV1 struct {
	owner     *finalauthorityadapter.PreparedSecurePrivateCASOwnerRecoveryV1
	validated bool
}

func PrepareRecoveryV1(
	ctx context.Context,
	root string,
	access finalauthorityadapter.SecurePrivateCASRecoveryAccessAuthority,
) (*PreparedRecoveryV1, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil {
		return nil, errors.New("private pending work prepared recovery root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, errors.New("private pending work prepared recovery root is invalid")
	}
	owner, err := finalauthorityadapter.PrepareSecurePrivateCASOwnerRecoveryV1(
		ctx, absolute, []finalauthorityadapter.SecurePrivateCASOwnerLeafV1{
			{Name: "receipts", MaxBytes: maxRecordBytes},
			{Name: "dispositions", MaxBytes: maxRecordBytes},
		}, access,
	)
	if err != nil {
		return nil, err
	}
	return &PreparedRecoveryV1{owner: owner}, nil
}

func (prepared *PreparedRecoveryV1) ValidateSemantics(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil {
		return errors.New("private pending work recovery plan is invalid")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if err := validatePreparedRecoveryOwnerV1(ctx, prepared.owner); err != nil {
		return err
	}
	prepared.validated = true
	return nil
}

func validatePreparedRecoveryOwnerV1(ctx context.Context, owner *finalauthorityadapter.PreparedSecurePrivateCASOwnerRecoveryV1) error {
	_, _, err := preparedRecoveryInventoryV1(ctx, owner)
	return err
}

// SnapshotInventory exposes only immutable committed input from the prepared
// owner. It does not open a live CAS, recover a residue, or create an empty
// replacement. The app owner must still verify the installation signatures.
func (prepared *PreparedRecoveryV1) SnapshotInventory(ctx context.Context) ([]domainpendingwork.PendingWorkReceiptV1, []domainpendingwork.PendingWorkDispositionV1, error) {
	if prepared == nil || prepared.owner == nil || ctx == nil || ctx.Err() != nil {
		return nil, nil, errors.New("private pending work prepared snapshot is unavailable")
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return nil, nil, err
	}
	receipts, dispositions, err := preparedRecoveryInventoryV1(ctx, prepared.owner)
	if err != nil {
		return nil, nil, err
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return nil, nil, err
	}
	return receipts, dispositions, nil
}

func preparedRecoveryInventoryV1(ctx context.Context, owner *finalauthorityadapter.PreparedSecurePrivateCASOwnerRecoveryV1) ([]domainpendingwork.PendingWorkReceiptV1, []domainpendingwork.PendingWorkDispositionV1, error) {
	receipts, dispositions, err := readCanonicalPreparedRecoveryRecordsV1(ctx, owner)
	if err != nil {
		return nil, nil, err
	}
	byID := make(map[string]domainpendingwork.PendingWorkReceiptV1, len(receipts))
	for _, receipt := range receipts {
		byID[receipt.WorkID] = receipt
	}
	for _, disposition := range dispositions {
		receipt, found := byID[disposition.WorkID]
		if !found || domainpendingwork.ValidatePendingWorkDispositionForReceiptV1(disposition, receipt) != nil {
			return nil, nil, errors.New("private pending work prepared disposition lost its exact receipt authority")
		}
	}
	return receipts, dispositions, nil
}

// SnapshotCanonicalRecordsV1 observes every physical canonical record without
// claiming complete cross-leaf or installation authority. A caller explaining
// a signed interrupted transaction must retain this physical input separately
// and verify complete original and final graphs before any effects.
func (prepared *PreparedRecoveryV1) SnapshotCanonicalRecordsV1(ctx context.Context) ([]domainpendingwork.PendingWorkReceiptV1, []domainpendingwork.PendingWorkDispositionV1, error) {
	if prepared == nil || prepared.owner == nil || ctx == nil {
		return nil, nil, errors.New("private pending canonical observation is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return nil, nil, err
	}
	receipts, dispositions, err := readCanonicalPreparedRecoveryRecordsV1(ctx, prepared.owner)
	if err != nil {
		return nil, nil, err
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return nil, nil, err
	}
	return receipts, dispositions, nil
}

func readCanonicalPreparedRecoveryRecordsV1(ctx context.Context, owner *finalauthorityadapter.PreparedSecurePrivateCASOwnerRecoveryV1) ([]domainpendingwork.PendingWorkReceiptV1, []domainpendingwork.PendingWorkDispositionV1, error) {
	if owner == nil {
		return nil, nil, errors.New("private pending work prepared owner is unavailable")
	}
	if !owner.Present() {
		return nil, nil, nil
	}
	receipts := make(map[string]domainpendingwork.PendingWorkReceiptV1)
	orderedReceipts := []domainpendingwork.PendingWorkReceiptV1{}
	dispositions := []domainpendingwork.PendingWorkDispositionV1{}
	if err := owner.VisitCommittedFiles(ctx, "receipts", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		receipt, err := domainpendingwork.ParsePendingWorkReceiptV1(file.Body)
		canonical, canonicalErr := domainpendingwork.PendingWorkReceiptV1Bytes(receipt)
		if err != nil || canonicalErr != nil || receipt.WorkID != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("private pending work prepared receipt is non-canonical or content-address corrupt"), err, canonicalErr)
		}
		if _, duplicate := receipts[file.Digest]; duplicate {
			return errors.New("private pending work canonical receipt inventory contains a duplicate address")
		}
		receipts[file.Digest] = receipt
		orderedReceipts = append(orderedReceipts, receipt)
		return nil
	}); err != nil {
		return nil, nil, err
	}
	if err := owner.VisitCommittedFiles(ctx, "dispositions", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		disposition, err := domainpendingwork.ParsePendingWorkDispositionV1(file.Body)
		canonical, canonicalErr := domainpendingwork.PendingWorkDispositionV1Bytes(disposition)
		if err != nil || canonicalErr != nil || disposition.WorkID != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("private pending work prepared disposition lost its exact receipt authority"), err, canonicalErr)
		}
		dispositions = append(dispositions, disposition)
		return nil
	}); err != nil {
		return nil, nil, err
	}
	return orderedReceipts, dispositions, nil
}

func (prepared *PreparedRecoveryV1) Revalidate(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil {
		return errors.New("private pending work recovery plan is invalid")
	}
	return prepared.owner.Revalidate(ctx)
}

func (prepared *PreparedRecoveryV1) PrivateCASRecoveryTopologiesV3() []finalauthorityadapter.SecurePrivateCASRecoveryTopologyAuthorityV3 {
	if prepared == nil || prepared.owner == nil {
		return nil
	}
	return []finalauthorityadapter.SecurePrivateCASRecoveryTopologyAuthorityV3{prepared.owner}
}

func (prepared *PreparedRecoveryV1) SecurePrivateCASRecoveryPlansV2() []*finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1 {
	if prepared == nil || prepared.owner == nil {
		return nil
	}
	return prepared.owner.SecurePrivateCASRecoveryPlansV2()
}
