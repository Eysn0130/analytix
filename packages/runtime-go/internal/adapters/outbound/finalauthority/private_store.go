package finalauthority

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

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const maxPrivateAcceptedFinalBytes = 16 * 1024 * 1024

type PrivateStore struct {
	mu             sync.Mutex
	records        string
	dispositions   string
	recordCAS      *SecurePrivateCAS
	dispositionCAS *SecurePrivateCAS
}

func NewPrivateStore(root string, access SecurePrivateCASAccessAuthority) (*PrivateStore, error) {
	return NewPrivateStoreContext(context.Background(), root, access)
}

func NewPrivateStoreContext(
	ctx context.Context,
	root string,
	access SecurePrivateCASAccessAuthority,
) (*PrivateStore, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil {
		return nil, errors.New("private accepted final root is required")
	}
	records := filepath.Join(root, "records")
	recordCAS, err := OpenSecurePrivateCASWithAccessAuthorityContext(ctx, records, maxPrivateAcceptedFinalBytes, access)
	if err != nil {
		return nil, err
	}
	dispositions := filepath.Join(root, "dispositions")
	dispositionCAS, err := OpenSecurePrivateCASWithAccessAuthorityContext(ctx, dispositions, maxPrivateAcceptedFinalBytes, access)
	if err != nil {
		return nil, err
	}
	return &PrivateStore{
		records: records, dispositions: dispositions, recordCAS: recordCAS, dispositionCAS: dispositionCAS,
	}, nil
}

func PreflightPrivateStoreRecovery(
	ctx context.Context,
	root string,
	access SecurePrivateCASRecoveryAccessAuthority,
) error {
	prepared, err := PreparePrivateStoreRecoveryV1(ctx, root, access)
	if err != nil {
		return err
	}
	return prepared.Revalidate(ctx)
}

func (store *PrivateStore) PutIfAbsent(ctx context.Context, record domainevidence.PrivateAcceptedFinalRecord) error {
	if store == nil || domainevidence.ValidatePrivateAcceptedFinalRecord(record) != nil {
		return errors.New("private accepted final record is invalid")
	}
	if err := contextErr(ctx); err != nil {
		return err
	}
	body, err := domainevidence.PrivateAcceptedFinalRecordBytes(record)
	if err != nil || len(body) > maxPrivateAcceptedFinalBytes {
		return errors.New("private accepted final record exceeds storage limit")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	digest := record.AcceptedFinal.RecordDigest
	if existing, err := store.readRecord(ctx, digest); err == nil {
		existingBody, _ := domainevidence.PrivateAcceptedFinalRecordBytes(existing)
		if bytes.Equal(existingBody, body) {
			return nil
		}
		return errors.New("private accepted final record conflicts with existing authority")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := store.recordCAS.PutIfAbsent(ctx, digest, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		existing, readErr := store.readRecord(ctx, digest)
		if readErr != nil {
			return readErr
		}
		existingBody, _ := domainevidence.PrivateAcceptedFinalRecordBytes(existing)
		if bytes.Equal(existingBody, body) {
			return nil
		}
		return errors.New("private accepted final record conflicts with concurrently created authority")
	}
	written, err := store.readRecord(ctx, digest)
	if err != nil || written.StoreDigest != record.StoreDigest {
		return errors.New("private accepted final write verification failed")
	}
	return nil
}

func (store *PrivateStore) Resolve(ctx context.Context, acceptedFinalDigest string) (domainevidence.PrivateAcceptedFinalRecord, error) {
	if store == nil || !domainsecurity.IsSHA256Hex(strings.TrimSpace(acceptedFinalDigest)) {
		return domainevidence.PrivateAcceptedFinalRecord{}, errors.New("accepted final digest is invalid")
	}
	if err := contextErr(ctx); err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.readRecord(ctx, strings.TrimSpace(acceptedFinalDigest))
}

func (store *PrivateStore) List(ctx context.Context) ([]domainevidence.PrivateAcceptedFinalRecord, error) {
	if store == nil {
		return nil, errors.New("private accepted final store is unavailable")
	}
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	files, err := store.recordCAS.List(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]domainevidence.PrivateAcceptedFinalRecord, 0, len(files))
	for _, file := range files {
		record, err := domainevidence.ParsePrivateAcceptedFinalRecord(file.Body)
		if err != nil || record.AcceptedFinal.RecordDigest != file.Digest {
			return nil, errors.New("private accepted final filename does not match its authority digest")
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].AcceptedFinal.RecordDigest < records[j].AcceptedFinal.RecordDigest
	})
	return records, nil
}

func (store *PrivateStore) VisitAcceptedFinals(ctx context.Context, visit func(domainevidence.PrivateAcceptedFinalRecord) error) error {
	if store == nil || visit == nil {
		return errors.New("private accepted final visitor is unavailable")
	}
	if err := contextErr(ctx); err != nil {
		return err
	}
	store.mu.Lock()
	records := make([]domainevidence.PrivateAcceptedFinalRecord, 0)
	err := store.visitAcceptedFinalsLocked(ctx, func(record domainevidence.PrivateAcceptedFinalRecord) error {
		body, bodyErr := domainevidence.PrivateAcceptedFinalRecordBytes(record)
		if bodyErr != nil {
			return bodyErr
		}
		clone, cloneErr := domainevidence.ParsePrivateAcceptedFinalRecord(body)
		if cloneErr != nil {
			return cloneErr
		}
		records = append(records, clone)
		return nil
	})
	store.mu.Unlock()
	if err != nil {
		return err
	}
	for _, record := range records {
		if err := contextErr(ctx); err != nil {
			return err
		}
		if err := visit(record); err != nil {
			return err
		}
	}
	return nil
}

func (store *PrivateStore) visitAcceptedFinalsLocked(ctx context.Context, visit func(domainevidence.PrivateAcceptedFinalRecord) error) error {
	return store.recordCAS.Visit(ctx, func(file SecurePrivateCASFile) error {
		record, err := domainevidence.ParsePrivateAcceptedFinalRecord(file.Body)
		if err != nil || record.AcceptedFinal.RecordDigest != file.Digest {
			return errors.New("private accepted final filename does not match its authority digest")
		}
		return visit(record)
	})
}

func (store *PrivateStore) HasRecords(ctx context.Context) (bool, error) {
	if store == nil {
		return false, errors.New("private accepted final store is unavailable")
	}
	if err := contextErr(ctx); err != nil {
		return false, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	hasRecords := false
	if err := store.visitAcceptedFinalsLocked(ctx, func(domainevidence.PrivateAcceptedFinalRecord) error {
		hasRecords = true
		return nil
	}); err != nil {
		return false, err
	}
	if err := store.visitDispositionsLocked(ctx, func(domainevidence.AcceptedFinalDispositionRecord) error {
		hasRecords = true
		return nil
	}); err != nil {
		return false, err
	}
	return hasRecords, nil
}

func (store *PrivateStore) PutDispositionIfAbsent(ctx context.Context, record domainevidence.AcceptedFinalDispositionRecord) error {
	if store == nil || domainevidence.ValidateAcceptedFinalDispositionRecord(record) != nil {
		return errors.New("accepted final disposition record is invalid")
	}
	if err := contextErr(ctx); err != nil {
		return err
	}
	body, err := domainevidence.AcceptedFinalDispositionRecordBytes(record)
	if err != nil || len(body) > maxPrivateAcceptedFinalBytes {
		return errors.New("accepted final disposition record exceeds storage limit")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	digest := record.AcceptedFinalDigest
	if existing, err := store.readDisposition(ctx, digest); err == nil {
		existingBody, _ := domainevidence.AcceptedFinalDispositionRecordBytes(existing)
		if bytes.Equal(existingBody, body) {
			return nil
		}
		return errors.New("accepted final disposition conflicts with existing authority")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := store.dispositionCAS.PutIfAbsent(ctx, digest, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		existing, readErr := store.readDisposition(ctx, digest)
		if readErr != nil {
			return readErr
		}
		existingBody, _ := domainevidence.AcceptedFinalDispositionRecordBytes(existing)
		if bytes.Equal(existingBody, body) {
			return nil
		}
		return errors.New("accepted final disposition conflicts with concurrently created authority")
	}
	written, err := store.readDisposition(ctx, digest)
	if err != nil || written.RecordDigest != record.RecordDigest {
		return errors.New("accepted final disposition write verification failed")
	}
	return nil
}

func (store *PrivateStore) ResolveDisposition(ctx context.Context, acceptedFinalDigest string) (domainevidence.AcceptedFinalDispositionRecord, error) {
	if store == nil || !domainsecurity.IsSHA256Hex(strings.TrimSpace(acceptedFinalDigest)) {
		return domainevidence.AcceptedFinalDispositionRecord{}, errors.New("accepted final digest is invalid")
	}
	if err := contextErr(ctx); err != nil {
		return domainevidence.AcceptedFinalDispositionRecord{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.readDisposition(ctx, strings.TrimSpace(acceptedFinalDigest))
}

func (store *PrivateStore) ListDispositions(ctx context.Context) ([]domainevidence.AcceptedFinalDispositionRecord, error) {
	if store == nil {
		return nil, errors.New("accepted final disposition store is unavailable")
	}
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	files, err := store.dispositionCAS.List(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]domainevidence.AcceptedFinalDispositionRecord, 0, len(files))
	for _, file := range files {
		record, err := domainevidence.ParseAcceptedFinalDispositionRecord(file.Body)
		if err != nil || record.AcceptedFinalDigest != file.Digest {
			return nil, errors.New("accepted final disposition filename does not match its authority digest")
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].AcceptedFinalDigest < records[j].AcceptedFinalDigest
	})
	return records, nil
}

func (store *PrivateStore) VisitDispositions(ctx context.Context, visit func(domainevidence.AcceptedFinalDispositionRecord) error) error {
	if store == nil || visit == nil {
		return errors.New("accepted final disposition visitor is unavailable")
	}
	if err := contextErr(ctx); err != nil {
		return err
	}
	store.mu.Lock()
	records := make([]domainevidence.AcceptedFinalDispositionRecord, 0)
	err := store.visitDispositionsLocked(ctx, func(record domainevidence.AcceptedFinalDispositionRecord) error {
		body, bodyErr := domainevidence.AcceptedFinalDispositionRecordBytes(record)
		if bodyErr != nil {
			return bodyErr
		}
		clone, cloneErr := domainevidence.ParseAcceptedFinalDispositionRecord(body)
		if cloneErr != nil {
			return cloneErr
		}
		records = append(records, clone)
		return nil
	})
	store.mu.Unlock()
	if err != nil {
		return err
	}
	for _, record := range records {
		if err := contextErr(ctx); err != nil {
			return err
		}
		if err := visit(record); err != nil {
			return err
		}
	}
	return nil
}

func (store *PrivateStore) visitDispositionsLocked(ctx context.Context, visit func(domainevidence.AcceptedFinalDispositionRecord) error) error {
	return store.dispositionCAS.Visit(ctx, func(file SecurePrivateCASFile) error {
		record, err := domainevidence.ParseAcceptedFinalDispositionRecord(file.Body)
		if err != nil || record.AcceptedFinalDigest != file.Digest {
			return errors.New("accepted final disposition filename does not match its authority digest")
		}
		return visit(record)
	})
}

func (store *PrivateStore) recordPath(digest string) string {
	digest = strings.TrimSpace(digest)
	return filepath.Join(store.records, digest[:2], digest+".json")
}

func (store *PrivateStore) dispositionPath(digest string) string {
	digest = strings.TrimSpace(digest)
	return filepath.Join(store.dispositions, digest[:2], digest+".json")
}

func (store *PrivateStore) readRecord(ctx context.Context, digest string) (domainevidence.PrivateAcceptedFinalRecord, error) {
	body, err := store.recordCAS.Read(ctx, digest)
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, err
	}
	record, err := domainevidence.ParsePrivateAcceptedFinalRecord(body)
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, fmt.Errorf("read private accepted final: %w", err)
	}
	return record, nil
}

func (store *PrivateStore) readDisposition(ctx context.Context, digest string) (domainevidence.AcceptedFinalDispositionRecord, error) {
	body, err := store.dispositionCAS.Read(ctx, digest)
	if err != nil {
		return domainevidence.AcceptedFinalDispositionRecord{}, err
	}
	record, err := domainevidence.ParseAcceptedFinalDispositionRecord(body)
	if err != nil {
		return domainevidence.AcceptedFinalDispositionRecord{}, fmt.Errorf("read accepted final disposition: %w", err)
	}
	return record, nil
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
