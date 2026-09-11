package server

import (
	"net/http"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
	terminalapp "analytix.local/runtime-go/internal/app/terminal"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	provider "analytix.local/runtime-go/internal/provider"
)

func (h *runtimeServerHandler) handleRuntimeInfo(w http.ResponseWriter, r *http.Request) {
	httpapi.RuntimeDiagnosticsHandlers{Service: runtimeDiagnosticsHTTPService{handler: h}}.HandleInfo(w, r)
}

func (h *runtimeServerHandler) handleRuntimeTools(w http.ResponseWriter, r *http.Request) {
	httpapi.RuntimeDiagnosticsHandlers{Service: runtimeDiagnosticsHTTPService{handler: h}}.HandleTools(w, r)
}

type runtimeDiagnosticsHTTPService struct {
	handler *runtimeServerHandler
}

func (s runtimeDiagnosticsHTTPService) RuntimeInfo() runtimeinfoapp.PublicRuntimeInfoV2 {
	h := s.handler
	return runtimeinfoapp.RuntimeInfo(runtimeinfoapp.RuntimeInfoInput{
		StartedAt:       h.startedAt,
		Host:            h.host,
		Port:            h.port,
		DataDir:         h.infoDataDir,
		Insecure:        h.insecure,
		ApprovalPolicy:  h.approvalPolicy,
		SandboxMode:     h.sandboxMode,
		NetworkProxy:    h.runtimeNetworkProxyDiagnostics(),
		DefaultProvider: h.providerConfig.TurnConfig("", ""),
		Capabilities:    h.runtimeCapabilities(),
	})
}

func (s runtimeDiagnosticsHTTPService) RuntimeTools(request httpapi.RuntimeToolsRequest) runtimeinfoapp.PublicRuntimeToolsV2 {
	h := s.handler
	mcpDiagnostics := h.mcp.Diagnostics()
	if request.Refresh {
		mcpDiagnostics = h.mcp.RefreshCatalog()
	}
	return runtimeinfoapp.RuntimeToolsDiagnostics(runtimeinfoapp.RuntimeToolsDiagnosticsInput{
		Providers:     mapsListAny(h.providerConfig.Diagnostics()),
		ToolContracts: h.runtimeToolContractDiagnostics(),
		MCPServers:    h.mcp.ServerDiagnostics(),
		MCPSearch:     h.runtimeMCPSearchDiagnostics(mcpDiagnostics),
		MCPPrompts:    h.mcp.Prompts(),
		MCPResources:  h.mcp.Resources(),
		Commands:      terminalapp.CommandDiagnostics(h.commandProbe, h.commandHomeDir),
		NetworkProxy:  h.runtimeNetworkProxyDiagnostics(),
		WebProviders:  runtimeinfoapp.WebProviderDiagnostics(h.web),
		Skills:        h.skillToolDiagnostics(),
		Attachments:   h.attachmentDiagnostics(),
		Memory:        h.memoryDiagnostics(),
		Subagents:     h.runtimeGlobalSubagentToolDiagnostics(),
	})
}

func (h *runtimeServerHandler) runtimeToolContractDiagnostics() []any {
	liveMCPToolNames := toolcatalogapp.LiveToolNames(h.mcp)
	tools := h.runtimeToolSchemasForPromptWithGoalToolsAndMCPNames(false, nil, false, false, runtimeToolSchemaDiagnosticsKey, h.runtimeGoalToolsPromptActive(runtimeToolSchemaDiagnosticsKey, nil), liveMCPToolNames)
	return toolcatalogapp.ToolContractDiagnostics(tools)
}

func (h *runtimeServerHandler) runtimeNetworkProxyDiagnostics() map[string]any {
	return provider.NetworkProxyDiagnostics(h.modelProxyURL)
}
