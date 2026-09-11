package thread

import (
	"errors"
	"fmt"
	"strings"

	"analytix.local/runtime-go/internal/contracts"
)

// RehydrateSummaryIndex treats an index as an ID/order hint only. Every
// returned projection is rebuilt from the current durable thread so stale or
// tampered summary text cannot become read authority.
func RehydrateSummaryIndex(
	indexed []map[string]any,
	readThread func(string) (map[string]any, error),
) ([]map[string]any, error) {
	if readThread == nil {
		return nil, errors.New("summary index authority dependencies are required")
	}
	summaries := make([]map[string]any, 0, len(indexed))
	for _, indexedSummary := range indexed {
		threadID := strings.TrimSpace(contracts.StringField(indexedSummary, "id"))
		if threadID == "" || contracts.SafeRecordID(threadID) != threadID {
			return nil, errors.New("thread summary index contains an invalid thread id")
		}
		thread, err := readThread(threadID)
		if err != nil {
			return nil, fmt.Errorf("read indexed thread %s: %w", threadID, err)
		}
		if thread == nil || strings.TrimSpace(contracts.StringField(thread, "id")) != threadID {
			return nil, fmt.Errorf("indexed thread %s is missing or has mismatched identity", threadID)
		}
		summaries = append(summaries, SummaryIndexProjection(thread))
	}
	return summaries, nil
}

func SummaryIndexProjection(thread map[string]any) map[string]any {
	summary := contracts.ThreadSummary(thread)
	for _, key := range []string{"archived", "pinned", "caseProjectId", "caseId"} {
		if value, ok := thread[key]; ok {
			summary[key] = contracts.CloneValue(value)
		}
	}
	turns, _ := thread["turns"].([]any)
	for index := len(turns) - 1; index >= 0; index-- {
		turn, _ := turns[index].(map[string]any)
		if turnID := strings.TrimSpace(contracts.StringField(turn, "id")); turnID != "" {
			summary["latestTurnId"] = turnID
			break
		}
	}
	if strings.EqualFold(contracts.StringField(thread, "status"), "running") || threadHasRunningTurn(turns) {
		summary["hasRunningTurn"] = true
	}
	status := strings.TrimSpace(contracts.StringField(summary, "status"))
	if strings.EqualFold(status, "archived") {
		summary["archived"] = true
	}
	return summary
}

func threadHasRunningTurn(turns []any) bool {
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		status := strings.TrimSpace(contracts.StringField(turn, "status"))
		if status == "running" || status == "queued" {
			return true
		}
	}
	return false
}
