package authoritycredentials

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	CredentialProfileSchemaVersionV1 = 1
	CredentialProfilePurposeV1       = "analytix.authority-credential-profile/v1"

	RoleWitnessRootCA               FileRoleV1 = "witness_root_ca"
	RoleWitnessMTLSClientChain      FileRoleV1 = "witness_mtls_client_chain"
	RoleWitnessMTLSClientPrivateKey FileRoleV1 = "witness_mtls_client_private_key"

	MaxRootCABytesV1      uint64 = 64 << 10
	MaxClientChainBytesV1 uint64 = 256 << 10
	MaxClientKeyBytesV1   uint64 = 64 << 10
	MaxProfileBytesV1            = 64 << 10
	MaxBundleBytesV1      uint64 = 768 << 10
	MaxSafeJSONIntegerV1  uint64 = 1<<53 - 1
)

var credentialProfileDigestDomainV1 = []byte("analytix.authority-credential-profile/digest/v1\x00")

// FileRoleV1 is closed. Root CA files contain exactly one DER certificate,
// client-chain files use the canonical analytix length-prefixed DER chain/v1
// format, and client private keys contain exactly one PKCS#8 DER key.
type FileRoleV1 string

// FileDescriptorV1 binds both the exact file representation and its parsed
// security meaning. SemanticSHA256 is SHA-256 over root/leaf Raw DER for
// certificate roles and SHA-256 over the PKIX public-key DER for private keys.
type FileDescriptorV1 struct {
	Role           FileRoleV1 `json:"role"`
	Namespace      string     `json:"namespace"`
	EnrollmentID   string     `json:"enrollmentId"`
	Generation     uint64     `json:"generation"`
	FixedName      string     `json:"fixedName"`
	SizeBytes      uint64     `json:"sizeBytes"`
	FileSHA256     string     `json:"fileSha256"`
	SemanticSHA256 string     `json:"semanticSha256"`
}

type CredentialProfileV1 struct {
	SchemaVersion     int                `json:"schemaVersion"`
	Purpose           string             `json:"purpose"`
	InstallationID    string             `json:"installationId"`
	AuthorityKeyID    string             `json:"authorityKeyId"`
	ProfileGeneration uint64             `json:"profileGeneration"`
	Files             []FileDescriptorV1 `json:"files"`
	ProfileDigest     string             `json:"profileDigest"`
}

type FileDescriptorInputV1 struct {
	Role           FileRoleV1
	Namespace      string
	EnrollmentID   string
	SizeBytes      uint64
	FileSHA256     string
	SemanticSHA256 string
}

type CredentialProfileInputV1 struct {
	InstallationID    string
	AuthorityKeyID    string
	ProfileGeneration uint64
	Files             []FileDescriptorInputV1
}

func NewCredentialProfileV1(input CredentialProfileInputV1) (CredentialProfileV1, error) {
	profile := CredentialProfileV1{
		SchemaVersion:     CredentialProfileSchemaVersionV1,
		Purpose:           CredentialProfilePurposeV1,
		InstallationID:    input.InstallationID,
		AuthorityKeyID:    input.AuthorityKeyID,
		ProfileGeneration: input.ProfileGeneration,
		Files:             make([]FileDescriptorV1, 0, len(input.Files)),
	}
	for _, file := range input.Files {
		fixedName, err := fixedNameV1(file.Namespace, file.Role)
		if err != nil {
			return CredentialProfileV1{}, err
		}
		profile.Files = append(profile.Files, FileDescriptorV1{
			Role: file.Role, Namespace: file.Namespace, EnrollmentID: file.EnrollmentID, Generation: input.ProfileGeneration,
			FixedName: fixedName, SizeBytes: file.SizeBytes,
			FileSHA256: file.FileSHA256, SemanticSHA256: file.SemanticSHA256,
		})
	}
	sort.Slice(profile.Files, func(left, right int) bool {
		return descriptorOrderKeyV1(profile.Files[left]) < descriptorOrderKeyV1(profile.Files[right])
	})
	profile.ProfileDigest = credentialProfileDigestV1(profile)
	if err := ValidateCredentialProfileV1(profile); err != nil {
		return CredentialProfileV1{}, err
	}
	return profile, nil
}

func ValidateCredentialProfileV1(profile CredentialProfileV1) error {
	if profile.SchemaVersion != CredentialProfileSchemaVersionV1 || profile.Purpose != CredentialProfilePurposeV1 ||
		!canonicalSHA256V1(profile.InstallationID) || !canonicalSHA256V1(profile.AuthorityKeyID) ||
		profile.ProfileGeneration == 0 || profile.ProfileGeneration > MaxSafeJSONIntegerV1 ||
		len(profile.Files) < 2 || len(profile.Files) > 6 {
		return errors.New("authority credential profile is incomplete")
	}
	encoded, err := json.Marshal(profile)
	if err != nil || len(encoded) > MaxProfileBytesV1 {
		return errors.New("authority credential profile exceeds its canonical bound")
	}
	seen := make(map[string]struct{}, len(profile.Files))
	namespaceRoles := make(map[string]map[FileRoleV1]bool, 2)
	var totalBytes uint64
	previousOrder := ""
	for index, file := range profile.Files {
		fixedName, fixedNameErr := fixedNameV1(file.Namespace, file.Role)
		limit, limitErr := roleLimitV1(file.Role)
		order := descriptorOrderKeyV1(file)
		if fixedNameErr != nil || limitErr != nil || !canonicalSHA256V1(file.EnrollmentID) ||
			file.Generation != profile.ProfileGeneration || file.FixedName != fixedName ||
			file.SizeBytes == 0 || file.SizeBytes > limit || !canonicalSHA256V1(file.FileSHA256) ||
			!canonicalSHA256V1(file.SemanticSHA256) || (index > 0 && order <= previousOrder) {
			return errors.New("authority credential profile file descriptor is invalid")
		}
		previousOrder = order
		identity := strings.ToLower(file.FixedName)
		if _, duplicate := seen[identity]; duplicate {
			return errors.New("authority credential profile contains duplicate files")
		}
		seen[identity] = struct{}{}
		if file.SizeBytes > MaxBundleBytesV1-totalBytes {
			return errors.New("authority credential profile exceeds aggregate size")
		}
		totalBytes += file.SizeBytes
		if namespaceRoles[file.Namespace] == nil {
			namespaceRoles[file.Namespace] = make(map[FileRoleV1]bool, 3)
		}
		if namespaceRoles[file.Namespace][file.Role] {
			return errors.New("authority credential profile duplicates a namespace role")
		}
		namespaceRoles[file.Namespace][file.Role] = true
		for otherIndex := 0; otherIndex < index; otherIndex++ {
			other := profile.Files[otherIndex]
			if other.Namespace == file.Namespace && other.EnrollmentID != file.EnrollmentID {
				return errors.New("authority credential profile mixes enrollment lineages")
			}
		}
	}
	for _, namespace := range []string{domainsecurity.ThreadRiskAuthorityNamespaceV1, domainsecurity.EvidenceRegistryAuthorityNamespaceV1} {
		roles := namespaceRoles[namespace]
		if roles == nil || !roles[RoleWitnessRootCA] || roles[RoleWitnessMTLSClientChain] != roles[RoleWitnessMTLSClientPrivateKey] {
			return errors.New("authority credential profile namespace inventory is incomplete")
		}
	}
	if len(namespaceRoles) != 2 || totalBytes == 0 || totalBytes > MaxBundleBytesV1 {
		return errors.New("authority credential profile namespace or aggregate inventory is invalid")
	}
	if !canonicalSHA256V1(profile.ProfileDigest) || profile.ProfileDigest != credentialProfileDigestV1(profile) {
		return errors.New("authority credential profile digest is invalid")
	}
	return nil
}

func ParseCredentialProfileV1(body []byte) (CredentialProfileV1, error) {
	object, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: MaxProfileBytesV1, MaxDepth: 8, MaxTokens: 512, MaxStringBytes: 4 << 10,
	})
	if err != nil {
		return CredentialProfileV1{}, err
	}
	required := [...]string{"schemaVersion", "purpose", "installationId", "authorityKeyId", "profileGeneration", "files", "profileDigest"}
	if len(object) != len(required) {
		return CredentialProfileV1{}, errors.New("authority credential profile fields are not exact")
	}
	for _, field := range required {
		value, ok := object[field]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return CredentialProfileV1{}, errors.New("authority credential profile required field is missing or null")
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var profile CredentialProfileV1
	if err := decoder.Decode(&profile); err != nil {
		return CredentialProfileV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return CredentialProfileV1{}, errors.New("authority credential profile contains trailing JSON")
	}
	canonical, err := json.Marshal(profile)
	if err != nil || !bytes.Equal(body, canonical) {
		return CredentialProfileV1{}, errors.New("authority credential profile is not canonically encoded")
	}
	return profile, ValidateCredentialProfileV1(profile)
}

func CredentialProfileV1Bytes(profile CredentialProfileV1) ([]byte, error) {
	if err := ValidateCredentialProfileV1(profile); err != nil {
		return nil, err
	}
	return json.Marshal(profile)
}

func fixedNameV1(namespace string, role FileRoleV1) (string, error) {
	var prefix string
	switch namespace {
	case domainsecurity.ThreadRiskAuthorityNamespaceV1:
		prefix = "thread-risk"
	case domainsecurity.EvidenceRegistryAuthorityNamespaceV1:
		prefix = "shared-evidence"
	default:
		return "", errors.New("authority credential namespace is unknown")
	}
	switch role {
	case RoleWitnessRootCA:
		return prefix + "-root-ca.der", nil
	case RoleWitnessMTLSClientChain:
		return prefix + "-client-chain-v1.bin", nil
	case RoleWitnessMTLSClientPrivateKey:
		return prefix + "-client-key.pk8", nil
	default:
		return "", errors.New("authority credential file role is unknown")
	}
}

func roleLimitV1(role FileRoleV1) (uint64, error) {
	switch role {
	case RoleWitnessRootCA:
		return MaxRootCABytesV1, nil
	case RoleWitnessMTLSClientChain:
		return MaxClientChainBytesV1, nil
	case RoleWitnessMTLSClientPrivateKey:
		return MaxClientKeyBytesV1, nil
	default:
		return 0, errors.New("authority credential file role is unknown")
	}
}

func descriptorOrderKeyV1(file FileDescriptorV1) string {
	namespaceOrder := "9"
	switch file.Namespace {
	case domainsecurity.ThreadRiskAuthorityNamespaceV1:
		namespaceOrder = "0"
	case domainsecurity.EvidenceRegistryAuthorityNamespaceV1:
		namespaceOrder = "1"
	}
	roleOrder := "9"
	switch file.Role {
	case RoleWitnessRootCA:
		roleOrder = "0"
	case RoleWitnessMTLSClientChain:
		roleOrder = "1"
	case RoleWitnessMTLSClientPrivateKey:
		roleOrder = "2"
	}
	return namespaceOrder + roleOrder + file.FixedName
}

func credentialProfileDigestV1(profile CredentialProfileV1) string {
	profile.ProfileDigest = ""
	body, _ := json.Marshal(profile)
	payload := append([]byte(nil), credentialProfileDigestDomainV1...)
	return domainsecurity.SHA256Hex(append(payload, body...))
}

func canonicalSHA256V1(value string) bool {
	return value == strings.TrimSpace(value) && value == strings.ToLower(value) && domainsecurity.IsSHA256Hex(value)
}
