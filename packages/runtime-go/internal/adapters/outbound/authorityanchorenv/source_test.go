package authorityanchorenv

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestSourceLoadsCanonicalIndependentAnchor(t *testing.T) {
	publicKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)).Public().(ed25519.PublicKey)
	envelope := envelopeV1{
		SchemaVersion: 1, InstallationID: domainsecurity.SHA256Hex([]byte("installation")),
		AuthorityKeyID:        domainsecurity.SHA256Hex(publicKey),
		AuthorityPublicKey:    base64.RawURLEncoding.EncodeToString(publicKey),
		CurrentManifestDigest: domainsecurity.SHA256Hex([]byte("manifest")),
	}
	raw := marshalEnvelope(t, envelope)
	calls := 0
	source := Source{Lookup: func(name string) (string, bool) {
		calls++
		return raw, name == AnchorEnvelopeV1Variable
	}}
	anchor, err := source.Load(context.Background())
	if err != nil || calls != 1 || anchor.InstallationID != envelope.InstallationID ||
		anchor.AuthorityKeyID != envelope.AuthorityKeyID || anchor.CurrentManifestDigest != envelope.CurrentManifestDigest ||
		string(anchor.AuthorityPublicKey) != string(publicKey) {
		t.Fatalf("independent anchor mismatch: anchor=%#v err=%v", anchor, err)
	}
	anchor.AuthorityPublicKey[0] ^= 0xff
	second, err := source.Load(context.Background())
	if err != nil || string(second.AuthorityPublicKey) != string(publicKey) {
		t.Fatal("source returned mutable shared authority key bytes")
	}
}

func TestSourceFailsClosedOnPartialOrNoncanonicalAnchor(t *testing.T) {
	publicKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)).Public().(ed25519.PublicKey)
	valid := envelopeV1{
		SchemaVersion: 1, InstallationID: domainsecurity.SHA256Hex([]byte("installation")),
		AuthorityKeyID:        domainsecurity.SHA256Hex(publicKey),
		AuthorityPublicKey:    base64.RawURLEncoding.EncodeToString(publicKey),
		CurrentManifestDigest: domainsecurity.SHA256Hex([]byte("manifest")),
	}
	for name, raw := range map[string]string{
		"wrong key id": marshalEnvelope(t, func(value envelopeV1) envelopeV1 {
			value.AuthorityKeyID = domainsecurity.SHA256Hex([]byte("wrong"))
			return value
		}(valid)),
		"padded key":             marshalEnvelope(t, func(value envelopeV1) envelopeV1 { value.AuthorityPublicKey += "="; return value }(valid)),
		"surrounding whitespace": marshalEnvelope(t, valid) + " ",
		"digest whitespace": marshalEnvelope(t, func(value envelopeV1) envelopeV1 {
			value.InstallationID = " " + value.InstallationID
			return value
		}(valid)),
		"digest escaped newline": marshalEnvelope(t, func(value envelopeV1) envelopeV1 {
			value.CurrentManifestDigest += "\n"
			return value
		}(valid)),
		"unknown field":      strings.TrimSuffix(marshalEnvelope(t, valid), "}") + `,"unknown":true}`,
		"noncanonical json":  strings.Replace(marshalEnvelope(t, valid), `{"schemaVersion":1`, `{ "schemaVersion":1`, 1),
		"trailing object":    marshalEnvelope(t, valid) + `{}`,
		"trailing scalar":    marshalEnvelope(t, valid) + `true`,
		"trailing malformed": marshalEnvelope(t, valid) + `junk`,
	} {
		t.Run(name, func(t *testing.T) {
			source := Source{Lookup: func(string) (string, bool) { return raw, true }}
			if _, err := source.Load(context.Background()); !errors.Is(err, ErrInvalid) {
				t.Fatalf("invalid anchor did not fail closed: %v", err)
			}
		})
	}
	if _, err := (Source{Lookup: func(string) (string, bool) { return "", false }}).Load(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("absent anchor classification = %v", err)
	}
	if _, err := (Source{Lookup: func(string) (string, bool) { return "", true }}).Load(context.Background()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("present empty anchor classification = %v", err)
	}
}

func TestSourceHonorsCancellationAfterAtomicLookup(t *testing.T) {
	publicKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)).Public().(ed25519.PublicKey)
	raw := marshalEnvelope(t, envelopeV1{
		SchemaVersion: 1, InstallationID: domainsecurity.SHA256Hex([]byte("installation")),
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
		CurrentManifestDigest: domainsecurity.SHA256Hex([]byte("manifest")),
	})
	ctx, cancel := context.WithCancel(context.Background())
	source := Source{Lookup: func(string) (string, bool) {
		cancel()
		return raw, true
	}}
	if _, err := source.Load(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation during anchor lookup was not preserved: %v", err)
	}
}

func marshalEnvelope(t *testing.T, envelope envelopeV1) string {
	t.Helper()
	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
