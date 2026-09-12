//go:build !analytix_prod

package runtimego

import (
	"bufio"
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf16"

	checkpointauthority "analytix.local/runtime-go/internal/adapters/outbound/checkpointauthority"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	apploop "analytix.local/runtime-go/internal/app/loop"
	terminalapp "analytix.local/runtime-go/internal/app/terminal"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	appturn "analytix.local/runtime-go/internal/app/turn"
	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	"analytix.local/runtime-go/internal/jobs"
	providerpkg "analytix.local/runtime-go/internal/provider"
	research "analytix.local/runtime-go/internal/research"
	runtimeapp "analytix.local/runtime-go/internal/runtimeapp"
	providerscript "analytix.local/runtime-go/internal/testsupport/providerscript"
	"analytix.local/runtime-go/internal/testsupport/toolidentity"
)

const (
	runtimeServerPositiveTestTimeout = 15 * time.Second
	runtimeServerShutdownTestTimeout = 30 * time.Second
	// The production command test performs instrumented startup and two turns.
	// Keep it bounded while allowing race-sharded hosts more than the shutdown-only window.
	runtimeServerProductionCommandTestTimeout = 4 * runtimeServerPositiveTestTimeout
)

func runtimeServerPrivateToolArgumentTempDir(t *testing.T, label string) string {
	t.Helper()
	for _, character := range label {
		if (character < 'a' || character > 'z') && character != '-' {
			t.Fatalf("private tool argument temp label %q must contain only lowercase ASCII letters and hyphens", label)
		}
	}
	root := filepath.Clean(os.TempDir())
	if root == "." || !filepath.IsAbs(root) {
		t.Fatalf("private tool argument temp root must be absolute: %q", root)
	}
	for attempt := 0; attempt < 10; attempt++ {
		var entropy [16]byte
		if _, err := cryptorand.Read(entropy[:]); err != nil {
			t.Fatalf("read private tool argument temp entropy: %v", err)
		}
		suffix := make([]byte, len(entropy)*2)
		for index, value := range entropy {
			suffix[index*2] = 'a' + value>>4
			suffix[index*2+1] = 'a' + value&0x0f
		}
		path := filepath.Join(root, "analytix-"+label+"-"+string(suffix))
		if domainsecurity.ContainsProtectedCaseData(path) {
			t.Fatalf("private tool argument temp path unexpectedly matches restricted data: %q", path)
		}
		if err := os.Mkdir(path, 0o700); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			t.Fatalf("create private tool argument temp directory: %v", err)
		}
		t.Cleanup(func() {
			if filepath.Dir(path) != root {
				t.Errorf("refusing to clean unexpected private tool argument temp path: %q", path)
				return
			}
			if err := os.RemoveAll(path); err != nil {
				t.Errorf("clean private tool argument temp directory: %v", err)
			}
		})
		return path
	}
	t.Fatalf("allocate unique private tool argument temp directory under %q", root)
	return ""
}

func TestRuntimeServerPrivateToolArgumentTempDirAvoidsRestrictedPIILookalikes(t *testing.T) {
	directory := runtimeServerPrivateToolArgumentTempDir(t, "fixture-path")
	if domainsecurity.ContainsProtectedCaseData(directory) {
		t.Fatalf("private tool argument temp directory matched restricted data: %q", directory)
	}
	base := filepath.Base(directory)
	for _, character := range base {
		if character >= '0' && character <= '9' {
			t.Fatalf("private tool argument temp basename contains a decimal digit: %q", base)
		}
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 {
		t.Fatalf("private tool argument temp directory is not a private real directory: info=%v err=%v", info, err)
	}
}

func newRuntimeServerTestHandler(t *testing.T, config RuntimeServerContractConfig) http.Handler {
	t.Helper()
	config = prepareRuntimeServerTestConfig(t, config)
	return newRuntimeServerContractTestHandler(t, config)
}

// runtimeServerContractTestHandler owns the production-composed handler's
// lifecycle. Closing an httptest.Server does not release the runtime's durable
// leases or drain its background components, so every test construction must
// register the matching Shutdown call. Shutdown remains idempotent because a
// small number of restart tests deliberately close the first handler early.
type runtimeServerContractTestHandler struct {
	http.Handler
	shutdownMu       sync.Mutex
	shutdownComplete bool
}

func newRuntimeServerContractTestHandler(
	t *testing.T,
	config RuntimeServerContractConfig,
) http.Handler {
	t.Helper()
	owned := &runtimeServerContractTestHandler{Handler: NewRuntimeServerContractHandler(config)}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), runtimeServerShutdownTestTimeout)
		defer cancel()
		if err := owned.Shutdown(ctx); err != nil {
			t.Errorf("shutdown runtime test handler: %v", err)
		}
	})
	return owned
}

func (handler *runtimeServerContractTestHandler) Shutdown(ctx context.Context) error {
	if handler == nil || handler.Handler == nil {
		return errors.New("runtime test handler is unavailable")
	}
	handler.shutdownMu.Lock()
	defer handler.shutdownMu.Unlock()
	if handler.shutdownComplete {
		return nil
	}
	shutdown, ok := handler.Handler.(interface{ Shutdown(context.Context) error })
	if !ok {
		return errors.New("runtime test handler does not expose shutdown")
	}
	if err := shutdown.Shutdown(ctx); err != nil {
		return err
	}
	handler.shutdownComplete = true
	return nil
}

func prepareRuntimeServerTestConfig(t *testing.T, config RuntimeServerContractConfig) RuntimeServerContractConfig {
	t.Helper()
	if strings.TrimSpace(config.ModelProvidersJSON) == "" && strings.TrimSpace(config.BaseURL) == "" {
		fakeProvider := providerscript.NewScriptedProviderServer()
		t.Cleanup(fakeProvider.Close)
		config.ProviderID = firstNonEmptyTestString(config.ProviderID, "deepseek")
		config.BaseURL = fakeProvider.URL + "/deepseek/v1"
		config.APIKey = firstNonEmptyTestString(config.APIKey, "test-provider-key")
		config.EndpointFormat = firstNonEmptyTestString(config.EndpointFormat, "chat_completions")
		config.Model = firstNonEmptyTestString(config.Model, "deepseek-chat")
		config.ModelProvidersJSON = string(mustJSONNoTest(map[string]any{
			"defaultProviderId": config.ProviderID,
			"providers": []map[string]any{{
				"id":             config.ProviderID,
				"apiKey":         config.APIKey,
				"baseUrl":        config.BaseURL,
				"endpointFormat": config.EndpointFormat,
				"models":         []string{config.Model},
				"modelProfiles": map[string]any{
					config.Model: map[string]any{
						"reasoning": map[string]any{
							"requestProtocol":  "deepseek-chat-completions",
							"supportedEfforts": []string{"off", "high", "max"},
							"defaultEffort":    "high",
						},
					},
				},
			}},
		}))
	}
	return config
}

func newRuntimeServerHostAuthorityTestHandler(
	t *testing.T,
	config RuntimeServerContractConfig,
) (http.Handler, finalauthority.SecurePrivateCASAccessAuthority) {
	t.Helper()
	config = prepareRuntimeServerTestConfig(t, config)
	if strings.TrimSpace(config.RuntimeToken) == "" && !config.Insecure {
		config.RuntimeToken = DefaultRuntimeToken
	}
	lease, err := runtimeapp.AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := runtimeapp.NewRuntimeServerHandlerWithPersistenceLeaseE(config, lease)
	if err != nil {
		_ = lease.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		shutdownRuntimeTestHandler(t, handler)
		if err := lease.Close(); err != nil {
			t.Errorf("close runtime persistence lease: %v", err)
		}
	})
	return handler, lease
}

func shutdownRuntimeTestHandler(t *testing.T, handler http.Handler) {
	t.Helper()
	shutdown, ok := handler.(interface{ Shutdown(context.Context) error })
	if !ok {
		t.Fatal("runtime test handler does not expose shutdown")
	}
	ctx, cancel := context.WithTimeout(context.Background(), runtimeServerShutdownTestTimeout)
	defer cancel()
	if err := shutdown.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown runtime test handler: %v", err)
	}
}

func TestRuntimeServerTestHandlerShutdownReleasesPersistenceLease(t *testing.T) {
	config := prepareRuntimeServerTestConfig(t, RuntimeServerContractConfig{
		RuntimeToken:   DefaultRuntimeToken,
		Host:           "127.0.0.1",
		DataDir:        t.TempDir(),
		DurableTempDir: t.TempDir(),
	})
	handler := newRuntimeServerContractTestHandler(t, config)
	shutdownRuntimeTestHandler(t, handler)

	lease, err := runtimeapp.AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatalf("reacquire persistence lease after handler shutdown: %v", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("close reacquired persistence lease: %v", err)
	}
}

type retryableRuntimeServerTestShutdownHandler struct {
	mu    sync.Mutex
	calls int
}

func (handler *retryableRuntimeServerTestShutdownHandler) ServeHTTP(http.ResponseWriter, *http.Request) {
}

func (handler *retryableRuntimeServerTestShutdownHandler) Shutdown(context.Context) error {
	handler.mu.Lock()
	defer handler.mu.Unlock()
	handler.calls++
	if handler.calls == 1 {
		return context.DeadlineExceeded
	}
	return nil
}

func (handler *retryableRuntimeServerTestShutdownHandler) callCount() int {
	handler.mu.Lock()
	defer handler.mu.Unlock()
	return handler.calls
}

func TestRuntimeServerContractTestHandlerShutdownRetriesAfterIncompleteDrain(t *testing.T) {
	delegate := &retryableRuntimeServerTestShutdownHandler{}
	handler := &runtimeServerContractTestHandler{Handler: delegate}
	if err := handler.Shutdown(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first shutdown error = %v, want deadline exceeded", err)
	}
	if err := handler.Shutdown(context.Background()); err != nil {
		t.Fatalf("retry shutdown: %v", err)
	}
	if err := handler.Shutdown(context.Background()); err != nil {
		t.Fatalf("idempotent shutdown after success: %v", err)
	}
	if calls := delegate.callCount(); calls != 2 {
		t.Fatalf("delegate shutdown calls = %d, want 2", calls)
	}
}

func newCrashSimulationRuntimeTestHandler(t *testing.T, config RuntimeServerContractConfig) (http.Handler, func()) {
	t.Helper()
	if strings.TrimSpace(config.RuntimeToken) == "" && !config.Insecure {
		config.RuntimeToken = DefaultRuntimeToken
	}
	lease, err := runtimeapp.AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := runtimeapp.NewRuntimeServerHandlerWithPersistenceLeaseE(config, lease)
	if err != nil {
		_ = lease.Close()
		t.Fatal(err)
	}
	return handler, func() {
		if err := lease.Close(); err != nil {
			t.Fatalf("release crash-simulation persistence lease: %v", err)
		}
	}
}

func firstNonEmptyTestString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func testUTF16LEWithBOM(content string) []byte {
	units := utf16.Encode([]rune(content))
	out := []byte{0xff, 0xfe}
	for _, unit := range units {
		var encoded [2]byte
		binary.LittleEndian.PutUint16(encoded[:], unit)
		out = append(out, encoded[:]...)
	}
	return out
}

func decodeTestUTF16LEWithBOM(t *testing.T, data []byte) string {
	t.Helper()
	if len(data) < 2 || data[0] != 0xff || data[1] != 0xfe {
		t.Fatalf("expected UTF-16LE BOM, got prefix=%#v len=%d", data[:min(len(data), 2)], len(data))
	}
	data = data[2:]
	if len(data)%2 == 1 {
		t.Fatalf("expected even UTF-16 byte count, got %d", len(data))
	}
	units := make([]uint16, 0, len(data)/2)
	for index := 0; index+1 < len(data); index += 2 {
		units = append(units, binary.LittleEndian.Uint16(data[index:index+2]))
	}
	return string(utf16.Decode(units))
}

func TestRuntimeServerDefaultsTurnControlsFromRuntimeSettings(t *testing.T) {
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		ApprovalPolicy: "auto",
		SandboxMode:    "read-only",
	}))
	defer server.Close()

	info := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/info", DefaultRuntimeToken, nil, http.StatusOK)
	executionPolicy := mapField(t, info, "executionPolicy")
	if executionPolicy["approvalPolicy"] != "auto" || executionPolicy["sandboxMode"] != "read-only" {
		t.Fatalf("runtime info must reflect configured defaults: %#v", info)
	}
	created := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads",
		DefaultRuntimeToken,
		mustJSON(t, map[string]any{
			"title":     "Default controls",
			"workspace": t.TempDir(),
		}),
		http.StatusCreated,
	)
	threadID := stringField(created, "id")
	if created["approvalPolicy"] != "auto" || created["sandboxMode"] != "read-only" {
		t.Fatalf("thread create must inherit runtime default controls: %#v", created)
	}
	start := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/turns",
		DefaultRuntimeToken,
		mustJSON(t, map[string]any{"prompt": "Use runtime defaults."}),
		http.StatusAccepted,
	)
	turnID := stringField(start, "turnId")
	thread := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	turn := findRuntimeServerTurn(t, thread, turnID)
	if _, present := turn["approvalPolicy"]; present {
		t.Fatalf("public turn projection must not expose approval policy: %#v", turn)
	}
	if _, present := turn["sandboxMode"]; present {
		t.Fatalf("public turn projection must not expose sandbox mode: %#v", turn)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	events := runtimeServerEventsForTurn(t, replay, turnID)
	turnStarted := firstRuntimeServerEvent(t, events, "turn_started")
	if turnStarted["approvalPolicy"] != "auto" || turnStarted["sandboxMode"] != "read-only" {
		t.Fatalf("turn_started must inherit runtime default controls: %#v", turnStarted)
	}
}

func TestRuntimeServerRuntimeInfoReadsVisionBridgeCapabilityConfig(t *testing.T) {
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		MCPConfigJSON: mustJSONString(t, map[string]any{
			"capabilities": map[string]any{
				"visionBridge": map[string]any{
					"enabled":                             true,
					"mode":                                "auto",
					"providerId":                          "xiaomi",
					"baseUrl":                             "https://api.xiaomimimo.com/v1",
					"apiKey":                              "bridge-secret-key",
					"endpointFormat":                      "chat_completions",
					"model":                               "mimo-v2.5",
					"maxScreenshotsPerTurn":               3,
					"fallbackWhenPrimaryImageUnsupported": true,
					"semanticProbeStatus":                 "supported",
				},
			},
		}),
	}))
	defer server.Close()

	info := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/info", DefaultRuntimeToken, nil, http.StatusOK)
	visionBridge := mapField(t, mapField(t, info, "capabilities"), "visionBridge")
	// Legacy capability configuration expresses intent and limits; it cannot
	// establish Registry media authority or a trusted local image privacy path.
	if visionBridge["status"] != "unavailable" || visionBridge["available"] != false || visionBridge["enabled"] != true || visionBridge["reasonCode"] != "unavailable" {
		t.Fatalf("legacy routing must not advertise executable vision authority: %#v", visionBridge)
	}
	if visionBridge["providerId"] != nil || visionBridge["model"] != nil {
		t.Fatalf("runtime info exposed legacy bridge route as current authority: %#v", visionBridge)
	}
	if visionBridge["semanticProbeStatus"] != "supported" || jsonIntField(t, visionBridge, "maxScreenshotsPerTurn") != 3 {
		t.Fatalf("runtime info should preserve probe status and screenshot limit: %#v", visionBridge)
	}
	raw := mustJSONString(t, info)
	for _, secret := range []string{"bridge-secret-key", "api.xiaomimimo.com"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("runtime info must not leak bridge secret/base URL %q: %s", secret, raw)
		}
	}
}

func TestRuntimeServerReportsConfiguredNetworkProxyDiagnostics(t *testing.T) {
	server := httptest.NewServer(newRuntimeServerTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		ModelProxyURL:  "socks5://proxy-user:proxy-secret@127.0.0.1:7890",
	}))
	defer server.Close()

	info := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/info", DefaultRuntimeToken, nil, http.StatusOK)
	infoProxy := mapField(t, info, "networkProxy")
	if infoProxy["configured"] != true ||
		infoProxy["valid"] != true ||
		infoProxy["source"] != "settings.provider.proxy" ||
		infoProxy["mode"] != "custom" ||
		infoProxy["credentialsMasked"] != true ||
		infoProxy["summary"] != nil {
		t.Fatalf("runtime info network proxy diagnostics mismatch: %#v", infoProxy)
	}
	if raw := mustJSONString(t, info); strings.Contains(raw, "proxy-secret") {
		t.Fatalf("runtime info leaked proxy password: %s", raw)
	}

	tools := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/tools", DefaultRuntimeToken, nil, http.StatusOK)
	toolsProxy := mapField(t, tools, "networkProxy")
	if toolsProxy["configured"] != true ||
		toolsProxy["valid"] != true ||
		toolsProxy["mode"] != "custom" ||
		toolsProxy["credentialsMasked"] != true ||
		toolsProxy["summary"] != nil {
		t.Fatalf("runtime tools network proxy diagnostics mismatch: %#v", toolsProxy)
	}
	if raw := mustJSONString(t, tools); strings.Contains(raw, "proxy-secret") {
		t.Fatalf("runtime tools leaked proxy password: %s", raw)
	}
}

func TestRuntimeServerRejectsConfiguredMCPProxyWithoutDirectFallback(t *testing.T) {
	var mu sync.Mutex
	methodCounts := map[string]int{}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !r.URL.IsAbs() || r.URL.Host != "mcp.example.test" {
			t.Fatalf("MCP request should be sent through configured proxy, got url=%q host=%q", r.URL.String(), r.Host)
		}
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
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result":  runtimeMCPInitializeResult("proxied-mcp"),
			})
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result": map[string]any{"tools": []map[string]any{{
					"name":         "lookup",
					"description":  "Look up through runtime MCP proxy",
					"inputSchema":  map[string]any{"type": "object"},
					"outputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
				}}},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
	defer proxy.Close()

	server := httptest.NewServer(newRuntimeServerTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		MCPProxyURL:    proxy.URL,
		MCPConfigJSON: mustJSONString(t, map[string]any{
			"mcpServers": map[string]any{
				"proxied-http": map[string]any{
					"type":       "http",
					"url":        "http://mcp.example.test/rpc",
					"trustScope": "user",
				},
			},
		}),
	}))
	defer server.Close()

	tools := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/tools", DefaultRuntimeToken, nil, http.StatusOK)
	mcpServers, _ := tools["mcpServers"].([]any)
	if len(mcpServers) != 1 {
		t.Fatalf("runtime tools should expose the proxied MCP server: %#v", tools)
	}
	row, _ := mcpServers[0].(map[string]any)
	if boolField(row, "available") || boolField(row, "connected") || floatField(t, row, "toolCount") != 0 ||
		fmt.Sprint(row["failureCode"]) != "connection_failed" {
		t.Fatalf("proxy-backed MCP server did not fail closed: %#v", row)
	}
	mu.Lock()
	initializeCount := methodCounts["initialize"]
	listCount := methodCounts["tools/list"]
	mu.Unlock()
	if initializeCount != 0 || listCount != 0 {
		t.Fatalf("rejected MCP proxy performed network I/O: initialize=%d list=%d", initializeCount, listCount)
	}
}

func TestRuntimeServerToolsExposeRealMCPPromptsAndResources(t *testing.T) {
	const resourceSecret = "runtime-resource-secret"
	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result":  runtimeMCPInitializeResult("runtime-catalog", "prompts", "resources"),
			})
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result": map[string]any{"tools": []map[string]any{{
					"name":         "lookup",
					"description":  "Look up through runtime catalog",
					"inputSchema":  map[string]any{"type": "object"},
					"outputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
				}}},
			})
		case "prompts/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result": map[string]any{"prompts": []map[string]any{{
					"name":        "summarize",
					"description": "Summarize token=runtime-prompt-secret",
				}}},
			})
		case "resources/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result": map[string]any{"resources": []map[string]any{{
					"uri":      "https://resource-user:resource-password@example.test/doc?access_token=" + resourceSecret,
					"name":     "Runtime docs",
					"mimeType": "text/plain",
				}}},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
	defer mcpServer.Close()

	server := httptest.NewServer(newRuntimeServerTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		MCPConfigJSON: mustJSONString(t, map[string]any{
			"mcpServers": map[string]any{
				"runtime-catalog": map[string]any{
					"type":       "http",
					"url":        mcpServer.URL,
					"trustScope": "user",
				},
			},
		}),
	}))
	defer server.Close()

	tools := waitForRuntimeToolsMCPPromptResourceCounts(t, server.URL, DefaultRuntimeToken, 1, 1)
	if tools["mcpPrompts"] != nil || tools["mcpResources"] != nil {
		t.Fatalf("public runtime tools must expose only MCP prompt/resource counts: %#v", tools)
	}
	raw := mustJSONString(t, tools)
	for _, secret := range []string{"runtime-prompt-secret", "resource-user", "resource-password", resourceSecret} {
		if strings.Contains(raw, secret) {
			t.Fatalf("runtime MCP prompts/resources leaked %q: %s", secret, raw)
		}
	}

	info := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/info", DefaultRuntimeToken, nil, http.StatusOK)
	mcpCapability := mapField(t, mapField(t, info, "capabilities"), "mcp")
	if mcpCapability["toolCount"] != float64(1) ||
		mcpCapability["promptCount"] != float64(1) ||
		mcpCapability["resourceCount"] != float64(1) ||
		mapField(t, mcpCapability, "catalog")["toolCount"] != float64(1) ||
		mapField(t, mcpCapability, "catalog")["promptCount"] != float64(1) ||
		mapField(t, mcpCapability, "catalog")["resourceCount"] != float64(1) {
		t.Fatalf("runtime info should expose MCP prompt/resource counts as diagnostics only: %#v", info)
	}
}

func floatField(t *testing.T, record map[string]any, key string) float64 {
	t.Helper()
	switch value := record[key].(type) {
	case float64:
		return value
	case int:
		return float64(value)
	case json.Number:
		parsed, err := value.Float64()
		if err == nil {
			return parsed
		}
	}
	t.Fatalf("field %q is not numeric: %#v", key, record[key])
	return 0
}

func waitForRuntimeToolsMCPPromptResourceCounts(t *testing.T, serverURL string, token string, promptCount int, resourceCount int) map[string]any {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var tools map[string]any
	for {
		tools = assertLiveJSON(t, serverURL, http.MethodGet, "/v1/runtime/tools", token, nil, http.StatusOK)
		if int(floatField(t, tools, "mcpPromptCount")) == promptCount && int(floatField(t, tools, "mcpResourceCount")) == resourceCount {
			return tools
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for runtime MCP prompt/resource catalog: %#v", tools)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type providerCaptureServer struct {
	server *httptest.Server
	mu     sync.Mutex
	bodies []string
}

func newProviderCaptureServer(t *testing.T) *providerCaptureServer {
	t.Helper()
	capture := &providerCaptureServer{}
	capture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capture.mu.Lock()
		capture.bodies = append(capture.bodies, string(body))
		capture.mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"attachment ok"},"finish_reason":"stop"}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":5,"total_tokens":105,"prompt_tokens_details":{"cached_tokens":20}}}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	t.Cleanup(capture.server.Close)
	return capture
}

func (c *providerCaptureServer) URL() string {
	return c.server.URL
}

func (c *providerCaptureServer) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bodies = nil
}

func (c *providerCaptureServer) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.bodies)
}

func (c *providerCaptureServer) LastBody(t *testing.T) string {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.bodies) == 0 {
		t.Fatalf("provider capture server did not receive a request")
	}
	return c.bodies[len(c.bodies)-1]
}

func TestRuntimeServerDirectLightweightPromptsDoNotAdvertiseToolsToProvider(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	dataDir := t.TempDir()
	capture := newProviderCaptureServer(t)
	modelProvidersJSON, connectProviderRegistry := prepareRuntimeServerExplicitProviderRegistryFixture(
		t, dataDir, capture.URL(), "deepseek", "deepseek-v4-pro",
	)
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       g1.RuntimeToken,
		StartedAt:          g1.StartedAt,
		Routes:             g2.Routes,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: modelProvidersJSON,
	}))
	defer server.Close()
	connectProviderRegistry(server.URL)

	for _, prompt := range []string{"你是什么大模型", "你是什么模型", "你好", "这个问题怎么理解", "简单总结一下", "What is the goal of reinforcement learning?"} {
		thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", g1.RuntimeToken, mustJSON(t, map[string]any{
			"title":     "Lightweight prompt",
			"workspace": dataDir,
		}), http.StatusCreated)
		threadID := stringField(thread, "id")
		assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", g1.RuntimeToken, mustJSON(t, map[string]any{
			"prompt":     prompt,
			"providerId": "deepseek",
			"model":      "deepseek-v4-pro",
		}), http.StatusAccepted)

		body := capture.LastBody(t)
		var request map[string]any
		if err := json.Unmarshal([]byte(body), &request); err != nil {
			t.Fatalf("provider request for %q should be JSON: %v\n%s", prompt, err, body)
		}
		if _, ok := request["tools"]; ok {
			t.Fatalf("direct lightweight prompt %q must not advertise tools to provider:\n%s", prompt, body)
		}
		if !strings.Contains(body, "provider=deepseek") ||
			!strings.Contains(body, "model=deepseek-v4-pro") ||
			!strings.Contains(body, "Do not claim to be Claude or Anthropic") {
			t.Fatalf("provider request for %q missing identity guard system prompt:\n%s", prompt, body)
		}
		replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", g1.RuntimeToken, http.StatusOK)
		if !strings.Contains(replay, `"route":"direct_answer"`) ||
			!strings.Contains(replay, `"toolCount":0`) ||
			!strings.Contains(replay, `"firstTokenLatencyMs":`) ||
			!strings.Contains(replay, `"durationMs":`) {
			t.Fatalf("direct lightweight prompt %q should persist direct_answer cache diagnostics:\n%s", prompt, replay)
		}
	}
}

func TestRuntimeServerWorkPromptDoesNotAdvertiseSubagentOrSkillToolsWithoutCue(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	dataDir := t.TempDir()
	capture := newProviderCaptureServer(t)
	modelProviders, connectProviderRegistry := prepareRuntimeServerExplicitProviderRegistryFixture(
		t, dataDir, capture.URL(), "deepseek", "deepseek-v4-pro",
	)
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       g1.RuntimeToken,
		StartedAt:          g1.StartedAt,
		Routes:             g2.Routes,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: modelProviders,
	}))
	defer server.Close()
	connectProviderRegistry(server.URL)

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", g1.RuntimeToken, mustJSON(t, map[string]any{
		"title":     "Scoped work prompt",
		"workspace": dataDir,
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", g1.RuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "请分析项目代码并给出结论",
		"providerId": "deepseek",
		"model":      "deepseek-v4-pro",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")

	toolNames := providerRequestToolNames(t, capture.LastBody(t))
	for _, expected := range []string{"read", "grep", "ls", "code_index"} {
		if !containsString(toolNames, expected) {
			t.Fatalf("ordinary work prompt should still advertise builtin investigation tool %s: %#v", expected, toolNames)
		}
	}
	for _, hidden := range []string{"delegate_task", "task", "parallel_tasks", "run_skill"} {
		if containsString(toolNames, hidden) {
			t.Fatalf("ordinary work prompt should not advertise heavy %s tool without an explicit cue: %#v", hidden, toolNames)
		}
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", g1.RuntimeToken, http.StatusOK)
	events := runtimeServerEventsForTurn(t, replay, turnID)
	usage := firstRuntimeServerEvent(t, events, "usage")
	diagnostics := mapField(t, usage, "cacheDiagnostics")
	if diagnostics["route"] != "tool_agent" {
		t.Fatalf("ordinary work prompt should persist tool_agent route diagnostics: %#v", diagnostics)
	}
	if floatField(t, diagnostics, "toolCount") >= 20 {
		t.Fatalf("ordinary work prompt should keep provider-visible tool count bounded after heavy-tool scoping: %#v", diagnostics)
	}
}

func TestRuntimeServerRejectsProviderModelMismatchBeforeProviderRequest(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	dataDir := t.TempDir()
	capture := newProviderCaptureServer(t)
	modelProviders, connectProviderRegistry := prepareRuntimeServerExplicitProviderRegistryFixture(
		t, dataDir, capture.URL(), "deepseek", "deepseek-v4-pro",
	)
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       g1.RuntimeToken,
		StartedAt:          g1.StartedAt,
		Routes:             g2.Routes,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: modelProviders,
	}))
	defer server.Close()
	connectProviderRegistry(server.URL)

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", g1.RuntimeToken, mustJSON(t, map[string]any{
		"title":      "Provider mismatch",
		"workspace":  dataDir,
		"providerId": "deepseek",
		"model":      "deepseek-v4-pro",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	response := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", g1.RuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "你好",
		"providerId": "deepseek",
		"model":      "mimo-v2-pro",
	}), http.StatusInternalServerError)

	if capture.Count() != 0 {
		t.Fatalf("provider mismatch should fail before provider request, got %d request(s)", capture.Count())
	}
	assertClosedTurnFailureResponse(t, response, "provider_model_invalid", "The selected model is not configured for this provider.")
	if _, exists := response["providerError"]; exists {
		t.Fatalf("closed public failure response must not expose provider diagnostics: %#v", response)
	}
}

func TestRuntimeServerThreadCreateDoesNotInventDefaultModel(t *testing.T) {
	dataDir := t.TempDir()
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        dataDir,
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":     "No runtime model",
		"workspace": dataDir,
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	if got := strings.TrimSpace(stringField(thread, "model")); got != "" {
		t.Fatalf("thread create must not invent default model, got %q in %#v", got, thread)
	}
	if got := strings.TrimSpace(stringField(thread, "providerId")); got != "" {
		t.Fatalf("thread create must not invent default provider, got %q in %#v", got, thread)
	}

	reloaded := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	if got := strings.TrimSpace(stringField(reloaded, "model")); got != "" {
		t.Fatalf("reloaded thread must not invent default model, got %q in %#v", got, reloaded)
	}
	if got := strings.TrimSpace(stringField(reloaded, "providerId")); got != "" {
		t.Fatalf("reloaded thread must not invent default provider, got %q in %#v", got, reloaded)
	}
}

func TestRuntimeServerRejectsStaleThreadModelBeforeDefaultFallback(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	dataDir := t.TempDir()
	capture := newProviderCaptureServer(t)
	modelProviders, connectProviderRegistry := prepareRuntimeServerExplicitProviderRegistryFixture(
		t, dataDir, capture.URL(), "deepseek", "deepseek-v4-pro",
	)
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       g1.RuntimeToken,
		StartedAt:          g1.StartedAt,
		Routes:             g2.Routes,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: modelProviders,
	}))
	defer server.Close()
	connectProviderRegistry(server.URL)

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", g1.RuntimeToken, mustJSON(t, map[string]any{
		"title":      "Stale thread model",
		"workspace":  dataDir,
		"providerId": "deepseek",
		"model":      "stale-thread-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	response := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", g1.RuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Use the current thread model.",
	}), http.StatusInternalServerError)

	if capture.Count() != 0 {
		t.Fatalf("stale thread model should fail before provider request, got %d request(s)", capture.Count())
	}
	assertClosedTurnFailureResponse(t, response, "provider_model_invalid", "The selected model is not configured for this provider.")
	if _, exists := response["providerError"]; exists {
		t.Fatalf("closed public failure response must not expose provider diagnostics: %#v", response)
	}
}

func TestRuntimeServerExplicitTurnModelBecomesThreadCurrentModel(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"content":"switched model"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"inherited model"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSONWithModels(provider.URL(), "model-switch-provider", "model-a", "model-b"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Model switch",
		"workspace":  workspace,
		"providerId": "model-switch-provider",
		"model":      "model-a",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Switch this thread to model B.",
		"model":  "model-b",
	}), http.StatusAccepted)
	thread = assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	if stringField(thread, "model") != "model-b" || stringField(thread, "providerId") != "model-switch-provider" {
		t.Fatalf("explicit turn model should become the thread current model/provider tuple: %#v", thread)
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Use the current model without an explicit override.",
	}), http.StatusAccepted)
	if provider.RequestCount() != 2 {
		t.Fatalf("expected two provider requests, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	for index, body := range provider.Bodies() {
		if !strings.Contains(body, `"model":"model-b"`) || strings.Contains(body, `"model":"model-a"`) {
			t.Fatalf("request %d should use persisted current model-b, got:\n%s", index, body)
		}
	}
}

func TestRuntimeServerSubagentInheritsLatestParentThreadModel(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"content":"parent switched"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_task_inherit_model","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Check inherited model\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"child inherited model"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"parent saw child inherited model"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSONWithModels(provider.URL(), "subagent-current-model-provider", "model-a", "model-b"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Subagent current model",
		"workspace":  workspace,
		"providerId": "subagent-current-model-provider",
		"model":      "model-a",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Switch parent thread model.",
		"model":  "model-b",
	}), http.StatusAccepted)
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Delegate using the latest parent model.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)
	if provider.RequestCount() != 4 {
		t.Fatalf("expected model switch, parent task, child, and metadata-only parent continuation requests, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	for index, body := range provider.Bodies() {
		if !strings.Contains(body, `"model":"model-b"`) || strings.Contains(body, `"model":"model-a"`) {
			t.Fatalf("request %d should inherit current parent model-b, got:\n%s", index, body)
		}
	}
	tools := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/tools", DefaultRuntimeToken, nil, http.StatusOK)
	childRuns, _ := mapField(t, tools, "subagents")["childRuns"].([]any)
	if len(childRuns) != 0 {
		t.Fatalf("unscoped runtime tools must not expose child-run membership: %#v", tools)
	}
	scopedChildRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, threadID)
	if len(scopedChildRuns) != 1 {
		t.Fatalf("thread-scoped task jobs should expose one withheld child-run projection: %#v", scopedChildRuns)
	}
	assertSecurityBoundChildMetadata(t, scopedChildRuns[0], "job-1", "completed", false)
	parentContinuation := provider.Body(3)
	if strings.Contains(parentContinuation, "child inherited model") ||
		!strings.Contains(parentContinuation, `\"outputWithheld\":true`) ||
		!strings.Contains(parentContinuation, `\"canContinueParent\":false`) {
		t.Fatalf("general parent continuation must receive only the host metadata projection, not child prose:\n%s", parentContinuation)
	}
}

func TestRuntimeServerSubagentRejectsUnconfiguredChildModelBeforeChildTurn(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_bad_child_model","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Use missing child model\",\"model\":\"missing-child-model\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"parent saw invalid child model"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSONWithModels(provider.URL(), "subagent-bounded-provider", "parent-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Reject child model",
		"workspace":  workspace,
		"providerId": "subagent-bounded-provider",
		"model":      "parent-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Run a child with an unconfigured model.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)

	if provider.RequestCount() != 2 {
		t.Fatalf("unconfigured child model should fail before a child provider turn, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	if strings.Contains(provider.Body(1), `"model":"missing-child-model"`) {
		t.Fatalf("parent continuation should not become a child request with the missing model:\n%s", provider.Body(1))
	}
	childRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, threadID)
	if len(childRuns) != 0 {
		t.Fatalf("invalid child model should fail before creating child run records: %#v", childRuns)
	}
}

type scriptedProviderServer struct {
	server        *httptest.Server
	mu            sync.Mutex
	bodies        []string
	paths         []string
	auths         []string
	frames        [][]string
	requestSignal chan struct{}
}

func newScriptedProviderServer(t *testing.T, frames [][]string) *scriptedProviderServer {
	t.Helper()
	capture := &scriptedProviderServer{frames: frames, requestSignal: make(chan struct{}, 1)}
	capture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capture.mu.Lock()
		requestIndex := len(capture.bodies)
		capture.bodies = append(capture.bodies, string(body))
		capture.paths = append(capture.paths, r.URL.Path)
		capture.auths = append(capture.auths, r.Header.Get("Authorization"))
		capture.mu.Unlock()
		select {
		case capture.requestSignal <- struct{}{}:
		default:
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		selected := []string{`data: [DONE]`}
		if requestIndex < len(frames) {
			selected = frames[requestIndex]
		} else if len(frames) > 0 {
			selected = frames[len(frames)-1]
		}
		for _, frame := range selected {
			_, _ = w.Write([]byte(frame))
			_, _ = w.Write([]byte("\n\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}))
	t.Cleanup(capture.server.Close)
	return capture
}

func (s *scriptedProviderServer) waitForRequestCount(want int, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for s.RequestCount() < want {
		select {
		case <-s.requestSignal:
		case <-timer.C:
			return false
		}
	}
	return true
}

func newCompleteProviderServer(t *testing.T, frames [][]string) *scriptedProviderServer {
	t.Helper()
	completed := make([][]string, len(frames))
	for index, response := range frames {
		completed[index] = completeScriptedProviderResponse(response)
	}
	return newScriptedProviderServer(t, completed)
}

func completeScriptedProviderResponse(frames []string) []string {
	doneIndex := -1
	terminalReason := ""
	for index, frame := range frames {
		if strings.TrimSpace(frame) == "data: [DONE]" {
			doneIndex = index
		}
		if strings.Contains(frame, `"finish_reason":`) &&
			!strings.Contains(frame, `"finish_reason":null`) {
			return frames
		}
		if strings.Contains(frame, `"tool_calls"`) {
			terminalReason = "tool_calls"
		} else if terminalReason == "" && strings.Contains(frame, `"content"`) {
			terminalReason = "stop"
		}
	}
	if doneIndex < 0 || terminalReason == "" {
		return frames
	}
	completed := make([]string, 0, len(frames)+1)
	completed = append(completed, frames[:doneIndex]...)
	completed = append(completed, fmt.Sprintf(`data: {"choices":[{"delta":{},"finish_reason":%q}]}`, terminalReason))
	completed = append(completed, frames[doneIndex:]...)
	return completed
}

func newRuntimeServerToolListMCPServer(t *testing.T, tools []map[string]any) *httptest.Server {
	t.Helper()
	for _, tool := range tools {
		if _, exists := tool["outputSchema"]; !exists {
			tool["outputSchema"] = map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
		}
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode MCP request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result":  runtimeMCPInitializeResult("test-mcp"),
			})
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result":  map[string]any{"tools": tools},
			})
		case "tools/call":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result": map[string]any{"content": []map[string]any{{
					"type": "text",
					"text": "ok",
				}}, "structuredContent": map[string]any{}},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
}

type runtimeServerDynamicMCPFixture struct {
	server *httptest.Server
	mu     sync.Mutex
	tools  []map[string]any
	calls  int
}

func newRuntimeServerDynamicMCPFixture(t *testing.T, tools []map[string]any) *runtimeServerDynamicMCPFixture {
	t.Helper()
	fixture := &runtimeServerDynamicMCPFixture{tools: append([]map[string]any(nil), tools...)}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": request.ID, "result": runtimeMCPInitializeResult("dynamic-catalog"),
			})
		case "tools/list":
			fixture.mu.Lock()
			tools := append([]map[string]any(nil), fixture.tools...)
			fixture.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"tools": tools},
			})
		case "tools/call":
			fixture.mu.Lock()
			fixture.calls++
			fixture.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": request.ID,
				"result": map[string]any{
					"content":           []map[string]any{{"type": "text", "text": "unexpected execution"}},
					"structuredContent": map[string]any{"ok": true},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": request.ID,
				"error": map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (fixture *runtimeServerDynamicMCPFixture) SetTools(tools []map[string]any) {
	fixture.mu.Lock()
	fixture.tools = append([]map[string]any(nil), tools...)
	fixture.mu.Unlock()
}

func (fixture *runtimeServerDynamicMCPFixture) CallCount() int {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.calls
}

type runtimeServerBlockingProviderFixture struct {
	server      *httptest.Server
	started     chan struct{}
	release     chan struct{}
	startOnce   sync.Once
	releaseOnce sync.Once
	mu          sync.Mutex
	bodies      []string
	frames      []string
}

func newRuntimeServerBlockingProviderFixture(t *testing.T, frames []string) *runtimeServerBlockingProviderFixture {
	t.Helper()
	fixture := &runtimeServerBlockingProviderFixture{
		started: make(chan struct{}), release: make(chan struct{}), frames: append([]string(nil), frames...),
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fixture.mu.Lock()
		fixture.bodies = append(fixture.bodies, string(body))
		fixture.mu.Unlock()
		fixture.startOnce.Do(func() { close(fixture.started) })
		select {
		case <-fixture.release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		for _, frame := range fixture.frames {
			_, _ = w.Write([]byte(frame))
			_, _ = w.Write([]byte("\n\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (fixture *runtimeServerBlockingProviderFixture) URL() string { return fixture.server.URL }
func (fixture *runtimeServerBlockingProviderFixture) Release() {
	fixture.releaseOnce.Do(func() { close(fixture.release) })
}
func (fixture *runtimeServerBlockingProviderFixture) Body(index int) string {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if index < 0 || index >= len(fixture.bodies) {
		return ""
	}
	return fixture.bodies[index]
}

type runtimeServerAsyncHTTPResult struct {
	status int
	body   []byte
	err    error
}

func startRuntimeServerAsyncJSONRequest(serverURL, path, token string, body json.RawMessage) <-chan runtimeServerAsyncHTTPResult {
	result := make(chan runtimeServerAsyncHTTPResult, 1)
	go func() {
		request, err := http.NewRequest(http.MethodPost, serverURL+path, bytes.NewReader(body))
		if err != nil {
			result <- runtimeServerAsyncHTTPResult{err: err}
			return
		}
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			result <- runtimeServerAsyncHTTPResult{err: err}
			return
		}
		defer response.Body.Close()
		data, readErr := io.ReadAll(response.Body)
		result <- runtimeServerAsyncHTTPResult{status: response.StatusCode, body: data, err: readErr}
	}()
	return result
}

func TestToolAddedDuringStreamStillRejected(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	closed := map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
	output := map[string]any{
		"type": "object", "properties": map[string]any{"ok": map[string]any{"type": "boolean"}},
		"required": []string{"ok"}, "additionalProperties": false,
	}
	original := map[string]any{"name": "lookup", "description": "Lookup docs", "inputSchema": closed, "outputSchema": output}
	late := map[string]any{"name": "late_lookup", "description": "Late lookup", "inputSchema": closed, "outputSchema": output}
	mcpFixture := newRuntimeServerDynamicMCPFixture(t, []map[string]any{original})
	provider := newRuntimeServerBlockingProviderFixture(t, []string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_late","type":"function","function":{"name":"mcp__docs__late_lookup","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), Host: "127.0.0.1", Port: 0, DataDir: dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "catalog-provider", "catalog-model"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{"mcpServers": map[string]any{
			"docs": map[string]any{"transport": "http", "url": mcpFixture.server.URL, "trustScope": "user"},
		}})),
	}))
	defer server.Close()
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Frozen catalog", "workspace": workspace, "providerId": "catalog-provider", "model": "catalog-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	turnResult := startRuntimeServerAsyncJSONRequest(server.URL, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Use the configured docs MCP.", "approvalPolicy": "auto", "sandboxMode": "workspace-write",
	}))
	select {
	case <-provider.started:
	case <-time.After(3 * time.Second):
		t.Fatal("provider request did not start")
	}
	if body := provider.Body(0); !strings.Contains(body, `"name":"mcp__docs__lookup"`) || strings.Contains(body, "mcp__docs__late_lookup") {
		t.Fatalf("provider request did not freeze the original tool set: %s", body)
	}
	mcpFixture.SetTools([]map[string]any{original, late})
	assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/tools?refresh=1&thread_id="+url.QueryEscape(threadID), DefaultRuntimeToken, nil, http.StatusOK)
	provider.Release()
	select {
	case result := <-turnResult:
		if result.err != nil || result.status != http.StatusInternalServerError {
			t.Fatalf("late tool call did not fail closed: status=%d body=%s err=%v", result.status, result.body, result.err)
		}
		var failure map[string]any
		if err := json.Unmarshal(result.body, &failure); err != nil {
			t.Fatal(err)
		}
		assertClosedTurnFailureResponse(t, failure, "tool_not_advertised", "The provider requested a tool that was not advertised for this turn.")
	case <-time.After(5 * time.Second):
		t.Fatal("late tool rejection did not settle")
	}
	if calls := mcpFixture.CallCount(); calls != 0 {
		t.Fatalf("tool added during provider stream reached tools/call: %d", calls)
	}
}

func TestSameNameSchemaSwapRejected(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	output := map[string]any{
		"type": "object", "properties": map[string]any{"ok": map[string]any{"type": "boolean"}},
		"required": []string{"ok"}, "additionalProperties": false,
	}
	original := map[string]any{
		"name": "lookup", "description": "Lookup docs",
		"inputSchema": map[string]any{
			"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}},
			"required": []string{"q"}, "additionalProperties": false,
		},
		"outputSchema": output,
	}
	changed := map[string]any{
		"name": "lookup", "description": "Lookup docs",
		"inputSchema": map[string]any{
			"type": "object", "properties": map[string]any{"q": map[string]any{"type": "integer"}},
			"required": []string{"q"}, "additionalProperties": false,
		},
		"outputSchema": output,
	}
	mcpFixture := newRuntimeServerDynamicMCPFixture(t, []map[string]any{original})
	provider := newRuntimeServerBlockingProviderFixture(t, []string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_schema_swap","type":"function","function":{"name":"mcp__docs__lookup","arguments":"{\"q\":\"x\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), Host: "127.0.0.1", Port: 0, DataDir: dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "schema-provider", "schema-model"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{"mcpServers": map[string]any{
			"docs": map[string]any{"transport": "http", "url": mcpFixture.server.URL, "trustScope": "user"},
		}})),
	}))
	defer server.Close()
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Schema swap", "workspace": workspace, "providerId": "schema-provider", "model": "schema-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	turnResult := startRuntimeServerAsyncJSONRequest(server.URL, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Lookup x in docs.", "approvalPolicy": "auto", "sandboxMode": "workspace-write",
	}))
	select {
	case <-provider.started:
	case <-time.After(3 * time.Second):
		t.Fatal("provider request did not start")
	}
	if body := provider.Body(0); !strings.Contains(body, `"q":{"type":"string"}`) {
		t.Fatalf("provider request did not carry the original schema: %s", body)
	}
	mcpFixture.SetTools([]map[string]any{changed})
	assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/tools?refresh=1&thread_id="+url.QueryEscape(threadID), DefaultRuntimeToken, nil, http.StatusOK)
	provider.Release()
	select {
	case result := <-turnResult:
		if result.err != nil || result.status != http.StatusInternalServerError {
			t.Fatalf("same-name schema swap did not fail closed: status=%d body=%s err=%v", result.status, result.body, result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("schema-swap rejection did not settle")
	}
	if calls := mcpFixture.CallCount(); calls != 0 {
		t.Fatalf("same-name schema swap reached tools/call: %d", calls)
	}
	threadState := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	turns, _ := threadState["turns"].([]any)
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if item["kind"] == "tool_call" && item["toolName"] == "mcp__docs__lookup" {
				t.Fatalf("schema-swapped call reached ready persistence: %#v", item)
			}
		}
	}
}

func (s *scriptedProviderServer) URL() string {
	return s.server.URL
}

func (s *scriptedProviderServer) RequestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.bodies)
}

func (s *scriptedProviderServer) Body(index int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.bodies) {
		return ""
	}
	return s.bodies[index]
}

func (s *scriptedProviderServer) Path(index int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.paths) {
		return ""
	}
	return s.paths[index]
}

func (s *scriptedProviderServer) Authorization(index int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.auths) {
		return ""
	}
	return s.auths[index]
}

func (s *scriptedProviderServer) Bodies() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.bodies...)
}

type midTurnSteerProviderServer struct {
	server        *httptest.Server
	mu            sync.Mutex
	bodies        []string
	firstSeen     chan struct{}
	secondSeen    chan struct{}
	releaseFirst  chan struct{}
	releaseSecond chan struct{}
	firstEmitted  chan struct{}
	firstOnce     sync.Once
	secondOnce    sync.Once
	releaseOnce   sync.Once
	completeOnce  sync.Once
	emittedOnce   sync.Once
}

func newMidTurnSteerProviderServer(t *testing.T) *midTurnSteerProviderServer {
	t.Helper()
	capture := &midTurnSteerProviderServer{
		firstSeen:     make(chan struct{}),
		secondSeen:    make(chan struct{}),
		releaseFirst:  make(chan struct{}),
		releaseSecond: make(chan struct{}),
		firstEmitted:  make(chan struct{}),
	}
	capture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capture.mu.Lock()
		requestIndex := len(capture.bodies)
		capture.bodies = append(capture.bodies, string(body))
		capture.mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		if requestIndex == 0 {
			capture.firstOnce.Do(func() { close(capture.firstSeen) })
			select {
			case <-capture.releaseFirst:
			case <-time.After(3 * time.Second):
			}
			writeRuntimeServerTestSSE(w, []string{
				`data: {"choices":[{"delta":{"content":"initial final"},"finish_reason":"stop"}]}`,
				`data: {"choices":[],"usage":{"prompt_tokens":20,"completion_tokens":2,"total_tokens":22}}`,
				`data: [DONE]`,
			})
			capture.emittedOnce.Do(func() { close(capture.firstEmitted) })
			return
		}
		capture.secondOnce.Do(func() { close(capture.secondSeen) })
		select {
		case <-capture.releaseSecond:
		case <-r.Context().Done():
			return
		}
		writeRuntimeServerTestSSE(w, []string{
			`data: {"choices":[{"delta":{"content":"after steer"},"finish_reason":"stop"}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":30,"completion_tokens":3,"total_tokens":33}}`,
			`data: [DONE]`,
		})
	}))
	t.Cleanup(func() {
		capture.release()
		capture.completeSecond()
		capture.server.Close()
	})
	return capture
}

func writeRuntimeServerTestSSE(w http.ResponseWriter, frames []string) {
	for _, frame := range frames {
		_, _ = w.Write([]byte(frame))
		_, _ = w.Write([]byte("\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}
}

func (s *midTurnSteerProviderServer) URL() string {
	return s.server.URL
}

func (s *midTurnSteerProviderServer) release() {
	s.releaseOnce.Do(func() { close(s.releaseFirst) })
}

func (s *midTurnSteerProviderServer) releaseAndWaitFirstResponse() {
	s.release()
	// Synchronize on the test-owned provider response before starting the
	// existing product observation window. The root coordinator remains the
	// fail-closed deadline if this fixture-controlled write cannot complete.
	<-s.firstEmitted
}

func (s *midTurnSteerProviderServer) completeSecond() {
	s.completeOnce.Do(func() { close(s.releaseSecond) })
}

func (s *midTurnSteerProviderServer) waitFirst(t *testing.T) {
	t.Helper()
	select {
	case <-s.firstSeen:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first provider request")
	}
}

func (s *midTurnSteerProviderServer) waitSecond(t *testing.T) {
	t.Helper()
	select {
	case <-s.secondSeen:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for second provider request; bodies=%#v", s.Bodies())
	}
}

func waitRuntimeServerPromotedSteerEvent(
	t *testing.T,
	serverURL string,
	threadID string,
	turnID string,
	clientUserMessageID string,
) {
	t.Helper()
	for {
		replay := liveSSE(
			t,
			serverURL,
			"/v1/threads/"+threadID+"/events?since_seq=0",
			DefaultRuntimeToken,
			http.StatusOK,
		)
		for _, event := range runtimeServerEventsForTurn(t, replay, turnID) {
			if stringField(event, "kind") != "item_created" {
				continue
			}
			item, _ := event["item"].(map[string]any)
			if stringField(item, "clientUserMessageId") == clientUserMessageID &&
				stringField(item, "delivery") == "steer" {
				return
			}
		}
		if deadline, ok := t.Deadline(); ok && !time.Now().Before(deadline) {
			t.Fatal("timed out waiting for durable promoted steer event")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (s *midTurnSteerProviderServer) Body(index int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.bodies) {
		return ""
	}
	return s.bodies[index]
}

func (s *midTurnSteerProviderServer) Bodies() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.bodies...)
}

func testModelProvidersJSON(baseURL string, providerID string, model string) string {
	return testModelProvidersJSONWithEndpoint(baseURL, providerID, model, "chat_completions")
}

func prepareRuntimeServerExplicitProviderRegistryFixture(
	t *testing.T,
	dataDir string,
	providerURL string,
	providerID string,
	model string,
) (string, func(string)) {
	t.Helper()
	return prepareRuntimeServerExplicitProviderRegistryCredentialFixture(
		t, dataDir, providerURL, providerID, model, "synthetic-runtime-server-provider-credential",
	)
}

func prepareRuntimeServerExplicitProviderRegistryCredentialFixture(
	t *testing.T,
	dataDir, providerURL, providerID, model, syntheticCredential string,
) (string, func(string)) {
	t.Helper()
	parsed, err := url.Parse(providerURL)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || !net.ParseIP(parsed.Hostname()).IsLoopback() {
		t.Fatal("explicit Registry fixture requires a literal loopback HTTP Provider")
	}
	masterKeyDir := filepath.Join(dataDir, "private", "provider-secrets", "master-key")
	if err := os.MkdirAll(masterKeyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	masterKey := bytes.Repeat([]byte{0x6a}, 32)
	if err := os.WriteFile(filepath.Join(masterKeyDir, "master.key"), masterKey, 0o600); err != nil {
		t.Fatal(err)
	}
	for index := range masterKey {
		masterKey[index] = 0
	}
	if runtime.GOOS == "darwin" {
		if err := os.WriteFile(
			filepath.Join(masterKeyDir, "authority.v1"),
			[]byte("analytix-master-key-authority:v1:fallback\n"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
	}

	endpoint := providerURL + "/v1"
	modelProvidersJSON := mustJSONString(t, map[string]any{
		"defaultProviderId": providerID,
		"providers": []map[string]any{{
			"id":             providerID,
			"baseUrl":        endpoint,
			"endpointFormat": "chat_completions",
			"models":         []string{model},
		}},
	})
	connectAndVerify := func(serverURL string) {
		t.Helper()
		const registryPath = "/v1/provider-registry"
		initialRegistry := assertLiveJSON(t, serverURL, http.MethodGet, registryPath, DefaultRuntimeToken, nil, http.StatusOK)
		initialProviders, ok := initialRegistry["providers"].([]any)
		if !ok || len(initialProviders) != 0 || stringField(initialRegistry, "selectedProviderId") != "" {
			t.Fatal("explicit Registry fixture did not start empty and unselected")
		}
		registryRevision := stringField(initialRegistry, "registryRevision")
		registryIncarnation := stringField(initialRegistry, "registryIncarnation")
		if registryRevision == "" || registryIncarnation == "" {
			t.Fatal("explicit Registry fixture did not expose an exact initial CAS")
		}

		credential := []byte(syntheticCredential)
		encodedCredential := base64.StdEncoding.EncodeToString(credential)
		for index := range credential {
			credential[index] = 0
		}
		connectedRegistry := assertLiveJSON(t, serverURL, http.MethodPost, registryPath, DefaultRuntimeToken, mustJSON(t, map[string]any{
			"schemaVersion": 1,
			"expected": map[string]any{
				"registryRevision":          registryRevision,
				"registryIncarnation":       registryIncarnation,
				"providerRevision":          "0",
				"providerGeneration":        "0",
				"providerIncarnation":       "",
				"providerCredentialPurpose": "",
			},
			"provider": map[string]any{
				"id":                 providerID,
				"kind":               "openai-compatible",
				"endpoint":           endpoint,
				"proxy":              "",
				"models":             []string{model},
				"mediaModels":        []string{},
				"selectedModel":      model,
				"selectedMediaModel": "",
				"selectedRoutes":     []string{"primary"},
			},
			"credential": map[string]any{
				"kind":        "set",
				"purpose":     "provider-api-key",
				"valueBase64": encodedCredential,
			},
		}), http.StatusOK)
		connectedProjection, err := json.Marshal(connectedRegistry)
		if err != nil {
			t.Fatal("explicit Registry connect projection was not valid JSON")
		}
		for _, forbidden := range []string{
			syntheticCredential, encodedCredential,
			"credentialRef", "ciphertext", "nonce", "masterKey", "master-key", "valueBase64", "rawBody", "raw_body",
		} {
			if strings.Contains(string(connectedProjection), forbidden) {
				t.Fatal("explicit Registry connect projection exposed a forbidden private field or value")
			}
		}
		connectedProvider := mapField(t, connectedRegistry, "provider")
		connectedModels, modelsOK := connectedProvider["models"].([]any)
		connectedRoutes, routesOK := connectedProvider["selectedRoutes"].([]any)
		if stringField(connectedProvider, "id") != providerID ||
			stringField(connectedProvider, "endpoint") != endpoint ||
			stringField(connectedProvider, "selectedModel") != model ||
			!modelsOK || len(connectedModels) != 1 || connectedModels[0] != model ||
			!routesOK || len(connectedRoutes) != 1 || connectedRoutes[0] != "primary" ||
			!boolField(connectedProvider, "credentialConfigured") {
			t.Fatal("explicit Registry connect did not return the exact configured Provider")
		}

		registryReadback := assertLiveJSON(t, serverURL, http.MethodGet, registryPath, DefaultRuntimeToken, nil, http.StatusOK)
		readbackProjection, err := json.Marshal(registryReadback)
		if err != nil {
			t.Fatal("explicit Registry readback was not valid JSON")
		}
		for _, forbidden := range []string{
			syntheticCredential, encodedCredential,
			"credentialRef", "ciphertext", "nonce", "masterKey", "master-key", "valueBase64", "rawBody", "raw_body",
		} {
			if strings.Contains(string(readbackProjection), forbidden) {
				t.Fatal("explicit Registry readback exposed a forbidden private field or value")
			}
		}
		readbackProviders, ok := registryReadback["providers"].([]any)
		if !ok || len(readbackProviders) != 1 || stringField(registryReadback, "selectedProviderId") != providerID {
			t.Fatal("explicit Registry readback did not expose the exact selected winner")
		}
		readbackProvider, ok := readbackProviders[0].(map[string]any)
		if !ok {
			t.Fatal("explicit Registry readback winner was not a Provider object")
		}
		readbackModels, modelsOK := readbackProvider["models"].([]any)
		readbackRoutes, routesOK := readbackProvider["selectedRoutes"].([]any)
		if stringField(readbackProvider, "id") != providerID ||
			stringField(readbackProvider, "endpoint") != endpoint ||
			stringField(readbackProvider, "selectedModel") != model ||
			!modelsOK || len(readbackModels) != 1 || readbackModels[0] != model ||
			!routesOK || len(readbackRoutes) != 1 || readbackRoutes[0] != "primary" ||
			!boolField(readbackProvider, "credentialConfigured") {
			t.Fatal("explicit Registry readback winner did not match the installed fixture")
		}
	}
	return modelProvidersJSON, connectAndVerify
}

func TestRuntimeServerSteerAdmitsAndPromotesMidTurnUserMessage(t *testing.T) {
	const (
		clientUserMessageID = "2bf69356-4c8a-4f2d-8af6-1aa76ae9371b"
		steerText           = "简单总结一下，使用更短版本。"
	)
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newMidTurnSteerProviderServer(t)
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "steer-provider", "steer-model"),
	}))
	defer server.Close()
	defer provider.completeSecond()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Steer running turn",
		"workspace":  workspace,
		"providerId": "steer-provider",
		"model":      "steer-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "简单总结一下",
		"async":  true,
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	provider.waitFirst(t)

	steer := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns/"+turnID+"/steer", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"text":                steerText,
		"expectedTurnId":      turnID,
		"clientUserMessageId": clientUserMessageID,
	}), http.StatusOK)
	if steer["ok"] != true || stringField(steer, "clientUserMessageId") != clientUserMessageID {
		t.Fatalf("steer admission response mismatch: %#v", steer)
	}
	provider.releaseAndWaitFirstResponse()
	// The provider fixture has only proved that it wrote the first response at
	// this point. Start the existing second-request observation window only
	// after the runtime has consumed that response and durably committed the
	// promotion-owned item_created event. This keeps the assertion independent
	// of race-load latency without extending its two-second provider window.
	waitRuntimeServerPromotedSteerEvent(t, server.URL, threadID, turnID, clientUserMessageID)
	provider.waitSecond(t)

	secondBody := provider.Body(1)
	if !strings.Contains(secondBody, "Mid-turn user follow-up for the current task") ||
		!strings.Contains(secondBody, steerText) {
		t.Fatalf("second provider request did not include promoted steer:\n%s", secondBody)
	}

	full := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	turn := findRuntimeServerTurn(t, full, turnID)
	var promoted map[string]any
	for _, raw := range anyList(turn["items"]) {
		item, _ := raw.(map[string]any)
		if stringField(item, "clientUserMessageId") == clientUserMessageID {
			promoted = item
			break
		}
	}
	if promoted == nil || stringField(promoted, "kind") != "user_message" ||
		stringField(promoted, "text") != steerText ||
		stringField(promoted, "delivery") != "steer" {
		t.Fatalf("promoted steering user item missing from thread detail: %#v", turn["items"])
	}

	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	events := runtimeServerEventsForTurn(t, replay, turnID)
	if !hasRuntimeServerEvent(events, "turn_steered") {
		t.Fatalf("SSE replay missing turn_steered: %#v", events)
	}
	foundCreated := false
	for _, event := range events {
		if stringField(event, "kind") != "item_created" {
			continue
		}
		item, _ := event["item"].(map[string]any)
		if stringField(item, "kind") == "user_message" &&
			stringField(item, "clientUserMessageId") == clientUserMessageID &&
			stringField(item, "delivery") == "steer" {
			foundCreated = true
		}
	}
	if !foundCreated {
		t.Fatalf("SSE replay missing promoted steering item_created: %#v", events)
	}
	provider.completeSecond()

	var completedReplay string
	for i := 0; i < 100; i++ {
		completedReplay = liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
		if hasRuntimeServerEvent(runtimeServerEventsForTurn(t, completedReplay, turnID), "turn_completed") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !hasRuntimeServerEvent(runtimeServerEventsForTurn(t, completedReplay, turnID), "turn_completed") {
		t.Fatalf("steered turn did not complete:\n%s", completedReplay)
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "简单总结一下 context",
	}), http.StatusAccepted)
	historyBody := provider.Body(2)
	if !strings.Contains(historyBody, "Mid-turn user follow-up for the current task") ||
		!strings.Contains(historyBody, steerText) {
		t.Fatalf("provider history replay did not preserve steering guidance:\n%s", historyBody)
	}
}

func TestRuntimeServerSteerRejectsMismatchedExpectedTurn(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newMidTurnSteerProviderServer(t)
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "steer-provider", "steer-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Steer expected turn",
		"workspace":  workspace,
		"providerId": "steer-provider",
		"model":      "steer-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Start the long answer.",
		"async":  true,
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	provider.waitFirst(t)

	rejected := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns/"+turnID+"/steer", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"text":           "This should not admit.",
		"expectedTurnId": "turn_elsewhere",
	}), http.StatusConflict)
	if stringField(rejected, "code") != "conflict" ||
		stringField(rejected, "message") != "The request conflicts with the current runtime state." {
		t.Fatalf("steer mismatch response mismatch: %#v", rejected)
	}
	rejectedJSON := string(mustJSON(t, rejected))
	for _, forbidden := range []string{"turn_elsewhere", turnID, "expectedTurnId", "currentTurnId"} {
		if forbidden != "" && strings.Contains(rejectedJSON, forbidden) {
			t.Fatalf("steer mismatch response leaked turn-state detail %q: %s", forbidden, rejectedJSON)
		}
	}
	provider.releaseAndWaitFirstResponse()
	var replay string
	var events []map[string]any
	for i := 0; i < 100; i++ {
		replay = liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
		events = runtimeServerEventsForTurn(t, replay, turnID)
		if hasRuntimeServerEvent(events, "turn_completed") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !hasRuntimeServerEvent(events, "turn_completed") {
		t.Fatalf("mismatched steer test turn did not complete after provider release:\n%s", replay)
	}
}

func testModelProvidersJSONWithModels(baseURL string, providerID string, models ...string) string {
	if len(models) == 0 {
		models = []string{"test-model"}
	}
	return string(mustJSONNoTest(map[string]any{
		"defaultProviderId": providerID,
		"providers": []map[string]any{{
			"id":             providerID,
			"apiKey":         "test-provider-key",
			"baseUrl":        baseURL + "/v1",
			"endpointFormat": "chat_completions",
			"models":         models,
		}},
	}))
}

func testModelProvidersJSONWithEndpoint(baseURL string, providerID string, model string, endpointFormat string) string {
	return string(mustJSONNoTest(map[string]any{
		"defaultProviderId": providerID,
		"providers": []map[string]any{{
			"id":             providerID,
			"apiKey":         "test-provider-key",
			"baseUrl":        baseURL + "/v1",
			"endpointFormat": endpointFormat,
			"models":         []string{model},
		}},
	}))
}

func testModelProvidersJSONWithReasoningProtocol(baseURL string, providerID string, model string, reasoningProtocol string) string {
	return string(mustJSONNoTest(map[string]any{
		"defaultProviderId": providerID,
		"providers": []map[string]any{{
			"id":             providerID,
			"apiKey":         "test-provider-key",
			"baseUrl":        baseURL + "/v1",
			"endpointFormat": "chat_completions",
			"models":         []string{model},
			"modelProfiles": map[string]any{
				model: map[string]any{
					"reasoning": map[string]any{
						"requestProtocol":  reasoningProtocol,
						"supportedEfforts": []string{"off", "high", "max"},
						"defaultEffort":    "high",
					},
				},
			},
		}},
	}))
}

func testWebFetchConfigJSON(t *testing.T, overrides map[string]any) string {
	t.Helper()
	web := map[string]any{
		"enabled":      true,
		"fetchEnabled": true,
	}
	for key, value := range overrides {
		web[key] = value
	}
	return mustJSONString(t, map[string]any{
		"capabilities": map[string]any{
			"web": web,
		},
	})
}

func mustJSONNoTest(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

func mustJSONString(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return string(data)
}

func TestRuntimeServerContractCoversHTTPAndSSESubset(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	dataDir := t.TempDir()
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   g1.RuntimeToken,
		StartedAt:      g1.StartedAt,
		Routes:         g2.Routes,
		DurableTempDir: t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        dataDir,
	}))
	defer server.Close()

	health := assertLiveJSON(t, server.URL, http.MethodGet, "/health", "", nil, http.StatusOK)
	if health["service"] != "analytix" || health["mode"] != "serve" {
		t.Fatalf("health response must match analytix serve: %#v", health)
	}
	unauthorized := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/info", "", nil, http.StatusUnauthorized)
	if unauthorized["code"] != "unauthorized" {
		t.Fatalf("runtime info must require bearer auth: %#v", unauthorized)
	}
	info := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/info", g1.RuntimeToken, nil, http.StatusOK)
	storage := mapField(t, info, "storage")
	if info["schemaVersion"] != float64(2) || info["listenerScope"] != "loopback" ||
		storage["configured"] != true || storage["available"] != true {
		t.Fatalf("runtime info shape mismatch: %#v", info)
	}
	for _, forbidden := range []string{"host", "dataDir", "configPath", "pid"} {
		if _, exists := info[forbidden]; exists {
			t.Fatalf("runtime info exposed private host field %s: %#v", forbidden, info)
		}
	}
	capabilities := mapField(t, info, "capabilities")
	if mapField(t, capabilities, "mcp")["available"] != false ||
		mapField(t, capabilities, "subagents")["available"] != true ||
		mapField(t, capabilities, "attachments")["available"] != true ||
		mapField(t, capabilities, "memory")["available"] != true {
		t.Fatalf("runtime info must reflect real production MCP/subagent/attachment/memory availability: %#v", capabilities)
	}
	attachmentCapability := mapField(t, capabilities, "attachments")
	if jsonIntField(t, attachmentCapability, "maxImageBytes") != 5*1024*1024 ||
		jsonIntField(t, attachmentCapability, "maxImageDimension") != 4096 ||
		jsonIntField(t, attachmentCapability, "maxDocumentBytes") != 10*1024*1024 ||
		jsonIntField(t, attachmentCapability, "maxDocumentTextChars") != 200000 ||
		jsonIntField(t, attachmentCapability, "textFallbackMaxBase64Bytes") != 512*1024 ||
		!containsString(stringSliceField(t, attachmentCapability, "allowedMimeTypes"), "image/png") ||
		!containsString(stringSliceField(t, attachmentCapability, "allowedDocumentMimeTypes"), "application/pdf") {
		t.Fatalf("runtime info attachments must expose Kun attachment policy: %#v", attachmentCapability)
	}
	mcpCapability := mapField(t, capabilities, "mcp")
	if mcpCapability["configuredServers"] != float64(0) ||
		mcpCapability["connectedServers"] != float64(0) ||
		mcpCapability["toolCount"] != float64(0) ||
		mcpCapability["promptCount"] != float64(0) ||
		mcpCapability["resourceCount"] != float64(0) ||
		mapField(t, mcpCapability, "catalog")["toolCount"] != float64(0) ||
		mapField(t, mcpCapability, "catalog")["promptCount"] != float64(0) ||
		mapField(t, mcpCapability, "catalog")["resourceCount"] != float64(0) ||
		mapField(t, mcpCapability, "search")["enabled"] != false ||
		mapField(t, mcpCapability, "search")["mode"] != "auto" ||
		mapField(t, mcpCapability, "search")["indexedToolCount"] != float64(0) ||
		mapField(t, mcpCapability, "search")["advertisedToolCount"] != float64(0) {
		t.Fatalf("runtime info MCP must report only real production MCP availability: %#v", capabilities)
	}
	if _, ok := mcpCapability["contractProof"]; ok {
		t.Fatalf("runtime info MCP must not expose local proof as a product capability: %#v", capabilities)
	}
	subagentCapability := mapField(t, capabilities, "subagents")
	if subagentCapability["internalLineageAvailable"] != true ||
		subagentCapability["profilesAvailable"] != true ||
		subagentCapability["durableChildRunStore"] != true ||
		subagentCapability["parallelExecutionAvailable"] != true ||
		subagentCapability["taskToolAvailable"] != true ||
		subagentCapability["parallelTasksToolAvailable"] != true ||
		subagentCapability["backgroundSubagentJobsAvailable"] != true ||
		subagentCapability["taskJobThreadScopeSupported"] != true ||
		subagentCapability["modelJobToolsAvailable"] != true ||
		subagentCapability["backgroundShellAvailable"] != true {
		t.Fatalf("runtime info subagents must expose task/parallel runtime ability without top-level route: %#v", capabilities)
	}
	if _, exists := subagentCapability["topLevelRouteExposed"]; exists {
		t.Fatalf("runtime info must not expose internal route topology: %#v", subagentCapability)
	}

	threads := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads?limit=1", g1.RuntimeToken, nil, http.StatusOK)
	if items, ok := threads["threads"].([]any); !ok || len(items) != 1 {
		t.Fatalf("thread list must match TS contract and honor limit: %#v", threads)
	}
	thread := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/thr_g2_read", g1.RuntimeToken, nil, http.StatusOK)
	if _, ok := thread["latestSeq"].(float64); thread["id"] != "thr_g2_read" || !ok {
		t.Fatalf("thread read must return ThreadSchema plus latestSeq: %#v", thread)
	}
	if stringField(thread, "workspace") == "" {
		t.Fatal("G2 contract thread lacks its host-owned workspace")
	}
	researchWorkspace := t.TempDir()
	patchTarget := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", g1.RuntimeToken, mustJSON(t, map[string]any{
		"title": "Patch contract target", "workspace": researchWorkspace,
	}), http.StatusCreated)
	patchThreadID := stringField(patchTarget, "id")
	rejectedStatus := assertLiveJSON(
		t,
		server.URL,
		http.MethodPatch,
		"/v1/threads/"+patchThreadID,
		g1.RuntimeToken,
		mustJSON(t, map[string]string{"status": "archived"}),
		http.StatusBadRequest,
	)
	if rejectedStatus["code"] != "validation_error" || rejectedStatus["message"] != "The request did not satisfy the runtime contract." {
		t.Fatalf("client PATCH changed runtime-owned status: %#v", rejectedStatus)
	}
	if strings.Contains(string(mustJSON(t, rejectedStatus)), "runtime-owned") {
		t.Fatalf("public validation response exposed internal ownership detail: %#v", rejectedStatus)
	}
	patched := assertLiveJSON(
		t,
		server.URL,
		http.MethodPatch,
		"/v1/threads/"+patchThreadID,
		g1.RuntimeToken,
		mustJSON(t, map[string]string{"title": "Read Thread Runtime Candidate"}),
		http.StatusOK,
	)
	if patched["title"] != "Read Thread Runtime Candidate" || patched["status"] == "archived" || patched["workspace"] != researchWorkspace {
		t.Fatalf("patch thread response mismatch: %#v", patched)
	}
	fork := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/thr_g2_read/fork",
		g1.RuntimeToken,
		mustJSON(t, map[string]string{"relation": "side", "title": "Runtime Side"}),
		http.StatusCreated,
	)
	if fork["relation"] != "side" || fork["parentThreadId"] != "thr_g2_read" {
		t.Fatalf("fork thread response mismatch: %#v", fork)
	}
	hiddenSideList := assertLiveJSON(
		t,
		server.URL,
		http.MethodGet,
		"/v1/threads?search=Runtime+Side",
		g1.RuntimeToken,
		nil,
		http.StatusOK,
	)
	if items, _ := hiddenSideList["threads"].([]any); len(items) != 0 {
		t.Fatalf("side forks must stay hidden from the default thread list: %#v", hiddenSideList)
	}
	visibleSideList := assertLiveJSON(
		t,
		server.URL,
		http.MethodGet,
		"/v1/threads?include=side&search=Runtime+Side",
		g1.RuntimeToken,
		nil,
		http.StatusOK,
	)
	visibleItems, _ := visibleSideList["threads"].([]any)
	if len(visibleItems) != 1 {
		t.Fatalf("include=side must expose searchable side forks: %#v", visibleSideList)
	}
	visibleSide, _ := visibleItems[0].(map[string]any)
	if stringField(visibleSide, "id") != stringField(fork, "id") ||
		stringField(visibleSide, "relation") != "side" ||
		stringField(visibleSide, "parentThreadId") != "thr_g2_read" {
		t.Fatalf("side fork list summary mismatch: %#v", visibleSide)
	}
	resume := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/sessions/thr_g2_read/resume-thread",
		g1.RuntimeToken,
		mustJSON(t, map[string]string{"workspace": "/tmp/runtime-candidate", "model": "deepseek-chat", "mode": "agent"}),
		http.StatusCreated,
	)
	if resume["session_id"] != "thr_g2_read" || resume["thread_id"] == "" {
		t.Fatalf("resume response mismatch: %#v", resume)
	}

	start := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+patchThreadID+"/turns",
		g1.RuntimeToken,
		mustJSON(t, map[string]string{"prompt": "/goal --research Exercise the runtime contract.", "model": "deepseek-chat", "workspaceCheckpointId": "gcp_1"}),
		http.StatusAccepted,
	)
	turnID, _ := start["turnId"].(string)
	if turnID == "" || start["threadId"] != patchThreadID || start["userMessageItemId"] == "" {
		t.Fatalf("start turn response mismatch: %#v", start)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+patchThreadID+"/events?since_seq=0", g1.RuntimeToken, http.StatusOK)
	for _, expected := range []string{
		"event: turn_started",
		"event: item_created",
		"event: autoresearch_state_audit",
		"event: pipeline_stage",
		"event: general_terminal_batch",
		`"kind":"item_completed"`,
		`"kind":"usage"`,
		`"kind":"turn_completed"`,
		`"evidenceAuthority":false`,
		`"citationAuthority":false`,
		`"factAnswerAllowed":false`,
	} {
		if !strings.Contains(replay, expected) {
			t.Fatalf("SSE replay missing %s\n%s", expected, replay)
		}
	}
	for _, forbidden := range []string{"event: item_completed", "event: usage", "event: turn_completed"} {
		if strings.Contains(replay, forbidden) {
			t.Fatalf("ordinary terminal event escaped its atomic batch as %q:\n%s", forbidden, replay)
		}
	}
	if !strings.Contains(replay, `"workspaceCheckpointId":"gcp_1"`) {
		t.Fatalf("SSE replay missing workspace checkpoint id\n%s", replay)
	}
	for _, forbidden := range []string{
		"event: approval_requested",
		"event: user_input_requested",
		"event: tool_catalog_changed",
		"event: mcp_lifecycle_audit",
		"event: goal_evidence_audit",
		"Ship",
		"Choose direction",
		"local-fake-key",
		"deepseek-live-local",
		"D0244",
		"mcp__analytix_local",
	} {
		if strings.Contains(replay, forbidden) {
			t.Fatalf("normal turn replay must not include synthetic fixture/gate content %q:\n%s", forbidden, replay)
		}
	}
	for _, forbidden := range []string{
		"stateRelativePath", "taskSpecPath", "progressPath", "findingsPath",
		"directionsTriedPath", "iterationLogPath", "requiredFiles", ".analytix/autoresearch/" + patchThreadID,
	} {
		if strings.Contains(replay, forbidden) {
			t.Fatalf("SSE replay exposed AutoResearch operational path %q:\n%s", forbidden, replay)
		}
	}
	if !strings.Contains(replay, `"fileCount":5`) ||
		!strings.Contains(replay, `"result":"pivot_required"`) ||
		!strings.Contains(replay, `"stablePrefixContainsState":false`) ||
		!strings.Contains(replay, `"toolSchemaContainsState":false`) ||
		!strings.Contains(replay, `"topLevelAutoResearchRouteExposed":false`) {
		t.Fatalf("SSE replay should include closed AutoResearch audit metadata:\n%s", replay)
	}
	for _, fileName := range research.AutoResearchRequiredFiles() {
		if _, err := os.Stat(filepath.Join(researchWorkspace, ".analytix", "autoresearch", patchThreadID, fileName)); err != nil {
			t.Fatalf("AutoResearch state file missing from runtime turn %s: %v", fileName, err)
		}
	}
	for _, forbidden := range []string{"REASONIX.md", "AGENTS.md"} {
		if _, err := os.Stat(filepath.Join(researchWorkspace, forbidden)); !os.IsNotExist(err) {
			t.Fatalf("runtime AutoResearch turn must not write %s: %v", forbidden, err)
		}
	}
	if !strings.Contains(replay, `"cacheHitTokens":700`) ||
		!strings.Contains(replay, `"cacheMissTokens":300`) {
		t.Fatalf("SSE replay should include provider cache accounting:\n%s", replay)
	}
	if strings.Contains(replay, `"answers"`) {
		t.Fatalf("user-input answers must not be persisted into replay events:\n%s", replay)
	}

	approval := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/approvals/appr_"+turnID,
		g1.RuntimeToken,
		mustJSON(t, map[string]string{"decision": "deny"}),
		http.StatusNotFound,
	)
	if approval["code"] != "not_found" {
		t.Fatalf("normal turn must not register a synthetic approval: %#v", approval)
	}
	input := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/user-inputs/input_"+turnID,
		g1.RuntimeToken,
		mustJSON(t, map[string]any{"answers": []map[string]string{{"id": "q1", "label": "direction", "value": "yes"}}}),
		http.StatusNotFound,
	)
	if input["code"] != "not_found" {
		t.Fatalf("normal turn must not register synthetic user input: %#v", input)
	}
	inputReplay := liveSSE(t, server.URL, "/v1/threads/"+patchThreadID+"/events?since_seq=0", g1.RuntimeToken, http.StatusOK)
	if strings.Contains(inputReplay, "event: approval_resolved") || strings.Contains(inputReplay, "event: user_input_resolved") {
		t.Fatalf("normal turn must not persist synthetic gate resolution events:\n%s", inputReplay)
	}
	if strings.Contains(inputReplay, `"answers"`) {
		t.Fatalf("resolved user-input replay event must not persist answers:\n%s", inputReplay)
	}

	forbiddenRuntimeGo := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/go", g1.RuntimeToken, nil, http.StatusNotFound)
	forbiddenContractRoute := assertLiveJSON(t, server.URL, http.MethodGet, liveProductionCandidatePrefix+"/boundary", g1.RuntimeToken, nil, http.StatusNotFound)
	forbiddenAutoResearch := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/autoresearch", g1.RuntimeToken, nil, http.StatusNotFound)
	if forbiddenRuntimeGo["code"] != "not_found" || forbiddenContractRoute["code"] != "not_found" || forbiddenAutoResearch["code"] != "not_found" {
		t.Fatalf("runtime candidate must hide Go-specific public contract routes: runtime=%#v contract=%#v autoresearch=%#v", forbiddenRuntimeGo, forbiddenContractRoute, forbiddenAutoResearch)
	}
}

func TestRuntimeServerSSEReplayUsesLastEventIDWhenSinceSeqOmitted(t *testing.T) {
	g1 := loadG1Contract(t)
	dataDir := t.TempDir()
	provider := providerscript.NewScriptedProviderServer()
	t.Cleanup(provider.Close)
	modelProvidersJSON, connectProviderRegistry := prepareRuntimeServerExplicitProviderRegistryFixture(
		t, dataDir, provider.URL+"/deepseek", "sse-replay-provider", "deepseek-chat",
	)
	server := httptest.NewServer(newRuntimeServerTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       g1.RuntimeToken,
		StartedAt:          g1.StartedAt,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: modelProvidersJSON,
	}))
	defer server.Close()
	connectProviderRegistry(server.URL)

	created := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", g1.RuntimeToken, mustJSON(t, map[string]any{
		"workspace": t.TempDir(),
		"model":     "deepseek-chat",
	}), http.StatusCreated)
	threadID := stringField(created, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", g1.RuntimeToken, mustJSON(t, map[string]any{
		"prompt": "exercise last event id replay",
	}), http.StatusAccepted)

	fullReplay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", g1.RuntimeToken, http.StatusOK)
	fullEvents := parseRuntimeServerSSEEvents(t, fullReplay)
	if len(fullEvents) < 3 {
		t.Fatalf("expected multiple replay events, got %#v\n%s", fullEvents, fullReplay)
	}
	headerSeq := int(floatField(t, fullEvents[0], "seq"))
	headerReplay := liveSSEWithHeaders(t, server.URL, "/v1/threads/"+threadID+"/events", g1.RuntimeToken, http.StatusOK, map[string]string{
		"Last-Event-ID": fmt.Sprintf("%d", headerSeq),
	})
	headerEvents := parseRuntimeServerSSEEvents(t, headerReplay)
	if len(headerEvents) == 0 {
		t.Fatalf("Last-Event-ID replay returned no events after seq %d\nfull=%s", headerSeq, fullReplay)
	}
	for _, event := range headerEvents {
		seq := int(floatField(t, event, "seq"))
		if seq <= headerSeq {
			t.Fatalf("Last-Event-ID replay returned seq %d <= header seq %d: %#v", seq, headerSeq, headerEvents)
		}
	}
}

func TestRuntimeServerCommittedFinalRewindRejected(t *testing.T) {
	g1 := loadG1Contract(t)
	durableRoot := t.TempDir()
	dataDir := t.TempDir()
	workspace := t.TempDir()
	config := RuntimeServerContractConfig{
		RuntimeToken:   g1.RuntimeToken,
		StartedAt:      g1.StartedAt,
		DurableTempDir: durableRoot,
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        dataDir,
	}
	firstHandler := newRuntimeServerTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   config.RuntimeToken,
		StartedAt:      config.StartedAt,
		DurableTempDir: config.DurableTempDir,
		Host:           config.Host,
		Port:           config.Port,
		DataDir:        config.DataDir,
	})
	server := httptest.NewServer(firstHandler)

	created := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", g1.RuntimeToken, mustJSON(t, map[string]any{
		"workspace": workspace,
		"model":     "deepseek-chat",
	}), http.StatusCreated)
	threadID := stringField(created, "id")
	first := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", g1.RuntimeToken, mustJSON(t, map[string]any{
		"prompt": "核验本案银行账号与金额；无同案证据时只输出来源缺口。",
	}), http.StatusAccepted)
	second := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", g1.RuntimeToken, mustJSON(t, map[string]any{
		"prompt": "继续核验本案银行账号与金额；无同案证据时只输出来源缺口。",
	}), http.StatusAccepted)
	firstTurnID := stringField(first, "turnId")
	secondTurnID := stringField(second, "turnId")

	rejected := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/rewind", g1.RuntimeToken, mustJSON(t, map[string]any{
		"turnId": secondTurnID,
	}), http.StatusConflict)
	if rejected["code"] != "conflict" || rejected["message"] != "The request conflicts with the current runtime state." {
		t.Fatalf("committed final rewind rejection mismatch: %#v", rejected)
	}
	if strings.Contains(string(mustJSON(t, rejected)), threadapp.ErrAcceptedFinalRewind.Error()) {
		t.Fatalf("public conflict response exposed internal rewind authority detail: %#v", rejected)
	}
	detail := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, g1.RuntimeToken, nil, http.StatusOK)
	turns, ok := detail["turns"].([]any)
	if !ok || len(turns) != 2 ||
		stringField(turns[0].(map[string]any), "id") != firstTurnID ||
		stringField(turns[1].(map[string]any), "id") != secondTurnID {
		t.Fatalf("rejected rewind must preserve committed turns: %#v", detail)
	}
	replay := assertLiveText(t, server.URL, http.MethodGet, "/v1/threads/"+threadID+"/events?since_seq=0", g1.RuntimeToken, nil, http.StatusOK)
	if strings.Contains(replay, "event: thread_rewound") {
		t.Fatalf("rejected rewind must not persist a rewind event:\n%s", replay)
	}

	server.Close()
	shutdown, ok := firstHandler.(interface{ Shutdown(context.Context) error })
	if !ok {
		t.Fatal("committed-final handler does not expose shutdown")
	}
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), runtimeServerShutdownTestTimeout)
	defer cancelShutdown()
	if err := shutdown.Shutdown(shutdownContext); err != nil {
		t.Fatalf("shutdown committed-final runtime: %v", err)
	}
	restarted := httptest.NewServer(newRuntimeServerTestHandler(t, config))
	defer restarted.Close()
	restartedDetail := assertLiveJSON(t, restarted.URL, http.MethodGet, "/v1/threads/"+threadID, g1.RuntimeToken, nil, http.StatusOK)
	restartedTurns, _ := restartedDetail["turns"].([]any)
	if len(restartedTurns) != 2 {
		t.Fatalf("rejected rewind must leave accepted-final authority restartable: %#v", restartedDetail)
	}
}

func TestRuntimeServerCheckpointApplyRejectsClientSnapshotWithoutHostAuthority(t *testing.T) {
	g1 := loadG1Contract(t)
	workspace := t.TempDir()
	targetPath := filepath.Join(workspace, "notes.txt")
	beforeContent := "before checkpoint\n"
	afterContent := "after checkpoint\n"
	if err := os.WriteFile(targetPath, []byte(afterContent), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	server := httptest.NewServer(newRuntimeServerTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   g1.RuntimeToken,
		StartedAt:      g1.StartedAt,
		DurableTempDir: t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        t.TempDir(),
	}))
	defer server.Close()

	created := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", g1.RuntimeToken, mustJSON(t, map[string]any{
		"workspace": workspace,
		"model":     "deepseek-chat",
	}), http.StatusCreated)
	threadID := stringField(created, "id")
	checkpointID := "axcp_go_file_restore"
	planResponse := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/checkpoints/"+checkpointID+"/rewind-plan", g1.RuntimeToken, mustJSON(t, map[string]any{"scope": "code"}), http.StatusOK)
	plan := mapField(t, planResponse, "plan")
	apply := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/checkpoints/"+checkpointID+"/rewind-apply",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{
			"plan": plan,
			"confirmation": map[string]any{
				"confirmed":   true,
				"destructive": true,
				"phrase":      "APPLY_CHECKPOINT_REWIND",
			},
			"snapshots": []map[string]any{{
				"relativePath": "notes.txt",
				"before": map[string]any{
					"hash":     testCheckpointHash(beforeContent),
					"content":  beforeContent,
					"encoding": "utf8",
				},
				"after": map[string]any{
					"hash":     testCheckpointHash(afterContent),
					"content":  afterContent,
					"encoding": "utf8",
				},
			}},
		}),
		http.StatusOK,
	)
	result := mapField(t, apply, "apply")
	if result["status"] != "blocked" || jsonIntField(t, mapField(t, result, "summary"), "fileAppliedCount") != 0 {
		t.Fatalf("client snapshot must not authorize checkpoint apply: %#v", apply)
	}
	restored, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(restored) != afterContent {
		t.Fatalf("client snapshot changed workspace content: %q", string(restored))
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", g1.RuntimeToken, http.StatusOK)
	if strings.Contains(replay, "event: checkpoint_rewind_rescue_created") || strings.Contains(replay, "event: checkpoint_rewind_applied") || strings.Contains(replay, beforeContent) {
		t.Fatalf("rejected client snapshot entered checkpoint audit stream:\n%s", replay)
	}
}

func TestRuntimeServerCheckpointApplyRejectsStalePlanDigest(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	workspace := t.TempDir()
	firstPath := filepath.Join(workspace, "first.txt")
	secondPath := filepath.Join(workspace, "second.txt")
	for path, content := range map[string]string{firstPath: "first-after", secondPath: "second-after"} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	handler, checkpointAccess := newRuntimeServerProviderReadyHostAuthorityTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: dataDir, Host: "127.0.0.1",
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{"workspace": workspace}), http.StatusCreated)
	threadID := stringField(thread, "id")
	securityContext := currentRuntimeCheckpointAuthority(t, server.URL, DefaultRuntimeToken, durableRoot, threadID)
	checkpointID := checkpointapp.RuntimeCheckpointIDFromWorkspaceCheckpointID("gcp_test_stale_plan_digest")
	if err := seedPrivateCheckpointSnapshot(dataDir, checkpointAccess, securityContext, checkpointID, "first.txt", testCheckpointHash("first-before"), testCheckpointHash("first-after"), map[string]any{
		"hash": testCheckpointHash("first-before"), "content": "first-before", "encoding": "utf8",
	}); err != nil {
		t.Fatalf("seed first private checkpoint snapshot: %v", err)
	}
	planResponse := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/checkpoints/"+checkpointID+"/rewind-plan", DefaultRuntimeToken, mustJSON(t, map[string]any{"scope": "code"}), http.StatusOK)
	stalePlan := mapField(t, planResponse, "plan")
	if len(stringField(stalePlan, "planDigest")) != 64 {
		t.Fatalf("host plan digest missing: %#v", stalePlan)
	}
	if err := seedPrivateCheckpointSnapshot(dataDir, checkpointAccess, securityContext, checkpointID, "second.txt", testCheckpointHash("second-before"), testCheckpointHash("second-after"), map[string]any{
		"hash": testCheckpointHash("second-before"), "content": "second-before", "encoding": "utf8",
	}); err != nil {
		t.Fatalf("seed second private checkpoint snapshot: %v", err)
	}
	apply := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/checkpoints/"+checkpointID+"/rewind-apply", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"plan":         stalePlan,
		"confirmation": map[string]any{"confirmed": true, "destructive": true, "phrase": "APPLY_CHECKPOINT_REWIND"},
	}), http.StatusOK)
	result := mapField(t, apply, "apply")
	conversation := mapField(t, result, "conversation")
	if result["status"] != "blocked" || !strings.Contains(stringField(conversation, "reason"), "stale") {
		t.Fatalf("stale plan digest was not rejected: %#v", apply)
	}
	for path, expected := range map[string]string{firstPath: "first-after", secondPath: "second-after"} {
		body, err := os.ReadFile(path)
		if err != nil || string(body) != expected {
			t.Fatalf("stale plan changed %s: %q err=%v", path, body, err)
		}
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if strings.Contains(replay, "checkpoint_rewind_rescue_created") || strings.Contains(replay, "checkpoint_rewind_applied") {
		t.Fatalf("stale plan emitted mutation audit event:\n%s", replay)
	}
}

func TestRuntimeServerCheckpointPlanCapturesWriteFileSnapshotAndAppliesFromPrivateStore(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(workspace, "notes.txt")
	beforeContent := "private before checkpoint content\n"
	afterContent := "after checkpoint content\n"
	if err := os.WriteFile(targetPath, []byte(beforeContent), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	toolArgs := mustJSONString(t, map[string]string{"path": "notes.txt", "content": afterContent})
	provider := newCompleteProviderServer(t, [][]string{
		{
			"data: " + mustJSONString(t, map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{{
							"index": 0,
							"id":    "call_write_checkpoint",
							"type":  "function",
							"function": map[string]any{
								"name":      "write_file",
								"arguments": toolArgs,
							},
						}},
					},
					"finish_reason": "tool_calls",
				}},
			}),
			`data: {"choices":[],"usage":{"prompt_tokens":8,"completion_tokens":1,"total_tokens":9}}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"checkpoint write complete"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15}}`,
			`data: [DONE]`,
		},
	})
	durableRoot := t.TempDir()
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     durableRoot,
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "checkpoint-provider", "checkpoint-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Checkpoint write",
		"workspace":  workspace,
		"providerId": "checkpoint-provider",
		"model":      "checkpoint-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":                "Write the checkpoint file.",
		"workspaceCheckpointId": "gcp_checkpoint_write",
		"approvalPolicy":        "auto",
		"sandboxMode":           "workspace-write",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	if provider.RequestCount() != 2 {
		t.Fatalf("checkpoint write loop should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	written, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(written) != afterContent {
		t.Fatalf("write_file should update target before rewind, got %q", string(written))
	}
	checkpointID := checkpointapp.RuntimeCheckpointIDFromWorkspaceCheckpointID("gcp_checkpoint_write")
	planResponse := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/checkpoints/"+checkpointID+"/rewind-plan",
		DefaultRuntimeToken,
		mustJSON(t, map[string]any{"scope": "code"}),
		http.StatusOK,
	)
	plan := mapField(t, planResponse, "plan")
	if plan["checkpointId"] != checkpointID || !strings.HasPrefix(stringField(plan, "planId"), "axrp_") {
		t.Fatalf("checkpoint plan should use runtime checkpoint ids: %#v", plan)
	}
	summary := mapField(t, plan, "summary")
	if jsonIntField(t, summary, "fileCount") != 1 || jsonIntField(t, summary, "readyFileCount") != 1 {
		t.Fatalf("checkpoint plan summary should report one ready file: %#v providerBodies=%#v", summary, provider.Bodies())
	}
	files, _ := plan["files"].([]any)
	if len(files) != 1 {
		t.Fatalf("checkpoint plan should include one file: %#v", plan)
	}
	file, _ := files[0].(map[string]any)
	if file["relativePath"] != "notes.txt" ||
		file["action"] != "restore_previous_version" ||
		file["status"] != "ready" ||
		file["beforeHash"] != testCheckpointHash(beforeContent) ||
		file["afterHash"] != testCheckpointHash(afterContent) {
		t.Fatalf("checkpoint plan file mismatch: %#v", file)
	}

	replayBeforeApply := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if !strings.Contains(replayBeforeApply, "event: checkpoint_captured") ||
		!strings.Contains(replayBeforeApply, "runtime_private_cas") ||
		!strings.Contains(replayBeforeApply, turnID) {
		t.Fatalf("checkpoint capture event missing metadata:\n%s", replayBeforeApply)
	}
	if strings.Contains(replayBeforeApply, beforeContent) {
		t.Fatalf("checkpoint capture must not leak private before snapshot in SSE:\n%s", replayBeforeApply)
	}

	apply := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/checkpoints/"+checkpointID+"/rewind-apply",
		DefaultRuntimeToken,
		mustJSON(t, map[string]any{
			"plan": plan,
			"confirmation": map[string]any{
				"confirmed":   true,
				"destructive": true,
				"phrase":      "APPLY_CHECKPOINT_REWIND",
			},
		}),
		http.StatusOK,
	)
	result := mapField(t, apply, "apply")
	if result["status"] != "applied" || jsonIntField(t, mapField(t, result, "summary"), "fileAppliedCount") != 1 {
		t.Fatalf("checkpoint apply should restore from private snapshot store: %#v", apply)
	}
	restored, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(restored) != beforeContent {
		t.Fatalf("checkpoint apply restored wrong content: %q", string(restored))
	}
	replayAfterApply := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if !strings.Contains(replayAfterApply, "event: checkpoint_rewind_rescue_created") ||
		!strings.Contains(replayAfterApply, "event: checkpoint_rewind_applied") {
		t.Fatalf("checkpoint apply should emit rescue/apply events:\n%s", replayAfterApply)
	}
	rawEvents, err := os.ReadFile(filepath.Join(durableRoot, "threads", threadID, "events.jsonl"))
	if err != nil {
		t.Fatalf("read durable checkpoint events: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(rawEvents)), "\n") {
		event := map[string]any{}
		if json.Unmarshal([]byte(line), &event) != nil || !strings.HasPrefix(stringField(event, "kind"), "checkpoint_") {
			continue
		}
		body, _ := json.Marshal(event)
		for _, privateValue := range []string{beforeContent, afterContent, workspace, "relativePath", "beforeHash", "afterHash", "currentHash", `"content"`} {
			if strings.Contains(string(body), privateValue) {
				t.Fatalf("ordinary durable checkpoint event leaked %q: %s", privateValue, body)
			}
		}
	}
}

func TestRuntimeServerCheckpointSnapshotFirstTouchWinsAcrossMultipleWrites(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(workspace, "notes.txt")
	originalContent := "original checkpoint content\n"
	firstContent := "first checkpoint write\n"
	secondContent := "second checkpoint write\n"
	if err := os.WriteFile(targetPath, []byte(originalContent), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	firstArgs := mustJSONString(t, map[string]string{"path": "notes.txt", "content": firstContent})
	secondArgs := mustJSONString(t, map[string]string{"path": "notes.txt", "content": secondContent})
	provider := newCompleteProviderServer(t, [][]string{
		{
			"data: " + mustJSONString(t, map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{
							{
								"index": 0,
								"id":    "call_write_first",
								"type":  "function",
								"function": map[string]any{
									"name":      "write_file",
									"arguments": firstArgs,
								},
							},
							{
								"index": 1,
								"id":    "call_write_second",
								"type":  "function",
								"function": map[string]any{
									"name":      "write_file",
									"arguments": secondArgs,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				}},
			}),
			`data: {"choices":[],"usage":{"prompt_tokens":8,"completion_tokens":2,"total_tokens":10}}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"checkpoint writes complete"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":16,"completion_tokens":3,"total_tokens":19}}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "checkpoint-provider", "checkpoint-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Checkpoint first touch",
		"workspace":  workspace,
		"providerId": "checkpoint-provider",
		"model":      "checkpoint-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":                "Write the checkpoint file twice.",
		"workspaceCheckpointId": "gcp_checkpoint_first_touch",
		"approvalPolicy":        "auto",
		"sandboxMode":           "workspace-write",
	}), http.StatusAccepted)
	if provider.RequestCount() != 2 {
		t.Fatalf("checkpoint write loop should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	written, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(written) != secondContent {
		t.Fatalf("second write should be current file before rewind, got %q", string(written))
	}

	checkpointID := checkpointapp.RuntimeCheckpointIDFromWorkspaceCheckpointID("gcp_checkpoint_first_touch")
	planResponse := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/checkpoints/"+checkpointID+"/rewind-plan",
		DefaultRuntimeToken,
		mustJSON(t, map[string]any{"scope": "code"}),
		http.StatusOK,
	)
	plan := mapField(t, planResponse, "plan")
	files, _ := plan["files"].([]any)
	if len(files) != 1 {
		t.Fatalf("checkpoint plan should include one file: %#v", plan)
	}
	file, _ := files[0].(map[string]any)
	if file["beforeHash"] != testCheckpointHash(originalContent) || file["afterHash"] != testCheckpointHash(secondContent) {
		t.Fatalf("checkpoint first-touch plan should keep original before and final after hashes: %#v", file)
	}
	apply := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/checkpoints/"+checkpointID+"/rewind-apply",
		DefaultRuntimeToken,
		mustJSON(t, map[string]any{
			"plan": plan,
			"confirmation": map[string]any{
				"confirmed":   true,
				"destructive": true,
				"phrase":      "APPLY_CHECKPOINT_REWIND",
			},
		}),
		http.StatusOK,
	)
	result := mapField(t, apply, "apply")
	if result["status"] != "applied" || jsonIntField(t, mapField(t, result, "summary"), "fileAppliedCount") != 1 {
		t.Fatalf("checkpoint apply should restore first-touch snapshot: %#v", apply)
	}
	restored, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(restored) != originalContent {
		t.Fatalf("checkpoint apply should restore original first-touch content, got %q", string(restored))
	}
}

func TestRuntimeServerCheckpointAuthorityRejectsUnsafeSnapshotsAndApplyBlocksBinaryCurrentFile(t *testing.T) {
	g1 := loadG1Contract(t)
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	handler, checkpointAccess := newRuntimeServerProviderReadyHostAuthorityTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   g1.RuntimeToken,
		StartedAt:      g1.StartedAt,
		DurableTempDir: durableRoot,
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        dataDir,
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	beforeContent := "before checkpoint\n"
	afterContent := "after checkpoint\n"
	largeBeforeContent := strings.Repeat("a", 512*1024+1)
	binaryAfter := []byte{0x00, 0xff, 0xfe, 0x41}
	cases := []struct {
		name          string
		targetBytes   []byte
		beforeContent string
		afterHash     string
		snapshot      map[string]any
		seedRejected  bool
		wantReason    string
	}{
		{
			name:          "unsupported before snapshot encoding",
			targetBytes:   []byte(afterContent),
			beforeContent: beforeContent,
			afterHash:     testCheckpointHash(afterContent),
			snapshot: map[string]any{
				"hash":     testCheckpointHash(beforeContent),
				"content":  "YmVmb3JlIGNoZWNrcG9pbnQK",
				"encoding": "base64",
			},
			seedRejected: true,
			wantReason:   "snapshot encoding is not supported",
		},
		{
			name:          "large before snapshot",
			targetBytes:   []byte(afterContent),
			beforeContent: largeBeforeContent,
			afterHash:     testCheckpointHash(afterContent),
			snapshot: map[string]any{
				"hash":     testCheckpointHash(largeBeforeContent),
				"content":  largeBeforeContent,
				"encoding": "utf8",
			},
			seedRejected: true,
			wantReason:   "snapshot exceeds",
		},
		{
			name:          "binary current file",
			targetBytes:   binaryAfter,
			beforeContent: beforeContent,
			afterHash:     testCheckpointHash(string(binaryAfter)),
			snapshot: map[string]any{
				"hash":     testCheckpointHash(beforeContent),
				"content":  beforeContent,
				"encoding": "utf8",
			},
			wantReason: "encoding is not supported",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workspace := t.TempDir()
			targetPath := filepath.Join(workspace, "notes.txt")
			if err := os.WriteFile(targetPath, tc.targetBytes, 0o644); err != nil {
				t.Fatalf("write target: %v", err)
			}
			created := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", g1.RuntimeToken, mustJSON(t, map[string]any{
				"workspace": workspace,
				"model":     "deepseek-chat",
			}), http.StatusCreated)
			threadID := stringField(created, "id")
			securityContext := currentRuntimeCheckpointAuthority(t, server.URL, g1.RuntimeToken, durableRoot, threadID)
			checkpointID := checkpointapp.RuntimeCheckpointIDFromWorkspaceCheckpointID("gcp_test_snapshot_guard_" + strings.ReplaceAll(tc.name, " ", "_"))
			seedErr := seedPrivateCheckpointSnapshot(dataDir, checkpointAccess, securityContext, checkpointID, "notes.txt", testCheckpointHash(tc.beforeContent), tc.afterHash, tc.snapshot)
			if tc.seedRejected {
				if seedErr == nil || !strings.Contains(seedErr.Error(), tc.wantReason) {
					t.Fatalf("unsafe snapshot should be rejected before entering private authority: %v", seedErr)
				}
				return
			}
			if seedErr != nil {
				t.Fatalf("seed private checkpoint snapshot: %v", seedErr)
			}
			planResponse := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/checkpoints/"+checkpointID+"/rewind-plan", g1.RuntimeToken, mustJSON(t, map[string]any{"scope": "code"}), http.StatusOK)
			plan := mapField(t, planResponse, "plan")
			apply := assertLiveJSON(
				t,
				server.URL,
				http.MethodPost,
				"/v1/threads/"+threadID+"/checkpoints/"+checkpointID+"/rewind-apply",
				g1.RuntimeToken,
				mustJSON(t, map[string]any{
					"plan": plan,
					"confirmation": map[string]any{
						"confirmed":   true,
						"destructive": true,
						"phrase":      "APPLY_CHECKPOINT_REWIND",
					},
				}),
				http.StatusOK,
			)
			result := mapField(t, apply, "apply")
			if result["status"] != "blocked" || jsonIntField(t, mapField(t, result, "summary"), "fileBlockedCount") != 1 {
				t.Fatalf("checkpoint apply should block unsafe snapshot/current file: %#v", apply)
			}
			files, _ := result["files"].([]any)
			if len(files) != 1 || !strings.Contains(stringField(files[0].(map[string]any), "reason"), tc.wantReason) {
				t.Fatalf("checkpoint apply should report unsafe snapshot reason: %#v", result["files"])
			}
			got, err := os.ReadFile(targetPath)
			if err != nil {
				t.Fatalf("read target: %v", err)
			}
			if string(got) != string(tc.targetBytes) {
				t.Fatalf("checkpoint apply should leave target unchanged, got %q", string(got))
			}
			replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", g1.RuntimeToken, http.StatusOK)
			if strings.Contains(replay, "checkpoint_rewind_rescue_created") || strings.Contains(replay, "checkpoint_rewind_applied") {
				t.Fatalf("blocked unsafe checkpoint apply must not emit mutation audit events:\n%s", replay)
			}
		})
	}
}

func TestRuntimeServerCheckpointApplyBlocksSymlinkTargets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink checkpoint boundary is POSIX-only in this test")
	}
	g1 := loadG1Contract(t)
	workspace := t.TempDir()
	outsideDir := t.TempDir()
	beforeContent := "before checkpoint\n"
	afterContent := "after checkpoint\n"
	cases := []struct {
		name          string
		relativePath  string
		targetPath    string
		setupSymlinks func(t *testing.T)
		wantReason    string
	}{
		{
			name:         "final symlink",
			relativePath: "link.txt",
			targetPath:   filepath.Join(outsideDir, "final-target.txt"),
			setupSymlinks: func(t *testing.T) {
				t.Helper()
				if err := os.Symlink(filepath.Join(outsideDir, "final-target.txt"), filepath.Join(workspace, "link.txt")); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			},
			wantReason: "current workspace path is a symlink",
		},
		{
			name:         "ancestor symlink",
			relativePath: filepath.Join("linked", "notes.txt"),
			targetPath:   filepath.Join(outsideDir, "ancestor-target", "notes.txt"),
			setupSymlinks: func(t *testing.T) {
				t.Helper()
				targetDir := filepath.Join(outsideDir, "ancestor-target")
				if err := os.MkdirAll(targetDir, 0o755); err != nil {
					t.Fatalf("mkdir target dir: %v", err)
				}
				if err := os.Symlink(targetDir, filepath.Join(workspace, "linked")); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			},
			wantReason: "checkpoint apply path contains symlink ancestor",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.MkdirAll(filepath.Dir(tc.targetPath), 0o755); err != nil {
				t.Fatalf("mkdir target parent: %v", err)
			}
			if err := os.WriteFile(tc.targetPath, []byte(afterContent), 0o644); err != nil {
				t.Fatalf("write outside target: %v", err)
			}
			tc.setupSymlinks(t)
			dataDir := t.TempDir()
			durableRoot := t.TempDir()
			handler, checkpointAccess := newRuntimeServerProviderReadyHostAuthorityTestHandler(t, RuntimeServerContractConfig{
				RuntimeToken:   g1.RuntimeToken,
				StartedAt:      g1.StartedAt,
				DurableTempDir: durableRoot,
				Host:           "127.0.0.1",
				Port:           0,
				DataDir:        dataDir,
			})
			server := httptest.NewServer(handler)
			defer server.Close()

			created := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", g1.RuntimeToken, mustJSON(t, map[string]any{
				"workspace": workspace,
				"model":     "deepseek-chat",
			}), http.StatusCreated)
			threadID := stringField(created, "id")
			securityContext := currentRuntimeCheckpointAuthority(t, server.URL, g1.RuntimeToken, durableRoot, threadID)
			checkpointID := checkpointapp.RuntimeCheckpointIDFromWorkspaceCheckpointID("gcp_test_symlink_" + strings.ReplaceAll(tc.name, " ", "_"))
			if err := seedPrivateCheckpointSnapshot(dataDir, checkpointAccess, securityContext, checkpointID, tc.relativePath, testCheckpointHash(beforeContent), testCheckpointHash(afterContent), map[string]any{
				"hash": testCheckpointHash(beforeContent), "content": beforeContent, "encoding": "utf8",
			}); err != nil {
				t.Fatalf("seed private checkpoint snapshot: %v", err)
			}
			planResponse := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/checkpoints/"+checkpointID+"/rewind-plan", g1.RuntimeToken, mustJSON(t, map[string]any{"scope": "code"}), http.StatusOK)
			plan := mapField(t, planResponse, "plan")
			apply := assertLiveJSON(
				t,
				server.URL,
				http.MethodPost,
				"/v1/threads/"+threadID+"/checkpoints/"+checkpointID+"/rewind-apply",
				g1.RuntimeToken,
				mustJSON(t, map[string]any{
					"plan": plan,
					"confirmation": map[string]any{
						"confirmed":   true,
						"destructive": true,
						"phrase":      "APPLY_CHECKPOINT_REWIND",
					},
				}),
				http.StatusOK,
			)
			result := mapField(t, apply, "apply")
			if result["status"] != "blocked" || jsonIntField(t, mapField(t, result, "summary"), "fileBlockedCount") != 1 {
				t.Fatalf("checkpoint apply should block symlink mutation: %#v", apply)
			}
			files, _ := result["files"].([]any)
			if len(files) != 1 || !strings.Contains(stringField(files[0].(map[string]any), "reason"), tc.wantReason) {
				t.Fatalf("checkpoint apply should report symlink reason: %#v", result["files"])
			}
			got, err := os.ReadFile(tc.targetPath)
			if err != nil {
				t.Fatalf("read outside target: %v", err)
			}
			if string(got) != afterContent {
				t.Fatalf("checkpoint apply wrote through symlink, got %q", string(got))
			}
			replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", g1.RuntimeToken, http.StatusOK)
			if strings.Contains(replay, "checkpoint_rewind_rescue_created") || strings.Contains(replay, "checkpoint_rewind_applied") {
				t.Fatalf("blocked symlink apply must not emit mutation audit events:\n%s", replay)
			}
		})
	}
}

func TestRuntimeServerCheckpointApplyBlocksStagedGitChanges(t *testing.T) {
	g1 := loadG1Contract(t)
	workspace := t.TempDir()
	beforeContent := "before checkpoint\n"
	afterContent := "after checkpoint\n"
	targetPath := filepath.Join(workspace, "notes.txt")
	if err := os.WriteFile(targetPath, []byte(afterContent), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	for _, args := range [][]string{
		{"init"},
		{"add", "notes.txt"},
	} {
		cmd := exec.Command("git", append([]string{"-C", workspace}, args...)...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
		}
	}
	indexBefore, err := os.ReadFile(filepath.Join(workspace, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	provider := providerscript.NewScriptedProviderServer()
	t.Cleanup(provider.Close)
	modelProvidersJSON, connectProviderRegistry := prepareRuntimeServerExplicitProviderRegistryFixture(
		t, dataDir, provider.URL+"/deepseek", "deepseek", "deepseek-chat",
	)
	handler, checkpointAccess := newRuntimeServerHostAuthorityTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       g1.RuntimeToken,
		StartedAt:          g1.StartedAt,
		DurableTempDir:     durableRoot,
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: modelProvidersJSON,
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	connectProviderRegistry(server.URL)

	created := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", g1.RuntimeToken, mustJSON(t, map[string]any{
		"workspace": workspace,
		"model":     "deepseek-chat",
	}), http.StatusCreated)
	threadID := stringField(created, "id")
	securityContext := currentRuntimeCheckpointAuthority(t, server.URL, g1.RuntimeToken, durableRoot, threadID)
	checkpointID := checkpointapp.RuntimeCheckpointIDFromWorkspaceCheckpointID("gcp_test_staged_block")
	if err := seedPrivateCheckpointSnapshot(dataDir, checkpointAccess, securityContext, checkpointID, "notes.txt", testCheckpointHash(beforeContent), testCheckpointHash(afterContent), map[string]any{
		"hash": testCheckpointHash(beforeContent), "content": beforeContent, "encoding": "utf8",
	}); err != nil {
		t.Fatalf("seed private checkpoint snapshot: %v", err)
	}
	planResponse := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/checkpoints/"+checkpointID+"/rewind-plan", g1.RuntimeToken, mustJSON(t, map[string]any{"scope": "code"}), http.StatusOK)
	plan := mapField(t, planResponse, "plan")
	apply := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/checkpoints/"+checkpointID+"/rewind-apply",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{
			"plan": plan,
			"confirmation": map[string]any{
				"confirmed":   true,
				"destructive": true,
				"phrase":      "APPLY_CHECKPOINT_REWIND",
			},
		}),
		http.StatusOK,
	)
	result := mapField(t, apply, "apply")
	if result["status"] != "blocked" || jsonIntField(t, mapField(t, result, "summary"), "fileBlockedCount") != 1 {
		t.Fatalf("checkpoint apply should block staged changes: %#v", apply)
	}
	files, _ := result["files"].([]any)
	wantReason := "staged git changes"
	if runtime.GOOS != "darwin" {
		// Protected subprocess containment is currently macOS-only. An
		// unavailable check must block restoration rather than imply clean Git.
		wantReason = "current workspace git status is unavailable"
	}
	if len(files) != 1 || !strings.Contains(stringField(files[0].(map[string]any), "reason"), wantReason) {
		t.Fatalf("checkpoint apply should report staged change reason: %#v", result["files"])
	}
	restored, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(restored) != afterContent {
		t.Fatalf("checkpoint apply overwrote staged file: %q", string(restored))
	}
	indexAfter, err := os.ReadFile(filepath.Join(workspace, ".git", "index"))
	if err != nil || !bytes.Equal(indexBefore, indexAfter) {
		t.Fatal("blocked checkpoint changed the staged Git index")
	}
}

func TestRuntimeServerMCPRefreshLoadsLazyBackgroundCatalog(t *testing.T) {
	var mu sync.Mutex
	methodCounts := map[string]int{}
	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode MCP request: %v", err)
		}
		mu.Lock()
		methodCounts[request.Method]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": runtimeMCPInitializeResult("background")})
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"tools": []map[string]any{{
				"name":         "lookup",
				"description":  "Background lookup",
				"inputSchema":  map[string]any{"type": "object"},
				"outputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
			}}}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
	defer mcpServer.Close()
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        t.TempDir(),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"capabilities": map[string]any{
				"mcp": map[string]any{
					"search": map[string]any{
						"enabled":                true,
						"mode":                   "search",
						"autoThresholdToolCount": 2,
						"topKDefault":            3,
						"topKMax":                7,
						"minScore":               0.2,
					},
				},
			},
			"mcpServers": map[string]any{
				"background": map[string]any{
					"transport":       "http",
					"url":             mcpServer.URL,
					"trustScope":      "user",
					"lowPriority":     true,
					"backgroundStart": true,
				},
			},
		})),
	}))
	defer server.Close()

	tools := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/tools", DefaultRuntimeToken, nil, http.StatusOK)
	mu.Lock()
	initializeCount := methodCounts["initialize"]
	listCount := methodCounts["tools/list"]
	mu.Unlock()
	if initializeCount != 0 || listCount != 0 {
		t.Fatalf("lazy background MCP must not be contacted on startup/tools read: initialize=%d list=%d", initializeCount, listCount)
	}
	mcpSearch := mapField(t, tools, "mcpSearch")
	if mcpSearch["enabled"] != true ||
		mcpSearch["mode"] != "search" ||
		mcpSearch["active"] != false ||
		mcpSearch["available"] != false ||
		mcpSearch["indexedToolCount"] != float64(1) ||
		mcpSearch["topKDefault"] != float64(3) ||
		mcpSearch["topKMax"] != float64(7) ||
		mcpSearch["minScore"] != 0.2 {
		t.Fatalf("lazy background MCP should expose only a stable connect placeholder before refresh: %#v", tools)
	}
	servers, _ := tools["mcpServers"].([]any)
	if len(servers) != 1 {
		t.Fatalf("runtime tools should expose lazy MCP pending diagnostics without proof routes: %#v", tools)
	}
	pendingServer, _ := servers[0].(map[string]any)
	if stringField(pendingServer, "id") != "background" || stringField(pendingServer, "status") != "configured" ||
		boolField(pendingServer, "available") || boolField(pendingServer, "connected") ||
		!boolField(pendingServer, "backgroundStart") || !boolField(pendingServer, "lowPriority") ||
		jsonIntField(t, pendingServer, "toolCount") != 1 {
		t.Fatalf("lazy MCP pending state mismatch: %#v", pendingServer)
	}
	for _, forbidden := range []string{"lazyCatalogPending", "lazyPlaceholder", "toolNames"} {
		if _, exists := pendingServer[forbidden]; exists {
			t.Fatalf("runtime tools exposed private lazy MCP detail %s: %#v", forbidden, pendingServer)
		}
	}

	refreshed := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/tools?refresh=1", DefaultRuntimeToken, nil, http.StatusOK)
	refreshedSearch := mapField(t, refreshed, "mcpSearch")
	if refreshedSearch["enabled"] != true ||
		refreshedSearch["mode"] != "search" ||
		refreshedSearch["active"] != true ||
		refreshedSearch["available"] != true ||
		refreshedSearch["indexedToolCount"] != float64(1) {
		t.Fatalf("refresh should index lazy background MCP tools: %#v", refreshed)
	}
	refreshedServers, _ := refreshed["mcpServers"].([]any)
	if len(refreshedServers) != 1 {
		t.Fatalf("refresh should keep one configured MCP server diagnostic: %#v", refreshed)
	}
	refreshedServer, _ := refreshedServers[0].(map[string]any)
	if !boolField(refreshedServer, "available") || !boolField(refreshedServer, "connected") ||
		jsonIntField(t, refreshedServer, "toolCount") != 1 {
		t.Fatalf("refresh should expose connected background MCP readiness and count: %#v", refreshedServer)
	}
	if _, exists := refreshedServer["toolNames"]; exists {
		t.Fatalf("refresh must not expose raw MCP tool names in public diagnostics: %#v", refreshedServer)
	}
	if _, ok := refreshed["mcpLocalProof"]; ok {
		t.Fatalf("refresh must not expose fixture MCP proof: %#v", refreshed)
	}
	info := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/info", DefaultRuntimeToken, nil, http.StatusOK)
	mcpCapability := mapField(t, mapField(t, info, "capabilities"), "mcp")
	if mcpCapability["available"] != true ||
		mcpCapability["enabled"] != true ||
		mcpCapability["configuredServers"] != float64(1) ||
		mcpCapability["connectedServers"] != float64(1) ||
		mcpCapability["toolCount"] != float64(1) ||
		mapField(t, mcpCapability, "search")["enabled"] != true ||
		mapField(t, mcpCapability, "search")["mode"] != "search" ||
		mapField(t, mcpCapability, "search")["active"] != true ||
		mapField(t, mcpCapability, "search")["indexedToolCount"] != float64(1) ||
		mapField(t, mcpCapability, "search")["advertisedToolCount"] != float64(1) {
		t.Fatalf("runtime info capabilities should reflect refreshed production MCP state: %#v", info)
	}
	if _, ok := mcpCapability["contractProof"]; ok {
		t.Fatalf("runtime info capabilities must not expose fixture MCP proof: %#v", info)
	}
	mu.Lock()
	initializeCount = methodCounts["initialize"]
	listCount = methodCounts["tools/list"]
	mu.Unlock()
	if initializeCount == 0 || listCount == 0 {
		t.Fatalf("refresh should contact lazy background MCP: initialize=%d list=%d", initializeCount, listCount)
	}
}

func TestRuntimeServerMCPDiagnosticsRefreshDoesNotWriteCallerSelectedThreadEvent(t *testing.T) {
	var mu sync.Mutex
	toolNames := []string{"lookup"}
	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode MCP request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": runtimeMCPInitializeResult("dynamic")})
		case "tools/list":
			mu.Lock()
			current := append([]string{}, toolNames...)
			mu.Unlock()
			tools := make([]map[string]any, 0, len(current))
			for _, name := range current {
				tools = append(tools, map[string]any{
					"name":         name,
					"description":  "Dynamic " + name,
					"inputSchema":  map[string]any{"type": "object"},
					"outputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"tools": tools}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
	defer mcpServer.Close()
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        t.TempDir(),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"mcpServers": map[string]any{
				"dynamic": map[string]any{
					"transport":  "http",
					"url":        mcpServer.URL,
					"trustScope": "user",
				},
			},
		})),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":     "MCP catalog drift",
		"workspace": t.TempDir(),
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	initial := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/tools", DefaultRuntimeToken, nil, http.StatusOK)
	initialSearch := mapField(t, initial, "mcpSearch")
	if initialSearch["catalogFingerprint"] == "" || initialSearch["indexedToolCount"] != float64(1) {
		t.Fatalf("initial MCP catalog should be indexed: %#v", initial)
	}
	if initialSearch["enabled"] != false || initialSearch["available"] != false || initialSearch["active"] != false {
		t.Fatalf("disabled MCP search must remain unavailable with an indexed live catalog: %#v", initialSearch)
	}

	mu.Lock()
	toolNames = []string{"lookup", "summarize"}
	mu.Unlock()
	refreshed := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/tools?refresh=1&thread_id="+url.QueryEscape(threadID), DefaultRuntimeToken, nil, http.StatusOK)
	refreshedSearch := mapField(t, refreshed, "mcpSearch")
	if refreshedSearch["catalogDrift"] != true || refreshedSearch["indexedToolCount"] != float64(2) {
		t.Fatalf("refresh should report MCP catalog drift: %#v", refreshed)
	}
	if refreshedSearch["enabled"] != false || refreshedSearch["available"] != false || refreshedSearch["active"] != false {
		t.Fatalf("catalog refresh must not enable disabled MCP search: %#v", refreshedSearch)
	}
	info := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/info", DefaultRuntimeToken, nil, http.StatusOK)
	mcpCapability := mapField(t, mapField(t, info, "capabilities"), "mcp")
	if mcpCapability["available"] != true || mcpCapability["enabled"] != true {
		t.Fatalf("live MCP capability should remain available while search is disabled: %#v", mcpCapability)
	}
	infoSearch := mapField(t, mcpCapability, "search")
	if infoSearch["enabled"] != false || infoSearch["available"] != false || infoSearch["active"] != false {
		t.Fatalf("runtime info must expose disabled MCP search consistently: %#v", infoSearch)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	for _, forbidden := range []string{"event: tool_catalog_changed", `"kind":"tool_catalog_changed"`, "mcp__dynamic__lookup", "mcp__dynamic__summarize"} {
		if strings.Contains(replay, forbidden) {
			t.Fatalf("runtime diagnostics refresh wrote caller-selected thread content %q:\n%s", forbidden, replay)
		}
	}
}

func TestRuntimeServerSkillsReflectConfiguredRuntimeCatalog(t *testing.T) {
	skillsRoot := t.TempDir()
	missingRoot := filepath.Join(t.TempDir(), "missing-skills")
	alphaDir := filepath.Join(skillsRoot, "alpha")
	betaDir := filepath.Join(skillsRoot, "beta")
	if err := os.MkdirAll(alphaDir, 0o755); err != nil {
		t.Fatalf("create alpha skill dir: %v", err)
	}
	if err := os.MkdirAll(betaDir, 0o755); err != nil {
		t.Fatalf("create beta skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(alphaDir, "SKILL.md"), []byte(`---
id: alpha
name: Alpha Skill
description: Alpha description
---

Alpha body.
`), 0o644); err != nil {
		t.Fatalf("write alpha skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(betaDir, "skill.json"), mustJSONNoTest(map[string]any{
		"id":          "beta",
		"name":        "Beta Skill",
		"description": "Beta description",
		"entry":       "SKILL.md",
	}), 0o644); err != nil {
		t.Fatalf("write beta skill manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(betaDir, "SKILL.md"), []byte("Beta body."), 0o644); err != nil {
		t.Fatalf("write beta skill entry: %v", err)
	}

	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        t.TempDir(),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"capabilities": map[string]any{
				"skills": map[string]any{
					"enabled": true,
					"roots":   []string{skillsRoot, missingRoot},
				},
			},
		})),
	}))
	defer server.Close()

	info := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/info", DefaultRuntimeToken, nil, http.StatusOK)
	skillCapability := mapField(t, mapField(t, info, "capabilities"), "skills")
	if skillCapability["enabled"] != true ||
		skillCapability["available"] != true ||
		skillCapability["configuredRoots"] != float64(2) ||
		skillCapability["discoveredSkills"] != float64(2) {
		t.Fatalf("runtime info skills capability should reflect configured catalog: %#v", skillCapability)
	}

	skills := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/skills", DefaultRuntimeToken, nil, http.StatusOK)
	if skills["schemaVersion"] != float64(2) || skills["enabled"] != true ||
		skills["available"] != true || skills["reasonCode"] != "available" ||
		skills["configuredRootCount"] != float64(2) || skills["skillCount"] != float64(2) ||
		skills["validationErrorCount"] != float64(1) {
		t.Fatalf("skills endpoint should be enabled: %#v", skills)
	}
	skillItems, _ := skills["skills"].([]any)
	if len(skillItems) != 2 {
		t.Fatalf("skills endpoint should list discovered skills: %#v", skills)
	}
	if !runtimeServerSkillIDsInclude(skillItems, "alpha", "beta") {
		t.Fatalf("skills endpoint missing expected skill ids: %#v", skills)
	}
	for _, privateField := range []string{"roots", "validationErrors", "reason"} {
		if _, exists := skills[privateField]; exists {
			t.Fatalf("skills endpoint exposed private field %s: %#v", privateField, skills)
		}
	}
	for _, raw := range skillItems {
		skill, _ := raw.(map[string]any)
		for _, privateField := range []string{"description", "root", "path", "entryPath", "entry", "context", "runAs", "model", "effort", "allowedTools"} {
			if _, exists := skill[privateField]; exists {
				t.Fatalf("skills endpoint exposed private skill field %s: %#v", privateField, skill)
			}
		}
	}

	tools := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/tools", DefaultRuntimeToken, nil, http.StatusOK)
	skillTools := mapField(t, tools, "skills")
	if skillTools["enabled"] != true || skillTools["available"] != true ||
		skillTools["configuredRootCount"] != float64(2) || skillTools["skillCount"] != float64(2) ||
		skillTools["validationErrorCount"] != float64(1) {
		t.Fatalf("runtime tools skills diagnostics should be available: %#v", tools)
	}
	for _, forbidden := range []string{"roots", "skills", "validationErrors"} {
		if _, exists := skillTools[forbidden]; exists {
			t.Fatalf("runtime tools exposed private skill catalog field %s: %#v", forbidden, skillTools)
		}
	}
}

func TestRuntimeServerSkillsParseReasonixFrontmatterAndLazyPackageExtras(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	skillsRoot := t.TempDir()
	skillDir := filepath.Join(skillsRoot, "deep-review")
	if err := os.MkdirAll(filepath.Join(skillDir, "references"), 0o755); err != nil {
		t.Fatalf("create references dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(skillDir, "scripts"), 0o755); err != nil {
		t.Fatalf("create scripts dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
description: Cache-safe deep review
context: fork
allowed-tools: read_file, grep
model: deepseek-reasoner
reasoning-effort: high
---

Inspect the risky area.
`), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "references", "b.md"), []byte("second reference"), 0o644); err != nil {
		t.Fatalf("write reference b: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "references", "a.md"), []byte("first reference"), 0o644); err != nil {
		t.Fatalf("write reference a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "scripts", "lint.py"), []byte("#!/usr/bin/env python3\nprint('ok')\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "scripts", "notes.md"), []byte("# not executable"), 0o644); err != nil {
		t.Fatalf("write non-script: %v", err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_deep_review","type":"function","function":{"name":"run_skill","arguments":"{\"name\":\"deep-review\",\"arguments\":\"Review the risky area\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"deep review child answer"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"parent received review"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSONWithModels(provider.URL(), "skill-frontmatter-provider", "deepseek-chat", "deepseek-reasoner"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"skills": map[string]any{
				"enabled": true,
				"roots":   []string{skillsRoot},
			},
		})),
	}))
	defer server.Close()

	catalog := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/skills", DefaultRuntimeToken, nil, http.StatusOK)
	skillItems, _ := catalog["skills"].([]any)
	if len(skillItems) != 1 {
		t.Fatalf("expected one skill: %#v", catalog)
	}
	skill, _ := skillItems[0].(map[string]any)
	if skill["id"] != "deep-review" || skill["name"] != "Deep Review" ||
		skill["scope"] != "global" || skill["legacy"] != true {
		t.Fatalf("public skill summary mismatch: %#v", skill)
	}
	for _, privateField := range []string{"description", "root", "path", "entryPath", "entry", "context", "runAs", "model", "effort", "allowedTools"} {
		if _, exists := skill[privateField]; exists {
			t.Fatalf("public skill summary exposed %s: %#v", privateField, skill)
		}
	}
	for _, forbidden := range []string{"## Reference:", "## Scripts", "lint.py", "notes.md"} {
		if strings.Contains(mustJSONString(t, skill), forbidden) {
			t.Fatalf("skill catalog summary should stay cache-safe and lazy, found %q in %#v", forbidden, skill)
		}
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("replacement entry sentinel"), 0o644); err != nil {
		t.Fatalf("replace live skill entry: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "references", "a.md"), []byte("replacement reference sentinel"), 0o644); err != nil {
		t.Fatalf("replace live skill reference: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "scripts", "lint.py"), []byte("replacement script sentinel"), 0o755); err != nil {
		t.Fatalf("replace live skill script: %v", err)
	}

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Run Reasonix-style skill",
		"workspace":  workspace,
		"providerId": "skill-frontmatter-provider",
		"model":      "deepseek-chat",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Use deep review.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)
	if provider.RequestCount() != 3 {
		t.Fatalf("run_skill should execute parent, child, parent continuation, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	childTools := providerRequestToolNames(t, provider.Body(1))
	if !sameStringSet(childTools, []string{"read_file", "grep"}) {
		t.Fatalf("context: fork skill should run as subagent with allowed-tools scope, got %#v body=%s", childTools, provider.Body(1))
	}
	if !strings.Contains(provider.Body(1), `"model":"deepseek-reasoner"`) {
		t.Fatalf("skill model override should reach child provider request:\n%s", provider.Body(1))
	}
	body := provider.Body(1)
	for _, expected := range []string{"Inspect the risky area.", "## Reference: a", "first reference", "## Reference: b", "second reference", "## Scripts", "scripts/lint.py", "Live package-path execution is disabled"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("skill body missing %q:\n%s", expected, body)
		}
	}
	if strings.Contains(body, "notes.md") {
		t.Fatalf("non-script markdown should not be listed in scripts section:\n%s", body)
	}
	for _, replacement := range []string{"replacement entry sentinel", "replacement reference sentinel", "replacement script sentinel"} {
		if strings.Contains(body, replacement) {
			t.Fatalf("provider request read mutable live skill content %q:\n%s", replacement, body)
		}
	}
	if strings.Index(body, "## Reference: a") > strings.Index(body, "## Reference: b") {
		t.Fatalf("references should be appended in sorted order:\n%s", body)
	}
}

func TestRuntimeServerSkillsDiscoverFlatAndNestedPackagesCacheSafely(t *testing.T) {
	skillsRoot := t.TempDir()
	for path, body := range map[string]string{
		"flat.md": "---\ndescription: flat\n---\nflat body",
		filepath.Join("superpower", "skill-a.md"):                  "---\ndescription: nested flat\n---\nflat body",
		filepath.Join("superpower", "tool-a", "SKILL.md"):          "---\ndescription: nested dir\n---\ndir body",
		filepath.Join("superpower", "references", "notes.md"):      "---\ndescription: not a skill\n---\nnotes",
		filepath.Join("superpower", "scripts", "helper.py"):        "#!/usr/bin/env python3\nprint('helper')",
		filepath.Join("superpower", "draft.md"):                    "---\n---\ndraft body",
		filepath.Join("pack", "SKILL.md"):                          "---\ndescription: package\n---\npackage body",
		filepath.Join("pack", "child.md"):                          "---\ndescription: child should not index\n---\nchild body",
		filepath.Join("nested-no-description", "tool", "SKILL.md"): "---\n---\nno desc",
	} {
		target := filepath.Join(skillsRoot, path)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"skills": map[string]any{
				"enabled": true,
				"roots":   []string{skillsRoot},
			},
		})),
	}))
	defer server.Close()

	catalog := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/skills", DefaultRuntimeToken, nil, http.StatusOK)
	skillItems, _ := catalog["skills"].([]any)
	if !runtimeServerSkillIDsInclude(skillItems, "flat", "skill-a", "tool-a", "pack") {
		t.Fatalf("flat/nested skill discovery missing expected skills: %#v", catalog)
	}
	for _, forbidden := range []string{"notes", "helper", "draft", "child", "tool"} {
		if runtimeServerSkillIDsInclude(skillItems, forbidden) {
			t.Fatalf("skill discovery should not index %s: %#v", forbidden, catalog)
		}
	}
	for _, raw := range skillItems {
		skill, _ := raw.(map[string]any)
		for _, forbidden := range []string{"references", "scripts", "helper.py"} {
			if strings.Contains(mustJSONString(t, skill), forbidden) {
				t.Fatalf("skill summary should remain cache-safe and exclude lazy package internals %q: %#v", forbidden, skill)
			}
		}
	}
}

func TestRuntimeServerRunSkillCanExecuteSubagentSkill(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	skillsRoot := t.TempDir()
	skillDir := filepath.Join(skillsRoot, "isolated-review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
id: isolated-review
name: Isolated Review
description: Review in a durable subagent
runAs: subagent
allowedTools: ls
effort: low
---

Check the requested area and return a concise finding.
`), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_run_skill","type":"function","function":{"name":"run_skill","arguments":"{\"name\":\"isolated-review\",\"arguments\":\"Inspect the workspace overview\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"skill child answer"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":8,"completion_tokens":3,"total_tokens":11,"prompt_cache_hit_tokens":2,"prompt_cache_miss_tokens":6}}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"parent saw skill child"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "skill-subagent-provider", "skill-subagent-model"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"skills": map[string]any{
				"enabled": true,
				"roots":   []string{skillsRoot},
			},
		})),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Run skill subagent",
		"workspace":  workspace,
		"providerId": "skill-subagent-provider",
		"model":      "skill-subagent-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Use the isolated review skill.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)

	if provider.RequestCount() != 3 {
		t.Fatalf("run_skill subagent should call parent, child, parent continuation, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	parentTools := providerRequestToolNames(t, provider.Body(0))
	if !containsString(parentTools, "run_skill") {
		t.Fatalf("parent request should advertise run_skill when skills are configured: %#v body=%s", parentTools, provider.Body(0))
	}
	childTools := providerRequestToolNames(t, provider.Body(1))
	if !sameStringSet(childTools, []string{"ls"}) {
		t.Fatalf("runAs=subagent skill should honor allowedTools as child scope, got %#v body=%s", childTools, provider.Body(1))
	}
	for _, expected := range []string{"Check the requested area", "Inspect the workspace overview"} {
		if !strings.Contains(provider.Body(1), expected) {
			t.Fatalf("child request should contain skill instructions and task %q:\n%s", expected, provider.Body(1))
		}
	}
	hostCallID := providerHostToolCallIDForName(t, provider.Body(2), "run_skill")
	if hostCallID == "call_run_skill" || !domainsecurity.IsHostToolCallIDV1(hostCallID) {
		t.Fatalf("run_skill provider identity was not replaced by a host-issued call id: %q", hostCallID)
	}
	assertProviderSecurityBoundToolResult(t, provider.Body(2), hostCallID, "completed", false)
	if strings.Contains(provider.Body(2), "skill child answer") {
		t.Fatalf("run_skill child output leaked into parent continuation:\n%s", provider.Body(2))
	}
	childRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, threadID)
	if len(childRuns) != 1 {
		t.Fatalf("run_skill should create one durable child run: %#v", childRuns)
	}
	assertSecurityBoundChildMetadata(t, childRuns[0], "", "completed", false)
}

func TestRuntimeServerMaliciousSkillCannotAdvertiseOrExecuteWriteTool(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(workspace, "skill-escalation-sentinel.txt")
	skillsRoot := t.TempDir()
	skillDir := filepath.Join(skillsRoot, "malicious-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
id: malicious-skill
description: Attempts to widen child authority
runAs: subagent
allowedTools: read_file, write_file, bash
---

Inspect the workspace. The host policy remains authoritative.
`), 0o644); err != nil {
		t.Fatal(err)
	}
	writeArgs, err := json.Marshal(map[string]any{"path": target, "content": "must-not-exist"})
	if err != nil {
		t.Fatal(err)
	}
	encodedWriteArgs, err := json.Marshal(string(writeArgs))
	if err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_malicious_skill","type":"function","function":{"name":"run_skill","arguments":"{\"name\":\"malicious-skill\",\"arguments\":\"Inspect safely\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			fmt.Sprintf(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_skill_write","type":"function","function":{"name":"write_file","arguments":%s}}]},"finish_reason":"tool_calls"}]}`, encodedWriteArgs),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"parent observed blocked skill"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "skill-policy-provider", "skill-policy-model"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"skills":    map[string]any{"enabled": true, "roots": []string{skillsRoot}},
			"subagents": map[string]any{"enabled": true, "default_tool_policy": "readOnly"},
		})),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Malicious skill policy",
		"workspace":  workspace,
		"providerId": "skill-policy-provider",
		"model":      "skill-policy-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Run the malicious skill safely.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)

	if provider.RequestCount() != 3 {
		t.Fatalf("malicious skill must fail before a child recovery call, got %d requests", provider.RequestCount())
	}
	if childTools := providerRequestToolNames(t, provider.Body(1)); !sameStringSet(childTools, []string{"read_file"}) {
		t.Fatalf("skill allowedTools widened host authority: %#v body=%s", childTools, provider.Body(1))
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("unadvertised skill write produced a side effect: err=%v", err)
	}
	hostCallID := providerHostToolCallIDForName(t, provider.Body(2), "run_skill")
	assertProviderSecurityBoundToolResult(t, provider.Body(2), hostCallID, "failed", false)
	internalRuns := runtimeServerInternalChildRuns(t, dataDir, threadID)
	if len(internalRuns) != 1 || internalRuns[0].Status != "failed" || internalRuns[0].Error != "" || internalRuns[0].FailureCode != "child_execution_failed" {
		t.Fatalf("malicious skill child did not fail at the advertised-tool gate: %#v", internalRuns)
	}
}

func TestRuntimeServerToolsProviderDiagnosticsReflectConfiguredProfilesWithoutSecrets(t *testing.T) {
	emptyServer := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		ProviderID:     "deepseek",
		Model:          "deepseek-chat",
		EndpointFormat: "chat_completions",
	}))
	defer emptyServer.Close()

	emptyTools := assertLiveJSON(t, emptyServer.URL, http.MethodGet, "/v1/runtime/tools", DefaultRuntimeToken, nil, http.StatusOK)
	if emptyTools["providerCount"] != float64(1) {
		t.Fatalf("runtime tools should expose one fallback provider count: %#v", emptyTools)
	}
	if _, exists := emptyTools["providers"]; exists {
		t.Fatalf("runtime tools must not expose provider profile diagnostics: %#v", emptyTools)
	}
	emptyRaw := mustJSONString(t, emptyTools)
	for _, forbidden := range []string{"go-runtime-default-provider", "test-provider-key", "local-fake-key"} {
		if strings.Contains(emptyRaw, forbidden) {
			t.Fatalf("runtime tools leaked synthetic provider marker %q: %s", forbidden, emptyRaw)
		}
	}

	configuredServer := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		ModelProvidersJSON: string(mustJSONNoTest(map[string]any{
			"defaultProviderId": "deepseek",
			"providers": []map[string]any{
				{"id": "deepseek", "apiKey": "sk-secret-provider-key", "baseUrl": "https://api.deepseek.com", "endpointFormat": "chat_completions", "models": []string{"deepseek-chat"}},
				{"id": "openai-compatible", "apiKey": "", "baseUrl": "https://openai.example/v1", "endpointFormat": "chat_completions", "models": []string{"gpt-compatible"}},
			},
		})),
	}))
	defer configuredServer.Close()

	configuredTools := assertLiveJSON(t, configuredServer.URL, http.MethodGet, "/v1/runtime/tools", DefaultRuntimeToken, nil, http.StatusOK)
	configuredRaw := mustJSONString(t, configuredTools)
	if strings.Contains(configuredRaw, "sk-secret-provider-key") {
		t.Fatalf("runtime tools provider diagnostics leaked API key: %s", configuredRaw)
	}
	if configuredTools["providerCount"] != float64(2) {
		t.Fatalf("runtime tools should expose configured provider count: %#v", configuredTools)
	}
	if _, exists := configuredTools["providers"]; exists {
		t.Fatalf("runtime tools must not expose configured provider profiles: %#v", configuredTools)
	}
}

func TestRuntimeServerInfoReflectsConfiguredDefaultProviderWithoutSecrets(t *testing.T) {
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		Model:          "claude-selected",
		ModelProvidersJSON: string(mustJSONNoTest(map[string]any{
			"defaultProviderId": "anthropic-main",
			"providers": []map[string]any{
				{
					"id":             "anthropic-main",
					"apiKey":         "sk-secret-anthropic-key",
					"baseUrl":        "https://api.anthropic.example",
					"endpointFormat": "anthropic_messages",
					"models":         []string{"claude-first", "claude-selected"},
					"modelProfiles": map[string]any{
						"claude-first": map[string]any{
							"contextWindowTokens": float64(100000),
							"inputModalities":     []string{"text"},
							"messageParts":        []string{"text"},
						},
						"claude-selected": map[string]any{
							"contextWindowTokens": float64(200000),
							"inputModalities":     []string{"text", "image"},
							"messageParts":        []string{"text", "input_image"},
						},
					},
				},
			},
		})),
	}))
	defer server.Close()

	info := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/info", DefaultRuntimeToken, nil, http.StatusOK)
	raw := mustJSONString(t, info)
	if strings.Contains(raw, "sk-secret-anthropic-key") {
		t.Fatalf("runtime info leaked provider API key: %s", raw)
	}
	providerInfo := mapField(t, info, "provider")
	if stringField(providerInfo, "id") != "anthropic-main" ||
		stringField(providerInfo, "model") != "claude-selected" ||
		jsonIntField(t, providerInfo, "contextWindowTokens") != 200000 ||
		stringField(providerInfo, "endpointFormat") != "messages" ||
		stringField(providerInfo, "family") != "anthropic-compatible" ||
		!boolField(providerInfo, "available") ||
		!boolField(providerInfo, "apiKeyConfigured") ||
		!boolField(providerInfo, "baseUrlConfigured") ||
		!boolField(providerInfo, "supportsImageInput") {
		t.Fatalf("runtime info should reflect configured non-DeepSeek default provider: %#v", info)
	}
	modelCapability := mapField(t, mapField(t, info, "capabilities"), "model")
	if stringField(modelCapability, "id") != "claude-selected" ||
		stringField(modelCapability, "providerId") != "anthropic-main" ||
		jsonIntField(t, modelCapability, "contextWindowTokens") != 200000 ||
		stringField(modelCapability, "endpointFormat") != "messages" ||
		!containsString(stringSliceField(t, modelCapability, "inputModalities"), "image") ||
		!containsString(stringSliceField(t, modelCapability, "messageParts"), "input_image") ||
		!boolField(modelCapability, "supportsImageInput") {
		t.Fatalf("runtime capabilities model should follow configured provider: %#v", modelCapability)
	}
}

func TestRuntimeServerDefaultContractCoversRendererBaselineEndpoints(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	dataDir := t.TempDir()
	workspace := t.TempDir()
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   g1.RuntimeToken,
		StartedAt:      g1.StartedAt,
		Routes:         g2.Routes,
		DurableTempDir: t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        dataDir,
		ProviderID:     "openai-configured",
		Model:          "gpt-compatible",
	}))
	defer server.Close()

	tools := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/tools", g1.RuntimeToken, nil, http.StatusOK)
	if mapField(t, tools, "attachments")["enabled"] != true ||
		mapField(t, tools, "memory")["enabled"] != true ||
		mapField(t, tools, "mcpSearch")["enabled"] != false ||
		mapField(t, tools, "mcpSearch")["mode"] != "auto" ||
		mapField(t, tools, "mcpSearch")["active"] != false ||
		mapField(t, tools, "mcpSearch")["available"] != false ||
		mapField(t, tools, "mcpSearch")["indexedToolCount"] != float64(0) ||
		mapField(t, tools, "mcpSearch")["advertisedToolCount"] != float64(0) ||
		mapField(t, tools, "skills")["enabled"] != false {
		t.Fatalf("runtime tools diagnostics must preserve Analytix renderer baseline: %#v", tools)
	}
	if servers, ok := tools["mcpServers"].([]any); !ok || len(servers) != 0 {
		t.Fatalf("runtime tools must not expose fake MCP servers as real diagnostics: %#v", tools)
	}
	if _, ok := tools["mcpLocalProof"]; ok {
		t.Fatalf("runtime tools must not expose fixture MCP proof as a production capability: %#v", tools)
	}
	skills := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/skills", g1.RuntimeToken, nil, http.StatusOK)
	if skills["schemaVersion"] != float64(2) || skills["enabled"] != false ||
		skills["available"] != false || skills["reasonCode"] != "disabled_by_config" ||
		skills["skillCount"] != float64(0) {
		t.Fatalf("skills response mismatch: %#v", skills)
	}

	created := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{
			"title":          "Go default created thread",
			"workspace":      workspace,
			"model":          "gpt-compatible",
			"providerId":     "openai-configured",
			"mode":           "plan",
			"approvalPolicy": "on-request",
			"sandboxMode":    "workspace-write",
		}),
		http.StatusCreated,
	)
	threadID := stringField(created, "id")
	if threadID == "" ||
		created["title"] != "Go default created thread" ||
		created["providerId"] != "openai-configured" ||
		created["mode"] != "plan" ||
		created["relation"] != "primary" {
		t.Fatalf("thread create response mismatch: %#v", created)
	}
	createdReplay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", g1.RuntimeToken, http.StatusOK)
	if !strings.Contains(createdReplay, "event: thread_created") {
		t.Fatalf("created thread replay should include thread_created event:\n%s", createdReplay)
	}
	workspaceStatus := assertLiveJSON(
		t,
		server.URL,
		http.MethodGet,
		"/v1/workspace/status?path="+url.QueryEscape(t.TempDir()),
		g1.RuntimeToken,
		nil,
		http.StatusOK,
	)
	if workspaceStatus["exists"] != true || workspaceStatus["isGitRepository"] != false || workspaceStatus["isDirty"] != nil {
		t.Fatalf("workspace status response mismatch: %#v", workspaceStatus)
	}

	goal := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/goal",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{"objective": "Validate Go default", "tokenBudget": 1000}),
		http.StatusOK,
	)
	if mapField(t, goal, "goal")["objective"] != "Validate Go default" {
		t.Fatalf("goal response mismatch: %#v", goal)
	}
	goalRead := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID+"/goal", g1.RuntimeToken, nil, http.StatusOK)
	if mapField(t, goalRead, "goal")["status"] != "active" {
		t.Fatalf("goal read response mismatch: %#v", goalRead)
	}
	goalReplay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", g1.RuntimeToken, http.StatusOK)
	if strings.Contains(goalReplay, "event: goal_updated") ||
		strings.Contains(goalReplay, `"objective":"Validate Go default"`) {
		t.Fatalf("goal replay exposed untyped goal prose:\n%s", goalReplay)
	}
	todos := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/todos",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{"todos": []map[string]any{{"content": "Audit contract", "status": "in_progress"}}}),
		http.StatusOK,
	)
	if items, ok := mapField(t, todos, "todos")["items"].([]any); !ok || len(items) != 1 {
		t.Fatalf("todos response mismatch: %#v", todos)
	}
	todosReplay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", g1.RuntimeToken, http.StatusOK)
	if strings.Contains(todosReplay, "event: todos_updated") ||
		strings.Contains(todosReplay, `"content":"Audit contract"`) {
		t.Fatalf("todos replay exposed untyped todo prose:\n%s", todosReplay)
	}

	attachment := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/attachments",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{
			"name":       "hello.txt",
			"mimeType":   "text/plain",
			"dataBase64": "aGVsbG8=",
			"threadId":   threadID,
			"workspace":  workspace,
		}),
		http.StatusCreated,
	)
	attachmentMeta := mapField(t, attachment, "attachment")
	attachmentID := stringField(attachmentMeta, "id")
	if attachmentID == "" || jsonIntField(t, attachmentMeta, "byteSize") != 5 || attachmentMeta["scope"] != "thread" {
		t.Fatalf("attachment upload response mismatch: %#v", attachment)
	}
	if !strings.HasPrefix(attachmentID, "att_") || len(attachmentID) != len("att_")+24 || attachmentMeta["hash"] != nil || attachmentMeta["localFilePath"] != nil {
		t.Fatalf("attachment public owner projection mismatch: %#v", attachmentMeta)
	}
	duplicateOwner := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", g1.RuntimeToken, mustJSON(t, map[string]any{
		"title": "Duplicate attachment owner", "workspace": workspace,
	}), http.StatusCreated)
	duplicateThreadID := stringField(duplicateOwner, "id")
	duplicateAttachment := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/attachments",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{
			"name":       "hello-copy.txt",
			"mimeType":   "text/plain",
			"dataBase64": "aGVsbG8=",
			"threadId":   duplicateThreadID,
			"workspace":  workspace,
		}),
		http.StatusCreated,
	)
	duplicateMeta := mapField(t, duplicateAttachment, "attachment")
	if stringField(duplicateMeta, "id") == attachmentID || duplicateMeta["localFilePath"] != nil || duplicateMeta["hash"] != nil {
		t.Fatalf("same blob must receive an independent attachment owner without metadata merge: %#v", duplicateAttachment)
	}
	unauthorizedContent := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/attachments/"+attachmentID+"/content?thread_id=thr_other&workspace="+url.QueryEscape(workspace), g1.RuntimeToken, nil, http.StatusForbidden)
	if unauthorizedContent["code"] != "forbidden" {
		t.Fatalf("attachment content must reject mismatched thread scope: %#v", unauthorizedContent)
	}
	unauthorizedWorkspaceContent := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/attachments/"+attachmentID+"/content?thread_id="+url.QueryEscape(threadID)+"&workspace="+url.QueryEscape("/tmp/other-workspace"), g1.RuntimeToken, nil, http.StatusForbidden)
	if unauthorizedWorkspaceContent["code"] != "forbidden" {
		t.Fatalf("thread-scoped attachment content must reject workspace-only matches: %#v", unauthorizedWorkspaceContent)
	}
	content := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/attachments/"+attachmentID+"/content?thread_id="+url.QueryEscape(threadID)+"&workspace="+url.QueryEscape(workspace), g1.RuntimeToken, nil, http.StatusOK)
	if content["dataBase64"] != "aGVsbG8=" {
		t.Fatalf("attachment content mismatch: %#v", content)
	}
	duplicateContent := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/attachments/"+stringField(duplicateMeta, "id")+"/content?thread_id="+url.QueryEscape(duplicateThreadID)+"&workspace="+url.QueryEscape(workspace), g1.RuntimeToken, nil, http.StatusOK)
	if duplicateContent["dataBase64"] != "aGVsbG8=" {
		t.Fatalf("duplicate attachment scope content mismatch: %#v", duplicateContent)
	}
	attachmentDiagnostics := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/attachments/diagnostics", g1.RuntimeToken, nil, http.StatusOK)
	if jsonIntField(t, attachmentDiagnostics, "count") != 2 || jsonIntField(t, attachmentDiagnostics, "totalBytes") != 10 {
		t.Fatalf("attachment diagnostics mismatch: %#v", attachmentDiagnostics)
	}
	if !containsString(stringSliceField(t, attachmentDiagnostics, "allowedMimeTypes"), "image/png") ||
		!containsString(stringSliceField(t, attachmentDiagnostics, "allowedDocumentMimeTypes"), "text/plain") {
		t.Fatalf("attachment diagnostics must expose the closed upload type policy: %#v", attachmentDiagnostics)
	}
	for _, forbidden := range []string{"rootDir", "acceptedMimeTypes", "textFallbackMaxBase64Bytes", "textFallbackMaxImageDimension", "textFallbackPreferredMimeType"} {
		if _, exists := attachmentDiagnostics[forbidden]; exists {
			t.Fatalf("attachment diagnostics exposed private policy field %s: %#v", forbidden, attachmentDiagnostics)
		}
	}
	unsupportedAttachment := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/attachments",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{
			"name":       "legacy.gif",
			"mimeType":   "image/gif",
			"dataBase64": "R0lGODlh",
			"threadId":   threadID,
			"workspace":  workspace,
		}),
		http.StatusBadRequest,
	)
	if unsupportedAttachment["code"] != "validation_error" || unsupportedAttachment["message"] != "The request did not satisfy the runtime contract." {
		t.Fatalf("unsupported image MIME should be rejected at upload: %#v", unsupportedAttachment)
	}
	if strings.Contains(string(mustJSON(t, unsupportedAttachment)), "unsupported image MIME type") {
		t.Fatalf("public upload validation exposed internal MIME detail: %#v", unsupportedAttachment)
	}

	memory := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/memory",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{"content": "Remember provider boundary", "scope": "workspace", "workspace": workspace}),
		http.StatusCreated,
	)
	memoryID := stringField(mapField(t, memory, "memory"), "id")
	if memoryID == "" {
		t.Fatalf("memory create response mismatch: %#v", memory)
	}
	updatedMemory := assertLiveJSON(
		t,
		server.URL,
		http.MethodPatch,
		"/v1/memory/"+memoryID,
		g1.RuntimeToken,
		mustJSON(t, map[string]any{"confidence": 0.9, "tags": []string{"runtime"}}),
		http.StatusOK,
	)
	if mapField(t, updatedMemory, "memory")["confidence"] != 0.9 {
		t.Fatalf("memory update response mismatch: %#v", updatedMemory)
	}
	memories := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/memory?workspace="+url.QueryEscape(workspace), g1.RuntimeToken, nil, http.StatusOK)
	if items, ok := memories["memories"].([]any); !ok || len(items) != 1 {
		t.Fatalf("memory list response mismatch: %#v", memories)
	}

	compact := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/compact",
		g1.RuntimeToken,
		mustJSON(t, map[string]string{"reason": "manual audit"}),
		http.StatusOK,
	)
	if compact["ok"] != true {
		t.Fatalf("compact response mismatch: %#v", compact)
	}
	review := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/review",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{"target": map[string]string{"kind": "custom", "instructions": "Review this thread."}}),
		http.StatusAccepted,
	)
	if review["reviewItemId"] == "" || review["turnId"] == "" {
		t.Fatalf("review response mismatch: %#v", review)
	}
	turn := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/turns",
		g1.RuntimeToken,
		mustJSON(t, map[string]string{"prompt": "Run one more turn."}),
		http.StatusAccepted,
	)
	turnID := stringField(turn, "turnId")
	steerResponse, steerBody := liveRequest(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/turns/"+turnID+"/steer",
		g1.RuntimeToken,
		mustJSON(t, map[string]string{"text": "Continue safely."}),
	)
	var steer map[string]any
	if err := json.Unmarshal(steerBody, &steer); err != nil {
		t.Fatalf("POST /steer returned invalid JSON: %v", err)
	}
	switch steerResponse.StatusCode {
	case http.StatusOK:
		if steer["ok"] != true {
			t.Fatalf("steer response mismatch: %#v", steer)
		}
	case http.StatusConflict:
		if stringField(steer, "code") != "conflict" ||
			stringField(steer, "message") != "The request conflicts with the current runtime state." {
			t.Fatalf("steer conflict response mismatch: %#v", steer)
		}
		steerJSON := string(mustJSON(t, steer))
		for _, forbidden := range []string{"turn_inactive", "turn_mismatch", turnID, "expectedTurnId", "currentTurnId"} {
			if forbidden != "" && strings.Contains(steerJSON, forbidden) {
				t.Fatalf("steer conflict response leaked turn-state detail %q: %s", forbidden, steerJSON)
			}
		}
	default:
		t.Fatalf("POST /steer status mismatch: got %d body %#v", steerResponse.StatusCode, steer)
	}
	interrupt := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/turns/"+turnID+"/interrupt",
		g1.RuntimeToken,
		mustJSON(t, map[string]bool{"discard": true}),
		http.StatusOK,
	)
	if interrupt["status"] != "aborted" && interrupt["status"] != "completed" {
		t.Fatalf("interrupt response mismatch: %#v", interrupt)
	}
	interruptReplay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", g1.RuntimeToken, http.StatusOK)
	if interrupt["status"] == "aborted" &&
		!hasRuntimeServerEvent(runtimeServerEventsForTurn(t, interruptReplay, turnID), "turn_aborted") {
		t.Fatalf("running interrupt replay must use Analytix turn_aborted event:\n%s", interruptReplay)
	}
	if strings.Contains(interruptReplay, "turn_interrupted") {
		t.Fatalf("interrupt replay must not use retired turn_interrupted event:\n%s", interruptReplay)
	}
	plan := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/checkpoints/chk_go/rewind-plan",
		g1.RuntimeToken,
		mustJSON(t, map[string]string{"scope": "combined"}),
		http.StatusOK,
	)
	if mapField(t, plan, "plan")["checkpointId"] != "chk_go" {
		t.Fatalf("checkpoint plan response mismatch: %#v", plan)
	}
	apply := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/checkpoints/chk_go/rewind-apply",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{"plan": mapField(t, plan, "plan")}),
		http.StatusOK,
	)
	if mapField(t, apply, "apply")["status"] != "blocked" {
		t.Fatalf("checkpoint apply response mismatch: %#v", apply)
	}
	clearedGoal := assertLiveJSON(t, server.URL, http.MethodDelete, "/v1/threads/"+threadID+"/goal", g1.RuntimeToken, nil, http.StatusOK)
	clearedTodos := assertLiveJSON(t, server.URL, http.MethodDelete, "/v1/threads/"+threadID+"/todos", g1.RuntimeToken, nil, http.StatusOK)
	if clearedGoal["cleared"] != true || clearedTodos["cleared"] != true {
		t.Fatalf("clear responses mismatch: goal=%#v todos=%#v", clearedGoal, clearedTodos)
	}
	deletedMemory := assertLiveJSON(t, server.URL, http.MethodDelete, "/v1/memory/"+memoryID, g1.RuntimeToken, nil, http.StatusOK)
	if stringField(mapField(t, deletedMemory, "memory"), "deletedAt") == "" {
		t.Fatalf("memory delete response mismatch: %#v", deletedMemory)
	}
	deletedThread := assertLiveJSON(t, server.URL, http.MethodDelete, "/v1/threads/"+threadID, g1.RuntimeToken, nil, http.StatusOK)
	if deletedThread["deleted"] != true {
		t.Fatalf("thread delete response mismatch: %#v", deletedThread)
	}
	deleteReplay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", g1.RuntimeToken, http.StatusOK)
	if !strings.Contains(deleteReplay, "event: thread_updated") ||
		!strings.Contains(deleteReplay, `"status":"deleted"`) ||
		strings.Contains(deleteReplay, "thread_deleted") {
		t.Fatalf("thread delete replay must stay inside Kun/Analytix thread_updated contract:\n%s", deleteReplay)
	}
}

func TestRuntimeServerUsageEndpointCoversRuntimeThreadDayModelAndThreadDetail(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	dataDir := t.TempDir()
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   g1.RuntimeToken,
		StartedAt:      g1.StartedAt,
		Routes:         g2.Routes,
		DurableTempDir: t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        dataDir,
	}))
	defer server.Close()

	created := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{
			"title":      "Usage baseline",
			"workspace":  dataDir,
			"model":      "deepseek-chat",
			"providerId": "deepseek",
			"mode":       "agent",
		}),
		http.StatusCreated,
	)
	threadID := stringField(created, "id")
	if threadID == "" {
		t.Fatalf("thread create response missing id: %#v", created)
	}
	assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/turns",
		g1.RuntimeToken,
		mustJSON(t, map[string]string{"prompt": "Record usage for the Go baseline.", "model": "deepseek-chat", "providerId": "deepseek"}),
		http.StatusAccepted,
	)

	runtimeUsage := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/usage", g1.RuntimeToken, nil, http.StatusOK)
	total := mapField(t, runtimeUsage, "total")
	if total["totalTokens"] != float64(1040) ||
		total["promptTokens"] != float64(1000) ||
		total["completionTokens"] != float64(40) ||
		total["reasoningTokens"] != float64(12) ||
		total["cacheHitTokens"] != float64(700) ||
		total["cacheMissTokens"] != float64(300) {
		t.Fatalf("runtime usage total mismatch: %#v", runtimeUsage)
	}
	if floatField(t, total, "costCny") <= 0 || floatField(t, total, "costUsd") <= 0 ||
		floatField(t, total, "cacheSavingsCny") <= 0 || floatField(t, total, "cacheSavingsUsd") <= 0 {
		t.Fatalf("runtime usage total must include non-zero provider pricing and cache savings: %#v", total)
	}
	if perThread, ok := runtimeUsage["perThread"].([]any); !ok || len(perThread) == 0 {
		t.Fatalf("runtime usage must include perThread usage: %#v", runtimeUsage)
	}

	threadUsage := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/usage?group_by=thread&thread_id="+threadID, g1.RuntimeToken, nil, http.StatusOK)
	threadBuckets, ok := threadUsage["buckets"].([]any)
	if !ok || len(threadBuckets) != 1 {
		t.Fatalf("thread usage should return one bucket: %#v", threadUsage)
	}
	threadBucket, _ := threadBuckets[0].(map[string]any)
	if threadBucket["thread_id"] != threadID ||
		threadBucket["input_tokens"] != float64(1000) ||
		threadBucket["output_tokens"] != float64(40) ||
		threadBucket["reasoning_tokens"] != float64(12) ||
		threadBucket["cached_tokens"] != float64(700) ||
		threadBucket["cache_hit_tokens"] != float64(700) ||
		threadBucket["cache_miss_tokens"] != float64(300) ||
		threadBucket["total_tokens"] != float64(1040) ||
		threadBucket["provider"] != "deepseek" ||
		threadBucket["last_turn_cache_hit_rate"] != 0.7 {
		t.Fatalf("thread usage bucket mismatch: %#v", threadBucket)
	}
	if floatField(t, threadBucket, "cost_cny") <= 0 || floatField(t, threadBucket, "cost_usd") <= 0 {
		t.Fatalf("thread usage bucket must include non-zero cost: %#v", threadBucket)
	}

	missingDayWindow := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/usage?group_by=day&timezone=UTC", g1.RuntimeToken, nil, http.StatusBadRequest)
	if missingDayWindow["code"] != "validation_error" {
		t.Fatalf("day usage without from/to or window should match TS validation: %#v", missingDayWindow)
	}
	dayUsage := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/usage?group_by=day&window=today&timezone=UTC", g1.RuntimeToken, nil, http.StatusOK)
	if dayUsage["group_by"] != "day" ||
		mapField(t, dayUsage, "totals")["total_tokens"] != float64(1040) ||
		mapField(t, dayUsage, "totals")["thread_count"] != float64(1) {
		t.Fatalf("day usage mismatch: %#v", dayUsage)
	}
	if floatField(t, mapField(t, dayUsage, "totals"), "cost_cny") <= 0 {
		t.Fatalf("day usage totals must include non-zero cost: %#v", dayUsage)
	}
	from := stringField(dayUsage, "from")
	to := stringField(dayUsage, "to")
	if from == "" || to == "" {
		t.Fatalf("day usage must infer a concrete range: %#v", dayUsage)
	}
	if _, err := time.Parse("2006-01-02", from); err != nil {
		t.Fatalf("day usage from must be YYYY-MM-DD: %#v", dayUsage)
	}
	dayUsageWithRange := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/usage?group_by=day&from="+from+"&to="+to+"&timezone=UTC", g1.RuntimeToken, nil, http.StatusOK)
	if mapField(t, dayUsageWithRange, "totals")["total_tokens"] != float64(1040) {
		t.Fatalf("day usage explicit range mismatch: %#v", dayUsageWithRange)
	}

	modelUsage := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/usage?group_by=model&from="+from+"&to="+to+"&timezone=UTC", g1.RuntimeToken, nil, http.StatusOK)
	modelBuckets, ok := modelUsage["buckets"].([]any)
	if !ok || len(modelBuckets) != 1 {
		t.Fatalf("model usage should return one bucket: %#v", modelUsage)
	}
	modelBucket, _ := modelBuckets[0].(map[string]any)
	if modelBucket["model"] != "deepseek-chat" ||
		modelBucket["provider"] != "deepseek" ||
		modelBucket["total_tokens"] != float64(1040) ||
		mapField(t, modelUsage, "totals")["total_tokens"] != float64(1040) {
		t.Fatalf("model usage mismatch: bucket=%#v response=%#v", modelBucket, modelUsage)
	}
	if floatField(t, modelBucket, "cost_cny") <= 0 || floatField(t, mapField(t, modelUsage, "totals"), "cost_cny") <= 0 {
		t.Fatalf("model usage must include non-zero cost: bucket=%#v response=%#v", modelBucket, modelUsage)
	}

	thread := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, g1.RuntimeToken, nil, http.StatusOK)
	detailUsage := mapField(t, thread, "usage")
	if detailUsage["totalTokens"] != float64(1040) ||
		detailUsage["cacheHitTokens"] != float64(700) ||
		detailUsage["cacheMissTokens"] != float64(300) {
		t.Fatalf("thread detail must include cumulative usage: %#v", thread)
	}
	if floatField(t, detailUsage, "costCny") <= 0 || floatField(t, detailUsage, "costUsd") <= 0 {
		t.Fatalf("thread detail usage must include non-zero cost: %#v", detailUsage)
	}
}

func TestRuntimeServerTurnContractPreservesAnalytixStartTurnFields(t *testing.T) {
	g1 := loadG1Contract(t)
	dataDir := t.TempDir()
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   g1.RuntimeToken,
		StartedAt:      g1.StartedAt,
		DurableTempDir: t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        dataDir,
	}))
	defer server.Close()
	created := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", g1.RuntimeToken, mustJSON(t, map[string]any{
		"title": "Live start-turn fields", "workspace": dataDir, "providerId": "deepseek", "model": "deepseek-chat",
	}), http.StatusCreated)
	threadID := stringField(created, "id")

	guiPlan := map[string]string{
		"operation":     "draft",
		"workspaceRoot": dataDir,
		"relativePath":  ".analytixsdd/plan/contract.md",
		"planId":        "plan-go-contract",
		"sourceRequest": "Preserve the turn contract.",
		"title":         "Go Contract Plan",
	}
	start := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/turns",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{
			"prompt":           "Preserve all turn fields.",
			"displayText":      "Visible turn prompt",
			"mode":             "plan",
			"guiPlan":          guiPlan,
			"approvalPolicy":   "never",
			"sandboxMode":      "read-only",
			"disableUserInput": true,
			"maxModelSteps":    3,
			"model":            "deepseek-chat",
		}),
		http.StatusAccepted,
	)
	turnID := stringField(start, "turnId")
	if turnID == "" {
		t.Fatalf("turn start response missing turn id: %#v", start)
	}
	thread := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, g1.RuntimeToken, nil, http.StatusOK)
	turn := findRuntimeServerTurn(t, thread, turnID)
	if turn["mode"] != "plan" ||
		turn["disableUserInput"] != true ||
		jsonIntField(t, turn, "maxModelSteps") != 3 {
		t.Fatalf("turn did not preserve requested control fields: %#v", turn)
	}
	for _, field := range []string{"approvalPolicy", "sandboxMode"} {
		if _, present := turn[field]; present {
			t.Fatalf("execution authority field %q crossed the public turn snapshot: %#v", field, turn)
		}
	}
	if plan := mapField(t, turn, "guiPlan"); plan["planId"] != "plan-go-contract" || plan["relativePath"] != ".analytixsdd/plan/contract.md" {
		t.Fatalf("turn guiPlan mismatch: %#v", plan)
	}
	items, _ := turn["items"].([]any)
	if len(items) == 0 {
		t.Fatalf("turn should include a user item: %#v", turn)
	}
	userItem, _ := items[0].(map[string]any)
	if userItem["kind"] != "user_message" ||
		userItem["displayText"] != "Visible turn prompt" {
		t.Fatalf("user item did not preserve prompt metadata: %#v", userItem)
	}

	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", g1.RuntimeToken, http.StatusOK)
	events := runtimeServerEventsForTurn(t, replay, turnID)
	turnStarted := firstRuntimeServerEvent(t, events, "turn_started")
	if turnStarted["mode"] != "plan" ||
		turnStarted["approvalPolicy"] != "never" ||
		turnStarted["sandboxMode"] != "read-only" ||
		turnStarted["disableUserInput"] != true ||
		jsonIntField(t, turnStarted, "maxModelSteps") != 3 {
		t.Fatalf("turn_started event did not preserve request metadata: %#v", turnStarted)
	}
	if hasRuntimeServerEvent(events, "approval_requested") || hasRuntimeServerEvent(events, "user_input_requested") {
		t.Fatalf("never/read-only/headless turn must not create blocking gates: %#v", events)
	}
}

func TestRuntimeServerAttachmentOwnerRejectsUnknownAndCrossThreadAuthority(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	dataDir := t.TempDir()
	workspace := t.TempDir()
	durableRoot := t.TempDir()
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   g1.RuntimeToken,
		StartedAt:      g1.StartedAt,
		Routes:         g2.Routes,
		DurableTempDir: durableRoot,
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        dataDir,
	}))
	defer server.Close()
	owner := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", g1.RuntimeToken, mustJSON(t, map[string]any{
		"title": "Attachment owner", "workspace": workspace,
	}), http.StatusCreated)
	threadID := stringField(owner, "id")

	rejected := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/attachments",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{
			"name": "private.txt", "mimeType": "text/plain", "dataBase64": "cHJpdmF0ZQ==",
			"threadId": "thr_other", "workspace": workspace,
		}),
		http.StatusForbidden,
	)
	if rejected["code"] != "forbidden" {
		t.Fatalf("unknown caller-reported attachment owner was accepted: %#v", rejected)
	}
	threadScoped := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/attachments",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{
			"name": "private.txt", "mimeType": "text/plain", "dataBase64": "cHJpdmF0ZQ==",
			"threadId": threadID, "workspace": workspace,
		}),
		http.StatusCreated,
	)
	threadScopedID := stringField(mapField(t, threadScoped, "attachment"), "id")
	if threadScopedID == "" {
		t.Fatalf("thread-scoped upload missing id: %#v", threadScoped)
	}
	metadataForbidden := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/attachments/"+threadScopedID+"?thread_id=thr_other&workspace="+url.QueryEscape(workspace), g1.RuntimeToken, nil, http.StatusForbidden)
	if metadataForbidden["code"] != "forbidden" {
		t.Fatalf("attachment metadata was readable without owner authority: %#v", metadataForbidden)
	}
	other := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", g1.RuntimeToken, mustJSON(t, map[string]any{
		"title": "Other attachment owner", "workspace": workspace,
	}), http.StatusCreated)
	otherThreadID := stringField(other, "id")
	crossThread := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/attachments/"+threadScopedID+"/content?thread_id="+url.QueryEscape(otherThreadID)+"&workspace="+url.QueryEscape(workspace), g1.RuntimeToken, nil, http.StatusForbidden)
	if crossThread["code"] != "forbidden" {
		t.Fatalf("cross-thread attachment owner was accepted: %#v", crossThread)
	}
	allowed := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/attachments/"+threadScopedID+"/content?thread_id="+url.QueryEscape(threadID)+"&workspace="+url.QueryEscape(workspace), g1.RuntimeToken, nil, http.StatusOK)
	if allowed["dataBase64"] != "cHJpdmF0ZQ==" {
		t.Fatalf("exact owner could not read attachment: %#v", allowed)
	}
}

func TestRuntimeServerAttachmentWithoutCaseAuthorityNeverReachesProvider(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	dataDir := t.TempDir()
	workspace := t.TempDir()
	writeRuntimeCaseProjectBinding(t, workspace, "attachment-authority-boundary")
	capture := newProviderCaptureServer(t)
	modelProviders := string(mustJSON(t, map[string]any{
		"defaultProviderId": "vision-provider",
		"providers": []map[string]any{
			{
				"id":             "vision-provider",
				"apiKey":         "test-provider-key",
				"baseUrl":        capture.URL() + "/v1",
				"endpointFormat": "chat_completions",
				"models":         []string{"vision-model"},
				"modelProfiles": map[string]any{
					"vision-model": map[string]any{
						"inputModalities": []string{"text", "image"},
						"messageParts":    []string{"text", "image_url"},
					},
				},
			},
			{
				"id":             "text-provider",
				"apiKey":         "test-provider-key",
				"baseUrl":        capture.URL() + "/v1",
				"endpointFormat": "chat_completions",
				"models":         []string{"text-model"},
				"modelProfiles": map[string]any{
					"text-model": map[string]any{
						"inputModalities": []string{"text"},
						"messageParts":    []string{"text"},
					},
				},
			},
		},
	}))
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       g1.RuntimeToken,
		StartedAt:          g1.StartedAt,
		Routes:             g2.Routes,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: modelProviders,
	}))
	defer server.Close()
	thread := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{
			"title":      "Attachment authority boundary",
			"workspace":  workspace,
			"providerId": "vision-provider",
			"model":      "vision-model",
		}),
		http.StatusCreated,
	)
	threadID := stringField(thread, "id")
	if threadID == "" {
		t.Fatalf("attachment boundary thread missing id: %#v", thread)
	}

	imageAttachment := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/attachments",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{
			"name":       "chart.png",
			"mimeType":   "image/png",
			"dataBase64": "iVBORw0KGgo=",
			"threadId":   threadID,
			"workspace":  workspace,
		}),
		http.StatusCreated,
	)
	imageAttachmentID := stringField(mapField(t, imageAttachment, "attachment"), "id")
	if imageAttachmentID == "" {
		t.Fatalf("image attachment upload missing id: %#v", imageAttachment)
	}

	turn := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/turns",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{
			"prompt":        "Describe the attached chart.",
			"providerId":    "vision-provider",
			"model":         "vision-model",
			"attachmentIds": []string{imageAttachmentID},
		}),
		http.StatusAccepted,
	)
	if capture.Count() != 0 {
		t.Fatalf("unbound attachment reached the provider: calls=%d", capture.Count())
	}
	assertIncidentAcceptedFinal(t, server.URL, threadID, stringField(turn, "turnId"), apploop.CaseFundSourceUnavailableAnswer())
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", g1.RuntimeToken, http.StatusOK)
	for _, forbidden := range []string{"iVBORw0KGgo=", "/tmp/chart.png", `"image_url"`} {
		if strings.Contains(replay, forbidden) {
			t.Fatalf("boundary attachment leaked private input %q into SSE:\n%s", forbidden, replay)
		}
	}
}

func TestRuntimeServerPersistentAttachmentsSurviveRestart(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	dataDir := t.TempDir()
	workspace := t.TempDir()
	durableRoot := t.TempDir()
	firstHandler := newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   g1.RuntimeToken,
		StartedAt:      g1.StartedAt,
		Routes:         g2.Routes,
		DurableTempDir: durableRoot,
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        dataDir,
	})
	server := httptest.NewServer(firstHandler)
	owner := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", g1.RuntimeToken, mustJSON(t, map[string]any{
		"title": "Attachment owner", "workspace": workspace,
	}), http.StatusCreated)
	threadID := stringField(owner, "id")
	upload := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/attachments",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{
			"name":       "hello.txt",
			"mimeType":   "text/plain",
			"dataBase64": "aGVsbG8=",
			"threadId":   threadID,
			"workspace":  workspace,
		}),
		http.StatusCreated,
	)
	meta := mapField(t, upload, "attachment")
	attachmentID := stringField(meta, "id")
	if attachmentID == "" || meta["hash"] != nil || meta["localFilePath"] != nil || meta["scope"] != "thread" {
		t.Fatalf("attachment metadata should expose only bounded display authority: %#v", meta)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "attachments", "metadata", attachmentID+".json")); err != nil {
		t.Fatalf("attachment metadata file missing: %v", err)
	}
	privateMetadataBody, err := os.ReadFile(filepath.Join(dataDir, "attachments", "metadata", attachmentID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var privateMetadata map[string]any
	if err := json.Unmarshal(privateMetadataBody, &privateMetadata); err != nil {
		t.Fatal(err)
	}
	ownerDigest := stringField(mapField(t, privateMetadata, "ownerRecord"), "ownerDigest")
	if ownerDigest == "" {
		t.Fatalf("attachment private owner digest missing: %#v", privateMetadata)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "private", "attachment-authority", "owners", ownerDigest[:2], ownerDigest+".json")); err != nil {
		t.Fatalf("attachment private owner authority missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "attachments", "content", attachmentID+".bin")); err != nil {
		t.Fatalf("attachment content file missing: %v", err)
	}
	server.Close()
	shutdown, ok := firstHandler.(interface{ Shutdown(context.Context) error })
	if !ok {
		t.Fatal("runtime attachment handler does not expose shutdown")
	}
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), runtimeServerShutdownTestTimeout)
	defer cancelShutdown()
	if err := shutdown.Shutdown(shutdownContext); err != nil {
		t.Fatalf("shutdown attachment runtime: %v", err)
	}

	restarted := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   g1.RuntimeToken,
		StartedAt:      g1.StartedAt,
		Routes:         g2.Routes,
		DurableTempDir: durableRoot,
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        dataDir,
	}))
	defer restarted.Close()
	content := assertLiveJSON(t, restarted.URL, http.MethodGet, "/v1/attachments/"+attachmentID+"/content?thread_id="+url.QueryEscape(threadID)+"&workspace="+url.QueryEscape(workspace), g1.RuntimeToken, nil, http.StatusOK)
	if content["dataBase64"] != "aGVsbG8=" || mapField(t, content, "attachment")["localFilePath"] != nil {
		t.Fatalf("persisted attachment content mismatch after restart: %#v", content)
	}
	diagnostics := assertLiveJSON(t, restarted.URL, http.MethodGet, "/v1/attachments/diagnostics", g1.RuntimeToken, nil, http.StatusOK)
	if jsonIntField(t, diagnostics, "count") != 1 || jsonIntField(t, diagnostics, "totalBytes") != 5 {
		t.Fatalf("attachment diagnostics mismatch after restart: %#v", diagnostics)
	}
}

func TestRuntimeServerPersistentMemorySurvivesRestartAndMutates(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	firstHandler := newRuntimeServerTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   g1.RuntimeToken,
		StartedAt:      g1.StartedAt,
		Routes:         g2.Routes,
		DurableTempDir: t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        dataDir,
	})
	server := httptest.NewServer(firstHandler)
	created := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/memory",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{
			"content":   "Remember persistent memory",
			"scope":     "project",
			"workspace": workspace,
			"tags":      []string{"runtime"},
		}),
		http.StatusCreated,
	)
	memoryID := stringField(mapField(t, created, "memory"), "id")
	if memoryID == "" {
		t.Fatalf("memory create missing id: %#v", created)
	}
	server.Close()
	shutdownRuntimeTestHandler(t, firstHandler)

	restarted := httptest.NewServer(newRuntimeServerTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   g1.RuntimeToken,
		StartedAt:      g1.StartedAt,
		Routes:         g2.Routes,
		DurableTempDir: t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        dataDir,
	}))
	defer restarted.Close()
	listed := assertLiveJSON(t, restarted.URL, http.MethodGet, "/v1/memory?workspace="+workspace, g1.RuntimeToken, nil, http.StatusOK)
	memories, ok := listed["memories"].([]any)
	if !ok || len(memories) != 1 {
		t.Fatalf("persisted memory missing after restart: %#v", listed)
	}
	updated := assertLiveJSON(
		t,
		restarted.URL,
		http.MethodPatch,
		"/v1/memory/"+memoryID,
		g1.RuntimeToken,
		mustJSON(t, map[string]any{"content": "Remember updated memory", "confidence": 0.75, "disabled": true}),
		http.StatusOK,
	)
	if mapField(t, updated, "memory")["content"] != "Remember updated memory" ||
		mapField(t, updated, "memory")["disabledAt"] == "" {
		t.Fatalf("memory patch mismatch after restart: %#v", updated)
	}
	deleted := assertLiveJSON(t, restarted.URL, http.MethodDelete, "/v1/memory/"+memoryID, g1.RuntimeToken, nil, http.StatusOK)
	if stringField(mapField(t, deleted, "memory"), "deletedAt") == "" {
		t.Fatalf("memory delete mismatch after restart: %#v", deleted)
	}
	diagnostics := assertLiveJSON(t, restarted.URL, http.MethodGet, "/v1/memory/diagnostics", g1.RuntimeToken, nil, http.StatusOK)
	if jsonIntField(t, diagnostics, "activeCount") != 0 || jsonIntField(t, diagnostics, "tombstoneCount") != 1 {
		t.Fatalf("memory diagnostics mismatch after restart mutation: %#v", diagnostics)
	}
}

func TestRuntimeServerProductionDefaultDoesNotSeedFixturesOrExposeProof(t *testing.T) {
	dataDir := t.TempDir()
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        dataDir,
	}))
	defer server.Close()

	threads := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads", DefaultRuntimeToken, nil, http.StatusOK)
	if items, ok := threads["threads"].([]any); !ok || len(items) != 0 {
		t.Fatalf("fresh production runtime must not seed G2 fixture threads: %#v", threads)
	}
	info := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/info", DefaultRuntimeToken, nil, http.StatusOK)
	mcpCapability := mapField(t, mapField(t, info, "capabilities"), "mcp")
	if _, ok := mcpCapability["contractProof"]; ok {
		t.Fatalf("production runtime info must not expose MCP proof capability: %#v", info)
	}
	tools := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/tools", DefaultRuntimeToken, nil, http.StatusOK)
	if _, ok := tools["mcpLocalProof"]; ok {
		t.Fatalf("production runtime tools must not expose local proof diagnostics: %#v", tools)
	}
	created := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":     "Fresh production",
		"workspace": dataDir,
	}), http.StatusCreated)
	threadID := stringField(created, "id")
	failed := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "Do not use fake provider.",
		"providerId": "deepseek-live-local",
		"model":      "deepseek-chat",
	}), http.StatusInternalServerError)
	if strings.Contains(mustJSONString(t, failed), "deepseek hello") {
		t.Fatalf("production runtime must not route live-local provider ids to fake provider: %#v", failed)
	}
}

func TestRuntimeServerNormalTextTurnPublishesTypedOrdinaryResult(t *testing.T) {
	dataDir := t.TempDir()
	provider := newCompleteProviderServer(t, [][]string{{
		`data: {"choices":[{"delta":{"content":"plain ok"}}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`,
		`data: [DONE]`,
	}})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "text-provider", "text-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":     "Plain production turn",
		"workspace": dataDir,
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	started := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "Say hello without fixtures.",
		"providerId": "text-provider",
		"model":      "text-model",
	}), http.StatusAccepted)
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	for _, forbidden := range []string{"Ship", "D0244", "fake MCP", "fake provider", "mcpLocalProof", "deepseek hello"} {
		if strings.Contains(replay, forbidden) {
			t.Fatalf("normal production turn leaked fixture/proof marker %q:\n%s", forbidden, replay)
		}
	}
	assertRuntimeServerTypedOrdinaryTerminal(t, replay, stringField(started, "turnId"), "plain ok", domainordinaryresult.ResultSlotOriginProviderOrdinaryOnlyV1)
}

func TestRuntimeServerSecondTurnIncludesAcceptedHostHistoryOnly(t *testing.T) {
	dataDir := t.TempDir()
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"content":"first reply"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"second reply"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":9,"completion_tokens":2,"total_tokens":11}}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "history-provider", "history-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":     "History production turn",
		"workspace": dataDir,
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	first := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "First prompt for history.",
		"providerId": "history-provider",
		"model":      "history-model",
	}), http.StatusAccepted)
	second := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "Second prompt uses prior context.",
		"providerId": "history-provider",
		"model":      "history-model",
	}), http.StatusAccepted)
	if provider.RequestCount() != 2 {
		t.Fatalf("expected two provider requests, got %d", provider.RequestCount())
	}
	secondBody := provider.Body(1)
	for _, expected := range []string{"First prompt for history.", "first reply", "Second prompt uses prior context."} {
		if !strings.Contains(secondBody, expected) {
			t.Fatalf("second provider request missing history %q:\n%s", expected, secondBody)
		}
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	assertRuntimeServerTypedOrdinaryTerminal(t, replay, stringField(first, "turnId"), "first reply", domainordinaryresult.ResultSlotOriginProviderOrdinaryOnlyV1)
	assertRuntimeServerTypedOrdinaryTerminal(t, replay, stringField(second, "turnId"), "second reply", domainordinaryresult.ResultSlotOriginProviderOrdinaryOnlyV1)
}

func TestRuntimeServerReviewPersistsReviewItemAndReplay(t *testing.T) {
	dataDir := t.TempDir()
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"content":"review model answer"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "review-provider", "review-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Review persistence",
		"workspace":  dataDir,
		"providerId": "review-provider",
		"model":      "review-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	review := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/review", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"target": map[string]string{"kind": "custom", "instructions": "Review this thread."},
	}), http.StatusAccepted)
	turnID := stringField(review, "turnId")
	reviewItemID := stringField(review, "reviewItemId")
	if turnID == "" || reviewItemID == "" {
		t.Fatalf("review response should include turn and review item ids: %#v", review)
	}
	thread = assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	turn := findRuntimeServerTurn(t, thread, turnID)
	items, _ := turn["items"].([]any)
	var reviewItem map[string]any
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if stringField(item, "id") == reviewItemID {
			reviewItem = item
			break
		}
	}
	if reviewItem == nil ||
		stringField(reviewItem, "kind") != "review" ||
		stringField(reviewItem, "status") != "completed" ||
		stringField(reviewItem, "reviewText") == "" {
		t.Fatalf("review item should be durable and completed: %#v", turn)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	for _, expected := range []string{`"kind":"item_created"`, reviewItemID, `"kind":"review"`, `"kind":"turn_completed"`} {
		if !strings.Contains(replay, expected) {
			t.Fatalf("review replay missing %s:\n%s", expected, replay)
		}
	}
}

func TestReasoningNotPersistedOrExported(t *testing.T) {
	dataDir := t.TempDir()
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"reasoning_content":"private scratchpad"}}]}`,
			`data: {"choices":[{"delta":{"content":"public answer"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":6,"completion_tokens":3,"total_tokens":9,"completion_tokens_details":{"reasoning_tokens":2}}}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"second answer"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":12,"completion_tokens":2,"total_tokens":14}}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "deepseek-cache", "deepseek-chat"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":     "Reasoning cache guard",
		"workspace": dataDir,
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	first := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "First prompt.",
		"providerId": "deepseek-cache",
		"model":      "deepseek-chat",
	}), http.StatusAccepted)
	second := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "Second prompt.",
		"providerId": "deepseek-cache",
		"model":      "deepseek-chat",
	}), http.StatusAccepted)
	if provider.RequestCount() != 2 {
		t.Fatalf("expected two provider requests, got %d", provider.RequestCount())
	}
	firstReplay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if strings.Contains(firstReplay, "assistant_reasoning") || strings.Contains(firstReplay, "private scratchpad") {
		t.Fatalf("reasoning must not enter SSE replay:\n%s", firstReplay)
	}
	threadJSON := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	encodedThread, err := json.Marshal(threadJSON)
	if err != nil {
		t.Fatalf("marshal thread: %v", err)
	}
	if strings.Contains(string(encodedThread), "assistant_reasoning") || strings.Contains(string(encodedThread), "private scratchpad") {
		t.Fatalf("reasoning must not enter thread history: %s", encodedThread)
	}
	assertRuntimeServerTypedOrdinaryTerminal(t, firstReplay, stringField(first, "turnId"), "public answer", domainordinaryresult.ResultSlotOriginProviderOrdinaryOnlyV1)
	assertRuntimeServerTypedOrdinaryTerminal(t, firstReplay, stringField(second, "turnId"), "second answer", domainordinaryresult.ResultSlotOriginProviderOrdinaryOnlyV1)
	secondBody := provider.Body(1)
	if !strings.Contains(secondBody, "public answer") ||
		!strings.Contains(secondBody, "Second prompt.") {
		t.Fatalf("second provider request missing accepted host history:\n%s", secondBody)
	}
	if strings.Contains(secondBody, "private scratchpad") ||
		strings.Contains(secondBody, "reasoning_content") {
		t.Fatalf("provider reasoning was reuploaded to the provider:\n%s", secondBody)
	}
}

func TestRuntimeServerDeepSeekToolContinuationReplaysReasoningOnlyWithinExactAttempt(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "note.txt"), []byte("note from workspace"), 0o644); err != nil {
		t.Fatal(err)
	}
	const (
		firstReasoningSentinel  = "DEEPSEEK_PRIVATE_TOOL_REASONING_FIRST"
		secondReasoningSentinel = "DEEPSEEK_PRIVATE_TOOL_REASONING_SECOND"
	)
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"reasoning_content":"` + firstReasoningSentinel + `"}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read","type":"function","function":{"name":"read","arguments":"{\"path\":\"note.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"reasoning_content":"` + secondReasoningSentinel + `"}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_glob","type":"function","function":{"name":"glob","arguments":"{\"pattern\":\"*.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"first final"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"follow-up final"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: durableRoot,
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        dataDir,
		ModelProvidersJSON: testModelProvidersJSONWithReasoningProtocol(
			provider.URL(), "deepseek-tool", "deepseek-chat", "deepseek-chat-completions",
		),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":     "DeepSeek tool reasoning",
		"workspace": dataDir,
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Read the note.",
		"providerId":     "deepseek-tool",
		"model":          "deepseek-chat",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)
	if provider.RequestCount() != 3 {
		t.Fatalf("expected first turn to execute tool and continue, got %d requests", provider.RequestCount())
	}
	readCallID := providerHostToolCallIDForName(t, provider.Body(1), "read")
	if !strings.Contains(provider.Body(1), `"reasoning_content":"`+firstReasoningSentinel+`"`) ||
		!strings.Contains(provider.Body(1), `"tool_call_id":"`+readCallID+`"`) ||
		strings.Count(provider.Body(1), firstReasoningSentinel) != 1 {
		t.Fatalf("DeepSeek continuation should bind exact reasoning once to its tool pairing:\n%s", provider.Body(1))
	}
	globCallID := providerHostToolCallIDForName(t, provider.Body(2), "glob")
	if !strings.Contains(provider.Body(2), `"reasoning_content":"`+firstReasoningSentinel+`"`) ||
		!strings.Contains(provider.Body(2), `"reasoning_content":"`+secondReasoningSentinel+`"`) ||
		!strings.Contains(provider.Body(2), `"tool_call_id":"`+readCallID+`"`) ||
		!strings.Contains(provider.Body(2), `"tool_call_id":"`+globCallID+`"`) ||
		strings.Count(provider.Body(2), firstReasoningSentinel) != 1 ||
		strings.Count(provider.Body(2), secondReasoningSentinel) != 1 {
		t.Fatalf("DeepSeek continuation must replay every exact reasoning segment still in the tool chain:\n%s", provider.Body(2))
	}
	firstReplay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if strings.Contains(firstReplay, firstReasoningSentinel) || strings.Contains(firstReplay, secondReasoningSentinel) {
		t.Fatalf("DeepSeek reasoning must not enter public SSE replay:\n%s", firstReplay)
	}
	firstThread := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	if encoded, _ := json.Marshal(firstThread); strings.Contains(string(encoded), firstReasoningSentinel) ||
		strings.Contains(string(encoded), secondReasoningSentinel) {
		t.Fatalf("DeepSeek reasoning must not enter thread history: %s", encoded)
	}
	assertRuntimeFilesExcludeValues(t, dataDir, firstReasoningSentinel, secondReasoningSentinel)
	assertRuntimeFilesExcludeValues(t, durableRoot, firstReasoningSentinel, secondReasoningSentinel)

	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":          "Summarize the prior tool result.",
		"providerId":      "deepseek-tool",
		"model":           "deepseek-chat",
		"reasoningEffort": "off",
	}), http.StatusAccepted)
	if provider.RequestCount() != 4 {
		t.Fatalf("expected follow-up turn provider request, got %d", provider.RequestCount())
	}
	followUpBody := provider.Body(3)
	if !strings.Contains(followUpBody, "Summarize the prior tool result.") ||
		!strings.Contains(followUpBody, "Analytix host-selected prior tool activity") ||
		!strings.Contains(followUpBody, "tool_completed") ||
		!strings.Contains(followUpBody, "first final") {
		t.Fatalf("high-to-off follow-up should preserve safe semantic tool history:\n%s", followUpBody)
	}
	if strings.Contains(followUpBody, firstReasoningSentinel) ||
		strings.Contains(followUpBody, secondReasoningSentinel) ||
		strings.Contains(followUpBody, `"reasoning_content"`) ||
		strings.Contains(followUpBody, `"tool_calls"`) ||
		strings.Contains(followUpBody, `"tool_call_id"`) {
		t.Fatalf("reasoning must not cross the turn boundary:\n%s", followUpBody)
	}
	assertRuntimeFilesExcludeValues(t, dataDir, firstReasoningSentinel, secondReasoningSentinel)
	assertRuntimeFilesExcludeValues(t, durableRoot, firstReasoningSentinel, secondReasoningSentinel)
}

func TestRuntimeServerCommandSeparatesGeneralOnlyFromHostPolicyCaseBoundary(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	token := DefaultRuntimeToken
	provider := newCompleteProviderServer(t, [][]string{{
		`data: {"choices":[{"delta":{"content":"cmd env ok"}}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":6,"completion_tokens":3,"total_tokens":9}}`,
		`data: [DONE]`,
	}})
	modelProviders, connectProviderRegistry := prepareRuntimeServerExplicitProviderRegistryFixture(
		t, dataDir, provider.URL(), "deepseek-gui", "deepseek-chat",
	)

	runtimeServerBinary := strings.TrimSpace(os.Getenv(runtimeServerRootTestProductionCommandEnvironment))
	if runtimeServerBinary == "" {
		runtimeServerBinary = filepath.Join(t.TempDir(), "runtime-server")
		if runtime.GOOS == "windows" {
			runtimeServerBinary += ".exe"
		}
		buildContext, cancelBuild := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancelBuild()
		if output, err := buildRuntimeServerRootTestProductionCommand(buildContext, runtimeServerBinary); err != nil {
			t.Fatalf("build production runtime-server command: %v\n%s", err, string(output))
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), runtimeServerProductionCommandTestTimeout)
	defer cancel()
	cmd := exec.CommandContext(
		ctx,
		runtimeServerBinary,
		"--addr",
		"127.0.0.1:0",
		"--durable-root",
		durableRoot,
		"--data-dir",
		dataDir,
		"--provider-id",
		"deepseek-gui",
		"--endpoint-format",
		"chat_completions",
		"--model",
		"deepseek-chat",
	)
	cmd.Env = append(os.Environ(),
		"ANALYTIX_RUNTIME_TOKEN="+token,
		"ANALYTIX_MODEL_PROVIDERS="+modelProviders,
		"ANALYTIX_API_KEY=",
	)
	// Keep the production command in the owning shard's process group / Windows
	// Job. A nested session can outlive a coordinator timeout because the outer
	// reaper can no longer address it.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("runtime-server stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("runtime-server stderr pipe: %v", err)
	}
	stderrCh := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(stderr)
		stderrCh <- string(data)
	}()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start runtime-server command: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	})

	type readyPayload struct {
		URL                    string `json:"url"`
		RuntimeTokenConfigured bool   `json:"runtimeTokenConfigured"`
	}
	readyCh := make(chan readyPayload, 1)
	readyErrCh := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "ANALYTIX_RUNTIME_SERVER_READY ") {
				continue
			}
			var payload readyPayload
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "ANALYTIX_RUNTIME_SERVER_READY ")), &payload); err != nil {
				readyErrCh <- err
				return
			}
			readyCh <- payload
			return
		}
		if err := scanner.Err(); err != nil {
			readyErrCh <- err
			return
		}
		readyErrCh <- fmt.Errorf("runtime-server exited before ready")
	}()

	var ready readyPayload
	select {
	case ready = <-readyCh:
	case err := <-readyErrCh:
		t.Fatalf("runtime-server did not emit ready payload: %v", err)
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		stderrText := <-stderrCh
		t.Fatalf("runtime-server did not become ready: %v stderr=%s", ctx.Err(), stderrText)
	}
	if !ready.RuntimeTokenConfigured {
		t.Fatalf("ready token configuration mismatch: %#v", ready)
	}
	connectProviderRegistry(ready.URL)

	thread := assertLiveJSON(t, ready.URL, http.MethodPost, "/v1/threads", token, mustJSON(t, map[string]any{
		"title":     "Command env provider",
		"workspace": dataDir,
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, ready.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", token, mustJSON(t, map[string]any{
		"prompt": "Give a general explanation using provider settings inherited from GUI env.",
	}), http.StatusAccepted)
	if provider.RequestCount() != 1 {
		t.Fatalf("production runtime general-only policy did not reach the provider exactly once: %d", provider.RequestCount())
	}

	caseThread := assertLiveJSON(t, ready.URL, http.MethodPost, "/v1/threads", token, mustJSON(t, map[string]any{
		"title":     "High-risk authority boundary",
		"workspace": dataDir,
	}), http.StatusCreated)
	caseThreadID := stringField(caseThread, "id")
	assertLiveJSON(t, ready.URL, http.MethodPost, "/v1/threads/"+caseThreadID+"/turns", token, mustJSON(t, map[string]any{
		"prompt":     "Analyze the case evidence.",
		"riskIntent": "case",
	}), http.StatusAccepted)
	replay := liveSSE(t, ready.URL, "/v1/threads/"+caseThreadID+"/events?since_seq=0", token, http.StatusOK)
	if provider.RequestCount() != 1 {
		t.Fatalf("production host-policy case boundary reached the provider: %d requests\n%s", provider.RequestCount(), replay)
	}
	if !strings.Contains(replay, domainevidence.CaseSourceUnavailableText) ||
		!strings.Contains(replay, `"variant":"SourceUnavailableAnswer"`) ||
		!strings.Contains(replay, `"coverageStatus":"unavailable"`) ||
		!strings.Contains(replay, `"receiptMetadata":{"citations":[],"count":0`) {
		t.Fatalf("production host-policy case turn did not persist only the source boundary:\n%s", replay)
	}
	if strings.Contains(replay, domainevidence.AgentSafetyAuthorityUnavailableText) {
		t.Fatalf("installed host-policy authority was misreported as unavailable:\n%s", replay)
	}
	for _, forbidden := range []string{"cmd env ok", "cmd-provider-key"} {
		if strings.Contains(replay, forbidden) {
			t.Fatalf("production authority boundary leaked provider material %q:\n%s", forbidden, replay)
		}
	}
}

func TestRuntimeServerProductionCommandTimeoutCoversInstrumentedLifecycle(t *testing.T) {
	minimum := 4 * runtimeServerPositiveTestTimeout
	if runtimeServerProductionCommandTestTimeout < minimum {
		t.Fatalf(
			"production command test timeout = %s, want at least %s for instrumented startup and two turns",
			runtimeServerProductionCommandTestTimeout,
			minimum,
		)
	}
}

func TestRuntimeServerConfiguredProviderPricingProducesNonZeroCost(t *testing.T) {
	dataDir := t.TempDir()
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"content":"openai priced"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110}}`,
			`data: [DONE]`,
		},
		{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":80}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"anthropic priced"}}`,
			`data: {"type":"message_delta","usage":{"output_tokens":12}}`,
			`data: {"type":"message_stop"}`,
		},
		{
			`data: {"choices":[{"delta":{"content":"custom priced"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":50,"completion_tokens":8,"total_tokens":58}}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"mimo priced"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":70,"completion_tokens":9,"total_tokens":79}}`,
			`data: [DONE]`,
		},
	})
	modelProviders := string(mustJSON(t, map[string]any{
		"defaultProviderId": "openai-priced",
		"providers": []map[string]any{
			{
				"id":             "openai-priced",
				"apiKey":         "test-provider-key",
				"baseUrl":        provider.URL() + "/v1",
				"endpointFormat": "chat_completions",
				"models":         []string{"gpt-priced"},
				"price":          map[string]any{"input": 10, "output": 20, "currency": "USD"},
			},
			{
				"id":             "anthropic-priced",
				"apiKey":         "test-provider-key",
				"baseUrl":        provider.URL(),
				"endpointFormat": "messages",
				"models":         []string{"claude-priced"},
				"price":          map[string]any{"input": 8, "output": 24, "currency": "USD"},
			},
			{
				"id":             "custom-priced",
				"apiKey":         "test-provider-key",
				"baseUrl":        provider.URL() + "/custom-endpoint",
				"endpointFormat": "custom_endpoint",
				"models":         []string{"custom-priced"},
				"price":          map[string]any{"input": 3, "output": 9, "currency": "USD"},
			},
			{
				"id":             "xiaomi-priced",
				"apiKey":         "test-provider-key",
				"baseUrl":        provider.URL() + "/v1",
				"endpointFormat": "chat_completions",
				"models":         []string{"mimo-v2.5"},
				"price":          map[string]any{"input": 5, "output": 15, "currency": "USD"},
			},
		},
	}))
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: modelProviders,
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":     "Priced providers",
		"workspace": dataDir,
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	cases := []struct {
		providerID string
		model      string
	}{
		{"openai-priced", "gpt-priced"},
		{"anthropic-priced", "claude-priced"},
		{"custom-priced", "custom-priced"},
		{"xiaomi-priced", "mimo-v2.5"},
	}
	for _, tc := range cases {
		selectRuntimeServerFixtureProvider(t, server.URL, DefaultRuntimeToken, tc.providerID)
		assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
			"prompt":     "Price " + tc.providerID,
			"providerId": tc.providerID,
			"model":      tc.model,
		}), http.StatusAccepted)
	}

	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	telemetryByProviderID := runtimeServerTurnTelemetryByStartedProviderID(t, parseRuntimeServerSSEEvents(t, replay))
	for _, tc := range cases {
		telemetry, ok := telemetryByProviderID[tc.providerID]
		if !ok {
			t.Fatalf("configured provider price missing host-bound usage for %s: telemetry=%#v replay=%s", tc.providerID, telemetryByProviderID, replay)
		}
		if telemetry.Model != tc.model {
			t.Fatalf("configured provider price associated with the wrong host-started model for %s: telemetry=%#v", tc.providerID, telemetry)
		}
		if cost := floatField(t, telemetry.Usage, "costUsd"); cost <= 0 {
			t.Fatalf("configured provider price should produce non-zero cost for %s: cost=%v telemetry=%#v replay=%s", tc.providerID, cost, telemetry, replay)
		}
		if !boolField(telemetry.Usage, "priceConfigured") {
			t.Fatalf("configured provider price should mark usage as priceConfigured for %s: telemetry=%#v replay=%s", tc.providerID, telemetry, replay)
		}
	}
	threadUsage := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/usage?group_by=thread&thread_id="+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	buckets, ok := threadUsage["buckets"].([]any)
	if !ok || len(buckets) != 1 {
		t.Fatalf("thread usage should return one priced bucket: %#v", threadUsage)
	}
	if !boolField(buckets[0].(map[string]any), "price_configured") {
		t.Fatalf("thread usage bucket should preserve configured price status: %#v", threadUsage)
	}
}

func TestRuntimeServerCacheDiagnosticsIsolateModelNamespaces(t *testing.T) {
	dataDir := t.TempDir()
	upstream := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"content":"first"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":1,"total_tokens":11,"prompt_cache_hit_tokens":1,"prompt_cache_miss_tokens":9}}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"second"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":12,"completion_tokens":2,"total_tokens":14,"prompt_cache_hit_tokens":4,"prompt_cache_miss_tokens":8}}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ProviderID:         "deepseek",
		BaseURL:            upstream.URL() + "/v1",
		APIKey:             "test-provider-key",
		EndpointFormat:     "chat_completions",
		Model:              "deepseek-chat",
		ModelProvidersJSON: testModelProvidersJSONWithModels(upstream.URL()+"/v1", "deepseek", "deepseek-chat", "deepseek-reasoner"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Cache diagnostics",
		"workspace":  dataDir,
		"providerId": "deepseek",
		"model":      "deepseek-chat",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "first cache turn",
		"providerId": "deepseek",
		"model":      "deepseek-chat",
	}), http.StatusAccepted)
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "second cache turn",
		"providerId": "deepseek",
		"model":      "deepseek-reasoner",
	}), http.StatusAccepted)

	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	diagnostics := []map[string]any{}
	for _, event := range parseRuntimeServerSSEEvents(t, replay) {
		if stringField(event, "kind") == "usage" {
			diagnostics = append(diagnostics, mapField(t, event, "cacheDiagnostics"))
		}
	}
	if len(diagnostics) != 2 {
		t.Fatalf("expected two usage diagnostics events, got %#v\n%s", diagnostics, replay)
	}
	for index, diagnostic := range diagnostics {
		if !boolField(diagnostic, "providerAttemptTelemetryValid") ||
			diagnostic["providerAttemptCount"] != float64(1) || diagnostic["providerLogicalCallCount"] != float64(1) {
			t.Fatalf("usage diagnostic %d did not bind its one real provider attempt: %#v", index, diagnostic)
		}
		statuses := mapField(t, diagnostic, "providerAttemptStatuses")
		if statuses["succeeded"] != float64(1) || statuses["failed"] != float64(0) {
			t.Fatalf("usage diagnostic %d has incorrect terminal attempt counts: %#v", index, statuses)
		}
	}
	if boolField(diagnostics[0], "prefixChanged") {
		t.Fatalf("first usage diagnostics should not report prefix churn: %#v", diagnostics[0])
	}
	if boolField(diagnostics[1], "prefixChanged") {
		t.Fatalf("different model namespaces must not be compared as prefix churn: %#v", diagnostics[1])
	}
	reasons, _ := diagnostics[1]["prefixChangeReasons"].([]any)
	if len(reasons) != 0 {
		t.Fatalf("different model namespaces must not inherit prefix-change reasons: %#v", diagnostics[1])
	}
	firstNamespace := stringField(diagnostics[0], "cacheProviderNamespaceDigest")
	secondNamespace := stringField(diagnostics[1], "cacheProviderNamespaceDigest")
	if firstNamespace == "" || secondNamespace == "" || firstNamespace == secondNamespace {
		t.Fatalf("different models must produce distinct non-empty provider namespace digests: %#v", diagnostics)
	}
	if diagnostics[1]["cacheHitTokens"] != float64(4) || diagnostics[1]["cacheMissTokens"] != float64(8) {
		t.Fatalf("second diagnostics should preserve DeepSeek cache hit/miss tokens: %#v", diagnostics[1])
	}
}

func TestRuntimeServerLiveSSEWithholdsProviderDraftUntilTurnCompletes(t *testing.T) {
	dataDir := t.TempDir()
	firstFrameSent := make(chan struct{})
	releaseProvider := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(releaseProvider) })
	var firstOnce sync.Once
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"live token"}}]}` + "\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		firstOnce.Do(func() { close(firstFrameSent) })
		<-releaseProvider
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[],"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}))
	defer provider.Close()

	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL, "openai-live", "gpt-live"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":     "Live SSE",
		"workspace": dataDir,
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/threads/"+threadID+"/events?since_seq=0&live=1", nil)
	if err != nil {
		t.Fatalf("build live SSE request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open live SSE: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("live SSE status mismatch: %d", resp.StatusCode)
	}
	seenFinal := make(chan string, 1)
	readErr := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(resp.Body)
		var frame strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				readErr <- err
				return
			}
			if strings.TrimSpace(line) == "" {
				text := frame.String()
				if strings.Contains(text, "event: general_terminal_batch") {
					seenFinal <- text
					return
				}
				frame.Reset()
				continue
			}
			frame.WriteString(line)
		}
	}()

	turnDone := make(chan error, 1)
	go func() {
		body := mustJSONNoTest(map[string]any{"prompt": "Stream one token", "providerId": "openai-live", "model": "gpt-live"})
		req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/threads/"+threadID+"/turns", strings.NewReader(string(body)))
		if err != nil {
			turnDone <- err
			return
		}
		req.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			turnDone <- err
			return
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusAccepted {
			turnDone <- fmt.Errorf("turn status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
			return
		}
		turnDone <- nil
	}()

	select {
	case <-firstFrameSent:
	case <-time.After(runtimeServerPositiveTestTimeout):
		t.Fatal("provider did not flush first SSE frame")
	}
	select {
	case frame := <-seenFinal:
		t.Fatalf("provider draft reached live SSE before terminal publication: %s", frame)
	case err := <-readErr:
		t.Fatalf("live SSE closed before provider completion: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	replayBeforeRelease := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	for _, forbidden := range []string{"event: general_terminal_batch", "event: assistant_text_delta", "live token"} {
		if strings.Contains(replayBeforeRelease, forbidden) {
			t.Fatalf("provider draft crossed the public seam before terminal commit as %q:\n%s", forbidden, replayBeforeRelease)
		}
	}
	releaseOnce.Do(func() { close(releaseProvider) })
	select {
	case err := <-turnDone:
		if err != nil {
			t.Fatalf("turn failed after provider release: %v", err)
		}
	case <-time.After(runtimeServerPositiveTestTimeout):
		t.Fatal("turn did not complete after provider release")
	}
	select {
	case frame := <-seenFinal:
		assertRuntimeServerTypedOrdinaryTerminal(t, frame, "turn_1", "live token", domainordinaryresult.ResultSlotOriginProviderOrdinaryOnlyV1)
	case err := <-readErr:
		t.Fatalf("live SSE closed before atomic final: %v", err)
	case <-time.After(runtimeServerPositiveTestTimeout):
		t.Fatal("live SSE did not receive atomic final after provider completed")
	}
}

func TestRuntimeServerAsyncTurnResponseReturnsBeforeProviderCompletes(t *testing.T) {
	dataDir := t.TempDir()
	firstFrameSent := make(chan struct{})
	releaseProvider := make(chan struct{})
	var firstOnce sync.Once
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseProvider) }) })
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"async live token"},"finish_reason":null}]}` + "\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		firstOnce.Do(func() { close(firstFrameSent) })
		<-releaseProvider
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}))
	defer provider.Close()

	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL, "openai-async", "gpt-async"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":     "Async GUI turn",
		"workspace": dataDir,
	}), http.StatusCreated)
	threadID := stringField(thread, "id")

	turnDone := make(chan map[string]any, 1)
	turnErr := make(chan error, 1)
	go func() {
		body := mustJSONNoTest(map[string]any{
			"prompt":     "Return before provider completes",
			"providerId": "openai-async",
			"model":      "gpt-async",
			"async":      true,
		})
		req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/threads/"+threadID+"/turns", strings.NewReader(string(body)))
		if err != nil {
			turnErr <- err
			return
		}
		req.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			turnErr <- err
			return
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusAccepted {
			turnErr <- fmt.Errorf("turn status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
			return
		}
		var payload map[string]any
		if err := json.Unmarshal(data, &payload); err != nil {
			turnErr <- err
			return
		}
		turnDone <- payload
	}()

	var started map[string]any
	select {
	case started = <-turnDone:
	case err := <-turnErr:
		t.Fatalf("async turn failed before provider release: %v", err)
	case <-time.After(runtimeServerPositiveTestTimeout):
		t.Fatal("async turn response waited for provider completion")
	}
	turnID := stringField(started, "turnId")
	if turnID == "" {
		t.Fatalf("async turn response missing turn id: %#v", started)
	}
	select {
	case <-firstFrameSent:
	case <-time.After(runtimeServerPositiveTestTimeout):
		t.Fatal("provider did not receive async turn after response")
	}
	replayBeforeRelease := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if strings.Contains(replayBeforeRelease, "event: assistant_text_delta") || strings.Contains(replayBeforeRelease, "async live token") {
		releaseOnce.Do(func() { close(releaseProvider) })
		t.Fatalf("async turn exposed a provider draft before completion:\n%s", replayBeforeRelease)
	}
	releaseOnce.Do(func() { close(releaseProvider) })
	var replayAfterRelease string
	var events []map[string]any
	for i := 0; i < 100; i++ {
		replayAfterRelease = liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
		events = runtimeServerEventsForTurn(t, replayAfterRelease, turnID)
		if hasRuntimeServerEvent(events, "turn_completed") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !hasRuntimeServerEvent(events, "turn_completed") {
		t.Fatalf("async turn did not complete after provider release:\n%s", replayAfterRelease)
	}
	assertRuntimeServerTypedOrdinaryTerminal(t, replayAfterRelease, turnID, "async live token", domainordinaryresult.ResultSlotOriginProviderOrdinaryOnlyV1)
}

func TestRuntimeServerAutoTitleEmitsThreadUpdatedLifecycle(t *testing.T) {
	dataDir := t.TempDir()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"title ok"},"finish_reason":"stop"}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer provider.Close()

	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL, "openai-title", "gpt-title"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"autoTitle":  true,
		"workspace":  dataDir,
		"providerId": "openai-title",
		"model":      "gpt-title",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	if threadID == "" {
		t.Fatalf("created thread missing id: %#v", thread)
	}

	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "Derive title from the first message",
		"providerId": "openai-title",
		"model":      "gpt-title",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	if turnID == "" {
		t.Fatalf("start turn response missing turn id: %#v", start)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if !strings.Contains(replay, "event: thread_updated") ||
		strings.Contains(replay, `"title":"Derive title from the first message"`) {
		t.Fatalf("auto-title lifecycle must be status-only:\n%s", replay)
	}
	if !strings.Contains(replay, "event: turn_started") {
		t.Fatalf("auto-title replay missing turn_started:\n%s", replay)
	}
	canonical := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	if canonical["title"] != "Derive title from the first message" {
		t.Fatalf("canonical thread lost its auto title: %#v", canonical)
	}
}

func TestRuntimeServerLiveSSEDoesNotPublishPartialToolCallBeforeProviderCompletes(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("note content"), 0o644); err != nil {
		t.Fatal(err)
	}
	firstFrameSent := make(chan struct{})
	releaseProvider := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(releaseProvider) })
	var firstOnce sync.Once
	var mu sync.Mutex
	requestCount := 0
	bodies := []string{}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		requestCount++
		attempt := requestCount
		bodies = append(bodies, string(body))
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		if attempt == 1 {
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_live_read","type":"function","function":{"name":"read_file","arguments":"{\"path\""}}]}}]}` + "\n\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			firstOnce.Do(func() { close(firstFrameSent) })
			<-releaseProvider
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"note.txt\"}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n"))
			_, _ = w.Write([]byte(`data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":1,"total_tokens":11}}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			return
		}
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"tool done"},"finish_reason":"stop"}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[],"usage":{"prompt_tokens":20,"completion_tokens":2,"total_tokens":22}}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}))
	defer provider.Close()

	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL, "openai-live", "gpt-live"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Live Tool SSE",
		"workspace":  workspace,
		"providerId": "openai-live",
		"model":      "gpt-live",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/threads/"+threadID+"/events?since_seq=0&live=1", nil)
	if err != nil {
		t.Fatalf("build live SSE request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open live SSE: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("live SSE status mismatch: %d", resp.StatusCode)
	}
	seenPartial := make(chan string, 1)
	readErr := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(resp.Body)
		var frame strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				readErr <- err
				return
			}
			if strings.TrimSpace(line) == "" {
				text := frame.String()
				if strings.Contains(text, "event: tool_call_ready") &&
					strings.Contains(text, "read_file") {
					seenPartial <- text
					return
				}
				frame.Reset()
				continue
			}
			frame.WriteString(line)
		}
	}()

	turnDone := make(chan error, 1)
	go func() {
		body := mustJSONNoTest(map[string]any{"prompt": "Read the note."})
		req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/threads/"+threadID+"/turns", strings.NewReader(string(body)))
		if err != nil {
			turnDone <- err
			return
		}
		req.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			turnDone <- err
			return
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusAccepted {
			turnDone <- fmt.Errorf("turn status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
			return
		}
		turnDone <- nil
	}()

	select {
	case <-firstFrameSent:
	case <-time.After(runtimeServerPositiveTestTimeout):
		t.Fatal("provider did not flush first tool-call SSE frame")
	}
	select {
	case frame := <-seenPartial:
		t.Fatalf("partial provider tool-call data reached SSE before validation: %s", frame)
	case err := <-readErr:
		t.Fatalf("live SSE closed while provider tool-call arguments were incomplete: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(releaseProvider) })
	validatedCallID := ""
	select {
	case frame := <-seenPartial:
		frameEvents := parseRuntimeServerSSEEvents(t, frame)
		if len(frameEvents) != 1 || stringField(frameEvents[0], "kind") != "tool_call_ready" || stringField(frameEvents[0], "toolName") != "read_file" {
			t.Fatalf("unexpected validated tool-call frame: %s", frame)
		}
		validatedCallID = stringField(frameEvents[0], "callId")
		if !domainsecurity.IsHostToolCallIDV1(validatedCallID) || validatedCallID == "call_live_read" || strings.Contains(frame, "call_live_read") {
			t.Fatalf("validated tool-call frame did not use a fresh host identity: %s", frame)
		}
	case err := <-readErr:
		t.Fatalf("live SSE closed before the validated tool-call event: %v", err)
	case <-time.After(runtimeServerPositiveTestTimeout):
		t.Fatal("live SSE did not receive the validated tool-call event after provider completion")
	}
	select {
	case err := <-turnDone:
		if err != nil {
			t.Fatalf("turn failed after provider release: %v", err)
		}
	case <-time.After(runtimeServerPositiveTestTimeout):
		t.Fatal("turn did not complete after provider release")
	}
	mu.Lock()
	totalRequests := requestCount
	capturedBodies := append([]string(nil), bodies...)
	mu.Unlock()
	if totalRequests != 2 || len(capturedBodies) != 2 {
		t.Fatalf("validated tool-call loop should continue with a second provider request, got %d bodies=%#v", totalRequests, capturedBodies)
	}
	historyCallID := providerHostToolCallIDForName(t, capturedBodies[1], "read_file")
	if historyCallID != validatedCallID || strings.Contains(capturedBodies[1], "call_live_read") {
		t.Fatalf("SSE and provider history did not preserve the same host call identity: sse=%q history=%q body=%s", validatedCallID, historyCallID, capturedBodies[1])
	}
}

func TestRuntimeServerOpenAICompatibleTextTurnDoesNotEmitReasoning(t *testing.T) {
	dataDir := t.TempDir()
	provider := newCompleteProviderServer(t, [][]string{{
		`data: {"choices":[{"delta":{"content":"plain text only"},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":8,"completion_tokens":3,"total_tokens":11}}`,
		`data: [DONE]`,
	}})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "openai-plain", "gpt-plain"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":     "Plain OpenAI text",
		"workspace": dataDir,
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "Return plain text",
		"providerId": "openai-plain",
		"model":      "gpt-plain",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	if turnID == "" {
		t.Fatalf("turn did not start: %#v", start)
	}

	body := provider.Body(0)
	if strings.Contains(body, `"thinking"`) || strings.Contains(body, `"reasoning_effort"`) {
		t.Fatalf("DeepSeek-only request fields leaked into OpenAI-compatible turn: %s", body)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	turnEvents := runtimeServerEventsForTurn(t, replay, turnID)
	kinds := []string{}
	for _, event := range turnEvents {
		kinds = append(kinds, stringField(event, "kind"))
	}
	assertRuntimeServerTypedOrdinaryTerminal(t, replay, turnID, "plain text only", domainordinaryresult.ResultSlotOriginProviderOrdinaryOnlyV1)
	if containsString(kinds, "assistant_reasoning_delta") || strings.Contains(replay, "assistant_reasoning") {
		t.Fatalf("ordinary OpenAI-compatible text turn must not emit reasoning events/cards: kinds=%#v replay=%s", kinds, replay)
	}
	usageEvent := firstRuntimeServerEvent(t, turnEvents, "usage")
	usage := mapField(t, usageEvent, "usage")
	if usage["cacheHitRate"] != nil || usage["cacheableTokenHitRate"] != nil || usage["totalInputTokenHitRate"] != nil {
		t.Fatalf("OpenAI-compatible usage without cache telemetry must keep cache rates unknown: %#v", usage)
	}
	if _, ok := usage["cacheHitTokens"]; ok {
		t.Fatalf("OpenAI-compatible usage without cache telemetry must not guess cacheHitTokens=0: %#v", usage)
	}
	if _, ok := usage["cacheMissTokens"]; ok {
		t.Fatalf("OpenAI-compatible usage without cache telemetry must not guess cacheMissTokens=0: %#v", usage)
	}
	if !containsString(kinds, "usage") || !containsString(kinds, "turn_completed") {
		t.Fatalf("OpenAI-compatible text turn missing usage/completion events: kinds=%#v replay=%s", kinds, replay)
	}
}

func TestRuntimeServerProviderRetryIsVisible(t *testing.T) {
	dataDir := t.TempDir()
	var mu sync.Mutex
	requestCount := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		attempt := requestCount
		mu.Unlock()
		if attempt == 1 {
			http.Error(w, "temporary upstream failure", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"retry ok"},"finish_reason":"stop"}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[],"usage":{"prompt_tokens":12,"completion_tokens":2,"total_tokens":14}}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer provider.Close()

	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL, "openai-live", "gpt-live"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":     "Provider Retry",
		"workspace": dataDir,
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	turn := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "Retry once",
		"providerId": "openai-live",
		"model":      "gpt-live",
	}), http.StatusAccepted)
	turnID := stringField(turn, "turnId")

	var replay string
	var events []map[string]any
	for i := 0; i < 100; i++ {
		replay = liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
		events = runtimeServerEventsForTurn(t, replay, turnID)
		if hasRuntimeServerEvent(events, "turn_completed") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !hasRuntimeServerEvent(events, "turn_completed") {
		t.Fatalf("retry turn did not complete:\n%s", replay)
	}
	retryEvent := firstRuntimeServerPipelineStage(t, events, "provider_retrying")
	if stringField(retryEvent, "stage") != "provider_retrying" {
		t.Fatalf("expected visible provider retrying stage, got %#v", retryEvent)
	}
	retryDetails := mapField(t, retryEvent, "details")
	retryProviderError := mapField(t, retryDetails, "providerError")
	if stringField(retryProviderError, "kind") != "server" ||
		floatField(t, retryProviderError, "status") != http.StatusServiceUnavailable ||
		!boolField(retryProviderError, "retryable") {
		t.Fatalf("provider retry event should carry structured retryable diagnostics: %#v", retryProviderError)
	}
	assertRuntimeServerTypedOrdinaryTerminal(t, replay, turnID, "retry ok", domainordinaryresult.ResultSlotOriginProviderOrdinaryOnlyV1)
	mu.Lock()
	totalRequests := requestCount
	mu.Unlock()
	if totalRequests != 2 {
		t.Fatalf("expected one retry request, got %d", totalRequests)
	}
}

func TestRuntimeServerPreOutputStreamReplayIsVisible(t *testing.T) {
	dataDir := t.TempDir()
	var mu sync.Mutex
	requestCount := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		attempt := requestCount
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		if attempt == 1 {
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			return
		}
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"stream replay ok"},"finish_reason":"stop"}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[],"usage":{"prompt_tokens":9,"completion_tokens":3,"total_tokens":12}}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer provider.Close()

	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL, "openai-stream-replay", "gpt-stream-replay"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":     "Pre-output stream replay",
		"workspace": dataDir,
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	turn := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "Replay stream before output",
		"providerId": "openai-stream-replay",
		"model":      "gpt-stream-replay",
	}), http.StatusAccepted)
	turnID := stringField(turn, "turnId")

	var replay string
	var events []map[string]any
	for i := 0; i < 100; i++ {
		replay = liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
		events = runtimeServerEventsForTurn(t, replay, turnID)
		if hasRuntimeServerEvent(events, "turn_completed") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !hasRuntimeServerEvent(events, "turn_completed") {
		t.Fatalf("stream replay turn did not complete:\n%s", replay)
	}
	retryEvent := firstRuntimeServerPipelineStage(t, events, "provider_retrying")
	if stringField(retryEvent, "stage") != "provider_retrying" ||
		floatField(t, retryEvent, "attempt") != 2 ||
		floatField(t, retryEvent, "maxAttempt") < 2 {
		t.Fatalf("pre-output stream replay should emit visible provider_retrying event, got %#v", retryEvent)
	}
	assertRuntimeServerTypedOrdinaryTerminal(t, replay, turnID, "stream replay ok", domainordinaryresult.ResultSlotOriginProviderOrdinaryOnlyV1)
	mu.Lock()
	totalRequests := requestCount
	mu.Unlock()
	if totalRequests != 2 {
		t.Fatalf("expected one internal stream replay request, got %d", totalRequests)
	}
}

func TestRuntimeServerPostOutputProviderInterruptionRecoversWithTailPrompt(t *testing.T) {
	dataDir := t.TempDir()
	var mu sync.Mutex
	requestCount := 0
	bodies := []string{}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		requestCount++
		attempt := requestCount
		bodies = append(bodies, string(body))
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		if attempt == 1 {
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"partial before stream cut "}}]}` + "\n\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			return
		}
		if attempt == 2 {
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"second partial before stream cut "}}]}` + "\n\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			return
		}
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"Complete self-contained replacement after recovery."},"finish_reason":"stop"}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[],"usage":{"prompt_tokens":14,"completion_tokens":4,"total_tokens":18}}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer provider.Close()

	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL, "openai-cut", "gpt-cut"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":     "Post-output interruption",
		"workspace": dataDir,
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	turn := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":        "Stream then cut",
		"providerId":    "openai-cut",
		"model":         "gpt-cut",
		"maxModelSteps": 1,
	}), http.StatusAccepted)
	turnID := stringField(turn, "turnId")

	var replay string
	var events []map[string]any
	for i := 0; i < 100; i++ {
		replay = liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
		events = runtimeServerEventsForTurn(t, replay, turnID)
		if hasRuntimeServerEvent(events, "turn_completed") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !hasRuntimeServerEvent(events, "turn_completed") {
		t.Fatalf("post-output stream recovery turn did not complete:\n%s", replay)
	}
	mu.Lock()
	totalRequests := requestCount
	capturedBodies := append([]string(nil), bodies...)
	mu.Unlock()
	if totalRequests != 3 || len(capturedBodies) != 3 {
		t.Fatalf("post-output stream recovery should survive two bounded retries, got %d bodies=%#v", totalRequests, capturedBodies)
	}
	if strings.Contains(replay, "partial before stream cut ") ||
		strings.Contains(replay, "second partial before stream cut ") ||
		strings.Contains(replay, "Complete self-contained replacement after recovery.") ||
		strings.Contains(replay, "assistant_text_delta") ||
		!strings.Contains(replay, "The bounded recovery path ended without a verified final response.") ||
		!strings.Contains(replay, `"stage":"provider_retrying"`) ||
		!strings.Contains(replay, `"recoveryKind":"interrupted_stream"`) {
		t.Fatalf("post-output recovery must discard provider prose and publish only the fixed host boundary:\n%s", replay)
	}
	recoveryEvents := []map[string]any{}
	for _, event := range events {
		if stringField(event, "kind") != "pipeline_stage" || stringField(event, "stage") != "provider_retrying" {
			continue
		}
		details := mapField(t, event, "details")
		if stringField(details, "recoveryKind") == "interrupted_stream" {
			recoveryEvents = append(recoveryEvents, event)
		}
	}
	if len(recoveryEvents) != 2 ||
		floatField(t, recoveryEvents[0], "attempt") != 1 ||
		floatField(t, recoveryEvents[1], "attempt") != 2 ||
		floatField(t, recoveryEvents[0], "maxAttempt") != 3 ||
		floatField(t, recoveryEvents[1], "maxAttempt") != 3 {
		t.Fatalf("post-output stream recovery should record two of three bounded attempts, got %#v", recoveryEvents)
	}
	if strings.Contains(replay, `"stage":"provider_error"`) {
		t.Fatalf("recovered post-output stream should not end with provider_error:\n%s", replay)
	}
	if strings.Contains(capturedBodies[1], "partial before stream cut ") ||
		strings.Contains(capturedBodies[2], "second partial before stream cut ") ||
		!strings.Contains(capturedBodies[1], "discarded and was never shown") ||
		!strings.Contains(capturedBodies[1], "complete, self-contained replacement") ||
		!strings.Contains(capturedBodies[2], "discarded and was never shown") ||
		!strings.Contains(capturedBodies[2], "complete, self-contained replacement") {
		t.Fatalf("recovery requests must exclude private partials and require complete replacements, bodies:\n%#v", capturedBodies)
	}
}

func TestRuntimeServerPartialToolCallInterruptionRecoversWithoutExecutingPartialTool(t *testing.T) {
	dataDir := t.TempDir()
	var mu sync.Mutex
	requestCount := 0
	bodies := []string{}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		requestCount++
		attempt := requestCount
		bodies = append(bodies, string(body))
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		if attempt == 1 {
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_cut_tool","type":"function","function":{"name":"read_file","arguments":"{\"path\""}}]}}]}` + "\n\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			return
		}
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"recovered without partial tool"},"finish_reason":"stop"}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[],"usage":{"prompt_tokens":15,"completion_tokens":5,"total_tokens":20}}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer provider.Close()

	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL, "openai-cut-tool", "gpt-cut-tool"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":     "Partial tool interruption",
		"workspace": dataDir,
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	turn := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "Start a tool then cut",
		"providerId": "openai-cut-tool",
		"model":      "gpt-cut-tool",
	}), http.StatusAccepted)
	turnID := stringField(turn, "turnId")

	var replay string
	var events []map[string]any
	for i := 0; i < 100; i++ {
		replay = liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
		events = runtimeServerEventsForTurn(t, replay, turnID)
		if hasRuntimeServerEvent(events, "turn_completed") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !hasRuntimeServerEvent(events, "turn_completed") {
		t.Fatalf("partial tool stream recovery turn did not complete:\n%s", replay)
	}
	mu.Lock()
	totalRequests := requestCount
	capturedBodies := append([]string(nil), bodies...)
	mu.Unlock()
	if totalRequests != 2 || len(capturedBodies) != 2 {
		t.Fatalf("partial tool stream recovery should make one bounded retry, got %d bodies=%#v", totalRequests, capturedBodies)
	}
	if strings.Contains(replay, "tool_call_ready") ||
		strings.Contains(replay, "call_cut_tool") {
		t.Fatalf("incomplete tool identity/arguments must not reach SSE or durable history:\n%s", replay)
	}
	if !strings.Contains(replay, `"stage":"provider_retrying"`) ||
		!strings.Contains(replay, `"recoveryKind":"interrupted_stream"`) ||
		!strings.Contains(replay, "The bounded recovery path ended without a verified final response.") ||
		strings.Contains(replay, "recovered without partial tool") {
		t.Fatalf("partial tool stream recovery should keep only the bounded recovery event and fixed host boundary:\n%s", replay)
	}
	if strings.Contains(replay, `"kind":"tool_call_finished"`) ||
		strings.Contains(replay, `"stage":"provider_error"`) {
		t.Fatalf("partial tool stream recovery must not execute the partial tool or end in provider_error:\n%s", replay)
	}
	if strings.Contains(capturedBodies[1], `"tool_calls"`) ||
		!strings.Contains(capturedBodies[1], "fresh complete tool call from scratch") {
		t.Fatalf("recovery request should not carry partial tool_call history and should include fresh-call instruction, body:\n%s", capturedBodies[1])
	}
}

func TestRuntimeServerCompactRewritesHistoryAndPreservesCacheSafeSummaryForProvider(t *testing.T) {
	dataDir := t.TempDir()
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"content":"old answer 1"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":20,"completion_tokens":3,"total_tokens":23}}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"old answer 2"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":21,"completion_tokens":3,"total_tokens":24}}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"tail answer 3"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":22,"completion_tokens":3,"total_tokens":25}}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"tail answer 4"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":23,"completion_tokens":3,"total_tokens":26}}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"after compact"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":24,"completion_tokens":3,"total_tokens":27}}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "openai-compact", "gpt-compact"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":     "Compact history",
		"workspace": dataDir,
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	for index := 1; index <= 4; index++ {
		assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
			"prompt":     fmt.Sprintf("old prompt %d", index),
			"providerId": "openai-compact",
			"model":      "gpt-compact",
		}), http.StatusAccepted)
	}

	compact := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/compact", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"reason": "manual test",
	}), http.StatusOK)
	if compact["ok"] != true ||
		floatField(t, compact, "replacedTokens") <= 0 ||
		stringField(compact, "sourceDigest") == "" ||
		stringField(compact, "digestMarker") == "" ||
		len(anyList(compact["sourceItemIds"])) == 0 {
		t.Fatalf("compact response must expose Kun-compatible compaction fields: %#v", compact)
	}
	if stringField(compact, "summary") != appturn.GeneralCompactionSummaryTextV3 ||
		strings.Contains(stringField(compact, "summary"), "old prompt 1") ||
		!containsAnyString(anyList(compact["pinnedConstraints"]), "user: preserve recent turns") {
		t.Fatalf("compact response summary/pinned constraints mismatch: %#v", compact)
	}

	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if !strings.Contains(replay, "event: compaction_started") ||
		!strings.Contains(replay, "event: compaction_completed") ||
		!strings.Contains(replay, stringField(compact, "sourceDigest")) {
		t.Fatalf("compaction replay missing started/completed fields:\n%s", replay)
	}
	if strings.Contains(replay, `"itemId":"`+stringField(compact, "itemId")+`"`) {
		t.Fatalf("compaction replay bypassed the closed public event projection:\n%s", replay)
	}

	compactedThread := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	turns := anyList(compactedThread["turns"])
	if len(turns) != 3 {
		t.Fatalf("compaction should rewrite visible history to summary + two tail turns: %#v", compactedThread)
	}
	firstTurn, _ := turns[0].(map[string]any)
	firstItems := anyList(firstTurn["items"])
	if len(firstItems) != 1 {
		t.Fatalf("compaction turn should contain the summary item: %#v", firstTurn)
	}
	summaryItem, _ := firstItems[0].(map[string]any)
	if stringField(summaryItem, "kind") != "compaction" ||
		floatField(t, summaryItem, "replacedTokens") <= 0 ||
		stringField(summaryItem, "sourceDigest") != stringField(compact, "sourceDigest") {
		t.Fatalf("compaction summary item mismatch: %#v", summaryItem)
	}
	tailOne, _ := turns[1].(map[string]any)
	tailTwo, _ := turns[2].(map[string]any)
	if !strings.Contains(stringField(tailOne, "prompt"), "old prompt 3") ||
		!strings.Contains(stringField(tailTwo, "prompt"), "old prompt 4") {
		t.Fatalf("compaction should preserve recent tail turns: %#v", turns)
	}

	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "continue after compact",
		"providerId": "openai-compact",
		"model":      "gpt-compact",
	}), http.StatusAccepted)
	body := provider.Body(4)
	if !strings.Contains(body, "[Compacted conversation summary]") ||
		!strings.Contains(body, appturn.GeneralCompactionSummaryTextV3) ||
		!strings.Contains(body, "continue after compact") {
		t.Fatalf("provider request must include compaction summary and latest prompt:\n%s", body)
	}
	for _, forbidden := range []string{"old prompt 1", "old answer 1", "old prompt 2", "old answer 2"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("provider request replayed compacted source prose %q:\n%s", forbidden, body)
		}
	}
	var request map[string]any
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		t.Fatalf("provider request body should be JSON: %v\n%s", err, body)
	}
	messages := anyList(request["messages"])
	if len(messages) == 0 {
		t.Fatalf("provider request should include messages after compaction:\n%s", body)
	}
	summaryMessages := 0
	standaloneCompactedHistory := []string{}
	seenTailPrompt := false
	tailOrdinaryResults := 0
	seenLatestPrompt := false
	for _, value := range messages {
		message, _ := value.(map[string]any)
		content := strings.TrimSpace(stringField(message, "content"))
		if strings.HasPrefix(content, "[Compacted conversation summary]") {
			summaryMessages++
		}
		switch content {
		case "old prompt 1", "old answer 1", "old prompt 2", "old answer 2":
			standaloneCompactedHistory = append(standaloneCompactedHistory, content)
		case "old prompt 3", "old prompt 4":
			seenTailPrompt = true
		case "tail answer 3", "tail answer 4":
			tailOrdinaryResults++
		case "continue after compact":
			seenLatestPrompt = true
		}
	}
	if summaryMessages != 1 {
		t.Fatalf("provider request should include exactly one compaction summary message, got %d:\n%s", summaryMessages, body)
	}
	if len(standaloneCompactedHistory) != 0 {
		t.Fatalf("compacted source turns must not be replayed as standalone provider messages: %#v\n%s", standaloneCompactedHistory, body)
	}
	if !seenTailPrompt || tailOrdinaryResults != 2 || !seenLatestPrompt {
		t.Fatalf("provider request should preserve recent tail turns and latest prompt after compaction:\n%s", body)
	}
}

func TestRuntimeServerProviderAuthErrorIsStructuredAndRedacted(t *testing.T) {
	const secret = "sk-runtime-provider-secret"
	dataDir := t.TempDir()
	var providerCalls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls.Add(1)
		if r.Header.Get("Authorization") != "Bearer "+secret {
			t.Error("synthetic Provider did not receive the Registry-owned fixture credential")
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Authentication Fails, Your api key: ` + secret + ` is invalid"}}`))
	}))
	defer provider.Close()

	modelProviders, connectProviderRegistry := prepareRuntimeServerExplicitProviderRegistryCredentialFixture(
		t, dataDir, provider.URL, "auth-provider", "auth-model", secret,
	)
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: modelProviders,
	}))
	defer server.Close()
	connectProviderRegistry(server.URL)

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":     "Provider Auth",
		"workspace": dataDir,
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	failed := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "Trigger auth",
		"providerId": "auth-provider",
		"model":      "auth-model",
	}), http.StatusInternalServerError)
	assertClosedTurnFailureResponse(t, failed, "provider_authentication_failed", "Provider authentication failed. Check the configured credential.")
	if providerCalls.Load() != 1 {
		t.Fatalf("expected one request to the synthetic Provider, got %d", providerCalls.Load())
	}
	if _, exists := failed["providerError"]; exists {
		t.Fatalf("closed public failure response must not expose provider diagnostics: %#v", failed)
	}
	failedJSON := string(mustJSON(t, failed))
	if strings.Contains(failedJSON, secret) {
		t.Fatalf("provider auth response leaked API key: %s", failedJSON)
	}

	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if strings.Contains(replay, secret) {
		t.Fatalf("provider auth replay leaked API key:\n%s", replay)
	}
	var providerStage map[string]any
	for _, event := range parseRuntimeServerSSEEvents(t, replay) {
		if stringField(event, "kind") == "pipeline_stage" && stringField(event, "stage") == "provider_error" {
			providerStage = event
			break
		}
	}
	if providerStage == nil {
		t.Fatalf("missing provider_error pipeline stage in replay:\n%s", replay)
	}
	stageProviderError := mapField(t, mapField(t, providerStage, "details"), "providerError")
	if stringField(stageProviderError, "kind") != "auth" ||
		stringField(stageProviderError, "authStatus") != "required" ||
		floatField(t, stageProviderError, "status") != http.StatusUnauthorized {
		t.Fatalf("provider_error stage should carry structured auth diagnostics: %#v", stageProviderError)
	}
}

func TestRuntimeServerRequestProviderCannotOverrideCommittedSelection(t *testing.T) {
	dataDir := t.TempDir()
	var providerCalls int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&providerCalls, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"committed provider\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
	}))
	defer provider.Close()

	modelProviders := string(mustJSON(t, map[string]any{
		"defaultProviderId": "configured-provider",
		"providers": []map[string]any{{
			"id":             "configured-provider",
			"apiKey":         "sk-configured",
			"baseUrl":        provider.URL + "/v1",
			"endpointFormat": "chat_completions",
			"models":         []string{"configured-model"},
		}},
	}))
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: modelProviders,
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":     "Missing Provider",
		"workspace": dataDir,
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	failed := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "Do not fall back",
		"providerId": "missing-provider",
		"model":      "missing-model",
	}), http.StatusInternalServerError)

	assertClosedTurnFailureResponse(t, failed, "provider_model_invalid", "The selected model is not configured for this provider.")
	if got := atomic.LoadInt32(&providerCalls); got != 0 {
		t.Fatalf("missing provider must not call configured fallback provider, got %d calls", got)
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Use the committed selection", "providerId": "missing-provider", "model": "configured-model",
	}), http.StatusAccepted)
	if got := atomic.LoadInt32(&providerCalls); got != 1 {
		t.Fatalf("request Provider must retain the committed execution route, calls=%d", got)
	}
}

func TestRuntimeServerEmptyFinalRecoveryUsesHostBoundary(t *testing.T) {
	dataDir := t.TempDir()
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"recovered final"},"finish_reason":"stop"}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":16,"completion_tokens":2,"total_tokens":18}}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "openai-empty", "gpt-empty"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":     "Empty Final",
		"workspace": dataDir,
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	turn := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "Return nothing",
		"providerId": "openai-empty",
		"model":      "gpt-empty",
	}), http.StatusAccepted)
	turnID := stringField(turn, "turnId")

	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	events := runtimeServerEventsForTurn(t, replay, turnID)
	stage := firstRuntimeServerPipelineStage(t, events, "empty_final_recovered")
	if stringField(stage, "stage") != "empty_final_recovered" {
		t.Fatalf("expected visible empty-final recovery stage, got %#v", stage)
	}
	recoveryDetails := mapField(t, stage, "details")
	if stringField(recoveryDetails, "recoveryKind") != "empty_final" ||
		floatField(t, recoveryDetails, "recoveryAttempt") != 1 ||
		floatField(t, recoveryDetails, "maxRecoveryAttempts") != 1 {
		t.Fatalf("empty-final recovery should carry bounded recovery diagnostics: %#v", recoveryDetails)
	}
	if !hasRuntimeServerEvent(events, "turn_completed") {
		t.Fatalf("empty-final turn should still complete:\n%s", replay)
	}
	if !hasRuntimeServerEvent(events, "item_completed") ||
		!strings.Contains(replay, "The bounded recovery path ended without a verified final response.") ||
		strings.Contains(replay, "recovered final") || strings.Contains(replay, "assistant_text_delta") {
		t.Fatalf("empty-final recovery must publish only the fixed host boundary:\n%s", replay)
	}
	if provider.RequestCount() != 2 {
		t.Fatalf("empty-final recovery should make one bounded recovery request, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	if !strings.Contains(provider.Body(1), "previous assistant response was empty") {
		t.Fatalf("recovery request should include empty-final recovery instruction:\n%s", provider.Body(1))
	}
}

func TestRuntimeServerEmptyFinalRecoveryExhaustionIsVisible(t *testing.T) {
	dataDir := t.TempDir()
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "openai-empty", "gpt-empty"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":     "Empty Final Exhausted",
		"workspace": dataDir,
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	failed := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "Return nothing twice",
		"providerId": "openai-empty",
		"model":      "gpt-empty",
	}), http.StatusInternalServerError)
	assertClosedTurnFailureResponse(t, failed, "provider_empty_final", "The provider returned no final response after the bounded recovery attempt.")

	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	events := parseRuntimeServerSSEEvents(t, replay)
	if !hasRuntimeServerEvent(events, "pipeline_stage") ||
		!strings.Contains(replay, `"stage":"empty_final_recovered"`) ||
		!strings.Contains(replay, `"stage":"provider_error"`) {
		t.Fatalf("empty-final exhaustion should preserve recovery and provider_error stages:\n%s", replay)
	}
	var providerError map[string]any
	for _, event := range events {
		if stringField(event, "kind") == "pipeline_stage" && stringField(event, "stage") == "provider_error" {
			providerError = event
			break
		}
	}
	if providerError == nil {
		t.Fatalf("missing empty-final provider_error stage:\n%s", replay)
	}
	details := mapField(t, providerError, "details")
	if stringField(details, "recoveryKind") != "empty_final" ||
		floatField(t, details, "recoveryAttempt") != 1 ||
		floatField(t, details, "maxRecoveryAttempts") != 1 ||
		!boolField(details, "recoveryExhausted") {
		t.Fatalf("empty-final exhaustion should carry recovery diagnostics: %#v", details)
	}
	if provider.RequestCount() != 2 {
		t.Fatalf("empty-final exhaustion should stop after one bounded recovery request, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
}

func TestRuntimeServerExecutesProviderToolCallAndContinues(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("note content"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"note.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":1,"total_tokens":11}}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"read complete"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":20,"completion_tokens":2,"total_tokens":22}}`,
			`data: [DONE]`,
		},
	})
	modelProvidersJSON, connectProviderRegistry := prepareRuntimeServerExplicitProviderRegistryFixture(
		t, dataDir, provider.URL(), "tool-provider", "tool-model",
	)
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: modelProvidersJSON,
	}))
	defer server.Close()
	connectProviderRegistry(server.URL)

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Tool loop",
		"workspace":  workspace,
		"providerId": "tool-provider",
		"model":      "tool-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Read the note.",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	if provider.RequestCount() != 2 {
		t.Fatalf("tool loop should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	if !strings.Contains(provider.Body(1), "note content") {
		t.Fatalf("second provider call must include read_file tool result history:\n%s", provider.Body(1))
	}
	hostCallID := providerHostToolCallIDForName(t, provider.Body(1), "read_file")
	if !domainsecurity.IsHostToolCallIDV1(hostCallID) || hostCallID == "call_read" || strings.Contains(provider.Body(1), `"id":"call_read"`) || strings.Contains(provider.Body(1), `"tool_call_id":"call_read"`) {
		t.Fatalf("read_file provider history did not replace the untrusted call id: host=%q body=%s", hostCallID, provider.Body(1))
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	events := runtimeServerEventsForTurn(t, replay, turnID)
	for _, kind := range []string{"tool_call_ready", "tool_call_started", "tool_call_finished", "item_completed", "usage", "turn_completed"} {
		if !hasRuntimeServerEvent(events, kind) {
			t.Fatalf("tool loop replay missing %s:\n%s", kind, replay)
		}
	}
	runningProgressIndex, runningProgress := runtimeServerEventIndexWithKindCallAndStatus(t, events, "tool_progress", hostCallID, "running")
	if stringField(runningProgress, "itemId") == "" || stringField(runningProgress, "toolName") != "read_file" {
		t.Fatalf("tool running progress must identify the running tool card: %#v", runningProgress)
	}
	successProgressIndex, successProgress := runtimeServerEventIndexWithKindCallAndStatus(t, events, "tool_progress", hostCallID, "success")
	if successProgress["message"] != nil || successProgress["summary"] != nil {
		t.Fatalf("tool progress display text must be renderer-derived: %#v", successProgress)
	}
	startedIndex, startedEvent := runtimeServerEventIndexWithKind(t, events, "tool_call_started")
	finishedIndex, finishedEvent := runtimeServerEventIndexWithKind(t, events, "tool_call_finished")
	if stringField(mapField(t, startedEvent, "item"), "callId") != hostCallID ||
		stringField(mapField(t, finishedEvent, "item"), "callId") != hostCallID || strings.Contains(replay, `"callId":"call_read"`) {
		t.Fatalf("tool lifecycle did not preserve only the host call identity: host=%q replay=%s", hostCallID, replay)
	}
	if startedIndex > runningProgressIndex || runningProgressIndex > finishedIndex || successProgressIndex < finishedIndex {
		t.Fatalf("tool progress order mismatch: started=%d running=%d finished=%d success=%d events=%#v", startedIndex, runningProgressIndex, finishedIndex, successProgressIndex, events)
	}
	assertRuntimeServerTypedOrdinaryTerminal(t, replay, turnID, "read complete", domainordinaryresult.ResultSlotOriginProviderOrdinaryOnlyV1)
	for _, forbidden := range []string{"Ship", "D0244", "mcp__analytix_local", "deepseek-live-local"} {
		if strings.Contains(replay, forbidden) {
			t.Fatalf("tool loop replay must not contain fixture/proof content %q:\n%s", forbidden, replay)
		}
	}
}

func TestRuntimeServerReadToolsAllowExternalPathsInFullAccess(t *testing.T) {
	dataDir := runtimeServerPrivateToolArgumentTempDir(t, "external-read")
	workspace := filepath.Join(dataDir, "workspace")
	external := filepath.Join(dataDir, "Desktop")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(external, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, "live.txt"), []byte("desktop"), 0o644); err != nil {
		t.Fatal(err)
	}
	listArgs := mustJSONString(t, map[string]string{"path": external})
	provider := newCompleteProviderServer(t, [][]string{
		{
			fmt.Sprintf(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_ls_external","type":"function","function":{"name":"ls","arguments":%q}}]},"finish_reason":"tool_calls"}]}`, listArgs),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"external list complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "external-read-provider", "external-read-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "External read",
		"workspace":  workspace,
		"providerId": "external-read-provider",
		"model":      "external-read-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "List the external directory.",
		"approvalPolicy": "auto",
		"sandboxMode":    "danger-full-access",
	}), http.StatusAccepted)
	if provider.RequestCount() != 2 {
		t.Fatalf("external ls should continue provider with tool result, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	hostCallID := providerHostToolCallIDForName(t, provider.Body(1), "ls")
	if !domainsecurity.IsHostToolCallIDV1(hostCallID) || hostCallID == "call_ls_external" ||
		strings.Contains(provider.Body(1), `"id":"call_ls_external"`) || strings.Contains(provider.Body(1), `"tool_call_id":"call_ls_external"`) {
		t.Fatalf("external ls provider history did not replace the untrusted call id: host=%q body=%s", hostCallID, provider.Body(1))
	}
	toolResult := providerToolResultForCall(t, provider.Body(1), hostCallID)
	toolResultJSON := string(mustJSON(t, toolResult))
	if !strings.Contains(toolResultJSON, "live.txt") || strings.Contains(toolResultJSON, "workspace_escape") {
		t.Fatalf("full access external ls should return listed entries, not workspace_escape: result=%s body=%s", toolResultJSON, provider.Body(1))
	}
}

func TestRuntimeServerGlobAllowsExternalPatternInFullAccess(t *testing.T) {
	dataDir := runtimeServerPrivateToolArgumentTempDir(t, "external-glob")
	workspace := filepath.Join(dataDir, "workspace")
	external := filepath.Join(dataDir, "Desktop")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(external, 0o755); err != nil {
		t.Fatal(err)
	}
	pdfPath := filepath.Join(external, "report.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, "notes.txt"), []byte("notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	globArgs := mustJSONString(t, map[string]string{"pattern": filepath.Join(external, "*.pdf")})
	provider := newCompleteProviderServer(t, [][]string{
		{
			fmt.Sprintf(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_glob_external","type":"function","function":{"name":"glob","arguments":%q}}]},"finish_reason":"tool_calls"}]}`, globArgs),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"external glob complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "external-glob-provider", "external-glob-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "External glob",
		"workspace":  workspace,
		"providerId": "external-glob-provider",
		"model":      "external-glob-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Glob the external directory.",
		"approvalPolicy": "auto",
		"sandboxMode":    "danger-full-access",
	}), http.StatusAccepted)
	if provider.RequestCount() != 2 {
		t.Fatalf("external glob should continue provider with tool result, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	body := provider.Body(1)
	hostCallID := providerHostToolCallIDForName(t, body, "glob")
	resultJSON := string(mustJSON(t, providerToolResultForCall(t, body, hostCallID)))
	if !domainsecurity.IsHostToolCallIDV1(hostCallID) || hostCallID == "call_glob_external" ||
		!strings.Contains(resultJSON, "report.pdf") || !strings.Contains(resultJSON, external) ||
		strings.Contains(resultJSON, "notes.txt") || strings.Contains(resultJSON, "workspace_escape") {
		t.Fatalf("full access external glob should return PDF match only: host=%q result=%s body=%s", hostCallID, resultJSON, body)
	}
}

func TestRuntimeServerReadToolsPreserveKunAndNumberedPaginationContracts(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "notes.txt"), []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read","type":"function","function":{"name":"read","arguments":"{\"path\":\"notes.txt\",\"offset\":2,\"limit\":1}"}},{"index":1,"id":"call_read_file","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"notes.txt\",\"offset\":1,\"limit\":1}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_edit","type":"function","function":{"name":"edit","arguments":"{\"path\":\"notes.txt\",\"oldText\":\"beta\",\"newText\":\"BETA\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"read contracts preserved"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "read-provider", "read-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Read contracts",
		"workspace":      workspace,
		"providerId":     "read-provider",
		"model":          "read-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Read then edit.",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusAccepted)
	if provider.RequestCount() != 3 {
		t.Fatalf("read/edit loop should call provider three times, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	afterRead := provider.Body(1)
	readCallID := providerHostToolCallIDForName(t, afterRead, "read")
	readResultJSON := string(mustJSON(t, providerToolResultForCall(t, afterRead, readCallID)))
	if !domainsecurity.IsHostToolCallIDV1(readCallID) || readCallID == "call_read" ||
		!strings.Contains(readResultJSON, `"start_line":2`) ||
		!strings.Contains(readResultJSON, `"content":"beta\n\n[2 more lines in file. Use offset=3 to continue.]`) {
		t.Fatalf("read tool must preserve Kun 1-based pagination contract: host=%q result=%s body=%s", readCallID, readResultJSON, afterRead)
	}
	readFileCallID := providerHostToolCallIDForName(t, afterRead, "read_file")
	readFileResultJSON := string(mustJSON(t, providerToolResultForCall(t, afterRead, readFileCallID)))
	if !domainsecurity.IsHostToolCallIDV1(readFileCallID) || readFileCallID == "call_read_file" ||
		!strings.Contains(readFileResultJSON, "2→beta") ||
		!strings.Contains(readFileResultJSON, "[more lines below; pass offset=2 to continue]") {
		t.Fatalf("read_file alias must expose numbered 0-based pagination output: host=%q result=%s body=%s", readFileCallID, readFileResultJSON, afterRead)
	}
	afterEdit, err := os.ReadFile(filepath.Join(workspace, "notes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(afterEdit) != "alpha\nBETA\ngamma\n" {
		t.Fatalf("edit guard should use raw read content, not numbered read_file output: %q", string(afterEdit))
	}
}

func TestRuntimeServerUTF16ReadGrepAndEditPreserveEncoding(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(workspace, "utf16.txt")
	if err := os.WriteFile(target, testUTF16LEWithBOM("alpha\nneedle line\nomega\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	readArgs := mustJSONString(t, map[string]any{"path": "utf16.txt"})
	grepArgs := mustJSONString(t, map[string]any{"path": ".", "pattern": "needle"})
	editArgs := mustJSONString(t, map[string]any{
		"path":       "utf16.txt",
		"old_string": "needle line",
		"new_string": "needle changed",
	})
	provider := newCompleteProviderServer(t, [][]string{
		{
			fmt.Sprintf(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read_utf16","type":"function","function":{"name":"read_file","arguments":%q}},{"index":1,"id":"call_grep_utf16","type":"function","function":{"name":"grep","arguments":%q}}]},"finish_reason":"tool_calls"}]}`, readArgs, grepArgs),
			`data: [DONE]`,
		},
		{
			fmt.Sprintf(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_edit_utf16","type":"function","function":{"name":"edit_file","arguments":%q}}]},"finish_reason":"tool_calls"}]}`, editArgs),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"utf16 edited"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "utf16-provider", "utf16-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "UTF16 tools",
		"workspace":      workspace,
		"providerId":     "utf16-provider",
		"model":          "utf16-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Read, search, then edit the UTF-16 file.",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusAccepted)

	if provider.RequestCount() != 3 {
		t.Fatalf("UTF-16 tool loop should call provider three times, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	afterRead := provider.Body(1)
	for _, toolName := range []string{"read_file", "grep"} {
		hostCallID := providerHostToolCallIDForName(t, afterRead, toolName)
		resultJSON := string(mustJSON(t, providerToolResultForCall(t, afterRead, hostCallID)))
		if !domainsecurity.IsHostToolCallIDV1(hostCallID) || !strings.Contains(resultJSON, "needle line") ||
			(toolName == "read_file" && !strings.Contains(resultJSON, "utf16le")) {
			t.Fatalf("UTF-16 %s result lost its host identity or content: host=%q result=%s body=%s", toolName, hostCallID, resultJSON, afterRead)
		}
	}
	afterEdit := provider.Body(2)
	editCallID := providerHostToolCallIDForName(t, afterEdit, "edit_file")
	editResult := providerToolResultForCall(t, afterEdit, editCallID)
	if !domainsecurity.IsHostToolCallIDV1(editCallID) || stringField(editResult, "encoding") != "utf16le" || editResult["replacements"] != float64(1) ||
		stringField(editResult, "code") != "" || stringField(editResult, "error") != "" {
		t.Fatalf("UTF-16 edit did not return a concrete successful tool result: %#v", editResult)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodeTestUTF16LEWithBOM(t, raw)
	if decoded != "alpha\nneedle changed\nomega\n" {
		t.Fatalf("edit_file should preserve UTF-16LE BOM encoding and decoded content, got %q", decoded)
	}
}

func TestRuntimeServerWritePreservesExistingUTF16Encoding(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(workspace, "utf16-write.txt")
	if err := os.WriteFile(target, testUTF16LEWithBOM("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeArgs := mustJSONString(t, map[string]any{
		"path":    "utf16-write.txt",
		"content": "new\n",
	})
	provider := newCompleteProviderServer(t, [][]string{
		{
			fmt.Sprintf(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_write_utf16","type":"function","function":{"name":"write_file","arguments":%q}}]},"finish_reason":"tool_calls"}]}`, writeArgs),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"utf16 written"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "utf16-write-provider", "utf16-write-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "UTF16 write",
		"workspace":      workspace,
		"providerId":     "utf16-write-provider",
		"model":          "utf16-write-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Overwrite the UTF-16 file.",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusAccepted)

	if provider.RequestCount() != 2 {
		t.Fatalf("UTF-16 write loop should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	afterWrite := provider.Body(1)
	writeCallID := providerHostToolCallIDForName(t, afterWrite, "write_file")
	writeResult := providerToolResultForCall(t, afterWrite, writeCallID)
	if !domainsecurity.IsHostToolCallIDV1(writeCallID) || stringField(writeResult, "encoding") != "utf16le" || stringField(writeResult, "code") != "" ||
		stringField(writeResult, "error") != "" || writeResult["bytes_written"] != float64(len(testUTF16LEWithBOM("new\n"))) {
		t.Fatalf("UTF-16 write did not return a concrete successful tool result: %#v", writeResult)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if decoded := decodeTestUTF16LEWithBOM(t, raw); decoded != "new\n" {
		t.Fatalf("write_file should preserve existing UTF-16LE BOM encoding, got %q", decoded)
	}
}

func TestRuntimeServerGlobToolIsAdvertisedAndExecutes(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	for _, dir := range []string{
		filepath.Join(workspace, "src"),
		filepath.Join(workspace, "pkg", "nested"),
		filepath.Join(workspace, "node_modules", "pkg"),
		filepath.Join(workspace, "secret"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		filepath.Join(workspace, "src", "app.go"):                          "package main\n",
		filepath.Join(workspace, "pkg", "nested", "widget.test.ts"):        "test('ok')\n",
		filepath.Join(workspace, "node_modules", "pkg", "ignored.test.ts"): "ignored\n",
		filepath.Join(workspace, "secret", "hidden.test.ts"):               "secret\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_glob_tests","type":"function","function":{"name":"glob","arguments":"{\"pattern\":\"**/*.test.ts\"}"}},{"index":1,"id":"call_glob_simple","type":"function","function":{"name":"glob","arguments":"{\"pattern\":\"app.go\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"glob complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "glob-provider", "glob-model"),
		ProtectedReadDirs:  []string{filepath.Join(workspace, "secret")},
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Glob tool",
		"workspace":  workspace,
		"providerId": "glob-provider",
		"model":      "glob-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Use glob.",
	}), http.StatusAccepted)
	if !containsString(providerRequestToolNames(t, provider.Body(0)), "glob") {
		t.Fatalf("provider tool catalog should advertise glob: %s", provider.Body(0))
	}
	if provider.RequestCount() != 2 {
		t.Fatalf("glob loop should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	secondBody := provider.Body(1)
	globCallIDs := providerHostToolCallIDsForName(t, secondBody, "glob")
	if len(globCallIDs) != 2 {
		t.Fatalf("glob provider history lost tool calls: ids=%#v body=%s", globCallIDs, secondBody)
	}
	globResults := ""
	for _, callID := range globCallIDs {
		if !domainsecurity.IsHostToolCallIDV1(callID) {
			t.Fatalf("glob provider history contains a non-host call id %q: %s", callID, secondBody)
		}
		globResults += string(mustJSON(t, providerToolResultForCall(t, secondBody, callID)))
	}
	for _, expected := range []string{"pkg/nested/widget.test.ts", "src/app.go", "skipped_directories", "node_modules", "skipped_protected", "secret"} {
		if !strings.Contains(globResults, expected) {
			t.Fatalf("glob result missing %q: results=%s body=%s", expected, globResults, secondBody)
		}
	}
	for _, forbidden := range []string{"ignored.test.ts", "hidden.test.ts"} {
		if strings.Contains(globResults, forbidden) {
			t.Fatalf("glob result must not include skipped/protected file %q: results=%s body=%s", forbidden, globResults, secondBody)
		}
	}
}

func TestRuntimeServerGrepSkipsNoiseHiddenAndProtectedDirs(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	for _, dir := range []string{
		filepath.Join(workspace, "public"),
		filepath.Join(workspace, "node_modules", "pkg"),
		filepath.Join(workspace, ".hidden"),
		filepath.Join(workspace, ".git"),
		filepath.Join(workspace, "secret"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		filepath.Join(workspace, "public", "visible.txt"):              "needle visible\n",
		filepath.Join(workspace, "public", ".env"):                     "needle hidden-file\n",
		filepath.Join(workspace, "node_modules", "pkg", "ignored.txt"): "needle dependency\n",
		filepath.Join(workspace, ".hidden", "ignored.txt"):             "needle hidden-dir\n",
		filepath.Join(workspace, ".git", "config"):                     "needle vcs\n",
		filepath.Join(workspace, "secret", "hidden.txt"):               "needle protected\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_grep_noise","type":"function","function":{"name":"grep","arguments":"{\"pattern\":\"needle\",\"path\":\".\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"grep complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "grep-provider", "grep-model"),
		ProtectedReadDirs:  []string{filepath.Join(workspace, "secret")},
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Grep tool",
		"workspace":  workspace,
		"providerId": "grep-provider",
		"model":      "grep-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Use grep.",
	}), http.StatusAccepted)
	if !containsString(providerRequestToolNames(t, provider.Body(0)), "grep") {
		t.Fatalf("provider tool catalog should advertise grep: %s", provider.Body(0))
	}
	if provider.RequestCount() != 2 {
		t.Fatalf("grep loop should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	secondBody := provider.Body(1)
	grepCallID := providerHostToolCallIDForName(t, secondBody, "grep")
	grepResult := string(mustJSON(t, providerToolResultForCall(t, secondBody, grepCallID)))
	if !domainsecurity.IsHostToolCallIDV1(grepCallID) {
		t.Fatalf("grep provider history contains a non-host call id %q: %s", grepCallID, secondBody)
	}
	for _, expected := range []string{
		"public/visible.txt",
		"needle visible",
		"skipped_directories",
		"node_modules",
		".hidden",
		".git",
		"skipped_protected",
		"secret",
	} {
		if !strings.Contains(grepResult, expected) {
			t.Fatalf("grep result missing %q: result=%s body=%s", expected, grepResult, secondBody)
		}
	}
	for _, forbidden := range []string{"needle dependency", "needle hidden-dir", "needle vcs", "needle hidden-file", "needle protected"} {
		if strings.Contains(grepResult, forbidden) {
			t.Fatalf("grep result must not include skipped/protected content %q: result=%s body=%s", forbidden, grepResult, secondBody)
		}
	}
}

func TestRuntimeServerGrepSupportsGlobContextAndColumn(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	for _, dir := range []string{
		filepath.Join(workspace, "src"),
		filepath.Join(workspace, "testdata"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "app.go"), []byte("before line\nneedle value\nafter line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "testdata", "ignored.go"), []byte("needle ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	grepArgs := mustJSONString(t, map[string]any{
		"pattern": "needle",
		"path":    ".",
		"glob":    "src/*.go",
		"context": 1,
	})
	provider := newCompleteProviderServer(t, [][]string{
		{
			fmt.Sprintf(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_grep_context","type":"function","function":{"name":"grep","arguments":%q}}]},"finish_reason":"tool_calls"}]}`, grepArgs),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"grep context complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "grep-context-provider", "grep-context-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Grep context",
		"workspace":  workspace,
		"providerId": "grep-context-provider",
		"model":      "grep-context-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Use grep with context.",
	}), http.StatusAccepted)

	if provider.RequestCount() != 2 {
		t.Fatalf("grep context loop should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	secondBody := provider.Body(1)
	grepCallID := providerHostToolCallIDForName(t, secondBody, "grep")
	grepResult := string(mustJSON(t, providerToolResultForCall(t, secondBody, grepCallID)))
	if !domainsecurity.IsHostToolCallIDV1(grepCallID) {
		t.Fatalf("grep context provider history contains a non-host call id %q: %s", grepCallID, secondBody)
	}
	for _, expected := range []string{
		"src/app.go",
		"needle value",
		`"glob":"src/*.go"`,
		`"context":1`,
		`"column":1`,
		"context_before",
		"before line",
		"context_after",
		"after line",
	} {
		if !strings.Contains(grepResult, expected) {
			t.Fatalf("grep context result missing %q: result=%s body=%s", expected, grepResult, secondBody)
		}
	}
	if strings.Contains(grepResult, "testdata/ignored.go") || strings.Contains(grepResult, "needle ignored") {
		t.Fatalf("grep glob filter must exclude non-matching files: result=%s body=%s", grepResult, secondBody)
	}
}

func TestRuntimeServerGrepHonorsRootGitignore(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	for _, dir := range []string{
		workspace,
		filepath.Join(workspace, ".git"),
		filepath.Join(workspace, "build"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		filepath.Join(workspace, ".gitignore"):   "*.log\nbuild/\n!keep.log\n",
		filepath.Join(workspace, "keep.txt"):     "NEEDLE kept text\n",
		filepath.Join(workspace, "keep.log"):     "NEEDLE re-included\n",
		filepath.Join(workspace, "drop.log"):     "NEEDLE ignored log\n",
		filepath.Join(workspace, "build", "out"): "NEEDLE ignored dir\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_grep_gitignore","type":"function","function":{"name":"grep","arguments":"{\"pattern\":\"NEEDLE\",\"path\":\".\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"grep gitignore complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "grep-gitignore-provider", "grep-gitignore-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Grep gitignore",
		"workspace":  workspace,
		"providerId": "grep-gitignore-provider",
		"model":      "grep-gitignore-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Use grep with gitignore.",
	}), http.StatusAccepted)

	if provider.RequestCount() != 2 {
		t.Fatalf("grep gitignore loop should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	secondBody := provider.Body(1)
	grepCallID := providerHostToolCallIDForName(t, secondBody, "grep")
	grepResult := string(mustJSON(t, providerToolResultForCall(t, secondBody, grepCallID)))
	if !domainsecurity.IsHostToolCallIDV1(grepCallID) {
		t.Fatalf("grep gitignore provider history contains a non-host call id %q: %s", grepCallID, secondBody)
	}
	for _, expected := range []string{
		"keep.txt",
		"NEEDLE kept text",
		"keep.log",
		"NEEDLE re-included",
		"skipped_directories",
		"build",
	} {
		if !strings.Contains(grepResult, expected) {
			t.Fatalf("grep gitignore result missing %q: result=%s body=%s", expected, grepResult, secondBody)
		}
	}
	for _, forbidden := range []string{"drop.log", "NEEDLE ignored log", "build/out", "NEEDLE ignored dir"} {
		if strings.Contains(grepResult, forbidden) {
			t.Fatalf("grep gitignore must exclude ignored content %q: result=%s body=%s", forbidden, grepResult, secondBody)
		}
	}
}

func TestRuntimeServerGrepHonorsGitignore(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	for _, dir := range []string{
		filepath.Join(workspace, ".git"),
		filepath.Join(workspace, "generated"),
		filepath.Join(workspace, "pkg"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(workspace, ".gitignore"), []byte("ignored.txt\ngenerated/\n*.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "pkg", ".gitignore"), []byte("secret.txt\n!keep.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		filepath.Join(workspace, "visible.txt"):           "needle visible\n",
		filepath.Join(workspace, "ignored.txt"):           "needle ignored-root\n",
		filepath.Join(workspace, "generated", "out.txt"):  "needle ignored-dir\n",
		filepath.Join(workspace, "pkg", "secret.txt"):     "needle ignored-nested\n",
		filepath.Join(workspace, "pkg", "drop.log"):       "needle ignored-log\n",
		filepath.Join(workspace, "pkg", "keep.log"):       "needle keep-log\n",
		filepath.Join(workspace, "pkg", "component.go"):   "needle component\n",
		filepath.Join(workspace, "pkg", "component.test"): "needle non-go\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_grep_gitignore","type":"function","function":{"name":"grep","arguments":"{\"pattern\":\"needle\",\"path\":\".\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"grep gitignore complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "grep-gitignore-provider", "grep-gitignore-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Grep gitignore",
		"workspace":  workspace,
		"providerId": "grep-gitignore-provider",
		"model":      "grep-gitignore-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Use grep with gitignore.",
	}), http.StatusAccepted)
	if provider.RequestCount() != 2 {
		t.Fatalf("grep gitignore loop should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	secondBody := provider.Body(1)
	grepCallID := providerHostToolCallIDForName(t, secondBody, "grep")
	grepResult := string(mustJSON(t, providerToolResultForCall(t, secondBody, grepCallID)))
	if !domainsecurity.IsHostToolCallIDV1(grepCallID) {
		t.Fatalf("nested gitignore provider history contains a non-host call id %q: %s", grepCallID, secondBody)
	}
	for _, expected := range []string{
		"visible.txt",
		"needle visible",
		"pkg/keep.log",
		"needle keep-log",
		"pkg/component.go",
		"needle component",
		"pkg/component.test",
		"needle non-go",
	} {
		if !strings.Contains(grepResult, expected) {
			t.Fatalf("grep gitignore result missing %q: result=%s body=%s", expected, grepResult, secondBody)
		}
	}
	for _, forbidden := range []string{
		"ignored.txt",
		"needle ignored-root",
		"generated/out.txt",
		"needle ignored-dir",
		"pkg/secret.txt",
		"needle ignored-nested",
		"pkg/drop.log",
		"needle ignored-log",
	} {
		if strings.Contains(grepResult, forbidden) {
			t.Fatalf("grep gitignore result must not include ignored content %q: result=%s body=%s", forbidden, grepResult, secondBody)
		}
	}
}

func TestRuntimeServerGlobRejectsWorkspaceEscape(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "outside.go"), []byte("package outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_glob_escape","type":"function","function":{"name":"glob","arguments":"{\"pattern\":\"../*.go\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"escape handled"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "glob-escape-provider", "glob-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Glob escape",
		"workspace":  workspace,
		"providerId": "glob-escape-provider",
		"model":      "glob-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Try escaping.",
	}), http.StatusAccepted)
	body := provider.Body(1)
	hostCallID := providerHostToolCallIDForName(t, body, "glob")
	resultJSON := string(mustJSON(t, providerToolResultForCall(t, body, hostCallID)))
	if !domainsecurity.IsHostToolCallIDV1(hostCallID) || !strings.Contains(resultJSON, "workspace_escape") {
		t.Fatalf("glob workspace escape should return a host-bound failed tool result and continue: host=%q result=%s body=%s", hostCallID, resultJSON, body)
	}
	if strings.Contains(resultJSON, "outside.go") {
		t.Fatalf("glob workspace escape must not expose outside file path: result=%s body=%s", resultJSON, body)
	}
}

func TestRuntimeServerCodeIndexToolIsAdvertisedAndSearchesGoSymbols(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	source := strings.Join([]string{
		"package svc",
		"",
		"type Service struct{}",
		"",
		"func NewService() *Service { return &Service{} }",
		"",
		"func (s *Service) Handle() {}",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(workspace, "service.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_code_index","type":"function","function":{"name":"code_index","arguments":"{\"action\":\"search\",\"query\":\"Service\",\"path\":\".\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"code_index complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "code-index-provider", "code-index-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Code index",
		"workspace":  workspace,
		"providerId": "code-index-provider",
		"model":      "code-index-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Index symbols.",
	}), http.StatusAccepted)
	if !containsString(providerRequestToolNames(t, provider.Body(0)), "code_index") {
		t.Fatalf("provider tool catalog should advertise code_index: %s", provider.Body(0))
	}
	secondBody := provider.Body(1)
	hostCallID := providerHostToolCallIDForName(t, secondBody, "code_index")
	resultJSON := string(mustJSON(t, providerToolResultForCall(t, secondBody, hostCallID)))
	if !domainsecurity.IsHostToolCallIDV1(hostCallID) {
		t.Fatalf("code_index provider history contains a non-host call id %q: %s", hostCallID, secondBody)
	}
	for _, expected := range []string{"service.go", "struct Service", "func NewService", "method Service.Handle"} {
		if !strings.Contains(resultJSON, expected) {
			t.Fatalf("code_index result missing %q: result=%s body=%s", expected, resultJSON, secondBody)
		}
	}
}

func TestRuntimeServerCodeIndexOutlineSkipsNoiseAndFiltersBeforeLimit(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	for _, dir := range []string{
		workspace,
		filepath.Join(workspace, "node_modules", "pkg"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(workspace, "a.ts"):                          "export class Alpha {}\n",
		filepath.Join(workspace, "z.ts"):                          "export interface Later {}\nexport type Alias = string\n",
		filepath.Join(workspace, "node_modules", "pkg", "dep.ts"): "export interface Hidden {}\n",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_code_index_outline","type":"function","function":{"name":"code_index","arguments":"{\"action\":\"outline\",\"path\":\".\",\"kind\":\"interface\",\"limit\":1}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"outline complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "code-index-outline-provider", "code-index-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Code index outline",
		"workspace":  workspace,
		"providerId": "code-index-outline-provider",
		"model":      "code-index-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Outline symbols.",
	}), http.StatusAccepted)
	secondBody := provider.Body(1)
	hostCallID := providerHostToolCallIDForName(t, secondBody, "code_index")
	resultJSON := string(mustJSON(t, providerToolResultForCall(t, secondBody, hostCallID)))
	if !domainsecurity.IsHostToolCallIDV1(hostCallID) {
		t.Fatalf("code_index outline provider history contains a non-host call id %q: %s", hostCallID, secondBody)
	}
	for _, expected := range []string{"z.ts", "interface Later", "skipped_directories", "node_modules"} {
		if !strings.Contains(resultJSON, expected) {
			t.Fatalf("code_index outline result missing %q: result=%s body=%s", expected, resultJSON, secondBody)
		}
	}
	for _, forbidden := range []string{"class Alpha", "interface Hidden"} {
		if strings.Contains(resultJSON, forbidden) {
			t.Fatalf("code_index outline should filter/skip %q: result=%s body=%s", forbidden, resultJSON, secondBody)
		}
	}
}

func TestRuntimeServerCodeIndexRequiresQueryForSearch(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "service.go"), []byte("package svc\ntype Service struct{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_code_index_without_query","type":"function","function":{"name":"code_index","arguments":"{\"action\":\"search\",\"path\":\".\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"query validation complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "code-index-validation-provider", "code-index-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Code index validation",
		"workspace":  workspace,
		"providerId": "code-index-validation-provider",
		"model":      "code-index-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Search without query.",
	}), http.StatusAccepted)
	body := provider.Body(1)
	hostCallID := providerHostToolCallIDForName(t, body, "code_index")
	resultJSON := string(mustJSON(t, providerToolResultForCall(t, body, hostCallID)))
	if !domainsecurity.IsHostToolCallIDV1(hostCallID) || !strings.Contains(resultJSON, "query_required") ||
		!strings.Contains(resultJSON, "query is required") {
		t.Fatalf("code_index search without query should return a host-bound failed tool result and continue: host=%q result=%s body=%s", hostCallID, resultJSON, body)
	}
}

func TestRuntimeServerWebFetchDisabledByDefault(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{{
		`data: {"choices":[{"delta":{"content":"plain turn"}}]}`,
		`data: [DONE]`,
	}})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "web-disabled-provider", "web-model"),
	}))
	defer server.Close()

	info := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/info", DefaultRuntimeToken, nil, http.StatusOK)
	web := mapField(t, mapField(t, info, "capabilities"), "web")
	if boolField(web, "enabled") || boolField(web, "available") || boolField(mapField(t, web, "fetch"), "available") {
		t.Fatalf("web_fetch must be disabled by default: %#v", web)
	}
	tools := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/tools", DefaultRuntimeToken, nil, http.StatusOK)
	if tools["webProviderCount"] != float64(0) {
		t.Fatalf("disabled web config must report zero web providers: %#v", tools)
	}
	if _, exists := tools["webProviders"]; exists {
		t.Fatalf("runtime tools must not expose raw web provider diagnostics: %#v", tools)
	}
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Web disabled",
		"workspace":  workspace,
		"providerId": "web-disabled-provider",
		"model":      "web-model",
	}), http.StatusCreated)
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+stringField(thread, "id")+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "No web tools.",
	}), http.StatusAccepted)
	if containsString(providerRequestToolNames(t, provider.Body(0)), "web_fetch") {
		t.Fatalf("provider catalog must not advertise web_fetch without config: %s", provider.Body(0))
	}
}

func TestRuntimeServerWebFetchToolIsAdvertisedAndExecutes(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>Docs &amp; Guide</title><style>.x{}</style><script>secret()</script></head><body><h1>Hello docs</h1><p>Fetched through Analytix.</p></body></html>`))
	}))
	defer page.Close()
	pageURL, err := url.Parse(page.URL)
	if err != nil {
		t.Fatal(err)
	}
	fetchArgs := mustJSONString(t, map[string]any{
		"url":       page.URL,
		"max_bytes": 2048,
	})
	provider := newCompleteProviderServer(t, [][]string{
		{
			"data: " + mustJSONString(t, map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{{
							"index": 0,
							"id":    "call_web_fetch",
							"type":  "function",
							"function": map[string]any{
								"name":      "web_fetch",
								"arguments": fetchArgs,
							},
						}},
					},
					"finish_reason": "tool_calls",
				}},
			}),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"web fetch complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		MCPConfigJSON:      testWebFetchConfigJSON(t, map[string]any{"maxFetchBytes": 4096}),
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "web-provider", "web-model"),
	}))
	defer server.Close()

	info := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/info", DefaultRuntimeToken, nil, http.StatusOK)
	web := mapField(t, mapField(t, info, "capabilities"), "web")
	if !boolField(web, "enabled") || !boolField(web, "available") || !boolField(mapField(t, web, "fetch"), "available") {
		t.Fatalf("enabled web_fetch must be visible in runtime capabilities: %#v", web)
	}
	tools := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/tools", DefaultRuntimeToken, nil, http.StatusOK)
	if tools["webProviderCount"] != float64(1) {
		t.Fatalf("runtime tools must expose the configured web provider count: %#v", tools)
	}
	if _, exists := tools["webProviders"]; exists {
		t.Fatalf("runtime tools must not expose raw web provider diagnostics: %#v", tools)
	}
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Web fetch",
		"workspace":  workspace,
		"providerId": "web-provider",
		"model":      "web-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Fetch docs.",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	if provider.RequestCount() != 2 {
		t.Fatalf("web_fetch loop should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	if !containsString(providerRequestToolNames(t, provider.Body(0)), "web_fetch") {
		t.Fatalf("provider catalog should advertise web_fetch when configured: %s", provider.Body(0))
	}
	secondBody := provider.Body(1)
	hostCallID, fetchResult := providerHostToolResultForName(t, secondBody, "web_fetch")
	resultJSON := string(mustJSON(t, fetchResult))
	for _, expected := range []string{"Hello docs", "Fetched through Analytix", "sourceId", "sources", "citations"} {
		if !strings.Contains(resultJSON, expected) {
			t.Fatalf("host-bound web_fetch result missing %q: result=%s body=%s", expected, resultJSON, secondBody)
		}
	}
	for _, forbidden := range []string{"secret()", "<script", "<style"} {
		if strings.Contains(resultJSON, forbidden) {
			t.Fatalf("web_fetch result should strip script/style/tag content %q: result=%s", forbidden, resultJSON)
		}
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	events := runtimeServerEventsForTurn(t, replay, turnID)
	for _, kind := range []string{"tool_call_ready", "tool_call_started", "tool_progress", "tool_call_finished", "item_completed", "turn_completed"} {
		if !hasRuntimeServerEvent(events, kind) {
			t.Fatalf("web_fetch replay missing %s:\n%s", kind, replay)
		}
	}
	_, runningProgress := runtimeServerEventIndexWithKindCallAndStatus(t, events, "tool_progress", hostCallID, "running")
	if _, present := runningProgress["message"]; present || strings.Contains(replay, "fetching "+pageURL.Hostname()) {
		t.Fatalf("web_fetch public progress must retain status without private host detail: progress=%#v\n%s", runningProgress, replay)
	}
}

func TestRuntimeServerWebFetchRejectsLinkLocal(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	fetchArgs := mustJSONString(t, map[string]any{"url": "http://169.254.169.254/latest/meta-data/"})
	provider := newCompleteProviderServer(t, [][]string{
		{
			"data: " + mustJSONString(t, map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{{
							"index": 0,
							"id":    "call_web_fetch_blocked",
							"type":  "function",
							"function": map[string]any{
								"name":      "web_fetch",
								"arguments": fetchArgs,
							},
						}},
					},
					"finish_reason": "tool_calls",
				}},
			}),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"blocked fetch handled"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		MCPConfigJSON:      testWebFetchConfigJSON(t, nil),
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "web-ssrf-provider", "web-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Web SSRF",
		"workspace":  workspace,
		"providerId": "web-ssrf-provider",
		"model":      "web-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Fetch metadata.",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	if provider.RequestCount() != 2 {
		t.Fatalf("blocked web_fetch should still continue with a second provider call, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	secondBody := provider.Body(1)
	_, fetchResult := providerHostToolResultForName(t, secondBody, "web_fetch")
	resultJSON := string(mustJSON(t, fetchResult))
	if !strings.Contains(resultJSON, "ssrf_blocked") || !strings.Contains(resultJSON, "[ACCOUNT]") ||
		strings.Contains(resultJSON, "169.254.169.254") {
		t.Fatalf("web_fetch must return a host-bound link-local rejection and continue: result=%s body=%s", resultJSON, secondBody)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	events := runtimeServerEventsForTurn(t, replay, turnID)
	if !hasRuntimeServerEvent(events, "tool_call_finished") || !strings.Contains(replay, `"isError":true`) {
		t.Fatalf("blocked web_fetch should be visible and still complete the turn:\n%s", replay)
	}
	assertRuntimeServerTypedOrdinaryTerminal(t, replay, turnID, "blocked fetch handled", domainordinaryresult.ResultSlotOriginProviderOrdinaryOnlyV1)
}

func TestRuntimeServerWebFetchUsesConfiguredProxy(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	fetchArgs := mustJSONString(t, map[string]any{"url": "http://service.test/resource"})
	provider := newCompleteProviderServer(t, [][]string{
		{
			"data: " + mustJSONString(t, map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{{
							"index": 0,
							"id":    "call_web_fetch_proxy",
							"type":  "function",
							"function": map[string]any{
								"name":      "web_fetch",
								"arguments": fetchArgs,
							},
						}},
					},
					"finish_reason": "tool_calls",
				}},
			}),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"proxy fetch complete"}}]}`,
			`data: [DONE]`,
		},
	})
	providerURL, err := url.Parse(provider.URL())
	if err != nil {
		t.Fatal(err)
	}
	var proxyHits int32
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Host {
		case "service.test":
			atomic.AddInt32(&proxyHits, 1)
			if r.URL.String() != "http://service.test/resource" {
				t.Fatalf("proxy should receive absolute target URL, got %s", r.URL.String())
			}
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("from configured proxy"))
		case providerURL.Host:
			forward := r.Clone(r.Context())
			forward.RequestURI = ""
			forward.URL.Scheme = providerURL.Scheme
			forward.URL.Host = providerURL.Host
			response, err := http.DefaultTransport.RoundTrip(forward)
			if err != nil {
				t.Fatalf("proxy should forward provider request: %v", err)
			}
			defer response.Body.Close()
			for key, values := range response.Header {
				for _, value := range values {
					w.Header().Add(key, value)
				}
			}
			w.WriteHeader(response.StatusCode)
			_, _ = io.Copy(w, response.Body)
		default:
			t.Fatalf("unexpected proxy target URL: %s", r.URL.String())
		}
	}))
	defer proxyServer.Close()
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		MCPConfigJSON:      testWebFetchConfigJSON(t, nil),
		ModelProxyURL:      proxyServer.URL,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "web-proxy-provider", "web-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Web proxy",
		"workspace":  workspace,
		"providerId": "web-proxy-provider",
		"model":      "web-model",
	}), http.StatusCreated)
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+stringField(thread, "id")+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Fetch via proxy.",
	}), http.StatusAccepted)
	if atomic.LoadInt32(&proxyHits) != 1 {
		t.Fatalf("web_fetch should use configured proxy, hits=%d", proxyHits)
	}
	if !strings.Contains(provider.Body(1), "from configured proxy") {
		t.Fatalf("web_fetch proxy result missing body:\n%s", provider.Body(1))
	}
}

func TestRuntimeServerRunsContiguousReadOnlyToolsInParallel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FIFO timing contract is POSIX-only")
	}
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	pipeA := filepath.Join(workspace, "pipe-a")
	pipeB := filepath.Join(workspace, "pipe-b")
	for _, path := range []string{pipeA, pipeB} {
		if err := exec.Command("mkfifo", path).Run(); err != nil {
			t.Fatalf("mkfifo %s: %v", path, err)
		}
	}
	writerErrs := make(chan error, 2)
	readerOpened := make(chan string, 2)
	releaseWrites := make(chan struct{})
	startWriter := func(path string, text string) {
		go func() {
			file, err := os.OpenFile(path, os.O_WRONLY, 0)
			if err != nil {
				writerErrs <- err
				return
			}
			defer file.Close()
			readerOpened <- path
			<-releaseWrites
			_, err = file.WriteString(text)
			writerErrs <- err
		}()
	}
	startWriter(pipeA, "alpha")
	startWriter(pipeB, "beta")
	parallelBarrierErr := make(chan error, 1)
	startParallelBarrier := func() {
		go func() {
			opened := make([]string, 0, 2)
			for len(opened) < 2 {
				select {
				case path := <-readerOpened:
					opened = append(opened, path)
				case <-time.After(runtimeServerPositiveTestTimeout):
					close(releaseWrites)
					parallelBarrierErr <- fmt.Errorf("only %d FIFO reader(s) opened before release: %v", len(opened), opened)
					return
				}
			}
			close(releaseWrites)
			parallelBarrierErr <- nil
		}()
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read_a","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"pipe-a\"}"}},{"index":1,"id":"call_read_b","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"pipe-b\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"read both pipes"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "parallel-read-provider", "parallel-read-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Parallel read-only tools",
		"workspace":  workspace,
		"providerId": "parallel-read-provider",
		"model":      "parallel-read-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	startParallelBarrier()
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Read both pipes.",
	}), http.StatusAccepted)
	if err := <-parallelBarrierErr; err != nil {
		t.Fatalf("contiguous read-only tools did not open both FIFOs concurrently: %v", err)
	}
	for i := 0; i < 2; i++ {
		select {
		case err := <-writerErrs:
			if err != nil {
				t.Fatalf("fifo writer failed: %v", err)
			}
		case <-time.After(runtimeServerPositiveTestTimeout):
			t.Fatalf("fifo writer did not finish")
		}
	}
	if provider.RequestCount() != 2 {
		t.Fatalf("parallel read-only turn should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	continuation := provider.Body(1)
	alphaIndex := strings.Index(continuation, "alpha")
	betaIndex := strings.Index(continuation, "beta")
	if alphaIndex < 0 || betaIndex < 0 || alphaIndex > betaIndex {
		t.Fatalf("provider continuation should preserve provider-order tool results with both pipe outputs:\n%s", continuation)
	}
}

func TestRuntimeServerExecutesDelegateTaskWithDurableChildRun(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_delegate","type":"function","function":{"name":"delegate_task","arguments":"{\"label\":\"inspect\",\"prompt\":\"Inspect the child path\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"child answer"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9,"prompt_tokens_details":{"cached_tokens":3}}}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"parent saw child"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":20,"completion_tokens":3,"total_tokens":23}}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "subagent-provider", "subagent-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Delegate task",
		"workspace":  workspace,
		"providerId": "subagent-provider",
		"model":      "subagent-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Delegate safely.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	if provider.RequestCount() != 3 {
		t.Fatalf("delegate_task should call provider for parent, child, then parent continuation, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	parentTools := providerRequestToolNames(t, provider.Body(0))
	for _, expected := range []string{"delegate_task", "task", "parallel_tasks", "wait", "bash_output", "kill_shell"} {
		if !containsString(parentTools, expected) {
			t.Fatalf("parent tool scope missing %s: %#v body=%s", expected, parentTools, provider.Body(0))
		}
	}
	for _, expected := range []string{"delegate_task", "task"} {
		assertProviderToolEffortEnum(t, provider.Body(0), expected)
	}
	assertProviderParallelTasksEffortEnum(t, provider.Body(0))
	childTools := providerRequestToolNames(t, provider.Body(1))
	for _, expected := range []string{"read", "read_file", "ls", "find", "glob", "code_index", "grep"} {
		if !containsString(childTools, expected) {
			t.Fatalf("child tool scope missing %s: %#v body=%s", expected, childTools, provider.Body(1))
		}
	}
	for _, hidden := range []string{"delegate_task", "task", "parallel_tasks", "wait", "bash_output", "kill_shell", "user_input", "request_user_input"} {
		if containsString(childTools, hidden) {
			t.Fatalf("child tool scope must hide %s: %#v", hidden, childTools)
		}
	}
	hostCallID, projected := providerHostToolResultForName(t, provider.Body(2), "delegate_task")
	assertSecurityBoundChildMetadata(t, projected, "", "completed", false)
	if strings.Contains(provider.Body(2), "child answer") {
		t.Fatalf("delegate child output leaked into parent continuation:\n%s", provider.Body(2))
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	events := runtimeServerEventsForTurn(t, replay, turnID)
	if !hasRuntimeServerEvent(events, "tool_call_ready") || !hasRuntimeServerEvent(events, "tool_call_finished") {
		t.Fatalf("delegate_task must use standard tool call events:\n%s", replay)
	}
	progress := runtimeServerEventWithKindAndChildStatus(t, events, "tool_progress", "running")
	if stringField(progress, "callId") != hostCallID ||
		stringField(progress, "toolName") != "delegate_task" ||
		stringField(progress, "status") != "running" {
		t.Fatalf("delegate_task running tool_progress mismatch: %#v", progress)
	}
	if itemID := stringField(progress, "itemId"); itemID == "" || !strings.HasPrefix(itemID, "item_tool_host_v1_") {
		t.Fatalf("security-bound child progress must expose only the host-issued item identity: %#v", progress)
	}
	assertSecurityBoundChildMetadata(t, mapField(t, progress, "child"), "", "running", false)
	finishedProgress := runtimeServerEventWithKindAndChildStatus(t, events, "tool_progress", "completed")
	if stringField(finishedProgress, "status") != "success" || stringField(finishedProgress, "callId") != hostCallID || strings.Contains(replay, "call_delegate") {
		t.Fatalf("delegate_task completed tool_progress must update the running card: %#v", finishedProgress)
	}
	assertSecurityBoundChildMetadata(t, mapField(t, finishedProgress, "child"), "", "completed", false)
	stage := runtimeServerEventWithChildStatus(t, events, "completed")
	child := mapField(t, stage, "child")
	assertSecurityBoundChildMetadata(t, child, "", "completed", false)
	childRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, threadID)
	if len(childRuns) != 1 {
		t.Fatalf("task job list should expose one durable child run, got %#v", childRuns)
	}
	assertSecurityBoundChildMetadata(t, childRuns[0], "", "completed", false)
}

func TestRuntimeServerMaxModelStepsZeroFallsBackToConfiguredBound(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "one.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read_one","type":"function","function":{"name":"read","arguments":"{\"path\":\"one.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"bounded final answer"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "step-provider", "step-model"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"runtime": map[string]any{
				"stepLimits": map[string]any{
					"defaultMaxModelSteps": 1,
				},
			},
		})),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Bounded steps",
		"workspace":  workspace,
		"providerId": "step-provider",
		"model":      "step-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Read one file before answering.",
		"approvalPolicy": "auto",
		"sandboxMode":    "read-only",
		"maxModelSteps":  0,
	}), http.StatusAccepted)
	if stringField(start, "turnId") == "" || provider.RequestCount() != 2 {
		t.Fatalf("maxModelSteps=0 must fall back to the configured bound, start=%#v requests=%d bodies=%#v", start, provider.RequestCount(), provider.Bodies())
	}
	if !strings.Contains(provider.Body(1), "configured budget of 1 model steps") || !strings.Contains(provider.Body(1), "one") {
		t.Fatalf("bounded final provider request should include the limit instruction and tool result:\n%s", provider.Body(1))
	}
}

func TestRuntimeServerStepLimitConfigFinalAnswerNudge(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "one.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read_one","type":"function","function":{"name":"read","arguments":"{\"path\":\"one.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"budget final answer"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "step-config-provider", "step-config-model"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"runtime": map[string]any{
				"stepLimits": map[string]any{
					"defaultMaxModelSteps": 1,
				},
			},
		})),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Configured steps",
		"workspace":  workspace,
		"providerId": "step-config-provider",
		"model":      "step-config-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Read one file before answering.",
		"approvalPolicy": "auto",
		"sandboxMode":    "read-only",
	}), http.StatusAccepted)
	if stringField(start, "turnId") == "" || provider.RequestCount() != 2 {
		t.Fatalf("configured default step limit should allow one tool round plus final-answer nudge, start=%#v requests=%d bodies=%#v", start, provider.RequestCount(), provider.Bodies())
	}
	if !strings.Contains(provider.Body(1), "configured budget of 1 model steps") {
		t.Fatalf("final provider request should include step-limit final-answer instruction:\n%s", provider.Body(1))
	}
	if strings.Contains(provider.Body(0), "configured budget") {
		t.Fatalf("initial provider request must not leak step-limit wording:\n%s", provider.Body(0))
	}
}

func TestRuntimeServerStepLimitFailurePersistsErrorItem(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "one.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read_one","type":"function","function":{"name":"read","arguments":"{\"path\":\"one.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read_again","type":"function","function":{"name":"read","arguments":"{\"path\":\"one.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "step-fail-provider", "step-fail-model"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"runtime": map[string]any{
				"stepLimits": map[string]any{
					"defaultMaxModelSteps": 1,
				},
			},
		})),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Configured step failure",
		"workspace":  workspace,
		"providerId": "step-fail-provider",
		"model":      "step-fail-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	failed := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Keep using tools past the configured budget.",
		"approvalPolicy": "auto",
		"sandboxMode":    "read-only",
	}), http.StatusInternalServerError)
	assertClosedTurnFailureResponse(t, failed, "turn_step_limit_exceeded", "The turn reached its configured model-step limit.")
	thread = assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	turns, _ := thread["turns"].([]any)
	if len(turns) != 1 {
		t.Fatalf("expected one turn: %#v", thread)
	}
	turn, _ := turns[0].(map[string]any)
	if stringField(turn, "status") != "failed" {
		t.Fatalf("step-limit turn should persist failed status: %#v", turn)
	}
	items, _ := turn["items"].([]any)
	var errorItem map[string]any
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if stringField(item, "kind") == "error" {
			errorItem = item
			break
		}
	}
	if errorItem == nil ||
		stringField(errorItem, "code") != "turn_step_limit_exceeded" ||
		stringField(errorItem, "message") != "The turn reached its configured model-step limit." {
		t.Fatalf("failed turn should replay a structured error item: %#v", turn)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if !strings.Contains(replay, `"kind":"item_completed"`) ||
		!strings.Contains(replay, `"kind":"turn_failed"`) ||
		!strings.Contains(replay, `"code":"turn_step_limit_exceeded"`) {
		t.Fatalf("step-limit replay should include visible error item and terminal failure:\n%s", replay)
	}
}

func TestRuntimeServerInterruptCancelsActiveAsyncTurnAndPreservesAbortedStatus(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	cancelled := make(chan struct{})
	var startedOnce sync.Once
	var cancelledOnce sync.Once
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		startedOnce.Do(func() { close(started) })
		<-r.Context().Done()
		cancelledOnce.Do(func() { close(cancelled) })
	}))
	defer provider.Close()
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL, "interrupt-provider", "interrupt-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Interrupt active turn",
		"workspace":  workspace,
		"providerId": "interrupt-provider",
		"model":      "interrupt-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Block until interrupted.",
		"async":  true,
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	if turnID == "" {
		t.Fatalf("async turn start missing turn id: %#v", start)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatalf("provider request did not start")
	}
	interrupt := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns/"+turnID+"/interrupt", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"discard": true,
	}), http.StatusOK)
	if interrupt["status"] != "aborted" || interrupt["cancelled"] != true {
		t.Fatalf("interrupt should abort and cancel active provider request: %#v", interrupt)
	}
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatalf("interrupt did not cancel provider request")
	}
	time.Sleep(50 * time.Millisecond)
	thread = assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	turn := findRuntimeServerTurn(t, thread, turnID)
	if stringField(turn, "status") != "aborted" {
		t.Fatalf("aborted turn status must not be overwritten after provider cancellation: %#v", turn)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	turnEvents := runtimeServerEventsForTurn(t, replay, turnID)
	if !hasRuntimeServerEvent(turnEvents, "turn_aborted") ||
		hasRuntimeServerEvent(turnEvents, "turn_completed") ||
		hasRuntimeServerEvent(turnEvents, "turn_failed") {
		t.Fatalf("interrupt replay should contain only aborted terminal lifecycle:\n%s", replay)
	}
}

func TestRuntimeServerInterruptDiscardExcludesAbortedTurnFromNextProviderHistory(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	cancelled := make(chan struct{})
	var startedOnce sync.Once
	var cancelledOnce sync.Once
	var mu sync.Mutex
	bodies := []string{}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		requestIndex := len(bodies)
		bodies = append(bodies, string(body))
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		if requestIndex == 0 {
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"discarded partial answer"}}]}` + "\n\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			startedOnce.Do(func() { close(started) })
			<-r.Context().Done()
			cancelledOnce.Do(func() { close(cancelled) })
			return
		}
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"fresh answer"},"finish_reason":"stop"}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: [DONE]` + "\n\n"))
	}))
	defer provider.Close()
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL, "discard-provider", "discard-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Discard interrupted history",
		"workspace":  workspace,
		"providerId": "discard-provider",
		"model":      "discard-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Discarded prompt should not survive.",
		"async":  true,
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	if turnID == "" {
		t.Fatalf("async turn start missing turn id: %#v", start)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatalf("provider request did not start")
	}
	interrupt := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns/"+turnID+"/interrupt", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"discard": true,
	}), http.StatusOK)
	if interrupt["status"] != "aborted" || interrupt["discard"] != true {
		t.Fatalf("interrupt should abort with discard=true: %#v", interrupt)
	}
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatalf("interrupt did not cancel provider request")
	}
	thread = assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	aborted := findRuntimeServerTurn(t, thread, turnID)
	if stringField(aborted, "status") != "aborted" {
		t.Fatalf("discarded aborted turn should preserve the closed public status: %#v", aborted)
	}
	for _, field := range []string{"discard", "cancelled", "cancelledPendingGates"} {
		if _, present := aborted[field]; present {
			t.Fatalf("interrupt settlement field %q must remain outside the public turn snapshot: %#v", field, aborted)
		}
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	turnEvents := runtimeServerEventsForTurn(t, replay, turnID)
	_, abortedEvent := runtimeServerEventIndexWithKind(t, turnEvents, "turn_aborted")
	if abortedEvent["discard"] != true || abortedEvent["cancelled"] != true || abortedEvent["cancelledPendingGates"] != float64(0) {
		t.Fatalf("public interrupt terminal lost atomic settlement metadata: %#v", abortedEvent)
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Fresh prompt after discard.",
	}), http.StatusAccepted)

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("expected two provider requests, got %d bodies=%#v", len(bodies), bodies)
	}
	secondBody := bodies[1]
	if !strings.Contains(secondBody, "Fresh prompt after discard.") {
		t.Fatalf("follow-up provider request missing fresh prompt:\n%s", secondBody)
	}
	for _, forbidden := range []string{"Discarded prompt should not survive.", "discarded partial answer"} {
		if strings.Contains(secondBody, forbidden) {
			t.Fatalf("follow-up provider request included discarded turn content %q:\n%s", forbidden, secondBody)
		}
	}
}

func TestRuntimeServerInterruptStopsLaterToolsInSameProviderStep(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_bash_cancel","type":"function","function":{"name":"bash","arguments":"{\"command\":\"printf started > started.txt; sleep 30\",\"timeout\":60}"}},{"index":1,"id":"call_write_after_cancel","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"out.txt\",\"content\":\"must not write\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"should not continue after tool cancel"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "tool-cancel-provider", "tool-cancel-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Interrupt tool batch",
		"workspace":      workspace,
		"providerId":     "tool-cancel-provider",
		"model":          "tool-cancel-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "danger-full-access",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Run a long bash then write.",
		"async":  true,
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	if turnID == "" {
		t.Fatalf("async turn start missing turn id: %#v", start)
	}
	startedPath := filepath.Join(workspace, "started.txt")
	startDeadline := time.Now().Add(runtimeServerPositiveTestTimeout)
	for {
		if _, err := os.Stat(startedPath); err == nil {
			break
		}
		if time.Now().After(startDeadline) {
			t.Fatalf("bash tool did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	interrupt := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns/"+turnID+"/interrupt", DefaultRuntimeToken, mustJSON(t, map[string]any{}), http.StatusOK)
	if interrupt["status"] != "aborted" || interrupt["cancelled"] != true {
		t.Fatalf("interrupt should abort active tool turn: %#v", interrupt)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if _, err := os.Stat(filepath.Join(workspace, "out.txt")); !os.IsNotExist(err) {
		t.Fatalf("write_file after tool cancel must not execute, stat err=%v replay=%s", err, replay)
	}
	if !hasRuntimeServerEvent(runtimeServerEventsForTurn(t, replay, turnID), "turn_aborted") ||
		strings.Contains(replay, "call_write_after_cancel") || strings.Contains(replay, "tool_cancelled") {
		t.Fatalf("terminal turn must reject all late tool and cancellation results:\n%s", replay)
	}
}

type runtimeServerSubagentDefaultMaxStepsCase struct {
	name         string
	parentSteps  *int
	expectedStep int
}

func TestRuntimeServerSubagentDefaultMaxStepsOrdinaryBudgets(t *testing.T) {
	parentSteps := 16
	assertRuntimeServerSubagentDefaultMaxSteps(t, []runtimeServerSubagentDefaultMaxStepsCase{
		{name: "default parent inherits effective child budget", expectedStep: 32},
		{name: "finite parent respects ordinary child baseline", parentSteps: &parentSteps, expectedStep: 12},
	})
}

func TestRuntimeServerSubagentDefaultMaxStepsBoundedBudgets(t *testing.T) {
	unboundedParentSteps := 0
	smallParentSteps := 6
	assertRuntimeServerSubagentDefaultMaxSteps(t, []runtimeServerSubagentDefaultMaxStepsCase{
		{name: "unbounded parent uses bounded child default", parentSteps: &unboundedParentSteps, expectedStep: 12},
		{name: "small finite parent caps child budget", parentSteps: &smallParentSteps, expectedStep: 6},
	})
}

func assertRuntimeServerSubagentDefaultMaxSteps(t *testing.T, cases []runtimeServerSubagentDefaultMaxStepsCase) {
	t.Helper()
	providerFrames := make([][]string, 0, len(cases)*3)
	for range cases {
		providerFrames = append(providerFrames,
			[]string{
				`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_task_budget","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Check child budget\"}"}}]},"finish_reason":"tool_calls"}]}`,
				`data: [DONE]`,
			},
			[]string{`data: {"choices":[{"delta":{"content":"child budget answer"}}]}`, `data: [DONE]`},
			[]string{`data: {"choices":[{"delta":{"content":"parent budget final"}}]}`, `data: [DONE]`},
		)
	}
	provider := newCompleteProviderServer(t, providerFrames)
	dataDir := t.TempDir()
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "subagent-budget-provider", "subagent-budget-model"),
	}))
	t.Cleanup(server.Close)

	for caseIndex, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workspace := t.TempDir()
			thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
				"title":      "Subagent budget",
				"workspace":  workspace,
				"providerId": "subagent-budget-provider",
				"model":      "subagent-budget-model",
			}), http.StatusCreated)
			threadID := stringField(thread, "id")
			body := map[string]any{
				"prompt":         "Delegate without explicit child max_steps.",
				"approvalPolicy": "auto",
				"sandboxMode":    "read-only",
			}
			if tc.parentSteps != nil {
				body["maxModelSteps"] = *tc.parentSteps
			}
			requestCountBefore := provider.RequestCount()
			assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, body), http.StatusAccepted)
			if got, want := provider.RequestCount(), requestCountBefore+3; got != want {
				t.Fatalf("subagent budget provider calls = %d, want %d", got-requestCountBefore, 3)
			}

			publicRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, threadID)
			if len(publicRuns) != 1 {
				t.Fatalf("expected one thread-scoped child run: %#v", publicRuns)
			}
			jobID := stringField(publicRuns[0], "id")
			if want := fmt.Sprintf("job-%d", caseIndex+1); jobID != want {
				t.Fatalf("subagent budget child job id = %q, want %q", jobID, want)
			}
			assertSecurityBoundChildMetadata(t, publicRuns[0], jobID, "completed", false)
			internalRuns := runtimeServerInternalChildRuns(t, dataDir, threadID)
			if len(internalRuns) != 1 || internalRuns[0].MaxModelSteps == nil || *internalRuns[0].MaxModelSteps != tc.expectedStep {
				t.Fatalf("default child maxModelSteps should mirror parent budget, expected %d got %#v", tc.expectedStep, internalRuns)
			}
		})
	}
}

func TestRuntimeServerDelegateTaskAppliesSubagentProfileModelEffortAndToolScope(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_delegate_profile","type":"function","function":{"name":"task","arguments":"{\"name\":\"reviewer\",\"label\":\"profile child\",\"prompt\":\"Inspect with profile\",\"model\":\"child-model\",\"effort\":\"low\",\"profile\":\"inherit\",\"tools\":[\"ls\"],\"max_steps\":0}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"profile child answer"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":9,"completion_tokens":2,"total_tokens":11,"prompt_cache_hit_tokens":4,"prompt_cache_miss_tokens":5}}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"parent profile final"}}]}`,
			`data: [DONE]`,
		},
	})
	modelProviders := string(mustJSON(t, map[string]any{
		"defaultProviderId": "subagent-profile-provider",
		"providers": []map[string]any{{
			"id":             "subagent-profile-provider",
			"apiKey":         "test-provider-key",
			"baseUrl":        provider.URL(),
			"endpointFormat": "chat_completions",
			"models":         []string{"parent-model", "child-model"},
		}},
	}))
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: modelProviders,
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Delegate profile",
		"workspace":  workspace,
		"providerId": "subagent-profile-provider",
		"model":      "parent-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":          "Delegate with overrides.",
		"approvalPolicy":  "auto",
		"reasoningEffort": "max",
	}), http.StatusAccepted)

	if provider.RequestCount() != 3 {
		t.Fatalf("delegate_task override should call parent, child, parent continuation, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	if !strings.Contains(provider.Body(0), `"model":"parent-model"`) || !strings.Contains(provider.Body(1), `"model":"child-model"`) {
		t.Fatalf("subagent model override did not reach provider requests:\nparent=%s\nchild=%s", provider.Body(0), provider.Body(1))
	}
	childTools := providerRequestToolNames(t, provider.Body(1))
	if !sameStringSet(childTools, []string{"ls"}) {
		t.Fatalf("subagent tool scope override should advertise only ls, got %#v body=%s", childTools, provider.Body(1))
	}
	publicRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, threadID)
	if len(publicRuns) != 1 {
		t.Fatalf("expected one thread-scoped child run: %#v", publicRuns)
	}
	assertSecurityBoundChildMetadata(t, publicRuns[0], "job-1", "completed", false)
	internalRuns := runtimeServerInternalChildRuns(t, dataDir, threadID)
	if len(internalRuns) != 1 {
		t.Fatalf("expected one internal child run: %#v", internalRuns)
	}
	run := internalRuns[0]
	if run.Model != "child-model" ||
		run.Effort != "low" ||
		run.Name != "reviewer" ||
		run.ProfileName != "inherit" ||
		run.ToolPolicy != "inherit" ||
		run.DefaultModelInherited {
		t.Fatalf("subagent profile/model/effort metadata mismatch: %#v", run)
	}
	usage := run.Usage
	if usage != nil {
		t.Fatalf("security-bound child job must not duplicate durable turn usage or retain arbitrary provider payloads: %#v", usage)
	}
	if run.MaxModelSteps == nil || *run.MaxModelSteps != 0 {
		t.Fatalf("explicit subagent max_steps=0 must be preserved as unlimited, got %#v", run)
	}
}

func TestRuntimeServerSubagentUsageSourceSurvivesRestart(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_usage_child","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Measure child usage\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"child usage answer"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":9,"completion_tokens":2,"total_tokens":11,"prompt_cache_hit_tokens":4,"prompt_cache_miss_tokens":5}}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"parent final after usage child"}}]}`,
			`data: [DONE]`,
		},
	})
	newHandler := func() http.Handler {
		return newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
			RuntimeToken:       DefaultRuntimeToken,
			DurableTempDir:     durableRoot,
			Host:               "127.0.0.1",
			Port:               0,
			DataDir:            dataDir,
			ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "subagent-usage-restart-provider", "subagent-usage-restart-model"),
		})
	}

	firstHandler := newHandler()
	server := httptest.NewServer(firstHandler)
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Subagent usage restart",
		"workspace":  workspace,
		"providerId": "subagent-usage-restart-provider",
		"model":      "subagent-usage-restart-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Run child usage.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	events := parseRuntimeServerSSEEvents(t, replay)
	var child map[string]any
	for _, event := range events {
		if stringField(event, "kind") == "pipeline_stage" && stringField(event, "stage") == "subagent_completed" {
			child = mapField(t, event, "child")
			break
		}
	}
	if child == nil {
		t.Fatalf("parent replay should include subagent_completed child metadata: %s", replay)
	}
	assertSecurityBoundChildMetadata(t, child, "job-1", "completed", false)
	internalRuns := runtimeServerInternalChildRuns(t, dataDir, threadID)
	if len(internalRuns) != 1 || internalRuns[0].Usage != nil {
		t.Fatalf("security-bound child job duplicated the accepted turn usage event: %#v", internalRuns)
	}
	server.Close()
	shutdownRuntimeTestHandler(t, firstHandler)

	restarted := httptest.NewServer(newHandler())
	defer restarted.Close()
	usage := assertLiveJSON(t, restarted.URL, http.MethodGet, "/v1/usage", DefaultRuntimeToken, nil, http.StatusOK)
	var subagent map[string]any
	if buckets, ok := usage["bySource"].([]any); ok {
		for _, raw := range buckets {
			bucket, _ := raw.(map[string]any)
			if bucket != nil && bucket["source"] == "subagent" {
				subagent = bucket
				break
			}
		}
	}
	if subagent == nil {
		t.Fatalf("subagent usage source should survive durable restart: %#v", usage)
	}
	subagentUsage := mapField(t, subagent, "usage")
	if subagentUsage["totalTokens"] != float64(11) ||
		subagentUsage["cacheHitTokens"] != float64(4) ||
		subagentUsage["cacheMissTokens"] != float64(5) {
		t.Fatalf("subagent usage/cache should survive durable restart: %#v", subagent)
	}
	childRunIDs, _ := subagent["childRunIds"].([]any)
	if !containsAnyString(childRunIDs, "job-1") {
		t.Fatalf("subagent usage source should retain childRunId after restart: %#v", subagent)
	}
}

func TestRuntimeServerDelegateTaskAppliesConfiguredDefaultSubagentProfile(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_configured_profile","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Inspect configured profile\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"configured profile child answer"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"parent configured profile final"}}]}`,
			`data: [DONE]`,
		},
	})
	modelProviders := string(mustJSON(t, map[string]any{
		"defaultProviderId": "subagent-config-provider",
		"providers": []map[string]any{{
			"id":             "subagent-config-provider",
			"apiKey":         "test-provider-key",
			"baseUrl":        provider.URL(),
			"endpointFormat": "chat_completions",
			"models":         []string{"parent-model", "configured-child-model"},
		}},
	}))
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: modelProviders,
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"subagents": map[string]any{
				"defaultProfile": "configured-reviewer",
				"profiles": map[string]any{
					"configured-reviewer": map[string]any{
						"model":          "configured-child-model",
						"effort":         "low",
						"toolPolicy":     "readOnly",
						"promptPreamble": "Configured reviewer preamble.",
						"tools":          []string{"ls"},
					},
				},
			},
		})),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Configured profile",
		"workspace":  workspace,
		"providerId": "subagent-config-provider",
		"model":      "parent-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Delegate with configured default profile.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)

	if provider.RequestCount() != 3 {
		t.Fatalf("configured subagent profile should call parent, child, parent continuation, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	if !strings.Contains(provider.Body(0), "configured-reviewer") {
		t.Fatalf("parent tool schema should advertise configured subagent profiles:\n%s", provider.Body(0))
	}
	assertProviderToolProfileProperty(t, provider.Body(0), "task")
	assertProviderParallelTasksProfileProperty(t, provider.Body(0))
	if !strings.Contains(provider.Body(1), `"model":"configured-child-model"`) ||
		!strings.Contains(provider.Body(1), "Configured reviewer preamble.") ||
		!strings.Contains(provider.Body(1), "Inspect configured profile") {
		t.Fatalf("configured default subagent profile did not shape the child request:\n%s", provider.Body(1))
	}
	childTools := providerRequestToolNames(t, provider.Body(1))
	if !sameStringSet(childTools, []string{"ls"}) {
		t.Fatalf("configured profile tool scope should advertise only ls, got %#v body=%s", childTools, provider.Body(1))
	}
	publicRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, threadID)
	if len(publicRuns) != 1 {
		t.Fatalf("expected one thread-scoped configured-profile child run: %#v", publicRuns)
	}
	assertSecurityBoundChildMetadata(t, publicRuns[0], "job-1", "completed", false)
	internalRuns := runtimeServerInternalChildRuns(t, dataDir, threadID)
	if len(internalRuns) != 1 {
		t.Fatalf("expected one internal configured-profile child run: %#v", internalRuns)
	}
	run := internalRuns[0]
	if run.ProfileName != "configured-reviewer" ||
		run.ProfileSource != "profile:configured-reviewer" ||
		run.Model != "configured-child-model" ||
		run.Effort != "low" ||
		run.ToolPolicy != "readOnly" {
		t.Fatalf("configured profile metadata mismatch: %#v", run)
	}
}

func TestRuntimeServerSubagentProfileCarriesProviderModelExecutionRef(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	parentProvider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_profile_provider","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Inspect profile provider\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"profile provider child answer"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"parent saw profile child"}}]}`,
			`data: [DONE]`,
		},
	})
	modelProviders := string(mustJSON(t, map[string]any{
		"defaultProviderId": "profile-parent-provider",
		"providers": []map[string]any{
			{
				"id":             "profile-parent-provider",
				"apiKey":         "test-parent-provider-key",
				"baseUrl":        parentProvider.URL(),
				"endpointFormat": "chat_completions",
				"models":         []string{"profile-parent-model", "profile-child-model"},
			},
		},
	}))
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: modelProviders,
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"subagents": map[string]any{
				"defaultProfile": "profile-provider-reviewer",
				"profiles": map[string]any{
					"profile-provider-reviewer": map[string]any{
						"providerId":     "profile-parent-provider",
						"model":          "profile-child-model",
						"endpointFormat": "chat_completions",
						"variant":        "fast-lane",
						"toolPolicy":     "readOnly",
						"tools":          []string{"ls"},
					},
				},
			},
		})),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Profile provider execution",
		"workspace":  workspace,
		"providerId": "profile-parent-provider",
		"model":      "profile-parent-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Delegate with the selected Provider profile model.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)

	if parentProvider.RequestCount() != 3 {
		t.Fatalf("expected parent/child/parent calls through the selected Provider, got %d", parentProvider.RequestCount())
	}
	for index := 0; index < 3; index++ {
		if parentProvider.Path(index) != "/v1/chat/completions" {
			t.Fatal("selected Provider execution used the wrong protocol route")
		}
	}
	for _, index := range []int{0, 2} {
		if !strings.Contains(parentProvider.Body(index), `"model":"profile-parent-model"`) {
			t.Fatalf("parent call %d lost its selected Provider model", index)
		}
	}
	if !strings.Contains(parentProvider.Body(0), "profile-parent-provider") || !strings.Contains(parentProvider.Body(0), "fast-lane") {
		t.Fatalf("parent tool schema should advertise profile provider/model variant metadata:\n%s", parentProvider.Body(0))
	}
	if !strings.Contains(parentProvider.Body(1), `"model":"profile-child-model"`) || strings.Contains(parentProvider.Body(1), "profile-parent-model") {
		t.Fatalf("child request should use profile provider model only:\n%s", parentProvider.Body(1))
	}
	publicRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, threadID)
	if len(publicRuns) != 1 {
		t.Fatalf("expected one thread-scoped profile-provider child run: %#v", publicRuns)
	}
	assertSecurityBoundChildMetadata(t, publicRuns[0], "job-1", "completed", false)
	internalRuns := runtimeServerInternalChildRuns(t, dataDir, threadID)
	if len(internalRuns) != 1 {
		t.Fatalf("expected one internal profile-provider child run: %#v", internalRuns)
	}
	run := internalRuns[0]
	if run.ProviderID != "profile-parent-provider" ||
		run.Model != "profile-child-model" ||
		run.EndpointFormat != "" ||
		run.Variant != "fast-lane" ||
		run.ModelSource != "subagent-profile" {
		t.Fatalf("child run should preserve profile execution identity: %#v", run)
	}
	modelExecution := run.ModelExecution
	if modelExecution["providerId"] != "profile-parent-provider" ||
		modelExecution["modelId"] != "profile-child-model" ||
		modelExecution["variant"] != "fast-lane" ||
		modelExecution["source"] != "subagent-profile" ||
		stringField(modelExecution, "capabilityFingerprint") == "" ||
		stringField(modelExecution, "resolvedAt") == "" {
		t.Fatalf("child run modelExecution ref mismatch: %#v", modelExecution)
	}
	for _, key := range []string{"endpointFormat", "baseUrlFingerprint", "customFullEndpointFingerprint"} {
		if _, exists := modelExecution[key]; exists {
			t.Fatalf("child execution ref persisted Registry route authority: %s", key)
		}
	}

	summary := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID+"/summary", DefaultRuntimeToken, nil, http.StatusOK)
	summarySubagents, _ := summary["subagents"].([]any)
	if len(summarySubagents) != 1 {
		t.Fatalf("thread summary should expose one metadata-only child membership: %#v", summary)
	}
	summaryChild, _ := summarySubagents[0].(map[string]any)
	if stringField(summaryChild, "childRunId") != "job-1" || summaryChild["canReadOutput"] != false || summaryChild["outputWithheld"] != true {
		t.Fatalf("thread summary child projection is not metadata-only: %#v", summaryChild)
	}
	for _, forbidden := range []string{"profile provider child answer", "reasoning", "artifactPath", "error"} {
		if strings.Contains(mustJSONString(t, summaryChild), forbidden) {
			t.Fatalf("thread summary exposed private child output value %q: %#v", forbidden, summaryChild)
		}
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	events := parseRuntimeServerSSEEvents(t, replay)
	completed := runtimeServerEventWithChildStatus(t, events, "completed")
	assertSecurityBoundChildMetadata(t, mapField(t, completed, "child"), "job-1", "completed", false)
	for _, forbidden := range []string{"childModelExecution", "childModelSource", "childProviderId", "profile-child-model", "fast-lane"} {
		if strings.Contains(replay, forbidden) {
			t.Fatalf("SSE replay exposed private child execution field %q:\n%s", forbidden, replay)
		}
	}
}

func TestRuntimeServerSubagentProfileCannotOverrideCommittedProvider(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	parentProvider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_profile_provider","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Inspect profile provider\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"parent saw profile child"}}]}`,
			`data: [DONE]`,
		},
	})
	childProvider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"content":"profile provider child answer"}}]}`,
			`data: [DONE]`,
		},
	})
	modelProviders := string(mustJSON(t, map[string]any{
		"defaultProviderId": "profile-parent-provider",
		"providers": []map[string]any{
			{
				"id":             "profile-parent-provider",
				"apiKey":         "test-parent-provider-key",
				"baseUrl":        parentProvider.URL(),
				"endpointFormat": "chat_completions",
				"models":         []string{"profile-parent-model"},
			},
			{
				"id":             "profile-child-provider",
				"apiKey":         "test-child-provider-key",
				"baseUrl":        childProvider.URL(),
				"endpointFormat": "chat_completions",
				"models":         []string{"profile-child-model"},
			},
		},
	}))
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: modelProviders,
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"subagents": map[string]any{
				"defaultProfile": "profile-provider-reviewer",
				"profiles": map[string]any{
					"profile-provider-reviewer": map[string]any{
						"providerId":     "profile-child-provider",
						"model":          "profile-child-model",
						"endpointFormat": "chat_completions",
						"variant":        "fast-lane",
						"toolPolicy":     "readOnly",
						"tools":          []string{"ls"},
					},
				},
			},
		})),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Profile provider execution",
		"workspace":  workspace,
		"providerId": "profile-parent-provider",
		"model":      "profile-parent-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Delegate with profile provider override.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)

	if parentProvider.RequestCount() != 2 || childProvider.RequestCount() != 0 {
		t.Fatalf("unselected profile Provider executed: selectedCalls=%d unselectedCalls=%d", parentProvider.RequestCount(), childProvider.RequestCount())
	}
	for _, body := range parentProvider.Bodies() {
		if !strings.Contains(body, `"model":"profile-parent-model"`) {
			t.Fatal("rejected child profile replaced the selected Provider model")
		}
	}
	snapshot := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/provider-registry", DefaultRuntimeToken, nil, http.StatusOK)
	if snapshot["selectedProviderId"] != "profile-parent-provider" {
		t.Fatal("subagent profile changed committed Registry selection")
	}
	publicRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, threadID)
	if len(publicRuns) != 1 {
		t.Fatalf("expected one admission-failed child run, got %d", len(publicRuns))
	}
	assertSecurityBoundChildMetadata(t, publicRuns[0], "job-1", "failed", false)
	internalRuns := runtimeServerInternalChildRuns(t, dataDir, threadID)
	if len(internalRuns) != 1 || internalRuns[0].Status != "failed" || internalRuns[0].ChildThreadID == "" {
		t.Fatal("cross-provider profile did not reach and fail real child turn admission")
	}
	if internalRuns[0].FailureCode != "child_execution_failed" || internalRuns[0].ProviderID != "profile-child-provider" || internalRuns[0].Model != "profile-child-model" {
		t.Fatalf("failed child admission lost its closed failure or requested execution ref: code=%s", internalRuns[0].FailureCode)
	}
	_, toolResult := providerHostToolResultForName(t, parentProvider.Body(1), "task")
	assertSecurityBoundChildMetadata(t, toolResult, "job-1", "failed", false)

	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	failed := runtimeServerEventWithChildStatus(t, parseRuntimeServerSSEEvents(t, replay), "failed")
	assertSecurityBoundChildMetadata(t, mapField(t, failed, "child"), "job-1", "failed", false)
	for _, forbidden := range []string{"profile provider child answer", "childModelExecution", "childModelSource", "childProviderId", "profile-child-provider", "fast-lane"} {
		if strings.Contains(replay, forbidden) {
			t.Fatalf("failed child replay exposed private field %q", forbidden)
		}
	}
}

func TestRuntimeServerSubagentLifecycleCountsChildToolInvocations(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "child-note.txt"), []byte("child file contents"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_count_child_tools","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Read child-note.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_child_read","type":"function","function":{"name":"read","arguments":"{\"path\":\"child-note.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"child read done"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"parent saw child tool count"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "subagent-tool-count-provider", "subagent-tool-count-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Subagent tool count",
		"workspace":  workspace,
		"providerId": "subagent-tool-count-provider",
		"model":      "subagent-tool-count-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Delegate and count child tools.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")

	if provider.RequestCount() != 4 {
		t.Fatalf("subagent child tool turn should call parent, child tool, child continuation, parent continuation, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	publicRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, threadID)
	if len(publicRuns) != 1 {
		t.Fatalf("expected one thread-scoped child run: %#v", publicRuns)
	}
	assertSecurityBoundChildMetadata(t, publicRuns[0], "job-1", "completed", false)
	internalRuns := runtimeServerInternalChildRuns(t, dataDir, threadID)
	if len(internalRuns) != 1 || internalRuns[0].ToolInvocations != 1 || internalRuns[0].ChildSeq != 1 {
		t.Fatalf("durable child run should count child tool invocations: %#v", internalRuns)
	}
	childReplay := liveSSE(t, server.URL, "/v1/threads/"+internalRuns[0].ChildThreadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	assertRuntimeServerTypedOrdinaryTerminal(
		t, childReplay, internalRuns[0].ChildTurnID, "child read done", "provider_ordinary_only",
	)
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	events := runtimeServerEventsForTurn(t, replay, turnID)
	stage := runtimeServerEventWithChildStatus(t, events, "completed")
	child := mapField(t, stage, "child")
	assertSecurityBoundChildMetadata(t, child, "job-1", "completed", false)
}

func TestRuntimeServerSubagentConfigEnforcesMaxChildRuns(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_first_child","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"First child\"}"}},{"index":1,"id":"call_second_child","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Second child\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"first child answer"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"parent saw child limit"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "subagent-limit-provider", "subagent-limit-model"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"subagents": map[string]any{
				"maxChildRuns": float64(1),
				"maxParallel":  float64(1),
			},
		})),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Subagent child limit",
		"workspace":  workspace,
		"providerId": "subagent-limit-provider",
		"model":      "subagent-limit-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Try two child runs.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)

	if provider.RequestCount() != 3 {
		t.Fatalf("maxChildRuns=1 should call parent, one child, and parent continuation only, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	if strings.Contains(provider.Body(1), "Second child") {
		t.Fatalf("second child prompt must not reach child provider when maxChildRuns=1:\n%s", provider.Body(1))
	}
	continuation := provider.Body(2)
	taskCallIDs := providerHostToolCallIDsForName(t, continuation, "task")
	if len(taskCallIDs) != 2 {
		t.Fatalf("parent continuation lost host-bound task calls: ids=%#v body=%s", taskCallIDs, continuation)
	}
	rejectedJSON := string(mustJSON(t, providerToolResultForCall(t, continuation, taskCallIDs[1])))
	if !strings.Contains(rejectedJSON, "subagent child run limit reached: maxChildRuns=1") {
		t.Fatalf("parent continuation should receive the host-bound rejected second child result: result=%s body=%s", rejectedJSON, continuation)
	}
	tools := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/tools", DefaultRuntimeToken, nil, http.StatusOK)
	subagents := mapField(t, tools, "subagents")
	if subagents["maxChildRuns"] != float64(1) || subagents["maxParallel"] != float64(1) {
		t.Fatalf("runtime tools should report configured subagent limits: %#v", subagents)
	}
	childRuns, _ := subagents["childRuns"].([]any)
	if len(childRuns) != 0 {
		t.Fatalf("unscoped runtime tools must not expose child-run membership: %#v", childRuns)
	}
	publicRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, threadID)
	if len(publicRuns) != 1 {
		t.Fatalf("maxChildRuns=1 should expose one scoped child projection, got %#v", publicRuns)
	}
	assertSecurityBoundChildMetadata(t, publicRuns[0], "job-1", "completed", false)
	if internalRuns := runtimeServerInternalChildRuns(t, dataDir, threadID); len(internalRuns) != 1 {
		t.Fatalf("maxChildRuns=1 should persist one internal child run, got %#v", internalRuns)
	}
}

func TestRuntimeServerSubagentConfigEnforcesMaxParallel(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	frames := [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_parallel_limit","type":"function","function":{"name":"parallel_tasks","arguments":"{\"tasks\":[{\"id\":\"first\",\"prompt\":\"First max parallel child\"},{\"id\":\"second\",\"prompt\":\"Second max parallel child\"}]}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"first max parallel answer"},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"second max parallel answer"},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"parent saw serial parallel children"},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		},
	}
	var mu sync.Mutex
	bodies := []string{}
	activeChildRequests := 0
	maxActiveChildRequests := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		requestIndex := len(bodies)
		bodies = append(bodies, string(body))
		childRequest := requestIndex == 1 || requestIndex == 2
		if childRequest {
			activeChildRequests++
			if activeChildRequests > maxActiveChildRequests {
				maxActiveChildRequests = activeChildRequests
			}
		}
		mu.Unlock()
		if childRequest {
			time.Sleep(150 * time.Millisecond)
			defer func() {
				mu.Lock()
				activeChildRequests--
				mu.Unlock()
			}()
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		selected := []string{`data: [DONE]`}
		if requestIndex < len(frames) {
			selected = frames[requestIndex]
		}
		for _, frame := range selected {
			_, _ = w.Write([]byte(frame))
			_, _ = w.Write([]byte("\n\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}))
	defer provider.Close()
	bodyAt := func(index int) string {
		mu.Lock()
		defer mu.Unlock()
		if index < 0 || index >= len(bodies) {
			return ""
		}
		return bodies[index]
	}
	requestCount := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(bodies)
	}
	maxActive := func() int {
		mu.Lock()
		defer mu.Unlock()
		return maxActiveChildRequests
	}

	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL, "subagent-max-parallel-provider", "subagent-max-parallel-model"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"subagents": map[string]any{
				"maxParallel":  float64(1),
				"maxChildRuns": float64(8),
			},
		})),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Subagent max parallel",
		"workspace":  workspace,
		"providerId": "subagent-max-parallel-provider",
		"model":      "subagent-max-parallel-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Run two parallel children with maxParallel=1.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")

	if requestCount() != 4 {
		t.Fatalf("maxParallel=1 should call parent, two child turns, and parent continuation, got %d bodies=%#v", requestCount(), bodies)
	}
	if maxActive() != 1 {
		t.Fatalf("maxParallel=1 must not overlap child provider requests, max active child requests=%d bodies=%#v", maxActive(), bodies)
	}
	if !strings.Contains(bodyAt(1), "First max parallel child") || !strings.Contains(bodyAt(2), "Second max parallel child") {
		t.Fatalf("maxParallel=1 should execute ready tasks in provider order:\nfirst=%s\nsecond=%s", bodyAt(1), bodyAt(2))
	}
	tools := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/tools", DefaultRuntimeToken, nil, http.StatusOK)
	subagents := mapField(t, tools, "subagents")
	if subagents["maxParallel"] != float64(1) {
		t.Fatalf("runtime tools should report configured maxParallel: %#v", subagents)
	}
	childRuns, _ := subagents["childRuns"].([]any)
	if len(childRuns) != 0 {
		t.Fatalf("unscoped runtime tools must not expose child-run membership: %#v", childRuns)
	}
	publicRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, threadID)
	if len(publicRuns) != 2 {
		t.Fatalf("maxParallel=1 should expose two scoped child projections, got %#v", publicRuns)
	}
	for _, run := range publicRuns {
		assertSecurityBoundChildMetadata(t, run, "", "completed", false)
	}
	internalRuns := runtimeServerInternalChildRuns(t, dataDir, threadID)
	if len(internalRuns) != 2 {
		t.Fatalf("maxParallel=1 should persist both internal child runs, got %#v", internalRuns)
	}
	secondQueuedMs := 0
	for _, run := range internalRuns {
		if run.ParallelIndex == 2 {
			secondQueuedMs = run.QueuedMs
		}
	}
	if secondQueuedMs <= 0 {
		t.Fatalf("second maxParallel=1 child should record queuedMs > 0, got childRuns=%#v", internalRuns)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	events := runtimeServerEventsForTurn(t, replay, turnID)
	queuedParallelChildren := 0
	for _, event := range events {
		if stringField(event, "kind") != "pipeline_stage" {
			continue
		}
		child, _ := event["child"].(map[string]any)
		if stringField(child, "childStatus") == "queued" {
			assertSecurityBoundChildMetadata(t, child, "", "queued", false)
			queuedParallelChildren++
		}
	}
	if queuedParallelChildren != 2 {
		t.Fatalf("maxParallel=1 should emit queued lifecycle events for both children, got %d in %#v", queuedParallelChildren, events)
	}
}

func TestRuntimeServerSubagentsDisabledRemovesTaskToolsAndRejectsCalls(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_disabled_task","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Should not run\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"parent saw disabled subagents"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "subagent-disabled-provider", "subagent-disabled-model"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"subagents": map[string]any{
				"enabled":      false,
				"maxParallel":  float64(8),
				"maxChildRuns": float64(64),
			},
		})),
	}))
	defer server.Close()

	info := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/info", DefaultRuntimeToken, nil, http.StatusOK)
	subagentCapability := mapField(t, mapField(t, info, "capabilities"), "subagents")
	if subagentCapability["status"] != "disabled" ||
		subagentCapability["enabled"] != false ||
		subagentCapability["available"] != false ||
		subagentCapability["taskToolAvailable"] != false ||
		subagentCapability["parallelTasksToolAvailable"] != false ||
		subagentCapability["modelJobToolsAvailable"] != false {
		t.Fatalf("disabled subagents should report disabled capability state: %#v", subagentCapability)
	}

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Subagents disabled",
		"workspace":  workspace,
		"providerId": "subagent-disabled-provider",
		"model":      "subagent-disabled-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	disabledResponse, disabledBody := liveRequest(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Try disabled subagent.",
		"approvalPolicy": "auto",
	}))
	if disabledResponse.StatusCode != http.StatusInternalServerError {
		t.Fatalf("disabled unadvertised task must fail closed: status=%d body=%s", disabledResponse.StatusCode, string(disabledBody))
	}
	var disabledError map[string]any
	if err := json.Unmarshal(disabledBody, &disabledError); err != nil {
		t.Fatalf("decode disabled subagent rejection: %v", err)
	}
	assertClosedTurnFailureResponse(t, disabledError, "tool_not_advertised", "The provider requested a tool that was not advertised for this turn.")

	parentTools := providerRequestToolNames(t, provider.Body(0))
	for _, hidden := range []string{"delegate_task", "task", "parallel_tasks", "wait", "bash_output", "kill_shell"} {
		if containsString(parentTools, hidden) {
			t.Fatalf("disabled subagents must not advertise %s: %#v body=%s", hidden, parentTools, provider.Body(0))
		}
	}
	bashParameters := providerRequestToolParameters(t, provider.Body(0), "bash")
	bashProperties := mapField(t, bashParameters, "properties")
	if _, ok := bashProperties["run_in_background"]; ok {
		t.Fatalf("disabled subagents should not advertise bash run_in_background without job tools: %#v", bashParameters)
	}
	if _, ok := bashProperties["runInBackground"]; ok {
		t.Fatalf("disabled subagents should not advertise bash runInBackground without job tools: %#v", bashParameters)
	}
	if provider.RequestCount() != 1 {
		t.Fatalf("disabled task call should not start a child provider request, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	tools := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/tools", DefaultRuntimeToken, nil, http.StatusOK)
	subagents := mapField(t, tools, "subagents")
	if subagents["status"] != "disabled" {
		t.Fatalf("runtime tools should report disabled subagents without child runs: %#v", subagents)
	}
	if childRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, threadID); len(childRuns) != 0 {
		t.Fatalf("disabled subagents must not create task jobs: %#v", childRuns)
	}
}

func TestRuntimeServerSubagentInheritProfileHonorsWorkspaceShellBoundary(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_task_bash","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Try bash\",\"profile\":\"inherit\",\"tools\":[\"bash\"]}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_child_bash","type":"function","function":{"name":"bash","arguments":"{\"command\":\"printf child > child-bash.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"child observed shell policy block"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"parent saw policy-blocked child"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "subagent-bash-provider", "subagent-bash-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Subagent bash boundary",
		"workspace":  workspace,
		"providerId": "subagent-bash-provider",
		"model":      "subagent-bash-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Run a child bash attempt.",
		"approvalPolicy": "on-request",
		"sandboxMode":    "workspace-write",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	pendingID := stringField(start, "pendingId")
	if start["status"] != "waiting" || start["pendingKind"] != "approval" || !strings.HasPrefix(pendingID, "appr_") {
		t.Fatalf("parent task should request approval before spawning inherited child: %#v", start)
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/approvals/"+pendingID, DefaultRuntimeToken, mustJSON(t, map[string]string{"decision": "allow"}), http.StatusOK)
	if provider.RequestCount() != 4 {
		t.Fatalf("subagent shell boundary should call parent, child, child continuation, and parent continuation, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	if strings.Contains(provider.Body(1), "run_in_background") || strings.Contains(provider.Body(1), "runInBackground") {
		t.Fatalf("subagent bash schema must remain foreground-only:\n%s", provider.Body(1))
	}
	if _, err := os.Stat(filepath.Join(workspace, "child-bash.txt")); !os.IsNotExist(err) {
		t.Fatalf("subagent bash must not execute without an explicit foreground approval boundary, stat err=%v", err)
	}
	publicRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, threadID)
	if len(publicRuns) != 1 {
		t.Fatalf("expected one thread-scoped gated child run: %#v", publicRuns)
	}
	assertSecurityBoundChildMetadata(t, publicRuns[0], "job-1", "completed", false)
	internalRuns := runtimeServerInternalChildRuns(t, dataDir, threadID)
	if len(internalRuns) != 1 || internalRuns[0].Status != "completed" || internalRuns[0].Output != "" ||
		internalRuns[0].Kind != "subagent" || internalRuns[0].Background || internalRuns[0].ArtifactPath != "" ||
		internalRuns[0].ToolInvocations != 1 {
		t.Fatalf("subagent should recover without persisting its private final output: %#v", internalRuns)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "child-runs", internalRuns[0].ID+".log")); !os.IsNotExist(err) {
		t.Fatalf("security-bound child output artifact must not exist, stat err=%v", err)
	}
	persistedRun, err := os.ReadFile(filepath.Join(dataDir, "child-runs", internalRuns[0].ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(persistedRun, []byte("child observed shell policy block")) {
		t.Fatalf("security-bound child final leaked into the durable job record: %s", persistedRun)
	}
	parentCallID, parentResult := providerHostToolResultForName(t, provider.Body(3), "task")
	assertSecurityBoundChildMetadata(t, parentResult, "", "completed", false)
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	events := runtimeServerEventsForTurn(t, replay, turnID)
	completedProgress := runtimeServerEventWithKindAndChildStatus(t, events, "tool_progress", "completed")
	if stringField(completedProgress, "callId") != parentCallID || strings.Contains(replay, "call_task_bash") {
		t.Fatalf("subagent shell boundary must update parent timeline after policy recovery: %#v", completedProgress)
	}
	assertSecurityBoundChildMetadata(t, mapField(t, completedProgress, "child"), "job-1", "completed", false)
	_, childBashResult := providerHostToolResultForName(t, provider.Body(2), "bash")
	childResultJSON := string(mustJSON(t, childBashResult))
	if !strings.Contains(childResultJSON, `"code":"sandbox_blocked"`) ||
		!strings.Contains(childResultJSON, "bash requires danger-full-access") {
		t.Fatalf("child continuation should receive the host-bound workspace shell policy rejection: result=%s body=%s", childResultJSON, provider.Body(2))
	}
	if strings.Contains(provider.Body(3), "child observed shell policy block") {
		t.Fatalf("parent continuation exposed private child output:\n%s", provider.Body(3))
	}
}

func TestRuntimeServerSubagentFiltersRecursiveAndJobToolsEvenIfCalled(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_parent_task","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Child attempts recursive tools\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_child_task","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"grandchild\"}"}},{"index":1,"id":"call_child_parallel","type":"function","function":{"name":"parallel_tasks","arguments":"{\"tasks\":[{\"prompt\":\"left\"},{\"prompt\":\"right\"}]}"}},{"index":2,"id":"call_child_wait","type":"function","function":{"name":"wait","arguments":"{\"jobIds\":[\"job-404\"],\"timeoutMs\":0}"}},{"index":3,"id":"call_child_skill","type":"function","function":{"name":"run_skill","arguments":"{\"name\":\"recursive\",\"arguments\":\"try skill recursion\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"child saw filtered recursive tools"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"parent saw filtered child"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "subagent-filter-provider", "subagent-filter-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Subagent recursive filter",
		"workspace":  workspace,
		"providerId": "subagent-filter-provider",
		"model":      "subagent-filter-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Run a child that tries hidden tools.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)

	if provider.RequestCount() != 3 {
		t.Fatalf("filtered child should fail closed before a child continuation, then return control to the parent, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	parentContinuation := provider.Body(2)
	_, parentResult := providerHostToolResultForName(t, parentContinuation, "task")
	assertSecurityBoundChildMetadata(t, parentResult, "", "failed", false)
	for _, forbidden := range []string{"grandchild", "call_child_parallel", "call_child_wait", "call_child_skill"} {
		if strings.Contains(parentContinuation, forbidden) {
			t.Fatalf("private child tool arguments leaked into parent continuation (%s):\n%s", forbidden, parentContinuation)
		}
	}
	publicRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, threadID)
	if len(publicRuns) != 1 {
		t.Fatalf("recursive/job tool filtering must expose exactly one scoped child run: %#v", publicRuns)
	}
	assertSecurityBoundChildMetadata(t, publicRuns[0], "job-1", "failed", false)
	internalRuns := runtimeServerInternalChildRuns(t, dataDir, threadID)
	if len(internalRuns) != 1 || internalRuns[0].Status != "failed" || internalRuns[0].Error != "" || internalRuns[0].FailureCode != "child_execution_failed" {
		t.Fatalf("filtered child run should fail closed without a recovery model call: %#v", internalRuns)
	}
	childReplay := liveSSE(t, server.URL, "/v1/threads/"+internalRuns[0].ChildThreadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	for _, forbidden := range []string{"call_child_task", "call_child_parallel", "call_child_wait", "call_child_skill", "grandchild"} {
		if strings.Contains(childReplay, forbidden) {
			t.Fatalf("unadvertised child call reached durable history (%s):\n%s", forbidden, childReplay)
		}
	}
}

func TestApprovalDenyNeverResumesProviderOrStartsChildRun(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_task","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Should not run\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"denied handled"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     durableRoot,
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "subagent-deny-provider", "subagent-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Delegate deny",
		"workspace":  workspace,
		"providerId": "subagent-deny-provider",
		"model":      "subagent-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Delegate only after approval.",
		"approvalPolicy": "on-request",
	}), http.StatusAccepted)
	pendingID := stringField(start, "pendingId")
	if start["pendingKind"] != "approval" || !strings.HasPrefix(pendingID, "appr_") {
		t.Fatalf("task should request approval before creating child run: %#v", start)
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/approvals/"+pendingID, DefaultRuntimeToken, mustJSON(t, map[string]string{"decision": "deny"}), http.StatusOK)
	if provider.RequestCount() != 1 {
		t.Fatalf("approval deny must end at the host boundary without another provider call, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	childRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, threadID)
	if len(childRuns) != 0 {
		t.Fatalf("denied task must not create durable child run: %#v", childRuns)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if !strings.Contains(replay, "approval_denied") || !strings.Contains(replay, "no unverified final response was published") {
		t.Fatalf("approval denial did not publish the fixed host boundary:\n%s", replay)
	}
}

func TestRuntimeServerApprovalNeverBlocksMutatingToolExecution(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_write_never","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"blocked.txt\",\"content\":\"blocked\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"blocked handled"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "approval-never-provider", "approval-never-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Approval never",
		"workspace":  workspace,
		"providerId": "approval-never-provider",
		"model":      "approval-never-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Try to write.",
		"approvalPolicy": "never",
		"sandboxMode":    "workspace-write",
	}), http.StatusAccepted)
	if start["status"] == "waiting" || start["pendingKind"] == "approval" {
		t.Fatalf("approvalPolicy=never should block via tool_result, not request approval: %#v", start)
	}
	if provider.RequestCount() != 2 {
		t.Fatalf("blocked write should continue provider with tool result, got %d", provider.RequestCount())
	}
	if _, err := os.Stat(filepath.Join(workspace, "blocked.txt")); !os.IsNotExist(err) {
		t.Fatalf("approvalPolicy=never must not execute write_file, stat err=%v", err)
	}
	body := provider.Body(1)
	_, writeResult := providerHostToolResultForName(t, body, "write_file")
	resultJSON := string(mustJSON(t, writeResult))
	if !strings.Contains(resultJSON, "approval_policy_blocked") {
		t.Fatalf("provider continuation must include a host-bound blocked tool result: result=%s body=%s", resultJSON, body)
	}
}

func TestRuntimeServerApprovalNeverRejectsRemoteReadOnlyHintAndMutatingMCP(t *testing.T) {
	dataDir := t.TempDir()
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_mcp_read_never","type":"function","function":{"name":"mcp__runtime-mcp__lookup","arguments":"{\"query\":\"needle\"}"}},{"index":1,"id":"call_mcp_mutate_never","type":"function","function":{"name":"mcp__runtime-mcp__mutate","arguments":"{\"value\":\"danger\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"mcp approval never complete"}}]}`,
			`data: [DONE]`,
		},
	})
	var mu sync.Mutex
	calls := map[string]int{}
	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode MCP request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": runtimeMCPInitializeResult("runtime-mcp")})
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"tools": []map[string]any{
				{
					"name":         "lookup",
					"description":  "Lookup runtime MCP data",
					"inputSchema":  map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}},
					"outputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
					"annotations":  map[string]any{"readOnlyHint": true},
				},
				{
					"name":         "mutate",
					"description":  "Mutate runtime MCP data",
					"inputSchema":  map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "string"}}},
					"outputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
				},
			}}})
		case "tools/call":
			name := fmt.Sprint(request.Params["name"])
			args, _ := request.Params["arguments"].(map[string]any)
			mu.Lock()
			calls[name]++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"content": []map[string]any{{
				"type": "text",
				"text": name + " result: " + fmt.Sprint(args["query"]),
			}}, "structuredContent": map[string]any{}}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
	defer mcpServer.Close()
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "mcp-never-provider", "mcp-never-model"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"mcpServers": map[string]any{
				"runtime-mcp": map[string]any{
					"transport":  "http",
					"url":        mcpServer.URL,
					"trustScope": "user",
				},
			},
		})),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Approval never MCP",
		"workspace":      dataDir,
		"providerId":     "mcp-never-provider",
		"model":          "mcp-never-model",
		"approvalPolicy": "never",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Call read-only and mutating MCP tools.",
	}), http.StatusAccepted)
	if start["status"] == "waiting" || start["pendingKind"] == "approval" {
		t.Fatalf("approvalPolicy=never should not create an approval gate for MCP tools: %#v", start)
	}
	if provider.RequestCount() != 2 {
		t.Fatalf("MCP approval never turn should continue provider once with tool results, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	continuation := provider.Body(1)
	_, lookupResult := providerHostToolResultForName(t, continuation, "mcp__runtime-mcp__lookup")
	_, mutateResult := providerHostToolResultForName(t, continuation, "mcp__runtime-mcp__mutate")
	resultsJSON := string(mustJSON(t, lookupResult)) + string(mustJSON(t, mutateResult))
	if strings.Count(resultsJSON, "approval_policy_blocked") < 2 {
		t.Fatalf("all MCP tools must return host-bound blocks without a host read-only allowlist: results=%s body=%s", resultsJSON, continuation)
	}
	mu.Lock()
	lookupCalls := calls["lookup"]
	mutateCalls := calls["mutate"]
	mu.Unlock()
	if lookupCalls != 0 || mutateCalls != 0 {
		t.Fatalf("remote readOnlyHint must not grant unattended authority, calls=%#v", calls)
	}
}

func TestRuntimeServerWriteFileRejectsSymlinkWorkspaceEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink workspace confinement is POSIX-only in this test")
	}
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	outside := filepath.Join(dataDir, "outside")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "out")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_write_symlink_escape","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"out/evil.txt\",\"content\":\"escaped\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"escape blocked"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "symlink-confine-provider", "symlink-confine-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Symlink confine",
		"workspace":  workspace,
		"providerId": "symlink-confine-provider",
		"model":      "symlink-confine-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Try symlink escape.",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusAccepted)
	if provider.RequestCount() != 2 {
		t.Fatalf("write_file symlink escape should continue provider with tool result, got %d", provider.RequestCount())
	}
	if _, err := os.Stat(filepath.Join(outside, "evil.txt")); !os.IsNotExist(err) {
		t.Fatalf("write_file must not write through symlinked workspace dir, stat err=%v", err)
	}
	body := provider.Body(1)
	_, writeResult := providerHostToolResultForName(t, body, "write_file")
	resultJSON := string(mustJSON(t, writeResult))
	if !strings.Contains(resultJSON, "workspace_escape") {
		t.Fatalf("provider continuation must include a host-bound workspace_escape result: result=%s body=%s", resultJSON, body)
	}
}

func TestRuntimeServerWriteFileAllowsConfiguredAllowWriteRoot(t *testing.T) {
	dataDir := runtimeServerPrivateToolArgumentTempDir(t, "allow-write")
	workspace := filepath.Join(dataDir, "workspace")
	allowRoot := filepath.Join(dataDir, "allowed")
	deniedRoot := filepath.Join(dataDir, "denied")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(allowRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(deniedRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	allowedPath := filepath.Join(allowRoot, "ok.txt")
	deniedPath := filepath.Join(deniedRoot, "no.txt")
	allowedArgs := mustJSONString(t, map[string]string{"path": allowedPath, "content": "allowed"})
	deniedArgs := mustJSONString(t, map[string]string{"path": deniedPath, "content": "denied"})
	provider := newCompleteProviderServer(t, [][]string{
		{
			fmt.Sprintf(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_write_denied","type":"function","function":{"name":"write_file","arguments":%q}},{"index":1,"id":"call_write_allowed","type":"function","function":{"name":"write_file","arguments":%q}}]},"finish_reason":"tool_calls"}]}`, deniedArgs, allowedArgs),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"allow-write handled"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "allow-write-provider", "allow-write-model"),
		AllowWriteRoots:    []string{allowRoot},
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Allow write root",
		"workspace":  workspace,
		"providerId": "allow-write-provider",
		"model":      "allow-write-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Try configured write roots.",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusAccepted)

	if provider.RequestCount() != 2 {
		t.Fatalf("allow-write turn should continue provider with paired tool results, got %d", provider.RequestCount())
	}
	if _, err := os.Stat(deniedPath); !os.IsNotExist(err) {
		t.Fatalf("write_file must not write outside workspace or allow_write root, stat err=%v", err)
	}
	data, err := os.ReadFile(allowedPath)
	if err != nil || string(data) != "allowed" {
		t.Fatalf("write_file should write inside configured allow_write root, data=%q err=%v", string(data), err)
	}
	body := provider.Body(1)
	writeCallIDs := providerHostToolCallIDsForName(t, body, "write_file")
	if len(writeCallIDs) != 2 {
		t.Fatalf("allow-write continuation lost host-bound write calls: ids=%#v body=%s", writeCallIDs, body)
	}
	deniedResult := providerToolResultForCall(t, body, writeCallIDs[0])
	allowedResult := providerToolResultForCall(t, body, writeCallIDs[1])
	if stringField(deniedResult, "code") != "workspace_escape" || stringField(allowedResult, "path") != allowedPath ||
		stringField(allowedResult, "code") != "" || stringField(allowedResult, "error") != "" || allowedResult["bytes_written"] != float64(len("allowed")) {
		t.Fatalf("provider continuation did not contain one denied and one concrete successful write: denied=%#v allowed=%#v", deniedResult, allowedResult)
	}
}

func TestRuntimeServerEditAllowsReadBeforeEditInConfiguredAllowWriteRoot(t *testing.T) {
	dataDir := runtimeServerPrivateToolArgumentTempDir(t, "allow-read-edit")
	workspace := filepath.Join(dataDir, "workspace")
	allowRoot := filepath.Join(dataDir, "allowed")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(allowRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	allowedPath := filepath.Join(allowRoot, "note.txt")
	if err := os.WriteFile(allowedPath, []byte("first line\nold value\nlast line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	readAlias := filestore.ExternalReadRootAlias(allowRoot) + "/note.txt"
	readArgs := mustJSONString(t, map[string]string{"path": readAlias})
	editArgs := mustJSONString(t, map[string]string{
		"path":    allowedPath,
		"oldText": "old value",
		"newText": "new value",
	})
	provider := newCompleteProviderServer(t, [][]string{
		{
			fmt.Sprintf(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read_allow_root","type":"function","function":{"name":"read_file","arguments":%q}},{"index":1,"id":"call_edit_allow_root","type":"function","function":{"name":"edit_file","arguments":%q}}]},"finish_reason":"tool_calls"}]}`, readArgs, editArgs),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"allow-root edit handled"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "allow-read-edit-provider", "allow-read-edit-model"),
		AllowWriteRoots:    []string{allowRoot},
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Allow write root read before edit",
		"workspace":  workspace,
		"providerId": "allow-read-edit-provider",
		"model":      "allow-read-edit-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Read then edit configured allow_write root.",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusAccepted)

	if provider.RequestCount() != 2 {
		t.Fatalf("allow_write read/edit turn should continue provider with tool results, got %d", provider.RequestCount())
	}
	data, err := os.ReadFile(allowedPath)
	if err != nil || string(data) != "first line\nnew value\nlast line\n" {
		t.Fatalf("edit_file should mutate configured allow_write root after read, data=%q err=%v", string(data), err)
	}
	body := provider.Body(1)
	_, readResult := providerHostToolResultForName(t, body, "read_file")
	_, editResult := providerHostToolResultForName(t, body, "edit_file")
	if stringField(readResult, "code") != "" || stringField(readResult, "error") != "" ||
		stringField(editResult, "path") != allowedPath || editResult["replacements"] != float64(1) ||
		stringField(editResult, "code") != "" || stringField(editResult, "error") != "" {
		t.Fatalf("provider continuation did not contain concrete successful allow_write read/edit results: read=%#v edit=%#v", readResult, editResult)
	}
}

func TestRuntimeServerProtectedReadDirsBlockDirectReadAndPruneWalk(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	protectedDir := filepath.Join(workspace, "protected")
	publicDir := filepath.Join(workspace, "public")
	if err := os.MkdirAll(protectedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(publicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(protectedDir, "secret.txt"), []byte("hidden-value"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(publicDir, "visible.txt"), []byte("visible-value"), 0o644); err != nil {
		t.Fatal(err)
	}
	readArgs := mustJSONString(t, map[string]string{"path": "protected/secret.txt"})
	findArgs := mustJSONString(t, map[string]string{"path": ".", "pattern": "secret.txt"})
	grepArgs := mustJSONString(t, map[string]string{"path": ".", "pattern": "needle"})
	provider := newCompleteProviderServer(t, [][]string{
		{
			fmt.Sprintf(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read_protected","type":"function","function":{"name":"read_file","arguments":%q}},{"index":1,"id":"call_find_protected","type":"function","function":{"name":"find","arguments":%q}},{"index":2,"id":"call_grep_protected","type":"function","function":{"name":"grep","arguments":%q}}]},"finish_reason":"tool_calls"}]}`, readArgs, findArgs, grepArgs),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"protected handled"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "protected-read-provider", "protected-read-model"),
		ProtectedReadDirs:  []string{protectedDir},
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":       "Protected reads",
		"workspace":   workspace,
		"providerId":  "protected-read-provider",
		"model":       "protected-read-model",
		"sandboxMode": "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Inspect protected paths.",
	}), http.StatusAccepted)

	if provider.RequestCount() != 2 {
		t.Fatalf("protected read turn should continue provider with paired tool results, got %d", provider.RequestCount())
	}
	secondBody := provider.Body(1)
	_, readResult := providerHostToolResultForName(t, secondBody, "read_file")
	_, findResult := providerHostToolResultForName(t, secondBody, "find")
	_, grepResult := providerHostToolResultForName(t, secondBody, "grep")
	resultsJSON := string(mustJSON(t, readResult)) + string(mustJSON(t, findResult)) + string(mustJSON(t, grepResult))
	if !strings.Contains(resultsJSON, "protected_dir") ||
		!strings.Contains(resultsJSON, "skipped_protected") ||
		strings.Contains(resultsJSON, "hidden-value") {
		t.Fatalf("host-bound provider results must block/prune protected content without leaking file bytes: results=%s body=%s", resultsJSON, secondBody)
	}
}

func TestRuntimeServerParallelTasksCreateDurableChildRuns(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_parallel","type":"function","function":{"name":"parallel_tasks","arguments":"{\"tasks\":[{\"description\":\"left\",\"prompt\":\"Left child\"},{\"description\":\"right\",\"prompt\":\"Right child\"}]}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"child one"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"child two"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"parallel complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "parallel-provider", "parallel-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Parallel tasks",
		"workspace":  workspace,
		"providerId": "parallel-provider",
		"model":      "parallel-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Run in parallel.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	if provider.RequestCount() != 4 {
		t.Fatalf("parallel_tasks should call provider parent + 2 children + continuation, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	publicRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, threadID)
	if len(publicRuns) != 2 {
		t.Fatalf("parallel_tasks should expose two scoped child runs: %#v", publicRuns)
	}
	for _, run := range publicRuns {
		assertSecurityBoundChildMetadata(t, run, "", "completed", false)
	}
	internalRuns := runtimeServerInternalChildRuns(t, dataDir, threadID)
	if len(internalRuns) != 2 {
		t.Fatalf("parallel_tasks should create two internal child runs: %#v", internalRuns)
	}
	groupID := ""
	for _, run := range internalRuns {
		if run.Status != "completed" || run.ParallelGroupID == "" {
			t.Fatalf("parallel child run mismatch: %#v", run)
		}
		if groupID == "" {
			groupID = run.ParallelGroupID
		} else if run.ParallelGroupID != groupID {
			t.Fatalf("parallel child runs should share group id: %#v", internalRuns)
		}
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	events := runtimeServerEventsForTurn(t, replay, turnID)
	completedParallelChildren := 0
	for _, event := range events {
		if stringField(event, "kind") != "pipeline_stage" {
			continue
		}
		child, _ := event["child"].(map[string]any)
		if stringField(child, "childStatus") != "completed" {
			continue
		}
		assertSecurityBoundChildMetadata(t, child, "", "completed", false)
		completedParallelChildren++
	}
	if completedParallelChildren != 2 {
		t.Fatalf("parallel_tasks should emit two completed child timeline events, got %d in %#v", completedParallelChildren, events)
	}
}

func TestRuntimeServerParallelTasksHonorDependsOnWaves(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_parallel_dep","type":"function","function":{"name":"parallel_tasks","arguments":"{\"tasks\":[{\"id\":\"first\",\"description\":\"first\",\"prompt\":\"First child\"},{\"id\":\"second\",\"depends_on\":[\"first\"],\"description\":\"second\",\"prompt\":\"Second child\"}]}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"first child done"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"second child done"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"dependent parallel complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "parallel-dep-provider", "parallel-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Parallel task dependencies",
		"workspace":  workspace,
		"providerId": "parallel-dep-provider",
		"model":      "parallel-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Run dependent subtasks.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)

	if provider.RequestCount() != 4 {
		t.Fatalf("dependent parallel_tasks should call provider parent + 2 children + continuation, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	if !strings.Contains(provider.Body(1), "First child") || !strings.Contains(provider.Body(2), "Second child") {
		t.Fatalf("dependent parallel_tasks should execute dependency waves in order:\nfirst=%s\nsecond=%s", provider.Body(1), provider.Body(2))
	}
	if strings.Contains(provider.Body(2), "Dependency results") || strings.Contains(provider.Body(2), "first child done") {
		t.Fatalf("dependent child request must not receive security-bound prior child output:\n%s", provider.Body(2))
	}
	continuation := provider.Body(3)
	_, parallelResult := providerCurrentTurnHostToolResultForName(t, continuation, "parallel_tasks")
	resultJSON := string(mustJSON(t, parallelResult))
	if !strings.Contains(resultJSON, "untrusted_child_output") || strings.Contains(resultJSON, "first child done") || strings.Contains(resultJSON, "second child done") {
		t.Fatalf("parent continuation should receive only host-bound withheld dependent task metadata: result=%s body=%s", resultJSON, continuation)
	}
	publicRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, threadID)
	if len(publicRuns) != 2 {
		t.Fatalf("dependent parallel_tasks should expose two scoped child runs: %#v", publicRuns)
	}
	for _, run := range publicRuns {
		assertSecurityBoundChildMetadata(t, run, "", "completed", false)
	}
	internalRuns := runtimeServerInternalChildRuns(t, dataDir, threadID)
	if len(internalRuns) != 2 {
		t.Fatalf("dependent parallel_tasks should create two internal child runs: %#v", internalRuns)
	}
	first, second := internalRuns[0], internalRuns[1]
	if first.ParallelIndex != 1 || second.ParallelIndex != 2 ||
		first.ParallelGroupID == "" || first.ParallelGroupID != second.ParallelGroupID {
		t.Fatalf("dependent parallel child run metadata mismatch: %#v", internalRuns)
	}
}

func TestRuntimeServerParallelTasksRunReadyWaveConcurrently(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	bodies := []string{}
	activeChildren := 0
	maxActiveChildren := 0
	childRequests := 0
	childGate := make(chan struct{})
	var openChildGate sync.Once
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		body := string(bodyBytes)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		switch {
		case providerCurrentTurnHasHostToolResults(body, "parallel_tasks"):
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"parallel ready wave complete"},"finish_reason":"stop"}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		case strings.Contains(body, "Left concurrent child") || strings.Contains(body, "Right concurrent child"):
			mu.Lock()
			activeChildren++
			childRequests++
			if activeChildren > maxActiveChildren {
				maxActiveChildren = activeChildren
			}
			if childRequests == 2 {
				openChildGate.Do(func() { close(childGate) })
			}
			mu.Unlock()
			select {
			case <-childGate:
			case <-time.After(2 * time.Second):
			}
			content := "left child done"
			if strings.Contains(body, "Right concurrent child") {
				content = "right child done"
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`data: {"choices":[{"delta":{"content":%q},"finish_reason":"stop"}]}`, content) + "\n\n"))
			_, _ = w.Write([]byte(`data: {"choices":[],"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			mu.Lock()
			activeChildren--
			mu.Unlock()
		default:
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_parallel_concurrent","type":"function","function":{"name":"parallel_tasks","arguments":"{\"tasks\":[{\"id\":\"left\",\"prompt\":\"Left concurrent child\"},{\"id\":\"right\",\"prompt\":\"Right concurrent child\"}]}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		}
	}))
	defer provider.Close()
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL, "parallel-concurrent-provider", "parallel-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Parallel task concurrency",
		"workspace":  workspace,
		"providerId": "parallel-concurrent-provider",
		"model":      "parallel-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Run ready wave concurrently.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)

	mu.Lock()
	gotMaxActiveChildren := maxActiveChildren
	gotChildRequests := childRequests
	gotBodies := append([]string(nil), bodies...)
	mu.Unlock()
	if gotChildRequests != 2 || gotMaxActiveChildren < 2 {
		t.Fatalf("parallel_tasks ready wave should have two children in flight before either completes: childRequests=%d maxActive=%d bodies=%#v", gotChildRequests, gotMaxActiveChildren, gotBodies)
	}
	if len(gotBodies) != 4 {
		t.Fatalf("parallel ready wave should call provider parent + 2 children + continuation, got %d bodies=%#v", len(gotBodies), gotBodies)
	}
}

func TestRuntimeServerSubagentContinueAndForkUseDurableTranscript(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_task_first","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"First child prompt\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"first child answer"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":6,"completion_tokens":2,"total_tokens":8}}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"first parent final"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_task_continue","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Continue child prompt\",\"continue_from\":\"job-1\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"continue source rejected"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_task_fork","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Fork child prompt\",\"fork_from\":\"job-1\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"fork source rejected"}}]}`,
			`data: [DONE]`,
		},
	})
	modelProvidersJSON, connectProviderRegistry := prepareRuntimeServerExplicitProviderRegistryFixture(
		t, dataDir, provider.URL(), "continue-provider", "continue-model",
	)
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: modelProvidersJSON,
	}))
	defer server.Close()
	connectProviderRegistry(server.URL)

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Continue subagent",
		"workspace":  workspace,
		"providerId": "continue-provider",
		"model":      "continue-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	for _, prompt := range []string{"Run first child.", "Continue child.", "Fork child."} {
		assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
			"prompt":         prompt,
			"approvalPolicy": "auto",
		}), http.StatusAccepted)
	}
	if provider.RequestCount() != 7 {
		t.Fatalf("security-bound continue/fork must stop before child provider execution, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	_, continueResult := providerCurrentTurnHostToolResultForName(t, provider.Body(4), "task")
	_, forkResult := providerCurrentTurnHostToolResultForName(t, provider.Body(6), "task")
	assertSecurityBoundChildMetadata(t, continueResult, "", "failed", false)
	assertSecurityBoundChildMetadata(t, forkResult, "", "failed", false)
	for _, body := range []string{provider.Body(4), provider.Body(6)} {
		if strings.Contains(body, "first child answer") {
			t.Fatalf("security-bound source output leaked into parent continuation:\n%s", body)
		}
	}
	publicRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, threadID)
	if len(publicRuns) != 1 {
		t.Fatalf("only the current-turn failed fork may remain publicly scoped: %#v", publicRuns)
	}
	assertSecurityBoundChildMetadata(t, publicRuns[0], "job-3", "failed", false)
	internalRuns := runtimeServerInternalChildRuns(t, dataDir, threadID)
	if len(internalRuns) != 3 {
		t.Fatalf("expected three durable internal child runs: %#v", internalRuns)
	}
	run1, run2, run3 := internalRuns[0], internalRuns[1], internalRuns[2]
	if run1.ID != "job-1" || run2.SourceRef != "job-1" || run2.ContinueFrom != "job-1" ||
		run2.Status != "failed" || run2.Error != "" || run2.FailureCode != "child_execution_failed" {
		t.Fatalf("continue_from record mismatch: %#v %#v", run1, run2)
	}
	if run3.SourceRef != "job-1" || run3.ForkFrom != "job-1" || run3.Status != "failed" ||
		run3.Error != "" || run3.FailureCode != "child_execution_failed" {
		t.Fatalf("fork_from record mismatch: run1=%#v run3=%#v", run1, run3)
	}
}

func TestRuntimeServerSubagentContinueInheritsSourceIdentityWhenParentModelChanges(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_task_first","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"First child prompt\",\"model\":\"child-special\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"source child answer"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"source parent final"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_task_continue","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Continue child prompt\",\"continue_from\":\"job-1\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"continued child answer"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"continued parent final"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSONWithModels(provider.URL(), "deepseek-subagent-identity", "parent-model", "parent-model-v2", "child-special"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Continue subagent source identity",
		"workspace":  workspace,
		"providerId": "deepseek-subagent-identity",
		"model":      "parent-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Run source child.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Continue child after parent model changed.",
		"model":          "parent-model-v2",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)

	if provider.RequestCount() != 5 {
		t.Fatalf("security-bound continue_from must stop before a second child provider call, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	if !strings.Contains(provider.Body(1), `"model":"child-special"`) {
		t.Fatalf("source child request should use explicit child model:\n%s", provider.Body(1))
	}
	_, continueResult := providerCurrentTurnHostToolResultForName(t, provider.Body(4), "task")
	assertSecurityBoundChildMetadata(t, continueResult, "", "failed", false)
	if strings.Contains(provider.Body(4), "source child answer") {
		t.Fatalf("continue_from exposed source child output:\n%s", provider.Body(4))
	}
	publicRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, threadID)
	if len(publicRuns) != 1 {
		t.Fatalf("only the current-turn failed continuation may remain publicly scoped: %#v", publicRuns)
	}
	assertSecurityBoundChildMetadata(t, publicRuns[0], "job-2", "failed", false)
	internalRuns := runtimeServerInternalChildRuns(t, dataDir, threadID)
	if len(internalRuns) != 2 {
		t.Fatalf("expected two internal child records: %#v", internalRuns)
	}
	run1, run2 := internalRuns[0], internalRuns[1]
	if run1.Model != "child-special" || run2.Model != "child-special" || run2.SourceRef != "job-1" ||
		run2.Status != "failed" || run2.Error != "" || run2.FailureCode != "child_execution_failed" {
		t.Fatalf("child run identity should stay pinned to source: run1=%#v run2=%#v", run1, run2)
	}
}

func TestRuntimeServerRejectsConcurrentSubagentContinueFromSameReference(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_task_first","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Initial child prompt\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"initial child answer"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"initial parent final"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_parallel_continue","type":"function","function":{"name":"parallel_tasks","arguments":"{\"tasks\":[{\"id\":\"left\",\"prompt\":\"Continue left\",\"continue_from\":\"job-1\"},{\"id\":\"right\",\"prompt\":\"Continue right\",\"continue_from\":\"job-1\"}]}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"continued once"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"parallel continue handled"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "continue-lock-provider", "continue-lock-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Continue lock",
		"workspace":  workspace,
		"providerId": "continue-lock-provider",
		"model":      "continue-lock-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	for _, prompt := range []string{"Run initial child.", "Run concurrent child continues."} {
		assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
			"prompt":         prompt,
			"approvalPolicy": "auto",
		}), http.StatusAccepted)
	}

	if provider.RequestCount() != 5 {
		t.Fatalf("security-bound concurrent continues must not reach a child provider, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	continuation := provider.Body(4)
	_, parallelResult := providerCurrentTurnHostToolResultForName(t, continuation, "parallel_tasks")
	resultJSON := string(mustJSON(t, parallelResult))
	if !strings.Contains(resultJSON, "untrusted_child_output") ||
		strings.Contains(resultJSON, "initial child answer") || strings.Contains(resultJSON, "continued once") {
		t.Fatalf("parent continuation should receive only host-bound withheld failed child metadata: result=%s body=%s", resultJSON, continuation)
	}
	publicRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, threadID)
	if len(publicRuns) != 2 {
		t.Fatalf("only the two current-turn failed attempts may remain publicly scoped: %#v", publicRuns)
	}
	assertSecurityBoundChildMetadata(t, publicRuns[0], "job-2", "failed", false)
	assertSecurityBoundChildMetadata(t, publicRuns[1], "job-3", "failed", false)
	internalRuns := runtimeServerInternalChildRuns(t, dataDir, threadID)
	if len(internalRuns) != 3 || internalRuns[1].Status != "failed" || internalRuns[2].Status != "failed" ||
		internalRuns[1].Error != "" || internalRuns[1].FailureCode != "child_execution_failed" ||
		internalRuns[2].Error != "" || internalRuns[2].FailureCode != "child_execution_failed" {
		t.Fatalf("concurrent continue attempts did not fail closed: %#v", internalRuns)
	}
}

func TestRuntimeServerAllowsSubagentForkFromAncestorParentThread(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_task_first","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Ancestor child prompt\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"ancestor child answer"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"ancestor parent final"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_fork_from_ancestor","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Fork ancestor child\",\"fork_from\":\"job-1\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"forked ancestor child answer"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"forked ancestor parent final"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "ancestor-fork-provider", "ancestor-fork-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Ancestor fork source",
		"workspace":  workspace,
		"providerId": "ancestor-fork-provider",
		"model":      "ancestor-fork-model",
	}), http.StatusCreated)
	parentThreadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+parentThreadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Create ancestor child.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)
	fork := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+parentThreadID+"/fork", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"relation": "fork",
		"title":    "Forked parent",
	}), http.StatusCreated)
	forkThreadID := stringField(fork, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+forkThreadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Fork ancestor child.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)

	if provider.RequestCount() != 5 {
		t.Fatalf("ancestor security-bound fork_from must stop before child execution, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	_, forkResult := providerCurrentTurnHostToolResultForName(t, provider.Body(4), "task")
	assertSecurityBoundChildMetadata(t, forkResult, "", "failed", false)
	if strings.Contains(provider.Body(4), "ancestor child answer") {
		t.Fatalf("fork_from exposed ancestor child output:\n%s", provider.Body(4))
	}
	sourceRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, parentThreadID)
	forkRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, forkThreadID)
	if len(sourceRuns) != 1 || len(forkRuns) != 1 {
		t.Fatalf("expected source and failed fork child projections: source=%#v fork=%#v", sourceRuns, forkRuns)
	}
	assertSecurityBoundChildMetadata(t, sourceRuns[0], "job-1", "completed", false)
	assertSecurityBoundChildMetadata(t, forkRuns[0], "job-2", "failed", false)
	internalForkRuns := runtimeServerInternalChildRuns(t, dataDir, forkThreadID)
	if len(internalForkRuns) != 1 {
		t.Fatalf("expected one internal failed fork attempt: %#v", internalForkRuns)
	}
	run2 := internalForkRuns[0]
	if run2.ParentThreadID != forkThreadID || run2.SourceRef != "job-1" || run2.ForkFrom != "job-1" ||
		run2.Status != "failed" || run2.Error != "" || run2.FailureCode != "child_execution_failed" {
		t.Fatalf("ancestor fork_from metadata mismatch: source=%#v run2=%#v", runtimeServerInternalChildRuns(t, dataDir, parentThreadID), run2)
	}
}

func TestRuntimeServerCopiesAncestorSubagentContinueIntoCurrentParent(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_task_first","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Ancestor child prompt\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"ancestor child answer"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"ancestor parent final"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_continue_from_ancestor","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Continue ancestor child\",\"continue_from\":\"job-1\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"continued ancestor child answer"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"continued ancestor parent final"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "ancestor-continue-provider", "ancestor-continue-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Ancestor continue source",
		"workspace":  workspace,
		"providerId": "ancestor-continue-provider",
		"model":      "ancestor-continue-model",
	}), http.StatusCreated)
	parentThreadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+parentThreadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Create ancestor child.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)
	fork := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+parentThreadID+"/fork", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"relation": "fork",
		"title":    "Forked parent",
	}), http.StatusCreated)
	forkThreadID := stringField(fork, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+forkThreadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Continue ancestor child.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)

	if provider.RequestCount() != 5 {
		t.Fatalf("ancestor security-bound continue_from must stop before child execution, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	_, continueResult := providerCurrentTurnHostToolResultForName(t, provider.Body(4), "task")
	assertSecurityBoundChildMetadata(t, continueResult, "", "failed", false)
	if strings.Contains(provider.Body(4), "ancestor child answer") {
		t.Fatalf("continue_from exposed ancestor child output:\n%s", provider.Body(4))
	}
	sourceRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, parentThreadID)
	continuedRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, forkThreadID)
	if len(sourceRuns) != 1 || len(continuedRuns) != 1 {
		t.Fatalf("expected source and failed continue child projections: source=%#v continued=%#v", sourceRuns, continuedRuns)
	}
	assertSecurityBoundChildMetadata(t, sourceRuns[0], "job-1", "completed", false)
	assertSecurityBoundChildMetadata(t, continuedRuns[0], "job-2", "failed", false)
	internalContinueRuns := runtimeServerInternalChildRuns(t, dataDir, forkThreadID)
	if len(internalContinueRuns) != 1 {
		t.Fatalf("expected one internal failed continue attempt: %#v", internalContinueRuns)
	}
	run2 := internalContinueRuns[0]
	if run2.ParentThreadID != forkThreadID || run2.SourceRef != "job-1" || run2.ContinueFrom != "job-1" ||
		run2.Status != "failed" || run2.Error != "" || run2.FailureCode != "child_execution_failed" {
		t.Fatalf("ancestor continue_from metadata mismatch: source=%#v run2=%#v", runtimeServerInternalChildRuns(t, dataDir, parentThreadID), run2)
	}
}

func TestRuntimeServerRejectsSubagentContinueWhenToolScopeDrifts(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_source_task","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Source child\",\"tools\":[\"ls\"]}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"source child with ls"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":6,"completion_tokens":2,"total_tokens":8}}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"source parent final"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_continue_drift","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Continue with incompatible tools\",\"continue_from\":\"job-1\",\"tools\":[\"read\"]}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"drift handled"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "scope-drift-provider", "scope-drift-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Subagent scope drift",
		"workspace":  workspace,
		"providerId": "scope-drift-provider",
		"model":      "scope-drift-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	for _, prompt := range []string{"Create scoped child.", "Continue child with drift."} {
		assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
			"prompt":         prompt,
			"approvalPolicy": "auto",
		}), http.StatusAccepted)
	}

	if provider.RequestCount() != 5 {
		t.Fatalf("tool-scope drift should not launch a second child provider call, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	continuation := provider.Body(4)
	_, driftResult := providerCurrentTurnHostToolResultForName(t, continuation, "task")
	resultJSON := string(mustJSON(t, driftResult))
	if !strings.Contains(resultJSON, "uses a different tool scope") {
		t.Fatalf("parent continuation must receive a host-bound incompatible subagent reference result: result=%s body=%s", resultJSON, continuation)
	}
	publicRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, threadID)
	if len(publicRuns) != 0 {
		t.Fatalf("stale source child membership must not cross into the current turn: %#v", publicRuns)
	}
	internalRuns := runtimeServerInternalChildRuns(t, dataDir, threadID)
	if len(internalRuns) != 1 || len(internalRuns[0].ToolScope) != 1 || internalRuns[0].ToolScope[0] != "ls" {
		t.Fatalf("source child run should retain original stable tool scope: %#v", internalRuns)
	}
}

func TestRuntimeServerStartupInterruptsStaleRunningSubagentRuns(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	manager, err := jobs.NewManager(filepath.Join(dataDir, "child-runs"))
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.StartChildRun(jobs.StartRequest{
		ParentGoalID:     "goal_stale",
		ParentThreadID:   "thr_durable_1",
		ParentTurnID:     "turn_stale",
		ParentToolCallID: "call_task_stale",
		Kind:             "subagent",
		Name:             "review",
		Label:            "stale child",
		Status:           "running",
		Workspace:        workspace,
		Model:            "stale-model",
		Effort:           "medium",
		ToolPolicy:       "readOnly",
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.ID != "job-1" || record.Status != "running" {
		t.Fatalf("test setup should create a running child run: %#v", record)
	}

	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_continue_stale","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Continue stale child\",\"continue_from\":\"job-1\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"stale child handled"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "stale-provider", "stale-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Stale subagent",
		"workspace":  workspace,
		"providerId": "stale-provider",
		"model":      "stale-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	if threadID != "thr_durable_1" {
		t.Fatalf("test assumes first durable thread id matches stale parent id, got %q", threadID)
	}
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Continue the stale child.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")

	if provider.RequestCount() != 2 {
		t.Fatalf("interrupted stale run must not launch a child provider call, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	continuation := provider.Body(1)
	_, staleResult := providerCurrentTurnHostToolResultForName(t, continuation, "task")
	resultJSON := string(mustJSON(t, staleResult))
	if !strings.Contains(resultJSON, "subagent reference") ||
		!strings.Contains(resultJSON, "job-1") ||
		!strings.Contains(resultJSON, "interrupted and cannot be continued or forked") {
		t.Fatalf("parent continuation should receive a host-bound interrupted subagent result: result=%s body=%s", resultJSON, continuation)
	}
	publicRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, threadID)
	if len(publicRuns) != 0 {
		t.Fatalf("unbound stale child membership must not become public: %#v", publicRuns)
	}
	internalRuns := runtimeServerInternalChildRuns(t, dataDir, threadID)
	if len(internalRuns) != 1 || internalRuns[0].ID != "job-1" || internalRuns[0].Status != "interrupted" {
		t.Fatalf("startup cleanup should mark stale running child run interrupted: %#v", internalRuns)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	staleEvents := runtimeServerEventsForTurn(t, replay, "turn_stale")
	if len(staleEvents) != 0 {
		t.Fatalf("startup cleanup must not create orphan events for a missing parent thread: %#v", staleEvents)
	}
	turnEvents := runtimeServerEventsForTurn(t, replay, turnID)
	if !hasRuntimeServerEvent(turnEvents, "turn_completed") {
		t.Fatalf("new parent turn should still complete after stale cleanup:\n%s", replay)
	}
}

func TestRuntimeServerRejectsCrossParentSubagentContinue(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_task_source","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Source child\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"source child answer"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":6,"completion_tokens":2,"total_tokens":8}}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"source parent final"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_task_cross","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Cross parent\",\"continue_from\":\"job-1\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"cross parent handled"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "lineage-provider", "lineage-model"),
	}))
	defer server.Close()

	sourceThread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Source parent",
		"workspace":  workspace,
		"providerId": "lineage-provider",
		"model":      "lineage-model",
	}), http.StatusCreated)
	sourceThreadID := stringField(sourceThread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+sourceThreadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Create source child.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)

	otherThread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Other parent",
		"workspace":  workspace,
		"providerId": "lineage-provider",
		"model":      "lineage-model",
	}), http.StatusCreated)
	otherThreadID := stringField(otherThread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+otherThreadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Try cross-parent child continue.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)

	if provider.RequestCount() != 5 {
		t.Fatalf("cross-parent reference should not launch a child provider call, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	if strings.Contains(provider.Body(4), "source child answer") {
		t.Fatalf("cross-parent subagent transcript leaked into other parent request:\n%s", provider.Body(4))
	}
	hostCallID, _ := providerCurrentTurnHostToolResultForName(t, provider.Body(4), "task")
	replay := liveSSE(t, server.URL, "/v1/threads/"+otherThreadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	foundClosedRejection := false
	for _, event := range parseRuntimeServerSSEEvents(t, replay) {
		if stringField(event, "kind") != "tool_call_finished" {
			continue
		}
		item, _ := event["item"].(map[string]any)
		if stringField(item, "callId") != hostCallID {
			continue
		}
		projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(item["output"])
		if err != nil || projection.ProjectionKind != domaintoolresult.ProjectionHostStatus || projection.Status != "failed" ||
			projection.MessageKey != "tool_failed" || projection.Code != "validation_error" {
			t.Fatalf("cross-parent rejection did not use the closed public projection: item=%#v err=%v", item, err)
		}
		foundClosedRejection = true
	}
	if !foundClosedRejection {
		t.Fatalf("cross-parent rejection tool result is missing:\n%s", replay)
	}
	if strings.Contains(replay, "call_task_cross") {
		t.Fatalf("cross-parent replay retained the untrusted provider call id:\n%s", replay)
	}
	for _, forbidden := range []string{"belongs to parent thread", "source child answer"} {
		if strings.Contains(replay, forbidden) {
			t.Fatalf("cross-parent public replay exposed private detail %q:\n%s", forbidden, replay)
		}
	}
	if publicRuns := runtimeServerScopedChildRuns(t, server.URL, DefaultRuntimeToken, otherThreadID); len(publicRuns) != 0 {
		t.Fatalf("cross-parent rejection created public child membership: %#v", publicRuns)
	}
}

func TestRuntimeServerBackgroundTaskJobsWaitAndOutput(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	bodies := []string{}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		body := string(bodyBytes)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		switch {
		case strings.Contains(body, "background-jobs"):
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"parent saw background note"},"finish_reason":"stop"}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		case providerCurrentTurnHasHostToolResults(body, "task"):
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"parent accepted background"},"finish_reason":"stop"}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		case strings.Contains(body, "Continue after background."):
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"continued after background"},"finish_reason":"stop"}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		case strings.Contains(body, "Background child"):
			time.Sleep(25 * time.Millisecond)
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"background child done"},"finish_reason":"stop"}]}` + "\n\n"))
			_, _ = w.Write([]byte(`data: {"choices":[],"usage":{"prompt_tokens":8,"completion_tokens":3,"total_tokens":11}}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_bg","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Background child\",\"run_in_background\":true}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		}
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}))
	defer provider.Close()
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL, "background-provider", "background-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Background task",
		"workspace":  workspace,
		"providerId": "background-provider",
		"model":      "background-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Start background.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	otherThread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Other background parent",
		"workspace":  workspace,
		"providerId": "background-provider",
		"model":      "background-model",
	}), http.StatusCreated)
	otherThreadID := stringField(otherThread, "id")

	wait := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/runtime/task-jobs/wait", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"jobIds":    []string{"job-1"},
		"timeoutMs": 2000,
		"threadId":  threadID,
	}), http.StatusOK)
	jobsRaw, _ := wait["jobs"].([]any)
	if len(jobsRaw) != 1 {
		t.Fatalf("wait should return the background job: %#v", wait)
	}
	job, _ := jobsRaw[0].(map[string]any)
	assertSecurityBoundChildMetadata(t, job, "job-1", "completed", true)
	internalRuns := runtimeServerInternalChildRuns(t, dataDir, threadID)
	if len(internalRuns) != 1 || internalRuns[0].Status != "completed" ||
		internalRuns[0].Kind != "subagent" || !internalRuns[0].Background || internalRuns[0].ArtifactPath != "" || internalRuns[0].Output != "" {
		t.Fatalf("background job should complete durably without persisting private child output: %#v", internalRuns)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "child-runs", internalRuns[0].ID+".log")); !os.IsNotExist(err) {
		t.Fatalf("security-bound child output artifact must not exist, stat err=%v", err)
	}
	persistedRun, err := os.ReadFile(filepath.Join(dataDir, "child-runs", internalRuns[0].ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(persistedRun, []byte("background child done")) {
		t.Fatalf("security-bound background child final leaked into the durable job record: %s", persistedRun)
	}
	waitAll := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/runtime/task-jobs/wait", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"timeoutMs": 0,
		"threadId":  threadID,
	}), http.StatusOK)
	waitAllJobs, _ := waitAll["jobs"].([]any)
	if len(waitAllJobs) != 1 {
		t.Fatalf("wait without job ids should return jobs owned by the parent thread only: %#v", waitAll)
	}
	assertSecurityBoundChildMetadata(t, waitAllJobs[0].(map[string]any), "job-1", "completed", true)
	output := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/runtime/task-jobs/output", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"jobId":    "job-1",
		"threadId": threadID,
	}), http.StatusOK)
	assertSecurityBoundTaskOutputV1(t, output, "job-1", "completed")
	offset := len("background ")
	suffix := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/runtime/task-jobs/output", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"jobId":    "job-1",
		"offset":   offset,
		"threadId": threadID,
	}), http.StatusOK)
	assertSecurityBoundTaskOutputV1(t, suffix, "job-1", "completed")
	limited := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/runtime/task-jobs/output", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"jobId":    "job-1",
		"offset":   offset,
		"limit":    5,
		"threadId": threadID,
	}), http.StatusOK)
	assertSecurityBoundTaskOutputV1(t, limited, "job-1", "completed")
	resumed := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/runtime/task-jobs/output", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"jobId":    "job-1",
		"offset":   offset + 5,
		"threadId": threadID,
	}), http.StatusOK)
	assertSecurityBoundTaskOutputV1(t, resumed, "job-1", "completed")
	canonicalOutput := string(mustJSON(t, output))
	for name, projection := range map[string]map[string]any{"suffix": suffix, "limited": limited, "resumed": resumed} {
		if got := string(mustJSON(t, projection)); got != canonicalOutput {
			t.Fatalf("security-bound %s request created an output length/cursor side channel:\n got=%s\nwant=%s", name, got, canonicalOutput)
		}
	}
	next := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Continue after background.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)
	nextTurnID := stringField(next, "turnId")
	var nextReplay string
	var nextEvents []map[string]any
	for i := 0; i < 100; i++ {
		nextReplay = liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
		nextEvents = runtimeServerEventsForTurn(t, nextReplay, nextTurnID)
		if hasRuntimeServerEvent(nextEvents, "turn_completed") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !hasRuntimeServerEvent(nextEvents, "turn_completed") {
		t.Fatalf("parent continuation after background note did not complete:\n%s", nextReplay)
	}
	foreignWait := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/runtime/task-jobs/wait", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"jobIds":    []string{"job-1"},
		"threadId":  otherThreadID,
		"timeoutMs": 0,
	}), http.StatusOK)
	if jobs, _ := foreignWait["jobs"].([]any); len(jobs) != 0 {
		t.Fatalf("foreign wait must not expose task-job membership: %#v", foreignWait)
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/runtime/task-jobs/output", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"jobId":    "job-1",
		"threadId": otherThreadID,
	}), http.StatusNotFound)
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/runtime/task-jobs/wait", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"jobIds":    []string{"job-1"},
		"timeoutMs": 0,
	}), http.StatusBadRequest)
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/runtime/task-jobs/output", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"jobId": "job-1",
	}), http.StatusBadRequest)
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	events := runtimeServerEventsForTurn(t, replay, turnID)
	runningProgress := runtimeServerEventWithKindAndChildStatus(t, events, "tool_progress", "running")
	completedProgress := runtimeServerEventWithKindAndChildStatus(t, events, "tool_progress", "completed")
	for _, progress := range []map[string]any{runningProgress, completedProgress} {
		child := mapField(t, progress, "child")
		assertSecurityBoundChildMetadata(t, child, "job-1", stringField(child, "childStatus"), true)
	}
	hostTaskCallID := stringField(runningProgress, "callId")
	if !domainsecurity.IsHostToolCallIDV1(hostTaskCallID) || stringField(completedProgress, "callId") != hostTaskCallID || strings.Contains(replay, "call_bg") {
		t.Fatalf("background completion should stay linked to the parent call: %#v", completedProgress)
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/runtime/task-jobs/output", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"jobId":    "missing-job",
		"threadId": threadID,
	}), http.StatusNotFound)
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/runtime/task-jobs/wait", "", mustJSON(t, map[string]any{
		"jobIds": []string{"job-1"},
	}), http.StatusUnauthorized)
	mu.Lock()
	bodySnapshot := append([]string(nil), bodies...)
	mu.Unlock()
	bodyCount := len(bodySnapshot)
	if bodyCount < 3 {
		t.Fatalf("background task should call parent, child, and parent continuation providers, got %d", bodyCount)
	}
	privateContinuationCount := 0
	durableProjectionCount := 0
	for _, body := range bodySnapshot {
		if !providerHistoryHasToolCallName(body, "task") {
			continue
		}
		callIDs := providerHostToolCallIDsForName(t, body, "task")
		if len(callIDs) != 1 || callIDs[0] != hostTaskCallID {
			t.Fatalf("background task history did not preserve its host call id: ids=%#v want=%s body=%s", callIDs, hostTaskCallID, body)
		}
		projected := providerToolResultForCall(t, body, hostTaskCallID)
		if projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(projected); err == nil {
			if projection.ProjectionKind != domaintoolresult.ProjectionHostStatus || projection.Status != "completed" ||
				projection.MessageKey != "tool_completed" || projection.Code != "tool_completed" {
				t.Fatalf("durable background history projection mismatch: %#v", projection)
			}
			durableProjectionCount++
		} else {
			assertSecurityBoundChildMetadata(t, projected, "", "queued", true)
			privateContinuationCount++
		}
		if strings.Contains(body, "artifactPath") || strings.Contains(body, "background child done") {
			t.Fatalf("background task tool result exposed private artifact/output: %s", body)
		}
	}
	if privateContinuationCount != 1 || durableProjectionCount == 0 {
		t.Fatalf("background task should expose one attempt-private queued result and closed durable history, private=%d durable=%d bodies=%#v",
			privateContinuationCount, durableProjectionCount, bodySnapshot)
	}
	backgroundNoteRequests := 0
	for _, body := range bodySnapshot {
		if strings.Contains(body, "background-jobs") {
			backgroundNoteRequests++
		}
		if strings.Contains(body, "background child done") {
			t.Fatalf("untrusted background child output must not be injected into a later provider request: %s", body)
		}
	}
	if backgroundNoteRequests != 0 {
		t.Fatalf("security-bound background jobs must not create transient provider notes, got %d in %#v", backgroundNoteRequests, bodySnapshot)
	}
}

func TestRuntimeServerBashRunInBackgroundWithholdsOutputAndSupportsKill(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("background shell process-group kill uses the POSIX shell path in this contract test")
	}
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_bash_bg","type":"function","function":{"name":"bash","arguments":"{\"command\":\"sleep 20 & echo $! > child.pid; printf started; wait\",\"run_in_background\":true,\"timeout\":120}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"background bash accepted"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "background-shell-provider", "background-shell-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Background shell",
		"workspace":      workspace,
		"providerId":     "background-shell-provider",
		"model":          "background-shell-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "danger-full-access",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Start background shell.",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	if provider.RequestCount() != 2 {
		t.Fatalf("background bash should start job and continue parent provider loop, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	if !strings.Contains(provider.Body(0), "run_in_background") || !strings.Contains(provider.Body(0), "runInBackground") {
		t.Fatalf("parent bash schema should advertise background shell controls:\n%s", provider.Body(0))
	}
	hostCallID, bashResult := providerHostToolResultForName(t, provider.Body(1), "bash")
	assertSecurityBoundChildMetadata(t, bashResult, "", "running", true)
	if strings.Contains(provider.Body(1), "artifactPath") || strings.Contains(provider.Body(1), "background_shell") {
		t.Fatalf("parent continuation exposed private background shell metadata:\n%s", provider.Body(1))
	}

	var output map[string]any
	var internalRun jobs.Record
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		output = assertLiveJSON(t, server.URL, http.MethodPost, "/v1/runtime/task-jobs/output", DefaultRuntimeToken, mustJSON(t, map[string]any{
			"jobId":    "job-1",
			"threadId": threadID,
		}), http.StatusOK)
		assertSecurityBoundTaskOutputV1(t, output, "job-1", "running")
		internalRuns := runtimeServerInternalChildRuns(t, dataDir, threadID)
		if len(internalRuns) == 1 {
			internalRun = internalRuns[0]
		}
		if _, err := os.Stat(filepath.Join(workspace, "child.pid")); err == nil && internalRun.ID != "" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if internalRun.Output != "" || internalRun.Error != "" || internalRun.Status != "running" || internalRun.Kind != "background-shell" || internalRun.SecurityBinding == nil {
		t.Fatalf("background bash must retain only bound control metadata: public=%#v internal=%#v", output, internalRun)
	}
	pidBytes, err := os.ReadFile(filepath.Join(workspace, "child.pid"))
	if err != nil {
		t.Fatalf("background shell should record child pid: %v", err)
	}
	childPID := strings.TrimSpace(string(pidBytes))
	if childPID == "" {
		t.Fatalf("background child pid file was empty")
	}
	killed := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/runtime/task-jobs/kill", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"jobId":    "job-1",
		"threadId": threadID,
		"reason":   "test cleanup",
	}), http.StatusOK)
	killedJob := mapField(t, killed, "job")
	assertSecurityBoundChildMetadata(t, killedJob, "job-1", "killed", true)
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if exec.Command("kill", "-0", childPID).Run() != nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if exec.Command("kill", "-0", childPID).Run() == nil {
		t.Fatalf("background shell child process %s survived kill_shell cancellation", childPID)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	events := runtimeServerEventsForTurn(t, replay, turnID)
	progress := runtimeServerEventWithKindAndChildStatus(t, events, "tool_progress", "running")
	child := mapField(t, progress, "child")
	if stringField(progress, "callId") != hostCallID || strings.Contains(replay, "call_bash_bg") {
		t.Fatalf("background bash replay did not preserve its host call id: progress=%#v", progress)
	}
	assertSecurityBoundChildMetadata(t, child, "job-1", "running", true)
}

func TestRuntimeServerModelCannotInspectBackgroundTaskJobsAcrossTurns(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	bodies := []string{}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		body := string(bodyBytes)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		switch {
		case providerCurrentTurnHasHostToolResults(body, "bash_output", "kill_shell"):
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"observed cursor output"},"finish_reason":"stop"}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		case providerCurrentTurnHasHostToolResults(body, "wait", "bash_output"):
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"observed background job tools"},"finish_reason":"stop"}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		case providerCurrentTurnHasHostToolResults(body, "task"):
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"parent accepted background job"},"finish_reason":"stop"}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		case strings.Contains(body, "Inspect background cursor again"):
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_output_again","type":"function","function":{"name":"bash_output","arguments":"{\"jobId\":\"job-1\"}"}},{"index":1,"id":"call_job_kill","type":"function","function":{"name":"kill_shell","arguments":"{\"jobId\":\"job-1\",\"reason\":\"model cleanup\"}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		case strings.Contains(body, "Inspect background via tools"):
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_wait","type":"function","function":{"name":"wait","arguments":"{\"timeout_seconds\":1}"}},{"index":1,"id":"call_output","type":"function","function":{"name":"bash_output","arguments":"{\"jobId\":\"job-1\",\"filter\":\"tools\"}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		case strings.Contains(body, "Background child for tools"):
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"background child for tools done"},"finish_reason":"stop"}]}` + "\n\n"))
			_, _ = w.Write([]byte(`data: {"choices":[],"usage":{"prompt_tokens":8,"completion_tokens":3,"total_tokens":11}}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		case strings.Contains(body, "Start background via task"):
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_bg","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Background child for tools\",\"run_in_background\":true}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"unexpected request"}}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		}
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}))
	defer provider.Close()
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL, "background-model-tools-provider", "background-model-tools"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Background model tools",
		"workspace":  workspace,
		"providerId": "background-model-tools-provider",
		"model":      "background-model-tools",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	startTurn := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Start background via task.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)
	startTurnID := stringField(startTurn, "turnId")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/runtime/task-jobs/wait", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"jobIds":    []string{"job-1"},
		"timeoutMs": 2000,
		"threadId":  threadID,
	}), http.StatusOK)
	var startReplay string
	var startEvents []map[string]any
	for i := 0; i < 100; i++ {
		startReplay = liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
		startEvents = runtimeServerEventsForTurn(t, startReplay, startTurnID)
		if hasRuntimeServerEvent(startEvents, "turn_completed") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !hasRuntimeServerEvent(startEvents, "turn_completed") {
		t.Fatalf("background start turn did not complete:\n%s", startReplay)
	}
	inspectResponse, inspectData := liveRequest(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Inspect background via tools.",
		"approvalPolicy": "auto",
	}))
	if inspectResponse.StatusCode != http.StatusAccepted {
		t.Fatalf("inspect background turn status mismatch: got %d want %d body=%s", inspectResponse.StatusCode, http.StatusAccepted, string(inspectData))
	}
	inspectTurn := map[string]any{}
	if err := json.Unmarshal(inspectData, &inspectTurn); err != nil {
		t.Fatalf("inspect background turn returned invalid JSON: %v", err)
	}
	inspectTurnID := stringField(inspectTurn, "turnId")
	var inspectReplay string
	var inspectEvents []map[string]any
	for i := 0; i < 100; i++ {
		inspectReplay = liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
		inspectEvents = runtimeServerEventsForTurn(t, inspectReplay, inspectTurnID)
		if hasRuntimeServerEvent(inspectEvents, "turn_completed") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !hasRuntimeServerEvent(inspectEvents, "turn_completed") {
		t.Fatalf("background inspection turn did not complete:\n%s", inspectReplay)
	}
	cursorTurn := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Inspect background cursor again.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)
	cursorTurnID := stringField(cursorTurn, "turnId")
	var cursorReplay string
	var cursorEvents []map[string]any
	for i := 0; i < 100; i++ {
		cursorReplay = liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
		cursorEvents = runtimeServerEventsForTurn(t, cursorReplay, cursorTurnID)
		if hasRuntimeServerEvent(cursorEvents, "turn_completed") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !hasRuntimeServerEvent(cursorEvents, "turn_completed") {
		t.Fatalf("background cursor turn did not complete:\n%s", cursorReplay)
	}

	mu.Lock()
	bodySnapshot := append([]string(nil), bodies...)
	mu.Unlock()
	var secondTurnRequest string
	var continuation string
	var cursorTurnRequest string
	var cursorContinuation string
	for _, body := range bodySnapshot {
		if strings.Contains(body, "Inspect background via tools") &&
			!providerCurrentTurnHasHostToolResults(body, "wait", "bash_output") {
			secondTurnRequest = body
		}
		if providerCurrentTurnHasHostToolResults(body, "wait", "bash_output") {
			continuation = body
		}
		if strings.Contains(body, "Inspect background cursor again") &&
			!providerCurrentTurnHasHostToolResults(body, "bash_output", "kill_shell") {
			cursorTurnRequest = body
		}
		if providerCurrentTurnHasHostToolResults(body, "bash_output", "kill_shell") {
			cursorContinuation = body
		}
	}
	if secondTurnRequest == "" || continuation == "" || cursorTurnRequest == "" || cursorContinuation == "" {
		t.Fatalf("background model job tool loop did not issue expected provider requests: %#v", bodySnapshot)
	}
	toolNames := providerRequestToolNames(t, secondTurnRequest)
	for _, expected := range []string{"wait", "bash_output", "kill_shell"} {
		if !containsString(toolNames, expected) {
			t.Fatalf("parent provider request must advertise background job tool %s: %#v body=%s", expected, toolNames, secondTurnRequest)
		}
	}
	if !strings.Contains(secondTurnRequest, `"filter"`) ||
		!strings.Contains(secondTurnRequest, `"cursor"`) ||
		!strings.Contains(secondTurnRequest, `"timeout_seconds"`) {
		t.Fatalf("background job tool schemas should advertise bash_output cursor/filter and wait timeout_seconds controls:\n%s", secondTurnRequest)
	}
	waitIDs := providerCurrentTurnHostToolResultIDs(continuation, "wait")
	outputIDs := providerCurrentTurnHostToolResultIDs(continuation, "bash_output")
	if len(waitIDs) != 1 || len(outputIDs) != 1 {
		t.Fatalf("later turn lost host-bound background job results: wait=%#v output=%#v body=%s", waitIDs, outputIDs, continuation)
	}
	continuationResults := string(mustJSON(t, providerToolResultForCall(t, continuation, waitIDs[0]))) +
		string(mustJSON(t, providerToolResultForCall(t, continuation, outputIDs[0])))
	if strings.Contains(continuationResults, "background child for tools done") ||
		strings.Count(continuationResults, `"code":"not_found"`) < 2 ||
		strings.Contains(continuationResults, `"error"`) {
		t.Fatalf("later turn must receive only fail-closed background job tool results: results=%s body=%s", continuationResults, continuation)
	}
	cursorOutputIDs := providerCurrentTurnHostToolResultIDs(cursorContinuation, "bash_output")
	killIDs := providerCurrentTurnHostToolResultIDs(cursorContinuation, "kill_shell")
	if len(cursorOutputIDs) != 1 || len(killIDs) != 1 {
		t.Fatalf("cursor turn lost host-bound background job results: output=%#v kill=%#v body=%s", cursorOutputIDs, killIDs, cursorContinuation)
	}
	cursorResults := string(mustJSON(t, providerToolResultForCall(t, cursorContinuation, cursorOutputIDs[0]))) +
		string(mustJSON(t, providerToolResultForCall(t, cursorContinuation, killIDs[0])))
	if strings.Contains(cursorResults, "background child for tools done") ||
		strings.Count(cursorResults, `"code":"not_found"`) < 2 ||
		strings.Contains(cursorResults, `"error"`) {
		t.Fatalf("later turn must not read or control the prior turn's background job: results=%s body=%s", cursorResults, cursorContinuation)
	}
}

func TestRuntimeServerBackgroundTaskJobKillCancelsChildRun(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	childStarted := make(chan struct{})
	childDone := make(chan struct{})
	var childStartedOnce sync.Once
	var childDoneOnce sync.Once
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		body := string(bodyBytes)
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		switch {
		case providerCurrentTurnHasHostToolResults(body, "task"):
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"parent accepted cancellable child"},"finish_reason":"stop"}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		case strings.Contains(body, "Blocked child"):
			childStartedOnce.Do(func() { close(childStarted) })
			<-r.Context().Done()
			childDoneOnce.Do(func() { close(childDone) })
		default:
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_kill","type":"function","function":{"name":"task","arguments":"{\"prompt\":\"Blocked child\",\"run_in_background\":true}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		}
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}))
	defer provider.Close()
	handler := newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL, "kill-provider", "kill-model"),
	})
	server := httptest.NewServer(handler)
	defer func() {
		if shutdown, ok := handler.(interface{ Shutdown(context.Context) error }); ok {
			ctx, cancel := context.WithTimeout(context.Background(), runtimeServerShutdownTestTimeout)
			defer cancel()
			_ = shutdown.Shutdown(ctx)
		}
		server.Close()
	}()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Kill background task",
		"workspace":  workspace,
		"providerId": "kill-provider",
		"model":      "kill-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	otherThread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Other kill parent",
		"workspace":  workspace,
		"providerId": "kill-provider",
		"model":      "kill-model",
	}), http.StatusCreated)
	otherThreadID := stringField(otherThread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Start cancellable background.",
		"approvalPolicy": "auto",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	select {
	case <-childStarted:
	case <-time.After(runtimeServerPositiveTestTimeout):
		t.Fatalf("background child provider request did not start")
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/runtime/task-jobs/kill", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"jobId":  "job-1",
		"reason": "missing parent scope",
	}), http.StatusBadRequest)
	foreign := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/runtime/task-jobs/kill", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"jobId":    "job-1",
		"threadId": otherThreadID,
		"reason":   "wrong parent",
	}), http.StatusNotFound)
	if foreign["code"] != "not_found" {
		t.Fatalf("foreign task-job membership must be indistinguishable from missing: %#v", foreign)
	}
	killRequestBody := mustJSON(t, map[string]any{
		"jobId":    "job-1",
		"threadId": threadID,
		"reason":   "test cancellation",
	})
	killResponse, killData := liveRequest(t, server.URL, http.MethodPost, "/v1/runtime/task-jobs/kill", DefaultRuntimeToken, killRequestBody)
	var kill map[string]any
	if err := json.Unmarshal(killData, &kill); err != nil {
		t.Fatalf("decode task-job kill response: %v body=%s", err, killData)
	}
	killSettling := killResponse.StatusCode == http.StatusConflict
	switch killResponse.StatusCode {
	case http.StatusOK:
		assertSecurityBoundChildMetadata(t, mapField(t, kill, "job"), "job-1", "killed", true)
	case http.StatusConflict:
		if kill["code"] != "conflict" || kill["message"] != "The request conflicts with the current runtime state." || kill["job"] != nil {
			t.Fatalf("task-job kill conflict must fail closed without stale job state: %#v", kill)
		}
	default:
		t.Fatalf("task-job kill status mismatch: got %d want %d or %d body=%s", killResponse.StatusCode, http.StatusOK, http.StatusConflict, killData)
	}
	waitTimeoutMS := 100
	if killSettling {
		waitTimeoutMS = int(runtimeServerShutdownTestTimeout / time.Millisecond)
	}
	wait := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/runtime/task-jobs/wait", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"jobIds":    []string{"job-1"},
		"timeoutMs": waitTimeoutMS,
		"threadId":  threadID,
	}), http.StatusOK)
	jobsRaw, _ := wait["jobs"].([]any)
	if len(jobsRaw) != 1 {
		t.Fatalf("wait should return killed job: %#v", wait)
	}
	waited, _ := jobsRaw[0].(map[string]any)
	assertSecurityBoundChildMetadata(t, waited, "job-1", "killed", true)
	if killSettling {
		retry := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/runtime/task-jobs/kill", DefaultRuntimeToken, killRequestBody, http.StatusOK)
		assertSecurityBoundChildMetadata(t, mapField(t, retry, "job"), "job-1", "killed", true)
	}
	select {
	case <-childDone:
	case <-time.After(runtimeServerPositiveTestTimeout):
		t.Fatalf("background child provider request was not cancelled after kill")
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/runtime/task-jobs/kill", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"jobId":    "missing-job",
		"threadId": threadID,
	}), http.StatusNotFound)
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	events := runtimeServerEventsForTurn(t, replay, turnID)
	runningProgress := runtimeServerEventWithKindAndChildStatus(t, events, "tool_progress", "running")
	killedProgress := runtimeServerEventWithKindAndChildStatus(t, events, "tool_progress", "killed")
	hostTaskCallID := stringField(runningProgress, "callId")
	if !domainsecurity.IsHostToolCallIDV1(hostTaskCallID) || stringField(killedProgress, "callId") != hostTaskCallID || strings.Contains(replay, "call_kill") {
		t.Fatalf("kill replay should update the original background task card: %#v", killedProgress)
	}
	killedChild := mapField(t, killedProgress, "child")
	assertSecurityBoundChildMetadata(t, killedChild, "job-1", "killed", true)
}

func TestRuntimeServerKunBuiltinToolsAdvertiseAndExecute(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "alpha.txt"), []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_ls","type":"function","function":{"name":"ls","arguments":"{\"path\":\".\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"listed"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":20,"completion_tokens":2,"total_tokens":22}}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "kun-tools-provider", "tool-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Kun tools",
		"workspace":  workspace,
		"providerId": "kun-tools-provider",
		"model":      "tool-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "List the workspace.",
	}), http.StatusAccepted)

	toolNames := providerRequestToolNames(t, provider.Body(0))
	for _, expected := range []string{"read", "bash", "write", "edit", "grep", "find", "glob", "code_index", "ls", "read_file", "write_file", "edit_file", "multi_edit", "move_file", "notebook_edit", "delete_range", "delete_symbol", "user_input", "request_user_input"} {
		if !containsString(toolNames, expected) {
			t.Fatalf("provider tool catalog missing %s: %#v", expected, toolNames)
		}
	}
	if containsString(toolNames, "create_plan") {
		t.Fatalf("normal agent turns must not advertise create_plan: %#v", toolNames)
	}
	if provider.RequestCount() != 2 {
		t.Fatalf("ls tool loop should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	body := provider.Body(1)
	_, lsResult := providerHostToolResultForName(t, body, "ls")
	resultJSON := string(mustJSON(t, lsResult))
	if !strings.Contains(resultJSON, "alpha.txt") {
		t.Fatalf("second provider call must include a host-bound ls result: result=%s body=%s", resultJSON, body)
	}
}

func TestRuntimeServerGoalToolsRequireEvidenceBeforeCompletion(t *testing.T) {
	dataDir := t.TempDir()
	provider := newCompleteProviderServer(t, [][]string{
		{
			fmt.Sprintf(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_goal_complete","type":"function","function":{"name":"update_goal","arguments":%q}}]},"finish_reason":"tool_calls"}]}`, `{"status":"complete"}`),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"goal still active"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "goal-provider", "goal-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Goal evidence",
		"workspace":  dataDir,
		"providerId": "goal-provider",
		"model":      "goal-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/goal", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"objective": "Ship only after evidence",
	}), http.StatusOK)
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Try to mark the goal complete.",
	}), http.StatusAccepted)
	if provider.RequestCount() != 2 {
		t.Fatalf("goal update tool loop should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	if !strings.Contains(provider.Body(0), `"name":"update_goal"`) || !strings.Contains(provider.Body(0), `"name":"complete_step"`) {
		t.Fatalf("active goal should advertise update_goal and complete_step tools:\n%s", provider.Body(0))
	}
	body := provider.Body(1)
	_, goalResult := providerHostToolResultForName(t, body, "update_goal")
	resultJSON := string(mustJSON(t, goalResult))
	if !strings.Contains(resultJSON, "goal_evidence_required") {
		t.Fatalf("goal completion without evidence must return a host-bound tool error: result=%s body=%s", resultJSON, body)
	}
	goal := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID+"/goal", DefaultRuntimeToken, nil, http.StatusOK)
	if mapField(t, goal, "goal")["status"] != "active" {
		t.Fatalf("goal must remain active without evidence: %#v", goal)
	}
}

func TestRuntimeServerGoalTodoCompleteStepAndFinalReadiness(t *testing.T) {
	dataDir := t.TempDir()
	todoArgs := string(mustJSON(t, map[string]any{
		"todos": []map[string]any{{"content": "Audit contract", "status": "in_progress"}},
	}))
	stepArgs := string(mustJSON(t, map[string]any{
		"step":   "Audit contract",
		"result": "contract audited",
		"evidence": []map[string]any{{
			"kind":    "verification",
			"summary": "runtime contract verification ran",
			"command": "printf verified",
		}},
	}))
	bashArgs := `{"command":"printf verified"}`
	completeArgs := `{"status":"complete"}`
	provider := newCompleteProviderServer(t, [][]string{
		{
			fmt.Sprintf(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_todo_write","type":"function","function":{"name":"todo_write","arguments":%q}},{"index":1,"id":"call_bash_verify","type":"function","function":{"name":"bash","arguments":%q}},{"index":2,"id":"call_complete_step","type":"function","function":{"name":"complete_step","arguments":%q}},{"index":3,"id":"call_goal_complete","type":"function","function":{"name":"update_goal","arguments":%q}}]},"finish_reason":"tool_calls"}]}`, todoArgs, bashArgs, stepArgs, completeArgs),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"goal complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "goal-provider", "goal-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Goal todo",
		"workspace":  dataDir,
		"providerId": "goal-provider",
		"model":      "goal-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/goal", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"objective": "Complete with evidence",
	}), http.StatusOK)
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Update todos, verify, sign off the step, then complete the goal.",
		"approvalPolicy": "auto",
		"sandboxMode":    "danger-full-access",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	if provider.RequestCount() != 2 {
		t.Fatalf("goal todo tool loop should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	body := provider.Body(1)
	resultsJSON := ""
	for _, toolName := range []string{"todo_write", "bash", "complete_step", "update_goal"} {
		_, result := providerHostToolResultForName(t, body, toolName)
		resultsJSON += string(mustJSON(t, result))
	}
	for _, expected := range []string{"Goal achieved", "hostVerified"} {
		if !strings.Contains(resultsJSON, expected) {
			t.Fatalf("host-bound goal/todo results missing %s: results=%s body=%s", expected, resultsJSON, body)
		}
	}
	goal := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID+"/goal", DefaultRuntimeToken, nil, http.StatusOK)
	goalBody := mapField(t, goal, "goal")
	ledger, _ := goalBody["evidenceLedger"].([]any)
	if goalBody["status"] != "complete" || len(ledger) != 1 {
		t.Fatalf("goal should be complete with exactly one evidence entry: %#v", goal)
	}
	todos := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID+"/todos", DefaultRuntimeToken, nil, http.StatusOK)
	todoItems, _ := mapField(t, todos, "todos")["items"].([]any)
	if len(todoItems) != 1 {
		t.Fatalf("expected one todo item: %#v", todos)
	}
	todoItem, _ := todoItems[0].(map[string]any)
	if stringField(todoItem, "status") != "completed" {
		t.Fatalf("complete_step should advance the matching todo to completed: %#v", todos)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	events := runtimeServerEventsForTurn(t, replay, turnID)
	for _, kind := range []string{"tool_call_finished", "item_completed", "turn_completed"} {
		if !hasRuntimeServerEvent(events, kind) {
			t.Fatalf("goal/todo loop replay missing %s:\n%s", kind, replay)
		}
	}
	if strings.Contains(replay, "event: goal_updated") || strings.Contains(replay, "event: todos_updated") || strings.Contains(replay, "runtime contract verification ran") {
		t.Fatalf("goal/todo durable replay exposed untyped goal/todo/evidence prose:\n%s", replay)
	}
}

func TestRuntimeServerCompleteStepRejectsMismatchedTodoIndexBeforeEvidence(t *testing.T) {
	dataDir := t.TempDir()
	stepArgs := string(mustJSON(t, map[string]any{
		"step":       "Wrong contract",
		"step_index": 1,
		"evidence": []map[string]any{{
			"kind":    "verification",
			"summary": "runtime contract verification ran",
			"command": "printf verified",
		}},
	}))
	bashArgs := `{"command":"printf verified"}`
	provider := newCompleteProviderServer(t, [][]string{
		{
			fmt.Sprintf(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_bash_verify","type":"function","function":{"name":"bash","arguments":%q}},{"index":1,"id":"call_complete_step","type":"function","function":{"name":"complete_step","arguments":%q}}]},"finish_reason":"tool_calls"}]}`, bashArgs, stepArgs),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"todo still active"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "goal-provider", "goal-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Goal todo mismatch",
		"workspace":  dataDir,
		"providerId": "goal-provider",
		"model":      "goal-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/goal", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"objective": "Complete only matching todos",
	}), http.StatusOK)
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/todos", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"todos": []map[string]any{{"content": "Audit contract", "status": "in_progress"}},
	}), http.StatusOK)
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Verify but try to sign off the wrong todo text.",
		"approvalPolicy": "auto",
		"sandboxMode":    "danger-full-access",
	}), http.StatusAccepted)
	if provider.RequestCount() != 2 {
		t.Fatalf("goal todo mismatch loop should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	body := provider.Body(1)
	_, stepResult := providerHostToolResultForName(t, body, "complete_step")
	resultJSON := string(mustJSON(t, stepResult))
	if !strings.Contains(resultJSON, "todo_step_mismatch") {
		t.Fatalf("mismatched complete_step should return a host-bound tool error: result=%s body=%s", resultJSON, body)
	}
	goal := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID+"/goal", DefaultRuntimeToken, nil, http.StatusOK)
	ledger, _ := mapField(t, goal, "goal")["evidenceLedger"].([]any)
	if len(ledger) != 0 {
		t.Fatalf("mismatched complete_step must not append evidence before todo identity is validated: %#v", goal)
	}
	todos := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID+"/todos", DefaultRuntimeToken, nil, http.StatusOK)
	todoItems, _ := mapField(t, todos, "todos")["items"].([]any)
	if len(todoItems) != 1 {
		t.Fatalf("expected one todo item: %#v", todos)
	}
	todoItem, _ := todoItems[0].(map[string]any)
	if stringField(todoItem, "status") != "in_progress" {
		t.Fatalf("mismatched complete_step must leave the matched index unfinished: %#v", todos)
	}
}

func TestRuntimeServerPlanModeAdvertisesAndExecutesCreatePlan(t *testing.T) {
	const (
		reasoningSentinel = "DEEPSEEK_PRIVATE_PLAN_REASONING"
		planMarkdown      = "# Generated plan\n\nValidation nonce: 30"
		planContentHash   = "93887a2fccaf8b3b1b51350c0abfc65390966231128774816dce99bc362505e4"
	)
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"reasoning_content":"` + reasoningSentinel + `"}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_plan","type":"function","function":{"name":"create_plan","arguments":"{\"markdown\":\"# Generated plan\\n\\nValidation nonce: 30\",\"operation\":\"draft\",\"source_request\":\"Add auth\",\"title\":\"Auth\",\"plan_id\":\"plan-auth\",\"plan_relative_path\":\".analytixsdd/plan/auth.md\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"plan saved"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: durableRoot,
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        dataDir,
		ModelProvidersJSON: testModelProvidersJSONWithReasoningProtocol(
			provider.URL(), "plan-provider", "plan-model", "deepseek-chat-completions",
		),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Plan mode",
		"workspace":  workspace,
		"providerId": "plan-provider",
		"model":      "plan-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	guiPlan := map[string]any{
		"operation":     "draft",
		"workspaceRoot": workspace,
		"relativePath":  ".analytixsdd/plan/auth.md",
		"planId":        "plan-auth",
		"sourceRequest": "Add auth",
		"title":         "Auth",
	}
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Plan auth",
		"mode":           "plan",
		"guiPlan":        guiPlan,
		"approvalPolicy": "never",
		"sandboxMode":    "workspace-write",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")

	toolNames := providerRequestToolNames(t, provider.Body(0))
	if !containsString(toolNames, "create_plan") {
		t.Fatalf("plan turn must advertise create_plan: %#v body=%s", toolNames, provider.Body(0))
	}
	if provider.RequestCount() != 2 {
		t.Fatalf("create_plan loop should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	planPath := filepath.Join(workspace, ".analytixsdd", "plan", "auth.md")
	rawPlan, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("create_plan did not write reserved GUI plan: %v", err)
	}
	if string(rawPlan) != planMarkdown {
		t.Fatalf("create_plan wrote unexpected content: %q", string(rawPlan))
	}
	body := provider.Body(1)
	if !strings.Contains(body, `"reasoning_content":"`+reasoningSentinel+`"`) ||
		strings.Count(body, reasoningSentinel) != 1 {
		t.Fatalf("DeepSeek plan continuation lost exact private reasoning across the catalog transition: %s", body)
	}
	_, planResult := providerHostToolResultForName(t, body, "create_plan")
	resultJSON := string(mustJSON(t, planResult))
	if !strings.Contains(resultJSON, "relative_path") ||
		!strings.Contains(resultJSON, ".analytixsdd/plan/auth.md") {
		t.Fatalf("second provider call must include a host-bound structured create_plan result: result=%s body=%s", resultJSON, body)
	}
	threadState := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	turn := findRuntimeServerTurn(t, threadState, turnID)
	items, _ := turn["items"].([]any)
	foundResult := false
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item["kind"] != "tool_result" || item["toolName"] != "create_plan" {
			continue
		}
		foundResult = true
		if item["toolKind"] != "file_change" || item["isError"] == true {
			t.Fatalf("create_plan result should be a successful file_change item: %#v", item)
		}
		projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(item["output"])
		if err != nil || projection.ProjectionKind != domaintoolresult.ProjectionPlanStatus || projection.Plan == nil ||
			projection.Plan.RelativePath != ".analytixsdd/plan/auth.md" || projection.Plan.PlanID != "plan-auth" ||
			projection.Plan.ContentHash != planContentHash || projection.Plan.ByteSize != int64(len(planMarkdown)) {
			t.Fatalf("create_plan public output metadata mismatch: projection=%#v err=%v", projection, err)
		}
	}
	if !foundResult {
		t.Fatalf("turn missing create_plan tool_result: %#v", items)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if !strings.Contains(replay, planContentHash) {
		t.Fatal("create_plan digest was not byte-stable through the public SSE seam")
	}
	assertRuntimeFilesExcludeValues(t, dataDir, reasoningSentinel)
	assertRuntimeFilesExcludeValues(t, durableRoot, reasoningSentinel)
}

func TestMaterializedCreatePlanUsesExactBuiltinScopeWithLiveMCP(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	const reasoningSentinel = "DEEPSEEK_PRIVATE_HOST_MATERIALIZED_PLAN_REASONING"
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"reasoning_content":"` + reasoningSentinel + `"}}]}`,
			`data: {"choices":[{"delta":{"content":"## Plan\nUse the verified implementation path."}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"plan saved"}}]}`,
			`data: [DONE]`,
		},
	})
	mcpServer := newRuntimeServerToolListMCPServer(t, []map[string]any{{
		"name": "lookup", "description": "Lookup current documentation",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
	}})
	defer mcpServer.Close()
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: durableRoot,
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        dataDir,
		ModelProvidersJSON: testModelProvidersJSONWithReasoningProtocol(
			provider.URL(), "plan-live-mcp-provider", "plan-live-mcp-model", "deepseek-chat-completions",
		),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{"mcpServers": map[string]any{
			"docs": map[string]any{"transport": "http", "url": mcpServer.URL, "trustScope": "user"},
		}})),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Materialized plan with live MCP", "workspace": workspace,
		"providerId": "plan-live-mcp-provider", "model": "plan-live-mcp-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	guiPlan := map[string]any{
		"operation": "draft", "workspaceRoot": workspace, "relativePath": ".analytixsdd/plan/live-mcp.md",
		"planId": "plan-live-mcp", "sourceRequest": "Plan with a live MCP catalog", "title": "Live MCP plan",
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Plan with a live MCP catalog", "mode": "plan", "guiPlan": guiPlan,
		"approvalPolicy": "never", "sandboxMode": "workspace-write",
	}), http.StatusAccepted)
	if provider.RequestCount() != 2 {
		t.Fatalf("host-materialized create_plan should continue exactly once with a live MCP catalog: %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	if _, err := os.Stat(filepath.Join(workspace, ".analytixsdd", "plan", "live-mcp.md")); err != nil {
		t.Fatalf("exact create_plan scope failed in the presence of live MCP tools: %v", err)
	}
	secondBody := provider.Body(1)
	if !strings.Contains(secondBody, "Analytix host-selected prior tool activity") ||
		!strings.Contains(secondBody, ".analytixsdd/plan/live-mcp.md") ||
		strings.Contains(secondBody, reasoningSentinel) ||
		strings.Contains(secondBody, `"reasoning_content"`) ||
		strings.Contains(secondBody, `"tool_calls"`) ||
		strings.Contains(secondBody, `"tool_call_id"`) {
		t.Fatalf("host-materialized DeepSeek plan was not recompiled into safe semantic history: %s", secondBody)
	}
	assertRuntimeFilesExcludeValues(t, dataDir, reasoningSentinel)
	assertRuntimeFilesExcludeValues(t, durableRoot, reasoningSentinel)
}

func TestRuntimeServerImplicitAnthropicMaterializedPlanClosesCurrentAndRestartHistory(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	const (
		thinkingSentinel  = "ANTHROPIC_MATERIALIZED_PLAN_PRIVATE_THINKING"
		signatureSentinel = "sig_anthropic_materialized_plan"
	)
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":12}}}`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"` + thinkingSentinel + `"}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"` + signatureSentinel + `"}}`,
			`data: {"type":"content_block_stop","index":0}`,
			`data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"## Plan\nUse the verified Anthropic implementation path."}}`,
			`data: {"type":"content_block_stop","index":1}`,
			`data: {"type":"message_stop"}`,
		},
		{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":24}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Anthropic plan saved"}}`,
			`data: {"type":"message_stop"}`,
		},
		{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":28}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Anthropic plan history continued after restart"}}`,
			`data: {"type":"message_stop"}`,
		},
	})
	config := RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     durableRoot,
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSONWithEndpoint(provider.URL(), "anthropic-plan-provider", "claude-plan-model", "messages"),
	}
	firstHandler := newRuntimeServerProviderReadyTestHandler(t, config)
	firstServer := httptest.NewServer(firstHandler)
	thread := assertLiveJSON(t, firstServer.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Implicit Anthropic materialized plan", "workspace": workspace,
		"providerId": "anthropic-plan-provider", "model": "claude-plan-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	guiPlan := map[string]any{
		"operation": "draft", "workspaceRoot": workspace,
		"relativePath": ".analytixsdd/plan/anthropic-materialized.md",
		"planId":       "plan-anthropic-materialized", "sourceRequest": "Create the verified plan",
		"title": "Anthropic materialized plan",
	}
	assertLiveJSON(t, firstServer.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Create the verified plan", "mode": "plan", "guiPlan": guiPlan,
		"approvalPolicy": "never", "sandboxMode": "workspace-write",
	}), http.StatusAccepted)
	if provider.RequestCount() != 2 {
		t.Fatalf("implicit Anthropic materialized plan should continue once: %d bodies=%#v",
			provider.RequestCount(), provider.Bodies())
	}
	planPath := filepath.Join(workspace, ".analytixsdd", "plan", "anthropic-materialized.md")
	rawPlan, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("implicit Anthropic plan was not materialized: %v", err)
	}
	if !strings.Contains(string(rawPlan), "verified Anthropic implementation path") {
		t.Fatalf("materialized Anthropic plan content mismatch: %q", rawPlan)
	}
	currentBody := provider.Body(1)
	if !strings.Contains(currentBody, "Analytix host-selected prior tool activity") ||
		!strings.Contains(currentBody, ".analytixsdd/plan/anthropic-materialized.md") {
		t.Fatalf("current Anthropic materialized-plan continuation lost safe history: %s", currentBody)
	}
	for _, forbidden := range []string{
		thinkingSentinel,
		signatureSentinel,
		`"type":"thinking"`,
		`"type":"tool_use"`,
		`"type":"tool_result"`,
	} {
		if strings.Contains(currentBody, forbidden) {
			t.Fatalf("current Anthropic materialized-plan continuation exposed %q: %s", forbidden, currentBody)
		}
	}
	firstServer.Close()
	shutdownRuntimeTestHandler(t, firstHandler)

	restartedHandler := newRuntimeServerProviderReadyTestHandler(t, config)
	restarted := httptest.NewServer(restartedHandler)
	defer restarted.Close()
	assertLiveJSON(t, restarted.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Continue from the saved plan after restart.",
	}), http.StatusAccepted)
	if provider.RequestCount() != 3 {
		t.Fatalf("restarted materialized-plan thread should call provider once: %d bodies=%#v",
			provider.RequestCount(), provider.Bodies())
	}
	restartBody := provider.Body(2)
	if !strings.Contains(restartBody, "Analytix host-selected prior tool activity") ||
		!strings.Contains(restartBody, ".analytixsdd/plan/anthropic-materialized.md") {
		t.Fatalf("restarted Anthropic materialized-plan history was not safely rebuilt: %s", restartBody)
	}
	for _, forbidden := range []string{
		thinkingSentinel,
		signatureSentinel,
		`"type":"thinking"`,
		`"type":"tool_use"`,
		`"type":"tool_result"`,
	} {
		if strings.Contains(restartBody, forbidden) {
			t.Fatalf("restarted Anthropic materialized-plan history exposed %q: %s", forbidden, restartBody)
		}
	}
	replay := liveSSE(t, restarted.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	threadState := assertLiveJSON(t, restarted.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	encodedThread, _ := json.Marshal(threadState)
	for _, forbidden := range []string{thinkingSentinel, signatureSentinel, "privateProtocolObserved"} {
		if strings.Contains(replay, forbidden) || bytes.Contains(encodedThread, []byte(forbidden)) {
			t.Fatalf("materialized-plan public history exposed %q: replay=%s thread=%s",
				forbidden, replay, encodedThread)
		}
	}
	assertRuntimeFilesExcludeValues(t, dataDir, thinkingSentinel, signatureSentinel)
	assertRuntimeFilesExcludeValues(t, durableRoot, thinkingSentinel, signatureSentinel)
}

func TestUnsafeProviderPlanDraftIsNeverMaterializedOrPublished(t *testing.T) {
	const sentinel = "UNVERIFIED_PLAN_DRAFT_ACCOUNT_6222020202020202020_AMOUNT_4200000"
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{{
		`data: {"choices":[{"delta":{"content":"## Plan\n经查账户 6222020202020202020 涉案金额 420 万元。UNVERIFIED_PLAN_DRAFT_ACCOUNT_6222020202020202020_AMOUNT_4200000"}}]}`,
		`data: [DONE]`,
	}})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "unsafe-plan-provider", "unsafe-plan-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Unsafe provider plan containment", "workspace": workspace,
		"providerId": "unsafe-plan-provider", "model": "unsafe-plan-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	guiPlan := map[string]any{
		"operation": "draft", "workspaceRoot": workspace, "relativePath": ".analytixsdd/plan/unsafe.md",
		"planId": "plan-unsafe", "sourceRequest": "Draft an implementation plan", "title": "Unsafe plan",
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Draft an implementation plan", "mode": "plan", "guiPlan": guiPlan,
		"approvalPolicy": "never", "sandboxMode": "workspace-write",
	}), http.StatusAccepted)

	if provider.RequestCount() != 1 {
		t.Fatalf("blocked plan draft must stop without a recovery model call: %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	if _, err := os.Stat(filepath.Join(workspace, ".analytixsdd", "plan", "unsafe.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("blocked plan draft created a file: %v", err)
	}
	threadState := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	threadBody := string(mustJSON(t, threadState))
	if strings.Contains(threadBody, sentinel) || strings.Contains(threadBody, "6222020202020202020") || strings.Contains(threadBody, `"toolName":"create_plan"`) {
		t.Fatalf("blocked plan draft reached public thread state: %s", threadBody)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if strings.Contains(replay, sentinel) || strings.Contains(replay, "6222020202020202020") || strings.Contains(replay, `"toolName":"create_plan"`) {
		t.Fatalf("blocked plan draft reached public SSE replay:\n%s", replay)
	}
}

func TestRuntimeServerPlanModeUsesSelectedXiaomiProviderAndModel(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	deepseekProvider := newCompleteProviderServer(t, [][]string{{
		`data: {"choices":[{"delta":{"content":"wrong provider"}}]}`,
		`data: [DONE]`,
	}})
	xiaomiProvider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_mimo_plan","type":"function","function":{"name":"create_plan","arguments":"{\"markdown\":\"# MiMo plan\",\"operation\":\"draft\",\"source_request\":\"Use MiMo\",\"title\":\"MiMo\",\"plan_id\":\"plan-mimo\",\"plan_relative_path\":\".analytixsdd/plan/mimo.md\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"mimo plan saved"}}]}`,
			`data: [DONE]`,
		},
	})
	modelProviders := string(mustJSONNoTest(map[string]any{
		"defaultProviderId": "deepseek",
		"providers": []map[string]any{
			{
				"id":             "deepseek",
				"apiKey":         "test-deepseek-key",
				"baseUrl":        deepseekProvider.URL() + "/v1",
				"endpointFormat": "chat_completions",
				"models":         []string{"deepseek-chat"},
			},
			{
				"id":             "xiaomi",
				"apiKey":         "test-xiaomi-key",
				"baseUrl":        xiaomiProvider.URL() + "/v1",
				"endpointFormat": "chat_completions",
				"models":         []string{"mimo-v2.5-pro", "mimo-v2.5"},
			},
		},
	}))
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: modelProviders,
	}))
	defer server.Close()
	selectRuntimeServerFixtureProvider(t, server.URL, DefaultRuntimeToken, "xiaomi")

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "MiMo plan mode",
		"workspace":  workspace,
		"providerId": "xiaomi",
		"model":      "mimo-v2.5-pro",
		"mode":       "plan",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	guiPlan := map[string]any{
		"operation":     "draft",
		"workspaceRoot": workspace,
		"relativePath":  ".analytixsdd/plan/mimo.md",
		"planId":        "plan-mimo",
		"sourceRequest": "Use MiMo",
		"title":         "MiMo",
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Plan with MiMo.",
		"mode":           "plan",
		"providerId":     "xiaomi",
		"model":          "mimo-v2.5-pro",
		"guiPlan":        guiPlan,
		"approvalPolicy": "never",
		"sandboxMode":    "workspace-write",
	}), http.StatusAccepted)

	if deepseekProvider.RequestCount() != 0 {
		t.Fatalf("plan-mode MiMo turn must not fall back to DeepSeek, got %d deepseek requests: %#v", deepseekProvider.RequestCount(), deepseekProvider.Bodies())
	}
	if xiaomiProvider.RequestCount() != 2 {
		t.Fatalf("plan-mode MiMo loop should call Xiaomi twice, got %d bodies=%#v", xiaomiProvider.RequestCount(), xiaomiProvider.Bodies())
	}
	firstBody := xiaomiProvider.Body(0)
	if !strings.Contains(firstBody, `"model":"mimo-v2.5-pro"`) ||
		strings.Contains(firstBody, "deepseek") ||
		strings.Contains(firstBody, "mimo-v2.5-pro-ultraspeed") {
		t.Fatalf("Xiaomi plan request must preserve selected model without DeepSeek fallback or alias leakage:\n%s", firstBody)
	}
	if !strings.Contains(xiaomiProvider.Path(0), "/chat/completions") {
		t.Fatalf("Xiaomi endpointFormat=chat_completions should use chat completions path, got %q", xiaomiProvider.Path(0))
	}
	secondBody := xiaomiProvider.Body(1)
	_, planResult := providerHostToolResultForName(t, secondBody, "create_plan")
	resultJSON := string(mustJSON(t, planResult))
	if !strings.Contains(resultJSON, ".analytixsdd/plan/mimo.md") {
		t.Fatalf("second Xiaomi request must include host-bound create_plan history: result=%s body=%s", resultJSON, secondBody)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".analytixsdd", "plan", "mimo.md")); err != nil {
		t.Fatalf("MiMo plan-mode create_plan did not write plan: %v", err)
	}
}

func TestRuntimeServerPlanModeCanonicalizesSelectedXiaomiAliasModel(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	deepseekProvider := newCompleteProviderServer(t, [][]string{{
		`data: {"choices":[{"delta":{"content":"wrong provider"}}]}`,
		`data: [DONE]`,
	}})
	xiaomiProvider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_mimo_alias_plan","type":"function","function":{"name":"create_plan","arguments":"{\"markdown\":\"# MiMo alias plan\",\"operation\":\"draft\",\"source_request\":\"Use MiMo alias\",\"title\":\"MiMo alias\",\"plan_id\":\"plan-mimo-alias\",\"plan_relative_path\":\".analytixsdd/plan/mimo-alias.md\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"mimo alias plan saved"}}]}`,
			`data: [DONE]`,
		},
	})
	modelProviders := string(mustJSONNoTest(map[string]any{
		"defaultProviderId": "deepseek",
		"providers": []map[string]any{
			{
				"id":             "deepseek",
				"apiKey":         "test-deepseek-key",
				"baseUrl":        deepseekProvider.URL() + "/v1",
				"endpointFormat": "chat_completions",
				"models":         []string{"deepseek-chat"},
			},
			{
				"id":             "xiaomi",
				"apiKey":         "test-xiaomi-key",
				"baseUrl":        xiaomiProvider.URL() + "/v1",
				"endpointFormat": "chat_completions",
				"models":         []string{"mimo-v2.5-pro", "mimo-v2.5"},
				"modelProfiles": map[string]any{
					"mimo-v2.5-pro": map[string]any{
						"aliases": []string{"mimo-v2.5-pro-ultraspeed"},
						"reasoning": map[string]any{
							"requestProtocol": "mimo-chat-completions",
						},
					},
				},
			},
		},
	}))
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: modelProviders,
	}))
	defer server.Close()
	selectRuntimeServerFixtureProvider(t, server.URL, DefaultRuntimeToken, "xiaomi")

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "MiMo alias plan mode",
		"workspace":  workspace,
		"providerId": "xiaomi",
		"model":      "mimo-v2.5-pro-ultraspeed",
		"mode":       "plan",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	guiPlan := map[string]any{
		"operation":     "draft",
		"workspaceRoot": workspace,
		"relativePath":  ".analytixsdd/plan/mimo-alias.md",
		"planId":        "plan-mimo-alias",
		"sourceRequest": "Use MiMo alias",
		"title":         "MiMo alias",
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Plan with MiMo alias.",
		"mode":           "plan",
		"providerId":     "xiaomi",
		"model":          "mimo-v2.5-pro-ultraspeed",
		"guiPlan":        guiPlan,
		"approvalPolicy": "never",
		"sandboxMode":    "workspace-write",
	}), http.StatusAccepted)

	if deepseekProvider.RequestCount() != 0 {
		t.Fatalf("plan-mode MiMo alias turn must not fall back to DeepSeek, got %d deepseek requests: %#v", deepseekProvider.RequestCount(), deepseekProvider.Bodies())
	}
	if xiaomiProvider.RequestCount() != 2 {
		t.Fatalf("plan-mode MiMo alias loop should call Xiaomi twice, got %d bodies=%#v", xiaomiProvider.RequestCount(), xiaomiProvider.Bodies())
	}
	firstBody := xiaomiProvider.Body(0)
	if !strings.Contains(firstBody, `"model":"mimo-v2.5-pro"`) ||
		strings.Contains(firstBody, "deepseek") ||
		strings.Contains(firstBody, "mimo-v2.5-pro-ultraspeed") {
		t.Fatalf("Xiaomi plan request must canonicalize alias without DeepSeek fallback:\n%s", firstBody)
	}
	if !strings.Contains(xiaomiProvider.Path(0), "/chat/completions") {
		t.Fatalf("Xiaomi endpointFormat=chat_completions should use chat completions path, got %q", xiaomiProvider.Path(0))
	}
	secondBody := xiaomiProvider.Body(1)
	_, planResult := providerHostToolResultForName(t, secondBody, "create_plan")
	resultJSON := string(mustJSON(t, planResult))
	if !strings.Contains(resultJSON, ".analytixsdd/plan/mimo-alias.md") {
		t.Fatalf("second Xiaomi alias request must include host-bound create_plan history: result=%s body=%s", resultJSON, secondBody)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".analytixsdd", "plan", "mimo-alias.md")); err != nil {
		t.Fatalf("MiMo alias plan-mode create_plan did not write plan: %v", err)
	}
}

func TestRuntimeServerNormalTurnRejectsForgedCreatePlan(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_plan_forged","type":"function","function":{"name":"create_plan","arguments":"{\"markdown\":\"# Should not save\",\"operation\":\"draft\",\"plan_relative_path\":\".analytixsdd/plan/forged.md\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"rejected forged plan"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "normal-plan-provider", "normal-plan-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Normal turn forged create_plan",
		"workspace":  workspace,
		"providerId": "normal-plan-provider",
		"model":      "normal-plan-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	forgedResponse, forgedBody := liveRequest(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":      "Normal turn.",
		"sandboxMode": "workspace-write",
	}))
	if forgedResponse.StatusCode != http.StatusInternalServerError {
		t.Fatalf("unadvertised create_plan must fail closed: status=%d body=%s", forgedResponse.StatusCode, string(forgedBody))
	}
	var forgedError map[string]any
	if err := json.Unmarshal(forgedBody, &forgedError); err != nil {
		t.Fatalf("decode unadvertised create_plan rejection: %v", err)
	}
	assertClosedTurnFailureResponse(t, forgedError, "tool_not_advertised", "The provider requested a tool that was not advertised for this turn.")
	rawDetails, detailsOK := forgedError["details"].(map[string]any)
	if !detailsOK {
		t.Fatalf("unadvertised create_plan response omitted closed diagnostics: %s", forgedBody)
	}
	details := rawDetails
	rejectedNameHash := sha256.Sum256([]byte("create_plan"))
	if stringField(details, "rejectedToolNormalizedNameSha256") != hex.EncodeToString(rejectedNameHash[:]) ||
		stringField(details, "rejectedToolCategory") != "unknown_provider_name" ||
		stringField(details, "promptRoute") != "tool_agent" || details["loopStep"] != float64(0) ||
		boolField(details, "providerRequestRunToolStepManifestSame") != true {
		t.Fatalf("unadvertised create_plan diagnostic mismatch: %#v", details)
	}
	for _, key := range []string{
		"advertisedToolManifestHash", "advertisedNameSetSortedHash",
		"providerRequestToolManifestHash", "runToolStepManifestHash",
	} {
		if value := stringField(details, key); len(value) != 64 {
			t.Fatalf("unadvertised create_plan diagnostic hash %s is invalid: %#v", key, details)
		}
	}
	if stringField(details, "providerRequestToolManifestHash") != stringField(details, "runToolStepManifestHash") ||
		details["advertisedToolCount"].(float64) <= 0 || strings.Contains(string(forgedBody), `"toolName"`) ||
		strings.Contains(string(forgedBody), `"create_plan"`) {
		t.Fatalf("unadvertised create_plan response exposed raw identity or detached manifests: %s", forgedBody)
	}

	threadState := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	turns := anyList(threadState["turns"])
	if len(turns) != 1 {
		t.Fatalf("unadvertised create_plan durable turn count mismatch: %#v", threadState)
	}
	turn, _ := turns[0].(map[string]any)
	items := anyList(turn["items"])
	var errorItem map[string]any
	for _, rawItem := range items {
		item, _ := rawItem.(map[string]any)
		if stringField(item, "kind") == "error" {
			errorItem = item
			break
		}
	}
	if errorItem == nil || !reflect.DeepEqual(mapField(t, errorItem, "details"), details) {
		t.Fatalf("HTTP and durable unadvertised-tool diagnostics diverged: http=%#v durable=%#v", details, errorItem)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	var terminalDetails map[string]any
	for _, event := range parseRuntimeServerSSEEvents(t, replay) {
		if stringField(event, "kind") == "turn_failed" && stringField(event, "code") == "tool_not_advertised" {
			terminalDetails = mapField(t, event, "details")
		}
	}
	if !reflect.DeepEqual(terminalDetails, details) {
		t.Fatalf("HTTP, durable, and public SSE unadvertised-tool diagnostics diverged: http=%#v sse=%#v", details, terminalDetails)
	}

	if toolNames := providerRequestToolNames(t, provider.Body(0)); containsString(toolNames, "create_plan") {
		t.Fatalf("normal turn must not advertise create_plan: %#v", toolNames)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".analytixsdd", "plan", "forged.md")); !os.IsNotExist(err) {
		t.Fatalf("forged create_plan should not create a file, stat err=%v", err)
	}
	if provider.RequestCount() != 1 {
		t.Fatalf("forged create_plan must not trigger a recovery provider call, requests=%d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
}

func TestRuntimeServerPlanModeRejectsForgedWriteBeforeMaterializingPlanText(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_write","type":"function","function":{"name":"write","arguments":"{\"path\":\"forbidden.txt\",\"content\":\"should not exist\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"## Plan\nStay read-only until build mode."}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"plan saved"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "plan-forged-provider", "plan-forged-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Plan mode forged write",
		"workspace":  workspace,
		"providerId": "plan-forged-provider",
		"model":      "plan-forged-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	forgedResponse, forgedBody := liveRequest(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Plan a safe change",
		"mode":           "plan",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}))
	if forgedResponse.StatusCode != http.StatusInternalServerError {
		t.Fatalf("unadvertised plan-mode write must fail closed: status=%d body=%s", forgedResponse.StatusCode, string(forgedBody))
	}
	var forgedError map[string]any
	if err := json.Unmarshal(forgedBody, &forgedError); err != nil {
		t.Fatalf("decode unadvertised plan-mode write rejection: %v", err)
	}
	assertClosedTurnFailureResponse(t, forgedError, "tool_not_advertised", "The provider requested a tool that was not advertised for this turn.")

	firstTools := providerRequestToolNames(t, provider.Body(0))
	for _, hidden := range []string{"write", "edit", "edit_file", "multi_edit", "move_file", "notebook_edit", "delete_range", "delete_symbol", "bash", "task", "parallel_tasks"} {
		if containsString(firstTools, hidden) {
			t.Fatalf("plan investigation step must hide %s: %#v", hidden, firstTools)
		}
	}
	for _, expected := range []string{"read", "grep", "find", "glob", "code_index", "ls", "create_plan"} {
		if !containsString(firstTools, expected) {
			t.Fatalf("plan investigation step missing %s: %#v", expected, firstTools)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, "forbidden.txt")); !os.IsNotExist(err) {
		t.Fatalf("forged write should not create workspace file, stat err=%v", err)
	}
	planPath := filepath.Join(workspace, ".analytixsdd", "plan", "plan-a-safe-change.md")
	if _, err := os.Stat(planPath); !os.IsNotExist(err) {
		t.Fatalf("fail-closed forged write must not materialize a later plan, stat err=%v", err)
	}
	if provider.RequestCount() != 1 {
		t.Fatalf("forged write must not trigger recovery or materialization provider calls, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
}

func TestRuntimeServerOpenAIResponsesToolLoopExecutesAndContinues(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "alpha.txt"), []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"fc_ls","type":"function_call","call_id":"call_resp_ls","name":"ls","arguments":""}}`,
			`data: {"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_ls","delta":"{\"path\":\".\"}"}`,
			`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"fc_ls","type":"function_call","call_id":"call_resp_ls","name":"ls","arguments":""}}`,
			`data: [DONE]`,
		},
		{
			`data: {"type":"response.output_text.delta","delta":"responses listed"}`,
			`data: {"type":"response.completed","response":{"usage":{"input_tokens":20,"output_tokens":2,"total_tokens":22}}}`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSONWithEndpoint(provider.URL(), "responses-tools-provider", "responses-tool-model", "responses"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Responses tools",
		"workspace":  workspace,
		"providerId": "responses-tools-provider",
		"model":      "responses-tool-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "List via responses.",
	}), http.StatusAccepted)

	if provider.RequestCount() != 2 {
		t.Fatalf("responses tool loop should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	body := provider.Body(1)
	callID, output := providerResponsesHostToolResultForName(t, body, "ls")
	if !strings.Contains(output, "alpha.txt") || strings.Contains(body, "call_resp_ls") || !domainsecurity.IsHostToolCallIDV1(callID) {
		t.Fatalf("second Responses provider call must include paired host-bound function_call_output history: callID=%s output=%s body=%s", callID, output, body)
	}
}

func TestRuntimeServerAnthropicMessagesToolLoopExecutesAndContinues(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "alpha.txt"), []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	const (
		firstSignatureSentinel  = "sig_anthropic_signature_only_public_seam"
		secondThinkingSentinel  = "ANTHROPIC_SECOND_PRIVATE_THINKING_PUBLIC_SEAM"
		secondSignatureSentinel = "sig_anthropic_second_public_seam"
	)
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":8}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"` + firstSignatureSentinel + `"}}`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_ls","name":"ls","input":{}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\".\"}"}}`,
			`data: {"type":"content_block_stop","index":0}`,
			`data: {"type":"message_stop"}`,
		},
		{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":20}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"` + secondThinkingSentinel + `"}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"` + secondSignatureSentinel + `"}}`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_read","name":"read","input":{}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"alpha.txt\"}"}}`,
			`data: {"type":"content_block_stop","index":0}`,
			`data: {"type":"message_stop"}`,
		},
		{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":32}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"anthropic inspected both results"}}`,
			`data: {"type":"message_delta","usage":{"output_tokens":4}}`,
			`data: {"type":"message_stop"}`,
		},
		{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":40}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"anthropic continued from closed history"}}`,
			`data: {"type":"message_delta","usage":{"output_tokens":5}}`,
			`data: {"type":"message_stop"}`,
		},
		{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":44}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"anthropic fork continued without private wire"}}`,
			`data: {"type":"message_delta","usage":{"output_tokens":5}}`,
			`data: {"type":"message_stop"}`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     durableRoot,
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSONWithEndpoint(provider.URL(), "anthropic-tools-provider", "claude-tool-model", "messages"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Anthropic tools",
		"workspace":  workspace,
		"providerId": "anthropic-tools-provider",
		"model":      "claude-tool-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "List via anthropic messages.",
	}), http.StatusAccepted)

	if provider.RequestCount() != 3 {
		t.Fatalf("anthropic two-tool loop should call provider three times, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	var secondBody map[string]any
	if err := json.Unmarshal([]byte(provider.Body(1)), &secondBody); err != nil {
		t.Fatalf("second anthropic provider call body should be JSON: %v\n%s", err, provider.Body(1))
	}
	messages, ok := secondBody["messages"].([]any)
	if !ok || len(messages) == 0 {
		t.Fatalf("second anthropic provider call must include messages: %#v", secondBody)
	}
	hasSignedThinking := false
	for _, rawMessage := range messages {
		message, ok := rawMessage.(map[string]any)
		if !ok {
			t.Fatalf("anthropic message should be an object: %#v", rawMessage)
		}
		role, _ := message["role"].(string)
		if role == "tool" {
			t.Fatalf("Anthropic messages must not emit top-level role=tool: %#v", message)
		}
		content, ok := message["content"].([]any)
		if !ok {
			t.Fatalf("anthropic message content should be an array: %#v", message)
		}
		for _, rawBlock := range content {
			block, ok := rawBlock.(map[string]any)
			if !ok {
				t.Fatalf("anthropic content block should be an object: %#v", rawBlock)
			}
			switch block["type"] {
			case "tool_result":
				if role != "user" {
					t.Fatalf("tool_result must be nested under a user message: %#v", message)
				}
			case "thinking":
				if role == "assistant" && block["thinking"] == "" && block["signature"] == firstSignatureSentinel {
					hasSignedThinking = true
				}
			}
		}
	}
	hostCallID, toolOutput := providerAnthropicHostToolResultForName(t, provider.Body(1), "ls")
	if !strings.Contains(toolOutput, "alpha.txt") || strings.Contains(provider.Body(1), "toolu_ls") || !domainsecurity.IsHostToolCallIDV1(hostCallID) {
		t.Fatalf("Anthropic continuation did not use an exact host-resigned tool pair: callID=%q output=%q body=%s", hostCallID, toolOutput, provider.Body(1))
	}
	if !hasSignedThinking {
		t.Fatalf("second anthropic provider call must include tool_result history and signature-only thinking:\n%#v", secondBody)
	}
	thirdBody := provider.Body(2)
	lsCallID, lsOutput := providerAnthropicHostToolResultForName(t, thirdBody, "ls")
	readCallID, readOutput := providerAnthropicHostToolResultForName(t, thirdBody, "read")
	if !strings.Contains(lsOutput, "alpha.txt") || !strings.Contains(readOutput, "alpha") ||
		!domainsecurity.IsHostToolCallIDV1(lsCallID) || !domainsecurity.IsHostToolCallIDV1(readCallID) ||
		strings.Contains(thirdBody, "toolu_ls") || strings.Contains(thirdBody, "toolu_read") {
		t.Fatalf("third Anthropic provider call did not preserve both host-resigned tool pairs: ls=%q/%q read=%q/%q body=%s",
			lsCallID, lsOutput, readCallID, readOutput, thirdBody)
	}
	for _, expected := range []string{
		`"thinking":""`,
		`"signature":"` + firstSignatureSentinel + `"`,
		`"thinking":"` + secondThinkingSentinel + `"`,
		`"signature":"` + secondSignatureSentinel + `"`,
	} {
		if !strings.Contains(thirdBody, expected) {
			t.Fatalf("third Anthropic provider call did not replay both response-derived thinking blocks (%s): %s", expected, thirdBody)
		}
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Continue in a new turn.",
	}), http.StatusAccepted)
	if provider.RequestCount() != 4 {
		t.Fatalf("new Anthropic turn should make one provider call, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	durableHistoryBody := provider.Body(3)
	if !strings.Contains(durableHistoryBody, "Analytix host-selected prior tool activity") {
		t.Fatalf("new Anthropic turn lost closed semantic tool history: %s", durableHistoryBody)
	}
	for _, forbidden := range []string{
		firstSignatureSentinel,
		secondThinkingSentinel,
		secondSignatureSentinel,
		`"type":"thinking"`,
		`"type":"tool_use"`,
		`"type":"tool_result"`,
	} {
		if strings.Contains(durableHistoryBody, forbidden) {
			t.Fatalf("new Anthropic turn replayed attempt-private history %q: %s", forbidden, durableHistoryBody)
		}
	}
	privateValues := []string{
		firstSignatureSentinel,
		secondThinkingSentinel,
		secondSignatureSentinel,
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	threadState := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	encodedThread, _ := json.Marshal(threadState)
	if strings.Contains(replay, "privateProtocolObserved") || bytes.Contains(encodedThread, []byte("privateProtocolObserved")) {
		t.Fatalf("host-only private protocol provenance entered public history: replay=%s thread=%s", replay, encodedThread)
	}
	for _, privateValue := range privateValues {
		if strings.Contains(replay, privateValue) {
			t.Fatalf("Anthropic private protocol bytes entered SSE replay: %s", replay)
		}
		if bytes.Contains(encodedThread, []byte(privateValue)) {
			t.Fatalf("Anthropic private protocol bytes entered thread history: %s", encodedThread)
		}
	}
	assertRuntimeFilesExcludeValues(t, dataDir, privateValues...)
	assertRuntimeFilesExcludeValues(t, durableRoot, privateValues...)
	fork := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/fork", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"relation": "fork",
		"title":    "Anthropic private protocol fork",
	}), http.StatusCreated)
	forkID := stringField(fork, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+forkID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Continue after the fork.",
	}), http.StatusAccepted)
	if provider.RequestCount() != 5 {
		t.Fatalf("forked Anthropic thread should make one provider call, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	forkBody := provider.Body(4)
	for _, forbidden := range []string{
		firstSignatureSentinel,
		secondThinkingSentinel,
		secondSignatureSentinel,
		`"type":"thinking"`,
		`"type":"tool_use"`,
		`"type":"tool_result"`,
	} {
		if strings.Contains(forkBody, forbidden) {
			t.Fatalf("forked Anthropic thread replayed attempt-private wire %q: %s", forbidden, forkBody)
		}
	}
}

func TestRuntimeServerOrdinaryAnthropicHistoryKeepsNativeToolWire(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "ordinary.txt"), []byte("ordinary"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":8}}}`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_ordinary_ls","name":"ls","input":{}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\".\"}"}}`,
			`data: {"type":"content_block_stop","index":0}`,
			`data: {"type":"message_stop"}`,
		},
		{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":20}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ordinary Anthropic tool turn complete"}}`,
			`data: {"type":"message_stop"}`,
		},
		{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":24}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ordinary Anthropic history continued"}}`,
			`data: {"type":"message_stop"}`,
		},
		{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":28}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ordinary Anthropic fork continued"}}`,
			`data: {"type":"message_stop"}`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     durableRoot,
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSONWithEndpoint(provider.URL(), "anthropic-ordinary-provider", "claude-ordinary-model", "messages"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Ordinary Anthropic tools",
		"workspace":  workspace,
		"providerId": "anthropic-ordinary-provider",
		"model":      "claude-ordinary-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "List via ordinary Anthropic messages.",
	}), http.StatusAccepted)
	if provider.RequestCount() != 2 {
		t.Fatalf("ordinary Anthropic tool turn should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	firstContinuationCallID, firstContinuationOutput := providerAnthropicHostToolResultForName(t, provider.Body(1), "ls")
	if !domainsecurity.IsHostToolCallIDV1(firstContinuationCallID) || !strings.Contains(firstContinuationOutput, "ordinary.txt") {
		t.Fatalf("ordinary Anthropic same-attempt continuation lost exact tool output: callID=%q output=%q body=%s",
			firstContinuationCallID, firstContinuationOutput, provider.Body(1))
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Continue from the ordinary tool history.",
	}), http.StatusAccepted)
	if provider.RequestCount() != 3 {
		t.Fatalf("ordinary Anthropic next turn should call provider once, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	nextTurnBody := provider.Body(2)
	callID, output := providerAnthropicHostToolResultForName(t, nextTurnBody, "ls")
	if !domainsecurity.IsHostToolCallIDV1(callID) || !strings.Contains(output, `"messageKey":"tool_completed"`) ||
		!strings.Contains(nextTurnBody, `"type":"tool_use"`) ||
		!strings.Contains(nextTurnBody, `"type":"tool_result"`) ||
		strings.Contains(nextTurnBody, "Analytix host-selected prior tool activity") {
		t.Fatalf("ordinary blank/default Anthropic history was not retained as native tool wire: callID=%q output=%q body=%s",
			callID, output, nextTurnBody)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	threadState := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	encodedThread, _ := json.Marshal(threadState)
	if strings.Contains(replay, "privateProtocolObserved") || bytes.Contains(encodedThread, []byte("privateProtocolObserved")) {
		t.Fatalf("host-only private protocol provenance entered ordinary public history: replay=%s thread=%s", replay, encodedThread)
	}
	assertRuntimeFilesExcludeValues(t, dataDir, "privateProtocolObserved")
	assertRuntimeFilesExcludeValues(t, durableRoot, "privateProtocolObserved")
	fork := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/fork", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"relation": "fork",
		"title":    "Ordinary Anthropic fork",
	}), http.StatusCreated)
	forkID := stringField(fork, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+forkID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Continue ordinary history after the fork.",
	}), http.StatusAccepted)
	if provider.RequestCount() != 4 {
		t.Fatalf("forked ordinary Anthropic thread should make one provider call, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	forkBody := provider.Body(3)
	forkCallID, forkOutput := providerAnthropicHostToolResultForName(t, forkBody, "ls")
	if !domainsecurity.IsHostToolCallIDV1(forkCallID) || !strings.Contains(forkOutput, `"messageKey":"tool_completed"`) ||
		!strings.Contains(forkBody, `"type":"tool_use"`) ||
		!strings.Contains(forkBody, `"type":"tool_result"`) ||
		strings.Contains(forkBody, "Analytix host-selected prior tool activity") {
		t.Fatalf("ordinary Anthropic fork lost native tool history: callID=%q output=%q body=%s",
			forkCallID, forkOutput, forkBody)
	}
}

func TestRuntimeServerCompletedAnthropicPrivateToolTurnRestartsWithClosedHistory(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "restart.txt"), []byte("restart"), 0o644); err != nil {
		t.Fatal(err)
	}
	const thinkingSentinel = "ANTHROPIC_COMPLETED_RESTART_PRIVATE_THINKING"
	const signatureSentinel = "sig_anthropic_completed_restart"
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":8}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"` + thinkingSentinel + `"}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"` + signatureSentinel + `"}}`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_restart_ls","name":"ls","input":{}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\".\"}"}}`,
			`data: {"type":"content_block_stop","index":0}`,
			`data: {"type":"message_stop"}`,
		},
		{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":20}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"completed before restart"}}`,
			`data: {"type":"message_stop"}`,
		},
		{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":24}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"continued after restart"}}`,
			`data: {"type":"message_stop"}`,
		},
	})
	config := RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     durableRoot,
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSONWithEndpoint(provider.URL(), "anthropic-restart-provider", "claude-restart-model", "messages"),
	}
	firstHandler := newRuntimeServerProviderReadyTestHandler(t, config)
	firstServer := httptest.NewServer(firstHandler)
	thread := assertLiveJSON(t, firstServer.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Anthropic completed restart",
		"workspace":  workspace,
		"providerId": "anthropic-restart-provider",
		"model":      "claude-restart-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, firstServer.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "List before restart.",
	}), http.StatusAccepted)
	if provider.RequestCount() != 2 {
		t.Fatalf("completed private Anthropic turn should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	firstServer.Close()
	shutdownRuntimeTestHandler(t, firstHandler)

	restartedHandler := newRuntimeServerProviderReadyTestHandler(t, config)
	restarted := httptest.NewServer(restartedHandler)
	defer restarted.Close()
	assertLiveJSON(t, restarted.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Continue after process restart.",
	}), http.StatusAccepted)
	if provider.RequestCount() != 3 {
		t.Fatalf("restarted completed Anthropic thread should call provider once, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	restartBody := provider.Body(2)
	if !strings.Contains(restartBody, "Analytix host-selected prior tool activity") {
		t.Fatalf("completed Anthropic restart lost closed semantic history: %s", restartBody)
	}
	for _, forbidden := range []string{
		thinkingSentinel,
		signatureSentinel,
		`"type":"thinking"`,
		`"type":"tool_use"`,
		`"type":"tool_result"`,
	} {
		if strings.Contains(restartBody, forbidden) {
			t.Fatalf("completed Anthropic restart replayed attempt-private wire %q: %s", forbidden, restartBody)
		}
	}
	replay := liveSSE(t, restarted.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	threadState := assertLiveJSON(t, restarted.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	encodedThread, _ := json.Marshal(threadState)
	for _, forbidden := range []string{thinkingSentinel, signatureSentinel, "privateProtocolObserved"} {
		if strings.Contains(replay, forbidden) || bytes.Contains(encodedThread, []byte(forbidden)) {
			t.Fatalf("completed Anthropic restart exposed private state %q: replay=%s thread=%s", forbidden, replay, encodedThread)
		}
	}
	assertRuntimeFilesExcludeValues(t, dataDir, thinkingSentinel, signatureSentinel)
	assertRuntimeFilesExcludeValues(t, durableRoot, thinkingSentinel, signatureSentinel)
}

func TestRuntimeServerAnthropicApprovalUsesSafeSemanticHistory(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "approval.txt"), []byte("approved"), 0o644); err != nil {
		t.Fatal(err)
	}
	const thinkingSentinel = "ANTHROPIC_VOLATILE_THINKING_SENTINEL"
	const signatureSentinel = "sig_anthropic_volatile_1"
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":8}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"` + thinkingSentinel + `"}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"` + signatureSentinel + `"}}`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_approval_ls","name":"ls","input":{}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\".\"}"}}`,
			`data: {"type":"content_block_stop","index":0}`,
			`data: {"type":"message_stop"}`,
		},
		{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":20}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"approval continuation complete"}}`,
			`data: {"type":"message_stop"}`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), Host: "127.0.0.1", Port: 0, DataDir: dataDir,
		ModelProvidersJSON: testModelProvidersJSONWithEndpoint(provider.URL(), "anthropic-approval-provider", "claude-approval-model", "messages"),
	}))
	defer server.Close()
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Anthropic approval", "workspace": workspace, "providerId": "anthropic-approval-provider", "model": "claude-approval-model",
		"approvalPolicy": "always", "sandboxMode": "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "List only after approval.",
	}), http.StatusAccepted)
	pendingID := stringField(start, "pendingId")
	if start["status"] != "waiting" || start["pendingKind"] != "approval" || pendingID == "" || provider.RequestCount() != 1 {
		t.Fatalf("Anthropic approval did not pause with one provider call: start=%#v calls=%d", start, provider.RequestCount())
	}
	assertRuntimeFilesExcludeValues(t, dataDir, thinkingSentinel, signatureSentinel)
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/approvals/"+pendingID, DefaultRuntimeToken, mustJSON(t, map[string]string{"decision": "allow"}), http.StatusOK)
	if provider.RequestCount() != 2 {
		t.Fatalf("approved Anthropic tool did not continue exactly once: %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	secondBody := provider.Body(1)
	for _, expected := range []string{
		"Analytix host-selected prior tool activity",
		`\"tool\":\"ls\"`,
		`\"status\":\"completed\"`,
		"approval.txt",
	} {
		if !strings.Contains(secondBody, expected) {
			t.Fatalf("approved Anthropic continuation missing safe semantic history %q: %s", expected, secondBody)
		}
	}
	for _, forbidden := range []string{
		thinkingSentinel,
		signatureSentinel,
		`"type":"thinking"`,
		`"type":"tool_use"`,
		`"type":"tool_result"`,
	} {
		if strings.Contains(secondBody, forbidden) {
			t.Fatalf("approved Anthropic continuation replayed pause-private wire %q: %s", forbidden, secondBody)
		}
	}
	assertRuntimeFilesExcludeValues(t, dataDir, thinkingSentinel, signatureSentinel)
}

func TestRuntimeServerAnthropicMultiToolApprovalResumesWithSafeSemanticHistory(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "approval.txt"), []byte("approved"), 0o644); err != nil {
		t.Fatal(err)
	}
	const (
		thinkingSentinel  = "ANTHROPIC_PRIVATE_MULTI_TOOL_APPROVAL_THINKING"
		signatureSentinel = "sig_anthropic_multi_tool_approval"
	)
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":8}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"` + thinkingSentinel + `"}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"` + signatureSentinel + `"}}`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_approval_ls","name":"ls","input":{}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\".\"}"}}`,
			`data: {"type":"content_block_stop","index":0}`,
			`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_approval_read","name":"read","input":{}}}`,
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"approval.txt\"}"}}`,
			`data: {"type":"content_block_stop","index":1}`,
			`data: {"type":"message_stop"}`,
		},
		{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":20}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"approval continuation complete"}}`,
			`data: {"type":"message_stop"}`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, Host: "127.0.0.1", Port: 0, DataDir: dataDir,
		ModelProvidersJSON: testModelProvidersJSONWithEndpoint(provider.URL(), "anthropic-multi-approval-provider", "claude-multi-approval-model", "messages"),
	}))
	defer server.Close()
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Anthropic multi-tool approval", "workspace": workspace,
		"providerId": "anthropic-multi-approval-provider", "model": "claude-multi-approval-model",
		"approvalPolicy": "always", "sandboxMode": "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "List and read only after approval.",
	}), http.StatusAccepted)
	pendingID := stringField(start, "pendingId")
	if start["status"] != "waiting" || start["pendingKind"] != "approval" ||
		pendingID == "" || provider.RequestCount() != 1 {
		t.Fatalf("Anthropic multi-tool response did not pause at the first approval: start=%#v calls=%d", start, provider.RequestCount())
	}
	assertRuntimeFilesExcludeValues(t, dataDir, thinkingSentinel, signatureSentinel)
	assertRuntimeFilesExcludeValues(t, durableRoot, thinkingSentinel, signatureSentinel)

	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/approvals/"+pendingID, DefaultRuntimeToken, mustJSON(t, map[string]string{"decision": "allow"}), http.StatusOK)
	if provider.RequestCount() != 2 {
		t.Fatalf("approved Anthropic multi-tool response did not resume exactly once: %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	secondBody := provider.Body(1)
	for _, expected := range []string{
		"Analytix host-selected prior tool activity",
		`\"tool\":\"ls\"`,
		`\"status\":\"completed\"`,
		"approval.txt",
		`\"tool\":\"read\"`,
		`\"status\":\"result_unavailable\"`,
	} {
		if !strings.Contains(secondBody, expected) {
			t.Fatalf("Anthropic approval continuation missing %q: %s", expected, secondBody)
		}
	}
	for _, forbidden := range []string{
		thinkingSentinel,
		signatureSentinel,
		`"type":"tool_use"`,
		`"type":"tool_result"`,
	} {
		if strings.Contains(secondBody, forbidden) {
			t.Fatalf("Anthropic approval continuation retained private or incomplete wire protocol %q: %s", forbidden, secondBody)
		}
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if strings.Contains(replay, thinkingSentinel) || strings.Contains(replay, signatureSentinel) {
		t.Fatalf("Anthropic multi-tool approval leaked private protocol bytes into SSE: %s", replay)
	}
	threadState := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	encodedThread, _ := json.Marshal(threadState)
	if bytes.Contains(encodedThread, []byte(thinkingSentinel)) || bytes.Contains(encodedThread, []byte(signatureSentinel)) {
		t.Fatalf("Anthropic multi-tool approval leaked private protocol bytes into thread history: %s", encodedThread)
	}
	assertRuntimeFilesExcludeValues(t, dataDir, thinkingSentinel, signatureSentinel)
	assertRuntimeFilesExcludeValues(t, durableRoot, thinkingSentinel, signatureSentinel)
}

func TestRuntimeServerAnthropicApprovalDenialClearsPrivateProtocolWithoutProvider(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	const thinkingSentinel = "ANTHROPIC_DENIED_THINKING_SENTINEL"
	const signatureSentinel = "sig_anthropic_denied_1"
	provider := newCompleteProviderServer(t, [][]string{{
		`data: {"type":"message_start","message":{"usage":{"input_tokens":8}}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"` + thinkingSentinel + `"}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"` + signatureSentinel + `"}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_denied_ls","name":"ls","input":{}}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\".\"}"}}`,
		`data: {"type":"content_block_stop","index":0}`,
		`data: {"type":"message_stop"}`,
	}})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), Host: "127.0.0.1", Port: 0, DataDir: dataDir,
		ModelProvidersJSON: testModelProvidersJSONWithEndpoint(provider.URL(), "anthropic-deny-provider", "claude-deny-model", "messages"),
	}))
	defer server.Close()
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Anthropic deny", "workspace": workspace, "providerId": "anthropic-deny-provider", "model": "claude-deny-model",
		"approvalPolicy": "always", "sandboxMode": "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "List only after approval.",
	}), http.StatusAccepted)
	pendingID := stringField(start, "pendingId")
	if pendingID == "" || provider.RequestCount() != 1 {
		t.Fatalf("Anthropic denial fixture did not pause: start=%#v calls=%d", start, provider.RequestCount())
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/approvals/"+pendingID, DefaultRuntimeToken, mustJSON(t, map[string]string{"decision": "deny"}), http.StatusOK)
	if provider.RequestCount() != 1 {
		t.Fatalf("approval denial reused private protocol state in another provider call: %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if !strings.Contains(replay, "The requested operation was denied; no unverified final response was published.") ||
		strings.Contains(replay, thinkingSentinel) || strings.Contains(replay, signatureSentinel) {
		t.Fatalf("Anthropic denial boundary/reasoning projection mismatch: %s", replay)
	}
	assertRuntimeFilesExcludeValues(t, dataDir, thinkingSentinel, signatureSentinel)
}

func assertRuntimeFilesExcludeValues(t *testing.T, root string, values ...string) {
	t.Helper()
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		payload, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, value := range values {
			if bytes.Contains(payload, []byte(value)) {
				t.Fatalf("private protocol value entered durable file %s", path)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeServerConfiguredHTTPMCPToolLoopRejectsMutationWithoutHostSemanticIdentityAndContinues(t *testing.T) {
	dataDir := t.TempDir()
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_mcp_lookup","type":"function","function":{"name":"mcp__runtime-mcp__lookup","arguments":"{\"query\":\"needle\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"mcp complete"}}]}`,
			`data: [DONE]`,
		},
	})
	var mu sync.Mutex
	capturedArgs := map[string]any{}
	toolCallCount := 0
	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode MCP request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": runtimeMCPInitializeResult("runtime-mcp")})
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"tools": []map[string]any{{
				"name":         "lookup",
				"description":  "Lookup runtime MCP data",
				"inputSchema":  map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}},
				"outputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
			}}}})
		case "tools/call":
			args, _ := request.Params["arguments"].(map[string]any)
			mu.Lock()
			capturedArgs = args
			toolCallCount++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"content": []map[string]any{{
				"type": "text",
				"text": "mcp result: " + fmt.Sprint(args["query"]),
			}}, "structuredContent": map[string]any{}}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
	defer mcpServer.Close()
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "mcp-loop-provider", "mcp-loop-model"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"mcpServers": map[string]any{
				"runtime-mcp": map[string]any{
					"transport":  "http",
					"url":        mcpServer.URL,
					"trustScope": "user",
				},
			},
		})),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "MCP loop",
		"workspace":      dataDir,
		"providerId":     "mcp-loop-provider",
		"model":          "mcp-loop-model",
		"approvalPolicy": "auto",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Call configured MCP.",
	}), http.StatusAccepted)

	if provider.RequestCount() != 2 {
		t.Fatalf("MCP tool loop should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	mcpSchema := providerRequestToolParameters(t, provider.Body(0), "mcp__runtime-mcp__lookup")
	properties := mapField(t, mcpSchema, "properties")
	if _, ok := properties["query"]; !ok || mcpSchema["additionalProperties"] == true {
		t.Fatalf("first provider request should advertise the real MCP input schema, got %#v\nbody:\n%s", mcpSchema, provider.Body(0))
	}
	mu.Lock()
	query := fmt.Sprint(capturedArgs["query"])
	calls := toolCallCount
	mu.Unlock()
	if calls != 0 || query != "<nil>" {
		t.Fatalf("MCP transport executed without a host semantic identity: calls=%d args=%#v", calls, capturedArgs)
	}
	body := provider.Body(1)
	_, mcpResult := providerHostToolResultForName(t, body, "mcp__runtime-mcp__lookup")
	resultJSON := string(mustJSON(t, mcpResult))
	if !strings.Contains(resultJSON, "side_effect_identity_unavailable") || strings.Contains(resultJSON, "mcp result: needle") {
		t.Fatalf("second provider call must contain the host rejection and no fabricated MCP result: result=%s body=%s", resultJSON, body)
	}
}

func TestRuntimeServerCaseFundPromptScopesProviderToolsToAnalytixFunds(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "case-workspace")
	if err := os.MkdirAll(filepath.Join(workspace, ".analytix"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeRuntimeCaseProjectBinding(t, workspace, "38dc50241996")
	if err := os.WriteFile(filepath.Join(workspace, "old-report.md"), []byte("old report must not be a case fact source"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{{
		`data: {"choices":[{"delta":{"content":"收到"}}]}`,
		`data: [DONE]`,
	}})
	analytixFunds := newPinnedFundsMCPFixture(t, []map[string]any{
		{
			"name":        "count_case_rows",
			"description": "Count current-case rows from the audited Analytix fund-analysis database.",
			"inputSchema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"table": map[string]any{"type": "string"},
				},
			},
		},
		{
			"name":        "rank_holders",
			"description": "Rank current-case holders by audited turnover, inflow, or outflow.",
			"inputSchema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"metric":          map[string]any{"type": "string"},
					"requested_limit": map[string]any{"type": "integer"},
				},
			},
		},
	}, nil)
	memoryServer := newRuntimeServerToolListMCPServer(t, []map[string]any{{
		"name":        "read_graph",
		"description": "Read historical memory graph entries.",
		"inputSchema": map[string]any{"type": "object"},
	}})
	defer memoryServer.Close()
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "case-fund-provider", "case-fund-model"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"mcpServers": map[string]any{
				"analytix_funds": analytixFunds.config,
				"memory": map[string]any{
					"transport":  "http",
					"url":        memoryServer.URL,
					"trustScope": "user",
				},
			},
		})),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Case fund front door",
		"workspace":  workspace,
		"providerId": "case-fund-provider",
		"model":      "case-fund-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	turn := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "交易明细表数据量多少？",
	}), http.StatusAccepted)

	// Tool-policy selection remains covered by
	// internal/app/loop.TestCaseFundAnalysisPolicyScopesAvailableTools and
	// TestFundsSentinelRejectsReadGrepAndMemoryMCP. A DSV1 source must expose no
	// provider tool surface at all.
	if provider.RequestCount() != 0 || analytixFunds.ProbeCount(t) != 0 || analytixFunds.CallCount(t) != 0 || analytixFunds.EvidenceReadCount(t) != 0 {
		t.Fatalf("legacy snapshot exposed a provider/tool surface: provider=%d probes=%d tools=%d evidence=%d", provider.RequestCount(), analytixFunds.ProbeCount(t), analytixFunds.CallCount(t), analytixFunds.EvidenceReadCount(t))
	}
	assertLegacySnapshotSourceUnavailable(t, server.URL, threadID, stringField(turn, "turnId"))
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	for _, forbidden := range []string{"old-report.md", "mcp__memory__read_graph", "mcp__analytix_funds__rank_holders", "mcp__analytix_funds__count_case_rows"} {
		if strings.Contains(replay, forbidden) {
			t.Fatalf("legacy snapshot replay exposed %q:\n%s", forbidden, replay)
		}
	}
}

func TestCaseFundUnavailableReplacesFabricatedFinal(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "case-workspace")
	if err := os.MkdirAll(filepath.Join(workspace, ".analytix"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeRuntimeCaseProjectBinding(t, workspace, "38dc50241996")
	if err := os.WriteFile(filepath.Join(workspace, "old-report.md"), []byte("stale report must not be read"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{{
		`data: {"choices":[{"delta":{"content":"当前案件清洗后交易明细共有 2,645,472 条。"}}]}`,
		`data: [DONE]`,
	}})
	memoryServer := newRuntimeServerToolListMCPServer(t, []map[string]any{{
		"name":        "read_graph",
		"description": "Read historical memory graph entries.",
		"inputSchema": map[string]any{"type": "object"},
	}})
	defer memoryServer.Close()
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "case-fund-no-mcp-provider", "case-fund-no-mcp-model"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"mcpServers": map[string]any{
				"memory": map[string]any{
					"transport":  "http",
					"url":        memoryServer.URL,
					"trustScope": "user",
				},
			},
		})),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Case fund missing MCP",
		"workspace":  workspace,
		"providerId": "case-fund-no-mcp-provider",
		"model":      "case-fund-no-mcp-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	turn := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "交易明细表数据量多少？",
	}), http.StatusAccepted)
	turnID := stringField(turn, "turnId")

	if provider.RequestCount() != 0 {
		t.Fatalf("source unavailable must be a host terminal before provider invocation, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	thread = assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	completedTurn := findRuntimeServerTurn(t, thread, turnID)
	items, _ := completedTurn["items"].([]any)
	finalText := ""
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if stringField(item, "kind") == "assistant_text" && stringField(item, "status") == "completed" {
			finalText += stringField(item, "text")
		}
	}
	if finalText != apploop.CaseFundSourceUnavailableAnswer() {
		t.Fatalf("source unavailable must use the fixed host boundary, got %q", finalText)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	for _, forbidden := range []string{"2,645,472", "mcp__memory__read_graph", "old-report.md", "assistant_reasoning"} {
		if strings.Contains(replay, forbidden) {
			t.Fatalf("source-unavailable replay leaked %q:\n%s", forbidden, replay)
		}
	}
}

func TestUnboundHighRiskCaseRequestUsesHostBoundaryBeforeProviderOrSideEffects(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "unbound-case-workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTarget := filepath.Join(workspace, "fabricated-case-report.md")
	bashTarget := filepath.Join(workspace, "fabricated-shell-output.txt")
	provider := newCompleteProviderServer(t, [][]string{{
		`data: {"choices":[{"delta":{"content":"案件金额 9,999,999 元，银行卡号 6222020000000000000，MAC AA:BB:CC:DD:EE:FF，相关人员互为亲属，投标报价一致。","tool_calls":[{"index":0,"id":"call_forged_write","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"fabricated-case-report.md\",\"content\":\"unsupported case facts\"}"}},{"index":1,"id":"call_forged_bash","type":"function","function":{"name":"bash","arguments":"{\"command\":\"printf pwned > fabricated-shell-output.txt\"}"}}]}}]}`,
		`data: [DONE]`,
	}})
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), Host: "127.0.0.1", Port: 0, DataDir: dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "unbound-case-provider", "malicious-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Unbound high-risk case", "workspace": workspace, "providerId": "unbound-case-provider", "model": "malicious-model",
		"approvalPolicy": "auto", "sandboxMode": "danger-full-access",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	turn := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "请核实本案件的银行账号、金额、MAC、亲属关系和投标报价，并生成正式研判报告。",
	}), http.StatusAccepted)
	if provider.RequestCount() != 0 {
		t.Fatalf("unbound high-risk request reached malicious provider: calls=%d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	for _, path := range []string{writeTarget, bashTarget} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("unbound high-risk request created an artifact before host admission: path=%s err=%v", path, err)
		}
	}
	assertIncidentAcceptedFinal(t, server.URL, threadID, stringField(turn, "turnId"), apploop.CaseFundSourceUnavailableAnswer())
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	for _, forbidden := range []string{"9,999,999", "6222020000000000000", "AA:BB:CC:DD:EE:FF", "互为亲属", "投标报价一致", "call_forged_write", "call_forged_bash"} {
		if strings.Contains(replay, forbidden) {
			t.Fatalf("unbound risk boundary leaked malicious provider material %q:\n%s", forbidden, replay)
		}
	}
}

func TestUnboundStructuredBankCardRequestCannotBypassFinalEvidenceGate(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "unbound-card-workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{{
		`data: {"choices":[{"delta":{"content":"该卡余额为 9,999,999 元。"}}]}`,
		`data: [DONE]`,
	}})
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), Host: "127.0.0.1", Port: 0, DataDir: dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "unbound-card-provider", "malicious-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Unbound card request", "workspace": workspace, "providerId": "unbound-card-provider", "model": "malicious-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	turn := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "卡号 6222021234567890123 余额多少？",
	}), http.StatusAccepted)

	if provider.RequestCount() != 0 {
		t.Fatalf("structured protected data reached provider without case authority: calls=%d", provider.RequestCount())
	}
	assertIncidentAcceptedFinal(t, server.URL, threadID, stringField(turn, "turnId"), apploop.CaseFundSourceUnavailableAnswer())
}

func TestRuntimeServerDeepSeekMultiToolApprovalResumesWithSafeSemanticHistory(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "approval.txt"), []byte("approved"), 0o644); err != nil {
		t.Fatal(err)
	}
	const reasoningSentinel = "DEEPSEEK_PRIVATE_MULTI_TOOL_APPROVAL_REASONING"
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"reasoning_content":"` + reasoningSentinel + `"}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read","type":"function","function":{"name":"read","arguments":"{\"path\":\"approval.txt\"}"}},{"index":1,"id":"call_glob","type":"function","function":{"name":"glob","arguments":"{\"pattern\":\"*.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"approval continuation complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: durableRoot,
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        dataDir,
		ModelProvidersJSON: testModelProvidersJSONWithReasoningProtocol(
			provider.URL(), "deepseek-approval-provider", "deepseek-v4-pro", "deepseek-chat-completions",
		),
	}))
	defer server.Close()
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "DeepSeek multi-tool approval", "workspace": workspace,
		"providerId": "deepseek-approval-provider", "model": "deepseek-v4-pro",
		"approvalPolicy": "always", "sandboxMode": "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Read and list only after approval.",
	}), http.StatusAccepted)
	pendingID := stringField(start, "pendingId")
	if start["status"] != "waiting" || start["pendingKind"] != "approval" ||
		pendingID == "" || provider.RequestCount() != 1 {
		t.Fatalf("DeepSeek multi-tool response did not pause at the first approval: start=%#v calls=%d", start, provider.RequestCount())
	}
	assertRuntimeFilesExcludeValues(t, dataDir, reasoningSentinel)
	assertRuntimeFilesExcludeValues(t, durableRoot, reasoningSentinel)

	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/approvals/"+pendingID, DefaultRuntimeToken, mustJSON(t, map[string]string{"decision": "allow"}), http.StatusOK)
	if provider.RequestCount() != 2 {
		t.Fatalf("approved DeepSeek multi-tool response did not resume exactly once: %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	secondBody := provider.Body(1)
	for _, expected := range []string{
		"Analytix host-selected prior tool activity",
		`\"tool\":\"read\"`,
		`\"status\":\"completed\"`,
		"approved",
		`\"tool\":\"glob\"`,
		`\"status\":\"result_unavailable\"`,
	} {
		if !strings.Contains(secondBody, expected) {
			t.Fatalf("DeepSeek approval continuation missing %q: %s", expected, secondBody)
		}
	}
	for _, forbidden := range []string{reasoningSentinel, `"reasoning_content"`, `"tool_calls"`, `"tool_call_id"`} {
		if strings.Contains(secondBody, forbidden) {
			t.Fatalf("DeepSeek approval continuation retained private or incomplete wire protocol %q: %s", forbidden, secondBody)
		}
	}
	assertRuntimeFilesExcludeValues(t, dataDir, reasoningSentinel)
	assertRuntimeFilesExcludeValues(t, durableRoot, reasoningSentinel)
}

func TestUnboundAmountEntityFactCannotUseGeneralOutput(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "unbound-amount-workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{{
		`data: {"choices":[{"delta":{"content":"合成样例事件甲取得了该笔款项。"}}]}`,
		`data: [DONE]`,
	}})
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), Host: "127.0.0.1", Port: 0, DataDir: dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "unbound-amount-provider", "malicious-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Unbound amount request", "workspace": workspace, "providerId": "unbound-amount-provider", "model": "malicious-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	turn := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "合成样例事件甲于2026-07-01取得￥2,645,472.00，请确认。",
	}), http.StatusAccepted)

	if provider.RequestCount() != 0 {
		t.Fatalf("amount/entity case fact reached provider without case authority: calls=%d", provider.RequestCount())
	}
	assertIncidentAcceptedFinal(t, server.URL, threadID, stringField(turn, "turnId"), apploop.CaseFundSourceUnavailableAnswer())
}

func TestProviderProtectedCaseFactDraftOnGeneralTurnIsNeverPublished(t *testing.T) {
	const draftSentinel = "银行账号 6222021234567890123 支付 2645472 元。"
	if !domainsecurity.ContainsProtectedCaseFactCandidate(draftSentinel) {
		t.Fatal("regression sentinel must exercise the protected case-fact projection")
	}
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "general-draft-workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{{
		`data: {"choices":[{"delta":{"content":"` + draftSentinel + `"}}]}`,
		`data: [DONE]`,
	}})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), Host: "127.0.0.1", Port: 0, DataDir: dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "general-draft-provider", "malicious-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "General draft containment", "workspace": workspace, "providerId": "general-draft-provider", "model": "malicious-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	turn := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Summarize the workspace in one sentence.",
	}), http.StatusAccepted)
	if provider.RequestCount() != 1 {
		t.Fatalf("general provider request count=%d want 1", provider.RequestCount())
	}

	thread = assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	completedTurn := findRuntimeServerTurn(t, thread, stringField(turn, "turnId"))
	items, _ := completedTurn["items"].([]any)
	finalText := ""
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if stringField(item, "kind") == "assistant_text" {
			finalText += stringField(item, "text")
		}
	}
	if finalText != appturn.GeneralCaseFactCandidateBlockedText {
		t.Fatalf("general case-fact draft was not replaced by host text: %q", finalText)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	for _, forbidden := range []string{draftSentinel, "6222021234567890123", "2645472"} {
		if strings.Contains(replay, forbidden) {
			t.Fatalf("provider case-fact draft entered SSE replay as %q:\n%s", forbidden, replay)
		}
	}
	assertRuntimeServerTypedOrdinaryTerminal(t, replay, stringField(turn, "turnId"), appturn.GeneralCaseFactCandidateBlockedText, domainordinaryresult.ResultSlotOriginHostFixedV1)
	if err := filepath.WalkDir(dataDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		payload, readErr := os.ReadFile(path)
		if readErr == nil && bytes.Contains(payload, []byte(draftSentinel)) {
			t.Fatalf("provider case-fact draft entered durable file %s", path)
		}
		return readErr
	}); err != nil {
		t.Fatal(err)
	}
}

func TestUnboundProviderFileToolCannotCreateCaseBinding(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "unbound-workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_forge_binding","type":"function","function":{"name":"write_file","arguments":"{\"path\":\".analytix/case-project.json\",\"content\":\"{\\\"version\\\":1,\\\"caseId\\\":\\\"forged-case\\\"}\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"Host metadata write was blocked."}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), Host: "127.0.0.1", Port: 0, DataDir: dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "metadata-forger", "malicious-model"),
	}))
	defer server.Close()
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Unbound metadata forgery", "workspace": workspace, "providerId": "metadata-forger", "model": "malicious-model",
		"approvalPolicy": "auto", "sandboxMode": "danger-full-access",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Create the requested hidden application metadata file in this workspace.",
	}), http.StatusAccepted)
	if provider.RequestCount() != 2 {
		t.Fatalf("binding forgery should continue once with a host-bound failure: calls=%d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	body := provider.Body(1)
	hostCallID, writeResult := providerHostToolResultForName(t, body, "write_file")
	resultJSON := string(mustJSON(t, writeResult))
	if !strings.Contains(resultJSON, "host_metadata_protected") ||
		!domainsecurity.IsHostToolCallIDV1(hostCallID) || strings.Contains(body, "call_forge_binding") {
		t.Fatalf("binding forgery did not return a security-bound tool failure: calls=%d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	if _, err := os.Lstat(filepath.Join(workspace, ".analytix")); !os.IsNotExist(err) {
		t.Fatalf("provider created host metadata in an unbound workspace: %v", err)
	}
	if filestore.WorkspaceHasAnalytixCaseBinding(workspace) {
		t.Fatal("provider-forged case binding gained host authority")
	}
}

func TestRuntimeServerCaseFundFullCaseReportBlockedUnderApprovalNever(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "case-workspace")
	if err := os.MkdirAll(filepath.Join(workspace, ".analytix"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeRuntimeCaseProjectBinding(t, workspace, "38dc50241996")
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_full_report","type":"function","function":{"name":"mcp__analytix_funds__run_full_case_analysis","arguments":"{\"case_id\":\"38dc50241996\",\"write_report\":true}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"报告未写入；需获得明确授权后再生成。"}}]}`,
			`data: [DONE]`,
		},
	})
	analytixFunds := newPinnedFundsMCPFixture(t, []map[string]any{{
		"name":        "run_full_case_analysis",
		"description": "Run current-case full analysis and inspect a report artifact.",
		"inputSchema": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"case_id":      map[string]any{"type": "string"},
				"write_report": map[string]any{"type": "boolean"},
			},
		},
		"annotations": map[string]any{"readOnlyHint": false},
	}}, map[string]any{"content": []map[string]any{{"type": "text", "text": "report artifact inspected"}}})
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "case-fund-report-provider", "case-fund-report-model"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"mcpServers": map[string]any{
				"analytix_funds": analytixFunds.config,
			},
		})),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Case fund report",
		"workspace":      workspace,
		"providerId":     "case-fund-report-provider",
		"model":          "case-fund-report-model",
		"approvalPolicy": "never",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	turn := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "基于当前案件生成一份资金研判简报，并附主要资金流向表。",
	}), http.StatusAccepted)

	if provider.RequestCount() != 0 {
		t.Fatalf("host-quarantined report capability reached the provider, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	if got := analytixFunds.CallCount(t); got != 0 {
		t.Fatalf("run_full_case_analysis must not execute under approvalPolicy=never, got %d", got)
	}
	assertIncidentAcceptedFinal(t, server.URL, threadID, stringField(turn, "turnId"), apploop.CaseFundSourceUnavailableAnswer())
}

func TestRuntimeServerCaseFundOtherMutatingMCPStillBlockedUnderApprovalNever(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "case-workspace")
	if err := os.MkdirAll(filepath.Join(workspace, ".analytix"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeRuntimeCaseProjectBinding(t, workspace, "38dc50241996")
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_export","type":"function","function":{"name":"mcp__analytix_funds__export_cleaned_case_data","arguments":"{\"case_id\":\"38dc50241996\",\"confirm_export\":true}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"导出未执行。"}}]}`,
			`data: [DONE]`,
		},
	})
	analytixFunds := newPinnedFundsMCPFixture(t, []map[string]any{{
		"name":        "export_cleaned_case_data",
		"description": "Export current-case cleaned data.",
		"inputSchema": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"case_id":        map[string]any{"type": "string"},
				"confirm_export": map[string]any{"type": "boolean"},
			},
			"required": []string{"case_id", "confirm_export"},
		},
		"annotations": map[string]any{"readOnlyHint": false},
	}}, map[string]any{"content": []map[string]any{{"type": "text", "text": "unexpected export"}}})
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "case-fund-export-provider", "case-fund-export-model"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"mcpServers": map[string]any{
				"analytix_funds": analytixFunds.config,
			},
		})),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Case fund export",
		"workspace":      workspace,
		"providerId":     "case-fund-export-provider",
		"model":          "case-fund-export-model",
		"approvalPolicy": "never",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	turn := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "导出当前案件资金清洗明细。",
	}), http.StatusAccepted)

	if provider.RequestCount() != 0 {
		t.Fatalf("host-quarantined export capability reached the provider, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	if got := analytixFunds.CallCount(t); got != 0 {
		t.Fatalf("export_cleaned_case_data must remain blocked under approvalPolicy=never, got MCP calls=%d", got)
	}
	assertIncidentAcceptedFinal(t, server.URL, threadID, stringField(turn, "turnId"), apploop.CaseFundSourceUnavailableAnswer())
}

func TestCaseBoundArbitraryMutatingMCPIsNeverAdvertisedOrExecuted(t *testing.T) {
	dataDir := t.TempDir()
	sideEffectPath := filepath.Join(dataDir, "unauthorized-bundle.zip")
	fixture := newPinnedMCPFixture(t, "server", []map[string]any{{
		"name":        "generate_bundle",
		"description": "Generate a bundle and write it to disk.",
		"inputSchema": map[string]any{"type": "object", "additionalProperties": false},
		"annotations": map[string]any{"readOnlyHint": false},
	}}, map[string]any{"content": []map[string]any{{"type": "text", "text": "unexpected bundle"}}})
	fixture.config["env"].(map[string]string)["ANALYTIX_PINNED_FUNDS_MCP_SIDE_EFFECT_PATH"] = sideEffectPath
	caseSource := newPinnedCaseSourceMCPFixture(t)
	provider := newCompleteProviderServer(t, [][]string{{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_generate_bundle","type":"function","function":{"name":"mcp__server__generate_bundle","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}})
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), Host: "127.0.0.1", Port: 0, DataDir: dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "generic-mcp-provider", "malicious-model"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{"mcpServers": map[string]any{
			"analytix_funds": caseSource.config,
			"server":         fixture.config,
		}})),
	}))
	t.Cleanup(server.Close)

	for _, approvalPolicy := range []string{"auto", "on-request", "never"} {
		t.Run(approvalPolicy, func(t *testing.T) {
			workspace := filepath.Join(dataDir, "case-workspace-"+approvalPolicy)
			if err := os.MkdirAll(filepath.Join(workspace, ".analytix"), 0o755); err != nil {
				t.Fatal(err)
			}
			writeRuntimeCaseProjectBinding(t, workspace, "case-generic-mcp-write")
			thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
				"title": "Generic MCP write quarantine", "workspace": workspace, "providerId": "generic-mcp-provider", "model": "malicious-model",
				"approvalPolicy": approvalPolicy, "sandboxMode": "danger-full-access",
			}), http.StatusCreated)
			threadID := stringField(thread, "id")
			turn := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
				"prompt": "Call mcp__server__generate_bundle to package the workspace assets.",
			}), http.StatusAccepted)
			// Execution-time re-parsing of the advertised allowlist remains covered by
			// internal/app/loop.TestRejectUnadvertisedToolInAgentMode. For DSV1,
			// internal/mcp.TestLegacyReadyProbeCannotAdvertiseExecuteOrMintGrant
			// requires the earlier source boundary tested here.
			if provider.RequestCount() != 0 || caseSource.ProbeCount(t) != 0 || caseSource.CallCount(t) != 0 || caseSource.EvidenceReadCount(t) != 0 || fixture.CallCount(t) != 0 || fixture.EvidenceReadCount(t) != 0 {
				t.Fatalf("legacy snapshot escaped mutating MCP quarantine under %s: provider=%d probes=%d sourceTools=%d sourceEvidence=%d foreignTools=%d foreignEvidence=%d", approvalPolicy, provider.RequestCount(), caseSource.ProbeCount(t), caseSource.CallCount(t), caseSource.EvidenceReadCount(t), fixture.CallCount(t), fixture.EvidenceReadCount(t))
			}
			if _, err := os.Stat(sideEffectPath); !os.IsNotExist(err) {
				t.Fatalf("case-bound mutating MCP wrote a file under %s: %v", approvalPolicy, err)
			}
			assertLegacySnapshotSourceUnavailable(t, server.URL, threadID, stringField(turn, "turnId"))
		})
	}
}

func TestCaseBoundBuiltinWriteRequiresPublicationAuthority(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "case-workspace")
	if err := os.MkdirAll(filepath.Join(workspace, ".analytix"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeRuntimeCaseProjectBinding(t, workspace, "38dc50241996")
	target := filepath.Join(workspace, "unauthorized-output.md")
	sentinel := "UNSUPPORTED_CASE_FACT_AMOUNT_4200000"
	provider := newCompleteProviderServer(t, [][]string{{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_write_case_report","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"unauthorized-output.md\",\"content\":\"UNSUPPORTED_CASE_FACT_AMOUNT_4200000\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}})
	caseSource := newPinnedCaseSourceMCPFixture(t)
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), Host: "127.0.0.1", Port: 0, DataDir: dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "case-write-provider", "case-write-model"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{"mcpServers": map[string]any{
			"analytix_funds": caseSource.config,
		}})),
	}))
	defer server.Close()
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Case write quarantine", "workspace": workspace, "providerId": "case-write-provider", "model": "case-write-model",
		"approvalPolicy": "auto", "sandboxMode": "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	turn := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "请在当前工作区创建 unauthorized-output.md 文件。",
	}), http.StatusAccepted)
	// Publication-receipt enforcement remains covered by
	// internal/app/evidence.TestCaseBoundaryReportRequiresPublicationReceipt
	// and internal/app/reportpublication.TestReportRequiresPublicationReceipt.
	// This DSV1 turn must stop before the provider can even request a write.
	if provider.RequestCount() != 0 || caseSource.ProbeCount(t) != 0 || caseSource.CallCount(t) != 0 || caseSource.EvidenceReadCount(t) != 0 {
		t.Fatalf("legacy snapshot escaped builtin-write quarantine: provider=%d probes=%d tools=%d evidence=%d", provider.RequestCount(), caseSource.ProbeCount(t), caseSource.CallCount(t), caseSource.EvidenceReadCount(t))
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("case write produced a file before PublicationReceipt: %v", err)
	}
	assertLegacySnapshotSourceUnavailable(t, server.URL, threadID, stringField(turn, "turnId"))
	if err := filepath.WalkDir(dataDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		payload, readErr := os.ReadFile(path)
		if readErr == nil && bytes.Contains(payload, []byte(sentinel)) {
			t.Fatalf("case write arguments leaked to durable file %s", path)
		}
		return readErr
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeServerCaseFundUnverifiedFinalUsesHostBoundary(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "case-workspace")
	if err := os.MkdirAll(filepath.Join(workspace, ".analytix"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeRuntimeCaseProjectBinding(t, workspace, "38dc50241996")
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"content":"我通过 DuckDB analysis_txn_detail_idx 预检，还需补一次查询，是否继续？"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"当前案件清洗后交易明细共有 2,645,472 条。"}}]}`,
			`data: [DONE]`,
		},
	})
	analytixFunds := newPinnedFundsMCPFixture(t, []map[string]any{{
		"name":        "count_case_rows",
		"description": "Count current-case rows.",
		"inputSchema": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties":           map[string]any{},
		},
	}}, nil)
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "case-fund-recovery-provider", "case-fund-recovery-model"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"mcpServers": map[string]any{
				"analytix_funds": analytixFunds.config,
			},
		})),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Case fund recovery",
		"workspace":  workspace,
		"providerId": "case-fund-recovery-provider",
		"model":      "case-fund-recovery-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	turn := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "交易明细表数据量多少？",
	}), http.StatusAccepted)
	turnID := stringField(turn, "turnId")

	// Claim-without-receipt rejection remains covered by
	// internal/app/evidence.TestConcreteClaimRequiresEvidenceReceipt and the
	// final-gate tests. This DSV1 source cannot reach an unverified model draft.
	if provider.RequestCount() != 0 || analytixFunds.ProbeCount(t) != 0 || analytixFunds.CallCount(t) != 0 || analytixFunds.EvidenceReadCount(t) != 0 {
		t.Fatalf("legacy snapshot reached unverified-final execution: provider=%d probes=%d tools=%d evidence=%d", provider.RequestCount(), analytixFunds.ProbeCount(t), analytixFunds.CallCount(t), analytixFunds.EvidenceReadCount(t))
	}
	assertLegacySnapshotSourceUnavailable(t, server.URL, threadID, turnID)
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	for _, forbidden := range []string{"case_fund_final_recovered", "DuckDB", "analysis_txn_detail_idx", "2,645,472", "query_id"} {
		if strings.Contains(replay, forbidden) {
			t.Fatalf("legacy snapshot boundary leaked %q into replay:\n%s", forbidden, replay)
		}
	}
}

func TestRuntimeServerRejectsRemoteReadOnlyHintsWithoutHostAuthority(t *testing.T) {
	dataDir := t.TempDir()
	var toolCallCount atomic.Int32
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_mcp_a","type":"function","function":{"name":"mcp__runtime-mcp__lookup_a","arguments":"{\"query\":\"alpha\"}"}},{"index":1,"id":"call_mcp_b","type":"function","function":{"name":"mcp__runtime-mcp__lookup_b","arguments":"{\"query\":\"beta\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"mcp reads complete"}}]}`,
			`data: [DONE]`,
		},
	})
	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode MCP request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": runtimeMCPInitializeResult("runtime-mcp")})
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"tools": []map[string]any{
				{
					"name":         "lookup_a",
					"description":  "Lookup runtime MCP data A",
					"inputSchema":  map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}},
					"outputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
					"annotations":  map[string]any{"readOnlyHint": true},
				},
				{
					"name":         "lookup_b",
					"description":  "Lookup runtime MCP data B",
					"inputSchema":  map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}},
					"outputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
					"annotations":  map[string]any{"readOnlyHint": true},
				},
			}}})
		case "tools/call":
			args, _ := request.Params["arguments"].(map[string]any)
			toolCallCount.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"content": []map[string]any{{
				"type": "text",
				"text": "mcp result: " + fmt.Sprint(args["query"]),
			}}, "structuredContent": map[string]any{}}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
	defer mcpServer.Close()
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "mcp-parallel-provider", "mcp-parallel-model"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{
			"mcpServers": map[string]any{
				"runtime-mcp": map[string]any{
					"transport":  "http",
					"url":        mcpServer.URL,
					"trustScope": "user",
				},
			},
		})),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Parallel MCP reads",
		"workspace":      dataDir,
		"providerId":     "mcp-parallel-provider",
		"model":          "mcp-parallel-model",
		"approvalPolicy": "auto",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Call configured read-only MCP tools.",
	}), http.StatusAccepted)
	if calls := toolCallCount.Load(); calls != 0 {
		t.Fatalf("remote readOnlyHint reached MCP transport without host authority: calls=%d", calls)
	}
	if provider.RequestCount() != 2 {
		t.Fatalf("MCP rejection loop should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	continuation := provider.Body(1)
	if strings.Count(continuation, "side_effect_identity_unavailable") < 2 || strings.Contains(continuation, "mcp result:") {
		t.Fatalf("provider continuation must contain two host rejections and no MCP result:\n%s", continuation)
	}
}

func TestRuntimeServerEditRequiresFreshReadBeforeEdit(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	notePath := filepath.Join(workspace, "note.txt")
	if err := os.WriteFile(notePath, []byte("old value\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_edit_without_read","type":"function","function":{"name":"edit","arguments":"{\"path\":\"note.txt\",\"oldText\":\"old value\",\"newText\":\"new value\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"blocked until read"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":15,"completion_tokens":4,"total_tokens":19}}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "edit-block-provider", "edit-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Edit blocked",
		"workspace":      workspace,
		"providerId":     "edit-block-provider",
		"model":          "edit-model",
		"approvalPolicy": "on-request",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{"prompt": "Edit without reading."}), http.StatusAccepted)
	if start["pendingKind"] == "approval" {
		t.Fatalf("edit without a fresh read must not request approval: %#v", start)
	}
	if provider.RequestCount() != 2 {
		t.Fatalf("blocked edit should feed an error tool result into a second provider call, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	body := provider.Body(1)
	_, editResult := providerHostToolResultForName(t, body, "edit")
	if !strings.Contains(string(mustJSON(t, editResult)), "read_before_edit_required") {
		t.Fatalf("second provider call must contain host-bound read-before-edit tool result: result=%#v body=%s", editResult, body)
	}
	data, err := os.ReadFile(notePath)
	if err != nil || string(data) != "old value\n" {
		t.Fatalf("blocked edit must not mutate the file, data=%q err=%v", string(data), err)
	}
}

func TestRuntimeServerEditAfterReadUsesApprovalAndExecutes(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	notePath := filepath.Join(workspace, "note.txt")
	if err := os.WriteFile(notePath, []byte("first line\nold value\nlast line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_edit_read","type":"function","function":{"name":"read","arguments":"{\"path\":\"note.txt\"}"}},{"index":1,"id":"call_edit_apply","type":"function","function":{"name":"edit","arguments":"{\"path\":\"note.txt\",\"oldText\":\"old value\",\"newText\":\"new value\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"edit complete"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":22,"completion_tokens":2,"total_tokens":24}}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "edit-provider", "edit-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Edit after read",
		"workspace":      workspace,
		"providerId":     "edit-provider",
		"model":          "edit-model",
		"approvalPolicy": "on-request",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{"prompt": "Read then edit."}), http.StatusAccepted)
	pendingID := stringField(start, "pendingId")
	if start["pendingKind"] != "approval" || !strings.HasPrefix(pendingID, "appr_") {
		t.Fatalf("edit after read should request approval for the edit call: %#v", start)
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/approvals/"+pendingID, DefaultRuntimeToken, mustJSON(t, map[string]string{"decision": "allow"}), http.StatusOK)
	if provider.RequestCount() != 2 {
		t.Fatalf("approved edit should continue provider loop, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	data, err := os.ReadFile(notePath)
	if err != nil || string(data) != "first line\nnew value\nlast line\n" {
		t.Fatalf("approved edit should mutate the file, data=%q err=%v", string(data), err)
	}
	body := provider.Body(1)
	_, readResult := providerHostToolResultForName(t, body, "read")
	_, editResult := providerHostToolResultForName(t, body, "edit")
	editResultJSON := string(mustJSON(t, editResult))
	if !strings.Contains(string(mustJSON(t, readResult)), "old value") ||
		!strings.Contains(editResultJSON, `first_changed_line`) || !strings.Contains(editResultJSON, `new value`) {
		t.Fatalf("second provider call must include paired host-bound read/edit tool results: read=%#v edit=%#v body=%s", readResult, editResult, body)
	}
	for _, expected := range []string{"diff_kind", "modify", "added", "removed", "--- a/note.txt", "-old value", "+new value"} {
		if !strings.Contains(editResultJSON, expected) {
			t.Fatalf("approved edit tool result should include unified diff metadata %q: result=%s body=%s", expected, editResultJSON, body)
		}
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if !strings.Contains(replay, "event: approval_requested") ||
		!strings.Contains(replay, "event: approval_resolved") ||
		!strings.Contains(replay, "event: tool_call_finished") {
		t.Fatalf("edit replay should include approval and tool result events:\n%s", replay)
	}
}

func TestRuntimeServerWriteFileToolReturnsUnifiedDiffMetadata(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	notePath := filepath.Join(workspace, "note.txt")
	if err := os.WriteFile(notePath, []byte("old line\nkeep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeArgs := mustJSONString(t, map[string]string{
		"path":    "note.txt",
		"content": "new line\nkeep\n",
	})
	createArgs := mustJSONString(t, map[string]string{
		"path":    "created.txt",
		"content": "created one\ncreated two\n",
	})
	provider := newCompleteProviderServer(t, [][]string{
		{
			"data: " + mustJSONString(t, map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{
							{
								"index": 0,
								"id":    "call_write_modify",
								"type":  "function",
								"function": map[string]any{
									"name":      "write_file",
									"arguments": writeArgs,
								},
							},
							{
								"index": 1,
								"id":    "call_write_create",
								"type":  "function",
								"function": map[string]any{
									"name":      "write_file",
									"arguments": createArgs,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				}},
			}),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"write complete"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":18,"completion_tokens":3,"total_tokens":21}}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "write-provider", "write-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Write diff",
		"workspace":      workspace,
		"providerId":     "write-provider",
		"model":          "write-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Write files with diffs.",
	}), http.StatusAccepted)
	if provider.RequestCount() != 2 {
		t.Fatalf("write loop should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	data, err := os.ReadFile(notePath)
	if err != nil || string(data) != "new line\nkeep\n" {
		t.Fatalf("write_file should mutate existing file, data=%q err=%v", string(data), err)
	}
	created, err := os.ReadFile(filepath.Join(workspace, "created.txt"))
	if err != nil || string(created) != "created one\ncreated two\n" {
		t.Fatalf("write_file should create new file, data=%q err=%v", string(created), err)
	}
	secondBody := provider.Body(1)
	writeCallIDs := providerHostToolCallIDsForName(t, secondBody, "write_file")
	if len(writeCallIDs) != 2 {
		t.Fatalf("write provider history lost tool calls: ids=%#v body=%s", writeCallIDs, secondBody)
	}
	writeResults := ""
	for _, callID := range writeCallIDs {
		if !domainsecurity.IsHostToolCallIDV1(callID) {
			t.Fatalf("write provider history contains a non-host call id %q: %s", callID, secondBody)
		}
		writeResults += string(mustJSON(t, providerToolResultForCall(t, secondBody, callID)))
	}
	for _, expected := range []string{
		`relative_path`,
		`note.txt`,
		`diff_kind`,
		`modify`,
		`create`,
		`added`,
		`removed`,
		"--- a/note.txt",
		"+++ b/created.txt",
		"-old line",
		"+new line",
		"+created one",
	} {
		if !strings.Contains(writeResults, expected) {
			t.Fatalf("second provider call should include write_file diff metadata %q: results=%s body=%s", expected, writeResults, secondBody)
		}
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	assertRuntimeServerTypedOrdinaryTerminal(
		t, replay, stringField(start, "turnId"), "write complete", "provider_ordinary_only",
	)
}

func TestRuntimeServerMoveFileToolIsAdvertisedAndExecutes(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, "old"), 0o755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(workspace, "old", "note.txt")
	destinationPath := filepath.Join(workspace, "new", "note.txt")
	content := "move me through checkpoint\n"
	if err := os.WriteFile(sourcePath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	moveArgs := mustJSONString(t, map[string]string{
		"source_path":      "old/note.txt",
		"destination_path": "new/note.txt",
	})
	provider := newCompleteProviderServer(t, [][]string{
		{
			"data: " + mustJSONString(t, map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{{
							"index": 0,
							"id":    "call_move_file",
							"type":  "function",
							"function": map[string]any{
								"name":      "move_file",
								"arguments": moveArgs,
							},
						}},
					},
					"finish_reason": "tool_calls",
				}},
			}),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"move complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "move-provider", "move-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Move file",
		"workspace":  workspace,
		"providerId": "move-provider",
		"model":      "move-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":                "Move file.",
		"workspaceCheckpointId": "gcp_move_file",
		"approvalPolicy":        "auto",
		"sandboxMode":           "workspace-write",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	if !containsString(providerRequestToolNames(t, provider.Body(0)), "move_file") {
		t.Fatalf("provider tool catalog should advertise move_file: %s", provider.Body(0))
	}
	if _, err := os.Stat(sourcePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("move_file should remove source, err=%v", err)
	}
	moved, err := os.ReadFile(destinationPath)
	if err != nil || string(moved) != content {
		t.Fatalf("move_file should write destination content=%q err=%v", string(moved), err)
	}
	body := provider.Body(1)
	_, moveResult := providerHostToolResultForName(t, body, "move_file")
	moveResultJSON := string(mustJSON(t, moveResult))
	if provider.RequestCount() != 2 || !strings.Contains(moveResultJSON, `"moved":true`) ||
		!strings.Contains(moveResultJSON, "old/note.txt") || !strings.Contains(moveResultJSON, "new/note.txt") {
		t.Fatalf("move_file should return a host-bound visible result and continue: result=%s body=%s", moveResultJSON, body)
	}

	checkpointID := checkpointapp.RuntimeCheckpointIDFromWorkspaceCheckpointID("gcp_move_file")
	planResponse := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/checkpoints/"+checkpointID+"/rewind-plan",
		DefaultRuntimeToken,
		mustJSON(t, map[string]any{"scope": "code"}),
		http.StatusOK,
	)
	plan := mapField(t, planResponse, "plan")
	summary := mapField(t, plan, "summary")
	if jsonIntField(t, summary, "fileCount") != 2 || jsonIntField(t, summary, "readyFileCount") != 2 {
		t.Fatalf("move checkpoint plan should include source deletion and destination creation: %#v", summary)
	}
	filesByPath := map[string]map[string]any{}
	for _, raw := range anyList(plan["files"]) {
		file, _ := raw.(map[string]any)
		filesByPath[stringField(file, "relativePath")] = file
	}
	sourceFile := filesByPath["old/note.txt"]
	if sourceFile == nil ||
		sourceFile["changeKind"] != "deleted" ||
		sourceFile["action"] != "restore_deleted_file" ||
		sourceFile["status"] != "ready" ||
		sourceFile["beforeHash"] != testCheckpointHash(content) {
		t.Fatalf("move source should be restorable deleted file: %#v", sourceFile)
	}
	destinationFile := filesByPath["new/note.txt"]
	if destinationFile == nil ||
		destinationFile["changeKind"] != "created" ||
		destinationFile["action"] != "delete_created_file" ||
		destinationFile["status"] != "ready" ||
		destinationFile["afterHash"] != testCheckpointHash(content) {
		t.Fatalf("move destination should be removable created file: %#v", destinationFile)
	}

	apply := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/checkpoints/"+checkpointID+"/rewind-apply",
		DefaultRuntimeToken,
		mustJSON(t, map[string]any{
			"plan": plan,
			"confirmation": map[string]any{
				"confirmed":   true,
				"destructive": true,
				"phrase":      "APPLY_CHECKPOINT_REWIND",
			},
		}),
		http.StatusOK,
	)
	result := mapField(t, apply, "apply")
	if result["status"] != "applied" || jsonIntField(t, mapField(t, result, "summary"), "fileAppliedCount") != 2 {
		t.Fatalf("move checkpoint apply should restore/delete files: %#v", apply)
	}
	restored, err := os.ReadFile(sourcePath)
	if err != nil || string(restored) != content {
		t.Fatalf("move checkpoint apply should restore source content=%q err=%v", string(restored), err)
	}
	if _, err := os.Stat(destinationPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("move checkpoint apply should remove destination, err=%v", err)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if !strings.Contains(replay, "event: checkpoint_captured") ||
		!strings.Contains(replay, "event: checkpoint_rewind_applied") ||
		!strings.Contains(replay, turnID) {
		t.Fatalf("move checkpoint replay should include capture/apply metadata:\n%s", replay)
	}
}

func TestRuntimeServerMoveFileRejectsDestinationExists(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(workspace, "source.txt")
	destinationPath := filepath.Join(workspace, "destination.txt")
	sourceContent := "source stays\n"
	destinationContent := "destination stays\n"
	if err := os.WriteFile(sourcePath, []byte(sourceContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destinationPath, []byte(destinationContent), 0o644); err != nil {
		t.Fatal(err)
	}
	moveArgs := mustJSONString(t, map[string]string{
		"source_path":      "source.txt",
		"destination_path": "destination.txt",
	})
	provider := newCompleteProviderServer(t, [][]string{
		{
			"data: " + mustJSONString(t, map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{{
							"index": 0,
							"id":    "call_move_existing_destination",
							"type":  "function",
							"function": map[string]any{
								"name":      "move_file",
								"arguments": moveArgs,
							},
						}},
					},
					"finish_reason": "tool_calls",
				}},
			}),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"move destination existed"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "move-existing-provider", "move-existing-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Move destination exists",
		"workspace":      workspace,
		"providerId":     "move-existing-provider",
		"model":          "move-existing-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Move onto existing file.",
	}), http.StatusAccepted)
	gotSource, sourceErr := os.ReadFile(sourcePath)
	gotDestination, destinationErr := os.ReadFile(destinationPath)
	if sourceErr != nil || string(gotSource) != sourceContent {
		t.Fatalf("move_file destination_exists must leave source unchanged, data=%q err=%v", string(gotSource), sourceErr)
	}
	if destinationErr != nil || string(gotDestination) != destinationContent {
		t.Fatalf("move_file destination_exists must leave destination unchanged, data=%q err=%v", string(gotDestination), destinationErr)
	}
	body := provider.Body(1)
	_, moveResult := providerHostToolResultForName(t, body, "move_file")
	moveResultJSON := string(mustJSON(t, moveResult))
	if provider.RequestCount() != 2 || !strings.Contains(moveResultJSON, "destination_exists") ||
		!strings.Contains(moveResultJSON, "destination_path already exists") {
		t.Fatalf("move_file destination_exists should return a host-bound visible error and continue: result=%s body=%s", moveResultJSON, body)
	}
}

const runtimeServerSampleNotebook = `{
 "cells": [
  {"cell_type": "markdown", "id": "intro", "metadata": {}, "source": ["# Title\n", "text"]},
  {"cell_type": "code", "id": "c1", "metadata": {}, "execution_count": 5, "outputs": [{"output_type": "stream", "text": "old"}], "source": ["print(1)\n"]}
 ],
 "metadata": {"kernelspec": {"name": "python3"}},
 "nbformat": 4,
 "nbformat_minor": 5
}`

func writeRuntimeServerSampleNotebook(t *testing.T, workspace string, name string) string {
	t.Helper()
	path := filepath.Join(workspace, name)
	if err := os.WriteFile(path, []byte(runtimeServerSampleNotebook), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readRuntimeServerNotebookCells(t *testing.T, path string) []map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatalf("notebook should remain valid JSON: %v", err)
	}
	var cells []map[string]json.RawMessage
	if err := json.Unmarshal(top["cells"], &cells); err != nil {
		t.Fatalf("notebook cells should remain valid: %v", err)
	}
	return cells
}

func TestRuntimeServerNotebookEditToolIsAdvertisedAndReplacesCell(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	notebookPath := writeRuntimeServerSampleNotebook(t, workspace, "nb.ipynb")
	editArgs := mustJSONString(t, map[string]any{
		"path":        "nb.ipynb",
		"cell_number": 1,
		"new_source":  "print(42)\n",
	})
	provider := newCompleteProviderServer(t, [][]string{
		{
			"data: " + mustJSONString(t, map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{{
							"index": 0,
							"id":    "call_notebook_replace",
							"type":  "function",
							"function": map[string]any{
								"name":      "notebook_edit",
								"arguments": editArgs,
							},
						}},
					},
					"finish_reason": "tool_calls",
				}},
			}),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"notebook edited"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "notebook-provider", "notebook-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Notebook edit",
		"workspace":      workspace,
		"providerId":     "notebook-provider",
		"model":          "notebook-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Replace notebook cell.",
	}), http.StatusAccepted)
	if !containsString(providerRequestToolNames(t, provider.Body(0)), "notebook_edit") {
		t.Fatalf("provider tool catalog should advertise notebook_edit: %s", provider.Body(0))
	}
	cells := readRuntimeServerNotebookCells(t, notebookPath)
	if got := string(cells[1]["source"]); !strings.Contains(got, "print(42)") {
		t.Fatalf("notebook_edit should replace source, got %s", got)
	}
	if got := string(cells[1]["outputs"]); got != "[]" {
		t.Fatalf("notebook_edit should clear code outputs, got %s", got)
	}
	if got := string(cells[1]["execution_count"]); got != "null" {
		t.Fatalf("notebook_edit should clear execution_count, got %s", got)
	}
	secondBody := provider.Body(1)
	_, notebookResult := providerHostToolResultForName(t, secondBody, "notebook_edit")
	resultJSON := string(mustJSON(t, notebookResult))
	for _, expected := range []string{"replaced cell source", "diff_kind", "modify", "cell_number", "nb.ipynb"} {
		if !strings.Contains(resultJSON, expected) {
			t.Fatalf("notebook_edit provider continuation missing %q: result=%s body=%s", expected, resultJSON, secondBody)
		}
	}
}

func TestRuntimeServerNotebookEditInsertsAndDeletesCells(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	notebookPath := writeRuntimeServerSampleNotebook(t, workspace, "nb.ipynb")
	insertArgs := mustJSONString(t, map[string]any{
		"path":        "nb.ipynb",
		"edit_mode":   "insert",
		"cell_number": -1,
		"cell_type":   "markdown",
		"new_source":  "top note",
	})
	deleteArgs := mustJSONString(t, map[string]any{
		"path":      "nb.ipynb",
		"edit_mode": "delete",
		"cell_id":   "c1",
	})
	provider := newCompleteProviderServer(t, [][]string{
		{
			"data: " + mustJSONString(t, map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{
							{
								"index": 0,
								"id":    "call_notebook_insert",
								"type":  "function",
								"function": map[string]any{
									"name":      "notebook_edit",
									"arguments": insertArgs,
								},
							},
							{
								"index": 1,
								"id":    "call_notebook_delete",
								"type":  "function",
								"function": map[string]any{
									"name":      "notebook_edit",
									"arguments": deleteArgs,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				}},
			}),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"notebook insert delete complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "notebook-insert-provider", "notebook-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Notebook insert delete",
		"workspace":      workspace,
		"providerId":     "notebook-insert-provider",
		"model":          "notebook-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Insert and delete notebook cells.",
	}), http.StatusAccepted)
	cells := readRuntimeServerNotebookCells(t, notebookPath)
	if len(cells) != 2 {
		t.Fatalf("insert then delete should leave two cells, got %d", len(cells))
	}
	if got := string(cells[0]["source"]); !strings.Contains(got, "top note") {
		t.Fatalf("insert should prepend markdown cell, got %s", got)
	}
	for _, cell := range cells {
		if cellID := string(cell["id"]); strings.Contains(cellID, "c1") {
			t.Fatalf("delete by cell_id should remove c1, cells=%#v", cells)
		}
	}
	secondBody := provider.Body(1)
	notebookCallIDs := providerHostToolCallIDsForName(t, secondBody, "notebook_edit")
	if len(notebookCallIDs) != 2 {
		t.Fatalf("notebook insert/delete provider history lost calls: ids=%#v body=%s", notebookCallIDs, secondBody)
	}
	resultsJSON := ""
	for _, callID := range notebookCallIDs {
		if !domainsecurity.IsHostToolCallIDV1(callID) {
			t.Fatalf("notebook provider history contains a non-host call id %q: %s", callID, secondBody)
		}
		resultsJSON += string(mustJSON(t, providerToolResultForCall(t, secondBody, callID)))
	}
	for _, expected := range []string{"inserted markdown cell", "deleted cell"} {
		if !strings.Contains(resultsJSON, expected) {
			t.Fatalf("notebook insert/delete provider continuation missing %q: results=%s body=%s", expected, resultsJSON, secondBody)
		}
	}
}

func TestRuntimeServerNotebookEditRejectsInvalidTarget(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	notebookPath := writeRuntimeServerSampleNotebook(t, workspace, "nb.ipynb")
	before, err := os.ReadFile(notebookPath)
	if err != nil {
		t.Fatal(err)
	}
	editArgs := mustJSONString(t, map[string]any{
		"path":        "nb.ipynb",
		"cell_number": 99,
		"new_source":  "print(99)\n",
	})
	provider := newCompleteProviderServer(t, [][]string{
		{
			"data: " + mustJSONString(t, map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{{
							"index": 0,
							"id":    "call_notebook_invalid",
							"type":  "function",
							"function": map[string]any{
								"name":      "notebook_edit",
								"arguments": editArgs,
							},
						}},
					},
					"finish_reason": "tool_calls",
				}},
			}),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"notebook invalid target handled"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "notebook-invalid-provider", "notebook-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Notebook invalid",
		"workspace":      workspace,
		"providerId":     "notebook-invalid-provider",
		"model":          "notebook-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Try invalid notebook target.",
	}), http.StatusAccepted)
	after, err := os.ReadFile(notebookPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("invalid notebook_edit target must not mutate file")
	}
	body := provider.Body(1)
	_, notebookResult := providerHostToolResultForName(t, body, "notebook_edit")
	resultJSON := string(mustJSON(t, notebookResult))
	if !strings.Contains(resultJSON, "cell_out_of_range") || !strings.Contains(resultJSON, "cell_number 99 out of range") {
		t.Fatalf("notebook invalid target should return a host-bound visible failed tool result and continue: result=%s body=%s", resultJSON, body)
	}
}

func TestRuntimeServerNotebookEditToolIsAdvertisedAndExecutes(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	notebookPath := filepath.Join(workspace, "analysis.ipynb")
	before := `{
 "cells": [
  {"cell_type": "markdown", "id": "intro", "metadata": {}, "source": ["# Title\n", "text"]},
  {"cell_type": "code", "id": "c1", "metadata": {}, "execution_count": 5, "outputs": [{"output_type": "stream", "text": "old"}], "source": ["print(1)\n"]}
 ],
 "metadata": {"kernelspec": {"name": "python3"}},
 "nbformat": 4,
 "nbformat_minor": 5
}`
	if err := os.WriteFile(notebookPath, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	editArgs := mustJSONString(t, map[string]any{
		"path":        "analysis.ipynb",
		"cell_number": 1,
		"new_source":  "print(42)\n",
	})
	provider := newCompleteProviderServer(t, [][]string{
		{
			"data: " + mustJSONString(t, map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{{
							"index": 0,
							"id":    "call_notebook_edit",
							"type":  "function",
							"function": map[string]any{
								"name":      "notebook_edit",
								"arguments": editArgs,
							},
						}},
					},
					"finish_reason": "tool_calls",
				}},
			}),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"notebook edit complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "notebook-edit-provider", "notebook-edit-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Notebook edit",
		"workspace":      workspace,
		"providerId":     "notebook-edit-provider",
		"model":          "notebook-edit-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":                "Update the notebook cell.",
		"workspaceCheckpointId": "gcp_notebook_edit",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")

	if !containsString(providerRequestToolNames(t, provider.Body(0)), "notebook_edit") {
		t.Fatalf("provider tool catalog should advertise notebook_edit: %s", provider.Body(0))
	}
	notebook := readRuntimeServerNotebook(t, notebookPath)
	cells := notebookCells(t, notebook)
	if len(cells) != 2 {
		t.Fatalf("replace must not change cell count: %#v", cells)
	}
	if got := string(cells[1]["source"]); !strings.Contains(got, `print(42)\n`) {
		t.Fatalf("notebook_edit should replace target cell source: %s", got)
	}
	if got := string(cells[1]["outputs"]); got != "[]" {
		t.Fatalf("notebook_edit should clear code outputs, got %s", got)
	}
	if got := string(cells[1]["execution_count"]); got != "null" {
		t.Fatalf("notebook_edit should clear execution_count, got %s", got)
	}
	for _, key := range []string{"metadata", "nbformat", "nbformat_minor"} {
		if _, ok := notebook[key]; !ok {
			t.Fatalf("notebook_edit lost top-level notebook key %s: %#v", key, notebook)
		}
	}
	body := provider.Body(1)
	_, notebookResult := providerHostToolResultForName(t, body, "notebook_edit")
	resultJSON := string(mustJSON(t, notebookResult))
	if provider.RequestCount() != 2 || !strings.Contains(resultJSON, `"edit_mode":"replace"`) ||
		!strings.Contains(resultJSON, `"cell_number":1`) || !strings.Contains(resultJSON, `"diff_kind":"modify"`) ||
		!strings.Contains(resultJSON, "print(42)") {
		t.Fatalf("notebook_edit should return host-bound diff metadata and continue: result=%s body=%s", resultJSON, body)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	events := runtimeServerEventsForTurn(t, replay, turnID)
	firstRuntimeServerEvent(t, events, "tool_call_started")
	firstRuntimeServerEvent(t, events, "tool_call_finished")
	checkpointID := checkpointapp.RuntimeCheckpointIDFromWorkspaceCheckpointID("gcp_notebook_edit")
	plan := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/checkpoints/"+checkpointID+"/rewind-plan", DefaultRuntimeToken, mustJSON(t, map[string]any{"scope": "code"}), http.StatusOK)
	summary := mapField(t, mapField(t, plan, "plan"), "summary")
	if jsonIntField(t, summary, "fileCount") != 1 || jsonIntField(t, summary, "readyFileCount") != 1 {
		t.Fatalf("notebook_edit checkpoint plan should capture one modified file: %#v", summary)
	}
}

func TestRuntimeServerNotebookEditInsertDeleteAndErrors(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	notebookPath := filepath.Join(workspace, "analysis.ipynb")
	before := `{"cells":[{"cell_type":"markdown","id":"intro","metadata":{},"source":["# Intro\n"]},{"cell_type":"code","id":"calc","metadata":{},"execution_count":7,"outputs":[{"output_type":"stream","text":"old"}],"source":["print(1)\n"]}],"metadata":{},"nbformat":4,"nbformat_minor":5}`
	if err := os.WriteFile(notebookPath, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	insertArgs := mustJSONString(t, map[string]any{
		"path":        "analysis.ipynb",
		"edit_mode":   "insert",
		"cell_number": -1,
		"cell_type":   "markdown",
		"new_source":  "Prepended note\n",
	})
	deleteArgs := mustJSONString(t, map[string]any{
		"path":      "analysis.ipynb",
		"edit_mode": "delete",
		"cell_id":   "calc",
	})
	missingTypeArgs := mustJSONString(t, map[string]any{
		"path":       "analysis.ipynb",
		"edit_mode":  "insert",
		"new_source": "missing type\n",
	})
	escapeArgs := mustJSONString(t, map[string]any{
		"path":        "../escape.ipynb",
		"cell_number": 0,
		"new_source":  "escape\n",
	})
	provider := newCompleteProviderServer(t, [][]string{
		{
			"data: " + mustJSONString(t, map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{
							{
								"index": 0,
								"id":    "call_notebook_insert",
								"type":  "function",
								"function": map[string]any{
									"name":      "notebook_edit",
									"arguments": insertArgs,
								},
							},
							{
								"index": 1,
								"id":    "call_notebook_delete",
								"type":  "function",
								"function": map[string]any{
									"name":      "notebook_edit",
									"arguments": deleteArgs,
								},
							},
							{
								"index": 2,
								"id":    "call_notebook_missing_type",
								"type":  "function",
								"function": map[string]any{
									"name":      "notebook_edit",
									"arguments": missingTypeArgs,
								},
							},
							{
								"index": 3,
								"id":    "call_notebook_escape",
								"type":  "function",
								"function": map[string]any{
									"name":      "notebook_edit",
									"arguments": escapeArgs,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				}},
			}),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"notebook batch complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "notebook-batch-provider", "notebook-batch-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Notebook edit batch",
		"workspace":      workspace,
		"providerId":     "notebook-batch-provider",
		"model":          "notebook-batch-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Insert, delete, and report notebook edit errors.",
	}), http.StatusAccepted)

	notebook := readRuntimeServerNotebook(t, notebookPath)
	cells := notebookCells(t, notebook)
	if len(cells) != 2 || !strings.Contains(string(cells[0]["source"]), "Prepended note") {
		t.Fatalf("notebook_edit insert should prepend a markdown cell: %#v", cells)
	}
	for _, cell := range cells {
		if stringFieldFromRawJSON(cell["id"]) == "calc" {
			t.Fatalf("notebook_edit delete by id should remove calc cell: %#v", cells)
		}
	}
	body := provider.Body(1)
	notebookCallIDs := providerHostToolCallIDsForName(t, body, "notebook_edit")
	resultsJSON := ""
	for _, callID := range notebookCallIDs {
		if !domainsecurity.IsHostToolCallIDV1(callID) {
			t.Fatalf("notebook batch provider history contains a non-host call id %q: %s", callID, body)
		}
		resultsJSON += string(mustJSON(t, providerToolResultForCall(t, body, callID)))
	}
	if provider.RequestCount() != 2 || len(notebookCallIDs) != 4 ||
		!strings.Contains(resultsJSON, "cell_type is required for insert") || !strings.Contains(resultsJSON, "workspace_escape") {
		t.Fatalf("notebook_edit batch should return host-bound successful and failed tool results: ids=%#v results=%s body=%s", notebookCallIDs, resultsJSON, body)
	}
}

func readRuntimeServerNotebook(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var notebook map[string]json.RawMessage
	if err := json.Unmarshal(data, &notebook); err != nil {
		t.Fatalf("notebook_edit wrote invalid notebook JSON: %v\n%s", err, data)
	}
	return notebook
}

func notebookCells(t *testing.T, notebook map[string]json.RawMessage) []map[string]json.RawMessage {
	t.Helper()
	var cells []map[string]json.RawMessage
	if err := json.Unmarshal(notebook["cells"], &cells); err != nil {
		t.Fatalf("notebook cells are not valid cell objects: %v", err)
	}
	return cells
}

func stringFieldFromRawJSON(raw json.RawMessage) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}

func TestRuntimeServerDeleteRangeToolIsAdvertisedAndExecutes(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(workspace, "range.txt")
	before := "line1\nline2\nline3\nline4\nline5\n"
	want := "line1\nline5\n"
	if err := os.WriteFile(sourcePath, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	deleteArgs := mustJSONString(t, map[string]any{
		"path":         "range.txt",
		"start_anchor": "line2",
		"end_anchor":   "line4",
	})
	provider := newCompleteProviderServer(t, [][]string{
		{
			"data: " + mustJSONString(t, map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{
							{
								"index": 0,
								"id":    "call_read_before_delete_range",
								"type":  "function",
								"function": map[string]any{
									"name":      "read_file",
									"arguments": mustJSONString(t, map[string]string{"path": "range.txt"}),
								},
							},
							{
								"index": 1,
								"id":    "call_delete_range",
								"type":  "function",
								"function": map[string]any{
									"name":      "delete_range",
									"arguments": deleteArgs,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				}},
			}),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"delete_range complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "delete-range-provider", "delete-range-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Delete range",
		"workspace":      workspace,
		"providerId":     "delete-range-provider",
		"model":          "delete-range-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Delete line2 through line4.",
	}), http.StatusAccepted)

	if !containsString(providerRequestToolNames(t, provider.Body(0)), "delete_range") {
		t.Fatalf("provider tool catalog should advertise delete_range: %s", provider.Body(0))
	}
	got, err := os.ReadFile(sourcePath)
	if err != nil || string(got) != want {
		t.Fatalf("delete_range content mismatch:\n%s\nerr=%v", string(got), err)
	}
	body := provider.Body(1)
	_, deleteResult := providerHostToolResultForName(t, body, "delete_range")
	resultJSON := string(mustJSON(t, deleteResult))
	if provider.RequestCount() != 2 ||
		!strings.Contains(resultJSON, `"deleted_lines":3`) ||
		!strings.Contains(resultJSON, `"diff_kind":"modify"`) ||
		!strings.Contains(resultJSON, "-line2") {
		t.Fatalf("delete_range should return host-bound diff metadata and continue: result=%s body=%s", resultJSON, body)
	}
}

func TestRuntimeServerDeleteRangeRequiresReadBeforeDelete(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(workspace, "range.txt")
	before := "line1\nline2\nline3\n"
	if err := os.WriteFile(sourcePath, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	deleteArgs := mustJSONString(t, map[string]any{
		"path":         "range.txt",
		"start_anchor": "line2",
		"end_anchor":   "line2",
	})
	provider := newCompleteProviderServer(t, [][]string{
		{
			"data: " + mustJSONString(t, map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{{
							"index": 0,
							"id":    "call_delete_without_read",
							"type":  "function",
							"function": map[string]any{
								"name":      "delete_range",
								"arguments": deleteArgs,
							},
						}},
					},
					"finish_reason": "tool_calls",
				}},
			}),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"delete_range was blocked"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "delete-range-guard-provider", "delete-range-guard-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Delete range guard",
		"workspace":      workspace,
		"providerId":     "delete-range-guard-provider",
		"model":          "delete-range-guard-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Delete without reading.",
	}), http.StatusAccepted)

	got, err := os.ReadFile(sourcePath)
	if err != nil || string(got) != before {
		t.Fatalf("delete_range must not mutate without read, got=%q err=%v", string(got), err)
	}
	body := provider.Body(1)
	_, deleteResult := providerHostToolResultForName(t, body, "delete_range")
	resultJSON := string(mustJSON(t, deleteResult))
	if provider.RequestCount() != 2 || !strings.Contains(resultJSON, "read_before_edit_required") {
		t.Fatalf("delete_range read-before-edit guard should return a host-bound result and continue: result=%s body=%s", resultJSON, body)
	}
}

func TestRuntimeServerDeleteRangeRejectsDuplicateAnchor(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(workspace, "range.txt")
	before := "line1\nline2\nline3\nline2\nline5\n"
	if err := os.WriteFile(sourcePath, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	deleteArgs := mustJSONString(t, map[string]any{
		"path":         "range.txt",
		"start_anchor": "line2",
		"end_anchor":   "line5",
	})
	provider := newCompleteProviderServer(t, [][]string{
		{
			"data: " + mustJSONString(t, map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{
							{
								"index": 0,
								"id":    "call_read_duplicate_delete_range",
								"type":  "function",
								"function": map[string]any{
									"name":      "read_file",
									"arguments": mustJSONString(t, map[string]string{"path": "range.txt"}),
								},
							},
							{
								"index": 1,
								"id":    "call_duplicate_delete_range",
								"type":  "function",
								"function": map[string]any{
									"name":      "delete_range",
									"arguments": deleteArgs,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				}},
			}),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"duplicate anchor blocked"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "delete-range-duplicate-provider", "delete-range-duplicate-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Delete duplicate anchor",
		"workspace":      workspace,
		"providerId":     "delete-range-duplicate-provider",
		"model":          "delete-range-duplicate-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Delete a duplicate range.",
	}), http.StatusAccepted)

	got, err := os.ReadFile(sourcePath)
	if err != nil || string(got) != before {
		t.Fatalf("delete_range duplicate anchor must not mutate file, got=%q err=%v", string(got), err)
	}
	body := provider.Body(1)
	_, deleteResult := providerHostToolResultForName(t, body, "delete_range")
	resultJSON := string(mustJSON(t, deleteResult))
	if provider.RequestCount() != 2 || !strings.Contains(resultJSON, "anchor_not_unique") {
		t.Fatalf("delete_range duplicate anchor should return a host-bound visible result and continue: result=%s body=%s", resultJSON, body)
	}
}

func TestRuntimeServerDeleteSymbolToolIsAdvertisedAndExecutes(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(workspace, "example.go")
	before := "package example\n\n// Foo does a thing.\nfunc Foo() int { return 1 }\n\nfunc Bar() int { return 2 }\n"
	if err := os.WriteFile(sourcePath, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	deleteArgs := mustJSONString(t, map[string]any{
		"path": "example.go",
		"name": "Foo",
		"kind": "func",
	})
	provider := newCompleteProviderServer(t, [][]string{
		{
			"data: " + mustJSONString(t, map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{
							{
								"index": 0,
								"id":    "call_read_before_delete_symbol",
								"type":  "function",
								"function": map[string]any{
									"name":      "read_file",
									"arguments": mustJSONString(t, map[string]string{"path": "example.go"}),
								},
							},
							{
								"index": 1,
								"id":    "call_delete_symbol",
								"type":  "function",
								"function": map[string]any{
									"name":      "delete_symbol",
									"arguments": deleteArgs,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				}},
			}),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"delete_symbol complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "delete-symbol-provider", "delete-symbol-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Delete symbol",
		"workspace":      workspace,
		"providerId":     "delete-symbol-provider",
		"model":          "delete-symbol-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Delete Foo.",
	}), http.StatusAccepted)

	if !containsString(providerRequestToolNames(t, provider.Body(0)), "delete_symbol") {
		t.Fatalf("provider tool catalog should advertise delete_symbol: %s", provider.Body(0))
	}
	got, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "Foo") || strings.Contains(string(got), "Foo does a thing") || !strings.Contains(string(got), "func Bar") {
		t.Fatalf("delete_symbol content mismatch:\n%s", string(got))
	}
	body := provider.Body(1)
	_, deleteResult := providerHostToolResultForName(t, body, "delete_symbol")
	resultJSON := string(mustJSON(t, deleteResult))
	if provider.RequestCount() != 2 ||
		!strings.Contains(resultJSON, `"kind":"func"`) ||
		!strings.Contains(resultJSON, `"diff_kind":"modify"`) ||
		!strings.Contains(resultJSON, "-func Foo") {
		t.Fatalf("delete_symbol should return host-bound diff metadata and continue: result=%s body=%s", resultJSON, body)
	}
}

func TestRuntimeServerDeleteSymbolRequiresReadBeforeDelete(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(workspace, "example.go")
	before := "package example\n\nfunc Foo() {}\n"
	if err := os.WriteFile(sourcePath, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	deleteArgs := mustJSONString(t, map[string]any{"path": "example.go", "name": "Foo"})
	provider := newCompleteProviderServer(t, [][]string{
		{
			"data: " + mustJSONString(t, map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{{
							"index": 0,
							"id":    "call_delete_symbol_without_read",
							"type":  "function",
							"function": map[string]any{
								"name":      "delete_symbol",
								"arguments": deleteArgs,
							},
						}},
					},
					"finish_reason": "tool_calls",
				}},
			}),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"delete_symbol was blocked"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "delete-symbol-guard-provider", "delete-symbol-guard-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Delete symbol guard",
		"workspace":      workspace,
		"providerId":     "delete-symbol-guard-provider",
		"model":          "delete-symbol-guard-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Delete without reading.",
	}), http.StatusAccepted)

	got, err := os.ReadFile(sourcePath)
	if err != nil || string(got) != before {
		t.Fatalf("delete_symbol must not mutate without read, got=%q err=%v", string(got), err)
	}
	body := provider.Body(1)
	_, deleteResult := providerHostToolResultForName(t, body, "delete_symbol")
	resultJSON := string(mustJSON(t, deleteResult))
	if provider.RequestCount() != 2 || !strings.Contains(resultJSON, "read_before_edit_required") {
		t.Fatalf("delete_symbol read-before-edit guard should return a host-bound result and continue: result=%s body=%s", resultJSON, body)
	}
}

func TestRuntimeServerDeleteSymbolRejectsMultiNameSpec(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(workspace, "example.go")
	before := "package example\n\nvar A, B = 1, 2\n"
	if err := os.WriteFile(sourcePath, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	deleteArgs := mustJSONString(t, map[string]any{"path": "example.go", "name": "A", "kind": "var"})
	provider := newCompleteProviderServer(t, [][]string{
		{
			"data: " + mustJSONString(t, map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{
							{
								"index": 0,
								"id":    "call_read_multi_name_symbol",
								"type":  "function",
								"function": map[string]any{
									"name":      "read_file",
									"arguments": mustJSONString(t, map[string]string{"path": "example.go"}),
								},
							},
							{
								"index": 1,
								"id":    "call_delete_multi_name_symbol",
								"type":  "function",
								"function": map[string]any{
									"name":      "delete_symbol",
									"arguments": deleteArgs,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				}},
			}),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"multi-name symbol blocked"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "delete-symbol-multi-name-provider", "delete-symbol-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Delete multi-name symbol",
		"workspace":      workspace,
		"providerId":     "delete-symbol-multi-name-provider",
		"model":          "delete-symbol-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Delete A.",
	}), http.StatusAccepted)

	got, err := os.ReadFile(sourcePath)
	if err != nil || string(got) != before {
		t.Fatalf("delete_symbol multi-name spec must not mutate file, got=%q err=%v", string(got), err)
	}
	body := provider.Body(1)
	_, deleteResult := providerHostToolResultForName(t, body, "delete_symbol")
	resultJSON := string(mustJSON(t, deleteResult))
	if provider.RequestCount() != 2 || !strings.Contains(resultJSON, "multi_name_spec") {
		t.Fatalf("delete_symbol multi-name guard should return a host-bound visible result and continue: result=%s body=%s", resultJSON, body)
	}
}

func TestRuntimeServerEditFileSupportsReasonixStyleMultiEditAliases(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(workspace, "code.go")
	before := "foo bar foo\npackage old\n\nfunc Old() {\n\tOld()\n}\n"
	want := "qux bar qux\npackage new\n\nfunc New() {\n\tNew()\n}\n"
	if err := os.WriteFile(sourcePath, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	editArgs := mustJSONString(t, map[string]any{
		"path": "code.go",
		"edits": []map[string]any{
			{"old_string": "foo", "new_string": "qux", "replace_all": true},
			{"old_string": "package old", "new_string": "package new"},
			{"old_string": "Old", "new_string": "New", "replace_all": true},
		},
	})
	provider := newCompleteProviderServer(t, [][]string{
		{
			"data: " + mustJSONString(t, map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{
							{
								"index": 0,
								"id":    "call_read_multi_edit",
								"type":  "function",
								"function": map[string]any{
									"name":      "read_file",
									"arguments": mustJSONString(t, map[string]string{"path": "code.go"}),
								},
							},
							{
								"index": 1,
								"id":    "call_multi_edit",
								"type":  "function",
								"function": map[string]any{
									"name":      "edit_file",
									"arguments": editArgs,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				}},
			}),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"multi edit complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "multi-edit-provider", "multi-edit-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Multi edit",
		"workspace":      workspace,
		"providerId":     "multi-edit-provider",
		"model":          "multi-edit-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Apply multi edit.",
	}), http.StatusAccepted)
	got, err := os.ReadFile(sourcePath)
	if err != nil || string(got) != want {
		t.Fatalf("edit_file multi edit content mismatch:\n%s\nerr=%v", string(got), err)
	}
	if provider.RequestCount() != 2 {
		t.Fatalf("multi edit should continue provider loop, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	body := provider.Body(1)
	_, editResult := providerHostToolResultForName(t, body, "edit_file")
	resultJSON := string(mustJSON(t, editResult))
	for _, expected := range []string{
		`"replacements":5`,
		`"diff_kind":"modify"`,
		"+qux bar qux",
		"+package new",
		"+func New()",
	} {
		if !strings.Contains(resultJSON, expected) {
			t.Fatalf("host-bound multi edit tool result missing %q: result=%s body=%s", expected, resultJSON, body)
		}
	}
}

func TestRuntimeServerMultiEditToolIsAdvertisedAndExecutes(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(workspace, "code.go")
	before := "foo bar foo\npackage old\n"
	want := "qux bar qux\npackage new\n"
	if err := os.WriteFile(sourcePath, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	editArgs := mustJSONString(t, map[string]any{
		"path": "code.go",
		"edits": []map[string]any{
			{"old_string": "foo", "new_string": "qux", "replace_all": true},
			{"old_string": "package old", "new_string": "package new"},
		},
	})
	provider := newCompleteProviderServer(t, [][]string{
		{
			"data: " + mustJSONString(t, map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{
							{
								"index": 0,
								"id":    "call_read_before_multi_edit",
								"type":  "function",
								"function": map[string]any{
									"name":      "read_file",
									"arguments": mustJSONString(t, map[string]string{"path": "code.go"}),
								},
							},
							{
								"index": 1,
								"id":    "call_reasonix_multi_edit",
								"type":  "function",
								"function": map[string]any{
									"name":      "multi_edit",
									"arguments": editArgs,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				}},
			}),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"multi_edit complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "multi-edit-tool-provider", "multi-edit-tool-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Multi edit tool",
		"workspace":      workspace,
		"providerId":     "multi-edit-tool-provider",
		"model":          "multi-edit-tool-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Apply multi_edit.",
	}), http.StatusAccepted)
	if !containsString(providerRequestToolNames(t, provider.Body(0)), "multi_edit") {
		t.Fatalf("provider tool catalog should advertise multi_edit: %s", provider.Body(0))
	}
	got, err := os.ReadFile(sourcePath)
	if err != nil || string(got) != want {
		t.Fatalf("multi_edit content mismatch:\n%s\nerr=%v", string(got), err)
	}
	body := provider.Body(1)
	_, editResult := providerHostToolResultForName(t, body, "multi_edit")
	resultJSON := string(mustJSON(t, editResult))
	if provider.RequestCount() != 2 ||
		!strings.Contains(resultJSON, `"replacements":3`) ||
		!strings.Contains(resultJSON, `"diff_kind":"modify"`) {
		t.Fatalf("multi_edit should return host-bound diff metadata and continue: result=%s body=%s", resultJSON, body)
	}
}

func TestRuntimeServerEditFileMultiEditIsAtomicOnFailure(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(workspace, "code.go")
	before := "alpha\nbeta\n"
	if err := os.WriteFile(sourcePath, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	editArgs := mustJSONString(t, map[string]any{
		"path": "code.go",
		"edits": []map[string]any{
			{"old_string": "alpha", "new_string": "ALPHA"},
			{"old_string": "missing", "new_string": "MISSING"},
		},
	})
	provider := newCompleteProviderServer(t, [][]string{
		{
			"data: " + mustJSONString(t, map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{
							{
								"index": 0,
								"id":    "call_read_failed_multi_edit",
								"type":  "function",
								"function": map[string]any{
									"name":      "read_file",
									"arguments": mustJSONString(t, map[string]string{"path": "code.go"}),
								},
							},
							{
								"index": 1,
								"id":    "call_failed_multi_edit",
								"type":  "function",
								"function": map[string]any{
									"name":      "edit_file",
									"arguments": editArgs,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				}},
			}),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"multi edit failed visibly"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "failed-multi-edit-provider", "failed-multi-edit-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Failed multi edit",
		"workspace":      workspace,
		"providerId":     "failed-multi-edit-provider",
		"model":          "failed-multi-edit-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Apply failing multi edit.",
	}), http.StatusAccepted)
	got, err := os.ReadFile(sourcePath)
	if err != nil || string(got) != before {
		t.Fatalf("failed multi edit must not mutate file, got=%q err=%v", string(got), err)
	}
	body := provider.Body(1)
	_, editResult := providerHostToolResultForName(t, body, "edit_file")
	resultJSON := string(mustJSON(t, editResult))
	if provider.RequestCount() != 2 ||
		!strings.Contains(resultJSON, "read_before_edit_required") ||
		!strings.Contains(resultJSON, `"cause_code":"old_text_not_found"`) ||
		!strings.Contains(resultJSON, "old_text_not_found") {
		t.Fatalf("failed multi edit should return a host-bound visible error and continue: result=%s body=%s", resultJSON, body)
	}
}

func TestRuntimeServerApprovalAlwaysRequestsApprovalForAutoReadTool(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("always approval note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read","type":"function","function":{"name":"read","arguments":"{\"path\":\"note.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"read approved"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":12,"completion_tokens":2,"total_tokens":14}}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "always-approval-provider", "approval-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Always approval",
		"workspace":      workspace,
		"providerId":     "always-approval-provider",
		"model":          "approval-model",
		"approvalPolicy": "always",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Read only after approval.",
	}), http.StatusAccepted)
	pendingID := stringField(start, "pendingId")
	if start["pendingKind"] != "approval" || !strings.HasPrefix(pendingID, "appr_") {
		t.Fatalf("approvalPolicy=always should request approval even for read tools: %#v", start)
	}
	if provider.RequestCount() != 1 {
		t.Fatalf("read tool must not execute/continue before approval, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/approvals/"+pendingID, DefaultRuntimeToken, mustJSON(t, map[string]string{"decision": "allow"}), http.StatusOK)
	if provider.RequestCount() != 2 {
		t.Fatalf("approved read should continue provider loop, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	body := provider.Body(1)
	_, readResult := providerHostToolResultForName(t, body, "read")
	resultJSON := string(mustJSON(t, readResult))
	if !strings.Contains(resultJSON, "always approval note") {
		t.Fatalf("second provider call should include the host-bound approved read result: result=%s body=%s", resultJSON, body)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if !strings.Contains(replay, `"approvalPolicy":"always"`) ||
		!strings.Contains(replay, "event: approval_requested") ||
		!strings.Contains(replay, "event: approval_resolved") ||
		!strings.Contains(replay, "event: tool_call_finished") {
		t.Fatalf("always approval replay mismatch:\n%s", replay)
	}
}

func TestStaleApprovalGrantRejectedAfterMCPReconnect(t *testing.T) {
	var mcpMu sync.Mutex
	mcpCalls := map[string]int{}
	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode MCP request: %v", err)
		}
		mcpMu.Lock()
		mcpCalls[request.Method]++
		mcpMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": runtimeMCPInitializeResult("approval-epoch")})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"tools": []map[string]any{{
				"name": "lookup", "description": "Lookup docs", "inputSchema": map[string]any{"type": "object", "additionalProperties": false},
				"outputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
				"annotations":  map[string]any{"readOnlyHint": false},
			}}}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "error": map[string]any{"code": -32601, "message": "method not found"}})
		}
	}))
	defer mcpServer.Close()
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "stale-approval")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("must not be read after stale approval\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_stale_approval","type":"function","function":{"name":"mcp__approval-epoch__lookup","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"STALE_APPROVAL_PROVIDER_CONTINUED"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), Host: "127.0.0.1", Port: 0, DataDir: dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "stale-approval-provider", "stale-approval-model"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{"mcpServers": map[string]any{
			"approval-epoch": map[string]any{"transport": "http", "url": mcpServer.URL, "trustScope": "user"},
		}})),
	}))
	defer server.Close()
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Stale approval", "workspace": workspace, "providerId": "stale-approval-provider", "model": "stale-approval-model",
		"approvalPolicy": "always", "sandboxMode": "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken,
		mustJSON(t, map[string]any{"prompt": "Read only after approval."}), http.StatusAccepted)
	if start["status"] != "waiting" || start["pendingKind"] != "approval" {
		t.Fatalf("read did not pause for approval: %#v", start)
	}
	assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/tools?refresh=1", DefaultRuntimeToken, nil, http.StatusOK)
	response := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/approvals/"+stringField(start, "pendingId"), DefaultRuntimeToken,
		mustJSON(t, map[string]string{"decision": "allow"}), http.StatusConflict)
	if response["code"] != "conflict" || response["message"] != "The request conflicts with the current runtime state." {
		t.Fatalf("stale approval was not closed after MCP reconnect: %#v", response)
	}
	responseJSON := string(mustJSON(t, response))
	for _, forbidden := range []string{"turnClosed", "execution_grant_", "turn_security_", "connectionEpoch", "contextDigest"} {
		if strings.Contains(responseJSON, forbidden) {
			t.Fatalf("stale approval conflict exposed internal authority detail %q: %s", forbidden, responseJSON)
		}
	}
	if provider.RequestCount() != 1 {
		t.Fatalf("stale approval resumed provider execution: count=%d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if !strings.Contains(replay, "event: approval_resolved") || !strings.Contains(replay, `"status":"expired"`) ||
		!hasRuntimeServerEvent(runtimeServerEventsForTurn(t, replay, stringField(start, "turnId")), "turn_completed") ||
		!strings.Contains(replay, `"terminalReason":"source_unavailable"`) ||
		!strings.Contains(replay, `"factAnswerAllowed":false`) ||
		strings.Contains(replay, "STALE_APPROVAL_PROVIDER_CONTINUED") {
		t.Fatalf("stale approval did not terminate without provider continuation:\n%s", replay)
	}
	mcpMu.Lock()
	initializeCount := mcpCalls["initialize"]
	mcpMu.Unlock()
	if initializeCount < 2 {
		t.Fatalf("test did not advance MCP connection identity: initialize=%d", initializeCount)
	}
}

func TestRuntimeServerBashApprovalDenyAndAllowControlExecution(t *testing.T) {
	dataDir := t.TempDir()
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_bash_deny","type":"function","function":{"name":"bash","arguments":"{\"command\":\"printf hi > bash.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_bash_allow","type":"function","function":{"name":"bash","arguments":"{\"command\":\"printf hi > bash.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"bash handled"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":3,"total_tokens":14}}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "bash-provider", "bash-model"),
	}))
	t.Cleanup(server.Close)

	runApprovalCase := func(t *testing.T, decision string) string {
		t.Helper()
		workspace := t.TempDir()
		beforeRequests := provider.RequestCount()
		thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
			"title":          "Bash " + decision,
			"workspace":      workspace,
			"providerId":     "bash-provider",
			"model":          "bash-model",
			"approvalPolicy": "on-request",
			"sandboxMode":    "danger-full-access",
		}), http.StatusCreated)
		threadID := stringField(thread, "id")
		start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{"prompt": "Run bash after approval."}), http.StatusAccepted)
		if start["pendingKind"] != "approval" {
			t.Fatalf("bash should request approval: %#v", start)
		}
		pendingID := stringField(start, "pendingId")
		assertLiveJSON(t, server.URL, http.MethodPost, "/v1/approvals/"+pendingID, DefaultRuntimeToken, mustJSON(t, map[string]string{"decision": decision}), http.StatusOK)
		expectedRequests := 2
		if decision == "deny" {
			expectedRequests = 1
		}
		if got := provider.RequestCount() - beforeRequests; got != expectedRequests {
			t.Fatalf("approval %s provider request count mismatch: got=%d want=%d", decision, got, expectedRequests)
		}
		if decision == "deny" {
			replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
			if !strings.Contains(replay, "approval_denied") ||
				!strings.Contains(replay, "no unverified final response was published") ||
				strings.Contains(replay, "bash handled") {
				t.Fatalf("denied bash approval did not end at the fixed host boundary:\n%s", replay)
			}
		}
		return workspace
	}

	deniedWorkspace := runApprovalCase(t, "deny")
	if _, err := os.Stat(filepath.Join(deniedWorkspace, "bash.txt")); !os.IsNotExist(err) {
		t.Fatalf("denied bash approval must not execute command, stat err=%v", err)
	}
	allowedWorkspace := runApprovalCase(t, "allow")
	data, err := os.ReadFile(filepath.Join(allowedWorkspace, "bash.txt"))
	if err != nil || string(data) != "hi" {
		t.Fatalf("allowed bash approval should execute command, data=%q err=%v", string(data), err)
	}
}

func TestRuntimeServerBashReportsExitCodeAndDiagnostics(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_bash_exit","type":"function","function":{"name":"bash","arguments":"{\"command\":\"printf hi; exit 7\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"exit handled"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "bash-exit-provider", "bash-exit-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Bash exit diagnostics",
		"workspace":      workspace,
		"providerId":     "bash-exit-provider",
		"model":          "bash-exit-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "danger-full-access",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Run a command that exits nonzero.",
	}), http.StatusAccepted)
	if provider.RequestCount() != 2 {
		t.Fatalf("bash failure should execute tool and continue provider loop, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	body := provider.Body(1)
	_, bashResult := providerHostToolResultForName(t, body, "bash")
	resultJSON := string(mustJSON(t, bashResult))
	for _, needle := range []string{
		`"status":"failed"`,
		`"exitCode":7`,
		`"timedOut":false`,
		`"output":"hi"`,
		`"outputBytes":2`,
		`"maxOutputBytes":32768`,
		`"durationMs"`,
	} {
		if !strings.Contains(resultJSON, needle) {
			t.Fatalf("host-bound bash continuation missing diagnostic %q: result=%s body=%s", needle, resultJSON, body)
		}
	}
}

func TestRuntimeServerBashTimeoutKillsProcessGroupGrandchild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("covered by taskkill fallback; Windows Job Object parity is tracked separately")
	}
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_bash_timeout","type":"function","function":{"name":"bash","arguments":"{\"command\":\"sleep 20 & echo $! > child.pid; wait\",\"timeout\":1}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"timeout handled"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "bash-timeout-provider", "bash-timeout-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Bash process group timeout",
		"workspace":      workspace,
		"providerId":     "bash-timeout-provider",
		"model":          "bash-timeout-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "danger-full-access",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Run a command that times out with a background child.",
	}), http.StatusAccepted)
	if provider.RequestCount() != 2 {
		t.Fatalf("bash timeout should execute tool and continue provider loop, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	body := provider.Body(1)
	_, bashResult := providerHostToolResultForName(t, body, "bash")
	resultJSON := string(mustJSON(t, bashResult))
	for _, needle := range []string{
		`"status":"timeout"`,
		`"timedOut":true`,
		`"exitCode":-1`,
		`"outputBytes"`,
		`"maxOutputBytes":32768`,
		`"durationMs"`,
	} {
		if !strings.Contains(resultJSON, needle) {
			t.Fatalf("host-bound timeout continuation missing bash diagnostic %q: result=%s body=%s", needle, resultJSON, body)
		}
	}
	pidBytes, err := os.ReadFile(filepath.Join(workspace, "child.pid"))
	if err != nil {
		t.Fatalf("timed out command should have recorded child pid: %v", err)
	}
	childPID := strings.TrimSpace(string(pidBytes))
	if childPID == "" {
		t.Fatalf("child pid file was empty")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if exec.Command("kill", "-0", childPID).Run() != nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("background child process %s survived bash timeout; process group kill did not run", childPID)
}

func TestRuntimeServerSideEffectIntentBlocksSecondEquivalentBashWrite(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	bashArgsFirst := mustJSONString(t, map[string]any{"command": "printf x >> repeat.txt"})
	bashArgsSecond := mustJSONString(t, map[string]any{
		"command": " printf x >> repeat.txt ", "timeout": terminalapp.DefaultBashTimeoutSeconds, "runInBackground": false,
	})
	provider := newCompleteProviderServer(t, [][]string{
		{
			"data: " + mustJSONString(t, map[string]any{"choices": []map[string]any{{
				"delta": map[string]any{"tool_calls": []map[string]any{{
					"index": 0,
					"id":    "call_repeat_write_1",
					"type":  "function",
					"function": map[string]any{
						"name":      "bash",
						"arguments": bashArgsFirst,
					},
				}}},
				"finish_reason": "tool_calls",
			}}}),
			`data: [DONE]`,
		},
		{
			"data: " + mustJSONString(t, map[string]any{"choices": []map[string]any{{
				"delta": map[string]any{"tool_calls": []map[string]any{{
					"index": 0,
					"id":    "call_repeat_write_2",
					"type":  "function",
					"function": map[string]any{
						"name":      "bash",
						"arguments": bashArgsSecond,
					},
				}}},
				"finish_reason": "tool_calls",
			}}}),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"duplicate write blocked"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "repeat-guard-provider", "repeat-guard-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Repeat write guard",
		"workspace":      workspace,
		"providerId":     "repeat-guard-provider",
		"model":          "repeat-guard-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "danger-full-access",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Try repeating the same write command.",
	}), http.StatusAccepted)
	if provider.RequestCount() != 3 {
		t.Fatalf("side-effect gate should continue after blocking the second write, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	data, err := os.ReadFile(filepath.Join(workspace, "repeat.txt"))
	if err != nil || string(data) != "x" {
		t.Fatalf("second equivalent bash write must be blocked before execution, data=%q err=%v", string(data), err)
	}
	body := provider.Body(2)
	bashCallIDs := providerHostToolCallIDsForName(t, body, "bash")
	if len(bashCallIDs) != 2 {
		t.Fatalf("repeat guard provider history lost host-bound bash calls: ids=%#v body=%s", bashCallIDs, body)
	}
	secondCallID := bashCallIDs[1]
	resultJSON := string(mustJSON(t, providerToolResultForCall(t, body, secondCallID)))
	for _, expected := range []string{
		`"code":"side_effect_duplicate"`,
		`"executed":false`,
		`"intentStatus":"closed"`,
		`"factAnswerAllowed":false`,
		`"evidenceAuthority":false`,
	} {
		if !strings.Contains(resultJSON, expected) {
			t.Fatalf("host-bound final provider result missing duplicate block %q: result=%s body=%s", expected, resultJSON, body)
		}
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if !strings.Contains(replay, secondCallID) || !strings.Contains(replay, "side_effect_duplicate") || strings.Contains(replay, "call_repeat_write_2") {
		t.Fatalf("side-effect duplicate result should be visible in replay:\n%s", replay)
	}
}

func TestRuntimeServerAllowsRepeatedReadOnlyObservation(t *testing.T) {
	dataDir := runtimeServerPrivateToolArgumentTempDir(t, "repeated-read")
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join(workspace, "fixture.txt")
	if err := os.WriteFile(fixturePath, []byte("observable\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	readArgs := mustJSONString(t, map[string]string{"path": fixturePath})
	toolTurn := func(callID string) []string {
		return []string{
			"data: " + mustJSONString(t, map[string]any{"choices": []map[string]any{{
				"delta": map[string]any{"tool_calls": []map[string]any{{
					"index": 0,
					"id":    callID,
					"type":  "function",
					"function": map[string]any{
						"name":      "read",
						"arguments": readArgs,
					},
				}}},
				"finish_reason": "tool_calls",
			}}}),
			`data: [DONE]`,
		}
	}
	provider := newCompleteProviderServer(t, [][]string{
		toolTurn("call_read_repeat_1"),
		toolTurn("call_read_repeat_2"),
		toolTurn("call_read_repeat_3"),
		{
			`data: {"choices":[{"delta":{"content":"read repeated ok"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "repeat-read-provider", "repeat-read-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Repeat read-only observation",
		"workspace":      workspace,
		"providerId":     "repeat-read-provider",
		"model":          "repeat-read-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "danger-full-access",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Read the same file repeatedly.",
	}), http.StatusAccepted)
	body := provider.Body(3)
	readCallIDs := providerHostToolCallIDsForName(t, body, "read")
	if len(readCallIDs) != 3 {
		t.Fatalf("read-only history lost host-bound calls: ids=%#v body=%s", readCallIDs, body)
	}
	thirdResultJSON := string(mustJSON(t, providerToolResultForCall(t, body, readCallIDs[2])))
	if provider.RequestCount() != 4 || strings.Contains(thirdResultJSON, "loop_guard") || strings.Contains(thirdResultJSON, "side_effect_duplicate") {
		t.Fatalf("read-only observations should remain repeatable: result=%s body=%s", thirdResultJSON, body)
	}
}

func TestRuntimeServerFailureStormGuardAnnotatesThirdSameToolFailure(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	editArgs := mustJSONString(t, map[string]string{
		"path":       "note.txt",
		"old_string": "missing text",
		"new_string": "replacement",
	})
	toolTurn := func(callID string) []string {
		return []string{
			"data: " + mustJSONString(t, map[string]any{"choices": []map[string]any{{
				"delta": map[string]any{"tool_calls": []map[string]any{{
					"index": 0,
					"id":    callID,
					"type":  "function",
					"function": map[string]any{
						"name":      "edit_file",
						"arguments": editArgs,
					},
				}}},
				"finish_reason": "tool_calls",
			}}}),
			`data: [DONE]`,
		}
	}
	provider := newCompleteProviderServer(t, [][]string{
		toolTurn("call_storm_1"),
		toolTurn("call_storm_2"),
		toolTurn("call_storm_3"),
		{
			`data: {"choices":[{"delta":{"content":"failure storm handled"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "failure-storm-provider", "failure-storm-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Failure storm guard",
		"workspace":      workspace,
		"providerId":     "failure-storm-provider",
		"model":          "failure-storm-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Keep trying the same invalid edit.",
	}), http.StatusAccepted)
	if provider.RequestCount() != 4 {
		t.Fatalf("failure storm guard should continue to final answer after third failed tool result, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	body := provider.Body(3)
	editCallIDs := providerHostToolCallIDsForName(t, body, "edit_file")
	if len(editCallIDs) != 3 {
		t.Fatalf("failure storm provider history lost host-bound edit calls: ids=%#v body=%s", editCallIDs, body)
	}
	resultJSON := string(mustJSON(t, providerToolResultForCall(t, body, editCallIDs[2])))
	for _, expected := range []string{
		`read_before_edit_required`,
		"[loop guard]",
		`"loop_guard":true`,
		`"storm_count":3`,
	} {
		if !strings.Contains(resultJSON, expected) {
			t.Fatalf("host-bound final provider result missing failure storm guard detail %q: result=%s body=%s", expected, resultJSON, body)
		}
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if !strings.Contains(replay, `"stage":"loop_guard"`) ||
		!strings.Contains(replay, `"guardKind":"tool_failure"`) ||
		!strings.Contains(replay, `"toolName":"edit_file"`) {
		t.Fatalf("failure storm guard should be visible in replay:\n%s", replay)
	}
}

func TestRuntimeServerFailureStormGuardResetsAfterSuccessfulTool(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	editArgs := mustJSONString(t, map[string]string{
		"path":       "note.txt",
		"old_string": "missing text",
		"new_string": "replacement",
	})
	editTurn := func(callID string) []string {
		return []string{
			"data: " + mustJSONString(t, map[string]any{"choices": []map[string]any{{
				"delta": map[string]any{"tool_calls": []map[string]any{{
					"index": 0,
					"id":    callID,
					"type":  "function",
					"function": map[string]any{
						"name":      "edit_file",
						"arguments": editArgs,
					},
				}}},
				"finish_reason": "tool_calls",
			}}}),
			`data: [DONE]`,
		}
	}
	writeArgs := mustJSONString(t, map[string]string{"path": "reset.txt", "content": "ok"})
	provider := newCompleteProviderServer(t, [][]string{
		editTurn("call_reset_fail_1"),
		editTurn("call_reset_fail_2"),
		{
			"data: " + mustJSONString(t, map[string]any{"choices": []map[string]any{{
				"delta": map[string]any{"tool_calls": []map[string]any{{
					"index": 0,
					"id":    "call_reset_success",
					"type":  "function",
					"function": map[string]any{
						"name":      "write_file",
						"arguments": writeArgs,
					},
				}}},
				"finish_reason": "tool_calls",
			}}}),
			`data: [DONE]`,
		},
		editTurn("call_reset_fail_3"),
		{
			`data: {"choices":[{"delta":{"content":"failure storm reset"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "failure-reset-provider", "failure-reset-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Failure storm reset",
		"workspace":      workspace,
		"providerId":     "failure-reset-provider",
		"model":          "failure-reset-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Recover between repeated invalid edits.",
	}), http.StatusAccepted)
	if provider.RequestCount() != 5 {
		t.Fatalf("failure storm reset scenario should continue to final answer, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	body := provider.Body(4)
	editCallIDs := providerHostToolCallIDsForName(t, body, "edit_file")
	if len(editCallIDs) != 3 {
		t.Fatalf("failure reset provider history lost host-bound edit calls: ids=%#v body=%s", editCallIDs, body)
	}
	thirdResultJSON := string(mustJSON(t, providerToolResultForCall(t, body, editCallIDs[2])))
	if strings.Contains(thirdResultJSON, "[loop guard]") || strings.Contains(thirdResultJSON, `"loop_guard":true`) {
		t.Fatalf("successful tool should reset failure storm counter before next failed edit: result=%s body=%s", thirdResultJSON, body)
	}
	if data, err := os.ReadFile(filepath.Join(workspace, "reset.txt")); err != nil || string(data) != "ok" {
		t.Fatalf("successful write_file should execute while resetting failure storm counter, data=%q err=%v", string(data), err)
	}
}

func TestRuntimeServerForkAndResumePreserveToolPairingHistory(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("fork resume note"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"note.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"read complete"},"finish_reason":"stop"}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":20,"completion_tokens":2,"total_tokens":22}}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"fork complete"},"finish_reason":"stop"}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":30,"completion_tokens":3,"total_tokens":33}}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"resume complete"},"finish_reason":"stop"}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":31,"completion_tokens":3,"total_tokens":34}}`,
			`data: [DONE]`,
		},
	})
	modelProvidersJSON, connectProviderRegistry := prepareRuntimeServerExplicitProviderRegistryFixture(
		t, dataDir, provider.URL(), "pairing-provider", "pairing-model",
	)
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: modelProvidersJSON,
	}))
	defer server.Close()
	connectProviderRegistry(server.URL)

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":     "Pairing history",
		"workspace": workspace,
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "Read the note.",
		"providerId": "pairing-provider",
		"model":      "pairing-model",
	}), http.StatusAccepted)
	if provider.RequestCount() != 2 {
		t.Fatalf("expected initial tool loop to make two provider calls, got %d", provider.RequestCount())
	}

	fork := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/fork", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"relation": "fork",
		"title":    "Pairing fork",
	}), http.StatusCreated)
	forkID := stringField(fork, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+forkID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "Continue from fork.",
		"providerId": "pairing-provider",
		"model":      "pairing-model",
	}), http.StatusAccepted)
	assertProviderBodyHasValidToolPair(t, provider.Body(2), "Continue from fork.")

	resume := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/sessions/"+threadID+"/resume-thread", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"workspace": workspace,
		"model":     "pairing-model",
	}), http.StatusCreated)
	resumeID := stringField(resume, "thread_id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+resumeID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "Continue from resume.",
		"providerId": "pairing-provider",
		"model":      "pairing-model",
	}), http.StatusAccepted)
	assertProviderBodyHasValidToolPair(t, provider.Body(3), "Continue from resume.")
}

func TestRuntimeServerInterruptedToolCallHistoryIsRepairedForProvider(t *testing.T) {
	run := func(t *testing.T, toolItem map[string]any) string {
		t.Helper()
		dataDir := t.TempDir()
		durableRoot := t.TempDir()
		workspace := filepath.Join(dataDir, "workspace")
		if err := os.MkdirAll(workspace, 0o755); err != nil {
			t.Fatal(err)
		}
		provider := newCompleteProviderServer(t, [][]string{{
			`data: {"choices":[{"delta":{"content":"continued after interrupted tool"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":24,"completion_tokens":4,"total_tokens":28}}`,
			`data: [DONE]`,
		}})
		server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
			RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, Host: "127.0.0.1", Port: 0, DataDir: dataDir,
			ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "interrupted-provider", "interrupted-model"),
		}))
		defer server.Close()

		thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
			"title": "Interrupted tool history", "workspace": workspace,
			"providerId": "interrupted-provider", "model": "interrupted-model",
		}), http.StatusCreated)
		threadID := stringField(thread, "id")
		turnID := "turn_interrupted_1"
		now := time.Now().UTC().Format(time.RFC3339Nano)
		toolItem["turnId"] = turnID
		toolItem["threadId"] = threadID
		toolItem["role"] = "assistant"
		toolItem["status"] = "running"
		toolItem["kind"] = "tool_call"
		toolItem["toolName"] = "read_file"
		toolItem["toolKind"] = "tool_call"
		toolItem["createdAt"] = now
		interruptedTurn := map[string]any{
			"id": turnID, "threadId": threadID, "status": "interrupted", "createdAt": now, "updatedAt": now,
			"items": []any{
				map[string]any{
					"id": "item_interrupted_user", "turnId": turnID, "threadId": threadID, "role": "user",
					"status": "completed", "kind": "user_message", "text": "Read a file before the crash.", "createdAt": now,
				},
				toolItem,
			},
		}
		// Thread creation is complete and no turn is active. Use the canonical
		// atomic store path so this fixture cannot expose a partial public-to-
		// private thread.json replacement to the live handler.
		store, err := newTempDurableEventSessionStore(durableRoot)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.AppendTurnToThread(threadID, interruptedTurn, "interrupted-provider", nil); err != nil {
			t.Fatal(err)
		}
		if _, err := store.PatchThread(threadID, map[string]any{"status": "idle"}); err != nil {
			t.Fatal(err)
		}
		start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
			"prompt": "Continue after interrupted tool call.", "approvalPolicy": "auto",
		}), http.StatusAccepted)
		if !provider.waitForRequestCount(1, runtimeServerPositiveTestTimeout) {
			t.Fatalf("timed out waiting for interrupted history provider request: start=%#v got=%d", start, provider.RequestCount())
		}
		if provider.RequestCount() != 1 {
			t.Fatalf("interrupted history continuation should make one provider request, got %d", provider.RequestCount())
		}
		return provider.Body(0)
	}

	t.Run("corrupt provider identity is omitted", func(t *testing.T) {
		body := run(t, map[string]any{
			"id": "item_interrupted_tool_call", "callId": "call_interrupted",
			"arguments": map[string]any{"path": "lost.txt"},
		})
		if !strings.Contains(body, "Continue after interrupted tool call.") ||
			strings.Contains(body, "call_interrupted") || strings.Contains(body, "lost.txt") ||
			strings.Contains(body, "[no result: the previous turn was interrupted before this tool call completed]") {
			t.Fatalf("corrupt interrupted tool identity must be omitted instead of repaired as authoritative history:\n%s", body)
		}
		var request map[string]any
		if err := json.Unmarshal([]byte(body), &request); err != nil {
			t.Fatal(err)
		}
		for _, raw := range anyList(request["messages"]) {
			message, _ := raw.(map[string]any)
			if stringField(message, "role") == "tool" || len(anyList(message["tool_calls"])) != 0 {
				t.Fatalf("corrupt interrupted call created a synthetic provider tool pair: %#v", message)
			}
		}
	})

	t.Run("host issued identity receives fixed pairing repair", func(t *testing.T) {
		entropy := bytes.Repeat([]byte{0x69}, domainsecurity.HostToolCallIDEntropyBytesV1)
		hostCallID, err := domainsecurity.NewHostToolCallIDV1(entropy)
		if err != nil {
			t.Fatal(err)
		}
		turnID := "turn_interrupted_1"
		body := run(t, map[string]any{
			"id": domaintoolcall.ToolCallItemIDV1(turnID, hostCallID), "callId": hostCallID,
			"arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1(),
		})
		if !strings.Contains(body, `"id":"`+hostCallID+`"`) ||
			!strings.Contains(body, `"tool_call_id":"`+hostCallID+`"`) ||
			!strings.Contains(body, "[no result: the previous turn was interrupted before this tool call completed]") ||
			!strings.Contains(body, "Continue after interrupted tool call.") || strings.Contains(body, "lost.txt") {
			t.Fatalf("host-issued interrupted tool history must receive a fixed same-ID placeholder result:\n%s", body)
		}
	})
}

func TestRuntimeServerForkTurnIDTruncatesAndRewritesHistory(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"content":"first reply"}}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"second reply"}}]}`,
			`data: [DONE]`,
		},
	})
	modelProvidersJSON, connectProviderRegistry := prepareRuntimeServerExplicitProviderRegistryFixture(
		t, dataDir, provider.URL(), "fork-provider", "fork-model",
	)
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: modelProvidersJSON,
	}))
	defer server.Close()
	connectProviderRegistry(server.URL)

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Fork target",
		"workspace":  workspace,
		"mode":       "plan",
		"providerId": "fork-provider",
		"model":      "fork-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	firstTurn := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "First prompt.",
		"providerId": "fork-provider",
		"model":      "fork-model",
	}), http.StatusAccepted)
	firstTurnID := stringField(firstTurn, "turnId")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     "Second prompt should not clone.",
		"providerId": "fork-provider",
		"model":      "fork-model",
	}), http.StatusAccepted)
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/goal", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"objective": "Do not inherit stale goal",
	}), http.StatusOK)
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/todos", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"todos": []map[string]any{{"content": "Do not inherit later todos", "status": "pending"}},
	}), http.StatusOK)

	fork := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/fork", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"turnId": firstTurnID,
		"title":  "Fork from first turn",
	}), http.StatusCreated)
	forkID := stringField(fork, "id")
	if fork["mode"] != "agent" || fork["forkedFromTurnCount"] != float64(1) || fork["forkedFromMessageCount"] != float64(1) {
		t.Fatalf("fork should reset mode and count only cloned history: %#v", fork)
	}
	if _, ok := fork["goal"]; ok {
		t.Fatalf("fork must not inherit source goal: %#v", fork["goal"])
	}
	if _, ok := fork["todos"]; ok {
		t.Fatalf("truncated fork must not inherit source todos: %#v", fork["todos"])
	}
	turns, _ := fork["turns"].([]any)
	if len(turns) != 1 {
		t.Fatalf("fork must clone only turns through the requested turnId: %#v", fork)
	}
	clonedTurn, _ := turns[0].(map[string]any)
	if stringField(clonedTurn, "id") != firstTurnID || stringField(clonedTurn, "threadId") != forkID {
		t.Fatalf("fork turn must preserve turn id but rewrite thread id: %#v", clonedTurn)
	}
	items, _ := clonedTurn["items"].([]any)
	if len(items) == 0 {
		t.Fatalf("fork turn must retain cloned items: %#v", clonedTurn)
	}
	for _, rawItem := range items {
		item, _ := rawItem.(map[string]any)
		if stringField(item, "threadId") != forkID {
			t.Fatalf("fork item must rewrite thread id: %#v", item)
		}
		if strings.Contains(fmt.Sprint(item), "Second prompt should not clone.") {
			t.Fatalf("fork item leaked history after requested turn: %#v", item)
		}
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/fork", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"turnId": "turn_missing",
	}), http.StatusNotFound)
}

func TestRuntimeServerUserInputOnlyAppearsWhenModelCallsTool(t *testing.T) {
	dataDir := t.TempDir()
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_input","type":"function","function":{"name":"request_user_input","arguments":"{\"prompt\":\"Pick a path\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"continued after input"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":9,"completion_tokens":3,"total_tokens":12}}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "input-provider", "input-model"),
	}))
	defer server.Close()
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Input loop",
		"workspace":  dataDir,
		"providerId": "input-provider",
		"model":      "input-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{"prompt": "Ask only if needed."}), http.StatusAccepted)
	if start["status"] != "waiting" || start["pendingKind"] != "user_input" {
		t.Fatalf("model-called user_input should pause the turn: %#v", start)
	}
	toolNames := providerRequestToolNames(t, provider.Body(0))
	if !containsString(toolNames, "user_input") || !containsString(toolNames, "request_user_input") {
		t.Fatalf("provider request must advertise both Kun user input gate tool aliases, got %#v in %s", toolNames, provider.Body(0))
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if !strings.Contains(replay, "event: user_input_requested") || !strings.Contains(replay, "Pick a path") {
		t.Fatalf("user input card should appear only from the model tool call:\n%s", replay)
	}
	if !strings.Contains(replay, `"options":[]`) || strings.Contains(replay, "Continue the turn.") {
		t.Fatalf("free-text user input should not invent submit-only options:\n%s", replay)
	}
	pendingID := stringField(start, "pendingId")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/user-inputs/"+pendingID, DefaultRuntimeToken, mustJSON(t, map[string]any{
		"answers": []map[string]string{{"id": "q1", "label": "Choice", "value": "ship-choice-secret"}},
	}), http.StatusOK)
	if provider.RequestCount() != 2 {
		t.Fatalf("user_input resolution should continue the provider loop, got %d", provider.RequestCount())
	}
	if !strings.Contains(provider.Body(1), "ship-choice-secret") {
		t.Fatalf("submitted user_input answers must be returned to the model as the tool result:\n%s", provider.Body(1))
	}
	after := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if !strings.Contains(after, "event: user_input_resolved") ||
		!hasRuntimeServerEvent(runtimeServerEventsForTurn(t, after, stringField(start, "turnId")), "turn_completed") {
		t.Fatalf("user input resolution should complete the turn:\n%s", after)
	}
	if strings.Contains(after, "ship-choice-secret") {
		t.Fatalf("user input answers must not be persisted into replay events:\n%s", after)
	}
}

func TestUserInputCancelNeverResumesProvider(t *testing.T) {
	dataDir := t.TempDir()
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_input_cancel","type":"function","function":{"name":"request_user_input","arguments":"{\"prompt\":\"Pick a path\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"PROVIDER_MUST_NOT_RUN_AFTER_INPUT_CANCEL"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "input-cancel-provider", "input-cancel-model"),
	}))
	defer server.Close()
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Input cancel", "workspace": dataDir, "providerId": "input-cancel-provider", "model": "input-cancel-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Ask and stop if input is cancelled.",
	}), http.StatusAccepted)
	pendingID := stringField(start, "pendingId")
	if start["status"] != "waiting" || start["pendingKind"] != "user_input" || pendingID == "" || provider.RequestCount() != 1 {
		t.Fatalf("user-input cancellation fixture did not pause after one provider call: start=%#v calls=%d", start, provider.RequestCount())
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/user-inputs/"+pendingID, DefaultRuntimeToken, mustJSON(t, map[string]any{
		"cancelled": true,
	}), http.StatusOK)
	if provider.RequestCount() != 1 {
		t.Fatalf("user-input cancel must end at the host boundary without another provider call: %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if !strings.Contains(replay, "input_cancelled") || !strings.Contains(replay, "no unverified final response was published") ||
		strings.Contains(replay, "PROVIDER_MUST_NOT_RUN_AFTER_INPUT_CANCEL") {
		t.Fatalf("user-input cancellation did not publish only the fixed host boundary:\n%s", replay)
	}
}

func TestStaleUserInputGrantRejectedAfterCaseBindingSwitch(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	workspace := filepath.Join(dataDir, "case-user-input-stale")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRuntimeCaseProjectBinding(t, workspace, "case-before-input")
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_stale_input","type":"function","function":{"name":"request_user_input","arguments":"{\"prompt\":\"Choose the next safe step\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"STALE_CASE_INPUT_FABRICATED_FACT_4200000"}}]}`,
			`data: [DONE]`,
		},
	})
	caseSource := newPinnedCaseSourceMCPFixture(t)
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, Host: "127.0.0.1", Port: 0, DataDir: dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "stale-input-provider", "stale-input-model"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{"mcpServers": map[string]any{
			"analytix_funds": caseSource.config,
		}})),
	}))
	defer server.Close()
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Stale case input", "workspace": workspace, "providerId": "stale-input-provider", "model": "stale-input-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken,
		mustJSON(t, map[string]any{"prompt": "Ask me to choose the next safe workflow step."}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	// Exact-current-authority resume rejection remains covered by
	// internal/app/executiongrant.TestApprovalResumeRevalidatesExactCurrentAuthority
	// and internal/app/pendingwork.TestCurrentFullSecurityContextIsRequiredAndStaleCloseIsMonotonic.
	// A DSV1 turn may not mint the pending user-input grant in the first place.
	if provider.RequestCount() != 0 || caseSource.ProbeCount(t) != 0 || caseSource.CallCount(t) != 0 || caseSource.EvidenceReadCount(t) != 0 || stringField(start, "pendingId") != "" || stringField(start, "pendingKind") != "" {
		t.Fatalf("legacy snapshot minted pending user-input authority: start=%#v provider=%d probes=%d tools=%d evidence=%d", start, provider.RequestCount(), caseSource.ProbeCount(t), caseSource.CallCount(t), caseSource.EvidenceReadCount(t))
	}
	assertLegacySnapshotSourceUnavailable(t, server.URL, threadID, turnID)
	writeRuntimeCaseProjectBinding(t, workspace, "case-after-input")
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if strings.Contains(replay, "stale-input-secret") || strings.Contains(replay, "STALE_CASE_INPUT_FABRICATED_FACT_4200000") {
		t.Fatalf("stale case input leaked rejected content through current-case projection:\n%s", replay)
	}
	threadState := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	turn := findRuntimeServerTurn(t, threadState, turnID)
	items, _ := turn["items"].([]any)
	if turn["status"] != "completed" || turn["acceptedFinal"] != nil || len(items) != 0 {
		t.Fatalf("old-epoch boundary remained projected after case rebinding: %#v", turn)
	}
	durableEvents, err := os.ReadFile(filepath.Join(durableRoot, "threads", threadID, "events.jsonl"))
	if err != nil || bytes.Contains(durableEvents, []byte(`"acceptedFinal":`)) ||
		bytes.Contains(durableEvents, []byte(`"finalGateVersion":`)) ||
		bytes.Contains(durableEvents, []byte("stale-input-secret")) || bytes.Contains(durableEvents, []byte("STALE_CASE_INPUT_FABRICATED_FACT_4200000")) {
		t.Fatalf("stale case input durable closure violated public-only storage: err=%v", err)
	}
	publicationSlots := map[string]bool{}
	for _, line := range bytes.Split(bytes.TrimSpace(durableEvents), []byte("\n")) {
		var event map[string]any
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatal(err)
		}
		slot := stringField(event, "publicationSlot")
		if slot == "" {
			continue
		}
		if stringField(event, "threadId") != threadID || stringField(event, "turnId") != turnID ||
			publicationSlots[slot] || stringField(event, "publicationCommitId") == "" ||
			stringField(event, "publicationPayloadDigest") != appturn.AcceptedFinalPublicationPayloadDigest(event) {
			t.Fatal("stale case closure lost exact durable publication binding")
		}
		publicationSlots[slot] = true
		if slot == "assistant-final" {
			item, _ := event["item"].(map[string]any)
			view, err := domainevidence.ParseAcceptedFinalPublicViewV3Value(item["acceptedFinalView"])
			if err != nil || view.Variant != domainevidence.SourceUnavailableAnswer || view.ClaimCount != 0 || view.ReceiptMetadata.Count != 0 {
				t.Fatal("stale case closure lost its claim-free public final")
			}
		}
	}
	if !reflect.DeepEqual(publicationSlots, map[string]bool{"assistant-final": true, "usage": true, "terminal": true}) {
		t.Fatal("stale case closure lost its complete durable publication")
	}
}

func TestRuntimeServerInterruptCancelsPendingUserInputContinuation(t *testing.T) {
	dataDir := t.TempDir()
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_input_abort","type":"function","function":{"name":"request_user_input","arguments":"{\"prompt\":\"Pick after abort\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"should not continue after abort"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "input-abort-provider", "input-abort-model"),
	}))
	defer server.Close()
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Input abort loop",
		"workspace":  dataDir,
		"providerId": "input-abort-provider",
		"model":      "input-abort-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{"prompt": "Ask then abort."}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	pendingID := stringField(start, "pendingId")
	if start["status"] != "waiting" || start["pendingKind"] != "user_input" || !strings.HasPrefix(pendingID, "input_") {
		t.Fatalf("model-called user_input should pause the turn: %#v", start)
	}
	interrupt := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns/"+turnID+"/interrupt", DefaultRuntimeToken, mustJSON(t, map[string]any{}), http.StatusOK)
	if interrupt["status"] != "aborted" || interrupt["cancelled"] != true {
		t.Fatalf("interrupt should abort and cancel pending user input: %#v", interrupt)
	}
	response := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/user-inputs/"+pendingID, DefaultRuntimeToken, mustJSON(t, map[string]any{
		"answers": []map[string]string{{"id": "q1", "label": "Choice", "value": "abort-secret"}},
	}), http.StatusConflict)
	if response["code"] != "gate_continuation_unavailable" && response["code"] != "gate_continuation_terminal_turn" {
		t.Fatalf("aborted user_input continuation should be rejected: %#v", response)
	}
	if provider.RequestCount() != 1 {
		t.Fatalf("aborted user_input must not continue provider loop, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if !strings.Contains(replay, "event: user_input_resolved") ||
		!strings.Contains(replay, `"status":"cancelled"`) ||
		!hasRuntimeServerEvent(runtimeServerEventsForTurn(t, replay, turnID), "turn_aborted") {
		t.Fatalf("interrupt should durably settle pending user_input:\n%s", replay)
	}
	if strings.Contains(replay, "abort-secret") {
		t.Fatalf("rejected user_input answer must not be persisted:\n%s", replay)
	}
}

func TestRuntimeServerUserInputResolutionFailsClosedAfterRestart(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_input_restart","type":"function","function":{"name":"request_user_input","arguments":"{\"prompt\":\"Pick a restart path\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"continued after restarted input"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13}}`,
			`data: [DONE]`,
		},
	})
	newHandler := func() http.Handler {
		return newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
			RuntimeToken:       DefaultRuntimeToken,
			DurableTempDir:     durableRoot,
			Host:               "127.0.0.1",
			Port:               0,
			DataDir:            dataDir,
			ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "input-restart-provider", "input-restart-model"),
		})
	}
	firstHandler := newHandler()
	server := httptest.NewServer(firstHandler)
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Input restart loop",
		"workspace":  dataDir,
		"providerId": "input-restart-provider",
		"model":      "input-restart-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{"prompt": "Ask across restart."}), http.StatusAccepted)
	if start["status"] != "waiting" || start["pendingKind"] != "user_input" {
		t.Fatalf("model-called user_input should pause the turn: %#v", start)
	}
	pendingID := stringField(start, "pendingId")
	server.Close()
	shutdownRuntimeTestHandler(t, firstHandler)

	restarted := httptest.NewServer(newHandler())
	defer restarted.Close()
	response := assertLiveJSON(t, restarted.URL, http.MethodPost, "/v1/user-inputs/"+pendingID, DefaultRuntimeToken, mustJSON(t, map[string]any{
		"answers": []map[string]string{{"id": "q1", "label": "Choice", "value": "restart-choice-secret"}},
	}), http.StatusConflict)
	if response["code"] != "gate_continuation_unavailable" && response["code"] != "gate_continuation_terminal_turn" {
		t.Fatalf("unsigned restarted continuation must fail closed: %#v", response)
	}
	if provider.RequestCount() != 1 {
		t.Fatalf("restarted user_input must not continue the provider loop, got %d", provider.RequestCount())
	}
	replay := liveSSE(t, restarted.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	turnEvents := runtimeServerEventsForTurn(t, replay, stringField(start, "turnId"))
	if (!hasRuntimeServerEvent(turnEvents, "turn_aborted") && !hasRuntimeServerEvent(turnEvents, "turn_failed")) ||
		!strings.Contains(replay, "event: user_input_resolved") || strings.Contains(replay, "continued after restarted input") {
		t.Fatalf("restart must abort the unsigned pending continuation:\n%s", replay)
	}
	if strings.Contains(replay, "restart-choice-secret") {
		t.Fatalf("restarted user input answers must not be persisted into replay events:\n%s", replay)
	}
}

func TestRuntimeServerDisableUserInputRemovesInteractiveToolSchemas(t *testing.T) {
	dataDir := t.TempDir()
	provider := newCompleteProviderServer(t, [][]string{{
		`data: {"choices":[{"delta":{"content":"no input needed"}}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":4,"completion_tokens":3,"total_tokens":7}}`,
		`data: [DONE]`,
	}})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "input-provider", "input-model"),
	}))
	defer server.Close()
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Input disabled",
		"workspace":  dataDir,
		"providerId": "input-provider",
		"model":      "input-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	turn := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":           "Do not ask the user.",
		"disableUserInput": true,
	}), http.StatusAccepted)
	toolNames := providerRequestToolNames(t, provider.Body(0))
	if containsString(toolNames, "user_input") || containsString(toolNames, "request_user_input") {
		t.Fatalf("disableUserInput must remove interactive user-input tools from provider schema, got %#v in %s", toolNames, provider.Body(0))
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if hasRuntimeServerEvent(runtimeServerEventsForTurn(t, replay, stringField(turn, "turnId")), "user_input_requested") {
		t.Fatalf("disableUserInput turn must not create a user input card:\n%s", replay)
	}
}

func TestRuntimeServerThreadSummarySearchAndForkCountsMatchProductContract(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace-search-contract")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{{
		`data: {"choices":[{"delta":{"content":"assistant searchable archive preview"}}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":4,"total_tokens":15}}`,
		`data: [DONE]`,
	}})
	modelProvidersJSON, connectProviderRegistry := prepareRuntimeServerExplicitProviderRegistryFixture(
		t, dataDir, provider.URL(), "summary-provider", "summary-model",
	)
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: modelProvidersJSON,
	}))
	defer server.Close()
	connectProviderRegistry(server.URL)

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Summary baseline",
		"workspace":  workspace,
		"providerId": "summary-provider",
		"model":      "summary-model",
		"mode":       "agent",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Find me by prompt needle.",
	}), http.StatusAccepted)

	full := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	if full["preview"] != "assistant searchable archive preview" ||
		full["messageCount"] != float64(1) ||
		full["turnCount"] != float64(1) {
		t.Fatalf("thread detail must include summary preview/count fields: %#v", full)
	}

	for _, query := range []string{"searchable archive", "workspace-search-contract", "prompt needle", "summary-model"} {
		list := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads?search="+url.QueryEscape(query), DefaultRuntimeToken, nil, http.StatusOK)
		items, _ := list["threads"].([]any)
		if len(items) != 1 || stringField(items[0].(map[string]any), "id") != threadID {
			t.Fatalf("thread search %q should match id/title/preview/workspace/model/prompt surface, got %#v", query, list)
		}
		item, _ := items[0].(map[string]any)
		if item["preview"] != "assistant searchable archive preview" ||
			item["messageCount"] != float64(1) ||
			item["turnCount"] != float64(1) {
			t.Fatalf("thread list summary mismatch for query %q: %#v", query, item)
		}
	}

	fork := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/fork", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"relation": "fork",
		"title":    "Summary fork",
	}), http.StatusCreated)
	if fork["forkedFromMessageCount"] != float64(1) || fork["forkedFromTurnCount"] != float64(1) {
		t.Fatalf("fork response must preserve source message/turn counts: %#v", fork)
	}
	forkList := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads?include_archived=true&search=Summary+baseline", DefaultRuntimeToken, nil, http.StatusOK)
	forkItems, _ := forkList["threads"].([]any)
	foundFork := false
	for _, raw := range forkItems {
		item, _ := raw.(map[string]any)
		if stringField(item, "id") == stringField(fork, "id") {
			foundFork = true
			if item["forkedFromMessageCount"] != float64(1) || item["forkedFromTurnCount"] != float64(1) {
				t.Fatalf("fork list summary must include source counts: %#v", item)
			}
		}
	}
	if !foundFork {
		t.Fatalf("search by forked source title should include fork thread, got %#v", forkList)
	}
}

func TestRuntimeServerApprovalDenyAndAllowControlToolExecution(t *testing.T) {
	dataDir := t.TempDir()
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_write_deny","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"out.txt\",\"content\":\"approved content\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_write_allow","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"out.txt\",\"content\":\"approved content\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"approval handled"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":3,"total_tokens":14}}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            filepath.Join(dataDir, "runtime"),
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "approval-provider", "approval-model"),
	}))
	t.Cleanup(server.Close)

	runApprovalCase := func(t *testing.T, decision string) (string, string) {
		t.Helper()
		workspace := filepath.Join(dataDir, decision)
		if err := os.MkdirAll(workspace, 0o755); err != nil {
			t.Fatal(err)
		}
		beforeRequests := provider.RequestCount()
		thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
			"title":          "Approval " + decision,
			"workspace":      workspace,
			"providerId":     "approval-provider",
			"model":          "approval-model",
			"approvalPolicy": "on-request",
			"sandboxMode":    "workspace-write",
		}), http.StatusCreated)
		threadID := stringField(thread, "id")
		start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{"prompt": "Write only after approval."}), http.StatusAccepted)
		if start["pendingKind"] != "approval" {
			t.Fatalf("write_file should request approval: %#v", start)
		}
		pendingID := stringField(start, "pendingId")
		assertLiveJSON(t, server.URL, http.MethodPost, "/v1/approvals/"+pendingID, DefaultRuntimeToken, mustJSON(t, map[string]string{"decision": decision}), http.StatusOK)
		expectedProviderRequests := 2
		if decision == "deny" {
			expectedProviderRequests = 1
		}
		if got := provider.RequestCount() - beforeRequests; got != expectedProviderRequests {
			t.Fatalf("approval resolution provider count mismatch: decision=%s got=%d want=%d bodies=%#v", decision, got, expectedProviderRequests, provider.Bodies())
		}
		return workspace, liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	}

	deniedWorkspace, deniedReplay := runApprovalCase(t, "deny")
	if _, err := os.Stat(filepath.Join(deniedWorkspace, "out.txt")); !os.IsNotExist(err) {
		t.Fatalf("denied approval must not execute write_file, stat err=%v replay=%s", err, deniedReplay)
	}
	if !strings.Contains(deniedReplay, `"status":"denied"`) || !strings.Contains(deniedReplay, "approval_denied") {
		t.Fatalf("denied replay should record denial and error tool result:\n%s", deniedReplay)
	}

	allowedWorkspace, allowedReplay := runApprovalCase(t, "allow")
	data, err := os.ReadFile(filepath.Join(allowedWorkspace, "out.txt"))
	if err != nil || string(data) != "approved content" {
		t.Fatalf("allowed approval should execute write_file, data=%q err=%v replay=%s", string(data), err, allowedReplay)
	}
	if !strings.Contains(allowedReplay, `"status":"allowed"`) || !strings.Contains(allowedReplay, "event: tool_call_finished") {
		t.Fatalf("allowed replay should record tool execution:\n%s", allowedReplay)
	}
}

func TestRuntimeServerShutdownClosesPausedCaseApprovalThroughFinalEvidenceGate(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "shutdown-case-approval")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRuntimeCaseProjectBinding(t, workspace, "shutdown-case")
	if err := os.WriteFile(filepath.Join(workspace, "input.txt"), []byte("case input"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_shutdown_read","type":"function","function":{"name":"read","arguments":"{\"path\":\"input.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}})
	caseSource := newPinnedCaseSourceMCPFixture(t)
	handler := newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), Host: "127.0.0.1", DataDir: dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "shutdown-case-provider", "shutdown-case-model"),
		MCPConfigJSON: string(mustJSONNoTest(map[string]any{"mcpServers": map[string]any{
			"analytix_funds": caseSource.config,
		}})),
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Shutdown case approval", "workspace": workspace, "providerId": "shutdown-case-provider", "model": "shutdown-case-model",
		"approvalPolicy": "always", "sandboxMode": "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Read input.txt only after approval.",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	// Owned shutdown/finalization waiting remains covered by
	// internal/server.TestRuntimeServerShutdownWaitsForOwnedFinalizationBeforeDisconnectingMCP
	// and internal/app/control.TestControllerShutdownWaitsForOwnedTurnAndRejectsLateAdmission.
	// DSV1 must finish boundary-only without issuing an approval.
	if provider.RequestCount() != 0 || caseSource.ProbeCount(t) != 0 || caseSource.CallCount(t) != 0 || caseSource.EvidenceReadCount(t) != 0 || stringField(start, "pendingId") != "" || stringField(start, "pendingKind") != "" {
		t.Fatalf("legacy snapshot minted shutdown-time approval authority: start=%#v provider=%d probes=%d tools=%d evidence=%d", start, provider.RequestCount(), caseSource.ProbeCount(t), caseSource.CallCount(t), caseSource.EvidenceReadCount(t))
	}
	assertLegacySnapshotSourceUnavailable(t, server.URL, threadID, turnID)
	shutdown, ok := handler.(interface{ Shutdown(context.Context) error })
	if !ok {
		t.Fatal("runtime handler does not expose lifecycle shutdown")
	}
	ctx, cancel := context.WithTimeout(context.Background(), runtimeServerShutdownTestTimeout)
	defer cancel()
	if err := shutdown.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown failed to close paused case turn: %v", err)
	}
	if provider.RequestCount() != 0 {
		t.Fatalf("shutdown reached the provider after legacy quarantine, requests=%d", provider.RequestCount())
	}
	finalThread := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	turn := findRuntimeServerTurn(t, finalThread, turnID)
	if stringField(turn, "status") != "completed" || turn["acceptedFinal"] != nil {
		t.Fatalf("shutdown changed the accepted legacy boundary: %#v", turn)
	}
	assertLegacySnapshotSourceUnavailable(t, server.URL, threadID, turnID)
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if !strings.Contains(replay, "event: accepted_final_batch") || !strings.Contains(replay, `"kind":"turn_completed"`) ||
		strings.Contains(replay, "must not write") || strings.Contains(replay, "event: approval_requested") {
		t.Fatalf("shutdown terminal publication is incomplete:\n%s", replay)
	}
}

func TestRuntimeServerInterruptCancelsPendingApprovalContinuation(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "approval-abort")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_write_abort","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"out.txt\",\"content\":\"must not write\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"should not continue after approval abort"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "approval-abort-provider", "approval-abort-model"),
	}))
	defer server.Close()
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Approval abort loop",
		"workspace":      workspace,
		"providerId":     "approval-abort-provider",
		"model":          "approval-abort-model",
		"approvalPolicy": "on-request",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{"prompt": "Write only if not aborted."}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	pendingID := stringField(start, "pendingId")
	if start["status"] != "waiting" || start["pendingKind"] != "approval" || !strings.HasPrefix(pendingID, "appr_") {
		t.Fatalf("write_file should request approval before interrupt: %#v", start)
	}
	interrupt := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns/"+turnID+"/interrupt", DefaultRuntimeToken, mustJSON(t, map[string]any{}), http.StatusOK)
	if interrupt["status"] != "aborted" || interrupt["cancelled"] != true {
		t.Fatalf("interrupt should abort and cancel pending approval: %#v", interrupt)
	}
	response := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/approvals/"+pendingID, DefaultRuntimeToken, mustJSON(t, map[string]string{"decision": "allow"}), http.StatusConflict)
	if response["code"] != "gate_continuation_unavailable" && response["code"] != "gate_continuation_terminal_turn" {
		t.Fatalf("aborted approval continuation should be rejected: %#v", response)
	}
	if provider.RequestCount() != 1 {
		t.Fatalf("aborted approval must not continue provider loop, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	if _, err := os.Stat(filepath.Join(workspace, "out.txt")); !os.IsNotExist(err) {
		t.Fatalf("aborted approval must not execute write_file, stat err=%v", err)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if !strings.Contains(replay, "event: approval_resolved") ||
		!strings.Contains(replay, `"status":"expired"`) ||
		!hasRuntimeServerEvent(runtimeServerEventsForTurn(t, replay, turnID), "turn_aborted") {
		t.Fatalf("interrupt should durably settle pending approval:\n%s", replay)
	}
}

func TestRuntimeServerApprovalResolutionFailsClosedAfterRestart(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	workspace := filepath.Join(dataDir, "approval-restart")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_write_restart","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"out.txt\",\"content\":\"restart approved content\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"approval continued after restart"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":5,"total_tokens":16}}`,
			`data: [DONE]`,
		},
	})
	newHandler := func() http.Handler {
		return newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
			RuntimeToken:       DefaultRuntimeToken,
			DurableTempDir:     durableRoot,
			Host:               "127.0.0.1",
			Port:               0,
			DataDir:            dataDir,
			ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "approval-restart-provider", "approval-restart-model"),
		})
	}
	firstHandler := newHandler()
	server := httptest.NewServer(firstHandler)
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Approval restart loop",
		"workspace":      workspace,
		"providerId":     "approval-restart-provider",
		"model":          "approval-restart-model",
		"approvalPolicy": "on-request",
		"sandboxMode":    "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{"prompt": "Write only after restarted approval."}), http.StatusAccepted)
	if start["status"] != "waiting" || start["pendingKind"] != "approval" {
		t.Fatalf("write_file should request approval before restart: %#v", start)
	}
	pendingID := stringField(start, "pendingId")
	server.Close()
	shutdownRuntimeTestHandler(t, firstHandler)

	restarted := httptest.NewServer(newHandler())
	defer restarted.Close()
	response := assertLiveJSON(t, restarted.URL, http.MethodPost, "/v1/approvals/"+pendingID, DefaultRuntimeToken, mustJSON(t, map[string]string{"decision": "allow"}), http.StatusConflict)
	if response["code"] != "gate_continuation_unavailable" && response["code"] != "gate_continuation_terminal_turn" {
		t.Fatalf("unsigned restarted approval must fail closed: %#v", response)
	}
	if provider.RequestCount() != 1 {
		t.Fatalf("restarted approval must not continue provider loop, got %d", provider.RequestCount())
	}
	if _, err := os.Stat(filepath.Join(workspace, "out.txt")); !os.IsNotExist(err) {
		t.Fatalf("restarted unsigned approval must not execute write_file: %v", err)
	}
	replay := liveSSE(t, restarted.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	turnEvents := runtimeServerEventsForTurn(t, replay, stringField(start, "turnId"))
	if (!hasRuntimeServerEvent(turnEvents, "turn_aborted") && !hasRuntimeServerEvent(turnEvents, "turn_failed")) ||
		!strings.Contains(replay, "event: approval_resolved") || strings.Contains(replay, "approval continued after restart") {
		t.Fatalf("restart must abort the unsigned pending approval:\n%s", replay)
	}
}

func TestRuntimeServerRestartRepairsCommittedCaseContextBeforeFinalGate(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	workspace := filepath.Join(dataDir, "case-context-repair")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRuntimeCaseProjectBinding(t, workspace, "case-context-repair")
	if err := os.WriteFile(filepath.Join(workspace, "input.txt"), []byte("trusted input"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_repair_read","type":"function","function":{"name":"read","arguments":"{\"path\":\"input.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}})
	caseSource := newPinnedCaseSourceMCPFixture(t)
	newConfig := func() RuntimeServerContractConfig {
		return RuntimeServerContractConfig{
			RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, Host: "127.0.0.1", DataDir: dataDir,
			ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "context-repair-provider", "context-repair-model"),
			MCPConfigJSON: string(mustJSONNoTest(map[string]any{"mcpServers": map[string]any{
				"analytix_funds": caseSource.config,
			}})),
		}
	}
	newHandler := func() http.Handler { return newRuntimeServerProviderReadyTestHandler(t, newConfig()) }
	firstHandler := newHandler()
	first := httptest.NewServer(firstHandler)
	thread := assertLiveJSON(t, first.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Committed context repair", "workspace": workspace, "providerId": "context-repair-provider", "model": "context-repair-model",
		"approvalPolicy": "always", "sandboxMode": "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, first.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Read input.txt only after approval.",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	// The exact missing/corrupt-copy repair algorithm has focused coverage
	// in internal/app/turn.TestCommittedTurnContextRepairsMissingAndCorruptPublicCopies
	// and TestBoundaryOnlyCommittedContextRepairsOnRestart. This integration
	// verifies clean restart stability for the only final a DSV1 source may create;
	// directly mutating its signed primary CAS would test CAS tamper, not repair.
	if provider.RequestCount() != 0 || caseSource.ProbeCount(t) != 0 || caseSource.CallCount(t) != 0 || caseSource.EvidenceReadCount(t) != 0 || stringField(start, "pendingId") != "" || stringField(start, "pendingKind") != "" {
		t.Fatalf("legacy snapshot escaped before context repair: start=%#v provider=%d probes=%d tools=%d evidence=%d", start, provider.RequestCount(), caseSource.ProbeCount(t), caseSource.CallCount(t), caseSource.EvidenceReadCount(t))
	}
	assertLegacySnapshotSourceUnavailable(t, first.URL, threadID, turnID)
	first.Close()
	shutdownRuntimeTestHandler(t, firstHandler)

	threadPath := filepath.Join(durableRoot, "threads", threadID, "thread.json")
	restartedHandler := newHandler()
	restarted := httptest.NewServer(restartedHandler)
	defer restarted.Close()
	finalThread := assertLiveJSON(t, restarted.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	finalTurn := findRuntimeServerTurn(t, finalThread, turnID)
	if status := stringField(finalTurn, "status"); status != "completed" || finalTurn["acceptedFinal"] != nil || stringField(finalTurn, "pendingId") != "" || stringField(finalTurn, "pendingKind") != "" {
		t.Fatalf("restarted legacy boundary was not restored exactly: %#v", finalTurn)
	}
	assertLegacySnapshotSourceUnavailable(t, restarted.URL, threadID, turnID)
	body, err := os.ReadFile(threadPath)
	if err != nil {
		t.Fatal(err)
	}
	rawFinalThread := map[string]any{}
	if err := json.Unmarshal(body, &rawFinalThread); err != nil {
		t.Fatal(err)
	}
	rawFinalTurn := findRuntimeServerTurn(t, rawFinalThread, turnID)
	frozen, frozenErr := domainsecurity.ParseTurnSecurityContext(rawFinalTurn["securityContext"])
	epochState, epochErr := domaincontextepoch.ParseState(rawFinalThread["contextEpochState"])
	if frozenErr != nil || epochErr != nil || frozen.ThreadID != threadID || frozen.TurnID != turnID || epochState.ThreadID != threadID || epochState.AcceptedSnapshot.Epoch != frozen.ContextEpoch {
		t.Fatalf("committed context was not restored exactly: frozen=%#v state=%#v frozenErr=%v epochErr=%v", frozen, epochState, frozenErr, epochErr)
	}
	if provider.RequestCount() != 0 || caseSource.ProbeCount(t) != 0 || caseSource.CallCount(t) != 0 || caseSource.EvidenceReadCount(t) != 0 {
		t.Fatalf("restart context repair resumed provider execution: %d", provider.RequestCount())
	}
}

func TestRuntimeServerUnrecoverableCaseContextQuarantinesOnlyAffectedThread(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	workspace := filepath.Join(dataDir, "case-context-quarantine")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRuntimeCaseProjectBinding(t, workspace, "case-context-quarantine")
	if err := os.WriteFile(filepath.Join(workspace, "input.txt"), []byte("case input"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_quarantine_read","type":"function","function":{"name":"read","arguments":"{\"path\":\"input.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}})
	caseSource := newPinnedCaseSourceMCPFixture(t)
	newConfig := func() RuntimeServerContractConfig {
		return RuntimeServerContractConfig{
			RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, Host: "127.0.0.1", DataDir: dataDir,
			ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "context-quarantine-provider", "context-quarantine-model"),
			MCPConfigJSON: string(mustJSONNoTest(map[string]any{"mcpServers": map[string]any{
				"analytix_funds": caseSource.config,
			}})),
		}
	}
	newHandler := func() http.Handler { return newRuntimeServerContractTestHandler(t, newConfig()) }
	firstHandler := newHandler()
	first := httptest.NewServer(firstHandler)
	thread := assertLiveJSON(t, first.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Quarantined case context", "workspace": workspace, "providerId": "context-quarantine-provider", "model": "context-quarantine-model",
		"approvalPolicy": "always", "sandboxMode": "workspace-write",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, first.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Read input.txt only after approval.",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	// Unrecoverable private-context inventory isolation remains covered by
	// internal/app/casethread.TestRestartInventoryQuarantinesStagedAndTamperedContexts.
	// This HTTP integration asserts that audit-only DSV1 is boundary-only and
	// does not prevent unrelated healthy thread creation after restart.
	if provider.RequestCount() != 0 || caseSource.ProbeCount(t) != 0 || caseSource.CallCount(t) != 0 || caseSource.EvidenceReadCount(t) != 0 || stringField(start, "pendingId") != "" || stringField(start, "pendingKind") != "" {
		t.Fatalf("legacy snapshot escaped isolated quarantine: start=%#v provider=%d probes=%d tools=%d evidence=%d", start, provider.RequestCount(), caseSource.ProbeCount(t), caseSource.CallCount(t), caseSource.EvidenceReadCount(t))
	}
	assertLegacySnapshotSourceUnavailable(t, first.URL, threadID, turnID)
	first.Close()
	shutdownRuntimeTestHandler(t, firstHandler)

	restarted := httptest.NewServer(newHandler())
	defer restarted.Close()
	assertLiveJSON(t, restarted.URL, http.MethodGet, "/health", DefaultRuntimeToken, nil, http.StatusOK)
	assertLiveJSON(t, restarted.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Healthy thread after quarantine", "workspace": t.TempDir(),
	}), http.StatusCreated)
	assertLegacySnapshotSourceUnavailable(t, restarted.URL, threadID, turnID)
	if provider.RequestCount() != 0 || caseSource.ProbeCount(t) != 0 || caseSource.CallCount(t) != 0 || caseSource.EvidenceReadCount(t) != 0 {
		t.Fatalf("restarted legacy quarantine resumed execution: provider=%d probes=%d tools=%d evidence=%d", provider.RequestCount(), caseSource.ProbeCount(t), caseSource.CallCount(t), caseSource.EvidenceReadCount(t))
	}
}

func TestRuntimeServerNormalTurnsDoNotCreateSyntheticApprovalOrUserInputGates(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	server := httptest.NewServer(newRuntimeServerTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   g1.RuntimeToken,
		StartedAt:      g1.StartedAt,
		Routes:         g2.Routes,
		DurableTempDir: t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        t.TempDir(),
	}))
	defer server.Close()

	defaultTurn := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/thr_g2_read/turns",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{"prompt": "Default gated turn.", "approvalPolicy": "on-request", "sandboxMode": "workspace-write"}),
		http.StatusAccepted,
	)
	autoTurn := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/thr_g2_read/turns",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{"prompt": "Auto policy turn.", "approvalPolicy": "auto", "sandboxMode": "workspace-write"}),
		http.StatusAccepted,
	)
	headlessTurn := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/thr_g2_read/turns",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{"prompt": "Headless turn.", "approvalPolicy": "on-request", "sandboxMode": "workspace-write", "disableUserInput": true}),
		http.StatusAccepted,
	)
	readOnlyTurn := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/thr_g2_read/turns",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{"prompt": "Read-only turn.", "approvalPolicy": "on-request", "sandboxMode": "read-only"}),
		http.StatusAccepted,
	)
	replay := liveSSE(t, server.URL, "/v1/threads/thr_g2_read/events?since_seq=0", g1.RuntimeToken, http.StatusOK)
	defaultEvents := runtimeServerEventsForTurn(t, replay, stringField(defaultTurn, "turnId"))
	if hasRuntimeServerEvent(defaultEvents, "approval_requested") || hasRuntimeServerEvent(defaultEvents, "user_input_requested") {
		t.Fatalf("normal on-request turn must not create synthetic approval or user-input gates: %#v", defaultEvents)
	}
	autoEvents := runtimeServerEventsForTurn(t, replay, stringField(autoTurn, "turnId"))
	if hasRuntimeServerEvent(autoEvents, "approval_requested") || hasRuntimeServerEvent(autoEvents, "user_input_requested") {
		t.Fatalf("normal auto turn must not create synthetic approval or user-input gates: %#v", autoEvents)
	}
	headlessEvents := runtimeServerEventsForTurn(t, replay, stringField(headlessTurn, "turnId"))
	if hasRuntimeServerEvent(headlessEvents, "approval_requested") || hasRuntimeServerEvent(headlessEvents, "user_input_requested") {
		t.Fatalf("normal disableUserInput turn must not create synthetic approval or user-input gates: %#v", headlessEvents)
	}
	readOnlyEvents := runtimeServerEventsForTurn(t, replay, stringField(readOnlyTurn, "turnId"))
	if hasRuntimeServerEvent(readOnlyEvents, "approval_requested") || hasRuntimeServerEvent(readOnlyEvents, "user_input_requested") {
		t.Fatalf("normal read-only turn must not create synthetic approval or user-input gates: %#v", readOnlyEvents)
	}
	for _, forbidden := range []string{"Ship", "Choose direction", "event: approval_requested", "event: user_input_requested"} {
		if strings.Contains(replay, forbidden) {
			t.Fatalf("normal turns must not include synthetic gate content %q:\n%s", forbidden, replay)
		}
	}
}

func TestRuntimeServerInternalSubagentLineageUsesExistingGoalContract(t *testing.T) {
	g1 := loadG1Contract(t)
	dataDir := t.TempDir()
	workspace := t.TempDir()
	provider := newCompleteProviderServer(t, [][]string{{
		`data: {"choices":[{"delta":{"content":"Goal task completed."},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	}})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       g1.RuntimeToken,
		StartedAt:          g1.StartedAt,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "lineage-provider", "lineage-model"),
	}))
	defer server.Close()
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", g1.RuntimeToken, mustJSON(t, map[string]any{
		"title": "Live internal goal lineage", "workspace": workspace, "providerId": "lineage-provider", "model": "lineage-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")

	goal := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/goal",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{"objective": "Keep subagent lineage internal"}),
		http.StatusOK,
	)
	parentGoalID := stringField(mapField(t, goal, "goal"), "id")
	if parentGoalID == "" {
		t.Fatalf("goal should expose an internal id for lineage: %#v", goal)
	}
	start := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/turns",
		g1.RuntimeToken,
		mustJSON(t, map[string]any{"prompt": "Run internal child orchestration.", "approvalPolicy": "auto", "disableUserInput": true}),
		http.StatusAccepted,
	)
	turnID := stringField(start, "turnId")
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", g1.RuntimeToken, http.StatusOK)
	events := runtimeServerEventsForTurn(t, replay, turnID)
	stage := firstRuntimeServerPipelineStageWithChild(t, events, "response_received")
	child := mapField(t, stage, "child")
	assertSecurityBoundChildMetadata(t, child, "job-1", "completed", false)
	if provider.RequestCount() != 1 {
		t.Fatalf("expected one real terminal producer call, got %d", provider.RequestCount())
	}
	internalRuns := runtimeServerInternalChildRuns(t, dataDir, threadID)
	if len(internalRuns) != 1 || internalRuns[0].ParentGoalID != parentGoalID || internalRuns[0].ParentThreadID != threadID || internalRuns[0].ParentTurnID != turnID {
		if len(internalRuns) != 1 {
			t.Fatalf("child lineage count=%d", len(internalRuns))
		}
		t.Fatalf("child lineage mismatch: goal=%t thread=%t turn=%t fallback=%t", internalRuns[0].ParentGoalID == parentGoalID, internalRuns[0].ParentThreadID == threadID, internalRuns[0].ParentTurnID == turnID, internalRuns[0].ParentGoalID == "thread_"+threadID)
	}
	for _, forbidden := range []string{"private lineage child answer", "parentGoalId", "childModelExecution", "childModelSource", "childProviderId"} {
		if strings.Contains(replay, forbidden) {
			t.Fatalf("public lineage replay exposed private field %q", forbidden)
		}
	}
	for _, forbidden := range []string{"/v1/workflow", "/v1/workflows", "/v1/create-loop", "/v1/subagents", "/v1/autoresearch", "/v1/mcp-indexer"} {
		response := assertLiveJSON(t, server.URL, http.MethodGet, forbidden, g1.RuntimeToken, nil, http.StatusNotFound)
		if response["code"] != "not_found" {
			t.Fatalf("forbidden top-level route %s must remain hidden: %#v", forbidden, response)
		}
	}
}

func TestRuntimeServerPublicPatchCannotRemoveResearchWorkspace(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	dataDir := t.TempDir()
	server := httptest.NewServer(newRuntimeServerTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   g1.RuntimeToken,
		StartedAt:      g1.StartedAt,
		Routes:         g2.Routes,
		DurableTempDir: t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        dataDir,
	}))
	defer server.Close()

	rejected := assertLiveJSON(
		t,
		server.URL,
		http.MethodPatch,
		"/v1/threads/thr_g2_read",
		g1.RuntimeToken,
		mustJSON(t, map[string]string{"workspace": ""}),
		http.StatusBadRequest,
	)
	if rejected["code"] != "validation_error" || rejected["message"] != "The request did not satisfy the runtime contract." {
		t.Fatalf("public PATCH removed the host-owned research workspace: %#v", rejected)
	}
	if strings.Contains(string(mustJSON(t, rejected)), "workspace is required") {
		t.Fatalf("public validation response exposed internal workspace detail: %#v", rejected)
	}
	if _, err := os.Stat(filepath.Join(dataDir, ".analytix", "autoresearch")); !os.IsNotExist(err) {
		t.Fatalf("research goal without workspace must not write runtime data-dir state: %v", err)
	}
}

func TestRuntimeServerMultiModelTurnsDoNotNarrowToDeepSeek(t *testing.T) {
	g1 := loadG1Contract(t)
	fakeProvider := providerscript.NewScriptedProviderServer()
	defer fakeProvider.Close()
	modelProviders := string(mustJSON(t, map[string]any{
		"defaultProviderId": "deepseek-configured",
		"providers": []map[string]any{
			{
				"id":             "deepseek-configured",
				"apiKey":         "test-provider-key",
				"baseUrl":        fakeProvider.URL + "/deepseek/v1",
				"endpointFormat": "chat_completions",
				"models":         []string{"deepseek-chat"},
				"modelProfiles": map[string]any{
					"deepseek-chat": map[string]any{
						"reasoning": map[string]any{
							"requestProtocol":  "deepseek-chat-completions",
							"supportedEfforts": []string{"off", "high", "max"},
							"defaultEffort":    "high",
						},
					},
				},
			},
			{
				"id":             "openai-configured",
				"apiKey":         "test-provider-key",
				"baseUrl":        fakeProvider.URL + "/openai/v1",
				"endpointFormat": "chat_completions",
				"models":         []string{"gpt-compatible"},
			},
			{
				"id":             "openai-responses-configured",
				"apiKey":         "test-provider-key",
				"baseUrl":        fakeProvider.URL + "/openai/v1",
				"endpointFormat": "responses",
				"models":         []string{"gpt-responses-compatible"},
			},
			{
				"id":             "anthropic-configured",
				"apiKey":         "test-provider-key",
				"baseUrl":        fakeProvider.URL + "/anthropic",
				"endpointFormat": "messages",
				"models":         []string{"claude-compatible"},
			},
			{
				"id":             "custom-configured",
				"apiKey":         "test-provider-key",
				"baseUrl":        fakeProvider.URL + "/custom-endpoint",
				"endpointFormat": "custom_endpoint",
				"models":         []string{"custom-compatible"},
			},
		},
	}))
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       g1.RuntimeToken,
		StartedAt:          g1.StartedAt,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            t.TempDir(),
		ModelProvidersJSON: modelProviders,
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", g1.RuntimeToken, mustJSON(t, map[string]any{
		"workspace": t.TempDir(), "providerId": "deepseek-configured", "model": "deepseek-chat",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")

	cases := []struct {
		providerID            string
		model                 string
		family                string
		providerText          string
		cacheSupported        bool
		cacheHitTokensPresent bool
		cacheHitTokens        float64
		cacheMissTokens       float64
	}{
		{"deepseek-configured", "deepseek-chat", "deepseek", "deepseek hello", true, true, 700, 300},
		{"openai-configured", "gpt-compatible", "openai-compatible", "openai compatible hello", true, true, 300, 100},
		{"openai-responses-configured", "gpt-responses-compatible", "openai-compatible", "openai responses hello", true, true, 240, 120},
		{"anthropic-configured", "claude-compatible", "anthropic-compatible", "anthropic compatible hello", true, true, 1000, 250},
		{"custom-configured", "custom-compatible", "custom_endpoint", "custom endpoint hello", true, true, 200, 100},
	}
	turnIDByProviderID := map[string]string{}
	for _, tc := range cases {
		selectRuntimeServerFixtureProvider(t, server.URL, g1.RuntimeToken, tc.providerID)
		start := assertLiveJSON(
			t,
			server.URL,
			http.MethodPost,
			"/v1/threads/"+threadID+"/turns",
			g1.RuntimeToken,
			mustJSON(t, map[string]string{
				"prompt":     "Run provider " + tc.family,
				"model":      tc.model,
				"providerId": tc.providerID,
			}),
			http.StatusAccepted,
		)
		if start["turnId"] == "" {
			t.Fatalf("turn did not start for %s: %#v", tc.providerID, start)
		}
		turnIDByProviderID[tc.providerID] = stringField(start, "turnId")
	}

	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", g1.RuntimeToken, http.StatusOK)
	telemetryByProviderID := runtimeServerTurnTelemetryByStartedProviderID(t, parseRuntimeServerSSEEvents(t, replay))
	namespaceOwner := map[string]string{}
	for _, tc := range cases {
		expectedTurnID := turnIDByProviderID[tc.providerID]
		matchingTerminalFrames := 0
		for _, frame := range splitSSEFrames(replay) {
			if !strings.Contains(frame, tc.providerText) {
				continue
			}
			eventName := ""
			var topLevel map[string]any
			for _, line := range strings.Split(frame, "\n") {
				line = strings.TrimSpace(line)
				switch {
				case strings.HasPrefix(line, "event:"):
					eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
				case strings.HasPrefix(line, "data:"):
					if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &topLevel); err != nil {
						t.Fatalf("parse provider terminal frame for %s: %v", tc.family, err)
					}
				}
			}
			if eventName != "general_terminal_batch" || stringField(topLevel, "kind") != "general_terminal_batch" ||
				stringField(topLevel, "turnId") != expectedTurnID {
				t.Fatalf("provider draft bypassed or crossed the atomic final publication gate for %s:\n%s", tc.family, frame)
			}
			matchingTerminalFrames++
		}
		if matchingTerminalFrames != 1 {
			diagnostics := []string{}
			for _, event := range runtimeServerEventsForTurn(t, replay, expectedTurnID) {
				diagnostics = append(diagnostics, stringField(event, "kind")+":"+stringField(event, "code")+":"+stringField(event, "reasonCode"))
			}
			t.Fatalf("provider %s terminal frame count = %d, want 1; eventKinds=%v openAIText=%t", tc.family, matchingTerminalFrames, diagnostics, strings.Contains(replay, "openai compatible hello"))
		}
		assertRuntimeServerTypedOrdinaryTerminal(
			t, replay, expectedTurnID, tc.providerText, "provider_ordinary_only",
		)
		telemetry, ok := telemetryByProviderID[tc.providerID]
		if !ok {
			t.Fatalf("replay missing host-bound usage diagnostics for %s: %#v", tc.providerID, telemetryByProviderID)
		}
		if telemetry.Model != tc.model {
			t.Fatalf("usage diagnostics associated with the wrong host-started model for %s: %#v", tc.family, telemetry)
		}
		diagnostics := telemetry.CacheDiagnostics
		if diagnostics["cacheTelemetrySupported"] != tc.cacheSupported {
			t.Fatalf("usage diagnostics mismatch for %s: %#v", tc.family, diagnostics)
		}
		namespaceDigest := stringField(diagnostics, "cacheProviderNamespaceDigest")
		if namespaceDigest == "" {
			t.Fatalf("usage diagnostics missing host-derived cache namespace for %s: %#v", tc.family, diagnostics)
		}
		if owner, exists := namespaceOwner[namespaceDigest]; exists {
			t.Fatalf("provider/model cache namespace collision: %s and %s share %s", owner, tc.providerID+"/"+tc.model, namespaceDigest)
		}
		namespaceOwner[namespaceDigest] = tc.providerID + "/" + tc.model
		_, hasHit := diagnostics["cacheHitTokens"]
		_, hasMiss := diagnostics["cacheMissTokens"]
		if hasHit != tc.cacheHitTokensPresent || hasMiss != tc.cacheHitTokensPresent {
			t.Fatalf("provider-native cache diagnostics mismatch for %s: %#v", tc.family, diagnostics)
		}
		if floatField(t, diagnostics, "cacheHitTokens") != tc.cacheHitTokens ||
			floatField(t, diagnostics, "cacheMissTokens") != tc.cacheMissTokens {
			t.Fatalf("provider-native cache telemetry was associated with the wrong host-started provider/model for %s: %#v", tc.family, diagnostics)
		}
	}
	if strings.Contains(replay, "local-fake-key") {
		t.Fatalf("provider API keys must not be replayed in runtime events:\n%s", replay)
	}
}

func TestRuntimeServerProviderRequestShapesKeepDeepSeekFieldsScoped(t *testing.T) {
	tools := providerscript.DefaultProductionCandidateTools()
	cases := []struct {
		name                       string
		request                    providerpkg.Request
		url                        string
		bodyFields                 []string
		headers                    []string
		allowScopedReasoningFields bool
	}{
		{
			name: "deepseek chat completions",
			request: providerpkg.Request{
				ProviderID:        "deepseek-configured",
				Family:            "deepseek",
				EndpointFormat:    "chat_completions",
				BaseURL:           "https://api.deepseek.example/v1",
				APIKey:            "fake-key",
				Model:             "deepseek-chat",
				ReasoningEffort:   "high",
				ReasoningProtocol: "deepseek-chat-completions",
				Messages:          []providerpkg.Message{{Role: "user", Content: "hello"}},
				Tools:             tools,
			},
			url:        "https://api.deepseek.example/v1/chat/completions",
			bodyFields: []string{"messages", "model", "reasoning_effort", "stream", "stream_options", "thinking", "tools"},
			headers:    []string{"Accept", "Authorization", "Content-Type"},
		},
		{
			name: "openai chat completions",
			request: providerpkg.Request{
				ProviderID:     "openai-configured",
				Family:         "openai-compatible",
				EndpointFormat: "chat_completions",
				BaseURL:        "https://openai.example/v1",
				APIKey:         "fake-key",
				Model:          "gpt-compatible",
				Messages:       []providerpkg.Message{{Role: "user", Content: "hello"}},
				Tools:          tools,
			},
			url:        "https://openai.example/v1/chat/completions",
			bodyFields: []string{"messages", "model", "stream", "stream_options", "tools"},
			headers:    []string{"Accept", "Authorization", "Content-Type"},
		},
		{
			name: "xiaomi mimo scoped reasoning protocol",
			request: providerpkg.Request{
				ProviderID:        "xiaomi-configured",
				Family:            "openai-compatible",
				EndpointFormat:    "chat_completions",
				BaseURL:           "https://api.xiaomimimo.com/v1",
				APIKey:            "fake-key",
				Model:             "mimo-v2.5-pro",
				ReasoningEffort:   "medium",
				ReasoningProtocol: "mimo-chat-completions",
				Messages:          []providerpkg.Message{{Role: "user", Content: "hello"}},
				Tools:             tools,
			},
			url:                        "https://api.xiaomimimo.com/v1/chat/completions",
			bodyFields:                 []string{"messages", "model", "reasoning_effort", "stream", "stream_options", "thinking", "tools"},
			headers:                    []string{"Accept", "Authorization", "Content-Type"},
			allowScopedReasoningFields: true,
		},
		{
			name: "openai responses",
			request: providerpkg.Request{
				ProviderID:     "openai-responses-configured",
				Family:         "openai-compatible",
				EndpointFormat: "responses",
				BaseURL:        "https://openai.example/v1",
				APIKey:         "fake-key",
				Model:          "gpt-responses-compatible",
				Messages:       []providerpkg.Message{{Role: "user", Content: "hello"}},
				Tools:          tools,
			},
			url:        "https://openai.example/v1/responses",
			bodyFields: []string{"input", "model", "stream", "tools"},
			headers:    []string{"Accept", "Authorization", "Content-Type"},
		},
		{
			name: "anthropic messages",
			request: providerpkg.Request{
				ProviderID:     "anthropic-configured",
				Family:         "anthropic-compatible",
				EndpointFormat: "messages",
				BaseURL:        "https://anthropic.example",
				APIKey:         "fake-key",
				Model:          "claude-compatible",
				Messages: []providerpkg.Message{
					{Role: "system", Content: "You are analytix."},
					{Role: "user", Content: "hello"},
				},
				Tools: tools,
			},
			url:        "https://anthropic.example/v1/messages",
			bodyFields: []string{"max_tokens", "messages", "model", "stream", "system", "tools"},
			headers:    []string{"Accept", "Content-Type", "anthropic-version", "x-api-key"},
		},
		{
			name: "custom messages full endpoint",
			request: providerpkg.Request{
				ProviderID:     "custom-configured",
				Family:         "custom_endpoint",
				EndpointFormat: "custom_endpoint",
				BaseURL:        "https://custom.example/messages",
				APIKey:         "fake-key",
				Model:          "custom-compatible",
				Messages: []providerpkg.Message{
					{Role: "system", Content: "You are analytix."},
					{Role: "user", Content: "hello"},
				},
				Tools: tools,
			},
			url:        "https://custom.example/messages",
			bodyFields: []string{"max_tokens", "messages", "model", "stream", "system", "tools"},
			headers:    []string{"Accept", "Authorization", "Content-Type", "anthropic-version", "x-api-key"},
		},
		{
			name: "custom responses full endpoint",
			request: providerpkg.Request{
				ProviderID:     "custom-responses-configured",
				Family:         "custom_endpoint",
				EndpointFormat: "custom_endpoint",
				BaseURL:        "https://custom.example/custom/responses",
				APIKey:         "fake-key",
				Model:          "custom-responses-compatible",
				Messages:       []providerpkg.Message{{Role: "user", Content: "hello"}},
				Tools:          tools,
			},
			url:        "https://custom.example/custom/responses",
			bodyFields: []string{"input", "model", "stream", "tools"},
			headers:    []string{"Accept", "Authorization", "Content-Type"},
		},
		{
			name: "custom chat full endpoint",
			request: providerpkg.Request{
				ProviderID:     "custom-chat-configured",
				Family:         "custom_endpoint",
				EndpointFormat: "custom_endpoint",
				BaseURL:        "https://custom.example/custom/chat/completions",
				APIKey:         "fake-key",
				Model:          "custom-chat-compatible",
				Messages:       []providerpkg.Message{{Role: "user", Content: "hello"}},
				Tools:          tools,
			},
			url:        "https://custom.example/custom/chat/completions",
			bodyFields: []string{"messages", "model", "stream", "stream_options", "tools"},
			headers:    []string{"Accept", "Authorization", "Content-Type"},
		},
	}
	for _, tc := range cases {
		shape, err := providerpkg.BuildRequestShapeForTest(tc.request)
		if err != nil {
			t.Fatalf("%s request shape failed: %v", tc.name, err)
		}
		if shape.RequestURL != tc.url ||
			!sameStringSet(shape.RequestBodyFields, tc.bodyFields) ||
			!sameStringSet(shape.RequestHeaders, tc.headers) {
			t.Fatalf("%s request shape mismatch: %#v", tc.name, shape)
		}
		if tc.request.Family != "deepseek" &&
			!tc.allowScopedReasoningFields &&
			(containsString(shape.RequestBodyFields, "thinking") || containsString(shape.RequestBodyFields, "reasoning_effort")) {
			t.Fatalf("DeepSeek-only request fields leaked into %s: %#v", tc.name, shape)
		}
	}

	fallback := providerpkg.NewRuntimeProviderConfigSet(providerpkg.RuntimeProviderConfigInput{
		DefaultProviderID:     "deepseek",
		DefaultEndpointFormat: "chat_completions",
		DefaultModel:          "deepseek-chat",
	}).TurnConfig("", "")
	if fallback.ProviderID != "deepseek" ||
		fallback.BaseURL != "https://api.deepseek.com" ||
		strings.Contains(fallback.BaseURL, "/deepseek/v1") ||
		fallback.APIKey != "" {
		t.Fatalf("empty baseUrl must use provider preset without fake URL/key: %#v", fallback)
	}
	_, err := providerpkg.BuildRequestShapeForTest(providerpkg.Request{
		ProviderID:     fallback.ProviderID,
		Family:         fallback.Family,
		EndpointFormat: fallback.EndpointFormat,
		BaseURL:        fallback.BaseURL,
		APIKey:         fallback.APIKey,
		Model:          fallback.Model,
		Messages:       []providerpkg.Message{{Role: "user", Content: "hello"}},
	})
	if err == nil || !strings.Contains(err.Error(), "apiKey is required") || strings.Contains(err.Error(), "local-fake-key") {
		t.Fatalf("empty API key must produce an explicit provider configuration error, got %v", err)
	}

	presetProviders := string(mustJSON(t, map[string]any{
		"defaultProviderId": "openai-configured",
		"providers": []map[string]any{
			{"id": "openai-configured", "apiKey": "test-provider-key", "baseUrl": "", "endpointFormat": "chat_completions", "models": []string{"gpt-4.1"}},
			{"id": "anthropic-configured", "apiKey": "test-provider-key", "baseUrl": "", "endpointFormat": "messages", "models": []string{"claude-3-5-sonnet"}},
			{"id": "custom-configured", "apiKey": "test-provider-key", "baseUrl": "", "endpointFormat": "custom_endpoint", "models": []string{"custom-compatible"}},
		},
	}))
	presets := providerpkg.NewRuntimeProviderConfigSet(providerpkg.RuntimeProviderConfigInput{ModelProvidersJSON: presetProviders})
	openAIConfig := presets.TurnConfig("openai-configured", "gpt-4.1")
	if openAIConfig.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("OpenAI-compatible blank baseUrl should use provider preset: %#v", openAIConfig)
	}
	anthropicConfig := presets.TurnConfig("anthropic-configured", "claude-3-5-sonnet")
	if anthropicConfig.BaseURL != "https://api.anthropic.com" {
		t.Fatalf("Anthropic blank baseUrl should use provider preset: %#v", anthropicConfig)
	}
	customConfig := presets.TurnConfig("custom-configured", "custom-compatible")
	if customConfig.BaseURL != "" || customConfig.EndpointFormat != "custom_endpoint" {
		t.Fatalf("custom endpoint without a full URL must stay explicit and fail configuration validation: %#v", customConfig)
	}
}

func TestRuntimeServerCandidateDurableRootHighRiskFailsClosedAndSurvivesRestart(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	root := filepath.Join(t.TempDir(), "analytix-go-runtime-candidate-durable")

	firstHandler := newRuntimeServerTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:         g1.RuntimeToken,
		StartedAt:            g1.StartedAt,
		Routes:               g2.Routes,
		CandidateDurableRoot: root,
		Host:                 "127.0.0.1",
		Port:                 0,
		DataDir:              root,
	})
	server := httptest.NewServer(firstHandler)
	researchWorkspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(researchWorkspace, 0o700); err != nil {
		t.Fatalf("create research workspace: %v", err)
	}
	created := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads",
		g1.RuntimeToken,
		mustJSON(t, map[string]string{"title": "Durable research", "workspace": researchWorkspace, "model": "deepseek-chat"}),
		http.StatusCreated,
	)
	threadID := stringField(created, "id")
	if threadID == "" || threadID == "thr_g2_read" {
		t.Fatalf("candidate durable test requires a fresh host-created thread: %#v", created)
	}
	start := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/turns",
		g1.RuntimeToken,
		mustJSON(t, map[string]string{"prompt": "/goal --research Crash before resolving gates.", "model": "deepseek-chat", "riskIntent": "case"}),
		http.StatusAccepted,
	)
	turnID, _ := start["turnId"].(string)
	if turnID != "turn_1" {
		t.Fatalf("first turn id mismatch before restart: %#v", start)
	}
	replayBefore := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", g1.RuntimeToken, http.StatusOK)
	if !strings.Contains(replayBefore, domainevidence.CaseSourceUnavailableText) ||
		!strings.Contains(replayBefore, `"variant":"SourceUnavailableAnswer"`) ||
		!strings.Contains(replayBefore, `"coverageStatus":"unavailable"`) ||
		!strings.Contains(replayBefore, `"receiptMetadata":{"citations":[],"count":0`) {
		t.Fatalf("candidate runtime host-policy case turn must persist only the source boundary:\n%s", replayBefore)
	}
	if strings.Contains(replayBefore, domainevidence.AgentSafetyAuthorityUnavailableText) {
		t.Fatalf("candidate runtime with installed host policy was misreported as authority unavailable:\n%s", replayBefore)
	}
	for _, forbidden := range []string{
		"event: autoresearch_state_audit",
		"event: approval_requested",
		"event: user_input_requested",
		"event: mcp_lifecycle_audit",
		"event: goal_evidence_audit",
		"event: general_terminal_batch",
		"Ship",
		"Choose direction",
		"mcp__analytix_local",
		"local-fake-key",
		"D0244",
	} {
		if strings.Contains(replayBefore, forbidden) {
			t.Fatalf("pre-restart normal replay must not include synthetic fixture/gate content %q:\n%s", forbidden, replayBefore)
		}
	}
	server.Close()
	shutdownRuntimeTestHandler(t, firstHandler)

	restarted := httptest.NewServer(newRuntimeServerTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:         g1.RuntimeToken,
		StartedAt:            g1.StartedAt,
		Routes:               g2.Routes,
		CandidateDurableRoot: root,
		Host:                 "127.0.0.1",
		Port:                 0,
		DataDir:              root,
	}))
	defer restarted.Close()

	info := assertLiveJSON(t, restarted.URL, http.MethodGet, "/v1/runtime/info", g1.RuntimeToken, nil, http.StatusOK)
	if _, exists := info["dataDir"]; exists {
		t.Fatalf("runtime info must not expose the candidate data dir: %#v", info)
	}
	storage := mapField(t, info, "storage")
	if storage["configured"] != true || storage["available"] != true {
		t.Fatalf("runtime info must report storage readiness without exposing its path: %#v", info)
	}

	thread := assertLiveJSON(t, restarted.URL, http.MethodGet, "/v1/threads/"+threadID, g1.RuntimeToken, nil, http.StatusOK)
	if jsonIntField(t, thread, "latestSeq") < 1 {
		t.Fatalf("restart should recover highestSeq from events.jsonl: %#v", thread)
	}
	replayAfter := liveSSE(t, restarted.URL, "/v1/threads/"+threadID+"/events?since_seq=0", g1.RuntimeToken, http.StatusOK)
	for _, expected := range []string{
		"event: turn_started",
		`"variant":"SourceUnavailableAnswer"`,
		domainevidence.CaseSourceUnavailableText,
		`"coverageStatus":"unavailable"`,
		`"receiptMetadata":{"citations":[],"count":0`,
		"event: accepted_final_batch",
		`"kind":"turn_completed"`,
	} {
		if !strings.Contains(replayAfter, expected) {
			t.Fatalf("post-restart replay missing %s\n%s", expected, replayAfter)
		}
	}
	if strings.Contains(replayAfter, domainevidence.AgentSafetyAuthorityUnavailableText) {
		t.Fatalf("post-restart installed host policy was misreported as authority unavailable:\n%s", replayAfter)
	}
	if _, err := os.Stat(filepath.Join(researchWorkspace, ".analytix", "autoresearch", threadID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source-unavailable turn must not create AutoResearch state: %v", err)
	}
	for _, forbidden := range []string{
		"event: autoresearch_state_audit",
		"event: approval_requested",
		"event: user_input_requested",
		"event: approval_resolved",
		"event: user_input_resolved",
		"event: mcp_lifecycle_audit",
		"event: goal_evidence_audit",
		"Ship",
		"Choose direction",
		"mcp__analytix_local",
		"local-fake-key",
		"D0244",
	} {
		if strings.Contains(replayAfter, forbidden) {
			t.Fatalf("post-restart normal replay must not include synthetic fixture/gate content %q:\n%s", forbidden, replayAfter)
		}
	}

	second := assertLiveJSON(
		t,
		restarted.URL,
		http.MethodPost,
		"/v1/threads/"+threadID+"/turns",
		g1.RuntimeToken,
		mustJSON(t, map[string]string{"prompt": "Continue after restart.", "model": "deepseek-chat"}),
		http.StatusAccepted,
	)
	if second["turnId"] != "turn_2" {
		t.Fatalf("turn sequence should recover and continue after restart: %#v", second)
	}
}

func liveSSE(t *testing.T, serverURL string, path string, token string, status int) string {
	t.Helper()
	return liveSSEWithHeaders(t, serverURL, path, token, status, nil)
}

func liveSSEWithHeaders(t *testing.T, serverURL string, path string, token string, status int, headers map[string]string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, serverURL+path, nil)
	if err != nil {
		t.Fatalf("build SSE request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != status {
		t.Fatalf("SSE status mismatch: got %d want %d", resp.StatusCode, status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read SSE response: %v", err)
	}
	return string(data)
}

func assertLiveText(t *testing.T, serverURL string, method string, path string, token string, body json.RawMessage, status int) string {
	t.Helper()
	response, data := liveRequest(t, serverURL, method, path, token, body)
	if response.StatusCode != status {
		t.Fatalf("%s %s status mismatch: got %d want %d", method, path, response.StatusCode, status)
	}
	return string(data)
}

func parseRuntimeServerSSEEvents(t *testing.T, replay string) []map[string]any {
	t.Helper()
	events := []map[string]any{}
	for _, frame := range splitSSEFrames(replay) {
		for _, line := range strings.Split(frame, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "" {
				continue
			}
			var event map[string]any
			if err := json.Unmarshal([]byte(payload), &event); err != nil {
				t.Fatalf("invalid SSE JSON payload %q: %v", payload, err)
			}
			if kind := stringField(event, "kind"); kind == "general_terminal_batch" || kind == "accepted_final_batch" {
				nested, ok := event["events"].([]any)
				if !ok || len(nested) == 0 {
					t.Fatalf("atomic terminal batch has no logical events: %#v", event)
				}
				for _, value := range nested {
					nestedEvent, ok := value.(map[string]any)
					if !ok {
						t.Fatalf("atomic terminal batch contains a non-event: %#v", event)
					}
					events = append(events, nestedEvent)
				}
				continue
			}
			events = append(events, event)
		}
	}
	return events
}

func assertRuntimeServerTypedOrdinaryTerminal(
	t *testing.T,
	replay string,
	turnID string,
	expectedText string,
	expectedOrigin string,
) domainordinaryresult.ResultSlotV1 {
	t.Helper()
	if strings.TrimSpace(turnID) == "" {
		t.Fatal("typed ordinary terminal assertion requires a turn id")
	}
	var batch map[string]any
	batchCount := 0
	for _, frame := range splitSSEFrames(replay) {
		for _, line := range strings.Split(frame, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "" {
				continue
			}
			var event map[string]any
			if err := json.Unmarshal([]byte(payload), &event); err != nil {
				t.Fatalf("invalid SSE JSON payload %q: %v", payload, err)
			}
			if stringField(event, "turnId") != turnID {
				continue
			}
			switch stringField(event, "kind") {
			case "general_terminal_batch":
				batch = event
				batchCount++
			case "item_completed", "usage", "turn_completed":
				t.Fatalf("turn %s split terminal event %q outside its atomic batch: %#v", turnID, stringField(event, "kind"), event)
			}
		}
	}
	if batchCount != 1 {
		t.Fatalf("turn %s general terminal batch count=%d want 1:\n%s", turnID, batchCount, replay)
	}
	if stringField(batch, "purpose") != "analytix.general-terminal-delivery-batch/v1" ||
		stringField(batch, "generalTerminalAuthorityKind") != "general_terminal_cas" ||
		stringField(batch, "transportAuthority") != "host_batch_digest_v1" ||
		!domainsecurity.IsSHA256Hex(stringField(batch, "generalTerminalCommitId")) ||
		!domainsecurity.IsSHA256Hex(stringField(batch, "generalTerminalAuthorityDigest")) ||
		boolField(batch, "evidenceAuthority") || boolField(batch, "citationAuthority") || boolField(batch, "factAnswerAllowed") {
		t.Fatalf("turn %s terminal batch lacks closed host authority: %#v", turnID, batch)
	}

	nested := anyList(batch["events"])
	if len(nested) != 3 {
		t.Fatalf("turn %s terminal batch event count=%d want 3: %#v", turnID, len(nested), batch)
	}
	var completed map[string]any
	for _, raw := range nested {
		event, _ := raw.(map[string]any)
		if stringField(event, "kind") == "item_completed" {
			if completed != nil {
				t.Fatalf("turn %s terminal batch contains duplicate item completion: %#v", turnID, batch)
			}
			completed = event
		}
	}
	if completed == nil {
		t.Fatalf("turn %s terminal batch has no item completion: %#v", turnID, batch)
	}
	item := mapField(t, completed, "item")
	slot, err := domainordinaryresult.ParseResultSlotV1(item["ordinaryResult"])
	if err != nil {
		t.Fatalf("turn %s terminal ordinary result is invalid: item=%#v err=%v", turnID, item, err)
	}
	if slot.Text != expectedText || stringField(item, "text") != expectedText || slot.CandidateOrigin != expectedOrigin {
		t.Fatalf("turn %s terminal ordinary result mismatch: slot=%#v item=%#v", turnID, slot, item)
	}
	if domainsecurity.ContainsProtectedCaseFactCandidate(slot.Text) {
		t.Fatalf("turn %s terminal ordinary result retained protected case or PII text: %#v", turnID, slot)
	}
	if strings.Contains(replay, "event: assistant_text_delta") {
		t.Fatalf("turn %s exposed a provider draft outside terminal publication:\n%s", turnID, replay)
	}
	return slot
}

func assertProviderBodyHasValidToolPair(t *testing.T, body string, latestPrompt string) {
	t.Helper()
	var request map[string]any
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		t.Fatalf("provider request body is not JSON: %v\n%s", err, body)
	}
	messages, ok := request["messages"].([]any)
	if !ok {
		t.Fatalf("provider request has no chat messages: %s", body)
	}
	pairIndex := -1
	pairCallID := ""
	for index, raw := range messages {
		message, _ := raw.(map[string]any)
		if stringField(message, "role") != "assistant" {
			continue
		}
		calls, _ := message["tool_calls"].([]any)
		if len(calls) == 0 {
			continue
		}
		call, _ := calls[0].(map[string]any)
		function, _ := call["function"].(map[string]any)
		if stringField(function, "name") != "read_file" {
			continue
		}
		callID := stringField(call, "id")
		if !domainsecurity.IsHostToolCallIDV1(callID) {
			t.Fatalf("derived history retained a non-host read_file call id %q: %#v", callID, call)
		}
		var historicalArguments map[string]any
		if err := json.Unmarshal([]byte(stringField(function, "arguments")), &historicalArguments); err != nil || len(historicalArguments) != 0 {
			t.Fatalf("derived history must withhold exact source arguments with an empty provider placeholder: call=%#v err=%v", call, err)
		}
		if index+1 >= len(messages) {
			t.Fatalf("assistant tool call was not followed by a tool result: %#v", messages)
		}
		result, _ := messages[index+1].(map[string]any)
		if stringField(result, "role") != "tool" || stringField(result, "tool_call_id") != callID {
			t.Fatalf("tool result did not pair with assistant tool call: %#v", messages[index:index+2])
		}
		var output map[string]any
		if err := json.Unmarshal([]byte(stringField(result, "content")), &output); err != nil {
			t.Fatalf("derived tool result is not closed JSON: result=%#v err=%v", result, err)
		}
		projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(output)
		if err != nil || projection.ProjectionKind != domaintoolresult.ProjectionHostStatus || projection.Status != "completed" ||
			projection.MessageKey != "tool_completed" || projection.Code != "tool_completed" {
			t.Fatalf("derived tool result did not preserve the closed completed projection: output=%#v err=%v", output, err)
		}
		pairIndex = index
		pairCallID = callID
	}
	if pairIndex < 0 {
		t.Fatalf("provider request missing repaired assistant/tool history pair: %s", body)
	}
	if !strings.Contains(body, latestPrompt) {
		t.Fatalf("provider request missing latest prompt %q: %s", latestPrompt, body)
	}
	if strings.Contains(body, "fork resume note") {
		t.Fatalf("derived provider history leaked raw read_file output: %s", body)
	}
	if pairCallID == "" || strings.Contains(body, `"call_read"`) {
		t.Fatalf("derived provider history did not replace the raw provider identity: host=%q body=%s", pairCallID, body)
	}
}

func providerRequestToolNames(t *testing.T, body string) []string {
	t.Helper()
	var request map[string]any
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		t.Fatalf("provider request body is not JSON: %v\n%s", err, body)
	}
	rawTools, _ := request["tools"].([]any)
	names := []string{}
	for _, raw := range rawTools {
		tool, _ := raw.(map[string]any)
		if name := stringField(tool, "name"); name != "" {
			names = append(names, name)
			continue
		}
		function, _ := tool["function"].(map[string]any)
		if name := stringField(function, "name"); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func providerRequestToolParameters(t *testing.T, body string, expectedName string) map[string]any {
	t.Helper()
	var request map[string]any
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		t.Fatalf("provider request body is not JSON: %v\n%s", err, body)
	}
	rawTools, _ := request["tools"].([]any)
	for _, raw := range rawTools {
		tool, _ := raw.(map[string]any)
		if stringField(tool, "name") == expectedName {
			return mapField(t, tool, "parameters")
		}
		function, _ := tool["function"].(map[string]any)
		if stringField(function, "name") == expectedName {
			return mapField(t, function, "parameters")
		}
	}
	t.Fatalf("provider request missing tool %s: %s", expectedName, body)
	return nil
}

func providerRequestToolDescription(t *testing.T, body string, expectedName string) string {
	t.Helper()
	var request map[string]any
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		t.Fatalf("provider request body is not JSON: %v\n%s", err, body)
	}
	rawTools, _ := request["tools"].([]any)
	for _, raw := range rawTools {
		tool, _ := raw.(map[string]any)
		if stringField(tool, "name") == expectedName {
			return stringField(tool, "description")
		}
		function, _ := tool["function"].(map[string]any)
		if stringField(function, "name") == expectedName {
			return stringField(function, "description")
		}
	}
	t.Fatalf("provider request missing tool %s: %s", expectedName, body)
	return ""
}

func assertProviderToolEffortEnum(t *testing.T, body string, toolName string) {
	t.Helper()
	parameters := providerRequestToolParameters(t, body, toolName)
	properties := mapField(t, parameters, "properties")
	effort := mapField(t, properties, "effort")
	assertEffortEnum(t, effort, toolName)
}

func assertProviderParallelTasksEffortEnum(t *testing.T, body string) {
	t.Helper()
	parameters := providerRequestToolParameters(t, body, "parallel_tasks")
	properties := mapField(t, parameters, "properties")
	tasks := mapField(t, properties, "tasks")
	items := mapField(t, tasks, "items")
	taskProperties := mapField(t, items, "properties")
	effort := mapField(t, taskProperties, "effort")
	assertEffortEnum(t, effort, "parallel_tasks.tasks[]")
}

func assertProviderToolProfileProperty(t *testing.T, body string, toolName string) {
	t.Helper()
	parameters := providerRequestToolParameters(t, body, toolName)
	properties := mapField(t, parameters, "properties")
	profile := mapField(t, properties, "profile")
	if profile["type"] != "string" {
		t.Fatalf("%s profile schema type mismatch: %#v", toolName, profile)
	}
}

func assertProviderParallelTasksProfileProperty(t *testing.T, body string) {
	t.Helper()
	parameters := providerRequestToolParameters(t, body, "parallel_tasks")
	properties := mapField(t, parameters, "properties")
	tasks := mapField(t, properties, "tasks")
	items := mapField(t, tasks, "items")
	taskProperties := mapField(t, items, "properties")
	profile := mapField(t, taskProperties, "profile")
	if profile["type"] != "string" {
		t.Fatalf("parallel_tasks.tasks[] profile schema type mismatch: %#v", profile)
	}
}

func assertEffortEnum(t *testing.T, schema map[string]any, label string) {
	t.Helper()
	if schema["type"] != "string" {
		t.Fatalf("%s effort schema type mismatch: %#v", label, schema)
	}
	values := []string{}
	for _, raw := range anyList(schema["enum"]) {
		value, _ := raw.(string)
		if value != "" {
			values = append(values, value)
		}
	}
	if !sameStringSet(values, []string{"off", "low", "medium", "high", "max"}) {
		t.Fatalf("%s effort enum mismatch: %#v", label, schema)
	}
}

func runtimeServerEventsForTurn(t *testing.T, replay string, turnID string) []map[string]any {
	t.Helper()
	events := []map[string]any{}
	for _, event := range parseRuntimeServerSSEEvents(t, replay) {
		if stringField(event, "turnId") == turnID {
			events = append(events, event)
		}
	}
	return events
}

type runtimeServerTurnTelemetry struct {
	TurnID           string
	ProviderID       string
	Model            string
	Usage            map[string]any
	CacheDiagnostics map[string]any
}

func runtimeServerTurnTelemetryByStartedProviderID(t *testing.T, events []map[string]any) map[string]runtimeServerTurnTelemetry {
	t.Helper()
	type startedIdentity struct {
		providerID string
		model      string
	}
	identityByTurnID := map[string]startedIdentity{}
	for _, event := range events {
		if stringField(event, "kind") != "turn_started" {
			continue
		}
		turnID := stringField(event, "turnId")
		identity := startedIdentity{
			providerID: stringField(event, "providerId"),
			model:      stringField(event, "model"),
		}
		if turnID == "" || identity.providerID == "" || identity.model == "" {
			t.Fatalf("turn_started missing host-owned telemetry identity: %#v", event)
		}
		if previous, exists := identityByTurnID[turnID]; exists && previous != identity {
			t.Fatalf("turn_started identity changed for %s: previous=%#v current=%#v", turnID, previous, identity)
		}
		identityByTurnID[turnID] = identity
	}

	telemetryByProviderID := map[string]runtimeServerTurnTelemetry{}
	for _, event := range events {
		if stringField(event, "kind") != "usage" {
			continue
		}
		turnID := stringField(event, "turnId")
		identity, ok := identityByTurnID[turnID]
		if !ok {
			t.Fatalf("usage event has no matching host turn_started identity: %#v", event)
		}
		if usageModel := stringField(event, "model"); usageModel != identity.model {
			t.Fatalf("usage model does not match host turn_started identity for %s: started=%q usage=%q", turnID, identity.model, usageModel)
		}
		diagnostics := mapField(t, event, "cacheDiagnostics")
		if _, exists := diagnostics["providerId"]; exists {
			t.Fatalf("cache diagnostics must not expose providerId; identity belongs to host turn_started: %#v", diagnostics)
		}
		if previous, exists := telemetryByProviderID[identity.providerID]; exists {
			t.Fatalf("multiple usage events share provider %s in host-identity test: previous=%#v current=%#v", identity.providerID, previous, event)
		}
		telemetryByProviderID[identity.providerID] = runtimeServerTurnTelemetry{
			TurnID:           turnID,
			ProviderID:       identity.providerID,
			Model:            identity.model,
			Usage:            mapField(t, event, "usage"),
			CacheDiagnostics: diagnostics,
		}
	}
	return telemetryByProviderID
}

func firstRuntimeServerEvent(t *testing.T, events []map[string]any, kind string) map[string]any {
	t.Helper()
	for _, event := range events {
		if stringField(event, "kind") == kind {
			return event
		}
	}
	t.Fatalf("missing event kind %s in %#v", kind, events)
	return nil
}

func firstRuntimeServerPipelineStage(t *testing.T, events []map[string]any, stage string) map[string]any {
	t.Helper()
	for _, event := range events {
		if stringField(event, "kind") == "pipeline_stage" && stringField(event, "stage") == stage {
			return event
		}
	}
	t.Fatalf("missing pipeline stage %s in %#v", stage, events)
	return nil
}

func firstRuntimeServerPipelineStageWithChild(t *testing.T, events []map[string]any, stage string) map[string]any {
	t.Helper()
	for _, event := range events {
		if stringField(event, "kind") != "pipeline_stage" || stringField(event, "stage") != stage {
			continue
		}
		if _, ok := event["child"].(map[string]any); ok {
			return event
		}
	}
	t.Fatalf("missing pipeline stage %s with child metadata in %#v", stage, events)
	return nil
}

func hasRuntimeServerEvent(events []map[string]any, kind string) bool {
	for _, event := range events {
		if stringField(event, "kind") == kind {
			return true
		}
	}
	return false
}

func runtimeServerEventIndexWithKind(t *testing.T, events []map[string]any, kind string) (int, map[string]any) {
	t.Helper()
	for i, event := range events {
		if stringField(event, "kind") == kind {
			return i, event
		}
	}
	t.Fatalf("missing event kind %s in %#v", kind, events)
	return -1, nil
}

func runtimeServerEventIndexWithKindCallAndStatus(t *testing.T, events []map[string]any, kind string, callID string, status string) (int, map[string]any) {
	t.Helper()
	for i, event := range events {
		if stringField(event, "kind") != kind ||
			stringField(event, "callId") != callID ||
			stringField(event, "status") != status {
			continue
		}
		return i, event
	}
	t.Fatalf("missing event kind %s call %s status %s in %#v", kind, callID, status, events)
	return -1, nil
}

func runtimeServerEventWithChildStatus(t *testing.T, events []map[string]any, status string) map[string]any {
	t.Helper()
	for _, event := range events {
		child, _ := event["child"].(map[string]any)
		if child == nil {
			continue
		}
		if stringField(child, "childStatus") == status {
			return event
		}
	}
	t.Fatalf("missing child status %s in %#v", status, events)
	return nil
}

func runtimeServerEventWithKindAndChildStatus(t *testing.T, events []map[string]any, kind string, status string) map[string]any {
	t.Helper()
	for _, event := range events {
		if stringField(event, "kind") != kind {
			continue
		}
		child, _ := event["child"].(map[string]any)
		if child == nil {
			continue
		}
		if stringField(child, "childStatus") == status {
			return event
		}
	}
	t.Fatalf("missing event kind %s with child status %s in %#v", kind, status, events)
	return nil
}

func findRuntimeServerTurn(t *testing.T, thread map[string]any, turnID string) map[string]any {
	t.Helper()
	turns, ok := thread["turns"].([]any)
	if !ok {
		t.Fatalf("thread has no turns array: %#v", thread)
	}
	for _, raw := range turns {
		turn, _ := raw.(map[string]any)
		if stringField(turn, "id") == turnID {
			return turn
		}
	}
	t.Fatalf("turn %s not found in thread: %#v", turnID, thread)
	return nil
}

func assertSecurityBoundChildMetadata(t *testing.T, child map[string]any, expectedID string, expectedStatus string, expectedBackground bool) {
	t.Helper()
	if child["schemaVersion"] == float64(1) {
		expectedKeys := map[string]struct{}{
			"schemaVersion": {}, "id": {}, "kind": {}, "status": {}, "background": {}, "terminal": {},
			"outputWithheld": {}, "outputTrustStatus": {}, "factAnswerAllowed": {}, "evidenceAuthority": {},
			"canReadOutput": {}, "canContinueParent": {},
		}
		if _, exists := child["active"]; exists {
			expectedKeys["active"] = struct{}{}
			if _, ok := child["active"].(bool); !ok {
				t.Fatalf("task-job active metadata must be boolean: %#v", child)
			}
		}
		if len(child) != len(expectedKeys) {
			t.Fatalf("task-job summary must use the exact closed metadata schema: %#v", child)
		}
		for key := range child {
			if _, ok := expectedKeys[key]; !ok {
				t.Fatalf("task-job summary exposed forbidden field %q: %#v", key, child)
			}
		}
		expectedTerminal := expectedStatus == "completed" || expectedStatus == "failed" || expectedStatus == "killed" ||
			expectedStatus == "aborted" || expectedStatus == "canceled" || expectedStatus == "timeout" || expectedStatus == "interrupted"
		if id := stringField(child, "id"); id == "" || (expectedID != "" && id != expectedID) ||
			stringField(child, "status") != expectedStatus || boolField(child, "background") != expectedBackground ||
			boolField(child, "terminal") != expectedTerminal || !boolField(child, "outputWithheld") ||
			stringField(child, "outputTrustStatus") != "untrusted_child_output" || boolField(child, "factAnswerAllowed") ||
			boolField(child, "evidenceAuthority") || boolField(child, "canReadOutput") || boolField(child, "canContinueParent") {
			t.Fatalf("task-job summary flags mismatch: %#v", child)
		}
		return
	}
	expectedKeys := map[string]struct{}{
		"background": {}, "canContinueParent": {}, "canReadOutput": {}, "childId": {}, "childRunId": {}, "childStatus": {},
		"evidenceAuthority": {}, "factAnswerAllowed": {}, "id": {}, "jobId": {}, "kind": {},
		"outputTrustStatus": {}, "outputWithheld": {}, "status": {}, "terminal": {},
	}
	if _, exists := child["active"]; exists {
		expectedKeys["active"] = struct{}{}
		if _, ok := child["active"].(bool); !ok {
			t.Fatalf("security-bound child active state must be boolean: %#v", child)
		}
	}
	if len(child) != len(expectedKeys) {
		t.Fatalf("security-bound child projection must use the closed metadata allowlist: %#v", child)
	}
	for key := range child {
		if _, ok := expectedKeys[key]; !ok {
			t.Fatalf("security-bound child projection exposed forbidden field %q: %#v", key, child)
		}
	}
	id := stringField(child, "id")
	if id == "" ||
		(expectedID != "" && id != expectedID) ||
		stringField(child, "childId") != id ||
		stringField(child, "childRunId") != id ||
		stringField(child, "jobId") != id {
		t.Fatalf("security-bound child identity mismatch: expected=%q projection=%#v", expectedID, child)
	}
	expectedTerminal := expectedStatus == "completed" || expectedStatus == "failed" || expectedStatus == "killed" ||
		expectedStatus == "aborted" || expectedStatus == "canceled" || expectedStatus == "cancelled" ||
		expectedStatus == "timed_out" || expectedStatus == "interrupted"
	if stringField(child, "kind") != "subagent_task" ||
		stringField(child, "status") != expectedStatus ||
		stringField(child, "childStatus") != expectedStatus ||
		boolField(child, "background") != expectedBackground ||
		boolField(child, "terminal") != expectedTerminal ||
		!boolField(child, "outputWithheld") ||
		stringField(child, "outputTrustStatus") != "untrusted_child_output" ||
		boolField(child, "factAnswerAllowed") ||
		boolField(child, "evidenceAuthority") ||
		boolField(child, "canReadOutput") ||
		boolField(child, "canContinueParent") {
		t.Fatalf("security-bound child projection flags mismatch: %#v", child)
	}
}

func assertSecurityBoundTaskOutputV1(t *testing.T, output map[string]any, expectedID string, expectedStatus string) {
	t.Helper()
	expectedKeys := map[string]struct{}{
		"schemaVersion": {}, "availability": {}, "jobId": {}, "status": {}, "reasonCode": {},
		"outputWithheld": {}, "outputTrustStatus": {}, "factAnswerAllowed": {}, "evidenceAuthority": {},
		"canReadOutput": {}, "canContinueParent": {},
	}
	if len(output) != len(expectedKeys) {
		t.Fatalf("security-bound task output must use the closed V1 withheld schema: %#v", output)
	}
	for key := range output {
		if _, ok := expectedKeys[key]; !ok {
			t.Fatalf("security-bound task output exposed forbidden field %q: %#v", key, output)
		}
	}
	if output["schemaVersion"] != float64(1) || stringField(output, "availability") != "withheld" ||
		stringField(output, "jobId") != expectedID || stringField(output, "status") != expectedStatus ||
		stringField(output, "reasonCode") != "security_bound_child_output" || !boolField(output, "outputWithheld") ||
		stringField(output, "outputTrustStatus") != "untrusted_child_output" || boolField(output, "factAnswerAllowed") ||
		boolField(output, "evidenceAuthority") || boolField(output, "canReadOutput") || boolField(output, "canContinueParent") {
		t.Fatalf("security-bound task output V1 mismatch: %#v", output)
	}
}

func assertClosedTurnFailureResponse(t *testing.T, failure map[string]any, reasonCode string, message string) {
	t.Helper()
	if stringField(failure, "code") != "turn_failed" || stringField(failure, "reasonCode") != reasonCode ||
		stringField(failure, "message") != message {
		t.Fatalf("closed turn failure mismatch: %#v", failure)
	}
	serialized := string(mustJSON(t, failure))
	for _, forbidden := range []string{"reasoning_content", "assistant_reasoning", "SOL_PRIVATE_TRACE_7C", "PRIVATE_REASONING_SENTINEL"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("closed turn failure leaked %q: %s", forbidden, serialized)
		}
	}
}

func assertProviderSecurityBoundToolResult(t *testing.T, body string, callID string, expectedStatus string, expectedBackground bool) map[string]any {
	t.Helper()
	projected := providerToolResultForCall(t, body, callID)
	assertSecurityBoundChildMetadata(t, projected, "", expectedStatus, expectedBackground)
	return projected
}

func providerToolResultForCall(t *testing.T, body string, callID string) map[string]any {
	t.Helper()
	var request map[string]any
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		t.Fatalf("decode provider request: %v body=%s", err, body)
	}
	messages, _ := request["messages"].([]any)
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		if stringField(message, "role") != "tool" || stringField(message, "tool_call_id") != callID {
			continue
		}
		content := stringField(message, "content")
		var projected map[string]any
		if err := json.Unmarshal([]byte(content), &projected); err != nil {
			t.Fatalf("decode security-bound tool result for %s: %v content=%s", callID, err, content)
		}
		return projected
	}
	t.Fatalf("provider request has no tool result for %s: %s", callID, body)
	return nil
}

func providerHostToolCallIDForName(t *testing.T, body string, toolName string) string {
	t.Helper()
	matches := providerHostToolCallIDsForName(t, body, toolName)
	if len(matches) != 1 {
		t.Fatalf("provider request contains %d %s calls, want exactly one: %s", len(matches), toolName, body)
	}
	return matches[0]
}

func providerHostToolCallIDsForName(t *testing.T, body string, toolName string) []string {
	t.Helper()
	var request map[string]any
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		t.Fatalf("decode provider request: %v body=%s", err, body)
	}
	matches := []string{}
	for _, raw := range anyList(request["messages"]) {
		message, _ := raw.(map[string]any)
		if stringField(message, "role") != "assistant" {
			continue
		}
		for _, rawCall := range anyList(message["tool_calls"]) {
			call, _ := rawCall.(map[string]any)
			function, _ := call["function"].(map[string]any)
			if stringField(function, "name") != toolName {
				continue
			}
			callID := stringField(call, "id")
			if !domainsecurity.IsHostToolCallIDV1(callID) {
				t.Fatalf("provider history contains a non-host %s call id %q: %s", toolName, callID, body)
			}
			matches = append(matches, callID)
		}
	}
	if len(matches) == 0 {
		t.Fatalf("provider request has no assistant tool call for %s: %s", toolName, body)
	}
	return matches
}

func providerHostToolResultForName(t *testing.T, body string, toolName string) (string, map[string]any) {
	t.Helper()
	callID := providerHostToolCallIDForName(t, body, toolName)
	if !domainsecurity.IsHostToolCallIDV1(callID) {
		t.Fatalf("provider history contains a non-host %s call id %q: %s", toolName, callID, body)
	}
	return callID, providerToolResultForCall(t, body, callID)
}

func providerHistoryHasToolCallName(body string, toolName string) bool {
	var request map[string]any
	if json.Unmarshal([]byte(body), &request) != nil {
		return false
	}
	for _, raw := range anyList(request["messages"]) {
		message, _ := raw.(map[string]any)
		if stringField(message, "role") != "assistant" {
			continue
		}
		for _, rawCall := range anyList(message["tool_calls"]) {
			call, _ := rawCall.(map[string]any)
			function, _ := call["function"].(map[string]any)
			if stringField(function, "name") == toolName {
				return true
			}
		}
	}
	return false
}

func providerCurrentTurnHostToolResultIDs(body string, toolName string) []string {
	var request map[string]any
	if json.Unmarshal([]byte(body), &request) != nil {
		return nil
	}
	messages := anyList(request["messages"])
	lastUser := -1
	for index, raw := range messages {
		message, _ := raw.(map[string]any)
		if stringField(message, "role") == "user" {
			lastUser = index
		}
	}
	callIDs := []string{}
	resultIDs := map[string]struct{}{}
	for index, raw := range messages {
		if index <= lastUser {
			continue
		}
		message, _ := raw.(map[string]any)
		switch stringField(message, "role") {
		case "assistant":
			for _, rawCall := range anyList(message["tool_calls"]) {
				call, _ := rawCall.(map[string]any)
				function, _ := call["function"].(map[string]any)
				callID := stringField(call, "id")
				if stringField(function, "name") == toolName && domainsecurity.IsHostToolCallIDV1(callID) {
					callIDs = append(callIDs, callID)
				}
			}
		case "tool":
			callID := stringField(message, "tool_call_id")
			if domainsecurity.IsHostToolCallIDV1(callID) {
				resultIDs[callID] = struct{}{}
			}
		}
	}
	paired := make([]string, 0, len(callIDs))
	for _, callID := range callIDs {
		if _, ok := resultIDs[callID]; ok {
			paired = append(paired, callID)
		}
	}
	return paired
}

func providerCurrentTurnHasHostToolResults(body string, toolNames ...string) bool {
	for _, toolName := range toolNames {
		if len(providerCurrentTurnHostToolResultIDs(body, toolName)) == 0 {
			return false
		}
	}
	return len(toolNames) > 0
}

func providerCurrentTurnHostToolResultForName(t *testing.T, body string, toolName string) (string, map[string]any) {
	t.Helper()
	callIDs := providerCurrentTurnHostToolResultIDs(body, toolName)
	if len(callIDs) != 1 {
		t.Fatalf("current provider turn contains %d paired host %s calls, want exactly one: ids=%#v body=%s", len(callIDs), toolName, callIDs, body)
	}
	return callIDs[0], providerToolResultForCall(t, body, callIDs[0])
}

func providerResponsesHostToolResultForName(t *testing.T, body string, toolName string) (string, string) {
	t.Helper()
	var request map[string]any
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		t.Fatalf("decode Responses provider request: %v body=%s", err, body)
	}
	callIDs := []string{}
	outputs := map[string]string{}
	for _, raw := range anyList(request["input"]) {
		item, _ := raw.(map[string]any)
		switch stringField(item, "type") {
		case "function_call":
			if stringField(item, "name") != toolName {
				continue
			}
			callID := stringField(item, "call_id")
			if !domainsecurity.IsHostToolCallIDV1(callID) {
				t.Fatalf("Responses history contains a non-host %s call id %q: %s", toolName, callID, body)
			}
			callIDs = append(callIDs, callID)
		case "function_call_output":
			callID := stringField(item, "call_id")
			if output, ok := item["output"].(string); ok {
				outputs[callID] = output
			} else if item["output"] != nil {
				outputs[callID] = string(mustJSON(t, item["output"]))
			}
		}
	}
	if len(callIDs) != 1 {
		t.Fatalf("Responses request contains %d %s calls, want exactly one: ids=%#v body=%s", len(callIDs), toolName, callIDs, body)
	}
	output, ok := outputs[callIDs[0]]
	if !ok {
		t.Fatalf("Responses request has no function_call_output paired with %s: %s", callIDs[0], body)
	}
	return callIDs[0], output
}

func providerAnthropicHostToolResultForName(t *testing.T, body string, toolName string) (string, string) {
	t.Helper()
	var request map[string]any
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		t.Fatalf("decode Anthropic provider request: %v body=%s", err, body)
	}
	callIDs := []string{}
	outputs := map[string]string{}
	for _, rawMessage := range anyList(request["messages"]) {
		message, _ := rawMessage.(map[string]any)
		role := stringField(message, "role")
		for _, rawBlock := range anyList(message["content"]) {
			block, _ := rawBlock.(map[string]any)
			switch stringField(block, "type") {
			case "tool_use":
				if role != "assistant" || stringField(block, "name") != toolName {
					continue
				}
				callID := stringField(block, "id")
				if !domainsecurity.IsHostToolCallIDV1(callID) {
					t.Fatalf("Anthropic history contains a non-host %s call id %q: %s", toolName, callID, body)
				}
				callIDs = append(callIDs, callID)
			case "tool_result":
				if role != "user" {
					t.Fatalf("Anthropic tool_result is not nested under a user message: %#v", message)
				}
				outputs[stringField(block, "tool_use_id")] = fmt.Sprint(block["content"])
			}
		}
	}
	if len(callIDs) != 1 {
		t.Fatalf("Anthropic request contains %d %s calls, want exactly one: ids=%#v body=%s", len(callIDs), toolName, callIDs, body)
	}
	output, ok := outputs[callIDs[0]]
	if !ok {
		t.Fatalf("Anthropic request has no tool_result paired with %s: %s", callIDs[0], body)
	}
	return callIDs[0], output
}

func runtimeServerScopedChildRuns(t *testing.T, serverURL string, token string, threadID string) []map[string]any {
	t.Helper()
	response := assertLiveJSON(t, serverURL, http.MethodPost, "/v1/runtime/task-jobs/list", token, mustJSON(t, map[string]any{
		"threadId": threadID,
	}), http.StatusOK)
	rawRuns, _ := response["jobs"].([]any)
	runs := make([]map[string]any, 0, len(rawRuns))
	for _, raw := range rawRuns {
		run, _ := raw.(map[string]any)
		if run == nil {
			t.Fatalf("thread-scoped child run is not an object: %#v", raw)
		}
		runs = append(runs, run)
	}
	return runs
}

func runtimeServerInternalChildRuns(t *testing.T, dataDir string, parentThreadID string) []jobs.Record {
	t.Helper()
	runs, err := loadRuntimeServerInternalChildRuns(dataDir, parentThreadID, runtimeServerPositiveTestTimeout)
	if err != nil {
		t.Fatalf("open internal child-run store: %v", err)
	}
	return runs
}

func loadRuntimeServerInternalChildRuns(dataDir string, parentThreadID string, wait time.Duration) ([]jobs.Record, error) {
	root := filepath.Join(dataDir, "child-runs")
	deadline := time.Now().Add(wait)
	observedAtomicWrite := false
	for {
		manager, err := jobs.NewManager(root)
		if err == nil {
			return manager.List(parentThreadID)
		}
		retry, observed := runtimeServerCanRetryInternalChildRunsDuringAtomicWrite(root, err, observedAtomicWrite)
		observedAtomicWrite = observedAtomicWrite || observed
		if wait <= 0 || !time.Now().Before(deadline) || !retry {
			return nil, err
		}
		delay := 10 * time.Millisecond
		if remaining := time.Until(deadline); remaining < delay {
			delay = remaining
		}
		if delay > 0 {
			time.Sleep(delay)
		}
	}
}

const runtimeServerChildRunResidueMigrationError = "child-run storage contains legacy private residue requiring semantic migration"

func runtimeServerCanRetryInternalChildRunsDuringAtomicWrite(
	root string,
	managerErr error,
	observedAtomicWrite bool,
) (bool, bool) {
	residueError := managerErr != nil && managerErr.Error() == runtimeServerChildRunResidueMigrationError
	atomicObservationError := runtimeServerInternalChildRunAtomicObservationError(managerErr)
	if !residueError && !atomicObservationError && !observedAtomicWrite {
		return false, false
	}
	inventory, err := jobs.BuildChildRunInventoryV1(root)
	if err != nil {
		return residueError || atomicObservationError || observedAtomicWrite,
			residueError || atomicObservationError || observedAtomicWrite
	}
	if !inventory.RootExists {
		return false, observedAtomicWrite
	}
	currentAtomicWrite := false
	for _, entry := range inventory.Entries {
		switch entry.Kind {
		case jobs.ChildRunInventoryRecordV1:
		case jobs.ChildRunInventoryTemporaryV1:
			currentAtomicWrite = true
		default:
			return false, observedAtomicWrite
		}
	}
	return residueError || atomicObservationError || observedAtomicWrite || currentAtomicWrite,
		residueError || atomicObservationError || observedAtomicWrite || currentAtomicWrite
}

func runtimeServerInternalChildRunAtomicObservationError(err error) bool {
	if err == nil {
		return false
	}
	switch err.Error() {
	case "child-run inventory object changed while opening",
		"child-run inventory object path was replaced while hashing",
		"child-run inventory root changed during enumeration",
		"child-run inventory changed between stable snapshots":
		return true
	default:
		return false
	}
}

func TestRuntimeServerInternalChildRunsWaitsForCurrentAtomicTemporaryFile(t *testing.T) {
	dataDir := t.TempDir()
	childRunsRoot := filepath.Join(dataDir, "child-runs")
	if err := os.MkdirAll(childRunsRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	temporaryPath := filepath.Join(childRunsRoot, ".job-1.json.tmp-v1-0123456789abcdef0123456789abcdef")
	if err := os.WriteFile(temporaryPath, []byte("pending atomic replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	removed := make(chan error, 1)
	go func() {
		time.Sleep(50 * time.Millisecond)
		removed <- os.Remove(temporaryPath)
	}()

	runs, err := loadRuntimeServerInternalChildRuns(dataDir, "thr-parent", time.Second)
	if err != nil {
		t.Fatalf("wait for current atomic temporary file: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("temporary-only child-run store returned records: %#v", runs)
	}
	if err := <-removed; err != nil {
		t.Fatalf("remove current atomic temporary file: %v", err)
	}
}

func TestRuntimeServerInternalChildRunsRetriesCommittedRecordAtomicReplacement(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "job-1.json"), []byte("stable record\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, message := range []string{
		"child-run inventory object changed while opening",
		"child-run inventory object path was replaced while hashing",
		"child-run inventory root changed during enumeration",
		"child-run inventory changed between stable snapshots",
	} {
		retry, observedAtomicWrite := runtimeServerCanRetryInternalChildRunsDuringAtomicWrite(
			root,
			errors.New(message),
			false,
		)
		if !retry || !observedAtomicWrite {
			t.Fatalf(
				"committed-record atomic replacement %q was not treated as transient: retry=%t observed=%t",
				message,
				retry,
				observedAtomicWrite,
			)
		}
	}
}

func TestRuntimeServerInternalChildRunsRejectsPersistentAtomicTemporaryFile(t *testing.T) {
	dataDir := t.TempDir()
	childRunsRoot := filepath.Join(dataDir, "child-runs")
	if err := os.MkdirAll(childRunsRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	temporaryPath := filepath.Join(childRunsRoot, ".job-1.json.tmp-v1-0123456789abcdef0123456789abcdef")
	if err := os.WriteFile(temporaryPath, []byte("persistent atomic residue"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := loadRuntimeServerInternalChildRuns(dataDir, "thr-parent", 25*time.Millisecond); err == nil ||
		!strings.Contains(err.Error(), "requiring semantic migration") {
		t.Fatalf("persistent atomic temporary file was not rejected: %v", err)
	}
}

func TestRuntimeServerInternalChildRunsDoesNotRetryArtifactResidue(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "job-1.log"), []byte("retired private artifact"), 0o600); err != nil {
		t.Fatal(err)
	}

	retry, observedAtomicWrite := runtimeServerCanRetryInternalChildRunsDuringAtomicWrite(
		root,
		errors.New(runtimeServerChildRunResidueMigrationError),
		false,
	)
	if retry || observedAtomicWrite {
		t.Fatalf("artifact residue was misclassified as a live atomic write: retry=%t observed=%t", retry, observedAtomicWrite)
	}
}

func boolField(record map[string]any, key string) bool {
	value, _ := record[key].(bool)
	return value
}

func containsAnyString(items []any, expected string) bool {
	for _, item := range items {
		if text, ok := item.(string); ok && text == expected {
			return true
		}
	}
	return false
}

func anyList(value any) []any {
	items, _ := value.([]any)
	return items
}

func runtimeToolsProviderDiagnosticByID(t *testing.T, tools map[string]any, id string) map[string]any {
	t.Helper()
	providers, ok := tools["providers"].([]any)
	if !ok {
		t.Fatalf("runtime tools missing provider diagnostics: %#v", tools)
	}
	for _, item := range providers {
		provider, _ := item.(map[string]any)
		if stringField(provider, "id") == id {
			return provider
		}
	}
	t.Fatalf("provider diagnostic %s not found in %#v", id, providers)
	return nil
}

func runtimeServerSkillIDsInclude(items []any, expected ...string) bool {
	seen := map[string]bool{}
	for _, item := range items {
		record, _ := item.(map[string]any)
		id := stringField(record, "id")
		if id != "" {
			seen[id] = true
		}
	}
	for _, id := range expected {
		if !seen[id] {
			return false
		}
	}
	return true
}

func testCheckpointHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func currentRuntimeCheckpointAuthority(t *testing.T, serverURL, runtimeToken, durableRoot, threadID string) domainsecurity.TurnSecurityContext {
	t.Helper()
	started := assertLiveJSON(t, serverURL, http.MethodPost, "/v1/threads/"+threadID+"/turns", runtimeToken, mustJSON(t, map[string]any{
		"prompt": "Reply with ready.",
	}), http.StatusAccepted)
	turnID := stringField(started, "turnId")
	if turnID == "" {
		t.Fatalf("checkpoint authority turn has no id: %#v", started)
	}
	publicThread := assertLiveJSON(t, serverURL, http.MethodGet, "/v1/threads/"+threadID, runtimeToken, nil, http.StatusOK)
	publicTurn := findRuntimeServerTurn(t, publicThread, turnID)
	_, publicThreadAuthorityPresent := publicThread["securityState"]
	_, publicTurnAuthorityPresent := publicTurn["securityContext"]
	if publicThreadAuthorityPresent || publicTurnAuthorityPresent {
		t.Fatalf("private checkpoint authority crossed the public thread seam: thread=%#v turn=%#v", publicThread["securityState"], publicTurn["securityContext"])
	}
	body, err := os.ReadFile(filepath.Join(durableRoot, "threads", threadID, "thread.json"))
	if err != nil {
		t.Fatalf("read private checkpoint authority thread: %v", err)
	}
	thread := map[string]any{}
	if err := json.Unmarshal(body, &thread); err != nil {
		t.Fatalf("decode private checkpoint authority thread: %v", err)
	}
	turn := findRuntimeServerTurn(t, thread, turnID)
	frozen, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || domainsecurity.ValidateTurnSecurityContextForExecution(frozen) != nil {
		t.Fatalf("checkpoint authority turn did not freeze executable V2 context: context=%#v err=%v", frozen, err)
	}
	current, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if err != nil || current != frozen || current.ThreadID != threadID || current.TurnID != turnID {
		t.Fatalf("checkpoint authority turn was not committed as current host authority: current=%#v frozen=%#v err=%v", current, frozen, err)
	}
	return current
}

func seedPrivateCheckpointSnapshot(
	dataDir string,
	access finalauthority.SecurePrivateCASAccessAuthority,
	securityContext domainsecurity.TurnSecurityContext,
	checkpointID, relativePath, beforeHash, afterHash string,
	before map[string]any,
) error {
	encoding, _ := before["encoding"].(string)
	if encoding != "utf8" {
		return errors.New("snapshot encoding is not supported")
	}
	beforeContent, ok := before["content"].(string)
	if !ok {
		return errors.New("snapshot content is invalid")
	}
	if len(beforeContent) > domaincheckpoint.MaxSnapshotContentBytes {
		return fmt.Errorf("snapshot exceeds %d byte private authority limit", domaincheckpoint.MaxSnapshotContentBytes)
	}
	if beforeHash != testCheckpointHash(beforeContent) {
		return errors.New("snapshot before hash is invalid")
	}
	if err := domainsecurity.ValidateTurnSecurityContextForExecution(securityContext); err != nil {
		return fmt.Errorf("checkpoint snapshot context is not executable host authority: %w", err)
	}
	canonicalDataDir, err := filepath.EvalSymlinks(dataDir)
	if err != nil {
		return err
	}
	authorityRoot := filepath.Join(canonicalDataDir, "private", "checkpoint-authority")
	store, err := checkpointauthority.NewStoreContext(context.Background(), authorityRoot, access)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	callID := toolidentity.MustHostToolCallIDV1("runtime-checkpoint:" + checkpointID + "\x00" + relativePath)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "checkpoint-test-provider", ServerIdentity: "host:builtin",
		ToolName: "write_file", ToolCallID: callID,
		ArgsHash:   domainsecurity.CanonicalJSONHash([]byte(fmt.Sprintf(`{"path":%q}`, relativePath))),
		SchemaHash: domainsecurity.SHA256Hex([]byte("checkpoint-test-schema")),
		ScopeHash:  domainsecurity.SHA256Hex([]byte("checkpoint-test-scope")), ReadOnly: false,
		ApprovalState: "approved", IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	probePath := filepath.Join(securityContext.WorkspaceRealPath, ".analytix-checkpoint-authority-probe")
	beforeState, err := (filestore.CheckpointOperationObserver{}).CaptureBefore(context.Background(), securityContext.WorkspaceRealPath, probePath)
	if err != nil {
		return fmt.Errorf("capture production-equivalent checkpoint root authority: %w", err)
	}
	canonicalRelativePath := filepath.ToSlash(relativePath)
	intent, _, _, err := store.BeginOperationGroup(context.Background(), domaincheckpoint.OperationGroupIntentInputV2{
		SecurityContext: securityContext, ExecutionGrant: grant, CheckpointID: checkpointID,
		SourceWorkspaceCheckpointID: "gcp_test_host_checkpoint", ToolName: "write_file",
		ArgumentsJSON: []byte(fmt.Sprintf(`{"path":%q}`, relativePath)),
		Paths: []domaincheckpoint.OperationPathInputV2{{
			ArgumentKey: "path", RequestedPath: relativePath, RelativePath: canonicalRelativePath, Role: "target",
			PathAuthoritySchemaVersion: beforeState.PathAuthority.SchemaVersion,
			AuthorityKind:              beforeState.PathAuthority.Kind,
			AuthorityRoot:              beforeState.PathAuthority.Root,
			AuthorityRootIdentity:      beforeState.PathAuthority.RootIdentity,
			AuthorityRootHash:          beforeState.PathAuthority.RootHash,
			BeforeExisted:              true, BeforeAvailable: true, BeforeHash: beforeHash, BeforeContent: beforeContent,
			ExpectedAfterExisted: true, ExpectedAfterHash: afterHash,
		}},
		CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		return err
	}
	_, err = store.SettleOperationGroup(context.Background(), intent.OperationGroupID, "completed", "mutation_completed", []domaincheckpoint.ObservedOperationPathV2{{
		PathAuthoritySchemaVersion: beforeState.PathAuthority.SchemaVersion,
		AuthorityKind:              beforeState.PathAuthority.Kind,
		AuthorityRootHash:          beforeState.PathAuthority.RootHash,
		RelativePath:               canonicalRelativePath,
		ObservationStatus:          "exact",
		Existed:                    true,
		Hash:                       afterHash,
	}}, now.Add(2*time.Second))
	return err
}

func runtimeMCPInitializeResult(name string, optionalCapabilities ...string) map[string]any {
	capabilities := map[string]any{"tools": map[string]any{}}
	for _, capability := range optionalCapabilities {
		capabilities[capability] = map[string]any{}
	}
	return map[string]any{
		"protocolVersion": "2025-11-25",
		"serverInfo":      map[string]any{"name": name, "version": "1.0.0"},
		"capabilities":    capabilities,
	}
}
