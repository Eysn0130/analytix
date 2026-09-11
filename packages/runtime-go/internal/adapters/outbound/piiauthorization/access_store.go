package piiauthorization

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
)

type AccessStore struct {
	receipts      *finalauthorityadapter.SecurePrivateCAS
	dispositions  *finalauthorityadapter.SecurePrivateCAS
	semanticStage bool

	closeOnce sync.Once
	closeErr  error
}

var _ piiauthorizationport.AccessStore = (*AccessStore)(nil)
var _ piiauthorizationport.RestartAccessStore = (*AccessStore)(nil)
var _ piiauthorizationport.AccessInventoryStore = (*AccessStore)(nil)

func NewAccessStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*AccessStore, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil {
		return nil, errors.Join(piiauthorizationport.ErrCorrupt, errors.New("PII controlled access store root is invalid"))
	}
	receipts, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(root, "access-receipts"), domainpii.MaxControlledArtifactAccessRecordBytesV1, access,
	)
	if err != nil {
		return nil, err
	}
	dispositions, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(root, "access-dispositions"), domainpii.MaxControlledArtifactAccessRecordBytesV1, access,
	)
	if err != nil {
		return nil, errors.Join(err, receipts.Close())
	}
	stage, stageBound := access.(interface {
		IsSemanticStagePrivateCASAccessAuthority() bool
	})
	return &AccessStore{
		receipts: receipts, dispositions: dispositions,
		semanticStage: stageBound && stage.IsSemanticStagePrivateCASAccessAuthority(),
	}, nil
}

func (store *AccessStore) IsSemanticStageControlledAccessStore() bool {
	return store != nil && store.semanticStage
}

func (store *AccessStore) Close() error {
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

func (store *AccessStore) ReserveAccessReceipt(ctx context.Context, receipt domainpii.ControlledArtifactAccessReceiptV1) error {
	if store == nil || store.receipts == nil || domainpii.ValidateControlledArtifactAccessReceiptV1(receipt) != nil {
		return piiauthorizationport.ErrCorrupt
	}
	body, err := domainpii.ControlledArtifactAccessReceiptV1Bytes(receipt)
	if err != nil {
		return errors.Join(piiauthorizationport.ErrCorrupt, err)
	}
	return reserveControlledAccessReceiptV1(ctx, store.receipts, receipt.AccessID, body, func(candidate []byte) (string, error) {
		parsed, parseErr := domainpii.ParseControlledArtifactAccessReceiptV1(candidate)
		return parsed.AccessID, parseErr
	})
}

func (store *AccessStore) ResolveAccessReceipt(ctx context.Context, accessID string) (domainpii.ControlledArtifactAccessReceiptV1, error) {
	body, err := readControlledAccessRecordV1(ctx, store.receipts, accessID)
	if err != nil {
		return domainpii.ControlledArtifactAccessReceiptV1{}, err
	}
	receipt, err := domainpii.ParseControlledArtifactAccessReceiptV1(body)
	if err != nil || receipt.AccessID != strings.TrimSpace(accessID) {
		return domainpii.ControlledArtifactAccessReceiptV1{}, errors.Join(piiauthorizationport.ErrCorrupt, err)
	}
	return receipt, nil
}

func (store *AccessStore) PutAccessDispositionIfAbsent(ctx context.Context, disposition domainpii.ControlledArtifactAccessDispositionV1) error {
	if store == nil || store.dispositions == nil || domainpii.ValidateControlledArtifactAccessDispositionV1(disposition) != nil {
		return piiauthorizationport.ErrCorrupt
	}
	receipt, err := store.ResolveAccessReceipt(ctx, disposition.AccessID)
	if err != nil || domainpii.ValidateControlledArtifactAccessDispositionForReceiptV1(disposition, receipt) != nil {
		return errors.Join(piiauthorizationport.ErrCorrupt, err)
	}
	body, err := domainpii.ControlledArtifactAccessDispositionV1Bytes(disposition)
	if err != nil {
		return errors.Join(piiauthorizationport.ErrCorrupt, err)
	}
	return putControlledAccessDispositionV1(ctx, store.dispositions, disposition.AccessID, body, func(candidate []byte) (string, error) {
		parsed, parseErr := domainpii.ParseControlledArtifactAccessDispositionV1(candidate)
		return parsed.AccessID, parseErr
	})
}

func (store *AccessStore) ResolveAccessDisposition(ctx context.Context, accessID string) (domainpii.ControlledArtifactAccessDispositionV1, error) {
	body, err := readControlledAccessRecordV1(ctx, store.dispositions, accessID)
	if err != nil {
		return domainpii.ControlledArtifactAccessDispositionV1{}, err
	}
	disposition, err := domainpii.ParseControlledArtifactAccessDispositionV1(body)
	if err != nil || disposition.AccessID != strings.TrimSpace(accessID) {
		return domainpii.ControlledArtifactAccessDispositionV1{}, errors.Join(piiauthorizationport.ErrCorrupt, err)
	}
	receipt, receiptErr := store.ResolveAccessReceipt(ctx, disposition.AccessID)
	if receiptErr != nil || domainpii.ValidateControlledArtifactAccessDispositionForReceiptV1(disposition, receipt) != nil {
		return domainpii.ControlledArtifactAccessDispositionV1{}, errors.Join(piiauthorizationport.ErrCorrupt, receiptErr)
	}
	return disposition, nil
}

func (store *AccessStore) VisitAccessReceipts(ctx context.Context, visit func(domainpii.ControlledArtifactAccessReceiptV1) error) error {
	if store == nil || store.receipts == nil || ctx == nil || visit == nil {
		return piiauthorizationport.ErrCorrupt
	}
	return store.receipts.Visit(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		receipt, err := domainpii.ParseControlledArtifactAccessReceiptV1(file.Body)
		if err != nil || receipt.AccessID != file.Digest {
			return errors.Join(piiauthorizationport.ErrCorrupt, err)
		}
		return visit(receipt)
	})
}

func (store *AccessStore) VisitAccessDispositions(ctx context.Context, visit func(domainpii.ControlledArtifactAccessDispositionV1) error) error {
	if store == nil || store.dispositions == nil || ctx == nil || visit == nil {
		return piiauthorizationport.ErrCorrupt
	}
	receipts := make(map[string]domainpii.ControlledArtifactAccessReceiptV1)
	if err := store.VisitAccessReceipts(ctx, func(receipt domainpii.ControlledArtifactAccessReceiptV1) error {
		receipts[receipt.AccessID] = receipt
		return nil
	}); err != nil {
		return err
	}
	return store.dispositions.Visit(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		disposition, err := domainpii.ParseControlledArtifactAccessDispositionV1(file.Body)
		if err != nil || disposition.AccessID != file.Digest {
			return errors.Join(piiauthorizationport.ErrCorrupt, err)
		}
		receipt, ok := receipts[disposition.AccessID]
		if !ok || domainpii.ValidateControlledArtifactAccessDispositionForReceiptV1(disposition, receipt) != nil {
			return piiauthorizationport.ErrCorrupt
		}
		return visit(disposition)
	})
}

func (store *AccessStore) HasRecords(ctx context.Context) (bool, error) {
	if store == nil || store.receipts == nil || store.dispositions == nil {
		return false, piiauthorizationport.ErrCorrupt
	}
	stores := make([]*finalauthorityadapter.SecurePrivateCAS, 0, 2)
	stores = append(stores, store.receipts, store.dispositions)
	for _, cas := range stores {
		found := errors.New("PII controlled access record found")
		err := cas.Visit(ctx, func(finalauthorityadapter.SecurePrivateCASFile) error { return found })
		if errors.Is(err, found) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
	}
	return false, nil
}

func reserveControlledAccessReceiptV1(
	ctx context.Context,
	cas *finalauthorityadapter.SecurePrivateCAS,
	key string,
	body []byte,
	parseKey func([]byte) (string, error),
) error {
	if cas == nil || key == "" || key != strings.TrimSpace(key) || !domainsecurity.IsSHA256Hex(key) || len(body) == 0 || parseKey == nil {
		return piiauthorizationport.ErrCorrupt
	}
	addition, err := cas.PutIfAbsentWithAdditionReceipt(ctx, key, body)
	if errors.Is(err, os.ErrExist) {
		return classifyControlledAccessExistingV1(ctx, cas, key, body, parseKey, piiauthorizationport.ErrAlreadyReserved)
	}
	if err != nil {
		return err
	}
	finalized, err := cas.FinalizeCommittedAdditions(ctx, []finalauthorityadapter.SecurePrivateCASAdditionReceiptV2{addition})
	if err != nil || len(finalized) != 1 || !finalized[0].CreatedByThisCall || !finalized[0].Finalized || finalized[0].RecordDigest != key {
		return errors.Join(piiauthorizationport.ErrIndeterminate, err)
	}
	written, err := cas.Read(ctx, key)
	if err != nil || !bytes.Equal(written, body) {
		return errors.Join(piiauthorizationport.ErrIndeterminate, err)
	}
	return nil
}

func putControlledAccessDispositionV1(
	ctx context.Context,
	cas *finalauthorityadapter.SecurePrivateCAS,
	key string,
	body []byte,
	parseKey func([]byte) (string, error),
) error {
	if cas == nil || key == "" || key != strings.TrimSpace(key) || !domainsecurity.IsSHA256Hex(key) || len(body) == 0 || parseKey == nil {
		return piiauthorizationport.ErrCorrupt
	}
	if _, err := cas.Read(ctx, key); err == nil {
		return classifyControlledAccessExistingV1(ctx, cas, key, body, parseKey, nil)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := cas.PutIfAbsent(ctx, key, body); err != nil {
		if errors.Is(err, os.ErrExist) {
			return classifyControlledAccessExistingV1(ctx, cas, key, body, parseKey, nil)
		}
		return errors.Join(piiauthorizationport.ErrIndeterminate, err)
	}
	written, err := cas.Read(ctx, key)
	parsedKey, parseErr := parseKey(written)
	if err != nil {
		return errors.Join(piiauthorizationport.ErrIndeterminate, err)
	}
	if parseErr != nil || parsedKey != key || !bytes.Equal(written, body) {
		return errors.Join(piiauthorizationport.ErrCorrupt, parseErr)
	}
	return nil
}

func classifyControlledAccessExistingV1(
	ctx context.Context,
	cas *finalauthorityadapter.SecurePrivateCAS,
	key string,
	body []byte,
	parseKey func([]byte) (string, error),
	equalError error,
) error {
	current, err := cas.Read(ctx, key)
	if err != nil {
		return err
	}
	parsedKey, parseErr := parseKey(current)
	if parseErr != nil || parsedKey != key {
		return errors.Join(piiauthorizationport.ErrCorrupt, parseErr)
	}
	if !bytes.Equal(current, body) {
		return piiauthorizationport.ErrConflict
	}
	return equalError
}

func readControlledAccessRecordV1(ctx context.Context, cas *finalauthorityadapter.SecurePrivateCAS, key string) ([]byte, error) {
	if cas == nil || key == "" || key != strings.TrimSpace(key) || !domainsecurity.IsSHA256Hex(key) {
		return nil, piiauthorizationport.ErrNotFound
	}
	body, err := cas.Read(ctx, key)
	if errors.Is(err, os.ErrNotExist) {
		return nil, piiauthorizationport.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return body, nil
}
