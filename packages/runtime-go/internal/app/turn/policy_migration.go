package turn

import (
	"strings"

	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
)

type PendingGateSnapshot func(string) ([]string, []string, error)

func ThreadPolicyMigrationState(threadID string, thread map[string]any, snapshot PendingGateSnapshot) (int, bool) {
	version := numericPolicyVersion(thread["executionPolicyVersion"])
	if strings.TrimSpace(stringPolicyField(thread, "status")) != "idle" || snapshot == nil {
		return version, false
	}
	pendingApprovals, pendingInputs := domaincontinuation.PendingGateIDs(thread)
	replayApprovals, replayInputs, err := snapshot(threadID)
	return version, err == nil && len(pendingApprovals) == 0 && len(pendingInputs) == 0 && len(replayApprovals) == 0 && len(replayInputs) == 0
}

func numericPolicyVersion(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func stringPolicyField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}
