//go:build darwin || linux

package runtimeapp

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	casestore "analytix.local/runtime-go/internal/adapters/outbound/casethreadauthority"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	startupport "analytix.local/runtime-go/internal/ports/startup"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func runtimeCaseThreadSemanticFixtureV1(t *testing.T) (*runtimeChildIdentityStartupV1, domainsecurity.CaseThreadAuthorityRecord, domainsecurity.CaseThreadAuthorityRecord, *finalauthority.FileAuthority) {
	t.Helper()
	ctx := context.Background()
	core, _ := runtimeReportPreservationFixtureV1(t, false, true, false)
	scope, err := prepareRuntimeReportRestartScopeV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	frozen := scope.Contexts()[0]
	key, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	held, err := domainsecurity.NewCaseThreadAuthorityRecord(frozen, key.KeyID(), key.PublicKey(), func(body []byte) ([]byte, error) { return key.Sign(ctx, body) })
	if err != nil {
		t.Fatal(err)
	}
	independent, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{ThreadID: "thread-independent-case", TurnID: "turn-independent-case", WorkspaceRealPath: frozen.WorkspaceRealPath, CaseID: frozen.CaseID, CaseBindingHash: frozen.CaseBindingHash, ContextEpoch: 1, IssuedAt: time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	other, err := domainsecurity.NewCaseThreadAuthorityRecord(independent, key.KeyID(), key.PublicKey(), func(body []byte) ([]byte, error) { return key.Sign(ctx, body) })
	if err != nil {
		t.Fatal(err)
	}
	store, err := casestore.NewStore(filepath.Join(core.roots.DataDir, "private", "case-thread-authority"), core.access)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutIfAbsent(ctx, held); err != nil {
		t.Fatal(err)
	}
	core, err = runtimeObservePendingCoreForTestV1(t, core)
	if err != nil {
		t.Fatal(err)
	}
	return core, held, other, key
}

func TestRuntimeSemanticCaseThreadAuthenticatedCutsPreserveOriginalGraph(t *testing.T) {
	for _, scenario := range []string{"independent_context", "new_held_lineage", "applied_held_remove", "preexisting_orphan_future_repair"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			core, held, independent, key := runtimeCaseThreadSemanticFixtureV1(t)
			pathFor := func(id string) string { return filepath.Join("private", "case-thread-authority", id[:2], id+".json") }
			writeRecord := func(data string, record domainsecurity.CaseThreadAuthorityRecord) {
				t.Helper()
				body, err := domainsecurity.CaseThreadAuthorityRecordBytes(record)
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(data, pathFor(record.RecordDigest))
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, body, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			const prefix, tail = "private/aa-case-prefix.txt", "private/z-case-tail.txt"
			installed, absent := pathFor(independent.RecordDigest), tail
			toInstall := independent
			if scenario == "new_held_lineage" || scenario == "preexisting_orphan_future_repair" {
				parent := held
				if scenario == "preexisting_orphan_future_repair" {
					parent = independent
				}
				lineage, err := domainsecurity.NewCaseThreadLineageAuthorityRecord("thread-new-derived", domainsecurity.CaseThreadAuthorityThreadID(parent), parent.RecordDigest, "fork", key.KeyID(), key.PublicKey(), func(body []byte) ([]byte, error) { return key.Sign(ctx, body) })
				if err != nil {
					t.Fatal(err)
				}
				if scenario == "new_held_lineage" {
					toInstall, installed = lineage, pathFor(lineage.RecordDigest)
				} else {
					// A signed future parent must not repair an original orphan.
					writeRecord(core.roots.DataDir, lineage)
					installed, absent = prefix, pathFor(independent.RecordDigest)
				}
			}
			if scenario == "applied_held_remove" {
				if err := os.WriteFile(filepath.Join(core.roots.DataDir, tail), []byte("remaining removal"), 0o600); err != nil {
					t.Fatal(err)
				}
				installed, absent = prefix, pathFor(held.RecordDigest)
			}
			mutation := func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				if scenario == "applied_held_remove" {
					if err := os.Remove(filepath.Join(stage.DataDir, pathFor(held.RecordDigest))); err != nil {
						return err
					}
					if err := os.Remove(filepath.Join(stage.DataDir, tail)); err != nil {
						return err
					}
					return os.WriteFile(filepath.Join(stage.DataDir, prefix), []byte("applied prefix"), 0o600)
				}
				writeRecord(stage.DataDir, toInstall)
				if err := os.WriteFile(filepath.Join(stage.DataDir, tail), []byte("independent tail"), 0o600); err != nil {
					return err
				}
				if scenario == "preexisting_orphan_future_repair" {
					return os.WriteFile(filepath.Join(stage.DataDir, prefix), []byte("applied prefix"), 0o600)
				}
				return nil
			}
			if scenario == "independent_context" {
				preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
				if err != nil {
					t.Fatal(err)
				}
				// The positive cut also traverses the actual guarded Apply path.
				runtimePendingSemanticCutForTestV1(t, core, mutation, installed, absent, preserved)
			} else {
				// Model an authenticated transaction left by the previous writer.
				runtimePendingSemanticCutForTestV1(t, core, mutation, installed, absent)
			}
			fresh, err := runtimeObservePendingCoreForTestV1(t, core)
			if err != nil {
				t.Fatal(err)
			}
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, fresh)
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("case original observation wrote state")
			}
			if scenario != "independent_context" {
				if err == nil {
					t.Fatal("authenticated future case graph replaced original authority")
				}
				return
			}
			if err != nil {
				t.Fatalf("independent authenticated case cut rejected: %v", err)
			}
			root, err := persistencefs.FreezeRootAuthority(core.roots)
			if err != nil {
				t.Fatal(err)
			}
			journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(core.roots)
			if err != nil {
				t.Fatal(err)
			}
			if err := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(core.roots, root, journal, preserved).RecoverAuthenticatedExisting(ctx); err != nil {
				t.Fatal(err)
			}
			after := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			for path, original := range before {
				if !semanticOriginalRecordUnchangedForTestV1(original, after[path]) {
					t.Fatalf("case recovery changed original %s", path)
				}
			}
			body, err := os.ReadFile(filepath.Join(core.roots.DataDir, tail))
			if err != nil || string(body) != "independent tail" {
				t.Fatal("independent case recovery did not finish")
			}
		})
	}
}

func TestRuntimeSemanticCaseThreadPreservationUsesCompleteOriginalGraph(t *testing.T) {
	for _, scenario := range []string{"remove_held", "held_mode", "new_held_lineage", "independent_context", "orphan_lineage", "foreign_key", "noncanonical"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			core, held, independent, key := runtimeCaseThreadSemanticFixtureV1(t)
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			pathFor := func(id string) string { return "data/private/case-thread-authority/" + id[:2] + "/" + id + ".json" }
			state := func(body []byte) domainstartup.SemanticEntryStateV1 {
				return domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeFile, Mode: 0o600, Size: int64(len(body)), SHA256: domainsecurity.SHA256Hex(body)}
			}
			body, err := domainsecurity.CaseThreadAuthorityRecordBytes(independent)
			if err != nil {
				t.Fatal(err)
			}
			operation := domainstartup.SemanticStartupOperationV1{Kind: domainstartup.SemanticOperationInstallFile, Path: pathFor(independent.RecordDigest), Before: domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeAbsent}, After: state(body)}
			switch scenario {
			case "remove_held", "held_mode":
				body, err = domainsecurity.CaseThreadAuthorityRecordBytes(held)
				if err != nil {
					t.Fatal(err)
				}
				operation.Path, operation.Before = pathFor(held.RecordDigest), state(body)
				operation.Kind, operation.After = domainstartup.SemanticOperationRemoveFile, domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeAbsent}
				if scenario == "held_mode" {
					operation.Kind, operation.After = domainstartup.SemanticOperationSetMode, operation.Before
					operation.After.Mode = 0o400
				}
			case "new_held_lineage", "orphan_lineage":
				parent := held
				if scenario == "orphan_lineage" {
					parent = independent
				}
				lineage, err := domainsecurity.NewCaseThreadLineageAuthorityRecord("thread-new-derived", domainsecurity.CaseThreadAuthorityThreadID(parent), parent.RecordDigest, "fork", key.KeyID(), key.PublicKey(), func(body []byte) ([]byte, error) { return key.Sign(ctx, body) })
				if err != nil {
					t.Fatal(err)
				}
				body, err = domainsecurity.CaseThreadAuthorityRecordBytes(lineage)
				if err != nil {
					t.Fatal(err)
				}
				operation.Path, operation.After = pathFor(lineage.RecordDigest), state(body)
			case "foreign_key":
				foreign, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(t.TempDir(), "key.json"), false)
				if err != nil {
					t.Fatal(err)
				}
				other, err := domainsecurity.NewCaseThreadAuthorityRecord(*independent.SecurityContext, foreign.KeyID(), foreign.PublicKey(), func(body []byte) ([]byte, error) { return foreign.Sign(ctx, body) })
				if err != nil {
					t.Fatal(err)
				}
				body, err = domainsecurity.CaseThreadAuthorityRecordBytes(other)
				if err != nil {
					t.Fatal(err)
				}
				operation.Path, operation.After = pathFor(other.RecordDigest), state(body)
			case "noncanonical":
				body = append(body, '\n')
				operation.After = state(body)
			}
			digest := domainsecurity.SHA256Hex([]byte("synthetic-case-semantic-plan"))
			plan, err := domainstartup.NewSemanticStartupPlanV1(digest, digest, digest, []domainstartup.SemanticStartupOperationV1{operation})
			if err != nil {
				t.Fatal(err)
			}
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			err = preserved.ValidateSemanticOperationsV1(ctx, plan.Operations, func(domainstartup.SemanticStartupOperationV1) ([]byte, error) { return body, nil }, "")
			if scenario == "independent_context" {
				if err != nil {
					t.Fatalf("independent complete context rejected: %v", err)
				}
			} else if err == nil {
				t.Error("semantic program bypassed original case authority")
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("case semantic observation wrote state")
			}
		})
	}
}
