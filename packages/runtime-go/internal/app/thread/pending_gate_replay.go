package thread

import "analytix.local/runtime-go/internal/contracts"

func PendingGateIDsFromEventsV1(events []map[string]any) ([]string, []string) {
	approvalPending := map[string]bool{}
	inputPending := map[string]bool{}
	approvalTurn := map[string]string{}
	inputTurn := map[string]string{}
	approvalOrder := []string{}
	inputOrder := []string{}
	for _, event := range events {
		switch contracts.StringField(event, "kind") {
		case "approval_requested":
			id := contracts.StringField(event, "approvalId")
			if id == "" {
				continue
			}
			if !approvalPending[id] {
				approvalOrder = append(approvalOrder, id)
			}
			approvalPending[id] = true
			approvalTurn[id] = contracts.StringField(event, "turnId")
		case "approval_resolved":
			id := contracts.StringField(event, "approvalId")
			delete(approvalPending, id)
			delete(approvalTurn, id)
		case "user_input_requested":
			id := contracts.StringField(event, "inputId")
			if id == "" {
				continue
			}
			if !inputPending[id] {
				inputOrder = append(inputOrder, id)
			}
			inputPending[id] = true
			inputTurn[id] = contracts.StringField(event, "turnId")
		case "user_input_resolved":
			id := contracts.StringField(event, "inputId")
			delete(inputPending, id)
			delete(inputTurn, id)
		case "turn_completed", "turn_failed", "turn_aborted":
			turnID := contracts.StringField(event, "turnId")
			if turnID == "" {
				continue
			}
			for id, gateTurnID := range approvalTurn {
				if gateTurnID == turnID {
					delete(approvalPending, id)
					delete(approvalTurn, id)
				}
			}
			for id, gateTurnID := range inputTurn {
				if gateTurnID == turnID {
					delete(inputPending, id)
					delete(inputTurn, id)
				}
			}
		}
	}
	return filterPendingGateOrderV1(approvalOrder, approvalPending), filterPendingGateOrderV1(inputOrder, inputPending)
}

func filterPendingGateOrderV1(order []string, pending map[string]bool) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, id := range order {
		if id == "" || seen[id] || !pending[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
