package backendgenerationfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainbackend "analytix.local/runtime-go/internal/domain/backendgeneration"
	backendport "analytix.local/runtime-go/internal/ports/backendgeneration"
)

const MaxAllocationInventoryV1 = 1_000_000

const authorityDirectoryNameV1 = "runtime-sidecar-authority-v1"

var transactionGatesV1 sync.Map

type StoreV1 struct {
	root string
	cas  *finalauthority.SecurePrivateCAS
	gate *sync.Mutex
}

var _ backendport.StoreV1 = (*StoreV1)(nil)

func AllocationRootForUserDataV1(userData string) (string, error) {
	userData, err := canonicalRootV1(userData)
	if err != nil {
		return "", err
	}
	return filepath.Join(userData, "private", authorityDirectoryNameV1, "allocations"), nil
}

func OpenStoreV1(
	ctx context.Context,
	root string,
	access finalauthority.SecurePrivateCASAccessAuthority,
) (*StoreV1, error) {
	root, err := canonicalRootV1(root)
	if err != nil || access == nil {
		return nil, errors.New("backend generation store configuration is invalid")
	}
	cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthorityContext(
		ctx, root, domainbackend.MaxAllocationRecordBytesV1, access,
	)
	if err != nil {
		return nil, err
	}
	gate, _ := transactionGatesV1.LoadOrStore(root, &sync.Mutex{})
	return &StoreV1{root: root, cas: cas, gate: gate.(*sync.Mutex)}, nil
}

func (store *StoreV1) Close() error {
	if store == nil || store.gate == nil {
		return nil
	}
	store.gate.Lock()
	defer store.gate.Unlock()
	if store.cas == nil {
		return nil
	}
	err := store.cas.Close()
	store.cas = nil
	return err
}

func (store *StoreV1) WithExclusive(
	ctx context.Context,
	use func(backendport.TransactionV1) error,
) error {
	if store == nil || store.gate == nil || use == nil {
		return backendport.ErrCorrupt
	}
	if ctx == nil {
		return errors.New("backend generation transaction context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	store.gate.Lock()
	defer store.gate.Unlock()
	if store.cas == nil {
		return backendport.ErrCorrupt
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return use(transactionV1{cas: store.cas})
}

type transactionV1 struct {
	cas *finalauthority.SecurePrivateCAS
}

func (transaction transactionV1) Visit(
	ctx context.Context,
	visit func(backendport.StoredAllocationV1) error,
) error {
	if transaction.cas == nil || visit == nil {
		return backendport.ErrCorrupt
	}
	count := 0
	err := transaction.cas.Visit(ctx, func(file finalauthority.SecurePrivateCASFile) error {
		count++
		if count > MaxAllocationInventoryV1 {
			return backendport.ErrCorrupt
		}
		return visit(backendport.StoredAllocationV1{
			Digest: file.Digest, Body: append([]byte(nil), file.Body...),
		})
	})
	return normalizeStoreErrorV1(err)
}

func (transaction transactionV1) PutIfAbsent(
	ctx context.Context,
	digest string,
	body []byte,
) error {
	if transaction.cas == nil {
		return backendport.ErrCorrupt
	}
	provisional, err := transaction.cas.PutIfAbsentWithAdditionReceipt(ctx, digest, body)
	if err != nil {
		return normalizeStoreErrorV1(err)
	}
	finalized, err := transaction.cas.FinalizeCommittedAdditions(
		ctx, []finalauthority.SecurePrivateCASAdditionReceiptV2{provisional},
	)
	if err != nil || len(finalized) != 1 {
		return errors.Join(backendport.ErrCorrupt, err)
	}
	if err := transaction.cas.VerifyCommittedAddition(ctx, finalized[0]); err != nil {
		return errors.Join(backendport.ErrCorrupt, err)
	}
	return nil
}

func (transaction transactionV1) Resolve(ctx context.Context, digest string) ([]byte, error) {
	if transaction.cas == nil {
		return nil, backendport.ErrCorrupt
	}
	body, err := transaction.cas.Read(ctx, digest)
	if err != nil {
		return nil, normalizeStoreErrorV1(err)
	}
	return body, nil
}

func normalizeStoreErrorV1(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, os.ErrExist):
		return errors.Join(backendport.ErrConflict, err)
	case errors.Is(err, os.ErrNotExist):
		return errors.Join(backendport.ErrNotFound, err)
	default:
		return errors.Join(backendport.ErrCorrupt, err)
	}
}

func canonicalRootV1(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("backend generation root is empty")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return "", errors.New("backend generation root is invalid")
	}
	return absolute, nil
}
