package server

import (
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
)

func stringField(record map[string]any, key string) string {
	return contracts.StringField(record, key)
}

func cloneMap(value map[string]any) map[string]any {
	return contracts.CloneMap(value)
}

func cloneValue(value any) any {
	return contracts.CloneValue(value)
}

func listAny(value any) []any {
	if items, ok := value.([]any); ok {
		return items
	}
	return []any{}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func numericSeq(value any) (int, bool) {
	return contracts.NumericSeq(value)
}

func containsString(values []string, needle string) bool {
	return contracts.ContainsString(values, needle)
}

func safeDurableID(value string) string {
	return contracts.SafeRecordID(value)
}

func countThreadItems(thread map[string]any) int {
	return contracts.CountThreadItems(thread)
}

func threadSummary(thread map[string]any) map[string]any {
	return contracts.ThreadSummary(thread)
}
