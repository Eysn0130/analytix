//go:build darwin && !analytix_prod

package runtimego

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// This contract requires successful protected stdio startup. The required
// macOS lane executes it; unsupported-host denial remains in process tests.
func TestRuntimeServerConfiguredStdioMCPToolLoopRejectsMutationWithoutHostSemanticIdentityAndContinues(t *testing.T) {
	if os.Getenv("ANALYTIX_RUNTIME_STDIO_MCP_HELPER") == "1" {
		runRuntimeServerStdioMCPHelper()
		return
	}

	dataDir := t.TempDir()
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_stdio_lookup","type":"function","function":{"name":"mcp__runtime-stdio__lookup","arguments":"{\"query\":\"needle\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"stdio mcp complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "stdio-mcp-loop-provider", "stdio-mcp-loop-model"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"mcpServers": map[string]any{
				"runtime-stdio": map[string]any{
					"transport":  "stdio",
					"command":    os.Args[0],
					"args":       []string{"-test.run=TestRuntimeServerConfiguredStdioMCPToolLoopRejectsMutationWithoutHostSemanticIdentityAndContinues"},
					"env":        map[string]string{"ANALYTIX_RUNTIME_STDIO_MCP_HELPER": "1"},
					"trustScope": "user",
				},
			},
		})),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Stdio MCP loop",
		"workspace":      dataDir,
		"providerId":     "stdio-mcp-loop-provider",
		"model":          "stdio-mcp-loop-model",
		"approvalPolicy": "auto",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Call configured stdio MCP.",
	}), http.StatusAccepted)

	if provider.RequestCount() != 2 {
		t.Fatalf("stdio MCP tool loop should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	body := provider.Body(1)
	_, mcpResult := providerHostToolResultForName(t, body, "mcp__runtime-stdio__lookup")
	resultJSON := string(mustJSON(t, mcpResult))
	if !strings.Contains(resultJSON, "side_effect_identity_unavailable") || strings.Contains(resultJSON, "stdio result: needle") || strings.Contains(body, "call_stdio_lookup") {
		t.Fatalf("second provider call must contain the host rejection and no stdio MCP result: result=%s body=%s", resultJSON, body)
	}
}

func runRuntimeServerStdioMCPHelper() {
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
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": runtimeMCPInitializeResult("runtime-stdio")})
		case "tools/list":
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"tools": []map[string]any{{
				"name":         "lookup",
				"description":  "Lookup runtime stdio MCP data",
				"inputSchema":  map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}},
				"outputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
			}}}})
		case "tools/call":
			args, _ := request.Params["arguments"].(map[string]any)
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"content": []map[string]any{{
				"type": "text",
				"text": "stdio result: " + fmt.Sprint(args["query"]),
			}}, "structuredContent": map[string]any{}}})
		default:
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}
}
