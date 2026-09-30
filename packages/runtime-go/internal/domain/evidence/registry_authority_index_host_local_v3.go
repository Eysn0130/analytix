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
	EvidenceRegistryAuthorityIndexHostLocalVersionV3 = 3
	EvidenceRegistryAuthorityIndexHostLocalPurposeV3 = "analytix.evidence-registry-index/host-local/v3"
	EvidenceRegistryAuthorityIndexHostLocalModeV3    = "host_local"
)

var (
	evidenceRegistryAuthorityIndexHostLocalSignatureDomainV3 = []byte("analytix.evidence-registry-index/host-local/signature/v3\x00")
	evidenceRegistryAuthorityIndexHostLocalDigestDomainV3    = []byte("analytix.evidence-registry-index/host-local/digest/v3\x00")
)

type EvidenceRegistryAuthorityIndexHostLocalInputV3 struct {
	InstallationID       string
	ModeCommitmentDigest string
	Generation           uint64
	PreviousIndexDigest  string
	MutationID           string
}

// NewEvidenceRegistryAuthorityIndexHostLocalV3 signs a distinct closed child
// grammar. The per-turn capsule and receipt remain the existing exact values.
func NewEvidenceRegistryAuthorityIndexHostLocalV3(
	input EvidenceRegistryAuthorityIndexHostLocalInputV3,
	capsule EvidenceRegistryAuthorityCapsule,
	keyID string,
	publicKey []byte,
	sign EvidenceRegistryAuthoritySignFunc,
) (EvidenceRegistryAuthorityIndexV2, error) {
	if ValidateEvidenceRegistryAuthorityCapsule(capsule) != nil || sign == nil ||
		len(publicKey) != ed25519.PublicKeySize || domainsecurity.SHA256Hex(publicKey) != keyID ||
		capsule.Seal.AuthorityKeyID != keyID ||
		capsule.Seal.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(publicKey) {
		return EvidenceRegistryAuthorityIndexV2{}, errors.New("host-local registry index input is invalid")
	}
	entry, err := NewEvidenceRegistryAuthorityIndexEntry(capsule)
	if err != nil {
		return EvidenceRegistryAuthorityIndexV2{}, err
	}
	index := EvidenceRegistryAuthorityIndexV2{
		SchemaVersion:        EvidenceRegistryAuthorityIndexHostLocalVersionV3,
		Purpose:              EvidenceRegistryAuthorityIndexHostLocalPurposeV3,
		InstallationID:       strings.TrimSpace(input.InstallationID),
		Mode:                 EvidenceRegistryAuthorityIndexHostLocalModeV3,
		ModeCommitmentDigest: strings.TrimSpace(input.ModeCommitmentDigest),
		Generation:           input.Generation, PreviousIndexDigest: strings.TrimSpace(input.PreviousIndexDigest),
		MutationID: strings.TrimSpace(input.MutationID), Entry: entry,
		AuthorityAlgorithm: AcceptedFinalAuthorityAlgorithm, AuthorityKeyID: strings.TrimSpace(keyID),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	if validateEvidenceRegistryAuthorityIndexHostLocalUnsignedV3(index) != nil {
		return EvidenceRegistryAuthorityIndexV2{}, errors.New("host-local registry index unsigned input is invalid")
	}
	signature, err := sign(EvidenceRegistryAuthorityIndexHostLocalSigningBytesV3(index))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return EvidenceRegistryAuthorityIndexV2{}, errors.New("host-local registry index signing failed")
	}
	index.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	index.IndexDigest = evidenceRegistryAuthorityIndexHostLocalDigestV3(index)
	if err := ValidateEvidenceRegistryAuthorityIndexHostLocalV3(index); err != nil {
		return EvidenceRegistryAuthorityIndexV2{}, err
	}
	return index, nil
}

func ValidateEvidenceRegistryAuthorityIndexHostLocalV3(index EvidenceRegistryAuthorityIndexV2) error {
	if err := validateEvidenceRegistryAuthorityIndexHostLocalUnsignedV3(index); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(index.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(index.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize ||
		len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != index.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != index.AuthoritySignature ||
		index.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(publicKey, EvidenceRegistryAuthorityIndexHostLocalSigningBytesV3(index), signature) ||
		index.IndexDigest != evidenceRegistryAuthorityIndexHostLocalDigestV3(index) ||
		index.IndexDigest == index.PreviousIndexDigest || index.IndexDigest == index.Entry.CapsuleRecordDigest {
		return errors.New("host-local registry index signature or digest is invalid")
	}
	return nil
}

func ValidateEvidenceRegistryAuthorityIndexForHostLocalV3(index EvidenceRegistryAuthorityIndexV2,
	installationID, modeCommitmentDigest, authorityKeyID string, authorityPublicKey []byte) error {
	if ValidateEvidenceRegistryAuthorityIndexHostLocalV3(index) != nil ||
		index.InstallationID != installationID || index.ModeCommitmentDigest != modeCommitmentDigest ||
		index.AuthorityKeyID != authorityKeyID ||
		index.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(authorityPublicKey) ||
		authorityKeyID != domainsecurity.SHA256Hex(authorityPublicKey) {
		return errors.New("host-local registry index installation or mode anchor is invalid")
	}
	return nil
}

func ValidateEvidenceRegistryAuthorityIndexHostLocalTransitionV3(previous, next EvidenceRegistryAuthorityIndexV2) error {
	if ValidateEvidenceRegistryAuthorityIndexHostLocalV3(previous) != nil ||
		ValidateEvidenceRegistryAuthorityIndexHostLocalV3(next) != nil ||
		previous.Generation == ^uint64(0) || next.Generation != previous.Generation+1 ||
		next.PreviousIndexDigest != previous.IndexDigest || next.MutationID == previous.MutationID ||
		next.InstallationID != previous.InstallationID || next.ModeCommitmentDigest != previous.ModeCommitmentDigest ||
		next.AuthorityAlgorithm != previous.AuthorityAlgorithm || next.AuthorityKeyID != previous.AuthorityKeyID ||
		next.AuthorityPublicKey != previous.AuthorityPublicKey || next.Entry == previous.Entry {
		return errors.New("host-local registry index transition is invalid")
	}
	return nil
}

func ValidateEvidenceRegistryAuthorityIndexHostLocalRootV3(index EvidenceRegistryAuthorityIndexV2,
	rootDigest string, count uint64) error {
	if ValidateEvidenceRegistryAuthorityIndexHostLocalV3(index) != nil || count == 0 ||
		index.Generation != count || index.IndexDigest != strings.TrimSpace(rootDigest) {
		return errors.New("host-local registry index does not match selected root")
	}
	return nil
}

func EvidenceRegistryAuthorityIndexEntryMatchesCapsuleHostLocalV3(index EvidenceRegistryAuthorityIndexV2,
	capsule EvidenceRegistryAuthorityCapsule) bool {
	if ValidateEvidenceRegistryAuthorityIndexHostLocalV3(index) != nil ||
		ValidateEvidenceRegistryAuthorityCapsule(capsule) != nil ||
		capsule.Seal.AuthorityKeyID != index.AuthorityKeyID ||
		capsule.Seal.AuthorityPublicKey != index.AuthorityPublicKey {
		return false
	}
	expected, err := NewEvidenceRegistryAuthorityIndexEntry(capsule)
	return err == nil && expected == index.Entry
}

func ParseVersionedEvidenceRegistryAuthorityIndex(raw []byte) (EvidenceRegistryAuthorityIndexV2, error) {
	var version struct {
		SchemaVersion int    `json:"schemaVersion"`
		Purpose       string `json:"purpose"`
	}
	if err := json.Unmarshal(raw, &version); err != nil {
		return EvidenceRegistryAuthorityIndexV2{}, err
	}
	switch {
	case version.SchemaVersion == EvidenceRegistryAuthorityIndexVersionV2 &&
		version.Purpose == EvidenceRegistryAuthorityIndexPurposeV2:
		return ParseEvidenceRegistryAuthorityIndexV2(raw)
	case version.SchemaVersion == EvidenceRegistryAuthorityIndexHostLocalVersionV3 &&
		version.Purpose == EvidenceRegistryAuthorityIndexHostLocalPurposeV3:
		var index EvidenceRegistryAuthorityIndexV2
		if err := decodeStrictJSON(raw, &index, "host-local evidence registry index"); err != nil {
			return EvidenceRegistryAuthorityIndexV2{}, err
		}
		canonical, err := json.Marshal(index)
		if err != nil || !bytes.Equal(raw, canonical) {
			return EvidenceRegistryAuthorityIndexV2{}, errors.New("host-local registry index is not canonically encoded")
		}
		return index, ValidateEvidenceRegistryAuthorityIndexHostLocalV3(index)
	default:
		return EvidenceRegistryAuthorityIndexV2{}, errors.New("evidence registry index version or purpose is unsupported")
	}
}

func VersionedEvidenceRegistryAuthorityIndexBytes(index EvidenceRegistryAuthorityIndexV2) ([]byte, error) {
	switch index.SchemaVersion {
	case EvidenceRegistryAuthorityIndexVersionV2:
		return EvidenceRegistryAuthorityIndexV2Bytes(index)
	case EvidenceRegistryAuthorityIndexHostLocalVersionV3:
		if err := ValidateEvidenceRegistryAuthorityIndexHostLocalV3(index); err != nil {
			return nil, err
		}
		return json.Marshal(index)
	default:
		return nil, errors.New("evidence registry index version is unsupported")
	}
}

func EvidenceRegistryAuthorityIndexHostLocalSigningBytesV3(index EvidenceRegistryAuthorityIndexV2) []byte {
	index.AuthoritySignature = ""
	index.IndexDigest = ""
	body, _ := json.Marshal(index)
	digest := sha256.Sum256(body)
	return append(append([]byte(nil), evidenceRegistryAuthorityIndexHostLocalSignatureDomainV3...), digest[:]...)
}

func validateEvidenceRegistryAuthorityIndexHostLocalUnsignedV3(index EvidenceRegistryAuthorityIndexV2) error {
	if index.SchemaVersion != EvidenceRegistryAuthorityIndexHostLocalVersionV3 ||
		index.Purpose != EvidenceRegistryAuthorityIndexHostLocalPurposeV3 ||
		index.Mode != EvidenceRegistryAuthorityIndexHostLocalModeV3 || index.EnrollmentID != "" ||
		!domainsecurity.IsSHA256Hex(index.InstallationID) ||
		!domainsecurity.IsSHA256Hex(index.ModeCommitmentDigest) || index.Generation == 0 ||
		!domainsecurity.IsSHA256Hex(index.PreviousIndexDigest) || !domainsecurity.IsSHA256Hex(index.MutationID) ||
		!validEvidenceRegistryAuthorityIndexEntry(index.Entry) ||
		index.AuthorityAlgorithm != AcceptedFinalAuthorityAlgorithm ||
		!domainsecurity.IsSHA256Hex(index.AuthorityKeyID) ||
		(index.Generation == 1 && index.PreviousIndexDigest != EvidenceRegistryAuthorityIndexGenesisDigestV2()) ||
		(index.Generation > 1 && index.PreviousIndexDigest == EvidenceRegistryAuthorityIndexGenesisDigestV2()) {
		return errors.New("host-local registry index is incomplete")
	}
	return nil
}

func evidenceRegistryAuthorityIndexHostLocalDigestV3(index EvidenceRegistryAuthorityIndexV2) string {
	index.IndexDigest = ""
	body, _ := json.Marshal(index)
	return domainsecurity.SHA256Hex(append(append([]byte(nil),
		evidenceRegistryAuthorityIndexHostLocalDigestDomainV3...), body...))
}
