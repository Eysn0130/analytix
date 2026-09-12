package pendingworkstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	pendingworkstoreport "analytix.local/runtime-go/internal/ports/pendingworkstore"
)

const maxRecordBytes = 1024 * 1024

// Production holds one cross-process persistence lease for the complete data
// root. These process gates additionally serialize pre-opened Store siblings
// used by one runtime so a two-root inventory cannot be split by a receipt or
// disposition commit from another handle in the same process.
var pendingWorkRootGates [64]sync.RWMutex

var ErrInventoryChanged = errors.New("private pending work inventory changed during snapshot")

type Store struct {
	mu             sync.Mutex
	rootGate       *sync.RWMutex
	root           string
	receipts       string
	dispositions   string
	receiptCAS     *finalauthorityadapter.SecurePrivateCAS
	dispositionCAS *finalauthorityadapter.SecurePrivateCAS
	// Test-only deterministic concurrency cut. Production leaves this nil.
	afterReceiptSnapshot func()
	beforeMutationGate   func()
}

func NewStore(
	root string,
	access finalauthorityadapter.SecurePrivateCASAccessAuthority,
) (*Store, error) {
	return NewStoreContext(context.Background(), root, access)
}

func NewStoreContext(
	ctx context.Context,
	root string,
	access finalauthorityadapter.SecurePrivateCASAccessAuthority,
) (*Store, error) {
	if access == nil {
		return nil, errors.New("private pending work access authority is required")
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("private pending work root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	receipts := filepath.Join(absolute, "receipts")
	receiptCAS, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthorityContext(ctx, receipts, maxRecordBytes, access)
	if err != nil {
		return nil, err
	}
	dispositions := filepath.Join(absolute, "dispositions")
	dispositionCAS, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthorityContext(ctx, dispositions, maxRecordBytes, access)
	if err != nil {
		return nil, err
	}
	store := &Store{
		root: absolute, rootGate: pendingWorkRootGate(absolute), receipts: receipts, dispositions: dispositions,
		receiptCAS: receiptCAS, dispositionCAS: dispositionCAS,
	}
	// Re-open both roots through their captured identities and validate their
	// complete inventories before publishing a usable store instance.
	store.rootGate.RLock()
	defer store.rootGate.RUnlock()
	if _, err := store.listReceipts(ctx); err != nil {
		return nil, err
	}
	if _, err := store.listDispositions(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

func PreflightRecovery(
	ctx context.Context,
	root string,
	access finalauthorityadapter.SecurePrivateCASRecoveryAccessAuthority,
) error {
	prepared, err := PrepareRecoveryV1(ctx, root, access)
	if err != nil {
		return err
	}
	return prepared.Revalidate(ctx)
}

func (store *Store) PutReceiptIfAbsent(ctx context.Context, receipt domainpendingwork.PendingWorkReceiptV1) error {
	_, err := store.putReceiptIfAbsent(ctx, receipt)
	return err
}

// CreateReceiptExclusive distinguishes the unique creator from an exact
// idempotent replay. That distinction is required by write-effect admission:
// only the creator may receive a process-local dispatch lease.
func (store *Store) CreateReceiptExclusive(ctx context.Context, receipt domainpendingwork.PendingWorkReceiptV1) (bool, error) {
	return store.putReceiptIfAbsent(ctx, receipt)
}

func (store *Store) putReceiptIfAbsent(ctx context.Context, receipt domainpendingwork.PendingWorkReceiptV1) (bool, error) {
	if store == nil || domainpendingwork.ValidatePendingWorkReceiptV1(receipt) != nil {
		return false, errors.New("private pending work receipt is invalid")
	}
	if err := contextError(ctx); err != nil {
		return false, err
	}
	body, err := domainpendingwork.PendingWorkReceiptV1Bytes(receipt)
	if err != nil || len(body) == 0 || len(body) > maxRecordBytes {
		return false, errors.New("private pending work receipt exceeds storage limit")
	}
	if store.beforeMutationGate != nil {
		store.beforeMutationGate()
	}
	store.rootGate.Lock()
	defer store.rootGate.Unlock()
	store.mu.Lock()
	defer store.mu.Unlock()
	if existing, err := store.readReceipt(ctx, receipt.WorkID); err == nil {
		existingBody, _ := domainpendingwork.PendingWorkReceiptV1Bytes(existing)
		if bytes.Equal(existingBody, body) {
			return false, nil
		}
		return false, errors.New("private pending work receipt conflicts with existing authority")
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := store.receiptCAS.PutIfAbsent(ctx, receipt.WorkID, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return false, err
		}
		existing, readErr := store.readReceipt(ctx, receipt.WorkID)
		if readErr != nil {
			return false, readErr
		}
		existingBody, bodyErr := domainpendingwork.PendingWorkReceiptV1Bytes(existing)
		if bodyErr == nil && bytes.Equal(existingBody, body) {
			return false, nil
		}
		return false, errors.New("private pending work receipt conflicts with concurrently created authority")
	}
	written, err := store.readReceipt(ctx, receipt.WorkID)
	if err != nil || written.ReceiptID != receipt.ReceiptID {
		return false, errors.New("private pending work receipt write verification failed")
	}
	return true, nil
}

func (store *Store) ReadReceipt(ctx context.Context, workID string) (domainpendingwork.PendingWorkReceiptV1, error) {
	if store == nil || !validDigest(workID) {
		return domainpendingwork.PendingWorkReceiptV1{}, errors.New("pending work identity is invalid")
	}
	if err := contextError(ctx); err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	store.rootGate.RLock()
	defer store.rootGate.RUnlock()
	store.mu.Lock()
	defer store.mu.Unlock()
	receipt, err := store.readReceipt(ctx, workID)
	if errors.Is(err, os.ErrNotExist) {
		return domainpendingwork.PendingWorkReceiptV1{}, pendingworkstoreport.ErrNotFound
	}
	if err == nil && receipt.WorkID != workID {
		return domainpendingwork.PendingWorkReceiptV1{}, errors.New("private pending work receipt content address is corrupt")
	}
	return receipt, err
}

func (store *Store) ListReceipts(ctx context.Context) ([]domainpendingwork.PendingWorkReceiptV1, error) {
	if store == nil {
		return nil, errors.New("private pending work store is unavailable")
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	store.rootGate.RLock()
	defer store.rootGate.RUnlock()
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.listReceipts(ctx)
}

func (store *Store) PutDispositionIfAbsent(ctx context.Context, disposition domainpendingwork.PendingWorkDispositionV1) error {
	if store == nil || domainpendingwork.ValidatePendingWorkDispositionV1(disposition) != nil {
		return errors.New("private pending work disposition is invalid")
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	body, err := domainpendingwork.PendingWorkDispositionV1Bytes(disposition)
	if err != nil || len(body) == 0 || len(body) > maxRecordBytes {
		return errors.New("private pending work disposition exceeds storage limit")
	}
	if store.beforeMutationGate != nil {
		store.beforeMutationGate()
	}
	store.rootGate.Lock()
	defer store.rootGate.Unlock()
	store.mu.Lock()
	defer store.mu.Unlock()
	receipt, err := store.readReceipt(ctx, disposition.WorkID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("private pending work disposition has no receipt authority")
		}
		return err
	}
	if err := domainpendingwork.ValidatePendingWorkDispositionForReceiptV1(disposition, receipt); err != nil {
		return errors.New("private pending work disposition does not match its receipt authority")
	}
	if existing, err := store.readDisposition(ctx, disposition.WorkID); err == nil {
		existingBody, _ := domainpendingwork.PendingWorkDispositionV1Bytes(existing)
		if bytes.Equal(existingBody, body) {
			return nil
		}
		return errors.New("private pending work was already closed by another disposition")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := store.dispositionCAS.PutIfAbsent(ctx, disposition.WorkID, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		existing, readErr := store.readDisposition(ctx, disposition.WorkID)
		if readErr != nil {
			return readErr
		}
		existingBody, bodyErr := domainpendingwork.PendingWorkDispositionV1Bytes(existing)
		if bodyErr == nil && bytes.Equal(existingBody, body) {
			return nil
		}
		return errors.New("private pending work was concurrently closed by another disposition")
	}
	written, err := store.readDisposition(ctx, disposition.WorkID)
	if err != nil || written.DispositionID != disposition.DispositionID {
		return errors.New("private pending work disposition write verification failed")
	}
	return nil
}

func (store *Store) ReadDisposition(ctx context.Context, workID string) (domainpendingwork.PendingWorkDispositionV1, error) {
	if store == nil || !validDigest(workID) {
		return domainpendingwork.PendingWorkDispositionV1{}, errors.New("pending work identity is invalid")
	}
	if err := contextError(ctx); err != nil {
		return domainpendingwork.PendingWorkDispositionV1{}, err
	}
	store.rootGate.RLock()
	defer store.rootGate.RUnlock()
	store.mu.Lock()
	defer store.mu.Unlock()
	disposition, err := store.readDisposition(ctx, workID)
	if errors.Is(err, os.ErrNotExist) {
		return domainpendingwork.PendingWorkDispositionV1{}, pendingworkstoreport.ErrNotFound
	}
	if err == nil {
		if disposition.WorkID != workID {
			return domainpendingwork.PendingWorkDispositionV1{}, errors.New("private pending work disposition content address is corrupt")
		}
		receipt, receiptErr := store.readReceipt(ctx, workID)
		if receiptErr != nil || domainpendingwork.ValidatePendingWorkDispositionForReceiptV1(disposition, receipt) != nil {
			return domainpendingwork.PendingWorkDispositionV1{}, errors.New("private pending work disposition lost its exact receipt authority")
		}
	}
	return disposition, err
}

func (store *Store) ListDispositions(ctx context.Context) ([]domainpendingwork.PendingWorkDispositionV1, error) {
	if store == nil {
		return nil, errors.New("private pending work store is unavailable")
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	store.rootGate.RLock()
	defer store.rootGate.RUnlock()
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.listDispositions(ctx)
}

func (store *Store) SnapshotInventory(ctx context.Context) ([]domainpendingwork.PendingWorkReceiptV1, []domainpendingwork.PendingWorkDispositionV1, error) {
	if store == nil {
		return nil, nil, errors.New("private pending work store is unavailable")
	}
	if err := contextError(ctx); err != nil {
		return nil, nil, err
	}
	store.rootGate.RLock()
	defer store.rootGate.RUnlock()
	store.mu.Lock()
	defer store.mu.Unlock()
	receipts, err := store.listReceipts(ctx)
	if err != nil {
		return nil, nil, err
	}
	if store.afterReceiptSnapshot != nil {
		store.afterReceiptSnapshot()
	}
	dispositions, err := store.listDispositionsForReceiptSnapshot(ctx, receipts)
	if err != nil {
		return nil, nil, err
	}
	// The CAS roots are append-only. Two equal ordered observations prove an
	// inventory point even if a non-production writer bypasses the runtime's
	// cross-process data-root lease; any intervening addition changes at least
	// one second observation and fails closed.
	receiptsConfirm, err := store.listReceipts(ctx)
	if err != nil {
		return nil, nil, err
	}
	dispositionsConfirm, err := store.listDispositionsForReceiptSnapshot(ctx, receiptsConfirm)
	if err != nil {
		return nil, nil, err
	}
	if !sameReceiptInventory(receipts, receiptsConfirm) || !sameDispositionInventory(dispositions, dispositionsConfirm) {
		return nil, nil, ErrInventoryChanged
	}
	return receipts, dispositions, nil
}

func (store *Store) HasRecords(ctx context.Context) (bool, error) {
	if store == nil {
		return false, errors.New("private pending work store is unavailable")
	}
	if err := contextError(ctx); err != nil {
		return false, err
	}
	store.rootGate.RLock()
	defer store.rootGate.RUnlock()
	store.mu.Lock()
	defer store.mu.Unlock()
	receipts, err := store.listReceipts(ctx)
	if err != nil {
		return false, err
	}
	dispositions, err := store.listDispositions(ctx)
	if err != nil {
		return false, err
	}
	return len(receipts)+len(dispositions) > 0, nil
}

func (store *Store) listReceipts(ctx context.Context) ([]domainpendingwork.PendingWorkReceiptV1, error) {
	files, err := store.receiptCAS.List(ctx)
	if err != nil {
		return nil, err
	}
	receipts := make([]domainpendingwork.PendingWorkReceiptV1, 0, len(files))
	for _, file := range files {
		receipt, err := domainpendingwork.ParsePendingWorkReceiptV1(file.Body)
		if err != nil || receipt.WorkID != file.Digest {
			return nil, errors.New("private pending work receipt filename does not match its content address")
		}
		receipts = append(receipts, receipt)
	}
	sort.Slice(receipts, func(i, j int) bool { return receipts[i].WorkID < receipts[j].WorkID })
	return receipts, nil
}

func (store *Store) listDispositions(ctx context.Context) ([]domainpendingwork.PendingWorkDispositionV1, error) {
	return store.listDispositionsWithReceiptReader(ctx, func(workID string) (domainpendingwork.PendingWorkReceiptV1, error) {
		return store.readReceipt(ctx, workID)
	})
}

// SnapshotInventory already holds both store gates and observes both CAS roots
// twice. Bind each disposition to the receipts from that same observation,
// rather than reopening and rescanning the whole receipt CAS for every entry.
// The confirmation pass builds a fresh index; nothing survives this snapshot.
func (store *Store) listDispositionsForReceiptSnapshot(ctx context.Context, receipts []domainpendingwork.PendingWorkReceiptV1) ([]domainpendingwork.PendingWorkDispositionV1, error) {
	byWork := make(map[string]domainpendingwork.PendingWorkReceiptV1, len(receipts))
	for _, receipt := range receipts {
		if _, duplicate := byWork[receipt.WorkID]; duplicate || !validDigest(receipt.WorkID) {
			return nil, errors.New("private pending work receipt snapshot identity is invalid")
		}
		byWork[receipt.WorkID] = receipt
	}
	return store.listDispositionsWithReceiptReader(ctx, func(workID string) (domainpendingwork.PendingWorkReceiptV1, error) {
		if err := contextError(ctx); err != nil {
			return domainpendingwork.PendingWorkReceiptV1{}, err
		}
		receipt, found := byWork[workID]
		if !found {
			return domainpendingwork.PendingWorkReceiptV1{}, pendingworkstoreport.ErrNotFound
		}
		return receipt, nil
	})
}

func (store *Store) listDispositionsWithReceiptReader(ctx context.Context, readReceipt func(string) (domainpendingwork.PendingWorkReceiptV1, error)) ([]domainpendingwork.PendingWorkDispositionV1, error) {
	files, err := store.dispositionCAS.List(ctx)
	if err != nil {
		return nil, err
	}
	dispositions := make([]domainpendingwork.PendingWorkDispositionV1, 0, len(files))
	for _, file := range files {
		disposition, err := domainpendingwork.ParsePendingWorkDispositionV1(file.Body)
		if err != nil || disposition.WorkID != file.Digest {
			return nil, errors.New("private pending work disposition filename does not match its content address")
		}
		receipt, err := readReceipt(file.Digest)
		if err != nil || domainpendingwork.ValidatePendingWorkDispositionForReceiptV1(disposition, receipt) != nil {
			return nil, errors.New("private pending work disposition lost its exact receipt authority")
		}
		dispositions = append(dispositions, disposition)
	}
	sort.Slice(dispositions, func(i, j int) bool { return dispositions[i].WorkID < dispositions[j].WorkID })
	return dispositions, nil
}

func (store *Store) receiptPath(workID string) string {
	return filepath.Join(store.receipts, workID[:2], workID+".json")
}

func (store *Store) dispositionPath(workID string) string {
	return filepath.Join(store.dispositions, workID[:2], workID+".json")
}

func (store *Store) readReceipt(ctx context.Context, workID string) (domainpendingwork.PendingWorkReceiptV1, error) {
	body, err := store.receiptCAS.Read(ctx, workID)
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	receipt, err := domainpendingwork.ParsePendingWorkReceiptV1(body)
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, fmt.Errorf("read private pending work receipt: %w", err)
	}
	return receipt, nil
}

func (store *Store) readDisposition(ctx context.Context, workID string) (domainpendingwork.PendingWorkDispositionV1, error) {
	body, err := store.dispositionCAS.Read(ctx, workID)
	if err != nil {
		return domainpendingwork.PendingWorkDispositionV1{}, err
	}
	disposition, err := domainpendingwork.ParsePendingWorkDispositionV1(body)
	if err != nil {
		return domainpendingwork.PendingWorkDispositionV1{}, fmt.Errorf("read private pending work disposition: %w", err)
	}
	return disposition, nil
}

func validDigest(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func sameReceiptInventory(left, right []domainpendingwork.PendingWorkReceiptV1) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		leftBody, leftErr := domainpendingwork.PendingWorkReceiptV1Bytes(left[index])
		rightBody, rightErr := domainpendingwork.PendingWorkReceiptV1Bytes(right[index])
		if leftErr != nil || rightErr != nil || !bytes.Equal(leftBody, rightBody) {
			return false
		}
	}
	return true
}

func sameDispositionInventory(left, right []domainpendingwork.PendingWorkDispositionV1) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		leftBody, leftErr := domainpendingwork.PendingWorkDispositionV1Bytes(left[index])
		rightBody, rightErr := domainpendingwork.PendingWorkDispositionV1Bytes(right[index])
		if leftErr != nil || rightErr != nil || !bytes.Equal(leftBody, rightBody) {
			return false
		}
	}
	return true
}

func pendingWorkRootGate(root string) *sync.RWMutex {
	digest := sha256.Sum256([]byte(root))
	return &pendingWorkRootGates[digest[0]&63]
}
