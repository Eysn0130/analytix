package casethread

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestRegistryRestoresPreparedContextAndDerivedLineageWithoutPromotingAuthority(t *testing.T) {
	store := newMemoryStore()
	authority := newTestAuthority(3)
	registry, err := NewRegistry(context.Background(), authority, store)
	if err != nil {
		t.Fatal(err)
	}
	securityContext := testSecurityContext("thread-a", "turn-a", 4)
	if err := RegisterRequired(context.Background(), registry, securityContext); err != nil {
		t.Fatal(err)
	}
	refreshed := testSecurityContext("thread-a", "turn-a", 5)
	if err := registry.Register(context.Background(), refreshed); err != nil {
		t.Fatalf("host context refresh for one turn was not recorded: %v", err)
	}
	if err := registry.Derive(context.Background(), "thread-a", "thread-child", "fork"); err != nil {
		t.Fatal(err)
	}
	if err := registry.Derive(context.Background(), "thread-child", "thread-grandchild", "resume"); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewRegistry(context.Background(), authority, store)
	if err != nil || !restarted.IsCaseThread("thread-a") || restarted.ContainsContext(securityContext) ||
		restarted.ContainsContext(refreshed) || !restarted.IsCaseThread("thread-child") || !restarted.IsCaseThread("thread-grandchild") {
		t.Fatalf("case thread authority did not survive restart: registry=%#v err=%v", restarted, err)
	}
	if _, err := NewRegistry(context.Background(), newTestAuthority(4), store); err == nil {
		t.Fatal("case thread authority inventory trusted another installation key")
	}
	general := testGeneralSecurityContext("thread-u", "turn-u", 1)
	if err := RegisterRequired(context.Background(), restarted, general); err != nil || restarted.IsCaseThread(general.ThreadID) {
		t.Fatalf("general V2 thread was classified as a case: err=%v", err)
	}
}

type caseInventoryVerificationFailureV1 struct {
	*testAuthority
	err error
}

func (authority caseInventoryVerificationFailureV1) VerifyTrusted(context.Context, string, []byte, []byte, []byte) error {
	return authority.err
}

func TestRegistryPreservesOriginalInventoryVerificationErrors(t *testing.T) {
	ctx := context.Background()
	authority, store := newTestAuthority(67), newMemoryStore()
	registry, err := NewRegistry(ctx, authority, store)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(ctx, testSecurityContext("thread-original", "turn-original", 1)); err != nil {
		t.Fatal(err)
	}
	before, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, sentinel := range []error{errors.New("synthetic case inventory verification I/O failure"), context.Canceled} {
		observed, err := NewRegistry(ctx, caseInventoryVerificationFailureV1{testAuthority: authority, err: sentinel}, store)
		if observed != nil || !errors.Is(err, sentinel) {
			t.Errorf("original case inventory lost verification failure: %v", err)
		}
		after, err := store.List(ctx)
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatal("rejected original observation changed authority")
		}
	}
}

func TestRegistryReturnsExactIdempotentSignedLineageReceipt(t *testing.T) {
	registry, err := NewRegistry(context.Background(), newTestAuthority(31), newMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(context.Background(), testSecurityContext("thread-parent", "turn-parent", 1)); err != nil {
		t.Fatal(err)
	}
	first, err := registry.DeriveWithReceipt(
		context.Background(), "thread-parent", "thread-child", "fork",
	)
	if err != nil || ValidateDerivedLineageReceipt(first, "thread-parent", "thread-child", "fork") != nil {
		t.Fatalf("signed lineage receipt is invalid: receipt=%#v err=%v", first, err)
	}
	replayed, err := registry.DeriveWithReceipt(
		context.Background(), "thread-parent", "thread-child", "fork",
	)
	if err != nil || !reflect.DeepEqual(replayed, first) {
		t.Fatalf("lineage retry did not return the exact receipt: receipt=%#v err=%v", replayed, err)
	}
	verified, err := registry.VerifyDerivedLineageReceipt(
		context.Background(), "thread-parent", "thread-child", "fork", first.RecordDigest,
	)
	if err != nil || !reflect.DeepEqual(verified, first) {
		t.Fatalf("lineage receipt verification mismatch: receipt=%#v err=%v", verified, err)
	}
	wrongDigest := first.RecordDigest
	if wrongDigest[0] == '0' {
		wrongDigest = "1" + wrongDigest[1:]
	} else {
		wrongDigest = "0" + wrongDigest[1:]
	}
	if _, err := registry.VerifyDerivedLineageReceipt(
		context.Background(), "thread-parent", "thread-child", "fork", wrongDigest,
	); err == nil {
		t.Fatal("lineage receipt verification accepted a different durable digest")
	}
	tampered := first
	tampered.ParentThreadID = "thread-other"
	if ValidateDerivedLineageReceipt(tampered, "thread-parent", "thread-child", "fork") == nil {
		t.Fatal("tampered lineage receipt was accepted")
	}
}

func TestRegistryCommitsExactlyOneDurableTurnContext(t *testing.T) {
	store := newMemoryStore()
	authority := newTestAuthority(10)
	registry, err := NewRegistry(context.Background(), authority, store)
	if err != nil {
		t.Fatal(err)
	}
	staged := testSecurityContext("thread-commit", "turn-commit", 1)
	committed := testSecurityContext("thread-commit", "turn-commit", 2)
	if err := registry.Register(context.Background(), staged); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(context.Background(), committed); err != nil {
		t.Fatal(err)
	}
	state, err := contextepochapp.BootstrapState(committed.ThreadID, committed.ContextEpoch, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(committed)}, time.Unix(5, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := CommitRequired(context.Background(), registry, committed, state, time.Unix(6, 0)); err != nil {
		t.Fatal(err)
	}
	if err := CommitRequired(context.Background(), registry, committed, state, time.Unix(7, 0)); err != nil {
		t.Fatalf("idempotent committed context was rejected: %v", err)
	}
	resolved, ok := registry.CommittedContext(committed.ThreadID, committed.TurnID)
	if !ok || !reflect.DeepEqual(resolved.SecurityContext, committed) || !reflect.DeepEqual(resolved.EpochState, state) {
		t.Fatalf("committed context lookup mismatch: resolved=%#v ok=%v", resolved, ok)
	}
	otherState, err := contextepochapp.BootstrapState(staged.ThreadID, staged.ContextEpoch, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(staged)}, time.Unix(5, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := CommitRequired(context.Background(), registry, staged, otherState, time.Unix(8, 0)); err == nil {
		t.Fatal("a second context won the same committed turn")
	}
	restarted, err := NewRegistry(context.Background(), authority, store)
	restored, restoredOK := restarted.CommittedContext(committed.ThreadID, committed.TurnID)
	if err != nil || !restoredOK || !reflect.DeepEqual(restored.SecurityContext, committed) {
		t.Fatalf("committed context did not survive restart: restored=%#v ok=%v err=%v", restored, restoredOK, err)
	}
}

func TestBoundaryOnlyContextCommitsAndRestartsAsAuditAuthority(t *testing.T) {
	store := newMemoryStore()
	authority := newTestAuthority(24)
	registry, err := NewRegistry(context.Background(), authority, store)
	if err != nil {
		t.Fatal(err)
	}
	boundary := testBoundarySecurityContext("thread-boundary", "turn-boundary", 1)
	if err := RegisterRequired(context.Background(), registry, boundary); err != nil {
		t.Fatalf("boundary-only context was not registered: %v", err)
	}
	state, err := contextepochapp.BootstrapState(
		boundary.ThreadID, boundary.ContextEpoch,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(boundary)}, time.Unix(30, 0),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := CommitRequired(context.Background(), registry, boundary, state, time.Unix(31, 0)); err != nil {
		t.Fatalf("boundary-only context was not committed: %v", err)
	}
	if !registry.ContainsContext(boundary) {
		t.Fatal("committed boundary-only context was not publication authority")
	}
	var committedRecord domainsecurity.CaseThreadAuthorityRecord
	for _, record := range store.records {
		if record.SecurityContext != nil && record.CommittedTurnState != nil && record.SecurityContext.ContextDigest == boundary.ContextDigest {
			committedRecord = record
		}
	}
	if committedRecord.RecordDigest == "" || domainsecurity.CaseThreadAuthorityCanAuthorizeExecution(committedRecord) {
		t.Fatal("boundary-only audit authority authorized provider or tool execution")
	}
	restarted, err := NewRegistry(context.Background(), authority, store)
	if err != nil || !restarted.ContainsContext(boundary) {
		t.Fatalf("committed boundary-only authority did not survive restart: %v", err)
	}
	reader := &threadReaderStub{threads: map[string]map[string]any{
		boundary.ThreadID: {
			"id": boundary.ThreadID, "securityState": boundary, "contextEpochState": state,
			"turns": []any{map[string]any{"id": boundary.TurnID, "securityContext": boundary}},
		},
	}}
	inventory, err := PreflightRestartInventory(restarted, reader)
	if err != nil || inventory.Quarantined[boundary.ThreadID] != "" {
		t.Fatalf("valid boundary-only authority was quarantined at restart: inventory=%#v err=%v", inventory, err)
	}
	if err := RegistrationHook(restarted)(context.Background(), testGeneralSecurityContext(boundary.ThreadID, "turn-downgrade", 2)); err == nil {
		t.Fatal("existing case thread downgraded to a general publication policy")
	}
	if err := RegistrationHook(restarted)(context.Background(), testBoundarySecurityContext(boundary.ThreadID, "turn-next", 2)); err != nil {
		t.Fatalf("existing case thread rejected a continuing boundary-only context: %v", err)
	}
}

func TestRequiredAuthorityNoOpsOnlyForGeneralV2AndRejectsV1(t *testing.T) {
	general := testGeneralSecurityContext("thread-general", "turn-general", 1)
	if err := RegisterRequired(context.Background(), nil, general); err != nil {
		t.Fatalf("general V2 registration was not a no-op: %v", err)
	}
	if err := CommitRequired(context.Background(), nil, general, domaincontextepoch.State{}, time.Time{}); err != nil {
		t.Fatalf("general V2 commit was not a no-op: %v", err)
	}
	legacyGeneral := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-v1-general", TurnID: "turn-v1-general", WorkspaceRealPath: "/cases/a", IssuedAt: time.Unix(1, 0),
	})
	legacyCase := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-v1-case", TurnID: "turn-v1-case", WorkspaceRealPath: "/cases/a", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-a")), DatasetSnapshotID: "snapshot-a",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-a")), ContextEpoch: 1, IssuedAt: time.Unix(1, 0),
	})
	for name, legacy := range map[string]domainsecurity.TurnSecurityContext{"general": legacyGeneral, "case": legacyCase} {
		t.Run(name, func(t *testing.T) {
			if err := RegisterRequired(context.Background(), nil, legacy); err == nil {
				t.Fatal("legacy V1 registration was silently accepted")
			}
			if err := CommitRequired(context.Background(), nil, legacy, domaincontextepoch.State{}, time.Time{}); err == nil {
				t.Fatal("legacy V1 commit was silently accepted")
			}
		})
	}
	registry, err := NewRegistry(context.Background(), newTestAuthority(25), newMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(context.Background(), legacyCase); err == nil {
		t.Fatal("registry signed a new V1 case authority record")
	}
	if _, err := registry.PlanContexts(context.Background(), []domainsecurity.TurnSecurityContext{legacyCase}); err == nil {
		t.Fatal("migration planned a new V1 case authority overlay")
	}
	if _, err := domainsecurity.NewCaseThreadAuthorityRecord(
		legacyCase, registry.authority.KeyID(), registry.authority.PublicKey(),
		func(message []byte) ([]byte, error) { return registry.authority.Sign(context.Background(), message) },
	); err == nil {
		t.Fatal("domain constructor signed a new V1 case authority record")
	}
}

func TestLegacyV1InventoryRemainsAuditableButCannotAuthorize(t *testing.T) {
	authority := newTestAuthority(26)
	legacy := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-legacy", TurnID: "turn-legacy", WorkspaceRealPath: "/cases/a", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-a")), DatasetSnapshotID: "snapshot-a",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-a")), ContextEpoch: 1, IssuedAt: time.Unix(1, 0),
	})
	record := signedLegacyContextRecord(t, authority, legacy)
	store := newMemoryStore()
	store.records[record.RecordDigest] = record
	registry, err := NewRegistry(context.Background(), authority, store)
	if err != nil {
		t.Fatalf("legacy signed inventory was not readable for audit: %v", err)
	}
	if !registry.IsCaseThread(legacy.ThreadID) || len(registry.ContextTurnIDs(legacy.ThreadID)) != 1 {
		t.Fatal("legacy signed authority was not indexed for audit")
	}
	if registry.ContainsContext(legacy) || registry.CanExecute(legacy.ThreadID) || domainsecurity.CaseThreadAuthorityCanAuthorizeExecution(record) {
		t.Fatal("legacy V1 audit inventory became current authority")
	}
}

func TestMigrationPlanIsReadOnlyUntilVerifiedApply(t *testing.T) {
	store := newMemoryStore()
	registry, err := NewRegistry(context.Background(), newTestAuthority(5), store)
	if err != nil {
		t.Fatal(err)
	}
	securityContext := testSecurityContext("thread-migrate", "turn-migrate", 2)
	plan, err := registry.PlanContexts(context.Background(), []domainsecurity.TurnSecurityContext{securityContext})
	if err != nil || len(plan.Records) != 1 || store.puts != 0 || registry.IsCaseThread(securityContext.ThreadID) {
		t.Fatalf("case migration preflight mutated authority: plan=%#v puts=%d err=%v", plan, store.puts, err)
	}
	overlay, err := registry.WithPlan(context.Background(), plan)
	if err != nil || !overlay.IsCaseThread(securityContext.ThreadID) || !overlay.ContainsContext(securityContext) || store.puts != 0 {
		t.Fatalf("read-only migration overlay failed: puts=%d err=%v", store.puts, err)
	}
	if err := registry.ApplyPlan(context.Background(), plan); err != nil || store.puts != 1 {
		t.Fatalf("verified migration apply failed: puts=%d err=%v", store.puts, err)
	}
	restarted, err := NewRegistry(context.Background(), registry.authority, store)
	if err != nil || restarted.ContainsContext(securityContext) {
		t.Fatalf("staged migration record became live authority after restart: %v", err)
	}
	restartedPlan, err := restarted.PlanContexts(context.Background(), []domainsecurity.TurnSecurityContext{securityContext})
	if err != nil {
		t.Fatal(err)
	}
	restartedOverlay, err := restarted.WithPlan(context.Background(), restartedPlan)
	if err != nil || !restartedOverlay.ContainsContext(securityContext) {
		t.Fatalf("verified migration overlay was not reconstructed: %v", err)
	}
}

func TestStagedContextNeverBecomesPublicationAuthorityAcrossCrash(t *testing.T) {
	store := newMemoryStore()
	authority := newTestAuthority(12)
	registry, err := NewRegistry(context.Background(), authority, store)
	if err != nil {
		t.Fatal(err)
	}
	securityContext := testSecurityContext("thread-crash", "turn-crash", 7)
	if err := registry.Register(context.Background(), securityContext); err != nil {
		t.Fatal(err)
	}
	if registry.ContainsContext(securityContext) {
		t.Fatal("staged context became publication authority before commit")
	}
	restarted, err := NewRegistry(context.Background(), authority, store)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.ContainsContext(securityContext) {
		t.Fatal("staged context became publication authority after restart")
	}
	reader := &threadReaderStub{threads: map[string]map[string]any{
		securityContext.ThreadID: {
			"id": securityContext.ThreadID, "securityState": securityContext,
			"contextEpochState": map[string]any{"schemaVersion": 1},
			"turns":             []any{map[string]any{"id": securityContext.TurnID, "securityContext": securityContext}},
		},
	}}
	inventory, err := PreflightRestartInventory(restarted, reader)
	if err != nil || inventory.Quarantined[securityContext.ThreadID] == "" {
		t.Fatalf("append-before-authority-commit crash was not quarantined: inventory=%#v err=%v", inventory, err)
	}
}

func TestMigrationOverlayCannotOverrideCommittedTurnAuthority(t *testing.T) {
	registry, err := NewRegistry(context.Background(), newTestAuthority(13), newMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	committed := testSecurityContext("thread-migration-conflict", "turn-migration-conflict", 9)
	if err := registry.Register(context.Background(), committed); err != nil {
		t.Fatal(err)
	}
	state, err := contextepochapp.BootstrapState(
		committed.ThreadID, committed.ContextEpoch,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(committed)}, time.Unix(20, 0),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := CommitRequired(context.Background(), registry, committed, state, time.Unix(21, 0)); err != nil {
		t.Fatal(err)
	}
	conflict := testSecurityContext(committed.ThreadID, committed.TurnID, int(committed.ContextEpoch+1))
	if _, err := registry.PlanContexts(context.Background(), []domainsecurity.TurnSecurityContext{conflict}); err == nil {
		t.Fatal("migration overlay replaced an existing committed turn authority")
	}
}

func TestRestartInventoryQuarantinesStagedAndTamperedContexts(t *testing.T) {
	registry, err := NewRegistry(context.Background(), newTestAuthority(6), newMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	ready := testSecurityContext("thread-ready", "turn-ready", 3)
	staged := testSecurityContext("thread-staged", "turn-staged", 3)
	if err := registry.Register(context.Background(), ready); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(context.Background(), staged); err != nil {
		t.Fatal(err)
	}
	readyState, err := contextepochapp.BootstrapState(
		ready.ThreadID, ready.ContextEpoch, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(ready)}, time.Unix(10, 0),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := CommitRequired(context.Background(), registry, ready, readyState, time.Unix(11, 0)); err != nil {
		t.Fatal(err)
	}
	reader := &threadReaderStub{threads: map[string]map[string]any{
		"thread-ready": {
			"id": "thread-ready", "securityState": ready, "contextEpochState": map[string]any{"schemaVersion": 1},
			"turns": []any{map[string]any{"id": "turn-ready", "securityContext": ready}},
		},
		"thread-staged": {"id": "thread-staged", "turns": []any{}},
	}}
	inventory, err := PreflightRestartInventory(registry, reader)
	if err != nil || inventory.Quarantined["thread-staged"] == "" || inventory.Quarantined["thread-ready"] != "" {
		t.Fatalf("restart inventory classification mismatch: inventory=%#v err=%v", inventory, err)
	}
	if err := ApplyRestartInventory(registry, inventory); err != nil || registry.CanExecute("thread-staged") || !registry.CanExecute("thread-ready") {
		t.Fatalf("restart quarantine was not enforced: %v", err)
	}
	reader.threads["thread-ready"]["securityState"] = staged
	tampered, err := PreflightRestartInventory(registry, reader)
	if err != nil || tampered.Quarantined["thread-ready"] == "" {
		t.Fatalf("tampered current context was not quarantined: inventory=%#v err=%v", tampered, err)
	}
}

func TestRestartInventoryQuarantinesDerivedThreadAssistantLaundering(t *testing.T) {
	registry, err := NewRegistry(context.Background(), newTestAuthority(7), newMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	parent := testSecurityContext("thread-parent", "turn-parent", 2)
	if err := registry.Register(context.Background(), parent); err != nil {
		t.Fatal(err)
	}
	if err := registry.Derive(context.Background(), "thread-parent", "thread-derived", "fork"); err != nil {
		t.Fatal(err)
	}
	reader := &threadReaderStub{threads: map[string]map[string]any{
		"thread-derived": {"id": "thread-derived", "turns": []any{map[string]any{
			"id": "turn-laundered", "items": []any{map[string]any{"kind": "assistant_text", "text": "FABRICATED_CASE_FACT"}},
		}}},
	}}
	inventory, err := PreflightRestartInventory(registry, reader)
	if err != nil || inventory.Quarantined["thread-derived"] == "" {
		t.Fatalf("derived assistant laundering was not quarantined: inventory=%#v err=%v", inventory, err)
	}
}

type testAuthority struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	keyID      string
}

func newTestAuthority(seed byte) *testAuthority {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	return &testAuthority{privateKey: privateKey, publicKey: publicKey, keyID: domainsecurity.SHA256Hex(publicKey)}
}

func (authority *testAuthority) KeyID() string { return authority.keyID }
func (authority *testAuthority) PublicKey() []byte {
	return append([]byte(nil), authority.publicKey...)
}
func (authority *testAuthority) Sign(_ context.Context, message []byte) ([]byte, error) {
	return ed25519.Sign(authority.privateKey, message), nil
}
func (authority *testAuthority) VerifyTrusted(_ context.Context, keyID string, publicKey, message, signature []byte) error {
	if keyID != authority.keyID || !bytes.Equal(publicKey, authority.publicKey) || !ed25519.Verify(authority.publicKey, message, signature) {
		return errors.New("untrusted authority")
	}
	return nil
}

type memoryStore struct {
	records map[string]domainsecurity.CaseThreadAuthorityRecord
	puts    int
}

func newMemoryStore() *memoryStore {
	return &memoryStore{records: map[string]domainsecurity.CaseThreadAuthorityRecord{}}
}

func (store *memoryStore) PutIfAbsent(_ context.Context, record domainsecurity.CaseThreadAuthorityRecord) error {
	if current, found := store.records[record.RecordDigest]; found {
		if reflect.DeepEqual(current, record) {
			return nil
		}
		return errors.New("record conflict")
	}
	store.records[record.RecordDigest] = record
	store.puts++
	return nil
}

func (store *memoryStore) List(context.Context) ([]domainsecurity.CaseThreadAuthorityRecord, error) {
	records := make([]domainsecurity.CaseThreadAuthorityRecord, 0, len(store.records))
	for _, record := range store.records {
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].RecordDigest < records[j].RecordDigest })
	return records, nil
}

func (store *memoryStore) HasRecords(context.Context) (bool, error) {
	return len(store.records) != 0, nil
}

type threadReaderStub struct {
	threads map[string]map[string]any
}

func (reader *threadReaderStub) AllThreadIDs() ([]string, error) {
	ids := make([]string, 0, len(reader.threads))
	for threadID := range reader.threads {
		ids = append(ids, threadID)
	}
	sort.Strings(ids)
	return ids, nil
}

func (reader *threadReaderStub) GetThread(threadID string) (map[string]any, error) {
	thread, found := reader.threads[threadID]
	if !found {
		return nil, errors.New("thread missing")
	}
	return thread, nil
}

func testSecurityContext(threadID, turnID string, epoch int) domainsecurity.TurnSecurityContext {
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/cases/a", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-a")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-a"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-a")), ContextEpoch: uint64(epoch), IssuedAt: time.Unix(int64(epoch), 0),
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
	})
	if err != nil {
		panic(err)
	}
	return securityContext
}

func testBoundarySecurityContext(threadID, turnID string, epoch int) domainsecurity.TurnSecurityContext {
	publication, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: domainsecurity.SHA256Hex([]byte("risk-policy:" + threadID)), RiskClass: domainsecurity.RiskClassCase,
		Disposition: domainsecurity.PublicationDispositionCaseBoundaryOnly, CaseBindingState: domainsecurity.CaseBindingStateMissing,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("binding-observation:missing:" + threadID)),
		BlockerCode:              domainsecurity.PublicationBlockerCaseBindingMissing,
	})
	if err != nil {
		panic(err)
	}
	binding, err := securitycontexttest.WitnessedRiskBinding(threadID, "/cases/a", domainsecurity.RiskClassCase, publication.ThreadRiskPolicyDigest)
	if err != nil {
		panic(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/cases/a", TenantID: domainsecurity.LocalTenantID,
		UserID: domainsecurity.LocalUserID, CaseID: domainsecurity.UnboundCaseID,
		CaseBindingHash: domainsecurity.UnboundCaseBindingHash("/cases/a"), DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID,
		SourceManifestHash: domainsecurity.EmptySourceManifestHash, ContextEpoch: uint64(epoch), IssuedAt: time.Unix(int64(epoch), 0),
		PublicationPolicy: publication, RiskAuthorityBinding: binding,
	})
	if err != nil {
		panic(err)
	}
	return securityContext
}

func testGeneralSecurityContext(threadID, turnID string, epoch int) domainsecurity.TurnSecurityContext {
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/cases/a", TenantID: domainsecurity.LocalTenantID,
		UserID: domainsecurity.LocalUserID, CaseID: domainsecurity.UnboundCaseID,
		CaseBindingHash: domainsecurity.UnboundCaseBindingHash("/cases/a"), DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID,
		SourceManifestHash: domainsecurity.EmptySourceManifestHash, ContextEpoch: uint64(epoch), IssuedAt: time.Unix(int64(epoch), 0),
	})
	if err != nil {
		panic(err)
	}
	return securityContext
}

func signedLegacyContextRecord(t *testing.T, authority *testAuthority, securityContext domainsecurity.TurnSecurityContext) domainsecurity.CaseThreadAuthorityRecord {
	t.Helper()
	record := domainsecurity.CaseThreadAuthorityRecord{
		SchemaVersion: domainsecurity.CaseThreadAuthorityRecordVersion, AuthorityPurpose: domainsecurity.CaseThreadAuthorityPurpose,
		AuthorityAlgorithm: domainsecurity.CaseThreadAuthorityAlgorithm, AuthorityKeyID: authority.keyID,
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(authority.publicKey), SecurityContext: &securityContext,
	}
	record.AuthoritySignature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(authority.privateKey, domainsecurity.CaseThreadAuthoritySigningBytes(record)))
	body, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	record.RecordDigest = domainsecurity.SHA256Hex(body)
	if err := domainsecurity.ValidateCaseThreadAuthorityRecord(record); err != nil {
		t.Fatalf("legacy audit fixture is invalid: %v", err)
	}
	return record
}
