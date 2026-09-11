package authorityadvancefs

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"testing"

	authorityadvanceapp "analytix.local/runtime-go/internal/app/authorityadvance"
	domainauthority "analytix.local/runtime-go/internal/domain/authorityadvance"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

type preparedJournalVerifierV2 struct{ fixture journalFixtureV2 }

func (a preparedJournalVerifierV2) KeyID() string { return a.fixture.authorityKeyID }
func (a preparedJournalVerifierV2) PublicKey() []byte {
	return append([]byte(nil), a.fixture.authorityPub...)
}
func (a preparedJournalVerifierV2) Sign(context.Context, []byte) ([]byte, error) {
	return nil, errors.New("verification only")
}
func (a preparedJournalVerifierV2) VerifyTrusted(ctx context.Context, keyID string, publicKey, body, signature []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if keyID != a.KeyID() || !bytes.Equal(publicKey, a.PublicKey()) || !ed25519.Verify(a.fixture.authorityPub, body, signature) {
		return errors.New("foreign journal authority")
	}
	return nil
}

func TestPreparedAuthorityAdvanceInventoryPinsImmutableCompleteJournal(t *testing.T) {
	ctx := context.Background()
	fixture := newJournalFixtureV2(t, "prepared-snapshot")
	intent := fixture.intent(t, "policy")
	settlement := fixture.settlement(t, intent)
	root := t.TempDir()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutIntentIfAbsent(ctx, intent); err != nil {
		t.Fatal(err)
	}
	if err := store.PutSettlementIfAbsent(ctx, settlement); err != nil {
		t.Fatal(err)
	}
	if err := errors.Join(store.intents.Close(), store.settlements.Close()); err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareRecoveryV1(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := prepared.SnapshotInventory(ctx)
	if err != nil || !snapshot.HasRecords() {
		t.Fatalf("prepared journal snapshot: %v", err)
	}
	if err := authorityadvanceapp.VerifyTrustedInventoryV2(ctx, snapshot, snapshot, preparedJournalVerifierV2{fixture}); err != nil {
		t.Fatal(err)
	}
	if err := authorityadvanceapp.VerifyTrustedInventoryV2(ctx, snapshot, snapshot, preparedJournalVerifierV2{newJournalFixtureV2(t, "foreign-snapshot")}); err == nil {
		t.Fatal("foreign key accepted prepared journal")
	}
	intents, settlements := 0, 0
	if err := snapshot.VisitIntents(ctx, func(actual domainauthority.MonotonicAdvanceIntentV2) error {
		intents++
		if !sameIntentBytesV2(actual, intent) {
			t.Fatal("prepared intent bytes changed")
		}
		actual.AuthoritySignature = "caller mutation"
		actual.Transition.ThreadRiskGenesis.FirstIndex.Entries[0].ThreadID = "caller mutation"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.VisitSettlements(ctx, func(actual domainauthority.MonotonicAdvanceSettlementV2) error {
		settlements++
		if !sameSettlementBytesV2(actual, settlement) {
			t.Fatal("prepared settlement bytes changed")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if intents != 1 || settlements != 1 {
		t.Fatal("prepared journal denominator incomplete")
	}
	if err := authorityadvanceapp.VerifyTrustedInventoryV2(ctx, snapshot, snapshot, preparedJournalVerifierV2{fixture}); err != nil {
		t.Fatal("visitor changed immutable journal")
	}
	other := newJournalFixtureV2(t, "new-record")
	added := other.intent(t, "policy")
	path := filepath.Join(root, "v2", "intents", added.MutationID[:2], added.MutationID+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	body, err := domainauthority.MonotonicAdvanceIntentV2Bytes(added)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.SnapshotInventory(ctx); err == nil {
		t.Fatal("new record omitted from frozen denominator")
	}
}

func TestPreparedAuthorityAdvanceAbsentInventoryDoesNotCreateStore(t *testing.T) {
	root := filepath.Join(t.TempDir(), "journal")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareRecoveryV1(context.Background(), root, access)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := prepared.SnapshotInventory(context.Background())
	if err != nil || snapshot.HasRecords() {
		t.Fatalf("absent journal snapshot: %v", err)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("prepared snapshot created missing owner")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := prepared.SnapshotInventory(ctx); err == nil {
		t.Fatal("cancelled snapshot returned inventory")
	}
}
