package model

import (
	"errors"
	"strings"

	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	AnthropicApprovalDeniedBoundary = "Tool execution was denied. No further model continuation was run."
	AnthropicToolIncompleteBoundary = "Tool execution did not complete. No further model continuation was run."
)

var (
	ErrAnthropicPrivateProtocolEndpoint    = errors.New("anthropic private protocol continuation is routed to an incompatible endpoint")
	ErrAnthropicPrivateProtocolIncomplete  = errors.New("anthropic private protocol response is incomplete or routed to an incompatible endpoint")
	ErrAnthropicPrivateProtocolMissing     = errors.New("anthropic private protocol response is missing signed thinking")
	ErrAnthropicPrivateProtocolFlowBinding = errors.New("anthropic private protocol flow binding is invalid")
	ErrAnthropicPrivateProtocolSession     = errors.New("anthropic private protocol session is unavailable")
)

type privateProtocolResponseFailure struct{ cause error }

func (failure privateProtocolResponseFailure) Error() string {
	return failure.PublicFailureRecord().Message()
}

func (failure privateProtocolResponseFailure) Unwrap() error { return failure.cause }

func (privateProtocolResponseFailure) ProviderRetryForbidden() bool { return true }

func (privateProtocolResponseFailure) PublicFailureRecord() domainfailure.Record {
	return domainfailure.New(domainfailure.CodeProviderReasoningMarkupInvalid, nil)
}

func newPrivateProtocolResponseFailure(cause error) error {
	if cause == nil {
		return nil
	}
	return privateProtocolResponseFailure{cause: cause}
}

type AnthropicPrivateProtocolConsumeInput struct {
	Session            *domainmodel.PrivateProtocolSession
	Capsule            *domainmodel.AnthropicThinkingCapsule
	ProviderConfig     domainmodel.TurnConfig
	EndpointFormat     string
	CustomRequestShape string
	ContextDigest      string
	PromptRoute        string
	ToolManifestHash   string
	Sequence           uint64
	Messages           []domainmodel.Message
}

type AnthropicPrivateProtocolIssueInput struct {
	Session               *domainmodel.PrivateProtocolSession
	ProviderConfig        domainmodel.TurnConfig
	EndpointFormat        string
	CustomRequestShape    string
	ContextDigest         string
	PromptRoute           string
	ToolManifestHash      string
	Sequence              uint64
	Messages              []domainmodel.Message
	AssistantMessageIndex int
	AssistantMessage      domainmodel.Message
	Thinking              string
	Signature             string
	ToolCallCount         int
	Effort                string
}

type AnthropicPrivateProtocolIssueResult struct {
	Session *domainmodel.PrivateProtocolSession
	Capsule *domainmodel.AnthropicThinkingCapsule
}

type PrivateProtocolResponseObservationInput struct {
	ProviderConfig     domainmodel.TurnConfig
	EndpointFormat     string
	CustomRequestShape string
	Thinking           string
	Signature          string
	Effort             string
}

type AnthropicPrivateProtocolTerminalInput struct {
	Capsule           *domainmodel.AnthropicThinkingCapsule
	ProtocolObserved  bool
	ReasoningProtocol string
	Effort            string
	OutcomeCode       string
}

func AnthropicPrivateProtocolEndpointCapable(endpointFormat, customRequestShape string) bool {
	endpointFormat = strings.TrimSpace(endpointFormat)
	customRequestShape = strings.TrimSpace(customRequestShape)
	return endpointFormat == "messages" ||
		(endpointFormat == "custom_endpoint" && customRequestShape == "messages")
}

func DeepSeekPrivateProtocolEndpointCapable(endpointFormat, customRequestShape string) bool {
	endpointFormat = strings.TrimSpace(endpointFormat)
	customRequestShape = strings.TrimSpace(customRequestShape)
	return endpointFormat == "chat_completions" ||
		(endpointFormat == "custom_endpoint" && customRequestShape == "chat_completions")
}

func PrivateProtocolRequired(reasoningProtocol, effort string) bool {
	effort = strings.ToLower(strings.TrimSpace(effort))
	protocol := privateProtocolReasoningKind(reasoningProtocol)
	switch protocol {
	case "anthropic-thinking":
		return effort != "" && effort != "off"
	case "deepseek-chat-completions":
		// DeepSeek thinking defaults to enabled when the caller omits an
		// explicit effort/toggle, so only an explicit off is non-private.
		return effort != "off"
	default:
		return false
	}
}

func AnthropicPrivateProtocolRequired(reasoningProtocol, effort string) bool {
	return PrivateProtocolRequired(reasoningProtocol, effort)
}

// ObservePrivateProtocolResponse validates provider-private response fields
// without retaining or projecting their bytes. Callers use the returned
// host-owned fact when a response is converted into a synthetic tool action
// instead of going through the ordinary response-derived capsule path.
func ObservePrivateProtocolResponse(input PrivateProtocolResponseObservationInput) (bool, error) {
	protocol := privateProtocolReasoningKindForProvider(
		input.ProviderConfig,
		input.EndpointFormat,
		input.CustomRequestShape,
	)
	capable := privateProtocolEndpointCapable(protocol, input.EndpointFormat, input.CustomRequestShape)
	compatible := privateProtocolProviderCompatible(input.ProviderConfig, protocol)
	thinkingPresent := strings.TrimSpace(input.Thinking) != ""
	signaturePresent := strings.TrimSpace(input.Signature) != ""
	observed := thinkingPresent || signaturePresent
	if observed && protocol == "" {
		return false, newPrivateProtocolResponseFailure(ErrAnthropicPrivateProtocolFlowBinding)
	}
	if observed &&
		((protocol == "anthropic-thinking" &&
			strings.EqualFold(strings.TrimSpace(input.Effort), "off")) ||
			(protocol == "deepseek-chat-completions" &&
				strings.EqualFold(strings.TrimSpace(input.Effort), "off"))) {
		return false, newPrivateProtocolResponseFailure(ErrAnthropicPrivateProtocolIncomplete)
	}
	if observed && (!capable || !compatible) {
		return false, newPrivateProtocolResponseFailure(ErrAnthropicPrivateProtocolEndpoint)
	}
	if observed {
		if (protocol == "anthropic-thinking" && !signaturePresent) ||
			(protocol == "deepseek-chat-completions" && !thinkingPresent) ||
			(protocol == "deepseek-chat-completions" && signaturePresent) {
			return false, newPrivateProtocolResponseFailure(ErrAnthropicPrivateProtocolIncomplete)
		}
		return true, nil
	}
	if PrivateProtocolRequired(input.ProviderConfig.ReasoningProtocol, input.Effort) {
		return false, newPrivateProtocolResponseFailure(ErrAnthropicPrivateProtocolMissing)
	}
	return false, nil
}

func ConsumeAnthropicPrivateProtocol(input AnthropicPrivateProtocolConsumeInput) (*domainmodel.AnthropicThinkingReplay, error) {
	if input.Session == nil || input.Capsule == nil {
		return nil, ErrAnthropicPrivateProtocolSession
	}
	providerRouteHash, err := privateProtocolProviderRouteHash(input.ProviderConfig, input.EndpointFormat)
	if err != nil {
		return nil, err
	}
	protocol := privateProtocolReasoningKindForProvider(
		input.ProviderConfig,
		input.EndpointFormat,
		input.CustomRequestShape,
	)
	if !privateProtocolEndpointCapable(protocol, input.EndpointFormat, input.CustomRequestShape) ||
		!privateProtocolProviderCompatible(input.ProviderConfig, protocol) {
		return nil, ErrAnthropicPrivateProtocolEndpoint
	}
	if err := validatePrivateProtocolCallBinding(input.ContextDigest, input.PromptRoute, input.ToolManifestHash, input.Sequence, input.Messages); err != nil {
		return nil, err
	}
	return input.Session.ConsumeAnthropicThinking(input.Capsule, domainmodel.AnthropicThinkingConsumeInput{
		Protocol:      protocol,
		ContextDigest: input.ContextDigest, ProviderRouteHash: providerRouteHash,
		PromptRoute: input.PromptRoute, ToolManifestHash: input.ToolManifestHash,
		Sequence: input.Sequence, Messages: input.Messages,
	})
}

func IssueAnthropicPrivateProtocol(input AnthropicPrivateProtocolIssueInput) (AnthropicPrivateProtocolIssueResult, error) {
	result := AnthropicPrivateProtocolIssueResult{Session: input.Session}
	providerRouteHash, err := privateProtocolProviderRouteHash(input.ProviderConfig, input.EndpointFormat)
	if err != nil {
		return result, err
	}
	if err := validatePrivateProtocolCallBinding(input.ContextDigest, input.PromptRoute, input.ToolManifestHash, input.Sequence, input.Messages); err != nil {
		return result, err
	}
	if input.ToolCallCount <= 0 || input.ToolCallCount != len(input.AssistantMessage.ToolCalls) ||
		input.AssistantMessageIndex != len(input.Messages) {
		return result, ErrAnthropicPrivateProtocolFlowBinding
	}

	protocol := privateProtocolReasoningKindForProvider(
		input.ProviderConfig,
		input.EndpointFormat,
		input.CustomRequestShape,
	)
	observed, err := ObservePrivateProtocolResponse(PrivateProtocolResponseObservationInput{
		ProviderConfig: input.ProviderConfig, EndpointFormat: input.EndpointFormat,
		CustomRequestShape: input.CustomRequestShape, Thinking: input.Thinking,
		Signature: input.Signature, Effort: input.Effort,
	})
	if err != nil {
		return result, err
	}
	if observed {
		session := input.Session
		if session == nil {
			session, err = domainmodel.NewPrivateProtocolSession()
			if err != nil {
				return result, ErrAnthropicPrivateProtocolSession
			}
		}
		capsule, issueErr := session.IssueAnthropicThinking(domainmodel.AnthropicThinkingCapsuleInput{
			Protocol:      protocol,
			ContextDigest: input.ContextDigest, ProviderRouteHash: providerRouteHash,
			PromptRoute: input.PromptRoute, ToolManifestHash: input.ToolManifestHash,
			Sequence: input.Sequence, AssistantMessageIndex: input.AssistantMessageIndex,
			PrefixMessages: input.Messages, AssistantMessage: input.AssistantMessage,
			Thinking: input.Thinking, Signature: input.Signature,
		})
		if issueErr != nil {
			return result, issueErr
		}
		return AnthropicPrivateProtocolIssueResult{Session: session, Capsule: capsule}, nil
	}
	return result, nil
}

func privateProtocolProviderCompatible(config domainmodel.TurnConfig, protocol string) bool {
	family := strings.ToLower(strings.TrimSpace(config.Family))
	switch protocol {
	case "anthropic-thinking":
		return family != "deepseek"
	case "deepseek-chat-completions":
		return family != "anthropic" && family != "anthropic-compatible"
	default:
		return false
	}
}

func AnthropicPrivateProtocolTerminalBoundary(input AnthropicPrivateProtocolTerminalInput) (string, bool) {
	if input.Capsule == nil && !input.ProtocolObserved &&
		!PrivateProtocolRequired(input.ReasoningProtocol, input.Effort) {
		return "", false
	}
	switch strings.TrimSpace(input.OutcomeCode) {
	case "approval_denied":
		return AnthropicApprovalDeniedBoundary, true
	case "tool_cancelled", "tool_timeout", "user_input_cancelled", "input_cancelled":
		return AnthropicToolIncompleteBoundary, true
	default:
		return "", false
	}
}

func privateProtocolEndpointCapable(protocol, endpointFormat, customRequestShape string) bool {
	switch protocol {
	case "anthropic-thinking":
		return AnthropicPrivateProtocolEndpointCapable(endpointFormat, customRequestShape)
	case "deepseek-chat-completions":
		return DeepSeekPrivateProtocolEndpointCapable(endpointFormat, customRequestShape)
	default:
		return false
	}
}

func privateProtocolReasoningKind(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "anthropic-thinking":
		return "anthropic-thinking"
	case "deepseek-chat-completions":
		return "deepseek-chat-completions"
	default:
		return ""
	}
}

func privateProtocolReasoningKindForProvider(
	config domainmodel.TurnConfig,
	endpointFormat string,
	customRequestShape string,
) string {
	configuredProtocol := strings.TrimSpace(config.ReasoningProtocol)
	if configuredProtocol != "" {
		return privateProtocolReasoningKind(configuredProtocol)
	}
	// Anthropic-compatible message endpoints historically returned signed
	// thinking before model profiles exposed an explicit protocol selector, so
	// only a truly absent selector may use the legacy endpoint inference. An
	// explicit unknown selector must remain unknown and fail closed if private
	// bytes arrive.
	if AnthropicPrivateProtocolEndpointCapable(endpointFormat, customRequestShape) &&
		privateProtocolProviderCompatible(config, "anthropic-thinking") {
		return "anthropic-thinking"
	}
	return ""
}

// PrivateProtocolReasoningKindForProvider returns the effective volatile
// provider-protocol kind for the exact configured endpoint. It includes the
// legacy Anthropic messages inference used by response issuance, so restart
// and history-recompilation paths make the same decision.
func PrivateProtocolReasoningKindForProvider(
	config domainmodel.TurnConfig,
	endpointFormat string,
	customRequestShape string,
) string {
	return privateProtocolReasoningKindForProvider(config, endpointFormat, customRequestShape)
}

func privateProtocolProviderRouteHash(config domainmodel.TurnConfig, endpointFormat string) (string, error) {
	endpointFormat = strings.TrimSpace(endpointFormat)
	if endpointFormat == "" || endpointFormat != strings.TrimSpace(config.EndpointFormat) {
		return "", ErrAnthropicPrivateProtocolFlowBinding
	}
	hash := ProviderContinuationRouteHash(config)
	if !domainsecurity.IsSHA256Hex(hash) {
		return "", ErrAnthropicPrivateProtocolFlowBinding
	}
	return hash, nil
}

func validatePrivateProtocolCallBinding(contextDigest, promptRoute, toolManifestHash string, sequence uint64, messages []domainmodel.Message) error {
	if !domainsecurity.IsSHA256Hex(strings.TrimSpace(contextDigest)) || strings.TrimSpace(promptRoute) == "" ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(toolManifestHash)) || sequence == 0 ||
		domainmodel.ValidateNoHistoricalPrivateProtocolParts(messages) != nil {
		return ErrAnthropicPrivateProtocolFlowBinding
	}
	return nil
}
