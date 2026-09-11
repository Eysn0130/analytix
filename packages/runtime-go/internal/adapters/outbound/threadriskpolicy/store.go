package threadriskpolicy

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

const maxThreadRiskPolicyBytes = 64 * 1024

// Store keeps canonical signed thread risk policies in the host-private CAS.
// The shared CAS owns handle-relative filesystem safety and atomic no-replace
// commits; this adapter owns exact policy parsing, content addressing, and
// canonical-byte verification.
type Store struct {
	cas *finalauthorityadapter.SecurePrivateCAS
}

func NewStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil {
		return nil, errors.New("thread risk policy root is required")
	}
	cas, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(root, maxThreadRiskPolicyBytes, access)
	if err != nil {
		return nil, err
	}
	return &Store{cas: cas}, nil
}

func (store *Store) PutIfAbsent(ctx context.Context, policy domainsecurity.ThreadRiskPolicyV1) error {
	if store == nil || store.cas == nil {
		return errors.New("thread risk policy store is unavailable")
	}
	body, err := domainsecurity.ThreadRiskPolicyV1Bytes(policy)
	if err != nil || len(body) == 0 || len(body) > maxThreadRiskPolicyBytes {
		return errors.New("thread risk policy record is invalid")
	}
	if current, readErr := store.cas.Read(ctx, policy.PolicyDigest); readErr == nil {
		if !bytes.Equal(current, body) {
			return errors.New("thread risk policy conflicts with existing content address")
		}
		stored, parseErr := parseStoredPolicy(policy.PolicyDigest, current)
		if parseErr != nil || stored != policy {
			return errors.New("thread risk policy existing record verification failed")
		}
		return nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	if err := store.cas.PutIfAbsent(ctx, policy.PolicyDigest, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		current, readErr := store.cas.Read(ctx, policy.PolicyDigest)
		if readErr != nil || !bytes.Equal(current, body) {
			return errors.New("thread risk policy conflicts with concurrently created record")
		}
	}
	written, err := store.cas.Read(ctx, policy.PolicyDigest)
	if err != nil || !bytes.Equal(written, body) {
		return errors.New("thread risk policy write readback failed")
	}
	stored, err := parseStoredPolicy(policy.PolicyDigest, written)
	if err != nil || stored != policy {
		return errors.New("thread risk policy write semantic verification failed")
	}
	return nil
}

func (store *Store) Resolve(ctx context.Context, digest string) (domainsecurity.ThreadRiskPolicyV1, error) {
	if store == nil || store.cas == nil {
		return domainsecurity.ThreadRiskPolicyV1{}, errors.New("thread risk policy store is unavailable")
	}
	if digest != strings.TrimSpace(digest) || !domainsecurity.IsSHA256Hex(digest) {
		return domainsecurity.ThreadRiskPolicyV1{}, errors.New("thread risk policy content address is invalid")
	}
	body, err := store.cas.Read(ctx, digest)
	if err != nil {
		return domainsecurity.ThreadRiskPolicyV1{}, err
	}
	return parseStoredPolicy(digest, body)
}

func (store *Store) List(ctx context.Context) ([]domainsecurity.ThreadRiskPolicyV1, error) {
	if store == nil || store.cas == nil {
		return nil, errors.New("thread risk policy store is unavailable")
	}
	files, err := store.cas.List(ctx)
	if err != nil {
		return nil, err
	}
	policies := make([]domainsecurity.ThreadRiskPolicyV1, 0, len(files))
	for _, file := range files {
		policy, parseErr := parseStoredPolicy(file.Digest, file.Body)
		if parseErr != nil {
			return nil, fmt.Errorf("read thread risk policy: %w", parseErr)
		}
		policies = append(policies, policy)
	}
	sort.Slice(policies, func(left, right int) bool {
		return policies[left].PolicyDigest < policies[right].PolicyDigest
	})
	return policies, nil
}

func (store *Store) HasRecords(ctx context.Context) (bool, error) {
	policies, err := store.List(ctx)
	return len(policies) > 0, err
}

func parseStoredPolicy(digest string, body []byte) (domainsecurity.ThreadRiskPolicyV1, error) {
	if digest != strings.TrimSpace(digest) || !domainsecurity.IsSHA256Hex(digest) || len(body) == 0 || len(body) > maxThreadRiskPolicyBytes {
		return domainsecurity.ThreadRiskPolicyV1{}, errors.New("thread risk policy CAS record is invalid")
	}
	policy, err := domainsecurity.ParseThreadRiskPolicyV1(body)
	if err != nil {
		return domainsecurity.ThreadRiskPolicyV1{}, err
	}
	if policy.PolicyDigest != digest {
		return domainsecurity.ThreadRiskPolicyV1{}, errors.New("thread risk policy filename does not match policy digest")
	}
	canonical, err := domainsecurity.ThreadRiskPolicyV1Bytes(policy)
	if err != nil || !bytes.Equal(canonical, body) {
		return domainsecurity.ThreadRiskPolicyV1{}, errors.New("thread risk policy bytes are not canonical")
	}
	return policy, nil
}
