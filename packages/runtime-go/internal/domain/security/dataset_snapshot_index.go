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

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	DatasetSnapshotIndexSchemaVersion = 1
	DatasetSnapshotIndexPurpose       = "analytix.dataset-snapshot-index/v1"
	DatasetSnapshotIndexAlgorithm     = "Ed25519"
)

var (
	datasetSnapshotBindingKeyDigestDomainV1 = []byte("analytix.dataset-snapshot-binding-key/v1\x00")
	datasetSnapshotIndexSignatureDomainV1   = []byte("analytix.dataset-snapshot-index/signature/v1\x00")
	datasetSnapshotIndexDigestDomainV1      = []byte("analytix.dataset-snapshot-index/digest/v1\x00")
	datasetSnapshotIndexGenesisDomainV1     = []byte("analytix.dataset-snapshot-index/genesis/v1\x00")
)

// DatasetSnapshotBindingKeyV1 names one exact case-binding authority scope.
// BindingKeyDigest is derived from every other field and cannot be selected by
// an MCP server, provider, or caller. Installation and enrollment anchors live
// on DatasetSnapshotIndexV1 so that the key cannot be transplanted by itself.
type DatasetSnapshotBindingKeyV1 struct {
	TenantID                 string `json:"tenantId"`
	UserID                   string `json:"userId"`
	WorkspaceRealPath        string `json:"workspaceRealPath"`
	CaseID                   string `json:"caseId"`
	CaseBindingHash          string `json:"caseBindingHash"`
	BindingObservationDigest string `json:"bindingObservationDigest"`
	BindingKeyDigest         string `json:"bindingKeyDigest"`
}

type DatasetSnapshotBindingKeyInputV1 struct {
	TenantID                 string
	UserID                   string
	WorkspaceRealPath        string
	CaseID                   string
	CaseBindingHash          string
	BindingObservationDigest string
}

// DatasetSnapshotIndexV1 is one constant-size append record. It is not an
// inventory and carries no latest/current assertion. Freshness requires the
// independently witnessed evidence-authority bundle whose dataset child root
// equals IndexDigest.
type DatasetSnapshotIndexV1 struct {
	SchemaVersion        int                         `json:"schemaVersion"`
	Purpose              string                      `json:"purpose"`
	InstallationID       string                      `json:"installationId"`
	EnrollmentID         string                      `json:"enrollmentId"`
	Generation           uint64                      `json:"generation"`
	PreviousIndexDigest  string                      `json:"previousIndexDigest"`
	MutationID           string                      `json:"mutationId"`
	Binding              DatasetSnapshotBindingKeyV1 `json:"binding"`
	SnapshotRecordDigest string                      `json:"snapshotRecordDigest"`
	AuthorityAlgorithm   string                      `json:"authorityAlgorithm"`
	AuthorityKeyID       string                      `json:"authorityKeyId"`
	AuthorityPublicKey   string                      `json:"authorityPublicKey"`
	AuthoritySignature   string                      `json:"authoritySignature"`
	IndexDigest          string                      `json:"indexDigest"`
}

type DatasetSnapshotIndexInputV1 struct {
	InstallationID       string
	EnrollmentID         string
	Generation           uint64
	PreviousIndexDigest  string
	MutationID           string
	Binding              DatasetSnapshotBindingKeyV1
	SnapshotRecordDigest string
	AuthorityKeyID       string
	AuthorityPublicKey   []byte
}

type DatasetSnapshotIndexSignFuncV1 func([]byte) ([]byte, error)

func NewDatasetSnapshotBindingKeyV1(input DatasetSnapshotBindingKeyInputV1) (DatasetSnapshotBindingKeyV1, error) {
	key := DatasetSnapshotBindingKeyV1{
		TenantID:                 strings.TrimSpace(input.TenantID),
		UserID:                   strings.TrimSpace(input.UserID),
		WorkspaceRealPath:        strings.TrimSpace(input.WorkspaceRealPath),
		CaseID:                   strings.TrimSpace(input.CaseID),
		CaseBindingHash:          strings.TrimSpace(input.CaseBindingHash),
		BindingObservationDigest: strings.TrimSpace(input.BindingObservationDigest),
	}
	key.BindingKeyDigest = datasetSnapshotBindingKeyDigestV1(key)
	if err := ValidateDatasetSnapshotBindingKeyV1(key); err != nil {
		return DatasetSnapshotBindingKeyV1{}, err
	}
	return key, nil
}

func DatasetSnapshotBindingKeyFromRecordV1(record DatasetSnapshotAuthorityRecordV1) (DatasetSnapshotBindingKeyV1, error) {
	if err := ValidateDatasetSnapshotAuthorityRecordV1(record); err != nil {
		return DatasetSnapshotBindingKeyV1{}, err
	}
	return NewDatasetSnapshotBindingKeyV1(DatasetSnapshotBindingKeyInputV1{
		TenantID:                 record.TenantID,
		UserID:                   record.UserID,
		WorkspaceRealPath:        record.WorkspaceRealPath,
		CaseID:                   record.CaseID,
		CaseBindingHash:          record.CaseBindingHash,
		BindingObservationDigest: record.BindingObservationDigest,
	})
}

func ValidateDatasetSnapshotBindingKeyV1(key DatasetSnapshotBindingKeyV1) error {
	if !canonicalDatasetSnapshotAuthorityText(key.TenantID) || !canonicalDatasetSnapshotAuthorityText(key.UserID) ||
		!canonicalDatasetSnapshotAuthorityText(key.WorkspaceRealPath) || !canonicalDatasetSnapshotAuthorityText(key.CaseID) ||
		key.CaseID == UnboundCaseID || !isCanonicalSHA256Hex(key.CaseBindingHash) ||
		!isCanonicalSHA256Hex(key.BindingObservationDigest) || !isCanonicalSHA256Hex(key.BindingKeyDigest) ||
		key.BindingKeyDigest != datasetSnapshotBindingKeyDigestV1(key) {
		return errors.New("dataset snapshot binding key is invalid")
	}
	return nil
}

func ValidateDatasetSnapshotBindingKeyForObservationV1(
	key DatasetSnapshotBindingKeyV1,
	tenantID, userID string,
	observation CaseBindingObservationV1,
) error {
	if err := ValidateDatasetSnapshotBindingKeyV1(key); err != nil {
		return err
	}
	if err := ValidateCaseBindingObservationV1(observation); err != nil || observation.State != CaseBindingStateValid ||
		key.TenantID != strings.TrimSpace(tenantID) || key.UserID != strings.TrimSpace(userID) ||
		key.WorkspaceRealPath != observation.WorkspaceRealPath || key.CaseID != observation.CaseID ||
		key.CaseBindingHash != observation.CaseBindingHash || key.BindingObservationDigest != observation.ObservationDigest {
		return errors.New("dataset snapshot binding key observation mismatch")
	}
	return nil
}

func ParseDatasetSnapshotBindingKeyV1(body []byte) (DatasetSnapshotBindingKeyV1, error) {
	var key DatasetSnapshotBindingKeyV1
	if err := decodeStrictDatasetSnapshotIndexContractV1(body, &key); err != nil {
		return DatasetSnapshotBindingKeyV1{}, err
	}
	canonical, err := json.Marshal(key)
	if err != nil || !bytes.Equal(body, canonical) {
		return DatasetSnapshotBindingKeyV1{}, errors.New("dataset snapshot binding key is not canonically encoded")
	}
	return key, ValidateDatasetSnapshotBindingKeyV1(key)
}

func DatasetSnapshotBindingKeyV1Bytes(key DatasetSnapshotBindingKeyV1) ([]byte, error) {
	if err := ValidateDatasetSnapshotBindingKeyV1(key); err != nil {
		return nil, err
	}
	return json.Marshal(key)
}

func NewDatasetSnapshotIndexV1(input DatasetSnapshotIndexInputV1, sign DatasetSnapshotIndexSignFuncV1) (DatasetSnapshotIndexV1, error) {
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	index := DatasetSnapshotIndexV1{
		SchemaVersion:        DatasetSnapshotIndexSchemaVersion,
		Purpose:              DatasetSnapshotIndexPurpose,
		InstallationID:       strings.TrimSpace(input.InstallationID),
		EnrollmentID:         strings.TrimSpace(input.EnrollmentID),
		Generation:           input.Generation,
		PreviousIndexDigest:  strings.TrimSpace(input.PreviousIndexDigest),
		MutationID:           strings.TrimSpace(input.MutationID),
		Binding:              input.Binding,
		SnapshotRecordDigest: strings.TrimSpace(input.SnapshotRecordDigest),
		AuthorityAlgorithm:   DatasetSnapshotIndexAlgorithm,
		AuthorityKeyID:       strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey:   base64.RawURLEncoding.EncodeToString(publicKey),
	}
	if sign == nil || len(publicKey) != ed25519.PublicKeySize || index.AuthorityKeyID != SHA256Hex(publicKey) {
		return DatasetSnapshotIndexV1{}, errors.New("dataset snapshot index signing authority is invalid")
	}
	if err := validateDatasetSnapshotIndexUnsignedV1(index); err != nil {
		return DatasetSnapshotIndexV1{}, err
	}
	signature, err := sign(DatasetSnapshotIndexSigningBytesV1(index))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return DatasetSnapshotIndexV1{}, errors.New("dataset snapshot index signing failed")
	}
	index.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	index.IndexDigest = datasetSnapshotIndexDigestV1(index)
	if err := ValidateDatasetSnapshotIndexV1(index); err != nil {
		return DatasetSnapshotIndexV1{}, err
	}
	return index, nil
}

func ValidateDatasetSnapshotIndexV1(index DatasetSnapshotIndexV1) error {
	if err := validateDatasetSnapshotIndexUnsignedV1(index); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(index.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(index.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != index.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != index.AuthoritySignature ||
		index.AuthorityKeyID != SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), DatasetSnapshotIndexSigningBytesV1(index), signature) {
		return errors.New("dataset snapshot index signature is invalid")
	}
	if !isCanonicalSHA256Hex(index.IndexDigest) || index.IndexDigest != datasetSnapshotIndexDigestV1(index) ||
		index.IndexDigest == index.PreviousIndexDigest || index.IndexDigest == index.SnapshotRecordDigest {
		return errors.New("dataset snapshot index digest is invalid")
	}
	return nil
}

// ValidateDatasetSnapshotIndexForInstallationV1 anchors a self-signed index
// to the trusted installation identity, witness enrollment, and public key.
func ValidateDatasetSnapshotIndexForInstallationV1(
	index DatasetSnapshotIndexV1,
	installationID, enrollmentID, authorityKeyID string,
	authorityPublicKey []byte,
) error {
	if err := ValidateDatasetSnapshotIndexV1(index); err != nil {
		return err
	}
	authorityPublicKey = append([]byte(nil), authorityPublicKey...)
	if index.InstallationID != strings.TrimSpace(installationID) || index.EnrollmentID != strings.TrimSpace(enrollmentID) ||
		len(authorityPublicKey) != ed25519.PublicKeySize || strings.TrimSpace(authorityKeyID) != SHA256Hex(authorityPublicKey) ||
		index.AuthorityKeyID != strings.TrimSpace(authorityKeyID) ||
		index.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(authorityPublicKey) {
		return errors.New("dataset snapshot index installation anchor mismatch")
	}
	return nil
}

// ValidateDatasetSnapshotIndexTransitionV1 proves one immutable append. It
// does not prove that next is current; current selection belongs to the
// monotonic evidence-authority witness.
func ValidateDatasetSnapshotIndexTransitionV1(previous, next DatasetSnapshotIndexV1) error {
	if ValidateDatasetSnapshotIndexV1(previous) != nil || ValidateDatasetSnapshotIndexV1(next) != nil {
		return errors.New("dataset snapshot index transition authenticity is invalid")
	}
	if previous.Generation == ^uint64(0) || next.Generation != previous.Generation+1 ||
		next.PreviousIndexDigest != previous.IndexDigest || next.InstallationID != previous.InstallationID ||
		next.EnrollmentID != previous.EnrollmentID || next.AuthorityAlgorithm != previous.AuthorityAlgorithm ||
		next.AuthorityKeyID != previous.AuthorityKeyID || next.AuthorityPublicKey != previous.AuthorityPublicKey ||
		next.MutationID == previous.MutationID {
		return errors.New("dataset snapshot index transition lineage is invalid")
	}
	if next.SnapshotRecordDigest == previous.SnapshotRecordDigest {
		return errors.New("dataset snapshot index cannot append the same snapshot record twice")
	}
	return nil
}

// ValidateDatasetSnapshotIndexRecordV1 binds the node to the exact signed
// authority record. A digest string alone is insufficient: binding material,
// installation, and signing key must all match.
func ValidateDatasetSnapshotIndexRecordV1(index DatasetSnapshotIndexV1, record DatasetSnapshotAuthorityRecordV1) error {
	if err := ValidateDatasetSnapshotIndexV1(index); err != nil {
		return err
	}
	if err := ValidateDatasetSnapshotAuthorityRecordV1(record); err != nil {
		return err
	}
	binding, err := DatasetSnapshotBindingKeyFromRecordV1(record)
	if err != nil || index.InstallationID != record.InstallationID || index.Binding != binding ||
		index.SnapshotRecordDigest != record.RecordDigest || index.AuthorityAlgorithm != record.AuthorityAlgorithm ||
		index.AuthorityKeyID != record.AuthorityKeyID || index.AuthorityPublicKey != record.AuthorityPublicKey {
		return errors.New("dataset snapshot index does not match authority record")
	}
	return nil
}

func ValidateDatasetSnapshotIndexRecordForInstallationV1(
	index DatasetSnapshotIndexV1,
	record DatasetSnapshotAuthorityRecordV1,
	installationID, enrollmentID, authorityKeyID string,
	authorityPublicKey []byte,
) error {
	if err := ValidateDatasetSnapshotIndexForInstallationV1(index, installationID, enrollmentID, authorityKeyID, authorityPublicKey); err != nil {
		return err
	}
	if err := ValidateDatasetSnapshotAuthorityRecordForInstallationV1(record, installationID, authorityKeyID, authorityPublicKey); err != nil {
		return err
	}
	return ValidateDatasetSnapshotIndexRecordV1(index, record)
}

// ValidateDatasetSnapshotIndexRecordV2 reuses the existing append-only index
// wire format and monotonic dataset child root. V2 must not create a parallel
// current/head mechanism.
func ValidateDatasetSnapshotIndexRecordV2(index DatasetSnapshotIndexV1, record DatasetSnapshotAuthorityRecordV2) error {
	if err := ValidateDatasetSnapshotIndexV1(index); err != nil {
		return err
	}
	if err := ValidateDatasetSnapshotAuthorityRecordV2(record); err != nil {
		return err
	}
	if index.InstallationID != record.InstallationID || index.Binding != record.Binding ||
		index.SnapshotRecordDigest != record.RecordDigest || index.AuthorityAlgorithm != record.AuthorityAlgorithm ||
		index.AuthorityKeyID != record.AuthorityKeyID || index.AuthorityPublicKey != record.AuthorityPublicKey {
		return errors.New("dataset snapshot index does not match v2 authority record")
	}
	return nil
}

func ValidateDatasetSnapshotIndexRecordForInstallationV2(
	index DatasetSnapshotIndexV1,
	record DatasetSnapshotAuthorityRecordV2,
	installationID, enrollmentID, authorityKeyID string,
	authorityPublicKey []byte,
) error {
	if err := ValidateDatasetSnapshotIndexForInstallationV1(index, installationID, enrollmentID, authorityKeyID, authorityPublicKey); err != nil {
		return err
	}
	if err := ValidateDatasetSnapshotAuthorityRecordForInstallationV2(record, installationID, authorityKeyID, authorityPublicKey); err != nil {
		return err
	}
	return ValidateDatasetSnapshotIndexRecordV2(index, record)
}

// ValidateDatasetSnapshotIndexNodeForManifestV2 performs only structural
// binding of canonical manifest bytes to an installation-anchored record,
// exact case observation, and one index node. It does not prove currentness,
// complete ancestry, private-CAS admission, or a fresh monotonic witness and
// therefore cannot authorize facts by itself.
func ValidateDatasetSnapshotIndexNodeForManifestV2(
	index DatasetSnapshotIndexV1,
	record DatasetSnapshotAuthorityRecordV2,
	manifest DatasetSnapshotManifestV2,
	producer FundsProducerContentManifestV1,
	tenantID, userID string,
	observation CaseBindingObservationV1,
	installationID, enrollmentID, authorityKeyID string,
	authorityPublicKey []byte,
) error {
	if err := ValidateDatasetSnapshotIndexRecordForInstallationV2(
		index, record, installationID, enrollmentID, authorityKeyID, authorityPublicKey,
	); err != nil {
		return err
	}
	if err := ValidateDatasetSnapshotAuthorityRecordForFundsProducerContentV2(record, manifest, producer); err != nil {
		return err
	}
	if err := ValidateDatasetSnapshotBindingKeyForObservationV1(index.Binding, tenantID, userID, observation); err != nil {
		return err
	}
	if err := ValidateDatasetSnapshotAuthorityRecordForBindingV2(record, tenantID, userID, observation); err != nil {
		return errors.New("dataset snapshot v2 index node binding mismatch")
	}
	return nil
}

// ValidateDatasetSnapshotIndexNodeForFundsProducerContentManifestV2 is the
// exact Go producer-content counterpart to
// ValidateDatasetSnapshotIndexNodeForManifestV2. It performs structural
// comparison only and grants no currentness or fact authority.
func ValidateDatasetSnapshotIndexNodeForFundsProducerContentManifestV2(
	index DatasetSnapshotIndexV1,
	record DatasetSnapshotAuthorityRecordV2,
	manifest DatasetSnapshotManifestV2,
	producer FundsProducerContentManifestV2,
	tenantID, userID string,
	observation CaseBindingObservationV1,
	installationID, enrollmentID, authorityKeyID string,
	authorityPublicKey []byte,
) error {
	if err := ValidateDatasetSnapshotIndexRecordForInstallationV2(
		index, record, installationID, enrollmentID, authorityKeyID, authorityPublicKey,
	); err != nil {
		return err
	}
	if err := ValidateDatasetSnapshotAuthorityRecordForFundsProducerContentManifestV2(
		record,
		manifest,
		producer,
	); err != nil {
		return err
	}
	if err := ValidateDatasetSnapshotBindingKeyForObservationV1(index.Binding, tenantID, userID, observation); err != nil {
		return err
	}
	if err := ValidateDatasetSnapshotAuthorityRecordForBindingV2(record, tenantID, userID, observation); err != nil {
		return errors.New("dataset snapshot v2 index node binding mismatch")
	}
	return nil
}

// ValidateDatasetSnapshotSelectionV1 is the indivisible consumer-side check
// for one witnessed index node. It binds the installation and witness
// enrollment, exact tenant/user/case observation, immutable record, source
// manifest, parser lineage, and host-derived dataset snapshot id.
func ValidateDatasetSnapshotSelectionV1(
	index DatasetSnapshotIndexV1,
	record DatasetSnapshotAuthorityRecordV1,
	tenantID, userID string,
	observation CaseBindingObservationV1,
	installationID, enrollmentID, authorityKeyID string,
	authorityPublicKey []byte,
) error {
	if err := ValidateDatasetSnapshotIndexRecordForInstallationV1(
		index, record, installationID, enrollmentID, authorityKeyID, authorityPublicKey,
	); err != nil {
		return err
	}
	if err := ValidateDatasetSnapshotBindingKeyForObservationV1(index.Binding, tenantID, userID, observation); err != nil {
		return err
	}
	if record.TenantID != strings.TrimSpace(tenantID) || record.UserID != strings.TrimSpace(userID) ||
		ValidateDatasetSnapshotAuthorityRecordForBindingV1(record, observation) != nil {
		return errors.New("dataset snapshot selection binding mismatch")
	}
	return nil
}

// ValidateDatasetSnapshotIndexWitnessRootV1 binds the immutable node to the
// exact dataset child pair selected by the shared evidence-authority bundle.
func ValidateDatasetSnapshotIndexWitnessRootV1(index DatasetSnapshotIndexV1, rootDigest string, count uint64) error {
	if ValidateDatasetSnapshotIndexV1(index) != nil || count == 0 || index.Generation != count ||
		index.IndexDigest != strings.TrimSpace(rootDigest) {
		return errors.New("dataset snapshot index does not match witnessed root")
	}
	return nil
}

func ParseDatasetSnapshotIndexV1(body []byte) (DatasetSnapshotIndexV1, error) {
	var index DatasetSnapshotIndexV1
	if err := decodeStrictDatasetSnapshotIndexContractV1(body, &index); err != nil {
		return DatasetSnapshotIndexV1{}, err
	}
	canonical, err := json.Marshal(index)
	if err != nil || !bytes.Equal(body, canonical) {
		return DatasetSnapshotIndexV1{}, errors.New("dataset snapshot index is not canonically encoded")
	}
	return index, ValidateDatasetSnapshotIndexV1(index)
}

func DatasetSnapshotIndexV1Bytes(index DatasetSnapshotIndexV1) ([]byte, error) {
	if err := ValidateDatasetSnapshotIndexV1(index); err != nil {
		return nil, err
	}
	return json.Marshal(index)
}

func DatasetSnapshotIndexSigningBytesV1(index DatasetSnapshotIndexV1) []byte {
	index.AuthoritySignature = ""
	index.IndexDigest = ""
	body, _ := json.Marshal(index)
	digest := sha256.Sum256(body)
	out := append([]byte(nil), datasetSnapshotIndexSignatureDomainV1...)
	return append(out, digest[:]...)
}

func DatasetSnapshotIndexGenesisDigestV1() string {
	return SHA256Hex(datasetSnapshotIndexGenesisDomainV1)
}

func validateDatasetSnapshotIndexUnsignedV1(index DatasetSnapshotIndexV1) error {
	if index.SchemaVersion != DatasetSnapshotIndexSchemaVersion || index.Purpose != DatasetSnapshotIndexPurpose ||
		!isCanonicalSHA256Hex(index.InstallationID) || !isCanonicalSHA256Hex(index.EnrollmentID) || index.Generation == 0 ||
		!isCanonicalSHA256Hex(index.PreviousIndexDigest) || !isCanonicalSHA256Hex(index.MutationID) ||
		ValidateDatasetSnapshotBindingKeyV1(index.Binding) != nil || !isCanonicalSHA256Hex(index.SnapshotRecordDigest) ||
		index.AuthorityAlgorithm != DatasetSnapshotIndexAlgorithm || !isCanonicalSHA256Hex(index.AuthorityKeyID) ||
		(index.Generation == 1 && index.PreviousIndexDigest != DatasetSnapshotIndexGenesisDigestV1()) ||
		(index.Generation > 1 && index.PreviousIndexDigest == DatasetSnapshotIndexGenesisDigestV1()) {
		return errors.New("dataset snapshot index is incomplete")
	}
	return nil
}

func datasetSnapshotBindingKeyDigestV1(key DatasetSnapshotBindingKeyV1) string {
	key.BindingKeyDigest = ""
	body, _ := json.Marshal(key)
	payload := append([]byte(nil), datasetSnapshotBindingKeyDigestDomainV1...)
	return SHA256Hex(append(payload, body...))
}

func datasetSnapshotIndexDigestV1(index DatasetSnapshotIndexV1) string {
	index.IndexDigest = ""
	body, _ := json.Marshal(index)
	payload := append([]byte(nil), datasetSnapshotIndexDigestDomainV1...)
	return SHA256Hex(append(payload, body...))
}

func decodeStrictDatasetSnapshotIndexContractV1(body []byte, target any) error {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 64 << 10, MaxDepth: 4, MaxTokens: 128, MaxStringBytes: 32 << 10,
	}); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("dataset snapshot index contract contains trailing JSON")
	}
	return nil
}
