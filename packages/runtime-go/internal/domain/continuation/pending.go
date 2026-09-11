package continuation

import "strings"

// PendingGateIDs derives the stable approval and user-input identities from
// an untrusted thread snapshot. It returns no authority; callers must still
// validate the corresponding continuation receipts before resuming work.
func PendingGateIDs(thread map[string]any) ([]string, []string) {
	approvalIDs := []string{}
	inputIDs := []string{}
	seenApprovals := map[string]bool{}
	seenInputs := map[string]bool{}
	turns, _ := thread["turns"].([]any)
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if continuationField(item, "status") != "pending" {
				continue
			}
			switch continuationField(item, "kind") {
			case KindApproval:
				appendPendingGateID(&approvalIDs, seenApprovals, continuationField(item, "approvalId"), continuationField(item, "id"))
			case KindUserInput:
				appendPendingGateID(&inputIDs, seenInputs, continuationField(item, "inputId"), continuationField(item, "id"))
			}
		}
	}
	return approvalIDs, inputIDs
}

func appendPendingGateID(ids *[]string, seen map[string]bool, values ...string) {
	for _, value := range values {
		id := strings.TrimSpace(value)
		if id == "" {
			continue
		}
		if !seen[id] {
			seen[id] = true
			*ids = append(*ids, id)
		}
		return
	}
}

func continuationField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return strings.TrimSpace(value)
}
