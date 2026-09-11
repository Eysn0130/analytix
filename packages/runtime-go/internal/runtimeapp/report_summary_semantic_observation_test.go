//go:build darwin || linux

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

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	startupport "analytix.local/runtime-go/internal/ports/startup"
)

type summaryAfterCutContextV1 struct {
	context.Context
	cancel     context.CancelFunc
	path, tail string
	after      []byte
}

func (ctx summaryAfterCutContextV1) Err() error {
	body, err := os.ReadFile(ctx.path)
	if err == nil && reflect.DeepEqual(body, ctx.after) {
		if _, err := os.Stat(ctx.tail); os.IsNotExist(err) {
			ctx.cancel()
		}
	}
	return ctx.Context.Err()
}

func runtimeSummaryReplacementCutForTestV1(t *testing.T, core *runtimeChildIdentityStartupV1, index, tail string, after []byte, mutation func(context.Context, startupport.PersistenceRootsV1) error) {
	t.Helper()
	ctx := context.Background()
	snapshot, err := persistencefs.NewStartupSnapshotReader(core.roots).CaptureManagedSnapshotV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("synthetic-summary-replacement-cut"))
	baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := persistencefs.NewSemanticPlanBuilder(core.roots).Prepare(ctx, baseline, digest, mutation)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = prepared.Close() })
	cancelled, cancel := context.WithCancel(ctx)
	defer cancel()
	cut := summaryAfterCutContextV1{Context: cancelled, cancel: cancel, path: index, tail: tail, after: after}
	if err := prepared.Apply(cut); !errors.Is(err, context.Canceled) {
		t.Fatalf("summary replacement did not reach its exact cut: %v", err)
	}
	body, err := os.ReadFile(index)
	if err != nil || !reflect.DeepEqual(body, after) {
		t.Fatal("summary cut did not install exact After bytes")
	}
	if _, err := os.Stat(tail); !os.IsNotExist(err) {
		t.Fatal("summary replacement crossed the independent tail")
	}
}

func TestRuntimeReportSummarySemanticJournalKeepsProvableOriginalRows(t *testing.T) {
	for _, scenario := range []string{"replace_held", "remove_original", "absent_add_held", "absent_add_reserved_child", "absent_add_independent", "unapplied_independent_change"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			core, targets := runtimeChildRestartScopeFixtureV1(t, false, false)
			row := func(id, title string) []byte {
				body, err := json.Marshal(map[string]any{"schemaVersion": 1, "threadId": id, "summary": map[string]any{"id": id, "title": title}})
				if err != nil {
					t.Fatal(err)
				}
				return append(body, '\n')
			}
			original := row("thread-report-held", "original synthetic")
			index := filepath.Join(core.roots.DurableDir, "thread_summaries.jsonl")
			if scenario == "replace_held" || scenario == "remove_original" || scenario == "unapplied_independent_change" {
				if err := os.WriteFile(index, original, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			tailRoot := filepath.Join(core.roots.DurableDir, "threads", "zz-independent")
			if err := os.MkdirAll(tailRoot, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(tailRoot, "thread.json"), []byte(`{"id":"zz-independent","turns":[]}`), 0o600); err != nil {
				t.Fatal(err)
			}
			tail := filepath.Join(tailRoot, "z-tail.bin")
			if scenario == "remove_original" {
				if err := os.WriteFile(tail, []byte("original remaining removal"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			installed, err := filepath.Rel(core.roots.DataDir, index)
			if err != nil {
				t.Fatal(err)
			}
			absent, err := filepath.Rel(core.roots.DataDir, tail)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "remove_original" || scenario == "unapplied_independent_change" {
				installed, absent = "private/a-summary-prefix.bin", installed
			}
			after := row("thread-report-held", "changed synthetic")
			if scenario == "absent_add_reserved_child" {
				after = row(targets[1].ChildThreadID, "future synthetic")
			}
			if scenario == "absent_add_independent" {
				after = row("zz-independent", "independent synthetic")
			}
			if scenario == "unapplied_independent_change" {
				after = append(append([]byte(nil), original...), row("zz-independent", "independent synthetic")...)
			}
			// An unapplied update uses an existing index, so its cut is triggered
			// by a distinct still-absent file before the index operation instead.
			if scenario == "unapplied_independent_change" {
				absent = "private/b-summary-after-prefix.bin"
			}
			mutation := func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				if scenario == "remove_original" || scenario == "unapplied_independent_change" {
					if err := os.WriteFile(filepath.Join(stage.DataDir, "private", "a-summary-prefix.bin"), []byte("applied prefix"), 0o600); err != nil {
						return err
					}
				}
				if scenario == "remove_original" {
					if err := os.Remove(filepath.Join(stage.DurableDir, "thread_summaries.jsonl")); err != nil {
						return err
					}
					return os.Remove(filepath.Join(stage.DurableDir, "threads", "zz-independent", "z-tail.bin"))
				}
				if err := os.WriteFile(filepath.Join(stage.DurableDir, "thread_summaries.jsonl"), after, 0o600); err != nil {
					return err
				}
				if scenario == "unapplied_independent_change" {
					return os.WriteFile(filepath.Join(stage.DataDir, "private", "b-summary-after-prefix.bin"), []byte("remaining marker"), 0o600)
				}
				return os.WriteFile(filepath.Join(stage.DurableDir, "threads", "zz-independent", "z-tail.bin"), []byte("independent tail"), 0o600)
			}
			if scenario == "replace_held" {
				runtimeSummaryReplacementCutForTestV1(t, core, index, tail, after, mutation)
			} else {
				runtimePendingSemanticCutForTestV1(t, core, mutation, installed, absent)
			}
			fresh, err := runtimeObservePendingCoreForTestV1(t, core)
			if err != nil {
				t.Fatal(err)
			}
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, fresh)
			positive := scenario == "absent_add_independent" || scenario == "unapplied_independent_change"
			if positive && err != nil {
				t.Fatalf("provable independent summary cut rejected: %v", err)
			}
			if !positive && err == nil {
				t.Fatal("physical summary After became the original held-row inventory")
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("summary original observation changed state")
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
					t.Fatal("independent summary recovery lost exact final bytes")
				}
				if info, err := os.Lstat(index); err != nil || info.Mode().Perm() != 0o600 {
					t.Fatal("independent summary recovery changed index permissions")
				}
				final := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
				for path, entry := range before {
					if path == "1:thread_summaries.jsonl" {
						continue
					}
					if !semanticOriginalRecordUnchangedForTestV1(entry, final[path]) {
						t.Fatalf("independent summary recovery changed original file or mode at %s", path)
					}
				}
			}
		})
	}
}
