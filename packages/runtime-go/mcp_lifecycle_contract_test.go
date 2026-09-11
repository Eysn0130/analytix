//go:build !analytix_prod

package runtimego

import (
	"strings"
	"testing"

	mcpwire "analytix.local/runtime-go/internal/adapters/outbound/mcp/protocol"
	domainmcpprotocol "analytix.local/runtime-go/internal/domain/mcpprotocol"
	mcp "analytix.local/runtime-go/internal/mcp"
)

func TestMCPProtocolVersionSingleSourceOfTruth(t *testing.T) {
	if mcp.MCPProtocolVersion != domainmcpprotocol.PreferredVersion || mcpwire.ProtocolVersion != domainmcpprotocol.PreferredVersion {
		t.Fatalf("MCP protocol version drifted across lifecycle and transports: lifecycle=%q wire=%q domain=%q", mcp.MCPProtocolVersion, mcpwire.ProtocolVersion, domainmcpprotocol.PreferredVersion)
	}
}

func TestMCPLifecycleAuditCoversReasonixStrengthsWithoutPublicProtocol(t *testing.T) {
	audit := mcp.RunMCPLifecycleAudit()
	if audit.SchemaVersion != 1 || audit.ChangeID != "mcp-lifecycle-contract" || audit.RuntimeContract != "analytix-go-runtime" {
		t.Fatalf("audit identity mismatch: %#v", audit)
	}
	if !audit.Credentialed ||
		!audit.Initialized ||
		!audit.NotificationSent ||
		!audit.Connected ||
		!audit.Reconnect ||
		audit.ProtocolVersion != mcp.MCPProtocolVersion {
		t.Fatalf("MCP live-local lifecycle did not complete: %#v", audit)
	}
	if audit.ToolCount != 2 ||
		!audit.NamespacedToolNames ||
		len(audit.ToolNames) != 2 ||
		audit.NamespacePrefix != "mcp__analytix_local_56366d__" ||
		!strings.Contains(strings.Join(audit.ToolNames, ","), "mcp__analytix_local_56366d__search_issues_244952") {
		t.Fatalf("namespaced tool catalog mismatch: %#v", audit)
	}
	if len(audit.SearchMatches) != 1 || audit.SearchMatches[0] != "mcp__analytix_local_56366d__search_issues_244952" {
		t.Fatalf("MCP search results mismatch: %#v", audit)
	}
	if !audit.SchemaOrderStable || audit.SchemaHash == "" || !audit.ReadOnlyHintMapped {
		t.Fatalf("schema/readOnly evidence mismatch: %#v", audit)
	}
	if !audit.CallRequiresApproval || audit.DeniedCallExecuted || !audit.ApprovedCallExecuted || audit.ApprovedCallOutput == "" {
		t.Fatalf("approval-gated call evidence mismatch: %#v", audit)
	}
	if !audit.CredentialRedaction ||
		audit.RawSecretPresent ||
		strings.Contains(audit.Diagnostic, "contract-secret-token") ||
		!strings.Contains(audit.Diagnostic, "Authorization=<redacted>") {
		t.Fatalf("credential redaction mismatch: %#v", audit)
	}
	if audit.TopLevelMCPIndexerExposed ||
		audit.ReasonixPublicProtocolUsed ||
		audit.UsesReasonixConfigRoot ||
		audit.ChangesRendererContract ||
		audit.ChangesProductIdentity {
		t.Fatalf("MCP audit must stay inside analytix runtime contract: %#v", audit)
	}
}

func TestMCPNormalizationMatchesReasonixNamespacing(t *testing.T) {
	cases := map[string]string{
		"search issues": "search_issues_244952",
		"plain_name":    "plain_name",
		"   ":           "unnamed_873846",
	}
	for raw, expected := range cases {
		if got := mcp.NormalizeName(raw); got != expected {
			t.Fatalf("normalize %q = %q, want %q", raw, got, expected)
		}
	}
	if got := mcp.CanonicalToolName("mock server", "search/code"); !strings.HasPrefix(got, "mcp__mock_server_") || !strings.Contains(got, "__search_code_") {
		t.Fatalf("tool name should be namespaced and hash-normalized: %s", got)
	}
}

func TestMCPRedactionRemovesAuthMaterial(t *testing.T) {
	diagnostic, rawSecretPresent := mcp.RedactedDiagnostic(
		map[string]string{"Authorization": "Bearer secret-token", "X-Trace": "trace"},
		map[string]string{"MCP_API_KEY": "secret-token", "DEBUG": "1"},
		"https://mcp.example.test/mcp?access_token=secret-token&workspace=analytix",
	)
	if rawSecretPresent || strings.Contains(diagnostic, "secret-token") {
		t.Fatalf("diagnostic leaked secret: %s", diagnostic)
	}
	for _, token := range []string{"Authorization=<redacted>", "MCP_API_KEY=<redacted>", "DEBUG=1", "access_token=%3Credacted%3E"} {
		if !strings.Contains(diagnostic, token) {
			t.Fatalf("diagnostic missing %s: %s", token, diagnostic)
		}
	}
}

func TestMCPContractManagerExecutesOnlyAfterApprovalAndRefreshesCatalog(t *testing.T) {
	manager := &liveTestMCPManager{}
	manager.Connect()
	expectedSearchTool := mcp.CanonicalToolName("analytix local", "search issues")
	if matches := manager.Search("search"); len(matches) != 1 || matches[0] != expectedSearchTool {
		t.Fatalf("fake MCP search mismatch: %#v", matches)
	}
	denied := manager.CallTool(expectedSearchTool, false)
	if denied["executed"] != false || denied["code"] != "approval_denied" || manager.CallCount() != 0 {
		t.Fatalf("denied MCP call must not execute: denied=%#v calls=%d", denied, manager.CallCount())
	}
	unknownApproved := manager.CallTool("search_issues", true)
	if unknownApproved["executed"] != false || unknownApproved["code"] != "mcp_tool_not_found" || manager.CallCount() != 0 {
		t.Fatalf("unknown MCP tool must not execute even when approved: unknown=%#v calls=%d", unknownApproved, manager.CallCount())
	}
	approved := manager.CallTool(expectedSearchTool, true)
	if approved["executed"] != true || manager.CallCount() != 1 {
		t.Fatalf("approved MCP call should execute once: approved=%#v calls=%d", approved, manager.CallCount())
	}
	before := manager.Diagnostics()
	manager.Disconnect()
	if matches := manager.Search("search"); len(matches) != 0 {
		t.Fatalf("disconnected MCP manager should not search: %#v", matches)
	}
	after := manager.RefreshCatalog()
	if after["available"] != true ||
		after["active"] != true ||
		after["indexedToolCount"] != float64(2) ||
		after["advertisedToolCount"] != float64(4) ||
		after["catalogFingerprint"] == "" ||
		after["catalogFingerprint"] != before["catalogFingerprint"] ||
		after["contractReplayMCPTransportUsed"] != true ||
		after["productionFallbackMCPTransportUsed"] != false ||
		after["topLevelMCPIndexerRouteExposed"] != false {
		t.Fatalf("refresh diagnostics mismatch: before=%#v after=%#v", before, after)
	}
	if servers := manager.ServerDiagnostics(); len(servers) != 1 {
		t.Fatalf("expected one live-local MCP server diagnostic: %#v", servers)
	}
	reconnected := manager.RestartReconnect()
	if reconnected["available"] != true || reconnected["restartReconnectCount"] != float64(1) {
		t.Fatalf("restart/reconnect diagnostics mismatch: %#v", reconnected)
	}
}

func TestMCPManagerRedactsCredentialedFixtureDiagnostics(t *testing.T) {
	manager := mcpManagerWithCredentialedFixture()
	manager.Connect()
	diagnostics := manager.Diagnostics()
	if diagnostics["credentialedAvailable"] != true {
		t.Fatalf("credentialed fixture should be detected: %#v", diagnostics)
	}
	servers := manager.ServerDiagnostics()
	if len(servers) != 1 {
		t.Fatalf("expected one server: %#v", servers)
	}
	server, _ := servers[0].(map[string]any)
	diagnostic, _ := server["diagnostic"].(string)
	if server["rawSecretPresent"] != false ||
		strings.Contains(diagnostic, "secret-token") ||
		!strings.Contains(diagnostic, "Authorization=<redacted>") {
		t.Fatalf("credentialed diagnostic leaked secret: %#v", server)
	}
	toolName := mcp.CanonicalToolName("credentialed fixture", "status")
	denied := manager.CallTool(toolName, false)
	unknown := manager.CallTool("mcp__credentialed_fixture__missing", true)
	approved := manager.CallTool(toolName, true)
	if denied["executed"] != false ||
		unknown["executed"] != false ||
		approved["executed"] != true ||
		manager.CallCount() != 1 {
		t.Fatalf("credentialed fixture execution boundary mismatch: denied=%#v unknown=%#v approved=%#v calls=%d", denied, unknown, approved, manager.CallCount())
	}
}

func TestMCPLifecycleAuditEventShape(t *testing.T) {
	audit := mcp.RunMCPLifecycleAudit()
	event := mcp.MCPLifecycleAuditEvent(audit, "thr_mcp", "turn_mcp")
	for _, key := range []string{
		"kind",
		"threadId",
		"turnId",
		"toolNames",
		"searchMatches",
		"credentialRedaction",
		"reasonixPublicProtocolUsed",
	} {
		if _, ok := event[key]; !ok {
			t.Fatalf("event missing %s: %#v", key, event)
		}
	}
	if event["kind"] != "mcp_lifecycle_audit" ||
		event["threadId"] != "thr_mcp" ||
		event["turnId"] != "turn_mcp" ||
		event["credentialRedaction"] != true ||
		event["reasonixPublicProtocolUsed"] != false {
		t.Fatalf("event shape mismatch: %#v", event)
	}
}

func mcpManagerWithCredentialedFixture() *liveTestMCPManager {
	return mcp.NewManager([]mcp.ServerSpec{{
		ID:        "credentialed fixture",
		Transport: "http",
		URL:       "https://mcp.example.test/mcp?access_token=secret-token",
		Headers:   map[string]string{"Authorization": "Bearer secret-token"},
		Env:       map[string]string{"MCP_API_KEY": "secret-token"},
		Tools: []mcp.ToolSpec{{
			Name:         "status",
			Description:  "Read fixture status",
			InputSchema:  []byte(`{"type":"object","properties":{"q":{"type":"string"}}}`),
			ReadOnlyHint: true,
			ResultText:   "ok",
		}},
	}})
}
