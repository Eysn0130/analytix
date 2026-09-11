//go:build darwin

package runtimeapp

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	mcpprotocol "analytix.local/runtime-go/internal/adapters/outbound/mcp/protocol"
)

const runtimeAppGenericStdioContainmentChildV1 = "ANALYTIX_GENERIC_STDIO_CONTAINMENT_CHILD_V1"

func TestRuntimeProductionGenericStdioMCPRetainsMandatoryProtectedRoots(t *testing.T) {
	if os.Getenv(runtimeAppGenericStdioContainmentChildV1) == "1" {
		runRuntimeAppGenericStdioContainmentChildV1()
		return
	}

	dataDir := t.TempDir()
	userDataDir := t.TempDir()
	configuredProtectedRoot := t.TempDir()
	ordinaryRoot := t.TempDir()
	mandatoryProtectedFile := filepath.Join(userDataDir, "mandatory-sentinel.txt")
	configuredProtectedFile := filepath.Join(configuredProtectedRoot, "configured-sentinel.txt")
	for path, content := range map[string]string{
		mandatoryProtectedFile:  "mandatory-must-not-escape",
		configuredProtectedFile: "configured-must-not-escape",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	resultPath := filepath.Join(ordinaryRoot, "child-result.txt")
	configDocument, err := json.Marshal(map[string]any{
		"mcpServers": map[string]any{
			"ordinary_stdio": map[string]any{
				"command": os.Args[0],
				"args": []string{
					"-test.run=^TestRuntimeProductionGenericStdioMCPRetainsMandatoryProtectedRoots$",
				},
				"env": map[string]string{
					runtimeAppGenericStdioContainmentChildV1:                  "1",
					"ANALYTIX_GENERIC_STDIO_CONTAINMENT_MANDATORY_TARGET_V1":  mandatoryProtectedFile,
					"ANALYTIX_GENERIC_STDIO_CONTAINMENT_CONFIGURED_TARGET_V1": configuredProtectedFile,
					"ANALYTIX_GENERIC_STDIO_CONTAINMENT_RESULT_PATH_V1":       resultPath,
				},
				"expectedServerName":    "ordinary-stdio-contained",
				"expectedServerVersion": "1.0.0",
				"trustScope":            "user",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	handler, err := NewRuntimeServerHandlerE(Config{
		RuntimeToken:   DefaultRuntimeToken,
		DataDir:        dataDir,
		DurableTempDir: t.TempDir(),
		UserDataDir:    userDataDir,
		ProtectedReadDirs: []string{
			configuredProtectedRoot,
		},
		MCPConfigJSON: string(configDocument),
	})
	if err != nil {
		t.Fatalf("start runtime with contained ordinary stdio MCP: %v", err)
	}
	defer shutdownOwnedRuntimeHandler(t, handler)

	diagnostics := runtimeStartupJSON(
		t,
		handler,
		http.MethodGet,
		"/v1/runtime/tools",
		map[string]any{},
		http.StatusOK,
	)
	servers, ok := diagnostics["mcpServers"].([]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("ordinary stdio MCP diagnostics mismatch: %#v", diagnostics["mcpServers"])
	}
	server, ok := servers[0].(map[string]any)
	if !ok || server["id"] != "ordinary_stdio" ||
		server["status"] != "connected" || server["connected"] != true {
		t.Fatalf("ordinary stdio MCP did not connect through production composition: %#v", servers[0])
	}
	result, err := os.ReadFile(resultPath)
	if err != nil || string(result) != "blocked" {
		t.Fatalf("ordinary stdio MCP escaped mandatory protected roots: result=%q err=%v", result, err)
	}
}

func runRuntimeAppGenericStdioContainmentChildV1() {
	result := "blocked"
	for _, name := range []string{
		"ANALYTIX_GENERIC_STDIO_CONTAINMENT_MANDATORY_TARGET_V1",
		"ANALYTIX_GENERIC_STDIO_CONTAINMENT_CONFIGURED_TARGET_V1",
	} {
		if body, err := os.ReadFile(os.Getenv(name)); err == nil {
			result = "leaked:" + string(body)
			break
		}
	}
	if err := os.WriteFile(
		os.Getenv("ANALYTIX_GENERIC_STDIO_CONTAINMENT_RESULT_PATH_V1"),
		[]byte(result),
		0o600,
	); err != nil {
		return
	}

	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil || len(request.ID) == 0 {
			continue
		}
		response := map[string]any{"jsonrpc": "2.0", "id": request.ID}
		switch request.Method {
		case "initialize":
			response["result"] = map[string]any{
				"protocolVersion": mcpprotocol.ProtocolVersion,
				"serverInfo": map[string]any{
					"name": "ordinary-stdio-contained", "version": "1.0.0",
				},
				"capabilities": map[string]any{"tools": map[string]any{}},
			}
		case "tools/list":
			response["result"] = map[string]any{"tools": []any{}}
		default:
			response["error"] = map[string]any{"code": -32601, "message": "method not found"}
		}
		_ = encoder.Encode(response)
	}
}
