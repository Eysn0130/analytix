package evidenceregistry

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

const (
	maxEvidenceRegistryAuthorityIndexV2Bytes   = 256 << 10
	maxEvidenceRegistryAuthorityCapsuleV2Bytes = 16 << 20
)

var _ registryport.AuthorityIndexStore = (*AuthorityIndexStoreV2)(nil)
var _ registryport.AuthorityCapsuleStore = (*AuthorityCapsuleStoreV2)(nil)

type AuthorityIndexStoreV2 struct {
	cas *finalauthorityadapter.SecurePrivateCAS
}

type AuthorityCapsuleStoreV2 struct {
	cas *finalauthorityadapter.SecurePrivateCAS
}

func (store *AuthorityIndexStoreV2) Close() error {
	if store == nil || store.cas == nil {
		return nil
	}
	return store.cas.Close()
}

func (store *AuthorityCapsuleStoreV2) Close() error {
	if store == nil || store.cas == nil {
		return nil
	}
	return store.cas.Close()
}

func NewAuthorityIndexStoreV2(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*AuthorityIndexStoreV2, error) {
	if strings.TrimSpace(root) == "" || root != strings.TrimSpace(root) || access == nil {
		return nil, errors.New("evidence registry authority index V2 root is invalid")
	}
	cas, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(root, maxEvidenceRegistryAuthorityIndexV2Bytes, access)
	if err != nil {
		return nil, err
	}
	return &AuthorityIndexStoreV2{cas: cas}, nil
}

func NewAuthorityCapsuleStoreV2(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*AuthorityCapsuleStoreV2, error) {
	if strings.TrimSpace(root) == "" || root != strings.TrimSpace(root) || access == nil {
		return nil, errors.New("evidence registry authority capsule V2 root is invalid")
	}
	cas, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(root, maxEvidenceRegistryAuthorityCapsuleV2Bytes, access)
	if err != nil {
		return nil, err
	}
	return &AuthorityCapsuleStoreV2{cas: cas}, nil
}

func (store *AuthorityIndexStoreV2) PutIfAbsent(ctx context.Context, index domainevidence.EvidenceRegistryAuthorityIndexV2) error {
	if store == nil || store.cas == nil {
		return errors.New("evidence registry authority index V2 store is unavailable")
	}
	body, err := domainevidence.EvidenceRegistryAuthorityIndexV2Bytes(index)
	if err != nil || len(body) == 0 || len(body) > maxEvidenceRegistryAuthorityIndexV2Bytes {
		return errors.New("evidence registry authority index V2 is invalid")
	}
	return putCanonicalAuthorityRecord(ctx, store.cas, index.IndexDigest, body, func(stored []byte) (string, error) {
		parsed, parseErr := domainevidence.ParseEvidenceRegistryAuthorityIndexV2(stored)
		return parsed.IndexDigest, parseErr
	})
}

func (store *AuthorityIndexStoreV2) Resolve(ctx context.Context, digest string) (domainevidence.EvidenceRegistryAuthorityIndexV2, error) {
	if store == nil || store.cas == nil || strings.TrimSpace(digest) != digest || !domainsecurity.IsSHA256Hex(digest) {
		return domainevidence.EvidenceRegistryAuthorityIndexV2{}, errors.New("evidence registry authority index V2 address is invalid")
	}
	body, err := store.cas.Read(ctx, digest)
	if err != nil {
		return domainevidence.EvidenceRegistryAuthorityIndexV2{}, err
	}
	index, err := domainevidence.ParseEvidenceRegistryAuthorityIndexV2(body)
	if err != nil || index.IndexDigest != digest {
		return domainevidence.EvidenceRegistryAuthorityIndexV2{}, errors.New("evidence registry authority index V2 content address mismatch")
	}
	return index, nil
}

func (store *AuthorityCapsuleStoreV2) PutIfAbsent(ctx context.Context, capsule domainevidence.EvidenceRegistryAuthorityCapsule) error {
	if store == nil || store.cas == nil {
		return errors.New("evidence registry authority capsule V2 store is unavailable")
	}
	body, err := domainevidence.CanonicalEvidenceRegistryAuthorityCapsuleBytes(capsule)
	if err != nil || len(body) == 0 || len(body) > maxEvidenceRegistryAuthorityCapsuleV2Bytes {
		return errors.New("evidence registry authority capsule V2 is invalid")
	}
	return putCanonicalAuthorityRecord(ctx, store.cas, capsule.RecordDigest, body, func(stored []byte) (string, error) {
		parsed, parseErr := domainevidence.ParseEvidenceRegistryAuthorityCapsule(stored)
		return parsed.RecordDigest, parseErr
	})
}

func (store *AuthorityCapsuleStoreV2) Resolve(ctx context.Context, digest string) (domainevidence.EvidenceRegistryAuthorityCapsule, error) {
	if store == nil || store.cas == nil || strings.TrimSpace(digest) != digest || !domainsecurity.IsSHA256Hex(digest) {
		return domainevidence.EvidenceRegistryAuthorityCapsule{}, errors.New("evidence registry authority capsule V2 address is invalid")
	}
	body, err := store.cas.Read(ctx, digest)
	if err != nil {
		return domainevidence.EvidenceRegistryAuthorityCapsule{}, err
	}
	capsule, err := domainevidence.ParseEvidenceRegistryAuthorityCapsule(body)
	if err != nil || capsule.RecordDigest != digest {
		return domainevidence.EvidenceRegistryAuthorityCapsule{}, errors.New("evidence registry authority capsule V2 content address mismatch")
	}
	return capsule, nil
}

func putCanonicalAuthorityRecord(ctx context.Context, cas *finalauthorityadapter.SecurePrivateCAS, digest string, body []byte, parseDigest func([]byte) (string, error)) error {
	if cas == nil || !domainsecurity.IsSHA256Hex(digest) || len(body) == 0 || parseDigest == nil {
		return errors.New("private authority CAS record is invalid")
	}
	if current, err := cas.Read(ctx, digest); err == nil {
		parsedDigest, parseErr := parseDigest(current)
		if parseErr != nil || parsedDigest != digest || !bytes.Equal(current, body) {
			return errors.New("private authority CAS record conflicts with its content address")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := cas.PutIfAbsent(ctx, digest, body); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	written, err := cas.Read(ctx, digest)
	if err != nil || !bytes.Equal(written, body) {
		return errors.New("private authority CAS write readback failed")
	}
	parsedDigest, err := parseDigest(written)
	if err != nil || parsedDigest != digest {
		return errors.New("private authority CAS write semantic verification failed")
	}
	return nil
}
