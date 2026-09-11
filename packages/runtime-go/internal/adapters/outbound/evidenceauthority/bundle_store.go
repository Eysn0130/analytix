package evidenceauthority

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	storeport "analytix.local/runtime-go/internal/ports/evidenceauthority"
)

const maxEvidenceAuthorityBundleBytes = 512 << 10

var _ storeport.BundleStore = (*BundleStore)(nil)

// BundleStore is an immutable content-addressed store. It deliberately has no
// inventory or current-head operation; only an independently witnessed digest
// may select a bundle for resolution.
type BundleStore struct {
	cas *finalauthorityadapter.SecurePrivateCAS
}

func (store *BundleStore) Close() error {
	if store == nil || store.cas == nil {
		return nil
	}
	return store.cas.Close()
}

func NewBundleStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*BundleStore, error) {
	if root == "" || root != strings.TrimSpace(root) || access == nil {
		return nil, errors.New("evidence authority bundle root is invalid")
	}
	cas, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(root, maxEvidenceAuthorityBundleBytes, access)
	if err != nil {
		return nil, err
	}
	return &BundleStore{cas: cas}, nil
}

func (store *BundleStore) HasRecords(ctx context.Context) (bool, error) {
	if store == nil || store.cas == nil || ctx == nil {
		return false, errors.New("evidence authority bundle store is unavailable")
	}
	found := errors.New("evidence authority bundle record found")
	err := store.cas.Visit(ctx, func(finalauthorityadapter.SecurePrivateCASFile) error { return found })
	if errors.Is(err, found) {
		return true, nil
	}
	return false, err
}

func (store *BundleStore) PutIfAbsent(ctx context.Context, bundle domainevidence.EvidenceAuthorityBundleV1) error {
	if store == nil || store.cas == nil {
		return errors.New("evidence authority bundle store is unavailable")
	}
	body, err := domainevidence.EvidenceAuthorityBundleV1Bytes(bundle)
	if err != nil || len(body) == 0 || len(body) > maxEvidenceAuthorityBundleBytes {
		return errors.New("evidence authority bundle is invalid")
	}
	if current, readErr := store.cas.Read(ctx, bundle.RecordDigest); readErr == nil {
		if !bytes.Equal(current, body) {
			return errors.New("evidence authority bundle conflicts with its content address")
		}
		stored, parseErr := parseStoredBundle(bundle.RecordDigest, current)
		if parseErr != nil || !equalBundle(stored, bundle) {
			return errors.New("existing evidence authority bundle verification failed")
		}
		return nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	if err := store.cas.PutIfAbsent(ctx, bundle.RecordDigest, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		current, readErr := store.cas.Read(ctx, bundle.RecordDigest)
		if readErr != nil || !bytes.Equal(current, body) {
			return errors.New("evidence authority bundle conflicts with concurrently created content")
		}
	}
	written, err := store.cas.Read(ctx, bundle.RecordDigest)
	if err != nil || !bytes.Equal(written, body) {
		return errors.New("evidence authority bundle write readback failed")
	}
	stored, err := parseStoredBundle(bundle.RecordDigest, written)
	if err != nil || !equalBundle(stored, bundle) {
		return errors.New("evidence authority bundle write semantic verification failed")
	}
	return nil
}

func (store *BundleStore) Resolve(ctx context.Context, digest string) (domainevidence.EvidenceAuthorityBundleV1, error) {
	if store == nil || store.cas == nil {
		return domainevidence.EvidenceAuthorityBundleV1{}, errors.New("evidence authority bundle store is unavailable")
	}
	if digest == "" || digest != strings.TrimSpace(digest) || !domainsecurity.IsSHA256Hex(digest) {
		return domainevidence.EvidenceAuthorityBundleV1{}, errors.New("evidence authority bundle content address is invalid")
	}
	body, err := store.cas.Read(ctx, digest)
	if err != nil {
		return domainevidence.EvidenceAuthorityBundleV1{}, err
	}
	return parseStoredBundle(digest, body)
}

func parseStoredBundle(digest string, body []byte) (domainevidence.EvidenceAuthorityBundleV1, error) {
	if digest == "" || digest != strings.TrimSpace(digest) || !domainsecurity.IsSHA256Hex(digest) ||
		len(body) == 0 || len(body) > maxEvidenceAuthorityBundleBytes {
		return domainevidence.EvidenceAuthorityBundleV1{}, errors.New("evidence authority bundle CAS record is invalid")
	}
	bundle, err := domainevidence.ParseEvidenceAuthorityBundleV1(body)
	if err != nil || bundle.RecordDigest != digest {
		return domainevidence.EvidenceAuthorityBundleV1{}, errors.New("evidence authority bundle filename or content is invalid")
	}
	canonical, err := domainevidence.EvidenceAuthorityBundleV1Bytes(bundle)
	if err != nil || !bytes.Equal(canonical, body) {
		return domainevidence.EvidenceAuthorityBundleV1{}, errors.New("evidence authority bundle bytes are not canonical")
	}
	return bundle, nil
}

func equalBundle(left, right domainevidence.EvidenceAuthorityBundleV1) bool {
	leftBody, leftErr := domainevidence.EvidenceAuthorityBundleV1Bytes(left)
	rightBody, rightErr := domainevidence.EvidenceAuthorityBundleV1Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}
