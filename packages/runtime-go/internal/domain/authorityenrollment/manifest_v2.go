package authorityenrollment

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

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	ManifestSchemaVersionV2 = 2
	ManifestPurposeV2       = "analytix.authority-enrollment-manifest/v2"
)

var (
	manifestSignatureDomainV2 = []byte("analytix.authority-enrollment-manifest/signature/v2\x00")
	manifestDigestDomainV2    = []byte("analytix.authority-enrollment-manifest/digest/v2\x00")
)

// ManifestV2 signs the exact selected credential-profile generation and
// digest. Production credential loading must accept AnchoredManifestV2, then
// verify the canonical profile and every exact/semantic file binding; a V1
// manifest or a caller-constructed projection cannot select credentials.
type ManifestV2 struct {
	SchemaVersion                  int                 `json:"schemaVersion"`
	Purpose                        string              `json:"purpose"`
	InstallationID                 string              `json:"installationId"`
	InstallationAuthorityAlgorithm string              `json:"installationAuthorityAlgorithm"`
	InstallationAuthorityKeyID     string              `json:"installationAuthorityKeyId"`
	InstallationAuthorityPublicKey string              `json:"installationAuthorityPublicKey"`
	CredentialProfileGeneration    uint64              `json:"credentialProfileGeneration"`
	CredentialProfileDigest        string              `json:"credentialProfileDigest"`
	IssuedAt                       time.Time           `json:"issuedAt"`
	ThreadRisk                     WitnessEnrollmentV1 `json:"threadRisk"`
	SharedEvidence                 WitnessEnrollmentV1 `json:"sharedEvidence"`
	InstallationAuthoritySignature string              `json:"installationAuthoritySignature"`
	ManifestDigest                 string              `json:"manifestDigest"`
}

type ManifestInputV2 struct {
	InstallationID                 string
	InstallationAuthorityKeyID     string
	InstallationAuthorityPublicKey []byte
	CredentialProfileGeneration    uint64
	CredentialProfileDigest        string
	IssuedAt                       time.Time
	ThreadRisk                     WitnessEnrollmentInputV1
	SharedEvidence                 WitnessEnrollmentInputV1
}

type ManifestSignFuncV2 func([]byte) ([]byte, error)

type AnchoredManifestV2 struct {
	manifest     ManifestV2
	anchorDigest string
}

type ManifestAnchorProjectionV2 struct {
	ManifestDigest                 string
	InstallationID                 string
	InstallationAuthorityAlgorithm string
	InstallationAuthorityKeyID     string
	InstallationAuthorityPublicKey []byte
	CredentialProfileGeneration    uint64
	CredentialProfileDigest        string
	Enrollment                     WitnessEnrollmentV1
}

func NewManifestV2(input ManifestInputV2, sign ManifestSignFuncV2) (ManifestV2, error) {
	installationPublicKey := append([]byte(nil), input.InstallationAuthorityPublicKey...)
	installationID := strings.TrimSpace(input.InstallationID)
	threadRisk, err := newWitnessEnrollmentV1(installationID, ThreadRiskNamespaceV1, input.ThreadRisk)
	if err != nil {
		return ManifestV2{}, err
	}
	sharedEvidence, err := newWitnessEnrollmentV1(installationID, SharedEvidenceNamespaceV1, input.SharedEvidence)
	if err != nil {
		return ManifestV2{}, err
	}
	manifest := ManifestV2{
		SchemaVersion: ManifestSchemaVersionV2, Purpose: ManifestPurposeV2,
		InstallationID: installationID, InstallationAuthorityAlgorithm: ManifestAlgorithmV1,
		InstallationAuthorityKeyID:     strings.TrimSpace(input.InstallationAuthorityKeyID),
		InstallationAuthorityPublicKey: base64.RawURLEncoding.EncodeToString(installationPublicKey),
		CredentialProfileGeneration:    input.CredentialProfileGeneration,
		CredentialProfileDigest:        strings.TrimSpace(input.CredentialProfileDigest),
		IssuedAt:                       input.IssuedAt.UTC().Round(0), ThreadRisk: threadRisk, SharedEvidence: sharedEvidence,
	}
	if sign == nil || len(installationPublicKey) != ed25519.PublicKeySize ||
		manifest.InstallationAuthorityKeyID != domainsecurity.SHA256Hex(installationPublicKey) {
		return ManifestV2{}, errors.New("authority enrollment V2 installation authority is invalid")
	}
	if err := validateManifestPayloadV2(manifest); err != nil {
		return ManifestV2{}, err
	}
	signature, err := sign(ManifestSigningBytesV2(manifest))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ManifestV2{}, errors.New("authority enrollment V2 manifest signing failed")
	}
	manifest.InstallationAuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	manifest.ManifestDigest = manifestDigestV2(manifest)
	if err := ValidateManifestV2(manifest); err != nil {
		return ManifestV2{}, err
	}
	return manifest, nil
}

func ValidateManifestV2(manifest ManifestV2) error {
	if err := validateManifestPayloadV2(manifest); err != nil {
		return err
	}
	if !isCanonicalSHA256(manifest.ManifestDigest) || manifest.ManifestDigest != manifestDigestV2(manifest) {
		return errors.New("authority enrollment V2 manifest digest is invalid")
	}
	publicKey, publicKeyErr := decodeCanonicalPublicKey(manifest.InstallationAuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(manifest.InstallationAuthoritySignature)
	if publicKeyErr != nil || signatureErr != nil || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(signature) != manifest.InstallationAuthoritySignature ||
		manifest.InstallationAuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), ManifestSigningBytesV2(manifest), signature) {
		return errors.New("authority enrollment V2 manifest signature is invalid")
	}
	return nil
}

func ValidateManifestForInstallationV2(
	manifest ManifestV2,
	installationID, authorityKeyID string,
	authorityPublicKey []byte,
) error {
	if err := ValidateManifestV2(manifest); err != nil {
		return err
	}
	authorityPublicKey = append([]byte(nil), authorityPublicKey...)
	if installationID != strings.TrimSpace(installationID) || authorityKeyID != strings.TrimSpace(authorityKeyID) ||
		!isCanonicalSHA256(installationID) || !isCanonicalSHA256(authorityKeyID) ||
		manifest.InstallationID != installationID || len(authorityPublicKey) != ed25519.PublicKeySize ||
		authorityKeyID != domainsecurity.SHA256Hex(authorityPublicKey) ||
		manifest.InstallationAuthorityKeyID != authorityKeyID ||
		manifest.InstallationAuthorityPublicKey != base64.RawURLEncoding.EncodeToString(authorityPublicKey) {
		return errors.New("authority enrollment V2 installation anchor mismatch")
	}
	return nil
}

func AnchorManifestForInstallationV2(
	manifest ManifestV2,
	installationID, authorityKeyID string,
	authorityPublicKey []byte,
	expectedManifestDigest string,
) (AnchoredManifestV2, error) {
	if err := ValidateManifestForInstallationV2(manifest, installationID, authorityKeyID, authorityPublicKey); err != nil {
		return AnchoredManifestV2{}, err
	}
	if expectedManifestDigest != strings.TrimSpace(expectedManifestDigest) ||
		!isCanonicalSHA256(expectedManifestDigest) || manifest.ManifestDigest != expectedManifestDigest {
		return AnchoredManifestV2{}, errors.New("authority enrollment V2 current manifest anchor mismatch")
	}
	return AnchoredManifestV2{manifest: manifest, anchorDigest: expectedManifestDigest}, nil
}

func ParseAnchoredManifestV2(
	body []byte,
	installationID, authorityKeyID string,
	authorityPublicKey []byte,
	expectedManifestDigest string,
) (AnchoredManifestV2, error) {
	manifest, err := ParseManifestV2(body)
	if err != nil {
		return AnchoredManifestV2{}, err
	}
	return AnchorManifestForInstallationV2(
		manifest, installationID, authorityKeyID, authorityPublicKey, expectedManifestDigest,
	)
}

func ParseManifestV2(body []byte) (ManifestV2, error) {
	if err := validateManifestJSONShapeV2(body); err != nil {
		return ManifestV2{}, err
	}
	var manifest ManifestV2
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&manifest); err != nil {
		return ManifestV2{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ManifestV2{}, errors.New("authority enrollment V2 manifest contains trailing JSON")
	}
	canonical, err := json.Marshal(manifest)
	if err != nil || !bytes.Equal(body, canonical) {
		return ManifestV2{}, errors.New("authority enrollment V2 manifest is not canonically encoded")
	}
	return manifest, ValidateManifestV2(manifest)
}

func ManifestV2Bytes(manifest ManifestV2) ([]byte, error) {
	if err := ValidateManifestV2(manifest); err != nil {
		return nil, err
	}
	return json.Marshal(manifest)
}

func ManifestSigningBytesV2(manifest ManifestV2) []byte {
	payload := manifestPayloadBytesV2(manifest)
	digest := sha256.Sum256(payload)
	out := append([]byte(nil), manifestSignatureDomainV2...)
	return append(out, digest[:]...)
}

func EnrollmentForNamespaceV2(anchored AnchoredManifestV2, namespace string) (WitnessEnrollmentV1, error) {
	manifest := anchored.manifest
	if anchored.anchorDigest == "" || anchored.anchorDigest != manifest.ManifestDigest || ValidateManifestV2(manifest) != nil {
		return WitnessEnrollmentV1{}, errors.New("authority enrollment V2 manifest is not independently anchored")
	}
	switch namespace {
	case ThreadRiskNamespaceV1:
		return manifest.ThreadRisk, nil
	case SharedEvidenceNamespaceV1:
		return manifest.SharedEvidence, nil
	default:
		return WitnessEnrollmentV1{}, errors.New("authority enrollment V2 namespace is unsupported")
	}
}

func ProjectAnchoredManifestForNamespaceV2(
	anchored AnchoredManifestV2,
	namespace string,
) (ManifestAnchorProjectionV2, error) {
	manifest := anchored.manifest
	if anchored.anchorDigest == "" || anchored.anchorDigest != manifest.ManifestDigest || ValidateManifestV2(manifest) != nil {
		return ManifestAnchorProjectionV2{}, errors.New("authority enrollment V2 manifest is not independently anchored")
	}
	enrollment, err := EnrollmentForNamespaceV2(anchored, namespace)
	if err != nil {
		return ManifestAnchorProjectionV2{}, err
	}
	publicKey, err := decodeCanonicalPublicKey(manifest.InstallationAuthorityPublicKey)
	if err != nil {
		return ManifestAnchorProjectionV2{}, errors.New("authority enrollment V2 installation public key is invalid")
	}
	return ManifestAnchorProjectionV2{
		ManifestDigest: manifest.ManifestDigest, InstallationID: manifest.InstallationID,
		InstallationAuthorityAlgorithm: manifest.InstallationAuthorityAlgorithm,
		InstallationAuthorityKeyID:     manifest.InstallationAuthorityKeyID,
		InstallationAuthorityPublicKey: append([]byte(nil), publicKey...),
		CredentialProfileGeneration:    manifest.CredentialProfileGeneration,
		CredentialProfileDigest:        manifest.CredentialProfileDigest, Enrollment: enrollment,
	}, nil
}

func validateManifestPayloadV2(manifest ManifestV2) error {
	if manifest.SchemaVersion != ManifestSchemaVersionV2 || manifest.Purpose != ManifestPurposeV2 ||
		!isCanonicalSHA256(manifest.InstallationID) || manifest.InstallationAuthorityAlgorithm != ManifestAlgorithmV1 ||
		!isCanonicalSHA256(manifest.InstallationAuthorityKeyID) || manifest.CredentialProfileGeneration == 0 ||
		manifest.CredentialProfileGeneration > 1<<53-1 ||
		!isCanonicalSHA256(manifest.CredentialProfileDigest) || manifest.IssuedAt.IsZero() ||
		manifest.IssuedAt.Location() != time.UTC || manifest.IssuedAt.Year() < 1 || manifest.IssuedAt.Year() > 9999 {
		return errors.New("authority enrollment V2 manifest payload is incomplete")
	}
	installationPublicKey, err := decodeCanonicalPublicKey(manifest.InstallationAuthorityPublicKey)
	if err != nil || manifest.InstallationAuthorityKeyID != domainsecurity.SHA256Hex(installationPublicKey) {
		return errors.New("authority enrollment V2 installation public key is invalid")
	}
	if err := validateWitnessEnrollmentV1(manifest.ThreadRisk, manifest.InstallationID, ThreadRiskNamespaceV1); err != nil {
		return err
	}
	if err := validateWitnessEnrollmentV1(manifest.SharedEvidence, manifest.InstallationID, SharedEvidenceNamespaceV1); err != nil {
		return err
	}
	if manifest.ThreadRisk.EnrollmentID == manifest.SharedEvidence.EnrollmentID ||
		manifest.ThreadRisk.WitnessKeyID == manifest.InstallationAuthorityKeyID ||
		manifest.SharedEvidence.WitnessKeyID == manifest.InstallationAuthorityKeyID {
		return errors.New("authority enrollment V2 witness roles or namespace enrollment IDs conflict")
	}
	return nil
}

func manifestPayloadBytesV2(manifest ManifestV2) []byte {
	manifest.InstallationAuthoritySignature = ""
	manifest.ManifestDigest = ""
	body, _ := json.Marshal(manifest)
	return body
}

func manifestDigestV2(manifest ManifestV2) string {
	payload := append([]byte(nil), manifestDigestDomainV2...)
	return domainsecurity.SHA256Hex(append(payload, manifestPayloadBytesV2(manifest)...))
}

func validateManifestJSONShapeV2(body []byte) error {
	object, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 64 << 10, MaxDepth: 8, MaxTokens: 256, MaxStringBytes: 8 << 10,
	})
	if err != nil {
		return err
	}
	required := [...]string{
		"schemaVersion", "purpose", "installationId", "installationAuthorityAlgorithm",
		"installationAuthorityKeyId", "installationAuthorityPublicKey", "credentialProfileGeneration",
		"credentialProfileDigest", "issuedAt", "threadRisk", "sharedEvidence",
		"installationAuthoritySignature", "manifestDigest",
	}
	if err := requireExactFieldsV1(object, required[:], nil, "authority enrollment V2 manifest"); err != nil {
		return err
	}
	if err := validateWitnessEnrollmentJSONShapeV1(object["threadRisk"], "thread risk authority enrollment"); err != nil {
		return err
	}
	return validateWitnessEnrollmentJSONShapeV1(object["sharedEvidence"], "shared evidence authority enrollment")
}
