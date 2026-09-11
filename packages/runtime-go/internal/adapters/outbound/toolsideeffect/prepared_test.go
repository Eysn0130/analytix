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
				{"id":"todo_2","content":"second","status":"in_progress"}
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
