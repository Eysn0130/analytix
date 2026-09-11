//go:build !analytix_prod

package runtimego

import (
	"testing"

	goal "analytix.local/runtime-go/internal/goal"
)

func TestGoalEvidenceCommandMatchesReasonixCompatibility(t *testing.T) {
	matches := []struct {
		cited string
		ran   string
	}{
		{"go test ./...", "go test ./..."},
		{"go test ./...", "cd packages/runtime-go && go test ./..."},
		{"go test ./...", "go test ./... | tee /tmp/analytix-go.log"},
		{"go test -run TestRuntimeServer ./...", "go test ./... -run TestRuntimeServer -count=1"},
		{`printf "hello world"`, `printf 'hello world'`},
		{"npm run typecheck", "npm run typecheck # verified after migration"},
	}
	for _, tt := range matches {
		if !goal.GoalEvidenceCommandMatches(tt.cited, tt.ran) {
			t.Fatalf("expected cited command %q to match ran command %q", tt.cited, tt.ran)
		}
	}

	rejects := []struct {
		cited string
		ran   string
	}{
		{"go test ./... -race", "go test ./..."},
		{"npm run typecheck", "npm run build"},
		{"test", "go test ./..."},
		{"go test ./...", "npm test -- go test ./..."},
	}
	for _, tt := range rejects {
		if goal.GoalEvidenceCommandMatches(tt.cited, tt.ran) {
			t.Fatalf("expected cited command %q not to match ran command %q", tt.cited, tt.ran)
		}
	}
}

func TestGoalEvidenceBlocksThenRecoversAfterProjectCheck(t *testing.T) {
	base := goal.GoalEvidenceInput{
		ThreadID: "thr_goal",
		TurnID:   "turn_goal",
		GoalID:   "goal_thr_goal",
		Receipts: []goal.GoalEvidenceReceipt{{
			Index:           1,
			Command:         "apply_patch packages/runtime-go/internal/goal/goal_evidence.go",
			Success:         true,
			WritesWorkspace: true,
			TouchedPaths:    []string{"packages/runtime-go/internal/goal/goal_evidence.go"},
		}},
		ProjectChecks: []goal.GoalEvidenceProjectCheck{{
			ID:                 "runtime-go-unit",
			Command:            "go test ./...",
			RequiredAfterWrite: true,
		}},
		Todos: []goal.GoalEvidenceTodoItem{{ID: "ship", Status: "completed"}},
	}

	blocked := goal.EvaluateGoalEvidence(base)
	if blocked.Result != goal.GoalEvidenceBlocked ||
		blocked.MissingProjectChecks != 1 ||
		blocked.CommandMismatchMissing != 1 ||
		blocked.IncompleteTodos != 0 ||
		blocked.BlockedStateKey == "" {
		t.Fatalf("blocked audit mismatch: %#v", blocked)
	}
	if blocked.UsesReasonixPublicProtocol || blocked.UsesReasonixConfigRoot || blocked.ChangesRendererContract || blocked.ChangesProductIdentity {
		t.Fatalf("goal evidence audit must stay inside analytix runtime contract: %#v", blocked)
	}

	recoveredInput := base
	recoveredInput.Receipts = append([]goal.GoalEvidenceReceipt{}, base.Receipts...)
	recoveredInput.Receipts = append(recoveredInput.Receipts, goal.GoalEvidenceReceipt{
		Index:   2,
		Command: "cd packages/runtime-go && go test ./...",
		Success: true,
	})
	recoveredInput.PreviousBlockedAudits = []goal.GoalEvidenceAudit{blocked}

	recovered := goal.EvaluateGoalEvidence(recoveredInput)
	if recovered.Result != goal.GoalEvidenceAllowed ||
		!recovered.Recovered ||
		recovered.MissingProjectChecks != 0 ||
		recovered.CommandMismatchMissing != 0 ||
		recovered.BlockedStateKey != "" {
		t.Fatalf("recovered audit mismatch: %#v", recovered)
	}
}

func TestGoalEvidenceRepeatingSameBlockBecomesErrored(t *testing.T) {
	input := goal.GoalEvidenceInput{
		ThreadID: "thr_goal",
		TurnID:   "turn_goal",
		GoalID:   "goal_thr_goal",
		Receipts: []goal.GoalEvidenceReceipt{{
			Index:           1,
			Command:         "apply_patch packages/runtime-go/runtime_server.go",
			Success:         true,
			WritesWorkspace: true,
		}},
		ProjectChecks: []goal.GoalEvidenceProjectCheck{{
			ID:                 "runtime-go-unit",
			Command:            "go test ./...",
			RequiredAfterWrite: true,
		}},
		Todos: []goal.GoalEvidenceTodoItem{{ID: "finish-migration", Status: "in_progress"}},
	}

	first := goal.EvaluateGoalEvidence(input)
	secondInput := input
	secondInput.PreviousBlockedAudits = []goal.GoalEvidenceAudit{first}
	second := goal.EvaluateGoalEvidence(secondInput)
	thirdInput := input
	thirdInput.PreviousBlockedAudits = []goal.GoalEvidenceAudit{first, second}
	third := goal.EvaluateGoalEvidence(thirdInput)

	if first.Result != goal.GoalEvidenceBlocked || second.Result != goal.GoalEvidenceBlocked {
		t.Fatalf("first two audits should block before terminal escalation: first=%#v second=%#v", first, second)
	}
	if third.Result != goal.GoalEvidenceErrored ||
		third.MissingProjectChecks != 1 ||
		third.IncompleteTodos != 1 ||
		third.BlockedStateKey != first.BlockedStateKey {
		t.Fatalf("third repeated audit should become terminal errored: %#v", third)
	}
}

func TestRuntimeTurnGoalEvidenceAuditsEmitBlockedAndRecoveredEvents(t *testing.T) {
	audits := goal.RuntimeTurnGoalEvidenceAudits("thr_g2_read", "turn_d0242_1")
	if len(audits) != 2 {
		t.Fatalf("runtime turn should emit blocked and recovered goal evidence audits: %#v", audits)
	}
	if audits[0].Result != goal.GoalEvidenceBlocked || audits[1].Result != goal.GoalEvidenceAllowed || !audits[1].Recovered {
		t.Fatalf("runtime turn audit states mismatch: %#v", audits)
	}
	event := goal.GoalEvidenceAuditEvent(audits[0])
	if event["kind"] != "goal_evidence_audit" ||
		event["threadId"] != "thr_g2_read" ||
		event["turnId"] != "turn_d0242_1" ||
		event["usesReasonixPublicProtocol"] != false {
		t.Fatalf("goal evidence event shape mismatch: %#v", event)
	}
}
