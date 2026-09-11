package attachmentauthority

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
)

type preparedAttachmentRecoveryOwnerV1 interface {
	originalAttachmentPhysicalOwnerV1
	finalauthorityadapter.PreparedSecurePrivateCASRecoveryAuthorityV3
}

type PreparedRecoveryV1 struct {
	root      string
	owner     preparedAttachmentRecoveryOwnerV1
	validated bool
}

func PrepareRecoveryV1(
	ctx context.Context,
	root string,
	access finalauthorityadapter.SecurePrivateCASRecoveryAccessAuthority,
) (*PreparedRecoveryV1, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil {
		return nil, errors.New("attachment authority prepared recovery root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, errors.New("attachment authority prepared recovery root is invalid")
	}
	owner, err := finalauthorityadapter.PrepareSecurePrivateCASOwnerRecoveryV1(
		ctx, absolute, []finalauthorityadapter.SecurePrivateCASOwnerLeafV1{
			{Name: "owners", MaxBytes: domainattachment.MaxOwnerRecordBytesV1},
			{Name: "use-receipts", MaxBytes: domainattachment.MaxAttachmentUseRecordBytesV1},
			{Name: "use-dispositions", MaxBytes: domainattachment.MaxAttachmentUseRecordBytesV1},
			{Name: "upload-intents", MaxBytes: domainattachment.MaxAttachmentUploadRecordBytesV1},
			{Name: "upload-dispositions", MaxBytes: domainattachment.MaxAttachmentUploadRecordBytesV1},
		}, access,
	)
	if err != nil {
		return nil, err
	}
	return &PreparedRecoveryV1{root: absolute, owner: owner}, nil
}

func (prepared *PreparedRecoveryV1) ValidateSemantics(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil {
		return errors.New("attachment authority recovery plan is invalid")
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

func validatePreparedRecoveryOwnerV1(
	ctx context.Context,
	owner originalAttachmentPhysicalOwnerV1,
) error {
	if owner == nil || !owner.Present() {
		return nil
	}
	return validateOriginalAttachmentRecordsV1(ctx, func(leaf string, visit func(finalauthorityadapter.SecurePrivateCASFile) error) error {
		return owner.VisitCommittedFiles(ctx, leaf, visit)
	})
}

func validateOriginalAttachmentRecordsV1(ctx context.Context, visit func(string, func(finalauthorityadapter.SecurePrivateCASFile) error) error) error {
	owners := map[string]domainattachment.OwnerRecordV1{}
	attachmentOwners := map[string]string{}
	if err := visit("owners", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		record, err := parseCanonicalOwner(file.Body)
		if err != nil || record.OwnerDigest != file.Digest {
			return errors.Join(errors.New("attachment prepared owner filename is not its semantic digest"), err)
		}
		if existing := attachmentOwners[record.AttachmentID]; existing != "" && existing != record.OwnerDigest {
			return errors.New("attachment prepared owner id collision is ambiguous")
		}
		owners[record.OwnerDigest] = record
		attachmentOwners[record.AttachmentID] = record.OwnerDigest
		return nil
	}); err != nil {
		return err
	}
	intents := map[string]domainattachment.UploadIntentV1{}
	if err := visit("upload-intents", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		intent, err := parseCanonicalUploadIntent(file.Body)
		if err != nil || intent.UploadID != file.Digest {
			return errors.Join(errors.New("attachment prepared upload intent filename is not its semantic identity"), err)
		}
		if existing, ok := intents[intent.UploadID]; ok && !sameUploadIntent(existing, intent) {
			return errors.New("attachment prepared upload intent inventory conflicts")
		}
		if record, ok := owners[intent.Owner.OwnerDigest]; ok && !uploadIntentMatchesOwner(intent, record) {
			return errors.New("attachment prepared upload intent owner inventory conflicts")
		}
		intents[intent.UploadID] = intent
		return nil
	}); err != nil {
		return err
	}
	if err := visit("upload-dispositions", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		disposition, err := parseCanonicalUploadDisposition(file.Body)
		if err != nil || disposition.UploadID != file.Digest {
			return errors.Join(errors.New("attachment prepared upload disposition filename is not its upload identity"), err)
		}
		intent, ok := intents[disposition.UploadID]
		if !ok || domainattachment.ValidateUploadDispositionForIntentV1(disposition, intent) != nil {
			return errors.New("attachment prepared upload disposition intent inventory is incomplete")
		}
		record, hasOwner := owners[intent.Owner.OwnerDigest]
		if disposition.Status == domainattachment.UploadDispositionCommittedV1 {
			if !hasOwner || !uploadIntentMatchesOwner(intent, record) {
				return errors.New("committed attachment prepared upload owner inventory is incomplete")
			}
		} else if hasOwner {
			return errors.New("non-committed attachment prepared upload has owner authority")
		}
		return nil
	}); err != nil {
		return err
	}
	receipts := map[string]domainattachment.AttachmentUseReceiptV1{}
	if err := visit("use-receipts", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		receipt, err := domainattachment.ParseAttachmentUseReceiptV1(file.Body)
		canonical, canonicalErr := domainattachment.AttachmentUseReceiptV1Bytes(receipt)
		if err != nil || canonicalErr != nil || receipt.UseID != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("attachment prepared use receipt is non-canonical or content-address corrupt"), err, canonicalErr)
		}
		record, ok := owners[receipt.OwnerDigest]
		if !ok || !receiptMatchesOwner(receipt, record) {
			return errors.New("attachment prepared use receipt owner inventory is incomplete")
		}
		receipts[receipt.UseID] = receipt
		return nil
	}); err != nil {
		return err
	}
	return visit("use-dispositions", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		disposition, err := domainattachment.ParseAttachmentUseDispositionV1(file.Body)
		canonical, canonicalErr := domainattachment.AttachmentUseDispositionV1Bytes(disposition)
		if err != nil || canonicalErr != nil || disposition.UseID != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("attachment prepared use disposition is non-canonical or content-address corrupt"), err, canonicalErr)
		}
		receipt, ok := receipts[disposition.UseID]
		if !ok || domainattachment.ValidateAttachmentUseDispositionForReceiptV1(disposition, receipt) != nil {
			return errors.New("attachment prepared use disposition receipt inventory is incomplete")
		}
		return nil
	})
}

func (prepared *PreparedRecoveryV1) Revalidate(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil {
		return errors.New("attachment authority recovery plan is invalid")
	}
	return prepared.owner.Revalidate(ctx)
}

func (prepared *PreparedRecoveryV1) PrivateCASRecoveryTopologiesV3() []finalauthorityadapter.SecurePrivateCASRecoveryTopologyAuthorityV3 {
	if prepared == nil || prepared.owner == nil {
		return nil
	}
	return prepared.owner.PrivateCASRecoveryTopologiesV3()
}

func (prepared *PreparedRecoveryV1) SecurePrivateCASRecoveryPlansV2() []*finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1 {
	if prepared == nil || prepared.owner == nil {
		return nil
	}
	return prepared.owner.SecurePrivateCASRecoveryPlansV2()
}
