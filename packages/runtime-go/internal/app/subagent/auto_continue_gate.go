package subagent

import (
	"strings"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

func BackgroundAutoContinueGate(record domainjob.Record, thread map[string]any, records []domainjob.Record) string {
	if !record.Background || !record.AutoContinueParent || strings.TrimSpace(record.Status) != string(domainjob.StatusCompleted) {
		return "job_not_completed"
	}
	if record.LateCompletionSuppressed {
		return "late_completion_suppressed"
	}
	if strings.TrimSpace(record.ParentThreadID) == "" || strings.TrimSpace(record.ParentTurnID) == "" {
		return "missing_parent_lineage"
	}
	if thread == nil {
		return "parent_thread_missing"
	}
	if status := strings.TrimSpace(mapString(thread, "status")); status != "" && status != "idle" {
		return "parent_not_idle"
	}
	turns := autoContinueList(thread["turns"])
	if len(turns) == 0 {
		return "parent_turn_missing"
	}
	lastTurn, _ := turns[len(turns)-1].(map[string]any)
	if mapString(lastTurn, "id") != strings.TrimSpace(record.ParentTurnID) {
		return "parent_turn_not_latest"
	}
	parentTurn, ok := FindTurn(thread, record.ParentTurnID)
	if !ok {
		return "parent_turn_missing"
	}
	if status := strings.TrimSpace(mapString(parentTurn, "status")); status != "" && status != "completed" {
		return "parent_turn_not_completed"
	}
	if ThreadHasPendingGate(thread) {
		return "parent_has_pending_gate"
	}
	return BackgroundAutoContinueSiblingGate(record, records)
}

func FindTurn(thread map[string]any, turnID string) (map[string]any, bool) {
	turnID = strings.TrimSpace(turnID)
	for _, rawTurn := range autoContinueList(thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		if turn != nil && mapString(turn, "id") == turnID {
			return turn, true
		}
	}
	return nil, false
}

func ThreadHasPendingGate(thread map[string]any) bool {
	for _, rawTurn := range autoContinueList(thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		if turn == nil {
			continue
		}
		if status := strings.TrimSpace(mapString(turn, "status")); status == "queued" || status == "running" || status == "waiting" {
			return true
		}
		for _, rawItem := range autoContinueList(turn["items"]) {
			item, _ := rawItem.(map[string]any)
			if item == nil {
				continue
			}
			kind := strings.TrimSpace(mapString(item, "kind"))
			status := strings.TrimSpace(mapString(item, "status"))
			if (kind == "approval" || kind == "user_input") && (status == "pending" || status == "running" || status == "waiting") {
				return true
			}
		}
	}
	return false
}

func BackgroundAutoContinueSiblingGate(record domainjob.Record, records []domainjob.Record) string {
	for _, other := range records {
		if other.ID == record.ID || strings.TrimSpace(other.ParentThreadID) != strings.TrimSpace(record.ParentThreadID) ||
			strings.TrimSpace(other.ParentTurnID) != strings.TrimSpace(record.ParentTurnID) || !other.AutoContinueParent {
			continue
		}
		switch strings.TrimSpace(other.AutoContinueStatus) {
		case "started":
			return "auto_continue_already_started"
		case "starting":
			return "auto_continue_already_starting"
		}
	}
	return ""
}

func autoContinueList(value any) []any {
	values, _ := value.([]any)
	return values
}
