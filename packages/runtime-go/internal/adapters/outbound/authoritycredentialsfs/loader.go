package authoritycredentialsfs

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"errors"
	"time"

	monotonicheadhttp "analytix.local/runtime-go/internal/adapters/outbound/monotonicheadhttp"
	secureconfigfs "analytix.local/runtime-go/internal/adapters/outbound/secureconfigfs"
	domaincredentials "analytix.local/runtime-go/internal/domain/authoritycredentials"
	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityanchorport "analytix.local/runtime-go/internal/ports/authorityanchor"
	authoritycredentialsport "analytix.local/runtime-go/internal/ports/authoritycredentials"
	monotonicheadport "analytix.local/runtime-go/internal/ports/monotonichead"
)

const ProfileFileNameV1 = "profile.json"

var ErrInvalid = errors.New("authority credential bundle is invalid")

type secureExactReaderV1 func(secureconfigfs.ReadExactInput) ([]byte, error)
type secureBundleReaderV1 func(secureconfigfs.ReadBundleInput) (map[string][]byte, error)

type Loader struct {
	ProfileRoot string
	BundleRoot  string
	Anchor      authorityanchorport.Source
	Now         func() time.Time
}

var _ authoritycredentialsport.Loader = Loader{}

type namespaceMaterialV1 struct {
	projection   domainenrollment.BoundCredentialProjectionV1
	rootCAs      *x509.CertPool
	certificates []tls.Certificate
	validFrom    time.Time
	validUntil   time.Time
}

func (loader Loader) LoadCurrent(
	ctx context.Context,
	anchored domainenrollment.AnchoredManifestV2,
) (authoritycredentialsport.EnrolledWitnessesV1, error) {
	return loader.loadCurrentWithReads(ctx, anchored, secureconfigfs.ReadExact, secureconfigfs.ReadBundle)
}

func (loader Loader) loadCurrentWithReads(
	ctx context.Context,
	anchored domainenrollment.AnchoredManifestV2,
	readExact secureExactReaderV1,
	readBundle secureBundleReaderV1,
) (authoritycredentialsport.EnrolledWitnessesV1, error) {
	if ctx == nil || loader.Anchor == nil || loader.ProfileRoot == "" || loader.BundleRoot == "" ||
		loader.ProfileRoot == loader.BundleRoot || readExact == nil || readBundle == nil {
		return authoritycredentialsport.EnrolledWitnessesV1{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return authoritycredentialsport.EnrolledWitnessesV1{}, err
	}
	manifestProjection, err := domainenrollment.ProjectAnchoredManifestForNamespaceV2(
		anchored, domainenrollment.ThreadRiskNamespaceV1,
	)
	if err != nil {
		return authoritycredentialsport.EnrolledWitnessesV1{}, ErrInvalid
	}
	before, err := loader.Anchor.Load(ctx)
	if err != nil {
		return authoritycredentialsport.EnrolledWitnessesV1{}, err
	}
	if !anchorSelectsManifestV1(before, manifestProjection) {
		return authoritycredentialsport.EnrolledWitnessesV1{}, ErrInvalid
	}
	profileBody, err := readExact(secureconfigfs.ReadExactInput{
		Root: loader.ProfileRoot, Target: ProfileFileNameV1,
		AllowedNames: []string{ProfileFileNameV1}, MaxBytes: domaincredentials.MaxProfileBytesV1,
	})
	if err != nil {
		return authoritycredentialsport.EnrolledWitnessesV1{}, errors.Join(ErrInvalid, err)
	}
	if err := ctx.Err(); err != nil {
		return authoritycredentialsport.EnrolledWitnessesV1{}, err
	}
	profile, err := domaincredentials.ParseCredentialProfileV1(profileBody)
	if err != nil {
		return authoritycredentialsport.EnrolledWitnessesV1{}, errors.Join(ErrInvalid, err)
	}
	bound, err := domainenrollment.BindCredentialProfileForManifestV2(anchored, profile)
	if err != nil {
		return authoritycredentialsport.EnrolledWitnessesV1{}, errors.Join(ErrInvalid, err)
	}
	descriptors, bundleInput, err := bundleReadInputV1(bound, loader.BundleRoot)
	if err != nil {
		return authoritycredentialsport.EnrolledWitnessesV1{}, errors.Join(ErrInvalid, err)
	}
	files, err := readBundle(bundleInput)
	if err != nil {
		return authoritycredentialsport.EnrolledWitnessesV1{}, errors.Join(ErrInvalid, err)
	}
	defer wipePrivateKeyFilesV1(descriptors, files)
	if err := verifyExactFilesV1(descriptors, files); err != nil {
		return authoritycredentialsport.EnrolledWitnessesV1{}, err
	}
	clock := loader.Now
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	now := clock().UTC()
	if now.IsZero() {
		return authoritycredentialsport.EnrolledWitnessesV1{}, ErrInvalid
	}
	threadMaterial, err := parseNamespaceMaterialV1(bound, domainenrollment.ThreadRiskNamespaceV1, files, now)
	if err != nil {
		return authoritycredentialsport.EnrolledWitnessesV1{}, err
	}
	sharedMaterial, err := parseNamespaceMaterialV1(bound, domainenrollment.SharedEvidenceNamespaceV1, files, now)
	if err != nil {
		return authoritycredentialsport.EnrolledWitnessesV1{}, err
	}
	if err := ctx.Err(); err != nil {
		return authoritycredentialsport.EnrolledWitnessesV1{}, err
	}
	after, err := loader.Anchor.Load(ctx)
	if err != nil {
		return authoritycredentialsport.EnrolledWitnessesV1{}, err
	}
	if !sameAnchorV1(before, after) || !anchorSelectsManifestV1(after, manifestProjection) {
		return authoritycredentialsport.EnrolledWitnessesV1{}, ErrInvalid
	}
	threadWitness, err := newWitnessV1(threadMaterial, loader.Anchor, before, clock)
	if err != nil {
		return authoritycredentialsport.EnrolledWitnessesV1{}, err
	}
	sharedWitness, err := newWitnessV1(sharedMaterial, loader.Anchor, before, clock)
	if err != nil {
		return authoritycredentialsport.EnrolledWitnessesV1{}, err
	}
	if err := ctx.Err(); err != nil {
		return authoritycredentialsport.EnrolledWitnessesV1{}, err
	}
	return authoritycredentialsport.EnrolledWitnessesV1{
		ManifestDigest: manifestProjection.ManifestDigest, ProfileDigest: manifestProjection.CredentialProfileDigest,
		ProfileGeneration: manifestProjection.CredentialProfileGeneration,
		ThreadRisk:        threadWitness, SharedEvidence: sharedWitness,
	}, nil
}

func bundleReadInputV1(
	bound domainenrollment.BoundCredentialProfileV1,
	root string,
) (map[string]domaincredentials.FileDescriptorV1, secureconfigfs.ReadBundleInput, error) {
	descriptors := make(map[string]domaincredentials.FileDescriptorV1, 6)
	files := make([]secureconfigfs.BundleFile, 0, 6)
	var total int64
	for _, namespace := range []string{domainenrollment.ThreadRiskNamespaceV1, domainenrollment.SharedEvidenceNamespaceV1} {
		projection, err := domainenrollment.ProjectBoundCredentialProfileForNamespaceV1(bound, namespace)
		if err != nil {
			return nil, secureconfigfs.ReadBundleInput{}, err
		}
		for _, descriptor := range projection.Files {
			if _, duplicate := descriptors[descriptor.FixedName]; duplicate || descriptor.SizeBytes > uint64(^uint64(0)>>1) {
				return nil, secureconfigfs.ReadBundleInput{}, ErrInvalid
			}
			size := int64(descriptor.SizeBytes)
			if size <= 0 || total > int64(domaincredentials.MaxBundleBytesV1)-size {
				return nil, secureconfigfs.ReadBundleInput{}, ErrInvalid
			}
			total += size
			descriptors[descriptor.FixedName] = descriptor
			files = append(files, secureconfigfs.BundleFile{
				Name: descriptor.FixedName, MaxBytes: size,
				Sensitive: descriptor.Role == domaincredentials.RoleWitnessMTLSClientPrivateKey,
			})
		}
	}
	return descriptors, secureconfigfs.ReadBundleInput{Root: root, Files: files, MaxTotalBytes: total}, nil
}

func wipePrivateKeyFilesV1(
	descriptors map[string]domaincredentials.FileDescriptorV1,
	files map[string][]byte,
) {
	for name, descriptor := range descriptors {
		if descriptor.Role == domaincredentials.RoleWitnessMTLSClientPrivateKey {
			clear(files[name])
		}
	}
}

func verifyExactFilesV1(
	descriptors map[string]domaincredentials.FileDescriptorV1,
	files map[string][]byte,
) error {
	if len(files) != len(descriptors) {
		return ErrInvalid
	}
	for name, descriptor := range descriptors {
		body, ok := files[name]
		if !ok || uint64(len(body)) != descriptor.SizeBytes || domainsecurity.SHA256Hex(body) != descriptor.FileSHA256 {
			return ErrInvalid
		}
	}
	return nil
}

func parseNamespaceMaterialV1(
	bound domainenrollment.BoundCredentialProfileV1,
	namespace string,
	files map[string][]byte,
	now time.Time,
) (namespaceMaterialV1, error) {
	projection, err := domainenrollment.ProjectBoundCredentialProfileForNamespaceV1(bound, namespace)
	if err != nil {
		return namespaceMaterialV1{}, ErrInvalid
	}
	descriptors := make(map[domaincredentials.FileRoleV1]domaincredentials.FileDescriptorV1, len(projection.Files))
	for _, descriptor := range projection.Files {
		descriptors[descriptor.Role] = descriptor
	}
	rootDescriptor, ok := descriptors[domaincredentials.RoleWitnessRootCA]
	if !ok {
		return namespaceMaterialV1{}, ErrInvalid
	}
	root, roots, err := parseRootV1(files[rootDescriptor.FixedName], rootDescriptor.SemanticSHA256, now)
	if err != nil {
		return namespaceMaterialV1{}, err
	}
	chainDescriptor, hasChain := descriptors[domaincredentials.RoleWitnessMTLSClientChain]
	keyDescriptor, hasKey := descriptors[domaincredentials.RoleWitnessMTLSClientPrivateKey]
	if hasChain != hasKey {
		return namespaceMaterialV1{}, ErrInvalid
	}
	material := namespaceMaterialV1{
		projection: projection, rootCAs: roots,
		validFrom: root.NotBefore.UTC(), validUntil: root.NotAfter.UTC(),
	}
	if !hasChain {
		return material, nil
	}
	keyBody := files[keyDescriptor.FixedName]
	defer clear(keyBody)
	certificate, validFrom, validUntil, err := parseClientIdentityV1(
		files[chainDescriptor.FixedName], keyBody, root, roots,
		chainDescriptor.SemanticSHA256, keyDescriptor.SemanticSHA256, now,
	)
	if err != nil {
		return namespaceMaterialV1{}, err
	}
	material.certificates = []tls.Certificate{certificate}
	material.validFrom = latestTimeV1(material.validFrom, validFrom)
	material.validUntil = earliestTimeV1(material.validUntil, validUntil)
	if material.validUntil.Before(material.validFrom) {
		return namespaceMaterialV1{}, ErrInvalid
	}
	return material, nil
}

func parseRootV1(body []byte, expectedDigest string, now time.Time) (*x509.Certificate, *x509.CertPool, error) {
	certificate, err := x509.ParseCertificate(body)
	if err != nil || !bytes.Equal(certificate.Raw, body) || domainsecurity.SHA256Hex(certificate.Raw) != expectedDigest ||
		!certificate.BasicConstraintsValid || !certificate.IsCA || !validCAKeyUsageV1(certificate) ||
		now.Before(certificate.NotBefore) || now.After(certificate.NotAfter) ||
		!bytes.Equal(certificate.RawIssuer, certificate.RawSubject) || !validUnconstrainedRootV1(certificate) ||
		!validCertificateSignatureAlgorithmV1(certificate.SignatureAlgorithm) ||
		validateCertificatePublicKeyV1(certificate.PublicKey) != nil || certificate.CheckSignatureFrom(certificate) != nil {
		return nil, nil, ErrInvalid
	}
	roots := x509.NewCertPool()
	roots.AddCert(certificate)
	return certificate, roots, nil
}

func parseClientIdentityV1(
	chainBody, keyBody []byte,
	root *x509.Certificate,
	roots *x509.CertPool,
	expectedLeafDigest, expectedKeySPKIDigest string,
	now time.Time,
) (tls.Certificate, time.Time, time.Time, error) {
	chainDER, err := domaincredentials.ParseClientChainV1(chainBody)
	if err != nil {
		return tls.Certificate{}, time.Time{}, time.Time{}, ErrInvalid
	}
	certificates := make([]*x509.Certificate, 0, len(chainDER))
	seen := make(map[string]struct{}, len(chainDER))
	seenSubjects := map[string]struct{}{string(root.RawSubject): {}}
	seenSubjectPublicKeys := map[string]struct{}{string(root.RawSubjectPublicKeyInfo): {}}
	for _, der := range chainDER {
		certificate, parseErr := x509.ParseCertificate(der)
		digest := domainsecurity.SHA256Hex(der)
		if parseErr != nil || !bytes.Equal(certificate.Raw, der) || now.Before(certificate.NotBefore) || now.After(certificate.NotAfter) {
			return tls.Certificate{}, time.Time{}, time.Time{}, ErrInvalid
		}
		_, duplicateDigest := seen[digest]
		_, duplicateSubject := seenSubjects[string(certificate.RawSubject)]
		_, duplicateSubjectPublicKey := seenSubjectPublicKeys[string(certificate.RawSubjectPublicKeyInfo)]
		if duplicateDigest || duplicateSubject || duplicateSubjectPublicKey || bytes.Equal(certificate.Raw, root.Raw) {
			return tls.Certificate{}, time.Time{}, time.Time{}, ErrInvalid
		}
		seen[digest] = struct{}{}
		seenSubjects[string(certificate.RawSubject)] = struct{}{}
		seenSubjectPublicKeys[string(certificate.RawSubjectPublicKeyInfo)] = struct{}{}
		certificates = append(certificates, certificate)
	}
	leaf := certificates[0]
	if domainsecurity.SHA256Hex(leaf.Raw) != expectedLeafDigest || !leaf.BasicConstraintsValid || leaf.IsCA ||
		leaf.KeyUsage != x509.KeyUsageDigitalSignature || !explicitClientAuthV1(leaf) ||
		!validCertificateSignatureAlgorithmV1(leaf.SignatureAlgorithm) ||
		validateCertificatePublicKeyV1(leaf.PublicKey) != nil {
		return tls.Certificate{}, time.Time{}, time.Time{}, ErrInvalid
	}
	intermediates := x509.NewCertPool()
	for _, certificate := range certificates[1:] {
		if !certificate.BasicConstraintsValid || !certificate.IsCA || !validCAKeyUsageV1(certificate) ||
			!validCAExtendedKeyUsageV1(certificate) || !validCertificateSignatureAlgorithmV1(certificate.SignatureAlgorithm) ||
			validateCertificatePublicKeyV1(certificate.PublicKey) != nil {
			return tls.Certificate{}, time.Time{}, time.Time{}, ErrInvalid
		}
		intermediates.AddCert(certificate)
	}
	for index := 0; index+1 < len(certificates); index++ {
		if !bytes.Equal(certificates[index].RawIssuer, certificates[index+1].RawSubject) ||
			certificates[index].CheckSignatureFrom(certificates[index+1]) != nil {
			return tls.Certificate{}, time.Time{}, time.Time{}, ErrInvalid
		}
	}
	if !bytes.Equal(certificates[len(certificates)-1].RawIssuer, root.RawSubject) ||
		certificates[len(certificates)-1].CheckSignatureFrom(root) != nil {
		return tls.Certificate{}, time.Time{}, time.Time{}, ErrInvalid
	}
	verifiedChains, err := leaf.Verify(x509.VerifyOptions{
		Roots: roots, Intermediates: intermediates, CurrentTime: now,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	if err != nil || !isUniqueExactVerifiedChainV1(verifiedChains, certificates, root) {
		return tls.Certificate{}, time.Time{}, time.Time{}, ErrInvalid
	}
	var rawKey asn1.RawValue
	rest, err := asn1.Unmarshal(keyBody, &rawKey)
	if err != nil || len(rest) != 0 || !bytes.Equal(rawKey.FullBytes, keyBody) {
		return tls.Certificate{}, time.Time{}, time.Time{}, ErrInvalid
	}
	parsedKey, err := x509.ParsePKCS8PrivateKey(keyBody)
	if err != nil {
		return tls.Certificate{}, time.Time{}, time.Time{}, ErrInvalid
	}
	canonicalKey, err := x509.MarshalPKCS8PrivateKey(parsedKey)
	if err != nil {
		return tls.Certificate{}, time.Time{}, time.Time{}, ErrInvalid
	}
	defer clear(canonicalKey)
	if !bytes.Equal(canonicalKey, keyBody) {
		return tls.Certificate{}, time.Time{}, time.Time{}, ErrInvalid
	}
	signer, ok := parsedKey.(crypto.Signer)
	if !ok || validateSignerV1(signer) != nil {
		return tls.Certificate{}, time.Time{}, time.Time{}, ErrInvalid
	}
	keySPKI, keyErr := x509.MarshalPKIXPublicKey(signer.Public())
	leafSPKI, leafErr := x509.MarshalPKIXPublicKey(leaf.PublicKey)
	if keyErr != nil || leafErr != nil || !bytes.Equal(keySPKI, leafSPKI) ||
		domainsecurity.SHA256Hex(keySPKI) != expectedKeySPKIDigest {
		return tls.Certificate{}, time.Time{}, time.Time{}, ErrInvalid
	}
	validFrom, validUntil := root.NotBefore.UTC(), root.NotAfter.UTC()
	for _, certificate := range certificates {
		validFrom = latestTimeV1(validFrom, certificate.NotBefore.UTC())
		validUntil = earliestTimeV1(validUntil, certificate.NotAfter.UTC())
	}
	if validUntil.Before(validFrom) {
		return tls.Certificate{}, time.Time{}, time.Time{}, ErrInvalid
	}
	return tls.Certificate{Certificate: chainDER, PrivateKey: signer, Leaf: leaf}, validFrom, validUntil, nil
}

func isUniqueExactVerifiedChainV1(
	verifiedChains [][]*x509.Certificate,
	provided []*x509.Certificate,
	root *x509.Certificate,
) bool {
	if len(verifiedChains) != 1 || len(verifiedChains[0]) != len(provided)+1 {
		return false
	}
	chain := verifiedChains[0]
	for index, certificate := range provided {
		if !bytes.Equal(chain[index].Raw, certificate.Raw) {
			return false
		}
	}
	return bytes.Equal(chain[len(chain)-1].Raw, root.Raw)
}

func validateCertificatePublicKeyV1(publicKey any) error {
	switch key := publicKey.(type) {
	case *rsa.PublicKey:
		if key == nil || key.N == nil ||
			(key.N.BitLen() != 2048 && key.N.BitLen() != 3072 && key.N.BitLen() != 4096) || key.E != 65537 {
			return ErrInvalid
		}
	case *ecdsa.PublicKey:
		if key == nil || (key.Curve != elliptic.P256() && key.Curve != elliptic.P384() && key.Curve != elliptic.P521()) ||
			key.X == nil || key.Y == nil || !key.Curve.IsOnCurve(key.X, key.Y) {
			return ErrInvalid
		}
	case ed25519.PublicKey:
		if len(key) != ed25519.PublicKeySize {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

func explicitClientAuthV1(certificate *x509.Certificate) bool {
	return certificate != nil && len(certificate.UnknownExtKeyUsage) == 0 &&
		len(certificate.ExtKeyUsage) == 1 && certificate.ExtKeyUsage[0] == x509.ExtKeyUsageClientAuth
}

func validCAKeyUsageV1(certificate *x509.Certificate) bool {
	if certificate == nil || certificate.KeyUsage&x509.KeyUsageCertSign == 0 {
		return false
	}
	return certificate.KeyUsage & ^(x509.KeyUsageCertSign|x509.KeyUsageCRLSign) == 0
}

func validCAExtendedKeyUsageV1(certificate *x509.Certificate) bool {
	if certificate == nil || len(certificate.UnknownExtKeyUsage) != 0 {
		return false
	}
	return len(certificate.ExtKeyUsage) == 0 ||
		(len(certificate.ExtKeyUsage) == 1 && certificate.ExtKeyUsage[0] == x509.ExtKeyUsageClientAuth)
}

func validUnconstrainedRootV1(certificate *x509.Certificate) bool {
	return certificate != nil && len(certificate.ExtKeyUsage) == 0 && len(certificate.UnknownExtKeyUsage) == 0
}

func validCertificateSignatureAlgorithmV1(algorithm x509.SignatureAlgorithm) bool {
	switch algorithm {
	case x509.SHA256WithRSA, x509.SHA384WithRSA, x509.SHA512WithRSA,
		x509.SHA256WithRSAPSS, x509.SHA384WithRSAPSS, x509.SHA512WithRSAPSS,
		x509.ECDSAWithSHA256, x509.ECDSAWithSHA384, x509.ECDSAWithSHA512,
		x509.PureEd25519:
		return true
	default:
		return false
	}
}

func validateSignerV1(signer crypto.Signer) error {
	message := sha256.Sum256([]byte("analytix.authority-client-key-proof/v1"))
	switch key := signer.(type) {
	case *rsa.PrivateKey:
		if len(key.Primes) != 2 || validateCertificatePublicKeyV1(&key.PublicKey) != nil || key.Validate() != nil {
			return ErrInvalid
		}
		signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, message[:])
		if err != nil || rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, message[:], signature) != nil {
			return ErrInvalid
		}
	case *ecdsa.PrivateKey:
		if (key.Curve != elliptic.P256() && key.Curve != elliptic.P384() && key.Curve != elliptic.P521()) ||
			key.D == nil || key.D.Sign() <= 0 {
			return ErrInvalid
		}
		signature, err := ecdsa.SignASN1(rand.Reader, key, message[:])
		if err != nil || !ecdsa.VerifyASN1(&key.PublicKey, message[:], signature) {
			return ErrInvalid
		}
	case ed25519.PrivateKey:
		if len(key) != ed25519.PrivateKeySize {
			return ErrInvalid
		}
		signature := ed25519.Sign(key, message[:])
		if !ed25519.Verify(key.Public().(ed25519.PublicKey), message[:], signature) {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

func latestTimeV1(left, right time.Time) time.Time {
	if right.After(left) {
		return right
	}
	return left
}

func earliestTimeV1(left, right time.Time) time.Time {
	if right.Before(left) {
		return right
	}
	return left
}

func newWitnessV1(
	material namespaceMaterialV1,
	anchor authorityanchorport.Source,
	expected authorityanchorport.AnchorV1,
	clock func() time.Time,
) (monotonicheadport.MutationRecoveryWitness, error) {
	if anchor == nil || clock == nil || material.validFrom.IsZero() || material.validUntil.IsZero() ||
		material.validUntil.Before(material.validFrom) {
		return nil, ErrInvalid
	}
	projection := material.projection.Manifest
	witnessPublicKey, err := base64.RawURLEncoding.DecodeString(projection.Enrollment.WitnessPublicKey)
	if err != nil || base64.RawURLEncoding.EncodeToString(witnessPublicKey) != projection.Enrollment.WitnessPublicKey {
		return nil, ErrInvalid
	}
	client, err := monotonicheadhttp.New(monotonicheadhttp.Config{
		Endpoint: projection.Enrollment.EndpointOrigin, InstallationID: projection.InstallationID,
		EnrollmentID: projection.Enrollment.EnrollmentID, Namespace: projection.Enrollment.Namespace,
		AuthorityKeyID:     projection.InstallationAuthorityKeyID,
		AuthorityPublicKey: append(ed25519.PublicKey(nil), projection.InstallationAuthorityPublicKey...),
		WitnessKeyID:       projection.Enrollment.WitnessKeyID,
		WitnessPublicKey:   append(ed25519.PublicKey(nil), witnessPublicKey...),
		RootCAs:            material.rootCAs, ClientCertificates: material.certificates,
		ServerName: projection.Enrollment.ServerName, Timeout: time.Duration(projection.Enrollment.TimeoutMS) * time.Millisecond,
	})
	if err != nil {
		return nil, ErrInvalid
	}
	expected.AuthorityPublicKey = append([]byte(nil), expected.AuthorityPublicKey...)
	return &revalidatingWitnessV1{
		client: client, anchor: anchor, expected: expected, clock: clock,
		validFrom: material.validFrom, validUntil: material.validUntil,
	}, nil
}

type revalidatingWitnessV1 struct {
	client     monotonicheadport.MutationRecoveryWitness
	anchor     authorityanchorport.Source
	expected   authorityanchorport.AnchorV1
	clock      func() time.Time
	validFrom  time.Time
	validUntil time.Time
}

func (witness *revalidatingWitnessV1) Observe(
	ctx context.Context,
	request domainsecurity.MonotonicHeadObserveRequestV1,
) (domainsecurity.MonotonicHeadObservationV1, error) {
	if err := witness.validateCurrentAuthority(ctx); err != nil {
		return domainsecurity.MonotonicHeadObservationV1{}, err
	}
	observation, callErr := witness.client.Observe(ctx, request)
	if callErr != nil {
		return domainsecurity.MonotonicHeadObservationV1{}, callErr
	}
	if authorityErr := witness.validateCurrentAuthority(ctx); authorityErr != nil {
		if errors.Is(authorityErr, context.Canceled) || errors.Is(authorityErr, context.DeadlineExceeded) {
			return domainsecurity.MonotonicHeadObservationV1{}, authorityErr
		}
		return domainsecurity.MonotonicHeadObservationV1{}, errors.Join(monotonicheadport.ErrUnavailable, authorityErr)
	}
	return observation, nil
}

func (witness *revalidatingWitnessV1) Advance(
	ctx context.Context,
	request domainsecurity.MonotonicHeadAdvanceRequestV1,
) (domainsecurity.MonotonicHeadAdvanceReceiptV1, error) {
	if err := witness.validateCurrentAuthority(ctx); err != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, err
	}
	receipt, callErr := witness.client.Advance(ctx, request)
	if callErr != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, callErr
	}
	if authorityErr := witness.validateCurrentAuthority(ctx); authorityErr != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, errors.Join(monotonicheadport.ErrIndeterminate, authorityErr)
	}
	return receipt, nil
}

func (witness *revalidatingWitnessV1) ResolveMutation(
	ctx context.Context,
	request domainsecurity.MonotonicHeadMutationResolveRequestV1,
) (domainsecurity.MonotonicHeadMutationResolutionV1, error) {
	if err := witness.validateCurrentAuthority(ctx); err != nil {
		return domainsecurity.MonotonicHeadMutationResolutionV1{}, err
	}
	resolution, callErr := witness.client.ResolveMutation(ctx, request)
	if callErr != nil {
		return domainsecurity.MonotonicHeadMutationResolutionV1{}, callErr
	}
	if authorityErr := witness.validateCurrentAuthority(ctx); authorityErr != nil {
		if errors.Is(authorityErr, context.Canceled) || errors.Is(authorityErr, context.DeadlineExceeded) {
			return domainsecurity.MonotonicHeadMutationResolutionV1{}, authorityErr
		}
		return domainsecurity.MonotonicHeadMutationResolutionV1{}, errors.Join(monotonicheadport.ErrUnavailable, authorityErr)
	}
	return resolution, nil
}

func (witness *revalidatingWitnessV1) validateCurrentAuthority(ctx context.Context) error {
	if witness == nil || witness.client == nil || witness.anchor == nil || witness.clock == nil || ctx == nil ||
		witness.validFrom.IsZero() || witness.validUntil.IsZero() || witness.validUntil.Before(witness.validFrom) {
		return monotonicheadport.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	now := witness.clock().UTC()
	if now.IsZero() || now.Before(witness.validFrom) || now.After(witness.validUntil) {
		return errors.Join(monotonicheadport.ErrUnavailable, ErrInvalid)
	}
	current, err := witness.anchor.Load(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return errors.Join(monotonicheadport.ErrUnavailable, err)
	}
	if !sameAnchorV1(witness.expected, current) {
		return errors.Join(monotonicheadport.ErrUnavailable, ErrInvalid)
	}
	return nil
}

func anchorSelectsManifestV1(anchor authorityanchorport.AnchorV1, projection domainenrollment.ManifestAnchorProjectionV2) bool {
	return anchor.InstallationID == projection.InstallationID &&
		anchor.AuthorityKeyID == projection.InstallationAuthorityKeyID &&
		anchor.CurrentManifestDigest == projection.ManifestDigest &&
		bytes.Equal(anchor.AuthorityPublicKey, projection.InstallationAuthorityPublicKey)
}

func sameAnchorV1(left, right authorityanchorport.AnchorV1) bool {
	return left.InstallationID == right.InstallationID && left.AuthorityKeyID == right.AuthorityKeyID &&
		left.CurrentManifestDigest == right.CurrentManifestDigest && bytes.Equal(left.AuthorityPublicKey, right.AuthorityPublicKey)
}
