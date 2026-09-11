package httpmcp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
	"analytix.local/runtime-go/internal/testsupport/toolidentity"
)

func newLoopbackTransportClient(spec domainmcp.ServerSpec, proxyURL string) (*TransportClient, error) {
	if strings.TrimSpace(proxyURL) != "" {
		return NewTransportClient(spec, proxyURL)
	}
	timeout, err := domainmcp.ResolveServerTimeoutV1(spec.TimeoutMS)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return ErrMCPRedirectDisallowed
		},
	}
	client := &TransportClient{spec: spec, client: httpClient}
	if err := client.initialize(); err != nil {
		client.Close()
		return nil, err
	}
	return client, nil
}

func TestNewJSONRPCRequestAppliesHeadersAndSession(t *testing.T) {
	req, err := NewJSONRPCRequest(context.Background(), "https://example.invalid/mcp", map[string]string{
		"Authorization": "Bearer token",
		"X-Test":        "yes",
	}, " session-1 ", []byte(`{"jsonrpc":"2.0"}`))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if req.Method != http.MethodPost || req.URL.String() != "https://example.invalid/mcp" {
		t.Fatalf("unexpected request target: %s %s", req.Method, req.URL.String())
	}
	if req.Header.Get("Content-Type") != "application/json" ||
		req.Header.Get("Accept") != "application/json, text/event-stream" ||
		req.Header.Get("Authorization") != "Bearer token" ||
		req.Header.Get("X-Test") != "yes" ||
		req.Header.Get("Mcp-Session-Id") != "session-1" {
		t.Fatalf("headers not applied: %#v", req.Header)
	}
	data, _ := io.ReadAll(req.Body)
	if string(data) != `{"jsonrpc":"2.0"}` {
		t.Fatalf("body mismatch: %s", string(data))
	}
}

func TestTLSCertificateVerificationFailureIsIdentityCorruptionNotAvailability(t *testing.T) {
	var reached atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached.Store(true)
	}))
	defer server.Close()

	_, err := newLoopbackTransportClient(domainmcp.ServerSpec{ID: "tls-mismatch", Transport: "http", URL: server.URL}, "")
	if err == nil {
		t.Fatal("untrusted MCP TLS identity was accepted")
	}
	var classified interface {
		MCPTransportInvalidatesIdentity() bool
		MCPTransportUnavailable() bool
	}
	if !errors.As(err, &classified) || !classified.MCPTransportInvalidatesIdentity() || classified.MCPTransportUnavailable() {
		t.Fatalf("TLS identity failure was misclassified as an outage: %T %v", err, err)
	}
	if reached.Load() {
		t.Fatal("untrusted TLS connection reached the MCP handler")
	}
}

func TestMCPRedirectNeverCrossesConfiguredOrigin(t *testing.T) {
	var targetRequests atomic.Int32
	var targetSecretHeaders atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetRequests.Add(1)
		if r.Header.Get("X-API-Key") != "" {
			targetSecretHeaders.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jsonRPCResult(1, validInitializeResult("redirect-target")))
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()

	_, err := newLoopbackTransportClient(domainmcp.ServerSpec{
		ID: "redirect-source", Transport: "http", URL: redirector.URL,
		Headers: map[string]string{"X-API-Key": "synthetic-secret"},
	}, "")
	if err == nil {
		t.Fatal("MCP redirect was followed")
	}
	var classified interface {
		MCPTransportInvalidatesIdentity() bool
		MCPTransportUnavailable() bool
	}
	if !errors.As(err, &classified) || !classified.MCPTransportInvalidatesIdentity() || classified.MCPTransportUnavailable() {
		t.Fatalf("MCP redirect was not classified as identity corruption: %T %v", err, err)
	}
	if targetRequests.Load() != 0 || targetSecretHeaders.Load() != 0 {
		t.Fatalf("MCP redirect crossed origin: requests=%d secret_headers=%d", targetRequests.Load(), targetSecretHeaders.Load())
	}
}

func TestInvalidMCPProxyNeverFallsBackToDirect(t *testing.T) {
	var targetRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetRequests.Add(1)
	}))
	defer target.Close()

	_, err := NewTransportClient(
		domainmcp.ServerSpec{ID: "invalid-proxy", Transport: "http", URL: target.URL},
		"http://proxy-user:synthetic-secret@%",
	)
	if !errors.Is(err, ErrHTTPClientConfiguration) {
		t.Fatalf("invalid MCP proxy did not fail closed: %T %v", err, err)
	}
	if targetRequests.Load() != 0 {
		t.Fatalf("invalid MCP proxy fell back to direct connection: requests=%d", targetRequests.Load())
	}
}

func TestMalformedTLSHandshakeIsIdentityCorruptionNotAvailability(t *testing.T) {
	for name, cause := range map[string]error{
		"record header": tls.RecordHeaderError{},
		"tls alert":     tls.AlertError(40),
	} {
		t.Run(name, func(t *testing.T) {
			err := &TransportError{Method: "initialize", Cause: cause}
			if !err.MCPTransportInvalidatesIdentity() || err.MCPTransportUnavailable() {
				t.Fatalf("TLS protocol failure was misclassified as an outage: %#v", err)
			}
		})
	}
}

func TestConfiguredHeadersCannotOverrideMCPProtocolHeaders(t *testing.T) {
	for _, header := range []string{"Content-Type", "accept", "MCP-SESSION-ID", "MCP-PROTOCOL-VERSION"} {
		if _, err := NewJSONRPCRequest(context.Background(), "https://example.invalid/mcp", map[string]string{header: "forged"}, "", []byte(`{}`)); err == nil {
			t.Fatalf("reserved MCP header override passed: %s", header)
		}
	}
}

func TestSessionIDTrimsResponseHeader(t *testing.T) {
	resp := &http.Response{Header: http.Header{"Mcp-Session-Id": []string{" session-2 "}}}
	if got := SessionID(resp); got != "session-2" {
		t.Fatalf("session id mismatch: %q", got)
	}
}

func TestHTTPClientCloseTerminatesSessionExactlyOnce(t *testing.T) {
	var deleteCount atomic.Int32
	var headerMu sync.Mutex
	var deleteHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCount.Add(1)
			headerMu.Lock()
			deleteHeaders = r.Header.Clone()
			headerMu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		switch request.Method {
		case "initialize":
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Mcp-Session-Id", "close-session")
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, validInitializeResult("close-once")))
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		default:
			t.Errorf("unexpected method %q", request.Method)
		}
	}))
	defer server.Close()

	client, err := newLoopbackTransportClient(domainmcp.ServerSpec{
		ID: "close-once", URL: server.URL, Transport: "http",
		Headers: map[string]string{"Authorization": "Bearer close-test"},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	client.Close()

	if deleteCount.Load() != 1 {
		t.Fatalf("session close request count = %d, want 1", deleteCount.Load())
	}
	headerMu.Lock()
	headers := deleteHeaders.Clone()
	headerMu.Unlock()
	if headers.Get("Mcp-Session-Id") != "close-session" || headers.Get("MCP-Protocol-Version") != "2025-11-25" ||
		headers.Get("Authorization") != "Bearer close-test" {
		t.Fatalf("session close authority headers mismatch: %#v", headers)
	}
	if identity := client.ObservedServerIdentity(); identity.Name != "" || identity.Version != "" || identity.ProtocolVersion != "" {
		t.Fatalf("closed client retained observed authority: %#v", identity)
	}
}

func TestHTTPClientCloseAccepts405AndClearsAuthority(t *testing.T) {
	var deleteCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCount.Add(1)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		switch request.Method {
		case "initialize":
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Mcp-Session-Id", "unsupported-delete-session")
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, validInitializeResult("close-405")))
		case "notifications/initialized":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected method %q", request.Method)
		}
	}))
	defer server.Close()

	client, err := newLoopbackTransportClient(domainmcp.ServerSpec{ID: "close-405", URL: server.URL, Transport: "http"}, "")
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	if deleteCount.Load() != 1 || !errors.Is(closedListToolsError(client), ErrHTTPTransportClosed) {
		t.Fatalf("405 close did not terminate local authority: deletes=%d identity=%#v", deleteCount.Load(), client.ObservedServerIdentity())
	}
}

func TestHTTPClientCallAfterClosePerformsNoNetworkIO(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		switch request.Method {
		case "initialize":
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Mcp-Session-Id", "closed-io-session")
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, validInitializeResult("closed-io")))
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		default:
			t.Errorf("unexpected method %q", request.Method)
		}
	}))
	defer server.Close()

	client, err := newLoopbackTransportClient(domainmcp.ServerSpec{ID: "closed-io", URL: server.URL, Transport: "http"}, "")
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	requestsAfterClose := requestCount.Load()
	if _, err := client.ListTools(); !errors.Is(err, ErrHTTPTransportClosed) {
		t.Fatalf("closed tools/list error = %v", err)
	}
	if _, err := client.CallTool("lookup", map[string]any{}); !errors.Is(err, ErrHTTPTransportClosed) {
		t.Fatalf("closed tools/call error = %v", err)
	}
	if err := client.notify("notifications/cancelled", map[string]any{}); !errors.Is(err, ErrHTTPTransportClosed) {
		t.Fatalf("closed notification error = %v", err)
	}
	if requestCount.Load() != requestsAfterClose {
		t.Fatalf("closed client performed network I/O: before=%d after=%d", requestsAfterClose, requestCount.Load())
	}
}

func closedListToolsError(client *TransportClient) error {
	_, err := client.ListTools()
	return err
}

func TestSubsequentHTTPRequestsCarryNegotiatedProtocolVersion(t *testing.T) {
	seenInitialized := ""
	seenList := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		switch request.Method {
		case "initialize":
			if value := r.Header.Get("MCP-Protocol-Version"); value != "" {
				t.Fatalf("initialize sent a protocol header before negotiation: %q", value)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Mcp-Session-Id", "session-1")
			result := validInitializeResult("negotiated")
			result["protocolVersion"] = "2025-06-18"
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, result))
		case "notifications/initialized":
			seenInitialized = r.Header.Get("MCP-Protocol-Version")
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			seenList = r.Header.Get("MCP-Protocol-Version")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, validToolsResult()))
		default:
			t.Fatalf("unexpected method %q", request.Method)
		}
	}))
	defer server.Close()
	client, err := newLoopbackTransportClient(domainmcp.ServerSpec{ID: "negotiated", URL: server.URL, Transport: "http"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListTools(); err != nil {
		t.Fatal(err)
	}
	if seenInitialized != "2025-06-18" || seenList != "2025-06-18" || client.ObservedServerIdentity().ProtocolVersion != "2025-06-18" {
		t.Fatalf("negotiated protocol version was not frozen on subsequent requests: initialized=%q list=%q identity=%#v", seenInitialized, seenList, client.ObservedServerIdentity())
	}
}

func TestHTTPToolsListPaginationCollectsCompleteCatalog(t *testing.T) {
	cursors := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		switch request.Method {
		case "initialize":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, validInitializeResult("paged-http")))
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			cursor, _ := request.Params["cursor"].(string)
			cursors = append(cursors, cursor)
			toolName := "lookup_a"
			result := validToolsResult()
			result["tools"].([]map[string]any)[0]["name"] = toolName
			if cursor == "" {
				result["nextCursor"] = "page-2"
			} else if cursor == "page-2" {
				result["tools"].([]map[string]any)[0]["name"] = "lookup_b"
			} else {
				t.Fatalf("unexpected tools/list cursor %q", cursor)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, result))
		default:
			t.Fatalf("unexpected method %q", request.Method)
		}
	}))
	defer server.Close()
	client, err := newLoopbackTransportClient(domainmcp.ServerSpec{ID: "paged-http", URL: server.URL, Transport: "http"}, "")
	if err != nil {
		t.Fatal(err)
	}
	tools, err := client.ListTools()
	if err != nil || len(tools) != 2 || tools[0].Name != "lookup_a" || tools[1].Name != "lookup_b" || len(cursors) != 2 || cursors[0] != "" || cursors[1] != "page-2" {
		t.Fatalf("HTTP paginated tools mismatch: tools=%#v cursors=%#v err=%v", tools, cursors, err)
	}
}

func TestSession404RevokesIdentityAndRequiresInitialize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		switch request.Method {
		case "initialize":
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Mcp-Session-Id", "session-terminated")
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, validInitializeResult("session-404")))
		case "notifications/initialized":
			w.WriteHeader(http.StatusNoContent)
		case "tools/list":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Fatalf("unexpected method %q", request.Method)
		}
	}))
	defer server.Close()
	client, err := newLoopbackTransportClient(domainmcp.ServerSpec{ID: "session-404", URL: server.URL, Transport: "http"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListTools(); err == nil || !httpErrorInvalidatesIdentity(err) {
		t.Fatalf("session-bound 404 did not revoke the initialized identity: %v", err)
	}
}

func TestSSEMatchingResponseReturnsBeforeStreamClose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"ok\":true}}\n\n")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("test server lacks flush support")
		}
		flusher.Flush()
		select {
		case <-r.Context().Done():
		case <-time.After(500 * time.Millisecond):
		}
	}))
	defer server.Close()
	client := &TransportClient{
		spec: domainmcp.ServerSpec{ID: "sse-incremental", URL: server.URL}, client: server.Client(),
		sessionFrozen: true, protocolVersion: "2025-11-25",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	started := time.Now()
	result, err := client.requestContext(ctx, "tools/list", map[string]any{})
	if err != nil || !bytes.Contains(result, []byte(`"ok":true`)) {
		t.Fatalf("matching SSE response was not returned incrementally: result=%s err=%v", result, err)
	}
	if elapsed := time.Since(started); elapsed >= 150*time.Millisecond {
		t.Fatalf("matching SSE response waited for stream close: elapsed=%s", elapsed)
	}
}

func TestProxySpecModes(t *testing.T) {
	if spec := proxySpec(""); spec.Mode != "off" {
		t.Fatalf("blank proxy should disable environment proxy inheritance: %#v", spec)
	}
	if spec := proxySpec(" http://127.0.0.1:8080 "); spec.Mode != "custom" || !strings.Contains(spec.URL, "127.0.0.1") {
		t.Fatalf("custom proxy mismatch: %#v", spec)
	}
}

func TestTransportClientListsToolsAndCallsTool(t *testing.T) {
	wantContextDigest, wantGrantID := "", ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var request struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		switch request.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "session-1")
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, validInitializeResult("test")))
		case "notifications/initialized":
			if r.Header.Get("Mcp-Session-Id") != "session-1" {
				t.Fatalf("notification missing session header: %#v", r.Header)
			}
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			if r.Header.Get("Mcp-Session-Id") != "session-1" {
				t.Fatalf("tools/list missing session header: %#v", r.Header)
			}
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"tools": []map[string]any{{
				"name":         "echo",
				"inputSchema":  map[string]any{"type": "object"},
				"outputSchema": map[string]any{"type": "object", "properties": map[string]any{"ok": map[string]any{"type": "boolean"}}, "required": []string{"ok"}, "additionalProperties": false},
			}}}))
		case "tools/call":
			arguments, _ := request.Params["arguments"].(map[string]any)
			if arguments["text"] != "hi" {
				t.Fatalf("tools/call arguments mismatch: %#v", request.Params)
			}
			if _, leaked := arguments["_analytix"]; leaked {
				t.Fatalf("host runtime context leaked into provider arguments: %#v", request.Params)
			}
			meta, _ := request.Params["_meta"].(map[string]any)
			runtimeContext, _ := meta["analytixRuntimeContext"].(map[string]any)
			if runtimeContext["contextDigest"] != wantContextDigest || runtimeContext["grantId"] != wantGrantID {
				t.Fatalf("host runtime context missing from outer _meta: %#v", request.Params)
			}
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{"content": []map[string]any{{
				"type": "text",
				"text": "ok",
			}}, "structuredContent": map[string]any{"ok": true}}))
		case "analytix/sourceProbe":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, map[string]any{
				"version": 1, "serverName": "analytix_funds", "blocker": "", "rowCount": 2645472,
			}))
		default:
			t.Fatalf("unexpected method %q", request.Method)
		}
	}))
	defer server.Close()

	client, err := newLoopbackTransportClient(domainmcp.ServerSpec{ID: "remote", URL: server.URL, Transport: "http"}, "")
	if err != nil {
		t.Fatalf("new transport client: %v", err)
	}
	tools, err := client.ListTools()
	if err != nil || len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools mismatch tools=%#v err=%v", tools, err)
	}
	now := time.Now().UTC()
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-http", TurnID: "turn-http", WorkspaceRealPath: "/workspace", CaseID: "case-http",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-http"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 2, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	arguments := map[string]any{"text": "hi"}
	argumentBody, _ := json.Marshal(arguments)
	serverIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity("remote", "test", "test", domainsecurity.SHA256Hex([]byte("http-mcp-test-instance")), 1)
	if err != nil {
		t.Fatal(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-http", ServerIdentity: serverIdentity,
		ToolName: "mcp__remote__echo", ToolCallID: toolidentity.MustHostToolCallIDV1("http-host-context"), ConnectionEpoch: 1,
		ArgsHash: domainsecurity.CanonicalJSONHash(argumentBody), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	envelope, err := domainmcp.NewHostContextEnvelope(securityContext, grant)
	if err != nil {
		t.Fatal(err)
	}
	wantContextDigest, wantGrantID = securityContext.ContextDigest, grant.GrantID
	result, err := client.CallToolWithHostContext(context.Background(), "echo", arguments, envelope)
	lossless, ok := result.(domainmcp.LosslessToolResult)
	value, _ := lossless.Value.(map[string]any)
	structured, _ := value["structuredContent"].(map[string]any)
	if err != nil || !ok || value["text"] != "ok" || structured["ok"] != true || len(lossless.RawSHA256) != 64 {
		t.Fatalf("call result mismatch result=%#v err=%v", result, err)
	}
	native, err := client.CallNativeLosslessContext(context.Background(), "analytix/sourceProbe", map[string]any{})
	record, ok := native.Value.(map[string]any)
	if err != nil || !ok || record["serverName"] != "analytix_funds" || record["blocker"] != "" || record["rowCount"] != json.Number("2645472") {
		t.Fatalf("native result must preserve the complete lossless payload: result=%#v err=%v", native, err)
	}
}

func TestTransportClientPreservesTypedJSONRPCError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		switch request.Method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, validInitializeResult("typed-error")))
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/call":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": request.ID,
				"error": map[string]any{"code": -32602, "message": "account 6217000012345678901", "data": map[string]any{"secret": "pii"}},
			})
		default:
			t.Fatalf("unexpected method %q", request.Method)
		}
	}))
	defer server.Close()
	client, err := newLoopbackTransportClient(domainmcp.ServerSpec{ID: "typed", URL: server.URL, Transport: "http"}, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CallTool("lookup", map[string]any{})
	var typed *domainmcp.JSONRPCError
	if !errors.As(err, &typed) || typed.Code != -32602 || typed.Class() != "invalid_params" || string(typed.Data) != `{"secret":"pii"}` {
		t.Fatalf("typed JSON-RPC error was lost: %#v", err)
	}
	if strings.Contains(err.Error(), "6217000012345678901") || strings.Contains(err.Error(), "pii") {
		t.Fatalf("typed error string leaked untrusted data: %q", err.Error())
	}
	var invalidating interface{ MCPTransportInvalidatesIdentity() bool }
	if errors.As(err, &invalidating) && invalidating.MCPTransportInvalidatesIdentity() {
		t.Fatalf("valid JSON-RPC tool error invalidated the verified HTTP identity: %#v", err)
	}
}

func TestNullIDConnectionErrorInvalidatesIdentityWhilePreservingClass(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		switch request.Method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, validInitializeResult("null-id-error")))
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/call":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": nil,
				"error": map[string]any{"code": -32600, "message": "invalid request"},
			})
		default:
			t.Fatalf("unexpected method %q", request.Method)
		}
	}))
	defer server.Close()
	client, err := newLoopbackTransportClient(domainmcp.ServerSpec{ID: "null-id", URL: server.URL, Transport: "http"}, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CallTool("lookup", map[string]any{})
	var typed *domainmcp.JSONRPCError
	if !errors.As(err, &typed) || typed.Code != -32600 || typed.Class() != "invalid_request" {
		t.Fatalf("HTTP null-id invalid-request error was lost: %#v", err)
	}
	if !httpErrorInvalidatesIdentity(err) {
		t.Fatalf("connection-level null-id error did not revoke identity: %#v", err)
	}
}

func TestSessionEstablishedOnlyAfterValidatedInitialize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Mcp-Session-Id", "poison-session")
		_ = json.NewEncoder(w).Encode(jsonRPCResult(1, map[string]any{
			"protocolVersion": "2025-11-25", "capabilities": map[string]any{},
			"serverInfo": map[string]any{"name": ""},
		}))
	}))
	defer server.Close()
	client := &TransportClient{spec: domainmcp.ServerSpec{ID: "invalid", URL: server.URL}, client: server.Client()}
	if err := client.initialize(); err == nil {
		t.Fatal("invalid initialize identity was accepted")
	}
	if client.session != "" || client.sessionFrozen {
		t.Fatalf("failed initialize poisoned session state: session=%q frozen=%v", client.session, client.sessionFrozen)
	}
}

func TestRejectSessionRotationBeforeValidatedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		switch request.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "session-1")
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, validInitializeResult("session-test")))
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			w.Header().Set("Mcp-Session-Id", "session-2")
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, validToolsResult()))
		default:
			t.Fatalf("unexpected method %q", request.Method)
		}
	}))
	defer server.Close()
	client, err := newLoopbackTransportClient(domainmcp.ServerSpec{ID: "session-test", URL: server.URL}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListTools(); err == nil || client.session != "session-1" || !httpErrorInvalidatesIdentity(err) {
		t.Fatalf("session rotation was accepted or mutated authority: session=%q err=%v", client.session, err)
	}
}

func TestHTTPProtocolViolationsInvalidateVerifiedIdentity(t *testing.T) {
	tests := map[string]func(http.ResponseWriter, int){
		"content type": func(w http.ResponseWriter, requestID int) {
			w.Header().Set("Content-Type", "text/plain")
			_ = json.NewEncoder(w).Encode(jsonRPCResult(requestID, validToolsResult()))
		},
		"response id": func(w http.ResponseWriter, requestID int) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(jsonRPCResult(requestID+1, validToolsResult()))
		},
		"malformed jsonrpc": func(w http.ResponseWriter, _ int) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":`)
		},
		"session rotation": func(w http.ResponseWriter, requestID int) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Mcp-Session-Id", "session-2")
			_ = json.NewEncoder(w).Encode(jsonRPCResult(requestID, validToolsResult()))
		},
		"jsonrpc error session rotation": func(w http.ResponseWriter, requestID int) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Mcp-Session-Id", "session-2")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": requestID,
				"error": map[string]any{"code": -32602, "message": "invalid params"},
			})
		},
		"malformed jsonrpc error": func(w http.ResponseWriter, requestID int) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": requestID,
				"error": map[string]any{"code": -32602, "message": "invalid params", "unexpected": true},
			})
		},
	}
	for name, respond := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					ID int `json:"id"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatal(err)
				}
				respond(w, request.ID)
			}))
			defer server.Close()
			client := &TransportClient{
				spec: domainmcp.ServerSpec{ID: "protocol-test", URL: server.URL}, client: server.Client(),
				session: "session-1", sessionFrozen: true,
			}
			if _, err := client.request("tools/list", map[string]any{}); err == nil || !httpErrorInvalidatesIdentity(err) {
				t.Fatalf("HTTP protocol violation did not invalidate identity: err=%#v", err)
			}
		})
	}
}

func httpErrorInvalidatesIdentity(err error) bool {
	var typed interface{ MCPTransportInvalidatesIdentity() bool }
	return errors.As(err, &typed) && typed.MCPTransportInvalidatesIdentity()
}

func TestFailedResponseCannotPoisonMCPSession(t *testing.T) {
	listCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		switch request.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "session-1")
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, validInitializeResult("poison-test")))
		case "notifications/initialized":
			w.WriteHeader(http.StatusNoContent)
		case "tools/list":
			listCalls++
			if listCalls == 1 {
				w.Header().Set("Mcp-Session-Id", "poison-session")
				http.Error(w, "temporary failure", http.StatusBadGateway)
				return
			}
			w.Header().Set("Mcp-Session-Id", "session-1")
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, validToolsResult()))
		default:
			t.Fatalf("unexpected method %q", request.Method)
		}
	}))
	defer server.Close()
	client, err := newLoopbackTransportClient(domainmcp.ServerSpec{ID: "poison-test", URL: server.URL}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListTools(); err == nil || client.session != "session-1" {
		t.Fatalf("failed response changed session: session=%q err=%v", client.session, err)
	}
	if tools, err := client.ListTools(); err != nil || len(tools) != 1 || client.session != "session-1" {
		t.Fatalf("frozen session did not survive failed response: tools=%#v session=%q err=%v", tools, client.session, err)
	}
}

func TestNotificationJSONRPCErrorIsNotSwallowed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Method == "initialize" {
			w.Header().Set("Mcp-Session-Id", "session-1")
			_ = json.NewEncoder(w).Encode(jsonRPCResult(request.ID, validInitializeResult("notify-test")))
			return
		}
		w.Header().Set("Mcp-Session-Id", "poison-session")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": nil, "error": map[string]any{"code": -32600, "message": "invalid"}})
	}))
	defer server.Close()
	client := &TransportClient{spec: domainmcp.ServerSpec{ID: "notify-test", URL: server.URL}, client: server.Client()}
	if err := client.initialize(); err == nil {
		t.Fatal("notification 200 JSON-RPC error was swallowed")
	}
	if client.session != "session-1" {
		t.Fatalf("notification response rotated session: %q", client.session)
	}
}

func TestOutboundMCPMarshalFailurePerformsNoHTTPIO(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	client := &TransportClient{spec: domainmcp.ServerSpec{ID: "marshal", URL: server.URL}, client: server.Client(), sessionFrozen: true}
	if _, err := client.request("tools/call", map[string]any{"value": math.NaN()}); err == nil {
		t.Fatal("non-JSON outbound value was accepted")
	}
	if requests != 0 {
		t.Fatalf("marshal failure performed %d HTTP requests", requests)
	}
}

func TestReadBoundedBodyRejectsOversizeAndReadFailure(t *testing.T) {
	if _, err := readBoundedBody(bytes.NewReader([]byte("12345")), 4); err == nil {
		t.Fatal("max+1 response body was accepted")
	}
	if body, err := readBoundedBody(bytes.NewReader([]byte("1234")), 4); err != nil || string(body) != "1234" {
		t.Fatalf("exact-limit body failed: body=%q err=%v", body, err)
	}
	if _, err := readBoundedBody(errorReader{}, 4); err == nil {
		t.Fatal("response body read error was ignored")
	}
}

func TestTransportClientRejectsTruncatedOversizeJSONPrefix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := []byte(`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"name":"oversized"}}}`)
		body := append(prefix, bytes.Repeat([]byte(" "), maxMCPResponseBytes-len(prefix))...)
		body = append(body, []byte(`{}`)...)
		_, _ = w.Write(body)
	}))
	defer server.Close()
	if _, err := newLoopbackTransportClient(domainmcp.ServerSpec{ID: "oversized", URL: server.URL, Transport: "http"}, ""); err == nil {
		t.Fatal("oversized response with a valid truncated prefix was accepted")
	}
}

func TestTransportClientHTTPStatusNeverLeaksResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("account 6217000012345678901 Authorization: Bearer secret"))
	}))
	defer server.Close()
	_, err := newLoopbackTransportClient(domainmcp.ServerSpec{ID: "status", URL: server.URL, Transport: "http"}, "")
	if err == nil || strings.Contains(err.Error(), "6217000012345678901") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("HTTP status error leaked an untrusted body: %v", err)
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("fixture read failure") }

func jsonRPCResult(id int, result any) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "result": result}
}

func validInitializeResult(name string) map[string]any {
	return map[string]any{
		"protocolVersion": "2025-11-25",
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": name, "version": "1.0.0"},
	}
}

func validToolsResult() map[string]any {
	return map[string]any{"tools": []map[string]any{{
		"name":         "lookup",
		"inputSchema":  map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
		"outputSchema": map[string]any{"type": "object", "properties": map[string]any{"ok": map[string]any{"type": "boolean"}}, "required": []string{"ok"}, "additionalProperties": false},
	}}}
}
