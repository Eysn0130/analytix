package runtimeapp

import (
	"context"
	"crypto/ed25519"
	"errors"
	"net/http"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestFinalPublicationAuthorityIdentityIsValidatedClonedAndPreservesLifecycle(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := &finalPublicationIdentityLifecycle{}
	authority := &finalPublicationIdentityVerifier{
		keyID:     domainsecurity.SHA256Hex(publicKey),
		publicKey: append([]byte(nil), publicKey...),
	}

	bound, err := bindFinalPublicationAuthorityIdentityV1(lifecycle, authority)
	if err != nil {
		t.Fatal(err)
	}
	source, ok := bound.(FinalPublicationAuthorityIdentitySourceV1)
	if !ok {
		t.Fatal("bound handler did not expose final publication authority identity")
	}
	identity, err := source.FinalPublicationAuthorityIdentityV1()
	if err != nil {
		t.Fatal(err)
	}
	if identity.KeyID != authority.keyID || string(identity.PublicKey) != string(publicKey) {
		t.Fatal("bound handler returned the wrong final publication authority identity")
	}
	identity.PublicKey[0] ^= 0xff
	second, err := source.FinalPublicationAuthorityIdentityV1()
	if err != nil {
		t.Fatal(err)
	}
	if string(second.PublicKey) != string(publicKey) {
		t.Fatal("caller mutation changed the frozen final publication authority identity")
	}

	shutdown, ok := bound.(interface{ Shutdown(context.Context) error })
	if !ok {
		t.Fatal("bound handler lost the runtime lifecycle")
	}
	if err := shutdown.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if lifecycle.shutdownCalls != 1 {
		t.Fatalf("bound handler did not forward shutdown: calls=%d", lifecycle.shutdownCalls)
	}
}

func TestFinalPublicationAuthorityIdentityRejectsUntrustedBindings(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name      string
		handler   http.Handler
		authority *finalPublicationIdentityVerifier
	}{
		{name: "missing handler", authority: &finalPublicationIdentityVerifier{keyID: domainsecurity.SHA256Hex(publicKey), publicKey: publicKey}},
		{name: "missing authority", handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})},
		{name: "wrong key id", handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authority: &finalPublicationIdentityVerifier{keyID: domainsecurity.SHA256Hex([]byte("foreign")), publicKey: publicKey}},
		{name: "wrong key length", handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authority: &finalPublicationIdentityVerifier{keyID: domainsecurity.SHA256Hex(publicKey[:8]), publicKey: publicKey[:8]}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := bindFinalPublicationAuthorityIdentityV1(testCase.handler, testCase.authority); err == nil {
				t.Fatal("invalid final publication authority binding was accepted")
			}
		})
	}
}

func TestOwnedPersistenceLeaseForwardsFinalPublicationAuthorityIdentity(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	identity := FinalPublicationAuthorityIdentityV1{
		KeyID:     domainsecurity.SHA256Hex(publicKey),
		PublicKey: publicKey,
	}
	owned := &ownedPersistenceLeaseHandler{Handler: &finalPublicationIdentitySource{identity: identity}}

	got, err := owned.FinalPublicationAuthorityIdentityV1()
	if err != nil {
		t.Fatal(err)
	}
	if got.KeyID != identity.KeyID || string(got.PublicKey) != string(identity.PublicKey) {
		t.Fatal("owned persistence handler did not preserve final publication authority identity")
	}
}

type finalPublicationIdentityVerifier struct {
	keyID     string
	publicKey []byte
}

func (verifier *finalPublicationIdentityVerifier) KeyID() string {
	if verifier == nil {
		return ""
	}
	return verifier.keyID
}

func (verifier *finalPublicationIdentityVerifier) PublicKey() []byte {
	if verifier == nil {
		return nil
	}
	return append([]byte(nil), verifier.publicKey...)
}

func (*finalPublicationIdentityVerifier) VerifyTrusted(context.Context, string, []byte, []byte, []byte) error {
	return errors.New("not used by identity binding tests")
}

type finalPublicationIdentityLifecycle struct {
	shutdownCalls int
}

func (*finalPublicationIdentityLifecycle) ServeHTTP(http.ResponseWriter, *http.Request) {}

func (lifecycle *finalPublicationIdentityLifecycle) Shutdown(context.Context) error {
	lifecycle.shutdownCalls++
	return nil
}

type finalPublicationIdentitySource struct {
	identity FinalPublicationAuthorityIdentityV1
}

func (*finalPublicationIdentitySource) ServeHTTP(http.ResponseWriter, *http.Request) {}

func (source *finalPublicationIdentitySource) FinalPublicationAuthorityIdentityV1() (FinalPublicationAuthorityIdentityV1, error) {
	return FinalPublicationAuthorityIdentityV1{
		KeyID: source.identity.KeyID, PublicKey: append([]byte(nil), source.identity.PublicKey...),
	}, nil
}
