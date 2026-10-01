package security

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
)

const (
	DatasetSnapshotIndexHostLocalSchemaVersionV2 = 2
	DatasetSnapshotIndexHostLocalPurposeV2       = "analytix.dataset-snapshot-index/host-local/v2"
	DatasetSnapshotIndexHostLocalModeV2          = "host_local"
)

var (
	datasetSnapshotIndexHostLocalSignatureDomainV2 = []byte("analytix.dataset-snapshot-index/signature/host-local/v2\x00")
	datasetSnapshotIndexHostLocalDigestDomainV2    = []byte("analytix.dataset-snapshot-index/digest/host-local/v2\x00")
)

// DatasetSnapshotIndexHostLocalInputV2 binds the existing DSV2 record graph
// to a signed host-local mode commitment. It has no witness enrollment.
type DatasetSnapshotIndexHostLocalInputV2 struct {
	InstallationID       string
	ModeCommitmentDigest string
	Generation           uint64
	PreviousIndexDigest  string
	MutationID           string
	Binding              DatasetSnapshotBindingKeyV1
	SnapshotRecordDigest string
	AuthorityKeyID       string
	AuthorityPublicKey   []byte
}

func NewDatasetSnapshotIndexHostLocalV2(input DatasetSnapshotIndexHostLocalInputV2, sign DatasetSnapshotIndexSignFuncV1) (DatasetSnapshotIndexV1, error) {
	index := DatasetSnapshotIndexV1{
		SchemaVersion:        DatasetSnapshotIndexHostLocalSchemaVersionV2,
		Purpose:              DatasetSnapshotIndexHostLocalPurposeV2,
		InstallationID:       input.InstallationID,
		Mode:                 DatasetSnapshotIndexHostLocalModeV2,
		ModeCommitmentDigest: input.ModeCommitmentDigest,
		Generation:           input.Generation,
		PreviousIndexDigest:  input.PreviousIndexDigest,
		MutationID:           input.MutationID,
		Binding:              input.Binding,
		SnapshotRecordDigest: input.SnapshotRecordDigest,
		AuthorityAlgorithm:   DatasetSnapshotIndexAlgorithm,
		AuthorityKeyID:       input.AuthorityKeyID,
		AuthorityPublicKey:   base64.RawURLEncoding.EncodeToString(input.AuthorityPublicKey),
	}
	if sign == nil || validateDatasetSnapshotIndexHostLocalUnsignedV2(index) != nil {
		return DatasetSnapshotIndexV1{}, errors.New("host-local dataset snapshot index input is invalid")
	}
	signature, err := sign(datasetSnapshotIndexHostLocalSigningBytesV2(index))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return DatasetSnapshotIndexV1{}, errors.New("host-local dataset snapshot index signing failed")
	}
	index.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	index.IndexDigest = datasetSnapshotIndexHostLocalDigestV2(index)
	return index, ValidateDatasetSnapshotIndexHostLocalV2(index)
}

func ValidateDatasetSnapshotIndexHostLocalV2(index DatasetSnapshotIndexV1) error {
	if err := validateDatasetSnapshotIndexHostLocalUnsignedV2(index); err != nil {
		return err
	}
	publicKey, keyErr := base64.RawURLEncoding.DecodeString(index.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(index.AuthoritySignature)
	if keyErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize ||
		len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != index.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != index.AuthoritySignature ||
		index.AuthorityKeyID != SHA256Hex(publicKey) ||
		!ed25519.Verify(publicKey, datasetSnapshotIndexHostLocalSigningBytesV2(index), signature) ||
		!IsSHA256Hex(index.IndexDigest) || index.IndexDigest != datasetSnapshotIndexHostLocalDigestV2(index) ||
		index.IndexDigest == index.PreviousIndexDigest || index.IndexDigest == index.SnapshotRecordDigest {
		return errors.New("host-local dataset snapshot index signature or digest is invalid")
	}
	return nil
}

func ValidateDatasetSnapshotIndexForHostLocalV2(index DatasetSnapshotIndexV1,
	installationID, modeCommitmentDigest, authorityKeyID string, authorityPublicKey []byte) error {
	if ValidateDatasetSnapshotIndexHostLocalV2(index) != nil ||
		index.InstallationID != installationID || index.ModeCommitmentDigest != modeCommitmentDigest ||
		index.AuthorityKeyID != authorityKeyID ||
		index.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(authorityPublicKey) ||
		authorityKeyID != SHA256Hex(authorityPublicKey) {
		return errors.New("host-local dataset snapshot index installation or mode anchor is invalid")
	}
	return nil
}

func ValidateDatasetSnapshotIndexRecordForHostLocalV2(index DatasetSnapshotIndexV1,
	record DatasetSnapshotAuthorityRecordV2, installationID, modeCommitmentDigest, authorityKeyID string,
	authorityPublicKey []byte) error {
	if ValidateDatasetSnapshotIndexForHostLocalV2(index, installationID, modeCommitmentDigest,
		authorityKeyID, authorityPublicKey) != nil ||
		ValidateDatasetSnapshotAuthorityRecordForInstallationV2(record, installationID,
			authorityKeyID, authorityPublicKey) != nil ||
		index.InstallationID != record.InstallationID || index.Binding != record.Binding ||
		index.SnapshotRecordDigest != record.RecordDigest || index.AuthorityAlgorithm != record.AuthorityAlgorithm ||
		index.AuthorityKeyID != record.AuthorityKeyID || index.AuthorityPublicKey != record.AuthorityPublicKey {
		return errors.New("host-local dataset snapshot index does not match its v2 record")
	}
	return nil
}

func ValidateDatasetSnapshotIndexNodeForHostLocalManifestV2(index DatasetSnapshotIndexV1,
	record DatasetSnapshotAuthorityRecordV2, manifest DatasetSnapshotManifestV2,
	producer FundsProducerContentManifestV1, tenantID, userID string,
	observation CaseBindingObservationV1,
	installationID, modeCommitmentDigest, authorityKeyID string,
	authorityPublicKey []byte) error {
	if ValidateDatasetSnapshotIndexRecordForHostLocalV2(index, record, installationID,
		modeCommitmentDigest, authorityKeyID, authorityPublicKey) != nil ||
		ValidateDatasetSnapshotAuthorityRecordForFundsProducerContentV2(record, manifest, producer) != nil ||
		ValidateDatasetSnapshotBindingKeyForObservationV1(index.Binding, tenantID, userID, observation) != nil ||
		ValidateDatasetSnapshotAuthorityRecordForBindingV2(record, tenantID, userID, observation) != nil {
		return errors.New("host-local dataset snapshot node and material binding is invalid")
	}
	return nil
}

func ValidateDatasetSnapshotIndexHostLocalTransitionV2(previous, next DatasetSnapshotIndexV1) error {
	if ValidateDatasetSnapshotIndexHostLocalV2(previous) != nil || ValidateDatasetSnapshotIndexHostLocalV2(next) != nil ||
		previous.Generation == ^uint64(0) || next.Generation != previous.Generation+1 ||
		next.PreviousIndexDigest != previous.IndexDigest || next.InstallationID != previous.InstallationID ||
		next.ModeCommitmentDigest != previous.ModeCommitmentDigest || next.AuthorityKeyID != previous.AuthorityKeyID ||
		next.AuthorityPublicKey != previous.AuthorityPublicKey || next.MutationID == previous.MutationID ||
		next.SnapshotRecordDigest == previous.SnapshotRecordDigest {
		return errors.New("host-local dataset snapshot index transition is invalid")
	}
	return nil
}

func ValidateDatasetSnapshotIndexHostLocalRootV2(index DatasetSnapshotIndexV1, rootDigest string, count uint64) error {
	if ValidateDatasetSnapshotIndexHostLocalV2(index) != nil || count == 0 ||
		index.Generation != count || index.IndexDigest != rootDigest {
		return errors.New("host-local dataset snapshot index does not match selected root")
	}
	return nil
}

func ParseVersionedDatasetSnapshotIndex(body []byte) (DatasetSnapshotIndexV1, error) {
	var index DatasetSnapshotIndexV1
	if err := decodeStrictDatasetSnapshotIndexContractV1(body, &index); err != nil {
		return DatasetSnapshotIndexV1{}, err
	}
	canonical, err := json.Marshal(index)
	if err != nil || !bytes.Equal(body, canonical) {
		return DatasetSnapshotIndexV1{}, errors.New("versioned dataset snapshot index is not canonically encoded")
	}
	if err := ValidateVersionedDatasetSnapshotIndex(index); err != nil {
		return DatasetSnapshotIndexV1{}, err
	}
	return index, nil
}

func ValidateVersionedDatasetSnapshotIndex(index DatasetSnapshotIndexV1) error {
	switch index.SchemaVersion {
	case DatasetSnapshotIndexSchemaVersion:
		return ValidateDatasetSnapshotIndexV1(index)
	case DatasetSnapshotIndexHostLocalSchemaVersionV2:
		return ValidateDatasetSnapshotIndexHostLocalV2(index)
	default:
		return errors.New("dataset snapshot index version is unsupported")
	}
}

func VersionedDatasetSnapshotIndexBytes(index DatasetSnapshotIndexV1) ([]byte, error) {
	if err := ValidateVersionedDatasetSnapshotIndex(index); err != nil {
		return nil, err
	}
	return json.Marshal(index)
}

func datasetSnapshotIndexHostLocalSigningBytesV2(index DatasetSnapshotIndexV1) []byte {
	index.AuthoritySignature, index.IndexDigest = "", ""
	body, _ := json.Marshal(index)
	digest := sha256.Sum256(body)
	return append(append([]byte(nil), datasetSnapshotIndexHostLocalSignatureDomainV2...), digest[:]...)
}

func datasetSnapshotIndexHostLocalDigestV2(index DatasetSnapshotIndexV1) string {
	index.IndexDigest = ""
	body, _ := json.Marshal(index)
	return SHA256Hex(append(append([]byte(nil), datasetSnapshotIndexHostLocalDigestDomainV2...), body...))
}

func validateDatasetSnapshotIndexHostLocalUnsignedV2(index DatasetSnapshotIndexV1) error {
	key, err := base64.RawURLEncoding.DecodeString(index.AuthorityPublicKey)
	if index.SchemaVersion != DatasetSnapshotIndexHostLocalSchemaVersionV2 ||
		index.Purpose != DatasetSnapshotIndexHostLocalPurposeV2 ||
		index.Mode != DatasetSnapshotIndexHostLocalModeV2 || index.EnrollmentID != "" ||
		!IsSHA256Hex(index.InstallationID) || !IsSHA256Hex(index.ModeCommitmentDigest) ||
		index.Generation == 0 || !IsSHA256Hex(index.PreviousIndexDigest) || !IsSHA256Hex(index.MutationID) ||
		ValidateDatasetSnapshotBindingKeyV1(index.Binding) != nil || !IsSHA256Hex(index.SnapshotRecordDigest) ||
		index.AuthorityAlgorithm != DatasetSnapshotIndexAlgorithm || err != nil || len(key) != ed25519.PublicKeySize ||
		base64.RawURLEncoding.EncodeToString(key) != index.AuthorityPublicKey || index.AuthorityKeyID != SHA256Hex(key) ||
		(index.Generation == 1 && index.PreviousIndexDigest != DatasetSnapshotIndexGenesisDigestV1()) ||
		(index.Generation > 1 && index.PreviousIndexDigest == DatasetSnapshotIndexGenesisDigestV1()) {
		return errors.New("host-local dataset snapshot index structure is invalid")
	}
	return nil
}
