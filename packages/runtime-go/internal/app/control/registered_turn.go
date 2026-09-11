package control

import (
	"context"
	"errors"
)

// RunRegisteredTurn binds a resumed approval/user-input execution to the same
// cancellable ownership registry as a foreground provider turn. The entry is
// retained until the whole continuation, including terminal persistence, has
// returned.
func (c *Controller) RunRegisteredTurn(ctx context.Context, threadID, turnID string, run func(context.Context) ActionResult) ActionResult {
	if ctx == nil || run == nil {
		return ActionResult{StatusCode: 409, Body: map[string]any{"code": "turn_execution_conflict", "message": "turn execution authority is unavailable"}}
	}
	turnCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := c.RegisterTurnCancelWithError(threadID, turnID, cancel); err != nil {
		if errors.Is(err, ErrRuntimeShuttingDown) {
			return ActionResult{StatusCode: 503, Body: map[string]any{"code": "runtime_shutting_down", "message": "runtime is shutting down"}}
		}
		return ActionResult{StatusCode: 409, Body: map[string]any{"code": "turn_execution_conflict", "message": "turn execution is already active"}}
	}
	defer c.UnregisterTurnCancel(threadID, turnID)
	return run(turnCtx)
}
