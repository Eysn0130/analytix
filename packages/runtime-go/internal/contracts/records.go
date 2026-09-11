package contracts

import (
	"encoding/json"
	"math"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func StringField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}

func CloneMap(value map[string]any) map[string]any {
	data, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func CloneValue(value any) any {
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func ResponseID(body json.RawMessage) string {
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		return ""
	}
	id, _ := value["id"].(string)
	return id
}

func ThreadSummary(thread map[string]any) map[string]any {
	summary := make(map[string]any)
	turns, _ := thread["turns"].([]any)
	caseSensitive := caseSensitiveThreadHistory(thread, turns)
	for _, key := range []string{
		"id", "title", "workspace", "model", "providerId", "mode", "status", "executionPolicyVersion", "approvalPolicy",
		"sandboxMode", "relation", "parentThreadId", "forkedFromThreadId", "forkedFromTitle",
		"forkedAt", "forkedFromMessageCount", "forkedFromTurnCount", "preview", "goal", "todos", "createdAt", "updatedAt",
	} {
		if caseSensitive && (key == "preview" || key == "goal" || key == "todos") {
			continue
		}
		if value, ok := thread[key]; ok {
			summary[key] = CloneValue(value)
		}
	}
	summary["turnCount"] = float64(len(turns))
	summary["messageCount"] = float64(countUserMessages(turns))
	if strings.TrimSpace(StringField(summary, "preview")) == "" {
		delete(summary, "preview")
		if preview := threadPreview(turns, caseSensitive); preview != "" {
			summary["preview"] = preview
		}
	}
	if caseSensitive {
		summary["historyAuthority"] = "case_boundary_only_v1"
	}
	return summary
}

func NumericSeq(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), !math.Signbit(typed) && typed == float64(int(typed))
	case json.Number:
		parsed, err := typed.Int64()
		return int(parsed), err == nil && !(parsed == 0 && strings.HasPrefix(typed.String(), "-"))
	default:
		return 0, false
	}
}

func ContainsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func SafeRecordID(value string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_")
	cleaned := replacer.Replace(strings.TrimSpace(value))
	cleaned = strings.ReplaceAll(cleaned, "..", "__")
	if cleaned == "" || cleaned == "." {
		return "_"
	}
	return cleaned
}

func CountThreadItems(thread map[string]any) int {
	turns, _ := thread["turns"].([]any)
	total := 0
	for _, turnValue := range turns {
		turn, _ := turnValue.(map[string]any)
		items, _ := turn["items"].([]any)
		total += len(items)
	}
	return total
}

func EventKinds(events []map[string]any) []string {
	kinds := make([]string, 0, len(events))
	for _, event := range events {
		if kind := StringField(event, "kind"); kind != "" {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

func AllPersistBeforePublish(orders [][]string) bool {
	if len(orders) == 0 {
		return false
	}
	for _, order := range orders {
		if len(order) != 2 || order[0] != "persist" || order[1] != "publish" {
			return false
		}
	}
	return true
}

func countUserMessages(turns []any) int {
	count := 0
	for _, turnValue := range turns {
		turn, _ := turnValue.(map[string]any)
		items, _ := turn["items"].([]any)
		for _, itemValue := range items {
			item, _ := itemValue.(map[string]any)
			if item["kind"] == "user_message" {
				count++
			}
		}
	}
	return count
}

func threadPreview(turns []any, userOnly bool) string {
	for turnIndex := len(turns) - 1; turnIndex >= 0; turnIndex-- {
		turn, _ := turns[turnIndex].(map[string]any)
		items, _ := turn["items"].([]any)
		for itemIndex := len(items) - 1; itemIndex >= 0; itemIndex-- {
			item, _ := items[itemIndex].(map[string]any)
			kind := StringField(item, "kind")
			if kind != "user_message" && (userOnly || kind != "assistant_text") {
				continue
			}
			text := strings.Join(strings.Fields(StringField(item, "text")), " ")
			if text == "" {
				continue
			}
			const maxPreview = 160
			runes := []rune(text)
			if len(runes) > maxPreview {
				return string(runes[:maxPreview])
			}
			return text
		}
	}
	return ""
}

func caseSensitiveThreadHistory(thread map[string]any, turns []any) bool {
	_ = turns
	caseSensitive, err := domainsecurity.ClassifyCaseSensitiveThread(thread)
	return caseSensitive || err != nil
}
