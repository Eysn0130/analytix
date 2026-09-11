package steeringauthority

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
)

type authorityStub struct {
	keyID   string
	private ed25519.PrivateKey
	public  ed25519.PublicKey
}

func newAuthorityStub(seed byte) *authorityStub {
	private := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize))
	public := private.Public().(ed25519.PublicKey)
	return &authorityStub{keyID: domainsecurity.SHA256Hex(public), private: private, public: public}
}

func (authority *authorityStub) KeyID() string     { return authority.keyID }
func (authority *authorityStub) PublicKey() []byte { return append([]byte(nil), authority.public...) }
func (authority *authorityStub) Sign(ctx context.Context, body []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ed25519.Sign(authority.private, body), nil
}
func (authority *authorityStub) VerifyTrusted(ctx context.Context, keyID string, public, body, signature []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if keyID != authority.keyID || !bytes.Equal(public, authority.public) || !ed25519.Verify(authority.public, body, signature) {
		return errors.New("untrusted steering authority")
	}
	return nil
}

func TestServiceSealsAndVerifiesOnlyInstallationTrustedSteering(t *testing.T) {
	contextDigest := domainsecurity.SHA256Hex([]byte("steering-context"))
	entry, err := domainsteering.BindPendingEntryV1(map[string]any{
		"id": domainsteering.EntryIDV1("turn-1", "client-1"), "clientUserMessageId": "client-1",
		"text": "authorized guidance", "admittedAt": "2026-07-18T01:02:03Z", "delivery": "steer",
	}, contextDigest)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(newAuthorityStub(0x41))
	sealed, err := service.Seal(context.Background(), entry, contextDigest)
	if err != nil || service.Verify(context.Background(), sealed, contextDigest) != nil {
		t.Fatalf("installation authority did not round-trip: sealed=%#v err=%v", sealed, err)
	}
	tampered := make(map[string]any, len(sealed))
	for key, value := range sealed {
		tampered[key] = value
	}
	tampered["text"] = "fabricated guidance"
	if service.Verify(context.Background(), tampered, contextDigest) == nil {
		t.Fatal("tampered steering retained installation authority")
	}
	if NewService(newAuthorityStub(0x57)).Verify(context.Background(), sealed, contextDigest) == nil {
		t.Fatal("foreign installation accepted steering authority")
	}
	promoted := make(map[string]any, len(sealed)+3)
	for key, value := range sealed {
		promoted[key] = value
	}
	promoted["status"] = "promoted"
	promoted["promotedAt"] = "2026-07-18T01:02:04Z"
	promoted["promotedItemId"] = promoted["id"]
	promoted, err = service.Promote(context.Background(), promoted, contextDigest)
	if err != nil || service.Verify(context.Background(), promoted, contextDigest) != nil {
		t.Fatalf("installation promotion authority did not round-trip: promoted=%#v err=%v", promoted, err)
	}
	if NewService(newAuthorityStub(0x57)).Verify(context.Background(), promoted, contextDigest) == nil {
		t.Fatal("foreign installation accepted steering promotion")
	}
}

func TestServiceFailsClosedWithoutAuthorityOrLiveContext(t *testing.T) {
	if NewService(nil) != nil {
		t.Fatal("nil authority created a service")
	}
	service := NewService(newAuthorityStub(0x41))
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Seal(cancelled, map[string]any{}, domainsecurity.SHA256Hex([]byte("cancelled-context"))); err == nil {
		t.Fatal("cancelled authority context was ignored")
	}
}
