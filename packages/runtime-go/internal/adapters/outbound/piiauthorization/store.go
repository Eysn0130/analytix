package piiauthorization

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"sync"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
)

const maxPIIProjectionGrantBytesV1 = 2 << 20

type Store struct {
	grants *finalauthorityadapter.SecurePrivateCAS

	closeOnce sync.Once
	closeErr  error
}

var _ piiauthorizationport.Store = (*Store)(nil)
var _ piiauthorizationport.InventoryStore = (*Store)(nil)

func NewStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil {
		return nil, errors.Join(piiauthorizationport.ErrCorrupt, errors.New("PII authorization store root is invalid"))
	}
	grants, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(root, maxPIIProjectionGrantBytesV1, access)
	if err != nil {
		return nil, err
	}
	return &Store{grants: grants}, nil
}

func (store *Store) Close() error {
	if store == nil || store.grants == nil {
		return nil
	}
	store.closeOnce.Do(func() {
		store.closeErr = store.grants.Close()
	})
	return store.closeErr
}

func (store *Store) PutGrantIfAbsent(ctx context.Context, grant domainpii.PIIProjectionGrantV1) error {
	if store == nil || store.grants == nil || domainpii.ValidatePIIProjectionGrantV1(grant) != nil {
		return piiauthorizationport.ErrCorrupt
	}
	body, err := domainpii.PIIProjectionGrantV1Bytes(grant)
	if err != nil {
		return errors.Join(piiauthorizationport.ErrCorrupt, err)
	}
	if current, err := store.grants.Read(ctx, grant.RecordDigest); err == nil {
		parsed, parseErr := domainpii.ParsePIIProjectionGrantV1(current)
		if parseErr != nil || parsed.RecordDigest != grant.RecordDigest || !bytes.Equal(current, body) {
			return errors.Join(piiauthorizationport.ErrConflict, parseErr)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := store.grants.PutIfAbsent(ctx, grant.RecordDigest, body); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	written, err := store.grants.Read(ctx, grant.RecordDigest)
	if err != nil || !bytes.Equal(written, body) {
		return errors.Join(piiauthorizationport.ErrCorrupt, err)
	}
	parsed, err := domainpii.ParsePIIProjectionGrantV1(written)
	if err != nil || parsed.RecordDigest != grant.RecordDigest {
		return errors.Join(piiauthorizationport.ErrCorrupt, err)
	}
	return nil
}

func (store *Store) ResolveGrant(ctx context.Context, auditDigest string) (domainpii.PIIProjectionGrantV1, error) {
	auditDigest = strings.TrimSpace(auditDigest)
	if store == nil || store.grants == nil || !domainsecurity.IsSHA256Hex(auditDigest) {
		return domainpii.PIIProjectionGrantV1{}, piiauthorizationport.ErrNotFound
	}
	body, err := store.grants.Read(ctx, auditDigest)
	if errors.Is(err, os.ErrNotExist) {
		return domainpii.PIIProjectionGrantV1{}, piiauthorizationport.ErrNotFound
	}
	if err != nil {
		return domainpii.PIIProjectionGrantV1{}, err
	}
	grant, err := domainpii.ParsePIIProjectionGrantV1(body)
	if err != nil || grant.RecordDigest != auditDigest {
		return domainpii.PIIProjectionGrantV1{}, errors.Join(piiauthorizationport.ErrCorrupt, err)
	}
	return grant, nil
}

func (store *Store) HasRecords(ctx context.Context) (bool, error) {
	if store == nil || store.grants == nil {
		return false, piiauthorizationport.ErrCorrupt
	}
	found := errors.New("PII authorization record found")
	err := store.grants.Visit(ctx, func(finalauthorityadapter.SecurePrivateCASFile) error { return found })
	if errors.Is(err, found) {
		return true, nil
	}
	return false, err
}

func (store *Store) VisitGrants(ctx context.Context, visit func(domainpii.PIIProjectionGrantV1) error) error {
	if store == nil || store.grants == nil || ctx == nil || visit == nil {
		return piiauthorizationport.ErrCorrupt
	}
	return store.grants.Visit(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		grant, err := domainpii.ParsePIIProjectionGrantV1(file.Body)
		if err != nil || grant.RecordDigest != file.Digest {
			return errors.Join(piiauthorizationport.ErrCorrupt, err)
		}
		return visit(grant)
	})
}
