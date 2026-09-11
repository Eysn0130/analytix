package piiauthorization

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type desktopControlledHostTLSTestIdentityV2 struct {
	rootDER []byte
	leafDER []byte
	leafPin string
	server  tls.Certificate
}

type desktopControlledHostTLSTestServerV2 struct {
	*httptest.Server
	identity desktopControlledHostTLSTestIdentityV2
}

func newDesktopControlledHostTLSTestServerV2(
	t *testing.T,
	handler http.Handler,
) *desktopControlledHostTLSTestServerV2 {
	t.Helper()
	identity := newDesktopControlledHostTLSTestIdentityV2(t)
	server := httptest.NewUnstartedServer(handler)
	server.EnableHTTP2 = false
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{identity.server}, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13,
		NextProtos: []string{desktopControlledHostTLSALPNV2},
	}
	server.StartTLS()
	return &desktopControlledHostTLSTestServerV2{Server: server, identity: identity}
}

func (server *desktopControlledHostTLSTestServerV2) clientConfig(
	fixture *desktopControlledHostFixtureV2,
) DesktopControlledHostConfigV2 {
	return DesktopControlledHostConfigV2{
		Origin: server.URL, Secret: fixture.secret, BackendGeneration: fixture.receipt.BackendGeneration,
		TLSRootCertificateDER: append([]byte(nil), server.identity.rootDER...),
		TLSLeafSPKISHA256:     server.identity.leafPin,
	}
}

func newDesktopControlledHostTLSTestIdentityV2(t *testing.T) desktopControlledHostTLSTestIdentityV2 {
	t.Helper()
	now := time.Now().UTC()
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	rootTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "analytix controlled host test root"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), BasicConstraintsValid: true, IsCA: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign, SignatureAlgorithm: x509.ECDSAWithSHA256,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatal(err)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "analytix controlled host test leaf"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, SignatureAlgorithm: x509.ECDSAWithSHA256,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, root, &leafKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatal(err)
	}
	pin := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	return desktopControlledHostTLSTestIdentityV2{
		rootDER: rootDER, leafDER: leafDER, leafPin: domainSecurityHexForTLSTestV2(pin[:]),
		server: tls.Certificate{Certificate: [][]byte{leafDER}, PrivateKey: leafKey, Leaf: leaf},
	}
}

func domainSecurityHexForTLSTestV2(value []byte) string {
	const digits = "0123456789abcdef"
	encoded := make([]byte, len(value)*2)
	for index, current := range value {
		encoded[index*2] = digits[current>>4]
		encoded[index*2+1] = digits[current&0x0f]
	}
	return string(encoded)
}
