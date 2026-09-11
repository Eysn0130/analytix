package monotonicheadhttp

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"errors"
)

var errWitnessServerCertificatePolicy = errors.New("monotonic head witness server certificate policy rejected the connection")

// verifyWitnessServerConnectionV1 is an additional closed cryptographic
// policy. Go's ordinary hostname, validity, chain and ServerAuth verification
// remains enabled and must complete before this callback runs.
func verifyWitnessServerConnectionV1(state tls.ConnectionState) error {
	if len(state.VerifiedChains) == 0 {
		return errWitnessServerCertificatePolicy
	}
	for _, chain := range state.VerifiedChains {
		if !validWitnessServerChainV1(chain) {
			return errWitnessServerCertificatePolicy
		}
	}
	return nil
}

func validWitnessServerChainV1(chain []*x509.Certificate) bool {
	if len(chain) < 2 {
		return false
	}
	leaf := chain[0]
	if leaf == nil || !leaf.BasicConstraintsValid || leaf.IsCA ||
		leaf.KeyUsage != x509.KeyUsageDigitalSignature ||
		len(leaf.UnknownExtKeyUsage) != 0 || len(leaf.ExtKeyUsage) != 1 ||
		leaf.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth ||
		!validWitnessCertificateSignatureAlgorithmV1(leaf.SignatureAlgorithm) ||
		!validWitnessPublicKeyV1(leaf.PublicKey) {
		return false
	}
	for index, certificate := range chain[1:] {
		if certificate == nil || !certificate.BasicConstraintsValid || !certificate.IsCA ||
			!validWitnessCAKeyUsageV1(certificate) ||
			!validWitnessCertificateSignatureAlgorithmV1(certificate.SignatureAlgorithm) ||
			!validWitnessPublicKeyV1(certificate.PublicKey) || len(certificate.UnknownExtKeyUsage) != 0 {
			return false
		}
		isRoot := index == len(chain)-2
		if isRoot {
			if len(certificate.ExtKeyUsage) != 0 ||
				!bytes.Equal(certificate.RawIssuer, certificate.RawSubject) ||
				certificate.CheckSignatureFrom(certificate) != nil {
				return false
			}
		} else if len(certificate.ExtKeyUsage) != 0 &&
			(len(certificate.ExtKeyUsage) != 1 || certificate.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth) {
			return false
		}
	}
	for index := 0; index+1 < len(chain); index++ {
		if !bytes.Equal(chain[index].RawIssuer, chain[index+1].RawSubject) ||
			chain[index].CheckSignatureFrom(chain[index+1]) != nil {
			return false
		}
	}
	return true
}

func validWitnessCAKeyUsageV1(certificate *x509.Certificate) bool {
	return certificate != nil && certificate.KeyUsage&x509.KeyUsageCertSign != 0 &&
		certificate.KeyUsage & ^(x509.KeyUsageCertSign|x509.KeyUsageCRLSign) == 0
}

func validWitnessPublicKeyV1(publicKey any) bool {
	switch key := publicKey.(type) {
	case *rsa.PublicKey:
		return key != nil && key.N != nil &&
			(key.N.BitLen() == 2048 || key.N.BitLen() == 3072 || key.N.BitLen() == 4096) && key.E == 65537
	case *ecdsa.PublicKey:
		return key != nil && (key.Curve == elliptic.P256() || key.Curve == elliptic.P384() || key.Curve == elliptic.P521()) &&
			key.X != nil && key.Y != nil && key.Curve.IsOnCurve(key.X, key.Y)
	case ed25519.PublicKey:
		return len(key) == ed25519.PublicKeySize
	default:
		return false
	}
}

func validWitnessCertificateSignatureAlgorithmV1(algorithm x509.SignatureAlgorithm) bool {
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
