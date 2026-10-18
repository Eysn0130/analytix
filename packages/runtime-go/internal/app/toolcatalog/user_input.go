package toolcatalog

import (
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

// UserInputConfig is fixed when the Host is constructed. Zero value is legacy.
type UserInputConfig struct {
	Mode           string
	TimeoutSeconds *int
}

func NormalizeUserInputConfig(config UserInputConfig) (UserInputConfig, error) {
	if config.Mode == "" {
		config.Mode = "legacy"
	}
	if config.Mode != "legacy" && config.Mode != "timed" {
		return UserInputConfig{}, fmt.Errorf("invalid user input mode")
	}
	seconds := 120
	if config.TimeoutSeconds != nil {
		seconds = *config.TimeoutSeconds
	}
	if !domaincontinuation.ValidUserInputTimeoutSeconds(seconds) {
		return UserInputConfig{}, fmt.Errorf("invalid user input timeout")
	}
	config.TimeoutSeconds = &seconds
	return config, nil
}

func UserInputTimedWait(config UserInputConfig, arguments json.RawMessage) (*domaincontinuation.UserInputTimedWaitV1, error) {
	config, err := NormalizeUserInputConfig(config)
	if err != nil {
		return nil, err
	}
	if config.Mode != "timed" {
		return nil, nil
	}
	seconds := *config.TimeoutSeconds
	var args map[string]any
	if json.Unmarshal(arguments, &args) != nil || args == nil {
		return nil, fmt.Errorf("invalid user input arguments")
	}
	if raw, exists := args["timeout"]; exists {
		value, ok := raw.(float64)
		if !ok || math.Trunc(value) != value || value < -1 || value > 2147483 || value == 0 {
			return nil, fmt.Errorf("invalid user input timeout")
		}
		seconds = int(value)
	}
	return &domaincontinuation.UserInputTimedWaitV1{TimeoutSeconds: seconds}, nil
}

func UserInputToolSchemas(configs ...UserInputConfig) []domainmodel.ToolSchema {
	parameters := json.RawMessage(`{"type":"object","properties":{"prompt":{"type":"string"},"detail":{"type":"string"},"multi_select":{"type":"boolean"},"questions":{"type":"array","maxItems":3,"items":{"type":"object","properties":{"question":{"type":"string"},"header":{"type":"string"},"detail":{"type":"string"},"multi_select":{"type":"boolean"},"options":{"type":"array","maxItems":3,"items":{"anyOf":[{"type":"string"},{"type":"object","properties":{"label":{"type":"string"},"description":{"type":"string"}},"required":["label"],"additionalProperties":false}]}}},"required":["question"],"additionalProperties":false}},"options":{"type":"array","maxItems":3,"items":{"anyOf":[{"type":"string"},{"type":"object","properties":{"label":{"type":"string"},"description":{"type":"string"}},"required":["label"],"additionalProperties":false}]}}},"required":["prompt"],"additionalProperties":false}`)
	if len(configs) > 0 && configs[0].Mode == "timed" {
		var schema map[string]any
		_ = json.Unmarshal(parameters, &schema)
		schema["properties"].(map[string]any)["timeout"] = map[string]any{
			"description": "Seconds to wait for the whole question batch. -1 waits indefinitely for this call.",
			"anyOf":       []any{map[string]any{"type": "integer", "const": -1}, map[string]any{"type": "integer", "minimum": 1, "maximum": 2147483}},
		}
		parameters, _ = json.Marshal(schema)
	}
	description := "Ask the user for information only when the answer is required to continue."
	aliasDescription := description + " Alias kept for Kun/Analytix GUI gate parity."
	if len(configs) > 0 && configs[0].Mode == "timed" {
		description = "Ask the user a whole batch of questions. A finite foreground timeout returns pending, not an answer, skip or permission; continue independent work and accept a later reply to this call. -1 waits indefinitely for this call."
		aliasDescription = description
	}
	return []domainmodel.ToolSchema{
		{
			Name:        "user_input",
			Description: description,
			Parameters:  parameters,
			Source:      "user_input",
		},
		{
			Name:        "request_user_input",
			Description: aliasDescription,
			Parameters:  parameters,
			Source:      "user_input",
		},
	}
}

// UserInputArgumentsForTool derives display questions from the immutable
// signed original call. These arguments are never substituted into that call.
func UserInputArgumentsForTool(toolName string, args map[string]any) (map[string]any, error) {
	if toolName != ToolExitPlanModeName {
		return args, nil
	}
	plan, ok := args["plan"].(string)
	if len(args) != 1 || !ok || !utf8.ValidString(plan) || len(plan) > domaincontinuation.MaxUserInputToolArgumentsBytesV1 || !strings.HasPrefix(strings.TrimSpace(plan), "# ") {
		return nil, fmt.Errorf("invalid complete plan review arguments")
	}
	return map[string]any{"prompt": domaincontinuation.PlanReviewPromptV1, "questions": []any{map[string]any{
		"header": "Plan review", "question": domaincontinuation.PlanReviewPromptV1, "detail": plan,
		"options": []any{map[string]any{"label": "Approve", "description": "Leave plan mode; carry out the plan from the next step."}, map[string]any{"label": "Keep planning", "description": "Stay in plan mode; return feedback to the model."}},
	}}}, nil
}

// Approval is an actual Host option selection in the signed submission. A
// label or custom answer that happens to spell Approve is not authority.
func PlanReviewApprovedSubmissionV1(toolName, inputID string, submission *domaincontinuation.UserInputSubmissionV1) bool {
	if toolName != ToolExitPlanModeName || submission == nil || submission.Format != "structured" || domaincontinuation.ValidateUserInputSubmissionV1(*submission) != nil {
		return false
	}
	var answers []domaincontinuation.UserInputStructuredAnswerV1
	if json.Unmarshal(submission.Answers, &answers) != nil || len(answers) != 1 {
		return false
	}
	answer := answers[0]
	return answer.ID == inputID+"_1" && answer.Kind == "answer" && answer.CustomText == nil && answer.OptionIDs != nil && len(*answer.OptionIDs) == 1 && (*answer.OptionIDs)[0] == inputID+"_1_option_1"
}

func UserInputQuestions(args map[string]any, fallbackID string, prompt string) []map[string]any {
	baseID := strings.TrimSpace(fallbackID)
	if baseID == "" {
		baseID = "input"
	}
	if raw, ok := args["questions"].([]any); ok && len(raw) > 0 {
		questions := []map[string]any{}
		for _, item := range raw {
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
			ordinal := len(questions) + 1
			id := fmt.Sprintf("%s_%d", baseID, ordinal)
			header := firstNonEmptyString(record["header"], fmt.Sprintf("Question %d", ordinal))
			options := []map[string]string{}
			if rawOptions, ok := record["options"].([]any); ok {
				for _, optionAny := range rawOptions {
					option, ok := UserInputOption(optionAny)
					if !ok {
						continue
					}
					option["id"] = fmt.Sprintf("%s_option_%d", id, len(options)+1)
					options = append(options, option)
				}
			}
			multiSelect, _ := record["multi_select"].(bool)
			questions = append(questions, map[string]any{"header": header, "id": id, "question": question, "options": options, "multiSelect": multiSelect})
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
			option["id"] = fmt.Sprintf("%s_1_option_%d", baseID, len(options)+1)
			options = append(options, option)
		}
	}
	multiSelect, _ := args["multi_select"].(bool)
	return []map[string]any{{
		"header":      "Input",
		"id":          baseID + "_1",
		"question":    prompt,
		"options":     options,
		"multiSelect": multiSelect,
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

// UserInputQuestionsForLocalDisplay retains the original Provider string
// values, including whitespace, while using the same effective Core IDs as
// the public gate. It does not reverse aliases or issue display authority.
func UserInputQuestionsForLocalDisplay(args map[string]any, fallbackID, prompt string) []map[string]any {
	questions := UserInputQuestions(args, fallbackID, prompt)
	var sources []map[string]any
	if raw, ok := args["questions"].([]any); ok {
		for _, item := range raw {
			record, ok := item.(map[string]any)
			if ok && firstNonEmptyString(record["question"]) != "" {
				sources = append(sources, record)
			}
		}
	}
	for index, question := range questions {
		source := args
		if index < len(sources) {
			source = sources[index]
			question["question"] = originalUserInputText(source["question"])
			if firstNonEmptyString(source["header"]) != "" {
				question["header"] = originalUserInputText(source["header"])
			}
		} else {
			if original := originalUserInputText(args["prompt"], args["question"]); original != "" {
				question["question"] = original
			}
		}
		if detail, ok := source["detail"].(string); ok {
			question["detail"] = detail
		}
		options, _ := question["options"].([]map[string]string)
		ordinal := 0
		if raw, ok := source["options"].([]any); ok {
			for _, value := range raw {
				if _, valid := UserInputOption(value); !valid {
					continue
				}
				if ordinal >= len(options) {
					break
				}
				if record, ok := value.(map[string]any); ok {
					options[ordinal]["label"] = originalUserInputText(record["label"])
					if description, ok := record["description"].(string); ok {
						options[ordinal]["description"] = description
					}
				} else {
					options[ordinal]["label"] = originalUserInputText(value)
				}
				ordinal++
			}
		}
	}
	return questions
}

func originalUserInputText(values ...any) string {
	for _, value := range values {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}
