package reportpublication

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
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	PublicationIndexSchemaVersion = 1
	PublicationIndexPurpose       = "analytix.publication-index/v1"
	PublicationIndexAlgorithm     = "Ed25519"
)

var (
	publicationIndexSignatureDomainV1 = []byte("analytix.publication-index/signature/v1\x00")
	publicationIndexDigestDomainV1    = []byte("analytix.publication-index/digest/v1\x00")
	publicationIndexGenesisDomainV1   = []byte("analytix.publication-index/genesis/v1\x00")
)

// PublicationIndexV1 is one immutable append selected as current only by the
// shared evidence-authority witness. A locally present node has no delivery
// authority until the witnessed bundle names its IndexDigest and Generation.
type PublicationIndexV1 struct {
	SchemaVersion        int    `json:"schemaVersion"`
	Purpose              string `json:"purpose"`
	InstallationID       string `json:"installationId"`
	EnrollmentID         string `json:"enrollmentId"`
	Generation           uint64 `json:"generation"`
	PreviousIndexDigest  string `json:"previousIndexDigest"`
	MutationID           string `json:"mutationId"`
	ReceiptID            string `json:"receiptId"`
	ReceiptRecordDigest  string `json:"receiptRecordDigest"`
	TargetIdentityDigest string `json:"targetIdentityDigest"`
	AuthorityAlgorithm   string `json:"authorityAlgorithm"`
	AuthorityKeyID       string `json:"authorityKeyId"`
	AuthorityPublicKey   string `json:"authorityPublicKey"`
	AuthoritySignature   string `json:"authoritySignature"`
	IndexDigest          string `json:"indexDigest"`
}

type PublicationIndexInputV1 struct {
	InstallationID       string
	EnrollmentID         string
	Generation           uint64
	PreviousIndexDigest  string
	MutationID           string
	ReceiptID            string
	ReceiptRecordDigest  string
	TargetIdentityDigest string
	AuthorityKeyID       string
	AuthorityPublicKey   []byte
}

type PublicationIndexSignFuncV1 func([]byte) ([]byte, error)

func NewPublicationIndexV1(input PublicationIndexInputV1, sign PublicationIndexSignFuncV1) (PublicationIndexV1, error) {
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	index := PublicationIndexV1{
		SchemaVersion: PublicationIndexSchemaVersion, Purpose: PublicationIndexPurpose,
		InstallationID: strings.TrimSpace(input.InstallationID), EnrollmentID: strings.TrimSpace(input.EnrollmentID),
		Generation: input.Generation, PreviousIndexDigest: strings.TrimSpace(input.PreviousIndexDigest), MutationID: strings.TrimSpace(input.MutationID),
		ReceiptID: strings.TrimSpace(input.ReceiptID), ReceiptRecordDigest: strings.TrimSpace(input.ReceiptRecordDigest),
		TargetIdentityDigest: strings.TrimSpace(input.TargetIdentityDigest), AuthorityAlgorithm: PublicationIndexAlgorithm,
		AuthorityKeyID: strings.TrimSpace(input.AuthorityKeyID), AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	if sign == nil || len(publicKey) != ed25519.PublicKeySize || index.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) {
		return PublicationIndexV1{}, errors.New("publication index signing authority is invalid")
	}
	if err := validatePublicationIndexUnsignedV1(index); err != nil {
		return PublicationIndexV1{}, err
	}
	signature, err := sign(PublicationIndexSigningBytesV1(index))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return PublicationIndexV1{}, errors.New("publication index signing failed")
	}
	index.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	index.IndexDigest = publicationIndexDigestV1(index)
	if err := ValidatePublicationIndexV1(index); err != nil {
		return PublicationIndexV1{}, err
	}
	return index, nil
}

func ValidatePublicationIndexV1(index PublicationIndexV1) error {
	if err := validatePublicationIndexUnsignedV1(index); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(index.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(index.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != index.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != index.AuthoritySignature ||
		index.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), PublicationIndexSigningBytesV1(index), signature) {
		return errors.New("publication index signature is invalid")
	}
	if !domainsecurity.IsSHA256Hex(index.IndexDigest) || index.IndexDigest != publicationIndexDigestV1(index) ||
		index.IndexDigest == index.PreviousIndexDigest || index.IndexDigest == index.ReceiptRecordDigest || index.IndexDigest == index.TargetIdentityDigest {
		return errors.New("publication index digest is invalid")
	}
	return nil
}

func ValidatePublicationIndexForInstallationV1(index PublicationIndexV1, installationID, enrollmentID, authorityKeyID string, authorityPublicKey []byte) error {
	if err := ValidatePublicationIndexV1(index); err != nil {
		return err
	}
	authorityPublicKey = append([]byte(nil), authorityPublicKey...)
	if index.InstallationID != strings.TrimSpace(installationID) || index.EnrollmentID != strings.TrimSpace(enrollmentID) ||
		len(authorityPublicKey) != ed25519.PublicKeySize || strings.TrimSpace(authorityKeyID) != domainsecurity.SHA256Hex(authorityPublicKey) ||
		index.AuthorityKeyID != strings.TrimSpace(authorityKeyID) || index.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(authorityPublicKey) {
		return errors.New("publication index installation anchor mismatch")
	}
	return nil
}

func ValidatePublicationIndexTransitionV1(previous, next PublicationIndexV1) error {
	if ValidatePublicationIndexV1(previous) != nil || ValidatePublicationIndexV1(next) != nil {
		return errors.New("publication index transition authenticity is invalid")
	}
	if previous.Generation == ^uint64(0) || next.Generation != previous.Generation+1 || next.PreviousIndexDigest != previous.IndexDigest ||
		next.InstallationID != previous.InstallationID || next.EnrollmentID != previous.EnrollmentID ||
		next.AuthorityAlgorithm != previous.AuthorityAlgorithm || next.AuthorityKeyID != previous.AuthorityKeyID ||
		next.AuthorityPublicKey != previous.AuthorityPublicKey || next.MutationID == previous.MutationID {
		return errors.New("publication index transition lineage is invalid")
	}
	if next.ReceiptID == previous.ReceiptID || next.ReceiptRecordDigest == previous.ReceiptRecordDigest || next.TargetIdentityDigest == previous.TargetIdentityDigest {
		return errors.New("publication index cannot replay a receipt or delivery target")
	}
	return nil
}

func ValidatePublicationIndexWitnessRootV1(index PublicationIndexV1, rootDigest string, count uint64) error {
	if ValidatePublicationIndexV1(index) != nil || count == 0 || index.Generation != count || index.IndexDigest != strings.TrimSpace(rootDigest) {
		return errors.New("publication index does not match witnessed root")
	}
	return nil
}

func ParsePublicationIndexV1(body []byte) (PublicationIndexV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 64 << 10, MaxDepth: 4, MaxTokens: 128, MaxStringBytes: 32 << 10,
	}); err != nil {
		return PublicationIndexV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var index PublicationIndexV1
	if err := decoder.Decode(&index); err != nil {
		return PublicationIndexV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return PublicationIndexV1{}, errors.New("publication index contains trailing JSON")
	}
	canonical, err := json.Marshal(index)
	if err != nil || !bytes.Equal(body, canonical) {
		return PublicationIndexV1{}, errors.New("publication index is not canonically encoded")
	}
	return index, ValidatePublicationIndexV1(index)
}

func PublicationIndexV1Bytes(index PublicationIndexV1) ([]byte, error) {
	if err := ValidatePublicationIndexV1(index); err != nil {
		return nil, err
	}
	return json.Marshal(index)
}

func PublicationIndexSigningBytesV1(index PublicationIndexV1) []byte {
	index.AuthoritySignature = ""
	index.IndexDigest = ""
	body, _ := json.Marshal(index)
	digest := sha256.Sum256(body)
	out := append([]byte(nil), publicationIndexSignatureDomainV1...)
	return append(out, digest[:]...)
}

func PublicationIndexGenesisDigestV1() string {
	return domainsecurity.SHA256Hex(publicationIndexGenesisDomainV1)
}

func validatePublicationIndexUnsignedV1(index PublicationIndexV1) error {
	if index.SchemaVersion != PublicationIndexSchemaVersion || index.Purpose != PublicationIndexPurpose ||
		!domainsecurity.IsSHA256Hex(index.InstallationID) || !domainsecurity.IsSHA256Hex(index.EnrollmentID) || index.Generation == 0 ||
		!domainsecurity.IsSHA256Hex(index.PreviousIndexDigest) || !domainsecurity.IsSHA256Hex(index.MutationID) ||
		!domainsecurity.IsSHA256Hex(index.ReceiptID) || !domainsecurity.IsSHA256Hex(index.ReceiptRecordDigest) ||
		!domainsecurity.IsSHA256Hex(index.TargetIdentityDigest) || index.AuthorityAlgorithm != PublicationIndexAlgorithm ||
		!domainsecurity.IsSHA256Hex(index.AuthorityKeyID) ||
		(index.Generation == 1 && index.PreviousIndexDigest != PublicationIndexGenesisDigestV1()) ||
		(index.Generation > 1 && index.PreviousIndexDigest == PublicationIndexGenesisDigestV1()) {
		return errors.New("publication index is incomplete")
	}
	return nil
}

func publicationIndexDigestV1(index PublicationIndexV1) string {
	index.IndexDigest = ""
	body, _ := json.Marshal(index)
	payload := append([]byte(nil), publicationIndexDigestDomainV1...)
	return domainsecurity.SHA256Hex(append(payload, body...))
}
