package server

import (
	"testing"

	appmodel "analytix.local/runtime-go/internal/app/model"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
)

func TestProviderHistoryFromThreadDropsDuplicateAndMissingLegacyToolIdentities(t *testing.T) {
	thread := map[string]any{
		"turns": []any{
			map[string]any{
				"items": []any{
					map[string]any{"kind": "user_message", "text": "inspect files"},
					map[string]any{"kind": "tool_call", "callId": "dup", "toolName": "read_file", "arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1()},
					map[string]any{"kind": "tool_result", "callId": "dup", "toolName": "read_file", "output": "FILE-RESULT"},
					map[string]any{"kind": "tool_call", "callId": "dup", "toolName": "grep", "arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1()},
					map[string]any{"kind": "tool_result", "callId": "dup", "toolName": "grep", "output": "GREP-RESULT"},
					map[string]any{"kind": "tool_call", "toolName": "search", "arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1()},
					map[string]any{"kind": "tool_result", "toolName": "search", "output": "SEARCH-RESULT"},
				},
			},
		},
	}

	messages := appmodel.ProviderHistoryFromThread(thread)
	if len(messages) != 1 || messages[0].Role != "user" || messages[0].Content != "inspect files" {
		t.Fatalf("unsafe legacy tool identities were repaired into provider history: %#v", messages)
	}
}
