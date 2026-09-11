package schema

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func CanonicalJSONSchema(body json.RawMessage) string {
	return domainmodel.CanonicalProviderJSONSchema(body)
}

func NormalizeReasoningProtocol(value string) string {
	normalized := strings.Trim(strings.ToLower(strings.TrimSpace(value)), "/")
	normalized = strings.ReplaceAll(normalized, "_", "-")
	switch normalized {
	case "none", "deepseek-chat-completions", "glm-chat-completions", "mimo-chat-completions", "openai-responses", "anthropic-thinking":
		return normalized
	default:
		return ""
	}
}

func NormalizeRequestReasoningEffort(value string) string {
	projected, valid := domainmodel.ProjectReasoningEffortV1(value)
	if !valid {
		return ""
	}
	return projected
}

func RequestUsesDeepSeekReasoning(request domainmodel.Request) bool {
	return NormalizeReasoningProtocol(request.ReasoningProtocol) == "deepseek-chat-completions" &&
		NormalizeRequestReasoningEffort(request.ReasoningEffort) != "off"
}

func CanonicalProviderTools(tools []domainmodel.ToolSchema) []domainmodel.ToolSchema {
	if len(tools) == 0 {
		return nil
	}
	out := make([]domainmodel.ToolSchema, len(tools))
	copy(out, tools)
	for index := range out {
		out[index].Name = strings.TrimSpace(out[index].Name)
		out[index].Description = strings.TrimSpace(out[index].Description)
		out[index].Parameters = json.RawMessage(CanonicalJSONSchema(out[index].Parameters))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		if out[i].Description != out[j].Description {
			return out[i].Description < out[j].Description
		}
		return string(out[i].Parameters) < string(out[j].Parameters)
	})
	return out
}

func OpenAITools(tools []domainmodel.ToolSchema) []map[string]any {
	tools = CanonicalProviderTools(tools)
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		params := tool.Parameters
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  json.RawMessage(CanonicalJSONSchema(params)),
			},
		})
	}
	return out
}

func AnthropicTools(tools []domainmodel.ToolSchema, markCacheControl bool) []map[string]any {
	tools = CanonicalProviderTools(tools)
	out := make([]map[string]any, 0, len(tools))
	for index, tool := range tools {
		params := tool.Parameters
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
		}
		item := map[string]any{
			"name":         tool.Name,
			"description":  tool.Description,
			"input_schema": json.RawMessage(CanonicalJSONSchema(params)),
		}
		if markCacheControl && index == len(tools)-1 {
			item["cache_control"] = map[string]any{
				"type": "ephemeral",
			}
		}
		out = append(out, item)
	}
	return out
}

func ApplyAnthropicCacheControlToLastMessage(messages []map[string]any) bool {
	for index := len(messages) - 1; index >= 0; index-- {
		content, ok := messages[index]["content"].([]map[string]any)
		if !ok || len(content) == 0 {
			continue
		}
		content[len(content)-1]["cache_control"] = map[string]any{"type": "ephemeral"}
		return true
	}
	return false
}

func OpenAIChatMessages(
	messages []domainmodel.Message,
	replays []*domainmodel.AnthropicThinkingReplay,
	requireDeepSeekReasoning bool,
) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(messages))
	replayApplied := make([]bool, len(replays))
	for index, message := range messages {
		item := map[string]any{"role": message.Role}
		if message.Role == "tool" {
			item["tool_call_id"] = message.ToolCallID
			item["content"] = message.Content
		} else if len(message.Parts) == 0 {
			item["content"] = message.Content
		} else {
			item["content"] = OpenAIContentParts(message)
		}
		if len(message.ToolCalls) > 0 {
			if message.Role == "assistant" && !MessageHasOpenAIVisibleContent(message) {
				item["content"] = nil
			}
			item["tool_calls"] = OpenAIMessageToolCalls(message.ToolCalls)
			matchedReplay := -1
			for replayIndex, replay := range replays {
				if reasoning, ok := replay.ReasoningContent(index, message); ok {
					if matchedReplay >= 0 {
						return nil, errors.New("DeepSeek private protocol replay is ambiguous")
					}
					item["reasoning_content"] = reasoning
					matchedReplay = replayIndex
				}
			}
			if matchedReplay >= 0 {
				replayApplied[matchedReplay] = true
			} else if requireDeepSeekReasoning {
				return nil, errors.New("DeepSeek assistant tool history is missing exact private reasoning")
			}
		}
		out = append(out, item)
	}
	for _, applied := range replayApplied {
		if !applied {
			return nil, errors.New("DeepSeek private protocol replay does not match request history")
		}
	}
	return out, nil
}

func MessageHasOpenAIVisibleContent(message domainmodel.Message) bool {
	if strings.TrimSpace(message.Content) != "" {
		return true
	}
	for _, part := range message.Parts {
		if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
			return true
		}
		if part.Type == "image" || part.Type == "image_url" || part.Type == "input_image" {
			return true
		}
	}
	return false
}

func OpenAIMessageToolCalls(calls []domainmodel.ToolCall) []map[string]any {
	out := make([]map[string]any, 0, len(calls))
	for _, call := range calls {
		args := string(call.Arguments)
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		out = append(out, map[string]any{
			"id":   call.ID,
			"type": "function",
			"function": map[string]any{
				"name":      call.Name,
				"arguments": args,
			},
		})
	}
	return out
}

func OpenAIContentParts(message domainmodel.Message) []map[string]any {
	parts := make([]map[string]any, 0, len(message.Parts)+1)
	if strings.TrimSpace(message.Content) != "" {
		parts = append(parts, map[string]any{"type": "text", "text": message.Content})
	}
	for _, part := range message.Parts {
		switch part.Type {
		case "text":
			if strings.TrimSpace(part.Text) != "" {
				parts = append(parts, map[string]any{"type": "text", "text": part.Text})
			}
		case "image":
			imageURL := strings.TrimSpace(part.ImageURL)
			if imageURL == "" && part.Data != "" && part.MediaType != "" {
				imageURL = "data:" + part.MediaType + ";base64," + part.Data
			}
			if imageURL != "" {
				parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageURL}})
			}
		}
	}
	return parts
}

func ResponseContentParts(message domainmodel.Message) []map[string]any {
	parts := []map[string]any{}
	if strings.TrimSpace(message.Content) != "" {
		parts = append(parts, map[string]any{"type": "input_text", "text": message.Content})
	}
	for _, part := range message.Parts {
		switch part.Type {
		case "text":
			if strings.TrimSpace(part.Text) != "" {
				parts = append(parts, map[string]any{"type": "input_text", "text": part.Text})
			}
		case "image":
			imageURL := strings.TrimSpace(part.ImageURL)
			if imageURL == "" && part.Data != "" && part.MediaType != "" {
				imageURL = "data:" + part.MediaType + ";base64," + part.Data
			}
			if imageURL != "" {
				parts = append(parts, map[string]any{"type": "input_image", "image_url": imageURL})
			}
		}
	}
	return parts
}

func AnthropicContentParts(message domainmodel.Message) []map[string]any {
	parts := []map[string]any{}
	if strings.TrimSpace(message.Content) != "" {
		parts = append(parts, map[string]any{"type": "text", "text": message.Content})
	}
	for _, part := range message.Parts {
		switch part.Type {
		case "text":
			if strings.TrimSpace(part.Text) != "" {
				parts = append(parts, map[string]any{"type": "text", "text": part.Text})
			}
		case "image":
			if part.Data != "" && part.MediaType != "" {
				parts = append(parts, map[string]any{
					"type": "image",
					"source": map[string]any{
						"type":       "base64",
						"media_type": part.MediaType,
						"data":       part.Data,
					},
				})
			}
		}
	}
	return parts
}

func AnthropicMessageContent(message domainmodel.Message) []map[string]any {
	if message.Role == "tool" {
		return []map[string]any{{
			"type":        "tool_result",
			"tool_use_id": message.ToolCallID,
			"content":     AnthropicToolResultContent(message.Content),
		}}
	}
	parts := AnthropicContentParts(message)
	for _, call := range message.ToolCalls {
		input := map[string]any{}
		if len(call.Arguments) > 0 {
			_ = json.Unmarshal(call.Arguments, &input)
		}
		parts = append(parts, map[string]any{
			"type":  "tool_use",
			"id":    call.ID,
			"name":  call.Name,
			"input": input,
		})
	}
	return parts
}

func AnthropicToolResultContent(content string) string {
	if strings.TrimSpace(content) == "" {
		return "(no output)"
	}
	return content
}
