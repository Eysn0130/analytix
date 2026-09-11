package continuationstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	continuationstoreport "analytix.local/runtime-go/internal/ports/continuationstore"
)

const (
	maxRecordBytes         = 6 * 1024 * 1024
	receiptPartitionV2     = "receipts-v2"
	dispositionPartitionV2 = "dispositions-v2"
)

type Store struct {
	mu             sync.Mutex
	root           string
	receipts       string
	dispositions   string
	receiptCAS     *finalauthorityadapter.SecurePrivateCAS
	dispositionCAS *finalauthorityadapter.SecurePrivateCAS
}

func NewStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*Store, error) {
	return NewStoreContext(context.Background(), root, access)
}

func NewStoreContext(
	ctx context.Context,
	root string,
	access finalauthorityadapter.SecurePrivateCASAccessAuthority,
) (*Store, error) {
	return openStoreContext(ctx, root, access, false)
}

func openStoreContext(
	ctx context.Context,
	root string,
	access finalauthorityadapter.SecurePrivateCASAccessAuthority,
	allowLegacy bool,
) (*Store, error) {
	if access == nil {
		return nil, errors.New("private continuation access authority is required")
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("private continuation root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, errors.New("private continuation root is invalid")
	}
	legacy, err := legacyPartitionsPresent(absolute)
	if err != nil {
		return nil, err
	}
	if legacy && !allowLegacy {
		return nil, errors.New("legacy continuation authority requires semantic-stage migration")
	}
	receipts := filepath.Join(absolute, receiptPartitionV2)
	receiptCAS, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthorityContext(ctx, receipts, maxRecordBytes, access)
	if err != nil {
		return nil, err
	}
	dispositions := filepath.Join(absolute, dispositionPartitionV2)
	dispositionCAS, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthorityContext(ctx, dispositions, maxRecordBytes, access)
	if err != nil {
		_ = receiptCAS.Close()
		return nil, err
	}
	store := &Store{
		root: absolute, receipts: receipts, dispositions: dispositions,
		receiptCAS: receiptCAS, dispositionCAS: dispositionCAS,
	}
	if err := validateContinuationOwnerRoot(absolute, allowLegacy); err != nil {
		_ = store.Close()
		return nil, err
	}
	store.mu.Lock()
	_, _, err = store.loadInventoryLocked(ctx)
	store.mu.Unlock()
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

func (store *Store) Close() error {
	if store == nil {
		return nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return errors.Join(store.receiptCAS.Close(), store.dispositionCAS.Close())
}

func (store *Store) PutReceiptIfAbsent(ctx context.Context, receipt domaincontinuation.Receipt) error {
	if store == nil || domaincontinuation.ValidateReceipt(receipt) != nil {
		return errors.New("private continuation receipt is invalid")
	}
	body, err := domaincontinuation.ReceiptBytes(receipt)
	if err != nil || len(body) == 0 || len(body) > maxRecordBytes {
		return errors.New("private continuation receipt exceeds storage limit")
	}
	digest := gateIDDigest(receipt.Payload.GateID)
	if err := contextError(ctx); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if existing, err := store.readReceiptLocked(ctx, receipt.Payload.GateID); err == nil {
		existingBody, _ := domaincontinuation.ReceiptBytes(existing)
		if bytes.Equal(existingBody, body) {
			return nil
		}
		return errors.New("private continuation receipt conflicts with existing authority")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := store.receiptCAS.PutIfAbsent(ctx, digest, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		existing, readErr := store.readReceiptLocked(ctx, receipt.Payload.GateID)
		if readErr != nil {
			return readErr
		}
		existingBody, bodyErr := domaincontinuation.ReceiptBytes(existing)
		if bodyErr == nil && bytes.Equal(existingBody, body) {
			return nil
		}
		return errors.New("private continuation receipt conflicts with concurrently created authority")
	}
	written, err := store.readReceiptLocked(ctx, receipt.Payload.GateID)
	if err != nil || written.ReceiptID != receipt.ReceiptID {
		return errors.New("private continuation receipt write verification failed")
	}
	return nil
}

func (store *Store) ResolveReceipt(ctx context.Context, gateID string) (domaincontinuation.Receipt, error) {
	if store == nil || !validGateID(gateID) {
		return domaincontinuation.Receipt{}, errors.New("continuation gate identity is invalid")
	}
	if err := contextError(ctx); err != nil {
		return domaincontinuation.Receipt{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	receipt, err := store.readReceiptLocked(ctx, gateID)
	if errors.Is(err, os.ErrNotExist) {
		return domaincontinuation.Receipt{}, continuationstoreport.ErrNotFound
	}
	return receipt, err
}

func (store *Store) PutDispositionIfAbsent(ctx context.Context, disposition domaincontinuation.Disposition) error {
	if store == nil || domaincontinuation.ValidateDisposition(disposition) != nil {
		return errors.New("private continuation disposition is invalid")
	}
	body, err := domaincontinuation.DispositionBytes(disposition)
	if err != nil || len(body) == 0 || len(body) > maxRecordBytes {
		return errors.New("private continuation disposition exceeds storage limit")
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	receipt, err := store.readReceiptLocked(ctx, disposition.GateID)
	if err != nil || !dispositionMatchesReceipt(disposition, receipt) {
		return errors.New("private continuation disposition has no exact receipt authority")
	}
	if existing, err := store.readDispositionLocked(ctx, disposition.GateID); err == nil {
		existingBody, _ := domaincontinuation.DispositionBytes(existing)
		if bytes.Equal(existingBody, body) {
			return nil
		}
		return errors.New("private continuation was already consumed by another disposition")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	digest := gateIDDigest(disposition.GateID)
	if err := store.dispositionCAS.PutIfAbsent(ctx, digest, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		existing, readErr := store.readDispositionLocked(ctx, disposition.GateID)
		if readErr != nil {
			return readErr
		}
		existingBody, bodyErr := domaincontinuation.DispositionBytes(existing)
		if bodyErr == nil && bytes.Equal(existingBody, body) {
			return nil
		}
		return errors.New("private continuation was concurrently consumed by another disposition")
	}
	written, err := store.readDispositionLocked(ctx, disposition.GateID)
	if err != nil || written.DispositionID != disposition.DispositionID {
		return errors.New("private continuation disposition write verification failed")
	}
	return nil
}

func (store *Store) ResolveDisposition(ctx context.Context, gateID string) (domaincontinuation.Disposition, error) {
	if store == nil || !validGateID(gateID) {
		return domaincontinuation.Disposition{}, errors.New("continuation gate identity is invalid")
	}
	if err := contextError(ctx); err != nil {
		return domaincontinuation.Disposition{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	disposition, err := store.readDispositionLocked(ctx, gateID)
	if errors.Is(err, os.ErrNotExist) {
		return domaincontinuation.Disposition{}, continuationstoreport.ErrNotFound
	}
	if err == nil {
		receipt, receiptErr := store.readReceiptLocked(ctx, gateID)
		if receiptErr != nil || !dispositionMatchesReceipt(disposition, receipt) {
			return domaincontinuation.Disposition{}, errors.New("private continuation disposition lost its exact receipt authority")
		}
	}
	return disposition, err
}

func (store *Store) HasRecords(ctx context.Context) (bool, error) {
	if store == nil {
		return false, errors.New("private continuation store is unavailable")
	}
	if err := contextError(ctx); err != nil {
		return false, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	receipts, dispositions, err := store.loadInventoryLocked(ctx)
	return len(receipts)+len(dispositions) > 0, err
}

func (store *Store) VisitReceipts(ctx context.Context, visit func(domaincontinuation.Receipt) error) error {
	if store == nil || visit == nil {
		return errors.New("private continuation receipt visitor is required")
	}
	receipts, err := store.snapshotReceipts(ctx)
	if err != nil {
		return err
	}
	for _, receipt := range receipts {
		if err := contextError(ctx); err != nil {
			return err
		}
		if err := visit(receipt); err != nil {
			return err
		}
	}
	return nil
}

func (store *Store) VisitDispositions(ctx context.Context, visit func(domaincontinuation.Disposition) error) error {
	if store == nil || visit == nil {
		return errors.New("private continuation disposition visitor is required")
	}
	dispositions, err := store.snapshotDispositions(ctx)
	if err != nil {
		return err
	}
	for _, disposition := range dispositions {
		if err := contextError(ctx); err != nil {
			return err
		}
		if err := visit(disposition); err != nil {
			return err
		}
	}
	return nil
}

func (store *Store) snapshotReceipts(ctx context.Context) ([]domaincontinuation.Receipt, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	receipts, _, err := store.loadInventoryLocked(ctx)
	return receipts, err
}

func (store *Store) snapshotDispositions(ctx context.Context) ([]domaincontinuation.Disposition, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	_, dispositions, err := store.loadInventoryLocked(ctx)
	return dispositions, err
}

func (store *Store) loadInventoryLocked(ctx context.Context) ([]domaincontinuation.Receipt, []domaincontinuation.Disposition, error) {
	files, err := store.receiptCAS.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	receipts := make([]domaincontinuation.Receipt, 0, len(files))
	receiptByGate := make(map[string]domaincontinuation.Receipt, len(files))
	for _, file := range files {
		receipt, err := domaincontinuation.ParseReceipt(file.Body)
		canonical, canonicalErr := domaincontinuation.ReceiptBytes(receipt)
		if err != nil || canonicalErr != nil || gateIDDigest(receipt.Payload.GateID) != file.Digest || !bytes.Equal(canonical, file.Body) {
			return nil, nil, errors.New("private continuation receipt content address is corrupt")
		}
		if _, duplicate := receiptByGate[receipt.Payload.GateID]; duplicate {
			return nil, nil, errors.New("private continuation receipt inventory repeats a gate")
		}
		receiptByGate[receipt.Payload.GateID] = receipt
		receipts = append(receipts, receipt)
	}
	dispositionFiles, err := store.dispositionCAS.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	dispositions := make([]domaincontinuation.Disposition, 0, len(dispositionFiles))
	for _, file := range dispositionFiles {
		disposition, err := domaincontinuation.ParseDisposition(file.Body)
		canonical, canonicalErr := domaincontinuation.DispositionBytes(disposition)
		receipt, found := receiptByGate[disposition.GateID]
		if err != nil || canonicalErr != nil || gateIDDigest(disposition.GateID) != file.Digest || !bytes.Equal(canonical, file.Body) ||
			!found || !dispositionMatchesReceipt(disposition, receipt) {
			return nil, nil, errors.New("private continuation disposition lost its exact receipt authority")
		}
		dispositions = append(dispositions, disposition)
	}
	sort.Slice(receipts, func(i, j int) bool { return receipts[i].Payload.GateID < receipts[j].Payload.GateID })
	sort.Slice(dispositions, func(i, j int) bool { return dispositions[i].GateID < dispositions[j].GateID })
	return receipts, dispositions, nil
}

func (store *Store) readReceiptLocked(ctx context.Context, gateID string) (domaincontinuation.Receipt, error) {
	body, err := store.receiptCAS.Read(ctx, gateIDDigest(gateID))
	if err != nil {
		return domaincontinuation.Receipt{}, err
	}
	receipt, err := domaincontinuation.ParseReceipt(body)
	if err != nil || receipt.Payload.GateID != gateID {
		return domaincontinuation.Receipt{}, fmt.Errorf("read private continuation receipt: %w", errors.New("content address is corrupt"))
	}
	return receipt, nil
}

func (store *Store) readDispositionLocked(ctx context.Context, gateID string) (domaincontinuation.Disposition, error) {
	body, err := store.dispositionCAS.Read(ctx, gateIDDigest(gateID))
	if err != nil {
		return domaincontinuation.Disposition{}, err
	}
	disposition, err := domaincontinuation.ParseDisposition(body)
	if err != nil || disposition.GateID != gateID {
		return domaincontinuation.Disposition{}, fmt.Errorf("read private continuation disposition: %w", errors.New("content address is corrupt"))
	}
	return disposition, nil
}

func dispositionMatchesReceipt(disposition domaincontinuation.Disposition, receipt domaincontinuation.Receipt) bool {
	return disposition.GateID == receipt.Payload.GateID && disposition.ReceiptID == receipt.ReceiptID && disposition.Kind == receipt.Payload.Kind
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func validGateID(value string) bool {
	if len(value) != len("appr_")+64 && len(value) != len("input_")+64 {
		return false
	}
	digest := gateIDDigest(value)
	return len(digest) == 64 && digest == strings.ToLower(digest) && isLowerHex(digest)
}

func gateIDDigest(gateID string) string {
	return strings.TrimPrefix(strings.TrimPrefix(gateID, "appr_"), "input_")
}

func isLowerHex(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func validateContinuationOwnerRoot(root string, allowLegacy bool) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	allowed := map[string]bool{receiptPartitionV2: true, dispositionPartitionV2: true}
	if allowLegacy {
		allowed[legacyReceiptPartition] = true
		allowed[legacyDispositionPartition] = true
	}
	for _, entry := range entries {
		if !entry.IsDir() || !allowed[entry.Name()] {
			return errors.New("private continuation owner root contains an unknown entry")
		}
	}
	return nil
}
