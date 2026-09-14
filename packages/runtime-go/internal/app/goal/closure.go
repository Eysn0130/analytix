package goal

import (
	"encoding/json"
	"errors"
	"strings"
)

func ToolResponse(goal map[string]any, completionBudgetReport string) map[string]any {
	return ToolResponseWithAudits(goal, completionBudgetReport, nil, nil)
}

func ToolResponseWithAudits(goal map[string]any, completionBudgetReport string, blockedAudit map[string]any, completionAudit map[string]any) map[string]any {
	out := map[string]any{
		"goal":            goal,
		"remainingTokens": nil,
	}
	if goal != nil {
		if budget, ok := numericAny(goal["tokenBudget"]); ok {
			used := int(floatFromAny(goal["tokensUsed"]))
			remaining := budget - used
			if remaining < 0 {
				remaining = 0
			}
			out["remainingTokens"] = float64(remaining)
		}
		if completionBudgetReport != "" && stringField(goal, "status") == "complete" {
			out["completionBudgetReport"] = completionBudgetReport
		}
	}
	if blockedAudit != nil {
		out["blockedAudit"] = blockedAudit
	}
	if completionAudit != nil {
		out["completionAudit"] = completionAudit
	}
	return out
}

func NormalizeTokenBudget(value any) (int, bool, error) {
	if value == nil {
		return 0, false, nil
	}
	if budget, ok := numericAny(value); ok && budget > 0 {
		return budget, true, nil
	}
	return 0, false, errors.New("token_budget must be a positive integer")
}

func EvidenceDetailsHostVerified(details []any) bool {
	if len(details) == 0 {
		return false
	}
	for _, raw := range details {
		detail, _ := raw.(map[string]any)
		if detail["hostVerified"] != true {
			return false
		}
	}
	return true
}

func ToolWritesPath(toolName string) bool {
	switch strings.TrimSpace(toolName) {
	case "generate_office_document", "write", "write_file", "edit", "edit_file", "multi_edit", "move_file", "notebook_edit", "delete_range", "delete_symbol":
		return true
	default:
		return false
	}
}

func ToolReadsOrWritesPath(toolName string) bool {
	switch strings.TrimSpace(toolName) {
	case "read", "read_file", "ls", "find", "glob", "code_index", "grep", "bash":
		return true
	default:
		return ToolWritesPath(toolName)
	}
}

func OutputMentionsPath(output map[string]any, path string) bool {
	needle := NormalizeEvidencePath(path)
	if needle == "" {
		return false
	}
	candidates := []string{}
	for _, key := range []string{"path", "relative_path", "source_path", "destination_path", "source_relative_path", "destination_relative_path", "file", "relativePath"} {
		if value := strings.TrimSpace(firstNonEmptyAnyString(output[key])); value != "" {
			candidates = append(candidates, value)
		}
	}
	if diff, ok := output["diff"].(map[string]any); ok {
		for _, key := range []string{"path", "relative_path", "file"} {
			if value := strings.TrimSpace(firstNonEmptyAnyString(diff[key])); value != "" {
				candidates = append(candidates, value)
			}
		}
	}
	if command := strings.TrimSpace(stringField(output, "command")); command != "" {
		candidates = append(candidates, command)
	}
	if text := strings.TrimSpace(firstNonEmptyAnyString(output["output"], output["message"])); text != "" {
		candidates = append(candidates, text)
	}
	for _, candidate := range candidates {
		normalized := NormalizeEvidencePath(candidate)
		if normalized == needle || strings.Contains(normalized, needle) || strings.Contains(needle, normalized) {
			return true
		}
	}
	return false
}

func NormalizeEvidencePath(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	path = strings.TrimPrefix(path, "./")
	return strings.ToLower(path)
}

func FindTodoStepIndex(items []any, step string, stepIndex int, stepProvided bool) int {
	if stepIndex > 0 {
		if stepIndex > len(items) {
			return -1
		}
		index := stepIndex - 1
		if !stepProvided {
			return index
		}
		item, _ := items[index].(map[string]any)
		normalizedStep := NormalizeTodoStep(step)
		if normalizedStep != "" &&
			(NormalizeTodoStep(stringField(item, "content")) == normalizedStep ||
				NormalizeTodoStep(stringField(item, "id")) == normalizedStep) {
			return index
		}
		return -1
	}
	normalizedStep := NormalizeTodoStep(step)
	for index, raw := range items {
		item, _ := raw.(map[string]any)
		if NormalizeTodoStep(stringField(item, "content")) == normalizedStep || NormalizeTodoStep(stringField(item, "id")) == normalizedStep {
			return index
		}
	}
	return -1
}

func NormalizeTodoStep(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func CleanBlockedReason(reason string) string {
	return strings.Trim(reason, " \t\r\n:,.!?;_-'\"[]()")
}

func NormalizeBlockedReason(reason string) string {
	reason = strings.ToLower(CleanBlockedReason(reason))
	var builder strings.Builder
	lastSpace := false
	for _, r := range reason {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r > 127 {
			builder.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			builder.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

func CanonicalBlockedReasonV1(reason string) (string, string, error) {
	projected := truncateText(projectGoalText(CleanBlockedReason(reason)), 2000)
	comparisonKey := NormalizeBlockedReason(projected)
	if comparisonKey == "" {
		return "", "", errors.New("reason is required when marking a goal blocked")
	}
	return comparisonKey, comparisonKey, nil
}

func StrictCompletionSelfCheckInstructions() string {
	return strings.Join([]string{
		"Strict goal completion self-check required:",
		"1. Verify changed files compile or parse correctly when applicable.",
		"2. Run the relevant tests or explain concrete evidence that covers the change.",
		"3. Confirm the original requirements, current todos, and evidence ledger are complete.",
		"Record the result with complete_step using self_check: true, then call update_goal status \"complete\" again.",
	}, "\n")
}

func numericAny(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return int(parsed), err == nil
	default:
		return 0, false
	}
}

func floatFromAny(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	default:
		return 0
	}
}

func firstNonEmptyAnyString(values ...any) string {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return typed
			}
		case []byte:
			if strings.TrimSpace(string(typed)) != "" {
				return string(typed)
			}
		}
	}
	return ""
}

func stringField(record map[string]any, key string) string {
	if record == nil {
		return ""
	}
	value, _ := record[key].(string)
	return strings.TrimSpace(value)
}
