package security

import "errors"

// FundsProducerContentBindingV2 is a PII-free structural projection of the
// exactly-one producer-content value sealed by one DSV2 manifest. It grants no
// storage membership, currentness, execution, evidence, or publication
// authority.
type FundsProducerContentBindingV2 struct {
	Contract                  string
	ID                        string
	ManifestSHA256            string
	ManifestByteLength        uint64
	CaseID                    string
	RawArtifactManifestSHA256 string
	NormalizedContentSHA256   string
	DetailContentSHA256       string
	SourceRowCount            uint64
	AcceptedRowCount          uint64
	RejectedRowCount          uint64
	DuplicateRowCount         uint64
	DetailRowCount            uint64
}

// ResolveFundsProducerContentBindingV2 resolves only the two closed
// producer-content contracts currently accepted by DSV2. The manifest chooses
// the version; empty, additional, unknown, or cross-labelled values fail
// closed.
func ResolveFundsProducerContentBindingV2(
	manifest DatasetSnapshotManifestV2,
	producerV1 FundsProducerContentManifestV1,
	producerV2 FundsProducerContentManifestV2,
) (FundsProducerContentBindingV2, error) {
	switch manifest.ProducerContentContract {
	case FundsProducerContentManifestContractV1:
		if producerV2 != (FundsProducerContentManifestV2{}) ||
			ValidateDatasetSnapshotManifestV2FundsProducerContentV1(manifest, producerV1) != nil {
			return FundsProducerContentBindingV2{}, errors.New("dataset snapshot producer content v1 binding is invalid")
		}
		body, err := FundsProducerContentManifestV1Bytes(producerV1)
		if err != nil {
			return FundsProducerContentBindingV2{}, err
		}
		return FundsProducerContentBindingV2{
			Contract:                  producerV1.Contract,
			ID:                        DeriveFundsProducerContentIDV1(producerV1),
			ManifestSHA256:            SHA256Hex(body),
			ManifestByteLength:        uint64(len(body)),
			CaseID:                    producerV1.CaseID,
			RawArtifactManifestSHA256: producerV1.RawManifestSHA256,
			NormalizedContentSHA256:   producerV1.NormalizedContentSHA256,
			DetailContentSHA256:       producerV1.DetailContentSHA256,
			SourceRowCount:            producerV1.NormalizedRowCount,
			AcceptedRowCount:          producerV1.AcceptedRowCount,
			RejectedRowCount:          producerV1.RejectedRowCount,
			DuplicateRowCount:         producerV1.DuplicateRowCount,
			DetailRowCount:            producerV1.DetailRowCount,
		}, nil
	case FundsProducerContentManifestContractV2:
		if producerV1 != (FundsProducerContentManifestV1{}) ||
			ValidateDatasetSnapshotManifestV2FundsProducerContentV2(manifest, producerV2) != nil {
			return FundsProducerContentBindingV2{}, errors.New("dataset snapshot producer content v2 binding is invalid")
		}
		body, err := FundsProducerContentManifestV2Bytes(producerV2)
		if err != nil {
			return FundsProducerContentBindingV2{}, err
		}
		return FundsProducerContentBindingV2{
			Contract:                  producerV2.Contract,
			ID:                        DeriveFundsProducerContentIDV2(producerV2),
			ManifestSHA256:            SHA256Hex(body),
			ManifestByteLength:        uint64(len(body)),
			CaseID:                    producerV2.CaseID,
			RawArtifactManifestSHA256: producerV2.RawArtifactManifestSHA256,
			NormalizedContentSHA256:   producerV2.NormalizedContentSHA256,
			DetailContentSHA256:       producerV2.DetailContentSHA256,
			SourceRowCount:            producerV2.SourceRowCount,
			AcceptedRowCount:          producerV2.AcceptedRowCount,
			RejectedRowCount:          producerV2.RejectedRowCount,
			DuplicateRowCount:         producerV2.DuplicateRowCount,
			DetailRowCount:            producerV2.DetailRowCount,
		}, nil
	default:
		return FundsProducerContentBindingV2{}, errors.New("dataset snapshot producer content contract is unsupported")
	}
}
