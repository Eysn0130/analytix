package runtimeinfo

import (
	"encoding/json"
	"strings"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

const privateDiagnosticSentinel = "PRIVATE reasoning account=6222021234567890 /private/case.csv SELECT * FROM secret MCP_PAYLOAD"

func TestRuntimeInfoBuildsClosedPublicV2(t *testing.T) {
	capabilities := RuntimeCapabilities(RuntimeCapabilitiesInput{})
	computerUse := publicMap(capabilities["computerUse"])
	computerUse["reason"] = privateDiagnosticSentinel
	info := RuntimeInfo(RuntimeInfoInput{
		StartedAt:      "2026-07-03T08:00:00Z",
		Host:           "127.0.0.1",
		Port:           4242,
		DataDir:        "/tmp/analytix-runtime/" + privateDiagnosticSentinel,
		Insecure:       true,
		ApprovalPolicy: "on-request",
		SandboxMode:    "workspace-write",
		NetworkProxy: map[string]any{
			"mode": "custom", "configured": true, "source": "settings.provider.proxy", "valid": false,
			"credentialsMasked": true, "summary": privateDiagnosticSentinel, "error": privateDiagnosticSentinel,
		},
		DefaultProvider: domainmodel.TurnConfig{
			ProviderID: "deepseek", Model: "deepseek-chat", Family: "deepseek", EndpointFormat: "chat_completions",
			APIKey: "test-key", BaseURL: "https://api.deepseek.com", ContextWindowTokens: 128000,
		},
		Capabilities: capabilities,
	})
	if info.SchemaVersion != PublicRuntimeDiagnosticsSchemaVersion || info.Status != "ready" || info.ListenerScope != "loopback" || info.Port != 4242 {
		t.Fatalf("runtime info identity mismatch: %#v", info)
	}
	if !info.Storage.Configured || !info.Storage.Available || info.Provider.ID != "deepseek" || info.Provider.Model != "deepseek-chat" || !info.Provider.Available {
		t.Fatalf("runtime info closed state mismatch: %#v", info)
	}
	if info.Capabilities.ContractVersion != 1 || info.Capabilities.ComputerUse.ReasonCode == "" {
		t.Fatalf("runtime capabilities projection mismatch: %#v", info.Capabilities)
	}
	assertPublicDiagnosticsExclude(t, info, privateDiagnosticSentinel, "/tmp/analytix-runtime", "test-key", "api.deepseek.com", "dataDir", "summary", "error")
}

func TestUnconfiguredMCPIsDisabledRatherThanFactReady(t *testing.T) {
	capabilities := ProjectPublicRuntimeCapabilities(RuntimeCapabilities(RuntimeCapabilitiesInput{}))
	if capabilities.MCP.Status != "disabled" || capabilities.MCP.Enabled || capabilities.MCP.Available {
		t.Fatalf("unconfigured MCP must remain a disabled source: %#v", capabilities.MCP)
	}
	if capabilities.MCP.ReasonCode != "disabled_by_config" {
		t.Fatalf("unconfigured MCP must preserve its deterministic blocker: %#v", capabilities.MCP)
	}
}

func TestPublicMCPSearchFailsClosedWhenDisabledSourceIsLive(t *testing.T) {
	search := ProjectPublicMCPSearch(map[string]any{
		"enabled":             false,
		"mode":                "auto",
		"active":              true,
		"available":           true,
		"indexedToolCount":    float64(5),
		"advertisedToolCount": float64(4),
	})
	if search.Enabled || search.Available || search.Active || search.ReasonCode != "disabled_by_config" {
		t.Fatalf("disabled MCP search public projection must fail closed: %#v", search)
	}
	if search.IndexedToolCount != 5 || search.AdvertisedToolCount != 4 {
		t.Fatalf("public MCP search projection should retain non-authorizing catalog counts: %#v", search)
	}
}

func TestRuntimeToolsDiagnosticsIsRecursiveClosedAllowlist(t *testing.T) {
	tools := RuntimeToolsDiagnostics(RuntimeToolsDiagnosticsInput{
		Providers: []any{map[string]any{"id": privateDiagnosticSentinel, "apiKey": privateDiagnosticSentinel}},
		ToolContracts: []any{map[string]any{
			"name": "mcp__funds__query", "description": privateDiagnosticSentinel,
			"inputSchema":  map[string]any{"type": "object", "description": privateDiagnosticSentinel},
			"outputSchema": map[string]any{"type": "object"}, "toolKind": "tool_call", "providerKind": "mcp", "toolPolicy": "auto",
		}},
		MCPServers: []any{map[string]any{
			"id": "funds", "transport": "stdio", "authStatus": "required", "trustScope": "workspace",
			"enabled": true, "available": false, "connected": false, "schemaHintAvailable": true, "connectable": true,
			"toolCount": float64(3), "promptCount": float64(2), "resourceCount": float64(1), "sourceProbeCount": float64(1), "failure": privateDiagnosticSentinel,
			"cwd": privateDiagnosticSentinel, "authUrl": privateDiagnosticSentinel, "sourceCaseId": privateDiagnosticSentinel,
			"datasetSnapshotId": privateDiagnosticSentinel, "sourceReady": true, "sourceProbeDigest": privateDiagnosticSentinel,
			"rowCount": privateDiagnosticSentinel, "projectionDigest": privateDiagnosticSentinel,
			"rawResultSHA256": privateDiagnosticSentinel, "grantDigest": privateDiagnosticSentinel,
			"authorityDigest": privateDiagnosticSentinel, "connectionEpoch": float64(9),
			"sourcePath": privateDiagnosticSentinel, "sourceValue": privateDiagnosticSentinel,
			"toolNames": []any{privateDiagnosticSentinel},
		}},
		MCPSearch: map[string]any{
			"enabled": true, "mode": "auto", "active": false, "available": false,
			"indexedToolCount": float64(3), "advertisedToolCount": float64(0), "lastError": privateDiagnosticSentinel,
		},
		MCPPrompts:   []any{map[string]any{"description": privateDiagnosticSentinel}},
		MCPResources: []any{map[string]any{"uri": privateDiagnosticSentinel}},
		Commands: []any{
			map[string]any{"binary": "go", "found": true, "output": privateDiagnosticSentinel},
			map[string]any{"binary": privateDiagnosticSentinel, "found": true, "output": privateDiagnosticSentinel},
		},
		NetworkProxy: map[string]any{"mode": "custom", "configured": true, "source": "settings.provider.proxy", "valid": false, "credentialsMasked": true, "summary": privateDiagnosticSentinel},
		WebProviders: []any{map[string]any{"reason": privateDiagnosticSentinel}},
		Skills: map[string]any{
			"enabled": true, "available": true,
			"roots":            []any{map[string]any{"path": privateDiagnosticSentinel}},
			"skills":           []any{map[string]any{"description": privateDiagnosticSentinel}},
			"validationErrors": []any{map[string]any{"error": privateDiagnosticSentinel}},
		},
		Attachments: map[string]any{
			"enabled": true, "count": float64(2), "totalBytes": float64(10), "rootDir": privateDiagnosticSentinel,
		},
		Memory: map[string]any{"enabled": true, "activeCount": float64(1), "tombstoneCount": float64(0), "rootDir": privateDiagnosticSentinel, "lastInjectedIds": []any{privateDiagnosticSentinel}},
		Subagents: map[string]any{
			"status": "available", "enabled": true, "available": true, "active": float64(1), "queued": float64(2),
			"profiles": []any{map[string]any{"promptPreamble": privateDiagnosticSentinel}}, "childRuns": []any{map[string]any{"output": privateDiagnosticSentinel}},
		},
	})
	if tools.SchemaVersion != PublicRuntimeDiagnosticsSchemaVersion || tools.ProviderCount != 1 || tools.ToolContracts.Count != 1 || len(tools.ToolContracts.CatalogHash) != 64 {
		t.Fatalf("runtime tools summary mismatch: %#v", tools)
	}
	if len(tools.MCPServers) != 1 || tools.MCPServers[0].Status != "error" ||
		tools.MCPServers[0].FailureCode != "connection_failed" || tools.MCPServers[0].SourceProbeCount != 1 {
		t.Fatalf("MCP public diagnostics mismatch: %#v", tools.MCPServers)
	}
	if tools.MCPPromptCount != 1 || tools.MCPResourceCount != 1 || len(tools.Commands) != 1 || tools.Commands[0].Binary != "go" {
		t.Fatalf("public count/command diagnostics mismatch: %#v", tools)
	}
	if tools.Skills.SkillCount != 1 || tools.Attachments.Count != 2 || tools.Memory.ActiveCount == nil || *tools.Memory.ActiveCount != 1 || tools.Subagents.ProfileCount != 1 {
		t.Fatalf("public diagnostics stats mismatch: %#v", tools)
	}
	assertPublicDiagnosticsExclude(t, tools, privateDiagnosticSentinel, "description", "inputSchema", "outputSchema", "cwd", "authUrl",
		"sourceCaseId", "datasetSnapshotId", "sourceReady", "sourceProbeDigest", "rowCount", "projectionDigest",
		"rawResultSHA256", "grantDigest", "authorityDigest", "connectionEpoch", "sourcePath", "sourceValue",
		"rootDir", "childRuns", "lastError")
}

func TestPublicMCPSourceProbeCountUsesExistingBoundedCountProjection(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  int
	}{
		{name: "valid", value: float64(1), want: 1},
		{name: "malformed", value: "1", want: 0},
		{name: "negative", value: float64(-1), want: 0},
		{name: "fractional", value: float64(7.75), want: 7},
		{name: "over limit", value: float64(1_000_000_001), want: 1_000_000_000},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			servers := ProjectPublicMCPServers([]any{map[string]any{
				"id": "analytix_funds", "transport": "stdio", "sourceProbeCount": test.value,
				"sourceReady": true, "sourceProbeDigest": privateDiagnosticSentinel,
				"sourceCaseId": privateDiagnosticSentinel, "datasetSnapshotId": privateDiagnosticSentinel,
				"rowCount": privateDiagnosticSentinel, "grantDigest": privateDiagnosticSentinel,
			}})
			if len(servers) != 1 || servers[0].SourceProbeCount != test.want {
				t.Fatalf("source probe count projection mismatch: got=%#v want=%d", servers, test.want)
			}
			assertPublicDiagnosticsExclude(t, servers, privateDiagnosticSentinel, "sourceReady", "sourceProbeDigest",
				"sourceCaseId", "datasetSnapshotId", "rowCount", "grantDigest")
		})
	}
}

func TestPublicRuntimeDiagnosticsNormalizeTransportPortAndUnsupportedReasoning(t *testing.T) {
	info := RuntimeInfo(RuntimeInfoInput{
		StartedAt: "2026-07-03T08:00:00Z", Host: "127.0.0.1", Port: 100000, DataDir: t.TempDir(),
		Capabilities: map[string]any{
			"contractVersion": float64(1),
			"model": map[string]any{
				"id": "claude", "inputModalities": []any{"text"}, "outputModalities": []any{"text"},
				"messageParts": []any{"text"}, "reasoning": map[string]any{
					"supportedEfforts": []any{}, "defaultEffort": "", "requestProtocol": "none",
				},
			},
		},
	})
	if info.Port != 0 || info.Capabilities.Model.Reasoning != nil {
		t.Fatalf("invalid port or unsupported reasoning escaped public projection: %#v", info)
	}
	tools := RuntimeToolsDiagnostics(RuntimeToolsDiagnosticsInput{MCPServers: []any{map[string]any{"id": "remote", "transport": "http"}}})
	if len(tools.MCPServers) != 1 || tools.MCPServers[0].Transport != "streamable-http" {
		t.Fatalf("http MCP transport was not normalized: %#v", tools.MCPServers)
	}
}

func TestPublicMemoryDiagnosticsPreservesUnknownCounts(t *testing.T) {
	diagnostics := ProjectPublicMemory(map[string]any{
		"enabled": true, "status": "unavailable", "reasonCode": "memory_store_read_failed",
	}, true)
	if diagnostics.Status != "unavailable" || diagnostics.ReasonCode != "memory_store_read_failed" ||
		diagnostics.ActiveCount != nil || diagnostics.TombstoneCount != nil {
		t.Fatalf("unknown memory counts collapsed into facts: %#v", diagnostics)
	}
}

func TestPublicRuntimeIdentifiersRejectPathsEmailsAndAccountLikeNumbers(t *testing.T) {
	for _, raw := range []string{
		"/Users/private/mcp",
		"C:/private/mcp",
		"owner@example.invalid",
		"server-6222021234567890",
	} {
		servers := ProjectPublicMCPServers([]any{map[string]any{"id": raw, "transport": "stdio"}})
		if len(servers) != 1 || strings.Contains(servers[0].ID, raw) || !strings.HasPrefix(servers[0].ID, "server-redacted-") {
			t.Fatalf("private identifier was not replaced for %q: %#v", raw, servers)
		}
	}
}

func assertPublicDiagnosticsExclude(t *testing.T, value any, forbidden ...string) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, sentinel := range forbidden {
		if strings.Contains(string(encoded), sentinel) {
			t.Fatalf("public diagnostics leaked %q: %s", sentinel, encoded)
		}
	}
}
