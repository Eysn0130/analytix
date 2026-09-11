package turn

import (
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
)

type TerminalUpdateInput struct {
	Thread          map[string]any
	TurnID          string
	Status          string
	ProtectTerminal bool
	AppendItems     []map[string]any
	Fields          map[string]any
	ThreadFields    map[string]any
	FinishedAt      string
}

type TerminalUpdateResult struct {
	Thread        map[string]any
	Found         bool
	Applied       bool
	CurrentStatus string
}

func ApplyTerminalUpdate(input TerminalUpdateInput) TerminalUpdateResult {
	thread := contracts.CloneMap(input.Thread)
	turnID := strings.TrimSpace(input.TurnID)
	status := strings.TrimSpace(input.Status)
	finishedAt := strings.TrimSpace(input.FinishedAt)
	turns, _ := thread["turns"].([]any)
	for index, value := range turns {
		turn, _ := value.(map[string]any)
		if stringField(turn, "id") != turnID {
			continue
		}
		currentStatus := stringField(turn, "status")
		if input.ProtectTerminal && IsTerminalStatus(currentStatus) {
			return TerminalUpdateResult{Found: true, Applied: false, CurrentStatus: currentStatus}
		}
		turn["status"] = status
		turn["finishedAt"] = finishedAt
		turn["steering"] = CancelPendingSteeringEntries(turn["steering"], finishedAt, status)
		for key, value := range input.Fields {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if value == nil {
				delete(turn, key)
			} else {
				turn[key] = contracts.CloneValue(value)
			}
		}
		SettleItemsForTerminalStatus(turn, status, finishedAt)
		appendTerminalItems(turn, status, finishedAt, input.AppendItems)
		turns[index] = turn
		thread["turns"] = turns
		for key, value := range input.ThreadFields {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if value == nil {
				delete(thread, key)
			} else {
				thread[key] = contracts.CloneValue(value)
			}
		}
		thread["status"] = "idle"
		thread["updatedAt"] = finishedAt
		return TerminalUpdateResult{Thread: thread, Found: true, Applied: true, CurrentStatus: status}
	}
	return TerminalUpdateResult{Found: false}
}

func CancelPendingSteeringEntries(value any, cancelledAt string, reason string) []any {
	entries := listAny(value)
	if len(entries) == 0 {
		return []any{}
	}
	out := make([]any, 0, len(entries))
	for _, raw := range entries {
		entry, _ := raw.(map[string]any)
		if entry == nil {
			continue
		}
		status := strings.TrimSpace(stringField(entry, "status"))
		if status == "" || status == "pending" {
			cancelled := contracts.CloneMap(entry)
			cancelled["status"] = "cancelled"
			cancelled["cancelledAt"] = cancelledAt
			cancelled["cancelReason"] = reason
			out = append(out, cancelled)
			continue
		}
		out = append(out, contracts.CloneMap(entry))
	}
	return out
}

func FindSteeringEntryByID(value any, id string) map[string]any {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	for _, raw := range listAny(value) {
		entry, _ := raw.(map[string]any)
		if entry == nil {
			continue
		}
		entryID := strings.TrimSpace(stringField(entry, "id"))
		clientID := strings.TrimSpace(stringField(entry, "clientUserMessageId"))
		if entryID == id || clientID == id {
			return contracts.CloneMap(entry)
		}
	}
	return nil
}

func TurnAcceptsSteering(turn map[string]any) bool {
	if guiPlan, ok := turn["guiPlan"].(map[string]any); ok && len(guiPlan) > 0 {
		return false
	}
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		switch stringField(item, "kind") {
		case "review", "compaction":
			return false
		}
	}
	return true
}

func appendTerminalItems(turn map[string]any, status string, finishedAt string, appendItems []map[string]any) {
	for _, appendItem := range appendItems {
		if appendItem == nil {
			continue
		}
		items, _ := turn["items"].([]any)
		itemToAppend := contracts.CloneMap(appendItem)
		if strings.TrimSpace(stringField(itemToAppend, "status")) == "" {
			itemToAppend["status"] = status
		}
		if strings.TrimSpace(stringField(itemToAppend, "finishedAt")) == "" {
			itemToAppend["finishedAt"] = finishedAt
		}
		itemID := stringField(itemToAppend, "id")
		replaced := false
		for itemIndex, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if itemID != "" && stringField(item, "id") == itemID {
				items[itemIndex] = itemToAppend
				replaced = true
				break
			}
		}
		if !replaced {
			items = append(items, itemToAppend)
		}
		turn["items"] = items
	}
}
