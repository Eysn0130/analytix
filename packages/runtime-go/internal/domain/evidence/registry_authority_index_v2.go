package evidence

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	EvidenceRegistryAuthorityIndexVersionV2 = 2
	EvidenceRegistryAuthorityIndexPurposeV2 = "analytix.evidence-registry-index/v2"
)

var (
	evidenceRegistryAuthorityIndexSignatureDomainV2 = []byte("analytix.evidence-registry-index/signature/v2\x00")
	evidenceRegistryAuthorityIndexDigestDomainV2    = []byte("analytix.evidence-registry-index/digest/v2\x00")
	evidenceRegistryAuthorityIndexGenesisDomainV2   = []byte("analytix.evidence-registry-index/genesis/v2\x00")
)

// EvidenceRegistryAuthorityIndexV2 is a constant-size global append node.
// It is never locally "current": the shared evidence-authority witness names
// the exact latest IndexDigest and Generation. Consumers traverse immutable
// predecessor nodes to find the newest entry for one frozen turn context.
type EvidenceRegistryAuthorityIndexV2 struct {
	SchemaVersion       int                                 `json:"schemaVersion"`
	Purpose             string                              `json:"purpose"`
	InstallationID      string                              `json:"installationId"`
	EnrollmentID        string                              `json:"enrollmentId"`
	Generation          uint64                              `json:"generation"`
	PreviousIndexDigest string                              `json:"previousIndexDigest"`
	MutationID          string                              `json:"mutationId"`
	Entry               EvidenceRegistryAuthorityIndexEntry `json:"entry"`
	AuthorityAlgorithm  string                              `json:"authorityAlgorithm"`
	AuthorityKeyID      string                              `json:"authorityKeyId"`
	AuthorityPublicKey  string                              `json:"authorityPublicKey"`
	AuthoritySignature  string                              `json:"authoritySignature"`
	IndexDigest         string                              `json:"indexDigest"`
}

type EvidenceRegistryAuthorityIndexInputV2 struct {
	InstallationID      string
	EnrollmentID        string
	Generation          uint64
	PreviousIndexDigest string
	MutationID          string
}

func NewEvidenceRegistryAuthorityIndexV2(
	input EvidenceRegistryAuthorityIndexInputV2,
	capsule EvidenceRegistryAuthorityCapsule,
	keyID string,
	publicKey []byte,
	sign EvidenceRegistryAuthoritySignFunc,
) (EvidenceRegistryAuthorityIndexV2, error) {
	keyID = strings.TrimSpace(keyID)
	publicKey = append([]byte(nil), publicKey...)
	publicKeyEncoded := base64.RawURLEncoding.EncodeToString(publicKey)
	if ValidateEvidenceRegistryAuthorityCapsule(capsule) != nil || sign == nil || !domainsecurity.IsSHA256Hex(keyID) ||
		len(publicKey) != ed25519.PublicKeySize || domainsecurity.SHA256Hex(publicKey) != keyID ||
		capsule.Seal.AuthorityKeyID != keyID || capsule.Seal.AuthorityPublicKey != publicKeyEncoded {
		return EvidenceRegistryAuthorityIndexV2{}, errors.New("evidence registry authority index V2 input is invalid")
	}
	entry, err := NewEvidenceRegistryAuthorityIndexEntry(capsule)
	if err != nil {
		return EvidenceRegistryAuthorityIndexV2{}, err
	}
	index := EvidenceRegistryAuthorityIndexV2{
		SchemaVersion: EvidenceRegistryAuthorityIndexVersionV2, Purpose: EvidenceRegistryAuthorityIndexPurposeV2,
		InstallationID: strings.TrimSpace(input.InstallationID), EnrollmentID: strings.TrimSpace(input.EnrollmentID),
		Generation: input.Generation, PreviousIndexDigest: strings.TrimSpace(input.PreviousIndexDigest), MutationID: strings.TrimSpace(input.MutationID),
		Entry: entry, AuthorityAlgorithm: AcceptedFinalAuthorityAlgorithm, AuthorityKeyID: keyID, AuthorityPublicKey: publicKeyEncoded,
	}
	if err := validateEvidenceRegistryAuthorityIndexUnsignedV2(index); err != nil {
		return EvidenceRegistryAuthorityIndexV2{}, err
	}
	signature, err := sign(EvidenceRegistryAuthorityIndexSigningBytesV2(index))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return EvidenceRegistryAuthorityIndexV2{}, errors.New("evidence registry authority index V2 signing failed")
	}
	index.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	index.IndexDigest = evidenceRegistryAuthorityIndexDigestV2(index)
	if err := ValidateEvidenceRegistryAuthorityIndexV2(index); err != nil {
		return EvidenceRegistryAuthorityIndexV2{}, err
	}
	return index, nil
}

func ValidateEvidenceRegistryAuthorityIndexV2(index EvidenceRegistryAuthorityIndexV2) error {
	if err := validateEvidenceRegistryAuthorityIndexUnsignedV2(index); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(index.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(index.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != index.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != index.AuthoritySignature ||
		index.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), EvidenceRegistryAuthorityIndexSigningBytesV2(index), signature) {
		return errors.New("evidence registry authority index V2 signature is invalid")
	}
	if !domainsecurity.IsSHA256Hex(index.IndexDigest) || index.IndexDigest != evidenceRegistryAuthorityIndexDigestV2(index) ||
		index.IndexDigest == index.PreviousIndexDigest || index.IndexDigest == index.Entry.CapsuleRecordDigest {
		return errors.New("evidence registry authority index V2 digest is invalid")
	}
	return nil
}

func ValidateEvidenceRegistryAuthorityIndexForInstallationV2(
	index EvidenceRegistryAuthorityIndexV2,
	installationID, enrollmentID, authorityKeyID string,
	authorityPublicKey []byte,
) error {
	if err := ValidateEvidenceRegistryAuthorityIndexV2(index); err != nil {
		return err
	}
	authorityPublicKey = append([]byte(nil), authorityPublicKey...)
	if index.InstallationID != strings.TrimSpace(installationID) || index.EnrollmentID != strings.TrimSpace(enrollmentID) ||
		len(authorityPublicKey) != ed25519.PublicKeySize || strings.TrimSpace(authorityKeyID) != domainsecurity.SHA256Hex(authorityPublicKey) ||
		index.AuthorityKeyID != strings.TrimSpace(authorityKeyID) || index.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(authorityPublicKey) {
		return errors.New("evidence registry authority index V2 installation anchor mismatch")
	}
	return nil
}

func ValidateEvidenceRegistryAuthorityIndexTransitionV2(previous, next EvidenceRegistryAuthorityIndexV2) error {
	if ValidateEvidenceRegistryAuthorityIndexV2(previous) != nil || ValidateEvidenceRegistryAuthorityIndexV2(next) != nil {
		return errors.New("evidence registry authority index V2 transition authenticity is invalid")
	}
	if previous.Generation == ^uint64(0) || next.Generation != previous.Generation+1 || next.PreviousIndexDigest != previous.IndexDigest ||
		next.InstallationID != previous.InstallationID || next.EnrollmentID != previous.EnrollmentID ||
		next.AuthorityAlgorithm != previous.AuthorityAlgorithm || next.AuthorityKeyID != previous.AuthorityKeyID ||
		next.AuthorityPublicKey != previous.AuthorityPublicKey || next.MutationID == previous.MutationID || next.Entry == previous.Entry {
		return errors.New("evidence registry authority index V2 transition lineage is invalid")
	}
	return nil
}

func ValidateEvidenceRegistryAuthorityIndexEntryExtensionV2(previous, next EvidenceRegistryAuthorityIndexEntry, capsule EvidenceRegistryAuthorityCapsule) error {
	return validateEvidenceRegistryAuthorityIndexEntryExtension(previous, next, capsule)
}

func EvidenceRegistryAuthorityIndexEntryMatchesCapsuleV2(index EvidenceRegistryAuthorityIndexV2, capsule EvidenceRegistryAuthorityCapsule) bool {
	if ValidateEvidenceRegistryAuthorityIndexV2(index) != nil || ValidateEvidenceRegistryAuthorityCapsule(capsule) != nil ||
		capsule.Seal.AuthorityKeyID != index.AuthorityKeyID || capsule.Seal.AuthorityPublicKey != index.AuthorityPublicKey {
		return false
	}
	expected, err := NewEvidenceRegistryAuthorityIndexEntry(capsule)
	return err == nil && expected == index.Entry
}

func ValidateEvidenceRegistryAuthorityIndexWitnessRootV2(index EvidenceRegistryAuthorityIndexV2, rootDigest string, count uint64) error {
	if ValidateEvidenceRegistryAuthorityIndexV2(index) != nil || count == 0 || index.Generation != count || index.IndexDigest != strings.TrimSpace(rootDigest) {
		return errors.New("evidence registry authority index V2 does not match witnessed root")
	}
	return nil
}

func ParseEvidenceRegistryAuthorityIndexV2(raw []byte) (EvidenceRegistryAuthorityIndexV2, error) {
	var index EvidenceRegistryAuthorityIndexV2
	if err := decodeStrictJSON(raw, &index, "evidence registry authority index V2"); err != nil {
		return EvidenceRegistryAuthorityIndexV2{}, err
	}
	canonical, err := json.Marshal(index)
	if err != nil || !bytes.Equal(raw, canonical) {
		return EvidenceRegistryAuthorityIndexV2{}, errors.New("evidence registry authority index V2 is not canonically encoded")
	}
	return index, ValidateEvidenceRegistryAuthorityIndexV2(index)
}

func EvidenceRegistryAuthorityIndexV2Bytes(index EvidenceRegistryAuthorityIndexV2) ([]byte, error) {
	if err := ValidateEvidenceRegistryAuthorityIndexV2(index); err != nil {
		return nil, err
	}
	return json.Marshal(index)
}

func EvidenceRegistryAuthorityIndexSigningBytesV2(index EvidenceRegistryAuthorityIndexV2) []byte {
	index.AuthoritySignature = ""
	index.IndexDigest = ""
	body, _ := json.Marshal(index)
	digest := sha256.Sum256(body)
	out := append([]byte(nil), evidenceRegistryAuthorityIndexSignatureDomainV2...)
	return append(out, digest[:]...)
}

func EvidenceRegistryAuthorityIndexGenesisDigestV2() string {
	return domainsecurity.SHA256Hex(evidenceRegistryAuthorityIndexGenesisDomainV2)
}

func validateEvidenceRegistryAuthorityIndexUnsignedV2(index EvidenceRegistryAuthorityIndexV2) error {
	if index.SchemaVersion != EvidenceRegistryAuthorityIndexVersionV2 || index.Purpose != EvidenceRegistryAuthorityIndexPurposeV2 ||
		!domainsecurity.IsSHA256Hex(index.InstallationID) || !domainsecurity.IsSHA256Hex(index.EnrollmentID) || index.Generation == 0 ||
		!domainsecurity.IsSHA256Hex(index.PreviousIndexDigest) || !domainsecurity.IsSHA256Hex(index.MutationID) ||
		!validEvidenceRegistryAuthorityIndexEntry(index.Entry) || index.AuthorityAlgorithm != AcceptedFinalAuthorityAlgorithm ||
		!domainsecurity.IsSHA256Hex(index.AuthorityKeyID) ||
		(index.Generation == 1 && index.PreviousIndexDigest != EvidenceRegistryAuthorityIndexGenesisDigestV2()) ||
		(index.Generation > 1 && index.PreviousIndexDigest == EvidenceRegistryAuthorityIndexGenesisDigestV2()) {
		return errors.New("evidence registry authority index V2 is incomplete")
	}
	return nil
}

func validEvidenceRegistryAuthorityIndexEntry(entry EvidenceRegistryAuthorityIndexEntry) bool {
	return strings.TrimSpace(entry.ThreadID) != "" && strings.TrimSpace(entry.TurnID) != "" && domainsecurity.IsSHA256Hex(entry.ContextDigest) &&
		domainsecurity.IsSHA256Hex(entry.ProjectionKey) && entry.ProjectionKey == EvidenceRegistryProjectionKey(entry.ThreadID, entry.TurnID) &&
		domainsecurity.IsSHA256Hex(entry.CapsuleSHA256) && entry.CapsuleByteLength > 0 && domainsecurity.IsSHA256Hex(entry.CapsuleRecordDigest) &&
		entry.RegistrySequence > 0 && domainsecurity.IsSHA256Hex(entry.RegistryStateDigest) && domainsecurity.IsSHA256Hex(entry.LastEntryDigest) &&
		domainsecurity.IsSHA256Hex(entry.CanonicalLedgerSHA256) && entry.CanonicalLedgerByteLength > 0
}

func evidenceRegistryAuthorityIndexDigestV2(index EvidenceRegistryAuthorityIndexV2) string {
	index.IndexDigest = ""
	body, _ := json.Marshal(index)
	payload := append([]byte(nil), evidenceRegistryAuthorityIndexDigestDomainV2...)
	return domainsecurity.SHA256Hex(append(payload, body...))
}
