package turn

func IsTerminalStatus(status string) bool {
	switch status {
	case "completed", "failed", "aborted", "interrupted", "killed":
		return true
	default:
		return false
	}
}

func ThreadHasActiveTurn(thread map[string]any) bool {
	turns, _ := thread["turns"].([]any)
	for _, raw := range turns {
		turn, _ := raw.(map[string]any)
		status, _ := turn["status"].(string)
		if status == "running" || status == "queued" || status == "waiting" {
			return true
		}
	}
	return false
}

func SettleItemsForTerminalStatus(turn map[string]any, turnStatus string, finishedAt string) {
	if turnStatus == "completed" {
		return
	}
	items, _ := turn["items"].([]any)
	changed := false
	for index, raw := range items {
		item, _ := raw.(map[string]any)
		if item == nil {
			continue
		}
		status := stringField(item, "status")
		if status != "running" && status != "pending" {
			continue
		}
		switch stringField(item, "kind") {
		case "approval":
			if status != "pending" {
				continue
			}
			item["status"] = "expired"
		case "user_input":
			if status != "pending" {
				continue
			}
			item["status"] = "cancelled"
		default:
			item["status"] = turnStatus
		}
		item["finishedAt"] = finishedAt
		items[index] = item
		changed = true
	}
	if changed {
		turn["items"] = items
	}
}
