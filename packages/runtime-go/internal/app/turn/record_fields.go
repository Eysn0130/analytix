package turn

import "strings"

func stringField(record map[string]any, key string) string {
	return strings.TrimSpace(rawStringField(record, key))
}

func rawStringField(record map[string]any, key string) string {
	if record == nil {
		return ""
	}
	value, _ := record[key].(string)
	return value
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
