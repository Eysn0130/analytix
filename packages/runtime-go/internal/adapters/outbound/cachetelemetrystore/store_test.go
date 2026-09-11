package cachetelemetrystore

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	cachetelemetrystoreport "analytix.local/runtime-go/internal/ports/cachetelemetrystore"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

type storeLedgerFixtureV1 struct {
	publicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey
	keyID      string
	turn       string
}

type countingCacheTelemetryAccessAuthorityV1 struct {
	inner *privatecastest.AccessAuthority
	calls int
}

func (authority *countingCacheTelemetryAccessAuthorityV1) WithPrivateCASAccess(
	ctx context.Context,
	requestedRoot string,
	access func(privatecasport.RootBinding) error,
) error {
	authority.calls++
	return authority.inner.WithPrivateCASAccess(ctx, requestedRoot, access)
}

func TestStoreIntentCommitDoesNotRereadTheCommittedCASRecord(t *testing.T) {
	root := filepath.Join(t.TempDir(), "provider-cache-telemetry")
	inner, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	access := &countingCacheTelemetryAccessAuthorityV1{inner: inner}
	store, err := NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	access.calls = 0
	fixture := newStoreLedgerFixtureV1(t)
	intent := fixture.intent(t, "call-1", 1, 1, 1, "2026-07-14T00:00:00Z")
	if err := store.PutIntentIfAbsent(context.Background(), intent); err != nil {
		t.Fatal(err)
	}
	// Three pre-commit and three post-commit leaf observations preserve the
	// cross-leaf ledger invariant. The CAS commit itself provides exact
	// read-back, so a separate single-record observation would be redundant.
	if access.calls != 7 {
		t.Fatalf("intent commit private CAS observations: got %d want 7", access.calls)
	}
}

func TestStoreSettlementCommitDoesNotRereadTheCommittedCASRecord(t *testing.T) {
	root := filepath.Join(t.TempDir(), "provider-cache-telemetry")
	inner, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	access := &countingCacheTelemetryAccessAuthorityV1{inner: inner}
	store, err := NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	fixture := newStoreLedgerFixtureV1(t)
	intent := fixture.intent(t, "call-1", 1, 1, 1, "2026-07-14T00:00:00Z")
	if err := store.PutIntentIfAbsent(context.Background(), intent); err != nil {
		t.Fatal(err)
	}
	access.calls = 0
	settlement := fixture.successSettlement(t, intent, "2026-07-14T00:00:01Z")
	if err := store.PutSettlementIfAbsent(context.Background(), settlement); err != nil {
		t.Fatal(err)
	}
	if access.calls != 7 {
		t.Fatalf("settlement commit private CAS observations: got %d want 7", access.calls)
	}
}

func TestStorePersistsExactProviderLedgerAcrossRestart(t *testing.T) {
	root := filepath.Join(t.TempDir(), "provider-cache-telemetry")
	store := newTestCacheTelemetryStoreV1(t, root)
	if has, err := store.HasRecords(context.Background()); err != nil || has {
		t.Fatalf("new provider telemetry store is not empty: has=%v err=%v", has, err)
	}
	fixture := newStoreLedgerFixtureV1(t)
	intent := fixture.intent(t, "call-1", 1, 1, 1, "2026-07-14T00:00:00Z")
	settlement := fixture.successSettlement(t, intent, "2026-07-14T00:00:01Z")
	closure := fixture.closure(t, []domaincachetelemetry.ProviderAttemptIntentV1{intent}, []domaincachetelemetry.ProviderAttemptSettlementV1{settlement}, "2026-07-14T00:00:02Z")
	for _, put := range []func() error{
		func() error { return store.PutIntentIfAbsent(context.Background(), intent) },
		func() error { return store.PutIntentIfAbsent(context.Background(), intent) },
		func() error { return store.PutSettlementIfAbsent(context.Background(), settlement) },
		func() error { return store.PutSettlementIfAbsent(context.Background(), settlement) },
		func() error { return store.PutClosureIfAbsent(context.Background(), closure) },
		func() error { return store.PutClosureIfAbsent(context.Background(), closure) },
	} {
		if err := put(); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{
		filepath.Join(root, "attempts", intent.IntentID[:2], intent.IntentID+".json"),
		filepath.Join(root, "settlements", intent.IntentID[:2], intent.IntentID+".json"),
		filepath.Join(root, "turn-closures", fixture.turn[:2], fixture.turn+".json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("provider telemetry CAS record is missing at %s: %v", path, err)
		}
	}
	reopened := newTestCacheTelemetryStoreV1(t, root)
	readIntent, intentErr := reopened.ReadIntent(context.Background(), intent.IntentID)
	readSettlement, settlementErr := reopened.ReadSettlement(context.Background(), intent.IntentID)
	readClosure, closureErr := reopened.ReadClosure(context.Background(), fixture.turn)
	if intentErr != nil || settlementErr != nil || closureErr != nil || readIntent != intent ||
		readSettlement != settlement || readClosure != closure {
		t.Fatalf("reopened provider ledger mismatch: intentErr=%v settlementErr=%v closureErr=%v", intentErr, settlementErr, closureErr)
	}
	if has, err := reopened.HasRecords(context.Background()); err != nil || !has {
		t.Fatalf("persisted provider telemetry appears empty: has=%v err=%v", has, err)
	}
	visited := [3]int{}
	if err := reopened.VisitIntents(context.Background(), func(domaincachetelemetry.ProviderAttemptIntentV1) error { visited[0]++; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := reopened.VisitSettlements(context.Background(), func(domaincachetelemetry.ProviderAttemptSettlementV1) error { visited[1]++; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := reopened.VisitClosures(context.Background(), func(domaincachetelemetry.ProviderTurnClosureV1) error { visited[2]++; return nil }); err != nil {
		t.Fatal(err)
	}
	if visited != [3]int{1, 1, 1} {
		t.Fatalf("unexpected provider telemetry visitors: %v", visited)
	}
}

func TestStoreVisitInventoryUsesOneFrozenCrossLeafObservation(t *testing.T) {
	store := newTestCacheTelemetryStoreV1(t, filepath.Join(t.TempDir(), "provider-cache-telemetry"))
	fixture := newStoreLedgerFixtureV1(t)
	intent := fixture.intent(t, "call-1", 1, 1, 1, "2026-07-14T00:00:00Z")
	settlement := fixture.successSettlement(t, intent, "2026-07-14T00:00:01Z")
	if err := store.PutIntentIfAbsent(context.Background(), intent); err != nil {
		t.Fatal(err)
	}
	visited := [3]int{}
	if err := store.VisitInventory(
		context.Background(),
		func(domaincachetelemetry.ProviderAttemptIntentV1) error {
			visited[0]++
			return store.PutSettlementIfAbsent(context.Background(), settlement)
		},
		func(domaincachetelemetry.ProviderAttemptSettlementV1) error {
			visited[1]++
			return nil
		},
		func(domaincachetelemetry.ProviderTurnClosureV1) error {
			visited[2]++
			return nil
		},
	); err != nil {
		t.Fatal(err)
	}
	if visited != [3]int{1, 0, 0} {
		t.Fatalf("cross-leaf visitor mixed observations: %v", visited)
	}
	if _, err := store.ReadSettlement(context.Background(), intent.IntentID); err != nil {
		t.Fatalf("visitor callback could not persist after inventory lock release: %v", err)
	}
}

func TestStoreRejectsMissingIntentConflictingSettlementAndLateClosedTurnWrites(t *testing.T) {
	store := newTestCacheTelemetryStoreV1(t, filepath.Join(t.TempDir(), "provider-cache-telemetry"))
	fixture := newStoreLedgerFixtureV1(t)
	intent := fixture.intent(t, "call-1", 1, 1, 1, "2026-07-14T00:00:00Z")
	settlement := fixture.successSettlement(t, intent, "2026-07-14T00:00:01Z")
	if err := store.PutSettlementIfAbsent(context.Background(), settlement); err == nil {
		t.Fatal("store accepted settlement without intent")
	}
	if _, err := store.ReadIntent(context.Background(), intent.IntentID); !errors.Is(err, cachetelemetrystoreport.ErrNotFound) {
		t.Fatalf("missing provider intent did not return ErrNotFound: %v", err)
	}
	if err := store.PutIntentIfAbsent(context.Background(), intent); err != nil {
		t.Fatal(err)
	}
	if err := store.PutSettlementIfAbsent(context.Background(), settlement); err != nil {
		t.Fatal(err)
	}
	conflictingObservation := settlement.Observation
	conflictingObservation.SettledAt = "2026-07-14T00:00:01.5Z"
	conflicting := fixture.settlement(t, intent, domaincachetelemetry.ProviderDispatchStateSent, conflictingObservation, "provider_succeeded_after_retry")
	if err := store.PutSettlementIfAbsent(context.Background(), conflicting); err == nil {
		t.Fatal("store replaced an existing provider settlement")
	}
	closure := fixture.closure(t, []domaincachetelemetry.ProviderAttemptIntentV1{intent}, []domaincachetelemetry.ProviderAttemptSettlementV1{settlement}, "2026-07-14T00:00:02Z")
	if err := store.PutClosureIfAbsent(context.Background(), closure); err != nil {
		t.Fatal(err)
	}
	lateIntent := fixture.intent(t, "call-2", 2, 2, 1, "2026-07-14T00:00:03Z")
	if err := store.PutIntentIfAbsent(context.Background(), lateIntent); err == nil {
		t.Fatal("store accepted a new intent after turn closure")
	}
	lateSettlement := fixture.successSettlement(t, lateIntent, "2026-07-14T00:00:04Z")
	if err := store.PutSettlementIfAbsent(context.Background(), lateSettlement); err == nil {
		t.Fatal("store accepted a settlement after turn closure")
	}
	changedClosure := closure
	changedClosure.TerminalReasonCode = domaincachetelemetry.ProviderTurnTerminalProviderFailureV1
	if err := store.PutClosureIfAbsent(context.Background(), changedClosure); err == nil {
		t.Fatal("store replaced an existing turn closure")
	}
}

func TestStoreRequiresPriorSettlementBeforePhysicalRetryIntent(t *testing.T) {
	store := newTestCacheTelemetryStoreV1(t, filepath.Join(t.TempDir(), "provider-cache-telemetry"))
	fixture := newStoreLedgerFixtureV1(t)
	first := fixture.intent(t, "call-1", 1, 1, 1, "2026-07-14T00:00:00Z")
	second := fixture.intent(t, "call-1", 1, 1, 2, "2026-07-14T00:00:02Z")
	if err := store.PutIntentIfAbsent(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := store.PutIntentIfAbsent(context.Background(), second); err == nil {
		t.Fatal("store accepted retry before prior physical attempt settlement")
	}
	firstSettlement := fixture.successSettlement(t, first, "2026-07-14T00:00:01Z")
	if err := store.PutSettlementIfAbsent(context.Background(), firstSettlement); err != nil {
		t.Fatal(err)
	}
	if err := store.PutIntentIfAbsent(context.Background(), second); err != nil {
		t.Fatalf("store rejected ordered physical retry: %v", err)
	}
	secondSettlement := fixture.successSettlement(t, second, "2026-07-14T00:00:03Z")
	if err := store.PutSettlementIfAbsent(context.Background(), secondSettlement); err != nil {
		t.Fatal(err)
	}
}

func TestStoreAndPreparedRecoveryFailClosedOnCrossLeafSemanticCorruption(t *testing.T) {
	root := filepath.Join(t.TempDir(), "provider-cache-telemetry")
	store := newTestCacheTelemetryStoreV1(t, root)
	fixture := newStoreLedgerFixtureV1(t)
	intent := fixture.intent(t, "call-1", 1, 1, 1, "2026-07-14T00:00:00Z")
	settlement := fixture.successSettlement(t, intent, "2026-07-14T00:00:01Z")
	body, err := domaincachetelemetry.ProviderAttemptSettlementV1Bytes(settlement)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.settlementCAS.PutIfAbsent(context.Background(), settlement.IntentID, body); err != nil {
		t.Fatal(err)
	}
	if has, err := store.HasRecords(context.Background()); err == nil || has {
		t.Fatalf("HasRecords accepted orphan settlement: has=%v err=%v", has, err)
	}
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareRecoveryV1(context.Background(), root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.ValidateSemantics(context.Background()); err == nil {
		t.Fatal("prepared recovery accepted orphan settlement")
	}
}

func TestStoreHonorsCancellationAndNeverLeaksRawLedgerValues(t *testing.T) {
	root := filepath.Join(t.TempDir(), "provider-cache-telemetry")
	store := newTestCacheTelemetryStoreV1(t, root)
	fixture := newStoreLedgerFixtureV1(t)
	intent := fixture.intent(t, "call-1", 1, 1, 1, "2026-07-14T00:00:00Z")
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.PutIntentIfAbsent(cancelled, intent); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled provider telemetry write returned %v", err)
	}
	if err := store.PutIntentIfAbsent(context.Background(), intent); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadIntent(cancelled, intent.IntentID); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled provider telemetry read returned %v", err)
	}
	var material []byte
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			material = append(material, body...)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range [][]byte{
		[]byte("合成样例事件甲"), []byte("6222021234567890123"), []byte("sk-provider-secret"),
		[]byte("https://api.example.test/v1/chat?token=secret"), []byte("/Users/sun/Projects/case-a"),
	} {
		if bytes.Contains(material, forbidden) {
			t.Fatalf("provider telemetry CAS leaked forbidden material %q", forbidden)
		}
	}
}

func newTestCacheTelemetryStoreV1(t *testing.T, root string) *Store {
	t.Helper()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func newStoreLedgerFixtureV1(t *testing.T) storeLedgerFixtureV1 {
	t.Helper()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x3c}, ed25519.SeedSize))
	publicKey := append(ed25519.PublicKey(nil), privateKey.Public().(ed25519.PublicKey)...)
	return storeLedgerFixtureV1{
		publicKey: publicKey, privateKey: privateKey, keyID: domainsecurity.SHA256Hex(publicKey),
		turn: storeTelemetryHMACV1("turn-private"),
	}
}

func (fixture storeLedgerFixtureV1) sign(message []byte) ([]byte, error) {
	return ed25519.Sign(fixture.privateKey, message), nil
}

func (fixture storeLedgerFixtureV1) intent(t *testing.T, logical string, logicalOrdinal, channelOrdinal, attempt uint32, startedAt string) domaincachetelemetry.ProviderAttemptIntentV1 {
	t.Helper()
	shape := domaincachetelemetry.CacheVisibleShapeV1{
		SchemaVersion:   domaincachetelemetry.CacheVisibleShapeV1SchemaVersion,
		LogicalCallHMAC: storeTelemetryHMACV1(logical), Attempt: attempt,
		ProviderFamily: domaincachetelemetry.ProviderFamilyDeepSeek,
		ModelHMAC:      storeTelemetryHMACV1("model"), Endpoint: domaincachetelemetry.EndpointFormatChatCompletions,
		EndpointHMAC: storeTelemetryHMACV1("endpoint"), WireBodyHMAC: storeTelemetryHMACV1("body-" + logical),
		CredentialScopeHMAC: storeTelemetryHMACV1("credential"), ProviderConfigHMAC: storeTelemetryHMACV1("config"),
		DigestEpoch: 7, StartedAt: startedAt,
	}
	intent, err := domaincachetelemetry.NewProviderAttemptIntentV1(domaincachetelemetry.ProviderAttemptIntentInputV1{
		TurnBindingHMAC: fixture.turn, UsageSource: domaincachetelemetry.ProviderUsageSourceTurn,
		ChildRunHMAC: storeTelemetryHMACV1("root-run"), Channel: domaincachetelemetry.ProviderChannelPrimary,
		ChannelOrdinal: channelOrdinal, LogicalCallOrdinal: logicalOrdinal, Shape: shape,
		WireHeadersHMAC: storeTelemetryHMACV1("headers-" + logical), IssuedAt: storeTelemetryTimeV1(t, startedAt),
		AuthorityKeyID: fixture.keyID, AuthorityPublicKey: fixture.publicKey,
	}, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	return intent
}

func (fixture storeLedgerFixtureV1) successSettlement(t *testing.T, intent domaincachetelemetry.ProviderAttemptIntentV1, settledAt string) domaincachetelemetry.ProviderAttemptSettlementV1 {
	t.Helper()
	observation := domaincachetelemetry.ProviderCallObservationV1{
		SchemaVersion: domaincachetelemetry.ProviderCallObservationV1SchemaVersion, Shape: intent.Shape,
		Status: domaincachetelemetry.ProviderCallStatusSucceeded,
		Usage: domaincachetelemetry.ProviderUsageV1{
			InputTokens:     domaincachetelemetry.TokenCountV1{Known: true, Value: 100},
			OutputTokens:    domaincachetelemetry.TokenCountV1{Known: true, Value: 20},
			CacheHitTokens:  domaincachetelemetry.TokenCountV1{Known: true, Value: 90},
			CacheMissTokens: domaincachetelemetry.TokenCountV1{Known: true, Value: 10},
			ReasoningTokens: domaincachetelemetry.TokenCountV1{Known: false},
		},
		SettledAt: settledAt,
	}
	return fixture.settlement(t, intent, domaincachetelemetry.ProviderDispatchStateSent, observation, "provider_succeeded")
}

func (fixture storeLedgerFixtureV1) settlement(t *testing.T, intent domaincachetelemetry.ProviderAttemptIntentV1, dispatch domaincachetelemetry.ProviderDispatchStateV1, observation domaincachetelemetry.ProviderCallObservationV1, reason string) domaincachetelemetry.ProviderAttemptSettlementV1 {
	t.Helper()
	settlement, err := domaincachetelemetry.NewProviderAttemptSettlementV1(domaincachetelemetry.ProviderAttemptSettlementInputV1{
		Intent: intent, DispatchState: dispatch, Observation: observation, SafeReasonCode: reason,
		AuthorityKeyID: fixture.keyID, AuthorityPublicKey: fixture.publicKey,
	}, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	return settlement
}

func (fixture storeLedgerFixtureV1) closure(t *testing.T, intents []domaincachetelemetry.ProviderAttemptIntentV1, settlements []domaincachetelemetry.ProviderAttemptSettlementV1, closedAt string) domaincachetelemetry.ProviderTurnClosureV1 {
	t.Helper()
	closure, err := domaincachetelemetry.NewProviderTurnClosureV1(domaincachetelemetry.ProviderTurnClosureInputV1{
		TurnBindingHMAC: fixture.turn, Intents: intents, Settlements: settlements,
		TerminalReasonCode: domaincachetelemetry.ProviderTurnTerminalSuccessV1, ClosedAt: storeTelemetryTimeV1(t, closedAt),
		AuthorityKeyID: fixture.keyID, AuthorityPublicKey: fixture.publicKey,
	}, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	return closure
}

func storeTelemetryHMACV1(value string) string {
	digest := sha256.Sum256([]byte("provider-cache-store-test:" + value))
	return hex.EncodeToString(digest[:])
}

func storeTelemetryTimeV1(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
