package piiauthorization

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	desktopControlledHostTLSRootMaxBytesV2 = 2 << 10
	desktopControlledHostTLSALPNV2         = "http/1.1"
)

var errDesktopControlledHostTLSPolicyV2 = errors.New("controlled artifact host TLS policy rejected the connection")

func validateDesktopControlledHostOriginV2(raw string) (string, string, error) {
	if raw == "" || raw != strings.TrimSpace(raw) {
		return "", "", ErrDesktopControlledHostUnavailable
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Hostname() != "127.0.0.1" ||
		parsed.Path != "" || parsed.RawPath != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return "", "", ErrDesktopControlledHostUnavailable
	}
	host, portText, err := net.SplitHostPort(parsed.Host)
	port, portErr := strconv.Atoi(portText)
	if err != nil || portErr != nil || host != "127.0.0.1" || port < 1 || port > 65535 ||
		portText != strconv.Itoa(port) || parsed.String() != raw {
		return "", "", ErrDesktopControlledHostUnavailable
	}
	return raw, net.JoinHostPort(host, portText), nil
}

func newDesktopControlledHostTLSConfigV2(rootDER []byte, leafSPKISHA256 string) (*tls.Config, error) {
	rootDER = append([]byte(nil), rootDER...)
	leafSPKISHA256 = strings.TrimSpace(leafSPKISHA256)
	pin, pinErr := hex.DecodeString(leafSPKISHA256)
	root, rootErr := x509.ParseCertificate(rootDER)
	now := time.Now().UTC()
	if rootErr != nil || pinErr != nil || len(rootDER) == 0 || len(rootDER) > desktopControlledHostTLSRootMaxBytesV2 ||
		len(pin) != 32 || !domainsecurity.IsSHA256Hex(leafSPKISHA256) || !bytes.Equal(root.Raw, rootDER) ||
		!validDesktopControlledHostTLSRootV2(root, now) {
		clearDesktopControlledHostBytesV1(pin)
		return nil, ErrDesktopControlledHostUnavailable
	}
	roots := x509.NewCertPool()
	roots.AddCert(root)
	return &tls.Config{
		MinVersion: tls.VersionTLS13,
		MaxVersion: tls.VersionTLS13,
		RootCAs:    roots,
		ServerName: "127.0.0.1",
		NextProtos: []string{desktopControlledHostTLSALPNV2},
		VerifyConnection: func(state tls.ConnectionState) error {
			return verifyDesktopControlledHostTLSConnectionV2(state, root, pin)
		},
	}, nil
}

func verifyDesktopControlledHostTLSConnectionV2(
	state tls.ConnectionState,
	root *x509.Certificate,
	expectedLeafSPKI []byte,
) error {
	// VerifyConnection runs inside the handshake before HTTP can send headers or
	// a body, so HandshakeComplete is intentionally not required here.
	if root == nil || state.Version != tls.VersionTLS13 ||
		state.NegotiatedProtocol != desktopControlledHostTLSALPNV2 || len(state.PeerCertificates) != 1 ||
		len(state.VerifiedChains) != 1 || len(state.VerifiedChains[0]) != 2 {
		return errDesktopControlledHostTLSPolicyV2
	}
	leaf := state.PeerCertificates[0]
	chain := state.VerifiedChains[0]
	if leaf == nil || chain[0] == nil || chain[1] == nil || !bytes.Equal(chain[0].Raw, leaf.Raw) ||
		!bytes.Equal(chain[1].Raw, root.Raw) || !validDesktopControlledHostTLSLeafV2(leaf) ||
		!validDesktopControlledHostTLSRootV2(root, time.Now().UTC()) || leaf.CheckSignatureFrom(root) != nil {
		return errDesktopControlledHostTLSPolicyV2
	}
	leafSPKI := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	if len(expectedLeafSPKI) != len(leafSPKI) || subtle.ConstantTimeCompare(expectedLeafSPKI, leafSPKI[:]) != 1 {
		return errDesktopControlledHostTLSPolicyV2
	}
	return nil
}

func validDesktopControlledHostTLSRootV2(root *x509.Certificate, now time.Time) bool {
	if root == nil || !root.BasicConstraintsValid || !root.IsCA ||
		root.KeyUsage != (x509.KeyUsageCertSign|x509.KeyUsageCRLSign) || len(root.ExtKeyUsage) != 0 ||
		len(root.UnknownExtKeyUsage) != 0 || len(root.UnhandledCriticalExtensions) != 0 ||
		len(root.DNSNames) != 0 || len(root.IPAddresses) != 0 || len(root.EmailAddresses) != 0 || len(root.URIs) != 0 ||
		root.SignatureAlgorithm != x509.ECDSAWithSHA256 || !validDesktopControlledHostTLSPublicKeyV2(root.PublicKey) ||
		now.Before(root.NotBefore) || !now.Before(root.NotAfter) || !bytes.Equal(root.RawIssuer, root.RawSubject) ||
		root.CheckSignatureFrom(root) != nil {
		return false
	}
	return true
}

func validDesktopControlledHostTLSLeafV2(leaf *x509.Certificate) bool {
	loopback := net.ParseIP("127.0.0.1")
	return leaf != nil && leaf.BasicConstraintsValid && !leaf.IsCA && leaf.KeyUsage == x509.KeyUsageDigitalSignature &&
		len(leaf.ExtKeyUsage) == 1 && leaf.ExtKeyUsage[0] == x509.ExtKeyUsageServerAuth &&
		len(leaf.UnknownExtKeyUsage) == 0 && len(leaf.UnhandledCriticalExtensions) == 0 &&
		len(leaf.DNSNames) == 0 && len(leaf.IPAddresses) == 1 && leaf.IPAddresses[0].Equal(loopback) &&
		len(leaf.EmailAddresses) == 0 && len(leaf.URIs) == 0 && leaf.SignatureAlgorithm == x509.ECDSAWithSHA256 &&
		validDesktopControlledHostTLSPublicKeyV2(leaf.PublicKey)
}

func validDesktopControlledHostTLSPublicKeyV2(value any) bool {
	key, ok := value.(*ecdsa.PublicKey)
	return ok && key != nil && key.Curve == elliptic.P256() && key.X != nil && key.Y != nil && key.Curve.IsOnCurve(key.X, key.Y)
}
