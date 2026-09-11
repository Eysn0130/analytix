package monotonicheadhttp

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"analytix.local/runtime-go/internal/ports/monotonichead"
)

func TestWitnessServerCertificatePolicyV1(t *testing.T) {
	chain := newWitnessServerPolicyChainV1(t)
	if !validWitnessServerChainV1(chain) {
		t.Fatal("valid closed-policy witness server chain was rejected")
	}

	tests := map[string]func([]*x509.Certificate){
		"self signed leaf": func(candidate []*x509.Certificate) {
			candidate[0] = candidate[1]
			candidate = candidate[:1]
		},
		"leaf unknown eku": func(candidate []*x509.Certificate) {
			leaf := *candidate[0]
			leaf.UnknownExtKeyUsage = []asn1.ObjectIdentifier{{1, 2, 3, 7}}
			candidate[0] = &leaf
		},
		"leaf extra key usage": func(candidate []*x509.Certificate) {
			leaf := *candidate[0]
			leaf.KeyUsage |= x509.KeyUsageKeyEncipherment
			candidate[0] = &leaf
		},
		"root constrained eku": func(candidate []*x509.Certificate) {
			root := *candidate[len(candidate)-1]
			root.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
			candidate[len(candidate)-1] = &root
		},
		"sha1 signature": func(candidate []*x509.Certificate) {
			leaf := *candidate[0]
			leaf.SignatureAlgorithm = x509.SHA1WithRSA
			candidate[0] = &leaf
		},
		"weak rsa leaf": func(candidate []*x509.Certificate) {
			weak, err := rsa.GenerateKey(rand.Reader, 1024)
			if err != nil {
				t.Fatal(err)
			}
			leaf := *candidate[0]
			leaf.PublicKey = &weak.PublicKey
			candidate[0] = &leaf
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := append([]*x509.Certificate(nil), chain...)
			mutate(candidate)
			if validWitnessServerChainV1(candidate) {
				t.Fatal("unsafe witness server chain was accepted")
			}
		})
	}
	if validWitnessServerChainV1(chain[:1]) {
		t.Fatal("single self-signed witness server certificate was accepted")
	}
	invalidLeaf := *chain[0]
	invalidLeaf.KeyUsage |= x509.KeyUsageKeyEncipherment
	if err := verifyWitnessServerConnectionV1(tls.ConnectionState{
		VerifiedChains: [][]*x509.Certificate{chain, {&invalidLeaf, chain[1]}},
	}); err == nil {
		t.Fatal("connection with one valid and one invalid verified chain was accepted")
	}
}

func TestClientWiresClosedWitnessServerCertificatePolicy(t *testing.T) {
	fixture := newWitnessFixture(t)
	var handlerCalls atomic.Int32
	server, roots := newServerWithTLSLeafMutation(
		t,
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { handlerCalls.Add(1) }),
		tls.VersionTLS13,
		0,
		func(leaf *x509.Certificate) { leaf.KeyUsage |= x509.KeyUsageKeyEncipherment },
	)
	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Certificate().Verify(x509.VerifyOptions{
		Roots: roots, CurrentTime: time.Now(), DNSName: parsedURL.Hostname(),
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}); err != nil {
		t.Fatalf("fixture must pass Go's standard TLS verification: %v", err)
	}
	client := fixture.client(t, server.URL, roots)
	if _, err := client.Observe(context.Background(), fixture.observeRequest(t, "closed-server-policy")); !errors.Is(err, monotonichead.ErrUnavailable) {
		t.Fatalf("closed server certificate policy classification = %v", err)
	}
	if handlerCalls.Load() != 0 {
		t.Fatalf("closed server certificate policy reached HTTP handler: calls=%d", handlerCalls.Load())
	}
}

func newWitnessServerPolicyChainV1(t *testing.T) []*x509.Certificate {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	rootTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Witness Policy Root"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		BasicConstraintsValid: true, IsCA: true, KeyUsage: x509.KeyUsageCertSign,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatal(err)
	}
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "127.0.0.1"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, root, &serverKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	server, err := x509.ParseCertificate(serverDER)
	if err != nil {
		t.Fatal(err)
	}
	return []*x509.Certificate{server, root}
}
