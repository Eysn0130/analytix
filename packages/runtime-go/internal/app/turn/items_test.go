package turn

import (
	"errors"
	"reflect"
	"testing"
)

func TestAppendItemToTurnClonesThreadAndAppendsItem(t *testing.T) {
	source := map[string]any{
		"id":        "thr_1",
		"updatedAt": "old",
		"turns": []any{
			map[string]any{
				"id":    "turn_1",
				"items": []any{map[string]any{"id": "item_1"}},
			},
		},
	}
	item := map[string]any{"id": "item_2", "status": "completed"}
	thread, ok := AppendItemToTurn(AppendItemInput{
		Thread:    source,
		TurnID:    "turn_1",
		Item:      item,
		UpdatedAt: "now",
	})
	if !ok {
		t.Fatal("append did not find turn")
	}
	turns, _ := thread["turns"].([]any)
	turn, _ := turns[0].(map[string]any)
	items, _ := turn["items"].([]any)
	if len(items) != 2 || thread["updatedAt"] != "now" {
		t.Fatalf("append mismatch: %#v", thread)
	}
	item["status"] = "mutated"
	appended, _ := items[1].(map[string]any)
	if appended["status"] != "completed" {
		t.Fatalf("appended item should be cloned: %#v", appended)
	}
	sourceTurns, _ := source["turns"].([]any)
	sourceTurn, _ := sourceTurns[0].(map[string]any)
	sourceItems, _ := sourceTurn["items"].([]any)
	if len(sourceItems) != 1 || source["updatedAt"] != "old" {
		t.Fatalf("append should not mutate input: %#v", source)
	}
}

func TestEnsureItemToTurnExactIsIdempotentAndRejectsIdentityConflict(t *testing.T) {
	thread := map[string]any{"id": "thread", "turns": []any{map[string]any{
		"id": "turn", "items": []any{},
	}}}
	item := map[string]any{
		"id": "item_gate", "threadId": "thread", "turnId": "turn",
		"kind": "approval", "status": "pending", "approvalId": "appr",
		"contextEpoch": uint64(7), "authority": map[string]any{"sequence": uint64(9)},
	}
	first, changed, err := EnsureItemToTurnExact(AppendItemInput{Thread: thread, TurnID: "turn", Item: item, UpdatedAt: "first"})
	if err != nil || !changed {
		t.Fatalf("first ensure changed=%v err=%v", changed, err)
	}
	second, changed, err := EnsureItemToTurnExact(AppendItemInput{Thread: first, TurnID: "turn", Item: item, UpdatedAt: "second"})
	if err != nil || changed {
		t.Fatalf("exact retry changed=%v err=%v", changed, err)
	}
	turns := second["turns"].([]any)
	items := turns[0].(map[string]any)["items"].([]any)
	if len(items) != 1 || second["updatedAt"] != "first" {
		t.Fatalf("exact retry changed durable projection: %#v", second)
	}
	persisted := items[0].(map[string]any)
	if persisted["contextEpoch"] != float64(7) || persisted["authority"].(map[string]any)["sequence"] != float64(9) {
		t.Fatalf("exact item did not use canonical JSON numeric representation: %#v", persisted)
	}
	conflict := map[string]any{
		"id": "item_gate", "threadId": "thread", "turnId": "turn",
		"kind": "approval", "status": "pending", "approvalId": "other",
	}
	if _, _, err := EnsureItemToTurnExact(AppendItemInput{Thread: second, TurnID: "turn", Item: conflict}); err == nil {
		t.Fatal("same item id with different authority was accepted")
	}
}

func TestEnsureRecoveredToolSettlementExactIsAtomicAndIdempotent(t *testing.T) {
	const (
		threadID   = "thread_recovered"
		turnID     = "turn_recovered"
		callID     = "call_host_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		callItemID = "item_tool_host_v1_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		timestamp  = "2026-08-23T06:00:00Z"
	)
	result := map[string]any{
		"id":       "item_result_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"threadId": threadID, "turnId": turnID, "kind": "tool_result", "role": "tool",
		"toolName": "task", "callId": callID, "status": "failed", "isError": true,
		"createdAt": timestamp, "finishedAt": timestamp, "output": map[string]any{"status": "interrupted"},
	}
	source := map[string]any{"id": threadID, "updatedAt": "before", "turns": []any{map[string]any{
		"id": turnID, "threadId": threadID, "items": []any{map[string]any{
			"id": callItemID, "threadId": threadID, "turnId": turnID, "kind": "tool_call",
			"toolName": "task", "callId": callID, "status": "running",
		}},
	}}}
	input := RecoveredToolSettlementInput{
		Thread: source, ThreadID: threadID, TurnID: turnID, ToolCallItemID: callItemID,
		CallID: callID, ToolName: "task", Status: "failed", Timestamp: timestamp, ResultItem: result,
	}
	settled, changed, err := EnsureRecoveredToolSettlementExact(input)
	if err != nil || !changed {
		t.Fatalf("first settlement changed=%v err=%v", changed, err)
	}
	if reflect.DeepEqual(source, settled) {
		t.Fatal("settlement did not change cloned thread")
	}
	turn := settled["turns"].([]any)[0].(map[string]any)
	items := turn["items"].([]any)
	call := items[0].(map[string]any)
	if len(items) != 2 || call["status"] != "failed" || call["finishedAt"] != timestamp || settled["updatedAt"] != timestamp {
		t.Fatalf("atomic settlement mismatch: %#v", settled)
	}
	input.Thread = settled
	replayed, changed, err := EnsureRecoveredToolSettlementExact(input)
	if err != nil || changed || !reflect.DeepEqual(replayed, settled) {
		t.Fatalf("exact replay changed=%v err=%v replayed=%#v", changed, err, replayed)
	}

	patchOnly := map[string]any{"id": threadID, "turns": []any{map[string]any{
		"id": turnID, "threadId": threadID, "items": []any{map[string]any{
			"id": callItemID, "threadId": threadID, "turnId": turnID, "kind": "tool_call",
			"toolName": "task", "callId": callID, "status": "failed", "finishedAt": timestamp,
		}},
	}}}
	input.Thread = patchOnly
	repaired, changed, err := EnsureRecoveredToolSettlementExact(input)
	if err != nil || !changed || len(repaired["turns"].([]any)[0].(map[string]any)["items"].([]any)) != 2 {
		t.Fatalf("patch-only crash cut did not converge: changed=%v err=%v thread=%#v", changed, err, repaired)
	}
}

func TestEnsureRecoveredToolSettlementExactRejectsConflictsAndAcceptedFinal(t *testing.T) {
	const (
		threadID   = "thread_recovered"
		turnID     = "turn_recovered"
		callID     = "call_host_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		callItemID = "item_tool_host_v1_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		timestamp  = "2026-08-23T06:01:00Z"
	)
	result := map[string]any{
		"id":       "item_result_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"threadId": threadID, "turnId": turnID, "kind": "tool_result", "role": "tool",
		"toolName": "task", "callId": callID, "status": "failed", "isError": true,
		"createdAt": timestamp, "finishedAt": timestamp, "output": map[string]any{"status": "interrupted"},
	}
	baseTurn := map[string]any{"id": turnID, "threadId": threadID, "items": []any{map[string]any{
		"id": callItemID, "threadId": threadID, "turnId": turnID, "kind": "tool_call",
		"toolName": "task", "callId": callID, "status": "running",
	}}}
	input := RecoveredToolSettlementInput{
		Thread: map[string]any{"id": threadID, "turns": []any{baseTurn}}, ThreadID: threadID, TurnID: turnID,
		ToolCallItemID: callItemID, CallID: callID, ToolName: "task", Status: "failed", Timestamp: timestamp, ResultItem: result,
	}
	conflictThread := map[string]any{"id": threadID, "turns": []any{map[string]any{
		"id": turnID, "threadId": threadID, "items": append(baseTurn["items"].([]any), map[string]any{
			"id": result["id"], "threadId": threadID, "turnId": turnID, "kind": "tool_result",
			"toolName": "task", "callId": callID, "status": "failed", "createdAt": timestamp,
			"finishedAt": timestamp, "output": map[string]any{"status": "different"}, "isError": true,
		}),
	}}}
	input.Thread = conflictThread
	if _, _, err := EnsureRecoveredToolSettlementExact(input); !errors.Is(err, ErrTurnItemIdentityConflict) {
		t.Fatalf("same-id conflict was accepted: %v", err)
	}
	accepted := map[string]any{"id": threadID, "turns": []any{map[string]any{
		"id": turnID, "threadId": threadID, "acceptedFinal": map[string]any{"digest": "fixed"},
		"items": baseTurn["items"],
	}}}
	input.Thread = accepted
	if _, _, err := EnsureRecoveredToolSettlementExact(input); !errors.Is(err, ErrAcceptedFinalImmutable) {
		t.Fatalf("accepted-final mutation was accepted: %v", err)
	}
	generalTerminal := map[string]any{"id": threadID, "turns": []any{map[string]any{
		"id": turnID, "threadId": threadID, "status": "completed",
		"generalTerminalCASBinding":  map[string]any{"bindingDigest": "fixed"},
		"generalTerminalPublication": map[string]any{"commitDigest": "fixed"},
		"items":                      baseTurn["items"],
	}}}
	input.Thread = generalTerminal
	if _, _, err := EnsureRecoveredToolSettlementExact(input); !errors.Is(err, ErrAcceptedFinalImmutable) {
		t.Fatalf("general-terminal mutation was accepted: %v", err)
	}
}

func TestAppendItemToTurnProjectsOrdinaryContentWithoutChangingItemAuthority(t *testing.T) {
	thread, ok := AppendItemToTurn(AppendItemInput{
		Thread: map[string]any{
			"id": "thread_6222020000000000000",
			"turns": []any{map[string]any{
				"id": "turn_6222020000000000000", "items": []any{},
			}},
		},
		TurnID: "turn_6222020000000000000",
		Item: map[string]any{
			"id":      "item_6222020000000000000",
			"kind":    "tool_progress",
			"message": "卡号 6222020000000000000",
			"arguments": map[string]any{
				"outputPreview": "电话 13800138000",
			},
		},
	})
	if !ok {
		t.Fatal("append did not find projected turn")
	}
	item := thread["turns"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)
	if item["id"] != "item_6222020000000000000" || item["message"] != "卡号 [ACCOUNT]" {
		t.Fatalf("ordinary item projection = %#v", item)
	}
	arguments := item["arguments"].(map[string]any)
	if arguments["outputPreview"] != "电话 [PHONE]" {
		t.Fatalf("ordinary item nested projection = %#v", arguments)
	}
}

func TestEnsureUserInputItemPreservesQuestionAuthorityWhileProjectingQuestionText(t *testing.T) {
	thread := map[string]any{"id": "thread", "turns": []any{map[string]any{
		"id": "turn", "items": []any{},
	}}}
	item := map[string]any{
		"id": "item_input", "threadId": "thread", "turnId": "turn",
		"kind": "user_input", "status": "pending", "inputId": "input_123456789012",
		"questions": []map[string]any{{
			"id":       "input_123456789012_1",
			"question": "核对账号 6222020000000000000",
			"options": []map[string]string{{
				"label": "电话 13800138000",
			}},
		}},
	}
	projected, changed, err := EnsureItemToTurnExact(AppendItemInput{
		Thread: thread, TurnID: "turn", Item: item, UpdatedAt: "now",
	})
	if err != nil || !changed {
		t.Fatalf("ensure user-input item changed=%v err=%v", changed, err)
	}
	persisted := projected["turns"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)
	question := persisted["questions"].([]any)[0].(map[string]any)
	if question["id"] != "input_123456789012_1" || question["question"] != "核对账号 [ACCOUNT]" {
		t.Fatalf("persisted question authority or text projection = %#v", question)
	}
	option := question["options"].([]any)[0].(map[string]any)
	if option["label"] != "电话 [PHONE]" {
		t.Fatalf("persisted option PII was not projected: %#v", option)
	}
}

func TestPatchTurnItemStatusClonesThreadAndPatchesItem(t *testing.T) {
	source := map[string]any{
		"turns": []any{
			map[string]any{
				"id": "turn_1",
				"items": []any{
					map[string]any{"id": "item_1", "status": "running"},
					map[string]any{"id": "item_2", "status": "pending"},
				},
			},
		},
	}
	thread, ok := PatchTurnItemStatus(PatchItemStatusInput{
		Thread:     source,
		TurnID:     "turn_1",
		ItemID:     "item_2",
		Status:     "completed",
		FinishedAt: "done",
	})
	if !ok {
		t.Fatal("patch did not find item")
	}
	turns, _ := thread["turns"].([]any)
	turn, _ := turns[0].(map[string]any)
	items, _ := turn["items"].([]any)
	item, _ := items[1].(map[string]any)
	if item["status"] != "completed" || item["finishedAt"] != "done" || thread["updatedAt"] != "done" {
		t.Fatalf("patch mismatch: thread=%#v item=%#v", thread, item)
	}
	sourceTurns, _ := source["turns"].([]any)
	sourceTurn, _ := sourceTurns[0].(map[string]any)
	sourceItems, _ := sourceTurn["items"].([]any)
	sourceItem, _ := sourceItems[1].(map[string]any)
	if sourceItem["status"] != "pending" {
		t.Fatalf("patch should not mutate input: %#v", source)
	}
}

func TestTurnItemMutationsReturnFalseForMissingTargets(t *testing.T) {
	thread := map[string]any{"turns": []any{map[string]any{"id": "turn_1", "items": []any{map[string]any{"id": "item_1"}}}}}
	if _, ok := AppendItemToTurn(AppendItemInput{Thread: thread, TurnID: "missing", Item: map[string]any{"id": "new"}}); ok {
		t.Fatal("append should not find missing turn")
	}
	if _, ok := PatchTurnItemStatus(PatchItemStatusInput{Thread: thread, TurnID: "turn_1", ItemID: "missing"}); ok {
		t.Fatal("patch should not find missing item")
	}
}

func TestPatchTurnItemStatusRejectsAcceptedFinalMutation(t *testing.T) {
	for name, turn := range map[string]map[string]any{
		"turn authority": {
			"id": "turn_1", "acceptedFinal": map[string]any{"recordDigest": "signed"},
			"items": []any{map[string]any{"id": "tool_1", "status": "completed"}},
		},
		"item authority": {
			"id": "turn_1", "items": []any{
				map[string]any{"id": "tool_1", "status": "completed"},
				map[string]any{"id": "final_1", "acceptedFinal": map[string]any{"recordDigest": "signed"}},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			thread := map[string]any{"turns": []any{turn}}
			if err := ValidatePatchTurnItemStatus(thread, "turn_1"); !errors.Is(err, ErrAcceptedFinalImmutable) {
				t.Fatalf("accepted-final mutation was not rejected: %v", err)
			}
		})
	}
	ordinary := map[string]any{"turns": []any{map[string]any{
		"id": "turn_1", "status": "running", "items": []any{map[string]any{"id": "tool_1", "status": "running"}},
	}}}
	if err := ValidatePatchTurnItemStatus(ordinary, "turn_1"); err != nil {
		t.Fatalf("ordinary active turn patch was rejected: %v", err)
	}
}
