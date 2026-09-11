package toolcatalog

import (
	"encoding/json"
	"fmt"
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func UserInputToolSchemas() []domainmodel.ToolSchema {
	parameters := json.RawMessage(`{"type":"object","properties":{"prompt":{"type":"string"},"questions":{"type":"array","maxItems":3,"items":{"type":"object","properties":{"question":{"type":"string"},"header":{"type":"string"},"options":{"type":"array","maxItems":3,"items":{"anyOf":[{"type":"string"},{"type":"object","properties":{"label":{"type":"string"},"description":{"type":"string"}},"required":["label"],"additionalProperties":false}]}}},"required":["question"],"additionalProperties":false}},"options":{"type":"array","maxItems":3,"items":{"anyOf":[{"type":"string"},{"type":"object","properties":{"label":{"type":"string"},"description":{"type":"string"}},"required":["label"],"additionalProperties":false}]}}},"required":["prompt"],"additionalProperties":false}`)
	return []domainmodel.ToolSchema{
		{
			Name:        "user_input",
			Description: "Ask the user for information only when the answer is required to continue.",
			Parameters:  parameters,
			Source:      "user_input",
		},
		{
			Name:        "request_user_input",
			Description: "Ask the user for information only when the answer is required to continue. Alias kept for Kun/Analytix GUI gate parity.",
			Parameters:  parameters,
			Source:      "user_input",
		},
	}
}

func UserInputQuestions(args map[string]any, fallbackID string, prompt string) []map[string]any {
	baseID := strings.TrimSpace(fallbackID)
	if baseID == "" {
		baseID = "input"
	}
	if raw, ok := args["questions"].([]any); ok && len(raw) > 0 {
		questions := []map[string]any{}
		for index, item := range raw {
			record, ok := item.(map[string]any)
			if !ok {
				continue
			}
			question := firstNonEmptyString(record["question"])
			if question == "" {
				continue
			}
			// Question identifiers are host authority. Model-originated IDs are
			// untrusted display data and must never become correlation keys.
			id := fmt.Sprintf("%s_%d", baseID, index+1)
			header := firstNonEmptyString(record["header"], fmt.Sprintf("Question %d", index+1))
			options := []map[string]string{}
			if rawOptions, ok := record["options"].([]any); ok {
				for _, optionAny := range rawOptions {
					option, ok := UserInputOption(optionAny)
					if !ok {
						continue
					}
					options = append(options, option)
				}
			}
			questions = append(questions, map[string]any{"header": header, "id": id, "question": question, "options": options})
		}
		if len(questions) > 0 {
			return questions
		}
	}
	options := []map[string]string{}
	if rawOptions, ok := args["options"].([]any); ok {
		for _, optionAny := range rawOptions {
			option, ok := UserInputOption(optionAny)
			if !ok {
				continue
			}
			options = append(options, option)
		}
	}
	return []map[string]any{{
		"header":   "Input",
		"id":       baseID + "_1",
		"question": prompt,
		"options":  options,
	}}
}

func UserInputOption(value any) (map[string]string, bool) {
	if label := strings.TrimSpace(firstNonEmptyString(value)); label != "" {
		return map[string]string{"label": label, "description": ""}, true
	}
	record, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	label := firstNonEmptyString(record["label"])
	if label == "" {
		return nil, false
	}
	return map[string]string{
		"label":       label,
		"description": firstNonEmptyString(record["description"]),
	}, true
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if typed, ok := value.(string); ok {
			if trimmed := strings.TrimSpace(typed); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}
