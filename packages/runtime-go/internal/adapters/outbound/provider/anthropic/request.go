package anthropic

import (
	"errors"
	"strings"

	providerschema "analytix.local/runtime-go/internal/adapters/outbound/provider/schema"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func MessagesBody(request domainmodel.Request) (map[string]any, error) {
	if err := domainmodel.ValidateNoHistoricalPrivateProtocolParts(request.Messages); err != nil {
		return nil, err
	}
	messages := make([]map[string]any, 0, len(request.Messages))
	deepSeek := providerschema.NormalizeReasoningProtocol(request.ReasoningProtocol) == "deepseek-messages"
	if deepSeek {
		if err := validateDeepSeekMessagesHistory(request.Messages); err != nil {
			return nil, err
		}
	}
	system := []map[string]any{}
	replays := request.DeepSeekReasoningReplays
	if len(replays) == 0 && request.AnthropicThinkingReplay != nil {
		replays = []*domainmodel.AnthropicThinkingReplay{request.AnthropicThinkingReplay}
	}
	replayApplied := make([]bool, len(replays))
	conversationStarted := false
	for index, item := range request.Messages {
		if deepSeek {
			switch item.Role {
			case "system":
				if conversationStarted {
					return nil, errors.New("Messages baseline cannot encode an in-history system update")
				}
			case "user", "assistant", "tool":
				conversationStarted = true
			default:
				return nil, errors.New("Messages baseline role is unsupported")
			}
		}
		if item.Role == "system" {
			system = append(system, map[string]any{"type": "text", "text": item.Content})
			continue
		}
		blocks := providerschema.AnthropicMessageContent(item)
		for replayIndex, replay := range replays {
			if thinking, signature, ok := replay.ThinkingBlock(index, item); ok {
				block := map[string]any{"type": "thinking", "thinking": thinking}
				if signature != "" {
					block["signature"] = signature
				}
				blocks = append([]map[string]any{block}, blocks...)
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
		if !deepSeek {
			system[len(system)-1]["cache_control"] = map[string]any{"type": "ephemeral"}
		}
		body["system"] = system
	}
	if len(request.Tools) > 0 {
		body["tools"] = providerschema.AnthropicTools(request.Tools, !deepSeek && len(system) == 0)
	}
	if !deepSeek {
		providerschema.ApplyAnthropicCacheControlToLastMessage(messages)
	}
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
	protocol := providerschema.NormalizeReasoningProtocol(request.ReasoningProtocol)
	if protocol != "anthropic-thinking" && protocol != "deepseek-messages" {
		return
	}
	effort := providerschema.NormalizeRequestReasoningEffort(request.ReasoningEffort)
	if effort == "" || (protocol == "deepseek-messages" && effort == "auto") {
		return
	}
	if effort == "off" {
		body["thinking"] = map[string]any{"type": "disabled"}
		return
	}
	if protocol == "deepseek-messages" {
		body["thinking"] = map[string]any{"type": "enabled"}
		if effort == "medium" {
			effort = "high"
		}
	} else {
		body["thinking"] = map[string]any{"type": "adaptive"}
	}
	if effort == "low" || effort == "medium" || effort == "high" || effort == "max" {
		body["output_config"] = map[string]any{"effort": effort}
	}
}

// Public Messages has no authority to invent missing results, silently replace
// malformed tool JSON with {}, or treat a new system snapshot as user content.
func validateDeepSeekMessagesHistory(messages []domainmodel.Message) error {
	pending, seen := map[string]bool{}, map[string]bool{}
	for _, message := range messages {
		if message.Role == "tool" {
			if !pending[message.ToolCallID] || len(message.ToolCalls) != 0 {
				return errors.New("Messages tool result pairing is invalid")
			}
			delete(pending, message.ToolCallID)
			continue
		}
		if len(pending) != 0 {
			return errors.New("Messages tool results must precede the next message")
		}
		if len(message.ToolCalls) > 0 && message.Role != "assistant" {
			return errors.New("Messages tool call role is invalid")
		}
		for _, call := range message.ToolCalls {
			if strings.TrimSpace(call.ID) == "" || strings.TrimSpace(call.Name) == "" || seen[call.ID] {
				return errors.New("Messages tool identity is invalid")
			}
			if domainjsonstrict.Validate(call.Arguments, domainjsonstrict.Options{RequireObject: true}) != nil {
				return errors.New("Messages tool arguments are invalid")
			}
			pending[call.ID], seen[call.ID] = true, true
		}
	}
	if len(pending) != 0 {
		return errors.New("Messages tool result is missing")
	}
	return nil
}
