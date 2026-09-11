package model

import (
	"encoding/json"
	"errors"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestAnthropicPrivateProtocolEndpointAndRequiredPolicy(t *testing.T) {
	endpointTests := []struct {
		name         string
		format       string
		requestShape string
		wantCapable  bool
	}{
		{name: "anthropic messages", format: "messages", wantCapable: true},
		{name: "trimmed messages", format: " messages ", wantCapable: true},
		{name: "custom messages", format: "custom_endpoint", requestShape: "messages", wantCapable: true},
		{name: "custom chat completions", format: "custom_endpoint", requestShape: "chat_completions"},
		{name: "openai chat completions", format: "chat_completions"},
		{name: "openai responses", format: "responses"},
		{name: "missing format"},
	}
	for _, test := range endpointTests {
		t.Run(test.name, func(t *testing.T) {
			if got := AnthropicPrivateProtocolEndpointCapable(test.format, test.requestShape); got != test.wantCapable {
				t.Fatalf("endpoint capability mismatch: got %t want %t", got, test.wantCapable)
			}
		})
	}

	requiredTests := []struct {
		name     string
		protocol string
		effort   string
		required bool
	}{
		{name: "anthropic high", protocol: "anthropic-thinking", effort: "high", required: true},
		{name: "anthropic trimmed effort", protocol: "anthropic-thinking", effort: " HIGH ", required: true},
		{name: "anthropic off", protocol: "anthropic-thinking", effort: "off"},
		{name: "anthropic blank", protocol: "anthropic-thinking"},
		{name: "deepseek protocol", protocol: "deepseek-reasoning", effort: "high"},
		{name: "DeepSeek chat completions high", protocol: "deepseek-chat-completions", effort: "high", required: true},
		{name: "DeepSeek chat completions default-on", protocol: "deepseek-chat-completions", required: true},
		{name: "DeepSeek chat completions off", protocol: "deepseek-chat-completions", effort: "off"},
	}
	for _, test := range requiredTests {
		t.Run(test.name, func(t *testing.T) {
			if got := AnthropicPrivateProtocolRequired(test.protocol, test.effort); got != test.required {
				t.Fatalf("required policy mismatch: got %t want %t", got, test.required)
			}
		})
	}
}

func TestPrivateProtocolReasoningKindForProviderRejectsExplicitUnknown(t *testing.T) {
	config := privateProtocolFlowConfig("messages", "https://api.anthropic.test/v1")
	config.ReasoningProtocol = "future-private-protocol"
	if got := privateProtocolReasoningKindForProvider(config, "messages", ""); got != "" {
		t.Fatalf("explicit unknown protocol inferred as %q", got)
	}
	config.ReasoningProtocol = ""
	if got := privateProtocolReasoningKindForProvider(config, "messages", ""); got != "anthropic-thinking" {
		t.Fatalf("legacy blank Anthropic protocol inference = %q", got)
	}
}

func TestObservePrivateProtocolResponseValidatesActualProviderFields(t *testing.T) {
	implicitAnthropic := privateProtocolFlowConfig("messages", "https://api.anthropic.test/v1")
	implicitAnthropic.ReasoningProtocol = ""
	observed, err := ObservePrivateProtocolResponse(PrivateProtocolResponseObservationInput{
		ProviderConfig: implicitAnthropic, EndpointFormat: "messages",
		Signature: "signed-thinking", Effort: "",
	})
	if err != nil || !observed {
		t.Fatalf("implicit Anthropic signature was not observed: observed=%t err=%v", observed, err)
	}
	if _, err := ObservePrivateProtocolResponse(PrivateProtocolResponseObservationInput{
		ProviderConfig: implicitAnthropic, EndpointFormat: "messages",
		Thinking: "unsigned thinking",
	}); !errors.Is(err, ErrAnthropicPrivateProtocolIncomplete) {
		t.Fatalf("unsigned Anthropic thinking error = %v", err)
	}

	explicitAnthropic := implicitAnthropic
	explicitAnthropic.ReasoningProtocol = "anthropic-thinking"
	if _, err := ObservePrivateProtocolResponse(PrivateProtocolResponseObservationInput{
		ProviderConfig: explicitAnthropic, EndpointFormat: "messages", Effort: "high",
	}); !errors.Is(err, ErrAnthropicPrivateProtocolMissing) {
		t.Fatalf("missing required Anthropic thinking error = %v", err)
	}

	ordinary, err := ObservePrivateProtocolResponse(PrivateProtocolResponseObservationInput{
		ProviderConfig: implicitAnthropic, EndpointFormat: "messages",
	})
	if err != nil || ordinary {
		t.Fatalf("ordinary implicit Anthropic response became private: observed=%t err=%v", ordinary, err)
	}

	unknown := implicitAnthropic
	unknown.ReasoningProtocol = "future-private-protocol"
	if _, err := ObservePrivateProtocolResponse(PrivateProtocolResponseObservationInput{
		ProviderConfig: unknown, EndpointFormat: "messages", Signature: "unexpected-private",
	}); !errors.Is(err, ErrAnthropicPrivateProtocolFlowBinding) {
		t.Fatalf("unknown private protocol response error = %v", err)
	}
}

func TestIssueAndConsumeAnthropicPrivateProtocol(t *testing.T) {
	for _, endpoint := range []struct {
		name         string
		format       string
		requestShape string
		baseURL      string
	}{
		{name: "messages", format: "messages", baseURL: "https://api.anthropic.test/v1"},
		{name: "custom messages", format: "custom_endpoint", requestShape: "messages", baseURL: "https://custom.test/v1/messages"},
	} {
		t.Run(endpoint.name, func(t *testing.T) {
			fixture := newPrivateProtocolFlowFixture(endpoint.format, endpoint.requestShape, endpoint.baseURL)
			issued, err := IssueAnthropicPrivateProtocol(fixture.issue)
			if err != nil || issued.Session == nil || issued.Capsule == nil {
				t.Fatalf("issue private protocol capsule: result=%#v err=%v", issued, err)
			}
			messages := append([]domainmodel.Message(nil), fixture.issue.Messages...)
			messages = append(messages, fixture.issue.AssistantMessage, domainmodel.Message{
				Role: "tool", ToolCallID: fixture.issue.AssistantMessage.ToolCalls[0].ID, Content: "tool result",
			})
			replay, err := ConsumeAnthropicPrivateProtocol(AnthropicPrivateProtocolConsumeInput{
				Session: issued.Session, Capsule: issued.Capsule, ProviderConfig: fixture.issue.ProviderConfig,
				EndpointFormat: endpoint.format, CustomRequestShape: endpoint.requestShape,
				ContextDigest: fixture.issue.ContextDigest, PromptRoute: fixture.issue.PromptRoute,
				ToolManifestHash: fixture.issue.ToolManifestHash, Sequence: fixture.issue.Sequence, Messages: messages,
			})
			if err != nil {
				t.Fatalf("consume private protocol capsule: %v", err)
			}
			thinking, signature, ok := replay.ThinkingBlock(fixture.issue.AssistantMessageIndex, fixture.issue.AssistantMessage)
			if !ok || thinking != fixture.issue.Thinking || signature != fixture.issue.Signature {
				t.Fatalf("private replay mismatch: ok=%t thinking=%q signature=%q", ok, thinking, signature)
			}
		})
	}
}

func TestIssueAndConsumeImplicitAnthropicMessagesPrivateProtocol(t *testing.T) {
	fixture := newPrivateProtocolFlowFixture(
		"messages",
		"",
		"https://api.anthropic.test/v1",
	)
	fixture.issue.ProviderConfig.ReasoningProtocol = ""
	fixture.issue.Effort = ""
	issued, err := IssueAnthropicPrivateProtocol(fixture.issue)
	if err != nil || issued.Session == nil || issued.Capsule == nil {
		t.Fatalf("issue implicit Anthropic capsule: result=%#v err=%v", issued, err)
	}
	messages := append([]domainmodel.Message(nil), fixture.issue.Messages...)
	messages = append(messages, fixture.issue.AssistantMessage, domainmodel.Message{
		Role: "tool", ToolCallID: fixture.issue.AssistantMessage.ToolCalls[0].ID, Content: "tool result",
	})
	replay, err := ConsumeAnthropicPrivateProtocol(AnthropicPrivateProtocolConsumeInput{
		Session: issued.Session, Capsule: issued.Capsule, ProviderConfig: fixture.issue.ProviderConfig,
		EndpointFormat: fixture.issue.EndpointFormat,
		ContextDigest:  fixture.issue.ContextDigest, PromptRoute: fixture.issue.PromptRoute,
		ToolManifestHash: fixture.issue.ToolManifestHash, Sequence: fixture.issue.Sequence,
		Messages: messages,
	})
	if err != nil {
		t.Fatalf("consume implicit Anthropic capsule: %v", err)
	}
	thinking, signature, ok := replay.ThinkingBlock(
		fixture.issue.AssistantMessageIndex,
		fixture.issue.AssistantMessage,
	)
	if !ok || thinking != fixture.issue.Thinking || signature != fixture.issue.Signature {
		t.Fatalf("implicit Anthropic replay mismatch: ok=%t thinking=%q signature=%q", ok, thinking, signature)
	}
}

func TestIssueAndConsumeAnthropicSignatureOnlyPrivateProtocol(t *testing.T) {
	fixture := newPrivateProtocolFlowFixture(
		"messages",
		"",
		"https://api.anthropic.test/v1",
	)
	fixture.issue.Thinking = ""
	fixture.issue.Signature = "encrypted-signature-only"
	issued, err := IssueAnthropicPrivateProtocol(fixture.issue)
	if err != nil || issued.Session == nil || issued.Capsule == nil {
		t.Fatalf("issue signature-only Anthropic capsule: result=%#v err=%v", issued, err)
	}
	messages := append([]domainmodel.Message(nil), fixture.issue.Messages...)
	messages = append(messages, fixture.issue.AssistantMessage, domainmodel.Message{
		Role: "tool", ToolCallID: fixture.issue.AssistantMessage.ToolCalls[0].ID, Content: "tool result",
	})
	replay, err := ConsumeAnthropicPrivateProtocol(AnthropicPrivateProtocolConsumeInput{
		Session: issued.Session, Capsule: issued.Capsule, ProviderConfig: fixture.issue.ProviderConfig,
		EndpointFormat: fixture.issue.EndpointFormat,
		ContextDigest:  fixture.issue.ContextDigest, PromptRoute: fixture.issue.PromptRoute,
		ToolManifestHash: fixture.issue.ToolManifestHash, Sequence: fixture.issue.Sequence,
		Messages: messages,
	})
	if err != nil {
		t.Fatalf("consume signature-only Anthropic capsule: %v", err)
	}
	thinking, signature, ok := replay.ThinkingBlock(
		fixture.issue.AssistantMessageIndex,
		fixture.issue.AssistantMessage,
	)
	if !ok || thinking != "" || signature != fixture.issue.Signature {
		t.Fatalf("signature-only Anthropic replay mismatch: ok=%t thinking=%q signature=%q", ok, thinking, signature)
	}
}

func TestIssueAndConsumeDeepSeekPrivateProtocol(t *testing.T) {
	fixture := newPrivateProtocolFlowFixture(
		"chat_completions",
		"",
		"https://hub.example/v1",
	)
	fixture.issue.ProviderConfig.Family = "openai-compatible"
	fixture.issue.ProviderConfig.Model = "deepseek-v4-pro"
	fixture.issue.ProviderConfig.ReasoningProtocol = "deepseek-chat-completions"
	fixture.issue.Signature = ""
	issued, err := IssueAnthropicPrivateProtocol(fixture.issue)
	if err != nil || issued.Session == nil || issued.Capsule == nil {
		t.Fatalf("issue DeepSeek private protocol capsule: result=%#v err=%v", issued, err)
	}
	messages := append([]domainmodel.Message(nil), fixture.issue.Messages...)
	messages = append(messages, fixture.issue.AssistantMessage, domainmodel.Message{
		Role: "tool", ToolCallID: fixture.issue.AssistantMessage.ToolCalls[0].ID, Content: "tool result",
	})
	replay, err := ConsumeAnthropicPrivateProtocol(AnthropicPrivateProtocolConsumeInput{
		Session: issued.Session, Capsule: issued.Capsule, ProviderConfig: fixture.issue.ProviderConfig,
		EndpointFormat: fixture.issue.EndpointFormat,
		ContextDigest:  fixture.issue.ContextDigest, PromptRoute: fixture.issue.PromptRoute,
		ToolManifestHash: fixture.issue.ToolManifestHash, Sequence: fixture.issue.Sequence,
		Messages: messages,
	})
	if err != nil {
		t.Fatalf("consume DeepSeek private protocol capsule: %v", err)
	}
	reasoning, ok := replay.ReasoningContent(
		fixture.issue.AssistantMessageIndex,
		fixture.issue.AssistantMessage,
	)
	if !ok || reasoning != fixture.issue.Thinking {
		t.Fatalf("DeepSeek private replay mismatch: ok=%t reasoning=%q", ok, reasoning)
	}
	if _, _, ok := replay.ThinkingBlock(
		fixture.issue.AssistantMessageIndex,
		fixture.issue.AssistantMessage,
	); ok {
		t.Fatal("DeepSeek replay must not become an Anthropic thinking block")
	}
}

func TestIssueAnthropicPrivateProtocolFailsClosed(t *testing.T) {
	tests := []struct {
		name          string
		edit          func(*AnthropicPrivateProtocolIssueInput)
		want          error
		wantNoCapsule bool
	}{
		{
			name: "deepseek forged signature on messages endpoint",
			edit: func(input *AnthropicPrivateProtocolIssueInput) {
				input.ProviderConfig = privateProtocolFlowConfig("messages", "https://api.deepseek.test/v1/messages")
				input.ProviderConfig.Family = "deepseek"
				input.ProviderConfig.ReasoningProtocol = "deepseek-chat-completions"
				input.EndpointFormat = "messages"
			},
			want: ErrAnthropicPrivateProtocolEndpoint,
		},
		{
			name: "non-messages signature",
			edit: func(input *AnthropicPrivateProtocolIssueInput) {
				input.ProviderConfig = privateProtocolFlowConfig("chat_completions", "https://api.deepseek.test/v1")
				input.ProviderConfig.Family = "deepseek"
				input.ProviderConfig.ReasoningProtocol = "deepseek-chat-completions"
				input.EndpointFormat = "chat_completions"
			},
			want: ErrAnthropicPrivateProtocolIncomplete,
		},
		{
			name: "DeepSeek disabled reasoning response",
			edit: func(input *AnthropicPrivateProtocolIssueInput) {
				input.ProviderConfig = privateProtocolFlowConfig("chat_completions", "https://api.deepseek.test/v1")
				input.ProviderConfig.Family = "deepseek"
				input.ProviderConfig.ReasoningProtocol = "deepseek-chat-completions"
				input.EndpointFormat = "chat_completions"
				input.Effort = "off"
				input.Signature = ""
			},
			want: ErrAnthropicPrivateProtocolIncomplete,
		},
		{
			name: "Anthropic disabled reasoning response",
			edit: func(input *AnthropicPrivateProtocolIssueInput) {
				input.ProviderConfig.ReasoningProtocol = ""
				input.Effort = "off"
			},
			want: ErrAnthropicPrivateProtocolIncomplete,
		},
		{
			name: "explicit unknown protocol private response",
			edit: func(input *AnthropicPrivateProtocolIssueInput) {
				input.ProviderConfig.ReasoningProtocol = "future-private-protocol"
			},
			want: ErrAnthropicPrivateProtocolFlowBinding,
		},
		{
			name: "custom non-messages signature",
			edit: func(input *AnthropicPrivateProtocolIssueInput) {
				input.ProviderConfig = privateProtocolFlowConfig("custom_endpoint", "https://custom.test/v1/chat/completions")
				input.EndpointFormat = "custom_endpoint"
				input.CustomRequestShape = "chat_completions"
			},
			want: ErrAnthropicPrivateProtocolEndpoint,
		},
		{
			name: "thinking without signature",
			edit: func(input *AnthropicPrivateProtocolIssueInput) { input.Signature = "" },
			want: ErrAnthropicPrivateProtocolIncomplete,
		},
		{
			name: "required response missing pair",
			edit: func(input *AnthropicPrivateProtocolIssueInput) {
				input.Thinking = ""
				input.Signature = ""
				input.ProviderConfig.ReasoningProtocol = "anthropic-thinking"
				input.Effort = "high"
			},
			want: ErrAnthropicPrivateProtocolMissing,
		},
		{
			name: "tool count mismatch",
			edit: func(input *AnthropicPrivateProtocolIssueInput) { input.ToolCallCount++ },
			want: ErrAnthropicPrivateProtocolFlowBinding,
		},
		{
			name: "assistant index mismatch",
			edit: func(input *AnthropicPrivateProtocolIssueInput) { input.AssistantMessageIndex++ },
			want: ErrAnthropicPrivateProtocolFlowBinding,
		},
		{
			name: "endpoint config mismatch",
			edit: func(input *AnthropicPrivateProtocolIssueInput) { input.EndpointFormat = "custom_endpoint" },
			want: ErrAnthropicPrivateProtocolFlowBinding,
		},
		{
			name: "historical private part",
			edit: func(input *AnthropicPrivateProtocolIssueInput) {
				input.Messages[0].Parts = []domainmodel.MessagePart{{Type: "reasoning", Text: "must not persist"}}
			},
			want: ErrAnthropicPrivateProtocolFlowBinding,
		},
		{
			name: "invalid context digest",
			edit: func(input *AnthropicPrivateProtocolIssueInput) { input.ContextDigest = "not-a-digest" },
			want: ErrAnthropicPrivateProtocolFlowBinding,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPrivateProtocolFlowFixture("messages", "", "https://api.anthropic.test/v1")
			test.edit(&fixture.issue)
			result, err := IssueAnthropicPrivateProtocol(fixture.issue)
			if test.want != nil {
				if !errors.Is(err, test.want) || result.Capsule != nil {
					t.Fatalf("fail-closed mismatch: result=%#v err=%v want=%v", result, err, test.want)
				}
				return
			}
			if err != nil || (test.wantNoCapsule && result.Capsule != nil) {
				t.Fatalf("non-anthropic reasoning classification mismatch: result=%#v err=%v", result, err)
			}
		})
	}
}

func TestConsumeAnthropicPrivateProtocolRejectsEndpointBeforeBurningCapsule(t *testing.T) {
	fixture := newPrivateProtocolFlowFixture("custom_endpoint", "messages", "https://custom.test/v1/messages")
	issued, err := IssueAnthropicPrivateProtocol(fixture.issue)
	if err != nil {
		t.Fatal(err)
	}
	messages := append([]domainmodel.Message(nil), fixture.issue.Messages...)
	messages = append(messages, fixture.issue.AssistantMessage, domainmodel.Message{
		Role: "tool", ToolCallID: fixture.issue.AssistantMessage.ToolCalls[0].ID, Content: "tool result",
	})
	consume := AnthropicPrivateProtocolConsumeInput{
		Session: issued.Session, Capsule: issued.Capsule, ProviderConfig: fixture.issue.ProviderConfig,
		EndpointFormat: "custom_endpoint", CustomRequestShape: "chat_completions",
		ContextDigest: fixture.issue.ContextDigest, PromptRoute: fixture.issue.PromptRoute,
		ToolManifestHash: fixture.issue.ToolManifestHash, Sequence: fixture.issue.Sequence, Messages: messages,
	}
	if _, err := ConsumeAnthropicPrivateProtocol(consume); !errors.Is(err, ErrAnthropicPrivateProtocolEndpoint) {
		t.Fatalf("incompatible endpoint did not fail closed: %v", err)
	}
	consume.CustomRequestShape = "messages"
	if _, err := ConsumeAnthropicPrivateProtocol(consume); err != nil {
		t.Fatalf("endpoint rejection consumed the capsule: %v", err)
	}
}

func TestAnthropicPrivateProtocolTerminalBoundary(t *testing.T) {
	fixture := newPrivateProtocolFlowFixture("messages", "", "https://api.anthropic.test/v1")
	issued, err := IssueAnthropicPrivateProtocol(fixture.issue)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		input    AnthropicPrivateProtocolTerminalInput
		want     string
		terminal bool
	}{
		{name: "inactive denial is ordinary flow", input: AnthropicPrivateProtocolTerminalInput{OutcomeCode: "approval_denied"}},
		{name: "capsule approval denial", input: AnthropicPrivateProtocolTerminalInput{Capsule: issued.Capsule, OutcomeCode: "approval_denied"}, want: AnthropicApprovalDeniedBoundary, terminal: true},
		{name: "observed restart approval denial", input: AnthropicPrivateProtocolTerminalInput{ProtocolObserved: true, OutcomeCode: "approval_denied"}, want: AnthropicApprovalDeniedBoundary, terminal: true},
		{name: "required approval denial", input: AnthropicPrivateProtocolTerminalInput{ReasoningProtocol: "anthropic-thinking", Effort: "high", OutcomeCode: "approval_denied"}, want: AnthropicApprovalDeniedBoundary, terminal: true},
		{name: "DeepSeek required approval denial", input: AnthropicPrivateProtocolTerminalInput{ReasoningProtocol: "deepseek-chat-completions", Effort: "high", OutcomeCode: "approval_denied"}, want: AnthropicApprovalDeniedBoundary, terminal: true},
		{name: "tool cancelled", input: AnthropicPrivateProtocolTerminalInput{Capsule: issued.Capsule, OutcomeCode: "tool_cancelled"}, want: AnthropicToolIncompleteBoundary, terminal: true},
		{name: "tool timeout", input: AnthropicPrivateProtocolTerminalInput{Capsule: issued.Capsule, OutcomeCode: "tool_timeout"}, want: AnthropicToolIncompleteBoundary, terminal: true},
		{name: "user input cancelled", input: AnthropicPrivateProtocolTerminalInput{Capsule: issued.Capsule, OutcomeCode: "user_input_cancelled"}, want: AnthropicToolIncompleteBoundary, terminal: true},
		{name: "input cancelled", input: AnthropicPrivateProtocolTerminalInput{Capsule: issued.Capsule, OutcomeCode: "input_cancelled"}, want: AnthropicToolIncompleteBoundary, terminal: true},
		{name: "ordinary tool failure", input: AnthropicPrivateProtocolTerminalInput{Capsule: issued.Capsule, OutcomeCode: "tool_failed"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			boundary, terminal := AnthropicPrivateProtocolTerminalBoundary(test.input)
			if boundary != test.want || terminal != test.terminal {
				t.Fatalf("terminal boundary mismatch: boundary=%q terminal=%t", boundary, terminal)
			}
		})
	}
}

type privateProtocolFlowFixture struct {
	issue AnthropicPrivateProtocolIssueInput
}

func newPrivateProtocolFlowFixture(endpointFormat, requestShape, baseURL string) privateProtocolFlowFixture {
	config := privateProtocolFlowConfig(endpointFormat, baseURL)
	assistant := domainmodel.Message{Role: "assistant", Content: "checking", ToolCalls: []domainmodel.ToolCall{{
		ID: "toolu_read", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`),
	}}}
	messages := []domainmodel.Message{{Role: "system", Content: "stable"}, {Role: "user", Content: "inspect"}}
	return privateProtocolFlowFixture{issue: AnthropicPrivateProtocolIssueInput{
		ProviderConfig: config, EndpointFormat: endpointFormat, CustomRequestShape: requestShape,
		ContextDigest: domainsecurity.SHA256Hex([]byte("context")), PromptRoute: "agent",
		ToolManifestHash: domainsecurity.SHA256Hex([]byte("tools")), Sequence: 2, Messages: messages,
		AssistantMessageIndex: len(messages), AssistantMessage: assistant, Thinking: "PRIVATE_THINKING",
		Signature: "PRIVATE_SIGNATURE", ToolCallCount: len(assistant.ToolCalls), Effort: "high",
	}}
}

func privateProtocolFlowConfig(endpointFormat, baseURL string) domainmodel.TurnConfig {
	return domainmodel.TurnConfig{
		ProviderID: "provider", Family: "anthropic", EndpointFormat: endpointFormat, BaseURL: baseURL,
		Model: "claude-test", ReasoningEffort: "high", ReasoningProtocol: "anthropic-thinking",
	}
}
