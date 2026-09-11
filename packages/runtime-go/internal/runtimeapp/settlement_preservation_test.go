package runtimeapp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	evidenceregistrystore "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	evidencesettlement "analytix.local/runtime-go/internal/adapters/outbound/evidencesettlement"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	startupport "analytix.local/runtime-go/internal/ports/startup"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type runtimeRejectLiveSettlementRegistryV1 struct {
	registryport.Registry
	registryport.Inventory
	listCalls, commitCalls int
}

type runtimeQuarantinedSettlementReaderV1 struct {
	runtimeNoExecutableFinalThreadsV1
	quarantined string
}

func (reader runtimeQuarantinedSettlementReaderV1) QuarantinedThread(id string) bool {
	return id == reader.quarantined
}

func TestRuntimeSettlementActivationRetainsAbsentOriginalOwner(t *testing.T) {
	ctx := context.Background()
	core, _ := runtimeReportPreservationFixtureV1(t, false, true, false)
	preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(core.roots.DataDir, "private", "evidence-settlements")
	before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	store, err := preserved.openSettlementStoreV1(ctx, root, core.access)
	if err != nil {
		t.Fatal(err)
	}
	if records, err := store.ListPrepared(ctx); err != nil || len(records) != 0 {
		t.Fatalf("absent root activation read: %v", err)
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
		t.Fatal("root activation filled original absent settlement owner")
	}
}

func TestRuntimeSettlementPreservationAllowsIndependentSemanticApply(t *testing.T) {
	ctx := context.Background()
	core, _ := runtimeReportPreservationFixtureV1(t, false, true, false)
	scope, err := prepareRuntimeReportRestartScopeV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	held := scope.Contexts()[0]
	independent, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{ThreadID: "thread-settlement-independent", TurnID: "turn-settlement-independent", WorkspaceRealPath: held.WorkspaceRealPath, CaseID: held.CaseID, CaseBindingHash: held.CaseBindingHash, ContextEpoch: 1, IssuedAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	primary := map[string]any{"id": independent.ThreadID, "securityState": independent, "turns": []any{map[string]any{"id": independent.TurnID, "threadId": independent.ThreadID, "status": "running", "securityContext": independent, "items": []any{}}}}
	body, err := json.Marshal(primary)
	if err != nil {
		t.Fatal(err)
	}
	primaryPath := filepath.Join(core.roots.DurableDir, "threads", independent.ThreadID, "thread.json")
	if err := os.MkdirAll(filepath.Dir(primaryPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(primaryPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	rootAuthority, err := persistencefs.FreezeRootAuthority(core.roots)
	if err != nil {
		t.Fatal(err)
	}
	core, err = prepareRuntimeChildIdentityStartupV1(ctx, core.roots, rootAuthority, core.access, nil)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	registryRoot := filepath.Join(core.roots.DataDir, "private", "evidence-registry")
	registry, err := evidenceregistrystore.NewStore(registryRoot, authority)
	if err != nil {
		t.Fatal(err)
	}
	for _, frozen := range []domainsecurity.TurnSecurityContext{held, independent} {
		if _, err := registry.CommitPrepared(ctx, runtimeLegacyRegistryPreparedInputForContextAndLabelV1(t, frozen, frozen.ThreadID)); err != nil {
			t.Fatal(err)
		}
	}
	heldPrepared := runtimePreparedSettlementForSemanticTestV1(t, held, authority)
	heldBody, err := domainevidence.PreparedEvidenceSettlementBytes(heldPrepared)
	if err != nil {
		t.Fatal(err)
	}
	heldRelative := filepath.Join("private", "evidence-settlements", "prepared", heldPrepared.SettlementID[:2], heldPrepared.SettlementID+".json")
	if err := os.MkdirAll(filepath.Dir(filepath.Join(core.roots.DataDir, heldRelative)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(core.roots.DataDir, heldRelative), append(heldBody, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	original, err := preserved.ObserveOriginalEvidenceSettlementInventoryV1(ctx)
	if err != nil || len(original.Prepared) != 1 || len(original.Registries) != 1 {
		t.Fatalf("held inventory: %v", err)
	}
	contexts := []domainsecurity.TurnSecurityContext{held, independent}
	if records, err := preserved.ObserveRestartEvidenceRegistryInventoryV1(ctx, contexts); err != nil || len(records) != 2 {
		t.Fatalf("complete registry source: count=%d err=%v", len(records), err)
	}
	for _, incomplete := range [][]domainsecurity.TurnSecurityContext{nil, {held}, {held, held}} {
		if records, err := preserved.ObserveRestartEvidenceRegistryInventoryV1(ctx, incomplete); err == nil || records != nil {
			t.Fatal("incomplete or duplicate context denominator accepted")
		}
	}
	settlementStore, err := evidencesettlement.NewStore(filepath.Join(core.roots.DataDir, "private", "evidence-settlements"))
	if err != nil {
		t.Fatal(err)
	}
	live := &runtimeRejectLiveSettlementRegistryV1{}
	issuer := evidenceapp.Issuer{Authority: authority, Registry: live, SettlementStore: settlementStore}
	quarantinedReader := runtimeQuarantinedSettlementReaderV1{quarantined: independent.ThreadID}
	classified, err := evidenceapp.PreflightEvidenceSettlementInventoryWithPreservationV1(ctx, quarantinedReader, issuer, preserved)
	if err != nil || len(classified.Preserved) != 1 || len(classified.Pending) != 0 {
		t.Fatalf("unrelated quarantine blocked complete startup inventory: %v", err)
	}
	if err := evidenceapp.ApplyEvidenceSettlementReconciliationInventoryWithPreservationV1(ctx, quarantinedReader, issuer, classified, preserved); err != nil {
		t.Fatal(err)
	}
	if live.listCalls != 0 || live.commitCalls != 0 {
		t.Fatal("quarantined original inventory gained live effects")
	}
	independentPrepared := runtimePreparedSettlementForSemanticTestV1(t, independent, authority)
	independentBody, err := domainevidence.PreparedEvidenceSettlementBytes(independentPrepared)
	if err != nil {
		t.Fatal(err)
	}
	independentRelative := filepath.Join("private", "evidence-settlements", "prepared", independentPrepared.SettlementID[:2], independentPrepared.SettlementID+".json")
	digest := domainsecurity.SHA256Hex([]byte("independent-settlement-semantic-test"))
	apply := func(remove bool) {
		t.Helper()
		snapshot, err := persistencefs.NewStartupSnapshotReader(core.roots).CaptureManagedSnapshotV1(ctx)
		if err != nil {
			t.Fatal(err)
		}
		baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
		if err != nil {
			t.Fatal(err)
		}
		journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(core.roots)
		if err != nil {
			t.Fatal(err)
		}
		builder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(core.roots, rootAuthority, journal, preserved)
		plan, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
			target := filepath.Join(stage.DataDir, independentRelative)
			if remove {
				return os.Remove(target)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return err
			}
			return os.WriteFile(target, independentBody, 0o600)
		})
		if err != nil {
			t.Fatal(err)
		}
		defer plan.Close()
		if err := plan.Apply(ctx); err != nil {
			t.Fatalf("independent settlement apply: %v", err)
		}
		if err := preserved.validateSettlementSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
			t.Fatal(err)
		}
		observed, err := preserved.ObserveOriginalEvidenceSettlementInventoryV1(ctx)
		if err != nil || !reflect.DeepEqual(original, observed) {
			t.Fatalf("held original changed after independent transaction: %v", err)
		}
	}
	apply(false)
	classified, err = evidenceapp.PreflightEvidenceSettlementInventoryWithPreservationV1(ctx, quarantinedReader, issuer, preserved)
	if err != nil || len(classified.Preserved) != 1 || len(classified.Quarantined) != 1 || len(classified.Pending) != 0 {
		t.Fatalf("independent quarantined prepared record changed classification: held=%d audit=%d pending=%d err=%v", len(classified.Preserved), len(classified.Quarantined), len(classified.Pending), err)
	}
	if err := evidenceapp.ApplyEvidenceSettlementReconciliationInventoryWithPreservationV1(ctx, quarantinedReader, issuer, classified, preserved); err != nil {
		t.Fatal(err)
	}
	apply(true)
}

func (registry *runtimeRejectLiveSettlementRegistryV1) ListRegistries(context.Context, []domainsecurity.TurnSecurityContext) ([]registryport.InventoryRecord, error) {
	registry.listCalls++
	return nil, errors.New("held historical inventory reached live registry")
}

func (registry *runtimeRejectLiveSettlementRegistryV1) CommitPrepared(context.Context, registryport.CommitPreparedInput) (domainevidence.EvidenceReceipt, error) {
	registry.commitCalls++
	return domainevidence.EvidenceReceipt{}, errors.New("held settlement reached live commit")
}

func TestRuntimeSettlementPreservationUsesOriginalCompleteDenominator(t *testing.T) {
	for _, scenario := range []string{"original", "prepared_removed", "registry_removed", "primary_drift", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			core, _ := runtimeReportPreservationFixtureV1(t, false, true, false)
			scope, err := prepareRuntimeReportRestartScopeV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			frozen := scope.Contexts()[0]
			authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
			if err != nil {
				t.Fatal(err)
			}
			prepared := runtimePreparedSettlementForSemanticTestV1(t, frozen, authority)
			body, err := domainevidence.PreparedEvidenceSettlementBytes(prepared)
			if err != nil {
				t.Fatal(err)
			}
			settlementRoot := filepath.Join(core.roots.DataDir, "private", "evidence-settlements")
			preparedPath := filepath.Join(settlementRoot, "prepared", prepared.SettlementID[:2], prepared.SettlementID+".json")
			if err := os.MkdirAll(filepath.Dir(preparedPath), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(preparedPath, append(body, '\n'), 0o600); err != nil {
				t.Fatal(err)
			}
			settlementStore, err := evidencesettlement.NewStore(settlementRoot)
			if err != nil {
				t.Fatal(err)
			}
			registryRoot := filepath.Join(core.roots.DataDir, "private", "evidence-registry")
			registry, err := evidenceregistrystore.NewStore(registryRoot, authority)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := registry.CommitPrepared(ctx, runtimeLegacyRegistryPreparedInputForContextV1(t, frozen)); err != nil {
				t.Fatal(err)
			}
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			observer, ok := any(preserved).(evidenceapp.EvidenceSettlementRestartPreservationV1)
			if !ok {
				t.Fatal("root settlement original observer is unavailable")
			}
			if scenario == "prepared_removed" {
				if err := os.Remove(preparedPath); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "registry_removed" {
				if err := os.Remove(filepath.Join(registryRoot, ".registry-authority-index.json")); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "primary_drift" {
				if err := os.WriteFile(filepath.Join(core.roots.DurableDir, "threads", frozen.ThreadID, "thread.json"), []byte("{\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			live := &runtimeRejectLiveSettlementRegistryV1{}
			issuer := evidenceapp.Issuer{Authority: authority, Registry: live, SettlementStore: settlementStore}
			executable := runtimeNoExecutableFinalThreadsV1{}
			inventory, err := evidenceapp.PreflightEvidenceSettlementInventoryWithPreservationV1(ctx, executable, issuer, observer)
			if scenario == "original" {
				if err != nil || len(inventory.Preserved) != 1 || len(inventory.Pending) != 0 {
					t.Fatalf("original inventory was lost or became executable: preserved=%d pending=%d err=%v", len(inventory.Preserved), len(inventory.Pending), err)
				}
				if err := evidenceapp.ApplyEvidenceSettlementReconciliationInventoryWithPreservationV1(ctx, executable, issuer, inventory, observer); err != nil {
					t.Fatal(err)
				}
			} else if err == nil || scenario == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("original drift or cancellation was accepted: %v", err)
			}
			if live.listCalls != 0 || live.commitCalls != 0 || !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("original settlement observation reached live effects or changed bytes/modes")
			}
		})
	}
}
