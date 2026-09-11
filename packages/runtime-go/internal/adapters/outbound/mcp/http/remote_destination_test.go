package httpmcp

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
)

type staticRemoteResolver struct {
	addresses []netip.Addr
	err       error
}

func (resolver staticRemoteResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return append([]netip.Addr(nil), resolver.addresses...), resolver.err
}

type recordingRemoteDialer struct {
	mu        sync.Mutex
	addresses []string
}

func (dialer *recordingRemoteDialer) DialContext(_ context.Context, _ string, address string) (net.Conn, error) {
	dialer.mu.Lock()
	dialer.addresses = append(dialer.addresses, address)
	dialer.mu.Unlock()
	client, peer := net.Pipe()
	_ = peer.Close()
	return client, nil
}

func (dialer *recordingRemoteDialer) count() int {
	dialer.mu.Lock()
	defer dialer.mu.Unlock()
	return len(dialer.addresses)
}

func (dialer *recordingRemoteDialer) firstAddress() string {
	dialer.mu.Lock()
	defer dialer.mu.Unlock()
	if len(dialer.addresses) == 0 {
		return ""
	}
	return dialer.addresses[0]
}

func TestMCPRemoteEndpointRejectsNonPublicAndAmbiguousTargets(t *testing.T) {
	for name, rawURL := range map[string]string{
		"unix scheme":          "http+unix://%2Ftmp%2Fmcp.sock/mcp",
		"url userinfo":         "https://user:password@example.com/mcp",
		"fragment":             "https://example.com/mcp#other-target",
		"single label":         "https://mcpserver/mcp",
		"localhost":            "http://localhost:8080/mcp",
		"localhost suffix":     "http://service.localhost/mcp",
		"mdns local":           "http://service.local/mcp",
		"private dns suffix":   "http://service.internal/mcp",
		"home reverse dns":     "http://router.home.arpa/mcp",
		"onion proxy target":   "http://service.onion/mcp",
		"loopback ipv4":        "http://127.0.0.1:8080/mcp",
		"unspecified ipv4":     "http://0.0.0.0:8080/mcp",
		"private ipv4":         "http://10.1.2.3/mcp",
		"cgnat ipv4":           "http://100.64.1.2/mcp",
		"link local ipv4":      "http://169.254.1.2/mcp",
		"multicast ipv4":       "http://224.0.0.1/mcp",
		"decimal ipv4":         "http://2130706433/mcp",
		"short ipv4":           "http://127.1/mcp",
		"octal ipv4":           "http://0177.0.0.1/mcp",
		"hex ipv4":             "http://0x7f000001/mcp",
		"ipv6 loopback":        "http://[::1]/mcp",
		"ipv6 unspecified":     "http://[::]/mcp",
		"ipv6 private":         "http://[fd00::1]/mcp",
		"ipv6 nat64":           "http://[64:ff9b::7f00:1]/mcp",
		"ipv6 six to four":     "http://[2002:7f00:1::]/mcp",
		"ipv4 mapped ipv6":     "http://[::ffff:127.0.0.1]/mcp",
		"ipv6 link local zone": "http://[fe80::1%25en0]/mcp",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseMCPRemoteEndpoint(rawURL); !errors.Is(err, ErrMCPRemoteDestinationDisallowed) {
				t.Fatalf("non-public endpoint accepted: raw=%q err=%v", rawURL, err)
			}
		})
	}
}

func TestMCPRemoteEndpointAllowsDirectPublicHTTPAndHTTPS(t *testing.T) {
	for _, rawURL := range []string{
		"http://93.184.216.34/mcp",
		"https://mcp.example.com/v1?transport=streamable",
	} {
		endpoint, err := parseMCPRemoteEndpoint(rawURL)
		if err != nil {
			t.Fatalf("public endpoint rejected: raw=%q err=%v", rawURL, err)
		}
		if endpoint.scheme != "http" && endpoint.scheme != "https" {
			t.Fatalf("unexpected endpoint: %#v", endpoint)
		}
	}
}

func TestMCPRemoteDialRejectsEntireDNSAnswerSetBeforeDial(t *testing.T) {
	dialer := &recordingRemoteDialer{}
	dial := secureMCPRemoteDialContext(staticRemoteResolver{addresses: []netip.Addr{
		netip.MustParseAddr("93.184.216.34"),
		netip.MustParseAddr("127.0.0.1"),
	}}, dialer)

	connection, err := dial(context.Background(), "tcp", "mcp.example.com:443")
	if connection != nil {
		_ = connection.Close()
	}
	if !errors.Is(err, ErrMCPRemoteDestinationDisallowed) {
		t.Fatalf("mixed public/private DNS answer was accepted: %v", err)
	}
	if dialer.count() != 0 {
		t.Fatalf("dial occurred before the complete DNS answer set passed: calls=%d", dialer.count())
	}
}

func TestMCPRemoteDialRejectsResolutionFailureAndZonedAddressBeforeDial(t *testing.T) {
	for name, resolver := range map[string]staticRemoteResolver{
		"resolver error": {err: errors.New("synthetic DNS failure")},
		"empty answer":   {},
		"zoned address":  {addresses: []netip.Addr{netip.MustParseAddr("2606:2800:220:1:248:1893:25c8:1946").WithZone("en0")}},
	} {
		t.Run(name, func(t *testing.T) {
			dialer := &recordingRemoteDialer{}
			dial := secureMCPRemoteDialContext(resolver, dialer)
			connection, err := dial(context.Background(), "tcp", "mcp.example.com:443")
			if connection != nil {
				_ = connection.Close()
			}
			if !errors.Is(err, ErrMCPRemoteDestinationDisallowed) || dialer.count() != 0 {
				t.Fatalf("resolution uncertainty was not fail-closed: err=%v calls=%d", err, dialer.count())
			}
		})
	}
}

func TestMCPRemoteDialPinsValidatedNumericAddress(t *testing.T) {
	dialer := &recordingRemoteDialer{}
	dial := secureMCPRemoteDialContext(staticRemoteResolver{addresses: []netip.Addr{
		netip.MustParseAddr("93.184.216.34"),
	}}, dialer)

	connection, err := dial(context.Background(), "tcp", "mcp.example.com:443")
	if err != nil {
		t.Fatalf("validated public destination failed: %v", err)
	}
	_ = connection.Close()
	if got := dialer.firstAddress(); got != "93.184.216.34:443" {
		t.Fatalf("dial was not pinned to the validated numeric address: %q", got)
	}
}

func TestMCPRemoteClientDisablesProxyAndPreservesTLSServerName(t *testing.T) {
	if _, err := newClientForRemoteEndpoint(
		"https://mcp.example.com/v1",
		"http://proxy.example.com:8080",
		time.Second,
	); !errors.Is(err, ErrMCPRemoteProxyDisallowed) {
		t.Fatalf("generic MCP proxy was accepted: %v", err)
	}

	client, err := newClientForRemoteEndpoint("https://mcp.example.com/v1", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil || transport.TLSClientConfig == nil ||
		transport.TLSClientConfig.ServerName != "mcp.example.com" {
		t.Fatalf("direct TLS client lost destination identity: %#v", client.Transport)
	}
}

func TestNewTransportClientRejectsLoopbackBeforeMCPHandler(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	_, err := NewTransportClient(domainmcp.ServerSpec{
		ID: "loopback-denied", Transport: "http", URL: server.URL,
	}, "")
	if !errors.Is(err, ErrHTTPClientConfiguration) {
		t.Fatalf("loopback MCP endpoint did not fail configuration: %T %v", err, err)
	}
	if requests.Load() != 0 {
		t.Fatalf("loopback MCP handler was reached: requests=%d", requests.Load())
	}
}

func TestMCPRemoteDestinationFailureInvalidatesConnectionIdentity(t *testing.T) {
	err := &TransportError{Method: "tools/list", Cause: ErrMCPRemoteDestinationDisallowed}
	if !err.MCPTransportInvalidatesIdentity() || err.MCPTransportUnavailable() {
		t.Fatalf("DNS rebinding rejection did not revoke MCP identity: %#v", err)
	}
}
