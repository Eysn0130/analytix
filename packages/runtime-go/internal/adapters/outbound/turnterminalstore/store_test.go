package turnterminalstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	turnterminalstoreport "analytix.local/runtime-go/internal/ports/turnterminalstore"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	terminaltest "analytix.local/runtime-go/internal/testsupport/turnterminal"
)

func TestStorePersistsOneExactTerminalTransactionPerContext(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "turn-terminal-authority")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	if hasRecords, err := store.HasRecords(context.Background()); err != nil || hasRecords {
		t.Fatalf("cold terminal store state mismatch: hasRecords=%v err=%v", hasRecords, err)
	}
	if err := store.PutDispositionIfAbsent(context.Background(), fixture.Disposition); err == nil {
		t.Fatal("orphan turn terminal disposition was accepted")
	}
	if err := store.PutIntentIfAbsent(context.Background(), fixture.Intent); err != nil {
		t.Fatal(err)
	}
	if err := store.PutIntentIfAbsent(context.Background(), fixture.Intent); err != nil {
		t.Fatalf("exact terminal intent replay was not idempotent: %v", err)
	}
	otherIntent, err := domainturnterminal.NewTurnTerminalIntentV1(domainturnterminal.TurnTerminalIntentInputV1{
		PrivateFinal: fixture.PrivateFinal, EventManifestDigest: domainsecurity.SHA256Hex([]byte("other-manifest")),
		AuthorityKeyID: domainsecurity.SHA256Hex(fixture.PublicKey), AuthorityPublicKey: fixture.PublicKey,
	}, fixture.Sign)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutIntentIfAbsent(context.Background(), otherIntent); err == nil {
		t.Fatal("same context acquired a second terminal intent")
	}
	if err := store.PutDispositionIfAbsent(context.Background(), fixture.Disposition); err != nil {
		t.Fatal(err)
	}
	if err := store.PutDispositionIfAbsent(context.Background(), fixture.Disposition); err != nil {
		t.Fatalf("exact terminal disposition replay was not idempotent: %v", err)
	}
	otherClosure, err := domaincachetelemetry.NewProviderTurnClosureV1(domaincachetelemetry.ProviderTurnClosureInputV1{
		TurnBindingHMAC:    fixture.Closure.TurnBindingHMAC,
		Intents:            []domaincachetelemetry.ProviderAttemptIntentV1{},
		Settlements:        []domaincachetelemetry.ProviderAttemptSettlementV1{},
		TerminalReasonCode: fixture.Intent.TerminalReasonCode,
		ClosedAt:           terminaltest.FixtureTimeV1().Add(1500 * time.Millisecond),
		AuthorityKeyID:     domainsecurity.SHA256Hex(fixture.PublicKey), AuthorityPublicKey: fixture.PublicKey,
	}, fixture.Sign)
	if err != nil {
		t.Fatal(err)
	}
	otherDisposition, err := domainturnterminal.NewTurnTerminalDispositionV1(domainturnterminal.TurnTerminalDispositionInputV1{
		Intent: fixture.Intent, ProviderClosure: otherClosure, AcceptedFinalDisposition: fixture.AcceptedDisposition,
		AuthorityKeyID: domainsecurity.SHA256Hex(fixture.PublicKey), AuthorityPublicKey: fixture.PublicKey,
	}, fixture.Sign)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutDispositionIfAbsent(context.Background(), otherDisposition); err == nil {
		t.Fatal("same context acquired a second terminal disposition")
	}
	reopened, err := NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := reopened.ReadIntent(context.Background(), fixture.Context.ContextDigest)
	if err != nil || intent != fixture.Intent {
		t.Fatalf("terminal intent restart readback mismatch: intent=%#v err=%v", intent, err)
	}
	disposition, err := reopened.ReadDisposition(context.Background(), fixture.Context.ContextDigest)
	if err != nil || disposition != fixture.Disposition {
		t.Fatalf("terminal disposition restart readback mismatch: disposition=%#v err=%v", disposition, err)
	}
	if _, err := reopened.ReadIntent(context.Background(), domainsecurity.SHA256Hex([]byte("missing"))); !errors.Is(err, turnterminalstoreport.ErrNotFound) {
		t.Fatalf("missing terminal intent error = %v", err)
	}
}

func TestStoreConcurrentExactIntentPutIsIdempotent(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "turn-terminal-authority")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	errorsByWorker := make(chan error, 8)
	for index := 0; index < 8; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errorsByWorker <- store.PutIntentIfAbsent(context.Background(), fixture.Intent)
		}()
	}
	wait.Wait()
	close(errorsByWorker)
	for err := range errorsByWorker {
		if err != nil {
			t.Fatalf("concurrent exact terminal intent put failed: %v", err)
		}
	}
	count := 0
	if err := store.VisitIntents(context.Background(), func(domainturnterminal.TurnTerminalIntentV1) error {
		count++
		return nil
	}); err != nil || count != 1 {
		t.Fatalf("terminal intent count=%d err=%v", count, err)
	}
}

func TestPreparedRecoveryRejectsDispositionWithoutIntent(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "turn-terminal-authority")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutIntentIfAbsent(context.Background(), fixture.Intent); err != nil {
		t.Fatal(err)
	}
	if err := store.PutDispositionIfAbsent(context.Background(), fixture.Disposition); err != nil {
		t.Fatal(err)
	}
	key := fixture.Context.ContextDigest
	intentPath := filepath.Join(root, "intents", key[:2], key+".json")
	if err := os.Remove(intentPath); err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareRecoveryV1(context.Background(), root, access)
	if err == nil {
		err = prepared.ValidateSemantics(context.Background())
	}
	if err == nil {
		t.Fatal("prepared recovery accepted an orphan terminal disposition")
	}
	dispositionPath := filepath.Join(root, "dispositions", key[:2], key+".json")
	if _, statErr := os.Stat(dispositionPath); statErr != nil {
		t.Fatalf("failed recovery preflight mutated surviving disposition: %v", statErr)
	}
}

func TestStoreHonorsCancelledContextBeforeMutation(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "turn-terminal-authority")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.PutIntentIfAbsent(ctx, fixture.Intent); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled terminal intent put error=%v", err)
	}
	if hasRecords, err := store.HasRecords(context.Background()); err != nil || hasRecords {
		t.Fatalf("cancelled terminal intent mutated store: hasRecords=%v err=%v", hasRecords, err)
	}
}
