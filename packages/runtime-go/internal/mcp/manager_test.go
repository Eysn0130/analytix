package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	httpmcp "analytix.local/runtime-go/internal/adapters/outbound/mcp/http"
	mcpprotocol "analytix.local/runtime-go/internal/adapters/outbound/mcp/protocol"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/testsupport/toolidentity"
)

func init() {
	if err := UseLoopbackHTTPTransportForTests(); err != nil {
		panic(err)
	}
}

func TestProductionManagerEmptyConfigDoesNotLoadFixtureSpec(t *testing.T) {
	manager := NewProductionManager(nil)
	manager.Connect()
	defer manager.Disconnect()

	if tools := manager.Tools(); len(tools) != 0 {
		t.Fatalf("empty production MCP config must not seed fixture tools: %#v", tools)
	}
	if servers := manager.ServerDiagnostics(); len(servers) != 0 {
		t.Fatalf("empty production MCP config must not report fixture servers: %#v", servers)
	}
	diagnostics := manager.Diagnostics()
	if mcpBoolField(diagnostics, "enabled") || mcpBoolField(diagnostics, "available") {
		t.Fatalf("empty production MCP diagnostics should be unavailable without fixture transport: %#v", diagnostics)
	}
	if _, ok := diagnostics["contractReplayMCPTransportUsed"]; ok {
		t.Fatalf("production MCP diagnostics must not expose contract replay transport fields: %#v", diagnostics)
	}
	if _, ok := diagnostics["productionFallbackMCPTransportUsed"]; ok {
		t.Fatalf("production MCP diagnostics must not expose fallback transport fields: %#v", diagnostics)
	}
}

func TestVerifiedMCPIdentityChangesAcrossManagerInstancesAtSameNumericEpoch(t *testing.T) {
	first := NewProductionManager(nil)
	second := NewProductionManager(nil)
	observed := domainmcp.ServerIdentity{Name: "docs-server", Version: "1.0.0", ProtocolVersion: mcpprotocol.ProtocolVersion}
	firstIdentity := verifiedMCPServerIdentity("docs", observed, first.connectionInstanceID, 1)
	secondIdentity := verifiedMCPServerIdentity("docs", observed, second.connectionInstanceID, 1)
	if firstIdentity == "" || secondIdentity == "" || firstIdentity == secondIdentity {
		t.Fatalf("runtime instances reused MCP identity: first=%q second=%q", firstIdentity, secondIdentity)
	}
	parsed, err := domainsecurity.ParseVerifiedMCPServerIdentity(firstIdentity)
	if err != nil || parsed.ServerID != "docs" || parsed.ObservedName != observed.Name || parsed.ObservedVersion != observed.Version ||
		parsed.NegotiatedProtocolVersion != observed.ProtocolVersion || parsed.ConnectionEpoch != 1 {
		t.Fatalf("manager emitted invalid verified identity: parsed=%#v err=%v", parsed, err)
	}
}

func TestVerifiedMCPIdentityBindsNegotiatedCompatibleRevision(t *testing.T) {
	manager := NewProductionManager(nil)
	observed := domainmcp.ServerIdentity{Name: "docs-server", Version: "1.0.0", ProtocolVersion: "2025-06-18"}
	identity := verifiedMCPServerIdentity("docs", observed, manager.connectionInstanceID, 1)
	parsed, err := domainsecurity.ParseVerifiedMCPServerIdentity(identity)
	if err != nil || parsed.NegotiatedProtocolVersion != observed.ProtocolVersion || !domainsecurity.VerifiedMCPServerIdentityCanAuthorizeFacts(parsed) {
		t.Fatalf("compatible negotiated revision was not bound into host identity: parsed=%#v err=%v", parsed, err)
	}
}

func TestMCPToolAdvertisementSnapshotFreezesSchemaPolicyEpochAndIdentityAtomically(t *testing.T) {
	const serverID = "docs"
	const rawName = "lookup"
	toolName := CanonicalToolName(serverID, rawName)
	context := newExecutableCaseSecurityContext(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-advertisement", TurnID: "turn-advertisement", WorkspaceRealPath: "/workspace/case",
		CaseID: "case-advertisement", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-advertisement")),
		DatasetSnapshotID:  domainsecurity.DatasetSnapshotIDPrefixV2 + domainsecurity.SHA256Hex([]byte("snapshot-advertisement")),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-advertisement")), ContextEpoch: 3,
		IssuedAt: time.Date(2026, 7, 14, 8, 0, 0, 0, time.UTC),
	})
	identity := mcpTestVerifiedIdentity(t, serverID, "docs-server", "1.0.0", 9)
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	client := &failingCallClient{}
	manager := NewProductionManager(nil)
	manager.specs = []ServerSpec{{ID: serverID, ReadOnlyToolNames: map[string]bool{rawName: true}}}
	manager.clients[serverID] = client
	manager.connectionEpochs[serverID] = 9
	manager.serverIdentities[serverID] = identity
	manager.tools[toolName] = managedTool{
		ServerID: serverID, Name: toolName, RawName: rawName,
		Tool: ToolSpec{Name: rawName, InputSchema: closed, OutputSchema: closed, ReadOnlyHint: false},
	}

	snapshot := manager.MCPToolAdvertisementSnapshotV1(context)
	if len(snapshot) != 1 || snapshot[0].Name != toolName || !snapshot[0].ReadOnly ||
		snapshot[0].ConnectionEpoch != 9 || snapshot[0].ServerIdentity != identity {
		t.Fatalf("atomic advertisement authority mismatch: %#v", snapshot)
	}
	manager.mu.Lock()
	changed := manager.tools[toolName]
	changed.Tool.InputSchema = json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"}},"additionalProperties":false}`)
	manager.tools[toolName] = changed
	manager.connectionEpochs[serverID] = 10
	manager.serverIdentities[serverID] = mcpTestVerifiedIdentity(t, serverID, "docs-server", "1.0.0", 10)
	manager.mu.Unlock()
	if string(snapshot[0].InputSchema) != string(closed) || snapshot[0].ConnectionEpoch != 9 || snapshot[0].ServerIdentity != identity {
		t.Fatalf("frozen advertisement changed with the live manager: %#v", snapshot[0])
	}
}

func TestLoadMCPJSONDocumentReadsGUIAndRuntimeConfigShapes(t *testing.T) {
	disabled := false
	specs, err := LoadMCPJSONDocument([]byte(mcpMustJSONText(t, map[string]any{
		"servers": map[string]any{
			"docs-mcp": map[string]any{
				"trustScope": "user",
				"url":        "https://mcp.example.test/mcp",
				"headers": map[string]any{
					"Authorization": "Bearer secret-token",
				},
			},
			"stata-mcp": map[string]any{
				"trustScope": "user",
				"command":    "uvx",
				"args":       []string{"stata-mcp"},
				"env": map[string]any{
					"STATA_CLI": "/Applications/Stata/StataMP.app",
				},
				"enabled": true,
			},
			"disabled-mcp": map[string]any{
				"command":    "disabled",
				"disabled":   true,
				"trustScope": "user",
			},
			"explicit-disabled-mcp": map[string]any{
				"command":    "explicit-disabled",
				"enabled":    disabled,
				"trustScope": "user",
			},
		},
	})), "/workspace")
	if err != nil {
		t.Fatalf("load GUI MCP config: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("expected only enabled GUI MCP specs, got %#v", specs)
	}
	if specs[0].ID != "docs-mcp" || specs[0].Transport != "streamable-http" || specs[0].URL != "https://mcp.example.test/mcp" {
		t.Fatalf("HTTP GUI MCP spec mismatch: %#v", specs[0])
	}
	if specs[1].ID != "stata-mcp" || specs[1].Transport != "stdio" || specs[1].Command != "uvx" || specs[1].Env["STATA_CLI"] == "" {
		t.Fatalf("stdio GUI MCP spec mismatch: %#v", specs[1])
	}

	runtimeSpecs, err := LoadMCPJSONDocument([]byte(mcpMustJSONText(t, map[string]any{
		"capabilities": map[string]any{
			"mcp": map[string]any{
				"servers": map[string]any{
					"plugin-mcp": map[string]any{
						"transport":  "stdio",
						"command":    "node",
						"args":       []string{"server.cjs"},
						"cwd":        "/workspace/plugin",
						"trustScope": "user",
					},
				},
			},
		},
	})), "/workspace")
	if err != nil {
		t.Fatalf("load runtime MCP config: %v", err)
	}
	if len(runtimeSpecs) != 1 || runtimeSpecs[0].ID != "plugin-mcp" || runtimeSpecs[0].CWD != "/workspace/plugin" {
		t.Fatalf("runtime capabilities MCP spec mismatch: %#v", runtimeSpecs)
	}
}

func TestLoadMCPJSONDocumentToleratesUTF8BOM(t *testing.T) {
	specs, err := LoadMCPJSONDocument([]byte("\ufeff"+mcpMustJSONText(t, map[string]any{
		"mcpServers": map[string]any{
			"analytix-computer-use": map[string]any{
				"command":    "analytix-computer-use",
				"args":       []string{"mcp"},
				"trustScope": "user",
			},
		},
	})), "/workspace")
	if err != nil {
		t.Fatalf("load BOM MCP config: %v", err)
	}
	if len(specs) != 1 || specs[0].ID != "analytix-computer-use" || specs[0].Command != "analytix-computer-use" {
		t.Fatalf("BOM MCP spec mismatch: %#v", specs)
	}
}

func TestReservedFundsMCPNamespaceIsQuarantinedBeforeProcessCreation(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "malicious-node-executed")
	specs, err := LoadMCPJSONDocument([]byte(mcpMustJSONText(t, map[string]any{
		"mcpServers": map[string]any{
			"analytix_funds": map[string]any{
				"command":    "/bin/sh",
				"args":       []string{"-c", "printf compromised > " + marker},
				"cwd":        t.TempDir(),
				"env":        map[string]any{"PATH": t.TempDir()},
				"trustScope": "user",
			},
		},
	})), t.TempDir())
	if err != nil {
		t.Fatalf("load hostile reserved MCP config: %v", err)
	}
	if len(specs) != 1 || specs[0].Transport != caseFactHostQuarantineTransport ||
		specs[0].Command != "" || len(specs[0].Args) != 0 || specs[0].CWD != "" || len(specs[0].Env) != 0 {
		t.Fatalf("reserved funds MCP retained external execution authority: %#v", specs)
	}
	manager := NewProductionManager(specs)
	manager.Connect()
	defer manager.Disconnect()
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reserved funds MCP command executed: %v", err)
	}
	if len(manager.Tools()) != 0 {
		t.Fatalf("quarantined funds namespace advertised tools: %#v", manager.Tools())
	}
	diagnostic := mcpServerDiagnosticByID(t, manager.ServerDiagnostics(), "analytix_funds")
	if diagnostic["transport"] != caseFactHostQuarantineTransport || diagnostic["connected"] != false ||
		diagnostic["failure"] != caseFactHostQuarantineFailure || diagnostic["cwd"] != "" {
		t.Fatalf("reserved funds MCP quarantine diagnostic mismatch: %#v", diagnostic)
	}
	if !reservedCaseFactMCPServerID(" AnAlYtIx_FuNdS ") || !reservedCaseFactMCPServerID("analytix-fund-analysis") {
		t.Fatal("reserved funds namespace aliases are not normalized closed")
	}
}

func TestLoadMCPJSONDocumentRejectsExternalReadOnlyToolNames(t *testing.T) {
	_, err := LoadMCPJSONDocument([]byte(`{"mcpServers":{"spoofed":{"transport":"http","url":"https://example.test/mcp","readOnlyToolNames":{"write_report":true}}}}`), "/workspace")
	if err == nil || !strings.Contains(err.Error(), "reserved for verified host policy") {
		t.Fatalf("external MCP config minted host read-only authority: %v", err)
	}
}

func TestLoadMCPJSONDocumentAppliesWorkspaceTrustScope(t *testing.T) {
	workspaceRoot := filepath.Join(t.TempDir(), "workspace")
	workspaceProject := filepath.Join(workspaceRoot, "project")
	otherRoot := filepath.Join(t.TempDir(), "other")
	if err := os.MkdirAll(workspaceProject, 0o755); err != nil {
		t.Fatalf("create workspace project: %v", err)
	}
	if err := os.MkdirAll(otherRoot, 0o755); err != nil {
		t.Fatalf("create other root: %v", err)
	}

	specs, err := LoadMCPJSONDocument([]byte(mcpMustJSONText(t, map[string]any{
		"servers": map[string]any{
			"user-mcp": map[string]any{
				"command":    "node",
				"args":       []string{"user-server.cjs"},
				"trustScope": "user",
			},
			"trusted-mcp": map[string]any{
				"command":               "node",
				"args":                  []string{"trusted-server.cjs"},
				"trustedWorkspaceRoots": []string{workspaceRoot + string(os.PathSeparator)},
			},
			"blocked-mcp": map[string]any{
				"command":               "node",
				"trustScope":            "workspace",
				"trustedWorkspaceRoots": []string{otherRoot},
			},
		},
	})), workspaceProject)
	if err != nil {
		t.Fatalf("load MCP trust config: %v", err)
	}
	specByID := map[string]ServerSpec{}
	for _, spec := range specs {
		specByID[spec.ID] = spec
	}
	if len(specByID) != 3 {
		t.Fatalf("expected loader to retain all scopes for per-turn authorization, got %#v", specs)
	}
	if blocked := specByID["blocked-mcp"]; blocked.TrustScope != "workspace" || len(blocked.TrustedWorkspaceRoots) != 1 {
		t.Fatalf("workspace MCP server authority was discarded before turn binding: %#v", blocked)
	}
	if specByID["user-mcp"].TrustScope != "user" || len(specByID["user-mcp"].TrustedWorkspaceRoots) != 0 {
		t.Fatalf("user-scoped MCP default mismatch: %#v", specByID["user-mcp"])
	}
	trusted := specByID["trusted-mcp"]
	if trusted.TrustScope != "workspace" ||
		len(trusted.TrustedWorkspaceRoots) != 1 ||
		trusted.TrustedWorkspaceRoots[0] != filepath.ToSlash(workspaceRoot) {
		t.Fatalf("trusted workspace MCP normalization mismatch: %#v", trusted)
	}

	manager := NewProductionManager(specs)
	diagnostics := manager.ServerDiagnostics()
	trustedDiagnostic := mcpServerDiagnosticByID(t, diagnostics, "trusted-mcp")
	if mcpStringField(trustedDiagnostic, "trustScope") != "workspace" ||
		trustedDiagnostic["trustedWorkspaceRootCount"] != float64(1) ||
		!mcpBoolField(trustedDiagnostic, "workspaceTrustConfigured") {
		t.Fatalf("trusted workspace MCP diagnostics mismatch: %#v", trustedDiagnostic)
	}
	userDiagnostic := mcpServerDiagnosticByID(t, diagnostics, "user-mcp")
	if mcpStringField(userDiagnostic, "trustScope") != "user" ||
		userDiagnostic["trustedWorkspaceRootCount"] != float64(0) ||
		mcpBoolField(userDiagnostic, "workspaceTrustConfigured") {
		t.Fatalf("user MCP trust diagnostics mismatch: %#v", userDiagnostic)
	}
}

func TestWorkspaceScopedMCPRequiresFrozenTurnWorkspaceAtAdvertisementAndExecution(t *testing.T) {
	const serverID = "workspace-docs"
	const rawName = "lookup"
	toolName := CanonicalToolName(serverID, rawName)
	identity := mcpTestVerifiedIdentity(t, serverID, "workspace-docs", "1.0.0", 1)
	client := &failingCallClient{}
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	manager := NewProductionManager(nil)
	manager.specs = []ServerSpec{{
		ID: serverID, TrustScope: "workspace", TrustedWorkspaceRoots: []string{"/workspace/allowed"},
	}}
	manager.clients[serverID] = client
	manager.connectionEpochs[serverID] = 1
	manager.serverIdentities[serverID] = identity
	manager.tools[toolName] = managedTool{
		ServerID: serverID, Name: toolName, RawName: rawName,
		Tool: ToolSpec{Name: rawName, InputSchema: closed, OutputSchema: closed},
	}
	newContext := func(turnID string, workspace string) domainsecurity.TurnSecurityContext {
		return newExecutableCaseSecurityContext(t, domainsecurity.TurnSecurityContextInput{
			ThreadID: "thread-workspace-trust", TurnID: turnID, WorkspaceRealPath: workspace,
			CaseID: "case-workspace-trust", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-workspace-trust")),
			DatasetSnapshotID:  domainsecurity.DatasetSnapshotIDPrefixV2 + domainsecurity.SHA256Hex([]byte("snapshot-workspace-trust")),
			SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-workspace-trust")), ContextEpoch: 2,
			IssuedAt: time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC),
		})
	}
	allowed := newContext("turn-workspace-allowed", "/workspace/allowed/case")
	outside := newContext("turn-workspace-outside", "/workspace/outside")

	if live := manager.LiveTools(); len(live) != 0 {
		t.Fatalf("workspace MCP was executable without a frozen turn: %#v", live)
	}
	if live := manager.LiveToolsForSecurityContext(allowed); len(live) != 1 || live[0] != toolName {
		t.Fatalf("trusted workspace MCP was not advertised: %#v", live)
	}
	if live := manager.LiveToolsForSecurityContext(outside); len(live) != 0 {
		t.Fatalf("workspace MCP crossed the frozen trust root: %#v", live)
	}
	if advertisements := manager.MCPToolAdvertisementSnapshotV1(outside); len(advertisements) != 0 {
		t.Fatalf("outside workspace minted MCP grant material: %#v", advertisements)
	}
	if got, ok := manager.ToolServerIdentity(toolName, outside); ok || got != "" {
		t.Fatalf("outside workspace resolved server identity: %q ok=%v", got, ok)
	}
	result := manager.callToolContext(
		context.Background(), toolName, true, 1, "", identity, nil, true, &outside, nil, map[string]any{},
	)
	if result["code"] != "mcp_workspace_trust_mismatch" || result["executed"] != false || client.calls != 0 {
		t.Fatalf("outside workspace reached MCP transport: result=%#v calls=%d", result, client.calls)
	}
}

func TestLoadMCPJSONDocumentRejectsImplicitGlobalTrustScope(t *testing.T) {
	if specs, err := LoadMCPJSONDocument(
		[]byte(`{"servers":{"docs":{"command":"node"}}}`), "/workspace",
	); err == nil || len(specs) != 0 || !strings.Contains(err.Error(), "trust scope is required") {
		t.Fatalf("implicit global MCP scope passed: specs=%#v err=%v", specs, err)
	}
}

func TestLoadMCPJSONDocumentRejectsWorkspaceScopeWithoutTrustedRoot(t *testing.T) {
	_, err := LoadMCPJSONDocument([]byte(`{"servers":{"invalid-workspace-mcp":{"command":"node","trustScope":"workspace"}}}`), "/workspace")
	if err == nil {
		t.Fatal("workspace MCP server without trusted roots passed closed configuration validation")
	}
}

func TestLoadMCPJSONDocumentRejectsAmbiguousContainersAndServerFields(t *testing.T) {
	for _, body := range []string{
		`{"servers":{},"mcpServers":{}}`,
		`{"servers":{},"capabilities":{"mcp":{"servers":{}}}}`,
		`{"mcpServers":{"docs":{"command":"node","unknown":true}}}`,
		`{"mcpServers":{"docs":{"command":"node","env":{"TOKEN":null}}}}`,
		`{"mcpServers":{"docs":{"command":"node","args":[null]}}}`,
		`{"mcpServers":{"docs":{"command":"node","enabled":true,"disabled":false}}}`,
		`{"mcpServers":{"docs":{"command":"node","transport":"stdio","type":"stdio"}}}`,
		`{"mcpServers":{"Docs":{"command":"node"}}}`,
		`{"mcpServers":{"docs__shadow":{"command":"node"}}}`,
		`{"mcpServers":{"docs":{"command":"node","url":"https://example.test/mcp"}}}`,
		`{"mcpServers":{"docs":{"transport":"stdio"}}}`,
		`{"mcpServers":{"docs":{"transport":"streamable-http"}}}`,
		`{"mcpServers":{"docs":{"command":"node","timeoutMs":0}}}`,
		`{"mcpServers":{"docs":{"command":"node","timeoutMs":3600001}}}`,
	} {
		if specs, err := LoadMCPJSONDocument([]byte(body), "/workspace"); err == nil || len(specs) != 0 {
			t.Fatalf("ambiguous MCP configuration passed: body=%s specs=%#v err=%v", body, specs, err)
		}
	}
}

func TestLoadMCPJSONDocumentValidatesDisabledServersBeforeFiltering(t *testing.T) {
	for _, body := range []string{
		`{"mcpServers":{"docs":{"command":"node","disabled":true,"readOnlyToolNames":{}}}}`,
		`{"mcpServers":{"docs":{"command":"node","disabled":true,"unknown":true}}}`,
	} {
		if specs, err := LoadMCPJSONDocument([]byte(body), "/workspace"); err == nil || len(specs) != 0 {
			t.Fatalf("invalid disabled MCP server bypassed validation: specs=%#v err=%v", specs, err)
		}
	}
}

func TestLoadMCPJSONDocumentPreservesExactTimeoutAndLegacyHTTPType(t *testing.T) {
	specs, err := LoadMCPJSONDocument([]byte(`{"mcpServers":{"docs":{"type":"http","url":"https://example.test/mcp","timeoutMs":45000,"trustScope":"user"}}}`), "/workspace")
	if err != nil || len(specs) != 1 || specs[0].Transport != "streamable-http" || specs[0].TimeoutMS != 45_000 {
		t.Fatalf("canonical MCP projection mismatch: specs=%#v err=%v", specs, err)
	}
}

func TestMCPRedactedDiagnosticRedactsURLUserinfo(t *testing.T) {
	diagnostic, rawSecretPresent := RedactedDiagnostic(
		map[string]string{"Authorization": "Bearer header-secret"},
		map[string]string{"MCP_API_KEY": "env-secret"},
		"https://mcp-user:mcp-password@mcp.example.test/rpc?access_token=query-secret",
	)
	for _, secret := range []string{"header-secret", "env-secret", "mcp-user", "mcp-password", "query-secret"} {
		if strings.Contains(diagnostic, secret) {
			t.Fatalf("MCP diagnostic leaked %q: %s", secret, diagnostic)
		}
	}
	if rawSecretPresent {
		t.Fatalf("MCP diagnostic should report no raw secret after URL userinfo redaction: %s", diagnostic)
	}
	if !strings.Contains(diagnostic, "%3Credacted%3E@mcp.example.test") ||
		!strings.Contains(diagnostic, "access_token=%3Credacted%3E") {
		t.Fatalf("MCP diagnostic should preserve redacted URL shape: %s", diagnostic)
	}
}

func TestProductionManagerHTTPListCallReconnectAndRedaction(t *testing.T) {
	var mu sync.Mutex
	methodCounts := map[string]int{}
	capturedArgs := map[string]any{}
	capturedAuthorization := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		var request struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		mu.Lock()
		methodCounts[request.Method]++
		if request.Method == "tools/call" {
			capturedArgs, _ = request.Params["arguments"].(map[string]any)
			capturedAuthorization = r.Header.Get("Authorization")
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"serverInfo": map[string]any{"name": "http-test"}}))
		case "tools/list":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"tools": []map[string]any{{
				"name":        "lookup",
				"description": "Look up a record",
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}},
				"annotations": map[string]any{"readOnlyHint": true},
			}}}))
		case "tools/call":
			query := fmt.Sprint(capturedArgs["query"])
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"content": []map[string]any{{
				"type": "text",
				"text": "http result: " + query,
			}}}))
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
	defer server.Close()

	manager := NewProductionManager([]ServerSpec{{
		ID:        "http-server",
		Transport: "http",
		URL:       server.URL + "?token=contract-secret-token",
		Headers:   map[string]string{"Authorization": "Bearer contract-secret-token"},
		Env:       map[string]string{"MCP_API_KEY": "contract-secret-token"},
	}})
	manager.Connect()
	defer manager.Disconnect()

	toolName := CanonicalToolName("http-server", "lookup")
	if tools := manager.Tools(); len(tools) != 1 || tools[0] != toolName {
		t.Fatalf("unexpected HTTP MCP tools: %#v", tools)
	}
	denied := manager.CallTool(toolName, false, map[string]any{"query": "blocked"})
	if mcpBoolField(denied, "executed") || mcpStringField(denied, "code") != "approval_denied" {
		t.Fatalf("denied MCP call should not execute: %#v", denied)
	}
	called := manager.CallTool(toolName, true, map[string]any{"query": "needle"})
	if !mcpBoolField(called, "executed") || mcpResultText(called) != "http result: needle" || !mcpBoolField(called, "readOnlyHint") {
		t.Fatalf("unexpected approved MCP call result: %#v", called)
	}
	if capturedAuthorization != "Bearer contract-secret-token" {
		t.Fatalf("HTTP MCP transport did not forward configured header: %q", capturedAuthorization)
	}
	if fmt.Sprint(capturedArgs["query"]) != "needle" {
		t.Fatalf("HTTP MCP transport did not forward tool arguments: %#v", capturedArgs)
	}

	serverDiagnostics := manager.ServerDiagnostics()
	diagnosticsJSON := mcpMustJSONText(t, serverDiagnostics)
	if strings.Contains(diagnosticsJSON, "contract-secret-token") {
		t.Fatalf("MCP diagnostics leaked credential: %s", diagnosticsJSON)
	}
	firstDiagnostic, _ := serverDiagnostics[0].(map[string]any)
	if !strings.Contains(mcpStringField(firstDiagnostic, "diagnostic"), "<redacted>") {
		t.Fatalf("MCP diagnostics missed redaction: %s", diagnosticsJSON)
	}
	reconnected := manager.RestartReconnect()
	if !mcpBoolField(reconnected, "available") {
		t.Fatalf("expected MCP reconnect to be available: %#v", reconnected)
	}
	mu.Lock()
	initializeCount := methodCounts["initialize"]
	listCount := methodCounts["tools/list"]
	mu.Unlock()
	if initializeCount < 2 || listCount < 2 {
		t.Fatalf("reconnect did not relist tools: initialize=%d list=%d", initializeCount, listCount)
	}
}

func TestProductionManagerQuarantinesSchemaInvalidSiblingWithoutExecutionSurface(t *testing.T) {
	const quarantinedRawName = "bad_6217000012345678901"
	var mu sync.Mutex
	var toolCalls int
	includeInvalid := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{
				"serverInfo": map[string]any{"name": "quarantine-test"},
			}))
		case "tools/list":
			closed := map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
			tools := []map[string]any{{"name": "valid", "inputSchema": closed, "outputSchema": closed}}
			mu.Lock()
			if includeInvalid {
				tools = append(tools, map[string]any{
					"name": quarantinedRawName, "inputSchema": map[string]any{"type": "string"}, "outputSchema": closed,
				})
			}
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"tools": tools}))
		case "tools/call":
			mu.Lock()
			toolCalls++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{
				"content": []map[string]any{{"type": "text", "text": "unexpected"}},
			}))
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": request.ID,
				"error": map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
	defer server.Close()

	manager := NewProductionManager([]ServerSpec{{
		ID: "quarantine-server", Transport: "http", URL: server.URL,
	}})
	manager.Connect()
	defer manager.Disconnect()

	validName := CanonicalToolName("quarantine-server", "valid")
	badName := CanonicalToolName("quarantine-server", quarantinedRawName)
	if tools := manager.Tools(); len(tools) != 1 || tools[0] != validName {
		t.Fatalf("schema-invalid sibling entered the executable catalog: %#v", tools)
	}
	diagnostic := mcpServerDiagnosticByID(t, manager.ServerDiagnostics(), "quarantine-server")
	issues, _ := diagnostic["toolContractQuarantines"].([]any)
	if diagnostic["toolContractQuarantineCount"] != float64(1) || len(issues) != 1 {
		t.Fatalf("closed quarantine diagnostics mismatch: %#v", diagnostic)
	}
	issue, _ := issues[0].(map[string]any)
	if issue["toolName"] != "bad_[ACCOUNT]" ||
		strings.Contains(fmt.Sprint(issue["toolName"]), "6217000012345678901") ||
		issue["code"] != string(mcpprotocol.ToolContractInvalidInputSchema) {
		t.Fatalf("quarantine diagnostic exposed the wrong identity/code: %#v", issue)
	}
	validResult := manager.CallTool(validName, true, map[string]any{})
	mu.Lock()
	callsAfterValid := toolCalls
	mu.Unlock()
	if validResult["executed"] != true || callsAfterValid != 1 {
		t.Fatalf("valid sibling did not remain executable: result=%#v calls=%d", validResult, callsAfterValid)
	}
	beforeInvalid := callsAfterValid
	result := manager.CallTool(badName, true, map[string]any{})
	mu.Lock()
	callsAfterInvalid := toolCalls
	mu.Unlock()
	if result["executed"] != false || result["code"] != "mcp_tool_not_found" || callsAfterInvalid != beforeInvalid {
		t.Fatalf("quarantined tool reached transport: result=%#v calls=%d", result, callsAfterInvalid)
	}

	beforeFingerprint := mcpStringField(manager.Diagnostics(), "catalogFingerprint")
	mu.Lock()
	includeInvalid = false
	mu.Unlock()
	refresh := manager.RefreshCatalog()
	refreshedDiagnostic := mcpServerDiagnosticByID(t, manager.ServerDiagnostics(), "quarantine-server")
	if refreshedDiagnostic["toolContractQuarantineCount"] != float64(0) ||
		len(refreshedDiagnostic["toolContractQuarantines"].([]any)) != 0 ||
		mcpStringField(refresh, "catalogFingerprint") == beforeFingerprint ||
		refresh["catalogDrift"] != true {
		t.Fatalf("refresh accumulated stale quarantine state or fingerprint: refresh=%#v server=%#v", refresh, refreshedDiagnostic)
	}
}

func TestProductionManagerLiveCatalogCorruptionCannotHideBehindCachedSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{
				"serverInfo": map[string]any{"name": "corrupt-catalog"},
			}))
		case "tools/list":
			tools := []map[string]any{{
				"name": "live", "inputSchema": map[string]any{"type": "object"},
				"outputSchema": map[string]any{"type": "object"}, "unknown": true,
			}}
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"tools": tools}))
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": request.ID,
				"error": map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	spec := ServerSpec{ID: "corrupt-server", Transport: "http", URL: server.URL}
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	if err := saveCachedSchema(cacheDir, spec.ID, CachedSchema{
		SpecHash: SpecFingerprint(spec),
		Tools: []ToolSpec{{
			Name: "cached", InputSchema: closed, OutputSchema: closed,
		}},
		LastValidated: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("save cached schema: %v", err)
	}
	manager := NewProductionManagerWithCache([]ServerSpec{spec}, cacheDir)
	manager.Connect()
	defer manager.Disconnect()

	if tools := manager.Tools(); len(tools) != 0 {
		t.Fatalf("stale cache hid live catalog corruption: %#v", tools)
	}
	diagnostic := mcpServerDiagnosticByID(t, manager.ServerDiagnostics(), spec.ID)
	if mcpBoolField(diagnostic, "schemaHintAvailable") || mcpBoolField(diagnostic, "available") {
		t.Fatalf("catalog corruption retained cached or live authority: %#v", diagnostic)
	}
}

func TestProductionManagerPureTransportOutageRetainsOnlyNonExecutableCacheHint(t *testing.T) {
	var mu sync.Mutex
	methods := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(body, &request)
		mu.Lock()
		methods[request.Method]++
		mu.Unlock()
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	spec := ServerSpec{ID: "outage-server", Transport: "http", URL: server.URL}
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	if err := saveCachedSchema(cacheDir, spec.ID, CachedSchema{
		SpecHash: SpecFingerprint(spec),
		Tools: []ToolSpec{{
			Name: "cached", InputSchema: closed, OutputSchema: closed,
		}},
		LastValidated: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	manager := NewProductionManagerWithCache([]ServerSpec{spec}, cacheDir)
	manager.Connect()
	defer manager.Disconnect()

	toolName := CanonicalToolName(spec.ID, "cached")
	if tools := manager.Tools(); len(tools) != 1 || tools[0] != toolName || len(manager.LiveTools()) != 0 {
		t.Fatalf("transport outage did not retain exactly one non-executable schema hint: tools=%#v live=%#v", tools, manager.LiveTools())
	}
	diagnostic := mcpServerDiagnosticByID(t, manager.ServerDiagnostics(), spec.ID)
	if !mcpBoolField(diagnostic, "schemaHintAvailable") || mcpBoolField(diagnostic, "available") {
		t.Fatalf("outage cache hint diagnostics gained live authority: %#v", diagnostic)
	}
	result := manager.CallTool(toolName, true, map[string]any{})
	mu.Lock()
	toolCalls := methods["tools/call"]
	mu.Unlock()
	if result["executed"] != false || toolCalls != 0 {
		t.Fatalf("cache hint reached tools/call during outage: result=%#v calls=%d", result, toolCalls)
	}
}

func TestProductionManagerIdentityMismatchCannotHideBehindCachedSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		_ = json.Unmarshal(body, &request)
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{
				"serverInfo": map[string]any{"name": "spoofed", "version": "9.9.9"},
			}))
		default:
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	spec := ServerSpec{
		ID: "identity-server", Transport: "http", URL: server.URL,
		ExpectedServerName: "trusted", ExpectedServerVersion: "1.0.0",
	}
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	if err := saveCachedSchema(cacheDir, spec.ID, CachedSchema{
		SpecHash: SpecFingerprint(spec),
		Tools: []ToolSpec{{
			Name: "cached", InputSchema: closed, OutputSchema: closed,
		}},
		LastValidated: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	manager := NewProductionManagerWithCache([]ServerSpec{spec}, cacheDir)
	manager.Connect()
	defer manager.Disconnect()

	if len(manager.Tools()) != 0 || len(manager.LiveTools()) != 0 {
		t.Fatalf("identity mismatch retained stale catalog authority: tools=%#v live=%#v", manager.Tools(), manager.LiveTools())
	}
	diagnostic := mcpServerDiagnosticByID(t, manager.ServerDiagnostics(), spec.ID)
	if mcpBoolField(diagnostic, "schemaHintAvailable") || mcpBoolField(diagnostic, "available") {
		t.Fatalf("identity mismatch was downgraded to a cache hint: %#v", diagnostic)
	}
}

func TestServerCatalogFingerprintIsIsolatedFromOtherServerQuarantine(t *testing.T) {
	manager := NewProductionManager([]ServerSpec{{ID: "server-a"}, {ID: "server-b"}})
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	for _, serverID := range []string{"server-a", "server-b"} {
		rawName := "lookup"
		name := CanonicalToolName(serverID, rawName)
		manager.tools[name] = managedTool{
			ServerID: serverID, Name: name, RawName: rawName,
			Tool: ToolSpec{Name: rawName, InputSchema: closed, OutputSchema: closed},
		}
	}
	before := manager.ServerDiagnostics()
	aBefore := mcpStringField(mcpServerDiagnosticByID(t, before, "server-a"), "catalogFingerprint")
	bBefore := mcpStringField(mcpServerDiagnosticByID(t, before, "server-b"), "catalogFingerprint")
	manager.toolContractIssues["server-a"] = []mcpprotocol.ToolContractIssue{{
		Name: "bad", Code: mcpprotocol.ToolContractInvalidInputSchema,
	}}
	after := manager.ServerDiagnostics()
	aAfter := mcpStringField(mcpServerDiagnosticByID(t, after, "server-a"), "catalogFingerprint")
	bAfter := mcpStringField(mcpServerDiagnosticByID(t, after, "server-b"), "catalogFingerprint")
	if aBefore == aAfter || bBefore != bAfter {
		t.Fatalf("per-server fingerprint was cross-contaminated: a=%s/%s b=%s/%s", aBefore, aAfter, bBefore, bAfter)
	}
}

func TestProductionManagerHTTPCallToolContextCancelsInFlightRequest(t *testing.T) {
	callStarted := make(chan struct{})
	var closeStarted sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"serverInfo": map[string]any{"name": "http-cancel"}}))
		case "tools/list":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"tools": []map[string]any{{
				"name":        "slow",
				"description": "Slow cancellable tool",
				"inputSchema": map[string]any{"type": "object"},
			}}}))
		case "tools/call":
			closeStarted.Do(func() { close(callStarted) })
			<-r.Context().Done()
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
	defer server.Close()

	manager := NewProductionManager([]ServerSpec{{
		ID:        "http-cancel",
		Transport: "http",
		URL:       server.URL,
	}})
	manager.Connect()
	defer manager.Disconnect()

	toolName := CanonicalToolName("http-cancel", "slow")
	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan map[string]any, 1)
	go func() {
		resultCh <- manager.CallToolContext(ctx, toolName, true, map[string]any{"query": "wait"})
	}()

	select {
	case <-callStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP MCP tools/call did not start")
	}
	cancel()

	select {
	case result := <-resultCh:
		if mcpBoolField(result, "executed") ||
			mcpStringField(result, "code") != "mcp_call_cancelled" ||
			!strings.Contains(mcpStringField(result, "error"), "context canceled") {
			t.Fatalf("cancelled MCP call returned unexpected result: %#v", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP MCP tools/call did not unblock after context cancel")
	}
}

func TestProductionManagerHTTPFailureRedactsResponseSecrets(t *testing.T) {
	const configuredSecret = "configured-http-secret-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"serverInfo": map[string]any{"name": "http-redaction-test"}}))
		case "tools/list":
			http.Error(w, `unauthorized Authorization: Bearer echoed-secret-token token=response-secret-token access_token=list-secret-token`, http.StatusUnauthorized)
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
	defer server.Close()

	manager := NewProductionManager([]ServerSpec{{
		ID:        "http-failing-server",
		Transport: "http",
		URL:       server.URL + "?access_token=" + configuredSecret,
		Headers:   map[string]string{"Authorization": "Bearer " + configuredSecret},
	}})
	manager.Connect()
	defer manager.Disconnect()

	if tools := manager.Tools(); len(tools) != 0 {
		t.Fatalf("failing HTTP MCP server must not advertise tools: %#v", tools)
	}
	serverDiagnostics := manager.ServerDiagnostics()
	diagnosticsJSON := mcpMustJSONText(t, serverDiagnostics)
	for _, secret := range []string{configuredSecret, "echoed-secret-token", "response-secret-token", "list-secret-token"} {
		if strings.Contains(diagnosticsJSON, secret) {
			t.Fatalf("HTTP MCP diagnostics leaked credential %q: %s", secret, diagnosticsJSON)
		}
	}
	row := mcpServerDiagnosticByID(t, serverDiagnostics, "http-failing-server")
	failure := mcpStringField(row, "failure")
	if !strings.Contains(failure, "401") ||
		mcpStringField(row, "authStatus") != "required" ||
		mcpBoolField(row, "available") ||
		mcpBoolField(row, "connected") {
		t.Fatalf("HTTP MCP failure should be unavailable and redacted with auth diagnostics: %#v", row)
	}
}

func TestProductionManagerHTTPCallFailureRedactsResponseSecrets(t *testing.T) {
	const configuredSecret = "configured-call-secret-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"serverInfo": map[string]any{"name": "http-call-redaction-test"}}))
		case "tools/list":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"tools": []map[string]any{{
				"name":        "lookup",
				"description": "Look up a record",
				"inputSchema": map[string]any{"type": "object"},
			}}}))
		case "tools/call":
			http.Error(w, `upstream failed Authorization=Bearer call-echo-secret-token api_key=call-api-secret-token token=call-token-secret`, http.StatusUnauthorized)
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
	defer server.Close()

	manager := NewProductionManager([]ServerSpec{{
		ID:        "http-call-failing-server",
		Transport: "http",
		URL:       server.URL + "?token=" + configuredSecret,
		Headers:   map[string]string{"Authorization": "Bearer " + configuredSecret},
	}})
	manager.Connect()
	defer manager.Disconnect()

	toolName := CanonicalToolName("http-call-failing-server", "lookup")
	called := manager.CallTool(toolName, true, map[string]any{"query": "needle"})
	if mcpBoolField(called, "executed") ||
		mcpStringField(called, "code") != "mcp_call_failed" ||
		mcpBoolField(called, "retried") {
		t.Fatalf("HTTP MCP 401 call failure should not execute or retry: %#v", called)
	}
	callJSON := mcpMustJSONText(t, called)
	for _, secret := range []string{configuredSecret, "call-echo-secret-token", "call-api-secret-token", "call-token-secret"} {
		if strings.Contains(callJSON, secret) {
			t.Fatalf("HTTP MCP call result leaked credential %q: %s", secret, callJSON)
		}
	}
	if !strings.Contains(mcpStringField(called, "error"), "401") ||
		strings.Contains(mcpStringField(called, "error"), "<redacted>") {
		t.Fatalf("HTTP MCP call error should preserve only the status without reflecting a response body: %#v", called)
	}
}

func TestProductionManagerHTTPTransportRejectsConfiguredProxyWithoutDirectFallback(t *testing.T) {
	var proxyRequests int
	proxy := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		proxyRequests++
	}))
	defer proxy.Close()

	manager := NewProductionManagerWithOptions([]ServerSpec{{
		ID:        "proxied-http-server",
		Transport: "http",
		URL:       "http://mcp.example.test/rpc",
	}}, ProductionManagerOptions{ProxyURL: proxy.URL})
	manager.Connect()
	defer manager.Disconnect()

	if tools := manager.Tools(); len(tools) != 0 {
		t.Fatalf("proxy-backed MCP unexpectedly exposed tools: %#v", tools)
	}
	diagnostics := manager.ServerDiagnostics()
	if len(diagnostics) != 1 ||
		mcpStringField(diagnostics[0].(map[string]any), "failure") != "MCP HTTP client configuration is invalid" {
		t.Fatalf("proxy rejection was not reported as closed configuration: %#v", diagnostics)
	}
	if proxyRequests != 0 {
		t.Fatalf("rejected MCP proxy received network traffic: requests=%d", proxyRequests)
	}
}

func TestProductionManagerHTTPPromptsAndResourcesCatalog(t *testing.T) {
	const resourceSecret = "resource-secret-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{
				"serverInfo":   map[string]any{"name": "catalog-test"},
				"capabilities": map[string]any{"prompts": map[string]any{}, "resources": map[string]any{}},
			}))
		case "tools/list":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"tools": []map[string]any{{
				"name":        "lookup",
				"description": "Look up a record",
				"inputSchema": map[string]any{"type": "object"},
			}}}))
		case "prompts/list":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"prompts": []map[string]any{{
				"name":        "summarize",
				"description": "Summarize without token=prompt-secret",
				"arguments": []map[string]any{{
					"name":        "topic",
					"description": "Topic to summarize",
					"required":    true,
				}},
			}}}))
		case "resources/list":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"resources": []map[string]any{{
				"uri":         "https://mcp-user:mcp-password@example.test/docs?access_token=" + resourceSecret,
				"name":        "Docs token=resource-name-secret",
				"description": "Remote docs",
				"mimeType":    "text/markdown",
			}}}))
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
	defer server.Close()

	manager := NewProductionManager([]ServerSpec{{
		ID:        "catalog-server",
		Transport: "http",
		URL:       server.URL,
	}})
	manager.Connect()
	defer manager.Disconnect()

	if tools := manager.Tools(); len(tools) != 1 || tools[0] != CanonicalToolName("catalog-server", "lookup") {
		t.Fatalf("MCP catalog should keep tools while adding prompts/resources diagnostics: %#v", tools)
	}
	prompts, resources := waitForMCPPromptResourceCatalog(t, manager, 1, 1)
	diagnostics := manager.Diagnostics()
	if diagnostics["promptCount"] != float64(1) || diagnostics["resourceCount"] != float64(1) {
		t.Fatalf("MCP diagnostics should count prompts/resources without changing tool count: %#v", diagnostics)
	}
	serverDiagnostics := manager.ServerDiagnostics()
	if len(serverDiagnostics) != 1 ||
		serverDiagnostics[0].(map[string]any)["promptCount"] != float64(1) ||
		serverDiagnostics[0].(map[string]any)["resourceCount"] != float64(1) {
		t.Fatalf("MCP server diagnostics should count prompts/resources per server: %#v", serverDiagnostics)
	}
	prompt, _ := prompts[0].(map[string]any)
	if mcpStringField(prompt, "serverId") != "catalog-server" ||
		mcpStringField(prompt, "name") != "summarize" ||
		!strings.Contains(mcpStringField(prompt, "description"), "<redacted>") {
		t.Fatalf("MCP prompt diagnostics mismatch: %#v", prompt)
	}
	resource, _ := resources[0].(map[string]any)
	if mcpStringField(resource, "serverId") != "catalog-server" ||
		(!strings.Contains(mcpStringField(resource, "uri"), "%3Credacted%3E") &&
			!strings.Contains(mcpStringField(resource, "uri"), "[EMAIL]")) ||
		!strings.Contains(mcpStringField(resource, "name"), "<redacted>") {
		t.Fatalf("MCP resource diagnostics mismatch: %#v", resource)
	}
	raw := mcpMustJSONText(t, map[string]any{"prompts": prompts, "resources": resources})
	for _, secret := range []string{"prompt-secret", "resource-name-secret", "mcp-user", "mcp-password", resourceSecret} {
		if strings.Contains(raw, secret) {
			t.Fatalf("MCP prompts/resources diagnostics leaked %q: %s", secret, raw)
		}
	}
}

func TestProductionManagerSlowPromptResourcesDoNotBlockToolsCatalog(t *testing.T) {
	resourcesGate := make(chan struct{})
	resourcesRequested := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{
				"serverInfo":   map[string]any{"name": "slow-catalog"},
				"capabilities": map[string]any{"prompts": map[string]any{}, "resources": map[string]any{}},
			}))
		case "tools/list":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"tools": []map[string]any{{
				"name":        "lookup",
				"description": "Lookup while resources are slow",
				"inputSchema": map[string]any{"type": "object"},
			}}}))
		case "prompts/list":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"prompts": []map[string]any{{
				"name":        "summarize",
				"description": "Summarize",
			}}}))
		case "resources/list":
			close(resourcesRequested)
			<-resourcesGate
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"resources": []map[string]any{{
				"uri":  "file:///workspace/slow.md",
				"name": "Slow resource",
			}}}))
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
	defer server.Close()
	defer close(resourcesGate)

	manager := NewProductionManager([]ServerSpec{{
		ID:        "slow-catalog",
		Transport: "http",
		URL:       server.URL,
	}})
	startedAt := time.Now()
	manager.Connect()
	defer manager.Disconnect()

	toolName := CanonicalToolName("slow-catalog", "lookup")
	if tools := manager.Tools(); len(tools) != 1 || tools[0] != toolName {
		t.Fatalf("slow prompt/resource catalog must not block tools: %#v", tools)
	}
	if elapsed := time.Since(startedAt); elapsed > 250*time.Millisecond {
		t.Fatalf("Connect should return after tools/list, not wait for slow resources: %s", elapsed)
	}
	select {
	case <-resourcesRequested:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected Phase-B resources/list request after tools became available")
	}
	resourcesGate <- struct{}{}
	prompts, resources := waitForMCPPromptResourceCatalog(t, manager, 1, 1)
	if mcpStringField(prompts[0].(map[string]any), "name") != "summarize" ||
		mcpStringField(resources[0].(map[string]any), "uri") != "file:///workspace/slow.md" {
		t.Fatalf("Phase-B prompt/resource catalog mismatch: prompts=%#v resources=%#v", prompts, resources)
	}
}

func TestProductionManagerHTTPSendsInitializedNotificationBeforeListingTools(t *testing.T) {
	var mu sync.Mutex
	methodCounts := map[string]int{}
	initialized := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		mu.Lock()
		methodCounts[request.Method]++
		if request.Method == "notifications/initialized" {
			initialized = true
		}
		ready := initialized
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"serverInfo": map[string]any{"name": "strict-http-test"}}))
		case "tools/list":
			if !ready {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      request.ID,
					"error":   map[string]any{"code": -32002, "message": "not initialized"},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"tools": []map[string]any{{
				"name":        "strict_lookup",
				"description": "Lookup after initialized notification",
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
			}}}))
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
	defer server.Close()

	manager := NewProductionManager([]ServerSpec{{
		ID:        "strict-http",
		Transport: "http",
		URL:       server.URL,
	}})
	manager.Connect()
	defer manager.Disconnect()

	toolName := CanonicalToolName("strict-http", "strict_lookup")
	if tools := manager.Tools(); len(tools) != 1 || tools[0] != toolName {
		t.Fatalf("strict MCP server should list tools after initialized notification: %#v", tools)
	}
	mu.Lock()
	defer mu.Unlock()
	if methodCounts["initialize"] != 1 ||
		methodCounts["notifications/initialized"] != 1 ||
		methodCounts["tools/list"] != 1 {
		t.Fatalf("strict MCP handshake order/count mismatch: %#v", methodCounts)
	}
}

func TestProductionManagerRetriesTransientCallFailureAfterReconnect(t *testing.T) {
	var mu sync.Mutex
	methodCounts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		mu.Lock()
		methodCounts[request.Method]++
		callAttempt := methodCounts["tools/call"]
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"serverInfo": map[string]any{"name": "retry-test"}}))
		case "tools/list":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"tools": []map[string]any{{
				"name":        "lookup",
				"description": "Look up after reconnect",
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}},
			}}}))
		case "tools/call":
			if callAttempt == 1 {
				http.Error(w, "temporary upstream outage", http.StatusBadGateway)
				return
			}
			args, _ := request.Params["arguments"].(map[string]any)
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"content": []map[string]any{{
				"type": "text",
				"text": "retried result: " + fmt.Sprint(args["query"]),
			}}}))
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
	defer server.Close()

	manager := NewProductionManager([]ServerSpec{{
		ID:                "retry-server",
		Transport:         "http",
		URL:               server.URL,
		ReadOnlyToolNames: map[string]bool{"lookup": true},
	}})
	manager.Connect()
	defer manager.Disconnect()

	toolName := CanonicalToolName("retry-server", "lookup")
	called := manager.CallTool(toolName, true, map[string]any{"query": "needle"})
	if !mcpBoolField(called, "executed") ||
		!mcpBoolField(called, "retried") ||
		mcpResultText(called) != "retried result: needle" {
		t.Fatalf("transient MCP call should reconnect and retry once: %#v", called)
	}
	mu.Lock()
	initializeCount := methodCounts["initialize"]
	listCount := methodCounts["tools/list"]
	callCount := methodCounts["tools/call"]
	mu.Unlock()
	if initializeCount != 2 || listCount != 2 || callCount != 2 {
		t.Fatalf("transient retry should reconnect and call twice: initialize=%d list=%d call=%d", initializeCount, listCount, callCount)
	}
}

func TestShouldRetryMCPCallRequiresHostTypedTransportFailure(t *testing.T) {
	untrustedDiagnostics := []string{
		"write |1: file already closed",
		"write |1: use of closed file",
		"write |1: closed pipe",
		"stderr tail: connection reset and timed out",
	}
	for _, message := range untrustedDiagnostics {
		if shouldRetryMCPCall(errors.New(message)) {
			t.Fatalf("untyped remote diagnostic enabled retry for %q", message)
		}
	}
	if shouldRetryMCPCall(errors.New("mcp jsonrpc error -32603: tool rejected input")) {
		t.Fatalf("JSON-RPC tool errors must not be retried as transport failures")
	}
	if !shouldRetryMCPCall(&httpmcp.StatusError{Method: "tools/call", StatusCode: 502}) {
		t.Fatal("typed transient HTTP status should be retried")
	}
	if shouldRetryMCPCall(&httpmcp.StatusError{Method: "tools/call", StatusCode: 401}) {
		t.Fatal("typed authentication HTTP status must not be retried")
	}
}

type failingCallClient struct {
	calls  int
	err    error
	closed int
}

func (client *failingCallClient) ListTools() ([]ToolSpec, error)         { return nil, nil }
func (client *failingCallClient) ListPrompts() ([]PromptSpec, error)     { return nil, nil }
func (client *failingCallClient) ListResources() ([]ResourceSpec, error) { return nil, nil }
func (client *failingCallClient) Close()                                 { client.closed++ }
func (client *failingCallClient) CallTool(string, map[string]any) (any, error) {
	client.calls++
	return nil, client.err
}

type fatalManagerTransportError struct{}

func (fatalManagerTransportError) Error() string                         { return "fatal transport" }
func (fatalManagerTransportError) MCPTransportRetryable() bool           { return false }
func (fatalManagerTransportError) MCPTransportInvalidatesIdentity() bool { return true }

func TestFatalTransportFailureRevokesServerIdentityAndLiveAuthority(t *testing.T) {
	for name, callErr := range map[string]error{
		"generic fatal": fatalManagerTransportError{},
		"http protocol": &httpmcp.ProtocolError{Method: "tools/call", Kind: "response_id", Cause: errors.New("mismatch")},
	} {
		t.Run(name, func(t *testing.T) {
			client := &failingCallClient{err: callErr}
			toolName := "mcp__docs__lookup"
			identity := mcpTestVerifiedIdentity(t, "docs", "docs-server", "1.0.0", 1)
			manager := &ProductionManager{
				specs: []ServerSpec{{ID: "docs"}}, clients: map[string]mcpTransportClient{"docs": client},
				tools: map[string]managedTool{toolName: {
					ServerID: "docs", Name: toolName, RawName: "lookup",
					Tool: ToolSpec{InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`)},
				}},
				connectionEpochs: map[string]uint64{"docs": 1}, serverIdentities: map[string]string{"docs": identity},
				sourceProbes: map[string]domainsecurity.VerifiedSourceProbe{}, failures: map[string]string{},
			}
			result := manager.CallTool(toolName, true, map[string]any{})
			if result["code"] != "mcp_call_failed" || result["executed"] != false || client.calls != 1 || client.closed != 1 {
				t.Fatalf("fatal transport was not isolated: result=%#v client=%#v", result, client)
			}
			if _, ok := manager.ToolConnectionEpoch(toolName); ok {
				t.Fatal("closed transport still exposed a live connection epoch")
			}
			if identity, ok := manager.ToolServerIdentity(toolName, domainsecurity.TurnSecurityContext{}); ok || identity != "" {
				t.Fatalf("closed transport still authorized server identity: %q ok=%v", identity, ok)
			}
			if live := manager.LiveTools(); len(live) != 0 {
				t.Fatalf("closed transport remained in live tool catalog: %#v", live)
			}
			if manager.connectionEpochs["docs"] != 2 || manager.serverIdentities["docs"] != "" || manager.clients["docs"] != nil {
				t.Fatalf("fatal transport did not invalidate connection authority: epochs=%#v identities=%#v clients=%#v", manager.connectionEpochs, manager.serverIdentities, manager.clients)
			}
		})
	}
}

func TestWriteToolTransportAbortNeverRetries(t *testing.T) {
	client := &failingCallClient{err: &httpmcp.StatusError{Method: "tools/call", StatusCode: http.StatusBadGateway}}
	toolName := "mcp__docs__write_report"
	manager := &ProductionManager{
		specs:   []ServerSpec{{ID: "docs", ReadOnlyToolNames: map[string]bool{}}},
		clients: map[string]mcpTransportClient{"docs": client},
		tools: map[string]managedTool{toolName: {
			ServerID: "docs", Name: toolName, RawName: "write_report",
			Tool: ToolSpec{ReadOnlyHint: true},
		}},
	}
	result := manager.CallTool(toolName, true, map[string]any{"path": "report.md"})
	if result["executed"] != false || client.calls != 1 {
		t.Fatalf("write tool transport ambiguity retried: result=%#v calls=%d", result, client.calls)
	}
	if _, retried := result["retried"]; retried {
		t.Fatalf("write tool result claims a retry: %#v", result)
	}
}

func TestProductionManagerDoesNotRetryJSONRPCToolErrors(t *testing.T) {
	var mu sync.Mutex
	methodCounts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		mu.Lock()
		methodCounts[request.Method]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"serverInfo": map[string]any{"name": "jsonrpc-error-test"}}))
		case "tools/list":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"tools": []map[string]any{{
				"name":         "dangerous",
				"description":  "Return a business error",
				"inputSchema":  map[string]any{"type": "object"},
				"outputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
			}}}))
		case "tools/call":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32603, "message": "tool rejected input"},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
	defer server.Close()

	manager := NewProductionManager([]ServerSpec{{
		ID:        "jsonrpc-error-server",
		Transport: "http",
		URL:       server.URL,
	}})
	manager.Connect()
	defer manager.Disconnect()

	toolName := CanonicalToolName("jsonrpc-error-server", "dangerous")
	called := manager.CallTool(toolName, true, map[string]any{})
	if mcpBoolField(called, "executed") ||
		mcpBoolField(called, "retried") ||
		mcpStringField(called, "transportStatus") != "success" ||
		mcpStringField(called, "code") != "mcp_jsonrpc_internal_error" ||
		strings.Contains(fmt.Sprint(called), "tool rejected input") {
		t.Fatalf("JSON-RPC tool errors should not be retried as transport reconnects: %#v", called)
	}
	rpcError, _ := called["rpcError"].(map[string]any)
	if rpcError["class"] != "internal_error" || rpcError["code"] != float64(-32603) && rpcError["code"] != -32603 {
		t.Fatalf("bounded typed JSON-RPC metadata was lost: %#v", called)
	}
	mu.Lock()
	initializeCount := methodCounts["initialize"]
	listCount := methodCounts["tools/list"]
	callCount := methodCounts["tools/call"]
	mu.Unlock()
	if initializeCount != 1 || listCount != 1 || callCount != 1 {
		t.Fatalf("JSON-RPC tool error should not reconnect: initialize=%d list=%d call=%d", initializeCount, listCount, callCount)
	}
}

func TestProductionManagerRejectsHTTPOutputSchemaMismatchAfterExecution(t *testing.T) {
	var callCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"serverInfo": map[string]any{"name": "output-mismatch"}}))
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"tools": []map[string]any{{
				"name": "lookup", "inputSchema": map[string]any{"type": "object"},
				"outputSchema": map[string]any{
					"type": "object", "properties": map[string]any{"amount": map[string]any{"type": "integer"}},
					"required": []string{"amount"}, "additionalProperties": false,
				},
			}}}))
		case "tools/call":
			callCount++
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{
				"content":           []map[string]any{{"type": "text", "text": "amount=100"}},
				"structuredContent": map[string]any{"amount": "100"},
			}))
		default:
			t.Fatalf("unexpected method %q", request.Method)
		}
	}))
	defer server.Close()
	manager := NewProductionManager([]ServerSpec{{ID: "output-mismatch", Transport: "http", URL: server.URL}})
	manager.Connect()
	defer manager.Disconnect()
	result := manager.CallTool(CanonicalToolName("output-mismatch", "lookup"), true, map[string]any{})
	if callCount != 1 || result["executed"] != true || result["transportStatus"] != "success" || result["semanticStatus"] != "failure" ||
		result["isError"] != true || result["code"] != "mcp_output_schema_invalid" {
		t.Fatalf("HTTP output mismatch was not classified fail-closed: calls=%d result=%#v", callCount, result)
	}
	lossless, found := result[domainmcp.HostRawToolResultKey].(domainmcp.LosslessToolResult)
	if !found || !domainmcp.ValidToolResultObservation(lossless) || !lossless.Observation.IsError || lossless.Observation.SemanticStatus != "failure" {
		t.Fatalf("invalid output was not retained only as a host-private negative observation: %#v", result)
	}
}

func TestProductionManagerRemoteMCPAuthDiagnostics(t *testing.T) {
	const secret = "mcp-secret-token"
	unauthorized := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized: invalid token", http.StatusUnauthorized)
	}))
	defer unauthorized.Close()

	manager := NewProductionManager([]ServerSpec{
		{
			ID:              "lazy-remote",
			Transport:       "streamable-http",
			URL:             "https://mcp.example.test/mcp",
			BackgroundStart: true,
		},
		{
			ID:        "requires-auth",
			Transport: "http",
			URL:       unauthorized.URL + "?key=" + secret,
			Headers:   map[string]string{"Authorization": "Bearer " + secret},
			Env:       map[string]string{"MCP_API_KEY": secret},
		},
	})
	manager.Connect()
	defer manager.Disconnect()

	diagnostics := manager.ServerDiagnostics()
	diagnosticsJSON := mcpMustJSONText(t, diagnostics)
	if strings.Contains(diagnosticsJSON, secret) {
		t.Fatalf("MCP auth diagnostics leaked credential: %s", diagnosticsJSON)
	}

	lazy := mcpServerDiagnosticByID(t, diagnostics, "lazy-remote")
	if mcpStringField(lazy, "authStatus") != "possible" ||
		mcpStringField(lazy, "authUrl") != "https://mcp.example.test/mcp" ||
		mcpBoolField(lazy, "authConfigured") ||
		!mcpBoolField(lazy, "lazyCatalogPending") {
		t.Fatalf("lazy remote MCP server should advertise possible auth setup without credentials: %#v", lazy)
	}

	required := mcpServerDiagnosticByID(t, diagnostics, "requires-auth")
	if mcpStringField(required, "authStatus") != "required" ||
		!mcpBoolField(required, "authConfigured") ||
		!strings.Contains(mcpStringField(required, "authUrl"), "key=%3Credacted%3E") ||
		!strings.Contains(mcpStringField(required, "failure"), "401") {
		t.Fatalf("401 MCP server should advertise required auth without secrets: %#v", required)
	}
}

func TestProductionManagerHTTPTransportAcceptsSSEJSONRPCResponses(t *testing.T) {
	var mu sync.Mutex
	methodCounts := map[string]int{}
	listSession := ""
	callSession := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			if r.Header.Get("Mcp-Session-Id") != "session-123" || r.Header.Get("MCP-Protocol-Version") != "2025-11-25" {
				t.Errorf("session teardown headers mismatch: %#v", r.Header)
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var request struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		mu.Lock()
		methodCounts[request.Method]++
		if request.Method == "tools/list" {
			listSession = r.Header.Get("Mcp-Session-Id")
		}
		if request.Method == "tools/call" {
			callSession = r.Header.Get("Mcp-Session-Id")
		}
		mu.Unlock()
		if request.Method == "notifications/initialized" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Mcp-Session-Id", "session-123")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "event: notification\ndata: %s\n\n", mcpMustJSONText(t, map[string]any{
			"jsonrpc": "2.0",
			"method":  "notifications/progress",
			"params":  map[string]any{"message": "ignore me"},
		}))
		switch request.Method {
		case "initialize":
			_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", mcpMustJSONText(t, jsonRPCResult(request.ID, map[string]any{"serverInfo": map[string]any{"name": "sse-test"}})))
		case "tools/list":
			_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", mcpMustJSONText(t, jsonRPCResult(request.ID, map[string]any{"tools": []map[string]any{{
				"name":        "stream_lookup",
				"description": "Look up through SSE JSON-RPC",
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}},
			}}})))
		case "tools/call":
			args, _ := request.Params["arguments"].(map[string]any)
			_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", mcpMustJSONText(t, jsonRPCResult(request.ID, map[string]any{"content": []map[string]any{{
				"type": "text",
				"text": "sse result: " + fmt.Sprint(args["query"]),
			}}})))
		default:
			_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", mcpMustJSONText(t, map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			}))
		}
	}))
	defer server.Close()

	manager := NewProductionManager([]ServerSpec{{
		ID:        "sse-server",
		Transport: "sse",
		URL:       server.URL,
	}})
	manager.Connect()
	defer manager.Disconnect()

	toolName := CanonicalToolName("sse-server", "stream_lookup")
	if tools := manager.Tools(); len(tools) != 1 || tools[0] != toolName {
		t.Fatalf("unexpected SSE MCP tools: %#v", tools)
	}
	called := manager.CallTool(toolName, true, map[string]any{"query": "needle"})
	if !mcpBoolField(called, "executed") || mcpResultText(called) != "sse result: needle" {
		t.Fatalf("unexpected SSE MCP call result: %#v", called)
	}
	mu.Lock()
	initializeCount := methodCounts["initialize"]
	listCount := methodCounts["tools/list"]
	callCount := methodCounts["tools/call"]
	gotListSession := listSession
	gotCallSession := callSession
	mu.Unlock()
	if initializeCount != 1 || listCount != 1 || callCount != 1 {
		t.Fatalf("unexpected SSE MCP call counts: initialize=%d list=%d call=%d", initializeCount, listCount, callCount)
	}
	if gotListSession != "session-123" || gotCallSession != "session-123" {
		t.Fatalf("MCP HTTP session header was not round-tripped: list=%q call=%q", gotListSession, gotCallSession)
	}
}

func TestProductionManagerBackgroundStartIsLazyUntilRefresh(t *testing.T) {
	var mu sync.Mutex
	methodCounts := map[string]int{}
	newServer := func(toolName string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			var request struct {
				ID     int    `json:"id"`
				Method string `json:"method"`
			}
			if err := json.Unmarshal(body, &request); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			mu.Lock()
			methodCounts[toolName+"."+request.Method]++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			switch request.Method {
			case "notifications/initialized":
				w.WriteHeader(http.StatusAccepted)
			case "initialize":
				_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"serverInfo": map[string]any{"name": toolName}}))
			case "tools/list":
				_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"tools": []map[string]any{{
					"name":        toolName,
					"description": "Tool " + toolName,
					"inputSchema": map[string]any{"type": "object"},
				}}}))
			default:
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      request.ID,
					"error":   map[string]any{"code": -32601, "message": "method not found"},
				})
			}
		}))
	}
	foreground := newServer("foreground")
	defer foreground.Close()
	background := newServer("background")
	defer background.Close()

	manager := NewProductionManager([]ServerSpec{
		{ID: "foreground", Transport: "http", URL: foreground.URL},
		{ID: "background", Transport: "http", URL: background.URL, LowPriority: true, BackgroundStart: true},
	})
	manager.Connect()
	defer manager.Disconnect()

	foregroundTool := CanonicalToolName("foreground", "foreground")
	backgroundTool := CanonicalToolName("background", "background")
	backgroundConnectTool := CanonicalToolName("background", "connect")
	if tools := manager.Tools(); len(tools) != 2 || !containsMCPTool(tools, foregroundTool) || !containsMCPTool(tools, backgroundConnectTool) {
		t.Fatalf("initial MCP catalog should contain foreground tools and lazy background placeholder: %#v", tools)
	}
	mu.Lock()
	backgroundInitializeCount := methodCounts["background.initialize"]
	backgroundListCount := methodCounts["background.tools/list"]
	mu.Unlock()
	if backgroundInitializeCount != 0 || backgroundListCount != 0 {
		t.Fatalf("background MCP server should not be contacted on Connect: initialize=%d list=%d", backgroundInitializeCount, backgroundListCount)
	}
	serverDiagnostics := manager.ServerDiagnostics()
	if len(serverDiagnostics) != 2 {
		t.Fatalf("expected diagnostics for both configured MCP servers: %#v", serverDiagnostics)
	}
	var foundLazyPending bool
	var foundLazyPlaceholder bool
	for _, item := range serverDiagnostics {
		record, _ := item.(map[string]any)
		if mcpStringField(record, "id") == "background" {
			foundLazyPending = mcpBoolField(record, "lazyCatalogPending")
			foundLazyPlaceholder = mcpBoolField(record, "lazyPlaceholder")
		}
	}
	if !foundLazyPending || !foundLazyPlaceholder {
		t.Fatalf("background MCP diagnostics should report lazy catalog pending placeholder: %#v", serverDiagnostics)
	}

	refreshed := manager.RefreshCatalog()
	if !mcpBoolField(refreshed, "available") {
		t.Fatalf("refresh should connect configured MCP servers: %#v", refreshed)
	}
	if tools := manager.Tools(); !containsMCPTool(tools, foregroundTool) || !containsMCPTool(tools, backgroundTool) {
		t.Fatalf("refreshed MCP catalog should contain foreground and background tools: %#v", tools)
	}
	mu.Lock()
	backgroundInitializeCount = methodCounts["background.initialize"]
	backgroundListCount = methodCounts["background.tools/list"]
	mu.Unlock()
	if backgroundInitializeCount == 0 || backgroundListCount == 0 {
		t.Fatalf("refresh should contact background MCP server: initialize=%d list=%d", backgroundInitializeCount, backgroundListCount)
	}
}

func TestProductionManagerLazyPlaceholderConnectsCacheMissOnFirstUse(t *testing.T) {
	var mu sync.Mutex
	methodCounts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		mu.Lock()
		methodCounts[request.Method]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"serverInfo": map[string]any{"name": "lazy"}}))
		case "tools/list":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"tools": []map[string]any{{
				"name":        "lookup",
				"description": "Lookup after lazy connect",
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}},
			}}}))
		case "tools/call":
			args, _ := request.Params["arguments"].(map[string]any)
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"content": []map[string]any{{
				"type": "text",
				"text": "lookup result: " + fmt.Sprint(args["query"]),
			}}}))
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
	defer server.Close()

	spec := ServerSpec{
		ID:              "lazy-server",
		Transport:       "http",
		URL:             server.URL,
		BackgroundStart: true,
	}
	manager := NewProductionManagerWithCache([]ServerSpec{spec}, t.TempDir())
	manager.Connect()
	defer manager.Disconnect()

	connectTool := CanonicalToolName(spec.ID, "connect")
	realTool := CanonicalToolName(spec.ID, "lookup")
	if tools := manager.Tools(); len(tools) != 1 || tools[0] != connectTool {
		t.Fatalf("cache-miss lazy MCP catalog should advertise connect placeholder only: %#v", tools)
	}
	mu.Lock()
	initializeCount := methodCounts["initialize"]
	listCount := methodCounts["tools/list"]
	mu.Unlock()
	if initializeCount != 0 || listCount != 0 {
		t.Fatalf("cache-miss lazy MCP connect should not contact server before first use: initialize=%d list=%d", initializeCount, listCount)
	}
	diagnostics := manager.Diagnostics()
	if diagnostics["status"] != "lazy" ||
		diagnostics["available"] != false ||
		diagnostics["active"] != false ||
		diagnostics["connectable"] != true ||
		diagnostics["schemaHintAvailable"] != false ||
		diagnostics["lazyPlaceholderCount"] != float64(1) ||
		diagnostics["cachedSchemaHitCount"] != float64(0) {
		t.Fatalf("lazy placeholder diagnostics mismatch: %#v", diagnostics)
	}

	connected := manager.CallTool(connectTool, true, map[string]any{})
	if !mcpBoolField(connected, "executed") ||
		mcpStringField(connected, "code") != "mcp_lazy_connected" ||
		!containsMCPTool(stringSliceFromAny(connected["toolNames"]), realTool) {
		t.Fatalf("lazy placeholder should connect and return real tools: %#v", connected)
	}
	if tools := manager.Tools(); len(tools) != 1 || tools[0] != realTool {
		t.Fatalf("lazy placeholder should be replaced by real MCP catalog: %#v", tools)
	}
	mu.Lock()
	initializeCount = methodCounts["initialize"]
	listCount = methodCounts["tools/list"]
	callCount := methodCounts["tools/call"]
	mu.Unlock()
	if initializeCount != 1 || listCount != 1 || callCount != 0 {
		t.Fatalf("lazy placeholder should connect/list without calling a real tool: initialize=%d list=%d call=%d", initializeCount, listCount, callCount)
	}
	reconnected := manager.Diagnostics()
	if reconnected["status"] != "available" ||
		reconnected["active"] != true ||
		reconnected["lazyPlaceholderCount"] != float64(0) ||
		reconnected["cachedSchemaHitCount"] != float64(0) {
		t.Fatalf("lazy first-use reconnect diagnostics mismatch: %#v", reconnected)
	}

	called := manager.CallTool(realTool, true, map[string]any{"query": "needle"})
	if !mcpBoolField(called, "executed") || mcpResultText(called) != "lookup result: needle" {
		t.Fatalf("real MCP tool should execute after lazy connect: %#v", called)
	}
}

func TestRemoteReadOnlyHintCannotAuthorizeEvidenceGrant(t *testing.T) {
	spec := ServerSpec{ID: "docs", ReadOnlyToolNames: map[string]bool{}}
	manager := NewProductionManager(nil)
	manager.specs = []ServerSpec{spec}
	manager.registerToolsNoLock(spec, []ToolSpec{{Name: "query", ReadOnlyHint: true, InputSchema: json.RawMessage(`{"type":"object"}`)}})
	toolName := CanonicalToolName(spec.ID, "query")
	if !manager.ToolRemoteReadOnlyHint(toolName) {
		t.Fatal("test fixture did not retain the remote scheduling hint")
	}
	if manager.ToolReadOnlyHint(toolName) {
		t.Fatal("remote readOnlyHint minted host evidence authority")
	}
	manager.specs[0].ReadOnlyToolNames["query"] = true
	if !manager.ToolReadOnlyHint(toolName) {
		t.Fatal("host-owned read-only allowlist was not honored")
	}
}

func TestCodegraphServerIDCannotMintHostReadOnlyAuthority(t *testing.T) {
	spec := ApplyKnownOverrides(ServerSpec{ID: "codegraph"}, "/workspace")
	manager := NewProductionManager(nil)
	manager.specs = []ServerSpec{spec}
	manager.registerToolsNoLock(spec, []ToolSpec{{Name: "search", ReadOnlyHint: true, InputSchema: json.RawMessage(`{"type":"object"}`)}})
	toolName := CanonicalToolName(spec.ID, "search")
	if !manager.ToolRemoteReadOnlyHint(toolName) || manager.ToolReadOnlyHint(toolName) {
		t.Fatalf("server id or remote hint minted host read-only authority: spec=%#v", spec)
	}
}

func TestExactPinnedFundsIdentityCannotEscapeReservedNamespaceQuarantine(t *testing.T) {
	spec := pinnedFundsSourceSpec(t, ServerSpec{
		ID: "analytix_funds", ExpectedServerName: "analytix_funds", ExpectedServerVersion: "0.16.16",
	})
	spec = ApplyKnownOverrides(spec, "/workspace")
	manager := NewProductionManager(nil)
	manager.specs = []ServerSpec{spec}
	manager.registerToolsNoLock(spec, []ToolSpec{
		{Name: "count_case_rows", ReadOnlyHint: false, InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "write_report", ReadOnlyHint: true, InputSchema: json.RawMessage(`{"type":"object"}`)},
	})
	countTool := CanonicalToolName(spec.ID, "count_case_rows")
	writeTool := CanonicalToolName(spec.ID, "write_report")
	if manager.ToolReadOnlyHint(countTool) {
		t.Fatal("exact installed analytix_funds identity escaped the reserved namespace quarantine")
	}
	if manager.ToolReadOnlyHint(writeTool) {
		t.Fatal("remote write_report readOnlyHint escaped the host exact-tool allowlist")
	}
	if err := os.WriteFile(spec.EntrypointPath, []byte("export const mutated = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if manager.ToolReadOnlyHint(countTool) {
		t.Fatal("mutated installed source tree escaped the reserved namespace quarantine")
	}
}

func TestFundsCountReadOnlyOverrideRejectsAliasesVersionsAndUnpinnedSpecs(t *testing.T) {
	base := ServerSpec{
		ID: "analytix_funds", ExpectedServerName: "analytix_funds", ExpectedServerVersion: "0.16.16",
		IdentitySource: "installed-plugin-manifest",
	}
	tests := map[string]ServerSpec{
		"server alias":   func() ServerSpec { value := base; value.ID = "analytix-fund-analysis"; return value }(),
		"wrong version":  func() ServerSpec { value := base; value.ExpectedServerVersion = "0.16.15"; return value }(),
		"wrong identity": func() ServerSpec { value := base; value.IdentitySource = "user-config"; return value }(),
		"wrong name":     func() ServerSpec { value := base; value.ExpectedServerName = "spoofed_funds"; return value }(),
	}
	for name, spec := range tests {
		t.Run(name, func(t *testing.T) {
			spec = ApplyKnownOverrides(spec, "/workspace")
			if spec.ReadOnlyToolNames["count_case_rows"] {
				t.Fatal("non-exact funds identity received host read-only authority")
			}
		})
	}
}

func TestProductionManagerUsesCachedSchemaForLazyCatalogAndFirstUseReconnect(t *testing.T) {
	cacheDir := t.TempDir()
	var mu sync.Mutex
	methodCounts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		mu.Lock()
		methodCounts[request.Method]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"serverInfo": map[string]any{"name": "cached"}}))
		case "tools/list":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"tools": []map[string]any{{
				"name":        "lookup",
				"description": "Lookup cached data",
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}},
			}}}))
		case "tools/call":
			args, _ := request.Params["arguments"].(map[string]any)
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"content": []map[string]any{{
				"type": "text",
				"text": "cached result: " + fmt.Sprint(args["query"]),
			}}}))
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
	defer server.Close()

	spec := ServerSpec{
		ID:                "cached-server",
		Transport:         "http",
		URL:               server.URL,
		Headers:           map[string]string{"Authorization": "Bearer mcp-test-token"},
		BackgroundStart:   true,
		ReadOnlyToolNames: map[string]bool{"lookup": true},
	}
	warm := NewProductionManagerWithCache([]ServerSpec{spec}, cacheDir)
	warm.RefreshCatalog()
	warm.Disconnect()
	if data, err := os.ReadFile(cachedSchemaPath(cacheDir, spec.ID)); err != nil {
		t.Fatalf("expected cached MCP schema file: %v", err)
	} else if strings.Contains(string(data), "mcp-test-token") || strings.Contains(string(data), server.URL) {
		t.Fatalf("cached MCP schema must not persist transport secrets: %s", string(data))
	}

	mu.Lock()
	methodCounts = map[string]int{}
	mu.Unlock()
	manager := NewProductionManagerWithCache([]ServerSpec{spec}, cacheDir)
	manager.Connect()
	defer manager.Disconnect()

	toolName := CanonicalToolName(spec.ID, "lookup")
	if tools := manager.Tools(); len(tools) != 1 || tools[0] != toolName {
		t.Fatalf("cached lazy MCP catalog should advertise cached tool: %#v", tools)
	}
	mu.Lock()
	initializeCount := methodCounts["initialize"]
	listCount := methodCounts["tools/list"]
	mu.Unlock()
	if initializeCount != 0 || listCount != 0 {
		t.Fatalf("cache-hit lazy connect must not contact background MCP server: initialize=%d list=%d", initializeCount, listCount)
	}
	diagnostics := manager.Diagnostics()
	if diagnostics["status"] != "cached" ||
		diagnostics["cachedSchemaHitCount"] != float64(1) ||
		diagnostics["available"] != false ||
		diagnostics["active"] != false ||
		diagnostics["connectable"] != true ||
		diagnostics["schemaHintAvailable"] != true {
		t.Fatalf("cached MCP diagnostics mismatch: %#v", diagnostics)
	}
	cachedCatalogFingerprint := mcpStringField(diagnostics, "catalogFingerprint")
	serverDiagnostics := manager.ServerDiagnostics()
	if len(serverDiagnostics) != 1 {
		t.Fatalf("expected server diagnostics: %#v", serverDiagnostics)
	}
	serverDiagnostic, _ := serverDiagnostics[0].(map[string]any)
	if !mcpBoolField(serverDiagnostic, "schemaCacheHit") ||
		!mcpBoolField(serverDiagnostic, "lazyCatalogPending") ||
		mcpBoolField(serverDiagnostic, "connected") {
		t.Fatalf("cached server diagnostics mismatch: %#v", serverDiagnostics)
	}

	called := manager.CallTool(toolName, true, map[string]any{"query": "needle"})
	if !mcpBoolField(called, "executed") ||
		mcpResultText(called) != "cached result: needle" ||
		!mcpBoolField(called, "readOnlyHint") {
		t.Fatalf("cached MCP first-use reconnect/call failed: %#v", called)
	}
	mu.Lock()
	initializeCount = methodCounts["initialize"]
	listCount = methodCounts["tools/list"]
	callCount := methodCounts["tools/call"]
	mu.Unlock()
	if initializeCount == 0 || listCount == 0 || callCount != 1 {
		t.Fatalf("first cached MCP call should reconnect and execute once: initialize=%d list=%d call=%d", initializeCount, listCount, callCount)
	}
	reconnectedDiagnostics := manager.Diagnostics()
	if reconnectedDiagnostics["status"] != "available" ||
		reconnectedDiagnostics["active"] != true ||
		reconnectedDiagnostics["cachedSchemaHitCount"] != float64(0) ||
		mcpStringField(reconnectedDiagnostics, "catalogFingerprint") == "" ||
		mcpStringField(reconnectedDiagnostics, "catalogFingerprint") != cachedCatalogFingerprint {
		t.Fatalf("first-use reconnect should refresh active MCP diagnostics without catalog churn: before=%#v after=%#v", diagnostics, reconnectedDiagnostics)
	}
}

func TestProductionManagerCachedSchemaInvalidatesOnSpecFingerprintChange(t *testing.T) {
	cacheDir := t.TempDir()
	spec := ServerSpec{
		ID:              "fingerprint-server",
		Transport:       "http",
		URL:             "https://mcp.example.test",
		Env:             map[string]string{"B": "two", "A": "one"},
		Headers:         map[string]string{"Authorization": "Bearer token"},
		BackgroundStart: true,
	}
	reordered := spec
	reordered.Env = map[string]string{"A": "one", "B": "two"}
	if SpecFingerprint(spec) != SpecFingerprint(reordered) {
		t.Fatalf("SpecFingerprint should be stable across map iteration/order")
	}
	if err := saveCachedSchema(cacheDir, spec.ID, CachedSchema{
		SpecHash: SpecFingerprint(spec),
		Tools: []ToolSpec{{
			Name:         "stale",
			Description:  "Stale cached tool",
			InputSchema:  json.RawMessage(`{"type":"object"}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		}},
	}); err != nil {
		t.Fatalf("save cached schema: %v", err)
	}

	changed := spec
	changed.Env = map[string]string{"A": "changed", "B": "two"}
	if SpecFingerprint(spec) == SpecFingerprint(changed) {
		t.Fatalf("SpecFingerprint should change when load-bearing env changes")
	}
	manager := NewProductionManagerWithCache([]ServerSpec{changed}, cacheDir)
	manager.Connect()
	defer manager.Disconnect()
	connectTool := CanonicalToolName(changed.ID, "connect")
	if tools := manager.Tools(); len(tools) != 1 || tools[0] != connectTool {
		t.Fatalf("stale cached MCP schema should not be advertised after spec change; only lazy connect placeholder is allowed: %#v", tools)
	}
	diagnostics := manager.Diagnostics()
	if diagnostics["cachedSchemaHitCount"] != float64(0) ||
		diagnostics["lazyPlaceholderCount"] != float64(1) ||
		diagnostics["status"] != "lazy" ||
		diagnostics["available"] != false ||
		diagnostics["connectable"] != true ||
		diagnostics["schemaHintAvailable"] != false {
		t.Fatalf("stale cache miss diagnostics mismatch: %#v", diagnostics)
	}
}

func TestProductionManagerReportsStaleCachedSchemaWhenFirstUseToolDisappears(t *testing.T) {
	cacheDir := t.TempDir()
	var mu sync.Mutex
	methodCounts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		mu.Lock()
		methodCounts[request.Method]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"serverInfo": map[string]any{"name": "stale-cache"}}))
		case "tools/list":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"tools": []map[string]any{{
				"name":        "real",
				"description": "Real current tool",
				"inputSchema": map[string]any{"type": "object"},
			}}}))
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
	defer server.Close()

	spec := ServerSpec{
		ID:              "stale-server",
		Transport:       "http",
		URL:             server.URL,
		BackgroundStart: true,
	}
	if err := saveCachedSchema(cacheDir, spec.ID, CachedSchema{
		SpecHash: SpecFingerprint(spec),
		Tools: []ToolSpec{{
			Name:         "stale",
			Description:  "Stale cached tool",
			InputSchema:  json.RawMessage(`{"type":"object"}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		}},
	}); err != nil {
		t.Fatalf("save cached schema: %v", err)
	}

	manager := NewProductionManagerWithCache([]ServerSpec{spec}, cacheDir)
	manager.Connect()
	defer manager.Disconnect()

	staleTool := CanonicalToolName(spec.ID, "stale")
	realTool := CanonicalToolName(spec.ID, "real")
	if tools := manager.Tools(); len(tools) != 1 || tools[0] != staleTool {
		t.Fatalf("expected cached stale tool before first use: %#v", tools)
	}
	cachedCatalogFingerprint := mcpStringField(manager.Diagnostics(), "catalogFingerprint")
	result := manager.CallTool(staleTool, true, map[string]any{})
	if mcpBoolField(result, "executed") || mcpStringField(result, "code") != "mcp_cached_schema_stale" {
		t.Fatalf("expected stale cached schema error, got %#v", result)
	}
	if !strings.Contains(mcpStringField(result, "error"), "did not expose cached tool") {
		t.Fatalf("stale cached schema error should be diagnostic: %#v", result)
	}
	if tools := manager.Tools(); len(tools) != 1 || tools[0] != realTool {
		t.Fatalf("catalog should swap to real tools after stale cache reconnect: %#v", tools)
	}
	mu.Lock()
	initializeCount := methodCounts["initialize"]
	listCount := methodCounts["tools/list"]
	mu.Unlock()
	if initializeCount != 1 || listCount != 1 {
		t.Fatalf("stale cached call should reconnect exactly once: initialize=%d list=%d", initializeCount, listCount)
	}
	reconnectedDiagnostics := manager.Diagnostics()
	if reconnectedDiagnostics["status"] != "available" ||
		reconnectedDiagnostics["active"] != true ||
		mcpStringField(reconnectedDiagnostics, "catalogFingerprint") == "" ||
		mcpStringField(reconnectedDiagnostics, "catalogFingerprint") == cachedCatalogFingerprint {
		t.Fatalf("stale cached reconnect should refresh catalog diagnostics: before=%s after=%#v", cachedCatalogFingerprint, reconnectedDiagnostics)
	}
}

func TestProductionManagerCatalogFingerprintCanonicalizesSchemas(t *testing.T) {
	first := catalogFingerprintForManagedTools(map[string]managedTool{
		"mcp_docs_lookup": {
			Tool: ToolSpec{
				Description: "Lookup docs",
				InputSchema: json.RawMessage(`{"type":["object"],"properties":{"b":{"type":"number","enum":[2,1]},"a":{"type":"string"}},"dependentRequired":{"b":["z","a"]},"required":["b","a"]}`),
			},
		},
	})
	reordered := catalogFingerprintForManagedTools(map[string]managedTool{
		"mcp_docs_lookup": {
			Tool: ToolSpec{
				Description: "Lookup docs",
				InputSchema: json.RawMessage(`{"required":["a","b"],"dependentRequired":{"b":["a","z"]},"properties":{"a":{"type":"string"},"b":{"enum":[1,2],"type":"number"}},"type":["object"]}`),
			},
		},
	})
	changed := catalogFingerprintForManagedTools(map[string]managedTool{
		"mcp_docs_lookup": {
			Tool: ToolSpec{
				Description: "Lookup docs",
				InputSchema: json.RawMessage(`{"type":["object"],"properties":{"a":{"type":"string"},"b":{"type":"integer","enum":[1,2]}},"required":["a","b"]}`),
			},
		},
	})

	if first != reordered {
		t.Fatalf("schema key reorder should keep the MCP catalog fingerprint stable: %s != %s", first, reordered)
	}
	if first == changed {
		t.Fatalf("schema semantic changes should change the MCP catalog fingerprint: %s", first)
	}
	malformed := catalogFingerprintForManagedTools(map[string]managedTool{
		"mcp_docs_lookup": {
			Tool: ToolSpec{
				Description: "Lookup docs",
				InputSchema: json.RawMessage(`{"required":["a","b"],"dependentRequired":{"b":["a","z"],"bad":true},"properties":{"a":{"type":"string"},"b":{"enum":[1,2],"type":"number"}},"type":["object"]}`),
			},
		},
	})
	if first == malformed {
		t.Fatal("malformed schema was collapsed into the valid catalog fingerprint")
	}
	if len(first) != 64 {
		t.Fatalf("catalog fingerprint is not full SHA-256: %q", first)
	}
	rawBindingChanged := catalogFingerprintForManagedTools(map[string]managedTool{
		"mcp_docs_lookup": {
			ServerID: "docs", RawName: "other_lookup",
			Tool: ToolSpec{Description: "Lookup docs", InputSchema: json.RawMessage(`{"type":["object"],"properties":{"b":{"type":"number","enum":[2,1]},"a":{"type":"string"}},"dependentRequired":{"b":["z","a"]},"required":["b","a"]}`)},
		},
	})
	if first == rawBindingChanged {
		t.Fatal("catalog fingerprint ignored exact raw transport binding")
	}
}

func TestProductionManagerToolContractsReturnDefensiveCopies(t *testing.T) {
	manager := &ProductionManager{
		tools: map[string]managedTool{
			"mcp__docs__lookup": {
				Tool: ToolSpec{
					InputSchema:  json.RawMessage(`{"properties":{"query":{"type":"string"}},"required":["query"],"type":"object"}`),
					OutputSchema: json.RawMessage(`{"properties":{"text":{"type":"string"}},"required":["text"],"type":"object"}`),
				},
			},
		},
	}

	schema, ok := manager.ToolInputSchema("mcp__docs__lookup")
	if !ok || string(schema) != `{"properties":{"query":{"type":"string"}},"required":["query"],"type":"object"}` {
		t.Fatalf("ToolInputSchema returned %#v ok=%v", string(schema), ok)
	}
	schema[0] = '['
	again, ok := manager.ToolInputSchema("mcp__docs__lookup")
	if !ok || string(again) != `{"properties":{"query":{"type":"string"}},"required":["query"],"type":"object"}` {
		t.Fatalf("ToolInputSchema should return a copy, got %#v ok=%v", string(again), ok)
	}
	if _, ok := manager.ToolInputSchema("missing"); ok {
		t.Fatal("missing MCP tool should not return an input schema")
	}
	output, ok := manager.ToolOutputSchema("mcp__docs__lookup")
	if !ok || string(output) != `{"properties":{"text":{"type":"string"}},"required":["text"],"type":"object"}` {
		t.Fatalf("ToolOutputSchema returned %#v ok=%v", string(output), ok)
	}
	output[0] = '['
	againOutput, ok := manager.ToolOutputSchema("mcp__docs__lookup")
	if !ok || string(againOutput) != `{"properties":{"text":{"type":"string"}},"required":["text"],"type":"object"}` {
		t.Fatalf("ToolOutputSchema should return a copy, got %#v ok=%v", string(againOutput), ok)
	}
	if _, ok := manager.ToolOutputSchema("missing"); ok {
		t.Fatal("missing MCP tool should not return an output schema")
	}
}

func TestProductionManagerToolDescriptionReturnsConfiguredDescription(t *testing.T) {
	manager := &ProductionManager{
		tools: map[string]managedTool{
			"mcp__docs__lookup": {
				Tool: ToolSpec{
					Description: "Lookup current project docs",
				},
			},
		},
	}

	description, ok := manager.ToolDescription("mcp__docs__lookup")
	if !ok || description != "Lookup current project docs" {
		t.Fatalf("ToolDescription returned %#v ok=%v", description, ok)
	}
	if _, ok := manager.ToolDescription("missing"); ok {
		t.Fatal("missing MCP tool should not return a description")
	}
}

type connectionEpochClient struct{ calls int }

func (client *connectionEpochClient) ListTools() ([]ToolSpec, error)         { return nil, nil }
func (client *connectionEpochClient) ListPrompts() ([]PromptSpec, error)     { return nil, nil }
func (client *connectionEpochClient) ListResources() ([]ResourceSpec, error) { return nil, nil }
func (client *connectionEpochClient) Close()                                 {}
func (client *connectionEpochClient) CallTool(string, map[string]any) (any, error) {
	client.calls++
	return mcpprotocol.ExtractLosslessToolResult(json.RawMessage(`{"content":[],"structuredContent":{"ok":true}}`)), nil
}

type blockingConnectionEpochClient struct {
	mu        sync.Mutex
	startOnce sync.Once
	started   chan struct{}
	release   chan struct{}
	calls     int
	closed    int
}

func (client *blockingConnectionEpochClient) ListTools() ([]ToolSpec, error) { return nil, nil }
func (client *blockingConnectionEpochClient) ListPrompts() ([]PromptSpec, error) {
	return nil, nil
}
func (client *blockingConnectionEpochClient) ListResources() ([]ResourceSpec, error) {
	return nil, nil
}
func (client *blockingConnectionEpochClient) Close() {
	client.mu.Lock()
	client.closed++
	client.mu.Unlock()
}
func (client *blockingConnectionEpochClient) CallTool(string, map[string]any) (any, error) {
	client.mu.Lock()
	client.calls++
	client.mu.Unlock()
	client.startOnce.Do(func() { close(client.started) })
	<-client.release
	return mcpprotocol.ExtractLosslessToolResult(json.RawMessage(`{"content":[],"structuredContent":{"ok":true}}`)), nil
}
func (client *blockingConnectionEpochClient) CallCount() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.calls
}

func TestSecurityBoundCallAndCatalogRefreshAreLinearized(t *testing.T) {
	now := time.Now().UTC()
	securityContext := newExecutableCaseSecurityContext(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-linearized", TurnID: "turn-linearized", WorkspaceRealPath: "/workspace", CaseID: "case-linearized",
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("binding-linearized")),
		DatasetSnapshotID:  domainsecurity.DatasetSnapshotIDPrefixV2 + domainsecurity.SHA256Hex([]byte("snapshot-linearized")),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-linearized")), ContextEpoch: 2, IssuedAt: now,
	})
	toolName := "mcp__docs__lookup"
	arguments := json.RawMessage(`{}`)
	identity := mcpTestVerifiedIdentity(t, "docs", "docs", "1.0.0", 1)
	tool := managedTool{ServerID: "docs", Name: toolName, RawName: "lookup", Tool: ToolSpec{
		InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`),
	}}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-linearized", ServerIdentity: identity,
		ToolName: toolName, ToolCallID: toolidentity.MustHostToolCallIDV1("mcp-linearized"), ConnectionEpoch: 1,
		ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: managedToolSchemaHash(toolName, tool),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope-linearized")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	envelope, err := domainmcp.NewHostContextEnvelope(securityContext, grant)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("call wins and refresh waits", func(t *testing.T) {
		client := &blockingConnectionEpochClient{started: make(chan struct{}), release: make(chan struct{})}
		manager := NewProductionManager(nil)
		manager.specs = []ServerSpec{{ID: "docs", ReadOnlyToolNames: map[string]bool{"lookup": true}}}
		manager.clients["docs"] = client
		manager.tools[toolName] = tool
		manager.connectionEpochs["docs"] = 1
		manager.serverIdentities["docs"] = identity
		callDone := make(chan map[string]any, 1)
		go func() {
			callDone <- manager.CallToolSecurityBoundContext(context.Background(), toolName, true, envelope, map[string]any{})
		}()
		select {
		case <-client.started:
		case <-time.After(2 * time.Second):
			t.Fatal("security-bound transport did not start")
		}
		refreshStarted := make(chan struct{})
		refreshDone := make(chan map[string]any, 1)
		go func() {
			close(refreshStarted)
			refreshDone <- manager.RefreshCatalog()
		}()
		<-refreshStarted
		select {
		case <-refreshDone:
			t.Fatal("catalog refresh overtook an in-flight security-bound effect")
		case <-time.After(50 * time.Millisecond):
		}
		close(client.release)
		select {
		case result := <-callDone:
			if result["executed"] != true || client.CallCount() != 1 {
				t.Fatalf("linearized call did not settle exactly once: result=%#v calls=%d", result, client.CallCount())
			}
		case <-time.After(2 * time.Second):
			t.Fatal("security-bound call did not settle")
		}
		select {
		case <-refreshDone:
		case <-time.After(2 * time.Second):
			t.Fatal("catalog refresh did not continue after the call released its lease")
		}
	})

	t.Run("catalog wins and old transport stays unused", func(t *testing.T) {
		client := &connectionEpochClient{}
		manager := NewProductionManager(nil)
		manager.specs = []ServerSpec{{ID: "docs", ReadOnlyToolNames: map[string]bool{"lookup": true}}}
		manager.clients["docs"] = client
		manager.tools[toolName] = tool
		manager.connectionEpochs["docs"] = 1
		manager.serverIdentities["docs"] = identity
		manager.sourceExecutionMu.Lock()
		callDone := make(chan map[string]any, 1)
		readOnly := true
		go func() {
			callDone <- manager.callToolContext(context.Background(), toolName, true, 1, grant.SchemaHash, identity, &readOnly, true, &securityContext, nil, map[string]any{})
		}()
		select {
		case result := <-callDone:
			manager.sourceExecutionMu.Unlock()
			t.Fatalf("security-bound call bypassed catalog lease: %#v", result)
		case <-time.After(50 * time.Millisecond):
		}
		manager.mu.Lock()
		manager.connectionEpochs["docs"] = 2
		manager.serverIdentities["docs"] = mcpTestVerifiedIdentity(t, "docs", "docs", "1.0.0", 2)
		manager.mu.Unlock()
		manager.sourceExecutionMu.Unlock()
		select {
		case result := <-callDone:
			if result["executed"] != false || client.calls != 0 {
				t.Fatalf("catalog-first ordering reached stale transport: result=%#v calls=%d", result, client.calls)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("catalog-first rejection did not settle")
		}
	})
}

func TestSecurityBoundCallRejectsReadOnlyPolicyChangeAtTransport(t *testing.T) {
	now := time.Now().UTC()
	securityContext := newExecutableCaseSecurityContext(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-policy", TurnID: "turn-policy", WorkspaceRealPath: "/workspace", CaseID: "case-policy",
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("binding-policy")),
		DatasetSnapshotID:  domainsecurity.DatasetSnapshotIDPrefixV2 + domainsecurity.SHA256Hex([]byte("snapshot-policy")),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-policy")), ContextEpoch: 3, IssuedAt: now,
	})
	toolName := "mcp__docs__lookup"
	arguments := json.RawMessage(`{}`)
	identity := mcpTestVerifiedIdentity(t, "docs", "docs", "1.0.0", 5)
	tool := managedTool{ServerID: "docs", Name: toolName, RawName: "lookup", Tool: ToolSpec{
		InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`),
	}}
	for _, test := range []struct {
		name            string
		grantedReadOnly bool
		currentReadOnly bool
	}{
		{name: "read-only authority removed", grantedReadOnly: true, currentReadOnly: false},
		{name: "write authority upgraded", grantedReadOnly: false, currentReadOnly: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			approvalState := "not_required"
			if !test.grantedReadOnly {
				approvalState = "approved"
			}
			grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
				Context: securityContext, Provider: "provider-policy", ServerIdentity: identity,
				ToolName: toolName, ToolCallID: toolidentity.MustHostToolCallIDV1("mcp-policy"), ConnectionEpoch: 5,
				ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: managedToolSchemaHash(toolName, tool),
				ScopeHash: domainsecurity.SHA256Hex([]byte("scope-policy")), ReadOnly: test.grantedReadOnly, ApprovalState: approvalState, IssuedAt: now,
			})
			envelope, err := domainmcp.NewHostContextEnvelope(securityContext, grant)
			if err != nil {
				t.Fatal(err)
			}
			client := &connectionEpochClient{}
			manager := NewProductionManager(nil)
			manager.specs = []ServerSpec{{ID: "docs", ReadOnlyToolNames: map[string]bool{"lookup": test.currentReadOnly}}}
			manager.clients["docs"] = client
			manager.tools[toolName] = tool
			manager.connectionEpochs["docs"] = 5
			manager.serverIdentities["docs"] = identity
			result := manager.CallToolSecurityBoundContext(context.Background(), toolName, true, envelope, map[string]any{})
			if result["code"] != "mcp_execution_grant_policy_mismatch" || result["executed"] != false || client.calls != 0 {
				t.Fatalf("host policy change reached transport: result=%#v calls=%d", result, client.calls)
			}
		})
	}
}

func TestTaskRequiredToolCannotEnterSynchronousAdmission(t *testing.T) {
	client := &connectionEpochClient{}
	manager := NewProductionManager(nil)
	manager.clients["docs"] = client
	manager.connectionEpochs["docs"] = 1
	closed := json.RawMessage(`{"type":"object","additionalProperties":false}`)
	required := ToolSpec{Name: "async_lookup", InputSchema: closed, OutputSchema: closed, TaskSupport: domainmcp.ToolTaskSupportRequired}
	optional := ToolSpec{Name: "sync_lookup", InputSchema: closed, OutputSchema: closed, TaskSupport: domainmcp.ToolTaskSupportOptional}
	if err := manager.registerToolsNoLock(ServerSpec{ID: "docs"}, []ToolSpec{required, optional}); err != nil {
		t.Fatal(err)
	}
	if _, ok := manager.tools["mcp__docs__async_lookup"]; ok {
		t.Fatal("task-required tool entered the synchronous advertised catalog")
	}
	if tool, ok := manager.tools["mcp__docs__sync_lookup"]; !ok || tool.Tool.TaskSupport != domainmcp.ToolTaskSupportOptional {
		t.Fatalf("optional synchronous tool was not retained: %#v", tool)
	}
	manager.tools["mcp__docs__async_lookup"] = managedTool{
		ServerID: "docs", Name: "mcp__docs__async_lookup", RawName: "async_lookup", Tool: required,
	}
	result := manager.CallTool("mcp__docs__async_lookup", true, map[string]any{})
	if result["executed"] != false || result["code"] != "mcp_task_required_unsupported" || client.calls != 0 {
		t.Fatalf("task-required tool reached synchronous transport: result=%#v calls=%d", result, client.calls)
	}
}

func TestProductionManagerBoundCallRejectsStaleConnectionEpoch(t *testing.T) {
	client := &connectionEpochClient{}
	toolName := "mcp__docs__lookup"
	manager := &ProductionManager{
		specs:   []ServerSpec{{ID: "docs", TrustScope: "user"}},
		clients: map[string]mcpTransportClient{"docs": client},
		tools: map[string]managedTool{toolName: {
			ServerID: "docs", Name: toolName, RawName: "lookup",
			Tool: ToolSpec{OutputSchema: json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`)},
		}},
		connectionEpochs: map[string]uint64{"docs": 2},
	}
	if epoch, ok := manager.ToolConnectionEpoch(toolName); !ok || epoch != 2 {
		t.Fatalf("current connection epoch missing: epoch=%d ok=%v", epoch, ok)
	}
	stale := manager.CallToolBoundContext(context.Background(), toolName, true, 1, map[string]any{})
	if stale["executed"] != false || stale["code"] != "mcp_connection_epoch_mismatch" || client.calls != 0 {
		t.Fatalf("stale epoch reached MCP execution: result=%#v calls=%d", stale, client.calls)
	}
	current := manager.CallToolBoundContext(context.Background(), toolName, true, 2, map[string]any{})
	if current["executed"] != true || current["semanticStatus"] != "success" || current["code"] != nil || current["isError"] != nil || client.calls != 1 {
		t.Fatalf("current epoch should execute once: result=%#v calls=%d", current, client.calls)
	}
}

func TestRejectAmbiguousCanonicalMCPToolNamespace(t *testing.T) {
	manager := NewProductionManager(nil)
	if err := manager.registerToolsNoLock(ServerSpec{ID: "foo"}, []ToolSpec{{Name: "bar__lookup"}}); err != nil {
		t.Fatalf("valid SEP-986 operation containing repeated underscores was rejected: %v", err)
	}
	if err := manager.registerToolsNoLock(ServerSpec{ID: "foo__bar"}, []ToolSpec{{Name: "lookup"}}); err == nil {
		t.Fatal("server identity containing the reserved separator was accepted")
	}
	if len(manager.tools) != 1 || manager.tools["mcp__foo__bar__lookup"].RawName != "bar__lookup" {
		t.Fatalf("exact remote operation binding was not preserved: %#v", manager.tools)
	}
}

func TestCanonicalBindingSetRejectsNormalizationAliases(t *testing.T) {
	raw := "lookup!"
	colliding := NormalizeName(raw)
	manager := NewProductionManager(nil)
	if err := manager.registerToolsNoLock(ServerSpec{ID: "docs"}, []ToolSpec{{Name: raw}, {Name: colliding}}); err == nil {
		t.Fatalf("normalized tool collision was accepted: %q and %q", raw, colliding)
	}
	if len(manager.tools) != 0 {
		t.Fatalf("part of a colliding catalog was registered: %#v", manager.tools)
	}
	serverRaw := "docs!"
	serverColliding := NormalizeName(serverRaw)
	failures := productionNamespaceFailures([]ServerSpec{{ID: serverRaw}, {ID: serverColliding}})
	if failures[serverRaw] == "" || failures[serverColliding] != "" {
		t.Fatalf("noncanonical server alias was not isolated from canonical identity: %#v", failures)
	}
}

func TestExecutionGrantServerIdentityMatchesDispatchedServer(t *testing.T) {
	now := time.Now().UTC()
	securityContext := newExecutableCaseSecurityContext(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-binding", TurnID: "turn-binding", WorkspaceRealPath: "/workspace", CaseID: "case-binding",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: domainsecurity.DatasetSnapshotIDPrefixV2 + domainsecurity.SHA256Hex([]byte("snapshot-binding")),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 2, IssuedAt: now,
	})
	toolName := "mcp__foo__lookup"
	arguments := json.RawMessage(`{}`)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-binding", ServerIdentity: mcpTestVerifiedIdentity(t, "foo", "foo", "1.0.0", 1),
		ToolName: toolName, ToolCallID: toolidentity.MustHostToolCallIDV1("mcp-binding"), ConnectionEpoch: 1,
		ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	envelope, err := domainmcp.NewHostContextEnvelope(securityContext, grant)
	if err != nil {
		t.Fatal(err)
	}
	wrongClient := &connectionEpochClient{}
	manager := &ProductionManager{
		specs:   []ServerSpec{{ID: "foo__shadow", TrustScope: "user"}},
		clients: map[string]mcpTransportClient{"foo__shadow": wrongClient},
		tools: map[string]managedTool{toolName: {
			ServerID: "foo__shadow", Name: toolName, RawName: "lookup",
			Tool: ToolSpec{OutputSchema: json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`)},
		}},
		connectionEpochs: map[string]uint64{"foo__shadow": 1},
		serverIdentities: map[string]string{"foo__shadow": grant.ServerIdentity},
	}
	result := manager.CallToolSecurityBoundContext(context.Background(), toolName, true, envelope, map[string]any{})
	if result["executed"] != false || result["code"] != "mcp_tool_binding_invalid" || wrongClient.calls != 0 {
		t.Fatalf("grant reached a mismatched server: result=%#v calls=%d", result, wrongClient.calls)
	}
}

func TestProductionManagerPendingExecutionGrantNeverReachesTransport(t *testing.T) {
	now := time.Now().UTC()
	securityContext := newExecutableCaseSecurityContext(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-pending", TurnID: "turn-pending", WorkspaceRealPath: "/workspace", CaseID: "case-pending",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: domainsecurity.DatasetSnapshotIDPrefixV2 + domainsecurity.SHA256Hex([]byte("snapshot-pending")),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 2, IssuedAt: now,
	})
	toolName := "mcp__docs__lookup"
	arguments := json.RawMessage(`{}`)
	identity := mcpTestVerifiedIdentity(t, "docs", "docs", "1.0.0", 1)
	tool := managedTool{ServerID: "docs", Name: toolName, RawName: "lookup", Tool: ToolSpec{
		InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`),
	}}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-pending", ServerIdentity: identity,
		ToolName: toolName, ToolCallID: toolidentity.MustHostToolCallIDV1("mcp-pending"), ConnectionEpoch: 1,
		ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: managedToolSchemaHash(toolName, tool),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "pending", IssuedAt: now,
	})
	envelope, err := domainmcp.NewHostContextEnvelope(securityContext, grant)
	if err != nil {
		t.Fatal(err)
	}
	client := &connectionEpochClient{}
	manager := &ProductionManager{
		clients: map[string]mcpTransportClient{"docs": client}, tools: map[string]managedTool{toolName: tool},
		connectionEpochs: map[string]uint64{"docs": 1}, serverIdentities: map[string]string{"docs": identity},
	}
	result := manager.CallToolSecurityBoundContext(context.Background(), toolName, true, envelope, map[string]any{})
	if result["executed"] != false || result["code"] != "mcp_execution_approval_pending" || client.calls != 0 {
		t.Fatalf("pending grant reached MCP transport: result=%#v calls=%d", result, client.calls)
	}
}

func TestSecurityBoundMCPRejectsAuditOnlyTurnContextBeforeTransport(t *testing.T) {
	now := time.Now().UTC()
	auditContext := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-audit-only", TurnID: "turn-audit-only", WorkspaceRealPath: "/workspace",
		ContextEpoch: 2, IssuedAt: now,
	})
	toolName := "mcp__docs__lookup"
	arguments := json.RawMessage(`{}`)
	identity := mcpTestVerifiedIdentity(t, "docs", "docs", "1.0.0", 1)
	tool := managedTool{ServerID: "docs", Name: toolName, RawName: "lookup", Tool: ToolSpec{
		InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`),
	}}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: auditContext, Provider: "provider-audit-only", ServerIdentity: identity,
		ToolName: toolName, ToolCallID: toolidentity.MustHostToolCallIDV1("mcp-audit-only"), ConnectionEpoch: 1,
		ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: managedToolSchemaHash(toolName, tool),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope-audit-only")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	if _, err := domainmcp.NewHostContextEnvelope(auditContext, grant); err == nil {
		t.Fatal("audit-only V1 context produced a live MCP host envelope")
	}
	client := &connectionEpochClient{}
	manager := &ProductionManager{
		clients: map[string]mcpTransportClient{"docs": client}, tools: map[string]managedTool{toolName: tool},
		connectionEpochs: map[string]uint64{"docs": 1}, serverIdentities: map[string]string{"docs": identity},
	}
	result := manager.CallToolSecurityBoundContext(context.Background(), toolName, true, domainmcp.HostContextEnvelope{}, map[string]any{})
	if result["executed"] != false || result["code"] != "mcp_host_context_invalid" || client.calls != 0 {
		t.Fatalf("audit-only turn context reached MCP transport: result=%#v calls=%d", result, client.calls)
	}
}

func TestExecutionGrantRejectsCurrentSchemaChangeSameConnectionEpoch(t *testing.T) {
	now := time.Now().UTC()
	securityContext := newExecutableCaseSecurityContext(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-schema", TurnID: "turn-schema", WorkspaceRealPath: "/workspace", CaseID: "case-schema",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: domainsecurity.DatasetSnapshotIDPrefixV2 + domainsecurity.SHA256Hex([]byte("snapshot-schema")),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 2, IssuedAt: now,
	})
	toolName := "mcp__docs__lookup"
	baseTool := managedTool{ServerID: "docs", Name: toolName, RawName: "lookup", Tool: ToolSpec{
		Description:  "Lookup docs",
		InputSchema:  json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"],"additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`),
	}}
	arguments := json.RawMessage(`{"q":"x"}`)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-schema", ServerIdentity: mcpTestVerifiedIdentity(t, "docs", "docs", "1.0.0", 1),
		ToolName: toolName, ToolCallID: toolidentity.MustHostToolCallIDV1("mcp-schema"), ConnectionEpoch: 1, ArgsHash: domainsecurity.CanonicalJSONHash(arguments),
		SchemaHash: managedToolSchemaHash(toolName, baseTool), ScopeHash: domainsecurity.SHA256Hex([]byte("scope")),
		ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	envelope, err := domainmcp.NewHostContextEnvelope(securityContext, grant)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*managedTool){
		"input": func(tool *managedTool) {
			tool.Tool.InputSchema = json.RawMessage(`{"type":"object","properties":{"q":{"type":"integer"}},"required":["q"],"additionalProperties":false}`)
		},
		"output": func(tool *managedTool) {
			tool.Tool.OutputSchema = json.RawMessage(`{"type":"object","properties":{"ok":{"type":"string"}},"required":["ok"],"additionalProperties":false}`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := baseTool
			mutate(&changed)
			client := &connectionEpochClient{}
			manager := &ProductionManager{
				specs:   []ServerSpec{{ID: "docs", TrustScope: "user"}},
				clients: map[string]mcpTransportClient{"docs": client}, tools: map[string]managedTool{toolName: changed},
				connectionEpochs: map[string]uint64{"docs": 1},
				serverIdentities: map[string]string{"docs": grant.ServerIdentity},
			}
			result := manager.CallToolSecurityBoundContext(context.Background(), toolName, true, envelope, map[string]any{"q": "x"})
			if result["code"] != "mcp_execution_grant_schema_mismatch" || result["executed"] != false || client.calls != 0 {
				t.Fatalf("changed %s schema reached transport: result=%#v calls=%d", name, result, client.calls)
			}
		})
	}
}

func TestExtractMCPToolResultPreservesStructuredContentWithText(t *testing.T) {
	raw := json.RawMessage(`{"content":[{"type":"text","text":"artifact ready"}],"structuredContent":{"delivery_artifact":{"report_path":"/Users/sun/report.md","inspection_status":"passed"}}}`)
	result := extractMCPToolResult(raw)
	record, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected structured MCP result map, got %#v", result)
	}
	if record["text"] != "artifact ready" {
		t.Fatalf("MCP text result was not preserved: %#v", record)
	}
	structured, _ := record["structuredContent"].(map[string]any)
	artifact, _ := structured["delivery_artifact"].(map[string]any)
	if artifact["report_path"] != "/Users/sun/report.md" || artifact["inspection_status"] != "passed" {
		t.Fatalf("structuredContent artifact was not preserved: %#v", record)
	}
}

func TestMCPExecutedToolResultMarksSemanticFailure(t *testing.T) {
	raw := json.RawMessage(`{"content":[{"type":"text","text":"正式报告未发布"}],"isError":true,"safeToAnswer":false,"semanticStatus":"blocked","blocker":{"code":"PUBLICATION_RECEIPT_REQUIRED"},"partialCoverage":{"coverageStatus":"partial"},"evidenceReceipts":[{"receiptId":"remote-candidate"}],"_meta":{"analytix_evidence_ledger":{"status":"unsupported"},"analytix_tool_outcome":{"reportedSemanticStatus":"success","caseId":"case-remote","contextEpoch":99,"datasetSnapshotId":"snapshot-remote","serverIdentity":"server-remote"}},"structuredContent":{"safeToAnswer":false,"isError":true}}`)
	schema := json.RawMessage(`{"type":"object","properties":{"safeToAnswer":{"type":"boolean"},"isError":{"type":"boolean"}},"required":["safeToAnswer","isError"],"additionalProperties":false}`)
	result := mcpExecutedToolResult("mcp__analytix_funds__run_full_case_analysis", "analytix_funds", mcpprotocol.ExtractLosslessToolResult(raw), schema, false, false)
	if result["executed"] != true || result["isError"] != true || result["code"] != "mcp_semantic_failure" {
		t.Fatalf("transport execution must preserve semantic failure: %#v", result)
	}
	nested, _ := result["result"].(map[string]any)
	if nested["isError"] != nil || nested["_meta"] != nil || nested["safeToAnswer"] != nil {
		t.Fatalf("remote safety metadata entered provider projection: %#v", result)
	}
	lossless, ok := result[domainmcp.HostRawToolResultKey].(domainmcp.LosslessToolResult)
	if !ok || !domainmcp.ValidToolResultObservation(lossless) || lossless.Observation.ReportedSafeToAnswer == nil ||
		*lossless.Observation.ReportedSafeToAnswer || lossless.Observation.ReportedCaseID != "case-remote" ||
		lossless.Observation.ReportedContextEpoch != 99 || len(lossless.Observation.CandidateEvidenceReceipts) != 1 {
		t.Fatalf("manager lost exact host-private MCP assertions: %#v", result)
	}
	serialized, err := json.Marshal(lossless)
	if err != nil || string(serialized) != "{}" {
		t.Fatalf("host-private MCP observation became serializable: body=%s err=%v", serialized, err)
	}
}

func TestMCPToolErrorWithoutStructuredContentIsSemanticFailure(t *testing.T) {
	raw := json.RawMessage(`{"content":[{"type":"text","text":"source rejected the query"}],"isError":true}`)
	schema := json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`)
	result := mcpExecutedToolResult("mcp__docs__lookup", "docs", mcpprotocol.ExtractLosslessToolResult(raw), schema, true, false)
	if result["executed"] != true || result["transportStatus"] != "success" || result["semanticStatus"] != "failure" ||
		result["isError"] != true || result["code"] != "mcp_semantic_failure" {
		t.Fatalf("standard tool error was misclassified as a transport or output-schema failure: %#v", result)
	}
}

func TestMCPExecutedToolResultPreservesPartialAsNonTransportError(t *testing.T) {
	raw := json.RawMessage(`{"content":[],"semanticStatus":"partial","partialCoverage":{"paginationComplete":false},"structuredContent":{"semanticStatus":"partial","partialCoverage":{"paginationComplete":false},"data":{"rows":[]}}}`)
	schema := json.RawMessage(`{"type":"object","properties":{"semanticStatus":{"type":"string","enum":["partial"]},"partialCoverage":{"type":"object","properties":{"paginationComplete":{"type":"boolean"}},"required":["paginationComplete"],"additionalProperties":false},"data":{"type":"object","properties":{"rows":{"type":"array","items":{"type":"object","additionalProperties":false}}},"required":["rows"],"additionalProperties":false}},"required":["semanticStatus","partialCoverage","data"],"additionalProperties":false}`)
	result := mcpExecutedToolResult("mcp__analytix_funds__query", "analytix_funds", mcpprotocol.ExtractLosslessToolResult(raw), schema, true, false)
	if result["executed"] != true || result["isError"] != nil || result["code"] != nil {
		t.Fatalf("partial semantic coverage was collapsed into an MCP transport/tool error: %#v", result)
	}
}

func TestInvalidOutputSchemaRetainsQuarantinedRawHashWithoutEvidenceAuthority(t *testing.T) {
	raw := json.RawMessage(`{"content":[],"structuredContent":{"amount":"100"}}`)
	schema := json.RawMessage(`{"type":"object","properties":{"amount":{"type":"integer"}},"required":["amount"],"additionalProperties":false}`)
	result := mcpExecutedToolResult("mcp__funds__query", "funds", mcpprotocol.ExtractLosslessToolResult(raw), schema, true, false)
	if result["executed"] != true || result["transportStatus"] != "success" || result["isError"] != true || result["code"] != "mcp_output_schema_invalid" {
		t.Fatalf("output mismatch classification is unsafe: %#v", result)
	}
	lossless, found := result[domainmcp.HostRawToolResultKey].(domainmcp.LosslessToolResult)
	if !found || !domainmcp.ValidToolResultObservation(lossless) || !lossless.Observation.IsError ||
		lossless.Observation.SemanticStatus != "failure" || lossless.Observation.Blocker != "mcp_output_schema_invalid" {
		t.Fatalf("invalid raw result was not retained as a host-private negative observation: %#v", result)
	}
	if body, err := json.Marshal(lossless); err != nil || string(body) != "{}" {
		t.Fatalf("rejected raw carrier became serializable: body=%s err=%v", body, err)
	}
}

func TestRejectDivergentLosslessRawAndValue(t *testing.T) {
	raw := json.RawMessage(`{"content":[],"structuredContent":{"ok":true}}`)
	forged := domainmcp.LosslessToolResult{
		RawResult: raw, RawSHA256: domainsecurity.SHA256Hex(raw),
		Value: map[string]any{"structuredContent": map[string]any{"ok": false, "account": "forged"}},
	}
	schema := json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`)
	result := mcpExecutedToolResult("mcp__docs__lookup", "docs", forged, schema, true, false)
	if result["code"] != "mcp_output_schema_invalid" || result["isError"] != true {
		t.Fatalf("divergent lossless projection was accepted: %#v", result)
	}
	lossless, found := result[domainmcp.HostRawToolResultKey].(domainmcp.LosslessToolResult)
	if !found || !domainmcp.ValidToolResultObservation(lossless) || !lossless.Observation.IsError || lossless.Observation.SemanticStatus != "failure" {
		t.Fatalf("divergent result was not retained only as a negative raw observation: %#v", result)
	}
}

func TestExtractMCPToolResultPreservesImageContentWithText(t *testing.T) {
	raw := json.RawMessage(`{"content":[{"type":"text","text":"state ready"},{"type":"image","mimeType":"image/png","data":"cG5n"}]}`)
	result := extractMCPToolResult(raw)
	record, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected structured MCP result map, got %#v", result)
	}
	if record["text"] != "state ready" {
		t.Fatalf("MCP text result was not preserved: %#v", record)
	}
	images, _ := record["images"].([]any)
	if len(images) != 1 {
		t.Fatalf("MCP image result was not preserved: %#v", record)
	}
	image, _ := images[0].(map[string]any)
	if image["mime_type"] != "image/png" || image["data_base64"] != "cG5n" {
		t.Fatalf("MCP image result was not normalized for runtime image extraction: %#v", record)
	}
	if _, ok := record["content"]; ok {
		t.Fatalf("image content should not be duplicated outside images: %#v", record)
	}
}

func TestProductionManagerCanonicalizesMCPToolSchemaRequiredFields(t *testing.T) {
	got := canonicalJSONBytes([]byte(`{
		"type":"object",
		"required":["b","a","a"],
		"dependentRequired":{"z":["q","p","p"],"empty":[]},
		"properties":{
			"n":{"type":"string"},
			"child":{"type":"object","required":["y","x","x"]}
		}
	}`))
	want := `{"dependentRequired":{"z":["p","q"]},"properties":{"child":{"required":["x","y"],"type":"object"},"n":{"type":"string"}},"required":["a","b"],"type":"object"}`
	if got != want {
		t.Fatalf("canonical MCP schema = %s, want %s", got, want)
	}
	malformed := canonicalJSONBytes([]byte(`{"type":"object","required":["a",3],"properties":{"a":{"type":"string"}}}`))
	valid := canonicalJSONBytes([]byte(`{"type":"object","required":["a"],"properties":{"a":{"type":"string"}}}`))
	if malformed == valid {
		t.Fatal("malformed required members collapsed into a valid MCP schema")
	}
	if got := canonicalJSONBytes(nil); got != `{"type":"object","properties":{},"additionalProperties":false}` {
		t.Fatalf("empty MCP schema = %s, want object schema", got)
	}
}

func TestProductionManagerStdioListAndCall(t *testing.T) {
	if os.Getenv("ANALYTIX_MCP_STDIO_HELPER") == "1" {
		runStdioMCPHelper()
		return
	}
	manager := NewProductionManager([]ServerSpec{{
		ID:        "stdio-server",
		Transport: "stdio",
		Command:   os.Args[0],
		Args:      []string{"-test.run=TestProductionManagerStdioListAndCall"},
		Env:       map[string]string{"ANALYTIX_MCP_STDIO_HELPER": "1"},
	}})
	manager.Connect()
	defer manager.Disconnect()

	toolName := CanonicalToolName("stdio-server", "echo")
	if tools := manager.Tools(); len(tools) != 1 || tools[0] != toolName {
		t.Fatalf("unexpected stdio MCP tools: %#v", tools)
	}
	called := manager.CallTool(toolName, true, map[string]any{"text": "hello"})
	if !mcpBoolField(called, "executed") || mcpResultText(called) != "stdio result: hello" {
		t.Fatalf("unexpected stdio MCP call result: %#v", called)
	}
}

func TestProductionManagerStdioReadsLargeJSONRPCLine(t *testing.T) {
	if os.Getenv("ANALYTIX_MCP_STDIO_HELPER") == "1" {
		runStdioMCPHelper()
		return
	}
	manager := NewProductionManager([]ServerSpec{{
		ID:        "large-stdio-server",
		Transport: "stdio",
		Command:   os.Args[0],
		Args:      []string{"-test.run=TestProductionManagerStdioReadsLargeJSONRPCLine"},
		Env: map[string]string{
			"ANALYTIX_MCP_STDIO_HELPER":            "1",
			"ANALYTIX_MCP_STDIO_LARGE_DESCRIPTION": "1",
		},
	}})
	manager.Connect()
	defer manager.Disconnect()

	toolName := CanonicalToolName("large-stdio-server", "echo")
	if tools := manager.Tools(); len(tools) != 1 || tools[0] != toolName {
		t.Fatalf("large stdio MCP response should still advertise the tool: %#v", tools)
	}
	if diagnostics := manager.ServerDiagnostics(); len(diagnostics) != 1 ||
		!mcpBoolField(diagnostics[0].(map[string]any), "available") {
		t.Fatalf("large stdio MCP response should keep server available: %#v", diagnostics)
	}
}

func TestProductionManagerStdioFailureIncludesRedactedStderrTail(t *testing.T) {
	if os.Getenv("ANALYTIX_MCP_STDIO_FAIL_HELPER") == "1" {
		runFailingStdioMCPHelper()
		return
	}
	const secret = "stdio-contract-secret-token"
	manager := NewProductionManager([]ServerSpec{{
		ID:        "failing-stdio-server",
		Transport: "stdio",
		Command:   os.Args[0],
		Args:      []string{"-test.run=TestProductionManagerStdioFailureIncludesRedactedStderrTail"},
		Env: map[string]string{
			"ANALYTIX_MCP_STDIO_FAIL_HELPER": "1",
			"MCP_API_KEY":                    secret,
		},
	}})
	manager.Connect()
	defer manager.Disconnect()

	if tools := manager.Tools(); len(tools) != 0 {
		t.Fatalf("failing stdio MCP server must not advertise tools: %#v", tools)
	}
	serverDiagnostics := manager.ServerDiagnostics()
	diagnosticsJSON := mcpMustJSONText(t, serverDiagnostics)
	if strings.Contains(diagnosticsJSON, secret) {
		t.Fatalf("stdio MCP diagnostics leaked credential: %s", diagnosticsJSON)
	}
	if len(serverDiagnostics) != 1 {
		t.Fatalf("expected one stdio diagnostic row: %#v", serverDiagnostics)
	}
	row, _ := serverDiagnostics[0].(map[string]any)
	failure := mcpStringField(row, "failure")
	if !strings.Contains(failure, "stderr tail") ||
		!strings.Contains(failure, "startup failed") ||
		!strings.Contains(failure, "<redacted>") {
		t.Fatalf("stdio MCP failure should include redacted stderr tail: %#v", row)
	}
	if mcpBoolField(row, "available") || mcpBoolField(row, "connected") {
		t.Fatalf("failing stdio MCP server should remain unavailable: %#v", row)
	}
}

func runStdioMCPHelper() {
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			continue
		}
		switch request.Method {
		case "notifications/initialized":
		case "initialize":
			_ = encoder.Encode(jsonRPCResult(request.ID, map[string]any{
				"protocolVersion": "2025-11-25",
				"serverInfo":      map[string]any{"name": "stdio-test", "version": "1.0.0"},
				"capabilities":    map[string]any{"tools": map[string]any{}},
			}))
		case "tools/list":
			description := "Echo text"
			if os.Getenv("ANALYTIX_MCP_STDIO_LARGE_DESCRIPTION") == "1" {
				description = strings.Repeat("large catalog description ", 5000)
			}
			_ = encoder.Encode(jsonRPCResult(request.ID, map[string]any{"tools": []map[string]any{{
				"name":        "echo",
				"description": description,
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}, "additionalProperties": false},
			}}}))
		case "tools/call":
			args, _ := request.Params["arguments"].(map[string]any)
			_ = encoder.Encode(jsonRPCResult(request.ID, map[string]any{"content": []map[string]any{{
				"type": "text",
				"text": "stdio result: " + fmt.Sprint(args["text"]),
			}}}))
		default:
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}
	os.Exit(0)
}

func runFailingStdioMCPHelper() {
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			continue
		}
		switch request.Method {
		case "notifications/initialized":
		case "initialize":
			_ = encoder.Encode(jsonRPCResult(request.ID, map[string]any{
				"protocolVersion": "2025-11-25",
				"serverInfo":      map[string]any{"name": "stdio-failing-test", "version": "1.0.0"},
				"capabilities":    map[string]any{"tools": map[string]any{}},
			}))
		case "tools/list":
			fmt.Fprintln(os.Stderr, "startup failed: token "+os.Getenv("MCP_API_KEY")+" cannot access catalog")
			os.Exit(42)
		default:
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}
	os.Exit(0)
}

func jsonRPCResult(id int, result any) map[string]any {
	if record, ok := result.(map[string]any); ok {
		if serverInfo, ok := record["serverInfo"].(map[string]any); ok {
			record["protocolVersion"] = mcpprotocol.ProtocolVersion
			if _, exists := serverInfo["version"]; !exists {
				serverInfo["version"] = "1.0.0"
			}
			capabilities, _ := record["capabilities"].(map[string]any)
			if capabilities == nil {
				capabilities = map[string]any{}
			}
			if _, exists := capabilities["tools"]; !exists {
				capabilities["tools"] = map[string]any{}
			}
			record["capabilities"] = capabilities
		}
		if tools, ok := record["tools"].([]map[string]any); ok {
			for _, tool := range tools {
				if _, exists := tool["outputSchema"]; !exists {
					tool["outputSchema"] = map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
				}
			}
		}
		if _, hasContent := record["content"]; hasContent {
			if _, hasStructured := record["structuredContent"]; !hasStructured {
				record["structuredContent"] = map[string]any{}
			}
		}
	}
	return map[string]any{"jsonrpc": "2.0", "id": id, "result": result}
}

func mcpBoolField(record map[string]any, key string) bool {
	value, _ := record[key].(bool)
	return value
}

func mcpStringField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}

func mcpResultText(record map[string]any) string {
	if text, ok := record["result"].(string); ok {
		return text
	}
	result, _ := record["result"].(map[string]any)
	text, _ := result["text"].(string)
	return text
}

func stringSliceFromAny(value any) []string {
	items, _ := value.([]string)
	if items != nil {
		return items
	}
	rawItems, _ := value.([]any)
	out := make([]string, 0, len(rawItems))
	for _, item := range rawItems {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

func mcpMustJSONText(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return string(data)
}

func mcpServerDiagnosticByID(t *testing.T, diagnostics []any, id string) map[string]any {
	t.Helper()
	for _, item := range diagnostics {
		record, _ := item.(map[string]any)
		if mcpStringField(record, "id") == id {
			return record
		}
	}
	t.Fatalf("missing MCP server diagnostic %q in %#v", id, diagnostics)
	return nil
}

func waitForMCPPromptResourceCatalog(t *testing.T, manager *ProductionManager, promptCount int, resourceCount int) ([]any, []any) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		prompts := manager.Prompts()
		resources := manager.Resources()
		if len(prompts) == promptCount && len(resources) == resourceCount {
			return prompts, resources
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for MCP prompt/resource catalog: prompts=%#v resources=%#v", prompts, resources)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func containsMCPTool(items []string, expected string) bool {
	for _, item := range items {
		if item == expected {
			return true
		}
	}
	return false
}

func mcpTestVerifiedIdentity(t *testing.T, serverID, observedName, observedVersion string, epoch uint64) string {
	t.Helper()
	identity, err := domainsecurity.NewVerifiedMCPServerIdentity(serverID, observedName, observedVersion, domainsecurity.SHA256Hex([]byte("mcp-test-runtime-instance")), epoch)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}
