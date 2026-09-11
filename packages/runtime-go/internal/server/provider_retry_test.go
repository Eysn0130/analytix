package server

import (
	"net/http"
	"strings"
	"testing"
	"time"

	apploop "analytix.local/runtime-go/internal/app/loop"
	appmodel "analytix.local/runtime-go/internal/app/model"
	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	"analytix.local/runtime-go/internal/provider"
)

func TestProviderRetryDelayUsesRetryAfterDiagnostics(t *testing.T) {
	delay := apploop.ProviderRetryDelay(&provider.ProviderError{
		Status:       http.StatusTooManyRequests,
		Kind:         "rate_limit",
		RetryAfterMs: 1500,
	})
	if delay != 1500*time.Millisecond {
		t.Fatalf("expected retry-after delay, got %s", delay)
	}
	if apploop.ProviderRetryDelay(&provider.ProviderError{Status: http.StatusTooManyRequests, Kind: "rate_limit"}) != 0 {
		t.Fatal("missing retry-after should not add delay")
	}
}

func TestAnthropicPrivateProtocolRestartRecompilesSafeSemanticContinuation(t *testing.T) {
	call := provider.ToolCall{ID: "toolu_restart", Name: "read"}
	messages, session, capsules, err := runtimePrivateProtocolContinuationState(runtimePendingToolCall{
		ProviderConfig:          provider.TurnConfig{Family: "anthropic-compatible", EndpointFormat: "messages"},
		PrivateProtocolObserved: true,
	}, []provider.Message{
		{Role: "assistant", ToolCalls: []provider.ToolCall{call}},
		{Role: "tool", ToolCallID: call.ID, Content: "result"},
	})
	if err != nil {
		t.Fatalf("recompile restart continuation: %v", err)
	}
	if session != nil || len(capsules) != 0 || len(messages) != 1 ||
		messages[0].Role != "user" || len(messages[0].ToolCalls) != 0 {
		t.Fatalf("restart retained provider-private wire state: messages=%#v session=%v capsules=%d", messages, session, len(capsules))
	}
}

func TestPrivateProtocolPauseContinuationSeparatesExactAndSafeHistory(t *testing.T) {
	newPrivateState := func(t *testing.T) (*domainmodel.PrivateProtocolSession, *domainmodel.AnthropicThinkingCapsule) {
		t.Helper()
		session, err := domainmodel.NewPrivateProtocolSession()
		if err != nil {
			t.Fatalf("create private protocol session: %v", err)
		}
		return session, &domainmodel.AnthropicThinkingCapsule{}
	}
	assertSafeHistory := func(t *testing.T, messages []provider.Message) {
		t.Helper()
		if len(messages) == 0 {
			t.Fatal("safe history is empty")
		}
		for _, message := range messages {
			if message.Role == "tool" || (message.Role == "assistant" && len(message.ToolCalls) > 0) {
				t.Fatalf("safe history retained provider tool wire: %#v", messages)
			}
		}
	}

	t.Run("deepseek always recompiles across pause", func(t *testing.T) {
		session, capsule := newPrivateState(t)
		messages := []provider.Message{
			{Role: "assistant", ToolCalls: []provider.ToolCall{{ID: "call_deepseek", Name: "read"}}},
			{Role: "tool", ToolCallID: "call_deepseek", Content: "result"},
		}
		projected, gotSession, capsules, err := runtimePrivateProtocolContinuationState(runtimePendingToolCall{
			ProviderConfig: provider.TurnConfig{
				Family: "openai-compatible", EndpointFormat: "chat_completions",
				ReasoningProtocol: "deepseek-chat-completions",
			},
			Effort: "high", PrivateProtocolSession: session,
			PrivateProtocolCapsules: []*domainmodel.AnthropicThinkingCapsule{capsule},
		}, messages)
		if err != nil {
			t.Fatalf("recompile DeepSeek pause: %v", err)
		}
		assertSafeHistory(t, projected)
		if gotSession != nil || len(capsules) != 0 {
			t.Fatal("DeepSeek pause retained volatile private protocol state")
		}
	})

	t.Run("anthropic complete batch recompiles across pause", func(t *testing.T) {
		session, capsule := newPrivateState(t)
		messages := []provider.Message{
			{Role: "assistant", ToolCalls: []provider.ToolCall{
				{ID: "toolu_a", Name: "read"},
				{ID: "toolu_b", Name: "list"},
			}},
			{Role: "tool", ToolCallID: "toolu_a", Content: "a"},
			{Role: "tool", ToolCallID: "toolu_b", Content: "b"},
		}
		projected, gotSession, capsules, err := runtimePrivateProtocolContinuationState(runtimePendingToolCall{
			ProviderConfig: provider.TurnConfig{
				Family: "anthropic-compatible", EndpointFormat: "messages",
				ReasoningProtocol: "anthropic-thinking",
			},
			Effort: "high", PrivateProtocolSession: session,
			PrivateProtocolCapsules: []*domainmodel.AnthropicThinkingCapsule{capsule},
		}, messages)
		if err != nil {
			t.Fatalf("recompile complete Anthropic batch: %v", err)
		}
		assertSafeHistory(t, projected)
		if gotSession != nil || len(capsules) != 0 {
			t.Fatal("complete Anthropic batch replayed volatile state across pause")
		}
	})

	t.Run("anthropic incomplete batch recompiles", func(t *testing.T) {
		session, capsule := newPrivateState(t)
		messages := []provider.Message{
			{Role: "assistant", ToolCalls: []provider.ToolCall{
				{ID: "toolu_a", Name: "read"},
				{ID: "toolu_b", Name: "list"},
			}},
			{Role: "tool", ToolCallID: "toolu_a", Content: "a"},
		}
		projected, gotSession, capsules, err := runtimePrivateProtocolContinuationState(runtimePendingToolCall{
			ProviderConfig: provider.TurnConfig{
				Family: "anthropic-compatible", EndpointFormat: "messages",
				ReasoningProtocol: "anthropic-thinking",
			},
			Effort: "high", PrivateProtocolSession: session,
			PrivateProtocolCapsules: []*domainmodel.AnthropicThinkingCapsule{capsule},
		}, messages)
		if err != nil {
			t.Fatalf("recompile incomplete Anthropic batch: %v", err)
		}
		assertSafeHistory(t, projected)
		if gotSession != nil || len(capsules) != 0 {
			t.Fatal("incomplete Anthropic batch retained volatile private protocol state")
		}
	})

	t.Run("ordinary anthropic tool wire remains ordinary", func(t *testing.T) {
		messages := []provider.Message{
			{Role: "assistant", ToolCalls: []provider.ToolCall{{ID: "toolu_plain", Name: "read"}}},
			{Role: "tool", ToolCallID: "toolu_plain", Content: "plain"},
		}
		got, session, capsules, err := runtimePrivateProtocolContinuationState(runtimePendingToolCall{
			ProviderConfig: provider.TurnConfig{
				Family: "anthropic-compatible", EndpointFormat: "messages",
			},
			Effort: "off",
		}, messages)
		if err != nil {
			t.Fatalf("ordinary Anthropic continuation: %v", err)
		}
		if len(got) != len(messages) || session != nil || len(capsules) != 0 ||
			got[0].Role != "assistant" || len(got[0].ToolCalls) != 1 || got[1].Role != "tool" {
			t.Fatal("ordinary Anthropic continuation was rewritten as private history")
		}
	})

	t.Run("ordinary anthropic default effort remains ordinary", func(t *testing.T) {
		messages := []provider.Message{
			{Role: "assistant", ToolCalls: []provider.ToolCall{{ID: "toolu_plain_default", Name: "read"}}},
			{Role: "tool", ToolCallID: "toolu_plain_default", Content: "plain"},
		}
		got, session, capsules, err := runtimePrivateProtocolContinuationState(runtimePendingToolCall{
			ProviderConfig: provider.TurnConfig{
				Family: "anthropic-compatible", EndpointFormat: "messages",
			},
		}, messages)
		if err != nil {
			t.Fatalf("ordinary Anthropic default continuation: %v", err)
		}
		if len(got) != len(messages) || session != nil || len(capsules) != 0 ||
			got[0].Role != "assistant" || len(got[0].ToolCalls) != 1 || got[1].Role != "tool" {
			t.Fatal("ordinary Anthropic default continuation was rewritten as private history")
		}
	})

	t.Run("observed legacy anthropic missing state recompiles", func(t *testing.T) {
		messages := []provider.Message{
			{Role: "assistant", ToolCalls: []provider.ToolCall{{ID: "toolu_legacy", Name: "read"}}},
			{Role: "tool", ToolCallID: "toolu_legacy", Content: "legacy"},
		}
		projected, session, capsules, err := runtimePrivateProtocolContinuationState(runtimePendingToolCall{
			ProviderConfig: provider.TurnConfig{
				Family: "anthropic-compatible", EndpointFormat: "messages",
			},
			PrivateProtocolObserved: true,
		}, messages)
		if err != nil {
			t.Fatalf("legacy Anthropic continuation: %v", err)
		}
		assertSafeHistory(t, projected)
		if session != nil || len(capsules) != 0 || len(projected) != 1 ||
			!strings.Contains(projected[0].Content, "Analytix host-selected prior tool activity") {
			t.Fatalf("legacy Anthropic continuation did not use safe history: %#v", projected)
		}
	})

	t.Run("unknown protocol rejects private state", func(t *testing.T) {
		session, capsule := newPrivateState(t)
		_, _, _, err := runtimePrivateProtocolContinuationState(runtimePendingToolCall{
			ProviderConfig: provider.TurnConfig{
				Family: "anthropic-compatible", EndpointFormat: "messages",
				ReasoningProtocol: "future-private-protocol",
			},
			Effort: "off", PrivateProtocolSession: session,
			PrivateProtocolCapsules: []*domainmodel.AnthropicThinkingCapsule{capsule},
		}, nil)
		if err == nil {
			t.Fatal("unrecognized protocol accepted volatile private state")
		}
	})

	t.Run("unknown protocol rejects durable observation", func(t *testing.T) {
		_, _, _, err := runtimePrivateProtocolContinuationState(runtimePendingToolCall{
			ProviderConfig: provider.TurnConfig{
				Family: "anthropic-compatible", EndpointFormat: "messages",
				ReasoningProtocol: "future-private-protocol",
			},
			PrivateProtocolObserved: true,
		}, nil)
		if err == nil {
			t.Fatal("unrecognized protocol accepted a durable private-protocol observation")
		}
	})
}

func TestProviderConfigSupportsReasoningEffortOnlyForScopedProtocols(t *testing.T) {
	cases := []struct {
		name   string
		config provider.TurnConfig
		want   bool
	}{
		{"deepseek family without host protocol", provider.TurnConfig{Family: "deepseek"}, false},
		{"explicit deepseek protocol", provider.TurnConfig{Family: "openai-compatible", ReasoningProtocol: "deepseek-chat-completions"}, true},
		{"mimo profile", provider.TurnConfig{Family: "openai-compatible", ReasoningProtocol: "mimo-chat-completions"}, true},
		{"openai responses profile", provider.TurnConfig{Family: "openai-compatible", ReasoningProtocol: "openai-responses"}, true},
		{"anthropic thinking profile", provider.TurnConfig{Family: "anthropic-compatible", ReasoningProtocol: "anthropic-thinking"}, true},
		{"ordinary openai compatible", provider.TurnConfig{Family: "openai-compatible"}, false},
		{"reasoning disabled profile", provider.TurnConfig{Family: "openai-compatible", ReasoningProtocol: "none"}, false},
	}
	for _, tc := range cases {
		if got := appmodel.SupportsReasoningEffort(tc.config); got != tc.want {
			t.Fatalf("%s support mismatch: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestRuntimeDefaultReasoningProtocolKeepsEndpointFormatOutOfReasoning(t *testing.T) {
	cases := []struct {
		name   string
		config provider.TurnConfig
		want   string
	}{
		{
			name: "deepseek family without host protocol",
			config: provider.TurnConfig{
				Family:         "deepseek",
				EndpointFormat: "chat_completions",
			},
			want: "none",
		},
		{
			name: "explicit deepseek protocol",
			config: provider.TurnConfig{
				Family:            "openai-compatible",
				EndpointFormat:    "chat_completions",
				ReasoningProtocol: "deepseek-chat-completions",
			},
			want: "deepseek-chat-completions",
		},
		{
			name: "ordinary openai compatible",
			config: provider.TurnConfig{
				Family:         "openai-compatible",
				EndpointFormat: "chat_completions",
			},
			want: "none",
		},
		{
			name: "anthropic messages without signed thinking",
			config: provider.TurnConfig{
				Family:         "anthropic-compatible",
				EndpointFormat: "messages",
			},
			want: "none",
		},
		{
			name: "explicit anthropic thinking",
			config: provider.TurnConfig{
				Family:            "anthropic-compatible",
				EndpointFormat:    "messages",
				ReasoningProtocol: "anthropic-thinking",
			},
			want: "anthropic-thinking",
		},
	}
	for _, tc := range cases {
		if got := runtimeinfoapp.DefaultReasoningProtocol(runtimeinfoapp.ModelCapabilityConfig{
			Family:            tc.config.Family,
			ReasoningProtocol: tc.config.ReasoningProtocol,
		}); got != tc.want {
			t.Fatalf("%s protocol mismatch: got %q want %q", tc.name, got, tc.want)
		}
	}
}
