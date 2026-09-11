package datasetsnapshot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
)

const (
	authorityBundleSchemaVersionV2 = 2
	authorityBundlePurposeV2       = "analytix.dataset-snapshot-authority-bundle/v2"
	maxAuthorityBundleBytesV2      = 1024 * 1024
	maxAdmissionMaterialBytesV2    = maxAuthorityBundleBytesV2 * 16
)

const (
	legacyRecordsLeafV2    = "legacy-records"
	authorityBundlesLeafV2 = "authority-bundles-v2"
	indexesLeafV2          = "indexes"
	materialsLeafV2        = "materials"
)

// MaterialStoreV2 is an exact-read-only view over separately owned private
// CAS roots. It exposes no inventory, latest selector, or write path. The CAS
// handles are injected by the composition owner so this adapter cannot create
// an unregistered persistence root.
type MaterialStoreV2 struct {
	stores map[datasetsnapshotport.MaterialKindV2]*finalauthorityadapter.SecurePrivateCAS
}

var _ datasetsnapshotport.AdmissionMaterialReaderV2 = (*MaterialStoreV2)(nil)

func NewMaterialStoreV2(
	stores map[datasetsnapshotport.MaterialKindV2]*finalauthorityadapter.SecurePrivateCAS,
) (*MaterialStoreV2, error) {
	if len(stores) == 0 {
		return nil, errors.New("dataset snapshot material stores are unavailable")
	}
	cloned := make(map[datasetsnapshotport.MaterialKindV2]*finalauthorityadapter.SecurePrivateCAS, len(stores))
	for kind, store := range stores {
		if !validMaterialKindV2(kind) || store == nil {
			return nil, errors.New("dataset snapshot material store configuration is invalid")
		}
		cloned[kind] = store
	}
	return &MaterialStoreV2{stores: cloned}, nil
}

func (store *MaterialStoreV2) ResolveExact(
	ctx context.Context,
	kind datasetsnapshotport.MaterialKindV2,
	reference datasetsnapshotport.ExactMaterialReferenceV2,
) ([]byte, error) {
	cas := (*finalauthorityadapter.SecurePrivateCAS)(nil)
	if store != nil {
		cas = store.stores[kind]
	}
	if cas == nil || !validMaterialKindV2(kind) ||
		reference.Address != reference.SHA256 || !domainsecurity.IsSHA256Hex(reference.Address) ||
		reference.ByteLength == 0 || reference.ByteLength > uint64(maxAdmissionMaterialBytesV2) {
		return nil, errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot material reference is invalid"))
	}
	body, err := cas.Read(ctx, reference.Address)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errors.Join(datasetsnapshotport.ErrNotFound, err)
		}
		return nil, errors.Join(datasetsnapshotport.ErrUnavailable, err)
	}
	if uint64(len(body)) != reference.ByteLength || domainsecurity.SHA256Hex(body) != reference.SHA256 {
		return nil, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot material exact bytes mismatch"))
	}
	return append([]byte(nil), body...), nil
}

// StoresV2 owns the four fixed private-CAS leaves used by the read-only DSV2
// runtime path. Material kinds share one SHA-256-addressed opaque CAS: the app
// still parses each object under its exact expected kind and re-reads the full
// parent graph before returning a witnessed snapshot.
type StoresV2 struct {
	LegacyRecords    *RecordStore
	AuthorityBundles *AuthorityBundleStoreV2
	Indexes          *IndexStore
	Materials        *MaterialStoreV2

	materialCAS *finalauthorityadapter.SecurePrivateCAS
	bundleCAS   *finalauthorityadapter.SecurePrivateCAS
}

func OpenStoresV2(
	root string,
	access finalauthorityadapter.SecurePrivateCASAccessAuthority,
) (_ *StoresV2, resultErr error) {
	root = strings.TrimSpace(root)
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root || access == nil {
		return nil, errors.New("dataset snapshot v2 store root is invalid")
	}
	stores := &StoresV2{}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, stores.Close())
		}
	}()
	legacy, err := NewRecordStore(filepath.Join(root, legacyRecordsLeafV2), access)
	if err != nil {
		return nil, err
	}
	stores.LegacyRecords = legacy
	indexes, err := NewIndexStore(filepath.Join(root, indexesLeafV2), access)
	if err != nil {
		return nil, err
	}
	stores.Indexes = indexes
	bundleCAS, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(root, authorityBundlesLeafV2), maxAuthorityBundleBytesV2, access,
	)
	if err != nil {
		return nil, err
	}
	stores.bundleCAS = bundleCAS
	bundles, err := NewAuthorityBundleStoreV2(bundleCAS)
	if err != nil {
		return nil, err
	}
	stores.AuthorityBundles = bundles
	materialCAS, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(root, materialsLeafV2), maxAdmissionMaterialBytesV2, access,
	)
	if err != nil {
		return nil, err
	}
	stores.materialCAS = materialCAS
	materialStores := make(map[datasetsnapshotport.MaterialKindV2]*finalauthorityadapter.SecurePrivateCAS, len(materialKindsV2()))
	for _, kind := range materialKindsV2() {
		materialStores[kind] = materialCAS
	}
	materials, err := NewMaterialStoreV2(materialStores)
	if err != nil {
		return nil, err
	}
	stores.Materials = materials
	return stores, nil
}

func (stores *StoresV2) Close() error {
	if stores == nil {
		return nil
	}
	var result error
	for _, cas := range []*finalauthorityadapter.SecurePrivateCAS{stores.materialCAS, stores.bundleCAS} {
		if cas != nil {
			result = errors.Join(result, cas.Close())
		}
	}
	if stores.Indexes != nil && stores.Indexes.cas != nil {
		result = errors.Join(result, stores.Indexes.cas.Close())
	}
	if stores.LegacyRecords != nil && stores.LegacyRecords.cas != nil {
		result = errors.Join(result, stores.LegacyRecords.cas.Close())
	}
	return result
}

func (stores *StoresV2) HasRecords(ctx context.Context) (bool, error) {
	if stores == nil || stores.LegacyRecords == nil || stores.LegacyRecords.cas == nil || stores.Indexes == nil ||
		stores.Indexes.cas == nil || stores.bundleCAS == nil || stores.materialCAS == nil || ctx == nil {
		return false, errors.New("dataset snapshot v2 stores are unavailable")
	}
	found := errors.New("dataset snapshot v2 inventory record found")
	storesToVisit := make([]*finalauthorityadapter.SecurePrivateCAS, 0, 4)
	storesToVisit = append(storesToVisit, stores.LegacyRecords.cas, stores.bundleCAS, stores.Indexes.cas, stores.materialCAS)
	for _, cas := range storesToVisit {
		err := cas.Visit(ctx, func(finalauthorityadapter.SecurePrivateCASFile) error { return found })
		if errors.Is(err, found) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
	}
	return false, nil
}

// PutFundsCanonicalCSVAdmissionMaterialV1 is the only production writer for
// the fixed CSV admission composer. It accepts no caller-selected kind or
// address, and every immutable write is read back before the material can be
// passed to SealedServiceV2 for full-graph verification.
func (stores *StoresV2) PutFundsCanonicalCSVAdmissionMaterialV1(
	ctx context.Context,
	material domainevidence.FundsCanonicalCSVAdmissionMaterialV1,
) error {
	if stores == nil || stores.materialCAS == nil || ctx == nil || ctx.Err() != nil {
		return errors.Join(datasetsnapshotport.ErrUnavailable, errors.New("dataset snapshot material writer is unavailable"))
	}
	return material.UseExactV1(func(
		kind domainevidence.FundsCanonicalCSVAdmissionMaterialKindV1,
		address string,
		body []byte,
	) error {
		if !validMaterialKindV2(datasetsnapshotport.MaterialKindV2(kind)) ||
			len(body) == 0 || len(body) > maxAdmissionMaterialBytesV2 {
			return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot admission material is invalid"))
		}
		if err := stores.materialCAS.PutIfAbsent(ctx, address, body); err != nil && !errors.Is(err, os.ErrExist) {
			return errors.Join(datasetsnapshotport.ErrUnavailable, err)
		}
		written, err := stores.materialCAS.Read(ctx, address)
		if err != nil || !bytes.Equal(written, body) || domainsecurity.SHA256Hex(written) != address {
			return errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot admission material write readback failed"), err)
		}
		return nil
	})
}

// AuthorityBundleStoreV2 persists the signed record and the exact manifest in
// one immutable CAS body. A crash can therefore leave an unreferenced bundle,
// but can never expose a torn record/manifest pair through the witnessed index.
type AuthorityBundleStoreV2 struct {
	cas *finalauthorityadapter.SecurePrivateCAS
}

var _ datasetsnapshotport.AuthorityBundleStoreV2 = (*AuthorityBundleStoreV2)(nil)

func NewAuthorityBundleStoreV2(cas *finalauthorityadapter.SecurePrivateCAS) (*AuthorityBundleStoreV2, error) {
	if cas == nil {
		return nil, errors.New("dataset snapshot authority bundle CAS is unavailable")
	}
	return &AuthorityBundleStoreV2{cas: cas}, nil
}

type authorityBundleEnvelopeV2 struct {
	SchemaVersion int                                             `json:"schemaVersion"`
	Purpose       string                                          `json:"purpose"`
	Record        domainsecurity.DatasetSnapshotAuthorityRecordV2 `json:"record"`
	Manifest      domainsecurity.DatasetSnapshotManifestV2        `json:"manifest"`
}

func (store *AuthorityBundleStoreV2) PutIfAbsent(
	ctx context.Context,
	bundle datasetsnapshotport.AuthorityBundleV2,
) error {
	if store == nil || store.cas == nil || validateAuthorityBundleV2(bundle) != nil {
		return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot authority bundle is invalid"))
	}
	body, err := authorityBundleBytesV2(bundle)
	if err != nil {
		return errors.Join(datasetsnapshotport.ErrCorrupt, err)
	}
	if current, readErr := store.cas.Read(ctx, bundle.Record.RecordDigest); readErr == nil {
		if !bytes.Equal(current, body) {
			return errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot authority bundle content address conflicts"))
		}
		_, err := parseAuthorityBundleV2(bundle.Record.RecordDigest, current)
		return err
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return errors.Join(datasetsnapshotport.ErrUnavailable, readErr)
	}
	if err := store.cas.PutIfAbsent(ctx, bundle.Record.RecordDigest, body); err != nil && !errors.Is(err, os.ErrExist) {
		return errors.Join(datasetsnapshotport.ErrUnavailable, err)
	}
	written, err := store.cas.Read(ctx, bundle.Record.RecordDigest)
	if err != nil || !bytes.Equal(written, body) {
		return errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot authority bundle write readback failed"), err)
	}
	_, err = parseAuthorityBundleV2(bundle.Record.RecordDigest, written)
	return err
}

func (store *AuthorityBundleStoreV2) Resolve(
	ctx context.Context,
	recordDigest string,
) (datasetsnapshotport.AuthorityBundleV2, error) {
	if store == nil || store.cas == nil || !domainsecurity.IsSHA256Hex(recordDigest) {
		return datasetsnapshotport.AuthorityBundleV2{}, errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot authority bundle address is invalid"))
	}
	body, err := store.cas.Read(ctx, recordDigest)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return datasetsnapshotport.AuthorityBundleV2{}, errors.Join(datasetsnapshotport.ErrNotFound, err)
		}
		return datasetsnapshotport.AuthorityBundleV2{}, errors.Join(datasetsnapshotport.ErrUnavailable, err)
	}
	return parseAuthorityBundleV2(recordDigest, body)
}

func authorityBundleBytesV2(bundle datasetsnapshotport.AuthorityBundleV2) ([]byte, error) {
	if err := validateAuthorityBundleV2(bundle); err != nil {
		return nil, err
	}
	body, err := json.Marshal(authorityBundleEnvelopeV2{
		SchemaVersion: authorityBundleSchemaVersionV2,
		Purpose:       authorityBundlePurposeV2,
		Record:        bundle.Record,
		Manifest:      bundle.Manifest,
	})
	if err != nil || len(body) == 0 || len(body) > maxAuthorityBundleBytesV2 {
		return nil, errors.New("dataset snapshot authority bundle bytes are invalid")
	}
	return body, nil
}

func parseAuthorityBundleV2(
	recordDigest string,
	body []byte,
) (datasetsnapshotport.AuthorityBundleV2, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: maxAuthorityBundleBytesV2, MaxDepth: 16,
		MaxTokens: 200_000, MaxStringBytes: 512 * 1024,
	}); err != nil {
		return datasetsnapshotport.AuthorityBundleV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var envelope authorityBundleEnvelopeV2
	if err := decoder.Decode(&envelope); err != nil {
		return datasetsnapshotport.AuthorityBundleV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return datasetsnapshotport.AuthorityBundleV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot authority bundle contains trailing JSON"))
	}
	bundle := datasetsnapshotport.AuthorityBundleV2{Record: envelope.Record, Manifest: envelope.Manifest}
	canonical, err := authorityBundleBytesV2(bundle)
	if envelope.SchemaVersion != authorityBundleSchemaVersionV2 || envelope.Purpose != authorityBundlePurposeV2 ||
		err != nil || !bytes.Equal(body, canonical) || bundle.Record.RecordDigest != recordDigest {
		return datasetsnapshotport.AuthorityBundleV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot authority bundle is not canonical"))
	}
	return bundle, nil
}

func validateAuthorityBundleV2(bundle datasetsnapshotport.AuthorityBundleV2) error {
	return domainsecurity.ValidateDatasetSnapshotAuthorityRecordForManifestV2(bundle.Record, bundle.Manifest)
}

func validMaterialKindV2(kind datasetsnapshotport.MaterialKindV2) bool {
	switch kind {
	case datasetsnapshotport.MaterialSnapshotManifestV2,
		datasetsnapshotport.MaterialFundsProducerContentV1,
		datasetsnapshotport.MaterialRawAcquisitionIntentV1,
		datasetsnapshotport.MaterialRawManifestV1,
		datasetsnapshotport.MaterialRawManifestPageV1,
		datasetsnapshotport.MaterialRawEntryV1,
		datasetsnapshotport.MaterialRawSourceLocatorV1,
		datasetsnapshotport.MaterialRawContentRootV1,
		datasetsnapshotport.MaterialRawContentIndexPageV1,
		datasetsnapshotport.MaterialRawContentChunkV1,
		datasetsnapshotport.MaterialParsedConfigurationV1,
		datasetsnapshotport.MaterialParsedIdentityV1,
		datasetsnapshotport.MaterialParsedReceiptV1,
		datasetsnapshotport.MaterialClassificationLedgerV1,
		datasetsnapshotport.MaterialParsedIndexPageV1,
		datasetsnapshotport.MaterialParsedPageV1,
		datasetsnapshotport.MaterialSourceRowLedgerRootV1,
		datasetsnapshotport.MaterialSourceRowIndexPageV1,
		datasetsnapshotport.MaterialSourceRowPageV1,
		datasetsnapshotport.MaterialSourceRowRecordV1,
		datasetsnapshotport.MaterialSourceRowLineageV1:
		return true
	default:
		return false
	}
}

func materialKindsV2() []datasetsnapshotport.MaterialKindV2 {
	return []datasetsnapshotport.MaterialKindV2{
		datasetsnapshotport.MaterialSnapshotManifestV2,
		datasetsnapshotport.MaterialFundsProducerContentV1,
		datasetsnapshotport.MaterialRawAcquisitionIntentV1,
		datasetsnapshotport.MaterialRawManifestV1,
		datasetsnapshotport.MaterialRawManifestPageV1,
		datasetsnapshotport.MaterialRawEntryV1,
		datasetsnapshotport.MaterialRawSourceLocatorV1,
		datasetsnapshotport.MaterialRawContentRootV1,
		datasetsnapshotport.MaterialRawContentIndexPageV1,
		datasetsnapshotport.MaterialRawContentChunkV1,
		datasetsnapshotport.MaterialParsedConfigurationV1,
		datasetsnapshotport.MaterialParsedIdentityV1,
		datasetsnapshotport.MaterialParsedReceiptV1,
		datasetsnapshotport.MaterialClassificationLedgerV1,
		datasetsnapshotport.MaterialParsedIndexPageV1,
		datasetsnapshotport.MaterialParsedPageV1,
		datasetsnapshotport.MaterialSourceRowLedgerRootV1,
		datasetsnapshotport.MaterialSourceRowIndexPageV1,
		datasetsnapshotport.MaterialSourceRowPageV1,
		datasetsnapshotport.MaterialSourceRowRecordV1,
		datasetsnapshotport.MaterialSourceRowLineageV1,
	}
}
