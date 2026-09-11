package goal

import (
	"strings"
	"testing"
)

func TestApplyStatePatchCreatesAndUpdatesGoal(t *testing.T) {
	goal, err := ApplyStatePatch(StatePatchInput{
		ThreadID: "thr/one",
		Patch: map[string]any{
			"objective":        "  Ship it  ",
			"strictCompletion": true,
			"blockedCount":     float64(2),
			"research": map[string]any{
				"enabled":      true,
				"requirements": []any{"one", "two"},
			},
		},
		Now: "2026-07-03T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if goal["threadId"] != "thr/one" || goal["objective"] != "Ship it" || goal["status"] != "active" {
		t.Fatalf("unexpected goal: %#v", goal)
	}
	if goal["strictCompletion"] != true || goal["blockedCount"] != float64(2) {
		t.Fatalf("patch fields missing: %#v", goal)
	}
	research, _ := goal["research"].(map[string]any)
	if research["requirementCount"] != float64(2) {
		t.Fatalf("research projection mismatch: %#v", research)
	}
	stateRefDigest, _ := research["stateRefDigest"].(string)
	if len(stateRefDigest) != 64 || strings.Contains(stateRefDigest, "thr/one") {
		t.Fatalf("research state reference should be opaque: %#v", research)
	}
	for _, forbidden := range []string{
		"stateRelativePath", "taskSpecPath", "progressPath", "findingsPath", "directionsTriedPath", "iterationLogPath",
	} {
		if research[forbidden] != nil {
			t.Fatalf("goal research state exposed an operational path: %#v", research)
		}
	}
}

func TestApplyStatePatchValidatesCompletion(t *testing.T) {
	base := map[string]any{
		"id":               "goal_1",
		"status":           "active",
		"strictCompletion": true,
		"evidenceLedger":   []any{map[string]any{"step": "done"}},
	}
	_, err := ApplyStatePatch(StatePatchInput{
		ThreadID: "thr_1",
		Existing: base,
		Patch:    map[string]any{"status": "complete"},
		Todos: map[string]any{
			"items": []any{map[string]any{"status": "pending"}},
		},
		Now: "now",
	})
	if err == nil || !strings.Contains(err.Error(), "todos") {
		t.Fatalf("expected incomplete todos error, got %v", err)
	}
	_, err = ApplyStatePatch(StatePatchInput{
		ThreadID: "thr_1",
		Existing: base,
		Patch:    map[string]any{"status": "complete"},
		Todos: map[string]any{
			"items": []any{map[string]any{"status": "completed"}},
		},
		Now: "now",
	})
	if err == nil || !strings.Contains(err.Error(), "self-check") {
		t.Fatalf("expected self-check error, got %v", err)
	}
	complete, err := ApplyStatePatch(StatePatchInput{
		ThreadID: "thr_1",
		Existing: base,
		Patch: map[string]any{
			"status":             "complete",
			"selfCheckCompleted": true,
		},
		Todos: map[string]any{
			"items": []any{map[string]any{"status": "completed"}},
		},
		Now: "now",
	})
	if err != nil {
		t.Fatalf("complete goal: %v", err)
	}
	if complete["status"] != "complete" || complete["selfCheckCompleted"] != true {
		t.Fatalf("completion patch mismatch: %#v", complete)
	}
}

func TestAppendEvidenceNormalizesLedger(t *testing.T) {
	goal := map[string]any{
		"id":             "goal_1",
		"status":         "active",
		"evidenceLedger": []any{},
	}
	updated, record, err := AppendEvidence(EvidenceAppendInput{
		ThreadID: "thr_1",
		Goal:     goal,
		Entry: map[string]any{
			"step":       "  Verify  ",
			"evidence":   []any{"  command passed  ", map[string]any{"command": "go test ./..."}},
			"summary":    "  checked  ",
			"toolCallId": "call_1",
		},
		Now: "now",
	})
	if err != nil {
		t.Fatalf("append evidence: %v", err)
	}
	if record["id"] != "goal_ev_1" || record["step"] != "Verify" || record["summary"] != "checked" {
		t.Fatalf("record mismatch: %#v", record)
	}
	evidence, _ := record["evidence"].([]any)
	if len(evidence) != 2 || evidence[0] != "command passed" || evidence[1] != "go test ./..." {
		t.Fatalf("evidence mismatch: %#v", evidence)
	}
	ledger, _ := updated["evidenceLedger"].([]any)
	if len(ledger) != 1 {
		t.Fatalf("ledger mismatch: %#v", ledger)
	}
}

func TestAppendEvidenceRejectsMissingOrInactiveGoal(t *testing.T) {
	_, _, err := AppendEvidence(EvidenceAppendInput{})
	if err == nil {
		t.Fatal("expected missing goal error")
	}
	_, _, err = AppendEvidence(EvidenceAppendInput{
		Goal:  map[string]any{"status": "complete"},
		Entry: map[string]any{"step": "x", "evidence": []any{"x"}},
	})
	if err == nil || !strings.Contains(err.Error(), "not active") {
		t.Fatalf("expected inactive goal error, got %v", err)
	}
}

func TestGoalStateProjectsRestrictedPIIWithoutChangingAuthority(t *testing.T) {
	goal, err := ApplyStatePatch(StatePatchInput{
		ThreadID: "thread_6222020000000000000",
		Patch: map[string]any{
			"objective":     "核对卡号 6222020000000000000",
			"blockedReason": "电话 13800138000 无法联系",
		},
		Now: "now",
	})
	if err != nil {
		t.Fatalf("create projected goal: %v", err)
	}
	if goal["objective"] != "核对卡号 [ACCOUNT]" || goal["blockedReason"] != "电话 [PHONE] 无法联系" {
		t.Fatalf("goal display projection = %#v", goal)
	}
	if goal["threadId"] != "thread_6222020000000000000" {
		t.Fatalf("goal authority was mutated: %#v", goal)
	}

	updated, record, err := AppendEvidence(EvidenceAppendInput{
		ThreadID: "thread_6222020000000000000",
		Goal:     goal,
		Entry: map[string]any{
			"step":     "核对账号 6222020000000000000",
			"evidence": []any{"命令检查账号 6222020000000000000"},
			"summary":  "邮箱 analyst@example.com",
			"evidenceDetails": []any{map[string]any{
				"hostVerified": true,
				"command":      "verify 6222020000000000000",
			}},
		},
		Now: "later",
	})
	if err != nil {
		t.Fatalf("append projected evidence: %v", err)
	}
	if record["step"] != "核对账号 [ACCOUNT]" || record["summary"] != "邮箱 [EMAIL]" {
		t.Fatalf("goal evidence projection = %#v", record)
	}
	evidence := record["evidence"].([]any)
	if evidence[0] != "命令检查账号 [ACCOUNT]" {
		t.Fatalf("goal evidence text projection = %#v", evidence)
	}
	details := record["evidenceDetails"].([]any)
	if details[0].(map[string]any)["command"] != "verify [ACCOUNT]" || details[0].(map[string]any)["hostVerified"] != true {
		t.Fatalf("goal evidence details projection = %#v", details)
	}
	if updated["threadId"] != "thread_6222020000000000000" {
		t.Fatalf("goal update mutated authority: %#v", updated)
	}
}
