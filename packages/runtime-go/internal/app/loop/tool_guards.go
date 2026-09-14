package loop

import (
	"encoding/json"
	"fmt"
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

const RepeatSuccessBreakThreshold = 2
const FailureStormBreakThreshold = 3
const InvalidToolArgumentsHardStopThreshold = 3
const FailureStormHardStopThreshold = 6
const MaxInterruptedStreamRecoveries = 3

func ApplyFailureStormGuard(call domainmodel.ToolCall, output any, isError bool, signature *string, count *int) (any, bool) {
	if !isError {
		*signature = ""
		*count = 0
		return output, false
	}
	current, reason, ok := failureStormSignature(call, output)
	if !ok {
		*signature = ""
		*count = 0
		return output, false
	}
	if current != *signature {
		*signature = current
		*count = 1
		return output, false
	}
	*count++
	if *count < FailureStormBreakThreshold {
		return output, false
	}
	return outputWithFailureStormGuard(call.Name, output, *count, reason), true
}

func FailureStormTurnFailure(call domainmodel.ToolCall, output any, isError bool, count int) (TurnFailureError, bool) {
	if !isError || count <= 0 {
		return TurnFailureError{}, false
	}
	reason := failureStormOutputReason(output)
	if reason == "" {
		reason = call.Name + " failed"
	}
	if count >= InvalidToolArgumentsHardStopThreshold && emptyRequiredArgumentFailure(call, output) {
		return TurnFailureError{
			Message:  fmt.Sprintf("Turn stopped because %q repeatedly called tools with empty or missing required arguments (%s).", call.Name, reason),
			Code:     "tool_invalid_arguments_storm",
			Severity: "error",
			Details: map[string]any{
				"toolName":   call.Name,
				"stormCount": float64(count),
				"reason":     reason,
			},
		}, true
	}
	if count >= FailureStormHardStopThreshold {
		return TurnFailureError{
			Message:  fmt.Sprintf("Turn stopped because %q failed %d times in a row with the same error (%s).", call.Name, count, reason),
			Code:     "tool_failure_storm",
			Severity: "error",
			Details: map[string]any{
				"toolName":   call.Name,
				"stormCount": float64(count),
				"reason":     reason,
			},
		}, true
	}
	return TurnFailureError{}, false
}

func RepeatSuccessBlock(call domainmodel.ToolCall, counts map[string]int) (map[string]any, bool) {
	signature, ok := repeatSuccessSignature(call)
	if !ok {
		return nil, false
	}
	count := counts[signature]
	if count < RepeatSuccessBreakThreshold {
		return nil, false
	}
	message := fmt.Sprintf(
		"blocked: [loop guard] %q has already succeeded %d times with the same write-like arguments in this user turn. Re-running it is unlikely to help and may repeat file writes. Change approach: use edit_file or multi_edit for file changes, verify with a read/test command, or explain the blocker in your final answer.",
		call.Name,
		count,
	)
	return map[string]any{
		"code":         "loop_guard",
		"error":        message,
		"toolName":     call.Name,
		"repeat_count": float64(count),
	}, true
}

func RecordRepeatSuccess(call domainmodel.ToolCall, counts map[string]int) {
	signature, ok := repeatSuccessSignature(call)
	if !ok {
		return
	}
	counts[signature]++
}

func repeatSuccessSignature(call domainmodel.ToolCall) (string, bool) {
	switch call.Name {
	case "generate_office_document", "write", "write_file", "edit", "edit_file", "multi_edit", "move_file", "notebook_edit", "delete_range", "delete_symbol":
		return call.Name + "\x00" + canonicalJSON(call.Arguments), true
	case "bash":
		args := map[string]any{}
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return "", false
		}
		if toolGuardBoolField(args, "run_in_background") || toolGuardBoolField(args, "runInBackground") {
			return "", false
		}
		command := strings.TrimSpace(toolGuardStringField(args, "command"))
		if command == "" || !shellCommandWritesFile(command) {
			return "", false
		}
		return "bash\x00" + normalizeShellCommand(command), true
	default:
		return "", false
	}
}

func failureStormSignature(call domainmodel.ToolCall, output any) (string, string, bool) {
	reason := ""
	code := ""
	causeCode := ""
	switch typed := output.(type) {
	case map[string]any:
		code = strings.TrimSpace(toolGuardStringField(typed, "code"))
		if failureStormExcludedCode(code) {
			return "", "", false
		}
		causeCode = strings.TrimSpace(toolGuardStringField(typed, "cause_code"))
		reason = strings.TrimSpace(firstNonEmptyToolGuardString(typed["error"], typed["message"]))
	case string:
		reason = strings.TrimSpace(typed)
	default:
		return "", "", false
	}
	if reason == "" && code == "" && causeCode == "" {
		return "", "", false
	}
	normalizedReason := failureStormNormalizeReason(reason)
	signature := call.Name + "\x00" + code + "\x00" + causeCode + "\x00" + normalizedReason
	return signature, firstNonEmptyToolGuardString(normalizedReason, code, causeCode, call.Name+" failed"), true
}

func failureStormExcludedCode(code string) bool {
	switch strings.TrimSpace(code) {
	case "",
		"loop_guard",
		"tool_not_advertised",
		"approval_denied",
		"approval_policy_blocked",
		"sandbox_blocked",
		"workspace_escape",
		"subagent_tool_filtered",
		"subagents_disabled":
		return true
	default:
		return false
	}
}

func failureStormNormalizeReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ""
	}
	line := reason
	if index := strings.IndexAny(line, "\r\n"); index >= 0 {
		line = line[:index]
	}
	line = strings.Join(strings.Fields(line), " ")
	if len(line) > 240 {
		line = line[:240]
	}
	return line
}

func failureStormOutputReason(output any) string {
	switch typed := output.(type) {
	case map[string]any:
		return strings.TrimSpace(firstNonEmptyToolGuardString(typed["loop_guard_reason"], typed["error"], typed["message"], typed["code"]))
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func emptyRequiredArgumentFailure(call domainmodel.ToolCall, output any) bool {
	if !toolArgumentsEmpty(call.Arguments) {
		return false
	}
	code := ""
	reason := ""
	if record, ok := output.(map[string]any); ok {
		code = strings.TrimSpace(toolGuardStringField(record, "code"))
		reason = strings.TrimSpace(firstNonEmptyToolGuardString(record["loop_guard_reason"], record["error"], record["message"]))
	} else if text, ok := output.(string); ok {
		reason = strings.TrimSpace(text)
	}
	if code != "" && code != "validation_error" {
		return false
	}
	lower := strings.ToLower(reason)
	return strings.Contains(lower, " is required") ||
		strings.Contains(lower, " are required") ||
		strings.Contains(lower, "missing required")
}

func toolArgumentsEmpty(raw json.RawMessage) bool {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "{}" || text == "null" {
		return true
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(text), &args); err != nil {
		return false
	}
	return len(args) == 0
}

func outputWithFailureStormGuard(toolName string, output any, count int, reason string) any {
	message := fmt.Sprintf(
		"[loop guard] %q has failed %d times in a row with the same error (%s). Re-sending it unchanged will not help. Change the arguments, split the work into smaller calls, use a different tool, or explain the blocker in your final answer.",
		toolName,
		count,
		reason,
	)
	switch typed := output.(type) {
	case map[string]any:
		updated := toolGuardCloneMap(typed)
		updated["loop_guard"] = true
		updated["storm_count"] = float64(count)
		updated["loop_guard_reason"] = reason
		existing := strings.TrimSpace(firstNonEmptyToolGuardString(updated["error"], updated["message"]))
		if existing == "" {
			updated["error"] = message
		} else {
			updated["error"] = existing + "\n\n" + message
		}
		return updated
	case string:
		existing := strings.TrimSpace(typed)
		if existing == "" {
			return message
		}
		return existing + "\n\n" + message
	default:
		return map[string]any{
			"code":              "tool_failed",
			"error":             message,
			"toolName":          toolName,
			"loop_guard":        true,
			"storm_count":       float64(count),
			"loop_guard_reason": reason,
		}
	}
}

func normalizeShellCommand(command string) string {
	return strings.Join(strings.Fields(command), " ")
}

func shellCommandWritesFile(command string) bool {
	lower := strings.ToLower(command)
	switch {
	case shellPythonOpenWrites(lower):
		return true
	case strings.Contains(lower, "set-content") || strings.Contains(lower, "add-content") || strings.Contains(lower, "out-file"):
		return true
	case strings.Contains(lower, "sed -i") || strings.Contains(lower, "perl -pi"):
		return true
	case shellHasWriteRedirect(command):
		return true
	default:
		return false
	}
}

func shellPythonOpenWrites(lower string) bool {
	if !strings.Contains(lower, "open(") {
		return false
	}
	if strings.Contains(lower, ".write(") {
		return true
	}
	for _, marker := range []string{", 'w", `, "w`, ", 'a", `, "a`, ", 'x", `, "x`, "mode='w", `mode="w`, "mode='a", `mode="a`, "mode='x", `mode="x`} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func shellHasWriteRedirect(command string) bool {
	var quote rune
	var previous rune
	for _, char := range command {
		if quote != 0 {
			if char == quote {
				quote = 0
			}
			previous = char
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			previous = char
			continue
		}
		if char == '>' {
			if previous == '2' {
				previous = char
				continue
			}
			return true
		}
		previous = char
	}
	return false
}

func canonicalJSON(body json.RawMessage) string {
	if len(body) == 0 {
		return ""
	}
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return strings.TrimSpace(string(body))
	}
	data, err := json.Marshal(value)
	if err != nil {
		return strings.TrimSpace(string(body))
	}
	return string(data)
}

func toolGuardStringField(record map[string]any, key string) string {
	if record == nil {
		return ""
	}
	value, _ := record[key].(string)
	return strings.TrimSpace(value)
}

func toolGuardBoolField(record map[string]any, key string) bool {
	if record == nil {
		return false
	}
	value, _ := record[key].(bool)
	return value
}

func firstNonEmptyToolGuardString(values ...any) string {
	for _, value := range values {
		text, _ := value.(string)
		if strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

func toolGuardCloneMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
