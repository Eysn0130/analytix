package httpmcp

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	mcpidentity "analytix.local/runtime-go/internal/adapters/outbound/mcp/identity"
	mcpprotocol "analytix.local/runtime-go/internal/adapters/outbound/mcp/protocol"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	"analytix.local/runtime-go/internal/netclient"
)

type TransportClient struct {
	spec            domainmcp.ServerSpec
	client          *http.Client
	capabilities    mcpprotocol.Capabilities
	identity        domainmcp.ServerIdentity
	operationMu     sync.RWMutex
	mu              sync.Mutex
	nextID          int
	session         string
	sessionFrozen   bool
	protocolVersion string
	closed          bool
}

const maxMCPNotificationBytes = 1024 * 1024
const maxMCPResponseBytes = 4 * 1024 * 1024
const mcpSessionCloseTimeout = 2 * time.Second

var (
	ErrHTTPTransportClosed     = errors.New("MCP HTTP transport is closed")
	ErrMCPRedirectDisallowed   = errors.New("MCP HTTP redirect is disallowed")
	ErrHTTPClientConfiguration = errors.New("MCP HTTP client configuration is invalid")
)

type StatusError struct {
	Method       string
	StatusCode   int
	Notify       bool
	SessionBound bool
}

type TransportError struct {
	Method string
	Cause  error
}

// ProtocolError represents a response that can no longer be attributed to
// the verified MCP connection identity (for example a rotated session,
// mismatched request id, malformed JSON-RPC frame, or invalid media type).
// The manager must revoke the current identity/epoch before any later call.
type ProtocolError struct {
	Method string
	Kind   string
	Cause  error
}

func (err *TransportError) Error() string {
	return "mcp http transport failure"
}

func (err *TransportError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func (err *TransportError) MCPTransportRetryable() bool {
	return err != nil
}

func (err *TransportError) MCPTransportInvalidatesIdentity() bool {
	return err != nil && isHTTPTransportIdentityFailure(err.Cause)
}
func (err *TransportError) MCPTransportUnavailable() bool {
	return err != nil && !err.MCPTransportInvalidatesIdentity()
}

func isHTTPTransportIdentityFailure(err error) bool {
	if errors.Is(err, ErrMCPRedirectDisallowed) ||
		errors.Is(err, ErrMCPRemoteDestinationDisallowed) {
		return true
	}
	var verificationErr *tls.CertificateVerificationError
	if errors.As(err, &verificationErr) {
		return true
	}
	var unknownAuthority x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthority) {
		return true
	}
	var hostname x509.HostnameError
	if errors.As(err, &hostname) {
		return true
	}
	var invalidCertificate x509.CertificateInvalidError
	if errors.As(err, &invalidCertificate) {
		return true
	}
	var recordHeader tls.RecordHeaderError
	if errors.As(err, &recordHeader) {
		return true
	}
	var alert tls.AlertError
	if errors.As(err, &alert) {
		return true
	}
	var systemRoots x509.SystemRootsError
	return errors.As(err, &systemRoots)
}

func (err *ProtocolError) Error() string {
	return "mcp http protocol failure"
}

func (err *ProtocolError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func (err *ProtocolError) MCPTransportRetryable() bool { return false }

func (err *ProtocolError) MCPTransportInvalidatesIdentity() bool { return err != nil }
func (err *ProtocolError) MCPTransportUnavailable() bool         { return false }

func (err *StatusError) Error() string {
	if err == nil {
		return "mcp http status error"
	}
	if err.Notify {
		return fmt.Sprintf("mcp http %s notification returned status %d", err.Method, err.StatusCode)
	}
	return fmt.Sprintf("mcp http %s returned status %d", err.Method, err.StatusCode)
}

func (err *StatusError) MCPTransportRetryable() bool {
	if err == nil || err.Notify {
		return false
	}
	switch err.StatusCode {
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func (err *StatusError) MCPTransportInvalidatesIdentity() bool {
	return err != nil && err.SessionBound && err.StatusCode == http.StatusNotFound
}

func (err *StatusError) MCPTransportUnavailable() bool {
	return err != nil && err.MCPTransportRetryable() && !err.MCPTransportInvalidatesIdentity()
}

func NewTransportClient(spec domainmcp.ServerSpec, proxyURL string) (*TransportClient, error) {
	timeout, err := domainmcp.ResolveServerTimeoutV1(spec.TimeoutMS)
	if err != nil {
		return nil, err
	}
	httpClient, err := newClientForRemoteEndpoint(spec.URL, proxyURL, timeout)
	if err != nil {
		return nil, ErrHTTPClientConfiguration
	}
	return newTransportClientWithHTTPClient(spec, httpClient)
}

// LoopbackTestAuthority is an opaque capability minted only from a Go test
// frame. It lets background test goroutines keep using their in-process MCP
// server without weakening the production constructor.
type LoopbackTestAuthority struct {
	approved bool
}

func AuthorizeLoopbackTransportForTests() (LoopbackTestAuthority, error) {
	if !calledFromGoTest() {
		return LoopbackTestAuthority{}, ErrHTTPClientConfiguration
	}
	return LoopbackTestAuthority{approved: true}, nil
}

// NewLoopbackTransportClientForTests preserves protocol tests that use an
// in-process httptest server. Only a previously minted opaque test capability
// enables the loopback path.
func NewLoopbackTransportClientForTests(
	authority LoopbackTestAuthority,
	spec domainmcp.ServerSpec,
	proxyURL string,
) (*TransportClient, error) {
	if !authority.approved || strings.TrimSpace(proxyURL) != "" {
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
	return newTransportClientWithHTTPClient(spec, httpClient)
}

func calledFromGoTest() bool {
	for skip := 1; skip < 32; skip++ {
		_, callerFile, _, callerOK := runtime.Caller(skip)
		if !callerOK {
			return false
		}
		if strings.HasSuffix(callerFile, "_test.go") {
			return true
		}
	}
	return false
}

func newTransportClientWithHTTPClient(spec domainmcp.ServerSpec, httpClient *http.Client) (*TransportClient, error) {
	if httpClient == nil {
		return nil, ErrHTTPClientConfiguration
	}
	client := &TransportClient{spec: spec, client: httpClient}
	if err := client.initialize(); err != nil {
		client.Close()
		return nil, err
	}
	return client, nil
}

func NewClient(proxyURL string) (*http.Client, error) {
	timeout, _ := domainmcp.ResolveServerTimeoutV1(0)
	return NewClientWithTimeout(proxyURL, timeout)
}

func NewClientWithTimeout(proxyURL string, timeout time.Duration) (*http.Client, error) {
	return newClientForRemoteEndpoint("", proxyURL, timeout)
}

func newClientForRemoteEndpoint(rawURL string, proxyURL string, timeout time.Duration) (*http.Client, error) {
	if timeout <= 0 {
		timeout, _ = domainmcp.ResolveServerTimeoutV1(0)
	}
	if strings.TrimSpace(proxyURL) != "" {
		return nil, ErrMCPRemoteProxyDisallowed
	}
	var endpoint remoteEndpoint
	var err error
	if strings.TrimSpace(rawURL) != "" {
		endpoint, err = parseMCPRemoteEndpoint(rawURL)
		if err != nil {
			return nil, err
		}
	}
	transport, err := netclient.NewTransport(proxySpec(proxyURL), netclient.TransportOptions{
		DialTimeout:           30 * time.Second,
		KeepAlive:             30 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	transport.DialContext = secureMCPRemoteDialContext(net.DefaultResolver, dialer)
	transport.Proxy = nil
	if endpoint.scheme == "https" {
		tlsConfig := transport.TLSClientConfig
		if tlsConfig == nil {
			tlsConfig = &tls.Config{}
		} else {
			tlsConfig = tlsConfig.Clone()
		}
		tlsConfig.ServerName = endpoint.hostname
		transport.TLSClientConfig = tlsConfig
	}
	client := &http.Client{Transport: transport, Timeout: timeout}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return ErrMCPRedirectDisallowed
	}
	return client, nil
}

func NewJSONRPCRequest(ctx context.Context, rawURL string, headers map[string]string, sessionID string, body []byte) (*http.Request, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(body) > mcpprotocol.MaxMCPJSONBytes {
		return nil, errors.New("MCP JSON-RPC request exceeds limit")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		switch http.CanonicalHeaderKey(strings.TrimSpace(key)) {
		case "Content-Type", "Accept", "Mcp-Session-Id", "Mcp-Protocol-Version":
			return nil, fmt.Errorf("MCP configuration cannot override reserved header %q", key)
		}
		req.Header.Set(key, value)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if strings.TrimSpace(sessionID) != "" {
		req.Header.Set("Mcp-Session-Id", strings.TrimSpace(sessionID))
	}
	return req, nil
}

func NewSessionDeleteRequest(ctx context.Context, rawURL string, headers map[string]string, sessionID string, protocolVersion string) (*http.Request, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("MCP session id is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, rawURL, nil)
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		switch http.CanonicalHeaderKey(strings.TrimSpace(key)) {
		case "Content-Type", "Accept", "Mcp-Session-Id", "Mcp-Protocol-Version":
			return nil, fmt.Errorf("MCP configuration cannot override reserved header %q", key)
		}
		req.Header.Set(key, value)
	}
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Session-Id", sessionID)
	if protocolVersion = strings.TrimSpace(protocolVersion); protocolVersion != "" {
		req.Header.Set("MCP-Protocol-Version", protocolVersion)
	}
	return req, nil
}

func SessionID(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	return strings.TrimSpace(resp.Header.Get("Mcp-Session-Id"))
}

func (c *TransportClient) initialize() error {
	c.operationMu.RLock()
	defer c.operationMu.RUnlock()
	if c.isClosed() {
		return ErrHTTPTransportClosed
	}
	result, responseSession, err := c.requestContextCandidateOpen(context.Background(), "initialize", map[string]any{
		"protocolVersion": mcpprotocol.ProtocolVersion,
		"clientInfo": map[string]any{
			"name":    "analytix",
			"version": "0.1.0",
		},
		"capabilities": map[string]any{},
	})
	if err != nil {
		return err
	}
	identity, capabilities, err := mcpprotocol.ParseInitializeResult(result, mcpprotocol.ProtocolVersion)
	if err != nil {
		return err
	}
	if err := mcpidentity.VerifyObserved(c.spec, identity); err != nil {
		return err
	}
	c.mu.Lock()
	c.identity = identity
	c.capabilities = capabilities
	c.protocolVersion = identity.ProtocolVersion
	c.mu.Unlock()
	if err := c.freezeInitializedSession(responseSession); err != nil {
		return err
	}
	return c.notifyOpen("notifications/initialized", map[string]any{})
}

func (c *TransportClient) ListTools() ([]domainmcp.ToolSpec, error) {
	return c.ListToolsContext(context.Background())
}

func (c *TransportClient) ListToolsContext(ctx context.Context) ([]domainmcp.ToolSpec, error) {
	catalog, err := c.ListToolCatalogContext(ctx)
	if err != nil {
		return nil, err
	}
	return catalog.Tools, nil
}

func (c *TransportClient) ListToolCatalogContext(ctx context.Context) (mcpprotocol.ToolCatalog, error) {
	capabilities, err := c.currentCapabilities()
	if err != nil {
		return mcpprotocol.ToolCatalog{}, err
	}
	if !capabilities.Tools {
		return mcpprotocol.ToolCatalog{}, errors.New("MCP server did not advertise tools capability")
	}
	return mcpprotocol.CollectToolCatalogPages(ctx, func(callCtx context.Context, params map[string]any) (json.RawMessage, error) {
		return c.requestContext(callCtx, "tools/list", params)
	})
}

func (c *TransportClient) ListPrompts() ([]domainmcp.PromptSpec, error) {
	capabilities, err := c.currentCapabilities()
	if err != nil {
		return nil, err
	}
	if !capabilities.Prompts {
		return nil, nil
	}
	result, err := c.request("prompts/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	return mcpprotocol.ParsePrompts(result), nil
}

func (c *TransportClient) ListResources() ([]domainmcp.ResourceSpec, error) {
	capabilities, err := c.currentCapabilities()
	if err != nil {
		return nil, err
	}
	if !capabilities.Resources {
		return nil, nil
	}
	result, err := c.request("resources/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	return mcpprotocol.ParseResources(result), nil
}

func (c *TransportClient) CallTool(name string, arguments map[string]any) (any, error) {
	return c.CallToolContext(context.Background(), name, arguments)
}

func (c *TransportClient) CallToolContext(ctx context.Context, name string, arguments map[string]any) (any, error) {
	result, err := c.requestContext(ctx, "tools/call", mcpprotocol.ToolCallParams(name, arguments))
	if err != nil {
		return nil, err
	}
	return mcpprotocol.ExtractLosslessToolResult(result), nil
}

func (c *TransportClient) CallToolWithHostContext(ctx context.Context, name string, arguments map[string]any, envelope domainmcp.HostContextEnvelope) (any, error) {
	_, grant, authorityErr := envelope.Authority()
	if authorityErr != nil || grant.ToolName != "mcp__"+strings.TrimSpace(c.spec.ID)+"__"+strings.TrimSpace(name) {
		return nil, errors.New("MCP host context tool authority is invalid")
	}
	params, err := mcpprotocol.ToolCallParamsWithHostContext(name, arguments, envelope)
	if err != nil {
		return nil, err
	}
	result, err := c.requestContext(ctx, "tools/call", params)
	if err != nil {
		return nil, err
	}
	return mcpprotocol.ExtractLosslessToolResult(result), nil
}

func (c *TransportClient) CallNativeContext(ctx context.Context, method string, params map[string]any) (any, error) {
	result, err := c.CallNativeLosslessContext(ctx, method, params)
	return result.Value, err
}

func (c *TransportClient) CallNativeLosslessContext(ctx context.Context, method string, params map[string]any) (domainmcp.LosslessToolResult, error) {
	result, err := c.requestContext(ctx, strings.TrimSpace(method), params)
	if err != nil {
		return domainmcp.LosslessToolResult{}, err
	}
	return mcpprotocol.DecodeLosslessJSONResult(result), nil
}

func (c *TransportClient) Close() {
	c.operationMu.Lock()
	defer c.operationMu.Unlock()

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	session := c.session
	protocolVersion := c.protocolVersion
	c.session = ""
	c.sessionFrozen = false
	c.protocolVersion = ""
	c.identity = domainmcp.ServerIdentity{}
	c.capabilities = mcpprotocol.Capabilities{}
	c.mu.Unlock()

	if strings.TrimSpace(session) == "" || c.client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), mcpSessionCloseTimeout)
	defer cancel()
	req, err := NewSessionDeleteRequest(ctx, c.spec.URL, c.spec.Headers, session, protocolVersion)
	if err != nil {
		return
	}
	resp, err := c.client.Do(req)
	if err != nil {
		closeResponseBody(resp)
		return
	}
	_ = resp.Body.Close()
}

func (c *TransportClient) ObservedServerIdentity() domainmcp.ServerIdentity {
	c.operationMu.RLock()
	defer c.operationMu.RUnlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.identity
}

func (c *TransportClient) notify(method string, params map[string]any) error {
	c.operationMu.RLock()
	defer c.operationMu.RUnlock()
	if c.isClosed() {
		return ErrHTTPTransportClosed
	}
	return c.notifyOpen(method, params)
}

func (c *TransportClient) notifyOpen(method string, params map[string]any) error {
	c.mu.Lock()
	session := c.session
	protocolVersion := c.protocolVersion
	c.mu.Unlock()
	payload := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	body, err := mcpprotocol.MarshalBoundedJSONRPC(payload)
	if err != nil {
		return err
	}
	req, err := NewJSONRPCRequest(context.Background(), c.spec.URL, c.spec.Headers, session, body)
	if err != nil {
		return err
	}
	if protocolVersion != "" {
		req.Header.Set("MCP-Protocol-Version", protocolVersion)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		closeResponseBody(resp)
		return &TransportError{Method: method, Cause: err}
	}
	defer resp.Body.Close()
	data, err := readBoundedBody(resp.Body, maxMCPNotificationBytes)
	if err != nil {
		return newProtocolError(method, "notification_body", err)
	}
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusNoContent {
		return &StatusError{Method: method, StatusCode: resp.StatusCode, Notify: true, SessionBound: session != ""}
	}
	if len(bytes.TrimSpace(data)) != 0 {
		return newProtocolError(method, "notification_body", errors.New("MCP notification response must have an empty body"))
	}
	if err := c.validateFrozenSession(SessionID(resp)); err != nil {
		return newProtocolError(method, "session", err)
	}
	return nil
}

func (c *TransportClient) request(method string, params map[string]any) (json.RawMessage, error) {
	return c.requestContext(context.Background(), method, params)
}

func (c *TransportClient) requestContext(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	c.operationMu.RLock()
	defer c.operationMu.RUnlock()
	if c.isClosed() {
		return nil, ErrHTTPTransportClosed
	}
	result, responseSession, candidateErr := c.requestContextCandidateOpen(ctx, method, params)
	var rpcError *domainmcp.JSONRPCError
	validateSession := candidateErr == nil || responseSession != "" || errors.As(candidateErr, &rpcError)
	if validateSession {
		if err := c.validateFrozenSession(responseSession); err != nil {
			return nil, newProtocolError(method, "session", err)
		}
	}
	if candidateErr != nil {
		return nil, candidateErr
	}
	return result, nil
}

func (c *TransportClient) requestContextCandidateOpen(ctx context.Context, method string, params map[string]any) (json.RawMessage, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	c.mu.Lock()
	c.nextID++
	requestID := c.nextID
	session := c.session
	protocolVersion := c.protocolVersion
	c.mu.Unlock()
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      requestID,
		"method":  method,
		"params":  params,
	}
	body, err := mcpprotocol.MarshalBoundedJSONRPC(payload)
	if err != nil {
		return nil, "", err
	}
	req, err := NewJSONRPCRequest(ctx, c.spec.URL, c.spec.Headers, session, body)
	if err != nil {
		return nil, "", err
	}
	if protocolVersion != "" {
		req.Header.Set("MCP-Protocol-Version", protocolVersion)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		closeResponseBody(resp)
		return nil, "", &TransportError{Method: method, Cause: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", &StatusError{Method: method, StatusCode: resp.StatusCode, SessionBound: session != ""}
	}
	mediaType, _, mimeErr := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if mimeErr != nil || mediaType != "application/json" && mediaType != "text/event-stream" {
		return nil, "", newProtocolError(method, "content_type", errors.New("MCP response Content-Type is invalid"))
	}
	responseSession := SessionID(resp)
	if mediaType == "text/event-stream" {
		result, err := mcpprotocol.ReadSSEJSONRPCResponse(resp.Body, requestID, maxMCPResponseBytes, 1024)
		if err != nil {
			var rpcError *domainmcp.JSONRPCError
			if errors.As(err, &rpcError) {
				if rpcError.Code == -32700 || rpcError.Code == -32600 {
					return nil, responseSession, newProtocolError(method, "connection_jsonrpc", err)
				}
				return nil, responseSession, err
			}
			return nil, "", newProtocolError(method, "sse_jsonrpc", err)
		}
		return result, responseSession, err
	}
	data, err := readBoundedBody(resp.Body, maxMCPResponseBytes)
	if err != nil {
		return nil, "", newProtocolError(method, "response_body", err)
	}
	result, responseID, err := mcpprotocol.ParseJSONRPCResponseWithID(data)
	if err != nil {
		var rpcError *domainmcp.JSONRPCError
		if errors.As(err, &rpcError) && responseID.Null && (rpcError.Code == -32700 || rpcError.Code == -32600) {
			return nil, responseSession, newProtocolError(method, "connection_jsonrpc", err)
		}
		if !responseID.Matches(requestID) {
			return nil, "", newProtocolError(method, "response_id", errors.New("MCP error response id does not match request id"))
		}
		if errors.As(err, &rpcError) {
			return nil, responseSession, err
		}
		return nil, "", newProtocolError(method, "jsonrpc", err)
	}
	if !responseID.Matches(requestID) {
		return nil, "", newProtocolError(method, "response_id", errors.New("MCP response id does not match request id"))
	}
	return result, responseSession, nil
}

func newProtocolError(method string, kind string, cause error) error {
	return &ProtocolError{Method: strings.TrimSpace(method), Kind: strings.TrimSpace(kind), Cause: cause}
}

func (c *TransportClient) freezeInitializedSession(responseSession string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return ErrHTTPTransportClosed
	}
	if c.sessionFrozen {
		if responseSession != "" && responseSession != c.session {
			return errors.New("MCP session changed after initialization")
		}
		return nil
	}
	c.session = responseSession
	c.sessionFrozen = true
	return nil
}

func (c *TransportClient) validateFrozenSession(responseSession string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return ErrHTTPTransportClosed
	}
	if !c.sessionFrozen {
		return errors.New("MCP session is not initialized")
	}
	if responseSession == "" {
		return nil
	}
	if c.session == "" || responseSession != c.session {
		return errors.New("MCP response attempted to establish or rotate the frozen session")
	}
	return nil
}

func (c *TransportClient) currentCapabilities() (mcpprotocol.Capabilities, error) {
	c.operationMu.RLock()
	defer c.operationMu.RUnlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return mcpprotocol.Capabilities{}, ErrHTTPTransportClosed
	}
	return c.capabilities, nil
}

func (c *TransportClient) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func readBoundedBody(body io.Reader, maxBytes int) ([]byte, error) {
	if body == nil || maxBytes <= 0 {
		return nil, errors.New("response body limit is invalid")
	}
	data, err := io.ReadAll(io.LimitReader(body, int64(maxBytes)+1))
	if err != nil {
		return nil, errors.New("response body read failed")
	}
	if len(data) > maxBytes {
		return nil, errors.New("response body exceeds limit")
	}
	return data, nil
}

func closeResponseBody(response *http.Response) {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
}

func proxySpec(proxyURL string) netclient.ProxySpec {
	if strings.TrimSpace(proxyURL) == "" {
		return netclient.ProxySpec{Mode: netclient.ModeOff}
	}
	return netclient.ProxySpec{Mode: netclient.ModeCustom, URL: strings.TrimSpace(proxyURL)}
}
