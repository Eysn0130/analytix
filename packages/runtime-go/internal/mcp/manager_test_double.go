//go:build !analytix_prod

package mcp

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type ContractReplayManager struct {
	mu              sync.Mutex
	connected       bool
	specs           []ServerSpec
	tools           map[string]managedTool
	calls           int
	refreshes       int
	restarts        int
	lastRefreshedAt string
	catalogHash     string
	catalogDrift    bool
	failures        map[string]string
}

func NewManager(specs []ServerSpec) *ContractReplayManager {
	return &ContractReplayManager{specs: normalizeSpecs(specs)}
}

func (m *ContractReplayManager) Connect() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectNoLock()
}

func (m *ContractReplayManager) connectNoLock() {
	m.connected = true
	m.failures = map[string]string{}
	m.tools = map[string]managedTool{}
	for _, spec := range normalizeSpecs(m.specs) {
		for _, tool := range toolsForSpec(spec) {
			if spec.ReadOnlyToolNames[tool.Name] {
				tool.ReadOnlyHint = true
			}
			namespaced := CanonicalToolName(spec.ID, tool.Name)
			m.tools[namespaced] = managedTool{ServerID: spec.ID, Name: namespaced, RawName: tool.Name, Tool: tool}
		}
	}
	m.lastRefreshedAt = time.Now().UTC().Format(time.RFC3339Nano)
	m.catalogHash = catalogFingerprint(m.sortedToolsNoLock())
}

func (m *ContractReplayManager) Search(query string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.connected || strings.TrimSpace(query) == "" {
		return nil
	}
	query = strings.ToLower(strings.TrimSpace(query))
	matches := []string{}
	for _, name := range m.sortedToolsNoLock() {
		tool := m.tools[name].Tool
		if strings.Contains(strings.ToLower(name+" "+tool.Description), query) {
			matches = append(matches, name)
		}
	}
	return matches
}

func (m *ContractReplayManager) ToolReadOnlyHint(toolName string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	tool, ok := m.tools[strings.TrimSpace(toolName)]
	if !ok {
		return false
	}
	for _, spec := range m.specs {
		if spec.ID == tool.ServerID {
			return spec.ReadOnlyToolNames[tool.RawName]
		}
	}
	return false
}

func (m *ContractReplayManager) ToolInputSchema(toolName string) (json.RawMessage, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tool, ok := m.tools[strings.TrimSpace(toolName)]
	if !ok || len(tool.Tool.InputSchema) == 0 {
		return nil, false
	}
	return append(json.RawMessage(nil), tool.Tool.InputSchema...), true
}

func (m *ContractReplayManager) ToolOutputSchema(toolName string) (json.RawMessage, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tool, ok := m.tools[strings.TrimSpace(toolName)]
	if !ok || len(tool.Tool.OutputSchema) == 0 {
		return nil, false
	}
	return append(json.RawMessage(nil), tool.Tool.OutputSchema...), true
}

func (m *ContractReplayManager) ToolDescription(toolName string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tool, ok := m.tools[strings.TrimSpace(toolName)]
	if !ok || strings.TrimSpace(tool.Tool.Description) == "" {
		return "", false
	}
	return strings.TrimSpace(tool.Tool.Description), true
}

func (m *ContractReplayManager) CallTool(toolName string, approved bool, arguments ...map[string]any) map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	toolName = strings.TrimSpace(toolName)
	if !m.connected {
		return map[string]any{"toolName": toolName, "executed": false, "code": "mcp_not_connected"}
	}
	tool, ok := m.tools[toolName]
	if !ok {
		return map[string]any{"toolName": toolName, "executed": false, "code": "mcp_tool_not_found"}
	}
	if !approved {
		return map[string]any{"toolName": toolName, "executed": false, "code": "approval_denied"}
	}
	m.calls++
	return map[string]any{
		"toolName":     toolName,
		"serverId":     tool.ServerID,
		"executed":     true,
		"result":       firstNonEmpty(tool.Tool.ResultText, "MCP contract result"),
		"readOnlyHint": tool.Tool.ReadOnlyHint,
	}
}

func (m *ContractReplayManager) RefreshCatalog() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	before := m.catalogHash
	if !m.connected {
		m.connectNoLock()
	}
	m.refreshes++
	m.lastRefreshedAt = time.Now().UTC().Format(time.RFC3339Nano)
	m.catalogHash = catalogFingerprint(m.sortedToolsNoLock())
	m.catalogDrift = before != "" && before != m.catalogHash
	return m.diagnosticsNoLock()
}

func (m *ContractReplayManager) RestartReconnect() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = false
	m.restarts++
	m.connectNoLock()
	m.refreshes++
	return m.diagnosticsNoLock()
}

func (m *ContractReplayManager) Disconnect() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = false
}

func (m *ContractReplayManager) Diagnostics() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.connected {
		m.connectNoLock()
	}
	return m.diagnosticsNoLock()
}

func (m *ContractReplayManager) ServerDiagnostics() []any {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.connected {
		m.connectNoLock()
	}
	out := make([]any, 0, len(normalizeSpecs(m.specs)))
	for _, spec := range normalizeSpecs(m.specs) {
		toolNames := []string{}
		for _, name := range m.sortedToolsNoLock() {
			if m.tools[name].ServerID == spec.ID {
				toolNames = append(toolNames, name)
			}
		}
		diagnostic, rawSecretPresent := RedactedDiagnostic(spec.Headers, spec.Env, spec.URL)
		transport := firstNonEmpty(spec.Transport, "stdio")
		authConfigured := hasAuthConfig(spec)
		authDiagnostic := diagnoseMCPAuth(transport, "connected", "", spec.URL, authConfigured)
		out = append(out, map[string]any{
			"id":                 spec.ID,
			"transport":          transport,
			"enabled":            true,
			"available":          m.connected,
			"connected":          m.connected,
			"toolCount":          float64(len(toolNames)),
			"toolNames":          toolNames,
			"lastRefreshedAt":    m.lastRefreshedAt,
			"catalogFingerprint": m.catalogHash,
			"diagnostic":         diagnostic,
			"rawSecretPresent":   rawSecretPresent,
			"credentialed":       authConfigured,
			"authConfigured":     authConfigured,
			"authStatus":         authDiagnostic.Status,
			"authUrl":            authDiagnostic.URL,
			"cwd":                filepath.ToSlash(spec.CWD),
			"lowPriority":        spec.LowPriority,
			"backgroundStart":    spec.BackgroundStart,
		})
	}
	return out
}

func (m *ContractReplayManager) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func (m *ContractReplayManager) Tools() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.connected {
		m.connectNoLock()
	}
	return m.sortedToolsNoLock()
}

func (m *ContractReplayManager) Prompts() []any {
	return nil
}

func (m *ContractReplayManager) Resources() []any {
	return nil
}

func (m *ContractReplayManager) diagnosticsNoLock() map[string]any {
	toolNames := m.sortedToolsNoLock()
	configured := len(normalizeSpecs(m.specs))
	credentialed := false
	for _, spec := range normalizeSpecs(m.specs) {
		if hasAuthConfig(spec) {
			credentialed = true
			break
		}
	}
	return map[string]any{
		"enabled":                            true,
		"mode":                               "auto",
		"active":                             m.connected,
		"available":                          m.connected,
		"reason":                             "Go runtime MCP contract lifecycle manager is connected inside the existing Analytix tool contract",
		"configuredServerCount":              float64(configured),
		"connectedServerCount":               float64(configured),
		"indexedToolCount":                   float64(len(toolNames)),
		"advertisedToolCount":                float64(len(toolNames) + 2),
		"topKDefault":                        float64(5),
		"topKMax":                            float64(20),
		"minScore":                           float64(0),
		"lastRefreshedAt":                    m.lastRefreshedAt,
		"catalogFingerprint":                 m.catalogHash,
		"catalogDrift":                       m.catalogDrift,
		"refreshCount":                       float64(m.refreshes),
		"restartReconnectCount":              float64(m.restarts),
		"contractReplayMCPTransportUsed":     true,
		"productionFallbackMCPTransportUsed": false,
		"credentialedAvailable":              credentialed,
		"topLevelMCPIndexerRouteExposed":     false,
	}
}

func (m *ContractReplayManager) sortedToolsNoLock() []string {
	names := make([]string, 0, len(m.tools))
	for name := range m.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func RunManagerContractExercise() map[string]any {
	manager := NewManager(nil)
	manager.Connect()
	initialTools := manager.Search("issue")
	searchToolName := CanonicalToolName("analytix local", "search issues")
	denied := manager.CallTool(searchToolName, false)
	unknownApproved := manager.CallTool("search_issues", true)
	approved := manager.CallTool(searchToolName, true)
	manager.Disconnect()
	reconnected := manager.RestartReconnect()
	return map[string]any{
		"runtimeGoContractParitySlice":       true,
		"contractReplayMCPTransportUsed":     true,
		"productionFallbackMCPTransportUsed": false,
		"connectedInitially":                 len(initialTools) == 2,
		"initialTools":                       initialTools,
		"deniedMCPToolExecuted":              denied["executed"],
		"unknownMCPToolExecuted":             unknownApproved["executed"],
		"unknownMCPToolCode":                 unknownApproved["code"],
		"approvedMCPToolExecuted":            approved["executed"],
		"callCount":                          manager.calls,
		"reconnected":                        reconnected["available"],
		"credentialRead":                     false,
		"topLevelMCPIndexerRouteExposed":     false,
		"reasonixPublicProtocolAllowed":      false,
	}
}

func normalizeSpecs(specs []ServerSpec) []ServerSpec {
	if len(specs) == 0 {
		return []ServerSpec{defaultFixtureSpec()}
	}
	out := make([]ServerSpec, 0, len(specs))
	for _, spec := range specs {
		if strings.TrimSpace(spec.ID) == "" {
			spec.ID = "analytix local"
		}
		if strings.TrimSpace(spec.Transport) == "" {
			spec.Transport = "stdio"
		}
		spec.Env = cloneStringMap(spec.Env)
		spec.Headers = cloneStringMap(spec.Headers)
		spec.ReadOnlyToolNames = cloneBoolMap(spec.ReadOnlyToolNames)
		out = append(out, spec)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func defaultFixtureSpec() ServerSpec {
	return ServerSpec{
		ID:          "analytix local",
		Transport:   "contract-jsonrpc",
		CWD:         "",
		LowPriority: false,
		Tools: []ToolSpec{
			{
				Name:         "search issues",
				Description:  "Search project issues from the MCP contract server.",
				InputSchema:  json.RawMessage(`{"required":["query"],"properties":{"query":{"type":"string"},"limit":{"type":"integer"}},"type":"object"}`),
				ReadOnlyHint: true,
				ResultText:   "issue-42: runtime MCP lifecycle absorbed",
			},
			{
				Name:         "create issue",
				Description:  "Create a project issue through the MCP contract server.",
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"title":{"type":"string"},"body":{"type":"string"}},"required":["title"]}`),
				ReadOnlyHint: false,
				ResultText:   "issue-created",
			},
		},
	}
}

func toolsForSpec(spec ServerSpec) []ToolSpec {
	if len(spec.Tools) > 0 {
		out := append([]ToolSpec(nil), spec.Tools...)
		sort.SliceStable(out, func(i, j int) bool {
			return CanonicalToolName(spec.ID, out[i].Name) < CanonicalToolName(spec.ID, out[j].Name)
		})
		return out
	}
	return defaultFixtureSpec().Tools
}
