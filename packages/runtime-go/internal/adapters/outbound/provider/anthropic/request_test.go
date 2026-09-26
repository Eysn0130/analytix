package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestDeepSeekMessagesBaselineReasoningAndExtensionFallback(t *testing.T) {
	for effort, want := range map[string]string{"low": "low", "medium": "high", "high": "high", "max": "max", "off": "disabled", "auto": "", "": ""} {
		t.Run(effort, func(t *testing.T) {
			body := mustMessagesBody(t, domainmodel.Request{
				Model: "deepseek-flash", ReasoningProtocol: "deepseek-messages", ReasoningEffort: effort, MaxOutputTokens: 8192,
				Messages: []domainmodel.Message{{Role: "system", Content: "current system"}, {Role: "user", Content: "current input"}},
				Tools:    []domainmodel.ToolSchema{{Name: "read", Parameters: json.RawMessage(`{"type":"object"}`)}},
			})
			thinking, _ := body["thinking"].(map[string]any)
			switch want {
			case "":
				if thinking != nil || body["output_config"] != nil {
					t.Fatal("auto must leave the provider default intact")
				}
			case "disabled":
				if thinking["type"] != "disabled" || body["output_config"] != nil {
					t.Fatal("off did not disable thinking")
				}
			default:
				if thinking["type"] != "enabled" || body["output_config"].(map[string]any)["effort"] != want {
					t.Fatal("DeepSeek effort encoding mismatch")
				}
			}
			encoded, _ := json.Marshal(body)
			for _, extension := range []string{"cache_control", "tool_update", "system_update", "adaptive", "user_id", "session_log"} {
				if strings.Contains(string(encoded), extension) {
					t.Fatalf("unconfirmed/default-off extension %s was sent", extension)
				}
			}
			if body["max_tokens"] != 8192 || body["system"].([]map[string]any)[0]["text"] != "current system" {
				t.Fatal("full baseline was not rebuilt")
			}
		})
	}
}

func TestMessagesBodyCarriesHostOutputTokenBudget(t *testing.T) {
	body := mustMessagesBody(t, domainmodel.Request{
		Model: "bounded", MaxOutputTokens: 321,
		Messages: []domainmodel.Message{{Role: "user", Content: "bounded"}},
	})
	if body["max_tokens"] != 321 {
		t.Fatalf("messages output token budget missing: %#v", body)
	}
}

func TestMessagesBodyPreservesLargeHostOutputTokenBudget(t *testing.T) {
	body := mustMessagesBody(t, domainmodel.Request{
		Model: "bounded", MaxOutputTokens: 8192,
		Messages: []domainmodel.Message{{Role: "user", Content: "bounded"}},
	})
	if body["max_tokens"] != 8192 {
		t.Fatalf("messages output token budget was reduced: %#v", body)
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

func TestDeepSeekMessagesRejectsAmbiguousToolHistoryAndRebuildsChangedCapabilities(t *testing.T) {
	good := []domainmodel.Message{{Role: "user", Content: "read"}, {Role: "assistant", ToolCalls: []domainmodel.ToolCall{{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{"n":9007199254740993}`)}}}, {Role: "tool", ToolCallID: "call-1", Content: `{"ok":false,"code":"access_denied"}`}, {Role: "user", Content: "Explain the failure"}}
	request := domainmodel.Request{Model: "deepseek-flash", ReasoningProtocol: "deepseek-messages", Messages: good}
	body := mustMessagesBody(t, request)
	encoded, _ := json.Marshal(body)
	if !strings.Contains(string(encoded), "9007199254740993") || !strings.Contains(string(encoded), "access_denied") {
		t.Fatal("numeric tool input or failure semantics lost")
	}
	for _, args := range []string{`null`, `[]`, `{"x":1,"x":2}`, `{"x":`} {
		bad := append([]domainmodel.Message(nil), good...)
		bad[1].ToolCalls = []domainmodel.ToolCall{{ID: "call-1", Name: "read", Arguments: json.RawMessage(args)}}
		request.Messages = bad
		if _, err := MessagesBody(request); err == nil {
			t.Fatal("ambiguous input accepted", args)
		}
	}
	for _, bad := range [][]domainmodel.Message{good[:2], {good[2]}, {good[0], {Role: "system", Content: "history update"}}, {{Role: "developer", Content: "unsupported role"}}} {
		request.Messages = bad
		if _, err := MessagesBody(request); err == nil {
			t.Fatal("invalid role or tool pairing accepted")
		}
	}
	request.Messages = []domainmodel.Message{{Role: "system", Content: "current snapshot"}, {Role: "user", Content: "first"}}
	request.Tools = []domainmodel.ToolSchema{{Name: "old", Parameters: json.RawMessage(`{"type":"object"}`)}}
	mustMessagesBody(t, request)
	request.Messages = []domainmodel.Message{{Role: "user", Content: "corrected current input"}}
	request.Tools = nil
	body = mustMessagesBody(t, request)
	if body["system"] != nil || body["tools"] != nil {
		t.Fatal("cleared system or removed tool resurrected")
	}
}
