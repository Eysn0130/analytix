package gateprojection

import domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"

func PendingGateIDs(thread map[string]any) ([]string, []string) {
	return domaincontinuation.PendingGateIDs(thread)
}
