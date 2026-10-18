package eventlog

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	apploop "analytix.local/runtime-go/internal/app/loop"
	appturn "analytix.local/runtime-go/internal/app/turn"
	domaineventlifecycle "analytix.local/runtime-go/internal/domain/eventlifecycle"
)

func TestLogicalStepPublicationPreservesNumericTailUUIDThroughEventLogV1(t *testing.T) {
	const id = "bbbbbbbb-bbbb-4bbb-abbb-123456789012"
	const threadID = "thread-logical-publication"
	stages := []string{"logical_step_started", "logical_step_assembled", "logical_step_closed"}
	for _, route := range []string{"single", "single_atomic", "batch"} {
		for _, hostTimestamp := range []bool{false, true} {
			name := route + "/host_metadata_after_projection"
			if hostTimestamp {
				name = route + "/producer_timestamp"
			}
			t.Run(name, func(t *testing.T) {
				root := t.TempDir()
				store := NewStore(root)
				thread := map[string]any{"id": threadID, "turns": []any{map[string]any{"id": "turn-logical"}}}
				events := []map[string]any{}
				for index, stage := range stages {
					details := map[string]any{"schemaVersion": float64(1), "logicalStepId": id}
					if stage == "logical_step_closed" {
						details["assistantMessageAssembled"], details["llmMs"] = true, float64(80)
						details["ttftMs"], details["decodeMs"], details["decodeTokens"] = float64(30), float64(50), float64(12)
					}
					timestamp := ""
					if hostTimestamp {
						timestamp = "2026-10-18T00:00:00.123456789Z"
					}
					// The real producer emits a draft before eventrecording assigns seq.
					draft := apploop.BuildPipelineStageEvent(apploop.PipelineStageEventInput{ThreadID: threadID, TurnID: "turn-logical", Stage: stage, Timestamp: timestamp, Details: details})
					before, _ := json.Marshal(draft)
					projected, err := appturn.SanitizeCaseEventPublication(thread, draft)
					if err != nil {
						t.Fatalf("%s publication projection: %v", stage, err)
					}
					after, _ := json.Marshal(draft)
					if !bytes.Equal(before, after) {
						t.Fatal("sanitizer mutated the producer draft")
					}
					projected["seq"] = float64(index + 1)
					if !hostTimestamp {
						projected["timestamp"] = "2026-10-18T00:00:00.123456789Z"
					}
					if err := domaineventlifecycle.ValidateNewEventLifecycleV1(projected); err != nil {
						t.Fatalf("%s lifecycle after publication projection: %v", stage, err)
					}
					events = append(events, projected)
					if route != "batch" {
						if route == "single_atomic" && stage == "logical_step_assembled" {
							err = store.AppendEvents(threadID, []map[string]any{projected}, true)
						} else {
							err = store.AppendEvent(threadID, projected)
						}
						if err != nil {
							t.Fatalf("%s append: %v", stage, err)
						}
					}
				}
				if route == "batch" {
					if err := store.AppendEventsAtomic(threadID, events); err != nil {
						t.Fatalf("atomic lifecycle append: %v", err)
					}
				}
				body, err := os.ReadFile(store.EventsPath(threadID))
				if err != nil {
					t.Fatal(err)
				}
				strict, err := ParseCommittedEventLogBytesV1(context.Background(), threadID, body)
				if err != nil {
					t.Fatalf("strict committed JSON readback: %v", err)
				}
				assertLogicalStepPublicationIDsV1(t, strict, stages, id)
				for _, reader := range []*Store{store, NewStore(root)} {
					loaded, err := reader.LoadSince(threadID, 0)
					if err != nil || len(loaded.Diagnostics) != 0 {
						t.Fatalf("live/restart readback: diagnostics=%#v err=%v", loaded.Diagnostics, err)
					}
					assertLogicalStepPublicationIDsV1(t, loaded.Events, stages, id)
				}
			})
		}
	}
}

func assertLogicalStepPublicationIDsV1(t *testing.T, events []map[string]any, stages []string, id string) {
	t.Helper()
	if len(events) != len(stages) {
		t.Fatalf("durable lifecycle count = %d, want %d", len(events), len(stages))
	}
	for index, event := range events {
		details, _ := event["details"].(map[string]any)
		if event["stage"] != stages[index] || details["logicalStepId"] != id {
			t.Fatalf("durable lifecycle correlation changed at %s: %#v", stages[index], event)
		}
		if err := domaineventlifecycle.ValidateNewEventLifecycleV1(event); err != nil {
			t.Fatalf("strict/live/restart lifecycle invalid at %s: %v", stages[index], err)
		}
	}
}
