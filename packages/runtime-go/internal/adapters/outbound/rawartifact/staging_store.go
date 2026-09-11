//go:build !analytix_prod

package rawartifact

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	rawartifactport "analytix.local/runtime-go/internal/ports/rawartifact"
)

const (
	maxAcquisitionIntentBytes = 64 << 10
	maxSourceLocatorBytes     = 64 << 10
	maxChunkBytes             = 8 << 20
	maxContentIndexPageBytes  = 512 << 10
	maxContentRootBytes       = 512 << 10
	maxEntryBytes             = 64 << 10
	maxManifestPageBytes      = 1 << 20
	maxManifestBytes          = 512 << 10
)

// StagingStore is deliberately excluded from analytix_prod. SecurePrivateCAS
// currently validates an eagerly materialized inventory capped below the raw
// artifact V1 hierarchy's 1 TiB / 131,072-chunk contract. This bounded adapter
// remains a conformance oracle only; production must use the authenticated
// host-stream acquisition path plus a large-opaque transactional store whose
// capacity and restart behavior pass the full contract.
type StagingStore struct {
	intents       *finalauthorityadapter.SecurePrivateCAS
	locators      *finalauthorityadapter.SecurePrivateCAS
	chunks        *finalauthorityadapter.SecurePrivateCAS
	contentPages  *finalauthorityadapter.SecurePrivateCAS
	contentRoots  *finalauthorityadapter.SecurePrivateCAS
	entries       *finalauthorityadapter.SecurePrivateCAS
	manifestPages *finalauthorityadapter.SecurePrivateCAS
	manifests     *finalauthorityadapter.SecurePrivateCAS

	owners    []*finalauthorityadapter.SecurePrivateCAS
	closeOnce sync.Once
	closeErr  error
}

var _ rawartifactport.StagingStore = (*StagingStore)(nil)
var _ rawartifactport.ExactReader = (*StagingStore)(nil)

func NewStagingStore(
	root string,
	access finalauthorityadapter.SecurePrivateCASAccessAuthority,
) (_ *StagingStore, resultErr error) {
	if root == "" || root != strings.TrimSpace(root) || access == nil {
		return nil, errors.New("raw artifact staging root is invalid")
	}
	store := &StagingStore{}
	complete := false
	defer func() {
		if !complete {
			resultErr = errors.Join(resultErr, store.Close())
		}
	}()
	open := func(leaf string, maxBytes int) (*finalauthorityadapter.SecurePrivateCAS, error) {
		cas, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(root, leaf), maxBytes, access)
		if err == nil {
			store.owners = append(store.owners, cas)
		}
		return cas, err
	}
	var err error
	if store.intents, err = open("acquisition-intents", maxAcquisitionIntentBytes); err != nil {
		return nil, err
	}
	if store.locators, err = open("source-locators", maxSourceLocatorBytes); err != nil {
		return nil, err
	}
	if store.chunks, err = open("content-chunks", maxChunkBytes); err != nil {
		return nil, err
	}
	if store.contentPages, err = open("content-index-pages", maxContentIndexPageBytes); err != nil {
		return nil, err
	}
	if store.contentRoots, err = open("content-roots", maxContentRootBytes); err != nil {
		return nil, err
	}
	if store.entries, err = open("artifact-entries", maxEntryBytes); err != nil {
		return nil, err
	}
	if store.manifestPages, err = open("manifest-pages", maxManifestPageBytes); err != nil {
		return nil, err
	}
	if store.manifests, err = open("manifests", maxManifestBytes); err != nil {
		return nil, err
	}
	complete = true
	return store, nil
}

func (store *StagingStore) Close() error {
	if store == nil {
		return nil
	}
	store.closeOnce.Do(func() {
		for index := len(store.owners) - 1; index >= 0; index-- {
			store.closeErr = errors.Join(store.closeErr, store.owners[index].Close())
		}
	})
	return store.closeErr
}

func (store *StagingStore) StageAcquisitionIntentExact(
	ctx context.Context,
	value domainevidence.RawArtifactAcquisitionIntentV1,
) error {
	if store == nil {
		return errors.New("raw artifact staging store is unavailable")
	}
	body, err := domainevidence.RawArtifactAcquisitionIntentV1Bytes(value)
	if err != nil {
		return err
	}
	return stageCanonical(ctx, store.intents, value.IntentDigest, body, func(raw []byte) error {
		parsed, err := domainevidence.ParseRawArtifactAcquisitionIntentV1(raw)
		if err != nil || parsed.IntentDigest != value.IntentDigest {
			return errors.New("raw artifact acquisition intent CAS body is invalid")
		}
		return nil
	})
}

func (store *StagingStore) StageSourceLocatorExact(
	ctx context.Context,
	value domainevidence.RawArtifactSourceLocatorV1,
) error {
	if store == nil {
		return errors.New("raw artifact staging store is unavailable")
	}
	body, err := domainevidence.RawArtifactSourceLocatorV1Bytes(value)
	if err != nil {
		return err
	}
	return stageCanonical(ctx, store.locators, value.LocatorDigest, body, func(raw []byte) error {
		parsed, err := domainevidence.ParseRawArtifactSourceLocatorV1(raw)
		if err != nil || parsed.LocatorDigest != value.LocatorDigest {
			return errors.New("raw artifact source locator CAS body is invalid")
		}
		return nil
	})
}

func (store *StagingStore) StageChunkExact(
	ctx context.Context,
	descriptor domainevidence.RawArtifactContentChunkDescriptorV1,
	body []byte,
) error {
	if store == nil {
		return errors.New("raw artifact staging store is unavailable")
	}
	if domainevidence.ValidateRawArtifactContentChunkBytesV1(descriptor, body) != nil {
		return errors.New("raw artifact chunk staging input is invalid")
	}
	return stageCanonical(ctx, store.chunks, descriptor.DescriptorDigest, body, func(raw []byte) error {
		return domainevidence.ValidateRawArtifactContentChunkBytesV1(descriptor, raw)
	})
}

func (store *StagingStore) StageContentIndexPageExact(
	ctx context.Context,
	value domainevidence.RawArtifactContentIndexPageV1,
) error {
	if store == nil {
		return errors.New("raw artifact staging store is unavailable")
	}
	body, err := domainevidence.RawArtifactContentIndexPageV1Bytes(value)
	if err != nil {
		return err
	}
	return stageCanonical(ctx, store.contentPages, value.IndexPageDigest, body, func(raw []byte) error {
		parsed, err := domainevidence.ParseRawArtifactContentIndexPageV1(raw)
		if err != nil || parsed.IndexPageDigest != value.IndexPageDigest {
			return errors.New("raw artifact content index CAS body is invalid")
		}
		return nil
	})
}

func (store *StagingStore) StageContentRootExact(
	ctx context.Context,
	value domainevidence.RawArtifactContentRootV1,
) error {
	if store == nil {
		return errors.New("raw artifact staging store is unavailable")
	}
	body, err := domainevidence.RawArtifactContentRootV1Bytes(value)
	if err != nil {
		return err
	}
	return stageCanonical(ctx, store.contentRoots, value.RootDigest, body, func(raw []byte) error {
		parsed, err := domainevidence.ParseRawArtifactContentRootV1(raw)
		if err != nil || parsed.RootDigest != value.RootDigest {
			return errors.New("raw artifact content root CAS body is invalid")
		}
		return nil
	})
}

func (store *StagingStore) StageEntryExact(
	ctx context.Context,
	value domainevidence.RawArtifactEntryV1,
) error {
	if store == nil {
		return errors.New("raw artifact staging store is unavailable")
	}
	body, err := domainevidence.RawArtifactEntryV1Bytes(value)
	if err != nil {
		return err
	}
	return stageCanonical(ctx, store.entries, value.EntryDigest, body, func(raw []byte) error {
		parsed, err := domainevidence.ParseRawArtifactEntryV1(raw)
		if err != nil || parsed.EntryDigest != value.EntryDigest {
			return errors.New("raw artifact entry CAS body is invalid")
		}
		return nil
	})
}

func (store *StagingStore) StageManifestPageExact(
	ctx context.Context,
	value domainevidence.RawArtifactManifestPageV1,
) error {
	if store == nil {
		return errors.New("raw artifact staging store is unavailable")
	}
	body, err := domainevidence.RawArtifactManifestPageV1Bytes(value)
	if err != nil {
		return err
	}
	return stageCanonical(ctx, store.manifestPages, value.PageDigest, body, func(raw []byte) error {
		parsed, err := domainevidence.ParseRawArtifactManifestPageV1(raw)
		if err != nil || parsed.PageDigest != value.PageDigest {
			return errors.New("raw artifact manifest page CAS body is invalid")
		}
		return nil
	})
}

func (store *StagingStore) StageManifestExact(
	ctx context.Context,
	value domainevidence.RawArtifactManifestV1,
) error {
	if store == nil {
		return errors.New("raw artifact staging store is unavailable")
	}
	body, err := domainevidence.RawArtifactManifestV1Bytes(value)
	if err != nil {
		return err
	}
	return stageCanonical(ctx, store.manifests, value.ManifestDigest, body, func(raw []byte) error {
		parsed, err := domainevidence.ParseRawArtifactManifestV1(raw)
		if err != nil || parsed.ManifestDigest != value.ManifestDigest {
			return errors.New("raw artifact manifest CAS body is invalid")
		}
		return nil
	})
}

func (store *StagingStore) ResolveAcquisitionIntentExact(
	ctx context.Context,
	manifest domainevidence.RawArtifactManifestV1,
) (domainevidence.RawArtifactAcquisitionIntentV1, error) {
	if store == nil || domainevidence.ValidateRawArtifactManifestV1(manifest) != nil {
		return domainevidence.RawArtifactAcquisitionIntentV1{}, errors.New("raw artifact exact reader is unavailable")
	}
	intent, err := readCanonicalExact(
		ctx, store.intents,
		rawartifactport.ExactObjectReferenceV1{
			Digest: manifest.AcquisitionIntentDigest, SHA256: manifest.AcquisitionIntentSHA256,
			ByteLength: manifest.AcquisitionIntentByteLength,
		},
		maxAcquisitionIntentBytes,
		domainevidence.ParseRawArtifactAcquisitionIntentV1,
		func(value domainevidence.RawArtifactAcquisitionIntentV1) string { return value.IntentDigest },
	)
	if err != nil {
		return domainevidence.RawArtifactAcquisitionIntentV1{}, err
	}
	if domainevidence.ValidateRawArtifactManifestForIntentV1(intent, manifest) != nil {
		return domainevidence.RawArtifactAcquisitionIntentV1{}, errors.New("raw artifact acquisition intent exact readback failed")
	}
	return intent, nil
}

func (store *StagingStore) ResolveManifestExact(
	ctx context.Context,
	reference rawartifactport.ExactObjectReferenceV1,
) (domainevidence.RawArtifactManifestV1, error) {
	if store == nil {
		return domainevidence.RawArtifactManifestV1{}, errors.New("raw artifact exact reader is unavailable")
	}
	return readCanonicalExact(
		ctx, store.manifests, reference, maxManifestBytes,
		domainevidence.ParseRawArtifactManifestV1,
		func(value domainevidence.RawArtifactManifestV1) string { return value.ManifestDigest },
	)
}

func (store *StagingStore) ResolveManifestPageExact(
	ctx context.Context,
	descriptor domainevidence.RawArtifactManifestPageDescriptorV1,
) (domainevidence.RawArtifactManifestPageV1, error) {
	if store == nil || domainevidence.ValidateRawArtifactManifestPageDescriptorV1(descriptor) != nil {
		return domainevidence.RawArtifactManifestPageV1{}, errors.New("raw artifact manifest page exact reference is invalid")
	}
	page, err := readCanonicalExact(
		ctx, store.manifestPages,
		rawartifactport.ExactObjectReferenceV1{
			Digest: descriptor.PageDigest, SHA256: descriptor.PageSHA256, ByteLength: descriptor.PageByteLength,
		},
		maxManifestPageBytes,
		domainevidence.ParseRawArtifactManifestPageV1,
		func(value domainevidence.RawArtifactManifestPageV1) string { return value.PageDigest },
	)
	if err != nil {
		return domainevidence.RawArtifactManifestPageV1{}, err
	}
	if domainevidence.ValidateRawArtifactManifestPageAgainstDescriptorV1(descriptor, page) != nil {
		return domainevidence.RawArtifactManifestPageV1{}, errors.New("raw artifact manifest page exact readback failed")
	}
	return page, nil
}

func (store *StagingStore) ResolveEntryExact(
	ctx context.Context,
	reference rawartifactport.ExactObjectReferenceV1,
) (domainevidence.RawArtifactEntryV1, error) {
	if store == nil {
		return domainevidence.RawArtifactEntryV1{}, errors.New("raw artifact exact reader is unavailable")
	}
	return readCanonicalExact(
		ctx, store.entries, reference, maxEntryBytes,
		domainevidence.ParseRawArtifactEntryV1,
		func(value domainevidence.RawArtifactEntryV1) string { return value.EntryDigest },
	)
}

func (store *StagingStore) ResolveSourceLocatorExact(
	ctx context.Context,
	entry domainevidence.RawArtifactEntryV1,
) (domainevidence.RawArtifactSourceLocatorV1, error) {
	if store == nil || domainevidence.ValidateRawArtifactEntryV1(entry) != nil {
		return domainevidence.RawArtifactSourceLocatorV1{}, errors.New("raw artifact source locator exact reference is invalid")
	}
	locator, err := readCanonicalExact(
		ctx, store.locators,
		rawartifactport.ExactObjectReferenceV1{
			Digest: entry.SourceLocatorDigest, SHA256: entry.SourceLocatorSHA256, ByteLength: entry.SourceLocatorByteLength,
		},
		maxSourceLocatorBytes,
		domainevidence.ParseRawArtifactSourceLocatorV1,
		func(value domainevidence.RawArtifactSourceLocatorV1) string { return value.LocatorDigest },
	)
	if err != nil {
		return domainevidence.RawArtifactSourceLocatorV1{}, err
	}
	if locator.BindingKeyDigest != entry.BindingKeyDigest ||
		locator.AcquisitionIntentDigest != entry.AcquisitionIntentDigest ||
		locator.SourceOccurrenceOrdinal != entry.ArtifactOrdinal || locator.SourceFileIDDigest != entry.SourceFileIDDigest {
		return domainevidence.RawArtifactSourceLocatorV1{}, errors.New("raw artifact source locator exact readback failed")
	}
	return locator, nil
}

func (store *StagingStore) ResolveContentRootExact(
	ctx context.Context,
	entry domainevidence.RawArtifactEntryV1,
) (domainevidence.RawArtifactContentRootV1, error) {
	if store == nil || domainevidence.ValidateRawArtifactEntryV1(entry) != nil {
		return domainevidence.RawArtifactContentRootV1{}, errors.New("raw artifact content root exact reference is invalid")
	}
	root, err := readCanonicalExact(
		ctx, store.contentRoots,
		rawartifactport.ExactObjectReferenceV1{
			Digest: entry.ContentRootDigest, SHA256: entry.ContentRootSHA256, ByteLength: entry.ContentRootByteLength,
		},
		maxContentRootBytes,
		domainevidence.ParseRawArtifactContentRootV1,
		func(value domainevidence.RawArtifactContentRootV1) string { return value.RootDigest },
	)
	if err != nil {
		return domainevidence.RawArtifactContentRootV1{}, err
	}
	if domainevidence.ValidateRawArtifactEntryWithContentRootV1(entry, root) != nil {
		return domainevidence.RawArtifactContentRootV1{}, errors.New("raw artifact content root exact readback failed")
	}
	return root, nil
}

func (store *StagingStore) ResolveContentIndexPageExact(
	ctx context.Context,
	descriptor domainevidence.RawArtifactContentIndexPageDescriptorV1,
) (domainevidence.RawArtifactContentIndexPageV1, error) {
	if store == nil || domainevidence.ValidateRawArtifactContentIndexPageDescriptorV1(descriptor) != nil {
		return domainevidence.RawArtifactContentIndexPageV1{}, errors.New("raw artifact content page exact reference is invalid")
	}
	page, err := readCanonicalExact(
		ctx, store.contentPages,
		rawartifactport.ExactObjectReferenceV1{
			Digest: descriptor.IndexPageDigest, SHA256: descriptor.IndexPageSHA256, ByteLength: descriptor.IndexPageByteLength,
		},
		maxContentIndexPageBytes,
		domainevidence.ParseRawArtifactContentIndexPageV1,
		func(value domainevidence.RawArtifactContentIndexPageV1) string { return value.IndexPageDigest },
	)
	if err != nil {
		return domainevidence.RawArtifactContentIndexPageV1{}, err
	}
	if domainevidence.ValidateRawArtifactContentIndexPageAgainstDescriptorV1(descriptor, page) != nil {
		return domainevidence.RawArtifactContentIndexPageV1{}, errors.New("raw artifact content page exact readback failed")
	}
	return page, nil
}

func (store *StagingStore) ResolveChunkExact(
	ctx context.Context,
	descriptor domainevidence.RawArtifactContentChunkDescriptorV1,
) ([]byte, error) {
	if store == nil || domainevidence.ValidateRawArtifactContentChunkDescriptorV1(descriptor) != nil {
		return nil, errors.New("raw artifact chunk exact reference is invalid")
	}
	body, err := readExactBytes(
		ctx, store.chunks,
		rawartifactport.ExactObjectReferenceV1{
			Digest: descriptor.DescriptorDigest, SHA256: descriptor.ChunkSHA256, ByteLength: descriptor.ChunkByteLength,
		},
		maxChunkBytes,
	)
	if err != nil {
		return nil, err
	}
	if domainevidence.ValidateRawArtifactContentChunkBytesV1(descriptor, body) != nil {
		return nil, errors.New("raw artifact chunk exact readback failed")
	}
	return body, nil
}

func stageCanonical(
	ctx context.Context,
	cas *finalauthorityadapter.SecurePrivateCAS,
	digest string,
	body []byte,
	validate func([]byte) error,
) error {
	if ctx == nil || cas == nil || !domainsecurity.IsSHA256Hex(strings.TrimSpace(digest)) ||
		digest != strings.TrimSpace(digest) || len(body) == 0 || validate == nil {
		return errors.New("raw artifact private CAS staging input is invalid")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	stable := append([]byte(nil), body...)
	if current, readErr := cas.Read(ctx, digest); readErr == nil {
		if !bytes.Equal(current, stable) || validate(current) != nil {
			return errors.New("raw artifact private CAS content-address conflict")
		}
		return nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	if err := cas.PutIfAbsent(ctx, digest, stable); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	readback, err := cas.Read(ctx, digest)
	if err != nil || !bytes.Equal(readback, stable) || validate(readback) != nil {
		return errors.New("raw artifact private CAS exact write readback failed")
	}
	return nil
}

func readCanonicalExact[T any](
	ctx context.Context,
	cas *finalauthorityadapter.SecurePrivateCAS,
	reference rawartifactport.ExactObjectReferenceV1,
	maxBytes int,
	parse func([]byte) (T, error),
	logicalDigest func(T) string,
) (T, error) {
	var zero T
	if parse == nil || logicalDigest == nil {
		return zero, errors.New("raw artifact canonical exact reader is invalid")
	}
	body, err := readExactBytes(ctx, cas, reference, maxBytes)
	if err != nil {
		return zero, err
	}
	value, err := parse(body)
	if err != nil || logicalDigest(value) != reference.Digest {
		return zero, errors.New("raw artifact canonical exact body is invalid")
	}
	return value, nil
}

func readExactBytes(
	ctx context.Context,
	cas *finalauthorityadapter.SecurePrivateCAS,
	reference rawartifactport.ExactObjectReferenceV1,
	maxBytes int,
) ([]byte, error) {
	if ctx == nil || cas == nil || maxBytes <= 0 || reference.Digest != strings.TrimSpace(reference.Digest) ||
		reference.SHA256 != strings.TrimSpace(reference.SHA256) || !domainsecurity.IsSHA256Hex(reference.Digest) ||
		!domainsecurity.IsSHA256Hex(reference.SHA256) || reference.ByteLength == 0 ||
		reference.ByteLength > uint64(maxBytes) {
		return nil, errors.New("raw artifact exact object reference is invalid")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	body, err := cas.Read(ctx, reference.Digest)
	if err != nil {
		return nil, err
	}
	if uint64(len(body)) != reference.ByteLength || domainsecurity.SHA256Hex(body) != reference.SHA256 {
		return nil, errors.New("raw artifact exact object physical address mismatch")
	}
	return body, nil
}
