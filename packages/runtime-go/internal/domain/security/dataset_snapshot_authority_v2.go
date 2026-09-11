package security

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	DatasetSnapshotAuthorityRecordSchemaVersionV2 = 2
	DatasetSnapshotAuthorityRecordPurposeV2       = "analytix.dataset-snapshot-authority/v2"
)

var (
	datasetSnapshotAuthoritySignatureDomainV2    = []byte("analytix.dataset-snapshot-authority/signature/v2\x00")
	datasetSnapshotAuthorityRecordDigestDomainV2 = []byte("analytix.dataset-snapshot-authority/record-digest/v2\x00")
)

// DatasetSnapshotAuthorityRecordV2 authenticates exact canonical manifest
// bytes and their case binding. Authenticity is not currentness: only exact
// membership in the existing DatasetSnapshotIndexV1 chain selected by a fresh
// monotonic witness can make this record current.
type DatasetSnapshotAuthorityRecordV2 struct {
	SchemaVersion  int                         `json:"schemaVersion"`
	Purpose        string                      `json:"purpose"`
	InstallationID string                      `json:"installationId"`
	Binding        DatasetSnapshotBindingKeyV1 `json:"binding"`

	DatasetSnapshotID  string `json:"datasetSnapshotId"`
	ManifestDigest     string `json:"manifestDigest"`
	ManifestSHA256     string `json:"manifestSha256"`
	ManifestByteLength uint64 `json:"manifestByteLength"`
	SourceManifestHash string `json:"sourceManifestHash"`

	AcceptedAt              string `json:"acceptedAt"`
	PredecessorRecordDigest string `json:"predecessorRecordDigest"`
	AuthorityAlgorithm      string `json:"authorityAlgorithm"`
	AuthorityKeyID          string `json:"authorityKeyId"`
	AuthorityPublicKey      string `json:"authorityPublicKey"`
	AuthoritySignature      string `json:"authoritySignature"`
	RecordDigest            string `json:"recordDigest"`
}

type datasetSnapshotAuthorityRecordInputV2 struct {
	InstallationID          string
	Manifest                DatasetSnapshotManifestV2
	FundsProducerContent    FundsProducerContentManifestV1
	AcceptedAt              time.Time
	PredecessorRecordDigest string
	AuthorityKeyID          string
	AuthorityPublicKey      []byte
}

type datasetSnapshotAuthoritySignFuncV2 func([]byte) ([]byte, error)

// DatasetSnapshotAuthoritySealedIssueInputV2 contains only host-owned signing
// metadata. The manifest and funds producer content are captured by the
// callback-scoped sealed admission and cannot be supplied to Issue.
type DatasetSnapshotAuthoritySealedIssueInputV2 struct {
	InstallationID          string
	AcceptedAt              time.Time
	PredecessorRecordDigest string
	AuthorityKeyID          string
	AuthorityPublicKey      []byte
}

// DatasetSnapshotAuthoritySealedAdmissionV2 cannot be implemented outside
// this package. Its one-use Issue method is bound to the exact manifest and
// producer content validated before the callback began. The capability is
// inactive after the callback returns and is not evidence or currentness.
type DatasetSnapshotAuthoritySealedAdmissionV2 interface {
	Issue(
		DatasetSnapshotAuthoritySealedIssueInputV2,
		func([]byte) ([]byte, error),
	) (DatasetSnapshotAuthorityRecordV2, error)
	sealedDatasetSnapshotAuthorityAdmissionV2()
}

type datasetSnapshotAuthoritySealedAdmissionV2 struct {
	mu       sync.Mutex
	active   bool
	used     bool
	manifest DatasetSnapshotManifestV2
}

// WithExactFundsProducerAuthorityAdmissionV2 creates a callback-scoped,
// one-use issuer for one exact DSV2/FPC1 pair. The app layer must call this
// only after it has double-read and fully validated all referenced private-CAS
// material. This receiver method deliberately does not accept a signer or
// return a record, avoiding a generic arbitrary-struct minting function.
func (manifest DatasetSnapshotManifestV2) WithExactFundsProducerAuthorityAdmissionV2(
	producer FundsProducerContentManifestV1,
	use func(DatasetSnapshotAuthoritySealedAdmissionV2) error,
) error {
	if use == nil || ValidateDatasetSnapshotManifestV2FundsProducerContentV1(manifest, producer) != nil {
		return errors.New("dataset snapshot authority v2 sealed admission is invalid")
	}
	capability := &datasetSnapshotAuthoritySealedAdmissionV2{
		active: true, manifest: manifest,
	}
	defer capability.close()
	return use(capability)
}

// WithExactFundsProducerContentV2AuthorityAdmissionV2 creates the same
// callback-scoped one-use issuer for an exact DSV2/FPC2 pair. The closed Go
// count-only producer payload must have already been read back and validated
// against the DSV2 raw graph before the issuer exists.
func (manifest DatasetSnapshotManifestV2) WithExactFundsProducerContentV2AuthorityAdmissionV2(
	producer FundsProducerContentManifestV2,
	use func(DatasetSnapshotAuthoritySealedAdmissionV2) error,
) error {
	if use == nil || ValidateDatasetSnapshotManifestV2FundsProducerContentV2(manifest, producer) != nil {
		return errors.New("dataset snapshot authority v2 funds producer content v2 sealed admission is invalid")
	}
	capability := &datasetSnapshotAuthoritySealedAdmissionV2{
		active: true, manifest: manifest,
	}
	defer capability.close()
	return use(capability)
}

func (*datasetSnapshotAuthoritySealedAdmissionV2) sealedDatasetSnapshotAuthorityAdmissionV2() {}

func (capability *datasetSnapshotAuthoritySealedAdmissionV2) Issue(
	input DatasetSnapshotAuthoritySealedIssueInputV2,
	sign func([]byte) ([]byte, error),
) (DatasetSnapshotAuthorityRecordV2, error) {
	if capability == nil {
		return DatasetSnapshotAuthorityRecordV2{}, errors.New("dataset snapshot authority v2 sealed admission is inactive")
	}
	capability.mu.Lock()
	defer capability.mu.Unlock()
	if !capability.active || capability.used {
		return DatasetSnapshotAuthorityRecordV2{}, errors.New("dataset snapshot authority v2 sealed admission is inactive")
	}
	capability.used = true
	return newDatasetSnapshotAuthorityRecordForValidatedManifestV2(datasetSnapshotAuthorityRecordInputV2{
		InstallationID:          strings.TrimSpace(input.InstallationID),
		Manifest:                capability.manifest,
		AcceptedAt:              input.AcceptedAt,
		PredecessorRecordDigest: strings.TrimSpace(input.PredecessorRecordDigest),
		AuthorityKeyID:          strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey:      append([]byte(nil), input.AuthorityPublicKey...),
	}, datasetSnapshotAuthoritySignFuncV2(sign))
}

func (capability *datasetSnapshotAuthoritySealedAdmissionV2) close() {
	if capability == nil {
		return
	}
	capability.mu.Lock()
	capability.active = false
	capability.mu.Unlock()
}

// newDatasetSnapshotAuthorityRecordV2 is deliberately package-private until
// the app layer can supply a sealed admission produced by exact private-CAS
// readback in one callback. Signing an arbitrary structurally valid manifest
// would turn caller-selected lineage strings into host authority.
func newDatasetSnapshotAuthorityRecordV2(
	input datasetSnapshotAuthorityRecordInputV2,
	sign datasetSnapshotAuthoritySignFuncV2,
) (DatasetSnapshotAuthorityRecordV2, error) {
	if ValidateDatasetSnapshotManifestV2FundsProducerContentV1(input.Manifest, input.FundsProducerContent) != nil {
		return DatasetSnapshotAuthorityRecordV2{}, errors.New("dataset snapshot authority v2 manifest is invalid")
	}
	return newDatasetSnapshotAuthorityRecordForValidatedManifestV2(input, sign)
}

// newDatasetSnapshotAuthorityRecordForValidatedManifestV2 is callable only
// after a package-owned exact-pair validator has produced a sealed one-use
// capability. Keeping it package-private prevents a structurally valid DSV2
// reference from becoming signing authority on its own.
func newDatasetSnapshotAuthorityRecordForValidatedManifestV2(
	input datasetSnapshotAuthorityRecordInputV2,
	sign datasetSnapshotAuthoritySignFuncV2,
) (DatasetSnapshotAuthorityRecordV2, error) {
	if ValidateDatasetSnapshotManifestV2(input.Manifest) != nil {
		return DatasetSnapshotAuthorityRecordV2{}, errors.New("dataset snapshot authority v2 manifest is invalid")
	}
	manifestBody, err := DatasetSnapshotManifestV2Bytes(input.Manifest)
	if err != nil {
		return DatasetSnapshotAuthorityRecordV2{}, errors.New("dataset snapshot authority v2 manifest bytes are invalid")
	}
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	acceptedAt := input.AcceptedAt.UTC()
	record := DatasetSnapshotAuthorityRecordV2{
		SchemaVersion:           DatasetSnapshotAuthorityRecordSchemaVersionV2,
		Purpose:                 DatasetSnapshotAuthorityRecordPurposeV2,
		InstallationID:          strings.TrimSpace(input.InstallationID),
		Binding:                 input.Manifest.Binding,
		DatasetSnapshotID:       DeriveDatasetSnapshotIDV2(input.Manifest),
		ManifestDigest:          input.Manifest.ManifestDigest,
		ManifestSHA256:          SHA256Hex(manifestBody),
		ManifestByteLength:      uint64(len(manifestBody)),
		SourceManifestHash:      input.Manifest.SourceManifestHash,
		PredecessorRecordDigest: strings.TrimSpace(input.PredecessorRecordDigest),
		AuthorityAlgorithm:      DatasetSnapshotAuthorityAlgorithm,
		AuthorityKeyID:          strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey:      base64.RawURLEncoding.EncodeToString(publicKey),
	}
	if !acceptedAt.IsZero() {
		record.AcceptedAt = acceptedAt.Format(time.RFC3339Nano)
	}
	if sign == nil || len(publicKey) != ed25519.PublicKeySize || record.AuthorityKeyID != SHA256Hex(publicKey) {
		return DatasetSnapshotAuthorityRecordV2{}, errors.New("dataset snapshot authority v2 signing authority is invalid")
	}
	if err := validateDatasetSnapshotAuthorityRecordUnsignedV2(record); err != nil {
		return DatasetSnapshotAuthorityRecordV2{}, err
	}
	signature, err := sign(datasetSnapshotAuthorityRecordSigningBytesV2(record))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return DatasetSnapshotAuthorityRecordV2{}, errors.New("dataset snapshot authority v2 signing failed")
	}
	record.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	record.RecordDigest = datasetSnapshotAuthorityRecordDigestV2(record)
	if err := ValidateDatasetSnapshotAuthorityRecordForManifestV2(record, input.Manifest); err != nil {
		return DatasetSnapshotAuthorityRecordV2{}, err
	}
	return record, nil
}

func ValidateDatasetSnapshotAuthorityRecordV2(record DatasetSnapshotAuthorityRecordV2) error {
	if err := validateDatasetSnapshotAuthorityRecordUnsignedV2(record); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(record.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(record.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != record.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != record.AuthoritySignature ||
		record.AuthorityKeyID != SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), datasetSnapshotAuthorityRecordSigningBytesV2(record), signature) {
		return errors.New("dataset snapshot authority v2 signature is invalid")
	}
	if !isCanonicalSHA256Hex(record.RecordDigest) || record.RecordDigest != datasetSnapshotAuthorityRecordDigestV2(record) ||
		record.PredecessorRecordDigest == record.RecordDigest || record.ManifestDigest == record.RecordDigest {
		return errors.New("dataset snapshot authority v2 record digest is invalid")
	}
	return nil
}

func ValidateDatasetSnapshotAuthorityRecordForManifestV2(
	record DatasetSnapshotAuthorityRecordV2,
	manifest DatasetSnapshotManifestV2,
) error {
	if ValidateDatasetSnapshotAuthorityRecordV2(record) != nil || ValidateDatasetSnapshotManifestV2(manifest) != nil {
		return errors.New("dataset snapshot authority v2 record or manifest is invalid")
	}
	body, err := DatasetSnapshotManifestV2Bytes(manifest)
	if err != nil || record.Binding != manifest.Binding || record.DatasetSnapshotID != DeriveDatasetSnapshotIDV2(manifest) ||
		record.ManifestDigest != manifest.ManifestDigest || record.ManifestSHA256 != SHA256Hex(body) ||
		record.ManifestByteLength != uint64(len(body)) || record.SourceManifestHash != manifest.SourceManifestHash {
		return errors.New("dataset snapshot authority v2 manifest binding mismatch")
	}
	return nil
}

// ValidateDatasetSnapshotAuthorityRecordForFundsProducerContentV2 binds the
// signed host manifest to the exact canonical funds producer payload. It does
// not replace private-CAS membership, index membership, or witness freshness.
func ValidateDatasetSnapshotAuthorityRecordForFundsProducerContentV2(
	record DatasetSnapshotAuthorityRecordV2,
	manifest DatasetSnapshotManifestV2,
	producer FundsProducerContentManifestV1,
) error {
	if err := ValidateDatasetSnapshotAuthorityRecordForManifestV2(record, manifest); err != nil {
		return err
	}
	return ValidateDatasetSnapshotManifestV2FundsProducerContentV1(manifest, producer)
}

// ValidateDatasetSnapshotAuthorityRecordForFundsProducerContentManifestV2
// binds the signed host manifest to the exact canonical Go count-only
// producer payload. It does not replace private-CAS membership, index
// membership, or witness freshness.
func ValidateDatasetSnapshotAuthorityRecordForFundsProducerContentManifestV2(
	record DatasetSnapshotAuthorityRecordV2,
	manifest DatasetSnapshotManifestV2,
	producer FundsProducerContentManifestV2,
) error {
	if err := ValidateDatasetSnapshotAuthorityRecordForManifestV2(record, manifest); err != nil {
		return err
	}
	return ValidateDatasetSnapshotManifestV2FundsProducerContentV2(manifest, producer)
}

func ValidateDatasetSnapshotAuthorityRecordForInstallationV2(
	record DatasetSnapshotAuthorityRecordV2,
	installationID, authorityKeyID string,
	authorityPublicKey []byte,
) error {
	if err := ValidateDatasetSnapshotAuthorityRecordV2(record); err != nil {
		return err
	}
	authorityPublicKey = append([]byte(nil), authorityPublicKey...)
	if record.InstallationID != strings.TrimSpace(installationID) || len(authorityPublicKey) != ed25519.PublicKeySize ||
		strings.TrimSpace(authorityKeyID) != SHA256Hex(authorityPublicKey) || record.AuthorityKeyID != strings.TrimSpace(authorityKeyID) ||
		record.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(authorityPublicKey) {
		return errors.New("dataset snapshot authority v2 installation mismatch")
	}
	return nil
}

func ValidateDatasetSnapshotAuthorityRecordForBindingV2(
	record DatasetSnapshotAuthorityRecordV2,
	tenantID, userID string,
	observation CaseBindingObservationV1,
) error {
	if ValidateDatasetSnapshotAuthorityRecordV2(record) != nil ||
		ValidateDatasetSnapshotBindingKeyForObservationV1(record.Binding, tenantID, userID, observation) != nil {
		return errors.New("dataset snapshot authority v2 binding observation mismatch")
	}
	return nil
}

func ValidateDatasetSnapshotAuthorityTransitionV2(
	previous, next DatasetSnapshotAuthorityRecordV2,
) error {
	if ValidateDatasetSnapshotAuthorityRecordV2(previous) != nil || ValidateDatasetSnapshotAuthorityRecordV2(next) != nil {
		return errors.New("dataset snapshot authority v2 transition authenticity is invalid")
	}
	if previous.RecordDigest == next.RecordDigest {
		return nil
	}
	if next.InstallationID != previous.InstallationID || next.Binding != previous.Binding ||
		next.AuthorityAlgorithm != previous.AuthorityAlgorithm || next.AuthorityKeyID != previous.AuthorityKeyID ||
		next.AuthorityPublicKey != previous.AuthorityPublicKey || next.PredecessorRecordDigest != previous.RecordDigest {
		return errors.New("dataset snapshot authority v2 transition lineage is invalid")
	}
	previousAcceptedAt, _ := time.Parse(time.RFC3339Nano, previous.AcceptedAt)
	nextAcceptedAt, _ := time.Parse(time.RFC3339Nano, next.AcceptedAt)
	if nextAcceptedAt.Before(previousAcceptedAt) {
		return errors.New("dataset snapshot authority v2 transition time is invalid")
	}
	if next.DatasetSnapshotID == previous.DatasetSnapshotID || next.ManifestDigest == previous.ManifestDigest ||
		next.ManifestSHA256 == previous.ManifestSHA256 {
		return errors.New("dataset snapshot authority v2 content cannot be reissued")
	}
	return nil
}

func ValidateDatasetSnapshotAuthorityTransitionV1ToV2(
	previous DatasetSnapshotAuthorityRecordV1,
	next DatasetSnapshotAuthorityRecordV2,
) error {
	if ValidateDatasetSnapshotAuthorityRecordV1(previous) != nil || ValidateDatasetSnapshotAuthorityRecordV2(next) != nil {
		return errors.New("dataset snapshot authority v1 to v2 transition authenticity is invalid")
	}
	previousBinding, err := DatasetSnapshotBindingKeyFromRecordV1(previous)
	if err != nil || next.InstallationID != previous.InstallationID || next.Binding != previousBinding ||
		next.AuthorityAlgorithm != previous.AuthorityAlgorithm || next.AuthorityKeyID != previous.AuthorityKeyID ||
		next.AuthorityPublicKey != previous.AuthorityPublicKey || next.PredecessorRecordDigest != previous.RecordDigest {
		return errors.New("dataset snapshot authority v1 to v2 transition lineage is invalid")
	}
	previousAcceptedAt, _ := time.Parse(time.RFC3339Nano, previous.AcceptedAt)
	nextAcceptedAt, _ := time.Parse(time.RFC3339Nano, next.AcceptedAt)
	if nextAcceptedAt.Before(previousAcceptedAt) {
		return errors.New("dataset snapshot authority v1 to v2 transition time is invalid")
	}
	return nil
}

func ParseDatasetSnapshotAuthorityRecordV2(raw []byte) (DatasetSnapshotAuthorityRecordV2, error) {
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 128 * 1024, MaxDepth: 8, MaxTokens: 256, MaxStringBytes: 32 * 1024,
	}); err != nil {
		return DatasetSnapshotAuthorityRecordV2{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var record DatasetSnapshotAuthorityRecordV2
	if err := decoder.Decode(&record); err != nil {
		return DatasetSnapshotAuthorityRecordV2{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return DatasetSnapshotAuthorityRecordV2{}, errors.New("dataset snapshot authority v2 contains trailing JSON")
	}
	canonical, err := json.Marshal(record)
	if err != nil || !bytes.Equal(raw, canonical) {
		return DatasetSnapshotAuthorityRecordV2{}, errors.New("dataset snapshot authority v2 is not canonically encoded")
	}
	return record, ValidateDatasetSnapshotAuthorityRecordV2(record)
}

func DatasetSnapshotAuthorityRecordV2Bytes(record DatasetSnapshotAuthorityRecordV2) ([]byte, error) {
	if err := ValidateDatasetSnapshotAuthorityRecordV2(record); err != nil {
		return nil, err
	}
	return json.Marshal(record)
}

func datasetSnapshotAuthorityRecordSigningBytesV2(record DatasetSnapshotAuthorityRecordV2) []byte {
	record.AuthoritySignature = ""
	record.RecordDigest = ""
	body, _ := json.Marshal(record)
	digest := sha256.Sum256(body)
	return append(append([]byte(nil), datasetSnapshotAuthoritySignatureDomainV2...), digest[:]...)
}

func DeriveDatasetSnapshotIDV2(manifest DatasetSnapshotManifestV2) string {
	if ValidateDatasetSnapshotManifestV2(manifest) != nil {
		return ""
	}
	return DatasetSnapshotIDPrefixV2 + manifest.ManifestDigest
}

func DatasetSnapshotBindingKeyFromRecordV2(record DatasetSnapshotAuthorityRecordV2) (DatasetSnapshotBindingKeyV1, error) {
	if ValidateDatasetSnapshotAuthorityRecordV2(record) != nil {
		return DatasetSnapshotBindingKeyV1{}, errors.New("dataset snapshot authority v2 record is invalid")
	}
	return record.Binding, nil
}

// VersionedDatasetSnapshotAuthorityRecord is an in-memory closed union. The
// CAS body remains the original canonical V1 or V2 record bytes; no wrapper is
// introduced and an unknown or ambiguous version never falls back to V1.
type VersionedDatasetSnapshotAuthorityRecord struct {
	SchemaVersion int
	V1            *DatasetSnapshotAuthorityRecordV1
	V2            *DatasetSnapshotAuthorityRecordV2
}

func ParseVersionedDatasetSnapshotAuthorityRecord(raw []byte) (VersionedDatasetSnapshotAuthorityRecord, error) {
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 128 * 1024, MaxDepth: 8, MaxTokens: 256, MaxStringBytes: 32 * 1024,
	}); err != nil {
		return VersionedDatasetSnapshotAuthorityRecord{}, err
	}
	var header struct {
		SchemaVersion int    `json:"schemaVersion"`
		Purpose       string `json:"purpose"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return VersionedDatasetSnapshotAuthorityRecord{}, err
	}
	switch {
	case header.SchemaVersion == DatasetSnapshotAuthorityRecordSchemaVersion && header.Purpose == DatasetSnapshotAuthorityRecordPurpose:
		record, err := ParseDatasetSnapshotAuthorityRecordV1(raw)
		if err != nil {
			return VersionedDatasetSnapshotAuthorityRecord{}, err
		}
		versioned := VersionedDatasetSnapshotAuthorityRecord{SchemaVersion: header.SchemaVersion, V1: &record}
		return versioned, ValidateVersionedDatasetSnapshotAuthorityRecord(versioned)
	case header.SchemaVersion == DatasetSnapshotAuthorityRecordSchemaVersionV2 && header.Purpose == DatasetSnapshotAuthorityRecordPurposeV2:
		record, err := ParseDatasetSnapshotAuthorityRecordV2(raw)
		if err != nil {
			return VersionedDatasetSnapshotAuthorityRecord{}, err
		}
		versioned := VersionedDatasetSnapshotAuthorityRecord{SchemaVersion: header.SchemaVersion, V2: &record}
		return versioned, ValidateVersionedDatasetSnapshotAuthorityRecord(versioned)
	default:
		return VersionedDatasetSnapshotAuthorityRecord{}, errors.New("dataset snapshot authority record version is unknown")
	}
}

// ValidateVersionedDatasetSnapshotAuthorityRecord enforces the exact closed
// union discriminator. A pointer shape alone is never accepted as a version.
func ValidateVersionedDatasetSnapshotAuthorityRecord(record VersionedDatasetSnapshotAuthorityRecord) error {
	switch record.SchemaVersion {
	case DatasetSnapshotAuthorityRecordSchemaVersion:
		if record.V1 == nil || record.V2 != nil || ValidateDatasetSnapshotAuthorityRecordV1(*record.V1) != nil {
			return errors.New("dataset snapshot authority v1 union is invalid")
		}
	case DatasetSnapshotAuthorityRecordSchemaVersionV2:
		if record.V1 != nil || record.V2 == nil || ValidateDatasetSnapshotAuthorityRecordV2(*record.V2) != nil {
			return errors.New("dataset snapshot authority v2 union is invalid")
		}
	default:
		return errors.New("dataset snapshot authority record version is unknown")
	}
	return nil
}

func ValidateVersionedDatasetSnapshotAuthorityTransition(
	previous, next VersionedDatasetSnapshotAuthorityRecord,
) error {
	if ValidateVersionedDatasetSnapshotAuthorityRecord(previous) != nil ||
		ValidateVersionedDatasetSnapshotAuthorityRecord(next) != nil {
		return errors.New("dataset snapshot authority transition union is invalid")
	}
	switch {
	case previous.SchemaVersion == DatasetSnapshotAuthorityRecordSchemaVersion &&
		next.SchemaVersion == DatasetSnapshotAuthorityRecordSchemaVersion:
		return ValidateDatasetSnapshotAuthorityTransitionV1(*previous.V1, *next.V1)
	case previous.SchemaVersion == DatasetSnapshotAuthorityRecordSchemaVersion &&
		next.SchemaVersion == DatasetSnapshotAuthorityRecordSchemaVersionV2:
		return ValidateDatasetSnapshotAuthorityTransitionV1ToV2(*previous.V1, *next.V2)
	case previous.SchemaVersion == DatasetSnapshotAuthorityRecordSchemaVersionV2 &&
		next.SchemaVersion == DatasetSnapshotAuthorityRecordSchemaVersionV2:
		return ValidateDatasetSnapshotAuthorityTransitionV2(*previous.V2, *next.V2)
	case previous.SchemaVersion == DatasetSnapshotAuthorityRecordSchemaVersionV2 &&
		next.SchemaVersion == DatasetSnapshotAuthorityRecordSchemaVersion:
		return errors.New("dataset snapshot authority v2 to v1 downgrade is forbidden")
	default:
		return errors.New("dataset snapshot authority transition union is invalid")
	}
}

func validateDatasetSnapshotAuthorityRecordUnsignedV2(record DatasetSnapshotAuthorityRecordV2) error {
	acceptedAt, acceptedAtErr := time.Parse(time.RFC3339Nano, record.AcceptedAt)
	if record.SchemaVersion != DatasetSnapshotAuthorityRecordSchemaVersionV2 || record.Purpose != DatasetSnapshotAuthorityRecordPurposeV2 ||
		!isCanonicalSHA256Hex(record.InstallationID) || ValidateDatasetSnapshotBindingKeyV1(record.Binding) != nil ||
		!IsDatasetSnapshotIDV2Syntax(record.DatasetSnapshotID) || record.DatasetSnapshotID != DatasetSnapshotIDPrefixV2+record.ManifestDigest ||
		!isCanonicalSHA256Hex(record.ManifestDigest) || !isCanonicalSHA256Hex(record.ManifestSHA256) ||
		record.ManifestByteLength == 0 || record.ManifestByteLength > maxDatasetSnapshotManifestBytesV2 ||
		!isCanonicalSHA256Hex(record.SourceManifestHash) ||
		acceptedAtErr != nil || acceptedAt.IsZero() || acceptedAt.UTC().Format(time.RFC3339Nano) != record.AcceptedAt ||
		(record.PredecessorRecordDigest != "" && !isCanonicalSHA256Hex(record.PredecessorRecordDigest)) ||
		record.AuthorityAlgorithm != DatasetSnapshotAuthorityAlgorithm || !isCanonicalSHA256Hex(record.AuthorityKeyID) {
		return errors.New("dataset snapshot authority v2 record is incomplete")
	}
	return nil
}

func datasetSnapshotAuthorityRecordDigestV2(record DatasetSnapshotAuthorityRecordV2) string {
	record.RecordDigest = ""
	body, _ := json.Marshal(record)
	return SHA256Hex(append(append([]byte(nil), datasetSnapshotAuthorityRecordDigestDomainV2...), body...))
}
