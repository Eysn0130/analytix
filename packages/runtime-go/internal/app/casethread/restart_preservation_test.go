package casethread

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type restartCountingAuthorityV1 struct {
	*testAuthority
	signs int
}

func (authority *restartCountingAuthorityV1) Sign(ctx context.Context, body []byte) ([]byte, error) {
	authority.signs++
	return authority.testAuthority.Sign(ctx, body)
}

type restartNoReadStoreV1 struct{ reads, writes int }

func TestRegistryOriginalRestartObservationPreservesAuthorityAndOverlayBoundary(t *testing.T) {
	for _, scenario := range []string{"committed", "original_migration", "staged_only", "independent_future_overlay", "independent_applied_overlay", "held_future_overlay", "held_future_applied", "removed_original", "foreign_record", "wrong_context", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			authority := &restartCountingAuthorityV1{testAuthority: newTestAuthority(61)}
			store := newMemoryStore()
			registry, err := NewRegistry(ctx, authority, store)
			if err != nil {
				t.Fatal(err)
			}
			frozen := testSecurityContext("held", "held-turn", 1)
			if err := registry.Register(ctx, frozen); err != nil {
				t.Fatal(err)
			}
			if scenario != "staged_only" && scenario != "original_migration" {
				state, err := contextepochapp.BootstrapState(frozen.ThreadID, frozen.ContextEpoch, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(frozen)}, time.Unix(5, 0))
				if err != nil {
					t.Fatal(err)
				}
				if err := registry.Commit(ctx, frozen, state, time.Unix(6, 0)); err != nil {
					t.Fatal(err)
				}
			}
			reader := registry
			var plan MigrationPlan
			if strings.Contains(scenario, "overlay") || scenario == "held_future_applied" || scenario == "original_migration" {
				planned := testSecurityContext("independent", "next-turn", 1)
				if strings.HasPrefix(scenario, "held_future") {
					planned = testSecurityContext("held", "future-held-turn", 2)
				}
				if scenario == "original_migration" {
					planned = frozen
				}
				plan, err = registry.PlanContexts(ctx, []domainsecurity.TurnSecurityContext{planned})
				if err != nil {
					t.Fatal(err)
				}
				reader, err = registry.WithPlan(ctx, plan)
				if err != nil {
					t.Fatal(err)
				}
				if strings.HasPrefix(scenario, "held_future") {
					frozen = planned
				}
			}
			if err := registry.PreserveRestartScopeV1(ctx, []string{"held"}); err != nil {
				t.Fatal(err)
			}
			if scenario == "independent_applied_overlay" {
				if err := reader.ApplyPlan(ctx, plan); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "held_future_applied" {
				// An external synthetic write cannot turn future membership into original.
				for _, record := range plan.Records {
					if err := store.PutIfAbsent(ctx, record); err != nil {
						t.Fatal(err)
					}
				}
			}
			if scenario == "removed_original" {
				for id := range store.records {
					delete(store.records, id)
					break
				}
			}
			if scenario == "foreign_record" {
				foreignStore := newMemoryStore()
				foreign, err := NewRegistry(ctx, newTestAuthority(62), foreignStore)
				if err != nil {
					t.Fatal(err)
				}
				if err := foreign.Register(ctx, testSecurityContext("independent-foreign", "turn", 1)); err != nil {
					t.Fatal(err)
				}
				for id, record := range foreignStore.records {
					store.records[id] = record
				}
			}
			if scenario == "wrong_context" {
				frozen = testSecurityContext("held", "held-turn", 3)
			}
			if scenario == "cancelled" {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			before, err := json.Marshal(store.records)
			if err != nil {
				t.Fatal(err)
			}
			signs, puts := authority.signs, store.puts
			err = reader.ObserveOriginalContextForRestartV1(ctx, frozen)
			positive := scenario == "committed" || scenario == "original_migration" || scenario == "independent_future_overlay" || scenario == "independent_applied_overlay"
			if positive && err != nil || !positive && err == nil {
				t.Fatalf("original private observation crossed its boundary: %v", err)
			}
			if reader.ContainsContext(frozen) || reader.CanExecute(frozen.ThreadID) {
				t.Fatal("original observation restored publication or execution authority")
			}
			after, _ := json.Marshal(store.records)
			if !reflect.DeepEqual(before, after) || authority.signs != signs || store.puts != puts {
				t.Fatal("original observation signed or wrote authority")
			}
		})
	}
}

func (store *restartNoReadStoreV1) AllThreadIDs() ([]string, error) {
	return []string{"held", "unindexed-held"}, nil
}

func (store *restartNoReadStoreV1) GetThread(string) (map[string]any, error) {
	store.reads++
	return nil, errors.New("held primary must not be read by this recovery writer")
}

func (store *restartNoReadStoreV1) GetThreadForAuthorityRepair(id string) (map[string]any, error) {
	return store.GetThread(id)
}

func (store *restartNoReadStoreV1) ReplaceThreadForAuthorityRepair(string, map[string]any) error {
	store.writes++
	return errors.New("held primary must not be rewritten")
}

func TestRegistryRestartPreservationDeniesEffectsAndKeepsFullAuthorityInventory(t *testing.T) {
	ctx := context.Background()
	authority := &restartCountingAuthorityV1{testAuthority: newTestAuthority(51)}
	store := newMemoryStore()
	registry, err := NewRegistry(ctx, authority, store)
	if err != nil {
		t.Fatal(err)
	}
	frozen := testSecurityContext("held", "held-turn", 1)
	if err := registry.Register(ctx, frozen); err != nil {
		t.Fatal(err)
	}
	state, err := contextepochapp.BootstrapState(frozen.ThreadID, frozen.ContextEpoch, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(frozen)}, time.Unix(5, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Commit(ctx, frozen, state, time.Unix(6, 0)); err != nil {
		t.Fatal(err)
	}
	lineage, err := registry.DeriveWithReceipt(ctx, "held", "existing-child", "fork")
	if err != nil {
		t.Fatal(err)
	}
	active := testSecurityContext("active", "active-turn", 1)
	if err := registry.Register(ctx, active); err != nil {
		t.Fatal(err)
	}
	beforeInventory := registry.CommittedContexts()
	beforeRecords, _ := json.Marshal(store.records)
	beforeSigns, beforePuts := authority.signs, store.puts
	ids := []string{"held", "unindexed-held"}
	if err := registry.PreserveRestartScopeV1(ctx, ids); err != nil {
		t.Fatal(err)
	}
	ids[0] = "active"
	if !registry.IsCaseThread("held") || registry.IsCaseThread("unindexed-held") || !reflect.DeepEqual(beforeInventory, registry.CommittedContexts()) || len(registry.ContextTurnIDs("held")) != 1 {
		t.Fatal("preservation erased or invented original case authority")
	}
	if _, found := registry.CommittedContext("held", "held-turn"); !found {
		t.Fatal("original committed context disappeared")
	}
	for _, id := range []string{"held", "unindexed-held", " held "} {
		if !registry.RestartPreservesThreadV1(id) || registry.CanExecute(id) {
			t.Fatal("held thread remains executable")
		}
		if !errors.Is(RequireExecutable(registry, id), ErrRestartPreserved) || !errors.Is(ValidateRestartState(registry, id, nil), ErrRestartPreserved) {
			t.Fatal("classification bypassed the hold")
		}
	}
	if registry.ContainsContext(frozen) {
		t.Fatal("held context still grants publication authority")
	}
	general := testGeneralSecurityContext("unindexed-held", "general-turn", 1)
	checks := []func() error{
		func() error { return registry.Register(ctx, frozen) },
		func() error { return registry.Commit(ctx, frozen, state, time.Unix(6, 0)) },
		func() error { return registry.Derive(ctx, "held", "existing-child", "fork") },
		func() error { return registry.Derive(ctx, "held", "new-child", "fork") },
		func() error { return registry.Derive(ctx, "active", "unindexed-held", "resume") },
		func() error {
			_, err := registry.VerifyDerivedLineageReceipt(ctx, "held", "existing-child", "fork", lineage.RecordDigest)
			return err
		},
		func() error { return RegisterRequired(ctx, registry, general) },
		func() error { return CommitRequired(ctx, registry, general, domaincontextepoch.State{}, time.Time{}) },
		func() error { return RegistrationHook(registry)(ctx, general) },
	}
	for i, check := range checks {
		if !errors.Is(check(), ErrRestartPreserved) {
			t.Fatalf("writer %d did not preserve the hold", i)
		}
	}
	repair := &restartNoReadStoreV1{}
	if err := RepairCommittedContexts(registry, repair); err != nil {
		t.Fatal(err)
	}
	inventory, err := PreflightRestartInventory(registry, repair)
	if err != nil || len(inventory.Quarantined) != 0 || repair.reads != 0 || repair.writes != 0 {
		t.Fatalf("hold became recovery/quarantine work: %+v %v", inventory, err)
	}
	if err := ApplyRestartInventory(registry, inventory); err != nil {
		t.Fatal(err)
	}
	registry.ReplaceQuarantine(map[string]string{})
	if registry.CanExecute("held") || registry.ContainsContext(frozen) {
		t.Fatal("quarantine replacement released preservation")
	}
	afterRecords, _ := json.Marshal(store.records)
	if string(beforeRecords) != string(afterRecords) || beforeSigns != authority.signs || beforePuts != store.puts {
		t.Fatal("held operations signed or persisted authority")
	}
	if err := registry.Register(ctx, testSecurityContext("active", "next-active-turn", 2)); err != nil {
		t.Fatalf("independent case writer failed: %v", err)
	}
	if authority.signs != beforeSigns+1 || store.puts != beforePuts+1 {
		t.Fatal("independent case control did not execute")
	}
}

func TestRegistryRestartPreservationRejectsOldPlanAndSurvivesOldOverlay(t *testing.T) {
	ctx := context.Background()
	authority := &restartCountingAuthorityV1{testAuthority: newTestAuthority(52)}
	store := newMemoryStore()
	registry, err := NewRegistry(ctx, authority, store)
	if err != nil {
		t.Fatal(err)
	}
	held := testSecurityContext("held", "held-turn", 1)
	active := testSecurityContext("active", "active-turn", 1)
	oldPlan, err := registry.PlanContexts(ctx, []domainsecurity.TurnSecurityContext{active, held})
	if err != nil {
		t.Fatal(err)
	}
	overlay, err := registry.WithPlan(ctx, oldPlan)
	if err != nil || !overlay.ContainsContext(held) {
		t.Fatalf("old overlay fixture lacks authority: %v", err)
	}
	signs := authority.signs
	if err := registry.PreserveRestartScopeV1(ctx, []string{"held"}); err != nil {
		t.Fatal(err)
	}
	if !overlay.RestartPreservesThreadV1("held") || overlay.ContainsContext(held) || overlay.CanExecute("held") || !overlay.IsCaseThread("held") {
		t.Fatal("preexisting overlay escaped the shared hold")
	}
	if _, err := registry.WithPlan(ctx, oldPlan); !errors.Is(err, ErrRestartPreserved) {
		t.Fatal("old plan reissued an overlay")
	}
	for _, candidate := range []*Registry{registry, overlay} {
		if err := candidate.ApplyPlan(ctx, oldPlan); !errors.Is(err, ErrRestartPreserved) {
			t.Fatal("old plan was applied")
		}
		candidate.ReplaceQuarantine(nil)
		if candidate.CanExecute("held") {
			t.Fatal("overlay quarantine reset released hold")
		}
	}
	if authority.signs != signs || store.puts != 0 {
		t.Fatal("rejected mixed plan wrote an independent prefix")
	}
	plan, err := registry.PlanContexts(ctx, []domainsecurity.TurnSecurityContext{held, active, active})
	if err != nil || len(plan.Contexts) != 1 || plan.Contexts[0] != active || len(plan.Records) != 1 || authority.signs != signs+1 {
		t.Fatalf("migration did not isolate the exact held scope: %+v %v", plan, err)
	}
	current, err := registry.WithPlan(ctx, plan)
	if err != nil || !current.ContainsContext(active) || current.ContainsContext(held) || !current.RestartPreservesThreadV1("held") {
		t.Fatalf("new overlay failed: %v", err)
	}
	if err := registry.ApplyPlan(ctx, plan); err != nil || store.puts != 1 {
		t.Fatalf("independent migration failed: %v", err)
	}
}

func TestRegistryRestartPreservationValidatesFullInputBeforeSigning(t *testing.T) {
	ctx := context.Background()
	authority := &restartCountingAuthorityV1{testAuthority: newTestAuthority(53)}
	registry, err := NewRegistry(ctx, authority, newMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	active := testSecurityContext("active", "turn", 1)
	held := testSecurityContext("held", "turn", 1)
	if err := registry.Register(ctx, held); err != nil {
		t.Fatal(err)
	}
	state, err := contextepochapp.BootstrapState(held.ThreadID, held.ContextEpoch, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(held)}, time.Unix(5, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Commit(ctx, held, state, time.Unix(6, 0)); err != nil {
		t.Fatal(err)
	}
	if err := registry.PreserveRestartScopeV1(ctx, []string{"held"}); err != nil {
		t.Fatal(err)
	}
	signs := authority.signs
	invalid := held
	invalid.ContextDigest = domainsecurity.SHA256Hex([]byte("tampered"))
	conflict := testSecurityContext("held", "turn", 2)
	for _, input := range [][]domainsecurity.TurnSecurityContext{{active, invalid}, {active, held, conflict}} {
		if _, err := registry.PlanContexts(ctx, input); err == nil {
			t.Fatal("hold excused malformed/conflicting full input")
		}
		if authority.signs != signs {
			t.Fatal("active prefix was signed before invalid held context was rejected")
		}
	}
}

func TestRegistryRestartPreservationRejectsInvalidScopeAndChangedInventory(t *testing.T) {
	ctx := context.Background()
	for _, ids := range [][]string{{""}, {" held"}, {"held", "held"}} {
		registry, _ := NewRegistry(ctx, newTestAuthority(54), newMemoryStore())
		if err := registry.PreserveRestartScopeV1(ctx, ids); err == nil {
			t.Fatal("invalid scope installed")
		}
		if err := registry.PreserveRestartScopeV1(ctx, []string{"held"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, corrupt := range []bool{false, true} {
		store := newMemoryStore()
		authority := newTestAuthority(55)
		registry, _ := NewRegistry(ctx, authority, store)
		other, _ := NewRegistry(ctx, authority, store)
		if err := other.Register(ctx, testSecurityContext("held", "turn", 1)); err != nil {
			t.Fatal(err)
		}
		if corrupt {
			for key, record := range store.records {
				record.AuthoritySignature = "tampered"
				store.records[key] = record
			}
		}
		if err := registry.PreserveRestartScopeV1(ctx, []string{"held"}); err == nil || registry.RestartPreservesThreadV1("held") {
			t.Fatal("changed full authority inventory became preservation authority")
		}
	}
	registry, _ := NewRegistry(ctx, newTestAuthority(56), newMemoryStore())
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := registry.PreserveRestartScopeV1(cancelled, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled scope installed")
	}
	if err := registry.PreserveRestartScopeV1(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err := registry.PreserveRestartScopeV1(ctx, []string{"held"}); err == nil {
		t.Fatal("empty one-shot scope was replaceable")
	}
}

func TestRegistryRestartPreservationConcurrentQueriesKeepSharedScope(t *testing.T) {
	ctx := context.Background()
	registry, _ := NewRegistry(ctx, newTestAuthority(57), newMemoryStore())
	overlay, err := registry.WithPlan(ctx, MigrationPlan{})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.PreserveRestartScopeV1(ctx, []string{"held"}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				overlay.ReplaceQuarantine(nil)
				if !overlay.RestartPreservesThreadV1("held") || overlay.CanExecute("held") {
					t.Error("concurrent query released hold")
				}
			}
		}()
	}
	wg.Wait()
}

type restartRepairBarrierStoreV1 struct {
	thread                     map[string]any
	entered, release           chan struct{}
	once                       sync.Once
	installed                  *atomic.Bool
	writes, writesAfterInstall int
}

func (store *restartRepairBarrierStoreV1) GetThreadForAuthorityRepair(string) (map[string]any, error) {
	store.once.Do(func() { close(store.entered); <-store.release })
	return store.thread, nil
}

func (store *restartRepairBarrierStoreV1) ReplaceThreadForAuthorityRepair(_ string, thread map[string]any) error {
	store.writes++
	if store.installed.Load() {
		store.writesAfterInstall++
	}
	store.thread = thread
	return nil
}

func TestRegistryRestartPreservationInstallationWaitsForInFlightContextRepair(t *testing.T) {
	ctx := context.Background()
	registry, err := NewRegistry(ctx, newTestAuthority(58), newMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	frozen := testSecurityContext("held", "turn", 1)
	if err := registry.Register(ctx, frozen); err != nil {
		t.Fatal(err)
	}
	state, err := contextepochapp.BootstrapState(frozen.ThreadID, frozen.ContextEpoch, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(frozen)}, time.Unix(5, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Commit(ctx, frozen, state, time.Unix(6, 0)); err != nil {
		t.Fatal(err)
	}
	installed := &atomic.Bool{}
	store := &restartRepairBarrierStoreV1{thread: map[string]any{"id": "held", "turns": []any{map[string]any{"id": "turn", "status": "running"}}}, entered: make(chan struct{}), release: make(chan struct{}), installed: installed}
	repaired := make(chan error, 1)
	go func() { repaired <- RepairCommittedContexts(registry, store) }()
	<-store.entered
	installing := make(chan struct{})
	preserved := make(chan error, 1)
	go func() {
		close(installing)
		err := registry.PreserveRestartScopeV1(ctx, []string{"held"})
		if err == nil {
			installed.Store(true)
		}
		preserved <- err
	}()
	<-installing
	returnedEarly := false
	select {
	case err := <-preserved:
		returnedEarly = true
		if err != nil {
			t.Error(err)
		}
	case <-time.After(100 * time.Millisecond):
	}
	close(store.release)
	if err := <-repaired; err != nil {
		t.Fatal(err)
	}
	if !returnedEarly {
		if err := <-preserved; err != nil {
			t.Fatal(err)
		}
	}
	if returnedEarly || store.writes != 1 || store.writesAfterInstall != 0 {
		t.Fatalf("hold installation crossed in-flight repair: early=%v writes=%d after-install=%d", returnedEarly, store.writes, store.writesAfterInstall)
	}
	if err := RepairCommittedContexts(registry, store); err != nil || store.writes != 1 {
		t.Fatalf("held retry mutated repaired primary: %v", err)
	}
}
