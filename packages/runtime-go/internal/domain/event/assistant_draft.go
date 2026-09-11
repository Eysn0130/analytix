package event

import "strings"

// IsLegacyAssistantDraftItem identifies assistant prose that older runtime
// versions persisted before a turn reached its terminal publication boundary.
// It is intentionally narrow and versioned by the legacy item-id contracts;
// current finals must use a host-authorized terminal publication record.
func IsLegacyAssistantDraftItem(item map[string]any) bool {
	if strings.TrimSpace(recordString(item, "kind")) != "assistant_text" {
		return false
	}
	status := strings.TrimSpace(recordString(item, "status"))
	if status != "" && status != "completed" {
		return true
	}
	itemID := strings.TrimSpace(recordString(item, "id"))
	turnID := strings.TrimSpace(recordString(item, "turnId"))
	if itemID == "" {
		return true
	}
	if strings.Contains(itemID, "_assistant_process_") {
		return true
	}
	return turnID != "" && itemID == "item_"+turnID+"_assistant_text"
}

func recordString(record map[string]any, key string) string {
	if record == nil {
		return ""
	}
	value, _ := record[key].(string)
	return value
}
