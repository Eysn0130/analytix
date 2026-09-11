//go:build analytix_prod

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	provider "analytix.local/runtime-go/internal/provider"
)

func TestRuntimeServerCapabilitiesHideEvidenceMetadataInProduction(t *testing.T) {
	capabilities := httpapi.RuntimeServerContractCapabilities()
	if _, ok := capabilities["upstreamAbsorption"]; ok {
		t.Fatalf("production runtime capabilities must not expose internal validation metadata: %#v", capabilities)
	}
	mcp, _ := capabilities["mcp"].(map[string]any)
	if _, ok := mcp["contractProof"]; ok {
		t.Fatalf("production runtime capabilities must not expose MCP internal validation metadata: %#v", capabilities)
	}
}

func TestProductionRuntimeInfoExposesOnlyProductCapabilities(t *testing.T) {
	server := httptest.NewServer(NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
	}))
	defer server.Close()

	info := productionRuntimeJSON(t, server.URL+"/v1/runtime/info")
	assertNoProductionRuntimeMarkers(t, "runtime info", info)
	storage, _ := info["storage"].(map[string]any)
	if info["schemaVersion"] != float64(2) || info["listenerScope"] != "loopback" || storage["configured"] != true || storage["available"] != true {
		t.Fatalf("production runtime info public v2 contract mismatch: %#v", info)
	}
	for _, forbidden := range []string{"host", "dataDir", "configPath", "pid", "approvalPolicy", "sandboxMode"} {
		if _, exists := info[forbidden]; exists {
			t.Fatalf("production runtime info exposed retired raw field %s: %#v", forbidden, info)
		}
	}
}

func TestProductionRuntimeToolsExposeOnlyProductDiagnostics(t *testing.T) {
	server := httptest.NewServer(NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
	}))
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/runtime/tools?refresh=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", res.StatusCode)
	}
	var tools map[string]any
	if err := json.NewDecoder(res.Body).Decode(&tools); err != nil {
		t.Fatal(err)
	}

	for _, forbidden := range []string{"mcpLocalProof", "contractProof", "upstreamAbsorption"} {
		if _, ok := tools[forbidden]; ok {
			t.Fatalf("production runtime tools must not expose %s: %#v", forbidden, tools)
		}
	}
	raw, err := json.Marshal(tools)
	if err != nil {
		t.Fatal(err)
	}
	lowered := strings.ToLower(string(raw))
	for _, forbidden := range []string{"fixture", "fake mcp", "mcpLocalProof", "go-runtime-default-provider", "local-fake-key"} {
		if strings.Contains(lowered, strings.ToLower(forbidden)) {
			t.Fatalf("production runtime tools must not expose internal validation markers %q: %s", forbidden, string(raw))
		}
	}
	if tools["schemaVersion"] != float64(2) || tools["providerCount"] != float64(1) {
		t.Fatalf("production diagnostics must expose only the public v2 provider count: %#v", tools)
	}
	networkProxy, ok := tools["networkProxy"].(map[string]any)
	if !ok ||
		networkProxy["configured"] != false ||
		networkProxy["valid"] != true ||
		networkProxy["source"] != "environment" ||
		networkProxy["credentialsMasked"] != true {
		t.Fatalf("empty production runtime tools should expose auto/env network proxy diagnostics: %#v", tools)
	}
	mcpServers, ok := tools["mcpServers"].([]any)
	if !ok || len(mcpServers) != 0 {
		t.Fatalf("empty production MCP config must expose no MCP servers: %#v", tools)
	}
	if tools["mcpPromptCount"] != float64(0) || tools["mcpResourceCount"] != float64(0) {
		t.Fatalf("empty production runtime tools must expose only prompt/resource counts: %#v", tools)
	}
	commands, ok := tools["commands"].([]any)
	if !ok || len(commands) == 0 {
		t.Fatalf("production runtime tools must expose command diagnostics: %#v", tools)
	}
	firstCommand, ok := commands[0].(map[string]any)
	if !ok || stringField(firstCommand, "binary") == "" {
		t.Fatalf("command diagnostics must expose binary/found status: %#v", commands)
	}
	if _, ok := firstCommand["found"]; !ok {
		t.Fatalf("command diagnostics missing found status: %#v", firstCommand)
	}
	for _, forbidden := range []string{"command", "output", "error", "stdout", "stderr", "cwd"} {
		if _, exists := firstCommand[forbidden]; exists {
			t.Fatalf("command diagnostics exposed private field %s: %#v", forbidden, firstCommand)
		}
	}
	contracts, ok := tools["toolContracts"].(map[string]any)
	contractCount, _ := contracts["count"].(float64)
	if !ok || contractCount < 1 || len(stringField(contracts, "catalogHash")) != 64 {
		t.Fatalf("runtime tools must expose only a count and content hash for tool contracts: %#v", tools["toolContracts"])
	}
	mcpSearch, ok := tools["mcpSearch"].(map[string]any)
	if !ok {
		t.Fatalf("runtime tools missing mcpSearch diagnostics: %#v", tools)
	}
	if mcpSearch["enabled"] != false || mcpSearch["mode"] != "auto" ||
		mcpSearch["active"] != false || mcpSearch["available"] != false ||
		mcpSearch["indexedToolCount"] != float64(0) || mcpSearch["advertisedToolCount"] != float64(0) {
		t.Fatalf("empty production MCP config must be unavailable/empty: %#v", tools)
	}
	assertNoProductionRuntimeMarkers(t, "runtime tools", tools)
	for _, forbidden := range []string{"dataDir", "rootDir", "mcpPrompts", "mcpResources", "providers", "webProviders"} {
		if _, exists := tools[forbidden]; exists {
			t.Fatalf("public runtime tools exposed retired raw field %s: %#v", forbidden, tools)
		}
	}
}

func TestProductionBuildTagExcludesTestOnlyEvidencePackages(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	moduleRoot := filepath.Clean(filepath.Join(cwd, "../.."))
	deps := exec.Command("go", "list", "-tags", "analytix_prod", "-deps", "./cmd/runtime-server")
	deps.Dir = moduleRoot
	depsOutput, err := deps.CombinedOutput()
	if err != nil {
		t.Fatalf("go list production runtime-server deps failed: %v\n%s", err, string(depsOutput))
	}
	for _, forbidden := range []string{
		"/internal/upstreamaudit",
		"/internal/testsupport/providerscript",
		"/cmd/contract-sidecar",
		"/cmd/runtime-engine-absorption-report",
	} {
		if strings.Contains(string(depsOutput), forbidden) {
			t.Fatalf("production runtime-server dependency graph must not include %s:\n%s", forbidden, string(depsOutput))
		}
	}

	cmd := exec.Command("go", "list", "-tags", "analytix_prod", "./...")
	cmd.Dir = moduleRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list production packages failed: %v\n%s", err, string(output))
	}
	listed := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, pkg := range listed {
		for _, forbidden := range []string{
			"/internal/upstreamaudit",
			"/internal/testsupport/providerscript",
			"/cmd/contract-sidecar",
			"/cmd/runtime-engine-absorption-report",
		} {
			if strings.Contains(pkg, forbidden) {
				t.Fatalf("production package list must not include internal validation package %s: %s\n%s", forbidden, pkg, string(output))
			}
		}
	}

	files := exec.Command("go", "list", "-tags", "analytix_prod", "-f", "{{.ImportPath}} {{.GoFiles}}", "./...")
	files.Dir = moduleRoot
	filesOutput, err := files.CombinedOutput()
	if err != nil {
		t.Fatalf("go list production package files failed: %v\n%s", err, string(filesOutput))
	}
	for _, line := range strings.Split(string(filesOutput), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		fileList := trimmed
		if fields := strings.SplitN(trimmed, " ", 2); len(fields) == 2 {
			fileList = fields[1]
		}
		for _, forbidden := range []string{"d024", "d025", "live_local", "shadow", "oracle", "conformance"} {
			if strings.Contains(fileList, forbidden) {
				t.Fatalf("production package files must not include internal validation implementation marker %q: %s\n%s", forbidden, trimmed, string(filesOutput))
			}
		}
	}
}

func TestProductionMCPManagerUsesCanonicalRuntimeNaming(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	managerPath := filepath.Clean(filepath.Join(cwd, "..", "mcp", "manager.go"))
	source, err := os.ReadFile(managerPath)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(source)
	for _, forbidden := range []string{"D0244MCPToolName", "D0244RedactedMCPDiagnostic"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("production MCP manager must use canonical runtime helpers, found %s", forbidden)
		}
	}
}

func TestSubagentCapabilitiesReflectRuntimeToolAndStoreState(t *testing.T) {
	available := runtimeinfoapp.SubagentCapabilityState(true, []string{"read", "task", "parallel_tasks", "wait", "bash_output", "kill_shell", "restart_job", "steer_job", "pause_job", "resume_job"})
	if available["available"] != true ||
		available["taskToolAvailable"] != true ||
		available["parallelTasksToolAvailable"] != true ||
		available["durableChildRunStore"] != true ||
		available["modelJobToolsAvailable"] != true {
		t.Fatalf("subagent capability should be available with task/parallel tools and durable store: %#v", available)
	}
	profiles, _ := available["profiles"].([]any)
	if available["profilesAvailable"] != false || len(profiles) != 0 {
		t.Fatalf("generic subagent capability helper must not invent configured profiles: %#v", available)
	}
	missingParallel := runtimeinfoapp.SubagentCapabilityState(true, []string{"read", "task"})
	if missingParallel["available"] != false ||
		missingParallel["parallelTasksToolAvailable"] != false ||
		missingParallel["modelJobToolsAvailable"] != false {
		t.Fatalf("subagent capability should be unavailable without parallel_tasks: %#v", missingParallel)
	}
	missingStore := runtimeinfoapp.SubagentCapabilityState(false, []string{"task", "parallel_tasks", "wait", "bash_output", "kill_shell", "restart_job", "steer_job", "pause_job", "resume_job"})
	if missingStore["available"] != false ||
		missingStore["durableChildRunStore"] != false ||
		missingStore["modelJobToolsAvailable"] != false {
		t.Fatalf("subagent capability should be unavailable without durable store: %#v", missingStore)
	}
}

func TestParallelTaskDependencyValidation(t *testing.T) {
	valid, err := subagentapp.ParallelTaskRequestsFromArgs(map[string]any{"tasks": []any{
		map[string]any{"id": "first", "prompt": "First"},
		map[string]any{"id": "second", "prompt": "Second", "depends_on": []any{"first"}},
	}})
	if err != nil || len(valid) != 2 || len(valid[1].Request.DependsOn) != 1 {
		t.Fatalf("valid parallel task DAG rejected: tasks=%#v err=%v", valid, err)
	}
	for label, args := range map[string]map[string]any{
		"duplicate": {"tasks": []any{
			map[string]any{"id": "same", "prompt": "First"},
			map[string]any{"id": "same", "prompt": "Second"},
		}},
		"unknown": {"tasks": []any{
			map[string]any{"id": "first", "prompt": "First", "depends_on": []any{"missing"}},
			map[string]any{"id": "second", "prompt": "Second"},
		}},
		"cycle": {"tasks": []any{
			map[string]any{"id": "first", "prompt": "First", "depends_on": []any{"second"}},
			map[string]any{"id": "second", "prompt": "Second", "depends_on": []any{"first"}},
		}},
	} {
		if _, err := subagentapp.ParallelTaskRequestsFromArgs(args); err == nil {
			t.Fatalf("expected %s parallel task DAG to be rejected", label)
		}
	}
}

func TestRuntimeToolSchemaHashCanonicalizesOrderAndJSONKeys(t *testing.T) {
	left := []provider.ToolSchema{
		{
			Name:        "write",
			Description: "Write a file",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`),
		},
		{
			Name:        "read",
			Description: "Read a file",
			Parameters:  json.RawMessage(`{"required":["path"],"properties":{"path":{"description":"File path","type":"string"}},"type":"object"}`),
		},
	}
	right := []provider.ToolSchema{
		{
			Name:        "read",
			Description: "Read a file",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"File path"}},"required":["path"]}`),
		},
		{
			Name:        "write",
			Description: "Write a file",
			Parameters:  json.RawMessage(`{"required":["path","content"],"properties":{"content":{"type":"string"},"path":{"type":"string"}},"type":"object"}`),
		},
	}
	if toolcatalogapp.ToolSchemaHash(left) != toolcatalogapp.ToolSchemaHash(right) {
		t.Fatalf("schema hash should ignore tool order and JSON key order: left=%s right=%s", toolcatalogapp.ToolSchemaHash(left), toolcatalogapp.ToolSchemaHash(right))
	}
	changed := append([]provider.ToolSchema(nil), right...)
	changed[0].Parameters = json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"limit":{"type":"integer"}},"required":["path"]}`)
	if toolcatalogapp.ToolSchemaHash(left) == toolcatalogapp.ToolSchemaHash(changed) {
		t.Fatalf("schema hash should change when provider-visible tool parameters change")
	}
}

func productionRuntimeJSON(t *testing.T, url string) map[string]any {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", res.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func assertNoProductionRuntimeMarkers(t *testing.T, label string, body map[string]any) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	lowered := strings.ToLower(string(raw))
	for _, forbidden := range []string{
		"d024",
		"d025",
		"proof",
		"fixture",
		"fake",
		"mcpLocalProof",
		"upstreamAbsorption",
		"reasonixPublicProtocol",
	} {
		if strings.Contains(lowered, strings.ToLower(forbidden)) {
			t.Fatalf("production %s must not expose marker %q: %s", label, forbidden, string(raw))
		}
	}
}
