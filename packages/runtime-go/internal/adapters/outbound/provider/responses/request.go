package responses

import (
	"strings"

	providerschema "analytix.local/runtime-go/internal/adapters/outbound/provider/schema"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func Body(request domainmodel.Request) map[string]any {
	body := map[string]any{
		"model":  request.Model,
		"input":  InputMessages(request.Messages),
		"stream": true,
	}
	if request.MaxOutputTokens > 0 {
		body["max_output_tokens"] = request.MaxOutputTokens
	}
	if len(request.Tools) > 0 {
		body["tools"] = providerschema.OpenAITools(request.Tools)
	}
	ApplyReasoning(body, request)
	return body
}

func ApplyReasoning(body map[string]any, request domainmodel.Request) {
	protocol := providerschema.NormalizeReasoningProtocol(request.ReasoningProtocol)
	if protocol != "" && protocol != "openai-responses" {
		return
	}
	effort := providerschema.NormalizeRequestReasoningEffort(request.ReasoningEffort)
	switch effort {
	case "low", "medium":
		body["reasoning"] = map[string]any{"effort": effort}
	case "high", "max":
		body["reasoning"] = map[string]any{"effort": "high"}
	}
}

func InputMessages(messages []domainmodel.Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		if message.Role == "tool" {
			out = append(out, map[string]any{
				"type":    "function_call_output",
				"call_id": message.ToolCallID,
				"output":  message.Content,
			})
			continue
		}
		for _, call := range message.ToolCalls {
			args := string(call.Arguments)
			if strings.TrimSpace(args) == "" {
				args = "{}"
			}
			out = append(out, map[string]any{
				"type":      "function_call",
				"call_id":   call.ID,
				"name":      call.Name,
				"arguments": args,
			})
		}
		content := providerschema.ResponseContentParts(message)
		if len(content) == 0 {
			continue
		}
		out = append(out, map[string]any{
			"role":    message.Role,
			"content": content,
		})
	}
	return out
}
