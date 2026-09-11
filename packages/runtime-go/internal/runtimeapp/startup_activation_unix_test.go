//go:build darwin || linux

package runtimeapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestStartupPlanningPerformsZeroMCPProviderProcessNetworkActivation(t *testing.T) {
	var networkCalls atomic.Int64
	endpoint := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		networkCalls.Add(1)
	}))
	defer endpoint.Close()
	spawned := filepath.Join(t.TempDir(), "spawned")
	providers, _ := json.Marshal(map[string]any{
		"defaultProviderId": "provider-test",
		"providers": []map[string]any{{
			"id": "provider-test", "apiKey": "test-key", "baseUrl": endpoint.URL + "/v1",
			"endpointFormat": "chat_completions", "models": []string{"model-test"},
		}},
	})
	mcp, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{
		"remote":  map[string]any{"transport": "http", "url": endpoint.URL + "/mcp", "trustScope": "user"},
		"process": map[string]any{"command": "/bin/sh", "args": []string{"-c", "printf spawned > " + spawned}, "trustScope": "user"},
	}})
	config := Config{
		RuntimeToken: DefaultRuntimeToken, DataDir: t.TempDir(), DurableTempDir: t.TempDir(),
		ModelProvidersJSON: string(providers), MCPConfigJSON: string(mcp),
	}
	lease, err := AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if _, err := PrepareRuntimeServerStartupWithPersistenceLeaseE(config, lease); err != nil {
		t.Fatalf("prepare runtime startup: %v", err)
	}
	if networkCalls.Load() != 0 {
		t.Fatalf("semantic planning activated provider or MCP network calls: %d", networkCalls.Load())
	}
	if _, err := os.Lstat(spawned); !os.IsNotExist(err) {
		t.Fatalf("semantic planning spawned an MCP process: %v", err)
	}
}
