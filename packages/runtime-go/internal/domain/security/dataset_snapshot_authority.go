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
	"time"
	"unicode/utf8"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	DatasetSnapshotAuthorityRecordSchemaVersion = 1
	DatasetSnapshotAuthorityRecordPurpose       = "analytix.dataset-snapshot-authority/v1"
	DatasetSnapshotAuthorityAlgorithm           = "Ed25519"
	DatasetSnapshotIDPrefixV1                   = "dsv1_"
)

var datasetSnapshotAuthoritySignatureDomainV1 = []byte("analytix.dataset-snapshot-authority/v1\x00")
var datasetSnapshotIDDomainV1 = []byte("analytix.dataset-snapshot-id/v1\x00")

// DatasetSnapshotAuthorityRecordV1 binds one immutable, host-accepted dataset
// snapshot to the exact case-binding observation that preceded turn freeze.
// Its signature proves authenticity only. It deliberately carries no
// "current" flag: current selection belongs to an independently maintained
// host authority, never a directory scan, MCP response, provider claim, or
// caller-supplied record.
type DatasetSnapshotAuthorityRecordV1 struct {
	SchemaVersion            int    `json:"schemaVersion"`
	Purpose                  string `json:"purpose"`
	InstallationID           string `json:"installationId"`
	TenantID                 string `json:"tenantId"`
	UserID                   string `json:"userId"`
	WorkspaceRealPath        string `json:"workspaceRealPath"`
	CaseID                   string `json:"caseId"`
	CaseBindingHash          string `json:"caseBindingHash"`
	BindingObservationDigest string `json:"bindingObservationDigest"`
	DatasetSnapshotID        string `json:"datasetSnapshotId"`
	SourceManifestHash       string `json:"sourceManifestHash"`
	RawManifestSHA256        string `json:"rawManifestSHA256"`
	ParserVersion            string `json:"parserVersion"`
	AcceptedAt               string `json:"acceptedAt"`
	PredecessorRecordDigest  string `json:"predecessorRecordDigest"`
	AuthorityAlgorithm       string `json:"authorityAlgorithm"`
	AuthorityKeyID           string `json:"authorityKeyId"`
	AuthorityPublicKey       string `json:"authorityPublicKey"`
	AuthoritySignature       string `json:"authoritySignature"`
	RecordDigest             string `json:"recordDigest"`
}

type DatasetSnapshotAuthorityRecordInputV1 struct {
	InstallationID           string
	TenantID                 string
	UserID                   string
	WorkspaceRealPath        string
	CaseID                   string
	CaseBindingHash          string
	BindingObservationDigest string
	DatasetSnapshotID        string
	SourceManifestHash       string
	RawManifestSHA256        string
	ParserVersion            string
	AcceptedAt               time.Time
	PredecessorRecordDigest  string
	AuthorityKeyID           string
	AuthorityPublicKey       []byte
}

type DatasetSnapshotAuthoritySignFuncV1 func([]byte) ([]byte, error)

func NewDatasetSnapshotAuthorityRecordV1(input DatasetSnapshotAuthorityRecordInputV1, sign DatasetSnapshotAuthoritySignFuncV1) (DatasetSnapshotAuthorityRecordV1, error) {
	acceptedAt := input.AcceptedAt.UTC()
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	record := DatasetSnapshotAuthorityRecordV1{
		SchemaVersion:            DatasetSnapshotAuthorityRecordSchemaVersion,
		Purpose:                  DatasetSnapshotAuthorityRecordPurpose,
		InstallationID:           strings.TrimSpace(input.InstallationID),
		TenantID:                 strings.TrimSpace(input.TenantID),
		UserID:                   strings.TrimSpace(input.UserID),
		WorkspaceRealPath:        strings.TrimSpace(input.WorkspaceRealPath),
		CaseID:                   strings.TrimSpace(input.CaseID),
		CaseBindingHash:          strings.TrimSpace(input.CaseBindingHash),
		BindingObservationDigest: strings.TrimSpace(input.BindingObservationDigest),
		DatasetSnapshotID:        strings.TrimSpace(input.DatasetSnapshotID),
		SourceManifestHash:       strings.TrimSpace(input.SourceManifestHash),
		RawManifestSHA256:        strings.TrimSpace(input.RawManifestSHA256),
		ParserVersion:            strings.TrimSpace(input.ParserVersion),
		PredecessorRecordDigest:  strings.TrimSpace(input.PredecessorRecordDigest),
		AuthorityAlgorithm:       DatasetSnapshotAuthorityAlgorithm,
		AuthorityKeyID:           strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey:       base64.RawURLEncoding.EncodeToString(publicKey),
	}
	if !acceptedAt.IsZero() {
		record.AcceptedAt = acceptedAt.Format(time.RFC3339Nano)
	}
	derivedSnapshotID, deriveErr := deriveDatasetSnapshotIDV1(record)
	if deriveErr != nil || input.DatasetSnapshotID != "" && strings.TrimSpace(input.DatasetSnapshotID) != derivedSnapshotID {
		return DatasetSnapshotAuthorityRecordV1{}, errors.New("dataset snapshot id does not match immutable host material")
	}
	record.DatasetSnapshotID = derivedSnapshotID
	if sign == nil || len(publicKey) != ed25519.PublicKeySize || record.AuthorityKeyID != SHA256Hex(publicKey) {
		return DatasetSnapshotAuthorityRecordV1{}, errors.New("dataset snapshot authority signing authority is invalid")
	}
	if err := validateDatasetSnapshotAuthorityRecordUnsignedV1(record); err != nil {
		return DatasetSnapshotAuthorityRecordV1{}, err
	}
	signature, err := sign(DatasetSnapshotAuthorityRecordSigningBytesV1(record))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return DatasetSnapshotAuthorityRecordV1{}, errors.New("dataset snapshot authority signing failed")
	}
	record.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	record.RecordDigest = datasetSnapshotAuthorityRecordDigestV1(record)
	if err := ValidateDatasetSnapshotAuthorityRecordV1(record); err != nil {
		return DatasetSnapshotAuthorityRecordV1{}, err
	}
	return record, nil
}

func ValidateDatasetSnapshotAuthorityRecordV1(record DatasetSnapshotAuthorityRecordV1) error {
	if err := validateDatasetSnapshotAuthorityRecordUnsignedV1(record); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(record.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(record.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != record.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != record.AuthoritySignature ||
		record.AuthorityKeyID != SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), DatasetSnapshotAuthorityRecordSigningBytesV1(record), signature) {
		return errors.New("dataset snapshot authority signature is invalid")
	}
	if !isCanonicalSHA256Hex(record.RecordDigest) || record.RecordDigest != datasetSnapshotAuthorityRecordDigestV1(record) ||
		record.PredecessorRecordDigest == record.RecordDigest {
		return errors.New("dataset snapshot authority record digest is invalid")
	}
	return nil
}

// ValidateDatasetSnapshotAuthorityRecordForInstallationV1 anchors a
// self-contained signature to this installation's trusted identity and key.
// A well-formed record signed by an attacker-controlled key is not authority.
func ValidateDatasetSnapshotAuthorityRecordForInstallationV1(record DatasetSnapshotAuthorityRecordV1, installationID, authorityKeyID string, authorityPublicKey []byte) error {
	if err := ValidateDatasetSnapshotAuthorityRecordV1(record); err != nil {
		return err
	}
	installationID = strings.TrimSpace(installationID)
	authorityKeyID = strings.TrimSpace(authorityKeyID)
	authorityPublicKey = append([]byte(nil), authorityPublicKey...)
	if record.InstallationID != installationID || len(authorityPublicKey) != ed25519.PublicKeySize ||
		authorityKeyID != SHA256Hex(authorityPublicKey) || record.AuthorityKeyID != authorityKeyID ||
		record.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(authorityPublicKey) {
		return errors.New("dataset snapshot authority installation mismatch")
	}
	return nil
}

// ValidateDatasetSnapshotAuthorityRecordForBindingV1 requires the exact valid
// host observation used when the snapshot was accepted. Matching only a case
// id or a non-empty binding hash is insufficient.
func ValidateDatasetSnapshotAuthorityRecordForBindingV1(record DatasetSnapshotAuthorityRecordV1, observation CaseBindingObservationV1) error {
	if err := ValidateDatasetSnapshotAuthorityRecordV1(record); err != nil {
		return err
	}
	if err := ValidateCaseBindingObservationV1(observation); err != nil || observation.State != CaseBindingStateValid ||
		record.WorkspaceRealPath != observation.WorkspaceRealPath || record.CaseID != observation.CaseID ||
		record.CaseBindingHash != observation.CaseBindingHash || record.BindingObservationDigest != observation.ObservationDigest {
		return errors.New("dataset snapshot authority binding observation mismatch")
	}
	return nil
}

func DatasetSnapshotAuthorityRecordMatchesBindingV1(record DatasetSnapshotAuthorityRecordV1, observation CaseBindingObservationV1) bool {
	return ValidateDatasetSnapshotAuthorityRecordForBindingV1(record, observation) == nil
}

// ValidateDatasetSnapshotAuthorityTransitionV1 validates immutable lineage,
// not freshness. An identical record is an idempotent replay. A distinct
// snapshot for the same exact binding must name the immediately preceding
// record, and an existing snapshot id can never be rebound to other content.
func ValidateDatasetSnapshotAuthorityTransitionV1(previous, next DatasetSnapshotAuthorityRecordV1) error {
	if ValidateDatasetSnapshotAuthorityRecordV1(previous) != nil || ValidateDatasetSnapshotAuthorityRecordV1(next) != nil {
		return errors.New("dataset snapshot authority transition authenticity is invalid")
	}
	if previous.RecordDigest == next.RecordDigest {
		return nil
	}
	if next.InstallationID != previous.InstallationID || next.TenantID != previous.TenantID || next.UserID != previous.UserID ||
		next.WorkspaceRealPath != previous.WorkspaceRealPath || next.CaseID != previous.CaseID ||
		next.CaseBindingHash != previous.CaseBindingHash || next.BindingObservationDigest != previous.BindingObservationDigest ||
		next.AuthorityAlgorithm != previous.AuthorityAlgorithm || next.AuthorityKeyID != previous.AuthorityKeyID ||
		next.AuthorityPublicKey != previous.AuthorityPublicKey || next.PredecessorRecordDigest != previous.RecordDigest {
		return errors.New("dataset snapshot authority transition lineage is invalid")
	}
	previousAcceptedAt, _ := time.Parse(time.RFC3339Nano, previous.AcceptedAt)
	nextAcceptedAt, _ := time.Parse(time.RFC3339Nano, next.AcceptedAt)
	if nextAcceptedAt.Before(previousAcceptedAt) {
		return errors.New("dataset snapshot authority transition time is invalid")
	}
	if next.DatasetSnapshotID == previous.DatasetSnapshotID {
		return errors.New("dataset snapshot id cannot be overwritten")
	}
	if next.SourceManifestHash == previous.SourceManifestHash && next.RawManifestSHA256 == previous.RawManifestSHA256 &&
		next.ParserVersion == previous.ParserVersion {
		return errors.New("dataset snapshot content cannot be assigned another id")
	}
	return nil
}

func ParseDatasetSnapshotAuthorityRecordV1(raw []byte) (DatasetSnapshotAuthorityRecordV1, error) {
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 128 * 1024, MaxDepth: 8, MaxTokens: 256, MaxStringBytes: 32 * 1024,
	}); err != nil {
		return DatasetSnapshotAuthorityRecordV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var record DatasetSnapshotAuthorityRecordV1
	if err := decoder.Decode(&record); err != nil {
		return DatasetSnapshotAuthorityRecordV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return DatasetSnapshotAuthorityRecordV1{}, errors.New("dataset snapshot authority record contains trailing JSON")
	}
	canonical, err := json.Marshal(record)
	if err != nil || !bytes.Equal(raw, canonical) {
		return DatasetSnapshotAuthorityRecordV1{}, errors.New("dataset snapshot authority record is not canonically encoded")
	}
	return record, ValidateDatasetSnapshotAuthorityRecordV1(record)
}

func DatasetSnapshotAuthorityRecordV1Bytes(record DatasetSnapshotAuthorityRecordV1) ([]byte, error) {
	if err := ValidateDatasetSnapshotAuthorityRecordV1(record); err != nil {
		return nil, err
	}
	return json.Marshal(record)
}

func DatasetSnapshotAuthorityRecordSigningBytesV1(record DatasetSnapshotAuthorityRecordV1) []byte {
	record.AuthoritySignature = ""
	record.RecordDigest = ""
	body, _ := json.Marshal(record)
	digest := sha256.Sum256(body)
	out := append([]byte(nil), datasetSnapshotAuthoritySignatureDomainV1...)
	return append(out, digest[:]...)
}

func validateDatasetSnapshotAuthorityRecordUnsignedV1(record DatasetSnapshotAuthorityRecordV1) error {
	acceptedAt, acceptedAtErr := time.Parse(time.RFC3339Nano, record.AcceptedAt)
	if record.SchemaVersion != DatasetSnapshotAuthorityRecordSchemaVersion || record.Purpose != DatasetSnapshotAuthorityRecordPurpose ||
		!isCanonicalSHA256Hex(record.InstallationID) || !canonicalDatasetSnapshotAuthorityText(record.TenantID) ||
		!canonicalDatasetSnapshotAuthorityText(record.UserID) || !canonicalDatasetSnapshotAuthorityText(record.WorkspaceRealPath) ||
		!canonicalDatasetSnapshotAuthorityText(record.CaseID) || record.CaseID == UnboundCaseID ||
		!isCanonicalSHA256Hex(record.CaseBindingHash) || !isCanonicalSHA256Hex(record.BindingObservationDigest) ||
		!validDatasetSnapshotIDV1(record.DatasetSnapshotID) || !isCanonicalSHA256Hex(record.SourceManifestHash) ||
		!isCanonicalSHA256Hex(record.RawManifestSHA256) || !canonicalDatasetSnapshotAuthorityText(record.ParserVersion) ||
		acceptedAtErr != nil || acceptedAt.IsZero() || acceptedAt.UTC().Format(time.RFC3339Nano) != record.AcceptedAt ||
		(record.PredecessorRecordDigest != "" && !isCanonicalSHA256Hex(record.PredecessorRecordDigest)) ||
		record.AuthorityAlgorithm != DatasetSnapshotAuthorityAlgorithm || !isCanonicalSHA256Hex(record.AuthorityKeyID) {
		return errors.New("dataset snapshot authority record is incomplete")
	}
	derivedSnapshotID, err := deriveDatasetSnapshotIDV1(record)
	if err != nil || record.DatasetSnapshotID != derivedSnapshotID {
		return errors.New("dataset snapshot id is not bound to immutable host material")
	}
	return nil
}

type datasetSnapshotIdentityV1 struct {
	SchemaVersion            int    `json:"schemaVersion"`
	Purpose                  string `json:"purpose"`
	InstallationID           string `json:"installationId"`
	TenantID                 string `json:"tenantId"`
	UserID                   string `json:"userId"`
	WorkspaceRealPath        string `json:"workspaceRealPath"`
	CaseID                   string `json:"caseId"`
	CaseBindingHash          string `json:"caseBindingHash"`
	BindingObservationDigest string `json:"bindingObservationDigest"`
	SourceManifestHash       string `json:"sourceManifestHash"`
	RawManifestSHA256        string `json:"rawManifestSHA256"`
	ParserVersion            string `json:"parserVersion"`
}

// DeriveDatasetSnapshotIDV1 gives an immutable dataset snapshot one
// installation- and case-bound content identity. Accepted time, lineage and
// signing material deliberately do not participate: re-accepting identical
// content cannot mint an alias, while a changed binding, source/raw manifest,
// or parser version necessarily produces another id.
func DeriveDatasetSnapshotIDV1(input DatasetSnapshotAuthorityRecordInputV1) (string, error) {
	record := DatasetSnapshotAuthorityRecordV1{
		SchemaVersion:            DatasetSnapshotAuthorityRecordSchemaVersion,
		Purpose:                  DatasetSnapshotAuthorityRecordPurpose,
		InstallationID:           strings.TrimSpace(input.InstallationID),
		TenantID:                 strings.TrimSpace(input.TenantID),
		UserID:                   strings.TrimSpace(input.UserID),
		WorkspaceRealPath:        strings.TrimSpace(input.WorkspaceRealPath),
		CaseID:                   strings.TrimSpace(input.CaseID),
		CaseBindingHash:          strings.TrimSpace(input.CaseBindingHash),
		BindingObservationDigest: strings.TrimSpace(input.BindingObservationDigest),
		SourceManifestHash:       strings.TrimSpace(input.SourceManifestHash),
		RawManifestSHA256:        strings.TrimSpace(input.RawManifestSHA256),
		ParserVersion:            strings.TrimSpace(input.ParserVersion),
	}
	return deriveDatasetSnapshotIDV1(record)
}

func deriveDatasetSnapshotIDV1(record DatasetSnapshotAuthorityRecordV1) (string, error) {
	identity := datasetSnapshotIdentityV1{
		SchemaVersion:  DatasetSnapshotAuthorityRecordSchemaVersion,
		Purpose:        "analytix.dataset-snapshot-id/v1",
		InstallationID: record.InstallationID, TenantID: record.TenantID, UserID: record.UserID,
		WorkspaceRealPath: record.WorkspaceRealPath, CaseID: record.CaseID, CaseBindingHash: record.CaseBindingHash,
		BindingObservationDigest: record.BindingObservationDigest, SourceManifestHash: record.SourceManifestHash,
		RawManifestSHA256: record.RawManifestSHA256, ParserVersion: record.ParserVersion,
	}
	if !isCanonicalSHA256Hex(identity.InstallationID) || !canonicalDatasetSnapshotAuthorityText(identity.TenantID) ||
		!canonicalDatasetSnapshotAuthorityText(identity.UserID) || !canonicalDatasetSnapshotAuthorityText(identity.WorkspaceRealPath) ||
		!canonicalDatasetSnapshotAuthorityText(identity.CaseID) || identity.CaseID == UnboundCaseID ||
		!isCanonicalSHA256Hex(identity.CaseBindingHash) || !isCanonicalSHA256Hex(identity.BindingObservationDigest) ||
		!isCanonicalSHA256Hex(identity.SourceManifestHash) || !isCanonicalSHA256Hex(identity.RawManifestSHA256) ||
		!canonicalDatasetSnapshotAuthorityText(identity.ParserVersion) {
		return "", errors.New("dataset snapshot identity material is incomplete")
	}
	body, err := json.Marshal(identity)
	if err != nil {
		return "", errors.New("dataset snapshot identity encoding failed")
	}
	payload := append([]byte(nil), datasetSnapshotIDDomainV1...)
	return DatasetSnapshotIDPrefixV1 + SHA256Hex(append(payload, body...)), nil
}

func canonicalDatasetSnapshotAuthorityText(value string) bool {
	if value == "" || len(value) > 32*1024 || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func validDatasetSnapshotIDV1(value string) bool {
	return strings.HasPrefix(value, DatasetSnapshotIDPrefixV1) &&
		isCanonicalSHA256Hex(strings.TrimPrefix(value, DatasetSnapshotIDPrefixV1))
}

func datasetSnapshotAuthorityRecordDigestV1(record DatasetSnapshotAuthorityRecordV1) string {
	record.RecordDigest = ""
	body, _ := json.Marshal(record)
	return SHA256Hex(body)
}
