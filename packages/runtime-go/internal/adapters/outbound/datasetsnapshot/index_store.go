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

const maxDatasetSnapshotIndexBytes = 256 << 10

type IndexStore struct {
	cas *finalauthorityadapter.SecurePrivateCAS
}

var _ datasetsnapshotport.IndexStore = (*IndexStore)(nil)

func NewIndexStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*IndexStore, error) {
	if root == "" || root != strings.TrimSpace(root) || access == nil {
		return nil, errors.New("dataset snapshot index root is invalid")
	}
	cas, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(root, maxDatasetSnapshotIndexBytes, access)
	if err != nil {
		return nil, err
	}
	return &IndexStore{cas: cas}, nil
}

func (store *IndexStore) PutIfAbsent(ctx context.Context, index domainsecurity.DatasetSnapshotIndexV1) error {
	if store == nil || store.cas == nil || domainsecurity.ValidateDatasetSnapshotIndexV1(index) != nil {
		return errors.New("dataset snapshot index is invalid")
	}
	body, err := domainsecurity.DatasetSnapshotIndexV1Bytes(index)
	if err != nil || len(body) == 0 || len(body) > maxDatasetSnapshotIndexBytes {
		return errors.New("dataset snapshot index bytes are invalid")
	}
	if current, readErr := store.cas.Read(ctx, index.IndexDigest); readErr == nil {
		if !bytes.Equal(current, body) {
			return errors.New("dataset snapshot index conflicts with its content address")
		}
		_, err := parseIndex(index.IndexDigest, current)
		return err
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	if err := store.cas.PutIfAbsent(ctx, index.IndexDigest, body); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	written, err := store.cas.Read(ctx, index.IndexDigest)
	if err != nil || !bytes.Equal(written, body) {
		return errors.New("dataset snapshot index write readback failed")
	}
	_, err = parseIndex(index.IndexDigest, written)
	return err
}

func (store *IndexStore) Resolve(ctx context.Context, digest string) (domainsecurity.DatasetSnapshotIndexV1, error) {
	if store == nil || store.cas == nil || digest == "" || digest != strings.TrimSpace(digest) || !domainsecurity.IsSHA256Hex(digest) {
		return domainsecurity.DatasetSnapshotIndexV1{}, errors.New("dataset snapshot index content address is invalid")
	}
	body, err := store.cas.Read(ctx, digest)
	if err != nil {
		return domainsecurity.DatasetSnapshotIndexV1{}, err
	}
	return parseIndex(digest, body)
}

func parseIndex(digest string, body []byte) (domainsecurity.DatasetSnapshotIndexV1, error) {
	if len(body) == 0 || len(body) > maxDatasetSnapshotIndexBytes {
		return domainsecurity.DatasetSnapshotIndexV1{}, errors.New("dataset snapshot index CAS body is invalid")
	}
	index, err := domainsecurity.ParseDatasetSnapshotIndexV1(body)
	if err != nil || index.IndexDigest != digest {
		return domainsecurity.DatasetSnapshotIndexV1{}, errors.New("dataset snapshot index filename or content is invalid")
	}
	canonical, err := domainsecurity.DatasetSnapshotIndexV1Bytes(index)
	if err != nil || !bytes.Equal(canonical, body) {
		return domainsecurity.DatasetSnapshotIndexV1{}, errors.New("dataset snapshot index bytes are not canonical")
	}
	return index, nil
}
