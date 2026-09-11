package netclient

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestCustomProxyURLAndSummaryRedactsPassword(t *testing.T) {
	spec := ProxySpec{
		Mode:     ModeCustom,
		Type:     "socks5",
		Server:   "127.0.0.1",
		Port:     7890,
		Username: "user",
		Password: "secret",
	}
	proxy, err := ProxyFunc(spec)
	if err != nil {
		t.Fatalf("ProxyFunc: %v", err)
	}
	got, err := proxy(&http.Request{URL: mustNetclientURL(t, "https://api.deepseek.com/chat/completions")})
	if err != nil {
		t.Fatalf("proxy lookup: %v", err)
	}
	if got.Scheme != "socks5" || got.Host != "127.0.0.1:7890" {
		t.Fatalf("proxy URL = %s, want socks5://127.0.0.1:7890", got)
	}
	if pass, ok := got.User.Password(); !ok || pass != "secret" {
		t.Fatalf("proxy password not preserved in transport URL")
	}
	if summary := Summary(spec); summary != "custom (socks5://user@127.0.0.1:7890)" {
		t.Fatalf("Summary should redact proxy password, got %q", summary)
	}
}

func TestSocksProxyURLAliasNormalizesToSocks5(t *testing.T) {
	proxy, err := ProxyFunc(ProxySpec{
		Mode: ModeCustom,
		URL:  "socks://proxy-user:proxy-secret@127.0.0.1:7890",
	})
	if err != nil {
		t.Fatalf("ProxyFunc should accept socks:// as a socks5 alias: %v", err)
	}
	got, err := proxy(&http.Request{URL: mustNetclientURL(t, "https://api.deepseek.com/chat/completions")})
	if err != nil {
		t.Fatalf("proxy lookup: %v", err)
	}
	if got.Scheme != "socks5" || got.Host != "127.0.0.1:7890" {
		t.Fatalf("socks:// alias should normalize to socks5://, got %s", got)
	}
	if summary := Summary(ProxySpec{Mode: ModeCustom, URL: "socks://proxy-user:proxy-secret@127.0.0.1:7890"}); summary != "custom (socks5://proxy-user@127.0.0.1:7890)" {
		t.Fatalf("Summary should normalize and redact socks proxy URL, got %q", summary)
	}
}

func TestUnsupportedSocks4ProxyURLIsRejected(t *testing.T) {
	if err := Validate(ProxySpec{Mode: ModeCustom, URL: "socks4://127.0.0.1:7890"}); err == nil {
		t.Fatalf("socks4 proxy URLs should remain unsupported until a transport implementation exists")
	}
}

func TestProxyFuncHonorsNoProxyAndDirectHosts(t *testing.T) {
	proxy, err := ProxyFunc(ProxySpec{
		Mode:        ModeCustom,
		URL:         "http://proxy.example.com:8080",
		NoProxy:     "api.deepseek.com",
		DirectHosts: []string{"token-plan-cn.xiaomimimo.com"},
	})
	if err != nil {
		t.Fatalf("ProxyFunc: %v", err)
	}
	for _, rawURL := range []string{
		"https://api.deepseek.com/chat/completions",
		"https://sub.api.deepseek.com/chat/completions",
		"https://token-plan-cn.xiaomimimo.com/v1/chat",
		"https://region.token-plan-cn.xiaomimimo.com/v1/chat",
	} {
		got, err := proxy(&http.Request{URL: mustNetclientURL(t, rawURL)})
		if err != nil {
			t.Fatalf("proxy lookup for %s: %v", rawURL, err)
		}
		if got != nil {
			t.Fatalf("%s should bypass proxy, got %s", rawURL, got)
		}
	}
	got, err := proxy(&http.Request{URL: mustNetclientURL(t, "https://example.com/resource")})
	if err != nil {
		t.Fatalf("proxy lookup: %v", err)
	}
	if got == nil || got.String() != "http://proxy.example.com:8080" {
		t.Fatalf("non-direct host should use custom proxy, got %v", got)
	}
}

func TestNewTransportAppliesTimeoutKnobs(t *testing.T) {
	transport, err := NewTransport(ProxySpec{Mode: ModeOff}, TransportOptions{
		DialTimeout:           3 * time.Second,
		KeepAlive:             4 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 6 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	if transport.Proxy != nil {
		t.Fatalf("off mode should disable proxy")
	}
	if transport.DialContext == nil {
		t.Fatalf("timeout options should install a DialContext")
	}
	if transport.TLSHandshakeTimeout != 5*time.Second || transport.ResponseHeaderTimeout != 6*time.Second {
		t.Fatalf("timeout knobs not applied: %#v", transport)
	}
}

func mustNetclientURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse URL %q: %v", raw, err)
	}
	return parsed
}
