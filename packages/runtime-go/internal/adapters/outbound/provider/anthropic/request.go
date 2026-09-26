package anthropic

import (
	"errors"

	providerschema "analytix.local/runtime-go/internal/adapters/outbound/provider/schema"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func MessagesBody(request domainmodel.Request) (map[string]any, error) {
	if err := domainmodel.ValidateNoHistoricalPrivateProtocolParts(request.Messages); err != nil {
		return nil, err
	}
	messages := make([]map[string]any, 0, len(request.Messages))
	system := []map[string]any{}
	replays := request.DeepSeekReasoningReplays
	if len(replays) == 0 && request.AnthropicThinkingReplay != nil {
		replays = []*domainmodel.AnthropicThinkingReplay{request.AnthropicThinkingReplay}
	}
	replayApplied := make([]bool, len(replays))
	for index, item := range request.Messages {
		if item.Role == "system" {
			system = append(system, map[string]any{"type": "text", "text": item.Content})
			continue
		}
		blocks := providerschema.AnthropicMessageContent(item)
		for replayIndex, replay := range replays {
			if thinking, signature, ok := replay.ThinkingBlock(index, item); ok {
				blocks = append([]map[string]any{{
					"type": "thinking", "thinking": thinking, "signature": signature,
				}}, blocks...)
				replayApplied[replayIndex] = true
			}
		}
		appendAnthropicMessageBlocks(&messages, anthropicMessageRole(item.Role), blocks)
	}
	for _, applied := range replayApplied {
		if !applied {
			return nil, errors.New("anthropic private protocol replay does not match request history")
		}
	}
	maxTokens := 4096
	if request.MaxOutputTokens > 0 {
		maxTokens = request.MaxOutputTokens
	}
	body := map[string]any{
		"model":      request.Model,
		"max_tokens": maxTokens,
		"messages":   messages,
		"stream":     true,
	}
	if len(system) > 0 {
		system[len(system)-1]["cache_control"] = map[string]any{"type": "ephemeral"}
		body["system"] = system
	}
	if len(request.Tools) > 0 {
		body["tools"] = providerschema.AnthropicTools(request.Tools, len(system) == 0)
	}
	providerschema.ApplyAnthropicCacheControlToLastMessage(messages)
	ApplyReasoning(body, request)
	return body, nil
}

func anthropicMessageRole(role string) string {
	if role == "assistant" {
		return "assistant"
	}
	return "user"
}

func appendAnthropicMessageBlocks(messages *[]map[string]any, role string, blocks []map[string]any) {
	if len(blocks) == 0 {
		return
	}
	lastIndex := len(*messages) - 1
	if lastIndex >= 0 && (*messages)[lastIndex]["role"] == role {
		if existing, ok := (*messages)[lastIndex]["content"].([]map[string]any); ok {
			(*messages)[lastIndex]["content"] = append(existing, blocks...)
			return
		}
	}
	*messages = append(*messages, map[string]any{
		"role":    role,
		"content": blocks,
	})
}

func StreamHeaders(apiKey string, includeBearer bool) map[string]string {
	headers := map[string]string{
		"Content-Type":      "application/json",
		"Accept":            "text/event-stream",
		"x-api-key":         apiKey,
		"anthropic-version": "2023-06-01",
	}
	if includeBearer {
		headers["Authorization"] = "Bearer " + apiKey
	}
	return headers
}

func ApplyReasoning(body map[string]any, request domainmodel.Request) {
	if providerschema.NormalizeReasoningProtocol(request.ReasoningProtocol) != "anthropic-thinking" {
		return
	}
	effort := providerschema.NormalizeRequestReasoningEffort(request.ReasoningEffort)
	if effort == "" {
		return
	}
	if effort == "off" {
		body["thinking"] = map[string]any{"type": "disabled"}
		return
	}
	body["thinking"] = map[string]any{"type": "adaptive"}
	if effort == "low" || effort == "medium" || effort == "high" || effort == "max" {
		body["output_config"] = map[string]any{"effort": effort}
	}
}
