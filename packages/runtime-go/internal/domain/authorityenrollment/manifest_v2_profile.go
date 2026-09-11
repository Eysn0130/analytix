package authorityenrollment

import (
	"errors"

	domaincredentials "analytix.local/runtime-go/internal/domain/authoritycredentials"
)

type BoundCredentialProfileV1 struct {
	manifest AnchoredManifestV2
	profile  domaincredentials.CredentialProfileV1
}

// BoundCredentialProjectionV1 is detached comparison/configuration data, not
// an authorization capability. Credential/TLS loaders must accept the opaque
// BoundCredentialProfileV1 and project it internally.
type BoundCredentialProjectionV1 struct {
	Manifest ManifestAnchorProjectionV2
	Files    []domaincredentials.FileDescriptorV1
}

func BindCredentialProfileForManifestV2(
	anchored AnchoredManifestV2,
	profile domaincredentials.CredentialProfileV1,
) (BoundCredentialProfileV1, error) {
	if err := ValidateCredentialProfileForManifestV2(anchored, profile); err != nil {
		return BoundCredentialProfileV1{}, err
	}
	profile.Files = append([]domaincredentials.FileDescriptorV1(nil), profile.Files...)
	return BoundCredentialProfileV1{manifest: anchored, profile: profile}, nil
}

func ProjectBoundCredentialProfileForNamespaceV1(
	bound BoundCredentialProfileV1,
	namespace string,
) (BoundCredentialProjectionV1, error) {
	if err := ValidateCredentialProfileForManifestV2(bound.manifest, bound.profile); err != nil {
		return BoundCredentialProjectionV1{}, err
	}
	manifest, err := ProjectAnchoredManifestForNamespaceV2(bound.manifest, namespace)
	if err != nil {
		return BoundCredentialProjectionV1{}, err
	}
	files := make([]domaincredentials.FileDescriptorV1, 0, 3)
	for _, file := range bound.profile.Files {
		if file.Namespace == namespace {
			files = append(files, file)
		}
	}
	if len(files) == 0 {
		return BoundCredentialProjectionV1{}, errors.New("bound authority credential namespace is empty")
	}
	return BoundCredentialProjectionV1{Manifest: manifest, Files: files}, nil
}

// ValidateCredentialProfileForManifestV2 is the only domain bridge from an
// opaque independently anchored enrollment to exact credential files. It
// rejects a valid but unselected generation, profile, namespace, root, or
// client identity before any TLS client can be constructed.
func ValidateCredentialProfileForManifestV2(
	anchored AnchoredManifestV2,
	profile domaincredentials.CredentialProfileV1,
) error {
	if err := domaincredentials.ValidateCredentialProfileV1(profile); err != nil {
		return err
	}
	threadProjection, err := ProjectAnchoredManifestForNamespaceV2(anchored, ThreadRiskNamespaceV1)
	if err != nil {
		return err
	}
	sharedProjection, err := ProjectAnchoredManifestForNamespaceV2(anchored, SharedEvidenceNamespaceV1)
	if err != nil {
		return err
	}
	if threadProjection.ManifestDigest != sharedProjection.ManifestDigest ||
		threadProjection.InstallationID != profile.InstallationID ||
		threadProjection.InstallationAuthorityKeyID != profile.AuthorityKeyID ||
		threadProjection.CredentialProfileGeneration != profile.ProfileGeneration ||
		threadProjection.CredentialProfileDigest != profile.ProfileDigest ||
		sharedProjection.CredentialProfileGeneration != profile.ProfileGeneration ||
		sharedProjection.CredentialProfileDigest != profile.ProfileDigest {
		return errors.New("authority credential profile is not selected by the anchored manifest")
	}
	if err := validateCredentialNamespaceForEnrollmentV2(profile, threadProjection.Enrollment); err != nil {
		return err
	}
	return validateCredentialNamespaceForEnrollmentV2(profile, sharedProjection.Enrollment)
}

func validateCredentialNamespaceForEnrollmentV2(
	profile domaincredentials.CredentialProfileV1,
	enrollment WitnessEnrollmentV1,
) error {
	var root, chain, key *domaincredentials.FileDescriptorV1
	for index := range profile.Files {
		file := &profile.Files[index]
		if file.Namespace != enrollment.Namespace {
			continue
		}
		if file.EnrollmentID != enrollment.EnrollmentID {
			return errors.New("authority credential profile enrollment lineage mismatch")
		}
		switch file.Role {
		case domaincredentials.RoleWitnessRootCA:
			root = file
		case domaincredentials.RoleWitnessMTLSClientChain:
			chain = file
		case domaincredentials.RoleWitnessMTLSClientPrivateKey:
			key = file
		}
	}
	if root == nil || root.SemanticSHA256 != enrollment.RootCASHA256 {
		return errors.New("authority credential root CA does not match the enrolled namespace")
	}
	if enrollment.MTLSClientIdentityCertificateSHA256 == "" {
		if chain != nil || key != nil {
			return errors.New("authority credential profile adds an unenrolled mTLS identity")
		}
		return nil
	}
	if chain == nil || key == nil || chain.SemanticSHA256 != enrollment.MTLSClientIdentityCertificateSHA256 {
		return errors.New("authority credential mTLS identity does not match the enrolled namespace")
	}
	return nil
}
