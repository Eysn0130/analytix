package openai

import (
	"encoding/json"
	"strings"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestReasoningProtocolIsSoleDeepSeekWireAuthority(t *testing.T) {
	toolTurn := domainmodel.Message{Role: "assistant", ToolCalls: []domainmodel.ToolCall{{
		ID: "provider-call", Name: "read", Arguments: json.RawMessage(`{"path":"a"}`),
	}}}
	for _, request := range []domainmodel.Request{
		{Family: "deepseek", ProviderID: "deepseek", BaseURL: "https://api.deepseek.com", Messages: []domainmodel.Message{toolTurn}},
		{Family: "openai-compatible", ProviderID: "deepseek-proxy", BaseURL: "https://gateway.example/deepseek", ReasoningProtocol: "none", Messages: []domainmodel.Message{toolTurn}},
	} {
		body, err := ChatCompletionsBody(request)
		if err != nil {
			t.Fatal(err)
		}
		messages := body["messages"].([]map[string]any)
		if _, found := messages[0]["reasoning_content"]; found {
			t.Fatalf("family, provider label, URL, or explicit none enabled DeepSeek replay fields: %#v", messages[0])
		}
		if _, found := body["thinking"]; found {
			t.Fatalf("family, provider label, URL, or explicit none enabled DeepSeek thinking: %#v", body)
		}
	}

	explicit := domainmodel.Request{
		Family: "openai-compatible", ProviderID: "neutral", BaseURL: "https://gateway.example",
		ReasoningProtocol: "deepseek-chat-completions", ReasoningEffort: "high",
		Messages: []domainmodel.Message{{Role: "user", Content: "inspect"}},
	}
	body, err := ChatCompletionsBody(explicit)
	if err != nil {
		t.Fatal(err)
	}
	messages := body["messages"].([]map[string]any)
	if _, found := messages[0]["reasoning_content"]; found ||
		body["reasoning_effort"] != "high" {
		t.Fatalf("explicit DeepSeek protocol did not control the neutral-proxy wire shape: %#v", body)
	}

	explicit.Messages = []domainmodel.Message{toolTurn}
	if _, err := ChatCompletionsBody(explicit); err == nil {
		t.Fatal("DeepSeek thinking tool history without exact private replay must fail closed")
	}
	explicit.ReasoningEffort = "off"
	body, err = ChatCompletionsBody(explicit)
	if err != nil {
		t.Fatal(err)
	}
	messages = body["messages"].([]map[string]any)
	thinking, _ := body["thinking"].(map[string]any)
	if thinking["type"] != "disabled" {
		t.Fatalf("DeepSeek off effort did not disable thinking: %#v", body)
	}
	if _, found := messages[0]["reasoning_content"]; found {
		t.Fatalf("DeepSeek off effort must not add continuation reasoning: %#v", messages[0])
	}
}

func TestChatCompletionsBodyCarriesHostOutputTokenBudget(t *testing.T) {
	body, err := ChatCompletionsBody(domainmodel.Request{Model: "bounded", MaxOutputTokens: 321})
	if err != nil {
		t.Fatal(err)
	}
	if body["max_tokens"] != 321 {
		t.Fatalf("chat completions output token budget missing: %#v", body)
	}
}

func TestDeepSeekChatReasoningEffortWireMapping(t *testing.T) {
	for _, test := range []struct {
		input, expected string
	}{
		{"low", "low"}, {"medium", "high"}, {"high", "high"},
		{"max", "max"},
		{"off", ""}, {"auto", ""},
	} {
		t.Run(test.input, func(t *testing.T) {
			body, err := ChatCompletionsBody(domainmodel.Request{
				Family: "deepseek", Model: "deepseek-chat", ReasoningProtocol: "deepseek-chat-completions",
				ReasoningEffort: test.input,
			})
			if err != nil {
				t.Fatal(err)
			}
			if test.expected == "" {
				if _, present := body["reasoning_effort"]; present {
					t.Fatal("off/auto must not send a reasoning effort")
				}
			} else if body["reasoning_effort"] != test.expected {
				t.Fatalf("reasoning effort %q encoded as %#v, want %q", test.input, body["reasoning_effort"], test.expected)
			}
			if test.input == "off" && body["thinking"].(map[string]any)["type"] != "disabled" {
				t.Fatal("off must explicitly disable DeepSeek thinking")
			}
			if test.input == "auto" && body["thinking"] != nil {
				t.Fatal("auto must preserve the provider default")
			}
		})
	}
}

func TestChatCompletionsBodyReplaysExactDeepSeekToolReasoning(t *testing.T) {
	assistant := domainmodel.Message{Role: "assistant", ToolCalls: []domainmodel.ToolCall{{
		ID: "provider-call", Name: "read", Arguments: json.RawMessage(`{"path":"a"}`),
	}}}
	messages := []domainmodel.Message{
		{Role: "user", Content: "inspect"},
		assistant,
		{Role: "tool", ToolCallID: "provider-call", Content: "result"},
	}
	session, err := domainmodel.NewPrivateProtocolSession()
	if err != nil {
		t.Fatal(err)
	}
	contextDigest := domainsecurity.SHA256Hex([]byte("context"))
	routeHash := domainsecurity.SHA256Hex([]byte("route"))
	manifestHash := domainsecurity.SHA256Hex([]byte("tools"))
	capsule, err := session.IssueAnthropicThinking(domainmodel.AnthropicThinkingCapsuleInput{
		Protocol: "deepseek-chat-completions", ContextDigest: contextDigest,
		ProviderRouteHash: routeHash, PromptRoute: "general", ToolManifestHash: manifestHash,
		Sequence: 2, AssistantMessageIndex: 1, AssistantMessage: assistant,
		PrefixMessages: messages[:1], Thinking: "exact private reasoning",
	})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := session.ConsumeAnthropicThinking(capsule, domainmodel.AnthropicThinkingConsumeInput{
		Protocol: "deepseek-chat-completions", ContextDigest: contextDigest,
		ProviderRouteHash: routeHash, PromptRoute: "general", ToolManifestHash: manifestHash,
		Sequence: 2, Messages: messages,
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := ChatCompletionsBody(domainmodel.Request{
		Model: "deepseek-v4-pro", ReasoningProtocol: "deepseek-chat-completions",
		ReasoningEffort: "high", Messages: messages,
		DeepSeekReasoningReplays: []*domainmodel.AnthropicThinkingReplay{replay},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"reasoning_content":"exact private reasoning"`) ||
		strings.Count(string(encoded), "exact private reasoning") != 1 {
		t.Fatalf("exact process-local reasoning was not bound once to the tool-call message: %s", encoded)
	}
}
