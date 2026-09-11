package server

import (
	"context"

	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func (h *runtimeServerHandler) runtimeCapabilities() map[string]any {
	input := runtimeinfoapp.RuntimeCapabilitiesInput{
		DefaultModel: h.runtimeDefaultModelCapabilityConfig(),
		MCP:          runtimeinfoapp.MCPCapabilityState(h.mcpSearch, h.mcp.Diagnostics()),
		Skills:       h.skillCapabilityState(),
		Subagents:    subagentapp.CapabilityState(h.subagents, h.jobs != nil, h.runtimeSubagentToolNames()),
		Web:          runtimeinfoapp.WebCapabilityState(h.web),
		VisionBridge: h.runtimeVisionBridgeCapabilityState(),
		ComputerUse:  h.runtimeComputerUseCapabilityState(),
	}
	if h.attachments != nil {
		input.Attachments = h.runtimeAttachmentCapabilityState()
	}
	if h.memories != nil {
		input.Memory = h.runtimeMemoryCapabilityState()
	}
	capabilities := runtimeinfoapp.RuntimeCapabilities(input)
	return capabilities
}

func (h *runtimeServerHandler) runtimeAttachmentCapabilityState() map[string]any {
	return runtimeinfoapp.AttachmentCapabilityState(
		domainmodel.AttachmentMaxImageBytes,
		domainmodel.AttachmentMaxImageDimension,
		domainmodel.AttachmentImageMimeTypes,
		domainmodel.AttachmentDocumentMimeTypes,
		domainmodel.AttachmentMaxDocumentBytes,
		domainmodel.AttachmentMaxDocumentTextChars,
		domainmodel.AttachmentTextFallbackMaxBase64Bytes,
		domainmodel.AttachmentTextFallbackMaxImageDimension,
		domainmodel.AttachmentTextFallbackPreferredMimeType,
	)
}

func (h *runtimeServerHandler) runtimeMemoryCapabilityState() map[string]any {
	return runtimeinfoapp.MemoryCapabilityState()
}

func (h *runtimeServerHandler) runtimeComputerUseCapabilityState() map[string]any {
	if h == nil || h.mcp == nil {
		return runtimeinfoapp.ComputerUseCapabilityState(nil)
	}
	return runtimeinfoapp.ComputerUseCapabilityState(h.mcp.ServerDiagnostics())
}

func (h *runtimeServerHandler) runtimeVisionBridgeCapabilityState() map[string]any {
	return runtimeinfoapp.VisionBridgeCapabilityState(runtimeinfoapp.NormalizeVisionBridgeConfig(h.visionBridge))
}

func (h *runtimeServerHandler) runtimeVisionBridgeReadyReason() (bool, string) {
	return runtimeinfoapp.VisionBridgeReadyReason(runtimeinfoapp.NormalizeVisionBridgeConfig(h.visionBridge))
}

func (h *runtimeServerHandler) runtimeDefaultModelCapabilityConfig() runtimeinfoapp.ModelCapabilityConfig {
	return runtimeinfoapp.ModelCapabilityConfigFromTurnConfig(h.providerConfig.TurnConfig("", ""))
}

func (h *runtimeServerHandler) runtimeMCPSearchDiagnostics(mcpDiagnostics map[string]any) map[string]any {
	return runtimeinfoapp.MCPSearchDiagnostics(h.mcpSearch, mcpDiagnostics)
}

func (h *runtimeServerHandler) acquireRuntimeSubagentSlot(ctx context.Context) (func(), error) {
	return h.runtimeSubagentState().AcquireSlot(ctx, subagentapp.MaxParallel(h.subagents))
}

func (h *runtimeServerHandler) runtimeSubagentToolDiagnostics(threadID string) map[string]any {
	records, _ := h.runtimeSubagentService().ListTaskJobs(threadID)
	return subagentapp.ScopedToolDiagnostics(h.subagents, h.jobs != nil, h.runtimeSubagentToolNames(), records)
}

func (h *runtimeServerHandler) runtimeGlobalSubagentToolDiagnostics() map[string]any {
	return subagentapp.ToolDiagnostics(h.subagents, h.jobs != nil, h.runtimeSubagentToolNames(), 0, 0, nil)
}

func (h *runtimeServerHandler) runtimeSubagentToolNames() []string {
	toolNames := []string{}
	for _, tool := range h.runtimeToolSchemasFor(true, nil, false, false) {
		toolNames = append(toolNames, tool.Name)
	}
	return toolNames
}
