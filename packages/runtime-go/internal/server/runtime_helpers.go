package server

import (
	"os"
	"strings"

	controlapp "analytix.local/runtime-go/internal/app/control"
)

func mapsListAny(items []map[string]any) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, cloneMap(item))
	}
	return out
}

func normalizeTurnMode(value string) string {
	return controlapp.NormalizeTurnMode(value)
}

func normalizeReasoningEffort(value string) string {
	return controlapp.NormalizeReasoningEffort(value)
}

func normalizeApprovalPolicy(value string) string {
	return controlapp.NormalizeApprovalPolicy(value)
}

func normalizeSandboxMode(value string) string {
	return controlapp.NormalizeSandboxMode(value)
}

func runtimeThreadTraceEnabled() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("ANALYTIX_THREAD_TRACE")))
	return value == "1" || value == "true" || value == "yes"
}

func stringListAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
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

func boolField(record map[string]any, key string) bool {
	value, _ := record[key].(bool)
	return value
}

func mapFieldClone(record map[string]any, key string) map[string]any {
	value, ok := record[key].(map[string]any)
	if !ok {
		return nil
	}
	return cloneMap(value)
}
