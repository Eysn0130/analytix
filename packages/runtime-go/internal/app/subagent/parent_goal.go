package subagent

import (
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
)

func ParentGoalIdentity(goal map[string]any, threadID string) (string, string) {
	threadID = strings.TrimSpace(threadID)
	if goal != nil {
		goalID := strings.TrimSpace(firstNonEmptyAnyString(goal["id"]))
		if goalID == "" {
			goalID = "goal_" + contracts.SafeRecordID(threadID)
		}
		return goalID, strings.TrimSpace(firstNonEmptyAnyString(goal["objective"]))
	}
	return "thread_" + contracts.SafeRecordID(threadID), ""
}
