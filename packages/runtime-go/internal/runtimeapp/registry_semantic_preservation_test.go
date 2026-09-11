//go:build darwin || linux

package runtimeapp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	evidenceregistrystore "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	startupport "analytix.local/runtime-go/internal/ports/startup"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestRuntimeLegacyRegistryStartupDoesNotBackfillHeldProjection(t *testing.T) {
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
	root := filepath.Join(core.roots.DataDir, "private", "evidence-registry")
	store, err := evidenceregistrystore.NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPrepared(ctx, runtimeLegacyRegistryPreparedInputForContextV1(t, frozen)); err != nil {
		t.Fatal(err)
	}
	head := filepath.Join(root, ".registry-projections", domainevidence.EvidenceRegistryProjectionKey(frozen.ThreadID, frozen.TurnID)+".head.json")
	if err := os.Remove(head); err != nil {
		t.Fatal(err)
	}
	plan, err := evidenceregistrystore.PrepareRecoveryV2(ctx, root, core.access)
	if err != nil || !plan.LegacyV1ActivationAllowed() {
		t.Fatalf("fixture did not reach live legacy activation branch: %v", err)
	}
	_, err = prepareRuntimeReportRestartPreservationV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	if present, err := store.HasRecords(ctx); err != nil || !present {
		t.Fatalf("original legacy presence unavailable: %v", err)
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
		t.Fatal("legacy startup presence filled original held projection")
	}
}

func TestRuntimeSemanticRegistryPreservesOriginalHeldRowsAndProjections(t *testing.T) {
	for _, scenario := range []string{"remove-index", "remove-head", "head-mode", "refill-absent-head"} {
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
			root := filepath.Join(core.roots.DataDir, "private", "evidence-registry")
			store, err := evidenceregistrystore.NewStore(root, authority)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.CommitPrepared(ctx, runtimeLegacyRegistryPreparedInputForContextV1(t, frozen)); err != nil {
				t.Fatal(err)
			}
			relative := ".registry-projections/" + domainevidence.EvidenceRegistryProjectionKey(frozen.ThreadID, frozen.TurnID) + ".head.json"
			if scenario == "remove-index" {
				relative = ".registry-authority-index.json"
			}
			path := filepath.Join(root, filepath.FromSlash(relative))
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			before := domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeFile, Mode: 0o600, Size: int64(len(body)), SHA256: domainsecurity.SHA256Hex(body)}
			after := domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeAbsent}
			kind := domainstartup.SemanticOperationRemoveFile
			if scenario == "head-mode" {
				kind = domainstartup.SemanticOperationSetMode
				after = before
				after.Mode = 0o400
			}
			if scenario == "refill-absent-head" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				kind = domainstartup.SemanticOperationInstallFile
				after = before
				before = domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeAbsent}
			}
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			digest := domainsecurity.SHA256Hex([]byte("registry-semantic-test"))
			plan, err := domainstartup.NewSemanticStartupPlanV1(digest, digest, digest, []domainstartup.SemanticStartupOperationV1{{Kind: kind, Path: "data/private/evidence-registry/" + relative, Before: before, After: after}})
			if err != nil {
				t.Fatal(err)
			}
			original := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			err = preserved.ValidateSemanticOperationsV1(ctx, plan.Operations, func(domainstartup.SemanticStartupOperationV1) ([]byte, error) { return body, nil }, "")
			if err == nil {
				t.Fatal("semantic operation changed original held registry row or projection")
			}
			if !reflect.DeepEqual(original, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("semantic validation changed physical bytes or modes")
			}
		})
	}
}

func TestRuntimeSemanticRegistryAllowsIndependentSharedIndexExtension(t *testing.T) {
	ctx := context.Background()
	core, _ := runtimeReportPreservationFixtureV1(t, false, true, false)
	scope, err := prepareRuntimeReportRestartScopeV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	held := scope.Contexts()[0]
	independent, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{ThreadID: "thread-registry-independent", TurnID: "turn-registry-independent", WorkspaceRealPath: held.WorkspaceRealPath, CaseID: held.CaseID, CaseBindingHash: held.CaseBindingHash, ContextEpoch: 1, IssuedAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)})
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
	root := filepath.Join(core.roots.DataDir, "private", "evidence-registry")
	store, err := evidenceregistrystore.NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPrepared(ctx, runtimeLegacyRegistryPreparedInputForContextV1(t, held)); err != nil {
		t.Fatal(err)
	}
	preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	original := preserved.registry.original
	candidateRoot := filepath.Join(t.TempDir(), "registry")
	for name, entry := range original {
		target := filepath.Join(candidateRoot, filepath.FromSlash(name))
		if entry.Directory {
			if err := os.MkdirAll(target, os.FileMode(entry.Mode)); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, entry.Body, os.FileMode(entry.Mode)); err != nil {
			t.Fatal(err)
		}
	}
	candidateStore, err := evidenceregistrystore.NewStore(candidateRoot, authority)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := candidateStore.CommitPrepared(ctx, runtimeLegacyRegistryPreparedInputForContextAndLabelV1(t, independent, "-independent")); err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(filepath.Dir(candidateRoot))
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := evidenceregistrystore.PrepareRecoveryV2(ctx, candidateRoot, access)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := prepared.SnapshotOriginalLegacyFilesV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	operations := []domainstartup.SemanticStartupOperationV1{}
	for name, entry := range candidate {
		previous, exists := original[name]
		before, after := runtimeOriginalSemanticStateV1(previous, exists), runtimeOriginalSemanticStateV1(entry, true)
		if before == after {
			continue
		}
		kind := domainstartup.SemanticOperationInstallFile
		if entry.Directory {
			kind = domainstartup.SemanticOperationCreateDirectory
		}
		operations = append(operations, domainstartup.SemanticStartupOperationV1{Kind: kind, Path: runtimeRegistrySemanticRootV1 + "/" + name, Before: before, After: after})
	}
	digest := domainsecurity.SHA256Hex([]byte("independent-registry-semantic-test"))
	plan, err := domainstartup.NewSemanticStartupPlanV1(digest, digest, digest, operations)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Operations) < 2 {
		t.Fatal("fixture omitted shared index and capsule changes")
	}
	if err := preserved.ValidateSemanticOperationsV1(ctx, plan.Operations, func(op domainstartup.SemanticStartupOperationV1) ([]byte, error) {
		return candidate[op.Path[len(runtimeRegistrySemanticRootV1)+1:]].Body, nil
	}, ""); err != nil {
		t.Fatalf("independent complete shared index extension rejected: %v", err)
	}
	apply := func(candidate map[string]evidenceregistrystore.OriginalLegacyEntryV1) {
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
		prepared, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
			for name, entry := range candidate {
				target := filepath.Join(stage.DataDir, "private", "evidence-registry", filepath.FromSlash(name))
				if entry.Directory {
					if err := os.MkdirAll(target, os.FileMode(entry.Mode)); err != nil {
						return err
					}
					continue
				}
				if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
					return err
				}
				if err := os.WriteFile(target, entry.Body, os.FileMode(entry.Mode)); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		defer prepared.Close()
		if err := prepared.Apply(ctx); err != nil {
			t.Fatalf("independent registry semantic Apply: %v", err)
		}
		if err := preserved.validateRegistrySemanticOperationsV1(ctx, nil, nil, ""); err != nil {
			t.Fatalf("independent registry fixed point: %v", err)
		}
	}
	apply(candidate)
	independentInput := runtimeLegacyRegistryPreparedInputForContextAndLabelV1(t, independent, "-independent")
	if err := candidateStore.Revoke(ctx, registryport.RevokeInput{Context: independent, ReceiptID: independentInput.Draft.ReceiptID, ReasonCode: "source_retracted", RevokedAt: time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	secondPlan, err := evidenceregistrystore.PrepareRecoveryV2(ctx, candidateRoot, access)
	if err != nil {
		t.Fatal(err)
	}
	second, err := secondPlan.SnapshotOriginalLegacyFilesV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	apply(second)
	_, revalidate, err := runtimeRegistryContextsV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(primaryPath, append(body, ' '), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := revalidate(ctx); err == nil {
		t.Fatal("complete primary denominator accepted late independent drift")
	}
}

func TestRuntimeRegistryInitialObservationRejectsSignedHeldJournal(t *testing.T) {
	for _, scenario := range []string{"unapplied-remove-head", "applied-refill-head"} {
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
			root := filepath.Join(core.roots.DataDir, "private", "evidence-registry")
			store, err := evidenceregistrystore.NewStore(root, authority)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.CommitPrepared(ctx, runtimeLegacyRegistryPreparedInputForContextV1(t, frozen)); err != nil {
				t.Fatal(err)
			}
			relative := filepath.Join("private", "evidence-registry", ".registry-projections", domainevidence.EvidenceRegistryProjectionKey(frozen.ThreadID, frozen.TurnID)+".head.json")
			head := filepath.Join(core.roots.DataDir, relative)
			originalHead, err := os.ReadFile(head)
			if err != nil {
				t.Fatal(err)
			}
			installed, tail := "private/a-registry-prefix.bin", "private/z-registry-tail.bin"
			if scenario == "applied-refill-head" {
				if err := os.Remove(head); err != nil {
					t.Fatal(err)
				}
				installed = relative
			}
			runtimePendingSemanticCutForTestV1(t, core, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				if scenario == "unapplied-remove-head" {
					if err := os.WriteFile(filepath.Join(stage.DataDir, installed), []byte("independent-prefix"), 0o600); err != nil {
						return err
					}
					if err := os.Remove(filepath.Join(stage.DataDir, relative)); err != nil {
						return err
					}
				} else {
					if err := os.WriteFile(filepath.Join(stage.DataDir, relative), originalHead, 0o600); err != nil {
						return err
					}
				}
				return os.WriteFile(filepath.Join(stage.DataDir, tail), []byte("independent-tail"), 0o600)
			}, installed, tail)
			fresh, err := runtimeObservePendingCoreForTestV1(t, core)
			if err != nil {
				t.Fatal(err)
			}
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			if _, err := prepareRuntimeReportRestartPreservationV1(ctx, fresh); err == nil || !strings.Contains(err.Error(), "changed original held inventory") {
				t.Fatalf("initial registry observation admitted signed held mutation: %v", err)
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("initial journal rejection mutated physical state")
			}
		})
	}
}
