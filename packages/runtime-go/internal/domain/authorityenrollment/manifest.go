package authorityenrollment

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	ManifestSchemaVersionV1 = 1
	ManifestPurposeV1       = "analytix.authority-enrollment-manifest/v1"
	ManifestAlgorithmV1     = "Ed25519"
	WitnessAlgorithmV1      = "Ed25519"

	ThreadRiskNamespaceV1     = domainsecurity.ThreadRiskAuthorityNamespaceV1
	SharedEvidenceNamespaceV1 = domainsecurity.EvidenceRegistryAuthorityNamespaceV1

	MinWitnessTimeoutMSV1 uint32 = 100
	MaxWitnessTimeoutMSV1 uint32 = 60_000
)

var (
	manifestSignatureDomainV1 = []byte("analytix.authority-enrollment-manifest/signature/v1\x00")
	manifestDigestDomainV1    = []byte("analytix.authority-enrollment-manifest/digest/v1\x00")
)

// WitnessEnrollmentV1 contains only public connection and identity anchors.
// It deliberately has no bearer credential, client private key, or raw client
// certificate field. RootCASHA256 is SHA-256 over one enrolled root
// certificate's Raw DER. The optional mTLS value is SHA-256 over the enrolled
// client leaf certificate's Raw DER, not certificate or private-key material.
type WitnessEnrollmentV1 struct {
	Namespace                           string                                   `json:"namespace"`
	EnrollmentID                        string                                   `json:"enrollmentId"`
	EndpointOrigin                      string                                   `json:"endpointOrigin"`
	WitnessAlgorithm                    string                                   `json:"witnessAlgorithm"`
	WitnessKeyID                        string                                   `json:"witnessKeyId"`
	WitnessPublicKey                    string                                   `json:"witnessPublicKey"`
	RootCASHA256                        string                                   `json:"rootCaSha256"`
	MTLSClientIdentityCertificateSHA256 string                                   `json:"mtlsClientIdentityCertificateSha256,omitempty"`
	ServerName                          string                                   `json:"serverName"`
	TimeoutMS                           uint32                                   `json:"timeoutMs"`
	InitialCheckpoint                   domainsecurity.MonotonicHeadCheckpointV1 `json:"initialCheckpoint"`
}

type WitnessEnrollmentInputV1 struct {
	EnrollmentID                        string
	EndpointOrigin                      string
	WitnessKeyID                        string
	WitnessPublicKey                    []byte
	RootCASHA256                        string
	MTLSClientIdentityCertificateSHA256 string
	ServerName                          string
	TimeoutMS                           uint32
	InitialCheckpoint                   domainsecurity.MonotonicHeadCheckpointV1
}

// ManifestV1 is signed by the installation authority. Self-validation proves
// internal authenticity only; production callers must additionally anchor it
// with ValidateManifestForInstallationV1 rather than trusting the public key
// carried by the manifest itself.
type ManifestV1 struct {
	SchemaVersion                  int                 `json:"schemaVersion"`
	Purpose                        string              `json:"purpose"`
	InstallationID                 string              `json:"installationId"`
	InstallationAuthorityAlgorithm string              `json:"installationAuthorityAlgorithm"`
	InstallationAuthorityKeyID     string              `json:"installationAuthorityKeyId"`
	InstallationAuthorityPublicKey string              `json:"installationAuthorityPublicKey"`
	IssuedAt                       time.Time           `json:"issuedAt"`
	ThreadRisk                     WitnessEnrollmentV1 `json:"threadRisk"`
	SharedEvidence                 WitnessEnrollmentV1 `json:"sharedEvidence"`
	InstallationAuthoritySignature string              `json:"installationAuthoritySignature"`
	ManifestDigest                 string              `json:"manifestDigest"`
}

type ManifestInputV1 struct {
	InstallationID                 string
	InstallationAuthorityKeyID     string
	InstallationAuthorityPublicKey []byte
	IssuedAt                       time.Time
	ThreadRisk                     WitnessEnrollmentInputV1
	SharedEvidence                 WitnessEnrollmentInputV1
}

type ManifestSignFuncV1 func([]byte) ([]byte, error)

// AnchoredManifestV1 is the only capability from which connection enrollment
// may be extracted. Its fields are private so a merely self-signed manifest
// cannot be confused with one bound to an independent installation/current
// manifest anchor.
type AnchoredManifestV1 struct {
	manifest     ManifestV1
	anchorDigest string
}

// ManifestAnchorProjectionV1 is detached comparison data derived from an
// opaque AnchoredManifestV1. It is not itself an anchor capability: consumers
// that mint authority must accept AnchoredManifestV1 and call
// ProjectAnchoredManifestForNamespaceV1 internally rather than accepting a
// caller-constructed projection.
type ManifestAnchorProjectionV1 struct {
	ManifestDigest                 string
	InstallationID                 string
	InstallationAuthorityAlgorithm string
	InstallationAuthorityKeyID     string
	InstallationAuthorityPublicKey []byte
	Enrollment                     WitnessEnrollmentV1
}

func NewManifestV1(input ManifestInputV1, sign ManifestSignFuncV1) (ManifestV1, error) {
	installationPublicKey := append([]byte(nil), input.InstallationAuthorityPublicKey...)
	installationID := strings.TrimSpace(input.InstallationID)
	threadRisk, err := newWitnessEnrollmentV1(installationID, ThreadRiskNamespaceV1, input.ThreadRisk)
	if err != nil {
		return ManifestV1{}, err
	}
	sharedEvidence, err := newWitnessEnrollmentV1(installationID, SharedEvidenceNamespaceV1, input.SharedEvidence)
	if err != nil {
		return ManifestV1{}, err
	}
	manifest := ManifestV1{
		SchemaVersion:                  ManifestSchemaVersionV1,
		Purpose:                        ManifestPurposeV1,
		InstallationID:                 installationID,
		InstallationAuthorityAlgorithm: ManifestAlgorithmV1,
		InstallationAuthorityKeyID:     strings.TrimSpace(input.InstallationAuthorityKeyID),
		InstallationAuthorityPublicKey: base64.RawURLEncoding.EncodeToString(installationPublicKey),
		IssuedAt:                       input.IssuedAt.UTC().Round(0),
		ThreadRisk:                     threadRisk,
		SharedEvidence:                 sharedEvidence,
	}
	if sign == nil || len(installationPublicKey) != ed25519.PublicKeySize ||
		manifest.InstallationAuthorityKeyID != domainsecurity.SHA256Hex(installationPublicKey) {
		return ManifestV1{}, errors.New("authority enrollment installation authority is invalid")
	}
	if err := validateManifestPayloadV1(manifest); err != nil {
		return ManifestV1{}, err
	}
	signature, err := sign(ManifestSigningBytesV1(manifest))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ManifestV1{}, errors.New("authority enrollment manifest signing failed")
	}
	manifest.InstallationAuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	manifest.ManifestDigest = manifestDigestV1(manifest)
	if err := ValidateManifestV1(manifest); err != nil {
		return ManifestV1{}, err
	}
	return manifest, nil
}

func ValidateManifestV1(manifest ManifestV1) error {
	if err := validateManifestPayloadV1(manifest); err != nil {
		return err
	}
	if !isCanonicalSHA256(manifest.ManifestDigest) || manifest.ManifestDigest != manifestDigestV1(manifest) {
		return errors.New("authority enrollment manifest digest is invalid")
	}
	publicKey, publicKeyErr := decodeCanonicalPublicKey(manifest.InstallationAuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(manifest.InstallationAuthoritySignature)
	if publicKeyErr != nil || signatureErr != nil || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(signature) != manifest.InstallationAuthoritySignature ||
		manifest.InstallationAuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), ManifestSigningBytesV1(manifest), signature) {
		return errors.New("authority enrollment manifest signature is invalid")
	}
	return nil
}

// ValidateManifestForInstallationV1 anchors a self-authentic manifest to
// installation identity supplied by a separate trusted configuration path.
func ValidateManifestForInstallationV1(
	manifest ManifestV1,
	installationID, authorityKeyID string,
	authorityPublicKey []byte,
) error {
	if err := ValidateManifestV1(manifest); err != nil {
		return err
	}
	authorityPublicKey = append([]byte(nil), authorityPublicKey...)
	if installationID != strings.TrimSpace(installationID) || authorityKeyID != strings.TrimSpace(authorityKeyID) ||
		!isCanonicalSHA256(installationID) || !isCanonicalSHA256(authorityKeyID) ||
		manifest.InstallationID != installationID ||
		len(authorityPublicKey) != ed25519.PublicKeySize ||
		authorityKeyID != domainsecurity.SHA256Hex(authorityPublicKey) ||
		manifest.InstallationAuthorityKeyID != authorityKeyID ||
		manifest.InstallationAuthorityPublicKey != base64.RawURLEncoding.EncodeToString(authorityPublicKey) {
		return errors.New("authority enrollment installation anchor mismatch")
	}
	return nil
}

// AnchorManifestForInstallationV1 binds a self-authentic manifest to both an
// independently supplied installation identity and the exact independently
// selected current manifest digest. The returned capability, not ManifestV1,
// is required to obtain witness connection parameters.
func AnchorManifestForInstallationV1(
	manifest ManifestV1,
	installationID, authorityKeyID string,
	authorityPublicKey []byte,
	expectedManifestDigest string,
) (AnchoredManifestV1, error) {
	if err := ValidateManifestForInstallationV1(manifest, installationID, authorityKeyID, authorityPublicKey); err != nil {
		return AnchoredManifestV1{}, err
	}
	if expectedManifestDigest != strings.TrimSpace(expectedManifestDigest) ||
		!isCanonicalSHA256(expectedManifestDigest) || manifest.ManifestDigest != expectedManifestDigest {
		return AnchoredManifestV1{}, errors.New("authority enrollment current manifest anchor mismatch")
	}
	return AnchoredManifestV1{manifest: manifest, anchorDigest: expectedManifestDigest}, nil
}

func ParseAnchoredManifestV1(
	body []byte,
	installationID, authorityKeyID string,
	authorityPublicKey []byte,
	expectedManifestDigest string,
) (AnchoredManifestV1, error) {
	manifest, err := ParseManifestV1(body)
	if err != nil {
		return AnchoredManifestV1{}, err
	}
	return AnchorManifestForInstallationV1(
		manifest, installationID, authorityKeyID, authorityPublicKey, expectedManifestDigest,
	)
}

func ParseManifestV1(body []byte) (ManifestV1, error) {
	if err := validateManifestJSONShapeV1(body); err != nil {
		return ManifestV1{}, err
	}
	var manifest ManifestV1
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return ManifestV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ManifestV1{}, errors.New("authority enrollment manifest contains trailing JSON")
	}
	canonical, err := json.Marshal(manifest)
	if err != nil || !bytes.Equal(body, canonical) {
		return ManifestV1{}, errors.New("authority enrollment manifest is not canonically encoded")
	}
	return manifest, ValidateManifestV1(manifest)
}

func ManifestV1Bytes(manifest ManifestV1) ([]byte, error) {
	if err := ValidateManifestV1(manifest); err != nil {
		return nil, err
	}
	return json.Marshal(manifest)
}

// ManifestSigningBytesV1 and ManifestDigest are both derived from the exact
// canonical payload with signature and digest cleared. Neither field is
// self-referential, and the two domain separators prevent cross-protocol use.
func ManifestSigningBytesV1(manifest ManifestV1) []byte {
	payload := manifestPayloadBytesV1(manifest)
	digest := sha256.Sum256(payload)
	out := append([]byte(nil), manifestSignatureDomainV1...)
	return append(out, digest[:]...)
}

// EnrollmentForNamespaceV1 returns only the field dedicated to the requested
// protocol namespace. Unknown aliases and cross-namespace substitution fail.
func EnrollmentForNamespaceV1(anchored AnchoredManifestV1, namespace string) (WitnessEnrollmentV1, error) {
	manifest := anchored.manifest
	if anchored.anchorDigest == "" || anchored.anchorDigest != manifest.ManifestDigest || ValidateManifestV1(manifest) != nil {
		return WitnessEnrollmentV1{}, errors.New("authority enrollment manifest is not independently anchored")
	}
	if err := ValidateManifestV1(manifest); err != nil {
		return WitnessEnrollmentV1{}, err
	}
	switch namespace {
	case ThreadRiskNamespaceV1:
		return manifest.ThreadRisk, nil
	case SharedEvidenceNamespaceV1:
		return manifest.SharedEvidence, nil
	default:
		return WitnessEnrollmentV1{}, errors.New("authority enrollment namespace is unsupported")
	}
}

// ProjectAnchoredManifestForNamespaceV1 exposes the exact selected manifest
// identity and one namespace enrollment only after revalidating the opaque
// anchor. Returned public-key bytes are copied so callers cannot mutate the
// anchored manifest through the projection.
func ProjectAnchoredManifestForNamespaceV1(
	anchored AnchoredManifestV1,
	namespace string,
) (ManifestAnchorProjectionV1, error) {
	manifest := anchored.manifest
	if anchored.anchorDigest == "" || anchored.anchorDigest != manifest.ManifestDigest ||
		ValidateManifestV1(manifest) != nil {
		return ManifestAnchorProjectionV1{}, errors.New("authority enrollment manifest is not independently anchored")
	}
	enrollment, err := EnrollmentForNamespaceV1(anchored, namespace)
	if err != nil {
		return ManifestAnchorProjectionV1{}, err
	}
	publicKey, err := decodeCanonicalPublicKey(manifest.InstallationAuthorityPublicKey)
	if err != nil {
		return ManifestAnchorProjectionV1{}, errors.New("authority enrollment installation public key is invalid")
	}
	return ManifestAnchorProjectionV1{
		ManifestDigest:                 manifest.ManifestDigest,
		InstallationID:                 manifest.InstallationID,
		InstallationAuthorityAlgorithm: manifest.InstallationAuthorityAlgorithm,
		InstallationAuthorityKeyID:     manifest.InstallationAuthorityKeyID,
		InstallationAuthorityPublicKey: append([]byte(nil), publicKey...),
		Enrollment:                     enrollment,
	}, nil
}

func newWitnessEnrollmentV1(installationID, namespace string, input WitnessEnrollmentInputV1) (WitnessEnrollmentV1, error) {
	publicKey := append([]byte(nil), input.WitnessPublicKey...)
	enrollment := WitnessEnrollmentV1{
		Namespace:                           namespace,
		EnrollmentID:                        strings.TrimSpace(input.EnrollmentID),
		EndpointOrigin:                      strings.TrimSpace(input.EndpointOrigin),
		WitnessAlgorithm:                    WitnessAlgorithmV1,
		WitnessKeyID:                        strings.TrimSpace(input.WitnessKeyID),
		WitnessPublicKey:                    base64.RawURLEncoding.EncodeToString(publicKey),
		RootCASHA256:                        strings.TrimSpace(input.RootCASHA256),
		MTLSClientIdentityCertificateSHA256: strings.TrimSpace(input.MTLSClientIdentityCertificateSHA256),
		ServerName:                          strings.TrimSpace(input.ServerName),
		TimeoutMS:                           input.TimeoutMS,
		InitialCheckpoint:                   input.InitialCheckpoint,
	}
	if err := validateWitnessEnrollmentV1(enrollment, installationID, namespace); err != nil {
		return WitnessEnrollmentV1{}, err
	}
	return enrollment, nil
}

func validateManifestPayloadV1(manifest ManifestV1) error {
	if manifest.SchemaVersion != ManifestSchemaVersionV1 || manifest.Purpose != ManifestPurposeV1 ||
		!isCanonicalSHA256(manifest.InstallationID) || manifest.InstallationAuthorityAlgorithm != ManifestAlgorithmV1 ||
		!isCanonicalSHA256(manifest.InstallationAuthorityKeyID) || manifest.IssuedAt.IsZero() ||
		manifest.IssuedAt.Location() != time.UTC || manifest.IssuedAt.Year() < 1 || manifest.IssuedAt.Year() > 9999 {
		return errors.New("authority enrollment manifest payload is incomplete")
	}
	installationPublicKey, err := decodeCanonicalPublicKey(manifest.InstallationAuthorityPublicKey)
	if err != nil || manifest.InstallationAuthorityKeyID != domainsecurity.SHA256Hex(installationPublicKey) {
		return errors.New("authority enrollment installation public key is invalid")
	}
	if err := validateWitnessEnrollmentV1(manifest.ThreadRisk, manifest.InstallationID, ThreadRiskNamespaceV1); err != nil {
		return err
	}
	if err := validateWitnessEnrollmentV1(manifest.SharedEvidence, manifest.InstallationID, SharedEvidenceNamespaceV1); err != nil {
		return err
	}
	if manifest.ThreadRisk.WitnessKeyID == manifest.InstallationAuthorityKeyID ||
		manifest.SharedEvidence.WitnessKeyID == manifest.InstallationAuthorityKeyID {
		return errors.New("authority enrollment witness must be independent from installation authority")
	}
	return nil
}

func validateWitnessEnrollmentV1(enrollment WitnessEnrollmentV1, installationID, expectedNamespace string) error {
	if expectedNamespace != ThreadRiskNamespaceV1 && expectedNamespace != SharedEvidenceNamespaceV1 {
		return errors.New("authority enrollment namespace is unsupported")
	}
	if enrollment.Namespace != expectedNamespace || !isCanonicalSHA256(enrollment.EnrollmentID) ||
		enrollment.WitnessAlgorithm != WitnessAlgorithmV1 || !isCanonicalSHA256(enrollment.WitnessKeyID) ||
		!isCanonicalSHA256(enrollment.RootCASHA256) ||
		(enrollment.MTLSClientIdentityCertificateSHA256 != "" && !isCanonicalSHA256(enrollment.MTLSClientIdentityCertificateSHA256)) ||
		enrollment.TimeoutMS < MinWitnessTimeoutMSV1 || enrollment.TimeoutMS > MaxWitnessTimeoutMSV1 {
		return errors.New("authority witness enrollment is incomplete")
	}
	publicKey, err := decodeCanonicalPublicKey(enrollment.WitnessPublicKey)
	if err != nil || enrollment.WitnessKeyID != domainsecurity.SHA256Hex(publicKey) {
		return errors.New("authority witness public key is invalid")
	}
	host, err := validateHTTPSOriginV1(enrollment.EndpointOrigin)
	if err != nil {
		return err
	}
	if err := validateServerNameV1(enrollment.ServerName); err != nil || enrollment.ServerName != host {
		return errors.New("authority witness server name is invalid")
	}
	if enrollment.InitialCheckpoint.Generation != 0 ||
		domainsecurity.ValidateMonotonicHeadCheckpointForWitnessV1(
			enrollment.InitialCheckpoint, installationID, enrollment.EnrollmentID,
			enrollment.WitnessKeyID, publicKey,
		) != nil || enrollment.InitialCheckpoint.Namespace != expectedNamespace {
		return errors.New("authority witness initial checkpoint is invalid")
	}
	return nil
}

func validateHTTPSOriginV1(origin string) (string, error) {
	if origin == "" || origin != strings.TrimSpace(origin) || strings.Contains(origin, "\\") {
		return "", errors.New("authority witness endpoint origin is invalid")
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "https" || parsed.Opaque != "" || parsed.User != nil || parsed.Host == "" ||
		parsed.Path != "" || parsed.RawPath != "" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" ||
		parsed.String() != origin {
		return "", errors.New("authority witness endpoint must be an HTTPS origin")
	}
	host := parsed.Hostname()
	if err := validateServerNameV1(host); err != nil {
		return "", errors.New("authority witness endpoint host is invalid")
	}
	port := parsed.Port()
	if strings.HasSuffix(parsed.Host, ":") {
		return "", errors.New("authority witness endpoint port is invalid")
	}
	if port != "" {
		value, err := strconv.ParseUint(port, 10, 16)
		if err != nil || value == 0 || strconv.FormatUint(value, 10) != port {
			return "", errors.New("authority witness endpoint port is invalid")
		}
	}
	canonicalHost := host
	if net.ParseIP(host) != nil && strings.Contains(host, ":") {
		canonicalHost = "[" + host + "]"
	}
	if port != "" {
		canonicalHost += ":" + port
	}
	if parsed.Host != canonicalHost {
		return "", errors.New("authority witness endpoint host is not canonical")
	}
	return host, nil
}

func validateServerNameV1(serverName string) error {
	if serverName == "" || serverName != strings.TrimSpace(serverName) || len(serverName) > 253 ||
		strings.ContainsAny(serverName, "/\\?#@*[]") || strings.Contains(serverName, "..") {
		return errors.New("authority witness server name is invalid")
	}
	if ip := net.ParseIP(serverName); ip != nil {
		if ip.String() != serverName {
			return errors.New("authority witness IP server name is not canonical")
		}
		return nil
	}
	if serverName != strings.ToLower(serverName) || strings.HasPrefix(serverName, ".") || strings.HasSuffix(serverName, ".") {
		return errors.New("authority witness DNS server name is not canonical")
	}
	for _, label := range strings.Split(serverName, ".") {
		if len(label) == 0 || len(label) > 63 || !isASCIILetterOrDigit(label[0]) || !isASCIILetterOrDigit(label[len(label)-1]) {
			return errors.New("authority witness DNS server name is invalid")
		}
		for index := 1; index+1 < len(label); index++ {
			if !isASCIILetterOrDigit(label[index]) && label[index] != '-' {
				return errors.New("authority witness DNS server name is invalid")
			}
		}
	}
	return nil
}

func isASCIILetterOrDigit(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= '0' && value <= '9'
}

func decodeCanonicalPublicKey(encoded string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(decoded) != ed25519.PublicKeySize || base64.RawURLEncoding.EncodeToString(decoded) != encoded {
		return nil, errors.New("Ed25519 public key is invalid")
	}
	return decoded, nil
}

func manifestPayloadBytesV1(manifest ManifestV1) []byte {
	manifest.InstallationAuthoritySignature = ""
	manifest.ManifestDigest = ""
	body, _ := json.Marshal(manifest)
	return body
}

func manifestDigestV1(manifest ManifestV1) string {
	payload := append([]byte(nil), manifestDigestDomainV1...)
	return domainsecurity.SHA256Hex(append(payload, manifestPayloadBytesV1(manifest)...))
}

func isCanonicalSHA256(value string) bool {
	return value == strings.TrimSpace(value) && domainsecurity.IsSHA256Hex(value)
}

func validateManifestJSONShapeV1(body []byte) error {
	object, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 64 << 10, MaxDepth: 8, MaxTokens: 256, MaxStringBytes: 8 << 10,
	})
	if err != nil {
		return err
	}
	required := [...]string{
		"schemaVersion", "purpose", "installationId", "installationAuthorityAlgorithm",
		"installationAuthorityKeyId", "installationAuthorityPublicKey", "issuedAt", "threadRisk",
		"sharedEvidence", "installationAuthoritySignature", "manifestDigest",
	}
	if err := requireExactFieldsV1(object, required[:], nil, "authority enrollment manifest"); err != nil {
		return err
	}
	if err := validateWitnessEnrollmentJSONShapeV1(object["threadRisk"], "thread risk authority enrollment"); err != nil {
		return err
	}
	return validateWitnessEnrollmentJSONShapeV1(object["sharedEvidence"], "shared evidence authority enrollment")
}

func validateWitnessEnrollmentJSONShapeV1(body []byte, name string) error {
	object, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 16 << 10, MaxDepth: 2, MaxTokens: 64, MaxStringBytes: 8 << 10,
	})
	if err != nil {
		return err
	}
	required := [...]string{
		"namespace", "enrollmentId", "endpointOrigin", "witnessAlgorithm", "witnessKeyId",
		"witnessPublicKey", "rootCaSha256", "serverName", "timeoutMs", "initialCheckpoint",
	}
	optional := [...]string{"mtlsClientIdentityCertificateSha256"}
	return requireExactFieldsV1(object, required[:], optional[:], name)
}

func requireExactFieldsV1(object map[string]json.RawMessage, required, optional []string, name string) error {
	allowed := make(map[string]struct{}, len(required)+len(optional))
	for _, field := range required {
		allowed[field] = struct{}{}
		raw, exists := object[field]
		if !exists || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return errors.New(name + " required field is missing or null")
		}
	}
	for _, field := range optional {
		allowed[field] = struct{}{}
		if raw, exists := object[field]; exists && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return errors.New(name + " optional field cannot be null")
		}
	}
	for field := range object {
		if _, exists := allowed[field]; !exists {
			return errors.New(name + " contains an unknown field")
		}
	}
	return nil
}
