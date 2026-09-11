package stdio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	mcpprotocol "analytix.local/runtime-go/internal/adapters/outbound/mcp/protocol"
	appmcp "analytix.local/runtime-go/internal/app/mcp"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
)

func TestTransportErrorSeparatesOutageFromProtocolCorruption(t *testing.T) {
	for name, test := range map[string]struct {
		err         *TransportError
		unavailable bool
	}{
		"process eof": {
			err: &TransportError{Cause: io.EOF, Retryable: true, InvalidatesIdentity: true}, unavailable: true,
		},
		"malformed frame": {
			err: &TransportError{Cause: errors.New("malformed frame"), Retryable: false, InvalidatesIdentity: true}, unavailable: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if test.err.MCPTransportUnavailable() != test.unavailable || !test.err.MCPTransportInvalidatesIdentity() {
				t.Fatalf("stdio failure classification mismatch: %#v", test.err)
			}
		})
	}
}

func TestMain(m *testing.M) {
	if target := os.Getenv("MCP_CONTAINMENT_READ_TARGET"); target != "" {
		result := "blocked"
		if body, err := os.ReadFile(target); err == nil {
			result = "leaked:" + string(body)
		}
		_ = os.WriteFile(
			os.Getenv("MCP_CONTAINMENT_RESULT_PATH"),
			[]byte(result),
			0o600,
		)
		runProtocolHelper("contained-startup")
		os.Exit(0)
	}
	if os.Getenv("ANALYTIX_MCP_PROCESS_TREE_HELPER") == "1" {
		var command *exec.Cmd
		if runtime.GOOS == "windows" {
			command = exec.Command("cmd", "/c", "ping", "-n", "30", "127.0.0.1")
		} else {
			command = exec.Command("sh", "-c", "printf 'ready\\n'; sleep 60 & wait")
		}
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		_ = command.Run()
		os.Exit(0)
	}
	if mode := os.Getenv("ANALYTIX_MCP_STDIO_PROTOCOL_HELPER"); mode != "" {
		runProtocolHelper(mode)
		os.Exit(0)
	}
	if os.Getenv("ANALYTIX_MCP_CHILD_ENV_HELPER") == "1" {
		if os.Getenv("ANALYTIX_RUNTIME_TOKEN") != "" {
			fmt.Print("leaked")
			os.Exit(91)
		}
		fmt.Print("clean")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestStdioMCPInitializationCannotReadProtectedRoot(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("packaged macOS process containment seam")
	}
	protectedRoot := t.TempDir()
	ordinaryRoot := t.TempDir()
	secret := filepath.Join(protectedRoot, "sentinel.txt")
	if err := os.WriteFile(secret, []byte("must-not-escape"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(ordinaryRoot, "sentinel-link")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{secret, link} {
		resultPath := filepath.Join(ordinaryRoot, fmt.Sprintf("result-%x", len(target)))
		client, err := NewTransportClientWithProtectedReadDirs(
			domainmcp.ServerSpec{
				ID: "stdio-contained", Transport: "stdio", Command: os.Args[0],
				Env: map[string]string{
					"MCP_CONTAINMENT_READ_TARGET": target,
					"MCP_CONTAINMENT_RESULT_PATH": resultPath,
				},
				ExpectedServerName: "stdio-contract", ExpectedServerVersion: "1.0.0",
			},
			[]string{protectedRoot},
		)
		if err != nil {
			t.Fatalf("contained MCP initialization failed: %v", err)
		}
		client.Close()
		body, err := os.ReadFile(resultPath)
		if err != nil || string(body) != "blocked" {
			t.Fatalf("MCP read protected bytes during initialization: result=%q err=%v", body, err)
		}
	}
}

func TestOrdinaryMCPMetadataCannotMintProtectedRootException(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("packaged macOS process containment seam")
	}
	protectedRoot := t.TempDir()
	ordinaryRoot := t.TempDir()
	secret := filepath.Join(protectedRoot, "sentinel.txt")
	if err := os.WriteFile(secret, []byte("must-not-escape"), 0o600); err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(ordinaryRoot, "result")
	client, err := NewTransportClientWithProtectedReadDirs(
		domainmcp.ServerSpec{
			ID: "stdio-forged-plugin-root", Transport: "stdio", Command: os.Args[0],
			PluginRootPath: protectedRoot, EntrypointPath: secret,
			Env: map[string]string{
				"MCP_CONTAINMENT_READ_TARGET": secret,
				"MCP_CONTAINMENT_RESULT_PATH": resultPath,
			},
			ExpectedServerName: "stdio-contract", ExpectedServerVersion: "1.0.0",
		},
		[]string{protectedRoot},
	)
	if err != nil {
		t.Fatalf("contained MCP initialization failed: %v", err)
	}
	client.Close()
	body, err := os.ReadFile(resultPath)
	if err != nil || string(body) != "blocked" {
		t.Fatalf("ordinary MCP metadata minted a protected-root exception: result=%q err=%v", body, err)
	}
}

func runProtocolHelper(mode string) {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 16*1024), maxMCPStdioFrameBytes)
	encoder := json.NewEncoder(os.Stdout)
	toolCalls := 0
	for scanner.Scan() {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params map[string]any  `json:"params"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil || len(request.ID) == 0 {
			continue
		}
		response := map[string]any{"jsonrpc": "2.0", "id": request.ID}
		switch request.Method {
		case "initialize":
			response["result"] = map[string]any{
				"protocolVersion": "2025-11-25",
				"serverInfo":      map[string]any{"name": "stdio-contract", "version": "1.0.0"},
				"capabilities":    map[string]any{"tools": map[string]any{}},
			}
		case "tools/list":
			toolName := "lookup"
			if mode == "paged-tools" {
				if cursor, _ := request.Params["cursor"].(string); cursor == "" {
					toolName = "lookup_a"
				} else if cursor == "page-2" {
					toolName = "lookup_b"
				} else {
					response["error"] = map[string]any{"code": -32602, "message": "invalid cursor"}
					break
				}
			}
			result := map[string]any{"tools": []map[string]any{{
				"name":         toolName,
				"inputSchema":  map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
				"outputSchema": map[string]any{"type": "object", "properties": map[string]any{"ok": map[string]any{"type": "boolean"}}, "required": []string{"ok"}, "additionalProperties": false},
			}}}
			if mode == "paged-tools" && toolName == "lookup_a" {
				result["nextCursor"] = "page-2"
			}
			response["result"] = result
		case "tools/call":
			toolCalls++
			if mode == "mismatched-then-valid" {
				_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": 999, "result": map[string]any{"content": []any{}, "structuredContent": map[string]any{"ok": true}}})
				response["result"] = map[string]any{"content": []any{}, "structuredContent": map[string]any{"ok": true}}
				_ = encoder.Encode(response)
				continue
			}
			if mode == "progress-then-valid" {
				_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/progress", "params": map[string]any{"progressToken": "p", "progress": 1}})
				response["result"] = map[string]any{"content": []any{}, "structuredContent": map[string]any{"ok": true}}
				_ = encoder.Encode(response)
				continue
			}
			if mode == "typed-error" || mode == "null-id-error" || mode == "typed-error-once" && toolCalls == 1 {
				code := -32602
				if mode == "null-id-error" {
					response["id"] = nil
					code = -32600
				}
				response["error"] = map[string]any{
					"code": code, "message": "account 6217000012345678901", "data": map[string]any{"secret": "pii"},
				}
			} else if mode == "typed-error-once" {
				response["result"] = map[string]any{"content": []any{}, "structuredContent": map[string]any{"ok": true}}
			} else {
				response["result"] = map[string]any{
					"content":           []map[string]any{{"type": "text", "text": "invalid"}},
					"structuredContent": map[string]any{"ok": "not-a-boolean"},
				}
			}
		default:
			response["error"] = map[string]any{"code": -32601, "message": "method not found"}
		}
		_ = encoder.Encode(response)
	}
}

func TestCommandEnvironmentAppendsConfiguredEnv(t *testing.T) {
	env, err := CommandEnvironment([]string{"A=1"}, map[string]string{"B": "2"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "A=1") || !strings.Contains(joined, "B=2") {
		t.Fatalf("environment mismatch: %#v", env)
	}
}

func TestRuntimeBearerTokenNeverInheritedByMCPProcess(t *testing.T) {
	t.Setenv("ANALYTIX_RUNTIME_TOKEN", "host-bearer-secret")
	t.Setenv("ANALYTIX_MCP_CHILD_ENV_HELPER", "1")
	started, err := Start(CommandSpec{Command: os.Args[0]})
	if err != nil {
		t.Fatal(err)
	}
	output, readErr := io.ReadAll(started.Stdout)
	started.Handle.KillAndWait()
	if readErr != nil || string(output) != "clean" {
		t.Fatalf("MCP child inherited a host secret: output=%q err=%v", output, readErr)
	}
}

func TestStdioToolsListPaginationCollectsCompleteCatalog(t *testing.T) {
	client, err := NewTransportClient(domainmcp.ServerSpec{
		ID: "stdio-contract", Transport: "stdio", Command: os.Args[0],
		Env: map[string]string{"ANALYTIX_MCP_STDIO_PROTOCOL_HELPER": "paged-tools"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	tools, err := client.ListTools()
	if err != nil || len(tools) != 2 || tools[0].Name != "lookup_a" || tools[1].Name != "lookup_b" {
		t.Fatalf("stdio paginated tools mismatch: tools=%#v err=%v", tools, err)
	}
}

func TestReservedHostSecretsCannotBeReintroducedByMCPSpecEnv(t *testing.T) {
	if _, err := CommandEnvironment([]string{"PATH=/bin"}, map[string]string{
		"ANALYTIX_RUNTIME_TOKEN": "forged-bearer",
	}); err == nil {
		t.Fatal("MCP spec reintroduced the reserved runtime bearer token")
	}
}

func TestProcessHandleKillAndWaitIsNilSafe(t *testing.T) {
	var handle *ProcessHandle
	handle.KillAndWait()
	(&ProcessHandle{}).KillAndWait()
}

func TestProcessHandleKillAndWaitReapsTrackedDescendants(t *testing.T) {
	started, err := Start(CommandSpec{
		Command: os.Args[0],
		Env:     map[string]string{"ANALYTIX_MCP_PROCESS_TREE_HELPER": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(started.Stdout)
	ready := make(chan error, 1)
	go func() {
		_, readErr := reader.ReadString('\n')
		ready <- readErr
	}()
	select {
	case readErr := <-ready:
		if readErr != nil {
			started.Handle.KillAndWait()
			t.Fatalf("tracked helper did not become ready: %v", readErr)
		}
	case <-time.After(5 * time.Second):
		started.Handle.KillAndWait()
		t.Fatal("tracked helper readiness timed out")
	}

	done := make(chan struct{})
	go func() {
		started.Handle.KillAndWait()
		started.Handle.KillAndWait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("tracked MCP process retained a descendant or inherited pipe")
	}
}

func TestReadBoundedFrameLineRejectsOversizeAndUnterminatedFrames(t *testing.T) {
	if line, err := readBoundedFrameLine(bufio.NewReaderSize(strings.NewReader("1234\r\n"), 2), 4); err != nil || string(line) != "1234" {
		t.Fatalf("exact-limit CRLF frame failed: line=%q err=%v", line, err)
	}
	if _, err := readBoundedFrameLine(bufio.NewReaderSize(strings.NewReader("12345\n"), 2), 4); err == nil {
		t.Fatal("oversized stdio frame was accepted")
	}
	if _, err := readBoundedFrameLine(bufio.NewReaderSize(strings.NewReader("{}"), 2), 4); err == nil {
		t.Fatal("unterminated stdio frame was accepted")
	}
}

func TestReadBoundedFrameLineDoesNotTrimNonJSONWhitespace(t *testing.T) {
	body := "\u00a0{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\u00a0\n"
	line, err := readBoundedFrameLine(bufio.NewReaderSize(strings.NewReader(body), 8), 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := mcpprotocol.ParseJSONRPCResponseWithID(line); err == nil {
		t.Fatal("Unicode whitespace outside JSON was trimmed and accepted")
	}
}

func TestStdioTransportPreservesTypedJSONRPCError(t *testing.T) {
	client, err := NewTransportClient(domainmcp.ServerSpec{
		ID: "stdio-contract", Transport: "stdio", Command: os.Args[0],
		Env: map[string]string{"ANALYTIX_MCP_STDIO_PROTOCOL_HELPER": "typed-error"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_, err = client.CallTool("lookup", map[string]any{})
	var typed *domainmcp.JSONRPCError
	if !errors.As(err, &typed) || typed.Code != -32602 || typed.Class() != "invalid_params" {
		t.Fatalf("stdio typed JSON-RPC error was lost: %#v", err)
	}
	if strings.Contains(err.Error(), "6217000012345678901") || strings.Contains(err.Error(), "pii") {
		t.Fatalf("stdio typed error leaked untrusted content: %q", err.Error())
	}
}

func TestStdioSemanticJSONRPCErrorLeavesConnectionUsable(t *testing.T) {
	client, err := NewTransportClient(domainmcp.ServerSpec{
		ID: "stdio-contract", Transport: "stdio", Command: os.Args[0],
		Env: map[string]string{"ANALYTIX_MCP_STDIO_PROTOCOL_HELPER": "typed-error-once"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.CallTool("lookup", map[string]any{}); err == nil {
		t.Fatal("first semantic JSON-RPC error was swallowed")
	}
	result, err := client.CallTool("lookup", map[string]any{})
	if err != nil {
		t.Fatalf("semantic JSON-RPC error killed a healthy connection: %v", err)
	}
	lossless, ok := result.(domainmcp.LosslessToolResult)
	if !ok || lossless.Value == nil {
		t.Fatalf("second response lost its lossless result: %#v", result)
	}
}

func TestMismatchedStdioResponseCannotPrecedeAcceptedResponse(t *testing.T) {
	client, err := NewTransportClient(domainmcp.ServerSpec{
		ID: "stdio-contract", Transport: "stdio", Command: os.Args[0],
		Env: map[string]string{"ANALYTIX_MCP_STDIO_PROTOCOL_HELPER": "mismatched-then-valid"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_, err = client.CallTool("lookup", map[string]any{})
	var invalidating interface{ MCPTransportInvalidatesIdentity() bool }
	if err == nil || !errors.As(err, &invalidating) || !invalidating.MCPTransportInvalidatesIdentity() {
		t.Fatalf("unrelated stdio response did not fail closed and revoke identity: %#v", err)
	}
}

func TestValidProgressNotificationCanPrecedeResponse(t *testing.T) {
	client, err := NewTransportClient(domainmcp.ServerSpec{
		ID: "stdio-contract", Transport: "stdio", Command: os.Args[0],
		Env: map[string]string{"ANALYTIX_MCP_STDIO_PROTOCOL_HELPER": "progress-then-valid"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	result, err := client.CallTool("lookup", map[string]any{})
	lossless, ok := result.(domainmcp.LosslessToolResult)
	if err != nil || !ok || !domainmcp.ValidLosslessToolResult(lossless) {
		t.Fatalf("valid progress notification prevented the matching stdio response: result=%#v err=%v", result, err)
	}
}

type recordingWriteCloser struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (writer *recordingWriteCloser) Write(body []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.buffer.Write(body)
}

func (*recordingWriteCloser) Close() error { return nil }

func (writer *recordingWriteCloser) Len() int {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.buffer.Len()
}

type observedContext struct {
	context.Context
	once    sync.Once
	checked chan struct{}
}

func (ctx *observedContext) Err() error {
	ctx.once.Do(func() { close(ctx.checked) })
	return ctx.Context.Err()
}

func TestStdioCancelWhileWaitingForRequestLockDoesNotWrite(t *testing.T) {
	base, cancel := context.WithCancel(context.Background())
	ctx := &observedContext{Context: base, checked: make(chan struct{})}
	writer := &recordingWriteCloser{}
	client := &TransportClient{stdin: writer}
	client.mu.Lock()
	done := make(chan error, 1)
	go func() {
		_, err := client.requestContext(ctx, "tools/call", map[string]any{})
		done <- err
	}()
	select {
	case <-ctx.checked:
	case <-time.After(time.Second):
		client.mu.Unlock()
		t.Fatal("request did not reach the pre-lock context check")
	}
	cancel()
	client.mu.Unlock()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled request returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled lock waiter did not return")
	}
	if writer.Len() != 0 {
		t.Fatalf("cancelled request wrote %d bytes", writer.Len())
	}
}

func TestStdioTransportPreservesNullIDInvalidRequestError(t *testing.T) {
	client, err := NewTransportClient(domainmcp.ServerSpec{
		ID: "stdio-contract", Transport: "stdio", Command: os.Args[0],
		Env: map[string]string{"ANALYTIX_MCP_STDIO_PROTOCOL_HELPER": "null-id-error"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_, err = client.CallTool("lookup", map[string]any{})
	var typed *domainmcp.JSONRPCError
	if !errors.As(err, &typed) || typed.Code != -32600 || typed.Class() != "invalid_request" {
		t.Fatalf("stdio null-id invalid-request error was lost: %#v", err)
	}
}

func TestStdioTransportPreservesRawOutputForHostValidation(t *testing.T) {
	client, err := NewTransportClient(domainmcp.ServerSpec{
		ID: "stdio-contract", Transport: "stdio", Command: os.Args[0],
		Env: map[string]string{"ANALYTIX_MCP_STDIO_PROTOCOL_HELPER": "invalid-output"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	tools, err := client.ListTools()
	if err != nil || len(tools) != 1 {
		t.Fatalf("stdio catalog failed: tools=%#v err=%v", tools, err)
	}
	result, err := client.CallTool("lookup", map[string]any{})
	lossless, ok := result.(domainmcp.LosslessToolResult)
	if err != nil || !ok {
		t.Fatalf("stdio lossless result failed: result=%#v err=%v", result, err)
	}
	if err := appmcp.ValidateToolResultOutput(lossless, tools[0].OutputSchema); err == nil {
		t.Fatal("stdio output schema mismatch passed host validation")
	}
}
