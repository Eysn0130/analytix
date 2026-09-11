package server

import (
	"context"

	apploop "analytix.local/runtime-go/internal/app/loop"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	visionbridgeapp "analytix.local/runtime-go/internal/app/visionbridge"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func (h *runtimeServerHandler) prepareRuntimeToolResultForModel(
	_ context.Context,
	pending runtimePendingToolCall,
	output any,
	isError bool,
) (domaintoolresult.PublicToolResultProjectionV1, string) {
	privateOutput, publicOutput, _, valid := subagentapp.ProjectForegroundParentToolOutputV1(pending, output)
	return visionbridgeapp.PrepareToolResultForModel(visionbridgeapp.ToolResultProjectionInput{
		ToolName: pending.Call.Name, RawOutput: output, PersistOutput: runtimePersistableToolOutput(pending, output),
		IsError: isError, Config: h.visionBridge, ExactPrivateModelOutput: privateOutput,
		ExactPublicOutput: publicOutput, HasExactPrivateOutput: valid,
	})
}

func runtimePersistableToolOutput(pending runtimePendingToolCall, output any) any {
	return apploop.PersistableToolOutputV1(pending, output, subagentapp.ProjectForegroundParentPublicToolOutputV1)
}
