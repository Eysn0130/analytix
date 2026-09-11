package authoritycredentialsfs

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"testing"
	"time"

	domaincredentials "analytix.local/runtime-go/internal/domain/authoritycredentials"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestCredentialChainRejectsSkippedOrMisorderedCertificate(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	root, rootKey, roots := newCryptoPolicyRootV1(t, now)
	sharedCAKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	intermediateBTemplate := cryptoPolicyCATemplateV1(2, "Intermediate B", now)
	intermediateBDER := createCryptoPolicyCertificateV1(
		t, intermediateBTemplate, root, &sharedCAKey.PublicKey, rootKey,
	)
	intermediateB, err := x509.ParseCertificate(intermediateBDER)
	if err != nil {
		t.Fatal(err)
	}
	intermediateATemplate := cryptoPolicyCATemplateV1(3, "Intermediate A", now)
	intermediateADER := createCryptoPolicyCertificateV1(
		t, intermediateATemplate, intermediateB, &sharedCAKey.PublicKey, sharedCAKey,
	)

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(4), Subject: pkix.Name{CommonName: "Client Leaf"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	leafDER := createCryptoPolicyCertificateV1(t, leafTemplate, intermediateB, &leafKey.PublicKey, sharedCAKey)
	keyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		t.Fatal(err)
	}
	keySPKI, err := x509.MarshalPKIXPublicKey(&leafKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	validChain, err := domaincredentials.ClientChainV1Bytes([][]byte{leafDER, intermediateBDER})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := parseClientIdentityV1(
		validChain, keyDER, root, roots, domainsecurity.SHA256Hex(leafDER), domainsecurity.SHA256Hex(keySPKI), now,
	); err != nil {
		t.Fatalf("valid leaf-first chain rejected: %v", err)
	}

	ambiguousChain, err := domaincredentials.ClientChainV1Bytes([][]byte{leafDER, intermediateADER, intermediateBDER})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := parseClientIdentityV1(
		ambiguousChain, keyDER, root, roots, domainsecurity.SHA256Hex(leafDER), domainsecurity.SHA256Hex(keySPKI), now,
	); err == nil {
		t.Fatal("chain with a signature-compatible but wrong-subject intermediate was accepted")
	}

	sameSubjectATemplate := cryptoPolicyCATemplateV1(8, "Intermediate B", now)
	sameSubjectADER := createCryptoPolicyCertificateV1(
		t, sameSubjectATemplate, intermediateB, &sharedCAKey.PublicKey, sharedCAKey,
	)
	sameSubjectA, err := x509.ParseCertificate(sameSubjectADER)
	if err != nil {
		t.Fatal(err)
	}
	if sameSubjectA.Equal(intermediateB) || !bytes.Equal(sameSubjectA.RawSubject, intermediateB.RawSubject) ||
		!bytes.Equal(sameSubjectA.RawSubjectPublicKeyInfo, intermediateB.RawSubjectPublicKeyInfo) {
		t.Fatal("fixture did not produce distinct CA certificates with a shared subject and public key")
	}
	alternativeChain, err := domaincredentials.ClientChainV1Bytes([][]byte{leafDER, sameSubjectADER, intermediateBDER})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := parseClientIdentityV1(
		alternativeChain, keyDER, root, roots, domainsecurity.SHA256Hex(leafDER), domainsecurity.SHA256Hex(keySPKI), now,
	); err == nil {
		t.Fatal("credential chain with multiple verified paths was accepted")
	}

	unknownLeafTemplate := *leafTemplate
	unknownLeafTemplate.SerialNumber = big.NewInt(5)
	unknownLeafTemplate.UnknownExtKeyUsage = []asn1.ObjectIdentifier{{1, 2, 3, 5}}
	unknownLeafDER := createCryptoPolicyCertificateV1(
		t, &unknownLeafTemplate, intermediateB, &leafKey.PublicKey, sharedCAKey,
	)
	unknownLeafChain, err := domaincredentials.ClientChainV1Bytes([][]byte{unknownLeafDER, intermediateBDER})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := parseClientIdentityV1(
		unknownLeafChain, keyDER, root, roots, domainsecurity.SHA256Hex(unknownLeafDER), domainsecurity.SHA256Hex(keySPKI), now,
	); err == nil {
		t.Fatal("client leaf with an unknown extended key usage was accepted")
	}

	unknownIntermediateTemplate := cryptoPolicyCATemplateV1(6, "Unknown EKU Intermediate", now)
	unknownIntermediateTemplate.UnknownExtKeyUsage = []asn1.ObjectIdentifier{{1, 2, 3, 6}}
	unknownIntermediateDER := createCryptoPolicyCertificateV1(
		t, unknownIntermediateTemplate, root, &sharedCAKey.PublicKey, rootKey,
	)
	unknownIntermediate, err := x509.ParseCertificate(unknownIntermediateDER)
	if err != nil {
		t.Fatal(err)
	}
	unknownIntermediateLeafTemplate := *leafTemplate
	unknownIntermediateLeafTemplate.SerialNumber = big.NewInt(7)
	unknownIntermediateLeafDER := createCryptoPolicyCertificateV1(
		t, &unknownIntermediateLeafTemplate, unknownIntermediate, &leafKey.PublicKey, sharedCAKey,
	)
	unknownIntermediateChain, err := domaincredentials.ClientChainV1Bytes(
		[][]byte{unknownIntermediateLeafDER, unknownIntermediateDER},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := parseClientIdentityV1(
		unknownIntermediateChain, keyDER, root, roots,
		domainsecurity.SHA256Hex(unknownIntermediateLeafDER), domainsecurity.SHA256Hex(keySPKI), now,
	); err == nil {
		t.Fatal("client intermediate with an unknown extended key usage was accepted")
	}
}

func TestRootRequiresSelfIssuedSelfSignatureAndClosedPolicy(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	validRoot, _, _ := newCryptoPolicyRootV1(t, now)
	if _, _, err := parseRootV1(validRoot.Raw, domainsecurity.SHA256Hex(validRoot.Raw), now); err != nil {
		t.Fatalf("valid root rejected: %v", err)
	}

	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := cryptoPolicyCATemplateV1(10, "Self Subject", now)
	issuer := *template
	issuer.Subject = pkix.Name{CommonName: "Different Issuer"}
	nonSelfIssuedDER := createCryptoPolicyCertificateV1(t, template, &issuer, &rootKey.PublicKey, rootKey)
	nonSelfIssued, err := x509.ParseCertificate(nonSelfIssuedDER)
	if err != nil {
		t.Fatal(err)
	}
	if err := nonSelfIssued.CheckSignatureFrom(nonSelfIssued); err != nil {
		t.Fatalf("fixture must be signature-compatible with itself: %v", err)
	}
	if _, _, err := parseRootV1(nonSelfIssuedDER, domainsecurity.SHA256Hex(nonSelfIssuedDER), now); err == nil {
		t.Fatal("self-signed but non-self-issued root was accepted")
	}

	unknownEKUTemplate := cryptoPolicyCATemplateV1(11, "Unknown EKU Root", now)
	unknownEKUTemplate.UnknownExtKeyUsage = []asn1.ObjectIdentifier{{1, 2, 3, 4}}
	unknownEKUDER := createCryptoPolicyCertificateV1(
		t, unknownEKUTemplate, unknownEKUTemplate, &rootKey.PublicKey, rootKey,
	)
	if _, _, err := parseRootV1(unknownEKUDER, domainsecurity.SHA256Hex(unknownEKUDER), now); err == nil {
		t.Fatal("root with an unknown extended key usage was accepted")
	}
}

func TestCertificateCryptoPolicyV1(t *testing.T) {
	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil || validateCertificatePublicKeyV1(&ecdsaKey.PublicKey) != nil {
		t.Fatalf("P-256 policy result: keyErr=%v policyErr=%v", err, validateCertificatePublicKeyV1(&ecdsaKey.PublicKey))
	}
	ed25519Public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil || validateCertificatePublicKeyV1(ed25519Public) != nil {
		t.Fatalf("Ed25519 policy result: keyErr=%v policyErr=%v", err, validateCertificatePublicKeyV1(ed25519Public))
	}
	rsa2048, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCertificatePublicKeyV1(&rsa2048.PublicKey); err != nil {
		t.Fatalf("RSA-2048 rejected: %v", err)
	}
	weakRSA, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if validateCertificatePublicKeyV1(&weakRSA.PublicKey) == nil {
		t.Fatal("RSA-1024 was accepted")
	}
	nonCanonicalExponent := rsa2048.PublicKey
	nonCanonicalExponent.E = 3
	if validateCertificatePublicKeyV1(&nonCanonicalExponent) == nil {
		t.Fatal("RSA exponent 3 was accepted")
	}
	if validCertificateSignatureAlgorithmV1(x509.SHA1WithRSA) {
		t.Fatal("SHA-1 certificate signature was accepted")
	}
}

func newCryptoPolicyRootV1(
	t *testing.T,
	now time.Time,
) (*x509.Certificate, *ecdsa.PrivateKey, *x509.CertPool) {
	t.Helper()
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := cryptoPolicyCATemplateV1(1, "Policy Root", now)
	rootDER := createCryptoPolicyCertificateV1(t, template, template, &rootKey.PublicKey, rootKey)
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(root)
	return root, rootKey, roots
}

func cryptoPolicyCATemplateV1(serial int64, commonName string, now time.Time) *x509.Certificate {
	return &x509.Certificate{
		SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: commonName},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		BasicConstraintsValid: true, IsCA: true, KeyUsage: x509.KeyUsageCertSign,
	}
}

func createCryptoPolicyCertificateV1(
	t *testing.T,
	template, parent *x509.Certificate,
	publicKey any,
	parentKey any,
) []byte {
	t.Helper()
	body, err := x509.CreateCertificate(rand.Reader, template, parent, publicKey, parentKey)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
