package server

import (
	"encoding/json"
	"testing"

	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type runtimeComputerUseCapabilityMCPStub struct {
	diagnostics []any
	tools       []string
}

func (m runtimeComputerUseCapabilityMCPStub) Connect() {}

func (m runtimeComputerUseCapabilityMCPStub) Diagnostics() map[string]any {
	return map[string]any{
		"available":             len(m.diagnostics) > 0,
		"reason":                "configured MCP servers are connected",
		"configuredServerCount": float64(len(m.diagnostics)),
		"connectedServerCount":  float64(len(m.diagnostics)),
		"indexedToolCount":      float64(len(m.tools)),
		"advertisedToolCount":   float64(len(m.tools)),
		"promptCount":           float64(0),
		"resourceCount":         float64(0),
		"catalogFingerprint":    "test-catalog",
		"catalogDrift":          false,
	}
}

func (m runtimeComputerUseCapabilityMCPStub) ServerDiagnostics() []any {
	return append([]any(nil), m.diagnostics...)
}

func (m runtimeComputerUseCapabilityMCPStub) Search(string) []string { return nil }

func (m runtimeComputerUseCapabilityMCPStub) CallTool(string, bool, ...map[string]any) map[string]any {
	return map[string]any{"executed": false}
}

func (m runtimeComputerUseCapabilityMCPStub) RefreshCatalog() map[string]any { return m.Diagnostics() }

func (m runtimeComputerUseCapabilityMCPStub) RestartReconnect() map[string]any {
	return m.Diagnostics()
}

func (m runtimeComputerUseCapabilityMCPStub) Disconnect() {}

func (m runtimeComputerUseCapabilityMCPStub) Tools() []string {
	return append([]string(nil), m.tools...)
}

func (m runtimeComputerUseCapabilityMCPStub) MCPToolAdvertisementSnapshotV1(domainsecurity.TurnSecurityContext) []domainmcp.ToolAdvertisementV1 {
	return nil
}

func (m runtimeComputerUseCapabilityMCPStub) Prompts() []any { return nil }

func (m runtimeComputerUseCapabilityMCPStub) Resources() []any { return nil }

func (m runtimeComputerUseCapabilityMCPStub) ToolReadOnlyHint(string) bool { return false }

func (m runtimeComputerUseCapabilityMCPStub) ToolInputSchema(string) (json.RawMessage, bool) {
	return nil, false
}

func (m runtimeComputerUseCapabilityMCPStub) ToolOutputSchema(string) (json.RawMessage, bool) {
	return nil, false
}

func (m runtimeComputerUseCapabilityMCPStub) ToolDescription(string) (string, bool) {
	return "", false
}

func TestRuntimeComputerUseCapabilityReflectsConnectedMCPBackend(t *testing.T) {
	tools := []string{
		"mcp__analytix-computer-use__get_app_state",
		"mcp__analytix-computer-use__select_text",
		"mcp__analytix-computer-use__click",
	}
	handler := &runtimeServerHandler{
		mcp: runtimeComputerUseCapabilityMCPStub{
			tools: tools,
			diagnostics: []any{map[string]any{
				"id":        "analytix-computer-use",
				"enabled":   true,
				"available": true,
				"connected": true,
				"toolCount": float64(len(tools)),
				"toolNames": tools,
			}},
		},
	}

	state, ok := handler.runtimeCapabilities()["computerUse"].(map[string]any)
	if !ok {
		t.Fatalf("computerUse capability missing")
	}
	if state["status"] != "available" || state["available"] != true || state["enabled"] != true {
		t.Fatalf("connected Analytix Computer Use backend should be available: %#v", state)
	}
	if state["backendId"] != "analytix-computer-use" || state["provider"] != "mcp" {
		t.Fatalf("computerUse backend metadata mismatch: %#v", state)
	}
}

func TestRuntimeComputerUseCapabilityRequiresStateAndActionTools(t *testing.T) {
	handler := &runtimeServerHandler{
		mcp: runtimeComputerUseCapabilityMCPStub{
			tools: []string{"mcp__analytix-computer-use__get_app_state"},
			diagnostics: []any{map[string]any{
				"id":        "analytix-computer-use",
				"enabled":   true,
				"available": true,
				"connected": true,
				"toolCount": float64(1),
				"toolNames": []string{"mcp__analytix-computer-use__get_app_state"},
			}},
		},
	}

	state, ok := handler.runtimeCapabilities()["computerUse"].(map[string]any)
	if !ok {
		t.Fatalf("computerUse capability missing")
	}
	if state["available"] == true || state["status"] == "available" {
		t.Fatalf("computerUse should not be available without an action tool: %#v", state)
	}
	if state["reason"] != "computer-use MCP backend is missing required tools" {
		t.Fatalf("unexpected unavailable reason: %#v", state)
	}
}
