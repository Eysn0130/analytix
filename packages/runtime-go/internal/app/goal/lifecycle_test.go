package goal

import (
	domaingoal "analytix.local/runtime-go/internal/domain/goal"
	"errors"
	"reflect"
	"testing"

	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
)

func lifecycleGoalV1(t *testing.T) map[string]any {
	t.Helper()
	goal, err := ApplyLifecycleV1(StatePatchInput{ThreadID: "thread-goal", Patch: map[string]any{"operation": "create", "objective": "Verify the deliverable", "maxGoalRounds": 3}, Now: "2026-10-11T00:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	return goal
}
func goalRefPatchV1(goal map[string]any, operation string) map[string]any {
	return map[string]any{"operation": operation, "ref": map[string]any{"id": goal["id"], "revision": goal["revision"]}}
}
func TestGoalLifecycleCASPreservesPhaseAndCumulativeRounds(t *testing.T) {
	goal := lifecycleGoalV1(t)
	goal["roundsStarted"] = 2
	paused, err := ApplyLifecycleV1(StatePatchInput{Existing: goal, Patch: goalRefPatchV1(goal, "pause"), Now: "paused"})
	if err != nil {
		t.Fatal(err)
	}
	patch := goalRefPatchV1(paused, "edit")
	patch["objective"] = "Verify the updated deliverable"
	patch["maxGoalRounds"] = 4
	edited, err := ApplyLifecycleV1(StatePatchInput{Existing: paused, Patch: patch, Now: "edited"})
	_, _, editedRounds, editedKnown := domaingoal.CountedRecordV1(edited)
	if err != nil || edited["status"] != "paused" || !editedKnown || editedRounds != 2 {
		t.Fatal("edit changed phase or cumulative count", edited, err)
	}
	if _, err = ApplyLifecycleV1(StatePatchInput{Existing: edited, Patch: goalRefPatchV1(paused, "resume")}); !errors.Is(err, ErrGoalRefConflictV1) {
		t.Fatal("stale reference admitted", err)
	}
	resumed, err := ApplyLifecycleV1(StatePatchInput{Existing: edited, Patch: goalRefPatchV1(edited, "resume"), Now: "resumed"})
	_, _, resumedRounds, resumedKnown := domaingoal.CountedRecordV1(resumed)
	if err != nil || resumed["status"] != "active" || !resumedKnown || resumedRounds != 2 {
		t.Fatal("resume reset count", resumed, err)
	}
}
func TestGoalBudgetExhaustionRequiresExplicitIncreaseAndFreshCreateIdentity(t *testing.T) {
	goal := lifecycleGoalV1(t)
	goal["roundsStarted"] = 3
	goal["status"] = "blocked"
	if _, err := ApplyLifecycleV1(StatePatchInput{Existing: goal, Patch: goalRefPatchV1(goal, "resume")}); !errors.Is(err, ErrGoalRoundLimitV1) {
		t.Fatal("budget exhaustion admitted", err)
	}
	patch := goalRefPatchV1(goal, "edit")
	patch["maxGoalRounds"] = 2
	if _, err := ApplyLifecycleV1(StatePatchInput{Existing: goal, Patch: patch}); err == nil {
		t.Fatal("cap below started admitted")
	}
	cleared, err := ApplyLifecycleV1(StatePatchInput{Existing: goal, Patch: goalRefPatchV1(goal, "clear")})
	if err != nil || cleared != nil {
		t.Fatal("clear failed", err)
	}
	fresh := lifecycleGoalV1(t)
	_, maximum, started, known := domaingoal.CountedRecordV1(fresh)
	if !known || maximum != 3 || started != 0 || fresh["id"] == goal["id"] {
		t.Fatal("create reused identity or counters", fresh)
	}
}
func TestLegacyGoalMutationKeepsMissingFactsUnknown(t *testing.T) {
	legacy := map[string]any{"threadId": "thread-goal", "objective": "Imported objective", "status": "paused"}
	updated, err := ApplyLifecycleV1(StatePatchInput{Existing: legacy, Patch: map[string]any{"status": "active"}, Now: "resumed"})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"id", "revision", "maxGoalRounds", "roundsStarted"} {
		if _, present := updated[key]; present {
			t.Fatalf("legacy %s was invented", key)
		}
	}
	if _, err = ApplyLifecycleV1(StatePatchInput{Existing: updated, Patch: map[string]any{"operation": "resume", "ref": map[string]any{"id": "invented", "revision": 1}}}); !errors.Is(err, ErrGoalRefConflictV1) {
		t.Fatal("legacy adopted counted identity", err)
	}
}
func TestGoalLifecycleCannotBypassCompletionEvidence(t *testing.T) {
	goal := lifecycleGoalV1(t)
	if _, err := ApplyLifecycleV1(StatePatchInput{Existing: goal, Patch: map[string]any{"status": "complete"}}); !errors.Is(err, ErrGoalRefConflictV1) {
		t.Fatal("host patch bypassed model closure", err)
	}
	if _, err := ApplyStatePatch(StatePatchInput{Existing: goal, Patch: map[string]any{"status": "complete"}}); err == nil {
		t.Fatal("empty evidence completed goal")
	}
	for _, patch := range []map[string]any{{"operation": "create", "objective": "x", "strictCompletion": "true"}, {"operation": "create", "objective": "x", "tokenBudget": 0}, {"operation": "create", "objective": "x", "maxGoalRounds": 1.5}} {
		if _, err := ApplyLifecycleV1(StatePatchInput{Patch: patch}); err == nil {
			t.Fatal("invalid lifecycle accepted", patch)
		}
	}
}

func TestAppendGoalClearHistoryUsesNilInterfaceSnapshot(t *testing.T) {
	for _, counted := range []bool{false, true} {
		name, operation := "legacy", "legacy_clear"
		previous := map[string]any{"threadId": "thread-goal", "objective": "Imported objective", "status": "paused"}
		if counted {
			name, operation = "counted", "clear"
			previous = lifecycleGoalV1(t)
		}
		t.Run(name, func(t *testing.T) {
			thread := map[string]any{"id": "thread-goal"}
			AppendHistoryV1(thread, previous, nil, operation, "2026-10-11T00:01:00Z")
			history, ok := thread["goalHistory"].([]any)
			if !ok || len(history) != 1 {
				t.Fatal("clear did not append one history entry", thread)
			}
			entry, ok := history[0].(map[string]any)
			if !ok {
				t.Fatal("clear history entry is not a record")
			}
			if snapshot, present := entry["goal"]; !present || snapshot != nil {
				t.Fatalf("clear snapshot must be a nil interface, got %T %#v", snapshot, snapshot)
			}
			if counted {
				ref, valid := ExpectedRefV1(entry)
				previousRef, _ := domaingoal.RefFromRecordV1(previous)
				if !valid || ref.ID != previousRef.ID || ref.Revision != previousRef.Revision+1 {
					t.Fatal("clear history lost the next counted reference", entry)
				}
			} else if _, present := entry["ref"]; present {
				t.Fatal("legacy clear invented a counted reference", entry)
			}
		})
	}
}

func TestAppendGoalHistoryPublishesSyntheticMetadata(t *testing.T) {
	const now = "2026-10-18T09:11:13.123456789Z"
	const id = "abcdefab-cdef-4abc-8def-131234567890"
	for _, counted := range []bool{false, true} {
		name, operation := "legacy-manual", "legacy_update"
		patch := map[string]any{"objective": "Verify the deliverable"}
		if counted {
			name, operation = "counted", "create"
			patch["operation"] = "create"
		}
		t.Run(name, func(t *testing.T) {
			goal, err := ApplyLifecycleV1(StatePatchInput{ThreadID: "thread-goal", Patch: patch, Now: now})
			if err != nil {
				t.Fatal(err)
			}
			if counted {
				goal["id"] = id
			}
			if _, _, _, known := domaingoal.CountedRecordV1(goal); known != counted {
				t.Fatal("legacy mutation changed counted admission", goal)
			}
			thread := map[string]any{
				"id": "thread-goal", "goal": goal, "status": "idle", "turns": []any{}, "createdAt": now, "updatedAt": now,
			}
			AppendHistoryV1(thread, nil, goal, operation, now)
			snapshot := thread["goalHistory"].([]any)[0].(map[string]any)["goal"].(map[string]any)
			if _, present := snapshot["evidenceLedger"]; present {
				t.Fatal("history copied the evidence ledger")
			}
			if err := domainprivacy.ValidatePublicValue(thread); err != nil {
				t.Fatal("ordinary Goal history publication rejected metadata", err)
			}
			projected, changed := domainprivacy.ProjectPublicValue(thread)
			if changed || !reflect.DeepEqual(projected, thread) {
				t.Fatal("ordinary Goal history projection changed metadata", projected)
			}
		})
	}
}
