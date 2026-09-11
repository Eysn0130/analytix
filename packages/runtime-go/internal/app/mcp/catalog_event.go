package mcp

import (
	"sort"
	"strings"
)

type CatalogChangedEventInput struct {
	ThreadID    string
	Diagnostics map[string]any
	ToolNames   []string
}

func BuildCatalogChangedEvent(input CatalogChangedEventInput) (map[string]any, bool) {
	threadID := strings.TrimSpace(input.ThreadID)
	if threadID == "" || !boolField(input.Diagnostics, "catalogDrift") {
		return nil, false
	}
	fingerprint := strings.TrimSpace(stringField(input.Diagnostics, "catalogFingerprint"))
	if fingerprint == "" {
		return nil, false
	}
	toolCount, ok := numericAny(input.Diagnostics["advertisedToolCount"])
	if !ok {
		toolCount, _ = numericAny(input.Diagnostics["indexedToolCount"])
	}
	toolNames := append([]string{}, input.ToolNames...)
	sort.Strings(toolNames)
	return map[string]any{
		"kind":        "tool_catalog_changed",
		"threadId":    threadID,
		"fingerprint": fingerprint,
		"toolCount":   toolCount,
		"toolNames":   stringListAny(toolNames),
		"message":     "MCP tool catalog changed after refresh.",
	}, true
}

func stringField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}

func boolField(record map[string]any, key string) bool {
	value, _ := record[key].(bool)
	return value
}

func numericAny(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	default:
		return 0, false
	}
}

func stringListAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}
