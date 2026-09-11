//go:build darwin || linux

package runtimeapp

import (
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	"analytix.local/runtime-go/internal/server"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRuntimeReportRestartScopeUsesExactOriginalPrimaryFamilyWithoutWrites(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, unknown := range []bool{false, true} {
			t.Run(map[bool]string{false: "current", true: "legacy"}[legacy]+map[bool]string{false: "_open", true: "_unknown"}[unknown], func(t *testing.T) {
				core, path := runtimeReportPreservationFixtureV1(t, legacy, unknown, false)
				before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
				scope, err := prepareRuntimeReportRestartScopeV1(context.Background(), core)
				if err != nil || scope == nil || !reflect.DeepEqual(scope.ThreadIDs(), []string{"thread-report-held"}) || len(scope.Contexts()) != 1 {
					t.Fatalf("original scope unavailable: %v", err)
				}
				if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
					t.Fatal("scope construction wrote or recovered original state")
				}
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, append(body, ' '), 0o600); err != nil {
					t.Fatal(err)
				}
				if scope.RevalidatePrimary(context.Background()) == nil {
					t.Fatal("scope accepted byte drift in original family")
				}
			})
		}
	}
}

func TestRuntimeReportRestartScopeRejectsMissingOriginalGrantAndCancelledObservation(t *testing.T) {
	core, _ := runtimeReportPreservationFixtureV1(t, false, false, true)
	before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	if scope, err := prepareRuntimeReportRestartScopeV1(context.Background(), core); err == nil || scope != nil {
		t.Fatal("signed report without its original grant became held trusted Core")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if scope, err := prepareRuntimeReportRestartScopeV1(ctx, core); !errors.Is(err, context.Canceled) || scope != nil {
		t.Fatalf("cancelled observation: %v", err)
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
		t.Fatal("invalid graph refusal changed original bytes")
	}
}

func TestRuntimeReportRestartScopeEmptyInventoryDoesNotCreateAuthority(t *testing.T) {
	roots, err := persistencefs.ResolveRootSet(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(filepath.Join(roots.DataDir, "private"))
	if err != nil {
		t.Fatal(err)
	}
	rootAuthority, err := persistencefs.FreezeRootAuthority(roots)
	if err != nil {
		t.Fatal(err)
	}
	before := startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)
	core, err := prepareRuntimeChildIdentityStartupV1(context.Background(), roots, rootAuthority, access, nil)
	if err != nil {
		t.Fatal(err)
	}
	if scope, err := prepareRuntimeReportRestartScopeV1(context.Background(), core); err != nil || scope != nil {
		t.Fatalf("empty scope requires new authority: %v", err)
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)) {
		t.Fatal("empty scope minted authority or owner storage")
	}
}

func TestRuntimeReportRestartPreservationReachesConfiguredDurableConstructor(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, simulation := range []bool{false, true} {
			t.Run(map[bool]string{false: "current", true: "legacy"}[legacy]+map[bool]string{false: "_live", true: "_stage"}[simulation], func(t *testing.T) {
				core, primary := runtimeReportPreservationFixtureV1(t, legacy, true, false)
				before, err := os.ReadFile(primary)
				if err != nil {
					t.Fatal(err)
				}
				preserved, err := prepareRuntimeReportRestartPreservationV1(context.Background(), core)
				if err != nil || preserved.report == nil || preserved.durable == nil {
					t.Fatalf("root preservation unavailable: %v", err)
				}
				store, err := server.NewRuntimeEventSessionStoreWithPreservationV1(server.RuntimeServerConfig{DurableTempDir: core.roots.DurableDir}, simulation, preserved.durable, preserved.usage, core.floors)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := store.PatchThread("thread-report-held", map[string]any{"title": "FORBIDDEN"}); !errors.Is(err, casethreadapp.ErrRestartPreserved) {
					t.Fatalf("root scope did not reach primary guard: %v", err)
				}
				if simulation {
					if err := store.ApplySemanticStartupMigrationsAfterAuthorityRepair(); err != nil {
						t.Fatal(err)
					}
				}
				after, err := os.ReadFile(primary)
				if err != nil || string(after) != string(before) {
					t.Fatal("configured constructor changed original held primary")
				}
				if err := preserved.report.RevalidatePrimary(context.Background()); err != nil {
					t.Fatal(err)
				}
				if err := preserved.durable.Revalidate(context.Background(), core.roots.DurableDir); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
