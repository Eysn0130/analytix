package piiauthorization

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
)

// AccessStoreV2 owns only schema-V2 controlled access authority. Production
// composition must give it the dedicated private/controlled-artifact-access-v2
// root; legacy V1 bytes are never upgraded or interpreted as V2 authority.
type AccessStoreV2 struct {
	receipts      *finalauthorityadapter.SecurePrivateCAS
	dispositions  *finalauthorityadapter.SecurePrivateCAS
	semanticStage bool

	closeOnce sync.Once
	closeErr  error
}

var _ piiauthorizationport.AccessStoreV2 = (*AccessStoreV2)(nil)
var _ piiauthorizationport.RestartAccessStoreV2 = (*AccessStoreV2)(nil)
var _ piiauthorizationport.AccessInventoryStoreV2 = (*AccessStoreV2)(nil)

func NewAccessStoreV2(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*AccessStoreV2, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil {
		return nil, errors.Join(piiauthorizationport.ErrCorrupt, errors.New("PII controlled access V2 store root is invalid"))
	}
	receipts, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(root, "access-receipts"), domainpii.MaxControlledArtifactAccessRecordBytesV2, access,
	)
	if err != nil {
		return nil, err
	}
	dispositions, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(root, "access-dispositions"), domainpii.MaxControlledArtifactAccessRecordBytesV2, access,
	)
	if err != nil {
		return nil, errors.Join(err, receipts.Close())
	}
	stage, stageBound := access.(interface {
		IsSemanticStagePrivateCASAccessAuthority() bool
	})
	return &AccessStoreV2{
		receipts: receipts, dispositions: dispositions,
		semanticStage: stageBound && stage.IsSemanticStagePrivateCASAccessAuthority(),
	}, nil
}

func (store *AccessStoreV2) IsSemanticStageControlledAccessStoreV2() bool {
	return store != nil && store.semanticStage
}

func (store *AccessStoreV2) Close() error {
	if store == nil {
		return nil
	}
	store.closeOnce.Do(func() {
		if store.dispositions != nil {
			store.closeErr = errors.Join(store.closeErr, store.dispositions.Close())
		}
		if store.receipts != nil {
			store.closeErr = errors.Join(store.closeErr, store.receipts.Close())
		}
	})
	return store.closeErr
}

func (store *AccessStoreV2) ReserveAccessReceiptV2(
	ctx context.Context,
	receipt domainpii.ControlledArtifactAccessReceiptV2,
) error {
	if store == nil || store.receipts == nil || domainpii.ValidateControlledArtifactAccessReceiptV2(receipt) != nil {
		return piiauthorizationport.ErrCorrupt
	}
	body, err := domainpii.ControlledArtifactAccessReceiptV2Bytes(receipt)
	if err != nil {
		return errors.Join(piiauthorizationport.ErrCorrupt, err)
	}
	return reserveControlledAccessReceiptV1(ctx, store.receipts, receipt.AccessID, body, func(candidate []byte) (string, error) {
		parsed, parseErr := domainpii.ParseControlledArtifactAccessReceiptV2(candidate)
		return parsed.AccessID, parseErr
	})
}

func (store *AccessStoreV2) ResolveAccessReceiptV2(
	ctx context.Context,
	accessID string,
) (domainpii.ControlledArtifactAccessReceiptV2, error) {
	body, err := readControlledAccessRecordV1(ctx, store.receipts, accessID)
	if err != nil {
		return domainpii.ControlledArtifactAccessReceiptV2{}, err
	}
	receipt, err := domainpii.ParseControlledArtifactAccessReceiptV2(body)
	if err != nil || receipt.AccessID != strings.TrimSpace(accessID) {
		return domainpii.ControlledArtifactAccessReceiptV2{}, errors.Join(piiauthorizationport.ErrCorrupt, err)
	}
	return receipt, nil
}

func (store *AccessStoreV2) PutAccessDispositionIfAbsentV2(
	ctx context.Context,
	disposition domainpii.ControlledArtifactAccessDispositionV2,
) error {
	if store == nil || store.dispositions == nil || domainpii.ValidateControlledArtifactAccessDispositionV2(disposition) != nil {
		return piiauthorizationport.ErrCorrupt
	}
	receipt, err := store.ResolveAccessReceiptV2(ctx, disposition.AccessID)
	if err != nil || domainpii.ValidateControlledArtifactAccessDispositionForReceiptV2(disposition, receipt) != nil {
		return errors.Join(piiauthorizationport.ErrCorrupt, err)
	}
	body, err := domainpii.ControlledArtifactAccessDispositionV2Bytes(disposition)
	if err != nil {
		return errors.Join(piiauthorizationport.ErrCorrupt, err)
	}
	return putControlledAccessDispositionV1(ctx, store.dispositions, disposition.AccessID, body, func(candidate []byte) (string, error) {
		parsed, parseErr := domainpii.ParseControlledArtifactAccessDispositionV2(candidate)
		return parsed.AccessID, parseErr
	})
}

func (store *AccessStoreV2) ResolveAccessDispositionV2(
	ctx context.Context,
	accessID string,
) (domainpii.ControlledArtifactAccessDispositionV2, error) {
	body, err := readControlledAccessRecordV1(ctx, store.dispositions, accessID)
	if err != nil {
		return domainpii.ControlledArtifactAccessDispositionV2{}, err
	}
	disposition, err := domainpii.ParseControlledArtifactAccessDispositionV2(body)
	if err != nil || disposition.AccessID != strings.TrimSpace(accessID) {
		return domainpii.ControlledArtifactAccessDispositionV2{}, errors.Join(piiauthorizationport.ErrCorrupt, err)
	}
	receipt, receiptErr := store.ResolveAccessReceiptV2(ctx, disposition.AccessID)
	if receiptErr != nil || domainpii.ValidateControlledArtifactAccessDispositionForReceiptV2(disposition, receipt) != nil {
		return domainpii.ControlledArtifactAccessDispositionV2{}, errors.Join(piiauthorizationport.ErrCorrupt, receiptErr)
	}
	return disposition, nil
}

func (store *AccessStoreV2) VisitAccessReceiptsV2(
	ctx context.Context,
	visit func(domainpii.ControlledArtifactAccessReceiptV2) error,
) error {
	if store == nil || store.receipts == nil || ctx == nil || visit == nil {
		return piiauthorizationport.ErrCorrupt
	}
	return store.receipts.Visit(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		receipt, err := domainpii.ParseControlledArtifactAccessReceiptV2(file.Body)
		if err != nil || receipt.AccessID != file.Digest {
			return errors.Join(piiauthorizationport.ErrCorrupt, err)
		}
		return visit(receipt)
	})
}

func (store *AccessStoreV2) VisitAccessDispositionsV2(
	ctx context.Context,
	visit func(domainpii.ControlledArtifactAccessDispositionV2) error,
) error {
	if store == nil || store.dispositions == nil || ctx == nil || visit == nil {
		return piiauthorizationport.ErrCorrupt
	}
	receipts := make(map[string]domainpii.ControlledArtifactAccessReceiptV2)
	if err := store.VisitAccessReceiptsV2(ctx, func(receipt domainpii.ControlledArtifactAccessReceiptV2) error {
		receipts[receipt.AccessID] = receipt
		return nil
	}); err != nil {
		return err
	}
	return store.dispositions.Visit(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		disposition, err := domainpii.ParseControlledArtifactAccessDispositionV2(file.Body)
		if err != nil || disposition.AccessID != file.Digest {
			return errors.Join(piiauthorizationport.ErrCorrupt, err)
		}
		receipt, ok := receipts[disposition.AccessID]
		if !ok || domainpii.ValidateControlledArtifactAccessDispositionForReceiptV2(disposition, receipt) != nil {
			return piiauthorizationport.ErrCorrupt
		}
		return visit(disposition)
	})
}

func (store *AccessStoreV2) HasRecordsV2(ctx context.Context) (bool, error) {
	if store == nil || store.receipts == nil || store.dispositions == nil || ctx == nil {
		return false, piiauthorizationport.ErrCorrupt
	}
	visitUntilFound := func(cas *finalauthorityadapter.SecurePrivateCAS) (bool, error) {
		found := errors.New("PII controlled access V2 record found")
		err := cas.Visit(ctx, func(finalauthorityadapter.SecurePrivateCASFile) error { return found })
		if errors.Is(err, found) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		return false, nil
	}
	if found, err := visitUntilFound(store.receipts); err != nil || found {
		return found, err
	}
	return visitUntilFound(store.dispositions)
}
