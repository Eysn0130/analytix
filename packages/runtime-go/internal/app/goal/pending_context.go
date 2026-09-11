package goal

import appmodel "analytix.local/runtime-go/internal/app/model"

func PendingToolContextFrom(pending appmodel.PendingToolCall) PendingToolContext {
	return PendingToolContext{ThreadID: pending.ThreadID, TurnID: pending.TurnID, ToolCallID: pending.Call.ID}
}
