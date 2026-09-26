package model

import (
	"encoding/json"
	"sort"
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func CapturePrefixShape(request domainmodel.Request) domainmodel.PrefixShape {
	system := request.SystemPrompt
	if system == "" {
		for _, msg := range request.Messages {
			if msg.Role == "system" {
				system = msg.Content
				break
			}
		}
	}
	tools := make([]domainmodel.ToolSchema, len(request.Tools))
	copy(tools, request.Tools)
	for index := range tools {
		if len(tools[index].Parameters) > 0 {
			tools[index].Parameters = json.RawMessage(domainmodel.CanonicalProviderJSONSchema(tools[index].Parameters))
		}
	}
	sort.SliceStable(tools, func(i, j int) bool {
		if tools[i].Name != tools[j].Name {
			return tools[i].Name < tools[j].Name
		}
		if tools[i].Description != tools[j].Description {
			return tools[i].Description < tools[j].Description
		}
		return domainmodel.CanonicalProviderJSONSchema(tools[i].Parameters) < domainmodel.CanonicalProviderJSONSchema(tools[j].Parameters)
	})
	toolData, _ := json.Marshal(tools)
	toolSourceIDs := canonicalToolSourceIDs(request.Tools)
	toolSourceData, _ := json.Marshal(toolSourceIDs)
	prefixItems := stablePrefixMessageItems(request.Messages)
	prefixItemsData, _ := json.Marshal(prefixItems)
	prefixData, _ := json.Marshal(map[string]any{
		"system": system,
		"tools":  json.RawMessage(toolData),
	})
	return domainmodel.PrefixShape{
		SystemHash:          domainmodel.BytesHash([]byte(system)),
		ToolsHash:           domainmodel.BytesHash(toolData),
		PrefixHash:          domainmodel.BytesHash(prefixData),
		PrefixItemsHash:     domainmodel.BytesHash(prefixItemsData),
		ToolSchemaTokens:    len(toolData) / 4,
		ToolCount:           len(tools),
		ToolSourcesHash:     domainmodel.BytesHash(toolSourceData),
		ToolSourceIDs:       toolSourceIDs,
		Route:               strings.TrimSpace(request.Route),
		Provider:            request.Family,
		ProviderID:          request.ProviderID,
		EndpointFormat:      request.EndpointFormat,
		Model:               request.Model,
		DynamicStateCheck:   "not_checked",
		ToolSchemaEstimator: "utf8_bytes_div4",
	}
}

func stablePrefixMessageItems(messages []domainmodel.Message) []map[string]any {
	end := len(messages)
	for end > 0 {
		role := normalizedRole(messages[end-1].Role)
		if role == "system" || role == "user" {
			end--
			continue
		}
		break
	}
	out := make([]map[string]any, 0, end)
	for _, message := range messages[:end] {
		role := normalizedRole(message.Role)
		if role == "" || role == "system" {
			continue
		}
		item := map[string]any{"role": role}
		if name := strings.TrimSpace(message.Name); name != "" {
			item["name"] = name
		}
		if content := strings.TrimSpace(message.Content); content != "" {
			item["content"] = content
		}
		if parts := stablePrefixMessageParts(message.Parts); len(parts) > 0 {
			item["parts"] = parts
		}
		if len(message.ToolCalls) > 0 {
			item["toolCalls"] = stablePrefixToolCalls(message.ToolCalls)
		}
		if id := strings.TrimSpace(message.ToolCallID); id != "" {
			item["toolCallId"] = id
		}
		out = append(out, item)
	}
	return out
}

func stablePrefixMessageParts(parts []domainmodel.MessagePart) []map[string]any {
	out := []map[string]any{}
	for _, part := range parts {
		switch strings.TrimSpace(strings.ToLower(part.Type)) {
		case "text":
			if text := strings.TrimSpace(part.Text); text != "" {
				out = append(out, map[string]any{"type": "text", "text": text})
			}
		case "image", "image_url", "input_image":
			item := map[string]any{"type": "image"}
			if mediaType := strings.TrimSpace(part.MediaType); mediaType != "" {
				item["mediaType"] = mediaType
			}
			if imageURL := strings.TrimSpace(part.ImageURL); imageURL != "" {
				item["imageUrlHash"] = shortHexHash([]byte(imageURL))
			}
			if data := strings.TrimSpace(part.Data); data != "" {
				item["dataHash"] = shortHexHash([]byte(data))
				item["dataBytes"] = len(data)
			}
			out = append(out, item)
		}
	}
	return out
}

func stablePrefixToolCalls(calls []domainmodel.ToolCall) []map[string]any {
	out := make([]map[string]any, 0, len(calls))
	for _, call := range calls {
		out = append(out, map[string]any{
			"id":        strings.TrimSpace(call.ID),
			"name":      strings.TrimSpace(call.Name),
			"arguments": json.RawMessage(SanitizeJSONArguments(string(call.Arguments))),
		})
	}
	return out
}

func normalizedRole(role string) string {
	return strings.TrimSpace(strings.ToLower(role))
}

func canonicalToolSourceIDs(tools []domainmodel.ToolSchema) []string {
	if len(tools) == 0 {
		return nil
	}
	seen := map[string]bool{}
	for _, tool := range tools {
		source := strings.TrimSpace(tool.Source)
		if source == "" {
			source = "builtin"
		}
		seen[source] = true
	}
	out := make([]string, 0, len(seen))
	for source := range seen {
		out = append(out, source)
	}
	sort.Strings(out)
	return out
}

func shortHexHash(data []byte) string {
	hash := domainmodel.BytesHash(data)
	if len(hash) <= 16 {
		return hash
	}
	return hash[:16]
}
