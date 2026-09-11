package caseentity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type countedAliasPreparationStoreV1 struct {
	*caseEntityMemoryStoreV1
	puts int
}

func (store *countedAliasPreparationStoreV1) PutThreadContextIfAbsent(ctx context.Context, record domaincaseentity.ThreadCaseContextRecord) error {
	store.puts++
	return store.caseEntityMemoryStoreV1.PutThreadContextIfAbsent(ctx, record)
}

func preparedCaseAliasFixtureV1(t *testing.T, epoch uint64) (*Service, *countedAliasPreparationStoreV1, domainsecurity.TurnSecurityContext, []domaincaseentity.ModelEntityAliasV1) {
	t.Helper()
	origin := caseEntityTestContextV1(t, caseEntityTestContextInputV1{ThreadID: "thread-origin", TurnID: "turn-origin", TenantID: "tenant-synthetic", UserID: "user-synthetic", CaseID: "case-synthetic", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "synthetic-snapshot", ContextEpoch: 1})
	store := &countedAliasPreparationStoreV1{caseEntityMemoryStoreV1: newCaseEntityMemoryStoreV1()}
	service := newPersistentCaseEntityServiceV1(&recordingKeyedDigesterV1{key: []byte("synthetic-installation-key")}, store)
	references := []domaincaseentity.ReferenceV1{}
	identities := []domaincaseentity.CaseEntityIdentityStateV1{}
	aliases := []domaincaseentity.ModelEntityAliasV1{}
	for ordinal := 1; ordinal <= 2; ordinal++ {
		reference, err := service.BindReferenceV1(context.Background(), NewDeriveReferenceInputV1(origin, domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1, fmt.Sprintf("62220200000000000%02d", ordinal)))
		if err != nil {
			t.Fatal(err)
		}
		record, err := store.ResolveBindingByStableOrdinal(context.Background(), origin, domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1, uint32(ordinal))
		if err != nil {
			t.Fatal(err)
		}
		alias, err := domaincaseentity.NewModelEntityAliasV1(record.EntityType, record.StableOrdinal)
		if err != nil {
			t.Fatal(err)
		}
		references = append(references, reference)
		aliases = append(aliases, alias)
		identities = append(identities, domaincaseentity.CaseEntityIdentityStateV1{Reference: reference, EntityType: record.EntityType, StableOrdinal: record.StableOrdinal})
	}
	index, err := domaincaseentity.NewCaseLongitudinalIndexRecordV1(domaincaseentity.ThreadCaseContextRecordInputV1{SecurityContext: origin, Generation: 1, EntityReferences: references, EntityIdentities: identities, Snapshots: []domaincaseentity.CaseSnapshotStateV1{{DatasetSnapshotID: origin.DatasetSnapshotID, ContextEpoch: origin.ContextEpoch, Currentness: domaincaseentity.SnapshotCurrentV1}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutThreadContextIfAbsent(context.Background(), index); err != nil {
		t.Fatal(err)
	}
	store.puts = 0
	current := caseEntityTestContextV1(t, caseEntityTestContextInputV1{ThreadID: "thread-target", TurnID: "turn-target", TenantID: origin.TenantID, UserID: origin.UserID, CaseID: origin.CaseID, CaseBindingHash: origin.CaseBindingHash, SnapshotSeed: "synthetic-snapshot", ContextEpoch: epoch})
	return service, store, current, aliases
}

func aliasPreparationInventoryV1(t *testing.T, store *countedAliasPreparationStoreV1) string {
	t.Helper()
	body, err := json.Marshal(store.threadContext)
	if err != nil {
		t.Fatal(err)
	}
	return domainsecurity.SHA256Hex(body)
}

func TestPreparedCaseAliasesHaveNoPersistenceOrProspectiveRecordAuthority(t *testing.T) {
	for _, epoch := range []uint64{1, 2} {
		t.Run(fmt.Sprintf("epoch_%d", epoch), func(t *testing.T) {
			service, store, current, aliases := preparedCaseAliasFixtureV1(t, epoch)
			before := aliasPreparationInventoryV1(t, store)
			input := UseCaseLongitudinalAliasesInputV1{SecurityContext: current, Aliases: aliases[:1], ProviderText: "Host-delegated aliases: " + string(aliases[0])}
			plan, err := service.PrepareCaseLongitudinalAliasesV1(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			if err := plan.InspectSelectionV1(func(selection CaseLongitudinalAliasSelectionV1) error {
				if selection.Record != (PrivateRecordReferenceV1{}) {
					t.Fatal("unwritten thread record became private selection authority")
				}
				if len(selection.References) != 1 || !reflect.DeepEqual(selection.Aliases, aliases[:1]) {
					t.Fatal("prepared aliases changed")
				}
				return errors.New("synthetic later sibling rejects preparation")
			}); err == nil {
				t.Fatal("rejected inspection succeeded")
			}
			if store.puts != 0 || aliasPreparationInventoryV1(t, store) != before {
				t.Fatal("preparation wrote index or child-related continuity before receipt")
			}
			if _, err := json.Marshal(plan); err == nil || !strings.Contains(fmt.Sprintf("%#v", plan), "[REDACTED]") {
				t.Fatal("prepared private state gained an ordinary serialization surface")
			}
			copyPlan := plan
			if err := service.ApplyPreparedCaseLongitudinalAliasesV1(context.Background(), plan, func(selection CaseLongitudinalAliasSelectionV1) error {
				if !domainsecurity.IsSHA256Hex(selection.Record.RecordID) || !domainsecurity.IsSHA256Hex(selection.Record.RecordDigest) {
					t.Fatal("committed alias selection lacks real record authority")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			want := 1
			if epoch == 2 {
				want = 2
			}
			if store.puts != want {
				t.Fatalf("unexpected existing CAS write count: %d", store.puts)
			}
			after := aliasPreparationInventoryV1(t, store)
			if err := service.ApplyPreparedCaseLongitudinalAliasesV1(context.Background(), copyPlan, func(CaseLongitudinalAliasSelectionV1) error { return nil }); err == nil {
				t.Fatal("copied alias plan replayed its write")
			}
			if store.puts != want || aliasPreparationInventoryV1(t, store) != after {
				t.Fatal("alias plan replay rewrote current records")
			}
		})
	}
}

func TestPreparedCaseAliasSiblingsRecomputeOriginalThreadUnionBeforeCAS(t *testing.T) {
	service, store, current, aliases := preparedCaseAliasFixtureV1(t, 1)
	plans := []PreparedCaseLongitudinalAliasesV1{}
	for _, alias := range aliases {
		plan, err := service.PrepareCaseLongitudinalAliasesV1(context.Background(), UseCaseLongitudinalAliasesInputV1{SecurityContext: current, Aliases: []domaincaseentity.ModelEntityAliasV1{alias}, ProviderText: "Host-delegated aliases: " + string(alias)})
		if err != nil {
			t.Fatal(err)
		}
		plans = append(plans, plan)
	}
	if store.puts != 0 {
		t.Fatal("sibling preparation persisted")
	}
	for index := len(plans) - 1; index >= 0; index-- {
		if err := service.ApplyPreparedCaseLongitudinalAliasesV1(context.Background(), plans[index], func(selection CaseLongitudinalAliasSelectionV1) error {
			if len(selection.References) != 1 {
				t.Fatal("sibling union expanded selected aliases")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	record, err := store.ResolveLatestThreadContextForScope(context.Background(), current, current.ThreadID)
	if err != nil || record.Generation != 2 || len(record.EntityReferences) != 2 || store.puts != 2 {
		t.Fatalf("sibling CAS lost original union/evolution: %v", err)
	}
}

func TestPreparedCaseAliasRejectsStaleBindingAndCancellationBeforeCAS(t *testing.T) {
	for _, mode := range []string{"binding_changed", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			service, store, current, aliases := preparedCaseAliasFixtureV1(t, 1)
			plan, err := service.PrepareCaseLongitudinalAliasesV1(context.Background(), UseCaseLongitudinalAliasesInputV1{SecurityContext: current, Aliases: aliases[:1], ProviderText: "Host-delegated aliases: " + string(aliases[0])})
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if mode == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			} else {
				for key, record := range store.bindings {
					record.RecordDigest = strings.Repeat("b", 64)
					store.bindings[key] = record
				}
			}
			before := aliasPreparationInventoryV1(t, store)
			uses := 0
			if err := service.ApplyPreparedCaseLongitudinalAliasesV1(ctx, plan, func(CaseLongitudinalAliasSelectionV1) error { uses++; return nil }); err == nil {
				t.Fatal("stale alias plan was applied")
			}
			if uses != 0 || store.puts != 0 || aliasPreparationInventoryV1(t, store) != before {
				t.Fatal("stale preparation crossed the first CAS")
			}
		})
	}
}
