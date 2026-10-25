package toolsideeffect

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	appmodel "analytix.local/runtime-go/internal/app/model"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestPrepareTodoWriteOwnerValidationReturnsRecoverableRejection(t *testing.T) {
	pending := appmodel.PendingToolCall{
		Call: domainmodel.ToolCall{
			ID:   "call_1",
			Name: "todo_write",
			Arguments: json.RawMessage(`{"todos":[
				{"id":"todo_1","content":"first","status":"in_progress"},
				{"id":"todo_1","content":"second","status":"in_progress"}
			]}`),
		},
		ExecutionGrant: domainsecurity.ExecutionGrant{ToolName: "todo_write"},
	}

	prepared, err := Prepare(context.Background(), pending, time.Unix(0, 0).UTC(), Dependencies{})
	if err != nil {
		t.Fatalf("provider-correctable todo validation must not fail the turn: %v", err)
	}
	if prepared.Rejection == nil || !prepared.Rejection.IsError {
		t.Fatalf("invalid todo replacement must return a failed tool result: %#v", prepared)
	}
	output, ok := prepared.Rejection.Output.(map[string]any)
	if !ok || output["code"] != "todo_write_failed" {
		t.Fatalf("unexpected todo rejection: %#v", prepared.Rejection.Output)
	}
}

func TestPrepareTodoWriteAdmitsParallelOwnerStateWithStableSemanticIdentity(t *testing.T) {
	pending := appmodel.PendingToolCall{Call: domainmodel.ToolCall{ID: "call_parallel", Name: "todo_write"},
		ExecutionGrant: domainsecurity.ExecutionGrant{ToolName: "todo_write"}}
	prepare := func(content string, secondStatus string) string {
		t.Helper()
		args, err := json.Marshal(map[string]any{"todos": []any{
			map[string]any{"id": "first", "content": content, "status": "in_progress"},
			map[string]any{"id": "second", "content": "Second", "status": secondStatus},
		}})
		if err != nil {
			t.Fatal(err)
		}
		pending.Call.Arguments = args
		prepared, err := Prepare(context.Background(), pending, time.Unix(0, 0).UTC(), Dependencies{})
		if err != nil || prepared.Rejection != nil || prepared.SemanticIdentity.ArgsHash == "" {
			t.Fatalf("parallel request was not admitted: %#v err=%v", prepared, err)
		}
		return prepared.SemanticIdentity.ArgsHash
	}
	parallel := prepare("  First  ", "in_progress")
	if parallel != prepare("First", "in_progress") {
		t.Fatal("normalization differs across semantic admission")
	}
	if parallel == prepare("First", "pending") {
		t.Fatal("true active state vanished from semantic identity")
	}
}
