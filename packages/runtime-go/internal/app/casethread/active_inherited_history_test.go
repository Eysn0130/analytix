package casethread

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type activeHistoryTestAuthorityV1 struct {
	*testAuthority
	signs   int
	signErr error
}

func (authority *activeHistoryTestAuthorityV1) Sign(ctx context.Context, body []byte) ([]byte, error) {
	authority.signs++
	if authority.signErr != nil {
		return nil, authority.signErr
	}
	return authority.testAuthority.Sign(ctx, body)
}

type activeHistoryTestStoreV1 struct {
	*memoryStore
	putErr error
}

func (store *activeHistoryTestStoreV1) PutIfAbsent(ctx context.Context, record domainsecurity.CaseThreadAuthorityRecord) error {
	if store.putErr != nil {
		return store.putErr
	}
	return store.memoryStore.PutIfAbsent(ctx, record)
}

type activeHistoryRegistryFixtureV1 struct {
	registry  *Registry
	authority *activeHistoryTestAuthorityV1
	store     *activeHistoryTestStoreV1
	source    map[string]any
	binding   domainsecurity.ActiveInheritedHistoryBindingV1
}

func newActiveHistoryRegistryFixtureV1(t *testing.T) activeHistoryRegistryFixtureV1 {
	t.Helper()
	ctx := context.Background()
	authority := &activeHistoryTestAuthorityV1{testAuthority: newTestAuthority(113)}
	store := &activeHistoryTestStoreV1{memoryStore: newMemoryStore()}
	registry, err := NewRegistry(ctx, authority, store)
	if err != nil {
		t.Fatal(err)
	}
	securityContext := testSecurityContext("active-source", "source-current-turn", 3)
	if err := registry.Register(ctx, securityContext); err != nil {
		t.Fatal(err)
	}
	state, err := contextepochapp.BootstrapState(securityContext.ThreadID, securityContext.ContextEpoch,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(securityContext)}, time.Unix(5, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Commit(ctx, securityContext, state, time.Unix(6, 0)); err != nil {
		t.Fatal(err)
	}
	var committedDigest string
	for _, record := range store.records {
		if domainsecurity.CaseThreadAuthorityIsCommittedContext(record) {
			committedDigest = record.RecordDigest
		}
	}
	if committedDigest == "" {
		t.Fatal("fixture did not commit source authority")
	}
	registry.SetActiveInheritedHistoryIdentityValidatorV1(func(context.Context, string, []string) error { return nil })
	turns := []domainsecurity.ActiveInheritedTurnV1{
		{TurnID: "inherited-turn-1", ContentSHA256: domainsecurity.SHA256Hex([]byte("exact inert first turn"))},
		{TurnID: "inherited-turn-2", ContentSHA256: domainsecurity.SHA256Hex([]byte("exact inert second turn"))},
	}
	return activeHistoryRegistryFixtureV1{
		registry: registry, authority: authority, store: store,
		source: map[string]any{"id": securityContext.ThreadID, "securityState": securityContext, "contextEpochState": state},
		binding: domainsecurity.ActiveInheritedHistoryBindingV1{
			SchemaVersion: 1, Purpose: domainsecurity.ActiveInheritedHistoryPurposeV1,
			SourceThreadID: securityContext.ThreadID, SourcePrimarySHA256: domainsecurity.SHA256Hex([]byte("anchored immutable source primary")),
			SourceAuthorityRecordDigest: committedDigest, TargetThreadID: "active-derived", Derivation: "fork",
			CutoffTurnID: "inherited-turn-2", SourceTurnCount: 3, TargetCreatedAt: "2026-09-09T01:02:03Z", TargetRelation: "fork",
			Turns: turns, InventoryDigest: domainsecurity.ActiveInheritedHistoryInventoryDigestV1(turns),
		},
	}
}

func deriveActiveHistoryFixtureV1(t *testing.T, fixture activeHistoryRegistryFixtureV1) domainsecurity.CaseThreadAuthorityRecord {
	t.Helper()
	record, err := fixture.registry.DeriveWithInheritedHistoryV1(context.Background(), fixture.binding)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func TestRegistryActiveInheritedHistorySourceAdmissionRequiresExactCommitV1(t *testing.T) {
	fixture := newActiveHistoryRegistryFixtureV1(t)
	beforeSigns, beforePuts := fixture.authority.signs, fixture.store.puts
	digest, err := fixture.registry.SourceAdmissionDigestV1(fixture.source)
	if err != nil || digest != fixture.binding.SourceAuthorityRecordDigest {
		t.Fatalf("source observation did not select exact committed record: digest=%s err=%v", digest, err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"different route": func(source map[string]any) { source["id"] = "another-source" },
		"uncommitted current context": func(source map[string]any) {
			source["securityState"] = testSecurityContext("active-source", "source-current-turn", 4)
		},
		"missing context": func(source map[string]any) { delete(source, "securityState") },
		"missing epoch":   func(source map[string]any) { delete(source, "contextEpochState") },
		"different committed state": func(source map[string]any) {
			changed, err := contextepochapp.BootstrapState("active-source", 4,
				[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(testSecurityContext("active-source", "source-current-turn", 4))}, time.Unix(7, 0))
			if err != nil {
				t.Fatal(err)
			}
			source["contextEpochState"] = changed
		},
	} {
		t.Run(name, func(t *testing.T) {
			source := make(map[string]any, len(fixture.source))
			for key, value := range fixture.source {
				source[key] = value
			}
			mutate(source)
			if digest, err := fixture.registry.SourceAdmissionDigestV1(source); err == nil || digest != "" {
				t.Fatal("uncommitted or mismatched source received admission")
			}
		})
	}
	if fixture.authority.signs != beforeSigns || fixture.store.puts != beforePuts {
		t.Fatal("source admission observation signed or wrote authority")
	}
}

func TestRegistryActiveInheritedHistorySurvivesFreshReadOnlyRegistryV1(t *testing.T) {
	fixture := newActiveHistoryRegistryFixtureV1(t)
	beforeSigns, beforePuts := fixture.authority.signs, fixture.store.puts
	record := deriveActiveHistoryFixtureV1(t, fixture)
	if fixture.authority.signs != beforeSigns+1 || fixture.store.puts != beforePuts+1 ||
		!reflect.DeepEqual(fixture.store.records[record.RecordDigest], record) || domainsecurity.CaseThreadAuthorityCanAuthorizeExecution(record) {
		t.Fatal("derivation did not persist exactly one inert installation-signed receipt")
	}
	beforeSigns, beforePuts = fixture.authority.signs, fixture.store.puts
	fresh, err := NewRegistry(context.Background(), fixture.authority, fixture.store)
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := fresh.ActiveInheritedHistoryRecordV1(context.Background(), record.ThreadID); err == nil || found {
		t.Fatal("fresh lookup admitted history without its existing-identity observer")
	}
	checks := 0
	fresh.SetActiveInheritedHistoryIdentityValidatorV1(func(_ context.Context, target string, ids []string) error {
		checks++
		if target != record.ThreadID || !reflect.DeepEqual(ids, []string{"inherited-turn-1", "inherited-turn-2"}) {
			t.Fatal("fresh observer received another target or incomplete identity inventory")
		}
		return nil
	})
	for index := 0; index < 2; index++ {
		observed, found, err := fresh.ActiveInheritedHistoryRecordV1(context.Background(), record.ThreadID)
		if err != nil || !found || !reflect.DeepEqual(observed, record) {
			t.Fatalf("fresh signed lookup failed: found=%t err=%v", found, err)
		}
		observed.ActiveInheritedHistory.Turns[0].ContentSHA256 = domainsecurity.SHA256Hex([]byte("caller changes returned value"))
	}
	if checks != 2 || fixture.authority.signs != beforeSigns || fixture.store.puts != beforePuts {
		t.Fatal("fresh observation reconstructed authority or failed to recheck identities")
	}
	if _, err := NewRegistry(context.Background(), newTestAuthority(114), fixture.store); err == nil {
		t.Fatal("fresh inherited history trusted another installation")
	}
}

func TestRegistryActiveInheritedHistoryRejectsReusedTargetsAndMissingSourcesV1(t *testing.T) {
	for _, scenario := range []string{"duplicate target", "legacy lineage", "missing source", "prepared source", "missing observer"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newActiveHistoryRegistryFixtureV1(t)
			switch scenario {
			case "duplicate target":
				deriveActiveHistoryFixtureV1(t, fixture)
			case "legacy lineage":
				if err := fixture.registry.Derive(context.Background(), fixture.binding.SourceThreadID, fixture.binding.TargetThreadID, "fork"); err != nil {
					t.Fatal(err)
				}
			case "missing source":
				fixture.binding.SourceAuthorityRecordDigest = domainsecurity.SHA256Hex([]byte("missing source receipt"))
			case "prepared source":
				for _, record := range fixture.store.records {
					if record.SecurityContext != nil && !domainsecurity.CaseThreadAuthorityIsCommittedContext(record) {
						fixture.binding.SourceAuthorityRecordDigest = record.RecordDigest
					}
				}
			case "missing observer":
				fixture.registry.SetActiveInheritedHistoryIdentityValidatorV1(nil)
			}
			beforeSigns, beforePuts, beforeCount := fixture.authority.signs, fixture.store.puts, len(fixture.store.records)
			if _, err := fixture.registry.DeriveWithInheritedHistoryV1(context.Background(), fixture.binding); err == nil {
				t.Fatal("inadmissible derivation was signed")
			}
			if fixture.authority.signs != beforeSigns || fixture.store.puts != beforePuts || len(fixture.store.records) != beforeCount {
				t.Fatal("rejected derivation mutated signed authority")
			}
		})
	}
}

func TestRegistryActiveInheritedHistoryCancellationAndCommitFailureV1(t *testing.T) {
	for _, scenario := range []string{"cancelled", "sign failure", "CAS write failure"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newActiveHistoryRegistryFixtureV1(t)
			ctx := context.Background()
			sentinel := errors.New("synthetic inherited history commit failure")
			switch scenario {
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "sign failure":
				fixture.authority.signErr = sentinel
			case "CAS write failure":
				fixture.store.putErr = sentinel
			}
			beforeSigns, beforePuts, beforeCount := fixture.authority.signs, fixture.store.puts, len(fixture.store.records)
			record, err := fixture.registry.DeriveWithInheritedHistoryV1(ctx, fixture.binding)
			if err == nil || record.RecordDigest != "" || fixture.registry.IsCaseThread(fixture.binding.TargetThreadID) {
				t.Fatal("failed signing/commit reported active authority")
			}
			if scenario == "cancelled" && (!errors.Is(err, context.Canceled) || fixture.authority.signs != beforeSigns) {
				t.Fatalf("cancelled operation signed or lost cancellation: %v", err)
			}
			if scenario == "CAS write failure" && !errors.Is(err, sentinel) {
				t.Fatalf("CAS failure was hidden: %v", err)
			}
			if fixture.store.puts != beforePuts || len(fixture.store.records) != beforeCount {
				t.Fatal("failed operation changed durable inventory")
			}
		})
	}
}

func TestRegistryActiveInheritedHistoryRechecksDurableProofV1(t *testing.T) {
	for _, fault := range []string{"deleted proof", "deleted source", "tampered proof"} {
		t.Run(fault, func(t *testing.T) {
			fixture := newActiveHistoryRegistryFixtureV1(t)
			record := deriveActiveHistoryFixtureV1(t, fixture)
			switch fault {
			case "deleted proof":
				delete(fixture.store.records, record.RecordDigest)
			case "deleted source":
				delete(fixture.store.records, record.ParentRecordDigest)
			case "tampered proof":
				tampered := record
				tampered.ActiveInheritedHistory = domainsecurity.CloneActiveInheritedHistoryBindingV1(record.ActiveInheritedHistory)
				tampered.ActiveInheritedHistory.SourcePrimarySHA256 = domainsecurity.SHA256Hex([]byte("substituted source state"))
				fixture.store.records[record.RecordDigest] = tampered
			}
			beforeSigns, beforePuts := fixture.authority.signs, fixture.store.puts
			if _, found, err := fixture.registry.ActiveInheritedHistoryRecordV1(context.Background(), record.ThreadID); err == nil || found {
				t.Fatal("in-memory lineage concealed missing or tampered durable proof")
			}
			if fixture.authority.signs != beforeSigns || fixture.store.puts != beforePuts {
				t.Fatal("read repaired missing inherited proof")
			}
		})
	}
}

func TestRegistryActiveInheritedHistoryRejectsTargetExecutionIdentityCollisionV1(t *testing.T) {
	fixture := newActiveHistoryRegistryFixtureV1(t)
	record := deriveActiveHistoryFixtureV1(t, fixture)
	beforeSigns, beforePuts, beforeCount := fixture.authority.signs, fixture.store.puts, len(fixture.store.records)
	conflicting := testSecurityContext(record.ThreadID, "inherited-turn-1", 1)
	if err := fixture.registry.Register(context.Background(), conflicting); err == nil {
		t.Fatal("inert inherited turn acquired target execution context")
	}
	if fixture.authority.signs != beforeSigns || fixture.store.puts != beforePuts || len(fixture.store.records) != beforeCount {
		t.Fatal("rejected identity collision poisoned durable inventory")
	}
	fresh, err := NewRegistry(context.Background(), fixture.authority, fixture.store)
	if err != nil {
		t.Fatal(err)
	}
	fresh.SetActiveInheritedHistoryIdentityValidatorV1(func(context.Context, string, []string) error { return nil })
	if _, found, err := fresh.ActiveInheritedHistoryRecordV1(context.Background(), record.ThreadID); err != nil || !found {
		t.Fatalf("rejected collision damaged fresh inherited authority: %v", err)
	}
	// Independently signed context bytes inserted after activation must also be
	// detected by read-only admission and by fresh inventory construction.
	poison, err := domainsecurity.NewCaseThreadAuthorityRecord(conflicting, fixture.authority.KeyID(), fixture.authority.PublicKey(), func(body []byte) ([]byte, error) {
		return fixture.authority.testAuthority.Sign(context.Background(), body)
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.store.records[poison.RecordDigest] = poison
	if _, found, err := fixture.registry.ActiveInheritedHistoryRecordV1(context.Background(), record.ThreadID); err == nil || found {
		t.Fatal("current inherited lookup ignored target execution identity collision")
	}
	if _, err := NewRegistry(context.Background(), fixture.authority, fixture.store); err == nil {
		t.Fatal("fresh inventory admitted conflicting inert/execution identities")
	}
}

func TestRegistryActiveInheritedHistoryHonorsExistingOwnerCollisionObserverV1(t *testing.T) {
	for _, owner := range []string{"pending", "child"} {
		t.Run(owner, func(t *testing.T) {
			fixture := newActiveHistoryRegistryFixtureV1(t)
			sentinel := errors.New("synthetic " + owner + " identity collision")
			check := func(_ context.Context, target string, ids []string) error {
				if target != fixture.binding.TargetThreadID || !reflect.DeepEqual(ids, []string{"inherited-turn-1", "inherited-turn-2"}) {
					t.Fatal("collision observer did not receive exact target prefix identities")
				}
				return sentinel
			}
			fixture.registry.SetActiveInheritedHistoryIdentityValidatorV1(check)
			beforeSigns, beforePuts := fixture.authority.signs, fixture.store.puts
			if _, err := fixture.registry.DeriveWithInheritedHistoryV1(context.Background(), fixture.binding); !errors.Is(err, sentinel) {
				t.Fatalf("producer ignored %s owner: %v", owner, err)
			}
			if fixture.authority.signs != beforeSigns || fixture.store.puts != beforePuts {
				t.Fatal("collision observer rejection followed signing or persistence")
			}
			fixture.registry.SetActiveInheritedHistoryIdentityValidatorV1(func(context.Context, string, []string) error { return nil })
			record := deriveActiveHistoryFixtureV1(t, fixture)
			fixture.registry.SetActiveInheritedHistoryIdentityValidatorV1(check)
			beforeSigns, beforePuts = fixture.authority.signs, fixture.store.puts
			if _, found, err := fixture.registry.ActiveInheritedHistoryRecordV1(context.Background(), record.ThreadID); !errors.Is(err, sentinel) || found {
				t.Fatalf("reader ignored current %s collision: %v", owner, err)
			}
			if fixture.authority.signs != beforeSigns || fixture.store.puts != beforePuts {
				t.Fatal("collision lookup mutated authority")
			}
		})
	}
}

func TestRegistryActiveInheritedHistoryLegacyReceiptsCannotAliasBindingV1(t *testing.T) {
	for _, method := range []string{"derive", "verify"} {
		t.Run(method, func(t *testing.T) {
			fixture := newActiveHistoryRegistryFixtureV1(t)
			record := deriveActiveHistoryFixtureV1(t, fixture)
			beforeSigns, beforePuts := fixture.authority.signs, fixture.store.puts
			var returned domainsecurity.CaseThreadAuthorityRecord
			var err error
			if method == "derive" {
				returned, err = fixture.registry.DeriveWithReceipt(context.Background(), record.ParentThreadID, record.ThreadID, record.Derivation)
			} else {
				returned, err = fixture.registry.VerifyDerivedLineageReceipt(context.Background(), record.ParentThreadID, record.ThreadID, record.Derivation, record.RecordDigest)
			}
			if err != nil || !reflect.DeepEqual(returned, record) {
				t.Fatalf("legacy receipt lookup failed: %v", err)
			}
			returned.ActiveInheritedHistory.SourcePrimarySHA256 = domainsecurity.SHA256Hex([]byte("caller substituted source"))
			returned.ActiveInheritedHistory.Turns[0].ContentSHA256 = domainsecurity.SHA256Hex([]byte("caller substituted inherited content"))
			observed, found, err := fixture.registry.ActiveInheritedHistoryRecordV1(context.Background(), record.ThreadID)
			if err != nil || !found || !reflect.DeepEqual(observed, record) {
				t.Fatalf("legacy return allowed caller to mutate signed registry state: %v", err)
			}
			if fixture.authority.signs != beforeSigns || fixture.store.puts != beforePuts {
				t.Fatal("legacy receipt observation signed or wrote authority")
			}
		})
	}
}

func TestRegistryActiveInheritedHistoryRechecksFullAncestryV1(t *testing.T) {
	fixture := newActiveHistoryRegistryFixtureV1(t)
	parent := deriveActiveHistoryFixtureV1(t, fixture)
	childBinding := *domainsecurity.CloneActiveInheritedHistoryBindingV1(&fixture.binding)
	childBinding.SourceThreadID = parent.ThreadID
	childBinding.SourceAuthorityRecordDigest = parent.RecordDigest
	childBinding.SourcePrimarySHA256 = domainsecurity.SHA256Hex([]byte("immutable derived source primary"))
	childBinding.TargetThreadID = "active-grandchild"
	childBinding.SourceTurnCount = len(childBinding.Turns)
	child, err := fixture.registry.DeriveWithInheritedHistoryV1(context.Background(), childBinding)
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := fixture.registry.ActiveInheritedHistoryRecordV1(context.Background(), child.ThreadID); err != nil || !found {
		t.Fatalf("complete multi-generation lineage was not readable: %v", err)
	}
	delete(fixture.store.records, parent.ParentRecordDigest)
	beforeSigns, beforePuts := fixture.authority.signs, fixture.store.puts
	if _, found, err := fixture.registry.ActiveInheritedHistoryRecordV1(context.Background(), child.ThreadID); err == nil || found {
		t.Fatal("live read trusted direct parent while its committed ancestor was missing")
	}
	if _, err := NewRegistry(context.Background(), fixture.authority, fixture.store); err == nil {
		t.Fatal("fresh registry accepted a lineage with missing committed ancestor")
	}
	childBinding.TargetThreadID = "another-grandchild"
	if _, err := fixture.registry.DeriveWithInheritedHistoryV1(context.Background(), childBinding); err == nil {
		t.Fatal("derivation signed through a missing committed ancestor")
	}
	if fixture.authority.signs != beforeSigns || fixture.store.puts != beforePuts {
		t.Fatal("missing-ancestor rejection signed or reconstructed authority")
	}
}

func TestRegistryActiveInheritedHistoryRejectsDeletedParentBeforeSigningV1(t *testing.T) {
	fixture := newActiveHistoryRegistryFixtureV1(t)
	delete(fixture.store.records, fixture.binding.SourceAuthorityRecordDigest)
	beforeSigns, beforePuts, beforeCount := fixture.authority.signs, fixture.store.puts, len(fixture.store.records)
	record, err := fixture.registry.DeriveWithInheritedHistoryV1(context.Background(), fixture.binding)
	if err == nil || record.RecordDigest != "" || fixture.registry.IsCaseThread(fixture.binding.TargetThreadID) {
		t.Fatal("cached source authority concealed deleted durable parent")
	}
	if fixture.authority.signs != beforeSigns || fixture.store.puts != beforePuts || len(fixture.store.records) != beforeCount {
		t.Fatal("missing-parent derivation signed or wrote authority")
	}
}
