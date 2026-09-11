package datasetsnapshot

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
)

const maxDatasetSnapshotRecordBytes = 512 << 10

type RecordStore struct {
	cas *finalauthorityadapter.SecurePrivateCAS
}

var _ datasetsnapshotport.RecordStore = (*RecordStore)(nil)

func NewRecordStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*RecordStore, error) {
	if root == "" || root != strings.TrimSpace(root) || access == nil {
		return nil, errors.New("dataset snapshot record root is invalid")
	}
	cas, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(root, maxDatasetSnapshotRecordBytes, access)
	if err != nil {
		return nil, err
	}
	return &RecordStore{cas: cas}, nil
}

func (store *RecordStore) PutIfAbsent(ctx context.Context, record domainsecurity.DatasetSnapshotAuthorityRecordV1) error {
	if store == nil || store.cas == nil || domainsecurity.ValidateDatasetSnapshotAuthorityRecordV1(record) != nil {
		return errors.New("dataset snapshot authority record is invalid")
	}
	body, err := domainsecurity.DatasetSnapshotAuthorityRecordV1Bytes(record)
	if err != nil || len(body) == 0 || len(body) > maxDatasetSnapshotRecordBytes {
		return errors.New("dataset snapshot authority record bytes are invalid")
	}
	if current, readErr := store.cas.Read(ctx, record.RecordDigest); readErr == nil {
		if !bytes.Equal(current, body) {
			return errors.New("dataset snapshot record conflicts with its content address")
		}
		_, err := parseRecord(record.RecordDigest, current)
		return err
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	if err := store.cas.PutIfAbsent(ctx, record.RecordDigest, body); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	written, err := store.cas.Read(ctx, record.RecordDigest)
	if err != nil || !bytes.Equal(written, body) {
		return errors.New("dataset snapshot record write readback failed")
	}
	_, err = parseRecord(record.RecordDigest, written)
	return err
}

func (store *RecordStore) Resolve(ctx context.Context, digest string) (domainsecurity.DatasetSnapshotAuthorityRecordV1, error) {
	if store == nil || store.cas == nil || digest == "" || digest != strings.TrimSpace(digest) || !domainsecurity.IsSHA256Hex(digest) {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, errors.New("dataset snapshot record content address is invalid")
	}
	body, err := store.cas.Read(ctx, digest)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, errors.Join(datasetsnapshotport.ErrNotFound, err)
		}
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, err
	}
	return parseRecord(digest, body)
}

func parseRecord(digest string, body []byte) (domainsecurity.DatasetSnapshotAuthorityRecordV1, error) {
	if len(body) == 0 || len(body) > maxDatasetSnapshotRecordBytes {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, errors.New("dataset snapshot record CAS body is invalid")
	}
	record, err := domainsecurity.ParseDatasetSnapshotAuthorityRecordV1(body)
	if err != nil || record.RecordDigest != digest {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, errors.New("dataset snapshot record filename or content is invalid")
	}
	canonical, err := domainsecurity.DatasetSnapshotAuthorityRecordV1Bytes(record)
	if err != nil || !bytes.Equal(canonical, body) {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, errors.New("dataset snapshot record bytes are not canonical")
	}
	return record, nil
}
