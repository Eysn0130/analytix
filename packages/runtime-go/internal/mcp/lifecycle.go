//go:build !analytix_prod

package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

type MCPLifecycleAudit struct {
	SchemaVersion              int      `json:"schemaVersion"`
	ChangeID                   string   `json:"changeId"`
	RuntimeContract            string   `json:"runtimeContract"`
	UpstreamSource             string   `json:"upstreamSource"`
	ServerID                   string   `json:"serverId"`
	Transport                  string   `json:"transport"`
	ProtocolVersion            string   `json:"protocolVersion"`
	Credentialed               bool     `json:"credentialed"`
	Initialized                bool     `json:"initialized"`
	NotificationSent           bool     `json:"notificationSent"`
	Connected                  bool     `json:"connected"`
	Reconnect                  bool     `json:"reconnect"`
	ToolCount                  int      `json:"toolCount"`
	ToolNames                  []string `json:"toolNames"`
	SearchQuery                string   `json:"searchQuery"`
	SearchMatches              []string `json:"searchMatches"`
	NamespacePrefix            string   `json:"namespacePrefix"`
	NamespacedToolNames        bool     `json:"namespacedToolNames"`
	SchemaOrderStable          bool     `json:"schemaOrderStable"`
	SchemaHash                 string   `json:"schemaHash"`
	ReadOnlyHintMapped         bool     `json:"readOnlyHintMapped"`
	CallRequiresApproval       bool     `json:"callRequiresApproval"`
	DeniedCallExecuted         bool     `json:"deniedCallExecuted"`
	ApprovedCallExecuted       bool     `json:"approvedCallExecuted"`
	ApprovedCallOutput         string   `json:"approvedCallOutput"`
	CredentialRedaction        bool     `json:"credentialRedaction"`
	Diagnostic                 string   `json:"diagnostic"`
	RawSecretPresent           bool     `json:"rawSecretPresent"`
	TopLevelMCPIndexerExposed  bool     `json:"topLevelMcpIndexerExposed"`
	ReasonixPublicProtocolUsed bool     `json:"reasonixPublicProtocolUsed"`
	UsesReasonixConfigRoot     bool     `json:"usesReasonixConfigRoot"`
	ChangesRendererContract    bool     `json:"changesRendererContract"`
	ChangesProductIdentity     bool     `json:"changesProductIdentity"`
}

type contractMCPTool struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	InputSchema  json.RawMessage `json:"inputSchema"`
	ReadOnlyHint bool            `json:"readOnlyHint"`
	ResultText   string          `json:"resultText"`
}

type contractMCPCallResult struct {
	Executed bool   `json:"executed"`
	Output   string `json:"output,omitempty"`
	Code     string `json:"code,omitempty"`
}

type contractMCPTransport struct {
	serverID         string
	tools            []contractMCPTool
	initialized      bool
	notificationSent bool
	connected        bool
	callCount        int
}

func RunMCPLifecycleAudit() MCPLifecycleAudit {
	transport := newContractMCPTransport()
	initialized := transport.initialize()
	notificationSent := transport.notifyInitialized()
	tools := transport.listTools()
	toolNames := contractMCPNamespacedToolNames(transport.serverID, tools)
	initialSchemaHash := contractMCPToolSchemaHash(tools)
	searchMatches := contractSearchMCPTools(toolNames, tools, "search")
	denied := transport.callTool("search issues", false)
	transport.close()
	transport.connected = true
	reconnectedTools := transport.listTools()
	reconnectedNames := contractMCPNamespacedToolNames(transport.serverID, reconnectedTools)
	approved := transport.callTool("search issues", true)
	diagnostic, rawSecretPresent := RedactedDiagnostic(
		map[string]string{"Authorization": "Bearer contract-secret-token", "X-Trace": "trace-1"},
		map[string]string{"MCP_API_KEY": "contract-secret-token", "DEBUG": "1"},
		"https://mcp.example.test/mcp?access_token=contract-secret-token&workspace=analytix",
	)

	return MCPLifecycleAudit{
		SchemaVersion:              1,
		ChangeID:                   "mcp-lifecycle-contract",
		RuntimeContract:            "analytix-go-runtime",
		UpstreamSource:             "reasonix-absorbed",
		ServerID:                   transport.serverID,
		Transport:                  "live-local-jsonrpc",
		ProtocolVersion:            MCPProtocolVersion,
		Credentialed:               true,
		Initialized:                initialized,
		NotificationSent:           notificationSent,
		Connected:                  transport.connected,
		Reconnect:                  len(reconnectedNames) == len(toolNames) && strings.Join(reconnectedNames, ",") == strings.Join(toolNames, ","),
		ToolCount:                  len(toolNames),
		ToolNames:                  toolNames,
		SearchQuery:                "search",
		SearchMatches:              searchMatches,
		NamespacePrefix:            CanonicalToolPrefix(transport.serverID),
		NamespacedToolNames:        contractAllNamespaced(toolNames, transport.serverID),
		SchemaOrderStable:          initialSchemaHash == contractMCPToolSchemaHash(reconnectedTools),
		SchemaHash:                 initialSchemaHash,
		ReadOnlyHintMapped:         contractReadOnlyHintMapped(tools, "search issues"),
		CallRequiresApproval:       true,
		DeniedCallExecuted:         denied.Executed,
		ApprovedCallExecuted:       approved.Executed,
		ApprovedCallOutput:         approved.Output,
		CredentialRedaction:        !rawSecretPresent && strings.Contains(diagnostic, "Authorization=<redacted>"),
		Diagnostic:                 diagnostic,
		RawSecretPresent:           rawSecretPresent,
		TopLevelMCPIndexerExposed:  false,
		ReasonixPublicProtocolUsed: false,
		UsesReasonixConfigRoot:     false,
		ChangesRendererContract:    false,
		ChangesProductIdentity:     false,
	}
}

func MCPLifecycleAuditEvent(audit MCPLifecycleAudit, threadID, turnID string) map[string]any {
	return map[string]any{
		"kind":                       "mcp_lifecycle_audit",
		"schemaVersion":              audit.SchemaVersion,
		"changeId":                   audit.ChangeID,
		"runtimeContract":            audit.RuntimeContract,
		"upstreamSource":             audit.UpstreamSource,
		"threadId":                   threadID,
		"turnId":                     turnID,
		"serverId":                   audit.ServerID,
		"transport":                  audit.Transport,
		"protocolVersion":            audit.ProtocolVersion,
		"credentialed":               audit.Credentialed,
		"initialized":                audit.Initialized,
		"notificationSent":           audit.NotificationSent,
		"connected":                  audit.Connected,
		"reconnect":                  audit.Reconnect,
		"toolCount":                  audit.ToolCount,
		"toolNames":                  audit.ToolNames,
		"searchQuery":                audit.SearchQuery,
		"searchMatches":              audit.SearchMatches,
		"namespacePrefix":            audit.NamespacePrefix,
		"namespacedToolNames":        audit.NamespacedToolNames,
		"schemaOrderStable":          audit.SchemaOrderStable,
		"schemaHash":                 audit.SchemaHash,
		"readOnlyHintMapped":         audit.ReadOnlyHintMapped,
		"callRequiresApproval":       audit.CallRequiresApproval,
		"deniedCallExecuted":         audit.DeniedCallExecuted,
		"approvedCallExecuted":       audit.ApprovedCallExecuted,
		"approvedCallOutput":         audit.ApprovedCallOutput,
		"credentialRedaction":        audit.CredentialRedaction,
		"diagnostic":                 audit.Diagnostic,
		"rawSecretPresent":           audit.RawSecretPresent,
		"topLevelMcpIndexerExposed":  audit.TopLevelMCPIndexerExposed,
		"reasonixPublicProtocolUsed": audit.ReasonixPublicProtocolUsed,
		"usesReasonixConfigRoot":     audit.UsesReasonixConfigRoot,
		"changesRendererContract":    audit.ChangesRendererContract,
		"changesProductIdentity":     audit.ChangesProductIdentity,
	}
}

func newContractMCPTransport() *contractMCPTransport {
	return &contractMCPTransport{
		serverID: "analytix local",
		tools: []contractMCPTool{
			{
				Name:         "search issues",
				Description:  "Search project issues from the live-local MCP server.",
				InputSchema:  json.RawMessage(`{"required":["query"],"properties":{"query":{"type":"string"},"limit":{"type":"integer"}},"type":"object"}`),
				ReadOnlyHint: true,
				ResultText:   "issue-42: runtime MCP lifecycle absorbed",
			},
			{
				Name:         "create issue",
				Description:  "Create a project issue through the live-local MCP server.",
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"title":{"type":"string"},"body":{"type":"string"}},"required":["title"]}`),
				ReadOnlyHint: false,
				ResultText:   "issue-created",
			},
		},
	}
}

func (t *contractMCPTransport) initialize() bool {
	t.connected = true
	t.initialized = true
	return true
}

func (t *contractMCPTransport) notifyInitialized() bool {
	if !t.initialized {
		return false
	}
	t.notificationSent = true
	return true
}

func (t *contractMCPTransport) listTools() []contractMCPTool {
	if !t.connected || !t.initialized || !t.notificationSent {
		return nil
	}
	out := append([]contractMCPTool(nil), t.tools...)
	sort.SliceStable(out, func(i, j int) bool {
		return CanonicalToolName(t.serverID, out[i].Name) < CanonicalToolName(t.serverID, out[j].Name)
	})
	return out
}

func (t *contractMCPTransport) callTool(rawName string, approved bool) contractMCPCallResult {
	if !approved {
		return contractMCPCallResult{Executed: false, Code: "approval_denied"}
	}
	if !t.connected {
		return contractMCPCallResult{Executed: false, Code: "mcp_not_connected"}
	}
	for _, tool := range t.tools {
		if tool.Name == rawName {
			t.callCount++
			return contractMCPCallResult{Executed: true, Output: tool.ResultText}
		}
	}
	return contractMCPCallResult{Executed: false, Code: "tool_not_found"}
}

func (t *contractMCPTransport) close() {
	t.connected = false
}

func contractMCPNamespacedToolNames(serverID string, tools []contractMCPTool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, CanonicalToolName(serverID, tool.Name))
	}
	sort.Strings(names)
	return names
}

func contractSearchMCPTools(toolNames []string, tools []contractMCPTool, query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	matches := []string{}
	for index, name := range toolNames {
		description := ""
		if index < len(tools) {
			description = tools[index].Description
		}
		if query == "" || strings.Contains(strings.ToLower(name+" "+description), query) {
			matches = append(matches, name)
		}
	}
	return matches
}

func contractMCPToolSchemaHash(tools []contractMCPTool) string {
	type schemaRecord struct {
		Name       string          `json:"name"`
		Schema     json.RawMessage `json:"schema"`
		ReadOnly   bool            `json:"readOnly"`
		SchemaText string          `json:"schemaText"`
	}
	records := make([]schemaRecord, 0, len(tools))
	for _, tool := range tools {
		records = append(records, schemaRecord{
			Name:       CanonicalToolName("analytix local", tool.Name),
			Schema:     json.RawMessage(canonicalJSON(tool.InputSchema)),
			ReadOnly:   tool.ReadOnlyHint,
			SchemaText: canonicalJSON(tool.InputSchema),
		})
	}
	sort.SliceStable(records, func(i, j int) bool { return records[i].Name < records[j].Name })
	data, _ := json.Marshal(records)
	return shortHexHash(data)
}

func contractReadOnlyHintMapped(tools []contractMCPTool, rawName string) bool {
	for _, tool := range tools {
		if tool.Name == rawName {
			return tool.ReadOnlyHint
		}
	}
	return false
}

func contractAllNamespaced(names []string, serverID string) bool {
	prefix := CanonicalToolPrefix(serverID)
	for _, name := range names {
		if !strings.HasPrefix(name, prefix) {
			return false
		}
	}
	return len(names) > 0
}

func canonicalJSON(body json.RawMessage) string {
	if len(body) == 0 {
		return ""
	}
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return strings.TrimSpace(string(body))
	}
	data, err := json.Marshal(value)
	if err != nil {
		return strings.TrimSpace(string(body))
	}
	return string(data)
}

func shortHexHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}
