package datasetsnapshot

import (
	"context"
	"errors"
	"reflect"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
)

// ValidateOriginalMaterialGraphV2 verifies retained bytes only. It does not
// initialize a service, perform admission, contact a witness or select a head.
// Missing historical material remains a missing graph, never a new admission.
func ValidateOriginalMaterialGraphV2(ctx context.Context, bundle datasetport.AuthorityBundleV2, materials datasetport.AdmissionMaterialReaderV2) error {
	if ctx == nil || materials == nil {
		return errors.New("original dataset material reader is unavailable")
	}
	service := &SealedServiceV2{materials: materials}
	material, err := service.verifyExactMaterialV2(ctx,
		exactReferenceV2(bundle.Record.ManifestSHA256, bundle.Record.ManifestByteLength),
		exactReferenceV2(bundle.Manifest.ProducerContentManifestSHA256, bundle.Manifest.ProducerContentManifestByteLength))
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(material.manifest, bundle.Manifest) {
		return errors.New("original dataset manifest differs from its retained bundle")
	}
	return domainsecurity.ValidateDatasetSnapshotAuthorityRecordForFundsProducerContentV2(bundle.Record, material.manifest, material.producer)
}
