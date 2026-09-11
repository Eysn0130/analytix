package model

import (
	"encoding/json"
	"errors"
	"strings"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// ProviderContinuationRouteHash binds resumable work to the exact non-secret
// provider execution route. Credential rotation is allowed, but endpoint,
// protocol, model, modality, and request-shape drift fail closed.
func ProviderContinuationRouteHash(config domainmodel.TurnConfig) string {
	reasoningEffort, valid := domainmodel.ProjectReasoningEffortV1(config.ReasoningEffort)
	if !valid {
		return ""
	}
	value := struct {
		ProviderID                string   `json:"providerId"`
		Family                    string   `json:"family"`
		EndpointFormat            string   `json:"endpointFormat"`
		BaseURL                   string   `json:"baseUrl"`
		Model                     string   `json:"model"`
		ReasoningEffort           string   `json:"reasoningEffort"`
		ReasoningProtocol         string   `json:"reasoningProtocol"`
		CacheTelemetrySupported   bool     `json:"cacheTelemetrySupported"`
		DeepSeekPrefixEnhancement bool     `json:"deepSeekPrefixEnhancement"`
		SupportsImageInput        bool     `json:"supportsImageInput"`
		InputModalities           []string `json:"inputModalities"`
		MessageParts              []string `json:"messageParts"`
		ContextWindowTokens       int      `json:"contextWindowTokens"`
	}{
		ProviderID: strings.TrimSpace(config.ProviderID), Family: strings.TrimSpace(config.Family),
		EndpointFormat: strings.TrimSpace(config.EndpointFormat), BaseURL: strings.TrimSpace(config.BaseURL), Model: strings.TrimSpace(config.Model),
		ReasoningEffort: reasoningEffort, ReasoningProtocol: strings.TrimSpace(config.ReasoningProtocol),
		CacheTelemetrySupported: config.CacheTelemetrySupported, DeepSeekPrefixEnhancement: config.DeepSeekPrefixEnhancement,
		SupportsImageInput: config.SupportsImageInput, InputModalities: append([]string(nil), config.InputModalities...),
		MessageParts: append([]string(nil), config.MessageParts...), ContextWindowTokens: config.ContextWindowTokens,
	}
	body, err := json.Marshal(value)
	if err != nil || value.ProviderID == "" || value.Model == "" || value.EndpointFormat == "" || value.BaseURL == "" {
		return ""
	}
	return domainsecurity.SHA256Hex(body)
}

func ProviderContinuationCallRouteHash(config domainmodel.TurnConfig, promptRoute string, toolManifestHash string) string {
	promptRoute = strings.TrimSpace(promptRoute)
	toolManifestHash = strings.TrimSpace(toolManifestHash)
	providerRouteHash := ProviderContinuationRouteHash(config)
	if providerRouteHash == "" || promptRoute == "" || !domainsecurity.IsSHA256Hex(toolManifestHash) {
		return ""
	}
	value := struct {
		Version           string `json:"version"`
		ProviderRouteHash string `json:"providerRouteHash"`
		PromptRoute       string `json:"promptRoute"`
		ToolManifestHash  string `json:"toolManifestHash"`
	}{"provider_continuation_call_route_v1", providerRouteHash, promptRoute, toolManifestHash}
	body, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return domainsecurity.SHA256Hex(body)
}

// ProviderContinuationSafeShapeDigest describes the non-secret structure of a
// logical provider continuation. Private message
// text, tool-result bytes, images, reasoning text, and reasoning signatures are
// deliberately excluded. This digest is diagnostic only; it is not payload
// authority because low-entropy private content must not be publicly guessable.
func ProviderContinuationSafeShapeDigest(messages []domainmodel.Message, toolManifestHash string, registryStateDigest string, sequence uint64) string {
	type safePart struct {
		Type           string `json:"type"`
		ContentPresent bool   `json:"contentPresent"`
	}
	type safeToolCall struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		ArgsHash string `json:"argsHash"`
	}
	type safeMessage struct {
		Role           string         `json:"role"`
		Name           string         `json:"name"`
		ToolCallID     string         `json:"toolCallId"`
		ContentPresent bool           `json:"contentPresent"`
		Parts          []safePart     `json:"parts"`
		ToolCalls      []safeToolCall `json:"toolCalls"`
	}

	safeMessages := make([]safeMessage, 0, len(messages))
	for _, message := range messages {
		safe := safeMessage{
			Role:           strings.TrimSpace(message.Role),
			Name:           strings.TrimSpace(message.Name),
			ToolCallID:     strings.TrimSpace(message.ToolCallID),
			ContentPresent: message.Content != "",
			Parts:          make([]safePart, 0, len(message.Parts)),
			ToolCalls:      make([]safeToolCall, 0, len(message.ToolCalls)),
		}
		for _, part := range message.Parts {
			partType := strings.ToLower(strings.TrimSpace(part.Type))
			if partType == "reasoning" || partType == "thinking" {
				partType = "private_protocol_part"
			}
			safe.Parts = append(safe.Parts, safePart{
				Type:           partType,
				ContentPresent: part.Text != "" || part.ImageURL != "" || part.Data != "" || part.Signature != "",
			})
		}
		for _, call := range message.ToolCalls {
			safe.ToolCalls = append(safe.ToolCalls, safeToolCall{
				ID:       strings.TrimSpace(call.ID),
				Name:     strings.TrimSpace(call.Name),
				ArgsHash: domainsecurity.CanonicalJSONHash(call.Arguments),
			})
		}
		safeMessages = append(safeMessages, safe)
	}
	value := struct {
		Version             string        `json:"version"`
		Messages            []safeMessage `json:"messages"`
		ToolManifestHash    string        `json:"toolManifestHash"`
		RegistryStateDigest string        `json:"registryStateDigest"`
		Sequence            uint64        `json:"sequence"`
	}{
		Version:             "provider_continuation_payload_digest_v1",
		Messages:            safeMessages,
		ToolManifestHash:    strings.TrimSpace(toolManifestHash),
		RegistryStateDigest: strings.TrimSpace(registryStateDigest),
		Sequence:            sequence,
	}
	body, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return domainsecurity.SHA256Hex(body)
}

// ProviderContinuationPayloadBytes returns the exact semantic request material
// for an in-memory, secret-keyed digest. Callers must never persist or expose
// these bytes; only a keyed digest produced by the private host authority may
// enter PendingWorkReceiptV1.
func ProviderContinuationPayloadBytes(systemPrompt string, messages []domainmodel.Message, attachmentPlanDigest string, toolManifestHash string, registryStateDigest string, sequence uint64) ([]byte, error) {
	attachmentPlanDigest = strings.TrimSpace(attachmentPlanDigest)
	toolManifestHash = strings.TrimSpace(toolManifestHash)
	registryStateDigest = strings.TrimSpace(registryStateDigest)
	if (attachmentPlanDigest != "" && !domainsecurity.IsSHA256Hex(attachmentPlanDigest)) ||
		!domainsecurity.IsSHA256Hex(toolManifestHash) || !domainsecurity.IsSHA256Hex(registryStateDigest) || sequence == 0 {
		return nil, errors.New("provider continuation payload authority is invalid")
	}
	value := struct {
		Version              string                `json:"version"`
		SystemPrompt         string                `json:"systemPrompt"`
		Messages             []domainmodel.Message `json:"messages"`
		AttachmentPlanDigest string                `json:"attachmentPlanDigest,omitempty"`
		ToolManifestHash     string                `json:"toolManifestHash"`
		RegistryStateDigest  string                `json:"registryStateDigest"`
		Sequence             uint64                `json:"sequence"`
	}{
		Version:              "provider_continuation_payload_v1",
		SystemPrompt:         systemPrompt,
		Messages:             append([]domainmodel.Message(nil), messages...),
		AttachmentPlanDigest: attachmentPlanDigest,
		ToolManifestHash:     toolManifestHash,
		RegistryStateDigest:  registryStateDigest,
		Sequence:             sequence,
	}
	body, err := json.Marshal(value)
	if err != nil || len(body) == 0 {
		return nil, errors.New("provider continuation payload is not canonical")
	}
	canonicalValue, err := domainjsonstrict.DecodeValue(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 16 * 1024 * 1024, MaxDepth: 128, MaxTokens: 1_000_000, MaxStringBytes: 16 * 1024 * 1024,
	})
	if err != nil {
		return nil, errors.New("provider continuation payload is not canonical")
	}
	canonical, err := json.Marshal(canonicalValue)
	if err != nil || len(canonical) == 0 {
		return nil, errors.New("provider continuation payload is not canonical")
	}
	return canonical, nil
}
