package control

import "strings"

type RestartGateReplay struct {
	Approvals                 map[string]GateRecord
	Inputs                    map[string]GateRecord
	ClosedApprovals           map[string]GateRecord
	ClosedInputs              map[string]GateRecord
	ClosedApprovalResolutions map[string]RestartGateResolution
	ClosedInputResolutions    map[string]RestartGateResolution
	InvalidGateIDs            map[string]bool
}

type RestartGateResolution struct {
	Record      GateRecord
	Status      string
	CancelledBy string
}

func ReplayPendingGates(threadID string, events []map[string]any) RestartGateReplay {
	replay := RestartGateReplay{
		Approvals:                 map[string]GateRecord{},
		Inputs:                    map[string]GateRecord{},
		ClosedApprovals:           map[string]GateRecord{},
		ClosedInputs:              map[string]GateRecord{},
		ClosedApprovalResolutions: map[string]RestartGateResolution{},
		ClosedInputResolutions:    map[string]RestartGateResolution{},
		InvalidGateIDs:            map[string]bool{},
	}
	threadID = strings.TrimSpace(threadID)
	for _, event := range events {
		if eventThreadID := replayString(event, "threadId"); eventThreadID != "" && eventThreadID != threadID {
			if id := replayGateID(event); id != "" {
				invalidateRestartGate(&replay, id)
			}
			continue
		}
		turnID := replayString(event, "turnId")
		switch replayString(event, "kind") {
		case "approval_requested":
			if approvalID := replayString(event, "approvalId"); approvalID != "" {
				if restartGateIDExists(replay, approvalID) {
					invalidateRestartGate(&replay, approvalID)
					continue
				}
				replay.Approvals[approvalID] = GateRecord{
					ThreadID: threadID, TurnID: turnID, ItemID: replayString(event, "itemId"), ToolName: replayString(event, "toolName"), ContinuationReceiptID: replayString(event, "continuationReceiptId"),
				}
			}
		case "approval_resolved":
			approvalID := replayString(event, "approvalId")
			if record, exists := replay.Approvals[approvalID]; exists {
				if !validRestartResolutionRecord("approval", record, event) {
					invalidateRestartGate(&replay, approvalID)
					continue
				}
				delete(replay.Approvals, approvalID)
				replay.ClosedApprovals[approvalID] = record
				replay.ClosedApprovalResolutions[approvalID] = restartGateResolution(record, event)
			}
		case "user_input_requested":
			if inputID := replayString(event, "inputId"); inputID != "" {
				if restartGateIDExists(replay, inputID) {
					invalidateRestartGate(&replay, inputID)
					continue
				}
				replay.Inputs[inputID] = GateRecord{
					ThreadID: threadID, TurnID: turnID, ItemID: replayString(event, "itemId"), Prompt: replayString(event, "prompt"), ContinuationReceiptID: replayString(event, "continuationReceiptId"),
				}
			}
		case "user_input_resolved":
			inputID := replayString(event, "inputId")
			if record, exists := replay.Inputs[inputID]; exists {
				if !validRestartResolutionRecord("user_input", record, event) {
					invalidateRestartGate(&replay, inputID)
					continue
				}
				delete(replay.Inputs, inputID)
				replay.ClosedInputs[inputID] = record
				replay.ClosedInputResolutions[inputID] = restartGateResolution(record, event)
			}
		case "turn_completed", "turn_failed", "turn_aborted":
			closeRestartGatesForTurn(&replay, threadID, turnID)
		}
	}
	return replay
}

func AddClosedRestartGateTombstones[T any](approvals, inputs map[string]PendingGateState[T], replay RestartGateReplay) {
	for id, record := range replay.ClosedApprovals {
		approvals[id] = PendingGateState[T]{Record: record}
	}
	for id, record := range replay.ClosedInputs {
		inputs[id] = PendingGateState[T]{Record: record}
	}
}

func validRestartResolutionRecord(kind string, record GateRecord, event map[string]any) bool {
	if replayString(event, "turnId") != strings.TrimSpace(record.TurnID) ||
		replayString(event, "itemId") != strings.TrimSpace(record.ItemID) {
		return false
	}
	status := replayString(event, "status")
	switch kind {
	case "approval":
		return status == "allowed" || status == "denied" || status == "expired"
	case "user_input":
		return status == "submitted" || status == "cancelled"
	default:
		return false
	}
}

func restartGateResolution(record GateRecord, event map[string]any) RestartGateResolution {
	return RestartGateResolution{
		Record: record, Status: replayString(event, "status"), CancelledBy: replayString(event, "cancelledBy"),
	}
}

func restartGateIDExists(replay RestartGateReplay, id string) bool {
	if replay.InvalidGateIDs[id] {
		return true
	}
	if _, exists := replay.Approvals[id]; exists {
		return true
	}
	if _, exists := replay.Inputs[id]; exists {
		return true
	}
	if _, exists := replay.ClosedApprovals[id]; exists {
		return true
	}
	_, exists := replay.ClosedInputs[id]
	return exists
}

func invalidateRestartGate(replay *RestartGateReplay, id string) {
	id = strings.TrimSpace(id)
	if replay == nil || id == "" {
		return
	}
	replay.InvalidGateIDs[id] = true
	delete(replay.Approvals, id)
	delete(replay.Inputs, id)
	delete(replay.ClosedApprovals, id)
	delete(replay.ClosedInputs, id)
	delete(replay.ClosedApprovalResolutions, id)
	delete(replay.ClosedInputResolutions, id)
}

func closeRestartGatesForTurn(replay *RestartGateReplay, threadID, turnID string) {
	if replay == nil || strings.TrimSpace(turnID) == "" {
		return
	}
	for id, record := range replay.Approvals {
		if record.ThreadID == threadID && record.TurnID == turnID {
			delete(replay.Approvals, id)
			replay.ClosedApprovals[id] = record
		}
	}
	for id, record := range replay.Inputs {
		if record.ThreadID == threadID && record.TurnID == turnID {
			delete(replay.Inputs, id)
			replay.ClosedInputs[id] = record
		}
	}
}

func replayGateID(event map[string]any) string {
	if id := replayString(event, "approvalId"); id != "" {
		return id
	}
	return replayString(event, "inputId")
}

func DeletePendingGateRecordsForTurn(approvals, inputs map[string]GateRecord, threadID, turnID string) {
	for id, record := range approvals {
		if record.ThreadID == threadID && record.TurnID == turnID {
			delete(approvals, id)
		}
	}
	for id, record := range inputs {
		if record.ThreadID == threadID && record.TurnID == turnID {
			delete(inputs, id)
		}
	}
}

func replayString(value map[string]any, key string) string {
	text, _ := value[key].(string)
	return strings.TrimSpace(text)
}
