package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestMessagesBodyCarriesHostOutputTokenBudget(t *testing.T) {
	body := mustMessagesBody(t, domainmodel.Request{
		Model: "bounded", MaxOutputTokens: 321,
		Messages: []domainmodel.Message{{Role: "user", Content: "bounded"}},
	})
	if body["max_tokens"] != 321 {
		t.Fatalf("messages output token budget missing: %#v", body)
	}
}

func TestMessagesBodyAddsBoundedCacheBreakpoints(t *testing.T) {
	body := mustMessagesBody(t, domainmodel.Request{
		Model: "claude-test",
		Messages: []domainmodel.Message{{
			Role:    "system",
			Content: "stable system prefix",
		}, {
			Role:    "user",
			Content: "first cached history message",
		}, {
			Role:    "assistant",
			Content: "assistant history",
		}, {
			Role:    "user",
			Content: "latest user request",
		}},
		Tools: []domainmodel.ToolSchema{{
			Name:        "read_file",
			Description: "Read a file",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		}},
	})

	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	if count := strings.Count(string(data), `"cache_control"`); count != 2 {
		t.Fatalf("expected system and message cache breakpoints, got %d in %s", count, string(data))
	}
	system := body["system"].([]map[string]any)
	if _, ok := system[len(system)-1]["cache_control"]; !ok {
		t.Fatalf("final system block should carry cache_control: %#v", system)
	}
	tools := body["tools"].([]map[string]any)
	if _, ok := tools[len(tools)-1]["cache_control"]; ok {
		t.Fatalf("tool block should not carry cache_control when system is present: %#v", tools)
	}
	messages := body["messages"].([]map[string]any)
	content := messages[len(messages)-1]["content"].([]map[string]any)
	if _, ok := content[len(content)-1]["cache_control"]; !ok {
		t.Fatalf("final message content block should carry cache_control: %#v", content)
	}
}

func TestMessagesBodyUsesToolCacheBreakpointWhenSystemIsAbsent(t *testing.T) {
	body := mustMessagesBody(t, domainmodel.Request{
		Model: "claude-test",
		Messages: []domainmodel.Message{{
			Role:    "user",
			Content: "latest user request",
		}},
		Tools: []domainmodel.ToolSchema{{
			Name:        "read_file",
			Description: "Read a file",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		}},
	})

	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	if count := strings.Count(string(data), `"cache_control"`); count != 2 {
		t.Fatalf("expected tool and message cache breakpoints, got %d in %s", count, string(data))
	}
	tools := body["tools"].([]map[string]any)
	if _, ok := tools[len(tools)-1]["cache_control"]; !ok {
		t.Fatalf("final tool block should carry cache_control without system: %#v", tools)
	}
	messages := body["messages"].([]map[string]any)
	content := messages[len(messages)-1]["content"].([]map[string]any)
	if _, ok := content[len(content)-1]["cache_control"]; !ok {
		t.Fatalf("final message content block should carry cache_control: %#v", content)
	}
}

func TestMessagesBodyMapsToolResultsToUserMessages(t *testing.T) {
	body := mustMessagesBody(t, domainmodel.Request{
		Model: "claude-test",
		Messages: []domainmodel.Message{{
			Role:    "user",
			Content: "inspect files",
		}, {
			Role: "assistant",
			ToolCalls: []domainmodel.ToolCall{{
				ID:        "toolu_read",
				Name:      "read_file",
				Arguments: json.RawMessage(`{"path":"README.md"}`),
			}},
		}, {
			Role:       "tool",
			ToolCallID: "toolu_read",
			Content:    "file contents",
		}, {
			Role:       "tool",
			ToolCallID: "toolu_grep",
			Content:    "grep contents",
		}, {
			Role:    "user",
			Content: "continue",
		}},
	})

	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	if strings.Contains(string(data), `"role":"tool"`) {
		t.Fatalf("Anthropic messages must not emit top-level role=tool: %s", string(data))
	}
	messages := body["messages"].([]map[string]any)
	if len(messages) != 3 {
		t.Fatalf("expected user, assistant, merged user messages, got %#v", messages)
	}
	if messages[2]["role"] != "user" {
		t.Fatalf("tool_result blocks should live under a user message: %#v", messages[2])
	}
	content := messages[2]["content"].([]map[string]any)
	if content[0]["type"] != "tool_result" || content[1]["type"] != "tool_result" || content[2]["type"] != "text" {
		t.Fatalf("expected adjacent tool results and following user text to merge into user content blocks: %#v", content)
	}
	if content[0]["tool_use_id"] != "toolu_read" || content[1]["tool_use_id"] != "toolu_grep" {
		t.Fatalf("tool_result blocks should preserve tool_use_id: %#v", content)
	}
}

func mustMessagesBody(t *testing.T, request domainmodel.Request) map[string]any {
	t.Helper()
	body, err := MessagesBody(request)
	if err != nil {
		t.Fatalf("build anthropic messages body: %v", err)
	}
	return body
}
