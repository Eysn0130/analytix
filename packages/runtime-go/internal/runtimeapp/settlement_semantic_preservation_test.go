//go:build darwin || linux

package runtimeapp

import (
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	startupport "analytix.local/runtime-go/internal/ports/startup"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// A signed audit-storage fixture, not current DSV2 execution authority.

type cancelAfterOriginalOwnerModeV1 struct {
	context.Context
	cancel context.CancelFunc
	path   string
}

func (ctx cancelAfterOriginalOwnerModeV1) Err() error {
	if info, err := os.Stat(ctx.path); err == nil && info.Mode().Perm() == 0o500 {
		ctx.cancel()
	}
	return ctx.Context.Err()
}

func TestRuntimeOriginalRegistrySettlementRejectAppliedAncestorJournal(t *testing.T) {
	ctx := context.Background()
	core, _ := runtimeReportPreservationFixtureV1(t, false, true, false)
	private := filepath.Join(core.roots.DataDir, "private")
	t.Cleanup(func() { _ = os.Chmod(private, 0o700) })
	tail := filepath.Join(private, "z-original-mode-tail.bin")
	if err := os.WriteFile(tail, []byte("independent-tail"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := persistencefs.NewStartupSnapshotReader(core.roots).CaptureManagedSnapshotV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("original-owner-ancestor-cut"))
	baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	stagePrivate := ""
	plan, err := persistencefs.NewSemanticPlanBuilder(core.roots).Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		stagePrivate = filepath.Join(stage.DataDir, "private")
		if err := os.Chmod(filepath.Join(stage.DataDir, "private", "z-original-mode-tail.bin"), 0o400); err != nil {
			return err
		}
		return os.Chmod(filepath.Join(stage.DataDir, "private"), 0o500)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		// Restore only this fixture's exact planning directory after all byte
		// and mode assertions, so the normal authenticated Close can remove it.
		if err := os.Chmod(stagePrivate, 0o700); err != nil {
			t.Error(err)
		}
		if err := plan.Close(); err != nil {
			t.Error(err)
		}
	}()
	cancelled, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := plan.Apply(cancelAfterOriginalOwnerModeV1{Context: cancelled, cancel: cancel, path: private}); !errors.Is(err, context.Canceled) {
		t.Fatalf("signed ancestor operation did not reach physical cut: %v", err)
	}
	if info, err := os.Stat(private); err != nil || info.Mode().Perm() != 0o500 {
		t.Fatalf("ancestor cut not physically applied: %v", err)
	}
	if info, err := os.Stat(tail); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("ancestor cut crossed its remaining mode operation: %v", err)
	}
	fresh, err := runtimeObservePendingCoreForTestV1(t, core)
	if err != nil {
		t.Fatal(err)
	}
	before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	if _, err := prepareRuntimeReportRestartPreservationV1(ctx, fresh); err == nil || !strings.Contains(err.Error(), "ancestor") {
		t.Fatalf("original owner accepted applied ancestor journal: %v", err)
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
		t.Fatal("ancestor rejection changed physical inventory")
	}
}

func TestRuntimeSemanticSettlementPreservesOriginalHeldRecords(t *testing.T) {
	for _, scenario := range []string{"remove-held", "held-mode", "add-held"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			core, _ := runtimeReportPreservationFixtureV1(t, false, true, false)
			scope, err := prepareRuntimeReportRestartScopeV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
			if err != nil {
				t.Fatal(err)
			}
			record := runtimePreparedSettlementForSemanticTestV1(t, scope.Contexts()[0], authority)
			body, err := domainevidence.PreparedEvidenceSettlementBytes(record)
			if err != nil {
				t.Fatal(err)
			}
			relative := "private/evidence-settlements/prepared/" + record.SettlementID[:2] + "/" + record.SettlementID + ".json"
			file := filepath.Join(core.roots.DataDir, filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
				t.Fatal(err)
			}
			state := domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeFile, Mode: 0o600, Size: int64(len(body)), SHA256: domainsecurity.SHA256Hex(body)}
			before, after := state, domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeAbsent}
			kind := domainstartup.SemanticOperationRemoveFile
			if scenario == "add-held" {
				before, after = after, before
				kind = domainstartup.SemanticOperationInstallFile
			} else if err := os.WriteFile(file, body, 0o600); err != nil {
				t.Fatal(err)
			}
			if scenario == "held-mode" {
				after = before
				after.Mode = 0o400
				kind = domainstartup.SemanticOperationSetMode
			}
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			digest := domainsecurity.SHA256Hex([]byte("settlement-semantic-original"))
			plan, err := domainstartup.NewSemanticStartupPlanV1(digest, digest, digest, []domainstartup.SemanticStartupOperationV1{{Kind: kind, Path: "data/" + relative, Before: before, After: after}})
			if err != nil {
				t.Fatal(err)
			}
			if err := preserved.ValidateSemanticOperationsV1(ctx, plan.Operations, func(domainstartup.SemanticStartupOperationV1) ([]byte, error) { return body, nil }, ""); err == nil {
				t.Fatal("semantic operation changed original held prepared settlement")
			}
		})
	}
}
