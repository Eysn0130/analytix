package goal

import (
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
)

func BuildUpdatedEvent(threadID string, goal map[string]any) map[string]any {
	return map[string]any{
		"kind":     "goal_updated",
		"threadId": strings.TrimSpace(threadID),
		"goal":     contracts.CloneMap(goal),
	}
}

func BuildTodosUpdatedEvent(threadID string, todos map[string]any) map[string]any {
	return map[string]any{
		"kind":     "todos_updated",
		"threadId": strings.TrimSpace(threadID),
		"todos":    contracts.CloneMap(todos),
	}
}
