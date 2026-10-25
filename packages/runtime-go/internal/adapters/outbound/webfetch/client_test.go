package webfetch

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
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

func TestClientFetchReportsActualResponseMetadata(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    string
		title   string
		text    string
		outcome string
		isError bool
	}{
		{name: "created", status: http.StatusCreated, body: "<title>Created</title><p>retained model body</p>", title: "Created", text: "Created retained model body", outcome: "success"},
		{name: "no content", status: http.StatusNoContent, outcome: "success"},
		{name: "HTTP failure", status: http.StatusServiceUnavailable, body: "<title>Unavailable</title><p>retained failure body</p>", title: "Unavailable", text: "Unavailable retained failure body", outcome: "http_error", isError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(tc.status)
				if tc.body != "" {
					_, _ = w.Write([]byte(tc.body))
				}
			}))
			defer server.Close()
			requestURL := server.URL + "/page"
			output, isError := Client{ProxySpec: netclient.ProxySpec{Mode: netclient.ModeOff}, DefaultMaxBytes: 1024}.Fetch(
				context.Background(), Request{URL: requestURL, MaxBytes: 1024, Timeout: time.Second})
			if isError != tc.isError {
				t.Fatalf("response error mismatch: want=%v output=%#v", tc.isError, output)
			}
			assertFetchResponseMetadata(t, output, tc.outcome, tc.status, len(tc.body), false)
			if stringField(output, "text") != tc.text || stringField(output, "title") != tc.title {
				t.Fatalf("response metadata must preserve existing model text and title: %#v", output)
			}
			if stringField(output, "url") != requestURL || stringField(output, "finalUrl") != requestURL {
				t.Fatalf("response metadata must preserve existing source URLs: %#v", output)
			}
			sources, ok := output["sources"].([]any)
			if !ok || len(sources) != 1 {
				t.Fatalf("response must retain its existing source: %#v", output["sources"])
			}
			source, ok := sources[0].(map[string]any)
			if !ok || stringField(source, "url") != requestURL || stringField(source, "title") != tc.title {
				t.Fatalf("response source mismatch: %#v", sources[0])
			}
			citations, ok := output["citations"].([]any)
			if !ok || len(citations) != 1 {
				t.Fatalf("response must retain its existing citation: %#v", output["citations"])
			}
			if tc.isError && stringField(output, "code") != "fetch_failed" {
				t.Fatalf("HTTP failure must retain its error code: %#v", output)
			}
		})
	}
}

func TestClientFetchReportsMeasuredTruncation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("abcdef"))
	}))
	defer server.Close()
	output, isError := Client{ProxySpec: netclient.ProxySpec{Mode: netclient.ModeOff}, DefaultMaxBytes: 1024}.Fetch(
		context.Background(), Request{URL: server.URL, MaxBytes: 3, Timeout: time.Second})
	if isError {
		t.Fatalf("capped successful response should succeed: %#v", output)
	}
	assertFetchResponseMetadata(t, output, "success", http.StatusOK, 3, true)
	if stringField(output, "text") != "abc" {
		t.Fatalf("retained model text must match the measured body cap: %#v", output)
	}
}

func TestClientFetchReadFailureRetainsOnlyObservedMeasurements(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Length", "10")
		w.Header().Set("Connection", "close")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("abc"))
	}))
	defer server.Close()
	output, isError := Client{ProxySpec: netclient.ProxySpec{Mode: netclient.ModeOff}, DefaultMaxBytes: 1024}.Fetch(
		context.Background(), Request{URL: server.URL, MaxBytes: 1024, Timeout: time.Second})
	if !isError || stringField(output, "outcome") != "read_error" || !strings.Contains(stringField(output, "error"), "unexpected EOF") {
		t.Fatalf("incomplete HTTP body should report a read failure: %#v", output)
	}
	if output["statusCode"] != float64(http.StatusOK) || output["byteCount"] != float64(3) {
		t.Fatalf("read failure must retain observed response and partial bytes: %#v", output)
	}
	if _, exists := output["truncated"]; exists {
		t.Fatalf("read failure cannot claim a known truncation state: %#v", output)
	}
}

func TestClientFetchBeforeResponseFailuresHaveNoMeasurements(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	for _, tc := range []struct {
		name    string
		context func() (context.Context, context.CancelFunc)
		outcome string
	}{
		{name: "cancelled", context: func() (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx, cancel
		}, outcome: "cancelled"},
		{name: "deadline elapsed", context: func() (context.Context, context.CancelFunc) {
			return context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		}, outcome: "timeout"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := tc.context()
			defer cancel()
			output, isError := Client{ProxySpec: netclient.ProxySpec{Mode: netclient.ModeOff}}.Fetch(
				ctx, Request{URL: server.URL, MaxBytes: 1024, Timeout: time.Second})
			if !isError || stringField(output, "outcome") != tc.outcome {
				t.Fatalf("pre-response failure outcome mismatch: want=%s output=%#v", tc.outcome, output)
			}
			assertFetchMeasurementsAbsent(t, output)
		})
	}
	if received.Load() != 0 {
		t.Fatalf("cancelled or elapsed contexts must not contact the local HTTP handler")
	}

	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("closed local listener must not receive a request")
	}))
	closedURL := closed.URL
	closed.Close()
	output, isError := Client{ProxySpec: netclient.ProxySpec{Mode: netclient.ModeOff}}.Fetch(
		context.Background(), Request{URL: closedURL, MaxBytes: 1024, Timeout: time.Second})
	if !isError || stringField(output, "outcome") != "transport_error" {
		t.Fatalf("closed local listener should report a transport failure: %#v", output)
	}
	assertFetchMeasurementsAbsent(t, output)
}

func TestClientFetchRejectedRedirectKeepsResponseStatusWithoutBodyMeasurements(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "file:///synthetic/blocked")
		w.WriteHeader(http.StatusFound)
		_, _ = w.Write([]byte("redirect body is not consumed"))
	}))
	defer server.Close()
	output, isError := Client{ProxySpec: netclient.ProxySpec{Mode: netclient.ModeOff}}.Fetch(
		context.Background(), Request{URL: server.URL, MaxBytes: 1024, Timeout: time.Second})
	if !isError || stringField(output, "outcome") != "policy_blocked" {
		t.Fatalf("redirect policy refusal should retain its outcome: %#v", output)
	}
	if output["statusCode"] != float64(http.StatusFound) {
		t.Fatalf("redirect refusal must retain the actual received status: %#v", output)
	}
	for _, key := range []string{"byteCount", "truncated"} {
		if _, exists := output[key]; exists {
			t.Fatalf("unread redirect body must omit %s: %#v", key, output)
		}
	}
}

func assertFetchResponseMetadata(t *testing.T, output map[string]any, outcome string, statusCode, byteCount int, truncated bool) {
	t.Helper()
	if stringField(output, "outcome") != outcome || output["statusCode"] != float64(statusCode) || output["byteCount"] != float64(byteCount) || output["truncated"] != truncated {
		t.Fatalf("measured response metadata mismatch: outcome=%s statusCode=%d byteCount=%d truncated=%v output=%#v", outcome, statusCode, byteCount, truncated, output)
	}
}

func assertFetchMeasurementsAbsent(t *testing.T, output map[string]any) {
	t.Helper()
	for _, key := range []string{"statusCode", "byteCount", "truncated"} {
		if _, exists := output[key]; exists {
			t.Fatalf("failure before a response must omit %s: %#v", key, output)
		}
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

func TestClientFetchCapturesRealCompleteResponseAndFailureV1(t *testing.T) {
	for _, status := range []int{http.StatusCreated, http.StatusNotFound, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			const body = "<title>Observed title</title><p>" + "ordinary full body" + "</p>"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/request" {
					http.Redirect(w, r, "/observed?source=actual", http.StatusFound)
					return
				}
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			ctx := domaintoolresult.WithProtectedCaptureV1(context.Background(), &domaintoolresult.ProtectedCaptureCollectorV1{})
			output, isError := Client{ProxySpec: netclient.ProxySpec{Mode: netclient.ModeOff}}.Fetch(ctx, Request{URL: server.URL + "/request", MaxBytes: 1000000, Timeout: time.Second})
			capture, ok := domaintoolresult.ConsumeProtectedToolResultV1(ctx)
			if !ok || capture.Kind != "web_fetch" || capture.Status != "completed" || capture.Body != "Observed title ordinary full body" || capture.Truncated || capture.Web == nil || !capture.Web.ResponseComplete || capture.Web.ToolIsError != isError || capture.Web.StatusCode == nil || *capture.Web.StatusCode != status || capture.Web.FetchTruncated == nil || *capture.Web.FetchTruncated || capture.Web.Title != "Observed title" {
				t.Fatal("actual complete response capture missing or fabricated")
			}
			if capture.Web.RequestURL != server.URL+"/request" || capture.Web.FinalURL != server.URL+"/observed?source=actual" || capture.WebTargets == nil || capture.WebTargets.RequestURL != capture.Web.RequestURL || capture.WebTargets.ResultURL != capture.Web.FinalURL {
				t.Fatal("actual request/final URL or sealed target lost")
			}
			if status >= 400 && (capture.Web.Error == nil || capture.Web.Error.Message != stringField(output, "error")) {
				t.Fatal("actual error missing from non-2xx capture")
			}
			if _, again := domaintoolresult.ConsumeProtectedToolResultV1(ctx); again {
				t.Fatal("capture is not single-use")
			}
		})
	}
}

func TestClientFetchCaptureHasOwnExplicitByteBudgetV1(t *testing.T) {
	for _, size := range []int{70_000, 1_000_000, 1_000_003} {
		t.Run(strings.Repeat("a", 1)+http.StatusText(http.StatusOK)+"-"+time.Duration(size).String(), func(t *testing.T) {
			body := strings.Repeat("a", size)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			ctx := domaintoolresult.WithProtectedCaptureV1(context.Background(), &domaintoolresult.ProtectedCaptureCollectorV1{})
			output, isError := Client{ProxySpec: netclient.ProxySpec{Mode: netclient.ModeOff}}.Fetch(ctx, Request{URL: server.URL, MaxBytes: size, Timeout: time.Second})
			capture, ok := domaintoolresult.ConsumeProtectedToolResultV1(ctx)
			if isError || !ok || len(stringField(output, "text")) != size || len(capture.Body) != min(size, 1_000_000) || capture.Truncated != (size > 1_000_000) || capture.Web.FetchTruncated == nil || *capture.Web.FetchTruncated {
				t.Fatal("capture budget changed producer result or concealed truncation")
			}
		})
	}
}

func TestClientFetchIncompleteReadDoesNotClaimCompleteCardV1(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "10")
		w.Header().Set("Connection", "close")
		_, _ = w.Write([]byte("abc"))
	}))
	defer server.Close()
	ctx := domaintoolresult.WithProtectedCaptureV1(context.Background(), &domaintoolresult.ProtectedCaptureCollectorV1{})
	output, isError := Client{ProxySpec: netclient.ProxySpec{Mode: netclient.ModeOff}}.Fetch(ctx, Request{URL: server.URL, MaxBytes: 1024, Timeout: time.Second})
	capture, ok := domaintoolresult.ConsumeProtectedToolResultV1(ctx)
	if !isError || !ok || capture.Status != "failed" || capture.Web.ResponseComplete || capture.Web.FetchTruncated != nil || capture.Web.StatusCode == nil || *capture.Web.StatusCode != http.StatusOK || capture.Body != stringField(output, "error") || capture.Web.Error == nil || capture.Web.Error.Message != stringField(output, "error") {
		t.Fatal("incomplete read invented complete response or lost actual error")
	}
}

func TestClientFetchDoesNotTurnURLPrefixIntoTargetV1(t *testing.T) {
	ctx := domaintoolresult.WithProtectedCaptureV1(context.Background(), &domaintoolresult.ProtectedCaptureCollectorV1{})
	_, failed := Client{Policy: Policy{DenyDomains: []string{"blocked.example"}}}.Fetch(ctx, Request{URL: "https://blocked.example/" + strings.Repeat("x", 8192), MaxBytes: 1024, Timeout: time.Second})
	// The deny rule stops before any network request.
	capture, ok := domaintoolresult.ConsumeProtectedToolResultV1(ctx)
	if !failed || !ok || capture.Web.RequestURL != "" || !capture.Web.RequestURLTruncated || capture.WebTargets != nil {
		t.Fatal("a URL prefix became a display source or navigation target")
	}
}

func TestClientFetchUnrepresentableURLAndTitleKeepActualFailureV1(t *testing.T) {
	for _, input := range []string{"https://example.test/\x00", "https://example.test/" + string([]byte{0xff})} {
		ctx := domaintoolresult.WithProtectedCaptureV1(context.Background(), &domaintoolresult.ProtectedCaptureCollectorV1{})
		output, isError := Client{Policy: Policy{DenyDomains: []string{"example.test"}}}.Fetch(ctx, Request{URL: input, MaxBytes: 1024, Timeout: time.Second})
		capture, ok := domaintoolresult.ConsumeProtectedToolResultV1(ctx)
		if !isError || !ok || capture.Web.RequestURL != "" || capture.Web.RequestURLTruncated || capture.WebTargets != nil || !capture.Truncated || capture.Body != stringField(output, "error") {
			t.Fatal("unrepresentable URL dropped actual failure or invented a budget truncation")
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<title>actual\x00title</title><p>body</p>"))
	}))
	defer server.Close()
	ctx := domaintoolresult.WithProtectedCaptureV1(context.Background(), &domaintoolresult.ProtectedCaptureCollectorV1{})
	output, isError := Client{ProxySpec: netclient.ProxySpec{Mode: netclient.ModeOff}}.Fetch(ctx, Request{URL: server.URL, MaxBytes: 1024, Timeout: time.Second})
	capture, ok := domaintoolresult.ConsumeProtectedToolResultV1(ctx)
	if isError || !ok || !capture.Truncated || capture.Web.Title != "" || stringField(output, "title") != "actual\x00title" || capture.Body != stringField(output, "text") {
		t.Fatal("display title bound changed model output or dropped real body")
	}
}
