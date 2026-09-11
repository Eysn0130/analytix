package threadriskauthority

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	storeport "analytix.local/runtime-go/internal/ports/threadriskauthority"
)

const maxThreadRiskAuthorityIndexBytes = 4 << 20

var _ storeport.IndexStore = (*IndexStore)(nil)

// IndexStore keeps canonical signed thread-risk inventories in an immutable
// private CAS. It deliberately exposes no inventory or current-head method:
// callers may resolve only the exact digest selected by a fresh witness.
type IndexStore struct {
	cas *finalauthorityadapter.SecurePrivateCAS
}

func NewIndexStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*IndexStore, error) {
	if root == "" || root != strings.TrimSpace(root) || access == nil {
		return nil, errors.New("thread risk authority index root is invalid")
	}
	cas, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(root, maxThreadRiskAuthorityIndexBytes, access)
	if err != nil {
		return nil, err
	}
	return &IndexStore{cas: cas}, nil
}

func (store *IndexStore) PutIfAbsent(ctx context.Context, index domainsecurity.ThreadRiskAuthorityIndexV1) error {
	if store == nil || store.cas == nil {
		return errors.New("thread risk authority index store is unavailable")
	}
	body, err := domainsecurity.ThreadRiskAuthorityIndexV1Bytes(index)
	if err != nil || len(body) == 0 || len(body) > maxThreadRiskAuthorityIndexBytes {
		return errors.New("thread risk authority index is invalid")
	}
	if current, readErr := store.cas.Read(ctx, index.IndexDigest); readErr == nil {
		if !bytes.Equal(current, body) {
			return errors.New("thread risk authority index conflicts with existing content address")
		}
		stored, parseErr := parseStoredIndex(index.IndexDigest, current)
		if parseErr != nil || !equalIndex(stored, index) {
			return errors.New("thread risk authority existing index verification failed")
		}
		return nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	if err := store.cas.PutIfAbsent(ctx, index.IndexDigest, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		current, readErr := store.cas.Read(ctx, index.IndexDigest)
		if readErr != nil || !bytes.Equal(current, body) {
			return errors.New("thread risk authority index conflicts with concurrently created record")
		}
	}
	written, err := store.cas.Read(ctx, index.IndexDigest)
	if err != nil || !bytes.Equal(written, body) {
		return errors.New("thread risk authority index write readback failed")
	}
	stored, err := parseStoredIndex(index.IndexDigest, written)
	if err != nil || !equalIndex(stored, index) {
		return errors.New("thread risk authority index write semantic verification failed")
	}
	return nil
}

func (store *IndexStore) Resolve(ctx context.Context, digest string) (domainsecurity.ThreadRiskAuthorityIndexV1, error) {
	if store == nil || store.cas == nil {
		return domainsecurity.ThreadRiskAuthorityIndexV1{}, errors.New("thread risk authority index store is unavailable")
	}
	if digest == "" || digest != strings.TrimSpace(digest) || !domainsecurity.IsSHA256Hex(digest) {
		return domainsecurity.ThreadRiskAuthorityIndexV1{}, errors.New("thread risk authority index content address is invalid")
	}
	body, err := store.cas.Read(ctx, digest)
	if err != nil {
		return domainsecurity.ThreadRiskAuthorityIndexV1{}, err
	}
	return parseStoredIndex(digest, body)
}

func parseStoredIndex(digest string, body []byte) (domainsecurity.ThreadRiskAuthorityIndexV1, error) {
	if digest == "" || digest != strings.TrimSpace(digest) || !domainsecurity.IsSHA256Hex(digest) ||
		len(body) == 0 || len(body) > maxThreadRiskAuthorityIndexBytes {
		return domainsecurity.ThreadRiskAuthorityIndexV1{}, errors.New("thread risk authority index CAS record is invalid")
	}
	index, err := domainsecurity.ParseThreadRiskAuthorityIndexV1(body)
	if err != nil {
		return domainsecurity.ThreadRiskAuthorityIndexV1{}, err
	}
	if index.IndexDigest != digest {
		return domainsecurity.ThreadRiskAuthorityIndexV1{}, errors.New("thread risk authority index filename does not match index digest")
	}
	canonical, err := domainsecurity.ThreadRiskAuthorityIndexV1Bytes(index)
	if err != nil || !bytes.Equal(canonical, body) {
		return domainsecurity.ThreadRiskAuthorityIndexV1{}, errors.New("thread risk authority index bytes are not canonical")
	}
	return index, nil
}

func equalIndex(left, right domainsecurity.ThreadRiskAuthorityIndexV1) bool {
	leftBody, leftErr := domainsecurity.ThreadRiskAuthorityIndexV1Bytes(left)
	rightBody, rightErr := domainsecurity.ThreadRiskAuthorityIndexV1Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}
