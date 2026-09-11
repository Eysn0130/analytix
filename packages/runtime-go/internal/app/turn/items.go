package turn

import (
	"errors"
	"reflect"
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
)

var (
	ErrAcceptedFinalImmutable   = errors.New("accepted-final turn is immutable")
	ErrTurnItemIdentityConflict = errors.New("turn item identity conflicts with an existing projection")
)

type AppendItemInput struct {
	Thread    map[string]any
	TurnID    string
	Item      map[string]any
	UpdatedAt string
}

// EnsureItemToTurnExact applies an append-only item projection with exact
// readback semantics. A committed retry succeeds without adding a duplicate;
// reusing the same item id for different content fails closed.
func EnsureItemToTurnExact(input AppendItemInput) (map[string]any, bool, error) {
	thread := contracts.CloneMap(input.Thread)
	projected, _ := domainevent.ProjectPublicValuePreservingClosedPlanDigestV1(input.Item)
	item, ok := projected.(map[string]any)
	if !ok {
		return nil, false, errors.New("turn item projection is invalid")
	}
	// Normalize through the same JSON boundary as the cloned durable thread.
	// Without this, an in-memory uint64 and its persisted float64 round-trip
	// compare unequal even when they are the same canonical JSON record.
	item = contracts.CloneMap(item)
	itemID := strings.TrimSpace(stringField(item, "id"))
	turnID := strings.TrimSpace(input.TurnID)
	if itemID == "" || turnID == "" {
		return nil, false, errors.New("turn item identity is invalid")
	}
	turns, _ := thread["turns"].([]any)
	for index, value := range turns {
		turn, _ := value.(map[string]any)
		if stringField(turn, "id") != turnID {
			continue
		}
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			existing, _ := rawItem.(map[string]any)
			if strings.TrimSpace(stringField(existing, "id")) != itemID {
				continue
			}
			if reflect.DeepEqual(existing, item) {
				return thread, false, nil
			}
			return nil, false, ErrTurnItemIdentityConflict
		}
		turn["items"] = append(items, contracts.CloneMap(item))
		turns[index] = turn
		thread["turns"] = turns
		thread["updatedAt"] = strings.TrimSpace(input.UpdatedAt)
		return thread, true, nil
	}
	return nil, false, errors.New("turn item owner is unavailable")
}

type PatchItemStatusInput struct {
	Thread     map[string]any
	TurnID     string
	ItemID     string
	Status     string
	FinishedAt string
	UpdatedAt  string
}

type RecoveredToolSettlementInput struct {
	Thread         map[string]any
	ThreadID       string
	TurnID         string
	ToolCallItemID string
	CallID         string
	ToolName       string
	Status         string
	Timestamp      string
	ResultItem     map[string]any
}

// EnsureRecoveredToolSettlementExact applies the recovered tool-call patch
// and its exact tool-result append as one in-memory mutation. Exact replay is
// side-effect free; every identity or byte mismatch fails closed.
func EnsureRecoveredToolSettlementExact(input RecoveredToolSettlementInput) (map[string]any, bool, error) {
	thread := contracts.CloneMap(input.Thread)
	projected, _ := domainevent.ProjectPublicValuePreservingClosedPlanDigestV1(input.ResultItem)
	resultItem, ok := projected.(map[string]any)
	if !ok {
		return nil, false, errors.New("recovered tool result projection is invalid")
	}
	resultItem = contracts.CloneMap(resultItem)
	threadID := strings.TrimSpace(input.ThreadID)
	turnID := strings.TrimSpace(input.TurnID)
	toolCallItemID := strings.TrimSpace(input.ToolCallItemID)
	callID := strings.TrimSpace(input.CallID)
	toolName := strings.TrimSpace(input.ToolName)
	status := strings.TrimSpace(input.Status)
	timestamp := strings.TrimSpace(input.Timestamp)
	resultItemID := strings.TrimSpace(stringField(resultItem, "id"))
	if threadID == "" || turnID == "" || toolCallItemID == "" || callID == "" || toolName == "" ||
		status == "" || timestamp == "" || resultItemID == "" ||
		strings.TrimSpace(stringField(thread, "id")) != threadID ||
		stringField(resultItem, "threadId") != threadID || stringField(resultItem, "turnId") != turnID ||
		stringField(resultItem, "kind") != "tool_result" || stringField(resultItem, "role") != "tool" ||
		stringField(resultItem, "callId") != callID ||
		stringField(resultItem, "toolName") != toolName || stringField(resultItem, "status") != status ||
		stringField(resultItem, "createdAt") != timestamp || stringField(resultItem, "finishedAt") != timestamp {
		return nil, false, errors.New("recovered tool settlement identity is invalid")
	}
	isError, isErrorOK := resultItem["isError"].(bool)
	if !isErrorOK || !isError {
		return nil, false, errors.New("recovered tool settlement lifecycle is invalid")
	}
	turns, _ := thread["turns"].([]any)
	for turnIndex, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		if stringField(turn, "id") != turnID {
			continue
		}
		items, _ := turn["items"].([]any)
		toolCallIndex := -1
		resultExact := false
		for itemIndex, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			itemID := strings.TrimSpace(stringField(item, "id"))
			itemCallID := strings.TrimSpace(stringField(item, "callId"))
			kind := strings.TrimSpace(stringField(item, "kind"))
			if itemID == toolCallItemID || (kind == "tool_call" && itemCallID == callID) {
				if toolCallIndex >= 0 || itemID != toolCallItemID || kind != "tool_call" ||
					itemCallID != callID || stringField(item, "toolName") != toolName ||
					stringField(item, "threadId") != threadID || stringField(item, "turnId") != turnID {
					return nil, false, ErrTurnItemIdentityConflict
				}
				toolCallIndex = itemIndex
			}
			if itemID == resultItemID || (kind == "tool_result" && itemCallID == callID) {
				if itemID != resultItemID || kind != "tool_result" || itemCallID != callID ||
					!reflect.DeepEqual(item, resultItem) || resultExact {
					return nil, false, ErrTurnItemIdentityConflict
				}
				resultExact = true
			}
		}
		if toolCallIndex < 0 {
			return nil, false, ErrTurnItemIdentityConflict
		}
		toolCall, _ := items[toolCallIndex].(map[string]any)
		if resultExact {
			if stringField(toolCall, "status") != status || stringField(toolCall, "finishedAt") != timestamp {
				return nil, false, ErrTurnItemIdentityConflict
			}
			return thread, false, nil
		}
		if turn["generalTerminalCASBinding"] != nil || turn["generalTerminalPublication"] != nil {
			return nil, false, ErrAcceptedFinalImmutable
		}
		if err := ValidatePatchTurnItemStatus(thread, turnID); err != nil {
			return nil, false, err
		}
		toolCall["status"] = status
		toolCall["finishedAt"] = timestamp
		items[toolCallIndex] = toolCall
		turn["items"] = append(items, contracts.CloneMap(resultItem))
		turns[turnIndex] = turn
		thread["turns"] = turns
		thread["updatedAt"] = timestamp
		return thread, true, nil
	}
	return nil, false, errors.New("recovered tool settlement owner is unavailable")
}

// ValidatePatchTurnItemStatus rejects every late item mutation once the raw
// turn contains accepted-final authority. Even an idempotent-looking patch
// would change timestamps and invalidate the V2 TurnCASDigest.
func ValidatePatchTurnItemStatus(thread map[string]any, turnID string) error {
	turnID = strings.TrimSpace(turnID)
	turns, _ := thread["turns"].([]any)
	for _, value := range turns {
		turn, _ := value.(map[string]any)
		if stringField(turn, "id") != turnID {
			continue
		}
		if turn["acceptedFinal"] != nil {
			return ErrAcceptedFinalImmutable
		}
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if item["acceptedFinal"] != nil {
				return ErrAcceptedFinalImmutable
			}
		}
		return nil
	}
	return nil
}

func AppendItemToTurn(input AppendItemInput) (map[string]any, bool) {
	thread := contracts.CloneMap(input.Thread)
	projected, _ := domainevent.ProjectPublicValuePreservingClosedPlanDigestV1(input.Item)
	item, ok := projected.(map[string]any)
	if !ok || item == nil {
		return thread, false
	}
	turnID := strings.TrimSpace(input.TurnID)
	turns, _ := thread["turns"].([]any)
	for index, value := range turns {
		turn, _ := value.(map[string]any)
		if stringField(turn, "id") != turnID {
			continue
		}
		items, _ := turn["items"].([]any)
		turn["items"] = append(items, contracts.CloneMap(item))
		turns[index] = turn
		thread["turns"] = turns
		thread["updatedAt"] = strings.TrimSpace(input.UpdatedAt)
		return thread, true
	}
	return nil, false
}

func PatchTurnItemStatus(input PatchItemStatusInput) (map[string]any, bool) {
	thread := contracts.CloneMap(input.Thread)
	turnID := strings.TrimSpace(input.TurnID)
	itemID := strings.TrimSpace(input.ItemID)
	finishedAt := strings.TrimSpace(input.FinishedAt)
	updatedAt := strings.TrimSpace(input.UpdatedAt)
	if updatedAt == "" {
		updatedAt = finishedAt
	}
	turns, _ := thread["turns"].([]any)
	for turnIndex, value := range turns {
		turn, _ := value.(map[string]any)
		if stringField(turn, "id") != turnID {
			continue
		}
		items, _ := turn["items"].([]any)
		for itemIndex, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "id") != itemID {
				continue
			}
			item["status"] = strings.TrimSpace(input.Status)
			item["finishedAt"] = finishedAt
			items[itemIndex] = item
			turn["items"] = items
			turns[turnIndex] = turn
			thread["turns"] = turns
			thread["updatedAt"] = updatedAt
			return thread, true
		}
	}
	return nil, false
}
