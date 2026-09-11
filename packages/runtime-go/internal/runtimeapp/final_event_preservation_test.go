package runtimeapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	appturn "analytix.local/runtime-go/internal/app/turn"
	turnterminalapp "analytix.local/runtime-go/internal/app/turnterminal"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	"analytix.local/runtime-go/internal/server"
)

type runtimeNoExecutableFinalThreadsV1 struct{}

func (runtimeNoExecutableFinalThreadsV1) AllThreadIDs() ([]string, error) { return []string{}, nil }
func (runtimeNoExecutableFinalThreadsV1) GetThread(string) (map[string]any, error) {
	return nil, errors.New("no executable thread")
}

func TestRuntimeFinalEventPreservationUsesOriginalCompleteDenominator(t *testing.T) {
	for _, scenario := range []string{"completed_child", "no_final", "omitted_private", "primary_drift", "events_drift", "event_hardlink", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			var core *runtimeChildIdentityStartupV1
			if scenario == "no_final" {
				core, _ = runtimeReportPreservationFixtureV1(t, false, true, false)
			} else {
				core = runtimeStoredChildCompletionFixtureV1(t, "complete")
			}
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			original, _, _, err := readRuntimeAcceptedFinalSemanticInventoryV1(ctx, core, preserved.report)
			if err != nil {
				t.Fatal(err)
			}
			inventory := evidenceapp.FinalAuthorityInventory{}
			terminal := turnterminalapp.RestartRecoveryResultV1{}
			for _, record := range original.records {
				if !preserved.report.OwnsThread(record.SecurityContext.ThreadID) {
					continue
				}
				inventory.Committed = append(inventory.Committed, record)
				terminal.Preserved = append(terminal.Preserved, record)
			}
			if scenario == "omitted_private" {
				terminal.Preserved = nil
			}
			store, err := server.NewRuntimeEventSessionStoreWithPreservationV1(server.RuntimeServerConfig{DurableTempDir: core.roots.DurableDir}, false, preserved.durable, preserved.usage, core.floors)
			if err != nil {
				t.Fatal(err)
			}
			executable := runtimeNoExecutableFinalThreadsV1{}
			observer := preserved.finalEventObserverV1(inventory, terminal, executable)
			if scenario == "event_hardlink" {
				path := filepath.Join(core.roots.DurableDir, "threads", core.jobRecords[0].ChildThreadID, "events.jsonl")
				if err := os.Link(path, filepath.Join(t.TempDir(), "synthetic-event-link")); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "primary_drift" || scenario == "events_drift" {
				leaf := "thread.json"
				if scenario == "events_drift" {
					leaf = "events.jsonl"
				}
				path := filepath.Join(core.roots.DurableDir, "threads", preserved.report.ThreadIDs()[0], leaf)
				if err := os.WriteFile(path, []byte("{\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			calls := 0
			io := evidenceapp.FinalPublicationEventIO{
				ReadThread: func(context.Context, appturn.AcceptedFinalCompletionStore, domainevidence.PrivateAcceptedFinalRecord) (map[string]any, error) {
					calls++
					return nil, errors.New("held live read")
				},
				LoadEvents: func(context.Context, appturn.AcceptedFinalCompletionStore, string) ([]map[string]any, error) {
					calls++
					return nil, errors.New("held live replay")
				},
				AppendEvents: func(context.Context, appturn.AcceptedFinalCompletionStore, []map[string]any) ([]map[string]any, error) {
					calls++
					return nil, errors.New("held append")
				},
			}
			plans, err := evidenceapp.PreflightAcceptedFinalEventsWithPreservationV1(ctx, io, store, executable, nil, nil, nil, observer)
			if scenario == "completed_child" || scenario == "no_final" {
				if err != nil || len(plans) != 0 {
					t.Fatalf("original held proof rejected or became repairable: %v", err)
				}
				if err := evidenceapp.ApplyAcceptedFinalEventsWithPreservationV1(ctx, io, store, executable, plans, nil, nil, nil, observer); err != nil {
					t.Fatal(err)
				}
			} else if err == nil || scenario == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("original drift/denominator/cancellation accepted: %v", err)
			}
			if scenario == "event_hardlink" {
				t.Logf("original hardlink refusal: %v", err)
			}
			if calls != 0 || !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("original observation reached live effects or rewrote state")
			}
		})
	}
}
