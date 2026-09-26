package turn

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	threaddomain "analytix.local/runtime-go/internal/domain/thread"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestContinuationUserSourcesRepeatedCompactionReadbackAndRevision(t *testing.T) {
	thread := automaticCompactionThreadV1("thr-sources", 8)
	workspace := t.TempDir()
	scope, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{ThreadID: "thr-sources", TurnID: "active", WorkspaceRealPath: workspace, ContextEpoch: 1, IssuedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	thread["workspace"] = workspace
	// This fixture exercises ordinary durable JSON readback without a Goal.
	turns := listAny(thread["turns"])
	first := turns[0].(map[string]any)["items"].([]any)[0].(map[string]any)
	first["text"] = "Only modify A.txt; never modify B.txt. " + strings.Repeat("large original user context. ", 1000)
	plan := BuildThreadCompactionWithMode(thread, "thr-sources", "automatic_context_threshold", 100, "2026-09-26T00:00:00Z", true)
	if plan.Error != nil || !plan.Changed {
		t.Fatal("first compaction", plan.Error)
	}
	thread["turns"] = append(plan.NextTurns, automaticCompactionTurnV1("thr-sources", "turn-correction", "Correction: use C.txt instead of A.txt; B.txt remains forbidden."), automaticCompactionTurnV1("thr-sources", "turn-next", "Continue the current task."))
	plan = BuildThreadCompactionWithMode(thread, "thr-sources", "automatic_context_threshold", 200, "2026-09-26T00:01:00Z", true)
	if plan.Error != nil || !plan.Changed {
		t.Fatal("second compaction", plan.Error)
	}
	thread["turns"] = plan.NextTurns
	body, _ := json.Marshal(thread)
	var reopened map[string]any
	if json.Unmarshal(body, &reopened) != nil {
		t.Fatal("readback")
	}
	item := compactionItemFromTurnsV1(t, listAny(reopened["turns"]))
	snapshot, err := threaddomain.ParseTaskContinuationSnapshotV1(item["taskContinuation"])
	if err != nil || snapshot.UserHistory == nil || len(snapshot.UserHistory.Sources) != 10 {
		t.Fatal("source ancestry lost or duplicated", err)
	}
	if !strings.HasPrefix(snapshot.UserHistory.Sources[0].Text, "Only modify A.txt") || !strings.HasPrefix(snapshot.UserHistory.Sources[8].Text, "Correction:") {
		t.Fatal("source revision order changed")
	}
	projected, _ := json.Marshal(threaddomain.ProviderContinuationMapV1(snapshot))
	if strings.Contains(string(projected), "large original user context") || !strings.Contains(string(projected), "read_task_history") {
		t.Fatal("large source was inlined or became unreadable")
	}
	ref := snapshot.UserHistory.Sources[0].Reference
	page, err := ReadTaskContinuationSourceV1(reopened, scope, ref, 0, 80)
	if err != nil || !strings.HasPrefix(page["text"].(string), "Only modify A.txt") || page["complete"] != false {
		t.Fatal("budgeted source read failed", err)
	}
	if _, err := ReadTaskContinuationSourceV1(reopened, scope, ref, 0, 16001); err == nil {
		t.Fatal("read budget bypassed")
	}
	reopened["workspace"] = workspace + "-other"
	if _, err := ReadTaskContinuationSourceV1(reopened, scope, ref, 0, 80); err == nil {
		t.Fatal("old workspace source revived")
	}
	if ContinuationSourceReadableInThreadV1(reopened, snapshot.UserHistory) {
		t.Fatal("inline history crossed workspace")
	}
}

func TestContinuationSourceOverflowDoesNotDropEarlyInstructions(t *testing.T) {
	thread := automaticCompactionThreadV1("thr-source-budget", 8)
	first := listAny(thread["turns"])[0].(map[string]any)["items"].([]any)[0].(map[string]any)
	first["text"] = strings.Repeat("字", 400_000)
	before, _ := json.Marshal(thread)
	plan := BuildThreadCompactionWithMode(thread, "thr-source-budget", "automatic_context_threshold", 100, "2026-09-26T00:00:00Z", true)
	after, _ := json.Marshal(thread)
	if plan.Error == nil || plan.Changed || string(before) != string(after) {
		t.Fatal("source budget silently truncated or mutated original history")
	}
}

func TestContinuationSourceReadRejectsChangedPrincipalCaseAndThread(t *testing.T) {
	workspace := t.TempDir()
	current, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{ThreadID: "thr-scoped-source", TurnID: "active", WorkspaceRealPath: workspace, UserID: "source-user", ContextEpoch: 1})
	if err != nil {
		t.Fatal(err)
	}
	thread := automaticCompactionThreadV1(current.ThreadID, 8)
	thread["workspace"], thread["securityState"] = workspace, current
	for _, raw := range listAny(thread["turns"]) {
		turn := raw.(map[string]any)
		frozen, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{ThreadID: current.ThreadID, TurnID: stringField(turn, "id"), WorkspaceRealPath: workspace, UserID: current.UserID, ContextEpoch: 1})
		if err != nil {
			t.Fatal(err)
		}
		turn["securityContext"] = frozen
	}
	history, err := continuationUserHistoryV1(thread)
	if err != nil || history == nil || len(history.Sources) == 0 {
		t.Fatal("current source missing", err)
	}
	ref := history.Sources[0].Reference
	if _, err := ReadTaskContinuationSourceV1(thread, current, ref, 0, 80); err != nil {
		t.Fatal("current source rejected", err)
	}
	other, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{ThreadID: current.ThreadID, TurnID: "other", WorkspaceRealPath: workspace, UserID: "another-user", ContextEpoch: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadTaskContinuationSourceV1(thread, other, ref, 0, 80); err == nil {
		t.Fatal("another principal read old source")
	}
	thread["securityState"] = other
	if _, err := ReadTaskContinuationSourceV1(thread, other, ref, 0, 80); err == nil {
		t.Fatal("revoked source revived under new principal")
	}
	caseContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{ThreadID: current.ThreadID, TurnID: "case", WorkspaceRealPath: workspace, ContextEpoch: 3})
	if err != nil {
		t.Fatal(err)
	}
	thread["securityState"] = caseContext
	if _, err := ReadTaskContinuationSourceV1(thread, caseContext, ref, 0, 80); err == nil {
		t.Fatal("ordinary source crossed case boundary")
	}
	thread["securityState"], thread["id"] = current, "another-thread"
	if _, err := ReadTaskContinuationSourceV1(thread, current, ref, 0, 80); err == nil {
		t.Fatal("source crossed thread boundary")
	}
}
