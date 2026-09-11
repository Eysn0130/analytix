package control

const SafeInternalControlMessage = "The runtime request could not be completed safely."

func GateTerminalActionResult(successBody map[string]any, err error) ActionResult {
	if err != nil {
		return ActionResult{
			StatusCode: 500,
			Body:       map[string]any{"code": "turn_failed", "message": SafeInternalControlMessage},
		}
	}
	return ActionResult{StatusCode: 200, Body: successBody}
}
