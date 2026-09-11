package webfetch

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	netclient "analytix.local/runtime-go/internal/netclient"
)

func TestValidateURLAppliesSSRFAndDomainPolicy(t *testing.T) {
	if _, failure, failed := ValidateURL("http://169.254.169.254/latest", Policy{}); !failed || stringField(failure, "code") != "ssrf_blocked" {
		t.Fatalf("link-local URL should be blocked: failed=%v failure=%#v", failed, failure)
	}
	if _, failure, failed := ValidateURL("https://docs.example.com/path", Policy{DenyDomains: []string{"example.com"}}); !failed || stringField(failure, "code") != "policy_blocked" {
		t.Fatalf("denied domain should be blocked: failed=%v failure=%#v", failed, failure)
	}
	if _, failure, failed := ValidateURL("https://other.example.net/path", Policy{AllowDomains: []string{"example.com"}}); !failed || stringField(failure, "code") != "policy_blocked" {
		t.Fatalf("outside allowlist should be blocked: failed=%v failure=%#v", failed, failure)
	}
	parsed, failure, failed := ValidateURL("https://docs.example.com/path", Policy{AllowDomains: []string{"example.com"}})
	if failed || parsed == nil || parsed.Hostname() != "docs.example.com" {
		t.Fatalf("allowed domain should pass: parsed=%v failed=%v failure=%#v", parsed, failed, failure)
	}
}

func TestParseRequestAppliesCapsAndDefaults(t *testing.T) {
	request, failure, failed := ParseRequest(map[string]any{
		"url":       "https://example.com",
		"max_bytes": 1_000_000.0,
	}, 4096, 8192, 512, 2*time.Second)
	if failed || failure != nil {
		t.Fatalf("request should parse: failed=%v failure=%#v", failed, failure)
	}
	if request.URL != "https://example.com" || request.MaxBytes != 4096 || request.Timeout != 2*time.Second {
		t.Fatalf("request mismatch: %#v", request)
	}
	request, _, failed = ParseRequest(map[string]any{"url": "https://example.com", "max_bytes": 10.0}, 4096, 8192, 512, time.Second)
	if failed || request.MaxBytes != 512 {
		t.Fatalf("request should enforce minimum: %#v failed=%v", request, failed)
	}
	if _, failure, failed := ParseRequest(map[string]any{}, 4096, 8192, 512, time.Second); !failed || stringField(failure, "code") != "invalid_url" {
		t.Fatalf("missing URL should fail: failed=%v failure=%#v", failed, failure)
	}
}

func TestProxySpecForURLDefaultsAndCustomizes(t *testing.T) {
	if got := ProxySpecForURL(" "); got.Mode != netclient.ModeAuto || got.URL != "" {
		t.Fatalf("empty proxy should use auto mode: %#v", got)
	}
	if got := ProxySpecForURL(" http://127.0.0.1:8080 "); got.Mode != netclient.ModeCustom || got.URL != "http://127.0.0.1:8080" {
		t.Fatalf("custom proxy mismatch: %#v", got)
	}
}

func TestClientFetchProducesSanitizedSourceAndProgress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "analytix-web-fetch/1.0" {
			t.Fatalf("user agent mismatch: %q", got)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>Docs</title></head><body><p>Hello</p></body></html>`))
	}))
	defer server.Close()
	progressHost := ""
	output, isError := Client{
		ProviderID:      "web",
		DefaultMaxBytes: 1024,
		DefaultTimeout:  time.Second,
		Now:             func() time.Time { return time.Unix(100, 0).UTC() },
		OnProgress:      func(host string) { progressHost = host },
	}.Fetch(context.Background(), Request{URL: server.URL + "/path", MaxBytes: 1024, Timeout: time.Second})
	if isError {
		t.Fatalf("fetch should succeed: %#v", output)
	}
	if progressHost == "" {
		t.Fatalf("progress host should be reported")
	}
	if stringField(output, "title") != "Docs" || stringField(output, "text") != "Docs Hello" {
		t.Fatalf("output text mismatch: %#v", output)
	}
	if stringField(output, "retrievedAt") != "1970-01-01T00:01:40Z" {
		t.Fatalf("retrievedAt mismatch: %#v", output["retrievedAt"])
	}
	if _, ok := output["sources"].([]any); !ok {
		t.Fatalf("sources should be included: %#v", output["sources"])
	}
}

func TestReadLimitedResponseAndExtractReadableText(t *testing.T) {
	body, truncated, err := ReadLimitedResponse(strings.NewReader("abcdef"), 3, 10)
	if err != nil || string(body) != "abc" || !truncated {
		t.Fatalf("limited response mismatch: body=%q truncated=%v err=%v", body, truncated, err)
	}
	title, text := ExtractReadableText(`<!doctype html><html><head><title>Hello &amp; Docs</title><style>.x{}</style><script>x()</script></head><body><h1>Fetched</h1><!--drop--><p>Text</p></body></html>`, "text/html")
	if title != "Hello & Docs" || text != "Hello & Docs Fetched Text" {
		t.Fatalf("html extraction mismatch: title=%q text=%q", title, text)
	}
	plainTitle, plain := ExtractReadableText("hello\n\nworld", "text/plain")
	if plainTitle != "" || plain != "hello world" {
		t.Fatalf("plain extraction mismatch: title=%q text=%q", plainTitle, plain)
	}
}

func TestURLSanitizationAndIdentity(t *testing.T) {
	if got := SanitizeURL("https://user:secret@example.com/a?b=c"); got != "https://example.com/a?b=c" {
		t.Fatalf("sanitized URL mismatch: %q", got)
	}
	if SourceID("https://example.com/a") != SourceID("https://example.com/a") || SourceID("https://example.com/a") == SourceID("https://example.com/b") {
		t.Fatalf("source IDs should be deterministic and URL-specific")
	}
	if !DomainMatches("docs.example.com", ".example.com") || DomainMatches("badexample.com", "example.com") {
		t.Fatalf("domain matching mismatch")
	}
}

func TestBlockedIPKeepsLoopbackForLocalContractTests(t *testing.T) {
	if BlockedIP(net.ParseIP("127.0.0.1")) {
		t.Fatalf("loopback must remain allowed for local contract provider tests")
	}
	for _, raw := range []string{"10.0.0.1", "172.16.0.1", "192.168.1.1", "169.254.1.1", "100.64.0.1"} {
		if !BlockedIP(net.ParseIP(raw)) {
			t.Fatalf("private/link-local/cgnat IP %s should be blocked", raw)
		}
	}
}

func stringField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}
