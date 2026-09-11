package usage

import "strings"

func LatestCacheDiagnostics(events []map[string]any, turnID string) map[string]any {
	turnID = strings.TrimSpace(turnID)
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if stringField(event, "kind") != "usage" {
			continue
		}
		if turnID != "" && strings.TrimSpace(stringField(event, "turnId")) != turnID {
			continue
		}
		if diagnostics := mapFieldClone(event, "cacheDiagnostics"); len(diagnostics) > 0 {
			return diagnostics
		}
	}
	return nil
}

func mapFieldClone(record map[string]any, key string) map[string]any {
	value, _ := record[key].(map[string]any)
	if len(value) == 0 {
		return nil
	}
	out := make(map[string]any, len(value))
	for childKey, childValue := range value {
		out[childKey] = childValue
	}
	return out
}
