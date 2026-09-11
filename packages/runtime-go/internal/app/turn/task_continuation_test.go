package turn

import (
	"encoding/json"
	"strings"
	"testing"

	threaddomain "analytix.local/runtime-go/internal/domain/thread"
)

func TestAutomaticCompactionPreservesTypedContinuationWithoutEvidenceUpgrade(t *testing.T) {
	thread := automaticCompactionThreadV1("thr_auto", 5)
	thread["goal"] = map[string]any{
		"id": "goal_thr_auto", "objective": "核实账号 6222020000000000000 后仅发布证据支持内容", "status": "active",
		"evidenceLedger": []any{map[string]any{"id": "goal_ev_1", "step": "unverified check"}},
	}
	thread["todos"] = map[string]any{"items": []any{
		map[string]any{"id": "todo_pending", "content": "保留账号 6222020000000000000 的待核验状态", "status": "pending"},
		map[string]any{"id": "todo_failed", "content": "外部核验", "status": "failed", "statusReasonCode": "source_unavailable"},
		map[string]any{"id": "todo_done", "content": "已完成", "status": "completed"},
	}}
	lastTurn := listAny(thread["turns"])[4].(map[string]any)
	listAny(lastTurn["items"])[0].(map[string]any)["text"] = "<think>PRIVATE_REASONING_SENTINEL</think> keep latest constraint"
	plan := BuildThreadCompactionWithMode(thread, "thr_auto", "automatic_context_threshold", 100, "2026-07-22T00:00:00Z", true)
	if plan.Error != nil || !plan.Changed {
		t.Fatalf("automatic compaction plan = %#v err=%v", plan, plan.Error)
	}
	item := compactionItemFromTurnsV1(t, plan.NextTurns)
	continuation, err := threaddomain.ParseTaskContinuationSnapshotV1(item["taskContinuation"])
	if err != nil {
		t.Fatal(err)
	}
	if item["auto"] != true || continuation.Goal == nil || !strings.Contains(continuation.Goal.Objective, "[ACCOUNT]") ||
		len(continuation.Todos) != 2 || continuation.Todos[1].Status != "failed" || continuation.Todos[1].StatusReasonCode != "source_unavailable" ||
		len(continuation.EvidenceReferences) != 1 || continuation.EvidenceReferences[0].SupportStatus != threaddomain.TaskContinuationEvidenceStateV1 ||
		continuation.EvidenceAuthority != threaddomain.TaskContinuationEvidenceStateV1 {
		t.Fatalf("continuation projection = %#v", continuation)
	}
	if body := strings.Join(continuation.LatestUserConstraints, "\n"); strings.Contains(body, "6222020000000000000") {
		t.Fatalf("restricted PII entered continuation: %q", body)
	}
	body, _ := json.Marshal(item)
	if strings.Contains(string(body), "PRIVATE_REASONING_SENTINEL") {
		t.Fatalf("private reasoning entered compaction: %s", body)
	}
}

func TestRepeatedAutomaticCompactionCarriesValidatedAncestry(t *testing.T) {
	thread := automaticCompactionThreadV1("thr_repeat", 5)
	first := BuildThreadCompactionWithMode(thread, "thr_repeat", "automatic_context_threshold", 100, "2026-07-22T00:00:00Z", true)
	if first.Error != nil || !first.Changed {
		t.Fatalf("first compaction: %#v err=%v", first, first.Error)
	}
	firstItem := compactionItemFromTurnsV1(t, first.NextTurns)
	firstContinuation, err := threaddomain.ParseTaskContinuationSnapshotV1(firstItem["taskContinuation"])
	if err != nil {
		t.Fatal(err)
	}
	thread["turns"] = append(first.NextTurns,
		automaticCompactionTurnV1("thr_repeat", "turn_6", "new constraint"),
		automaticCompactionTurnV1("thr_repeat", "turn_7", "latest constraint"),
	)
	second := BuildThreadCompactionWithMode(thread, "thr_repeat", "automatic_context_threshold", 200, "2026-07-22T00:01:00Z", true)
	if second.Error != nil || !second.Changed {
		t.Fatalf("second compaction: %#v err=%v", second, second.Error)
	}
	secondContinuation, err := threaddomain.ParseTaskContinuationSnapshotV1(compactionItemFromTurnsV1(t, second.NextTurns)["taskContinuation"])
	if err != nil {
		t.Fatal(err)
	}
	if secondContinuation.PreviousContinuationDigest != firstContinuation.StateDigest ||
		secondContinuation.PreviousCompactionSourceDigest != first.Result.SourceDigest ||
		secondContinuation.EvidenceAuthority != threaddomain.TaskContinuationEvidenceStateV1 {
		t.Fatalf("repeated compaction ancestry = %#v", secondContinuation)
	}
}

func automaticCompactionThreadV1(threadID string, count int) map[string]any {
	turns := make([]any, 0, count)
	for index := 1; index <= count; index++ {
		turns = append(turns, automaticCompactionTurnV1(threadID, "turn_"+string(rune('0'+index)), "constraint "+string(rune('0'+index))))
	}
	return map[string]any{"id": threadID, "status": "idle", "turns": turns}
}

func automaticCompactionTurnV1(threadID, turnID, text string) map[string]any {
	return map[string]any{
		"id": turnID, "threadId": threadID, "status": "completed",
		"items": []any{map[string]any{"id": "item_" + turnID, "turnId": turnID, "threadId": threadID, "kind": "user_message", "role": "user", "status": "completed", "createdAt": "2026-07-22T00:00:00Z", "finishedAt": "2026-07-22T00:00:00Z", "text": text}},
	}
}

func compactionItemFromTurnsV1(t *testing.T, turns []any) map[string]any {
	t.Helper()
	turn, _ := turns[0].(map[string]any)
	items, _ := turn["items"].([]any)
	item, _ := items[0].(map[string]any)
	if item == nil || item["kind"] != "compaction" {
		t.Fatalf("compaction item missing: %#v", turns)
	}
	return item
}
