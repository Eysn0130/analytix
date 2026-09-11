package schema

import (
	"bytes"
	"encoding/json"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestNormalizeRequestReasoningEffortUsesExactClosedSet(t *testing.T) {
	for _, value := range []string{"", "auto", "off", "low", "medium", "high", "max"} {
		if got := NormalizeRequestReasoningEffort(value); got != value {
			t.Fatalf("exact effort projection mismatch: got=%q want=%q", got, value)
		}
	}
	for _, value := range []string{" high ", "HIGH", "adaptive", "disabled", "none", "false", "minimal", "mid", "maximum", "xhigh"} {
		if got := NormalizeRequestReasoningEffort(value); got != "" {
			t.Fatalf("effort alias must not be upgraded: raw=%q got=%q", value, got)
		}
	}
}

func TestOpenAIChatMessagesNeverCopiesGenericPrivateReasoning(t *testing.T) {
	messages := []domainmodel.Message{{
		Role:    "assistant",
		Content: "plain final",
		Parts: []domainmodel.MessagePart{{
			Type: "reasoning",
			Text: "ordinary private reasoning",
		}},
	}, {
		Role: "assistant",
		Parts: []domainmodel.MessagePart{{
			Type: "reasoning",
			Text: "need the tool",
		}},
		ToolCalls: []domainmodel.ToolCall{{
			ID:        "call_read",
			Name:      "read_file",
			Arguments: json.RawMessage(`{"path":"note.txt"}`),
		}},
	}}

	body, err := OpenAIChatMessages(messages, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := body[0]["reasoning_content"]; ok {
		t.Fatalf("ordinary assistant reasoning must not be re-uploaded: %#v", body[0])
	}
	if _, ok := body[1]["reasoning_content"]; ok {
		t.Fatalf("tool_calls reasoning requires an exact process-local replay: %#v", body[1])
	}
	encoded, _ := json.Marshal(body)
	if string(encoded) == "" || json.Valid(encoded) == false || bytes.Contains(encoded, []byte("need the tool")) || bytes.Contains(encoded, []byte("ordinary private reasoning")) {
		t.Fatalf("private reasoning reached DeepSeek continuation: %s", encoded)
	}

	withoutReasoning, err := OpenAIChatMessages(messages[1:], nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := withoutReasoning[0]["reasoning_content"]; ok {
		t.Fatalf("non-DeepSeek request should not include reasoning_content: %#v", withoutReasoning[0])
	}
}

func TestRequestUsesDeepSeekReasoningRequiresEnabledEffort(t *testing.T) {
	for _, test := range []struct {
		name   string
		effort string
		want   bool
	}{
		{name: "high", effort: "high", want: true},
		{name: "max", effort: "max", want: true},
		{name: "auto", effort: "auto", want: true},
		{name: "off", effort: "off"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := RequestUsesDeepSeekReasoning(domainmodel.Request{
				ReasoningProtocol: "deepseek-chat-completions",
				ReasoningEffort:   test.effort,
			})
			if got != test.want {
				t.Fatalf("DeepSeek reasoning activation mismatch: got=%t want=%t", got, test.want)
			}
		})
	}
	if RequestUsesDeepSeekReasoning(domainmodel.Request{
		ReasoningProtocol: "none", ReasoningEffort: "high",
	}) {
		t.Fatal("non-DeepSeek protocol enabled DeepSeek reasoning")
	}
}

func TestAnthropicToolsAddsABoundedCacheControlBreakpoint(t *testing.T) {
	tools := AnthropicTools([]domainmodel.ToolSchema{{
		Name:        "alpha",
		Description: "first",
	}, {
		Name:        "beta",
		Description: "second",
	}, {
		Name:        "gamma",
		Description: "third",
	}}, true)

	marked := 0
	for index, tool := range tools {
		if _, ok := tool["cache_control"]; ok {
			marked++
			if index != len(tools)-1 {
				t.Fatalf("only the final tool schema should carry cache_control: %#v", tools)
			}
		}
	}
	if marked != 1 {
		t.Fatalf("expected exactly one cache_control breakpoint, got %d in %#v", marked, tools)
	}
}

func TestAnthropicToolsDoesNotAddCacheControlUnlessRequested(t *testing.T) {
	tools := AnthropicTools([]domainmodel.ToolSchema{{
		Name:        "alpha",
		Description: "first",
	}}, false)
	if _, ok := tools[0]["cache_control"]; ok {
		t.Fatalf("Anthropic tool schema should only carry cache_control when requested: %#v", tools)
	}
}

func TestProviderToolsUseCanonicalOrderForCacheableRequestBodies(t *testing.T) {
	left := []domainmodel.ToolSchema{{
		Name:        "zeta",
		Description: "Z tool",
		Parameters:  json.RawMessage(`{"type":"object","required":["z","a","z"],"properties":{"z":{"type":"string"},"a":{"type":"number"}}}`),
	}, {
		Name:        "alpha",
		Description: "A tool",
		Parameters:  json.RawMessage(`{"properties":{"q":{"type":["string","null"]}},"type":"object"}`),
	}}
	right := []domainmodel.ToolSchema{{
		Name:        "alpha",
		Description: "A tool",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"q":{"type":["null","string"]}}}`),
	}, {
		Name:        "zeta",
		Description: "Z tool",
		Parameters:  json.RawMessage(`{"properties":{"a":{"type":"number"},"z":{"type":"string"}},"required":["a","z"],"type":"object"}`),
	}}

	if got, want := mustJSON(t, OpenAITools(left)), mustJSON(t, OpenAITools(right)); got != want {
		t.Fatalf("OpenAI provider tools should be stable across input order/schema key order:\n got: %s\nwant: %s", got, want)
	}
	if got, want := mustJSON(t, AnthropicTools(left, true)), mustJSON(t, AnthropicTools(right, true)); got != want {
		t.Fatalf("Anthropic provider tools should be stable across input order/schema key order:\n got: %s\nwant: %s", got, want)
	}

	anthropicTools := AnthropicTools(left, true)
	if anthropicTools[len(anthropicTools)-1]["name"] != "zeta" {
		t.Fatalf("Anthropic cache breakpoint should land on final canonical tool, got %#v", anthropicTools)
	}
	if _, ok := anthropicTools[len(anthropicTools)-1]["cache_control"]; !ok {
		t.Fatalf("final canonical Anthropic tool should carry cache_control: %#v", anthropicTools)
	}
}

func TestProviderEmptySchemaFallbackIsClosed(t *testing.T) {
	tools := []domainmodel.ToolSchema{{Name: "empty", Description: "empty fallback"}}
	assertClosed := func(t *testing.T, raw any) {
		t.Helper()
		encoded, err := json.Marshal(raw)
		if err != nil {
			t.Fatal(err)
		}
		var definition map[string]any
		if err := json.Unmarshal(encoded, &definition); err != nil {
			t.Fatal(err)
		}
		if definition["type"] != "object" || definition["additionalProperties"] != false {
			t.Fatalf("provider emitted an open empty-schema fallback: %s", encoded)
		}
		properties, ok := definition["properties"].(map[string]any)
		if !ok || len(properties) != 0 {
			t.Fatalf("provider empty-schema fallback was not explicit and deterministic: %s", encoded)
		}
	}

	openAI := OpenAITools(tools)
	function := openAI[0]["function"].(map[string]any)
	assertClosed(t, function["parameters"])
	anthropic := AnthropicTools(tools, false)
	assertClosed(t, anthropic[0]["input_schema"])
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal provider value: %v", err)
	}
	return string(data)
}
