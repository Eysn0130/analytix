package loop

import (
	"context"
	"errors"
	"reflect"
	"testing"

	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestPersistReadOnlyBatchResultsAttemptsEverySettlementAndJoinsErrors(t *testing.T) {
	firstErr := errors.New("first settlement failed")
	thirdErr := errors.New("third settlement failed")
	results := []toolcatalogapp.BatchResult[appmodel.PendingToolCall]{
		{Call: appmodel.PendingToolCall{Call: domainmodel.ToolCall{ID: "call-1", Name: "read"}}, Output: "one"},
		{Call: appmodel.PendingToolCall{Call: domainmodel.ToolCall{ID: "call-2", Name: "grep"}}, Output: "two"},
		{Call: appmodel.PendingToolCall{Call: domainmodel.ToolCall{ID: "call-3", Name: "find"}}, Output: "three"},
	}
	attempted := []string{}
	messages, err := PersistReadOnlyBatchResults(context.Background(), results,
		func(_ context.Context, pending appmodel.PendingToolCall, _ any, _ bool) (domainmodel.Message, error) {
			attempted = append(attempted, pending.Call.ID)
			switch pending.Call.ID {
			case "call-1":
				return domainmodel.Message{}, firstErr
			case "call-3":
				return domainmodel.Message{}, thirdErr
			default:
				return domainmodel.Message{Role: "tool", ToolCallID: pending.Call.ID}, nil
			}
		},
	)
	if !reflect.DeepEqual(attempted, []string{"call-1", "call-2", "call-3"}) {
		t.Fatalf("settlement attempts stopped early: %#v", attempted)
	}
	if !errors.Is(err, firstErr) || !errors.Is(err, thirdErr) {
		t.Fatalf("settlement errors were not joined: %v", err)
	}
	if len(messages) != 1 || messages[0].ToolCallID != "call-2" {
		t.Fatalf("successful settlements were not preserved: messages=%#v", messages)
	}
}
