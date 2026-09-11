package thread

import (
	"errors"
	"testing"
)

func TestBuildRewindTruncatesTurnsAndReturnsRemovedIDs(t *testing.T) {
	source := map[string]any{
		"id":        "thr_1",
		"status":    "completed",
		"updatedAt": "old",
		"turns": []any{
			map[string]any{"id": "turn_1"},
			map[string]any{"id": "turn_2"},
			map[string]any{"id": "turn_3"},
		},
	}
	plan, err := BuildRewind(RewindInput{
		Thread:   source,
		ThreadID: "thr_1",
		TurnID:   "turn_2",
		Now:      "2026-07-03T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("build rewind: %v", err)
	}
	if plan.Thread["status"] != "idle" || plan.Thread["updatedAt"] != "2026-07-03T00:00:00Z" {
		t.Fatalf("rewound thread status mismatch: %#v", plan.Thread)
	}
	remaining, _ := plan.Thread["turns"].([]any)
	if len(remaining) != 1 {
		t.Fatalf("remaining turns mismatch: %#v", remaining)
	}
	removed, _ := plan.Response["removedTurnIds"].([]any)
	if plan.Response["removedTurns"] != float64(2) || plan.Response["remainingTurns"] != float64(1) {
		t.Fatalf("rewind counts mismatch: %#v", plan.Response)
	}
	if len(removed) != 2 || removed[0] != "turn_2" || removed[1] != "turn_3" {
		t.Fatalf("removed turn IDs mismatch: %#v", removed)
	}
	sourceTurns, _ := source["turns"].([]any)
	if len(sourceTurns) != 3 || source["updatedAt"] != "old" {
		t.Fatalf("rewind should not mutate input: %#v", source)
	}
}

func TestBuildRewindRejectsRunningThread(t *testing.T) {
	_, err := BuildRewind(RewindInput{
		Thread: map[string]any{"status": "RUNNING", "turns": []any{map[string]any{"id": "turn_1"}}},
		TurnID: "turn_1",
	})
	if !errors.Is(err, ErrThreadRunning) {
		t.Fatalf("expected ErrThreadRunning, got %v", err)
	}
}

func TestBuildRewindRejectsMissingTurn(t *testing.T) {
	_, err := BuildRewind(RewindInput{
		Thread: map[string]any{"status": "idle", "turns": []any{map[string]any{"id": "turn_1"}}},
		TurnID: "missing",
	})
	if !errors.Is(err, ErrTurnNotFound) {
		t.Fatalf("expected ErrTurnNotFound, got %v", err)
	}
}

func TestRewindCommittedFinalRejected(t *testing.T) {
	tests := []struct {
		name string
		turn map[string]any
	}{
		{
			name: "turn authority",
			turn: map[string]any{
				"id":            "turn_final",
				"acceptedFinal": map[string]any{"schemaVersion": float64(2)},
			},
		},
		{
			name: "item authority",
			turn: map[string]any{
				"id": "turn_final",
				"items": []any{map[string]any{
					"kind":          "assistant_text",
					"acceptedFinal": map[string]any{"schemaVersion": float64(2)},
				}},
			},
		},
		{
			name: "terminal authority residue",
			turn: map[string]any{
				"id":              "turn_final",
				"status":          "completed",
				"securityContext": map[string]any{"contextDigest": "case-context"},
				"items": []any{map[string]any{
					"kind":                "error",
					"acceptedFinalDigest": "accepted-final-digest",
				}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := map[string]any{
				"id":     "thr_1",
				"status": "completed",
				"turns": []any{
					map[string]any{"id": "turn_before"},
					test.turn,
					map[string]any{"id": "turn_after"},
				},
			}
			_, err := BuildRewind(RewindInput{
				Thread: source,
				TurnID: "turn_final",
				Now:    "2026-07-11T00:00:00Z",
			})
			if !errors.Is(err, ErrAcceptedFinalRewind) {
				t.Fatalf("expected ErrAcceptedFinalRewind, got %v", err)
			}
			if len(source["turns"].([]any)) != 3 {
				t.Fatalf("rejected rewind mutated source: %#v", source)
			}
			_, err = BuildRewind(RewindInput{
				Thread: source,
				TurnID: "turn_before",
				Now:    "2026-07-11T00:00:00Z",
			})
			if !errors.Is(err, ErrAcceptedFinalRewind) {
				t.Fatalf("rewind starting before accepted final should be rejected, got %v", err)
			}
		})
	}
}

func TestAcceptedFinalRewindPreflightRejectsBeforeMutationAuthority(t *testing.T) {
	thread := map[string]any{"id": "thr", "turns": []any{
		map[string]any{"id": "turn-1", "items": []any{}},
		map[string]any{"id": "turn-2", "acceptedFinal": map[string]any{"recordDigest": "signed"}, "items": []any{}},
	}}
	if err := ValidateRewindAcceptedFinalProtection(thread, "turn-1"); !errors.Is(err, ErrAcceptedFinalRewind) {
		t.Fatalf("accepted final preflight did not reject: %v", err)
	}
	if err := ValidateRewindAcceptedFinalProtection(thread, "missing"); !errors.Is(err, ErrTurnNotFound) {
		t.Fatalf("missing turn preflight mismatch: %v", err)
	}
}
