//go:build darwin || linux

package runtimeapp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	startupport "analytix.local/runtime-go/internal/ports/startup"
)

func TestRuntimeUsageSemanticInstallRejectsHeldGlobalModeChangeBeforeEffects(t *testing.T) {
	ctx := context.Background()
	core, _ := runtimeChildRestartScopeFixtureV1(t, false, false)
	path := filepath.Join(core.roots.DurableDir, "usage_events", "index.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	original := []byte("{\"threadId\":\"thread-report-held\",\"seq\":1}\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	core, err := runtimeObservePendingCoreForTestV1(t, core)
	if err != nil {
		t.Fatal(err)
	}
	preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	root, err := persistencefs.FreezeRootAuthority(core.roots)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(core.roots)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := persistencefs.NewStartupSnapshotReader(core.roots).CaptureManagedSnapshotV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("synthetic-usage-mode-change"))
	baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(core.roots, root, journal, preserved).Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		after := append(append([]byte(nil), original...), []byte("{\"threadId\":\"usage-independent\",\"seq\":1}\n")...)
		target := filepath.Join(stage.DurableDir, "usage_events", "index.jsonl")
		if err := os.WriteFile(target, after, 0o600); err != nil {
			return err
		}
		return os.Chmod(target, 0o640)
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = prepared.Close() })
	if err := prepared.Apply(ctx); err == nil {
		t.Error("semantic install admitted held global mode change")
	}
	after, err := os.ReadFile(path)
	info, statErr := os.Lstat(path)
	if err != nil || statErr != nil || !reflect.DeepEqual(original, after) || info.Mode().Perm() != 0o600 {
		t.Fatal("semantic install changed held global bytes or mode before rejecting")
	}
}

func TestRuntimeUsageSemanticJournalKeepsOriginalRowsAndHeldFiles(t *testing.T) {
	for _, scenario := range []string{"replace_held", "remove_held_file", "absent_add_held", "absent_add_reserved", "absent_add_independent", "unapplied_independent", "applied_independent_replace"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			core, targets := runtimeChildRestartScopeFixtureV1(t, false, false)
			row := func(id, source string) []byte {
				body, err := json.Marshal(map[string]any{"threadId": id, "turnId": "turn-usage", "seq": 1, "usageSource": source, "usage": map[string]any{"totalTokens": 7}})
				if err != nil {
					t.Fatal(err)
				}
				return append(body, '\n')
			}
			for _, path := range []string{filepath.Join(core.roots.DurableDir, "usage_events", "threads"), filepath.Join(core.roots.DurableDir, "threads", "usage-independent")} {
				if err := os.MkdirAll(path, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(core.roots.DurableDir, "threads", "usage-independent", "thread.json"), []byte(`{"id":"usage-independent","turns":[]}`), 0o600); err != nil {
				t.Fatal(err)
			}
			index := filepath.Join(core.roots.DurableDir, "usage_events", "index.jsonl")
			heldFile := filepath.Join(core.roots.DurableDir, "usage_events", "threads", "thread-report-held.jsonl")
			tail := filepath.Join(core.roots.DurableDir, "usage_events", "threads", "usage-independent.jsonl")
			original := row("thread-report-held", "original")
			if scenario == "replace_held" || scenario == "unapplied_independent" || scenario == "applied_independent_replace" {
				if err := os.WriteFile(index, original, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "remove_held_file" {
				if err := os.WriteFile(heldFile, original, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(tail, row("usage-independent", "original"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var err error
			core, err = runtimeObservePendingCoreForTestV1(t, core)
			if err != nil {
				t.Fatal(err)
			}
			after := row("thread-report-held", "future")
			if scenario == "absent_add_reserved" {
				after = row(targets[1].ChildThreadID, "future")
			}
			if scenario == "absent_add_independent" {
				after = row("usage-independent", "independent")
			}
			if scenario == "unapplied_independent" || scenario == "applied_independent_replace" {
				after = append(append([]byte(nil), original...), row("usage-independent", "independent")...)
			}
			prefix, marker := "private/a-usage-prefix.bin", "private/b-usage-marker.bin"
			mutation := func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				if scenario == "remove_held_file" || scenario == "unapplied_independent" {
					if err := os.WriteFile(filepath.Join(stage.DataDir, prefix), []byte("synthetic prefix"), 0o600); err != nil {
						return err
					}
				}
				if scenario == "remove_held_file" {
					if err := os.Remove(filepath.Join(stage.DurableDir, "usage_events", "threads", "thread-report-held.jsonl")); err != nil {
						return err
					}
					return os.Remove(filepath.Join(stage.DurableDir, "usage_events", "threads", "usage-independent.jsonl"))
				}
				if err := os.WriteFile(filepath.Join(stage.DurableDir, "usage_events", "index.jsonl"), after, 0o600); err != nil {
					return err
				}
				if scenario == "unapplied_independent" {
					return os.WriteFile(filepath.Join(stage.DataDir, marker), []byte("synthetic marker"), 0o600)
				}
				return os.WriteFile(filepath.Join(stage.DurableDir, "usage_events", "threads", "usage-independent.jsonl"), row("usage-independent", "independent"), 0o600)
			}
			if scenario == "replace_held" || scenario == "applied_independent_replace" {
				runtimeSummaryReplacementCutForTestV1(t, core, index, tail, after, mutation)
			} else {
				installed, err := filepath.Rel(core.roots.DataDir, index)
				if err != nil {
					t.Fatal(err)
				}
				absent, err := filepath.Rel(core.roots.DataDir, tail)
				if err != nil {
					t.Fatal(err)
				}
				if scenario == "remove_held_file" {
					installed = prefix
					absent, err = filepath.Rel(core.roots.DataDir, heldFile)
					if err != nil {
						t.Fatal(err)
					}
				}
				if scenario == "unapplied_independent" {
					installed, absent = prefix, marker
				}
				runtimePendingSemanticCutForTestV1(t, core, mutation, installed, absent)
			}
			fresh, err := runtimeObservePendingCoreForTestV1(t, core)
			if err != nil {
				t.Fatal(err)
			}
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, fresh)
			positive := scenario == "absent_add_independent" || scenario == "unapplied_independent"
			if positive && err != nil {
				t.Fatalf("provable independent usage cut was refused: %v", err)
			}
			if !positive && err == nil {
				t.Fatal("future usage supplied missing original rows or held file authority")
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("usage original observation wrote state")
			}
			if positive {
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
				body, err := os.ReadFile(index)
				if err != nil || !reflect.DeepEqual(body, after) {
					t.Fatal("independent usage recovery lost exact After bytes")
				}
				if err := preserved.usage.Revalidate(ctx, core.roots.DurableDir); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
