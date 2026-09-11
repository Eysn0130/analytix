package casethreadauthority

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const maxCaseThreadAuthorityBytes = 128 * 1024

// Store keeps signed case/thread authority records in the shared host-private
// CAS primitive. Filesystem identity, no-follow traversal, single-link files,
// no-replace commit, directory durability, and readback are owned by that
// primitive; this adapter owns the record's semantic validation.
type Store struct {
	cas *finalauthorityadapter.SecurePrivateCAS
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
		return nil, errors.New("case thread authority access authority is required")
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("case thread authority root is required")
	}
	cas, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthorityContext(ctx, root, maxCaseThreadAuthorityBytes, access)
	if err != nil {
		return nil, err
	}
	return &Store{cas: cas}, nil
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

func (store *Store) PutIfAbsent(ctx context.Context, record domainsecurity.CaseThreadAuthorityRecord) error {
	if store == nil || store.cas == nil || domainsecurity.ValidateCaseThreadAuthorityRecord(record) != nil {
		return errors.New("case thread authority record is invalid")
	}
	body, err := domainsecurity.CaseThreadAuthorityRecordBytes(record)
	if err != nil || len(body) == 0 || len(body) > maxCaseThreadAuthorityBytes {
		return errors.New("case thread authority record exceeds storage limit")
	}
	if current, readErr := store.cas.Read(ctx, record.RecordDigest); readErr == nil {
		if bytes.Equal(current, body) {
			return nil
		}
		return errors.New("case thread authority conflicts with existing record")
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	if err := store.cas.PutIfAbsent(ctx, record.RecordDigest, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		current, readErr := store.cas.Read(ctx, record.RecordDigest)
		if readErr == nil && bytes.Equal(current, body) {
			return nil
		}
		return errors.New("case thread authority conflicts with concurrently created record")
	}
	written, err := store.cas.Read(ctx, record.RecordDigest)
	if err != nil || !bytes.Equal(written, body) {
		return errors.New("case thread authority write verification failed")
	}
	return nil
}

func (store *Store) List(ctx context.Context) ([]domainsecurity.CaseThreadAuthorityRecord, error) {
	if store == nil || store.cas == nil {
		return nil, errors.New("case thread authority store is unavailable")
	}
	files, err := store.cas.List(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]domainsecurity.CaseThreadAuthorityRecord, 0, len(files))
	for _, file := range files {
		record, parseErr := domainsecurity.ParseCaseThreadAuthorityRecord(file.Body)
		if parseErr != nil {
			return nil, fmt.Errorf("read case thread authority: %w", parseErr)
		}
		if file.Digest != record.RecordDigest {
			return nil, errors.New("case thread authority filename does not match record digest")
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].RecordDigest < records[j].RecordDigest })
	return records, nil
}

func (store *Store) HasRecords(ctx context.Context) (bool, error) {
	records, err := store.List(ctx)
	return len(records) > 0, err
}
