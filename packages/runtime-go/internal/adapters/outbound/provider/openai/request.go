package openai

import (
	"errors"

	providerschema "analytix.local/runtime-go/internal/adapters/outbound/provider/schema"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func ChatCompletionsBody(request domainmodel.Request) (map[string]any, error) {
	if request.AnthropicThinkingReplay != nil {
		return nil, errors.New("Anthropic private protocol replay is incompatible with chat completions")
	}
	deepSeekReasoning := providerschema.RequestUsesDeepSeekReasoning(request)
	if len(request.DeepSeekReasoningReplays) > 0 && !deepSeekReasoning {
		return nil, errors.New("DeepSeek private protocol replay is incompatible with the request")
	}
	messages, err := providerschema.OpenAIChatMessages(
		request.Messages,
		request.DeepSeekReasoningReplays,
		deepSeekReasoning,
	)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"model":          request.Model,
		"messages":       messages,
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
	}
	if request.MaxOutputTokens > 0 {
		body["max_tokens"] = request.MaxOutputTokens
	}
	if len(request.Tools) > 0 {
		body["tools"] = providerschema.OpenAITools(request.Tools)
	}
	ApplyChatCompletionsReasoning(body, request)
	return body, nil
}

func BearerStreamHeaders(apiKey string) map[string]string {
	return map[string]string{
		"Content-Type":  "application/json",
		"Accept":        "text/event-stream",
		"Authorization": "Bearer " + apiKey,
	}
}

func ApplyChatCompletionsReasoning(body map[string]any, request domainmodel.Request) {
	effort := providerschema.NormalizeRequestReasoningEffort(request.ReasoningEffort)
	if effort == "" {
		return
	}
	switch RequestReasoningProtocol(request) {
	case "deepseek-chat-completions":
		applyDeepSeekChatReasoning(body, effort, true)
	case "glm-chat-completions":
		applyGLMChatReasoning(body, effort)
	case "mimo-chat-completions":
		applyMimoChatReasoning(body, effort)
	}
}

func RequestReasoningProtocol(request domainmodel.Request) string {
	return providerschema.NormalizeReasoningProtocol(request.ReasoningProtocol)
}

func applyDeepSeekChatReasoning(body map[string]any, effort string, nativeDeepSeek bool) {
	if effort == "off" {
		if nativeDeepSeek {
			body["thinking"] = map[string]any{"type": "disabled"}
		}
		return
	}
	if effort == "auto" {
		return
	}
	if effort == "low" {
		body["reasoning_effort"] = "low"
	} else if effort == "max" {
		body["reasoning_effort"] = "max"
	} else {
		body["reasoning_effort"] = "high"
	}
	if nativeDeepSeek {
		body["thinking"] = map[string]any{"type": "enabled"}
	}
}

func applyGLMChatReasoning(body map[string]any, effort string) {
	if effort == "auto" {
		return
	}
	thinkingType := "enabled"
	if effort == "off" {
		thinkingType = "disabled"
	}
	body["thinking"] = map[string]any{
		"type":           thinkingType,
		"clear_thinking": true,
	}
}

func applyMimoChatReasoning(body map[string]any, effort string) {
	if effort == "auto" {
		return
	}
	if effort == "off" {
		body["thinking"] = map[string]any{"type": "disabled"}
		return
	}
	if effort == "max" {
		effort = "high"
	}
	body["reasoning_effort"] = effort
	body["thinking"] = map[string]any{"type": "enabled"}
}
