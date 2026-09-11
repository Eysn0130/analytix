package turnterminal

import (
	"context"
	"crypto/ed25519"
	"errors"

	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	turnterminalstoreport "analytix.local/runtime-go/internal/ports/turnterminalstore"
	terminaltest "analytix.local/runtime-go/internal/testsupport/turnterminal"
)

type trustedInventoryStoreV1 struct {
	intents      []domainturnterminal.TurnTerminalIntentV1
	dispositions []domainturnterminal.TurnTerminalDispositionV1
}

func (store trustedInventoryStoreV1) PutIntentIfAbsent(context.Context, domainturnterminal.TurnTerminalIntentV1) error {
	return errors.New("not implemented")
}
func (store trustedInventoryStoreV1) ReadIntent(context.Context, string) (domainturnterminal.TurnTerminalIntentV1, error) {
	return domainturnterminal.TurnTerminalIntentV1{}, turnterminalstoreport.ErrNotFound
}
func (store trustedInventoryStoreV1) VisitIntents(_ context.Context, visit func(domainturnterminal.TurnTerminalIntentV1) error) error {
	for _, intent := range store.intents {
		if err := visit(intent); err != nil {
			return err
		}
	}
	return nil
}
func (store trustedInventoryStoreV1) PutDispositionIfAbsent(context.Context, domainturnterminal.TurnTerminalDispositionV1) error {
	return errors.New("not implemented")
}
func (store trustedInventoryStoreV1) ReadDisposition(context.Context, string) (domainturnterminal.TurnTerminalDispositionV1, error) {
	return domainturnterminal.TurnTerminalDispositionV1{}, turnterminalstoreport.ErrNotFound
}
func (store trustedInventoryStoreV1) VisitDispositions(_ context.Context, visit func(domainturnterminal.TurnTerminalDispositionV1) error) error {
	for _, disposition := range store.dispositions {
		if err := visit(disposition); err != nil {
			return err
		}
	}
	return nil
}
func (store trustedInventoryStoreV1) HasRecords(context.Context) (bool, error) {
	return len(store.intents)+len(store.dispositions) != 0, nil
}

type trustedInventoryAuthorityV1 struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	keyID      string
}

func newTrustedInventoryAuthorityV1(privateKey ed25519.PrivateKey) trustedInventoryAuthorityV1 {
	publicKey := privateKey.Public().(ed25519.PublicKey)
	return trustedInventoryAuthorityV1{privateKey: privateKey, publicKey: publicKey, keyID: domainsecurity.SHA256Hex(publicKey)}
}
func (authority trustedInventoryAuthorityV1) KeyID() string { return authority.keyID }
func (authority trustedInventoryAuthorityV1) PublicKey() []byte {
	return append([]byte(nil), authority.publicKey...)
}
func (authority trustedInventoryAuthorityV1) Sign(_ context.Context, message []byte) ([]byte, error) {
	return ed25519.Sign(authority.privateKey, message), nil
}
func (authority trustedInventoryAuthorityV1) VerifyTrusted(_ context.Context, keyID string, publicKey, message, signature []byte) error {
	if keyID != authority.keyID || !ed25519.PublicKey(publicKey).Equal(authority.publicKey) ||
		!ed25519.Verify(authority.publicKey, message, signature) {
		return errors.New("authority mismatch")
	}
	return nil
}

func TestVerifyTrustedInventoryAnchorsEveryRecordToInstallationAuthority(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	store := trustedInventoryStoreV1{
		intents:      []domainturnterminal.TurnTerminalIntentV1{fixture.Intent},
		dispositions: []domainturnterminal.TurnTerminalDispositionV1{fixture.Disposition},
	}
	authority := newTrustedInventoryAuthorityV1(fixture.PrivateKey)
	if err := VerifyTrustedInventoryV1(context.Background(), store, authority); err != nil {
		t.Fatal(err)
	}
	otherKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	if err := VerifyTrustedInventoryV1(context.Background(), store, newTrustedInventoryAuthorityV1(otherKey)); err == nil {
		t.Fatal("self-signed terminal inventory from another authority was trusted")
	}
}
