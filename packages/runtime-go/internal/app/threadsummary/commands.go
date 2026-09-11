package threadsummary

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

type CommandTask struct {
	Task map[string]any
}

type commandGroup struct {
	CallID    string
	Status    string
	IsError   bool
	HasResult bool
}

func CommandTasks(thread map[string]any, now time.Time) []CommandTask {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	groups := map[string]commandGroup{}
	for _, item := range Items(thread) {
		kind := stringField(item, "kind")
		if kind != "tool_call" && kind != "tool_result" {
			continue
		}
		if stringField(item, "toolKind") != "command_execution" && stringField(item, "toolName") != "bash" {
			continue
		}
		callID := firstNonEmptyAnyString(item["callId"], stringField(item, "id"))
		if callID == "" {
			continue
		}
		group := groups[callID]
		if group.CallID == "" {
			group.CallID = callID
		}
		if kind == "tool_result" {
			group.HasResult = true
			group.Status = stringField(item, "status")
			group.IsError = boolField(item, "isError")
		} else if !group.HasResult {
			group.Status = stringField(item, "status")
		}
		groups[callID] = group
	}
	tasks := make([]CommandTask, 0, len(groups))
	for _, group := range groups {
		// The closed public result projection has its own projection status;
		// it is not command-process lifecycle authority. A durable tool_result
		// settles the command, with isError selecting the terminal class.
		rawStatus := "pending"
		if group.HasResult {
			if group.IsError {
				rawStatus = "error"
			} else if strings.TrimSpace(group.Status) == "" {
				rawStatus = "completed"
			} else {
				rawStatus = PublicCommandStatusV1(group.Status, false)
			}
		} else if status := PublicCommandStatusV1(group.Status, false); status == "running" || status == "pending" {
			rawStatus = status
		}
		id := "command:" + group.CallID
		normalizedStatus := NormalizeCommandStatus(rawStatus, group.IsError)
		task := map[string]any{
			"schemaVersion":     1,
			"id":                id,
			"kind":              "command",
			"status":            rawStatus,
			"background":        false,
			"active":            normalizedStatus == "active",
			"terminal":          normalizedStatus == "done" || normalizedStatus == "terminal",
			"outputWithheld":    true,
			"outputTrustStatus": "private_tool_output",
			"factAnswerAllowed": false,
			"evidenceAuthority": false,
			"canReadOutput":     false,
			"canContinueParent": false,
		}
		tasks = append(tasks, CommandTask{Task: task})
	}
	return tasks
}

func Items(thread map[string]any) []map[string]any {
	items := []map[string]any{}
	for _, rawTurn := range listAny(thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		if turn == nil {
			continue
		}
		for _, rawItem := range listAny(turn["items"]) {
			item, _ := rawItem.(map[string]any)
			if item != nil {
				items = append(items, item)
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		left := stringField(items[i], "createdAt")
		right := stringField(items[j], "createdAt")
		if left == right {
			return stringField(items[i], "id") < stringField(items[j], "id")
		}
		return left < right
	})
	return items
}

func firstNonEmptyAnyString(values ...any) string {
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text != "" {
			return text
		}
	}
	return ""
}

func listAny(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []map[string]any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func boolField(record map[string]any, key string) bool {
	value, _ := record[key].(bool)
	return value
}

func floatFromAny(value any) float64 {
	switch typed := value.(type) {
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case float64:
		return typed
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	default:
		return 0
	}
}
