package thread

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestNormalizeTodoOpsForSemanticIdentityMatchesExecutionDefaultsPrivacyAndSource(t *testing.T) {
	phoneA := "13800138000"
	phoneB := "13900139000"
	first, err := NormalizeTodoOpsForSemanticIdentityV1([]any{map[string]any{
		"op": "append", "content": "contact " + phoneA, "note": "call " + phoneA,
		"source": map[string]any{"kind": "manual"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NormalizeTodoOpsForSemanticIdentityV1([]any{map[string]any{
		"op": "append", "content": "contact " + phoneB, "note": "call " + phoneB,
		"status": "pending", "source": map[string]any{"kind": "manual"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	firstBody, _ := json.Marshal(first)
	secondBody, _ := json.Marshal(second)
	if string(firstBody) != string(secondBody) {
		t.Fatalf("execution-equivalent todo ops diverged: first=%s second=%s", firstBody, secondBody)
	}
	changedSource, err := NormalizeTodoOpsForSemanticIdentityV1([]any{map[string]any{
		"op": "append", "content": "contact " + phoneA, "note": "call " + phoneA,
		"source": map[string]any{"kind": "child", "childRunId": "run-1"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	changedBody, _ := json.Marshal(changedSource)
	if string(changedBody) == string(firstBody) {
		t.Fatal("different todo provenance collapsed to one semantic operation")
	}
}

func TestNormalizeTodoOpsForSemanticIdentityAcceptsProviderReachableAppendWithoutSource(t *testing.T) {
	normalized, err := NormalizeTodoOpsForSemanticIdentityV1([]any{map[string]any{
		"op": "append", "content": "inspect evidence",
	}})
	if err != nil {
		t.Fatal(err)
	}
	op := normalized[0].(map[string]any)
	if op["status"] != "pending" || op["content"] != "inspect evidence" {
		t.Fatalf("provider-reachable append defaults = %#v", op)
	}
	if _, exists := op["source"]; exists {
		t.Fatalf("absent provider source became synthetic provenance: %#v", op)
	}
	if _, _, err := ApplyTodoOps(nil, []any{map[string]any{
		"op": "append", "content": "inspect evidence",
	}}); err != nil {
		t.Fatalf("execution rejected provider-reachable append: %v", err)
	}
}

func TestNormalizeTodos(t *testing.T) {
	todos, err := NormalizeTodos("thr_1", []any{
		map[string]any{"content": "  first  ", "status": "in_progress", "note": "  keep this  ", "source": map[string]any{"kind": "plan"}},
		map[string]any{"content": "second", "status": "pending"},
		map[string]any{"content": "done", "status": "completed", "createdAt": "earlier"},
	}, "now")
	if err != nil {
		t.Fatalf("normalize todos: %v", err)
	}
	if todos["threadId"] != "thr_1" || todos["updatedAt"] != "now" {
		t.Fatalf("unexpected todo envelope: %#v", todos)
	}
	items, _ := todos["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("expected three todos, got %#v", items)
	}
	first, _ := items[0].(map[string]any)
	second, _ := items[1].(map[string]any)
	third, _ := items[2].(map[string]any)
	if first["id"] != "todo_1" || first["status"] != "in_progress" || first["content"] != "first" {
		t.Fatalf("first todo mismatch: %#v", first)
	}
	if first["note"] != "keep this" {
		t.Fatalf("note should be preserved and trimmed: %#v", first)
	}
	if second["status"] != "pending" {
		t.Fatalf("second todo mismatch: %#v", second)
	}
	if third["createdAt"] != "earlier" {
		t.Fatalf("createdAt should be preserved: %#v", third)
	}
	if got := IncompleteTodoCount(todos); got != 2 {
		t.Fatalf("incomplete count = %d", got)
	}
}

func TestNormalizeTodosRejectsMalformedItemsWithoutCoercion(t *testing.T) {
	for _, test := range []struct {
		name  string
		items []any
	}{
		{name: "non-object", items: []any{"todo"}},
		{name: "blank", items: []any{map[string]any{"content": " ", "status": "pending"}}},
		{name: "unknown property", items: []any{map[string]any{"content": "one", "status": "pending", "private": true}}},
		{name: "duplicate id", items: []any{
			map[string]any{"id": "one", "content": "one", "status": "pending"},
			map[string]any{"id": "one", "content": "two", "status": "pending"},
		}},
		{name: "multiple active", items: []any{
			map[string]any{"content": "one", "status": "in_progress"},
			map[string]any{"content": "two", "status": "in_progress"},
		}},
		{name: "failed without reason", items: []any{map[string]any{"content": "one", "status": "failed"}}},
		{name: "canceled with failure reason", items: []any{map[string]any{"content": "one", "status": "canceled", "statusReasonCode": "tool_failed"}}},
		{name: "pending with reason", items: []any{map[string]any{"content": "one", "status": "pending", "statusReasonCode": "tool_failed"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NormalizeTodos("thr_1", test.items, "now"); err == nil {
				t.Fatalf("malformed todos passed: %#v", test.items)
			}
		})
	}
}

func TestNormalizeTodosPreservesTerminalReasonCodes(t *testing.T) {
	todos, err := NormalizeTodos("thr_1", []any{
		map[string]any{"id": "failed", "content": "failed", "status": "failed", "statusReasonCode": "tool_failed"},
		map[string]any{"id": "canceled", "content": "canceled", "status": "canceled", "statusReasonCode": "user_canceled"},
	}, "now")
	if err != nil {
		t.Fatal(err)
	}
	items := todos["items"].([]any)
	if items[0].(map[string]any)["statusReasonCode"] != "tool_failed" || items[1].(map[string]any)["statusReasonCode"] != "user_canceled" {
		t.Fatalf("terminal reason codes were not preserved: %#v", items)
	}
	if got := IncompleteTodoCount(todos); got != 2 {
		t.Fatalf("failed/canceled todos must remain unfinished, got %d", got)
	}
}

func TestNormalizeTodosRejectsInvalidStatus(t *testing.T) {
	_, err := NormalizeTodos("thr_1", []any{
		map[string]any{"content": "bad", "status": "blocked"},
	}, "now")
	if err == nil {
		t.Fatal("expected invalid status error")
	}
}

func TestNormalizeTodosPreservesAcceptedSourcesAndRejectsUnknownSourceFields(t *testing.T) {
	todos, err := NormalizeTodos("thr_1", []any{
		map[string]any{"content": "manual", "status": "pending", "source": map[string]any{"kind": "manual"}},
		map[string]any{"content": "plan", "status": "pending", "source": map[string]any{
			"kind": "plan", "planId": "plan_1", "relativePath": "plan.md", "ordinal": float64(0), "contentHash": "hash_1",
		}},
		map[string]any{"content": "child", "status": "completed", "source": map[string]any{
			"kind": "child", "childRunId": "run_1", "projectionId": "projection_1",
		}},
	}, "now")
	if err != nil {
		t.Fatalf("normalize accepted sources: %v", err)
	}
	items := todos["items"].([]any)
	if items[0].(map[string]any)["source"].(map[string]any)["kind"] != "manual" ||
		items[1].(map[string]any)["source"].(map[string]any)["planId"] != "plan_1" ||
		items[2].(map[string]any)["source"].(map[string]any)["childRunId"] != "run_1" {
		t.Fatalf("accepted sources were not preserved: %#v", items)
	}
	for _, source := range []any{
		"manual",
		map[string]any{"kind": "manual", "privateField": "must-not-pass"},
		map[string]any{"kind": "plan", "ordinal": float64(0.5)},
		map[string]any{"kind": "child", "jobId": ""},
	} {
		if _, err := NormalizeTodos("thr_1", []any{
			map[string]any{"content": "invalid", "status": "pending", "source": source},
		}, "now"); err == nil {
			t.Fatalf("invalid source was accepted: %#v", source)
		}
	}
}

func TestTodoContentAndNotesUseOrdinaryPrivacyProjection(t *testing.T) {
	todos, err := NormalizeTodos("thread_6222020000000000000", []any{
		map[string]any{
			"id":      "todo_6222020000000000000",
			"content": "核对账号 6222020000000000000",
			"note":    "电话 13800138000",
			"status":  "pending",
		},
	}, "now")
	if err != nil {
		t.Fatalf("normalize private todo: %v", err)
	}
	item := todos["items"].([]any)[0].(map[string]any)
	if item["content"] != "核对账号 [ACCOUNT]" || item["note"] != "电话 [PHONE]" {
		t.Fatalf("todo display projection = %#v", item)
	}
	if item["id"] != "todo_6222020000000000000" || todos["threadId"] != "thread_6222020000000000000" {
		t.Fatalf("todo authority was mutated: %#v", todos)
	}
	next, _, err := ApplyTodoOps(todos, []any{map[string]any{"op": "note", "content": "核对账号 6222020000000000000", "note": "邮箱 analyst@example.com"}})
	if err != nil {
		t.Fatalf("match projected todo content: %v", err)
	}
	if next[0].(map[string]any)["note"] != "邮箱 [EMAIL]" {
		t.Fatalf("todo mutation projection = %#v", next)
	}
}

func TestNormalizeTodosRejectsTooManyItems(t *testing.T) {
	items := make([]any, 0, MaxTodoItems+1)
	for index := 0; index < MaxTodoItems+1; index++ {
		items = append(items, map[string]any{"content": "todo", "status": "pending"})
	}
	_, err := NormalizeTodos("thr_1", items, "now")
	if err == nil {
		t.Fatal("expected too many todos error")
	}
}

func TestApplyTodoOpsAppliesOrderedOperations(t *testing.T) {
	nextItems, operations, err := ApplyTodoOps(map[string]any{
		"items": []any{
			map[string]any{"id": "one", "content": "One", "status": "in_progress"},
			map[string]any{"id": "two", "content": "Two", "status": "pending"},
		},
	}, []any{
		map[string]any{"op": "append", "id": "three", "content": "Three", "source": map[string]any{"kind": "child", "jobId": "job_1"}},
		map[string]any{"op": "start", "id": "two"},
		map[string]any{"op": "note", "id": "two", "note": "  working  "},
		map[string]any{"op": "done", "id": "two"},
		map[string]any{"op": "drop", "id": "one"},
	})
	if err != nil {
		t.Fatalf("apply todo ops: %v", err)
	}
	if len(operations) != 5 {
		t.Fatalf("operation count mismatch: %#v", operations)
	}
	if len(nextItems) != 2 {
		t.Fatalf("expected two remaining todos, got %#v", nextItems)
	}
	first := nextItems[0].(map[string]any)
	second := nextItems[1].(map[string]any)
	if first["id"] != "two" || first["status"] != "completed" || first["note"] != "working" {
		t.Fatalf("first todo mismatch: %#v", first)
	}
	if second["id"] != "three" || second["status"] != "in_progress" {
		t.Fatalf("second todo should be promoted after active completion/drop: %#v", second)
	}
	source, _ := second["source"].(map[string]any)
	if source["kind"] != "child" || source["jobId"] != "job_1" {
		t.Fatalf("child source should be preserved: %#v", second)
	}
}

func TestApplyTodoOpsRejectsAmbiguousContentMatch(t *testing.T) {
	_, _, err := ApplyTodoOps(map[string]any{
		"items": []any{
			map[string]any{"id": "one", "content": "Same", "status": "pending"},
			map[string]any{"id": "two", "content": "Same", "status": "pending"},
		},
	}, []any{map[string]any{"op": "start", "content": "Same"}})
	if err == nil {
		t.Fatal("expected ambiguous content error")
	}
}

func TestPrepareTodoOpsV1ResolvesIDAndContentSelectorsToSameDurableOperation(t *testing.T) {
	todos := map[string]any{"items": []any{
		map[string]any{"id": "todo_1", "content": "Inspect evidence", "status": "pending"},
	}}
	byID, err := PrepareTodoOpsV1(todos, []any{map[string]any{"op": "start", "id": "todo_1"}})
	if err != nil {
		t.Fatal(err)
	}
	byContent, err := PrepareTodoOpsV1(todos, []any{map[string]any{"op": "start", "content": "Inspect evidence"}})
	if err != nil {
		t.Fatal(err)
	}
	idBody, _ := json.Marshal(byID.Operations)
	contentBody, _ := json.Marshal(byContent.Operations)
	if string(idBody) != string(contentBody) {
		t.Fatalf("selector aliases resolved differently: byID=%s byContent=%s", idBody, contentBody)
	}
	if byContent.Operations[0].(map[string]any)["target"] != "todo:todo_1" {
		t.Fatalf("durable todo id missing from prepared operation: %#v", byContent.Operations)
	}
}

func TestPrepareTodoOpsV1UsesStableSemanticReferenceForGeneratedAppendID(t *testing.T) {
	first, err := PrepareTodoOpsV1(nil, []any{map[string]any{"op": "append", "content": "Next"}})
	if err != nil {
		t.Fatal(err)
	}
	afterFirst := map[string]any{"items": first.NextItems}
	retry, err := PrepareTodoOpsV1(afterFirst, []any{map[string]any{"op": "append", "content": "Next"}})
	if err != nil {
		t.Fatal(err)
	}
	firstBody, _ := json.Marshal(first.Operations)
	retryBody, _ := json.Marshal(retry.Operations)
	if string(firstBody) != string(retryBody) {
		t.Fatalf("generated durable IDs leaked into semantic identity: first=%s retry=%s", firstBody, retryBody)
	}
}

func TestApplyTodoOpsRejectsInvalidSourceKind(t *testing.T) {
	_, _, err := ApplyTodoOps(nil, []any{
		map[string]any{"op": "append", "content": "bad source", "source": map[string]any{"kind": "remote"}},
	})
	if err == nil {
		t.Fatal("expected invalid source kind error")
	}
}

func TestTodoFailCancelRetryLifecycleIsExplicitAndAudited(t *testing.T) {
	todos := map[string]any{"items": []any{
		map[string]any{"id": "active", "content": "Run tool", "status": "in_progress"},
		map[string]any{"id": "queued", "content": "Wait", "status": "pending"},
	}}
	failed, applied, err := ApplyTodoOps(todos, []any{
		map[string]any{"op": "fail", "id": "active", "statusReasonCode": "tool_failed"},
	})
	if err != nil {
		t.Fatal(err)
	}
	failedItem := failed[0].(map[string]any)
	if failedItem["status"] != "failed" || failedItem["statusReasonCode"] != "tool_failed" || failed[1].(map[string]any)["status"] != "in_progress" {
		t.Fatalf("failure transition mismatch: %#v", failed)
	}
	if applied[0].(map[string]any)["previousStatus"] != "in_progress" || applied[0].(map[string]any)["nextStatus"] != "failed" {
		t.Fatalf("failure audit mismatch: %#v", applied)
	}
	retried, retryAudit, err := ApplyTodoOps(map[string]any{"items": failed}, []any{
		map[string]any{"op": "retry", "id": "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	retriedItem := retried[0].(map[string]any)
	if retriedItem["status"] != "in_progress" {
		t.Fatalf("retry did not reactivate todo: %#v", retried)
	}
	if _, exists := retriedItem["statusReasonCode"]; exists {
		t.Fatalf("retry retained stale reason: %#v", retriedItem)
	}
	if retryAudit[0].(map[string]any)["previousStatus"] != "failed" || retryAudit[0].(map[string]any)["nextStatus"] != "in_progress" {
		t.Fatalf("retry audit mismatch: %#v", retryAudit)
	}
	canceled, _, err := ApplyTodoOps(map[string]any{"items": retried}, []any{
		map[string]any{"op": "cancel", "id": "active", "statusReasonCode": "user_canceled"},
	})
	if err != nil || canceled[0].(map[string]any)["status"] != "canceled" {
		t.Fatalf("cancel transition mismatch: todos=%#v err=%v", canceled, err)
	}
}

func TestTodoTerminalLifecycleRejectsBypassesAndDeletion(t *testing.T) {
	failed := map[string]any{"items": []any{
		map[string]any{"id": "failed", "content": "Run tool", "status": "failed", "statusReasonCode": "tool_failed"},
	}}
	for _, op := range []map[string]any{
		{"op": "start", "id": "failed"},
		{"op": "done", "id": "failed"},
		{"op": "drop", "id": "failed"},
		{"op": "retry", "id": "failed", "statusReasonCode": "tool_failed"},
	} {
		if _, _, err := ApplyTodoOps(failed, []any{op}); err == nil {
			t.Fatalf("terminal bypass passed: %#v", op)
		}
	}
	if _, _, err := ApplyTodoOps(map[string]any{"items": []any{
		map[string]any{"id": "pending", "content": "Wait", "status": "pending"},
	}}, []any{map[string]any{"op": "cancel", "id": "pending", "statusReasonCode": "tool_failed"}}); err == nil {
		t.Fatal("cancel accepted a failure reason code")
	}
}

func TestValidateTodoReplacementRequiresExplicitTerminalOperations(t *testing.T) {
	current, err := NormalizeTodos("thr_1", []any{
		map[string]any{"id": "one", "content": "One", "status": "in_progress"},
	}, "before")
	if err != nil {
		t.Fatal(err)
	}
	failed, err := NormalizeTodos("thr_1", []any{
		map[string]any{"id": "one", "content": "One", "status": "failed", "statusReasonCode": "tool_failed"},
	}, "after")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTodoReplacement(current, failed); err == nil {
		t.Fatal("replacement bypassed explicit fail operation")
	}
	terminal := failed
	retried, err := NormalizeTodos("thr_1", []any{
		map[string]any{"id": "one", "content": "One", "status": "in_progress"},
	}, "retry")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTodoReplacement(terminal, retried); err == nil {
		t.Fatal("replacement bypassed explicit retry operation")
	}
	empty, err := NormalizeTodos("thr_1", nil, "empty")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTodoReplacement(terminal, empty); !errors.Is(err, ErrTodoTerminalAuditRetention) {
		t.Fatalf("terminal deletion error = %v", err)
	}
}
