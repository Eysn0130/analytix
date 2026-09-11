package datasetsnapshot

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// PreparedRecoveryV2 freezes all DSV2 leaves as one owner. Physical CAS
// integrity is necessary but not sufficient: semantic validation also binds
// every index to an immutable record/bundle and every bundle to the exact
// top-level material objects it names.
type PreparedRecoveryV2 struct {
	owner     *finalauthorityadapter.PreparedSecurePrivateCASOwnerRecoveryV1
	validated bool
}

func PrepareRecoveryV2(
	ctx context.Context,
	root string,
	access finalauthorityadapter.SecurePrivateCASRecoveryAccessAuthority,
) (*PreparedRecoveryV2, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil {
		return nil, errors.New("dataset snapshot v2 prepared recovery root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, errors.New("dataset snapshot v2 prepared recovery root is invalid")
	}
	owner, err := finalauthorityadapter.PrepareSecurePrivateCASOwnerRecoveryV1(
		ctx,
		absolute,
		[]finalauthorityadapter.SecurePrivateCASOwnerLeafV1{
			{Name: legacyRecordsLeafV2, MaxBytes: maxDatasetSnapshotRecordBytes},
			{Name: authorityBundlesLeafV2, MaxBytes: maxAuthorityBundleBytesV2},
			{Name: indexesLeafV2, MaxBytes: maxDatasetSnapshotIndexBytes},
			{Name: materialsLeafV2, MaxBytes: maxAdmissionMaterialBytesV2},
		},
		access,
	)
	if err != nil {
		return nil, err
	}
	return &PreparedRecoveryV2{owner: owner}, nil
}

func (prepared *PreparedRecoveryV2) ValidateSemantics(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil {
		return errors.New("dataset snapshot v2 recovery plan is invalid")
	}
	err := prepared.owner.ValidateDomainSemantics(ctx, func(visit finalauthorityadapter.PrivateCASDomainVisitor, visitMaterials finalauthorityadapter.PrivateCASDomainMaterialVisitor) error {
		return prepared.validateDomainSemantics(ctx, visit, visitMaterials)
	})
	prepared.validated = err == nil
	return err
}

func (prepared *PreparedRecoveryV2) validateDomainSemantics(ctx context.Context, visit finalauthorityadapter.PrivateCASDomainVisitor, visitMaterials finalauthorityadapter.PrivateCASDomainMaterialVisitor) error {
	records := make(map[string]struct{})
	bundles := make(map[string]authorityBundleEnvelopeV2)
	indexes := make(map[string]string)
	materials := make(map[string][]byte)
	if err := visit(legacyRecordsLeafV2, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		record, err := parseRecord(file.Digest, file.Body)
		if err != nil || record.RecordDigest != file.Digest {
			return errors.Join(errors.New("dataset snapshot v2 recovery contains a corrupt legacy record"), err)
		}
		records[file.Digest] = struct{}{}
		return nil
	}); err != nil {
		return err
	}
	if err := visit(authorityBundlesLeafV2, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		bundle, err := parseAuthorityBundleV2(file.Digest, file.Body)
		if err != nil {
			return errors.Join(errors.New("dataset snapshot v2 recovery contains a corrupt authority bundle"), err)
		}
		if _, duplicate := records[file.Digest]; duplicate {
			return errors.New("dataset snapshot v2 recovery repeats a record digest across versions")
		}
		records[file.Digest] = struct{}{}
		bundles[file.Digest] = authorityBundleEnvelopeV2{Record: bundle.Record, Manifest: bundle.Manifest}
		return nil
	}); err != nil {
		return err
	}
	if err := visit(indexesLeafV2, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		index, err := parseIndex(file.Digest, file.Body)
		if err != nil || index.IndexDigest != file.Digest {
			return errors.Join(errors.New("dataset snapshot v2 recovery contains a corrupt index"), err)
		}
		indexes[file.Digest] = index.SnapshotRecordDigest
		return nil
	}); err != nil {
		return err
	}
	if err := visit(materialsLeafV2, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		if len(file.Body) == 0 || len(file.Body) > maxAdmissionMaterialBytesV2 || domainsecurity.SHA256Hex(file.Body) != file.Digest {
			return errors.New("dataset snapshot v2 recovery contains corrupt material bytes")
		}
		materials[file.Digest] = append([]byte(nil), file.Body...)
		return nil
	}); err != nil {
		return err
	}
	for _, recordDigest := range indexes {
		if _, found := records[recordDigest]; !found {
			return errors.New("dataset snapshot v2 recovery index lost its immutable record")
		}
	}
	for digest, envelope := range bundles {
		bundle := envelope
		manifestBody, err := domainsecurity.DatasetSnapshotManifestV2Bytes(bundle.Manifest)
		if err != nil || !bytes.Equal(materials[domainsecurity.SHA256Hex(manifestBody)], manifestBody) {
			return errors.Join(errors.New("dataset snapshot v2 recovery bundle lost its exact manifest material"), err)
		}
		producerBody := materials[bundle.Manifest.ProducerContentManifestSHA256]
		producer, err := domainsecurity.ParseFundsProducerContentManifestV1(producerBody)
		if err != nil || domainsecurity.ValidateDatasetSnapshotAuthorityRecordForFundsProducerContentV2(
			bundle.Record, bundle.Manifest, producer,
		) != nil {
			return errors.Join(errors.New("dataset snapshot v2 recovery bundle lost its exact producer material"), err)
		}
		for _, reference := range []struct {
			sha256 string
			length uint64
		}{
			{bundle.Manifest.RawArtifactManifestSHA256, bundle.Manifest.RawArtifactManifestByteLength},
			{bundle.Manifest.ParsedGenerationReceiptSHA256, bundle.Manifest.ParsedGenerationReceiptByteLength},
			{bundle.Manifest.ClassificationLedgerSHA256, bundle.Manifest.ClassificationLedgerByteLength},
			{bundle.Manifest.SourceRowLedgerRootSHA256, bundle.Manifest.SourceRowLedgerRootByteLength},
		} {
			body, found := materials[reference.sha256]
			if !found || uint64(len(body)) != reference.length {
				return errors.New("dataset snapshot v2 recovery bundle lost a top-level immutable material")
			}
		}
		if bundle.Record.RecordDigest != digest {
			return errors.New("dataset snapshot v2 recovery authority bundle address changed")
		}
	}
	return nil
}

func (prepared *PreparedRecoveryV2) Revalidate(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil || !prepared.validated {
		return errors.New("dataset snapshot v2 recovery plan is not validated")
	}
	return prepared.owner.Revalidate(ctx)
}

func (prepared *PreparedRecoveryV2) PrivateCASRecoveryTopologiesV3() []finalauthorityadapter.SecurePrivateCASRecoveryTopologyAuthorityV3 {
	if prepared == nil || prepared.owner == nil {
		return nil
	}
	return []finalauthorityadapter.SecurePrivateCASRecoveryTopologyAuthorityV3{prepared.owner}
}

func (prepared *PreparedRecoveryV2) SecurePrivateCASRecoveryPlansV2() []*finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1 {
	if prepared == nil || prepared.owner == nil {
		return nil
	}
	return prepared.owner.SecurePrivateCASRecoveryPlansV2()
}
