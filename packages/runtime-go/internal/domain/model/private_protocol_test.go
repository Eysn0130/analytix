package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestAnthropicThinkingCapsuleIsExactChainBoundAndVolatile(t *testing.T) {
	session, err := newPrivateProtocolSession(bytes.NewReader(bytes.Repeat([]byte{0x42}, 32)))
	if err != nil {
		t.Fatalf("create private protocol session: %v", err)
	}
	assistant := Message{Role: "assistant", Content: "checking", ToolCalls: []ToolCall{{
		ID: "toolu_read", Name: "read_file", Arguments: json.RawMessage(`{"path":"README.md"}`),
	}}}
	contextDigest := domainsecurity.SHA256Hex([]byte("context"))
	providerRouteHash := domainsecurity.SHA256Hex([]byte("provider-route"))
	toolManifestHash := domainsecurity.SHA256Hex([]byte("tool-manifest"))
	capsule, err := session.IssueAnthropicThinking(AnthropicThinkingCapsuleInput{
		ContextDigest: contextDigest, ProviderRouteHash: providerRouteHash, PromptRoute: "general",
		ToolManifestHash: toolManifestHash, Sequence: 2, AssistantMessageIndex: 1, AssistantMessage: assistant,
		PrefixMessages: []Message{{Role: "user", Content: "inspect"}},
		Thinking:       "PRIVATE_REASONING_SENTINEL", Signature: "sig-anthropic",
	})
	if err != nil {
		t.Fatalf("issue capsule: %v", err)
	}
	messages := []Message{{Role: "user", Content: "inspect"}, assistant, {
		Role: "tool", ToolCallID: "toolu_read", Content: "README contents",
	}}
	replay, err := session.ConsumeAnthropicThinking(capsule, AnthropicThinkingConsumeInput{
		ContextDigest: contextDigest, ProviderRouteHash: providerRouteHash, PromptRoute: "general",
		ToolManifestHash: toolManifestHash, Sequence: 2, Messages: messages,
	})
	if err != nil {
		t.Fatalf("consume capsule: %v", err)
	}
	thinking, signature, ok := replay.ThinkingBlock(1, assistant)
	if !ok || thinking != "PRIVATE_REASONING_SENTINEL" || signature != "sig-anthropic" {
		t.Fatalf("private replay mismatch: ok=%v thinking=%q signature=%q", ok, thinking, signature)
	}
	if _, err := session.ConsumeAnthropicThinking(capsule, AnthropicThinkingConsumeInput{
		ContextDigest: contextDigest, ProviderRouteHash: providerRouteHash, PromptRoute: "general",
		ToolManifestHash: toolManifestHash, Sequence: 3, Messages: messages,
	}); err != nil {
		t.Fatalf("replaying the same exact block later in its uninterrupted chain failed: %v", err)
	}
	encoded, err := json.Marshal(struct {
		Capsule *AnthropicThinkingCapsule `json:"capsule"`
		Replay  *AnthropicThinkingReplay  `json:"replay"`
	}{Capsule: capsule, Replay: replay})
	if err != nil {
		t.Fatalf("marshal volatile handles: %v", err)
	}
	if strings.Contains(string(encoded), "PRIVATE_REASONING_SENTINEL") || strings.Contains(string(encoded), "sig-anthropic") {
		t.Fatalf("private protocol bytes became JSON-visible: %s", encoded)
	}
	for _, format := range []string{"%s", "%v", "%+v", "%#v", "%q"} {
		diagnostic := fmt.Sprintf(format, struct {
			Session *PrivateProtocolSession
			Capsule *AnthropicThinkingCapsule
			Replay  *AnthropicThinkingReplay
		}{Session: session, Capsule: capsule, Replay: replay})
		if strings.Contains(diagnostic, "PRIVATE_REASONING_SENTINEL") || strings.Contains(diagnostic, "sig-anthropic") {
			t.Fatalf("private protocol bytes became diagnostic-visible for %s: %s", format, diagnostic)
		}
	}
}

func TestAnthropicThinkingCapsuleAcceptsSignatureOnlyBlock(t *testing.T) {
	session, err := NewPrivateProtocolSession()
	if err != nil {
		t.Fatal(err)
	}
	assistant := Message{Role: "assistant", ToolCalls: []ToolCall{{
		ID: "toolu_signature", Name: "lookup", Arguments: json.RawMessage(`{"q":"x"}`),
	}}}
	contextDigest := domainsecurity.SHA256Hex([]byte("signature-only-context"))
	providerRouteHash := domainsecurity.SHA256Hex([]byte("signature-only-route"))
	toolManifestHash := domainsecurity.SHA256Hex([]byte("signature-only-tools"))
	capsule, err := session.IssueAnthropicThinking(AnthropicThinkingCapsuleInput{
		Protocol: "anthropic-thinking", ContextDigest: contextDigest,
		ProviderRouteHash: providerRouteHash, PromptRoute: "general",
		ToolManifestHash: toolManifestHash, Sequence: 2, AssistantMessageIndex: 1,
		PrefixMessages:   []Message{{Role: "user", Content: "inspect"}},
		AssistantMessage: assistant, Thinking: "", Signature: "encrypted-signature-only",
	})
	if err != nil {
		t.Fatalf("issue signature-only capsule: %v", err)
	}
	replay, err := session.ConsumeAnthropicThinking(capsule, AnthropicThinkingConsumeInput{
		Protocol: "anthropic-thinking", ContextDigest: contextDigest,
		ProviderRouteHash: providerRouteHash, PromptRoute: "general",
		ToolManifestHash: toolManifestHash, Sequence: 2,
		Messages: []Message{
			{Role: "user", Content: "inspect"},
			assistant,
			{Role: "tool", ToolCallID: "toolu_signature", Content: "ok"},
		},
	})
	if err != nil {
		t.Fatalf("consume signature-only capsule: %v", err)
	}
	thinking, signature, ok := replay.ThinkingBlock(1, assistant)
	if !ok || thinking != "" || signature != "encrypted-signature-only" {
		t.Fatalf("signature-only replay mismatch: ok=%t thinking=%q signature=%q", ok, thinking, signature)
	}
}

func TestAnthropicThinkingRequestShapeChangedAuthenticatesCapsuleAndKeepsProtocolPrivate(t *testing.T) {
	session, err := newPrivateProtocolSession(bytes.NewReader(bytes.Repeat([]byte{0x31}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	assistant := Message{Role: "assistant", ToolCalls: []ToolCall{{
		ID: "toolu_shape", Name: "lookup", Arguments: json.RawMessage(`{"q":"shape"}`),
	}}}
	contextDigest := domainsecurity.SHA256Hex([]byte("shape-context"))
	providerRouteHash := domainsecurity.SHA256Hex([]byte("shape-provider-route"))
	toolManifestHash := domainsecurity.SHA256Hex([]byte("shape-tools"))
	otherManifestHash := domainsecurity.SHA256Hex([]byte("other-shape-tools"))
	issue := func(t *testing.T, protocol string) *AnthropicThinkingCapsule {
		t.Helper()
		input := AnthropicThinkingCapsuleInput{
			Protocol: protocol, ContextDigest: contextDigest,
			ProviderRouteHash: providerRouteHash, PromptRoute: "general",
			ToolManifestHash: toolManifestHash, Sequence: 2, AssistantMessageIndex: 1,
			PrefixMessages:   []Message{{Role: "user", Content: "inspect"}},
			AssistantMessage: assistant,
		}
		if protocol == "deepseek-chat-completions" {
			input.Thinking = "PRIVATE_DEEPSEEK_REASONING"
		} else {
			input.Thinking = "PRIVATE_ANTHROPIC_THINKING"
			input.Signature = "PRIVATE_ANTHROPIC_SIGNATURE"
		}
		capsule, issueErr := session.IssueAnthropicThinking(input)
		if issueErr != nil {
			t.Fatalf("issue %s capsule: %v", protocol, issueErr)
		}
		return capsule
	}

	anthropic := issue(t, "anthropic-thinking")
	for name, test := range map[string]struct {
		promptRoute      string
		toolManifestHash string
		wantChanged      bool
	}{
		"same shape":    {promptRoute: " general ", toolManifestHash: " " + toolManifestHash + " "},
		"prompt route":  {promptRoute: "plan", toolManifestHash: toolManifestHash, wantChanged: true},
		"tool manifest": {promptRoute: "general", toolManifestHash: otherManifestHash, wantChanged: true},
	} {
		t.Run(name, func(t *testing.T) {
			changed, shapeErr := session.AnthropicThinkingRequestShapeChanged(
				anthropic,
				test.promptRoute,
				test.toolManifestHash,
			)
			if shapeErr != nil {
				t.Fatalf("inspect request shape: %v", shapeErr)
			}
			if changed != test.wantChanged {
				t.Fatalf("shape change mismatch: got %t want %t", changed, test.wantChanged)
			}
		})
	}

	deepSeek := issue(t, "deepseek-chat-completions")
	changed, err := session.AnthropicThinkingRequestShapeChanged(deepSeek, "agent", otherManifestHash)
	if err != nil {
		t.Fatalf("inspect DeepSeek request shape: %v", err)
	}
	if changed {
		t.Fatal("DeepSeek catalog changes must remain permitted in the current volatile chain")
	}

	tampered := *anthropic
	tampered.payload.PromptRoute = "tampered"
	if _, err := session.AnthropicThinkingRequestShapeChanged(
		&tampered,
		"tampered",
		toolManifestHash,
	); err == nil {
		t.Fatal("tampered capsule must fail authentication")
	}

	encoded, err := json.Marshal(struct {
		Anthropic *AnthropicThinkingCapsule `json:"anthropic"`
		DeepSeek  *AnthropicThinkingCapsule `json:"deepSeek"`
	}{Anthropic: anthropic, DeepSeek: deepSeek})
	if err != nil {
		t.Fatalf("marshal private protocol handles: %v", err)
	}
	for _, privateValue := range []string{
		"PRIVATE_ANTHROPIC_THINKING",
		"PRIVATE_ANTHROPIC_SIGNATURE",
		"PRIVATE_DEEPSEEK_REASONING",
	} {
		if strings.Contains(string(encoded), privateValue) {
			t.Fatalf("private protocol bytes became JSON-visible: %s", encoded)
		}
	}
}

func TestAnthropicThinkingCapsuleRejectsBindingAndToolResultDrift(t *testing.T) {
	newFixture := func(t *testing.T) (*PrivateProtocolSession, *AnthropicThinkingCapsule, AnthropicThinkingConsumeInput) {
		t.Helper()
		session, err := NewPrivateProtocolSession()
		if err != nil {
			t.Fatalf("create session: %v", err)
		}
		assistant := Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "toolu_a", Name: "read", Arguments: json.RawMessage(`{"path":"a"}`)}}}
		input := AnthropicThinkingConsumeInput{
			ContextDigest: domainsecurity.SHA256Hex([]byte("context")), ProviderRouteHash: domainsecurity.SHA256Hex([]byte("route")),
			PromptRoute: "general", ToolManifestHash: domainsecurity.SHA256Hex([]byte("tools")), Sequence: 2,
			Messages: []Message{{Role: "user", Content: "inspect"}, assistant, {Role: "tool", ToolCallID: "toolu_a", Content: "a"}},
		}
		capsule, err := session.IssueAnthropicThinking(AnthropicThinkingCapsuleInput{
			ContextDigest: input.ContextDigest, ProviderRouteHash: input.ProviderRouteHash, PromptRoute: input.PromptRoute,
			ToolManifestHash: input.ToolManifestHash, Sequence: input.Sequence, AssistantMessageIndex: 1,
			PrefixMessages: input.Messages[:1], AssistantMessage: assistant,
			Thinking: "private", Signature: "signature",
		})
		if err != nil {
			t.Fatalf("issue capsule: %v", err)
		}
		return session, capsule, input
	}

	t.Run("context epoch", func(t *testing.T) {
		session, capsule, input := newFixture(t)
		input.ContextDigest = domainsecurity.SHA256Hex([]byte("other-context"))
		if _, err := session.ConsumeAnthropicThinking(capsule, input); err == nil {
			t.Fatal("context drift must reject private protocol replay")
		}
	})
	t.Run("tool manifest", func(t *testing.T) {
		session, capsule, input := newFixture(t)
		input.ToolManifestHash = domainsecurity.SHA256Hex([]byte("other-tools"))
		if _, err := session.ConsumeAnthropicThinking(capsule, input); err == nil {
			t.Fatal("tool manifest drift must reject private protocol replay")
		}
	})
	t.Run("tool result", func(t *testing.T) {
		session, capsule, input := newFixture(t)
		input.Messages[2].ToolCallID = "toolu_other"
		if _, err := session.ConsumeAnthropicThinking(capsule, input); err == nil {
			t.Fatal("tool-result pairing drift must reject private protocol replay")
		}
	})
	t.Run("extra contiguous tool result", func(t *testing.T) {
		session, capsule, input := newFixture(t)
		input.Messages = append(input.Messages, Message{
			Role: "tool", ToolCallID: "toolu_extra", Content: "unexpected",
		})
		if _, err := session.ConsumeAnthropicThinking(capsule, input); err == nil {
			t.Fatal("an extra immediate tool result must reject private protocol replay")
		}
	})
	t.Run("tool result content after first replay", func(t *testing.T) {
		session, capsule, input := newFixture(t)
		if _, err := session.ConsumeAnthropicThinking(capsule, input); err != nil {
			t.Fatalf("consume initial exact result: %v", err)
		}
		input.Sequence++
		input.Messages[2].Content = "changed result"
		if _, err := session.ConsumeAnthropicThinking(capsule, input); err == nil {
			t.Fatal("tool-result content drift must reject later private protocol replay")
		}
	})
	t.Run("message prefix", func(t *testing.T) {
		session, capsule, input := newFixture(t)
		input.Messages[0].Content = "different request"
		if _, err := session.ConsumeAnthropicThinking(capsule, input); err == nil {
			t.Fatal("provider message prefix drift must reject private protocol replay")
		}
	})
	t.Run("historical thinking", func(t *testing.T) {
		session, capsule, input := newFixture(t)
		input.Messages[1].Parts = []MessagePart{{Type: "thinking", Text: "forged", Signature: "forged"}}
		if _, err := session.ConsumeAnthropicThinking(capsule, input); err == nil {
			t.Fatal("generic historical thinking must fail closed")
		}
	})
}

func TestDeepSeekReasoningCapsulesReplayCompleteCurrentChainAcrossCatalogChanges(t *testing.T) {
	session, err := NewPrivateProtocolSession()
	if err != nil {
		t.Fatal(err)
	}
	contextDigest := domainsecurity.SHA256Hex([]byte("context"))
	providerRouteHash := domainsecurity.SHA256Hex([]byte("provider-route"))
	firstManifest := domainsecurity.SHA256Hex([]byte("plan-tools"))
	secondManifest := domainsecurity.SHA256Hex([]byte("agent-tools"))
	firstAssistant := Message{Role: "assistant", ToolCalls: []ToolCall{{
		ID: "call_plan", Name: "create_plan", Arguments: json.RawMessage(`{"steps":[]}`),
	}}}
	secondAssistant := Message{Role: "assistant", ToolCalls: []ToolCall{{
		ID: "call_read", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`),
	}}}
	messages := []Message{
		{Role: "user", Content: "plan and inspect"},
		firstAssistant,
		{Role: "tool", ToolCallID: "call_plan", Content: "plan created"},
		secondAssistant,
		{Role: "tool", ToolCallID: "call_read", Content: "README contents"},
	}
	first, err := session.IssueAnthropicThinking(AnthropicThinkingCapsuleInput{
		Protocol: "deepseek-chat-completions", ContextDigest: contextDigest,
		ProviderRouteHash: providerRouteHash, PromptRoute: "plan",
		ToolManifestHash: firstManifest, Sequence: 2, AssistantMessageIndex: 1,
		PrefixMessages: messages[:1], AssistantMessage: firstAssistant, Thinking: "private plan reasoning",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := session.IssueAnthropicThinking(AnthropicThinkingCapsuleInput{
		Protocol: "deepseek-chat-completions", ContextDigest: contextDigest,
		ProviderRouteHash: providerRouteHash, PromptRoute: "agent",
		ToolManifestHash: secondManifest, Sequence: 3, AssistantMessageIndex: 3,
		PrefixMessages: messages[:3], AssistantMessage: secondAssistant, Thinking: "private read reasoning",
	})
	if err != nil {
		t.Fatal(err)
	}
	consume := func(capsule *AnthropicThinkingCapsule, sequence uint64) *AnthropicThinkingReplay {
		t.Helper()
		replay, consumeErr := session.ConsumeAnthropicThinking(capsule, AnthropicThinkingConsumeInput{
			Protocol: "deepseek-chat-completions", ContextDigest: contextDigest,
			ProviderRouteHash: providerRouteHash, PromptRoute: "agent",
			ToolManifestHash: secondManifest, Sequence: sequence, Messages: messages,
		})
		if consumeErr != nil {
			t.Fatalf("consume DeepSeek capsule at sequence %d: %v", sequence, consumeErr)
		}
		return replay
	}
	if reasoning, ok := consume(first, 3).ReasoningContent(1, firstAssistant); !ok || reasoning != "private plan reasoning" {
		t.Fatalf("first DeepSeek reasoning replay mismatch: ok=%t reasoning=%q", ok, reasoning)
	}
	if reasoning, ok := consume(second, 3).ReasoningContent(3, secondAssistant); !ok || reasoning != "private read reasoning" {
		t.Fatalf("second DeepSeek reasoning replay mismatch: ok=%t reasoning=%q", ok, reasoning)
	}
	if _, ok := consume(first, 4).ReasoningContent(1, firstAssistant); !ok {
		t.Fatal("DeepSeek reasoning must remain available for all later requests in the same volatile chain")
	}

	extraResult := make([]Message, 0, len(messages)+1)
	extraResult = append(extraResult, messages[:3]...)
	extraResult = append(extraResult, Message{
		Role: "tool", ToolCallID: "call_extra", Content: "unexpected result",
	})
	extraResult = append(extraResult, messages[3:]...)
	if _, err := session.ConsumeAnthropicThinking(first, AnthropicThinkingConsumeInput{
		Protocol: "deepseek-chat-completions", ContextDigest: contextDigest,
		ProviderRouteHash: providerRouteHash, PromptRoute: "agent",
		ToolManifestHash: secondManifest, Sequence: 4, Messages: extraResult,
	}); err == nil {
		t.Fatal("DeepSeek replay accepted an extra contiguous tool result")
	}

	contentDrifted := clonePrivateProtocolTestMessages(messages)
	contentDrifted[2].Content = "tampered plan result"
	if _, err := session.ConsumeAnthropicThinking(first, AnthropicThinkingConsumeInput{
		Protocol: "deepseek-chat-completions", ContextDigest: contextDigest,
		ProviderRouteHash: providerRouteHash, PromptRoute: "agent",
		ToolManifestHash: secondManifest, Sequence: 4, Messages: contentDrifted,
	}); err == nil {
		t.Fatal("DeepSeek replay accepted changed immediate tool-result content")
	}

	drifted := clonePrivateProtocolTestMessages(messages)
	drifted[0].Content = "different request"
	if _, err := session.ConsumeAnthropicThinking(first, AnthropicThinkingConsumeInput{
		Protocol: "deepseek-chat-completions", ContextDigest: contextDigest,
		ProviderRouteHash: providerRouteHash, PromptRoute: "agent",
		ToolManifestHash: secondManifest, Sequence: 4, Messages: drifted,
	}); err == nil {
		t.Fatal("DeepSeek replay accepted a changed provider message prefix")
	}
}

func clonePrivateProtocolTestMessages(messages []Message) []Message {
	out := make([]Message, len(messages))
	copy(out, messages)
	return out
}
