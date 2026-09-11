package control

import appturn "analytix.local/runtime-go/internal/app/turn"

type DrainedGateCancellationInput[T any] struct {
	ThreadID        string
	TurnID          string
	Drained         []DrainedGate[T]
	PendingToolName func(T) string
}

type PendingGateCancellation struct {
	Kind   string
	ID     string
	Record appturn.PendingGateCancellation
}

func ApprovalGateCancellation(id string, record GateRecord) PendingGateCancellation {
	return PendingGateCancellation{
		Kind: "approval",
		ID:   id,
		Record: appturn.PendingGateCancellation{
			Kind:     "approval",
			ID:       id,
			ThreadID: record.ThreadID,
			TurnID:   record.TurnID,
			ItemID:   record.ItemID,
			ToolName: GateRecordToolName(record),
		},
	}
}

func UserInputGateCancellation(id string, record GateRecord) PendingGateCancellation {
	return PendingGateCancellation{
		Kind: "user_input",
		ID:   id,
		Record: appturn.PendingGateCancellation{
			Kind:     "user_input",
			ID:       id,
			ThreadID: record.ThreadID,
			TurnID:   record.TurnID,
			ItemID:   record.ItemID,
			Prompt:   GateRecordPrompt(record),
		},
	}
}

func PendingGateCancellationsFromDrained[T any](input DrainedGateCancellationInput[T]) []PendingGateCancellation {
	cancellations := []PendingGateCancellation{}
	for _, drained := range input.Drained {
		switch drained.Kind {
		case "approval":
			record := drained.Record
			if record.ThreadID == "" {
				record = GateRecord{
					ThreadID: input.ThreadID,
					TurnID:   input.TurnID,
					ItemID:   "item_" + drained.ID,
					ToolName: pendingToolName(input.PendingToolName, drained.Pending),
				}
			}
			cancellations = append(cancellations, ApprovalGateCancellation(drained.ID, record))
		case "user_input":
			record := drained.Record
			if record.ThreadID == "" {
				record = GateRecord{
					ThreadID: input.ThreadID,
					TurnID:   input.TurnID,
					ItemID:   "item_" + drained.ID,
					Prompt:   "User input required",
				}
			}
			cancellations = append(cancellations, UserInputGateCancellation(drained.ID, record))
		}
	}
	return cancellations
}

func PendingGateCancellationEvents(cancellations []PendingGateCancellation, reason string) []map[string]any {
	records := make([]appturn.PendingGateCancellation, 0, len(cancellations))
	for _, cancellation := range cancellations {
		records = append(records, cancellation.Record)
	}
	return appturn.PendingGateCancellationEvents(records, reason)
}

func pendingToolName[T any](toolName func(T) string, pending T) string {
	if toolName == nil {
		return ""
	}
	return toolName(pending)
}
