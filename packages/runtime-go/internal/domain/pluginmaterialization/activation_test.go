package pluginmaterialization

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func activationFixtureV1(t *testing.T) (ReceiptV1, string, []byte, SignFuncV1, time.Time) {
	t.Helper()
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	seed := sha256.Sum256([]byte("synthetic-plugin-activation-authority"))
	private := ed25519.NewKeyFromSeed(seed[:])
	public := private.Public().(ed25519.PublicKey)
	keyID := sha256Hex(public)
	sign := func(body []byte) ([]byte, error) { return ed25519.Sign(private, body), nil }
	receipt, err := NewReceiptV1(testIntent(t, now), strings.Repeat("b", 64), "plugins/cache/analytix-hub/analytix-fund-analysis/0.16.16", now, keyID, public, sign)
	if err != nil {
		t.Fatal(err)
	}
	return receipt, keyID, public, sign, now
}

func TestActivationCanonicalSignatureAndCurrentReceiptBindingV1(t *testing.T) {
	receipt, keyID, public, sign, now := activationFixtureV1(t)
	activation, err := NewActivationV1(receipt, 1, DesiredDisabledV1, now, keyID, public, sign)
	if err != nil {
		t.Fatal(err)
	}
	body, err := ActivationV1Bytes(activation)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseActivationV1(body)
	if err != nil || parsed != activation {
		t.Fatalf("activation did not round trip: %v", err)
	}
	if err := ValidateTrustedActivationForReceiptV1(parsed, receipt, keyID, public); err != nil {
		t.Fatal(err)
	}
	otherReceipt, err := NewReceiptV1(testIntent(t, now), strings.Repeat("c", 64), receipt.ActiveRelativePath, now, keyID, public, sign)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateTrustedActivationForReceiptV1(parsed, otherReceipt, keyID, public) == nil {
		t.Fatal("accepted another generation")
	}
	otherPublic := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{2}, ed25519.SeedSize)).Public().(ed25519.PublicKey)
	if ValidateTrustedActivationForReceiptV1(parsed, receipt, sha256Hex(otherPublic), otherPublic) == nil {
		t.Fatal("accepted another installation authority")
	}
	for _, invalid := range [][]byte{
		append(append([]byte(nil), body...), '\n'),
		bytes.Replace(body, []byte(`"revision":1`), []byte(`"revision":1,"revision":1`), 1),
		bytes.Replace(body, []byte(`"desiredState":"disabled"`), []byte(`"desiredState":"enabled"`), 1),
	} {
		if _, err := ParseActivationV1(invalid); err == nil {
			t.Fatal("accepted tampered or noncanonical activation")
		}
	}
	activation.AuthoritySignature = base64.RawURLEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	badSignature, _ := json.Marshal(activation)
	if _, err := ParseActivationV1(badSignature); err == nil {
		t.Fatal("accepted invalid signature")
	}
	if _, err := NewActivationV1(receipt, 0, DesiredDisabledV1, now, keyID, public, sign); err == nil {
		t.Fatal("accepted revision zero")
	}
	if _, err := NewActivationV1(receipt, 1, "running", now, keyID, public, sign); err == nil {
		t.Fatal("accepted runtime state as desired state")
	}
}
