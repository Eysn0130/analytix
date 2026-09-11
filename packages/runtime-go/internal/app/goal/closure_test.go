package goal

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToolResponseCalculatesRemainingBudgetAndAudits(t *testing.T) {
	response := ToolResponseWithAudits(
		map[string]any{"status": "complete", "tokenBudget": float64(100), "tokensUsed": float64(35)},
		"done",
		map[string]any{"count": float64(3)},
		map[string]any{"selfCheckRequired": true},
	)
	if response["remainingTokens"] != float64(65) || response["completionBudgetReport"] != "done" {
		t.Fatalf("budget response mismatch: %#v", response)
	}
	if response["blockedAudit"] == nil || response["completionAudit"] == nil {
		t.Fatalf("audits missing: %#v", response)
	}
}

func TestNormalizeTokenBudget(t *testing.T) {
	if budget, ok, err := NormalizeTokenBudget(json.Number("42")); err != nil || !ok || budget != 42 {
		t.Fatalf("unexpected budget result budget=%d ok=%v err=%v", budget, ok, err)
	}
	if _, ok, err := NormalizeTokenBudget(nil); err != nil || ok {
		t.Fatalf("nil budget should be absent ok=%v err=%v", ok, err)
	}
	if _, _, err := NormalizeTokenBudget(0); err == nil {
		t.Fatal("zero budget should be rejected")
	}
}

func TestEvidenceDetailsHostVerified(t *testing.T) {
	if !EvidenceDetailsHostVerified([]any{map[string]any{"hostVerified": true}, map[string]any{"hostVerified": true}}) {
		t.Fatal("all host verified details should pass")
	}
	if EvidenceDetailsHostVerified([]any{map[string]any{"hostVerified": true}, map[string]any{"hostVerified": false}}) {
		t.Fatal("mixed host verified details should fail")
	}
	if EvidenceDetailsHostVerified(nil) {
		t.Fatal("empty details should fail")
	}
}

func TestOutputMentionsPath(t *testing.T) {
	output := map[string]any{
		"command": "go test ./packages/runtime-go",
		"diff":    map[string]any{"path": "./packages/runtime-go/internal/server/tools_execution.go"},
	}
	if !OutputMentionsPath(output, "packages/runtime-go/internal/server/tools_execution.go") {
		t.Fatalf("expected path match in output: %#v", output)
	}
	if OutputMentionsPath(output, "src/main/index.ts") {
		t.Fatal("unexpected unrelated path match")
	}
	if !ToolWritesPath("edit_file") || !ToolReadsOrWritesPath("grep") || ToolReadsOrWritesPath("web_fetch") {
		t.Fatal("tool read/write classification mismatch")
	}
}

func TestFindTodoStepIndex(t *testing.T) {
	items := []any{
		map[string]any{"id": "setup", "content": "Read current code"},
		map[string]any{"id": "fix", "content": "Implement fix"},
	}
	if got := FindTodoStepIndex(items, " implement   fix ", 0, true); got != 1 {
		t.Fatalf("content lookup mismatch: %d", got)
	}
	if got := FindTodoStepIndex(items, "setup", 1, true); got != 0 {
		t.Fatalf("id lookup mismatch: %d", got)
	}
	if got := FindTodoStepIndex(items, "wrong", 2, true); got != -1 {
		t.Fatalf("mismatched indexed step should fail: %d", got)
	}
	if got := FindTodoStepIndex(items, "", 2, false); got != 1 {
		t.Fatalf("index-only step should pass: %d", got)
	}
}

func TestBlockedReasonNormalizationAndSelfCheckInstructions(t *testing.T) {
	if got := CleanBlockedReason("  :Needs API key!! "); got != "Needs API key" {
		t.Fatalf("clean reason mismatch: %q", got)
	}
	if got := NormalizeBlockedReason("Needs--API_key!!"); got != "needs api key" {
		t.Fatalf("normalized reason mismatch: %q", got)
	}
	if !strings.Contains(StrictCompletionSelfCheckInstructions(), "complete_step using self_check: true") {
		t.Fatal("self-check instructions should include complete_step guidance")
	}
}
