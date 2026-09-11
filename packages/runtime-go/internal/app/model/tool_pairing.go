package model

import (
	"encoding/json"
	"fmt"
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

const interruptedToolResult = "[no result: the previous turn was interrupted before this tool call completed]"

func SanitizeToolArgumentsJSON(data []byte) string {
	return SanitizeJSONArguments(string(data))
}

func SanitizeJSONArguments(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "{}"
	}
	var value any
	if err := json.Unmarshal([]byte(trimmed), &value); err == nil {
		data, err := json.Marshal(value)
		if err == nil {
			return string(data)
		}
	}
	if !strings.HasPrefix(trimmed, "{") {
		return "{}"
	}
	candidate := CloseTruncatedJSONArguments(trimmed)
	if err := json.Unmarshal([]byte(candidate), &value); err == nil {
		data, err := json.Marshal(value)
		if err == nil {
			return string(data)
		}
	}
	return "{}"
}

func CloseTruncatedJSONArguments(raw string) string {
	var stack []byte
	inString := false
	escaped := false
	out := raw
	for index := 0; index < len(out); index++ {
		character := out[index]
		if inString {
			switch {
			case escaped:
				escaped = false
			case character == '\\':
				escaped = true
			case character == '"':
				inString = false
			}
			continue
		}
		switch character {
		case '"':
			inString = true
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if escaped {
		out = out[:len(out)-1]
	}
	if inString {
		out += `"`
	}
	trimmed := strings.TrimRight(out, " \t\r\n")
	switch {
	case strings.HasSuffix(trimmed, ","):
		out = trimmed[:len(trimmed)-1]
	case strings.HasSuffix(trimmed, ":"):
		out = trimmed + "null"
	}
	for index := len(stack) - 1; index >= 0; index-- {
		out += string(stack[index])
	}
	return out
}

func SanitizeToolPairing(messages []domainmodel.Message) []domainmodel.Message {
	return normalizeToolPairing(messages, true)
}

func NormalizeSessionMessages(messages []domainmodel.Message) []domainmodel.Message {
	return normalizeToolPairing(messages, false)
}

func normalizeToolPairing(messages []domainmodel.Message, dropOrphanTools bool) []domainmodel.Message {
	if normalized, ok := toolPairingFastPath(messages, dropOrphanTools); ok {
		return normalized
	}
	out := make([]domainmodel.Message, 0, len(messages))
	for index := 0; index < len(messages); {
		message := messages[index]
		if len(message.ToolCalls) == 0 {
			if message.Role != "tool" || !dropOrphanTools {
				out = append(out, message)
			}
			index++
			continue
		}
		next := index + 1
		results := []domainmodel.Message{}
		for next < len(messages) && messages[next].Role == "tool" {
			results = append(results, messages[next])
			next++
		}
		calls := sanitizeToolCalls(message.ToolCalls, results, dropOrphanTools)
		if len(calls) == 0 {
			if strings.TrimSpace(message.Content) != "" {
				message.ToolCalls = nil
				out = append(out, message)
			}
			index = next
			continue
		}
		message.ToolCalls = calls
		out = append(out, message)
		if dropOrphanTools {
			out = append(out, pairToolResults(calls, results)...)
		} else {
			out = append(out, sessionToolResults(calls, results)...)
		}
		index = next
	}
	return out
}

func toolPairingFastPath(messages []domainmodel.Message, dropOrphanTools bool) ([]domainmodel.Message, bool) {
	for index := 0; index < len(messages); {
		message := messages[index]
		if len(message.ToolCalls) == 0 {
			if message.Role == "tool" && dropOrphanTools {
				return nil, false
			}
			index++
			continue
		}
		if toolCallsNeedRepair(message.ToolCalls, dropOrphanTools) {
			return nil, false
		}
		next := index + 1
		results := []domainmodel.Message{}
		for next < len(messages) && messages[next].Role == "tool" {
			results = append(results, messages[next])
			next++
		}
		if !toolResultsMatchCalls(message.ToolCalls, results) {
			return nil, false
		}
		index = next
	}
	return messages, true
}

func toolCallsNeedRepair(calls []domainmodel.ToolCall, requireHostIdentity bool) bool {
	for _, call := range calls {
		if strings.TrimSpace(call.ID) == "" || strings.TrimSpace(call.Name) == "" {
			return true
		}
		if requireHostIdentity && !domainmodel.IsHostToolCallIDV1(call.ID) {
			return true
		}
		raw := strings.TrimSpace(string(call.Arguments))
		if raw == "" || !json.Valid([]byte(raw)) {
			return true
		}
	}
	return false
}

func toolResultsMatchCalls(calls []domainmodel.ToolCall, results []domainmodel.Message) bool {
	if len(calls) != len(results) {
		return false
	}
	if !distinctNonEmptyToolCallIDs(calls) {
		return false
	}
	for index, call := range calls {
		if strings.TrimSpace(results[index].ToolCallID) != call.ID {
			return false
		}
	}
	return true
}

func sanitizeToolCalls(calls []domainmodel.ToolCall, results []domainmodel.Message, requireHostIdentity bool) []domainmodel.ToolCall {
	calls = backfillToolCallNames(calls, results)
	out := make([]domainmodel.ToolCall, 0, len(calls))
	for index, call := range calls {
		if requireHostIdentity && !domainmodel.IsHostToolCallIDV1(call.ID) {
			continue
		}
		if strings.TrimSpace(call.ID) == "" {
			call.ID = fmt.Sprintf("call_%d", index)
		} else {
			call.ID = strings.TrimSpace(call.ID)
		}
		if strings.TrimSpace(call.Name) == "" {
			call.Name = "unknown_tool"
		} else {
			call.Name = strings.TrimSpace(call.Name)
		}
		call.Arguments = json.RawMessage(SanitizeJSONArguments(string(call.Arguments)))
		out = append(out, call)
	}
	return out
}

func backfillToolCallNames(calls []domainmodel.ToolCall, results []domainmodel.Message) []domainmodel.ToolCall {
	hasEmpty := false
	for _, call := range calls {
		if strings.TrimSpace(call.Name) == "" {
			hasEmpty = true
			break
		}
	}
	if !hasEmpty || len(results) == 0 {
		return calls
	}
	out := append([]domainmodel.ToolCall(nil), calls...)
	if distinctNonEmptyToolCallIDs(calls) {
		namesByID := map[string]string{}
		for _, result := range results {
			id := strings.TrimSpace(result.ToolCallID)
			name := strings.TrimSpace(result.Name)
			if id != "" && name != "" {
				namesByID[id] = name
			}
		}
		for index, call := range out {
			if strings.TrimSpace(call.Name) != "" {
				continue
			}
			if name := namesByID[strings.TrimSpace(call.ID)]; name != "" {
				out[index].Name = name
			}
		}
		return out
	}
	for index := range out {
		if strings.TrimSpace(out[index].Name) != "" || index >= len(results) {
			continue
		}
		if name := strings.TrimSpace(results[index].Name); name != "" {
			out[index].Name = name
		}
	}
	return out
}

func pairToolResults(calls []domainmodel.ToolCall, results []domainmodel.Message) []domainmodel.Message {
	out := make([]domainmodel.Message, 0, len(calls))
	if distinctNonEmptyToolCallIDs(calls) {
		byID := map[string]domainmodel.Message{}
		for _, result := range results {
			if strings.TrimSpace(result.ToolCallID) != "" {
				byID[result.ToolCallID] = result
			}
		}
		for index, call := range calls {
			if result, ok := byID[call.ID]; ok {
				out = append(out, result)
				continue
			}
			if index < len(results) && strings.TrimSpace(results[index].ToolCallID) == "" {
				result := results[index]
				result.ToolCallID = call.ID
				if strings.TrimSpace(result.Name) == "" {
					result.Name = call.Name
				}
				out = append(out, result)
				continue
			}
			out = append(out, interruptedToolResultMessage(call))
		}
		return out
	}
	for index, call := range calls {
		if index < len(results) {
			result := results[index]
			result.ToolCallID = call.ID
			if strings.TrimSpace(result.Name) == "" {
				result.Name = call.Name
			}
			out = append(out, result)
			continue
		}
		out = append(out, interruptedToolResultMessage(call))
	}
	return out
}

func sessionToolResults(calls []domainmodel.ToolCall, results []domainmodel.Message) []domainmodel.Message {
	out := append([]domainmodel.Message(nil), results...)
	if distinctNonEmptyToolCallIDs(calls) {
		answered := map[string]bool{}
		for _, result := range results {
			if strings.TrimSpace(result.ToolCallID) != "" {
				answered[result.ToolCallID] = true
			}
		}
		for _, call := range calls {
			if !answered[call.ID] {
				out = append(out, interruptedToolResultMessage(call))
			}
		}
		return out
	}
	for index := len(results); index < len(calls); index++ {
		out = append(out, interruptedToolResultMessage(calls[index]))
	}
	return out
}

func distinctNonEmptyToolCallIDs(calls []domainmodel.ToolCall) bool {
	seen := map[string]bool{}
	for _, call := range calls {
		if strings.TrimSpace(call.ID) == "" || seen[call.ID] {
			return false
		}
		seen[call.ID] = true
	}
	return true
}

func interruptedToolResultMessage(call domainmodel.ToolCall) domainmodel.Message {
	return domainmodel.Message{Role: "tool", Name: call.Name, ToolCallID: call.ID, Content: interruptedToolResult}
}
